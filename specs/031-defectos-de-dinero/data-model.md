# Data model: 031

**0080_money_by_shift_and_day.sql** (esquema + backfill de `order_refunds`) y **0081_day_of_each_payment.sql** (backfill de `order_payments.business_date`).

## order_refunds (existe desde 0060)

| Columna | Cambio | Regla |
|---|---|---|
| `register_session_id bigint` | nueva, nullable | Turno en que se devolvió. FK compuesta `(company_id, register_session_id) → register_sessions(company_id, id) on delete no action`. Nulo = sin turno abierto (solo medios que no tocan el cajón) o de un turno cerrado antes de 0080 |
| `business_date date` | nueva, nullable | Día de negocio de la devolución, reloj de la app. Backfill con la expresión de zona de 0062 (fallback `America/Mexico_City`) |
| `tip_amount numeric(10,2) not null default 0` | nueva | Propina devuelta (solo al cancelar con devolución). No suma a `orders.refund_amount` |
| `check (amount > 0)` | cambia | `amount >= 0 and tip_amount >= 0 and amount + tip_amount > 0` |

Índice nuevo: `(company_id, register_session_id)` — lo que lee el corte.

Backfill (dentro de la migración):
- `register_session_id` ← sesión de la salida de caja (`cash_movement_id`, misma empresa); y las sin
  salida hechas durante el turno principal que sigue abierto ← ese turno.
- `business_date` ← `created_at` en la zona de su empresa (expresión de 0062).
- `lock_timeout = '3s'`. Down: se niega si hay filas con `amount = 0`.

Grants: sin cambio (`select, insert, update` desde 0060; `update` lo necesita reclamar huérfanas).

## order_payments

| Columna | Cambio | Regla |
|---|---|---|
| `business_date date` | nueva, nullable | Día de negocio del cobro, reloj de la app. 0081 rellena lo anterior con `o.business_date`. Nullable para que un binario anterior en rollback siga cobrando |

Índice nuevo: `(company_id, business_date)`. Predicado: `op.business_date between … or (op.business_date is null and o.business_date between …)`.

## Reglas de dominio nuevas o cambiadas (`server/internal/domain`)

- `CobradoPorMetodo.Devuelto` y `RepartirDevolucion` sobre `Monto − Devuelto` (D1).
- `MontoDevolvibleDeRenglon` (D4).
- `VoidKeepsRefunds` + `ErrPaymentHasRefunds` (D2).
- `ErrRefundOnRefundedOrder` (D18), `ErrCashRefundNeedsOpenRegister` (D7).
- `RepartirPropinaDevuelta`: propina por medio menos la ya devuelta (D9).
- `PagosCubren` exacto (D16).
- `SplitLineTotal` (D17).

## Transiciones

Sin estados nuevos. `reembolsada` deja de admitir devoluciones; `cancelada` sigue terminal.
