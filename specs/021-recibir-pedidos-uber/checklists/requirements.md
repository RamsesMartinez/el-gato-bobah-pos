# Specification Quality Checklist: Recibir los pedidos de Uber Eats

**Purpose**: Validate specification completeness and quality before proceeding to planning
**Created**: 2026-09-16
**Feature**: [spec.md](../spec.md)

## Content Quality

- [x] No implementation details (languages, frameworks, APIs)
- [x] Focused on user value and business needs
- [x] Written for non-technical stakeholders
- [x] All mandatory sections completed

## Requirement Completeness

- [x] No [NEEDS CLARIFICATION] markers remain — los 2 que había los resolvió el dueño
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

Las dos decisiones abiertas las resolvió el dueño el 2026-09-16:

- **El plazo se deja expirar** (FR-027). El sistema no rechaza por su cuenta: eso sería software
  decidiendo no vender.
- **Aceptar no exige turno de caja** (FR-022) y el pedido entra al corte del siguiente turno, con
  la condición de que al **abrir** se vea de un vistazo qué pedidos ya vienen considerados
  (FR-023). Esa condición la puso el dueño y no estaba en la pregunta.

Y salió un requisito que nadie había visto: **la plataforma cierra la tienda** cuando se dejan
pedidos sin responder (FR-028, historia 5). Es el costo real de dejar expirar un pedido, es
invisible, y entra al alcance.

Lo que **no** se marcó como duda, y por qué, para que no se reabra en `/speckit-analyze`:

- El precio lo manda la plataforma: ya está documentado en `AGENTS.md`.
- Un platillo sin pareja no impide aceptar: rechazar un pedido pagado por un hueco de nuestra
  contabilidad interna no es una opción defendible.
- El pedido pendiente vive aparte del pedido del POS: lo exige el principio III.
- No hay aceptación automática: el operador decide, y hoy nada sabe si hay ingredientes.
