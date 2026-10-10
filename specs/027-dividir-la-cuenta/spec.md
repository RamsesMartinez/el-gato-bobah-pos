# Feature Specification: Dividir la cuenta por productos

**Feature Branch**: `027-dividir-la-cuenta`

**Created**: 2026-10-05

**Status**: Draft

**Input**: User description: "Diagnostica por qué en producción no podemos cerrar un pedido […] lo que querían al final era dividir la cuenta, pero el sistema no lo soportaba […] también considera qué pasa si, como hoy, un cliente solo quiere separar algunos productos, o varios clientes quieren separar sus productos de la venta. […] Quiero que salga bien: no por etapas, sino la solución completa, bien pensada y robusta, de una vez, para que queden todos los escenarios."

## El diseño ya está decidido, y se eligió mirándolo

Las tres alternativas se dibujaron a tamaño real (1024 × 600, la tableta) con la misma mesa de
ejemplo, y el dueño eligió sobre los dibujos:
[el lienzo «Dividir la cuenta»](https://claude.ai/artifact/AzeHHQ4fQAFZ1BQ3gYr1xD), fila
**Opción A · Todo desde la hoja de cobro** (tableros A1–A8; A7 y A8 se agregaron a pedido del dueño
para ver los otros dos modos dentro del mismo selector) más la fila **En las tres opciones**
(X1–X4). Se descartaron la B (personas dentro del pedido) y la C (separar en pedidos desde el
tablero).

Este spec no vuelve a discutir el diseño: dice qué tiene que ser cierto cuando esté construido.

## Por qué existe: el incidente del 2026-10-04

Una mesa de tres personas acumuló todo en un solo pedido y al pagar cada quien quiso lo suyo. El
POS no sabía dividir por productos, y la operadora improvisó durante unos 40 minutos:

| Lo que hizo | Lo que costó |
|---|---|
| Abrió dos pedidos nuevos y volvió a capturar ahí los productos de dos personas | El inventario se descontó **dos veces**: lo quitado ya había salido a cocina y no se repuso |
| Quitó del original, uno por uno, todo lo que no era de la primera persona, con el motivo «Ya no lo quiere» | 18 cancelaciones en el reporte, de las cuales 12 no lo fueron |
| Intentó 7 veces cancelar el pedido original | Rebotaba con «conflicto: … cancela los que falten o haz un reembolso», y el reembolso no existe para un pedido abierto |
| — | El original se quedó en «Todo entregado» sin ningún botón y **bloqueó el corte de caja** hasta que se cerró a mano en la base |

El atorón ya está arreglado aparte (commit `f7eb353`: quitar el último renglón pendiente cierra el
pedido). Este spec ataca la causa: **dividir una cuenta no puede requerir cancelar y recapturar**.

Medido sobre la reproducción en el ambiente de pruebas: la improvisación costó **~69 toques**, 21 de
ellos en intentos de cancelar que el sistema iba a rechazar. El diseño elegido resuelve la misma
mesa en **~16** (estimado sobre los tableros).

## Contexto *(no se re-deriva)*

| Hecho | De dónde sale |
|---|---|
| Un pedido ya admite varios pagos, cada uno con su método y su propina, con tope en lo que falta | `OrdersService.Charge`, `domain.ValidarCobro` |
| La hoja de cobro ya divide «Entre N personas» y «Otro monto», pero un pago **no sabe qué productos cubrió**, y lo cobrado vive solo en la pantalla y se pierde al recargar | `web/src/shared/CobrarSheet.tsx` (`yaCobrado`) |
| Todo renglón se marca «enviado a cocina» en la misma transacción que lo inserta, sea o no de preparación | `OrdersService.Create` / `AddLines` |
| Por eso quitar un renglón **nunca** repone inventario en producción: la rama de reposición solo corre en el test que borra esa marca a mano | `CancelarRenglon`, `domain.ReponeInventario` |
| El botón de quitar un renglón lo quita **completo**: de un renglón de 2 no se puede quitar 1 | Reproducción en el ambiente de pruebas, 2026-10-05 |
| Devolver dinero es salida de caja y hoy lo exige admin o gerente | `router.go`, `/orders/{id}/refund` y `/cancel` |
| El presupuesto real es 1024 × 600, controles ≥ 44 px, sin `<select>` nativo | Constitución, *Restricciones del producto* |

## User Scenarios & Testing *(mandatory)*

Los cinco casos que la solución cubre, con los nombres que usa el lienzo:

| Caso | Ejemplo | Historia |
|---|---|---|
| A. Uno separa lo suyo | Paga su bebida y se va; la mesa sigue y pide más | 1, 3 |
| B. Varios separan lo suyo | La mesa del incidente: tres personas, tres pagos, tres tickets | 1, 2 |
| C. Partes iguales o por monto | «Entre los cuatro» | 4 |
| D. Se capturó en la cuenta equivocada | Un producto de una mesa quedó en otra | 5 |
| E. Quiere su propio pedido | Necesita un folio y un ticket independientes | 5 |

### User Story 1 - Cobrar a una persona solo lo suyo (Priority: P1)

En la hoja de cobro, quien cobra elige «Dividir → Por productos», toca los productos de la persona
que va a pagar, elige el método y cobra. El sistema calcula el monto de esa selección, registra qué
productos cubrió ese pago e imprime un ticket con solo esos productos. El pedido sigue abierto con
lo demás.

**Why this priority**: es la feature. Sin esto la operadora vuelve a cancelar y recapturar, que es
justo lo que descuadró el inventario y los reportes el 4 de octubre.

**Independent Test**: un pedido de 11 renglones; se cobra solo el Soju a una persona con tarjeta;
el pedido queda con «Falta» igual al total menos ese producto, el Soju aparece pagado y el ticket
impreso trae solo el Soju.

**Acceptance Scenarios**:

1. **Given** un pedido abierto con varios productos sin pagar, **When** se abre la hoja de cobro y
   se toca «Dividir», **Then** se ofrecen tres modos —Por productos, Entre personas, Por monto— y
   «Por productos» muestra todos los renglones del pedido con una casilla de al menos 48 px de alto.
2. **Given** el modo Por productos, **When** se tocan uno o más productos, **Then** la barra
   inferior dice cuántos productos lleva esa persona y el botón de cobrar muestra el monto que
   **calculó el sistema** para esa selección; el monto no se teclea.
3. **Given** una selección y un método elegidos, **When** se toca Cobrar, **Then** el pago queda
   registrado con los productos y las piezas que cubrió, la propina va aparte del monto, y «Falta»
   baja exactamente en ese monto.
4. **Given** un pago por productos recién cobrado, **When** se imprime su ticket, **Then** el
   ticket trae solo esos productos, el método, la propina si hubo, el número de pago («Pago 2») y
   cuánto queda por pagar del pedido.
5. **Given** un renglón con cantidad 2, **When** se toca, **Then** aparece un contador «1 de 2»
   con − y + de al menos 44 px, que arranca en 1, para pagar solo una pieza.
6. **Given** que ya se cobraron unos productos, **When** se recarga la tableta o se abre el pedido
   en otra, **Then** esos productos siguen apareciendo pagados, con su pago.

---

### User Story 2 - Lo pagado no se cobra dos veces y queda a la vista (Priority: P1)

Mientras se divide, lo que ya se pagó se queda en la lista en gris, con el pago que lo cubrió, y no
se puede volver a seleccionar. El último en pagar toca «Todo lo que falta».

**Why this priority**: es lo que hace confiable a la historia 1. Sin esto, una tableta recargada a
media división o dos tabletas cobrando la misma mesa cobrarían un producto dos veces, y el error
saldría hasta el corte.

**Independent Test**: se cobra un producto en una tableta y, sin recargar la otra, se intenta
cobrar el mismo producto desde ella: el sistema lo rechaza con un mensaje que dice que ya se pagó.

**Acceptance Scenarios**:

1. **Given** productos ya pagados, **When** se mira el modo Por productos, **Then** aparecen en gris,
   no seleccionables, con el pago que los cubrió («Pagado · Tarjeta débito»).
2. **Given** pagos hechos, **When** se mira el encabezado de la hoja, **Then** cada pago aparece
   como una ficha tocable con su número, método y monto.
3. **Given** dos tabletas sobre el mismo pedido, **When** las dos intentan cobrar la misma pieza,
   **Then** solo un cobro pasa; el otro recibe «Ese producto ya se pagó» y su pantalla se actualiza.
4. **Given** productos sin pagar, **When** se toca «Todo lo que falta», **Then** se seleccionan
   todos los que faltan y el monto es exactamente lo que falta del pedido, incluido lo que no es
   producto (el envío).

---

### User Story 3 - El pedido sigue vivo después de un pago parcial (Priority: P1)

Una persona pagó lo suyo y se fue; los demás siguen en la mesa y piden más. Los productos nuevos se
agregan al mismo pedido como hoy, quedan sin pagar y aparecen en la siguiente división. Lo pagado
no se toca.

**Why this priority**: es el caso A del dueño, y el que más va a pasar: rara vez todos pagan al
mismo tiempo.

**Independent Test**: tras un pago por productos se agregan dos productos al pedido; la hoja de
cobro los muestra sin pagar, el pago anterior sigue intacto y «Falta» suma los nuevos.

**Acceptance Scenarios**:

1. **Given** un pedido con un pago parcial, **When** se le agregan productos desde Vender, **Then**
   los productos nuevos llegan sin pagar y la cocina recibe solo los nuevos, como hoy.
2. **Given** ese pedido, **When** se mira en «Pedidos por cobrar», **Then** dice cuánto se pagó y
   cuánto falta, y no se presenta como «1 de 3»: el sistema no sabe cuántas personas pagarán.
3. **Given** productos ya pagados, **When** alguien intenta quitarlos del pedido, **Then** no se
   puede sin antes devolver ese pago (historia 6).

---

### User Story 4 - Partes iguales y por monto siguen funcionando (Priority: P2)

«Entre personas» y «Por monto» siguen como hoy, ahora dentro del mismo selector, y pueden convivir
con pagos por productos en el mismo pedido.

**Why this priority**: ya existe y se usa; romperlo sería un retroceso. Va en P2 porque el trabajo
es de convivencia, no de capacidad nueva.

**Independent Test**: un pedido se cobra en dos partes iguales; ambas quedan registradas como pagos
sin productos y el pedido queda saldado.

**Acceptance Scenarios**:

1. **Given** el selector de modo, **When** se elige Entre personas, **Then** se reparte **lo que
   falta** —no el total— entre N personas con − y + de al menos 56 px, se ven las partes como
   fichas («Parte 1 de 3 · $177.00 · Pagada», «Parte 2 de 3 · Se cobra ahora», «Parte 3 de 3 · Por
   cobrar») y la última parte absorbe el centavo del redondeo (tablero A7).
2. **Given** partes ya cobradas, **When** se recarga la tableta, **Then** las partes pagadas siguen
   marcadas como pagadas; hoy viven solo en la pantalla y se pierden.
3. **Given** el modo Por monto, **When** se abre, **Then** hay un teclado numérico de teclas de al
   menos 52 px, montos rápidos y «Lo que falta», y se ve cuánto faltará después de ese pago
   (tablero A8). Ese pago no cubre productos y su ticket dice el monto, no productos.
4. **Given** un pago por monto ya hecho, **When** después se cobra por productos, **Then** el monto
   de esa selección no puede pasar de lo que falta, y si lo pasaría el sistema lo dice en palabras
   de quien opera («Ya se cobraron $X sin elegir productos. Esta selección pasa de lo que falta:
   usa «Todo lo que falta»») en lugar de cobrar de más.

---

### User Story 5 - Pasar productos a otro pedido o a uno nuevo (Priority: P2)

Desde la misma selección del modo Por productos, «Pasar a otro pedido» abre una lista de filas
grandes con «+ Pedido nuevo» arriba y los pedidos abiertos debajo. Los productos elegidos se
**mueven**: no se cancelan ni se vuelven a capturar.

**Why this priority**: cubre los casos D y E, que hoy se resuelven igual que el incidente. Va en P2
porque el caso común (A y B) se resuelve cobrando, sin mover nada.

**Independent Test**: se pasan dos productos ya enviados a cocina a un pedido nuevo; la cocina no
recibe una comanda nueva, el inventario no cambia, el pedido de origen baja su total y el nuevo nace
con esos dos productos.

**Acceptance Scenarios**:

1. **Given** productos seleccionados sin pagar, **When** se toca «Pasar a otro pedido», **Then** se
   abre una lista de filas de al menos 56 px con un buscador, «+ Pedido nuevo» y cada pedido abierto
   con nombre, número, cuántos productos lleva y su total.
2. **Given** que se elige un destino, **When** se confirma, **Then** los productos dejan el pedido
   de origen y aparecen en el destino con su estado de cocina y de entrega intactos: lo que ya salió
   a cocina no se vuelve a preparar y lo entregado sigue entregado.
3. **Given** ese movimiento, **When** se revisa el inventario, **Then** no hubo ningún descuento ni
   reposición: el consumo viaja con el producto.
4. **Given** «+ Pedido nuevo», **When** se confirma, **Then** nace un pedido con nombre y número
   propios, en el mismo turno de caja, y se cobra e imprime como cualquier otro.
5. **Given** que el pedido de origen se queda sin nada pendiente, **When** termina el movimiento,
   **Then** se cierra solo si ya está todo entregado y pagado, igual que al quitar el último
   pendiente.
6. **Given** un error al pasar, **When** se pasan de vuelta los mismos productos, **Then** el
   resultado es el de antes, sin recapturar nada.
7. **Given** que se eligieron **todos** los productos del pedido, **When** se mira «+ Pedido nuevo»,
   **Then** no se ofrece y dice «Ya es su propio pedido; no hace falta pasarlo».
8. **Given** que se pasan **todos** los productos a un pedido existente (cuenta equivocada), **When**
   termina el movimiento, **Then** el pedido de origen queda cerrado como **juntado con** el destino,
   sin reponer inventario, y el reporte de cancelaciones no lo cuenta.

---

### User Story 6 - Corregir un cobro hecho a la persona equivocada (Priority: P2)

Tocar la ficha de un pago abre su detalle —productos, método, hora— con dos acciones: «Reimprimir
su ticket» y, separada y en rojo, «Devolver este pago». Devolverlo regresa el dinero por el mismo
método y deja sus productos otra vez por cobrar.

**Why this priority**: sin esto, un dedazo al dividir se corrige cancelando y recapturando, que es
el incidente otra vez.

**Independent Test**: se cobra un producto a la persona equivocada, se devuelve ese pago y se cobra
a la correcta; el corte muestra el cobro, la devolución y el cobro bueno, y el neto del método es el
correcto.

**Acceptance Scenarios**:

1. **Given** un pago hecho, **When** se toca su ficha, **Then** se ve su detalle y las dos acciones,
   con «Devolver este pago» separada de la otra.
2. **Given** «Devolver este pago», **When** lo confirma un usuario con el permiso «Devolver pagos»
   y elige un motivo, **Then** el pago queda devuelto —con su rastro—, sus productos vuelven a quedar por cobrar, y
   el corte deja de esperar ese dinero en su método y lo lista como pago devuelto.
3. **Given** un usuario sin ese permiso, **When** abre el detalle del pago, **Then** «Devolver este
   pago» aparece deshabilitado con «Tu usuario no puede devolver pagos», y el sistema lo rechaza
   aunque se salte la pantalla.
4. **Given** un pago con tarjeta, **When** se confirma devolverlo, **Then** la pantalla dice que el
   reembolso en la terminal se hace aparte; con efectivo, dice cuánto regresar al cliente.

---

### User Story 7 - Ningún pedido se queda sin salida (Priority: P1)

Las pantallas comunes del lienzo (X1–X3): una tarjeta sin nada pendiente ofrece «Cerrar pedido»;
«Cancelar pedido» con algo entregado ofrece lo que sí se puede hacer; y se puede quitar una pieza de
un renglón de 2.

**Why this priority**: es lo que convirtió el problema de dividir en un incidente. Va en P1 aunque
exista el arreglo del servidor, porque ese arreglo evita el estado nuevo, pero la pantalla tiene que
dar salida a cualquier pedido que llegue a él por otro camino.

**Independent Test**: un pedido con todo lo vivo entregado, en estado abierto (forzado en la base de
prueba), muestra «Cerrar pedido», y al tocarlo se cierra.

**Acceptance Scenarios**:

1. **Given** una tarjeta del tablero sin nada pendiente de entregar, **When** se mira, **Then**
   ofrece una salida según debe o no y según el tablero cobre (FR-018), nunca solo
   el menú; con muchos productos, los botones siguen a la vista.
2. **Given** un pedido con algo entregado, **When** se toca «Cancelar pedido», **Then** en lugar de
   un rechazo se ofrece «Quitar los N que faltan» con su motivo, y el texto dice que lo entregado no
   se puede cancelar; ningún mensaje de la pantalla dice «conflicto:» ni sugiere un reembolso que no
   aplica.
3. **Given** un renglón con cantidad 2, **When** se toca quitar, **Then** se elige cuántas piezas
   quitar («1 de 2») y los motivos son filas de al menos 44 px, sin uno elegido de antemano.
4. **Given** un usuario sin el permiso «Cancelar pedidos», **When** abre el menú de la tarjeta,
   **Then** no se le ofrece «Cancelar pedido», y el sistema lo rechaza aunque se salte la pantalla.

---

### User Story 8 - Quitar un producto devuelve lo que no se consumió (Priority: P2)

Quitar un producto que no lleva preparación —un refresco del refri— devuelve su existencia al
almacén. Lo que sí se preparó sigue sin reponerse, como hoy.

**Why this priority**: es el otro daño del incidente. Hoy ningún producto quitado repone nada,
porque todo se marca «enviado a cocina» al capturarse.

**Independent Test**: se captura y se quita un refresco embotellado; su existencia vuelve a la de
antes. Se captura y se quita un frappé ya enviado; su consumo no se repone.

**Acceptance Scenarios**:

1. **Given** un producto sin preparación capturado y no entregado, **When** se quita, **Then** su
   existencia en el almacén vuelve a la de antes de capturarlo.
2. **Given** un producto con preparación ya enviado a cocina, **When** se quita, **Then** no se
   repone y la pantalla lo dice como hoy («Ya se está preparando…»).
3. **Given** que se quita solo una pieza de un renglón de 2, **When** se repone, **Then** se repone
   solo esa pieza.

---

### Edge Cases

Pensados antes del diseño, por familia (constitución IV):

**El valor vacío que significa algo**
- Cobrar por productos sin seleccionar ninguno: el botón no se habilita; el servidor rechaza una
  selección vacía como 400, no como un pago de $0.
- Una selección de piezas en 0 («0 de 2») no cuenta como producto elegido.

**El estado que no sobrevive**
- Recargar a media división, o que se caiga la red tras tocar Cobrar: el pago o se registró
  completo (con sus productos) o no se registró; reintentar con el mismo toque no cobra dos veces.
- Dos tabletas sobre el mismo pedido: lo pagado se valida en el servidor, no en la pantalla.

**El camino nuevo que se salta el control viejo**
- Pasar productos a otro pedido no puede servir para sacar dinero: no se mueve un producto pagado;
  y no se mueve nada si, sin esos productos, el pedido de origen quedaría con más pagado que su
  total.
- Devolver un pago exige el permiso «Devolver pagos», también por la API directa.
- Un pedido de plataforma (Uber, DiDi, Rappi) no se divide ni recibe productos movidos: su folio y
  su liquidación son por pedido. La opción no se ofrece y el servidor lo rechaza.
- Un pedido de un turno de caja ya cerrado no se divide ni se mueve: el servidor rechaza cobrar por
  productos o por partes y pasar productos desde él.
- No se puede pasar productos a un pedido de otra empresa ni a uno cerrado, cancelado o reembolsado.

**El hermano que no se movió**
- Quitar un renglón de un pedido que ya tiene pagos: no puede dejar el total por debajo de lo
  pagado sin devolver la diferencia, igual que ya lo exige el descuento. Se verifica primero con un
  test contra la base (hoy no está verificado si pasa).
- «Entregar todo» en un pedido sin productos vivos no lo puede cerrar como venta de $0; ese pedido
  se cierra con «Cerrar pedido» como cancelación (si no tiene pagos), y lo puede hacer cualquier rol.
- Un pedido cobrado, al que se le devolvió el pago y luego se vació, sigue diciendo «Tiene pagos»
  y «Cerrar pedido» lo rechaza: lo pagado no resta devoluciones (hallazgo fuera de alcance en
  research.md). Falla hacia lo seguro; lo cierra quien puede cancelar pedidos.
- El monto que se vio antes de cobrar puede quedar viejo si otra tableta cobró o agregó algo: se
  cobra lo que calcula el servidor al cobrar, y la pantalla lo dice.
- Quitar un producto pagado: no se permite; primero se devuelve su pago.
- El descuento del pedido se reparte entre sus productos en proporción a su precio, y cada pago por
  productos paga su parte ya descontada; el redondeo cae en el último pago para que la suma de los
  pagos sea exactamente el total.
- El envío no es un producto: lo cubre «Todo lo que falta».
- La propina de cada pago es de ese pago, no del pedido, y no entra en el total de venta.
- El reporte de cancelaciones no cuenta como cancelado lo que se pasó a otro pedido.
- Pasar **todos** los productos: hacia un pedido nuevo no cambia nada (mismo folio, mismo turno) y se
  rechaza; hacia uno existente deja el origen vacío, que se cierra como juntado con el destino y no
  como cancelación.
- «Todo lo que falta» cuando ya no queda producto sin pagar pero sí saldo (p. ej. tras devolver un
  pago por monto): cobra el saldo, no se trata como selección vacía.

## Requirements *(mandatory)*

### Functional Requirements

**Dividir por productos**

- **FR-001**: La hoja de cobro MUST ofrecer, dentro de «Dividir», los modos Por productos, Entre
  personas y Por monto.
- **FR-002**: En Por productos, quien cobra MUST poder elegir productos y, en renglones con
  cantidad mayor a 1, cuántas piezas de cada uno.
- **FR-003**: El sistema MUST calcular en el servidor el monto de una selección, con el descuento
  del pedido ya repartido; el monto que mande la pantalla se ignora. Quien cobra MUST ver ese monto
  **antes** de cobrar, calculado por el servidor con la misma regla; si el pedido cambió entre que lo
  vio y el cobro, se cobra lo que el servidor calcule en ese momento y la pantalla lo muestra.
- **FR-004**: Cada pago MUST guardar qué productos y cuántas piezas cubrió, y eso MUST sobrevivir a
  recargar la pantalla y verse desde cualquier tableta.
- **FR-005**: El sistema MUST rechazar cobrar una pieza que ya está cubierta por otro pago, también
  cuando dos cobros llegan al mismo tiempo.
- **FR-006**: «Todo lo que falta» MUST cobrar exactamente lo que falta del pedido, incluidos el
  envío y lo pagado sin productos.
- **FR-007**: El monto de un pago por productos MUST NOT superar lo que falta del pedido.
- **FR-008**: Cada pago MUST poder imprimir su propio ticket con solo sus productos, su número de
  pago, el método, la propina y lo que queda por pagar del pedido.
- **FR-009**: Un pedido con pagos parciales MUST seguir recibiendo productos, sin alterar los pagos
  existentes.

**Pasar a otro pedido**

- **FR-010**: Desde una selección de productos sin pagar, quien cobra MUST poder pasarlos a un
  pedido abierto existente o a uno nuevo.
- **FR-011**: Pasar productos MUST conservar su estado de cocina y de entrega, y MUST NOT generar
  comanda nueva, descuento ni reposición de inventario.
- **FR-012**: El pedido nuevo MUST nacer en el mismo turno de caja y con nombre y número propios.
- **FR-013**: El sistema MUST rechazar pasar productos pagados, de o hacia un pedido de plataforma,
  de un turno cerrado, hacia un pedido que no esté abierto, que dejen el origen con más pagado
  que su total, todos los productos de un pedido hacia un pedido nuevo, o todos hacia uno
  existente cuando el origen cobra envío.
- **FR-014**: Tras pasar productos, cada pedido involucrado MUST cerrarse solo si ya no le falta
  nada por entregar y está pagado, con la misma regla que entregar el último producto.
- **FR-015**: Debe quedar registro de qué productos se pasaron, de qué pedido a cuál, quién y
  cuándo, y el reporte de cancelaciones MUST NOT contarlos como cancelados. Si se pasan todos los
  productos a un pedido existente, el origen MUST cerrarse como **juntado con** ese pedido —con
  registro de con cuál— y MUST NOT contar como cancelación en ningún reporte.

**Corregir un pago**

- **FR-016**: Un pago MUST poder devolverse completo, por el mismo método, solo por un usuario cuyo
  rol tenga el permiso «Devolver pagos» (hoy, admin y gerente); el control vive en el servidor y se
  pregunta por el permiso, nunca por el nombre del rol.
- **FR-017**: Un pago devuelto MUST quedar registrado —quién, cuándo, por qué y qué cubría—, sus
  productos MUST quedar otra vez por cobrar, y MUST dejar de contar en el esperado del corte de su
  método (el dinero regresó en ese momento por el mismo método); el corte MUST listar los pagos
  devueltos del turno. Solo se devuelve así un pago del turno abierto: el de un turno ya cerrado va
  por la devolución de siempre.

**Ningún pedido sin salida**

- **FR-018**: Una tarjeta del tablero sin nada pendiente de entregar MUST ofrecer siempre una salida:
  «Cerrar pedido» si no debe, «Cobrar $X» si debe y el tablero cobra, o el texto «Falta cobrar $X
  en caja» si debe y el tablero no cobra. Un pedido **sin productos vivos y sin pagos** MUST poder
  cerrarlo cualquier rol con «Cerrar pedido»; queda cancelado con el motivo «Sin productos» y cuenta
  como cancelación. Con pagos, la tarjeta dice «Tiene pagos por devolver».
- **FR-019**: «Cancelar pedido» con algo entregado MUST ofrecer quitar lo que falta por entregar en
  una sola acción, con motivo; ningún texto de la pantalla MUST mostrar «conflicto:» ni sugerir una
  acción que en ese estado no existe.
- **FR-020**: Quitar un renglón con cantidad mayor a 1 MUST permitir elegir cuántas piezas quitar.
- **FR-021**: Los motivos de quitar MUST mostrarse como filas de al menos 44 px, sin uno elegido de
  antemano.

**Defectos que se corrigen**

- **FR-022**: Quitar un producto sin preparación y sin entregar MUST reponer su existencia; quitar
  uno con preparación ya enviado a cocina MUST NOT reponerla.
- **FR-023**: Quitar un renglón de un pedido con pagos MUST NOT dejar su total por debajo de lo
  pagado.
- **FR-024**: «Entregar todo» MUST NOT cerrar como entregado un pedido sin productos vivos.

**Entre personas y por monto**

- **FR-026**: Entre personas MUST repartir lo que falta, no el total, y el reparto en curso (cuántas
  partes, cuáles ya se pagaron) MUST sobrevivir a recargar la pantalla; la suma de las partes MUST
  ser exactamente lo que faltaba.
- **FR-027**: Por monto MUST ofrecer teclado numérico táctil, montos rápidos y «Lo que falta», y
  mostrar cuánto faltará después del pago antes de cobrarlo.

**Reglas que salieron de la revisión de arquitectura**

- **FR-028**: Todo control nuevo de esta feature MUST preguntar por un permiso, no por el nombre de
  un rol, en el servidor y en la pantalla. «Cancelar pedido», que la tarjeta del tablero ofrece,
  pasa también al permiso «Cancelar pedidos» (hoy, admin y gerente).
- **FR-029**: Un pedido con pagos MUST NOT aceptar que se aplique o cambie su descuento.
- **FR-030**: Cada pago MUST conservar su número dentro del pedido; un pago devuelto MUST seguir a la
  vista, tachado, con su número.
- **FR-031**: Pasar productos MUST pedir confirmar el destino, y MUST avisar antes de abrir la lista
  cuando lo elegido no se puede pasar.
- **FR-032**: En la lista Por productos, lo pagado MUST ir al final, compacto, detrás de lo pendiente.

**Restricciones de pantalla**

FR-025 conserva su número aunque va al final: aplica a todas las pantallas de arriba, y renumerarlo
rompería las referencias del plan y las tareas.

- **FR-025**: Toda pantalla nueva o cambiada MUST caber en 1024 × 600, con controles de al menos
  44 px y sin selectores nativos; las listas para elegir pedido usan la hoja de filas grandes del
  POS.

### Key Entities

- **Pago**: ya existe. Gana la lista de productos y piezas que cubrió (vacía para los pagos por
  monto o entre personas) y un estado de devuelto, con quién y cuándo.
- **Pieza cubierta**: la relación entre un pago y un renglón del pedido, con cuántas piezas y
  cuánto dinero de ese renglón pagó. Es la que impide cobrar dos veces lo mismo.
- **Movimiento de productos entre pedidos**: registro de qué renglones (y cuántas piezas) pasaron
  de un pedido a otro, quién y cuándo. Es lo que distingue «se pasó a otra cuenta» de «se canceló».
- **Pedido**: ya existe. Gana con qué pedido se juntó cuando se le pasaron todos sus productos a
  otro; así un reporte distingue «se juntó» de «se canceló».
- **Renglón**: ya existe. Una pieza movida o pagada parcialmente puede obligar a partir un renglón
  en dos; la copia conserva precio, costo, modificadores y estado de cocina.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: La mesa del incidente (3 personas, 11 renglones, pagos de 1, 4 y 7 productos) se
  cobra en **20 toques o menos** desde que se abre la hoja de cobro, con tarjeta y el
  último pago con «Todo lo que falta», contra ~69 de la improvisación.
- **SC-002**: Dividir una cuenta, de cualquiera de los cinco casos, deja **cero cancelaciones** en
  el reporte y **sin cambio neto en existencias** (partir un renglón inserta pares de movimientos
  que se anulan), también cuando se pasan todos los
  productos a otro pedido y el origen queda juntado.
- **SC-003**: En ninguna secuencia de cobrar, quitar, pasar y devolver queda un pedido abierto sin
  una acción visible que lo cierre: lo vigila una prueba que recorre esas secuencias.
- **SC-004**: La suma de los pagos de un pedido dividido es exactamente su total, al centavo, con y
  sin descuento.
- **SC-005**: El corte de caja de un turno con pagos divididos y una devolución cuadra por método
  sin ajuste manual.
- **SC-006**: Cobrar la misma pieza desde dos tabletas al mismo tiempo produce un solo pago.

## Assumptions

- La regla «el último pago absorbe el redondeo del descuento» se toma por defecto; si el dueño
  prefiere otra, cambia el reparto, no la pantalla.
- Devolver un pago exige el permiso «Devolver pagos», que hoy tienen admin y gerente. Consecuencia que
  no se ve en el dibujo: si el dedazo lo hace un cajero solo en el turno, tiene que llamar a alguien
  con ese permiso. «Cancelar pedido» pasa al permiso «Cancelar pedidos», con los mismos roles de hoy.
- Pasar todos los productos a un pedido existente cierra el origen como juntado (decidido por el
  dueño el 2026-10-05); hacia un pedido nuevo no se permite porque no cambia nada.
- **Los roles serán configurables por empresa** (decidido por el dueño el 2026-10-05): cada control
  nuevo de esta feature se pregunta por permiso, y el mapa rol → permisos de hoy es lo único que
  pasará a la base. Es una puerta nueva de la constitución.
- Quién opera es el mismo usuario de la tableta; el pago registra quién lo cobró como hoy. Saber
  qué *persona de la mesa* pagó no se modela (se descartó la opción B).
- Corregir los datos del 4 de octubre en producción queda fuera: es un ajuste de datos con su propio
  SQL revisado, no parte de la feature.
- Las mesas como concepto quedan fuera.
