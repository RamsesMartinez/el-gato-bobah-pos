# Tasks: Asistencia del personal y auditoría de acciones

**Input**: [plan.md](plan.md), [spec.md](spec.md), [data-model.md](data-model.md),
[contracts/api.md](contracts/api.md), [research.md](research.md), [quickstart.md](quickstart.md)

**Tests**: obligatorios (constitución IV, TDD). Cada test se escribe antes que su código y se ve
fallar por la razón correcta. Integración bajo `appRoleStore` + `inTheThreeCases`
(`server/internal/integration/`).

Rutas relativas a la raíz del repo. Backend: `cd server && go build ./... && go test ./...` al cerrar
cada fase.

## Phase 1: Setup

- [X] T001 Crear la migración vacía con `make migrate-new name=attendance_and_audit` → `server/migrations/0089_attendance_and_audit.sql`, y los archivos de queries vacíos `server/queries/attendance.sql` y `server/queries/audit.sql`

## Phase 2: Foundational (bloquea todas las historias)

**Origen del request, auditoría y esquema. Sin esto ninguna historia escribe su evento.**

### Tests primero

- [X] T002 [P] Test de la migración en `server/internal/integration/migration_0089_test.go`, con el patrón y la justificación de `migracion_conteo_de_efectivo_test.go`: la 0089 **no rellena datos** (tablas nuevas vacías y columnas con default constante), así que no exige respaldo restaurado; el comentario de cabecera lo dice y nombra por qué. Si al escribirla aparece un `update` sobre filas existentes, la excepción deja de aplicar y el test pasa a correr contra un respaldo. Dos empresas con `makeCompany` (una con dos sucursales); restricciones con `exigeViolacionDeRestriccion`. Casos: las 5 tablas nuevas existen con RLS y política `nullif`; `missing_out` con `clock_out_at` → 23514; `clock_out_at <= clock_in_at` → 23514; `update` de `clock_in_at` sin `corrected` → error; `update` de `user_id`/`source` como `gatobobah_app` → 42501; Down sin filas funciona y con una fila en `audit_events` se niega
- [X] T003 [P] Test append-only en `server/internal/integration/audit_test.go`: como `gatobobah_app`, `update`/`delete` de `audit_events` → 42501; como owner, `delete` de un evento suelto → error y `truncate` → error; como owner, `delete from companies` de una empresa con eventos, entradas, correcciones y horarios funciona (FK `no action` + trigger que deja pasar el cascade)
- [X] T004 [P] Unitarios en `server/internal/domain/origin_test.go`: `NormalizeOrigin` acepta clave `^[0-9a-f]{32}$` y deja nula cualquier otra (vacía, 31 caracteres, mayúsculas, inyección); recorta la descripción a 300 bytes y descarta llaves desconocidas; nunca devuelve error (una clave mala no rechaza la acción)
- [X] T005 [P] Unitarios en `server/internal/domain/audit_test.go`: `AllAuditActions()` contiene exactamente las acciones de data-model.md; `ValidAuditAction` rechaza las demás; `AuditEvent.Validate` exige `actor_user_id` sólo si `actor_kind = user`
- [X] T006 [P] Unitarios en `server/internal/domain/permission_test.go`: los 5 permisos nuevos (`attendance.configure`, `attendance.report`, `attendance.correct_others`, `attendance.correct_own`, `audit.view`) según la tabla R8 de research.md; cajero/mesero no tiene ninguno

### Implementación

