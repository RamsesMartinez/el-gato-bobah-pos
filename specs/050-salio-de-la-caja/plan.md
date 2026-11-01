# Implementation Plan: Importe que salió de la caja

**Branch**: `050-salio-de-la-caja` | **Date**: 2026-11-01 | **Spec**: [spec.md](spec.md)

## Summary

`expense_payments.drawer_extra` (≥ 0): lo que salió del cajón además del importe. La salida de caja de
ese pago (una sola, la de la 048, ligada por `expense_payment_id`) se registra por `amount +
drawer_extra`, con el concepto «… (incluye $X que no es del negocio)». Así siguen valiendo la unique
de una salida por pago, el barrido de pendientes y el «desmarcar borra la salida» de la 048: todo usa
el mismo movimiento. El tope se valida en el servidor: `drawer_extra ≤ suma de renglones excluidos
del gasto`.

## Constitution Check

| Principio | Cómo |
|---|---|
| I | `domain.ValidDrawerExtra(extra, excludedSum)` puro; `app.validatePayments` lo aplica; la salida se arma en un solo helper que usan alta, barrido y casilla. |
| II | `ErrDrawerExtraTooHigh` (400). |
| III | `Round2` + `ValidMoney`; el esperado baja exactamente `amount + drawer_extra`; test que nombra el concepto: `TestLoQueNoEsDelNegocioSaleDelCorteUnaSolaVez`. |
| IV | Tabla de dominio; integración: alta en corte, pendiente barrido al abrir, desmarcar; migración con dos empresas. |
| V | Mismo permiso que el pago; el tope lo pone el servidor. |
| VI | Sin nuevo movimiento ni nueva tabla. |
| VIII | La diferencia queda guardada por pago (hecho irrecuperable). Puerta «varias cajas vendiendo»: no se toca. |
| Subidas sin ventana | Columna con default 0; la API vieja inserta 0 = comportamiento de hoy. |

## Capas

| Pieza | Capa |
|---|---|
| `0102_expense_payment_drawer_extra.sql` | migrations |
| `ValidDrawerExtra`, sentinel | domain |
| `drawerExtra` en `ExpensePaymentInput`; la salida usa `amount + drawer_extra` en alta, barrido y casilla | app |
| `drawerExtra` en el JSON del pago | httpapi |
| Campo «Salió de la caja» en el renglón de pago; 44 px en los `Switch` de Caja › Administrar; «Excluir» con texto y `ConfirmSheet` si ya entró; color del botón de pasar ventas | web |

## Pantallas

- Renglón de pago (`PaymentRow` y `PayDialog`): `Field` «Salió de la caja» de 120 px, en la misma fila,
  solo con efectivo + caja con arqueo + excluidos en el gasto; por omisión el importe.
- Caja › Administrar: cada `Switch` envuelto en una caja de `minH="44px"` que también recibe el toque.
- Detalle: «Excluir» como botón con icono y texto; si el renglón tiene algo recibido, `ConfirmSheet`
  «Sacar del almacén lo que llegó de {artículo}».
- `ConfirmSheet` de pasar ventas con el `colorPalette` de las demás acciones principales.

## Bordes

- B1 `drawerExtra` sin excluidos → 400. B2 negativo/NaN → 400. B3 pendiente barrido → entra con extra.
- B4 desmarcar → se borra la salida completa (ya es una sola). B5 «+1 pago» (`/pay`): el tope son los
  excluidos menos lo ya usado por otros pagos del gasto.

## Ajustes tras la revisión de arquitectura y el análisis (2026-11-01) — prevalecen

- R1. El tope es por GASTO: `ValidDrawerExtra(extra, excludedSum, alreadyUsed)` (dominio). Se valida
  DENTRO de la transacción, tras `LockExpenseForStock(expenseID)`; en el alta, después de `insertItems`.
  Consultas `SumDrawerExtraOfExpense` y suma de renglones `excluded and voided_at is null`.
- R2. Volver a incluir un renglón (`SetItemExcluded`) bajo el mismo candado se rechaza con
  `ErrDrawerExtraTooHigh` si lo excluido quedaría por debajo de la suma de `drawer_extra` del gasto.
- R3. Una sola fuente del monto: trigger `before insert` en `register_cash_movements`: con
  `expense_payment_id` y `kind = 'salida'`, `amount := ep.amount + ep.drawer_extra`. Go deja de sumar.
  La API vieja durante la subida queda cubierta por la misma regla.
- R4. Corte sin faltante falso: `ListCashMovements` trae `drawer_extra` del pago; el desglose cuenta
  `amount − extra` en «Gastos» y `extra` en un renglón propio «No es del negocio» dentro de egresos.
  `ListExpensesBySession`, la lista de pendientes y su vista exponen `drawerExtra`. Test que nombra el
  concepto: `TestLoQueNoEsDelNegocioNoSeCuentaComoGasto`. El resumen diario lo muestra igual.
- R5. `drawerExtra > 0` solo con efectivo + caja con arqueo + `includeInCount` + excluidos disponibles;
  si no, 400.
- R6. `ValidMoney(amount + extra)` en el dominio.
- R7. Migración con `lock_timeout 3s`, `StatementBegin/End` en el `do $$`; Down se niega con algún
  extra; tras el primero, rollback = restaurar respaldo.
- R8. Motivo único: «Parte que no es del negocio».
- R9. Pantalla: «Salió de la caja» va justo después de «Importe»; en `PayDialog` su valor por omisión
  sigue al importe que se va a usar (el total si está vacío). Los tres `Switch` de Caja › Administrar
  con `aria-label` y celda de 44 px; se quita «Para cambiarla, enciende otra» y se pasa a `title`.
  «Excluir» con texto corto; captura a 1024×600 para revisar que la fila no desborde.
- R10. Puerta: el movimiento único por pago no permite atribuir la diferencia a una persona; aceptado
  (principio VI).
