-- Unir insumos (spec 056). Los candados, el traspaso y lo que se mueve al destino.

-- name: MergeTargetOf :one
-- A qué insumo se unió éste, si es origen de una unión.
select r.new_ingredient_id as id, i.name::text as name, r.created_at
  from ingredient_replacements r join ingredients i on i.id = r.new_ingredient_id
 where r.old_ingredient_id = sqlc.arg(id) and r.kind = 'merge';

-- name: CountMergedSources :one
select count(*)::int from ingredient_replacements where new_ingredient_id = sqlc.arg(id) and kind = 'merge';

-- name: InsertMerge :one
insert into ingredient_replacements (old_ingredient_id, new_ingredient_id, factor, user_id, kind)
values (sqlc.arg(old_ingredient_id), sqlc.arg(new_ingredient_id), sqlc.arg(factor), sqlc.arg(user_id), 'merge')
returning id;

-- name: InsertMergeMove :exec
-- Un lado del traspaso, con su sucursal explícita y ligado a la unión (el trigger de la 0104 no
-- redirige un movimiento con replacement_id).
insert into stock_movements (item_type, ingredient_id, movement_type, quantity, unit_cost, branch_id, user_id, reason, replacement_id)
values ('ingrediente', sqlc.arg(ingredient_id), 'ajuste', sqlc.arg(quantity), sqlc.narg(unit_cost), sqlc.arg(branch_id),
        sqlc.arg(user_id), 'unión de insumos', sqlc.arg(replacement_id));

-- name: RecipeItemOfIngredient :one
-- El renglón del destino en una receta, si ya lo lleva: el del origen se suma a éste.
select ri.id, ri.quantity, ri.unit_id, u.to_base as unit_to_base, u.code as unit_code
  from recipe_items ri join units u on u.id = ri.unit_id
 where ri.recipe_id = sqlc.arg(recipe_id) and ri.ingredient_id = sqlc.arg(ingredient_id)
   for update of ri;

-- name: DeleteRecipeItem :exec
delete from recipe_items where id = sqlc.arg(id);

-- name: UnitCode :one
select code from units where id = sqlc.arg(id);

-- name: LastPurchaseCost :one
-- La compra más reciente con costo del insumo (sin regalos, renglones anulados o excluidos, ni gastos
-- cancelados): la misma regla que el historial de costos.
select m.created_at, m.unit_cost::numeric(12,6) as unit_cost
  from stock_movements m
  join expense_items ei on ei.id = m.expense_item_id
  join expenses e on e.id = ei.expense_id
 where m.ingredient_id = sqlc.arg(ingredient_id) and m.movement_type = 'compra'
   and m.quantity > 0 and m.unit_cost is not null and not ei.is_gift
   and ei.voided_at is null and not ei.excluded and e.status <> 'cancelada'
 order by m.created_at desc, m.id desc
 limit 1;

-- name: SetArticleMergedCost :exec
update ingredients
   set current_cost = sqlc.arg(current_cost), cost_source = sqlc.arg(cost_source)::cost_source,
       min_stock = sqlc.narg(min_stock), updated_at = now()
 where id = sqlc.arg(id);

-- name: MovePresentation :exec
-- La presentación pasa al destino conservando su id: las compras pasadas la siguen nombrando.
update ingredient_presentations
   set ingredient_id = sqlc.arg(ingredient_id), qty_in_base = sqlc.arg(qty_in_base), brand = sqlc.narg(brand)
 where id = sqlc.arg(id);

-- name: ArchivePresentation :exec
update ingredient_presentations set active = false where id = sqlc.arg(id);

-- name: BranchNames :many
select id, name::text as name from branches order by name;

-- name: MergedSourceNames :many
-- Los insumos que se unieron en éste, directos o por cadena, con su nombre: el historial dice de cuál
-- viene cada renglón.
with recursive src (ingredient_id, depth) as (
  select sqlc.arg(id)::bigint, 0
  union all
  select r.old_ingredient_id, src.depth + 1
    from ingredient_replacements r join src on r.new_ingredient_id = src.ingredient_id
   where r.kind = 'merge' and src.depth < 16
)
select i.id, i.name::text as name from src join ingredients i on i.id = src.ingredient_id where src.depth > 0;

-- name: MergeCandidates :many
-- Los insumos que se pueden proponer como duplicados: activos y no preparados, con su existencia (para
-- que quien revisa vea cuál tiene más). La de la empresa y no la de «la» sucursal: con dos sucursales
-- y sin selector, current_branch_id() truena y tumbaba la lista entera por un dato de apoyo.
select i.id, i.name::text as name, i.base_unit_id, u.code as base_unit_code, coalesce(sl.on_hand, 0)::numeric(14,4) as on_hand
  from ingredients i
  join units u on u.id = i.base_unit_id
  left join (select ingredient_id, sum(on_hand) as on_hand from stock_levels group by ingredient_id) sl on sl.ingredient_id = i.id
 where i.is_active and not i.is_prep
 order by i.name, i.id;

-- name: DistinctPairs :many
select low_ingredient_id, high_ingredient_id from ingredient_distinct_pairs;

-- name: InsertDistinctPair :exec
-- Marcar dos veces el mismo par no es un error: la pantalla lo puede mandar al cerrar y al deshacer.
insert into ingredient_distinct_pairs (low_ingredient_id, high_ingredient_id, user_id)
values (sqlc.arg(low_ingredient_id), sqlc.arg(high_ingredient_id), sqlc.arg(user_id))
on conflict (company_id, low_ingredient_id, high_ingredient_id) do nothing;

-- name: MergeSubtree :one
-- Lo que la unión hereda de un origen y de lo que ya se le había unido: qué tan larga es su cadena y
-- el costo por unidad más alto de su historia, en la unidad del origen. Con eso se sabe, antes de
-- escribir, si con la equivalencia el historial del destino seguiría cabiendo en numeric(12,6).
with recursive src (ingredient_id, factor, depth) as (
  select sqlc.arg(id)::bigint, 1::numeric, 0
  union all
  select r.old_ingredient_id, src.factor * r.factor, src.depth + 1
    from ingredient_replacements r join src on r.new_ingredient_id = src.ingredient_id
   where r.kind = 'merge' and src.depth < 16
)
select coalesce(max(src.depth), 0)::int as depth,
       coalesce(max(greatest(
         (select max(m.unit_cost) from stock_movements m where m.ingredient_id = src.ingredient_id and m.movement_type = 'compra'),
         (select max(c.new_cost) from ingredient_cost_corrections c where c.ingredient_id = src.ingredient_id),
         (select i.current_cost from ingredients i where i.id = src.ingredient_id)
       ) / src.factor), 0)::numeric as max_unit_cost
  from src;
