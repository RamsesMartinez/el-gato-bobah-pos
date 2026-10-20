-- Gestión de artículos del almacén (spec 042): la lista con buscador y filtros, la ficha, editar,
-- archivar, el historial de costos, la bitácora y el cambio de unidad.

-- name: ListArticles :many
-- La lista de Almacén › Artículos. Buscar no distingue mayúsculas ni acentos y mira también el nombre
-- de los empaques activos: quien busca «garrafa» quiere la leche. Los filtros se combinan con «y».
-- Archivados se ven solo cuando se piden: un archivado no es un artículo en uso.
select i.id, i.name::text as name, i.is_active, i.base_unit_id, u.code as base_unit_code, u.kind as base_unit_kind,
       i.current_cost, i.min_stock, i.is_prep, i.category_id, ic.name as category,
       i.count_place_id, cp.name as count_place,
       coalesce(sl.on_hand, 0)::numeric(14,4) as on_hand
  from ingredients i
  join units u on u.id = i.base_unit_id
  left join count_places cp on cp.id = i.count_place_id
  left join ingredient_categories ic on ic.id = i.category_id
  -- Existencias de la sucursal (0076): con dos sucursales, sin el filtro cada artículo saldría dos veces.
  left join stock_levels sl on sl.ingredient_id = i.id and sl.branch_id = current_branch_id()
 where i.is_active = not sqlc.arg(archived)::boolean
   and (sqlc.narg('q')::text is null
        or translate(lower(i.name::text), 'áéíóúüñ', 'aeiouun') like '%' || translate(lower(sqlc.narg('q')::text), 'áéíóúüñ', 'aeiouun') || '%'
        or exists (select 1 from ingredient_presentations p
                    where p.ingredient_id = i.id and p.active
                      and translate(lower(p.name), 'áéíóúüñ', 'aeiouun') like '%' || translate(lower(sqlc.narg('q')::text), 'áéíóúüñ', 'aeiouun') || '%'))
   and (sqlc.narg('place_id')::bigint is null or i.count_place_id = sqlc.narg('place_id'))
   and (not sqlc.arg(no_place)::boolean or i.count_place_id is null)
   and (not sqlc.arg(no_cost)::boolean or coalesce(i.current_cost, 0) = 0)
   and (not sqlc.arg(negative)::boolean or coalesce(sl.on_hand, 0) < 0)
 order by i.name, i.id;

-- name: FindArticleByName :one
-- El artículo de la empresa con ese nombre, sin distinguir mayúsculas, acentos ni espacios de más (la
-- normalización de IngredientNameTaken). Va después de LockIngredientNames.
select id, is_active from ingredients
 where regexp_replace(translate(lower(trim(name)), 'áéíóúüñ', 'aeiouun'), '\s+', ' ', 'g')
     = regexp_replace(translate(lower(trim(sqlc.arg(name)::text)), 'áéíóúüñ', 'aeiouun'), '\s+', ' ', 'g')
 order by is_active desc, id
 limit 1;

-- name: SetArticleCostManual :one
-- Corrección a mano: el costo nuevo cuenta desde ahora y una compra posterior lo vuelve a fijar.
-- Devuelve el costo anterior para la corrección y la bitácora.
update ingredients i set current_cost = sqlc.arg(cost), cost_source = 'manual', updated_at = now()
  from (select p.id as prev_id, p.current_cost as prev_cost from ingredients p where p.id = sqlc.arg(id) for update) prev
 where i.id = prev.prev_id
returning prev.prev_cost::numeric(12,6) as previous_cost;

-- name: InsertCostCorrection :exec
insert into ingredient_cost_corrections (ingredient_id, previous_cost, new_cost, user_id)
values (sqlc.arg(ingredient_id), sqlc.arg(previous_cost), sqlc.arg(new_cost), sqlc.arg(user_id));

-- name: InsertInitialStock :exec
-- La existencia con la que nace un artículo: un ajuste. La sucursal la pone el trigger con la de la
-- empresa; con dos y sin selector truena con EGB01 y no se crea nada.
insert into stock_movements (item_type, ingredient_id, movement_type, quantity, user_id, reason)
values ('ingrediente', sqlc.arg(ingredient_id), 'ajuste', sqlc.arg(quantity), sqlc.arg(user_id), 'existencia inicial');
