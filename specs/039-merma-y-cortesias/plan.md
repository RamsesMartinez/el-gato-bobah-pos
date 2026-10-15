# Implementation Plan: Merma y faltante sin explicar

**Branch**: `039-merma-y-cortesias` | **Date**: 2026-10-15 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `specs/039-merma-y-cortesias/spec.md`

## Summary

Registrar la merma cuando pasa (insumo o producto, cantidad en empaques, motivo de la empresa),
restarla del almacén con un movimiento `merma` ligado a su registro, y pedir aprobación cuando su
costo pasa el tope que configura la empresa. Después de cada conteo, mostrar por insumo lo perdido
en el periodo partido en **registrado como merma** y **sin explicar** (lo que el conteo encontró de
menos). Un reporte de merma por motivo y la administración de motivos cierran la feature.

El enfoque reutiliza lo que ya existe: el tipo de movimiento `merma` (0008), `domain.ExpandSale`
para lo que resta un producto, los empaques y el teclado numérico del 038, el renglón del conteo con
`system_qty` y `captured_at` (0092), `recordAudit` y los permisos de `domain`.

## Technical Context

**Language/Version**: Go 1.27.2 (server), TypeScript + React 19 (web)

**Primary Dependencies**: chi, pgx + sqlc, goose; Chakra UI v3, TanStack Query (sin dependencias nuevas)

**Storage**: PostgreSQL 16 con RLS por empresa; una migración nueva, `0094_waste.sql`

**Testing**: `go test` (domain, table-driven), integración contra Postgres real bajo `appRoleStore`
con `inTheThreeCases`, prueba de la migración sobre respaldo restaurado con dos empresas; vitest
para las pantallas; Playwright a 1024×600 contra el ambiente de pruebas

**Target Platform**: tabletas de 7 a 10 pulgadas (1024×600), VM Linux

**Project Type**: web (server + web)

**Performance Goals**: registrar una merma en ≤ 3 toques (SC-001); el reporte de un conteo con
~600 insumos en una sola consulta por conteo

**Constraints**: registrar no espera a una venta ni la hace esperar (NOWAIT → 409, como el 038);
nada que dependa de la red más de lo que ya depende; textos para quien opera

**Scale/Scope**: una sucursal, ~600 insumos, decenas de mermas por semana

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principio | Cómo se cumple |
|---|---|
| I Layering | Reglas (estado, tope, periodo, partido del faltante) en `domain/waste.go`; servicio en `app/waste.go` con `store.WithTx`; handlers finos en `httpapi/handlers_waste.go`; SQL solo en `queries/waste.sql` |
| II Errores | Sentinels de `domain` (`ErrValidation`, `ErrConflict`, `ErrNotFound`) envueltos con `%w`; 55P03 → `ErrConflict` como `lockErr` del 038 |
| III Dinero | Costo por unidad `numeric(12,6)` y total `numeric(12,2)` con `Round2`/`ValidMoney`; el tope con `ValidMoney`. El reporte declara qué incluye: merma que restó en el periodo; excluye deshecha, rechazada, pendiente y cubierta. Prueba que falla nombrando lo duplicado: «la merma se contó dos veces en el faltante» |
| IV Tests | Dominio table-driven primero; integración bajo `appRoleStore` en los tres casos; `TestEveryCompanyTableIsIsolated` recoge las dos tablas nuevas; migración probada sobre respaldo con dos empresas; cada defecto con su test en rojo |
| V Seguridad | Todo por `RequirePermission`, fuera de grupos con rol; cantidades con `ValidQty`/`MaxStockQty`; motivo y nota con tope de largo; un motivo, insumo o producto de otra empresa → 404 por llave compuesta |
| VI YAGNI | Sin estados de flujo más allá de los cinco que pide el spec; sin reporte de consumo teórico |
| VII Comentarios e idioma | Identificadores nuevos en inglés (`waste_records`, `waste_reasons`, `inventory.write_off`); texto de pantalla en español de quien opera |
| VIII Puertas | Ver abajo |
| Roles | Permisos nuevos `inventory.write_off` (los roles sugeridos con acceso al local, incluida cocina) e `inventory.write_off_approve` (admin y gerente); motivos y tope con `inventory.configure` |
| Tableta | Sin `<select>` nativo (botones grandes + `Picker`), controles ≥ 44 px, teclado numérico del 038, alto contado contra 600 px |

### Pregunta VIII: ¿se puede agregar después al mismo costo?

