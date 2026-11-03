# Tasks: Compras al almacén

**Input**: [spec.md](spec.md), [plan.md](plan.md), [research.md](research.md), [data-model.md](data-model.md), [contracts/api.md](contracts/api.md), [quickstart.md](quickstart.md)

**Tests**: obligatorios (constitución IV, TDD). Cada test se escribe antes que su código y se ve fallar por la razón correcta. Toda consulta nueva sobre una tabla de empresa lleva su test bajo `appRoleStore` con `inTheThreeCases`.

**Formato**: `[ID] [P?] [Story] Descripción`. `[P]` = archivo distinto y sin dependencia pendiente. Las historias siguen la numeración del spec: US1 (comprar), US1b (por recibir), US2 (empaques), US3 (conteo).

---

## Phase 1: Setup

- [X] T001 Confirmar la infraestructura del worktree (`make worktree-env n=37` ya hecho; `make start`) y restaurar la copia de producción con `make db-restaurar`; anotar en research.md cuántos `expense_items` tienen importe 0 (decide si el check de la 0092 entra `not valid`)
- [X] T002 Crear `server/queries/purchases.sql` y `server/queries/stock_counts.sql` con su encabezado y confirmar que `make sqlc` corre limpio

---

## Phase 2: Foundational (bloquea a todas las historias)

### Tests (primero, en rojo)

- [X] T003 [P] Test de integración de la migración 0091 (`longitud` existe en `unit_kind`; su Down lo deja y no truena) y de la 0092 sobre la copia restaurada con dos empresas: índices `ingredients_tenant_key` y `expense_items_tenant_key`; FK compuestas que rechazan cruzar empresas como owner (`presentation_id`, `count_id`, `expense_item_id`, `stock_count_id`); trigger de presentaciones (pieza entera, aproximado, nombre de unidad); checks de `is_gift`, `not_coming_qty`, `count_place`, `stock_item_label`; el Down se niega con datos, en `server/internal/integration/migration_0092_test.go`
- [X] T004 [P] Test de que las tres tablas nuevas entran a `TestEveryCompanyTableIsIsolated` (RLS, política con `nullif`, grants de `gatobobah_app` sin `delete`), en `server/internal/integration/rls_all_tables_test.go` (existente)
- [X] T005 [P] Test table-driven de `BaseQty` con longitud (m → cm, cm → cm, m → kg rechazado) y de `PresentationQty` (entero en pieza, aproximado solo entre tipos distintos, nombre igual a una unidad rechazado salvo aproximado), en `server/internal/domain/purchaseqty_test.go`
- [X] T006 [P] Test table-driven de `NextIngredientCost` (última compra; ponderado con existencia positiva; ponderado con existencia 0 o negativa toma la compra; importe 0 o regalo no cambia el costo), en `server/internal/domain/purchases_test.go`
- [X] T007 [P] Test de que `inventory.receive`, `inventory.configure` e `inventory.count` los tienen admin y gerente y no cajero ni cocina, en `server/internal/domain/permission_test.go`

### Implementación

- [X] T008 Migración `server/migrations/0091_unit_kind_length.sql` (solo `alter type unit_kind add value 'longitud'`) hasta que su parte de T003 pase
- [X] T009 Migración `server/migrations/0092_purchases_into_stock.sql` según data-model.md (unidades `cm`/`m`, índices de llave por empresa, tablas, columnas, checks, trigger `ingredient_presentations_before_write`, índices, RLS y grants, Down que se niega con datos) hasta que T003 y T004 pasen; `make sqlc`
- [X] T010 [P] `BaseQty` para longitud y `PresentationQty` en `server/internal/domain/purchaseqty.go` hasta que T005 pase
- [X] T011 [P] `NextIngredientCost` y `CostingMethod` en `server/internal/domain/purchases.go` hasta que T006 pase
- [X] T012 [P] Permisos `PermInventoryReceive`, `PermInventoryConfigure`, `PermInventoryCount` en `server/internal/domain/permission.go` y su espejo en `web/src/app/permissions.ts` hasta que T007 pase
- [X] T013 [P] Acciones `expense.received`, `expense.stock_corrected` (grupo Caja), `stock.counted` (grupo Cancelaciones), `settings.costing_method` (grupo Precios) en `server/internal/domain/audit.go` y sus etiquetas en `web/src/shared/audit/labels.ts`; cada una con su test productor (lo exige `TestEveryAuditActionIsProduced`) en la historia que la escribe

