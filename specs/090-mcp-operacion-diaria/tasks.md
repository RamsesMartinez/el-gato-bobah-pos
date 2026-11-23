---
description: "Tareas de la 090: MCP para la operación diaria (paquete 1 del #289)"
---

# Tasks: MCP para la operación diaria (paquete 1 del #289)

**Input**: `specs/090-mcp-operacion-diaria/` (plan, spec, research, data-model, contracts, quickstart)

**Tests**: TDD obligatorio (constitución IV). Cada tarea de test va antes de su implementación y se ve
en rojo por la razón correcta. Las pruebas de integración van en `server/internal/integration/` y
corren con `make test-integration` (Postgres de pruebas del worktree, API como `gatobobah_app`).

## Phase 1: Setup

- [X] T001 Confirmar que la 0116 sigue libre (`git fetch` y `git ls-tree -r --name-only origin/<rama> server/migrations | grep 0116` en cada rama remota, y `ls ../egb-*/server/migrations/0116*`); anotar el resultado en `specs/090-mcp-operacion-diaria/research.md` R10
- [ ] T002 Restaurar en la base del worktree el respaldo más reciente de producción (`make db-restaurar`) y crear un token MCP de prueba para un admin de `gatobobah` (quickstart)

## Phase 2: Foundational (bloquea las historias 2 a 7)

- [ ] T003 [P] Test unitario de `DerivedUUID` (determinista, cambia con cada parte, versión 8 y variante RFC) en `server/internal/domain/mcp_writes_test.go`
- [ ] T004 Implementar `domain.DerivedUUID(parts ...string) uuid.UUID` en `server/internal/domain/mcp_writes.go`
- [ ] T005 [P] Test del despachador con parámetro UUID: acepta un UUID y rechaza `../`, `%2F`, vacío y texto (`TestDispatcherRejectsNonUUIDDraftID`) en `server/internal/httpapi/mcp_dispatch_test.go`
- [ ] T006 Extender `mcpSession.dispatch` con parámetros `uuid.UUID` (escritos con `String()`), sin cambiar a los llamadores enteros, en `server/internal/httpapi/mcp.go`
- [ ] T007 Agregar las 22 herramientas nuevas al catálogo (`mcpTools`) con alcance y permiso según plan.md «Herramientas», y ampliar las descripciones de `product_update` y `modifier_option_update`, en `server/internal/domain/apitoken.go`; extender `TestEveryToolMatchesItsRouteGate` si hace falta para las rutas nuevas
- [ ] T008 Crear el esqueleto `server/internal/integration/mcp_090_test.go` con los ayudantes de la 076 (servidor, token, llamada `tools/call`), un subtest por herramienta que hoy falla con «herramienta desconocida»

## Phase 3: User Story 1 — Los tres defectos (P1) 🎯 MVP

**Goal**: el `PATCH` parcial no pisa, la herramienta encuentra opciones de grupos inactivos, y el
grupo apagado se ve.
**Independent Test**: los tres tests de regresión pasan; antes fallaban.

- [ ] T009 [P] [US1] Test de regresión `TestOptionPatchKeepsWhatItDoesNotSend` (solo nombre, solo precio, solo tope; otra empresa y los tres casos de RLS) en `server/internal/integration/option_patch_test.go` — rojo hoy
- [ ] T010 [P] [US1] Test de regresión `TestOptionUpdateFindsOptionOfInactiveGroup` (MCP, vista previa avisa del grupo) en `server/internal/integration/mcp_090_test.go` — rojo hoy
- [ ] T011 [P] [US1] Test `inactive_group_count` en la lista de productos (y aislamiento en los tres casos) en `server/internal/integration/option_patch_test.go`
- [ ] T012 [US1] Consultas `AdminGetOption` y `AdminLockOption` (`for update of mo`) e `inactive_group_count` en `server/queries/admin.sql`; `make sqlc`
- [ ] T013 [US1] `AdminService.GetOption` y `UpdateOptionFields` con parche (`OptionFieldsPatch`, lee con candado y escribe en una transacción) en `server/internal/app/modifiers_admin.go`; `InactiveGroupCount` en `AdminProductView` en `server/internal/app/admin.go`
- [ ] T014 [US1] Handler `AdminGetOption` (`GET /admin/modifier-options/{id}`) y `AdminUpdateOption` con parche en `server/internal/httpapi/handlers_admin.go`; ruta en `server/internal/httpapi/router.go`
- [ ] T015 [US1] `modifier_option_update` ampliada (nombre, precio, tope, activa; lee con la ruta por id; manda solo lo pedido; aviso de grupo inactivo) en `server/internal/httpapi/mcp_tools_write.go` y su ruta en `mcpToolRoutes`
- [ ] T016 [P] [US1] Vitest de la insignia «N inact.» con `title` (y que el número siga contando los activos) en `web/src/features/admin/ProductsAdminPage.test.tsx` — plan.md «Pantallas y maquetas», fila Menú › Productos
- [ ] T017 [P] [US1] Vitest del aviso al archivar un grupo con productos activos (singular y plural; reactivar no repite) en `web/src/features/admin/ModifierOptionsPage.test.tsx` — fila Menú › Modificadores
- [ ] T018 [US1] `inactiveGroupCount` en `web/src/api/admin.ts`; insignia en `web/src/features/admin/ProductsAdminPage.tsx`; descripción del aviso en `web/src/features/admin/ModifierOptionsPage.tsx` (contando activos de la lista de productos del grupo)

