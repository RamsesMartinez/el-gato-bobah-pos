---
description: "Tareas de la 084 — notificaciones de inventario"
---

# Tasks: Notificaciones de inventario

**Input**: `specs/084-notificaciones-de-inventario/` — plan.md, spec.md, research.md, data-model.md,
contracts/api.md, quickstart.md

**Tests**: obligatorios (constitución IV, TDD). En cada historia el test va **antes** que el código y
se ve fallar por la razón correcta. Integración = `server/internal/integration/` con
`//go:build integration`, bajo `appRoleStore` y con `inTheThreeCases` cuando lee una tabla de empresa.

**Orden**: una sola entrega, en el orden de dependencias del issue: (1) migración y función
compartida → (2) «Se acabó» y «No lo encuentro» → (3) detección conectada y despachador → (4)
campana y franja → (5) ficha de alertas → (6) correo, destinatarios, resumen y lista de compra →
(7) e2e. Las fases de historia siguen ese orden, no el de prioridad del spec.

## Format: `[ID] [P?] [Story] Description`

---

## Phase 1: Setup

- [X] T001 Confirmar que la 0115 y el permiso 38 siguen libres contra `origin/develop` y las ramas abiertas (`git ls-tree` de `server/migrations`, `permissions` en las migraciones) y anotarlo en plan.md
- [X] T002 [P] Agregar `mjml@5.4.1` como dependencia de desarrollo en `web/package.json` con `bun add -d`, correr `bun audit --audit-level=high` y anotar el resultado (los dos avisos *moderate* ya los cubren los overrides de `minimatch` y `glob`)
- [X] T003 [P] Crear `emails/` (fuente MJML, `logo.png` del POS) y el script `web/scripts/compilar-correos.ts` con su comando `bun run compilar-correos` en `web/package.json`; salida en `server/internal/mailtpl/templates/`

---

## Phase 2: Foundational (bloquea todas las historias)

**Tests primero**

- [X] T004 [P] Tests de dominio de reglas puras en `server/internal/domain/notifications_test.go` (efecto de alertas, pedir conteo, bloque de 5 min, enfriamiento de 12 h, hora de la mañana, llaves de búsqueda, texto de «no lo encuentro», tope por hora, ventana de deshacer) — ya escritos; mantener en verde
- [X] T005 [P] Test de paridad `TestNotificationReasonsMatchTheCheck` y `TestNotificationKindsMatchTheCheck` en `server/internal/integration/notifications_schema_test.go`: las listas de `domain` contra los `check` de la 0115
- [X] T006 Test de migración `TestMigration0115OnARealBackup` en `server/internal/integration/migration_0115_test.go`: sobre el respaldo restaurado con dos empresas, sube la 0115 con la API anterior escribiendo movimientos (ventas y compras siguen), verifica columnas con default, grants, RLS, permiso 38 en los 4 sugeridos, orden de triggers en `pg_trigger`, y que el Down se niega con datos y baja limpio con su `guardedDownCleanup` (incluidos los overrides del permiso)
- [X] T007 Test `TestEvaluateLowStockMatchesTheDomain` en `server/internal/integration/notifications_trigger_test.go`: la función de la base produce la tabla de verdad de `domain.LowStockEffect` caso por caso, y `needs_count` con existencia negativa y sin conteo en 30 días
- [X] T008 Tests del trigger en `server/internal/integration/notifications_trigger_test.go`: `TestTwoConnectionsCrossingTheSameMinimumOpenOne` (dos conexiones `appRoleStore`, una sola notificación), `TestANotificationFailureNeverBreaksASale` (falla forzada del aviso, la venta se guarda), `TestAQuietSaleOpensNoSubtransaction` (artículo ya bajo el mínimo con aviso abierto: sin `savepoint` ni `pg_notify`), `TestReplacementMovementsDoNotNotify`, `TestInactiveOrMergedSourceNeverNotifies`, `TestRedirectedMovementNotifiesOnTheTarget`
- [X] T009 [P] Agregar las tablas nuevas a `TestEveryCompanyTableIsIsolated` (automático) y clasificar columnas en `scripts/respaldo-anonimo.sh` paso 5 y `noPersonalData` de `server/internal/integration/respaldo_anonimo_test.go` según data-model; agregar `stock_out_reports`, `branch_stock_minimums` y `notifications` a `app.MergeTables` y `app.UnitChangeTables` (`server/internal/app/ingredient_merge.go`, `unit_change.go`)

