# Phase 1 — Modelo de datos

**Feature**: 021 · Recibir los pedidos de Uber Eats
**Migración**: `0073_pedidos_de_plataforma.sql`
**Revisado por `db-architect` el 2026-09-17**; los cambios que pidió están aplicados y marcados.

Cuatro tablas nuevas, dos índices de tenant que faltaban en tablas viejas, y un `check` de la 0007
que hay que relajar.

Todas las tablas nuevas llevan `company_id` con su default de `app.company_id`, su RLS y sus grants
para `gatobobah_app` — los grants se prueban en integración, porque el `grant` puntual de la 0024
enseñó que un `42501` en producción no se ve nunca en desarrollo.

---

## 0 · Dos índices que faltan en tablas que ya existen

**Sin esto la migración no corre.** Verificado contra Postgres real el 2026-09-17: `users` solo
tiene `users_pkey`, `users_company_username_key` y `users_pin_lookup_unico`; `platform_connections`
solo `platform_connections_pkey` y `platform_connections_tienda`. Ninguna tiene `(company_id, id)`,
así que **toda FK compuesta contra ellas falla al aplicarse**.

```sql
create unique index users_tenant_key on users (company_id, id);
create unique index platform_connections_tenant_key on platform_connections (company_id, id);
```

Mismo patrón y misma razón que `products_tenant_key` (0071) y `register_sessions_tenant_key`
(0061): los chequeos de integridad referencial de Postgres **saltan RLS**, así que una FK simple no
impide apuntar a la fila de otra empresa.

> **Nota aparte, que NO es de esta feature**: `platform_menu_reads` y `platform_item_links` de la
> 0071 tienen el mismo hueco con `connection_id`. Ahí es lectura de menú; aquí hay dinero, porque
> aceptar crea una fila en `orders`. Se nombra para que quede registrado, no se arregla en la 0072.

---

## 1 · `platform_webhook_keys` — con qué se verifica la firma

Por `(company_id, delivery_platform_id)`. **Dos llaves válidas a la vez** (FR-003).

| Columna | Tipo | Notas |
|---|---|---|
| `id` | `bigint` identity | |
| `delivery_platform_id` | `smallint not null` | **FK compuesta** con `company_id` contra `delivery_platforms (id, company_id)`, **`on delete no action` explícito**: perder en silencio una llave capturada a mano cuesta lo mismo que perder el emparejamiento |
| `key_primary` | `text not null` | El valor que se captura del tablero de Uber |
| `key_secondary` | `text` | Nullable. Solo con valor mientras dura una rotación |
| `rotated_at` | `timestamptz` | Cuándo se puso la secundaria. Delata una rotación que nadie terminó |
| `company_id` | `bigint not null` | default `app.company_id`, FK a `companies` |

- `unique (company_id, delivery_platform_id)` — una llave por app, y una app por empresa y plataforma.
- `check (char_length(key_primary) between 16 and 512)` — una llave corta es un dedazo, y un dedazo
  aquí es "ningún pedido entra" sin que nada falle.
- `check (key_secondary is null or key_secondary <> key_primary)` — dos llaves iguales no son una
  rotación, son un descuido que hace creer que hay respaldo.
- **Grants restringidos**: solo lo que el servicio necesita.

### Cifrado en reposo: NO, y por qué

Es el único secreto **recuperable** de la base (no se puede hashear: hay que recalcular el HMAC con
él). Se deja en claro con grants restringidos, porque cifrarlo con una llave del entorno mueve el
secreto, no lo protege: quien lee la base desde la aplicación tiene también el entorno. Añadiría un
camino de rotación y un modo de fallo nuevos —"la llave de cifrado cambió y ningún pedido entra"—
sin cerrar ningún ataque real. El día que exista un gestor de secretos de verdad, se mueve ahí.

---

## 2 · `platform_webhook_events` — qué llegó

Uno por entrega de la plataforma. Permite deduplicar (FR-007) y reconstruir qué pasó (FR-015).

