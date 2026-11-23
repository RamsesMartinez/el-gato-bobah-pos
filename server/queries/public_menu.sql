-- Menú público (spec 097). Las dos primeras resuelven SIN empresa, sobre la vista de la 0121: lo
-- único que devuelven es a qué empresa pertenece un nombre o un código, que es público por diseño.

-- name: ResolvePublicMenuName :one
select company_id, is_current::bool as is_current, current_name from public_menu_routes where name = $1;

-- name: ResolvePublicMenuToken :one
select company_id, current_name from public_menu_routes where qr_token = $1 and is_current;

-- El resto corre con la empresa fijada, bajo RLS.

-- name: EnsurePublicMenu :exec
-- Una empresa que la consola creó con la versión anterior durante el traslape nació sin menú. Idempotente.
with m as (
  insert into public_menus (company_id, qr_token)
  values (sqlc.arg(company_id), substr(replace(gen_random_uuid()::text, '-', ''), 1, 12))
  on conflict (company_id) do nothing
  returning id, company_id
)
insert into public_menu_names (company_id, menu_id, name)
select m.company_id, m.id, sqlc.arg(name) from m;

-- name: LockPublicMenu :one
-- Toma el menú de la empresa: dos cambios de nombre a la vez (dos pestañas, doble toque) se forman.
select id, qr_token, brand_palette from public_menus where company_id = sqlc.arg(company_id) for update;

-- name: GetPublicMenu :one
select m.id, m.qr_token, m.brand_palette, m.template_key,
       coalesce((select n.name from public_menu_names n where n.menu_id = m.id and n.retired_at is null), '')::text as name
from public_menus m where m.company_id = sqlc.arg(company_id);

-- name: PublicMenuNames :many
-- Los nombres de la empresa: el vigente primero y los anteriores del más reciente al más viejo.
select name, retired_at from public_menu_names where company_id = sqlc.arg(company_id)
order by retired_at desc nulls first, id desc;

-- name: MenuNameTakenElsewhere :one
-- Por la vista: el nombre puede ser de otra empresa, que RLS esconde.
select exists (select 1 from public_menu_routes where name = sqlc.arg(name) and company_id <> sqlc.arg(company_id))::bool;

-- name: RetireCurrentMenuName :execrows
update public_menu_names set retired_at = now()
where company_id = sqlc.arg(company_id) and retired_at is null;

-- name: ReviveMenuName :execrows
update public_menu_names set retired_at = null
where company_id = sqlc.arg(company_id) and name = sqlc.arg(name) and retired_at is not null;

-- name: InsertMenuName :exec
insert into public_menu_names (company_id, menu_id, name) values (sqlc.arg(company_id), sqlc.arg(menu_id), sqlc.arg(name));

-- name: SetMenuPalette :exec
update public_menus set brand_palette = sqlc.arg(palette), updated_at = now() where company_id = sqlc.arg(company_id);

-- name: CompanyDisplayName :one
-- El nombre del negocio para el encabezado del menú y para nombrar un menú nuevo: el de la empresa, o
-- el de los tickets si la empresa no tiene.
select coalesce(nullif(trim(c.name), ''), b.business_name, '')::text
from companies c left join business_settings b on b.company_id = c.id
where c.id = sqlc.arg(company_id);

-- name: FreeMenuNameFor :one
-- Para nombrar un menú nuevo con la misma regla que el relleno de la 0121.
select public_menu_free_name(public_menu_slugify(sqlc.arg(display)::text), sqlc.arg(company_id)::bigint)::text;

-- name: PublicMenuCategories :many
-- Categorías activas con su principal (dos niveles, spec 085).
select id, parent_id, name, sort_key, image_url, coalesce(food_family::text, '')::text as food_family
from categories where is_active
order by sort_key, name;
