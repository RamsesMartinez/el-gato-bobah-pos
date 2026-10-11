# Implementation Plan: Asistencia del personal y auditoría de acciones

**Branch**: `033-asistencia-y-auditoria` | **Date**: 2026-10-10 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `specs/033-asistencia-y-auditoria/spec.md`

## Summary

Dos piezas que comparten **el origen de cada request**: quién, IP, descripción del aparato y la
clave que el propio navegador genera.

1. **Asistencia** (activable por empresa): checar con usuario + PIN sin cambiar la sesión de la
   estación; horarios por versión semanal; reporte con la medición de retardos/extra apagada por
   omisión y tolerancias por empresa; corrección y captura a mano con motivo, con dos permisos
   (otros / propios); casilla «Lleva asistencia» por usuario.
2. **Auditoría** (siempre encendida): `audit_events` de solo agregar, escrita **dentro de la misma
   transacción** que la acción desde los servicios de `app`; historial por pedido y bitácora general.

Sin alta ni revisión de aparatos (decisión del dueño, 2026-10-10).

Diseño en [research.md](research.md), esquema en [data-model.md](data-model.md), endpoints en
[contracts/api.md](contracts/api.md), validación en [quickstart.md](quickstart.md).

## Technical Context

**Language/Version**: Go 1.27.2 (server), TypeScript + React 19 (web)

**Primary Dependencies**: las existentes (chi, pgx + sqlc, goose; Chakra v3, TanStack Query,
Zustand). Ninguna nueva.

**Storage**: PostgreSQL con RLS por `company_id`; migración `0089`.

**Testing**: `go test` (unitarios en `domain`; integración bajo `appRoleStore` + `inTheThreeCases`),
vitest, Playwright e2e contra el ambiente de pruebas a 1024×600.

**Target Platform**: API en VM Linux; POS en tabletas/Surface 1024×600, cualquier navegador.

**Project Type**: Web application (server/ + web/).

**Performance Goals**: checar en < 15 s (SC-001); historial de un pedido con un índice parcial;
bitácora paginada con `Count` gemela.

**Constraints**: evento y acción en la misma transacción (FR-013); el PIN comparte lockout con el
desbloqueo (FR-002); una clave de aparato ausente o malformada nunca rechaza una acción.

**Scale/Scope**: estimado ~5–10 eventos por pedido, ~400 bytes cada uno: decenas de miles de
eventos al año por local, pocos MB al mes. Asistencia: 1 renglón por persona por día.

## Constitution Check

| Principio | Cómo se cumple |
|---|---|
| I. Layering | `domain`: lista blanca de acciones, cálculo de horas/retardo/extra, normalización del origen (clave, descripción), reglas de corrección y de solape. `app`: `AttendanceService`, `AuditService` y `recordAudit(ctx, q, …)` dentro de cada `WithTx`. `httpapi`: handlers finos + middleware `withRequestOrigin`. `store/db`: sqlc |
| II. Errores | Sentinels nuevos en `domain` (`ErrAttendanceDisabled`, `ErrAttendanceAlreadyOpen`, `ErrAttendanceNotOpen`, `ErrAttendanceOverlap`, `ErrReasonRequired`, `ErrNoPIN`), mapeados solo en `httpapi.Error` |
| III. Dinero | `before`/`after` guardan montos ya con `Round2`; ninguna cifra agregada nueva |
| IV. Test-first | Integración adicional: `delete from companies` como owner con filas en `audit_events` funciona; un evento suelto no se borra; `missing_out` con salida → 23514; dos capturas a mano simultáneas del mismo turno → una falla; un `update` de `clock_in_at` sin `corrected` → error. Unitarios table-driven del cálculo (medianoche, tolerancia, versión de horario que quita un día, sin salida, >16 h, cambio de zona horaria). Integración: las 5 tablas nuevas en `inTheThreeCases`; atomicidad acción+evento; append-only como `gatobobah_app` y como owner; borrar una empresa con eventos funciona; test de la migración con dos empresas, sin respaldo restaurado porque no rellena datos (mismo criterio y justificación escrita que la 0066) |
| V. Seguridad | PIN con `authFails` y la llave `pin:<id>` (la del desbloqueo) + `rateLimit` por IP; IP con `rateKeyIP` (respeta `behindProxy`), no con `clientIP`; la clave del aparato se valida por forma (32 hex) y se recorta; IP/aparato fuera de toda respuesta sin permiso; `RequirePermission` en cada endpoint de lectura y corrección |
| VI. YAGNI | Sin registro de aparatos, sin auditar lecturas, sin vista propia del empleado, sin roles por empresa, sin offline |
| VII. Idioma | Identificadores y claves de acción en inglés (`order.created`); textos en español, con un mapa acción → etiqueta en el front |
| VIII. Puertas | Ver abajo |

