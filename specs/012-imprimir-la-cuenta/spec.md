# Feature Specification: Imprimir la cuenta antes de que sea una venta

**Feature Branch**: `012-imprimir-la-cuenta`

**Created**: 2026-09-05

**Status**: Draft

**Input**: Ver *Origen* al final.

## Contexto

La [001](../001-ticket-preview-print/spec.md) resolvió imprimir el ticket de un pedido que **ya
existe**: al cerrarlo, reimprimirlo desde el tablero, y la impresión automática al cobrar. Nada
cubre el momento anterior — cuando el cliente pide la cuenta para revisarla antes de pagar.

Y desde la [011](#) ese momento se volvió más largo: tocar COBRAR ya no crea el pedido, así que la
hoja de cobro se abre sobre una cuenta que todavía no existe en el servidor. Ahí no hay número de
pedido, ni folio confirmado, ni nada que reimprimir.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - La cuenta que el cliente revisa antes de pagar (Priority: P1)

Quien atiende abre el cobro, el cliente pide ver la cuenta, y sale un papel con lo que se capturó y
cuánto suma. El cliente lo revisa, decide cómo paga, y la venta sigue su curso.

**Why this priority**: Es el momento en que el cliente decide, y hoy no hay por dónde. Las
alternativas son peores: cobrar primero para que salga el ticket —y entonces ya no hay nada que
revisar— o leerle la pantalla en voz alta.

**Independent Test**: Con una cuenta capturada y sin confirmar, abrir el cobro, tocar el icono de
imprimir, y comprobar que sale un papel con los productos y el total, sin que se cree ningún pedido.

**Acceptance Scenarios**:

1. **Given** una cuenta capturada, **When** se abre el cobro y se toca imprimir, **Then** sale un
   papel con los productos, sus cantidades y el total.
2. **Given** ese papel impreso, **When** se mira el servidor, **Then** no se creó ningún pedido y
   cocina no recibió nada.
3. **Given** el cobro abierto sobre un pedido que SÍ existe —el camino del botón naranja—,
   **When** se toca imprimir, **Then** sale el ticket real de ese pedido, con su folio.
4. **Given** el papel impreso y luego la venta cobrada, **When** sale el ticket de la venta,
   **Then** los dos papeles se distinguen entre sí sin tener que leerlos con cuidado.

---

### User Story 2 - Ese papel no puede pasar por un comprobante de venta (Priority: P1)

El papel sale de la misma impresora que los tickets. Quien lo tenga en la mano —cliente, cajero,
quien cuadra la caja— tiene que poder decir de un vistazo que **no** es el comprobante de una venta.

**Why this priority**: Es la mitad del trabajo, no un adorno. Dos papeles indistinguibles del mismo
pedido pueden circular como dos ventas; es exactamente lo que la marca de reimpresión existe para
impedir, y este papel es peor porque ni siquiera corresponde a una venta que ocurrió.

**Independent Test**: Imprimir la cuenta de un pedido sin confirmar y comprobar que el papel trae su
marca y no trae número de pedido.

**Acceptance Scenarios**:

1. **Given** una cuenta sin confirmar, **When** se imprime, **Then** el papel lleva una marca
   visible que dice que es una pre-cuenta, en el mismo lugar y con el mismo peso que las marcas que
   ya existen.
2. **Given** ese papel, **When** se busca en él un número de pedido, **Then** no hay ninguno: el
   pedido todavía no existe y el papel no inventa uno.
3. **Given** ese papel, **When** se compara con el ticket de una venta cobrada, **Then** el de la
   venta trae número, folio y el estado del cobro, y el otro no.

---

### User Story 3 - El control se toca con el dedo, a la primera (Priority: P2)

El icono de imprimir vive en la hoja de cobro, visible sin buscarlo y con tamaño de dedo.

**Why this priority**: La hoja de cobro es la pantalla con menos margen del sistema y la que se opera
con el cliente enfrente. Un control escondido en un menú no se usa; uno chico se toca dos veces y la
segunda cae en el botón de cobrar.

**Independent Test**: A 1024×600, verificar que el control mide al menos 44 px, que se ve sin abrir
ningún menú, y que la hoja sigue cabiendo en la pantalla.

**Acceptance Scenarios**:

1. **Given** la hoja de cobro abierta a 1024×600, **When** se mira, **Then** el control de imprimir
   está visible sin abrir nada y mide al menos 44 px por lado.
2. **Given** la hoja con el control, **When** se mide su alto, **Then** sigue cabiendo en 600 px.
3. **Given** el control, **When** se toca, **Then** no cobra nada ni cierra la hoja.

---

### Edge Cases

- **Una cuenta vacía NO puede llegar aquí.** COBRAR está deshabilitado sin renglones, así que la
  hoja de cobro nunca se abre sobre una cuenta vacía. Se verificó antes de escribirlo, para no
  especificar un camino que no existe.
- **El costo de envío.** La cuenta puede llevarlo. El papel tiene que sumar lo mismo que la pantalla,
  o el cliente revisa una cifra y paga otra.
- **Un producto que dejó de existir mientras estaba en el carrito.** La pantalla ya lo excluye del
  envío; el papel tiene que mostrar lo mismo que se va a cobrar, no lo que el carrito tenía.
- **Imprimir dos veces la misma cuenta.** Es legítimo —el cliente perdió el papel— y no crea nada,
  así que no hay nada que impedir. Los dos papeles son idénticos y ninguno es un comprobante.
- **Imprimir, y que el cliente después agregue algo.** El papel viejo queda desactualizado. No se
  puede evitar; lo que sí se puede es que no parezca un comprobante de una venta que no ocurrió.
- **La impresora apagada o sin papel.** No puede tumbar el cobro: la cuenta sigue capturada y la
  venta se puede seguir cobrando.
- **El cobro abierto sobre un pedido que ya existe.** Ese camino ya funciona y no cambia: sale el
  ticket real, con su folio y su marca de reimpresión si ya se pagó.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: Desde la hoja de cobro, quien opera MUST poder imprimir la cuenta que está por cobrar.
- **FR-002**: Imprimir la cuenta MUST NOT crear el pedido, mandar nada a cocina, ni mover dinero.
- **FR-003**: El papel de una cuenta sin confirmar MUST llevar una marca visible que lo identifique
  como pre-cuenta, en el mismo lugar y con el mismo peso que las marcas que el ticket ya usa.
- **FR-004**: Ese papel MUST llevar el NOMBRE que la pantalla propone ("Chartreux") y MUST NOT llevar
  número de pedido: el número no existe todavía y uno inventado no coincidiría con el ticket.
- **FR-004b**: El papel MUST poder decir que el nombre es una propuesta, no una asignación, mientras
  el pedido no exista. Es un techo conocido y acotado: la pantalla propone de la misma bolsa de la
  que el servidor reparte, así que coincide casi siempre; cuando otra estación toma ese nombre
  antes, el papel impreso queda con uno distinto al del ticket. Se acepta porque el daño es bajo —
  nadie busca la venta por ese papel— y porque la [013](../013-la-orden-nace-al-primer-producto/spec.md)
  lo cierra del todo: cuando la orden nazca al primer producto, el nombre estará amarrado y el papel
  llevará el folio real.
- **FR-005**: Ese papel MUST mostrar los mismos productos, cantidades y total que la pantalla está a
  punto de cobrar, incluido el costo de envío cuando aplique.
- **FR-006**: Cuando la hoja de cobro se abre sobre un pedido que ya existe, imprimir MUST sacar el
  ticket real de ese pedido, con su folio y su marca de reimpresión cuando corresponda.
- **FR-007**: El control MUST estar visible sin abrir ningún menú y medir al menos 44 px por lado.
- **FR-008**: Agregar el control MUST NOT hacer que la hoja de cobro deje de caber en 600 px de alto.
- **FR-009**: Una falla de impresión MUST NOT impedir cobrar.

### Key Entities

- **Cuenta**: lo que la pantalla tiene capturado y todavía no es un pedido. Tiene productos,
  cantidades, un costo de envío posible y un total. **No tiene** número, folio ni fecha de negocio:
  esos los asigna el servidor al confirmar.
- **Pre-cuenta**: el papel que representa a una cuenta. Se distingue del ticket de venta por su
  marca y por la ausencia de folio.
- **Ticket de venta**: lo que ya existe. No cambia.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Se puede entregar la cuenta al cliente sin cobrarle y sin mandar nada a cocina,
  verificable preguntándole al servidor cuántos pedidos en curso hay antes y después.
- **SC-002**: Un papel de pre-cuenta y un ticket de venta se distinguen sin leerlos con cuidado:
  uno trae marca y no trae folio, el otro al revés.
- **SC-003**: Cero identificadores en el papel que no existan en el sistema.
- **SC-004**: La hoja de cobro sigue cabiendo en 600 px con el control puesto.
- **SC-005**: El total del papel y el que la pantalla cobra coinciden al centavo, envío incluido.

## Assumptions

- **La marca es `** PRE-CUENTA **`** (decidido por el dueño), con el mismo formato y en el mismo
  lugar que `** REIMPRESIÓN **` y `** TICKET DE PRUEBA **`. Es la palabra que se usa en un
  restaurante, así que el cliente no tiene que deducir nada. Se descartó reusar el `POR COBRAR` que
  ya sale: ese renglón también aparece en el ticket de un pedido REAL sin cobrar, y los dos papeles
  quedarían iguales justo donde deben distinguirse.
- **El pie NO lleva `POR COBRAR` y SÍ lleva el mensaje del negocio** (decidido por el dueño). El
  estado del cobro se quita porque es lo que confunde los dos papeles; el mensaje se queda porque es
  identidad y no estado — sin él, el papel parece un borrador y no algo que el negocio entregó.
- **El control dice "Cuenta" junto a su icono** (decidido por el dueño). Se reconoce sin adivinar, y
  es el mismo criterio del botón "Ticket" de la lista del botón naranja. El botón de al lado cobra,
  así que un control que se toca por descarte es caro.
- **El control va en la hoja de cobro**, no en el panel lateral. El dueño lo pidió ahí, y además es
  donde el operador está cuando el cliente pide la cuenta.
- Se reutiliza el armado del ticket que ya existe; lo nuevo es de dónde salen los datos cuando no
  hay pedido y la marca que lleva el papel.

## Out of Scope

- Imprimir desde el panel lateral del POS. Se evaluó y se descartó: el operador pide la cuenta desde
  el cobro, y dos entradas a lo mismo son dos lugares donde arreglar el mismo defecto.
- Cambiar cuándo nace el pedido. Eso ya lo resolvió la 011 y esta feature depende de ello.
- Un formato de pre-cuenta distinto del ticket. Es el mismo papel con otra marca y sin folio.

## Origen

Pedido por el dueño el 2026-09-05. Preguntó primero si imprimir desde el panel lateral estaba
especificado —no lo estaba— y al plantearle las opciones redirigió: el icono va en la hoja de cobro,
"visible y fácil de presionar", porque la interfaz es de toque.

Las tres decisiones que dejó abiertas (qué marca lleva el papel, qué identificador, y dónde va el
control) se resolvieron aquí con su porqué, y se cambian corrigiendo este documento.
