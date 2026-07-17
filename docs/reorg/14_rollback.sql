-- ROLLBACK migración #14.
begin;
-- borrar lo creado por #14 (pmg primero: group_id no tiene cascade; opciones sí)
delete from product_modifier_groups
  where group_id in (select id from modifier_groups
                     where name in ('Ingrediente crepa','Ingrediente extra crepa','Fruta crepa','Decora tu crepa'));
delete from modifier_groups
  where name in ('Ingrediente crepa','Ingrediente extra crepa','Fruta crepa','Decora tu crepa');
delete from products where name='Galleta Oreo Crepa';

-- reponer los enlaces FUDO (grupos 11/12/60) a las especiales desde el snapshot
insert into product_modifier_groups (product_id,group_id,title,min_select,max_select,position)
  select product_id,group_id,title,min_select,max_select,position
  from _bak_pmg_pre14 b where b.group_id in (11,12,60)
    and not exists (select 1 from product_modifier_groups pmg
                    where pmg.product_id=b.product_id and pmg.group_id=b.group_id);

-- restaurar nombre/desc/precio/estado/favorito/orden desde el snapshot
update products p set name=b.name, description=b.description, price=b.price,
  is_active=b.is_active, is_favorite=b.is_favorite, sort_key=b.sort_key, category_id=b.category_id
  from _bak_p_pre14 b where p.id=b.id;
commit;
\echo 'rollback #14 aplicado.'
