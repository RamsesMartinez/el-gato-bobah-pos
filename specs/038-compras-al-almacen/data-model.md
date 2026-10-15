# Data model: Compras al almacén

Dos migraciones, con su test de integración (hook `migracion-con-test`). Todo identificador nuevo en
inglés.

- **`0091_unit_kind_length.sql`** — solo `alter type unit_kind add value 'longitud'`. Postgres no
  deja **usar** un valor de enum en la misma transacción que lo agrega, y goose corre cada migración
  en una transacción (regla ya escrita en `0018_order_refund.sql:5`). Su Down no puede quitar el
  valor: lo deja y lo dice.
- **`0092_purchases_into_stock.sql`** — todo lo demás, en una transacción: las unidades `cm` y `m`,
  los índices de llave por empresa, las tablas y columnas nuevas, checks, trigger, RLS y grants. Su
  Down se niega si hay conteos, presentaciones usadas o movimientos con `expense_item_id`, como la
  0089.

## Llaves por empresa que faltan (al inicio de la 0092)

El patrón de FK compuesta de la 0041 necesita un único exacto en el destino, y hoy no existe en
estas dos tablas:

- `create unique index ingredients_tenant_key on ingredients (company_id, id);`
- `create unique index expense_items_tenant_key on expense_items (company_id, id);`

Toda referencia nueva entre tablas de empresa es **compuesta con `company_id`**: una escritura como
owner (un arreglo de datos, la importación) no puede colgar una fila de la empresa A en una de la B.

## Tablas nuevas

### `ingredient_presentations`

| Columna | Tipo | Regla |
|---|---|---|
| `id` | bigint identity | único `(company_id, id)` para las FK que llegan |
| `company_id` | bigint not null | default del GUC, FK a `companies` on delete cascade; RLS con `nullif` |
| `ingredient_id` | bigint not null | FK compuesta `(company_id, ingredient_id)` a `ingredients`, on delete cascade |
| `name` | text not null | `check (char_length(name) between 1 and 40)`; único por insumo, normalizado (abajo) |
| `qty_in_base` | numeric(14,4) not null | `> 0`, en la unidad base del insumo; entero si el insumo es por pieza (trigger) |
| `active` | bool not null default true | una presentación se desactiva, no se borra |
| `purchase_unit_id` | smallint null | FK a `units`. Nulo: el empaque es una pieza de compra. Con valor: empaque aproximado de otro tipo («por kilo») |
| `is_approximate` | bool not null default false | verdadero si y solo si `purchase_unit_id` es de otro tipo que la base del insumo (trigger) |
| `created_at` | timestamptz | |

- Único: `(company_id, ingredient_id, <nombre normalizado>)` donde `active`, con la normalización
  de `IngredientNameTaken` (`translate(lower(trim(name)), 'áéíóúüñ', 'aeiouun')` y espacios
  colapsados), sin `unaccent`, que no está instalada.
- **Trigger `ingredient_presentations_before_write`** (no `check`: compara con `units` y con la base
  del insumo, otras tablas): exige `qty_in_base` entero si la base es pieza, `is_approximate` igual a
  «`purchase_unit_id` de otro tipo», y rechaza un nombre igual a una unidad (`units.name`/`code`)
  salvo en un empaque aproximado. Pruebas en el test de la migración.

### `stock_counts`

| Columna | Tipo | Regla |
|---|---|---|
| `id` | bigint identity | único `(company_id, id)` |
| `company_id` | bigint not null | RLS |
| `branch_id` | bigint not null | FK compuesta a `branches`; se fija al abrir con `current_branch_id()` |
| `status` | text not null | `check in ('open','closed','discarded')` |
| `opened_by` / `opened_at` | | |
| `closed_by` / `closed_at` | nulos mientras `open` | check: llenos si y solo si `status <> 'open'` |
| `previewed_at` | timestamptz null | La primera vez que se pidió la vista previa del cierre (que ya dice cuánto sobraba o faltaba). El cierre deja en la bitácora cuántas cifras se capturaron después (revisión de código) |

Único parcial `(company_id, branch_id)` donde `status = 'open'`: un conteo abierto por sucursal.

### `stock_count_lines`

| Columna | Tipo | Regla |
|---|---|---|
| `count_id` | bigint not null | **FK compuesta** `(company_id, count_id)` a `stock_counts`, on delete cascade |
| `company_id` | bigint not null | RLS |
| `ingredient_id` | bigint not null | FK compuesta a `ingredients` |
| `counted_qty` | numeric(14,4) not null | `>= 0` (0 = «no hay»; sin renglón = «no se contó») |
| `system_qty` | numeric(14,4) not null | lo guardado en la sucursal del conteo al capturar |
| `captured_by` / `captured_at` | | se actualizan al recapturar |

Llave primaria `(count_id, ingredient_id)`.

## Tipos y catálogo

- `unit_kind` gana `longitud` (0091).
- `units` gana `cm` (`longitud`, `to_base 1`) y `m` (`longitud`, `to_base 100`) (0092). El
  catálogo de unidades es global, como hoy.

## Columnas nuevas

