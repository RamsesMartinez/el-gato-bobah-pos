-- +goose Up
create table recipes (
  id         bigint generated always as identity primary key,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);

create table ingredient_categories (
  id        bigint generated always as identity primary key,
  name      citext not null unique,
  is_active boolean not null default true
);

create type cost_source as enum ('manual', 'compra', 'receta');

create table ingredients (
  id           bigint generated always as identity primary key,
  name         citext not null unique,
  category_id  bigint references ingredient_categories(id),
  base_unit_id smallint not null references units(id),
  is_prep      boolean not null default false,        -- subingrediente preparado
  recipe_id    bigint unique references recipes(id),  -- solo cuando is_prep
  yield_qty    numeric(14,4),                         -- rendimiento del lote (base units)
  waste_pct    numeric(5,2) not null default 0 check (waste_pct >= 0 and waste_pct < 100), -- merma %
  current_cost numeric(12,6) not null default 0,      -- por base unit (cache, motor de costeo)
  cost_source  cost_source not null default 'manual',
  supplier_id  bigint references suppliers(id),
  is_packaging boolean not null default false,
  track_stock  boolean not null default true,
  min_stock    numeric(14,4),
  is_active    boolean not null default true,
  created_at   timestamptz not null default now(),
  updated_at   timestamptz not null default now(),
  check (not is_prep or (recipe_id is not null and yield_qty > 0))
);

create table recipe_items (
  id            bigint generated always as identity primary key,
  recipe_id     bigint not null references recipes(id) on delete cascade,
  ingredient_id bigint not null references ingredients(id),
  quantity      numeric(14,4) not null check (quantity > 0),
  unit_id       smallint not null references units(id),
  position      int not null default 0,
  unique (recipe_id, ingredient_id)
);
create index recipe_items_ingredient on recipe_items (ingredient_id);

create table ingredient_purchase_formats (
  id            bigint generated always as identity primary key,
  ingredient_id bigint not null references ingredients(id) on delete cascade,
  name          text not null,                 -- 'Frasco 870 g', 'Barra 20 rebanadas'
  qty_in_base   numeric(14,4) not null check (qty_in_base > 0),
  last_cost     numeric(10,2),
  supplier_id   bigint references suppliers(id),
  is_default    boolean not null default false
);

-- +goose Down
drop table if exists ingredient_purchase_formats;
drop table if exists recipe_items;
drop table if exists ingredients;
drop type if exists cost_source;
drop table if exists ingredient_categories;
drop table if exists recipes;