**Implementación**

- [X] T010 Migración `server/migrations/0115_inventory_notifications.sql` según data-model.md (columnas, 6 tablas, llaves e índices con nombre, RLS, grants, guardas de unión, `evaluate_low_stock`, `notify_stock_movement` con filtro sin subtransacción, trigger con `lock_timeout`, permiso 38, `notifications_morning_at`, Down con guarda) y su renglón en `guardedDownCleanup` (`server/internal/integration/harness_restaurado_test.go`); `migrations/version.go` a 115
- [X] T011 Correr `db-architect` sobre la 0115 escrita antes de aplicarla en la base local; corregir lo que encuentre con su test
- [X] T012 [P] Permiso `PermInventoryReportOut` en `server/internal/domain/permission.go` (lista, los 4 sugeridos, mensaje), espejo en `web/src/app/permissions.ts`; acciones de bitácora `stock.out_reported`, `stock.out_undone`, `stock.low_stock_configured`, `notification.recipients_changed`, `notification.resolved` en `server/internal/domain/audit.go` y `web/src/shared/audit/labels.ts`
- [X] T013 Consultas sqlc en `server/queries/notifications.sql`, `stock_out_reports.sql`, `notification_recipients.sql`, `shopping_list.sql`; `make sqlc`
- [X] T014 [P] Sentinels nuevos mapeados en `server/internal/httpapi/respond.go` (`ErrReportLimit` 429, `ErrUndoExpired`/`ErrUndoAfterMovement`/`ErrNotificationClosed` 409) con su test en `respond_test.go`

**Checkpoint**: la base detecta y abre avisos; T006–T008 en verde.

---

## Phase 3: User Story 1 — «Se acabó» (P1)

**Goal**: el cajero reporta que algo se acabó; la existencia queda en 0 y se abre el aviso.
**Independent Test**: cajero con solo `inventory.report_out` reporta, deshace, repite; no puede ajustar.

- [X] T015 [P] [US1] Tests de integración en `server/internal/integration/stock_out_test.go`: reporte deja 0 con ajuste «se acabó» y no cuenta como consumo; idempotencia por `clientKey`; segundo reporte con aviso abierto = confirmación sin ajuste; negativa sube a 0; funciona sin alerta ni mínimo (avisa por campana y correo, sin franja); deshacer ≤ 90 s con reloj de la base; deshacer niega tras movimiento o conteo posterior; tope de 20/h; `TestConcurrentStockOutReports`; `TestConcurrentStockOutReportAndSale` (sin 40P01); aislamiento `inTheThreeCases`; `TestCashierCanReportOutButNotAdjust`
- [X] T016 [P] [US1] Tests de handler en `server/internal/httpapi/handlers_stock_out_test.go`: `quantity` en el cuerpo → 400; `clientKey` no uuid → 400; rutas en `testdata/route_gates.golden` (regenerado a propósito, con el porqué en el PR)
- [X] T017 [US1] `StockOutService` en `server/internal/app/stock_out.go`: buscar, sugerencias, reportar (movimiento primero, después aviso — research R12), deshacer, auditoría en la misma transacción
- [X] T018 [US1] Handlers y rutas en `server/internal/httpapi/handlers_stock_out.go` y `router.go` según contracts/api.md
- [X] T019 [P] [US1] Tests de pantalla en `web/src/features/se-acabo/SeAcaboPage.test.tsx`: segundo toque confirma, se desarma a los 4 s / al teclear, tocar otro mueve el armado, el hueco del deshacer no mueve los mosaicos, sin `quantity` en la petición, clave por toque
- [X] T020 [US1] Pantalla `web/src/features/se-acabo/SeAcaboPage.tsx` y `api.ts` (Maquetas: fila M2, opción B v2), ruta `/se-acabo` en `web/src/App.tsx`, renglón en `web/src/app/nav.ts` debajo de «Se echó a perder» por permiso

