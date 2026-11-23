-- +goose Up
-- Conteo, merma, «se acabó», mínimos y lugares por almacén (spec 094, historia 6).
--
-- 1. Los avisos de inventario miden el almacén DEL MOVIMIENTO. Hasta aquí medían siempre el de
--    venta: una merma en Bodega se evaluaba contra Local, y el mínimo de Bodega no avisaba nunca.
--    Las funciones por sucursal se quedan como envoltura sobre el de venta: la API anterior las
--    llama durante la subida (constitución IV) y siguen diciendo lo mismo que antes.
-- 2. El nombre de un lugar de conteo es único por almacén: «Refri» puede existir en Local y en
--    Bodega.
-- 3. Se validan las dos reglas que la 0124 dejó `not valid` sobre stock_movements. `validate` toma
--    share update exclusive, que no choca con los insert de las ventas.

set local lock_timeout = '3s';

-- ---------------------------------------------------------------------------------------------
-- 1. AVISOS POR ALMACÉN
--
-- Un almacén que no es el de venta solo avisa si tiene su propio mínimo: el mínimo del artículo y
-- «se acabó» hablan de lo que el mostrador puede vender, y una bodega vaciada por un traspaso no es
-- un mostrador sin producto. El «se acabó» que una persona reporta a mano en ese almacén sí se cierra
-- solo cuando vuelve a haber (la regla de abajo lo resuelve con should_out falso y on_hand > 0).
-- ---------------------------------------------------------------------------------------------
-- +goose StatementBegin
create function warehouse_low_stock_state(p_company bigint, p_warehouse bigint, p_type stock_item_type, p_ingredient bigint, p_product bigint)
returns table (active boolean, enabled boolean, on_min boolean, should_low boolean, should_out boolean, on_hand numeric, min_stock numeric,
               low_open boolean, out_open boolean, out_manual boolean, item_name text, unit_code text)
