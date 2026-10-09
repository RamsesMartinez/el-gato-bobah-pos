# Implementation Plan: Defectos de dinero de la auditoría

**Branch**: `031-defectos-de-dinero` | **Date**: 2026-10-08 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `/specs/031-defectos-de-dinero/spec.md`

## Summary

Dieciocho defectos de dinero (D1–D18). El núcleo: cada devolución sabe por qué medio, en qué turno y
qué día salió, y cada pago en qué día entró; con eso el reparto por medio (D1), el corte (D6, D7, D12)
y Ventas por método (D12) se calculan de los hechos y no del turno o el día del pedido. Lo demás son
candados (D3, D8), topes (D2, D4, D18) y consultas que excluían o contaban de más (D10, D11, D14, D16,
D17, D13). Detalle de cada decisión: [research.md](research.md).

## Technical Context

**Language/Version**: Go 1.27 (server), TypeScript + React 19 (web)

**Primary Dependencies**: chi, pgx, sqlc, goose; Chakra UI v3, TanStack Query

**Storage**: PostgreSQL con RLS; migraciones 0080 (esquema) y 0081 (backfill del día de cada pago)

**Testing**: `go test` (dominio), integración contra Postgres real (`-tags=integration`), vitest

**Target Platform**: API Linux + PWA en tabletas de 7–10" (1024×600)

**Project Type**: web-service + web app

**Performance Goals**: el corte y Ventas responden como hoy; las consultas nuevas usan índices que
empiezan por `company_id`

**Constraints**: datos de producción vivos; `lock_timeout` en las migraciones; columnas nuevas nullable para que un rollback del binario siga cobrando

**Scale/Scope**: ~18 consultas y 8 servicios tocados; 2 componentes de pantalla

## Constitution Check

| Principio | Cómo se cumple |
|---|---|
| I Layering | Reglas nuevas en `domain` (reparto, topes, saldado, partir); `app` orquesta y bloquea; SQL solo vía sqlc |
| II Errores | Sentinels nuevos en `domain` envueltos con `%w`; mapeo solo en `httpapi.Error` |
| III Dinero | `decimal` + `Round2` en cada frontera; cada peso se clasifica una vez: la propina devuelta va en su columna, `refund_amount` sigue siendo solo venta; lo que salió del cajón no se resta dos veces del esperado (R6) |
| IV TDD | Cada defecto: test que se ve en rojo antes del arreglo. Unitarios en `domain`; integración para candados, corte, RLS, migración (respaldo real, dos empresas); consultas nuevas con `inTheThreeCases` |
| V Seguridad | Sin endpoints nuevos ni cambios de rol; los rechazos nuevos son 4xx limpios |
| VI YAGNI | Sin tablas nuevas; columnas que se llenan desde hoy |
| VII Comentarios | Porqué; identificadores nuevos en inglés, textos en español |
| VIII Puertas | ¿Se puede agregar después al mismo costo? **No** para el turno y el día de una devolución o un pago: es un hecho que, si no se registra hoy, no se recupera (la ventana de tiempo ya se rechazó). Por eso se registra ahora. «Más de una caja» y «más de una sucursal» siguen abiertas: el turno se guarda por fila, no se deduce de "la" caja; reclamar huérfanas es por sucursal |

Gate: pasa. Sin violaciones.

### Dónde aterriza cada pieza

| Pieza | Capa |
|---|---|
| Reparto neto, tope de renglón, `VoidKeepsRefunds`, `PagosCubren`, `SplitLineTotal`, propina por medio | `domain` |
| Candados, turno/día de la devolución, reclamar huérfanas, cierre en transacción | `app` |
| Consultas de esperado, sin cobrar, cobros de otros turnos, Ventas por método, utilidad, renglones cancelados | `queries/*.sql` + `make sqlc` |
| Columnas, FK compuesta, backfill, índices | `migrations/0080`, `0081` |
| `refunds` del corte, conceptos del desglose | `app` (vista) |
| «Total Dif.», lista de devoluciones | `web/src/features/backoffice/CashPage.tsx` |

## Project Structure

### Documentation (this feature)

```text
specs/031-defectos-de-dinero/
├── plan.md
├── research.md
├── data-model.md
├── quickstart.md
├── contracts/api.md
└── tasks.md
```

### Source Code

```text
server/
├── migrations/0080_money_by_shift_and_day.sql, 0081_day_of_each_payment.sql
├── queries/{orders,cash,sales,reports,pedidos_de_plataforma}.sql
├── internal/domain/{devolucion,order,split_bill}.go (+ _test)
├── internal/app/{devolucion,orders,backoffice,pedidos_de_plataforma,sales}.go
└── internal/integration/money_*_test.go, migration_money_by_shift_test.go
web/src/
├── api/backoffice.ts
└── features/backoffice/CashPage.tsx (+ test), features/sales/SalesPage.test.tsx
docs/matriz-de-cobro.md
```

**Structure Decision**: monorepo existente; no se agregan paquetes.

## Pantalla (tableta 1024×600)

- «Total Dif.»: misma tabla, otra cifra. Sin alto nuevo.
- «Devoluciones (N)»: **plegada** con su contador (un toque para abrirla, 0 px cuando está cerrada),
  tope `35dvh` al abrir, no se pinta vacía. Va en el resumen del corte (histórico y vivo). Es dinero
  devuelto al cliente; «Pagos devueltos» (027) es un cobro anulado: se quedan como dos listas con
  nombres distintos porque son dos hechos distintos y el esperado los trata distinto.
- El desglose suma dos conceptos («Cobros de otros turnos», «Devoluciones») solo cuando hay; un
  importe negativo se pinta en rojo, como los egresos.
- Un esperado negativo (devolución en este turno de un cobro de otro) se muestra con signo y no
  rompe el arqueo.
- Una devolución huérfana la reclama la primera caja principal que abra en la sucursal; con una sola
  caja que vende es exacto. Anotado para cuando haya dos.

## Complexity Tracking

Sin violaciones.
