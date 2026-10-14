# Implementation Plan: Compras al almacén

**Branch**: `038-compras-al-almacen` | **Date**: 2026-10-14 | **Spec**: [spec.md](spec.md)

## Summary

Las compras ya podían sumar al almacén (renglones de gasto que, al recibirse, dejan un movimiento
`compra`), pero la captura no lo permitía de verdad: la pantalla marca todo gasto como recibido aunque
no lleve mercancía, y un gasto recibido no se puede completar ni corregir. Este plan:

- arregla el «recibido» vacío (R1) y deja agregar, corregir y quitar mercancía de un gasto con una
  sola operación por diferencia (R2);
- agrega presentaciones de compra por insumo, copiadas al renglón (R3);
- actualiza el costo del insumo al comprar, con método por empresa: última compra o promedio
  ponderado (R4);
- agrega el conteo de una sucursal, que corrige por diferencia contra lo que había al capturar (R5).

Decisiones en [research.md](research.md); esquema en [data-model.md](data-model.md); rutas en
[contracts/api.md](contracts/api.md).

## Technical Context

**Language/Version**: Go 1.27.2 (server), TypeScript / React 19 (web)
**Primary Dependencies**: chi, pgx + sqlc, goose; Chakra UI v3, TanStack Query
**Storage**: PostgreSQL con RLS (`gatobobah_app`); dos migraciones (0091 tipo de unidad, 0092 el resto)
**Testing**: `go test` (domain table-driven; integración bajo `appRoleStore` con `inTheThreeCases`, dos empresas), vitest, capturas a 1024×600 contra el ambiente de pruebas
**Target Platform**: API en VM Linux; tabletas 1024×600
**Project Type**: web-service + SPA
**Performance Goals**: guardar un gasto de 10 renglones con recálculo de costos en menos de 1 s con el catálogo de producción
**Constraints**: se sube con el local operando; recibir, corregir y cerrar un conteo no bloquean ventas más que la inserción de un movimiento
**Scale/Scope**: 3 pantallas cambiadas (captura y detalle del gasto, insumos), 2 nuevas (por recibir, conteo), 1 renglón de configuración; 3 permisos; 4 acciones de bitácora; un tipo de unidad nuevo (longitud)

## Constitution Check

| Principio | Cómo se cumple |
|---|---|
| I Layering | `queries/purchases.sql`, `queries/stock_counts.sql` (sqlc) → `domain` puro (`NextIngredientCost`, `PlanStockLines`, `CountAdjustment`, validaciones) → `app.ExpensesService` (recibir, corregir, anular), `app.StockCountService`, `app.PresentationsService` con sus transacciones → handlers finos |
| II Errores | `ErrValidation`, `ErrConflict`, `ErrBranchAmbiguous` envueltos con `%w`; mapeo solo en `httpapi.Error` |
| III Dinero | `unit_cost` por unidad base con `domain.UnitCost` (6 decimales), cantidades con `Round4`, importes con `Round2` y `ValidMoney`. El costo actual no se suma a ningún total; las ventas conservan su costo (snapshot) |
| IV Test-first | casos de borde del spec antes del código; dominio table-driven para costo, diferencias y ajustes; integración de la migración con dos empresas y bajo `appRoleStore` con `inTheThreeCases` para cada consulta nueva; prueba de regresión del «recibido» vacío vista en rojo; concurrencia: dos correcciones del mismo renglón (409) y dos compras del mismo insumo con promedio (sin perder una) |
| IV Local = producción | las tres tablas nuevas entran a `TestEveryCompanyTableIsIsolated`; ningún arreglo como owner |
| V Seguridad | `RequirePermission` con `inventory.receive`, `inventory.configure` e `inventory.count`, fuera de los grupos con rol; parámetros mal formados → 400; `countedQty` y cantidades con `ValidQty`/`MaxStockQty`; una presentación de otro insumo o de otra empresa → 400 (RLS + chequeo) |
| VI YAGNI | sin kárdex, sin valor del inventario, sin conteos parciales periódicos, sin órdenes de compra; corrección del costo hacia atrás marcada `ponytail:` (R4) |
| VII Comentarios / idioma | identificadores nuevos en inglés; UI y docs en español; sin nombres del negocio |
| VIII Puertas | se construyen hoy `expense_item_id` y `stock_count_id` en el movimiento y la equivalencia copiada al renglón: después no se recuperan (R9b). Ninguna puerta se cierra; con dos sucursales, 409 en vez de adivinar |

Sin violaciones.

## Capas: dónde aterriza cada pieza

