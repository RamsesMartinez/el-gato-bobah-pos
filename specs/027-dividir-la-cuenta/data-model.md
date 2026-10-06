# Data Model: Dividir la cuenta por productos

Una migración, con nombre de trabajo `NNNN_split_bill.sql`. **Toma el siguiente número
libre de `develop` al fusionar** (D-11): 0073–0077 ya están en otras ramas y goose no acepta huecos.

Contiene:
- cuatro tablas nuevas;
- tres columnas en `order_payments` y una en `orders`;
- dos `unique (id, company_id)` para colgar FKs compuestas.

Todas las tablas nuevas siguen el patrón de RLS de 021:

```text
company_id bigint not null
  default nullif(current_setting('app.company_id', true), '')::bigint
  references companies(id) on delete cascade
enable row level security
policy tenant_isolation using / with check
  (company_id = nullif(current_setting('app.company_id', true), '')::bigint)
grant select, insert … to gatobobah_app   -- explícito: 0024 no dejó default privileges
set local lock_timeout = '3s'
```

Las FKs que no son la cobertura en cascada van **`on delete no action`**, no `restrict`. `restrict`
no se difiere y puede abortar el borrado en cascada de una empresa; `no action` se evalúa al final
de la sentencia y fuera de ese caso se comporta igual.

## Prerrequisitos de FK compuesta

| Cambio | Por qué |
|---|---|
| `order_lines` gana `unique (id, company_id)` | Los chequeos de FK saltan RLS: sin la empresa en la llave, una fila de una empresa podría apuntar a un renglón de otra |
| `order_payments` gana `unique (id, company_id)` | Lo mismo para la cobertura |

Medido en producción: 568 y 222 filas (304 kB y 136 kB). El `ACCESS EXCLUSIVE` dura milisegundos y
lo acota el `lock_timeout`. **No** se usa `create unique index concurrently`: obliga a
`NO TRANSACTION`, pierde la atomicidad del Down y puede dejar un índice inválido, y para este
tamaño no compra nada.

`orders` ya tiene `orders_id_company_key` (0065).

## `order_payment_lines` — qué cubrió cada pago

| Columna | Tipo | Regla |
|---|---|---|
| `id` | `bigint generated always as identity` | PK |
| `company_id` | `bigint` | patrón |
| `order_payment_id` | `bigint not null` | FK `(order_payment_id, company_id)` → `order_payments` **on delete cascade**: si el pago se devuelve, su cobertura se va con él (la copia queda en la bitácora) |
| `order_line_id` | `bigint not null` | FK `(order_line_id, company_id)` → `order_lines`, `no action` |
| `qty` | `numeric(8,2) not null` | `check (qty > 0)` |
| `amount` | `numeric(10,2) not null` | `check (amount >= 0)`; lo pagado de ese renglón, ya con su parte de descuento |

- `unique (order_payment_id, order_line_id)`.
- Índice `(company_id, order_line_id)`.
- **Invariante** (en `app`, bajo el `FOR UPDATE` del pedido): Σ `qty` por renglón ≤ `quantity`.
- Σ `amount` de un pago ≤ `order_payments.amount`. La diferencia es lo cubierto sin producto (envío,
  redondeo, el último pago).
- Grants: `select, insert`. El borrado en cascada corre con los privilegios del dueño de la tabla.

## `order_payment_voids` — bitácora de pagos devueltos

| Columna | Tipo | Regla |
|---|---|---|
| `id` | identity | PK |
| `company_id` | `bigint` | patrón |
| `order_id` | `bigint not null` | FK compuesta → `orders`, `no action` |
| `original_payment_id` | `bigint not null` | Sin FK (la fila ya no existe). `unique (company_id, original_payment_id)` |
| `payment_number` | `smallint not null` | El número que tenía en el pedido (D-12) |
| `payment_method_id` | `smallint not null` | FK |
| `amount` | `numeric(10,2) not null` | `check (amount > 0)` |
| `tip_amount` | `numeric(10,2) not null` | `check (tip_amount >= 0)` |
| `reference` | `text` | El folio o voucher del cobro |
| `register_session_id` | `bigint not null` | FK. Nunca nula aquí porque solo se devuelven pagos de un turno abierto, aunque en `order_payments` sí puede serlo |
| `received_by` | `bigint` | FK users, `no action` |
| `paid_at` | `timestamptz not null` | El `created_at` original |
| `client_uuid` | `uuid` | `unique (company_id, client_uuid) where client_uuid is not null` |
| `split_part`, `split_of` | `smallint` | Como estaban |
| `covered` | `jsonb not null default '[]'` | `[{lineId, qty, amount}]`; solo bitácora |
| `voided_by` | `bigint not null` | FK users, `no action` |
| `voided_at` | `timestamptz not null default now()` | |
| `reason` | `text not null` | `check (length(trim(reason)) > 0)` |

