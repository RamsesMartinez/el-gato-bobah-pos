-- +goose Up
-- Llave foránea COMPUESTA para los precios de plataforma: el esquema, y no solo el servicio, se
-- niega a que una empresa escriba un precio sobre el producto, la opción o la plataforma de otra.
--
-- Por qué hace falta si ya hay RLS: los chequeos de integridad referencial de Postgres SALTAN las
-- políticas de RLS por diseño (corren como el dueño de la tabla referenciada). Así que
-- `product_id references products(id)` acepta cualquier id existente, venga de la empresa que
-- venga. La fila quedaba con el company_id del atacante ocupando la llave primaria global
-- (product_id, platform_id): el dueño legítimo ya no podía capturar su propio precio —su upsert
-- caía en el ON CONFLICT y chocaba con la política, saliendo como 500— ni borrar la fila intrusa,
-- porque bajo RLS no la ve. Irreparable desde el producto.
--
-- La validación de pertenencia en PlatformPricesService cierra la vía por HTTP; esta migración
-- cierra la clase entera, incluido el próximo endpoint que se escriba sin acordarse de validar.
-- Es el mismo patrón que 0037 usó para payment_methods → delivery_platforms.

-- +goose StatementBegin
do $$
declare
  cruzados bigint;
begin
  select count(*) into cruzados
    from product_platform_prices p join products pr on pr.id = p.product_id
   where pr.company_id <> p.company_id;
  if cruzados > 0 then
    raise exception 'product_platform_prices tiene % fila(s) cuyo producto es de otra empresa; '
      'resuélvelas a mano antes de aplicar la llave compuesta', cruzados;
  end if;

  select count(*) into cruzados
    from modifier_option_platform_prices m join modifier_options o on o.id = m.option_id
   where o.company_id <> m.company_id;
  if cruzados > 0 then
    raise exception 'modifier_option_platform_prices tiene % fila(s) cuya opción es de otra empresa; '
      'resuélvelas a mano antes de aplicar la llave compuesta', cruzados;
  end if;

  -- La plataforma también, aunque el propio ALTER la rechazaría: lo que cambia es el mensaje. Un
  -- deploy que aborta con "violates foreign key constraint" obliga a ir a buscar qué fila fue; uno
  -- que aborta diciendo la tabla y el conteo se arregla de inmediato.
  select count(*) into cruzados
    from (
      select 1 from product_platform_prices p join delivery_platforms d on d.id = p.platform_id
       where d.company_id <> p.company_id
      union all
      select 1 from modifier_option_platform_prices m join delivery_platforms d on d.id = m.platform_id
       where d.company_id <> m.company_id
    ) x;
  if cruzados > 0 then
    raise exception 'hay % precio(s) apuntando a la plataforma de otra empresa; '
      'resuélvelos a mano antes de aplicar la llave compuesta', cruzados;
  end if;
end
$$;
-- +goose StatementEnd

-- lock_timeout: cada ALTER toma un lock exclusivo breve sobre products/modifier_options. Con este
-- volumen la ventana es de milisegundos, pero si justo hay una transacción larga abierta sobre la
-- tabla, el ALTER se encola detrás de ella Y arrastra a toda lectura nueva. Es preferible que la
-- migración falle limpio en 3 segundos —se reintenta— a que el POS se quede mudo esperando.
set local lock_timeout = '3s';

-- El destino de una FK compuesta necesita un índice único con ESAS dos columnas en ESE orden.
-- (id, company_id) y no al revés: id ya es único por sí solo, así que el índice queda igual de
-- selectivo y sirve además para buscar por id sin la empresa.
alter table products         add constraint products_id_company_key         unique (id, company_id);
alter table modifier_options add constraint modifier_options_id_company_key unique (id, company_id);

alter table product_platform_prices drop constraint product_platform_prices_product_id_fkey;
alter table product_platform_prices add constraint product_platform_prices_product_fkey
  foreign key (product_id, company_id) references products (id, company_id) on delete cascade;
-- La plataforma no cascadea, igual que en 0037: borrar una plataforma no debe llevarse en silencio
-- una lista de precios capturada a mano.
alter table product_platform_prices drop constraint product_platform_prices_platform_id_fkey;
alter table product_platform_prices add constraint product_platform_prices_platform_fkey
  foreign key (platform_id, company_id) references delivery_platforms (id, company_id);

alter table modifier_option_platform_prices drop constraint modifier_option_platform_prices_option_id_fkey;
alter table modifier_option_platform_prices add constraint modifier_option_platform_prices_option_fkey
  foreign key (option_id, company_id) references modifier_options (id, company_id) on delete cascade;
alter table modifier_option_platform_prices drop constraint modifier_option_platform_prices_platform_id_fkey;
alter table modifier_option_platform_prices add constraint modifier_option_platform_prices_platform_fkey
  foreign key (platform_id, company_id) references delivery_platforms (id, company_id);

-- +goose Down
alter table modifier_option_platform_prices drop constraint modifier_option_platform_prices_platform_fkey;
alter table modifier_option_platform_prices add constraint modifier_option_platform_prices_platform_id_fkey
  foreign key (platform_id) references delivery_platforms (id);
alter table modifier_option_platform_prices drop constraint modifier_option_platform_prices_option_fkey;
alter table modifier_option_platform_prices add constraint modifier_option_platform_prices_option_id_fkey
  foreign key (option_id) references modifier_options (id) on delete cascade;

alter table product_platform_prices drop constraint product_platform_prices_platform_fkey;
alter table product_platform_prices add constraint product_platform_prices_platform_id_fkey
  foreign key (platform_id) references delivery_platforms (id);
alter table product_platform_prices drop constraint product_platform_prices_product_fkey;
alter table product_platform_prices add constraint product_platform_prices_product_id_fkey
  foreign key (product_id) references products (id) on delete cascade;

alter table modifier_options drop constraint modifier_options_id_company_key;
alter table products         drop constraint products_id_company_key;
