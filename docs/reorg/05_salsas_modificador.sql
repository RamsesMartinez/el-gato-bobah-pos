-- ============================================================================
-- Migración #05 — consolidar salsas de Boneless/Alitas en UN grupo "Salsas".
-- - Renombra el grupo 43 → "Salsas" y limpia los 32 nombres (sin "SALSA CH", con
--   acrónimos correctos). Nombres = referencia del PDF 2026 + limpieza.
-- - Re-apunta los productos del grupo gemelo 44 → 43 (conserva min/max por tamaño:
--   2 salsas estándar, 3 en 1 kg). Desactiva el grupo 44 y sus opciones (no se borra;
--   el histórico de órdenes las sigue referenciando).
-- - Inactiva 5 productos-opción sueltos (salsas/genéricos $0 que ensuciaban el grid).
-- Reversible: snapshots _bak_modgroups/_bak_modopts/_bak_pmg/_bak_products (05_rollback.sql).
-- ============================================================================
begin;
create table if not exists _bak_modgroups_20260706 as select * from modifier_groups;
-- (_bak_modopts_20260706, _bak_pmg_20260706, _bak_products_20260706 ya existen)

update modifier_groups set name = 'Salsas' where id = 43;

-- nombres limpios (mapeo explícito por id; preserva BBQ y artículos en minúscula)
update modifier_options set name='Ajo Parmesano'     where id=319;
update modifier_options set name='BBQ a la Diabla'   where id=320;
update modifier_options set name='BBQ Chipotle'      where id=321;
update modifier_options set name='BBQ Habanero'      where id=322;
update modifier_options set name='BBQ'               where id=323;
update modifier_options set name='Búfalo Champi'     where id=324;
update modifier_options set name='Búfalo'            where id=325;
update modifier_options set name='Cajún'             where id=326;
update modifier_options set name='Cheetos'           where id=327;
update modifier_options set name='Chingu Teriyaki'   where id=328;
update modifier_options set name='Doritos Nacho'     where id=329;
update modifier_options set name='Flamin Hot'        where id=330;
update modifier_options set name='Fresa Spicy'       where id=331;
update modifier_options set name='Jalapeño Spicy'    where id=332;
update modifier_options set name='Krazy Hot'         where id=333;
update modifier_options set name='Lemon Pepper'      where id=334;
update modifier_options set name='Louisiana'         where id=335;
update modifier_options set name='Mango Guajillo'    where id=336;
update modifier_options set name='Mango Habanero'    where id=337;
update modifier_options set name='Mango Hot'         where id=338;
update modifier_options set name='Mezcat'            where id=339;
update modifier_options set name='Naranja Chipotle'  where id=340;
update modifier_options set name='Original Hot'      where id=341;
update modifier_options set name='Pelónrico Gatuno'  where id=342;
update modifier_options set name='Piña Guajillo'     where id=343;
update modifier_options set name='Rancheritos'       where id=344;
update modifier_options set name='Ruffles Queso'     where id=345;
update modifier_options set name='Salsa Brava'       where id=346;
update modifier_options set name='Sriracha'          where id=347;
update modifier_options set name='Tamarindo'         where id=348;
update modifier_options set name='Zarzamora Chipotle' where id=349;
update modifier_options set name='Tupsi Pop'         where id=350;

-- re-apuntar los productos del grupo gemelo 44 → 43 (conserva min/max de cada producto)
update product_modifier_groups set group_id = 43 where group_id = 44;

-- desactivar el grupo gemelo y sus opciones (ya no se usa)
update modifier_options set is_active = false where group_id = 44;
update modifier_groups  set is_active = false where id = 44;

-- inactivar productos-opción sueltos que quedaban en el grid
update products set is_active = false where id in (232,233,234,238,239);

commit;

\echo '--- grupo Salsas: opciones activas (debe ~32, nombres limpios) ---'
select count(*) from modifier_options where group_id=43 and is_active;
\echo '--- boneless/alitas ahora usan el grupo 43 con su max por tamaño ---'
select pmg.product_id, p.name, pmg.group_id, pmg.min_select, pmg.max_select
from product_modifier_groups pmg join products p on p.id=pmg.product_id
where p.id in (212,213,214,216,217,218,219,220) order by p.name;
\echo '--- grupo 44 desactivado ---'
select is_active from modifier_groups where id=44;
