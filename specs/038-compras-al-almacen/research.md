# Research: Compras al almacén

Decisiones de diseño del 038, con lo que ya existe medido en el código (14-oct). Las rutas y
símbolos citados son de `develop` en `c33f50e`.

## R0. Lo que ya existe y por qué no suma

- Un gasto ya puede llevar renglones de mercancía (`expense_items`, 0028) con insumo, cantidad en la
  unidad de compra, `pack_qty_in_base` y `qty_received`. Al **recibir** (`ExpensesService.ReceiveExpense`,
  `server/internal/app/expenses.go:251`), cada renglón con `qty_received > 0` deja un movimiento
  `compra` (`InsertPurchaseMovement`) convertido con `domain.BaseQty`.
- La lectura de tickets (`/expenses/parse-doc`) propone renglones y los formatos aprendidos por
  proveedor (`supplier_items`) llenan insumo y tamaño de paquete.
- **El defecto (encontrado al planear):** `ExpenseDialog.tsx:110` arranca con «recibida» en `true`
  y manda `receivedAt` aunque el gasto no tenga renglones. El servidor lo guarda, no hay nada que
  sumar, y como recibir es idempotente por `received_at`, ese gasto ya no puede recibir nada
  después. Medido en la copia de producción: todos los gastos recibidos, ninguno con un renglón de
  insumo, ningún movimiento `compra`.
- Un gasto recibido no se puede corregir ni anular (`CancelExpense` responde 409): el almacén nunca
  se revierte. Recibir no toca `ingredients.current_cost` ni recalcula recetas; el costo de una
  materia prima solo lo escribió la importación de FUDO.

## R1. «Recibida» solo con mercancía

- **Decisión:** el servidor guarda `received_at` solo si el gasto lleva al menos un renglón de
  almacén; sin renglones lo deja en nulo aunque llegue la fecha. La pantalla muestra «Ya llegó» solo
  con renglones (ya lo hace) y deja de mandar la fecha sin ellos.
- **Por qué en el servidor:** una pestaña vieja o un script seguirían marcando gastos vacíos como
  recibidos. Ignorar el campo (y no rechazarlo) evita romper la captura de un gasto de luz desde una
  tableta con la versión anterior.
- **Alternativa descartada:** rechazar con 400. Rompe a quien opera por un dato que no decide nada.
- **Lo heredado no se toca** (FR-007): los gastos ya marcados quedan como están; su mercancía, si
  importa, la corrige el conteo.

## R2. Corregir lo que llegó: una sola operación por diferencia

- **Decisión:** `PUT /expenses/{id}/stock-lines` recibe el conjunto **deseado** de renglones de
  almacén del gasto (agregar, cambiar cantidad, presentación o insumo, quitar). El servidor, con el
  gasto bloqueado (`FOR UPDATE`), calcula por renglón lo ya aplicado (suma de sus movimientos
  `compra` por `expense_item_id`) contra lo deseado en unidad base, e inserta **solo la diferencia**
  como movimiento `compra` (positivo o negativo) con el motivo «corrección de compra».
- Un renglón quitado no se borra: se marca `voided_at` y su diferencia es `−aplicado`. Mandar dos
  veces lo mismo deja diferencia cero: es idempotente sin llave extra.
- **Anular un gasto recibido** pasa por lo mismo con el conjunto vacío y después por la anulación
  de siempre (`CancelExpense` deja de responder 409 cuando todo quedó revertido).
- Cada renglón existente viaja con `seenApplied` (lo aplicado que mostró la pantalla, en unidad
  base); si ya no coincide con lo aplicado bajo el bloqueo, 409. `expenses` no tiene `updated_at`,
  y la cifra aplicada es justo lo que una edición simultánea cambiaría (mismo patrón que
  `seenProductOnHand` en el 037).
- **Alternativas descartadas:** editar y borrar movimientos (la bitácora del almacén no se reescribe);
  un endpoint por tipo de cambio (tres caminos que tendrían que coincidir).
- Necesita `stock_movements.expense_item_id`: hoy el movimiento solo guarda `expense_id`, y con dos
  renglones del mismo insumo no se sabe cuál corregir.

## R3. Presentaciones: una tabla por insumo, copiada al renglón

- **Decisión:** `ingredient_presentations` (empresa, insumo, nombre, `qty_in_base` en la unidad base
  del insumo, `active`). Un renglón comprado en presentación guarda `presentation_id` como referencia
  y **copia** la equivalencia en `pack_qty_in_base` con unidad «pieza»: `domain.BaseQty` ya convierte
  pieza → base con ese dato. Cambiar la presentación no toca compras anteriores (FR-009) porque el
  renglón ya tiene su copia.
- La equivalencia siempre está en la unidad base del insumo, así que no hay conversión entre tipos
  que adivinar (FR-010): «caja = 12» en un insumo en litros es 12 L.
- Nombre único por insumo sin distinguir mayúsculas ni acentos (misma normalización que los
  insumos del 037).
- **Los formatos aprendidos por proveedor siguen**: si el proveedor ya tiene un tamaño aprendido, la
  captura lo propone; la presentación es lo que se elige a mano.
- **Alternativa descartada:** presentaciones por proveedor. El requerimiento las pide por insumo, y
  por proveedor ya existe lo aprendido.

## R4. Costo del insumo al comprar

