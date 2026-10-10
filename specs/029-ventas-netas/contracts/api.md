# Contratos (cambios aditivos; nada se quita)

## GET /api/v1/sales/summary
- `total`: ahora = Σ `byMethod[].total` (cobro neto). Antes: importe de pedidos no cancelados.
- `average`: ticket promedio de los pedidos del periodo (sin cambio de cálculo).
- `pending`: `{ count, amount }` — por cobrar, fuera del total. Nuevo.
- `byMethod[].tipRefunds`: propina devuelta por el medio en el periodo. Nuevo.

## GET /api/v1/sales
- `items[].paid`: Σ pagos vivos. Nuevo.
- `items[].lastRefundAt`: momento de la última devolución, o `null`. Nuevo.

## GET /api/v1/orders/{id}
- `refund`: lo devuelto real (antes siempre `"0"`).

## GET /api/v1/cash-sessions/{id} y turno vivo de Cajas
- Turno abierto: `totals` y `breakdown` en vivo (antes vacíos en el detalle).
- `totals[].tips`: neta de la propina devuelta en el turno.
- `breakdown.ingresos[].items`: «Devoluciones» = toda devolución de venta del medio (cajón o no);
  «Propinas» neta. `breakdown.ingresos[].note`: texto cuando el total del medio es negativo. Nuevo.
- `breakdown.egresos`: «Salidas de efectivo» ya no incluye salidas de devoluciones.

## GET /api/v1/reports/product-margins (o el endpoint vigente de utilidad)
- `uncostedRevenue` por producto. Nuevo. `margin` solo sobre renglones con costo.

## Rechazos nuevos (400/422)
- POST /orders con plataforma y `serviceType: "mostrador"`.
- POST /orders/{id}/pay con monto < $0.01.
- POST /orders/{id}/refund sin nada por devolver (pedido o renglón): mensaje explícito.