| Pieza | Capa | Archivo |
|---|---|---|
| Tipo de unidad «longitud» (sola: un valor de enum no se usa en la transacción que lo agrega) | migración | `server/migrations/0091_unit_kind_length.sql` |
| Esquema (llaves por empresa, presentaciones, conteos, columnas nuevas, trigger de presentaciones, método de costeo) | migración | `server/migrations/0092_purchases_into_stock.sql` + `server/internal/integration/migration_0092_test.go` |
| Lo aplicado por renglón, inserción de diferencias, bloqueo del gasto y del insumo | store (sqlc) | `server/queries/purchases.sql` |
| Conteos y sus renglones | store (sqlc) | `server/queries/stock_counts.sql` |
| Presentaciones | store (sqlc) | `server/queries/ingredients.sql` |
| `NextIngredientCost`, `PlanStockLines` (deseado contra aplicado), `CountAdjustment`, validaciones | domain | `server/internal/domain/purchases.go`, `stock_count.go` (+ `_test`) |
| Permisos `inventory.receive`, `inventory.configure`, `inventory.count` | domain | `server/internal/domain/permission.go` (+ espejo en `web/src/app/permissions.ts`) |
| Acciones `expense.received`, `expense.stock_corrected`, `stock.counted`, `settings.costing_method` | domain | `server/internal/domain/audit.go` + `web/src/shared/audit/labels.ts` |
| Recibir con costo, corregir, anular recibido, «recibido» solo con mercancía | app | `server/internal/app/expenses.go` |
| Costo del insumo y propagación | app | `server/internal/app/costing.go` (`ApplyPurchaseCost` + `RecomputeAll` en la misma tx) |
| Conteo | app | `server/internal/app/stock_counts.go` |
| Presentaciones | app | `server/internal/app/articles.go` |
| Método de costeo | app | `server/internal/app/settings.go` |
| Rutas | httpapi | `handlers_backoffice.go` (gastos), `handlers_stock_counts.go`, `router.go` |
| Salud: «sin compras» por suma positiva | store | `server/queries/inventory_health.sql` (R8) |
| Filtro «Con pendientes» de la lista de gastos | store | `server/queries/expenses.sql` (`ListExpenses`/`CountExpenses`, mismo `where`) |

## Pantallas y maquetas

Probadas sin contexto con dos personas de comida (un café y una fonda) y, por error, una ferretería;
los hallazgos están en el spec (Clarifications del 14-oct). Las listas de categorías, proveedores y
nombres de empaque salen de la empresa.

1. **Captura de una compra** (cambiada, `ExpenseDialog`) — opción **C, primero el ticket**:
   - Abre con «Sube la foto del ticket»; «Capturar sin ticket» lleva a la captura a mano, que sirve
     también para un gasto sin mercancía («¿Trae mercancía para el almacén? Sí / No, solo el gasto»).
   - Encabezado editable: fecha (`DatePickerButton`), categoría y proveedor (`Picker` de la empresa,
     con «+ Nuevo»), total pagado. No se guarda sin categoría ni total.
   - Renglones del ticket editables: insumo («Buscar o crear», que pregunta en qué se mide), cantidad,
     empaque con su contenido o unidad, «+ Otro empaque» (número y unidad, con la equivalencia antes
     de guardar), importe, y estado **Llegó / Llega después**. Un renglón dudoso («¿Es …?») no suma
     hasta confirmarlo y no bloquea guardar; un renglón surtido se **parte**.
   - El pie dice qué suma, si cuadra el dinero (falta importe, renglones sobre el total) y cuántos
     renglones siguen sin confirmar; los dudosos van arriba de la lista (FR-005a). Si el ticket trae
     otro total, pregunta cuál usar. Los avisos aparecen debajo del encabezado, sin taparlo. Quitar
     un renglón ya capturado pide confirmación y el botón va separado del importe.
2. **Corregir lo que llegó** (cambiada, `ExpenseDetailDialog`) — opción **B, editar todo y ver la
   diferencia**: por renglón, cantidad y estado (Llegó / Llega después / No va a llegar), con
   «Tendrás X guardados (hoy Y)», cifra que se vuelve a pedir al servidor justo antes de habilitar
   «Guardar» (una venta pudo moverla); el pie resume la diferencia antes de guardar. «Anular compra»
   confirma diciendo cuánto quedará y la compra se ve anulada después. El total pagado no cambia.
3. **Almacén › Insumos, «Cómo llega»** (cambiada, columna en la tabla de insumos) — opción **B, a
   la vista en la lista**: empaques activos con su contenido, «Ya no llega en: …», «+ Agregar» y
   «Cambiar ›» que abre la hoja con editar y «Ya no llega así». Nombres sugeridos: los que la empresa
   ya usa, más «Otro…». Empaque aproximado marcado «≈».
**Pestañas de Almacén, en este orden y con estos nombres:** Existencias · Movimientos · Insumos
(con la palabra configurable) · Por recibir · Conteo · Salud. Medido: caben en 1024 px con ~340 px
de sobra.