## Phase 4: User Story 2 — Pedido completo (P1)

**Goal**: los seis pasos de un pedido por MCP.
**Independent Test**: pedido de plataforma creado, enviado, entregado, con descuento y cobrado por
MCP, con total igual al de la vista previa.

- [ ] T019 [P] [US2] Test unitario `PlatformPriceSource` (mostrador, propio, margen) en `server/internal/domain/platform_price_test.go`
- [ ] T020 [US2] `domain.PlatformPriceSource` en `server/internal/domain/platform_price.go`
- [ ] T021 [P] [US2] Test de integración de `POST /pos/drafts/quote`: total igual al de la cuenta guardada con plataforma, extras y descuento; origen por renglón; rechaza lo que crear rechaza; no crea nada; aislamiento en los tres casos — en `server/internal/integration/drafts_quote_test.go`
- [ ] T022 [US2] Extraer `priceDraft` de `buildDraftView` y agregar `DraftsService.Quote` (con `validLineCmd`, tope y `checkLineSells`) en `server/internal/app/drafts.go`
- [ ] T023 [US2] Handler `QuoteDraft` y ruta `POST /pos/drafts/quote` en `server/internal/httpapi/handlers_drafts.go` y `server/internal/httpapi/router.go`
- [ ] T024 [P] [US2] Subtests MCP: `order_draft_create` (vista previa, aplicar, misma clave), `order_draft_send` (repetir no duplica), `order_draft_discard`, `order_deliver`, `order_discount_set` (monto, porcentaje, `clear`, ninguno es error), `order_pay` (vista previa con quote, misma clave cobra una vez, método de otra plataforma rechazado) en `server/internal/integration/mcp_090_test.go`
- [ ] T025 [US2] Herramientas de pedido en `server/internal/httpapi/mcp_tools_090_orders.go` (rutas en `mcpToolRoutes`, `mcpListTool`)

## Phase 5: User Story 3 — Corregir un cobro (P1)

**Goal**: anular, cambiar terminal y reembolsar por MCP.
**Independent Test**: los tres efectos en el pedido y en el corte; permisos negados.

- [ ] T026 [P] [US3] Subtests MCP: `order_payment_void` (motivo obligatorio, turno cerrado rechazado, evento de seguridad), `order_payment_terminal_set`, `order_refund` (vista previa = refund-info; folio exigido), `card_terminals_list`, y rol sin `payments.void` / `payments.change_terminal` en `server/internal/integration/mcp_090_test.go`
- [ ] T027 [US3] Herramientas de corrección de cobro en `server/internal/httpapi/mcp_tools_090_orders.go`

## Phase 6: User Story 4 — Costo con motivo (P1)

**Goal**: `ingredient_cost_set` con motivo en el historial.
**Independent Test**: el historial muestra el motivo; la pantalla sigue sin motivo.

- [ ] T028 [P] [US4] Test de migración `migration_0116_test.go` (base restaurada con dos empresas: sube, inserta sin y con motivo, rechaza vacío y con espacios, baja) en `server/internal/integration/migration_0116_test.go`
- [ ] T029 [US4] Migración `server/migrations/0116_cost_correction_reason.sql` (Up/Down) — `db-architect` antes de aplicarla
- [ ] T030 [US4] Respaldo anónimo: `reason` → `'motivo anonimizado'` en el paso 5 y en `FUGAS` de `scripts/respaldo-anonimo.sh`
- [ ] T031 [P] [US4] Test de la ruta con y sin `reason` y del historial con `reason` en `server/internal/integration/ingredients_admin_test.go`
- [ ] T032 [US4] `InsertCostCorrection` con `reason` y el historial con `reason` (`null::text` en las otras ramas) en `server/queries/ingredients_admin.sql`; `make sqlc`; `CorrectArticleCost(ctx, id, cost, reason *string)` recortando, con `reason` en la bitácora, en `server/internal/app/ingredients_admin.go`; handler en `server/internal/httpapi/handlers_ingredients_admin.go`
- [ ] T033 [P] [US4] Subtests MCP de `ingredient_cost_set` (vista previa con ficha e historial, sin motivo rechazado, preparado rechazado, permiso) en `server/internal/integration/mcp_090_test.go`
- [ ] T034 [US4] `ingredient_cost_set` en `server/internal/httpapi/mcp_tools_090_catalog.go`

