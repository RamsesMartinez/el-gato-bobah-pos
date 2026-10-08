# Tasks: Defectos de dinero de la auditoría (031)

**Input**: [spec.md](spec.md), [plan.md](plan.md), [research.md](research.md), [data-model.md](data-model.md), [contracts/api.md](contracts/api.md)

**Tests**: obligatorios (constitución IV). En cada defecto la tarea de test va ANTES que la del
arreglo y se ve en rojo por la razón correcta antes de seguir.

Rutas relativas a la raíz del worktree. `[P]` = archivo distinto, sin dependencia pendiente.

## Phase 1: Setup

- [ ] T001 Base restaurada `gatobobah_restored` en `egb031-pg` con el respaldo más reciente (dos empresas), con dueños y GRANT, vía `scripts/restaurar-respaldo.sh` (quickstart §Prerrequisitos)

## Phase 2: Foundational — migración 0080 (bloquea US1, US3, US5)

- [ ] T002 Test de la migración sobre respaldo real: `server/internal/integration/migracion_dinero_por_turno_test.go` (`TestMigrationMoneyByShiftOnARealBackup`): columnas nuevas, backfill de turno desde la salida de caja y de día desde `created_at` sin cruzar empresas, FK compuesta rechaza un turno de otra empresa, check nuevo, Down limpio y Down que se niega con una devolución de solo propina, devolución por tarjeta del turno abierto ligada a él, `gatobobah_app` lee/escribe
- [ ] T003 `server/migrations/0080_dinero_por_turno_y_dia.sql` y `0081_dia_de_cada_pago.sql` según data-model.md; `make sqlc`
- [ ] T004 `TestEveryCompanyTableIsIsolated` en `server/internal/integration/` sigue verde (sin tabla nueva)

## Phase 3: US1 — una devolución nunca saca más ni por otro medio (P1)

- [ ] T005 [P] [US1] Unitarios en `server/internal/domain/devolucion_test.go`: reparto descuenta lo devuelto por medio (D1); `MontoDevolvibleDeRenglon` (D4)
- [ ] T006 [US1] Dominio: `CobradoPorMetodo.Devuelto`, `RepartirDevolucion` neto, `MontoDevolvibleDeRenglon`, `ErrRefundOnRefundedOrder` en `server/internal/domain/devolucion.go`
- [ ] T007 [US1] Integración `server/internal/integration/dinero_devoluciones_test.go`: segunda devolución por tarjeta sin segunda salida (D1); dos devoluciones simultáneas y devolución + cancelación simultáneas (D3); tope por renglón, renglón ajeno, renglón tras devolver todo (D4); reembolsada vieja rechazada (D18)
- [ ] T008 [US1] `SumOrderPaymentsByMethod` con devuelto por medio (pre-agregado) y `GetOrderLineForRefund` en `server/queries/orders.sql`; `make sqlc`
- [ ] T009 [US1] `devolverEnTx`: candado del pedido, estado, tope de renglón, reparto neto; `PorDevolver` con la misma regla, en `server/internal/app/devolucion.go`

## Phase 4: US2 — devolver un pago no regresa lo ya devuelto (P1)

- [ ] T010 [P] [US2] Unitario `VoidKeepsRefunds` en `server/internal/domain/devolucion_test.go` (D2)
- [ ] T011 [US2] Integración en `dinero_devoluciones_test.go`: devolver $40 y luego el pago de $100 → rechazado; devolución por tarjeta no bloquea el pago en efectivo (D2)
- [ ] T012 [US2] `VoidKeepsRefunds` + `ErrPaymentHasRefunds` en dominio; `VoidPayment` lo aplica por medio (`server/internal/app/devolucion.go`); mapeo en `httpapi.Error` si hace falta

## Phase 5: US3 — el corte espera lo que el turno movió (P1)

- [ ] T013 [US3] Integración `server/internal/integration/dinero_corte_test.go`: tarjeta devuelta en el mismo turno → esperado 0 y listada (D6); pago en A y devolución en B, con esperado negativo en B (D6); efectivo sin turno rechazado y sin rastro (D7); tarjeta sin turno entra al siguiente turno, no la de antes del último cierre (D7); Uber sin turno entra con su pago (D5); cobro concurrente al cierre dentro del esperado o rebota, movimiento y traspaso igual (D8); pedido de A cobrado en B: sin cobrar de A fijo y «Cobros de otros turnos» en B (D12)
- [ ] T014 [US3] Aislamiento `inTheThreeCases` de `ListSessionRefunds`, `ClaimOrphanRefunds` y `SessionMoneyByMethod` bajo `appRoleStore`
- [ ] T015 [US3] Consultas: `ExpectedByMethodForSession` (+`refunded`, `earlier`), `SessionMoneyByMethod`, `ListSessionRefunds`, `ClaimOrphanRefunds`, `ClaimPlatformOrderPayments`, `UncollectedInSession` por turno del pago, `LockSessionForClose`, `LockOpenSessionForShare`, `InsertOrderRefund` con turno/día/propina, pagos con `business_date` en `server/queries/{cash,orders,pedidos_de_plataforma}.sql`; `make sqlc`
- [ ] T016 [US3] `devolverEnTx` guarda turno y día; efectivo sin turno → `ErrCashRefundNeedsOpenRegister` (`server/internal/app/devolucion.go`, `server/internal/domain/devolucion.go`)
- [ ] T017 [US3] `OpenSession` reclama pagos de plataforma y devoluciones huérfanas; `CloseSession` en transacción con candado; `RecordCashMovement`/`Transfer` con candado; `sessionWithExpected(q)`; desglose con «Cobros de otros turnos»/«Devoluciones»; `Refunds` en las vistas (`server/internal/app/backoffice.go`, `pedidos_de_plataforma.go`)
- [ ] T018 [US3] Pagos con `business_date` en `Charge` y en aceptar pedido de plataforma (`server/internal/app/orders.go`, `pedidos_de_plataforma.go`)

