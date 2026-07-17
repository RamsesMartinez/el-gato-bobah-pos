-- ROLLBACK migración #15.
begin;
update products p set name=b.name from _bak_pnames_pre15 b where p.id=b.id;
commit;
\echo 'rollback #15 aplicado. Recordá: redis-cli DEL pos:menu'
