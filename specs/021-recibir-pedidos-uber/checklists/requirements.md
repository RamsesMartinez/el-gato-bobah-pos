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

## Revisión de arquitectura — 2026-09-17

`db-architect` y `tablet-ui-reviewer` corrieron sobre el plan (hook `after_plan`). **Veredicto:
cambios requeridos**, todos aplicados antes de `/speckit-tasks`.

Lo que cambió de verdad, no de redacción:

- **La migración no corría**: faltaban los índices `(company_id, id)` en `users` y en
  `platform_connections`. Verificado contra Postgres real.
- **`platform_webhook_events` no podía recibir la fila que prometía**: sin empresa no hay
  `company_id` que fijar. El aviso de tienda desconocida ahora se rechaza antes de tocar la base, y
  SC-004 se ajustó para decirlo.
- **La red de seguridad no guardaba lo que decía**: `raw_body` es el aviso, no el detalle. Se agregó
  `raw_detail`.
- **Se revirtió el único global** de `(plataforma, tienda)`: rompería la prueba de un segundo
  cliente en sandbox. La ambigüedad la resuelve la firma.
- **El aviso quedaba tapado** por cualquier hoja abierta del POS. Va en su propio overlay, con dos
  escenarios de aceptación nuevos (US1 · 5 y 6).
- **La puerta de varias empresas pasó de «abierta» a «parcialmente abierta»**: recibir sí, decidir
  no, porque aceptar y rechazar salen con la identidad global de la 020.

## /speckit-analyze — 2026-09-17

Cobertura: **31 de 31 requisitos con al menos una tarea**. Sin hallazgos CRITICAL. Tres ALTOS, los
tres de `tasks.md`, ya corregidos:

- **La llave de firma se capturaba en la última fase** y sin ella no se puede verificar ni una
  firma: el MVP no se podía demostrar. Movida a fundacional (T011-T015). No se ve leyendo las fases
  por separado; se ve al preguntar qué hace falta para correr el primer test de firma.
- **Una tarea apuntaba a `server/internal/app/cash.go`, que no existe.** Abrir turno es
  `OpenSession` en `server/internal/app/backoffice.go:870`.
- **`orders` exige `client_uuid` y `daily_number`** y ninguna tarea decía de dónde salen.
  `platform_order_ref` **no** sustituye a `daily_number`: son columnas distintas, y `order_counters`
  es por `business_date`, así que funciona sin turno abierto. Queda escrito en T047.

Dos MEDIOS corregidos: la ventana de poda quedó fijada en **60 días** (el ciclo de pago de la
plataforma es mensual), y nueve tareas que juntaban implementación y test se partieron en dos, como
exige el principio IV.

Dos tareas sin requisito que las pida —instrumentar la pantalla nueva para la medición de uso— se
quedan: las pide `AGENTS.md`, no el spec.
