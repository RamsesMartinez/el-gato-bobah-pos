---
description: "Tareas de la 056: unir insumos, marca en la presentación, posibles duplicados"
---

# Tasks: Un insumo con varias marcas (entrega 1)

**Input**: [spec.md](spec.md), [plan.md](plan.md), [research.md](research.md), [data-model.md](data-model.md), [contracts/api.md](contracts/api.md), [quickstart.md](quickstart.md)

**Tests**: obligatorios (constitución IV, TDD). En cada historia, la tarea de prueba va antes que la de
código y se ve fallar por la razón correcta antes de implementar.

**Restricción**: no se tocan `server/internal/app/purchases.go` ni `server/internal/app/purchaseqty.go`
(los cambia la 055 en paralelo). Antes de la Fase 2 se trae `develop` en local (commit firmado).

## Format: `[ID] [P?] [Story] Description`

- **[P]**: se puede hacer en paralelo (otro archivo, sin dependencias pendientes)
- **[Story]**: historia del spec (US1…US6)

---

## Phase 1: Setup

- [X] T001 Traer `origin/develop` a la rama `056-unir-insumos` (merge firmado) y resolver; si la 055 ya llegó, revisar que nada de este plan choque con sus cambios en `server/internal/app/purchases.go` y `purchaseqty.go`; `cd server && go build ./...` en verde
- [X] T002 Verificar que la base local restaurada (`make db-restaurar`, respaldo del 2026-11-01) y la de pruebas (`make test-integration` con `TEST_RESTORED_DATABASE_URL` de `make respaldo-anonimo`) existen para las pruebas de migración contra respaldo real con dos empresas

---

## Phase 2: Foundational (bloquea todas las historias)

**Migración 0104 y su prueba (pasa por el db-architect antes de aplicarse)**

- [X] T003 Escribir la prueba de la migración en `server/internal/integration/migration_0104_test.go` contra respaldo real con dos empresas: columnas `kind` (default `unit_change` en los reemplazos existentes), `brand`, `presentation_brand` con el relleno; índice de presentaciones con nombre+marca; tabla `ingredient_distinct_pairs` con RLS; triggers presentes; Down que se niega con una unión, una marca o un par, y que con la limpieza baja y vuelve a subir
- [X] T004 Agregar el renglón `104` a `guardedDownCleanup` en `server/internal/integration/harness_restaurado_test.go` en el orden de [data-model.md](data-model.md) «Down de la 0104»
- [X] T005 Escribir `server/migrations/0104_ingredient_merge.sql` según [data-model.md](data-model.md): `kind` + `check_ingredient_merge`; `brand` + índice nuevo (crear antes de borrar el viejo, `lock_timeout`); `expense_items.presentation_brand` + `snapshot_presentation_brand`; `ingredient_distinct_pairs` (RLS `nullif`, grants `select, insert`, índice por el lado alto); `redirect_merged_ingredient_movement` (before insert, `exists` primero, CTE con tope 16, piso ±0.0001); `reject_movement_on_merged_ingredient` (constraint diferido); `reject_merged_ingredient` en las 8 tablas, inmediato y diferido; `reject_reactivating_merged_ingredient`; solo las de trigger `security definer` con `search_path` fijo, `revoke execute from public`; Down con guarda
- [X] T006 Pasar la 0104 por el subagente `db-architect` y corregir lo que encuentre ANTES de aplicarla en la base local
- [X] T007 Clasificar `ingredient_replacements.kind`, `ingredient_presentations.brand` y `expense_items.presentation_brand` en `noPersonalData` («catálogo») en `server/internal/integration/respaldo_anonimo_test.go`
- [X] T008 Prueba de aislamiento de `ingredient_distinct_pairs` bajo `appRoleStore` con `inTheThreeCases` en `server/internal/integration/ingredient_merge_rls_test.go` (además de que `TestEveryCompanyTableIsIsolated` la recorra)
- [X] T009 `make sqlc` con la 0104 aplicada (sqlc lee el esquema de las migraciones) y `cd server && go build ./...` en verde

**Errores y traducción de la base**

