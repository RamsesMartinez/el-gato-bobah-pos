# Specification Quality Checklist: La hora del negocio manda

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

## Las cinco preguntas que el dueño pidió resolver, y su respuesta

1. **El pedido abierto que nadie va a cerrar.** Se queda en la lista —es lo que fuerza a resolverlo—
   pero **se distingue a la vista de los del día** (FR-010). La salida es cancelarlo, que ya existe.
   No se inventa un barrido automático: un pedido que se borra solo es una venta que desaparece sin
   que nadie decida.
2. **Los tres modos de corte.** En este negocio "medianoche" y "cierre de caja" caen a horas
   distintas pero casi siempre con el mismo efecto visible. Se construyen los tres porque el producto
   se vende a negocios con otros horarios, y queda anotado en las suposiciones que dos de ellos hoy
   no se usan — el plan tiene que decir cuánto cuestan.
3. **Dónde vive la conversión.** El spec exige que sea **un solo lugar** (FR-007) y que una pantalla
   nueva la herede sin acordarse; cuál mecanismo concreto es decisión del plan, no del spec.
4. **La zona inválida.** Cae al default, sigue funcionando y deja constancia (FR-006). Ni pantalla
   caída ni UTC en silencio.
5. **El aviso al cambiar de zona.** Sí, y dice las dos cosas: las horas mostradas cambian, las ventas
   ya registradas no se mueven de día (FR-017). Es informativo, no una confirmación de doble paso.

## Notas de la validación

Dos cosas que la primera pasada dejó mal y se corrigieron:

- **Los nombres de archivo se salieron del spec.** La primera versión citaba `utils/format.ts`,
  `ListDeliveredToday` y los `toLocaleString`. Eso es plan, no spec. Quedaron en el contexto medido,
  que es donde el dato pertenece, y fuera de los requisitos.
- **Faltaba el borde del horario de verano.** El corte "a la medianoche" no es "24 horas después del
  anterior": dos veces al año esa distancia es de 23 o 25 horas, y un corte que suma horas fijas se
  desfasa justo el día que cambia el horario.
