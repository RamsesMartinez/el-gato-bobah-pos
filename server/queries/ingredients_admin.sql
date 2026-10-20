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

-- name: GetArticle :one
-- La ficha. updated_at es la versión: editar exige la que se vio (STALE si cambió).
select i.id, i.name::text as name, i.is_active, i.base_unit_id, u.code as base_unit_code, u.kind as base_unit_kind,
       i.current_cost, i.cost_source::text as cost_source, i.min_stock, i.is_prep, i.category_id, ic.name as category,
       i.count_place_id, cp.name as count_place, i.updated_at,
       coalesce(sl.on_hand, 0)::numeric(14,4) as on_hand
  from ingredients i
  join units u on u.id = i.base_unit_id
  left join count_places cp on cp.id = i.count_place_id
  left join ingredient_categories ic on ic.id = i.category_id
  left join stock_levels sl on sl.ingredient_id = i.id and sl.branch_id = current_branch_id()
 where i.id = sqlc.arg(id);

-- name: FindOtherArticleByName :one
-- Como FindArticleByName, sin contarse a sí mismo: renombrar a su propio nombre no es repetirlo.
select id, is_active from ingredients
 where id <> sqlc.arg(id)
   and regexp_replace(translate(lower(trim(name)), 'áéíóúüñ', 'aeiouun'), '\s+', ' ', 'g')
     = regexp_replace(translate(lower(trim(sqlc.arg(name)::text)), 'áéíóúüñ', 'aeiouun'), '\s+', ' ', 'g')
 order by is_active desc, id
 limit 1;

-- name: UpdateArticle :execrows
-- Solo si nadie la cambió desde que se vio: 0 filas es «cambió mientras la veías».
update ingredients
   set name = sqlc.arg(name), category_id = sqlc.narg(category_id), count_place_id = sqlc.narg(place_id),
       min_stock = sqlc.narg(min_stock), updated_at = now()
 where id = sqlc.arg(id) and updated_at = sqlc.arg(expected_updated_at);

-- name: ArticleCostHistory :many
-- El historial de costos: cada compra recibida (solo lo que entró con costo: ni las diferencias de
-- corregir lo recibido ni los regalos son un precio) y cada corrección a mano. La más reciente arriba,
-- por llave (fecha, tipo, id) para que «Ver más» no repita ni salte renglones.
select kind, at, id, unit_cost, presentation, pack_qty_in_base, supplier, user_name from (
  -- Las correcciones van primero en el union a propósito: sqlc toma los tipos de la primera rama, y
  -- con null::text ahí sabe que proveedor y empaque pueden faltar.
  select 'correction'::text as kind, c.created_at as at, c.id, c.new_cost as unit_cost,
         null::text as presentation, null::numeric(14,4) as pack_qty_in_base, null::text as supplier, u.name as user_name
    from ingredient_cost_corrections c
    left join users u on u.id = c.user_id
   where c.ingredient_id = sqlc.arg(ingredient_id)
  union all
  select 'purchase'::text, m.created_at, m.id, m.unit_cost::numeric(12,6),
         p.name, ei.pack_qty_in_base, (select s.name::text from suppliers s where s.id = e.supplier_id), null::text
    from stock_movements m
    join expense_items ei on ei.id = m.expense_item_id
    join expenses e on e.id = ei.expense_id
    left join ingredient_presentations p on p.id = ei.presentation_id
   where m.ingredient_id = sqlc.arg(ingredient_id) and m.movement_type = 'compra'
     and m.quantity > 0 and m.unit_cost is not null and not ei.is_gift
) h
 where sqlc.narg(before_at)::timestamptz is null
    or (h.at, h.kind, h.id) < (sqlc.narg(before_at)::timestamptz, sqlc.narg(before_kind)::text, sqlc.narg(before_id)::bigint)
 order by h.at desc, h.kind desc, h.id desc
 limit sqlc.arg(lim);

