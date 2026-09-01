-- Pricing (autoritativo en el servidor)

-- name: GetPricedProducts :many
select id, name, price, current_cost, is_active, needs_prep
from products where id = any($1::bigint[]);

-- name: GetPricedOptions :many
-- max_per_line viaja porque el servidor lo valida: es el tope de veces que una opción puede ir en
-- la misma línea, y desde que la pantalla deja pedir dos salsas del mismo sabor deja de ser un
-- valor que nadie ejercía.
select mo.id, mo.name, mo.price_delta, mo.current_cost, mo.max_per_line, mg.name as group_title
from modifier_options mo
join modifier_groups mg on mg.id = mo.group_id
where mo.id = any($1::bigint[]);

-- Creación

-- name: NextDailyNumber :one
-- company_id lo auto-sella el default (GUC del tenant); el folio diario es por-empresa. Se
-- arbitra por NOMBRE de la PK compuesta (company_id, business_date): referir la columna por
-- nombre haría fallar a sqlc, que no ve las columnas agregadas dinámicamente en la migración.
insert into order_counters (business_date, last_number)
values ($1, 1)
on conflict on constraint order_counters_pkey do update set last_number = order_counters.last_number + 1
returning last_number;

-- name: GetOrderIDByClientUUID :one
select id from orders where client_uuid = $1;

-- name: CreateOrder :one
-- status y completed_at los decide quien llama: un pedido que se cobra y se entrega en el mismo
-- acto —el refresco de mostrador— nace entregado y nunca pasa por el tablero. El resto nace abierto.
insert into orders (client_uuid, business_date, daily_number, service_type, delivery_platform_id,
                    customer_name, notes, register_session_id, opened_by, subtotal, total, delivery_fee,
                    folio_name, status, completed_at)
values ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,@folio_name,
        @status, case when @status::order_status = 'entregada' then now() end)
returning *;

-- name: CreateOrderLine :one
insert into order_lines (order_id, product_id, product_name, quantity, unit_price,
                         modifiers_total, unit_cost, line_total, notes, delivered_qty)
values ($1,$2,$3,$4,$5,$6,$7,$8,$9,
        case when sqlc.arg(nace_entregada)::boolean then $4::numeric else 0::numeric end)
returning id;

-- name: CreateOrderLineModifier :exec
insert into order_line_modifiers (order_line_id, modifier_option_id, group_title, option_name,
                                  quantity, price_delta, unit_cost)
values ($1,$2,$3,$4,$5,$6,$7);

-- name: CreateOrderPayment :exec
insert into order_payments (order_id, payment_method_id, amount, tip_amount, register_session_id, received_by, reference)
values ($1,$2,$3,$4,$5,$6,$7);

-- Board / detalle

-- name: ListActiveOrders :many
select o.id, o.daily_number, o.folio_name, o.status, o.service_type, o.delivery_platform_id,
       o.customer_name, o.total, o.currency,
       o.opened_at, o.ready_at,
       coalesce((select sum(amount) from order_payments p where p.order_id = o.id), 0)::numeric(10,2) as paid,
       (select count(*) from order_lines l
         where l.order_id = o.id and l.cancelled_at is null)::int as lineas_vivas,
       (select count(*) from order_lines l
         where l.order_id = o.id and l.cancelled_at is null and l.delivered_qty >= l.quantity)::int as lineas_entregadas
from orders o
where o.status in ('abierta','lista')
order by o.opened_at;

-- name: GetOrder :one
select * from orders where id = $1;

-- name: ListOrderLines :many
select id, product_id, product_name, quantity, unit_price, modifiers_total, line_total, notes,
       delivered_qty, cancelled_at
from order_lines where order_id = $1 order by id;

-- name: ListOrderLineModifiers :many
select olm.order_line_id, olm.group_title, olm.option_name, olm.quantity, olm.price_delta
from order_line_modifiers olm
join order_lines ol on ol.id = olm.order_line_id
where ol.order_id = $1;

-- name: ListOrderPayments :many
select id, payment_method_id, amount, tip_amount, created_at from order_payments where order_id = $1;

-- name: RecentModifierPicks :many
-- Histórico de opciones elegidas por producto/grupo, para defaults contextuales.
-- Excluye canceladas y limita la ventana (el decaimiento por recencia hace irrelevante lo viejo).
select ol.product_id, mo.group_id, olm.modifier_option_id::bigint as option_id, ol.created_at
from order_line_modifiers olm
join order_lines ol on ol.id = olm.order_line_id
join modifier_options mo on mo.id = olm.modifier_option_id
join orders o on o.id = ol.order_id
where o.status <> 'cancelada'
  and ol.created_at >= now() - interval '90 days';

-- name: SetOrderStatus :exec
update orders set status = $2,
  ready_at = case when $2 = 'lista'::order_status then now() else ready_at end,
  completed_at = case when $2 = 'entregada'::order_status then now() else completed_at end
where id = $1;

