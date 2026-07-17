-- ROLLBACK migración #06 (bebidas).
begin;
update modifier_options o set name=b.name, is_active=b.is_active
  from _bak_modopts_20260706 b
 where o.id=b.id and o.group_id in (27,25,17,3,52)
   and (o.name is distinct from b.name or o.is_active is distinct from b.is_active);
update product_modifier_groups pmg set title=b.title
  from _bak_pmg_20260706 b where pmg.id=b.id and pmg.group_id in (27,17,3,52);
update products p set name=b.name from _bak_products_20260706 b where p.id=b.id and p.id in (128,104);
commit;
\echo 'rollback #06 aplicado.'
