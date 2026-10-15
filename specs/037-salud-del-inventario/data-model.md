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
| `products` | `track_stock` (solo se apaga, nunca se prende); `recipe_id` y `composition_status` como hoy en `SaveComposition` |
| `stock_movements` | dos `ajuste` ligados al trasladar existencia, o uno al cerrarla; `reason = 'cambio de modo'` y nota con el modo de origen y destino |
| `audit_events` | acción `product.consumption_mode`, `entity_type = 'product'`, `before` y `after` con el modo |

## Tipos de dominio (puros, `server/internal/domain`)

- **`ConsumptionMode`**: `none` (no gasta insumos), `own_stock` (solo heredado: se lee, nunca se elige), `recipe`, `package`, `undecided`. Se deriva con `ModeOf(product)`. «Se compra hecho» es `recipe` de un insumo por pieza: no es un modo aparte.
- **`ModeChange` / `PlanModeChange(from, to, productOnHand, ingredientOnHand, actualStock, recipe)`**: devuelve los ajustes de existencia a insertar y si `track_stock` se apaga. Solo hay transición con existencia cuando `from = own_stock`:
  - a receta de un insumo **sin** existencia: traslado (el producto a cero, el insumo sube N × cantidad);
  - a receta de un insumo **con** existencia: exige `actualStock` (la cifra que eligió quien configura, FR-014a); el insumo queda en esa cifra y el producto en cero; sin `actualStock` → `ErrValidation`;
  - a receta de varios insumos, combo o «No se cuenta»: cierre del producto en cero, con confirmación previa (FR-014b);
  - cualquier otra transición: ningún ajuste.
  - `productOnHand` e `ingredientOnHand` son los leídos con `FOR UPDATE` dentro de la tx, no los de la pantalla; si difieren de `seen…` de la petición → `ErrConflict`.
- **`HealthFilter{From, To, Limit, Offset, Section}`** con `Validate()`:
  - un rango invertido, de más de 366 días o con fecha mal formada → `ErrValidation`;
  - sin fechas, por omisión los últimos 30 días.
- **`HealthIssueKind`**: `unsold_deduction`, `negative_stock`, `no_purchases`, `malformed_recipe`, `pending_recipe`.

## Reglas que salen de aquí

- Corregir una receta no toca movimientos anteriores (FR-002a): ninguna consulta ni servicio de esta feature escribe `stock_movements` de tipo `venta`.
- Un combo se señala (FR-002b) si algún hueco activo tiene más de un producto.
- «Lo guardado en negativo» abre resumido si la empresa no tiene ningún movimiento `compra` (FR-005).
- Un renglón que no descontó y cuyo producto está confirmado «No gasta insumos» no es problema (FR-004).
- `platform_unpaired` se reporta aparte, nunca como producto del catálogo.
- Un insumo es «duplicado» si otro insumo activo de la misma empresa tiene el mismo nombre, sin distinguir mayúsculas ni acentos.
