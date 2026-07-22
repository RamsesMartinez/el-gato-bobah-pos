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
insert into users (name, username, role, pin_hash, password_hash, recovery_email, must_change_password)
values ($1, $2, $3, $4, $5, $6, $7)
returning *;

-- name: UpdateUser :one
update users
set name = $2, role = $3, is_active = $4, recovery_email = $5, updated_at = now()
where id = $1
returning *;

-- name: SetUserPin :exec
update users set pin_hash = $2, updated_at = now() where id = $1;

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
