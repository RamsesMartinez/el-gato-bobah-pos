# Tasks: Sucursales en la base de datos

**Input**: [plan.md](plan.md), [spec.md](spec.md), [data-model.md](data-model.md), [research.md](research.md), [quickstart.md](quickstart.md)

**Tests**: obligatorios (constitución IV, TDD). Cada test se ve fallar antes del código que lo
pone en verde. Los de aislamiento van bajo `appRoleStore` con `inTheThreeCases`; los de migración,
sobre `restoredStore` (respaldo con dos empresas).

## Phase 1: Setup

- [x] T001 Confirmar que 0076 es el siguiente número libre en esta rama y en las ramas vivas (`git ls-tree` de develop, 021, 022, 024) y fijar `server/migrations/version.go` a 76 junto con `server/migrations/version_test.go`

## Phase 2: Foundational (bloquea todo)

**Tests primero**

- [x] T002 [P] Test de dominio para el código derivado del slug (`gatobobah`→`GATOBOBAH`, `bobah-pruebas`→`BOBAHPRUEBA`, corte a 10, solo `[A-Z0-9]`) y el sentinel `ErrBranchAmbiguous` en server/internal/domain/branch_test.go
- [x] T003 Test de migración sobre `restoredStore` en server/internal/integration/branches_migration_test.go: una matriz por empresa con número 1; cero filas sin sucursal en `cash_registers`, `platform_connections`, `orders` (incluidos los pedidos sin turno), `stock_movements`, `stock_levels`; dirección y teléfono copiados de `business_settings`; ventas y corte de un día idénticos antes y después (SC-005); Down que se niega si hay dos sucursales
- [x] T004 Test de `current_branch_id()` en los tres casos (otra empresa, conexión reciclada con `app.company_id=''`, sin empresa) bajo `appRoleStoreWithOneConn` en server/internal/integration/branches_test.go: con una sucursal devuelve su id; con cero o dos lanza `branch_ambiguous`, nunca 22P02

**Implementación**

- [x] T005 Implementar `ErrBranchAmbiguous` y `HeadquartersCode(slug)` en server/internal/domain/branch.go
- [x] T006 Escribir server/migrations/0076_branches.sql, Up: tabla `branches` (columnas, únicos, índice parcial de matriz, `branches_tenant_key`, RLS con `nullif`, grants sin `delete`); función `current_branch_id()` con `nullif`; triggers `before insert` (número consecutivo con `for update` sobre la empresa) y `before update` (código inmutable, última activa); relleno de matrices desde `companies` con dirección y teléfono de `business_settings`
- [x] T007 En la misma 0076: columna `branch_id` en `cash_registers`, `platform_connections`, `orders`, `stock_movements`, `stock_levels` (relleno con la matriz directa, luego `not null`) y en `expenses` (nullable); `default current_branch_id()` en `cash_registers`, `platform_connections`, `stock_movements`; FK compuestas `(company_id, branch_id)`; `cash_registers_one_primary` a `(company_id, branch_id)`; comprobación final que aborta si queda una fila sin sucursal
- [x] T008 En la misma 0076: únicos de `stock_levels` a `(branch_id, ingredient_id)` y `(branch_id, product_id)`, y el trigger de existencias de server/migrations/0009_triggers.sql reescrito con `branch_id` en el insert **y** en el `on conflict`; revisar que el camino de devoluciones de 0060 siga pasando por él
- [x] T009 Down de 0076: se niega con `raise exception` si alguna empresa tiene más de una sucursal; si no, deshace en orden inverso y restaura el único por empresa y el trigger viejo
- [x] T010 `CreateCompany` crea la matriz en la misma consulta (CTE) en server/queries/companies.sql; `make sqlc`; actualizar `makeCompany` en server/internal/integration/harness_test.go y `docs/corte-produccion/01_nueva_empresa.sql` (agregar `branches` a su lista de tablas)
- [x] T011 Mapear `ErrBranchAmbiguous` (y el error `branch_ambiguous` de Postgres traducido en `app`) a una respuesta clara en server/internal/httpapi/respond.go
- [x] T012 Pasar `branch_id` explícito en los inserts como owner con otra empresa: server/internal/integration/fecha_y_folio_separados_test.go:367, migracion_conteo_de_efectivo_test.go:50 (`turnoDe`), menus_plataforma_test.go:195 y :264, pedidos_de_plataforma_test.go:117

**Checkpoint**: `go build ./... && go test ./...` en verde con una sucursal por empresa.

## Phase 3: US1 — Toda empresa tiene al menos una sucursal (P1)

**Independent test**: crear una empresa y ver su matriz; migrar un respaldo con dos empresas y ver una matriz cada una.

