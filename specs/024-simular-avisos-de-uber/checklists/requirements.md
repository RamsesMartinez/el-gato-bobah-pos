# Specification Quality Checklist: Simular los avisos de Uber

**Purpose**: Validate specification completeness and quality before proceeding to planning
**Created**: 2026-09-21
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
- [x] Success criteria are technology-agnostic
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

**Cero marcadores de duda, y eso fue una decisión, no un descuido.** Las tres cosas que podrían
haberlo sido se resolvieron con defaults que el propio repositorio ya tiene decididos:

- **Dónde viven los escenarios.** El dueño preguntó si caben en `http/`. Ese directorio es de SOLO
  LECTURA a propósito —ningún endpoint que escriba en Uber vive ahí, porque el `PUT` de menú es
  reemplazo total sin deshacer—. Un simulador escribe contra NUESTRA API, no contra Uber, así que la
  garantía no se rompe; pero mezclarlos haría que un archivo de ese directorio sí dispare una
  escritura, y esa distinción se pierde en un mes. El spec deja el «dónde» al plan, y nombra la
  restricción para que se decida a conciencia.
- **Cómo se marca un pedido simulado.** Se decidió que la marca **viaja en el aviso** y no se
  deduce del ambiente ni de la dirección de origen. Deducirla es adivinar, y lo que está en juego es
  la integridad de un corte de caja.
- **Qué se limpia.** Se copia el criterio que la suite e2e ya usa: se anota qué había antes y se
  cierra solo lo propio. Cerrarle a alguien una cuenta viva es peor que dejar basura.

**Lo que más pesa del spec no es una funcionalidad**: es la historia 3, que no agrega nada y es P1.
Un pedido simulado entra al POS como venta real, así que contaminar producción no se descubre hasta
que un corte no cuadra — y para entonces ya no se puede saber qué venta fue de verdad. Por eso la
defensa va en los dos lados: quien dispara se niega, y quien recibe rechaza.

**Dependencia dura**: esta feature se apoya en la 021, que todavía no está mezclada a `develop`.
No se puede implementar antes que ella.
