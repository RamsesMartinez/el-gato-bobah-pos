# Feature Specification: Una sola puerta para cobrar — la cuenta vive en el servidor

**Feature Branch**: `030-una-sola-puerta`

**Created**: 2026-10-08

**Status**: Draft

**Input**: El dueño eligió la **opción A** del lienzo «Una sola puerta para cobrar» (versión 2, con
los casos raros) el 2026-10-08: *"vamos por la opción A, impleméntala… debemos atender en esta
corrida el 100% de estos flujos"*. Absorbe la [013](../013-la-orden-nace-al-primer-producto/spec.md).

## Contexto

Hoy hay tres puertas para cobrar y dos palabras para lo mismo:

1. **COBRAR del ticket** cobra lo capturado como pedido nuevo y no ofrece «Por productos» porque la
   cuenta todavía no existe en el servidor.
2. **El botón naranja «Pedidos por cobrar»** lista lo ya enviado que debe dinero; ahí sí está «Por
   productos», pero nada lleva a él, y no lista un pedido **ya pagado que sigue en cocina**, así que
   a ése no se le puede agregar nada.
3. **El tablero** solo ofrece «Cobrar» cuando ya no falta nada.

Las pestañas «Cuenta 1 · 2» son borradores que viven **solo en esa tableta** (`egb:ticket:v2`); los
«Pedidos por cobrar» son cuentas del servidor. Para quien cobra, las dos son «la cuenta de la mesa».
El creador de los specs se confundió con su propio flujo.

La solución elegida (A): **una sola fila de cuentas**. Toda cuenta viva —se esté capturando, esté en
cocina, deba dinero o la haya pagado alguien a medias— está en esa fila, en todas las tabletas.
Tocarla la carga en el ticket: agregar, mandar a cocina y cobrar son el mismo lugar. «Cobrar» abre
siempre los tres modos.

## Decisiones (tomadas o derivadas, con su consecuencia)

