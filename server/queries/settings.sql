-- name: GetBusinessSettings :one
-- Una fila por empresa (la PK pasó a company_id en 0023); RLS ya la restringe al tenant actual,
-- así que basta con leer su única fila. (Antes: where id = true, con la columna id que 0023 quitó
-- → 42703 en runtime; sqlc no valida columnas del WHERE, por eso no lo atrapaba make sqlc.)
--
-- NO selecciona logo_bytes a propósito: esta query corre en el camino del cobro (el POS la pide
-- para el costo de envío) y no tiene por qué mover 256 KB de imagen en cada pedido. El binario
-- se pide aparte con GetTicketLogo.
select delivery_fee,
       business_name,
       address,
       phone,
       header_note,
       footer_note,
       auto_print_on_close,
       (logo_bytes is not null)::boolean as has_logo,
       logo_updated_at,
       updated_at,
       updated_by
from business_settings
limit 1;

-- name: UpdateDeliveryFee :one
-- Sin WHERE: RLS acota el UPDATE a la fila de la empresa actual (hay exactamente una).
update business_settings
set delivery_fee = $1, updated_at = now(), updated_by = $2
returning delivery_fee, updated_at, updated_by;

-- name: UpdateBusinessInfo :exec
-- La identidad del ticket y el interruptor de impresión automática. Los strings vacíos se guardan
-- como NULL para que "sin dato" tenga una sola representación en la base.
-- Los ::text no son decorativos: sin ellos sqlc no infiere el tipo del argumento de nullif y
-- genera interface{}, que tira al piso el chequeo de tipos entre Go y la query.
update business_settings
set business_name       = sqlc.arg(business_name),
    address             = nullif(sqlc.arg(address)::text, ''),
    phone               = nullif(sqlc.arg(phone)::text, ''),
    header_note         = nullif(sqlc.arg(header_note)::text, ''),
    footer_note         = nullif(sqlc.arg(footer_note)::text, ''),
    auto_print_on_close = sqlc.arg(auto_print_on_close),
    updated_at          = now(),
    updated_by          = sqlc.arg(updated_by);

-- name: GetTicketLogo :one
-- La única query que trae el binario. Se sirve por su propio endpoint, con su propio caché.
select logo_bytes, logo_mime, logo_updated_at
from business_settings
limit 1;

-- name: SetTicketLogo :exec
update business_settings
set logo_bytes = $1, logo_mime = $2, logo_updated_at = now(), updated_at = now(), updated_by = $3;

-- name: ClearTicketLogo :exec
-- Los tres a NULL juntos: el check business_settings_logo_pair no admite bytes sin mime.
update business_settings
set logo_bytes = null, logo_mime = null, logo_updated_at = null, updated_at = now(), updated_by = $1;
