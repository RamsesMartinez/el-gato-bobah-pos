# Implementation Plan: Salud del inventario

**Branch**: `037-salud-del-inventario` | **Date**: 2026-10-13 | **Spec**: [spec.md](spec.md)

## Summary

Una pestaña nueva en Almacén, «Salud», muestra cinco secciones calculadas en el servidor y
exportables a CSV:
- lo que se vende sin restar (medido por las ventas que no dejaron movimiento), con costo estimado
  y los combos con un hueco de varios productos;
- lo guardado en negativo (resumido mientras no haya compras);
- insumos sin compras;
- recetas con un error;
- recetas por revisar.

La hoja de receta del producto gana la tarjeta «Se compra hecho». La existencia propia deja de
ofrecerse: un producto que la tiene sale de ella eligiendo otra tarjeta, y su existencia se traslada
o se cierra con la bitácora. Nada recalcula ventas anteriores (criterio del dueño, 13-oct: de aquí en
adelante, no hacia atrás).

No hay migración: todo se deduce de columnas que ya existen (research R1). Decisiones en
[research.md](research.md); contratos en [contracts/api.md](contracts/api.md).

## Technical Context

**Language/Version**: Go 1.27.2 (server), TypeScript / React 19 (web)
**Primary Dependencies**: chi, pgx + sqlc; Chakra UI v3, TanStack Query; `encoding/csv` (stdlib) para la exportación
**Storage**: PostgreSQL con RLS (`gatobobah_app`); sin migración
**Testing**: `go test` (domain table-driven, integración bajo `appRoleStore` con `inTheThreeCases`), vitest, capturas a 1024×600
**Target Platform**: API en VM Linux; tabletas 1024×600
**Project Type**: web-service + SPA
**Performance Goals**: el reporte de 30 días responde en menos de 1 s con el volumen de producción (unos cientos de productos y miles de renglones)
**Constraints**: se sube con el local operando; ninguna lectura del reporte bloquea ventas
**Scale/Scope**: 1 pantalla nueva (Almacén › Salud), 1 hoja cambiada (receta del producto), 1 permiso, 1 acción de bitácora

## Constitution Check

| Principio | Cómo se cumple |
|---|---|
| I Layering | `queries/inventory_health.sql` (sqlc) → `domain` puro (`HealthFilter.Validate`, `ModeOf`, `PlanModeChange`, `HealthIssueKind`) → `app.InventoryHealthService` (lectura y CSV) y `app.AdminService.SaveComposition` (transición en su tx) → handlers finos `GetInventoryHealth` y `ExportInventoryHealth` |
| II Errores | `domain.ErrValidation` / `ErrConflict` envueltos con `%w`; mapeo solo en `httpapi.Error`; ctx propagado hasta la query |
| III Dinero | no toca dinero. Las cantidades de existencia usan `Round4`; el traslado multiplica por la cantidad de la receta y redondea una vez |
| IV Test-first | casos de borde del spec enumerados antes (sección *Edge Cases*); tests de dominio de `PlanModeChange` y `HealthFilter`; integración del reporte **y de la transición de modo en `SaveComposition`** (que ahora escribe `stock_levels`/`stock_movements`) bajo `appRoleStore` con `inTheThreeCases`; test de concurrencia: una venta entre mostrar y guardar responde 409 y nunca deja el producto distinto de cero ni inventa existencia en el insumo; test que falla nombrando «la exportación y la pantalla divergen»; test de regresión con el caso de la auditoría (producto cuyo extra obligatorio descuenta: no se marca) |
| IV Local = producción | el test del reporte corre con dos empresas y bajo el rol de la app |
| V Seguridad | `RequirePermission(PermInventoryHealthView)` en las dos rutas; parámetros mal formados → 400; el CSV no lleva datos de otra empresa (RLS + test) y escapa celdas que empiezan con `=`, `+`, `-` o `@` (inyección de fórmulas en hojas de cálculo) |
| VI YAGNI | sin tabla de modo, sin endpoint de conversión, sin permiso por sección, sin PDF |
| VII Comentarios / idioma | identificadores en inglés (`inventory_health`, `consumption_mode`); UI y docs en español; sin nombres del negocio en código ni migraciones (FR-018) |
| VIII Puertas | ninguna se cierra: sin cambios de esquema; con más de una sucursal el reporte muestra la sucursal en vez de asumir una (R8) |

Sin violaciones.

## Capas: dónde aterriza cada pieza