| Tabla | Columna | Por qué |
|---|---|---|
| `stock_movements` | `expense_item_id bigint null` | FK compuesta a `expense_items`, on delete restrict. Cada entrada, corrección y reversión de compra apunta a su renglón (R2) |
| `stock_movements` | `stock_count_id bigint null` | FK compuesta a `stock_counts`, on delete restrict (SC-003) |
| `expense_items` | `presentation_id bigint null` | FK compuesta a `ingredient_presentations`, **on delete set null**: la equivalencia vive copiada en `pack_qty_in_base` |
| `expense_items` | `applied_qty_base numeric(14,4) not null default 0` | Lo ya sumado al almacén, en unidad base. Se actualiza en la **misma transacción** que cada movimiento del renglón; un test compara con la suma de sus movimientos |
| `expense_items` | `not_coming_qty numeric(14,4) not null default 0` | Lo que «ya no va a llegar», en la unidad del renglón; puede ser una parte (R2b) |
| `expense_items` | `voided_at timestamptz null` | Renglón quitado en una corrección; sus movimientos se conservan |
| `expense_items` | `is_gift bool not null default false` | Regalo del proveedor (FR-007d) |
| `ingredients` | `count_place text null` | «Lugar» para ordenar el conteo; `check (char_length(count_place) between 1 and 30)`. No es un almacén (puerta del principio VIII, issue #65) |
| `business_settings` | `costing_method text not null default 'last'` | `check in ('last','weighted')` (R4) |
| `business_settings` | `stock_item_label text not null default 'artículo'` | `check (char_length(stock_item_label) between 1 and 20)` (R11) |

Checks que cambian en `expense_items`:
- `check (amount > 0 or is_gift)` y `check (not is_gift or amount = 0)` (FR-004a, FR-007d). La 0028
  tenía `amount >= 0`: el test de la migración cuenta los renglones existentes con importe 0 sobre
  la copia de producción; si los hay, la migración no los reescribe y el check entra `not valid`
  (aplica solo a lo nuevo).
- `check (not_coming_qty >= 0 and not_coming_qty <= quantity)`.
- `check (coalesce(qty_received, 0) + not_coming_qty <= quantity) not valid`: lo de más sube lo
  pedido con su importe, nunca queda recibido > pedido (dividía el costo entre lo que no entró).
  `not valid` como el de arriba; la copia de producción no tenía ninguno que lo violara.

Llaves que cambian (revisión de código del 038): `expense_items.ingredient_id`,
`stock_movements.ingredient_id` y `stock_levels.ingredient_id` pasan de la llave simple de la 0008
y la 0028 a la compuesta `(ingredient_id, company_id)`, igual que las de producto en la 0041. Sin
esto una compra de una empresa podía apuntar al insumo de otra: la llave foránea salta RLS.

Índices:
- `stock_movements (company_id, expense_item_id)` y `(company_id, stock_count_id)` donde no son
  nulos.
- **Pendientes por recibir**: `expense_items (company_id, expense_id)` donde `voided_at is null` —
  plano: «pendiente» compara `applied_qty_base` con la cantidad pedida convertida por su empaque, y
  la condición de un índice parcial no puede convertir unidades. La consulta filtra pendiente.

## Reglas de estado

- **Renglón:** pedido en base = `quantity` convertido con su empaque. **Pendiente** = pedido −
  `applied_qty_base` − `not_coming_qty` convertido, mayor que 0 y sin `voided_at`. Toda escritura
  inserta un movimiento por la diferencia y actualiza `applied_qty_base` en la misma transacción.
- **Gasto:** `received_at` es la fecha en que ya no quedó nada pendiente, y solo con al menos un
  renglón de almacén (R1). **Vuelve a nulo** si después se le agrega un renglón «Llega después» o se
  corrige uno por debajo de lo pedido sin marcar «ya no va a llegar». Los gastos heredados
  (recibidos sin renglones) quedan inertes.
- **Conteo:** `open` acepta capturas; `closed` inserta un `ajuste` por renglón con
  `counted_qty − system_qty` y no acepta más; `discarded` no toca el almacén.

## Movimientos: sucursal y orden

- **La sucursal se pasa explícita** en los movimientos de recibir (la de la sesión, resuelta con
  `current_branch_id()` una vez por petición) y de conteo (`stock_counts.branch_id`). El trigger
  genérico (`fill_stock_movement_branch_id`, 0078) solo la resuelve por pedido y, con dos
  sucursales, truena con `BRANCH_AMBIGUOUS` aunque el conteo ya la sepa.
- **Orden de inserción por `ingredient_id` ascendente** al corregir, recibir varios renglones y
  cerrar un conteo. El disparador de existencias retiene el bloqueo de `stock_levels` hasta el
  commit; dos escrituras que tocan los mismos insumos en distinto orden se bloquearían entre sí. El
  orden fijo lo evita entre ellas; contra una venta, que bloquea en el orden de su ticket, el
  bloqueo explícito del insumo para el costo (R4) usa `NOWAIT` como en el 037 y responde 409.

## Lo que no se copia (anotado, no construido)

- **El nombre del empaque** no se copia al renglón: si se renombra «Caja», una compra vieja muestra
  el nombre nuevo con el contenido viejo. Mismo caso que la categoría en `order_lines` (constitución,
  «El snapshot…»). Renombrar es raro; se construye si deja de serlo.

## RLS y grants

Las tres tablas nuevas: RLS encendido, política con `nullif(current_setting('app.company_id'))`,
`select, insert, update` para `gatobobah_app` (sin `delete`). Las vigila
`TestEveryCompanyTableIsIsolated`; cada consulta nueva lleva su test bajo `appRoleStore` con
`inTheThreeCases`.
