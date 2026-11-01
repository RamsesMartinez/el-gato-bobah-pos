# Implementation Plan: Arqueo por hora de los gastos

**Branch**: `048-arqueo-por-hora` | **Date**: 2026-11-01 | **Spec**: [spec.md](spec.md)

## Summary

Un tercer estado para el pago en efectivo: **pendiente de corte** (incluido, sin corte todavía). Dos
columnas en `expense_payments` (`include_in_count`, `paid_at`), una función de «barrido» que al abrir y
al cerrar un corte le pega los pendientes de su caja cuya hora cae en la ventana, apertura con hora
anterior por permiso, y un interruptor por pago con permiso. La regla vive en `domain`; el barrido en
una sola función de `app` que usan apertura, cierre y el interruptor.

## Technical Context

Go 1.27 · chi · pgx + sqlc · goose; React 19 · Chakra v3 · TanStack Query. Tabletas 1024×600. Se sube
con el negocio operando: migración aditiva.

## Constitution Check

| Principio | Cómo |
|---|---|
| I. Capas | `domain.ExpensePaymentTarget` decide Corte / Pendiente / Fuera; `domain.ValidBackdatedOpening` valida la hora; `app.sweepPendingExpenses` es el único que pega pendientes a un corte. |
| II. Errores | Sentinels nuevos `ErrExpenseBeforeLastClose` (409 `EXPENSE_BEFORE_LAST_CLOSE`), `ErrPaymentInClosedCount` (409 `PAYMENT_IN_CLOSED_COUNT`), `ErrInvalidOpeningTime` (400). |
| III. Dinero | Cada pendiente se descuenta una vez: el barrido toma la fila con `register_session_id is null` en el `update … returning`, bajo el candado del turno. Test que nombra el concepto: `TestUnPendienteSeDescuentaUnaSolaVez`. |
| IV. Tests | Tabla de dominio primero; integración bajo `appRoleStore` con `inTheThreeCases` para la lista de pendientes; migración con dos empresas. |
| V. Seguridad | Hora anterior y casilla: permiso nuevo `cash.adjust_count` revisado en el servidor; test de 403. |
| VI. YAGNI | Mover la hora solo al abrir; nada de reabrir cortes cerrados. |
| VIII. Puertas | `paid_at` es el hecho irrecuperable que habilita cortes por hora; se guarda desde hoy. No cierra ninguna puerta. |
| Subidas sin ventana | Migración aditiva; los inserts de la API vieja toman `include_in_count = false` (el trigger lo vuelve `true` con corte) y `paid_at = clock_timestamp()`: se comporta igual que antes (R1, R5). |

## Capas

| Pieza | Capa |
|---|---|
| `0100_expense_payment_count_window.sql` | migrations |
| `ExpensePaymentTarget` (ampliada), `ValidBackdatedOpening`, `PermCashAdjustCount`, acciones de bitácora `expense.count_changed`, `cash.opening_backdated` | domain |
| `validatePayments`, `insertPayments`, `sweepPendingExpenses`, `OpenSession` (hora), `CloseSession` (barrido), `SetPaymentInCount`, `PendingExpenses` | app |
| `includeInCount` en el pago, `openedAt` en la apertura, `PATCH /expenses/{id}/payments/{pid}`, `GET /cash-sessions/pending-expenses?registerId=` | httpapi |
| Casilla en `PaymentRow`, hoja de aviso (R9), línea de pendientes con «Incluirlos (desde las HH:MM)» en la apertura (R11), interruptor en el detalle del gasto | web |

## Reglas

- **Hora del pago (`paid_at`)**: `now()` si `paidOn` es el día de negocio de hoy; si no, la medianoche
  de `paidOn` en la zona del negocio.
- **Destino de un pago en efectivo de caja con arqueo**:
  1. `includeInCount = false` → Fuera.
  2. Turno abierto y (`paidOn` = día del turno o `paid_at` ≥ apertura) → Corte.
  3. Si no: si `paid_at` ≤ último cierre de esa caja → 409 `EXPENSE_BEFORE_LAST_CLOSE` (solo «sin
     incluir» o cancelar); si hay turno abierto y `paid_at` < su apertura → mismo 409 (ese corte ya
     tiene ventana); si no, sin confirmación → 409 `EXPENSE_OUTSIDE_COUNT`, con confirmación → Pendiente.
  - Compatibilidad 047: `includeInCount` ausente y `outsideCount = true` = Fuera.
  - Tarjeta, transferencia y cajas sin arqueo: `include_in_count = false`, como hoy.
- **Barrido**: `update expense_payments … set register_session_id = $turno where register_id = $caja
  and register_session_id is null and include_in_count and paid_at between $desde and $hasta and
  método mueve cajón returning …`, y por cada fila una salida de caja, en la MISMA transacción que
  abre o cierra (bajo el candado del turno). Se corre al abrir (ventana `[apertura, ahora]`) y al
  cerrar (`[apertura, cierre]`) antes de calcular el esperado.
