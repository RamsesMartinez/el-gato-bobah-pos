# Implementation Plan: Merma esperada y categorías del menú desde el MCP

**Branch**: `099-mcp-merma-y-categorias` | **Date**: 2026-11-28 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `specs/099-mcp-merma-y-categorias/spec.md`

## Summary

Tres herramientas nuevas del MCP, todas sobre rutas de la API (el MCP arma el mismo cuerpo que una
pantalla y llama la misma ruta, como el resto de sus escrituras):

1. **`ingredient_expected_waste_set`** — herramienta propia (no un campo de `ingredient_update`),
   con `pct` o `clear: true` y `reason`. Su vista previa usa una ruta nueva de solo lectura que
   simula el cambio sobre el grafo de costeo en memoria y devuelve el costo antes y después de cada
   producto, extra y preparado que usa el insumo. Aplicar usa la ruta que ya tiene la pantalla
   (`PUT /stock/articles/{id}/expected-waste`), que acepta ahora un `reason` opcional que va a la
   bitácora.
2. **`category_create`** — ruta nueva `POST /admin/categories`.
3. **`category_update`** — `PATCH /admin/categories/{id}` acepta además `name`, `isActive` y
   `parentId`. `categories_list` gana `include_inactive`.

Las reglas del árbol de categorías (dos niveles, padre raíz y activo, no apagar con productos o
subcategorías activas) son una función pura en `domain`; el servicio lee los hechos con candado y la
aplica. **Sin migración.**

## Technical Context

**Language/Version**: Go 1.27.4 (server). Sin cambios en `web/`.

**Primary Dependencies**: chi, pgx + sqlc, `github.com/modelcontextprotocol/go-sdk` (servidor MCP ya existente), shopspring/decimal.

**Storage**: PostgreSQL. Tablas existentes: `categories`, `ingredients.expected_waste_pct`, `audit_events`. Sin columnas nuevas.

**Testing**: `go test` table-driven en `domain`; integración contra Postgres bajo `appRoleStore` con `inTheThreeCases`; pruebas del despacho MCP en `httpapi` (`mcp_dispatch_test.go`, `TestEveryToolMatchesItsRouteGate`, `TestEveryWriteToolWithIDsDeclaresItsRefs`).

**Target Platform**: API en la VM de producción (azul/verde).

**Project Type**: web-service (backend del POS + servidor MCP embebido).

**Performance Goals**: la vista previa de merma carga el grafo de costeo completo una vez (lo mismo que ya hace cada recálculo); con el catálogo de hoy (~180 productos) es despreciable.

**Constraints**: la fórmula de costeo no cambia; la API vieja y la nueva conviven en la subida (no hay cambio de esquema, así que trivialmente sí).

**Scale/Scope**: 3 herramientas MCP, 2 rutas nuevas, 2 rutas ampliadas, 1 función de dominio de reglas de categoría, 1 función de dominio de impacto de merma.

## Constitution Check

