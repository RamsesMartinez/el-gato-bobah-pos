-- La existencia con la que se vende (spec 092). Hoy es la de la sucursal; con varios almacenes por
-- sucursal (#65) será la del almacén de venta, y solo cambian estas dos consultas y
-- app/sales_stock.go, que es lo único que las llama.

-- name: SalesStockLevels :many
-- La sucursal es la misma con la que vende la caja principal (`current_branch_id()`).
select item_type, ingredient_id, product_id, on_hand
from stock_levels
where branch_id = current_branch_id();

-- name: LockSalesStock :many
-- Los renglones que una venta va a revisar, bloqueados en un orden fijo: dos ventas que tocan los
-- mismos renglones los toman en el mismo orden y no se abrazan. El on_hand que devuelve es el de
-- después de esperar el candado.
select item_type, ingredient_id, product_id, on_hand
from stock_levels
where branch_id = sqlc.arg(branch_id)
  and ((item_type = 'ingrediente' and ingredient_id = any(sqlc.arg(ingredient_ids)::bigint[]))
    or (item_type = 'producto' and product_id = any(sqlc.arg(product_ids)::bigint[])))
order by item_type, product_id, ingredient_id
for update;
