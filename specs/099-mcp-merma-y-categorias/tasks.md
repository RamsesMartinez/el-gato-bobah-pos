---
description: "Tasks de la 099 — merma esperada y categorías del menú desde el MCP"
---

# Tasks: Merma esperada y categorías del menú desde el MCP

**Input**: `specs/099-mcp-merma-y-categorias/` (plan, spec, research, data-model, contracts/api.md, contracts/mcp.md, quickstart)

**Tests**: obligatorios (constitución IV, TDD): cada test se escribe antes que su código y se ve fallar por la razón correcta.

**Organización**: por historia. Sin pantallas (plan, «Pantallas y maquetas»). Sin migración.

## Format: `[ID] [P?] [Story] Description`

## Phase 1: Setup

- [ ] T001 Confirmar base del worktree: `cd server && go build ./... && go test ./internal/domain/... ./internal/httpapi/...` en verde antes de tocar nada; `make test-integration` arranca contra el Postgres de pruebas (puerto 5699).

## Phase 2: Foundational (bloquea US2 y US3; US1 no depende)

- [ ] T002 [P] Test de sentinels y acciones nuevas: `category.created` y `category.updated` en la lista de `server/internal/domain/audit_test.go` y en su grupo de catálogo (junto a `category.family_set`); falla porque no existen.
- [ ] T003 [P] Tests table-driven de `CheckCategoryChange` en `server/internal/domain/category_rules_test.go`: nombre vacío/espacios/>60 (trim), padre = sí misma, padre no raíz, padre inactivo con categoría activa (alta, mover, reactivar), categoría con hijos que recibe padre, desactivar con N productos activos (mensaje con N), desactivar con hijos activos (los nombra), desactivar sin nada, volver raíz, sin cambios. Cada caso nombra el defecto que atrapa (research R5).
- [ ] T004 Implementar `server/internal/domain/category_rules.go` (`CategoryNode`, `CategoryFacts`, `CategoryNext`, `CheckCategoryChange`) y sentinels `ErrCategoryNameTaken`, `ErrCategoryHasActiveProducts`, `ErrCategoryHasActiveChildren` (envuelven `ErrConflict`) y `ErrCategoryParentInvalid` (validación) hasta que T003 pase.
- [ ] T005 Agregar `AuditCategoryCreated`/`AuditCategoryUpdated` en `server/internal/domain/audit.go` hasta que T002 pase.
- [ ] T006 Mapear los cuatro sentinels en `server/internal/httpapi/respond.go` a 409 `CATEGORY_NAME_TAKEN`/`CATEGORY_HAS_ACTIVE_PRODUCTS`/`CATEGORY_HAS_ACTIVE_CHILDREN` y 422 `CATEGORY_PARENT_INVALID`, con test en `server/internal/httpapi/respond_test.go` (o el archivo de mapeo existente) antes del cambio.
- [ ] T007 Consultas sqlc en `server/queries/admin.sql`: `LockCategoriesForChange` (`where id = any(@ids) order by id for update`, devuelve id, parent_id, is_active, name), `CategoryHasChildren`, `CategoryActiveChildrenNames`, `CategoryActiveProductCount` (`is_active and system_kind is null`), `FindCategorySibling` (`parent_id is not distinct from`, `id <> @self`, nombre citext), `InsertCategory`, `UpdateCategory` (name, parent_id, is_active), y `AdminCategoryList` con `@status` (`active`|`all`) que agrega `is_active` e `inactive_product_count`; `make sqlc` y `sqlc vet` limpios. Sin nombrar `company_id`.
- [ ] T008 `AdminService.CategoryList(ctx, includeInactive bool)` en `server/internal/app/admin.go` y `GET /admin/categories?status=all` en `server/internal/httpapi/handlers_admin.go` (otro valor → 400; sin parámetro igual que hoy). Test de handler para el 400 y test de integración bajo `appRoleStore` de que `all` trae inactivas con `isActive:false` y que la pantalla (sin parámetro) sigue sin ellas, más `inTheThreeCases` (otra empresa, conexión reciclada, sin empresa: ninguna categoría ajena), en `server/internal/integration/categories_099_test.go`.

## Phase 3: User Story 1 — Merma esperada del insumo (P1)

**Goal**: `ingredient_expected_waste_set` con vista previa (antes/después por producto) y motivo en bitácora.

**Independent Test**: quickstart §2 pasos 1–3.

