begin;
update products p set name=b.name, price=b.price, is_active=b.is_active from _bak_products_pre12 b where p.id=b.id;
delete from product_modifier_groups where group_id in (select id from modifier_groups where name in ('Tamaño boneless','Tamaño alitas'));
delete from modifier_groups where name in ('Tamaño boneless','Tamaño alitas');
commit;
\echo 'rollback #12 aplicado.'
