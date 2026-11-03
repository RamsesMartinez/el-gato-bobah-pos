# Implementation Plan: Renglones excluidos y caja que vende

**Branch**: `049-compras-con-excluidos` | **Date**: 2026-11-01 | **Spec**: [spec.md](spec.md)

## Summary

1. **Excluido = la marca «personal» que hoy se tira, guardada**: `expense_items.excluded`. El servidor
   deja de descartar `Personal` y lo guarda con `excluded = true`. `PlanStockLines` trata un excluido
   como objetivo 0 (si ya había entrado, sale con «corrección de compra»), y `checkLinesFitTotal` no lo
   suma. Un `PATCH` por renglón cambia la marca después de guardar.
2. **Total del ticket**: `expenses.document_total`, que se llena con el total del documento leído (ya
   viene en la extracción) y se muestra en el detalle.
3. **Caja que vende**: `POST /cash-registers/{id}/sells` mueve `is_primary` dentro de una transacción
   con las dos cajas `for update`, ambas cerradas y de la misma sucursal; la nueva queda con arqueo.

## Constitution Check

| Principio | Cómo |
|---|---|
| I | La regla «excluido = objetivo 0» en `domain.PlanStockLines`; la de «se puede pasar la venta» en `domain.CanMoveSales`. |
| II | `ErrRegistersMustBeClosed` (409 `REGISTERS_MUST_BE_CLOSED`), `ErrOtherBranch` (400). |
| III | Un excluido no suma: la suma de incluidos ≤ total sigue validada en el servidor. |
| IV | Tabla de dominio; integración con existencias antes/después, la regla de la 038 con excluidos, el cambio de caja con turnos y en dos empresas (`inTheThreeCases`); migración con dos empresas. |
| V | `PATCH` de renglón por `PermInventoryReceive`; cambio de caja por `PermCashConfigure`; test de 403 de cada uno. |
| VI | Sin «importe que salió de la caja» ni varias cajas vendiendo. |
| VIII | Puerta «más de una caja vendiendo»: no se cruza; mover `is_primary` no asume nada nuevo. «Promociones de plataforma» y «Descuentos» no se tocan. El excluido guarda la evidencia que hoy se pierde. |
| Subidas sin ventana | Aditiva: `excluded default false`, `document_total` nulo. La API vieja sigue descartando «personal» durante la subida. |

## Capas

| Pieza | Capa |
|---|---|
| `0101_excluded_items_and_document_total.sql` | migrations |
| `PlanStockLines` (excluido), `CanMoveSales`, sentinels, acciones `expense.item_excluded`, `cash.selling_register_changed` | domain |
| `insertItems` (guarda excluidos), `checkLinesFitTotal`, `SetItemExcluded`, `MoveSales`, `document_total` en la creación | app |
| `PATCH /expenses/{id}/items/{itemId}`, `POST /cash-registers/{id}/sells` | httpapi |
| Casilla «Excluir» (la marca «personal» existente, con ese nombre), renglones atenuados en el detalle, «Total del ticket», columna «Vende» | web |

## Pantallas (sin pantallas nuevas)

1. **Renglón de compra**: ver R9 (reemplaza esta línea).
2. **Detalle del gasto**: los excluidos atenuados con su importe; para quien recibe mercancía, el mismo
   control «Excluir» por renglón. «Total del ticket $X» bajo el total, solo si existe.
3. **Caja › Administrar**: ver R11 (reemplaza a «Tipo»); el de la caja que vende va encendido y
   deshabilitado; encender otro abre la `ConfirmSheet` existente: «Pasar las ventas a {Caja}».

## Bordes

- B1 dos personas mueven la venta a la vez: candado de las dos filas; el índice de una principal por
  sucursal garantiza una sola.
- B2 excluir un renglón ya recibido con existencias consumidas: la corrección puede dejar existencia
  negativa, igual que hoy una corrección de compra.
- B3 des-excluir que rompe la regla de la 038: 400 con su mensaje.
- B4 regalo excluido: válido.