language sql stable as $$
  with wh as (
    select w.is_sales from warehouses w where w.id = p_warehouse and w.company_id = p_company
  ), art as (
    select i.is_active and not exists (
             select 1 from ingredient_replacements r
              where r.company_id = i.company_id and r.old_ingredient_id = i.id and r.kind = 'merge') as active,
           i.low_stock_enabled and ((select is_sales from wh) or b.warehouse_id is not null) as enabled,
           i.low_stock_on_min as on_min, i.low_stock_on_out as on_out,
           case when (select is_sales from wh) then coalesce(b.min_stock, i.min_stock) else b.min_stock end as min_stock,
           i.name, u.code as unit_code
      from ingredients i
      join units u on u.id = i.base_unit_id
      left join branch_stock_minimums b on b.warehouse_id = p_warehouse and b.ingredient_id = i.id
     where p_type = 'ingrediente' and i.id = p_ingredient and i.company_id = p_company
    union all
    select p.is_active, p.low_stock_enabled and ((select is_sales from wh) or b.warehouse_id is not null),
           p.low_stock_on_min, p.low_stock_on_out,
           case when (select is_sales from wh) then coalesce(b.min_stock, p.min_stock) else b.min_stock end, p.name, 'pieza'
      from products p
      left join branch_stock_minimums b on b.warehouse_id = p_warehouse and b.product_id = p.id
     where p_type = 'producto' and p.id = p_product and p.company_id = p_company
  ), lvl as (
    -- Dos ramas con guarda: cada una usa su índice único (almacén, artículo).
    select coalesce((select s.on_hand from stock_levels s
                      where p_type = 'ingrediente' and s.warehouse_id = p_warehouse and s.ingredient_id = p_ingredient
                     union all
                     select s.on_hand from stock_levels s
                      where p_type = 'producto' and s.warehouse_id = p_warehouse and s.product_id = p_product), 0) as on_hand
  ), open as (
    select bool_or(n.kind = 'stock_low') as low_open, bool_or(n.kind = 'stock_out') as out_open,
           bool_or(n.kind = 'stock_out' and n.opened_by is not null) as out_manual
      from notifications n
     where n.company_id = p_company and n.warehouse_id = p_warehouse and n.status = 'open' and n.kind <> 'not_found'
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
create function evaluate_warehouse_low_stock(p_company bigint, p_warehouse bigint, p_type stock_item_type, p_ingredient bigint,
                                             p_product bigint, p_count_id bigint default null)
returns void language plpgsql as $$
declare
  s record;
  v_branch bigint;
  v_needs boolean;
  v_changed integer := 0;
  v_n integer;
begin
  select w.branch_id into v_branch from warehouses w where w.id = p_warehouse and w.company_id = p_company;
  if v_branch is null then
    return;
  end if;
  select * into s from warehouse_low_stock_state(p_company, p_warehouse, p_type, p_ingredient, p_product);
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
   where n.company_id = p_company and n.warehouse_id = p_warehouse and n.status = 'open' and n.kind <> 'not_found'
     and n.item_key = coalesce(p_ingredient, -p_product, 0)
     and (not s.active
          or (n.kind = 'stock_low' and not s.should_low)
          or (n.kind = 'stock_out' and not s.should_out and (n.opened_by is null or s.on_hand > 0)));
  get diagnostics v_n = row_count;
  v_changed := v_changed + v_n;

  if s.should_low or s.should_out then
    v_needs := s.on_hand < 0 or (p_count_id is null and not exists (
      select 1 from stock_count_lines l join stock_counts c on c.id = l.count_id and c.company_id = l.company_id
       where p_type = 'ingrediente' and l.ingredient_id = p_ingredient and c.warehouse_id = p_warehouse
         and c.status = 'closed' and c.closed_at > now() - interval '30 days'
      union all
      select 1 from stock_count_product_lines l join stock_counts c on c.id = l.count_id and c.company_id = l.company_id
       where p_type = 'producto' and l.product_id = p_product and c.warehouse_id = p_warehouse
         and c.status = 'closed' and c.closed_at > now() - interval '30 days'));
    insert into notifications (company_id, branch_id, warehouse_id, kind, item_type, ingredient_id, product_id, item_name, unit_code,
                               on_hand_seen, min_seen, needs_count)
    select p_company, v_branch, p_warehouse, k, p_type, p_ingredient, p_product, s.item_name, s.unit_code, s.on_hand, s.min_stock, v_needs
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

-- Las de sucursal, para la API anterior: el almacén de venta, como hasta hoy.
-- +goose StatementBegin
create or replace function low_stock_state(p_company bigint, p_branch bigint, p_type stock_item_type, p_ingredient bigint, p_product bigint)
returns table (active boolean, enabled boolean, on_min boolean, should_low boolean, should_out boolean, on_hand numeric, min_stock numeric,
               low_open boolean, out_open boolean, out_manual boolean, item_name text, unit_code text)
language sql stable as $$
  select * from warehouse_low_stock_state(p_company, sales_warehouse_of(p_company, p_branch), p_type, p_ingredient, p_product)
$$;
-- +goose StatementEnd

-- +goose StatementBegin
create or replace function evaluate_low_stock(p_company bigint, p_branch bigint, p_type stock_item_type, p_ingredient bigint, p_product bigint,
                                              p_count_id bigint default null)
returns void language plpgsql as $$
begin
  perform evaluate_warehouse_low_stock(p_company, sales_warehouse_of(p_company, p_branch), p_type, p_ingredient, p_product, p_count_id);
end;
$$;
-- +goose StatementEnd

-- +goose StatementBegin
create or replace function notify_stock_movement() returns trigger language plpgsql as $$
declare
  s record;
begin
  -- Los traspasos de una unión o de un cambio de unidad no son consumo: los evalúa el servicio que
  -- los hace, al final, con todo ya convertido.
  if new.replacement_id is not null then
    return null;
  end if;
  select * into s from warehouse_low_stock_state(new.company_id, new.warehouse_id, new.item_type, new.ingredient_id, new.product_id);
  if not found then
    return null;
  end if;
  -- Solo cuando lo que debería estar abierto difiere de lo que está: cada bloque con excepción es una
  -- subtransacción, y con más de 64 vivas en una sesión la base entera se alenta.
  if (not s.active and (s.low_open or s.out_open))
     or s.should_low <> s.low_open
     or (s.should_out and not s.out_open)
     or (s.out_open and not s.should_out and (not s.out_manual or s.on_hand > 0)) then
    begin
      perform evaluate_warehouse_low_stock(new.company_id, new.warehouse_id, new.item_type, new.ingredient_id, new.product_id, new.stock_count_id);
    exception when others then
      -- La venta nunca falla por el aviso. Como la regla es por estado, el siguiente movimiento del
      -- artículo (o el barrido de la mañana) lo vuelve a intentar.
      raise warning 'notification_failed: % (%)', sqlerrm, sqlstate;
    end;
  end if;
  return null;
end;
$$;
-- +goose StatementEnd

revoke execute on function warehouse_low_stock_state(bigint, bigint, stock_item_type, bigint, bigint) from public;
revoke execute on function evaluate_warehouse_low_stock(bigint, bigint, stock_item_type, bigint, bigint, bigint) from public;
grant execute on function warehouse_low_stock_state(bigint, bigint, stock_item_type, bigint, bigint) to gatobobah_app;
grant execute on function evaluate_warehouse_low_stock(bigint, bigint, stock_item_type, bigint, bigint, bigint) to gatobobah_app;

-- ---------------------------------------------------------------------------------------------
-- 2. LUGARES: NOMBRE ÚNICO POR ALMACÉN
-- ---------------------------------------------------------------------------------------------
create unique index count_places_name_per_warehouse on count_places
  (company_id, warehouse_id, regexp_replace(translate(lower(btrim(name)), 'áéíóúüñ', 'aeiouun'), '\s+', ' ', 'g'))
  where archived_at is null;
drop index count_places_name_key;

-- ---------------------------------------------------------------------------------------------
-- 3. LO QUE LA 0124 DEJÓ SIN VALIDAR
-- ---------------------------------------------------------------------------------------------
alter table stock_movements validate constraint stock_movements_transfer_of_branch;
alter table stock_movements validate constraint stock_movements_transfer_link;

-- +goose Down

set local lock_timeout = '3s';

-- Las dos reglas validadas se quedan así: «validada» solo dice que las filas viejas ya se revisaron.

-- +goose StatementBegin
do $$
begin
  if exists (select 1 from count_places where archived_at is null
              group by company_id, regexp_replace(translate(lower(btrim(name)), 'áéíóúüñ', 'aeiouun'), '\s+', ' ', 'g')
             having count(*) > 1) then
    raise exception 'hay lugares de conteo con el mismo nombre en dos almacenes: bajar la 0127 exige renombrarlos primero';
  end if;
end $$;
-- +goose StatementEnd

create unique index count_places_name_key on count_places
  (company_id, regexp_replace(translate(lower(btrim(name)), 'áéíóúüñ', 'aeiouun'), '\s+', ' ', 'g'))
  where archived_at is null;
drop index count_places_name_per_warehouse;

-- +goose StatementBegin
create or replace function notify_stock_movement() returns trigger language plpgsql as $$
declare
  s record;
begin
  if new.replacement_id is not null then
    return null;
  end if;
  select * into s from low_stock_state(new.company_id, new.branch_id, new.item_type, new.ingredient_id, new.product_id);
  if not found then
    return null;
  end if;
  if (not s.active and (s.low_open or s.out_open))
     or s.should_low <> s.low_open
     or (s.should_out and not s.out_open)
     or (s.out_open and not s.should_out and (not s.out_manual or s.on_hand > 0)) then
    begin
      perform evaluate_low_stock(new.company_id, new.branch_id, new.item_type, new.ingredient_id, new.product_id, new.stock_count_id);
    exception when others then
      raise warning 'notification_failed: % (%)', sqlerrm, sqlstate;
    end;
  end if;
  return null;
end;
$$;
-- +goose StatementEnd

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

drop function evaluate_warehouse_low_stock(bigint, bigint, stock_item_type, bigint, bigint, bigint);
drop function warehouse_low_stock_state(bigint, bigint, stock_item_type, bigint, bigint);
