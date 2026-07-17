begin;
update products p set category_id=b.category_id, is_active=b.is_active
  from _bak_otro_pre09 b where p.id=b.id;
update categories set name='Otro (revisar)' where id=30;
commit;
\echo 'rollback #09 aplicado.'
