-- ============================================================================
-- Migración #16 — limpieza de GRUPOS de modificadores (heredados de FUDO).
-- Contexto: los nombres "Grupo NN" son IDs de grupo de FUDO. En el POS no se ven
-- mientras el enlace producto→grupo tenga `title` (menú = coalesce(pmg.title, mg.name)),
-- así que renombrar es sobre todo higiene para el panel de admin... EXCEPTO 8 grupos
-- sin título, que hoy muestran "Grupo NN" literal al armar la orden.
--
-- Hace, en una sola transacción:
--   1. Renombra todos los grupos genéricos activos a nombres reales (derivados del
--      título que ya les dan los productos, o de su contenido).
--   2. Fusiona grupos duplicados IDÉNTICOS (mismo precio verificado):
--        - Tipo de leche: 4 grupos idénticos (+$12/$0) → 1. Incluye Grupo 91 (leche
--          +$10 anómala) → al reapuntarlo al canónico se corrige el precio.
--        - Refresco: 2 idénticos → 1 (min/max es override por-producto, se conserva).
--        - Perlas explosivas (+$20): 3 casi idénticos → 1 (el canónico tiene "Sin"=$0,
--          lo que corrige de paso el bug de "Sin"=+$20 del grupo gemelo).
--        - Salsa extra (+$15): 2 gemelos que solo diferían en "Salsa Brava CH/G" → 1.
--      Re-apunta los productos al grupo canónico (conserva min/max/título por producto)
--      y desactiva el grupo gemelo + sus opciones (NO se borran: el histórico de
--      órdenes las sigue referenciando).
--   3. Desambigua Chilaquiles: dos grupos titulados "Proteina"/"Proteína" → "Proteína"
--      (base) y "Proteína extra".
--   4. Corrige typo "Ruffless Queso" → "Ruffles Queso" y unifica "Salsa Brava CH" →
--      "Salsa Brava" en el grupo de salsa extra canónico.
--   5. Arregla producto con grupo equivocado: "Paldo Kokomen" tenía "Aderezos Extra"
--      (ranch de alitas) en vez de "Ingrediente ramen extra" como los demás ramen.
--
-- NO toca: los 12 grupos ya inactivos (invisibles en POS; borrarlos es riesgoso por el
-- FK del histórico de órdenes) ni el enlace muerto de Grupo 97 en Combo Corean (grupo
-- inactivo, se filtra del menú; inofensivo).
--
-- Reversible: snapshots _bak_*_pre16 (16_rollback.sql).
-- IDs = ids INTERNOS de modifier_groups (no el número del nombre "Grupo NN").
-- Ejecutar UNA vez.
-- ============================================================================
begin;

create table if not exists _bak_modgroups_pre16 as select id, name, is_active from modifier_groups;
create table if not exists _bak_pmg_pre16       as select id, group_id, title, min_select, max_select from product_modifier_groups;
create table if not exists _bak_modopts_pre16   as select id, name, is_active from modifier_options;

-- ---------------------------------------------------------------------------
-- 1. Renombrar grupos genéricos → nombres reales
-- ---------------------------------------------------------------------------
update modifier_groups set name='Aderezo de cortesía'             where id=1;
update modifier_groups set name='Aderezos extra'                  where id=2;
update modifier_groups set name='Crema batida'                    where id=3;
update modifier_groups set name='Refresco'                        where id=4;   -- canónico refresco
update modifier_groups set name='Topping banderilla'             where id=6;
update modifier_groups set name='Relleno banderilla'             where id=8;
update modifier_groups set name='Tipo de crepa/crepizza'          where id=10;
update modifier_groups set name='Decoración crepa'                where id=11;
update modifier_groups set name='Fruta crepa (dulce)'             where id=12;  -- evita choque con 73 "Fruta crepa"
update modifier_groups set name='Ingrediente ramen extra'         where id=13;
update modifier_groups set name='Ingrediente ramen incluido'      where id=14;
update modifier_groups set name='Perlas explosivas'               where id=18;  -- canónico perlas +$20
update modifier_groups set name='Perlas granizado'                where id=19;  -- +$15, se conserva aparte
update modifier_groups set name='Perlas explosivas (soda)'        where id=20;  -- superset, se conserva aparte
update modifier_groups set name='Proteína'                        where id=21;
update modifier_groups set name='Proteína extra'                  where id=22;
update modifier_groups set name='Preparación ramen'               where id=24;
update modifier_groups set name='Sabor bobah tea'                 where id=25;
update modifier_groups set name='Sabor frappé'                    where id=27;
update modifier_groups set name='Sabor frappé de agua granizado'  where id=28;
update modifier_groups set name='Sabor licuado'                   where id=29;
update modifier_groups set name='Sabor malteada'                  where id=30;
update modifier_groups set name='Milkis'                          where id=31;
update modifier_groups set name='Sabor mini donas 14pz'           where id=32;
update modifier_groups set name='Sabor mini donas 8pz'            where id=33;
update modifier_groups set name='Sabor frappé de leche'           where id=35;
update modifier_groups set name='Sabor ramen'                     where id=36;
update modifier_groups set name='Sabor soda explosiva'            where id=39;
update modifier_groups set name='Sabor de té'                     where id=40;
update modifier_groups set name='Tteokbokki'                      where id=41;
update modifier_groups set name='Salsa extra'                     where id=46;  -- canónico salsa extra +$15
update modifier_groups set name='Sazonador alitas'                where id=48;
update modifier_groups set name='Arizona'                         where id=49;
update modifier_groups set name='Tamaño vaso'                     where id=50;
update modifier_groups set name='Tamaño frappé de agua granizado' where id=51;
update modifier_groups set name='Tamaño frappé'                   where id=52;
update modifier_groups set name='Tipo de leche (con agua)'        where id=55;
update modifier_groups set name='Tipo de leche'                   where id=56;  -- canónico leche +$12/$0
update modifier_groups set name='Toppings crepa'                  where id=60;
update modifier_groups set name='Decoración crepa (básica)'       where id=61;
update modifier_groups set name='Granizado'                       where id=62;

