-- Bitácora de auditoría (spec 033). Solo se inserta y se lee: la tabla es de solo agregar. RLS
-- acota a la empresa; company_id lo pone el default desde la sesión.

-- name: InsertAuditEvent :exec
insert into audit_events (branch_id, actor_kind, actor_user_id, actor_label, action, entity_type, entity_id,
                          order_id, subject_user_id, ip, device, device_key, before, after)
values (sqlc.narg('branch_id'), sqlc.arg('actor_kind'), sqlc.narg('actor_user_id'), sqlc.narg('actor_label'),
        sqlc.arg('action'), sqlc.arg('entity_type'), sqlc.arg('entity_id'), sqlc.narg('order_id'),
        sqlc.narg('subject_user_id'), sqlc.narg('ip'), sqlc.narg('device'), sqlc.narg('device_key'),
        sqlc.narg('before'), sqlc.narg('after'));

-- name: GetOrderBranch :one
-- La sucursal de un evento de pedido es la del pedido, no la del aparato (un aparato sin registro
-- no sabe dónde está).
select branch_id from orders where id = $1;

-- name: ListOrderHistory :many
-- El historial de un pedido, en orden. El nombre del autor se lee aquí y no se copia al evento: la
-- persona es la misma aunque cambie de nombre.
select e.id, e.occurred_at, e.action, e.actor_kind, e.actor_label, u.name as actor_name,
       coalesce(host(e.ip), '')::text as ip, e.device, e.device_key, e.before, e.after, e.branch_id
  from audit_events e
  left join users u on u.id = e.actor_user_id
 where e.order_id = $1
 order by e.occurred_at, e.id;

-- name: GetRegisterSessionBranch :one
-- La sucursal de un evento de caja es la de la caja del turno.
select r.branch_id from register_sessions s join cash_registers r on r.id = s.register_id where s.id = $1;

-- name: GetProductPriceForAudit :one
select price from products where id = $1 for update;

-- name: GetProductPlatformPriceForAudit :many
-- :many y no :one: sin precio capturado no hay fila, y eso es un «antes» válido (vacío).
select price from product_platform_prices where product_id = $1 and platform_id = $2;

-- name: GetOptionPlatformPriceForAudit :many
select price_delta from modifier_option_platform_prices where option_id = $1 and platform_id = $2;

-- La bitácora general (spec 033). Lista, conteo y conteo por acción comparten el MISMO where: si
-- divergen, la lista y sus contadores dejan de cuadrar y quien los lee no sabe cuál miente.

-- name: ListAuditEvents :many
select e.id, e.occurred_at, e.action, e.actor_kind, e.actor_label, u.name as actor_name,
       coalesce(host(e.ip), '')::text as ip, e.device, e.device_key, e.before, e.after, e.order_id
  from audit_events e
  left join users u on u.id = e.actor_user_id
 where e.occurred_at >= sqlc.arg('from_at') and e.occurred_at < sqlc.arg('to_at')
   and (sqlc.narg('actor_user_id')::bigint is null or e.actor_user_id = sqlc.narg('actor_user_id'))
   and (sqlc.narg('actions')::text[] is null or e.action = any(sqlc.narg('actions')::text[]))
   and (sqlc.narg('device_key')::text is null or e.device_key = sqlc.narg('device_key'))
 order by e.occurred_at desc, e.id desc
 limit sqlc.arg('lim') offset sqlc.arg('off');

-- name: CountAuditEvents :one
select count(*) from audit_events e
 where e.occurred_at >= sqlc.arg('from_at') and e.occurred_at < sqlc.arg('to_at')
   and (sqlc.narg('actor_user_id')::bigint is null or e.actor_user_id = sqlc.narg('actor_user_id'))
   and (sqlc.narg('actions')::text[] is null or e.action = any(sqlc.narg('actions')::text[]))
   and (sqlc.narg('device_key')::text is null or e.device_key = sqlc.narg('device_key'));

-- name: CountAuditEventsByAction :many
-- Sin el filtro de acción a propósito: los contadores de grupo no se vacían al tocar uno.
select e.action, count(*) as n from audit_events e
 where e.occurred_at >= sqlc.arg('from_at') and e.occurred_at < sqlc.arg('to_at')
   and (sqlc.narg('actor_user_id')::bigint is null or e.actor_user_id = sqlc.narg('actor_user_id'))
   and (sqlc.narg('device_key')::text is null or e.device_key = sqlc.narg('device_key'))
 group by e.action;

-- name: ListAuditDeviceKeys :many
-- Los aparatos que el filtro «Aparato» ofrece: los de todo el rango, no los de la página. Sin el
-- filtro de aparato a propósito: elegir uno no debe borrar a los demás del selector.
select distinct e.device_key::text as device_key from audit_events e
 where e.occurred_at >= sqlc.arg('from_at') and e.occurred_at < sqlc.arg('to_at')
   and (sqlc.narg('actor_user_id')::bigint is null or e.actor_user_id = sqlc.narg('actor_user_id'))
   and (sqlc.narg('actions')::text[] is null or e.action = any(sqlc.narg('actions')::text[]))
   and e.device_key is not null
 order by 1;

-- name: ListAuditActors :many
-- Las personas por las que se puede filtrar la bitácora, activas o no: quien ya se fue también
-- hizo cosas que se revisan.
select id, name from users order by lower(name);

-- name: GetPlatformRefForUpdate :one
-- El folio de plataforma antes de corregirlo, con candado: es el «antes» del evento.
select platform_order_ref from orders where id = $1 for update;
