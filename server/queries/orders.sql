-- Pricing (autoritativo en el servidor)

-- name: GetPricedProducts :many
select id, name, price, current_cost, is_active
from products where id = any($1::bigint[]);

-- name: GetPricedOptions :many
select mo.id, mo.name, mo.price_delta, mo.current_cost, mg.name as group_title
from modifier_options mo
join modifier_groups mg on mg.id = mo.group_id
where mo.id = any($1::bigint[]);

-- Creación

-- name: NextDailyNumber :one
insert into order_counters (business_date, last_number)
values ($1, 1)
on conflict (business_date) do update set last_number = order_counters.last_number + 1
returning last_number;

-- name: GetOrderIDByClientUUID :one
select id from orders where client_uuid = $1;

-- name: CreateOrder :one
insert into orders (client_uuid, business_date, daily_number, service_type, delivery_platform_id,
                    customer_name, notes, register_session_id, opened_by, subtotal, total)
values ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
returning *;

-- name: CreateOrderLine :one
insert into order_lines (order_id, product_id, product_name, quantity, unit_price,
                         modifiers_total, unit_cost, line_total, notes)
values ($1,$2,$3,$4,$5,$6,$7,$8,$9)
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
select o.id, o.daily_number, o.status, o.service_type, o.customer_name, o.total, o.currency,
       o.opened_at, o.ready_at,
       coalesce((select sum(amount) from order_payments p where p.order_id = o.id), 0)::numeric(10,2) as paid
from orders o
where o.status in ('abierta','lista')
order by o.opened_at;

-- name: GetOrder :one
select * from orders where id = $1;

-- name: ListOrderLines :many
select id, product_id, product_name, quantity, unit_price, modifiers_total, line_total, notes
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

-- name: CancelOrder :exec
update orders set status = 'cancelada', cancelled_at = now(), cancelled_by = $2, cancel_reason = $3
where id = $1;

-- name: RestockCancelledOrder :exec
-- Repone el stock de una orden cancelada: movimientos 'cancelacion' que invierten las ventas.
insert into stock_movements (item_type, ingredient_id, product_id, movement_type, quantity, order_id, user_id, reason)
select sm.item_type, sm.ingredient_id, sm.product_id, 'cancelacion', -sm.quantity, sm.order_id, sqlc.arg(actor_id), 'cancelación de orden'
from stock_movements sm where sm.order_id = sqlc.arg(oid) and sm.movement_type = 'venta';
