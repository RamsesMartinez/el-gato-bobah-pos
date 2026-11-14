-- Destinatarios de los avisos de inventario por correo (spec 084, historia 7). Por omisión, quien
-- puede configurar el almacén y tiene correo; las filas de notification_recipients quitan a alguien de
-- la omisión o agregan un correo extra, por tipo de aviso. RLS pone la empresa.

-- name: UsersWithPermission :many
-- Las personas activas cuyo rol (activo) tiene el permiso: lo del sugerido menos lo quitado, más lo
-- agregado, igual que UserEffectivePermissions. Su correo es el de recuperación ('' si no tiene).
select u.id, u.name::text as name, coalesce(r.name, s.name)::text as role_name, coalesce(u.recovery_email::text, '')::text as email
  from users u
  join roles r on r.id = u.role_id and r.active
  left join system_roles s on s.id = r.system_role_id
  join permissions p on p.code = sqlc.arg(code)::text
 where u.is_active
   and ((exists (select 1 from system_role_permissions sp where sp.system_role_id = r.system_role_id and sp.permission_id = p.id)
         and not exists (select 1 from role_permission_overrides o
                          where o.company_id = r.company_id and o.role_id = r.id and o.permission_id = p.id and not o.granted))
        or exists (select 1 from role_permission_overrides o
                    where o.company_id = r.company_id and o.role_id = r.id and o.permission_id = p.id and o.granted))
 order by u.name, u.id;

-- name: ListNotificationRecipients :many
select user_id, coalesce(email::text, '')::text as email, kind, enabled from notification_recipients order by id;

-- name: ClearNotificationRecipients :exec
delete from notification_recipients;

-- name: InsertNotificationRecipient :exec
insert into notification_recipients (user_id, email, kind, enabled, updated_by)
values (sqlc.narg(user_id), sqlc.narg(email), sqlc.arg(kind), sqlc.arg(enabled), sqlc.arg(updated_by));

-- name: NotificationsMorningAt :one
select to_char(notifications_morning_at, 'HH24:MI')::text from business_settings limit 1;

-- name: SetNotificationsMorningAt :exec
update business_settings set notifications_morning_at = (sqlc.arg(at)::text)::time;

-- name: LockNotificationRecipients :exec
-- Dos personas guardando destinatarios a la vez: la segunda espera y reemplaza con su lista completa,
-- en vez de mezclarse filas de las dos.
select pg_advisory_xact_lock(28084, coalesce(nullif(current_setting('app.company_id', true), '')::int, 0));