- [X] T007 Escribir `server/migrations/0089_attendance_and_audit.sql` según data-model.md: columnas en `business_settings` y `users`; tablas `attendance_entries`, `attendance_corrections`, `work_schedule_versions`, `work_schedule_days`, `audit_events` con el patrón de 0088 (default de `company_id`, `*_tenant_key`, RLS, FK compuestas `no action` a users/branches), checks, índices, grants por tabla y por columna, `usage` de secuencias, triggers (append-only con excepción de cascade, `before truncate`, hora corregida en `attendance_entries`) y Down que se niega con filas y suelta triggers/funciones antes de los `drop`
- [X] T008 Agregar las 5 tablas a lo que recorre `TestEveryCompanyTableIsIsolated` en `server/internal/integration/rls_all_tables_test.go` si la lista no es automática
- [X] T009 [P] `server/internal/domain/origin.go`: tipo `RequestOrigin{IP, Device json, DeviceKey *string}` y `NormalizeOrigin`
- [X] T010 [P] `server/internal/domain/audit.go`: `AuditAction` (lista blanca), `AuditEvent`, `ActorKind`, sentinels nuevos de research.md en `server/internal/domain/errors.go`
- [X] T011 [P] Agregar los 5 permisos a `server/internal/domain/permission.go` (`AllPermissions`, `PermissionsFor`, `PermissionDeniedMessage`) y su espejo en `web/src/app/permissions.ts`
- [X] T012 Queries `InsertAuditEvent` en `server/queries/audit.sql`; `make sqlc`
- [X] T013 `server/internal/httpapi/request_origin.go`: middleware `withRequestOrigin` que lee `rateKeyIP(r, behindProxy)`, `X-Device-Key`, `X-Device-Info`, normaliza con `domain.NormalizeOrigin` y lo pone en el contexto; cablearlo en `server/internal/httpapi/router.go` para todo `/api/v1` del POS. Test en `server/internal/httpapi/request_origin_test.go`: detrás del proxy toma el último X-Forwarded-For; sin proxy ignora el header
- [X] T014 `server/internal/app/audit.go`: `OriginFrom(ctx)` y `recordAudit(ctx, q *db.Queries, ev domain.AuditEvent) error` (rellena actor y origen del contexto; montos con `Round2`)
- [X] T015 Agregar `X-Device-Key` y `X-Device-Info` a cada request en `web/src/api/origin.ts` y `web/src/api/client.ts`: clave de 16 bytes con `crypto.getRandomValues` guardada en `localStorage` `egb:device:v1` (con try/catch: si falla, se manda sin clave); descripción con `navigator.userAgentData.getHighEntropyValues(['platformVersion','model'])` cuando existe. Test en `web/src/api/origin.test.ts`: la clave se genera una vez y se reusa; con `localStorage` que lanza, el request sale igual

**Checkpoint**: `go test ./...` verde; la migración corre arriba y abajo.

## Phase 3: User Story 1 - Checar entrada y salida en la tableta (P1) MVP

**Goal**: el empleado checa con su usuario y PIN sin cambiar la sesión de la estación.

**Independent Test**: quickstart.md pasos 2–3.

### Tests primero

- [X] T016 [P] [US1] Unitarios en `server/internal/domain/attendance_test.go`: `NextClock` decide `in`/`out` según la entrada abierta; `expected` distinto → `ErrAttendanceStateChanged`; entrada abierta de más de 16 h → la vieja pasa a `missing_out` y se abre otra; `business_date` de una entrada a las 23:50 y de una salida a la 1:00 es el día de la entrada
- [X] T017 [P] [US1] Integración en `server/internal/integration/attendance_test.go`: checar entrada y salida; con la asistencia apagada → `ErrAttendanceDisabled`; usuario con `tracks_attendance = false` → no está en el roster y no puede checar; usuario sin PIN → `ErrNoPIN`; dos checadas simultáneas de la misma persona dejan una sola entrada abierta; `branch_id` se llena con una sucursal y queda nulo con dos; se escribe `attendance.clock_in`/`clock_out` en `audit_events` en la misma transacción; todo con `inTheThreeCases`
- [X] T018 [P] [US1] Test de handler en `server/internal/httpapi/handlers_attendance_test.go`: PIN incorrecto cuenta en `authFails` con la llave `pin:<userId>` (la del desbloqueo) y al pasar el máximo devuelve 429 con `retryAfterSeconds`, y el desbloqueo de ese usuario también queda bloqueado; checar no emite tokens ni cambia la cookie; la respuesta nunca trae IP ni aparato; el roster no dice quién no tiene PIN fuera de `hasPin`

