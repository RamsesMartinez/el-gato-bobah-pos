-- +goose Up
-- Caja y cobro con tarjeta (spec 032, decisiones del dueño del 2026-10-09, puntos 3 a 10).
-- Los catálogos nuevos son por empresa (conceptos) o por sucursal (terminales): puerta de «más de
-- una sucursal» (constitución VIII). Todas las FK entre tablas de empresa son compuestas: los
-- chequeos de integridad referencial saltan RLS.

-- 1. CONCEPTOS DE SALIDA (punto 3). El nombre se compara sin mayúsculas ni espacios extremos para
--    que «Hielo» y «hielo » no sean dos conceptos. Archivar y fusionar no borran: las salidas viejas
--    siguen apuntando a su concepto.
-- Llaves compuestas para que un concepto no se ligue a la categoría o al proveedor de otra empresa
-- (la FK simple saltaría RLS).
create unique index expense_categories_tenant_key on expense_categories (company_id, id);
create unique index suppliers_tenant_key on suppliers (company_id, id);

create table cash_concepts (
  id                  bigint generated always as identity primary key,
  company_id          bigint not null default nullif(current_setting('app.company_id', true), '')::bigint
                      references companies(id) on delete cascade,
  name                text not null check (name = btrim(name) and length(name) between 1 and 60),
  expense_category_id bigint,
  supplier_id         bigint,
  archived_at         timestamptz,
  merged_into_id      bigint,
  created_at          timestamptz not null default now(),
  constraint cash_concepts_tenant_key unique (company_id, id),
  constraint cash_concepts_category_fkey
    foreign key (company_id, expense_category_id) references expense_categories (company_id, id) on delete restrict,
  constraint cash_concepts_supplier_fkey
    foreign key (company_id, supplier_id) references suppliers (company_id, id) on delete restrict,
  constraint cash_concepts_merged_fkey
    foreign key (company_id, merged_into_id) references cash_concepts (company_id, id) on delete restrict,
  constraint cash_concepts_not_into_itself check (merged_into_id is distinct from id),
  -- Fusionado implica archivado: un concepto que apunta a otro no se ofrece.
  constraint cash_concepts_merged_is_archived check (merged_into_id is null or archived_at is not null)
);
create unique index cash_concepts_active_name on cash_concepts (company_id, lower(name)) where archived_at is null;

alter table cash_concepts enable row level security;
create policy tenant_isolation on cash_concepts
  using (company_id = nullif(current_setting('app.company_id', true), '')::bigint)
  with check (company_id = nullif(current_setting('app.company_id', true), '')::bigint);
grant select, insert, update on cash_concepts to gatobobah_app;

-- Conceptos frecuentes por empresa (los de la decisión del punto 3).
insert into cash_concepts (company_id, name)
select c.id, n.name from companies c cross join (values ('Basura'), ('Hielo'), ('Vigilancia'), ('Insumos')) as n(name);

-- 2. SALIDAS CON CONCEPTO Y CORRECCIÓN POR REVERSO (puntos 3 y 4). Nada se edita ni se borra: un
--    reverso es un movimiento nuevo ligado a la salida que corrige, y a lo más hay uno por salida.
alter table register_cash_movements drop constraint register_cash_movements_kind_check;
alter table register_cash_movements add constraint register_cash_movements_kind_check
  check (kind in ('entrada', 'salida', 'propina', 'reverso'));
alter table register_cash_movements
  add column concept_id  bigint,
  add column reverses_id bigint;
alter table register_cash_movements add constraint register_cash_movements_concept_fkey
  foreign key (company_id, concept_id) references cash_concepts (company_id, id) on delete restrict;
alter table register_cash_movements add constraint register_cash_movements_reverses_fkey
  foreign key (company_id, reverses_id) references register_cash_movements (company_id, id) on delete restrict;
alter table register_cash_movements add constraint register_cash_movements_reverse_shape
  check ((kind = 'reverso') = (reverses_id is not null));
-- Sin company_id a propósito: reverses_id ya es un id de esta empresa (FK compuesta), y dos reversos
-- de la misma salida son el defecto que se cierra.
create unique index register_cash_movements_one_reverse on register_cash_movements (reverses_id)
  where reverses_id is not null;
create index register_cash_movements_company_concept on register_cash_movements (company_id, concept_id)
  where concept_id is not null;

-- 3. DÍA DEL GASTO Y FECHA DEL DOCUMENTO (punto 5). `expense_date` sigue siendo el día del gasto (el
--    que usan los reportes); la fecha del ticket o factura va aparte y no lo mueve.
alter table expenses add column document_date date;

