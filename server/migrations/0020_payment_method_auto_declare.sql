-- +goose Up
-- A nivel negocio: qué métodos de pago se declaran automáticamente al cerrar caja (el
-- declarado = esperado, sin que el cajero capture nada) en vez de requerir conteo manual.
-- Default false para todos: no cambia el comportamiento actual hasta que el negocio elija
-- explícitamente qué métodos marcar (p. ej. tarjeta/transferencia/plataformas, que no se cuentan
-- a mano como el efectivo).
alter table payment_methods add column auto_declare boolean not null default false;

-- +goose Down
alter table payment_methods drop column auto_declare;
