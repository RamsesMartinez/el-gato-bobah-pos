# Data Model: Ventas netas y devoluciones a la vista

Sin cambios de esquema. Lecturas nuevas sobre lo que ya existe:

| Cifra | Fuente | Regla |
|---|---|---|
| Cobro neto por medio | `order_payments.amount` por `business_date` del pago − `order_refunds.amount` por `business_date` de la devolución | Ya existe (031) |
| Propina devuelta por medio (Ventas) | `order_refunds.tip_amount` por día de la devolución | Nueva columna de salida `tip_refunds` |
| Total de Ventas | Σ cobro neto por medio | `domain.NetCollected` |
| Por cobrar | Pedidos vivos del periodo con `total − Σ pagos > 0` | Consulta `SalesPending` |
| Marca de devolución | `orders.refund_amount`, `max(order_refunds.created_at)` | Columnas `last_refund_at`, `paid` en la lista |
| Propina neta del corte | `order_payments.tip_amount` del turno − `order_refunds.tip_amount` del turno | `refunded_tips` + `drawer_refunded_tips` |
| Devolución de venta del corte | `order_refunds.amount` del turno, del cajón o no | `refunded − refunded_tips` + `drawer_refunded − drawer_refunded_tips` |
| Salida de caja de devolución | `register_cash_movements` referida por `order_refunds.cash_movement_id` | `is_refund` en `ListCashMovements` |
| Utilidad sin costo | `order_lines.unit_cost = 0` | `uncosted_revenue`; margen solo sobre costeados |