### Implementación

- [X] T019 [P] [US1] `server/internal/domain/attendance.go`: `NextClock`, regla de 16 h, `BusinessDateOf` (con `domain.BusinessDate`)
- [X] T020 [US1] Queries en `server/queries/attendance.sql`: `LockUserForAttendance` (`select … from users … for update`), `GetOpenAttendance`, `InsertAttendanceEntry`, `CloseAttendanceEntry`, `MarkMissingOut`, `ListAttendanceRoster`; `GetAttendanceSettings` en el query de `business_settings`; `make sqlc`
- [X] T021 [US1] `server/internal/app/attendance.go`: `AttendanceService.Roster` y `Clock(ctx, userID, pin, expected)`: dentro de `WithTx`, candado por persona, verificación del PIN con la rutina de `relevar` en `server/internal/app/auth.go` (extraída a una función compartida, sin emitir tokens), escritura y `recordAudit`
- [X] T022 [US1] `server/internal/httpapi/handlers_attendance.go`: `GET /attendance/roster`, `POST /attendance/clock` con `rateLimit` por IP y `authFails` `pin:<userId>`; rutas en `server/internal/httpapi/router.go` bajo `RequireAuth` + `WithTenant`; mapeo de sentinels en `server/internal/httpapi/respond.go`
- [X] T023 [US1] (Resuelto con `enabled` en `GET /attendance/roster`, sin tocar la sesión) `GET /me` agrega `attendanceEnabled` en `server/internal/httpapi/handlers.go` (o donde se arma la sesión) y en el store de sesión del front
- [X] T024 [P] [US1] Extraer el teclado de `web/src/features/auth/LockScreen.tsx` a `web/src/components/PinPad.tsx` con `actionLabel`/`onAction` en la tecla inferior derecha; `PinPad` es el único dueño del oyente `keydown` (el de `LockScreen.tsx:85-116` se mueve ahí), conservando el arreglo del teclado físico. Test: con dos `PinPad` montados solo el activo recibe dígitos; `LockScreen` lo usa sin cambiar su comportamiento (sus tests siguen verdes)
- [X] T025 [US1] Pantalla según la fila «Checar» de «Pantallas y maquetas» del plan (maqueta C) en `web/src/shared/attendance/ClockView.tsx`: columnas «Por entrar» y «Dentro desde…», cada una con scroll propio; tocar a alguien abre la hoja que vuelve a pedir el roster y muestra `PinPad` con «Entrada»/«Salida» y «No soy yo»; sin PIN, deshabilitado con «Pide tu PIN a tu gerente»; error genérico «No coincide» y los minutos solo con 429; hoja con `maxH` en dvh; manda `expected`; 409 → muestra lo que pasó; 429 → «Intenta de nuevo en N min»; confirmación con la cifra del servidor que se cierra a los 3 s. Test en `web/src/features/attendance/ClockView.test.tsx` contra esos casos
- [X] T026 [US1] Ítem «Asistencia» en `web/src/app/nav.ts` después de «Reportes» (actualizar el comentario), ruta `/asistencia` en `web/src/App.tsx` y `web/src/app/roles.ts` (todos los roles), visible sólo con `attendanceEnabled`; botón «Registrar asistencia» en `web/src/features/auth/LockScreen.tsx` que muestra `ClockView` encima del bloqueo sin desbloquear, desmontando el teclado del bloqueo y reiniciando su persona y PIN; `ClockView` y su hoja con `role="dialog"`. Test: con Checar abierto encima del bloqueo, un Enter del teclado físico manda un solo intento
- [X] T027 [US1] Instrumentar la pantalla nueva en los tres lugares: `pantallasMedibles` y `rolesPorPantalla` en `server/internal/domain/uso.go`, `PANTALLAS` en `web/src/app/rutas-medidas.ts`, y la etiqueta en `web/src/consola/etiquetas-de-uso.ts`

**Checkpoint**: se checa desde el riel y desde la pantalla de bloqueo.