**Checkpoint**: esquema, dominio base y permisos listos.

---

## Phase 3: User Story 2 — Empaques por insumo (P1)

Va antes de US1 porque la compra en empaque los usa.

**Independent Test**: crear «Caja = 12 L» para la leche y verla ofrecida al capturar; cambiar la caja a 10 y ver que una compra vieja conserva 12.

### Tests (primero, en rojo)

- [X] T014 [P] [US2] Test de integración bajo `appRoleStore` con `inTheThreeCases` de las consultas de presentaciones (listar, crear, editar, desactivar) con dos empresas; nombre repetido sin acentos ni mayúsculas → 409; aproximado por kilo en un insumo por pieza; piezas con decimales → 400 (el aviso de «otro tamaño de pieza es otro insumo» lo da la pantalla: el servidor no tiene cómo saber que dos empaques son tamaños distintos); en `server/internal/integration/presentations_test.go`
- [X] T015 [P] [US2] Test del handler por el router real: cajero → 403 en escribir; `qtyInBase` 0, negativo o con decimales en pieza → 400; presentación de otro insumo → 400; JSON crudo con arreglo vacío `[]`, en `server/internal/integration/presentations_http_test.go`
- [X] T016 [P] [US2] Tests de vitest de la columna «Cómo llega» (maqueta «Cómo llega un insumo», opción B): empaques con su contenido, «Ya no llega en: …», «+ Agregar» con nombres que la empresa ya usa y «Otro…», número **y** unidad con la equivalencia antes de guardar (caso borde 1), aproximado marcado «≈», sin `<select>` nativo, en `web/src/features/backoffice/PresentationsSheet.test.tsx`

### Implementación

- [X] T017 [US2] Consultas de presentaciones en `server/queries/ingredients.sql`; `make sqlc`
- [X] T018 [US2] Servicio de empaques (crear, editar, desactivar) en `server/internal/app/presentations.go`
- [X] T019 [US2] Rutas `GET/POST /stock/ingredients/{id}/presentations` y `PUT /stock/presentations/{id}` con `RequirePermission(PermInventoryConfigure)` para escribir, fuera de los grupos con rol, en `server/internal/httpapi/handlers_backoffice.go` y `router.go`, hasta que T015 pase
- [X] T020 [P] [US2] Cliente y tipos en `web/src/api/backoffice.ts`
- [X] T021 [US2] Columna «Cómo llega» y hoja `PresentationsSheet` según plan.md › Maquetas, fila «Cómo llega un insumo» (B), en `web/src/features/backoffice/StockPage.tsx` y `PresentationsSheet.tsx`, hasta que T016 pase

---

## Phase 4: User Story 1 — Comprar y que sume (P1) — MVP

**Independent Test**: un gasto con «3 cajas de leche» que llegó suma 36 L, fija el costo y saca la leche de «Insumos sin compras registradas»; un gasto de luz no queda «recibido».

### Tests (primero, en rojo)

