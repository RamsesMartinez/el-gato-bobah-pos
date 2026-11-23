# Data model: 092

Una sola migración, **0117** (`0117_product_channel_price.sql`), aditiva. Si otra rama llega antes
a develop con 0117, se renumera a la siguiente libre.

## Lo que ya existe y se reusa

| Tabla | Columnas que importan | Desde |
| --- | --- | --- |
| `channels` | `id smallint`, `company_id`, `code text` (único por empresa), `name` | 0002 / 0023 |
| `product_channels` | `product_id`, `channel_id`, `visibility channel_visibility ('visible','oculto')`, `company_id`; PK `(product_id, channel_id)`; FK compuesta al producto de la misma empresa | 0004 / 0041 |
| `products.allow_oversell` | `boolean not null default true` | 0004 |
| `products.description` | `text` null — «descripción para el cliente» | 0004 |

Lugares de venta de esta feature: **Mostrador = `channels.code = 'pos'`**, **Web = `'online'`**.

## Cambios de 0117

### Up

1. `alter table product_channels add column price numeric(12,2)` — null = hereda el precio de
   mostrador.
   - `check (price is null or price > 0)` con nombre `product_channels_price_positive`.
   - Nullable y sin default: la versión anterior de la API (que inserta en `product_channels` solo
     al duplicar, con columnas explícitas) sigue funcionando sin tocarla.
2. Catálogo de lugares de venta para toda empresa que no lo tenga:
   `insert into channels (company_id, code, name) select c.id, v.code, v.name from companies c
   cross join (values ('pos','Mostrador'),('qr','Menú QR'),('online','Web')) v(code,name)
   on conflict (company_id, code) do nothing`.
   - Las filas existentes de `pos` y `online` se renombran a «Mostrador» y «Web» (el nombre no lo
     lee ningún código hoy; es el que verá una pantalla futura de lugares de venta).
3. `update products set allow_oversell = true where not allow_oversell` (research R-3). Toca 352
   filas medidas en las dos empresas; el trigger de sentencia de 0093 sube la versión de catálogo una
   vez por empresa. Sin bloqueo largo: una sola sentencia sobre ~1,000 filas.

### Down

- `alter table product_channels drop column price` (cae con ella su check).
- No borra las filas de `channels` que agregó: otras filas pueden ya apuntarles (`product_channels`
  con FK). Dejarlas no rompe a la versión anterior.
- No restaura los `allow_oversell = false` (no hay de dónde; ver R-3). No se niega con datos, así
  que no necesita renglón en `guardedDownCleanup`.

### Tolerancia a la versión anterior corriendo (constitución IV)

| Qué hace la versión anterior durante el traslape | ¿Se rompe? |
| --- | --- |
| Lee el menú (`MenuProducts`: `not exists ... visibility = 'oculto'`) | No: la columna nueva no la lee |
| Duplica un producto (`CloneProductChannels`, columnas explícitas) | No: `price` queda null en el duplicado durante esos segundos (el de una fila sin precio) |
| Crea productos y pedidos | No: `allow_oversell` no lo lee |

## Reglas (dominio, sin I/O)

- `domain.ChannelPrice(base decimal, override *decimal) decimal` — `override` si existe, si no
  `base`.
- `domain.ValidChannelPrice(*decimal) error` — null válido; si no, `> 0`, 2 decimales,
  `ValidMoney`.
- `domain.SoldOut(graph StockGraph, levels StockLevels, productID) bool` — `ExpandSale` de una
  unidad sin extras; agotado si algún renglón pide más de lo que hay (R-4).
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

**Snapshot**: el pedido de mostrador guarda su precio en `order_lines.unit_price` como hoy. Un
pedido web (#328) guardará el precio web efectivo en el mismo campo; el precio web **no** se
reescribe en el pasado al editarlo.
