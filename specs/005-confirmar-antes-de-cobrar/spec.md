# Feature Specification: Confirmar el pedido antes de cobrar, y verlo en curso

**Feature Branch**: `005-confirmar-antes-de-cobrar`

**Created**: 2026-09-01

**Status**: Draft

**Input**: Ver [Contexto medido](#contexto-medido). Las cuatro decisiones de diseño (A1, B2, C1, D1)
las tomó el dueño antes de este spec y aquí se implementan, no se reabren.

## Contexto medido

Tres números del negocio en operación, del 29 al 31 de agosto de 2026:

| Qué | Cuánto |
| --- | --- |
| Pedidos por día | 4 a 10 |
| Renglones por pedido | 2.2 de promedio, 6 el máximo |
| Pedidos que recibieron un renglón después de abrirse | **0** |

Ese cero es el problema. Agregarle algo a un pedido ya mandado **se puede** hoy, pero cuesta cinco
toques por un camino que vive dentro de la pantalla de cobro, y nadie lo ha usado nunca. Y en
paralelo, cobrar sin que cocina se entere también se puede: es el camino corto y es el que se usa.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - El pedido confirmado sigue a la vista (Priority: P1)

Quien atiende arma la cuenta, la confirma, y el pedido **no desaparece**: queda en la barra de
pedidos en curso, identificado por su nombre de folio. Cuando el cliente pide algo más, un toque
sobre ese nombre vuelve a abrir el pedido y se le agrega sin pasar por ninguna pantalla de dinero.

**Why this priority**: Es la que entrega valor sola y sin forzar nada. Hoy el pedido se desvanece al
mandarlo y recuperarlo cuesta cinco toques; con esto cuesta uno. Y es requisito de la US2: exigir
confirmar antes de cobrar sin haber resuelto esto dejaría el flujo peor de lo que está.

**Independent Test**: Se confirma un pedido, se verifica que aparece en la barra con su folio, se le
agrega un producto desde ahí y se comprueba que el pedido quedó con los renglones nuevos — todo sin
abrir la pantalla de cobro.

**Acceptance Scenarios**:

1. **Given** una cuenta con productos, **When** se confirma, **Then** el pedido aparece en la barra
   de pedidos en curso con su nombre de folio y su monto, y la cuenta local queda vacía y lista para
   el siguiente cliente.
2. **Given** un pedido en curso en la barra, **When** se toca su folio, **Then** el POS lo abre y
   los productos que se agreguen se suman a ese pedido, no a uno nuevo.
3. **Given** un pedido en curso creado en la primera tableta, **When** se mira la segunda tableta,
   **Then** el mismo pedido aparece en su barra con el mismo folio y monto.
4. **Given** un pedido en curso abierto en el POS, **When** se recarga la aplicación, **Then** el
   pedido sigue en la barra y sigue siendo el mismo — no se duplica ni se pierde.

---

### User Story 2 - Cobrar exige haber confirmado (Priority: P1)

No se puede cobrar un pedido que cocina no ha visto. La pantalla de cobro deja de poder crear un
pedido: solo cobra pedidos que ya existen y ya se confirmaron.

**Why this priority**: Es el objetivo de la feature. Hoy el camino corto —agregar y cobrar— se salta
la comanda por completo, y es el que se usa. Depende de la US1 para no cobrar toques de más.

**Independent Test**: Se intenta cobrar una cuenta que nunca se confirmó y el servidor lo rechaza,
no solo la pantalla.

**Acceptance Scenarios**:

1. **Given** una cuenta armada y sin confirmar, **When** se busca cobrarla, **Then** el POS ofrece
   confirmar primero, y la acción de cobrar no está disponible.
2. **Given** una petición de cobro construida a mano contra un pedido inexistente, **When** llega al
   servidor, **Then** se rechaza — la barrera vive en el servidor, no en la pantalla.
3. **Given** un pedido en curso ya confirmado, **When** se cobra, **Then** se cobra sin pasos
   adicionales y el pedido sale de la barra de pedidos en curso.

---

### User Story 3 - Lo que se agrega sale a cocina solo, marcado (Priority: P2)

Cuando se le agrega algo a un pedido que ya se confirmó, la comanda que sale lleva **únicamente lo
agregado**, marcada como agregado y con el mismo nombre de folio, para que cocina la junte con lo
que ya está preparando y no vuelva a preparar lo anterior.

**Why this priority**: Sin esto, la US1 le crea un problema a cocina: o no se entera de lo agregado,
o vuelve a preparar el pedido entero. Es P2 porque la US1 ya entrega valor con la comanda completa
manual mientras esto no exista.

**Independent Test**: Se confirma un pedido de dos productos, se le agrega un tercero, y se verifica
que el segundo papel trae solo ese tercer producto, el mismo folio, y la marca de agregado.

**Acceptance Scenarios**:

1. **Given** un pedido confirmado con dos productos, **When** se le agrega un tercero, **Then** la
   comanda que sale lleva solo el tercero, con el mismo folio y una marca visible de que es un
   agregado.
2. **Given** ese mismo pedido, **When** se pide la reimpresión desde el tablero de pedidos,
   **Then** sale la comanda completa con los tres productos.
3. **Given** un pedido al que ya se le agregó, **When** cocina lee los dos papeles, **Then** el
   folio es el mismo en ambos.

---

### User Story 4 - El negocio nuevo nace imprimiendo la comanda (Priority: P3)

Una empresa que se da de alta empieza con la impresión de comanda encendida, en vez de tener que
descubrir el ajuste.

**Why this priority**: Es un cambio de default, no de comportamiento. No afecta a ningún negocio que
ya exista — esos conservan lo que tengan configurado.

**Independent Test**: Se provisiona una empresa nueva y se verifica que su ajuste de comanda nace
encendido, y que el de una empresa existente no cambió.

**Acceptance Scenarios**:

1. **Given** una empresa recién dada de alta, **When** se leen sus ajustes de impresión, **Then** la
   comanda de cocina está encendida.
2. **Given** una empresa que ya existía con la comanda apagada, **When** se despliega esta feature,
   **Then** sigue apagada.

---

### User Story 5 - Cobrar un pedido en curso, entero o por pedazos (Priority: P1)

Quien cobra abre un pedido en curso y elige **cuánto cobra ahora**: todo, o el pedazo que le toca al
comensal que está pagando. Elige con qué paga, deja propina si la hay, y cobra. Si queda saldo, la
hoja no se cierra: se prepara para el siguiente comensal con lo que falta.

**Why this priority**: es P1 porque **esta feature lo rompió**. Al mover el cobro a un pedido que ya
existe, el camino principal quedó con la hoja mínima —método de pago y con cuánto paga— y perdió
dividir la cuenta y la propina, que solo existían en la pantalla del carrito. No es una carencia
heredada: es un hueco que abrió el cambio de flujo, y el dueño lo reportó desde la tableta.

**Independent Test**: se confirma un pedido de $500, se cobra la mitad con tarjeta y la otra mitad en
efectivo con propina, y se verifica que el pedido queda saldado, que las dos propinas quedaron
atribuidas a su método, y que el corte de caja las separa del ingreso.

**Acceptance Scenarios**:

1. **Given** un pedido en curso de $500 sin cobrar, **When** quien cobra elige "Entre 2" y cobra con
   tarjeta, **Then** el pedido queda con $250 de saldo, la hoja sigue abierta y ofrece cobrar $250.
2. **Given** ese mismo pedido con $250 de saldo, **When** se cobra en efectivo con $300 recibidos,
   **Then** la pantalla dice cuánto es el cambio y el pedido queda saldado.
3. **Given** un cobro en efectivo donde el cliente dice "quédese con el cambio", **When** quien cobra
   toca ese botón, **Then** el excedente se registra como propina y no como venta.
4. **Given** que otra caja cobró el mismo pedido mientras esta hoja estaba abierta, **When** se
   intenta cobrar, **Then** la pantalla lo dice con palabras del operador y muestra el estado real.
5. **Given** un cobro cuya respuesta se perdió, **When** quien cobra vuelve a tocar, **Then** el
   sistema reconoce que ese cobro ya entró y no lo registra dos veces.

### Edge Cases

Las formas de fallar, enumeradas antes de escribir nada. Cada una deja su test.

#### El valor vacío que significa algo

- **Confirmar una cuenta sin renglones.** Un pedido de cero productos ocuparía un folio, aparecería
  en la barra y sacaría una comanda en blanco. Se rechaza en el servidor, no solo deshabilitando el
  botón.
- **Agregar cero renglones a un pedido en curso.** No es un error del operador, es un doble toque:
  no debe sacar una comanda vacía ni tocar el pedido.

#### El estado que no sobrevive

- **Recargar la tableta con un pedido abierto en el POS.** El pedido vive en el servidor, así que
  tiene que seguir ahí y seguir siendo el mismo. Lo que no puede pasar es que la recarga cree un
  pedido duplicado.
- **La tableta se suspende con el pedido abierto y despierta media hora después.** Mientras dormía,
  la otra tableta pudo cobrarlo o entregarlo. Al despertar, agregarle tiene que fallar con un
  mensaje que diga qué pasó, no crear un renglón sobre un pedido terminado.
- **Se pierde la red al confirmar.** No puede quedar el pedido creado en el servidor y la cuenta
  local intacta: eso produce dos pedidos con lo mismo cuando el operador reintenta.

#### El camino nuevo que se salta el control viejo

- **Cobrar desde el tablero de pedidos.** Ese camino cobra pedidos que ya existen, así que ya están
  confirmados; pero al mover la barrera hay que comprobar que sigue funcionando y que no quedó
  pidiendo una confirmación que ahí no aplica.
- **Agregar renglones a un pedido ya entregado o cancelado.** Son estados terminales; el agregado se
  rechaza.

  > **El ENTREGADO se abrió por decisión del dueño (2026-09-02).** El cliente que ya recibió su
  > comida y sigue en la mesa pide una más; mandarla como pedido aparte deja dos cuentas para la
  > misma mesa y una de las dos se pierde de vista. Su dinero tampoco estaba cerrado: el pago se
  > registra al cobrar, no al entregar. El pedido que recibe renglones **vuelve a `abierta`**
  > (`domain.ReabreAlAgregar`) — si se quedara en `entregada`, el tablero no lo listaría y nadie
  > prepararía la comida recién pedida. Cancelada y reembolsada siguen rechazando: ahí el dinero ya
  > lo contó un arqueo firmado.
- **Agregar renglones a un pedido ya cobrado por completo.** Se permite —el cliente pidió más—, y el
  pedido reaparece en la barra con el saldo nuevo. Lo que no puede es quedar cobrado con renglones
  que nadie pagó y sin que se vea.

  > **Revertido por decisión del dueño (2026-09-02).** La hoja del botón naranja abría con todos los
  > pedidos en curso —medido en pruebas: 30 renglones, 14 ya cobrados, sobre una pantalla donde caben
  > cinco— y ahora el servidor solo manda lo que falta por cobrar (`GET /orders/open?porCobrar=true`).
  > El endpoint `POST /orders/:id/lines` sigue aceptando el pedido saldado, pero la aplicación ya no
  > tiene ningún control que llegue a él: esa venta se captura como un pedido aparte. Se anota aquí y
  > no solo en el código porque el requisito de arriba prometía lo contrario.
- **Las dos tabletas agregan al mismo pedido al mismo tiempo.** Los agregados se suman, no se pisan:
  el pedido termina con los renglones de las dos. Cada tableta ve el pedido completo al refrescar.
- **Una tableta agrega mientras la otra cobra.** El agregado que llega después del cobro deja saldo
  pendiente y el pedido vuelve a la barra; el cobro no se pierde ni se duplica.

#### El hermano que no se movió

- **El selector "agregar a un pedido en curso" de la pantalla de cobro.** Es el camino viejo que
  nadie usa. Al llegar el nuevo, hay que quitarlo o dejarlo consistente: dos caminos para lo mismo,
  uno de ellos escondido, es de dónde salen los defectos.
- **La píldora "Por cobrar" del encabezado.** Muestra exactamente lo mismo que la barra nueva. Si se
  queda, la misma información vive en dos lugares y en 1024×600 eso es alto que se le quita a la
  lista de productos.
- **Los pedidos que ya existen en producción.** Nacieron sin pasar por "confirmar". No pueden
  cambiar de significado ni quedar bloqueados.

#### La impresora

- **La comanda no sale al confirmar.** El pedido **queda confirmado igual** y el operador ve un
  aviso que dice que no salió y cómo sacarla. Revertir la confirmación perdería el pedido con el
  cliente enfrente, y en producción gana la opción que no pierde datos. Es el mismo criterio que ya
  se aplicó al ticket del cliente.

## Requirements *(mandatory)*

### Functional Requirements

#### Confirmar

- **FR-001**: El sistema MUST exigir que un pedido esté confirmado antes de poder cobrarlo, para
  todos los pedidos, sin condicionar por producto ni por tipo de servicio.
- **FR-002**: El sistema MUST rechazar en el servidor un cobro sobre un pedido que no existe o que
  no se confirmó. La pantalla puede esconder el botón, pero la barrera vive en el servidor.
- **FR-003**: El sistema MUST rechazar confirmar una cuenta sin renglones.
- **FR-004**: Al confirmar, el sistema MUST producir la comanda de cocina si el negocio la tiene
  encendida, y MUST dejar el pedido confirmado aunque la impresión falle.
- **FR-005**: Cuando la comanda no sale, el sistema MUST avisarlo de forma visible y accionable, sin
  bloquear la operación.

#### El pedido en curso

- **FR-006**: El sistema MUST mostrar los pedidos confirmados y no terminados en una barra
  permanente del punto de venta, cada uno identificado por su nombre de folio y su monto.
- **FR-007**: La lista de pedidos en curso MUST venir del servidor, de modo que las dos estaciones
  vean y editen los mismos pedidos.
- **FR-008**: Los usuarios MUST poder abrir un pedido en curso con **un solo toque** desde esa barra
  y agregarle productos sin pasar por la pantalla de cobro.
- **FR-009**: El sistema MUST permitir agregar renglones a un pedido mientras no esté entregado ni
  cancelado, incluso si ya se cobró; en ese caso el saldo pendiente resultante MUST verse en la
  barra.
- **FR-010**: El sistema MUST rechazar agregar renglones a un pedido entregado o cancelado, con un
  mensaje que diga en qué estado quedó el pedido.
- **FR-011**: Los agregados simultáneos desde dos estaciones MUST sumarse, nunca pisarse.
- **FR-012**: El sistema MUST tolerar que la misma cuenta se confirme dos veces por reintento
  —doble toque, red que se cae— sin crear dos pedidos.
- **FR-013**: Al confirmar, la cuenta local MUST quedar vacía y lista para el siguiente cliente; el
  pedido pasa a existir solo en la barra, para que no haya dos versiones del mismo pedido.

#### La comanda de lo agregado

- **FR-014**: Al agregar renglones a un pedido ya confirmado, la comanda que sale MUST llevar
  únicamente los renglones agregados.
- **FR-015**: Esa comanda MUST llevar el mismo nombre de folio del pedido y una marca visible de que
  es un agregado.
- **FR-016**: El sistema MUST permitir reimprimir la comanda **completa** desde el tablero de
  pedidos, como acción explícita.
- **FR-017**: El sistema MUST poder decir qué renglones de un pedido no han salido en ninguna
  comanda, para que una impresión fallida se pueda recuperar sin sacar el pedido entero.

#### Ajustes y compatibilidad

- **FR-018**: Una empresa nueva MUST nacer con la impresión de comanda encendida.
- **FR-019**: Las empresas existentes MUST conservar el valor que ya tengan.
- **FR-020**: Los pedidos creados antes de esta feature MUST seguir siendo cobrables y entregables
  sin cambios.

#### Interfaz

- **FR-021**: La barra de pedidos en curso MUST reemplazar a la píldora "Por cobrar", conservando el
  monto total pendiente a la vista, y MUST NOT consumir alto adicional de la pantalla.
- **FR-022**: Todo control de la barra MUST medir al menos 44 px de alto.
- **FR-023**: La barra MUST seguir siendo usable con más pedidos de los que caben a lo ancho, sin
  desplegables del sistema operativo.

**Lo que la US5 agrega a los requisitos:**

- **FR-018**: Al cobrar un pedido en curso se puede cobrar **una parte**, no solo el total. Lo que
  falta lo dice el servidor entre pedazo y pedazo; la pantalla no lo calcula por su cuenta.
- **FR-019**: Repartir la cuenta entre dos, tres o cuatro se ofrece como atajo de un toque. El
  residuo del redondeo se cobra en el último pedazo, nunca queda un centavo colgando.
- **FR-020**: Cada cobro puede llevar propina, y esa propina queda atribuida **al método con el que
  entró**. Nunca se monta en un pago que se hizo con otro instrumento.
- **FR-021**: Un cobro **no se registra dos veces** aunque se reintente. Si el reintento cambia el
  método o el monto, se rechaza en vez de darse por hecho: no es un reintento, es otro cobro.
- **FR-022**: Un pedido que ya se cobró **sigue pudiendo recibir renglones** mientras esté en cocina.
  Es el caso más común de "agrégame una más".
- **FR-023**: La propina no puede superar el total de la cuenta. El tope es de cordura contra un
  error de captura, y se aplica por cobro.
- **FR-024**: Cobrar avisa a las demás pantallas en el momento, igual que confirmar, agregar,
  entregar y cancelar.

### Key Entities

- **Pedido en curso**: un pedido confirmado que todavía no se entregó ni se canceló. Lo identifica
  su nombre de folio; lleva su monto, su saldo pendiente y sus renglones. Vive en el servidor y lo
  ven todas las estaciones.
- **Renglón del pedido**: producto, cantidad, adicionales y notas. Necesita poder decir **si ya
  salió en una comanda**, que es lo que permite imprimir solo lo agregado y recuperar una impresión
  que falló.
- **Comanda**: el papel de cocina. Existen dos formas: la del pedido completo (al confirmar, y por
  reimpresión) y la del agregado (solo los renglones nuevos, marcada como tal). Ninguna lleva
  precios.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Agregar un producto a un pedido en curso toma **un toque** para llegar al pedido,
  contra los cinco que cuesta hoy.
- **SC-002**: Ningún pedido puede cobrarse sin haber pasado por cocina: el intento se rechaza en el
  servidor, comprobado con una petición construida a mano.
- **SC-003**: Confirmar y cobrar un pedido de dos renglones toma como máximo **un toque más** que
  el camino de hoy.
- **SC-004**: Un pedido confirmado en una estación se ve en la otra en menos de 30 segundos sin que
  nadie recargue nada.
- **SC-005**: Con seis pedidos en curso —el máximo observado en un día— la lista de productos
  conserva la misma cantidad de renglones visibles que antes de la feature, en 1024×600.
- **SC-006**: Cocina nunca recibe dos veces el mismo renglón: verificable contando renglones entre
  las comandas de un pedido con agregados.
- **SC-007**: Cobrar una cuenta de $500 repartida entre tres comensales toma como máximo **tres
  toques por comensal** (cuánto, con qué, cobrar) y **cero capturas de teclado** cuando el reparto
  es parejo.
- **SC-008**: Ningún cobro se registra dos veces: verificable mandando el mismo cobro dos veces y
  contando las filas de pago.
- **SC-009**: El corte de caja de un turno con cuentas divididas cuadra contra el efectivo real, con
  las propinas separadas del ingreso y atribuidas a su método.

## Assumptions

- **No se editan ni se quitan renglones de un pedido en curso.** Esta feature solo agrega. Quitar un
  renglón que cocina ya está preparando necesita una comanda de cancelación y una decisión sobre el
  dinero ya cobrado; es un spec aparte. Hoy tampoco se puede, así que no se pierde nada.
- El nombre de folio ya existe y es el identificador con el que se canta el pedido; esta feature lo
  usa y no lo cambia.
- La comanda como documento —sin precios, folio grande, notas y adicionales gratis— ya está
  resuelta y no se rediseña. Lo nuevo es la variante de agregado.
- El refresco de la barra entre estaciones puede ser periódico; no hace falta tiempo real. El
  intervalo actual de la píldora (30 s) es la referencia.
- Las dos estaciones son de confianza y del mismo negocio: no hace falta bloqueo pesimista de
  pedidos, basta con que los agregados se sumen.
- El tablero de pedidos (`/pedidos`) sigue siendo la pantalla de cocina y no cambia de propósito.
- **Un cobro es un acto físico y se registra de a uno.** Capturar N pagos y mandarlos de un golpe
  registraría dinero que todavía no se recibió: si la terminal declina la tarjeta del segundo
  comensal después de que el servidor acusó, no hay forma de deshacer ese pago —no existe la
  operación— y el reembolso es de la cuenta entera. Por eso no hay un formulario de N renglones.
- **La barra del POS deja de tener un chip por pedido.** Se midió contra el presupuesto real de la
  tableta y la fila no cabía ni vacía: pedía 667.6 px sobre 612.6, y lo que se salía lo recortaba el
  contenedor. La US1 sigue cumpliéndose —agregarle a un pedido en curso cuesta dos toques en vez de
  uno— y a cambio deja de haber controles que no se pueden tocar.
