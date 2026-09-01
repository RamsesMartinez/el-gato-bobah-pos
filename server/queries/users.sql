-- name: CountUsers :one
select count(*) from users;

-- name: GetUserByID :one
-- RLS acota a la empresa del tenant activo (GUC app.company_id); no hace falta filtrar por company_id aquí.
select * from users where id = $1;

-- name: GetUserByUsername :one
select * from users where username = $1 and is_active;

-- name: ListActiveUsers :many
select * from users where is_active order by name;

-- name: CreateUser :one
-- company_id lo auto-sella el default (current_setting) desde el GUC del tenant; RLS lo exige.
insert into users (name, username, role, pin_hash, pin_lookup, password_hash, recovery_email, must_change_password)
values ($1, $2, $3, $4, sqlc.narg(pin_lookup), $5, $6, $7)
returning *;

-- name: UpdateUser :one
update users
set name = $2, role = $3, is_active = $4, recovery_email = $5, updated_at = now()
where id = $1
returning *;

-- name: SetUserPin :exec
update users set pin_hash = $2, pin_lookup = sqlc.narg(pin_lookup), updated_at = now() where id = $1;

-- name: SetUserPassword :exec
-- must_change_password lo fija quien llama (true tras reset por admin, false en cambio propio).
update users set password_hash = $2, must_change_password = $3, updated_at = now() where id = $1;

-- name: SetUserRecoveryEmail :exec
update users set recovery_email = $2, updated_at = now() where id = $1;

-- name: SetUserSecretsByUsername :execrows
update users set password_hash = $2, pin_hash = $3, is_active = true, updated_at = now()
where username = $1;

-- name: GetUserPreference :one
select value from user_preferences where user_id = $1 and key = $2;

-- name: SetUserPreference :exec
insert into user_preferences (user_id, key, value, updated_at)
values ($1, $2, $3, now())
on conflict (user_id, key) do update set value = excluded.value, updated_at = now();

-- name: CreateRefreshToken :one
insert into refresh_tokens (user_id, token_hash, expires_at)
values ($1, $2, $3)
returning id, user_id, token_hash, expires_at, revoked_at, created_at;

-- name: GetRefreshToken :one
select id, user_id, token_hash, expires_at, revoked_at, created_at
from refresh_tokens where token_hash = $1;

-- name: RevokeRefreshToken :exec
update refresh_tokens set revoked_at = now() where token_hash = $1;

-- name: RevokeRefreshTokenIfActive :execrows
-- Rotación atómica: revoca solo si sigue activo. RowsAffected=0 => otro request ya lo
-- revocó (rotación concurrente o reuso). Cierra el TOCTOU del rotate read-then-revoke.
update refresh_tokens set revoked_at = now() where token_hash = $1 and revoked_at is null;

-- name: RevokeUserRefreshTokens :exec
update refresh_tokens set revoked_at = now() where user_id = $1 and revoked_at is null;

-- name: UnlockCandidates :many
-- Quiénes pueden desbloquear una estación con su PIN.
--
-- SOLO id y nombre: esta lista se pinta en una tableta a la vista del público, así que el correo,
-- el rol y el teléfono no tienen por qué salir del servidor.
--
-- Solo activos y CON PIN: quien no lo tiene configurado no entraría aunque lo tocara, y ofrecerlo
-- sería mandarlo a un callejón. Esa persona entra con usuario y contraseña, que sigue funcionando.
select id, name from users
where is_active and pin_hash is not null
order by name;

-- name: LiveRefreshExpiry :one
-- Cuándo vence la sesión que ESTA estación viene presentando.
--
-- Se busca por el HASH del token, no por user_id. Buscar por persona tomaba el vencimiento más
-- lejano de cualquiera de sus tabletas, así que una estación heredaba el reloj de otra: entrar
-- fresco en la segunda le regalaba horas a la primera, con un PIN y de forma repetible.
select expires_at from refresh_tokens
where token_hash = $1 and revoked_at is null and expires_at > $2;

-- name: RevokeRefreshTokenByHash :exec
-- Revoca UNA sesión: la que la estación venía presentando antes del relevo.
--
-- Solo esa. Revocar todas las de la persona tumbaba sus otras tabletas: entregar la estación 1
-- dejaba al compañero de la estación 2 con "terminó el turno" a media venta. El modo de fallo del
-- resto de la feature es dejar trabajar.
update refresh_tokens set revoked_at = now()
where token_hash = $1 and revoked_at is null;


-- name: ClearAllPins :exec
-- Borra los PINs de TODAS las personas de la empresa, activas o no.
--
-- Lo usa el encendido del modo de solo-PIN: los PINs de antes son de 4 dígitos y sin garantía de ser
-- distintos, y de lo guardado no se puede saber ni una cosa ni la otra. Obligar a recapturarlos es
-- el único momento en que el PIN está en claro y se puede validar largo y unicidad.
--
-- También a quien está dado de baja: decía `where is_active` y a esa persona le quedaba su PIN de
-- cuatro dígitos intacto, así que reactivarla metía en un negocio de seis dígitos un PIN de cuatro
-- que abre la caja — 10,000 combinaciones en vez de un millón, y sin nada que lo delatara.
--
-- Nadie queda encerrado: quien no tiene PIN entra con usuario y contraseña, que sigue funcionando.
update users set pin_hash = null, pin_lookup = null, updated_at = now()
where pin_hash is not null or pin_lookup is not null;

-- name: UserByPinLookup :one
-- De quién es este PIN. Solo tiene sentido con el modo de solo-PIN, donde el PIN identifica.
select id, name from users
where is_active and pin_lookup = $1;
