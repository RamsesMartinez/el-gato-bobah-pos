-- Nombres alternativos de productos y artículos del almacén (spec 063, issue #221): otros nombres con
-- los que se busca lo mismo. Solo para encontrar; ticket, comanda, reportes y plataformas siguen
-- leyendo `name`.
--
-- Columna en la fila y no tabla aparte: el choque cruza el nombre principal y los alternativos, y
-- ningún índice lo cubre (el nombre de un producto solo es único exacto). Lo decide la API bajo un
-- candado por empresa; la columna hereda RLS, GRANT y el trigger de versión del catálogo (0093).
--
-- Convive con la API anterior: la columna tiene default, y un insert o update que no la nombra la
-- deja en '{}', que cumple el check.

-- +goose Up
-- Una sentencia por tabla (un solo ACCESS EXCLUSIVE) y con tope de espera: sin él, una venta con la
-- fila del producto tomada deja al ALTER esperando, y detrás de él se forman todas las ventas. Si no
-- llega en 3 s, la migración falla, la API nueva no arranca y la vieja sigue atendiendo.
set local lock_timeout = '3s';

-- Un elemento nulo o vacío no se admite: escanear un text[] con un NULL en []string truena, y eso
-- tumbaría el menú de toda la empresa, no un renglón.
alter table products add column alt_names text[] not null default '{}'
  constraint products_alt_names_valid check (cardinality(alt_names) <= 10
    and array_position(alt_names, null) is null and array_position(alt_names, '') is null);
alter table ingredients add column alt_names text[] not null default '{}'
  constraint ingredients_alt_names_valid check (cardinality(alt_names) <= 10
    and array_position(alt_names, null) is null and array_position(alt_names, '') is null);

-- La llave con que dos nombres son el mismo: sin mayúsculas, acentos ni espacios de más. Gemela de
-- domain.CatalogNameKey (prueba de paridad). btrim va DESPUÉS de juntar espacios: antes, un tabulador
-- en la orilla quedaba como espacio sin recortar. Los espacios Unicode que junta strings.Fields (el duro,
-- el estrecho, el ideográfico…) van explícitos y escapados: no se puede asumir que \s los cubra en la
-- locale de la base. Literal E'…': el
-- cuerpo se vuelve a leer en cada llamada con el ajuste de la sesión, y con
-- standard_conforming_strings=off un '\s' simple se lee como «s».
-- +goose StatementBegin
create function catalog_name_key(t text) returns text
  language sql immutable parallel safe
  as $$ select btrim(regexp_replace(translate(lower(t), 'áéíóúüñ', 'aeiouun'), E'[\\s\\u0085\\u00a0\\u1680\\u2000-\\u200a\\u2028\\u2029\\u202f\\u205f\\u3000]+', ' ', 'g')) $$;
-- +goose StatementEnd

-- +goose Down
-- Bajar pierde los alternativos capturados: el Up los vuelve a aceptar, pero vacíos.
set local lock_timeout = '3s';
drop function catalog_name_key(text);
alter table ingredients drop column alt_names;
alter table products drop column alt_names;