---

## Phase 4: User Story 2 — «No lo encuentro» (P1)

**Independent Test**: un texto sin coincidencias abre un aviso que solo ve quien configura; ninguna existencia cambia.

- [X] T021 [P] [US2] Tests en `server/internal/integration/stock_out_test.go`: no toca `stock_levels` ni `stock_movements`; visible solo con `inventory.configure`; resolver con `alias` usa la regla de otros nombres (`TestAliasFromNotFoundUsesTheAltNamesRules`), con `created` y con `dismiss`
- [X] T022 [US2] `ReportNotFound` y resolución `alias`/`created`/`dismiss` en `server/internal/app/stock_out.go` y `notifications.go`; ruta `POST /stock/not-found` y `POST /notifications/{id}/resolve`
- [X] T023 [US2] Fila «No lo encuentro» en `web/src/features/se-acabo/SeAcaboPage.tsx` (Maquetas M2, paso 7) y acciones «Dar de alta» (abre `NewArticleSheet` con el nombre), «Es otro nombre de…» (Picker de insumos), «Descartar» en el panel (T030)

---

## Phase 5: User Story 4 — Detección conectada y despachador (P1)

**Independent Test**: cada camino que mueve existencia abre o cierra; conteo, unión, archivado y cambio de unidad evalúan.

- [X] T024 [P] [US4] Tests `TestEveryMovementPathEvaluates` (venta, compra recibida, conteo con y sin diferencia, merma, ajuste, cancelación) y `TestMergeOrsLowStockAndClosesSourceNotifications`, `TestArchiveClosesNotifications`, `TestUnitChangeClosesAndReevaluates`, `TestOldArticlePatchEvaluatesLowStock` en `server/internal/integration/notifications_paths_test.go`
- [X] T025 [US4] Llamadas a `evaluate_low_stock` (después de los movimientos, misma transacción) en `CloseCount`/`closeProducts` (`stock_counts.go`, `stock_count_products.go`, que además resuelve los `stock_out` de lo contado), `MergeIngredients` (OR de la casilla, cierra avisos del origen), `ArchiveArticle`/`ReactivateArticle`, `ChangeUnit` y `UpdateArticle` en `server/internal/app/`
- [X] T026 [P] [US4] Test `TestNotificationListenerPublishesOnCommitOnly` en `server/internal/integration/notification_listener_test.go`: commit publica `notifications.changed`, rollback no
- [X] T027 [US4] `NotificationListener` en `server/internal/app/notification_listener.go` (conexión dedicada, `LISTEN notifications`, reconecta, termina con ctx) y cableado en `server/cmd/api/main.go`
- [X] T028 [US4] `TestLowStockTriggerLatency` en `server/internal/integration/notifications_trigger_test.go`: pedido de 30 renglones con receta sobre insumos con alerta, con y sin trigger, y `explain (analyze, buffers)` del filtro; imprime las cifras para el reporte

---

## Phase 6: User Story 5 — Campana y franja (P2)

**Independent Test**: dos personas, lectura por persona; franja espera hoja abierta y se va a los 90 s.

- [X] T029 [P] [US5] Tests de integración `TestUnreadIsPerPerson`, `TestBannerSeenIsPerPerson`, lista con `not_found` solo para `inventory.configure`, arreglos vacíos como `[]` en el JSON crudo, `status` desconocido 400, en `server/internal/integration/notifications_test.go`
- [X] T030 [US5] `NotificationsService` (lista, contador, leídas, franja vista, resolver `arrived`) en `server/internal/app/notifications.go`; handlers `server/internal/httpapi/handlers_notifications.go` y rutas
- [X] T031 [P] [US5] Tests de pantalla `web/src/features/notificaciones/Campana.test.tsx` y `CapaDeAvisos.test.tsx`: contador, panel con pestañas, motivo de cierre, franja difiere con hoja abierta, se quita a los 90 s, no sale a quien reportó, se apila bajo el aviso de pedido sin encimarse; test con admin a 1280×854 que fija las opciones del riel visibles
- [X] T032 [US5] Campana (icono 44 px) en `web/src/app/AppShell.tsx` y panel `web/src/features/notificaciones/PanelDeAvisos.tsx` (Maquetas M3, opción A v2); `useNotificationEvents` sobre SSE con `alReconectar` y relectura cada 60 s
- [X] T033 [US5] `CapaDeAvisos` (un portal, capa 2000) que apila `AvisoDePedidoEntrante` y `FranjaDeAgotado` en `web/src/features/notificaciones/` y su uso en `web/src/features/pos/AvisoDePlataforma.tsx` (Maquetas M4, opción B v2)

