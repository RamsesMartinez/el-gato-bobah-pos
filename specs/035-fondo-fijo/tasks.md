# Tasks: Fondo de caja fijo

**Input**: [spec.md](spec.md), [plan.md](plan.md), [research.md](research.md), [data-model.md](data-model.md), [contracts/api.md](contracts/api.md)
**Tests**: obligatorios (constitución IV): cada test se escribe y se ve en rojo antes de su código.

## Phase 1: Setup

- [X] T001 Confirmar el siguiente número libre de migración sobre develop y crear `server/migrations/0090_fixed_cash_float.sql` vacío con `-- +goose Up/Down`; anotar en el encabezado que se renumera al fusionar (plan, «Migración: número»)

## Phase 2: Foundational (bloquea todas las historias)

- [X] T002 Test de migración en `server/internal/integration/fixed_float_migration_test.go`: dos empresas con cierres previos (con y sin `float_left`); tras migrar, `float_left` y `LastClosingCountOfRegister` idénticos, ningún `retiro` creado (FR-009, SC-003); `Down` aborta si hay `retiro`/`fixed_float`/`fixed_float_at_close`
- [X] T003 Escribir la migración en `server/migrations/0090_fixed_cash_float.sql` según data-model.md: `cash_registers.fixed_float`, `register_sessions.fixed_float_at_close`, `kind_check` con `retiro`, `tip_shape` con sus tres cláusulas (`NOT VALID` + `VALIDATE`), índice `register_cash_movements_one_withdrawal`, `Down` con guardas
- [X] T004 [P] Tests table-driven en `server/internal/domain/cash_float_test.go`: `Withdrawal` (negativo, > contado, NaN, igual = 0) y `ResolveFloatLeft` (enviado, fijo > contado → `min`, sin fijo → contado, enviado > contado → `ErrValidation`)
- [X] T005 Implementar `Withdrawal`, `ResolveFloatLeft` y la constante `CashRetiro` en `server/internal/domain/cash_float.go` y `server/internal/domain/cash.go`
- [X] T006 [P] Test en `server/internal/domain/permission_test.go`: `cash.configure` para admin y gerente, no para cajero ni mesero; agregar `PermCashConfigure` en `server/internal/domain/permission.go` y en `AllPermissions`
- [X] T007 Actualizar `server/queries/cash.sql` (`NetCashMovements` resta solo `salida`/`propina`; `ListAllCashRegisters` y `GetCashRegister` con `fixed_float`; `ListCashMovements` con `recipient_name`; nuevas `SetRegisterFixedFloat`, `InsertWithdrawal`; `SetFloatLeft` con `fixed_float_at_close`) y correr `make sqlc`

## Phase 3: US1 — Configurar el fondo fijo (P1)

**Independent test**: capturar $1,000 en Ajustes › Caja, recargar y ver $1,000; un cajero recibe 403.

- [X] T008 [P] [US1] Test de integración en `server/internal/integration/fixed_float_test.go`: `SetFixedFloat` bajo `appRoleStore` con `inTheThreeCases`; null quita el fijo; negativo/malformado → 400; caja de otra empresa → 404; `GET /cash-registers` del cajero NO trae `fixedFloat` (research R7)
- [X] T009 [US1] `BackofficeService.SetFixedFloat` en `server/internal/app/backoffice.go` (Round2 + ValidMoney)
- [X] T010 [US1] Handler `PUT /cash-registers/{id}/fixed-float` en `server/internal/httpapi/handlers_backoffice.go` y ruta con `RequirePermission(PermCashConfigure)` en `server/internal/httpapi/router.go`; `fixedFloat` en la respuesta de `/cash-registers/all`
- [X] T011 [P] [US1] Test en `web/src/features/admin/AjustesDeCaja.test.tsx` (Maquetas: «Ajustes › Caja: fondo fijo», opción B): renglón de 56 px por caja con monto o «Sin configurar»; la hoja con teclado propio y campo de solo lectura guarda y quita; sin `cash.configure` el renglón no abre nada
- [X] T012 [US1] Implementar el bloque «Fondo fijo» en `web/src/features/admin/AjustesDeCaja.tsx` y `fixedFloat`/`setFixedFloat` en `web/src/api/backoffice.ts`, y `cash.configure` en el espejo de permisos del front para `can()` (Maquetas: «Ajustes › Caja: fondo fijo», B; «Quitar» a la izquierda, ghost y rojo)

