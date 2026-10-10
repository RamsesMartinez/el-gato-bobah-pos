# Contratos HTTP (`/api/v1`)

Todas con `RequireAuth`; roles entre corchetes. Montos en pesos, 2 decimales. Arreglos siempre `[]`, nunca `null`.

| Método y ruta | Rol | Cuerpo / respuesta | Errores |
| --- | --- | --- | --- |
| `GET /cash/sessions/{id}/tips/pending` | cajero+ | `{byMethod:[{kind,pending}], carriedOver, total}` | 404 |
| `POST /cash/sessions/{id}/tips/payouts` | cajero+ | `{recipients:[{userId, amount?}]}` (amount omitido = parte del pendiente; método de origen lo decide el servidor) → movimiento | 422 `tip_exceeds_pending`, 409 sesión cerrada |
| `POST /cash/sessions/{id}/close` | cajero+ | se amplía: `{tipsDecision: "entregar"|"quedan_en_caja", terminalCounts:[{terminalId,declared}], ...}` | 422 `tips_decision_required`, `terminal_count_required` |
| `POST /cash/sessions/open` | cajero+ | `{registerId, denominations:[{value,qty}], reason?, note?}`; nunca devuelve el cierre anterior | 422 `opening_reason_required` |
| `POST /cash/movements/{id}/correct` | gerente+ | `{conceptId, amount, ...}` → `{reversal, replacement}` | 409 `already_reversed`, 422 |
| `GET/POST /cash/concepts` · `PATCH /cash/concepts/{id}` · `POST /cash/concepts/{id}/merge` | lectura cajero+, edición admin | | 409 nombre duplicado |
| `GET/POST /branches/{id}/terminals` · `PATCH /terminals/{id}` | admin | | 409 |
| `PATCH /branches/{id}/card-count-mode` | admin | `{mode}` | 400 valor desconocido |
| `PUT /me/default-terminal` | cualquiera | `{terminalId|null}` | 422 otra sucursal/archivada |
| `POST /orders/{id}/pay` | se amplía: pago tarjeta exige `{terminalId, cardKind}` | 422 `card_terminal_required` |
| `POST /orders/{id}/refunds` | se amplía: `{cardRefundFolio}` si el pago fue tarjeta | 422 `refund_folio_required` |
