begin;
update products p set category_id=b.category_id from _bak_bigcat_pre10 b where p.id=b.id;
update categories set is_active=false where parent_id in (4,37) and name in
  ('Embotelladas','Frappés & Licuados','Chamoyadas & Sodas','Especiales & KPOP','Salados','Dulces & Mochi');
commit;
\echo 'rollback #10 aplicado (productos vuelven a la raíz; subs desactivadas).'
