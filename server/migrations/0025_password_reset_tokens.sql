-- +goose Up
-- Tokens de recuperación de contraseña. Mismo patrón que refresh_tokens: opaco, guardado solo
-- por su sha256, un solo uso, con caducidad. Per-tenant (RLS): la confirmación fija el tenant
-- desde el cid embebido en el link (cid.token), como el refresh.
create table password_reset_tokens (
  id         bigint generated always as identity primary key,
  company_id bigint not null default current_setting('app.company_id', true)::bigint references companies(id) on delete cascade,
  user_id    bigint not null references users(id) on delete cascade,
  token_hash text not null unique,
  expires_at timestamptz not null,
  used_at    timestamptz,
  created_at timestamptz not null default now()
);
create index password_reset_tokens_company on password_reset_tokens (company_id);
create index password_reset_tokens_user on password_reset_tokens (user_id);

alter table password_reset_tokens enable row level security;
create policy tenant_isolation on password_reset_tokens
  using (company_id = current_setting('app.company_id', true)::bigint)
  with check (company_id = current_setting('app.company_id', true)::bigint);

-- El rol del app necesita CRUD sobre la tabla nueva (los grants de 0024 fueron sobre las tablas
-- existentes en ese momento). Identity GENERATED ALWAYS no requiere permiso de secuencia.
grant select, insert, update, delete on password_reset_tokens to gatobobah_app;

-- +goose Down
drop table if exists password_reset_tokens;
