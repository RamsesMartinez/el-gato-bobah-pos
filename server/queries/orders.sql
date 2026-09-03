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
insert into order_payments (order_id, payment_method_id, amount, tip_amount, register_session_id, received_by, reference, client_uuid)
values ($1,$2,$3,$4,$5,$6,$7,$8);

-- name: GetOrderPaymentByClientUUID :one
-- ¿Este cobro exacto ya entró, y sobre qué pedido? Es la red que el cobro dividido necesita: dos
-- mitades iguales son indistinguibles entre sí, así que sin la llave del cliente el reenvío de la
-- primera pasa todas las validaciones y deja el pedido saldado con una sola mitad cobrada.
--
-- Devuelve la CARGA del pago, no un booleano. Un no-op solo es inocuo si la llamada es idéntica:
-- si el pago entró y su respuesta se perdió, el operador puede cambiar el método —"la terminal no
-- jaló, me paga en efectivo"— y volver a tocar. Dando eso por reintento, la pantalla canta cobrado,
-- el operador mete los billetes al cajón, y el corte cierra esperando la tarjeta que nunca llegó y
-- sin esperar el efectivo que sí está. Descuadre en los dos métodos a la vez.
--
-- Sin filtro de empresa: RLS lo agrega, y el índice único que respalda esto va por
-- (company_id, client_uuid).
select id, order_id, payment_method_id, amount, tip_amount
from order_payments where client_uuid = $1;

-- Board / detalle

-- name: ListActiveOrders :many
select o.id, o.daily_number, o.folio_name, o.status, o.service_type, o.delivery_platform_id,
       o.customer_name, o.total, o.currency, o.refund_amount,
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
       o.customer_name, o.total, o.currency, o.refund_amount,
       o.opened_at, o.ready_at,
       coalesce((select sum(amount) from order_payments p where p.order_id = o.id), 0)::numeric(10,2) as paid,
       (select count(*) from order_lines l
         where l.order_id = o.id and l.cancelled_at is null)::int as lineas_vivas,
       (select count(*) from order_lines l
         where l.order_id = o.id and l.cancelled_at is null and l.delivered_qty >= l.quantity)::int as lineas_entregadas
from orders o
-- Desde un INSTANTE, y mirando cuándo se COMPLETÓ.
--
-- Filtraba `business_date = <hoy según el reloj del servidor>`, que corre en UTC: en México la
-- medianoche UTC cae a las 18:00 locales, así que la lista se vaciaba a media hora pico con los
-- pedidos del día todavía frescos. Ahora el instante lo decide el negocio.
--
-- Y por `completed_at`, no por `opened_at`: lo que decide si un entregado pertenece a la ventana
-- visible es cuándo se completó. Un pedido levantado a las 23:50 y entregado a las 00:05 quedaba
-- fuera justo cuando se vuelve accionable — recién entregado y candidato a reembolso. El `order by`
-- de esta misma consulta ya usaba `completed_at`: el filtro miraba una columna y el orden otra.
where o.status = 'entregada' and o.completed_at >= $1
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
select l.id, l.order_id, l.product_name, l.quantity, l.delivered_qty, l.notes,
       (l.enviado_a_cocina_at is not null)::boolean as enviado_a_cocina
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
-- Lo ya cobrado de un pedido, y la propina ya registrada. Se lee dentro de la tx del cobro y con el
-- pedido bloqueado, que es lo que impide que dos cajeros cobrando a la vez registren cada uno el
-- total completo.
--
-- La propina viaja en la misma pasada porque su tope es ACUMULADO: se topa cada cobro por separado
-- y tres pagos de $250 de propina sobre una cuenta de $250 pasan los tres, con el cajón esperando
-- $750 que nadie dejó.
select coalesce(sum(amount), 0)::numeric(10,2) as pagado,
       coalesce(sum(tip_amount), 0)::numeric(10,2) as propina
from order_payments where order_id = $1;

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
-- Lo pagado se calcula UNA vez, con un lateral, y se reusa en el select y en el where. Escrito
-- como dos subconsultas iguales, Postgres no las deduplica: en el plan real salían dos SubPlan y el
-- mismo agregado se recorría dos veces por cada pedido entregado sin cobrar.
select o.id, o.daily_number, o.folio_name, o.status, o.service_type, o.delivery_platform_id,
       o.customer_name, o.total, o.currency, o.opened_at, o.business_date,
       pagos.paid::numeric(10,2) as paid,
       (o.status in ('abierta', 'lista'))::boolean as en_preparacion,
       (select count(*) from order_lines l where l.order_id = o.id and l.cancelled_at is null)::int as renglones
