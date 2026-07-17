-- ============================================================================
-- Migración #14 — Crepas 2026.
-- Dos caminos: (a) Crepas Especiales = productos fijos con relleno precargado;
--              (b) "Arma la Tuya" = 1 producto configurable con modificadores.
-- Reglas 2026 (PDF pág. 17): base 1 ingrediente $43, ingrediente extra +$14,
--   agrega fruta +$20, decora sin costo. Especiales todas $95 salvo Kit Kat $85.
-- - Reutiliza el producto 337 "¡arma la Tuya!" como base configurable ($43).
-- - Reprecios + descripciones a las 9 especiales que siguen en el menú 2026.
-- - Crea "Galleta Oreo Crepa" (faltaba) y desactiva las que salieron del menú
--   (Kinder Bueno, Gansito) y las 8 crepas de 1-topping que absorbe la base.
-- Reversible: snapshot _bak_p_pre14 + borrado de lo creado (14_rollback.sql).
-- ============================================================================
begin;
create table if not exists _bak_p_pre14 as
  select id,name,description,price,is_active,is_favorite,sort_key,category_id
  from products where category_id in (26,27,28);
create table if not exists _bak_pmg_pre14 as
  select * from product_modifier_groups where product_id in (337,334,325);

-- soltar los grupos FUDO heredados de las crepas activas: Toppings +$12 (60),
-- Decoración (11) y Fruta (12). Chocan con los grupos nuevos de "Arma la Tuya" y
-- las especiales van precargadas. Los grupos se conservan (histórico de órdenes);
-- solo se quitan los enlaces a los productos activos.
delete from product_modifier_groups where group_id in (11,12,60) and product_id in (337,334,325);

-- ===== (a) CREPAS ESPECIALES: reprecio 2026 + descripción + orden =====
update products set price=95, sort_key=20,
  description='Crepa rellena de Nutella, Philadelphia, Ferrero y fresas, decorada con almendras y un toque de chocolate Hershey.'
  where id=331;  -- Ferrero
update products set price=85, sort_key=30,
  description='Crepa rellena de Nutella, Philadelphia, Kit Kat y frutos rojos, decorada con chispas de chocolate y un toque de chocolate Hershey.'
  where id=335;  -- Kit Kat
update products set price=95, sort_key=40,
  description='Crepa rellena de Nutella, Philadelphia, Kinder Delice, fresa y durazno, decorada con nuez.'
  where id=334;  -- Kinder Delice
update products set name='Crepa Baileys', price=95, sort_key=50,
  description='Crepa rellena de Philadelphia, pastelito Baileys y fresas, decorada con nuez y un toque de Baileys.'
  where id=325;  -- Creppa Baileys (fix typo)
update products set price=95, sort_key=70,
  description='Crepa rellena de Nutella, Philadelphia, Conejito Turín y duraznos, decorada con chispas de chocolate.'
  where id=326;  -- Conejito Turín
update products set price=95, sort_key=80,
  description='Crepa rellena de Philadelphia, salsa de tomate, hierbas finas, queso Mozzarella y Pepperoni, con un toque de queso parmesano.'
  where id=330;  -- Crepizza Pepperoni
update products set price=95, sort_key=90,
  description='Crepa rellena de Philadelphia, salsa de tomate, hierbas finas, queso Mozzarella y jamón con piña, con un toque de queso parmesano.'
  where id=329;  -- Crepizza Hawaiana
update products set price=95, sort_key=100,
  description='Crepa rellena de Philadelphia, jamón con piña salteados con hierbas finas y queso Mozzarella.'
  where id=328;  -- Crepa Hawaiana
update products set price=95, sort_key=110,
  description='Crepa rellena de Philadelphia, champiñones salteados con hierbas finas y queso Mozzarella.'
  where id=327;  -- Crepa Champiñones

-- especial que faltaba en el catálogo
insert into products (name,description,type,category_id,price,sort_key,is_active)
  values ('Galleta Oreo Crepa',
          'Crepa rellena de queso Philadelphia, galleta Oreo y duraznos, decorada con chispas de chocolate.',
          'simple',26,95,60,true);

