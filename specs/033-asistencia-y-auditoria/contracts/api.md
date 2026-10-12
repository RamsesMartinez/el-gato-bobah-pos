# Contratos — /api/v1

Todo request del POS manda `X-Device-Key` y `X-Device-Info`; si faltan o vienen malformados, la
acción sigue y el origen se guarda sin ellos. Toda lista sale como arreglo (nunca `null`). Filtros
inválidos → 400 `VALIDATION`. Los mensajes de error van para el operador; los códigos no se pintan.

## Checar (sesión de la estación, cualquier rol)

| Método | Ruta | Cuerpo / respuesta |
|---|---|---|
| GET | `/attendance/roster` | `{enabled, people: [{userId, name, openSince?, leftAt?, hasPin}]}` (`leftAt`: última salida del día del negocio) — activos con `tracks_attendance`, primero los de entrada abierta. Con la asistencia apagada, `enabled: false` y `people: []` (el riel y la pantalla de bloqueo lo usan para mostrar u ocultar Asistencia) |
| POST | `/attendance/clock` | `{userId, pin, expected: "in"\|"out"}` → `{kind, at, workedMinutes?}`. 409 `ATTENDANCE_STATE_CHANGED` (con `openSince`) si `expected` ya no corresponde. 401 PIN incorrecto o persona sin PIN (cuenta para lockout; no se distingue para no enumerar), 429 bloqueado con header `Retry-After`, 409 `ATTENDANCE_DISABLED` |

Nunca devuelve IP, aparato ni registros anteriores.

## Administración de asistencia

| Método | Ruta | Permiso | |
|---|---|---|---|
| GET/PUT | `/admin/attendance/settings` | `attendance.configure` | `{enabled, measureSchedule, graceInMin, graceOutMin}` |
| GET | `/admin/attendance/report?from&to&userId` | `attendance.report` | por persona y día: `entries[{id, in, out, status, source, corrected, inIp, inDevice, inDeviceKey, …}]`, `workedMinutes`, `lateMinutes?`, `overtimeMinutes?`, `missingOut`, `noSchedule`; totales. Sin medición, los opcionales no vienen |
| POST | `/admin/attendance/entries/{id}/corrections` | `correct_others` / `correct_own` (lo decide el servicio) | `{field, at, reason}`; 400 sin motivo; 409 `ATTENDANCE_OVERLAP` |
| POST | `/admin/attendance/entries` | `correct_others` / `correct_own` | captura a mano `{userId, in, out, reason}` |
| GET/POST | `/admin/attendance/schedules/{userId}` | `attendance.configure` | GET `{tracksAttendance, versions}`; POST `{effectiveFrom, days:[{weekday, shiftNo, start, end}]}` (minutos desde la medianoche); misma vigencia dos veces → 409 |
| PUT | `/admin/attendance/people/{userId}/tracks` | `attendance.configure` | `{tracks}` — «Lleva asistencia» |

## Auditoría (`audit.view`)

| Método | Ruta | |
|---|---|---|
| GET | `/orders/{id}/history` | eventos del pedido, cronológico |
| GET | `/admin/audit/actors` | `[{id, name}]` — personas para filtrar (también las inactivas) |
| GET | `/admin/audit?from&to&actorUserId&action&group&deviceKey&page&size` | `{items, total, countsByGroup, deviceKeys}`; cada item de pedido trae `orderLabel` como lo nombra Ventas («Mesa 4 · #12»); `deviceKeys` son los aparatos de todo el rango (sin el filtro de aparato, para poder cambiar de uno a otro); `Count` y conteo por grupo con el mismo `where` (sin el filtro `group`, para que los cinco contadores no se vacíen al tocar uno); `group` ∈ {`money`, `cancel`, `price`, `cash`, `attendance`}; `size` ∈ {25, 50} |

## Sesión

`GET /me` agrega a `permissions` los cinco permisos nuevos (el front pregunta con `can()`). Si la
asistencia está activa lo dice `GET /attendance/roster`, sin tocar la sesión.

Todo request lleva además `X-Device-Key` y `X-Device-Info`, que CORS anuncia; CORS expone también
`Retry-After` para que la pantalla diga en cuánto reintentar.
