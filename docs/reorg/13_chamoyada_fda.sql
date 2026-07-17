-- Migración #13 — Chamoyada y Frappé de Agua como categorías propias (1 producto + Sabor + Tamaño).
begin;
create table if not exists _bak_p_pre13 as select id,name,price,category_id,is_active from products
  where id in (123,161,163,168,173,177,84,211,210,162,167,169);
create table if not exists _bak_c_pre13 as select id,name,is_active from categories where id in (10,41);

update categories set is_active=true where id=10;      -- reactivar "Chamoyadas"
update categories set name='Sodas' where id=41;
insert into categories (name,parent_id,sort_key,is_active) values ('Frappé de Agua',4,1000,true) returning id as fda_cat \gset

-- ===== CHAMOYADA: canónico = 161 (plantilla existente "Chamoyada") =====
update products set price=57, category_id=10, is_active=true where id=161;
insert into modifier_groups (name) values ('Sabor chamoyada') returning id as sc \gset
insert into modifier_options (group_id,name,price_delta,sort_key) values
  (:sc,'Fresa Sandía',0,1),(:sc,'Limón',0,2),(:sc,'Mango',0,3),(:sc,'Mora Azul',0,4),(:sc,'Sandía',0,5),(:sc,'Pelonrico',12,6);
insert into modifier_groups (name) values ('Tamaño chamoyada') returning id as tc \gset
insert into modifier_options (group_id,name,price_delta,sort_key) values (:tc,'14 oz',0,1),(:tc,'16 oz',17,2);
insert into product_modifier_groups (product_id,group_id,title,min_select,max_select,position) values
  (161,:sc,'Sabor',1,1,0),(161,:tc,'Tamaño',1,1,1);
update products set is_active=false where id in (123,163,168,173,177);   -- flavors sueltos

-- ===== FRAPPÉ DE AGUA: canónico = 167 =====
update products set name='Frappé de Agua', price=50, category_id=:fda_cat where id=167;
insert into modifier_groups (name) values ('Sabor frappé de agua') returning id as sf \gset
insert into modifier_options (group_id,name,price_delta,sort_key) values
  (:sf,'Ice Cereza',0,1),(:sf,'Dragón Fruit',5,2),(:sf,'Frambuesa',0,3),(:sf,'Fresa',0,4),(:sf,'Kiwi',0,5),
  (:sf,'Limón',0,6),(:sf,'Maracuyá',0,7),(:sf,'Ice Mora Azul',0,8),(:sf,'Mango',0,9),(:sf,'Pink Lemonade',0,10);
insert into modifier_groups (name) values ('Tamaño frappé de agua') returning id as tf \gset
insert into modifier_options (group_id,name,price_delta,sort_key) values (:tf,'14 oz',0,1),(:tf,'16 oz',10,2);
insert into product_modifier_groups (product_id,group_id,title,min_select,max_select,position) values
  (167,:sf,'Sabor',1,1,0),(167,:tf,'Tamaño',1,1,1);
update products set is_active=false where id in (169,162);
commit;
\echo '--- sub-categorías de Bebidas Frías ---'
select id,name from categories where parent_id=4 and is_active order by name;
\echo '--- configurables (Chamoyada 161, Frappé de Agua 167) ---'
select p.id,p.name,p.price,c.name cat,(select count(*) from product_modifier_groups g where g.product_id=p.id) grupos
from products p join categories c on c.id=p.category_id where p.id in (161,167);
