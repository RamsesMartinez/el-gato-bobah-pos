begin;
create table if not exists _bak_bigcat_pre10 as select id,category_id from products where is_active and category_id in (4,37);
insert into categories (name,parent_id,sort_key,is_active) values ('Especiales & KPOP',4,1000,true) on conflict do nothing;
insert into categories (name,parent_id,sort_key,is_active) values ('Salados',37,1000,true) on conflict do nothing;
insert into categories (name,parent_id,sort_key,is_active) values ('Embotelladas',4,1000,true) on conflict do nothing;
insert into categories (name,parent_id,sort_key,is_active) values ('Chamoyadas & Sodas',4,1000,true) on conflict do nothing;
insert into categories (name,parent_id,sort_key,is_active) values ('Frappés & Licuados',4,1000,true) on conflict do nothing;
insert into categories (name,parent_id,sort_key,is_active) values ('Dulces & Mochi',37,1000,true) on conflict do nothing;
update products set category_id=(select id from categories where name='Especiales & KPOP' and parent_id=4) where id in (104,98,99,100,101,103,113,179,115);
update products set category_id=(select id from categories where name='Salados' and parent_id=37) where id in (388,387,454,455,458,460,473,474,487,488,489,495,496,437,448);
update products set category_id=(select id from categories where name='Embotelladas' and parent_id=4) where id in (22,26,27,28,29,30,31,32,33,34,35,36,37,38,39,59,67,68,69,70,71,76,77,78,79,81,82,83,85,86,87,88,89,90,91,93,94,95,96,97,102,202,200,204);
update products set category_id=(select id from categories where name='Chamoyadas & Sodas' and parent_id=4) where id in (84,123,163,168,173,177,210,208,211);
update products set category_id=(select id from categories where name='Frappés & Licuados' and parent_id=4) where id in (158,128,16,117,125,162,167,169,181,182,185);
update products set category_id=(select id from categories where name='Dulces & Mochi' and parent_id=37) where id in (453,462,463,464,465,467,468,469,470,471,472,475,499,500,501,438,440,441,442,466,443,445,446,447,449,450);
commit;
\echo '--- sub-categorías creadas + conteo ---'
select pc.name parent, c.name sub, count(p.id) filter (where p.is_active) act from categories c join categories pc on pc.id=c.parent_id left join products p on p.category_id=c.id where c.parent_id in (4,37) and c.is_active group by pc.name,c.name order by pc.name,act desc;
\echo '--- productos aun directamente en la raíz 4/37 (debe ser 0) ---'
select category_id,count(*) from products where is_active and category_id in (4,37) group by category_id;
