# Implementation Plan: Roles por empresa — entrega 1: el cajero ve los pedidos entregados

**Branch**: `068-roles-por-empresa` | **Date**: 2026-11-06 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `specs/068-roles-por-empresa/spec.md`

## Summary

Un permiso nuevo, `orders.delivered_view` («ver los pedidos entregados del día y reimprimir su
ticket»), en el mapa fijo de `domain.PermissionsFor` para admin, gerente y cajero. `GET
/orders/delivered` pasa de `RequireRole(admin, gerente)` a `RequirePermission(orders.delivered_view)`;
`POST /orders/{id}/refund` y `GET /orders/{id}/refund-info` pasan a `RequirePermission(payments.void)`
(mismo conjunto de roles hoy: admin y gerente). En la pantalla de Pedidos, `canRefund` (pregunta
por rol) se reemplaza por dos preguntas por permiso: `can('orders.delivered_view')` decide si se
pide y se pinta la sección; `can('payments.void')` decide si aparece «Devolver». Sin migración: los
permisos de los roles sugeridos no se guardan todavía en la base, así que el mapa reparte el permiso
a todas las empresas, nuevas y existentes, en la misma subida.

## Technical Context

**Language/Version**: Go 1.27.3 (server), TypeScript + React 19 (web)

**Primary Dependencies**: chi, pgx/sqlc (sin cambios de consultas), Chakra UI v3, TanStack Query

**Storage**: N/A — no cambia el esquema ni hay migración

**Testing**: `go test` (domain, httpapi), integración con build tag `integration` contra Postgres de pruebas, vitest

**Target Platform**: API Linux + tabletas 1024×600

**Project Type**: web-service + web app

**Performance Goals**: sin cambio (la consulta de entregadas ya existe; ahora la pide también la cajera, una tableta más cada 15 s)

**Constraints**: la API anterior y la nueva conviven durante la subida; la pantalla anterior y la nueva también

**Scale/Scope**: 1 permiso, 3 rutas, 1 pantalla (sin cambio de disposición)

## Constitution Check

| Principio | Cómo se cumple |
| --- | --- |
| I. Layering | El permiso vive en `domain/permission.go`; el gate en `httpapi/router.go`; la pantalla en `web/src/features/orders/`. Sin lógica nueva en handlers ni en `app` |
| II. Errores | El 403 sale de `RequirePermission` con `PermissionDeniedMessage` y el evento `forbidden` que ya emite |
| III. Dinero | No se toca ningún cálculo; devolver sigue exigiendo el permiso de sacar dinero |
| IV. Test-first | Primero: `TestPermissionsForEachRoleOfToday` y el de mensajes con el permiso nuevo; integración por ruta y rol (cajero ve entregadas y no devuelve; mesero no ve; gerente devuelve); vitest de la pantalla con cajero y mesero. Sin migración: no aplica el test contra respaldo ni `inTheThreeCases` (no hay tabla ni consulta nueva; la consulta de entregadas ya va bajo RLS) |
| IV. Migración tolera la API anterior | No hay migración. API anterior + pantalla nueva: la sesión no trae `orders.delivered_view` y la sección no aparece (igual que hoy para la cajera). API nueva + pantalla anterior: la pantalla pregunta por rol, la cajera no ve la sección, nada se rompe |
| V. Seguridad | Autorización en el backend (`RequirePermission`); el front solo espeja. «¿Un atacante lo evade?»: un cajero que pide `/refund` directo recibe 403 — lo cubre el test de integración por ruta |
| VI. YAGNI | Un permiso, sin tabla. Devolver reutiliza `payments.void` en vez de inventar `orders.refund` (misma capacidad) |
| VII. Idioma | Nombre del permiso en inglés y estable; mensajes en español |
| VIII. Puerta de roles | Esta entrega no la cruza: no guarda roles ni toca `users.role`. La deja más abierta: tres rutas y un `role === …` del front pasan de rol a permiso. La entrega 2 (issue nuevo) cruza la puerta y su diseño lo decide el dueño antes de construir |
| Roles: modelo híbrido | El código solo pregunta por permiso; el mapa rol → permisos sigue siendo la semilla que pasará a la base |
| Tableta 1024×600 | La sección de entregados ya existe y ya se mide; la cajera la ve igual que la gerente, sin «Devolver». No cambia la disposición |

Sin violaciones; no hay tabla de complejidad.

## Pantallas y maquetas

Sin pantallas nuevas ni cambio de disposición: la sección «Entregados» de Pedidos ya existe tal cual
y solo cambia quién la ve. `/maquetas` no tiene nada que publicar.

## Project Structure

### Documentation (this feature)

```text
specs/068-roles-por-empresa/
├── plan.md
├── research.md
├── data-model.md
├── quickstart.md
├── contracts/orders-delivered.md
└── tasks.md
```

### Source Code

```text
server/internal/domain/permission.go         # PermOrdersDeliveredView, mapa y mensaje
server/internal/domain/permission_test.go
server/internal/httpapi/router.go            # 3 rutas: RequireRole → RequirePermission
server/internal/integration/entregadas_por_permiso_test.go   # nuevo, por ruta y rol
web/src/app/permissions.ts                   # 'orders.delivered_view' en el tipo
web/src/features/orders/OrdersBoardPage.tsx  # canRefund → can(...)
web/src/features/orders/OrdersBoardPage.test.tsx
```

**Structure Decision**: monorepo existente `server/` + `web/`; ningún paquete nuevo.

## Entrega 2 (fuera de esta implementación)

Se abre un issue nuevo enlazado al #61 con: roles por empresa en la base, pantalla para
administrarlos, migrar los `RequireRole` y `ROUTE_ROLES`/`role === …` que quedan, y qué pasa con
`users.role`. Lleva las preguntas de diseño para el dueño (puerta del principio VIII) y las reglas que
ya decidió el 2026-10-13. El #61 se comenta con lo entregado y se deja abierto hasta que la entrega 2
esté en producción.
