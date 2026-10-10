-- +goose Up

-- LAS CREDENCIALES DE LA APLICACIÓN DE CADA PLATAFORMA SE CAPTURAN EN PANTALLA, POR EMPRESA, Y
-- NINGÚN SECRETO DE UN TERCERO QUEDA EN TEXTO PLANO EN LA BASE.
--
-- Hasta aquí el client_id y el client_secret de Uber vivían en el entorno, uno para todo el
-- sistema: la empresa B habría aceptado pedidos con la aplicación de la empresa A. Es la exención
-- que dejó escrita el plan de la 021 («recibir sí; decidir no») y que esta migración cierra.
--
-- CIFRADO CON CLOUD KMS, Y ESO CORRIGE A LA 0073. Aquélla dejó la llave de firma sin cifrar con el
-- argumento de que «cifrar con una llave del entorno mueve el secreto, no lo protege: quien lee la
-- base desde la aplicación tiene también el entorno». Era cierto para quien entra por la
-- aplicación y falso para lo que SALE del servidor: cada `pg_dump` se copia a la máquina de quien
-- programa y se restaura completo en local. Con KMS la llave no está ni en el entorno ni en el
-- respaldo; un dump filtrado trae bytes que solo la cuenta de servicio de su ambiente descifra.
--
-- Las columnas son `bytea` opacos: el formato (qué respaldo cifró, nonce, AAD) es del paquete
-- `internal/secrets`, no de la base. La AAD ata cada valor a su empresa y su plataforma, así que un
-- valor copiado a la fila de otra empresa no descifra — una segunda barrera además de RLS.
--
-- Ningún `check` de longitud sobre el secreto: su largo real ya no se ve. La validación vive en
-- `domain.NormalizeAppCredentials` / `NormalizarLlaveDeFirma`, antes de cifrar. Los `check`
-- de aquí solo acotan el tamaño del cifrado, que sí es lo que la base guarda.

set local lock_timeout = '3s';

-- ---------------------------------------------------------------------------------------------
-- 1. LAS CREDENCIALES DE LA APLICACIÓN
--
-- POR EMPRESA Y PLATAFORMA, NO POR TIENDA: la aplicación se registra una vez en el tablero de la
-- plataforma y todas las sucursales de la empresa cuelgan de ella. Mismo criterio que la llave de
-- firma (0073).
--
-- `client_id` va en claro: no es secreto —viaja en cada petición de token y el tablero lo muestra
-- sin ocultar— y la pantalla lo necesita para que quien opera reconozca QUÉ app capturó.
-- ---------------------------------------------------------------------------------------------
create table platform_credentials (
  id                    bigint generated always as identity primary key,
  delivery_platform_id  smallint not null,
  client_id             text not null,
  client_secret_encrypted bytea not null,
  updated_at            timestamptz not null default now(),
  -- Quién la capturó. Con credenciales de un tercero, «¿quién puso esto?» es la primera pregunta
  -- cuando algo deja de funcionar.
  updated_by            bigint not null,
  company_id            bigint not null default nullif(current_setting('app.company_id', true), '')::bigint
                        references companies(id) on delete cascade,

  constraint platform_credentials_one_per_app unique (company_id, delivery_platform_id),
  constraint platform_credentials_client_id_length check (char_length(client_id) between 8 and 256),
  constraint platform_credentials_ciphertext_size
    check (octet_length(client_secret_encrypted) between 16 and 4096),

  -- FK COMPUESTAS: los chequeos de integridad referencial SALTAN RLS, así que una FK simple dejaría
  -- apuntar a la plataforma o al usuario de otra empresa. Mismo patrón que la 0073.
  -- Sin `on delete` en la plataforma: borrarla del catálogo no debe llevarse en silencio una
  -- credencial que alguien sacó a mano del tablero de un tercero.
  constraint platform_credentials_platform_of_company
    foreign key (delivery_platform_id, company_id) references delivery_platforms (id, company_id),
  constraint platform_credentials_updated_by_of_company
    foreign key (company_id, updated_by) references users (company_id, id)
);