**Puertas (¿se puede agregar después al mismo costo?)**

| Puerta | Decisión |
|---|---|
| Roles por empresa | Todo control nuevo es `domain.Permission`; nada pregunta por nombre de rol |
| Saber de quién es un pedido / más de una caja | Cada evento guarda autor **y** clave del aparato: distingue estaciones que comparten cuenta. Ponerle nombre a una clave después no requiere tocar datos |
| Sucursales | `audit_events.branch_id` sale del objeto auditado (pedido, caja), no del aparato. `attendance_entries.branch_id` se llena solo con una sucursal; con varias queda nulo **y no se rellena después** — se documenta en `AGENTS.md` como lo que el selector de sucursal tendrá que resolver |
| Historial de horas | `business_date` guardado en cada entrada; horarios por versión completa; los minutos de retardo/extra se calculan al vuelo con el ajuste **vigente** (aceptado: cambiar la tolerancia reescribe reportes pasados; es el comportamiento que el dueño espera de un ajuste) |
| Horario partido | `shift_no` en el día del horario: la puerta queda abierta sin construir la pantalla |

Un evento que no se registró no se reconstruye: por eso se audita todo FR-011 desde el primer
despliegue, aunque la bitácora general sea P2.

**Gate**: PASA.

## Pantallas (1024×600, riel lateral de 76 px → 948 px útiles)

