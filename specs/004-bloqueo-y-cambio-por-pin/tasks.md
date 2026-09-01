---
description: "Tareas de implementación: bloqueo por inactividad y cambio de operador por PIN"
---

# Tasks: Bloqueo por inactividad y cambio de operador por PIN

**Input**: documentos de diseño en `/specs/004-bloqueo-y-cambio-por-pin/`

**Prerequisites**: [plan.md](./plan.md), [spec.md](./spec.md), [research.md](./research.md),
[data-model.md](./data-model.md), [contracts/api.md](./contracts/api.md)

**Tests**: obligatorios. La constitución (principio IV) exige el test antes del código, y **un task
de implementación sin su task de test antes está mal ordenado**. Aquí además el principio V pide que
ningún control de seguridad se mergee sin su test — la primera versión de esta lista lo incumplió en
tres lugares y `/speckit-analyze` lo marcó como HIGH.

**Organization**: por historia de usuario, para que cada una se pueda implementar y probar sola.

## Format: `[ID] [P?] [Story] Descripción`

- **[P]**: se puede hacer en paralelo (archivos distintos, sin dependencias)
- **[Story]**: a qué historia pertenece
- Cada tarea lleva su ruta exacta

---

## Fase 1: Fundación — los ajustes del negocio

**Desbloquea todo lo demás.** Por sí sola no cambia comportamiento: los defaults dejan el sistema
como está hoy.

- [X] T001 Crear la migración de los ajustes en `server/migrations/`, con las tres columnas de
      [data-model.md](./data-model.md) sobre `business_settings`: `pin_only_unlock` (default
      `false`), `lock_after_seconds` (default `180`) y `session_hours` (default `8`). Reversible,
      con su `Down`.
      **El número se asigna al implementar, no ahora**: el spec 003 también va a agregar migraciones
      y la que aterrice segunda tiene que tomar el siguiente libre.
- [X] T002 Escribir `server/internal/integration/ajustes_identificacion_test.go`: los tres nacen con
      su default, y guardarlos NO pisa los demás ajustes del ticket que viven en la misma fila
      (`print_kitchen_ticket`, `kitchen_can_charge`). Es el fallo clásico de esa tabla y ya mordió una vez.
- [X] T003 Agregar las tres columnas al `select` y al `update` de `server/queries/settings.sql` y
      correr `make sqlc`.
- [X] T004 Exponerlas en `server/internal/app/settings.go` y `server/internal/httpapi/handlers_settings.go`
      según [contracts/api.md](./contracts/api.md).
- [X] T005 [P] Reflejarlas en `web/src/api/pos.ts` (tipo `BusinessSettings` y el cuerpo del PUT).
- [X] T006 [P] Agregar los controles a `web/src/features/admin/BusinessSettingsPage.tsx`: el
      interruptor de solo-PIN y los dos tiempos. El texto dice **qué hace**, no por qué — el porqué
      vive en la migración.

**Checkpoint**: los ajustes se leen y se guardan; nada más cambió.

---

## Fase 2: US2 — La tableta se bloquea sola (P1)

**Independiente**: se puede probar y entregar sin el cambio de operador. Ya entrega valor —la
tableta deja de quedarse abierta— aunque desbloquear sea todavía con la misma persona.

- [X] T007 [US2] Escribir `web/src/features/auth/inactividad.test.ts` **antes** del código: el
      temporizador vence tras el tiempo configurado, cualquier interacción lo reinicia, y un tiempo
      de 0 o negativo se trata como "no bloquear" en vez de bloquear a cada instante.
- [X] T008 [US2] Implementar `web/src/features/auth/useInactividad.ts`: temporizador del CLIENTE,
      reiniciado por interacción. No habla con el servidor — ver hallazgo 7 de
      [research.md](./research.md).
- [X] T009 [US2] Escribir el test que fija FR-011 y SC-006 **antes** de pintar la pantalla: desde el
      bloqueo se puede llegar a entrar con usuario y contraseña. Sin ese camino, quien olvida su PIN
      a media noche queda encerrado fuera del punto de venta con el local abierto, y es la clase de
      salida que se cae en un refactor sin que nadie lo note hasta esa noche.
- [X] T010 [US2] Crear `web/src/features/auth/LockScreen.tsx`: cubre la aplicación cuando está
      bloqueada. Controles de 44 px, sin `<select>` nativo, y el camino de "entrar con usuario y
      contraseña" **visible**, no escondido.
