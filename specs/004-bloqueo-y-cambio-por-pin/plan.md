# Implementation Plan: Bloqueo por inactividad y cambio de operador por PIN

**Branch**: `004-bloqueo-y-cambio-por-pin` | **Date**: 2026-09-01 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/004-bloqueo-y-cambio-por-pin/spec.md`

## Summary

Con dos estaciones sobre un solo cajón, el desglose por cajero que el arqueo ya muestra solo
significa algo si cada venta lleva a quien de verdad la cobró. Hoy no: la sesión dura 30 días, nada
revoca las anteriores —un usuario de producción tiene 4 vivas— y la tableta que alguien dejó abierta
el viernes atribuye a esa persona todo lo que se cobre el lunes.

El enfoque tiene tres piezas, y la tercera es la que sostiene a las otras dos:

1. **La pantalla de bloqueo y la de cambiar de operador son la misma.** Desbloquear *es*
   identificarse, así que no hay una acción aparte que alguien pueda saltarse.
2. **Por default se elige a la persona y luego se teclea el PIN.** El modo de solo-PIN es un ajuste
   por negocio, con compuerta: no se puede encender mientras haya PINs cortos o repetidos.
3. **El cambio de operador conserva el vencimiento de la sesión.** Es lo que impide que una tableta
   usada cada veinte minutos nunca caduque.

Lo grueso del trabajo es de pantalla y de política de sesión: el cambio por PIN **ya existe en el
servidor y ninguna pantalla lo usa**.

## Technical Context

**Language/Version**: Go 1.27 (backend), TypeScript / React 19 (frontend)

**Primary Dependencies**: chi, pgx + sqlc, goose, bcrypt, JWT (backend); Chakra UI v3, TanStack
Query, Zustand (frontend)

**Storage**: PostgreSQL con RLS por empresa. Tres columnas nuevas en `business_settings`; ninguna
tabla nueva.

**Testing**: `go test` (unitarios en `domain`, integración contra Postgres real), vitest

**Target Platform**: tabletas de 7 a 10 pulgadas, presupuesto real ~1024×600, táctil

**Project Type**: aplicación web (backend Go + frontend React), monorepo

**Performance Goals**: cambiar de operador en menos de 5 segundos de punta a punta (SC-001). El
temporizador de inactividad no debe generar tráfico: vive en el cliente.

**Constraints**: controles de al menos 44 px; prohibido `<select>` nativo; sin desplegables del
sistema operativo. El bloqueo no puede perder nada de lo capturado.

**Scale/Scope**: 8 usuarios activos por negocio, 2 estaciones. Dos pantallas nuevas (bloqueo y
ajustes), un endpoint nuevo, dos que cambian.

## Constitution Check

*GATE: revisado antes de Fase 0 y de nuevo tras el diseño de Fase 1.*

| Principio | Cómo lo cumple este plan |
| --- | --- |
| **I. Layering estricto** | La regla de largo y unicidad del PIN va a `domain` como función pura. El servicio de auth orquesta; el handler solo decodifica y mapea. Ninguna consulta fuera de sqlc |
| **II. Errores envueltos** | Reusa `domain.ErrValidation` y `ErrInvalidCredentials`. El 422 de "PINs que no cumplen" necesita un sentinel nuevo que cargue **a quiénes**, envuelto con `%w` |
| **III. Dinero** | No aplica: esta feature no toca dinero. Sí toca **a quién se le atribuye**, que es lo que vuelve confiable el desglose del arqueo |
| **IV. Test-first, lógica en `domain`** | La validación del PIN y la decisión de qué pedir al desbloquear son funciones puras con test. La caducidad y el cambio de operador conservando el reloj son de **integración**: dependen del tiempo y de la base |
| **V. Seguridad adversarial** | Es el corazón de la feature. Detalle abajo |
| **VI. YAGNI** | Sin tabla nueva, sin PIN maestro, sin biometría. El modo de solo-PIN es P3 y se construye solo si se pide |
| **VII. Comentarios del porqué** | Cada decisión no obvia —por qué el reloj se conserva, por qué la lista no dice quién tiene el PIN repetido— va como comentario donde vive |

### Principio V, en detalle

| Pregunta adversarial | Respuesta concreta |
| --- | --- |
| ¿Puedo cobrar a nombre de otro? | No. La atribución sale del **token**, no de un campo de la petición. Hace falta el PIN de esa persona |
| ¿Puedo enumerar usuarios probando ids? | No. La rama de "no existe" ya corre `CheckDummySecret` para igualar la latencia, y la respuesta no distingue id de PIN |
| ¿Puedo hacer fuerza bruta a un PIN de 4 dígitos? | El lockout **per-usuario** ya existe; `pin-switch` está exento del throttle per-IP a propósito. Falta registrar el intento fallido como evento de seguridad |
| ¿Puedo averiguar el PIN de un compañero? | No. Al rechazar un PIN repetido, el mensaje **no dice de quién** — si lo dijera, el formulario sería un oráculo |
| ¿Puedo dejar una sesión viva para siempre? | Ya no: desbloquear conserva el vencimiento en vez de reponerlo. Es el hallazgo 2 de la investigación |
| ¿Se filtra la plantilla del negocio? | La rejilla muestra **solo id y nombre**, y con solo-PIN encendido no muestra a nadie |
| ¿Un PIN llega a un log? | No. El evento de seguridad lleva clave estable y usuario, nunca el secreto |

**Sin violaciones que justificar.** La sección de complejidad va vacía.

## Project Structure

### Documentation (this feature)

```text
specs/004-bloqueo-y-cambio-por-pin/
├── plan.md              # Este archivo
├── research.md          # Fase 0 — lo medido sobre el código y producción
├── data-model.md        # Fase 1 — columnas, reglas y lo que NO cambia
├── quickstart.md        # Fase 1 — cómo verificarlo
├── contracts/api.md     # Fase 1 — endpoints que nacen y que cambian
├── checklists/
└── tasks.md             # Lo crea /speckit-tasks
```

### Source Code

```text
server/
├── migrations/
│   └── 00NN_ajustes_identificacion.sql    # 3 columnas en business_settings
├── queries/
│   ├── settings.sql                        # los 3 ajustes al select y al update
│   └── users.sql                           # personas activas con PIN, para la rejilla
├── internal/
│   ├── domain/
│   │   ├── pin.go                          # largo y unicidad: funciones puras
│   │   └── pin_test.go
│   ├── app/
│   │   ├── auth.go                         # PinSwitch conserva el vencimiento
│   │   ├── settings.go                     # compuerta al encender solo-PIN
│   │   └── users.go                        # valida el PIN contra la política
│   ├── httpapi/
│   │   ├── handlers.go                     # pin-switch con userId opcional
│   │   └── handlers_settings.go            # los 3 ajustes
│   └── integration/
│       ├── pin_switch_test.go              # el reloj se conserva; la sesión caduca
│       └── pin_unico_test.go               # la compuerta del modo solo-PIN

