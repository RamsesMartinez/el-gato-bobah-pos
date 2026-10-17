# Data Model: Merma y faltante sin explicar

Una migración, `0094_waste.sql`, con su test de integración sobre respaldo restaurado con dos
empresas. Ajustada tras `/revision-de-arquitectura` (2026-10-15).

Las dos tablas nuevas siguen el patrón de 0092 §9:
- `company_id` con el default de la sesión y FK `on delete cascade`;
- RLS forzado con la política `tenant_isolation` (`nullif(current_setting('app.company_id', true), '')`);
- grants `select, insert, update` a `gatobobah_app`, sin `delete`;
- `unique (id, company_id)` para las llaves compuestas.

Nada se borra.

## waste_reasons

| Columna | Tipo | Regla |
|---|---|---|
| id | bigint identity | |
| company_id | bigint | |
| name | text | `check (char_length(btrim(name)) between 1 and 40)` |
| description | text | `check (char_length(btrim(description)) between 1 and 200)` |
| is_staff_meal | boolean | default false |
| archived_at | timestamptz null | |
| merged_into_id | bigint null | FK `(merged_into_id, company_id)` → `waste_reasons`, no action; un solo nivel (el destino no puede estar juntado ni archivado, lo valida `domain`) |
| created_by | bigint **null** | FK `(company_id, created_by)` → `users`; null en los sembrados por la migración |
| created_at | timestamptz | |

Unicidad del nombre en la base, como `ingredient_presentations_name_key` de 0092:
`create unique index waste_reasons_name_key on waste_reasons (company_id,
regexp_replace(translate(lower(btrim(name)),'áéíóúüñ','aeiouun'),'\s+',' ','g')) where archived_at is
null and merged_into_id is null`. El 23505 se traduce a 409 con el id existente.

## waste_records

| Columna | Tipo | Regla |
|---|---|---|
| id | bigint identity | `unique (id, company_id)` y `unique (id, company_id, branch_id)` |
| company_id | bigint | |
| branch_id | bigint | FK `(company_id, branch_id)` → `branches`, como `stock_counts_branch_fkey`; lo resuelve el servicio con `current_branch_id()` |
| reason_id | bigint | FK compuesta a `waste_reasons`, no action. **Nunca cambia**: juntar motivos no lo toca |
| item_type | `stock_item_type` | `check ((item_type='ingrediente') = (ingredient_id is not null))` y su gemelo de producto |
| ingredient_id / product_id | bigint null | FK compuestas, no action |
| quantity | numeric(14,4) | `check (quantity > 0)`; unidad base del insumo o piezas |
| entered_qty | numeric(14,4) | `check (entered_qty > 0)`; lo tecleado |
| presentation_id | bigint null | FK `on delete set null (presentation_id)`, como `expense_items_presentation_fkey` |
| unit_cost | numeric(12,6) null | |
| total_cost | numeric(12,2) null | `check ((unit_cost is null) = (total_cost is null))`; validado con `ValidMoney` antes del insert (tope `MaxMoney`) |
| is_staff_meal | boolean | copia de la marca del motivo al registrar: cambiar la marca del motivo no reclasifica el pasado |
| note | text null | `check (char_length(note) <= 200)` |
| status | text | `check (status in ('applied','pending','rejected','covered','undone'))` |
| recorded_by, recorded_at | | `recorded_at default clock_timestamp()` |
| resolved_by, resolved_at | null | `check ((status in ('applied','pending')) or resolved_at is not null)`; en `applied` que vino de aprobar, también lleno |
| covered_by_count_id | bigint null | FK compuesta a `stock_counts`, no action; `check ((status='covered') = (covered_by_count_id is not null))` |

Índices:
- `(company_id, branch_id, recorded_at desc)` para el día y la pestaña de los registros.
- Parcial `(company_id, branch_id) where status = 'pending'` para la franja.
- `(company_id, reason_id)` para juntar y para el reporte por motivo.

### Transiciones (en `domain`)

```
applied ──deshacer (mismo día, insumo sin contar después)──▶ undone
pending ──aprobar──▶ applied        pending ──rechazar──▶ rejected
pending ──deshacer──▶ undone        pending ──cierre de conteo (ver abajo)──▶ covered
```

Ninguna otra transición existe; la que no está responde 409.
- **Aprobar** fija `unit_cost` y `total_cost` con el costo vigente si el registro no tenía.
- **Cubrir** solo pasa si el registro se hizo antes de que ese conteo capturara alguno de sus insumos, en la misma sucursal: `recorded_at <= stock_count_lines.captured_at`.

## stock_movements

- `waste_record_id bigint null`, con FK `(waste_record_id, company_id, branch_id)` → `waste_records`
  `(id, company_id, branch_id)`. Es **no action, no restrict**: borrar una empresa cae por cascade, y
  `no action` se revisa al final de la sentencia. Un movimiento de una sucursal no puede colgar de
  un registro de otra.
- Los movimientos de merma (registrar, aprobar, deshacer) se insertan con `branch_id` explícito (el
  del registro) y con `created_at = clock_timestamp()`. El `unit_cost` es el del registro.
- Índice parcial `(company_id, branch_id, ingredient_id, id) where waste_record_id is not null`.
  Sirve al faltante y revisa la FK.

## stock_count_lines (columnas nuevas)

- `last_movement_id bigint null`: el `max(stock_movements.id)` del insumo en la sucursal, leído
  dentro de la captura con el insumo tomado (`lockIngredients`). **El periodo del faltante se ancla
  por id de movimiento, no por hora**: una merma que confirmó después de la captura queda fuera
  aunque haya empezado antes.
- `unit_cost numeric(12,6) null`: el costo del insumo al cerrar el conteo. Los conteos viejos quedan
  en null y el reporte dice «sin costo guardado».

Si el `explain` sobre el respaldo lo pide: índice `(company_id, ingredient_id, captured_at desc)`
para encontrar el renglón anterior.

## business_settings

- `waste_approval_threshold numeric(12,2) null`, con `check (waste_approval_threshold > 0)`. Null
  significa sin aprobación.

## Siembra

- **Migración**: para cada empresa existente, como owner y con `company_id` explícito, inserta los
  cinco motivos genéricos si el nombre normalizado no existe. `is_staff_meal` va en «consumo del
  personal».
- **`CreateCompany`**: siembra los mismos en un CTE de la misma consulta.

## Down

- Se niega si hay cualquier `waste_records` o algún `stock_movements.waste_record_id`.
- Orden: columnas de `stock_count_lines`, columna de `stock_movements` (con su FK e índice),
  `waste_records`, `waste_reasons` y la columna de `business_settings`.
- Probado con tablas llenas y vacías.

## Reporte del faltante (calculado)

Por cada renglón de un conteo cerrado que tenga un renglón anterior del mismo insumo y sucursal:
- `registered` = −Σ `quantity` de los movimientos con `waste_record_id`, del insumo y la sucursal,
  con `id` en `(prev.last_movement_id, this.last_movement_id]`.
- `unexplained` = `system_qty − counted_qty`.
- `lost` = `registered + max(unexplained, 0)`.

Los pesos usan el `unit_cost` del renglón (costo al cerrar). La merma por motivo usa el costo del
registro y agrupa por la fecha en que restó (la del movimiento), igual que el faltante.