- [X] T010 [P] Prueba en `server/internal/store/errors_test.go`: `EGB02` → `domain.ErrIngredientMerged`, también envuelto; prueba de integración de que un `EGB02` nacido en el commit (trigger diferido) sale como 409 y no 500
- [X] T011 [P] Centinelas en `server/internal/domain/ingredients.go`: `ErrIngredientMerged`, `ErrMergeBlocked` (con mensaje de qué falta), `ErrMergeStale`, `ErrMergePresentationConflict`; mapeo en `server/internal/httpapi/respond.go` a 409 `INGREDIENT_MERGED`, `MERGE_BLOCKED`, `MERGE_STALE`, `MERGE_PRESENTATION_CONFLICT`
- [X] T012 `EGB02` en `DomainError` (`server/internal/store/errors.go`) y `DomainError(tx.Commit(ctx))` en los tres commits de `server/internal/store/store.go`

**Clasificación de tablas**

- [X] T013 Prueba: `TestEveryTableThatPointsToAnArticleIsClassified` revisa también `app.MergeTables`, y `TestEveryRejectingTableHasTheMergeGuard` revisa en `pg_trigger` que cada tabla clasificada «rechaza» tiene la guarda inmediata y la diferida, en `server/internal/integration/unit_change_test.go`
- [X] T014 `app.MergeTables` en `server/internal/app/ingredient_merge.go` y `ingredient_distinct_pairs` en `app.UnitChangeTables` (`server/internal/app/unit_change.go`)

**Bitácora**

- [X] T015 [P] Acciones `ingredient.merged` y `ingredient.merged_into` en `server/internal/domain/audit.go` (grupo de precios) y sus etiquetas en `web/src/shared/audit/labels.ts`; `TestEveryAuditActionIsProduced` las exige (se cumple en US1)

**Vista previa compartida (front)**

- [X] T016 [P] Extraer de `web/src/features/backoffice/EditPreviewSheet.tsx` las piezas genéricas a `web/src/shared/preview/` (`PreviewGroup`, renglón antes/después/cambio, `signed`, `tone`, aviso «Algo cambió mientras revisabas»); `EditPreviewSheet` las usa y pinta idéntico: `EditPreviewSheet.test.tsx` y `ExpenseDetailDialog.test.tsx` en verde sin cambiarlos

**Checkpoint**: 0104 aplicada en local y en pruebas, `go build` y `go test ./internal/store ./internal/domain` verdes.

---

## Phase 3: User Story 1 - Unir dos marcas del mismo insumo (P1) 🎯 MVP

**Goal**: unir uno o varios orígenes medidos igual en un destino, con vista previa y confirmación; recetas y existencia pasan; los orígenes se archivan.

**Independent Test**: A (500 g) y B (300 g), receta con 100 g de B → vista previa «A 500 → 800», confirmar, vender: A queda en 700 y B no se mueve.

### Tests (primero, en rojo)

