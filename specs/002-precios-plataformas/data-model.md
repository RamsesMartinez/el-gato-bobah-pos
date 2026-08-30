# Data Model: venta por plataformas digitales

Tres cambios de esquema, todos aditivos. Ninguna tabla existente cambia de forma; `delivery_platforms`
gana una columna con default constante, así que las filas actuales quedan válidas sin backfill y sin
reescritura de tabla.

Los tipos están casados con los de las tablas que referencian, verificados contra la base real:
`products.id` y `modifier_options.id` son `bigint`; `delivery_platforms.id` es `smallint`;
`products.price` y `modifier_options.price_delta` son `numeric(10,2)`.

> Este documento incorpora la revisión de `db-architect` (1 crítico, 3 altos, 5 medios). Los puntos
> que cambiaron respecto al primer borrador están marcados con **[rev]**.

## 1. `delivery_platforms.price_markup_pct` (columna nueva)

| | |
|---|---|
| Tipo | `numeric(5,2) not null default 0` |
| Rango | `check (price_markup_pct >= 0 and price_markup_pct <= 500)` |
| Semilla | 35.00 para Didi, Uber Eats y Rappi. **0 para Propio** |

**Por qué `numeric(5,2)` y no `int`**: un margen de 32.5% es normal al calibrar contra los precios
publicados. El tope de 500% es cota de cordura: sin él, un dedo de más convierte un producto de $100
en uno de $600 y el error se ve hasta el ticket.

**Por qué default 0 y no 35**: el default aplica a las plataformas que alguien cree después, y una
plataforma nueva que empieza cobrando 35% más sin que nadie lo pidiera es una sorpresa cara.

**[rev] El `update` de la semilla corre como owner, así que RLS NO aplica** y siembra las filas de
todas las empresas — que es justo lo buscado hoy. Hay que decirlo en el comentario o el siguiente
lector asume que está acotado al tenant. **Y no cubre empresas futuras**: `provisionCompany` no
siembra lookups per-tenant, así que una empresa creada después toma el default 0. FR-003 se cumple
para el tenant actual; no es una garantía general.

**Sin desbordamiento** (verificado con el catálogo real): al tope de 500%, el producto más caro
(434.98) llega a 2,609.88 y el delta más caro (1,510.00) a 9,060.00. `numeric(10,2)` topa en
99,999,999.99 y `MaxMoney` en 10,000,000 — cuatro órdenes de holgura.

## 2. `product_platform_prices` (tabla nueva)

El precio manual de un producto en una plataforma. **Solo existen las excepciones**: la ausencia de
fila significa "usa el calculado", que es el caso de la mayoría de los 502 productos.

| Columna | Tipo | Notas |
|---|---|---|
| `product_id` | `bigint not null` | → `products(id)` **on delete cascade** |
| `platform_id` | `smallint not null` | → `delivery_platforms(id)`, **[rev] sin `on delete`** |
| `price` | `numeric(10,2) not null` | `check (price > 0)` |
| `updated_by` | **[rev] `bigint not null`** | → `users(id)`, sin `on delete` |
| `updated_at` | `timestamptz not null default now()` | **[rev]** con trigger `set_updated_at()` |
| `company_id` | `bigint not null` | default del GUC, → `companies(id)` **on delete cascade** |

- **PK**: `(product_id, platform_id)`. **No lleva `company_id` y es correcto**: las dos columnas son
  FKs a tablas per-tenant con identity global, así que dos empresas nunca comparten un `product_id`
  ni un `platform_id`. **No es el caso de `categories_name_scope`** (migración 0036), donde el
  índice incluía el literal `0` del `coalesce(parent_id, 0)`, que sí era un valor compartido entre
  empresas. Mismo patrón que `product_channels_pkey`. Va en el comentario de la migración para que
  nadie lo reabra.
- **[rev] Índices**: `(platform_id, product_id, price)` — cubre listar la lista completa de una
  plataforma sin tocar la tabla — y `(company_id)`, que es el patrón de las 33 tablas per-tenant
  desde `0023` y el que usa RLS al pegarle `company_id = …` a toda query.
