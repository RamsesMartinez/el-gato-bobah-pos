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

- [ ] No [NEEDS CLARIFICATION] markers remain — **2 abiertos** (FR-014, FR-015), ver Notas
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

Dos decisiones quedaron abiertas a propósito, porque las dos cambian el alcance y ninguna tiene un
default defendible sin el dueño:

- **FR-014** (Q2): si se puede seguir escribiendo el total a mano o el conteo por denominaciones es
  el único camino.
- **FR-015** (Q3): qué manda cuando el conteo y un total escrito a mano no coinciden. Solo aplica si
  FR-014 admite las dos vías.

La decisión 1 del prompt original —si el desglose se guarda— **sí** se resolvió: se guarda (FR-007,
US3). Sin él, un corte con faltante vuelve a ser un número sin historia, que es el problema que este
spec existe para cerrar.

La decisión 4 —cómo se ve en 1024×600— se dejó como requisito verificable (FR-013) y no como
solución: la disposición concreta es trabajo de `/speckit-plan`, no del spec.

Ambos marcadores deben resolverse antes de `/speckit-plan`.
