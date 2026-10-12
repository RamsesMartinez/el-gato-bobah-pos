# Contrato

## POST /api/v1/orders/{id}/payments/{paymentId}/terminal
Permiso `payments.change_terminal`.

Request: `{ "fromTerminalId": 2, "terminalId": 3, "reason": "Se cobró en otra terminal" }` (`fromTerminalId` nulo si el pago no tenía terminal)

200: el pedido completo (`OrderView`), como devolver un pago.

| Error | Cuándo |
|---|---|
| 400 VALIDATION | motivo vacío o largo; terminal inactiva, de otra sucursal o igual a la actual; pago que no es con tarjeta |
| 403 | sin permiso |
| 404 | pedido o pago inexistente (o de otra empresa) |
| 409 | pedido cancelado, o `fromTerminalId` ya no es la terminal del pago (otro cambio llegó antes) |

## GET /api/v1/orders/{id}
`payments[]` agrega `terminalId: number | null` y `terminalName: string | null`.

## Corte cerrado
Cada renglón del arqueo por terminal agrega `expectedNow` (lo esperado hoy) junto a `expected` (lo esperado al cerrar).

## POST cobrar
Sin cambio de forma: `terminalId` sigue opcional; sin él y con 2+ terminales activas responde 400, ahora también cuando el usuario tenía una recordada.
