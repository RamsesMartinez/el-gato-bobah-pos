-- Gestión de artículos del almacén (spec 042): la lista con buscador y filtros, la ficha, editar,
-- archivar, el historial de costos, la bitácora y el cambio de unidad.

-- name: ListArticles :many
-- La lista de Almacén › Artículos. Buscar no distingue mayúsculas ni acentos y mira también el nombre
-- de los empaques activos: quien busca «garrafa» quiere la leche. Los filtros se combinan con «y».
-- Archivados se ven solo cuando se piden: un archivado no es un artículo en uso.
select i.id, i.name::text as name, i.is_active, i.base_unit_id, u.code as base_unit_code, u.kind as base_unit_kind,
       i.current_cost, i.min_stock, i.is_prep, i.category_id, ic.name as category,
       i.count_place_id, cp.name as count_place, i.composition_status,
       coalesce(sl.on_hand, 0)::numeric(14,4) as on_hand
  from ingredients i
  join units u on u.id = i.base_unit_id
  left join count_places cp on cp.id = i.count_place_id
  left join ingredient_categories ic on ic.id = i.category_id
  -- Existencias de la sucursal (0076): con dos sucursales, sin el filtro cada artículo saldría dos veces.
  left join stock_levels sl on sl.ingredient_id = i.id and sl.branch_id = current_branch_id()
 where i.is_active = not sqlc.arg(archived)::boolean
   and (sqlc.narg('q')::text is null
        -- strpos y no like: lo que alguien escriba es texto, nunca un comodín.
        or strpos(translate(lower(i.name::text), 'áéíóúüñ', 'aeiouun'), translate(lower(sqlc.narg('q')::text), 'áéíóúüñ', 'aeiouun')) > 0
        or exists (select 1 from ingredient_presentations p
                    where p.ingredient_id = i.id and p.active
                      and strpos(translate(lower(p.name), 'áéíóúüñ', 'aeiouun'), translate(lower(sqlc.narg('q')::text), 'áéíóúüñ', 'aeiouun')) > 0))
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
       i.count_place_id, cp.name as count_place, i.updated_at, i.composition_status,
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

-- name: UpdateArticle :one
-- Solo si nadie la cambió desde que se vio: sin fila es «cambió mientras la veías». Devuelve la versión
-- nueva para que el guardado siguiente de la misma ficha no choque contra este.
update ingredients
   set name = sqlc.arg(name), category_id = sqlc.narg(category_id), count_place_id = sqlc.narg(place_id),
       min_stock = sqlc.narg(min_stock), updated_at = now()
 where id = sqlc.arg(id) and updated_at = sqlc.arg(expected_updated_at)
returning updated_at;

