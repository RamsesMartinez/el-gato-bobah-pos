# Tasks: Conteo con productos de existencia propia, y quitarles el contador

**Input**: Design documents from `specs/069-conteo-productos-reventa/`

**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/api.md, quickstart.md

**Tests**: obligatorios (constitución IV, TDD). Cada tarea de test va ANTES de su código y se ve en
rojo por la razón correcta antes de escribir el código. Los dos criterios de aceptación del issue
tienen nombre propio: `TestAProductAtMinusTwoCountedAtFifteenEndsAtFifteen` (T012) y
`TestAProductWhoseFlavorsDeductSellsOnlyTheFlavorArticle` (T030).

**Organization**: por historia de usuario (spec.md): US1 conteo (P1), US2 «Lo restan sus opciones» (P2).

## Format: `[ID] [P?] [Story] Description`

## Path Conventions

`server/` (Go) y `web/src/` (React). Integración en `server/internal/integration/` contra Postgres
real (`make test-integration`).

---

## Phase 1: Setup

- [X] T001 Volver a revisar el número de migración contra las ramas y worktrees de 065, 066 y 067 (`git fetch`, `git ls-tree origin/<rama> server/migrations/`, `ls ../egb-06*/server/migrations`); si alguna ya usa 0110, tomar el siguiente libre y corregir research R7 y data-model en specs/069-conteo-productos-reventa/

---

## Phase 2: Foundational (bloquea las dos historias)

- [ ] T002 Test de la migración en server/internal/integration/migration_0110_test.go: sobre la base restaurada con dos empresas, la tabla `stock_count_product_lines` existe con RLS `nullif`, grants de `gatobobah_app` (select, insert, update, delete), FK compuestas a `stock_counts`, `products` y `users`, check de entero ≥ 0 (rechaza 2.5 y −1); el Down se niega con renglones y con la tabla vacía la borra. Verlo en rojo
- [ ] T003 Migración server/migrations/0110_count_product_lines.sql según data-model.md (comentarios del porqué: tabla aparte, `last_movement_id` sin FK a propósito, `unit_cost` nulo = cerrado sin ajustar, Down que se niega con datos y deja escritos los ajustes que sobreviven)
- [ ] T004 Renglón de la 0110 en `guardedDownCleanup` de server/internal/integration/harness_restaurado_test.go (borra `stock_count_product_lines` en la copia) y confirmar `TestEveryGuardedDownHasItsTestCleanup`, `TestEveryCompanyTableIsIsolated` y `TestEveryColumnIsCoveredByTheAnonymousBackup` en verde
- [ ] T005 Pedir revisión del `db-architect` sobre server/migrations/0110_count_product_lines.sql ANTES de aplicarla a la base del worktree; corregir lo que encuentre

**Checkpoint**: tabla lista y vigilada.

---

## Phase 3: User Story 1 — Contar los productos que se venden tal cual (P1) MVP

**Goal**: los productos con existencia propia se listan, capturan, previsualizan y ajustan al cerrar el conteo.

**Independent Test**: producto en −2, capturar 15, cerrar → 15 y un `ajuste` de +17 ligado al conteo.

### Tests para US1 (escribir primero, ver en rojo)

