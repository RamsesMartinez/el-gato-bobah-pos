-- ============================================================================
-- Spec 032, decisión del punto 1: los gastos que en realidad fueron propinas entregadas pasan a
-- movimiento de caja tipo propina. NO SE HA CORRIDO EN NINGÚN AMBIENTE.
--
-- Qué hace, por gasto:
--   * su salida de caja (el movimiento con expense_id) pasa a kind = 'propina', con quien la
--     recibió, y deja de apuntar al gasto;
--   * el gasto queda cancelado con un motivo que dice por qué: deja de contar en gastos.
-- El efectivo de cada turno NO cambia: la salida sigue siendo salida del cajón (NetCashMovements la
-- resta igual). La verificación del final lo comprueba y, si no cuadra, aborta.
--
-- Los ids y las personas NO viven en este repositorio (es público): se capturan al correrlo, de la
-- lista aprobada por el dueño. Cómo correrlo:
--   1. Respaldo de producción (make db-respaldo o el pg_dump de siempre) y restaurarlo en local
--      con make db-restaurar para ensayar ahí primero.
--   2. psql como owner, filtrando por la empresa por slug (company_id = 1 NO es El Gato Bobah):
--        \set slug gatobobah
--   3. Llenar la tabla temporal de abajo con (gasto, persona que recibió) y correr el resto.
--   4. Si algo no cuadra, el bloque de verificación aborta y la transacción se deshace sola.
--   Para revertir después de confirmar: 01_rollback.sql.
-- ============================================================================
begin;

-- psql no sustituye variables dentro de un bloque $$: el slug pasa por una tabla temporal.
create temp table _cfg as select :'slug'::text as slug;
create temp table _propinas_a_migrar (expense_id bigint primary key, recipient_user_id bigint not null);
-- insert into _propinas_a_migrar values (<gasto>, <usuario>), (<gasto>, <usuario>);

-- Respaldos para el rollback (en el esquema public, con fecha, como los de docs/reorg).
create table if not exists _bak_032_movimientos as
  select m.* from register_cash_movements m join _propinas_a_migrar p on p.expense_id = m.expense_id;
create table if not exists _bak_032_gastos as
  select e.* from expenses e join _propinas_a_migrar p on p.expense_id = e.id;

-- Los gastos tienen que ser de la empresa por slug, pagados en efectivo con su salida de caja, y la
-- persona activa de la misma empresa. Si no, se aborta antes de tocar nada.
do $$
declare faltan int;
begin
  select count(*) into faltan
    from _propinas_a_migrar p
    left join expenses e on e.id = p.expense_id
    left join companies c on c.id = e.company_id and c.slug = (select slug from _cfg)
    left join register_cash_movements m on m.expense_id = p.expense_id
    left join users u on u.id = p.recipient_user_id and u.company_id = e.company_id
   where e.id is null or c.id is null or m.id is null or u.id is null;
  if faltan > 0 then
    raise exception '% renglones no son gastos en efectivo de la empresa o la persona no es de ella', faltan;
  end if;
end $$;

create temp table _neto_antes as
  select m.session_id, sum(case when m.kind in ('entrada', 'reverso') then m.amount else -m.amount end) as neto
    from register_cash_movements m
   where m.session_id in (select session_id from _bak_032_movimientos)
   group by m.session_id;

update register_cash_movements m
   set kind = 'propina', expense_id = null,
       recipient_user_id = p.recipient_user_id,
       recipient_name = (select u.name from users u where u.id = p.recipient_user_id),
       concept = 'Propina (antes registrada como gasto)'
  from _propinas_a_migrar p
 where m.expense_id = p.expense_id;

update expenses e
   set status = 'cancelada', cancelled_at = now(),
       cancel_reason = 'Era una propina entregada: pasó a movimiento de caja tipo propina (spec 032)'
  from _propinas_a_migrar p
 where e.id = p.expense_id;

-- Verificación: el neto de efectivo de cada turno tocado es el mismo.
do $$
declare distintos int;
begin
  select count(*) into distintos
    from _neto_antes a
    join (select m.session_id, sum(case when m.kind in ('entrada', 'reverso') then m.amount else -m.amount end) as neto
            from register_cash_movements m where m.session_id in (select session_id from _neto_antes)
           group by m.session_id) d on d.session_id = a.session_id
   where a.neto <> d.neto;
  if distintos > 0 then
    raise exception 'el efectivo de % turnos cambió: se aborta', distintos;
  end if;
end $$;

-- Revisar a mano antes de confirmar: los gastos ya no salen en gastos y las propinas sí en caja.
select e.id, e.status, e.cancel_reason from expenses e join _propinas_a_migrar p on p.expense_id = e.id;
select m.id, m.session_id, m.kind, m.amount, m.recipient_name from register_cash_movements m
  join _bak_032_movimientos b on b.id = m.id;

-- commit;   -- solo con el visto bueno del dueño, después de revisar lo de arriba
rollback;
