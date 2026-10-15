# Tasks: Merma y faltante sin explicar

**Input**: [spec.md](spec.md), [plan.md](plan.md), [research.md](research.md), [data-model.md](data-model.md), [contracts/api.md](contracts/api.md), [quickstart.md](quickstart.md)

TDD estricto (constitución IV): cada test se escribe y se ve fallar por la razón correcta antes de su
implementación. Las tareas de pantalla citan la fila de «Maquetas» del plan que implementan.

## Phase 1: Setup

- [X] T001 Confirmar que `0094` es la siguiente migración libre en `server/migrations/` y que el worktree tiene Postgres de pruebas y una base restaurada con dos empresas (`make respaldo-anonimo` a `gatobobah_restaurado`)

## Phase 2: Foundational (bloquea a todas las historias)

- [X] T002 [P] Test de dominio de estados y transiciones de merma (`applied`, `pending`, `rejected`, `covered`, `undone`; las no listadas → `ErrConflict`), de «¿pide aprobación?» (sin tope, bajo, sobre, sin costo) y de «¿se puede deshacer?» (mismo día, insumo no contado después) en `server/internal/domain/waste_test.go`
- [X] T003 [P] Test de dominio de `SplitLoss` (perdido = registrado + sin explicar; «sobraba»; sin costo; primer conteo) nombrando el concepto que se duplicaría, y de la normalización del nombre de motivo, en `server/internal/domain/waste_test.go`
- [X] T004 [P] Test de que `inventory.write_off` lo tienen admin, gerente, cajero y mesero, e `inventory.write_off_approve` solo admin y gerente, en `server/internal/domain/permission_test.go`
- [X] T005 Test de la migración 0094 sobre el respaldo restaurado con dos empresas: tablas con RLS, `nullif`, grants sin `delete`; motivos sembrados una vez por empresa; los checks rechazan estado inválido, cantidad 0, costo a medias y `covered` sin conteo; el índice de nombre rechaza un duplicado con otra mayúscula o acento; el Down se niega con registros y deja todo limpio sin ellos, en `server/internal/integration/migration_0094_test.go`
- [X] T006 Migración `server/migrations/0094_waste.sql` según `data-model.md` (tablas, FKs con su `on delete`, checks, índices, columnas nuevas en `stock_movements`, `stock_count_lines` y `business_settings`, siembra, Down protegido) hasta que T005 pase
- [X] T007 Siembra de motivos en el CTE de `CreateCompany` en `server/queries/companies.sql` y test de que una empresa nueva nace con sus cinco motivos en `server/internal/integration/companies_test.go`
- [X] T008 `domain/waste.go`: estados, transiciones, regla de aprobación y de deshacer, `SplitLoss`, normalización de nombre, hasta que T002 y T003 pasen
- [X] T009 Permisos `PermInventoryWriteOff` y `PermInventoryWriteOffApprove` en `server/internal/domain/permission.go` (con su mensaje de permiso negado) y su espejo en `web/src/app/permissions.ts` hasta que T004 pase
- [X] T010 Acciones de bitácora `waste.recorded`, `waste.approved`, `waste.rejected`, `waste.undone`, `settings.waste_threshold` en `server/internal/domain/audit.go` y sus etiquetas en `web/src/shared/audit/labels.ts`
- [X] T011 Consultas en `server/queries/waste.sql` (motivos con usos, insertar registro y movimientos con `branch_id` explícito y `clock_timestamp()`, `for update` del registro, pendientes de la sucursal, registros del periodo, por motivo siguiendo `merged_into_id`) y en `server/queries/stock_counts.sql` (`last_movement_id` al capturar, costo al cerrar, faltante por id de movimiento); `make sqlc`

## Phase 3: User Story 1 — Registrar lo que se echó a perder (P1) 🎯 MVP

**Independent test**: insumo con 10, registrar 2 con «Caducó» → quedan 8, con motivo, quién y cuándo; con tope, una cara queda por aprobar y no resta.

