-- Pantalla de Ventas (análisis). Distinta del tablero de pedidos: aquí se mira lo que YA pasó.
--
-- Las cinco consultas comparten el mismo `where` y viven juntas A PROPÓSITO: si el filtro de la
-- lista y el del resumen divergen, las cifras de arriba dejan de cuadrar con la tabla de abajo y
-- nadie sabe cuál de las dos miente. Se editan en la misma pasada.
--
-- Ninguna filtra por company_id, y no es un olvido: RLS agrega ese predicado a toda consulta del rol
-- `gatobobah_app`. Además sqlc NO conoce la columna —la migración 0023 la agregó con SQL dinámico
-- que su parser no puede leer—, así que nombrarla aquí rompería `sqlc generate` por una columna que
-- sí existe en Postgres.
--
-- El resumen va en tres consultas y no en una: `order_payments` y `order_lines` son ambas 1:N con
-- `orders`, así que unirlas en la misma consulta multiplica las filas (2 pagos × 3 líneas = 6) y
-- duplica las sumas.

-- name: ListSales :many
-- Orden por columna: @sort ∈ (fecha|folio|total|estado|tipo) × @dir (asc|desc). En SQL y no en el
-- cliente porque la lista está paginada: ordenar solo las 20 filas visibles daría un orden falso.
-- El desempate fijo (opened_at desc, id desc) evita que dos ventas del mismo total bailen entre
-- páginas.
select o.id, o.daily_number, o.folio_name, o.business_date, o.opened_at, o.completed_at,
       o.status, o.service_type, o.customer_name, o.total, o.delivery_fee, o.refund_amount,
       dp.name as platform,
       u.name as opened_by_name,
       (select coalesce(sum(op.tip_amount), 0) from order_payments op where op.order_id = o.id)::numeric(10,2) as tips,
       (select string_agg(distinct pm.name, ' + ' order by pm.name)
          from order_payments op join payment_methods pm on pm.id = op.payment_method_id
         where op.order_id = o.id) as methods
from orders o
left join delivery_platforms dp on dp.id = o.delivery_platform_id
left join users u on u.id = o.opened_by
where o.business_date between @desde and @hasta
  and (sqlc.narg('status')::order_status is null or o.status = sqlc.narg('status'))
  and (sqlc.narg('service_type')::service_type is null or o.service_type = sqlc.narg('service_type'))
order by
  case when @sort::text = 'total'  and @dir::text = 'asc'  then o.total end asc  nulls last,
  case when @sort::text = 'total'  and @dir::text <> 'asc' then o.total end desc nulls last,
  case when @sort::text = 'folio'  and @dir::text = 'asc'  then o.daily_number end asc  nulls last,
  case when @sort::text = 'folio'  and @dir::text <> 'asc' then o.daily_number end desc nulls last,
  case when @sort::text = 'estado' and @dir::text = 'asc'  then o.status::text end asc  nulls last,
  case when @sort::text = 'estado' and @dir::text <> 'asc' then o.status::text end desc nulls last,
  case when @sort::text = 'tipo'   and @dir::text = 'asc'  then o.service_type::text end asc  nulls last,
  case when @sort::text = 'tipo'   and @dir::text <> 'asc' then o.service_type::text end desc nulls last,
  case when @sort::text = 'fecha'  and @dir::text = 'asc'  then o.opened_at end asc,
  o.opened_at desc, o.id desc
limit sqlc.arg('lim') offset sqlc.arg('off');

-- name: CountSales :one
-- El mismo `where` que ListSales, palabra por palabra. Es el total del filtro para el paginador.
select count(*) from orders o
where o.business_date between @desde and @hasta
  and (sqlc.narg('status')::order_status is null or o.status = sqlc.narg('status'))
  and (sqlc.narg('service_type')::service_type is null or o.service_type = sqlc.narg('service_type'));

