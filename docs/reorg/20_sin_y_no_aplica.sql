-- 20 — Los extras «Sin…» y «No Aplica» no gastan insumos.
--
-- POR QUÉ. Decisión del dueño (2026-10-11, opción A): un «Sin cebolla» no devuelve al almacén lo que
-- la receta del platillo ya descuenta; se acepta ese sobredescuento chico a cambio de no capturar
-- recetas en negativo. Estos extras quedaban como «pendientes» en Menú › Recetas sin serlo. Se
-- marcan igual que la pantalla al guardar «No gasta insumos»: sin receta, sin producto ligado y
-- confirmados por una persona.
--
-- Lista cerrada, sacada de producción el 2026-10-11 (respaldo pre-sin-noaplica-20261011-2235):
-- solo los activos en grupos activos, que son los que muestra Recetas. Quedan FUERA a propósito
-- «Sin Perlas» (622, extra y grupo apagados) y «No Aplica» de Granizado (1031, grupo apagado).
--
-- Guardas: corre como gatobobah_app con la empresa 2 (RLS filtra), y aborta si la lista no
-- coincide en id, nombre, estado o si alguno ya tiene receta. Por eso una segunda corrida aborta.
-- Termina en ROLLBACK: para aplicarlo se cambia la última línea por COMMIT.

\set ON_ERROR_STOP on
begin;
select set_config('app.company_id', '2', true);
set local role gatobobah_app;
-- El mismo candado que toma la pantalla al capturar una receta.
select pg_advisory_xact_lock(28028, 2);

create temp table sin_lista (id bigint primary key, nombre text not null) on commit drop;
insert into sin_lista values
  (596,  'No Aplica'),
  (603,  'No Aplica'),
  (1101, 'No Aplica'),
  (1100, 'No Aplica'),
  (555,  'Sin Crema Batida'),
  (1110, 'Sin Crema Batida'),
  (690,  'Sin Preparar'),
  (682,  'Sin Proteina'),
  (1126, 'Sin perlas explosivas'),
  (636,  'Sin perlas explosivas'),
  (1137, 'Sin perlas explosivas'),
  (664,  'Sin perlas explosivas'),
  (650,  'Sin perlas explosivas');

do $$
declare
  listos int;
  quien bigint;
begin
  select count(*) into listos
    from sin_lista l
    join modifier_options o on o.id = l.id and o.name = l.nombre
    join modifier_groups g on g.id = o.group_id
   where o.is_active and g.is_active
     and o.composition_status is null and o.recipe_id is null and o.linked_product_id is null;
  if listos <> 13 then
    raise exception 'se esperaban 13 extras pendientes y sin receta, hay %: nada se cambió', listos;
  end if;
  select id into quien from users where username = 'admin' and is_active;
  if quien is null then
    raise exception 'no está el usuario admin de gatobobah';
  end if;
  update modifier_options o
     set composition_status = 'confirmed', composition_confirmed_by = quien, composition_confirmed_at = now()
    from sin_lista l
   where o.id = l.id;
  get diagnostics listos = row_count;
  if listos <> 13 then
    raise exception 'se marcaron % y no 13: se deshace', listos;
  end if;
end $$;

-- Verificación: los 13, confirmados y sin receta.
select o.id, o.name, g.name as grupo, o.composition_status, o.recipe_id, o.linked_product_id, o.composition_confirmed_at
  from sin_lista l join modifier_options o on o.id = l.id join modifier_groups g on g.id = o.group_id
 order by o.name, g.name;

rollback;