- Índices: `(company_id, register_session_id)` para el corte y `(company_id, order_id)` para la vista.
- Grants: `select, insert`.

## Número estable de cada pago

`order_payments` gana `payment_number smallint` (check `> 0`, `unique (order_id, payment_number)`
en los nuevos). Lo asigna `Charge` bajo el candado del pedido como (pagos vivos + devueltos del
pedido, **contando los viejos sin número**) + 1. Los pagos existentes quedan nulos y la vista los
numera por `created_at` con la misma regla; al devolver uno, `order_payment_voids.payment_number`
guarda el número que la vista le daba. Así un número nuevo nunca repite el de un pago viejo. La
migración no rellena nada.

## `order_line_move_batches` y `order_line_moves` — qué se pasó de un pedido a otro

**`order_line_move_batches`** (una petición):

| Columna | Tipo | Regla |
|---|---|---|
| `company_id` | `bigint` | patrón |
| `client_uuid` | `uuid not null` | PK `(company_id, client_uuid)` |
| `from_order_id`, `to_order_id` | `bigint not null` | FKs compuestas → `orders`, `no action`; `check (from_order_id <> to_order_id)` |
| `moved_by` | `bigint not null` | FK users |
| `moved_at` | `timestamptz not null default now()` | |

**`order_line_moves`** (cada renglón del lote):

| Columna | Tipo | Regla |
|---|---|---|
| `id` | identity | PK |
| `company_id` | `bigint` | patrón |
| `client_uuid` | `uuid not null` | FK `(company_id, client_uuid)` → lote |
| `order_line_id` | `bigint not null` | El renglón que viajó (el nuevo, si se partió). FK compuesta, `no action` |
| `split_from_line_id` | `bigint` | El original cuando se movieron solo algunas piezas. FK compuesta, `no action` |
| `qty` | `numeric(8,2) not null` | `check (qty > 0)` |

- Índices: lote `(company_id, from_order_id)` y `(company_id, to_order_id)`; renglones
  `(company_id, order_line_id)` y `(company_id, split_from_line_id)`.
- Grants: `select, insert` en las dos.

## `order_payments` — columnas nuevas

| Columna | Tipo | Regla |
|---|---|---|
| `split_part` | `smallint` | `check ((split_part is null) = (split_of is null))` |
| `split_of` | `smallint` | `check (split_part is null or (split_of >= 2 and split_part between 1 and split_of))`. El tope de 12 vive en `domain` |
| `payment_number` | `smallint` | arriba |

Columnas nulas, sin reescritura de tabla.

## `orders` — columna nueva

| Columna | Tipo | Regla |
|---|---|---|
| `merged_into_order_id` | `bigint null` | FK `(merged_into_order_id, company_id)` → `orders (id, company_id)` (`orders_id_company_key`, 0065), `on delete no action`. `check (merged_into_order_id is null or status = 'cancelada')` y `check (merged_into_order_id <> id)` |

- Se llena solo cuando «Pasar» deja vacío el origen hacia un pedido **existente** (D-5): ese pedido
  queda `cancelada` con el motivo fijo «Se juntó con otro pedido».
- Índice parcial `(company_id, merged_into_order_id) where merged_into_order_id is not null`, para
  llegar del destino a lo que se juntó con él.
