---
description: "Tareas de la feature 094 — varios almacenes por sucursal"
---

# Tasks: Varios almacenes por sucursal

**Input**: `specs/094-varios-almacenes/` (spec, plan, research R1–R12, data-model, contracts/api.md, quickstart)

**Tests**: obligatorios (constitución IV, TDD). En cada historia la tarea de test va antes de la de
código y se ve fallar por la razón correcta. Toda consulta o servicio que lea una tabla de empresa
lleva su caso bajo `appRoleStore` con `inTheThreeCases`.

**Entregas**: cada fase de historia termina en una subida a producción (research R3). Los números de
migración y de permiso se fijan al llegar a develop (R11 M7); aquí se escriben `NNNN`.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: paralelizable (archivo distinto, sin dependencia pendiente)
- **[Story]**: historia del spec (US1…US8)

---

## Phase 1: Setup

- [X] T001 Traer `origin/develop` a la rama en local con merge firmado y correr `codegraph sync`; renumerar migración y permisos si otra rama ya tomó NNNN (`server/migrations/`)
- [X] T002 Restaurar el respaldo `backups/prod/pre-094-20261123-0241.dump` en la base del worktree con `make stop && make db-restaurar dump=… && make start` y anotar en `specs/094-varios-almacenes/quickstart.md` los totales por sucursal y artículo de antes de migrar (solo la consulta, no las cifras)

---

## Phase 2: Foundational (subida 1 — esquema)

**⚠️ Bloquea todas las historias.** db-architect ya revisó el diseño (R11, R12); vuelve a revisar el SQL antes de aplicarlo (T012).

### Tests primero

- [X] T003 Test de migración sobre respaldo restaurado con dos empresas: ninguna fila sin almacén en las nueve tablas (en `expense_items`, todo renglón con `item_type`) y la existencia total por sucursal y artículo idéntica antes y después, en `server/internal/integration/warehouses_migration_test.go` (`TestWarehouseMigrationKeepsBranchTotals`)
- [X] T004 [P] Test de convivencia con la API anterior: inserts sin `warehouse_id` en `stock_movements`, `stock_levels` (`EnsureProductStockLevel` viejo con `on conflict (branch_id, product_id)`), `stock_counts`, `waste_records`, `branch_stock_minimums` (`on conflict (branch_id, …)`), `stock_out_reports` (`OpenManualOut` viejo) y `expense_items` caen en el de venta sin error, en `server/internal/integration/warehouses_migration_test.go`
- [X] T005 [P] Test: un movimiento a un almacén que no es el de venta responde `EGB04` y cambiar `is_sales` se rechaza mientras rija la 0117 (R11 H2), en `server/internal/integration/warehouses_test.go`
- [X] T006 [P] Test: con existencias en dos almacenes por artículo (sembradas por SQL con el trigger de H2 apagado), una venta no truena en `low_stock_state` y el aviso mide el almacén de venta (Local 0 / Bodega 50 / mínimo 5 → abre «se acabó»), en `server/internal/integration/warehouses_test.go`
- [X] T007 [P] Test: venta cruzando el mínimo **crea la fila** de aviso con el `on conflict` por almacén (R12), en `server/internal/integration/warehouses_test.go`
- [X] T008 [P] Tests de aislamiento bajo `appRoleStore` con `inTheThreeCases` para `warehouses`; agregar `warehouses` a lo que vigila `TestEveryCompanyTableIsIsolated` (`server/internal/integration/rls_all_tables_test.go`)
- [X] T009 [P] Test: un almacén de otra sucursal o de otra empresa no se puede guardar en ninguna de las nueve tablas aunque RLS falle (llaves compuestas; conexión como owner), en `server/internal/integration/warehouses_test.go`
- [X] T010 [P] Clasificar `warehouses.name` en `noPersonalData` (`server/internal/integration/respaldo_anonimo_test.go`) y renglón de `guardedDownCleanup` para el Down de NNNN (`server/internal/integration/harness_restaurado_test.go`)
- [X] T011 [P] Helper de pruebas que crea el almacén de venta junto con la sucursal y reemplazo de los inserts directos a `branches` en los tests que listó R11 M2 (`server/internal/integration/*_test.go`)

### Implementación

