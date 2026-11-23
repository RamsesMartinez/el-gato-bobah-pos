-- Almacenes de una sucursal (spec 094). La empresa la pone RLS; la sucursal, current_branch_id().

-- name: LockWarehouses :exec
-- Alta, renombre y archivo deciden sobre el conjunto de la sucursal (nombre libre, de venta): sin
-- este candado por empresa, dos altas con el mismo nombre a la vez pasarían el chequeo las dos.
select pg_advisory_xact_lock(28094, coalesce(nullif(current_setting('app.company_id', true), '')::int, 0));

-- name: ListWarehouses :many
-- Los de la sucursal, el de venta primero; con cuántos artículos tienen existencia distinta de cero.
select w.id, w.name, w.is_sales, w.is_active,
       (select count(*) from stock_levels s where s.warehouse_id = w.id and s.on_hand <> 0)::bigint as items_with_stock
  from warehouses w
 where w.branch_id = current_branch_id()
   and (w.is_active or sqlc.arg(include_archived)::boolean)
 order by w.is_sales desc, w.is_active desc, w.name, w.id;

-- name: CreateWarehouse :one
insert into warehouses (branch_id, name) values (current_branch_id(), sqlc.arg(name))
returning id;

-- name: WarehouseByName :one
-- El activo de la sucursal con ese nombre normalizado (la misma expresión que el índice).
select id from warehouses
 where branch_id = current_branch_id() and is_active
   and catalog_name_key(name) = catalog_name_key(sqlc.arg(name)::text);

-- name: LockWarehouse :one
-- for update: archivar espera a un traspaso o una compra en curso que lo tomaron for share.
select id, name, is_sales, is_active from warehouses
 where id = sqlc.arg(id) and branch_id = current_branch_id()
   for update;

-- name: RenameWarehouse :exec
update warehouses set name = sqlc.arg(name) where id = sqlc.arg(id);

-- name: SetWarehouseActive :exec
update warehouses set is_active = sqlc.arg(is_active) where id = sqlc.arg(id);

-- name: WarehouseItemsWithStock :one
select count(*)::bigint from stock_levels where warehouse_id = sqlc.arg(id) and on_hand <> 0;

-- name: WarehouseHasOpenCount :one
select exists (select 1 from stock_counts where warehouse_id = sqlc.arg(id) and status = 'open');

-- name: ResolveWarehouse :one
-- El almacén que pidió una lectura o escritura, si es de la sucursal de la sesión y está activo. Uno
-- de otra empresa no existe para RLS; uno de otra sucursal o archivado tampoco se ofrece.
-- for share: dentro de la transacción que escribe, archivarlo (for update) espera a que termine; sin
-- eso, un movimiento podía quedar en un almacén que se archivó en medio.
select id, name, is_sales from warehouses
 where id = sqlc.arg(id) and branch_id = current_branch_id() and is_active
   for share;

-- name: SalesWarehouse :one
select id, name, is_sales from warehouses where id = current_sales_warehouse_id();

-- name: ClearSalesWarehouse :exec
-- Quita la marca al de venta de la sucursal; en la misma transacción se la pone otro (la 0122 exige,
-- al confirmar, que la sucursal tenga uno).
update warehouses set is_sales = false where branch_id = current_branch_id() and is_sales;

-- name: MarkSalesWarehouse :exec
update warehouses set is_sales = true where id = sqlc.arg(id);

-- name: WarehouseHasPendingPurchases :one
-- Renglones de compra vivos que todavía esperan mercancía para este almacén: archivarlo dejaría que lo
-- que llegue entre a un almacén que no sale en ninguna lista.
select exists (
  select 1 from expense_items ei join expenses e on e.id = ei.expense_id
   where ei.warehouse_id = sqlc.arg(id) and ei.item_type is not null and ei.voided_at is null and not ei.excluded
     and e.status <> 'cancelada'
     and coalesce(ei.qty_received, 0) + ei.not_coming_qty < ei.quantity);