-- name: ListDeliveredToday :many
-- Órdenes entregadas del día (para la sección de reembolsos del tablero). Acotada a la
-- fecha de negocio para no arrastrar todo el histórico.
select o.id, o.daily_number, o.folio_name, o.status, o.service_type, o.delivery_platform_id,
       o.customer_name, o.total, o.currency,
       o.opened_at, o.ready_at,
       coalesce((select sum(amount) from order_payments p where p.order_id = o.id), 0)::numeric(10,2) as paid,
       (select count(*) from order_lines l
         where l.order_id = o.id and l.cancelled_at is null)::int as lineas_vivas,
       (select count(*) from order_lines l
         where l.order_id = o.id and l.cancelled_at is null and l.delivered_qty >= l.quantity)::int as lineas_entregadas
from orders o
where o.status = 'entregada' and o.business_date = $1
order by o.completed_at desc nulls last, o.id desc;

-- name: RefundOrder :exec
-- Devolución de una orden entregada: la marca 'reembolsada' (pérdida). Sin restock.
update orders set status = 'reembolsada', refunded_at = now(),
  refunded_by = $2, refund_reason = $3, refund_amount = $4
where id = $1;

-- name: CancelOrder :exec
update orders set status = 'cancelada', cancelled_at = now(), cancelled_by = $2, cancel_reason = $3
where id = $1;

-- name: RestockCancelledOrder :exec
-- Repone el stock de una orden cancelada: movimientos 'cancelacion' que invierten las ventas.
insert into stock_movements (item_type, ingredient_id, product_id, movement_type, quantity, order_id, user_id, reason)
select sm.item_type, sm.ingredient_id, sm.product_id, 'cancelacion', -sm.quantity, sm.order_id, sqlc.arg(actor_id), 'cancelación de orden'
from stock_movements sm where sm.order_id = sqlc.arg(oid) and sm.movement_type = 'venta';

-- name: RecalcOrderTotals :exec
-- Recalcula el total del pedido desde SUS renglones, después de agregarle más.
--
-- Se suma en la base y no en Go a propósito: los renglones ya guardados son la verdad, y volver a
-- calcularlos desde el comando obligaría a traerlos, re-precisarlos con la lista de precios de HOY
-- —que puede haber cambiado— y reescribirlos. Un pedido de ayer cambiaría de precio por agregarle
-- un café.
--
-- El envío no se toca: se decidió al crear el pedido y agregar renglones no lo cambia.
update orders o
set subtotal = coalesce((select sum(ol.line_total) from order_lines ol
                          where ol.order_id = o.id and ol.cancelled_at is null), 0),
    total    = coalesce((select sum(ol.line_total) from order_lines ol
                          where ol.order_id = o.id and ol.cancelled_at is null), 0) + o.delivery_fee,
    updated_at = now()
where o.id = $1;

-- name: GetOrderForUpdate :one
-- El pedido al que se le va a agregar, bloqueado dentro de la transacción: dos meseros agregando a
-- la misma cuenta al mismo tiempo recalcularían el total sobre el estado viejo y uno de los dos
-- agregados desaparecería del importe.
select id, status, service_type, delivery_platform_id, total
from orders where id = $1
for update;

-- name: ListLinesForDelivery :many
-- Lo mínimo para razonar sobre la entrega de un pedido: ni precio ni producto, porque entregar no
-- mueve dinero. `for update` porque de esto cuelga el cierre automático del pedido, y dos personas
-- marcando renglones a la vez podrían dejarlo abierto con todo entregado.
select id, quantity, delivered_qty, cancelled_at
from order_lines
where order_id = $1
order by id
for update;

-- name: DeliverOrderLine :execrows
-- Suma a lo ya entregado de un renglón. El tope contra `quantity` lo repite aquí la base aunque el
-- dominio ya lo validó: entre validar y escribir cabe otra transacción entregando lo mismo, y el
-- resultado sería un renglón con más entregado de lo que se pidió.
update order_lines
   set delivered_qty = delivered_qty + sqlc.arg(cantidad)::numeric
 where id = sqlc.arg(line_id)
   and order_id = sqlc.arg(order_id)
   and cancelled_at is null
   and delivered_qty + sqlc.arg(cantidad)::numeric <= quantity;

-- name: DeliverAllOrderLines :exec
-- "Entregar todo": el camino de un tap, que es el caso común. Lo cancelado se queda como está.
update order_lines
   set delivered_qty = quantity
 where order_id = $1 and cancelled_at is null;

-- name: CountLinesPendingDelivery :one
-- Cuántos productos vivos le faltan al pedido. Alimenta la guardia del cierre de caja y el resumen
-- del tablero sin traerse los renglones.
select count(*) from order_lines
where order_id = $1 and cancelled_at is null and delivered_qty < quantity;

