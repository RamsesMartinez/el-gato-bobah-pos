# Tasks: Salud del inventario

**Input**: [spec.md](spec.md), [plan.md](plan.md), [research.md](research.md), [data-model.md](data-model.md), [contracts/api.md](contracts/api.md), [quickstart.md](quickstart.md)

**Tests**: obligatorios (constitución IV, TDD). Cada test se escribe antes que su código y se ve fallar por la razón correcta.

**Formato**: `[ID] [P?] [Story] Descripción`. `[P]` = archivo distinto y sin dependencia pendiente.

---

## Phase 1: Setup

- [X] T001 Levantar la infraestructura del worktree con `make worktree-env n=37` y `make start`; restaurar el respaldo de producción del 13-oct con `make db-restaurar` (base de SC-001), sin tocar la carpeta principal
- [X] T002 Crear `server/queries/inventory_health.sql` vacío con su encabezado y confirmar que `make sqlc` corre limpio

---

## Phase 2: Foundational (bloquea a todas las historias)

- [X] T003 [P] Test table-driven de `HealthFilter.Validate` (rango invertido, más de 366 días, fecha mal formada → `ErrValidation`; ausente → últimos 30 días; `section`, `limit`, `offset` fuera de rango → `ErrValidation`) en `server/internal/domain/inventory_health_test.go`
- [X] T004 [P] Test table-driven de `ModeOf` (own_stock heredado, recipe, package, none confirmado, undecided) en `server/internal/domain/consumption_mode_test.go`
- [X] T005 Implementar `HealthFilter`, `HealthIssueKind` en `server/internal/domain/inventory_health.go` y `ConsumptionMode`, `ModeOf` en `server/internal/domain/consumption_mode.go` hasta que T003 y T004 pasen
- [X] T006 [P] Test de que `inventory.health_view` y `menu.configure` los tienen admin y gerente y no cajero ni cocina, en `server/internal/domain/permission_test.go`
- [X] T007 Hasta que T006 pase: agregar `PermInventoryHealthView` y `PermMenuConfigure` en `server/internal/domain/permission.go` y su espejo en `web/src/app/permissions.ts`

**Checkpoint**: dominio base listo.

---

## Phase 3: User Story 1 — Ver qué está mal en el inventario (P1) — MVP

**Goal**: la pestaña Almacén › Salud con sus cinco secciones, costo estimado, combos con varias opciones y negativos resumidos sin compras.

**Independent Test**: sobre el respaldo del 13-oct, la pantalla marca lo vendido sin restar, los 126 insumos en negativo y los productos sin decidir (SC-001), y cada renglón lleva a corregir en ≤2 toques (SC-002).

### Tests (primero, en rojo)

