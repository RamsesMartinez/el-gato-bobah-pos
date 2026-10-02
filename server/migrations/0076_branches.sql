-- +goose Up

-- SUCURSALES (spec 025). TODA EMPRESA TIENE UNA MATRIZ Y PUEDE TENER N SUCURSALES.
--
-- Hasta aquí todo lo que pasaba en un negocio se le atribuía a la empresa entera. Con una sola
-- sucursal da igual; el día que exista la segunda, todo lo que no haya guardado su sucursal queda
-- ambiguo para siempre: un pedido sin sucursal no se puede repartir después entre dos locales. Por
-- eso se guarda desde hoy, aunque nada en pantalla lo muestre (puerta «Más de una sucursal» del
-- principio VIII).
--
-- QUÉ ES DE LA SUCURSAL: lo que ocurre en un lugar. Cajas (y por ellas turnos, conteos y
-- movimientos de efectivo), pedidos, tiendas de plataforma, existencias y sus movimientos. Los
-- gastos pueden ser de una sucursal o de toda la empresa (`branch_id` nulo).
-- QUÉ SE QUEDA EN LA EMPRESA: lo fiscal y lo compartido. Catálogo, empleados, proveedores y las
-- credenciales de la app de plataformas. Precio o disponibilidad por sucursal se agregarán como
-- excepciones sobre el maestro, en tablas nuevas, sin tocar lo de aquí.
--
-- CÓMO SE ELIGE LA SUCURSAL SIN PANTALLA: `branch_for_company(empresa)` devuelve la única sucursal
-- activa de esa empresa y TRUENA (EGB01) si hay cero o más de una. Nunca escoge la matriz en
-- silencio: con dos sucursales y sin selector, adivinar mezclaría las ventas de las dos. Los
-- triggers `fill_branch_id` la usan para llenar la columna cuando el insert no la trae, así que
-- ninguna consulta de escritura existente cambia.
--
-- POR QUÉ SE RESUELVE CON LA EMPRESA DE LA FILA Y NO CON LA DE LA SESIÓN: hay escrituras como owner
-- (pruebas, el corte de 2026-09) que insertan para una empresa distinta de la del ajuste de sesión.
-- Con la de la sesión, esa fila quedaría apuntando a una sucursal ajena y la llave compuesta la
-- rechazaría; con la de la fila, cae siempre en su propia empresa.
--
-- LA MATRIZ NO LA CREA UN TRIGGER SOBRE `companies`: `pg_restore` carga `companies` con COPY, que
-- dispara triggers, y crearía una matriz que choca con la real que viene después en el respaldo.
-- La crea `CreateCompany` en la misma consulta que la empresa.

set local lock_timeout = '3s';

-- ---------------------------------------------------------------------------------------------
-- 1. LA TABLA
-- ---------------------------------------------------------------------------------------------
create table branches (
  id              bigint generated always as identity primary key,
  company_id      bigint not null default nullif(current_setting('app.company_id', true), '')::bigint
                    references companies(id) on delete cascade,
  -- Consecutivo POR EMPRESA y no una secuencia global: una global le diría a cada cliente cuántas
  -- sucursales tienen los demás.
  branch_number   int not null,
  -- Va en el ticket y puede servir de serie fiscal: no cambia una vez creado (trigger abajo).
  code            citext not null,
  name            text not null,
  is_headquarters boolean not null default false,
  is_active       boolean not null default true,
  address         text,
  phone           text,
  -- Lugar de expedición del CFDI: el SAT pide el código postal de la sucursal que emite.
  postal_code     text,
  -- Nulo = el de la empresa (business_settings.timezone). La hora del CFDI es la local del lugar.
  timezone        text,
  created_at      timestamptz not null default now(),

  constraint branches_number_per_company unique (company_id, branch_number),
  constraint branches_code_per_company unique (company_id, code),
  constraint branches_number_positive check (branch_number > 0),
  constraint branches_code_shape check (code ~ '^[A-Z0-9]{1,10}$'),
  constraint branches_name_len check (char_length(name) between 1 and 60),
  constraint branches_postal_code_shape check (postal_code is null or postal_code ~ '^[0-9]{5}$')
);