| # | Decisión | Por qué / consecuencia |
|---|---|---|
| D-1 | **La cuenta en captura persiste en el servidor desde el primer producto**, en su propia entidad («cuenta en captura»), **no** como un estado nuevo de pedido. Al mandarla a cocina (o al cobrarla) se convierte en pedido por el camino que ya existe. | Cumple la regla de la 013 («desde el primer producto persiste, con un estado que dice que se sigue cargando») sin tocar las ~30 consultas de dinero que filtran pedidos por exclusión de estado: un borrador nunca es un pedido, así que ninguna consulta de ventas, corte, reportes, recetas o Top puede contarlo. Es la forma de cumplir FR-003 de la 013 por construcción y no por revisar 30 consultas. |
| D-2 | **El nombre se amarra al nacer la cuenta; el número (folio) al mandarla a cocina.** | El nombre es lo que se le canta al cliente y lo que lleva la pestaña: se amarra desde el primer producto. El número se da al confirmar, así el consecutivo no crece con cada cuenta abandonada y no hay huecos que expliquen (riesgo de la 013 resuelto). Diferencia con la 013 FR-002 —que pedía también el número al nacer—: la cuenta impresa antes de enviar lleva el nombre, sin número. |
| D-3 | **Lo que se agrega a un pedido ya enviado queda «Nuevo · aún no va a cocina»** hasta que alguien toque «Enviar N a cocina» (o cobre). También vive en el servidor: es una cuenta en captura que apunta a ese pedido. | Otra tableta lo ve, y una tableta que se apaga no se lo lleva. Cocina recibe solo lo nuevo (camino de agregar que ya existe, idempotente). |
| D-4 | **Capturar exige conexión** (decisión del dueño en la 013, 2026-09-08, con el costo aceptado por escrito). Un producto que el servidor no confirmó se ve como «guardando»; si falla, sale de la cuenta y un aviso ofrece reintentar. Mandar a cocina y cobrar se apagan mientras haya algo sin guardar o sin conexión. | Una cuenta que muestra algo que el servidor no tiene es peor que un error (013 FR-012). |
| D-5 | **Si dos tabletas tocan la misma cuenta, gana el servidor** (013). Lo que agrega cada una se suma; cambiar o quitar algo que la otra ya cambió se rechaza y la cuenta se recarga con un aviso. | Sin historias divergentes que reconciliar. |
| D-6 | **Una cuenta en captura no cuelga de ningún turno, no descuenta inventario ni tiene fecha de negocio**; todo eso ocurre al mandarla a cocina (013 decisiones 1–3). | Igual que la 013. |
| D-7 | **Descartar** una cuenta en captura la deja como «descartada» (conserva el rastro, no es venta cancelada) y **devuelve su nombre a la bolsa**. Una cuenta vacía se descarta sin preguntar; con productos, pregunta en una hoja de la app. | 013 US3 y FR-008, sin folio que conservar (D-2). |
| D-8 | **Una cuenta en captura sin tocar en 12 horas se descarta sola.** El cierre de caja la lista con «Descartar» pero **no bloquea** el cierre. | 013 FR-015 y decisión 2. |
| D-9 | **Cerrada = pagada completa Y entregada.** Una cerrada ya no recibe productos (lo que pidan después es otra cuenta). Una «entregada que debe» sí recibe y vuelve a cocina. | Lienzo V2-8 / X1. Hoy un pedido entregado y pagado se reabre al agregarle: cambia. |
| D-10 | ~~**El cierre de caja sigue bloqueando solo con pedidos en cocina o listos**. Las cuentas que deben se listan sin bloquear.~~ **Reemplazada por D-13 (dueño, 2026-10-09).** Las que se están capturando siguen listándose con «Abrir» / «Descartar», sin bloquear. | Se deja tachada: el «fiado que pasa al día siguiente» era la razón y el dueño la descartó. |
| D-11 | **Un pedido de plataforma** se ve en la fila, se cobra completo con su método y **no recibe productos** ni se divide. | Lienzo V2-6. |
| D-12 | **Las cuentas guardadas en la tableta con la versión anterior se suben al servidor** la primera vez que abre la versión nueva, y el almacenamiento local de cuentas desaparece. | Un deploy no puede tirar lo que alguien estaba capturando; y la pantalla en blanco por cuenta guardada con forma vieja deja de ser posible. |
| D-13 | **No hay fiados** (decisión del dueño, 2026-10-09). La caja principal **no cierra** mientras haya un pedido de mostrador entregado que deba, de cualquier día. El cierre los lista con lo que debe cada uno y ofrece **«Cobrar»** (lleva a su cuenta) o **«Cancelar»** con motivo («se fue sin pagar»). La barrera es el servidor (409 `UNPAID_ORDERS`, que nombra cada pedido y lo que debe); la pantalla lo refleja. | Lo que se entrega se cobra o se cancela con su rastro; el corte deja de firmarse con venta sin cobrar de mostrador. **Consecuencias que no se ven**: (a) un fiado de meses atrás también bloquea; (b) «Cancelar» solo existe si el pedido **no tiene pagos** —cancelar uno pagado a medias sacaría de Ventas dinero que sí está en el cajón—; el pagado a medias sale cobrándolo o con **«Cancelar lo que falta»** (D-15); (c) cancelar un entregado **no repone inventario** (la comida ya salió); (d) los de plataforma no bloquean: los paga la plataforma, y siguen en «Cuentas pendientes». |
| D-14 | **Se quita el ajuste «el tablero puede cobrar»** (decisión del dueño, 2026-10-09). Migración 0084. | No tenía efecto en la operación de hoy y era una cuarta puerta de cobro. Quien no puede entrar a Vender ve en el tablero cuánto falta, sin botón de cobro. El valor que cada empresa tenía se pierde (el Down devuelve la columna apagada). |
| D-15 | **«Cancelar lo que falta»** (decisión del dueño, 2026-10-09, opción A). En un pedido de mostrador **entregado y pagado a medias** cuyo cliente se fue, lo pagado **se queda como venta** (sin devolución: el dinero sigue en el cajón) y el resto se **da por perdido** con su motivo, quién y cuándo (migración 0085). El pedido deja de deber y de bloquear el cierre; no se repone inventario. Se ofrece en el cierre de caja y en el menú de la cuenta; el servidor es la barrera (409 sin pagos, sin entregar, de plataforma, o si ya no debe; cobrar después de darlo por perdido también se niega: aprobado por el dueño el 2026-10-09, spec 031 DO-5). | **Cómo se ve**: en Ventas, tile propio «Perdido» por el día en que se dio por perdido, **fuera del Total** (no se cobró), fuera de Devoluciones (no salió dinero) y fuera de Pendiente (ya no se debe). En el corte, línea propia «Perdido»: no está en «Sin cobrar» ni en el esperado; vendido = cobrado + sin cobrar + perdido. Lo vigila `TestWriteOffIsCountedOnce`, que falla nombrando el concepto que lo duplicó. |

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Todas las cuentas vivas en una fila (Priority: P1)