- [X] T008 [P] [US1] Test de integración bajo `appRoleStore` con `inTheThreeCases` (otra empresa, conexión reciclada, sin empresa) para cada consulta de `inventory_health.sql`, con dos empresas, en `server/internal/integration/inventory_health_test.go`
- [X] T009 [US1] Test de integración «se vende sin restar» contra el borde: un producto cuyo extra obligatorio descuenta NO se marca; la opción de ese grupo que no descuenta SÍ; lo confirmado «No gasta insumos» no; `platform_unpaired` sale en su propio contador; el componente de paquete que no descuenta se marca; renglones y pedidos cancelados o reembolsados no cuentan; un periodo sin ventas devuelve `salesInPeriod = 0` y las demás secciones intactas; piezas por canal y costo estimado (piezas × costo de hoy) correctos; en `server/internal/integration/inventory_health_test.go`
- [X] T010 [US1] Test de integración de negativos (`collapsed = true` sin ningún movimiento `compra`, `false` con uno; sucursal visible solo con más de una), «sin compras» (resumen N de M), recetas con un error (cantidad cero, unidad de otro tipo, insumo archivado, duplicado por nombre sin acentos ni mayúsculas) y combos con un hueco de más de un producto; `noPurchasesSince` cuenta solo compras desde esa fecha (un insumo con compra anterior aparece, uno con compra posterior no); «Recetas pendientes» da dos cuentas —sin decidir (vendidos o no) y cargadas por el sistema—, cada una de productos y de extras activos, iguales a los conteos de las pestañas de Menú › Recetas, y no cuenta los confirmados ni los inactivos (FR-008), en `server/internal/integration/inventory_health_test.go`
- [X] T011 [P] [US1] Test del handler sobre JSON crudo: todo arreglo viaja como `[]`, nunca `null`, con la empresa vacía; `from=2026-13-01` → 400; cajero → 403; cada tipo de renglón trae su `fixAt` correcto (producto y extra → la receta en Menú › Recetas con `?open=`, insumo → su ficha en Almacén, renglón de receta → la receta que lo contiene, pendientes → `?status=review`), que es el segundo y último toque de SC-002 (FR-009); en `server/internal/integration/inventory_health_http_test.go` (por el router real: necesita base)
- [X] T012 [US1] Test de regresión de FR-002a: guardar una receta nueva no modifica ningún `stock_movements` de tipo `venta` previo, en `server/internal/integration/inventory_health_test.go`
- [X] T013 [US1] Test de permiso de `GET /stock/health` con un `PermissionResolver` de prueba (el que ya recibe `RequirePermission`): un rol inventado con `inventory.health_view` entra; admin con un resolver que se lo niega recibe 403. Prueba que decide el permiso y no el nombre del rol (FR-012), en `server/internal/integration/inventory_health_http_test.go` (por el router real: necesita base)
- [X] T014 [P] [US1] Test de que Menú › Recetas abre con el filtro de `?status=pending` o `?status=review` y con la receta de `?open=product:N` / `?open=option:N` ya abierta; un valor desconocido no rompe la pantalla y se ignora sin filtrar (FR-008, FR-009), en `web/src/features/admin/RecipesPage.test.tsx`
- [X] T015 [P] [US1] Test de `StockPage`: la pestaña «Salud» aparece con `can('inventory.health_view')` y no aparece sin él, sin mirar el nombre del rol; las pestañas miden al menos 44 px; las tres pestañas de antes siguen funcionando dentro de `<Page fill>` (FR-012), en `web/src/features/backoffice/StockPage.test.tsx`
- [X] T016 [P] [US1] Test de que «ver-salud» y «exportar-salud» son acciones medibles de «inventario» y de que todo rol con `inventory.health_view` puede abrir esa pantalla (la pestaña Salud no es una ruta: se mide como acciones de Almacén, decidido al implementar), en `server/internal/domain/uso_test.go`
- [X] T017 [P] [US1] Tests de vitest de la pestaña (secciones que se abren, primera abierta, «El inventario está sano» solo con ventas en el periodo y cero problemas, «No hubo ventas en este periodo» sin ventas (y las demás secciones igual visibles), negativos con renglón explicativo, «Recetas pendientes» con «Falta decir qué resta» hacia `?status=pending` y «Las cargó el sistema y nadie las ha confirmado» hacia `?status=review` (FR-008), «Las ventas anteriores no se recalculan», combos con varias opciones con su pastilla «Siempre resta el de fábrica» (FR-002b), un periodo rechazado por el servidor muestra el mensaje y no cambia la lista (FR-010), textos de FR-019 tal cual, renglón completo como botón, sin `Page` anidado contando `max-width: 1150px`) en `web/src/features/backoffice/InventoryHealthTab.test.tsx`

### Implementación

