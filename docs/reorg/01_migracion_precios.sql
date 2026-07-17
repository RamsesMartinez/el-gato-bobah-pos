-- ============================================================================
-- Migración de datos #01 — limpieza de precios y descontinuados (El Gato Bobah)
-- Reversible: crea snapshots _bak_*_20260706 y hay rollback en 01_rollback.sql
-- Alcance: precios boneless/alitas del menú 2026, opciones $0.01→$0, inactivar
--          líneas descontinuadas (Mojitos, Chupa Chups, Smoothies).
-- NO toca: chilaquiles (no están en el PDF de bebidas), papas (confirmar aparte),
--          nombres/categorías/duplicados (etapa revisada aparte).
-- ============================================================================
begin;

-- 1) SNAPSHOT (para rollback) — solo una vez
create table if not exists _bak_products_20260706   as select * from products;
create table if not exists _bak_modopts_20260706     as select * from modifier_options;

-- 2) PRECIOS boneless/alitas = menú 2026 (fuente de verdad). Mapeo por tamaño.
update products set price = 150 where id = 217; -- BONELESS CH 250g  (129.98 → 150)
update products set price = 215 where id = 220; -- BONELESS M 370g   (194.98 → 215)
update products set price = 280 where id = 218; -- BONELESS G 500g   (244.98 → 280)
update products set price = 500 where id = 219; -- BONELESS J 1 Kg   (434.98 → 500)
update products set price = 130 where id = 212; -- ALITAS CH 250g    (114.98 → 130)
update products set price = 190 where id = 216; -- ALITAS M 370g     (164.98 → 190)
update products set price = 260 where id = 213; -- ALITAS G 500g     (234.98 → 260)
update products set price = 459 where id = 214; -- ALITAS J 1 Kg     (398.98 → 459)

-- 3) Opciones de modificador con el truco de ticket $0.01 → $0 (ya no se necesita)
update modifier_options set price_delta = 0 where price_delta = 0.01;

-- 4) Inactivar líneas descontinuadas (no en el menú 2026). Reversible desde admin.
update products set is_active = false
where id in (195,196,197,198,   -- Mojitos: Amore Mio, Blue Butterfly, Dalgi, Mango Go
             41,42,44,45,        -- Chupa Chups (activos): Fresa, Mango, Naranja, Raspberry
             178,187,188)        -- Smoothies (activos): Frutos Rojos, Fresa, Mango
  and is_active;

commit;

-- Verificación rápida
\echo '--- boneless/alitas tras migración ---'
select id,name,price from products where id in (212,213,214,216,217,218,219,220) order by name;
\echo '--- opciones aun en 0.01 (debe ser 0) ---'
select count(*) from modifier_options where price_delta = 0.01;
\echo '--- descontinuados activos (debe ser 0) ---'
select count(*) from products where id in (195,196,197,198,41,42,44,45,178,187,188) and is_active;
