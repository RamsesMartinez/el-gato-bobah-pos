-- name: SalesByDay :many
select o.business_date,
       count(*)::int as orders,
       coalesce(sum(o.total), 0)::numeric(12,2) as revenue
from orders o
where o.status not in ('cancelada', 'reembolsada') and o.business_date between $1 and $2
group by o.business_date
order by o.business_date;

-- name: SalesByMethod :many
-- Cobros por medio de pago, con EL MISMO predicado que SalesByDay. No es cosmético: las dos tablas
-- viven en la misma pantalla, y si una responde otro periodo o incluye otras ventas, la suma de los
-- métodos no cuadra con el total de arriba y quien lo lee no tiene forma de saber cuál miente.
--
-- Dos cosas cambiaron y las dos habían costado:
--   * Filtraba por `op.created_at >= $1`, SIN cota superior: elegir julio mostraba julio en una
--     tabla y "de julio a hoy" en la otra. Además `created_at` es el instante del cobro y no el día
--     de negocio, así que un cobro de un turno que cruza la medianoche caía en otro día que su venta.
--   * No excluía canceladas ni reembolsadas. El cobro de una venta reembolsada seguía sumando aquí
--     mientras el total de arriba —que sí la excluye— no lo contaba: ingreso que no ocurrió,
--     clasificado dos veces distinto en la misma pantalla.
select pm.name as method,
       count(*)::int as payments,
       coalesce(sum(op.amount), 0)::numeric(12,2) as total
from order_payments op
join orders o on o.id = op.order_id
join payment_methods pm on pm.id = op.payment_method_id
where o.status not in ('cancelada', 'reembolsada')
  and o.business_date between $1 and $2
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
--
-- Agrupa por u.id y NO por u.name. Con el nombre, dos empleados que se llamen igual salían en un
-- solo renglón con la suma de los dos, y un renglón así no se puede repartir: quien lo lee no sabe
-- cuánto le toca a cada quien. Es el único reporte que existe para entregar dinero a una persona.
select coalesce(u.name, 'Sin asignar') as employee,
       count(*)::int as payments,
       coalesce(sum(op.tip_amount), 0)::numeric(12,2) as tips
from order_payments op
join orders o on o.id = op.order_id
left join users u on u.id = op.received_by
where o.status not in ('cancelada', 'reembolsada')
  and op.tip_amount > 0
  and o.business_date between $1 and $2
group by u.id, u.name
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
--
-- Acotada por DÍA DE NEGOCIO y en los dos extremos, igual que sus hermanas de pantalla. Filtraba
-- por `o.opened_at >= $1` sin cota superior: con un filtro de fechas encima, esta tabla habría
-- seguido contestando "desde esa fecha hasta hoy" mientras el resto de la pantalla contestaba el
-- rango elegido. Y `opened_at` es un instante en UTC, no el día con el que el negocio cuadra su
-- caja: un pedido abierto a las 19:00 de México ya es del día siguiente en UTC.
select ol.product_name,
       sum(ol.quantity)::numeric(12,2) as qty,
       coalesce(sum(ol.line_total), 0)::numeric(12,2) as revenue,
       coalesce(sum(ol.unit_cost * ol.quantity), 0)::numeric(12,2) as cost,
       coalesce(sum(ol.line_total) - sum(ol.unit_cost * ol.quantity), 0)::numeric(12,2) as margin
from order_lines ol
join orders o on o.id = ol.order_id
where o.status not in ('cancelada', 'reembolsada')
  and o.business_date between $1 and $2
group by ol.product_name
order by margin desc
limit $3;
