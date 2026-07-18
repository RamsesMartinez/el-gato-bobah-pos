-- Lecturas planas para armar el documento del menú POS (se ensambla en Go y se cachea en Redis).

-- name: MenuCategories :many
select id, name, parent_id, sort_key, color, image_url
from categories
where is_active
order by parent_id nulls first, sort_key, name;

-- name: MenuProducts :many
select p.id, p.name, p.description, p.price, p.current_cost,
       p.category_id, p.type, p.is_favorite, p.image_url, p.track_stock
from products p
where p.is_active
  and (p.available_from is null or p.available_from <= current_date)
  and (p.available_until is null or p.available_until >= current_date)
  and not exists (
    select 1 from product_channels pc
    join channels c on c.id = pc.channel_id
    where pc.product_id = p.id and c.code = 'pos' and pc.visibility = 'oculto'
  )
order by p.sort_key, p.name;

-- name: MenuProductGroups :many
-- min/max efectivos: el override del producto o, si es NULL, el default del grupo.
select pmg.product_id, pmg.group_id, coalesce(pmg.title, mg.name) as title,
       coalesce(pmg.min_select, mg.default_min_select) as min_select,
       coalesce(pmg.max_select, mg.default_max_select) as max_select,
       pmg.position
from product_modifier_groups pmg
join modifier_groups mg on mg.id = pmg.group_id
where mg.is_active
order by pmg.product_id, pmg.position;

-- name: MenuOptions :many
select mo.id, mo.group_id, mo.name, mo.price_delta, mo.max_per_line, mo.is_favorite
from modifier_options mo
where mo.is_active
order by mo.group_id, mo.sort_key, mo.name;

-- name: PopularProducts :many
-- Productos más vendidos por cantidad en los últimos 30 días; alimenta la pestaña "Top" del POS.
-- ceiling: tope 60 filas (el N configurable del front es << 60). Subir si algún día se necesita más.
select ol.product_id, sum(ol.quantity)::numeric(12,2) as qty
from order_lines ol
join orders o on o.id = ol.order_id
where o.status <> 'cancelada'
  and ol.cancelled_at is null
  and o.opened_at >= now() - interval '30 days'
group by ol.product_id
order by qty desc, ol.product_id
limit 60;
