-- name: AdminListProducts :many
-- Página filtrada por estado (''=todos | 'act' | 'inact'), búsqueda (''=sin filtro), categoría
-- (@category_id=0 → todas; elegir una raíz incluye sus subcategorías vía c.parent_id) y grupos
-- (''=todos | 'none'=sin grupos activos | 'some'=con grupos). Orden por columna: @sort ∈
-- (''|name|price|cost|margin|category|groups) × @dir (asc|desc); default nombre asc. count(*)
-- over() (tras el filtro de grupos) = total del filtro en la misma consulta (sin viaje extra).
select q.*, count(*) over() as total
from (
  select p.id, p.name, p.price, p.current_cost, p.type, p.is_active, p.is_favorite,
         p.available_from, p.available_until, p.needs_prep, c.name as category, p.category_id,
         (select count(*) from product_modifier_groups pmg
            join modifier_groups mg on mg.id = pmg.group_id
           where pmg.product_id = p.id and mg.is_active)::int as group_count,
         (select count(*) from product_modifier_groups pmg
           where pmg.product_id = p.id and pmg.min_select is not null)::int as override_count
  from products p
  join categories c on c.id = p.category_id
  where (@status::text = ''
          or (@status = 'act' and p.is_active)
          or (@status = 'inact' and not p.is_active))
    and (@search::text = '' or p.name ilike '%' || @search || '%')
    and (@category_id::bigint = 0 or p.category_id = @category_id or c.parent_id = @category_id)
) q
where (@groups::text = ''
        or (@groups = 'none' and q.group_count = 0)
        or (@groups = 'some' and q.group_count > 0))
order by
  case when @sort::text = 'price'    and @dir::text = 'asc'  then q.price end asc  nulls last,
  case when @sort::text = 'price'    and @dir::text <> 'asc' then q.price end desc nulls last,
  case when @sort::text = 'cost'     and @dir::text = 'asc'  then q.current_cost end asc  nulls last,
  case when @sort::text = 'cost'     and @dir::text <> 'asc' then q.current_cost end desc nulls last,
  case when @sort::text = 'margin'   and @dir::text = 'asc'  then (q.price - q.current_cost) end asc  nulls last,
  case when @sort::text = 'margin'   and @dir::text <> 'asc' then (q.price - q.current_cost) end desc nulls last,
  case when @sort::text = 'groups'   and @dir::text = 'asc'  then q.group_count end asc  nulls last,
  case when @sort::text = 'groups'   and @dir::text <> 'asc' then q.group_count end desc nulls last,
  case when @sort::text = 'category' and @dir::text = 'asc'  then q.category end asc  nulls last,
  case when @sort::text = 'category' and @dir::text <> 'asc' then q.category end desc nulls last,
  case when @sort::text = 'name'     and @dir::text = 'desc' then q.name end desc nulls last,
  q.name
limit nullif(@lim::int, 0) offset @off;  -- lim=0 → sin límite (POS modo edición pide todo)

-- name: AdminCreateProduct :one
-- Alta mínima de producto (tipo 'simple', activo por defecto). El costo/receta/canales se
-- configuran después; sku/imagen quedan null.
insert into products (name, category_id, price, is_favorite, track_stock)
values ($1, $2, $3, $4, $5)
returning id;

-- ==== Duplicar producto (clon profundo con sus relaciones) ====

-- name: GetProductCloneInfo :one
-- Datos que deciden el clon: tipo (para slots de combo) y si tiene receta propia (a clonar).
select type, recipe_id from products where id = $1;

-- name: CloneRecipe :one
-- Receta vacía nueva (hereda company_id del GUC vía default); luego se copian sus ítems.
insert into recipes default values returning id;

-- name: CloneRecipeItems :exec
-- Alias en la fuente: el insert y el select son sobre tablas del mismo esquema; sin alias, sqlc
-- reporta columnas ambiguas entre el target del insert y la fuente del select.
insert into recipe_items (recipe_id, ingredient_id, quantity, unit_id, position)
select sqlc.arg(dst_recipe), ri.ingredient_id, ri.quantity, ri.unit_id, ri.position
from recipe_items ri where ri.recipe_id = sqlc.arg(src_recipe);

