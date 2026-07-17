-- +goose Up
create extension if not exists citext;

create type user_role as enum ('admin', 'gerente', 'cajero', 'mesero');

create table users (
  id            bigint generated always as identity primary key,
  name          text not null,
  username      citext unique,
  role          user_role not null,
  pin_hash      text,
  password_hash text,
  is_active     boolean not null default true,
  created_at    timestamptz not null default now(),
  updated_at    timestamptz not null default now()
);

create table refresh_tokens (
  id         bigint generated always as identity primary key,
  user_id    bigint not null references users(id) on delete cascade,
  token_hash text not null unique,
  expires_at timestamptz not null,
  revoked_at timestamptz,
  created_at timestamptz not null default now()
);
create index refresh_tokens_user on refresh_tokens (user_id);

-- +goose Down
drop table if exists refresh_tokens;
drop table if exists users;
drop type if exists user_role;
