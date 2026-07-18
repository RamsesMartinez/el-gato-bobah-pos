-- +goose Up
-- Separa el pago con tarjeta en débito/crédito (comisiones y conciliación distintas).
-- Reutiliza la fila 2 ("Tarjeta" → "Tarjeta débito") y agrega "Tarjeta crédito".
-- Idempotente: la API se auto-migra en boot y este archivo puede haberse aplicado a mano.
update payment_methods set name = 'Tarjeta débito' where id = 2 and name = 'Tarjeta';
insert into payment_methods (name, kind, affects_cash_drawer, sort_key)
  values ('Tarjeta crédito', 'tarjeta', false, 250)
  on conflict (name) do nothing;

-- +goose Down
delete from payment_methods where name = 'Tarjeta crédito';
update payment_methods set name = 'Tarjeta' where id = 2 and name = 'Tarjeta débito';