-- name: CloneProductRow :one
-- Copia la fila del producto de origen con nombre nuevo. sku queda null (es unique; no se copia)
-- y recipe_id apunta a la receta ya clonada (o null). company_id lo sella el default del GUC.
insert into products (
  name, description, type, category_id, price, cost_source, manual_cost, current_cost,
  recipe_id, track_stock, allow_oversell, min_stock, is_favorite, sort_key, image_url, is_active
)
select sqlc.arg(name), p.description, p.type, p.category_id, p.price, p.cost_source, p.manual_cost, p.current_cost,
       sqlc.narg(recipe_id), p.track_stock, p.allow_oversell, p.min_stock, p.is_favorite, p.sort_key, p.image_url, p.is_active
from products p where p.id = sqlc.arg(src_id)
returning id;

-- name: CloneProductModifierGroups :exec
insert into product_modifier_groups (product_id, group_id, title, min_select, max_select, position)
select sqlc.arg(dst_product), pmg.group_id, pmg.title, pmg.min_select, pmg.max_select, pmg.position
from product_modifier_groups pmg where pmg.product_id = sqlc.arg(src_product);

-- name: CloneProductChannels :exec
insert into product_channels (product_id, channel_id, visibility)
select sqlc.arg(dst_product), pc.channel_id, pc.visibility
from product_channels pc where pc.product_id = sqlc.arg(src_product);

-- name: ListComboSlots :many
select id, name, min_select, max_select, position from combo_slots where combo_id = $1 order by position, id;

-- name: CloneComboSlot :one
insert into combo_slots (combo_id, name, min_select, max_select, position)
values ($1, $2, $3, $4, $5)
returning id;

-- name: CloneComboSlotProducts :exec
insert into combo_slot_products (slot_id, product_id, price_delta, is_default)
select sqlc.arg(dst_slot), csp.product_id, csp.price_delta, csp.is_default
from combo_slot_products csp where csp.slot_id = sqlc.arg(src_slot);

-- name: AdminProductCounts :one
-- Totales del catálogo por estado, para las pestañas (independientes de la búsqueda, como antes).
select count(*) filter (where is_active)::int as active,
       count(*) filter (where not is_active)::int as inactive
from products;

-- name: AdminUpdateProduct :exec
update products
set name = $2, price = $3, is_favorite = $4, is_active = $5,
    available_from = $6, available_until = $7, needs_prep = sqlc.arg(needs_prep),
    updated_at = now()
where id = $1;

-- name: AdminListModifierOptions :many
-- Página de opciones (de grupos activos) filtrada por estado (''=todas | 'act' | 'inact') y
-- búsqueda (nombre de opción o de grupo). count(*) over() = total del filtro, para el paginador.
-- Incluye inactivas y price_delta: el POS puede mostrar/cobrar una opción archivada al reactivarla.
select mo.id, mo.group_id, mg.name as group_name, mo.name, mo.price_delta, mo.is_favorite, mo.is_active,
       count(*) over() as total
from modifier_options mo
join modifier_groups mg on mg.id = mo.group_id
where mg.is_active
  and (@status::text = ''
        or (@status = 'act' and mo.is_active)
        or (@status = 'inact' and not mo.is_active))
  and (@search::text = '' or mo.name ilike '%' || @search || '%' or mg.name ilike '%' || @search || '%')
order by mg.name, mo.sort_key, mo.name
limit nullif(@lim::int, 0) offset @off;  -- lim=0 → sin límite (el POS pide todas)

-- name: AdminModifierOptionCounts :one
-- Totales por estado (opciones de grupos activos), para las pestañas. Independiente de la búsqueda.
select count(*) filter (where mo.is_active)::int as active,
       count(*) filter (where not mo.is_active)::int as inactive
from modifier_options mo
join modifier_groups mg on mg.id = mo.group_id
where mg.is_active;

-- name: AdminSetOptionFavorite :exec
update modifier_options set is_favorite = $2 where id = $1;

-- name: AdminSetOptionActive :exec
update modifier_options set is_active = $2 where id = $1;

-- name: CategoryExists :one
-- Comprobación de PERTENENCIA bajo RLS antes de mover un producto de categoría. La llave foránea
-- no alcanza: sus chequeos saltan RLS por diseño, así que un category_id de otra empresa entraría
-- sin protestar y el producto desaparecería de su propio menú —el join sí corre bajo RLS y no
-- encontraría la categoría— sin que nada avise y sin forma de arreglarlo desde la pantalla.
select exists(select 1 from categories where id = $1);

-- name: AdminUpdateProductCategory :exec
-- Va aparte del update general para que la categoría solo se toque cuando de verdad se pidió: una
-- petición sin categoría no debe mover el producto, y un cliente viejo que no manda el campo no
-- puede terminar mandándolos todos a la categoría 0.
update products set category_id = $2, updated_at = now() where id = $1;
