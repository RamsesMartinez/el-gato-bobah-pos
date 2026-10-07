-- +goose Up

-- EL ALMACÉN DESCUENTA LO QUE DE VERDAD SE VENDIÓ (spec 028).
--
-- Hasta aquí solo bajaba el producto principal: los extras, los componentes de un paquete y los
-- pedidos de plataforma salían del local sin tocar el almacén. Cuatro cosas:
--
-- 1. SI LA COMPOSICIÓN ES ESTIMADA O CONFIRMADA. Lo que vino de FUDO es un estimado y descuenta desde
--    que se carga (decisión del dueño, 2026-10-06); confirmarlo es de quien administra, no de la
--    carga. Un producto con existencias propias queda confirmado: su composición es él mismo, y sin
--    esto llenaría el filtro «sin capturar» desde el primer día.
-- 2. EL ORIGEN DE CADA MOVIMIENTO: el extra que lo causó o el paquete del que salió el componente.
-- 3. LOS COMPONENTES DE UN PAQUETE VENDIDO, copiados al vender: si el paquete se edita después, el
--    historial de lo que se vendió no cambia.
-- 4. LA OPCIÓN EMPAREJADA DE UN RENGLÓN DE PLATAFORMA, para descontarla al aceptar el pedido.

set local lock_timeout = '3s';

-- ---------------------------------------------------------------------------------------------
-- 1. ESTADO DE LA COMPOSICIÓN
-- ---------------------------------------------------------------------------------------------
alter table products add column composition_status text;
alter table products add column composition_confirmed_by bigint references users(id) on delete set null;
alter table products add column composition_confirmed_at timestamptz;
alter table modifier_options add column composition_status text;
alter table modifier_options add column composition_confirmed_by bigint references users(id) on delete set null;
alter table modifier_options add column composition_confirmed_at timestamptz;
alter table ingredients add column composition_status text;
alter table ingredients add column composition_confirmed_by bigint references users(id) on delete set null;
alter table ingredients add column composition_confirmed_at timestamptz;

-- Un paquete ya armado en el POS lo configuró alguien aquí, no vino de FUDO: queda confirmado. El
-- confirmador nulo dice que lo confirmó la migración y no una persona.
update products set composition_status = 'confirmed', composition_confirmed_at = now()
 where track_stock or type = 'combo';
update products set composition_status = 'estimated'
 where composition_status is null and recipe_id is not null;
update modifier_options set composition_status = 'estimated'
 where recipe_id is not null or linked_product_id is not null;
update ingredients set composition_status = 'estimated' where is_prep;

alter table products add constraint products_composition_status check (
  composition_status in ('estimated', 'confirmed')
  and (composition_status = 'estimated' or composition_confirmed_at is not null));
alter table modifier_options add constraint modifier_options_composition_status check (
  composition_status in ('estimated', 'confirmed')
  and (composition_status = 'estimated' or composition_confirmed_at is not null));
alter table ingredients add constraint ingredients_composition_status check (
  composition_status in ('estimated', 'confirmed')
  and (composition_status = 'estimated' or composition_confirmed_at is not null));

-- ---------------------------------------------------------------------------------------------
-- 2. ORIGEN DEL MOVIMIENTO
--
-- FK simple con `set null`: el libro del almacén es inmutable. Borrar un extra viejo no se bloquea
-- por sus movimientos ni se los lleva; se pierde la etiqueta, no el movimiento. Compuesta no puede
-- ser: con `set null` pondría en nulo también `company_id`.
-- ---------------------------------------------------------------------------------------------
alter table stock_movements add column modifier_option_id bigint references modifier_options(id) on delete set null;
alter table stock_movements add column component_of_product_id bigint references products(id) on delete set null;

-- ---------------------------------------------------------------------------------------------
-- 3. COMPONENTES DE UN PAQUETE VENDIDO
-- ---------------------------------------------------------------------------------------------
alter table order_lines add constraint order_lines_id_company_key unique (id, company_id);

create table order_line_components (
  id            bigint generated always as identity primary key,
  order_line_id bigint not null,
  product_id    bigint not null,
  quantity      numeric(14,4) not null check (quantity > 0),
  company_id    bigint not null default nullif(current_setting('app.company_id', true), '')::bigint
                  references companies(id) on delete cascade,
  constraint order_line_components_line foreign key (order_line_id, company_id)
    references order_lines (id, company_id) on delete cascade,
  -- restrict, como los renglones: el reorg de datos borra productos, y llevarse lo vendido sería
  -- reescribir el historial.
  constraint order_line_components_product foreign key (company_id, product_id)
    references products (company_id, id) on delete restrict
);
create index order_line_components_line on order_line_components (order_line_id);
create index order_line_components_product on order_line_components (company_id, product_id);

alter table order_line_components enable row level security;
create policy tenant_isolation on order_line_components
  using      (company_id = nullif(current_setting('app.company_id', true), '')::bigint)
  with check (company_id = nullif(current_setting('app.company_id', true), '')::bigint);
grant select, insert on order_line_components to gatobobah_app;

-- ---------------------------------------------------------------------------------------------
-- 4. LA OPCIÓN EMPAREJADA DEL RENGLÓN DE PLATAFORMA
-- ---------------------------------------------------------------------------------------------
alter table platform_incoming_order_lines add column modifier_option_id bigint;
alter table platform_incoming_order_lines add constraint platform_incoming_order_lines_option_of_company
  foreign key (modifier_option_id, company_id) references modifier_options (id, company_id) on delete restrict;

-- +goose Down

set local lock_timeout = '3s';

alter table platform_incoming_order_lines drop constraint platform_incoming_order_lines_option_of_company;
alter table platform_incoming_order_lines drop column modifier_option_id;

drop table order_line_components;
alter table order_lines drop constraint order_lines_id_company_key;

alter table stock_movements drop column component_of_product_id;
alter table stock_movements drop column modifier_option_id;

alter table ingredients drop constraint ingredients_composition_status;
alter table ingredients drop column composition_confirmed_at;
alter table ingredients drop column composition_confirmed_by;
alter table ingredients drop column composition_status;
alter table modifier_options drop constraint modifier_options_composition_status;
alter table modifier_options drop column composition_confirmed_at;
alter table modifier_options drop column composition_confirmed_by;
alter table modifier_options drop column composition_status;
alter table products drop constraint products_composition_status;
alter table products drop column composition_confirmed_at;
alter table products drop column composition_confirmed_by;
alter table products drop column composition_status;
