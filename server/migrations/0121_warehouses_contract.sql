-- +goose Up

-- VARIOS ALMACENES POR SUCURSAL, SEGUNDA SUBIDA (spec 094). Contrae lo que la 0120 dejó vivo para la
-- versión anterior de la API, que ya es la de la 0120 y escribe con el almacén:
--   * Se borran las llaves únicas por sucursal: desde aquí una sucursal puede tener existencias del
--     mismo artículo en dos almacenes. La API de la 0120 ya hace sus `on conflict` por almacén.
--   * El almacén pasa a obligatorio (check `not valid` + `validate` + `set not null`, para no tomar
--     ACCESS EXCLUSIVE durante un escaneo).
--   * Se quitan las dos compuertas de la 0120: ya se escribe en cualquier almacén y se puede cambiar
--     cuál es el de venta. Una reposición de cancelación sin almacén sí se rechaza: tiene que volver al
--     almacén del que salió, y adivinar el de venta de hoy la mandaría al equivocado (FR-011).
--
-- DESPUÉS DE ESTA MIGRACIÓN YA NO SE PUEDE VOLVER A UNA IMAGEN ANTERIOR A LA 094: su `on conflict
-- (branch_id, …)` no encontraría llave.

set local lock_timeout = '3s';

-- Una sucursal que dio de alta la API anterior a la 0120 durante la subida nació sin almacén de venta
-- (R11 M2, R13): se crea aquí antes de exigirlo.
insert into warehouses (company_id, branch_id, name, is_sales)
select b.company_id, b.id, 'Local', true
  from branches b
 where not exists (select 1 from warehouses w where w.company_id = b.company_id and w.branch_id = b.id and w.is_sales);

-- Lo que esa API escribió sin almacén durante el traslape lo llenó el trigger; esto es solo por si una
-- fila llegó por un camino sin trigger (un restore a mano).
update stock_levels t set warehouse_id = w.id from warehouses w where t.warehouse_id is null and w.company_id = t.company_id and w.branch_id = t.branch_id and w.is_sales;
update stock_movements t set warehouse_id = w.id from warehouses w where t.warehouse_id is null and w.company_id = t.company_id and w.branch_id = t.branch_id and w.is_sales;
update stock_counts t set warehouse_id = w.id from warehouses w where t.warehouse_id is null and w.company_id = t.company_id and w.branch_id = t.branch_id and w.is_sales;
update waste_records t set warehouse_id = w.id from warehouses w where t.warehouse_id is null and w.company_id = t.company_id and w.branch_id = t.branch_id and w.is_sales;
update branch_stock_minimums t set warehouse_id = w.id from warehouses w where t.warehouse_id is null and w.company_id = t.company_id and w.branch_id = t.branch_id and w.is_sales;
update notifications t set warehouse_id = w.id from warehouses w where t.warehouse_id is null and w.company_id = t.company_id and w.branch_id = t.branch_id and w.is_sales;
update stock_out_reports t set warehouse_id = w.id from warehouses w where t.warehouse_id is null and w.company_id = t.company_id and w.branch_id = t.branch_id and w.is_sales;

-- ---------------------------------------------------------------------------------------------
-- 1. EL ALMACÉN ES OBLIGATORIO
-- ---------------------------------------------------------------------------------------------
alter table stock_levels          add constraint stock_levels_warehouse_nn check (warehouse_id is not null) not valid;
alter table stock_movements       add constraint stock_movements_warehouse_nn check (warehouse_id is not null) not valid;
alter table stock_counts          add constraint stock_counts_warehouse_nn check (warehouse_id is not null) not valid;
alter table waste_records         add constraint waste_records_warehouse_nn check (warehouse_id is not null) not valid;
alter table branch_stock_minimums add constraint branch_stock_minimums_warehouse_nn check (warehouse_id is not null) not valid;
alter table notifications         add constraint notifications_warehouse_nn check (warehouse_id is not null) not valid;
alter table stock_out_reports     add constraint stock_out_reports_warehouse_nn check (warehouse_id is not null) not valid;
alter table count_places          add constraint count_places_warehouse_nn check (warehouse_id is not null) not valid;
-- Un renglón de compra sin artículo (gas, envío) no es del almacén; uno con artículo, excluido o no,
-- siempre dice a cuál (R12).
alter table expense_items add constraint expense_items_stock_line_has_warehouse
  check (item_type is null or warehouse_id is not null) not valid;

