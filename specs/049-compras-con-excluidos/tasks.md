# Tasks: Renglones excluidos y caja que vende

**Input**: [spec.md](spec.md), [plan.md](plan.md) (R1–R11 prevalecen), [data-model.md](data-model.md), [contracts/api.md](contracts/api.md)
**Tests**: antes que el código (constitución IV). Rutas relativas a `/home/ramy/git/egb-049-compras-con-excluidos`.

## Phase 1: Setup
- [ ] T001 Confirmar el número de migración libre contra `origin/develop` y la rama de la 048 (0100 la toma la 048: esta usa 0101)
- [ ] T002 Clasificar cada `is_primary` de server/queries/ y server/internal/app/ en research.md (R1)

## Phase 2: Foundational
- [ ] T003 Test de migración en server/internal/integration/migracion_excluidos_test.go con dos empresas: columnas con default, check de `document_total`, Down se niega con un excluido o con un `document_total` (R7)
- [ ] T004 Migración server/migrations/0101_excluded_items_and_document_total.sql (R7)
- [ ] T005 Test de dominio (tabla) de `PlanStockLines` con excluidos (objetivo 0, sin pendiente, sale con corrección si ya entró) y de `CanMoveSales` en server/internal/domain/
- [ ] T006 Dominio: `PlanStockLines`, `CanMoveSales`, sentinels, acciones de bitácora; códigos en server/internal/httpapi/respond.go
- [ ] T006b Test de integración en server/internal/integration/caja_que_vende_test.go (verlo fallar): A vende y cierra, se mueve `is_primary` a B como owner, y el corte de A, sus reportes y `CerroLaCajaPrincipal` no cambian (R1)
- [ ] T007 Consultas (server/queries/*.sql) con `excluded` según R2, R3, R5; `MoveSales` según R6; R1 en `CerroLaCajaPrincipal`; `make sqlc`

## Phase 3: US1 — Excluir renglones (P1)
- [ ] T008 [US1] Integración en server/internal/integration/excluidos_test.go: crear con excluidos (existencias y costo solo de incluidos; la regla de la 038 ignora excluidos), compra con excluido no queda pendiente, «recibir todo» no lo mete, historial de precios sin él, `SaveStockLines` conserva la marca, `PATCH` excluir/incluir con corrección, regla de la 038 al incluir (igual pasa, +1 centavo 400), dos PATCH opuestos (0 o N), excluir todos los renglones deja el gasto con su total como «otros», 403 sin permiso, bajo `appRoleStore` con `inTheThreeCases`
- [ ] T009 [US1] App: `insertItems` guarda excluidos (y lo «personal»), `checkLinesFitTotal` con incluidos en sus tres lugares, `SetItemExcluded` (R4), `PATCH /expenses/{id}/items/{itemId}`
- [ ] T010a [P] [US1] Tests front (verlos fallar): «Excluir» por renglón conserva el artículo, atenúa, no suma; lo detectado como personal llega marcado; un ticket sin excluidos no agrega pasos (SC-003); detalle con «Incluir»/«Excluir»
- [ ] T010 [US1] Front según R9 y R10 en web/src/features/backoffice/ExpenseDialog.tsx y ExpenseDetailDialog.tsx; web/src/api/backoffice.ts

## Phase 4: US2 — Total del ticket (P2)
- [ ] T011 [US2] Integración: `documentTotal` se guarda y sale en el detalle; ausente queda nulo; negativo, NaN o fuera de `MaxMoney` → 400 (se valida con `ValidMoney`/`Round2`)
- [ ] T012 [US2] App/handler: `documentTotal` en la creación y el detalle
- [ ] T013a [P] [US2] Test front: el documento leído manda su total; el detalle lo muestra solo si existe
- [ ] T013 [US2] Front

## Phase 5: US3 — Caja que vende (P3)
- [ ] T014 [US3] Integración: mover con las dos cerradas; con una abierta 409; otra sucursal 400; destino inactivo 400; ya vende → no-op; nunca dos ni ninguna; 403 sin permiso; concurrencia contra abrir turno; el historial de A no cambia tras mover (R1)
- [ ] T015 [US3] `MoveSales` y `POST /cash-registers/{id}/sells`
- [ ] T016a [P] [US3] Test front de la columna «Vende» (R11)
- [ ] T016 [US3] Front en web/src/features/backoffice/CashPage.tsx

## Phase 6: Polish
- [ ] T017 AGENTS.md: excluidos y mover la caja que vende
- [ ] T018 Gates y `make ci-local`
- [ ] T019 Quickstart a 1024×600 y prueba con usuario nuevo
