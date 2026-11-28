-- Cargas para el motor de costeo (recompute batch).

-- name: ListIngredientsForCosting :many
select id, is_prep, recipe_id, yield_qty, expected_waste_pct, current_cost, cost_source, is_packaging
from ingredients;

-- name: ListRecipeItemsForCosting :many
select ri.recipe_id, ri.ingredient_id, (ri.quantity * u.to_base)::numeric(20,6) as qty_base
from recipe_items ri
join units u on u.id = ri.unit_id;

-- name: ListProductsForCosting :many
select id, type, cost_source, manual_cost, recipe_id, expected_waste_pct from products;

-- name: GetGeneralExpectedWaste :one
-- La merma general de la empresa (spec 096). Sin cast a propósito: con `::numeric` sqlc la tipa no
-- nula y el NULL de «sin merma general» —el estado con el que arranca toda empresa— tumbaría el
-- recálculo entero (AGENTS.md, agregados que se pueden vaciar). Sin fila = sin merma general.
select bs.expected_waste_pct from business_settings bs limit 1;

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

-- Los tres % de merma esperada (spec 096). Cada uno devuelve el valor anterior para la bitácora; sin
-- fila = no existe o es de otra empresa (RLS), y sale como 404.

-- name: SetGeneralExpectedWaste :one
-- Sin where a propósito: business_settings tiene una fila por empresa y RLS la acota a la de la
-- sesión (sqlc no conoce company_id, AGENTS.md §1). Solo corre como gatobobah_app; como owner tocaría
-- a todas las empresas.
update business_settings bs set expected_waste_pct = sqlc.narg(pct), updated_at = now(), updated_by = sqlc.arg(updated_by)
  from (select expected_waste_pct from business_settings limit 1) old
returning old.expected_waste_pct as previous;

-- name: SetProductExpectedWaste :one
update products p set expected_waste_pct = sqlc.narg(pct), updated_at = now()
  from (select expected_waste_pct from products where id = sqlc.arg(id)) old
 where p.id = sqlc.arg(id)
returning old.expected_waste_pct as previous;

-- name: SetIngredientExpectedWaste :one
update ingredients i set expected_waste_pct = sqlc.narg(pct), updated_at = now()
  from (select expected_waste_pct from ingredients where id = sqlc.arg(id)) old
 where i.id = sqlc.arg(id)
returning old.expected_waste_pct as previous;

-- name: IngredientWasteInfo :one
-- La ficha que encabeza la vista previa de merma del MCP (spec 099). Bajo RLS: un insumo de otra
-- empresa no vuelve.
select i.id, i.name::text as name, i.is_active, i.expected_waste_pct, u.code::text as base_unit
  from ingredients i
  join units u on u.id = i.base_unit_id
 where i.id = @id;

-- name: CostingNames :many
-- Los nombres de lo que la vista previa dice que cambia de costo.
select 'product'::text as kind, id, name::text as name from products where id = any(@product_ids::bigint[])
union all
select 'option'::text, id, name::text from modifier_options where id = any(@option_ids::bigint[])
union all
select 'prep'::text, id, name::text from ingredients where id = any(@prep_ids::bigint[]);