- [X] T017 [P] [US1] Unitarios table-driven en `server/internal/domain/ingredient_merge_test.go`: `ValidMergeRequest` (sin orígenes, más de 20, repetidos, destino entre orígenes, factor ≤ 0 o que no cabe, factor ≠ 1 con la misma unidad); `PlanMergedRecipe` (cambia de insumo conservando unidad; suma en la base del destino si la receta ya lo lleva); `MergedCost` (última compra entre todos, sin compras conserva el del destino, destino en cero toma el de un origen, convertido por factor)
- [X] T018 [US1] Integración en `server/internal/integration/ingredient_merge_test.go` — `TestMergeMovesEverythingAndLeavesNoRecipeOnTheSource`: existencia por sucursal (dos sucursales), recetas de producto activo, inactivo, extra y preparado, receta que ya llevaba los dos, costo, mínimo, origen archivado, ninguna `recipe_items` en el origen, eventos de bitácora en destino y orígenes; origen en negativo se traspasa tal cual y la vista previa marca `negativeAfter` si el destino queda en negativo; venta posterior descuenta del destino; conteo nuevo incluye la suma y no el origen; tres orígenes a la vez
- [X] T019 [US1] Integración de bloqueos y concurrencia en el mismo archivo: conteo abierto, compra por recibir, merma pendiente, preparado, origen archivado o ya unido → 409 `MERGE_BLOCKED` con mensaje; `TestASaleRacingTheMergeNeverLosesStock` (venta con el insumo tomado → unión 409; unión primero y venta después → la venta truena 409 al confirmar y el reintento descuenta del destino; la suma de movimientos cuadra con `stock_levels`); dos uniones a la vez se serializan; `TestALateMovementOnAMergedSourceGoesToTheTarget` (cancelar un pedido vendido antes de unir repone en el destino); un movimiento histórico del origen no se redirige al restaurar
- [X] T020 [US1] Integración de la huella: vista previa → venta → confirmar con la huella vieja = 409 `MERGE_STALE`; la vista previa no deja nada escrito (ni ids consumidos que cambien la huella)
- [X] T021 [US1] Integración HTTP y permisos: `POST /stock/articles/{id}/merge` con cajero → 403; `preview` distinto de `1` → 400; id de otra empresa → 404; los tres casos de RLS (`inTheThreeCases`) para la lectura de orígenes de un destino
- [X] T022 [US1] Integración: reactivar un origen unido → 409 (servicio y trigger); cambiar la unidad de un destino con orígenes → 409 `MERGE_BLOCKED`

### Implementation

- [X] T023 [P] [US1] `server/internal/domain/ingredient_merge.go`: `ValidMergeRequest`, `PlanMergedRecipe`, `MergedCost`, `MergeMinStock` (hasta que T017 pase)
- [X] T024 [US1] Consultas en `server/queries/ingredient_merge.sql`: candado ordenado de insumos (`for update nowait` por id), recetas del origen con su dueño, borrar renglón de receta, sumar a renglón existente, insertar unión (`kind = 'merge'`), última compra con costo por insumo, fijar costo resultante, orígenes de un destino con CTE recursivo; `make sqlc`
- [X] T025 [US1] `MergeIngredients(ctx, targetID, req, preview, fingerprint)` en `server/internal/app/ingredient_merge.go`: `LockCompositionEdits` → insumos ordenados `nowait` → existencias `nowait` → validar y bloqueos → recetas → traspaso por sucursal (`InsertReplacementMove` con la unión) → presentaciones/formatos/proveedor (US2) → costo y mínimo → archivar → `recomputeCosts` → bitácora → huella; con `preview` revierte con `errPreviewRollback`
- [X] T026 [US1] `planUnitChange` se niega con orígenes unidos y reactivar se niega para un origen unido en `server/internal/app/unit_change.go` y el servicio de archivar/reactivar
- [X] T027 [US1] Handler `POST /stock/articles/{id}/merge` (`?preview=1`) en `server/internal/httpapi/handlers_ingredients_admin.go` y ruta con `RequirePermission(inventory.configure)` en `server/internal/httpapi/router.go`; `GetArticle` devuelve `mergedInto` y `mergedSources`
- [X] T028 [US1] `ArticleCostHistory` y `ArticleAudit` leen también los orígenes (CTE), con `fromName` y el costo dividido entre el factor, en `server/queries/ingredients_admin.sql` y `server/internal/app/ingredients_admin.go`; prueba en `ingredient_merge_test.go` (FR-006)
- [X] T029 [P] [US1] Cliente en `web/src/api/backoffice.ts`: `previewMerge`, `merge`, tipos `MergePreview`, `mergedInto`, `fromName`
- [X] T030 [US1] Pruebas primero en `web/src/features/backoffice/articles/MergeSheet.test.tsx` — fila «Unir insumos» de Maquetas (A): «Unir otros aquí» solo con permiso y en activos no preparados; parecidos máx. 3 con «Ver todos»; renglón entero de 52 px como área de toque; «Siguiente» deshabilitado sin elegidos; «Volver» desde la vista previa conserva lo elegido; 409 `BUSY` dice «vuelve a intentar» sin perder nada; `MERGE_STALE` pide la vista previa de nuevo
- [X] T031 [US1] `web/src/features/backoffice/articles/MergeSheet.tsx` (pasos 1 y 3 de la maqueta A, vista previa con `shared/preview/`, pie fijo «Volver»/«Unir») y el botón en `ArticleSheet.tsx`; un destino que queda en negativo se marca «⚠ quedará en negativo» como en la 053
- [X] T032 [US1] Ficha de un origen unido: franja «Unido a <destino> el <fecha> · Abrir» en lugar de «Reactivar», en `web/src/features/backoffice/articles/ArticleSheet.tsx`, con prueba en `ArticleSheets.test.tsx` (fila «Unir insumos» de Maquetas)
- [X] T033 [US1] Historial de costos muestra «de <origen>» en los renglones que vienen de un origen, en `web/src/features/backoffice/articles/ArticleSheet.tsx`/`CostTrend.tsx`, con prueba

