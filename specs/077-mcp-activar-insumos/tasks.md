---
description: "Tareas de la 077: desactivar y reactivar insumos desde el MCP"
---

# Tasks: Desactivar y reactivar insumos desde el servidor MCP

**Input**: [spec.md](spec.md), [plan.md](plan.md), [research.md](research.md), [contracts/mcp-tools.md](contracts/mcp-tools.md), [quickstart.md](quickstart.md)

**Tests**: obligatorios (constitución IV, TDD): cada test se escribe y se ve en rojo por la razón
correcta antes del código que lo pone en verde. Lo que ya cumple se fija con prueba que pasa al
escribirla, y se dice. Integración con `//go:build integration` (`make test-integration`).

## Phase 1: Setup

- [x] T001 Confirmar que no hace falta migración ni alcance nuevo (0113 libre en `origin/develop` y ramas remotas) en specs/077-mcp-activar-insumos/plan.md (Technical Context; ya anotado el 2026-11-12)

## Phase 2: Foundational (qué hace hoy un insumo archivado — FR-008)

**Purpose**: verificar con pruebas, antes de escribir código, el comportamiento del archivado; lo que
ya cumple queda fijado, lo que no se arregla en la consulta.

- [x] T002 Pruebas en server/internal/integration/archived_ingredient_test.go (`TestArchivedIngredientLeavesCountPurchasesAndShortages`), por la ruta y bajo `gatobobah_app`: tras archivar por `POST /stock/articles/{id}/archive`, el insumo no sale en el conteo (lista y «sin contar»), ni en `GET /articles?q=`, ni en `GET /articles/suggest` por nombre parecido; sus movimientos (`/stock/movements`), historial de costo y bitácora del artículo siguen legibles. Esperado: verde (ya cumple); anotar el resultado aquí
  - 2026-11-12: conteo (lista y «sin contar»), buscador `/articles` e historial (movimientos, costo,
    bitácora) en verde al escribirlos: ya cumplían. La sugerencia por nombre sin proveedor salía en
    rojo, pero por el paso 2 (lo aprendido de otros proveedores), el mismo hueco de T003
- [x] T003 Pruebas en el mismo archivo, en rojo hoy: (a) `GET /articles/suggest` con proveedor y renglón aprendido hacia el insumo archivado ya no lo propone como «aprendido» y, con un activo parecido, propone el activo como sugerencia (A5); (b) aprendido de otro proveedor hacia el archivado no sale; (c) `GET /stock/levels` no lista el insumo archivado, sí un producto archivado con control de existencias marcado `is_active: false` (A4), y cada renglón trae `is_active`; (d) reactivar lo devuelve a todo lo anterior
  - 2026-11-12: en rojo (a) «aprendido» hacia el archivado, (b) otro proveedor, (c) existencias lo
    listaba y sin `is_active`; T004 pasó al escribirla (guarda)
- [x] T004 Aislamiento de las tres consultas cambiadas con `appRoleStore` e `inTheThreeCases` en server/internal/integration/archived_ingredient_test.go (`ListStockLevels`, `LookupSupplierItem`, `SuggestFromSupplierItems`)
- [x] T005 Arreglo en server/queries/supplier_items.sql (`LookupSupplierItem`, `SuggestFromSupplierItems`: `and (si.ingredient_id is null or i.is_active)`) y server/queries/stock.sql (`ListStockLevels`: omite insumos archivados y trae `coalesce(i.is_active, p.is_active, true) as is_active`); `make sqlc` y confirmar `IsActive bool` en server/internal/store/db/stock.sql.go. T003 y T004 en verde
- [x] T006 [P] `StockLevel.is_active?` en web/src/api/backoffice.ts y texto de la tabla vacía «Sin existencias que mostrar» en web/src/features/backoffice/StockPage.tsx, con su test en web/src/features/backoffice/StockPage.test.tsx visto en rojo (plan, «Maquetas», fila Almacén › Existencias; A1)

## Phase 3: User Story 1 - Desactivar un insumo descontinuado (P1)

**Goal**: `ingredient_update` con `active: false` y `confirm_on_hand`, con la revisión previa de la hoja.

**Independent Test**: vista previa de uno con existencia y de uno con receta activa; desactivar uno sin nada.

