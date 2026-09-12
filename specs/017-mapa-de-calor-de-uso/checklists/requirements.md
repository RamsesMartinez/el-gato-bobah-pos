# Specification Quality Checklist: Un mapa de calor de uso

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

Las dos preguntas que habrían sido `[NEEDS CLARIFICATION]` —qué mide y si distingue a la persona—
se resolvieron CON EL DUEÑO antes de escribir el spec, no después. Quedan en *Origen*.

Tres cosas que esta revisión sí movió:

- **US2 (por rol) subió a P1.** Estaba como P2 por parecer una capa de privacidad que se agrega
  encima. No lo es: si el primer evento se escribe con la identidad, ya se registró, y quitarlo
  después no borra lo guardado. La decisión se toma antes de la primera fila.
- **Nació FR-009** (no ofrecer el corte por rol cuando identifica por eliminación). Sin él, FR-002
  es una promesa que se rompe en la pantalla: «el rol gerente hizo 40 acciones» en una empresa con
  un gerente es decir su nombre.
- **FR-007 dejó de ser una preferencia técnica.** El «sin librerías» del pedido es evitar licencias
  y peso en la tableta, y así quedó escrito — no como gusto de implementación, que el checklist
  habría marcado como detalle técnico filtrado.

Lo que este spec **no** resuelve y el plan tiene que fijar con números: el techo de espacio y el
periodo de conservación (FR-011).
