# Data model: Notificaciones de inventario (084)

Una sola migración aditiva, **0115_inventory_notifications.sql** (la siguiente libre en `develop` y en
las ramas abiertas el 2026-11-14; si el menú en dos niveles llega antes con una migración, ésta se
renumera). Nada se quita ni se renombra: la API anterior sigue sirviendo durante la subida y el
trigger nuevo aplica también a sus movimientos.

Convenciones de todas las tablas nuevas: `company_id` con default de `app.company_id`, RLS con
política `nullif`, llaves foráneas compuestas `(company_id, …)`, `grant` explícito a
`gatobobah_app` (el único rol que inserta movimientos de existencia), y renglón en
`TestEveryCompanyTableIsIsolated`, en la clasificación del respaldo anónimo y en
`app.MergeTables`/`app.UnitChangeTables` si apuntan a `ingredients`.

## 1. Columnas nuevas en `ingredients` y `products`

| Columna | Tipo | Default | Para qué |
|---|---|---|---|
| `low_stock_enabled` | boolean not null | false | La casilla «Enviar alerta de stock bajo» |
| `low_stock_on_min` | boolean not null | true | Avisar al llegar al mínimo |
| `low_stock_on_out` | boolean not null | true | Avisar al agotarse |
| `low_stock_banner` | boolean not null | false | Mostrar franja en pantalla |
| `coverage_days` | smallint null, check 1..90 | null (= 14) | Días de cobertura de la lista de compra |
| `low_stock_asked_at` | timestamptz null | null | Ya se hizo la pregunta de alertas |

`min_stock` ya existe en las dos tablas (numeric(14,4), nulo). Nulo o 0 = sin mínimo.
Las columnas con default se agregan sin reescribir la tabla (Postgres ≥ 11). Ninguna es texto: no
entran a la clasificación del respaldo anónimo. El cambio de unidad ya convierte `min_stock`.

## 2. `branch_stock_minimums` — el mínimo de una sucursal

Sobrescribe el mínimo de la empresa para una sucursal (decisión del dueño: «cada sucursal puede
definir sus reglas»). Solo el mínimo: la casilla y los eventos son del artículo.

| Columna | Tipo | Notas |
|---|---|---|
| `company_id`, `branch_id` | bigint | FK `(company_id, branch_id) → branches` |
| `item_type` | `stock_item_type` | |
| `ingredient_id` / `product_id` | bigint null | Exactamente uno, como `stock_movements` |
| `min_stock` | numeric(14,4) not null, check ≥ 0 | 0 = sin mínimo en esa sucursal |
| `updated_by`, `updated_at` | | |

Únicos `(branch_id, ingredient_id)` y `(branch_id, product_id)`. Mínimo efectivo =
`coalesce(branch_stock_minimums.min_stock, articulo.min_stock)`.
Unión: el destino conserva el suyo; las filas del origen se borran (el origen queda archivado).
Cambio de unidad: se convierte con el mismo factor que `min_stock`; con reemplazo pasa al nuevo.

## 3. `notifications`

| Columna | Tipo | Notas |
|---|---|---|
| `id` | bigint identity | |
| `company_id` | bigint | |
| `branch_id` | bigint not null | FK compuesta |
| `kind` | text check in (`stock_low`, `stock_out`, `not_found`) | Genérico: otros tipos después |
| `item_type` | `stock_item_type` null | Nulo solo en `not_found` |
| `ingredient_id` / `product_id` | bigint null | Uno en los de existencia, ninguno en `not_found` |
| `item_key` | bigint generated (`coalesce(ingredient_id, -product_id, 0)`) stored | Para el índice único |
| `query_text` | text null, 1..80 | Solo `not_found`: lo que se escribió |
| `status` | text check in (`open`, `resolved`, `dismissed`) | |
| `on_hand_seen`, `min_seen` | numeric(14,4) null | Cifras al abrir |
| `needs_count` | boolean not null default false | «Pedir conteo» |
| `opened_by` | bigint null | Nulo = lo abrió el sistema |
| `opened_at` | timestamptz default now() | |
| `resolved_at`, `resolved_by` | | `resolved_by` nulo = un movimiento |
| `resolved_reason` | text null check in (`above_min`, `restocked`, `counted`, `arrived`, `undone`, `alert_off`, `archived`, `merged`, `dismissed`, `created`, `alias`) | |
| `email_state` | text check in (`pending`, `sent`, `skipped`) default `pending` | |
| `email_send_id` | bigint null | FK a `notification_email_sends` |

Índices: único parcial `(company_id, branch_id, item_key, kind) where status = 'open' and kind <> 'not_found'`;
`(company_id, status, opened_at desc)` para la campana; `(company_id, email_state, opened_at) where
email_state = 'pending'` para el despachador. Checks de forma: `not_found` ⇔ `query_text` no nulo y
sin artículo; `status = 'open'` ⇔ `resolved_at` nulo.

