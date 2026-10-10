-- +goose Up
-- «Cancelar lo que falta» (decisión del dueño, 2026-10-09, opción A).
--
-- Un entregado pagado a medias cuyo cliente se fue: lo pagado se queda como venta y lo que falta se
-- da por PERDIDO, con su motivo, quién y cuándo. Va en el pedido y no en una tabla aparte: es UNA
-- vez por pedido (después ya no debe nada) y así toda consulta que calcula lo que se debe lo tiene a
-- la mano sin otra unión 1:N.
--
-- `written_off_business_date` es el día en que se dio por perdido en la zona del negocio: Ventas lo
-- reporta ese día, igual que un cobro o una devolución cuentan el día en que pasaron.
alter table orders
  add column written_off_amount        numeric(10,2) not null default 0 check (written_off_amount >= 0),
  add column written_off_reason        text,
  add column written_off_by            bigint references users(id),
  add column written_off_at            timestamptz,
  add column written_off_business_date date,
  add constraint orders_write_off_complete check (
    (written_off_amount = 0 and written_off_at is null)
    or (written_off_amount > 0 and written_off_at is not null and written_off_business_date is not null
        and written_off_by is not null and length(btrim(coalesce(written_off_reason, ''))) > 0)
  );

-- +goose Down
alter table orders
  drop constraint orders_write_off_complete,
  drop column written_off_business_date,
  drop column written_off_at,
  drop column written_off_by,
  drop column written_off_reason,
  drop column written_off_amount;
