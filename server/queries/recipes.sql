-- Menú › Recetas (spec 028): productos, extras y preparados con el estado de su receta.
--
-- Cada lista tiene su gemela de conteo por estado con el MISMO `where` (AGENTS.md §1): si se
-- separan, los números de los filtros dejan de cuadrar con la lista. La empresa la pone RLS.
--
-- La búsqueda ignora mayúsculas y acentos («cafe» encuentra «Café») y busca también en los insumos
-- directos de la receta (no los de un preparado que lleve): «tapioca» trae las recetas que la llevan.
-- Sin índice: translate() no usa los trigram de 0030; con cientos de filas tarda milisegundos.
-- Las ventas son de los últimos 30 días por día de negocio, sin renglones ni pedidos cancelados,
-- agregadas una vez y no por renglón.

-- name: ListProductRecipes :many
with sales as (
  select ol.product_id, sum(ol.quantity) as qty
    from order_lines ol join orders o on o.id = ol.order_id
   where ol.cancelled_at is null and o.status not in ('cancelada', 'reembolsada')
     and o.business_date >= sqlc.arg(since)::date
   group by ol.product_id
), base as (
  select p.id, p.name::text as name, c.name::text as category, p.type::text as type, p.recipe_id, p.track_stock,
         case when p.track_stock or p.composition_status = 'confirmed' then 'done'
              when p.composition_status = 'estimated' then 'review' else 'pending' end as status,
         coalesce(s.qty, 0)::numeric(14,2) as sold
    from products p
    join categories c on c.id = p.category_id
    left join sales s on s.product_id = p.id
   where p.is_active and p.system_kind is null
     and (sqlc.arg(category_id)::bigint = 0 or p.category_id = sqlc.arg(category_id) or c.parent_id = sqlc.arg(category_id))
     and (sqlc.arg(q)::text = ''
          or translate(lower(p.name), 'áéíóúüñ', 'aeiouun') like '%' || translate(lower(sqlc.arg(q)), 'áéíóúüñ', 'aeiouun') || '%'
          or exists (select 1 from recipe_items ri join ingredients i on i.id = ri.ingredient_id
                      where ri.recipe_id = p.recipe_id
                        and translate(lower(i.name), 'áéíóúüñ', 'aeiouun') like '%' || translate(lower(sqlc.arg(q)), 'áéíóúüñ', 'aeiouun') || '%'))
)
select b.id, b.name, b.category, b.type, b.track_stock, b.status, b.sold,
       coalesce((select string_agg(x.txt, ' · ' order by x.pos, x.id) from (
          -- Lo que no llega a un kilo o un litro se lee en gramos o mililitros, como en la hoja.
          select ri.position as pos, ri.id, case when u.code in ('kg', 'l') and ri.quantity < 1
                 then trim_scale(ri.quantity * 1000)::text || ' ' || case u.code when 'kg' then 'g' else 'ml' end
                 else trim_scale(ri.quantity)::text || ' ' || u.code end || ' ' || i.name as txt
            from recipe_items ri join units u on u.id = ri.unit_id join ingredients i on i.id = ri.ingredient_id
           where ri.recipe_id = b.recipe_id order by ri.position, ri.id limit 3) x), '')::text as summary,
       (select count(*) from recipe_items ri where ri.recipe_id = b.recipe_id)::int as lines,
       coalesce((select string_agg(greatest(cs.min_select, 1) || ' ' || cp.name, ' + ' order by cs.position, cs.id)
          from combo_slots cs
          join combo_slot_products csp on csp.slot_id = cs.id and csp.is_default
          join products cp on cp.id = csp.product_id
         where cs.combo_id = b.id), '')::text as combo_summary,
       count(*) over() as total
  from base b
 where b.status = sqlc.arg(status)::text
 order by case when sqlc.arg(sort)::text = 'sales' then b.sold end desc nulls last, b.name, b.id
 limit sqlc.arg(lim)::int offset sqlc.arg(off)::int;