-- name: FolioNamesUsedToday :many
-- Los nombres ya repartidos hoy, para no repetir uno cuando la pantalla propone el suyo.
--
-- Se lee dentro de la MISMA transacción que toma NextDailyNumber, y eso es lo que la hace segura:
-- ese insert bloquea la fila del contador del día hasta el commit, así que dos ventas de la misma
-- empresa y fecha no pueden estar aquí a la vez. Sin ese lock haría falta uno propio.
select folio_name from orders
where business_date = $1 and folio_name is not null;

-- name: ListLinesOfActiveOrders :many
-- Los renglones de TODOS los pedidos del tablero, en una consulta.
--
-- El tablero los muestra desplegados —lo que falta por entregar es lo que el operador vino a leer,
-- no algo que deba destapar con un tap—, y pedirlos pedido por pedido serían N peticiones en cada
-- refresco de una pantalla que se refresca sola cada diez segundos.
select l.id, l.order_id, l.product_name, l.quantity, l.delivered_qty, l.notes
from order_lines l
join orders o on o.id = l.order_id
where o.status in ('abierta','lista') and l.cancelled_at is null
order by l.order_id, l.id;

-- name: ListModifiersOfActiveOrders :many
-- Los modificadores de esos renglones. En una cocina "Alitas" y "Alitas BBQ" son platillos
-- distintos, así que sin esto el tablero no alcanza a reemplazar la libreta.
select olm.order_line_id, olm.option_name, olm.quantity
from order_line_modifiers olm
join order_lines l on l.id = olm.order_line_id
join orders o on o.id = l.order_id
where o.status in ('abierta','lista') and l.cancelled_at is null
order by olm.order_line_id, olm.id;

-- name: SumOrderPayments :one
-- Lo ya cobrado de un pedido. Se lee dentro de la tx del cobro y con el pedido bloqueado, que es
-- lo que impide que dos cajeros cobrando a la vez registren cada uno el total completo.
select coalesce(sum(amount), 0)::numeric(10,2) from order_payments where order_id = $1;

-- name: ListOpenOrders :many
-- Los pedidos que el punto de venta tiene que seguir viendo: la barra de pedidos en curso.
--
-- Es la UNIÓN de dos conjuntos que no son el mismo, y confundirlos ya costó una vez:
--
--   * en preparación — `abierta` o `lista`: se les puede AGREGAR y cobrar. Es al que el cliente le
--     pide algo más, y el que antes desaparecía de la pantalla al mandarlo a cocina.
--   * con saldo — debe dinero y no está cancelada ni reembolsada. Incluye el pedido ENTREGADO y sin
--     cobrar, que es el caro: el cliente ya se fue. Esa es la razón de ser de la píldora que esta
--     lista reemplaza, y quedarse solo con "en preparación" lo habría borrado del encabezado.
--
-- `en_preparacion` viaja como dato y no se deduce del estado en el front: la pantalla tiene que
-- poder decir cuál se puede ampliar sin volver a implementar la regla.
--
-- Cancelada y reembolsada quedan fuera siempre: su dinero ya se decidió, y listarlas mandaría al
-- operador a perseguir cobros que nadie debe.
select o.id, o.daily_number, o.folio_name, o.status, o.service_type, o.delivery_platform_id,
       o.customer_name, o.total, o.currency, o.opened_at,
       coalesce((select sum(amount) from order_payments p where p.order_id = o.id), 0)::numeric(10,2) as paid,
       (o.status in ('abierta', 'lista'))::boolean as en_preparacion,
       (select count(*) from order_lines l where l.order_id = o.id and l.cancelled_at is null)::int as renglones
from orders o
where o.business_date = $1
  and o.status not in ('cancelada', 'reembolsada')
  and (
    o.status in ('abierta', 'lista')
    or coalesce((select sum(amount) from order_payments p where p.order_id = o.id), 0) < o.total
  )
order by o.opened_at;

-- name: RenglonesSinEnviarACocina :many
-- Los renglones de un pedido que todavía no han salido en ninguna comanda.
--
-- Es lo que hace posible imprimir SOLO lo agregado, y recuperar una impresión que falló sin volver
-- a sacar el pedido entero — que haría que cocina prepare dos veces lo mismo.
--
-- El cancelado queda fuera: mandarlo sería pedirle a cocina que prepare algo que el cliente quitó.
select l.id, l.product_name, l.quantity, l.notes
from order_lines l
where l.order_id = $1 and l.enviado_a_cocina_at is null and l.cancelled_at is null
order by l.id;

-- name: MarcarRenglonesEnviadosACocina :exec
-- Marca como salidos en comanda los renglones dados de un pedido.
--
-- Acotado por `order_id` además de por los ids: los ids vienen del servicio, pero un filtro que solo
-- mira la lista deja la puerta abierta a marcar renglones de otro pedido si algún día esa lista se
-- arma desde otro lado. Es el mismo predicado que ya usa el resto del archivo.
update order_lines set enviado_a_cocina_at = now()
where order_id = $1 and id = any($2::bigint[]);
