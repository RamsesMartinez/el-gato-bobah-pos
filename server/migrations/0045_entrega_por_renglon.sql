-- +goose Up
-- +goose StatementBegin
-- Cuánto de este renglón ya se le dio al cliente.
--
-- Es CANTIDAD y no un booleano porque en un pedido grande la comida sale por tandas: de 5 alitas
-- salen 3 y las otras 2 siguen en la freidora. Con un booleano el operador tendría que elegir
-- entre mentir ("ya está todo") u olvidar lo que sí entregó, y lo que se pierde es justo el dato
-- que la pantalla existe para llevar.
--
-- lock_timeout: order_lines es la tabla más grande del sistema y esto corre sobre un negocio en
-- operación. Con el tope, una migración que llega en hora pico falla rápido y se reintenta en vez
-- de encolar detrás de sí todas las ventas del local.
set local lock_timeout = '3s';

-- Sin reescritura de tabla: Postgres 11+ guarda el default en el catálogo.
alter table order_lines
  add column delivered_qty numeric(8,2) not null default 0
    check (delivered_qty >= 0);

-- Los pedidos ya entregados nacen con sus renglones completos. Sin esto el tablero los leería como
-- pendientes: nadie los volvería a tocar, pero cualquier conteo de "qué falta" arrancaría mintiendo.
update order_lines l
   set delivered_qty = l.quantity
  from orders o
 where o.id = l.order_id
   and o.status = 'entregada'
   and l.cancelled_at is null;

-- El tope contra quantity lo pone la base y no solo el servicio: entre validar y escribir cabe
-- otra transacción entregando lo mismo, y de esto cuelga el cierre automático del pedido.
--
-- NOT VALID + VALIDATE en dos pasos, no un check directo: añadirlo validado escanea la tabla
-- entera reteniendo ACCESS EXCLUSIVE, y ahí el lock_timeout de arriba no ayuda —el lock se toma
-- rápido y se suelta tarde—. VALIDATE hace el escaneo con un lock que sí deja vender.
alter table order_lines
  add constraint order_lines_entregado_no_excede
    check (delivered_qty <= quantity) not valid;

alter table order_lines validate constraint order_lines_entregado_no_excede;

-- El tablero pregunta "de este pedido, ¿qué renglones faltan?" en cada pintada.
create index order_lines_pendientes on order_lines (order_id)
  where cancelled_at is null and delivered_qty < quantity;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
drop index if exists order_lines_pendientes;
alter table order_lines drop constraint if exists order_lines_entregado_no_excede;
alter table order_lines drop column delivered_qty;
-- +goose StatementEnd
