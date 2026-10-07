-- Depleción en venta

-- name: InsertStockMovement :exec
-- order_line_id: de QUÉ renglón salió este descuento.
--
-- Sin él, reponer un renglón cancelado obliga a recalcular su consumo con la receta de HOY, y una
-- receta que cambió entre la venta y la cancelación repondría una cantidad distinta de la que salió.
-- NULL en los movimientos que no vienen de una venta (ajustes, compras, mermas).
-- modifier_option_id / component_of_product_id: el extra o el paquete que originó el descuento.
insert into stock_movements (item_type, ingredient_id, product_id, movement_type, quantity, unit_cost, order_id, order_line_id, user_id, reason, note,
                             modifier_option_id, component_of_product_id)
values ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13);

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
-- Las de la sucursal (0076): con dos, el mismo insumo saldría dos veces sin decir de cuál es.
where sl.branch_id = current_branch_id()
order by item_name;

-- name: ListStockMovements :many
select sm.id, sm.item_type, coalesce(i.name, p.name) as item_name, sm.movement_type,
       sm.quantity, sm.reason, sm.created_at
from stock_movements sm
left join ingredients i on i.id = sm.ingredient_id
left join products p on p.id = sm.product_id
order by sm.created_at desc
limit $1;
