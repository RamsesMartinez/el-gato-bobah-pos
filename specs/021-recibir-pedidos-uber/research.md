# Phase 0 — Investigación

**Feature**: 021 · Recibir los pedidos de Uber Eats
**Fecha**: 2026-09-17

Lo que aquí se decide sale de tres fuentes: la documentación pública de Uber (con sus
contradicciones, que se nombran), el esquema y el código que ya existen en este repo, y dos
decisiones que tomó el dueño el 2026-09-16. Nada de esto es preferencia de estilo.

---

## D1 · Cuándo se le contesta a la plataforma: síncrono con presupuesto

**Decisión**: el aviso se guarda crudo, se trae el detalle del pedido **dentro del mismo request**
con un presupuesto de tiempo acotado, y se contesta 200 solo si todo salió. Si algo falla, se
contesta con error para que Uber reintente.

**Por qué**: hay dos requisitos que parecen pelearse. FR-005 dice que no se confirme lo que falló —
si se confirma, Uber no reintenta y el pedido se pierde en silencio. FR-006 dice que se conteste
antes de que Uber considere fallida la entrega. La salida es que traer el detalle es **una sola
llamada HTTP a Uber**, no un trabajo largo: con un presupuesto de segundos cabe de sobra dentro de
lo que Uber tolera, y el reintento de Uber (hasta 7 entregas con backoff) es mejor red de seguridad
que cualquier cola propia.

**Alternativa rechazada — confirmar primero y procesar después, con cola propia**: habría que
construir reintentos, backoff y un almacén de trabajos pendientes; es exactamente lo que Uber ya
hace y lo hace mejor. Y rompe FR-005: una vez confirmado, Uber ya no reintenta y el único que puede
recuperar el pedido es nuestro propio mecanismo — el día que ese mecanismo falle, no hay segunda
red. Principio VI: no se construye una cola para un caso que el proveedor ya resuelve.

**Lo que esto obliga**: el guardado del aviso crudo va **antes** de intentar el detalle, en su
propia transacción. Si el proceso se muere a media llamada a Uber, el aviso ya está en disco y el
reintento lo encuentra.

**Y el detalle también se guarda crudo** (corregido tras la revisión, 2026-09-17). La versión
anterior de este documento decía que guardar el aviso permitía reconstruir un pedido mal mapeado.
Era falso: el aviso son cuatro campos y una liga. Lo que puede traer una forma distinta de la
documentada es **el detalle**, y ése no se estaba guardando en ningún lado. Si el mapeo fallaba, no
había byte desde el cual corregir, y días después Uber puede ya no entregar ese pedido. Va en
`platform_incoming_orders.raw_detail`.

---

## D2 · Cómo se sabe de quién es un pedido, sin sesión y sin empresa

**Decisión**: una única consulta, por el pool y **sin contexto de empresa**, que traduce
`(plataforma, external_store_id)` → `(company_id, connection_id)`. Vive aislada, tiene nombre propio
y su propio test.

**Por qué**: quien llama al webhook es Uber, no una persona. No hay sesión, no hay JWT y por lo
tanto no hay `app.company_id`: no se puede usar `store.QC(ctx)` porque no hay a qué empresa atarlo.
La empresa **es el resultado** de la consulta, no su entrada.

**El riesgo que abre, y cómo se cierra**: la 020 dejó dicho que los servicios usan `QC` y que usar
`Q` fue un defecto que costó una fuga entre empresas. Esta es la excepción, y una excepción que no
se acota se copia. Se acota así:

- La consulta devuelve **solo** `company_id` y `connection_id`. Nada del negocio.
- A partir de ahí, **todo** lo demás corre con el tenant ya fijado en esa empresa.
- Un test verifica que un aviso de la tienda de la empresa A jamás escribe en la B.

**Cómo se resuelve la ambigüedad** (corregido tras la revisión de `db-architect`, 2026-09-17):
hoy la llave única es `(company_id, delivery_platform_id, external_store_id)`, así que **dos
empresas pueden registrar el mismo id de tienda** y la consulta puede devolver más de una fila.

La primera propuesta fue volver ese único **global**, con el argumento de que en el mundo real una
tienda de Uber pertenece a un solo comercio. **Se descarta**: el argumento vale en producción y no
en el ambiente de pruebas, donde las plataformas reparten ids de tienda de demostración
compartidos. La segunda empresa que quisiera probar chocaría contra el único — y demostrar la
feature por empresa es justamente lo que FR-031 exige.

