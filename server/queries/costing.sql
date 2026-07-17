-- Cargas para el motor de costeo (recompute batch).

-- name: ListIngredientsForCosting :many
select id, is_prep, recipe_id, yield_qty, waste_pct, current_cost, cost_source
from ingredients;

-- name: ListRecipeItemsForCosting :many
select ri.recipe_id, ri.ingredient_id, (ri.quantity * u.to_base)::numeric(20,6) as qty_base
from recipe_items ri
join units u on u.id = ri.unit_id;

-- name: ListProductsForCosting :many
select id, type, cost_source, manual_cost, recipe_id from products;

-- name: ListComboSlotDefaultsForCosting :many
select cs.combo_id, csp.product_id, cs.min_select
from combo_slots cs
join combo_slot_products csp on csp.slot_id = cs.id
where csp.is_default;

-- name: ListModifierOptionsForCosting :many
select id, recipe_id, linked_product_id from modifier_options;

-- Escrituras de costo (una por fila; el motor las agrupa en una tx).

-- name: UpdateIngredientCost :exec
update ingredients set current_cost = $2, updated_at = now() where id = $1;

-- name: UpdateProductCost :exec
update products set current_cost = $2, updated_at = now() where id = $1;

-- name: UpdateOptionCost :exec
update modifier_options set current_cost = $2 where id = $1;