Quien atiende ve arriba del menú una fila con las cuentas vivas de todas las tabletas: las que se
están capturando, las que están en cocina, las pagadas que siguen en cocina, las entregadas que deben
y las de pago parcial. Cada una dice su nombre, su estado y lo que falta. Si no caben, «+N» abre la
lista completa agrupada (Capturando · En cocina · Entregadas que deben · De días anteriores) con hora,
antigüedad y buscador. Tocar una la carga en el ticket.

**Why this priority**: Es la opción A. Sin esto siguen las tres puertas.

**Independent Test**: Con una cuenta capturándose en la tableta 1, una enviada a cocina, una pagada
en cocina, una entregada que debe y una de ayer que debe, la tableta 2 ve las cinco en la fila o en
«+N», con su estado correcto, y al tocar cualquiera aparece en su ticket.

**Acceptance Scenarios**:

1. **Given** una cuenta capturándose en otra tableta, **When** se mira la fila, **Then** aparece con «Capturando».
2. **Given** un pedido pagado completo que sigue en cocina, **When** se mira la fila, **Then** aparece como «Pagada · en cocina» y se le pueden agregar productos.
3. **Given** un pedido entregado que debe de un día anterior, **When** se abre «+N», **Then** está en «De días anteriores» con su fecha.
4. **Given** un pedido pagado y entregado, **When** se mira la fila, **Then** no aparece (está cerrado).
5. **Given** más cuentas de las que caben, **When** se mira la fila a 1024×600, **Then** se ven las que caben, «+N» y el botón de cuenta nueva, sin desbordar ni empujar nada fuera de pantalla.
6. **Given** la tarjeta de un pedido en el tablero, **When** se toca «Abrir cuenta», **Then** se abre Vender con esa cuenta cargada.

---

### User Story 2 - La cuenta existe desde el primer producto (Priority: P1)

Tocar el primer producto crea la cuenta en el servidor con su nombre. Lo que se agrega se guarda en el
servidor; si la tableta se apaga o se recarga, la cuenta sigue ahí y en las otras tabletas.

**Why this priority**: Sin esto la fila mezcla borradores de una tableta con cuentas de todas (lienzo V2).

**Independent Test**: Agregar 3 productos, recargar la página y abrir otra tableta: la cuenta está en
las dos con los 3 productos y el mismo nombre; no aparece en Ventas, en el corte, en el tablero ni
descontó inventario.

**Acceptance Scenarios**:

1. **Given** una cuenta nueva, **When** se toca un producto, **Then** la cuenta queda guardada con un nombre de la bolsa que ninguna otra cuenta viva tiene.
2. **Given** dos tabletas que empiezan cuenta a la vez, **When** las dos agregan su primer producto, **Then** reciben nombres distintos.
3. **Given** una cuenta capturándose, **When** se mira Ventas, el corte, Reportes, el tablero o el almacén, **Then** no existe para ninguno.
4. **Given** sin conexión, **When** se toca un producto, **Then** no se agrega en silencio: se ve que no se guardó y se ofrece reintentar; «Enviar» y «Cobrar» quedan apagados.
5. **Given** una tableta con cuentas guardadas por la versión anterior, **When** abre la versión nueva, **Then** esas cuentas aparecen en la fila con sus productos y nada se pierde.

