-- +goose Up
-- Menú público v1 (spec 097, #316): el nombre con el que cada negocio aparece en
-- menu.elgatobobah.com/<nombre>, su historial y el código estable de su QR.
--
-- Tolera a la versión anterior corriendo (constitución IV): todo es nuevo y la API anterior no lee
-- nada de esto. Una empresa que la consola cree con la versión anterior durante el traslape nace sin
-- menú; la API nueva se lo crea la primera vez que alguien abre Ajustes › Venta en web.
set local lock_timeout = '3s';

-- Un menú por empresa hoy. Con id propio para que el menú por sucursal (puerta VIII) sea una columna
-- `branch_id` nullable sin reconstruir la llave del historial.
create table public_menus (
  id            bigint generated always as identity primary key,
  company_id    bigint not null unique references companies (id) on delete cascade,
  -- Lo que codifica el QR impreso: no cambia nunca, aunque cambie el nombre. Aleatorio y no el id de
  -- la empresa, para que no se puedan recorrer los negocios contando.
  qr_token      text not null unique constraint public_menus_qr_token_shape check (qr_token ~ '^[0-9a-f]{12}$'),
  -- Los ids de PALETTES de la pantalla Interfaz (web/src/stores/ui.ts) y de domain.ValidPalette.
  brand_palette text not null default 'brand' constraint public_menus_brand_palette_known
    check (brand_palette in ('brand', 'blue', 'teal', 'green', 'cyan', 'purple', 'pink', 'orange')),
  -- El #330 agrega plantillas ampliando este check.
  template_key  text not null default 'classic' constraint public_menus_template_known check (template_key in ('classic')),
  created_at    timestamptz not null default now(),
  updated_at    timestamptz not null default now(),
  unique (company_id, id)
);

-- El nombre vigente y los anteriores. `name` es único GLOBAL, vigentes y retirados: un nombre usado
-- nunca pasa a otro negocio, porque un QR impreso o un enlace viejo de WhatsApp caería en un menú
-- ajeno. Volver a un nombre propio anterior reactiva su fila.
create table public_menu_names (
  id         bigint generated always as identity primary key,
  company_id bigint not null,
  menu_id    bigint not null,
  name       text not null constraint public_menu_names_name_key unique
             constraint public_menu_names_name_shape
             check (char_length(name) between 3 and 40 and name ~ '^[a-z0-9]+(-[a-z0-9]+)*$'),
  retired_at timestamptz,
  created_at timestamptz not null default now(),
  -- El nombre y su menú, de la misma empresa: los chequeos de integridad saltan RLS.
  foreign key (company_id, menu_id) references public_menus (company_id, id) on delete cascade
);
create unique index public_menu_names_one_current on public_menu_names (company_id) where retired_at is null;
create index public_menu_names_company on public_menu_names (company_id);

-- Sin `force`: el relleno de abajo y CreateCompany corren como owner, sin app.company_id.
alter table public_menus enable row level security;
alter table public_menu_names enable row level security;
create policy tenant_isolation on public_menus
  using (company_id = nullif(current_setting('app.company_id', true), '')::bigint)
  with check (company_id = nullif(current_setting('app.company_id', true), '')::bigint);
create policy tenant_isolation on public_menu_names
  using (company_id = nullif(current_setting('app.company_id', true), '')::bigint)
  with check (company_id = nullif(current_setting('app.company_id', true), '')::bigint);
grant select, insert, update on public_menus to gatobobah_app;
grant select, insert, update on public_menu_names to gatobobah_app;

-- ---------------------------------------------------------------------------------------------
-- RESOLVER UN NOMBRE SIN SESIÓN. Quien abre el menú no tiene empresa: la empresa es el RESULTADO de
-- leer el nombre. Bajo el rol de la aplicación y sin tenant, RLS no devuelve nada. La salida es la de
-- `candidatas_del_aviso` (0073): una vista cuyo dueño es un rol propio, sin login, con políticas que
-- solo a él le dejan ver todas las filas. La excepción queda acotada a esta proyección.
--
-- LA VISTA NO PROYECTA NADA MÁS QUE NOMBRES Y CÓDIGOS, que son públicos por diseño: cualquier
-- petición del rol de la aplicación puede leer qué nombre y qué código son de qué empresa, y nada
-- más. Nunca `companies.slug`: es la mitad de la credencial usuario@slug. Por eso el grant sobre
-- companies es por columna. Con la empresa resuelta, el servicio fija el tenant y lo demás corre
-- bajo RLS como cualquier petición.
-- ---------------------------------------------------------------------------------------------
-- +goose StatementBegin
do $$
begin
  if not exists (select 1 from pg_roles where rolname = 'gatobobah_public_menu') then
    create role gatobobah_public_menu nologin;
  end if;
end $$;
-- +goose StatementEnd
alter role gatobobah_public_menu nologin nobypassrls;

grant select on public_menus, public_menu_names to gatobobah_public_menu;
grant select (id, is_active) on companies to gatobobah_public_menu;
create policy public_menu_resolves on public_menus for select to gatobobah_public_menu using (true);
create policy public_menu_resolves on public_menu_names for select to gatobobah_public_menu using (true);
create policy public_menu_resolves on companies for select to gatobobah_public_menu using (true);

-- security_barrier: ningún predicado de quien consulta corre antes del filtro de empresa activa.
create view public_menu_routes with (security_barrier = true) as
  select n.name,
         n.company_id,
         (n.retired_at is null) as is_current,
         cur.name               as current_name,
         m.qr_token
    from public_menu_names n
    join public_menus m      on m.id = n.menu_id and m.company_id = n.company_id
    join public_menu_names cur on cur.menu_id = m.id and cur.retired_at is null
    join companies c         on c.id = n.company_id
   where c.is_active;
alter view public_menu_routes owner to gatobobah_public_menu;
revoke all on public_menu_routes from public;
grant select on public_menu_routes to gatobobah_app;

-- ---------------------------------------------------------------------------------------------
-- Nombres. public_menu_slugify es el espejo EXACTO de domain.SlugifyMenuName (misma tabla de
-- acentos; una prueba compara los dos). Las mayúsculas se bajan con translate y no con lower(), que
-- depende de la collation: así es immutable de verdad.
-- ---------------------------------------------------------------------------------------------
create function public_menu_slugify(s text) returns text
language sql immutable parallel safe as $$
  select trim(both '-' from left(
           trim(both '-' from regexp_replace(
             translate(
               translate(coalesce(s, ''),
                 'áàâäãéèêëíìîïóòôöõúùûüñçÁÀÂÄÃÉÈÊËÍÌÎÏÓÒÔÖÕÚÙÛÜÑÇ',
                 'aaaaaeeeeiiiiooooouuuuncAAAAAEEEEIIIIOOOOOUUUUNC'),
               'ABCDEFGHIJKLMNOPQRSTUVWXYZ', 'abcdefghijklmnopqrstuvwxyz'),
             '[^a-z0-9]+', '-', 'g')),
           40))
$$;

-- public_menu_free_name da un nombre libre a partir de una base ya normalizada. Nunca devuelve algo
-- que viole el check: base vacía → menu-<empresa>; corta → menu-<base>; tomada o reservada → con
-- sufijo, recortando la base para no pasar de 40. Los reservados son domain.reservedMenuNames.
-- +goose StatementBegin
create function public_menu_free_name(base text, cid bigint) returns text
language plpgsql volatile set search_path = pg_catalog, public as $$
declare
  b        text := coalesce(base, '');
  cand     text;
  n        int := 1;
  sfx      text;
  reserved text[] := array['q', 'api', 'admin', 'www', 'menu', 'menus', 'static', 'assets', 'login',
                           'ayuda', 'soporte', 'app', 'staff', 'media', 'cdn', 'robots-txt',
                           'favicon-ico', 'sitemap-xml', 'functions'];
begin
  if b = '' then
    b := 'menu-' || cid::text;
  elsif char_length(b) < 3 then
    b := 'menu-' || b;
  end if;
  b := trim(both '-' from left(b, 40));
  cand := b;
  while cand = any (reserved) or exists (select 1 from public_menu_names where name = cand) loop
    n := n + 1;
    sfx := '-' || n::text;
    cand := rtrim(left(b, 40 - char_length(sfx)), '-') || sfx;
  end loop;
  return cand;
end;
$$;
-- +goose StatementEnd

-- Relleno: cada empresa con su menú y un nombre derivado del NOMBRE de la empresa, nunca del slug; el
-- de los tickets solo si la empresa no tiene. No al revés: en producción la empresa de pruebas imprime
-- «El Gato Bobah» en sus tickets y, por ser la más vieja, le quitaba el nombre al negocio real. En un loop ordenado: cada vuelta ve lo que insertó la anterior,
-- y la empresa más vieja gana un nombre empatado.
-- +goose StatementBegin
do $$
declare
  r   record;
  mid bigint;
begin
  for r in
    select c.id, coalesce(nullif(trim(c.name), ''), b.business_name) as display
      from companies c
      left join business_settings b on b.company_id = c.id
     order by c.id
  loop
    insert into public_menus (company_id, qr_token)
    values (r.id, substr(replace(gen_random_uuid()::text, '-', ''), 1, 12))
    returning id into mid;
    insert into public_menu_names (company_id, menu_id, name)
    values (r.id, mid, public_menu_free_name(public_menu_slugify(r.display), r.id));
  end loop;
end $$;
-- +goose StatementEnd

-- +goose Down
-- Pierde los nombres elegidos y los códigos QR: al volver a subir, el Up genera códigos nuevos y un QR
-- impreso deja de llevar al menú. No se niega: no hay a dónde guardarlos. El rol es del clúster y se
-- queda (nologin), como en la 0073.
set local lock_timeout = '3s';
drop view if exists public_menu_routes;
drop function if exists public_menu_free_name(text, bigint);
drop function if exists public_menu_slugify(text);
drop policy if exists public_menu_resolves on companies;
revoke select (id, is_active) on companies from gatobobah_public_menu;
drop table if exists public_menu_names;
drop table if exists public_menus;
