-- Corrige la fecha de negocio de las ventas del 29 de agosto de 2026 que quedaron guardadas con
-- la del 30.
--
-- POR QUÉ PASÓ: el servidor calculaba la fecha con el reloj en UTC. México es UTC-6, así que la
-- medianoche caía a las 18:00 del local: todo lo vendido de las 6pm en adelante se contaba en el
-- día siguiente, y el folio diario se reiniciaba a media cena dejando dos tickets #1 esa noche.
-- La causa la cerró la migración 0038 (zona horaria del local) más que la venta herede la fecha
-- del turno; esto arregla las filas que alcanzó a escribir el código viejo.
--
-- LAS HORAS NO SE TOCAN. `opened_at` es un instante y está bien: 02:28 UTC ES la hora a la que se
-- vendió. Lo que estaba mal era la FECHA derivada de ese instante. Cambiar la hora para "cuadrar"
-- el día falsificaría el registro de cuándo ocurrió la venta.
\set ON_ERROR_STOP on
BEGIN;

\echo '=== ANTES ==='
select id, business_date, daily_number,
       to_char(opened_at at time zone 'America/Mexico_City', 'DD/MM HH24:MI') as hora_mexico,
       total
from orders where company_id = 2 order by id;

-- +goose-nada: esto es un cambio de DATOS, no de esquema (patrón docs/reorg/).
do $fix$
declare
  v_empresa   constant bigint := 2;   -- gatobobah (producción)
  v_correcta  constant date   := date '2026-08-29';
  v_movidas   bigint;
  v_desc      bigint;
begin
  -- EN DOS FASES, y no de un tirón: `orders_company_daily_key` es único sobre
  -- (empresa, fecha, folio) y NO es deferrable, así que Postgres lo verifica fila por fila. Mover
  -- el #1 del día 30 al 29 choca con el #1 que ya está ahí, aunque al terminar la sentencia
  -- ninguno vaya a repetirse. Se aparta a un rango que nadie usa y de ahí se acomoda.

  -- Fase 1: apartar todas las ventas a folios temporales ÚNICOS POR FILA. Un simple +10000 no
  -- sirve: el #1 del día 29 y el #1 del día 30 seguirían siendo el mismo número y volverían a
  -- chocar al juntarlos en un día. Con el id —que es único— ningún par colisiona, esté en la
  -- fecha que esté.
  update orders set daily_number = 100000 + id::int where company_id = v_empresa;

  -- Fase 2a: la fecha correcta, derivada de la hora real. Ya sin conflicto posible.
  update orders o
     set business_date = (o.opened_at at time zone 'America/Mexico_City')::date
   where o.company_id = v_empresa
     and o.business_date <> (o.opened_at at time zone 'America/Mexico_City')::date;
  get diagnostics v_movidas = row_count;
  raise notice 'ventas con la fecha corregida: %', v_movidas;

  -- Fase 2b: el folio definitivo, en orden de HORA REAL — que es el orden en que se atendieron.
  with ordenadas as (
    select id, row_number() over (partition by business_date order by opened_at) as folio
    from orders
    where company_id = v_empresa
  )
  update orders o set daily_number = x.folio
  from ordenadas x
  where o.id = x.id;
  get diagnostics v_desc = row_count;
  raise notice 'folios asignados: %', v_desc;

  -- El contador del día tiene que quedar donde terminó la numeración, o la siguiente venta
  -- repetiría un folio ya usado.
  delete from order_counters where company_id = v_empresa;
  insert into order_counters (company_id, business_date, last_number)
  select v_empresa, business_date, max(daily_number)
  from orders where company_id = v_empresa
  group by business_date;

  -- Verificación: ningún folio repetido dentro de un mismo día.
  if exists (
    select 1 from orders where company_id = v_empresa
    group by business_date, daily_number having count(*) > 1
  ) then
    raise exception 'quedaron folios repetidos en el mismo día';
  end if;

  -- Y la fecha de cada venta tiene que coincidir con su hora real.
  if exists (
    select 1 from orders
    where company_id = v_empresa
      and business_date <> (opened_at at time zone 'America/Mexico_City')::date
  ) then
    raise exception 'alguna venta quedó con una fecha que no corresponde a su hora';
  end if;
end
$fix$;

\echo '=== DESPUES (las horas deben ser IDÉNTICAS a las de arriba) ==='
select id, business_date, daily_number,
       to_char(opened_at at time zone 'America/Mexico_City', 'DD/MM HH24:MI') as hora_mexico,
       total
from orders where company_id = 2 order by id;

\echo '=== contadores ==='
select business_date, last_number from order_counters where company_id = 2 order by 1;

COMMIT;
