# Data Model: Salud del inventario

**Sin tablas ni columnas nuevas** (research R1). Lo que esta feature lee y escribe:

## Se lee

| Tabla | Para qué |
|---|---|
| `products` | `track_stock`, `recipe_id`, `type`, `composition_status`, `system_kind`, `is_active` → modo de consumo |
| `modifier_options` / `modifier_groups` | `recipe_id`, `linked_product_id`, `composition_status`, activos |
| `recipe_items` / `ingredients` / `units` | renglones de receta: cantidad, tipo de unidad contra el del insumo, insumo archivado o duplicado |
| `order_lines` / `order_line_modifiers` / `orders` | lo vendido por día de negocio y canal (`delivery_platform_id`) |
| `stock_movements` | si una venta descontó (`venta` con `order_line_id`, y `modifier_option_id` para extras); última `compra` por insumo |
| `stock_levels` | existencia por sucursal e insumo |
| `branches` | mostrar la sucursal solo si hay más de una |

## Se escribe (solo al cambiar el modo de consumo)

| Tabla | Qué |
|---|---|
| `products` | `track_stock` (prende o apaga); `recipe_id` y `composition_status` como hoy en `SaveComposition` |
| `stock_movements` | dos `ajuste` ligados al trasladar existencia, o uno al cerrarla; `reason = 'cambio de modo'` y nota con el modo de origen y destino |
| `audit_events` | acción `product.consumption_mode`, `entity_type = 'product'`, `before` y `after` con el modo |

## Tipos de dominio (puros, `server/internal/domain`)

- **`ConsumptionMode`**: `none` (no gasta insumos), `own_stock`, `recipe`, `package`, `undecided`. Se deriva con `ModeOf(product)`.
- **`ModeChange` / `PlanModeChange(from, to, onHand, recipe)`**: devuelve los ajustes de existencia a insertar. Puede ser traslado (un insumo), cierre en cero (varios insumos) o nada. También devuelve si `track_stock` cambia.
- **`HealthFilter{From, To, Limit, Offset, Section}`** con `Validate()`:
  - un rango invertido, de más de 366 días o con fecha mal formada → `ErrValidation`;
  - sin fechas, por omisión los últimos 30 días.
- **`HealthIssueKind`**: `unsold_deduction`, `negative_stock`, `no_purchases`, `malformed_recipe`, `pending_recipe`.

## Reglas que salen de aquí

- Un renglón que no descontó y cuyo producto está confirmado «No gasta insumos» no es problema (FR-004).
- `platform_unpaired` se reporta aparte, nunca como producto del catálogo.
- Un insumo es «duplicado» si otro insumo activo de la misma empresa tiene el mismo nombre, sin distinguir mayúsculas ni acentos.