create unique index branches_one_headquarters on branches (company_id) where is_headquarters;
-- Para las llaves compuestas: los chequeos de llave foránea SALTAN RLS (0040, 0041, 0073).
create unique index branches_tenant_key on branches (company_id, id);

alter table branches enable row level security;
create policy tenant_isolation on branches
  using      (company_id = nullif(current_setting('app.company_id', true), '')::bigint)
  with check (company_id = nullif(current_setting('app.company_id', true), '')::bigint);
-- Sin `delete`: una sucursal con historia se desactiva, no se borra.
grant select, insert, update on branches to gatobobah_app;

-- ---------------------------------------------------------------------------------------------
-- 2. RESOLVER LA SUCURSAL
-- ---------------------------------------------------------------------------------------------

-- +goose StatementBegin
create function branch_for_company(p_company bigint) returns bigint
language plpgsql stable as $$
declare
  -- `app.branch_id` no lo pone nadie todavía: es la puerta del selector de sucursal. `nullif`
  -- por lo mismo que la 0074: una conexión reciclada trae cadena vacía, no NULL.
  selected text := nullif(current_setting('app.branch_id', true), '');
  found_id bigint;
  active   int;
begin
  if p_company is null then
    raise exception 'branch_ambiguous: no hay empresa para resolver la sucursal' using errcode = 'EGB01';
  end if;
  if selected is not null then
    select id into found_id from branches
     where id = selected::bigint and company_id = p_company and is_active;
    if found_id is null then
      raise exception 'branch_ambiguous: la sucursal elegida no es de esta empresa' using errcode = 'EGB01';
    end if;
    return found_id;
  end if;
  select count(*), min(id) into active, found_id from branches where company_id = p_company and is_active;
  if active <> 1 then
    raise exception 'branch_ambiguous: la empresa % tiene % sucursales activas y no hay una elegida', p_company, active
      using errcode = 'EGB01';
  end if;
  return found_id;
end;
$$;
-- +goose StatementEnd

-- +goose StatementBegin
create function current_branch_id() returns bigint
language sql stable as $$
  select branch_for_company(nullif(current_setting('app.company_id', true), '')::bigint)
$$;
-- +goose StatementEnd

-- El código de la matriz se deriva del slug. Vive en SQL y no en Go porque lo usan la migración y
-- `CreateCompany`, y dos copias de la regla terminarían diciendo cosas distintas.
-- +goose StatementBegin
create function headquarters_code(p_slug text) returns text
language sql immutable as $$
  select coalesce(nullif(left(upper(regexp_replace(p_slug, '[^a-zA-Z0-9]', '', 'g')), 10), ''), 'MATRIZ')
$$;
-- +goose StatementEnd

-- ---------------------------------------------------------------------------------------------
-- 3. REGLAS DE LA SUCURSAL
-- ---------------------------------------------------------------------------------------------

-- +goose StatementBegin
create function branches_before_insert() returns trigger
language plpgsql as $$
begin
  if new.branch_number is null then
    -- Candado por empresa: dos altas a la vez sacarían el mismo número. No se bloquea la fila de
    -- `companies` porque el rol de la app no tiene `update` sobre ella.
    perform pg_advisory_xact_lock(7600, new.company_id::int);
    select coalesce(max(branch_number), 0) + 1 into new.branch_number
      from branches where company_id = new.company_id;
  end if;
  new.code := upper(new.code);
  return new;
end;
$$;
-- +goose StatementEnd

create trigger trg_branches_before_insert before insert on branches
  for each row execute function branches_before_insert();

-- +goose StatementBegin
create function branches_before_update() returns trigger
language plpgsql as $$
begin
  if new.code is distinct from old.code then
    raise exception 'el código de una sucursal no cambia: ya está impreso en tickets' using errcode = '23514';
  end if;
  if new.company_id is distinct from old.company_id or new.branch_number is distinct from old.branch_number then
    raise exception 'una sucursal no cambia de empresa ni de número' using errcode = '23514';
  end if;
  if old.is_headquarters and not new.is_headquarters then
    raise exception 'la matriz no deja de serlo' using errcode = '23514';
  end if;
  if old.is_active and not new.is_active and not exists (
    select 1 from branches where company_id = old.company_id and is_active and id <> old.id
  ) then
    raise exception 'no se puede desactivar la última sucursal activa' using errcode = '23514';
  end if;
  return new;
