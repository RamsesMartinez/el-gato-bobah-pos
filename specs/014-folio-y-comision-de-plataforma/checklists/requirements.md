# Specification Quality Checklist: El folio con el que llegó el pedido, y lo que la plataforma se quedó

**Purpose**: Validate specification completeness and quality before proceeding to planning
**Created**: 2026-09-06
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

## Notas de la validación

Cómo se verificó cada bloque, y las decisiones de redacción que lo sostienen:

- **Sin detalles de implementación, verificado y no supuesto**: se buscaron en el spec nombres de
  tabla y columna, extensiones de archivo de código, el nombre del motor de base de datos y los
  términos de aislamiento por empresa. Cero coincidencias. Los requisitos hablan de "identificador de
  plataforma del pedido" y "liquidación del pedido de plataforma"; cómo se guardan es del `plan.md`.
- **Las únicas rutas que el spec cita** son documentos de `docs/`, y viven en la sección de contexto
  medido, donde funcionan como evidencia de que una cifra no se inventó.
- **Cero marcadores de clarificación, a propósito.** Las ocho decisiones que el dueño dejó abiertas se
  resolvieron y quedaron en *Assumptions* con su porqué, que es donde se pueden discutir sin bloquear
  el plan. La única que no se resolvió —un pedido de plataforma sin turno de caja abierto— **no se
  escondió**: está declarada por su nombre, con la consecuencia de descubrirla tarde.
- **Lo que sí quedó pendiente de medir**: SC-003 y SC-007 hablan de toques y de renglones a 1024×600.
  Ninguno de los dos está medido todavía — son la vara que el plan tiene que contestar con un número,
  no afirmaciones ya verificadas.

## Riesgos que el plan tiene que atender, no el spec

Se anotan aquí porque son la vara con la que `/speckit-analyze` va a medir el plan:

1. **La unicidad del identificador toca el camino de cada venta.** El índice que la sostiene y la
   transacción que crea el pedido tienen que convivir sin perder la serialización que hoy hace segura
   la numeración de folios.
2. **El filtro nuevo de la pantalla de Ventas entra en una lista que ya tiene lista y resumen
   gemelos.** Es exactamente el patrón donde ya se rompió antes.
3. **La liquidación es dinero que no pasó por la caja.** El plan tiene que decir explícitamente por
   dónde NO entra a un total.
4. **El alto de la pantalla del POS.** El campo nuevo tiene que declarar cuántos renglones de producto
   quita a 1024×600, medido.
