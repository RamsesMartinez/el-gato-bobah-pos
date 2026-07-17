-- ROLLBACK de la migración #01 — restaura desde los snapshots _bak_*_20260706.
-- Deja products y modifier_options exactamente como estaban antes.
begin;
update products p
   set price = b.price, is_active = b.is_active
  from _bak_products_20260706 b
 where p.id = b.id
   and (p.price is distinct from b.price or p.is_active is distinct from b.is_active);

update modifier_options o
   set price_delta = b.price_delta
  from _bak_modopts_20260706 b
 where o.id = b.id
   and o.price_delta is distinct from b.price_delta;
commit;

\echo 'rollback aplicado. (los snapshots _bak_*_20260706 se conservan; bórralos manualmente cuando confirmes)'
