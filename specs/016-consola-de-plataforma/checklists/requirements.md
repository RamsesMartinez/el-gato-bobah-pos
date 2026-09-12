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