- [X] T022 [US1] Test de regresión del defecto encontrado al planear: `CreateExpense` con `receivedAt` y sin renglones de almacén deja `received_at` nulo (hoy lo guarda), visto en rojo antes del arreglo, en `server/internal/integration/purchases_test.go`
- [X] T023 [US1] Test de regresión del segundo defecto: crear un insumo con una unidad que no es la base de su tipo (kg) → 400; con g, ml, pieza o cm → ok; y un test de rutas que falla si aparece una ruta que cambie `base_unit_id` de un insumo (regla 4: hoy ninguna lo hace), en `server/internal/integration/purchases_test.go`
- [X] T024 [US1] Test de integración de la compra bajo `appRoleStore` con `inTheThreeCases`: renglón en empaque suma en base con movimiento ligado a gasto y renglón; `applied_qty_base` igual a la suma de sus movimientos; cada movimiento guarda quién lo hizo (SC-003); renglones no de almacén no tocan nada; reglas de dinero (importe > 0 salvo regalo, suma ≤ total → 400 desde el servidor); categoría y total obligatorios; costo por última compra y por ponderado con `RecomputeAll` propagando a recetas; gasto heredado sin renglones queda inerte (FR-007), en `server/internal/integration/purchases_test.go`
- [X] T025 [US1] Test de corrección por diferencia (`PUT /expenses/{id}/stock-lines`): cambiar cantidad, empaque o insumo, quitar un renglón (`voided_at`), reenviar lo mismo deja diferencia cero, `seenApplied` viejo → 409; anular un recibido revierte y nunca borra un movimiento (SC-004); el total no cambia, en `server/internal/integration/purchases_test.go`
- [X] T026 [US1] Test de concurrencia: dos correcciones del mismo renglón → una 409; dos compras del mismo insumo con promedio ponderado no pierden una; una corrección que toca dos insumos y otra que los toca en orden inverso no se bloquean entre sí (orden por insumo); una venta con el insumo tomado → 409 por `NOWAIT`, nunca espera, en `server/internal/integration/purchases_test.go`
- [X] T027 [US1] Test de que los movimientos de compra llevan la sucursal explícita y con dos sucursales sin selector responde `BRANCH_AMBIGUOUS` en vez de adivinar, en `server/internal/integration/purchases_test.go`
- [X] T028 [P] [US1] Test del handler por el router real: cajero → 403 en recibir, corregir y método de costeo; `PUT /settings/inventory` con método desconocido o palabra vacía o de más de 20 → 400; JSON crudo con arreglos `[]`, en `server/internal/integration/purchases_http_test.go`
- [X] T029 [P] [US1] Test de que «Insumos sin compras registradas» y la apertura resumida de «Lo guardado en negativo» del 037 usan la suma positiva de `compra` (una compra anulada ya no cuenta en ninguna de las dos), en `server/internal/integration/inventory_health_test.go`
- [X] T030 [P] [US1] Tests de vitest de la captura (maqueta «Captura de una compra», opción C): primero el ticket y «Capturar sin ticket»; encabezado vacío al empezar con categoría y proveedor de la empresa y «+ Nuevo»; no guarda sin categoría ni total; renglón editable con estado «Llegó / Llega después»; dudoso arriba y «N renglones sin confirmar» en el pie (FR-005a); partir un renglón surtido; «+ Otro empaque» con unidad y equivalencia; crear insumo pregunta en qué se mide (cuatro tipos); total distinto del ticket pregunta (FR-004c); quitar un renglón capturado pide confirmación; la palabra configurable; categoría y proveedor nuevos se crean con las rutas existentes (`POST /expense-categories`, `POST /suppliers`); cucharada, cucharadita y onza no se ofrecen; cantidades en kg o L desde 1,000 (regla 10); el empaque elegido gana sobre el tamaño aprendido del proveedor (regla 11); sin `<select>` ni `type=date`, en `web/src/features/backoffice/ExpenseDialog.test.tsx` (nuevo)
- [X] T031 [P] [US1] Tests de vitest de «Corregir lo que llegó» (maqueta B): tres estados por renglón, «Tendrás X guardados» pedido al servidor antes de habilitar Guardar, resumen de diferencia, «Anular compra» con confirmación y estado anulado visible, en `web/src/features/backoffice/ExpenseDetailDialog.test.tsx` (nuevo)

### Implementación

