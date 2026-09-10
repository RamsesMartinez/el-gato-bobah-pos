# Specification Quality Checklist: Conteo de efectivo por denominaciones

**Purpose**: Validate specification completeness and quality before proceeding to planning
**Created**: 2026-08-31
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

## Notas

Las cuatro decisiones del prompt original quedaron resueltas:

1. **El desglose se guarda** (FR-007, US3). Sin él, un corte con faltante vuelve a ser un número sin
   historia, que es el problema que este spec existe para cerrar.
2. **Dos caminos excluyentes** (FR-014): contar por denominaciones, o escribir el total con un
   motivo obligatorio. Nunca los dos.
3. **No pueden coexistir** (FR-015), así que no hay nada que reconciliar. Es la lectura estricta de
   "no deja continuar hasta que cuadren": no deja que existan dos cifras. La alternativa —dejarlos
   convivir y bloquear si difieren— haría inútil el campo de total, porque su única razón de ser es
   expresar lo que el conteo no puede.
4. **La disposición en 1024×600** queda como requisito verificable (FR-013), no como solución: el
   cómo es trabajo de `/speckit-plan`.

De ahí sale FR-016, que es lo que las tres primeras compran juntas: **ningún arqueo queda con una
cifra suelta**. O tiene desglose, o tiene el motivo de por qué no lo tiene.

Listo para `/speckit-plan`.
