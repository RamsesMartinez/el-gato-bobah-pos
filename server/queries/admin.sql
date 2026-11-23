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
         -- La miniatura de la lista (spec 091): la foto propia o, si no tiene, la de su categoría o
         -- su familia. Una subcategoría sin las suyas hereda las de su padre. Viajan para que la
         -- pantalla no pida nada más.
         p.image_url, coalesce(c.image_url, pc.image_url) as category_image_url,
         coalesce(c.food_family::text, pc.food_family::text, '')::text as category_food_family,
         p.composition_status, p.cost_source, p.manual_cost, (p.recipe_id is not null)::bool as has_recipe,
         (select count(*) from product_modifier_groups pmg
            join modifier_groups mg on mg.id = pmg.group_id
           where pmg.product_id = p.id and mg.is_active)::int as group_count,
         (select count(*) from product_modifier_groups pmg
           where pmg.product_id = p.id and pmg.min_select is not null)::int as override_count,
         -- Los grupos ligados que están apagados (spec 090, defecto 3): no salen en la pantalla de
         -- venta y el producto se veía igual que los demás.
         (select count(*) from product_modifier_groups pmg
            join modifier_groups mg on mg.id = pmg.group_id
           where pmg.product_id = p.id and not mg.is_active)::int as inactive_group_count,
         p.alt_names,
         -- Dónde se vende y a qué precio en la web (spec 092). Subconsultas por código y no un join:
         -- cuando el lugar de venta sea también por sucursal, un join multiplicaría las filas.
         -- Sin fila = se ofrece, al precio de mostrador.
         (not exists (select 1 from product_channels pc join channels ch on ch.id = pc.channel_id
                       where pc.product_id = p.id and ch.code = 'pos' and pc.visibility = 'oculto'))::bool as sold_at_counter,
         (not exists (select 1 from product_channels pc join channels ch on ch.id = pc.channel_id
                       where pc.product_id = p.id and ch.code = 'online' and pc.visibility = 'oculto'))::bool as sold_on_web,
         (select pc.price from product_channels pc join channels ch on ch.id = pc.channel_id
           where pc.product_id = p.id and ch.code = 'online') as web_price,
         p.allow_oversell, p.track_stock, coalesce(p.description, '')::text as description,
         p.expected_waste_pct,
         (exists (select 1 from recipe_items ri where ri.recipe_id = p.recipe_id))::bool as has_recipe_items,
         -- El alternativo que coincidió (spec 063): '' si no se busca o si coincide el nombre; el
         -- primero en el orden capturado. coalesce: la subconsulta sin coincidencia es NULL.
         (case when @search::text = '' or strpos(catalog_name_key(p.name), catalog_name_key(@search)) > 0 then ''
               else coalesce((select a from unnest(p.alt_names) with ordinality u(a, n)
                               where strpos(catalog_name_key(a), catalog_name_key(@search)) > 0
                               order by n limit 1), '') end)::text as matched_alt
  from products p
  join categories c on c.id = p.category_id
  left join categories pc on pc.id = c.parent_id
  -- El producto genérico de plataforma (0077) no es del catálogo: lo usa el sistema para renglones
  -- sin pareja. Editarlo o borrarlo desde aquí dejaría a esos pedidos sin a dónde ir.
  where p.system_kind is null
    and (@status::text = ''
          or (@status = 'act' and p.is_active)
          or (@status = 'inact' and not p.is_active))
    -- Sin mayúsculas ni acentos, por nombre o alternativo (spec 063). strpos y no ilike: lo que alguien
    -- escribe es texto, nunca un comodín.
    and (@search::text = ''
          or strpos(catalog_name_key(p.name), catalog_name_key(@search)) > 0
          or exists (select 1 from unnest(p.alt_names) a where strpos(catalog_name_key(a), catalog_name_key(@search)) > 0))
    and (@category_id::bigint = 0 or p.category_id = @category_id or c.parent_id = @category_id)
    -- Composición (spec 028): ''=todas | 'none'=sin capturar | 'estimated'=por revisar.
    and (@composition::text = ''
          or (@composition = 'none' and p.composition_status is null)
          or (@composition = 'estimated' and p.composition_status = 'estimated'))
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
-- La foto NO se copia (spec 091): su archivo lleva el id del original, y quitar la del duplicado
-- borraría la del original.
insert into products (
  name, description, type, category_id, price, cost_source, manual_cost, current_cost,
  recipe_id, track_stock, allow_oversell, min_stock, is_favorite, sort_key, is_active
)
select sqlc.arg(name), p.description, p.type, p.category_id, p.price, p.cost_source, p.manual_cost, p.current_cost,
       sqlc.narg(recipe_id), p.track_stock, p.allow_oversell, p.min_stock, p.is_favorite, p.sort_key, p.is_active
from products p where p.id = sqlc.arg(src_id)
returning id;