alter table stock_levels          validate constraint stock_levels_warehouse_nn;
alter table stock_movements       validate constraint stock_movements_warehouse_nn;
alter table stock_counts          validate constraint stock_counts_warehouse_nn;
alter table waste_records         validate constraint waste_records_warehouse_nn;
alter table branch_stock_minimums validate constraint branch_stock_minimums_warehouse_nn;
alter table notifications         validate constraint notifications_warehouse_nn;
alter table stock_out_reports     validate constraint stock_out_reports_warehouse_nn;
alter table count_places          validate constraint count_places_warehouse_nn;
alter table expense_items         validate constraint expense_items_stock_line_has_warehouse;

-- Con el check validado, `set not null` no vuelve a escanear la tabla.
alter table stock_levels          alter column warehouse_id set not null;
alter table stock_movements       alter column warehouse_id set not null;
alter table stock_counts          alter column warehouse_id set not null;
alter table waste_records         alter column warehouse_id set not null;
alter table branch_stock_minimums alter column warehouse_id set not null;
alter table notifications         alter column warehouse_id set not null;
alter table stock_out_reports     alter column warehouse_id set not null;
alter table count_places          alter column warehouse_id set not null;

alter table stock_levels          drop constraint stock_levels_warehouse_nn;
alter table stock_movements       drop constraint stock_movements_warehouse_nn;
alter table stock_counts          drop constraint stock_counts_warehouse_nn;
alter table waste_records         drop constraint waste_records_warehouse_nn;
alter table branch_stock_minimums drop constraint branch_stock_minimums_warehouse_nn;
alter table notifications         drop constraint notifications_warehouse_nn;
alter table stock_out_reports     drop constraint stock_out_reports_warehouse_nn;
alter table count_places          drop constraint count_places_warehouse_nn;

-- ---------------------------------------------------------------------------------------------
-- 2. SE BORRAN LAS LLAVES POR SUCURSAL
-- ---------------------------------------------------------------------------------------------
alter table stock_levels drop constraint stock_levels_ingredient_per_branch, drop constraint stock_levels_product_per_branch;
drop index branch_stock_minimums_ingredient_key;
drop index branch_stock_minimums_product_key;
drop index notifications_one_open;
drop index stock_counts_one_open;

-- ---------------------------------------------------------------------------------------------
-- 3. SIN COMPUERTAS
-- ---------------------------------------------------------------------------------------------
-- +goose StatementBegin
create or replace function fill_stock_movement_branch_id() returns trigger
language plpgsql as $$
begin
  if new.branch_id is null and new.warehouse_id is not null then
    select w.branch_id into new.branch_id
      from warehouses w where w.id = new.warehouse_id and w.company_id = new.company_id;
  end if;
  if new.branch_id is null and new.order_id is not null then
    select o.branch_id into new.branch_id
      from orders o where o.id = new.order_id and o.company_id = new.company_id;
  end if;
  if new.branch_id is null then
    new.branch_id := branch_for_company(new.company_id);
  end if;
  if new.warehouse_id is null then
    -- Una reposición vuelve al almacén del que salió; el de venta de hoy puede ser otro (FR-011).
    if new.movement_type = 'cancelacion' then
      raise exception 'warehouse_not_writable_yet: una reposición de cancelación va al almacén del que salió'
        using errcode = 'EGB04';
    end if;
    new.warehouse_id := sales_warehouse_of(new.company_id, new.branch_id);
  end if;
  return new;
end;
$$;
-- +goose StatementEnd

-- +goose StatementBegin
create or replace function warehouses_before_update() returns trigger
language plpgsql as $$
begin
  if new.company_id <> old.company_id or new.branch_id <> old.branch_id then
    raise exception 'un almacén no cambia de sucursal' using errcode = '23514';
  end if;
  if old.is_sales and not new.is_active and new.is_sales then
    raise exception 'el almacén de venta no se archiva' using errcode = '23514';
  end if;
  new.updated_at := now();
  return new;
end;
$$;
-- +goose StatementEnd

-- Siempre un almacén de venta por sucursal: quitarle la marca a uno sin dársela a otro en la misma
-- transacción deja la sucursal sin de dónde vender. Diferido: el cambio quita y pone en dos updates.
-- +goose StatementBegin
create function warehouses_sales_exists() returns trigger
language plpgsql as $$
begin
  if not exists (select 1 from warehouses w where w.company_id = new.company_id and w.branch_id = new.branch_id and w.is_sales) then
    raise exception 'la sucursal se quedaría sin almacén de venta' using errcode = '23514';
  end if;
  return null;
