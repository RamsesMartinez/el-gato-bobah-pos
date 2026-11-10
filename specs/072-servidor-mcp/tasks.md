---
description: "Tareas del servidor MCP del POS"
---

# Tasks: Servidor MCP del POS

**Input**: `specs/072-servidor-mcp/` — plan.md, spec.md, research.md, data-model.md, contracts/, quickstart.md

**Tests**: obligatorios (constitución IV: TDD; V: ningún control de seguridad sin su test). Cada
tarea de test va **antes** de su implementación y se ve en rojo por la razón correcta.

**Rutas**: backend en `server/`, front en `web/src/`.

## Phase 1: Setup

- [X] T001 Agregar `github.com/modelcontextprotocol/go-sdk v1.8.0` a `server/go.mod` (`go get` + `go mod tidy`) y correr `govulncheck ./...` en `server/` sin hallazgos nuevos
- [X] T002 [P] Hook de pre-commit que rechaza el patrón `egbpat_[0-9]+_[0-9]+_[0-9a-f]{64}` en lo que se commitea: `scripts/hooks/sin-tokens-mcp.sh`, su test `scripts/tests/sin-tokens-mcp-test.sh` y su entrada en `lefthook.yml` (auditoría M7)

---

## Phase 2: Foundational (bloquea todas las historias)

### Dominio (puro, sin I/O)

- [X] T003 [P] Tests table-driven en `server/internal/domain/apitoken_test.go`: `FormatAPIToken`/`ParseAPIToken` (regex canónica, secreto hex de 64, sin ceros a la izquierda, `+2`, `0`, separadores de más, empresa/id fuera de rango → error), `ValidateAPITokenInput` (nombre 1–60 tras recortar, alcances conocidos, `read` agregado solo, duplicados, vigencias 30/90/180 y nada más), `ScopeAllows`, `DecideIdempotency` (nueva / misma huella terminada = repetir / huella distinta = conflicto / en curso / desconocida), `ValidIdempotencyKey`, `ToolErrorHint` por estado (400, 401, 403 permiso, 403 alcance, 404, 409, 422, 429, 5xx) y el catálogo `MCPTools()` (cada escritura tiene un alcance `write:*`, cada lectura `read`, nombres únicos en snake_case inglés)
- [X] T004 Implementar `server/internal/domain/apitoken.go` para que T003 pase
- [X] T005 [P] `PermIntegrationsManage = "integrations.manage"` en `server/internal/domain/permission.go` (posición 37 de `AllPermissions`, admin en `PermissionsFor`, área `business`, etiqueta y mensaje de permiso) y acciones `mcp.tool_called`, `api_token.created`, `api_token.revoked` en `server/internal/domain/audit.go`, con sus casos en los tests existentes de esos archivos
- [X] T006 [P] Espejo del permiso en `web/src/app/permissions.ts` (tipo `Permission`) y su etiqueta donde la pantalla de roles muestra el catálogo

### Migración 0111

- [X] T007 Test de integración `server/internal/integration/migration_0111_test.go` contra base restaurada con dos empresas: tablas con RLS y política `nullif`, checks (alcances, vigencia ≤ 181 días, `revoked_at`/`revoked_by`, clave, estado/resultado, tamaño), trigger de revocación terminal, `update(name)` → 42501 bajo `appRoleStore`, el permiso 37 en el catálogo y en el admin, permisos efectivos de un admin y listado de roles sin trono con la fila 37 presente (versión anterior), y el Down con su guarda; renglón en `guardedDownCleanup` de `server/internal/integration/harness_restaurado_test.go`
- [X] T008 Escribir `server/migrations/0111_api_tokens.sql` según data-model.md (Up y Down) hasta que T007 pase; aplicarla a la base local
- [X] T009 [P] Respaldo anónimo: `truncate api_tokens, mcp_idempotency_keys` en el paso 5 y sus conteos en `FUGAS` de `scripts/respaldo-anonimo.sh`; `scopes`, `tool`, `status` en `noPersonalData` (o cobertura por truncate) para que pase `TestEveryColumnIsCoveredByTheAnonymousBackup`
- [X] T010 `server/queries/api_tokens.sql` (insertar, listar por empresa con dueño, leer para autenticar con usuario y rol activos, marcar uso cada minuto, revocar idempotente; idempotencia: insertar `pending` con `on conflict do nothing`, leer, terminar, marcar desconocida, borrar, purgar 7 días del token) y `make sqlc`

### Servicio

