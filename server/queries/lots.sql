-- Lotes con caducidad (spec 098). La empresa la pone RLS; la regla de consumo vive en el trigger de la
-- 0128 (apply_stock_lots), aquí solo lo que los servicios hacen sin un movimiento.

-- name: MoveLotsToIngredient :exec
-- Unión y reemplazo (R7): los lotes del origen pasan al destino por almacén y fecha con la
-- equivalencia (se suman si el destino ya tiene esa fecha) y el origen queda en cero. Quien llama ya
-- tiene tomadas las existencias de los dos artículos.
with src as (
  select l.company_id, l.branch_id, l.warehouse_id, l.expires_on, round(l.qty * sqlc.arg(factor)::numeric, 4) as qty
    from stock_lots l where l.ingredient_id = sqlc.arg(old_id)::bigint and l.qty > 0
  -- Sin `for update`: el update de abajo modifica estas filas en la misma sentencia, y un `for update`
  -- salta las que su propia sentencia ya tocó (el destino se quedaba sin nada). Los lotes de un
  -- artículo se escriben siempre detrás de su fila de stock_levels, que quien llama ya tomó.
), moved as (
  insert into stock_lots as d (company_id, branch_id, warehouse_id, ingredient_id, expires_on, qty)
  select company_id, branch_id, warehouse_id, sqlc.arg(new_id)::bigint, expires_on, qty from src where qty > 0
  on conflict (company_id, warehouse_id, ingredient_id, expires_on)
    do update set qty = d.qty + excluded.qty, updated_at = now()
)
update stock_lots set qty = 0, updated_at = now()
 where ingredient_id = sqlc.arg(old_id)::bigint and qty > 0;

-- name: ScaleIngredientLots :exec
-- Cambio de unidad directo: los lotes del artículo en la unidad nueva.
update stock_lots set qty = round(qty * sqlc.arg(factor)::numeric, 4), updated_at = now()
 where ingredient_id = sqlc.arg(id)::bigint;

-- name: ScaleIngredientLotAllocations :exec
-- Lo que tomó o devolvió cada movimiento, en la unidad nueva: una reposición posterior devuelve lo
-- mismo que se llevó. Lo que redondea a cero se deja como estaba (la regla no admite ceros).
update stock_lot_allocations a set qty = round(a.qty * sqlc.arg(factor)::numeric, 4)
  from stock_lots l
 where l.id = a.lot_id and l.ingredient_id = sqlc.arg(id)::bigint and round(a.qty * sqlc.arg(factor)::numeric, 4) <> 0;

-- name: ScaleIngredientCountLots :exec
-- El desglose de fechas de los conteos del artículo, en la unidad nueva.
update stock_count_line_lots set qty = round(qty * sqlc.arg(factor)::numeric, 4)
 where ingredient_id = sqlc.arg(id)::bigint and round(qty * sqlc.arg(factor)::numeric, 4) > 0;

-- name: ExpenseItemExpiry :one
select expires_on from expense_items where id = sqlc.arg(id) and expense_id = sqlc.arg(expense_id);

-- name: SetExpenseItemExpiry :exec
update expense_items set expires_on = sqlc.narg(expires_on)::date where id = sqlc.arg(id) and expense_id = sqlc.arg(expense_id);

-- name: RelocateLineLot :exec
-- Lo ya sumado de un renglón pasa de su fecha vieja a la nueva, en cada almacén (relocate_lot, 0128).
select relocate_lot(nullif(current_setting('app.company_id', true), '')::bigint, a.warehouse_id, sqlc.arg(ingredient_id)::bigint,
                    sqlc.narg(from_date)::date, sqlc.narg(to_date)::date, a.qty)
  from (select warehouse_id, sum(quantity) as qty from stock_movements
         where expense_item_id = sqlc.arg(item_id)::bigint group by warehouse_id having sum(quantity) > 0) a;

-- name: BusinessToday :one
select business_today(nullif(current_setting('app.company_id', true), '')::bigint)::date as today;

-- name: BranchLots :many
-- Los lotes con algo de la sucursal (todos sus almacenes: la proyección los junta), con lo que mermas
-- por aprobar ya piden de cada uno. Los artículos archivados no se ofrecen (spec 077).
select l.id, l.ingredient_id, i.name, u.code as unit_code, l.warehouse_id, w.name as warehouse_name,
       l.expires_on, l.qty,
       coalesce((select sum(wr.quantity) from waste_records wr
                  where wr.status = 'pending' and wr.warehouse_id = l.warehouse_id
                    and wr.ingredient_id = l.ingredient_id and wr.lot_expires_on = l.expires_on), 0)::numeric as pending_waste
  from stock_lots l
  join ingredients i on i.id = l.ingredient_id
  join units u on u.id = i.base_unit_id
  join warehouses w on w.id = l.warehouse_id
 where l.branch_id = current_branch_id() and l.qty > 0 and i.is_active
 order by l.expires_on, i.name, w.name;

-- name: BranchUndated :many
-- Lo sin fecha de cada artículo con lotes, en toda la sucursal: existencia menos lotes.
select s.ingredient_id::bigint as ingredient_id,
       (sum(s.on_hand) - coalesce((select sum(l.qty) from stock_lots l
                                   where l.branch_id = current_branch_id() and l.ingredient_id = s.ingredient_id), 0))::numeric as undated
  from stock_levels s
 where s.branch_id = current_branch_id() and s.ingredient_id = any(sqlc.arg(ids)::bigint[])
 group by s.ingredient_id;

-- name: BranchDailyUse :many
-- Consumo promedio diario de 30 días: ventas netas de cancelaciones de la sucursal. Un traspaso no es
-- consumo (como la lista de compra).
select ingredient_id::bigint as ingredient_id, (-sum(quantity) / 30)::numeric as daily
  from stock_movements
 where branch_id = current_branch_id() and ingredient_id = any(sqlc.arg(ids)::bigint[])
   and movement_type in ('venta', 'cancelacion') and created_at >= now() - interval '30 days'
 group by ingredient_id;

-- name: CountLotsOf :many
-- El desglose de fechas de un conteo, por artículo.
select ingredient_id, expires_on, qty from stock_count_line_lots where count_id = sqlc.arg(count_id) order by ingredient_id, expires_on;

-- name: KnownLotDates :many
-- Las fechas que el sistema ya conoce de cada artículo en el almacén (lotes con algo): el conteo las
-- ofrece sin cantidad, para no reteclearlas ni enseñar lo esperado.
select ingredient_id, expires_on from stock_lots where warehouse_id = sqlc.arg(warehouse_id) and qty > 0 order by ingredient_id, expires_on;

-- name: CountLotsSum :one
select coalesce(sum(qty), 0)::numeric from stock_count_line_lots where count_id = sqlc.arg(count_id) and ingredient_id = sqlc.arg(ingredient_id);

-- name: DeleteCountLots :exec
delete from stock_count_line_lots where count_id = sqlc.arg(count_id) and ingredient_id = sqlc.arg(ingredient_id);

-- name: InsertCountLot :exec
insert into stock_count_line_lots (count_id, ingredient_id, expires_on, qty)
values (sqlc.arg(count_id), sqlc.arg(ingredient_id), sqlc.arg(expires_on), sqlc.arg(qty));

-- name: CountLotIngredients :many
select distinct ingredient_id from stock_count_line_lots where count_id = sqlc.arg(count_id) order by ingredient_id;

-- name: ApplyCountLots :exec
select apply_count_lots(nullif(current_setting('app.company_id', true), '')::bigint, sqlc.arg(count_id)::bigint, sqlc.arg(ingredient_id)::bigint);
