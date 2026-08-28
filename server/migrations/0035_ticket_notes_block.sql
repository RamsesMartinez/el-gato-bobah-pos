-- +goose Up
-- Los textos del ticket dejan de ser un renglón y pasan a ser un BLOQUE: ahí va el aviso de que el
-- ticket no es comprobante fiscal, con los datos para pedir factura. 400 caracteres son ~13
-- renglones de 32, es decir unos 5 cm de papel por venta — más que eso deja de ser un aviso.
alter table business_settings drop constraint business_settings_header_len;
alter table business_settings drop constraint business_settings_footer_len;
alter table business_settings
  add constraint business_settings_header_len check (header_note is null or char_length(header_note) <= 400),
  add constraint business_settings_footer_len check (footer_note is null or char_length(footer_note) <= 400);

-- Pie por default. Se siembra SOLO donde no hay nada configurado: un negocio que ya escribió el
-- suyo no debe encontrárselo reemplazado después de un deploy.
--
-- Los separadores miden 32 caracteres porque ése es el ancho real del papel de 80mm con la fuente
-- del ticket. Uno más largo no se ve más largo: se parte en dos renglones y rompe el recuadro.
update business_settings
set footer_note = $fiscal$================================
    TICKET SIN VALOR FISCAL
================================
Si necesita factura, solicítela al
correo facturacion@elgatobobah.com
Envíe la foto de su ticket, sus datos
fiscales y su constancia de situación
fiscal actualizada.
--------------------------------
     GRACIAS POR SU CONSUMO$fiscal$
where footer_note is null or btrim(footer_note) = '';

-- +goose Down
-- Los checks vuelven a 120, así que primero hay que vaciar lo que ya no cabría: dejar el texto
-- rompería el alter y la migración quedaría a medias.
update business_settings set footer_note = null where char_length(footer_note) > 120;
update business_settings set header_note = null where char_length(header_note) > 120;

alter table business_settings drop constraint business_settings_header_len;
alter table business_settings drop constraint business_settings_footer_len;
alter table business_settings
  add constraint business_settings_header_len check (header_note is null or char_length(header_note) <= 120),
  add constraint business_settings_footer_len check (footer_note is null or char_length(footer_note) <= 120);
