# Specification Quality Checklist: Dónde cae el dedo

**Purpose**: Validar que el spec está completo antes de planear
**Created**: 2026-09-12
**Feature**: [spec.md](../spec.md)

## Content Quality

- [X] Sin detalles de implementación (lenguajes, frameworks, APIs)
- [X] Centrado en el valor para quien decide
- [X] Escrito para quien no programa
- [X] Todas las secciones obligatorias completas

## Requirement Completeness

- [X] No quedan marcas [NEEDS CLARIFICATION]
- [X] Los requisitos son verificables y no ambiguos
- [X] Los criterios de éxito son medibles
- [X] Los criterios de éxito no nombran tecnología
- [X] Los escenarios de aceptación están definidos
- [X] Los casos de borde están identificados
- [X] El alcance está acotado (hay sección *Out of scope*)
- [X] Dependencias y supuestos identificados

## Feature Readiness

- [X] Cada requisito funcional tiene criterio de aceptación
- [X] Los escenarios cubren los caminos principales
- [X] La feature cumple los criterios de éxito declarados
- [X] No se filtran detalles de implementación

## Notas

Dos decisiones que el spec toma y que conviene releer antes de planear, porque cierran caminos:

- **FR-004** decide medir contra el **área visible** y no contra el contenido. Responde «qué parte
  del vidrio usa la mano», no «qué control se toca» — eso último ya lo contesta la 017 con las
  acciones con nombre. Si se quisiera lo segundo habría que guardar el elemento, y eso es otra cosa.
- **FR-003** prohíbe guardar el instante del toque. Sale de un defecto real encontrado en la 017: el
  reloj por evento se cruza con `register_sessions` y deshace el anonimato aunque no se guarde el
  nombre. Es lo que descarta de entrada cualquier diseño con un renglón por toque.