-- 4. APERTURA A CIEGAS CON MOTIVO (punto 6) y modo de arqueo de tarjeta copiado al abrir (punto 9):
--    cambiar el ajuste con la caja abierta no cambia lo que se le pide a quien ya está contando.
alter table register_sessions
  add column opening_reason      text check (opening_reason in ('conteo_anterior_mal', 'cambio_de_fondo', 'retiro_no_registrado', 'otro')),
  add column opening_reason_note text check (opening_reason_note is null or length(opening_reason_note) between 1 and 200),
  add column card_count_mode     text not null default 'automatico' check (card_count_mode in ('automatico', 'por_terminal'));
alter table register_sessions add constraint register_sessions_other_needs_note
  check (opening_reason is distinct from 'otro' or opening_reason_note is not null);

-- 5. TERMINALES POR SUCURSAL (punto 8) y modo de arqueo por sucursal (punto 9).
alter table branches add column card_count_mode text not null default 'automatico'
  check (card_count_mode in ('automatico', 'por_terminal'));
-- El Gato Bobah arranca con arqueo por terminal (decisión del dueño). Por slug: el id 1 es otra empresa.
update branches b set card_count_mode = 'por_terminal'
  from companies c where c.id = b.company_id and c.slug = 'gatobobah';

create table card_terminals (
  id          bigint generated always as identity primary key,
  company_id  bigint not null default nullif(current_setting('app.company_id', true), '')::bigint
              references companies(id) on delete cascade,
  branch_id   bigint not null,
  name        text not null check (name = btrim(name) and length(name) between 1 and 40),
  archived_at timestamptz,
  created_at  timestamptz not null default now(),
  constraint card_terminals_branch_fkey
    foreign key (company_id, branch_id) references branches (company_id, id) on delete restrict
);
create unique index card_terminals_tenant_key on card_terminals (company_id, id);
create unique index card_terminals_active_name on card_terminals (company_id, branch_id, lower(name)) where archived_at is null;

alter table card_terminals enable row level security;
create policy tenant_isolation on card_terminals
  using (company_id = nullif(current_setting('app.company_id', true), '')::bigint)
  with check (company_id = nullif(current_setting('app.company_id', true), '')::bigint);
grant select, insert, update on card_terminals to gatobobah_app;

-- Un negocio nace con una terminal por omisión (punto 8): una por sucursal existente.
insert into card_terminals (company_id, branch_id, name) select company_id, id, 'Terminal' from branches where is_active;

-- 6. EL COBRO CON TARJETA GUARDA SU TERMINAL, con el nombre copiado: renombrar una terminal no
--    reescribe cobros pasados. Nulo en los cobros de antes de esta migración y en los que no son
--    tarjeta; el servicio lo exige en los nuevos con tarjeta.
alter table order_payments
  add column card_terminal_id   bigint,
  add column card_terminal_name text;
alter table order_payments add constraint order_payments_card_terminal_fkey
  foreign key (company_id, card_terminal_id) references card_terminals (company_id, id) on delete restrict;
alter table order_payments add constraint order_payments_card_terminal_shape
  check ((card_terminal_id is null) = (card_terminal_name is null));
create index order_payments_company_terminal on order_payments (company_id, register_session_id, card_terminal_id)
  where card_terminal_id is not null;

-- 7. FOLIO DE DEVOLUCIÓN CON TARJETA (punto 10): lo que imprime la terminal, y quién lo capturó.
alter table order_refunds
  add column card_refund_folio       text check (card_refund_folio is null or (card_refund_folio = btrim(card_refund_folio) and length(card_refund_folio) between 1 and 60)),
  add column card_refund_captured_by bigint;
alter table order_refunds add constraint order_refunds_folio_captured_by_fkey
  foreign key (company_id, card_refund_captured_by) references users (company_id, id) on delete restrict;
alter table order_refunds add constraint order_refunds_folio_shape
  check ((card_refund_folio is null) = (card_refund_captured_by is null));

