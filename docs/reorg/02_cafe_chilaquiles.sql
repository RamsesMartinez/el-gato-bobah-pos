-- ============================================================================
-- Migración #02 — chilaquiles sin centavos + café con modificador Temperatura.
-- Reversible: usa el snapshot _bak_products_20260706 (chilaquiles) y 02_rollback.sql
-- (borra el grupo Temperatura). NO toca papas (pendiente de confirmar precios).
-- ============================================================================
begin;

-- 1) Chilaquiles: regla "sin centavos" (no están en el PDF; redondeo al peso)
update products set price = 65 where id in (366,369);  -- CHILAQ GRANDES 64.99 -> 65
update products set price = 55 where id in (365,368);  -- CHILAQ CHICOS  54.99 -> 55

-- 2) Café: modificador "Temperatura" (Caliente por defecto / Frío) en los cafés/chocolates
--    de casa. Son 1 producto c/u (no hay versiones frío/caliente separadas que fusionar).
insert into modifier_groups (name) values ('Temperatura') returning id as gid \gset
insert into modifier_options (group_id, name, price_delta, sort_key) values
  (:gid, 'Caliente', 0, 1),   -- default (primer sort → se autoselecciona en el POS)
  (:gid, 'Frío',     0, 2);
insert into product_modifier_groups (product_id, group_id, title, min_select, max_select, position)
select v.id, :gid, 'Temperatura', 1, 1, 0
from (values (4),(5),(6),(7),(8),(10),(12),(13),(14)) as v(id);

commit;

\echo '--- chilaquiles ---'
select id,name,price from products where id in (365,366,368,369) order by id;
\echo '--- café con Temperatura (debe listar 9 productos) ---'
select p.id, p.name from products p
join product_modifier_groups pmg on pmg.product_id=p.id
join modifier_groups g on g.id=pmg.group_id
where g.name='Temperatura' order by p.name;
