-- ROLLBACK migración #02.
begin;
-- chilaquiles → precio original del snapshot
update products p set price = b.price
  from _bak_products_20260706 b
 where p.id = b.id and p.id in (365,366,368,369);

-- eliminar el grupo Temperatura (primero los enlaces; las opciones caen por ON DELETE CASCADE)
delete from product_modifier_groups
 where group_id in (select id from modifier_groups where name = 'Temperatura');
delete from modifier_groups where name = 'Temperatura';
commit;
\echo 'rollback #02 aplicado.'
