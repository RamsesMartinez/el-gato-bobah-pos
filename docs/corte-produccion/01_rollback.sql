-- ROLLBACK de 01_nueva_empresa.sql — deja la base exactamente como estaba antes del corte.
--
-- Es corto porque el esquema hace el trabajo: TODA tabla de negocio referencia
-- `companies(id) on delete cascade` (ver 0023_tenant_columns.sql), así que borrar la empresa nueva
-- arrastra su catálogo entero sin tener que listar 24 tablas en orden inverso.
--
-- El orden importa: primero se borra la empresa nueva (que libera el slug `gatobobah`) y hasta
-- entonces la vieja lo puede recuperar — `companies.slug` es unique y al revés truena.
\set ON_ERROR_STOP on
BEGIN;

do $rollback$
declare
  v_new bigint;
  v_old bigint;
  v_n   bigint;
begin
  select id into v_old from companies where slug = 'bobah-pruebas';
  if v_old is null then
    raise exception 'No existe bobah-pruebas: el corte no se ha hecho, no hay nada que revertir.';
  end if;

  select id into v_new from companies where slug = 'gatobobah';
  if v_new is null then
    raise exception 'No existe la empresa nueva con slug gatobobah.';
  end if;
  if v_new = v_old then
    raise exception 'bobah-pruebas y gatobobah son la misma empresa (id %) — estado inconsistente, revisa a mano.', v_new;
  end if;

  -- Candado: si alguien ya cobró en la empresa nueva, este rollback destruiría ventas REALES.
  -- Mejor tronar y que un humano decida que revertir a mano.
  select count(*) into v_n from orders where company_id = v_new;
  if v_n > 0 then
    raise exception 'La empresa nueva ya tiene % pedidos: revertir borraria ventas reales. Aborta.', v_n;
  end if;

  delete from companies where id = v_new;   -- el cascade arrastra el catálogo copiado
  get diagnostics v_n = row_count;
  raise notice 'empresa nueva borrada (% fila) — el cascade se llevo su catalogo', v_n;

  update companies set slug = 'gatobobah', name = 'El Gato Bobah', updated_at = now()
  where id = v_old;
  raise notice 'empresa % recupero el slug gatobobah', v_old;
end
$rollback$;

\echo '=== VERIFICACION: debe quedar UNA sola empresa, con el slug original ==='
select id, slug, name, is_active from companies order by id;

\echo '=== VERIFICACION: no debe quedar NINGUNA fila de la empresa borrada (todo en 0) ==='
-- Los ids copiados no viven en un rango fijo (el offset es por tabla, ver 01_nueva_empresa.sql),
-- así que la comprobación es por tenant: si el cascade hizo su trabajo, ninguna tabla tiene filas
-- de una empresa que ya no existe.
select 'products' t, count(*) n from products where company_id not in (select id from companies)
union all select 'categories',       count(*) from categories       where company_id not in (select id from companies)
union all select 'modifier_options', count(*) from modifier_options where company_id not in (select id from companies)
union all select 'ingredients',      count(*) from ingredients      where company_id not in (select id from companies)
union all select 'recipe_items',     count(*) from recipe_items     where company_id not in (select id from companies)
union all select 'users',            count(*) from users            where company_id not in (select id from companies)
order by 1;

\echo '=== VERIFICACION: los conteos deben volver a los del dry run (58 pedidos, 502 productos) ==='
select 'orders' t, count(*) n from orders
union all select 'order_lines',       count(*) from order_lines
union all select 'register_sessions', count(*) from register_sessions
union all select 'products',          count(*) from products
union all select 'categories',        count(*) from categories
union all select 'modifier_options',  count(*) from modifier_options
union all select 'users',             count(*) from users
order by 1;

COMMIT;
