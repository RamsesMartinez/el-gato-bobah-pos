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