- [x] T007 [US1] Unitarios en server/internal/domain/apitoken_test.go, en rojo: pista de `ARCHIVE_HAS_STOCK` menciona merma, ajuste y `confirm_on_hand`; la de `IN_USE` dice que se quita en la pantalla y que no cambia recetas; `ToolErrorHintWith` con archivado dice «se reactiva con ingredient_update y active: true» y ya no «en la pantalla»
- [x] T008 [US1] Implementar las pistas en server/internal/domain/apitoken.go y la descripción de `ingredient_update` (active, confirm_on_hand). T007 en verde
- [x] T009 [US1] Tests en server/internal/integration/mcp_077_test.go (`TestMCPIngredientDeactivate`), en rojo (parámetro desconocido): vista previa con existencia positiva y con negativa → `attention` primero con cantidad y sucursal, y nada cambia; aplicar sin `confirm_on_hand` → 409 con la pista y sigue activo; con `confirm_on_hand: true` → inactivo con la existencia intacta; receta activa → la vista previa la nombra con tipo e id, aplicar rechaza y receta y producto sin cambio; conteo abierto que lo capturó → aviso y rechazo; compra por recibir → nota; sin nada → diez veces la misma clave dejan un solo `ingredient.archived` y `mcp.tool_called` con antes y después con `isActive`; ya inactivo → la vista previa lo dice y aplicar no cambia nada; `active` con `name` (y con `alt_names`) y `confirm_on_hand` sin `active: false` → rechazo de argumentos sin despachar; token sin `write:inventory` → se niega nombrando el alcance; rol sin `inventory.configure` (con ver y recibir) → no se ofrece, 403 y sigue activo; token de otra empresa → «no existe» y nada cambia; rol con configurar y sin ver el almacén → la vista previa dice que no pudo revisar existencia ni recetas
  - 2026-11-12: en rojo (parámetro `active` desconocido). Sin el alcance, la herramienta no se registra para
    el token (072: «unknown tool»), no hay mensaje que nombre el alcance: la prueba comprueba que no se
    ofrece y que nada cambia; el spec se precisó
- [x] T010 [US1] Implementar en server/internal/httpapi/mcp_tools_076.go: argumentos `active` y `confirm_on_hand`, `check()` (va solo; confirm solo con false), `route` al POST de archivar con `{confirmOnHand}`, lista blanca (`…/archive`, `…/reactivate`, `GET …/archive-check`), vista previa con la ficha y la revisión (`attention` en el orden del contrato), `isActive` en la vista de la ficha. T009 en verde
- [x] T011 [US1] Ajustar `TestDispatcherRefusesRoutesOutsideCatalog` en server/internal/httpapi/mcp_dispatch_test.go: `ingredient_create` no despacha archivar ni reactivar; `ingredient_update` sigue sin despachar unir, unidad, costo ni densidad (en rojo si alguna pasara)

## Phase 4: User Story 2 - Reactivar un insumo (P1)

**Goal**: `active: true` sobre la ruta de reactivar.

**Independent Test**: reactivar el insumo desactivado y verlo de nuevo en conteo, compras y existencias.

- [x] T012 [US2] Tests en server/internal/integration/mcp_077_test.go (`TestMCPIngredientReactivate`), en rojo: vista previa dice que vuelve tal como estaba con su existencia; aplicar → activo, `ingredient.reactivated`, y vuelve a salir en `stock_count_current`, `articles_search` y `stock_levels`; nombre tomado por otro activo → 409 con el id del otro y sigue inactivo; insumo unido a otro → rechazo con el nombre del destino; ya activo → lo dice y no cambia nada; `confirm_on_hand` con `active: true` → rechazo
- [x] T013 [US2] Implementar en server/internal/httpapi/mcp_tools_076.go la rama `active: true` (ruta de reactivar y su vista previa). T012 en verde

## Phase 5: User Story 3 - Saber si un insumo está activo (P2)

**Goal**: `articles_search` con `active` e `include_inactive`; `stock_levels` con `is_active`.

**Independent Test**: comparar las dos lecturas antes y después de desactivar.

- [x] T014 [US3] Tests en server/internal/integration/mcp_077_test.go (`TestMCPArticlesSearchAndStockLevelsSayActive`), en rojo: `articles_search` sin el parámetro → los mismos renglones que la ruta, cada uno con `active: true`; con `include_inactive: true` → agrega el archivado (por nombre y por otro nombre) con `active: false`, sin repetir id; rol que captura compras y no ve el almacén → nota de permiso en vez de callarlo; `stock_levels` → `is_active` en cada renglón y sin el archivado
- [x] T015 [US3] Implementar `articles_search` como `readTool` en server/internal/httpapi/mcp_tools_076.go (quitar su `simpleRead` de server/internal/httpapi/mcp_tools_073.go; `GET /stock/articles` en su lista blanca) y su descripción en server/internal/domain/apitoken.go. `stock_levels` no cambia de código (pasa el campo). T014 en verde

## Phase 6: Polish & Cross-Cutting

