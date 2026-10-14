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

-- Cambio de modo de consumo (spec 037, research R6) -----------------------------------------------

-- name: LockProductStockLevels :many
-- Las existencias del producto, una por sucursal, BLOQUEADAS: una venta simultánea espera a que la
-- conversión termine en vez de aplicar su delta sobre una cifra que ya se trasladó. El trigger de
-- existencias suma deltas y no relee nada, así que sin este bloqueo se perdería o se inventaría.
select branch_id, on_hand from stock_levels
 where item_type = 'producto' and product_id = $1
 order by branch_id
   for update;

-- name: LockIngredientStockLevels :many
select branch_id, on_hand from stock_levels
 where item_type = 'ingrediente' and ingredient_id = $1
 order by branch_id
   for update;

-- name: InsertModeAdjustment :exec
-- Un ajuste del cambio de modo. Va con su sucursal explícita (la de la fila bloqueada), la nota que
-- liga los dos movimientos de un traslado y quién lo hizo.
insert into stock_movements (item_type, ingredient_id, product_id, movement_type, quantity, branch_id, user_id, reason, note)
values (sqlc.arg(item_type)::stock_item_type, sqlc.narg(ingredient_id), sqlc.narg(product_id), 'ajuste',
        sqlc.arg(quantity), sqlc.arg(branch_id), sqlc.arg(user_id), 'cambio de modo', sqlc.arg(note));

-- name: TurnOffProductTrackStock :exec
-- Antes de ponerle receta: el check de products no deja existencia propia y receta a la vez.
update products set track_stock = false, updated_at = now() where id = $1;

-- name: GetUnitToIngredientBase :one
-- Cuánto de la unidad base del insumo es una unidad capturada: «1 caja» de un insumo que se lleva
-- por pieza. La captura ya exige que sean del mismo tipo.
select (u.to_base / bu.to_base)::numeric as factor
  from units u, ingredients i join units bu on bu.id = i.base_unit_id
 where u.id = sqlc.arg(unit_id) and i.id = sqlc.arg(ingredient_id);

-- name: GetProductOnHand :one
-- La existencia propia de un producto, sumada: es lo que la hoja de receta muestra al preguntar
-- «¿Cuántas tienes de verdad?». La cifra que manda manda la vuelve a comparar contra la bloqueada.
select coalesce(sum(on_hand), 0)::numeric(14,4) as on_hand,
       count(*) filter (where on_hand <> 0)::int as branches
  from stock_levels where item_type = 'producto' and product_id = $1;
