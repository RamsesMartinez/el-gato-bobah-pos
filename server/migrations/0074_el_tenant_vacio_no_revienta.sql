-- +goose Up

-- EL FAIL-CLOSED DE RLS NO SE CUMPLÍA EN UNA CONEXIÓN RECICLADA.
--
-- `Store.QC` promete, por escrito, que con el rol de la aplicación y sin conexión de tenant RLS no
-- devuelve nada — fail-closed en vez de filtrar entre empresas. Es la propiedad sobre la que se
-- sostiene el modelo multi-tenant entero: es lo que hace que un `store.Q` olvidado devuelva cero
-- filas en vez de las de otra empresa.
--
-- Esa promesa es cierta en una conexión VIRGEN y FALSA en una RECICLADA. Medido contra Postgres 16
-- antes de escribir esto, en una base sin default a nivel base:
--
--   conexión virgen     current_setting('app.company_id', true)  →  NULL
--   tras `reset`        el mismo setting                         →  '' (cadena vacía)
--
-- Y `''::bigint` no es NULL: es `ERROR: invalid input syntax for type bigint: ""` (22P02).
--
-- El pool RECICLA, y el release de `AcquireTenant` hace `reset app.company_id`. O sea que la forma
-- que revienta es la NORMAL en producción, no el borde. La misma consulta falla de dos maneras
-- distintas según le toque una conexión nueva o una reusada.
--
-- POR QUÉ NO SE HABÍA VISTO. Las bases locales y las restauradas traen un `alter database ... set
-- app.company_id`, que le da valor al setting incluso sin tenant y tapa las dos formas. Producción
-- es el único ambiente sin ese default, o sea el único expuesto. El arnés de pruebas hacía lo mismo
-- a nivel base, así que `appRoleStore` significaba «empresa 1» y ninguna prueba podía verlo — eso
-- se corrigió en el mismo cambio que esta migración.
--
-- EL ARREGLO: `nullif(..., '')` antes del cast, en el `using` Y en el `with check` de las 56
-- políticas. Una cadena vacía pasa a ser NULL, y NULL es lo que ya se comportaba bien.
--
-- Se reescriben TODAS y no solo las de plataforma: el defecto es de la forma del predicado, que es
-- idéntica en las 56. Arreglar unas cuantas dejaría el resto con el mismo 22P02 esperando, y peor:
-- haría que dos tablas vecinas se comportaran distinto ante la misma conexión.

set local lock_timeout = '3s';

-- SE BUSCAN POR LA FORMA DEL PREDICADO, NO POR EL NOMBRE.
--
-- La primera versión de esta migración recorría las políticas llamadas `tenant_isolation` y dejó
-- fuera la de `companies`, que se llama `company_self` y compara contra `id` en vez de contra
-- `company_id`. El test la encontró: seguía reventando con 22P02.
--
-- Buscar por nombre supone que todas se llaman igual, y eso ya resultó falso una vez. Se busca por
-- lo que tienen en común de verdad: el `current_setting` sin proteger.
--
-- Y se reescribe el predicado que ya tenían, sustituyendo solo esa llamada: así una política con
-- otra columna, otro rol o otro verbo sobrevive intacta salvo por el arreglo.
-- +goose StatementBegin
do $$
declare
  r record;
  nuevo_using text;
  nuevo_check text;
  lista_de_roles text;
  n int := 0;