- [X] T011 [US2] Montarla en `web/src/App.tsx` de modo que cubra el POS sin desmontarlo: lo
      capturado tiene que seguir ahí al desbloquear.
- [X] T012 [US2] Escribir el test que fija FR-002: con una cuenta capturada, bloquear y desbloquear
      la deja intacta. Va contra el store `egb:ticket:v2`, que ya sobrevive a una recarga — el test
      existe para que nadie rompa esa garantía moviendo el estado a memoria.

**Checkpoint**: la tableta se bloquea sola, no pierde nada, y quien olvidó su PIN tiene salida. Entregable.

---

## Fase 3: US1 — Cobrar en la estación que esté libre (P1)

**Depende de US2**: la pantalla de bloqueo es donde se cambia de operador.

- [X] T013 [US1] Agregar a `server/queries/users.sql` la consulta de personas **activas y con PIN**
      para la rejilla, devolviendo **solo id y nombre** (contrato: es una lista que se pinta en un
      mostrador a la vista del público).
- [X] T014 [US1] Escribir el test de `GET /auth/unlock-options` en
      `server/internal/integration/unlock_options_test.go`: no lista a quien no tiene PIN ni a los
      inactivos, no devuelve correo ni rol, y con solo-PIN encendido devuelve la lista **vacía**.
- [X] T015 [US1] Escribir el test de FR-012 en el mismo archivo: **quien no tiene PIN sigue pudiendo
      entrar con usuario y contraseña**. No basta con verificar que no sale en la rejilla — hoy 2 de
      8 usuarios activos no tienen PIN y no pueden quedar encerrados fuera por una funcionalidad que
      no eligieron.
- [X] T016 [US1] Implementar el endpoint en `server/internal/httpapi/handlers.go` y su ruta.
- [X] T017 [US1] Escribir `server/internal/integration/pin_switch_conserva_reloj_test.go`
      **antes** de tocar el servicio: al cambiar de operador, el `expires_at` de la sesión se
      **conserva** y el refresh de quien estaba queda revocado. Es el hallazgo 2 de la investigación
      y lo que hace que `session_hours` signifique algo.
- [X] T018 [US1] Modificar `PinSwitch` en `server/internal/app/auth.go` para que cambie el operador
      conservando el vencimiento, en vez de emitir una sesión nueva con el plazo completo.
- [X] T019 [US1] Escribir el test de FR-010 **antes** de tocar el handler: un id que no existe y un
      PIN incorrecto devuelven **la misma respuesta**, y la latencia no los distingue. La igualación
      ya existe con `auth.CheckDummySecret`; el test es lo que impide que un refactor la quite sin
      que nadie note que el endpoint pasó a ser un enumerador de usuarios.
- [X] T020 [US1] Escribir el test del control de frontera de T021: con solo-PIN **apagado**, un
      `userId` ausente se **rechaza**. La constitución lo pide explícitamente — un parámetro de
      frontera inválido no cae a un default en silencio, y aquí el default silencioso sería aceptar
      cualquier PIN sin saber de quién.
- [X] T021 [US1] Aceptar `userId` opcional en el handler de `pin-switch`, rechazando su ausencia
      cuando el negocio NO tiene solo-PIN.
- [X] T022 [US1] Escribir el test de FR-015 **antes**: un desbloqueo fallido deja un evento de
      seguridad con clave estable, y ese evento **no contiene el PIN** ni datos personales. El
      principio V no deja mergear un control de seguridad sin su test, y un evento que filtre el
      secreto es peor que no tenerlo.
- [X] T023 [US1] Registrar el desbloqueo fallido con `logging.SecurityEvent`.
- [X] T024 [US1] Conectar la rejilla y el teclado de PIN en `LockScreen.tsx` contra
      `posApi.pinSwitch`, que ya existe y nadie usa.
- [X] T025 [US1] Hacer que `web/src/stores/session.ts` cambie de operador sin recargar la
      aplicación, conservando las cuentas abiertas (FR-013).
- [X] T026 [US1] Escribir el test de integración que cierra la historia: dos personas cobran en la
      misma estación identificándose por PIN, y el arqueo las separa. Reusa la tabla "Cobrado por"
      que ya existe.

**Checkpoint**: dos personas cobran en una estación y el arqueo lo refleja. Es el objetivo de la feature.

---

## Fase 4: US3 — La sesión no dura más que un turno (P2)

**Lo más delicado**: toca la política de sesión. Se hace con lo anterior estable.

- [X] T027 [US3] Escribir `server/internal/integration/sesion_caduca_test.go` **antes**: pasado el
      plazo, el refresh se rechaza y hace falta usuario y contraseña; dentro del plazo, sigue
      funcionando.
