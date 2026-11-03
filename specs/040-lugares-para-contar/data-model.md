# Data Model: Lugares para contar

## count_places (nueva, 0095)

| Columna | Tipo | Regla |
|---|---|---|
| id | bigint identity | PK |
| company_id | bigint | default de `app.company_id`; FK a `companies` `on delete cascade` |
| name | text | 1–30 letras tras `btrim` |
| position | int | ordinal del recorrido entre los activos, 0..n-1; ≥ 0 |
| archived_at | timestamptz | nulo = se ofrece |
| created_at | timestamptz | default `now()` |

- `unique (id, company_id)` para la liga compuesta.
- Índice único parcial `(company_id, regexp_replace(translate(lower(btrim(name)), 'áéíóúüñ', 'aeiouun'), '\s+', ' ', 'g')) where archived_at is null`, la misma expresión que `waste_reasons_name_key`.
- RLS con la política de empresa (`nullif`), `select, insert, update` para `gatobobah_app`, sin `delete`.
- Sin `created_by` ni historia del orden: ningún requisito los pide y se pueden agregar después al
  mismo costo.

## ingredients (cambia)

- **Nueva**: `count_place_id bigint`, FK `(count_place_id, company_id) → count_places (id, company_id)`
  `no action`, como el resto de las ligas entre tablas de una misma empresa (el borrado de la empresa
  ya cae por cascada en las dos).
- **Índice**: `(count_place_id) where count_place_id is not null`, para contar artículos por lugar y
  moverlos al archivar.
- **Se quita**: `count_place text`. La Down la recrea con su `check` de la 0092.

## Paso de los datos (Up)

1. Por empresa, agrupar `count_place` no nulo por la **misma expresión normalizada del índice**.
2. Un lugar por grupo. El nombre es el texto exacto más usado del grupo, con `btrim` y los espacios
   internos colapsados; en un empate gana `order by count desc, name collate "C"`. `position` va en
   orden alfabético.
3. `count_place_id` se liga por `(company_id, expresión normalizada)`, no por el nombre.
4. Quitar `count_place`.

## Down

Recrea `count_place text check (char_length(btrim(count_place)) between 1 and 30)`, la llena con el
nombre del lugar del artículo y después borra `count_places`. Se pierden el orden, los lugares sin
artículos y los archivados.

## Transiciones

- **Activo → archivado**: dentro de una transacción con `pg_advisory_xact_lock` de la empresa y el
  lugar tomado `for update`. El destino de sus artículos, activos o no, es otro lugar activo de la
  empresa (`for share`) o ninguno. Después se renumeran los activos.
- **No hay desarchivar** en esta feature: se crea de nuevo con el mismo nombre, porque el índice es
  parcial.
