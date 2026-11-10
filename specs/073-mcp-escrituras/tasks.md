---
description: "Tareas de las escrituras del servidor MCP"
---

# Tasks: Escrituras del servidor MCP

**Input**: `specs/073-mcp-escrituras/` — plan.md, spec.md, research.md, data-model.md, contracts/, quickstart.md

**Tests**: obligatorios (constitución IV: TDD; V: ningún control de seguridad sin su test). Cada
tarea de test va **antes** de su implementación y se ve en rojo por la razón correcta.

**Rutas**: backend en `server/`, front en `web/src/`. Herramientas nuevas en
`server/internal/httpapi/mcp_tools_073.go` (archivo nuevo, para no inflar `mcp_tools_write.go`).

## Phase 1: Setup

- [X] T001 Confirmar que ninguna rama remota abierta usa la migración 0112 (`git ls-tree` sobre `origin/*`): verificado el 2026-11-10, ninguna la usa

---

## Phase 2: Foundational (bloquea todas las historias)

### Alcances y catálogo (dominio)

- [X] T002 Tests en `server/internal/domain/apitoken_test.go`: `AllAPIScopes` incluye `write:purchases` y `write:orders` en su orden canónico; `Normalize` los acepta; `TestMCPToolCatalog` cubre las 16 herramientas nuevas (escrituras con su alcance `write:*`, lecturas `read`, nombres únicos snake_case) y su permiso según contracts/mcp-tools.md; `ToolErrorHintFor` con los códigos nuevos (`OPENING_REASON_REQUIRED`, `TIPS_DECISION_REQUIRED`, `REFUND_FOLIO_REQUIRED`, `TERMINAL_COUNT_REQUIRED`, `CLOCK_IN_REQUIRED`, `OPENER_NOT_CLOCKED_OUT`, `NEEDS_CONFIRM`, `INGREDIENT_MERGED`, `OPEN_ORDERS`, `UNPAID_ORDERS`) devuelve una corrección en palabras que nombra el argumento a mandar
- [X] T003 Implementar en `server/internal/domain/apitoken.go`: alcances, entradas del catálogo y `toolFixByCode` hasta que T002 pase
- [X] T004 [P] Tests table-driven en `server/internal/domain/mcp_writes_test.go`: `MissingCloseDeclarations` (cajón que pide conteo sin `counts` ni `counted_cash` → lo nombra; método no autodeclarado y fuera del cajón sin cifra → lo nombra; `"0.00"` presente cuenta como declarado; métodos del cajón y autodeclarados no se piden; orden del turno) y `CancelSummary` (sin cobros → no pide devolución; con cobros → total por método; con tarjeta → pide folio)
- [X] T005 Implementar `server/internal/domain/mcp_writes.go` hasta que T004 pase

### Migración 0112

- [X] T006 Test `server/internal/integration/migration_0112_test.go` contra base restaurada con dos empresas: antes de la 0112 un token con `write:purchases` viola el check; después lo acepta con hasta 7 alcances y sigue rechazando uno desconocido y uno sin `read`; el Down se niega con un token que usa un alcance nuevo y, sin él, restaura la lista de cinco y la cardinalidad 1–5; renglón de la 0112 en `guardedDownCleanup` de `server/internal/integration/harness_restaurado_test.go` (`array_remove` de los dos alcances, A5)
- [X] T007 Escribir `server/migrations/0112_api_token_write_scopes.sql` (Up/Down) hasta que T006 pase; aplicarla a la base local

### Despachador

- [X] T008 Tests en `server/internal/httpapi/mcp_dispatch_test.go`: una ruta con dos parámetros enteros (`/stock/counts/{id}/lines/{ingredientId}`) se despacha; con un parámetro faltante se rechaza; las rutas que siguen fuera (`/orders/{id}/pay`, `/orders/{id}/refund`, `/cash-sessions/transfer`, `/cash-sessions/tips/payouts`, `/expenses/{id}/edit`) se rechazan aunque otra herramienta nueva las rodee
- [X] T009 Ampliar `TestEveryToolMatchesItsRouteGate` en `server/internal/integration/mcp_test.go` para reemplazar todos los parámetros de ruta (`{id}`, `{ingredientId}`, `{productId}`) y cubrir las herramientas nuevas; agregar las rutas nuevas a `mcpToolRoutes` en `server/internal/httpapi/mcp_tools.go` y el golden `server/internal/httpapi/testdata/route_gates.golden` si cambia
- [X] T010 Soporte de escritura con varios despachos en `writeSpec` (`server/internal/httpapi/mcp_tools.go`): un `run` opcional que despacha en orden, se detiene al primer fallo y devuelve lo aplicado y lo fallido; un fallo 4xx libera la clave (capturar fija la cifra: repetir no duplica), 5xx la deja desconocida