- [X] T032 [US1] Consultas en `server/queries/purchases.sql`: bloquear gasto y renglones (`FOR UPDATE`), insertar movimiento con `expense_item_id` y sucursal explícita, actualizar `applied_qty_base`, bloquear insumo para costo (`FOR NO KEY UPDATE NOWAIT`, y sus existencias con `NOWAIT` desde T073), suma de existencias para ponderado; `make sqlc`
- [X] T033 [US1] `domain.PlanStockLines` (deseado contra aplicado, ordenado por insumo) con su test table-driven en `server/internal/domain/purchases.go` y `purchases_test.go`
- [X] T034 [US1] En `server/internal/app/expenses.go`: «recibido» solo con mercancía (T022), reglas de dinero, costo con `ApplyPurchaseCost` + `RecomputeAll` en la misma tx (`server/internal/app/costing.go`), `SaveStockLines`, anular recibido por reversión, eventos `expense.received` y `expense.stock_corrected`, hasta que T024–T027 pasen
- [X] T035 [US1] Alta de insumo que exige la unidad base de su tipo en `server/internal/app/articles.go` hasta que T023 pase
- [X] T036 [US1] Método de costeo y palabra configurable: `PUT /settings/inventory` en `server/internal/app/settings.go` y su handler, `GET /business-settings` los incluye, evento `settings.costing_method`
- [X] T037 [US1] Rutas `POST /expenses/{id}/receive` (pasa a `RequirePermission(PermInventoryReceive)`), `PUT /expenses/{id}/stock-lines`, `PUT /settings/inventory`, fuera de los grupos con rol, en `server/internal/httpapi/router.go` y `handlers_backoffice.go`, hasta que T028 pase
- [X] T038 [US1] «Sin compras» y `HealthAnyPurchase` por suma positiva en `server/queries/inventory_health.sql` hasta que T029 pase; `make sqlc`
- [X] T039 [P] [US1] Cliente y tipos (renglón con estado, `seenApplied`, `isGift`, `costingMethod`, `stockItemLabel`) en `web/src/api/backoffice.ts`. `lastClosedCount()` y el aviso de gasto con fecha anterior al último conteo pasaron a T061: dependen de las rutas de conteo
- [X] T040 [US1] Captura según plan.md › Maquetas, fila «Captura de una compra» (C), en `web/src/features/backoffice/ExpenseDialog.tsx` y `expenseDraft.ts`, hasta que T030 pase
- [X] T041 [US1] «Corregir lo que llegó» según plan.md › Maquetas, fila «Corregir lo que llegó» (B), en `web/src/features/backoffice/ExpenseDetailDialog.tsx`, hasta que T031 pase
- [X] T042 [US1] Renglón de costeo y palabra en `web/src/features/admin/BusinessSettingsPage.tsx` (+ test)

**Checkpoint**: US2 + US1 = comprar y que sume, el MVP.

---

## Phase 5: User Story 1b — Por recibir (P1)

**Independent Test**: 3 cajas «Llega después» no suman y aparecen en la lista; llegan 2 (suman 24 L, queda 1); 1 «ya no va a llegar» sale sin tocar el total; llegan 4 con la cuarta pagada y el total sube.

### Tests (primero, en rojo)