- [X] T018 [US1] Escribir las consultas (sin descontar con anti-join por `order_line_id` / `modifier_option_id`, sin agregar dos 1:N en la misma consulta, `coalesce` en todo agregado vaciable; negativos por sucursal; sin compras; recetas con un error; combos con varias opciones; conteos de productos y extras sin decidir y cargados por el sistema con el mismo criterio que `CountProductRecipes`/`CountExtraRecipes` (FR-008); existe alguna compra) y sus `Count…` con el mismo `where` en `server/queries/inventory_health.sql`; `make sqlc`
- [X] T019 [US1] Implementar `InventoryHealthService.Report` (arreglos normalizados a vacío, costo estimado con `CostGraph` y `Round2`, ordenado por costo) en `server/internal/app/inventory_health.go`
- [X] T020 [US1] Handler fino `GetInventoryHealth` y ruta `GET /stock/health` con `RequirePermission(PermInventoryHealthView)`, registrada FUERA del grupo `/stock` con `RequireRole`, hasta que T011 y T013 pasen, en `server/internal/httpapi/handlers_inventory_health.go` y `server/internal/httpapi/router.go`
- [X] T021 [US1] Medir con `EXPLAIN (ANALYZE, BUFFERS)` como `gatobobah_app` sobre el respaldo del 13-oct; 30 días < 1 s; anotar el resultado en `specs/037-salud-del-inventario/research.md`
- [X] T022 [P] [US1] Cliente `health()` y tipos (campos de arreglo opcionales) en `web/src/api/backoffice.ts`
- [X] T023 [US1] `InventoryHealthTab` según plan.md › Maquetas, fila «Almacén › Salud» (opción A): `Box flex="1" minH={0}`, periodo con `Picker` y dos `DatePickerButton`; al desplegar «sin compras», un `DatePickerButton` «Sin compras desde» (ausente = nunca) que manda `noPurchasesSince`; renglones de 54 px, «Ver 25 más» que pide `section=…`, en `web/src/features/backoffice/InventoryHealthTab.tsx`
- [X] T024 [US1] Hasta que T015 pase: `StockPage` pasa a `<Page fill>` con la pestaña «Salud» visible solo con `can('inventory.health_view')` (nunca por nombre de rol), y `Tabs.Trigger` con `minH="44px"` en `web/src/features/backoffice/StockPage.tsx`
- [X] T025 [P] [US1] Menú › Recetas lee `?status=` y `?open=` de la URL, hasta que T014 pase, en `web/src/features/admin/RecipesPage.tsx`
- [X] T026 [US1] Hasta que T016 pase: acciones «ver-salud» y «exportar-salud» en `accionesMedibles` de [uso.go](server/internal/domain/uso.go), sus etiquetas en `web/src/consola/etiquetas-de-uso.ts`, y `medirAccion('inventario', 'ver-salud')` al abrir la pestaña. Sin pantalla nueva en `rolesPorPantalla` ni en `rutas-medidas.ts`: la pestaña vive en `/almacen`, que ya se mide

**Checkpoint**: US1 funciona sola; escenarios 1, 2, 6 y 7 de quickstart.md.

---

## Phase 4: User Story 3 — Decidir qué resta cada producto (P1)

**Goal**: la hoja de receta con «Se compra hecho», la salida de la existencia propia con traslado o cierre, y la bitácora.

**Independent Test**: convertir un revendido en receta de 1 pieza, vender uno y ver que el insumo baja 1 (SC-003, SC-004); escenario 4 de quickstart.md.

### Tests (primero, en rojo)

- [X] T027 [P] [US3] Test table-driven de `PlanModeChange`: traslado a un insumo sin existencia (N × cantidad, `Round4`); insumo con existencia sin `actualStock` → `ErrValidation`; con `actualStock` negativo o mal formado → `ErrValidation`; `seen…` distinto de lo bloqueado → `ErrConflict`; varios insumos, combo o «No gasta insumos» → cierre en cero; sin existencia propia → ningún ajuste; en `server/internal/domain/consumption_mode_test.go`
- [X] T028 [P] [US3] Test de integración bajo `appRoleStore` con `inTheThreeCases` de la transición en `SaveComposition` (escribe `stock_levels` y `stock_movements`), con dos empresas: ajustes ligados con su nota, producto en cero, `track_stock` apagado, receta anterior conservada, un evento `product.consumption_mode` en la misma transacción; un combo cuyos componentes son revendidos ya convertidos resta cada insumo al venderse (Historia 3, escenario 4; SC-003); un cambio de receta a «No gasta insumos» sin existencia propia también deja su evento; guardar «Se prepara aquí» o «Es un combo» sin contenido → 400 (FR-014b); en `server/internal/integration/consumption_mode_test.go`
- [X] T029 [US3] Test de concurrencia: una venta del producto entre mostrar y guardar → 409 y nunca deja el producto distinto de cero ni inventa existencia en el insumo; dos sucursales con existencia en los dos → 409; en `server/internal/integration/consumption_mode_test.go`
- [X] T030 [US3] Test del hermano que no se movió: la carga de FUDO y `CloneProduct` siguen sin pasar por la transición y no cambian su resultado; un producto con existencia propia heredada que nadie tocó sigue restando de sí mismo al venderse (FR-013), en `server/internal/integration/consumption_mode_test.go`
- [X] T031 [P] [US3] Test de que un cajero recibe 403 en las rutas `PUT …/composition` y `…/confirm` de producto y de extra aunque la petición no venga de la pantalla, y que con un `PermissionResolver` de prueba decide el permiso `menu.configure` y no el nombre del rol (FR-016), en `server/internal/httpapi/handlers_composition_test.go`
- [X] T032 [US3] Tests en rojo de nombres de insumo sin duplicar (sin distinguir mayúsculas, acentos ni espacios de más), en `server/internal/integration/consumption_mode_test.go`: alta duplicada → 409; dos altas concurrentes «Jarabe» / «jarabe» → exactamente una gana. Y en `web/src/shared/createIngredient.test.ts`: con un 409, el alta desde el gasto o la receta elige el que ya existe y lo dice (al implementar se vio que ese aviso no existía: el error se perdía). Renombrar a un duplicado no se prueba: ninguna ruta renombra insumos hoy (la consulta `UpdateIngredient` existe y nadie la llama); quien agregue esa ruta aplica la misma regla
- [X] T033 [P] [US3] Test de que el renglón de Menú › Recetas de un producto «Se compra hecho» dice «Resta 1 por venta · tienes N» con N la existencia del insumo, y «Resta 6 por venta» cuando la receta lleva 6, en `web/src/features/admin/RecipesPage.test.tsx`
- [X] T034 [P] [US3] Tests de vitest de la hoja: cuatro tarjetas; «Se compra hecho» preselecciona el insumo de nombre parecido y deja «Crear una nueva» al final; «¿Cuántas tienes de verdad?» en rejilla 2×2 con «Guardar» apagado hasta elegir; «Otra» con `scrollIntoView`; «Guardar» apagado en «Se prepara aquí» o «Es un combo» sin contenido (FR-014b); «Crear una nueva» con un nombre que ya existe ofrece el existente en vez de crear; aviso de latas solo si había existencia propia (FR-020); confirmación antes de dejar en cero (FR-014b); 409 vuelve a preguntar con cifras nuevas; una receta por revisar muestra el aviso con «Las cargó el sistema y nadie las ha confirmado» tal cual (FR-019); «¿Una venta lleva más de una?» muestra el selector oculto y guardar manda la cantidad elegida; en `web/src/shared/CompositionSheet.test.tsx`

