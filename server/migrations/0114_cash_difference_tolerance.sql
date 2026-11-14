-- +goose Up
-- Tolerancia de diferencia por empresa (#290, spec 082 fase 4; decisión del dueño P1, 2026-11-13).
-- Un corte cuya diferencia la pasa y no tiene explicación sale «por explicar» en el Histórico. No
-- bloquea el cierre ni pide PIN. $0 por omisión: un negocio que no la configura no acepta
-- diferencias en silencio.
--
-- Aditiva con default: la API anterior no la lee y sus inserts de business_settings la llenan solos.
-- Un `select` largo sobre la tabla haría esperar al alter, y en la espera bloquearía las lecturas de
-- casi toda petición: mejor fallar el despliegue en 3 s que detener la caja.
set local lock_timeout = '3s';
alter table business_settings add column cash_difference_tolerance numeric(10,2) not null default 0
  constraint business_settings_cash_difference_tolerance_nonneg check (cash_difference_tolerance >= 0);

-- +goose Down
-- Pierde la tolerancia configurada: es configuración, no un hecho; al volver a subir queda en $0.
alter table business_settings drop column cash_difference_tolerance;
