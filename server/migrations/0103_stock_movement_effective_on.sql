-- +goose Up
-- El día en que pasó cada movimiento de una compra (spec 053): la fecha de la entrega, distinta del
-- instante en que alguien la capturó (`created_at`). Una entrega que se registra días después, y el
-- contra-movimiento de un renglón que se borra, llevan el día de la entrega original; sin esta
-- columna esa fecha no existía en ningún lado y no se puede recuperar después (principio VIII).
--
-- La llena un trigger cuando llega nula en una compra o en un movimiento de un gasto, con el día del
-- negocio de la EMPRESA de la fila (su zona en business_settings): la API anterior, durante la
-- subida, inserta sin ella y aun así queda fechada. Lo ya registrado se rellena con el día de su
-- captura en la zona de hoy de cada empresa: si la zona cambiara después, un nulo se leería distinto.
--
-- Se sube con el negocio operando: solo agrega una columna nullable y un trigger.
--
-- ROLLBACK: el Down se niega en cuanto una fecha no sale del día de su captura, porque esa la eligió
-- una persona y se perdería. Desde la primera, volver atrás es restaurar el respaldo.

set local lock_timeout = '3s';

alter table stock_movements add column effective_on date;

-- +goose StatementBegin
create function business_day_of(company bigint, at timestamptz) returns date
language sql stable as $$
  select (at at time zone coalesce(
    (select timezone from business_settings where company_id = company), 'America/Mexico_City'))::date
$$;
-- +goose StatementEnd

-- +goose StatementBegin
create function fill_stock_movement_effective_on() returns trigger
language plpgsql as $$
begin
  if new.effective_on is null and (new.movement_type = 'compra' or new.expense_id is not null) then
    new.effective_on := business_day_of(new.company_id, coalesce(new.created_at, now()));
  end if;
  return new;
end;
$$;
-- +goose StatementEnd

create trigger trg_stock_movements_effective_on
  before insert on stock_movements
  for each row execute function fill_stock_movement_effective_on();

update stock_movements set effective_on = business_day_of(company_id, created_at)
 where effective_on is null and (movement_type = 'compra' or expense_id is not null);

-- +goose Down
-- +goose StatementBegin
do $$
begin
  if exists (select 1 from stock_movements
              where effective_on is not null and effective_on <> business_day_of(company_id, created_at)) then
    raise exception 'hay entregas con su fecha capturada: el Down la perdería; restaura el respaldo';
  end if;
end;
$$;
-- +goose StatementEnd
drop trigger trg_stock_movements_effective_on on stock_movements;
drop function fill_stock_movement_effective_on();
drop function business_day_of(bigint, timestamptz);
alter table stock_movements drop column effective_on;