---

### User Story 3 - Agregar después de cocina manda solo lo nuevo (Priority: P1)

En una cuenta ya enviada, el ticket separa **Pagado** (con candado), **En cocina** (con lo entregado
marcado) y **Nuevo · aún no va a cocina** (con −/+). «Enviar N a cocina» manda solo lo nuevo; la
comanda sale como agregado.

**Why this priority**: Es el flujo de todos los días con la mesa que sigue pidiendo.

**Independent Test**: Pedido en cocina con 3 productos; agregar 1; el ticket lo muestra en «Nuevo»;
«Enviar 1 a cocina» imprime una comanda con solo ese producto y el tablero lo muestra pendiente.

**Acceptance Scenarios**:

1. **Given** un pedido en cocina, **When** se agrega un producto, **Then** queda en «Nuevo» y cocina no lo recibe todavía.
2. **Given** productos en «Nuevo», **When** se toca «Enviar N a cocina», **Then** cocina recibe solo esos y pasan a «En cocina».
3. **Given** productos en «Nuevo», **When** se cobra, **Then** primero se mandan a cocina y después se cobra (un pago nunca cubre algo que no existe en el pedido).
4. **Given** un pedido entregado que debe, **When** se le manda algo nuevo, **Then** vuelve a «En cocina».
5. **Given** un pedido cerrado (pagado y entregado), **When** se intenta agregar, **Then** se ofrece empezar una cuenta nueva.
6. **Given** un pedido de plataforma, **When** se intenta agregar, **Then** no se permite y se dice por qué.

---

### User Story 4 - Cobrar siempre ofrece los tres modos (Priority: P1)

«Cobrar» desde el ticket abre la hoja con Por productos · Entre personas · Por monto para cualquier
cuenta de mostrador, se haya enviado o no. Si la cuenta tiene algo sin enviar, cobrar lo manda a
cocina primero. Cerrar la hoja sin cobrar deja la cuenta en la fila tal como estaba.

**Why this priority**: Es la puerta única. Hoy «Por productos» no sale desde el ticket.

**Independent Test**: Cuenta capturándose con 4 productos; «Cobrar» → «Por productos» está disponible;
cobrar 2 productos; la cuenta queda en la fila como «Pago parcial · falta $X».

**Acceptance Scenarios**:

1. **Given** una cuenta capturándose, **When** se toca Cobrar, **Then** la hoja ofrece los tres modos.
2. **Given** la hoja abierta, **When** se cierra sin cobrar, **Then** la cuenta sigue en la fila y nada se canceló (si tenía algo sin enviar, ya quedó «En cocina»: cobrar lo manda primero, y el botón lo dice: «Enviar y cobrar»).
3. **Given** una cuenta de plataforma, **When** se toca Cobrar, **Then** solo se cobra completa con el método de su plataforma.

---

### User Story 5 - Nada se cierra ni se pierde por accidente (Priority: P1)

Una cuenta vacía se cierra sin preguntar. Una capturándose con productos pregunta en una hoja de la
app («¿Descartar la cuenta de Levkoy? Tiene 2 productos ($74) y no se ha mandado a cocina»). Una
cuenta enviada o con pagos no tiene «cerrar»: solo «⋮ › Cancelar pedido», con motivo y permiso. No
queda ningún diálogo del sistema (`confirm`, `prompt`) en la aplicación.

**Independent Test**: Intentar cerrar cada tipo de cuenta y verificar el comportamiento; buscar en el
código que no queden `confirm(`/`prompt(`/`alert(`.

**Acceptance Scenarios**:

