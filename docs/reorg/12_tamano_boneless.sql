-- Migración #12 — Boneless y Alitas: tamaño como MODIFICADOR (no en el nombre).
-- 1 producto base (250g) + grupo "Tamaño" con delta de precio. Inactiva las otras tallas.
-- ceiling: el perk "1kg = 3 salsas" se pierde al fusionar (queda "elige hasta 2" para todas);
--   el 3er sabor sigue disponible vía "Salsa extra +$15". Confirmar si se quiere max 3 universal.
begin;
create table if not exists _bak_products_pre12 as select id,name,price,is_active from products where id in (212,213,214,216,217,218,219,220);

-- BONELESS: canónico = 217 (250g, $150)
update products set name='Boneless', price=150 where id=217;
insert into modifier_groups (name) values ('Tamaño boneless') returning id as bg \gset
insert into modifier_options (group_id,name,price_delta,sort_key) values
  (:bg,'250 g',0,1),(:bg,'370 g',65,2),(:bg,'500 g',130,3),(:bg,'1 kg',350,4);
insert into product_modifier_groups (product_id,group_id,title,min_select,max_select,position)
  values (217,:bg,'Tamaño',1,1,0);
update products set is_active=false where id in (218,219,220);

-- ALITAS: canónico = 212 (250g, $130)
update products set name='Alitas', price=130 where id=212;
insert into modifier_groups (name) values ('Tamaño alitas') returning id as ag \gset
insert into modifier_options (group_id,name,price_delta,sort_key) values
  (:ag,'250 g',0,1),(:ag,'370 g',60,2),(:ag,'500 g',130,3),(:ag,'1 kg',329,4);
insert into product_modifier_groups (product_id,group_id,title,min_select,max_select,position)
  values (212,:ag,'Tamaño',1,1,0);
update products set is_active=false where id in (213,214,216);

commit;
\echo '--- Boneless/Alitas activos (deben ser 2, sin tamaño en el nombre) ---'
select id,name,price from products where is_active and (name='Boneless' or name='Alitas');
\echo '--- grupo Tamaño de Boneless ---'
select o.name,o.price_delta from modifier_options o join modifier_groups g on g.id=o.group_id where g.name='Tamaño boneless' order by o.sort_key;