---

## Phase 7: User Story 3 — Alertas en la ficha (P1)

**Independent Test**: encender sobre un insumo bajo abre al instante con confirmación; apagar cierra; sin casilla no avisa.

- [X] T034 [P] [US3] Tests de integración `TestLowStockConfigPreviewAndSave` (vista previa sin guardar, guardado audita y evalúa, `409 STALE`), mínimo por sucursal (`branchMinStock` se guarda en `branch_stock_minimums` y gana sobre el de la empresa en la evaluación), batch con preview y con mínimos, `asked`, producto sin existencia propia 400, en `server/internal/integration/low_stock_config_test.go`
- [X] T035 [US3] Servicio y rutas `PUT /stock/articles/{id}/low-stock`, `PUT /products/{id}/low-stock`, `POST …/asked`, `POST /stock/low-stock/batch` en `server/internal/app/notifications.go` y `server/internal/httpapi/handlers_notifications.go`
- [X] T036 [P] [US3] Herramienta MCP `low_stock_update` con `dry_run` e `idempotency_key` en `server/internal/httpapi/mcp_tools_084.go`, `domain.MCPTools()` y su test `TestMCPLowStockUpdate`
- [X] T037 [P] [US3] Tests de pantalla `web/src/features/backoffice/articles/SeccionAlertas.test.tsx`: mínimo vacío ≠ 0, la confirmación dice si se avisará, pregunta al crear y al abrir una ficha nunca preguntada, «No, por ahora» no vuelve
- [X] T038 [US3] Renglón «Alertas» y hoja en `web/src/features/backoffice/articles/SeccionAlertas.tsx`, en `ArticleSheet.tsx` en lugar del campo «Mínimo», pregunta en `NewArticleSheet.tsx`, y la sección en la ficha de producto con existencia propia (`web/src/shared/ProductEditDialog.tsx`) (Maquetas M1, opción B)
- [X] T038b [US3] Hoja «Alertas por lote» con buscador, mínimo editable e interruptor, y su test `AlertasPorLote.test.tsx` (vacío ≠ 0, confirmación con el efecto por insumo), en `web/src/features/backoffice/articles/AlertasPorLote.tsx` y botón en `ArticlesTab.tsx` (Maquetas M8, opción B)

---

## Phase 8: User Story 6 — Correo (P2)

**Independent Test**: tres avisos en un bloque = un correo; enfriamiento; resumen una vez por día aunque se reinicie.

- [X] T039 [P] [US6] Tests `server/internal/mailtpl/mailtpl_test.go`: golden por plantilla, < 102 KB, colores de marca, logo PNG absoluto, escape de lo escrito por personas
- [X] T040 [US6] Plantillas `emails/layout.mjml`, `stock-window.mjml`, `stock-morning.mjml` (Maquetas M7, opción B), compiladas a `server/internal/mailtpl/templates/` y `server/internal/mailtpl/mailtpl.go` (`go:embed` + `html/template`)
- [X] T041 [P] [US6] Paso de CI en `.github/workflows/ci.yml` (y en `scripts/ci-local.sh`): `bun run compilar-correos && git diff --exit-code server/internal/mailtpl`
- [X] T042 [P] [US6] Tests con reloj falso en `server/internal/integration/notification_mailer_test.go`: `TestOneEmailPerWindow`, `TestResolvedBeforeTheWindowIsNotEmailed`, `TestCooldownPerItem`, `TestMorningSummaryOncePerDayAcrossRestarts`, `TestMorningSummaryUsesTheBusinessTimezone`, `TestAMailFailureRetriesAndNeverBlocksASale`, y dos procesos reclamando el mismo bloque
- [X] T043 [US6] `NotificationMailer` en `server/internal/app/notification_mailer.go` (bloques, enfriamiento, mañana con barrido de reevaluación, reclamo por fila) y cableado en `server/cmd/api/main.go`

