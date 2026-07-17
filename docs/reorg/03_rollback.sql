-- ROLLBACK migración #03 — restaura categorías y category_id de productos.
begin;
update products p set category_id = b.category_id
  from _bak_products_cat_20260706 b where p.id = b.id and p.category_id is distinct from b.category_id;
update categories c set name = b.name, is_active = b.is_active, parent_id = b.parent_id
  from _bak_categories_20260706 b where c.id = b.id;
commit;
\echo 'rollback #03 aplicado.'