**Lo que se hace**: se conserva el único por empresa y, cuando la consulta devuelve más de una
candidata, **se verifica la firma contra cada una**. La que valide identifica la empresa; si
ninguna valida, se rechaza. Lo que parecía "un bucle de criptografía" es, en el caso normal, cero o
una iteración: un HMAC-SHA256 sobre unos cientos de bytes. Y tiene una propiedad que el único
global no tiene: la empresa no queda determinada por un dato que cualquiera puede escribir en el
cuerpo, sino por quién pudo firmarlo.

---

## D3 · Dónde vive la llave con la que se verifica la firma

**Decisión**: en la base, en una tabla nueva, por `(company_id, delivery_platform_id)`, con **dos
llaves válidas a la vez**.

**Por qué no en el entorno**: la URL del webhook es **una sola** para todas las empresas. Si la
llave vive en `UBER_EATS_*`, el sistema solo puede verificar los avisos de una empresa, y los de la
segunda llegan a la misma puerta sin forma de validarlos ni de saber de quién son. No es una columna
que se agregue después: es la feature que no funciona para el segundo cliente. Principio VIII — esto
no se puede agregar al mismo costo, así que hoy no se cierra la puerta.

**Por qué por empresa y no por tienda**: la llave la da el tablero de Uber por **aplicación**, y una
aplicación es de una empresa; esa empresa puede tener varias sucursales bajo la misma app. Ponerla
en `platform_connections` la duplicaría por sucursal y las dejaría divergir.

**Por qué dos**: el tablero de Uber ofrece una llave secundaria (FR-003). Sirve para cambiar la
llave sin dejar de recibir pedidos: durante el cambio las dos son válidas. Que exista una segunda
columna no es especulación — es un campo que el proveedor ya expone.

**Lo que NO se mueve, y es honesto decirlo**: `UBER_EATS_CLIENT_ID` y `UBER_EATS_CLIENT_SECRET`
siguen en el entorno. Los usa la llamada **saliente** (traer el detalle, aceptar, rechazar), que la
020 ya construyó así. O sea: el camino de salida sigue siendo de una sola empresa. Esta feature no
cierra esa puerta — **ya estaba cerrada** — pero tampoco la abre, y conviene que
`/speckit-analyze` lo vea escrito en vez de descubrirlo con el segundo cliente.

**Con qué secreto firma Uber sigue sin resolverse** (documentación contra tablero, ticket abierto).
Por eso la llave es **un valor configurable**, no una derivación de otra cosa: el día que Uber
conteste, se captura el valor correcto y nada del diseño cambia.

---

## D4 · El pedido que llegó no es todavía un pedido del POS

**Decisión**: tablas propias para el pedido entrante y sus renglones. La fila en `orders` se crea
**al aceptar**.

**Por qué**: `orders` es la tabla del dinero. Un pedido que aún no se acepta aparecería en el
tablero de cocina, en la deuda por cobrar y en el corte. Principio III: cada peso se clasifica una
sola vez y lo que no es ingreso no entra al total.

Hay además una razón mecánica: `orders.opened_by` es **not null** y referencia a un usuario. Un
pedido que llega solo no tiene quién lo abrió. Al crear la fila en el momento de aceptar, quien
acepta **es** `opened_by`, y la columna sigue diciendo la verdad sin inventar un usuario de sistema
— que es justo el parche que el dueño ya rechazó una vez en esta misma integración.

**Alternativa rechazada — un estado `pendiente` en `orders`**: obligaría a revisar cada consulta que
hoy filtra por estado (el tablero, el corte, los reportes, el arqueo) para excluir el nuevo. Son
decenas, y la que se olvide falla en silencio reportando dinero que no existe.

---

## D5 · Aceptar sin turno de caja abierto

**Decisión** (del dueño, 2026-09-16): se acepta igual. La fila en `orders` nace con
`register_session_id` en NULL y se enlaza al turno que se abra después. Al **abrir** un turno, quien
lo abre ve qué pedidos de plataforma quedan considerados en esa apertura.

**Por qué se puede**: `orders.register_session_id` ya es **nullable** desde la 0007. La puerta está
abierta en el esquema; lo que falta es el camino que la use y la pantalla que lo explique.

**Lo que hay que resolver en el plan**: hoy `OrdersService` exige `GetOpenPrimarySession` antes de
crear un pedido, y el folio se resuelve **dentro de la sesión** (`folioNamesUsedInSession`). Un
pedido sin sesión necesita su propio camino de folio. Como es un pedido de plataforma, su
identidad ya la da el folio de Uber (`orders.platform_order_ref`), así que no compite por los
folios de mostrador.