-- name: SalesTotalsByStatus :many
-- Agrupa POR ESTADO y deja que el dominio decida qué cuenta como ingreso. Postgres hace la suma
-- pesada —que es la que puede usar un índice— y en Go se queda la regla de clasificación, que es lo
-- que tiene que poder probarse sin base de datos.
--
-- No lleva el filtro de estado: el resumen tiene que poder decir cuánto se canceló aunque la tabla
-- esté filtrada a entregadas. Filtrar aquí haría que el tile de cancelaciones marcara cero justo
-- cuando se está buscando una. El de TIPO DE VENTA sí lo lleva, y en las dos ramas: con una
-- subconsulta que solo filtraba por fecha, la propina sumaba la de todos los tipos y salía inflada
-- junto a cifras correctas — que es peor que un número mal parejo, porque invita a confiar en el resto.
--
-- Las propinas se pre-agregan por order_id antes de unirse. Sin eso, una venta con dos pagos
-- duplicaría su total: order_payments es 1:N con orders.
with filtrado as (
  select o.id, o.status, o.total, o.delivery_fee
  from orders o
  where o.business_date between @desde and @hasta
    and (sqlc.narg('service_type')::service_type is null or o.service_type = sqlc.narg('service_type'))
), propinas as (
  select op.order_id, sum(op.tip_amount) as tip_amount
  from order_payments op
  join filtrado f on f.id = op.order_id
  group by op.order_id
)
select f.status,
       count(*)::int as ventas,
       coalesce(sum(f.total), 0)::numeric(12,2) as total,
       coalesce(sum(f.delivery_fee), 0)::numeric(12,2) as envios,
       coalesce(sum(p.tip_amount), 0)::numeric(12,2) as propinas
from filtrado f
left join propinas p on p.order_id = f.id
group by f.status;

-- name: SalesTotalsByMethod :many
-- Desglose por medio de pago: lo COBRADO, que no es lo mismo que lo vendido (una venta mandada a
-- cocina sin cobrar suma al total y no aparece aquí). Sale de order_payments porque una venta puede
-- pagarse con varios métodos.
--
-- El filtro de ESTADO DE LA PANTALLA no aplica, por el mismo motivo que SalesTotalsByStatus: el
-- resumen dice cuánto entró por cada medio aunque la tabla esté filtrada a un estado. El de tipo de
-- venta sí aplica.
--
-- Lo que SÍ se excluye son canceladas y reembolsadas, que no son un filtro de la pantalla sino la
-- misma regla que aplica el total de arriba. Sin ellas, los $500 de una venta devuelta salían en el
-- tile "Reembolsadas" Y en el de "Tarjeta" mientras el total —que sí las excluye— los ignoraba: el
-- mismo peso contado de tres maneras en tres renglones hermanos. Reconciliar contra la terminal
-- bancaria es trabajo del corte de caja, que es por turno y sí mira el flujo bruto; esta pantalla
-- responde qué VENDIÓ el negocio.
--
-- El hermano de esta consulta vive en reports.sql (`SalesByMethod`) y se corrigió primero; esta
-- copia se quedó con el defecto una versión entera. Se editan juntas.
select pm.id as method_id, pm.name as method,
       count(*)::int as pagos,
       coalesce(sum(op.amount), 0)::numeric(12,2) as total,
       coalesce(sum(op.tip_amount), 0)::numeric(12,2) as propinas
from order_payments op
join orders o on o.id = op.order_id
join payment_methods pm on pm.id = op.payment_method_id
where o.status not in ('cancelada', 'reembolsada')
  and o.business_date between @desde and @hasta
  and (sqlc.narg('service_type')::service_type is null or o.service_type = sqlc.narg('service_type'))
group by pm.id, pm.name
order by total desc;

-- name: SalesCancelledLines :one
-- Líneas canceladas dentro de ventas que NO se cancelaron enteras: es la merma que se pierde de
-- vista, porque el pedido se cobró y el renglón no.
select count(*)::int as lineas,
       coalesce(sum(ol.line_total), 0)::numeric(12,2) as monto
from order_lines ol
join orders o on o.id = ol.order_id
where o.status not in ('cancelada', 'reembolsada')
  and o.business_date between @desde and @hasta
  and ol.cancelled_at is not null
  -- El mismo filtro de tipo que el resto del resumen: sin él, filtrar la pantalla a domicilio
  -- seguía mostrando la merma de mostrador y las cifras dejaban de ser del mismo conjunto.
  and (sqlc.narg('service_type')::service_type is null or o.service_type = sqlc.narg('service_type'));