- [ ] T009 [P] [US1] Tests de dominio en `server/internal/domain/expected_waste_impact_test.go`: `CostGraph.WithIngredientWaste` no altera el grafo original (cachés separadas); `ExpectedWasteImpact` incluye usos directos, por preparado (transitivo), extras, y producto con costo manual (antes = después, `Manual`); nil → hereda la del producto o la general; 0 ≠ nil; insumo sin usos → arreglo vacío no nil; mismo valor → sin cambios.
- [ ] T010 [US1] Implementar `CostGraph.WithIngredientWaste` en `server/internal/domain/costing.go` y `ExpectedWasteImpact`/`CostImpact` en `server/internal/domain/expected_waste_impact.go` hasta que T009 pase.
- [ ] T011 [US1] Test de integración en `server/internal/integration/expected_waste_099_test.go`: vista previa sobre un insumo con receta y preparado devuelve antes/después y nombres, no escribe nada (costos y pct iguales tras la llamada); `inTheThreeCases` (insumo de otra empresa, conexión reciclada, sin empresa → no encontrado); `SetArticleExpectedWaste` con motivo deja `reason` en el `after` de `ingredient.expected_waste`, sin motivo igual que hoy; mismo valor no deja renglón.
- [ ] T012 [US1] `CostingService.ArticleExpectedWastePreview(ctx, id, pct *decimal.Decimal)` en `server/internal/app/expected_waste.go` (valida con `ValidExpectedWastePct`, carga el grafo, lee nombres y la ficha, redondea con `Round2`/`Round4`), y `reason` en `SetArticleExpectedWaste` (trim, 1–200 si llega) hasta que T011 pase. Lectura de nombres en `server/queries/costing.sql` si hace falta.
- [ ] T013 [US1] `server/internal/httpapi/handlers_expected_waste.go`: `GET /stock/articles/{id}/expected-waste/preview?pct=` (`none` o número; ausente o mal formado → 400) y `reason` opcional en el PUT (DecodeStrict lo acepta); rutas con `menu.configure` en `server/internal/httpapi/router.go`. Tests de handler: `pct` ausente, `"abc"`, `100`, `none`; PUT con reason de 201 letras → 400.
- [ ] T014 [US1] Herramienta `ingredient_expected_waste_set` en `server/internal/httpapi/mcp_tools_099.go` (args de contracts/mcp.md, `pct` xor `clear`, `reason` obligatorio, vista previa = preview + notas (incluida la de insumo inactivo), rutas en `mcpToolRoutes`, `mcpListTool` → `articles_search`), refs en `server/internal/httpapi/mcp_refs.go`, entrada en el catálogo de `server/internal/domain/apitoken.go` (`ScopeWriteCatalog`, `PermMenuConfigure`). Antes: casos en `mcp_dispatch_test.go`/test nuevo para ni `pct` ni `clear`, los dos, sin `reason`; `TestEveryToolMatchesItsRouteGate` y `TestEveryWriteToolWithIDsDeclaresItsRefs` en verde.
- [ ] T015 [US1] Prueba MCP de punta a punta en `server/internal/integration/mcp_099_test.go`: vista previa no escribe; aplicar con clave; repetir misma clave → misma respuesta y un solo renglón de bitácora; `clear` → nulo; `"0"` → 0; el costo guardado de cada producto de `uses` es el `after` que anunció la vista previa (SC-002, FR-005); el renglón de bitácora lleva el origen del MCP y el usuario del token (FR-006).

## Phase 4: User Story 2 — Editar y apagar una categoría (P1)

**Goal**: `category_update` (nombre, activa, padre) y `categories_list` con inactivas.

**Independent Test**: quickstart §2 pasos 4–5.

- [ ] T016 [US2] Tests de integración en `server/internal/integration/categories_099_test.go` para `AdminService.UpdateCategory`: apagar sin productos activos (desaparece de `MenuCategories`, sube `catalog_versions`, bitácora `category.updated` con antes/después); con productos activos → `ErrCategoryHasActiveProducts`; con hijos activos → `ErrCategoryHasActiveChildren`; reactivar bajo padre inactivo → `ErrCategoryParentInvalid`; renombrar a un hermano (mayúsculas distintas) → `ErrCategoryNameTaken` con su id; renombrar a su mismo nombre → sin cambio ni bitácora; mover bajo raíz activa; volver raíz; mover con hijos → inválido; padre de otra empresa, id de otra empresa, conexión reciclada y sin empresa con `inTheThreeCases`; dos movimientos cruzados (A bajo B, B bajo A) en goroutines terminan uno aplicado y otro rechazado por la regla, sin 40P01.
- [ ] T017 [US2] `AdminService.UpdateCategory(ctx, id, CategoryPatch, actor)` en `server/internal/app/categories.go`: tx, `LockCategoriesForChange` con target y padre en orden de id, hechos, `CheckCategoryChange`, homónimo, update, 23505 de `categories_name_scope` → `ErrCategoryNameTaken` devuelto sin escribir bitácora, `auditEntity` solo con lo que cambió; hasta que T016 pase.
- [ ] T018 [US2] Mover `PatchCategory` a `server/internal/httpapi/handlers_categories.go` y ampliarlo: `name`, `isActive`, `parentId` (`null` = raíz; distinguir ausente de null con `json.RawMessage`), `foodFamily` igual que hoy y no combinable con los otros (400), nada → 400. Tests de handler de esos casos; el test existente de familia sigue verde.
- [ ] T019 [US2] Herramienta `category_update` en `server/internal/httpapi/mcp_tools_099.go` (`parent_id` xor `make_root`, al menos un campo; vista previa con estado actual, nuevo, productos activos/inactivos, subcategorías, y avisos primero si la ruta lo rechazará), `categories_list` con `include_inactive` en `server/internal/httpapi/mcp_tools_090_catalog.go`, refs, catálogo en `apitoken.go` y pistas `CATEGORY_*` en `toolFixByCode`. Tests previos de argumentos; tests de catálogo en verde.
- [ ] T020 [US2] En `server/internal/integration/mcp_099_test.go`: apagar por MCP una categoría sin productos activos (vista previa sin avisos, aplicar, ya no sale en `categories_list` y sí con `include_inactive`; el renglón `category.updated` lleva el origen del MCP, FR-019); con productos activos → error con la pista de `CATEGORY_HAS_ACTIVE_PRODUCTS`.

