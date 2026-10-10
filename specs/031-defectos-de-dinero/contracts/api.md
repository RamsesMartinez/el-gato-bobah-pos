# Contratos que cambian (031)

Solo se AGREGAN campos; ninguno se quita ni cambia de tipo. Los arreglos nuevos van siempre como
`[]`, nunca `null` (AGENTS.md §1).

## GET /cash-sessions/current y GET /cash-sessions/{id}

`SessionView` y `SessionDetailView` agregan:

```json
"refunds": [
  { "method": "Tarjeta débito", "amount": "300.00", "tip": "0.00", "orderFolio": "#12",
    "fromDrawer": false, "refundedBy": "Ana", "refundedAt": "2026-10-08T18:00:00Z", "reason": "..." }
]
```

`breakdown.ingresos[].items[]` puede traer dos conceptos nuevos:
- `"Cobros de otros turnos"` (positivo): cobros del turno de pedidos abiertos en otro turno.
- `"Devoluciones"` (negativo): devoluciones del turno por un medio que no toca el cajón.

`totals[].expected` de un medio que no toca el cajón ya viene neto de sus devoluciones del turno.
`uncollected` deja de cambiar cuando otro turno cobra.

## POST /orders/{id}/refund

Errores nuevos:
- 409 `ErrRefundOnRefundedOrder` — pedido reembolsado por el flujo anterior.
- 409 `ErrCashRefundNeedsOpenRegister` — «Abre la caja para devolver efectivo».
- 404 — el renglón no es de ese pedido o ya se quitó.
- 422 `ErrDevolucionExcede` — también cuando el monto supera lo que vale el renglón.

## POST /orders/{id}/payments/{paymentId}/void

- 409 `ErrPaymentHasRefunds` — «Ese pago ya tiene una devolución: no se puede devolver otra vez».

## GET /sales/summary y el reporte de cobros por método (`SalesByMethod`)

Cada fila por método agrega `refunds` (devuelto en el periodo por ese medio). `total` = cobrado en el
periodo − `refunds`. Un cobro cuenta en su día; una devolución, en el suyo.
