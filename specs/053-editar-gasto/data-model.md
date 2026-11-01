# Data Model: Editar un gasto ya guardado (entrega 1)

## Migración 0103 (`0103_stock_movement_effective_on.sql`)

Una sola columna. Nada se borra ni se renombra: la API anterior sigue funcionando durante la subida
(constitución IV, «Toda migración tolera a la versión anterior»).

| Tabla | Cambio | Por qué |
|---|---|---|
| `stock_movements` | `add column effective_on date` (nullable, sin default) | Día de negocio en que pasó el movimiento (la entrega), distinto de `created_at` (cuándo se capturó). |
| `stock_movements` | trigger `before insert` `fill_stock_movement_effective_on` | Llena `effective_on` nula en un `compra` o un movimiento con `expense_id`, con el día de hoy en la zona de la empresa (`business_settings.timezone`, `America/Mexico_City` por omisión). Cubre a la API vieja durante la subida. |
| `stock_movements` (datos) | `update … set effective_on = (created_at at time zone tz)::date where movement_type = 'compra' and effective_on is null` | Relleno de lo existente con la zona vigente de cada empresa; después ningún `compra` queda nulo. |

- **Down**: se niega (`raise exception`) si algún movimiento tiene `effective_on` distinto del día de
  su `created_at` en la zona vigente: esa fecha la capturó una persona y no se recupera (precedente
  de 0089/0092/0099). Si no hay ninguna, borra el trigger, su función y la columna.
- **Índices**: ninguno nuevo. El historial se lee por `expense_item_id`
  (`stock_movements_by_expense_item`) y por gasto; la regla de conteo posterior se lee por
  `stock_count_lines_by_ingredient`.
- **RLS / GRANT**: columna en tabla existente; hereda política y grants. Sin tabla nueva, así que
  `TestEveryCompanyTableIsIsolated` no cambia.
- **Respaldo anónimo**: `date` no se clasifica (`TestEveryColumnIsCoveredByTheAnonymousBackup` solo
  pide texto, json, binario, IP, uuid o arreglo).
- **Test de la migración** (obligatorio, hook `migracion-con-test.sh`): sube y baja con dos empresas
  (zonas distintas); el relleno usa la zona de cada una; un insert sin la columna (API vieja) la llena;
  el Down se niega con una fecha capturada distinta.

## Entidades que cambian de comportamiento

### Gasto (`expenses`)

- `amount`: lo mueve una edición o una entrega según R4 (`nuevo = max(anterior − borrados − ajustados
  + de_más, suma de vivos incluidos)`), siempre > 0 y `ValidMoney`.
- `status`, `paid_at`, `paid_by`: se recalculan solo si cambió `amount`: `pagado ≥ amount` → `pagada`
  (si cambia a pagada: `paid_at = now()`, `paid_by` = quien edita); si no → `pendiente`, ambos nulos;
  una `pagada` sin ningún pago no se degrada. Un gasto `cancelada` no se edita, y uno con pagos no se
  anula (`CancelExpense` exige que no haya pagos).
- `supplier_id`, `category_id`, `description`, `expense_date`: editables (FR-001). `expense_date` ≤
  hoy en la zona del negocio.
- `received_at`: cuando no queda nada pendiente, el máximo `coalesce(effective_on, día de
  created_at)` de los movimientos `compra` con cantidad > 0 de sus renglones vivos; si queda algo
  pendiente, nulo (regla de `PlanStockLines`, sin cambio).

### Renglón (`expense_items`)

- Agregar: `CreateExpenseItem` (ya existe). Con «Ya llegó», `qty_received = quantity`.
- Borrar: `voided_at = now()` (ya existe, `VoidStockLine`); antes, sus contra-movimientos por entrega
  (R7). Un excluido no se borra desde esta edición.
- «Ajustar el importe a lo recibido»: `quantity = qty_received` (lo recibido en total),
  `amount = Round2(amount × recibido / pedido)`, `unit_cost = amount / quantity`. La reducción
  (`amount_anterior − amount_nuevo`) cuenta como `ajustados` en R4.
- «Sigue pendiente»: solo sube `qty_received` (como hoy).

### Movimiento de inventario (`stock_movements`)

| Caso | `movement_type` | `quantity` | `expense_id` | `expense_item_id` | `stock_count_id` | `effective_on` |
|---|---|---|---|---|---|---|
| Entrega | `compra` | + | gasto | renglón | — | día de la entrega |
| Contra-movimiento de un borrado (uno por día de entrega) | `compra` | − | gasto | renglón | — | ese día |
| Compensación de «No mover» | `ajuste` | contraria a la anterior | gasto | **nulo** | conteo posterior | la del movimiento que compensa |

La compensación lleva `branch_id` explícito: el del conteo (que es el de la sesión, por R5).

Invariante (ya existente, se conserva): `expense_items.applied_qty_base` = suma de los movimientos
con su `expense_item_id`. La compensación no lo toca.

### Conteo posterior (lectura, R5)

Existe para un insumo, un día `D` y un instante `R` si hay un `stock_count_lines` del insumo cuyo
`stock_counts.status = 'closed'`, `stock_counts.branch_id = current_branch_id()` y
`captured_at > least(R, fin del día D en la zona del negocio)`. Se
devuelve el `count_id` del más reciente.

## Tipos de dominio nuevos (`server/internal/domain/expense_edit.go`, puros)

- `ExpenseAmountAfterEdit(before, removed, adjusted, extra, liveIncluded decimal) (decimal, error)` —
  R4; error si ≤ 0 o fuera de `ValidMoney`.
- `PaymentStatusFor(paid, amount decimal) ExpenseStatus` — `pagada`/`pendiente`.
- `ShortfallDefault(buyKind string, byPresentation bool) Shortfall` — FR-017: pieza o empaque →
  `pending`; masa, volumen o longitud → `adjust`.
- `AdjustLineToReceived(qty, received, amount decimal) (newQty, newAmount decimal)` — FR-018.
- `CountDecision` (`adjust`/`keep`) y `MoneyDecision` (`pay`/`leave`) con `Parse…` que rechaza lo
  desconocido (constitución V: nunca cae a un default).
- `ValidDeliveryDate(day, expenseDate, today time.Time) error` — FR-015.
- `EditNeedsReceive(in EditInput) bool` — R8.
- `PreviewFingerprint(p EditPreview) string` — R2 (sha256 del JSON canónico).
- `PlanDeliveryReversal(days []DeliveryDay) []DeliveryDay` — R7: netea los días negativos contra los
  positivos más recientes y devuelve solo cantidades a restar, que suman `−aplicado`.
- `CanCancelExpense(status string, hasPayments bool) bool` — revisión A1.
- `CountPosteriorCutoff(registeredAt time.Time, day time.Time, loc *time.Location) time.Time` — R5:
  `least(R, fin del día D en la zona)`.

## Vista previa (respuesta, ver contracts/edit-expense.md)

`EditPreview { header[], lines[], inventory[], money, costs[], decisions[], fingerprint }`. Cada
efecto trae `before`/`after`; cada decisión trae su `key`, sus opciones, la elegida (o nula) y a qué
línea pertenece.