### Implementación

- [X] T035 [US3] Implementar `PlanModeChange` en `server/internal/domain/consumption_mode.go` hasta que T027 pase
- [X] T036 [US3] Consultas `LockStockLevelsForProduct` / `LockStockLevelsForIngredient` (`FOR UPDATE`, por sucursal) e insert de ajustes en `server/queries/stock.sql` (escritura de almacén; `inventory_health.sql` queda solo de lectura); `make sqlc`
- [X] T037 [US3] Hasta que T031 pase: sacar las rutas `PUT …/composition` y `…/confirm` de producto y de extra de los grupos con `RequireRole` y dejarlas solo con `RequirePermission(PermMenuConfigure)`, en `server/internal/httpapi/router.go`
- [X] T038 [US3] Nombres de insumo sin duplicar, hasta que T032 pase: `CreateIngredient` (`server/internal/app/articles.go`) rechaza con `ErrConflict` un nombre igual a otro insumo de la empresa sin distinguir mayúsculas, acentos ni espacios de más (`IngredientNameTaken`, la misma normalización que «duplicado» en «Recetas con un error»), detrás de un candado por empresa con llave propia (research R7d). `web/src/shared/createIngredient.ts` lo usan el diálogo del gasto y la hoja de receta
- [X] T039 [US3] Agregar la acción `product.consumption_mode` (grupo `inventory`) a la lista blanca de `server/internal/domain/audit.go` y su etiqueta en `web/src/shared/audit/labels.ts`. En `SaveComposition`: después de `LockCompositionEdits`, bloquear filas, llamar a `PlanModeChange`, insertar ajustes, apagar `track_stock`, `recordAudit` en la misma tx; `validateComposition` deja de rechazar receta en `track_stock`; aceptar `actualStock`, `seenProductOnHand`, `seenIngredientOnHand`; en `server/internal/app/composition.go` y su handler
- [X] T040 [US3] `CompositionSheet` según plan.md › Maquetas, fila «Hoja de receta: modo de consumo» (B simplificada, versión 7): título en pregunta, tarjeta «Se compra hecho» que crea el insumo con `POST /stock/ingredients` solo como última opción, stepper detrás de «¿Una venta lleva más de una?», aviso de recetas por revisar con la frase de FR-019, en `web/src/shared/CompositionSheet.tsx`
- [X] T041 [US3] Hasta que T033 pase: el renglón de Menú › Recetas muestra «Resta 1 por venta · tienes N» en `web/src/features/admin/RecipesPage.tsx`

**Checkpoint**: US1 + US3 = el MVP que corrige lo que encuentra.

---

## Phase 5: User Story 2 — Exportar (P2)

**Independent Test**: la exportación coincide renglón por renglón con la pantalla (SC-005).

