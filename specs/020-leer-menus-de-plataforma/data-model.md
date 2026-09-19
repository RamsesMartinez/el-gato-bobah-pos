# Phase 1 — Modelo de datos

**Feature** [020](spec.md) · Migración **`0071_menus_de_plataforma.sql`** · La última aplicada es
la 0070.

Cuatro tablas y dos enums. Ninguna toca una tabla existente: no hay `alter table` sobre nada que
tenga dinero apuntándole.

```text
delivery_platforms ──┐
                     ├── platform_connections ──┬── platform_menu_reads ── platform_menu_items
companies ───────────┘                          └── platform_item_links ── products
```

---

## Enums

```sql
create type platform_read_status as enum ('en_curso', 'ok', 'fallida');
create type platform_item_kind   as enum ('platillo', 'grupo', 'opcion');
```

`en_curso` es un estado de verdad y no un `null`: la lectura corre en una goroutine y la pantalla
tiene que poder decir «se está leyendo» sin confundirlo con «nunca se ha leído» (FR-003).

**`platform_item_kind` tiene tres valores porque el menú tiene tres niveles**, y los dos lados los
modelan igual — verificado contra el menú real el 2026-09-15:

| | POS | Uber |
| --- | --- | --- |
| Platillo | `products` | item referenciado desde una categoría |
| Grupo | `modifier_groups` | `modifier_group_ids[]` del item |
| Opción | `modifier_options` | `modifier_options[]` del grupo, que a su vez **es un item** |

Dos cosas medidas que el modelo tiene que aguantar:

- **La misma opción vive en varios grupos.** `Lemon_Pepper` está en «Elige tu salsa favorita» y en
  «Elige salsas extra». Por eso la pareja se guarda por `external_id` y **no** por (grupo, opción):
  una opción es una sola cosa, aparezca donde aparezca.
- **Hay anidamiento real**: 2 opciones cuelgan sus propios subgrupos. Uber es la única de las cuatro
  grandes que lo soporta, y «Arma tu Crepa» cae justo ahí.

`grupo` puede quedar sin usar en el MVP —se empareja platillo y opción, que es lo que un pedido
necesita para entrar solo— pero **el valor existe desde el día uno**: agregarlo después obligaría a
decidir de qué clase eran las filas ya escritas, y eso no se puede saber más que adivinando.

---

## 1. `platform_connections` — una tienda en una plataforma

Es la corrección que motivó rehacer el diseño: **una empresa va a tener varias tiendas**, cada una
distinta arriba.

| Columna | Tipo | Notas |
| --- | --- | --- |
| `id` | `bigint identity` | PK |
| `delivery_platform_id` | `smallint not null` | → `delivery_platforms(id)`, **sin `on delete`** |
| `external_store_id` | `text not null` | El id de la tienda del lado de la plataforma, **completo y sin normalizar**. Uber da un UUID; DiDi un entero de 19 dígitos. Nunca un número |
| `label` | `text not null` | Cómo la llama quien administra. Nadie distingue dos sucursales por UUID |
| `is_active` | `boolean not null default true` | |
| `created_at` | `timestamptz not null default now()` | |
| `company_id` | `bigint not null default current_setting('app.company_id',true)::bigint` | → `companies(id) on delete cascade` |

```sql
constraint platform_connections_tienda
  unique (company_id, delivery_platform_id, external_store_id)
```

**El `external_store_id` en la llave única no es adorno: es la puerta.** `unique (company_id,
delivery_platform_id)` sería el *«único por `company_id` que en realidad debería ser por
sucursal»* que la constitución nombra en su tabla de puertas abiertas, y la segunda sucursal no
entraría.

`branches` no existe todavía. Cuando exista: `alter table ... add column branch_id bigint null
references branches(id)`. Ninguna fila se reparte, porque cada conexión ya sabe de qué tienda es.

**Sin `on delete` hacia `delivery_platforms`**, igual que
[0037](../../server/migrations/0037_platform_prices.sql): borrar una plataforma del catálogo no
debe llevarse en silencio la conexión ni el emparejamiento que costó una sesión de trabajo.

Checks: `char_length(external_store_id) between 1 and 200` y `char_length(label) between 1 and 60`.

---

## 2. `platform_menu_reads` — una foto, con su resultado

Es lo que permite contestar «de cuándo es esto» (FR-002) y distinguir los tres estados que una
pantalla mal hecha muestra igual (FR-003).