| Columna | Tipo | Notas |
|---|---|---|
| `id` | `bigint` identity | |
| `event_id` | `text not null` | El identificador que manda Uber |
| `event_type` | `text not null` | `orders.notification`, `orders.cancel`, `store.provisioned`… |
| `connection_id` | `bigint not null` | **FK compuesta** con `company_id` |
| `received_at` | `timestamptz not null default now()` | |
| `processed_at` | `timestamptz` | NULL = llegó y no se terminó de procesar |
| `outcome` | `text` | `procesado`, `repetido`, `ignorado`, `fallido` |
| `failure_kind` | `text` | **La clase, jamás el mensaje crudo**, con `check` de lista cerrada |
| `raw_body` | `text not null` | El aviso **byte por byte**, como llegó |
| `company_id` | `bigint not null` | |

### Cambio pedido por la revisión: la tienda desconocida NO llega aquí

La versión anterior decía que un aviso de una tienda que no reconocemos "se registra igual", con
`connection_id` nullable. **No se puede**: `company_id` es `not null` y su default sale de
`app.company_id`, que en ese caso no existe — no hay empresa que fijar porque la empresa **es** el
resultado de reconocer la tienda. La fila no entraría, y el código tronaría con una violación en vez
del rechazo limpio que promete el contrato.

**Decisión**: un aviso de tienda desconocida se rechaza **antes de tocar Postgres** y deja solo su
evento de seguridad en el log. No es visible desde la pantalla de administración, y eso es correcto:
esa pantalla es de los pedidos del negocio, y ese aviso no es de ningún negocio nuestro. Se ajusta
SC-004 en consecuencia.

### `raw_body` va como `text`, no como `jsonb`

La firma se verifica sobre el **cuerpo crudo**. `jsonb` normaliza y reordena llaves, así que un
cuerpo guardado como `jsonb` **ya no se puede reverificar**. A decenas de pedidos al día, el ahorro
de poder consultarlo con operadores de JSON no compensa perder la única prueba de autenticidad que
tenemos. Si hiciera falta consultarlo, `raw_body::jsonb` funciona en la consulta.

### Deduplicación

- **`unique (event_id)` GLOBAL, no por empresa.** El identificador lo genera Uber y es único en su
  universo. Por empresa permitiría procesar el mismo aviso dos veces si el resolutor se equivocara
  de empresa — justo el caso que hay que hacer imposible.
- `check` de lista cerrada sobre `outcome` y `failure_kind`: de esa columna depende una garantía de
  seguridad, y una garantía que solo vive en Go se rompe al agregar una rama nueva.
- **`raw_body` trae datos del cliente.** Va con poda a los **60 días**: se borra el cuerpo y la fila
  se queda. El número no es arbitrario — el documento de pago de la plataforma llega mensual, y una
  discrepancia se descubre al conciliarlo; pasado ese ciclo el cuerpo crudo ya no sirve para nada y
  solo carga riesgo. La fila nunca se borra: es el rastro de auditoría.

---

## 3 · `platform_incoming_orders` — el pedido que espera decisión

Vive **fuera de `orders`** a propósito (D4). La fila de `orders` nace al aceptar.

| Columna | Tipo | Notas |
|---|---|---|
| `id` | `bigint` identity | |
| `connection_id` | `bigint not null` | **FK compuesta** con `company_id` contra `platform_connections (id, company_id)`. Sin `on delete cascade`: desconectar una tienda no debe llevarse el rastro de sus pedidos |
| `external_order_id` | `text not null` | El folio de Uber |
| `display_id` | `text` | El folio corto que ve el cliente, si la plataforma lo da |
| `placed_at` | `timestamptz` | Cuándo pidió el cliente. Nullable solo por el camino de abajo |
| `decide_before` | `timestamptz` | Cuándo expira. Es lo que pinta la pantalla (FR-026) |
| `state` | enum | `pendiente`, `aceptado`, `rechazado`, `cancelado`, `expirado` |
| `settled_at` | `timestamptz` | **Cuándo salió de pendiente.** Aplica a los cuatro estados terminales |
| `decided_by` | `bigint` | **Quién decidió.** FK compuesta a `users (id, company_id)`. Solo en `aceptado` y `rechazado` |
| `deny_reason` | `text` | El código de motivo que se le mandó a la plataforma |
| `order_id` | `bigint` | FK **compuesta** a `orders (id, company_id)`. Se llena al aceptar |
| `service_type` | `service_type` | Lo dice la plataforma: **puede ser `para_llevar`** |
| `customer_name` | `text` | Solo lo necesario para preparar y entregar |
| `total` | `numeric(10,2)` | Lo que **cobró la plataforma**. Es la verdad (FR-014) |
| `raw_detail` | `text` | **El detalle del pedido, byte por byte**, como lo devolvió `resource_href` |
| `company_id` | `bigint not null` | |