- [X] T011 Tests de integración bajo `appRoleStore` en `server/internal/integration/api_tokens_test.go`: `inTheThreeCases` para las dos tablas; `Create` siempre a nombre de quien llama; `Authenticate` rechaza igual inexistente, revocado, vencido, persona dada de baja, rol inactivo y empresa falsificada en el texto (`TestAPITokenRejectionsLookAlike`, `TestAPITokenFromOtherCompanyIsUnknown`); `last_used_at` como mucho una vez por minuto; idempotencia de punta a punta en el servicio
- [X] T012 Implementar `server/internal/app/api_tokens.go` (`APITokensService`: `Create` con verificación de contraseña, `List`, `Revoke`, `Authenticate`, `BeginIdempotent`/`FinishIdempotent`/`AbandonIdempotent`/`MarkUnknown`, `RecordCall`) con bitácora `api_token.created`/`api_token.revoked` en la misma transacción

### Transporte MCP

- [X] T013 Tests en `server/internal/httpapi/mcp_test.go`: `TestPATIsRejectedOnRegularRoutes` (Bearer egbpat en `/api/v1/me` → 401), `TestDispatcherRefusesRoutesOutsideCatalog` (`/me/pin`, `/me/password`, `/users`, `/roles`, `/integrations/tokens`, `/auth/pin-switch`, `/orders/{id}/pay` → rechazado sin llegar al handler), `TestEveryToolMatchesItsRouteGate` (el patrón de cada herramienta existe en el router y su permiso declarado es el de la ruta), lote JSON-RPC (`[`) → 400, `Origin` no permitido → 403, `GET`/`DELETE` → 405, token solo por header, límite por token con `recordAndOver` contando por `tools/call`, `tools/list` filtrado por alcance × permiso, herramienta no listada → desconocida, `dry_run` ausente/`null` = vista previa, el log no contiene `egbpat_` tras 401, 500 y nivel debug; por tabla, cada una de las 11 escrituras con un usuario sin su permiso → rechazada con el mensaje del permiso (SC-005); los esquemas tipan dinero como texto y fechas con patrón `AAAA-MM-DD`, y un número en `amount` se rechaza (FR-016)
- [X] T014 `server/internal/httpapi/mcp.go`: middleware del token (`store.WithTenant` corto, sin retener conexión), principal interno no exportado con el origen de afuera, `RequireAuth` lo honra, `withRequestOrigin` no lo pisa, despachador con lista blanca por patrón de chi, servidor MCP por petición (`Stateless`, `JSONResponse`), límites `mcpIPs` y `mcpTokens`, evento `api_token_rejected`, idempotencia y `RecordCall` con `context.WithoutCancel`; montar `/mcp` en `server/internal/httpapi/router.go` y cablearlo en `server/cmd/api`
- [X] T015 Prueba de pool: con `MaxConns = 2`, tres llamadas paralelas terminan (`server/internal/integration/mcp_pool_test.go`)

**Checkpoint**: `/mcp` responde `initialize` y `whoami` con un token creado en el test.

---

## Phase 3: User Story 2 — Crear y revocar tokens (P1)

**Goal**: el admin crea, ve una vez, lista y revoca tokens. **Independent Test**: crear en la pantalla, conectar, revocar y ver el 401.

- [X] T016 [P] [US2] Tests en `server/internal/httpapi/handlers_integrations_test.go`: sin `integrations.manage` → 403; contraseña equivocada → 401 y cuenta en el bloqueo; `userId` en el cuerpo se ignora; respuesta con el token una vez y `mcpUrl`; revocar de otra empresa → 404; revocar dos veces → 204
- [X] T017 [US2] `server/internal/httpapi/handlers_integrations.go` y rutas `GET/POST /integrations/tokens`, `POST /integrations/tokens/{id}/revoke` con `RequirePermission(integrations.manage)` y `rateLimitUser`; `SecurityEvent("api_token_created")`; regenerar `server/internal/httpapi/testdata/route_gates.golden` con el porqué en el PR
- [ ] T018 [P] [US2] Cliente en `web/src/api/integrations.ts` (tipos con arreglos opcionales)
- [ ] T019 [US2] Tests en `web/src/features/admin/IntegrationsPage.test.tsx`: activos primero y revocados plegados; nombre por omisión «Claude de <nombre>»; «Consultar» fijo y cuatro alcances tocables; vigencia con `Picker`; contraseña obligatoria; la hoja del token no se cierra al tocar el fondo; «Copiar token» → «Copiado»; el comando sale de la URL de la API del build y del slug; revocar pide confirmación (maqueta A, plan «Maquetas»)
- [ ] T020 [US2] `web/src/features/admin/IntegrationsPage.tsx` según la fila «Integraciones / A» de «Maquetas» del plan y sus ajustes; ruta `/integraciones` en `web/src/App.tsx` y `web/src/app/routes.ts`; ítem del riel en `web/src/app/nav.ts` con `permission: 'integrations.manage'`
- [ ] T021 [US2] Medir con Playwright a 1024×600 (local) que en la hoja de crear se ven los alcances y «Crear token» sin desplazar, y que la lista muestra los activos primero

---

## Phase 4: User Story 1 — Capturar un gasto desde Claude (P1) 🎯 MVP