- **Más de un almacén por sucursal** (issue #65): no se cierra. El registro guarda `branch_id`; el
  almacén será una columna más, asignando lo existente al principal, igual que en compras y conteo.
- **Costo de la merma**: se guarda el costo por unidad del momento (snapshot). Recalcularlo después
  no se puede al mismo costo, así que se guarda hoy.
- **Cortesía de promoción** (#74): no se cierra. La merma no toca pedidos; la cortesía será una línea
  de pedido con su promo.
- **Consumo teórico contra real**: se agrega después al mismo costo; no se construye.

## Pantallas y maquetas

| Pantalla | Dónde vive | Alto en 1024×600 |
|---|---|---|
| **Se echó a perder** (captura) | Una entrada que vean cajero y cocina: hoy no entran a Almacén. La maqueta decide entre el riel del POS, un botón en la cocina o una sección «Inventario» con acciones grandes (decisión del 9-oct, punto 35) | Motivos frecuentes como botones (2 renglones), buscador de insumo o producto, cantidad con el teclado del 038, Guardar fijo abajo |
| **Por aprobar** | Almacén, pestaña o aviso para admin/gerente | Lista de pendientes con costo, quién y cuándo; Aprobar / Rechazar por renglón |
| **Faltante sin explicar** | Al cerrar un conteo y desde los conteos cerrados (Almacén › Conteo) | Tabla por insumo: perdido, registrado, sin explicar, pesos; total arriba |
| **Merma por motivo** | Almacén o Reportes, con rango de fechas | Un renglón por motivo; consumo del personal aparte |
| **Motivos y tope** | Configuración del negocio › Inventario (sección del 038) | Lista editable; un renglón para el tope |

| Pantalla | Maquetas | Elegida | Por qué |
|----------|----------|---------|---------|
| Se echó a perder | pendiente | pendiente | pendiente |
| Por aprobar | pendiente | pendiente | pendiente |
| Faltante sin explicar | pendiente | pendiente | pendiente |
| Merma por motivo | pendiente | pendiente | pendiente |
| Motivos y tope | pendiente | pendiente | pendiente |

## Capas

| Pieza | Capa | Dónde |
|---|---|---|
| Estados y transiciones (`applied`, `pending`, `rejected`, `covered`, `undone`), ¿pide aprobación?, ¿se puede deshacer? | domain | `server/internal/domain/waste.go` |
| Lo que resta un producto (reusa `StockGraph.ExpandSale`) | domain | `server/internal/domain/composition.go` (sin cambios) |
| Partido del faltante por insumo (perdido = registrado + sin explicar) | domain | `server/internal/domain/waste.go` |
| Nombre de motivo normalizado (reusa la regla de nombres de insumo) | domain | `server/internal/domain/waste.go` |
| Permisos `inventory.write_off`, `inventory.write_off_approve` | domain | `server/internal/domain/permission.go` + `web/src/app/permissions.ts` |
| Acciones de bitácora `waste.recorded`, `waste.approved`, `waste.rejected`, `waste.undone`, `settings.waste_threshold` | domain | `server/internal/domain/audit.go` + `web/src/shared/audit/labels.ts` |
| Esquema: `waste_reasons`, `waste_records`, `stock_movements.waste_record_id`, `business_settings.waste_approval_threshold`, motivos sugeridos | migración | `server/migrations/0094_waste.sql` |
| Motivos sugeridos de una empresa nueva | sqlc | `CreateCompany` en `server/queries/companies.sql` (CTE, no trigger: ver 0076) |
| Consultas | sqlc | `server/queries/waste.sql` |
| Registrar, aprobar, rechazar, deshacer, cubrir al cerrar conteo | app | `server/internal/app/waste.go`; `CloseCount` en `stock_counts.go` llama a la cobertura en su transacción |
| Reportes | app | `server/internal/app/waste_reports.go` |
| Rutas | httpapi | `server/internal/httpapi/handlers_waste.go`, `router.go` |
| Medición de uso | domain + web | `uso.go` y `rutas-medidas.ts` |
| Pantallas | web | `web/src/features/waste/` (captura, por aprobar, reportes), `InventorySettingsSection.tsx` |

## Bordes (se piensan antes del código)

| # | Borde | Cómo se cierra | Test |
|---|---|---|---|
| E1 | Registrar con una venta en curso del mismo insumo | `lockIngredients` del 038 (NOWAIT) antes de escribir → 409 «vuelve a intentar» | integración con transacción abierta |
| E2 | Deshacer dos veces, o aprobar y deshacer a la vez | `FOR UPDATE` del registro y transición validada en `domain` → 409 | integración concurrente |
| E3 | Pendiente aprobada después de un conteo que la cubrió | El cierre del conteo la pasa a `covered` en la misma transacción | integración |
| E4 | Merma de producto sin nada que restar | `ExpandSale` vacío → 422 con texto para quien opera | domain + integración |
| E5 | Insumo sin costo con tope | Pendiente (FR-011) | domain |
| E6 | Faltante: la merma restada se resta otra vez | `domain.SplitLoss` toma lo que el conteo encontró de menos tal cual; prueba nombrada por el concepto duplicado | domain + integración |
| E7 | Periodo con merma aprobada en otro día | Cuenta por la fecha del movimiento (cuando restó), no del registro | integración |
| E8 | Deshacer después de que se contó el insumo | Rechazo: el conteo ya corrigió | integración |
| E9 | Motivo, insumo o producto de otra empresa | Llave compuesta `(id, company_id)` → 404 | integración tres casos |
| E10 | Juntar motivos cruzando empresas | Imposible por llave compuesta; prueba igual | integración |
| E11 | Dos sucursales sin selector | `current_branch_id()` → `BRANCH_AMBIGUOUS`, no adivina | integración |
| E12 | Cantidad vacía, 0, negativa, NaN o enorme | `ValidQty` + `MaxStockQty` → 400 | handler |
| E13 | Movimiento huérfano al deshacer | Deshacer inserta el movimiento inverso con el mismo `waste_record_id`; nunca borra | integración |

## Project Structure

### Documentation (this feature)

```text
specs/039-merma-y-cortesias/
├── plan.md
├── research.md
├── data-model.md
├── quickstart.md
├── contracts/api.md
└── tasks.md             # /speckit-tasks
```

### Source Code (repository root)

```text
server/
├── migrations/0094_waste.sql
├── queries/waste.sql, companies.sql
├── internal/domain/waste.go (+ _test), permission.go, audit.go, uso.go
├── internal/app/waste.go, waste_reports.go, stock_counts.go
├── internal/httpapi/handlers_waste.go, router.go
└── internal/integration/waste_*_test.go, migration_0094_test.go

web/src/
├── features/waste/ (captura, por aprobar, faltante, por motivo)
├── features/admin/InventorySettingsSection.tsx
├── api/backoffice.ts, app/permissions.ts, app/rutas-medidas.ts
└── shared/audit/labels.ts
```

**Structure Decision**: monorepo existente (`server/` + `web/`), sin paquetes nuevos.

## Complexity Tracking

Sin violaciones.
