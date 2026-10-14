# Tasks: Salud del inventario

**Input**: [spec.md](spec.md), [plan.md](plan.md), [research.md](research.md), [data-model.md](data-model.md), [contracts/api.md](contracts/api.md), [quickstart.md](quickstart.md)

**Tests**: obligatorios (constitución IV, TDD). Cada test se escribe antes que su código y se ve fallar por la razón correcta.

**Formato**: `[ID] [P?] [Story] Descripción`. `[P]` = archivo distinto y sin dependencia pendiente.

---

## Phase 1: Setup

- [ ] T001 Levantar la infraestructura del worktree con `make worktree-env n=37` y `make start`; restaurar el respaldo de producción del 13-oct con `make db-restaurar` (base de SC-001), sin tocar la carpeta principal
- [ ] T002 Crear `server/queries/inventory_health.sql` vacío con su encabezado y confirmar que `make sqlc` corre limpio

---

## Phase 2: Foundational (bloquea a todas las historias)

- [ ] T003 [P] Test table-driven de `HealthFilter.Validate` (rango invertido, más de 366 días, fecha mal formada → `ErrValidation`; ausente → últimos 30 días; `section`, `limit`, `offset` fuera de rango → `ErrValidation`) en `server/internal/domain/inventory_health_test.go`
- [ ] T004 [P] Test table-driven de `ModeOf` (own_stock heredado, recipe, package, none confirmado, undecided) en `server/internal/domain/consumption_mode_test.go`
- [ ] T005 Implementar `HealthFilter`, `HealthIssueKind` en `server/internal/domain/inventory_health.go` y `ConsumptionMode`, `ModeOf` en `server/internal/domain/consumption_mode.go` hasta que T003 y T004 pasen
- [ ] T006 [P] Test de que `inventory.health_view` y `menu.configure` los tienen admin y gerente y no cajero ni cocina, en `server/internal/domain/permission_test.go`; luego agregar `PermInventoryHealthView` y `PermMenuConfigure` en `server/internal/domain/permission.go` y su espejo en `web/src/app/permissions.ts`
- [ ] T007 (movida a T030: la acción de bitácora se agrega en el mismo cambio que la produce, para que `TestEveryAuditActionIsProduced` nunca quede en rojo entre commits)

**Checkpoint**: dominio base listo.

---

## Phase 3: User Story 1 — Ver qué está mal en el inventario (P1) — MVP

**Goal**: la pestaña Almacén › Salud con sus cinco secciones, costo estimado, combos con varias opciones y negativos resumidos sin compras.

**Independent Test**: sobre el respaldo del 13-oct, la pantalla marca los 31 productos y los 126 insumos en negativo (SC-001), y cada renglón lleva a corregir en ≤2 toques (SC-002).

### Tests (primero, en rojo)

