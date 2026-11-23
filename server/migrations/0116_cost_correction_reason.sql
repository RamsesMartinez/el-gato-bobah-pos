-- +goose Up
-- El motivo de una corrección de costo (spec 090, #289). La corrección desde el MCP lo exige: «6 de 23
-- ramen difieren de la última compra» no se puede auditar después sin saber por qué cambió cada uno.
-- La pantalla sigue guardando sin motivo, así que la columna es nullable.
--
-- Aditiva y sin default: la API anterior inserta sin la columna y su fila queda con null. La tabla
-- es chica (una fila por corrección a mano) y el check no tiene filas que validar al agregarse.
-- El check exige el texto ya recortado: el servicio recorta, y un motivo de solo espacios no es motivo.
set local lock_timeout = '3s';
alter table ingredient_cost_corrections add column reason text
  constraint ingredient_cost_corrections_reason_text
  check (reason is null or (reason = btrim(reason) and char_length(reason) between 1 and 200));

-- +goose Down
-- Pierde los motivos guardados; al volver a subir, la columna nace vacía y se acepta igual.
set local lock_timeout = '3s';
alter table ingredient_cost_corrections drop column reason;