- [X] T012 [P] [US1] Tests de integración de registrar bajo `appRoleStore` en los tres casos (`inTheThreeCases`): insumo en unidad y en empaque, producto con receta (resta como venderlo, no aparece en Ventas), producto sin nada que restar → 422, motivo/insumo/producto de otra empresa → 404, en `server/internal/integration/waste_record_test.go`
- [X] T013 [P] [US1] Tests de concurrencia y sucursal: registrar con una venta del mismo insumo en curso → 409 sin esperar; dos sucursales sin selector → `BRANCH_AMBIGUOUS`; el movimiento lleva la sucursal del registro (E18), en `server/internal/integration/waste_concurrency_test.go`
- [X] T014 [P] [US1] Tests de aprobación: sobre el tope queda `pending` y no resta; aprobar resta y fija el costo si no tenía; rechazar no resta; aprobar dos veces → 409; sin tope nada pide aprobación; insumo sin costo con tope → pendiente, en `server/internal/integration/waste_approve_test.go`
- [X] T015 [P] [US1] Tests de deshacer: devuelve exactamente lo restado con movimiento inverso y nunca borra; dos veces → 409; al día siguiente o con el insumo ya contado → 409; pendiente se deshace sin devolver nada; solo quien registró o quien tiene `inventory.configure`, en `server/internal/integration/waste_undo_test.go`
- [X] T016 [P] [US1] Tests del handler por el router real: permisos (cocina/cajero registran, no aprueban), cantidad vacía/0/negativa/NaN/enorme → 400, nota larga → 400, arreglos `[]` en JSON crudo, en `server/internal/integration/waste_http_test.go`
- [X] T017 [P] [US1] Test de que cada acción de bitácora nueva la produce su camino en la misma transacción (lo exige `TestEveryAuditActionIsProduced`), en `server/internal/integration/waste_audit_test.go`
- [X] T018 [US1] Servicio en `server/internal/app/waste.go`: `Record` (con `lockIngredients` del 038 y `ExpandSale` para producto), `Approve`, `Reject`, `Undo`, `Today`, `Pending`, motivos (listar y crear con el 23505 → 409 con el id existente), hasta que T012–T015 y T017 pasen
- [X] T019 [US1] Rutas en `server/internal/httpapi/handlers_waste.go` y `router.go` con `RequirePermission`, fuera de grupos con rol, hasta que T016 pase
- [X] T020 [US1] Tope en `PUT /settings/inventory` (`server/internal/app/settings.go`) con `ValidMoney` y su evento `settings.waste_threshold`, y test en `server/internal/integration/inventory_settings_http_test.go`
- [X] T021 [P] [US1] Medición de uso de la pantalla y sus acciones en `server/internal/domain/uso.go` (`pantallasMedibles` y `rolesPorPantalla`) y `web/src/app/rutas-medidas.ts`
- [X] T022 [P] [US1] Tests de vitest del riel: la entrada «Se echó a perder» sale con `inventory.write_off` y no sin él, va después de «Reportes» para el admin, y lleva el número de pendientes para quien aprueba, en `web/src/app/AppShell.test.tsx`
- [X] T023 [US1] Campo `permission` en `NAV` (`web/src/app/nav.ts`) y filtro con `can()` en `web/src/app/AppShell.tsx` hasta que T022 pase
- [X] T024 [P] [US1] Tests de vitest de la captura (Maquetas: «Se echó a perder», opción D): llega «Una pantalla» y el interruptor recuerda «Paso a paso» por tableta; motivos como botones y «Otro motivo…» pide descripción; el artículo se elige en `Picker`; cantidad con teclado (`inputMode="none"`) y empaques; «Mandar a aprobar» cuando pasa el tope; «Hoy» con Deshacer a la derecha y confirmación; sin `<select>` ni campos nativos, en `web/src/features/waste/WastePage.test.tsx`
- [X] T025 [US1] Pantalla `web/src/features/waste/WastePage.tsx` (vistas «Una pantalla» y «Paso a paso», `p=2`, columnas con `minH={0}` y su propio desplazamiento, Guardar fijo al pie) y llamadas en `web/src/api/backoffice.ts`, hasta que T024 pase
- [X] T026 [P] [US1] Tests de vitest de por aprobar (Maquetas: «Por aprobar», opción B): franja de 44 px solo con `inventory.write_off_approve`, solo con pendientes, no en `/pos` ni `/pedidos`, se cierra por sesión; la hoja aprueba y rechaza («Rechazar» apartado y con confirmación), en `web/src/features/waste/PendingWasteBar.test.tsx`
- [X] T027 [US1] Franja y hoja en `web/src/features/waste/PendingWasteBar.tsx`, montada como fila del shell en `web/src/app/AppShell.tsx` (`Outlet` con `flex=1 minH={0}`), hasta que T026 pase

## Phase 4: User Story 2 — Faltante sin explicar después de un conteo (P2)

**Independent test**: dos conteos cerrados con 3 de merma registrada entre ellos y el segundo encontró 2 de menos → perdido 5, registrado 3, sin explicar 2, en pesos al costo del cierre.

- [X] T028 [P] [US2] Tests de integración del faltante en los tres casos: partido correcto (E6); primer conteo; sobraba; sin costo; la merma que confirmó después de la captura cae en el periodo siguiente con dos transacciones (E14); la deshecha, rechazada y cubierta no cuentan; el reporte cerrado no cambia si cambia el costo después (E19), en `server/internal/integration/waste_loss_test.go`
- [X] T029 [P] [US2] Tests de cobertura al cerrar: la pendiente registrada antes de la captura queda `covered` y ya no se aprueba; la registrada entre captura y cierre sigue pendiente (E15); un conteo descartado no cubre nada, en `server/internal/integration/waste_covered_test.go`
- [X] T030 [US2] `CaptureCount` guarda `last_movement_id` con el insumo tomado y `CloseCount` guarda el costo y cubre en su transacción (`server/internal/app/stock_counts.go`); reporte en `server/internal/app/waste_reports.go`; ruta `GET /stock/counts/{id}/loss`, hasta que T028 y T029 pasen
- [X] T031 [P] [US2] Tests de vitest del faltante (Maquetas: «Faltante sin explicar», opción B): tres cifras arriba, una barra por insumo (registrado gris, sin explicar rojo), «Sobraba» sin barra, «sin costo guardado» en conteos viejos, primer conteo sin cifras inventadas, en `web/src/features/backoffice/CountLossReport.test.tsx`
- [X] T032 [US2] Reporte en `web/src/features/backoffice/CountLossReport.tsx`, abierto al cerrar un conteo y desde los conteos cerrados en `StockCountTab.tsx`, hasta que T031 pase

