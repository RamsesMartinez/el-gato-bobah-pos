-- ============================================================================
-- Migración #17 — Subcategorías por marca en "Ramen & Asiática" (id 23) + limpieza de títulos.
-- Reversible: snapshots _bak_categories_20260717 y _bak_products_20260717 (id, name, category_id).
-- Crea 6 subcategorías (Buldak, Nongshim, Ottogi, Paldo, Samyang, Otros), reasigna los 25
-- productos a la marca correcta y estandariza los nombres a "Sabor - Marca" (quita el "Ramen "
-- redundante, corrige errores: Bulkdak→Buldak, Compagguetti→Chapagetti [Nongshim], Nomgshim→Nongshim).
-- Marcas verificadas (incl. búsqueda web para los ambiguos): Buldak Taco = ramen Samyang/Buldak;
-- Chapagetti = Nongshim; MAMA OK Carbonara = MAMA (Tailandia); Shin Ramyun Toomba = Nongshim;
-- Ottogi Odongtong Myon = Ottogi (mariscos picante). Ningún producto se borra ni inactiva.
-- ============================================================================
begin;
create table if not exists _bak_categories_20260717 as select * from categories;
create table if not exists _bak_products_20260717   as select id, name, category_id from products;

-- 1) subcategorías bajo el root 23 (orden: Buldak primero por volumen)
insert into categories (name, parent_id, sort_key) values
  ('Buldak',   23, 100),
  ('Nongshim', 23, 200),
  ('Ottogi',   23, 300),
  ('Paldo',    23, 400),
  ('Samyang',  23, 500),
  ('Otros',    23, 900);

-- 2) reasignar categoría + renombrar, por marca
-- Buldak (línea estrella de Samyang; 11 sabores)
update products p set category_id = g.id, name = v.newname
from (values
  (253,'Taco - Buldak'), (256,'Hot Chicken 3X - Buldak'), (264,'Swicy - Buldak'),
  (265,'Carbonara - Buldak'), (271,'Cream Carbonara - Buldak'), (275,'Habanero Limón - Buldak'),
  (278,'Hot Chicken 2X - Buldak'), (281,'Hot Chicken - Buldak'), (283,'Hot Chicken Cheese - Buldak'),
  (296,'Quattro Cheese - Buldak'), (300,'Rosé - Buldak')
) as v(id, newname), (select id from categories where parent_id=23 and name='Buldak') g
where p.id = v.id;

-- Nongshim (Chapagetti jjajang, Soon Veggie vegano, Shin Ramyun, Shin Ramyun Toomba)
update products p set category_id = g.id, name = v.newname
from (values
  (254,'Chapagetti - Nongshim'), (260,'Soon Veggie - Nongshim'),
  (305,'Shin Ramyun - Nongshim'), (315,'Shin Ramyun Toomba - Nongshim')
) as v(id, newname), (select id from categories where parent_id=23 and name='Nongshim') g
where p.id = v.id;

-- Ottogi
update products p set category_id = g.id, name = v.newname
from (values
  (261,'Odongtong Myon - Ottogi'), (269,'Cheese - Ottogi'), (290,'Kimchi - Ottogi')
) as v(id, newname), (select id from categories where parent_id=23 and name='Ottogi') g
where p.id = v.id;

-- Paldo
update products p set category_id = g.id, name = v.newname
from (values
  (262,'Kokomen - Paldo'), (266,'Carbonara Volcano - Paldo'), (291,'Kimchi - Paldo')
) as v(id, newname), (select id from categories where parent_id=23 and name='Paldo') g
where p.id = v.id;

-- Samyang (los no-Buldak)
update products p set category_id = g.id, name = v.newname
from (values
  (292,'Kimchi - Samyang'), (301,'Original - Samyang')
) as v(id, newname), (select id from categories where parent_id=23 and name='Samyang') g
where p.id = v.id;

-- Otros: no coreano de marca — MAMA (Tailandia) + Udon (japonés)
update products p set category_id = g.id, name = v.newname
from (values
  (258,'Carbonara Bacon - MAMA'), (322,'Udon')
) as v(id, newname), (select id from categories where parent_id=23 and name='Otros') g
where p.id = v.id;

commit;

\echo '--- subcategorías de Ramen & Asiática (id, nombre, orden, # productos activos) ---'
select c.id, c.name, c.sort_key, count(p.id) filter (where p.is_active) as prod
from categories c left join products p on p.category_id=c.id
where c.parent_id=23 and c.is_active group by c.id, c.name, c.sort_key order by c.sort_key;
\echo '--- integridad: productos activos que quedaron en el root 23 sin subcategoría (debe ser 0) ---'
select count(*) from products where category_id=23 and is_active;
\echo '--- total productos bajo Ramen & Asiática (debe seguir 25) ---'
select count(*) from products p join categories c on c.id=p.category_id
where (c.id=23 or c.parent_id=23) and p.is_active;