- [ ] T008 [P] [US1] Test de integración bajo `appRoleStore` con `inTheThreeCases` (otra empresa, conexión reciclada, sin empresa) para cada consulta de `inventory_health.sql`, con dos empresas, en `server/internal/integration/inventory_health_test.go`
- [ ] T009 [US1] Test de integración «se vende sin restar» contra el borde: un producto cuyo extra obligatorio descuenta NO se marca; la opción de ese grupo que no descuenta SÍ; lo confirmado «No gasta insumos» no; `platform_unpaired` sale en su propio contador; el componente de paquete que no descuenta se marca; renglones y pedidos cancelados o reembolsados no cuentan; un periodo sin ventas devuelve `salesInPeriod = 0` y las demás secciones intactas; piezas por canal y costo estimado (piezas × costo de hoy) correctos; en `server/internal/integration/inventory_health_test.go`
- [ ] T010 [US1] Test de integración de negativos (`collapsed = true` sin ningún movimiento `compra`, `false` con uno; sucursal visible solo con más de una), «sin compras» (resumen N de M), recetas con un error (cantidad cero, unidad de otro tipo, insumo archivado, duplicado por nombre sin acentos ni mayúsculas) y combos con un hueco de más de un producto; `noPurchasesSince` cuenta solo compras desde esa fecha (un insumo con compra anterior aparece, uno con compra posterior no); «Recetas pendientes de confirmar» cuenta productos y extras activos «Por revisar» por separado y no cuenta los confirmados ni los inactivos (FR-008), en `server/internal/integration/inventory_health_test.go`
- [ ] T011 [P] [US1] Test del handler sobre JSON crudo: todo arreglo viaja como `[]`, nunca `null`, con la empresa vacía; `from=2026-13-01` → 400; cajero → 403; cada tipo de renglón trae su `fixAt` correcto (producto y extra → la receta en Menú › Recetas con `?open=`, insumo → su ficha en Almacén, renglón de receta → la receta que lo contiene, pendientes → `?status=review`), que es el segundo y último toque de SC-002 (FR-009); en `server/internal/httpapi/handlers_inventory_health_test.go`
- [ ] T012 [US1] Test de regresión de FR-002a: guardar una receta nueva no modifica ningún `stock_movements` de tipo `venta` previo, en `server/internal/integration/inventory_health_test.go`
- [ ] T012a [US1] Test de permiso de `GET /stock/health` con un `PermissionResolver` de prueba (el que ya recibe `RequirePermission`): un rol inventado con `inventory.health_view` entra; admin con un resolver que se lo niega recibe 403. Prueba que decide el permiso y no el nombre del rol (FR-012), en `server/internal/httpapi/handlers_inventory_health_test.go`
- [ ] T012b [P] [US1] Test de que Menú › Recetas abre con el filtro de `?status=review` y con la receta de `?open=product:N` / `?open=option:N` ya abierta; un valor desconocido no rompe la pantalla y se ignora sin filtrar (FR-008, FR-009), en `web/src/features/admin/RecipesPage.test.tsx`
- [ ] T012c [P] [US1] Test de `StockPage`: la pestaña «Salud» aparece con `can('inventory.health_view')` y no aparece sin él, sin mirar el nombre del rol; las pestañas miden al menos 44 px; las tres pestañas de antes siguen funcionando dentro de `<Page fill>` (FR-012), en `web/src/features/backoffice/StockPage.test.tsx`
- [ ] T012d [P] [US1] Test de que `rolesPorPantalla` de Salud coincide con los roles que tienen `inventory.health_view` en el mapa de permisos, y falla nombrando la pantalla si divergen, en `server/internal/domain/uso_test.go`

### Implementación

- [ ] T013 [US1] Escribir las consultas (sin descontar con anti-join por `order_line_id` / `modifier_option_id`, sin agregar dos 1:N en la misma consulta, `coalesce` en todo agregado vaciable; negativos por sucursal; sin compras; recetas con un error; combos con varias opciones; conteo de productos y extras «Por revisar» (FR-008); existe alguna compra) y sus `Count…` con el mismo `where` en `server/queries/inventory_health.sql`; `make sqlc`
- [ ] T014 [US1] Implementar `InventoryHealthService.Report` (arreglos normalizados a vacío, costo estimado con `CostGraph` y `Round2`, ordenado por costo) en `server/internal/app/inventory_health.go`
- [ ] T015 [US1] Handler fino `GetInventoryHealth` y ruta `GET /stock/health` con `RequirePermission(PermInventoryHealthView)`, registrada FUERA del grupo `/stock` con `RequireRole`, hasta que T011 y T012a pasen, en `server/internal/httpapi/handlers_inventory_health.go` y `server/internal/httpapi/router.go`
- [ ] T016 [US1] Medir con `EXPLAIN (ANALYZE, BUFFERS)` como `gatobobah_app` sobre el respaldo del 13-oct; 30 días < 1 s; anotar el resultado en `specs/037-salud-del-inventario/research.md`
- [ ] T017 [P] [US1] Cliente `health()` y tipos (campos de arreglo opcionales) en `web/src/api/backoffice.ts`
- [ ] T018 [P] [US1] Tests de vitest de la pestaña (secciones que se abren, primera abierta, «El inventario está sano» solo con ventas en el periodo y cero problemas, «No hubo ventas en este periodo» sin ventas (y las demás secciones igual visibles), negativos con renglón explicativo, «Recetas pendientes de confirmar» con «Las cargó el sistema y nadie las ha confirmado» y «Ir a Recetas ›» hacia `?status=review` (FR-008), «Las ventas anteriores no se recalculan», combos con varias opciones con su pastilla «Siempre resta el de fábrica» (FR-002b), un periodo rechazado por el servidor muestra el mensaje y no cambia la lista (FR-010), textos de FR-019 tal cual, renglón completo como botón, sin `Page` anidado contando `max-width: 1150px`) en `web/src/features/backoffice/InventoryHealthTab.test.tsx`
- [ ] T019 [US1] `InventoryHealthTab` según plan.md › Maquetas, fila «Almacén › Salud» (opción A): `Box flex="1" minH={0}`, periodo con `Picker` y dos `DatePickerButton`; al desplegar «sin compras», un `DatePickerButton` «Sin compras desde» (ausente = nunca) que manda `noPurchasesSince`; renglones de 54 px, «Ver 25 más» que pide `section=…`, en `web/src/features/backoffice/InventoryHealthTab.tsx`
- [ ] T020 [US1] Hasta que T012c pase: `StockPage` pasa a `<Page fill>` con la pestaña «Salud» visible solo con `can('inventory.health_view')` (nunca por nombre de rol), y `Tabs.Trigger` con `minH="44px"` en `web/src/features/backoffice/StockPage.tsx`
- [ ] T021 [P] [US1] Menú › Recetas lee `?status=` y `?open=` de la URL, hasta que T012b pase, en `web/src/features/admin/RecipesPage.tsx`
- [ ] T022 [US1] Instrumentar la pantalla en los tres lugares: `pantallasMedibles` y `rolesPorPantalla` en `server/internal/domain/uso.go`, y `PANTALLAS` en `web/src/app/rutas-medidas.ts`. En `rolesPorPantalla` va con los roles que hoy tienen `inventory.health_view` (admin, gerente), derivados del mapa de permisos y no escritos a mano, hasta que T012d pase; que el mapa entero pase a permisos queda en el issue #61

