# Tasks: Importe que salió de la caja
Rutas relativas a `/home/ramy/git/egb-050-salio-de-la-caja`. Tests antes que código.

- [ ] T001 Confirmar 0102 libre en `origin/develop`
- [ ] T002 Test de migración (dos empresas; default 0; check; Down se niega con extra) en server/internal/integration/migracion_salio_de_la_caja_test.go
- [ ] T003 Migración server/migrations/0102_expense_payment_drawer_extra.sql
- [ ] T004 Test de dominio de `ValidDrawerExtra` (0, igual al tope, tope+0.01, negativo, sin excluidos) en server/internal/domain/expense_payment_test.go
- [ ] T005 Dominio: `ValidDrawerExtra`, `ErrDrawerExtraTooHigh`, código en respond.go
- [ ] T006 [US1] Integración (R1–R5): dos «+1 pago» simultáneos no pasan el tope; volver a incluir un renglón con extra en uso → 400; el corte cuenta el extra en «No es del negocio» y no en «Gastos»; drawerExtra con tarjeta o sin incluir → 400; trigger fija el monto de la salida aunque el INSERT mande otro; anular gasto pendiente con extra. en server/internal/integration/salio_de_la_caja_test.go: alta en corte (esperado baja amount+extra, gasto conserva total), pendiente barrido al abrir entra con extra, desmarcar sube todo, `/pay` respeta lo ya usado, 400 por tope, `TestLoQueNoEsDelNegocioSaleDelCorteUnaSolaVez`
- [ ] T007 [US1] Consultas y trigger según R1–R4, `make sqlc`; app y handler con `drawerExtra`
- [ ] T008a [P] [US1] Tests front: el campo aparece solo con efectivo + arqueo + excluidos, por omisión el importe, viaja como `drawerExtra` (gasto nuevo y `PayDialog`)
- [ ] T008 [US1] Front en web/src/features/backoffice/ExpenseDialog.tsx, ExpensesPage.tsx, web/src/api/backoffice.ts
- [ ] T009a [P] [US2] Tests front: celdas de `Switch` de 44 px en CashPage; «Excluir» con texto y confirmación si hay recibido en ExpenseDetailDialog; color del botón de pasar ventas
- [ ] T009 [US2] Front en web/src/features/backoffice/CashPage.tsx y ExpenseDetailDialog.tsx
- [ ] T010 AGENTS.md: la salida incluye lo que no es del negocio
- [ ] T011 Gates y `make ci-local`; prueba con usuario nuevo a 1024×600
