# Specification Quality Checklist: Leer el menú de las plataformas y decir en qué difiere del nuestro

**Purpose**: Validate specification completeness and quality before proceeding to planning
**Created**: 2026-09-14
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

**Dos correcciones aplicadas durante la validación:**

1. La primera redacción de FR-017 decía «comparar el precio». Quedaba ambiguo contra cuál: el POS
   tiene precio de mostrador **y** precio por plataforma desde la migración 0037. Comparar contra el
   de mostrador reportaría una diferencia en cada renglón todos los días, y la pantalla sería
   inútil el primer día. Ahora lo dice.

2. Faltaba el caso de la lectura vacía. Un menú que vuelve sin productos y se compara como dato
   válido reporta que **todo el catálogo sobra arriba** — el peor reporte posible, y el que más
   invita a una acción destructiva. Está como FR-005 y como edge case.

**Nombres de plataforma fuera del spec, a propósito.** El documento nombra a Uber, Rappi y DiDi solo
en el contexto y en las dependencias; los requisitos hablan de «una plataforma». La razón no es
estilo: cuál se conecte primero **no lo decide el diseño, lo decide cuál conteste**, y un requisito
escrito contra una plataforma concreta habría que reescribirlo cuando conteste otra.

**Lo que este spec deliberadamente no resuelve y el plan tendrá que decidir:**

- Si la lectura se dispara sola cada cierto tiempo o solo cuando alguien la pide. El spec exige que
  toda comparación diga de cuándo es (FR-002, FR-004); con eso, las dos opciones cumplen.
- Cómo se guardan las credenciales. FR-021 y FR-022 fijan el resultado, no el mecanismo.
- Qué tan parecidos tienen que ser dos nombres para proponer una pareja. FR-011 exige que sea
  propuesta y no hecho consumado, que es lo que vuelve seguro equivocarse.
