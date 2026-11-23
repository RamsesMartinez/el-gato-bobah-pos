# Implementation Plan: Merma esperada, conteo contra ventas y rediseño del inventario

**Branch**: `096-merma-esperada` | **Date**: 2026-11-23 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `specs/096-merma-esperada/spec.md` (issue #226)

## Summary

S1 (costos): tres porcentajes nullable —general, producto, artículo— en columnas nuevas (migración
0121), una función pura de precedencia en `domain` que el motor de costeo usa por renglón, el
desglose del costo por producto en el endpoint de costeo que ya existe, y la merma real de 90 días
calculada al pedirla por artículo y almacén. S2 (inventario): reporte conteo vs ventas, opción de
descontar la merma esperada, vistas sencilla y avanzada de conteo y existencias, y la carga medida
contra una meta; va encima de las pantallas de la spec 094 cuando el #65 esté completo.

## Technical Context

**Language/Version**: Go 1.27.4 (server), TypeScript / React 19 (web)

**Primary Dependencies**: chi, pgx + sqlc, goose (embebidas); Chakra UI v3, TanStack Query

**Storage**: PostgreSQL con RLS (`gatobobah_app`); migración 0121 (S1) y una de S2 con número al
traer develop entonces

**Testing**: `go test` table-driven en `domain`; integración bajo `appRoleStore` con
`inTheThreeCases` para la merma real y los `PUT`; prueba de la 0121 sobre respaldo restaurado con dos
empresas; vitest; Playwright a 1024×600 (pedido del dueño) y 1280×854

**Target Platform**: equipo de mostrador táctil, 1280×854 mínimo; esta feature se diseña y prueba
también a 1024×600 por pedido del dueño

**Project Type**: web-service + web-app (monorepo `server/` + `web/`)

**Performance Goals**: el recálculo completo sigue en el orden de hoy (< 50 ms con ~750 productos,
comentario de `RecomputeAll`); el desglose de un producto en una petición; S2: SC-007

**Constraints**: subida con el negocio operando; la API anterior lee `waste_pct` y escribe sin las
columnas nuevas (R1); no se recalculan ventas pasadas (R7)

**Scale/Scope**: 2 empresas; ~763 artículos activos, ~750 productos por empresa

## Constitution Check

| Principio | Cómo se cumple |
| --- | --- |
| I Layering | Precedencia, validación, desglose y merma real puros en `domain/costing.go` y `domain/expected_waste.go`; orquestación y recálculo en `app/costing.go` y `app/expected_waste.go`; handlers finos en `httpapi/handlers_expected_waste.go`; SQL en `queries/costing.sql`, `settings.sql`, `admin.sql`, `ingredients_admin.sql`, `expected_waste.sql` |
| II Errores | `domain.ErrValidation` para un % fuera de rango o ausente; 404 por `notFound` en producto o artículo ajeno; mapeo solo en `httpapi.Error` |
| III Dinero | Costos con `Round4` por renglón y total = suma de renglones (R4); el % se valida con tope y `ValidMoney`-style para NaN/Inf. El desglose declara qué incluye: base, merma y total; la merma real no es dinero y no se suma a nada |
| IV TDD / RLS | Bordes enumerados abajo, cada uno con test antes del código; merma real y `PUT` bajo `appRoleStore` con `inTheThreeCases`; prueba de la 0121 con dos empresas (ningún costo cambia); test de que una venta pasada conserva su costo. La 0121 es aditiva y tolera la versión anterior (R1) |
| V Seguridad | `RequirePermission(menu.configure)` en los tres `PUT` y el desglose; `inventory.view` en la merma real; id ajeno = 404; `pct` malformado o ausente = 400, nunca default |
| VI YAGNI | Sin tabla para la merma real ni el desglose (se calculan); sin permiso nuevo; sin % por presentación ni por almacén |
| VII Idioma | `expected_waste_pct`, `ExpectedWastePct`, acciones `*.expected_waste`; textos en español |
| VIII Puertas | ¿Se puede agregar después al mismo costo? El % de cada nivel sí; lo que no: la distinción «0 a propósito / sin captura», que hoy no existe y no se podría reconstruir — por eso entra nullable en S1. «Costear con recetas»: el snapshot `order_lines.unit_cost` sigue siendo lo que cierra el pasado; esta feature no lo toca |

**Bordes enumerados antes de escribir código** (constitución IV):
- *Valor vacío que significa algo*: `pct` vacío en pantalla = `null` = sin captura, nunca 0; `"pct": 0`
  es 0 capturado; sin la llave `pct` = 400. En el front, `Number('')` no se usa: un campo vacío manda
  `null` explícito.
- *Estado que no sobrevive*: el % se guarda al servidor al confirmar; no hay borrador local.
- *Camino nuevo que se salta el control viejo*: los `PUT` nuevos recalculan con el mismo
  `recomputeCosts` (mismo candado de composición) que guardar una receta; si el candado está ocupado,
  409 «vuelve a intentar», como hoy.
- *Hermano que no se movió*: `ListIngredientsForCosting` (deja de leer `waste_pct`), `fudo-import`
  (escribe la columna nueva), la carga del grafo en `loadGraph`, `ProductCost` (usa el mismo
  grafo con el general), el detalle de artículo y de producto (exponen el campo), y el recálculo al
  arrancar (`RecomputeCompanies`) que con el general cargado da lo mismo que el recálculo en línea.
- *Compuesto*: un preparado dentro de un preparado no recibe el general (R2), test que lo nombra.
- *Concurrencia*: dos `PUT` al general: cada uno toma el candado de composición; gana el último y los
  dos quedan en la bitácora.

## Pantallas y maquetas

| Pantalla | Subida | Dónde vive | Qué cambia | Componentes |
| --- | --- | --- | --- | --- |
| Ajustes › Costos (merma general) | S1 | Configuración del negocio, sección nueva visible con `menu.configure` | Un renglón «Merma al preparar» con el % general, vacío por omisión, y una línea de qué hace | `Input` numérico de 44 px, botón Guardar, `InfoButton` |
| Ficha del producto: % y de dónde sale el costo | S1 | `ProductEditDialog`, bloque de Costo | Campo «Merma al preparar» (vacío = usa el general, con el general a la vista) y botón «Ver de dónde sale» que abre el desglose: renglón por artículo con cantidad, costo, %, origen y pesos sumados; merma real junto al %; total abajo | Hoja inferior (`DrawerRoot`), `Table`, etiquetas de estado del tema |
| Ficha del artículo: % y merma real | S1 | `ArticleSheet` | Campo «Merma al preparar» con estado «Sin capturar» distinto de 0 %, y «Real en 90 días» por almacén | `Input`, `Segmented` o botón «Sin capturar», texto auxiliar |
| Conteo vs ventas | S2 | Al cerrar un conteo y en el historial del conteo (junto a `CountLossReport`) | Tabla por artículo con las columnas de FR-020, sugerencia de % y marca «revisar» | `Table`, `SortHead`, `Segmented` de almacén (094) |
| Conteo: vista sencilla y avanzada | S2 | Almacén › Conteo (sobre la pantalla con almacén de la 094) | Sencilla: recorrido por lugar, a ciegas, avance N de M, teclado numérico; avanzada: esperado, diferencia, lugares, cerrar. Interruptor de vista | `NumericKeypad`, `Segmented`, `Table` |
| Existencias: vista sencilla y avanzada | S2 | Almacén › Existencias (con selector de almacén de la 094) | Sencilla: qué hay y qué falta; avanzada: valor, mínimos, movimientos | `Segmented`, `Table` |
| Ajustes › Almacén: descontar merma | S2 | `InventorySettingsSection` | Interruptor «La merma esperada también descuenta del almacén» | Interruptor existente del tema |

Maquetas a 1024×600 (pedido del dueño) y 1280×854, con la cifra de cada opción contra la pantalla
actual (toques y renglones visibles).

| Pantalla | Maquetas | Elegida | Por qué |
|----------|----------|---------|---------|
| Ajustes › Costos (merma general) | [maquetas](https://claude.ai/artifact/NFvuSJ9CPACsYwYboC7XuQ) | A · Sección «Costos» propia | La ve quien configura el menú sin depender del permiso de almacén; 3 toques |
| Ficha del producto: % y de dónde sale el costo | [maquetas](https://claude.ai/artifact/VFw8d6naWiKZKWsJa2YURD) | B · Pestaña «Costo» | % arriba, tres cifras y desglose en un lugar; el efecto del % se ve al teclear |
| Ficha del artículo: % y merma real | [maquetas](https://claude.ai/artifact/6RSvUpgY8serHMQGmqWK9f) | A · «Sin capturar / Capturada» — **por afinar** | El dueño pidió mejorarla con lo aprendido en las maquetas de otras specs (094: color heredado de Apariencia, estado como ícono, sin mayúsculas ni insignias de colores); falta rehacerla y confirmarla |
| Conteo vs ventas (S2) | [maquetas](https://claude.ai/artifact/KxsQHew5tGpHWaCcsYRr98) | Pendiente | |
| Conteo sencilla/avanzada (S2) | [maquetas](https://claude.ai/artifact/RPsFWUsVS8C2itYvBX9n1d) | Pendiente | |
| Existencias sencilla/avanzada (S2) | [maquetas](https://claude.ai/artifact/5wmZHee7CYAwTfU3NXSLtD) | Pendiente | |
| Ajustes › Almacén: descontar merma (S2) | [maquetas](https://claude.ai/artifact/NFvuSJ9CPACsYwYboC7XuQ) (pestaña S2) | Única disposición | Un renglón con interruptor; sin alternativa razonable |

## Project Structure

### Documentation (this feature)

```text
specs/096-merma-esperada/
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
├── migrations/0121_expected_waste.sql                 # S1
├── queries/costing.sql, settings.sql, admin.sql, ingredients_admin.sql
├── queries/expected_waste.sql                         # merma real (S1), conteo vs ventas (S2)
├── internal/domain/costing.go (+ _test.go)            # precedencia por renglón, desglose
├── internal/domain/expected_waste.go (+ _test.go)     # validar %, merma real, (S2) sugerencia
├── internal/app/costing.go, expected_waste.go
├── internal/httpapi/handlers_expected_waste.go, handlers_menu.go (desglose)
├── internal/integration/expected_waste_test.go, migracion_0121_test.go
└── cmd/fudo-import/main.go                            # escribe expected_waste_pct
web/src/
├── features/admin/CostSettingsSection.tsx (+ test)    # S1
├── shared/ProductEditDialog.tsx, shared/CostBreakdownSheet.tsx (+ test)
├── features/backoffice/articles/ArticleSheet.tsx, ExpectedWasteField.tsx (+ test)
├── features/backoffice/ (S2: CountVsSalesReport, vistas de conteo y existencias)
└── api/admin.ts, api/backoffice.ts, api/pos.ts
docs/mecanica/almacen.md                               # la precedencia y por qué el general no compone
```

**Structure Decision**: monorepo existente; sin carpetas nuevas.

## Complexity Tracking

| Violation | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|-------------------------------------|
| `waste_pct` y `expected_waste_pct` vivas a la vez en S1 | La API anterior lee `waste_pct` durante el traslape | Volver nullable `waste_pct` en S1 truena su escaneo (R1) |