-- especiales fuera del menú 2026
update products set is_active=false where id in (333,332);  -- Kinder Bueno, Gansito

-- ===== (b) ARMA LA TUYA: base configurable = producto 337 =====
update products set name='Crepa · Arma la Tuya', price=43, is_active=true, is_favorite=true,
  sort_key=10, category_id=26,
  description='Crepa dulce con el ingrediente que elijas (incluido). Suma ingredientes extra, fruta y decórala sin costo.'
  where id=337;

-- 1) Ingrediente incluido (elige 1)
insert into modifier_groups (name) values ('Ingrediente crepa') returning id as gi \gset
insert into modifier_options (group_id,name,price_delta,sort_key) values
  (:gi,'Nutella',0,1),(:gi,'Philadelphia',0,2),(:gi,'Cajeta',0,3),(:gi,'Lechera',0,4),
  (:gi,'Crema Pastelera',0,5),(:gi,'Miel Maple',0,6),(:gi,'Chocolate Hershey',0,7),
  (:gi,'Mermelada de Fresa',0,8),(:gi,'Mermelada de Cereza',0,9),
  (:gi,'Mermelada de Frutos Rojos',0,10),(:gi,'Mermelada de Zarzamora',0,11);

-- 2) Ingredientes extra (+$14 c/u)
insert into modifier_groups (name) values ('Ingrediente extra crepa') returning id as gx \gset
insert into modifier_options (group_id,name,price_delta,sort_key) values
  (:gx,'Nutella',14,1),(:gx,'Philadelphia',14,2),(:gx,'Cajeta',14,3),(:gx,'Lechera',14,4),
  (:gx,'Crema Pastelera',14,5),(:gx,'Miel Maple',14,6),(:gx,'Chocolate Hershey',14,7),
  (:gx,'Mermelada de Fresa',14,8),(:gx,'Mermelada de Cereza',14,9),
  (:gx,'Mermelada de Frutos Rojos',14,10),(:gx,'Mermelada de Zarzamora',14,11);

-- 3) Agrega fruta (+$20 c/u)
insert into modifier_groups (name) values ('Fruta crepa') returning id as gf \gset
insert into modifier_options (group_id,name,price_delta,sort_key) values
  (:gf,'Fresa',20,1),(:gf,'Mango en Almíbar',20,2),(:gf,'Durazno en Almíbar',20,3),(:gf,'Frutos Rojos',20,4);

-- 4) Decora tu crepa (sin costo)
insert into modifier_groups (name) values ('Decora tu crepa') returning id as gd \gset
insert into modifier_options (group_id,name,price_delta,sort_key) values
  (:gd,'Lunetas',0,1),(:gd,'Nuez',0,2),(:gd,'Galleta Oreo',0,3),
  (:gd,'Chispas de Chocolate',0,4),(:gd,'Granillo de Chocolate',0,5);

insert into product_modifier_groups (product_id,group_id,title,min_select,max_select,position) values
  (337,:gi,'Ingrediente incluido',1,1,0),
  (337,:gx,'Ingredientes extra (+$14)',0,6,1),
  (337,:gf,'Agrega fruta (+$20)',0,4,2),
  (337,:gd,'Decora tu crepa (gratis)',0,5,3);

-- crepas de 1 topping que ahora cubre la base "Arma la Tuya"
update products set is_active=false where id in (345,338,354,355,360,341,340,356);

commit;

\echo '--- Crepas activas (especiales $95/$85 + base $43) ---'
select id,name,price,is_favorite,sort_key from products
  where category_id=26 and is_active order by sort_key;
\echo '--- Grupos de la base configurable (337) ---'
select pmg.position,coalesce(pmg.title,mg.name) titulo,pmg.min_select,pmg.max_select,
       (select count(*) from modifier_options o where o.group_id=mg.id) opciones
from product_modifier_groups pmg join modifier_groups mg on mg.id=pmg.group_id
where pmg.product_id=337 order by pmg.position;
