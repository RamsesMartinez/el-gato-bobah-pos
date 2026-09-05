-- +goose Up
-- EL REUSO REVOCA LA CADENA COMPROMETIDA, NO TODAS LAS SESIONES DE LA PERSONA.
--
-- La constitución dice "si el veredicto es RefreshReused, revoca toda la FAMILIA". El código decía
-- lo mismo en un comentario y hacía otra cosa: `RevokeUserRefreshTokens`, que revoca por `user_id`.
-- No había con qué hacerlo bien — nada en el esquema decía qué credenciales descienden de qué
-- login.
--
-- En este negocio dos estaciones comparten cuenta, así que la diferencia es concreta: un robo
-- detectado en la tableta de la barra tumbaba también la de la caja, con el cliente enfrente y sin
-- que nadie entendiera por qué. Y ya costó caro por otra vía — el ping-pong de sesiones que
-- documenta `dos_estaciones_no_se_tumban_test.go` necesitaba exactamente esta pieza.
--
-- La familia es la cadena que nace en UN login y se propaga en cada rotación. Revocarla corta al
-- ladrón sin tocar la tableta del compañero.
alter table refresh_tokens add column family_id uuid;

-- Cada credencial que ya existe se queda en su PROPIA familia.
--
-- Es la opción conservadora a propósito: agruparlas por usuario reproduciría el defecto que esto
-- viene a cerrar —un reuso volvería a tumbarlas todas— y no hay forma de reconstruir el linaje
-- real, porque nunca se guardó. Con una familia por credencial, lo peor que puede pasar es revocar
-- de menos, que es el lado correcto en el que equivocarse mientras las viejas se renuevan solas.
update refresh_tokens set family_id = gen_random_uuid() where family_id is null;

-- Arranca por company_id como todo índice de este esquema: RLS le pega ese predicado a cada
-- consulta del rol de la app, y un índice que empiece por la familia se queda descartando filas de
-- otras empresas dentro del scan.
--
-- Parcial: nada nace ya sin familia, pero la columna es nullable para que el binario ANTERIOR siga
-- pudiendo insertar si hay que revertir el deploy sin revertir el esquema.
create index refresh_tokens_family on refresh_tokens (company_id, family_id)
  where family_id is not null;

-- +goose Down
drop index if exists refresh_tokens_family;
alter table refresh_tokens drop column if exists family_id;
