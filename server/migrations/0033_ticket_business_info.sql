-- +goose Up
-- Identidad del negocio y logo, para que el ticket deje de estar escrito en el código del front.
-- Van en business_settings (una fila por empresa desde 0023) y NO en una tabla nueva: esta tabla
-- ya está dada de alta en la policy tenant_isolation de 0024, y una tabla per-tenant sin policy es
-- una fuga de datos entre empresas esperando a pasar.
--
-- El logo va como bytea aquí y no en disco: el VPS es un e2-micro sin volumen persistente, así que
-- un archivo escrito en el contenedor se pierde en el siguiente deploy. Postgres lo TOASTea fuera
-- de la fila, y las queries nombran columnas, así que GetBusinessSettings no lo arrastra.
alter table business_settings
  add column business_name   text,
  add column address         text,
  add column phone           text,
  add column footer_note     text,
  add column logo_bytes      bytea,
  add column logo_mime       text,
  add column logo_updated_at timestamptz;

-- Se siembra desde el nombre del tenant: el front ya imprimía "El Gato Bobah" hardcodeado, así que
-- arrancar con ese valor no cambia lo que el cliente ve en el papel.
update business_settings bs
set business_name = c.name
from companies c
where c.id = bs.company_id;

-- Hasta ahora: agregar la columna ya not null y sin default reventaría con las filas existentes.
alter table business_settings alter column business_name set not null;

alter table business_settings
  -- Los largos se acotan en la base, no solo en Go. El ticket mide 32 caracteres de ancho: un
  -- nombre de 5 KB no "se ve feo", rompe el layout de todos los tickets que salgan después.
  add constraint business_settings_name_len   check (char_length(business_name) between 1 and 60),
  add constraint business_settings_addr_len   check (address     is null or char_length(address)     <= 120),
  add constraint business_settings_phone_len  check (phone       is null or char_length(phone)       <= 30),
  add constraint business_settings_footer_len check (footer_note is null or char_length(footer_note) <= 120),
  -- Bytes y mime van juntos o no van ninguno. Una fila con bytes y sin mime obliga al navegador a
  -- adivinar el tipo del binario, que es exactamente lo que la validación de subida cierra.
  add constraint business_settings_logo_pair  check ((logo_bytes is null) = (logo_mime is null)),
  -- 256 KB. El tope vive también aquí para que ninguna ruta de escritura futura lo salte.
  add constraint business_settings_logo_size  check (logo_bytes is null or octet_length(logo_bytes) <= 262144),
  add constraint business_settings_logo_mime  check (logo_mime is null or logo_mime in ('image/png', 'image/jpeg', 'image/webp'));

-- +goose Down
-- Los checks se van solos con sus columnas; basta con soltar las columnas en orden inverso.
alter table business_settings
  drop column logo_updated_at,
  drop column logo_mime,
  drop column logo_bytes,
  drop column footer_note,
  drop column phone,
  drop column address,
  drop column business_name;