web/src/
├── features/auth/
│   ├── LockScreen.tsx                      # rejilla de nombres + teclado de PIN
│   ├── useInactividad.ts                   # temporizador, reiniciado por interacción
│   └── inactividad.test.ts
├── features/admin/
│   └── BusinessSettingsPage.tsx            # los 3 ajustes
└── stores/session.ts                        # el operador activo cambia sin recargar
```

**Structure Decision**: monorepo existente. Ninguna carpeta nueva salvo `web/src/features/auth/`,
que agrupa la pantalla de bloqueo con su temporizador — hoy la autenticación vive dispersa entre el
store y la pantalla de login, y esta feature agrega la tercera pieza.

## Fases

### Fase 0 — Investigación ✅

Ver [research.md](./research.md). Nueve hallazgos, todos medidos. El que cambia el diseño:
**`PinSwitch` emite hoy una sesión nueva con el plazo completo**, así que usarlo tal cual haría que
cada desbloqueo reiniciara el reloj del turno.

### Fase 1 — Diseño ✅

- [data-model.md](./data-model.md) — tres columnas, cero tablas nuevas, y qué NO cambia
- [contracts/api.md](./contracts/api.md) — un endpoint nuevo, cuatro que cambian
- [quickstart.md](./quickstart.md) — un recorrido por historia, con el fallo esperado de cada uno

### Fase 2 — Tareas

La genera `/speckit-tasks`. El orden que sugiere el diseño:

1. **Los ajustes** (migración, consultas, servicio, pantalla). Desbloquea todo lo demás y no cambia
   comportamiento por sí solo.
2. **US2, el bloqueo por inactividad.** Es la que se puede probar sola y ya entrega valor: la tableta
   deja de quedarse abierta.
3. **US1, el cambio de operador.** Necesita 2 y es donde la atribución empieza a servir.
4. **US3, la caducidad.** Toca la política de sesión, que es lo más delicado: se hace con las tres
   anteriores estables.
5. **US4, el modo de solo-PIN.** P3, con su compuerta. Se construye si se pide.

## Riesgos, y qué hace el plan con cada uno

| Riesgo | Qué haría | Cómo se cierra |
| --- | --- | --- |
| Los refresh tokens ya emitidos traen 30 días | La caducidad no aplicaría hasta que cada tableta vuelva a entrar, semanas después | El despliegue decide a propósito qué hacer con ellos; el quickstart dice cómo comprobarlo |
| El bloqueo pierde lo capturado | El operador aprende a impedir el bloqueo y la protección se cae sola | El carrito ya vive en `localStorage` y sobrevive a una recarga. Se fija con test |
| Encender solo-PIN con los PINs de hoy | Dos personas se desbloquean la una a la otra y el arqueo miente | La compuerta lo impide y **nombra** a quiénes corregir |
| El temporizador de inactividad genera tráfico | Cada toque en cada tableta pegándole al servidor | El temporizador es del cliente; el servidor solo aplica la caducidad del refresh |
| La rejilla no cabe en 1024×600 | Se desplaza y el desbloqueo deja de ser un tap | Son 6 personas con PIN; el quickstart lo verifica en la resolución real |

## Complexity Tracking

Sin violaciones a la constitución. Nada que justificar.
