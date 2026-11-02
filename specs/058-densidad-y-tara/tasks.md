---
description: "Tareas de la 058: densidad, compras masa↔volumen, cambio de unidad, tara y conteo por peso"
---

# Tasks: Densidad y tara de los artículos del almacén

**Input**: [spec.md](spec.md), [plan.md](plan.md) (la sección «Revisión de arquitectura» manda sobre
research y data-model donde choquen), [research.md](research.md), [data-model.md](data-model.md),
[contracts/api.md](contracts/api.md), [quickstart.md](quickstart.md)

**Tests**: obligatorios (constitución IV, TDD). En cada historia la prueba va antes que el código y se
ve fallar por la razón correcta.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: se puede hacer en paralelo (otro archivo, sin dependencias pendientes)
- **[Story]**: historia del spec (US1…US4)

---

## Phase 1: Setup

- [X] T001 Confirmar que la rama `058-densidad-y-tara` está al día con `origin/develop` y que ninguna rama ni worktree trae una migración 0105; `cd server && go build ./...` en verde
- [X] T002 Verificar que existen la base local restaurada (`make db-restaurar`) y la restaurada de pruebas (`TEST_RESTORED_DATABASE_URL`) para las pruebas de migración con dos empresas

---

## Phase 2: Foundational (bloquea todas las historias)

**Migración 0105 (pasa por el db-architect antes de aplicarse)**

- [X] T003 Prueba de la migración en `server/internal/integration/migration_0105_test.go` contra respaldo real con dos empresas: columnas nuevas de `ingredients`, `ingredient_presentations`, `expense_items`, `stock_count_lines` con sus checks (rango 0.3–3, todo-o-nada de densidad, tara ≤ 100000, bruto junto); la FK compuesta de `density_set_by`; el trigger que limpia densidad y taras al pasar un artículo a pieza/cm con un `update ingredients set base_unit_id` a pelo; el rechazo de `tare_g` en un empaque de un artículo pieza; Down que se niega con cada dato nuevo y que con la limpieza baja y vuelve a subir
- [X] T004 Renglón `105` en `guardedDownCleanup` de `server/internal/integration/harness_restaurado_test.go` (pone en nulo las columnas nuevas)
- [X] T005 Escribir `server/migrations/0105_density_and_tare.sql` según [data-model.md](data-model.md) y la revisión M4/M5/A2 del plan: columnas, checks, FK dentro de `do $$ execute $$`, trigger `before update of base_unit_id` en `ingredients`, extender `ingredient_presentations_before_write`, Down con guarda
- [X] T006 Pasar la 0105 por el subagente `db-architect` y corregir lo que encuentre ANTES de aplicarla en local
- [X] T007 Clasificar `ingredients.density_method` en `noPersonalData` en `server/internal/integration/respaldo_anonimo_test.go`
- [X] T008 `make sqlc` con las consultas nuevas o cambiadas: `SetArticleDensity`, `ClearArticleDensity`, `GetArticle` y `SearchArticles` con densidad (`server/queries/ingredients_admin.sql`, `ingredients.sql`); `ListPresentations`/`CreatePresentation`/`SetPresentationRaw` con `tare_g` y `SetPresentationTare`; `ListStockLines` con `coalesce(ei.density_g_per_ml, i.density_g_per_ml)` y `fixed_density`, `SetExpenseItemDensity`, `UpdateStockLine` que limpia la densidad fija (`server/queries/purchases.sql`); `PendingDensityLinesOf`; consultas del conteo con densidad, tara y las cuatro columnas de pesada (`server/queries/stock_counts.sql`); `cd server && go build ./...`

**Dominio común**

- [X] T009 [P] Pruebas table-driven en `server/internal/domain/density_test.go`: `ValidDensity` (0.2999, 0.3, 3, 3.0001, NaN, 5 decimales), `DensityFromWeighing` (vacío = lleno, vacío > lleno, contenido 0, resultado fuera de rango con el valor en el mensaje, 1250/150/1000 → 1.1), `DensityFromLabel` (4300/3785 → 1.1361), `DensityFactor` (g→ml 1/1.3 a 6 decimales, ml→g 0.92, pieza o longitud → error), `NetFromGross` (g: 1000−3×50 = 850; ml: (850−240)/1.1 = 554.5455; neto 0 válido; bruto < tara → `ErrGrossBelowTare`; ml sin densidad; empaques 0 y 1000; tara > tope)
- [X] T010 [P] Implementar `server/internal/domain/density.go` hasta poner T009 en verde; centinelas `ErrDensityChanged`, `ErrDensityInUse`, `ErrGrossBelowTare`, `ErrCountInputsChanged` y el tipo `UnitKindMismatch{IngredientID, Name}` (envuelve `ErrUnitKindMismatch`); mapeo en `server/internal/httpapi/respond.go` a 409 `DENSITY_CHANGED`, `DENSITY_IN_USE`, `COUNT_INPUTS_CHANGED`, 400 y `UNIT_KIND_MISMATCH` con `details`
- [X] T011 [P] Acciones `ingredient.density_set`, `ingredient.density_cleared` y `presentation.tare_set` en `server/internal/domain/audit.go` y sus etiquetas en `web/src/shared/audit/labels.ts` (sin `weighing`/`label`/`manual` en pantalla: «por pesada», «de la etiqueta», «escrita»)