### Cambio pedido por la revisión: `raw_detail`

El plan afirmaba que guardar el aviso crudo permitía reconstruir un pedido mal mapeado. **Era
falso**: el aviso son cuatro campos y una liga; el detalle —donde vive el riesgo de que Uber mande
una forma distinta de la documentada— no se guardaba en ningún lado. Si el mapeo fallaba, no había
byte desde el cual corregir, y días después Uber puede ya no entregar ese pedido.

Es el snapshot barato del principio VIII: una columna, y cierra para siempre la puerta de "no se
puede reconstruir". Va con la misma poda de 60 días que `raw_body`, y por la misma razón.

### Cambio pedido por la revisión: `settled_at` separado de `decided_by`

`cancelado` lo hace la plataforma y `expirado` lo nota el sistema: **no hay humano detrás**. Un solo
check que ate `decided_by`, `decided_at` y "salió de pendiente" obligaría a inventar un usuario de
sistema para esos dos estados — el mismo parche que ya se rechazó con `opened_by`.

La matriz va **en el comentario de la migración**, no solo en Go:

| Estado | `settled_at` | `decided_by` | `deny_reason` | `order_id` |
|---|---|---|---|---|
| `pendiente` | NULL | NULL | NULL | NULL |
| `aceptado` | con valor | con valor | NULL | con valor |
| `rechazado` | con valor | con valor | con valor | NULL |
| `cancelado` | con valor | NULL | NULL | puede tener |
| `expirado` | con valor | NULL | NULL | NULL |

### Cambio pedido por la revisión: la cancelación que llega primero

Los avisos llegan **sin orden garantizado** (FR-008). Un `orders.cancel` puede ser el **primero** que
vemos de un pedido, y entonces nunca hubo un `GET` al detalle: no hay `placed_at`, ni
`decide_before`, ni `total` con qué llenar columnas `not null`.

Por eso las tres son **nullable**, con el check que lo acota a ese único caso:

```sql
check (state = 'cancelado'
       or (placed_at is not null and decide_before is not null and total is not null))
```

Un pedido que nació cancelado no tiene nada que decidir ni nada que cobrar: lo único que importa es
que quede registrado que existió y que la plataforma lo canceló.

### Llaves e índices

- `unique (company_id, connection_id, external_order_id)` — el mismo folio en dos plataformas es
  legítimo; repetido en la misma tienda, no.
- `(company_id, decide_before) where state = 'pendiente'` — lo único que la pantalla consulta cada
  pocos segundos. Empieza por `company_id` porque RLS agrega ese predicado a toda consulta del rol
  de la aplicación.
- `(company_id, placed_at desc)` — para la pantalla de historia (historia 4). Hoy no urge por
  volumen; agregarlo ahora es gratis y después se olvida.

---

## 4 · `platform_incoming_order_lines` — qué se pidió

| Columna | Tipo | Notas |
|---|---|---|
| `id` | `bigint` identity | |
| `incoming_order_id` | `bigint not null` | FK, `on delete cascade`: los renglones no sobreviven a su pedido |
| `parent_line_id` | `bigint` | Nullable, apunta a otro renglón: así entra una **opción** sin una segunda tabla |
| `external_item_id` | `text not null` | El id del platillo del lado de Uber |
| `external_name` | `text not null` | **Copia** del nombre que mandó la plataforma |
| `quantity` | `numeric(8,2) not null` | **Misma precisión que su destino** `order_lines.quantity`. Un staging más preciso que la tabla final solo esconde dónde se pierde el decimal |
| `unit_price` | `numeric(10,2) not null` | **Copia** del precio que cobró la plataforma |
| `product_id` | `bigint` | NULL = **sin emparejar**. FK **compuesta** con `company_id`, `on delete restrict` |
| `company_id` | `bigint not null` | |

- **`product_id` nullable es la feature, no un hueco**: un platillo sin pareja no impide aceptar
  (FR-013). Lo que sí hace es quedar contado y visible.
