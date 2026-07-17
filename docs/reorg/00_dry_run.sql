-- DRY RUN — no modifica nada. Muestra qué cambiaría la migración de datos.
\echo '=== A) precios boneless / alitas / papas ==='
select id, name, price
from products
where is_active and (name ilike '%boneless%' or name ilike '%alitas%'
      or name ilike '%papas a la francesa%' or name ilike '%papas%gajo%')
order by name;

\echo '=== B) opciones de modificador en $0.01 ==='
select count(*) as opts_en_0_01 from modifier_options where price_delta = 0.01;

\echo '=== B2) otros productos activos con centavos ==='
select id, name, price from products where is_active and price <> round(price) order by price desc;

\echo '=== C) descontinuados (Chupa Chups / Smoothie / Mojito) ==='
select id, name, is_active from products
where name ilike '%chupa chups%' or name ilike '%smoothie%' or name ilike '%mojito%'
order by name;
