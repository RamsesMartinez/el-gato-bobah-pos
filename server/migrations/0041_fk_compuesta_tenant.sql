-- +goose Up
-- Cierra la MISMA clase de defecto que 0040, pero sobre las tablas que sí mueven dinero.
--
-- 0040 blindó las dos tablas de precios de plataforma, que hoy están vacías. La auditoría de
-- esquema mostró que el hueco pesaba mucho más en otro lado: `order_lines.product_id`,
-- `order_line_modifiers.modifier_option_id` y `orders.delivery_platform_id` son cada línea de cada
-- ticket vendido, y referencian tablas per-tenant por id simple. Como los chequeos de integridad
-- referencial de Postgres SALTAN RLS, nada en el esquema impedía que el company_id del renglón y el
-- del producto divergieran. RLS y las validaciones de los servicios cierran la vía HTTP, pero
-- cualquier escritura que corra como owner —un data-fix de docs/reorg/, un backfill como el de
-- 0037— podía cruzarlas sin una sola protesta, y el error aparecía en el corte de caja, no en el
-- insert.
--
-- Se cierran las 12 de un golpe y no solo las tres de dinero: el mecanismo es idéntico en todas, y
-- dejar la mitad abierta deja también abierta la ruta para que un dato malo entre por el catálogo y
-- termine en una venta.
--
-- Los índices únicos destino ya existen: products/modifier_options desde 0040 y delivery_platforms
-- desde 0037.

set local lock_timeout = '3s';

-- +goose StatementBegin
do $$
declare
  r record;
  n bigint;
  cruces text := '';
begin
  -- Un solo recorrido que REPORTA TODO lo cruzado antes de tocar nada. Sin esto, la migración
  -- abortaría en el primer ALTER que topara con una fila mala y habría que repetir el deploy por
  -- cada tabla; y el mensaje de Postgres dice el nombre del constraint, no cuántas filas ni dónde.
  for r in
    select * from (values
      ('order_lines','product_id','products'),
      ('order_line_modifiers','modifier_option_id','modifier_options'),
      ('orders','delivery_platform_id','delivery_platforms'),
      ('expense_items','product_id','products'),
      ('stock_movements','product_id','products'),
      ('stock_levels','product_id','products'),
      ('supplier_items','product_id','products'),
      ('product_channels','product_id','products'),
      ('product_modifier_groups','product_id','products'),
      ('combo_slots','combo_id','products'),
      ('combo_slot_products','product_id','products'),
      ('modifier_options','linked_product_id','products')
    ) as t(hija, col, padre)
  loop
    execute format(
      'select count(*) from %I h join %I p on p.id = h.%I where p.company_id <> h.company_id',
      r.hija, r.padre, r.col) into n;
    if n > 0 then
      cruces := cruces || format(', %s.%s: %s fila(s)', r.hija, r.col, n);
    end if;
  end loop;

  if cruces <> '' then
    raise exception 'hay filas que cruzan empresas y la llave compuesta las rechazaría%', cruces
      using hint = 'Corrigelas a mano (o borra las de prueba) y vuelve a desplegar. Una fila asi ya esta contando dinero en la empresa equivocada.';
  end if;
end
$$;
-- +goose StatementEnd

-- El ON DELETE de cada una se conserva tal cual estaba: cambiarlo aquí sería colar una decisión de
-- borrado dentro de un cambio de aislamiento.
alter table order_lines drop constraint order_lines_product_id_fkey;
alter table order_lines add constraint order_lines_product_fkey
  foreign key (product_id, company_id) references products (id, company_id);

alter table order_line_modifiers drop constraint order_line_modifiers_modifier_option_id_fkey;
alter table order_line_modifiers add constraint order_line_modifiers_option_fkey
  foreign key (modifier_option_id, company_id) references modifier_options (id, company_id);

alter table orders drop constraint orders_delivery_platform_id_fkey;
alter table orders add constraint orders_delivery_platform_fkey
  foreign key (delivery_platform_id, company_id) references delivery_platforms (id, company_id);

