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
| Tableta | Controles ≥ 48 px en Caja; conteo con −/+ de 52 px y teclado numérico; sin `<select>`; maquetas elegidas (sección Maquetas) | OK |

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

Elegidas por el dueño el 2026-11-13 sobre el lienzo [https://claude.ai/artifact/1PJ62bqNXJJ4D9ume3e7nn](https://claude.ai/artifact/1PJ62bqNXJJ4D9ume3e7nn) (arnés con los componentes reales,
medido a 1024×600, 1366, 1920 y 2560). Lo que se construye:

- **M1 Histórico y detalle del corte — A «Resumen primero»** (fases 1, 3 y 4). Lista de cortes a la
  izquierda (320 px en 1024; 400 px desde 1440) con caja, día, abrió/cerró, `<Diferencia>` y marca
  («Corregido», «Por explicar», «Explicado al abrir»), y al pie la suma del periodo (P4). Detalle: tres
  cifras arriba (Cajón (arqueo) · Otros métodos (conciliación) · Total), el aviso «Diferencia
  explicada al abrir el turno #N» con motivo y nota, la corrección vigente con Revertir (fase 3), «El
  turno» (abrió, cerró, fondo que se dejó, retiro y quién, motivo al abrir) en lista en 1024 y en
  rejilla desde 1366, y debajo Arqueo del cajón (con «Corregir») y Conciliación, ambas en Esperado ·
  Contado · Diferencia con «ver arqueo» en el efectivo. Desde 1440 una tercera columna con los
  movimientos del corte; en 2560 una cuarta con las ventas. Corregir es una hoja inferior (M4).
- **M2 Turno abierto — B «Resumen fijo al lado»** (fase 4). Desde 1024: Operar a la izquierda
  (acciones de 52 px y movimientos de hoy) y el Corte X fijo a la derecha (320 px; `minmax(320px,1fr)`
  desde 1366) con «Cerrar caja» de 56 px al pie. En 2560 las cajas abiertas lado a lado, cada una con
  sus dos columnas. Debajo de 1024, una columna con el Corte X después de Operar.
- **M3 Asistente de cierre — B «Pasos al costado»** (fase 4). Columna de pasos (220 px en 1024, 300 px
  desde 1366) con el estado de cada paso en texto; el paso a la derecha con Atrás/Siguiente fijos al
  pie y, en el último, «Cerrar caja (Corte Z)». Conteo por denominación con −/+ de 52 px. Con arqueo
  ciego el último paso no muestra diferencia ni pide explicación.
- **M4 Corregir un corte** — hoja inferior única (fase 3): qué corriges (conteo, fondo, nota), lo
  firmado contra lo que debió decir con teclado numérico, motivo en botones y nota obligatoria.
- **M5 Caja → Administrar y botón de tema** — disposición única (fases 1 y 2): índice de secciones
  desde 1366, dos columnas desde 1920; sol/luna en la franja fija del riel, arriba del usuario.

### Precisiones de `/revision-de-arquitectura` (2026-11-13)

Afinan las opciones elegidas sin cambiar su disposición:

- **«Cómo va el turno» (Corte X) respeta el arqueo ciego en el servidor.** Lee la misma vista del
  turno abierto, que ya pasa por `ocultarLoEsperado`: sin `cash.configure` y con arqueo ciego no viajan
  el efectivo esperado, los ingresos por método ni lo cobrado por cajero; se ven pedidos por cobrar,
  entradas y salidas registradas y ventas totales solo si hoy ya se muestran. Test de integración que
  lo fija (`TestHowTheShiftIsGoingHidesTheDrawerWhenBlind`).
- **Rótulos para quien opera**: «Cómo va el turno» y «Cerrar caja»; «Corte X»/«Corte Z» solo en la
  ayuda (icono). Se quita el título «Caja» de la página: la pestaña ya lo dice (gana ~60 px a 1024).
  Las cinco acciones de Operar en una fila de 5 columnas desde 1024.
- **Estado de cada paso del asistente = la misma regla del botón** (`pendientesDelCierre`): gris sin
  visitar, naranja con bloqueos (con su cuenta), verde solo con cero bloqueos. Se puede pasar al
  conteo con bloqueos pendientes (se cuenta mientras se resuelven); el último paso vuelve a listar los
  bloqueos con su acción y el botón dice cuál falta.
- **Conteo a 1024×600 sin desplazar**: filas de 60 px y tres columnas desde 840 px; el total contado
  queda en la barra fija del pie.
- **Lista del Histórico a 1024**: 340 px; nombre de la caja en su línea, diferencia y marca en la
  segunda.
- **Hoja Corregir (M4)**: inferior, `maxH="90dvh"` con cuerpo desplazable; la aclaración va arriba
  («Se guarda con tu nombre y no borra lo firmado»); la línea «la diferencia queda en…» solo con
  `cash.configure`.
- **2560**: movimientos sin tope de renglones (crece con el alto); la tipografía base no se escala
  (decisión: mismo tamaño en todos los anchos, más contenido, no letras más grandes).
- **Color**: `fg.shortage`/`fg.overage` solo marcan diferencias; lo seleccionado y la acción
  principal siguen con `brand`, como el resto del POS.
- **Toques en tablas**: los renglones que se tocan (corte en la lista, movimiento corregible) miden
  ≥ 48 px; las celdas de lectura pueden ir en `size="sm"`.

## Maquetas

| Pantalla | Maquetas | Elegida | Por qué |
|----------|----------|---------|---------|
| M1 Histórico y detalle del corte | [lienzo](https://claude.ai/artifact/1PJ62bqNXJJ4D9ume3e7nn#m1) | A | La diferencia completa y su explicación se ven sin desplazar a 1024×600 |
| M2 Turno abierto: Operar y Cerrar | [lienzo](https://claude.ai/artifact/1PJ62bqNXJJ4D9ume3e7nn#m2) | B | Cómo va el turno a la vista sin un toque; Cerrar caja en un solo lugar |
| M3 Asistente de cierre | [lienzo](https://claude.ai/artifact/1PJ62bqNXJJ4D9ume3e7nn#m3) | B | Cada paso dice qué le falta sin entrar (evita el botón gris de #251) |
| M4 Corregir un corte | [lienzo](https://claude.ai/artifact/1PJ62bqNXJJ4D9ume3e7nn#m1) | Única | Aprobada |
| M5 Botón de tema y Administrar | [lienzo](https://claude.ai/artifact/1PJ62bqNXJJ4D9ume3e7nn#m5) | Única | Aprobada |

## Decisiones del dueño (P1–P4)

Aprobadas el 2026-11-13 tal como las recomienda el [spec](spec.md#decisiones-del-dueño-2026-11-13).

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
