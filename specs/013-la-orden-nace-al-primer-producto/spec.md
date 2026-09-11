# Feature Specification: La orden nace al primer producto

**Feature Branch**: `013-la-orden-nace-al-primer-producto`

**Created**: 2026-09-05

**Status**: Draft — desbloqueado el 2026-09-08, listo para `/speckit-plan`

**Input**: Ver *Origen* al final.

## Contexto

Hoy la cuenta que se arma en el POS vive **solo en la tableta** (`egb:ticket:v2`, en el navegador).
El pedido no existe en el servidor hasta que alguien lo confirma: con **Enviar**, o con el botón
final de cobro desde la [011](#).

Eso deja tres cosas sin resolver:

- El papel de la cuenta ([012](../012-imprimir-la-cuenta/spec.md)) lleva un nombre **propuesto**, no
  amarrado: si otra estación lo toma antes, el papel que el cliente tiene en la mano dice un nombre
  distinto al de su ticket.
- Una tableta que se muere a media captura se lleva la cuenta con ella.
- El botón que cambia entre cuentas dice "Cuenta 1", "Cuenta 2" — un número local que no significa
  nada fuera de esa tableta.

La regla que este spec implementa: **desde que una cuenta recibe su primer producto, la orden ya
empezó a ser tomada** y persiste en la base, con un estado que dice que todavía se está cargando.
No es una orden de cocina hasta que alguien lo indique.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - La orden empieza a existir con el primer producto (Priority: P1)

Quien atiende toca el primer producto y, desde ese momento, la orden existe: tiene nombre y número
propios. No aparece en la lista de pedidos ni llega a cocina — nadie ha dicho todavía que se
prepare.

**Why this priority**: Es la regla. Todo lo demás depende de que la orden exista antes de
confirmarse.

**Independent Test**: Tocar un producto y comprobar que la orden quedó en la base con su nombre y su
número, que no aparece en la lista de pedidos en curso y que cocina no recibió nada.

**Acceptance Scenarios**:

1. **Given** una cuenta vacía, **When** se le agrega el primer producto, **Then** la orden queda
   guardada con su nombre y su número, en estado de "cargando".
2. **Given** esa orden, **When** se mira la lista de pedidos en curso, **Then** no aparece.
3. **Given** esa orden, **When** se mira lo que llegó a cocina, **Then** no llegó nada.
4. **Given** esa orden, **When** se agregan más productos, **Then** se suman a la MISMA orden y no
   se crea otra.
5. **Given** esa orden, **When** se confirma —con Enviar o cobrando—, **Then** pasa a ser un pedido
   en curso normal y de ahí en adelante se comporta como hoy.

---

### User Story 2 - El nombre y el número ya son suyos (Priority: P1)

Desde el primer producto, el nombre que se le canta al cliente y el número del pedido están
amarrados: nadie más los puede tomar.

**Why this priority**: Es lo que vuelve confiable el papel de la cuenta. Un nombre propuesto que
cambia después deja al cliente con un papel que contradice su ticket.

**Independent Test**: Empezar una cuenta en una tableta, empezar otra en la segunda, y comprobar que
no comparten nombre ni número.

**Acceptance Scenarios**:

1. **Given** dos tabletas capturando a la vez, **When** las dos agregan su primer producto,
   **Then** cada orden recibe un nombre distinto y un número distinto.
2. **Given** una orden con nombre amarrado, **When** se imprime su cuenta, **Then** el papel lleva
   ese nombre y ese número, y coinciden con el ticket que sale al cobrar.
3. **Given** el botón que cambia entre cuentas, **When** hay una cuenta vacía, **Then** no muestra
   número; **When** ya tiene productos, **Then** muestra su nombre y su número.

---

### User Story 3 - Vaciar cancela la orden, y lo dice antes (Priority: P1)

El botón de vaciar deja de borrar una captura local: ahora cancela una orden que ya existe en la
base. Antes de hacerlo, dice qué se va a perder.

**Why this priority**: Es una acción destructiva sobre algo que persiste, en la pantalla que se toca
todo el día. Sin confirmación, un toque por error borra el trabajo del operador y deja una orden
cancelada que nadie pidió.

**Independent Test**: Con una cuenta de varios productos, tocar vaciar, comprobar que pregunta,
cancelar la pregunta y verificar que no pasó nada; aceptarla y verificar que la orden quedó
cancelada.

**Acceptance Scenarios**:

1. **Given** una cuenta con productos, **When** se toca vaciar, **Then** se pregunta nombrando la
   orden y cuántos productos se van a perder.
2. **Given** esa pregunta, **When** se descarta, **Then** la cuenta queda intacta.
3. **Given** esa pregunta, **When** se acepta, **Then** la orden queda **cancelada** conservando su
   número, y la tableta empieza una cuenta nueva.
4. **Given** la orden cancelada, **When** se agrega un producto a la cuenta nueva, **Then** esa
   nueva orden recibe el número **siguiente**, no el de la cancelada.
5. **Given** una cuenta vacía, **When** se toca vaciar, **Then** no se pregunta nada y no se cancela
   nada: no hay orden que cancelar.

---

### ~~User Story 4 - Sin red se sigue capturando~~ — FUERA DE ALCANCE (2026-09-08)

Se retira por decisión del dueño: *"por ahora, 100% internet... no es el foco del desarrollo ni
generará retorno aún."* La captura exige conexión y el servidor es la única fuente de verdad.

Era la historia más cara del spec —cola local, reconciliación al volver la red, idempotencia por
producto para no duplicar ni perder— y la que menos devuelve hoy, con un local que siempre tiene
conexión.

**El costo que se acepta, por escrito:** hoy el carrito vive en la tableta y una caída de wifi NO
detiene a nadie. Al nacer la orden en el servidor con el primer producto, un módem que parpadee sí
detiene la captura, y en hora pico eso es una fila. Es un retroceso conocido, no un descuido.

**Lo que el plan SÍ debe resolver, porque no es offline:** que un fallo de red al agregar un
producto se vea y se pueda reintentar, en vez de perderse en silencio. Un producto que la pantalla
muestra y el servidor no tiene es la peor de las dos opciones.

---

### Edge Cases

- **La orden nace sin caja abierta.** Crear un pedido exige turno abierto. Hoy el POS ni siquiera
  muestra la pantalla de venta sin caja, así que el caso no debería llegar — pero la regla tiene que
  decir qué pasa si llega, porque ahora crear ocurre en el primer toque y no en el confirmar.
- **Una orden en "cargando" al cerrar la caja.** DECIDIDO: no bloquea, y no queda colgando de nada
  porque el borrador no nace atado a un turno — se amarra al confirmar. Ver *Decisiones del modelo*.
- **Un borrador de las 23:50 confirmado a las 00:10.** La fecha de la venta la da el reloj
  ([008](../008-fecha-y-folio-separados/spec.md)). Tiene que sellarse al **confirmar**, no al nacer
  el borrador, o se reintroduce el defecto que esa feature cerró.
- **El inventario.** Crear un pedido descuenta stock. Un borrador NO puede descontarlo: los
  abandonos se comerían el almacén. El descuento se mueve al momento de confirmar.
- **Borradores viejos.** Una tableta que se apaga con una cuenta a medias deja un borrador que nadie
  va a cerrar. Se acumulan.
- **Dos tabletas sobre el mismo borrador.** Con la sincronización de la US4, dos aparatos pueden
  haber tocado la misma orden. Hay que decidir quién gana.
- **La bolsa de 88 nombres.** Cada borrador amarra uno. Un día con muchos abandonos la agota más
  rápido de lo que su diseño supone, y empiezan a salir nombres con vuelta ("Tigre 2").
- **El consecutivo crece más rápido que las ventas.** Cada abandono avanza el número. Al final del
  día el último folio será mayor que el número de ventas, y quien lo lea sin saber esto va a creer
  que faltan pedidos.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: Al agregarse el primer producto a una cuenta, el sistema MUST crear la orden en la base
  con un estado que la identifique como todavía en captura.
- **FR-002**: Esa orden MUST recibir su nombre y su número en ese momento, y ninguna otra orden MUST
  poder tomarlos.
- **FR-003**: Una orden en captura MUST NOT aparecer en la lista de pedidos en curso, MUST NOT llegar
  a cocina, y MUST NOT contar como venta en ningún reporte, resumen ni arqueo.
- **FR-004**: Una orden en captura MUST NOT descontar inventario. El descuento ocurre al confirmarse.
- **FR-005**: Confirmar una orden en captura —con Enviar o al cobrar— MUST sellar su fecha de negocio
  con el reloj de ese momento, no con el de su creación.
- **FR-006**: Vaciar la cuenta MUST preguntar antes, nombrando la orden y cuántos productos se
  pierden, y MUST NOT preguntar cuando la cuenta está vacía.
- **FR-007**: Al aceptarse, vaciar MUST dejar la orden **cancelada conservando su número**. La
  siguiente orden MUST tomar el número siguiente, nunca el de la cancelada.
- **FR-008**: El sistema MUST distinguir una orden cancelada **antes** de llegar a cocina de una
  venta cancelada: no son lo mismo y no pueden leerse igual en un reporte.
- **FR-009**: El selector de cuentas MUST mostrar el nombre de la orden, y su número solo cuando ya
  exista.
- **FR-010**: ~~Sin red, capturar MUST seguir funcionando~~ — **RETIRADO el 2026-09-08** con la US4.
  La captura exige conexión; el costo aceptado está escrito en esa historia.
- **FR-011**: Cobrar MUST seguir exigiendo servidor y MUST decirlo con claridad cuando no lo haya.
- **FR-012**: Un fallo de red al agregar un producto MUST verse y MUST poder reintentarse. El
  producto MUST NOT quedar pintado en la pantalla si el servidor no lo tiene: una cuenta que muestra
  algo que no existe es peor que un error, porque se cobra tarde y nadie lo audita.
- **FR-013**: La orden en captura MUST NOT nacer atada a un turno de caja. El vínculo con
  `register_session_id` MUST establecerse al confirmarse. (Ver *Decisiones del modelo*: es lo único
  irreversible del spec.)
- **FR-014**: La orden en captura MUST registrar quién la capturó desde que nace (`opened_by`), aun
  cuando hoy las dos tabletas compartan cuenta: es el dato que no se puede recuperar después.
- **FR-015**: Un borrador abandonado MUST limpiarse por edad y MUST NOT depender del cierre del
  turno, que es consecuencia de FR-013. El plazo lo fija el plan.

### Key Entities

- **Orden en captura**: una orden real, con nombre y número propios, que todavía no es una orden de
  cocina. Invisible para la lista de pedidos, para cocina y para todo lo que cuenta dinero.
- **Orden en curso**: lo que hoy existe. Una orden en captura se vuelve una de éstas al confirmarse.
- **Orden cancelada antes de cocina**: lo que deja vaciar. Conserva su número para que el
  consecutivo no tenga huecos sin explicar.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Cero órdenes en captura contadas como venta, medido sobre las 39 consultas del backend
  que filtran por estado.
- **SC-002**: El nombre y el número que el papel de la cuenta imprime coinciden siempre con los del
  ticket de esa venta.
- **SC-003**: Cero órdenes en captura que descuenten inventario.
- **SC-004**: Capturar sin red no pierde ni duplica un solo producto al reconectar.
- **SC-005**: Ningún número de folio se repite dentro de un turno, incluidas las órdenes canceladas
  antes de cocina.
- **SC-006**: Cerrar la caja sigue siendo posible y sigue exigiendo lo mismo que hoy.

## Assumptions

- **Vaciar cancela y conserva el número** (decidido por el dueño). Se descartó borrar la fila: vaciar
  dejaría de tener rastro y un operador que vacía por error no tendría a qué volver. Se descartó
  reutilizar el número: obligaría a quitar el índice único de folio por turno, y entonces dos
  pedidos del mismo turno podrían llamarse #14 — el folio dejaría de identificar un pedido.
- **Vaciar pregunta antes** (decidido por el dueño), porque pasó de perder una captura local a
  cancelar un pedido con número.
- **Sin red se sigue capturando y se sincroniza al volver** (decidido por el dueño).
- **El nombre de un borrador cancelado vuelve a la bolsa.** Se deduce de la regla de vaciar: la
  cuenta nueva reutiliza el nombre. La bolsa son 88 y cada abandono se quedaría uno.
- Se reutiliza la llave de idempotencia por lote que ya existe
  ([0063](../../server/migrations/0063_agregar_renglones_idempotente.sql)): es lo que hace segura la
  sincronización de la US4 cuando la red vuelve y la tableta reenvía lo que capturó.

## Riesgos que este spec reconoce

| Riesgo | Por qué importa |
| --- | --- |
| **39 consultas** filtran por estado de pedido | Cada una tiene que excluir el borrador. Una que se escape es dinero inexistente en un reporte o un arqueo |
| Cada toque de producto pasa a tocar el servidor | Hoy el carrito es local e instantáneo. La US4 existe para que eso no se note, y es la parte más difícil de la feature |
| El consecutivo avanza con cada abandono | El último folio del día será mayor que el número de ventas. Hay que decirlo donde se lea, o parecerá que faltan pedidos |

## Decisiones del modelo — tomadas el 2026-09-08

Tres bordes de este spec no se podían cerrar sin saber cuántas cajas venden y de quién es una orden
en captura. Se resolvieron juntas, que era el punto de dejarlas escritas en vez de contestarlas por
separado.

### Lo que hoy es cierto, medido

| | |
| --- | --- |
| Cajas configuradas por empresa | 3 (principal, clip, externa) |
| Cajas que RECIBEN ventas | **1** — el pedido siempre cuelga del turno de la caja principal (`GetOpenPrimarySession`) |
| Turnos abiertos a la vez por caja | 1 (`one_open_session_per_register`) |
| Personas que han abierto turno | 2 |
| Tabletas | 2, compartiendo la MISMA cuenta de usuario |
| Pedidos por día | 4 a 10 |

### La respuesta del dueño

> *"Hoy para El Gato Bobah, pero debe estar pensado que soporte el crecimiento de una vez a varias
> empresas, cadenas, etc."* y *"por ahora, 100% internet; ya llegará un momento cuando nos
> detengamos a optimizarlo para pensar en offline — por ahora no es el foco del desarrollo ni
> generará retorno aún."*

**Se construye para un local y se diseña para varios.** El modelo de operación deja de ser dato
conocido: cuántas cajas cobran y si hay meseros identificados lo decide un cliente que todavía no
existe. No se construye nada de eso hoy; tampoco se toma la decisión que lo impida.

### Cómo quedan los tres bordes

1. **El borrador nace SIN caja y se amarra al confirmar.** Es la única decisión irreversible del
   spec y por eso es la que se cuida: un borrador que nace atado a `register_session_id` fija que
   solo una caja captura, y desatarlo después exigiría repartir entre cajas pedidos que ya existen
   en la base de un cliente en operación — y a cuál pertenecía cada uno no se puede adivinar.
   Amarrarlo tarde cuesta lo mismo hoy.
2. **Un borrador NO bloquea el cierre de caja**, porque no cuelga de ninguna. Cerrar sigue exigiendo
   que no queden pedidos abiertos ni listos, que es la regla de hoy y no cambia. Un borrador sin
   confirmar no es comida que va a salir ni dinero que se decidió.
3. **Los borradores abandonados se limpian por edad, no por cierre de turno.** Es la consecuencia de
   (1) y (2): sin caja de la cual colgarse, el turno no puede ser quien los barra. El plan define el
   plazo; lo que este spec fija es que la limpieza NO se ata al cierre.
4. **Si dos tabletas tocan el mismo borrador, gana el servidor.** Con conexión obligatoria no hay
   historias divergentes que reconciliar: el servidor es la única fuente y la otra tableta ve el
   estado real en cuanto refresca. La regla de "gana el mesero de la mesa" se escribirá el día que
   exista mesero, y no antes.

### Lo que sigue SIN construirse, con su puerta abierta

- **Identificar quién captura.** Las dos tabletas siguen compartiendo cuenta. La puerta queda
  abierta porque `orders.opened_by` ya existe y el borrador lo llena desde que nace: el día que cada
  estación tenga su usuario, el dato histórico distingue solo. Darle cuenta propia a cada tableta es
  configuración, no migración.
- **Varias cajas cobrando a la vez.** Nada de este spec la asume, que es justo lo que decide (1).

## Out of Scope

- Cambiar cuándo se confirma un pedido. Eso lo decidieron la 005 y la 011.
- Compartir una cuenta a medias entre dos tabletas como funcionalidad ofrecida. La sincronización de
  la US4 es para que una tableta sin red no se detenga, no para capturar en dos a la vez.

## Origen

Regla dada por el dueño el 2026-09-05, al resolver qué identificador lleva el papel de la cuenta de
la [012](../012-imprimir-la-cuenta/spec.md): *"desde el momento en que un pedido empieza a recibir un
producto, en ese momento empieza a persistir un pedido/orden... persiste en base de datos pero con un
estatus que identifique que se sigue cargando los productos"*.

Se separó de la 012 a propósito: imprimir la cuenta se entrega sin esto, y esto toca todas las
consultas de dinero del sistema.

Decisiones tomadas por el dueño en esa conversación: partir en dos specs, vaciar cancela conservando
el número, vaciar pregunta antes, y sin red se sigue capturando.