-- name: CountProductRecipes :many
select x.status, count(*)::int as n from (
  select case when p.track_stock or p.composition_status = 'confirmed' then 'done'
              when p.composition_status = 'estimated' then 'review' else 'pending' end as status
    from products p
    join categories c on c.id = p.category_id
   where p.is_active and p.system_kind is null
     and (sqlc.arg(category_id)::bigint = 0 or p.category_id = sqlc.arg(category_id) or c.parent_id = sqlc.arg(category_id))
     and (sqlc.arg(q)::text = ''
          or translate(lower(p.name), 'áéíóúüñ', 'aeiouun') like '%' || translate(lower(sqlc.arg(q)), 'áéíóúüñ', 'aeiouun') || '%'
          or exists (select 1 from recipe_items ri join ingredients i on i.id = ri.ingredient_id
                      where ri.recipe_id = p.recipe_id
                        and translate(lower(i.name), 'áéíóúüñ', 'aeiouun') like '%' || translate(lower(sqlc.arg(q)), 'áéíóúüñ', 'aeiouun') || '%'))
) x group by x.status;

-- name: ListExtraRecipes :many
with sales as (
  select olm.modifier_option_id, sum(olm.quantity * ol.quantity) as qty
    from order_line_modifiers olm
    join order_lines ol on ol.id = olm.order_line_id
    join orders o on o.id = ol.order_id
   where ol.cancelled_at is null and o.status not in ('cancelada', 'reembolsada')
     and o.business_date >= sqlc.arg(since)::date
   group by olm.modifier_option_id
), base as (
  select mo.id, mo.name::text as name, mg.name::text as category, mo.recipe_id, mo.linked_product_id,
         case when mo.composition_status = 'confirmed' then 'done'
              when mo.composition_status = 'estimated' then 'review' else 'pending' end as status,
         coalesce(s.qty, 0)::numeric(14,2) as sold
    from modifier_options mo
    join modifier_groups mg on mg.id = mo.group_id
    left join sales s on s.modifier_option_id = mo.id
   where mo.is_active and mg.is_active
     and (sqlc.arg(category_id)::bigint = 0 or mo.group_id = sqlc.arg(category_id))
     and (sqlc.arg(q)::text = ''
          or translate(lower(mo.name || ' ' || mg.name), 'áéíóúüñ', 'aeiouun') like '%' || translate(lower(sqlc.arg(q)), 'áéíóúüñ', 'aeiouun') || '%'
          or exists (select 1 from recipe_items ri join ingredients i on i.id = ri.ingredient_id
                      where ri.recipe_id = mo.recipe_id
                        and translate(lower(i.name), 'áéíóúüñ', 'aeiouun') like '%' || translate(lower(sqlc.arg(q)), 'áéíóúüñ', 'aeiouun') || '%'))
)
select b.id, b.name, b.category, b.status, b.sold,
       coalesce((select string_agg(x.txt, ' · ' order by x.pos, x.id) from (
          select ri.position as pos, ri.id, case when u.code in ('kg', 'l') and ri.quantity < 1
                 then trim_scale(ri.quantity * 1000)::text || ' ' || case u.code when 'kg' then 'g' else 'ml' end
                 else trim_scale(ri.quantity)::text || ' ' || u.code end || ' ' || i.name as txt
            from recipe_items ri join units u on u.id = ri.unit_id join ingredients i on i.id = ri.ingredient_id
           where ri.recipe_id = b.recipe_id order by ri.position, ri.id limit 3) x), '')::text as summary,
       (select count(*) from recipe_items ri where ri.recipe_id = b.recipe_id)::int as lines,
       coalesce((select lp.name::text from products lp where lp.id = b.linked_product_id), '')::text as linked_name,
       count(*) over() as total
  from base b
 where b.status = sqlc.arg(status)::text
 order by case when sqlc.arg(sort)::text = 'sales' then b.sold end desc nulls last, b.name, b.category, b.id
 limit sqlc.arg(lim)::int offset sqlc.arg(off)::int;

-- name: CountExtraRecipes :many
select x.status, count(*)::int as n from (
  select case when mo.composition_status = 'confirmed' then 'done'
              when mo.composition_status = 'estimated' then 'review' else 'pending' end as status
    from modifier_options mo
    join modifier_groups mg on mg.id = mo.group_id
   where mo.is_active and mg.is_active
     and (sqlc.arg(category_id)::bigint = 0 or mo.group_id = sqlc.arg(category_id))
     and (sqlc.arg(q)::text = ''
          or translate(lower(mo.name || ' ' || mg.name), 'áéíóúüñ', 'aeiouun') like '%' || translate(lower(sqlc.arg(q)), 'áéíóúüñ', 'aeiouun') || '%'
          or exists (select 1 from recipe_items ri join ingredients i on i.id = ri.ingredient_id
                      where ri.recipe_id = mo.recipe_id
                        and translate(lower(i.name), 'áéíóúüñ', 'aeiouun') like '%' || translate(lower(sqlc.arg(q)), 'áéíóúüñ', 'aeiouun') || '%'))
) x group by x.status;