**Checkpoint**: US1 funciona sola; escenarios 1, 2, 6 y 7 de quickstart.md.

---

## Phase 4: User Story 3 — Decidir qué resta cada producto (P1)

**Goal**: la hoja de receta con «Se compra hecho», la salida de la existencia propia con traslado o cierre, y la bitácora.

**Independent Test**: convertir un revendido en receta de 1 pieza, vender uno y ver que el insumo baja 1 (SC-003, SC-004); escenario 4 de quickstart.md.

### Tests (primero, en rojo)

- [ ] T023 [P] [US3] Test table-driven de `PlanModeChange`: traslado a un insumo sin existencia (N × cantidad, `Round4`); insumo con existencia sin `actualStock` → `ErrValidation`; con `actualStock` negativo o mal formado → `ErrValidation`; `seen…` distinto de lo bloqueado → `ErrConflict`; varios insumos, combo o «No gasta insumos» → cierre en cero; sin existencia propia → ningún ajuste; en `server/internal/domain/consumption_mode_test.go`
- [ ] T024 [P] [US3] Test de integración bajo `appRoleStore` con `inTheThreeCases` de la transición en `SaveComposition` (escribe `stock_levels` y `stock_movements`), con dos empresas: ajustes ligados con su nota, producto en cero, `track_stock` apagado, receta anterior conservada, un evento `product.consumption_mode` en la misma transacción; un combo cuyos componentes son revendidos ya convertidos resta cada insumo al venderse (Historia 3, escenario 4; SC-003); un cambio de receta a «No gasta insumos» sin existencia propia también deja su evento; guardar «Se prepara aquí» o «Es un combo» sin contenido → 400 (FR-014b); en `server/internal/integration/consumption_mode_test.go`
- [ ] T025 [US3] Test de concurrencia: una venta del producto entre mostrar y guardar → 409 y nunca deja el producto distinto de cero ni inventa existencia en el insumo; dos sucursales con existencia en los dos → 409; en `server/internal/integration/consumption_mode_test.go`
- [ ] T026 [US3] Test del hermano que no se movió: la carga de FUDO y `CloneProduct` siguen sin pasar por la transición y no cambian su resultado; un producto con existencia propia heredada que nadie tocó sigue restando de sí mismo al venderse (FR-013), en `server/internal/integration/consumption_mode_test.go`
- [ ] T027 [P] [US3] Test de que un cajero recibe 403 en las rutas `PUT …/composition` y `…/confirm` de producto y de extra aunque la petición no venga de la pantalla, y que con un `PermissionResolver` de prueba decide el permiso `menu.configure` y no el nombre del rol (FR-016), en `server/internal/httpapi/handlers_composition_test.go`
- [ ] T027a [US3] Tests en rojo de nombres de insumo sin duplicar (sin distinguir mayúsculas ni acentos), en `server/internal/integration/consumption_mode_test.go`: alta duplicada → 409; renombrar a un duplicado con `UpdateIngredient` → 409; dos altas concurrentes «Lata» / «lata» → exactamente una gana. Y en `web/src/features/backoffice/ExpenseDialog.test.tsx`: el alta desde el diálogo del gasto con un nombre que solo cambia mayúsculas o acentos muestra el aviso de nombre repetido que ya existe para el 409
- [ ] T027b [P] [US3] Test de que el renglón de Menú › Recetas de un producto «Se compra hecho» dice «Resta 1 por venta · tienes N» con N la existencia del insumo, y «Resta 6 por venta» cuando la receta lleva 6, en `web/src/features/admin/RecipesPage.test.tsx`

