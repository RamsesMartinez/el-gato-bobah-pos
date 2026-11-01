# Contratos: 048

- Pago (`POST /expenses`, `POST /expenses/{id}/pay`): `includeInCount?: boolean`. Errores 409
  `EXPENSE_OUTSIDE_COUNT` (se puede Registrar) y `EXPENSE_BEFORE_LAST_CLOSE` (no). `outsideCount: true`
  + `includeInCount` ausente = sin incluir (compatibilidad 047); `outsideCount: true` + `includeInCount: true` = pendiente.
- `GET /cash-sessions/pending-expenses?registerId=` → `{ items: [{ paymentId, expenseId, description, amount, paidAt }] }`, desde el último cierre de la caja. `registerId` ausente o malformado → 400.
- Apertura (POST bajo `/cash-sessions`): sin campos nuevos incluye sola los pendientes desde el último
  cierre, abriendo desde la hora del primero (R13). `openedAt?: string` (RFC3339 con zona; sin zona →
  400) o `excludePending?: true` (abre ahora y deja los pendientes «sin incluir»): los dos exigen
  `cash.adjust_count` (403) y no van juntos (400). `openedAt` fuera de `(último cierre, ahora]`, o de
  más de 24 h atrás si la caja nunca cerró → 400.
- `PATCH` de la casilla: una salida de antes de la 0100 sin ligar a su pago → 409 `PAYMENT_NOT_LINKED`.
- `PATCH /expenses/{id}/payments/{paymentId}` `{ includeInCount: boolean }`: sin permiso 403; corte cerrado 409 `PAYMENT_IN_CLOSED_COUNT`; antes del último cierre 409 `EXPENSE_BEFORE_LAST_CLOSE`.
- Detalle del gasto: cada pago trae `includeInCount`, `paidAt`, `sessionClosed`.