-- name: CloneProductModifierGroups :exec
insert into product_modifier_groups (product_id, group_id, title, min_select, max_select, position)
select sqlc.arg(dst_product), pmg.group_id, pmg.title, pmg.min_select, pmg.max_select, pmg.position
from product_modifier_groups pmg where pmg.product_id = sqlc.arg(src_product);

-- name: CloneProductChannels :exec
insert into product_channels (product_id, channel_id, visibility, price)
select sqlc.arg(dst_product), pc.channel_id, pc.visibility, pc.price
from product_channels pc where pc.product_id = sqlc.arg(src_product);

-- ==== Dónde se vende (spec 092) ====

-- name: EnsureChannel :one
-- El lugar de venta de la empresa de la sesión (RLS pone el filtro), creado si faltaba: una empresa
-- que nació sin catálogo no puede quedarse con un «apagar Web» que responde bien sin escribir.
with created as (
  insert into channels (code, name) values (sqlc.arg(code), sqlc.arg(name))
  on conflict (company_id, code) do nothing
  returning id
)
select id from created
union all
select id from channels where code = sqlc.arg(code)
limit 1;

-- name: UpsertProductChannel :exec
-- Por la restricción con nombre: cuando el lugar de venta sea también por sucursal, cambia la
-- restricción y no esta consulta.
insert into product_channels (product_id, channel_id, visibility, price)
values (sqlc.arg(product_id), sqlc.arg(channel_id), sqlc.arg(visibility), sqlc.narg(price))
on conflict on constraint product_channels_pkey
do update set visibility = excluded.visibility, price = excluded.price;

-- name: GetProductSalesForAudit :one
-- Lo que había antes de guardar, con candado: el precio web cambia en la misma transacción que su
-- evento, como el de mostrador.
-- Trae también lo que decide si el producto descuenta del almacén: «vender sin existencia» apagado
-- en uno que no descuenta nada no significa nada hoy y lo agotaría el día que gane receta.
select p.id, p.allow_oversell, coalesce(p.description, '')::text as description,
       (select pc.price from product_channels pc join channels ch on ch.id = pc.channel_id
         where pc.product_id = p.id and ch.code = 'online') as web_price,
       (not exists (select 1 from product_channels pc join channels ch on ch.id = pc.channel_id
                     where pc.product_id = p.id and ch.code = 'pos' and pc.visibility = 'oculto'))::bool as sold_at_counter,
       (not exists (select 1 from product_channels pc join channels ch on ch.id = pc.channel_id
                     where pc.product_id = p.id and ch.code = 'online' and pc.visibility = 'oculto'))::bool as sold_on_web,
       p.track_stock, (p.type = 'combo')::bool as is_package, p.composition_status,
       (exists (select 1 from recipe_items ri where ri.recipe_id = p.recipe_id))::bool as has_recipe_items
from products p where p.id = sqlc.arg(id)
for update of p;

-- name: SetProductSalesFields :exec
update products set allow_oversell = sqlc.arg(allow_oversell), description = sqlc.narg(description), updated_at = now()
where id = sqlc.arg(id);

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

-- name: AdminSetProductManualCost :one
-- Costo capturado a mano. current_cost se escribe en la misma sentencia para que el margen de la
-- lista cambie aunque el recálculo posterior falle. Un combo no se toca: su costo es la suma de
-- sus componentes y el motor de costeo sobrescribiría cualquier captura.
update products
set cost_source = 'manual', manual_cost = sqlc.arg(cost), current_cost = sqlc.arg(cost), updated_at = now()
where id = sqlc.arg(id) and type <> 'combo'
returning id;

-- name: AdminSetProductRecipeCost :one
-- Vuelve a costear por receta; solo si el producto tiene una. El monto lo pone el motor de costeo.
update products
set cost_source = 'receta', updated_at = now()
where id = $1 and recipe_id is not null and type <> 'combo'
returning id;

-- name: AdminListModifierOptions :many
-- Página de opciones (de grupos activos) filtrada por estado (''=todas | 'act' | 'inact') y
-- búsqueda (nombre de opción o de grupo). count(*) over() = total del filtro, para el paginador.
-- Incluye inactivas y price_delta: el POS puede mostrar/cobrar una opción archivada al reactivarla.
select mo.id, mo.group_id, mg.name as group_name, mo.name, mo.price_delta, mo.is_favorite, mo.is_active,
       mo.composition_status, count(*) over() as total
from modifier_options mo
join modifier_groups mg on mg.id = mo.group_id
where mg.is_active
  and (@status::text = ''
        or (@status = 'act' and mo.is_active)
        or (@status = 'inact' and not mo.is_active))
  and (@search::text = '' or mo.name ilike '%' || @search || '%' or mg.name ilike '%' || @search || '%')
  and (@composition::text = ''
        or (@composition = 'none' and mo.composition_status is null)
        or (@composition = 'estimated' and mo.composition_status = 'estimated'))
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
