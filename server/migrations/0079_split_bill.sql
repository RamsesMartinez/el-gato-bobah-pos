-- +goose Up

-- DIVIDIR LA CUENTA POR PRODUCTOS (spec 027).
--
-- El número es de trabajo: toma el siguiente libre de develop al fusionar (goose corre sin
-- AllowMissing). Cuatro cosas:
--
-- 1. QUÉ CUBRIÓ CADA PAGO (`order_payment_lines`). Es el único hecho de esta feature que no se
--    registra hoy y que no se reconstruye después: qué productos y cuántas piezas pagó cada persona.
-- 2. LOS PAGOS DEVUELTOS SALEN DE `order_payments` a una bitácora (`order_payment_voids`). Así las
--    ~32 consultas que suman pagos quedan correctas sin tocarlas: un pago devuelto en su mismo turno
--    es, para el cajón, un pago que no ocurrió. La bitácora es la única copia de ese dinero.
-- 3. QUÉ SE PASÓ DE UN PEDIDO A OTRO (`order_line_move_batches`, `order_line_moves`), con el lote
--    como llave de idempotencia: un renglón partido tiene id nuevo en cada intento.
-- 4. EL PAGO RECUERDA SU PARTE Y SU NÚMERO, y el pedido con el que se juntó otro.
--
-- Los chequeos de FK saltan RLS: toda referencia a renglón, pago o pedido lleva la empresa en la
-- llave, para que una fila de una empresa no pueda colgarse de una fila de otra. Las FKs van
-- `no action` y no `restrict`: `restrict` no se difiere y aborta el borrado en cascada de una empresa.
--
-- ROLLBACK: el Down se niega en cuanto haya un solo cobro nuevo (todo pago nace con número, y el
-- ticket impreso lo lleva). Tras desplegar, deshacer esta migración es restaurar el respaldo.

set local lock_timeout = '3s';

-- order_lines ya tiene `unique (id, company_id)`; order_payments todavía no. users y
-- register_sessions ya tienen su `(company_id, id)`; payment_methods tampoco.
alter table order_payments add constraint order_payments_id_company_key unique (id, company_id);
create unique index payment_methods_tenant_key on payment_methods (company_id, id);

-- ---------------------------------------------------------------------------------------------
-- 1. COBERTURA
-- ---------------------------------------------------------------------------------------------
create table order_payment_lines (
  id               bigint generated always as identity primary key,
  company_id       bigint not null default nullif(current_setting('app.company_id', true), '')::bigint
                     references companies(id) on delete cascade,
  order_payment_id bigint not null,
  order_line_id    bigint not null,
  qty              numeric(8,2) not null check (qty > 0),
  -- Lo pagado de ese renglón, ya con su parte de descuento. La suma por pago puede quedar bajo el
  -- monto del pago: la diferencia es lo cubierto sin producto (envío, redondeo).
  amount           numeric(10,2) not null check (amount >= 0),
  -- En cascada: si el pago se devuelve, su cobertura se va con él (la copia queda en la bitácora).
  constraint order_payment_lines_payment foreign key (order_payment_id, company_id)
    references order_payments (id, company_id) on delete cascade,
  constraint order_payment_lines_line foreign key (order_line_id, company_id)
    references order_lines (id, company_id) on delete no action,
  constraint order_payment_lines_once unique (order_payment_id, order_line_id)
);
create index order_payment_lines_by_line on order_payment_lines (company_id, order_line_id);

-- ---------------------------------------------------------------------------------------------
-- 2. BITÁCORA DE PAGOS DEVUELTOS
-- ---------------------------------------------------------------------------------------------
create table order_payment_voids (
  id                  bigint generated always as identity primary key,
  company_id          bigint not null default nullif(current_setting('app.company_id', true), '')::bigint
                        references companies(id) on delete cascade,
  order_id            bigint not null,
  -- Sin FK: la fila ya no existe. Única por empresa: un pago se devuelve una sola vez.
  original_payment_id bigint not null,
  payment_number      smallint not null check (payment_number > 0),
  payment_method_id   smallint not null,
  amount              numeric(10,2) not null check (amount > 0),
  tip_amount          numeric(10,2) not null check (tip_amount >= 0),
  reference           text,
  -- Nunca nula aquí: solo se devuelven pagos de un turno abierto.
  register_session_id bigint not null,
  received_by         bigint,
  paid_at             timestamptz not null,
  client_uuid         uuid,
  split_part          smallint,
  split_of            smallint,
  covered             jsonb not null default '[]',
  voided_by           bigint not null,
  voided_at           timestamptz not null default now(),
  reason              text not null check (length(trim(reason)) > 0),
  constraint order_payment_voids_order foreign key (order_id, company_id)
    references orders (id, company_id) on delete no action,
  constraint order_payment_voids_method foreign key (company_id, payment_method_id)
    references payment_methods (company_id, id) on delete no action,
  constraint order_payment_voids_session foreign key (company_id, register_session_id)
    references register_sessions (company_id, id) on delete no action,
  constraint order_payment_voids_received_by foreign key (company_id, received_by)
    references users (company_id, id) on delete no action,
  constraint order_payment_voids_voided_by foreign key (company_id, voided_by)
    references users (company_id, id) on delete no action,
  constraint order_payment_voids_once unique (company_id, original_payment_id)
);
create unique index order_payment_voids_idem on order_payment_voids (company_id, client_uuid)
  where client_uuid is not null;
