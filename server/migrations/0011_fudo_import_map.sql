-- +goose Up
-- Trazabilidad del importador FUDO: mapea (entidad, id/nombre FUDO) → id nuevo.
create table fudo_import_map (
  id        bigint generated always as identity primary key,
  entity    text not null,
  fudo_id   bigint,
  fudo_name text,
  new_table text not null,
  new_id    bigint not null
);
create unique index fudo_import_map_key
  on fudo_import_map (entity, coalesce(fudo_id, 0), coalesce(fudo_name, ''));

-- +goose Down
drop table if exists fudo_import_map;
