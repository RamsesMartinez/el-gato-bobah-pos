-- +goose Up
create type order_status as enum ('abierta', 'lista', 'entregada', 'cancelada');
create type service_type as enum ('mostrador', 'para_llevar', 'domicilio');

create table order_counters (
  business_date date primary key,
  last_number   int not null
);

create table orders (
  id                   bigint generated always as identity primary key,
  client_uuid          uuid not null unique,
  business_date        date not null,
  daily_number         int not null,
  status               order_status not null default 'abierta',
  service_type         service_type not null,
  delivery_platform_id smallint references delivery_platforms(id),
  customer_name        text,
  notes                text,
  register_session_id  bigint references register_sessions(id),
  opened_by            bigint not null references users(id),
  subtotal             numeric(10,2) not null default 0,
  discount_total       numeric(10,2) not null default 0,
  total                numeric(10,2) not null default 0,
  opened_at            timestamptz not null default now(),
  ready_at             timestamptz,
  completed_at         timestamptz,
  cancelled_at         timestamptz,
  cancelled_by         bigint references users(id),
  cancel_reason        text,
  updated_at           timestamptz not null default now(),
  unique (business_date, daily_number),
  check (service_type = 'domicilio' or delivery_platform_id is null),
  check ((status = 'cancelada') = (cancelled_at is not null and cancelled_by is not null and cancel_reason is not null))
);
create index orders_board on orders (service_type, status) where status in ('abierta', 'lista');
create index orders_date_status on orders (business_date, status);

create table order_lines (
  id              bigint generated always as identity primary key,
  order_id        bigint not null references orders(id) on delete cascade,
  product_id      bigint not null references products(id),
  parent_line_id  bigint references order_lines(id),
  product_name    text not null,                    -- snapshot
  quantity        numeric(8,2) not null check (quantity > 0),
  unit_price      numeric(10,2) not null,           -- snapshot
  modifiers_total numeric(10,2) not null default 0,
  unit_cost       numeric(12,4) not null default 0, -- snapshot (utilidad histórica)
  line_total      numeric(10,2) not null,
  notes           text,
  cancelled_at    timestamptz,
  cancelled_by    bigint references users(id),
  cancel_reason   text,
  created_at      timestamptz not null default now()
);
create index order_lines_order on order_lines (order_id);
create index order_lines_product on order_lines (product_id, created_at desc);

create table order_line_modifiers (
  id                 bigint generated always as identity primary key,
  order_line_id      bigint not null references order_lines(id) on delete cascade,
  modifier_option_id bigint not null references modifier_options(id),
  group_title        text not null,       -- snapshot
  option_name        text not null,       -- snapshot
  quantity           smallint not null default 1 check (quantity > 0),
  price_delta        numeric(10,2) not null,  -- snapshot por unidad
  unit_cost          numeric(12,4) not null default 0  -- snapshot
);
create index olm_line on order_line_modifiers (order_line_id);

create table order_payments (
  id                  bigint generated always as identity primary key,
  order_id            bigint not null references orders(id),
  payment_method_id   smallint not null references payment_methods(id),
  amount              numeric(10,2) not null check (amount > 0),
  tip_amount          numeric(10,2) not null default 0 check (tip_amount >= 0),
  register_session_id bigint references register_sessions(id),
  received_by         bigint references users(id),
  reference           text,
  created_at          timestamptz not null default now()
);
create index order_payments_order on order_payments (order_id);
create index order_payments_session on order_payments (register_session_id, payment_method_id);

-- +goose Down
drop table if exists order_payments;
drop table if exists order_line_modifiers;
drop table if exists order_lines;
drop table if exists orders;
drop table if exists order_counters;
drop type if exists service_type;
drop type if exists order_status;
