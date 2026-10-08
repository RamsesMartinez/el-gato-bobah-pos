-- Pantalla de Ventas (análisis). Distinta del tablero de pedidos: aquí se mira lo que YA pasó.
--
-- Las cinco consultas comparten el mismo `where` y viven juntas A PROPÓSITO: si el filtro de la
-- lista y el del resumen divergen, las cifras de arriba dejan de cuadrar con la tabla de abajo y
-- nadie sabe cuál de las dos miente. Se editan en la misma pasada.
--
-- SON CINCO PARES, NO CINCO. Cada una tiene una gemela `…SinFolio` idéntica salvo por UNA línea: el
-- predicado literal `delivery_platform_id is not null and platform_order_ref is null` del filtro de
-- pendientes. La gemela existe y no un parámetro porque un `sqlc.narg(...) is null or (…)` NO usa el
-- índice parcial `orders_plataforma_sin_folio`: medido contra 150k pedidos, con plan genérico
-- —al que pgx cae solo, porque usa statements con nombre— Postgres no puede probar que el predicado
-- del índice se cumple y cae a un bitmap scan sobre la fecha, incluso con el parámetro en `true`.
--
-- El precio de esto es duplicación, y el riesgo es que un par se desincronice. Cada gemela va
-- INMEDIATAMENTE DESPUÉS de su original para que cualquier diff las muestre juntas, y
-- `TestLaListaYElResumenDescribenElMismoConjunto` falla si divergen.
--
-- Ninguna filtra por company_id, y no es un olvido: RLS agrega ese predicado a toda consulta del rol
-- `gatobobah_app`. Además sqlc NO conoce la columna —la migración 0023 la agregó con SQL dinámico
-- que su parser no puede leer—, así que nombrarla aquí rompería `sqlc generate` por una columna que
-- sí existe en Postgres.
--
-- EL PEDIDO JUNTADO (spec 027) NO ES UNA VENTA NI UNA CANCELACIÓN. Al pasarle todos sus productos a
-- otro pedido queda `cancelada` con `merged_into_order_id`, y sus productos se cuentan en el pedido
-- con el que se juntó. Lista, conteo y resumen lo excluyen con la misma línea
-- (`o.merged_into_order_id is null`); `SalesTotalsByMethod` ya lo deja fuera por estado.
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
       o.status, o.service_type, o.customer_name, o.total, o.discount_total, o.delivery_fee, o.refund_amount,
       o.platform_order_ref,
       dp.name as platform,
       u.name as opened_by_name,
       -- Quién aplicó el descuento. Sin esto el rastro solo se lee con un psql en la mano, y la
       -- decisión de no pedir rol para descontar se apoya justo en que el rastro sea consultable.
       coalesce(du.name, '') as discount_by_name,
       (select coalesce(sum(op.tip_amount), 0) from order_payments op where op.order_id = o.id)::numeric(10,2) as tips,
       (select string_agg(distinct pm.name, ' + ' order by pm.name)
          from order_payments op join payment_methods pm on pm.id = op.payment_method_id
         where op.order_id = o.id) as methods
from orders o
left join delivery_platforms dp on dp.id = o.delivery_platform_id
left join users u on u.id = o.opened_by
left join users du on du.id = o.discount_set_by
where o.business_date between @desde and @hasta
  and o.merged_into_order_id is null
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

-- name: ListSalesSinFolio :many
-- Gemela de ListSales con el predicado de pendientes LITERAL. Ver la cabecera del archivo: esa
-- línea es lo único que las distingue, y se editan juntas.
select o.id, o.daily_number, o.folio_name, o.business_date, o.opened_at, o.completed_at,
       o.status, o.service_type, o.customer_name, o.total, o.discount_total, o.delivery_fee, o.refund_amount,
       o.platform_order_ref,
       dp.name as platform,
       u.name as opened_by_name,
       -- Quién aplicó el descuento. Sin esto el rastro solo se lee con un psql en la mano, y la
       -- decisión de no pedir rol para descontar se apoya justo en que el rastro sea consultable.
       coalesce(du.name, '') as discount_by_name,
       (select coalesce(sum(op.tip_amount), 0) from order_payments op where op.order_id = o.id)::numeric(10,2) as tips,
       (select string_agg(distinct pm.name, ' + ' order by pm.name)
          from order_payments op join payment_methods pm on pm.id = op.payment_method_id
         where op.order_id = o.id) as methods
from orders o
left join delivery_platforms dp on dp.id = o.delivery_platform_id
left join users u on u.id = o.opened_by
left join users du on du.id = o.discount_set_by
where o.delivery_platform_id is not null and o.platform_order_ref is null
  and o.business_date between @desde and @hasta
  and o.merged_into_order_id is null
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
  and o.merged_into_order_id is null
  and (sqlc.narg('status')::order_status is null or o.status = sqlc.narg('status'))
  and (sqlc.narg('service_type')::service_type is null or o.service_type = sqlc.narg('service_type'));

