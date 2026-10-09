# Implementation Plan: Ventas netas y devoluciones a la vista

**Branch**: `029-ventas-netas` | **Date**: 2026-10-08 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `/specs/029-ventas-netas/spec.md`

## Summary

El Total de Ventas pasa a ser la suma de los medios de pago que la spec 031 ya calcula por el día de
cada pago y de cada devolución (FR-001); lo que falta por cobrar sale en una consulta propia
(FR-002). La lista de Ventas trae lo cobrado y el momento de la última devolución (FR-004). El corte
deja de mandar las devoluciones en efectivo a «Salidas» y separa la propina devuelta de la venta
devuelta igual que Ventas (FR-007, FR-008). Lo demás son arreglos puntuales: el detalle del pedido
sin `refund`, el detalle de un turno abierto sin totales, tres validaciones de frontera y la
presentación en la tableta. Sin migraciones. Detalle de cada decisión: [research.md](research.md).

## Technical Context

**Language/Version**: Go 1.27 (server), TypeScript + React 19 (web)

**Primary Dependencies**: chi, pgx, sqlc, goose; Chakra UI v3, TanStack Query

**Storage**: PostgreSQL con RLS. **Sin migraciones**: todo sale de columnas existentes
(`order_payments.business_date`, `order_refunds.business_date/register_session_id/tip_amount/cash_movement_id`).

**Testing**: `go test` (dominio), integración contra Postgres real (`-tags=integration`) bajo
`appRoleStore` donde se lee una tabla de empresa, vitest

**Target Platform**: API Linux + PWA en tabletas de 7–10" (1024×600)

**Project Type**: web-service + web app

**Performance Goals**: Ventas y el corte responden como hoy; la consulta nueva de «por cobrar» usa el
mismo predicado de fecha que la lista (índice de día por empresa)

**Constraints**: no tocar Vender, OrdersBoardPage, PedidosEnCurso ni CobrarSheet; conteos de
productos sin cambio

**Scale/Scope**: una pantalla de Ventas, el corte (abierto, histórico), Reportes; 5 arreglos de API

## Constitution Check

| Principio | Cómo se cumple |
|---|---|
| I Layering | Clasificación del resumen (Total = Σ medios) y del corte (devolución de venta vs propina devuelta, nota de negativo) en `domain`; `app` arma; SQL vía sqlc; validaciones nuevas (servicio de plataforma, centavo, nada por devolver) en `domain` |
| II Errores | Sentinels de `domain` envueltos con `%w`; nada de errores crudos de Postgres hacia el handler |
| III Dinero | Cada peso una vez: el Total es venta neta sin propina; la propina devuelta se nombra aparte en Ventas y en el corte, con la misma regla; «Por cobrar» no se suma. Lista y resumen salen del mismo predicado de fecha. El test falla nombrando el concepto duplicado |
| IV TDD | Cada hallazgo con su test en rojo primero: unitarios de `domain`; integración para el Total neto (dos meses), por cobrar (`inTheThreeCases`), detalle de pedido, detalle de turno abierto, corte con devoluciones en efectivo y tarjeta; vitest para la presentación |
| V Seguridad | Sin endpoints nuevos ni cambios de rol. Los rechazos nuevos son 4xx. El detalle de turno abierto conserva el ocultamiento del arqueo ciego |
| VI YAGNI | Sin tablas ni columnas; una consulta nueva (y su gemela de pendientes, obligada por el índice parcial) |
| VII Comentarios | Porqué; identificadores nuevos en inglés; textos en español |
| VIII Puertas | ¿Se puede agregar después al mismo costo? Sí: no se registra ni se deja de registrar ningún hecho; todo es lectura de lo que la 031 ya guarda. Ninguna puerta se cierra |

Gate: pasa. Sin violaciones.

### Dónde aterriza cada pieza

| Pieza | Capa |
|---|---|
| `NetCollected` (Total = Σ medios), `ValidChargeAmount` (centavo), `ValidPlatformServiceType`, «nada por devolver», clasificación del desglose del corte y su nota | `domain` |
| Resumen de Ventas (Total neto, por cobrar), detalle de pedido con `refund`, detalle de turno abierto en vivo, propina neta del corte | `app` |
| `SalesPending` (+ `…SinFolio`), columnas `paid`/`last_refund_at` en las listas de Ventas, `tip_refunds` en Ventas por método, `refunded_tips`/`drawer_*` en el esperado del turno, `is_refund` en movimientos, sin costo en Utilidad | `queries/*.sql` + `make sqlc` |
| Tiles, renglón de la lista, detalle de venta | `web/src/features/sales/` |
| Corte (Cajas, Histórico), Reportes | `web/src/features/backoffice/` |
| Dos decimales | `web/src/utils/format.ts` |

## Project Structure

### Documentation (this feature)

```text
specs/029-ventas-netas/
├── plan.md
├── research.md
├── data-model.md
├── quickstart.md
├── contracts/api.md
└── tasks.md
```

### Source Code (repository root)

```text
server/
├── queries/{sales,cash,reports}.sql
├── internal/domain/{sales,cash,devolucion,cobro,order}.go (+ _test)
├── internal/app/{sales,backoffice,orders,devolucion}.go
└── internal/integration/ventas_netas_test.go
web/src/
├── utils/format.ts
├── api/sales.ts, api de caja y reportes
├── features/sales/{SalesPage,SalesSummaryTiles,SaleDetailDialog}.tsx (+ test)
└── features/backoffice/{CashPage,ReportsPage}.tsx (+ test)
```

**Structure Decision**: monorepo existente; sin paquetes nuevos.

## Complexity Tracking

Sin violaciones que justificar.