- [X] T042 [US2] Test que falla nombrando «la exportación y la pantalla divergen» (mismo servicio, mismo filtro, mismos renglones), más BOM, acentos y escape de celdas que empiezan con `=`, `+`, `-`, `@`; la exportación trae la lista COMPLETA de insumos sin compras aunque la pantalla la muestre resumida (FR-006); y los cuatro renglones de resumen de «Recetas pendientes» con las mismas cuentas que la pantalla (FR-008, FR-011), en `server/internal/integration/inventory_health_test.go`
- [X] T043 [US2] Test del handler de `GET /stock/health.csv`: cajero → 403; con el permiso → 200 con `text/csv`, BOM y `Content-Disposition` con el periodo en el nombre; parámetro mal formado → 400 (FR-011, FR-012), en `server/internal/integration/inventory_health_http_test.go` (por el router real: necesita base)
- [X] T044 [P] [US2] Test de que «Exportar» descarga con el mismo periodo que muestra la pantalla y queda apagado mientras carga, en `web/src/features/backoffice/InventoryHealthTab.test.tsx`
- [X] T045 [US2] `InventoryHealthService.CSV` con `encoding/csv` sobre el mismo `Report` sin paginar, en `server/internal/app/inventory_health.go`
- [X] T046 [US2] Hasta que T043 pase: ruta `GET /stock/health.csv` con el mismo permiso y parámetros, fuera del grupo con `RequireRole`, en `server/internal/httpapi/handlers_inventory_health.go` y `router.go`
- [X] T047 [US2] Hasta que T044 pase: botón «Exportar» que descarga con el periodo de la pantalla, en `web/src/features/backoffice/InventoryHealthTab.tsx`

---

## Phase 6: User Story 4 — Extra que es producto (P2)

**Independent Test**: un extra sin receta ni producto ligado se vende sin aviso y aparece en Salud.

- [X] T048 [US4] Test de integración: vender un extra sin configurar no bloquea ni avisa (FR-017) y aparece con «Extra que no resta nada» y sus piezas; uno confirmado «No gasta insumos» no aparece; en `server/internal/integration/inventory_health_test.go`
- [X] T049 [US4] Ajustar lo que falte en la consulta o en la pastilla de estado para que T048 pase (`server/queries/inventory_health.sql`, `web/src/features/backoffice/InventoryHealthTab.tsx`)

---

## Phase 7: Polish

- [X] T050 [P] Test de FR-018 / SC-006: ningún nombre de producto o marca del negocio en el código ni en las migraciones de la feature (búsqueda en el diff), en `server/internal/integration/inventory_health_test.go` o script de CI
- [ ] T051 Capturas con Playwright a 1024×600 de las dos pantallas contra el ambiente de pruebas y un subagente de usuario nuevo que las use sin contexto
- [X] T052 Actualizar AGENTS.md (bloque del almacén: «Salud del inventario», salida de la existencia propia con bloqueo de filas) y `docs/matriz-de-pantallas.md`
- [X] T053 Gates: `make api-build && make api-test`, `make test-integration`, `bun run lint`, `bun run vitest run`, `bun run build`, `make ci-local`; correr quickstart.md completo
- [ ] T054 `/revision-de-codigo` (hook `after_implement`)

---

## Dependencies

- Phase 1 → Phase 2 → US1 y US3 (pueden ir en paralelo con dos cuidados: las dos tocan `RecipesPage.tsx` y su test (T014/T025 antes de T033/T041), y las dos tocan `router.go` (T020 antes de T037; T046 de US2 después de los dos)).
- US2 depende de US1 (mismo servicio). US4 depende de US1 (misma consulta).
- Dentro de cada historia: tests en rojo → dominio → consultas → servicio → handler → web.

## Parallel examples

- Phase 2: T003, T004 y T006 a la vez; T005 y T007 después.
- US1: T008, T011, T014, T015, T016 y T017 a la vez (archivos distintos); T009, T010 y T012 después de T008, y T013 después de T011, en el mismo archivo; luego T022 y T025 en paralelo con T018–T020.
- US3: T027, T028, T031, T033 y T034 a la vez (archivos distintos); T029, T030 y T032 después de T028, en el mismo archivo.

## Implementation strategy

1. **MVP**: Phases 1–2 + US1 (ver el problema) + US3 (corregirlo). Validar con quickstart 1–4.
2. Después US2 (exportar) y US4 (extras), cada una entregable sola.
3. Polish y revisión de código al final.