- Los reportes de Ventas y las ventas del turno del corte (`SessionSales`, `CountSessionSales`) lo
  excluyen con el mismo predicado en lista, conteo y resumen, y no lo cuentan como cancelación;
  `SalesCancelledLines` y su gemela, que hoy filtran `status not in ('cancelada', 'reembolsada')` y
  por eso no contarían nada de él, agregan `or o.merged_into_order_id is not null` para seguir
  contando lo que se quitó de él antes de juntarlo (`TestAMergedOrderIsNotACancellation`).
- Columna nula sin default: no reescribe la tabla. Los dos checks recorren `orders` una vez bajo el
  `lock_timeout`.

## Renglón partido

No hay columna nueva en `order_lines`. Partir crea una fila hermana con `quantity = k`, que copia
`product_id`, `product_name`, `unit_price`, `modifiers_total`, `unit_cost`, `notes`,
`enviado_a_cocina_at` y sus `order_line_modifiers`. `line_total` se recalcula para las dos con
`domain.Round2((precio + modificadores) × cantidad)`. `delivered_qty` se reparte con D-7. Los
movimientos de inventario se parten **insertando** pares `venta` con `reason = 'renglón partido'`
(D-7).

## Down

El Down se niega a correr con `raise exception` (el patrón de 0037, 0040 y 0041) si hay algo que
perdería sin rastro:
- filas en `order_payment_voids`: es la única copia de dinero devuelto;
- filas en `order_payment_lines`: qué productos cubrió cada pago, que no se puede reconstruir;
- filas en `order_line_move_batches` u `order_line_moves`: qué se pasó de un pedido a otro;
- algún `orders.merged_into_order_id` no nulo: sin él, el pedido juntado se leería como cancelado;
- algún `order_payments.split_part` o `payment_number` no nulo: sin ellos se pierde qué parte cubrió
  cada pago y el número impreso en su ticket.

Cada guarda tiene su caso en el test de la migración. Orden: las cuatro tablas, luego
las columnas (las tres de `order_payments` y `merged_into_order_id` con sus checks e índice), y al
final los dos `unique (id, company_id)`.

## Transiciones

| Operación | Estado del pedido antes | Después |
|---|---|---|
| Cobrar por productos / parte / monto | abierta, lista, entregada (con saldo) | igual; `entregada` si el cierre automático de `Charge` aplica |
| Devolver un pago | abierta, lista, entregada (no cancelada ni reembolsada), con el pago en un turno abierto | igual; vuelve a deber |
| Pasar productos (origen, parte) | abierta, lista, de un turno abierto | igual; `entregada` si ya no le falta nada |
| Pasar **todos** los productos hacia un pedido nuevo | — | rechazado: «Ya es su propio pedido; no hace falta pasarlo» |
| Pasar **todos** los productos hacia un pedido existente | abierta, lista, sin pagos | `cancelada`, motivo «Se juntó con otro pedido», `merged_into_order_id` = destino; sin reponer inventario; no cuenta como cancelación |
| Pasar productos (destino nuevo) | — | abierta, en el turno y el día del origen; `entregada` si todo lo que recibe ya estaba entregado |
| Pasar productos (destino existente) | abierta, lista, mismo turno y día | igual; `entregada` si ya no le falta nada |
| Quitar piezas / lo que falta | abierta, lista | igual; `entregada` si lo vivo está entregado |
| Quitar lo que falta sin productos vivos | abierta, lista, **sin pagos** | `cancelada`, motivo fijo «Sin productos», sin reponer; cuenta como cancelación |
| Quitar lo que falta sin productos vivos | con pagos | rechazado: «Tiene pagos: hay que devolverlos primero» |
| Entregar todo | abierta, lista **con renglones vivos** | entregada |

## Qué cubre cada test de integración (bajo `appRoleStore`)

- Las cuatro tablas: grant e `inTheThreeCases` sobre sus consultas.
- FK compuesta: una fila de la empresa A no puede apuntar a un renglón, pago o pedido de la B.
- `merged_into_order_id`: no cruza empresas y solo se acepta en un pedido `cancelada`.
- La migración, contra un respaldo real restaurado en `egb027-pg` con dos empresas, con dueños y
  GRANT (nunca `--no-owner` ni `--no-privileges`); el Down se niega con cada una de sus guardas.
- `TestEveryCompanyTableIsIsolated` (llega con 021) recorre las cuatro tablas nuevas.
