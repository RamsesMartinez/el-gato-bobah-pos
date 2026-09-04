# Specification Quality Checklist: La fecha la da el reloj, el folio lo da el turno

**Purpose**: Validate specification completeness and quality before proceeding to planning
**Created**: 2026-09-04
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

Cero marcadores de clarificación. Las cinco decisiones que el dueño dejó abiertas se resolvieron
dentro del spec, y cada una dice contra qué se resolvió:

1. **Reinicio del folio al reabrir la caja el mismo día** — se acepta. El argumento no es de gusto:
   cerrar un turno ya exige que no queden pedidos vivos, así que un reinicio no puede colisionar
   con nada en curso. Queda como caso de borde con su prueba.
2. **La bolsa de nombres pasa a ser por turno**, igual que el número, para que folio y nombre
   respondan a una sola cosa (FR-004).
3. **La corrección histórica se aplica**, y se decidió midiendo antes: 0 de 31 filas del negocio
   en operación, 2 de 61 de la cuenta de pruebas. FR-007 y FR-008.
4. **El aviso de turno viejo** es la historia 4, no bloqueante (FR-012, FR-013).
5. **Cuántas ventas caben en el detalle del corte**: sin controles de paginación, y si se muestra
   un subconjunto tiene que decir cuántas hay (FR-011).

Dos frases se reescribieron en la validación por hablar como programador y no como operador
("serialización", "columna").

Riesgo que el plan debe cerrar y que este documento no puede: continuar la numeración de los
turnos que ya están abiertos (FR-006). Sin eso, el turno de 158 pedidos del ambiente de pruebas
volvería a repartir el número 1.