end;
$$;
-- +goose StatementEnd

create trigger trg_branches_before_update before update on branches
  for each row execute function branches_before_update();

-- ---------------------------------------------------------------------------------------------
-- 4. UNA MATRIZ POR EMPRESA EXISTENTE
-- ---------------------------------------------------------------------------------------------
insert into branches (company_id, branch_number, code, name, is_headquarters, address, phone)
select c.id, 1, headquarters_code(c.slug), left(c.name, 60), true, bs.address, bs.phone
  from companies c
  left join business_settings bs on bs.company_id = c.id;

-- ---------------------------------------------------------------------------------------------
-- 5. QUIÉN ES DE QUÉ SUCURSAL
--
-- El relleno asigna la matriz DIRECTO, no por el turno: hay pedidos viejos sin turno (0061) y
-- la subconsulta los dejaría sin sucursal. Con una sucursal por empresa es lo mismo.
-- ---------------------------------------------------------------------------------------------
alter table cash_registers       add column branch_id bigint;
alter table platform_connections add column branch_id bigint;
alter table orders               add column branch_id bigint;
alter table stock_movements      add column branch_id bigint;
alter table stock_levels         add column branch_id bigint;
alter table expenses             add column branch_id bigint;

-- El relleno de `orders` no debe mover `updated_at`: es la hora de la última edición de quien
-- opera, y reescribirla en todos los pedidos de golpe borraría esa información.
alter table orders disable trigger trg_orders_updated;

update cash_registers t set branch_id = b.id from branches b where b.company_id = t.company_id and b.is_headquarters;
update platform_connections t set branch_id = b.id from branches b where b.company_id = t.company_id and b.is_headquarters;
update orders t set branch_id = b.id from branches b where b.company_id = t.company_id and b.is_headquarters;
update stock_movements t set branch_id = b.id from branches b where b.company_id = t.company_id and b.is_headquarters;
update stock_levels t set branch_id = b.id from branches b where b.company_id = t.company_id and b.is_headquarters;

alter table orders enable trigger trg_orders_updated;

alter table cash_registers       alter column branch_id set not null;
alter table platform_connections alter column branch_id set not null;
alter table orders               alter column branch_id set not null;
alter table stock_movements      alter column branch_id set not null;
alter table stock_levels         alter column branch_id set not null;

alter table cash_registers add constraint cash_registers_branch_of_company
  foreign key (company_id, branch_id) references branches (company_id, id) on delete restrict;
alter table platform_connections add constraint platform_connections_branch_of_company
  foreign key (company_id, branch_id) references branches (company_id, id) on delete restrict;
alter table orders add constraint orders_branch_of_company
  foreign key (company_id, branch_id) references branches (company_id, id) on delete restrict;
alter table stock_movements add constraint stock_movements_branch_of_company
  foreign key (company_id, branch_id) references branches (company_id, id) on delete restrict;
alter table stock_levels add constraint stock_levels_branch_of_company
  foreign key (company_id, branch_id) references branches (company_id, id) on delete restrict;
alter table expenses add constraint expenses_branch_of_company
  foreign key (company_id, branch_id) references branches (company_id, id) on delete restrict;

create index orders_branch on orders (company_id, branch_id);
create index stock_movements_branch on stock_movements (company_id, branch_id);

-- La caja principal es una por SUCURSAL: dos sucursales abren su caja principal a la vez.
drop index cash_registers_one_primary;
create unique index cash_registers_one_primary on cash_registers (company_id, branch_id) where is_primary;

-- ---------------------------------------------------------------------------------------------
-- 6. LLENAR LA SUCURSAL CUANDO EL INSERT NO LA TRAE
-- ---------------------------------------------------------------------------------------------

