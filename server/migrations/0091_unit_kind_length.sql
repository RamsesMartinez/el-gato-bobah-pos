-- +goose Up
-- Longitud como cuarto tipo de unidad (spec 038, decisión del dueño del 14-oct).
--
-- Va sola en su migración: Postgres no deja USAR un valor de enum en la misma transacción que lo
-- agrega, y goose corre cada migración en una transacción (la misma regla de la 0018). Las unidades
-- `cm` y `m` se crean en la 0092.
alter type unit_kind add value if not exists 'longitud';

-- +goose Down
-- Postgres no sabe quitar un valor de un enum. Se deja: sin unidades que lo usen (la 0092 las borra
-- en su Down) no estorba, y un Up posterior lo encuentra ya puesto gracias al `if not exists`.
select 1;
