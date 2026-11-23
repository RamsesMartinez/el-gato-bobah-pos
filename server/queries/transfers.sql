-- Traspasos entre almacenes de una sucursal (spec 094). La empresa la pone RLS.

-- name: InsertTransfer :one
-- on conflict: un reintento con la misma clave no crea otro; quien llama lee el existente y compara
-- la huella del cuerpo.
insert into stock_transfers (branch_id, from_warehouse_id, to_warehouse_id, note, client_key, body_hash, created_by)
values (current_branch_id(), sqlc.arg(from_warehouse_id), sqlc.arg(to_warehouse_id), sqlc.narg(note),
        sqlc.arg(client_key), sqlc.arg(body_hash), sqlc.arg(created_by))
on conflict (company_id, client_key) do nothing
returning id;

-- name: TransferByClientKey :one
select id, body_hash from stock_transfers where client_key = sqlc.arg(client_key);

-- name: InsertTransferLine :exec
insert into stock_transfer_lines (transfer_id, line_no, item_type, ingredient_id, product_id, qty_base)
values (sqlc.arg(transfer_id), sqlc.arg(line_no), sqlc.arg(item_type), sqlc.narg(ingredient_id), sqlc.narg(product_id), sqlc.arg(qty_base));

-- name: InsertStockTransferMovement :exec
-- Una pata del traspaso: la sucursal la pone el trigger desde el almacén. El costo es el vigente del
-- artículo (snapshot): un traspaso no cambia costos.
insert into stock_movements (warehouse_id, item_type, ingredient_id, product_id, movement_type, quantity, unit_cost,
                             transfer_id, user_id, reason, note)
values (sqlc.arg(warehouse_id), sqlc.arg(item_type), sqlc.narg(ingredient_id), sqlc.narg(product_id), 'traspaso',
        sqlc.arg(quantity), sqlc.narg(unit_cost), sqlc.arg(transfer_id), sqlc.arg(user_id), sqlc.arg(reason), sqlc.narg(note));

-- name: LockProductStockNowait :many
-- Las existencias de los productos de un traspaso, en todos los almacenes de la sucursal, en orden y
-- SIN ESPERAR. Una venta toma la existencia del producto en el orden de su ticket; si el traspaso
-- esperara aquí podría cerrar un ciclo y Postgres cancelaría una de las dos —a veces el cobro—. Así
-- responde 409 «vuelve a intentar» y la venta sigue (lo mismo que LockIngredientStockNowait).
select product_id from stock_levels
 where item_type = 'producto' and product_id = any(sqlc.arg(ids)::bigint[]) and branch_id = current_branch_id()
 order by warehouse_id, product_id
   for update nowait;

-- name: TransferArticles :many
-- Lo que el traspaso necesita de cada artículo: que exista en la empresa, esté activo y su costo vigente.
select 'ingrediente'::stock_item_type as item_type, i.id, i.is_active, i.current_cost
  from ingredients i where i.id = any(sqlc.arg(ingredient_ids)::bigint[])
union all
select 'producto'::stock_item_type, p.id, p.is_active, p.current_cost
  from products p where p.id = any(sqlc.arg(product_ids)::bigint[]) and p.track_stock;

-- name: WarehouseOnHand :many
-- Lo que hay en el origen de los artículos del traspaso (sin candado: la confirmación es un aviso, y
-- dejar el origen en negativo está permitido con ella).
select item_type, ingredient_id, product_id, on_hand from stock_levels
 where warehouse_id = sqlc.arg(warehouse_id)
   and (ingredient_id = any(sqlc.arg(ingredient_ids)::bigint[]) or product_id = any(sqlc.arg(product_ids)::bigint[]));

-- name: ListTransfers :many
select t.id, t.from_warehouse_id, f.name as from_name, t.to_warehouse_id, d.name as to_name, t.note, t.created_at,
       t.created_by, u.name as created_by_name
  from stock_transfers t
  join warehouses f on f.id = t.from_warehouse_id
  join warehouses d on d.id = t.to_warehouse_id
  join users u on u.id = t.created_by
 where t.branch_id = current_branch_id()
 order by t.created_at desc, t.id desc
 limit sqlc.arg(lim);

-- name: ListTransferLines :many
select l.transfer_id, l.line_no, l.item_type, l.ingredient_id, l.product_id, coalesce(i.name, p.name)::text as name,
       coalesce(iu.code, 'pieza')::text as unit_code, l.qty_base
  from stock_transfer_lines l
  left join ingredients i on i.id = l.ingredient_id
  left join units iu on iu.id = i.base_unit_id
  left join products p on p.id = l.product_id
 where l.transfer_id = any(sqlc.arg(transfer_ids)::bigint[])
 order by l.transfer_id, l.line_no;

-- name: TransferSources :many
-- Para cada artículo en negativo en el almacén de venta, los otros almacenes activos de la sucursal que
-- sí tienen: de ahí sale la sugerencia «traer de…» en Existencias.
select s.item_type, s.ingredient_id, s.product_id, w.id as warehouse_id, w.name, o.on_hand
  from stock_levels s
  join stock_levels o on o.branch_id = s.branch_id and o.warehouse_id <> s.warehouse_id and o.on_hand > 0
                     and o.ingredient_id is not distinct from s.ingredient_id and o.product_id is not distinct from s.product_id
  join warehouses w on w.id = o.warehouse_id and w.is_active
 where s.warehouse_id = current_sales_warehouse_id() and s.on_hand < 0
 order by w.name;

-- name: MergeTargets :many
-- A qué insumo se unió cada uno de los pedidos (un salto; quien llama sigue la cadena). Un traspaso de
-- un insumo unido va a su destino, como cualquier movimiento (0104).
select old_ingredient_id, new_ingredient_id from ingredient_replacements
 where kind = 'merge' and old_ingredient_id = any(sqlc.arg(ids)::bigint[]);