**Checkpoint**: 0105 aplicada en local y en pruebas; `go test ./internal/domain` verde.

---

## Phase 3: User Story 1 - Capturar la densidad (P1) 🎯 MVP

**Independent Test**: artículo en ml sin densidad; pesada 1,250/150/1,000 → la ficha dice 1.10 gramos por ml, por pesada, con quién y cuándo; la bitácora tiene la pesada.

- [X] T012 [US1] Pruebas de integración (incluye FR-021) en `server/internal/integration/density_test.go` bajo `appRoleStore`: guardar por pesada, por etiqueta y escrita; rehacer (evento con antes y después); quitar (204, idempotente sin evento); artículo pieza/cm → 400; archivado → 409; otra empresa → 404 (`inTheThreeCases` para `GetArticle` con densidad); quitar con una compra pendiente masa↔volumen sin densidad fija → 409 `DENSITY_IN_USE`; cuerpo con método desconocido o campo de más → 400
- [X] T013 [US1] `SetDensity`/`ClearDensity` en `server/internal/app/density.go`: `lockIngredients` (nowait), solo g/ml y activo, `PendingDensityLinesOf` antes de quitar, evento en la misma transacción
- [X] T014 [US1] Handlers `PUT`/`DELETE /stock/articles/{id}/density` (decodificación estricta) en `server/internal/httpapi/handlers_ingredients_admin.go` y rutas con `RequirePermission(inventory.configure)` en `router.go`; `ArticleSheet` con `density` (nombre de quien la puso)
- [X] T015 [P] [US1] Prueba vitest `web/src/features/backoffice/articles/DensitySheet.test.tsx`: resultado mostrado antes de guardar en los tres modos, «Guardar» deshabilitado con vacío ≥ lleno o contenido 0, vacío ≠ cero (borrar el campo no manda 0), mensaje de fuera de rango, confirmación en línea de «Quitar»
- [X] T016 [US1] `web/src/features/backoffice/articles/DensitySheet.tsx` (maqueta Densidad A, fila «Maquetas» del plan): «Opcional», (i) con el ejemplo del galón, «Con una pesada» / «Escribir el valor» (g/ml o «Con la etiqueta»), `NumericKeypad` en el pie solo con campo activo, monta cerrada y abre en el render siguiente; API en `web/src/api/backoffice.ts`
- [X] T017 [US1] Renglón «Densidad · Opcional» pegado a «Se mide en» en `web/src/features/backoffice/articles/ArticleSheet.tsx`, solo en g/ml, con (i) que despliega debajo; prueba en `ArticleSheets.test.tsx`

---

## Phase 4: User Story 2 - Compras masa↔volumen (P1)

**Independent Test**: ml con densidad 1.10; 4.3 kg recibidos → +3,909.0909 ml; rehacer a 1.08 y corregir lo recibido a 4 kg → baja exactamente 272.7273 ml.

