-- Depleción en venta

-- name: InsertStockMovement :exec
-- order_line_id: de QUÉ renglón salió este descuento.
--
-- Sin él, reponer un renglón cancelado obliga a recalcular su consumo con la receta de HOY, y una
-- receta que cambió entre la venta y la cancelación repondría una cantidad distinta de la que salió.
-- NULL en los movimientos que no vienen de una venta (ajustes, compras, mermas).
-- modifier_option_id / component_of_product_id: el extra o el paquete que originó el descuento.
-- warehouse_id: nulo en una venta (el trigger pone el de venta); explícito al partir un renglón, que
-- reparte movimientos que ya salieron de un almacén (spec 094).
insert into stock_movements (item_type, ingredient_id, product_id, movement_type, quantity, unit_cost, order_id, order_line_id, user_id, reason, note,
                             modifier_option_id, component_of_product_id, warehouse_id)
values ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,sqlc.narg(warehouse_id));

-- Almacén / niveles

-- name: ListStockLevels :many
-- Los almacenes que se piden, sumados por artículo (spec 094): uno solo (el de venta por omisión, o
-- el elegido) o todos los de la sucursal. Con un almacén, la suma es su cifra.
select sl.item_type,
       i.id as ingredient_id,
       p.id as product_id,
       coalesce(i.name, p.name) as item_name,
       sum(sl.on_hand)::numeric(14,4) as on_hand,
       coalesce(i.min_stock, p.min_stock) as min_stock,
       -- El mínimo propio del almacén leído (min_warehouse_id): el servicio decide con él y el del
       -- artículo cuál vale en ese almacén.
       bsm.min_stock as own_min_stock,
       coalesce(iu.code, 'pieza') as unit_code,
       -- El `true` final no cambia nada (todo renglón es de un insumo o de un producto): deja la
       -- columna no nulable sin depender de cómo infiera sqlc un coalesce de dos left join.
       coalesce(i.is_active, p.is_active, true) as is_active,
       -- Valor al costo vigente, almacén por almacén y sin negativos: lo que falta en un almacén no le
       -- resta valor a lo que sí hay en otro, y así la suma de los almacenes es el valor de todos.
       round(sum(greatest(sl.on_hand, 0)) * coalesce(i.current_cost, p.current_cost, 0), 2)::numeric(14,2) as value
from stock_levels sl
left join ingredients i on i.id = sl.ingredient_id
left join units iu on iu.id = i.base_unit_id
left join products p on p.id = sl.product_id
left join branch_stock_minimums bsm on bsm.warehouse_id = sqlc.narg('min_warehouse_id')::bigint
      and (bsm.ingredient_id = sl.ingredient_id or bsm.product_id = sl.product_id)
where sl.warehouse_id = any(sqlc.arg(warehouse_ids)::bigint[])
  -- Es la lista de faltantes de la pantalla (marca «bajo»): un insumo archivado ahí es un faltante
  -- falso (spec 077). Su existencia sigue en Almacén › Archivados. Un producto archivado sí sale,
  -- marcado: desactivar productos no es de esa feature.
  and (sl.ingredient_id is null or i.is_active)
group by sl.item_type, i.id, p.id, iu.code, bsm.min_stock
order by item_name;

-- name: ListStockMovements :many
-- Los últimos movimientos de los almacenes pedidos (spec 094). De un traspaso viajan sus dos
-- almacenes: la contraparte de cada pata se decide en el servicio. Por id y no por hora: el índice
-- por almacén ya los trae en ese orden.
select sm.id, sm.item_type, coalesce(i.name, p.name) as item_name, sm.movement_type,
       sm.quantity, sm.reason, sm.created_at, sm.warehouse_id, w.name as warehouse_name,
       coalesce(iu.code, 'pieza')::text as unit_code, sm.transfer_id,
       t.from_warehouse_id as transfer_from_id, fw.name as transfer_from_name, tw.name as transfer_to_name,
       fw.is_active as transfer_from_active, tw.is_active as transfer_to_active
