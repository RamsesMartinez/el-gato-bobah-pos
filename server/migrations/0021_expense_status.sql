-- +goose Up
-- Estados de pago del gasto (cuentas por pagar): se registra 'pendiente' (deuda) o directo
-- 'pagada'; 'cancelada' anula una pendiente. 'pagada' es terminal (el dinero salió); para
-- revertir un gasto pagado será una feature aparte, igual que el reembolso de una orden.
create type expense_status as enum ('pendiente', 'pagada', 'cancelada');

alter table expenses
  add column status        expense_status not null default 'pendiente',
  add column paid_at        timestamptz,
  add column paid_by        bigint references users(id),
  add column cancelled_at   timestamptz,
  add column cancelled_by   bigint references users(id),
  add column cancel_reason  text;

-- Backfill: lo que ya tenía método de pago se considera pagado (paid_at = alta); el resto queda
-- pendiente. Debe correr ANTES de crear el check (que exige paid_at+método en 'pagada').
update expenses set status = 'pagada', paid_at = created_at, paid_by = created_by
where payment_method_id is not null;

-- 'pagada' exige cuándo y con qué método; 'cancelada' exige quién/cuándo (auditoría contable).
alter table expenses add constraint expenses_paid_requires_method
  check (status <> 'pagada' or (paid_at is not null and payment_method_id is not null));
alter table expenses add constraint expenses_cancelled_requires_audit
  check (status <> 'cancelada' or (cancelled_at is not null and cancelled_by is not null));

create index expenses_status_date on expenses (status, expense_date desc);

-- +goose Down
drop index if exists expenses_status_date;
alter table expenses drop constraint if exists expenses_cancelled_requires_audit;
alter table expenses drop constraint if exists expenses_paid_requires_method;
alter table expenses
  drop column status, drop column paid_at, drop column paid_by,
  drop column cancelled_at, drop column cancelled_by, drop column cancel_reason;
drop type if exists expense_status;
