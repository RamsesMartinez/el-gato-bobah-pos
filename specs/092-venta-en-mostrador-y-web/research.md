# Research: 092 — dónde se vende cada producto, vender sin existencia, precio web y Configuración

Medido sobre el respaldo de producción del 2026-11-23 01:39 restaurado en local (`make db-restaurar`,
dos empresas). Cada decisión dice qué se eligió, por qué y qué se descartó.

## R-1. Dónde guardar «dónde se vende»: el catálogo de lugares de venta que ya existe

**Decision**: reusar `channels` (por empresa, códigos `pos`, `qr`, `online`) y `product_channels`
(`product_id`, `channel_id`, `visibility`), las dos de la 0004. **Mostrador = `pos`**, **Web =
`online`**. `qr` se queda sin uso (el QR apunta al mismo menú web; ver Assumptions del spec). La
regla «sin fila = se ofrece» ya existe y es la que usa el menú del POS (`MenuProducts`).

**Rationale**:
- Las dos tablas ya tienen RLS, `company_id`, llave compuesta producto↔empresa (0041), trigger de
  versión de catálogo (0093) y la copia al duplicar (`CloneProductChannels`). Una columna
  `products.sold_on_web` repetiría lo que ya existe y cerraría la puerta «una lista de productos por
  plataforma»: cada lugar de venta nuevo sería otra columna.
- **0 filas** en `product_channels` hoy → todo producto existente queda en Mostrador y Web sin
  migrar un solo dato (FR-002), y lo mismo cualquier producto nuevo.
- Un lugar de venta nuevo (cada plataforma, una sucursal) es una fila en `channels` o una columna
  nullable, nunca reescribir decisiones guardadas (FR-007). Ver data-model, «Puerta».

**Alternatives considered**:
- `products.sold_at_counter` / `products.sold_on_web` booleanos: más simple de leer, pero cada lugar
  nuevo es una migración y la plataforma no cabe. Descartada por la puerta del principio VIII.
- Tabla nueva `product_sales_channels`: duplicaría `product_channels` sin ganar nada.

**Hueco encontrado**: una empresa creada desde la consola (`CreateCompany`) **no** recibe filas en
`channels`. Hoy no importa (nadie escribe `product_channels`), pero en cuanto alguien apague
«Web» en esa empresa no habría a qué canal apuntar. La migración inserta los tres códigos donde
falten y `CreateCompany` los crea al nacer la empresa (FR-008).

## R-2. Precio web: una columna en la decisión por lugar de venta

**Decision**: `product_channels.price numeric(12,2) null`, con `check (price > 0)`. Null = «hereda
el de mostrador». Precio efectivo en un lugar = `coalesce(pc.price, p.price)`, calculado en un solo
lugar (`domain.ChannelPrice`).

**Rationale**: el precio pertenece a «producto en un lugar de venta», igual que la visibilidad. La
misma columna sirve mañana para el precio de otro lugar. El precio de plataforma
(`product_platform_prices`) **no** se toca: es margen sobre el de mostrador y su propia regla, y el
dueño pidió explícitamente no usar ese selector (FR-019).

**Alternatives considered**: `products.web_price` (otra columna por lugar, misma objeción que R-1);
reusar `product_platform_prices` con una «plataforma Web» falsa (contamina el corte por plataforma y
los métodos de pago de plataforma).

## R-3. «Vender sin existencia»: `products.allow_oversell` sirve; sus valores no

**Decision**: usar `products.allow_oversell` (0004, `not null default true`). La migración pone
`true` en todos los productos existentes.

**Medido**:

| Empresa | `track_stock` | `allow_oversell=false` |
| --- | --- | --- |
| gatobobah | sí | 23 |
| gatobobah | no | 155 |
| bobah-pruebas | sí | 30 |
| bobah-pruebas | no | 144 |

Los `false` vienen de `fudo-import` (`boolSi(r["Vender sin stock"])`): reflejan una configuración de
FUDO que este sistema nunca aplicó. Ninguna regla de venta lee la columna (solo `fudo-import` y la
copia al duplicar).

**Rationale**: aplicar esos valores el día de la subida dejaría «Agotado» productos que hoy se venden
sin problema, contra existencias que el negocio no ha conciliado desde FUDO (memoria
«inventario de aquí en adelante»). Ponerlos en `true` es exactamente el comportamiento de hoy, y el
dueño decidió que todo producto nace con la opción encendida. Lo que se pierde (un valor de FUDO
nunca usado) sigue en el export original fuera del repositorio.

**Down**: no restaura los `false` (no hay de dónde). La guarda no aplica: el Down no se niega; se
documenta en la migración que el Down solo quita la columna de precio.

## R-4. Qué es «Agotado»: lo que el producto descuenta, con la regla que ya decide qué sale

**Decision**: un producto está agotado si `allow_oversell = false` y, en la sucursal, algún renglón
que `domain.ExpandSale` descuenta al vender **uno** del producto **sin extras** tiene existencia
menor que lo que pide. Una función pura nueva en `domain` (`SoldOut`) recibe el grafo de existencias
y los niveles; la usa la lista de agotados y la validación de la venta.

**Rationale**: medido el 2026-11-23, lo que se compra hecho (refrescos, botellas) es **receta de un
insumo por pieza** (spec 037: «la existencia propia ya no se ofrece»). Limitar «Agotado» a
`track_stock` dejaría fuera justo el caso del ejemplo del dueño. `ExpandSale` es la única función que
decide qué sale de almacén; usarla evita una segunda definición de «existencia del producto».

**Alternatives considered**: solo `track_stock` (deja fuera los refrescos); contar extras elegidos
(depende de la elección; queda fuera de alcance, se agrega después al mismo costo).

## R-5. Cómo se entera Vender: lista de agotados aparte del menú