| Pieza | Capa | Archivo |
|---|---|---|
| Consultas del reporte (5 secciones + sus `Count`) | store (sqlc) | `server/queries/inventory_health.sql` |
| `HealthFilter`, `HealthIssueKind`, `ConsumptionMode`, `ModeOf`, `PlanModeChange` | domain | `server/internal/domain/inventory_health.go`, `consumption_mode.go` (+ `_test`) |
| Permiso `inventory.health_view` | domain | `server/internal/domain/permission.go` (+ espejo en `web/src/app/permissions.ts`) |
| Acción de bitácora `product.consumption_mode` | domain | `server/internal/domain/audit.go` (lista blanca, grupo nuevo `inventory`) |
| Reporte y CSV | app | `server/internal/app/inventory_health.go` |
| Transición de modo en la captura | app | `server/internal/app/composition.go` (`SaveComposition` con `actualStock`; `validateComposition` deja de rechazar receta en `track_stock`) |
| Rutas | httpapi | `server/internal/httpapi/handlers_inventory_health.go`, `router.go` (bajo `/stock`) |
| Pantalla Salud | web | `web/src/features/backoffice/InventoryHealthTab.tsx` (pestaña de `StockPage`) |
| Tarjeta «Se compra hecho» y «¿Cuántas tienes de verdad?» | web | `web/src/shared/CompositionSheet.tsx` |
| Filtro por URL en Recetas | web | `web/src/features/admin/RecipesPage.tsx` (`?status=`, `?open=`) |
| Medición de uso | web + domain | `pantallasMedibles` / `rolesPorPantalla` en `uso.go` y `PANTALLAS` en `rutas-medidas.ts` (AGENTS.md: tres lugares) |

## Pantallas y maquetas

Las llena `/maquetas` (hook `after_plan`): 2 o 3 opciones navegables por pantalla; el dueño elige.

1. **Almacén › Salud** (nueva, pestaña de `StockPage`) — opción **A**:
   - Arriba, una barra: el periodo, que abre una hoja inferior con «Hoy», «Últimos 7 días», «Últimos 30 días», «Este mes» y «Elegir fechas…», más un resumen de una línea y «Exportar».
   - Contenedor: `StockPage` pasa a `<Page fill>` (las tres pestañas de hoy igual, solo dentro de esa estructura) y `InventoryHealthTab` NO trae su propio `Page`: es un `Box flex="1" minH={0}` dentro de su `Tabs.Content`, con la barra fija arriba y la lista con scroll. Los `Tabs.Trigger` de Almacén ganan `minH="44px"` (corrige también las tres existentes).
   - El periodo: `Picker` con las cinco opciones; «Elegir fechas…» abre dos `DatePickerButton` (desde y hasta). Ningún `input type="date"`.
   - Debajo, una sola lista (`flex="1" minH={0}`, overflow con alto). Cada una de las cinco secciones es un renglón de 52 px con punto de color, nombre y número; se abre y se cierra con un toque. «Se vende sin descontar» abre por omisión.
   - Renglones de 54 px:
     - con nombre y grupo, más una pastilla de estado («Falta decir qué resta», «Extra que no resta nada»);
     - las piezas por canal como pastillas y el costo estimado a la derecha (ordena la lista); el renglón completo (54 px) es el botón que lleva a corregir, como en Recetas, y «›» solo lo señala;
     - «Ver 25 más · quedan N» como en Recetas.
   - Al pie de «Se vende sin restar», una línea fija: «Las ventas anteriores no se recalculan».
   - Los combos con un hueco de varios productos salen dentro de esa sección con su pastilla («Siempre resta el de fábrica»).
   - «Lo guardado en negativo», mientras la empresa no tenga compras, abre con un renglón que explica que lo guardado solo baja y se despliega con un toque.
   - «Insumos sin compras registradas» abierto muestra un renglón resumen («118 de 130 insumos sin compras registradas») que se despliega.
   - «Recetas por revisar» muestra un renglón con el texto de FR-019 tal cual, «Las cargó el sistema y nadie las ha confirmado», y «Ir a Recetas ›». El aviso de la hoja de receta (`CompositionSheet`, «Esta receta se cargó de tu sistema anterior…») cambia a la misma frase.
   - Una empresa sin problemas ve «El inventario está sano» en vez de cinco ceros.
