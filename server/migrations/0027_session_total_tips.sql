-- +goose Up
-- Propinas por método guardadas en el snapshot del corte (register_session_totals). Las propinas
-- son pass-through (dinero del personal, no venta) pero SÍ entran físicamente al cajón/tarjeta, así
-- que cuentan en el esperado al cerrar; guardarlas por método permite mostrarlas como línea
-- "Propinas" en el resumen del corte histórico, separadas de las ventas.
alter table register_session_totals
  add column tips numeric(10,2) not null default 0 check (tips >= 0);

-- +goose Down
alter table register_session_totals drop column tips;