- [X] T012 Escribir `server/migrations/NNNN_warehouses.sql` según data-model y R11/R12: `lock_timeout`, `lock table` en orden de la venta, tabla `warehouses` con RLS y grants, «Local» por sucursal, `warehouse_id` nullable + llaves compuestas `no action` + relleno en las nueve tablas, llaves únicas por almacén junto a las de sucursal, `sales_warehouse_of`, `fill_warehouse_id`, `fill_stock_movement_branch_id` extendida (almacén + compuerta EGB04), `warehouses_before_update` (is_sales inmutable, el de venta no se archiva), `apply_stock_movement` con destino por almacén, `low_stock_state` del almacén de venta, `evaluate_low_stock` con `on conflict` por almacén, `unique (id, company_id, warehouse_id)` en conteos y mermas con llave desde movimientos, índice `(company_id, warehouse_id, id desc)`, dos permisos con herencia a admin y gerente; Down con guarda. Pasarlo por el subagente db-architect antes de aplicarlo
- [X] T013 Mapear `EGB03`/`EGB04` a `domain.ErrWarehouseMissing` / `domain.ErrWarehouseNotWritableYet` en `server/internal/store/errors.go` y a 409 en `server/internal/httpapi/respond.go`
- [X] T014 `CreateCompany` crea «Local» en la misma consulta que la matriz, en `server/queries/companies.sql`; `make sqlc`
- [X] T015 Constantes `PermInventoryWarehouseManage` y `PermInventoryTransfer` con su etiqueta en `server/internal/domain/permission.go` y su espejo en `web/src/app/permissions.ts`
- [X] T016 Upserts por almacén en el código de la subida 1 (para que sea seguro como «versión anterior» de la subida 2, R11 H2b): `EnsureProductStockLevel` (`server/queries/stock_counts.sql`), `UpsertBranchMinimum`/`UpsertBranchProductMinimum` (`server/queries/notifications.sql`), `OpenManualOut` (`server/queries/stock_out_reports.sql`); `make sqlc`
- [X] T017 Test + arreglo: `RecomputeArticleStock`, `InsertMergeMove`, `InsertReplacementMove` y `LockArticleStockNowait` por almacén (`server/queries/ingredients_admin.sql`, `server/queries/ingredient_merge.sql`, `server/internal/app/ingredient_merge.go`, `server/internal/app/unit_change.go`), con existencias sembradas en un segundo almacén (`server/internal/integration/ingredient_merge_test.go`, `unit_change_test.go`)
- [X] T018 Test + arreglo: los `join stock_levels … current_branch_id()` de lectura (`ListStockLevels`, `ListArticles`, `GetArticle`, `ListIngredients`, `ShoppingListItems`, `ListLowStockConfigs`, `LowStockSnapshot`, `ArticleOnHandByBranch`) leen el almacén de venta o suman por artículo sin duplicar renglones (`server/queries/stock.sql`, `ingredients_admin.sql`, `ingredients.sql`, `shopping_list.sql`, `notifications.sql`)
- [X] T019 Test + arreglo: reposiciones de cancelación copian `warehouse_id` del movimiento original (`RestockCancelledOrder`, `RestockCancelledLine` y las otras dos de `server/queries/orders.sql`), test en `server/internal/integration/warehouses_test.go` (R12, FR-011)
- [X] T020 Test + arreglo: conteo y merma comparan `warehouse_id` donde hoy comparan `branch_id` (`server/queries/stock_counts.sql`, `server/queries/waste.sql`)
- [X] T021 Revisar que ningún mapa de `movement_type` en Go ni en `web/src` truene con un valor desconocido (prepara la subida 3)

**Checkpoint**: `cd server && go build ./... && go test ./...` y `make test-integration` en verde; quickstart › Migración con totales idénticos.

---

## Phase 3: US1 — Almacenes (P1, subida 1)

**Goal**: dar de alta la bodega sin que cambie nada de lo que hay.
**Independent Test**: spec US1, escenarios 1–6.

