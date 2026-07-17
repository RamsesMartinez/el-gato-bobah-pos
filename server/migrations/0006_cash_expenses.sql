-- +goose Up
create type session_status as enum ('abierta', 'cerrada');

create table register_sessions (
  id           bigint generated always as identity primary key,
  business_date date not null,
  status       session_status not null default 'abierta',
  opening_cash numeric(10,2) not null default 0,
  opened_by    bigint not null references users(id),
  opened_at    timestamptz not null default now(),
  closed_by    bigint references users(id),
  closed_at    timestamptz,
  notes        text
);
create unique index one_open_session on register_sessions (status) where status = 'abierta';

create table expenses (
  id                  bigint generated always as identity primary key,
  expense_date        date not null,
  category_id         bigint not null references expense_categories(id),
  supplier_id         bigint references suppliers(id),
  amount              numeric(10,2) not null check (amount > 0),
  payment_method_id   smallint references payment_methods(id),
  register_session_id bigint references register_sessions(id),
  description         text,
  created_by          bigint not null references users(id),
  created_at          timestamptz not null default now()
);
create index expenses_date on expenses (expense_date);

create table register_session_totals (
  session_id        bigint not null references register_sessions(id) on delete cascade,
  payment_method_id smallint not null references payment_methods(id),
  expected          numeric(10,2) not null,
  declared          numeric(10,2) not null,
  difference        numeric(10,2) generated always as (declared - expected) stored,
  primary key (session_id, payment_method_id)
);

create table register_cash_movements (
  id         bigint generated always as identity primary key,
  session_id bigint not null references register_sessions(id),
  kind       text not null check (kind in ('entrada', 'salida')),
  amount     numeric(10,2) not null check (amount > 0),
  concept    text not null,
  expense_id bigint references expenses(id),
  user_id    bigint not null references users(id),
  created_at timestamptz not null default now()
);

-- +goose Down
drop table if exists register_cash_movements;
drop table if exists register_session_totals;
drop table if exists expenses;
drop table if exists register_sessions;
drop type if exists session_status;
