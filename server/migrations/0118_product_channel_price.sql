-- +goose Up
-- Dónde se vende cada producto y a qué precio (spec 092, #326).
--
-- Reusa el catálogo de lugares de venta de la 0004 (`channels` + `product_channels`, «sin fila = se
-- ofrece»): Mostrador = 'pos' y Web = 'online'. Una columna por lugar de venta cerraría la puerta
-- «una lista de productos por plataforma» (constitución VIII): cada lugar nuevo sería otra migración.
--
-- Tolera a la versión anterior corriendo (constitución IV): todo es aditivo. La API anterior lee el
-- menú por `code` y duplica productos con columnas explícitas; ninguna escanea los ids que se
-- ensanchan, y ninguna regla suya lee `allow_oversell`.
set local lock_timeout = '3s';

-- Ids anchos: `channels.id` es global entre empresas y cada empresa gasta uno por lugar de venta.
-- Con smallint el techo son unos miles de empresas; hoy son 6 filas y ensanchar cuesta nada.
alter table product_channels drop constraint product_channels_channel_id_fkey;
alter table category_channels drop constraint category_channels_channel_id_fkey;
alter table channels alter column id type bigint;
alter sequence channels_id_seq as bigint;
alter table product_channels alter column channel_id type bigint;
alter table category_channels alter column channel_id type bigint;
alter table category_channels add constraint category_channels_channel_id_fkey
  foreign key (channel_id) references channels (id);

-- El producto y su lugar de venta, de la misma empresa. Los chequeos de integridad saltan RLS: sin
-- la llave compuesta, un arreglo de datos como owner podría ligar el producto de una empresa al lugar
-- de venta de otra, y el menú lo mostraría sin fallar.
alter table channels add constraint channels_company_id_key unique (company_id, id);
alter table product_channels add constraint product_channels_channel_fkey
  foreign key (company_id, channel_id) references channels (company_id, id) not valid;
alter table product_channels validate constraint product_channels_channel_fkey;

-- Precio propio en ese lugar de venta. Null = el de mostrador; vacío nunca significa cero.
alter table product_channels add column price numeric(12,2)
  constraint product_channels_price_positive check (price is null or price > 0);

-- Toda empresa con su catálogo de lugares de venta. Las que creó la consola nacieron sin él, y apagar
-- «Web» en una de ellas no tendría a qué apuntar. Los nombres pasan a los de la pantalla.
insert into channels (company_id, code, name)
select c.id, v.code, v.name
  from companies c
 cross join (values ('pos', 'Mostrador'), ('qr', 'Menú QR'), ('online', 'Web')) v(code, name)
    on conflict (company_id, code) do update set name = excluded.name;

-- «Vender sin existencia» encendido en todo producto. La columna existe desde la 0004 y nadie la
-- aplicaba; sus `false` vienen de la columna «Vender sin stock» del export de FUDO (que vive fuera
-- del repositorio, en ~/gatobobah-datos/references/). Aplicarlos tal cual dejaría agotados, el día de
-- la subida, productos que hoy se venden sin problema, contra existencias que no se han conciliado
-- desde FUDO. Va aquí y no en un SQL aparte porque tiene que ocurrir en la misma subida que empieza a
-- aplicar la columna. El trigger de updated_at se apaga para que el catálogo no diga «editado hoy» de
-- productos que nadie tocó (como en la 0076).
alter table products disable trigger trg_products_updated;
update products set allow_oversell = true where not allow_oversell;
alter table products enable trigger trg_products_updated;

-- +goose Down
-- Pierde los precios web capturados (es el costo de bajar, no hay a dónde guardarlos). No borra los
-- lugares de venta que sembró —pueden tener decisiones colgando— ni restaura los nombres viejos o los
-- `allow_oversell = false` de FUDO, que no se guardaron.
set local lock_timeout = '3s';
alter table product_channels drop column price;
alter table product_channels drop constraint product_channels_channel_fkey;
alter table channels drop constraint channels_company_id_key;
alter table category_channels drop constraint category_channels_channel_id_fkey;
alter table product_channels alter column channel_id type smallint;
alter table category_channels alter column channel_id type smallint;
alter sequence channels_id_seq as smallint;
alter table channels alter column id type smallint;
alter table product_channels add constraint product_channels_channel_id_fkey
  foreign key (channel_id) references channels (id);
alter table category_channels add constraint category_channels_channel_id_fkey
  foreign key (channel_id) references channels (id);