alter table expense_items drop constraint expense_items_product_id_fkey;
alter table expense_items add constraint expense_items_product_fkey
  foreign key (product_id, company_id) references products (id, company_id);

alter table stock_movements drop constraint stock_movements_product_id_fkey;
alter table stock_movements add constraint stock_movements_product_fkey
  foreign key (product_id, company_id) references products (id, company_id);

alter table stock_levels drop constraint stock_levels_product_id_fkey;
alter table stock_levels add constraint stock_levels_product_fkey
  foreign key (product_id, company_id) references products (id, company_id);

alter table supplier_items drop constraint supplier_items_product_id_fkey;
alter table supplier_items add constraint supplier_items_product_fkey
  foreign key (product_id, company_id) references products (id, company_id) on delete cascade;

alter table product_channels drop constraint product_channels_product_id_fkey;
alter table product_channels add constraint product_channels_product_fkey
  foreign key (product_id, company_id) references products (id, company_id) on delete cascade;

alter table product_modifier_groups drop constraint product_modifier_groups_product_id_fkey;
alter table product_modifier_groups add constraint product_modifier_groups_product_fkey
  foreign key (product_id, company_id) references products (id, company_id) on delete cascade;

alter table combo_slots drop constraint combo_slots_combo_id_fkey;
alter table combo_slots add constraint combo_slots_combo_fkey
  foreign key (combo_id, company_id) references products (id, company_id) on delete cascade;

alter table combo_slot_products drop constraint combo_slot_products_product_id_fkey;
alter table combo_slot_products add constraint combo_slot_products_product_fkey
  foreign key (product_id, company_id) references products (id, company_id);

alter table modifier_options drop constraint modifier_options_linked_product_id_fkey;
alter table modifier_options add constraint modifier_options_linked_product_fkey
  foreign key (linked_product_id, company_id) references products (id, company_id);

-- +goose Down
alter table modifier_options drop constraint modifier_options_linked_product_fkey;
alter table modifier_options add constraint modifier_options_linked_product_id_fkey
  foreign key (linked_product_id) references products (id);

alter table combo_slot_products drop constraint combo_slot_products_product_fkey;
alter table combo_slot_products add constraint combo_slot_products_product_id_fkey
  foreign key (product_id) references products (id);

alter table combo_slots drop constraint combo_slots_combo_fkey;
alter table combo_slots add constraint combo_slots_combo_id_fkey
  foreign key (combo_id) references products (id) on delete cascade;

alter table product_modifier_groups drop constraint product_modifier_groups_product_fkey;
alter table product_modifier_groups add constraint product_modifier_groups_product_id_fkey
  foreign key (product_id) references products (id) on delete cascade;

alter table product_channels drop constraint product_channels_product_fkey;
alter table product_channels add constraint product_channels_product_id_fkey
  foreign key (product_id) references products (id) on delete cascade;

alter table supplier_items drop constraint supplier_items_product_fkey;
alter table supplier_items add constraint supplier_items_product_id_fkey
  foreign key (product_id) references products (id) on delete cascade;

alter table stock_levels drop constraint stock_levels_product_fkey;
alter table stock_levels add constraint stock_levels_product_id_fkey
  foreign key (product_id) references products (id);

alter table stock_movements drop constraint stock_movements_product_fkey;
alter table stock_movements add constraint stock_movements_product_id_fkey
  foreign key (product_id) references products (id);

alter table expense_items drop constraint expense_items_product_fkey;
alter table expense_items add constraint expense_items_product_id_fkey
  foreign key (product_id) references products (id);

alter table orders drop constraint orders_delivery_platform_fkey;
alter table orders add constraint orders_delivery_platform_id_fkey
  foreign key (delivery_platform_id) references delivery_platforms (id);

alter table order_line_modifiers drop constraint order_line_modifiers_option_fkey;
alter table order_line_modifiers add constraint order_line_modifiers_modifier_option_id_fkey
  foreign key (modifier_option_id) references modifier_options (id);

alter table order_lines drop constraint order_lines_product_fkey;
alter table order_lines add constraint order_lines_product_id_fkey
  foreign key (product_id) references products (id);