## Phase 4: US2 — El cierre trae el fondo escrito (P1)

**Independent test**: con fijo $1,000, cerrar sin tocar guarda $1,000; cerrar con $800 guarda $800 y el fijo sigue en $1,000.

- [X] T013 [P] [US2] Tests de integración en `server/internal/integration/fixed_float_test.go`: cierre sin `floatLeft` usa el fijo; fijo > contado con cliente viejo cierra con `min` (no 400); `floatLeft` explícito > contado → 400; `fixed_float_at_close` se guarda; la apertura siguiente compara contra `float_left` (FR-010); si el fijo cambia con la hoja abierta, el snapshot es el `fixedFloatShown` que mandó el cliente (analyze U1)
- [X] T014 [US2] En `CloseSession` (`server/internal/app/backoffice.go`) usar `ResolveFloatLeft` con el fijo de `GetCashRegister`; guardar el snapshot (el `fixedFloatShown` del cuerpo si viene, si no el fijo vigente); la vista del turno abierto trae `fixedFloat`
- [X] T015 [P] [US2] Test en `web/src/features/backoffice/FondoQueSeDeja.test.tsx` (Maquetas: «Hoja de cierre», A): llega con el fijo; «Solo hoy» si se cambia; vacío bloquea y el aviso «Falta capturar…» lo nombra; con «No» queda lo contado y «(el fijo es $X)»
- [X] T016 [US2] Implementar en `web/src/features/backoffice/FondoQueSeDeja.tsx` y `CashPage.tsx` la caja única de la opción A: segmentos «Sí / No» (abre en Sí) en el renglón del título y «Se queda [$] → Retiro $X» en un renglón (Maquetas: «Hoja de cierre»)

## Phase 5: US3 — El retiro queda registrado (P2)

**Independent test**: contado $5,300, fondo $1,000 → retiro $4,300 con la persona elegida; la diferencia del arqueo no cambia.

- [X] T017 [P] [US3] Tests de integración en `server/internal/integration/fixed_float_test.go`:
  - El retiro no altera el esperado ni la diferencia (SC-004).
  - El corte cerrado no infla «Ventas» ni «Salidas de efectivo»; el test falla diciendo «el retiro se restó de las ventas» (research R7).
  - Un doble cierre deja un solo retiro, y el índice único se mapea a `ErrConflict`.
  - Sin `withdrawalUserId`, el retiro va a quien cierra.
  - Con un usuario de otra empresa, responde 400.
  - Con retiro de 0 no hay fila.
  - En una caja sin fijo, con fondo tecleado, el retiro se registra igual (analyze C1).
- [X] T018 [US3] En `CloseSession` insertar el retiro con `InsertWithdrawal` en la misma tx (`server/internal/app/backoffice.go`); `withdrawalUserId` en `CierreCmd` y en el handler; `traduceConflictoDeCaja` conoce el índice; `SessionView` con `floatLeft`, `fixedFloatAtClose`, `withdrawal` (siempre presentes)
- [X] T019 [US3] `corteBreakdown` en `server/internal/app/backoffice.go`: `case CashRetiro` fuera de `net` y de salidas, renglón «Retiro al cierre»
- [X] T020 [P] [US3] Test en `web/src/features/backoffice/CashPage.test.tsx` (Maquetas: «Hoja de cierre», A):
  - Las personas van en una fila, con quien cierra primero.
  - Si falta la persona, se bloquea y el aviso lo nombra.
  - El `ConfirmSheet` dice «Se retiran $X · se los lleva N» o «Todo se queda en el cajón».
  - El movimiento `retiro` se pinta como «Retiro» y no como «Salida».
