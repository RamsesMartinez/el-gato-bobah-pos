# Data Model: Una sola puerta para cobrar

Migración **`0082_order_drafts.sql`** (número de trabajo: toma el siguiente libre de `develop` al
fusionar; 0080–0081 son de otra rama). Solo tablas nuevas: **no toca `orders` ni ninguna consulta de
dinero**. Patrón de RLS, llaves compuestas y grants copiado de
[0079_split_bill.sql](../../server/migrations/0079_split_bill.sql).

## Tablas

### `order_drafts` — la cuenta en captura

| Columna | Tipo | Nota |
|---|---|---|
| `id` | `uuid primary key` | Lo pone la tableta. Llave de idempotencia de crear y de enviar (`orders.client_uuid` / `order_line_batches.client_uuid`) |
| `company_id` | `bigint not null default nullif(current_setting('app.company_id', true), '')::bigint references companies on delete cascade` | |
| `order_id` | `bigint null` | Nulo = cuenta nueva. No nulo = «Nuevo» de ese pedido. FK `(order_id, company_id) → orders (id, company_id) on delete no action` (unique de 0065) |
| `status` | `text not null default 'capturando'` | `check (status in ('capturando','enviada','descartada'))` — texto y no enum: un enum nuevo no se puede quitar en el Down sin recrear |
| `folio_name` | `text null` | Amarrado al nacer (D-2) y conservado al enviar (rastro). Nulo solo en la «Nuevo» de un pedido: `check (order_id is not null or folio_name is not null)` |
| `folio_scheme` | `folio_scheme null` | Con qué esquema se sacó, para devolverlo a SU bolsa. `check ((folio_name is null) = (folio_scheme is null))` |
| `service_type` | `service_type not null default 'mostrador'` | Enum existente |
| `customer_name` | `text null` | `check (char_length(customer_name) <= 60)` |
| `delivery_platform_id` | `smallint null` | FK `(delivery_platform_id, company_id) → delivery_platforms (id, company_id) on delete no action` — el orden del unique que ya existe (0037). Nulo salta el chequeo (MATCH SIMPLE) |
| `platform_order_ref` | `text null` | Mismo saneo que `orders.platform_order_ref` |
| `delivery_fee` | `numeric(10,2) not null default 0` | `check (delivery_fee >= 0)` |
| `discount_amount` | `numeric(10,2) null` | Excluyente con `discount_percent`: `check (discount_amount is null or discount_percent is null)` |
| `discount_percent` | `numeric(5,2) null` | `check (discount_percent between 0 and 100)` |
| `discount_set_by` | `bigint null` | Quién puso el descuento **en la cuenta**; al enviar pasa a `orders.discount_set_by`. `check ((discount_amount is null and discount_percent is null) = (discount_set_by is null))`; FK `(company_id, discount_set_by) → users on delete no action`. Sin ella el pedido diría que lo puso quien abrió la cuenta (puerta «Descuentos») |
| `platform_ref_set_by` | `bigint null` | Quién tecleó el folio de plataforma; al enviar pasa a `orders.platform_ref_set_by`. Mismo check de pareja con `platform_order_ref` y misma FK |
| `opened_by` | `bigint not null` | FK `(company_id, opened_by) → users (company_id, id) on delete no action` (`users_tenant_key`, 0073): un usuario con cuentas capturadas no se borra; el rastro sobrevive. FR-020 |
| `header_version` | `int not null default 1` | Versión esperada para editar la cabecera (R-2) |
| `created_at` | `timestamptz not null default now()` | Guarda de `taken_at` al soltar el nombre (R-3) |
| `updated_at` | `timestamptz not null default now()` | Cualquier cambio. Base de las 12 h (D-8) y del aviso entre tabletas |
| `sent_at` | `timestamptz null` | `check ((status = 'enviada') = (sent_at is not null))` |
| `discarded_at`, `discarded_by` | `timestamptz null`, `bigint null` | `check ((status = 'descartada') = (discarded_at is not null))`. `discarded_by` nulo = lo descartó el barrido; FK `(company_id, discarded_by) → users (company_id, id) on delete no action` |
| `discard_reason` | `text null` | `check (discard_reason in ('manual','expired','order_closed','empty'))` y el mismo check de pareja con `status` |