- **[rev] `platform_id` sin `on delete cascade`**: el borrador citaba `product_channels`, pero esa
  FK a `channels` es `no action` — ninguna FK a una tabla de lookup cascadea en este esquema. El
  cascade solo dispararía al borrar a mano una plataforma sin pedidos, que es exactamente el caso
  donde llevarse en silencio una lista de precios curada a mano es el resultado equivocado.
- **`on delete cascade` desde `products`**: un precio de un producto borrado no significa nada, y
  aquí no hay dinero — es catálogo, no un hecho contable.
- **[rev] `updated_by not null` y sin `on delete set null`**: las 15 FKs `*_by` del esquema son
  `no action` y `not null`, y la migración `0032` existe para deshacer dos `set null` que resultaron
  ser un bug. Además nunca dispararía: los usuarios se desactivan con `is_active`, no se borran. Lo
  único que conseguía era hacer la columna nullable, y una fila con `updated_by` en NULL rompe la
  auditoría que justifica dejar escribir a un cajero.
- **`price > 0` y no `>= 0`**: un producto de plataforma en $0 es siempre un error de captura. Nota
  para el lector: `products_price_check` sí es `>= 0` y hay 31 productos activos en 0 (los
  "Modificadores genéricos"); que la hija sea más estricta es deliberado, no un descuido.

## 3. `modifier_option_platform_prices` (tabla nueva)

Lo mismo para los extras, con las mismas correcciones.

| Columna | Tipo | Notas |
|---|---|---|
| `option_id` | `bigint not null` | → `modifier_options(id)` **on delete cascade** |
| `platform_id` | `smallint not null` | → `delivery_platforms(id)`, sin `on delete` |
| `price_delta` | `numeric(10,2) not null` | `check (price_delta >= 0)` |
| `updated_by` | `bigint not null` | → `users(id)`, sin `on delete` |
| `updated_at` | `timestamptz not null default now()` | con trigger |
| `company_id` | `bigint not null` | default del GUC, → `companies(id)` **on delete cascade** |

- **PK** `(option_id, platform_id)`; índices `(platform_id, option_id, price_delta)` y `(company_id)`.
- **`>= 0` aquí y `> 0` en productos**: un extra sin costo ("sin cebolla") es normal y su delta es 0;
  un producto en $0 no lo es. Son reglas distintas a propósito.

## 4. [rev] RLS **y grants** — el hallazgo crítico

Las dos tablas nuevas llevan `company_id`, su índice y la política `tenant_isolation` (`using` +
`with check`), igual que las demás.

**Y llevan `grant select, insert, update, delete … to gatobobah_app`.** El `grant` de `0024` fue
`on all tables in schema public`, que es puntual: **no hay `alter default privileges`**, así que
cada tabla creada después trae su grant explícito (`0025`, `0026`, `0028`, `0029`, `0030`).

Por qué esto es crítico y no se vería a tiempo:

| | Conexión | ¿Aplican RLS y grants? |
|---|---|---|
| Dev local | `gatobobah` (owner), sin `APP_DATABASE_URL` | **No** |
| Producción | `gatobobah_app` | **Sí** |

Sin el grant, la migración pasa, los tests pasan y `make start` pasa. En producción, el primer
request que resuelva un precio de plataforma devuelve `42501: permission denied` → 500. Si la query
de precios entra sin condicional, **se cae toda venta**, no solo las de plataforma. `assertRLSEnforced`
no lo detecta: solo sondea `users`.

**Mitigación obligatoria**: un test de integración que toque las dos tablas vía `appRoleStore` (el
rol de app, con RLS real). Esos sí corren en CI y bloquean el deploy.

## 5. Regla de precio efectivo (lógica pura, en `domain`)

```
precioEfectivo(base, margenPct, manual) =
    manual                              si existe
    Round2(base × (1 + margenPct/100))  si no
```

**[rev] El `Round2` cae sobre el precio UNITARIO, al construir el mapa de productos, antes de
`BuildOrder`.** No basta con redondear el total de línea: `order_lines.unit_price` es
`numeric(10,2)` y Postgres coacciona el valor al guardarlo, mientras `LineTotal` se calcula con el
valor sin coaccionar. Medido contra el catálogo real, 12 de 215 productos activos dan un tercer
decimal con 35%:

| Producto | Base | ×1.35 | Ticket (×3) | `line_total` guardado |
|---|---|---|---|---|
| BONELESS J - 1 Kg | 434.98 | 587.2230 | 1,761.66 | **1,761.67** |
| ALITAS J - 1 Kg | 398.98 | 538.6230 | 1,615.86 | **1,615.87** |

El ticket que se pega a la bolsa no cuadraría por un centavo, en productos que el negocio sí vende.
Caso obligatorio del test table-driven: 434.98 al 35%.

Sin plataforma (mostrador) el margen no se aplica: el precio es el base, tal cual hoy.

## 6. [rev] La plataforma del pedido hay que resolverla, no confiarla

Hoy `delivery_platform_id` llega del cliente como `*int16` y entra directo a `CreateOrder`: el único
guardián es la FK, y **los chequeos de integridad referencial de Postgres saltan RLS por diseño**.
Hoy da igual porque el id es una etiqueta que nadie resuelve. Con esta feature **elige la lista de
precios**.

Un id de otra empresa, o uno que el front cacheó y ya no existe, haría que la búsqueda del margen
bajo RLS devuelva 0 filas. Si el código cayera a 0% en "no encontrado", el pedido se cobraría a
precio de mostrador en Uber Eats, el ticket saldría bien impreso y el descuadre aparecería semanas
después al conciliar el depósito.

**Regla**: el margen se resuelve con una query propia bajo RLS y se devuelve **422 si no hay fila**.
Nunca `coalesce(…, 0)`.

## 7. [rev] Borrar una excepción es parte del alcance

El modelo solo sabía escribir. Un precio **plausible y equivocado** —$14.90 donde iban $149.00— pasa
el `> 0` y `ValidMoney`, y ese producto se vendería así para siempre: la pantalla de configuración
está fuera de alcance y el `check (price > 0)` cierra el idioma "pon 0 para limpiar".

Entra al alcance el borrado de la excepción (vuelve al precio calculado). Es barato ahora y un
data-fix a mano en producción después.

## 8. [rev] Escribir un precio invalida el caché del menú

`menuTTL` es 24 h y la llave es `pos:menu:<companyID>`. Sin invalidar al escribir, otra tablet
mostraría el precio viejo hasta un día entero mientras el servidor cobra el nuevo: total impreso ≠
total cobrado, que es justo lo que este diseño quiere evitar.

**La plataforma NO entra en la llave del caché.** El documento del menú carga el precio base, el
margen de cada plataforma y el mapa disperso de excepciones; así sigue habiendo una entrada por
empresa en vez de cuatro, con una sola invalidación.

## Lo que NO cambia

- **`order_lines` y `order_line_modifiers`**: ya guardan `unit_price`, `price_delta` y `unit_cost`
  como *snapshot*. Un ticket viejo sigue mostrando lo que se cobró aunque la lista cambie después.
- **`orders`**: ya tiene `delivery_platform_id`. El check `orders_check` (solo con
  `service_type='domicilio'`) se conserva: un pedido de plataforma es a domicilio.
- **Costos e inventario**: el margen es de **precio de venta**. `unit_cost`, las recetas y la
  depleción no se tocan — vender por Uber descuenta exactamente lo mismo que en mostrador (FR-017).
  El margen de utilidad calculado sale mayor, que es correcto: eso es lo que se va en comisión.
- **Métodos de pago**: ya existen los tres con `kind='plataforma'`.

## Riesgo residual aceptado

Nada fuerza que `company_id` de estas tablas coincida con el de `products`, y como los chequeos de
FK saltan RLS, el tenant A podría insertar una fila apuntando a un producto de B: quedaría invisible
para B e inútil para A. Cerrarlo cuesta un `unique (id, company_id)` en `products` más FK compuesta
— **no se hace en esta migración**: más radio de impacto que el bug que evita, y es el mismo estado
que ya tienen `product_channels` y `order_lines`.