### Implementación

- [ ] T028 [US3] Implementar `PlanModeChange` en `server/internal/domain/consumption_mode.go` hasta que T023 pase
- [ ] T029 [US3] Consultas `LockStockLevelsForProduct` / `LockStockLevelsForIngredient` (`FOR UPDATE`, por sucursal) e insert de ajustes en `server/queries/stock.sql` (escritura de almacén; `inventory_health.sql` queda solo de lectura); `make sqlc`
- [ ] T029a [US3] Hasta que T027 pase: sacar las rutas `PUT …/composition` y `…/confirm` de producto y de extra de los grupos con `RequireRole` y dejarlas solo con `RequirePermission(PermMenuConfigure)`, en `server/internal/httpapi/router.go`
- [ ] T030a [US3] Nombres de insumo sin duplicar, hasta que T027a pase: `CreateIngredient` (`server/internal/app/articles.go`) **y** `UpdateIngredient` (`server/internal/app/costing.go`, el que renombra) rechazan con `ErrConflict` un nombre igual a otro insumo de la empresa sin distinguir mayúsculas ni acentos (hoy el `unique (company_id, name)` solo atrapa el idéntico). La normalización vive en `domain` y es la misma que usa «duplicado» en «Recetas con un error» (FR-007). Dentro de la transacción, antes de buscar, se toma el candado por empresa con llave propia (research R7d)
- [ ] T030 [US3] Agregar la acción `product.consumption_mode` (grupo `inventory`) a la lista blanca de `server/internal/domain/audit.go` y su etiqueta en `web/src/shared/audit/labels.ts`. En `SaveComposition`: después de `LockCompositionEdits`, bloquear filas, llamar a `PlanModeChange`, insertar ajustes, apagar `track_stock`, `recordAudit` en la misma tx; `validateComposition` deja de rechazar receta en `track_stock`; aceptar `actualStock`, `seenProductOnHand`, `seenIngredientOnHand`; en `server/internal/app/composition.go` y su handler
- [ ] T031 [P] [US3] Tests de vitest de la hoja: cuatro tarjetas; «Se compra hecho» preselecciona el insumo de nombre parecido y deja «Crear una nueva» al final; «¿Cuántas tienes de verdad?» en rejilla 2×2 con «Guardar» apagado hasta elegir; «Otra» con `scrollIntoView`; «Guardar» apagado en «Se prepara aquí» o «Es un combo» sin contenido (FR-014b); «Crear una nueva» con un nombre que ya existe ofrece el existente en vez de crear; aviso de latas solo si había existencia propia (FR-020); confirmación antes de dejar en cero (FR-014b); 409 vuelve a preguntar con cifras nuevas; en `web/src/shared/CompositionSheet.test.tsx`
- [ ] T032 [US3] `CompositionSheet` según plan.md › Maquetas, fila «Hoja de receta: modo de consumo» (B simplificada, versión 7): título en pregunta, tarjeta «Se compra hecho» que crea el insumo con `POST /stock/ingredients` solo como última opción, stepper detrás de «¿Una venta lleva más de una?», aviso de recetas por revisar con la frase de FR-019, en `web/src/shared/CompositionSheet.tsx`
- [ ] T033 [US3] Hasta que T027b pase: el renglón de Menú › Recetas muestra «Resta 1 por venta · tienes N» en `web/src/features/admin/RecipesPage.tsx`

**Checkpoint**: US1 + US3 = el MVP que corrige lo que encuentra.

---

## Phase 5: User Story 2 — Exportar (P2)

**Independent Test**: la exportación coincide renglón por renglón con la pantalla (SC-005).

