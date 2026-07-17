-- ============================================================================
-- Migración #15 — nombres de crepas sin redundancia.
-- Dentro de la categoría "Crepas" el sufijo "Crepa" sobra en las dulces de marca.
-- Las saladas conservan prefijo porque distingue producto (Crepa Hawaiana ≠
-- Crepizza Hawaiana) y así lo lista el menú 2026 pág. 18.
-- Nombres cortos verificados sin colisión (products.name es unique global).
-- Reversible: snapshot _bak_pnames_pre15 (15_rollback.sql).
-- NOTA: el menú se cachea en Redis (pos:menu). Tras aplicar: redis-cli DEL pos:menu.
-- ============================================================================
begin;
create table if not exists _bak_pnames_pre15 as
  select id,name from products where id in (325,326,331,334,335,337,502);

update products set name='Ferrero'        where id=331;
update products set name='Kit Kat'        where id=335;
update products set name='Kinder Delice'  where id=334;
update products set name='Baileys'        where id=325;
update products set name='Galleta Oreo'   where id=502;
update products set name='Conejito Turín' where id=326;
update products set name='Arma tu Crepa'  where id=337;
commit;

\echo '--- Crepas activas (nombres finales) ---'
select id,name,price from products where category_id=26 and is_active order by sort_key;
