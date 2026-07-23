-- name: GetBusinessSettings :one
-- Una fila por empresa (la PK pasó a company_id en 0023); RLS ya la restringe al tenant actual,
-- así que basta con leer su única fila. (Antes: where id = true, con la columna id que 0023 quitó
-- → 42703 en runtime; sqlc no valida columnas del WHERE, por eso no lo atrapaba make sqlc.)
select delivery_fee, updated_at, updated_by from business_settings limit 1;

-- name: UpdateDeliveryFee :one
-- Sin WHERE: RLS acota el UPDATE a la fila de la empresa actual (hay exactamente una).
update business_settings
set delivery_fee = $1, updated_at = now(), updated_by = $2
returning delivery_fee, updated_at, updated_by;
