# Tasks: Gastos en efectivo sin turno abierto

**Input**: [spec.md](spec.md), [plan.md](plan.md), [data-model.md](data-model.md), [contracts/api.md](contracts/api.md)
**Tests**: obligatorios (constitución IV): cada test se escribe antes que su código y se ve fallar.

Rutas relativas al worktree `/home/ramy/git/egb-047-gastos-sin-turno`.

## Phase 1: Setup

- [ ] T001 Confirmar que el siguiente número libre de migración en `origin/develop` es 0099 (`git ls-tree origin/develop server/migrations/`) y anotarlo en plan.md si cambia

## Phase 2: Foundational (bloquea US2 y US3)

- [ ] T002 Test de la migración en server/internal/integration/migracion_pago_con_caja_test.go: con dos empresas, rellena `register_id` desde la sesión, el trigger lo deriva en inserts sin `register_id` (API vieja), la FK compuesta rechaza una caja de otra empresa, el check impide `counts_cash = false` en la principal, y el Down se niega si hay un pago con caja y sin sesión (R3–R5)
- [ ] T003 Migración server/migrations/0099_expense_payment_register.sql: `cash_registers.counts_cash`, `unique (id, company_id)`, `expense_payments.register_id` con FK compuesta `on delete restrict` (`not valid` + `validate`), trigger que deriva la caja de la sesión, relleno en un solo `update`, `do $$` de verificación, Down que se niega con datos nuevos
- [ ] T004 Consultas en server/queries/cash.sql (`ListCashRegisters`, `ListAllCashRegisters`, `GetCashRegister` con `counts_cash`; `GetCashRegisterForUpdate`; `UpdateCashRegister` con `coalesce(sqlc.narg('counts_cash'), counts_cash)`) y server/queries/expenses.sql (`CreateExpensePayment` y `ListExpensePayments` con `register_id`); `make sqlc`
- [ ] T005 Test de dominio en server/internal/domain/expense_payment_test.go (tabla) para `ExpensePaymentTarget`: sin arqueo → fuera sin confirmación; con arqueo + turno + fecha del turno → corte; con arqueo y sin turno o con otra fecha → requiere confirmación; con confirmación → fuera; turno abierto del 31-oct y pago del 1-nov → requiere confirmación (A1); tarjeta → como hoy
- [ ] T006 `ExpensePaymentTarget` y sentinels `ErrExpenseOutsideCount`, `ErrRegisterNotCounted`, `ErrRegisterHasOpenShift`, `ErrPrimaryRegisterCounts` en server/internal/domain/expense.go y server/internal/domain/errors.go; mapeo a 409 con códigos `EXPENSE_OUTSIDE_COUNT`, `REGISTER_NOT_COUNTED`, `REGISTER_HAS_OPEN_SHIFT`, `PRIMARY_REGISTER_COUNTS` en server/internal/httpapi/respond.go, con su caso en server/internal/httpapi/respond_test.go

## Phase 3: User Story 1 — Fechas del negocio (P1)

**Independent test**: a las 23:30 del 31-oct del negocio, el formulario propone 31-oct.

- [ ] T007 [P] [US1] Test en web/src/features/backoffice/ExpenseDialog.test.tsx con reloj falso en 2026-11-01T05:30Z: fecha del ticket, día del gasto, pago, pago rápido y recepción (`receivedAt`) dicen 2026-10-31; a 2026-11-01T06:30Z dicen 2026-11-01
- [ ] T008 [US1] Reemplazar `today()` (`toISOString`) por el día del negocio de `useHoraDelNegocio` en web/src/features/backoffice/ExpenseDialog.tsx (todas sus llamadas, incluida la recepción)
- [ ] T009 [P] [US1] Test en server/internal/integration/expense_day_test.go: un gasto y una compra (purchases.go usa la misma `parseDayOrToday`) sin fecha, capturados a las 05:30 UTC del 1-nov, quedan con el 31-oct
- [ ] T010 [US1] `parseDayOrToday` usa `domain.BusinessDate` con la zona del negocio en server/internal/app/expenses.go

## Phase 4: User Story 2 — Caja sin arqueo (P1)