4. **Almacén › Por recibir** (nueva, pestaña) — opción **A con buscador**: agrupado por compra
   (proveedor, fecha, pagado), por renglón «Recibir…» (cantidad tecleada, acepta de más con su
   importe) y «Ya no va a llegar…» (cuántas); «Llegó todo…» con confirmación; «Llegó algo más…»
   (lo pagué / fue regalo).
5. **Almacén › Conteo** (nueva, pestaña) — **las dos vistas** con interruptor «Lista / Uno por
   uno» (se recuerda por tableta), agrupadas por lugar; teclado numérico en pantalla; a ciegas;
   «✓ guardado»; el cierre nombra lo no contado y marca diferencias grandes.
6. **Configuración del negocio** (un renglón, sin cambiar la disposición): método de costeo y la
   palabra para lo del almacén; el lugar de cada insumo se edita en su hoja de Insumos.

| Pantalla | Maquetas | Elegida | Por qué |
|----------|----------|---------|---------|
| Captura de una compra | https://claude.ai/artifact/BASLWZZp7XrAv98LrfeC23 | C · Primero el ticket (v3) | Elección del dueño: el ticket es el camino normal; la captura a mano queda a un toque |
| Corregir lo que llegó | https://claude.ai/artifact/YXNh1NMBQbfAhgHxbNoF7i | B · Editar todo y ver la diferencia (v3) | Varios cambios a la vez con resumen; la única que dice cuánto quedará guardado |
| Cómo llega un insumo | https://claude.ai/artifact/BD8sJG6j2a8vBrorbDdtaQ | B · A la vista en la lista | Se ve qué insumos no tienen empaque; agregar es un toque |
| Almacén › Por recibir | https://claude.ai/artifact/GcNdLqbCqWHSujwN7Gsrm8 | A con buscador (v3) | El repartidor llega con una nota; el buscador cubre «¿dónde quedó la leche?» |
| Almacén › Conteo | https://claude.ai/artifact/JYz2e9k2Pej1FVHJFPhRax | Las dos vistas, con teclado en pantalla (v3) | El café cuenta con su hoja, la fonda caminando: cada quien elige |

## Project Structure

```text
specs/038-compras-al-almacen/  spec.md plan.md research.md data-model.md quickstart.md contracts/api.md

server/migrations/0091_unit_kind_length.sql, 0092_purchases_into_stock.sql
server/queries/purchases.sql, stock_counts.sql, ingredients.sql, inventory_health.sql, expenses.sql
server/internal/domain/purchases.go, stock_count.go, permission.go, audit.go (+_test)
server/internal/app/expenses.go, costing.go, stock_counts.go, articles.go, settings.go
server/internal/httpapi/handlers_backoffice.go, handlers_stock_counts.go, router.go
server/internal/integration/migration_0092_test.go, purchases_test.go, stock_counts_test.go

web/src/api/backoffice.ts
web/src/features/backoffice/ExpenseDialog.tsx, ExpenseDetailDialog.tsx, expenseDraft.ts (+test)
web/src/features/backoffice/StockPage.tsx, StockCountTab.tsx, PendingReceiptsTab.tsx, PresentationsSheet.tsx (+test)
web/src/components/NumericKeypad.tsx (+test)                 # teclado del conteo
web/src/features/admin/BusinessSettingsPage.tsx (+test)
web/src/app/permissions.ts, web/src/shared/audit/labels.ts
```

**Structure Decision**: monorepo existente (`server/` + `web/`), sin módulos nuevos.

## Riesgos

- **Recalcular costos en cada compra** recorre el grafo de costos de la empresa. Con el catálogo de
  hoy es barato, pero se mide con `EXPLAIN` y un gasto de 10 renglones sobre la copia de producción
  antes de darlo por bueno.
- **Venta durante el conteo:** resuelto por diferencia contra `system_qty` (R5); un renglón
  recapturado actualiza los dos números.
- **El hermano que no se movió:** la lectura de tickets y los formatos aprendidos llenan renglones
  por el mismo `CreateExpense`; la prueba de regresión cubre los dos caminos. `RecordMovement`
  (ajuste manual) sigue sin bitácora ni lista blanca de tipos: es anterior y queda fuera, anotado.
- **Rollback:** la 0092 solo agrega; su Down se niega si hay conteos, presentaciones usadas o
  movimientos con `expense_item_id`, como la 0089. La 0091 no puede quitar el valor del enum.
- **Bloqueos:** corregir, recibir y cerrar un conteo insertan movimientos ordenados por insumo, y el
  bloqueo del insumo para el costo es `NOWAIT` (data-model, «Movimientos: sucursal y orden»).