-- name: ArticleCostHistory :many
-- El historial de costos: cada compra recibida (solo lo que entró con costo: ni las diferencias de
-- corregir lo recibido ni los regalos son un precio) y cada corrección a mano. La más reciente arriba,
-- por llave (fecha, tipo, id) para que «Ver más» no repita ni salte renglones.
-- Incluye los de los insumos que se unieron en éste (spec 056), con su nombre y el costo convertido a
-- la unidad de éste: `src` sigue las uniones hacia atrás, con el producto de las equivalencias. Va
-- dentro de la consulta y no en una función de la base para que RLS aplique.
with recursive src (ingredient_id, factor, depth) as (
  select sqlc.arg(ingredient_id)::bigint, 1::numeric, 0
  union all
  select r.old_ingredient_id, src.factor * r.factor, src.depth + 1
    from ingredient_replacements r join src on r.new_ingredient_id = src.ingredient_id
   where r.kind = 'merge' and src.depth < 16
)
select kind, at, id, unit_cost, presentation, pack_qty_in_base, supplier, user_name, brand, source_id from (
  -- Las correcciones van primero en el union a propósito: sqlc toma los tipos de la primera rama, y
  -- con null::text ahí sabe que proveedor y empaque pueden faltar.
  select 'correction'::text as kind, c.created_at as at, c.id, round(c.new_cost / src.factor, 6)::numeric(12,6) as unit_cost,
         null::text as presentation, null::numeric(14,4) as pack_qty_in_base, null::text as supplier, u.name as user_name,
         null::text as brand, c.ingredient_id as source_id
    from ingredient_cost_corrections c
    join src on src.ingredient_id = c.ingredient_id
    left join users u on u.id = c.user_id
  union all
  -- Un renglón por compra: la primera entrada de cada renglón (una compra recibida en dos entregas
  -- es una compra), sin renglones anulados ni gastos cancelados, que no son un precio.
  select * from (
    select distinct on (ei.id) 'purchase'::text as kind, m.created_at as at, m.id,
           round(m.unit_cost / src.factor, 6)::numeric(12,6) as unit_cost,
           p.name as presentation, round(ei.pack_qty_in_base * src.factor, 4)::numeric(14,4) as pack_qty_in_base,
           (select s.name::text from suppliers s where s.id = e.supplier_id) as supplier,
           null::text as user_name, ei.presentation_brand as brand, m.ingredient_id as source_id
      from stock_movements m
      join src on src.ingredient_id = m.ingredient_id
      join expense_items ei on ei.id = m.expense_item_id
      join expenses e on e.id = ei.expense_id
      left join ingredient_presentations p on p.id = ei.presentation_id
     where m.movement_type = 'compra'
       and m.quantity > 0 and m.unit_cost is not null and not ei.is_gift
       and ei.voided_at is null and not ei.excluded and e.status <> 'cancelada'
     order by ei.id, m.created_at, m.id
  ) purchases
) h
 where sqlc.narg(before_at)::timestamptz is null
    or (h.at, h.kind, h.id) < (sqlc.narg(before_at)::timestamptz, sqlc.narg(before_kind)::text, sqlc.narg(before_id)::bigint)
 order by h.at desc, h.kind desc, h.id desc
 limit sqlc.arg(lim);

-- name: OldestOfLastPurchases :one
-- La compra más vieja de las últimas 20 y cuántas hay: la referencia del indicador de tendencia.
select count(*)::int as purchases,
       coalesce((array_agg(unit_cost order by created_at asc, id asc))[1], 0)::numeric(12,6) as oldest_cost
  from (select * from (
          select distinct on (ei.id) m.id, m.created_at, m.unit_cost
            from stock_movements m
            join expense_items ei on ei.id = m.expense_item_id
            join expenses e on e.id = ei.expense_id
           where m.ingredient_id = sqlc.arg(ingredient_id) and m.movement_type = 'compra'
             and m.quantity > 0 and m.unit_cost is not null and not ei.is_gift
             and ei.voided_at is null and not ei.excluded and e.status <> 'cancelada'
           order by ei.id, m.created_at, m.id) firsts
         order by created_at desc, id desc
         limit 20) last;