begin
  for r in
    select schemaname, tablename, policyname, permissive, cmd, roles, qual, with_check
      from pg_policies
     where schemaname = 'public'
       and (qual like '%current_setting(''app.company_id''%' or with_check like '%current_setting(''app.company_id''%')
       and coalesce(qual, '') || coalesce(with_check, '') not ilike '%nullif%'
     order by tablename, policyname
  loop
    nuevo_using := replace(r.qual,
      'current_setting(''app.company_id''::text, true)',
      'nullif(current_setting(''app.company_id''::text, true), '''')');
    nuevo_check := replace(r.with_check,
      'current_setting(''app.company_id''::text, true)',
      'nullif(current_setting(''app.company_id''::text, true), '''')');
    lista_de_roles := array_to_string(r.roles, ', ');

    execute format('drop policy %I on %I.%I', r.policyname, r.schemaname, r.tablename);
    -- `ilike` en las comparaciones de arriba: Postgres normaliza la expresión al guardarla y la
    -- devuelve como `NULLIF` en MAYÚSCULAS. Con `like` en minúsculas, la comprobación final creía
    -- que ninguna se había arreglado y la migración abortaba. Lo atrapó su propia guarda.
    execute format('create policy %I on %I.%I as %s for %s to %s %s %s',
      r.policyname, r.schemaname, r.tablename,
      case when r.permissive = 'PERMISSIVE' then 'permissive' else 'restrictive' end,
      r.cmd, lista_de_roles,
      case when nuevo_using is null then '' else 'using (' || nuevo_using || ')' end,
      case when nuevo_check is null then '' else 'with check (' || nuevo_check || ')' end);
    n := n + 1;
  end loop;

  -- Un cero significaría que esta migración no hizo nada, en silencio. Prefiero que falle el
  -- despliegue a que el fail-closed siga siendo mentira.
  if n = 0 then
    raise exception 'ninguna política usa current_setting(app.company_id) sin proteger: esta migración no habría hecho nada';
  end if;
  raise notice 'predicado de tenant reescrito en % políticas', n;

  -- Y que no quede una sola con la forma vieja, por si alguna se escribió distinto.
  if exists (
    select 1 from pg_policies
     where schemaname = 'public'
       and (qual like '%current_setting(''app.company_id''%' or with_check like '%current_setting(''app.company_id''%')
       and coalesce(qual, '') || coalesce(with_check, '') not ilike '%nullif%'
  ) then
    raise exception 'quedaron políticas con el cast sin proteger: la sustitución textual no las alcanzó';
  end if;
end $$;
-- +goose StatementEnd

-- +goose Down

-- Devuelve la forma anterior, con el defecto incluido. Se revierte tal cual estaba: un Down que
-- "mejora" algo hace que ir y volver no sea una identidad.
set local lock_timeout = '3s';

-- +goose StatementBegin
do $$
declare
  r record;
  viejo_using text;
  viejo_check text;
begin
  for r in
    select schemaname, tablename, policyname, permissive, cmd, roles, qual, with_check
      from pg_policies
     where schemaname = 'public'
       and (coalesce(qual, '') || coalesce(with_check, '')) like '%nullif(current_setting(''app.company_id''%'
  loop
    viejo_using := replace(r.qual,
      'nullif(current_setting(''app.company_id''::text, true), ''''::text)',
      'current_setting(''app.company_id''::text, true)');
    viejo_check := replace(r.with_check,
      'nullif(current_setting(''app.company_id''::text, true), ''''::text)',
      'current_setting(''app.company_id''::text, true)');
    execute format('drop policy %I on %I.%I', r.policyname, r.schemaname, r.tablename);
    -- `ilike` en las comparaciones de arriba: Postgres normaliza la expresión al guardarla y la
    -- devuelve como `NULLIF` en MAYÚSCULAS. Con `like` en minúsculas, la comprobación final creía
    -- que ninguna se había arreglado y la migración abortaba. Lo atrapó su propia guarda.
    execute format('create policy %I on %I.%I as %s for %s to %s %s %s',
      r.policyname, r.schemaname, r.tablename,
      case when r.permissive = 'PERMISSIVE' then 'permissive' else 'restrictive' end,
      r.cmd, array_to_string(r.roles, ', '),
      case when viejo_using is null then '' else 'using (' || viejo_using || ')' end,
      case when viejo_check is null then '' else 'with check (' || viejo_check || ')' end);
  end loop;
end $$;
-- +goose StatementEnd