**Independent test**: con todo cerrado, un gasto en efectivo en una caja sin arqueo se guarda y ningún corte cambia.

- [ ] T011 [US2] Tests de integración bajo `appRoleStore` con `inTheThreeCases` en server/internal/integration/gasto_sin_turno_test.go: pago en caja sin arqueo sin turno se guarda con `register_id` y sin sesión; `TestUnGastoFueraDeCorteNoEntraANingunCorte` (ni `ListExpensesBySession` ni el esperado de efectivo de un corte abierto ni de uno cerrado); abrir turno en caja sin arqueo → `ErrRegisterNotCounted`; quitar arqueo con turno abierto o a la principal → 409; carrera PATCH vs abrir turno con dos conexiones (R2)
- [ ] T012 [US2] `validatePayments`/`insertPayments` en server/internal/app/expenses.go: leer la caja (`GetCashRegister`, RLS), aplicar `ExpensePaymentTarget`, guardar `register_id`; movimiento de caja solo con sesión
- [ ] T013 [US2] `OpenSession` (guarda B5 con la caja `for update`) y `UpdateCashRegister` (guardas B4, `countsCash` puntero) en server/internal/app/backoffice.go; `countsCash` en el JSON de server/internal/httpapi/handlers_backoffice.go, exigiendo `PermCashConfigure` cuando viene (R15, FR-010); test de un rol sin el permiso → 403 en T011
- [ ] T014 [P] [US2] Test en web/src/features/backoffice/CashPage.test.tsx: columna «Lleva arqueo», deshabilitada en la principal, mensajes de R14 ante 409
- [ ] T015 [US2] Columna «Lleva arqueo» con `Switch` en la tabla de cajas de web/src/features/backoffice/CashPage.tsx; `countsCash` en web/src/api/backoffice.ts

## Phase 5: User Story 3 — Caja con arqueo cerrada o fecha pasada (P1)

**Independent test**: gasto en efectivo del 15-oct en la principal cerrada → hoja de aviso → registrado sin tocar cortes.

- [ ] T016 [US3] Tests de integración en server/internal/integration/gasto_sin_turno_test.go: con arqueo y sin turno, sin `outsideCount` → 409 `EXPENSE_OUTSIDE_COUNT` y nada guardado; con `outsideCount` → guardado fuera de corte; turno abierto + fecha anterior → 409 y no entra al corte; turno abierto + fecha del turno → entra como hoy; mismo comportamiento por `POST /expenses/{id}/pay`; un corte cerrado que cubre la fecha no cambia; un pago ya ligado a un corte cerrado sigue sin poder moverse
- [ ] T017 [US3] `outsideCount` en `ExpensePaymentInput` y su decodificación en server/internal/app/expenses.go y server/internal/httpapi/handlers_backoffice.go
- [ ] T018 [P] [US3] Tests en web/src/features/backoffice/ExpenseDialog.test.tsx: el selector lista cajas cerradas con sufijo «· sin turno»/«· sin arqueo»; sin turno aparece el chip «Efectivo» del pie; ante 409 `EXPENSE_OUTSIDE_COUNT` se abre la hoja, «Cancelar» no reenvía, confirmar reenvía con `outsideCount: true` (gasto nuevo y `/pay`)
- [ ] T019 [US3] En web/src/features/backoffice/ExpenseDialog.tsx: `openRegisters` → cajas activas en sus cuatro usos (R9), sufijos (R12), «Sin cajas.» (R11), `onError` de las dos mutaciones con `ConfirmSheet` y reenvío (R10, R13)

## Phase 6: Polish

- [ ] T020 AGENTS.md: un renglón sobre «lleva arqueo» y el pago fuera de corte (dónde vive la regla y que el Down de 0099 se niega)
- [ ] T021 Gates: `go build ./... && go test ./...`, `bun run lint && bun run vitest run && bun run build`, `make ci-local`
- [ ] T022 Quickstart Q1–Q6 contra `make start` a 1024×600; dejar el ambiente sin cajas abiertas

## Dependencies

Phase 2 → US2, US3. US1 es independiente. US3 depende de T012 (US2).

## Implementation strategy

MVP = US1 + US2 + US3 juntas (las tres son P1 y las necesita el cierre de octubre). Una sola subida.
