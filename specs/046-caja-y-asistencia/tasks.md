# Tasks: La caja exige la asistencia de quien la abre

**Input**: [plan.md](plan.md), [spec.md](spec.md), [research.md](research.md), [data-model.md](data-model.md), [contracts/cash-attendance.md](contracts/cash-attendance.md), [quickstart.md](quickstart.md)

**Tests**: obligatorios (constitución IV): cada test se escribe y se ve fallar antes de su código.
B# = bordes del plan. Sin migración. Las capturas de las pruebas van a `test-results/`, nunca al repo.

## Phase 1: Setup

- [X] T001 `codegraph sync`; levantar `make start` del worktree y restaurar datos (`make db-restaurar`); anotar `FRONTEND_PORT`/`BACKEND_PORT` de `.env.worktree.local` y si `gatobobah` (por slug) tiene la asistencia encendida en `business_settings`

## Phase 2: Foundational (bloquea a todas las historias)

- [X] T002 [P] Tests table-driven en server/internal/domain/cash_attendance_test.go para `CashOpenAttendance` y `CashCloseAttendance` según la tabla de data-model.md: apagada, no lleva asistencia, sin entrada, entrada de 16 h exactas (vigente, B1) y de 16 h + 1 min (B2, B3), sin `AuthorizedReason` (409), autorizado sin permiso con entrada ajena de más de 16 h (403 antes de mirar antigüedad, B9), cierre propio ≤ 16 h (409 `CLOCK_OUT_WITH_PIN`) y > 16 h (permitido, B10), motivo de espacios y de 201 caracteres (B11), `AuthorizedReason` con la regla sin aplicar (se ignora, B8); y los campos de `CashGate` (`Applies`, `Ready`, `Stale`, `IsSelf`, `CanAuthorize`) para cada caso
- [X] T003 Implementar server/internal/domain/cash_attendance.go: `CashAttendance`, `CashGate`, `CashCloseInput`, `CashCloseDecision`, sentinels `ErrCashClockInRequired`, `ErrCashOpenerClockedIn`, `ErrCashClockOutWithPIN` envueltos de `ErrConflict`; el mensaje de `ErrCashOpenerClockedIn` lleva el nombre de quien abrió (se envuelve con `fmt.Errorf("%w…", …)`, porque `httpapi.Error` responde con `err.Error()`), los otros dos son fijos, todos con el texto de contracts/cash-attendance.md; `CashAttendance.Active` sale de `users.is_active`; reusar `IsStaleOpen`
- [X] T004 [P] `PermCashCloseWithoutClockOut = "cash.close_without_clock_out"` en server/internal/domain/permission.go: en `AllPermissions`, en el rol gerente y con su `PermissionDeniedMessage`; caso en server/internal/domain/permission_test.go
- [X] T005 Consulta `LockUserAttendanceForCash` en server/queries/attendance.sql (`select id, name, is_active, tracks_attendance from users where id = $1 for share`, con el porqué del `for share` de research R2) y `make sqlc`
- [X] T006 [P] Test en server/internal/httpapi/respond_test.go y mapeo en server/internal/httpapi/respond.go: `ErrCashClockInRequired` → 409 `CLOCK_IN_REQUIRED`, `ErrCashOpenerClockedIn` → 409 `OPENER_NOT_CLOCKED_OUT`, `ErrCashClockOutWithPIN` → 409 `CLOCK_OUT_WITH_PIN`; el `message` de `OPENER_NOT_CLOCKED_OUT` es «Ana abrió esta caja y no ha marcado su salida», con el nombre y sin el prefijo del sentinel
- [X] T007 Tests de integración en server/internal/integration/cash_attendance_test.go para el estado del `GET` (`app.CashAttendanceState`) con caja cerrada (`stage=open`, persona = sesión) y abierta (`stage=close`, persona = quien abrió), bajo `appRoleStore` con `inTheThreeCases` (B17); persona que RLS no deja ver → `ErrNotFound`, nunca `applies=false` (B23)
- [X] T008 Implementar server/internal/app/cash_attendance.go: helper `cashAttendanceOf(ctx, q, userID)` que toma `LockUserAttendanceForCash` y lee ajustes y `GetOpenAttendance` **con el mismo `q`** (B22), y `CashAttendanceState(ctx, registerID, user, perms)` que arma `CashGate` con las funciones de T003
- [X] T009 Handler y ruta `GET /cash-sessions/attendance` en server/internal/httpapi/handlers_backoffice.go y server/internal/httpapi/router.go (dentro del grupo `/cash-sessions` existente); `registerId` ausente o malformado → 400 (B16) y caja que no existe → 404, con su test de handler
- [X] T010 [P] Web: tipo `CashAttendanceGate` y `cashAttendance(registerId)` en web/src/api/backoffice.ts; `'cash.close_without_clock_out'` en el tipo `Permission` de web/src/app/permissions.ts; mensajes de los tres códigos en web/src/api/mensajes.ts con su caso en web/src/api/mensajes.test.ts
- [X] T011 [P] Extraer el mapeo de errores de checar de web/src/shared/attendance/ClockView.tsx a web/src/shared/attendance/clockErrors.ts (429 con minutos, `ATTENDANCE_STATE_CHANGED`, sin conexión, «No coincide») con su test; `ClockView` lo usa sin cambiar textos (`ClockView.test.tsx` sigue verde)
- [X] T012 Tests en web/src/shared/attendance/ClockPinSheet.test.tsx: título «Marcar entrada · Ana» / «Marcar salida · Ana», botón verde para entrada y rojo para salida, llama a `attendanceApi.clock(userId, pin, expected)`, muestra el error de `clockErrors`, `PinPad` inactivo con la hoja cerrada o la tableta bloqueada, PIN limpio al cerrar; cancelar después de marcar no toca la caja (B14)
- [X] T013 Implementar web/src/shared/attendance/ClockPinSheet.tsx con el `Drawer` inferior del tema y el patrón de web/src/features/backoffice/ContadorDeEfectivo.tsx (plan.md › «Pantallas y maquetas», punto 3)