## Phase 7: User Story 5 — Recetas (P1)

- [ ] T035 [P] [US5] Subtests MCP de `recipe_get` y `recipe_set` (producto y extra; homónimos no cambian por omisión y sí con `also_in_same_name`; `based_on` viejo → 409 con pista; existencia propia: vista previa con la pregunta y rechazo que dice qué falta) en `server/internal/integration/mcp_090_test.go`
- [ ] T036 [P] [US5] Test de las pistas nuevas de receta en `toolFixByCode` en `server/internal/domain/apitoken_test.go`
- [ ] T037 [US5] Pistas en `server/internal/domain/apitoken.go`; `recipe_get` y `recipe_set` en `server/internal/httpapi/mcp_tools_090_catalog.go`

## Phase 8: User Story 6 — Grupos, opciones y ligas (P1)

- [ ] T038 [P] [US6] Subtests MCP de `modifier_groups_list`, `modifier_group_create` (nombre repetido), `modifier_group_update` (solo `active` no toca lo demás; vista previa con productos), `modifier_option_create` (nombre repetido en el grupo), `product_groups_get`, `product_group_link` (mínimo > máximo; sin ambos hereda; grupo inactivo avisa), `product_group_unlink` en `server/internal/integration/mcp_090_test.go`
- [ ] T039 [US6] Herramientas en `server/internal/httpapi/mcp_tools_090_catalog.go`

## Phase 9: User Story 7 — Productos (P1)

- [ ] T040 [P] [US7] Subtests MCP de `categories_list`, `product_create` (parecidos en `attention`, nombre repetido, categoría ajena) y `product_update` ampliada (solo nombre conserva precio, categoría, costo, horario, favorito; costo manual y de receta) en `server/internal/integration/mcp_090_test.go`
- [ ] T041 [US7] `categories_list` y `product_create` en `server/internal/httpapi/mcp_tools_090_catalog.go`; `product_update` ampliada en `server/internal/httpapi/mcp_tools_write.go`

## Phase 10: Polish

- [ ] T042 [P] Texto de los alcances (Almacén, Menú y precios, Pedidos) y su vitest en `web/src/features/admin/IntegrationsPage.tsx` / `.test.tsx` — plan.md fila Integraciones
- [ ] T043 [P] Documentar en `docs/mecanica/personal-y-permisos.md` qué agrega la 090 a la puerta para agentes (ensanche de tokens, UUID en el despachador, cotización de cuenta)
- [ ] T044 `make api-build && make api-test`, `make test-integration`, `make web-test`, `bun run lint` en `web/`
- [ ] T045 `/revision-de-codigo` y `/security-review`; cada hallazgo con su test
- [ ] T046 Prueba de punta a punta con cliente MCP real contra `make start` y base restaurada (quickstart), cobrando y entregando cada pedido de prueba; medir la insignia y la hoja de Integraciones a 1280×854
- [ ] T047 `make ci-local` verde; PR a `develop` al día; merge con CI verde; release `develop → main` con respaldo previo, nombrando los dos PR de Dependabot; deploy hasta `readyz` 200; journal sin ERROR ni 5xx (salvo #312)
- [ ] T048 Comentar en el #289 qué entró, el ensanche de tokens y los paquetes siguientes; `make cerrar-worktree datos=1`; borrar la rama remota

## Dependencies & Execution Order

- Setup → Foundational (T003–T008) → historias.
- US1 no depende de Foundational salvo T007/T008 para su subtest MCP; se puede subir antes.
- US2 y US3 comparten `mcp_tools_090_orders.go` (US3 después de US2 en el mismo archivo).
- US4–US7 comparten `mcp_tools_090_catalog.go`; independientes entre sí en lo funcional.
- Polish al final.

## Parallel Example

```text
T003, T005 (tests de fundación) en paralelo.
T009, T010, T011, T016, T017 (tests de US1) en paralelo.
T019, T021, T024 (tests de US2) en paralelo.
```

## Implementation Strategy

MVP = US1 (los tres defectos). Después US2 y US3 (dinero), luego US4–US7 (catálogo). Una sola subida
al final del ciclo, como pide el encargo.

## Cobertura requisito → tarea

| FR | Tareas |
| --- | --- |
| FR-001 | T009, T012–T014 |
| FR-002 | T010, T012–T015 |
| FR-003 | T011, T012, T013, T016, T018 |
| FR-004 | T017, T018 |
| FR-005–FR-007 | T021–T025, T005, T006, T003, T004 |
| FR-008, FR-009 | T024, T025 |
| FR-010–FR-013 | T026, T027 |
| FR-014, FR-015 | T028–T034 |
| FR-016–FR-018 | T035–T037 |
| FR-019, FR-020 | T015, T038, T039 |
| FR-021–FR-023 | T040, T041 |
| FR-024 | T007, T008 y cada subtest MCP (bitácora y clave) |
| FR-025 | cada subtest de rechazo |
| FR-026 | T048 |
