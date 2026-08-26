-- +goose Up
-- Pagos múltiples por gasto. Un ticket real cobra con más de un medio (Soriana: 640.06 a
-- tarjeta + 0.01 en efectivo en el mismo ticket), y con una sola columna payment_method_id en
-- expenses eso no se puede registrar.
--
-- Es el mismo patrón que order_payments (0007): los pagos son filas hijas, no columnas del
-- encabezado. Se copia a propósito en vez de inventar otro modelo.

create table expense_payments (
  id                bigint generated always as identity primary key,
  expense_id        bigint not null references expenses(id) on delete cascade,
  payment_method_id smallint not null references payment_methods(id),
  amount            numeric(10,2) not null check (amount > 0),
  -- paid_on: la fecha en que salió el dinero, que no es la del documento ni la de captura
  -- (una factura a crédito se paga semanas después).
  paid_on           date not null,
  -- register_session_id no-null ES el flag de "entra al arqueo": una columna carga el flag y
  -- la sesión a la que se atribuye. Para métodos que afectan el cajón el servicio la EXIGE —
  -- efectivo que sale sin movimiento de caja descuadra el corte.
  register_session_id bigint references register_sessions(id),
  reference         text,
  paid_by           bigint not null references users(id),
  created_at        timestamptz not null default now(),
  company_id        bigint not null default current_setting('app.company_id', true)::bigint
                    references companies(id) on delete cascade
);
create index expense_payments_expense on expense_payments (expense_id);
create index expense_payments_session on expense_payments (register_session_id, payment_method_id);
create index expense_payments_company on expense_payments (company_id);

alter table expense_payments enable row level security;
create policy tenant_isolation on expense_payments
  using (company_id = current_setting('app.company_id', true)::bigint)
  with check (company_id = current_setting('app.company_id', true)::bigint);
grant select, insert, update, delete on expense_payments to gatobobah_app;

-- Backfill: cada gasto ya pagado se convierte en un pago único equivalente. Debe correr ANTES
-- de soltar las columnas viejas.
insert into expense_payments (
  expense_id, payment_method_id, amount, paid_on, register_session_id, paid_by, created_at, company_id
)
select e.id, e.payment_method_id, e.amount, coalesce(e.paid_at::date, e.expense_date),
       e.register_session_id, coalesce(e.paid_by, e.created_by), coalesce(e.paid_at, e.created_at),
       e.company_id
from expenses e
where e.status = 'pagada' and e.payment_method_id is not null;

-- Las columnas de pago del encabezado se van: mantener las dos fuentes es una desincronización
-- esperando a pasar. El estado (status/paid_at) SÍ se queda — es el ciclo del gasto, no el pago.
alter table expenses drop constraint if exists expenses_paid_requires_method;
-- 'pagada' sigue exigiendo cuándo; con qué método ahora vive en expense_payments (y el servicio
-- exige que los pagos cubran el importe antes de marcarla pagada).
alter table expenses add constraint expenses_paid_requires_date
  check (status <> 'pagada' or paid_at is not null);
alter table expenses drop column payment_method_id;
alter table expenses drop column register_session_id;

-- +goose Down
alter table expenses add column payment_method_id smallint references payment_methods(id);
alter table expenses add column register_session_id bigint references register_sessions(id);
-- Restaura el pago "principal" (el más antiguo) en el encabezado; un gasto con pago partido
-- pierde los demás medios: la información multi-pago no cabe en el modelo viejo.
update expenses e set payment_method_id = p.payment_method_id, register_session_id = p.register_session_id
from (
  select distinct on (expense_id) expense_id, payment_method_id, register_session_id
  from expense_payments order by expense_id, created_at
) p
where p.expense_id = e.id;
alter table expenses drop constraint if exists expenses_paid_requires_date;
alter table expenses add constraint expenses_paid_requires_method
  check (status <> 'pagada' or (paid_at is not null and payment_method_id is not null));
drop table if exists expense_payments;