create index order_payment_voids_by_session on order_payment_voids (company_id, register_session_id);
create index order_payment_voids_by_order on order_payment_voids (company_id, order_id);

-- ---------------------------------------------------------------------------------------------
-- 3. PASAR PRODUCTOS
-- ---------------------------------------------------------------------------------------------
create table order_line_move_batches (
  company_id    bigint not null default nullif(current_setting('app.company_id', true), '')::bigint
                  references companies(id) on delete cascade,
  client_uuid   uuid not null,
  from_order_id bigint not null,
  to_order_id   bigint not null,
  moved_by      bigint not null,
  moved_at      timestamptz not null default now(),
  primary key (company_id, client_uuid),
  constraint order_line_move_batches_from foreign key (from_order_id, company_id)
    references orders (id, company_id) on delete no action,
  constraint order_line_move_batches_to foreign key (to_order_id, company_id)
    references orders (id, company_id) on delete no action,
  constraint order_line_move_batches_moved_by foreign key (company_id, moved_by)
    references users (company_id, id) on delete no action,
  constraint order_line_move_batches_two_orders check (from_order_id <> to_order_id)
);
create index order_line_move_batches_from_order on order_line_move_batches (company_id, from_order_id);
create index order_line_move_batches_to_order on order_line_move_batches (company_id, to_order_id);

create table order_line_moves (
  id                 bigint generated always as identity primary key,
  company_id         bigint not null default nullif(current_setting('app.company_id', true), '')::bigint
                       references companies(id) on delete cascade,
  client_uuid        uuid not null,
  -- El renglón que viajó: el nuevo, si se partió.
  order_line_id      bigint not null,
  -- El original, cuando se pasaron solo algunas piezas.
  split_from_line_id bigint,
  qty                numeric(8,2) not null check (qty > 0),
  constraint order_line_moves_batch foreign key (company_id, client_uuid)
    references order_line_move_batches (company_id, client_uuid) on delete no action,
  constraint order_line_moves_line foreign key (order_line_id, company_id)
    references order_lines (id, company_id) on delete no action,
  constraint order_line_moves_split_from foreign key (split_from_line_id, company_id)
    references order_lines (id, company_id) on delete no action
);
create index order_line_moves_by_batch on order_line_moves (company_id, client_uuid);
create index order_line_moves_by_line on order_line_moves (company_id, order_line_id);
create index order_line_moves_by_split_from on order_line_moves (company_id, split_from_line_id)
  where split_from_line_id is not null;

-- ---------------------------------------------------------------------------------------------
-- 4. COLUMNAS NUEVAS. Nulas y sin default: no reescriben la tabla, y la migración no rellena nada.
-- ---------------------------------------------------------------------------------------------
alter table order_payments add column split_part smallint;
alter table order_payments add column split_of smallint;
-- El tope de partes (12) vive en domain; la base solo exige una serie de al menos dos.
alter table order_payments add constraint order_payments_split_pair
  check ((split_part is null) = (split_of is null));
alter table order_payments add constraint order_payments_split_range
  check (split_part is null or (split_of >= 2 and split_part between 1 and split_of));

-- El número del pago dentro del pedido. Los pagos de antes quedan sin número y la vista los numera
-- por hora con la misma regla; un número nuevo cuenta también los viejos, así que no se repite.
alter table order_payments add column payment_number smallint;
alter table order_payments add constraint order_payments_number_positive check (payment_number > 0);
create unique index order_payments_number_once on order_payments (order_id, payment_number)
  where payment_number is not null;

