-- +goose Up

-- VARIOS ALMACENES POR SUCURSAL (spec 094, issue #65). PRIMERA DE DOS SUBIDAS.
--
-- Una sucursal deja de tener un solo montón de existencias: tiene almacenes, y uno es el de venta.
-- Todo lo que guardaba existencias «de la sucursal» gana `warehouse_id`. Es la puerta «Más de un
-- almacén por sucursal» del principio VIII: el hecho irrecuperable es en qué almacén ocurrió cada
-- movimiento, y por eso se registra desde hoy aunque solo exista uno.
--
-- CONVIVE CON LA API ANTERIOR (constitución IV). Durante la subida la versión vieja escribe sin
-- conocer la columna:
--   * `warehouse_id` entra NULLABLE y un trigger lo llena con el almacén de venta de la sucursal de la
--     fila. El `not null` va en la subida siguiente (research R1).
--   * Las llaves únicas por sucursal SE QUEDAN y se agregan las de almacén junto a ellas: la API vieja
--     hace `on conflict (branch_id, …)` y perderlas le daría 500 durante el traslape (research R2).
--     Se borran en la subida siguiente.
--   * Mientras vivan las dos llaves, una sucursal no puede tener existencias del mismo artículo en dos
--     almacenes. Por eso aquí la base RECHAZA (EGB04) cualquier movimiento a un almacén que no sea el
--     de venta y no deja cambiar cuál es el de venta. La subida siguiente quita las dos compuertas.
--
-- Las cifras medidas en producción el 2026-11-23: stock_movements 7,219 filas, stock_levels 407.

set local lock_timeout = '3s';

-- Mismo orden en que opera una venta (movimiento → existencia → aviso): tomarlas en otro orden con
-- una venta en curso a medio camino es un deadlock que Postgres resuelve matando a una de las dos.
lock table stock_movements, stock_levels, notifications, stock_counts, waste_records,
           branch_stock_minimums, stock_out_reports, expense_items, count_places in access exclusive mode;

-- ---------------------------------------------------------------------------------------------
-- 1. LA TABLA
-- ---------------------------------------------------------------------------------------------
create table warehouses (
  id          bigint generated always as identity primary key,
  company_id  bigint not null default nullif(current_setting('app.company_id', true), '')::bigint
              references companies (id) on delete cascade,
  branch_id   bigint not null,
  name        text not null check (char_length(btrim(name)) between 1 and 40),
  -- El de venta: de aquí descuentan las ventas, y es el que se toma cuando algo no dice almacén.
  is_sales    boolean not null default false,
  is_active   boolean not null default true,
  created_at  timestamptz not null default now(),
  updated_at  timestamptz not null default now(),
  constraint warehouses_branch_of_company foreign key (company_id, branch_id) references branches (company_id, id),
  constraint warehouses_tenant_key unique (company_id, id),
  -- Destino de las llaves compuestas: así un registro de la sucursal A no puede apuntar al almacén de
  -- la sucursal B, aunque RLS fallara (las llaves foráneas saltan RLS).
  constraint warehouses_branch_key unique (company_id, branch_id, id),
  constraint warehouses_sales_is_active check (not is_sales or is_active)
);
create unique index warehouses_one_sales on warehouses (company_id, branch_id) where is_sales;
-- Un nombre por sucursal entre los activos, sin distinguir mayúsculas, acentos ni espacios
-- (`catalog_name_key`, gemela de domain.CatalogNameKey).
create unique index warehouses_name_key on warehouses (company_id, branch_id, catalog_name_key(name)) where is_active;

alter table warehouses enable row level security;
create policy tenant_isolation on warehouses
  using      (company_id = nullif(current_setting('app.company_id', true), '')::bigint)
  with check (company_id = nullif(current_setting('app.company_id', true), '')::bigint);
grant select, insert, update on warehouses to gatobobah_app;

-- «Local», el de venta, para cada sucursal que ya existe. El de una sucursal nueva lo crea
-- CreateCompany en la misma consulta; no un trigger sobre branches, por la razón de la 0076 (un
-- restore con COPY lo dispararía y chocaría con el que trae el respaldo).
insert into warehouses (company_id, branch_id, name, is_sales)
select company_id, id, 'Local', true from branches;

-- +goose StatementBegin
-- sales_warehouse_of devuelve el almacén de venta de una sucursal y truena (EGB03) si no hay
-- ninguno, en vez de escoger otro en silencio. Conserva esta firma: un día puede ganar un parámetro
-- de lugar de venta (puerta «más de una caja», R12).
create function sales_warehouse_of(p_company bigint, p_branch bigint) returns bigint
language plpgsql stable as $$
declare
  v bigint;
begin
  select w.id into v from warehouses w
   where w.company_id = p_company and w.branch_id = p_branch and w.is_sales;
  if v is null then
    raise exception 'warehouse_missing: la sucursal % no tiene almacén de venta', p_branch using errcode = 'EGB03';
  end if;
  return v;
end;
$$;
-- +goose StatementEnd

-- +goose StatementBegin
-- current_sales_warehouse_id es el de venta de la sucursal de la sesión: lo que hoy leen las
-- pantallas que decían `branch_id = current_branch_id()`. Con dos almacenes, leer por sucursal
-- duplicaría cada artículo.
create function current_sales_warehouse_id() returns bigint
language sql stable as $$
  select sales_warehouse_of(nullif(current_setting('app.company_id', true), '')::bigint, current_branch_id())
$$;
-- +goose StatementEnd

-- +goose StatementBegin
create function warehouses_before_update() returns trigger
language plpgsql as $$
begin
  if new.company_id <> old.company_id or new.branch_id <> old.branch_id then
    raise exception 'un almacén no cambia de sucursal' using errcode = '23514';
  end if;
  -- Mientras rijan las llaves por sucursal, mover la marca de venta con existencias repartidas
  -- rompería esas llaves en la primera venta. La subida siguiente reemplaza esta función.
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
create trigger warehouses_before_update before update on warehouses
  for each row execute function warehouses_before_update();

-- ---------------------------------------------------------------------------------------------
-- 2. EL ALMACÉN EN CADA REGISTRO
-- ---------------------------------------------------------------------------------------------
alter table stock_levels          add column warehouse_id bigint;
alter table stock_movements       add column warehouse_id bigint;
alter table stock_counts          add column warehouse_id bigint;
alter table waste_records         add column warehouse_id bigint;
alter table branch_stock_minimums add column warehouse_id bigint;
alter table notifications         add column warehouse_id bigint;
alter table stock_out_reports     add column warehouse_id bigint;
alter table expense_items         add column warehouse_id bigint;
alter table count_places          add column warehouse_id bigint;

-- El relleno: todo al de venta de su sucursal. Los conteos y renglones de conteo heredan del conteo
-- (research R4). Los movimientos, conteos y mermas no tienen triggers de UPDATE que muevan cifras: el
-- trigger de existencias es solo de INSERT.
update stock_levels t set warehouse_id = w.id from warehouses w where w.company_id = t.company_id and w.branch_id = t.branch_id and w.is_sales;
update stock_movements t set warehouse_id = w.id from warehouses w where w.company_id = t.company_id and w.branch_id = t.branch_id and w.is_sales;
update stock_counts t set warehouse_id = w.id from warehouses w where w.company_id = t.company_id and w.branch_id = t.branch_id and w.is_sales;
update waste_records t set warehouse_id = w.id from warehouses w where w.company_id = t.company_id and w.branch_id = t.branch_id and w.is_sales;
update branch_stock_minimums t set warehouse_id = w.id from warehouses w where w.company_id = t.company_id and w.branch_id = t.branch_id and w.is_sales;
update notifications t set warehouse_id = w.id from warehouses w where w.company_id = t.company_id and w.branch_id = t.branch_id and w.is_sales;
update stock_out_reports t set warehouse_id = w.id from warehouses w where w.company_id = t.company_id and w.branch_id = t.branch_id and w.is_sales;

-- Un renglón de compra no tiene sucursal: toma la del movimiento que generó y, si no generó ninguno
-- (no ha llegado, o es un excluido), la matriz. Nunca branch_for_company: truena con dos sucursales.
-- Se llena todo renglón con artículo, excluido o no (R12): el check de la subida siguiente lo exige
-- y un excluido puede dejar de serlo.
update expense_items t set warehouse_id = w.id
  from warehouses w
 where t.item_type is not null and w.company_id = t.company_id and w.is_sales
   and w.branch_id = coalesce(
         (select m.branch_id from stock_movements m where m.expense_item_id = t.id and m.company_id = t.company_id order by m.id limit 1),
         (select b.id from branches b where b.company_id = t.company_id and b.is_headquarters));
update count_places t set warehouse_id = w.id
  from warehouses w join branches b on b.company_id = w.company_id and b.id = w.branch_id and b.is_headquarters
 where w.company_id = t.company_id and w.is_sales;

-- Llaves compuestas: el almacén es de la sucursal de la fila. no action (se evalúa al final de la
-- sentencia): con restrict, el borrado en cascada de una empresa tropezaría con el orden.
alter table stock_levels add constraint stock_levels_warehouse_of_branch
  foreign key (company_id, branch_id, warehouse_id) references warehouses (company_id, branch_id, id);
alter table stock_movements add constraint stock_movements_warehouse_of_branch
  foreign key (company_id, branch_id, warehouse_id) references warehouses (company_id, branch_id, id);
alter table stock_counts add constraint stock_counts_warehouse_of_branch
  foreign key (company_id, branch_id, warehouse_id) references warehouses (company_id, branch_id, id);
alter table waste_records add constraint waste_records_warehouse_of_branch
  foreign key (company_id, branch_id, warehouse_id) references warehouses (company_id, branch_id, id);
alter table branch_stock_minimums add constraint branch_stock_minimums_warehouse_of_branch
  foreign key (company_id, branch_id, warehouse_id) references warehouses (company_id, branch_id, id);
alter table notifications add constraint notifications_warehouse_of_branch
  foreign key (company_id, branch_id, warehouse_id) references warehouses (company_id, branch_id, id);
alter table stock_out_reports add constraint stock_out_reports_warehouse_of_branch
  foreign key (company_id, branch_id, warehouse_id) references warehouses (company_id, branch_id, id);
alter table expense_items add constraint expense_items_warehouse_of_company
  foreign key (company_id, warehouse_id) references warehouses (company_id, id);
alter table count_places add constraint count_places_warehouse_of_company
  foreign key (company_id, warehouse_id) references warehouses (company_id, id);

-- Un ajuste de conteo o de merma está en el almacén de su conteo o de su merma (R11 M3).
alter table stock_counts add constraint stock_counts_id_company_warehouse_key unique (id, company_id, warehouse_id);
alter table waste_records add constraint waste_records_id_company_warehouse_key unique (id, company_id, warehouse_id);
alter table stock_movements add constraint stock_movements_count_same_warehouse
  foreign key (stock_count_id, company_id, warehouse_id) references stock_counts (id, company_id, warehouse_id);
alter table stock_movements add constraint stock_movements_waste_same_warehouse
  foreign key (waste_record_id, company_id, warehouse_id) references waste_records (id, company_id, warehouse_id);

-- Las llaves únicas por almacén, JUNTO a las de sucursal (R2). `warehouse_id` es único global: no
-- hace falta company_id en las dos primeras (R11 L3).
alter table stock_levels add constraint stock_levels_ingredient_per_warehouse unique (warehouse_id, ingredient_id);
alter table stock_levels add constraint stock_levels_product_per_warehouse unique (warehouse_id, product_id);
-- Mismo predicado exacto que el de sucursal: el `on conflict … where` lo infiere por igualdad (L6).
create unique index branch_stock_minimums_ingredient_per_warehouse on branch_stock_minimums (warehouse_id, ingredient_id) where ingredient_id is not null;
create unique index branch_stock_minimums_product_per_warehouse on branch_stock_minimums (warehouse_id, product_id) where product_id is not null;
create unique index notifications_one_open_per_warehouse on notifications (company_id, warehouse_id, item_key, kind)
  where status = 'open' and kind <> 'not_found';
create unique index stock_counts_one_open_per_warehouse on stock_counts (company_id, warehouse_id) where status = 'open';

-- Movimientos por almacén (Movimientos, E6); empieza por company_id por RLS.
create index stock_movements_by_warehouse on stock_movements (company_id, warehouse_id, id desc);

-- ---------------------------------------------------------------------------------------------
-- 3. LLENAR EL ALMACÉN CUANDO EL INSERT NO LO TRAE
-- ---------------------------------------------------------------------------------------------
-- +goose StatementBegin
create function fill_warehouse_id() returns trigger
language plpgsql as $$
begin
  if new.warehouse_id is null then
    new.warehouse_id := sales_warehouse_of(new.company_id, new.branch_id);
  end if;
  return new;
end;
$$;
-- +goose StatementEnd

create trigger trg_stock_levels_warehouse before insert on stock_levels for each row execute function fill_warehouse_id();
create trigger trg_stock_counts_warehouse before insert on stock_counts for each row execute function fill_warehouse_id();
create trigger trg_waste_records_warehouse before insert on waste_records for each row execute function fill_warehouse_id();
create trigger trg_branch_stock_minimums_warehouse before insert on branch_stock_minimums for each row execute function fill_warehouse_id();
create trigger trg_notifications_warehouse before insert on notifications for each row execute function fill_warehouse_id();
create trigger trg_stock_out_reports_warehouse before insert on stock_out_reports for each row execute function fill_warehouse_id();

-- Sin sucursal en la fila: la matriz de la empresa. Dos funciones y no una con `tg_table_name`:
-- plpgsql resuelve `new.item_type` al ejecutar, y en count_places (sin esa columna) truena.
-- +goose StatementBegin
create function fill_headquarters_warehouse_id() returns trigger
language plpgsql as $$
begin
  if new.warehouse_id is null then
    new.warehouse_id := sales_warehouse_of(new.company_id,
      (select b.id from branches b where b.company_id = new.company_id and b.is_headquarters));
  end if;
  return new;
end;
$$;
-- +goose StatementEnd

-- +goose StatementBegin
create function fill_expense_item_warehouse_id() returns trigger
language plpgsql as $$
begin
  if new.warehouse_id is null and new.item_type is not null then
    new.warehouse_id := sales_warehouse_of(new.company_id,
      (select b.id from branches b where b.company_id = new.company_id and b.is_headquarters));
  end if;
  return new;
end;
$$;
-- +goose StatementEnd

-- En el renglón de compra también al cambiarle el artículo: un renglón que nació sin artículo y lo
-- gana después queda con almacén.
create trigger trg_expense_items_warehouse before insert or update of item_type on expense_items
  for each row execute function fill_expense_item_warehouse_id();
create trigger trg_count_places_warehouse before insert on count_places
  for each row execute function fill_headquarters_warehouse_id();

-- El movimiento llena sucursal y almacén en una sola función (sin depender del orden alfabético de
-- triggers, R11 L1). Trae almacén y no sucursal: la sucursal es la del almacén. Y la compuerta de esta
-- subida: nada entra a un almacén que no sea el de venta (EGB04, R11 H2).
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

-- ---------------------------------------------------------------------------------------------
-- 4. LA EXISTENCIA SE SUMA POR ALMACÉN
-- ---------------------------------------------------------------------------------------------
-- +goose StatementBegin
create or replace function apply_stock_movement() returns trigger as $$
begin
  if new.item_type = 'ingrediente' then
    insert into stock_levels (company_id, branch_id, warehouse_id, item_type, ingredient_id, on_hand, updated_at)
      values (new.company_id, new.branch_id, new.warehouse_id, 'ingrediente', new.ingredient_id, new.quantity, now())
    on conflict (warehouse_id, ingredient_id) do update
      set on_hand = stock_levels.on_hand + new.quantity, updated_at = now();
  else
    insert into stock_levels (company_id, branch_id, warehouse_id, item_type, product_id, on_hand, updated_at)
      values (new.company_id, new.branch_id, new.warehouse_id, 'producto', new.product_id, new.quantity, now())
    on conflict (warehouse_id, product_id) do update
      set on_hand = stock_levels.on_hand + new.quantity, updated_at = now();
  end if;
  return new;
end;
$$ language plpgsql;
-- +goose StatementEnd

-- ---------------------------------------------------------------------------------------------
-- 5. AVISOS DE INVENTARIO: MIDEN EL ALMACÉN DE VENTA (R11 H1, R12)
--
-- low_stock_state leía la existencia con un subselect de una fila por sucursal: con el artículo en
-- dos almacenes truena «more than one row», FUERA del exception del trigger, y tumba la venta. Ahora
-- lee el almacén de venta (con Local en 0 y Bodega en 50, la suma no avisaría «se acabó» aunque el
-- mostrador no pueda vender). evaluate_low_stock hace on conflict sobre el índice por almacén: el de
-- sucursal se borra en la subida siguiente y, sin esto, los avisos dejarían de crearse en silencio.
-- ---------------------------------------------------------------------------------------------
-- +goose StatementBegin
create or replace function low_stock_state(p_company bigint, p_branch bigint, p_type stock_item_type, p_ingredient bigint, p_product bigint)
returns table (active boolean, enabled boolean, on_min boolean, should_low boolean, should_out boolean, on_hand numeric, min_stock numeric,
               low_open boolean, out_open boolean, out_manual boolean, item_name text, unit_code text)
language sql stable as $$
  with wh as (
    select sales_warehouse_of(p_company, p_branch) as id
  ), art as (
    select i.is_active and not exists (
             select 1 from ingredient_replacements r
              where r.company_id = i.company_id and r.old_ingredient_id = i.id and r.kind = 'merge') as active,
           i.low_stock_enabled as enabled, i.low_stock_on_min as on_min, i.low_stock_on_out as on_out,
           coalesce(b.min_stock, i.min_stock) as min_stock, i.name, u.code as unit_code
      from ingredients i
      join units u on u.id = i.base_unit_id
      left join branch_stock_minimums b on b.warehouse_id = (select id from wh) and b.ingredient_id = i.id
     where p_type = 'ingrediente' and i.id = p_ingredient and i.company_id = p_company
    union all
    select p.is_active, p.low_stock_enabled, p.low_stock_on_min, p.low_stock_on_out,
           coalesce(b.min_stock, p.min_stock), p.name, 'pieza'
      from products p
      left join branch_stock_minimums b on b.warehouse_id = (select id from wh) and b.product_id = p.id
     where p_type = 'producto' and p.id = p_product and p.company_id = p_company
  ), lvl as (
    -- Dos ramas con guarda: cada una usa su índice único (almacén, artículo).
    select coalesce((select s.on_hand from stock_levels s
                      where p_type = 'ingrediente' and s.warehouse_id = (select id from wh) and s.ingredient_id = p_ingredient
                     union all
                     select s.on_hand from stock_levels s
                      where p_type = 'producto' and s.warehouse_id = (select id from wh) and s.product_id = p_product), 0) as on_hand
  ), open as (
    select bool_or(n.kind = 'stock_low') as low_open, bool_or(n.kind = 'stock_out') as out_open,
           bool_or(n.kind = 'stock_out' and n.opened_by is not null) as out_manual
      from notifications n
     where n.company_id = p_company and n.warehouse_id = (select id from wh) and n.status = 'open' and n.kind <> 'not_found'
       and n.item_key = coalesce(p_ingredient, -p_product, 0)
  )
  select a.active, a.enabled, a.on_min,
         a.active and a.enabled and a.on_min and coalesce(a.min_stock, 0) > 0 and l.on_hand <= a.min_stock,
         a.active and a.enabled and a.on_out and l.on_hand <= 0,
         l.on_hand, a.min_stock,
         coalesce(o.low_open, false), coalesce(o.out_open, false), coalesce(o.out_manual, false),
         a.name, a.unit_code
    from art a cross join lvl l cross join open o
$$;
-- +goose StatementEnd

-- +goose StatementBegin
create or replace function evaluate_low_stock(p_company bigint, p_branch bigint, p_type stock_item_type, p_ingredient bigint, p_product bigint,
                                              p_count_id bigint default null)
returns void language plpgsql as $$
declare
  s record;
  v_wh bigint := sales_warehouse_of(p_company, p_branch);
  v_needs boolean;
  v_changed integer := 0;
  v_n integer;
begin
  select * into s from low_stock_state(p_company, p_branch, p_type, p_ingredient, p_product);
  if not found then
    return;
  end if;

  update notifications n
     set status = 'resolved', resolved_at = now(),
         resolved_reason = case
           when not s.active then 'archived'
           when p_count_id is not null then 'counted'
           when n.kind = 'stock_low' then
             case when s.enabled and s.on_min and coalesce(s.min_stock, 0) > 0 then 'above_min' else 'alert_off' end
           when s.on_hand > 0 then 'restocked'
           else 'alert_off' end
   where n.company_id = p_company and n.warehouse_id = v_wh and n.status = 'open' and n.kind <> 'not_found'
     and n.item_key = coalesce(p_ingredient, -p_product, 0)
     and (not s.active
          or (n.kind = 'stock_low' and not s.should_low)
          or (n.kind = 'stock_out' and not s.should_out and (n.opened_by is null or s.on_hand > 0)));
  get diagnostics v_n = row_count;
  v_changed := v_changed + v_n;

  if s.should_low or s.should_out then
    v_needs := s.on_hand < 0 or (p_count_id is null and not exists (
      select 1 from stock_count_lines l join stock_counts c on c.id = l.count_id and c.company_id = l.company_id
       where p_type = 'ingrediente' and l.ingredient_id = p_ingredient and c.warehouse_id = v_wh
         and c.status = 'closed' and c.closed_at > now() - interval '30 days'
      union all
      select 1 from stock_count_product_lines l join stock_counts c on c.id = l.count_id and c.company_id = l.company_id
       where p_type = 'producto' and l.product_id = p_product and c.warehouse_id = v_wh
         and c.status = 'closed' and c.closed_at > now() - interval '30 days'));
    insert into notifications (company_id, branch_id, warehouse_id, kind, item_type, ingredient_id, product_id, item_name, unit_code,
                               on_hand_seen, min_seen, needs_count)
    select p_company, p_branch, v_wh, k, p_type, p_ingredient, p_product, s.item_name, s.unit_code, s.on_hand, s.min_stock, v_needs
      from unnest(array[case when s.should_low then 'stock_low' end, case when s.should_out then 'stock_out' end]) k
     where k is not null
    on conflict (company_id, warehouse_id, item_key, kind) where status = 'open' and kind <> 'not_found' do nothing;
    get diagnostics v_n = row_count;
    v_changed := v_changed + v_n;
  end if;

  if v_changed > 0 then
    perform pg_notify('notifications', p_company::text);
  end if;
end;
$$;
-- +goose StatementEnd

-- ---------------------------------------------------------------------------------------------
-- 6. PERMISOS NUEVOS (spec 068: herencia a los sugeridos que hoy configuran y cuentan el almacén)
-- ---------------------------------------------------------------------------------------------
insert into permissions (id, code, area) values
  (40, 'inventory.warehouse_manage', 'inventory'),
  (41, 'inventory.transfer', 'inventory');
insert into system_role_permissions (system_role_id, permission_id) values (1, 40), (2, 40), (1, 41), (2, 41);

-- ---------------------------------------------------------------------------------------------
-- 7. COMPROBACIÓN: ni una fila sin almacén, uno de venta por sucursal
-- ---------------------------------------------------------------------------------------------
-- +goose StatementBegin
do $$
begin
  if exists (select 1 from branches b where not exists (
       select 1 from warehouses w where w.company_id = b.company_id and w.branch_id = b.id and w.is_sales)) then
    raise exception 'quedó una sucursal sin almacén de venta';
  end if;
  if exists (select 1 from stock_levels where warehouse_id is null)
     or exists (select 1 from stock_movements where warehouse_id is null)
     or exists (select 1 from stock_counts where warehouse_id is null)
     or exists (select 1 from waste_records where warehouse_id is null)
     or exists (select 1 from branch_stock_minimums where warehouse_id is null)
     or exists (select 1 from notifications where warehouse_id is null)
     or exists (select 1 from stock_out_reports where warehouse_id is null)
     or exists (select 1 from expense_items where item_type is not null and warehouse_id is null)
     or exists (select 1 from count_places where warehouse_id is null) then
    raise exception 'quedaron filas sin almacén';
  end if;
end $$;
-- +goose StatementEnd

-- +goose Down

set local lock_timeout = '3s';

-- Bajar pierde en qué almacén está cada cosa. Solo se permite si todo está en el de venta y no hay
-- otro almacén: entonces la sucursal y el almacén dicen lo mismo y no se pierde nada.
-- +goose StatementBegin
do $$
begin
  if exists (select 1 from warehouses where not is_sales) then
    raise exception 'hay almacenes además del de venta: bajar la 0117 perdería en cuál está cada existencia';
  end if;
end $$;
-- +goose StatementEnd

lock table stock_movements, stock_levels, notifications, stock_counts, waste_records,
           branch_stock_minimums, stock_out_reports, expense_items, count_places in access exclusive mode;

-- Las funciones vuelven a su versión anterior ANTES de soltar las columnas que usan.
-- +goose StatementBegin
create or replace function fill_stock_movement_branch_id() returns trigger
language plpgsql as $$
begin
  if new.branch_id is null and new.order_id is not null then
    select o.branch_id into new.branch_id
      from orders o where o.id = new.order_id and o.company_id = new.company_id;
  end if;
  if new.branch_id is null then
    new.branch_id := branch_for_company(new.company_id);
  end if;
  return new;
end;
$$;
-- +goose StatementEnd

-- +goose StatementBegin
create or replace function apply_stock_movement() returns trigger as $$
begin
  if new.item_type = 'ingrediente' then
    insert into stock_levels (company_id, branch_id, item_type, ingredient_id, on_hand, updated_at)
      values (new.company_id, new.branch_id, 'ingrediente', new.ingredient_id, new.quantity, now())
    on conflict (branch_id, ingredient_id) do update
      set on_hand = stock_levels.on_hand + new.quantity, updated_at = now();
  else
    insert into stock_levels (company_id, branch_id, item_type, product_id, on_hand, updated_at)
      values (new.company_id, new.branch_id, 'producto', new.product_id, new.quantity, now())
    on conflict (branch_id, product_id) do update
      set on_hand = stock_levels.on_hand + new.quantity, updated_at = now();
  end if;
  return new;
end;
$$ language plpgsql;
-- +goose StatementEnd

-- +goose StatementBegin
create or replace function low_stock_state(p_company bigint, p_branch bigint, p_type stock_item_type, p_ingredient bigint, p_product bigint)
returns table (active boolean, enabled boolean, on_min boolean, should_low boolean, should_out boolean, on_hand numeric, min_stock numeric,
               low_open boolean, out_open boolean, out_manual boolean, item_name text, unit_code text)
language sql stable as $$
  with art as (
    select i.is_active and not exists (
             select 1 from ingredient_replacements r
              where r.company_id = i.company_id and r.old_ingredient_id = i.id and r.kind = 'merge') as active,
           i.low_stock_enabled as enabled, i.low_stock_on_min as on_min, i.low_stock_on_out as on_out,
           coalesce(b.min_stock, i.min_stock) as min_stock, i.name, u.code as unit_code
      from ingredients i
      join units u on u.id = i.base_unit_id
      left join branch_stock_minimums b on b.branch_id = p_branch and b.ingredient_id = i.id
     where p_type = 'ingrediente' and i.id = p_ingredient and i.company_id = p_company
    union all
    select p.is_active, p.low_stock_enabled, p.low_stock_on_min, p.low_stock_on_out,
           coalesce(b.min_stock, p.min_stock), p.name, 'pieza'
      from products p
      left join branch_stock_minimums b on b.branch_id = p_branch and b.product_id = p.id
     where p_type = 'producto' and p.id = p_product and p.company_id = p_company
  ), lvl as (
    select coalesce((select s.on_hand from stock_levels s
                      where p_type = 'ingrediente' and s.branch_id = p_branch and s.ingredient_id = p_ingredient
                     union all
                     select s.on_hand from stock_levels s
                      where p_type = 'producto' and s.branch_id = p_branch and s.product_id = p_product), 0) as on_hand
  ), open as (
    select bool_or(n.kind = 'stock_low') as low_open, bool_or(n.kind = 'stock_out') as out_open,
           bool_or(n.kind = 'stock_out' and n.opened_by is not null) as out_manual
      from notifications n
     where n.company_id = p_company and n.branch_id = p_branch and n.status = 'open' and n.kind <> 'not_found'
       and n.item_key = coalesce(p_ingredient, -p_product, 0)
  )
  select a.active, a.enabled, a.on_min,
         a.active and a.enabled and a.on_min and coalesce(a.min_stock, 0) > 0 and l.on_hand <= a.min_stock,
         a.active and a.enabled and a.on_out and l.on_hand <= 0,
         l.on_hand, a.min_stock,
         coalesce(o.low_open, false), coalesce(o.out_open, false), coalesce(o.out_manual, false),
         a.name, a.unit_code
    from art a cross join lvl l cross join open o
$$;
-- +goose StatementEnd

-- +goose StatementBegin
create or replace function evaluate_low_stock(p_company bigint, p_branch bigint, p_type stock_item_type, p_ingredient bigint, p_product bigint,
                                              p_count_id bigint default null)
returns void language plpgsql as $$
declare
  s record;
  v_needs boolean;
  v_changed integer := 0;
  v_n integer;
begin
  select * into s from low_stock_state(p_company, p_branch, p_type, p_ingredient, p_product);
  if not found then
    return;
  end if;
  update notifications n
     set status = 'resolved', resolved_at = now(),
         resolved_reason = case
           when not s.active then 'archived'
           when p_count_id is not null then 'counted'
           when n.kind = 'stock_low' then
             case when s.enabled and s.on_min and coalesce(s.min_stock, 0) > 0 then 'above_min' else 'alert_off' end
           when s.on_hand > 0 then 'restocked'
           else 'alert_off' end
   where n.company_id = p_company and n.branch_id = p_branch and n.status = 'open' and n.kind <> 'not_found'
     and n.item_key = coalesce(p_ingredient, -p_product, 0)
     and (not s.active
          or (n.kind = 'stock_low' and not s.should_low)
          or (n.kind = 'stock_out' and not s.should_out and (n.opened_by is null or s.on_hand > 0)));
  get diagnostics v_n = row_count;
  v_changed := v_changed + v_n;
  if s.should_low or s.should_out then
    v_needs := s.on_hand < 0 or (p_count_id is null and not exists (
      select 1 from stock_count_lines l join stock_counts c on c.id = l.count_id and c.company_id = l.company_id
       where p_type = 'ingrediente' and l.ingredient_id = p_ingredient and c.branch_id = p_branch
         and c.status = 'closed' and c.closed_at > now() - interval '30 days'
      union all
      select 1 from stock_count_product_lines l join stock_counts c on c.id = l.count_id and c.company_id = l.company_id
       where p_type = 'producto' and l.product_id = p_product and c.branch_id = p_branch
         and c.status = 'closed' and c.closed_at > now() - interval '30 days'));
    insert into notifications (company_id, branch_id, kind, item_type, ingredient_id, product_id, item_name, unit_code,
                               on_hand_seen, min_seen, needs_count)
    select p_company, p_branch, k, p_type, p_ingredient, p_product, s.item_name, s.unit_code, s.on_hand, s.min_stock, v_needs
      from unnest(array[case when s.should_low then 'stock_low' end, case when s.should_out then 'stock_out' end]) k
     where k is not null
    on conflict (company_id, branch_id, item_key, kind) where status = 'open' and kind <> 'not_found' do nothing;
    get diagnostics v_n = row_count;
    v_changed := v_changed + v_n;
  end if;
  if v_changed > 0 then
    perform pg_notify('notifications', p_company::text);
  end if;
end;
$$;
-- +goose StatementEnd

delete from role_permission_overrides where permission_id in (40, 41);
delete from system_role_permissions where permission_id in (40, 41);
delete from permissions where id in (40, 41);

drop trigger trg_stock_levels_warehouse on stock_levels;
drop trigger trg_stock_counts_warehouse on stock_counts;
drop trigger trg_waste_records_warehouse on waste_records;
drop trigger trg_branch_stock_minimums_warehouse on branch_stock_minimums;
drop trigger trg_notifications_warehouse on notifications;
drop trigger trg_stock_out_reports_warehouse on stock_out_reports;
drop trigger trg_expense_items_warehouse on expense_items;
drop trigger trg_count_places_warehouse on count_places;
drop function fill_warehouse_id();
drop function fill_headquarters_warehouse_id();
drop function fill_expense_item_warehouse_id();

drop index stock_movements_by_warehouse;
drop index stock_counts_one_open_per_warehouse;
drop index notifications_one_open_per_warehouse;
drop index branch_stock_minimums_ingredient_per_warehouse;
drop index branch_stock_minimums_product_per_warehouse;
alter table stock_levels drop constraint stock_levels_ingredient_per_warehouse, drop constraint stock_levels_product_per_warehouse;
alter table stock_movements drop constraint stock_movements_count_same_warehouse, drop constraint stock_movements_waste_same_warehouse;
alter table stock_counts drop constraint stock_counts_id_company_warehouse_key;
alter table waste_records drop constraint waste_records_id_company_warehouse_key;

alter table stock_levels drop column warehouse_id;
alter table stock_movements drop column warehouse_id;
alter table stock_counts drop column warehouse_id;
alter table waste_records drop column warehouse_id;
alter table branch_stock_minimums drop column warehouse_id;
alter table notifications drop column warehouse_id;
alter table stock_out_reports drop column warehouse_id;
alter table expense_items drop column warehouse_id;
alter table count_places drop column warehouse_id;

drop trigger warehouses_before_update on warehouses;
drop function warehouses_before_update();
drop function current_sales_warehouse_id();
drop function sales_warehouse_of(bigint, bigint);
drop table warehouses;