## Phase 6: US4 — cancelar con devolución regresa la propina (P2)

- [ ] T019 [P] [US4] Unitario de la propina por medio menos la devuelta en `server/internal/domain/devolucion_test.go`
- [ ] T020 [US4] Integración en `server/internal/integration/dinero_devoluciones_test.go`: $100 + $10 en efectivo cancelado → salida $110, `refund_amount` 100, esperado = fondo; devolución parcial no toca la propina (D9)
- [ ] T021 [US4] `CancelarConDevolucion` devuelve la propina por medio en el mismo renglón del libro (`server/internal/app/devolucion.go`)

## Phase 7: US5 — los reportes dicen lo mismo que la venta (P2)

- [ ] T022 [US5] Integración `server/internal/integration/dinero_reportes_test.go`: utilidad sin quitados y con descuento (D10); renglones cancelados de un pedido cancelado después (D14); cobro por monto de un turno cerrado en su día y devolución en el suyo, en `SalesByMethod` y en el resumen de Ventas (D12); método con solo devoluciones en el periodo sale; cancelado anterior a 0060 sin libro sigue fuera; `inTheThreeCases` de las consultas cambiadas
- [ ] T023 [US5] `ProductMargins`, `SalesByMethod` (reports.sql); `SalesTotalsByMethod`(+gemela) y `SalesCancelledLines`(+gemela) (sales.sql); `refunds` en `MethodTotals` (`server/internal/app/sales.go`)
- [ ] T024 [P] [US5] `web/src/features/backoffice/CashPage.test.tsx`: «Total Dif.» incluye la diferencia del cajón (D13); lista de devoluciones plegada con contador; «Devoluciones» en rojo
- [ ] T025 [US5] `CashPage.tsx`: `TotalsTable` con la diferencia del cajón; `RefundsList` plegada con contador; conceptos negativos en rojo; tipos en `web/src/api/backoffice.ts` (+ `refunds` en byMethod si el front lo tipa)
- [ ] T026 [P] [US5] `web/src/features/sales/SalesPage.test.tsx`: con filtro de tipo no se pide el resumen de plataformas (D15)

## Phase 8: US6 — inventario y centavos (P3)

- [ ] T027 [P] [US6] Unitarios: `PagosCubren` exacto (`server/internal/domain/order_test.go`), `SplitLineTotal` (`split_bill_test.go`)
- [ ] T028 [US6] Integración en `server/internal/integration/dinero_inventario_test.go`: cancelar con frappé enviado a cocina no repone (D11); cobrar $99.99 de $100 deja $0.01 (D16); partir $45.55 a la mitad suma $45.55 (D17)
- [ ] T029 [US6] `PagosCubren` exacto en `server/internal/domain/order.go`; `SplitLineTotal` y `ListLinesToSplit` con `line_total`; `splitOrderLine`; `CancelarConDevolucion` (`server/internal/app/devolucion.go`) repone por renglón con `ReponeInventario` y `RestockCancelledOrder` (`server/queries/orders.sql`) solo para movimientos sin renglón

## Phase 9: Polish

- [ ] T030 `docs/matriz-de-cobro.md`: casos nuevos D1–D18
- [ ] T031 Gates en `server/` y `web/`: `go build`, `go test`, `scripts/hooks/golangci-lint.sh`, integración completa, `bun run lint && bun run vitest run && bun run build`
- [ ] T032 `/revision-de-codigo` sobre el diff de `server/` y `web/`, y corregir lo que encuentre

## Dependencias

- Phase 2 antes de US1, US3, US5 (columnas nuevas). US2 depende de T008 (devuelto por medio).
- US4 depende de T015 (insert con propina). Lo demás es independiente.

## Paralelo

- Los unitarios de dominio (T005, T010, T019, T027) se escriben juntos.
- Los tests de front (T024, T026) no dependen del backend.

## Estrategia

MVP = Phase 2 + US1 + US2 (el dinero que sale físicamente del cajón). Después US3 (corte), US4, US5,
US6. Un commit por defecto o por grupo chico de defectos, cada uno con su test visto en rojo.