end;
$$;
-- +goose StatementEnd
create constraint trigger warehouses_sales_exists after update of is_sales on warehouses
  deferrable initially deferred for each row execute function warehouses_sales_exists();

-- +goose Down

set local lock_timeout = '3s';

-- Volver a las llaves por sucursal pierde en qué almacén está cada existencia: solo se baja si cada
-- artículo está en un almacén por sucursal y nadie cambió cuál es el de venta.
-- +goose StatementBegin
do $$
begin
  if exists (select 1 from stock_levels group by branch_id, ingredient_id having ingredient_id is not null and count(*) > 1)
     or exists (select 1 from stock_levels group by branch_id, product_id having product_id is not null and count(*) > 1) then
    raise exception 'hay artículos con existencia en dos almacenes de la misma sucursal: bajar la 0121 los mezclaría';
  end if;
  if exists (select 1 from stock_levels s join warehouses w on w.id = s.warehouse_id where not w.is_sales)
     or exists (select 1 from stock_movements m join warehouses w on w.id = m.warehouse_id where not w.is_sales) then
    raise exception 'hay mercancía fuera del almacén de venta: con la 0120 la base no la admite';
  end if;
end $$;
-- +goose StatementEnd

drop trigger warehouses_sales_exists on warehouses;
drop function warehouses_sales_exists();

-- Las compuertas de la 0120.
-- +goose StatementBegin
create or replace function fill_stock_movement_branch_id() returns trigger
language plpgsql as $$
begin
  if new.branch_id is null and new.warehouse_id is not null then
    select w.branch_id into new.branch_id
      from warehouses w where w.id = new.warehouse_id and w.company_id = new.company_id;
  end if;
  if new.branch_id is null and new.order_id is not null then
    select o.branch_id into new.branch_id
      from orders o where o.id = new.order_id and o.company_id = new.company_id;
  end if;
  if new.branch_id is null then
    new.branch_id := branch_for_company(new.company_id);
  end if;
  if new.warehouse_id is null then
    new.warehouse_id := sales_warehouse_of(new.company_id, new.branch_id);
  elsif new.warehouse_id <> sales_warehouse_of(new.company_id, new.branch_id) then
    raise exception 'warehouse_not_writable_yet: el almacén % todavía no recibe movimientos', new.warehouse_id
      using errcode = 'EGB04';
  end if;
  return new;
end;
$$;
-- +goose StatementEnd

-- +goose StatementBegin
create or replace function warehouses_before_update() returns trigger
language plpgsql as $$
begin
  if new.company_id <> old.company_id or new.branch_id <> old.branch_id then
    raise exception 'un almacén no cambia de sucursal' using errcode = '23514';
  end if;
  if new.is_sales <> old.is_sales then
    raise exception 'warehouse_not_writable_yet: el almacén de venta todavía no se puede cambiar' using errcode = 'EGB04';
  end if;
  if old.is_sales and not new.is_active then
    raise exception 'el almacén de venta no se archiva' using errcode = '23514';
  end if;
  new.updated_at := now();
  return new;
end;
$$;
-- +goose StatementEnd

create unique index stock_counts_one_open on stock_counts (company_id, branch_id) where status = 'open';
create unique index notifications_one_open on notifications (company_id, branch_id, item_key, kind)
  where status = 'open' and kind <> 'not_found';
create unique index branch_stock_minimums_ingredient_key on branch_stock_minimums (branch_id, ingredient_id) where ingredient_id is not null;
create unique index branch_stock_minimums_product_key on branch_stock_minimums (branch_id, product_id) where product_id is not null;
alter table stock_levels add constraint stock_levels_ingredient_per_branch unique (branch_id, ingredient_id);
alter table stock_levels add constraint stock_levels_product_per_branch unique (branch_id, product_id);

alter table expense_items drop constraint expense_items_stock_line_has_warehouse;
alter table stock_levels          alter column warehouse_id drop not null;
alter table stock_movements       alter column warehouse_id drop not null;
alter table stock_counts          alter column warehouse_id drop not null;
alter table waste_records         alter column warehouse_id drop not null;
alter table branch_stock_minimums alter column warehouse_id drop not null;
alter table notifications         alter column warehouse_id drop not null;
alter table stock_out_reports     alter column warehouse_id drop not null;
alter table count_places          alter column warehouse_id drop not null;
