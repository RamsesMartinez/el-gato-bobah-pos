-- +goose Up
-- Renglones excluidos y total del ticket (spec 049).
--
-- Un ticket trae cosas del negocio y cosas que no lo son. Hasta aquí el renglón «personal» se tiraba
-- al guardar: no quedaba evidencia de que venía en el ticket. Ahora se guarda con `excluded`, y no
-- entra a existencias, ni al costo, ni a la suma de renglones contra el total. El total impreso en el
-- documento leído se guarda aparte en `document_total`: el total del gasto sigue siendo el que se
-- escribe a mano.
--
-- Aditiva: la API vieja, durante la subida, sigue descartando lo «personal» e inserta con los
-- defaults.
--
-- ROLLBACK: el Down se niega en cuanto existe un excluido (al quitar la columna volvería a contar
-- en silencio) o un total del ticket (no se reconstruye). Desde la primera marca, volver atrás es
-- restaurar el respaldo.

set local lock_timeout = '3s';

alter table expense_items add column excluded boolean not null default false;
-- Un excluido es evidencia, no gasto: con importe 0 se guarda igual. Sigue `not valid` como en la
-- 0092 y no se valida aquí: los renglones heredados con importe 0 (medidos el 14-oct) no cumplen ni
-- la regla vieja, y validar haría que la subida truene en producción.
alter table expense_items drop constraint expense_items_amount_positive;
alter table expense_items add constraint expense_items_amount_positive
  check (amount > 0 or is_gift or (excluded and amount >= 0)) not valid;

-- Mismo tipo que expenses.amount.
alter table expenses add column document_total numeric(10,2);
alter table expenses add constraint expenses_document_total_nonnegative
  check (document_total is null or document_total >= 0) not valid;
alter table expenses validate constraint expenses_document_total_nonnegative;

-- +goose Down
-- +goose StatementBegin
do $$
begin
  if exists (select 1 from expense_items where excluded) then
    raise exception 'hay renglones excluidos: el Down los volvería a contar; restaura el respaldo';
  end if;
  if exists (select 1 from expenses where document_total is not null) then
    raise exception 'hay gastos con total del ticket: el Down lo perdería; restaura el respaldo';
  end if;
end;
$$;
-- +goose StatementEnd
alter table expenses drop constraint expenses_document_total_nonnegative;
alter table expenses drop column document_total;
alter table expense_items drop constraint expense_items_amount_positive;
alter table expense_items add constraint expense_items_amount_positive check (amount > 0 or is_gift) not valid;
alter table expense_items drop column excluded;
