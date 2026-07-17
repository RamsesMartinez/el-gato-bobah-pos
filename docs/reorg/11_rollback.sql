begin;
update products p set price=b.price, is_active=b.is_active from _bak_products_pre11 b where p.id=b.id;
commit;
\echo 'rollback #11 aplicado.'
