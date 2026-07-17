-- +goose Up
create type unit_kind as enum ('masa', 'volumen', 'pieza');

create table units (
  id      smallint generated always as identity primary key,
  code    text not null unique,          -- 'g','kg','ml','l','floz','cda','pieza'
  name    text not null,
  kind    unit_kind not null,
  to_base numeric(16,6) not null check (to_base > 0)  -- factor a la base del kind
);

create table suppliers (
  id         bigint generated always as identity primary key,
  name       citext not null unique,
  phone      text,
  notes      text,
  is_active  boolean not null default true,
  created_at timestamptz not null default now()
);

create table channels (
  id   smallint generated always as identity primary key,
  code text not null unique,             -- 'pos','qr','online'
  name text not null
);

create type payment_kind as enum ('efectivo', 'tarjeta', 'transferencia', 'plataforma', 'otro');

create table payment_methods (
  id                  smallint generated always as identity primary key,
  name                citext not null unique,
  kind                payment_kind not null,
  affects_cash_drawer boolean not null default false,
  is_active           boolean not null default true,
  sort_key            numeric(18,9) not null default 1000
);

create table delivery_platforms (
  id        smallint generated always as identity primary key,
  name      citext not null unique,      -- Uber Eats, Didi, Rappi, Propio
  is_active boolean not null default true
);

create type financial_group as enum ('operacional', 'administrativo', 'otro');

create table expense_categories (
  id              bigint generated always as identity primary key,
  name            citext not null,
  financial_group financial_group not null,
  is_active       boolean not null default true,
  unique (financial_group, name)
);

-- +goose Down
drop table if exists expense_categories;
drop type if exists financial_group;
drop table if exists delivery_platforms;
drop table if exists payment_methods;
drop type if exists payment_kind;
drop table if exists channels;
drop table if exists suppliers;
drop table if exists units;
drop type if exists unit_kind;