**Cómo se enlaza** (fijado tras la revisión, 2026-09-17): con un `update` dentro de la misma
transacción que abre el turno, que reclama los pedidos huérfanos **cuyo `business_date` sea el del
turno que se abre**. No todo lo huérfano: un pedido de hace tres días aparecería como ingreso de
hoy. `business_date` se calcula con la zona horaria del negocio y ya existe; se usa esa, no la hora
del servidor.

Dejarlos sueltos y que el corte los buscara por fecha obligaría a que **cada** consulta de corte
repitiera el mismo predicado, y el principio III ya advierte que la lista y el resumen que divergen
hacen que uno de los dos mienta.

**Lo que queda abierto y hay que decidir cuando exista la segunda caja**: si dos cajas abren casi al
mismo tiempo, la primera se lleva todos los pedidos huérfanos. Hoy es correcto —una sola caja
vende— pero no hay ningún criterio de a qué caja pertenece un pedido de Uber.

---

## D6 · Un pedido de plataforma HOY no puede ser para recoger

**Hallazgo, no decisión.** `orders` trae desde la 0007:

```sql
check (service_type = 'domicilio' or delivery_platform_id is null)
```

Es decir: si hay plataforma, el tipo de servicio **tiene que ser domicilio**. Un pedido de Uber Eats
que el cliente pidió **para recoger en tienda** no cabe en la tabla.

**Por qué importa aquí y no antes**: mientras la captura era manual, quien capturaba elegía
"domicilio" y nadie lo notaba. Con los pedidos entrando solos, el tipo lo dice Uber, y el primer
pedido de recoger revienta la inserción.

**Y es peor**: recoger en tienda es exactamente el camino que el dueño propuso para probar la
integración de punta a punta sin pagar repartidor. El `check` bloquea la prueba más barata que
tenemos.

**Lo que se propone**: relajar el `check` para admitir `para_llevar` con plataforma. Va al
`db-architect` con el resto del esquema. No se relaja "por si acaso": se relaja porque hay un caso
real, medido en la API de Uber, que hoy no entra.

---

## D7 · Cómo se le avisa a la tableta que llegó un pedido

**Decisión**: el `realtime.Broker` que ya existe, con un tipo de evento nuevo. Nada nuevo que
construir.

**Por qué**: la tableta ya escucha `GET /api/v1/events` (SSE) y ya reacciona a `order.created` y
`order.updated`. Publicar ahí es una línea, llega a todas las tabletas de la empresa, y el broker ya
está fuera del middleware de tenant por la razón correcta (una conexión abierta no acapara una
conexión de tenant). Principio VI: no se inventa un segundo canal.

---

## D8 · Quién imprime

**Decisión**: imprime la tableta que aceptó, con la configuración de impresión que ya existe.

**Por qué**: en este sistema la impresión es del cliente, no del servidor —`PrintSettings` vive en
la tableta y el servidor no habla con ninguna impresora—. Aceptar es un toque en una tableta
concreta; esa es la que tiene la impresora enfrente.

---

## D9 · Enterarse de que Uber cerró la tienda

**Decisión**: se recibe y se muestra el aviso de cambio de estado de la tienda. Si Uber no concede
el permiso que ese aviso exige, se cubre consultando el estado de la tienda, que **ya sabemos leer**
(`GET .../status`, verificado en la 020 y documentado en `http/uber-eats.http`).

**Por qué la alternativa importa**: ese aviso exige un permiso que hoy **no tenemos** y que está en
el ticket abierto con Uber. Atar FR-028 a un permiso que puede no llegar dejaría el requisito sin
cumplir por una razón ajena. Leer el estado cuesta una llamada que ya funciona.

---

## Contradicciones de la documentación de Uber que NO se resuelven leyendo

Se dejan aquí para que nadie las vuelva a investigar y para que el plan no dependa de ninguna:

| Qué | Versión A | Versión B |
|---|---|---|
| Secreto de la firma | El `client_secret` de la app (texto de la guía) | Una *Signing Key* propia (el tablero real) |
| Política de reintentos | 10s, 30s, 60s, 120s… solo ante 5xx (guía general) | 1s, 2s, 4s, 8s… ante cualquier no-200 (páginas de referencia) |
| Cuántas URL admite | "solo una URL primaria" (texto) | Un botón "Other Webhook" (su propia captura) |

El diseño no depende de cuál gane: la llave es configurable (D3), y la política de reintentos solo
cambia **cuándo** llega el reintento, no qué hacemos con él (D1 + deduplicación).

Sin documentar en ningún lado público: el campo *Developer UUID*, la *Secondary Signing Key*, el
*Client Credentials Format*, y **cómo se genera un pedido de prueba**. Los cuatro están en el ticket.
FR-031 existe justamente para que nada de eso bloquee construir y validar la feature.
