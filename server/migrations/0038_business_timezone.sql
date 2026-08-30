-- +goose Up
-- Zona horaria del local. La base sigue guardando TODO en UTC (`timestamptz`), que es lo correcto:
-- un instante es un instante. Lo que no puede salir en UTC es la FECHA DE NEGOCIO, que es una
-- decisión de calendario y depende de dónde está el local.
--
-- Sin esto, el servidor calculaba el día en UTC y la medianoche caía a las 18:00 en México: todo lo
-- vendido de las 6pm en adelante quedaba contado en el día siguiente —justo la franja donde más
-- vende un lugar de comida— y el folio diario se reiniciaba a media cena. Se vio el 29 de agosto de
-- 2026 con dos tickets #1 en la misma noche, y afectaba también cortes, gastos y reportes.
--
-- Va en business_settings y no en companies porque es configuración del LOCAL, editable desde su
-- pantalla de ajustes, no identidad del tenant.
alter table business_settings
  add column timezone text not null default 'America/Mexico_City'
  -- Nombre IANA no vacío. Que exista de verdad se valida en la frontera al guardarlo
  -- (domain.ValidTimezone): Postgres no conoce la lista y un check contra pg_timezone_names
  -- ataría el esquema a la versión de tzdata del servidor.
  constraint business_settings_timezone_no_vacio check (length(trim(timezone)) > 0);

-- +goose Down
alter table business_settings drop column timezone;
