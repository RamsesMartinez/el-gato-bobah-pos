-- +goose Up

-- EMPAREJAR LA TIENDA DE PLATAFORMA CONECTADA (spec 026).
--
-- Cuatro cosas, todas sobre el emparejamiento que dejó la 0071:
--
-- 1. UNA PAREJA PUEDE APUNTAR A UNA OPCIÓN DEL POS. La 0071 previó `local_kind` pero solo tenía
--    `product_id`: guardar una opción escribía su id en la columna de productos, y si coincidía con
--    un producto real, la pareja quedaba ligada al producto equivocado sin un solo error.
-- 2. EL PRECIO DE CAPTURA. Varios platillos de la plataforma pueden ir al mismo producto (cada crepa
--    a «Arma tu Crepa»). Cuál de sus precios cobra la captura a mano lo decide quien configura, una
--    vez, aquí; al operar nadie decide nada (constitución, 2026-10-03).
-- 3. LAS DECISIONES SIN PAREJA. «Solo existe en la plataforma» y «no se vende en la plataforma»:
--    sin guardarlas, «Sin pareja» nunca llega a cero y lo pendiente se confunde con lo decidido.
-- 4. QUIÉN PUSO EL PRECIO. Con la plataforma conectada, el precio lo pone la plataforma y lo
--    escribe la lectura del menú; la captura a mano de esa fila se rechaza. Lo que cambió cada
--    lectura queda escrito para el aviso de la pantalla.
--
-- Y el producto genérico: un platillo de un pedido de plataforma sin pareja entra ligado a él, con
-- el nombre y las opciones de la plataforma, y sale en la comanda como cualquier renglón.

set local lock_timeout = '3s';

-- ---------------------------------------------------------------------------------------------
-- 0. GUARDA: parejas de opción guardadas como producto por el defecto de arriba. No se adivina qué
-- se quiso ligar; se aborta con los ids para decidirlo a mano.
-- ---------------------------------------------------------------------------------------------
-- +goose StatementBegin
do $$
declare
  n int;
  ids text;
begin
  select count(*), string_agg(connection_id || ':' || external_id, ', ')
    into n, ids
    from platform_item_links where kind = 'opcion';
  if n > 0 then
    raise exception 'hay % parejas de opciones guardadas como producto (%): revisarlas a mano antes de migrar', n, ids;
  end if;
end $$;
-- +goose StatementEnd

-- ---------------------------------------------------------------------------------------------
-- 1. PAREJA A PRODUCTO O A OPCIÓN, Y PRECIO DE CAPTURA
-- ---------------------------------------------------------------------------------------------
alter table platform_item_links alter column product_id drop not null;
alter table platform_item_links add column modifier_option_id bigint;
alter table platform_item_links add column is_capture_price boolean not null default false;

alter table platform_item_links add constraint platform_item_links_option_of_company
  foreign key (modifier_option_id, company_id) references modifier_options (id, company_id) on delete restrict;
alter table platform_item_links add constraint platform_item_links_one_target check (
  (local_kind = 'producto') = (product_id is not null)
  and (local_kind = 'opcion_de_modificador') = (modifier_option_id is not null)
);
-- Un platillo se liga a un producto y una opción a una opción: lo otro es el defecto de arriba.
alter table platform_item_links add constraint platform_item_links_kind_matches check (
  (kind = 'opcion') = (local_kind = 'opcion_de_modificador')
);

create unique index platform_item_links_capture_product
  on platform_item_links (connection_id, product_id) where is_capture_price and product_id is not null;
create unique index platform_item_links_capture_option
  on platform_item_links (connection_id, modifier_option_id) where is_capture_price and modifier_option_id is not null;
create index platform_item_links_por_opcion on platform_item_links (company_id, modifier_option_id)
  where modifier_option_id is not null;

-- Relleno: de las parejas de cada producto en cada tienda, la más reciente da el precio de captura.
update platform_item_links l set is_capture_price = true
 where l.product_id is not null
   and not exists (
     select 1 from platform_item_links o
      where o.connection_id = l.connection_id and o.product_id = l.product_id
        and (o.created_at, o.external_id) > (l.created_at, l.external_id));