## Phase 4: User Story 2 - Saber desde dónde se checó (P1)

**Goal**: cada registro guarda IP, descripción y clave del aparato, visibles sólo con permiso.

**Independent Test**: quickstart.md paso 4.

- [X] T028 [P] [US2] Integración en `server/internal/integration/attendance_test.go`: una entrada y su salida desde dos orígenes distintos guardan `in_*` y `out_*` por separado; una clave malformada se guarda nula y la checada se acepta
- [X] T029 [US2] Guardar `in_ip/in_device/in_device_key` y `out_*` desde `OriginFrom(ctx)` en `AttendanceService.Clock` (`server/internal/app/attendance.go`)
- [X] T030 [US2] Verificar en `server/internal/httpapi/handlers_attendance_test.go` que ninguna respuesta de rol cajero/mesero incluye `ip`, `device` ni `deviceKey`

## Phase 5: User Story 3 - Historial de un pedido (P1)

**Goal**: cada acción sobre un pedido deja su evento; admin/gerente ven el historial.

**Independent Test**: quickstart.md paso 5.

### Tests primero

- [X] T031 [P] [US3] Integración en `server/internal/integration/audit_orders_test.go`, un caso por acción: `order.created`, `order.lines_added`, `order.line_removed`, `order.lines_moved` (un evento en cada pedido, con el otro en `after`), `order.delivered`, `order.cancelled`, `order.discount_set`, `order.platform_ref_set`, `payment.charged` (método, terminal, referencia), `payment.voided`, `payment.refunded`, `stock.written_off`; cada uno con autor, `branch_id` del pedido, `before`/`after`; un cobro que falla no deja evento; un pedido de plataforma aceptado deja `actor_kind = platform`
- [X] T032 [P] [US3] Test `TestEveryAuditActionIsProduced` en `server/internal/integration/audit_coverage_test.go`: recorre `domain.AllAuditActions()` y falla nombrando la acción que ningún test de integración produjo

### Implementación

- [X] T033 [US3] `recordAudit` dentro del `WithTx` de `Create`, `SetPlatformRef`, `SetDiscount`, `SetStatus`, `Cancel`, `Refund`, `AddLines`, `DeliverLine`, `DeliverAll`, `Charge` en `server/internal/app/orders.go`
- [X] T034 [US3] `recordAudit` en `Devolver`, `CancelarConDevolucion`, `WriteOff`, `CancelarRenglon`, `RemovePieces`, `VoidPayment`, `CancelPending` en `server/internal/app/devolucion.go` y en `MoveLines` de `server/internal/app/move_lines.go`
- [X] T035 [US3] Aceptación de pedidos de plataforma con actor `platform` en `server/internal/app/pedidos_de_plataforma.go`
- [X] T036 [US3] Query `ListOrderHistory` en `server/queries/audit.sql`, `AuditService.OrderHistory` en `server/internal/app/audit.go`, `GET /orders/{id}/history` con `RequirePermission(audit.view)` en `server/internal/httpapi/handlers_audit.go` y `router.go`; test de aislamiento con `inTheThreeCases`
- [X] T037 [US3] `web/src/shared/audit/labels.ts`: mapa acción → texto en español y `resumirCambio(before, after)` («Precio: $80 → $85»); test en `web/src/shared/audit/labels.test.ts` que falla si una acción de la lista no tiene etiqueta
- [X] T038 [US3] Según la fila «Historial del pedido» del plan (maqueta C): pestañas **Productos · Pagos · Historial** en `web/src/features/sales/SaleDetailDialog.tsx` (el detalle de un pedido en Ventas; el tablero de Pedidos no tiene vista de detalle), que pasa a abrirse por `orderId` y usa la carga que ya hace con `posApi.order`; fila de pestañas fija fuera del scroll y panel con `minH={0}`; «Ver historial del pedido» de la bitácora lo abre por id. Las pestañas aparecen solo con `can('audit.view')`; `web/src/shared/audit/OrderHistoryTimeline.tsx` con la línea de tiempo y scroll propio. Test: sin permiso el pedido se ve igual que hoy (sin fila de pestañas); con permiso, «Historial» muestra los eventos en orden

