-- name: FolioSchemeDelNegocio :one
-- Con qué se nombran los pedidos. RLS acota a la empresa actual, que tiene exactamente una fila.
--
-- Un negocio SIN fila de ajustes no es un error: se cae al default del dominio, nunca a la otra
-- lista. Quien llama trata pgx.ErrNoRows así.
select folio_scheme from business_settings limit 1;

-- name: FolioNamesConsumidos :many
-- Los nombres que ya salieron en la vuelta en curso de este esquema.
--
-- company_id no se nombra: lo pone RLS en el where y el DEFAULT en el insert, como en el resto del
-- repo. La PK (company_id, scheme, name) cubre esta lectura entera.
select name from folio_consumido where scheme = $1;

-- name: MarcarFolioConsumido :exec
-- Saca el nombre de la bolsa. `do nothing` porque dos cajas pueden sortear el mismo nombre a la vez:
-- el índice único del día es el que decide quién se lo queda, y aquí anotar dos veces es inofensivo.
insert into folio_consumido (scheme, name) values ($1, $2)
on conflict do nothing;

-- name: VaciarBolsaDeFolios :exec
-- Se agotó la vuelta: todos los nombres del esquema salieron al menos una vez y empieza otra.
--
-- Borra SOLO el esquema que se agotó. La bolsa del otro sigue donde iba: un negocio que prueba
-- animales una semana y vuelve a razas no pierde su vuelta a medias.
delete from folio_consumido where scheme = $1;
