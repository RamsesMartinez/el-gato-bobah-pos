# Specification Quality Checklist: Confirmar el pedido antes de cobrar

**Purpose**: Validar que el spec está completo antes de planear
**Created**: 2026-09-01
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

Tres cosas que la primera pasada dejó mal y se corrigieron:

- **Los nombres de archivo se salieron del spec.** La primera versión citaba `CheckoutSheet`,
  `useMandarPedido` y `stores/ticket.ts` en los requisitos. Eso es plan, no spec: un requisito que
  nombra un archivo ya decidió el diseño. Se movieron al plan y aquí quedó el comportamiento.
- **El orden de las historias estaba invertido.** "Cobrar exige confirmar" venía primero, pero
  entregarla sola deja el flujo PEOR que hoy: el pedido se confirma, desaparece, y cobrarlo cuesta
  más toques que antes. La historia del pedido en curso pasó a P1 y es el MVP.
- **Faltaba la pregunta del estado.** El spec original no decía si "confirmado" necesitaba un estado
  nuevo. La respuesta quedó en las suposiciones y en FR-017: el pedido NO necesita estado nuevo
  —confirmar es crearlo—, pero el RENGLÓN sí necesita saber si ya salió en una comanda, y eso es lo
  que hace posible imprimir solo lo agregado y recuperar una impresión que falló. Sin esa pregunta
  concreta, el estado habría sobrado.

## Lo que queda fuera a propósito

- **Quitar o editar renglones de un pedido en curso.** Necesita comanda de cancelación y una
  decisión sobre el dinero ya cobrado. Spec aparte. Hoy tampoco se puede.
- **Tiempo real entre estaciones.** El refresco periódico alcanza para dos tabletas en un local.