-- 8. ARQUEO POR TERMINAL AL CIERRE (punto 9).
create table session_terminal_counts (
  company_id       bigint not null default nullif(current_setting('app.company_id', true), '')::bigint
                   references companies(id) on delete cascade,
  session_id       bigint not null,
  card_terminal_id bigint not null,
  terminal_name    text not null,
  expected         numeric(10,2) not null check (expected >= 0),
  declared         numeric(10,2) not null check (declared >= 0),
  created_by       bigint not null,
  created_at       timestamptz not null default now(),
  primary key (session_id, card_terminal_id),
  constraint session_terminal_counts_session_fkey
    foreign key (company_id, session_id) references register_sessions (company_id, id) on delete restrict,
  constraint session_terminal_counts_terminal_fkey
    foreign key (company_id, card_terminal_id) references card_terminals (company_id, id) on delete restrict,
  constraint session_terminal_counts_user_fkey
    foreign key (company_id, created_by) references users (company_id, id) on delete restrict
);
create index session_terminal_counts_company_session on session_terminal_counts (company_id, session_id);

alter table session_terminal_counts enable row level security;
create policy tenant_isolation on session_terminal_counts
  using (company_id = nullif(current_setting('app.company_id', true), '')::bigint)
  with check (company_id = nullif(current_setting('app.company_id', true), '')::bigint);
grant select, insert on session_terminal_counts to gatobobah_app;

-- 9. RESUMEN DIARIO POR CORREO (punto 7, destinatarios decididos el 2026-10-09): las direcciones
--    viven en el ajuste del negocio, y un envío por empresa y día de negocio. La fila se escribe
--    ANTES de mandar, así un reintento o una segunda caja no manda dos veces.
alter table business_settings add column daily_summary_emails text[] not null default '{}'
  check (cardinality(daily_summary_emails) <= 10);

create table daily_summary_sends (
  id            bigint generated always as identity primary key,
  company_id    bigint not null default nullif(current_setting('app.company_id', true), '')::bigint
                references companies(id) on delete cascade,
  business_date date not null,
  status        text not null default 'pendiente' check (status in ('pendiente', 'enviado', 'fallido')),
  attempts      int not null default 0 check (attempts >= 0),
  last_error    text,
  sent_at       timestamptz,
  created_at    timestamptz not null default now(),
  constraint daily_summary_sends_once unique (company_id, business_date)
);

alter table daily_summary_sends enable row level security;
create policy tenant_isolation on daily_summary_sends
  using (company_id = nullif(current_setting('app.company_id', true), '')::bigint)
  with check (company_id = nullif(current_setting('app.company_id', true), '')::bigint);
grant select, insert, update on daily_summary_sends to gatobobah_app;

-- +goose Down
-- Falla en vez de borrar lo que ya registró dinero: reversos, cobros con terminal, folios y
-- conteos por terminal son hechos de caja.
-- +goose StatementBegin
do $$
begin
  if exists (select 1 from register_cash_movements where kind = 'reverso' or concept_id is not null)
     or exists (select 1 from order_payments where card_terminal_id is not null)
     or exists (select 1 from order_refunds where card_refund_folio is not null)
     or exists (select 1 from session_terminal_counts) then
    raise exception 'hay movimientos, cobros o conteos que dependen de esta migración';
  end if;
end $$;
-- +goose StatementEnd
drop table daily_summary_sends;
alter table business_settings drop column daily_summary_emails;
drop table session_terminal_counts;
alter table order_refunds drop constraint order_refunds_folio_shape;
alter table order_refunds drop constraint order_refunds_folio_captured_by_fkey;
alter table order_refunds drop column card_refund_captured_by, drop column card_refund_folio;
drop index order_payments_company_terminal;
alter table order_payments drop constraint order_payments_card_terminal_shape;
alter table order_payments drop constraint order_payments_card_terminal_fkey;
alter table order_payments drop column card_terminal_name, drop column card_terminal_id;
drop table card_terminals;
alter table branches drop column card_count_mode;
alter table register_sessions drop constraint register_sessions_other_needs_note;
alter table register_sessions drop column card_count_mode, drop column opening_reason_note, drop column opening_reason;
alter table expenses drop column document_date;
drop index register_cash_movements_company_concept;
drop index register_cash_movements_one_reverse;
alter table register_cash_movements drop constraint register_cash_movements_reverse_shape;
alter table register_cash_movements drop constraint register_cash_movements_reverses_fkey;
alter table register_cash_movements drop constraint register_cash_movements_concept_fkey;
alter table register_cash_movements drop column reverses_id, drop column concept_id;
alter table register_cash_movements drop constraint register_cash_movements_kind_check;
alter table register_cash_movements add constraint register_cash_movements_kind_check
  check (kind in ('entrada', 'salida', 'propina'));
drop table cash_concepts;
drop index suppliers_tenant_key;
drop index expense_categories_tenant_key;