| Principio | Cómo se cumple |
|---|---|
| I. Layering | `httpapi`: handlers finos (decodificar, validar forma, llamar servicio) y las herramientas MCP. `app`: `AdminService.CreateCategory/UpdateCategory` (tx + candados + bitácora) y `CostingService.ArticleExpectedWastePreview`. `domain`: `CategoryRules` (validación del árbol) y `ExpectedWasteImpact` (costo antes/después sobre el grafo). `store/db`: consultas sqlc nuevas en `queries/catalog_media.sql`/`admin.sql`. |
| II. Errores | Sentinels de `domain` (`ErrValidation`, `ErrNotFound`, `ErrConflict`) envueltos con `%w`; sentinels y códigos nuevos (`CATEGORY_NAME_TAKEN`, `CATEGORY_HAS_ACTIVE_PRODUCTS`, `CATEGORY_HAS_ACTIVE_CHILDREN`, `CATEGORY_PARENT_INVALID`) con su pista en `toolFixByCode`. Mapeo HTTP solo en `httpapi.Error`. |
| III. Dinero | El costo es estimado `float64` del motor existente; la vista previa redondea con `Round4` (costo por unidad base) y `Round2` (costo de producto) en la frontera. El % se valida con `domain.ValidExpectedWastePct` (ya existe). |
| IV. Test-first | Bordes enumerados en el spec y en [research.md](research.md); tests de `domain` primero, luego integración con `inTheThreeCases` para crear, editar, vista previa y aplicar. Sin migración, así que sin test de migración. |
| V. Seguridad | Permiso `menu.configure` en las rutas nuevas y ampliadas (el mismo de la pantalla); el MCP lo verifica con `TestEveryToolMatchesItsRouteGate`. Ids de otra empresa: chequeo de refs del MCP (#335) **y** lectura bajo RLS en el servicio (la FK `categories.parent_id` es de una sola columna y salta RLS: el servicio no puede confiar en ella). Parámetros inválidos (`pct`, `include_inactive`) → 400, nunca default silencioso. |
| VI. YAGNI | Sin pantalla nueva, sin orden ni color de categoría, sin borrar categorías, sin merma de producto ni general en el MCP. |
| VII. Idioma | Identificadores, códigos y acciones de bitácora en inglés (`category.created`, `category.updated`); mensajes al agente en español. |
| VIII. Puertas | **¿Se puede agregar después al mismo costo?** Sí: no hay columna ni hecho nuevo que registrar; la bitácora ya guarda el antes y el después. No se toca `order_lines`. **Puerta a medio cerrar desde antes, que este cambio vuelve más frecuente**: el renglón no guarda la categoría, y mover categorías o productos desde el MCP reescribe el pasado de cualquier reporte por categoría. No se construye el snapshot aquí; queda nombrado. Ninguna puerta de la tabla se cierra. |
| Migración tolera versión anterior | No hay migración. |
| Restricciones (tableta) | Sin pantallas nuevas. |

**Resultado**: pasa, sin excepciones.

## Pantallas y maquetas

Sin pantallas nuevas. Las rutas ampliadas mantienen exactamente el comportamiento que usa hoy la
pantalla de categorías (`foodFamily`) y la de merma del artículo (`pct` sin `reason`).

## Project Structure

### Documentation (this feature)

```text
specs/099-mcp-merma-y-categorias/
├── plan.md
├── research.md
├── data-model.md
├── quickstart.md
├── contracts/
│   ├── api.md      # rutas nuevas y ampliadas
│   └── mcp.md      # las tres herramientas y el cambio a categories_list
└── tasks.md        # /speckit-tasks
```

### Source Code (repository root)

```text
server/
├── queries/
│   ├── admin.sql                 # AdminCategoryList (+ is_active, inactivos), lecturas con candado, insert/update
│   └── costing.sql               # lectura de nombres para la vista previa de merma
├── internal/
│   ├── domain/
│   │   ├── category_rules.go(+_test)      # NUEVO: reglas del árbol y del nombre
│   │   ├── expected_waste_impact.go(+_test)  # NUEVO: costo antes/después sobre CostGraph
│   │   ├── costing.go                     # CostGraph.WithIngredientWaste (copia con caché limpia)
│   │   ├── audit.go(+_test)               # category.created, category.updated
│   │   └── apitoken.go                    # catálogo MCP: 3 herramientas, categories_list ampliada, pistas de códigos
│   ├── app/
│   │   ├── categories.go                  # NUEVO: AdminService.CreateCategory / UpdateCategory
│   │   ├── admin.go                       # CategoryList(status)
│   │   └── expected_waste.go              # reason en SetArticleExpectedWaste; ArticleExpectedWastePreview
│   ├── httpapi/
│   │   ├── handlers_categories.go         # NUEVO: POST /admin/categories; PATCH ampliado (se mueve aquí)
│   │   ├── handlers_expected_waste.go     # reason; GET …/expected-waste/preview
│   │   ├── router.go
│   │   ├── mcp_tools_099.go               # NUEVO: las tres herramientas
│   │   └── mcp_refs.go                    # refs de las tres
│   └── integration/
│       ├── categories_099_test.go         # NUEVO: reglas contra Postgres + inTheThreeCases
│       ├── expected_waste_099_test.go     # NUEVO: vista previa y reason + inTheThreeCases
│       └── mcp_099_test.go                # NUEVO: despacho MCP de punta a punta
docs/mecanica/catalogo-y-sucursales.md, docs/mecanica/almacen.md  # lo que no se ve
```

**Structure Decision**: backend únicamente, en las capas existentes de `server/internal`.

## Complexity Tracking

Sin violaciones.
