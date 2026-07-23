-- name: SalesByDay :many
select o.business_date,
       count(*)::int as orders,
       coalesce(sum(o.total), 0)::numeric(12,2) as revenue
from orders o
where o.status not in ('cancelada', 'reembolsada') and o.business_date between $1 and $2
group by o.business_date
order by o.business_date;

-- name: SalesByMethod :many
select pm.name as method,
       count(*)::int as payments,
       coalesce(sum(op.amount), 0)::numeric(12,2) as total
from order_payments op
join payment_methods pm on pm.id = op.payment_method_id
where op.created_at >= $1
group by pm.name
order by total desc;

-- name: RefundsByDay :many
-- Pérdidas por devolución: órdenes entregadas que se reembolsaron (no cuentan como ingreso
-- en SalesByDay/ProductMargins; aquí se ven como la pérdida que son).
select o.business_date,
       count(*)::int as refunds,
       coalesce(sum(o.refund_amount), 0)::numeric(12,2) as amount
from orders o
where o.status = 'reembolsada' and o.business_date between $1 and $2
group by o.business_date
order by o.business_date;

-- name: TipsByEmployee :many
-- Propinas por empleado que cobró (received_by), para repartirlas. "Sin asignar" = pago sin cajero.
-- Propinas son pass-through (del personal), no ingreso del negocio; este reporte es para reparto.
select coalesce(u.name, 'Sin asignar') as employee,
       count(*)::int as payments,
       coalesce(sum(op.tip_amount), 0)::numeric(12,2) as tips
from order_payments op
join orders o on o.id = op.order_id
left join users u on u.id = op.received_by
where o.status not in ('cancelada', 'reembolsada')
  and op.tip_amount > 0
  and o.business_date between $1 and $2
group by u.name
order by tips desc;

-- name: TipsByDay :many
select o.business_date,
       coalesce(sum(op.tip_amount), 0)::numeric(12,2) as tips
from order_payments op
join orders o on o.id = op.order_id
where o.status not in ('cancelada', 'reembolsada')
  and op.tip_amount > 0
  and o.business_date between $1 and $2
group by o.business_date
order by o.business_date;

-- name: ProductMargins :many
-- Utilidad por producto usando snapshots de las líneas (no depende del costo actual).
select ol.product_name,
       sum(ol.quantity)::numeric(12,2) as qty,
       coalesce(sum(ol.line_total), 0)::numeric(12,2) as revenue,
       coalesce(sum(ol.unit_cost * ol.quantity), 0)::numeric(12,2) as cost,
       coalesce(sum(ol.line_total) - sum(ol.unit_cost * ol.quantity), 0)::numeric(12,2) as margin
from order_lines ol
join orders o on o.id = ol.order_id
where o.status not in ('cancelada', 'reembolsada') and o.opened_at >= $1
group by ol.product_name
order by margin desc
limit $2;