-- name: CountSalesSinFolio :one
-- Gemela de CountSales con el predicado de pendientes LITERAL. Ver la cabecera del archivo: esa
-- línea es lo único que las distingue, y se editan juntas.
select count(*) from orders o
where o.delivery_platform_id is not null and o.platform_order_ref is null
  and o.business_date between @desde and @hasta
  and o.merged_into_order_id is null
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
    and o.merged_into_order_id is null
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

-- name: SalesTotalsByStatusSinFolio :many
-- Gemela de SalesTotalsByStatus con el predicado de pendientes LITERAL. Ver la cabecera del archivo: esa
-- línea es lo único que las distingue, y se editan juntas.
with filtrado as (
  select o.id, o.status, o.total, o.delivery_fee
  from orders o
  where o.delivery_platform_id is not null and o.platform_order_ref is null
    and o.business_date between @desde and @hasta
    and o.merged_into_order_id is null
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
-- cocina sin cobrar suma al total y no aparece aquí).
--
-- Desde la spec 031 cada cobro cuenta el día en que se COBRÓ y cada devolución resta el día en que
-- se DEVOLVIÓ (decisión del dueño): es dinero, no venta, y así un mes cerrado no cambia porque un
-- pedido se cobre o se devuelva después. Es la misma regla que `SalesByMethod` de reports.sql —se
-- editan juntas— y la explicación completa de qué pedidos cuentan vive ahí.
--
-- El filtro de ESTADO DE LA PANTALLA no aplica, por el mismo motivo que SalesTotalsByStatus: el
-- resumen dice cuánto entró por cada medio aunque la tabla esté filtrada a un estado. El de tipo de
-- venta sí aplica, en las dos ramas.
with pagos as (
  select op.payment_method_id, count(*) as pagos, sum(op.amount) as cobrado, sum(op.tip_amount) as propinas
    from order_payments op
    join orders o on o.id = op.order_id
   where (op.business_date between @desde and @hasta
          or (op.business_date is null and o.business_date between @desde and @hasta))
     and (o.status not in ('cancelada', 'reembolsada')
          or exists (select 1 from order_refunds r where r.order_id = o.id))
     and (sqlc.narg('service_type')::service_type is null or o.service_type = sqlc.narg('service_type'))
   group by op.payment_method_id
), devueltos as (
  select r.payment_method_id, sum(r.amount) as devuelto, sum(r.tip_amount) as propina_devuelta
    from order_refunds r
    join orders o on o.id = r.order_id
   where coalesce(r.business_date, o.business_date) between @desde and @hasta
     and (sqlc.narg('service_type')::service_type is null or o.service_type = sqlc.narg('service_type'))
   group by r.payment_method_id
)
select pm.id as method_id, pm.name as method,
       coalesce(p.pagos, 0)::int as pagos,
       (coalesce(p.cobrado, 0) - coalesce(d.devuelto, 0))::numeric(12,2) as total,
       (coalesce(p.propinas, 0) - coalesce(d.propina_devuelta, 0))::numeric(12,2) as propinas,
       coalesce(d.devuelto, 0)::numeric(12,2) as devoluciones
from payment_methods pm
left join pagos p on p.payment_method_id = pm.id
left join devueltos d on d.payment_method_id = pm.id
where p.payment_method_id is not null or d.payment_method_id is not null
order by total desc;

-- name: SalesTotalsByMethodSinFolio :many
-- Gemela de SalesTotalsByMethod con el predicado de pendientes LITERAL, en las dos ramas. Ver la
-- cabecera del archivo: esa línea es lo único que las distingue, y se editan juntas.
with pagos as (
  select op.payment_method_id, count(*) as pagos, sum(op.amount) as cobrado, sum(op.tip_amount) as propinas
    from order_payments op
    join orders o on o.id = op.order_id
   where o.delivery_platform_id is not null and o.platform_order_ref is null
     and (op.business_date between @desde and @hasta
          or (op.business_date is null and o.business_date between @desde and @hasta))
     and (o.status not in ('cancelada', 'reembolsada')
          or exists (select 1 from order_refunds r where r.order_id = o.id))
     and (sqlc.narg('service_type')::service_type is null or o.service_type = sqlc.narg('service_type'))
   group by op.payment_method_id
), devueltos as (
  select r.payment_method_id, sum(r.amount) as devuelto, sum(r.tip_amount) as propina_devuelta
    from order_refunds r
    join orders o on o.id = r.order_id
   where o.delivery_platform_id is not null and o.platform_order_ref is null
     and coalesce(r.business_date, o.business_date) between @desde and @hasta
     and (sqlc.narg('service_type')::service_type is null or o.service_type = sqlc.narg('service_type'))
   group by r.payment_method_id
)
select pm.id as method_id, pm.name as method,
       coalesce(p.pagos, 0)::int as pagos,
       (coalesce(p.cobrado, 0) - coalesce(d.devuelto, 0))::numeric(12,2) as total,
       (coalesce(p.propinas, 0) - coalesce(d.propina_devuelta, 0))::numeric(12,2) as propinas,
       coalesce(d.devuelto, 0)::numeric(12,2) as devoluciones
