-- +goose Up
-- La caja de un pago de gasto, aunque no pertenezca a ningún corte (spec 047).
--
-- Hasta aquí `register_session_id` era a la vez «de qué caja salió» y «a qué corte entra», así que
-- un pago en efectivo exigía un turno abierto: el efectivo de un tercero o un gasto de días pasados
-- obligaba a abrir un turno que descuadraba la caja de hoy. Ahora la caja vive en su propia columna
-- y el corte sigue en la suya. «Lleva arqueo» por caja distingue el dinero que se cuenta del que no.
--
-- ROLLBACK: el Down se niega en cuanto existe un pago con caja y sin corte, porque perdería de qué
-- caja salió. Desde ese primer pago, volver atrás es restaurar el respaldo.

set local lock_timeout = '3s';

alter table cash_registers add column counts_cash boolean not null default true;
-- La principal vende en efectivo: sin arqueo, sus ventas no tendrían corte.
alter table cash_registers add constraint cash_registers_primary_counts check (not is_primary or counts_cash);
-- Para la FK compuesta: una FK simple salta RLS y dejaría a un pago apuntar a la caja de otra empresa.
alter table cash_registers add constraint cash_registers_id_company unique (id, company_id);

alter table expense_payments add column register_id bigint;

-- La API vieja, durante la subida, inserta sin register_id; y la caja de un pago con corte no puede
-- ser otra que la de su turno. Un trigger lo deriva en los dos casos.
-- +goose StatementBegin
create function expense_payment_register_from_session() returns trigger
language plpgsql as $$
begin
  if new.register_session_id is not null then
    select register_id into new.register_id from register_sessions where id = new.register_session_id;
    -- Bajo RLS una sesión de otra empresa no se ve: sin esto el pago quedaría con corte y sin caja.
    if not found then
      raise exception 'el turno % no existe para esta empresa', new.register_session_id;
    end if;
  end if;
  return new;
end;
$$;
-- +goose StatementEnd

create trigger expense_payments_register_from_session
  before insert or update of register_session_id, register_id on expense_payments
  for each row execute function expense_payment_register_from_session();

update expense_payments ep set register_id = s.register_id
  from register_sessions s
 where s.id = ep.register_session_id and ep.register_id is null;

-- +goose StatementBegin
do $$
begin
  if exists (select 1 from expense_payments where register_session_id is not null and register_id is null) then
    raise exception 'quedó un pago con corte y sin caja';
  end if;
end;
$$;
-- +goose StatementEnd

-- Un pago es un hecho contable: borrar la caja no puede llevárselo.
alter table expense_payments add constraint expense_payments_register_fk
  foreign key (register_id, company_id) references cash_registers (id, company_id)
  on delete restrict not valid;
alter table expense_payments validate constraint expense_payments_register_fk;

-- +goose Down
-- +goose StatementBegin
do $$
begin
  if exists (select 1 from expense_payments where register_id is not null and register_session_id is null) then
    raise exception 'hay pagos de gasto fuera de corte: el Down perdería de qué caja salieron; restaura el respaldo';
  end if;
end;
$$;
-- +goose StatementEnd
alter table expense_payments drop constraint expense_payments_register_fk;
drop trigger expense_payments_register_from_session on expense_payments;
drop function expense_payment_register_from_session();
alter table expense_payments drop column register_id;
alter table cash_registers drop constraint cash_registers_id_company;
alter table cash_registers drop constraint cash_registers_primary_counts;
alter table cash_registers drop column counts_cash;