**Checkpoint**: US1 completa y probable sola (Q1–Q4, Q6, Q8, Q11 del quickstart).

---

## Phase 4: User Story 2 - Lo que se compraba de la otra marca sigue entrando (P1)

**Goal**: presentaciones, formatos de compra y artículos de proveedor pasan al destino; una compra nueva con ellos entra al destino.

**Independent Test**: presentación «Caja 12 × 1 L» de B, unir B en A, comprar con ella → A sube 12,000 ml; ticket de proveedor ligado a B sale ligado a A.

- [X] T034 [P] [US2] Unitarios de `PlanMergedPresentations` en `server/internal/domain/ingredient_merge_test.go`: sin marca toma el nombre del origen (recortado a 40); choque nombre+marca+contenido → archiva la del origen; choque con contenido distinto → conflicto; aproximado que deja de cuadrar → se queda archivado en el origen; inactivas se quedan
- [X] T035 [US2] Integración en `ingredient_merge_test.go`: la presentación se mueve conservando su id (las compras viejas la siguen nombrando); compra nueva con ella sube el destino; `supplier_items` y `ingredient_purchase_formats` ligados al destino; crear una receta, renglón de compra, presentación, artículo de proveedor, formato, merma, renglón de conteo o corrección de costo con el origen → 409 `INGREDIENT_MERGED` (inmediato y en la carrera diferida); choque con contenido distinto → 409 `MERGE_PRESENTATION_CONFLICT`; la lectura de un ticket con un renglón aprendido del origen propone el destino (escenario 3 de US2)
- [X] T036 [US2] `PlanMergedPresentations` en `server/internal/domain/ingredient_merge.go` y su aplicación en `MergeIngredients` (mover presentación, formatos con `SetPurchaseFormatQty`, proveedor con `SetSupplierItem`), consultas nuevas en `server/queries/ingredient_merge.sql`
- [X] T037 [US2] La vista previa lista presentaciones (`move|archive|conflict`), artículos de proveedor y formatos; el front pinta el grupo «Presentaciones» y bloquea «Unir» con un conflicto, en `MergeSheet.tsx` con su prueba

---

## Phase 5: User Story 3 - Unir insumos que se miden distinto (P2)

**Goal**: con unidad base distinta se exige la equivalencia y todo se convierte.

**Independent Test**: B en g (400 g) en A en piezas (10): sin equivalencia se niega; con «1 g = 0.25 piezas» → A 110 piezas y la receta de 4 g pasa a 1 pieza.

- [X] T038 [P] [US3] Unitarios en `ingredient_merge_test.go` (domain): conversión de receta, existencia, presentación y costo con factor; cantidad que queda en cero → error; factor que no cabe → error
- [X] T039 [US3] Integración: unión con factor convierte existencias, recetas, presentaciones, formatos y contenido aprendido del proveedor; el movimiento tardío redirigido se convierte (y uno diminuto no truena por el `check` de cero); el historial de costos del destino muestra el del origen convertido
- [X] T040 [US3] Aplicar el factor en `MergeIngredients` (`server/internal/app/ingredient_merge.go`)
- [X] T041 [US3] Prueba primero en `MergeSheet.test.tsx` y luego el paso 2 «¿Cuánto equivale?» en `MergeSheet.tsx` (un renglón por elegido de otra medida, «1 pieza = ___ ml», teclado numérico de la app; sin todos los valores no avanza) — fila «Unir insumos» de Maquetas

