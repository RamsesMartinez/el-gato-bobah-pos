# Implementation Plan: Venta por plataformas digitales con listas de precios propias

**Branch**: `002-precios-plataformas` | **Date**: 2026-08-29 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `specs/002-precios-plataformas/spec.md`

## Summary

Cada plataforma de reparto gana un **margen porcentual** sobre el precio base y una tabla de
**excepciones capturadas a mano**. El POS arranca en mostrador como hoy; un selector cambia la lista
activa y re-precia lo que ya está en el ticket. El servidor sigue siendo autoritativo: recalcula
cada precio contra la lista de la plataforma que venga en el comando.

Buena parte del andamiaje ya existe (plataformas, métodos de pago, `orders.delivery_platform_id`, y
el corte que ya los separa), así que el trabajo real es la **resolución del precio efectivo** y su
recorrido completo desde el dominio hasta el ticket impreso.

## Technical Context

**Language/Version**: Go 1.27 (backend) · TypeScript / React 19 (web)
**Primary Dependencies**: chi, pgx + sqlc, goose (migraciones embebidas), Redis (caché de menú) · Vite, Chakra UI v3, TanStack Query, Zustand
**Storage**: PostgreSQL 16 multi-tenant con RLS por `company_id`
**Testing**: `go test` (unitario + `-tags=integration` contra Postgres real) · vitest
**Target Platform**: Tablets de 7" en el mostrador; navegador, sin instalar nada
**Project Type**: Web (backend `server/` + frontend `web/`)
**Performance Goals**: Resolver el precio de las líneas de un pedido no debe agregar un viaje por producto; el menú se sirve del caché de Redis como hoy
**Constraints**: Producción con datos reales en operación. Cobrar exige caja principal abierta (no cambia). El servidor nunca acepta un precio del cliente
**Scale/Scope**: 1 empresa activa, 502 productos, 546 opciones de modificador, 3 plataformas con margen + 1 sin margen

## Constitution Check

| Principio | Cómo lo cumple este plan |
|---|---|
| **I. Layering** | La regla de precio efectivo es **pura** y vive en `domain` (sin I/O). `app` carga base + margen + excepciones y arma el mapa que ya recibe `BuildOrder`. `httpapi` solo decodifica y mapea el error. Las tablas nuevas se leen por **sqlc**, nunca SQL a mano |
| **II. Errores** | Sentinel nuevo en `domain` para "la plataforma no es de esta empresa", envuelto con `%w` y mapeado **solo** en `httpapi.Error` |
| **III. Dinero** | `Round2` sobre el precio **unitario** efectivo antes de que toque `numeric(10,2)` (ver el caso de 434.98 en el data model), y `ValidMoney` en la frontera al capturar un precio |
| **IV. Test-first** | Cada tarea de implementación va después de su test. La regla de precio se prueba en `domain` sin base de datos; el aislamiento y los grants, con test de integración bajo el **rol de app** |
| **V. Seguridad** | Endpoint de escritura nuevo accesible a cajero → pasa por `security-auditor` antes de mergear. La plataforma del pedido se **resuelve bajo RLS**, no se confía del cliente |
| **VI. YAGNI** | Sin tabla genérica de "listas de precios": la lista ES la plataforma, uno a uno. Sin pantalla de configuración del margen (fuera de alcance por decisión del dueño). Sin materializar 1,506 precios |
| **VII. Comentarios** | Cada decisión no obvia del esquema y del cálculo lleva su porqué: por qué el default es 0 y no 35, por qué `updated_by` es `not null`, por qué el `Round2` va en el unitario |

**Gate**: pasa. Sin violaciones que justificar en Complexity Tracking.

## Project Structure

### Documentation (this feature)

```text
specs/002-precios-plataformas/
├── spec.md              # el requerimiento
├── plan.md              # este archivo
├── research.md          # qué ya existía y por qué se eligió cada cosa
├── data-model.md        # esquema, ya revisado por db-architect
├── quickstart.md        # cómo validar que funciona
├── contracts/
│   └── api.md           # endpoints nuevos y los que cambian
└── checklists/
    └── requirements.md
```

### Source Code (repository root)

```text
server/
├── migrations/
│   └── 0037_platform_prices.sql        # margen + 2 tablas + RLS + GRANTS + triggers
├── queries/
│   ├── platform_prices.sql             # nuevo: resolver, listar, upsert, borrar
│   └── orders.sql                      # + resolver la plataforma bajo RLS
└── internal/
    ├── domain/
    │   ├── platform_price.go           # regla pura de precio efectivo
    │   └── platform_price_test.go      # table-driven, con el caso 434.98 @ 35%
    ├── app/
    │   ├── orders.go                   # arma el mapa con precios de la lista activa
    │   └── platform_prices.go          # capturar / quitar un precio + invalidar caché
    └── httpapi/
        ├── handlers_platform_prices.go
        └── router.go                   # rutas nuevas

web/src/
├── api/pos.ts                          # llamadas nuevas
├── stores/ticket.ts                    # la plataforma activa vive en la cuenta
└── features/pos/
    ├── PlatformPicker.tsx              # selector + indicador de lista activa
    ├── POSPage.tsx                     # precios de la lista activa en el grid
    ├── Ticket.tsx                      # re-precia al cambiar de lista
    └── CheckoutSheet.tsx               # método de pago de la plataforma, sin envío
```

**Structure Decision**: Web (opción 2). El backend hace el trabajo de precio y el frontend solo
pinta y elige la lista, coherente con que el servidor es autoritativo en dinero.

## Orden de implementación

Por historia, para que cada corte deje algo usable (el orden de tareas lo detalla `/speckit-tasks`):

1. **Esquema** — migración 0037 tal como la dejó la revisión, **con los grants**, más el test de
   integración bajo el rol de app que los fija. Sin esto, todo lo demás se cae solo en producción.
2. **US1** (P1) — regla pura en `domain` + resolución en `app` + el POS con selector. Al terminar,
   ya se puede capturar un pedido de plataforma completo.
3. **US2** (P1) — capturar y quitar un precio desde la pantalla de venta, con invalidación del
   caché.
4. **US3** (P2) — **verificar**, no construir: un test que fije que el corte separa cada plataforma
   y que no cuentan como efectivo en el cajón.
5. **US4** (P3) — precio manual por opción de modificador.

## Complexity Tracking

Sin desviaciones de la constitución que justificar.

## Riesgos vivos

| Riesgo | Cómo se cierra |
|---|---|
| **La pantalla muestra un total y el servidor cobra otro** (FR-011 + FR-012) | Re-preciar el ticket al cambiar de lista, y un test que compare el total de pantalla contra el que devuelve el servidor para el mismo ticket |
| **Los grants faltantes solo fallan en producción** | Test de integración con `appRoleStore`, que corre en CI y bloquea el deploy |
| **El envío de $20 se cuela en un pedido de plataforma** | El POS no ofrece envío cuando hay plataforma, y el servidor lo fuerza a 0 |
| **Un cajero captura $14.90 donde iban $149.00** | FR-019 (quitar la excepción) + `updated_by not null` para saber quién |
| **Redondeo a tres decimales en 12 productos reales** | `Round2` en el unitario, con esos productos como caso de test |
