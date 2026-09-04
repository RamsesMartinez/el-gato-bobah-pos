# Implementation Plan: La fecha la da el reloj, el folio lo da el turno

**Branch**: `008-fecha-y-folio-separados` | **Date**: 2026-09-04 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `specs/008-fecha-y-folio-separados/spec.md`

## Summary

Una venta se archiva hoy con la fecha del turno de caja abierto, sin techo: un turno que nadie
cierra sigue estampando su fecha días después, y la pantalla de Ventas del día sale vacía cuando sí
hubo ventas.

Se separan dos cosas que hoy cuelgan de la misma columna:

- **La fecha** de una venta pasa a ser el día de calendario en que ocurrió, en la zona del negocio,
  con las piezas que ya existen (`domain.BusinessDate`, `LoadBusinessLocation`, `GetBusinessTimezone`).
- **El folio** —número y nombre— pasa a contarse dentro del turno, en una tabla nueva
  `folio_counters`, para que un turno nocturno numere corrido sin depender de la fecha.

Ninguno de los dos lee al otro. Además: el histórico se corrige con una migración reversible (0 de
31 filas del negocio en operación), el detalle de un corte gana la lista de sus ventas, y el POS
avisa cuando el turno abierto ya no es de hoy.

## Technical Context

**Language/Version**: Go 1.27 (backend) · TypeScript 5 / React 19 (front)

**Primary Dependencies**: chi · pgx v5 + sqlc · goose (migraciones embebidas) · Chakra UI v3 ·
TanStack Query. Ninguna dependencia nueva.

**Storage**: PostgreSQL con RLS por empresa. Dos tablas nuevas, ninguna alterada.

**Testing**: `go test ./...` (unitarios en `domain`, integración contra Postgres real incluyendo el
rol `gatobobah_app`) · vitest (front) · Playwright a 1024×600 contra el ambiente desplegado.

**Target Platform**: API en contenedor Linux; front en tabletas de 7 a 10 pulgadas, ~1024×600.

**Project Type**: Monorepo web — `server/` (Go) + `web/` (React).

**Performance Goals**: Sin regresión en el camino de crear una venta. El costo agregado es una
lectura de una fila (la zona del negocio) por venta.

**Constraints**: Producción es un negocio en operación con arqueos ya cerrados. Ninguna cifra
histórica de un arqueo puede cambiar. La numeración concurrente no puede perder su garantía.

**Scale/Scope**: 2 empresas, decenas de ventas por día. 2 migraciones, 2 tablas nuevas, 2
endpoints que ganan campos, 1 sección de pantalla nueva, 1 aviso.

## Constitution Check

*GATE: revisado antes de Phase 0 y de nuevo tras el diseño.*

| Principio | Cómo lo cumple este plan |
|---|---|
| **I · Layering estricto** | La decisión de qué día es una venta ya vive en `domain.BusinessDate` (puro, sin I/O). El servicio `app/orders` orquesta: lee la zona del store, llama al dominio, y numera dentro de su transacción. Los handlers siguen finos. El SQL nuevo va por sqlc, nunca concatenado. |
| **II · Errores envueltos** | Sin errores nuevos. Los caminos existentes (`ErrNoOpenRegister`, `ErrNotFound`) se conservan y se siguen mapeando en `httpapi.Error`. |
| **III · Dinero clasificado una vez** | `salesTotal` del corte declara qué excluye —canceladas, reembolsadas y propinas— y la pantalla lo repite. La lista y el total de esa sección salen del **mismo** `where`, con su `Count` gemela. |
| **IV · Test-first, bordes primero** | Los bordes están enumerados en el spec antes que el código, y el quickstart dice qué defecto atrapa cada prueba. La migración lleva su test de integración (lo exige además el hook de pre-commit). Las pruebas de concurrencia, grants y RLS son de integración porque un unitario no puede verlas. |
| **V · Seguridad** | Sin superficie nueva de autenticación. Sí hay dos riesgos de la familia que este principio cubre: el **grant** de la tabla nueva (invisible en local, `42501` en producción) y su **política de RLS**, ambos con test bajo el rol de la aplicación. El id de corte que llega por URL ya se valida y se rechaza como 400. |
| **VI · YAGNI** | Se reutiliza todo lo que ya existe (cálculo del día, resolución de zona, pestaña de histórico, endpoint de estado de caja). No se agrega paginación con controles al detalle del corte, ni un endpoint nuevo, ni caché para la zona. La tabla `order_counters` no se refactoriza: se jubila después. |
| **VII · Comentarios del porqué** | El comentario que hoy explica la herencia de fecha en `orders.go` deja de ser cierto y **se reescribe**, no se borra: tiene que decir por qué ahora son dos caminos. Igual el de `businessdate.go`, que afirma "el día de una venta lo decide el turno". |

