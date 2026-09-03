-- +goose Up
-- Cuándo salió este renglón en una comanda de cocina. NULL = todavía no salió.
--
-- Va en el RENGLÓN y no en el pedido porque la pregunta que hay que poder responder es "¿qué
-- renglones de este pedido no han salido a cocina?", y un pedido con dos comandas —la de cuando se
-- confirmó y la de lo que se agregó después— tiene renglones de los dos lados. Una marca por pedido
-- no puede distinguirlos, y sin distinguirlos la comanda del agregado sale con todo y cocina
-- prepara dos veces lo mismo.
--
-- Timestamp y no booleano: cuando cocina reclame que no le llegó algo, la pregunta es CUÁNDO salió,
-- no si salió. Un booleano obliga a cruzar con los logs para responderla y cuesta lo mismo.
--
-- El backfill NO marca nada, y es la decisión: de un renglón de hace tres semanas nadie sabe si
-- salió en papel, y marcarlo como enviado sería afirmar algo que no consta. NULL significa "no se
-- sabe", que es la verdad. Ninguna pantalla decide nada con los renglones viejos — la comanda del
-- agregado se dispara al AGREGAR, no al leer.
--
-- Sin índice a propósito: la única consulta que la filtra es "los renglones sin enviar de ESTE
-- pedido", que ya entra por order_lines_order (order_id) sobre un pedido de 6 renglones como
-- máximo. Un índice ahí no compra nada y hay que mantenerlo.
alter table order_lines add column enviado_a_cocina_at timestamptz;

-- +goose Down
alter table order_lines drop column enviado_a_cocina_at;
