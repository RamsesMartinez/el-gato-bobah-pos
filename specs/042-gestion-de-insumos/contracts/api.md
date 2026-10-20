# API: Gestión de artículos del almacén

Todas las rutas van bajo `/api/v1` y piden sesión. **V** = `inventory.view` y **C** =
`inventory.configure`.

| Método y ruta | Permiso | Qué hace |
|---|---|---|
| `GET /stock/articles?q=&status=active\|archived&placeId=&noCost=1&negative=1` | V | Lista con existencia, costo, lugar y empaques |
| `GET /stock/articles/{id}` | V | Ficha con `updatedAt` |
| `POST /stock/articles` | C | `{name, baseUnitId, categoryId?, placeId?, minStock?, initialCost?, initialStock?, presentations?[]}` |
| `PATCH /stock/articles/{id}` | C | `{expectedUpdatedAt, name?, categoryId?, placeId?, minStock?}` |
| `POST /stock/articles/{id}/cost` | C | `{cost}`: corrección a mano |
| `GET /stock/articles/{id}/cost-history?before=<cursor>` | V | 20 renglones `{kind: purchase\|correction, at, unitCost, presentation?, supplier?, user?}` y `next` |
| `GET /stock/articles/{id}/audit?before=<cursor>` | V | 20 eventos del artículo y de sus empaques: `{at, actor, action, before, after}` y `next` |
| `GET /stock/articles/{id}/archive-check` | V | `{onHand[], activeRecipes[], openCount, pendingReceipts}` |
| `POST /stock/articles/{id}/archive` · `/reactivate` | C | |
| `POST /stock/articles/{id}/unit/preview` | C | `{baseUnitId, factor}` → `{path: direct\|replace, conversions[]}` |
| `POST /stock/articles/{id}/unit` | C | `{baseUnitId, factor, overrides[]}`: aplica el camino que devolvió la vista previa |
| `GET /stock/presentation-suggestions?unitKind=` | V | `{name, qtyInBase}` de la misma clase de unidad |
| `PUT /stock/counts/{id}/lines/{ingredientId}` (ya existe) | `inventory.count` | Con `confirmable: true` evalúa el aviso y puede responder 409 `NEEDS_CONFIRM` sin guardar; con `confirmed: true` guarda. Sin `confirmable`, guarda como hoy. Puede traer peso crudo y unidad: el servidor convierte a piezas |
| `POST/PUT /expenses`, `PUT …/composition` (ya existen) | los de hoy | Un renglón puede traer `newArticle {name, baseUnitId}` |

**Errores:**
- 409 `NAME_TAKEN` con `{id, archived}`.
- 409 `STALE`: la ficha cambió mientras se veía.
- 409 `IN_ACTIVE_RECIPES`, `OPEN_COUNT`, `PREP_COST` y `UNIT_CHANGE_PENDING` (con las compras por recibir y las mermas pendientes).
- `POST /ingredients` se conserva una subida más para las tabletas con la pantalla anterior.
- 409 `BUSY`, de un `nowait`.
- 409 `BRANCH_AMBIGUOUS`.
- 400 por validación.
