-- Depleción en venta

-- name: GetRecipeDepletion :many
-- Ingredientes a descontar por producto (cantidad en unidad base, sin merma).
select p.id as product_id, ri.ingredient_id, (ri.quantity * u.to_base)::numeric(20,6) as qty_base
from products p
join recipe_items ri on ri.recipe_id = p.recipe_id
join units u on u.id = ri.unit_id
where p.id = any($1::bigint[]);

-- name: GetTrackStockProductIDs :many
select id from products where id = any($1::bigint[]) and track_stock;

-- name: InsertStockMovement :exec
-- order_line_id: de QUÉ renglón salió este descuento.
--
-- Sin él, reponer un renglón cancelado obliga a recalcular su consumo con la receta de HOY, y una
-- receta que cambió entre la venta y la cancelación repondría una cantidad distinta de la que salió.
-- NULL en los movimientos que no vienen de una venta (ajustes, compras, mermas).
insert into stock_movements (item_type, ingredient_id, product_id, movement_type, quantity, unit_cost, order_id, order_line_id, user_id, reason, note)
values ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11);

-- Almacén / niveles

-- name: ListStockLevels :many
select sl.item_type,
       coalesce(i.name, p.name) as item_name,
       sl.on_hand,
       coalesce(i.min_stock, p.min_stock) as min_stock,
       coalesce(iu.code, 'pieza') as unit_code
from stock_levels sl
left join ingredients i on i.id = sl.ingredient_id
left join units iu on iu.id = i.base_unit_id
left join products p on p.id = sl.product_id
order by item_name;

-- name: ListStockMovements :many
select sm.id, sm.item_type, coalesce(i.name, p.name) as item_name, sm.movement_type,
       sm.quantity, sm.reason, sm.created_at
from stock_movements sm
left join ingredients i on i.id = sm.ingredient_id
left join products p on p.id = sm.product_id
order by sm.created_at desc
limit $1;
