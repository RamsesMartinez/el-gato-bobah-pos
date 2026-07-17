-- +goose Up

-- updated_at automático
-- +goose StatementBegin
create or replace function set_updated_at() returns trigger as $$
begin
  new.updated_at = now();
  return new;
end;
$$ language plpgsql;
-- +goose StatementEnd

create trigger trg_users_updated    before update on users      for each row execute function set_updated_at();
create trigger trg_ingredients_updated before update on ingredients for each row execute function set_updated_at();
create trigger trg_recipes_updated   before update on recipes    for each row execute function set_updated_at();
create trigger trg_products_updated  before update on products   for each row execute function set_updated_at();
create trigger trg_orders_updated    before update on orders     for each row execute function set_updated_at();

-- stock_levels = suma del ledger, mantenido por trigger (UPDATE de una fila, rápido)
-- +goose StatementBegin
create or replace function apply_stock_movement() returns trigger as $$
begin
  if new.item_type = 'ingrediente' then
    insert into stock_levels (item_type, ingredient_id, on_hand, updated_at)
      values ('ingrediente', new.ingredient_id, new.quantity, now())
    on conflict (ingredient_id) do update
      set on_hand = stock_levels.on_hand + new.quantity, updated_at = now();
  else
    insert into stock_levels (item_type, product_id, on_hand, updated_at)
      values ('producto', new.product_id, new.quantity, now())
    on conflict (product_id) do update
      set on_hand = stock_levels.on_hand + new.quantity, updated_at = now();
  end if;
  return new;
end;
$$ language plpgsql;
-- +goose StatementEnd

create trigger trg_stock_movement after insert on stock_movements
  for each row execute function apply_stock_movement();

-- categorías máximo 2 niveles: el padre no puede tener padre
-- +goose StatementBegin
create or replace function check_category_depth() returns trigger as $$
begin
  if new.parent_id is not null then
    if exists (select 1 from categories where id = new.parent_id and parent_id is not null) then
      raise exception 'las categorías solo admiten 2 niveles';
    end if;
  end if;
  return new;
end;
$$ language plpgsql;
-- +goose StatementEnd

create trigger trg_category_depth before insert or update on categories
  for each row execute function check_category_depth();

-- recipe_items: la unidad debe ser del mismo kind que la base del ingrediente
-- +goose StatementBegin
create or replace function check_recipe_unit_kind() returns trigger as $$
declare
  base_kind unit_kind;
  item_kind unit_kind;
begin
  select k.kind into base_kind from ingredients i join units k on k.id = i.base_unit_id where i.id = new.ingredient_id;
  select kind into item_kind from units where id = new.unit_id;
  if base_kind <> item_kind then
    raise exception 'la unidad de la receta (%) no coincide con la base del ingrediente (%)', item_kind, base_kind;
  end if;
  return new;
end;
$$ language plpgsql;
-- +goose StatementEnd

create trigger trg_recipe_unit_kind before insert or update on recipe_items
  for each row execute function check_recipe_unit_kind();

-- +goose Down
drop trigger if exists trg_recipe_unit_kind on recipe_items;
drop function if exists check_recipe_unit_kind();
drop trigger if exists trg_category_depth on categories;
drop function if exists check_category_depth();
drop trigger if exists trg_stock_movement on stock_movements;
drop function if exists apply_stock_movement();
drop trigger if exists trg_orders_updated on orders;
drop trigger if exists trg_products_updated on products;
drop trigger if exists trg_recipes_updated on recipes;
drop trigger if exists trg_ingredients_updated on ingredients;
drop trigger if exists trg_users_updated on users;
drop function if exists set_updated_at();