from orders o
left join lateral (
  select coalesce(sum(p.amount), 0) as paid from order_payments p where p.order_id = o.id
) pagos on true
where o.status not in ('cancelada', 'reembolsada')
  -- Redundante a propósito, y no se puede quitar. El OR de abajo referencia `pagos.paid`, que sale
  -- del lateral, así que Postgres no lo puede empujar al scan de `orders`: calculaba los pagos de
  -- CADA pedido histórico no cancelado antes de descartarlo. Medido con 30 mil pedidos: 175 ms y
  -- 90 mil buffers, en una consulta que cada tableta pide cada 30 segundos. Este predicado dice lo
  -- mismo pero sin tocar el lateral, y baja a 20 ms y 155 buffers usando los índices que ya hay.
  and (o.status in ('abierta', 'lista') or o.business_date = $1)
  and (
    -- SIN filtro de fecha en los que siguen en curso, a propósito: un pedido abierto se ve hasta que
    -- alguien lo cierre, sin importar de qué día sea. Es el mecanismo con el que se limpia el
    -- rezago — un pedido que nadie ve es un pedido que nadie cierra, y así había once desde julio.
    o.status in ('abierta', 'lista')
    -- Los que deben dinero sí se acotan al día en curso: el pendiente de hace tres meses ya no es
    -- algo que el cajero de hoy pueda cobrar, y traerlos convertiría la barra en un histórico.
    --
    -- El centavo de tolerancia es el MISMO de `domain.PedidoSaldado`, y tiene que moverse con él.
    -- Escrito como `pagos.paid < o.total` a secas, dividir $100 en tres partes de $33.33 cerraba el
    -- pedido —con el predicado tolerante— y esta consulta lo seguía listando con $0.01 de deuda que
    -- nadie podía cobrar. Lo cubre TestUnPedidoCerradoNoDejaCentavosDeDeuda, que pasa por los dos.
    or (o.total - pagos.paid > 0.01 and o.business_date = $1)
  )
order by o.opened_at;

-- name: MarcarTodoElPedidoEnviadoACocina :exec
-- Marca como salidos en comanda TODOS los renglones vivos de un pedido.
--
-- Lo usa el CONFIRMAR: ahí sale la comanda del pedido completo, así que ningún renglón queda
-- pendiente. Sin esto, el primer agregado sacaría una comanda con el pedido entero y cocina
-- prepararía dos veces lo que ya tenía en la plancha.
update order_lines set enviado_a_cocina_at = now()
where order_id = $1 and enviado_a_cocina_at is null and cancelled_at is null;

-- name: MarcarRenglonesEnviadosACocina :exec
-- Marca como salidos en comanda los renglones dados de un pedido.
--
-- Acotado por `order_id` además de por los ids: los ids vienen del servicio, pero un filtro que solo
-- mira la lista deja la puerta abierta a marcar renglones de otro pedido si algún día esa lista se
-- arma desde otro lado. Es el mismo predicado que ya usa el resto del archivo.
update order_lines set enviado_a_cocina_at = now()
where order_id = @order_id and id = any(@ids::bigint[]);

-- name: PedidoNecesitaPreparacion :one
-- Si a este pedido le queda algo que cocina tenga que preparar.
--
-- Lo usa el cobro para cerrar en el acto el pedido que no pasa por cocina y quedó saldado —una
-- embotellada en el mostrador—, que antes nacía entregado porque crear y cobrar eran una sola
-- llamada. Al separarlos, ese pedido se quedaba abierto para siempre en la barra y el operador
-- tenía que entregarlo a mano: un toque por cada refresco, en la venta más frecuente del día.
select exists (
  select 1 from order_lines l
  join products p on p.id = l.product_id
  where l.order_id = $1 and l.cancelled_at is null and p.needs_prep
)::boolean;

