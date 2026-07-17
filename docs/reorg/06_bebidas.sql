-- ============================================================================
-- Migración #06 — limpieza de nombres de bebidas (Frappé / Bobah Tea).
-- El colapso tipo×sabor ya existía (FUDO); esto limpia los nombres de las
-- opciones (quita "FRAPPÉ"/"BOBA TEA'S" redundante, typos, duplicados), pone
-- títulos a los grupos y renombra los productos configurables.
-- Reversible: 06_rollback.sql (snapshots _bak_modopts/_bak_pmg/_bak_products).
-- ============================================================================
begin;
update modifier_options set name='Con crema batida' where id=8;
update modifier_options set name='Sin crema batida' where id=9;
update modifier_options set name='Sin perlas' where id=76;
update modifier_options set name='Cereza' where id=77;
update modifier_options set name='Chicle' where id=78;
update modifier_options set name='Chocolate' where id=79;
update modifier_options set name='Dragon Fruit' where id=80;
update modifier_options set name='Fresa' where id=81;
update modifier_options set name='Kiwi' where id=82;
update modifier_options set name='Litchi' where id=83;
update modifier_options set name='Mango' where id=84;
update modifier_options set name='Manzana Verde' where id=85;
update modifier_options set name='Maracuya' where id=86;
update modifier_options set name='Mora Azul' where id=87;
update modifier_options set name='Tapioca' where id=88;
update modifier_options set name='Yogurth' where id=89;
update modifier_options set name='Algodón de Azúcar' where id=145;
update modifier_options set name='Chai Clásico' where id=146;
update modifier_options set name='Chocolate' where id=147;
update modifier_options set name='Frutos Rojos' where id=148;
update modifier_options set name='Horchata' where id=149;
update modifier_options set name='Matcha' where id=150;
update modifier_options set name='Matcha Coco' where id=151;
update modifier_options set name='Pink Lemonade' where id=152;
update modifier_options set name='Taro' where id=153;
update modifier_options set name='Taro Purple' where id=154;
update modifier_options set name='Vainilla' where id=155;
update modifier_options set is_active=false where id=156; -- dup de 'Matcha Coco' (id 151)
update modifier_options set name='Algodón de Azúcar' where id=170;
update modifier_options set name='Baileys' where id=171;
update modifier_options set name='Cajeta' where id=172;
update modifier_options set name='Calabaza' where id=173;
update modifier_options set name='Cappuccino' where id=174;
update modifier_options set name='Cereza' where id=175;
update modifier_options set name='Chai Manzana Canela' where id=176;
update modifier_options set name='Chai Vainilla' where id=177;
update modifier_options set name='Cheesecake' where id=178;
update modifier_options set name='Choco Avellana' where id=179;
update modifier_options set name='Chocolate Blanco' where id=180;
update modifier_options set name='Chocolate' where id=181;
update modifier_options set name='Chocoretas' where id=182;
update modifier_options set name='Conejito Turín' where id=183;
update modifier_options set name='Cookies & Cream (oreo)' where id=184;
update modifier_options set name='Dragon Fruit' where id=185;
update modifier_options set name='Ferrero' where id=186;
update modifier_options set name='Magnum' where id=187;
update modifier_options set is_active=false where id=188; -- dup de 'Chai Vainilla' (id 177)
update modifier_options set name='Frutos Rojos' where id=189;
update modifier_options set name='Gansito' where id=190;
update modifier_options set name='Horchata' where id=191;
update modifier_options set name='Java Chips' where id=192;
update modifier_options set name='Kitkat' where id=193;
update modifier_options set is_active=false where id=194; -- dup de 'Magnum' (id 187)
update modifier_options set name='Marshmallow Chocolate' where id=195;
update modifier_options set name='Matcha' where id=196;
update modifier_options set name='Mazapán' where id=197;
update modifier_options set name='Matcha Coco' where id=198;
update modifier_options set name='Mokaccino' where id=199;
update modifier_options set name='Pan de Muerto' where id=200;
update modifier_options set name='Pay de Limón' where id=201;
update modifier_options set name='Pistache' where id=202;
update modifier_options set name='Piña Colada' where id=203;
update modifier_options set name='Red Velvet Chocolate' where id=204;
update modifier_options set name='Rompope' where id=205;
update modifier_options set name='Strawberry Cream (fresa)' where id=206;
update modifier_options set name='Taro' where id=207;
update modifier_options set name='Taro Purple' where id=208;
update modifier_options set name='Vainilla' where id=209;
update modifier_options set name='Zarza Cremosa' where id=210;
update modifier_options set name='14 oz' where id=447;
update modifier_options set name='16 oz' where id=448;
-- títulos de grupo
update product_modifier_groups set title='Sabor' where group_id=27 and (title is null or title='');
update product_modifier_groups set title='Perlas explosivas' where group_id=17 and (title is null or title='');
update product_modifier_groups set title='Crema batida' where group_id=3 and (title is null or title='');
update product_modifier_groups set title='Tamaño' where group_id=52 and (title is null or title='');
-- nombres de producto configurables
update products set name='Frappé' where id=128;
update products set name='Bobah Tea' where id=104;
commit;
\echo '--- Frappé (128): grupos con título y # opciones activas ---'
select coalesce(pmg.title,'(sin)') titulo, count(o.id) filter (where o.is_active) ops
from product_modifier_groups pmg left join modifier_options o on o.group_id=pmg.group_id
where pmg.product_id=128 group by pmg.title order by ops desc;
\echo '--- muestra sabores limpios ---'
select name from modifier_options where group_id=27 and is_active order by name limit 12;