-- +goose StatementBegin
create function fill_branch_id() returns trigger
language plpgsql as $$
begin
  if new.branch_id is null then
    new.branch_id := branch_for_company(new.company_id);
  end if;
  return new;
end;
$$;
-- +goose StatementEnd

-- El pedido toma la sucursal de la caja de su turno: con dos sucursales, la del turno es la única
-- que dice dónde se vendió. Solo si el turno es de la misma empresa; si no, que la llave compuesta
-- del turno o de la sucursal lo rechace, no este trigger.
-- +goose StatementBegin
create function fill_order_branch_id() returns trigger
language plpgsql as $$
begin
  if new.branch_id is null and new.register_session_id is not null then
    select r.branch_id into new.branch_id
      from register_sessions s
      join cash_registers r on r.id = s.register_id
     where s.id = new.register_session_id and r.company_id = new.company_id;
  end if;
  if new.branch_id is null then
    new.branch_id := branch_for_company(new.company_id);
  end if;
  return new;
end;
$$;
-- +goose StatementEnd

create trigger trg_cash_registers_branch before insert on cash_registers
  for each row execute function fill_branch_id();
create trigger trg_platform_connections_branch before insert on platform_connections
  for each row execute function fill_branch_id();
create trigger trg_stock_movements_branch before insert on stock_movements
  for each row execute function fill_branch_id();
create trigger trg_orders_branch before insert on orders
  for each row execute function fill_order_branch_id();

-- ---------------------------------------------------------------------------------------------
-- 7. EXISTENCIAS POR SUCURSAL
--
-- El destino del `on conflict` cambia junto con los únicos: si no, cada venta truena con «no
-- unique or exclusion constraint matching». La empresa va explícita y no por el ajuste de sesión,
-- por la misma razón que `fill_branch_id`.
-- ---------------------------------------------------------------------------------------------
alter table stock_levels drop constraint stock_levels_ingredient_id_key;
alter table stock_levels drop constraint stock_levels_product_id_key;
alter table stock_levels add constraint stock_levels_ingredient_per_branch unique (branch_id, ingredient_id);
alter table stock_levels add constraint stock_levels_product_per_branch unique (branch_id, product_id);

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

-- ---------------------------------------------------------------------------------------------
-- 8. COMPROBACIÓN: ni una fila sin sucursal, una matriz por empresa
-- ---------------------------------------------------------------------------------------------
-- +goose StatementBegin
do $$
begin
  if exists (select 1 from companies c where not exists (
       select 1 from branches b where b.company_id = c.id and b.is_headquarters)) then
    raise exception 'quedó una empresa sin matriz';
  end if;
end $$;
-- +goose StatementEnd

-- +goose Down

set local lock_timeout = '3s';

-- +goose StatementBegin
do $$
begin
  if exists (select 1 from branches group by company_id having count(*) > 1) then
    raise exception 'hay empresas con más de una sucursal: bajar la 0076 mezclaría sus cajas, pedidos y existencias';
  end if;
end $$;
-- +goose StatementEnd

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

alter table stock_levels drop constraint stock_levels_ingredient_per_branch;
alter table stock_levels drop constraint stock_levels_product_per_branch;
alter table stock_levels add constraint stock_levels_ingredient_id_key unique (ingredient_id);
alter table stock_levels add constraint stock_levels_product_id_key unique (product_id);

drop trigger trg_cash_registers_branch on cash_registers;
drop trigger trg_platform_connections_branch on platform_connections;
drop trigger trg_stock_movements_branch on stock_movements;
drop trigger trg_orders_branch on orders;
drop function fill_branch_id();
drop function fill_order_branch_id();

drop index cash_registers_one_primary;
create unique index cash_registers_one_primary on cash_registers (company_id) where is_primary;

alter table cash_registers       drop column branch_id;
alter table platform_connections drop column branch_id;
alter table orders               drop column branch_id;
alter table stock_movements      drop column branch_id;
alter table stock_levels         drop column branch_id;
alter table expenses             drop column branch_id;

drop table branches;
drop function current_branch_id();
drop function branch_for_company(bigint);
drop function headquarters_code(text);
drop function branches_before_insert();
drop function branches_before_update();