- [ ] T006 [P] [US1] Tests de dominio table-driven en server/internal/domain/stock_count_test.go: `PlanCountClose` genérico con renglones de artículo y de producto (orden por clase y luego id, sin movimiento si no hay diferencia, −2 → 15 da +17), y `ValidCountPieces` (acepta 0 y enteros, rechaza 2.5, negativos, NaN/escala absurda)
- [ ] T007 [P] [US1] Test de dominio en server/internal/domain/consumption_mode_test.go: `CountsOwnStock` (o el nombre elegido) es verdadero solo para `ModeOwnStock` (paquete con `track_stock` puesto queda fuera)
- [ ] T008 [P] [US1] Test de integración en server/internal/integration/count_products_test.go: el conteo lista los productos activos con existencia propia y NO los de receta, paquete, «No se cuenta», sin decidir ni archivados (FR-001, FR-002, SC-003), con su categoría
- [ ] T009 [P] [US1] Test de integración en server/internal/integration/count_products_test.go: capturar guarda lo contado y lo que el sistema tenía; recapturar reemplaza; quitar vuelve a «sin contar»; 2.5 → validación; producto de receta → no encontrado; sobre el máximo de piezas → validación; `confirmable` con cifra rara → `ErrCountNeedsConfirm` (FR-003, FR-005)
- [ ] T010 [P] [US1] Test de integración en server/internal/integration/count_products_test.go: producto que nunca tuvo fila en `stock_levels` de la sucursal: capturar crea la fila en cero y la toma; una venta concurrente durante el cierre no se pierde (R8)
- [ ] T011 [P] [US1] Test de integración en server/internal/integration/count_products_test.go: capturado en 15 con sistema 10, venta de 2, cierre → 13 (ajuste +5 contra lo capturado); no contado → sin cambio y en `notCountedProducts`; producto que pasó a «No se cuenta» con el conteo abierto → no se ajusta y sale en `skippedProducts`; cambio de modo a media cerrada → «vuelve a intentar»; producto archivado con cifra ya capturada → se ajusta, archivado sin cifra → no aparece; dos sucursales → el ajuste toca solo la del conteo (FR-006, FR-007, FR-009, R8)
- [ ] T012 [US1] Test de aceptación `TestAProductAtMinusTwoCountedAtFifteenEndsAtFifteen` en server/internal/integration/count_products_test.go: producto en −2, conteo de 15, cierre → `stock_levels` en 15 y exactamente un `stock_movements` `ajuste` de +17 con `stock_count_id` del conteo, `user_id` de quien cerró y la sucursal del conteo; `unit_cost` del renglón = `current_cost` del producto (FR-008)
- [ ] T013 [P] [US1] Test de aislamiento en server/internal/integration/count_products_test.go con `appRoleStore` e `inTheThreeCases`: listar, capturar, quitar, previsualizar y cerrar no ven ni tocan cifras ni productos de otra empresa, con conexión reciclada ni sin empresa (SC-005)
- [ ] T014 [P] [US1] Test HTTP en server/internal/integration/stock_counts_http_test.go: `PUT/DELETE /stock/counts/{id}/product-lines/{productId}` exigen `inventory.count` (403 sin él), responden según contracts/api.md, y `GET /counts/current`, preview y close traen `products`, `adjustedProducts`, `notCountedProducts` y `skippedProducts` como arreglos en el JSON crudo (nunca `null`), y ningún renglón de `products` trae lo que el sistema tiene (conteo a ciegas, FR-004)

### Implementación US1

- [ ] T015 [US1] Dominio en server/internal/domain/stock_count.go y consumption_mode.go: generalizar `PlanCountClose` a renglones con clase (artículo/producto), `ValidCountPieces`, y el predicado de existencia propia (hasta poner T006–T007 en verde)
- [ ] T016 [US1] Consultas sqlc en server/queries/stock_counts.sql: `CountProducts` (activos con existencia propia, categoría y lo contado), `EnsureProductStockLevel`, `LockProductStockLevelForCapture`, `ProductOnHandForCapture`, `UpsertCountProductLine`, `DeleteCountProductLine`, `CountProductLines` (con el modo actual del producto), `NotCountedProducts`, `LockProductStockLevelsInBranchNowait`, `InsertCountProductMovement`, `SaveCountProductLineCosts`, `LastClosedCountedProductQty`; `make sqlc`
- [ ] T017 [US1] Servicio en server/internal/app/stock_counts.go: `CountView.Products`, `CaptureCountProduct`, `UncaptureCountProduct`, preview y cierre con productos (bloqueo de productos después de artículos, modo leído después del bloqueo, costo al cerrar), siempre arreglos; auditoría del cierre con el número de productos (hasta T008–T013 en verde)
- [ ] T018 [US1] Handlers y rutas en server/internal/httpapi/handlers_stock_counts.go y router.go dentro del grupo de `PermInventoryCount` (hasta T014 en verde)
- [ ] T019 [P] [US1] Tipos en web/src/api/backoffice.ts: `CountProduct`, `StockCount.products?`, `CountClose` con los arreglos nuevos opcionales (el compilador obliga a la guarda), `captureCountProduct`, `uncaptureCountProduct`
- [ ] T020 [US1] Tests de pantalla en web/src/features/backoffice/StockCountTab.test.tsx, vistos en rojo (fila «Conteo con productos» de «Maquetas», opción C): los productos van al final agrupados por categoría con «Producto del menú» apagado; el renglón de avance muestra «N productos del menú al final ›» como botón de ≥44 px y al tocarlo baja al primer grupo; la captura de un producto no ofrece punto decimal y guarda en `product-lines`; «Uno por uno» los recorre después de los artículos; la vista previa pone primero las diferencias grandes, marca «· producto», y esconde los nombres sin contar tras «Ver cuáles»; los productos que dejaron de contar su existencia salen aparte, diciendo que no se ajustan (FR-009); el resumen al cerrar cuenta artículos y productos con las mismas cifras que la vista previa y plural correcto (FR-017)
- [ ] T021 [US1] Implementación en web/src/features/backoffice/StockCountTab.tsx (fila «Conteo con productos» de «Maquetas», opción C) hasta T020 en verde