- **Apertura con hora** (`openedAt`): exige `cash.adjust_count`; `último cierre < openedAt ≤ ahora`;
  bitácora `cash.opening_backdated` con la hora.
- **Casilla** (`PATCH`): exige `cash.adjust_count`; corte cerrado → 409; desmarcar en corte abierto →
  movimiento `reverso` por el importe y se suelta del corte; marcar → si hay turno abierto con su hora en
  la ventana entra con su salida; si su hora ≤ último cierre → 409 `EXPENSE_BEFORE_LAST_CLOSE`; si no,
  queda pendiente. Bitácora `expense.count_changed`.

## Pantallas (sin pantallas nuevas)

Ver R9–R12 (reemplazan la versión anterior de esta sección).

## Bordes

- B1 dos aperturas a la vez: el índice de un turno abierto por caja deja una; el barrido corre dentro
  de esa transacción.
- B2 un pendiente pegado dos veces: imposible, el `update` filtra `register_session_id is null`.
- B3 cierre concurrente con el interruptor: el interruptor toma el turno `for share` como un pago.
- B4 legado 047 (`outsideCount` sin `includeInCount`) = Fuera.
- B5 caja archivada con pendientes: se quedan fuera, sin error.

## Ajustes tras la revisión de arquitectura (2026-11-01)

**Base de datos**
- R1. `include_in_count default false` (la API vieja inserta «fuera» y tarjeta como antes). El trigger
  de la 0099 se amplía: con `register_session_id` puesto fuerza `include_in_count := true`.
  `check (register_session_id is null or include_in_count)`. La API nueva manda `true` explícito para
  pendiente y corte. La fila «Subidas sin ventana» queda: la API vieja se comporta igual que antes.
- R2. `register_cash_movements.expense_payment_id` (nullable, FK compuesta con `unique (company_id, id)`
  nuevo en `expense_payments`, `on delete restrict`) y `unique (expense_payment_id) where kind =
  'salida'`: el doble descuento falla en la base. Toda salida y reverso de gasto lleva su pago. El
  reverso de «desmarcar» se ubica por `expense_payment_id`.
- R2b. Desmarcar un pago de un corte ABIERTO borra su salida (el corte no se ha contado; la bitácora
  guarda el cambio) en vez de agregar un reverso: así volver a marcarlo en el mismo corte no choca con
  la unique. En corte cerrado no se puede cambiar (409).
- R3. El PATCH toma el pago `for update` y decide con lo que ve; el paso de pendiente usa
  `update … where id = $1 and register_session_id is null returning`.
- R4. El camino de pago toma la caja `for share` (y luego el turno) y re-decide el destino tras el
  candado; la apertura ya la toma `for update`. Un pago que espera al cierre termina en 409, no pendiente.
- R5. `paid_at` se pasa desde Go con el reloj del servicio; la columna lleva `default clock_timestamp()` para los inserts de la API vieja durante la subida (A1).
- R6. Relleno de `paid_at`: `created_at` si `paid_on` es el día local de `created_at`; si no, la
  medianoche local de `paid_on` (hora fabricada, dicho en el comentario). `include_in_count` relleno =
  `register_session_id is not null`.
- R7. Migración: `lock_timeout 3s`; columnas → relleno → check `not valid` + validate → índices. Down en
  orden inverso; se niega con pendientes.
- R8. Lista de pendientes y barrido con test bajo `appRoleStore` + `inTheThreeCases`.

**Pantalla**
- R9. La hoja de aviso conserva sus dos botones de siempre: principal «Registrar» (pendiente) y
  «Cancelar». «Registrar sin incluir en el arqueo» va como botón `ghost` de 44 px en el cuerpo, aparte
  de los dos. Con `EXPENSE_BEFORE_LAST_CLOSE` el principal es «Registrar sin incluir en el arqueo».
- R10. La casilla «Incluir en el arqueo» va en la misma línea del icono de borrar del renglón de pago
  (`Checkbox minH="44px"`, sin `Field` aparte). En `PayDialog`, igual, en la línea del botón.
- R11. Apertura: sin pendientes no cambia nada. Con pendientes, una sola línea de 44 px sobre el pie:
  «N gastos sin corte · $X»; tocarla despliega el detalle. Para quien tiene el permiso, en esa misma
  línea un `Switch` «Incluirlos (desde las HH:MM)», apagado; con él encendido, «Abrir caja» manda
  `openedAt`. Solo en la apertura (prop del `ContadorDeEfectivo`), nunca en el cierre.
- R12. Detalle del gasto: el badge «en arqueo» se reemplaza por el `Switch` (a la izquierda, lejos del
  monto), con etiqueta «En el arqueo» o «Corte cerrado».