---

## Phase 6: User Story 4 - Marca en la presentación (P2)

**Goal**: marca opcional en cada presentación, visible donde se lista y donde se elige al comprar.

**Independent Test**: dos «Caja» con marcas distintas se guardan; el selector de compra muestra «Caja · Marca A» y «Caja · Marca B».

- [X] T042 [P] [US4] Integración en `server/internal/integration/presentations_brand_test.go`: crear/editar con marca; vacío o espacios = sin marca; > 40 → 400; mismo nombre y distinta marca conviven; misma marca (o ambas sin) → 409; `expense_items.presentation_brand` copia la marca al comprar (también con un insert sin la columna, como la API vieja) y editar la marca después no cambia la de compras pasadas
- [X] T043 [US4] `brand` en `ListPresentations`, `CreatePresentation`, `UpdatePresentation` (`server/queries/ingredients.sql`), `PresentationView`/`PresentationInput` y bitácora en `server/internal/app/presentations.go`, handler en `server/internal/httpapi`
- [X] T044 [P] [US4] Prueba y función `presentationLabel` en `web/src/shared/presentationLabel.ts` (+ `.test.ts`)
- [X] T045 [US4] Campo «Marca (opcional)» y etiqueta en `web/src/features/backoffice/PresentationsSheet.tsx` con prueba en `PresentationsSheet.test.tsx` (fila «Unir insumos» de Maquetas, pantalla «Presentaciones» del plan)
- [X] T046 [US4] `presentationLabel` en `ExpenseDialog.tsx`, `AddLineSheet.tsx`, `ArrivedItemSheet.tsx`, `StockCountTab.tsx` (y `key` por id en lugar de `p.name`), `web/src/features/waste/WastePage.tsx`; prueba en `StockCountTab.test.tsx` con dos presentaciones del mismo nombre y distinta marca

---

## Phase 7: User Story 6 - Posibles duplicados agrupados (P2)

**Goal**: grupos de nombres parecidos con «Revisar unión» y «No son lo mismo».

**Independent Test**: «Leche entera · Leche entera Marca B» aparece; elegir cuál se queda lleva a la vista previa; «No son lo mismo» en un par lo quita para siempre.

- [X] T047 [P] [US6] Unitarios de `SimilarNames` y `DuplicateGroups` en `server/internal/domain/similar_names_test.go`: contiene como palabras completas (más corto ≥ 4 letras), una letra cambiada, dos invertidas, acentos/mayúsculas/signos, «Azúcar» contra «Leche entera» no; transitividad A~B~C en un grupo; un par descartado parte el grupo
- [X] T048 [US6] `SimilarNames`, `DuplicateGroups` en `server/internal/domain/similar_names.go` (normalización de `NormalizeReasonName` más signos)
- [X] T049 [US6] Integración en `server/internal/integration/duplicates_test.go`: `GET /stock/articles/duplicate-groups` (solo activos no preparados, sin pares descartados, `[]` y no `null` en el JSON crudo); `POST /stock/articles/distinct-pairs` idempotente, ordena el par, 400 con `a == b`, 404 con id de otra empresa; cajero → 403
- [X] T050 [US6] Consultas (`server/queries/ingredient_merge.sql`), servicio y handlers de los dos endpoints, rutas con `inventory.configure`
- [X] T051 [US6] Pruebas primero en `web/src/features/backoffice/articles/DuplicatesSheet.test.tsx` — fila «Unir insumos» de Maquetas (C): botón con badge en la segunda fila de `ArticlesTab.tsx`, ausente con N = 0 o sin permiso; tocar un grupo lo expande en su lugar; «Se queda» por radio de 52 px; «No son lo mismo» por par con «Deshacer» 5 s (no se manda si se deshace; se manda al cerrar la hoja); «Revisar unión» abre la hoja de unir montada cerrada y abierta en el render siguiente, con los demás del grupo marcados y desmarcables antes de confirmar (FR-022)
- [X] T052 [US6] `DuplicatesSheet.tsx` y el botón en `web/src/features/backoffice/articles/ArticlesTab.tsx`; `MergeSheet` acepta orígenes ya elegidos