## Phase 5: User Story 3 — Por motivo y los motivos de la empresa (P3)

**Independent test**: mermas de la semana con dos motivos → cada uno con su total y el del personal aparte; juntar dos motivos suma bajo el que queda sin cambiar el motivo de ningún registro.

- [X] T033 [P] [US3] Tests de integración por motivo y motivos en los tres casos: totales por motivo siguiendo `merged_into_id`; juntar no cambia `reason_id` (E16) y rechaza destino juntado o archivado; archivar no cambia lo registrado; cambiar «no cuenta como pérdida» no reclasifica el pasado; fechas mal formadas → 400; borrar una empresa con mermas cae por cascade (E17), en `server/internal/integration/waste_reasons_test.go`
- [X] T034 [US3] Servicio y rutas de editar, archivar y juntar motivos, `GET /waste` y `GET /waste/by-reason` en `server/internal/app/waste_reports.go`, `waste.go` y `handlers_waste.go`, hasta que T033 pase
- [X] T035 [P] [US3] Tests de vitest de la pestaña (Maquetas: «Merma por motivo», opción B): pestaña «Se echó a perder» en Almacén con `Tabs.List` que se desplaza; fichas en una fila con la activa a la vista; tocar filtra; rango con `DatePickerButton`, en `web/src/features/backoffice/WasteTab.test.tsx`
- [X] T036 [US3] Pestaña `web/src/features/backoffice/WasteTab.tsx` (no en `features/waste/`: las fronteras de eslint) en `web/src/features/backoffice/StockPage.tsx`, hasta que T035 pase
- [X] T037 [P] [US3] Tests de vitest de motivos y tope (Maquetas: «Motivos y tope», opción A): renglón del tope, Editar (nombre, descripción, archivar, «No cuenta como pérdida del negocio») y Juntar apartado y con confirmación, en `web/src/features/admin/WasteSettings.test.tsx`
- [X] T038 [US3] Motivos y tope en `web/src/features/admin/WasteSettings.tsx`, dentro de `InventorySettingsSection.tsx`, hasta que T037 pase

## Phase 6: Polish

- [X] T044 [P] Test de nombres: ni `0094_waste.sql`, ni `waste.sql`, ni las pantallas de `web/src/features/waste/` traen nombres de un negocio, como el del 038 (FR-022), en `server/internal/app/waste_names_test.go`
- [X] T045 [P] Test de palabras de pantalla: los textos de `web/src/features/waste/` y del faltante dicen «Se echó a perder» y nunca «merma», «movimiento», «ajuste» ni «kardex» (FR-023), agregando esos archivos a `web/src/features/backoffice/operatorWords.test.ts`

- [X] T039 [P] Bloque de la 039 en `AGENTS.md` (lo que no se ve: el periodo por id de movimiento, juntar no reescribe, la sucursal explícita del movimiento) y filas en `docs/matriz-de-pantallas.md`
- [ ] T040 Medir con Playwright a 1024×600 contra el ambiente de pruebas la captura «Una pantalla» con empaques y con 8 o más motivos, la franja y la pestaña, con capturas; y una prueba con usuario sin contexto (SC-005)
- [ ] T041 Correr el `quickstart.md` contra la API local
- [ ] T042 `make ci-local`
- [ ] T043 `/revision-de-codigo` (hook `after_implement`)

## Dependencies

Phase 1 → Phase 2 → US1 → US2 → US3. US2 depende de que existan mermas (US1) y de las columnas del conteo (T006). US3 solo de Phase 2 y del servicio de US1. Dentro de cada historia: tests en rojo → servicio → rutas → web.

Archivos compartidos que obligan orden: `router.go` y `handlers_waste.go` (T019 → T034), `AppShell.tsx` (T023 → T027), `waste.go` (T018 → T034), `stock_counts.go` (T030), `backoffice.ts` (T025 → T032 → T036).

## Parallel

- Phase 2: T002, T003, T004 y T005 a la vez.
- US1: T012–T017, T022, T024 y T026 a la vez (archivos distintos).
- US2: T028, T029 y T031 a la vez.
- US3: T033, T035 y T037 a la vez.

## Implementation Strategy

MVP = Phase 1–2 + US1 (registrar, aprobar, deshacer; la merma ya resta). Después US2 (lo que el dueño quiere saber tras un conteo) y US3 (reporte y motivos), cada una entregable sola.