-- name: OldestOfLastPurchases :one
-- La compra más vieja de las últimas 20 y cuántas hay: la referencia del indicador de tendencia.
select count(*)::int as purchases,
       coalesce((array_agg(unit_cost order by created_at asc, id asc))[1], 0)::numeric(12,6) as oldest_cost
  from (select m.id, m.created_at, m.unit_cost
          from stock_movements m join expense_items ei on ei.id = m.expense_item_id
         where m.ingredient_id = sqlc.arg(ingredient_id) and m.movement_type = 'compra'
           and m.quantity > 0 and m.unit_cost is not null and not ei.is_gift
         order by m.created_at desc, m.id desc
         limit 20) last;

-- name: ArticleAudit :many
-- La bitácora del artículo y de sus empaques, sin los detalles de un cambio de unidad (son para
-- revertirlo, no para leerlos).
select a.id, a.occurred_at, a.action, a.entity_type, a.before, a.after, coalesce(u.name::text, a.actor_label) as actor
  from audit_events a
  left join users u on u.id = a.actor_user_id
 where ((a.entity_type = 'ingredient' and a.entity_id = sqlc.arg(ingredient_id))
        or (a.entity_type = 'presentation' and a.entity_id in (select p.id from ingredient_presentations p where p.ingredient_id = sqlc.arg(ingredient_id))))
   and a.action <> 'ingredient.unit_changed.detail'
   and (sqlc.narg(before_at)::timestamptz is null or (a.occurred_at, a.id) < (sqlc.narg(before_at)::timestamptz, sqlc.narg(before_id)::bigint))
 order by a.occurred_at desc, a.id desc
 limit sqlc.arg(lim);

-- name: ArticleOnHandByBranch :many
-- Lo que queda del artículo en cada sucursal, para la revisión de archivar.
select b.name::text as branch, sl.on_hand
  from stock_levels sl join branches b on b.id = sl.branch_id
 where sl.item_type = 'ingrediente' and sl.ingredient_id = sqlc.arg(ingredient_id) and sl.on_hand <> 0
 order by b.name;

-- name: ActiveRecipesUsing :many
-- Las recetas activas que restan el artículo: de un producto, de un extra o de otro preparado.
select 'product'::text as kind, p.id, p.name::text as name
  from recipe_items ri join products p on p.recipe_id = ri.recipe_id
 where ri.ingredient_id = sqlc.arg(ingredient_id) and p.is_active
union all
select 'option'::text, mo.id, mo.name::text
  from recipe_items ri join modifier_options mo on mo.recipe_id = ri.recipe_id
 where ri.ingredient_id = sqlc.arg(ingredient_id) and mo.is_active
union all
select 'ingredient'::text, i.id, i.name::text
  from recipe_items ri join ingredients i on i.recipe_id = ri.recipe_id
 where ri.ingredient_id = sqlc.arg(ingredient_id) and i.is_active
 order by 3;

-- name: OpenCountHasArticle :one
-- ¿Algún conteo abierto ya capturó el artículo? Archivarlo dejaría esa cifra sin artículo que ofrecer.
select exists (
  select 1 from stock_count_lines l join stock_counts c on c.id = l.count_id
   where c.status = 'open' and l.ingredient_id = sqlc.arg(ingredient_id)
)::bool;

-- name: PendingReceiptsOf :one
-- Cuántos renglones de compra del artículo faltan por llegar (la misma regla que «Por recibir»).
select count(*)::int from expense_items ei join expenses e on e.id = ei.expense_id
 where ei.ingredient_id = sqlc.arg(ingredient_id) and e.status <> 'cancelada' and ei.voided_at is null
   and ei.quantity - coalesce(ei.qty_received, 0) - ei.not_coming_qty > 0;

-- name: LockArticle :one
-- El artículo bloqueado para archivarlo, reactivarlo o cambiarle la unidad.
select id, is_active from ingredients where id = sqlc.arg(id) for update;

-- name: SetArticleActive :exec
update ingredients set is_active = sqlc.arg(active), updated_at = now() where id = sqlc.arg(id);
