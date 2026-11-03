# Data Model: Un insumo con varias marcas (entrega 1)

Migración **0104** (`server/migrations/0104_ingredient_merge.sql`). Todo es agregar: ninguna columna
ni tabla se quita o renombra, así que la API anterior sigue funcionando durante la subida azul/verde
(constitución IV).

## `ingredient_replacements` (0096) — gana `kind`

| Columna | Tipo | Regla |
|---|---|---|
| `kind` | `text not null default 'unit_change'` | `check (kind in ('unit_change', 'merge'))` |

- Un renglón por origen de una unión: `old_ingredient_id` = origen, `new_ingredient_id` = destino,
  `factor` = cuánto del destino vale una unidad base del origen (1 si se miden igual), `user_id`,
  `created_at`.
- `unique (company_id, old_ingredient_id)` ya impide unir dos veces el mismo origen o unir uno ya
  reemplazado por cambio de unidad.
- Sin índice nuevo: `ingredient_replacements_by_new` (0096) sirve para buscar los orígenes de un
  destino, y `unique (company_id, old_ingredient_id)` para saber si un insumo es origen.
- Trigger `check_ingredient_merge` (`before insert`, solo `kind = 'merge'`): rechaza un destino que ya
  es origen de una unión o fue reemplazado por cambio de unidad, un origen o destino archivado y un
  origen o destino preparado. Así no puede existir un ciclo A→B→A aunque un script se equivoque.
- Sigue sin `update` ni `delete` para `gatobobah_app`: una unión es un hecho.

## `ingredient_presentations` (0092) — gana `brand`

| Columna | Tipo | Regla |
|---|---|---|
| `brand` | `text null` | `check (brand is null or char_length(btrim(brand)) between 1 and 40)` |

- El índice único `ingredient_presentations_name_key` se reemplaza por uno con la marca normalizada:
  `(company_id, ingredient_id, norm(name), norm(coalesce(brand, ''))) where active`, con la misma
  normalización (`regexp_replace(translate(lower(btrim(x)), 'áéíóúüñ', 'aeiouun'), '\s+', ' ', 'g')`).
- Se crea el nuevo antes de borrar el viejo, en la misma migración (`lock_timeout = '3s'`).

## `expense_items` — gana `presentation_brand` (snapshot)

| Columna | Tipo | Regla |
|---|---|---|
| `presentation_brand` | `text null` | Copia de la marca de la presentación al momento de la compra |

- La llena un trigger `before insert or update of presentation_id` (`snapshot_presentation_brand`):
  cuando `presentation_id` cambia o llega, copia `brand` de esa presentación. La API anterior no la
  manda y queda llena igual; no se toca `purchases.go`.
- Por qué: la marca es editable. Sin la copia, editar «Caja · Coca» a «Pepsi» reescribe de qué marca
  fue cada compra pasada, y la comparación por marca (entrega 2) mentiría sin forma de reconstruirla.
  Es el patrón snapshot de `order_lines` (principio VIII). Las compras anteriores a la 0104 se
  rellenan en la migración con la marca vigente (nula: hoy no existe ninguna).

## `ingredient_distinct_pairs` (nueva) — «No son lo mismo»

| Columna | Tipo | Regla |
|---|---|---|
| `id` | `bigint generated always as identity primary key` | |
| `company_id` | `bigint not null default nullif(current_setting('app.company_id', true), '')::bigint references companies on delete cascade` | RLS `tenant_isolation` con `nullif`, como toda tabla de empresa |
| `low_ingredient_id` | `bigint not null` | FK compuesta `(low_ingredient_id, company_id) → ingredients (id, company_id)` |
| `high_ingredient_id` | `bigint not null` | FK compuesta igual; `check (low_ingredient_id < high_ingredient_id)`: el par se guarda ordenado |
| `user_id` | `bigint not null` | FK `(company_id, user_id) → users (company_id, id)` |
| `created_at` | `timestamptz not null default now()` | |

- `unique (company_id, low_ingredient_id, high_ingredient_id)`: marcar dos veces es idempotente
  (`on conflict do nothing`). Índice `(company_id, high_ingredient_id)` para la llave del lado alto.
- `grant select, insert on ingredient_distinct_pairs to gatobobah_app` (es un hecho, no se edita).
- Apunta a `ingredients`: entra a `UnitChangeTables` («no guarda cantidades») y a `MergeTables`
  («se queda: el par habla de dos insumos que ya no se proponen; uno archivado no se propone»).
  No lleva la guarda de rechazo: marcar un par con un origen unido no es posible porque los unidos
  no se proponen, y si llegara, no daña nada.
- Respaldo anónimo: sin columnas de texto. Las de texto nuevas en otras tablas
  (`ingredient_replacements.kind`, `ingredient_presentations.brand`, `expense_items.presentation_brand`)
  van a `noPersonalData` como «catálogo», igual que el nombre de la presentación.

## Funciones y triggers

