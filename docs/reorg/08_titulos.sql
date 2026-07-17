-- ============================================================================
-- Migración #08 — títulos de grupo concisos en los modales (product_modifier_groups.title).
-- "Elige el sabor de tus salsas" → "Salsas", "¿Agregar Salsa Extra?" → "Salsa extra", etc.
-- Solo cambia el texto que ve el operador como encabezado del grupo. Reversible.
-- ============================================================================
begin;
create table if not exists _bak_pmg_titles_pre08 as select id, title from product_modifier_groups;

update product_modifier_groups set title='Salsas'            where title='Elige el sabor de tus salsas';
update product_modifier_groups set title='Sabor'             where title in ('Elige el sabor de tu soda explosiva','Elige el sabor de tu arizona','Sabor de tu Bobah Tea');
update product_modifier_groups set title='Perlas explosivas' where title='Elige las perlas explosiva';
update product_modifier_groups set title='Tipo de leche'     where title='Elija el tipo de Leche';
update product_modifier_groups set title='Granizado'         where title='Has tu bobah Granizado';
update product_modifier_groups set title='Salsa extra'       where title='¿Agregar Salsa Extra?';
update product_modifier_groups set title='Preparación'       where title='¿Ramen preparado?';
update product_modifier_groups set title='Decoración'        where title='Elige tu decoración';
update product_modifier_groups set title='Refresco'          where title in ('Elige tu refresco','Agrega tus refrescos');
update product_modifier_groups set title='Fruta'             where title='Agrega fruta';
update product_modifier_groups set title='Toppings'          where title='Agrega tus toppings';

commit;
\echo '--- títulos verbosos restantes (debe quedar pocos/ninguno) ---'
select distinct title from product_modifier_groups
where title ~* '^(elige|elija|has|agrega|¿|selecciona)' order by 1;