- [X] T022 [P] [US1] Tests de dominio table-driven: nombre válido (1–40, normalizado con `CatalogNameKey`), archivar (no el de venta, existencias en cero, sin conteo abierto) en `server/internal/domain/warehouse_test.go`
- [X] T023 [US1] `domain/warehouse.go` con las reglas y sentinels (`ErrWarehouseNotFound`, `ErrWarehouseIsSales`, `ErrWarehouseHasStock`, `ErrWarehouseCountOpen`, `ErrWarehouseNameTaken`)
- [X] T024 [P] [US1] Tests de integración del servicio (crear, renombrar, archivar con existencias → 409, reactivar, nombre repetido sin acentos → 409, `inTheThreeCases`, bitácora `warehouse.*`) en `server/internal/integration/warehouses_test.go`
- [X] T025 [US1] Consultas `server/queries/warehouses.sql` (lista con artículos con existencia, crear, renombrar, archivar `for update`, reactivar); `make sqlc`
- [X] T026 [US1] Servicio `server/internal/app/warehouses.go` con transacción, candado de nombre y bitácora
- [X] T027 [US1] Handlers `server/internal/httpapi/handlers_warehouses.go` y rutas con `RequirePermission` en `server/internal/httpapi/router.go` (contracts › subida 1); `warehouseId` malformado = 400, ajeno = 404
- [X] T028 [P] [US1] Componente común `web/src/components/Segmented.tsx` (44 px, `aria-pressed`, activo que no es el color de error, `overflowX="auto"`) con su test en `web/src/components/Segmented.test.tsx` (R12)
- [X] T029 [US1] Pantalla Almacenes en Negocio › Inventario (`web/src/features/admin/WarehousesSettings.tsx`, montada en `InventorySettingsSection.tsx`) según la fila «Almacenes» de Maquetas del plan (A3); `ListRow` ampliado con ícono opcional; `ConfirmSheet` al archivar; test en `WarehousesSettings.test.tsx`
- [X] T030 [US1] API del front `web/src/api/backoffice.ts` (`warehouses`, `createWarehouse`, `renameWarehouse`, `archiveWarehouse`, `restoreWarehouse`) y hook `web/src/features/backoffice/warehouses/useWarehouses.ts`

---

## Phase 4: US2 + US3 — Venta del almacén de venta y Existencias por almacén (P1, subida 1)

**Goal**: la venta descuenta solo del de venta; Existencias con selector.
**Independent Test**: spec US2 y US3.

