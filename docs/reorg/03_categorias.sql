-- ============================================================================
-- Migración #03 — consolidar 37 categorías → 8 limpias (+ "Otro (revisar)").
-- Reversible: snapshots _bak_categories_20260706 y _bak_products_cat_20260706.
-- Reasigna productos de subcategorías al root correcto y desactiva las viejas.
-- Ningún producto se borra ni se inactiva aquí (solo cambia category_id).
-- "Otro (revisar)" retiene los ~31 productos-opción pendientes de convertir/depurar.
-- ============================================================================
begin;
create table if not exists _bak_categories_20260706   as select * from categories;
create table if not exists _bak_products_cat_20260706  as select id, category_id from products;

-- renombrar los roots que conservamos (nombres cortos, estables)
update categories set name = 'Bebidas Calientes' where id = 1;
update categories set name = 'Bebidas Frías'     where id = 4;
update categories set name = 'Boneless & Alitas' where id = 20;
update categories set name = 'Snacks'            where id = 37;
update categories set name = 'Ramen & Asiática'  where id = 23;
update categories set name = 'Otro (revisar)'    where id = 30;
-- 22 Combos, 26 Crepas, 29 Desayunos: mantienen nombre

-- reasignar productos de subcategorías → root nuevo
update products set category_id = 1  where category_id in (2,3);
update products set category_id = 4  where category_id in (5,6,7,8,9,10,11,12,13,14,15,16,17,18,19);
update products set category_id = 20 where category_id in (21);
update products set category_id = 37 where category_id in (36);
update products set category_id = 26 where category_id in (27,28);
update products set category_id = 23 where category_id in (24,25);
update products set category_id = 30 where category_id in (31,32,33,34,35);

-- desactivar las categorías viejas (subcategorías + roots fusionados)
update categories set is_active = false
 where id in (2,3,5,6,7,8,9,10,11,12,13,14,15,16,17,18,19,21,24,25,27,28,31,32,33,34,35,36);
commit;

\echo '--- categorías activas (debe ser 9) con # productos activos ---'
select c.id, c.name, count(p.id) filter (where p.is_active) as act
from categories c left join products p on p.category_id=c.id
where c.is_active group by c.id, c.name order by act desc;
\echo '--- integridad: productos activos apuntando a categoría inactiva (debe ser 0) ---'
select count(*) from products p join categories c on c.id=p.category_id where p.is_active and not c.is_active;
\echo '--- total productos activos (debe seguir 235) ---'
select count(*) from products where is_active;