**Sin violaciones que justificar.** La sección *Complexity Tracking* queda vacía a propósito.

Dos notas que el gate deja anotadas para la implementación:

- **`db-architect` corre antes de aplicar las migraciones** — es regla del repo para cualquier
  cambio en `server/migrations/` o `server/queries/`.
- **`tablet-ui-reviewer` corre sobre la sección nueva del corte y sobre el aviso**, porque cambian
  la disposición de pantallas que viven en 1024×600.

## Project Structure

### Documentation (this feature)

```text
specs/008-fecha-y-folio-separados/
├── plan.md              # Este archivo
├── spec.md
├── research.md          # Las 5 decisiones técnicas, con lo descartado
├── data-model.md        # Tablas nuevas e invariantes
├── quickstart.md        # Escenarios de verificación
├── contracts/
│   └── api.md           # Los 2 endpoints que ganan campos
├── checklists/
│   └── requirements.md
└── tasks.md             # Lo genera /speckit-tasks
```

### Source Code (repository root)

```text
server/
├── migrations/
│   ├── 0061_folio_por_turno.sql          # tabla + RLS + grants + semilla
│   └── 0062_fecha_de_venta_del_reloj.sql # corrección histórica reversible
├── queries/
│   ├── orders.sql        # NextFolioNumber / FolioNamesUsedInSession (reemplazan a las de fecha)
│   └── cash.sql          # SessionSales + CountSessionSales
├── internal/
│   ├── domain/
│   │   └── businessdate.go   # comentario que hay que corregir; posible helper del turno viejo
│   ├── app/
│   │   ├── orders.go         # bizDate del reloj; folio por turno
│   │   └── backoffice.go     # SessionDetail gana las ventas; SellingRegisterOpen gana el día
│   ├── httpapi/
│   │   └── handlers_backoffice.go  # CashStatus devuelve los campos nuevos
│   └── integration/
│       └── (pruebas de migración, concurrencia, grants y RLS)
└── ...

web/src/
├── api/pos.ts                          # tipos de los 2 endpoints
└── features/
    ├── backoffice/CashPage.tsx         # sección "Ventas del corte" en CorteDetail
    └── pos/                            # aviso de turno de otro día
```

**Structure Decision**: Monorepo existente, sin directorios nuevos. Todo aterriza en archivos que
ya existen salvo las dos migraciones.

## Orden de implementación

El orden importa por una razón concreta: **la migración del folio tiene que ir antes que el cambio
de fecha**. Si la fecha se suelta primero, el contador —que aún cuelga de `business_date`— empieza
a reiniciarse a medianoche y reintroduce el defecto de los dos tickets #1.

1. **Folio por turno** (migración + queries + servicio + pruebas de concurrencia, grants y RLS).
2. **Fecha del reloj** (servicio + pruebas, incluida la del turno de cuatro días).
3. **Corrección histórica** (migración reversible + prueba contra respaldo real con dos empresas).
4. **Ventas del corte** (query + servicio + contrato + pantalla).
5. **Aviso de turno viejo** (servicio + contrato + pantalla).

Los pasos 4 y 5 son independientes entre sí y del 1–3; los tres primeros son una cadena.

## Riesgos

| Riesgo | Mitigación |
|---|---|
| Perder la garantía de numeración concurrente al cambiar de tabla | Prueba de integración con dos transacciones simultáneas, vista en rojo quitando el candado. |
| El turno abierto de 158 pedidos vuelve a repartir el folio 1 | La semilla es parte de la migración, con su prueba (quickstart 4). |
| La tabla nueva sin grant → `42501` solo en producción | Prueba bajo `gatobobah_app`, no como owner (quickstart 8). |
| La corrección histórica toca algo que no debía | Respaldo en tabla + `Down` que restaura + comparación fila por fila de arqueos sobre datos reales (quickstart 6 y 7). |
| El front deploya ~7 min antes que el backend | Los campos nuevos son aditivos y el front los trata como ausentes: una pantalla sin aviso durante unos minutos, nunca una rota. |
| La sección nueva empuja el resto del detalle del corte fuera de pantalla | `tablet-ui-reviewer` sobre el cambio, a 1024×600. |