from payment_methods pm
left join pagos p on p.payment_method_id = pm.id
left join devueltos d on d.payment_method_id = pm.id
where p.payment_method_id is not null or d.payment_method_id is not null
order by total desc;

-- name: SalesCancelledLines :one
-- Los renglones QUITADOS: la merma que se pierde de vista, porque el renglón no se cobró.
--
-- Cuentan aunque el pedido se haya cancelado después (spec 031, D14). Excluir los pedidos cancelados
-- borraba lo que se les quitó antes: tres frappés quitados y el pedido cerrado sin productos salían
-- en $0 en las dos cifras. No hay doble conteo con «Canceladas»: el total del pedido ya no incluye
-- lo quitado (RecalcOrderTotals), y cancelar el pedido no toca sus renglones.
select count(*)::int as lineas,
       coalesce(sum(ol.line_total), 0)::numeric(12,2) as monto
from order_lines ol
join orders o on o.id = ol.order_id
where o.business_date between @desde and @hasta
  and ol.cancelled_at is not null
  -- El mismo filtro de tipo que el resto del resumen: sin él, filtrar la pantalla a domicilio
  -- seguía mostrando la merma de mostrador y las cifras dejaban de ser del mismo conjunto.
  and (sqlc.narg('service_type')::service_type is null or o.service_type = sqlc.narg('service_type'));

-- name: SalesCancelledLinesSinFolio :one
-- Gemela de SalesCancelledLines con el predicado de pendientes LITERAL. Ver la cabecera del archivo: esa
-- línea es lo único que las distingue, y se editan juntas.
select count(*)::int as lineas,
       coalesce(sum(ol.line_total), 0)::numeric(12,2) as monto
from order_lines ol
join orders o on o.id = ol.order_id
where o.delivery_platform_id is not null and o.platform_order_ref is null
  and o.business_date between @desde and @hasta
  and ol.cancelled_at is not null
  -- El mismo filtro de tipo que el resto del resumen: sin él, filtrar la pantalla a domicilio
  -- seguía mostrando la merma de mostrador y las cifras dejaban de ser del mismo conjunto.
  and (sqlc.narg('service_type')::service_type is null or o.service_type = sqlc.narg('service_type'));


-- name: FindSaleByPlatformRef :many
-- Buscar un pedido pegando el folio que trae el documento de pago. IGUALDAD exacta, no parcial: el
-- caso de uso es pegar el identificador, y una búsqueda parcial sobre 64 caracteres devuelve varios
-- candidatos y obliga a comparar — que es justo lo que esta feature viene a eliminar.
--
-- Va como consulta PROPIA y no como un filtro más de las cinco de arriba por dos razones:
--
--   1. El plan. Con `col = $1` el planner SÍ puede probar el predicado del índice parcial
--      `orders_platform_ref_busqueda` (una igualdad implica `is not null` sea cual sea el valor);
--      metido dentro de un `narg is null or (…)`, no.
--   2. El resultado es UNA fila o ninguna, así que paginar y agregar por estado no significa nada.
--      El resumen de la pantalla se deriva de esta misma fila, y por eso la lista y el resumen no
--      pueden divergir: salen del mismo lugar.
--
-- Devuelve :many y no :one a propósito: "ese folio no está capturado" es una respuesta legítima —el
-- renglón del documento todavía no se registró— y no un 404.
--
-- Sin filtro de empresa: RLS lo agrega.
select o.id, o.daily_number, o.folio_name, o.business_date, o.opened_at, o.completed_at,
       o.status, o.service_type, o.customer_name, o.total, o.discount_total, o.delivery_fee, o.refund_amount,
       o.platform_order_ref,
       dp.name as platform,
       u.name as opened_by_name,
       -- Quién aplicó el descuento. Sin esto el rastro solo se lee con un psql en la mano, y la
       -- decisión de no pedir rol para descontar se apoya justo en que el rastro sea consultable.
       coalesce(du.name, '') as discount_by_name,
       (select coalesce(sum(op.tip_amount), 0) from order_payments op where op.order_id = o.id)::numeric(10,2) as tips,
       (select string_agg(distinct pm.name, ' + ' order by pm.name)
          from order_payments op join payment_methods pm on pm.id = op.payment_method_id
         where op.order_id = o.id) as methods
from orders o
left join delivery_platforms dp on dp.id = o.delivery_platform_id
left join users u on u.id = o.opened_by
left join users du on du.id = o.discount_set_by
where o.platform_order_ref = @folio
  and o.business_date between @desde and @hasta;
