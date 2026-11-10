-- +goose Up
-- La puerta para agentes (spec 072): tokens de acceso personal con los que un cliente MCP opera el
-- negocio con la identidad de una persona real, y las claves de idempotencia de sus escrituras.
--
-- Solo agrega. La versión anterior de la API no lee estas tablas ni conoce el permiso 37: su
-- effectivePermissions descarta lo que el binario no conoce (KnownPermission).

create table api_tokens (
  id           bigint generated always as identity primary key,
  company_id   bigint not null default nullif(current_setting('app.company_id', true), '')::bigint
               references companies (id) on delete cascade,
  user_id      bigint not null,
  name         text not null,
  -- Lo único del secreto que se guarda en claro: sirve para reconocer un token pegado en algún
  -- lado, no para usarlo. El prefijo visible se compone en Go con la empresa y el id.
  secret_hint  text not null,
  -- sha256 del secreto. Sin unique: el secreto trae 256 bits aleatorios y la búsqueda es por id.
  secret_hash  bytea not null,
  scopes       text[] not null,
  created_at   timestamptz not null default now(),
  expires_at   timestamptz not null,
  last_used_at timestamptz,
  revoked_at   timestamptz,
  revoked_by   bigint,
  constraint api_tokens_company_id_key unique (company_id, id),
  constraint api_tokens_user_fkey foreign key (company_id, user_id) references users (company_id, id),
  constraint api_tokens_revoked_by_fkey foreign key (company_id, revoked_by) references users (company_id, id),
  constraint api_tokens_name_length check (char_length(btrim(name)) between 1 and 60),
  constraint api_tokens_secret_hint check (secret_hint ~ '^[0-9a-f]{4}$'),
  constraint api_tokens_secret_hash_length check (octet_length(secret_hash) = 32),
  -- Ampliar la lista para un alcance nuevo es retrocompatible: la versión anterior no lo escribe.
  constraint api_tokens_scopes check (
    scopes <@ array['read', 'write:expenses', 'write:cash', 'write:inventory', 'write:catalog']::text[]
    and 'read' = any (scopes)
    and array_position(scopes, null) is null
    and cardinality(scopes) between 1 and 5),
  -- Un token sin fecha es una llave que nadie recuerda; 181 y no 180 por el redondeo del reloj.
  constraint api_tokens_expiry check (expires_at > created_at and expires_at <= created_at + interval '181 days'),
  constraint api_tokens_revoked_pair check ((revoked_at is null) = (revoked_by is null))
);
create index api_tokens_list_idx on api_tokens (company_id, created_at desc);
create index api_tokens_user_idx on api_tokens (company_id, user_id);

-- Revocado es terminal en la base y no solo en Go: un update descuidado no resucita una llave que
-- alguien revocó por filtración.
-- +goose StatementBegin
create function api_tokens_revocation_is_final() returns trigger
language plpgsql as $$
begin
  if old.revoked_at is not null and new.revoked_at is distinct from old.revoked_at then
    raise exception 'un token revocado no se reactiva ni cambia de fecha de revocación'
      using errcode = 'check_violation';
  end if;
  return new;
end $$;
-- +goose StatementEnd
create trigger api_tokens_revocation_is_final before update on api_tokens
  for each row execute function api_tokens_revocation_is_final();

alter table api_tokens enable row level security;
create policy tenant_isolation on api_tokens
  using (company_id = nullif(current_setting('app.company_id', true), '')::bigint)
  with check (company_id = nullif(current_setting('app.company_id', true), '')::bigint);
-- Sin delete y sin update del nombre ni de los alcances: un token cambia creando otro.
grant select, insert on api_tokens to gatobobah_app;
grant update (last_used_at, revoked_at, revoked_by) on api_tokens to gatobobah_app;

create table mcp_idempotency_keys (
  company_id bigint not null default nullif(current_setting('app.company_id', true), '')::bigint
             references companies (id) on delete cascade,
  token_id   bigint not null,
  idem_key   text not null,
  tool       text not null,
  args_hash  bytea not null,
  status     text not null,
  result     jsonb,
  created_at timestamptz not null default now(),
  primary key (company_id, token_id, idem_key),
  constraint mcp_idempotency_keys_token_fkey
    foreign key (company_id, token_id) references api_tokens (company_id, id) on delete cascade,
  constraint mcp_idempotency_keys_key check (idem_key ~ '^[A-Za-z0-9_.:-]{8,100}$'),
  constraint mcp_idempotency_keys_tool check (char_length(tool) between 1 and 80),
  constraint mcp_idempotency_keys_hash check (octet_length(args_hash) = 32),
  constraint mcp_idempotency_keys_status check (status in ('pending', 'done', 'unknown')),
  constraint mcp_idempotency_keys_result check ((status = 'done') = (result is not null)),
  constraint mcp_idempotency_keys_result_size check (result is null or pg_column_size(result) < 262144)
);
create index mcp_idempotency_keys_purge_idx on mcp_idempotency_keys (company_id, token_id, created_at);
alter table mcp_idempotency_keys enable row level security;
create policy tenant_isolation on mcp_idempotency_keys
  using (company_id = nullif(current_setting('app.company_id', true), '')::bigint)
  with check (company_id = nullif(current_setting('app.company_id', true), '')::bigint);
grant select, insert, delete on mcp_idempotency_keys to gatobobah_app;
grant update (status, result) on mcp_idempotency_keys to gatobobah_app;

-- El permiso nuevo, solo al sugerido admin; llega a toda empresa por herencia.
insert into permissions (id, code, area) values (37, 'integrations.manage', 'business');
insert into system_role_permissions (system_role_id, permission_id) values (1, 37);

-- +goose Down
-- +goose StatementBegin
do $$
begin
  if exists (select 1 from api_tokens)
     or exists (select 1 from mcp_idempotency_keys)
     or exists (select 1 from role_permission_overrides where permission_id = 37) then
    raise exception '0111: hay tokens o ajustes del permiso integrations.manage; bajar los perdería';
  end if;
end $$;
-- +goose StatementEnd
delete from system_role_permissions where permission_id = 37;
delete from permissions where id = 37;
drop table mcp_idempotency_keys;
drop table api_tokens;
drop function api_tokens_revocation_is_final();
