# Data Model: La caja exige la asistencia de quien la abre

**Sin migración.** Todo lo que la regla lee ya existe (0089 y anteriores). Lo nuevo vive en código:
un permiso, una acción de bitácora y dos funciones de `domain`.

## Lo que se lee (existente)

| Dato | Dónde | Para qué |
|---|---|---|
| Asistencia encendida | `business_settings.attendance_enabled` (default `false`) | FR-003: apagada, la regla no aplica |
| «Lleva asistencia» | `users.tracks_attendance` | FR-003 |
| Entrada abierta y desde cuándo | `attendance_entries` con `status = 'open'` (`GetOpenAttendance`) | FR-001 (vigente = 16 h o menos, `domain.StaleOpenAfter`) y FR-002 (cualquiera) |
| Quién abrió el turno | `register_sessions.opened_by` | FR-002, FR-012 |
| Quién abre | la persona de la sesión (`userFrom(ctx)`) | FR-001 |

## Lo que se escribe

| Cuándo | Qué | Cómo |
|---|---|---|
| Marcar entrada o salida desde la hoja | Registro de asistencia y su evento | `POST /attendance/clock` sin cambios (R3) |
| Cierre autorizado | `attendance_entries.status` `open → missing_out` de la entrada de quien abrió | `MarkAttendanceMissingOut` (existe), dentro de la transacción del cierre |
| Cierre autorizado | Evento `cash.closed_without_opener_clock_out` | `recordAudit` en la misma transacción, con `SubjectUserID` = quien abrió |

## Tipos nuevos en `domain` (puros)

### `CashAttendance` — lo que se sabe de una persona

| Campo | Tipo | Nota |
|---|---|---|
| `Enabled` | bool | asistencia del negocio |
| `Tracks` | bool | «Lleva asistencia» de la persona |
| `OpenSince` | `*time.Time` | entrada abierta, nil si no hay |
| `Active` | bool | `users.is_active`; no cambia la regla, solo si se ofrece el PIN (B7) |

### `CashOpenAttendance(a, now) CashGate`

| Caso | `Applies` | `Ready` | Error al confirmar |
|---|---|---|---|
| Apagada o no lleva asistencia | false | true | — |
| Entrada abierta ≤ 16 h | true | true | — |
| Sin entrada, o entrada > 16 h | true | false | `ErrCashClockInRequired` → 409 `CLOCK_IN_REQUIRED` |

### `CashCloseAttendance(in, now) (CashCloseDecision, error)`

Entrada: `Opener CashAttendance`, `OpenerID`, `CloserID`, `Perms`, `AuthorizedReason *string`.

| Caso | Resultado |
|---|---|
| Apagada, no lleva asistencia, o sin entrada abierta | cierre normal; `AuthorizedReason` se ignora |
| Entrada abierta, sin `AuthorizedReason` | `ErrCashOpenerClockedIn` → 409 `OPENER_NOT_CLOCKED_OUT` |
| Con `AuthorizedReason`, sin `PermCashCloseWithoutClockOut` | `ErrForbidden` con `PermissionDeniedMessage` → 403 |
| Con permiso, `CloserID == OpenerID` y entrada ≤ 16 h | `ErrCashClockOutWithPIN` → 409 `CLOCK_OUT_WITH_PIN` |
| Con permiso, motivo de 3 a 200 caracteres | `{Authorized: true, MarkMissingOut: true, Reason}` |
| Motivo vacío o largo | `ErrValidation` → 400 |

Orden de las reglas: permiso **antes** que la del cierre propio, para que quien no tiene permiso
reciba el mismo 403 sea cual sea la antigüedad de la entrada. No oculta la antigüedad: el `GET` y la
pantalla de Asistencia ya la muestran a la misma empresa.

`CashGate` (lo que pinta la hoja, mismo predicado): `Applies`, `Ready`, `Stale`, `IsSelf`,
`CanAuthorize`.

## Permiso nuevo

| Nombre | Mensaje al negarse | Roles sugeridos |
|---|---|---|
| `cash.close_without_clock_out` | «Tu usuario no puede cerrar la caja sin la salida de quien la abrió» | admin (por `AllPermissions`), gerente |

## Acción de bitácora nueva

| Acción | Grupo | Entidad | Sujeto | `after` |
|---|---|---|---|---|
| `cash.closed_without_opener_clock_out` | `AuditGroupCash` | `register_session` | quien abrió | `{entryId, reason}` |

Etiqueta en `web/src/shared/audit/labels.ts`: «Caja cerrada sin la salida de quien la abrió».
