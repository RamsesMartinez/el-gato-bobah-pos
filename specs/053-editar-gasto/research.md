# Research: Editar un gasto ya guardado (entrega 1)

Lo que ya existe y se reusa (medido en el código el 2026-11-01):

- `expense_items` ya tiene `voided_at` (quitar sin borrar), `qty_received`, `not_coming_qty` y
  `applied_qty_base` (lo que el renglón ya sumó = suma de sus movimientos). Spec 038.
- `domain.PlanStockLines` + `app.applyStockLines` mueven el almacén por diferencia, actualizan el
  costo del insumo (`applyPurchaseCost`) y fijan `received_at`. `reverseLine` deja un renglón en cero
  con un movimiento contrario. Todo con el gasto tomado (`LockExpenseForStock`) y los insumos sin
  esperar (`lockIngredients`, 409 si una venta los tiene).
- `stock_movements` ya guarda `expense_id`, `expense_item_id` y `stock_count_id`. No tiene fecha de
  negocio: solo `created_at` (el instante de la captura).
- Un pago nuevo ya decide caja, corte y pendiente (`AddExpensePayment`, specs 047/048).
- La vista previa de la pantalla de detalle (hoy «Tendrás X») la calcula el cliente.

## R1. Vista previa = la misma transacción, revertida

- **Decision**: una sola función de servicio (`editExpense`) hace todo el trabajo dentro de
  `store.WithTx`. Al final lee el estado después (existencias, costo, importe, estado de pago) y arma
  la vista previa. Con `preview=true` devuelve un error centinela que revierte la transacción y
  entrega la vista previa; con `preview=false` confirma.
- **Rationale**: FR-025 y SC-002 piden que lo confirmado sea exactamente lo previsualizado. Dos
  implementaciones (una que calcula, otra que escribe) divergen con el tiempo; la constitución ya lo
  aprendió con `/quote` y `/pay` (misma función). Revertir garantiza la igualdad por construcción y
  cubre de paso el costo por promedio y los triggers de existencias.
- **Alternatives considered**: (a) calcular en `domain` sin escribir: duplica el trigger de
  existencias y el costeo; (b) calcular en el cliente como «Tendrás X»: viola FR-025.
- **Costo aceptado**: la vista previa toma los mismos candados (sin esperar) que la confirmación; si
  una venta tiene el insumo, la vista previa también responde «vuelve a intentar». Consume ids de
  identidad que se revierten (sin efecto visible).

## R2. Lo confirmado es lo previsualizado: huella

- **Decision**: la vista previa trae una `fingerprint` (sha256 del JSON canónico de sus efectos y de
  las decisiones usadas). La confirmación manda la huella que vio; el servidor recalcula dentro de la
  transacción y, si la huella difiere, revierte y responde 409 `EXPENSE_EDIT_STALE`. La pantalla pide
  otra vista previa y la muestra.
- **Rationale**: FR-029. Detecta cualquier cambio que altere el resultado (otra edición, una venta
  que movió existencias, un cambio de costo) sin enumerar cada causa, y no rechaza cambios
  irrelevantes.
- **Alternatives considered**: `seenApplied` por renglón (lo de la 038): no ve ventas ni costos.

## R3. Fecha de entrega en el movimiento

- **Decision**: columna nueva `stock_movements.effective_on date` (nullable). Un trigger `before
  insert` la llena cuando llega nula en un movimiento `compra` o en uno con `expense_id`, con el día
  de `now()` en la zona de la empresa (`business_settings.timezone`, `America/Mexico_City` si no hay
  fila): así la API vieja durante la subida también deja fecha. La 0103 rellena los `compra`
  existentes con el día de su `created_at` en la zona vigente (revisión M3: si la zona cambia
  después, un nulo se reinterpretaría y el hecho quedaría cerrado). La API nueva la manda explícita
  en las entregas con otra fecha y en los contra-movimientos.
