-- +goose Up
-- Los pagos anteriores a 0080 quedan en el día de su PEDIDO, que es como Ventas los reportaba hasta
-- hoy: rellenarlos con otro día movería cifras de periodos ya cerrados.
--
-- Va aparte de 0080 a propósito: es un update de toda la tabla de pagos, y separado del cambio de
-- esquema falla o se reintenta solo. Con la columna llena, las consultas pueden filtrar por el
-- índice `(company_id, business_date)` en vez de recorrer todos los pagos.
set local lock_timeout = '3s';

update order_payments op
   set business_date = o.business_date
  from orders o
 where o.id = op.order_id
   and op.business_date is null;

-- +goose Down
-- Nada que deshacer: la columna la quita el Down de 0080, y vaciarla aquí borraría también el día de
-- los pagos que entraron después, que sí es un hecho.
select 1;