- [X] T043 [US1b] Test de integración bajo `appRoleStore` con `inTheThreeCases` de `GET /stock/pending-receipts` y de recibir por partes: suma con la fecha del día; de más con `extraAmount` sube cantidad, importe y total; «ya no va a llegar» parcial con `not_coming_qty`; «Llegó algo más» pagado y regalo (no cambia total ni costo); `received_at` vuelve a nulo al agregar un pendiente y se fija cuando ya no queda nada; dos tabletas → una 409 con lo ya recibido; con dos sucursales y sin selector la lista responde `BRANCH_AMBIGUOUS` (las compras todavía no guardan sucursal, R9: no se adivina de cuál es), en `server/internal/integration/pending_receipts_test.go`
- [X] T044 [US1b] Test de integración bajo `appRoleStore` con `inTheThreeCases` del filtro «Con pendientes» en la lista de gastos: `ListExpenses` y `CountExpenses` con el mismo `where` (regla de listas de AGENTS.md), una compra sin pendientes no sale, una con un renglón «llega después» sí, en `server/internal/integration/pending_receipts_test.go`
- [X] T045 [P] [US1b] Test del handler: cajero → 403; recibir de más sin `extraAmount` → 400; regalo con importe ≠ 0 → 400; arreglo `[]` vacío, en `server/internal/integration/pending_receipts_http_test.go`
- [X] T046 [P] [US1b] Tests de vitest de «Por recibir» (maqueta A con buscador): agrupado por compra, buscador, «Recibir…» con cantidad tecleada y vacía al abrir, de más pide importe, «Ya no va a llegar…» con cuántas, «Llegó todo…» con confirmación, «Llegó algo más…» (lo pagué / fue regalo), también desde «Corregir lo que llegó» (`ExpenseDetailDialog`, «+ Agregar un artículo que faltó»), en `web/src/features/backoffice/PendingReceiptsTab.test.tsx`

### Implementación

- [X] T047 [US1b] Consultas de pendientes y de recibir/no llega/agregar renglón en `server/queries/purchases.sql`, y el filtro `sqlc.narg('pending')` en `ListExpenses`/`CountExpenses` de `server/queries/expenses.sql` hasta que T044 pase; `make sqlc`
- [X] T048 [US1b] `ReceiveItem`, `MarkNotComing`, `AddArrivedItem` en `server/internal/app/purchases_receive.go`, con eventos y orden por insumo, hasta que T043 pase
- [X] T049 [US1b] Rutas `GET /stock/pending-receipts`, `POST /expenses/{id}/items/{itemId}/receive`, `…/not-coming`, `POST /expenses/{id}/items`, con `RequirePermission(PermInventoryReceive)`, en `router.go` y `handlers_backoffice.go`, hasta que T045 pase
- [X] T050 [US1b] Pestaña «Por recibir» según plan.md › Maquetas, fila «Almacén › Por recibir» (A con buscador), en `web/src/features/backoffice/PendingReceiptsTab.tsx` y `StockPage.tsx` (pestañas en el orden fijado en el plan); filtro «Con pendientes» en `web/src/features/backoffice/ExpensesPage.tsx`; hasta que T046 pase

---

## Phase 6: User Story 3 — Conteo (P1)

**Independent Test**: contar 5 insumos en negativo, recargar y cambiar de tableta sin perder cifras, vender durante el conteo, cerrar: quedan en lo contado menos lo vendido, con su ajuste; los no contados igual.

### Tests (primero, en rojo)

- [X] T051 [P] [US3] Test table-driven de `CountAdjustment` (contado − sistema al capturar; recaptura usa la lectura nueva; ajuste 0 no inserta) en `server/internal/domain/stock_count_test.go`
- [X] T052 [US3] Test de integración bajo `appRoleStore` con `inTheThreeCases`: un conteo abierto por sucursal (segundo → 409 con su id); capturar guarda `system_qty`; una venta después de capturar queda restada al cerrar; no contados intactos; conteo cerrado no acepta capturas; descartar no toca lo guardado; `system_qty` no viaja mientras está abierto (a ciegas); `preview` igual a lo que hace `close`; `GET /stock/counts/last-closed` devuelve el cierre más reciente de la sucursal o `null`; ajustes ordenados por insumo, con `stock_count_id`, sucursal del conteo y quién cerró (SC-003); evento `stock.counted`, en `server/internal/integration/stock_counts_test.go`
- [X] T053 [P] [US3] Test del handler: cajero → 403; `GET /stock/counts/last-closed` exige `inventory.receive`; `countedQty` negativo, NaN o sobre `MaxStockQty` → 400; `PUT /stock/ingredients/{id}/place` de más de 30 → 400, en `server/internal/integration/stock_counts_http_test.go`
- [X] T054 [P] [US3] Tests de vitest del teclado numérico en pantalla (teclas ≥ 56 px, punto solo con decimales, borrar, siguiente casilla, no abre el teclado del sistema) en `web/src/components/NumericKeypad.test.tsx`
- [X] T055 [P] [US3] Tests de vitest del conteo (maqueta «Almacén › Conteo»): dos vistas con interruptor recordado en la tableta; agrupado por lugar; empaques con contenido; «—» y no «0» de fondo; guardar vacío no cierra; «✓ guardado»; cierre que nombra no contados, dice «sobraban/faltaban», marca diferencias grandes y nunca muestra una existencia negativa a secas; descartar con confirmación; estado cerrado visible, en `web/src/features/backoffice/StockCountTab.test.tsx`

