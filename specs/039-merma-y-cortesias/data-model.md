# Data Model: Merma y faltante sin explicar

Una migración, `0094_waste.sql`, con su test de integración sobre respaldo restaurado con dos
empresas. Tablas con `company_id`, RLS con `nullif`, `select/insert/update` para `gatobobah_app` y
llaves compuestas `(id, company_id)`. Nada se borra.

## waste_reasons

| Columna | Tipo | Regla |
|---|---|---|
| id | bigint identity | |
| company_id | bigint | default de la sesión; FK a companies |
| name | text | 1–40, único por empresa normalizado (en `domain`, con candado) |
| description | text | 1–200, obligatoria |
| is_staff_meal | boolean | default false; sale aparte en el reporte |
| archived_at | timestamptz null | archivado: no se ofrece |
| merged_into_id | bigint null | FK compuesta a otro motivo de la misma empresa; al juntar |
| created_by, created_at | | |

Unique `(id, company_id)` para llaves compuestas.

## waste_records

| Columna | Tipo | Regla |
|---|---|---|
| id | bigint identity | |
| company_id, branch_id | bigint | sucursal resuelta por `current_branch_id()` |
| reason_id | bigint | FK compuesta a `waste_reasons` |
| item_type | `stock_item_type` | `ingrediente` o `producto` |
| ingredient_id / product_id | bigint | uno de los dos (check), FK compuestas |
| quantity | numeric(14,4) | > 0; unidad base del insumo o piezas |
| entered_qty, presentation_id | | lo que se tecleó y el empaque (snapshot para mostrar) |
| unit_cost | numeric(12,6) | costo de ese momento; null si no tenía |
| total_cost | numeric(12,2) | null si no hay costo |
| note | text null | ≤ 200 |
| status | text | `applied`, `pending`, `rejected`, `covered`, `undone` |
| recorded_by, recorded_at | | |
| resolved_by, resolved_at | null | aprobó, rechazó o deshizo |
| covered_by_count_id | bigint null | FK compuesta a `stock_counts` |

Índices: `(company_id, branch_id, status, recorded_at desc)` para «por aprobar» y el día;
`(company_id, reason_id, recorded_at)` para el reporte por motivo.

### Transiciones (en `domain`)

```
applied ──deshacer (mismo día, insumo sin contar después)──▶ undone
pending ──aprobar──▶ applied        pending ──rechazar──▶ rejected
pending ──deshacer──▶ undone        pending ──cierre de conteo que contó su insumo──▶ covered
```

Ninguna otra; la que no está responde 409.

## stock_movements

- `waste_record_id bigint null`, FK compuesta a `waste_records`. Registrar o aprobar inserta
  movimientos `merma` negativos; deshacer, los inversos con el mismo `waste_record_id`. La sucursal
  del movimiento es la del registro.

## business_settings

- `waste_approval_threshold numeric(12,2) null` (null = sin aprobación; > 0 con `ValidMoney`).

## Reporte del faltante (calculado)

Por cada renglón de un conteo cerrado con renglón anterior del mismo insumo:
`registered = −Σ quantity` de movimientos `merma` con `waste_record_id` del insumo y sucursal, con
`created_at` en `(prev.captured_at, this.captured_at]`; `unexplained = system_qty − counted_qty`;
`lost = registered + max(unexplained, 0)`. Pesos con `ingredients.current_cost` de hoy.
