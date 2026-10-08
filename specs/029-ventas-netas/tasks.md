# Tasks: Ventas netas y devoluciones a la vista (029)

**Input**: [spec.md](spec.md), [plan.md](plan.md), [research.md](research.md), [data-model.md](data-model.md), [contracts/api.md](contracts/api.md)

**Tests**: obligatorios (constitución IV). La tarea de test va ANTES que la del arreglo y se ve en
rojo por la razón correcta antes de seguir.

Rutas relativas a la raíz del worktree. `[P]` = archivo distinto, sin dependencia pendiente.

## Phase 1: Setup

- [X] T001 Postgres propio `egb029-pg` en :5503 y `TEST_DATABASE_URL` para la integración (quickstart §1)

## Phase 2: Foundational

Sin tareas: no hay migración ni tipo compartido que bloquee a las historias.

## Phase 3: US1 — el Total de Ventas es lo que se puede facturar (P1)

**Independent test**: un día con un pedido cobrado, uno devuelto completo y uno abierto → Total = solo
el cobrado; «Por cobrar» aparte; Total = Σ medios.

- [X] T002 [P] [US1] Unitario `NetCollected` (Σ medios, con un medio negativo) y que `SummarizeSales` ya no fija el Total en `server/internal/domain/sales_test.go`
- [X] T003 [US1] Integración `server/internal/integration/ventas_netas_test.go`: Total sin el devuelto ni el abierto, `pending` con monto y conteo, Total = Σ `byMethod`, cobro de septiembre devuelto en octubre (dos meses), propina devuelta en `tipRefunds` y fuera del Total; `SalesPending` bajo `inTheThreeCases`
- [X] T004 [US1] `SalesPending` + `SalesPendingSinFolio` y `tip_refunds` en `SalesTotalsByMethod`(+gemela) en `server/queries/sales.sql`; `make sqlc`
- [X] T005 [US1] `NetCollected` en `server/internal/domain/sales.go`; resumen con Total neto, `Pending`, `TipRefunds` y Total de la búsqueda por folio en `server/internal/app/sales.go`
- [ ] T006 [P] [US1] Vitest en `web/src/features/sales/SalesPage.test.tsx`: tile «Por cobrar» fuera del total, medio con «ya restado» y propina devuelta, Total rotulado «cobrado, neto»
- [ ] T007 [US1] Tipos en `web/src/api/sales.ts`; tiles en `web/src/features/sales/SalesSummaryTiles.tsx` (orden Total → medios compactos → Por cobrar → separador → resto; «Ticket promedio · de los pedidos», «Ventas · sin canceladas», Por cobrar sin tooltip)

## Phase 4: US2 — devolución a primera vista en Ventas (P1)

- [X] T008 [US2] Integración en `ventas_netas_test.go`: la lista trae `paid` y `lastRefundAt`, `null` en el JSON crudo de un pedido sin devoluciones (y la búsqueda por folio igual)
- [X] T009 [US2] `paid` y `last_refund_at` en `ListSales`, `ListSalesSinFolio`, `FindSaleByPlatformRef` (`server/queries/sales.sql`); `SaleRow` en `server/internal/app/sales.go`
- [ ] T010 [P] [US2] Vitest en `SalesPage.test.tsx`: renglón con día y hora, «Devuelto $X · fecha hora», «Por cobrar $Y», pie «N pedidos en la lista»
- [ ] T011 [US2] Título en la fila del rango, marcas como 2ª línea de la celda Estado, pie, en `web/src/features/sales/SalesPage.tsx`; devuelto en `SaleDetailDialog.tsx`

## Phase 5: US3 — el corte y Ventas clasifican igual (P1)