- [X] T010a Dinero y cantidades como texto (FR-018): en `TestMCPMoneyIsText` (`server/internal/integration/mcp_test.go`) un número en `counted_cash`, `opening_cash`, `float_left`, en un monto de `declared`/`terminal_counts` o en `counted_qty` se rechaza antes de llegar a la herramienta; ampliar `moneyFields`/`quantityFields` en `server/internal/httpapi/mcp_tools.go` y aplicar el patrón también a los valores de los mapas (`additionalProperties`)

**Checkpoint**: alcances, catálogo, migración y despachador listos.

---

## Phase 3: User Story 1 — Capturar una compra con su mercancía (P1) 🎯 MVP

**Goal**: `purchase_create`, `purchase_receive` y sus lecturas de apoyo.

**Independent Test**: token con `write:purchases` de una persona con `expenses.capture`; compra de dos renglones en efectivo; existencias, costo vigente, salida de caja y bitácora.

### Vista previa de costo (ruta nueva)

- [X] T011 [US1] Tests de integración en `server/internal/integration/purchase_preview_test.go` bajo `appRoleStore`: `TestPurchasePreviewLeavesNothingBehind` (ni gasto, renglón, movimiento, costo, evento, aprendizaje de proveedor ni salida de caja quedan; `costs` por insumo con dos renglones del mismo insumo y promedio ponderado; «último precio» con una compra más reciente → `keptByNewerPurchase`), insumo unido falla igual en vista previa y al guardar (A1), `newArticle` → 400 (A2), `lines`/`costs` arreglos vacíos con una compra sin mercancía, `TestPurchasePreviewIsolation` con `inTheThreeCases` (insumo de otra empresa = 404)
- [X] T012 [US1] Extraer `createExpenseTx` en `server/internal/app/expenses.go` (validación previa incluida: `openShiftDay`, `validatePayments`, `checkPaymentsCover`) sin cambiar `CreateExpense`; `PreviewPurchase` corre lo mismo con `set local lock_timeout = '2s'`, lee costos antes y después dentro de la transacción, fuerza `set constraints all immediate` y revierte con `errPreviewRollback` (A1, A4); los tests de gastos existentes siguen verdes
- [X] T013 [US1] Handler fino `PurchasePreview` en `server/internal/httpapi/handlers_backoffice.go` y ruta `POST /expenses/purchase-preview` en `server/internal/httpapi/router.go` con `expenses.capture` y `rateLimitUser`

### Herramientas

- [X] T014 [US1] Tests de integración en `server/internal/integration/mcp_writes_test.go` (`TestMCPPurchases`): vista previa de renglones solo con nombre trae candidatos y no guarda nada; con insumo y empaque trae `cost`; aplicar con un renglón sin insumo o sin empaque/unidad se rechaza nombrando el renglón (`TestMCPPurchaseNeedsExplicitIds`); misma clave diez veces = una compra y el almacén sube una vez; efectivo deja la salida en el turno; `device.agent = "mcp"` en el evento; `purchase_receive` de una compra pendiente suma lo que llegó; token sin `write:purchases` y persona sin `inventory.receive` (para recibir) rechazados; otra empresa no ve la compra
- [X] T015 [US1] Implementar en `server/internal/httpapi/mcp_tools_073.go`: lecturas `articles_search`, `units_list`, `presentations_list`, `pending_receipts_list`; escrituras `purchase_create` (cuerpo de `POST /expenses` con `items`, `receivedAt` = `received_on` u hoy salvo `pending`; vista previa con `purchase-preview`, `articles/suggest` por renglón sin artículo, `stock/presentations` suave) y `purchase_receive`

