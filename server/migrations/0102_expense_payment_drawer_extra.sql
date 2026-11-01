-- +goose Up
-- Lo que salió del cajón además del importe de un pago (spec 050).
--
-- Un ticket mixto trae renglones que no son del negocio (excluidos, spec 049): el gasto es lo del
-- negocio, pero del cajón salió el ticket entero. `drawer_extra` guarda esa diferencia por pago, y la
-- salida de caja del pago (una sola, la de la 048) se registra por importe + extra. El total del
-- gasto no cambia.
--
-- Se sube con el negocio operando: la API vieja inserta sin la columna (extra 0 = lo de hoy), y el
-- monto de la salida lo fija un trigger, así que también una salida de la API vieja sale completa.
--
-- ROLLBACK: el Down se niega en cuanto un pago lleva extra, porque el corte perdería de dónde salió
-- esa diferencia. Desde el primero, volver atrás es restaurar el respaldo.

set local lock_timeout = '3s';

alter table expense_payments add column drawer_extra numeric(10,2) not null default 0;
alter table expense_payments add constraint expense_payments_drawer_extra_nonneg check (drawer_extra >= 0);

-- Una sola fuente del monto de la salida de un pago: el pago mismo. Go ya no suma.
-- +goose StatementBegin
create function expense_payment_salida_amount() returns trigger
language plpgsql as $$
begin
  if new.expense_payment_id is not null and new.kind = 'salida' then
    select ep.amount + ep.drawer_extra into new.amount from expense_payments ep where ep.id = new.expense_payment_id;
    -- Bajo RLS el pago de otra empresa no se ve: la FK compuesta lo rechaza igual.
    if not found then
      raise exception 'el pago % no existe para esta empresa', new.expense_payment_id;
    end if;
  end if;
  return new;
end;
$$;
-- +goose StatementEnd

create trigger register_cash_movements_salida_amount
  before insert on register_cash_movements
  for each row execute function expense_payment_salida_amount();

-- +goose Down
-- +goose StatementBegin
do $$
begin
  if exists (select 1 from expense_payments where drawer_extra > 0) then
    raise exception 'hay pagos con parte que no es del negocio: el Down la perdería; restaura el respaldo';
  end if;
end;
$$;
-- +goose StatementEnd
drop trigger register_cash_movements_salida_amount on register_cash_movements;
drop function expense_payment_salida_amount();
alter table expense_payments drop column drawer_extra;
