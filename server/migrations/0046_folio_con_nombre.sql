-- +goose Up
-- El nombre con el que se canta el pedido en cocina: "Tigre" en vez de "#14".
--
-- Se GUARDA en vez de derivarse del daily_number, aunque derivarlo sería gratis y sin colisiones.
-- El motivo es que este nombre se imprime en el ticket y el cliente lo usa para pedir su factura:
-- si saliera de una lista en el código, ampliar esa lista renombraría pedidos viejos y un ticket
-- impreso como "Tigre" se reimprimiría como "Zorro". Un identificador que va en papel no puede
-- cambiar de significado con un deploy.
--
-- Es nullable a propósito: los pedidos anteriores a esta migración no tienen nombre y no se les
-- inventa uno — el que se imprimió en su día llevaba solo el número, y ese sigue siendo su folio.
alter table orders
  add column folio_name text;

-- +goose Down
alter table orders drop column folio_name;
