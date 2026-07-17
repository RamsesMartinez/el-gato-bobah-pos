-- ROLLBACK migración #05.
begin;
-- re-apuntar productos de vuelta a su grupo original (43/44) desde el snapshot de pmg
update product_modifier_groups pmg set group_id = b.group_id
  from _bak_pmg_20260706 b
 where pmg.id = b.id and pmg.group_id is distinct from b.group_id;
-- restaurar nombres/estado de opciones y grupos
update modifier_options o set name = b.name, is_active = b.is_active
  from _bak_modopts_20260706 b where o.id = b.id
   and (o.name is distinct from b.name or o.is_active is distinct from b.is_active);
update modifier_groups g set name = b.name, is_active = b.is_active
  from _bak_modgroups_20260706 b where g.id = b.id;
-- reactivar los productos-opción
update products p set is_active = b.is_active
  from _bak_products_20260706 b where p.id = b.id and p.id in (232,233,234,238,239);
commit;
\echo 'rollback #05 aplicado.'