**Goal**: `expense_create` (tarjeta y efectivo), `expense_pay`, `expense_cancel`, `supplier_create` y las lecturas que necesitan. **Independent Test**: quickstart paso 3, gastos.

- [ ] T022 [P] [US1] Tests de integración en `server/internal/integration/mcp_expenses_test.go`: vista previa no guarda nada y resuelve nombres; aplicar con tarjeta y con efectivo (la salida aparece en el turno); `TestSameKeyTenTimesOneExpense`; misma clave con otros argumentos → error; `TestReadScopeCannotWrite`; sin `expenses.capture` → error con el mensaje del permiso; `TestEveryCallLeavesAnAuditEvent` con IP del cliente y `device.agent = "mcp"`; caja cerrada → error con causa y corrección
- [ ] T023 [US1] Herramientas de gastos en `server/internal/httpapi/mcp_tools.go`: `expenses_list`, `expense_get`, `expense_categories_list`, `suppliers_list`, `payment_methods_list`, `cash_registers_list`, `expense_create`, `expense_pay`, `expense_cancel`, `supplier_create`, con vista previa por GET despachados y anotaciones `readOnlyHint`/`destructiveHint`

---

## Phase 5: User Story 3 — Cambiar precios (P2)

**Goal**: `product_update`, `modifier_option_update`, `platform_price_set`, `platform_price_delete` y sus lecturas. **Independent Test**: cambiar un precio y verlo en el menú del POS.

- [ ] T024 [P] [US3] Tests en `server/internal/integration/mcp_catalog_test.go`: cambiar solo el precio no toca nombre, categoría, horario ni costo; el tope por renglón de un extra no cambia; `TestRoleLosesPermissionMidway` (quitar `menu.configure` al rol entre dos llamadas); el menú se invalida igual que desde la pantalla
- [ ] T025 [US3] Herramientas `products_list`, `modifier_options_list`, `platform_prices_list`, `product_update`, `modifier_option_update`, `platform_price_set`, `platform_price_delete` en `server/internal/httpapi/mcp_tools.go`

---

## Phase 6: User Story 4 — Consultar el negocio (P2)

**Goal**: lecturas de ventas, caja, inventario, reportes, asistencia y bitácora. **Independent Test**: token de solo lectura consulta y no escribe.

- [ ] T026 [P] [US4] Tests en `server/internal/integration/mcp_reads_test.go`: `TestMCPOtherCompanySeesNothing` (token de la empresa 1 no ve un pedido ni un gasto de la 2); rango mal escrito → error de validación con el campo; `order_get` no trae nombre ni notas del cliente; páginas con tope
- [ ] T027 [US4] Herramientas `whoami`, `sales_summary`, `sales_list`, `order_get`, `report_products_sold`, `attendance_report`, `cash_session_current`, `cash_sessions_history`, `stock_levels`, `stock_movements`, `audit_list` en `server/internal/httpapi/mcp_tools.go`

---

## Phase 7: User Story 5 — Caja e inventario (P3)

- [ ] T028 [P] [US5] Tests en `server/internal/integration/mcp_cash_inventory_test.go`: salida con concepto y entrada con texto; salida sin concepto → error que remite a `cash_concepts_list`; merma sobre el tope queda pendiente; ajuste con antes y después
- [ ] T029 [US5] Herramientas `cash_concepts_list`, `cash_movement_create`, `waste_reasons_list`, `waste_record`, `stock_adjust` en `server/internal/httpapi/mcp_tools.go`

---

## Phase 8: Polish

- [ ] T030 [P] `docs/mecanica/personal-y-permisos.md`: la puerta para agentes (cómo funciona el token, la lista blanca del despachador, qué no revoca un token, cómo agregar una herramienta) y su renglón en la tabla de `AGENTS.md` si cambia
- [ ] T031 Prueba real con un cliente MCP (Claude Code con `claude mcp add`) contra `make start` del worktree siguiendo quickstart.md; anotar en quickstart si la variable `${POS_MCP_TOKEN}` se expande en el alcance `user`
- [ ] T032 `cd server && go build ./... && go test ./...`, `make test-integration`, `cd web && bun run lint && bun run vitest run && bun run build`
- [ ] T033 `/revision-de-codigo` (hook `after_implement`) y corregir lo que encuentre con su test

## Dependencies

- Setup → Foundational (T003–T015) → historias. US2 primero en la práctica (sin token no hay prueba manual), aunque US1 se prueba con tokens creados en el test.
- US1, US3, US4, US5 tocan el mismo `mcp_tools.go`: van en serie; sus tests [P] se pueden escribir en paralelo.
- T031–T033 al final.

## Parallel

- T002, T003, T005, T006 en paralelo. T009 en paralelo con T010–T012. T016, T018 en paralelo con T019.

## Implementation Strategy

MVP = Foundational + US2 + US1 (crear token y capturar un gasto). Después US3 (criterio de aceptación del precio), US4 y US5.