- [X] T012 [P] [US3] Unitario de la clasificación del desglose (devolución de venta vs propina devuelta, salidas que son devolución fuera de «Salidas», nota del negativo) en `server/internal/domain/cash_test.go` (`SplitRefunds`, `NegativeMethodNote`)
- [X] T013 [US3] Integración `server/internal/integration/ventas_netas_corte_test.go`: devolución en efectivo y con tarjeta en el mismo turno → las dos en «Devoluciones» de su medio, ninguna en «Salidas de efectivo», esperado igual y fondo + ingresos − egresos del desglose = esperado del efectivo (falla nombrando el concepto); cancelar $100+$10 en efectivo → tips del efectivo 0, «Devoluciones» −100; Histórico de turno abierto con totales en vivo (y ciego ocultando); medio negativo con nota; misma cifra devuelta que Ventas
- [X] T014 [US3] `refunded_tips`, `drawer_refunded`, `drawer_refunded_tips` en `ExpectedByMethodForSession` e `is_refund` en `ListCashMovements` (`server/queries/cash.sql`); `make sqlc`
- [X] T015 [US3] `corteBreakdown`, `sessionWithExpected`, `SessionDetail` (turno abierto en vivo) en `server/internal/app/backoffice.go`
- [ ] T016 [P] [US3] Vitest `web/src/features/backoffice/CashPage.test.tsx`: nota del negativo visible
- [ ] T017 [US3] Nota y tipos en `web/src/features/backoffice/CashPage.tsx` y `web/src/api/backoffice.ts`

## Phase 6: US4 — el detalle y la API dicen la verdad (P2)

- [X] T018 [P] [US4] Unitarios en `server/internal/domain/{devolucion,cobro,order}_test.go`: nada por devolver (pedido y renglón), cobro < $0.01, plataforma en mostrador
- [X] T019 [US4] Integración en `ventas_netas_test.go`: `GET /orders/:id` (vía `Detail`) con `refund` real; renglón ya devuelto sin monto → mensaje de nada por devolver; pedido de plataforma en mostrador → `ErrValidation`; cobrar 0.005 → rechazo y sigue debiendo
- [X] T020 [US4] `ValidarDevolucion` y ruta de renglón en `server/internal/domain/devolucion.go` + `server/internal/app/devolucion.go`; `ValidChargeAmount` en `cobro.go` + `Charge`; `ValidPlatformServiceType` en `domain` + `Create`; `Refund` en `OrdersService.load`

## Phase 7: US5 — se lee sin ambigüedad en la tableta (P2)

- [ ] T021 [P] [US5] Vitest `web/src/utils/format.test.ts`: `money` con centavos a dos decimales, entero sin decimales
- [ ] T022 [US5] `money` en `web/src/utils/format.ts`
- [X] T023 [US5] Integración `ProductMargins`: renglón sin costo en `uncosted_revenue` y fuera del margen (`server/internal/integration/money_reports_test.go`)
- [X] T024 [US5] `ProductMargins` en `server/queries/reports.sql`; `make sqlc`
- [ ] T025 [P] [US5] Vitest `web/src/features/backoffice/ReportsPage.test.tsx`: «Por medio de pago» dice que resta devoluciones; «sin costo capturado»; fecha local en Propinas por día
- [ ] T026 [US5] `ReportsPage.tsx`
- [ ] T027 [P] [US5] Vitest `CashPage.test.tsx`: controles ≥44 px (Ver, Editar, pestañas, Traspaso, Monto/Concepto, desplegables), concepto sin `maxW`, Devoluciones y Pagos devueltos sin scroll propio
- [ ] T028 [US5] `CashPage.tsx`

## Phase 8: Polish

- [ ] T029 Gates: server `go build ./... && go test ./...`, `bash ../scripts/hooks/golangci-lint.sh`, integración; web `bun run lint && bun run vitest run && bun run build`
- [ ] T030 Capturas a 1024×600 contra API local (Ventas hoy/mes, Caja con devoluciones, Histórico → Ver abierto, Reportes); ≥4 renglones en Ventas (SC-004)
- [ ] T031 `docs/matriz-de-pantallas.md` y `docs/criterios-de-ventas.md`: el Total de Ventas ya es el cobrado neto

## Dependencies

- US1–US5 independientes entre sí salvo `SalesSummaryTiles.tsx` (US1) antes de la US2 en `SalesPage.tsx` (mismo archivo de test).
- Dentro de cada historia: test en rojo → consulta/`make sqlc` → dominio/app → front.

## Parallel

- T002, T006, T012, T018, T021 son de archivos distintos y pueden escribirse a la vez.

## MVP

US1 (el Total que se factura). Después US4 (arreglos de API cortos) y US3.