## Phase 6: User Story 4 - Bitácora general (P2)

**Goal**: todas las acciones de FR-011 auditadas y consultables con filtros.

**Independent Test**: quickstart.md paso 6.

### Tests primero

- [X] T039 [P] [US4] Integración en `server/internal/integration/audit_backoffice_test.go`: `cash.opened`, `cash.closed`, `cash.counted`, `expense.created`, `expense.paid`, `expense.cancelled`, `tip.paid_out`, `price.set`, `price.deleted`, `price.synced` (actor `system`, sin sucursal), cada uno con su `before`/`after`
- [X] T040 [P] [US4] Integración de la lista: filtros por rango, persona, acción y clave; `CountAuditEvents` devuelve lo mismo que la lista sin paginar (mismo `where`); un `from` malformado, una acción desconocida o un `size` fuera de {25, 50} → `ErrValidation`; aislamiento con `inTheThreeCases`

### Implementación

- [X] T041 [US4] `recordAudit` en `OpenSession`, `CloseSession`, `saveTerminalCounts`, `RecordCashMovement`, `CorrectCashOut` de `server/internal/app/backoffice.go`; `CreateExpense`, `ReceiveExpense`, `AddExpensePayment`, `CancelExpense` de `server/internal/app/expenses.go`; `Payout` de `server/internal/app/tips.go`
- [X] T042 [US4] Envolver en `WithTx` y agregar `recordAudit` a `SetProductPrice`, `DeleteProductPrice`, `SetOptionDelta`, `DeleteOptionDelta` en `server/internal/app/platform_prices.go`, y a `syncPrices` en `server/internal/app/pairing.go` (actor `system`)
- [X] T043 [US4] `domain.AuditFilter.Validate` y `domain.AuditGroupOf` en `server/internal/domain/audit.go` (con unitarios en `audit_test.go`: toda acción tiene grupo; `group` desconocido → `ErrValidation`); `ListAuditEvents`/`CountAuditEvents`/`CountAuditEventsByGroup` con `sqlc.narg` y el mismo `where` en `server/queries/audit.sql` (los conteos por grupo ignoran el filtro `group`); `GET /admin/audit` con `RequirePermission(audit.view)`; test de integración de que la suma de `countsByGroup` es igual a `total` sin filtro de grupo
- [X] T044 [US4] Pestaña «Bitácora» en `ReportsPage` (`web/src/features/...` que monta `/reportes`), componente `web/src/features/backoffice/AuditLogTab.tsx` según la fila «Bitácora» del plan (maqueta C): fecha y persona (`Picker`) visibles, cinco contadores-botón por grupo que filtran al tocarlos, una línea por evento con `labels.ts`, paginación con botones de 44 px, `Page fill` y lista con scroll propio; visible sólo con `can('audit.view')`

## Phase 7: User Story 5 - Banco de horas (P2)

**Goal**: ajustes por empresa, horarios por versión y reporte.

**Independent Test**: quickstart.md paso 7.

### Tests primero

- [X] T045 [P] [US5] Unitarios table-driven en `server/internal/domain/attendance_test.go` para `AttendanceDay`: medición apagada → sólo horas; tolerancia 10 con entrada 9:08 → 0 y con 9:12 → 12; 9:10–17:40 sin tolerancia → 10 retardo, 40 extra, 8 h 30; turno que cruza medianoche; horario `end < start`; sin horario → `noSchedule`; `missing_out` no suma; versión nueva sin sábado → el sábado es libre; día de cambio de versión usa la nueva
- [X] T046 [P] [US5] Integración: ajustes por empresa (0–60, fuera de rango → `ErrValidation`), versión de horario con sus días en una transacción, reporte por rango con totales; `attendance.settings_changed` y `attendance.schedule_set` auditados; aislamiento con `inTheThreeCases`

### Implementación

