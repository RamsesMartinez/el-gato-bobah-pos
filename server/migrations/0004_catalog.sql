-- +goose Up
create table categories (
  id        bigint generated always as identity primary key,
  name      citext not null,
  parent_id bigint references categories(id),
  sort_key  numeric(18,9) not null default 1000,
  color     text,
  image_url text,
  is_active boolean not null default true
);
create unique index categories_name_scope on categories (coalesce(parent_id, 0), name);

create table category_channels (
  category_id bigint not null references categories(id) on delete cascade,
  channel_id  smallint not null references channels(id),
  visible     boolean not null,
  primary key (category_id, channel_id)
);

create type product_type as enum ('simple', 'combo');

create table products (
  id             bigint generated always as identity primary key,
  sku            text unique,
  name           citext not null unique,
  description    text,
  type           product_type not null default 'simple',
  category_id    bigint not null references categories(id),
  price          numeric(10,2) not null check (price >= 0),
  cost_source    cost_source not null default 'manual',
  manual_cost    numeric(12,4),
  current_cost   numeric(12,4) not null default 0,
  margin_amount  numeric(12,4) generated always as (price - current_cost) stored,
  recipe_id      bigint unique references recipes(id),
  track_stock    boolean not null default false,
  allow_oversell boolean not null default true,
  min_stock      numeric(14,4),
  is_favorite    boolean not null default false,
  sort_key       numeric(18,9) not null default 1000,
  image_url      text,
  is_active      boolean not null default true,
  created_at     timestamptz not null default now(),
  updated_at     timestamptz not null default now(),
  check (type <> 'combo' or recipe_id is null),
  check (not track_stock or recipe_id is null)
);
create index products_category on products (category_id) where is_active;
create index products_favorite on products (is_favorite) where is_favorite;

create type channel_visibility as enum ('visible', 'oculto');
create table product_channels (
  product_id bigint not null references products(id) on delete cascade,
  channel_id smallint not null references channels(id),
  visibility channel_visibility not null,
  primary key (product_id, channel_id)
);

create table combo_slots (
  id         bigint generated always as identity primary key,
  combo_id   bigint not null references products(id) on delete cascade,
  name       text not null,
  min_select smallint not null default 1,
  max_select smallint not null default 1 check (max_select >= min_select),
  position   int not null default 0
);
create table combo_slot_products (
  slot_id     bigint not null references combo_slots(id) on delete cascade,
  product_id  bigint not null references products(id),
  price_delta numeric(10,2) not null default 0,
  is_default  boolean not null default false,
  primary key (slot_id, product_id)
);

-- +goose Down
drop table if exists combo_slot_products;
drop table if exists combo_slots;
drop table if exists product_channels;
drop type if exists channel_visibility;
drop table if exists products;
drop type if exists product_type;
drop table if exists category_channels;
drop table if exists categories;
