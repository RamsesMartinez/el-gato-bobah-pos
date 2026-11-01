# Contratos: 048

- Pago (`POST /expenses`, `POST /expenses/{id}/pay`): `includeInCount?: boolean`. Errores 409
  `EXPENSE_OUTSIDE_COUNT` (se puede Registrar) y `EXPENSE_BEFORE_LAST_CLOSE` (no). `outsideCount: true`
  + `includeInCount` ausente = sin incluir (compatibilidad 047); `outsideCount: true` + `includeInCount: true` = pendiente.
- `GET /cash-sessions/pending-expenses?registerId=` → `{ items: [{ paymentId, expenseId, description, amount, paidAt }] }`, desde el último cierre de la caja. `registerId` ausente o malformado → 400.
- Apertura (POST bajo `/cash-sessions`): `openedAt?: string` (RFC3339). Sin `cash.adjust_count` → 403; fuera de `(último cierre, ahora]` → 400.
- `PATCH /expenses/{id}/payments/{paymentId}` `{ includeInCount: boolean }`: sin permiso 403; corte cerrado 409 `PAYMENT_IN_CLOSED_COUNT`; antes del último cierre 409 `EXPENSE_BEFORE_LAST_CLOSE`.
- Detalle del gasto: cada pago trae `includeInCount`, `paidAt`, `sessionClosed`.