- [X] T031 [P] [US2] Test: con existencia en Bodega y cero en Local, vender (mostrador, plataforma, pedido web) baja Local y deja Bodega igual, en `server/internal/integration/warehouses_test.go`
- [ ] T032 [P] [US2] (pendiente: el #326 todavía no llega a develop; mientras, «hay existencia» se lee del almacén de venta en `per_sale_on_hand` y en Existencias) Test de dominio de la regla «hay existencia en el lugar de venta» (Bodega 20, Local 0 → sin existencia) en `server/internal/domain/warehouse_test.go`; si el #326 ya llegó a develop, el test va contra su función y se le cambia el interior (no se duplica, R8)
- [X] T033 [US2] `per_sale_on_hand` de recetas lee el almacén de venta (`server/queries/recipes.sql`)
- [X] T034 [P] [US3] Test de `GET /stock/levels?warehouseId=` (de venta por omisión, otro, `all` suma, `transferFrom` en negativos, 400/404) en `server/internal/integration/warehouses_test.go`
- [X] T035 [US3] `ListStockLevels` por almacén y `all`, y `BackofficeService.StockLevels` (`transferFrom` pasa a T053: en la subida 1 ningún otro almacén puede tener existencia) (`server/queries/stock.sql`, `server/internal/app/backoffice.go`, `server/internal/httpapi/handlers_backoffice.go`)
- [X] T036 [US3] Existencias con `Segmented` (solo con 2+ almacenes; 5+ pasa a botón con hoja; «Traer de Bodega» con hoja y `NumericKeypad` pasa a T053) según la fila «Existencias» de Maquetas, en `web/src/features/backoffice/StockPage.tsx` y `web/src/features/backoffice/warehouses/TraerSheet.tsx`, con test en `StockPage.test.tsx` (sin selector con un almacén)
- [X] T037 [US8] MCP `stock_levels` con `warehouse_id` opcional y `warehouses_list` nueva; `stock_adjust` con `warehouse_id` que responde `WAREHOUSE_NOT_WRITABLE_YET` fuera del de venta (contracts › MCP), en `server/internal/httpapi/mcp_tools_094.go` con test
- [X] T038 Editar la tabla de puertas de la constitución (puerta cruzada con la subida 1, R12) en `.specify/memory/constitution.md`, en un commit propio con versión MINOR y fecha de enmienda (Governance), y las notas de `docs/mecanica/almacen.md`
- [ ] T038b [US1] Prueba con alguien nuevo de una fonda (subagente) a 1280×854 y 1024×600 sobre la base restaurada: dar de alta «Bodega», ver Existencias por almacén y vender; hallazgos con su test (SC-004, SC-005)

**Checkpoint subida 1**: /revision-de-codigo, prueba con alguien nuevo de una fonda a 1280×854 y 1024×600 sobre la base restaurada, `make ci-local`, PR a develop, release a main con respaldo previo, `readyz` 200 y journal limpio.

---

## Phase 5: US4 — Compras con destino y captura rediseñada (P2, subida 2)

**Goal**: cada renglón dice dónde se guarda; la captura deja de verse amontonada.
**Independent Test**: spec US4.

- [ ] T039 [US4] Tests de la 0118: llaves por sucursal borradas, `not null` (check `not valid` + `validate` + `set not null`), «Local» creado donde falte, `expense_items` con `item_type is null or warehouse_id is not null`, compuerta EGB04 quitada salvo `cancelacion` sin almacén, `is_sales` cambiable; Down se niega con duplicados; en `server/internal/integration/warehouses_contract_test.go`
- [ ] T040 [US4] `server/migrations/NNNN_warehouses_contract.sql` (revisada por db-architect antes de aplicarla)
- [ ] T041 [P] [US4] Test de dominio: `PlanStockLines` por almacén (cambiar destino de un renglón aplicado = salida del anterior + entrada al nuevo; excluido no mueve) en `server/internal/domain/purchase_test.go`
- [ ] T042 [US4] Destino por renglón en gasto, edición, `stock-lines` y «Por recibir» (`server/internal/app/purchases.go`, `expense_edit.go`, `server/queries/purchases.sql`, `expenses.sql`, handlers) con tests de integración (escenario 8 renglones / 2 a Bodega / 1 excluido)
- [ ] T043 [US4] `PATCH /stock/warehouses/{id}` acepta `isSales` (cambiar el de venta) con test
- [ ] T044 [US4] Rediseño de la captura según la fila «Compra (captura completa)» de Maquetas (R3, R12): encabezado de una línea, lista + detalle al lado (hoja por debajo de 1100 px), «¿Dónde se guarda?» con 2+ almacenes, `ConfirmSheet` al quitar; sección regalo / no es del negocio según la fila «Regalo / no es del negocio» (S1: interruptores con explicación e `InfoButton`; panel de detalle con desplazamiento propio); color según la fila «Color de la captura» (P1: paleta heredada, estado como ícono al inicio); en `web/src/features/backoffice/ExpenseDialog.tsx` y componentes nuevos en `web/src/features/backoffice/expense/`, con tests vitest que cubran lo que hoy cubre `ExpenseDialog`
- [ ] T045 [US4] El último destino usado se recuerda por equipo en `localStorage` con try/catch (`web/src/features/backoffice/expense/useLastWarehouse.ts`) con test
- [ ] T046 [US8] MCP `purchase_create`, `purchase_receive`, `expense_create`, `expense_edit` con `warehouse_id` por renglón; `stock_adjust` ya escribe en cualquier almacén activo; tests en `server/internal/httpapi/mcp_tools_094_test.go`

---

## Phase 6: US5 — Traspasos (P2, subida 3)

- [ ] T047 [US5] `server/migrations/NNNN_stock_movement_transfer_type.sql` solo con `add value 'traspaso'` (R11 H3)
- [ ] T048 [P] [US5] Tests de dominio: plan del traspaso (origen ≠ destino, cantidades válidas, `NEEDS_CONFIRM` si deja el origen en negativo) en `server/internal/domain/transfer_test.go`
- [ ] T049 [P] [US5] Tests de integración: escenario 10/20 → 16/14 con dos movimientos ligados y `unit_cost` en las dos patas; reintento con la misma `clientKey` no duplica; misma clave con otro cuerpo → 409; almacén archivado o de otra sucursal → 404; `inTheThreeCases` para las dos tablas; guardas de unión en `stock_transfer_lines`; cada pata en su origen o destino; en `server/internal/integration/transfers_test.go`
- [ ] T050 [US5] `server/migrations/NNNN_stock_transfers.sql` (tablas, RLS, llaves, guardas de la 0104, `transfer_id` en movimientos) revisada por db-architect; `MergeTables` y `UnitChangeTables` (`server/internal/app/ingredient_merge.go`, `unit_change.go`); respaldo anónimo (`scripts/respaldo-anonimo.sh`, `respaldo_anonimo_test.go`)
- [ ] T051 [US5] `domain/transfer.go`, `queries/transfers.sql`, `app/transfers.go` (con `lockIngredients` nowait y el almacén `for share`), handlers y rutas `POST/GET /stock/transfers`, bitácora `transfer.created`
- [ ] T052 [US5] Pantalla Traspaso según la fila «Traspaso» de Maquetas: pestaña después de Existencias, `Picker` en De/A, buscador de 44 px, `NumericKeypad` fijo con `useKeypadField`, `clientKey` guardada en el borrador (`web/src/features/backoffice/warehouses/TransferTab.tsx`) con test
- [ ] T053 [US5] «Traer de Bodega» de Existencias usa el traspaso real (T036)
- [ ] T054 [US8] MCP `stock_transfer` con vista previa e idempotencia, test
- [ ] T054b [US5] E2E Playwright del traspaso de un artículo que mide los toques (≤ 6, SC-003) y prueba con alguien nuevo de una fonda: traspasar de Bodega a Local (SC-005), en `web/e2e/traspaso.spec.ts`

---

## Phase 7: US6 — Conteo, merma, mínimos y lugares por almacén (P2, subida 4)

- [ ] T055 [P] [US6] Tests: conteo de Bodega no toca Local; un conteo abierto por almacén; merma y «se acabó» con almacén; mínimos y salud por almacén; lugares con nombre único por almacén y artículos de otro almacén al final («Sin lugar en este almacén»); en `server/internal/integration/warehouses_count_test.go`
- [ ] T056 [US6] Migración si hace falta (índice único de lugares por almacén, `low_stock_state`/`evaluate_low_stock` por almacén) revisada por db-architect
- [ ] T057 [US6] Servicios y consultas de conteo, merma, se acabó, mínimos, lista de compra, salud y lugares con `warehouseId` (`server/internal/app/stock_counts.go`, `waste.go`, `stock_out.go`, `notifications.go`, `inventory_health.go`, queries respectivas)
- [ ] T058b [US6] Prueba con alguien nuevo de una fonda: contar la Bodega y cerrar sin tocar Local (SC-005)
- [ ] T058 [US6] Pantallas: elegir almacén al empezar el conteo (por omisión el de venta) en `web/src/features/backoffice/StockCountTab.tsx`; almacén en merma, se acabó, mínimos y avisos; con test
- [ ] T059 [US8] MCP `stock_count_*`, `waste_record`, `low_stock_update` con `warehouse_id`, tests

---

## Phase 8: US7 — Reportes y movimientos (P3, subida 5)

- [ ] T060 [P] [US7] Tests: `GET /stock/movements?warehouseId=` con almacén y traspaso contraparte; `GET /stock/valuation` (valor por almacén suma el total; negativos no suman valor, test que lo nombra; costo de ventas solo de `venta`) en `server/internal/integration/warehouses_reports_test.go`
- [ ] T061 [US7] Consultas y servicio de movimientos y valuación (`server/queries/stock.sql`, `server/internal/app/backoffice.go`)
- [ ] T062 [US7] Movimientos con almacén y traspaso; reporte de valor por almacén (`web/src/features/backoffice/StockPage.tsx`) con test
- [ ] T063 [US8] MCP `stock_movements` con `warehouse_id`; documentar en la descripción de las herramientas de recetas y pedidos que descuentan del de venta

---

## Phase 9: Polish

- [ ] T064 `docs/mecanica/almacen.md`: notas de lo que no se ve (compuerta EGB04, llaves expandidas y contraídas, no volver atrás después de la 0118, reposición al almacén original)
- [ ] T065 Quitar el arnés de maquetas `web/maquetas-094/` (no versionado) al cerrar
- [ ] T066 Comentario en el #65 con lo que entró por subida; cerrar solo si entraron las cinco

---

## Dependencies

- Phase 2 bloquea todo. US1 + US2/US3 forman la subida 1. US4 depende de la subida 1 en producción (la 0118 necesita que la versión anterior sea la de la subida 1). US5 depende de US4 (almacén no de venta escribible). US6 y US7 dependen de US5 solo por el tipo de movimiento; pueden ir en la misma subida si no rompen R2.
- US8 (MCP) va repartida en la subida de cada ruta.

## Parallel

- T003–T011 son archivos o casos distintos: en paralelo.
- T022, T024 y T028 en paralelo; T031, T032 y T034 en paralelo.

## Implementation Strategy

MVP = subida 1 (Phase 2 + US1 + US2/US3). Cada subida siguiente se sube a producción por su cuenta.
Todas las maquetas están elegidas.