- [X] T028 [US3] Hacer que el vencimiento del refresh salga de `session_hours` del negocio en vez de
      la constante de 30 días, en `server/internal/app/auth.go`.
- [X] T029 [US3] Manejar en el cliente el refresh rechazado por caducidad: mandar al login completo
      con un mensaje que diga que la sesión terminó, no un error genérico.
- [X] T030 [US3] Documentar en [quickstart.md](./quickstart.md) qué se hace con los refresh tokens
      **ya emitidos**, que traen 30 días y no se acortan solos. Es decisión de despliegue y tiene
      que quedar escrita antes de desplegar, no después.

**Checkpoint**: ninguna tableta queda autenticada más allá del turno.

---

## Fase 5: US4 — El modo de solo-PIN (P3)

**Solo si se pide.** Es donde está el riesgo y el menor beneficio.

- [ ] T031 [US4] Escribir `server/internal/domain/pin_test.go` **antes**: el largo mínimo sube a 6
      con solo-PIN y se queda en 4 sin él; se mantienen las reglas actuales contra secuencias y
      todo-iguales.
- [ ] T032 [US4] Implementar esas reglas como funciones puras en `server/internal/domain/pin.go`.
- [ ] T033 [US4] Escribir `server/internal/integration/pin_unico_test.go`: no se puede encender
      solo-PIN con PINs cortos o repetidos, y el error **nombra a quiénes** hay que corregir.
- [ ] T034 [US4] Implementar la compuerta en `server/internal/app/settings.go`, comparando el PIN
      candidato contra los hashes de las personas activas — bcrypt saliniza, así que un índice único
      sobre el hash no detectaría nada.
- [ ] T035 [US4] Validar la unicidad al fijar un PIN en `server/internal/app/users.go`. El rechazo
      **no dice de quién** es el PIN repetido: diría el oráculo para averiguarlo probando.
- [ ] T036 [US4] Deducir la persona por el PIN en `PinSwitch` cuando el modo está activo, sin
      cambiar la respuesta ni la latencia entre "no existe" y "PIN incorrecto" — el test de T019
      tiene que seguir pasando con el modo encendido.
- [ ] T037 [US4] Ocultar la rejilla de nombres en `LockScreen.tsx` cuando el modo está activo.

**Checkpoint**: un negocio puede elegir el modo rápido sin quedar en un estado donde dos personas se
desbloqueen mutuamente.

---

## Fase 6: Cierre

- [ ] T038 Verificar el recorrido completo de [quickstart.md](./quickstart.md) contra el ambiente de
      pruebas, en 1024×600 real. **Cronometrar el cambio de operador** — SC-001 dice menos de 5
      segundos y es la única forma de verificarlo.
- [ ] T039 [P] Correr `tablet-ui-reviewer` sobre `LockScreen.tsx` y la pantalla de ajustes: es
      pantalla nueva y el subagente corre ante cualquier pantalla nueva.
- [ ] T040 [P] Correr `security-auditor` sobre el cambio: toca autenticación, sesión y logging, que
      es exactamente su disparador.
- [ ] T041 Actualizar `docs/security-owasp.md` con la política de sesión nueva — hoy documenta la de
      30 días.

---

## Dependencias

```text
Fase 1 (ajustes)  ──►  US2 (bloqueo)  ──►  US1 (cambio de operador)  ──►  US3 (caducidad)
                                                                     └──►  US4 (solo-PIN, P3)
```

- **US2 es el primer entregable**: se puede desplegar sola.
- **US1 necesita US2** porque la pantalla de bloqueo es donde se cambia de operador.
- **US3 va después de US1** porque el cambio de operador tiene que conservar el reloj antes de que
  el reloj signifique algo.
- **US4 no bloquea a nadie**: es un ajuste opcional al final.

## Paralelismo

- T005 y T006 (frontend de ajustes) van en paralelo con T003–T004 (backend).
- T013–T016 (endpoint de la rejilla) van en paralelo con T017–T018 (el servicio de PIN).
- T039 y T040 (los dos subagentes) van juntos al final.

## MVP sugerido

**Fase 1 + US2.** La tableta se bloquea sola, no pierde nada capturado, y quien olvida su PIN tiene
salida. Ya reduce el riesgo de la tableta abierta sin tocar la política de sesión ni la atribución.

El valor completo llega con **US1**, que es donde el desglose por cajero del arqueo empieza a
significar algo.
