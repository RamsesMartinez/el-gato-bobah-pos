-- +goose Up
-- Fotos del catálogo (spec 091): la familia de comida de cada categoría (decide la ilustración de lo
-- que no tiene foto), la forma de image_url y los borrados de archivos que fallaron.
--
-- Solo agrega. La API anterior no escribe image_url (solo la copia al duplicar, y la copia pasa el
-- check) ni conoce food_family ni la tabla nueva, así que sigue funcionando durante el traslape.
set local lock_timeout = '3s';

-- La lista es la de domain.FoodFamilies y la del sprite del front. Un enum y no un check sobre texto:
-- agregar una familia es `alter type … add value`, sin reescribir la tabla.
create type food_family as enum (
  'cold_drink', 'coffee', 'tea', 'frappe', 'soda', 'soup', 'snack', 'wings', 'dessert', 'breakfast', 'dish'
);

-- Null = la genérica. No se rellena: ninguna categoría tiene familia hasta que alguien la elige.
alter table categories add column food_family food_family;

-- image_url la escribe solo la API, con la URL de la variante de 1200. `not valid`: vale para toda
-- escritura nueva y no revisa las filas que ya existen, así que una URL vieja escrita a mano no
-- impide que la API nueva arranque (migra al arrancar).
alter table products add constraint products_image_url_shape
  check (image_url is null or image_url ~ '^https?://[^\s?#]+-1200\.webp$') not valid;
alter table categories add constraint categories_image_url_shape
  check (image_url is null or image_url ~ '^https?://[^\s?#]+-1200\.webp$') not valid;

-- Archivos cuyo borrado falló (el almacenamiento no respondió). La foto nueva ya quedó puesta; el
-- archivo viejo se reintenta al terminar la siguiente escritura de foto de la misma empresa.
create table media_pending_deletions (
  id         bigint generated always as identity primary key,
  company_id bigint not null default nullif(current_setting('app.company_id', true), '')::bigint
             references companies (id) on delete cascade,
  object_key text not null,
  attempts   int not null default 1,
  last_error text not null,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  constraint media_pending_deletions_key unique (company_id, object_key),
  constraint media_pending_deletions_key_shape
    check (object_key ~ '^[0-9]+/(products|categories)/[0-9]+/[0-9a-f]{32}-(400|1200)\.webp$'),
  -- El barrido borra lo que dice esta columna: una llave de otra empresa sería borrar un archivo ajeno.
  constraint media_pending_deletions_key_company check (object_key like company_id::text || '/%'),
  constraint media_pending_deletions_attempts check (attempts >= 1),
  constraint media_pending_deletions_error check (char_length(last_error) <= 300)
);

alter table media_pending_deletions enable row level security;
create policy tenant_isolation on media_pending_deletions
  using (company_id = nullif(current_setting('app.company_id', true), '')::bigint)
  with check (company_id = nullif(current_setting('app.company_id', true), '')::bigint);
grant select, insert, delete on media_pending_deletions to gatobobah_app;
grant update (attempts, last_error, updated_at) on media_pending_deletions to gatobobah_app;

-- +goose Down
-- Se niega con borrados pendientes: bajar perdería las llaves y esos archivos quedarían en el bucket
-- para siempre. Las familias elegidas sí se pierden (la prueba lo limpia en guardedDownCleanup).
set local lock_timeout = '3s';
-- +goose StatementBegin
do $$
begin
  if exists (select 1 from media_pending_deletions) then
    raise exception 'hay archivos pendientes de borrar en media_pending_deletions: bórralos del almacenamiento antes de bajar la 0117';
  end if;
end $$;
-- +goose StatementEnd
drop table media_pending_deletions;
alter table categories drop constraint categories_image_url_shape;
alter table products drop constraint products_image_url_shape;
alter table categories drop column food_family;
drop type food_family;
