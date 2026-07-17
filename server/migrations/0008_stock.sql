-- +goose Up
create type stock_item_type as enum ('ingrediente', 'producto');
create type stock_movement_type as enum
  ('venta', 'compra', 'ajuste', 'merma', 'produccion', 'cancelacion');

create table stock_movements (
  id            bigint generated always as identity primary key,
  item_type     stock_item_type not null,
  ingredient_id bigint references ingredients(id),
  product_id    bigint references products(id),
  movement_type stock_movement_type not null,
  quantity      numeric(14,4) not null check (quantity <> 0),  -- delta firmado, base units
  unit_cost     numeric(12,6),
  order_id      bigint references orders(id),
  expense_id    bigint references expenses(id),
  user_id       bigint references users(id),
  reason        text,
  note          text,
  created_at    timestamptz not null default now(),
  check ((item_type = 'ingrediente') = (ingredient_id is not null)),
  check ((item_type = 'producto') = (product_id is not null))
);
create index sm_ingredient on stock_movements (ingredient_id, created_at desc);
create index sm_product on stock_movements (product_id, created_at desc);
create index sm_order on stock_movements (order_id);
create index sm_type on stock_movements (movement_type, created_at desc);

create table stock_levels (
  item_type     stock_item_type not null,
  ingredient_id bigint unique references ingredients(id),
  product_id    bigint unique references products(id),
  on_hand       numeric(14,4) not null default 0,
  updated_at    timestamptz not null default now(),
  check ((item_type = 'ingrediente') = (ingredient_id is not null)),
  check ((item_type = 'producto') = (product_id is not null))
);

-- +goose Down
drop table if exists stock_levels;
drop table if exists stock_movements;
drop type if exists stock_movement_type;
drop type if exists stock_item_type;