**Checkpoint**: la regla existe en `domain`, el `GET` responde y la hoja de PIN funciona; nada la usa todavía.

## Phase 3: US1 — Abrir caja pide haber marcado la entrada (P1) 🎯 MVP

**Prueba independiente**: con la asistencia encendida, alguien sin entrada no puede abrir; marca su entrada desde la tarjeta y abre. Maqueta: plan.md › «Maquetas», fila «Caja cerrada» (opción A).

- [X] T014 [US1] Tests de integración en server/internal/integration/cash_attendance_test.go: abrir sin entrada → 409 `CLOCK_IN_REQUIRED` y ningún turno escrito; con entrada de 2 h abre; con entrada de más de 16 h → 409, y tras `Clock` (que la deja «sin salida» y abre otra) abre (US1-4, B2); sesión de alguien que ya marcó salida → 409 (B15); B4 con dos conexiones reales: `Clock` de salida en otra conexión esperando el candado mientras se abre, y al revés; para cada caso, `ready` del `GET` coincide con el resultado de abrir (B18)
- [X] T015 [US1] En `OpenSession` de server/internal/app/backoffice.go, dentro de `WithTx` y antes de `q.OpenSession`: `cashAttendanceOf(ctx, q, userID)` y `domain.CashOpenAttendance`; nada de `s.store.QC` para estas lecturas (B22)
- [X] T016 [P] [US1] Tests en web/src/features/backoffice/AsistenciaDeCaja.test.tsx: con `applies && !ready` la tarjeta muestra «Para abrir la caja, primero marca tu entrada», «Marcar entrada» y «Contar el efectivo» deshabilitado; con `stale`, «Tu entrada anterior ya venció»; tras marcar, `Badge` «Entrada marcada …» y contar habilitado; si el `GET` falla, la tarjeta de hoy (B24) y el 409 del servidor se muestra con su mensaje
- [X] T017 [US1] Implementar la parte de apertura en web/src/features/backoffice/AsistenciaDeCaja.tsx y montarla en la tarjeta de caja cerrada de web/src/features/backoffice/CashPage.tsx:976 (consulta `['cash','attendance',registerId]`, invalidada tras marcar y tras abrir)

**Checkpoint**: US1 se puede subir sola.

## Phase 4: US2 — Cerrar caja pide la salida de quien la abrió (P1)

**Prueba independiente**: quien abrió marca su salida desde el aviso del cierre y la caja cierra. Maqueta: plan.md › «Maquetas», fila «Caja abierta» (opción A).