- [x] T016 [P] docs/mecanica/personal-y-permisos.md (qué agrega la 077 a la puerta para agentes y el ensanche de tokens con almacén) y docs/mecanica/almacen.md (archivado: sugerencias aprendidas, existencias sin archivados, productos archivados siguen, existencia confirmada que deja de verse en Existencias pero sigue en Archivados)
- [x] T017 `make api-build && make api-test`, `make test-integration` y `cd web && bun run lint && bun run vitest run` en verde
  - 2026-11-12: verdes. `EmployeesPage.test.tsx` › «al editar a alguien que cabe…» falló tres veces
    seguidas (no aparece el `dialog` del Picker) y después pasó sin cambios; falla igual sobre
    `origin/develop` limpio con las mismas dependencias: intermitencia previa, ajena a la 077
- [x] T018 `/revision-de-codigo` y corregir lo que encuentre con su test visto en rojo
  - 2026-11-12: go-backend-reviewer y security-auditor sin bloqueantes; ajustes C1–C7 en plan.md (C1 visto en rojo; C3 guarda)
- [x] T019 Caso de aceptación de specs/077-mcp-activar-insumos/quickstart.md contra `make start` con el respaldo de producción restaurado (`backups/prod/pre-077-20261112-0340.dump`) y un cliente MCP real (a–e); resultado anotado aquí
  - Medido el 2026-11-12 contra `make start` del worktree con el respaldo de producción de ese día
    (03:40) restaurado, cliente MCP por HTTP y la fila del token de la empresa (secreto reemplazado
    solo en la base local): `whoami` lo atiende sin regenerarlo.
  - (a) 727, con 2000 ml: `attention` primero con la existencia y merma o ajuste; aplicar sin
    confirmar → rechazo con la pista de `confirm_on_hand`. (b) 758: la vista previa nombra «Kokomen -
    Paldo (producto, id 764)»; aplicar → rechazo. El 436 resultó usado por 5 extras activos: la
    vista previa los nombró a todos y aplicar se rechazó.
  - (c) 727 llevado a 0 con `stock_adjust` (el flujo que pide la vista previa) y desactivado: antes
    salía en búsqueda, existencias, conteo y candidatos de compra; desactivado, en ninguno; su ajuste
    sigue en `stock_movements` y la bitácora tiene `ingredient.archived`. `articles_search` con
    `include_inactive` lo trae `active: false`.
  - (d) `active: true`: la vista previa dice que vuelve con 0 ml y cuándo se rechaza; aplicado,
    vuelve a salir en los cuatro lugares.
  - (e) token del administrador sin `write:inventory` y token con todos los alcances de un rol sin
    `inventory.configure`: ninguno ve `ingredient_update` (llamarla es «unknown tool») y el 727 sigue
    activo. Los dos tokens se borraron de la base local.
- [x] T020 `make ci-local` verde; PR a `develop`, CI verde (CodeQL sin hallazgo alto nuevo) y merge; PR `develop` → `main`, CI verde, merge, deploy y `readyz` 200 en producción
  - 2026-11-12: `make ci-local` verde; PR #278 con CI verde (CodeQL sin alertas altas ni críticas) y
    merge. El primer CI falló en `TestAlertEmailCarriesNoSecrets`, ajena a la 077: el PIN de prueba
    «4821» puede salir por azar en el id de la petición o en la hora del correo (0 fallas en 3000
    corridas locales; la causa es inferida, no reproducida); se cambió por uno largo. #278 entró
    detrás del #279 de otra sesión (solo `web/` de tickets) sin traer develop antes: el árbol
    combinado lo probó completo el CI del release. Release #280 con CI verde, respaldo previo
    (`gatobobah-20261112-0427.dump`), deploy en verde, `healthz` y `readyz` 200, sin 5xx en el
    journal; un `shutdown: context deadline exceeded` de la versión anterior, igual al de la subida
    de la 076.
- [x] T021 Cerrar: `make cerrar-worktree datos=1`, borrar la rama remota, comentar y cerrar #277 (qué entró, qué quedó fuera y el ensanche de tokens)
  - 2026-11-12: #277 comentado (qué entró, el ensanche de tokens con Almacén y lo que quedó fuera) y
    cerrado; rama remota borrada; worktree `egb-077-mcp-activar-insumos` cerrado con sus volúmenes
    después de mergear esta nota.

## Dependencies & Execution Order

- T001 → Foundational (T002 → T003/T004 → T005; T006 en paralelo) → US1 (T007 → T008; T009 → T010 → T011) → US2 (T012 → T013) → US3 (T014 → T015) → Polish.
- US2 usa la lista blanca y los argumentos de T010. US3 depende de T005 para `stock_levels`.
- US1–US3 comparten `mcp_tools_076.go` y `mcp_077_test.go`: van en serie.

## Parallel Example

```text
T006 (web)  |  T007/T008 (domain)  |  T016 (docs)
```

## Implementation Strategy

MVP = Foundational + US1 (desactivar con su revisión previa), que es el problema del issue. US2 y US3
van en la misma subida: reactivar es la decisión del issue y la búsqueda con inactivos es lo que lo
hace posible.
