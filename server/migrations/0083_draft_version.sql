-- +goose Up
-- La versión de la CUENTA ENTERA, no de un renglón ni de la cabecera: avanza con cualquier cambio de
-- lo que la cuenta contiene. Descartar la exige porque descartar se lleva todo lo de adentro, y sin
-- ella una tableta tiraba lo que otra acababa de agregar sin que ninguna de las dos se enterara.
--
-- `header_version` no sirve para esto: la cabecera se cambia con la versión de la cabecera, y que un
-- producto agregado en otra tableta la moviera rechazaría cambios de cabecera que no chocan con nada.
set local lock_timeout = '3s';

alter table order_drafts add column version int not null default 1;

-- +goose Down
-- Sin la columna las tabletas pierden la versión, nada más: ninguna fila depende de ella.
alter table order_drafts drop column version;