`query_text` es texto libre que escribió una persona: el respaldo anónimo lo borra.

Transiciones: `open → resolved` (movimiento, conteo, «ya llegó», alta, alias), `open → dismissed`
(descartar, deshacer). No hay regreso: si vuelve a bajar se abre otra.

## 4. `notification_reads` — leído por persona

`(company_id, notification_id, user_id)` llave primaria; `read_at timestamptz null`,
`banner_seen_at timestamptz null`. No leído = sin fila o `read_at` nulo (`left join`).

## 5. `stock_out_reports` — «Se acabó» y sus confirmaciones

| Columna | Tipo | Notas |
|---|---|---|
| `id` | bigint identity | |
| `company_id`, `branch_id` | | |
| `item_type`, `ingredient_id` / `product_id` | | Nulos en «No lo encuentro» |
| `notification_id` | bigint not null | FK compuesta |
| `kind` | text check in (`report`, `confirmation`, `not_found`) | |
| `movement_id` | bigint null | El ajuste a 0; nulo en confirmación, «no lo encuentro» o si ya estaba en 0 |
| `delta` | numeric(14,4) null | Lo que restó (o sumó, si estaba negativa) |
| `client_key` | uuid not null | Único `(company_id, client_key)`: idempotencia |
| `reported_by` | bigint not null | FK a `users` |
| `reported_at` | timestamptz default now() | Reloj de la base |
| `undone_at` | timestamptz null | |
| `undo_movement_id` | bigint null | |

Índice `(company_id, reported_by, reported_at)` para el tope de 20 por hora.
Unión: lo pasado se queda en el origen; después, rechaza (con las dos guardas de la 0104).
Cambio de unidad: un reporte deshacible (≤ 90 s) bloquea; los demás guardan su `delta` histórico.

## 6. `notification_recipients`

| Columna | Tipo | Notas |
|---|---|---|
| `id` | bigint identity | |
| `company_id` | | |
| `user_id` | bigint null | Persona del sistema |
| `email` | citext null | Correo extra |
| `kind` | text check in (`stock`, `not_found`) | Tipo de aviso |
| `enabled` | boolean not null | false = quitado de la omisión |
| `updated_by`, `updated_at` | | |

Exactamente uno de `user_id`/`email`. Únicos `(company_id, user_id, kind)` y `(company_id, email,
kind)`. Destinatarios efectivos de un tipo = (personas activas con `inventory.configure` y
`recovery_email`, salvo las que tienen fila `enabled = false`) ∪ correos extra con `enabled = true`.
`email` va al respaldo anónimo como dato personal.

## 7. `notification_email_sends`

| Columna | Tipo | Notas |
|---|---|---|
| `id` | bigint identity | |
| `company_id` | | |
| `kind` | text check in (`window`, `morning`) | |
| `period_key` | text | Inicio del bloque de 5 min (UTC, ISO) o la fecha del negocio |
| `status` | text check in (`pending`, `sent`, `failed`, `empty`) | |
| `attempts` | smallint default 0 | Hasta 5 |
| `last_error` | text null | Sin datos personales (solo el tipo de error) |
| `recipients` | smallint | Cuántos recibieron (no quiénes) |
| `created_at`, `sent_at` | | |

Único `(company_id, kind, period_key)`. Reclamo igual que `daily_summary_sends`: solo se vuelve a tomar
una `failed` o una `pending` de más de 10 min.

## 8. `business_settings`

`notifications_morning_at time not null default '08:00'`.

## 9. Funciones y trigger

- `evaluate_low_stock(p_branch bigint, p_type stock_item_type, p_ingredient bigint, p_product bigint)
  returns void` — `security invoker`, lee la empresa de `app.company_id`. Abre/cierra `stock_low` y
  `stock_out` del sistema y cierra el `stock_out` manual cuando la existencia pasa de 0. Termina con
  `pg_notify('notifications', company_id::text)` si cambió algo.
- `notify_stock_movement()` + trigger `trg_stock_movement_notify` AFTER INSERT en `stock_movements`:
  sale de inmediato si `replacement_id` no es nulo, o si el artículo no tiene alerta ni aviso abierto;
  si no, llama a la función dentro de `begin … exception when others then raise warning` (la venta
  nunca falla por el aviso).

## 10. Permiso y bitácora

- `permissions` id 38 `inventory.report_out` (área `inventory`), en `system_role_permissions` para los
  cuatro sugeridos.
- Acciones nuevas de bitácora: `stock.out_reported` y `stock.out_undone` (grupo `cancel`, como la
  merma), `stock.low_stock_configured` y `notification.recipients_changed` (grupo `price`, como la
  configuración del almacén), `notification.resolved`.

## Down

Se niega si hay filas en `notifications`, `stock_out_reports`, `notification_recipients` o
`branch_stock_minimums` (perdería avisos y configuración). Sin datos: quita trigger, funciones,
tablas, columnas, el permiso y sus filas. Su renglón en `guardedDownCleanup`.
