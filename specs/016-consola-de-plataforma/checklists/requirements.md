# Specification Quality Checklist: La consola de plataforma

**Purpose**: Validate specification completeness and quality before proceeding to planning
**Created**: 2026-09-11
**Feature**: [spec.md](../spec.md)

## Content Quality

- [X] No implementation details (languages, frameworks, APIs)
- [X] Focused on user value and business needs
- [X] Written for non-technical stakeholders
- [X] All mandatory sections completed

## Requirement Completeness

- [X] No [NEEDS CLARIFICATION] markers remain
- [X] Requirements are testable and unambiguous
- [X] Success criteria are measurable
- [X] Success criteria are technology-agnostic (no implementation details)
- [X] All acceptance scenarios are defined
- [X] Edge cases are identified
- [X] Scope is clearly bounded
- [X] Dependencies and assumptions identified

## Feature Readiness

- [X] All functional requirements have clear acceptance criteria
- [X] User scenarios cover primary flows
- [X] Feature meets measurable outcomes defined in Success Criteria
- [X] No implementation details leak into specification

## Notes

**Lo que esta revisión recortó del pedido, y por qué.** El dueño pidió en una sola etapa mapa de
calor, salud del sistema, lista de empresas y acciones rápidas —reseteo de usuarios y contraseñas—.
Se partió en tres specs:

| | Qué queda | Por qué separado |
|---|---|---|
| 016 (ésta) | La consola y **una** pantalla de solo lectura | Prueba identidad, despliegue y lectura cruzada sin poder destructivo |
| 017 | El mapa de calor | Lo irreversible es recolectar el dato; la pantalla se monta encima |
| 018 | Reseteo de contraseñas y usuarios | **No es leer de más: es suplantar al dueño de otro negocio.** Necesita bitácora, revisión adversarial y probablemente segundo factor |

**La reserva que quedó escrita.** Un acceso que cruza empresas es un bypass del aislamiento que
sostiene el producto entero. Se dijo, el dueño decidió aceptarlo priorizando velocidad, y la
mitigación acordada es FR-005 y FR-006: ese permiso **no alcanza al dinero ni a la operación**, y la
barrera vive también en la base y no solo en el código. Sin esas dos, este spec no debería
implementarse.

**FR-003 no es cosmético.** Rechazar con la misma respuesta *y la misma latencia* es lo que impide
descubrir por temporización que una cuenta de plataforma existe. El repo ya cierra esa fuga en el
login del negocio con `auth.CheckDummySecret`; aquí aplica igual.

**Lo que el plan tiene que resolver y este spec no fija**: cómo se impone FR-006 en la base, y cómo
se crea el primer operador (FR-014) sin pantalla pública ni credencial en el código.


## Post-revisión de arquitectura (2026-09-11)

Seis hallazgos; **cuatro eran defectos de este plan**, no del pedido. Los dos graves:

| # | Qué | Consecuencia si no se corrige |
|---|---|---|
| 1 | El `Down` del rol copiaba el patrón de la 0024, que **no es reversible en este clúster** | Un `goose down` revienta por los grants del rol en las otras cuatro bases. Verificado contra Postgres real |
| 2 | FR-007 exigía dos cosas imposibles hoy | «Esquema por empresa» no existe —una sola base, un solo número— y «última actividad» exige leer tablas que FR-005 prohíbe. El AC2 de US2 describía un escenario **irrealizable** |

El segundo es un descuido propio: `data-model.md` ya decía *«conviene que queden en el spec antes de
implementar»* y el spec no se había corregido. Ya está: FR-007 reescrito y FR-007b creado.

Los otros cuatro, todos del lado del front y del arranque: el chequeo anti-contaminación era un
`grep` manual sobre un paquete minificado (ahora es la regla de `eslint` que ya existe y corre en
cada commit); `tsc` cubría la consola y **un typo suyo habría bloqueado el deploy del POS**; el
segundo build podía **borrar** el `dist/` del POS antes de subirlo; y la consola habría heredado el
manifiesto de la PWA del restaurante.

**Lo que la revisión confirmó sin objeción**: grants explícitos en vez de `on all tables`, la
simetría que sale gratis, `platform_operators` sin RLS, `staff.` en vez de `admin.`, y que ninguna
puerta del principio VIII se cierra.


## Post-analyze (2026-09-11)

Cinco hallazgos; **uno crítico que habría hecho imposible cumplir FR-007**.

**El crítico**: el plan daba por hecho que «no ser superusuario» bastaba para que el rol de la
consola leyera `companies`. No es así — esa tabla lleva la política RLS `company_self`, y **RLS
también le aplica al rol de plataforma**. Verificado creando el rol de verdad: con solo `select`
veía **1 empresa de 2**. La consola habría listado una sola, o ninguna.

Arreglo verificado: una política acotada a **una tabla, un comando y un rol** —
`for select to gatobobah_platform using (true)`—, con la que pasa a ver 2 de 2 mientras
`gatobobah_app` sigue viendo solo la suya. **No** `BYPASSRLS`: resolvería esto y abriría todo lo
demás el día que alguien agregue un grant por comodidad.

Los otros cuatro, todos huecos de cobertura:

| | Qué faltaba |
|---|---|
| FR-009 | Nadie probaba que la consola **no pueda escribir**. Los grants son de `select`, así que la escritura está denegada **por omisión** — y lo omitido no se nota hasta que alguien lo agrega. Es lo que impide que la spec 018 ocurra por accidente |
| FR-016 | Sin verificar que no viajen datos de empleados de un cliente |
| FR-007b | Sin verificar que la última actividad **no** viaje todavía |
| T002 | Trabajo fuera del repositorio que ningún test puede comprobar; ahora va marcado como manual |

Cobertura final: **17 de 17 FR** con tarea, 6 de 6 SC. 44 tareas.
