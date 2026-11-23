# Data model: Varios almacenes por sucursal

Decisiones y su porqué en [research.md](research.md). Identificadores en inglés (constitución VII).

## warehouses (0117) — nueva

| Columna | Tipo | Regla |
| --- | --- | --- |
| id | bigint identity PK | |
| company_id | bigint not null | default de sesión, FK `companies`, RLS con `nullif` |
| branch_id | bigint not null | FK `(company_id, branch_id) → branches(company_id, id)` |
| name | text not null | 1–40 caracteres tras normalizar; único por sucursal entre activos con `catalog_name_key(name)` |
| is_sales | boolean not null default false | único parcial `(company_id, branch_id) where is_sales` |
| is_active | boolean not null default true | |
| created_at, updated_at | timestamptz | |

- `unique (company_id, id)` y `unique (company_id, branch_id, id)`: destinos de las llaves compuestas.
- Trigger `warehouses_before_update`: el de venta no se archiva (`is_sales and not is_active` →
  23514); quitarle `is_sales` a uno solo pasa dentro de la operación que se lo da a otro (la
  validación del «exactamente uno» la hace el índice único más la comprobación al final del servicio).
- Grants a `gatobobah_app`: select, insert, update. Sin delete.
- `name` va en `noPersonalData` del respaldo anónimo (como `branches.name` y `count_places.name`).
- Relleno: una fila «Local», `is_sales = true`, por sucursal existente.
- `CreateCompany` crea «Local» en la misma consulta que la matriz.

**Funciones**
- `sales_warehouse_of(p_company bigint, p_branch bigint) returns bigint` — el de venta; `EGB03`
  (`warehouse_missing`) si no hay. `security invoker`, `stable`.
- `fill_warehouse_id()` — trigger `BEFORE INSERT`: si `new.warehouse_id is null`, lo pone con
  `sales_warehouse_of(new.company_id, new.branch_id)`. Para tablas sin `branch_id`
  (`expense_items`, `count_places`) la variante usa la matriz de la empresa (nunca `branch_for_company`,
  que truena con dos sucursales); en `expense_items`, todo renglón con `item_type`.
- `fill_stock_movement_branch_id()` (de la 0078) gana una rama: si `branch_id` es nulo y
  `warehouse_id` no, toma la sucursal del almacén.

## Columna `warehouse_id` (0117, nullable; `not null` en 0118)

| Tabla | Llave compuesta | Relleno | Trigger de llenado | `not null` en 0118 |
| --- | --- | --- | --- | --- |
| stock_levels | `(company_id, branch_id, warehouse_id)` | de venta de su sucursal | sí | sí |
| stock_movements | `(company_id, branch_id, warehouse_id)` | ídem | sí (nombre ordena después del de sucursal) | sí |
| stock_counts | ídem | ídem | sí | sí |
| waste_records | ídem | ídem | sí | sí |
| branch_stock_minimums | ídem | ídem | sí | sí |
| notifications | ídem | ídem | sí | sí |
| stock_out_reports | ídem | ídem | sí | sí |
| expense_items | `(company_id, warehouse_id)` | de venta de la sucursal de sus movimientos; si no tiene, de la matriz explícita | sí, todo renglón con `item_type` (excluido o no) | check `item_type is null or warehouse_id is not null` |
| count_places | `(company_id, warehouse_id)` | de venta de la matriz | sí | sí |

`stock_count_lines` y `stock_count_product_lines` heredan del conteo (research R4).

**Llaves únicas** (research R2)

| Tabla | 0117 agrega | 0118 borra |
| --- | --- | --- |
| stock_levels | `(warehouse_id, ingredient_id)`, `(warehouse_id, product_id)` | `stock_levels_ingredient_per_branch`, `stock_levels_product_per_branch` |
| branch_stock_minimums | `(warehouse_id, ingredient_id)` y `(warehouse_id, product_id)` parciales | `…_ingredient_key`, `…_product_key` |
| notifications | `notifications_one_open_per_warehouse (company_id, warehouse_id, item_key, kind) where …` | `notifications_one_open` |
| stock_counts | `stock_counts_one_open_per_warehouse (company_id, warehouse_id) where status = 'open'` | `stock_counts_one_open` |

`apply_stock_movement()` pasa a `on conflict (warehouse_id, ingredient_id|product_id)` en la 0117.
`low_stock_state` mide el almacén de venta y `evaluate_low_stock` hace `on conflict` sobre
`notifications_one_open_per_warehouse` desde la 0117 (R12); en E5 los dos pasan a por almacén.

**Índices**: `stock_movements (company_id, warehouse_id, id desc)` para Movimientos por almacén
(empieza por `company_id`, AGENTS.md).

## stock_transfers (migración de traspasos; el `ADD VALUE` va en la anterior, R11 H3) — nueva

| Columna | Tipo | Regla |
| --- | --- | --- |
| id | bigint identity PK | |
| company_id, branch_id | bigint not null | FK a sucursal |
| from_warehouse_id, to_warehouse_id | bigint not null | FK `(company_id, branch_id, x) → warehouses`; `check (from <> to)` |
| note | text | ≤ 200; se borra en el respaldo anónimo |
| client_key | uuid not null | `unique (company_id, client_key)` |
| created_by | bigint not null | FK usuarios de la empresa |
| created_at | timestamptz default now() | hora de la base |

`stock_transfer_lines`: `(transfer_id, line_no)` PK, `item_type`, `ingredient_id | product_id`,
`qty_base numeric(14,4) > 0`, `presentation_id` nullable, `presentation_qty` nullable. Va en
`app.MergeTables` (se redirige como los movimientos: guarda el hecho histórico, «conserva») y en
`app.UnitChangeTables` (se reescribe con el cambio directo, como los ajustes).

`stock_movements.transfer_id` (nullable) con FK `(transfer_id, company_id, branch_id) → stock_transfers`; las dos patas guardan `unit_cost` (costo vigente).
`stock_movement_type` gana `traspaso`.

Sin update ni delete para `gatobobah_app` en las dos tablas: un traspaso no se edita.

## Permisos (0117)

| id | code | área | Sugeridos que lo heredan |
| --- | --- | --- | --- |
| siguiente libre | inventory.warehouse_manage | inventory | admin (1), gerente (2) |
| siguiente libre | inventory.transfer | inventory | admin (1), gerente (2) |

## Bitácora

`warehouse.created`, `warehouse.updated` (nombre, de venta), `warehouse.archived`,
`warehouse.restored`, `transfer.created`; el almacén va en el detalle de las acciones de conteo,
compra y merma.

## Estados

Almacén: activo ⇄ archivado. Archivar exige: no es el de venta, todas sus existencias en 0, sin
conteo abierto. Reactivar no exige nada.

## Ajustes de la revisión de base de datos

Lo de [research.md › R11](research.md) manda sobre las tablas de arriba donde difieran: compuerta
`EGB04` y `is_sales` inmutable en la 0117, `low_stock_state` sumado por sucursal, `expense_items`
llenado por `item_type`, llaves compuestas con `warehouse_id` desde movimientos a conteo y merma,
lugares con nombre único por almacén, `ON DELETE no action`, y Downs con guarda.
