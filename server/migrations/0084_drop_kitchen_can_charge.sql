-- +goose Up
-- Se quita el ajuste «el tablero puede cobrar» (decisión del dueño, 2026-10-09).
--
-- Desde la spec 030 se cobra en la cuenta, en Vender; el ajuste solo le dejaba un botón de cobro a
-- quien no puede entrar a Vender, y en la operación de hoy no lo usa nadie. Era una cuarta puerta de
-- cobro que nadie revisa.
--
-- IRREVERSIBLE EN LOS DATOS: el valor que cada empresa tenía se pierde. El Down vuelve a crear la
-- columna con su default de siempre (apagado), que es el valor seguro: un tablero que no cobra.
alter table business_settings drop column kitchen_can_charge;

-- +goose Down
alter table business_settings
  add column kitchen_can_charge boolean not null default false;
