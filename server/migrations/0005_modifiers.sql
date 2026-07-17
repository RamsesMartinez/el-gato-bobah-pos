-- +goose Up
create table modifier_groups (
  id        bigint generated always as identity primary key,
  name      citext not null unique,          -- 'Perlas explosivas'
  is_active boolean not null default true
);

create table modifier_options (
  id                bigint generated always as identity primary key,
  group_id          bigint not null references modifier_groups(id) on delete cascade,
  name              text not null,
  price_delta       numeric(10,2) not null default 0,       -- puede ser negativo
  recipe_id         bigint unique references recipes(id),   -- la opción descuenta stock vía receta
  linked_product_id bigint references products(id),         -- o la opción es un producto con stock directo
  max_per_line      smallint not null default 1,
  current_cost      numeric(12,4) not null default 0,
  sort_key          numeric(18,9) not null default 1000,
  is_active         boolean not null default true,
  unique (group_id, name),
  check (recipe_id is null or linked_product_id is null)
);

create table product_modifier_groups (
  id         bigint generated always as identity primary key,
  product_id bigint not null references products(id) on delete cascade,
  group_id   bigint not null references modifier_groups(id),
  title      text,
  min_select smallint not null default 0,
  max_select smallint not null default 1 check (max_select >= min_select),
  position   int not null default 0,
  unique (product_id, group_id)
);
create index pmg_product on product_modifier_groups (product_id);

-- +goose Down
drop table if exists product_modifier_groups;
drop table if exists modifier_options;
drop table if exists modifier_groups;
