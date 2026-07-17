begin;
update product_modifier_groups p set title=b.title from _bak_pmg_titles_pre08 b
 where p.id=b.id and p.title is distinct from b.title;
commit;
\echo 'rollback #08 aplicado.'