| Columna | Tipo | Notas |
| --- | --- | --- |
| `id` | `bigint identity` | PK |
| `connection_id` | `bigint not null` | → `platform_connections(id) on delete cascade` |
| `started_at` | `timestamptz not null default now()` | |
| `finished_at` | `timestamptz` | `null` mientras corre |
| `status` | `platform_read_status not null default 'en_curso'` | |
| `item_count` | `integer` | Cuántos items trajo |
| `failure_kind` | `text` **con `check`** | **La clase de fallo, jamás el mensaje crudo.** Ver abajo |
| `company_id` | … | igual que arriba |

```sql
constraint platform_menu_reads_coherente
  check ((status = 'en_curso') = (finished_at is null)),
-- FR-005 EN EL ESQUEMA, no en un `if`. Una lectura que vuelve sin productos NO es un menú vacío:
-- es una lectura que no sirve, y tratarla como dato válido propondría borrar el menú entero.
constraint platform_menu_reads_ok_trae_items
  check (status <> 'ok' or coalesce(item_count, 0) > 0),
-- LA LISTA CERRADA ES EL CONTROL DE SEGURIDAD, no la prosa que la describe.
constraint platform_menu_reads_clase_de_fallo
  check (failure_kind is null or failure_kind in (
    'sin_credenciales', 'auth_rechazada', 'tiempo_agotado',
    'respuesta_invalida', 'menu_vacio', 'menu_truncado'))
```

**`failure_kind` es una clase, no un texto de la plataforma.** El mensaje crudo de una API ajena
puede traer la dirección completa adentro, y DiDi manda su `app_secret` **en el query string** —
guardarlo sería escribir un secreto en la base por un camino que nadie está mirando (FR-022).

**Por eso el `check` no es cosmético.** De esta columna depende una garantía de seguridad, y una
garantía que vive solo en un comentario se rompe la primera vez que alguien agrega una rama nueva
al servicio y mete un `err.Error()` ahí. El `check` no se olvida. *(Añadido tras la revisión de
arquitectura: la primera versión declaraba la columna como `text` a secas.)*

| Valor | Cuándo |
| --- | --- |
| `sin_credenciales` | La plataforma no tiene credenciales configuradas |
| `auth_rechazada` | El token fue rechazado (401/403) |
| `tiempo_agotado` | Se venció el contexto de la lectura |
| `respuesta_invalida` | Llegó algo que no es el menú esperado |
| `menu_vacio` | Respondió bien y sin productos — FR-005 |
| `menu_truncado` | La plataforma cortó la lista. **Es el edge case explícito del spec**: una lectura truncada comparada como completa reporta como «falta arriba» lo que sí está. Tiene valor propio y no cae en `respuesta_invalida` porque la acción es distinta — se vuelve a leer paginando, no se revisa la credencial |

```sql
create index platform_menu_reads_recientes
  on platform_menu_reads (company_id, connection_id, started_at desc);
```

Empieza por `company_id` porque RLS agrega ese predicado a toda consulta del rol `gatobobah_app`
(AGENTS.md §1). La consulta que sirve es «la última lectura `ok` de esta conexión».

---

## 3. `platform_menu_items` — lo que la plataforma publicaba en ese momento

| Columna | Tipo | Notas |
| --- | --- | --- |
| `read_id` | `bigint not null` | → `platform_menu_reads(id) on delete cascade` |
| `external_id` | `text not null` | **Tal como lo entrega la plataforma** (FR-020) |
| `kind` | `platform_item_kind not null` | |
| `name` | `text not null` | |
| `price_cents` | `bigint not null` | **Centavos, como vienen.** Ver abajo |
| `available` | `boolean not null` | |
| `company_id` | … | |

```sql
primary key (read_id, external_id)
```

**La PK no lleva `company_id`, y es correcto** — mismo razonamiento que la de
`product_platform_prices` en [0037](../../server/migrations/0037_platform_prices.sql): `read_id`
es identity global sobre una tabla per-tenant, así que dos empresas nunca comparten uno.
Prefijarlo costaría 8 bytes por fila sin ganar selectividad.

**El precio se guarda en centavos enteros, tal como los entrega Uber.** Convertir a pesos aquí
obligaría a elegir un redondeo en la frontera de la base, que es el lugar donde menos se ve. La
conversión ocurre **una sola vez**, en `domain`, con su prueba (principio III).

**Si un item se referencia desde una categoría Y desde un grupo de modificadores, gana
`platillo`.** Medido el 2026-09-14 en la tienda real: **cero casos** de los 233. Se documenta
porque el menú de Uber es un grafo plano y no lo impide, y la consecuencia —que ese item no aparezca
como opción— es aceptable mientras esta feature no empareje opciones. Deja su prueba unitaria.