1. **Given** una cuenta vacía, **When** se cierra, **Then** se descarta sin preguntar.
2. **Given** una cuenta con productos sin enviar, **When** se toca cerrar, **Then** sale la hoja; «Seguir capturando» no cambia nada; «Descartar» la descarta y su nombre vuelve a la bolsa.
3. **Given** una cuenta enviada, **When** se mira su pestaña o su ticket, **Then** no hay forma de cerrarla salvo cancelar el pedido.

---

### User Story 6 - Quitar algo que ya está en cocina (Priority: P2)

Desde el ticket, quitar 1 de 2 de un producto en cocina abre la hoja con contador, motivos (sin
preselección) y el texto de qué pasa: cocina recibe el aviso y lo que ya se preparó no vuelve al
almacén. Lo pagado tiene candado.

**Acceptance Scenarios**:

1. **Given** un renglón ×2 en cocina, **When** se quita 1, **Then** queda ×1, el total baja y el tablero lo refleja.
2. **Given** un renglón pagado, **When** se mira el ticket, **Then** no tiene bote; tiene candado.

---

### User Story 7 - Red, recarga y otra tableta (Priority: P2)

Sin conexión, un aviso fijo dice que no se puede agregar, mandar a cocina ni cobrar, y se reintenta
solo. Si otra tableta cobró, canceló o cambió la cuenta que esta tiene abierta, un aviso lo dice y la
cuenta se actualiza.

**Acceptance Scenarios**:

1. **Given** la cuenta abierta en dos tabletas, **When** una la cobra, **Then** la otra muestra «Siamés se cobró en otra tableta» y su ticket se actualiza.
2. **Given** dos tabletas agregando a la misma cuenta, **When** las dos agregan, **Then** se suman los dos productos.
3. **Given** que la tableta A quitó un producto, **When** la tableta B intenta cambiar ese mismo producto, **Then** se rechaza con un aviso y B ve la cuenta real.

---

### User Story 8 - Cerrar caja con cuentas vivas (Priority: P2)

El cierre lista las cuentas vivas: las que bloquean (en cocina o listas) con «Abrir»; las que deben y
las que se capturan, sin bloquear, con «Abrir» o «Descartar». La confirmación de cierre es una hoja de
la app.

**Acceptance Scenarios**:

1. **Given** un pedido en cocina, **When** se intenta cerrar, **Then** el cierre está bloqueado y «Abrir» lleva a esa cuenta.
2. **Given** solo cuentas capturándose, **When** se cierra, **Then** cierra y siguen en la fila.
3. **Given** un pedido de mostrador entregado que debe (de cualquier día), **When** se intenta cerrar, **Then** el cierre está bloqueado y lo lista con lo que debe, «Cobrar» y —si no tiene pagos— «Cancelar» con motivo (D-13, 2026-10-09); si está pagado a medias, «Cancelar lo que falta» con motivo (D-15).

---

### Edge Cases

Los 30 casos del lienzo (V2-9) se cubren así; el número es el del lienzo:

1. Cuenta en curso se pierde de vista → US1.
2. Pagado pero en cocina no se le podía agregar → US1 AS2, US3.
3. Deuda de días anteriores solo se veía en el arqueo → US1 AS3.
4. «Entregada y debe» confundida con cerrada → D-9, estado propio en rojo.
5. Mandar a cocina borraba la cuenta de la pantalla → la cuenta enviada se queda en la fila (US1).
6. Agregar después de cocina: cocina recibe solo lo nuevo → US3.
7. Agregar a una entregada no llegaba a cocina → US3 AS4.
8. Quitar 1 de 2 y avisar a cocina → US6.
9. Quitar un pagado bajaba el total bajo lo cobrado → US6 AS2 (y el servidor ya lo rechaza).
10. `confirm()` nativo → US5.
11. Cerrar la hoja de cobro creyendo que se canceló → US4 AS2.
12. Pedido atorado sin botones → todo estado tiene salida; el cierre lista y abre cada cuenta (US8).
13. Cancelar con algo entregado rebotaba → ya existe («Quitar lo que falta»); se conserva desde ⋮.
14. Tableta se apaga y se lleva las cuentas → US2.
15. Red caída al confirmar creaba dos pedidos → la llave de idempotencia es la cuenta en captura.
16. Otra tableta cobró o canceló → US7 AS1.
17. Dos tabletas editan la misma cuenta → D-5, US7 AS2–3.
18. Pantalla en blanco por cuenta guardada vieja → D-12: no hay cuentas en el almacenamiento local.
19. Plataforma: un solo método, no se divide → D-11.
20. Uber aceptado sin caja → se corrige en la 031 (defectos de dinero).
21. Cerrar caja con cuentas vivas → US8.
22. Turno largo: fecha y folio → la fecha se sella al mandar a cocina (D-6), regla de la 008.
23. El nombre cambia al confirmar / la bolsa se agota → D-2: el nombre se amarra al nacer; descartar lo devuelve.
24. Borradores abandonados inflan folio y reportes → D-1, D-2, D-8.
25. Tres puertas → US1, US4; el tablero dice «Abrir cuenta».
26. «Por productos» no salía desde el ticket → US4.
27. Descuento después de pagos / productos movidos → reglas de la 027, sin cambio.
28. Devoluciones dobles → 031.
29. La fila no cabe a 1024×600 → se mide con prueba en la tableta (SC-004).
30. Iconos de cliente y descuento junto a Enviar/Cobrar → el pie solo lleva totales y dos botones.

Además:

- **Dos tabletas cobran la misma cuenta en captura a la vez**: la conversión a pedido es idempotente por la cuenta; el segundo cobro ve el pedido ya creado y cobra lo que falta.
- **Cobrar una cuenta en captura sin caja abierta**: hoy ni se muestra Vender sin caja; si llega, el envío se rechaza, la hoja de cobro no se abre, el motivo se ve en el pie y la cuenta sigue intacta.
- **Producto que dejó de venderse mientras la cuenta esperaba**: al mandar a cocina se avisa cuál, como hoy («Ya no están en el menú»).
- **Cambiar de canal (Mostrador → Uber) con productos**: reprecia en el servidor; una cuenta de plataforma capturada a mano pide su folio antes de mandarse, como hoy.
- **Una cuenta en captura de hace 12 horas** se descarta sola y su nombre vuelve a la bolsa.
- **Descartar una cuenta que otra tableta acaba de mandar a cocina**: se rechaza; ya es un pedido.
- **Cambiar de empresa en la tableta**: la fila es de la empresa de la sesión (RLS); no hay nada local que limpiar.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: Al agregarse el primer producto, el sistema MUST guardar la cuenta en el servidor con un nombre de la bolsa que ninguna otra cuenta viva ni pedido del turno tenga.
- **FR-002**: Una cuenta en captura MUST NOT aparecer en ventas, corte, reportes, recetas, «Top», tablero de cocina ni descontar inventario.
- **FR-003**: Agregar, cambiar cantidad, quitar, poner nota, cliente, canal, tipo de servicio, envío y descuento de una cuenta en captura MUST guardarse en el servidor; cada operación de agregar MUST ser idempotente ante reintentos.
- **FR-004**: Cambiar o quitar un renglón que otra tableta ya cambió MUST rechazarse sin aplicar, y la pantalla MUST recargar la cuenta y avisarlo.
- **FR-005**: «Enviar a cocina» de una cuenta en captura sin pedido MUST crear el pedido (folio, turno, fecha, inventario, comanda) por el camino de hoy y MUST ser idempotente por la cuenta.
- **FR-006**: Los productos agregados a un pedido enviado MUST quedar sin enviar hasta «Enviar N a cocina» o un cobro; al enviarse, MUST llegar a cocina solo esos.
- **FR-007**: Cobrar una cuenta con productos sin enviar MUST mandarlos a cocina antes de registrar el pago.
- **FR-008**: La hoja de cobro MUST ofrecer «Por productos», «Entre personas» y «Por monto» para toda cuenta de mostrador con algo por cobrar; para plataforma, solo cobrar completo con su método.
- **FR-009**: El sistema MUST listar las cuentas vivas de la empresa —en captura, en cocina, listas, pagadas en cocina, entregadas que deben (de cualquier día)— con estado, nombre, número si existe, hora, antigüedad, total, pagado y lo que falta.
- **FR-010**: Un pedido pagado completo y entregado MUST NOT recibir productos.
- **FR-011**: Un pedido de plataforma MUST NOT recibir productos ni dividirse.
- **FR-012**: Descartar MUST preguntar en una hoja de la app cuando hay productos y MUST NOT preguntar cuando la cuenta está vacía; MUST dejarla «descartada» y devolver su nombre a la bolsa.
- **FR-013**: Una cuenta en captura sin cambios en 12 horas MUST descartarse sola.
- **FR-014**: El cierre de caja MUST listar las cuentas vivas con su acción, MUST bloquear solo por pedidos en cocina o listos, y MUST confirmar con una hoja de la app.
- **FR-015**: La aplicación MUST NOT usar `window.confirm`, `window.prompt` ni `window.alert`.
- **FR-016**: Sin conexión o con algo sin guardar, «Enviar» y «Cobrar» MUST estar apagados y un aviso MUST decir por qué.
- **FR-017**: Si la cuenta abierta cambia en otra tableta (cobrada, cancelada, enviada, editada), la pantalla MUST actualizarse y avisar los cambios de estado.
- **FR-018**: Las cuentas guardadas localmente por la versión anterior MUST subirse al servidor una sola vez y el almacenamiento local de cuentas MUST dejar de usarse.
- **FR-019**: El tablero MUST ofrecer «Abrir cuenta», que lleva a Vender con esa cuenta cargada.
- **FR-020**: La cuenta en captura MUST registrar quién la capturó (`opened_by`) desde que nace.
- **FR-021**: Todo lo nuevo MUST caber a 1024×600 con controles de al menos 44 px y sin selector nativo.

