# Tasks: Cambiar la terminal de un pago ya cobrado

**Input**: [spec.md](spec.md), [plan.md](plan.md), [research.md](research.md), [data-model.md](data-model.md), [contracts/change-payment-terminal.md](contracts/change-payment-terminal.md)
**Tests**: obligatorios (constitución, principio IV). Cada test se escribe y se ve fallar antes de su implementación.
**Worktree**: `/home/ramy/git/egb-036-terminal`, rama `036-cambiar-terminal-de-pago`. Sin migración.

## Phase 1: Setup

- [X] T001 Verificar línea base en el worktree: `cd server && go build ./... && go test ./...` y `cd web && bun install --frozen-lockfile && bun run vitest run`; anotar fallos previos en specs/036-cambiar-terminal-de-pago/quickstart.md si los hay

## Phase 2: Foundational (dominio compartido)

- [X] T002 [P] Test table-driven de `ValidateTerminalChange` en server/internal/domain/terminal_change_test.go: método no tarjeta, pedido cancelado, destino igual al actual, destino fuera de las activas de la sucursal, `fromTerminalId` distinto del actual (→ `ErrConflict`), pago «Sin terminal» con `from` nulo, motivo vacío/solo espacios/2 caracteres/201 caracteres, caso válido real (Terminal A → Terminal B, $250)
- [X] T003 Implementar `domain.TerminalChange` y `ValidateTerminalChange` (motivo con `strings.TrimSpace`, 3–200 caracteres en runas) en server/internal/domain/terminal_change.go
- [X] T004 [P] Test en server/internal/domain/permission_test.go: `payments.change_terminal` lo tienen admin y gerente, no cajero ni mesero; está en `AllPermissions`; mensaje de permiso negado en español sin nombrar roles
- [X] T005 Agregar `PermPaymentsChangeTerminal` a `AllPermissions`, `PermissionsFor(RoleGerente)` y `PermissionDeniedMessage` («Tu usuario no puede cambiar la terminal de un pago») en server/internal/domain/permission.go
- [X] T006 [P] Test en server/internal/domain/audit_test.go: `payment.terminal_changed` es válida y pertenece a `AuditGroupMoney`
- [X] T007 Agregar `{"payment.terminal_changed", AuditGroupMoney}` a `auditActions` en server/internal/domain/audit.go, y su etiqueta en español donde la pantalla de bitácora/historial traduce acciones (buscar el mapa en web/src/shared/audit/)
- [X] T008 Espejar el permiso en web/src/app/permissions.ts (tipo de permiso) con su test si el archivo lo tiene

## Phase 3: User Story 1 + 2 — Corregir la terminal y dejar rastro (P1) 🎯 MVP

**Goal**: un admin o gerente cambia la terminal de un pago con tarjeta; los totales por terminal del turno se mueven y el evento queda en la bitácora en la misma transacción.
**Independent Test**: escenarios 1–4 de US1 y 1–2 de US2 por API, contra Postgres real.

### Tests (antes del código)

- [X] T009 [US1] Test de integración en server/internal/integration/cambiar_terminal_test.go — regresión del caso real: cobrar $250 débito en Terminal A, cambiar a Terminal B como gerente; `TerminalCollectedForSession` baja $250 en Terminal A y sube $250 en Terminal B; el pago guarda id y nombre de Terminal B leídos de la base (no del request)
- [X] T010 [US1] En el mismo archivo: rechazos — sin motivo (400), pago en efectivo (400), destino archivado o de otra sucursal (400), destino igual (400, sin evento), pedido cancelado (409), pago de OTRO pedido por la ruta de este (404), cajero (403)
- [X] T011 [US1] En el mismo archivo: concurrencia con dos transacciones — A y B parten de Terminal A con `fromTerminalId` = Terminal A; A cambia a Terminal B, B recibe 409 «El pago ya cambió»; la bitácora tiene un solo evento
- [X] T012 [US2] En el mismo archivo: el evento `payment.terminal_changed` trae autor, pedido, pago, y en before/after terminal (id, nombre), monto, turno y sucursal, más el motivo; y si la inserción del evento falla (forzar acción inválida o permiso revocado en la prueba) el pago conserva su terminal
- [X] T013 [US1] Aislamiento: la lectura `GetPaymentForTerminalChange` bajo `appRoleStore` con `inTheThreeCases` (otra empresa, conexión reciclada, sin empresa) en server/internal/integration/cambiar_terminal_test.go
- [X] T014 [US1] Test de handler por el router real en server/internal/integration/cambiar_terminal_test.go (`TestChangeTerminalOverHTTP`; el arnés de órdenes vive ahí): ruta `POST /api/v1/orders/{id}/payments/{paymentId}/terminal` exige `payments.change_terminal`; body inválido o ids no numéricos → 400