## Ajustes tras la revisión de arquitectura (2026-11-01) — prevalecen sobre lo anterior

**Base de datos y app**
- R1. Antes de implementar se clasifica cada `is_primary` de `server/queries/` y `server/internal/app/`
  (lista en research.md): «dónde cae la venta de HOY» se queda; «qué caja vendió un turno pasado» deja
  de leer `is_primary`. En particular `CerroLaCajaPrincipal` pasa a «último cierre de un turno que tuvo
  ventas» (turno con pedidos), que no depende de qué caja vende hoy. Test: A vende y cierra, se pasa la
  venta a B, el corte de A, sus reportes y «hasta cuándo se ve un entregado» no cambian.
- R2. Un excluido en `PlanStockLines`: objetivo 0, `received` 0 y no aporta a `pending`. `excluded` en
  `StockLineState` y `ListStockLines`. Ignoran excluidos: `PendingReceiptsOf`, «Por recibir»,
  `ItemsToDeplete` y el recibir-todo de purchases.sql. Test: compra con un excluido no queda pendiente;
  «recibir todo» no mete el excluido.
- R3. El historial de compras y `OldestOfLastPurchases` filtran `not ei.excluded`. `current_cost` no se
  revierte al excluir un renglón ya recibido: mismo techo que el `ponytail` de `NextIngredientCost`.
- R4. `SetItemExcluded`: `LockExpenseForStock` → `lockIngredients` (nowait) → `UPDATE excluded` →
  `checkLinesFitTotal` (incluidos de todo el gasto, igual que `SaveStockLines`) → `applyStockLines`. Un
  renglón sin inventario solo cambia la regla del total. Test: dos PATCH opuestos a la vez dejan 0 o N,
  nunca 2N; `55P03` → 409. Bitácora `expense.item_excluded` con renglón y valor anterior.
  Pruebas bajo `appRoleStore` con `inTheThreeCases`.
- R5. `StockLineInput`, `CreateExpenseItem` y `saveStockLine` llevan `excluded`; si la edición no lo
  manda, se conserva. Test de `SaveStockLines` sobre un gasto con excluido.
- R6. Pasar la venta: `for update` de las dos cajas en orden de `id`; validar bajo candado: destino
  activo, misma sucursal que la principal actual (leída de la fila, nunca `current_branch_id()`), sin
  turno abierto en ninguna; `UPDATE` primero la que deja de vender y luego la nueva con
  `counts_cash = true`. Destino que ya vende → no-op 200. Test de concurrencia contra abrir turno (la
  apertura toma la caja `for update`, ver 047). Mientras la nueva no tenga turno, vender da el 409 «sin
  caja abierta» de siempre. Bitácora `cash.selling_register_changed`.
- R7. Migración con `lock_timeout 3s`; check de `document_total` `not valid` + `validate`; tipo igual a
  `expenses.amount`. El Down se niega si hay excluidos o algún `document_total`: tras la primera marca,
  el rollback de binario es restaurar respaldo (dicho en el encabezado, como 0099).
- R8. Puertas: el renglón de devoluciones atado a la principal por ventana ya estaba cerrado desde
  0080; no cuenta contra este cambio. `document_total` nulo en gastos viejos: no se rellena.

**Pantalla**
- R9. «Excluir» es un control propio por renglón (icono de 44 px en la fila, conserva el artículo); la
  opción «De la casa» sale del Picker. Lo que la lectura detectaba como «personal» llega con «Excluir»
  marcado. Un excluido se atenúa con su nombre e importe.
- R10. Detalle: un excluido es una sola línea atenuada con su importe y «Incluir»; uno incluido lleva el
  icono «Excluir» en la misma fila. Sin stepper ni «Tendrás…» para el excluido.
- R11. Caja › Administrar: la columna «Tipo» se reemplaza por «Vende» (`Switch` en celda de 44 px); el
  de la que vende, encendido y deshabilitado con «Para cambiarla, enciende otra».