### Key Entities

- **Cuenta en captura**: lo que se está capturando, persistido. Tiene nombre, canal, cliente, tipo de servicio, envío, descuento, renglones, quién la abrió, cuándo cambió y una versión. Apunta opcionalmente a un pedido (cuando es «lo nuevo» de un pedido ya enviado). Estados: capturando, enviada, descartada.
- **Renglón en captura**: producto, cantidad, modificadores, nota, con identificador que pone la tableta (idempotencia).
- **Cuenta viva** (vista): unión de cuentas en captura y pedidos no cerrados, con el estado que ve quien atiende: Capturando · En cocina · Pagada · en cocina · Pago parcial · Entregada · debe.

## Success Criteria *(mandatory)*

- **SC-001**: Cero cuentas en captura contadas en ventas, corte, reportes, recetas, «Top» o inventario (test por cada consulta de dinero relevante).
- **SC-002**: Los 30 casos del lienzo tienen al menos un test que falla si el caso se rompe (lista en tasks/quickstart).
- **SC-003**: Recargar la tableta o abrir otra conserva el 100% de lo capturado y guardado.
- **SC-004**: A 1024×600 la fila, el ticket y las hojas nuevas caben sin desbordar (prueba en navegador a ese tamaño).
- **SC-005**: Cero diálogos del sistema en `web/src`.
- **SC-006**: Ningún folio se repite y ninguno se consume por una cuenta descartada.

## Assumptions

- La 031 (defectos de dinero) corrige los casos 20 y 28; esta no los toca.
- La identificación de quién captura sigue siendo la cuenta de usuario compartida (puerta del principio VIII abierta con `opened_by`).
- El tiempo real usa el canal de eventos que ya existe (SSE) más refresco periódico.

## Out of Scope

- Capturar sin conexión (013, decisión del dueño).
- Ventas netas y columnas de fecha en Ventas/Pedidos (029).
- Mesas como concepto.