- [X] T021 [US3] Implementar en `web/src/features/backoffice/CashPage.tsx` la fila de personas (o `Picker` con más de 6), el texto del `ConfirmSheet`, la etiqueta «Retiro» en movimientos y el renglón del corte cerrado

## Phase 6: Polish

- [X] T022 Correr `cd server && go build ./... && go test ./...` y `cd web && bun run lint && bun run vitest run && bun run build`
- [X] T023 [P] Documentar en `AGENTS.md` (sección de caja): `retiro` no es salida ni entra al neto, y toda suma de movimientos lo excluye; el rollback de esta feature es solo hacia adelante
- [X] T024 Recorrer [quickstart.md](quickstart.md) contra la API local como `gatobobah_app` — resultado a 1024×600 en [validacion.md](validacion.md), en texto; los pasos 4 y 5 de la guía quedaron cubiertos por las pruebas de integración y no por la pantalla

## Phase 7: Revisión de código (2026-10-12)

- [X] T025 [US3] Con arqueo ciego, el detalle de un turno cerrado oculta a quien no tiene `cash.configure` el fondo, el fijo y el retiro (en el campo, en los movimientos y en los egresos), en `server/internal/app/backoffice.go` (`SessionDetailFor`) y `server/internal/httpapi/handlers_backoffice.go`. Test: `TestClosedShiftDetailDoesNotRevealNextOpeningToCashier`. Ver research R8.
- [X] T026 [US3] La respuesta del cierre relee los movimientos y rehace el desglose, así trae el retiro igual que el corte reabierto (`movementViews`, `corteMethods`). Test: `TestCloseResponseShowsWithdrawalLikeTheReopenedShift`.
- [X] T027 [US3] Test de atomicidad: un retiro rechazado deja el turno abierto, sin fondo, sin el snapshot del fijo y sin conteo de cierre. Test: `TestRejectedWithdrawalLeavesTheShiftOpenAndUntouched`.
- [X] T028 [US3] Aviso propio «Elige quién se lleva el retiro de $X», fuera de «Falta capturar lo contado», en `avisoDeQuienSeLoLleva` (`web/src/features/backoffice/fondoQueSeDeja.ts`) y `CashPage.tsx`.
- [X] T029 [US2] `FondoQueSeDeja.tsx` pinta el borde rojo solo con «Sí» y cuando el fondo excede lo contado.

- [X] T030 [US3] Apertura a ciegas, lo contado (decisión del dueño 2026-10-12, research R8): `IsLatestSessionOfRegister` y `is_latest_of_register` en `server/queries/cash.sql`; `ocultarElArqueoDelUltimoCierre` y `SessionHistoryFor` en `server/internal/app/backoffice.go`; `arqueoHidden` en `web/src/api/backoffice.ts`, «Arqueo oculto» en `CashPage.tsx` y «Oculto» en `historicoDeCaja.ts`. Tests: `TestLastCloseHidesCountedFromCashierUntilReopened` (visto en rojo: el cajero leía 5317.45 en el detalle) y `CashPage.test.tsx › el último cierre con el arqueo oculto` (rojo: el texto no existía y `diferenciaDelHistorico` no era función)
- [X] T031 [US1] La hoja del fondo fijo en Ajustes usa `components/ui/drawer` (con `Positioner`) en `web/src/features/admin/FondoFijo.tsx`: con las piezas crudas de Chakra se pintaba a media página y «Guardar» quedaba fuera de los 600 px (encontrado en T024). Test: `AjustesDeCaja.test.tsx › la hoja va fija al borde inferior de la pantalla` (visto en rojo)

## Dependencies

- Setup → Foundational (T002–T007) → US1, US2 en paralelo → US3 (necesita el `CloseSession` de US2).
- Dentro de cada historia: test → código.

## Parallel examples

- Foundational: T004 y T006 juntos.
- US1 y US2: T008/T011 y T013/T015 juntos.

## Implementation strategy

MVP = Foundational + US1 + US2 (fondo fijo y cierre que lo trae). US3 cierra el rastro del dinero; se sube con las otras, no aparte, porque el `CloseSession` lo comparte.