from stock_movements sm
join warehouses w on w.id = sm.warehouse_id
left join ingredients i on i.id = sm.ingredient_id
left join units iu on iu.id = i.base_unit_id
left join products p on p.id = sm.product_id
left join stock_transfers t on t.id = sm.transfer_id
left join warehouses fw on fw.id = t.from_warehouse_id
left join warehouses tw on tw.id = t.to_warehouse_id
where sm.warehouse_id = any(sqlc.arg(warehouse_ids)::bigint[])
order by sm.id desc
limit sqlc.arg(lim);

-- Cambio de modo de consumo (spec 037, research R6) -----------------------------------------------

-- name: LockProductForComposition :exec
-- El producto ANTES que sus existencias, en el mismo orden que una venta (la llave foránea de su
-- renglón toma el producto y luego el trigger toma la existencia). Guardar la receta cambia
-- recipe_id, que es único, y eso choca con esa llave: tomado después de la existencia, una venta a
-- la mitad cerraba el círculo y Postgres cancelaba una de las dos (40P01).
select 1 from products where id = $1 for update;

-- name: LockProductStockLevels :many
-- Las existencias del producto, una por sucursal, BLOQUEADAS: una venta simultánea espera a que la
-- conversión termine en vez de aplicar su delta sobre una cifra que ya se trasladó. El trigger de
-- existencias suma deltas y no relee nada, así que sin este bloqueo se perdería o se inventaría.
select warehouse_id::bigint as warehouse_id, on_hand from stock_levels
 where item_type = 'producto' and product_id = $1
 order by warehouse_id
   for update;

-- name: LockIngredientStockLevels :many
-- NOWAIT a propósito: una venta bloquea en el orden de su ticket, y si ya tiene tomado el insumo y
-- espera por el producto que esta conversión ya bloqueó, esperar aquí sería un bloqueo mutuo y
-- Postgres cancelaría una de las dos (a veces el cobro). Sin esperar, la conversión falla con
-- 55P03, la app responde 409 y la venta sigue.
select warehouse_id::bigint as warehouse_id, on_hand from stock_levels
 where item_type = 'ingrediente' and ingredient_id = $1
 order by warehouse_id
   for update nowait;

-- name: InsertModeAdjustment :exec
-- Un ajuste del cambio de modo. Va con su almacén explícito (el de la fila bloqueada; la sucursal sale
-- del almacén), la nota que liga los dos movimientos de un traslado y quién lo hizo.
insert into stock_movements (item_type, ingredient_id, product_id, movement_type, quantity, warehouse_id, user_id, reason, note)
values (sqlc.arg(item_type)::stock_item_type, sqlc.narg(ingredient_id), sqlc.narg(product_id), 'ajuste',
        sqlc.arg(quantity), sqlc.arg(warehouse_id)::bigint, sqlc.arg(user_id), 'cambio de modo', sqlc.arg(note));

-- name: TurnOffProductTrackStock :exec
-- Antes de ponerle receta: el check de products no deja existencia propia y receta a la vez.
update products set track_stock = false, updated_at = now() where id = $1;

-- name: GetUnitToIngredientBase :one
-- Cuánto de la unidad base del insumo es una unidad capturada: «1 caja» de un insumo que se lleva
-- por pieza. La captura ya exige que sean del mismo tipo.
-- Con 6 decimales, como units.to_base: la división de Postgres devuelve 20, y multiplicada por una
-- existencia queda fuera de lo que Round4 acepta redondear.
select (u.to_base / bu.to_base)::numeric(20,6) as factor
  from units u, ingredients i join units bu on bu.id = i.base_unit_id
 where u.id = sqlc.arg(unit_id) and i.id = sqlc.arg(ingredient_id);

-- name: GetProductOnHand :one
-- La existencia propia de un producto, sumada: es lo que la hoja de receta muestra al preguntar
-- «¿Cuántas tienes de verdad?». La cifra que manda manda la vuelve a comparar contra la bloqueada.
select coalesce(sum(on_hand), 0)::numeric(14,4) as on_hand,
       -- Sucursales, no filas: desde la 094 cada sucursal tiene una fila por almacén.
       count(distinct branch_id) filter (where on_hand <> 0)::int as branches
  from stock_levels where item_type = 'producto' and product_id = $1;