Checks: `price_cents >= 0` y `char_length(external_id) between 1 and 200`.

### Retención

Los items cuelgan de la lectura, y las lecturas se podan con una constante en `domain`
(`RetencionDeLecturasEnDias`), igual que `RetencionDeToquesEnDias` de la 019. Volumen medido: 233
items por lectura; con una lectura diaria y 92 días son ~21 mil renglones por conexión.

Sobrescribir «el menú actual» habría sido más simple y habría perdido para siempre qué publicaba la
plataforma antes — un hecho que no se reconstruye (principio VIII).

**La poda SIEMPRE conserva la lectura más reciente de cada conexión, sin importar su edad.**
*(Añadido tras la revisión de arquitectura.)* Sin esa excepción, una conexión que nadie vuelve a
leer en más de la retención se queda sin ninguna fila, y la pantalla la muestra igual que una que
nunca se leyó — dos estados que FR-003 exige distinguir y que la poda volvía indistinguibles. La
lectura vieja se marca `stale`; eso es distinto de no existir.

La consulta de poda, en consecuencia, no es `delete where started_at < corte`: excluye la última
por conexión. Deja su prueba de integración, porque el defecto solo aparece meses después.

---

## 4. `platform_item_links` — el emparejamiento

Es el trabajo manual que hace posible todo lo demás, y **el que más caro sale perder**.

| Columna | Tipo | Notas |
| --- | --- | --- |
| `connection_id` | `bigint not null` | → `platform_connections(id) on delete cascade` |
| `external_id` | `text not null` | El id de allá. **No una FK a `platform_menu_items`** |
| `kind` | `platform_item_kind not null` | |
| `product_id` | `bigint not null` | → `products(id)` **`on delete restrict`**. Ver abajo |
| `local_kind` | `text not null default 'producto'` | Qué es el lado de ACÁ. Ver la puerta de profundidad |
| `confirmed_at` | `timestamptz` | `null` = **propuesta**, no hecho (FR-011) |
| `confirmed_by` | `bigint` | → `users(id) on delete set null` |
| `created_at` | `timestamptz not null default now()` | |
| `company_id` | … | |

```sql
primary key (connection_id, external_id),
constraint platform_item_links_confirmador
  check (confirmed_by is null or confirmed_at is not null)
```

Cómo cada requisito cae del esquema, sin código que lo vigile:

- **FR-012** (un item de la plataforma no puede apuntar a dos productos): es la PK.
- **FR-010** (un producto del POS puede ser varios items arriba): sale gratis — varias filas
  comparten `product_id`. Es el caso real: `Chamoyada` del POS son **12** chamoyadas en Uber.
- **FR-014** (sobrevive a que cambie el nombre de cualquiera de los dos lados): la llave son ids,
  nunca nombres.
- **FR-013** (deshacer): borrar la fila.

### El detalle que hace o rompe esta feature: el emparejamiento tiene DOS lados por donde se borra

Esta tabla es trabajo humano —una sesión completa por plataforma— y no se puede reconstruir. Tiene
dos vecinos que podrían llevárselo, y **los dos hay que cerrarlos**. La primera versión de este
documento cerró uno con mucho énfasis y dejó el otro abierto; lo encontró la revisión de
arquitectura.

**Lado 1 — la foto.** `platform_item_links` **NO referencia `platform_menu_items`**: guarda
`external_id` como texto, colgado de la **conexión**. Si colgara de la foto, podar lecturas viejas
se llevaría el emparejamiento en silencio y semanas después, con la pantalla reportando de pronto
que el 100% del menú difiere.

**Lado 2 — el producto.** `product_id` va con **`on delete restrict`**, no `cascade`. El argumento
para el cascade era que un enlace a un producto inexistente es basura; el argumento en contra es
más fuerte:

- **En este repo los productos SÍ se borran**, no solo se desactivan. El patrón de reorg de datos
  documentado en AGENTS.md §6 lo hace: [docs/reorg/14_rollback.sql](../../docs/reorg/14_rollback.sql)
  corre `delete from products where name = 'Galleta Oreo Crepa'`.
- **El precedente correcto ya estaba en el esquema**: `order_lines.product_id` referencia
  `products(id)` **sin `on delete`** ([0007_orders.sql](../../server/migrations/0007_orders.sql)),
  por esta misma razón — el hijo es precioso y no debe desaparecer solo.

Con `restrict`, un reorg que de verdad necesite borrar un producto emparejado **falla ruidoso** y
tiene que decidirlo y escribirlo en su script, en vez de heredar el borrado gratis.

