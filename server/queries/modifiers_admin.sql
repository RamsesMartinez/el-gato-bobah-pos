-- Gestión centralizada de modificadores (admin): catálogo de grupos + opciones,
-- y el enlace por-producto (product_modifier_groups) donde vive min/max/obligatorio.

-- Grupos --------------------------------------------------------------------

-- name: AdminListGroups :many
-- Catálogo paginado de grupos con su default min/max, conteo de opciones activas, de productos
-- que lo usan y de cuántos lo sobrescriben. count(*) over() = total (paginador).
-- Ordenable por nombre / #opciones / #productos (@sort) en @dir (asc|desc); subquery para poder
-- ordenar por los conteos calculados.
select g.id, g.name, g.is_active, g.default_min_select, g.default_max_select,
       g.option_count, g.product_count, g.override_count, g.total
from (
  select mg.id, mg.name, mg.is_active, mg.default_min_select, mg.default_max_select,
         (select count(*) from modifier_options mo where mo.group_id = mg.id and mo.is_active)::int as option_count,
         (select count(*) from product_modifier_groups pmg where pmg.group_id = mg.id)::int as product_count,
         (select count(*) from product_modifier_groups pmg where pmg.group_id = mg.id and pmg.min_select is not null)::int as override_count,
         count(*) over() as total
  from modifier_groups mg
  where (@status::text = '' or (@status = 'act' and mg.is_active) or (@status = 'inact' and not mg.is_active))
    and (@search::text = ''
         or mg.name ilike '%' || @search || '%'
         or exists (select 1 from modifier_options mo
                    where mo.group_id = mg.id and mo.name ilike '%' || @search || '%'))
) g
order by
  case when @sort::text = 'options'  and @dir::text = 'desc' then g.option_count  end desc,
  case when @sort::text = 'options'  and @dir::text = 'asc'  then g.option_count  end asc,
  case when @sort::text = 'products' and @dir::text = 'desc' then g.product_count end desc,
  case when @sort::text = 'products' and @dir::text = 'asc'  then g.product_count end asc,
  case when @sort::text = 'name'     and @dir::text = 'desc' then g.name end desc,
  g.name asc
limit nullif(@lim::int, 0) offset @off;

-- name: AdminGroupCounts :one
select count(*) filter (where is_active)::int as active,
       count(*) filter (where not is_active)::int as inactive
from modifier_groups;

-- name: AdminCreateGroup :one
insert into modifier_groups (name, default_min_select, default_max_select)
values (@name, @default_min_select, @default_max_select) returning id;

-- name: AdminUpdateGroup :exec
update modifier_groups
set name = @name, is_active = @is_active,
    default_min_select = @default_min_select, default_max_select = @default_max_select
where id = @id;

-- Opciones ------------------------------------------------------------------

-- name: AdminGroupOptions :many
-- Opciones de un grupo (incluye inactivas, para gestionarlas desde el admin).
select mo.id, mo.group_id, mo.name, mo.price_delta, mo.max_per_line,
       mo.current_cost, mo.is_favorite, mo.is_active
from modifier_options mo
where mo.group_id = @group_id
order by mo.sort_key, mo.name;

-- name: AdminReorderOptions :exec
-- Reasigna sort_key según el orden de @ids (1-based × 1000, deja hueco para inserciones).
-- El filtro por group_id evita tocar opciones de otro grupo por error.
update modifier_options mo
set sort_key = t.ord * 1000
from unnest(@ids::bigint[]) with ordinality as t(id, ord)
where mo.id = t.id and mo.group_id = @group_id;

-- name: AdminCreateOption :one
insert into modifier_options (group_id, name, price_delta, max_per_line)
values (@group_id, @name, @price_delta, @max_per_line)
returning id;

-- name: AdminUpdateOptionFields :exec
update modifier_options
set name = @name, price_delta = @price_delta, max_per_line = @max_per_line
where id = @id;

-- Productos ↔ grupos --------------------------------------------------------

-- name: AdminProductGroups :many
-- Grupos asignados a un producto. min/max efectivos (override o default del grupo), el flag
-- overridden, y el default del grupo (para mostrar "personalizado" vs "por defecto" y restablecer).
select pmg.id, pmg.group_id, mg.name as group_name, mg.is_active as group_active,
       coalesce(pmg.title, mg.name) as title,
       coalesce(pmg.min_select, mg.default_min_select) as min_select,
       coalesce(pmg.max_select, mg.default_max_select) as max_select,
       (pmg.min_select is not null)::bool as overridden,
       mg.default_min_select, mg.default_max_select,
       pmg.position,
       (select count(*) from modifier_options mo where mo.group_id = mg.id and mo.is_active)::int as option_count
from product_modifier_groups pmg
join modifier_groups mg on mg.id = pmg.group_id
where pmg.product_id = @product_id
order by pmg.position, mg.name;

-- name: AdminGroupProducts :many
-- Productos que usan un grupo. min/max efectivos + si el producto sobrescribe el default.
select p.id, p.name,
       coalesce(pmg.min_select, mg.default_min_select) as min_select,
       coalesce(pmg.max_select, mg.default_max_select) as max_select,
       (pmg.min_select is not null)::bool as overridden
from product_modifier_groups pmg
join products p on p.id = pmg.product_id
join modifier_groups mg on mg.id = pmg.group_id
where pmg.group_id = @group_id
order by p.name;

-- name: AdminAttachGroup :exec
-- Asigna/actualiza un grupo en un producto. title ''→NULL (usa el nombre del grupo).
-- min/max NULL = hereda el default del grupo; con valor = override (ambos o ninguno).
insert into product_modifier_groups (product_id, group_id, title, min_select, max_select, position)
values (@product_id, @group_id, nullif(@title::text, ''), sqlc.narg('min_select'), sqlc.narg('max_select'), @position)
on conflict (product_id, group_id) do update
set title = excluded.title, min_select = excluded.min_select,
    max_select = excluded.max_select, position = excluded.position;

-- name: AdminDetachGroup :exec
delete from product_modifier_groups where product_id = @product_id and group_id = @group_id;