-- name: SumOrderPaymentsByMethod :many
-- Cuánto entró por CADA medio de pago en un pedido, en el orden en que entró.
--
-- Es lo que decide de dónde sale cada peso al devolver: el dinero sale por donde entró. Devolver en
-- efectivo lo que entró por tarjeta saca del cajón dinero que nunca estuvo ahí, y el arqueo cierra
-- con un faltante inventado.
--
-- `is_active` viaja pero NO filtra: por un método desactivado ya no debe ENTRAR dinero, pero el que
-- entró tiene que poder salir por donde entró, o queda atrapado.
select pm.id as method_id, pm.name, pm.kind = 'efectivo' as es_efectivo, pm.is_active,
       coalesce(sum(op.amount), 0)::numeric(10,2) as cobrado
from order_payments op
join payment_methods pm on pm.id = op.payment_method_id
where op.order_id = $1
group by pm.id, pm.name, pm.kind, pm.is_active
order by min(op.created_at);

-- name: SumOrderRefunds :one
-- Lo ya devuelto de un pedido, y de UNO de sus renglones.
--
-- Las dos cifras en una pasada porque el tope de un renglón es lo cobrado de ESE renglón: sin esa
-- cota, devolver tres veces un platillo de $60 en un pedido de $500 pasa sin que nada lo frene.
select coalesce(sum(amount), 0)::numeric(10,2) as devuelto_total,
       coalesce(sum(amount) filter (where order_line_id = sqlc.narg('line_id')), 0)::numeric(10,2) as devuelto_del_renglon
from order_refunds where order_id = sqlc.arg('order_id');

-- name: InsertOrderRefund :one
insert into order_refunds (order_id, order_line_id, payment_method_id, amount, reason, refunded_by, cash_movement_id)
values ($1, $2, $3, $4, $5, $6, $7)
returning id;

-- name: RecalcOrderRefundAmount :exec
-- `orders.refund_amount` pasa a ser la SUMA del libro, no un número que se escribe aparte.
--
-- Se conserva la columna porque `RefundsByDay` ya la lee, y dos verdades sobre el mismo dinero es
-- exactamente lo que el principio III prohíbe. Recalcularla desde el libro es lo que las mantiene
-- siendo una sola.
update orders o
set refund_amount = coalesce((select sum(r.amount) from order_refunds r where r.order_id = o.id), 0)
where o.id = $1;

-- name: GetOrderLineForCancel :one
-- El renglón y el estado de su pedido, para decidir si se puede cancelar y si repone inventario.
--
-- `for update of ol`: dos cajeros cancelando el mismo renglón a la vez lo cancelarían dos veces y
-- repondrían el insumo dos veces. Solo el renglón, no el pedido: bloquear el pedido entero pararía
-- al que está cobrando en la otra tableta.
select ol.id, ol.order_id, ol.quantity, ol.delivered_qty, ol.cancelled_at, ol.enviado_a_cocina_at,
       o.status as order_status
from order_lines ol
join orders o on o.id = ol.order_id
where ol.id = $1 and ol.order_id = $2
for update of ol;

-- name: CancelOrderLine :exec
-- Marca el renglón, no lo borra: el histórico de qué se pidió y se canceló es lo que deja explicar
-- una merma más adelante. `RecalcOrderTotals` ya excluye los cancelados del total del pedido.
update order_lines set cancelled_at = now(), cancelled_by = $2, cancel_reason = $3
where id = $1 and cancelled_at is null;

-- name: RestockCancelledLine :exec
-- Repone el insumo de UN renglón, revirtiendo los movimientos que de verdad salieron por él.
--
-- Revierte lo registrado y no un recálculo con la receta de HOY: una receta que cambió entre la
-- venta y la cancelación repondría una cantidad distinta de la que se descontó.
--
-- Un renglón anterior a la migración 0060 no tiene movimientos ligados y no repone nada. Es la
-- decisión: de un movimiento viejo no consta a qué renglón pertenecía, y adivinarlo inventaría
-- existencias.
insert into stock_movements (item_type, ingredient_id, product_id, movement_type, quantity, order_id, order_line_id, user_id, reason)
select sm.item_type, sm.ingredient_id, sm.product_id, 'cancelacion', -sm.quantity, sm.order_id, sm.order_line_id,
       sqlc.arg(actor_id), 'cancelación de renglón'
from stock_movements sm
where sm.order_line_id = sqlc.arg(line_id) and sm.movement_type = 'venta';
