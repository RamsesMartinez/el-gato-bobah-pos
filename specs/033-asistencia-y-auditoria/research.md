# Research: Asistencia y auditoría

## R1. Clave del aparato

- **Decision**: el front genera 16 bytes aleatorios (`crypto.getRandomValues`) la primera vez,
  los guarda en `localStorage` (`egb:device:v1`) y los manda como 32 hex en `X-Device-Key`. El
  servidor la valida por forma (`^[0-9a-f]{32}$`); si no la cumple, la guarda nula. Se guarda tal
  cual (no es secreto: identifica, no autentica).
- **Rationale**: decisión del dueño (2026-10-10): pista de auditoría sin registro de aparatos.
- **Alternatives**: alta de aparatos con token del servidor (quitada por rebuscada); huella del
  navegador (frágil).
- **Límite aceptado**: se renueva al borrar datos del navegador y puede falsificarse.

## R2. Descripción del aparato

- **Decision**: `X-Device-Info` (JSON ≤ 300 bytes): `userAgentData` + `getHighEntropyValues
  (['platformVersion','model'])` cuando existe; si no, el user-agent; y `screen.width×height`.
  `domain.NormalizeOrigin` lo recorta y descarta lo que no sea esas llaves.
- **Rationale**: solo describe; no decide nada. Sin librería de parseo (VI).

## R3. IP

- **Decision**: `rateKeyIP(r, behindProxy)`. `clientIP` (logging.go:111) confía en
  X-Forwarded-For siempre: apto para logs, no para evidencia.

## R4. Checar sin cambiar la sesión

- **Decision**: `POST /attendance/clock` bajo `RequireAuth` + `WithTenant` (cualquier rol), con
  `{userId, pin}`; el servidor decide entrada o salida según si hay una abierta (la pantalla ya lo
  muestra, pero manda el servidor). Verifica el PIN con la misma rutina de `relevar` (bcrypt +
  dummy) **sin emitir tokens**. Lockout con `authFails` llave `pin:<userId>` y `rateLimit` por IP.
- **Rationale**: un camino nuevo que no reusa el limitador nace sin él (constitución, IV).
- **Respuesta del roster**: no revela quién no tiene PIN a quien no es admin; un usuario sin PIN
  sale deshabilitado con «Pide tu PIN a tu gerente».

## R5. Auditoría en la misma transacción

- **Decision**: `app.recordAudit(ctx, q, domain.AuditEvent)` dentro del `WithTx` de cada acción.
  El origen lo pone `httpapi.withRequestOrigin` en el contexto (`app.OriginFrom(ctx)`). Caminos sin
  request (webhook, sincronización de precios) declaran actor `platform`/`system`.
- **Alternatives**: triggers de Postgres — no conocen usuario ni aparato y el antes → después de un
  pedido abarca varias tablas.
- **Sucursal**: la del objeto auditado (pedido, caja, sesión); nula para precios y ajustes, que son
  de empresa.

## R6. Append-only sin bloquear el borrado de la empresa

- **Decision**: `grant select, insert` a `gatobobah_app`; trigger `before update or delete` que
  lanza error **salvo** que la empresa ya no exista (`not exists (select 1 from companies where id
  = old.company_id)`), que es el cascade de tenant; trigger `before truncate` que lanza error.
- **FK a users**: compuestas y `no action` (se valida al final del statement): el cascade de la
  empresa no tropieza por el orden de borrado; borrar un usuario suelto con historia sigue fallando.

## R7. Cálculo de horas

- `domain.AttendanceDay(entries, scheduleVersion, settings)`:
  - El día es `business_date`, guardado al insertar (zona del negocio en ese momento) y recalculado
    solo por una corrección de la entrada.
  - Horario vigente = la **versión** con mayor `effective_from <= día`; un día sin fila en esa
    versión es libre.
  - `end < start` = termina al día siguiente.
  - Medición apagada: solo minutos trabajados. Encendida: retardo = `in − start` si supera la
    tolerancia (se cuenta completo); extra = `out − end` igual.
  - Entrada abierta > 16 h al checar de nuevo → `missing_out`, no suma.
  - Registros a mano y corregidos suman igual; se marcan.

## R8. Permisos nuevos

| Permiso | admin | gerente | cajero/mesero |
|---|---|---|---|
| `attendance.configure` (activar, medición, horarios, «Lleva asistencia») | sí | no | no |
| `attendance.report` (ver reporte, IP, aparato) | sí | sí | no |
| `attendance.correct_others` | sí | sí | no |
| `attendance.correct_own` | sí | no | no |
| `audit.view` | sí | sí | no |