-- name: ArticleAudit :many
-- La bitácora del artículo y de sus empaques, sin los detalles de un cambio de unidad (son para
-- revertirlo, no para leerlos). Junta al leer los guardados seguidos del autoguardado (spec 043, E13):
-- `ingredient.updated` del mismo usuario y del mismo y único campo, cada uno a menos de 60 s del
-- anterior, en una entrada con el antes del primero y el después del último. audit_events es solo de
-- agregar, así que no se puede juntar al escribir. Se agrupa antes de paginar para que «Ver más» no
-- parta una entrada; la entrada lleva la hora y el id de su último evento, que es la llave del cursor.
-- jsonb_typeof va primero y en un case: jsonb_object_keys truena con un after que no es objeto, y en un
-- AND Postgres no garantiza el orden.
-- ponytail: relee y agrupa toda la historia del artículo en cada página; con cientos de eventos por
-- artículo no pesa. Si llegara a miles, se agrupa en una tabla aparte al escribir.
-- Incluye la de los insumos que se unieron en éste (spec 056), por la misma cadena de uniones que el
-- historial de costos.
with recursive src (ingredient_id, depth) as (
  select sqlc.arg(ingredient_id)::bigint, 0
  union all
  select r.old_ingredient_id, src.depth + 1
    from ingredient_replacements r join src on r.new_ingredient_id = src.ingredient_id
   where r.kind = 'merge' and src.depth < 16
), ev as (
  select a.id, a.occurred_at, a.action, a.entity_type, a.actor_user_id, a.before, a.after,
         coalesce(u.name::text, a.actor_label) as actor,
         case when a.action = 'ingredient.updated' and a.actor_user_id is not null
                   and jsonb_typeof(a.after) = 'object'
              then case when (select count(*) from jsonb_object_keys(a.after)) = 1
                        then (select k from jsonb_object_keys(a.after) k) end
         end as field
    from audit_events a
    left join users u on u.id = a.actor_user_id
   where ((a.entity_type = 'ingredient' and a.entity_id in (select ingredient_id from src))
          or (a.entity_type = 'presentation' and a.entity_id in (select p.id from ingredient_presentations p where p.ingredient_id in (select ingredient_id from src))))
     and a.action <> 'ingredient.unit_changed.detail'
), marked as (
  select ev.*,
         case when field is not null and field = lag(field) over w and actor_user_id = lag(actor_user_id) over w
                   and occurred_at - lag(occurred_at) over w < interval '60 seconds'
              then 0 else 1 end as starts
    from ev window w as (order by occurred_at, id)
), grouped as (
  select marked.*, sum(starts) over (order by occurred_at, id) as grp from marked
), spans as (
  select grouped.*,
         first_value(before) over g as first_before,
         count(*) over (partition by grp) as changes,
         row_number() over (partition by grp order by occurred_at desc, id desc) as rn
    from grouped
  window g as (partition by grp order by occurred_at, id rows between unbounded preceding and unbounded following)
)
select id, occurred_at, action, entity_type, first_before::jsonb as before, after, changes, actor
  from spans
 where rn = 1
   and (sqlc.narg(before_at)::timestamptz is null or (occurred_at, id) < (sqlc.narg(before_at)::timestamptz, sqlc.narg(before_id)::bigint))
 order by occurred_at desc, id desc
 limit sqlc.arg(lim);

-- name: ArticleOnHandByBranch :many
-- Lo que queda del artículo en cada sucursal, para la revisión de archivar y el cambio de unidad.
select b.id as branch_id, b.name::text as branch, sl.on_hand
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
 where ei.ingredient_id = sqlc.arg(ingredient_id) and e.status <> 'cancelada' and ei.voided_at is null and not ei.excluded
   and ei.quantity - coalesce(ei.qty_received, 0) - ei.not_coming_qty > 0;

-- name: LockArticle :one
-- El artículo bloqueado para archivarlo, reactivarlo o cambiarle la unidad.
select id, is_active from ingredients where id = sqlc.arg(id) for update;

-- name: SetArticleActive :exec
update ingredients set is_active = sqlc.arg(active), updated_at = now() where id = sqlc.arg(id);

-- name: PresentationSuggestions :many
-- Empaques que la empresa ya usa en artículos que se miden igual (spec 042, FR-011): el nombre y lo
-- que trae, para que «Bolsa · 15 kg» no aparezca en un artículo en litros.
select distinct p.name, p.qty_in_base
  from ingredient_presentations p
  join ingredients i on i.id = p.ingredient_id
  join units u on u.id = i.base_unit_id
 where p.active and not p.is_approximate and u.kind = sqlc.arg(kind)::unit_kind
 order by p.name, p.qty_in_base
 limit 60;

-- name: GetPresentationForAudit :one
select name, qty_in_base, active from ingredient_presentations where id = $1;

-- ---- Cambio de unidad (spec 042, FR-009) ----

