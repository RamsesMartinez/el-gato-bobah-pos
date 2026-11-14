# Implementation Plan: Rediseño del módulo Caja

**Branch**: `082-rediseno-de-caja` | **Date**: 2026-11-13 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `specs/082-rediseno-de-caja/spec.md` (issue #290)

## Summary

Cuatro entregas, cada una con su PR a `develop`, en el orden del issue:

1. **Lectura y claridad**: `domain.CutDifference` como única regla de diferencia (lista, detalle y
   cierre), vocabulario Esperado · Contado · Diferencia, `<Diferencia>` con palabra, signo, ícono y
   color, detalle del corte con quién, fondo, retiro y motivo de apertura enlazado al corte anterior,
   y el botón de tema. Sin esquema.
2. **Partir sin cambiar comportamiento**: `CashShiftService`, `CashMovementService` y
   `CashReportService` fuera de `BackofficeService`; `cash.sql` en cuatro archivos; `features/caja/`
   con rutas propias y Administrar unido. Sin esquema, sin cambios de contrato.
3. **Correcciones con rastro**: tabla `cash_session_adjustments` (0113), `CashAdjustmentService`, dos
   endpoints y la hoja «Corregir corte».
4. **Asistente de cierre, X/Z y pantallas grandes**: `pendientesDelCierre` como única regla del
   botón, asistente de 4 pasos con borrador, Operar/Cerrar y Corte X, tolerancia (0114) con «por
   explicar», y disposición por ancho hasta 2560 px.

Detalle técnico en [research.md](research.md), [data-model.md](data-model.md) y
[contracts/](contracts/).

## Technical Context

**Language/Version**: Go 1.27.4 (server), TypeScript + React 19 (web)

**Primary Dependencies**: chi, pgx + sqlc, goose; Chakra UI v3, TanStack Query, Zustand,
react-router 8, next-themes. Ninguna dependencia nueva.

**Storage**: PostgreSQL con RLS. Una tabla nueva (0113) y una columna nueva (0114).

**Testing**: `go test` (domain table-driven), integración contra Postgres real bajo
`appRoleStore` con `inTheThreeCases`, vitest, Playwright a 1024×600 y anchos grandes.

**Target Platform**: tabletas de 7–10" (1024×600) y PC/laptop/monitor 2K (1366, 1920, 2560).

**Project Type**: web (monorepo `server/` + `web/`).

**Performance Goals**: el Histórico calcula la diferencia en Go sobre las piezas que ya trae la
consulta (sin consultas por fila). El enlace «explicada al abrir» sale de una ventana `lead/lag` por
caja en la misma consulta.

**Constraints**: la API anterior convive con la nueva durante cada subida; el arqueo ciego no se
afloja; nada se vuelve dependiente de la red que hoy no lo sea (el borrador del cierre vive en el
dispositivo).

**Scale/Scope**: un archivo de 2,038 líneas y uno de 3,137 a partir; ~20 secciones de pantalla; 2
migraciones.

## Constitution Check

| Principio | Cómo se cumple | Estado |
| --- | --- | --- |
| I. Layering | `CutDifference`, `CutNeedsExplanation`, `CanAdjustSession`, `CanReverseAdjustment` en `domain`; servicios en `app` con `WithTx`; handlers finos; SQL solo por sqlc | OK |
| II. Errores | Sentinels nuevos `ErrSessionNotClosed`, `ErrAdjustmentReversed` envueltos con `%w`, mapeados solo en `httpapi.Error` | OK |
| III. Dinero | Una sola regla de diferencia para lista, detalle, cierre y corrección (corrige el defecto del issue). `Round2` y `ValidMoney` en `after` y tolerancia. Test que nombra el concepto duplicado: `TestElEfectivoNoSeCuentaDosVecesEnLaDiferencia`. La suma de diferencias del periodo (P4) no se mezcla con ventas | OK |
| IV. Test-first | Bordes antes del código (ver «Bordes» abajo). Migración 0113 con su test sobre respaldo restaurado y dos empresas; `inTheThreeCases`; Down con renglón en `guardedDownCleanup` | OK |
| IV. Tolera la versión anterior | 0113 es tabla nueva que la API vieja no conoce; 0114 columna con default. `totalDifference` se conserva | OK |
| V. Seguridad | Corregir y revertir con `RequirePermission(cash.adjust_count)`; explicación al cerrar con `cash.configure`, 403 en cierre ciego para que no sirva de sonda; tolerancia con `cash.configure` y 400 al vacío | OK |
| VI. YAGNI | Tres servicios, no cinco (R5); sin reabrir (P3); sin cuenta contable (P4); la tolerancia es por empresa | OK |
| VII. Idioma | Tabla, columnas, valores de `kind`/`reason` y acciones de bitácora en inglés; texto de UI en español | OK |
| VIII. Puertas | Ver abajo | OK |
| Tableta | Controles ≥ 48 px en Caja; conteo con teclado numérico; sin `<select>`; maqueta elegida por pantalla antes de implementar | Pendiente de la elección del dueño |

### Principio VIII: ¿se puede agregar después al mismo costo?

| Decisión | ¿Mismo costo después? | Qué se hace hoy |
| --- | --- | --- |
| Reabrir un corte (P3) | Sí: no se pierde ningún hecho; las correcciones ya dejan el rastro | No se construye |
| Tolerancia por caja o por sucursal | Sí: una columna que se llena desde la de empresa | Por empresa |
| Corrección de un pago o movimiento de un corte cerrado | Sí: `kind` es una lista cerrada que se amplía con una migración aditiva | Solo conteo, fondo y nota |
| **Quién corrigió y cuándo** | **No**: un ajuste hecho sin rastro no se reconstruye | Se guarda `created_by`, `created_at`, antes/después y nada se edita |
| **Más de una caja vendiendo a la vez** (puerta abierta) | La regla de diferencia y las correcciones son por corte, nunca por «la caja abierta»; la vista de 2K pone las cajas abiertas lado a lado | No se cierra |

### Bordes enumerados antes del código

- Vacío que significa algo: tolerancia vacía ≠ $0 (400); `after` vacío ≠ $0 (400).
- Estado que no sobrevive: borrador del cierre en el dispositivo; recarga, suspensión y otra tableta
  que cerró la caja.
- Camino nuevo que se salta el control viejo: la explicación al cerrar respeta el arqueo ciego; los
  endpoints de corrección piden permiso en el router y RLS en la base.
- Hermano que no se movió: `totalDifference` del histórico, el correo diario (`DaySummaryMoney`) y el
  MCP leen diferencias; el correo diario se deja igual (reporta cajón y terminal por separado y no
  contradice la regla) y se documenta.
- Cortes de tres épocas (antes de 0066, entre 0066 y 0067, después de 0067) en la regla única.
- Dos reversos simultáneos: índice único + `FOR UPDATE`.

## Pantallas y maquetas

Cinco pantallas cambian; las tres primeras tienen opciones de verdad distintas y van a maqueta con
opciones; las dos últimas tienen una sola disposición razonable y se muestran en la misma página
para que el dueño las vea.

| # | Pantalla | Fase | Qué decide el dueño |
| --- | --- | --- | --- |
| M1 | **Detalle del corte e Histórico** (lista + detalle, diferencia rotulada, motivo de apertura, correcciones) | 1, 3, 4 | Dónde va la diferencia (arriba como resumen o junto a cada tabla), cómo se ve la corrección, cuántas columnas por ancho |
| M2 | **Turno abierto: Operar y Cerrar** (incluye Corte X) | 4 | Operar y Cerrar como pestañas, como dos columnas o como dos pantallas |
| M3 | **Asistente de cierre** (4 pasos) | 4 | Barra de pasos arriba o lateral; pendientes como lista bloqueante o como tarjetas con su acción |
| M4 | Corregir un corte (hoja) | 3 | Una opción |
| M5 | Botón de tema y Caja → Administrar | 1, 2 | Una opción cada uno |

Cada opción se arma con los componentes reales del POS (arnés Vite fuera del repo) y se mide a
1024×600, 1366×768, 1920×1080 y 2560×1440, con su cifra contra la pantalla actual (ancho ocupado,
renglones visibles sin desplazar, toques para cerrar).

## Maquetas

| Pantalla | Maquetas | Elegida | Por qué |
|----------|----------|---------|---------|
| M1 Detalle del corte e Histórico | pendiente | pendiente | |
| M2 Turno abierto: Operar y Cerrar | pendiente | pendiente | |
| M3 Asistente de cierre | pendiente | pendiente | |
| M4 Corregir un corte | pendiente | pendiente | |
| M5 Botón de tema y Administrar | pendiente | pendiente | |

## Decisiones del dueño (P1–P4)

Pendientes de la vuelta de maquetas; recomendaciones en el [spec](spec.md#decisiones-pendientes-del-dueño-se-deciden-en-la-vuelta-de-maquetas).

## Project Structure

### Documentation (this feature)

```text
specs/082-rediseno-de-caja/
├── plan.md
├── research.md
├── data-model.md
├── quickstart.md
├── contracts/api.md
├── contracts/rutas-web.md
└── tasks.md
```

### Source Code (repository root)

```text
server/
├── internal/domain/
│   ├── cash_difference.go        # F1: CutDifference, MethodDiff; F4: CutNeedsExplanation
│   └── cash_adjustment.go        # F3: kinds, reasons, CanAdjustSession, CanReverseAdjustment, vigente
├── internal/app/
│   ├── backoffice.go             # F2: pierde lo de caja
│   ├── cash_shift.go             # F2: CashShiftService (abrir, cerrar, actual, conteo)
│   ├── cash_movements.go         # F2: CashMovementService (movimientos, corrección de salida, traspaso)
│   ├── cash_report.go            # F2: CashReportService (histórico, detalle, ventas del corte)
│   ├── cash_adjustments.go       # F3: CashAdjustmentService
│   └── cash_close_settings.go    # F4: tolerancia
├── internal/httpapi/
│   ├── handlers_cash.go          # F2: handlers de caja salen de handlers_backoffice.go
│   └── router.go                 # F3/F4: rutas nuevas
├── queries/
│   ├── cash_sessions.sql cash_counts.sql cash_movements.sql cash_history.sql   # F2
│   └── cash_adjustments.sql      # F3
├── migrations/
│   ├── 0113_cash_session_adjustments.sql   # F3
│   └── 0114_cash_difference_tolerance.sql  # F4
└── internal/integration/
    ├── cut_difference_test.go          # F1
    ├── cash_adjustments_test.go        # F3 (inTheThreeCases, permisos, reverso doble)
    └── migracion_0113_test.go          # F3

web/src/
├── features/caja/
│   ├── CajaPage.tsx              # F2: contenedor + rutas hijas
│   ├── diferencia.tsx            # F1: <Diferencia>, moneyFixed, montoOSinDato
│   ├── turno/  (Operar, Cierre, CorteX, asistente/)   # F2 partir, F4 asistente
│   ├── arqueo/  conciliacion/  movimientos/
│   ├── reporte/ (Historico, DetalleDelCorte, CorregirCorte)   # F1, F3
│   └── administrar/ (Cajas + AjustesDeCaja movido)            # F2
├── app/AppShell.tsx              # F1: TemaRapido
├── app/routes.ts  App.tsx        # F2: rutas de Caja
└── components/ui/provider.tsx    # F1: tokens fg.shortage/fg.overage; F4: breakpoints 3xl/4xl
web/e2e/
├── caja-diferencia.spec.ts       # F1
├── caja-correcciones.spec.ts     # F3
└── caja-anchos.spec.ts           # F4 (768, 1024×600, 1366, 1920, 2560)
docs/mecanica/caja-y-cobro.md     # cada fase anota lo que no se ve
```

**Structure Decision**: monorepo existente. Capas: `domain` (reglas puras), `app` (servicios y
transacciones), `httpapi` (handlers y rutas), `store/db` (sqlc), `web/src/features/caja`.

## Entregas y su orden

| Fase | PR | Migración | Red antes/después |
| --- | --- | --- | --- |
| 1 | `082 fase 1: una sola diferencia` | — | Integración de caja + vitest + e2e de caja |
| 2 | `082 fase 2: partir caja` | — | La red completa de R5, con el mismo número de casos |
| 3 | `082 fase 3: correcciones con rastro` | 0113 (reconfirmar) | + `cash_adjustments_test`, migración sobre respaldo |
| 4 | `082 fase 4: asistente de cierre y anchos` | 0114 (reconfirmar) | + `caja-anchos.spec.ts`, prueba con usuario nuevo |

Cada PR se mergea al día con `develop` y con CI verde antes de empezar el siguiente. `/revision-de-codigo` por fase. `db-architect` antes de aplicar 0113 y 0114.

## Complexity Tracking

| Desviación | Por qué | Alternativa descartada |
| --- | --- | --- |
| Tres servicios de caja en vez de los cinco del issue (sin `CashCount`; `CashAdjustment` llega en la fase 3) | El conteo solo existe dentro de la transacción de abrir y cerrar; separarlo exigiría pasar la `q` entre dos servicios o una interfaz de un consumidor (VI) | Cinco servicios como dice el issue |
| `money()` global no cambia; Caja usa `moneyFixed()` | Cambiar `money()` mueve tickets y precios de todo el POS, fuera del alcance | Cambiar `money()` |