-- El pedido con el que se juntó éste al pasarle todos sus productos. Una columna y no el motivo:
-- un reporte que filtre por texto se rompe al primer cambio de redacción.
alter table orders add column merged_into_order_id bigint;
alter table orders add constraint orders_merged_into foreign key (merged_into_order_id, company_id)
  references orders (id, company_id) on delete no action;
alter table orders add constraint orders_merged_is_cancelled
  check (merged_into_order_id is null or status = 'cancelada');
alter table orders add constraint orders_merged_not_itself
  check (merged_into_order_id <> id);
create index orders_merged_into on orders (company_id, merged_into_order_id)
  where merged_into_order_id is not null;

-- ---------------------------------------------------------------------------------------------
-- 5. RLS Y GRANTS de las tablas nuevas (el grant no se hereda, 0024)
-- ---------------------------------------------------------------------------------------------
alter table order_payment_lines enable row level security;
create policy tenant_isolation on order_payment_lines
  using      (company_id = nullif(current_setting('app.company_id', true), '')::bigint)
  with check (company_id = nullif(current_setting('app.company_id', true), '')::bigint);
-- Sin delete: la cobertura se va solo con su pago, en cascada, con los privilegios del dueño.
grant select, insert on order_payment_lines to gatobobah_app;

alter table order_payment_voids enable row level security;
create policy tenant_isolation on order_payment_voids
  using      (company_id = nullif(current_setting('app.company_id', true), '')::bigint)
  with check (company_id = nullif(current_setting('app.company_id', true), '')::bigint);
grant select, insert on order_payment_voids to gatobobah_app;

alter table order_line_move_batches enable row level security;
create policy tenant_isolation on order_line_move_batches
  using      (company_id = nullif(current_setting('app.company_id', true), '')::bigint)
  with check (company_id = nullif(current_setting('app.company_id', true), '')::bigint);
grant select, insert on order_line_move_batches to gatobobah_app;

alter table order_line_moves enable row level security;
create policy tenant_isolation on order_line_moves
  using      (company_id = nullif(current_setting('app.company_id', true), '')::bigint)
  with check (company_id = nullif(current_setting('app.company_id', true), '')::bigint);
grant select, insert on order_line_moves to gatobobah_app;

-- Partir un renglón de paquete reparte lo que lleva entre las dos mitades (0078 solo dio insert).
grant update (quantity) on order_line_components to gatobobah_app;

-- +goose Down

set local lock_timeout = '3s';

-- Se niega si perdería algo sin rastro: la bitácora es la única copia de dinero devuelto, la
-- cobertura no se reconstruye, sin la marca de juntado el pedido se leería como cancelado, y sin la
-- parte o el número se pierde lo que dice el ticket impreso.
-- +goose StatementBegin
do $$
begin
  if exists (select 1 from order_payment_voids) then
    raise exception 'hay pagos devueltos en order_payment_voids: es la única copia de ese dinero';
  end if;
  if exists (select 1 from order_payment_lines) then
    raise exception 'hay pagos con productos en order_payment_lines: no se puede reconstruir qué cubrió cada uno';
  end if;
  if exists (select 1 from order_line_move_batches) or exists (select 1 from order_line_moves) then
    raise exception 'hay productos pasados de un pedido a otro: se perdería de dónde venían';
  end if;
  if exists (select 1 from orders where merged_into_order_id is not null) then
    raise exception 'hay pedidos juntados con otro: sin la marca se leerían como cancelados';
  end if;
  if exists (select 1 from order_payments where split_part is not null or payment_number is not null) then
    raise exception 'hay pagos con parte o número: se perdería lo que dice su ticket';
  end if;
end $$;
-- +goose StatementEnd

revoke update (quantity) on order_line_components from gatobobah_app;

drop table order_line_moves;
drop table order_line_move_batches;
drop table order_payment_voids;
drop table order_payment_lines;

drop index orders_merged_into;
alter table orders drop constraint orders_merged_not_itself;
alter table orders drop constraint orders_merged_is_cancelled;
alter table orders drop constraint orders_merged_into;
alter table orders drop column merged_into_order_id;

drop index order_payments_number_once;
alter table order_payments drop constraint order_payments_number_positive;
alter table order_payments drop column payment_number;
alter table order_payments drop constraint order_payments_split_range;
alter table order_payments drop constraint order_payments_split_pair;
alter table order_payments drop column split_of;
alter table order_payments drop column split_part;

drop index payment_methods_tenant_key;
alter table order_payments drop constraint order_payments_id_company_key;
