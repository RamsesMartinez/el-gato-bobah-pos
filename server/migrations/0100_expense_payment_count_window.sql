-- +goose Up
-- El arqueo por hora de los gastos (spec 048).
--
-- Un pago en efectivo tenía dos estados: en un corte, o fuera de todo corte. Falta el de en medio:
-- incluido en el arqueo pero sin corte todavía («pendiente»), que el siguiente corte cuya ventana
-- cubra su hora se lleva. Para eso cada pago guarda si se incluye y a qué hora se pagó, y cada
-- salida de caja de un gasto dice de qué pago es, para que la base impida descontarlo dos veces.
--
-- Se sube con el negocio operando: todo es aditivo. La API vieja inserta sin las columnas nuevas y
-- se comporta igual que antes: «sin incluir» por omisión, «incluido» si lleva corte (lo pone el
-- trigger), y la hora de la base.
--
-- ROLLBACK: el Down se niega en cuanto existe un pendiente, porque perdería que ese gasto debe
-- entrar a un corte. Desde ese primer pendiente, volver atrás es restaurar el respaldo.

set local lock_timeout = '3s';

alter table expense_payments add column include_in_count boolean not null default false;
alter table expense_payments add column paid_at timestamptz;

-- Relleno. La hora de captura si se pagó el día local de su captura; si no, la medianoche local de
-- su día: es una hora FABRICADA, porque nadie guardó la real de un pago capturado días después, y
-- la medianoche lo deja antes del cierre de ese día, donde ningún corte lo toma.
with zona as (
  select ep.id, coalesce((select bs.timezone from business_settings bs where bs.company_id = ep.company_id),
                         'America/Mexico_City') as tz
    from expense_payments ep
)
update expense_payments ep
   set paid_at = case
         when ep.paid_on = (ep.created_at at time zone z.tz)::date then ep.created_at
         else ep.paid_on::timestamp at time zone z.tz
       end,
       include_in_count = ep.register_session_id is not null
  from zona z
 where z.id = ep.id;

-- La API nueva manda la hora con el reloj del servicio; este default es para la vieja.
alter table expense_payments alter column paid_at set default clock_timestamp();
alter table expense_payments alter column paid_at set not null;

-- Con corte, siempre incluido: lo fuerza el trigger de la 0099 (la API vieja no manda la columna) y
-- lo vigila el check.
-- +goose StatementBegin
create or replace function expense_payment_register_from_session() returns trigger
language plpgsql as $$
begin
  if new.register_session_id is not null then
    select register_id into new.register_id from register_sessions where id = new.register_session_id;
    -- Bajo RLS una sesión de otra empresa no se ve: sin esto el pago quedaría con corte y sin caja.
    if not found then
      raise exception 'el turno % no existe para esta empresa', new.register_session_id;
    end if;
    new.include_in_count := true;
  end if;
  return new;
end;
$$;
-- +goose StatementEnd

alter table expense_payments add constraint expense_payments_session_included
  check (register_session_id is null or include_in_count) not valid;
alter table expense_payments validate constraint expense_payments_session_included;

-- Para la FK compuesta del movimiento: una FK simple salta RLS y ligaría un movimiento al pago de
-- otra empresa.
alter table expense_payments add constraint expense_payments_company_id unique (company_id, id);

alter table register_cash_movements add column expense_payment_id bigint;

-- Las salidas viejas se ligan a su pago cuando el par es inequívoco: mismo turno, mismo gasto y
-- mismo importe, emparejados en orden de captura. Lo que no empareja se queda sin liga.
with pagos as (
  select ep.id, ep.register_session_id, ep.expense_id, ep.amount,
         row_number() over (partition by ep.register_session_id, ep.expense_id, ep.amount order by ep.id) as n
    from expense_payments ep
    join payment_methods pm on pm.id = ep.payment_method_id
   where ep.register_session_id is not null and pm.affects_cash_drawer
), salidas as (
  select m.id, m.session_id, m.expense_id, m.amount,
         row_number() over (partition by m.session_id, m.expense_id, m.amount order by m.id) as n
    from register_cash_movements m
   where m.kind = 'salida' and m.expense_id is not null
)
update register_cash_movements m
   set expense_payment_id = p.id
  from salidas s
  join pagos p on p.register_session_id = s.session_id and p.expense_id = s.expense_id
              and p.amount = s.amount and p.n = s.n
 where m.id = s.id;

-- Un pago es un hecho contable: no se borra mientras un movimiento lo nombre.
alter table register_cash_movements add constraint register_cash_movements_expense_payment_fk
  foreign key (company_id, expense_payment_id) references expense_payments (company_id, id)
  on delete restrict not valid;
alter table register_cash_movements validate constraint register_cash_movements_expense_payment_fk;

-- Un pago sale del cajón una sola vez: el doble descuento falla en la base.
create unique index register_cash_movements_one_salida_per_payment
  on register_cash_movements (expense_payment_id) where kind = 'salida';

-- Los pendientes de una caja, para la apertura y el barrido.
create index expense_payments_pending
  on expense_payments (company_id, register_id, paid_at)
  where register_session_id is null and include_in_count;

-- +goose Down
-- +goose StatementBegin
do $$
begin
  if exists (select 1 from expense_payments where register_session_id is null and include_in_count) then
    raise exception 'hay pagos de gasto pendientes de corte: el Down perdería que deben entrar a uno; restaura el respaldo';
  end if;
end;
$$;
-- +goose StatementEnd
drop index expense_payments_pending;
drop index register_cash_movements_one_salida_per_payment;
alter table register_cash_movements drop constraint register_cash_movements_expense_payment_fk;
alter table register_cash_movements drop column expense_payment_id;
alter table expense_payments drop constraint expense_payments_company_id;
alter table expense_payments drop constraint expense_payments_session_included;
-- +goose StatementBegin
create or replace function expense_payment_register_from_session() returns trigger
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
alter table expense_payments drop column paid_at;
alter table expense_payments drop column include_in_count;