- **Rationale**: el issue pide el historial «de los movimientos de compra por renglón, sin tabla
  nueva». `created_at` es cuándo se capturó, y el contra-movimiento tiene que llevar la fecha de la
  entrega original. Reescribir `created_at` rompería el orden por id que usa el conteo (spec 039) y
  los reportes por periodo.
- **Alternatives considered**: tabla `expense_receipts`: el issue la descarta; poner la fecha en
  `reason`: no se puede consultar.
- **Quién la lee en esta entrega**: el historial de entregas del renglón, la fecha del
  contra-movimiento, la regla de conteo posterior (R5) y `received_at`. Los reportes por periodo
  siguen con `created_at`; cambiarlos es otra decisión.

## R4. Importe

- **Decision** (FR-009): `nuevo = max(anterior − borrados − ajustados + de_más, suma_de_vivos_incluidos)`.
  `borrados` = importes de renglones no excluidos quitados; `ajustados` = lo que bajó un renglón por
  «Ajustar el importe a lo recibido»; `de_más` = lo pagado por lo que llegó de más (FR-021, regla que
  ya sube el total). Un resultado ≤ 0 se rechaza: el gasto no puede quedar en $0 (anularlo es de la
  entrega 2).
- **Rationale**: el caso del 1-nov ($67.50 sin desglosar + dos renglones de $67.50 → $67.50), la
  sustitución (borrar y agregar el mismo importe → sin cambio) y el borrado simple (baja por su
  importe) salen de la misma fórmula sin preguntar.
- **Estado de pago**: solo si el importe cambió. `pagado ≥ nuevo` → `pagada` (si estaba pendiente,
  `paid_at` = ahora y `paid_by` = quien edita); si no, `pendiente` con `paid_at` y `paid_by` en nulo.
  Una `pagada` **sin ningún pago** (de antes de la 0029) no se degrada (revisión M5). El check
  `expenses_paid_requires_date` se respeta.
- **Anular**: como un gasto pagado ahora puede volver a pendiente, `CanCancelExpense` y la consulta
  `CancelExpense` exigen además que no tenga pagos (409); anular con pagos es la entrega 2
  (revisión A1).
- **Spec 050**: un importe nuevo cambia el tope de `drawer_extra`; se valida con
  `excludedStillCovers` dentro de la misma transacción (400 `DRAWER_EXTRA_TOO_HIGH`).

## R5. Conteo posterior

- **Decision**: un movimiento con fecha de entrega `D` y registrado en el instante `R` (ahora para uno
  nuevo; el `created_at` del original para un contra-movimiento) es anterior a un conteo si existe un
  renglón de conteo **cerrado** del mismo insumo con `captured_at > least(R, fin del día D)`. Se toma
  el conteo más reciente que cumpla. Solo conteos de la sucursal del movimiento
  (`current_branch_id()`, revisión M1).
- **Rationale**: el conteo vio la mercancía física si se capturó después del día en que llegó, y vio
  el movimiento si se capturó después de registrarlo; cualquiera de los dos basta para que mover otra
  vez la cuente doble. Con `D` = hoy y un movimiento nuevo, `least` es ahora y ningún conteo cerrado
  es posterior: recibir hoy nunca pregunta.
- **Solo insumos**: los conteos son de insumos (`stock_count_lines.ingredient_id`); un producto con
  existencia propia no pregunta.
- **Un conteo abierto no cuenta** (spec 038: ajusta contra lo que había al capturar).

## R6. «No mover» = movimiento de compensación

- **Decision**: con «No mover», el movimiento del renglón se registra igual (así
  `applied_qty_base` sigue siendo la suma de los movimientos del renglón) y se agrega otro de tipo
  `ajuste` por la cantidad contraria, con `stock_count_id` = el conteo posterior, `expense_id` = el
  gasto, **sin** `expense_item_id`, la misma `effective_on`, el motivo «ya contado en el conteo N» y
  `branch_id` **explícito**: el del conteo (revisión A2; el trigger tomaría la de la empresa y con
  dos sucursales truena).