Restricciones e índices:

- `unique (id, company_id)` para las FKs compuestas de los renglones.
- **Enviada ⇒ pedido apuntado**: `check (status <> 'enviada' or order_id is not null)` — al enviar
  una cuenta sin pedido se llena `order_id` con el creado. Por eso «Nuevo» se distingue por
  `folio_name is null`, no por `order_id`.
- `order_drafts_live_name` **único parcial** `(company_id, folio_name) where status = 'capturando'
  and folio_name is not null` — dos cuentas vivas no comparten nombre (FR-001, US2 AS2).
- `order_drafts_live_per_order` **único parcial** `(company_id, order_id) where status =
  'capturando' and order_id is not null` — una sola «Nuevo» viva por pedido.
- `order_drafts_live` `(company_id, updated_at) where status = 'capturando'` — la lista y el barrido.
- `order_drafts_by_order` `(company_id, order_id) where order_id is not null` — lo usan el barrido de «Nuevo» de pedidos cancelados y la lista (pegar la «Nuevo» a su pedido); la enviada también se busca por él.

**La migración no crea índices en tablas existentes**: todos los unique que piden las FKs compuestas ya existen (`products` 0040/0071, `delivery_platforms` 0037, `orders` 0065, `users` 0073). Solo crea tablas nuevas, así que no toma locks sobre tablas vivas.

Grants: `select, insert, update` a `gatobobah_app`. **Sin `delete`**: la descartada se conserva.
`update` por columnas no aplica (casi todas cambian).

### `order_draft_lines` — los renglones

| Columna | Tipo | Nota |
|---|---|---|
| `id` | `uuid primary key` | Lo pone la tableta (= `opId` del agregado que lo creó) |
| `company_id` | igual que arriba | |
| `draft_id` | `uuid not null` | FK `(draft_id, company_id) → order_drafts (id, company_id) on delete cascade` |
| `product_id` | `bigint not null` | FK `(company_id, product_id) → products (company_id, id) on delete no action` — `restrict` no se difiere y abortaría el borrado en cascada de una empresa (0079); `no action` igual bloquea el reorg de datos que borra un producto con cuenta viva |
| `qty` | `numeric(8,2) not null` | `check (qty > 0)`; el tope es `domain.MaxOrderQty` en la frontera |
| `modifiers` | `jsonb not null default '[]'` | `[{optionId, qty, portion}]`, validado en `domain` (R-16). `check (jsonb_typeof(modifiers) = 'array')` |
| `notes` | `text null` | `check (char_length(notes) <= 200)` |
| `position` | `int not null` | Orden de captura; `max+1` dentro de la tx |
| `version` | `int not null default 1` | Versión esperada para cambiar o quitar (R-2) |
| `created_at`, `updated_at` | `timestamptz not null default now()` | |

Índice `(company_id, draft_id, position)`. `position` y la fusión son seguras porque **toda escritura de renglones toma primero `select … from order_drafts where id = $1 for update`** (serializa por cuenta; dos tabletas no duplican posición ni crean dos renglones del mismo producto). Grants: `select, insert, update, delete` — quitar un
renglón de una cuenta que no se ha mandado sí lo borra (no es dinero ni venta: D-1).

### `order_draft_adds` — idempotencia de cada «agregar»

