# Data Model: Gestión de artículos del almacén

## Lo que ya existe y se reutiliza

- `ingredients`: `name`, `category_id`, `base_unit_id`, `count_place_id`, `min_stock`,
  `current_cost`, `cost_source`, `is_active` y `updated_at` (se usa como versión).
- `ingredient_presentations`: `name`, `qty_in_base`, `is_approximate`, `active`.
- `stock_movements`: `movement_type`, `quantity`, `unit_cost`, `stock_count_id`, `expense_item_id`,
  `reason` y `note`.
- `recipe_items`: `ingredient_id`, `quantity`, `unit_id`.

## Nuevo (migración 0096)

### ingredient_cost_corrections

| Columna | Tipo | Regla |
|---|---|---|
| id | bigint identity | pk |
| company_id | bigint | default de la sesión; RLS |
| ingredient_id | bigint | FK compuesta `(ingredient_id, company_id)` a `ingredients`, `on delete restrict` |
| previous_cost | numeric(12,6) | ≥ 0 |
| new_cost | numeric(12,6) | ≥ 0 |
| user_id | bigint | quién |
| created_at | timestamptz | `now()` de la base |

Índice `(company_id, ingredient_id, created_at desc, id desc)` para el historial.

### ingredient_replacements

| Columna | Tipo | Regla |
|---|---|---|
| id | bigint identity | pk |
| company_id | bigint | RLS |
| old_ingredient_id | bigint | FK compuesta, `restrict`, único: un artículo se reemplaza una vez |
| new_ingredient_id | bigint | FK compuesta, `restrict` |
| factor | numeric(14,6) | > 0: cuánto del nuevo vale 1 del viejo, en sus unidades base |
| user_id | bigint | |
| created_at | timestamptz | |

Los dos `ajuste` del traspaso llevan `note = 'reemplazo #<id>'` y `reason = 'reemplazo'`.

Las dos tablas tienen RLS con `nullif`, `select, insert` para `gatobobah_app` y entran a
`TestEveryCompanyTableIsIsolated`.

## Transiciones del artículo

```text
activo ──archivar (sin receta activa, sin conteo abierto)──▶ archivado
archivado ──reactivar──▶ activo
activo ──cambio directo (solo ajustes)──▶ activo, otra unidad base
activo ──reemplazo──▶ archivado (+ uno nuevo activo)
```

## Bitácora (`audit_events`)

| Acción | entity_type | before / after |
|---|---|---|
| ingredient.created | ingredient | — / ficha |
| ingredient.updated | ingredient | campos cambiados |
| ingredient.archived / reactivated | ingredient | is_active |
| ingredient.cost_corrected | ingredient | costo anterior / nuevo |
| ingredient.unit_changed | ingredient | unidad y factor; por cada ajuste, renglón de conteo, receta y empaque convertido: id, valor y unidad originales |
| ingredient.replaced | ingredient | viejo / nuevo, factor, existencias traspasadas |
| presentation.created / updated / retired | presentation | nombre, cantidad, activo |