| Pantalla | Dónde | Disposición |
|---|---|---|
| **Checar** (maqueta C) | Ítem «Asistencia» en el riel, después de «Reportes» (solo con la asistencia activa; se actualiza el comentario de `nav.ts`) **y** botón «Registrar asistencia» en la pantalla de bloqueo, que abre esta misma vista encima del bloqueo sin desbloquear | Dos columnas: «Por entrar» (cuadrícula de 2, botones ≥ 64 px) y «Dentro» (con «desde 9:02»); encabezado «N en el local». Cada columna con scroll propio cuando no cabe. Tocar a alguien abre una hoja con su nombre, «No soy yo» y el teclado de PIN **extraído de `LockScreen` a `PinPad`**, cuya tecla de acción dice «Entrada» o «Salida» según la columna. Al abrir la hoja se vuelve a pedir el roster; el request manda `expected` (`in`/`out`) y un 409 se muestra como lo que pasó. Quien no tiene PIN aparece deshabilitado con «Pide tu PIN a tu gerente». Un PIN incorrecto cuenta para el mismo bloqueo que el desbloqueo (FR-002); el error dice «No coincide» igual que el bloqueo (anti-enumeración) y solo cuando el servidor responde 429 dice en cuántos minutos reintentar. `PinPad` es el único dueño del oyente de teclado físico: con Checar abierto encima del bloqueo, el teclado del bloqueo se desmonta y su persona y PIN se reinician (si no, cada dígito cae en los dos y un Enter manda dos intentos que cuentan doble para el bloqueo). La vista y la hoja llevan `role="dialog"` como el bloqueo, para que el medidor de toques no atribuya el teclado a la pantalla de abajo; la hoja lleva `maxH` en dvh. Confirmación «Entrada 9:02» / «Trabajaste 8 h 10 min» con la cifra del servidor, cerrada sola a los 3 s |
| **Asistencia › Reporte** (maquetas A, B y C, con selector) | Misma ruta `/asistencia`, pestaña visible solo con `attendance.report` | `Page fill`; en el renglón de filtros: rango (dos botones de fecha), persona (`Picker`) y un selector de tres segmentos de 44 px **Por persona / Día por día / Semana** (por omisión «Por persona»; la tableta recuerda la última en `localStorage`; «Semana» solo con un rango ≤ 7 días, si no se desactiva con «Solo para una semana»). Las tres vistas salen de la misma respuesta. **Por persona**: renglón con días, horas del rango y pendientes; se abre para ver sus días. **Día por día**: entrada, salida, horas. **Semana**: personas × días con horas y total. Todas en `flex=1 minH=0` con scroll propio y `overflowX=auto`; renglón ≥ 44 px; «Falta salida» / «A mano» / «Corregido» como texto. Tocar un día abre la hoja con IP, aparato, clave y «Corregir» / «Capturar a mano» (motivo en hoja inferior) |
| **Horario** (maqueta A, ronda 2) | Empleados › ficha de la persona | Una fila por día con hasta **dos turnos**, cada uno con dos botones de hora que abren un `Picker` de 15 min sobre las 24 h (nunca `type=time`); si la salida es menor que la entrada se marca «+1 día»; «Día libre», «+ Turno», «Copiar a lunes–viernes» y **«Copiar de otra persona»** (`Picker` de personas, nunca una lista propia: trae sus días al formulario y se guarda como versión nueva de esta persona, sin ligarla). Vigencia con un botón de fecha. Casilla «Lleva asistencia» en la misma ficha |
| **Ajustes** | Negocio › sección «Asistencia» | Interruptor activar; interruptor medir retardos y extra; dos tolerancias en minutos con `Picker` (0, 5, 10, 15, 20, 30, 45, 60) |
| **Bitácora** (maqueta C) | Reportes › pestaña «Bitácora» (no ocupa riel) | Filtros visibles: fecha y persona. Debajo, cinco contadores-botón por tipo (Cobros y pedidos, Cancelaciones y devoluciones, Precios, Caja, Asistencia) que filtran al tocarlos (~70 px). Lista con separador de día, una línea por evento: hora, persona, acción en español con punto de color, resumen y «antes → después». Paginación con botones de 44 px, orden fijo por hora descendente. Tocar abre la hoja de detalle (quién, cuándo, IP, aparato, clave, qué cambió y «Ver historial del pedido» cuando aplica). Los conteos salen de una consulta gemela con el mismo `where` |
| **Historial del pedido** (maqueta C) | Pestañas en el detalle del pedido de Ventas (`SaleDetailDialog`): **Venta · Historial** (implementado así: el detalle existente ya mezcla productos y pagos en una sola vista; partirla en dos pestañas no aportaba). El diálogo pasa a abrirse por `orderId` (hoy recibe la fila de la lista), para que «Ver historial del pedido» de la bitácora lo abra aunque el pedido siga abierto o esté fuera del filtro de Ventas. La fila de pestañas va fija bajo el encabezado, fuera del scroll; el scroll es solo del panel, con `minH={0}` | La fila de pestañas (48 px) aparece **solo** con `can('audit.view')`; sin el permiso, el pedido se ve como hoy. «Historial» es una línea de tiempo con hora, punto de color (cobro, cancelación/devolución, resto), acción y autor/aparato, con scroll propio. «Pagos» lista los cobros y los devueltos. Las acciones del pedido (Entregar, Cobrar) siguen fuera de las pestañas |

Horas siempre en la zona del negocio (`business_settings.timezone`), no la del navegador.

Tarea explícita: medir el riel a 600 px de alto con el rol admin tras agregar «Asistencia», con
captura real.

## Pantallas y maquetas

Elegidas por el dueño el 2026-10-10/11. Las tareas de pantalla citan su fila.