-- ---------------------------------------------------------------------------
-- 2. Fusiones (re-apuntar productos → grupo canónico, desactivar el gemelo)
-- ---------------------------------------------------------------------------
-- Refresco: Grupo 26(5) → Grupo 25(4)
update product_modifier_groups set group_id=4  where group_id=5;
-- Tipo de leche: Grupo 17(57), Grupo 93(58), Grupo 52(59), Grupo 91(15) → Grupo 36(56)
update product_modifier_groups set group_id=56 where group_id in (57,58,59,15);
-- Perlas explosivas: Grupo 34(16), Grupo 7(17) → Grupo 53(18)
update product_modifier_groups set group_id=18 where group_id in (16,17);
-- Salsa extra: Grupo 55(47) → Grupo 54(46)
update product_modifier_groups set group_id=46 where group_id=47;

-- desactivar grupos gemelos y sus opciones (no se borran; histórico de órdenes)
update modifier_options set is_active=false where group_id in (5,15,16,17,47,57,58,59);
update modifier_groups  set is_active=false where id       in (5,15,16,17,47,57,58,59);

-- ---------------------------------------------------------------------------
-- 3. Chilaquiles: desambiguar los dos grupos "Proteína"
-- ---------------------------------------------------------------------------
update product_modifier_groups set title='Proteína'       where group_id=21;
update product_modifier_groups set title='Proteína extra' where group_id=22;

-- ---------------------------------------------------------------------------
-- 4. Corregir opciones del grupo salsa extra canónico (id 46)
-- ---------------------------------------------------------------------------
update modifier_options set name='Ruffles Queso' where id=407; -- typo "Ruffless"
update modifier_options set name='Salsa Brava'   where id=408; -- unificar "Salsa Brava CH/G"

-- ---------------------------------------------------------------------------
-- 5. Paldo Kokomen (262): tenía "Aderezos Extra" (grupo 2) en vez del grupo ramen.
--    fila pmg id 12 → "Ingrediente ramen extra" (grupo 13), como sus hermanos ramen.
-- ---------------------------------------------------------------------------
update product_modifier_groups
   set group_id=13, title='Ingrediente Extra Ramen', min_select=1, max_select=8
 where id=12;

commit;

-- ===========================  VERIFICACIÓN  =================================
\echo '--- grupos con nombre genérico "Grupo NN" que sigan ACTIVOS (debe ser 0) ---'
select id, name from modifier_groups where is_active and name ~ '^Grupo ' order by id;

\echo '--- grupos ACTIVOS que un producto muestra SIN título (nombre crudo en POS) ---'
select distinct g.id, g.name from product_modifier_groups pmg
join modifier_groups g on g.id=pmg.group_id
where g.is_active and pmg.title is null order by g.id;

\echo '--- gemelos desactivados (debe: f para 5,15,16,17,47,57,58,59) ---'
select id, name, is_active from modifier_groups where id in (5,15,16,17,47,57,58,59) order by id;

\echo '--- Chilaquiles: sus dos grupos de proteína con títulos distintos ---'
select p.name, g.id, g.name grupo, pmg.title from product_modifier_groups pmg
join products p on p.id=pmg.product_id join modifier_groups g on g.id=pmg.group_id
where p.id in (365,366,368,369) and g.id in (21,22) order by p.name, g.id;

\echo '--- Paldo Kokomen (262): debe usar Ingrediente ramen extra(13) + Preparación(24) ---'
select g.id, g.name, pmg.title, pmg.min_select, pmg.max_select from product_modifier_groups pmg
join modifier_groups g on g.id=pmg.group_id where pmg.product_id=262 order by pmg.position;

\echo '--- salsa extra canónico (46): "Salsa Brava" y "Ruffles Queso" sin typo ---'
select id, name from modifier_options where id in (407,408);
