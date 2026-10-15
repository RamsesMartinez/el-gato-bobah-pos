-- +goose Up
-- Treinta `create trigger` toman un candado breve por tabla: si una transacción larga de catálogo
-- está abierta, mejor fallar ruidoso que encolar detrás de ella las escrituras del negocio.
set local lock_timeout = '3s';

-- Versión de catálogo por empresa (spec 034).
--
-- El 2026-10-10 el catálogo de producción se corrigió con un script directo y la tableta siguió
-- ofreciendo un grupo de extras que ya no pertenecía al producto: el menú guardado solo se
-- invalidaba desde las pantallas de administración. El servidor rechazaba cada venta y nadie
-- podía vender ese producto.
--
-- Aquí la versión la suben triggers de las tablas que lee el menú, así que cualquier camino —
-- pantalla, script, migración— la mueve. La llave del menú guardado lleva esta versión: un menú
-- viejo deja de existir en vez de tener que acordarse de borrarlo.
--
-- Aditiva a propósito: la API anterior no la lee y sigue funcionando durante el despliegue.
create table catalog_versions (
  company_id bigint primary key references companies(id) on delete cascade,
  version    bigint not null default 0,
  changed_at timestamptz not null default now()
);

alter table catalog_versions enable row level security;
create policy tenant_isolation on catalog_versions
  using (company_id = nullif(current_setting('app.company_id', true), '')::bigint)
  with check (company_id = nullif(current_setting('app.company_id', true), '')::bigint);
-- Solo lectura para la API: el único que escribe es la función del trigger.
grant select on catalog_versions to gatobobah_app;

-- La empresa sale de las FILAS tocadas y no de app.company_id: un script de corrección no siempre
-- lo pone, y uno solo puede tocar varias empresas.
--
-- security definer: corre como el dueño para escribir el contador sin darle a la API permiso de
-- escribirlo y sin pasar por la política RLS de quien escribe. No amplía nada: solo cuenta filas
-- que quien escribe ya pudo tocar.
--
-- El `exists` en companies: al borrar una empresa la cascada borra su catálogo y este trigger
-- intentaría apuntar un contador a la empresa que se está borrando; la llave foránea tumbaría el
-- borrado entero.
--
-- Tablas de transición y no `for each row`: un script de mil filas cuesta un upsert por empresa,
-- no mil. El `order by` fija el orden de los candados: dos scripts que tocan las mismas empresas en
-- orden distinto se interbloquearían sobre las filas del contador.
-- +goose StatementBegin
create function bump_catalog_version() returns trigger
language plpgsql security definer set search_path = pg_catalog, public, pg_temp as $$
begin
  if tg_op = 'INSERT' then
    insert into public.catalog_versions as cv (company_id, version, changed_at)
    select distinct r.company_id, 1, now() from new_rows r
    where exists (select 1 from public.companies c where c.id = r.company_id)
    order by r.company_id
    on conflict (company_id) do update set version = cv.version + 1, changed_at = now();
  elsif tg_op = 'DELETE' then
    insert into public.catalog_versions as cv (company_id, version, changed_at)
    select distinct r.company_id, 1, now() from old_rows r
    where exists (select 1 from public.companies c where c.id = r.company_id)
    order by r.company_id
    on conflict (company_id) do update set version = cv.version + 1, changed_at = now();
  else
    insert into public.catalog_versions as cv (company_id, version, changed_at)
    select distinct x.company_id, 1, now()
    from (select company_id from new_rows union select company_id from old_rows) x
    where exists (select 1 from public.companies c where c.id = x.company_id)
    order by x.company_id
    on conflict (company_id) do update set version = cv.version + 1, changed_at = now();
  end if;
  return null;
end;
$$;
-- +goose StatementEnd
-- Solo la invocan los triggers; nadie más la llama suelta.
revoke all on function bump_catalog_version() from public;

-- Las tablas que lee el menú del POS (queries/menu.sql y platform_prices.sql). Postgres no admite
-- tablas de transición en un trigger de varios eventos: van tres por tabla.
-- +goose StatementBegin
do $$
declare
  t text;
begin
  foreach t in array array[
    'categories', 'products', 'product_channels', 'channels', 'product_modifier_groups',
    'modifier_groups', 'modifier_options', 'delivery_platforms', 'product_platform_prices',
    'modifier_option_platform_prices'
  ] loop
    execute format('create trigger %I after insert on %I referencing new table as new_rows
      for each statement execute function bump_catalog_version()', t || '_catalog_version_ins', t);
    execute format('create trigger %I after update on %I referencing old table as old_rows new table as new_rows
      for each statement execute function bump_catalog_version()', t || '_catalog_version_upd', t);
    execute format('create trigger %I after delete on %I referencing old table as old_rows
      for each statement execute function bump_catalog_version()', t || '_catalog_version_del', t);
  end loop;
end;
$$;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
do $$
declare
  t text;
begin
  foreach t in array array[
    'categories', 'products', 'product_channels', 'channels', 'product_modifier_groups',
    'modifier_groups', 'modifier_options', 'delivery_platforms', 'product_platform_prices',
    'modifier_option_platform_prices'
  ] loop
    execute format('drop trigger if exists %I on %I', t || '_catalog_version_ins', t);
    execute format('drop trigger if exists %I on %I', t || '_catalog_version_upd', t);
    execute format('drop trigger if exists %I on %I', t || '_catalog_version_del', t);
  end loop;
end;
$$;
-- +goose StatementEnd
drop function if exists bump_catalog_version();
drop table if exists catalog_versions;
