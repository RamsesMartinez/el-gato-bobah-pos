-- name: CountUsers :one
select count(*) from users;

-- name: GetUserByID :one
select id, name, username, role, pin_hash, password_hash, is_active, created_at, updated_at
from users where id = $1;

-- name: GetUserByUsername :one
select id, name, username, role, pin_hash, password_hash, is_active, created_at, updated_at
from users where username = $1 and is_active;

-- name: ListActiveUsers :many
select id, name, username, role, pin_hash, password_hash, is_active, created_at, updated_at
from users where is_active order by name;

-- name: CreateUser :one
insert into users (name, username, role, pin_hash, password_hash)
values ($1, $2, $3, $4, $5)
returning id, name, username, role, pin_hash, password_hash, is_active, created_at, updated_at;

-- name: UpdateUser :one
update users
set name = $2, role = $3, is_active = $4, updated_at = now()
where id = $1
returning id, name, username, role, pin_hash, password_hash, is_active, created_at, updated_at;

-- name: SetUserPin :exec
update users set pin_hash = $2, updated_at = now() where id = $1;

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

-- name: RevokeUserRefreshTokens :exec
update refresh_tokens set revoked_at = now() where user_id = $1 and revoked_at is null;