- [X] T047 [US5] `AttendanceDay` y validación de horario (`weekday`, `end <> start`) en `server/internal/domain/attendance.go`
- [X] T048 [US5] Queries de ajustes, versiones, días y reporte en `server/queries/attendance.sql`; servicio en `server/internal/app/attendance.go` (`Settings`, `SetSettings`, `Schedules`, `SetSchedule`, `Report`); endpoints de contracts/api.md con `attendance.configure` y `attendance.report`
- [X] T049 [US5] `tracksAttendance` en `PATCH /admin/users/{id}` (`server/internal/app/users.go`) con `user.tracks_attendance_changed`
- [X] T050 [US5] Sección «Asistencia» en `web/src/features/admin/BusinessSettingsPage.tsx`: activar, medir retardos y extra, tolerancias con `Picker`
- [X] T051 [US5] Horario y casilla «Lleva asistencia» en la ficha de `web/src/features/admin/EmployeesPage.tsx`, componente `web/src/features/admin/ScheduleEditor.tsx` según la fila «Horario» del plan (maqueta A, ronda 2): una fila por día con hasta dos turnos (`shift_no` 1 y 2), horas con `Picker` de 15 min sobre 24 h (nunca `type=time`), «+1 día» cuando la salida es menor que la entrada, «Día libre», «+ Turno», «Copiar a lunes–viernes», «Copiar de otra persona» (`Picker`; copia los días al formulario, no liga), vigencia con botón de fecha. Test: copiar de otra persona y guardar crea una versión propia; un segundo turno se guarda con `shift_no = 2`
- [X] T052 [US5] Pestaña «Reporte» en `/asistencia`, `web/src/features/attendance/ReportTab.tsx` según la fila «Reporte» del plan (maquetas A+B+C): filtros y selector de tres segmentos Por persona / Día por día / Semana en un renglón (segmentos de 44 px; por omisión «Por persona», recordado en `localStorage` con try/catch; «Semana» desactivada con rango > 7 días), las tres vistas desde la misma respuesta, con scroll propio y `overflowX=auto`, renglón ≥ 44 px con estado en texto, columnas de retardo y extra sólo con la medición, horas en la zona del negocio; hoja de detalle con IP, aparato y clave; visible sólo con `can('attendance.report')`

## Phase 8: User Story 6 - Corregir un registro (P3)

**Goal**: corrección y captura a mano con motivo, por permiso.

**Independent Test**: quickstart.md paso 8.

### Tests primero

- [X] T053 [P] [US6] Unitarios en `server/internal/domain/attendance_test.go`: motivo de 3–200 caracteres; `CanCorrect(actor, subject, perms)` con los cuatro casos de permisos; solape contra entradas existentes (una abierta cuenta como `+∞`)
- [X] T054 [P] [US6] Integración: completar una salida faltante → `corrected`, fila en `attendance_corrections` con el valor original y `attendance.corrected` auditado; gerente sobre sí mismo → `ErrForbidden`; admin sobre sí mismo → ok; captura a mano → `source = manual`; captura que se encima → `ErrAttendanceOverlap`; dos capturas simultáneas del mismo turno → una falla (candado por persona); corregir un registro de otra empresa falla en los tres casos de `inTheThreeCases`

### Implementación

- [X] T055 [US6] `ValidateCorrection`, `CanCorrect`, `Overlaps` en `server/internal/domain/attendance.go`
- [X] T056 [US6] `Correct` y `ManualEntry` en `server/internal/app/attendance.go` (candado por persona, `recordAudit` con `subject_user_id`); endpoints de contracts/api.md
- [X] T057 [US6] Acciones «Corregir» y «Capturar a mano» en la hoja de detalle de `ReportTab`, con motivo en hoja inferior, sólo con el permiso que corresponda; test en `web/src/features/attendance/ReportTab.test.tsx`

## Phase 9: Polish