### Implementación

- [X] T056 [US3] `CountAdjustment` en `server/internal/domain/stock_count.go` hasta que T051 pase
- [X] T057 [US3] Consultas en `server/queries/stock_counts.sql` (abrir, capturar, quitar, vista previa, cerrar, descartar, lista por lugar) y `count_place` en `server/queries/ingredients.sql`; `make sqlc`
- [X] T058 [US3] Servicio de conteo (métodos de `BackofficeService`, que ya trae el almacén y las compras) en `server/internal/app/stock_counts.go` y lugar del insumo en `articles.go`, hasta que T052 pase
- [X] T059 [US3] Rutas de conteo con `RequirePermission(PermInventoryCount)` y `PUT /stock/ingredients/{id}/place` con `PermInventoryConfigure` (T087 la pasó a `PermInventoryCount`: el lugar se pone desde el conteo, FR-015f), en `server/internal/httpapi/handlers_stock_counts.go` y `router.go`, hasta que T053 pase
- [X] T060 [P] [US3] `NumericKeypad` en `web/src/components/NumericKeypad.tsx` hasta que T054 pase
- [X] T061 [US3] Pestaña «Conteo» según plan.md › Maquetas, fila «Almacén › Conteo» (las dos vistas), en `web/src/features/backoffice/StockCountTab.tsx`, y el lugar en la hoja del insumo («Cómo llega», `CountPlaceField.tsx`), hasta que T055 pase; además `lastClosedCount()` en `web/src/api/backoffice.ts` y, en la captura (`ExpenseDialog.tsx`), el aviso si la fecha del gasto es anterior al último conteo cerrado, con su prueba en `ExpenseDialog.test.tsx` (venía de T030/T039)

---

## Phase 7: Polish

- [X] T062 [P] Medición de uso: acciones de compras, por recibir y conteo en `accionesMedibles` de `server/internal/domain/uso.go` (+ test) y sus etiquetas en `web/src/consola/etiquetas-de-uso.ts`
- [X] T063 [P] Test de FR-018 / SC-006: ningún nombre de producto, marca ni proveedor del negocio en el código ni en las migraciones de la feature, en `server/internal/app/purchases_names_test.go` (espejo del de la 037)
- [X] T064 Medir el tiempo del servicio como `gatobobah_app` sobre la copia de producción (se midió la llamada completa y no cada `EXPLAIN`: la meta es de la operación): guardar un gasto de 10 renglones con recálculo de costos < 1 s, lista de pendientes y conteo; anotar en research.md
- [X] T065 Capturas con Playwright a 1024×600 de las cinco pantallas contra el ambiente de pruebas (tras el merge a develop) y prueba sin contexto con dos personas de comida (el café del cliente y otra fonda o taquería); resultado y pendientes del dueño en research.md, R13
- [X] T066 Actualizar AGENTS.md (compras que suman, corregir por diferencia, por recibir, conteo por diferencia, longitud, «lugar» no es almacén) y `docs/matriz-de-pantallas.md`
- [X] T067 Gates: `make api-build && make api-test`, `make test-integration`, `bun run lint`, `bun run vitest run`, `bun run build`, `make ci-local`; correr quickstart.md completo
- [X] T068 `/revision-de-codigo` (hook `after_implement`)