- [X] T018 [P] [US2] Pruebas en `server/internal/domain/purchaseqty_test.go` y `purchases_test.go`: `BaseQty` masa→volumen y volumen→masa con densidad, sin densidad → `ErrUnitKindMismatch`, longitud↔masa sigue rechazada, presentación y pieza no usan densidad; `PlanStockLines` con densidad fija distinta de la vigente usa la fija
- [X] T019 [US2] `PurchaseUnit.Density` y la regla en `server/internal/domain/purchaseqty.go` hasta verde
- [X] T020 [US2] Pruebas de integración en `server/internal/integration/density_purchases_test.go`: capturar y recibir 4.3 kg (cantidad y costo por ml); rehacer la pesada y luego corregir lo recibido, editar el gasto (053) y anular: todo con la densidad fija; renglón no recibido usa la vigente al recibir y desde ahí queda fija; `UpdateStockLine` que cambia de artículo limpia la densidad fija; vista previa de edición con la densidad en la huella (rehacer entre vista previa y confirmación → 409 `EXPENSE_EDIT_STALE`); sin densidad → 400 `UNIT_KIND_MISMATCH` con el artículo en `details`
- [X] T021 [US2] `stockStates` con `Density`; en `applyStockLinesOn` (`server/internal/app/purchases.go`) fijar `SetExpenseItemDensity` con el valor exacto usado al primer movimiento de un renglón masa↔volumen sin presentación; verificar que cada llamador toma `lockIngredients` antes de `ListStockLines`; huella de `server/internal/app/expense_edit.go` con la densidad efectiva; `ErrUnitKindMismatch` envuelto en `UnitKindMismatch` con el artículo
- [X] T022 [US2] Vistas de renglón con `densityGPerMl` (detalle, por recibir, vista previa de edición) y `SearchArticles` con `densityGPerMl`
- [X] T023 [P] [US2] Pruebas vitest en `web/src/features/backoffice/expenseDraft.test.ts`: `densityFactor` con los mismos casos que `BaseQty` y `conversionWords` («4,300 g ÷ 1.10 = 3,909 ml», «2,000 ml × 0.92 = 1,840 g»)
- [X] T024 [US2] `densityFactor`/`conversionWords` en `web/src/features/backoffice/expenseDraft.ts`; `factorOf` de `ExpenseDialog.tsx`, `AddLineSheet.tsx` y `ArrivedItemSheet.tsx` los usan; la conversión o el aviso «Falta la densidad para pasar kg a ml» solo en renglones masa↔volumen, en la misma fila; el atajo «Capturar la pesada» (44 px, uno por artículo) vive en el renglón de `ExpenseDialog` y abre `DensitySheet` (dos capas); dentro de `AddLineSheet` solo el aviso; al guardar la densidad se invalida `articles` y el renglón se reevalúa sin perder lo capturado; `PendingReceiptsTab.tsx`, `ExpenseDetailDialog.tsx` y `EditPreviewSheet.tsx` muestran la conversión; pruebas en sus `.test.tsx`

---

## Phase 5: User Story 3 - Cambio de unidad entre peso y líquido (P2)

**Independent Test**: g con 1,300 g, $0.10/g y densidad 1.30 → ml: 1,000 ml y $0.13/ml; la bitácora dice la densidad.

- [X] T025 [P] [US3] Pruebas en `server/internal/domain/ingredients_test.go`: `ValidUnitChange` masa↔volumen con densidad pasa, sin densidad falla; pieza sigue igual
- [X] T026 [US3] Pruebas de integración en `server/internal/integration/unit_change_test.go`: directo g→ml (existencia, recetas, empaques, costo, densidad y taras conservadas, densidad en la bitácora); reemplazo con compras (el nuevo trae densidad y taras); factor enviado distinto del derivado → 409 `DENSITY_CHANGED`; sin factor → lo deriva; sin densidad → 400; directo g→pieza limpia densidad y taras; rehacer la pesada después no cambia lo convertido
- [X] T027 [US3] `server/internal/domain/ingredients.go` y `server/internal/app/unit_change.go`: factor derivado con `DensityFactor`, `density` y `factor` en `UnitChangePreview`, la densidad copiada al artículo de reemplazo, `tare_g` en `SetPresentationRaw`/`CreatePresentation` del reemplazo, evento con `density`
- [X] T028 [US3] `web/src/features/backoffice/articles/UnitChangeSheet.tsx`: con densidad, g↔ml como destino con la equivalencia fija y la densidad dicha; sin densidad, destino deshabilitado con «Primero captura la densidad» que cierra la hoja y abre `DensitySheet` desde la ficha; no manda `factor` en masa↔volumen; 409 `DENSITY_CHANGED` recarga la vista previa; pruebas en `UnitChangeSheet.test.tsx`

---

## Phase 6: User Story 4 - Tara y conteo por peso con empaque (P2)

**Independent Test**: ml con densidad 1.10, «Botella 1 L» pesa vacía 120 g; 3 cerradas + 850 g con 2 empaques → 3,554.5455 ml, y la línea guarda lo pesado.