- **Decisión (dueño, 14-oct):** por omisión, el costo de la última compra; cada empresa puede elegir
  promedio ponderado en su configuración (`business_settings.costing_method`, `last` | `weighted`).
- Fórmula en `domain.NextIngredientCost(method, onHand, cost, qty, unitCost)`:
  - `last`: `unitCost`.
  - `weighted`: `(max(onHand,0)·cost + qty·unitCost) / (max(onHand,0) + qty)`; con `onHand ≤ 0` es
    `unitCost` (FR-006b: un negativo heredado no puede dar un costo absurdo).
  - `onHand` es la suma de todas las sucursales: el costo es de la empresa, no de la sucursal.
- **Un renglón con importe 0** (regalo, muestra) no cambia el costo: un costo en 0 es «sin
  capturar» desde el 037.
- Después de cambiar costos, `CostingService.RecomputeAll` propaga a insumos preparados, productos
  y extras, en la misma transacción. Se bloquea la fila del insumo (`FOR UPDATE`) para que dos
  compras simultáneas no promedien sobre el mismo costo viejo.
- **Límite aceptado (`ponytail:`):** una corrección o una reversión no recalcula el costo hacia
  atrás; el costo se arregla con la siguiente compra. Reconstruir el promedio exigiría reordenar la
  historia de compras.
- Cambiar el método no reescribe nada: las ventas guardan su costo (snapshot) y el costo actual
  cambia en la siguiente compra.

## R5. Conteo: por diferencia contra lo que había al capturar

- **Decisión:** `stock_counts` (empresa, sucursal, estado `open` | `closed` | `discarded`, quién y
  cuándo abrió y cerró) y `stock_count_lines` (conteo, insumo, `counted_qty` en unidad base,
  `system_qty` al capturar, quién y cuándo). Un solo conteo abierto por sucursal (índice único
  parcial).
- Cada cifra se guarda en el servidor al capturarla (FR-014): sobrevive a recargar y a cambiar de
  tableta.
- **Al cerrar**, cada renglón inserta un `ajuste` de `counted_qty − system_qty` con
  `stock_count_id`. Como lo vendido después de capturar ya restó de lo guardado, el resultado es
  «lo contado menos lo vendido desde que se contó» sin bloquear ventas durante el conteo.
- Recapturar un renglón actualiza `counted_qty` y `system_qty` (la nueva lectura manda).
- Los no contados no se tocan (dueño, 14-oct); al cerrar se dice cuántos quedaron sin contar.
- La captura es en unidad base; la pantalla ayuda con las presentaciones (3 cajas + 2 L), pero al
  servidor llega la cifra en base.
- **Alternativa descartada:** fijar `on_hand` al cerrar. El disparador de existencias solo suma
  deltas, y fijar pisaría las ventas que entraron mientras se contaba.

## R6. Permisos

- Nuevos: `inventory.receive` (agregar, recibir y corregir mercancía de un gasto),
  `inventory.configure` (presentaciones y método de costeo) e `inventory.count` (contar). Admin y
  gerente por omisión.
- Las rutas nuevas y las que se tocan (`/receive`, `/stock-lines`, presentaciones, conteos,
  método de costeo) se registran **fuera** de los grupos con `RequireRole`, solo con su permiso
  (mismo criterio que el 037).
- **`POST /expenses` se queda en su grupo con rol:** la captura del gasto no cambia de dueño. Sus
  renglones de mercancía siguen permitidos a quien hoy captura gastos (admin y gerente, los mismos
  que tienen `inventory.receive`). Se migra con el issue #61.

## R7. Bitácora

- `expense.received` (recibir) y `expense.stock_corrected` (corregir o revertir), grupo «Caja»
  junto a los `expense.*` de hoy; `stock.counted` (cerrar un conteo), grupo «Cancelaciones» junto a
  `stock.written_off`; `settings.costing_method`, grupo «Precios». Ningún grupo nuevo: la rejilla de
  la bitácora es una disposición de tableta (037).

## R8. Relación con Salud del inventario (037)

- «Lo guardado en negativo» abre resumido mientras no exista un movimiento `compra`: con la
  primera compra recibida deja de estarlo, sin cambios.
- «Insumos sin compras registradas» busca un `compra` del insumo. Con las reversiones negativas,
  un gasto anulado seguiría contando como compra: la consulta pasa a exigir que la **suma** de
  `compra` del insumo en el periodo sea mayor que cero.
- Un conteo es `ajuste`, no `compra`: no cuenta como compra, a propósito.

## R9. Sucursal

- Los movimientos de compra y de conteo toman la sucursal por el disparador de `stock_movements`
  (`branch_for_company`); con dos sucursales y sin elegir, 409 `BRANCH_AMBIGUOUS`, como el resto. El
  conteo guarda su sucursal explícita al abrirse.
- `expenses.branch_id` sigue nulo como hoy: decidir la sucursal del gasto es del selector de
  sucursal, no de esta feature.

## R9b. Puertas (principio VIII)

- **¿Se puede agregar después al mismo costo?** El renglón de compra con su equivalencia copiada,
  `expense_item_id` y `stock_count_id` en el movimiento: no. Sin ellos, una compra corregida o un
  ajuste de conteo no se pueden rastrear después (SC-003). Se construyen hoy.
- El valor del inventario y el kárdex (fuera de alcance) quedan posibles: cada entrada guarda su
  costo unitario.
