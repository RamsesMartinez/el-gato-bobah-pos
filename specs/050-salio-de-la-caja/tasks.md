# Tasks: Importe que salió de la caja
Rutas relativas a `/home/ramy/git/egb-050-salio-de-la-caja`. Tests antes que código.

- [x] T001 Confirmar 0102 libre en `origin/develop`
- [x] T002 Test de migración (dos empresas; default 0; check; trigger fija amount + drawer_extra aunque el INSERT mande otro monto; Down se niega con extra) en server/internal/integration/migracion_salio_de_la_caja_test.go
- [x] T003 Migración server/migrations/0102_expense_payment_drawer_extra.sql (columna, check y trigger de R3)
- [x] T004 Test de dominio de `ValidDrawerExtra(extra, excludedSum, alreadyUsed)` (0, igual al tope, tope+0.01, negativo, sin excluidos, ya usado por otro pago, amount+extra sobre MaxMoney) en server/internal/domain/expense_payment_test.go
- [x] T005 Dominio: `ValidDrawerExtra`, `ErrDrawerExtraTooHigh`, código en respond.go
- [x] T006 [US1] Integración (R1–R5): dos «+1 pago» simultáneos no pasan el tope; volver a incluir un renglón con extra en uso → 400; el corte cuenta el extra en «No es del negocio» y no en «Gastos»; drawerExtra con tarjeta o sin incluir → 400; trigger fija el monto de la salida aunque el INSERT mande otro; anular gasto pendiente con extra. en server/internal/integration/salio_de_la_caja_test.go: alta en corte (esperado baja amount+extra, gasto conserva total), pendiente barrido al abrir entra con extra, desmarcar sube todo, `/pay` respeta lo ya usado, 400 por tope, `TestLoQueNoEsDelNegocioSaleDelCorteUnaSolaVez`
- [x] T007 [US1] Consultas y trigger según R1–R4, `make sqlc`; app y handler con `drawerExtra`
- [x] T008a [P] [US1] Tests front: el campo aparece solo con efectivo + arqueo + excluidos, por omisión el importe, viaja como `drawerExtra` (gasto nuevo y `PayDialog`)
- [x] T008 [US1] Front en web/src/features/backoffice/ExpenseDialog.tsx, ExpensesPage.tsx, web/src/api/backoffice.ts
- [x] T009a [P] [US2] Tests front: renglón «No es del negocio» en el desglose del corte y del resumen diario (R4); celdas de `Switch` de 44 px en CashPage; «Excluir» con texto y confirmación si hay recibido en ExpenseDetailDialog; color del botón de pasar ventas
- [x] T009 [US2] Front (incluye el renglón «No es del negocio» de R4) en web/src/features/backoffice/CashPage.tsx y ExpenseDetailDialog.tsx
- [x] T010 AGENTS.md: la salida incluye lo que no es del negocio
- [ ] T011 Gates y `make ci-local`; prueba con usuario nuevo a 1024×600