### Implementación

- [X] T015 [US1] Consulta `GetPaymentForTerminalChange` en server/queries/terminals.sql: `where op.id = $1 and op.order_id = $2`, devuelve `payment_methods.kind`, `register_session_id`, `card_terminal_id`, `card_terminal_name`, `amount`, `tip_amount` y el `register_id` y `branch_id` de la sesión; `make sqlc`
- [X] T016 [US1] `OrdersService.ChangePaymentTerminal(ctx, cmd)` en server/internal/app/orders.go: `store.WithTx` → `GetOrderForUpdate` → `GetPaymentForTerminalChange` → `ActiveTerminalsOfRegisterBranch(register_id)` → `domain.ValidateTerminalChange` → `SetPaymentTerminal` con id y nombre de la fila leída → `recordAudit` con `payment.terminal_changed`; devuelve el `OrderView`
- [X] T017 [US1] Handler fino `ChangePaymentTerminal` en server/internal/httpapi/handlers_orders.go y ruta con `RequirePermission(h.permissions, domain.PermPaymentsChangeTerminal)` en server/internal/httpapi/router.go, junto a la de devolver pago
- [X] T018 [US1] `PaymentView` agrega `TerminalID *int64` y `TerminalName *string` (`terminalId`, `terminalName`) en server/internal/app/orders.go y la consulta que arma los pagos del pedido; test sobre JSON crudo de que salen `null` en un pago sin terminal
- [X] T019 [US1] Cliente `posApi.changePaymentTerminal(orderId, paymentId, { fromTerminalId, terminalId, reason })` y tipos de `terminalId`/`terminalName` en web/src/api/pos.ts

**Checkpoint**: corrección y bitácora funcionan por API; regresión del caso real en verde.

## Phase 4: User Story 3 — El detalle de venta muestra la terminal (P2)

**Goal**: sección «Pagos» en el detalle de venta y la hoja «Cambiar terminal» (maqueta A, plan.md › Maquetas; ajustes T2–T4, T6).
**Independent Test**: abrir una venta con dos pagos con tarjeta en terminales distintas y ver cada una; como gerente, cambiar una.

- [X] T020 [P] [US3] Test en web/src/features/sales/SaleDetailDialog.test.tsx (nuevo o junto a `SaleDetailDialog.historial.test.tsx`): la sección «Pagos» reemplaza «Medio de pago»; un renglón por pago con método, terminal («Sin terminal» si no hay) y monto; «Cambiar terminal» solo con `payments.change_terminal` y solo en tarjeta
- [X] T021 [P] [US3] Test en web/src/features/sales/CambiarTerminalSheet.test.tsx: monta cerrada y abre en el render siguiente; botones de 52 px con las terminales activas (la actual deshabilitada; con «Sin terminal», todas habilitadas); motivo prellenado «Se cobró en otra terminal» sin `autoFocus`; Confirmar deshabilitado sin terminal o con motivo < 3; manda `fromTerminalId`; un 409 muestra «El pago ya cambió» y recarga el pedido
- [X] T022 [US3] Implementar web/src/features/sales/CambiarTerminalSheet.tsx (hoja inferior con `maxH` en dvh y scroll interno; terminales de la sucursal vía la consulta de terminales existente en web/src/api/backoffice.ts)
- [X] T023 [US3] Implementar la sección «Pagos» en web/src/features/sales/SaleDetailDialog.tsx (renglón de 44 px, hoja montada dentro de `DialogContent`); al confirmar invalida `['orders', id]`, `['sales']` y las consultas de caja

## Phase 5: User Story 4 — Sin terminal predeterminada al cobrar (P2)