## Phase 8: Arreglos de la revisión de código (T068)

Cada uno con su prueba vista en rojo antes del arreglo.

- [X] T069 Llave compuesta `(ingredient_id, company_id)` en `expense_items`, `stock_movements` y `stock_levels` (en `server/migrations/0092_purchases_into_stock.sql`, que no está en producción) y 404 desde `lockIngredients` en `server/internal/app/purchases.go` si un insumo no es de la empresa; pruebas en `migration_0092_test.go` y `TestAPurchaseCannotUseAnotherCompanysIngredient`
- [X] T070 Lo recibido más lo que ya no llega no pasa de lo pedido: `saveStockLine`, `insertItems`, `SetItemsReceived` y el check `expense_items_received_range`; `TestReceivingMoreThanOrderedOutsideReceiveIsRejected`, `TestAbsurdQuantitiesAreRejectedQuickly`
- [X] T071 Cambiar el artículo de un renglón ya recibido regresa lo sumado al artículo viejo, en `server/internal/app/purchases.go`; `TestChangingTheIngredientOfAReceivedLineMovesWhatItAdded`
- [X] T072 Cantidades con exponente absurdo → 400 (`domain.ValidStockQtyOrZero` con guarda de escala; `seenApplied`, `notComing`, «ya no va a llegar»); `TestValidStockQtyOrZero`, `TestAbsurdQuantitiesAreRejectedQuickly`
- [X] T073 `lockIngredients` toma también las existencias sin esperar (anular, quitar, cambiar de artículo); `TestCancellingNeverWaitsForStockHeldElsewhere`
- [X] T074 Editar un empaque sin mandar `active` conserva su estado (`server/internal/app/presentations.go`); `TestEditingARetiredPresentationKeepsItRetired`
- [X] T075 Un empaque retirado no bloquea corregir compras viejas (`lineKeepsPresentation`); `TestARetiredPresentationDoesNotBlockCorrectingOldPurchases`
- [X] T076 Una compra pagada que vuelve a deberse borra `paid_at`/`paid_by`; el total y el costo unitario se validan antes de escribir (`RaiseExpenseAmount`, `raiseTotal`, `domain.ValidLineUnitCost`); `TestAPaidPurchaseThatGrowsOwesTheDifference`, `TestATotalThatGrowsPastTheMoneyCeilingIsRejected`
- [X] T077 El cierre de un conteo dice en la bitácora cuántas cifras cambiaron después de la vista previa (`stock_counts.previewed_at`); `TestRecapturingAfterThePreviewIsRecorded`
- [X] T078 `pending` y `pendingReceipt` en desacuerdo → 400; la reversa de la 0092 revisa lo aprendido de proveedores en cm/m; `TestPurchasesHTTP`, `TestMigration0092DownRefusesWithDataAndRevertsClean`
- [X] T079 La captura y «Corregir lo que llegó» a pantalla completa (`size="full"`, 100dvh): medían 400 de 600 px; `e2e/pantallas.spec.ts › B15`, medida con Playwright a 1024×600 contra la app local del worktree
- [X] T080 El pie de la captura muestra lo que suman tres renglones y «y N más»; `ExpenseDialog.test.tsx`
- [X] T081 Los filtros de Gastos miden 44 px; `ExpensesPage.test.tsx`


## Phase 9: Decisiones del dueño tras la prueba sin contexto (research R13)

Cada una con su prueba vista en rojo antes del código.

