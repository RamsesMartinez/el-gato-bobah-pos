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

-- El costo de un insumo que se compra hecho, al recibir una compra.

-- name: UpdateIngredientCost :exec
update ingredients set current_cost = $2, updated_at = now() where id = $1;

-- El recálculo escribe en una sentencia por tabla y solo lo que cambió: una fila reescrita con el
-- mismo costo sube la versión del catálogo (0093) y obliga a las tabletas a volver a bajar el menú,
-- y en ingredients mueve updated_at, que es la versión con la que se edita la ficha del artículo.
-- Los costos viajan como texto: es el decimal exacto y no depende de cómo pgx codifique numeric[].

-- name: SetPrepIngredientCosts :exec
update ingredients i set current_cost = v.cost::numeric, updated_at = now()
  from (select unnest(sqlc.arg(ids)::bigint[]) as id, unnest(sqlc.arg(costs)::text[]) as cost) v
 where i.id = v.id and i.current_cost is distinct from v.cost::numeric;

-- name: SetProductCosts :exec
update products p set current_cost = v.cost::numeric, updated_at = now()
  from (select unnest(sqlc.arg(ids)::bigint[]) as id, unnest(sqlc.arg(costs)::text[]) as cost) v
 where p.id = v.id and p.current_cost is distinct from v.cost::numeric;

-- name: SetOptionCosts :exec
update modifier_options o set current_cost = v.cost::numeric
  from (select unnest(sqlc.arg(ids)::bigint[]) as id, unnest(sqlc.arg(costs)::text[]) as cost) v
 where o.id = v.id and o.current_cost is distinct from v.cost::numeric;