2. **Hoja de receta del producto** (cambiada) — opción **B simplificada, versión 7**, después de cinco pruebas con usuarios sin contexto (las versiones 1 a 4, con dos preguntas y resúmenes, no se entendían):
   - Título en pregunta: «Cuando vendes X, ¿qué se resta de lo que tienes?».
   - Cuatro tarjetas: «Se prepara aquí» (la de «Lleva insumos» de hoy), «Se compra hecho» (nueva), «Es un combo» y «No se cuenta».
   - «Existencia propia» NO aparece en pantalla (decisión del dueño: los revendidos son receta). Un producto que hoy la tiene se convierte eligiendo «Se compra hecho».
   - «Se compra hecho» viene lleno: «Cada venta resta 1 de [insumo]». Si existe uno de nombre parecido, se preselecciona; si no, «se crea con este nombre». El buscador marca con ✓ lo elegido y deja «Crear una nueva» al final. «¿Una venta lleva más de una? (por ejemplo, un six)» muestra el − / +.
   - Avisos solo cuando hay existencia que mover:
     - Si el insumo es nuevo: «El sistema tenía contadas 12 latas… cada vez que vendas una, quedará una menos».
     - Si el insumo ya existía con existencia: «¿Cuántas tienes de verdad?» con 12, 20, «32 (las dos)» u «Otra» (número escrito), en rejilla de 2×2 como las tarjetas de modo; el campo de «Otra» lleva el mismo `onFocus` → `scrollIntoView({ block: 'center' })` que los otros campos de la hoja, para que el teclado no tape «Guardar»; «Guardar» se apaga hasta elegir. Reemplaza el «sumar o quedarse» de FR-014a: la cifra elegida queda como existencia del insumo, con un ajuste que lo dice.
     - En las otras tres tarjetas, y solo si el producto tenía existencia propia (FR-020): «Tus latas siguen donde están; el sistema solo deja de contarlas…», con confirmación antes de guardar (FR-014b).
   - «Guardar» cierra la hoja; el renglón de Menú › Recetas muestra «Resta 1 por venta · tienes N».
   - Palabras para quien no conoce el sistema: «se resta» y «lo que tienes guardado», no «insumo» ni «existencia propia».

| Pantalla | Maquetas | Elegida | Por qué |
|----------|----------|---------|---------|
| Almacén › Salud | https://claude.ai/artifact/3rvxVnxqBmmevh4WsEkSJj | A · Secciones que se abren | Los cinco problemas de un vistazo, sin alto fijo; la primera abierta |
| Hoja de receta: modo de consumo | https://claude.ai/artifact/WkmDrBVZgXaqDw2txF9uvm | B simplificada, versión 7 | La hoja de hoy + «Se compra hecho»: 2 toques en el caso normal; validada con 5 pruebas de usuario sin contexto |

## Project Structure

```text
specs/037-salud-del-inventario/  spec.md plan.md research.md data-model.md quickstart.md contracts/api.md

server/queries/inventory_health.sql
server/internal/domain/inventory_health.go (+_test)
server/internal/domain/consumption_mode.go (+_test)
server/internal/domain/permission.go               # inventory.health_view
server/internal/domain/audit.go                    # product.consumption_mode, grupo inventory
server/internal/app/inventory_health.go
server/internal/app/composition.go                 # transición de modo
server/internal/httpapi/handlers_inventory_health.go
server/internal/httpapi/router.go
server/internal/integration/inventory_health_test.go
server/internal/integration/consumption_mode_test.go

web/src/api/backoffice.ts                          # health, exportHealth
web/src/features/backoffice/InventoryHealthTab.tsx (+test)
web/src/features/backoffice/StockPage.tsx          # pestaña
web/src/shared/CompositionSheet.tsx (+test)        # «Se compra hecho», salida de existencia propia
web/src/features/admin/RecipesPage.tsx (+test)     # ?status= ?open=
web/src/app/permissions.ts, web/src/app/rutas-medidas.ts
```

**Structure Decision**: monorepo existente (`server/` + `web/`), sin módulos nuevos.

## Riesgos

- **El reporte es una consulta pesada** (anti-join de renglones contra movimientos). Hay índice en `stock_movements(order_line_id)` desde la 0060; el `EXPLAIN` sobre la copia de producción entra en la revisión de `db-architect`.
- **Venta simultánea a la conversión.** Las filas de `stock_levels` se bloquean con `FOR UPDATE` dentro de la tx y la petición trae lo que vio la pantalla; si cambió, 409 (research R6).
- **Una sucursal o varias.** «¿Cuántas tienes de verdad?» se resuelve con una sola sucursal con existencia; con varias responde 409 y lo resuelve el conteo del 038 (research R6).
- **Habilitar receta en un producto con existencia propia** cambia una validación vieja (`validateComposition` rechazaba). El hermano que no se movió (constitución IV): la carga de FUDO y `CloneProduct` siguen fijando `track_stock` sin pasar por la transición; quedan fuera, con test de que no cambian.