- [X] T082 [P] Tests de vitest de «¿Cómo se pagó?» (FR-004d): «Total de la compra»; no guarda sin contestar; un método manda un pago por el total; efectivo con una caja abierta la toma sola, con varias la pide y sin ninguna no se ofrece; «Todavía no se paga» guarda sin pagos; con pagos capturados abajo no se pregunta, en `web/src/features/backoffice/ExpenseDialog.test.tsx`
- [X] T083 [P] Tests de vitest de «Llegó una parte» (FR-007e): pide cuántos, acepta más que cero y menos que lo pedido, manda `qtyReceived` con esa cantidad y el pie dice lo que suma, en `web/src/features/backoffice/ExpenseDialog.test.tsx`
- [X] T084 [P] Test de integración: una compra con un renglón a medias queda en «Por recibir» con lo que falta, en `server/internal/integration/pending_receipts_test.go`
- [X] T085 [P] Tests del lugar desde el conteo (FR-015f): el renglón ofrece los lugares que ya usa la empresa y «Otro…», guarda y vuelve a ordenar; la ruta pide `inventory.count` y no `inventory.configure` (handler y vitest), en `server/internal/integration/stock_counts_http_test.go` y `web/src/features/backoffice/StockCountTab.test.tsx`
- [X] T086 «¿Cómo se pagó?» y «Llegó una parte» en `web/src/features/backoffice/ExpenseDialog.tsx` hasta que T082 y T083 pasen
- [X] T087 Ruta del lugar con `RequirePermission(PermInventoryCount)` en `server/internal/httpapi/router.go` y «Lugar» en `web/src/features/backoffice/StockCountTab.tsx` hasta que T085 pase
- [X] T088 Capturas a 1024×600 contra el ambiente de pruebas de las tres pantallas tocadas, tras el merge. Encontró dos defectos, arreglados con su prueba: sin empaque llegaba elegida la unidad base (ml) en vez de la grande, y «¿Cómo se pagó?» ofrecía los métodos de las plataformas


## Phase 10: Cobertura que el análisis encontró indirecta (analyze C1, C2)

Cada test se vio en rojo con una mutación antes de darlo por bueno.

- [X] T089 [P] Test de que ningún texto de las pantallas de compras, empaques y conteo dice «existencia», «movimiento», «unidad base» ni «presentación» (FR-019), leyendo los textos del código y no los comentarios, en `web/src/features/backoffice/operatorWords.test.ts`
- [X] T090 [P] Test de que un conteo a medias vuelve igual al montar la pantalla sin caché ni almacenamiento local, como al recargar o en otra tableta (SC-005), en `web/src/features/backoffice/StockCountTab.test.tsx`

SC-001 (5 insumos en menos de 2 minutos) no lleva tarea de código: un script mide la velocidad de un
robot, no la de una persona. Se mide con cronómetro en la siguiente prueba con alguien sin contexto.

---

## Dependencies

- Phase 1 → Phase 2 → US2 → US1 → US1b. US3 puede avanzar en paralelo desde Phase 2 hasta sus pruebas, dominio, consultas y servicio (T051–T058); sus rutas (T059) y su pestaña (T061) van después de las de US1b por compartir `router.go` y `StockPage.tsx`.
- Archivos compartidos que obligan orden: `router.go` (T019 → T037 → T049 → T059), `web/src/api/backoffice.ts` (T020 → T039), `StockPage.tsx` (T021 → T050 → T061), `server/queries/purchases.sql` (T032 → T047), `expenses.go` (T034 → T048), `articles.go` (T018 → T035 → T058).
- Dentro de cada historia: tests en rojo → dominio → consultas → servicio → handler → web.

## Parallel examples

- Phase 2: T003, T004, T005, T006 y T007 a la vez; luego T010, T011, T012 y T013 en paralelo tras T008–T009.
- US2: T014, T015 y T016 a la vez; T020 en paralelo con T017–T019.
- US1: T028, T029, T030 y T031 a la vez con T022–T027 (mismo archivo entre sí, en orden).
- US3: T051, T053, T054 y T055 a la vez; T060 en paralelo con T056–T059.

## Implementation strategy

1. **MVP**: Phases 1–2 + US2 + US1 (comprar en empaque y que sume, corregir, costo). Validar con quickstart 1–4.
2. Después US1b (por recibir) y US3 (conteo), cada una entregable sola.
3. Polish y revisión de código al final.
