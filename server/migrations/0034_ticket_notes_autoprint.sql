-- +goose Up
-- Texto libre arriba del detalle del pedido, y el interruptor de impresión automática al cerrar
-- una venta. Van aquí y no dentro de la 0033 porque esa ya corrió: editar una migración aplicada
-- deja esquemas distintos entre máquinas según quién migró antes.
alter table business_settings
  add column header_note         text,
  add column auto_print_on_close boolean not null default false;

alter table business_settings
  add constraint business_settings_header_len check (header_note is null or char_length(header_note) <= 120);

-- La lista blanca de la 0033 incluía image/webp, pero la validación de la app solo acepta PNG y
-- JPEG: son los que la stdlib sabe decodificar, y aceptar un formato cuyas dimensiones no podemos
-- leer sería aceptar un archivo que no podemos acotar. Se alinea el check con esa regla para que
-- ninguna ruta de escritura futura pueda meter un webp que nadie valida.
alter table business_settings drop constraint business_settings_logo_mime;
alter table business_settings
  add constraint business_settings_logo_mime check (logo_mime is null or logo_mime in ('image/png', 'image/jpeg'));

-- +goose Down
alter table business_settings drop constraint business_settings_logo_mime;
alter table business_settings
  add constraint business_settings_logo_mime check (logo_mime is null or logo_mime in ('image/png', 'image/jpeg', 'image/webp'));

alter table business_settings
  drop column auto_print_on_close,
  drop column header_note;