-- `updated_at` por trigger y no por la consulta: la columna existe para responder «¿cuándo cambió
-- la credencial de un tercero?», y un upsert que olvide listarla la dejaría congelada en la fecha de
-- la primera captura sin que nada falle. Señalado por la revisión de `db-architect`.
create trigger trg_platform_credentials_updated before update on platform_credentials
  for each row execute function set_updated_at();

-- ---------------------------------------------------------------------------------------------
-- 2. LA LLAVE DE FIRMA DEJA DE IR EN TEXTO PLANO
--
-- SE BORRAN LAS QUE HUBIERA, a propósito. Esta tabla nació en la 0073, que al escribir esto no
-- está aplicada ni en pruebas ni en producción, y las dos viajan en el MISMO despliegue (la 021): las únicas filas posibles son llaves de prueba en
-- la máquina de quien programa. Cifrarlas aquí no se puede —la migración no tiene la llave de
-- KMS— y dejarlas en claro es justo lo que esta migración viene a quitar. Se recapturan.
-- ---------------------------------------------------------------------------------------------
-- +goose StatementBegin
do $$
declare n int;
begin
  select count(*) into n from platform_webhook_keys;
  if n > 0 then
    raise notice 'se borran % llaves de firma en texto plano: hay que recapturarlas en Plataformas', n;
  end if;
end $$;
-- +goose StatementEnd
delete from platform_webhook_keys;

alter table platform_webhook_keys
  drop constraint platform_webhook_keys_primaria_seria,
  drop constraint platform_webhook_keys_secundaria_seria,
  -- «Dos llaves iguales» ya no se puede comparar aquí: el cifrado lleva un nonce aleatorio y dos
  -- cifrados del mismo valor salen distintos. Lo compara el servicio, en claro, antes de guardar.
  drop constraint platform_webhook_keys_distintas,
  drop column key_primary,
  drop column key_secondary,
  add column key_primary_encrypted   bytea not null,
  add column key_secondary_encrypted bytea,
  add constraint platform_webhook_keys_primary_size
    check (octet_length(key_primary_encrypted) between 16 and 4096),
  add constraint platform_webhook_keys_secondary_size
    check (key_secondary_encrypted is null or octet_length(key_secondary_encrypted) between 16 and 4096);

-- ---------------------------------------------------------------------------------------------
-- 3. RLS Y GRANTS de la tabla nueva. El grant NO se hereda (0024): sin él, 42501 en el primer
-- request. La forma del predicado es la de la 0074, con `nullif`: una conexión reciclada trae la
-- cadena vacía, y `''::bigint` revienta.
--
-- `gatobobah_platform` y `gatobobah_webhook` NO reciben nada: la consola no tiene por qué ver
-- credenciales de un restaurante, y la vista del webhook no las necesita.
-- ---------------------------------------------------------------------------------------------
alter table platform_credentials enable row level security;
create policy tenant_isolation on platform_credentials
  using      (company_id = nullif(current_setting('app.company_id', true), '')::bigint)
  with check (company_id = nullif(current_setting('app.company_id', true), '')::bigint);
grant select, insert, update, delete on platform_credentials to gatobobah_app;

-- +goose Down

set local lock_timeout = '3s';

drop table if exists platform_credentials;

-- De regreso tampoco hay cómo descifrar sin KMS: se borran y se recapturan, igual que al subir.
delete from platform_webhook_keys;
alter table platform_webhook_keys
  drop constraint platform_webhook_keys_primary_size,
  drop constraint platform_webhook_keys_secondary_size,
  drop column key_primary_encrypted,
  drop column key_secondary_encrypted,
  add column key_primary   text not null,
  add column key_secondary text,
  add constraint platform_webhook_keys_primaria_seria check (char_length(key_primary) between 16 and 512),
  add constraint platform_webhook_keys_secundaria_seria
    check (key_secondary is null or char_length(key_secondary) between 16 and 512),
  add constraint platform_webhook_keys_distintas check (key_secondary is null or key_secondary <> key_primary);
