# Specification Quality Checklist: Un solo arqueo de cajón

**Purpose**: Validate specification completeness and quality before proceeding to planning
**Created**: 2026-09-10
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

Tres cosas que la validación cambió, y una que dejó marcada para `/speckit-plan`.

**Se quitó el CÓMO de tres lugares.** El spec nombraba la base de datos como el lugar donde hoy se
cambian los interruptores, y una consulta concreta en un caso de borde. Los dos decían algo
verdadero de la manera equivocada: para quien lee este documento el hecho es *"hoy cambiarlo exige a
un desarrollador"*, no en qué tabla vive.

**Cero [NEEDS CLARIFICATION], y no por comodidad.** Las dos preguntas que faltaban las contestó el
dueño antes de escribir el spec: quién recoge el efectivo de un pedido de app —*depende de la
plataforma*, que es lo que vuelve necesaria US2— y qué le faltaba al conteo manual —*arqueo ciego*,
que es US3. Lo demás salió por default razonable y quedó escrito en *Assumptions*: el ciego aplica a
todo lo que se declara y no solo al efectivo, la diferencia se ve al confirmar sin restringir por
rol, el interruptor es por empresa, y un cambio de configuración aplica al siguiente cierre.

**El caso de borde que más pesa no es de pantalla.** Desactivar un método que ya cobró dinero en el
turno abierto no puede quitar ese dinero del arqueo, y hoy el corte solo considera métodos activos:
es un camino existente, no hipotético. FR-008 lo fija y el plan tiene que decir cómo, porque es la
clase de cambio que hace desaparecer dinero en silencio.

**Lo que el plan tiene que resolver y este spec deliberadamente no decide**: a qué renglón del corte
guardado se le atribuye la diferencia única del cajón, ahora que ya no es de un método. La respuesta
afecta cómo se leen los cortes viejos (SC-005) y es una decisión de diseño, no de producto.

Listo para `/speckit-plan`.
