-- Migración #11 — alinear precios de bebidas al menú 2026 + $0 resueltos + Durazno descontinuado.
begin;
create table if not exists _bak_products_pre11 as select id,price,is_active from products
  where id in (104,123,128,162,163,167,168,169,173,177,208,445,446);
-- Fix 1: Frappé de Agua $0 -> $50
update products set price=50 where id in (167,169);
-- Fix 2: realinear al menú
update products set price=65 where id=128;   -- Frappé
update products set price=70 where id=104;   -- Bobah Tea
update products set price=57 where id in (163,168,173,177);  -- Chamoyadas
update products set price=69 where id=123;   -- Chamoyada Pelonrico
update products set price=50 where id=162;   -- Frappé de Agua Granizado
-- Mini Donas: $35 (8pz) / $50 (14pz)
update products set price=35 where id=445;    -- Cajeta Mini Donas 8pz
update products set price=50 where id=446;    -- Chocolate Mini Donas 14pz
-- Durazno soda: descontinuado
update products set is_active=false where id=208;
commit;
\echo '--- resultado ---'
select id,name,price,is_active from products where id in (104,123,128,162,163,167,168,169,173,177,208,445,446) order by name;
\echo '--- productos activos con centavos o $0 (debe ser 0) ---'
select count(*) from products where is_active and (price<>round(price) or price=0);
