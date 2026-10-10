-- ---------------------------------------------------------------------------------------------
-- Credenciales de la aplicación de cada plataforma (0075). Todas corren bajo RLS: una empresa solo
-- ve y escribe las suyas. El secreto viaja CIFRADO en las dos direcciones; esta capa nunca lo ve en
-- claro.
-- ---------------------------------------------------------------------------------------------

-- name: UpsertPlatformCredential :exec
-- Capturar o reemplazar. `updated_at` lo pone el trigger.
insert into platform_credentials (delivery_platform_id, client_id, client_secret_encrypted, updated_by)
values ($1, $2, $3, $4)
on conflict (company_id, delivery_platform_id) do update
   set client_id = excluded.client_id,
       client_secret_encrypted = excluded.client_secret_encrypted,
       updated_by = excluded.updated_by;

-- name: GetPlatformCredentialState :one
-- Para la pantalla. Trae el cifrado solo para saber si ESTE ambiente puede leerlo; el handler nunca
-- lo recibe.
select pc.company_id, pc.client_id, pc.client_secret_encrypted, pc.updated_at, u.name as updated_by_name
  from platform_credentials pc
  join users u on u.id = pc.updated_by
 where pc.delivery_platform_id = $1;

-- name: GetPlatformCredentialByName :one
-- La credencial con la que se habla con la plataforma, por el nombre con el que el catálogo la
-- conoce ("Uber Eats"). Trae empresa y plataforma de la fila: son la AAD del cifrado.
select pc.company_id, pc.delivery_platform_id, pc.client_id, pc.client_secret_encrypted
  from platform_credentials pc
  join delivery_platforms p on p.id = pc.delivery_platform_id
 where p.name = $1;