-- name: LockArticleNowait :one
-- El artículo sin esperar: una venta en curso lo tiene y el cambio se reintenta (E6).
select id, is_active, name::text as name, base_unit_id, min_stock, current_cost, cost_source::text as cost_source,
       yield_qty, is_prep, category_id, count_place_id, track_stock, is_packaging
  from ingredients where id = sqlc.arg(id) for update nowait;

-- name: LockArticleStockNowait :many
select branch_id, on_hand from stock_levels
 where item_type = 'ingrediente' and ingredient_id = sqlc.arg(ingredient_id)
 order by branch_id
   for update nowait;

-- name: ArticleMovementTypes :many
select distinct movement_type::text from stock_movements where ingredient_id = sqlc.arg(ingredient_id);

-- name: PendingWasteOf :one
select count(*)::int from waste_records where ingredient_id = sqlc.arg(ingredient_id) and status = 'pending';

-- name: OpenExpenseItemsOf :one
-- Renglones de compra que todavía hablan en la unidad vieja: los no anulados. Con uno, el cambio
-- directo no se hace (en el directo no hay compras recibidas, así que serían pendientes).
select count(*)::int from expense_items where ingredient_id = sqlc.arg(ingredient_id) and voided_at is null;

-- name: ArticleAdjustments :many
select id, quantity, unit_cost from stock_movements
 where ingredient_id = sqlc.arg(ingredient_id) and movement_type = 'ajuste' order by id;

-- name: SetMovementQtyCost :exec
update stock_movements set quantity = sqlc.arg(quantity), unit_cost = sqlc.narg(unit_cost) where id = sqlc.arg(id);

-- name: ArticleCountLines :many
select count_id, counted_qty, system_qty, unit_cost from stock_count_lines where ingredient_id = sqlc.arg(ingredient_id);

-- name: SetCountLineQty :exec
update stock_count_lines set counted_qty = sqlc.arg(counted_qty), system_qty = sqlc.arg(system_qty), unit_cost = sqlc.narg(unit_cost)
 where count_id = sqlc.arg(count_id) and ingredient_id = sqlc.arg(ingredient_id);

-- name: ArticleRecipeItems :many
-- Los renglones de receta que restan el artículo, con si su receta está activa (de un producto, un
-- extra o un preparado activos): el reemplazo solo pasa al nuevo las activas.
select ri.id, ri.recipe_id, ri.quantity, ri.unit_id, u.to_base as unit_to_base, u.code as unit_code,
       coalesce((select p.name::text from products p where p.recipe_id = ri.recipe_id limit 1),
                (select mo.name::text from modifier_options mo where mo.recipe_id = ri.recipe_id limit 1),
                (select i.name::text from ingredients i where i.recipe_id = ri.recipe_id limit 1), 'Receta')::text as owner,
       (exists (select 1 from products p where p.recipe_id = ri.recipe_id and p.is_active)
        or exists (select 1 from modifier_options mo where mo.recipe_id = ri.recipe_id and mo.is_active)
        or exists (select 1 from ingredients i where i.recipe_id = ri.recipe_id and i.is_active))::bool as active
  from recipe_items ri join units u on u.id = ri.unit_id
 where ri.ingredient_id = sqlc.arg(ingredient_id)
 order by ri.id;

-- name: SetRecipeItem :exec
update recipe_items set ingredient_id = sqlc.arg(ingredient_id), quantity = sqlc.arg(quantity), unit_id = sqlc.arg(unit_id)
 where id = sqlc.arg(id);

-- name: SetPresentationRaw :exec
update ingredient_presentations
   set name = sqlc.arg(name), qty_in_base = sqlc.arg(qty_in_base), purchase_unit_id = sqlc.narg(purchase_unit_id),
       is_approximate = sqlc.arg(is_approximate), active = sqlc.arg(active)
 where id = sqlc.arg(id);

-- name: ArticlePurchaseFormats :many
select id, qty_in_base from ingredient_purchase_formats where ingredient_id = sqlc.arg(ingredient_id);

-- name: SetPurchaseFormatQty :exec
update ingredient_purchase_formats set ingredient_id = sqlc.arg(ingredient_id), qty_in_base = sqlc.arg(qty_in_base) where id = sqlc.arg(id);

