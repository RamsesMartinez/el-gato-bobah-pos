-- name: SalesByDay :many
select o.business_date,
       count(*)::int as orders,
       coalesce(sum(o.total), 0)::numeric(12,2) as revenue
from orders o
where o.status not in ('cancelada', 'reembolsada') and o.business_date between $1 and $2
group by o.business_date
order by o.business_date;

-- name: SalesByMethod :many
-- Cobros por medio de pago: lo que ENTRÓ cada día menos lo que se DEVOLVIÓ cada día (spec 031).
--
-- El dueño decidió que el dinero se clasifica por la hora de CADA movimiento: un cobro cuenta el día
-- en que se cobró y una devolución el día en que se devolvió. Antes los dos tomaban el día del
-- PEDIDO, así que un fiado cobrado días después aparecía en un día ya cerrado —mientras el corte de
-- caja lo esperaba hoy— y una devolución del mes siguiente borraba el cobro del mes en que entró.
-- Esto ya no es la venta del día (SalesByDay): es el dinero, y por eso puede diferir de ella.
--
-- Qué pedidos cuentan: los no cancelados ni reembolsados, más los cancelados que tienen su
-- devolución en el libro (su cobro cuenta en su día y su devolución resta en el suyo). Un cancelado
-- SIN libro es anterior a 0060 —cancelar no miraba los cobros— o un reembolsado por el flujo viejo,
-- que no escribía el libro: contarlos inflaría periodos cerrados con dinero que sí se regresó.
--
-- Cobros y devoluciones se agregan por método cada uno antes de unirse —son dos 1:N del pedido— y
-- se unen por el método, para que uno que solo tuvo devoluciones en el periodo también salga.
--
-- El predicado del día va partido para usar `order_payments_company_day`: el `or` cubre solo los
-- pagos que un binario anterior pudiera dejar sin día.
with pagos as (
  select op.payment_method_id, count(*) as payments, sum(op.amount) as cobrado
    from order_payments op
    join orders o on o.id = op.order_id
   where (op.business_date between sqlc.arg(desde) and sqlc.arg(hasta)
          or (op.business_date is null and o.business_date between sqlc.arg(desde) and sqlc.arg(hasta)))
     and (o.status not in ('cancelada', 'reembolsada')
          or exists (select 1 from order_refunds r where r.order_id = o.id))
   group by op.payment_method_id
), devueltos as (
  select r.payment_method_id, sum(r.amount) as devuelto
    from order_refunds r
    join orders o on o.id = r.order_id
   where coalesce(r.business_date, o.business_date) between sqlc.arg(desde) and sqlc.arg(hasta)
   group by r.payment_method_id
)
select pm.name as method,
       coalesce(p.payments, 0)::int as payments,
       (coalesce(p.cobrado, 0) - coalesce(d.devuelto, 0))::numeric(12,2) as total,
       coalesce(d.devuelto, 0)::numeric(12,2) as refunds
from payment_methods pm
left join pagos p on p.payment_method_id = pm.id
left join devueltos d on d.payment_method_id = pm.id
where p.payment_method_id is not null or d.payment_method_id is not null
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
--
-- Spec 031 (D10): sin los renglones QUITADOS —no se vendieron, y ProductsSold y el total del pedido
-- ya los excluían— y con el descuento del pedido repartido entre sus renglones en proporción a su
-- importe. Así el ingreso por producto suma lo vendido menos envíos, que no es de ningún producto.
--
-- Spec 029: lo vendido SIN COSTO CAPTURADO (`unit_cost = 0`) va en `uncosted_revenue` y no suma al
-- margen. Restarle un costo de cero mostraba como margen la venta entera, y el producto sin costo
-- parecía el más rentable de la carta. `unit_cost = 0` no distingue «gratis» de «sin capturar»: es
-- un hecho ya guardado así, y lo honesto es decir que no hay costo, no inventar un margen.
-- Misma expresión prorrateada que `revenue`, para que las dos cifras se puedan comparar.
select ol.product_name,
       sum(ol.quantity)::numeric(12,2) as qty,
       coalesce(sum(ol.line_total * (o.total - o.delivery_fee) / nullif(o.subtotal, 0)), 0)::numeric(12,2) as revenue,
       coalesce(sum(ol.unit_cost * ol.quantity), 0)::numeric(12,2) as cost,
       (coalesce(sum(ol.line_total * (o.total - o.delivery_fee) / nullif(o.subtotal, 0)) filter (where ol.unit_cost > 0), 0)
        - coalesce(sum(ol.unit_cost * ol.quantity), 0))::numeric(12,2) as margin,
       coalesce(sum(ol.line_total * (o.total - o.delivery_fee) / nullif(o.subtotal, 0)) filter (where ol.unit_cost = 0), 0)::numeric(12,2) as uncosted_revenue
from order_lines ol
join orders o on o.id = ol.order_id
where o.status not in ('cancelada', 'reembolsada')
  and ol.cancelled_at is null
  and o.business_date between $1 and $2
group by ol.product_name
order by margin desc
limit $3;

-- name: ProductsSold :many
-- Unidades vendidas por producto, sueltas y dentro de paquetes (spec 028). Mismo predicado que la
-- venta: pedido no cancelado ni reembolsado, renglón no cancelado, día de negocio en el rango. Los
-- componentes salen de la copia hecha al vender (order_line_components), no del paquete de hoy.
--
-- Cada rama se agrega por producto antes de unir: unir renglones y componentes en la misma consulta
-- multiplicaría las filas (AGENTS.md §1).
with alone as (
  select ol.product_id, sum(ol.quantity) as qty
    from order_lines ol join orders o on o.id = ol.order_id
   where o.status not in ('cancelada', 'reembolsada') and ol.cancelled_at is null
     and o.business_date between sqlc.arg(from_date) and sqlc.arg(to_date)
     and ol.product_id is not null
   group by ol.product_id
), packed as (
  select c.product_id, sum(c.quantity) as qty
    from order_line_components c
    join order_lines ol on ol.id = c.order_line_id
    join orders o on o.id = ol.order_id
   where o.status not in ('cancelada', 'reembolsada') and ol.cancelled_at is null
     and o.business_date between sqlc.arg(from_date) and sqlc.arg(to_date)
   group by c.product_id
)
select p.name as product_name,
       coalesce(a.qty, 0)::numeric(14,2) as alone,
       coalesce(k.qty, 0)::numeric(14,2) as in_packages
  from products p
  left join alone a on a.product_id = p.id
  left join packed k on k.product_id = p.id
 where (a.qty is not null or k.qty is not null) and p.type <> 'combo'
 order by coalesce(a.qty, 0) + coalesce(k.qty, 0) desc, p.name
 limit sqlc.arg(row_limit);