- [X] T018 [US2] Tests de integración en server/internal/integration/cash_attendance_test.go: cerrar con quien abrió dentro → 409 `OPENER_NOT_CLOCKED_OUT` y el turno sigue abierto; tras su salida cierra; salida a mediodía y entrada en la tarde → 409 (US2-4); entrada de más de 16 h → 409 (B3); asistencia encendida con la caja abierta (B5); «Lleva asistencia» apagado con la caja abierta (B6); persona dada de baja → 409 y el `GET` responde `person.active=false` (B7); dos cajas del mismo abridor, una salida libera las dos (B19); otra persona que lleva asistencia con su entrada abierta no bloquea el cierre (FR-012); B4 cierre contra `Clock` con dos conexiones; para cada caso, `ready` del `GET` coincide con el resultado de confirmar (B18)
- [X] T019 [US2] En `CloseSession` de server/internal/app/backoffice.go, después de `LockSessionForClose` y antes de `sinPedidosPendientes`: `cashAttendanceOf(ctx, q, sess.OpenedBy)` y `domain.CashCloseAttendance` con el `q` de la transacción
- [X] T020 [P] [US2] Tests en web/src/features/backoffice/AsistenciaDeCaja.test.tsx (cierre): con `applies && !ready` el aviso reemplaza todo el bloque de `TablaDelCierre` a `BotonCerrarCaja` (no está montado: ni «contar cajón», ni terminales, ni fondo, ni notas); título «Ana abrió esta caja y no ha marcado su salida»; texto con la hora y el día si no es de hoy; «Marcar salida de Ana» presente salvo con `stale` o con `person.active=false` (B7: el texto es «Ana ya no está activa: solo se puede cerrar con autorización»); tras marcar se monta el bloque con la franja «Salida de Ana marcada …»; sin `canAuthorize` el texto «Si ya se fue, pide a alguien con permiso que cierre la caja» y sin botón de autorizar; si el `GET` falla, el cierre de hoy (B24)
- [X] T021 [US2] Implementar la parte de cierre en web/src/features/backoffice/AsistenciaDeCaja.tsx y envolver el bloque de `TablaDelCierre` a `BotonCerrarCaja` de web/src/features/backoffice/CashPage.tsx (~:1052-1166) para que no se monte hasta resolver

**Checkpoint**: US1 + US2 cumplen la petición del dueño; falta la salida cuando quien abrió ya se fue.

## Phase 5: US3 — Cierre autorizado cuando quien abrió se fue (P2)

**Prueba independiente**: con Ana fuera, Gina cierra con motivo; la entrada de Ana queda «sin salida» y la bitácora muestra el cierre autorizado.

- [X] T022 [US3] Tests de integración en server/internal/integration/cash_attendance_test.go: con el permiso y motivo cierra, la entrada pasa a `missing_out` y hay un evento `cash.closed_without_opener_clock_out` con sujeto = quien abrió, `entryId` y motivo (US3-1); sin motivo → 400 y nada cambia (US3-2); sin el permiso → 403 (US3-3, FR-008); cierre propio ≤ 16 h → 409 y > 16 h permitido (US3-7, US3-8); `authorizedClose` sin necesidad → cierre normal sin evento (B8); falla posterior (pedido pendiente) → la entrada sigue `open` y sin evento (B12); la entrada se cerró en otra conexión antes de marcar → 409 sin evento (B21); dos cajas del mismo abridor → exactamente un evento autorizado (B19); dos cierres autorizados de la misma caja a la vez en dos conexiones → uno cierra, el otro ve la caja cerrada, y queda exactamente un evento (B13); al día siguiente, antes de 16 h desde la entrada, `Clock` registra entrada y no salida (US3-6); `TestEveryAuditActionIsProduced` en verde (B20)
- [X] T023 [US3] Acción `cash.closed_without_opener_clock_out` (`AuditGroupCash`) en server/internal/domain/audit.go y su etiqueta «Caja cerrada sin la salida de quien la abrió» en web/src/shared/audit/labels.ts (va aquí y no en la base: `TestEveryAuditActionIsProduced` truena mientras ninguna prueba la produzca); `authorizedClose.reason` en el cuerpo de `CloseCashSession` (server/internal/httpapi/handlers_backoffice.go) → `CierreCmd.AuthorizedReason`; en `CloseSession`, con `Authorized`, `MarkAttendanceMissingOut` exigiendo 1 fila (con 0, `ErrCashOpenerClockedIn`) y `recordAudit` con `SubjectUserID` = quien abrió, ambos con el `q` de la transacción
- [X] T024 [P] [US3] Tests en web/src/features/backoffice/CierreAutorizadoSheet.test.tsx: chips de 52 px «Se fue sin checar», «Salió por una emergencia», «Otro»; `Input` de una línea; con «Otro» la etiqueta es «¿Qué pasó?» y el botón espera texto; el `reason` que viaja es motivo + detalle; la franja verde dice «Se cerrará sin la salida de Ana» con el motivo y «Quitar autorización» la regresa al aviso; `cashClose` manda `authorizedClose`
- [X] T025 [US3] Implementar web/src/features/backoffice/CierreAutorizadoSheet.tsx con el patrón de web/src/features/backoffice/MotivoDeApertura.tsx (pie fijo, `maxH` 85dvh), el botón «Cerrar sin la salida de Ana» en web/src/features/backoffice/AsistenciaDeCaja.tsx solo con `canAuthorize`, y `authorizedClose` en `cashClose` de web/src/api/backoffice.ts

