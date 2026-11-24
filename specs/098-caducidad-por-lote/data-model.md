# Data Model: Caducidad por lote (098)

Migración **0128** (`0128_stock_lots.sql`). Aditiva: la API anterior no la lee ni la escribe, y sus
movimientos pasan por el trigger nuevo igual que los de la nueva.

## stock_lots — nueva

| Columna | Tipo | Regla |
| --- | --- | --- |
| id | bigint identity PK | |
| company_id | bigint not null | default de sesión (`nullif`), FK `companies`, RLS |
| branch_id | bigint not null | |
| warehouse_id | bigint not null | FK `(company_id, branch_id, warehouse_id) → warehouses` |
| ingredient_id | bigint not null | FK `(ingredient_id, company_id) → ingredients` |
| expires_on | date not null | `check (expires_on between '2000-01-01' and '2100-12-31')` |
| qty | numeric(14,4) not null default 0 | `check (qty >= 0)` |
| created_at, updated_at | timestamptz | |

- `unique (company_id, warehouse_id, ingredient_id, expires_on)`; `unique (company_id, id)`.
- Sin índice extra: la llave única sirve al `exists` del trigger y a la vista (R12).
- Grants `gatobobah_app`: select, insert, update. Sin delete.

## stock_lot_allocations — nueva

| Columna | Tipo | Regla |
| --- | --- | --- |
| id | bigint identity PK | |
| company_id | bigint not null | default de sesión, RLS |
| movement_id | bigint not null | FK `(movement_id, company_id) → stock_movements (id, company_id)`, `on delete no action` |
| lot_id | bigint not null | FK `(company_id, lot_id) → stock_lots(company_id, id)` |
| qty | numeric(14,4) not null | `check (qty <> 0)`; negativo = tomó del lote, positivo = devolvió |

- Índices `(company_id, movement_id)`, `(company_id, lot_id)`.
- Grants: select, insert, update (update solo para el cambio de unidad). El trigger corre como quien
  inserta el movimiento (`gatobobah_app`), así que ese rol necesita los grants de las dos tablas.

## stock_movements: llave nueva

`alter table stock_movements add constraint stock_movements_id_company_key unique (id, company_id)` (R12 M1).

## stock_count_line_lots — nueva

| Columna | Tipo | Regla |
| --- | --- | --- |
| company_id | bigint not null | default de sesión, RLS |
| count_id | bigint not null | FK `stock_counts` (misma empresa) |
| ingredient_id | bigint not null | FK `(ingredient_id, company_id) → ingredients` |
| expires_on | date not null | mismo check |
| qty | numeric(14,4) not null | `check (qty > 0)` |

- PK `(count_id, ingredient_id, expires_on)`. Grants: select, insert, delete (la captura reemplaza).

## Columnas nuevas (nullable, sin default)

| Tabla | Columna | Uso |
| --- | --- | --- |
| stock_movements | expires_on date | Lote al que va el movimiento (R4) |
| expense_items | expires_on date | Fecha capturada al recibir |
| waste_records | lot_expires_on date | Merma de un lote (R9) |

## notifications

- `notifications_kind` agrega `expired_left`; `notifications_item_shape` lo trata como los de stock.
- `notifications_resolved_reason` agrega `expired_cleared`.

## Funciones

- `business_today(p_company bigint) returns date` — `stable`.
- `apply_stock_lots()` — trigger `trg_stock_movement_lots` `AFTER INSERT` en `stock_movements` (R4).
- `consume_lots_fefo(p_movement, p_company, p_warehouse, p_ingredient, p_qty)` — interna.
- `evaluate_expired_left(p_company, p_branch, p_warehouse, p_ingredient)` — abre o cierra el aviso
  (R5); la llaman el trigger y el cierre de conteo.

## Down

Quita trigger, funciones, tablas, columnas y la llave nueva de `stock_movements`, y devuelve los checks
de `notifications` después de borrar las filas `expired_left` (el check viejo rechaza su kind y su
`expired_cleared`). Pierde las fechas: no se niega, porque una
nueva subida no las necesita para arrancar (vuelven a nacer con compras y conteos).

## Respaldo anónimo

Ninguna columna nueva es de texto: no entra a la clasificación.

## Clasificación de tablas que apuntan a `ingredients`

| Tabla | `MergeTables` | `UnitChangeTables` |
| --- | --- | --- |
| stock_lots | la mueve el servicio (R7) | se escala por el factor |
| stock_count_line_lots | conserva (histórico del conteo) | se escala por el factor |