- [ ] T034 [US2] Test que falla nombrando «la exportación y la pantalla divergen» (mismo servicio, mismo filtro, mismos renglones), más BOM, acentos y escape de celdas que empiezan con `=`, `+`, `-`, `@`; la exportación trae la lista COMPLETA de insumos sin compras aunque la pantalla la muestre resumida (FR-006), en `server/internal/integration/inventory_health_test.go`
- [ ] T034a [US2] Test del handler de `GET /stock/health.csv`: cajero → 403; con el permiso → 200 con `text/csv`, BOM y `Content-Disposition` con el periodo en el nombre; parámetro mal formado → 400 (FR-011, FR-012), en `server/internal/httpapi/handlers_inventory_health_test.go`
- [ ] T034b [P] [US2] Test de que «Exportar» descarga con el mismo periodo que muestra la pantalla y queda apagado mientras carga, en `web/src/features/backoffice/InventoryHealthTab.test.tsx`
- [ ] T035 [US2] `InventoryHealthService.CSV` con `encoding/csv` sobre el mismo `Report` sin paginar, en `server/internal/app/inventory_health.go`
- [ ] T036 [US2] Hasta que T034a pase: ruta `GET /stock/health.csv` con el mismo permiso y parámetros, fuera del grupo con `RequireRole`, en `server/internal/httpapi/handlers_inventory_health.go` y `router.go`
- [ ] T037 [US2] Hasta que T034b pase: botón «Exportar» que descarga con el periodo de la pantalla, en `web/src/features/backoffice/InventoryHealthTab.tsx`

---

## Phase 6: User Story 4 — Extra que es producto (P2)

**Independent Test**: un extra sin receta ni producto ligado se vende sin aviso y aparece en Salud.

- [ ] T038 [US4] Test de integración: vender un extra sin configurar no bloquea ni avisa (FR-017) y aparece con «Extra que no resta nada» y sus piezas; uno confirmado «No gasta insumos» no aparece; en `server/internal/integration/inventory_health_test.go`
- [ ] T039 [US4] Ajustar lo que falte en la consulta o en la pastilla de estado para que T038 pase (`server/queries/inventory_health.sql`, `web/src/features/backoffice/InventoryHealthTab.tsx`)

---

## Phase 7: Polish

- [ ] T040 [P] Test de FR-018 / SC-006: ningún nombre de producto o marca del negocio en el código ni en las migraciones de la feature (búsqueda en el diff), en `server/internal/integration/inventory_health_test.go` o script de CI
- [ ] T041 Capturas con Playwright a 1024×600 de las dos pantallas contra el ambiente de pruebas y un subagente de usuario nuevo que las use sin contexto
- [ ] T042 Actualizar AGENTS.md (bloque del almacén: «Salud del inventario», salida de la existencia propia con bloqueo de filas) y `docs/matriz-de-pantallas.md`
- [ ] T043 Gates: `make api-build && make api-test`, `make test-integration`, `bun run lint`, `bun run vitest run`, `bun run build`, `make ci-local`; correr quickstart.md completo
- [ ] T044 `/revision-de-codigo` (hook `after_implement`)

---

## Dependencies

- Phase 1 → Phase 2 → US1 y US3 (pueden ir en paralelo con dos cuidados: las dos tocan `RecipesPage.tsx` y su test (T012b/T021 antes de T027b/T033), y las dos tocan `router.go` (T015 antes de T029a; T036 de US2 después de los dos)).
- US2 depende de US1 (mismo servicio). US4 depende de US1 (misma consulta).
- Dentro de cada historia: tests en rojo → dominio → consultas → servicio → handler → web.

## Parallel examples

- Phase 2: T003, T004 y T006 a la vez.
- US1: T008, T011, T012b, T012c y T012d a la vez (archivos distintos); T009, T010 y T012 después de T008, y T012a después de T011, en el mismo archivo; luego T017, T018 y T021 en paralelo con T013–T015.
- US3: T023, T024, T027 y T027b a la vez (archivos distintos); T025, T026 y T027a después de T024, en el mismo archivo; T031 en paralelo con T028–T030.

## Implementation strategy

1. **MVP**: Phases 1–2 + US1 (ver el problema) + US3 (corregirlo). Validar con quickstart 1–4.
2. Después US2 (exportar) y US4 (extras), cada una entregable sola.
3. Polish y revisión de código al final.