**Checkpoint**: US1 completa y probada sola.

---

## Phase 4: User Story 2 — «Lo restan sus opciones» (P2)

**Goal**: un producto cuyas opciones restan deja de contar su propia existencia sin insumos.

**Independent Test**: producto con grupo obligatorio de sabores → `mode: "options"` sin insumos → vender con Fresa baja solo el artículo de Fresa.

### Tests para US2 (escribir primero, ver en rojo)

- [ ] T022 [P] [US2] Test de dominio en server/internal/domain/consumption_mode_test.go: `ModeOf` con `OptionsDeduct` devuelve `ModeOptions` solo si no es paquete, no lleva existencia propia ni insumos y está confirmado; sin `OptionsDeduct` sigue `ModeNone`
- [ ] T023 [P] [US2] Test de integración en server/internal/integration/options_mode_test.go: «una opción resta» = opción activa en grupo ligado y activo con receta con renglones o ligada a un producto que resta; opción inactiva, grupo desligado, receta vacía o producto ligado «No se cuenta» → no resta
- [ ] T024 [P] [US2] Test de integración en server/internal/integration/options_mode_test.go: `mode: "options"` con insumos, paquete o producto ligado → validación; sin opciones que resten → validación «Ninguna de sus opciones resta del almacén»; producto sin existencia propia → se guarda confirmado sin receta y sin movimientos (FR-012)
- [ ] T025 [P] [US2] Test de integración en server/internal/integration/options_mode_test.go: desde existencia propia en dos sucursales, guardar `options` cierra cada una en cero con el movimiento de cambio de modo y deja `product.consumption_mode` en la bitácora con antes `own_stock` y después `options` (FR-013)
- [ ] T026 [P] [US2] Test de integración en server/internal/integration/options_mode_test.go: `GET` de la composición trae `optionsDeduct`, `optionsRequired` y los grupos con lo que resta cada opción; Recetas responde `mode: "options"` y Salud del inventario `status: "options"` (FR-011, FR-014, FR-015)
- [ ] T027 [P] [US2] Test de aislamiento en server/internal/integration/options_mode_test.go con `appRoleStore` e `inTheThreeCases`: la consulta de opciones que restan no ve grupos ni opciones de otra empresa, con conexión reciclada ni sin empresa
- [ ] T028 [US2] Test de pantalla en web/src/shared/CompositionSheet.test.tsx (fila «Cambio a «Lo restan sus opciones»» de «Maquetas», opción B), visto en rojo: con `optionsDeduct` la cuarta tarjeta es «Lo restan sus opciones» y no hay «No se cuenta»; sin él sigue «No se cuenta»; elegirla manda `mode: "options"` sin insumos con Guardar encendido; lista grupos y lo que resta cada opción; avisa si ningún grupo que resta es obligatorio; la nota dice «Vendiste N más de las que tenías anotadas» o «Tenías N anotadas», que queda en 0 y que se cuenta cada opción; el diálogo dice solo lo que cambia y su botón «Sí, que lo resten sus opciones»
- [ ] T029 [P] [US2] Test en web/src/features/admin/RecipesPage.test.tsx (o el archivo de pruebas de la página): `mode: "options"` se lee «Lo restan sus opciones» y cabe en la celda de resumen