- [X] T058 [P] Anotar en `docs/security-owasp.md` que `clientIP` confía en X-Forwarded-For siempre y sólo sirve para logs, y que la evidencia usa `rateKeyIP`
- [X] T059 [P] `AGENTS.md`: sección «Asistencia y auditoría» con lo que no se ve — todo camino nuevo que mueva dinero llama `recordAudit` en su transacción y agrega su acción a la lista blanca; `attendance_entries.branch_id` queda nulo con varias sucursales y no se rellena; la clave del aparato es pista, no prueba
- [X] T060 Captura real del riel y de las pantallas nuevas a 1024×600 (incluido el renglón de filtros del reporte: si baja a dos renglones, medir cuántas filas quedan visibles) como admin y como cajero (ningún ítem oculto sin indicio; el paso 2 de checar cabe), y revisión de un usuario nuevo (memoria «Validar UX como usuario nuevo»)
- [X] T061 e2e en `web/e2e/asistencia-y-auditoria.spec.ts` usando `tokenDeApi` de `web/e2e/ambiente.ts`: checar entrada/salida en menos de 15 s desde que se abre la pantalla (SC-001), historial de un pedido creado y cobrado; los pedidos se cobran en el teardown
- [ ] T062 `make ci-local` verde

## Dependencies

- Phase 2 bloquea todo.
- US1 → US2 (amplía `Clock`) → US5 y US6 (necesitan registros).
- US3 y US4 dependen sólo de Phase 2; US4 reusa `labels.ts` de US3.
- US6 depende de la hoja de detalle de US5 (T052).

## Parallel Examples

- Phase 2: T002–T006 juntos; luego T009, T010, T011 juntos.
- US1: T016, T017, T018 juntos; T019 y T024 en paralelo con T020.
- US3 y US4 pueden avanzar en paralelo con US1 una vez cerrada la Phase 2.

## Implementation Strategy

- **MVP**: Phase 2 + US1 + US2 + US3 — se deja el grupo de WhatsApp y los pedidos ya tienen historial desde el primer despliegue (lo que no se audita desde el inicio no se recupera).
- Después US4 (bitácora), US5 (banco de horas, apagado por omisión) y US6.
- Cada fase cierra con `go build ./... && go test ./...` y vitest.

## Phase 10: Convergence

- [X] T063 CRITICAL: escribir la prueba del límite por IP de `POST /attendance/clock` en `server/internal/integration/attendance_ratelimit_test.go` —el router real necesita base— (agotar `authIPs` desde una IP y esperar 429 con el router real) per Constitution V (missing)
- [X] T064 Mostrar el traslado en `web/src/shared/audit/OrderHistoryTimeline.tsx`: «Pasado al pedido N» / «Viene del pedido N» con `toOrderId`/`fromOrderId` del evento, y abrir el historial del otro pedido al tocarlo, con su prueba per US3/AC2 (partial)
- [X] T065 Ofrecer en el filtro «Aparato» de `web/src/features/backoffice/AuditLogTab.tsx` las claves de todo el rango y no solo de la página (consulta `ListAuditDeviceKeys` con el mismo `where` en `server/queries/audit.sql`, devuelta como `deviceKeys` en la misma respuesta de la bitácora) per FR-016 (partial)
- [X] T066 Decir «Sin conexión» en `web/src/shared/attendance/ClockView.tsx` cuando no se puede cargar la lista para checar, con su prueba per Edge Cases: Sin red (partial)
- [X] T067 Al abrir el otro pedido desde un traslado, llevar la hoja al inicio en `web/src/shared/audit/OrderHistoryTimeline.tsx`, con su prueba (revisión de tableta)
- [X] T068 Distinguir en `web/src/shared/attendance/ClockView.tsx` «Sin conexión» (sin respuesta) de «No se pudo cargar la lista» (el servidor respondió con error), con su prueba (revisión de tableta)
- [X] T069 Medir la carga de la bitácora con el rango más ancho que acepta (un año, 730,000 eventos, más otra empresa igual) con y sin filtros, y dejar el techo de 1.5 s en `server/internal/integration/audit_volume_test.go` (revisión de base de datos)
