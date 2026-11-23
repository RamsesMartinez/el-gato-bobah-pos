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

-- name: PurgeOldIdempotencyKeys :exec
delete from mcp_idempotency_keys where token_id = @token_id and created_at < now() - interval '7 days';

-- name: FailIdempotencyKey :exec
-- La escritura falló sin confirmar nada (#335): la llave guarda el error y queda libre para reintentar.
update mcp_idempotency_keys set status = 'failed', result = @result
 where token_id = @token_id and idem_key = @idem_key and status = 'pending';

-- name: RetakeFailedIdempotencyKey :execrows
-- Retoma una llave cuyo intento falló sin aplicar nada, con la huella de ESTE intento (los argumentos
-- pudieron corregirse). Sin fila afectada, otro reintento la tomó primero.
update mcp_idempotency_keys set status = 'pending', result = null, args_hash = @args_hash, tool = @tool
 where token_id = @token_id and idem_key = @idem_key and status = 'failed';

-- name: FindReferences :many
-- Cuáles de los ids que manda una escritura del MCP existen en la empresa de la sesión (#335). RLS pone
-- el filtro por empresa en cada tabla: un id de otra empresa sale igual que uno que no existe. Las
-- llaves foráneas de una sola columna no sirven para esto porque Postgres las revisa saltándose RLS.
select r.kind::text as kind, r.id::bigint as id,
       (coalesce(p.id, i.id, o.id, g.id, c.id, ip.id, sc.id, e.id, ec.id, s.id, pm.id, cr.id, cc.id,
                 wr.id, ord.id, dp.id, w.id, ct.id, icat.id, pl.id) is not null)::boolean as found,
       coalesce(p.is_active, true)::boolean as active,
       coalesce(p.name, '')::text as name
  from (select k.kind, d.id from unnest(@kinds::text[]) with ordinality as k(kind, n)
          join unnest(@ids::bigint[]) with ordinality as d(id, n) using (n)) as r
  left join products p on r.kind = 'product' and p.id = r.id
  left join ingredients i on r.kind = 'ingredient' and i.id = r.id
  left join modifier_options o on r.kind = 'option' and o.id = r.id
  left join modifier_groups g on r.kind = 'group' and g.id = r.id
  left join categories c on r.kind = 'category' and c.id = r.id
  left join ingredient_presentations ip on r.kind = 'presentation' and ip.id = r.id
  left join stock_counts sc on r.kind = 'count' and sc.id = r.id
  left join expenses e on r.kind = 'expense' and e.id = r.id
  left join expense_categories ec on r.kind = 'expense_category' and ec.id = r.id
  left join suppliers s on r.kind = 'supplier' and s.id = r.id
  left join payment_methods pm on r.kind = 'payment_method' and pm.id = r.id
  left join cash_registers cr on r.kind = 'register' and cr.id = r.id
  left join cash_concepts cc on r.kind = 'cash_concept' and cc.id = r.id
  left join waste_reasons wr on r.kind = 'waste_reason' and wr.id = r.id
  left join orders ord on r.kind = 'order' and ord.id = r.id
  left join delivery_platforms dp on r.kind = 'platform' and dp.id = r.id
  left join warehouses w on r.kind = 'warehouse' and w.id = r.id
  left join card_terminals ct on r.kind = 'terminal' and ct.id = r.id
  left join ingredient_categories icat on r.kind = 'ingredient_category' and icat.id = r.id
  left join count_places pl on r.kind = 'place' and pl.id = r.id;