## Phase 6: US4 — Negocios y personas sin asistencia (P1)

**Prueba independiente**: con la asistencia apagada, o con alguien que no lleva asistencia, abrir y cerrar sin entradas.

- [X] T026 [US4] Tests de integración en server/internal/integration/cash_attendance_test.go: asistencia apagada → abre y cierra sin entradas y el `GET` dice `applies=false, ready=true`; persona con «Lleva asistencia» apagado → igual al abrir y al cerrar (US4-1, US4-2)
- [X] T027 [P] [US4] Test en web/src/features/backoffice/AsistenciaDeCaja.test.tsx: con `applies=false` la tarjeta y el cierre son idénticos a los de hoy (SC-004)
- [X] T028 [US4] Texto de ayuda en la configuración de Asistencia (junto a «Lleva asistencia», en web/src/features/admin/ScheduleEditor.tsx) redactado como instrucción: una cuenta que comparten varias personas para abrir caja necesita «Lleva asistencia» apagado; con su test de que el texto está detrás del icono de ayuda

## Phase 7: Polish

- [X] T029 Ayudante compartido web/e2e/asistencia.ts (marcar entrada y salida por `POST /attendance/clock` con el token de web/e2e/ambiente.ts y el PIN de la variable `E2E_PIN`; si la empresa tiene la asistencia encendida y falta `E2E_PIN`, falla con un mensaje que lo dice, no con un 409 de la caja); `E2E_PIN` va también en la línea de variables de `bun run e2e` en AGENTS.md; usarlo en web/e2e/contar-el-cajon.spec.ts, arqueo-ciego.spec.ts, caja-propinas-tarjeta.spec.ts y presupuesto-del-cierre.spec.ts cuando la empresa tenga la asistencia encendida
- [X] T030 E2E nuevo web/e2e/caja-y-asistencia.spec.ts a 1024×600 con `hasTouch` contra el `make start` local: escenarios Q1, Q2, Q5, Q6, Q7 y Q8 de quickstart.md; las personas (Beto cajero, Gina gerente, ambas con «Lleva asistencia») las crea la prueba en su arranque por la API de usuarios, con nombres marcados como de prueba y un PIN generado en la prueba que no se escribe en ningún archivo, y las da de baja al terminar; solo corre contra la base local; capturas a la carpeta de una variable de entorno con `test-results/` por omisión; cerrar los pedidos que cree
- [X] T031 Gates: `cd server && go build ./... && go test ./... && make test-integration`; `cd web && bun run lint && bun run vitest run && bun run build`
- [X] T032 Vuelta completa de quickstart.md (Q1 a Q12) contra el `make start` y contra `bun run build && bun run preview`; al terminar, pedidos entregados y cobrados y caja cerrada
- [X] T033 Prueba con usuario sin contexto (subagente, una fonda o taquería) sobre abrir y cerrar caja con asistencia; lo que encuentre va a plan.md › «Al construir»
- [X] T034 AGENTS.md: sección «Caja y asistencia (spec 046)» con lo que no se ve (candado `for share` y por qué, todas las lecturas con el `q` de la transacción, `MarkAttendanceMissingOut` exige 1 fila, la hoja reusa `POST /attendance/clock`, el cierre autorizado deja `missing_out`)

## Dependencies

- Phase 2 bloquea todo. Dentro de ella: T002 → T003; T005 → T008; T003 + T008 → T009; T011 → T012 → T013.
- US1 y US2 dependen solo de Phase 2 y se pueden hacer en paralelo, salvo que las dos tocan web/src/features/backoffice/AsistenciaDeCaja.tsx y CashPage.tsx (T017 y T021 en serie).
- US3 depende de US2 (el aviso del cierre y la decisión de `CloseSession`).
- US4 depende de Phase 2; sus tests confirman lo que T003 ya decide.
- Polish al final; T029 antes de correr la suite e2e de caja.

## Parallel

- T002, T004, T006, T010, T011 a la vez (archivos distintos).
- T016 y T020 (tests de front) mientras corren T014/T015 y T018/T019.
- T024 y T026/T027 en paralelo con T022.

## Implementation Strategy

MVP = Phase 2 + US1 + US2 + US4: la petición del dueño completa, con la salida solo por PIN. US3 entra antes de subir: sin ella, un olvido de quien abrió deja la caja sin poder cerrarse esa noche. Se sube junto (una sola subida, sin migración).