| Columna | Tipo | Nota |
|---|---|---|
| `company_id` | igual | |
| `op_id` | `uuid not null` | Uno por toque. `primary key (company_id, op_id)` — por empresa, como `order_line_batches` |
| `draft_id` | `uuid not null` | FK `(draft_id, company_id) → order_drafts on delete cascade` |
| `line_id` | `uuid not null` | El renglón que recibió el agregado (creado o fusionado). **Sin FK a propósito**: si el renglón se quitó después, el reintento del agregado tiene que seguir siendo no-op (va dicho en el comentario de la migración) |
| `created_at` | `timestamptz not null default now()` | |

Índice `(company_id, draft_id)` (el cascade y la vista). Grants: `select, insert`.

**Crecimiento** (estimado, sin `delete` a propósito): ~30 mil cuentas, ~150 mil renglones y ~200 mil agregados por año a 40–80 pedidos/día; decenas de MB/año. Techo y camino de purga (agregados de cuentas terminales con más de 30 días) en el comentario de la migración; no se construye hoy.

### RLS (las tres tablas)

`enable row level security` + `policy tenant_isolation using/with check (company_id =
nullif(current_setting('app.company_id', true), '')::bigint)`. Las cubre
`TestEveryCompanyTableIsIsolated` sin tocarlo.

### Down

Se niega si hay una cuenta `capturando` (se perdería lo que alguien está capturando) o una `enviada`
(rastro de quién capturó). Si no, `drop table` de las tres en orden inverso.

## Entidades de dominio (Go, `server/internal/domain/`)

- **`Draft`, `DraftLine`, `DraftModifier`** — entrada validada: `ValidateDraftLine` (qty con
  `ValidQty`, modificadores con `optionId > 0`, `qty ≥ 1`, `portion ∈ {"", "A", "B"}`, nota ≤ 200).
- **`MergeTarget(lines, add) (lineID, ok)`** — el renglón vivo con el que se fusiona un agregado sin
  modificadores ni nota (R-2).
- **`DraftIdleLimit = 12 * time.Hour`** y **`DraftExpired(updatedAt, now) bool`**.
- **`AccountState`** y **`AccountGroup`** (R-6), con sus constantes en inglés:
  `capturing · in_kitchen · paid_in_kitchen · partly_paid · delivered_owes` y
  `capturing · in_kitchen · delivered_owes · previous_days`.
- **`CanReceiveLines(OrderForAdd) error`** (R-11) con sentinels nuevos `ErrOrderClosed` y
  `ErrPlatformOrderNoLines`.
- **`ErrDraftChanged`** (409 `DRAFT_CHANGED`), **`ErrDraftDiscarded`** (409 `DRAFT_DISCARDED`),
  **`ErrDraftAlreadySent`** (409 `DRAFT_SENT`, lleva el `orderId` en el mensaje envuelto) y
  **`ErrDraftHasOrderHeader`** (422: la «Nuevo» no edita cabecera; la cabecera es del pedido).
- **`AvailableNames(lista, consumidos, usadosTurno, vivos []string)`** — envoltura de
  `DisponiblesDeLaBolsa` que suma los nombres vivos a los usados. Un solo predicado para la pantalla,
  la cuenta nueva y `resolverFolio`.

## Transiciones de la cuenta

```text
            crear (primer producto)
                   │
                   ▼
            ┌─ capturando ─┐
 enviar     │              │ descartar (a mano, vacía, 12 h, pedido cancelado)
 (tx única) ▼              ▼
         enviada        descartada      ← terminales; ninguna vuelve
```

- `enviada` y `descartada` no reciben nada: agregar, cambiar o editar → `409`.
- Descartar una `enviada` → `409 DRAFT_SENT` (edge case del spec: «ya es un pedido»).
- Una vez enviada, lo siguiente que se agregue a ese pedido nace como **otra** cuenta con
  `order_id` (D-3).

## Lo que NO cambia en `orders`

Ninguna columna nueva. `orders.client_uuid` recibe el id de la cuenta en vez del de la pestaña;
`order_line_batches.client_uuid` igual. `opened_by` del pedido = `opened_by` de la cuenta (quién la
abrió), no quien tocó «Enviar».
