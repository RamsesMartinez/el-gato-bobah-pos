# Specification Quality Checklist: Bloqueo por inactividad y cambio de operador por PIN

**Purpose**: Validate specification completeness and quality before proceeding to planning
**Created**: 2026-09-01
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

Sin marcadores abiertos: el dueño resolvió las dos decisiones de fondo antes de levantarlo
—elegir persona y luego PIN por default, con solo-PIN como ajuste por negocio— y el resto sale de
medir el estado actual del sistema.

**Seis dígitos y no ocho** (Assumptions). Con seis y diez personas, un dedazo cae en el PIN de otro
9 veces en un millón; con cuatro, 9 en diez mil. Ocho no compra nada sobre seis y sí garantiza el
papelito pegado a la tableta, que es peor que un PIN corto.

**Lo que hace caro el modo de solo-PIN, y por eso queda en P3**: exige PINs únicos, y hoy el sistema
no lo pide en ningún lado. Los 6 usuarios con PIN lo tienen de 4 dígitos y sin garantía de ser
distintos entre sí, así que encender el ajuste sin migrar primero dejaría a dos personas
desbloqueándose la una a la otra. De ahí FR-006, FR-007 y FR-008.

**FR-011 —olvidé mi PIN— no es un adorno**: sin salida propia, la única persona con acceso a media
noche queda encerrada fuera del punto de venta con el local abierto.

Datos medidos que sostienen el spec: la sesión dura hoy **30 días** y un usuario llegó a tener
**4 vivas a la vez**; **6 de 8** usuarios activos tienen PIN; el cambio por PIN existe en el
servidor y **ninguna pantalla lo usa**.

Listo para `/speckit-plan`.