-- ---------------------------------------------------------------------------------------------
-- 2. DECISIONES SIN PAREJA
-- ---------------------------------------------------------------------------------------------
create table platform_item_exclusions (
  connection_id bigint not null,
  external_id   text not null,
  kind          platform_item_kind not null,
  decided_by    bigint not null references users(id),
  decided_at    timestamptz not null default now(),
  company_id    bigint not null default nullif(current_setting('app.company_id', true), '')::bigint
                  references companies(id) on delete cascade,
  primary key (connection_id, external_id),
  -- La decisión es de esa tienda: borrar la tienda se la lleva (y el aviso previo la cuenta).
  constraint platform_item_exclusions_connection foreign key (company_id, connection_id)
    references platform_connections (company_id, id) on delete cascade,
  constraint platform_item_exclusions_id_bounded check (char_length(external_id) between 1 and 200)
);

create table local_item_exclusions (
  id                 bigint generated always as identity primary key,
  connection_id      bigint not null,
  product_id         bigint,
  modifier_option_id bigint,
  decided_by         bigint not null references users(id),
  decided_at         timestamptz not null default now(),
  company_id         bigint not null default nullif(current_setting('app.company_id', true), '')::bigint
                       references companies(id) on delete cascade,
  constraint local_item_exclusions_connection foreign key (company_id, connection_id)
    references platform_connections (company_id, id) on delete cascade,
  -- restrict, como las parejas (0071): el reorg de datos borra productos, y una decisión manual
  -- no debe desaparecer en silencio.
  constraint local_item_exclusions_product foreign key (company_id, product_id)
    references products (company_id, id) on delete restrict,
  constraint local_item_exclusions_option foreign key (modifier_option_id, company_id)
    references modifier_options (id, company_id) on delete restrict,
  constraint local_item_exclusions_one_target check ((product_id is null) <> (modifier_option_id is null))
);
create unique index local_item_exclusions_product_once on local_item_exclusions (connection_id, product_id)
  where product_id is not null;
create unique index local_item_exclusions_option_once on local_item_exclusions (connection_id, modifier_option_id)
  where modifier_option_id is not null;

-- ---------------------------------------------------------------------------------------------
-- 3. QUIÉN PUSO EL PRECIO, Y QUÉ CAMBIÓ CADA LECTURA
-- ---------------------------------------------------------------------------------------------
alter table product_platform_prices add column source text not null default 'manual';
alter table product_platform_prices add column synced_at timestamptz;
alter table product_platform_prices add constraint product_platform_prices_source check (
  source in ('manual', 'platform') and (source = 'manual' or synced_at is not null));

alter table modifier_option_platform_prices add column source text not null default 'manual';
alter table modifier_option_platform_prices add column synced_at timestamptz;
alter table modifier_option_platform_prices add constraint modifier_option_platform_prices_source check (
  source in ('manual', 'platform') and (source = 'manual' or synced_at is not null));

create table platform_price_changes (
  id                 bigint generated always as identity primary key,
  -- Se poda con su lectura: el aviso es de la última, no un histórico.
  read_id            bigint not null references platform_menu_reads(id) on delete cascade,
  external_id        text not null,
  name               text not null,
  product_id         bigint,
  modifier_option_id bigint,
  old_price          numeric(10,2),
  new_price          numeric(10,2) not null,
  company_id         bigint not null default nullif(current_setting('app.company_id', true), '')::bigint
                       references companies(id) on delete cascade,
  constraint platform_price_changes_product foreign key (company_id, product_id)
    references products (company_id, id) on delete cascade,
  constraint platform_price_changes_option foreign key (modifier_option_id, company_id)
    references modifier_options (id, company_id) on delete cascade,
  constraint platform_price_changes_one_target check ((product_id is null) <> (modifier_option_id is null))
);
create index platform_price_changes_read on platform_price_changes (company_id, read_id);

