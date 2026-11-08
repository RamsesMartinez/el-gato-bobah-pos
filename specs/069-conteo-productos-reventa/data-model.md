# Data Model: 069

## Tabla nueva `stock_count_product_lines` (migración 0110)

| Columna | Tipo | Regla |
| --- | --- | --- |
| `count_id` | bigint not null | FK `(count_id, company_id)` → `stock_counts (id, company_id)` on delete cascade |
| `company_id` | bigint not null | default de `app.company_id` con `nullif`; FK a `companies` on delete cascade |
| `product_id` | bigint not null | FK `(product_id, company_id)` → `products (id, company_id)` |
| `counted_qty` | numeric(14,4) not null | `>= 0` y entero (`counted_qty = trunc(counted_qty)`) |
| `system_qty` | numeric(14,4) not null | lo que el sistema tenía al capturar (puede ser negativo) |
| `captured_by` | bigint not null | FK `(company_id, captured_by)` → `users (company_id, id)` |
| `captured_at` | timestamptz not null default now() | |
| `last_movement_id` | bigint | último movimiento del producto en la sucursal al capturar (puerta VIII: periodo del faltante). Sin FK a propósito, como en `stock_count_lines`: 0 = sin movimientos |
| `unit_cost` | numeric(12,4) | `products.current_cost` al cerrar (puerta VIII: costo del faltante). Nulo en un conteo cerrado = la API anterior lo cerró sin ajustarlo |

- PK `(count_id, product_id)`; índice `(company_id, product_id)` (lookup del último conteo y FK).
- RLS con la política `tenant_isolation` (`nullif`), `grant select, insert, update, delete` a
  `gatobobah_app` (quitar una cifra borra el renglón, como en artículos).
- Sin columnas de texto: el respaldo anónimo no la tiene que limpiar (la vigila igual
  `TestEveryColumnIsCoveredByTheAnonymousBackup`).
- **Down**: se niega (`raise exception`) si hay renglones; con la tabla vacía, `drop table`. Su
  renglón en `guardedDownCleanup` borra los renglones en la copia de prueba. Los ajustes en
  `stock_movements` sobreviven al Down (y el Down de 0092 ya se niega con ellos).
- **Existencia**: capturar un producto asegura su fila en `stock_levels` (en cero) antes de tomarla
  `for share` (R8).

## Movimientos

Sin cambio de esquema: el ajuste es `stock_movements` con `item_type = 'producto'`, `product_id`,
`movement_type = 'ajuste'`, `stock_count_id`, `user_id`, `reason = 'conteo'`, `branch_id` del conteo.

## Modo deducido `options`

Sin columna. `domain.ProductModeFacts` gana `OptionsDeduct bool`; `ModeOf` devuelve `ModeOptions`
cuando no es paquete, no lleva existencia propia ni insumos, está confirmado y `OptionsDeduct`.

«Una opción resta» = activa, en un grupo activo ligado al producto, y (tiene receta con al menos un
renglón **o** está ligada a un producto que resta: existencia propia, receta con renglones o
paquete).

## Transiciones

- Conteo: `open` → `closed` | `discarded` (sin cambio). Cerrar ajusta artículos y productos.
- Producto: `own_stock` → `options` (cierra su contador, bitácora) ; `none`/`undecided`/`recipe` →
  `options` (sin movimientos; desde `recipe` la receta se quita como hoy con «No se cuenta»).
