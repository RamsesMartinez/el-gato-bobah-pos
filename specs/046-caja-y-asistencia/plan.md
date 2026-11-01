# Implementation Plan: La caja exige la asistencia de quien la abre

**Branch**: `046-caja-y-asistencia` | **Date**: 2026-10-30 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `specs/046-caja-y-asistencia/spec.md`

## Summary

Abrir caja exige una entrada vigente (de 16 h o menos) de quien abre. Cerrarla exige que quien la
abrió no tenga ninguna entrada abierta, salvo un **cierre autorizado** con un permiso nuevo y un
motivo, que deja esa entrada «sin salida». Con la asistencia apagada, o si la persona no lleva
asistencia, nada cambia.

- **Sin migración.** La regla lee datos que ya existen y escribe con consultas que ya existen
  (`MarkAttendanceMissingOut`), más una consulta de lectura con candado.
- **Una sola regla para tres puertas.** El `GET` que pinta la hoja, abrir y cerrar usan las mismas
  dos funciones de `domain` ([research R1](research.md#r1-dónde-se-decide-la-regla)).
- **Marcar desde la hoja reusa `POST /attendance/clock`**, con su límite y su bloqueo por PIN
  ([R3](research.md#r3-marcar-entrada-y-salida-desde-la-hoja)).
- **El servidor decide dentro de la transacción**, con la fila de la persona tomada `for share`, así
  que una entrada o salida en otra tableta no se cuela
  ([R2](research.md#r2-cómo-se-serializa-contra-checar-en-otra-tableta)).

## Technical Context

**Language/Version**: Go 1.27.3 (server) · TypeScript, React 19, Chakra UI v3 (web)

**Primary Dependencies**: las existentes (chi, pgx + sqlc, TanStack Query, Zustand); ninguna nueva

**Storage**: PostgreSQL; sin cambios de esquema. Una consulta nueva en `server/queries/attendance.sql`

**Testing**: `go test` (unitarios table-driven en `domain`), integración contra Postgres real con
`appRoleStore` e `inTheThreeCases`, vitest para las piezas de la hoja, Playwright local a 1024×600

**Target Platform**: tabletas de 7 a 10", ~1024×600

**Project Type**: web (server + web)

**Performance Goals**: el `GET` de estado es tres lecturas por llave; no se mide aparte

**Constraints**: controles ≥ 44 px, sin `<select>` nativo, un solo estilo, texto para quien opera;
la API anterior convive durante la subida (sin esquema nuevo, no hay riesgo)

**Scale/Scope**: ~4 archivos de `server/internal` (domain, app, httpapi) + 1 de queries; ~5 de
`web/src`; tests nuevos en `domain`, `integration` y junto a los componentes

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principio | Cómo se cumple |
|---|---|
| I Layering | Regla en `domain/cash_attendance.go`; lectura con candado y escritura en `app` (`OpenSession`, `CloseSession`, `CashAttendanceState`); handlers finos; SQL solo por sqlc |
| II Errores | Sentinels nuevos envueltos de `ErrConflict`; códigos `CLOCK_IN_REQUIRED`, `OPENER_NOT_CLOCKED_OUT`, `CLOCK_OUT_WITH_PIN` mapeados solo en `httpapi.Error` |
| III Dinero | No toca montos. El cierre autorizado no cambia el arqueo; el evento no lleva cifras |
| III Corolario (mismo predicado) | El `GET` y las dos escrituras salen de `CashOpenAttendance`/`CashCloseAttendance`; un test compara `ready` del `GET` contra el resultado de confirmar en cada caso de la tabla |
| IV Test-first y bordes | Bordes enumerados abajo antes del código; cada uno con su test. Integración bajo `appRoleStore`; el `GET` lee tablas de empresa → `inTheThreeCases` |
| IV Migración tolera la anterior | No hay migración. La API anterior ignora `authorizedClose`; una tableta con el front anterior recibe el 409 con mensaje para quien opera |
| V Seguridad | Barrera en el servidor; permiso nuevo `cash.close_without_clock_out`; checar desde la hoja usa el endpoint con `rateLimit` y `authFails`, sin superficie nueva para PINs. El `GET` no expone IP, aparato ni historial. Se valida permiso antes de la regla del cierre propio (no filtra antigüedad de una entrada ajena) |
| VI YAGNI | Sin endpoint de checar propio, sin configuración de las 16 h, sin revisar al resto del personal |
| VII Comentarios / idioma | Identificadores en inglés (`CashCloseAttendance`, `authorizedClose`, `cash.closed_without_opener_clock_out`); textos y comentarios en español |
| VIII Puertas | **¿Se puede agregar después al mismo costo?** No toca esquema. «Más de una caja vendiendo a la vez»: la regla es por turno y por quien lo abrió, no asume una sola caja. «Saber de quién es un pedido»: no lo toca. «Roles configurables»: el control nuevo es un permiso; el `GET` vive en el grupo `/cash-sessions` que ya existe con `RequireRole` y no agrega uno nuevo |
| Quien opera no decide en el momento | La regla se aplica sola; lo único que se pregunta es el PIN de quien marca (un hecho físico) o el motivo de quien autoriza. Quien no puede autorizar no ve opciones que no puede usar |
| Tableta | Ver «Pantallas y maquetas»; texto sin términos técnicos (FR-010) |

Pasa. Post-diseño: sin cambios.

**Exención firmada por el dueño (2026-10-30): el grupo `/cash-sessions` sigue con `RequireRole`.**
La constitución pide pasar a permiso las rutas con `RequireRole` que se tocan por otra razón, y esta
feature agrega `GET /cash-sessions/attendance` y cambia el cuerpo de `POST /cash-sessions/close`. No
se migran aquí: el grupo tiene 10 rutas que hoy no tienen permiso propio (no existe un
`cash.operate`), y crearlo y repartirlo es otra feature. Lo nuevo de esta feature que sí decide
algo —autorizar el cierre— cuelga de un permiso (`cash.close_without_clock_out`). La exención
cubre solo estas dos rutas; la siguiente feature que toque el grupo lo migra completo.

## Capas

| Pieza | Capa | Archivo |
|---|---|---|
| `CashAttendance`, `CashGate`, `CashOpenAttendance`, `CashCloseAttendance`, sentinels | domain | `server/internal/domain/cash_attendance.go` (+ `_test.go`) |
| `PermCashCloseWithoutClockOut`, mensaje, gerente | domain | `server/internal/domain/permission.go` |
| `cash.closed_without_opener_clock_out` | domain | `server/internal/domain/audit.go` |
| `LockUserAttendanceForCash` (`for share`, devuelve `tracks_attendance`, nombre) | store | `server/queries/attendance.sql` → `make sqlc` |
| Revisión en `OpenSession` (dentro de la tx, antes del insert) | app | `server/internal/app/backoffice.go` |
| Revisión y cierre autorizado en `CloseSession` (tras `LockSessionForClose`) | app | `server/internal/app/backoffice.go` |
| `CashAttendanceState(registerID, user, perms)` para el `GET` | app | `server/internal/app/cash_attendance.go` |
| `GET /cash-sessions/attendance`; `authorizedClose` en el cierre; códigos nuevos | httpapi | `handlers_backoffice.go`, `router.go`, `respond.go` |
| Tipos y llamada del `GET`, `authorizedClose` en `cashClose` | web api | `web/src/api/backoffice.ts` |
| Hoja de PIN para una persona fija, con el mismo mapeo de errores que `ClockView` (extraído a una función compartida) | web | `web/src/shared/attendance/ClockPinSheet.tsx`, `clockErrors.ts` |
| Aviso de apertura en la tarjeta de caja cerrada; aviso de cierre en lugar del bloque `TablaDelCierre`…`BotonCerrarCaja`, que no se monta sin resolver | web | `web/src/features/backoffice/CashPage.tsx`, componente nuevo `AsistenciaDeCaja.tsx` |
| Permiso en el tipo `Permission`; etiqueta de bitácora; mensajes de los tres códigos | web | `app/permissions.ts`, `shared/audit/labels.ts`, `api/mensajes` |

## Pantallas y maquetas

Dos cambios de disposición dentro de la pantalla de Caja (`CashPage`, `RegisterPanel`); ninguna
pantalla nueva. Lo que el spec llama «hoja de apertura» es aquí la tarjeta de caja cerrada, y la
«hoja de cierre» es la sección del cierre de la caja abierta.

1. **Caja cerrada (apertura)** — maqueta A. Con la regla aplicando y sin entrada vigente, la
   tarjeta «“X” está cerrada» agrega un aviso «Para abrir la caja, primero marca tu entrada» (con
   «Ana, no tienes entrada registrada hoy»), el botón principal «Marcar entrada» y debajo «Contar el
   efectivo» deshabilitado. Con una entrada de más de 16 h, el renglón dice «Tu entrada anterior ya
   venció». «Marcar entrada» abre la hoja de PIN (abajo). Al marcar, la tarjeta vuelve a la de hoy
   con el `Badge` de estado del tema «Entrada marcada 9:02» y «Contar el efectivo» habilitado. Sin
   la regla, la tarjeta es la de hoy.
2. **Caja abierta (cierre)** — maqueta A, con el aviso **arriba** (decisión del dueño, 2026-10-30,
   tras la prueba con usuario: en la sección del cierre quedaba en y≈590 y no se veía sin
   desplazar). Mientras quien abrió tenga entrada abierta, debajo del nombre de la caja y antes de
   las cifras va el aviso con título «Ana abrió esta caja y no ha marcado su salida» (el mismo texto
   del 409 y del spec), «Para cerrar la caja, primero hay que resolverlo» y «Ana sigue con la
   entrada abierta desde las 15:02» (con el día si no es de hoy; con `stale`: «Su entrada es muy
   antigua: solo se puede cerrar con autorización»). **El aviso reemplaza todo el bloque del
   cierre, desde `TablaDelCierre` hasta `BotonCerrarCaja`** (tabla y «contar cajón», terminales,
   fondo que se deja, notas y la lista «Falta capturar»): no se monta —en su lugar, «Para cerrar la
   caja, primero resuelve el aviso de arriba sobre Ana»—, así que no hay nada que
   contar antes de resolver (FR-005, SC-005; revisión de tableta A1). Al resolver se monta el
   bloque; lo que la otra tableta haya hecho mientras tanto no se pierde porque no se capturó nada. Botones: «Marcar salida de Ana» (hoja con `PinPad`) y, solo
   con `canAuthorize`, «Cerrar sin la salida de Ana». Sin el permiso, el aviso agrega «Si ya se fue,
   pide a alguien con permiso que cierre la caja» y no ofrece el botón. Con `stale`, no se ofrece
   «Marcar salida de Ana». Tampoco si quien abrió fue dada de baja (`person.active=false`, B7):
   su PIN ya no sirve, y el aviso dice «Ana ya no está activa: solo se puede cerrar con autorización». Resuelto, el aviso se vuelve una franja verde («Salida de Ana marcada
   22:04», o «Se cerrará sin la salida de Ana» con el motivo y «Quitar autorización»).
3. **Hoja de PIN** (apertura y cierre): el `Drawer` inferior del tema con el patrón de
   `ContadorDeEfectivo` (`maxH` en dvh, cuerpo con scroll), no la superposición de `ClockView`
   (sin portal: en la página de caja quedaría fuera de vista). Encabezado «Marcar entrada · Ana» /
   «Marcar salida · Ana» con cierre de 44 px, puntos, `PinPad` con `actionColor` verde para entrada
   y rojo para salida (como `ClockView`), y el error bajo el teclado con los mismos textos que
   `ClockView` (función compartida). `active={open && !bloqueada}`; el PIN se limpia al cerrar y al
   bloquearse la tableta. Cabe a 600 px (~470 px estimados).
4. **Hoja de cierre autorizado**: copia `MotivoDeApertura` (chips de 52 px en rejilla, un `Input` de
   una línea, botón fijo en el pie, `maxH` 85dvh) para que el teclado del sistema no tape el botón.
   Texto: «La entrada de Ana quedará “sin salida” y se corrige después en Asistencia. Queda
   registrado que lo autorizaste tú». Motivos: «Se fue sin checar», «Salió por una emergencia»,
   «Otro»; con «Otro» la etiqueta del campo pasa de «Detalle (opcional)» a «¿Qué pasó?» y el botón
   espera a que se escriba. Lo que viaja como `reason` es el motivo más el detalle.
5. **Si el `GET` de estado falla** (API anterior durante la subida, red): la hoja no da la regla por
   cumplida ni por bloqueante; muestra la tarjeta y el cierre de hoy y deja que mande el 409 del
   servidor, que se muestra con su mensaje.

| Pantalla | Maquetas | Elegida | Por qué |
|----------|----------|---------|---------|
| Caja cerrada: apertura con entrada pendiente | https://claude.ai/artifact/Gr4aVZLhGnh8HwcvMXGMcQ | A · aviso en la tarjeta, «Contar el efectivo» deshabilitado hasta marcar | El requisito se ve antes de tocar y el botón de contar no promete lo que no puede hacer |
| Caja abierta: cierre con salida pendiente | https://claude.ai/artifact/1MUdCKFpgTYoiW9MU4FR8t | A · aviso que reemplaza el conteo; **el aviso sube arriba de la caja** (2026-10-30) | Elegida A por no avisar de lo normal; la prueba con usuario mostró que en la sección del cierre no se veía sin desplazar, y el dueño pidió subirlo |

Descartadas: en la apertura, pedir el PIN dentro de «Contar el efectivo» (esconde el requisito) y el
teclado dentro de la tarjeta (760 px y un PIN a la vista); en el cierre, un renglón fijo arriba con
quién abrió (52 px todo el día).

## Bordes (se piensan antes del código)

| # | Borde | Cómo se cierra | Test |
|---|---|---|---|
| B1 | Entrada de exactamente 16 h | `IsStaleOpen` usa `>`: 16 h exactas siguen vigentes | domain, tabla |
| B2 | Entrada de más de 16 h al abrir | No vigente; marcar desde la hoja registra entrada y deja la vieja «sin salida» (`NextClock`) | domain + integración |
| B3 | Entrada de más de 16 h al cerrar | Bloquea igual; `stale=true` → la hoja no ofrece PIN, solo autorizar | domain + vitest |
| B4 | Otra tableta marca entrada/salida mientras se abre o cierra | Candado `for share` sobre la persona dentro de la tx; se decide con lo confirmado | integración con dos conexiones |
| B5 | Asistencia encendida entre abrir y cerrar | Se evalúa al cerrar: con entrada abierta, se pide; sin ninguna, cierra | integración |
| B6 | «Lleva asistencia» apagado a quien abrió con la caja abierta | Al cerrar ya no aplica | integración |
| B7 | Quien abrió fue dada de baja | Sigue pidiendo salida; solo cierre autorizado. El `GET` manda `person.active=false` y la hoja no ofrece el PIN, que el servidor rechazaría | integración + vitest |
| B8 | `authorizedClose` cuando ya no hace falta | Se ignora; cierre normal sin evento autorizado | integración (cuenta eventos) |
| B9 | `authorizedClose` sin permiso, con entrada ajena de más de 16 h | 403 antes de mirar la antigüedad | domain |
| B10 | Cierre propio autorizado con entrada ≤ 16 h | 409 `CLOCK_OUT_WITH_PIN`; con > 16 h, permitido | domain + integración |
| B11 | Motivo de espacios | `TrimSpace` antes de medir; 400 | domain |
| B12 | El cierre falla después de marcar «sin salida» (p. ej., pedidos pendientes) | Todo en la misma tx: la entrada vuelve a `open` y no queda evento | integración |
| B13 | Dos personas autorizan el mismo cierre a la vez | `LockSessionForClose` serializa; la segunda ve la caja cerrada (404 como hoy) | integración existente de cierre doble + un evento |
| B14 | Cancelar la hoja tras marcar | El clock es su propia operación; la caja no cambia | vitest |
| B15 | La tableta tiene la sesión de alguien que ya marcó salida | El `GET` lo dice (`isSelf`, `ready=false`); abrir responde 409 | integración |
| B16 | `GET` con `registerId` malformado | 400, no un default | handler |
| B17 | `GET` desde otra empresa, conexión reciclada y sin empresa | `inTheThreeCases` | integración |
| B18 | `ready` del `GET` y resultado de confirmar divergen | Test que recorre los casos de la tabla y compara ambos | integración |
| B19 | Varias cajas abiertas por la misma persona | Una salida libera todas (se mira la entrada, no la caja) | integración |
| B20 | El evento nuevo nunca se produce | `TestEveryAuditActionIsProduced` lo exige | integración |
| B21 | La entrada se cerró entre leerla y marcarla «sin salida» (otro cierre autorizado de otra caja de la misma persona: los dos `for share` no chocan) | `MarkAttendanceMissingOut` tiene que afectar exactamente 1 fila; con 0, 409 `ATTENDANCE_STATE_CHANGED` («vuelve a intentarlo») sin evento. Era `OPENER_NOT_CLOCKED_OUT`, que en ese momento ya es falso (revisión de código, 2026-10-30) | integración con dos conexiones |
| B22 | Una lectura de asistencia sale de otra conexión y el candado no protege | Candado, ajustes, «lleva asistencia», entrada abierta, marca y evento usan el `q` de `WithTx`, nunca `s.store.QC` (revisión de base M1) | integración B4 con dos conexiones reales |
| B23 | La persona no aparece (otra empresa, RLS) | `ErrNoRows` del candado es `ErrNotFound`, nunca «no aplica» | domain/app |
| B24 | El `GET` de estado falla o no existe (front nuevo, API vieja) | La hoja no decide con el error; manda el 409 del servidor | vitest |
| B25 | Durante el traslape la API anterior abre y cierra sin la regla | Esperado; no deja datos inconsistentes (no escribe evento ni «sin salida») | — |
| B26 | Un rechazo por asistencia llega después de contar (acota SC-005) | Solo en dos casos, aceptados: otra tableta cambia la entrada de esa persona mientras se cuenta (el servidor decide al confirmar, caso de borde 2 del spec) o la consulta del estado falló (B24). Lo contado no se guarda en el servidor, así que el rechazo no deja nada a medias; el mensaje del 409 dice qué hacer. SC-005 se mide como: con la consulta del estado respondiendo y sin cambios en otra tableta, ningún rechazo llega después de contar | B18 (el `GET` y confirmar coinciden) + vitest de B24 |

## Project Structure

### Documentation (this feature)

```text
specs/046-caja-y-asistencia/
├── spec.md
├── plan.md
├── research.md
├── data-model.md
├── quickstart.md
├── contracts/cash-attendance.md
├── checklists/requirements.md
└── tasks.md             # /speckit-tasks
```

### Source Code (repository root)

```text
server/
├── queries/attendance.sql                       # LockUserAttendanceForCash
└── internal/
    ├── domain/cash_attendance.go (+ _test.go)   # regla pura
    ├── domain/permission.go, audit.go           # permiso y acción
    ├── app/backoffice.go                        # OpenSession, CloseSession
    ├── app/cash_attendance.go                   # estado para el GET
    ├── httpapi/handlers_backoffice.go, router.go, respond.go
    └── integration/cash_attendance_test.go
web/src/
├── api/backoffice.ts
├── app/permissions.ts
├── shared/attendance/ClockPinSheet.tsx, clockErrors.ts (+ tests)
├── shared/audit/labels.ts
└── features/backoffice/CashPage.tsx, AsistenciaDeCaja.tsx (+ test)
```

**Structure Decision**: monorepo existente, `server/` y `web/`; sin carpetas nuevas.

## Revisión de arquitectura (2026-10-30)

`db-architect` y `tablet-ui-reviewer` sobre este plan. Sin hallazgos de puertas ni de esquema. Se
incorporaron: el bloque del cierre que se reemplaza completo (A1), la hoja de PIN en el `Drawer` del
tema con salida en rojo y `active` (M1 tableta), la hoja de autorización con el patrón de
`MotivoDeApertura` (M2, M3), los textos con el día y la entrada vencida, y los bordes B21 a B25. Lo
verificado sin hallazgo: `for share` sobre `users` choca con el `for update` de checar y no con el
`for key share` de las llaves foráneas; un solo `open` por persona (índice
`attendance_entries_one_open`); el disparador de correcciones no estorba `open → missing_out`;
`gatobobah_app` tiene los permisos que pide `for share` y el `update` de `status`.

## Al construir (2026-10-30)

Prueba con alguien sin contexto (T033: persona de una fonda, cajera y gerente, a 1024×600). Los
cuatro pasos salieron; nada falló.

**Corregido aquí:**

| Hallazgo | Cambio |
|---|---|
| La hoja de marcar nunca dice «PIN» | Renglón «Teclea el PIN de Ana.» bajo el título |
| El aviso del cierre no dice que es lo que bloquea cerrar | Renglón «Para cerrar la caja, primero hay que resolverlo.» |
| «no tienes entrada registrada hoy» es falso si ya entró y salió ese día | «Ana, ahora no tienes una entrada marcada.» |
| La confirmación del cierre autorizado no recuerda que la entrada queda sin salida | La hoja de confirmar empieza con «La entrada de Ana quedará «sin salida».» (lo comprueba la e2e Q7) |
| En la hoja de PIN, «Cerrar» quedaba pegado al título; los botones del aviso salían naranjas; «p.m..» con doble punto; el texto de la hoja de autorización iba al lado del título | Corregidos al revisar las capturas de la e2e |

**Visto y no cambiado:**

- ~~**El aviso del cierre queda debajo del pliegue**~~ Subido arriba de la caja por decisión del
  dueño (2026-10-30); la e2e Q5 comprueba que se ve completo sin desplazar a 1024×600.
- **La autorización no sobrevive a recargar ni a otra tableta**: vive en la pantalla hasta confirmar
  el cierre, como el resto de lo capturado en el cierre. Con recargar se pide otra vez.
- Ajenos a esta feature, del cierre de siempre: «Sí, retirar» marcado de entrada con «Falta capturar
  … el fondo que se queda»; la columna «Diferencia» en «—» tras contar con el arqueo ciego; el mismo
  dinero llamado «Cajón» y luego «Efectivo»; «Declarado», «Movimientos (neto)», «Sistema», «Dif.»;
  los métodos de plataformas en efectivo en una fonda sin apps; el nombre cortado en la barra lateral.

**Revisión de código (2026-10-30)** — go-backend-reviewer, security-auditor y tablet-ui-reviewer,
sin bloqueantes. Aplicado: el error del PIN va arriba del teclado con su alto apartado (debajo
quedaba fuera de los 600 px); título y «Teclea el PIN…» apilados; 24 px entre «Marcar salida» y
«Cerrar sin la salida»; B21 responde «vuelve a intentarlo» y no «no ha marcado su salida»; el
comentario del orden de reglas dice lo que de verdad logra; sin fila de ajustes la asistencia está
apagada, igual que para checar; el evento ya no repite `entryId`.

**Pruebas e2e de caja contra la base restaurada:** `contar-el-cajon` pasa completa (7/7) con el
arqueo ciego encendido y apagado: esperaban «Monto inicial» y la diferencia antes de cerrar, que el
ciego no pinta; ahora esperan «Fondo inicial» y buscan el faltante en el corte cerrado. En
`caja-propinas-tarjeta`, dos pruebas fallan por datos del ambiente y quedan anotadas como deuda en
el propio archivo: una cobra con tarjeta sin elegir terminal en una empresa con varias (422, regla de la 036) y
la otra busca a una persona llamada «Usuario 5» que el respaldo no tiene. Ninguna de las cuatro
depende de la asistencia después del ayudante `web/e2e/asistencia.ts`. **Resuelto el 2026-11-01:**
la devolución manda la terminal, la propina crea a su propia persona (`web/e2e/personas.ts`), y en
la base local la suite abre la caja principal si está cerrada y la cierra al terminar
(`web/e2e/caja-principal.ts`); con el respaldo recién restaurado la mitad de la suite se saltaba o
fallaba porque daba por hecho un turno abierto.

## Riesgos

- **Pruebas que abren caja en una empresa con asistencia encendida dejan de pasar.** Integración: las
  empresas de prueba nacen con la asistencia apagada (`default false`, 0089), así que no cambian.
  E2E contra una base restaurada: si `gatobobah` la tiene encendida, `contar-el-cajon`,
  `arqueo-ciego`, `caja-propinas-tarjeta` y `presupuesto-del-cierre` tienen que marcar entrada
  antes de abrir y salida antes de cerrar. Se agrega un ayudante compartido en `web/e2e/`, como el
  del login.
- **Al subir, el turno que esté abierto se cierra con la regla nueva** (supuesto del spec). Si quien
  lo abrió no checó, el cierre pedirá su salida o un cierre autorizado esa misma noche.
- **Cuentas compartidas**: un negocio que abre caja con una cuenta que no es una persona tiene que
  apagarle «Lleva asistencia»; si no, esa cuenta nunca podrá abrir. Queda en el texto de ayuda de
  Asistencia, no en una regla.

## Complexity Tracking

Sin violaciones que justificar.
