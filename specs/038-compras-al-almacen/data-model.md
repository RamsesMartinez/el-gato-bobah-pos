# Data model: Compras al almacén

Una migración, `0091_purchases_into_stock.sql`, con su test de integración (hook
`migracion-con-test`). Todo identificador nuevo en inglés.

## Tablas nuevas

### `ingredient_presentations`

| Columna | Tipo | Regla |
|---|---|---|
| `id` | bigint identity | |
| `company_id` | bigint not null | default del GUC, FK a `companies` on delete cascade; RLS con `nullif` |
| `ingredient_id` | bigint not null | FK compuesta `(company_id, ingredient_id)` a `ingredients` (patrón 0041), on delete cascade |
| `name` | text not null | 1–40 caracteres; único por insumo sin distinguir mayúsculas ni acentos |
| `qty_in_base` | numeric(14,4) not null | `> 0`, en la unidad base del insumo; entero si el insumo es por pieza |
| `active` | bool not null default true | una presentación se desactiva, no se borra si ya se usó |
| `purchase_unit_id` | smallint null | FK a `units`. Nulo: el empaque es una pieza de compra. Con valor: empaque aproximado de otro tipo («por kilo») |
| `is_approximate` | bool not null default false | check: verdadero si y solo si `purchase_unit_id` es de otro tipo que la base del insumo |
| `created_at` | timestamptz | |

Índice único: `(company_id, ingredient_id, <nombre normalizado>)` donde `active`, con la misma
normalización que `IngredientNameTaken` (`translate(lower(trim(name)), 'áéíóúüñ', 'aeiouun')` y
espacios colapsados); sin la extensión `unaccent`, que no está instalada.

### `stock_counts`

| Columna | Tipo | Regla |
|---|---|---|
| `id` | bigint identity | |
| `company_id` | bigint not null | RLS |
| `branch_id` | bigint not null | FK compuesta a `branches`; se fija al abrir con `current_branch_id()` |
| `status` | text not null | `open` → `closed` o `discarded`; check |
| `opened_by` / `opened_at` | | |
| `closed_by` / `closed_at` | nulos mientras `open` | check: llenos si y solo si `status <> 'open'` |

Índice único parcial: `(company_id, branch_id)` donde `status = 'open'` — un conteo abierto por
sucursal.

### `stock_count_lines`

| Columna | Tipo | Regla |
|---|---|---|
| `count_id` | bigint not null | FK on delete cascade |
| `company_id` | bigint not null | RLS |
| `ingredient_id` | bigint not null | FK compuesta a `ingredients` |
| `counted_qty` | numeric(14,4) not null | `>= 0` (0 = «no hay»; sin renglón = «no se contó») |
| `system_qty` | numeric(14,4) not null | lo guardado en la sucursal al capturar |
| `captured_by` / `captured_at` | | se actualizan al recapturar |

Llave primaria `(count_id, ingredient_id)`.

## Tipos y catálogo

- `unit_kind` gana el valor `longitud` (R10).
- `units` gana `cm` (`longitud`, `to_base 1`) y `m` (`longitud`, `to_base 100`). El catálogo de
  unidades es global, no por empresa, como hoy.

## Columnas nuevas

| Tabla | Columna | Por qué |
|---|---|---|
| `stock_movements` | `expense_item_id bigint null` | FK compuesta a `expense_items`, on delete restrict. Cada entrada y corrección de compra apunta a su renglón (R2) |
| `stock_movements` | `stock_count_id bigint null` | FK compuesta a `stock_counts`, on delete restrict. Cada ajuste de conteo apunta a su conteo (SC-003) |
| `expense_items` | `presentation_id bigint null` | Referencia a la presentación elegida; la equivalencia vive copiada en `pack_qty_in_base` |
| `expense_items` | `voided_at timestamptz null` | Renglón quitado en una corrección; sus movimientos se conservan |
| `expense_items` | `closed_at timestamptz null` | «Ya no va a llegar»: deja de estar pendiente sin sumar (R2b) |
| `expense_items` | `closed_by bigint null` | Quién lo cerró |
| `expense_items` | `is_gift bool not null default false` | Regalo del proveedor: importe 0 permitido y no mueve el costo (FR-007d) |
| `business_settings` | `costing_method text not null default 'last'` | check `in ('last','weighted')` (R4) |
| `business_settings` | `stock_item_label text not null default 'artículo'` | 1–20 caracteres (R11) |

Índices: `stock_movements (company_id, expense_item_id)` donde no es nulo, para sumar lo aplicado
por renglón; `stock_movements (company_id, stock_count_id)` donde no es nulo.

## Reglas de estado

- **Renglón de compra:** aplicado = suma de sus movimientos `compra`. Deseado = `qty_received` (o
  `quantity` si llegó completo) convertido a base; quitado (`voided_at`) = 0. Toda escritura inserta
  `deseado − aplicado`.
- **Renglón pendiente:** aplicado < pedido y sin `closed_at`. Recibir una parte inserta un `compra`
  por la parte. Índice parcial `expense_items (company_id, expense_id)` donde pendiente, para la
  lista «Por recibir».
- **Gasto:** `received_at` es la fecha en que ya no quedó nada pendiente (R2b); solo con al menos
  un renglón de almacén (R1). Anular uno recibido exige que todo lo aplicado sea 0.
- **Conteo:** `open` acepta capturas; `closed` inserta un `ajuste` por renglón con
  `counted_qty − system_qty` y no acepta más; `discarded` no toca el almacén.

## RLS y grants

Las tres tablas nuevas: RLS encendido, política con `nullif(current_setting('app.company_id'))`,
`select, insert, update` para `gatobobah_app` (sin `delete`). Las vigila
`TestEveryCompanyTableIsIsolated`.