-- name: ListPrepRecipes :many
-- Preparados: los insumos que se hacen en el local. No tienen ventas propias; van por nombre.
with base as (
  select i.id, i.name::text as name, i.recipe_id,
         case when i.composition_status = 'estimated' then 'review' else 'done' end as status
    from ingredients i
   where i.is_prep and i.is_active
     and (sqlc.arg(q)::text = ''
          or translate(lower(i.name), 'áéíóúüñ', 'aeiouun') like '%' || translate(lower(sqlc.arg(q)), 'áéíóúüñ', 'aeiouun') || '%'
          or exists (select 1 from recipe_items ri join ingredients c on c.id = ri.ingredient_id
                      where ri.recipe_id = i.recipe_id
                        and translate(lower(c.name), 'áéíóúüñ', 'aeiouun') like '%' || translate(lower(sqlc.arg(q)), 'áéíóúüñ', 'aeiouun') || '%'))
)
select b.id, b.name, b.status,
       coalesce((select string_agg(x.txt, ' · ' order by x.pos, x.id) from (
          select ri.position as pos, ri.id, case when u.code in ('kg', 'l') and ri.quantity < 1
                 then trim_scale(ri.quantity * 1000)::text || ' ' || case u.code when 'kg' then 'g' else 'ml' end
                 else trim_scale(ri.quantity)::text || ' ' || u.code end || ' ' || c.name as txt
            from recipe_items ri join units u on u.id = ri.unit_id join ingredients c on c.id = ri.ingredient_id
           where ri.recipe_id = b.recipe_id order by ri.position, ri.id limit 3) x), '')::text as summary,
       (select count(*) from recipe_items ri where ri.recipe_id = b.recipe_id)::int as lines,
       count(*) over() as total
  from base b
 where b.status = sqlc.arg(status)::text
 order by b.name, b.id
 limit sqlc.arg(lim)::int offset sqlc.arg(off)::int;

-- name: CountPrepRecipes :many
select x.status, count(*)::int as n from (
  select case when i.composition_status = 'estimated' then 'review' else 'done' end as status
    from ingredients i
   where i.is_prep and i.is_active
     and (sqlc.arg(q)::text = ''
          or translate(lower(i.name), 'áéíóúüñ', 'aeiouun') like '%' || translate(lower(sqlc.arg(q)), 'áéíóúüñ', 'aeiouun') || '%'
          or exists (select 1 from recipe_items ri join ingredients c on c.id = ri.ingredient_id
                      where ri.recipe_id = i.recipe_id
                        and translate(lower(c.name), 'áéíóúüñ', 'aeiouun') like '%' || translate(lower(sqlc.arg(q)), 'áéíóúüñ', 'aeiouun') || '%'))
) x group by x.status;

-- name: ConfirmEstimatedProducts :execrows
-- «Confirmar las que se ven»: solo las estimadas de la lista que mandó la pantalla. Una pendiente
-- no se confirma así: confirmarla sin receta diría «no gasta insumos», y eso lo decide una persona.
update products set composition_status = 'confirmed', composition_confirmed_by = sqlc.arg(actor), composition_confirmed_at = now()
 where id = any(sqlc.arg(ids)::bigint[]) and composition_status = 'estimated';

-- name: ConfirmEstimatedOptions :execrows
update modifier_options set composition_status = 'confirmed', composition_confirmed_by = sqlc.arg(actor), composition_confirmed_at = now()
 where id = any(sqlc.arg(ids)::bigint[]) and composition_status = 'estimated';

-- name: ConfirmEstimatedIngredients :execrows
update ingredients set composition_status = 'confirmed', composition_confirmed_by = sqlc.arg(actor), composition_confirmed_at = now()
 where id = any(sqlc.arg(ids)::bigint[]) and composition_status = 'estimated';

-- name: ListSameNameOptions :many
-- Los extras que se llaman igual en otro grupo («Tapioca» en Toppings y en Toppings de frappé): la
-- pantalla ofrece guardarles la misma receta.
select o.id, g.name::text as group_name, o.composition_confirmed_at
  from modifier_options o
  join modifier_groups g on g.id = o.group_id
  join modifier_options me on me.id = sqlc.arg(id)
 where o.id <> me.id and o.is_active and g.is_active and lower(o.name) = lower(me.name)
 order by g.name, o.id;
