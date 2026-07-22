-- Tokens de recuperación de contraseña (ver migración 0025). Todo bajo contexto de tenant.

-- name: CreatePasswordResetToken :one
insert into password_reset_tokens (user_id, token_hash, expires_at)
values ($1, $2, $3)
returning *;

-- name: GetPasswordResetToken :one
select * from password_reset_tokens where token_hash = $1;

-- name: MarkPasswordResetTokenUsed :exec
update password_reset_tokens set used_at = now() where id = $1;

-- name: InvalidateUserResetTokens :exec
-- Al fijar la contraseña, invalida cualquier otro token de reset pendiente del usuario.
update password_reset_tokens set used_at = now() where user_id = $1 and used_at is null;
