-- Tokens de acceso personal del servidor MCP (spec 072). company_id no se nombra: RLS lo pone.

-- name: InsertAPIToken :one
insert into api_tokens (user_id, name, secret_hint, secret_hash, scopes, expires_at)
values (@user_id, @name, @secret_hint, @secret_hash, @scopes::text[], @expires_at)
returning id, created_at;

-- name: ListAPITokens :many
-- Los de la empresa, vigentes y los que dejaron de estarlo hace menos de 90 días. Activos primero.
select t.id, t.name, t.secret_hint, t.user_id, u.name as owner_name, t.scopes::text[] as scopes,
       t.created_at, t.expires_at, t.last_used_at, t.revoked_at
  from api_tokens t
  join users u on u.id = t.user_id
 where coalesce(t.revoked_at, t.expires_at) > now() - interval '90 days'
 order by (t.revoked_at is null and t.expires_at > now()) desc, t.created_at desc, t.id desc;

-- name: GetAPITokenForAuth :one
-- Lo que hace falta para autenticar: la huella, la vigencia y si la persona y su rol siguen activos.
select t.id, t.company_id, t.user_id, t.name, t.secret_hash, t.scopes::text[] as scopes, t.expires_at, t.revoked_at,
       u.name as user_name, u.is_active as user_active, r.active as role_active,
       coalesce(s.code, '')::text as role_code
  from api_tokens t
  join users u on u.id = t.user_id
  join roles r on r.id = u.role_id
  left join system_roles s on s.id = r.system_role_id
 where t.id = @id;

-- name: TouchAPIToken :exec
-- Como mucho una escritura por minuto: la condición vive aquí, no en una lectura previa en Go, para
-- que dos llamadas a la vez no peleen por el renglón.
update api_tokens set last_used_at = now()
 where id = @id and (last_used_at is null or last_used_at < now() - interval '1 minute');

-- name: RevokeAPIToken :one
-- Idempotente: revocar uno ya revocado no pisa a quien lo revocó. Devuelve si este update lo revocó.
with hit as (
  update api_tokens t set revoked_at = now(), revoked_by = @revoked_by
   where t.id = @token_id and t.revoked_at is null
  returning t.id
)
select (select count(*) from hit)::int as revoked,
       (select count(*) from api_tokens a where a.id = @token_id)::int as found;

-- name: InsertIdempotencyKey :one
-- Toma la clave si es nueva. Sin fila de vuelta, la clave ya existía: se lee con GetIdempotencyKey.
insert into mcp_idempotency_keys (token_id, idem_key, tool, args_hash, status)
values (@token_id, @idem_key, @tool, @args_hash, 'pending')
on conflict do nothing
returning idem_key;

-- name: GetIdempotencyKey :one
select status, args_hash, result from mcp_idempotency_keys where token_id = @token_id and idem_key = @idem_key;

-- name: FinishIdempotencyKey :exec
update mcp_idempotency_keys set status = 'done', result = @result
 where token_id = @token_id and idem_key = @idem_key and status = 'pending';

-- name: MarkIdempotencyKeyUnknown :exec
update mcp_idempotency_keys set status = 'unknown'
 where token_id = @token_id and idem_key = @idem_key and status = 'pending';

-- name: DeleteIdempotencyKey :exec
delete from mcp_idempotency_keys where token_id = @token_id and idem_key = @idem_key and status = 'pending';

-- name: PurgeOldIdempotencyKeys :exec
delete from mcp_idempotency_keys where token_id = @token_id and created_at < now() - interval '7 days';
