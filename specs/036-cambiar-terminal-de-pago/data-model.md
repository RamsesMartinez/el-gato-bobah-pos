# Data Model

Sin tablas ni columnas nuevas.

| Entidad | Campos que se usan | Cambio |
|---|---|---|
| `order_payments` | `card_terminal_id`, `card_terminal_name`, `payment_method_id`, `register_session_id`, `order_id` | se actualizan los dos de terminal |
| `card_terminals` | `id`, `branch_id`, `name`, `archived_at` | solo lectura |
| `audit_events` | acción `payment.terminal_changed`, grupo `money`; `before = {terminalId, terminalName}`, `after = {terminalId, terminalName}`, `reason`, entidad = pago, pedido | inserción |
| `session_terminal_counts` | `expected` | solo lectura (comparación) |

## Validación (domain.ValidateTerminalChange)
- Método del pago = tarjeta; pedido no cancelado; pago vivo (los devueltos no están en `order_payments`).
- Destino ∈ activas de la sucursal del turno del pago; distinto del actual.
- Motivo: `btrim`, 3 a 200 caracteres (misma regla que `attendance_corrections.reason`).

## Permiso
- `payments.change_terminal`: admin y gerente.
