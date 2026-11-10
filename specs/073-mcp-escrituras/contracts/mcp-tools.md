# Contrato: herramientas MCP nuevas (073)

Las reglas comunes (dinero como texto, fechas `AAAA-MM-DD`, `dry_run` por omisión `true`,
`idempotency_key` al aplicar, errores con causa y corrección, visibilidad por alcance y permiso) son
las de [la 072](../../072-servidor-mcp/contracts/mcp-tools.md). Rutas relativas a `/api/v1`.

## Catálogo

| Herramienta | Lee/escribe | Ruta que la atiende | Lecturas de su vista previa | Permiso | Alcance |
| --- | --- | --- | --- | --- | --- |
| `articles_search` | Lee | `GET /articles?q=` | — | `expenses.capture` | `read` |
| `units_list` | Lee | `GET /units` | — | `expenses.capture` | `read` |
| `presentations_list` | Lee | `GET /stock/presentations` | — | `inventory.receive` | `read` |
| `pending_receipts_list` | Lee | `GET /stock/pending-receipts` | — | `inventory.receive` | `read` |
| `purchase_create` | Escribe | `POST /expenses/` | `POST /expenses/purchase-preview`, `GET /articles/suggest`, `GET /stock/presentations`, `GET /suppliers`, `GET /expense-categories`, `GET /payment-methods`, `GET /cash-sessions/current`, `GET /expenses/{id}` | `expenses.capture` | `write:purchases` |
| `purchase_receive` | Escribe | `POST /expenses/{id}/receive` | `GET /expenses/{id}` | `inventory.receive` | `write:purchases` |
| `stock_count_current` | Lee | `GET /stock/counts/current` | — | `inventory.count` | `read` |
| `stock_count_open` | Escribe | `POST /stock/counts` | `GET /stock/counts/current` | `inventory.count` | `write:inventory` |
| `stock_count_capture` | Escribe | `PUT /stock/counts/{id}/lines/{ingredientId}`, `PUT /stock/counts/{id}/product-lines/{productId}` | `GET /stock/counts/current` | `inventory.count` | `write:inventory` |
| `stock_count_close` | Escribe | `POST /stock/counts/{id}/close` | `GET /stock/counts/{id}/preview` | `inventory.count` | `write:inventory` |
| `stock_count_discard` | Escribe | `POST /stock/counts/{id}/discard` | `GET /stock/counts/current` | `inventory.count` | `write:inventory` |
| `order_cancel` | Escribe | `POST /orders/{id}/cancel` | `GET /orders/{id}` | `orders.cancel` | `write:orders` |
| `cash_denominations_list` | Lee | `GET /cash/denominations` | — | `cash.operate` | `read` |
| `cash_session_attendance` | Lee | `GET /cash-sessions/attendance?registerId=` | — | `cash.operate` | `read` |
| `cash_session_open` | Escribe | `POST /cash-sessions/` | `GET /cash-sessions/attendance`, `GET /cash-sessions/current`, `GET /cash-sessions/pending-expenses` | `cash.operate` | `write:cash` |
| `cash_session_close` | Escribe | `POST /cash-sessions/close` | `GET /cash-sessions/current`, `GET /cash-sessions/attendance`, `GET /cash-sessions/tips` | `cash.operate` | `write:cash` |

## Argumentos de las escrituras

Todas llevan además `dry_run` e `idempotency_key`.

- **`purchase_create`**: `category_id`, `amount` (total), `supplier_id?`, `document_date?`,
  `doc_folio?`, `description?`, `received_on?` (por omisión hoy), `pending?` (true = queda por
  recibir), `payments?` (como en `expense_create`), `lines` (1 a 300):
  `{ ingredient_id? | product_id?, presentation_id? | unit_id?, quantity, amount, is_gift?, name? }`.
  En vista previa un renglón puede traer solo `name` (lo que dice el ticket) y recibe candidatos; al
  aplicar, cada renglón exige artículo y empaque o unidad.
- **`purchase_receive`**: `expense_id`, `received?` (`[{item_id, qty}]`; ausente = llegó todo),
  `received_on?`.
- **`stock_count_open`**: sin argumentos.
- **`stock_count_capture`**: `count_id`, `lines` (1 a 50):
  `{ ingredient_id? | product_id?, counted_qty, confirmed? }`.
- **`stock_count_close`** / **`stock_count_discard`**: `count_id`.
- **`order_cancel`**: `order_id`, `reason`, `refund?` (obligatorio `true` si el pedido tiene cobros),
  `card_folio?` (obligatorio si se devuelve algo cobrado con tarjeta).
- **`cash_session_open`**: `register_id`, uno de `counts` (`[{denomination_id, pieces}]`) u
  `opening_cash` + `manual_reason`; `opening_reason?` (`last_count_wrong`, `float_changed`,
  `unrecorded_withdrawal`, `other`) + `opening_reason_note?`.
- **`cash_session_close`**: `register_id`, uno de `counts` o `counted_cash` + `manual_reason` (si el
  cajón pide conteo), `declared?` (`{method_id: "monto"}`), `terminal_counts?`
  (`{terminal_id: "monto"}`), `tips_decision?`, `float_left?`, `withdrawal_user_id?`, `notes?`,
  `authorized_close_reason?`.

## Respuestas que cambian respecto de la 072

- **`stock_count_capture`** al aplicar: `{"applied": true, "captured": [{…, "counted_qty"}],
  "failed": null}`; si un renglón falla: error que nombra el renglón (índice desde 1 y artículo) y
  lista los que sí quedaron.
- **`cash_session_close`** en vista previa: `resolved.to_declare` = `{ drawer: bool, methods:
  [nombres], tips_decision_required: bool, attendance: {…} }`. Nunca el esperado con el arqueo
  ciego encendido.
- **`purchase_create`** en vista previa: `resolved.lines[]` con `candidates` (renglón sin artículo),
  `presentations` (artículo elegido) y `cost` (`unitCost`, `costBefore`, `costAfter`,
  `keptByNewerPurchase`) cuando el renglón está completo.