**Checkpoint**: US1 completa y verificable sola.

---

## Phase 4: User Story 2 — Abrir y cerrar caja (P1)

**Goal**: `cash_session_open`, `cash_session_close`, `cash_denominations_list`, `cash_session_attendance`, con el arqueo ciego intacto.

**Independent Test**: arqueo ciego encendido; abrir por MCP, cobrar en la pantalla, cerrar por MCP.

- [X] T016 [US2] Tests de integración en `server/internal/integration/mcp_writes_test.go`: `TestMCPCashCloseKeepsTheBlindCount` (con el ajuste encendido, ninguna respuesta de `cash_session_current`, vista previa de `cash_session_close` y de `cash_movement_create` trae `expected`, `difference` ni ingresos no nulos; apagado, `cash_session_current` sí trae el esperado), `TestMCPCloseRejectsAMissingFigure` (método sin cifra → error que lo nombra y el turno sigue abierto), cierre completo deja el mismo corte que la ruta con el mismo cuerpo, respuesta aplicada trae la diferencia, `TestMCPCashOpenNeedsClockIn` (persona que lleva asistencia sin entrada → causa de la pantalla), quien abrió sin salida → rechazo, con cierre autorizado y permiso → cierra, `OPENING_REASON_REQUIRED` llega con su corrección, dos cierres a la vez → uno cierra, abrir y cerrar con la misma clave diez veces → un solo turno (SC-004), el turno queda con `opened_by`/`closed_by` = la persona del token (FR-015), otra empresa no ve la caja
- [X] T017 [US2] Implementar en `server/internal/httpapi/mcp_tools_073.go`: `cash_denominations_list`, `cash_session_attendance`, `cash_session_open` y `cash_session_close` (antes de despachar, `domain.MissingCloseDeclarations` sobre `GET /cash-sessions/current`; la vista previa solo lee `current`, `attendance`, `tips` y devuelve `to_declare`)

---

## Phase 5: User Story 3 — Contar el almacén (P2)

**Goal**: `stock_count_current`, `stock_count_open`, `stock_count_capture` (lote), `stock_count_close`, `stock_count_discard`.

- [X] T018 [US3] Tests en `server/internal/integration/mcp_writes_test.go`: abrir; con uno abierto, 409 que dice cuál; `TestMCPCountCaptureBatch` (tres renglones, uno de producto con existencia propia; ninguna respuesta trae lo que el sistema tiene; un renglón fuera de lo razonable detiene el lote y pide `confirmed: true` sin la referencia; reintento con la misma clave no duplica; más de 50 → rechazo); vista previa del cierre trae sistema, contado y ajuste por artículo y lo no contado aparte; cerrar deja los mismos ajustes que la ruta; descartar no toca el almacén; otra empresa no ve el conteo
- [X] T019 [US3] Implementar las herramientas de conteo en `server/internal/httpapi/mcp_tools_073.go` (captura con `confirmable: true` y `run` de T010)

---

## Phase 6: User Story 4 — Cancelar un pedido (P3)

- [X] T020 [US4] Tests en `server/internal/integration/mcp_writes_test.go` (`TestMCPOrderCancel`): sin cobros se cancela y deja el evento con el token; cobrado sin `refund: true` → causa de la pantalla; con tarjeta sin folio → pide folio; en efectivo con `refund: true` → el corte y el almacén quedan como la misma cancelación por la ruta; vista previa con `CancelSummary`; la misma clave diez veces → una sola cancelación y una sola devolución (SC-004); el evento lleva como actor a la persona del token (FR-015); sin `write:orders` o sin `orders.cancel` → rechazo; pedido de otra empresa → «no existe»
- [X] T021 [US4] Implementar `order_cancel` en `server/internal/httpapi/mcp_tools_073.go` (vista previa con `GET /orders/{id}` y `domain.CancelSummary`)

---

## Phase 7: Pantalla de Integraciones (FR-001, FR-002; plan › Maquetas, opción A)

