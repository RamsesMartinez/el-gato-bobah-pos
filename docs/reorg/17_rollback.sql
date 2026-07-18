-- Rollback #17 — restaura nombre y category_id previos de los 25 productos y borra las
-- subcategorías creadas. Requiere el snapshot _bak_products_20260717.
begin;
update products p set category_id = b.category_id, name = b.name
from _bak_products_20260717 b where b.id = p.id;
delete from categories where parent_id = 23 and name in ('Buldak','Nongshim','Ottogi','Paldo','Samyang','Otros');
commit;

\echo '--- productos activos de vuelta en el root 23 (debe ser 25) ---'
select count(*) from products where category_id = 23 and is_active;