- `external_name` y `unit_price` son **snapshot**: editar el catálogo no reescribe el pasado.

---

## 5 · Lo que hay que cambiar de lo que ya existe

### El `check` de la 0007 que impide un pedido de plataforma para recoger

```sql
-- hoy
check (service_type = 'domicilio' or delivery_platform_id is null)
```

Un pedido de Uber **para recoger en tienda** no entra. Mientras la captura era manual nadie lo
notaba; con los pedidos entrando solos, el tipo lo dice la plataforma y la inserción revienta. Y es
el camino de prueba más barato que tenemos (D6).

Se propone admitir también `para_llevar`. **No** se quita el `check`: que un pedido de plataforma
sea de mostrador seguiría sin tener sentido.

**Dos cosas que la revisión pidió y que no estaban:**

- **`set local lock_timeout = '3s'`**. El `drop constraint` / `add constraint` toma ACCESS EXCLUSIVE
  sobre `orders`, la tabla más transaccionada del sistema. Lo hacen explícito 0040, 0041, 0042, 0061
  y 0065; esta no puede ser la excepción.
- **El `Down` no puede ser "regresar el check" a secas.** En cuanto exista un pedido `para_llevar`
  con plataforma, revertir truena. El `Down` se detiene con un mensaje claro **sin haber tocado
  nada**, como el de la 0061.

### `orders.register_session_id` ya es nullable

No hay que migrar nada: la 0007 la dejó nullable. Falta el camino que la use.

### Cómo se enlazan los pedidos aceptados sin turno — con `update` al abrir

**Decisión** (recomendada por la revisión y adoptada): al abrir un turno, un `update` dentro de esa
misma transacción reclama los pedidos de plataforma huérfanos. Es lo que mantiene el corte, los
folios y todo lo que ya filtra por `register_session_id` funcionando **sin tocar una sola consulta
existente** — el mismo criterio con el que la 0061 movió el folio de `business_date` a
`register_session_id`. Dejarlos sueltos obligaría a que cada consulta de corte repitiera el mismo
predicado, y el principio III ya advierte a dónde lleva eso.

Dos reglas que el plan no fijaba y que ahora sí:

- **Qué reclama**: los pedidos con `register_session_id is null` **cuyo `business_date` sea el del
  turno que se abre**. No todo lo huérfano sin importar antigüedad: un pedido de hace tres días
  aparecería como ingreso de hoy.
- **Dos cajas abriendo casi al mismo tiempo**: la primera se lleva todos. Hoy es correcto —hay una
  sola caja que vende— pero **no hay ningún criterio de a qué caja pertenece un pedido de Uber**, y
  el día que haya dos, esto es lo primero que hay que decidir. Queda escrito para que no se descubra
  como defecto.

---

## Transiciones de estado del pedido entrante

```
                 ┌──────────── aceptar ──────────→ aceptado   (crea la fila en orders)
                 │
   pendiente ────┼──────────── rechazar ─────────→ rechazado  (con motivo)
                 │
                 ├──── la plataforma cancela ────→ cancelado  (también puede NACER aquí)
                 │
                 └──── se agota el plazo ────────→ expirado
```

- **Todos los estados distintos de `pendiente` son terminales.** Decidir dos veces se rechaza con un
  error que lo dice (FR-020), no falla en silencio.
- `expirado` lo escribe el sistema al notar que pasó `decide_before`; **no** dispara ninguna llamada
  a la plataforma — quien cancela es Uber (FR-027, decisión del dueño).
- `cancelado` puede llegar **sobre un pedido ya aceptado**: ahí la fila de `orders` ya existe y se
  cancela por el camino que ya existe, sin borrar nada (FR-029). Y puede ser el **primer** aviso que
  vemos, por el desorden de entrega.

## Entidades que NO se crean, y por qué

- **Nada para "el pedido aceptado"**: es un `orders` normal, y esa es la mitad del punto.
- **Nada para los motivos de rechazo**: lista cerrada de la plataforma, constantes en `domain` con
  su whitelist. Una tabla de catálogo para valores que define un tercero es config para algo que no
  cambia (principio VI).
- **Nada para la cola de reintentos**: Uber reintenta (D1).
