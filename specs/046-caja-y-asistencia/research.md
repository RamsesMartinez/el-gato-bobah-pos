# Research: La caja exige la asistencia de quien la abre

Todo lo que el spec deja abierto se resuelve con el código que ya existe. No queda ningún
NEEDS CLARIFICATION.

## R1. Dónde se decide la regla

- **Decision**: dos funciones puras en `domain` (`CashOpenAttendance` y `CashCloseAttendance`)
  devuelven el estado y la decisión; la usan las tres puertas: el `GET` que pinta la hoja, la
  apertura y el cierre.
- **Rationale**: principio IV (lógica en `domain`, probada sin base) y el corolario del III: lo que
  la hoja muestra y lo que el servidor exige salen del **mismo** predicado. Si la hoja calculara «ya
  marcó salida» por su cuenta, un día dirían cosas distintas y nadie sabría cuál miente.
- **Alternatives considered**: decidirlo en `app` dentro de `OpenSession`/`CloseSession` (no se
  prueba sin base y el `GET` lo duplicaría); decidirlo en el front (prohibido: la barrera vive en
  el servidor, principio V).

## R2. Cómo se serializa contra checar en otra tableta

- **Decision**: apertura y cierre leen la asistencia **dentro de su transacción**, después de tomar
  la fila de la persona con una consulta nueva `LockUserAttendanceForCash` (`for share` sobre
  `users`). El cierre la toma después de `LockSessionForClose`.
- **Rationale**: `Clock`, `Correct` y `ManualEntry` ya se serializan con `LockUserForAttendance`
  (`for update` sobre `users`). `for share` choca con ese `for update`, así que una entrada o salida
  que confirma a la vez espera a que la caja termine (o al revés), y la regla se evalúa con lo que de
  verdad quedó. No choca con el `for key share` de las llaves foráneas: un pedido que esa persona
  captura a la vez no espera.
- **Alternatives considered**: reusar `LockUserForAttendance` (`for update`): también choca con el
  `for key share` de cada insert que referencia al usuario (`orders.opened_by`), y frena ventas sin
  necesidad. Revisar fuera de la transacción: deja pasar la carrera del borde «Ana marca entrada
  después de que la hoja la vio sin entrada».
- **Orden de candados**: cierre = turno → persona; apertura = persona → (insert del turno). `Clock`
  solo toma la persona. No hay ciclo.

## R3. Marcar entrada y salida desde la hoja

- **Decision**: la hoja llama al mismo `POST /attendance/clock` de la pantalla de Asistencia, con el
  id de la persona, su PIN y `expected`. No hay endpoint nuevo para checar.
- **Rationale**: FR-006 pide que el registro sea idéntico y comparta el bloqueo por PIN. Reusar el
  endpoint lo garantiza por construcción: mismo `rateLimit` por IP, misma llave `pin:<id>` de
  `authFails`, mismo evento `attendance.clock_in/out`, misma `NextClock` (que con una entrada de más
  de 16 h registra entrada y deja la vieja «sin salida»). Un endpoint propio de caja sería otra
  superficie para probar PINs.
- **Alternatives considered**: mandar el PIN dentro del cuerpo de abrir/cerrar (mezcla un hecho de
  asistencia con la operación de caja y rompe FR-011: cancelar la hoja desharía la entrada).

## R4. El cierre autorizado

- **Decision**: `POST /cash-sessions/close` acepta un campo nuevo opcional
  `authorizedClose: { reason }`. Con la regla aplicando y una entrada abierta de quien abrió:
  - sin el campo → 409 `OPENER_NOT_CLOCKED_OUT`;
  - con el campo y sin el permiso `cash.close_without_clock_out` → 403 con el mensaje del permiso;
  - con el campo, el permiso, y quien cierra **es** quien abrió con entrada de 16 h o menos → 409
    `CLOCK_OUT_WITH_PIN` («Marca tu salida con tu PIN»);
  - con el campo, el permiso y motivo válido → cierra, marca la entrada `missing_out` con
    `MarkAttendanceMissingOut` (ya existe) y escribe `cash.closed_without_opener_clock_out` en la
    misma transacción.
  Si la regla no aplica o la persona ya no tiene entrada abierta, el campo se ignora y el cierre es
  normal, sin evento de cierre autorizado (borde del spec).
- **Rationale**: un permiso por capacidad (constitución, *Roles*). El motivo usa la misma vara de 3
  a 200 caracteres que la corrección de asistencia. `missing_out` es exactamente el estado que
  `NextClock` ya pone a una entrada vieja, así que el reporte, la corrección y la siguiente entrada
  de esa persona ya saben tratarlo (US3, escenarios 5 y 6).
- **Alternatives considered**: reusar `attendance.correct_others` (mezcla dos capacidades: corregir
  horas pagadas no es lo mismo que decidir sobre la caja); registrar la salida a la hora del cierre
  (lo rechazó el dueño: inventa horas).

## R5. Cómo sabe la hoja el estado antes de contar

- **Decision**: `GET /cash-sessions/attendance?registerId=` devuelve el estado para la acción que
  toca (abrir si la caja está cerrada, cerrar si está abierta), calculado con las funciones de R1.
- **Rationale**: FR-005 (nada de contar y luego ser rechazado). Ponerlo dentro de
  `GET /cash-sessions/current` no sirve para la apertura (responde `null` con la caja cerrada) y
  mete datos de una persona en la vista del corte, que se pide cada vez que se pinta la caja.
- **Alternatives considered**: que la hoja lea `/attendance/roster` y deduzca (roster no trae la
  antigüedad de la entrada ni el permiso, y vuelve a duplicar el predicado).

## R6. Compatibilidad con la versión anterior durante la subida

- **Decision**: sin migración. Lo nuevo es un permiso en el mapa de `domain`, una acción de
  bitácora, un campo opcional en el cierre y un `GET` nuevo.
- **Rationale**: principio IV, *Toda migración tolera a la versión anterior*: no hay esquema que
  cambie. Una tableta con el front anterior recibe el 409 al abrir o cerrar con el mensaje para
  quien opera (US1 y US2, escenario 5) y puede marcar en la pantalla de Asistencia; nada queda a
  medias porque el rechazo ocurre dentro de la transacción, antes de escribir.
- **Rollback**: la API anterior no conoce `authorizedClose` y lo ignora; los eventos ya escritos
  con la acción nueva quedan en la bitácora (la lista blanca solo filtra al escribir — verificar en
  la implementación que la lectura no los descarta).