---

## Phase 8: User Story 5 - Aviso de posible duplicado al crear (P3)

**Goal**: al guardar un insumo nuevo con parecidos, «¿Es alguno de estos?».

**Independent Test**: guardar «leche entera lala» pregunta; «Azúcar» crea directo; «Crear de todos modos» crea.

- [X] T053 [US5] Integración: `GET /stock/articles/similar?name=` (1–80, vacío o largo → 400; hasta 5; `[]` no `null`; solo activos de la empresa) en `server/internal/integration/duplicates_test.go`; medir la respuesta con los ~450 insumos de la base restaurada (meta < 300 ms)
- [X] T054 [US5] Servicio y handler de `similar` (reusa `SimilarNames`) y ruta con `inventory.configure`
- [X] T055 [US5] Pruebas primero en `web/src/features/backoffice/articles/NewArticleSheet.test.tsx` — fila «Aviso de insumo parecido» de Maquetas (B): sin parecidos crea directo; con parecidos muestra la lista (máx. 5, scroll interno), «Abrir» abre la ficha sin crear, «Volver» conserva lo escrito, «Crear de todos modos» crea; nombre idéntico sigue respondiendo 409 como hoy
- [X] T056 [US5] Implementación en `web/src/features/backoffice/articles/NewArticleSheet.tsx`

---

## Phase 9: Polish & Cross-Cutting

- [X] T057 Medición de uso: acciones `unir-insumos` y `no-son-lo-mismo` en `web/src/api/uso.ts`/`medirAccion` y la lista blanca de `server/internal/domain/uso.go` si son acciones nuevas
- [ ] T058 [P] E2E `web/e2e/unir-insumos.spec.ts` a 1024×600 contra el `make start` del worktree: unir dos insumos creados por la prueba (no los reales), vender, ver la existencia; los crea y los deja archivados; capturas solo en `test-results/`
- [ ] T059 Documentar en `AGENTS.md` §2 «Unir insumos (spec 056)»: las guardas de la base (redirige movimientos, rechaza lo demás, diferido para la carrera), que un camino nuevo que escriba en una tabla que apunta a `ingredients` se clasifica en `MergeTables`, la marca copiada en la compra y que un destino con unidos no cambia de unidad
- [ ] T060 `/revision-de-codigo` (hook `after_implement`) y corregir lo que encuentre con su prueba
- [ ] T061 Prueba con usuario nuevo (subagente sin contexto) a 1024×600 con datos reales restaurados: escenarios Q1–Q12 de [quickstart.md](quickstart.md); lo observado en texto
- [ ] T062 `make ci-local` verde; `cd server && go build ./... && go test ./...`; `cd web && bun run lint && bun run vitest run && bun run build`

---

## Dependencies & Execution Order

- Setup (T001–T002) → Foundational (T003–T016) → historias.
- US1 primero (MVP). US2 se apoya en `MergeIngredients` de US1. US3 extiende US1/US2 con el factor.
  US4 (marca) es independiente de US1 salvo que US2 usa `brand` al mover presentaciones: T043 antes
  de T036. US6 depende de US1 (abre la misma hoja). US5 depende de `SimilarNames` (T048).
- Dentro de cada historia: prueba en rojo → dominio → consultas/servicio → handler → front (prueba → código).

## Parallel Opportunities

- Fase 2: T010, T011, T015, T016 en paralelo; T003–T006 en serie.
- US1: T017 y T029 en paralelo con T018–T022.
- US4 (T042–T046) puede avanzar en paralelo a US1 en el front (T044–T046).
- US6/US5: T047/T048 en paralelo con la fase de US2.

## Implementation Strategy

MVP = Fases 1–3 (unir con la misma unidad, vista previa, guardas). Luego US2 y US4 (lo que hace que el
problema no regrese), US3, US6 y US5. Todo entra en la misma entrega del #185; la regla de costo por
empresa y comparar marcas van al issue nuevo.
