-- +goose Up
-- Varias cajas físicas con nombre (antes había UNA sola caja implícita por empresa). La caja
-- primaria es la que recibe las ventas del POS; las demás son cajas de manejo de efectivo
-- (p. ej. caja fuerte) que solo mueven fondo, traspasos y gastos en efectivo. Un register_session
-- (corte) pasa a pertenecer a una caja; los gastos pagados exigen una caja abierta; y un traspaso
-- mueve efectivo entre dos cajas abiertas generando el movimiento en AMBAS (ver cash_transfers).

create table cash_registers (
  id         bigint generated always as identity primary key,
  name       text not null,
  is_primary boolean not null default false,
  is_active  boolean not null default true,
  -- Mismo patrón tenant que 0023: el DEFAULT auto-sella el company_id en cada INSERT del app
  -- desde el GUC de sesión; missing_ok=true → NULL sin GUC (migraciones pasan company_id explícito).
  company_id bigint not null default current_setting('app.company_id', true)::bigint
             references companies(id) on delete cascade
);
create index cash_registers_company on cash_registers (company_id);
-- Exactamente UNA caja primaria por empresa (la que recibe ventas): unique sobre las filas is_primary.
create unique index cash_registers_one_primary on cash_registers (company_id) where is_primary;
-- Nombre único por empresa (no dos "Caja fuerte" en el mismo tenant).
create unique index cash_registers_company_name on cash_registers (company_id, name);

-- RLS: mismo aislamiento de tenant que el resto (0024). El app (gatobobah_app) NO es owner → aplica.
alter table cash_registers enable row level security;
create policy tenant_isolation on cash_registers
  using (company_id = current_setting('app.company_id', true)::bigint)
  with check (company_id = current_setting('app.company_id', true)::bigint);
grant select, insert, update, delete on cash_registers to gatobobah_app;

-- Siembra una caja principal (primaria) + una caja fuerte por CADA empresa existente.
-- (Empresas nuevas vía --create-company aún no siembran lookups per-tenant: mismo hueco
--  preexistente que payment_methods/expense_categories; sembrar su caja primaria va con ese TODO.)
insert into cash_registers (company_id, name, is_primary)
  select id, 'Caja principal', true from companies;
insert into cash_registers (company_id, name, is_primary)
  select id, 'Caja fuerte', false from companies;

-- Un corte ahora pertenece a una caja. Backfill: los cortes existentes son de la caja primaria.
alter table register_sessions add column register_id bigint references cash_registers(id);
update register_sessions s set register_id = (
  select r.id from cash_registers r where r.company_id = s.company_id and r.is_primary
);
alter table register_sessions alter column register_id set not null;

-- El límite "una caja abierta por empresa" (0023) pasa a "una sesión abierta por CAJA".
drop index one_open_session;
create unique index one_open_session_per_register on register_sessions (register_id) where status = 'abierta';

-- Traspaso entre cajas: registro de primera clase. Genera 2 movimientos (salida en origen +
-- entrada en destino) ligados a esta fila, insertados en la MISMA tx → ambas cajas lo detectan
-- de forma atómica. from<>to y monto>0 forzados aquí (defensa en profundidad sobre la validación
-- del servicio).
create table cash_transfers (
  id              bigint generated always as identity primary key,
  from_session_id bigint not null references register_sessions(id),
  to_session_id   bigint not null references register_sessions(id),
  amount          numeric(10,2) not null check (amount > 0),
  note            text,
  created_by      bigint not null references users(id),
  created_at      timestamptz not null default now(),
  company_id      bigint not null default current_setting('app.company_id', true)::bigint
                  references companies(id) on delete cascade,
  check (from_session_id <> to_session_id)
);
create index cash_transfers_company on cash_transfers (company_id);
alter table cash_transfers enable row level security;
create policy tenant_isolation on cash_transfers
  using (company_id = current_setting('app.company_id', true)::bigint)
  with check (company_id = current_setting('app.company_id', true)::bigint);
grant select, insert, update, delete on cash_transfers to gatobobah_app;

-- Un movimiento puede pertenecer a un traspaso (para distinguirlo en la lista de la caja).
alter table register_cash_movements add column transfer_id bigint references cash_transfers(id);

-- +goose Down
alter table register_cash_movements drop column transfer_id;
drop table cash_transfers;
drop index one_open_session_per_register;
create unique index one_open_session on register_sessions (company_id, status) where status = 'abierta';
alter table register_sessions drop column register_id;
drop table cash_registers;
