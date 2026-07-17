begin;
update products p set name=b.name from _bak_products_pre07 b where p.id=b.id and p.name<>b.name;
update modifier_options o set name=b.name, is_active=b.is_active from _bak_modopts_pre07 b
 where o.id=b.id and (o.name<>b.name or o.is_active<>b.is_active);
commit;
\echo 'rollback #07 aplicado.'