Lo cubren dos pruebas de integración, y las dos tienen que verse en rojo primero:

- `TestElEmparejamientoSobreviveALaPoda` — crea parejas, poda lecturas, verifica que siguen.
- `TestBorrarUnProductoEmparejadoFalla` — intenta `delete from products` sobre uno con pareja y
  espera el error de integridad, nombrando cuántas parejas protegía.

### La puerta de profundidad (FR-014b)

`local_kind` nace con un solo valor, `'producto'`, y **es exactamente el tipo de columna que el
principio VIII manda dejar puesta**: el día que exista inventario de insumos, un platillo de la
plataforma podrá corresponder a una receta o a un insumo, no a un producto del catálogo. Agregar la
columna después obliga a decidir qué eran las filas viejas; ponerla hoy cuesta una palabra.

Lo que **no** se construye hoy, a propósito: ni la tabla de recetas, ni la FK polimórfica, ni la
pantalla. Solo el hueco donde va.

```sql
create index platform_item_links_por_producto on platform_item_links (company_id, product_id);
-- Índice LISO por product_id, además del anterior. No es duplicado: el chequeo de `restrict` que
-- dispara un `delete from products` NO lleva `company_id` en su predicado —las comprobaciones de
-- integridad referencial de Postgres saltan RLS, verificado contra Postgres real en esta
-- revisión— así que el índice compuesto que empieza por `company_id` no lo sirve.
create index platform_item_links_producto on platform_item_links (product_id);
```

El compuesto sirve la vuelta «qué items de arriba corresponden a este producto», que es la que
necesita la comparación; el liso sirve el chequeo de integridad. Son dos lecturas distintas.

---

## Seguridad — igual en las cuatro tablas

```sql
alter table <t> enable row level security;
create policy tenant_isolation on <t>
  using      (company_id = current_setting('app.company_id', true)::bigint)
  with check (company_id = current_setting('app.company_id', true)::bigint);
grant select, insert, update, delete on <t> to gatobobah_app;
```

- **El `grant` no es opcional.** El de 0024 fue puntual y una tabla nueva no hereda nada; falta uno
  y en producción sale `42501` en el primer request mientras en dev nunca, porque la API de
  desarrollo se conecta como owner. Lo prueba el test de integración bajo `appRoleStore`.
- **`delete` sí va**, a diferencia de la 0070: hay que poder deshacer un emparejamiento (FR-013) y
  podar lecturas viejas.
- **`gatobobah_platform` NO recibe nada.** Esto es administración del catálogo del restaurante, no
  de la consola de quien vende el sistema. Una tabla que el rol de plataforma no necesita, no se le
  concede.
- `set local lock_timeout = '3s'` al inicio de la migración.

**Las cascadas de FK saltan RLS, y está medido.** La revisión de arquitectura lo verificó contra
Postgres real: dos tablas con la misma política `tenant_isolation`, una fila hija de otra empresa
—invisible a un `select` de esa sesión— y el `delete` del padre como `gatobobah_app` **sí la
borró**.

Aquí no abre una fuga: `platform_connections` ya está aislada por RLS, así que nadie alcanza a
borrar la conexión de otra empresa para empezar. Lo que sí cambia es qué índice sirve: el `delete`
interno del cascade no lleva `company_id` en su predicado, así que un índice que empieza por esa
columna no lo ayuda. Con el volumen de hoy da igual; se anota para que nadie lo descubra midiendo
un plan raro dentro de un año.

## Lo que la migración NO hace

- **No siembra ninguna conexión.** Una fila con el `store_id` de este negocio sembrada por una
  migración vive en este repositorio, que es **público** — misma razón por la que
  `platform_operators` nace vacía (0068). Las conexiones se dan de alta desde la pantalla.
- **No toca `products`, `delivery_platforms` ni `product_platform_prices`.**

## Lo que NO se guarda, a propósito

| | Por qué |
| --- | --- |
| Las diferencias | Se calculan al pedirlas, de la última lectura `ok` y las parejas. Guardarlas sería una tercera copia que se desincroniza |
| Credenciales de la plataforma | Por env, validadas al arranque (FR-021). El `store_id` **no** es secreto y por eso sí va en tabla |
| Categorías y grupos de la plataforma | Esta feature compara platillos. 16 de los 65 platillos de Uber están en más de una categoría, así que la categoría ni siquiera identifica |
| `price_info.overrides` interpretado | Se guarda el precio base. Interpretar un precio con contexto contra un precio del POS sin contexto reporta diferencias que no son |