---

## Phase 9: User Story 7 — Destinatarios (P2)

- [X] T044 [P] [US7] Tests `TestRecipientsByPermission` (rol propio sin permiso no recibe, persona sin correo no recibe, quitar a uno de omisión, correo extra sin usuario, por tipo), validación de correos y hora (400), auditoría, en `server/internal/integration/notification_recipients_test.go`
- [X] T045 [US7] Servicio y rutas `GET/PUT /settings/notification-recipients` en `server/internal/app/notifications.go` y `handlers_notifications.go`
- [X] T046 [US7] Pestaña «Avisos» en Almacén `web/src/features/notificaciones/Destinatarios.tsx` y `web/src/features/backoffice/StockPage.tsx` (Maquetas M5, opción A v2) con su test `Destinatarios.test.tsx`

---

## Phase 10: User Story 8 — Lista de compra (P3)

- [X] T047 [P] [US8] Tests de dominio `TestSuggestPurchase` (redondeo a empaques enteros del menor costo, sin consumo no sugiere, cobertura vacía = 14, existencia negativa pide conteo, el porqué) en `server/internal/domain/shopping_list_test.go`
- [X] T048 [US8] `domain.SuggestPurchase` en `server/internal/domain/shopping_list.go`
- [X] T049 [P] [US8] Test de integración `TestShoppingListExcludesOutReportsFromConsumption` y agrupado por proveedor de la última compra en `server/internal/integration/shopping_list_test.go`
- [X] T050 [US8] Consulta y ruta `GET /stock/shopping-list` (una sola consulta para pantalla y correo) y `POST /stock/shopping-list/email` («Enviarme por correo», al correo de quien lo pide, con el tope de envío por persona) en `server/internal/app/notifications.go`
- [X] T051 [US8] Pestaña «Lista de compra» en Almacén `web/src/features/notificaciones/ListaDeCompra.tsx` (Maquetas M6, opción B v2) con su test

---

## Phase 11: Polish

- [ ] T051b Regenerar `server/internal/integration/testdata/route_gates.golden` con TODAS las rutas nuevas (US1–US8) con `UPDATE_ROUTE_GATES=1`, revisar quién pasa por cada una y dejar el porqué en el PR

- [X] T052 e2e `web/e2e/notificaciones.spec.ts` a 1280×854 y 2560×1440 contra el `make start` del worktree: cajero reporta, gerente ve la campana en menos de 5 s sin recargar (SC-001, medido y reportado), correo en mailpit, lista de compra; capturas solo en `test-results/`; actualizar `docs/matriz-de-pantallas.md`
- [X] T053 Notas de área en `docs/mecanica/almacen.md` (trigger, orden de candados, filtro sin subtransacción) y `docs/mecanica/personal-y-permisos.md` (permiso nuevo)
- [X] T054 `/revision-de-codigo` (incluido `security-auditor`) y cada hallazgo con su test
- [X] T055 Prueba con usuario nuevo (cajero y gerente de un negocio de comida) a 1280×854 y 2K; correos en mailpit
- [X] T056 `make ci-local` verde

## Dependencies

Setup → Foundational → US1 → US2 → US4 → US5 → US3 → US6 → US7 → US8 → Polish. US7 y US8 solo
dependen de Foundational y pueden ir en paralelo a US6; la plantilla del correo (US6) usa la consulta
de la lista (US8) en el resumen de la mañana.

## Parallel Examples

- Foundational: T004, T005, T009, T012, T014 en paralelo; T006–T008 antes de T010.
- US1: T015, T016 y T019 en paralelo; luego T017 → T018 → T020.

## Implementation Strategy

Una sola entrega a producción (decisión del dueño). Se implementa por fases con los tests en verde
al cerrar cada una y commits pequeños.