- [X] T022 [P] Test en `web/src/features/admin/IntegrationsPage.test.tsx`: la hoja ofrece seis alcances de escritura con los textos del plan (Caja incluye abrir y cerrar; Almacén, conteos; Compras; Pedidos con devolución); un token con `write:purchases`/`write:orders` muestra «Compras»/«Pedidos» en su tarjeta
- [X] T023 Implementar en `web/src/api/integrations.ts` (`TokenScope`) y `web/src/features/admin/IntegrationsPage.tsx` (`WRITE_SCOPES`, `LABEL`) la opción A de la fila «Integraciones › Crear token» de Maquetas

---

## Phase 8: Polish

- [X] T024 [P] `TestMCPToolsListFollowsScopeAndPermission` en `server/internal/integration/mcp_test.go` ampliado: un token con `write:inventory` ve las herramientas de conteo y uno con `write:cash` las de abrir y cerrar (consecuencia de D1, explícita)
- [X] T025 [P] Contrato `specs/072-servidor-mcp/contracts/mcp-tools.md` intacto; `docs/mecanica/personal-y-permisos.md`: qué agrega la 073 (alcances nuevos, arqueo ciego por MCP, lote de conteo, vista previa revertida de compras)
- [X] T026 `cd server && go build ./... && go test ./...`, `make test-integration`, `cd web && bun run lint && bun run vitest run && bun run build`
- [X] T027 Prueba con un cliente MCP real contra la API local (quickstart.md); lo observado se escribe aquí, sin cifras del negocio; se cierra lo que la prueba dejó abierto

  **Observado el 2026-11-10** (cliente del SDK oficial de MCP por HTTP contra `make start` del
  worktree, base restaurada del respaldo de esa madrugada, arqueo ciego encendido):
  - Un token con los seis alcances de escritura ve 45 herramientas (las 49 menos las 4 de catálogo).
  - Compra: con solo el nombre del ticket, la vista previa propuso el insumo correcto como candidato
    y avisó que faltaba insumo y unidad; con ids, dijo cuánto entra en gramos, el costo por gramo de
    la compra y el costo vigente antes y después. Aplicar sin ids se rechazó nombrando el renglón;
    aplicar dos veces con la misma clave dejó una compra, recibida y pagada.
  - Conteo: abrir, capturar dos renglones en lote, vista previa del cierre con sistema/contado/ajuste
    y cerrar. **Defecto encontrado aquí y corregido con su test**: el antes y el después de cada
    escritura de conteo traían la lista completa de artículos (cientos, del orden de 100 KB por
    llamada); ahora va un resumen (`TestCountSummaryIsSmall`) y lo no contado de la vista previa y del
    cierre va como número (`TestClosePreviewKeepsTheAdjustmentsAndCountsTheRest`).
  - Caja: la lectura del turno y la vista previa del cierre no trajeron ninguna cifra esperada; abrir
    sin la entrada de quien abre y cerrar sin su salida se rechazaron con la causa de la pantalla y la
    corrección en palabras; con la entrada y la salida marcadas con PIN por la ruta de la tableta, abrió
    y cerró; repetir el cierre con la misma clave devolvió lo guardado; la diferencia llegó después de
    cerrar.
  - Cancelar: un pedido cobrado y no entregado se canceló con `refund: true` y la devolución quedó una
    vez. Un pedido entregado y cobrado se rechaza como en la pantalla (es una devolución); **ajuste**:
    la vista previa ahora lo avisa antes de aplicar, porque el rechazo no trae un código del que
    colgar la corrección.
  - Lo que la prueba dejó: los pedidos de prueba quedaron cerrados (entregados y cobrados, o
    cancelados) y la caja cerrada; todo en la base local.
- [ ] T028 `/revision-de-codigo` y corregir cada hallazgo con su test

## Dependencies

- Phase 2 bloquea todo. US1–US4 son independientes entre sí después de Phase 2 (comparten `mcp_tools_073.go`: se implementan en orden para no chocar en el archivo). Phase 7 depende solo de T003.

## Parallel examples

- T004 con T002; T006 con T008; T022 con cualquier historia.

## Implementation strategy

MVP = Phase 2 + US1 (el motivo del issue). Después US2 (diario, regla delicada), US3, US4, pantalla y pulido. Todo sube junto en un solo PR.