- [x] T013 [P] [US1] Test: `CreateCompany` deja exactamente una matriz con número 1 y código del slug; no se puede desactivar la última activa; no se puede cambiar el código; no se puede borrar una sucursal (falta de grant) en server/internal/integration/branches_test.go
- [x] T014 [P] [US1] Test: `TestEveryCompanyTableIsIsolated` incluye `branches` sin cambios al test (server/internal/integration/rls_all_tables_test.go) y `branches` aislada en los tres casos
- [x] T015 [US1] Ajustar lo que haga falta en 0076 o en `CreateCompany` hasta que T013 y T014 pasen

## Phase 4: US2 — Cada tienda de plataforma pertenece a una sucursal (P1)

**Independent test**: conectar una tienda y ver que queda en la sucursal; aceptar un pedido de esa tienda y ver que el pedido queda en la misma.

- [x] T016 [P] [US2] Test: una tienda nueva queda sola en la única sucursal; intentar ligar una tienda, una caja, un pedido, un movimiento de inventario o un gasto a una sucursal de otra empresa falla por la FK compuesta (ataque con `branch_id` ajeno); un gasto sin sucursal se acepta, en server/internal/integration/branches_test.go
- [x] T017 [P] [US2] Test: con dos sucursales, aceptar un pedido de la tienda de la sucursal B usa el turno abierto de B, no el de A, en server/internal/integration/pedidos_de_plataforma_test.go
- [x] T018 [US2] `GetOpenPrimarySession` recibe la sucursal en server/queries/cash.sql (`r.branch_id = @branch_id`); `make sqlc`
- [x] T019 [US2] Al aceptar un pedido de plataforma, buscar el turno con la sucursal de la tienda en server/internal/app/pedidos_de_plataforma.go:757

## Phase 5: US3 — Cajas, turnos, pedidos e inventario saben de qué sucursal son (P2)

**Independent test**: dos sucursales con su caja principal abiertas a la vez; cada pedido y cada movimiento de inventario en la suya.

- [x] T020 [P] [US3] Test: dos cajas principales en dos sucursales abiertas a la vez; dos principales en la misma sucursal se rechazan, en server/internal/integration/branches_test.go
- [x] T021 [P] [US3] Test: un pedido queda en la sucursal de la caja de su turno; con dos sucursales y sin selector, crear un pedido responde el error claro y no escoge una, en server/internal/integration/branches_test.go
- [x] T022 [P] [US3] Test: una venta descuenta existencias solo de su sucursal; `ListIngredients`, `ListStockLevels` y `GetOpenPrimarySession` no repiten ni cruzan sucursales, corridos con `inTheThreeCases`, en server/internal/integration/branches_test.go
- [x] T023 [US3] `insert into orders` toma `branch_id` de la caja del turno en server/queries/orders.sql; `make sqlc`
- [x] T024 [US3] Los llamadores de `GetOpenPrimarySession` pasan `current_branch_id()` (consulta auxiliar `CurrentBranch`) en server/internal/app/orders.go:188 y :1405 y server/internal/app/backoffice.go:1652
- [x] T025 [US3] `ListIngredients` (server/queries/ingredients.sql:25) y `ListStockLevels` (server/queries/stock.sql:26) unen `stock_levels` con `branch_id = current_branch_id()`; `make sqlc`

## Phase 6: Polish

- [x] T026 [P] Constitución, principio VIII: la puerta «Más de una sucursal» pasa a CRUZADA (spec 025) con lo que sigue abierto (catálogo, empleados y precio por sucursal; selector) en .specify/memory/constitution.md, commit propio con versión MINOR
- [x] T027 [P] Nota en AGENTS.md (§1, listas y consultas): `current_branch_id()` es la única forma de resolver la sucursal, un insert como owner con otra empresa pasa `branch_id` explícito, y el restore no debe disparar la creación de matrices
- [x] T028 [P] docs/emparejamiento-de-plataformas.md: la sucursal ya existe; el precio por platillo cuelga de la tienda, que cuelga de la sucursal
- [x] T029 `cd server && go build ./... && go test ./...`; `make lint`
- [ ] T030 Ensayo en `pos-vps-dev` con el respaldo más reciente de producción según quickstart.md §«En el ambiente de pruebas»; cobrar todo pedido de prueba; apagar la VM
- [ ] T031 `/revision-de-codigo` sobre el diff

## Dependencies

- Phase 2 bloquea todo. T006→T007→T008→T009 en el mismo archivo, en orden.
- US1 y US2 dependen solo de Phase 2. US3 depende de T018 (firma nueva de `GetOpenPrimarySession`).
- T030 al final, con todo en verde.

## Parallel

- T002, T003 y T004 se escriben a la vez.
- T013, T014, T016, T017, T020, T021 y T022 (tests) se escriben a la vez; viven en el mismo archivo salvo T014 y T017, así que en la práctica se agrupan.
- T026, T027 y T028 a la vez.

## Implementation Strategy

MVP = Phase 2 + US1: la base ya tiene sucursales y nada cambia para quien opera. US2 y US3 completan
lo que hace falta para que una segunda sucursal funcione sin mezclar datos. Todo va en una sola
migración, así que se entrega junto.