- [X] T029 [US4] Pruebas de integración de la tara en `server/internal/integration/density_test.go`: crear y editar empaque con `tareG`, `PUT` sin `tareG` no la toca, `null` la quita, > 100000 → 400, artículo pieza → 400, evento `presentation.tare_set`; unión de insumos (056): destino sin densidad recibe la coincidente de los orígenes, y una presentación del destino sin tara recibe la del duplicado
- [X] T030 [US4] `tareG` en `server/internal/app/presentations.go` (con `SetPresentationTare`), en sus handlers y en `server/internal/app/ingredient_merge.go`
- [X] T031 [US4] Pruebas de integración (FR-016–FR-020) en `server/internal/integration/stock_counts_test.go`: captura con `gross` en g y en ml (resultado, columnas de pesada guardadas, `grossNet` en la respuesta), sumado a `qty`; `gross` y `weight` juntos → 400; empaque sin tara, de otro artículo o inactivo → 400; bruto < tara → 400; `seenTareG`/`seenDensity` distintos de los vigentes → 409 `COUNT_INPUTS_CHANGED`; recapturar sin pesar limpia las columnas; máximo de la empresa y aviso de cifra rara sobre el total; `CountItem` con `densityGPerMl`, `grossWeighable` y `tareG` bajo `inTheThreeCases`
- [X] T032 [US4] `CountCapture.Gross` en `server/internal/app/stock_counts.go` con `NetFromGross`, columnas de pesada en `UpsertCountLine`, `grossWeighable`; handler en `server/internal/httpapi`
- [X] T033 [P] [US4] Pruebas vitest en `web/src/features/backoffice/countEntry.test.ts`: la cuenta en palabras con los números del `CountItem` y el total cerradas + pesado; que «Abierto o suelto» y lo pesado sean excluyentes
- [X] T034 [US4] `web/src/features/backoffice/PresentationsSheet.tsx`: «Pesa vacío (g) · Opcional» con (i), solo g/ml; en la lista «pesa vacío 120 g»; prueba en su `.test.tsx`
- [X] T035 [US4] `web/src/features/backoffice/StockCountTab.tsx` y `countEntry.ts` (maqueta Conteo A, fila «Maquetas» del plan): renglón plegado «Pesar con empaque · Opcional · (i)» solo con `grossWeighable`; abierto: báscula g/kg, `Picker` de empaque si hay varios con tara, empaques −/+ con número editable, la cuenta; oculta y limpia «Abierto o suelto»; `EntryFields` con scroll y «Total contado» fijo; en «Uno por uno» abre la hoja de captura completa; lo contado mostrado tras guardar es el de la respuesta; 409 `COUNT_INPUTS_CHANGED` recarga el artículo; pruebas en `StockCountTab.test.tsx`

---

## Phase 7: Polish & Cross-Cutting

- [X] T036 Medición de uso: acciones `capturar-densidad` y `pesar-con-empaque` en `medirAccion` y la lista blanca de `server/internal/domain/uso.go` si son nuevas
- [X] T037 [P] E2E `web/e2e/densidad-y-tara.spec.ts` a 1024×600 contra el `make start` del worktree: crea sus artículos, captura densidad, compra 4.3 kg, cambia de unidad otro, cuenta pesando con empaque; cierra lo que abre (conteo y gasto); capturas solo en `test-results/`; medir que la hoja de captura del conteo con la sección abierta y 1 y 4 empaques no saca «Guardar» de la pantalla
- [X] T038 Documentar en `AGENTS.md` §2 «Densidad y tara (spec 058)»: densidad fija por renglón y por qué, el trigger que limpia al cambiar a pieza/cm, quitar con compra pendiente → 409, lo pesado guardado en el conteo
- [X] T039 `/revision-de-codigo` (hook `after_implement`) y corregir lo que encuentre con su prueba
- [ ] T040 Prueba con usuario nuevo (subagente sin contexto) a 1024×600 con datos restaurados, recorrido de [quickstart.md](quickstart.md); lo observado en texto en el plan
- [ ] T041 `make ci-local` verde; `cd server && go build ./... && go test ./...`; `cd web && bun run lint && bun run vitest run && bun run build`

---

## Dependencies & Execution Order

- Setup → Foundational (T003–T011) → historias.
- US1 primero (MVP): las demás usan la densidad y `DensitySheet`.
- US2 depende de US1 (el atajo abre `DensitySheet`). US3 depende de US1. US4 depende de T010 y, para
  la unión, de T030.
- Dentro de cada historia: prueba en rojo → dominio → consultas/servicio → handler → front.

## Parallel Opportunities

- Fase 2: T009–T011 en paralelo tras T008.
- US2: T018 y T023 en paralelo. US3: T025 en paralelo con US2. US4: T033 en paralelo con T031.

## Implementation Strategy

MVP = Fases 1–3. Luego US2 (lo que más estorba al capturar compras), US3 y US4. Todo entra en la
misma entrega del #117.
