-- +goose Up
-- Favorito por opción de modificador: alimenta la estrategia de recomendación
-- "por favoritos" (además de "inteligente" por % y "alfabético").
alter table modifier_options add column is_favorite boolean not null default false;
create index modifier_options_favorite on modifier_options (group_id) where is_favorite;

-- +goose Down
drop index if exists modifier_options_favorite;
alter table modifier_options drop column is_favorite;
