# Data model: 092

Una sola migración, **0118** (la 0117 la tomó la 091, fotos del catálogo) (`0118_product_channel_price.sql`), aditiva. Si otra rama llega antes
a develop con 0117, se renumera a la siguiente libre.

## Lo que ya existe y se reusa

| Tabla | Columnas que importan | Desde |
| --- | --- | --- |
| `channels` | `id smallint`, `company_id`, `code text` (único por empresa), `name` | 0002 / 0023 |
| `product_channels` | `product_id`, `channel_id`, `visibility channel_visibility ('visible','oculto')`, `company_id`; PK `(product_id, channel_id)`; FK compuesta al producto de la misma empresa | 0004 / 0041 |
| `products.allow_oversell` | `boolean not null default true` | 0004 |
| `products.description` | `text` null — «descripción para el cliente» | 0004 |

Lugares de venta de esta feature: **Mostrador = `channels.code = 'pos'`**, **Web = `'online'`**.

## Cambios de 0118

### Up

Empieza con `set local lock_timeout = '3s';` (como 0041 y 0093): los `alter` toman candados de
metadatos y no deben encolar detrás de una transacción larga las lecturas del menú.

1. **Ancho de `channels.id`** (revisión de arquitectura, M2): `channels.id`,
   `product_channels.channel_id` y `category_channels.channel_id` pasan de `smallint` a `bigint`
   (con la secuencia de identidad `as bigint`). Hoy son 6 filas y 0 filas; con un id global por
   empresa y por lugar de venta, `smallint` topa en miles de empresas. Ninguna consulta de la
   versión anterior escanea esos ids en Go (el menú filtra por `code`, el duplicado copia por SQL).
2. **FK compuesta del canal** (M1): `unique (company_id, id)` en `channels` y
   `product_channels (company_id, channel_id) references channels (company_id, id)`, `not valid` y
   después `validate` (0 filas). Los chequeos de integridad saltan RLS: sin esto, un arreglo de datos
   como owner podría ligar el producto de una empresa al canal de otra.
3. `alter table product_channels add column price numeric(12,2)` — null = hereda el precio de
   mostrador; `check (price is null or price > 0)` con nombre `product_channels_price_positive`.
   Nullable y sin default: el insert de la versión anterior (`CloneProductChannels`, columnas
   explícitas) sigue funcionando.
4. **Catálogo de lugares de venta para toda empresa** (A1): `insert into channels (company_id,
   code, name) select c.id, v.code, v.name from companies c cross join (values
   ('pos','Mostrador'),('qr','Menú QR'),('online','Web')) v(code,name) on conflict (company_id,
   code) do update set name = excluded.name`. Renombra `pos` y `online` en las que ya los tienen.
5. **`allow_oversell = true` en todo producto** (research R-3), con `trg_products_updated`
   desactivado durante la sentencia (como 0076): sin eso, 352 productos amanecerían «editados hoy»
   en el catálogo. La lista de los que estaban en `false` no se guarda en la base: es la columna
   «Vender sin stock» del export de FUDO, que vive fuera del repositorio
   (`~/gatobobah-datos/references/`), y ninguna regla la aplicó nunca. Va en goose y no como SQL
   numerado aparte porque tiene que ocurrir en la misma subida que empieza a aplicar la columna: sin
   ella, la API nueva agotaría productos con un valor de FUDO.

### Down

- `drop column price` (cae su check). **Se pierden los precios web capturados**: es el costo
  esperado de bajar, no una guarda; el Down no se niega y no lleva renglón en `guardedDownCleanup`.
- Quita la FK compuesta y el `unique (company_id, id)`; regresa los ids a `smallint` solo si caben
  (con 6 filas, sí).
- No borra las filas de `channels` que agregó (pueden tener `product_channels` apuntándoles) ni
  restaura los nombres viejos o los `allow_oversell = false` (no hay de dónde).

### Tolerancia a la versión anterior corriendo (constitución IV)

| Qué hace la versión anterior durante el traslape | ¿Se rompe? |
| --- | --- |
| Lee el menú (`MenuProducts`: `not exists ... visibility = 'oculto'`) | No: la columna nueva no la lee |
| Duplica un producto (`CloneProductChannels`, columnas explícitas) | No: `price` queda null en el duplicado durante esos segundos (el de una fila sin precio) |
| Crea productos y pedidos | No: `allow_oversell` no lo lee |

## Escritura de «dónde se vende» (A2)

El servicio que guarda `sales` asegura el canal dentro de la misma transacción
(`EnsureChannel`: `insert … on conflict (company_id, code) do nothing` y luego `select`) antes del
upsert en `product_channels`. Si aun así no hay canal, falla con `domain.ErrNotFound` en vez de
insertar 0 filas y responder 200. El upsert apunta a la restricción por nombre
(`on conflict on constraint product_channels_pkey`) para que, cuando llegue `branch_id`, solo cambie
la restricción y no el SQL. `CloneProductChannels` copia también `price`. `CreateCompany` siembra
los tres canales. Lo prueba una empresa creada con `insert into companies` crudo (sin canales).

`AdminListProducts` lee lugar de venta y precio con **subconsultas escalares por código**, no con un
`join`: con `branch_id` un join multiplicaría filas.

## Reglas (dominio, sin I/O)

- `domain.ChannelPrice(base decimal, override *decimal) decimal` — `override` si existe, si no
  `base`.
- `domain.ValidChannelPrice(*decimal) error` — null válido; si no, `> 0`, 2 decimales,
  `ValidMoney`.
- `domain.SoldOut(graph StockGraph, levels StockLevels, productID) bool` — `ExpandSale` de una
  unidad sin extras; agotado si algún renglón pide más de lo que hay (R-4). **Sin fila de
  existencia = 0** (la fila nace con el primer movimiento).
- `domain.CheckStock(graph, levels, lines, oversell map) error` — suma por renglón de almacén lo que
  piden los productos con la opción apagada; `ErrOutOfStock` con nombre y lo que queda.
- `domain.MeasuresStock(ProductModeFacts) bool` — own_stock, recipe o package: decide si la casilla
  «Vender sin existencia» aparece (FR-011).

## Puerta «una lista de productos por plataforma» (principio VIII)

> ¿Esto se puede agregar después al mismo costo?

| Lugar de venta futuro | Cómo entra | ¿Migra datos vivos? |
| --- | --- | --- |
| Cada plataforma (Uber, DiDi, Rappi) | Una fila en `channels` por plataforma, con una columna nullable `delivery_platform_id` en `channels`. Las decisiones de Mostrador y Web no se tocan; los productos sin fila para la plataforma siguen «se ofrece» | No |
| Nombre propio del producto en un lugar | Columna nullable en `product_channels` (`name`) | No |
| Otra sucursal | Columna nullable `branch_id` en `product_channels` con un índice único parcial; null = toda la empresa, que es lo que significan las filas de hoy | No (la PK se reemplaza por un único que incluye `branch_id`, sobre filas que ya tienen null) |
| Producto uno-a-varios en una plataforma (crepa por sabor) | Ya lo cubre el emparejamiento de plataformas (`platform_*`); no es de esta tabla | — |

**Lo que esta feature no guarda y el #328 debe guardar desde su primer pedido**: en qué lugar de
venta se capturó un pedido (`orders` no lo dice). Un pedido web que no lo registre no se puede
distinguir después sin adivinar.

**Snapshot**: el pedido de mostrador guarda su precio en `order_lines.unit_price` como hoy. Un
pedido web (#328) guardará el precio web efectivo en el mismo campo; el precio web **no** se
reescribe en el pasado al editarlo.