**Goal**: la terminal no llega elegida con 2+ activas; botones a la vista con ≤ 4 (ajuste T1); se limpia tras cada cobro (T7).
**Independent Test**: dos cobros seguidos con tarjeta en la misma tableta: la segunda vez no llega elegida.

- [X] T024 [P] [US4] Test de integración en server/internal/integration/cambiar_terminal_test.go: un usuario con terminal recordada cobra con tarjeta sin `terminalId` y 2 activas → 400; con 1 activa → cobra con ella; el cobro con `terminalId` ya no escribe la preferencia
- [X] T025 [US4] Quitar el paso de preferencia de `resolveTerminal` y la escritura de `prefDefaultTerminal` en el cobro, en server/internal/app/terminals.go y server/internal/app/orders.go
- [X] T026 [US4] Buscar consumidores de `SetUserDefault`/`UserDefault`/`posApi.defaultTerminal`; si solo los usa el cobro, borrar rutas, handlers, servicio y cliente (server/internal/httpapi/, server/internal/app/terminals.go, web/src/api/pos.ts, web/src/api/pos.terminal.test.ts)
- [X] T027 [P] [US4] Tests en web/src/shared/terminalEfectiva.test.ts (o el existente) y web/src/shared/TerminalDelCobro.test.tsx: sin `porOmision`; con 2–4 activas, botones de 52 px sin ninguno marcado; con 5+, `Picker`; con 1, elegida
- [X] T028 [US4] Implementar en web/src/shared/terminalEfectiva.ts y web/src/shared/TerminalDelCobro.tsx; quitar `porOmision` y la consulta `card-terminal-default` de web/src/shared/CobrarSheet.tsx, y corregir los comentarios que dicen «llega puesta»
- [X] T029 [US4] Test y fix en web/src/shared/CobrarSheet.split.test.tsx / CobrarSheet.tsx: en cuenta dividida la terminal elegida se limpia tras cada cobro

## Phase 6: Corte cerrado (FR-006)

- [X] T030 Test de integración en server/internal/integration/cambiar_terminal_test.go: turno cerrado con arqueo; cambiar un pago a una terminal SIN fila de arqueo (el caso real) → el corte devuelve ambas terminales con `expected` (al cerrar, $0 si no había fila) y `expectedNow`; turno con arqueo automático también; pedido con devolución con tarjeta en otro turno cerrado → ese turno también refleja el cambio
- [X] T031 Implementar en server/internal/app/backoffice.go (`terminalCounts`): unión de `SessionTerminalCounts` y `TerminalCollectedForSession`, con `ExpectedNow` en `TerminalCountView`
- [X] T032 [P] Test y aviso en web/src/features/backoffice/CashPage.tsx (vista del corte cerrado) / CashPage.test.tsx: una línea por terminal con `expectedNow ≠ expected`: «Terminal B: el esperado cambió $250.00 después del arqueo. Diferencia ahora: −$250.00»; tipos en web/src/api/backoffice.ts

## Phase 7: Polish

- [X] T033 Documentar en AGENTS.md (sección de cobro/terminales): la terminal ya no se recuerda por usuario y por qué; la corrección deja `payment.terminal_changed`; y el límite de atribución de devoluciones (plan D3)
- [X] T034 `cd server && go build ./... && go test ./...`; `cd web && bun run lint && bun run vitest run && bun run build`; `make ci-local`
- [X] T035 Capturas reales a 1024×600 del detalle de venta, la hoja y el cobro (memoria: validar UX como usuario nuevo)
- [X] T036 `/revision-de-codigo` (hook after_implement) y atender sus hallazgos

## Dependencies

- Phase 2 bloquea todo. US1+US2 (Phase 3) antes de US3 (usa el endpoint y los campos de terminal). US4 (Phase 5) y Phase 6 dependen solo de Phase 2 y pueden ir en paralelo con Phase 4.
- Dentro de cada fase: test → implementación.

## Parallel

- T002, T004, T006 juntos. T020 y T021 juntos. T024 y T027 juntos. Phase 5 y Phase 6 en paralelo con Phase 4.

## Implementation Strategy

MVP = Phases 1–3 (la corrección por API con bitácora resuelve el caso que hoy se hace a mano en la base). Después Phase 4 (pantalla), Phase 5 (quitar la causa) y Phase 6 (corte cerrado). No se sube nada sin autorización y la función sube junto con la bitácora.
