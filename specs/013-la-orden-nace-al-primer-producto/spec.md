# Feature Specification: La orden nace al primer producto

**Feature Branch**: `013-la-orden-nace-al-primer-producto`

**Created**: 2026-09-05

**Status**: Draft — bloqueado por las *Decisiones aplazadas*, no pasa a plan

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

### User Story 4 - Sin red se sigue capturando (Priority: P2)

Si la tableta pierde la red a media captura, quien atiende sigue agregando productos y el servidor
se pone al día cuando vuelve.

**Why this priority**: Hoy el carrito es local y una caída de wifi no detiene a nadie. Convertir cada
toque en un viaje a la red sin resolver esto empeoraría el sistema para quien lo usa, que es lo
contrario de lo que esta feature busca.

**Independent Test**: Capturar productos con la red cortada, restablecerla, y comprobar que la orden
del servidor quedó con todos los productos y una sola vez cada uno.

**Acceptance Scenarios**:

1. **Given** la red caída, **When** se agregan productos, **Then** la pantalla los muestra y no
   bloquea nada.
2. **Given** esos productos capturados sin red, **When** la red vuelve, **Then** la orden del
   servidor queda con todos ellos, sin duplicados y sin faltantes.
3. **Given** la red caída, **When** se intenta cobrar, **Then** se dice que no se puede y por qué —
   cobrar sí necesita el servidor.

---

### Edge Cases

- **La orden nace sin caja abierta.** Crear un pedido exige turno abierto. Hoy el POS ni siquiera
  muestra la pantalla de venta sin caja, así que el caso no debería llegar — pero la regla tiene que
  decir qué pasa si llega, porque ahora crear ocurre en el primer toque y no en el confirmar.
- **Una orden en "cargando" al cerrar la caja.** Cerrar exige que no queden pedidos abiertos ni
  listos. Un borrador no es ninguno de los dos: hay que decidir si bloquea el cierre o no. Si no
  bloquea, queda colgando de un turno cerrado.
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
- **FR-010**: Sin red, capturar MUST seguir funcionando, y el servidor MUST quedar al día al volver,
  sin productos duplicados ni faltantes.
- **FR-011**: Cobrar MUST seguir exigiendo servidor y MUST decirlo con claridad cuando no lo haya.

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

## Decisiones aplazadas — el modelo de cajas y meseros

Tres bordes de este spec **no se pueden cerrar sin decidir antes cuántas cajas venden y de quién es
una orden en captura**. Contestarlos por separado fijaría ese modelo por accidente, así que quedan
aquí, nombrados, para resolverse juntos.

### Lo que hoy es cierto, medido

| | |
| --- | --- |
| Cajas configuradas por empresa | 3 (principal, clip, externa) |
| Cajas que RECIBEN ventas | **1** — el pedido siempre cuelga del turno de la caja principal (`GetOpenPrimarySession`) |
| Turnos abiertos a la vez por caja | 1 (`one_open_session_per_register`) |
| Personas que han abierto turno | 2 |
| Tabletas | 2, compartiendo la MISMA cuenta de usuario |
| Pedidos por día | 4 a 10 |

O sea: hoy el sistema es de **una caja que vende y dos estaciones que capturan con la misma
identidad**. Las otras dos cajas existen para traspasos y gastos, no para cobrar.

### Los tres bordes que dependen de eso

1. **¿Una orden en captura bloquea el cierre de caja?** Hoy cerrar exige que no queden pedidos
   abiertos ni listos. Un borrador no es ninguno de los dos. Con una sola caja que vende, "cerrar"
   es un evento del negocio y bloquear tiene sentido; con N cajas vendiendo, cerrar una no debería
   detener lo que se captura para otra.
2. **¿Quién limpia los borradores abandonados?** Con una caja, atarlos al cierre del turno es
   natural. Con varias, un borrador no sabría de qué turno colgarse hasta que se confirme.
3. **¿Quién gana si dos tabletas tocaron el mismo borrador?** Con dos estaciones compartiendo cuenta,
   el conflicto es entre dos personas del mismo mostrador. Con meseros identificados, la orden tiene
   dueño y la regla cambia: gana el mesero de la mesa, no el último que sincronizó.

### La pregunta que hay que responder primero

**¿Hacia dónde escala este negocio?** No es lo mismo diseñar para una caja con dos estaciones que
para N cajas con meseros identificados. Lo que este spec NO debe hacer es cerrar la puerta a lo
segundo por resolver lo primero de la forma más corta.

Dos cosas que ya empujan en esa dirección y conviene mirar juntas:

- **`register_session_id` en el pedido.** Hoy lo hereda de la caja principal. Si un borrador nace
  antes de confirmarse, hay que decidir si nace ya atado a una caja o si se ata al confirmar. Atarlo
  tarde es lo que deja la puerta abierta a varias cajas.
- **Dos estaciones comparten cuenta.** Mientras eso siga siendo así, "de quién es esta orden" no
  tiene respuesta en los datos. Identificar al mesero es un cambio anterior a este, no posterior.

Hasta que eso se decida, este spec queda **sin planear**: sus US1, US2 y US3 están completas y no
dependen del modelo, pero los tres bordes de arriba sí.

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
