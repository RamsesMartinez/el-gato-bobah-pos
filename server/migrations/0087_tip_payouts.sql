-- +goose Up
-- Entrega de propinas desde la caja (spec 032, decisiones del dueño del 2026-10-09, puntos 1 y 2).
--
-- La propina entregada es una SALIDA DE EFECTIVO del cajón, pero no es gasto ni venta: es dinero del
-- personal que pasa por la caja. Por eso vive como movimiento de caja de tipo `propina` (el arqueo
-- ya resta toda salida vía NetCashMovements) y NO como gasto (que es lo que hoy descuadra el corte).

-- 1. Tipo nuevo de movimiento y su forma: una propina siempre dice a quién se le dio, con el nombre
--    copiado (snapshot) para que renombrar al usuario no reescriba un corte ya firmado.
alter table register_cash_movements drop constraint register_cash_movements_kind_check;
alter table register_cash_movements add constraint register_cash_movements_kind_check
  check (kind in ('entrada', 'salida', 'propina'));

alter table register_cash_movements
  add column recipient_user_id bigint,
  add column recipient_name    text;

-- FK compuesta: los chequeos de integridad referencial saltan RLS, y una simple aceptaría entregarle
-- la propina a un usuario de otra empresa. `restrict`: quien recibió dinero no se borra por arrastre.
alter table register_cash_movements add constraint register_cash_movements_recipient_fkey
  foreign key (company_id, recipient_user_id) references users (company_id, id) on delete restrict;

alter table register_cash_movements add constraint register_cash_movements_tip_shape check (
  (kind = 'propina') = (recipient_user_id is not null)
  and (recipient_user_id is null) = (recipient_name is null)
  and (recipient_name is null or length(btrim(recipient_name)) between 1 and 120)
);

-- Destino de la FK compuesta de abajo.
create unique index register_cash_movements_tenant_key on register_cash_movements (company_id, id);

-- 2. De qué cobros salió cada entrega. Es lo que permite que una devolución posterior se refleje en
--    el pendiente y que el corte diga cuánta propina de tarjeta se pagó con efectivo del cajón.
--    order_payment_id nulo = lo heredado del turno anterior de la misma caja.
create table tip_payout_sources (
  id               bigint generated always as identity primary key,
  company_id       bigint not null default nullif(current_setting('app.company_id', true), '')::bigint
                   references companies(id) on delete cascade,
  movement_id      bigint not null,
  order_payment_id bigint,
  amount           numeric(10,2) not null check (amount > 0),
  constraint tip_payout_sources_movement_fkey
    foreign key (company_id, movement_id) references register_cash_movements (company_id, id) on delete restrict,
  -- El orden de columnas sigue al de `order_payments_id_company_key (id, company_id)` (0079):
  -- Postgres empareja por posición.
  constraint tip_payout_sources_payment_fkey
    foreign key (order_payment_id, company_id) references order_payments (id, company_id) on delete restrict
);
create index tip_payout_sources_company_payment on tip_payout_sources (company_id, order_payment_id);
create index tip_payout_sources_company_movement on tip_payout_sources (company_id, movement_id);
create index register_cash_movements_company_session_kind on register_cash_movements (company_id, session_id, kind);

alter table tip_payout_sources enable row level security;
create policy tenant_isolation on tip_payout_sources
  using (company_id = nullif(current_setting('app.company_id', true), '')::bigint)
  with check (company_id = nullif(current_setting('app.company_id', true), '')::bigint);
-- Sin update ni delete: una entrega no se edita.
grant select, insert on tip_payout_sources to gatobobah_app;

-- 3. Lo que se quedó en caja al cerrar. El siguiente turno de la MISMA caja lo hereda como pendiente
--    (por caja y no por empresa: puerta de «más de una caja», constitución VIII).
alter table register_sessions
  add column tips_carried_over numeric(10,2) not null default 0
  constraint register_sessions_tips_carried_over_check check (tips_carried_over >= 0);

-- 4. Qué se quedó en caja, COBRO POR COBRO. Un solo número perdía dos hechos: de qué medio era
--    (la propina de tarjeta heredada se entregaba como si fuera de efectivo y el corte dejaba de
--    decir «propina de tarjeta pagada en efectivo») y de qué pedido (una devolución posterior al
--    cierre no podía bajar lo heredado). `tips_carried_over` queda como total para mostrar.
create table tip_carryovers (
  id               bigint generated always as identity primary key,
  company_id       bigint not null default nullif(current_setting('app.company_id', true), '')::bigint
                   references companies(id) on delete cascade,
  session_id       bigint not null,
  order_payment_id bigint not null,
  amount           numeric(10,2) not null check (amount > 0),
  constraint tip_carryovers_unique unique (session_id, order_payment_id),
  constraint tip_carryovers_session_fkey
    foreign key (company_id, session_id) references register_sessions (company_id, id) on delete restrict,
  constraint tip_carryovers_payment_fkey
    foreign key (order_payment_id, company_id) references order_payments (id, company_id) on delete restrict
);
create index tip_carryovers_company_session on tip_carryovers (company_id, session_id);
create index tip_carryovers_company_payment on tip_carryovers (company_id, order_payment_id);

alter table tip_carryovers enable row level security;
create policy tenant_isolation on tip_carryovers
  using (company_id = nullif(current_setting('app.company_id', true), '')::bigint)
  with check (company_id = nullif(current_setting('app.company_id', true), '')::bigint);
grant select, insert on tip_carryovers to gatobobah_app;

-- +goose Down
-- Falla en vez de borrar: una propina entregada es dinero que salió del cajón, y quitarla haría que
-- el corte de ese turno esperara dinero que ya no está.
-- +goose StatementBegin
do $$
begin
  if exists (select 1 from register_cash_movements where kind = 'propina')
     or exists (select 1 from register_sessions where tips_carried_over > 0) then
    raise exception 'hay propinas entregadas o heredadas: revertir perdería su rastro';
  end if;
end $$;
-- +goose StatementEnd
drop table tip_carryovers;
alter table register_sessions drop column tips_carried_over;
drop index if exists register_cash_movements_company_session_kind;
drop table tip_payout_sources;
drop index register_cash_movements_tenant_key;
alter table register_cash_movements drop constraint register_cash_movements_tip_shape;
alter table register_cash_movements drop constraint register_cash_movements_recipient_fkey;
alter table register_cash_movements drop column recipient_name, drop column recipient_user_id;
alter table register_cash_movements drop constraint register_cash_movements_kind_check;
alter table register_cash_movements add constraint register_cash_movements_kind_check
  check (kind in ('entrada', 'salida'));