| Pantalla | Maquetas | Elegida | Por qué |
|----------|----------|---------|---------|
| Checar | https://claude.ai/artifact/FH3BLua2x99N7rcq4zk1EF | C · Por entrar / Dentro | Se ve de un vistazo quién está en el local y desde qué hora |
| Reporte de asistencia | https://claude.ai/artifact/3tSoN6cNmLtfQhRYY19eHK | A + B + C con selector, «Por persona» por omisión | El dueño quiere las tres vistas; salen de los mismos datos |
| Horario | https://claude.ai/artifact/L8BuMoynmWMjCFf3ySK6tx (ronda 2) | A · Lista por día con turnos + «Copiar de otra persona» | Cubre turnos que cruzan el día y dos turnos sin pantallas nuevas; las plantillas de turno se pueden agregar después al mismo costo |
| Bitácora | https://claude.ai/artifact/NeVU1shfxp4DXimMKU57CS | C · Por tipo de acción | Los contadores responden «¿hubo cancelaciones?» sin buscar |
| Historial del pedido | https://claude.ai/artifact/6qPPjMotvsX7Ap9su5zqsE | C · Pestaña del pedido | Se encuentra sin buscar; las pestañas solo aparecen con permiso |

## Project Structure

### Documentation

```text
specs/033-asistencia-y-auditoria/
├── plan.md  research.md  data-model.md  quickstart.md
├── contracts/api.md
└── tasks.md          # /speckit-tasks
```

### Source Code

```text
server/
├── migrations/0089_attendance_and_audit.sql
├── queries/{attendance.sql, audit.sql}
└── internal/
    ├── domain/{attendance.go, origin.go, audit.go, permission.go}   (+ _test.go)
    ├── app/{attendance.go, audit.go}  + recordAudit en orders.go, devolucion.go, move_lines.go,
    │        expenses.go, tips.go, platform_prices.go, pairing.go, servicio de caja, settings.go, users.go
    ├── httpapi/{handlers_attendance.go, handlers_audit.go, request_origin.go, router.go}
    └── integration/{attendance_test.go, audit_test.go, migration_0089_test.go}
web/src/
├── api/origin.ts                 # clave del aparato + descripción en cada request
├── components/PinPad.tsx         # extraído de LockScreen
├── features/attendance/          # checar, reporte, corrección
├── features/audit/            # pestaña de Reportes + hoja de historial del pedido
└── consola: etiquetas-de-uso.ts  # pantallas nuevas medidas
```

## Desviaciones de la implementación

- **Historial del pedido**: dos pestañas (Venta · Historial) en vez de tres; ver la fila de arriba.
  `SaleDetailDialog` no se cambió a abrirse por `orderId`: desde la bitácora, «Ver historial del
  pedido» abre una hoja con la línea de tiempo (`OrderHistoryTimeline`), que es lo que hacía falta.
- **Acciones de caja**: no existe un arqueo intermedio (el conteo va en el cierre), así que
  `cash.counted` se reemplazó por `cash.movement` (entradas, salidas, propinas y traspasos) y
  `cash.movement_corrected`. Se agregó el precio de mostrador (`price.set` al editar el producto).
- **«Lleva asistencia»** se guarda por `PUT /admin/attendance/people/{userId}/tracks` y no en la
  edición de usuario; si la asistencia está activa lo dice el roster, no `/me`.
- **Piezas compartidas** en `web/src/shared/` (checar, etiquetas e historial): la regla de fronteras
  de eslint prohíbe que una pantalla importe de otra. El horario y los ajustes viven en
  `features/admin`, la bitácora en `features/backoffice`; checar en `shared/attendance`.
- **Selector de fecha táctil** (`components/DatePickerButton.tsx`, decisión del dueño 2026-10-11):
  reemplaza a todo `input type="date"` del POS, no solo a los de esta feature.

## Complexity Tracking

| Riesgo | Por qué se acepta | Mitigación |
|---|---|---|
| ~25 caminos de escritura reciben `recordAudit` | FR-013 exige la misma transacción; un middleware no conoce el antes → después | Lista blanca en `domain` + test que recorre `AllAuditActions()` y exige un test de integración que produzca cada una |
| `platform_prices.go` escribe sin `WithTx` | El evento necesita transacción | Se envuelve en `WithTx` en la misma tarea |
| Pantallas medidas: 3 lugares | Regla de AGENTS.md | Tarea: `pantallasMedibles`, `rolesPorPantalla`, `PANTALLAS` |
| `clientIP` confía en X-Forwarded-For siempre | Hallazgo colateral; solo afecta logs | Anotarlo en `docs/security-owasp.md`; la evidencia usa `rateKeyIP` |