- [ ] T030 [US2] Test de aceptación `TestAProductWhoseFlavorsDeductSellsOnlyTheFlavorArticle` en server/internal/integration/options_mode_test.go: producto con existencia propia y grupo obligatorio «Sabor» (Fresa → artículo Fresa, Limón → artículo Limón), guardar `mode: "options"` sin insumos, vender uno con Fresa por el camino real de crear pedido → un solo movimiento `venta`, del artículo Fresa; el producto ya no tiene `track_stock` ni movimiento de venta propio. Verlo en rojo
### Implementación US2

- [ ] T031 [US2] Dominio en server/internal/domain/consumption_mode.go: `ModeOptions`, `ProductModeFacts.OptionsDeduct` (hasta T022 en verde)
- [ ] T032 [US2] Consulta sqlc `ProductOptionsDeduct` (y grupos con lo que resta cada opción) en server/queries/composition.sql; `make sqlc`
- [ ] T033 [US2] Servicio en server/internal/app/composition.go: `validateMode` acepta `options` (vacía y con opciones que restan), `targetMode`/`applyModeChange` lo tratan como `TargetNothing`, se guarda confirmado; la respuesta de composición trae `optionsDeduct`, `optionsRequired` y los grupos (hasta T023–T025, T027, T030 en verde)
- [ ] T034 [US2] Recetas y Salud en server/internal/app/recipes.go e inventory_health.go (y sus consultas) con el modo `options` (hasta T026 en verde)
- [ ] T035 [P] [US2] Tipos en web/src/api/admin.ts y web/src/api/backoffice.ts: `mode` acepta `'options'`, `Composition.optionsDeduct?`, `optionsRequired?`, `optionGroups?`, `RecipeRow.mode` con `'options'`; etiqueta en web/src/shared/audit/labels.ts
- [ ] T036 [US2] Implementación en web/src/shared/CompositionSheet.tsx y web/src/features/admin/RecipesPage.tsx (fila «Cambio a «Lo restan sus opciones»» de «Maquetas», opción B) hasta T028–T029 en verde

**Checkpoint**: US1 y US2 funcionan cada una por su lado.

---

## Phase 5: Polish & Cross-Cutting

- [ ] T037 [P] Nota en docs/mecanica/almacen.md: productos en el conteo (tabla aparte, fila de existencia asegurada al capturar, modo leído después del bloqueo, `unit_cost` nulo = cerrado por la API anterior sin ajustar) y el modo `options` deducido del catálogo vivo; ninguna regla depende de nombres de productos, grupos o categorías (FR-016)
- [ ] T038 `cd server && go build ./... && go test ./...` y `make test-integration` en verde
- [ ] T039 `cd web && bun run lint && bun run vitest run && bun run build` en verde
- [ ] T040 Prueba con usuario nuevo a 1024×600 (`hasTouch`) contra el `make start` del worktree con la base restaurada (`make db-restaurar`): subagente sin contexto que cuente un producto y le quite el contador a otro siguiendo quickstart.md; cerrar los pedidos de prueba (entregar y cobrar) y descartar o cerrar el conteo de prueba; lo observado se escribe en texto (sin capturas en el repo)
- [ ] T041 Borrar el arnés de maquetas `web/maquetas069/` y su línea de `.git/info/exclude`

---

## Dependencies & Execution Order

- Setup (T001) → Foundational (T002–T005) → US1 y US2.
- US1 y US2 son independientes entre sí; US2 no usa la tabla nueva.
- Dentro de cada historia: tests (en rojo) → dominio → sqlc → servicio → HTTP → pantalla.
- Polish al final.

### Parallel Opportunities

- T006, T007, T008–T011, T013, T014 se escriben en paralelo (archivos o casos distintos).
- T019 en paralelo con el backend de US1; T035 con el de US2.
- US2 completa puede avanzar en paralelo a US1 después de la fase 2.

## Implementation Strategy

MVP = US1 (el conteo con productos) con la migración. US2 entra en la misma subida: no toca la tabla
nueva ni el esquema.
