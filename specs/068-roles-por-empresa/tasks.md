---

description: "Tareas de la entrega 1 de la 068: el cajero ve los pedidos entregados"
---

# Tasks: Roles por empresa — entrega 1: el cajero ve los pedidos entregados

**Input**: Design documents from `specs/068-roles-por-empresa/`

**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/orders-delivered.md

**Tests**: TDD obligatorio (constitución IV): cada test se escribe y se ve fallar antes de su código.

## Format: `[ID] [P?] [Story] Description`

## Phase 1: Setup

Sin tareas: monorepo existente, sin dependencias nuevas ni migración.

---

## Phase 2: Foundational (el permiso existe)

**Purpose**: el permiso `orders.delivered_view` existe en el mapa de roles sugeridos y en el espejo del front. Bloquea todas las historias.

- [X] T001 Test que falla: en `server/internal/domain/permission_test.go`, `TestPermissionsForEachRoleOfToday` espera `PermOrdersDeliveredView` en admin (vía `all`), gerente y cajero, y NO en mesero; `TestEveryPermissionHasADeniedMessageWithoutRoleNames` espera «Tu usuario no puede ver los pedidos entregados». Para separar cajero de mesero en la tabla, cada uno lleva su propia lista.
- [X] T002 En `server/internal/domain/permission.go`: constante `PermOrdersDeliveredView Permission = "orders.delivered_view"` con su doc-comment (el porqué: reimprimir el ticket de un pedido cerrado es del mostrador; devolver dinero va aparte), agregarla a `AllPermissions`, a gerente y a cajero (no a mesero: separar `case RoleCajero, RoleMesero`), y su mensaje en `PermissionDeniedMessage`. Verificar T001 en verde.
- [X] T003 [P] En `web/src/app/permissions.ts`: agregar `'orders.delivered_view'` al tipo `Permission`.

**Checkpoint**: `go test ./internal/domain/` verde.

---

## Phase 3: User Story 1 — La cajera reimprime el ticket de un pedido entregado (P1) 🎯 MVP

**Goal**: con rol cajero, la lista de entregados llega del servidor y se pinta en Pedidos, sin «Devolver».

**Independent Test**: cajero → `GET /orders/delivered` 200 y `POST /orders/{id}/refund` 403; en la pantalla aparece la sección con «Ticket» y sin «Devolver».

### Tests (primero, en rojo)

- [X] T004 [P] [US1] Test de integración nuevo en `server/internal/integration/entregadas_por_permiso_test.go` (build tag `integration`, reusa `routerDePedidos`, `makeUser`, `crearYCobrar`, `abrirCajaPrincipal`, `do`): un pedido cobrado en efectivo, capturado y entregado por un gerente; con token de cajero, `GET /api/v1/orders/delivered` responde 200 y contiene ese pedido; `GET /api/v1/orders/{id}/refund-info` y `POST /api/v1/orders/{id}/refund` responden 403. Debe fallar hoy con 403 en la primera petición.
- [X] T005 [P] [US1] Test en `web/src/features/orders/OrdersBoardPage.test.tsx`: con una sesión de rol cajero que trae `orders.delivered_view` (y sin `payments.void`), la sección de entregados se pinta con su pedido, tiene «Ticket» y no tiene «Devolver»; y con rol `admin` pero sin `orders.delivered_view` en `permissions`, la sección NO se pinta (prueba que la pantalla pregunta por permiso y no por rol).

### Implementation

- [X] T006 [US1] En `server/internal/httpapi/router.go`: `GET /delivered` con `RequirePermission(h.permissions, domain.PermOrdersDeliveredView)`; `POST /{id}/refund` y `GET /{id}/refund-info` con `RequirePermission(h.permissions, domain.PermPaymentsVoid)`. Actualizar el comentario de esas líneas (ver: del mostrador; devolver: misma capacidad que devolver un pago).
- [X] T007 [US1] En `web/src/features/orders/OrdersBoardPage.tsx`: quitar `canRefund` (pregunta por rol); `puedeVerEntregadas = can('orders.delivered_view', user)` decide `enabled` de la consulta de entregadas y si se pinta `<Entregadas>`; `onRefund` se pasa solo si `can('payments.void', user)` y `Entregadas` pinta «Devolver» solo con `onRefund` (además de `quedaPorDevolver(o) > 0`). Actualizar el comentario.
- [X] T008 [US1] Correr T004 y T005: verdes.

**Checkpoint**: la cajera ve y reimprime; no devuelve.

---

## Phase 4: User Story 2 — Gerente y admin siguen igual (P1)

**Goal**: nada se le quita a quien ya podía.

- [X] T009 [US2] Extender `server/internal/integration/entregadas_por_permiso_test.go`: con token de gerente y de admin, `GET /orders/delivered` 200, `GET /orders/{id}/refund-info` 200 y `POST /orders/{id}/refund` acepta la devolución del pedido cobrado (200).
- [X] T010 [P] [US2] En `web/src/features/orders/OrdersBoardPage.test.tsx`: con permisos `orders.delivered_view` y `payments.void`, la sección se pinta con «Ticket» y «Devolver» (ajustar los tests existentes que armaban la sesión solo por rol para que traigan su lista de permisos).

---

## Phase 5: User Story 3 — El mesero no lo trae por omisión (P2)

- [X] T011 [US3] Extender `server/internal/integration/entregadas_por_permiso_test.go`: con token de mesero, `GET /orders/delivered` responde 403 con «Tu usuario no puede ver los pedidos entregados».
- [X] T012 [P] [US3] En `web/src/features/orders/OrdersBoardPage.test.tsx`: con la sesión de mesero (sin `orders.delivered_view`), la sección no se pinta y no se pide la lista.

---

## Phase 6: Polish & Cross-Cutting

- [X] T013 [P] Constitución, tabla de puertas del principio VIII (renglón de roles): actualizar la cuenta de rutas con `RequireRole` (tres menos) sin cambiar la regla; enmienda PATCH con su versión y fecha en `.specify/memory/constitution.md`.
- [ ] T014 Gates: `cd server && go build ./... && go test ./...`; `make test-integration`; `cd web && bun run lint && bun run vitest run && bun run build`.
- [ ] T015 Quickstart a 1024×600 contra el `make start` del worktree (`specs/068-roles-por-empresa/quickstart.md`, paso 4) con un cajero y un pedido entregado por otro usuario; cobrar y entregar lo que se cree. Observaciones en texto, capturas solo en `test-results/`.
- [X] T016 Abrir el issue de la entrega 2 enlazado al #61 (roles en la base, pantalla, migrar `RequireRole`/`ROUTE_ROLES`, `users.role`, reglas del 2026-10-13, preguntas de diseño para el dueño).

---

## Dependencies & Execution Order

- Phase 2 (T001→T002, T003) bloquea todo.
- US1: T004, T005 en paralelo (rojo) → T006, T007 → T008.
- US2 y US3 extienden los mismos archivos de test que US1: van después de US1, en el orden US2 → US3.
- Polish al final; T016 puede ir en cualquier momento.

## Parallel Example: User Story 1

```text
T004 (integración, server) ‖ T005 (vitest, web)
T006 (router) ‖ T007 (pantalla)
```

## Implementation Strategy

Una sola subida: US1 es el MVP y US2/US3 son sus bordes (lo que no debe cambiar y lo que no debe
abrirse). Sin migración, la subida no tiene paso de base.