-- name: ArticleSupplierItems :many
select id, pack_qty_in_base from supplier_items where ingredient_id = sqlc.arg(ingredient_id);

-- name: SetSupplierItem :exec
update supplier_items set ingredient_id = sqlc.arg(ingredient_id), pack_qty_in_base = sqlc.narg(pack_qty_in_base) where id = sqlc.arg(id);

-- name: ArticleWasteRecords :many
select id, quantity, unit_cost from waste_records where ingredient_id = sqlc.arg(ingredient_id) and status <> 'pending';

-- name: SetWasteQtyCost :exec
update waste_records set quantity = sqlc.arg(quantity), unit_cost = sqlc.narg(unit_cost) where id = sqlc.arg(id);

-- name: ArticleCostCorrections :many
select id, previous_cost, new_cost from ingredient_cost_corrections where ingredient_id = sqlc.arg(ingredient_id);

-- name: SetCostCorrection :exec
update ingredient_cost_corrections set previous_cost = sqlc.arg(previous_cost), new_cost = sqlc.arg(new_cost) where id = sqlc.arg(id);

-- name: SetArticleUnit :exec
update ingredients
   set base_unit_id = sqlc.arg(base_unit_id), min_stock = sqlc.narg(min_stock), current_cost = sqlc.arg(current_cost),
       yield_qty = sqlc.narg(yield_qty), updated_at = now()
 where id = sqlc.arg(id);

-- name: RecomputeArticleStock :exec
-- Tras convertir, lo guardado de cada sucursal es la suma de sus movimientos: redondear cada ajuste por
-- separado deja un residuo que el trigger (solo de insert) no corrige (Revisión B2).
update stock_levels sl
   set on_hand = coalesce((select sum(m.quantity) from stock_movements m
                            where m.ingredient_id = sl.ingredient_id and m.branch_id = sl.branch_id), 0),
       updated_at = now()
 where sl.item_type = 'ingrediente' and sl.ingredient_id = sqlc.arg(ingredient_id);

-- name: RenameArticle :exec
update ingredients set name = sqlc.arg(name), updated_at = now() where id = sqlc.arg(id);

-- name: InsertReplacement :one
insert into ingredient_replacements (old_ingredient_id, new_ingredient_id, factor, user_id)
values (sqlc.arg(old_ingredient_id), sqlc.arg(new_ingredient_id), sqlc.arg(factor), sqlc.arg(user_id))
returning id;

-- name: InsertReplacementMove :exec
-- Un lado del traspaso de un reemplazo, con su sucursal explícita: el trigger la tomaría de la empresa.
insert into stock_movements (item_type, ingredient_id, movement_type, quantity, unit_cost, branch_id, user_id, reason, replacement_id)
values ('ingrediente', sqlc.arg(ingredient_id), 'ajuste', sqlc.arg(quantity), sqlc.narg(unit_cost), sqlc.arg(branch_id),
        sqlc.arg(user_id), 'reemplazo', sqlc.arg(replacement_id));

-- name: SetArticleExtras :exec
update ingredients set count_place_id = sqlc.narg(place_id), track_stock = sqlc.arg(track_stock), is_packaging = sqlc.arg(is_packaging)
 where id = sqlc.arg(id);

-- name: CategoryVisible :one
-- La categoría, si es de la empresa de la sesión. La llave de ingredients.category_id no lleva la
-- empresa (0003) y una llave foránea salta RLS: sin esta lectura, un artículo podía ligarse a la
-- categoría de otra empresa.
select id from ingredient_categories where id = sqlc.arg(id);

-- name: ArticleNameExists :one
-- ¿Ya hay un artículo con ese nombre exacto (sin distinguir mayúsculas)? Para elegir el nombre del
-- viejo en un reemplazo: el índice único (company_id, name) cuenta también los archivados.
select exists (select 1 from ingredients where name = sqlc.arg(name)::citext)::bool;