- **Rationale**: el libro no pierde ningún hecho: entró (o salió) lo del renglón y el conteo ya lo
  había reflejado. Sin `expense_item_id`, el historial de precios de compra (`ingredients_admin.sql`,
  que filtra `compra` y une por renglón) y lo aplicado no lo ven. Ninguna consulta lee
  `stock_count_id` hoy (solo se inserta en `stock_counts.sql`), así que el reporte del conteo cerrado
  no cambia.
- **Alternatives considered**: no insertar nada con «No mover»: rompe el invariante de lo aplicado y
  el siguiente `PlanStockLines` lo «corregiría».

## R7. Contra-movimiento por entrega

- **Decision**: borrar un renglón recibido inserta un contra-movimiento por cada día de entrega con
  saldo positivo (suma de los movimientos `compra` del renglón agrupados por `effective_on`), con esa
  fecha. Antes, `domain.PlanDeliveryReversal` netea los días con saldo negativo (una corrección de la
  038 después de la entrega) contra los días positivos más recientes: un borrado nunca emite una
  cantidad positiva (revisión M2) y la suma sigue siendo exactamente `−applied_qty_base`.
- **Rationale**: FR-007 y el edge case de entregas parciales; la decisión de conteo se evalúa por
  cada grupo (su `D` y su `R` = el último `created_at` del grupo) y se toma por insumo.

## R8. Dos rutas, un motor, dos permisos

- **Decision**: `POST /expenses/{id}/edit` (permiso nuevo `expenses.edit`: encabezado, agregar, borrar)
  y `POST /expenses/{id}/deliveries` (permiso `inventory.receive`: entregas). Las dos con
  `?preview=1` y el mismo servicio. Agregar un renglón con «Ya llegó» exige además
  `inventory.receive`: lo decide `domain.EditNeedsReceive` y el handler lo verifica con el mismo
  resolvedor de permisos del router.
- **Rationale**: FR-031; quien recibe (gerente, hoy) no edita importes, y el issue fija «por omisión
  admin» para editar.
- `expenses.edit` se agrega a `AllPermissions` (admin) y no a gerente; su mensaje en
  `PermissionDeniedMessage` y su espejo en `web/src/app/permissions.ts`.

## R9. Lo que se queda y lo que se reemplaza en la pantalla

- **Se queda**: «Corregir lo que llegó» (SaveStockLines), «Excluir/Incluir», el interruptor de arqueo
  de cada pago y «Anular compra» (solo pendientes, como hoy).
- **Se reemplaza en el detalle**: «Agregar un artículo que faltó» (`AddArrivedItem`) por «Agregar
  renglón» con «Ya llegó». El endpoint viejo se queda (lo usa «Por recibir» → «Llegó algo más» y la
  tableta con la versión anterior durante la subida).
- **Por recibir**: su botón «Recibir» abre la hoja de entrega nueva del gasto (con fecha y vista
  previa). `ReceiveItem`/`MarkNotComing` se quedan para la versión anterior y graban `effective_on` =
  hoy.

## R10. Bitácora

- **Decision**: dos acciones. `expense.edited` (nueva): antes/después de cada campo del encabezado,
  renglones agregados y quitados, importe y estado, decisiones de conteo y la de dinero.
  `expense.received` (ya existe): ahora con la fecha de cada renglón, «pendiente/ajustar» y las
  decisiones de conteo. Las dos dentro de la transacción. `expense.edited` se agrega a la lista blanca
  de `domain/audit.go`, a `labels.ts` y a un test que la produce (`TestEveryAuditActionIsProduced`).

## R11. Coexistencia con la spec 052

- La 052 (costo de opciones e insumos) puede tocar `applyPurchaseCost`/`recomputeCosts`. Esta spec
  no cambia la regla de costo: solo la llama a través de `applyStockLines`. Antes del PR se trae
  develop en local y se resuelve; la vista previa del grupo Costos lee el costo antes/después, así que
  refleja la regla que quede.