| Pieza | Qué hace |
|---|---|
| `redirect_merged_ingredient_movement()` — `before insert` en `stock_movements` | Primero un `exists` por `(company_id, old_ingredient_id)`; solo si hay unión sigue la cadena (CTE recursivo con tope de profundidad 16) hasta el destino final con el producto de los factores. Si `replacement_id` es nulo: cambia `ingredient_id`, `quantity` × factor redondeado a 4 decimales (si queda en 0, ±0.0001 con el signo original) y `unit_cost` ÷ factor. |
| `reject_movement_on_merged_ingredient()` — `constraint trigger after insert ... deferrable initially deferred` en `stock_movements` | Al confirmar, con foto nueva: si el movimiento quedó en un origen unido sin `replacement_id`, `raise ... using errcode = 'EGB02'`. Atrapa la carrera de R2. |
| `reject_merged_ingredient()` — en `recipe_items`, `expense_items`, `ingredient_presentations`, `supplier_items`, `ingredient_purchase_formats`, `waste_records`, `stock_count_lines`, `ingredient_cost_corrections`, **dos veces por tabla**: `before insert or update of ingredient_id` (el caso normal) y `constraint trigger after insert or update of ingredient_id deferrable initially deferred` (la carrera: la fila esperó en la llave foránea a que la unión confirmara) | Si el insumo nuevo es un origen unido (y en un update, `old.ingredient_id is distinct from new.ingredient_id`): `EGB02` con el nombre del destino en el mensaje. |
| `reject_reactivating_merged_ingredient()` — `before update of is_active` en `ingredients` | Rechaza activar un insumo que es origen de una unión (la API anterior podría reactivarlo durante el traslape). |
| `check_ingredient_merge()` — `before insert` en `ingredient_replacements` | Ver arriba. |
| `snapshot_presentation_brand()` — `before insert or update of presentation_id` en `expense_items` | Ver arriba. |

**Seguridad de las funciones**: solo las funciones de **trigger** son `security definer`, con
`set search_path = pg_catalog, public, pg_temp`, todo calificado, y filtran por la empresa de la fila
(`new.company_id`): así funcionan igual bajo `gatobobah_app`, `gatobobah_webhook` u owner, y una
conexión reciclada con `app.company_id = ''` no las deja ciegas (el caso de la 0074). Postgres no
revisa `EXECUTE` al disparar un trigger, así que se hace `revoke execute ... from public` en todas.
No hay funciones que devuelvan conjuntos llamables desde la app: el historial y la bitácora leen los
orígenes con un CTE recursivo dentro de la consulta de sqlc, sujeto a RLS.

**`store`**: `EGB02` puede nacer en `tx.Commit` (el trigger diferido), donde `translating` no mira.
Los tres `tx.Commit` de `store.go` devuelven `DomainError(tx.Commit(ctx))`, y `DomainError` traduce
`EGB02` → `domain.ErrIngredientMerged` (409 `INGREDIENT_MERGED`).

## Clasificación (`app.MergeTables`)

| Tabla | Qué hace la unión |
|---|---|
| `stock_movements` | El traspaso por sucursal (salida del origen, entrada al destino, `replacement_id`); lo pasado no se toca; lo que llegue después se redirige |
| `stock_levels` | La suma del traspaso; el origen queda en cero |
| `recipe_items` | Pasan al destino (se suman si la receta ya lo lleva); después, rechaza |
| `ingredient_presentations` | Las activas que cuadran pasan al destino con marca; las demás se quedan archivadas en el origen; después, rechaza |
| `ingredient_purchase_formats` | Pasan al destino, convertidas; después, rechaza |
| `supplier_items` | Pasan al destino, convertido el contenido aprendido; después, rechaza |
| `expense_items` | Lo pasado se queda; uno por recibir bloquea; después, rechaza |
| `waste_records` | Lo pasado se queda; una pendiente bloquea; después, rechaza |
| `stock_count_lines` | Lo pasado se queda; un conteo abierto bloquea; después, rechaza |
| `ingredient_cost_corrections` | Las pasadas se quedan y se leen desde el destino; después, rechaza |
| `ingredient_replacements` | Registra la unión |
| `ingredient_distinct_pairs` | Se queda; los archivados no se proponen |

## Estados del insumo

```text
activo ──unir──▶ archivado «unido a <destino>»  (terminal en esta entrega: no se reactiva)
activo ──archivar──▶ archivado ──reactivar──▶ activo   (sin cambio)
```

«Unido» se deriva: archivado y con un `ingredient_replacements` `kind = 'merge'` como origen.

## Vista previa (respuesta de `POST /stock/articles/{id}/merge?preview=1`)

```text
MergePreview
  target        { id, name, baseUnit, costBefore, costAfter }
  sources[]     { id, name, baseUnit, factor }
  stock[]       { branchId, branch, ingredientId, name, before, after }   (destino y cada origen)
  recipes[]     { recipeKind (product|option|ingredient), ownerId, owner, sourceId,
                  before { qty, unit }, after { qty, unit }, merged (bool: se sumó a uno existente) }
  presentations[] { id, name, brand, qtyInBase, action (move|archive|conflict), sourceId }
  supplierItems  int   (cuántos artículos de proveedor se ligan al destino)
  purchaseFormats int
  minStock      { before, after }
  fingerprint   string
```

## Bitácora

| Acción | Entidad | Before / After |
|---|---|---|
| `ingredient.merged` | destino | after: orígenes, factor de cada uno, traspaso por sucursal, recetas movidas, presentaciones movidas/archivadas, costo antes/después |
| `ingredient.merged_into` | cada origen | after: destino, factor, traspaso por sucursal |

Las dos entran a la lista blanca de `domain/audit.go` (grupo de precios, como `ingredient.replaced`)
y a `web/src/shared/audit/labels.ts`.

## Down de la 0104

Se niega si hay alguna unión (`kind = 'merge'`), alguna marca capturada (en presentación o en compra)
o algún par descartado: perdería cuál insumo es sucesor de cuál, o la marca. Su limpieza de prueba va
en `guardedDownCleanup[104]` (`harness_restaurado_test.go`), en este orden: quitar `replacement_id`
de los movimientos de uniones; borrar las uniones; borrar los pares; desactivar las presentaciones
activas que quedarían repetidas por nombre al quitar la marca; poner las marcas en nulo. Sin datos: quita triggers, funciones, el índice nuevo (restaura el viejo) y las tres columnas.
