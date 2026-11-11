---
description: "Tareas de la 076: crear y editar insumos desde el MCP, y empaques por insumo"
---

# Tasks: Crear y editar insumos desde el servidor MCP, y empaques por insumo

**Input**: [spec.md](spec.md), [plan.md](plan.md), [research.md](research.md), [contracts/mcp-tools.md](contracts/mcp-tools.md), [quickstart.md](quickstart.md)

**Tests**: obligatorios (constitución IV, TDD): cada test se escribe y se ve en rojo por la razón
correcta antes del código que lo pone en verde. Integración en
`server/internal/integration/mcp_076_test.go` (`//go:build integration`, `make test-integration`).

## Phase 1: Setup

- [x] T001 Confirmar que no hace falta migración ni alcance nuevo (0113 libre en `origin/develop` y ramas remotas) y anotarlo en specs/076-mcp-insumos/plan.md (Technical Context)

## Phase 2: Foundational (pistas de error con el id del insumo)

- [x] T002 Test unitario en server/internal/domain/apitoken_test.go: la pista de `NAME_TAKEN` con `existingId` y `archived` dice «id N» y «archivado»; la de `ALT_NAME_TAKEN` dice el id del dueño; la de `STALE` dice «vuelve a intentar». Verlo en rojo
- [x] T003 Implementar en server/internal/domain/apitoken.go la pista con el id (función que recibe código, mensaje, id y archivado) y los tres códigos en `toolFixByCode`; en server/internal/httpapi/mcp.go `toolHint` pasa `details.existingId`/`details.archived` del sobre de error. T002 en verde

## Phase 3: User Story 1 - Dar de alta el insumo que trae el ticket (P1)

**Goal**: `ingredient_create` con vista previa que pone primero los parecidos.

**Independent Test**: vista previa de un nombre parecido a uno existente (lo señala primero); alta de uno nuevo; compra en vista previa con él.

- [x] T004 [US1] Tests en server/internal/integration/mcp_076_test.go (`TestMCPIngredientCreate`), en rojo (herramienta desconocida): vista previa no crea nada y su primer campo es `maybe_exists` con el parecido por nombre y por otro nombre, y la primera nota dice «Quizá ya existe: X (id N)»; sin parecidos lo dice; nota de existencia inicial y compra; diez veces la misma clave → un insumo con sus campos y `ingredient.created` en bitácora más `mcp.tool_called` con el después; nombre repetido (activo y archivado) → error con el id; unidad kg → rechazo de la ruta; token sin `write:inventory` → no se ofrece y se niega nombrando el alcance; rol sin `inventory.configure` (con ver y recibir) → no se ofrece, 403 y nada creado; token de otra empresa con la categoría de ésta → rechazo; rol con configurar y sin ver el almacén → la vista previa dice que no pudo revisar la búsqueda por otros nombres; insumo nuevo + `presentation_create` + vista previa de `purchase_create` que lo acepta
- [x] T005 [US1] Catálogo: renglón `ingredient_create` (`write:inventory`, `inventory.configure`) en server/internal/domain/apitoken.go
- [x] T006 [US1] Implementar `ingredient_create` en server/internal/httpapi/mcp_tools_076.go: rutas (`POST /stock/articles`, `GET /stock/articles/similar`, `GET /stock/articles`), cuerpo de la hoja «Nuevo artículo», vista previa con `maybe_exists` primero (R1), estado después con la ficha. T004 en verde

## Phase 4: User Story 2 - Renombrar un insumo y ajustar su ficha (P1)

**Goal**: `ingredient_update` sobre PATCH de la ficha y PUT de otros nombres, en ese orden.

**Independent Test**: renombrar un insumo y dejar el nombre viejo como otro nombre; lo demás se conserva.

- [x] T007 [US2] Tests en server/internal/integration/mcp_076_test.go (`TestMCPIngredientUpdate`), en rojo: vista previa con antes y después y parecidos al nombre nuevo sin el propio, y parecidos de cada otro nombre nuevo, sin cambiar nada; renombrar y dejar el nombre viejo como otro nombre en una llamada; cambiar solo el mínimo conserva categoría, ubicación y otros nombres; `clear_category`/`clear_place`/`clear_min_stock` quitan; `alt_names: []` los quita; sin `alt_names` se conservan; nada que cambiar → rechazo; `clear_category` con `category_id` → rechazo; nombre que ya usa otro insumo → error con su id y nada cambia; nombre aplicado y otro nombre de otro insumo → el error dice que el nombre ya quedó y nombra al dueño; diez veces la misma clave → un evento `ingredient.updated` con el cambio; token sin alcance, rol sin `inventory.configure`, rol con configurar y sin ver el almacén (se niega nombrando ver el almacén) y token de otra empresa → rechazos sin cambio
- [x] T008 [US2] Catálogo: renglón `ingredient_update` en server/internal/domain/apitoken.go
- [x] T009 [US2] Implementar `ingredient_update` en server/internal/httpapi/mcp_tools_076.go: lectura de la ficha y su versión, `run` con PATCH y luego PUT (R2, R3), vista previa con `changes`, `maybe_exists` y `alt_names_similar`. T007 en verde

