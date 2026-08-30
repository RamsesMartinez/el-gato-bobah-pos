# Specification Quality Checklist: Venta por plataformas digitales con listas de precios propias

**Purpose**: Validate specification completeness and quality before proceeding to planning
**Created**: 2026-08-29
**Feature**: [spec.md](../spec.md)

## Content Quality

- [x] No implementation details (languages, frameworks, APIs)
- [x] Focused on user value and business needs
- [x] Written for non-technical stakeholders
- [x] All mandatory sections completed

## Requirement Completeness

- [x] No [NEEDS CLARIFICATION] markers remain
- [x] Requirements are testable and unambiguous
- [x] Success criteria are measurable
- [x] Success criteria are technology-agnostic (no implementation details)
- [x] All acceptance scenarios are defined
- [x] Edge cases are identified
- [x] Scope is clearly bounded
- [x] Dependencies and assumptions identified

## Feature Readiness

- [x] All functional requirements have clear acceptance criteria
- [x] User scenarios cover primary flows
- [x] Feature meets measurable outcomes defined in Success Criteria
- [x] No implementation details leak into specification

## Notes

Las cuatro decisiones que normalmente habrían quedado como `[NEEDS CLARIFICATION]` se resolvieron
con el dueño **antes** de escribir la spec, así que entran como requisitos y no como preguntas:

| Decisión | Resuelta como |
|---|---|
| Cómo se define el precio de plataforma | Margen % sobre el base + excepciones manuales (FR-001, FR-002) |
| Qué pasa sin precio de plataforma | Nunca bloquea; se vende con el calculado (FR-004) |
| Quién edita y desde dónde | El cajero, desde la pantalla de venta, y persiste (FR-005, FR-006) |
| Modificadores | Llevan margen **y** admiten precio manual por opción (FR-008) |

Dos cosas que el dueño acotó explícitamente y que la spec respeta como **fuera de alcance**: la
pantalla de configuración del margen (el 35% va sembrado en migración) y cualquier integración con
las APIs de las plataformas.

Riesgo señalado para `/speckit-plan`, no para la spec: FR-011 ("al cambiar de lista, las líneas ya
agregadas cambian de precio") y FR-012 (el servidor recalcula todo) se tienen que resolver juntas, o
el ticket en pantalla y el cobro real pueden diferir. Es el punto que más merece test.