**Decision**: `GET /pos/menu/sold-out` → `{"productIds": [..]}`, sin caché, que Vender pide cada
60 s junto con la revisión del menú y al volver al frente. Si no hay ningún producto con
`allow_oversell = false`, responde `[]` sin cargar el grafo.

**Rationale**: el menú va en Redis con `ETag` por versión de catálogo (spec 034); la existencia
cambia con cada venta y **no** sube esa versión. Meter el agotado en el documento obligaría a
invalidar el menú en cada venta. Una lista corta y aparte cuesta una consulta por minuto por tableta.

**Alternatives considered**: invalidar el menú en cada movimiento de almacén (rompe el 304 que hace
barata la revisión); aviso en vivo por SSE (no llega con la tableta suspendida, el mismo problema que
la spec 034 resolvió con la revisión por minuto).

## R-5b. Una sola puerta a «la existencia con la que se vende» (coordinación con #65)

**Decision**: `app.salesStock(ctx, q, channel) (domain.StockLevels, error)` es la única lectura de
existencias para vender: hoy devuelve los niveles de `stock_levels` de la sucursal (la de
`current_branch_id()` o la del pedido), y con el #65 devolverá los del almacén de venta. La lista de
agotados (R-5) y el chequeo de la venta (R-6) solo la llaman a ella; el chequeo bloquea con `FOR
UPDATE` dentro de la misma función (variante `lockSalesStock`) para que el candado también cambie en
un solo lugar.

## R-6. Dónde se rechaza la venta: en la transacción del pedido, con candado

**Decision**: `createInTx` y `addLinesInTx` (los dos caminos que escriben renglones de mostrador; el
envío de una cuenta en captura pasa por `createInTx`) llaman a una función `checkStock` antes de
descontar: suma lo que piden los renglones de productos con `allow_oversell = false`, bloquea las
filas de `stock_levels` de esos renglones en la sucursal del pedido con un solo `select … for
update order by item_type, product_id, ingredient_id` (`stock_levels` no tiene `id`) y compara con
el `on_hand` que devuelve ese mismo select (en READ COMMITTED relee la versión nueva tras esperar).
Corre **antes** de cualquier `descontarRenglon`, para que ninguna transacción tome un candado de
existencia por el trigger sin haber tomado antes todos los del chequeo. Sin fila = 0: dos tabletas
rechazan igual. Error de dominio `ErrOutOfStock` (con nombre y lo que
queda) → **409 `OUT_OF_STOCK`**. Solo aplica a pedidos sin plataforma (FR-014).

**Rationale**: dos tabletas vendiendo la última pieza: el candado serializa y la segunda ve la
existencia ya descontada. El descuento de almacén ya ocurre en la misma transacción (trigger sobre
`stock_movements`), así que el chequeo y el descuento no pueden separarse.

**Alternatives considered**: validar en `prepareCreate` (fuera de la tx: carrera), o un `check
(on_hand >= 0)` en la base (rompería la venta con la opción encendida, que debe dejar negativo).

## R-7. «Oculto en mostrador» también lo rechaza el servidor

**Decision**: `GetPricedProducts` trae `sold_at_counter`; en un pedido sin plataforma, un producto
oculto en mostrador se trata como no disponible (`ProductUnavailable` → `MENU_CHANGED`, el mismo
camino que un producto archivado). Un pedido de plataforma no lo mira.

**Rationale**: la pantalla ya lo esconde; el servidor es la barrera (constitución V). `MENU_CHANGED`
hace que la tableta recargue el menú, que es lo correcto si el producto se apagó mientras estaba en
la cuenta.

## R-8. Contrato de escritura: campos opcionales en el PATCH del producto

**Decision**: `PATCH /admin/products/{id}` y `POST /admin/products` aceptan un objeto opcional
`sales` (`counter`, `web`, `webPrice`, `allowOversell`, `description`). **Ausente = no se toca.**

**Rationale**: durante la subida azul/verde y mientras una tableta tenga el front viejo en caché, el
front viejo manda el PATCH sin `sales`; si ausente significara «apagado» o «sin precio», borraría
decisiones (el defecto del PATCH parcial de la spec 090). Una sola petición mantiene «Guardar»
atómico.

## R-9. Configuración: menú lateral dentro de `/negocio`, sección en la URL

**Decision** (maqueta elegida por el dueño, tercera vuelta): `BusinessSettingsPage` se parte en
secciones con tres vistas —A menú lateral con nombres (por omisión), B menú de íconos, C índice— que
se cambian con un selector fijo en la barra de título. La vista se recuerda por tableta; la sección
abierta va en `?seccion=` (FR-024). No cambia ninguna ruta ni permiso; cada sección es un componente
con los bloques que hoy existen:

| Sección | Bloques de hoy | Permiso para verla |
| --- | --- | --- |
| Negocio | Identidad de la empresa, Zona horaria, Instalar en esta tablet | `business.configure` |
| Venta en mostrador | Nombres de los pedidos, Pedidos entregados en pantalla, Costo de envío | `business.configure` |
| Venta en web | Resumen: productos en la web y con precio propio, acceso a Menú | `business.configure` |
| Caja | Métodos de cobro, Caja (enlace a administrar), Contar sin ver lo esperado | `business.configure` |
| Inventario | Almacén (`InventorySettingsSection`) | `inventory.configure` |
| Personal | Asistencia (`AttendanceSettingsSection`) | `attendance.configure` |

Se descartan «Avisos» y «Apariencia» del issue: no hay ajustes de empresa de esos temas hoy
(Interfaz es por tableta, en `/apariencia`). Se agregan cuando haya qué poner (VI).

## R-10. Investigación de mercado

Ver la sección «Investigación de mercado» del spec.
