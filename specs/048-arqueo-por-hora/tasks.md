# Tasks: Arqueo por hora de los gastos

**Input**: [spec.md](spec.md), [plan.md](plan.md) (R1–R12 y R2b prevalecen), [data-model.md](data-model.md), [contracts/api.md](contracts/api.md)
**Tests**: obligatorios y antes que el código (constitución IV). Rutas relativas a `/home/ramy/git/egb-048-arqueo-por-hora`.

## Phase 1: Setup
- [ ] T001 Confirmar que 0100 es el siguiente número libre en `origin/develop`

## Phase 2: Foundational
- [ ] T002 Test de migración en server/internal/integration/migracion_arqueo_por_hora_test.go con dos empresas: relleno de `include_in_count` y `paid_at` (R6); un insert de la API vieja sin las columnas pasa, queda «fuera» sin corte y «dentro» con corte (R1, A1); el check rechaza corte sin incluir; la unique de salida por pago rechaza una segunda salida (R2); la FK del movimiento no cruza empresas; el Down se niega con un pendiente
- [ ] T003 Migración server/migrations/0100_expense_payment_count_window.sql según R1, R2, R5, R6, R7
- [ ] T004 Consultas en server/queries/expenses.sql y server/queries/cash.sql: `CreateExpensePayment` con `include_in_count`/`paid_at`; `ListExpensePayments` con ambos y si su corte está cerrado; `SweepPendingExpensePayments` (update … returning); `ListPendingExpensePayments(register, since)`; `LastCloseOfRegister`; `GetExpensePaymentForUpdate`; `SetPaymentInCount`; `InsertExpenseCashMovement` con `expense_payment_id`; `DeleteExpenseSalidaOfPayment`; `OpenSession` con `opened_at`; `GetCashRegisterForShare`; `make sqlc`
- [ ] T005 Test de dominio (tabla) en server/internal/domain/expense_payment_test.go: `PaymentTime` (hoy → ahora; otro día → medianoche en la zona del negocio, incluido el cambio de día a las 18:00 de México), `ExpensePaymentTarget` ampliada (fuera / corte / pendiente / 409 OUTSIDE / 409 BEFORE_LAST_CLOSE, legado 047, turno abierto con hora anterior a su apertura) y `ValidBackdatedOpening` (futura, antes del último cierre, válida)
- [ ] T006 Dominio: `PaymentTime`, `ExpensePaymentTarget` ampliada, `ValidBackdatedOpening`, sentinels `ErrExpenseBeforeLastClose`, `ErrPaymentInClosedCount`, `ErrInvalidOpeningTime`, permiso `PermCashAdjustCount` (admin, gerente) con su mensaje, acciones `AuditExpenseCountChanged`, `AuditCashOpeningBackdated`; códigos en server/internal/httpapi/respond.go

## Phase 3: US1 — Casilla (P1)
- [ ] T007 [US1] Integración en server/internal/integration/arqueo_por_hora_test.go: caja abierta + pago del día con `includeInCount=false` no cambia el esperado; tarjeta y caja sin arqueo guardan `include_in_count=false`; un pago que espera al cierre de su caja termina en 409 y no pendiente (R4)
- [ ] T008 [US1] `validatePayments`/`insertPayments` en server/internal/app/expenses.go: `includeInCount`, `paid_at` (R5), candado de caja `for share` y re-decisión (R4), salida con `expense_payment_id`; decodificar `includeInCount` en server/internal/httpapi/handlers_backoffice.go
- [ ] T009a [P] [US1] Tests front (verlos fallar) en web/src/features/backoffice/ExpenseDialog.test.tsx y ExpensesPage.test.tsx: la casilla aparece solo con efectivo y caja con arqueo, marcada, y viaja como `includeInCount`
- [ ] T009 [US1] Código front: `Checkbox` según R10 en `PaymentRow` (ExpenseDialog.tsx) y `PayDialog` (ExpensesPage.tsx); `includeInCount` en web/src/api/backoffice.ts

## Phase 4: US2 — Pendientes y barrido (P1)
- [ ] T010 [US2] Integración: caja cerrada → 409 OUTSIDE; confirmado → pendiente; antes del último cierre → 409 BEFORE_LAST_CLOSE; apertura con `openedAt` barre los pendientes de su ventana y baja el esperado; el cierre barre los que se registraron pendientes dentro de la ventana; `TestUnPendienteSeDescuentaUnaSolaVez`; caja archivada con pendientes se queda fuera sin error (B5); `GET pending-expenses` bajo `appRoleStore` con `inTheThreeCases`
- [ ] T011 [US2] `sweepPendingExpenses` en server/internal/app/expenses.go, llamado en `OpenSession` y `CloseSession` (server/internal/app/backoffice.go) antes del esperado; `PendingExpenses` y `GET /cash-sessions/pending-expenses`
- [ ] T012a [P] [US2] Tests front (verlos fallar): hoja de aviso según R9 (gasto nuevo y `/pay`); línea de pendientes en la apertura según R11, con y sin `can('cash.adjust_count')`, y que no aparece en el cierre
- [ ] T012 [US2] Código front: R9 (web/src/features/backoffice/cajaDelGasto.ts, ExpenseDialog.tsx, ExpensesPage.tsx) y R11 (web/src/features/backoffice/ContadorDeEfectivo.tsx, CashPage.tsx)

## Phase 5: US3 — Hora de apertura (P2)
- [ ] T013 [US3] Integración: `openedAt` sin permiso → 403; futura o antes del último cierre → 400; válida → evento `cash.opening_backdated`
- [ ] T014 [US3] `openedAt` en `AperturaCmd`, permiso en el handler de apertura, validación y bitácora en `OpenSession`

## Phase 6: US4 — Casilla después (P3)
- [ ] T015 [US4] Integración: desmarcar en corte abierto → se borra su salida y el esperado sube; volver a marcar en el mismo corte → una sola salida (R2b); marcar con hora en la ventana → salida; corte cerrado → 409 PAYMENT_IN_CLOSED_COUNT; sin permiso → 403; carrera PATCH contra cierre (R3); evento `expense.count_changed`
- [ ] T016 [US4] `SetPaymentInCount` en server/internal/app/expenses.go y `PATCH /expenses/{id}/payments/{paymentId}`
- [ ] T017a [P] [US4] Tests front (verlos fallar) del `Switch` del detalle (R12), visible solo con `can('cash.adjust_count')`
- [ ] T017 [US4] Código front en web/src/features/backoffice/ExpenseDetailDialog.tsx

## Phase 7: Polish
- [ ] T018 AGENTS.md: el barrido y el vínculo pago↔movimiento
- [ ] T019 Gates: `go build`, `go test ./...`, integración, `bun run lint/typecheck/vitest/build`, `make ci-local`
- [ ] T020 Quickstart Q1–Q6 a 1024×600 y prueba con usuario nuevo

## Dependencies
Phase 2 → todo. US2 depende de T008. US4 de T011.