## Phase 5: User Story 3 — Alta de categoría (P2)

**Goal**: `category_create`.

**Independent Test**: quickstart §2 paso 6.

- [ ] T021 [US3] Tests de integración en `server/internal/integration/categories_099_test.go` para `AdminService.CreateCategory`: raíz; subcategoría bajo raíz activa; bajo subcategoría → inválido; bajo raíz inactiva → inválido; nombre repetido (otra capitalización) → `ErrCategoryNameTaken` con id; raíces con el mismo nombre en dos empresas conviven; padre de otra empresa con `inTheThreeCases`; bitácora `category.created`.
- [ ] T022 [US3] `AdminService.CreateCategory` en `server/internal/app/categories.go` y `POST /admin/categories` (201) en `server/internal/httpapi/handlers_categories.go` con `menu.configure` en `router.go`, hasta que T021 pase; tests de handler (nombre vacío, `parentId` no numérico).
- [ ] T023 [US3] Herramienta `category_create` en `server/internal/httpapi/mcp_tools_099.go` (vista previa con `similar` primero, padre resuelto, notas), refs y catálogo; caso en `mcp_099_test.go`: alta con clave, repetición, nombre repetido con la pista, y `similar` que incluye una categoría inactiva de nombre parecido.

## Phase 6: Polish y entrega

- [ ] T024 [P] Documentar en `docs/mecanica/catalogo-y-sucursales.md`: reglas del árbol (dos niveles, por qué), la carrera aceptada de producto activo en categoría inactiva (research R7), que las reglas viven en el servicio (SQL manual y el importador no las aplican) y que `parent_id` sigue siendo FK de una columna (#344).
- [ ] T025 [P] Documentar en `docs/mecanica/almacen.md`: la vista previa de merma simula sobre el grafo en memoria, «sin captura» ≠ 0 y el `reason` opcional de la ruta.
- [ ] T026 `cd server && go build ./... && go test ./...` y `make test-integration` en verde.
- [ ] T027 `/revision-de-codigo` (hook `after_implement`); cada hallazgo se corrige con su test.
- [ ] T028 Quickstart §2 contra el `make start` del worktree (API :8099) con un token MCP; lo creado se desactiva al terminar.
- [ ] T029 `make ci-local` verde; commits firmados sin `Co-Authored-By`; PR a `develop`, CI verde, merge con develop al día.
- [ ] T030 PR de release `develop` → `main`, CI verde, merge; respaldo previo; seguir el deploy hasta `readyz` 200 en producción y revisar logs sin 5xx.
- [ ] T031 Aviso a finanzas en el buzón pos-finanzas (`respuesta_pos_a_finanzas_AAAA-MM-DD.md`, sin secretos) y comentario en el #289 con lo que entró, sin precios ni nombres del negocio; `make cerrar-worktree datos=1` y borrar la rama remota.

## Dependencies & Execution Order

- Phase 1 → Phase 2 (T002–T008) → US2 y US3. US1 (T009–T015) solo depende de T001 y puede ir en paralelo con Phase 2.
- Dentro de cada historia: test → dominio → servicio → handler → herramienta MCP → prueba MCP.
- US3 reutiliza `categories.go` de US2 (T017) pero sus tests son independientes.
- Phase 6 al final; T029–T031 en serie.

## Parallel Example

- T002, T003 y T009 en paralelo (archivos distintos de `domain`).
- T024 y T025 en paralelo.

## Implementation Strategy

MVP = US1 + US2 (los dos casos reales esperando: la merma del insumo y apagar la categoría vacía). US3 entra en la misma subida; no hay razón para partirla.