-- ---------------------------------------------------------------------------------------------
-- 4. EL PRODUCTO GENÉRICO
--
-- Se encuentra por `system_kind`, no por el nombre, que quien opera puede cambiar. Inactivo para que
-- no aparezca al vender; `needs_prep` para que salga en la comanda; sin receta ni existencias
-- porque no se sabe qué lleva. El renglón trae el precio de la plataforma, no el del producto.
-- ---------------------------------------------------------------------------------------------
alter table products add column system_kind text;
alter table products add constraint products_system_kind check (system_kind in ('platform_unpaired'));
create unique index products_one_system_kind on products (company_id, system_kind) where system_kind is not null;

-- +goose StatementBegin
create function ensure_platform_unpaired_product(p_company bigint) returns bigint
language plpgsql as $$
declare
  category bigint;
  product  bigint;
begin
  select id into product from products where company_id = p_company and system_kind = 'platform_unpaired';
  if product is not null then
    return product;
  end if;
  select id into category from categories
   where company_id = p_company and parent_id is null and name = 'Plataformas';
  if category is null then
    insert into categories (company_id, name, is_active) values (p_company, 'Plataformas', false)
      returning id into category;
  end if;
  insert into products (company_id, name, category_id, price, is_active, needs_prep, system_kind)
  values (p_company, 'Platillo de plataforma sin pareja', category, 0, false, true, 'platform_unpaired')
  returning id into product;
  return product;
end;
$$;
-- +goose StatementEnd

select ensure_platform_unpaired_product(id) from companies;

-- ---------------------------------------------------------------------------------------------
-- 5. RLS Y GRANTS de las tablas nuevas (el grant no se hereda, 0024)
-- ---------------------------------------------------------------------------------------------
alter table platform_item_exclusions enable row level security;
create policy tenant_isolation on platform_item_exclusions
  using      (company_id = nullif(current_setting('app.company_id', true), '')::bigint)
  with check (company_id = nullif(current_setting('app.company_id', true), '')::bigint);
grant select, insert, delete on platform_item_exclusions to gatobobah_app;

alter table local_item_exclusions enable row level security;
create policy tenant_isolation on local_item_exclusions
  using      (company_id = nullif(current_setting('app.company_id', true), '')::bigint)
  with check (company_id = nullif(current_setting('app.company_id', true), '')::bigint);
grant select, insert, delete on local_item_exclusions to gatobobah_app;

alter table platform_price_changes enable row level security;
create policy tenant_isolation on platform_price_changes
  using      (company_id = nullif(current_setting('app.company_id', true), '')::bigint)
  with check (company_id = nullif(current_setting('app.company_id', true), '')::bigint);
grant select, insert, delete on platform_price_changes to gatobobah_app;

-- +goose Down

set local lock_timeout = '3s';

-- +goose StatementBegin
do $$
begin
  if exists (select 1 from platform_item_links where modifier_option_id is not null) then
    raise exception 'hay parejas con opciones del POS: bajar la 0077 las perdería';
  end if;
end $$;
-- +goose StatementEnd

drop table platform_price_changes;
drop table local_item_exclusions;
drop table platform_item_exclusions;

-- El producto genérico se queda si ya tiene renglones: borrarlo rompería pedidos. Solo pierde la marca.
delete from products p where system_kind = 'platform_unpaired'
   and not exists (select 1 from order_lines l where l.product_id = p.id);
drop index products_one_system_kind;
alter table products drop constraint products_system_kind;
alter table products drop column system_kind;
drop function ensure_platform_unpaired_product(bigint);

alter table modifier_option_platform_prices drop constraint modifier_option_platform_prices_source;
alter table modifier_option_platform_prices drop column synced_at;
alter table modifier_option_platform_prices drop column source;
alter table product_platform_prices drop constraint product_platform_prices_source;
alter table product_platform_prices drop column synced_at;
alter table product_platform_prices drop column source;

drop index platform_item_links_por_opcion;
drop index platform_item_links_capture_option;
drop index platform_item_links_capture_product;
alter table platform_item_links drop constraint platform_item_links_kind_matches;
alter table platform_item_links drop constraint platform_item_links_one_target;
alter table platform_item_links drop constraint platform_item_links_option_of_company;
alter table platform_item_links drop column is_capture_price;
alter table platform_item_links drop column modifier_option_id;
alter table platform_item_links alter column product_id set not null;
