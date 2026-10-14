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
**Storage**: PostgreSQL con RLS (`gatobobah_app`); una migración (0091)
**Testing**: `go test` (domain table-driven; integración bajo `appRoleStore` con `inTheThreeCases`, dos empresas), vitest, capturas a 1024×600 contra el ambiente de pruebas
**Target Platform**: API en VM Linux; tabletas 1024×600
**Project Type**: web-service + SPA
**Performance Goals**: guardar un gasto de 10 renglones con recálculo de costos en menos de 1 s con el catálogo de producción
**Constraints**: se sube con el local operando; recibir, corregir y cerrar un conteo no bloquean ventas más que la inserción de un movimiento
**Scale/Scope**: 2 pantallas cambiadas (captura y detalle del gasto), 2 nuevas (presentaciones de un insumo, conteo), 1 renglón de configuración; 3 permisos; 4 acciones de bitácora

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
| Esquema (presentaciones, conteos, columnas nuevas, método de costeo) | migración | `server/migrations/0091_purchases_into_stock.sql` + `server/internal/integration/migration_0091_test.go` |
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

## Pantallas y maquetas

1. **Captura del gasto** (cambiada, `ExpenseDialog`): cada renglón de mercancía elige insumo,
   presentación (o unidad) y cantidad; muestra en una línea lo que suma («= 36 L»). Desde el
   renglón se crea la presentación que falta. «Ya llegó» solo aparece con renglones de mercancía y
   arranca encendido.
2. **Detalle del gasto** (cambiada, `ExpenseDetailDialog`): «Corregir lo que llegó» abre los
   renglones editables con lo que ya sumó cada uno; «Anular» de un gasto recibido avisa que se
   restará lo que sumó.
3. **Cómo llega un insumo** (nueva, desde Almacén › Insumos): una hoja con sus presentaciones
   («Caja = 12 L»), agregar, editar y desactivar.
4. **Almacén › Conteo** (nueva, pestaña de `StockPage`): empezar, buscar o recorrer insumos,
   capturar cuánto hay (con ayuda de presentaciones), ver cuántos van contados y cerrar o descartar.
5. **Costeo** (un renglón en la configuración del negocio): «Costo de los insumos: última compra /
   promedio». No cambia la disposición de la pantalla.

| Pantalla | Maquetas | Elegida | Por qué |
|----------|----------|---------|---------|
| Captura del gasto | | | |
| Detalle del gasto | | | |
| Cómo llega un insumo | | | |
| Almacén › Conteo | | | |

## Project Structure

```text
specs/038-compras-al-almacen/  spec.md plan.md research.md data-model.md quickstart.md contracts/api.md

server/migrations/0091_purchases_into_stock.sql
server/queries/purchases.sql, stock_counts.sql, ingredients.sql, inventory_health.sql
server/internal/domain/purchases.go, stock_count.go, permission.go, audit.go (+_test)
server/internal/app/expenses.go, costing.go, stock_counts.go, articles.go, settings.go
server/internal/httpapi/handlers_backoffice.go, handlers_stock_counts.go, router.go
server/internal/integration/migration_0091_test.go, purchases_test.go, stock_counts_test.go

web/src/api/backoffice.ts
web/src/features/backoffice/ExpenseDialog.tsx, ExpenseDetailDialog.tsx, expenseDraft.ts (+test)
web/src/features/backoffice/StockPage.tsx, StockCountTab.tsx, PresentationsSheet.tsx (+test)
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
- **Rollback:** la 0091 solo agrega; su Down se niega si hay conteos o movimientos con
  `expense_item_id`, como la 0089.