## Phase 5: User Story 3 - Empaques de un solo insumo (P2)

**Goal**: `presentations_list` con `ingredient_id` opcional.

**Independent Test**: con y sin el parámetro.

- [x] T010 [US3] Tests en server/internal/integration/mcp_076_test.go (`TestMCPPresentationsListByIngredient`), en rojo (hoy el parámetro se ignora): filtra por insumo; sin parámetro igual que antes; `ingredient_id: 0` → rechazo; insumo de otra empresa → vacío
- [x] T011 [US3] Implementar en server/internal/httpapi/mcp_tools_076.go el `readTool` de `presentations_list` con filtro y quitar su `simpleRead` de server/internal/httpapi/mcp_tools_073.go; ruta `GET /stock/ingredients/{id}/presentations` en la lista blanca; descripción del catálogo. T010 en verde

## Phase 6: Polish & Cross-Cutting

- [x] T012 [P] Lista blanca: ampliar `TestDispatcherRefusesRoutesOutsideCatalog` en server/internal/httpapi/mcp_dispatch_test.go con archivar, reactivar, unir, unidad, costo y densidad de un artículo (en rojo si alguna pasara)
- [x] T013 [P] Texto del alcance de almacén en web/src/features/admin/IntegrationsPage.tsx («Insumos y sus empaques, mermas, ajustes de existencia y conteos») con su test en web/src/features/admin/IntegrationsPage.test.tsx visto en rojo (plan, «Pantallas y maquetas», fila Integraciones › Crear token)
- [x] T014 [P] docs/mecanica/personal-y-permisos.md: qué agrega la 076 a la puerta para agentes y el ensanche de tokens con almacén
- [x] T015 `make api-build && make api-test` y `make test-integration` en verde
- [x] T016 `/revision-de-codigo` y corregir lo que encuentre con su test visto en rojo
- [x] T017 Caso de aceptación de specs/076-mcp-insumos/quickstart.md contra `make start` con el respaldo de producción restaurado y un cliente MCP real (a, b, c, d), con `whoami` del token «Token Gestion Claude»; resultado anotado aquí
  - Medido el 2026-11-11 contra `make start` del worktree con el respaldo de producción de ese día
    (03:20) restaurado, con un cliente MCP por HTTP (initialize + tools/call) y la fila del token
    «Token Gestion Claude» (secreto reemplazado solo en la base local): `whoami` lista
    `ingredient_create`, `ingredient_update` y `presentations_list` sin regenerar el token.
  - (a) `ingredient_create` «Jamon de pierna» en vista previa: `attention` primero con «Quizá ya
    existe: Jamón de pierna (id 749)». (b) alta de un insumo de prueba (id 786), su empaque (id 35,
    500 g) y `purchase_create` en vista previa con los dos: completa, costo 0 → 0.19 por g.
    (c) 749 renombrado a «Jamón» con otros nombres «Jamón de pierna» y «Jamón de pavo»; categoría,
    ubicación y costo sin cambio; `articles_search` «jamón de pavo» lo encuentra. (d)
    `presentations_list` con `ingredient_id: 749`: solo sus 4 empaques (Kir, Bafar, Member's Mark y
    la rebanada).
- [ ] T018 `make ci-local` verde; PR a `develop`, CI verde (CodeQL sin hallazgo alto nuevo) y merge; PR `develop` → `main`, CI verde, merge, deploy y `readyz` 200 en producción
- [ ] T019 Cerrar: `make cerrar-worktree datos=1`, borrar la rama remota, comentar y cerrar #271 (qué entró, qué quedó fuera y el ensanche de tokens)

## Dependencies & Execution Order

- T001 → T002 → T003 (foundational) → US1 (T004–T006) → US2 (T007–T009) → US3 (T010–T011) → Polish.
- US2 usa la pista de T003 y la búsqueda de parecidos de T006. US3 es independiente de US1/US2 y
  puede ir en paralelo si se trabaja en otro archivo; aquí comparte `mcp_tools_076.go`, así que va
  en serie.
- T012, T013 y T014 son paralelos entre sí (archivos distintos).

## Parallel Example

```text
T012 (mcp_dispatch_test.go)  |  T013 (IntegrationsPage.tsx)  |  T014 (docs/mecanica)
```

## Implementation Strategy

MVP = US1 (alta con aviso de duplicado): es lo que corta la captura de compras. US2 es el caso del
issue (749) y va en la misma subida. US3 es lectura y cierra el issue.
