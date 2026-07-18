-- Rollback #16 — restaura nombres/estado de grupos, enlaces producto-grupo
-- (group_id/título/min/max) y nombres/estado de opciones, desde los snapshots _pre16.
-- Seguro ante el UNIQUE de modifier_groups.name: los nombres destino son los originales
-- (únicos) y los nombres nuevos no colisionan con ninguno, así que no hay choque transitorio.
begin;

update modifier_groups g set name=b.name, is_active=b.is_active
  from _bak_modgroups_pre16 b
 where g.id=b.id
   and (g.name is distinct from b.name or g.is_active is distinct from b.is_active);

update product_modifier_groups p
   set group_id=b.group_id, title=b.title, min_select=b.min_select, max_select=b.max_select
  from _bak_pmg_pre16 b
 where p.id=b.id
   and (p.group_id is distinct from b.group_id
     or p.title is distinct from b.title
     or p.min_select is distinct from b.min_select
     or p.max_select is distinct from b.max_select);

update modifier_options o set name=b.name, is_active=b.is_active
  from _bak_modopts_pre16 b
 where o.id=b.id
   and (o.name is distinct from b.name or o.is_active is distinct from b.is_active);

commit;
\echo 'rollback #16 aplicado.'
