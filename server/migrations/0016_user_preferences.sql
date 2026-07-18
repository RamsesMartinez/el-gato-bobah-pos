-- +goose Up
-- Preferencias por usuario (clave→valor JSON). Genérico: sirve para el orden de categorías
-- del POS (key 'pos.cat-order') y cualquier preferencia futura, sincronizada entre tablets.
create table user_preferences (
  user_id    bigint not null references users(id) on delete cascade,
  key        text   not null,
  value      jsonb  not null,
  updated_at timestamptz not null default now(),
  primary key (user_id, key)
);

-- +goose Down
drop table if exists user_preferences;
