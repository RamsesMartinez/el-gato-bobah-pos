# Feature Specification: Venta por plataformas digitales con listas de precios propias

**Feature Branch**: `002-precios-plataformas`

**Created**: 2026-08-29

**Status**: Draft

**Input**: User description: "Soportar venta por plataformas digitales (Uber Eats, DiDi, Rappi) sin conexión a sus APIs: los pedidos se aceptan por fuera y se capturan a mano en el POS para imprimir el ticket que se pega al pedido. Cada plataforma tiene su propia lista de precios, separada del precio base de mostrador. La pantalla de venta arranca siempre en mostrador, como hoy, y un selector permite cambiar a una plataforma; al hacerlo todos los precios mostrados pasan a los de esa lista. El cajero puede sobrescribir el precio de un producto en el momento, y ese precio queda guardado para esa plataforma. El precio base nunca se toca. El almacén no cambia. Al cobrar, el método de pago es el de la plataforma, y la venta se distingue en el corte de caja."

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Capturar un pedido de plataforma con sus precios, sin capturar precios (Priority: P1)

Llega un pedido por Uber Eats a la tablet del repartidor. El operador cambia la pantalla de venta a
"Uber Eats", toca los productos del pedido igual que siempre y **cada uno entra ya con su precio de
Uber**, sin que nadie capture nada. Cobra con el método "Uber Eats" e imprime el ticket que se pega
a la bolsa.

**Why this priority**: Es el caso completo y el que justifica la feature. Hoy ese pedido o no se
captura —y no existe en el corte ni en los reportes ni descuenta inventario— o se captura con
precio de mostrador, lo que deja el corte descuadrado contra lo que la plataforma va a depositar.
Entregado solo, ya resuelve el problema.

**Independent Test**: Cambiar a una plataforma, agregar dos productos y un modificador, verificar
que los precios en pantalla sean los de esa lista y no los de mostrador, cobrar con el método de la
plataforma y confirmar que el ticket impreso trae esos precios.

**Acceptance Scenarios**:

1. **Given** la pantalla de venta en mostrador, **When** el operador cambia a "Uber Eats",
   **Then** todos los precios visibles (productos y modificadores) pasan a los de esa lista y la
   pantalla indica con cuál se está cobrando.
2. **Given** un producto sin precio propio para la plataforma, **When** el operador lo agrega,
   **Then** entra con el precio calculado a partir del base y el margen de la plataforma,
   redondeado a 2 decimales, sin pedir captura ni bloquear nada.
3. **Given** un ticket armado en una plataforma, **When** el operador cobra, **Then** solo puede
   elegir entre los dos métodos de esa plataforma —en línea o en efectivo— y la venta queda
   registrada como pedido de esa plataforma.
4. **Given** un ticket armado en una plataforma, **When** se imprime, **Then** el ticket muestra los
   precios de la plataforma, no los de mostrador.

---

### User Story 2 - Corregir un precio en el momento y que quede guardado (Priority: P1)

Al capturar el pedido, el operador ve que un producto trae $148.50 (el base más el margen) cuando en
la app de la plataforma está publicado en $149. Lo corrige ahí mismo, en la pantalla de venta, sin
salir a otra sección. La próxima vez que ese producto se agregue en esa plataforma, ya entra en
$149.

**Why this priority**: Sin esto, el margen automático solo acierta por casualidad y el operador
tendría que corregir el mismo producto en cada pedido. La persistencia es lo que convierte la
corrección en un trabajo que se hace una vez.

**Independent Test**: Sobrescribir el precio de un producto en una plataforma, cerrar la venta,
empezar otra en la misma plataforma y verificar que el producto entra con el precio corregido.

**Acceptance Scenarios**:

1. **Given** un producto con precio calculado, **When** el operador captura un precio distinto,
   **Then** la venta en curso usa el precio capturado.
2. **Given** un precio ya capturado para una plataforma, **When** el mismo producto se agrega en una
   venta posterior de esa plataforma, **Then** entra con el precio capturado, no con el calculado.
3. **Given** un precio capturado para una plataforma, **When** se vende el mismo producto en
   mostrador, **Then** el precio de mostrador es el base, sin rastro del cambio.
4. **Given** un precio capturado para una plataforma, **When** se vende el mismo producto en otra
   plataforma, **Then** esa otra plataforma no hereda el precio capturado.

---

### User Story 3 - Que el corte distinga lo cobrado por cada plataforma (Priority: P2)

Al cerrar el turno, el encargado ve en el corte cuánto se vendió por cada plataforma, separado del
efectivo y las tarjetas, para conciliarlo después contra lo que cada plataforma deposita.

**Why this priority**: Es la razón de negocio de capturar estos pedidos, pero llega después de poder
capturarlos: sin US1 no hay nada que separar en el corte.

**Independent Test**: Cobrar una venta por cada plataforma en un turno, cerrar la caja y verificar
que cada plataforma aparece con su total y que no se suman al efectivo esperado en el cajón.

**Acceptance Scenarios**:

1. **Given** ventas cobradas por distintas plataformas en un turno, **When** se cierra la caja,
   **Then** el corte muestra el total de cada plataforma en su propia línea.
2. **Given** una venta cobrada **en línea** por una plataforma, **When** se cierra la caja,
   **Then** ese importe no cuenta como efectivo a contar en el cajón y se autodeclara.
3. **Given** una venta de plataforma cobrada **en efectivo** al repartidor, **When** se cierra la
   caja, **Then** ese importe sí cuenta como efectivo a contar, y exige conteo físico como
   cualquier otro efectivo.

---

### User Story 4 - Corregir el precio de un extra por plataforma (Priority: P3)

Un extra que en mostrador cuesta $20 debe cobrarse en $30 en Rappi, y no en los $27 que da el
margen. El operador lo corrige igual que un producto y queda guardado para esa plataforma.

**Why this priority**: Los extras son una parte menor del ticket y el margen automático ya los deja
razonables. Corregirlos a mano es refinamiento sobre una feature que ya funciona.

**Independent Test**: Sobrescribir el precio de una opción de modificador en una plataforma y
verificar que persiste ahí y que no cambia ni en mostrador ni en las demás plataformas.

**Acceptance Scenarios**:

1. **Given** una opción de modificador sin precio propio, **When** se usa en una plataforma,
   **Then** su cargo sale del delta base más el margen de esa plataforma, redondeado a 2 decimales.
2. **Given** un precio capturado para una opción en una plataforma, **When** se usa en esa
   plataforma, **Then** se cobra el capturado; en mostrador y en las demás plataformas, no.

---

### Edge Cases

- **El operador cambia de plataforma con el ticket ya armado.** Los precios del ticket tienen que
  quedar consistentes con la lista activa: no puede quedar mitad Uber y mitad mostrador.
- **El operador captura un precio absurdo** (negativo, cero, o desmedido). La captura se rechaza en
  el servidor con un mensaje claro, no con un error genérico.
- **El precio base cambia después de haber capturado un precio de plataforma.** El precio capturado
  manda; el calculado se recalcula solo para los productos que no tienen captura.
- **Se vende en mostrador después de una venta de plataforma en la misma tablet.** La pantalla
  vuelve a mostrador sola, sin arrastrar la lista anterior.
- **Un mismo producto se agrega dos veces en el mismo ticket de plataforma.** Ambas líneas llevan el
  mismo precio de la lista.
- **Se cobra una venta de plataforma con la caja cerrada.** Se rechaza igual que cualquier venta:
  esta feature no abre una puerta al arqueo.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: El sistema DEBE mantener, por cada plataforma de reparto, una lista de precios propia
  y separada del precio base de mostrador.
- **FR-002**: El sistema DEBE calcular el precio de plataforma de un producto sin precio propio
  aplicando el margen porcentual de esa plataforma sobre el precio base, redondeado a 2 decimales.
- **FR-003**: El sistema DEBE traer un margen por default de 35% para cada plataforma, ya cargado,
  sin que nadie tenga que configurarlo antes de la primera venta.
- **FR-004**: El sistema NUNCA DEBE impedir agregar un producto ni cobrar una venta por falta de un
  precio de plataforma.
- **FR-005**: El sistema DEBE permitir al operador sobrescribir el precio de un producto para la
  plataforma activa, desde la propia pantalla de venta.
- **FR-006**: Un precio sobrescrito DEBE persistir: la siguiente venta en esa plataforma lo usa en
  lugar del calculado.
- **FR-007**: Sobrescribir un precio de plataforma NUNCA DEBE modificar el precio base ni el precio
  de otra plataforma.
- **FR-008**: El sistema DEBE aplicar la misma regla de margen y sobrescritura a las opciones de
  modificador.
- **FR-009**: La pantalla de venta DEBE arrancar siempre en mostrador, con el comportamiento actual.
- **FR-010**: La pantalla de venta DEBE ofrecer un selector para cambiar de lista de precios, y
  DEBE indicar en todo momento con cuál se está cobrando.
- **FR-011**: Al cambiar de lista, todos los precios mostrados DEBEN pasar a los de la lista nueva,
  incluidas las líneas ya agregadas al ticket.
- **FR-012**: El servidor DEBE recalcular de forma autoritativa el precio de cada línea según la
  lista de la venta, ignorando cualquier precio que mande el cliente.
- **FR-013**: Una venta capturada en una plataforma DEBE registrarse asociada a esa plataforma.
- **FR-014**: Una venta capturada en una plataforma DEBE cobrarse con uno de los dos métodos de esa
  plataforma: **en línea** (la plataforma deposita) o **en efectivo** (el repartidor entrega el
  dinero en el mostrador). Cualquier otro método se rechaza.
- **FR-015**: El corte de caja DEBE mostrar cada método de plataforma en su propia línea.
- **FR-015a**: Lo cobrado **en efectivo** por una plataforma DEBE contar como dinero a contar en el
  cajón; lo cobrado **en línea**, no. Sin esta distinción, el efectivo que entrega el repartidor
  aparece como un sobrante inexplicable al cerrar el turno.
- **FR-015b**: Los métodos de plataforma **en efectivo** DEBEN exigir conteo físico al cerrar (no se
  autodeclaran), igual que el efectivo de mostrador. Los de **en línea** sí se autodeclaran: no hay
  nada que contar, el monto lo reporta la plataforma.
- **FR-016**: El ticket impreso de una venta de plataforma DEBE mostrar los precios de esa lista.
- **FR-017**: El descuento de inventario de una venta de plataforma DEBE ser idéntico al de una
  venta de mostrador: mismos productos, mismas recetas, mismas cantidades.
- **FR-018**: El sistema DEBE rechazar un precio de plataforma inválido (negativo, cero o fuera de
  los topes de dinero del sistema) con un mensaje que diga qué pasó.
- **FR-019**: El sistema DEBE permitir quitar un precio capturado, devolviendo ese producto u opción
  al precio calculado. Sin esto, un precio equivocado pero plausible ($14.90 donde iban $149.00)
  pasa todas las validaciones y se cobra así indefinidamente, porque la pantalla de configuración
  está fuera de alcance y un precio en cero no se acepta.
- **FR-020**: Escribir o quitar un precio de plataforma DEBE reflejarse de inmediato en las demás
  tablas del negocio. Una pantalla que siga mostrando el precio anterior imprime un total distinto
  del que se cobra.
- **FR-021**: El sistema DEBE registrar quién capturó cada precio de plataforma. Es lo que permite
  dejar que un cajero los edite sin perder el rastro de una captura equivocada.
- **FR-022**: El sistema DEBE rechazar una venta cuya plataforma no pertenezca a la empresa de la
  sesión, en lugar de cobrarla a precio de mostrador.

### Key Entities *(include if feature involves data)*

- **Plataforma de reparto**: la app por la que llegó el pedido (Uber Eats, DiDi, Rappi, Propio). Ya
  existe. Gana un **margen porcentual** que define el precio de su lista.
- **Precio de plataforma de un producto**: la excepción capturada a mano para un producto en una
  plataforma. Solo existe para los productos corregidos; su ausencia significa "usa el calculado".
- **Precio de plataforma de una opción de modificador**: lo mismo, para los extras.
- **Venta**: ya guarda a qué plataforma pertenece. Sus líneas guardan el precio con el que se cobró,
  como hoy, para que el ticket y los reportes sigan siendo fieles aunque la lista cambie después.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Capturar un pedido de plataforma de 3 productos toma los mismos taps que capturarlo en
  mostrador, más uno: el de elegir la plataforma.
- **SC-002**: El día que se enciende la feature, las 3 plataformas ya tienen precio para los 502
  productos sin que nadie capture ninguno.
- **SC-003**: Corregir el precio de un producto en una plataforma se hace sin salir de la pantalla
  de venta y sin perder el ticket en curso.
- **SC-004**: Un precio corregido no se vuelve a corregir: la siguiente venta ya lo trae.
- **SC-005**: El total de mostrador de un turno no cambia por haber vendido por plataformas.
- **SC-006**: Al cerrar el turno se puede leer, sin hacer cuentas, cuánto se cobró por cada
  plataforma.
- **SC-007**: Vender el mismo producto en mostrador y en las 3 plataformas descuenta exactamente el
  mismo inventario en los 4 casos.

## Assumptions

- **Las 3 plataformas ya existen**; sus métodos de pago **se desdoblan en dos cada uno** (en línea y
  en efectivo), porque los repartidores de las tres a veces pagan en efectivo en el mostrador. Los
  tres métodos actuales no tienen ni un pago real registrado, así que se renombran a "en línea" sin
  romper histórico.
- **"Propio" queda fuera del selector de plataformas.** Es reparto del propio negocio: no hay
  comisión que absorber ni depósito que conciliar, y se sigue vendiendo como hoy (domicilio, precio
  base, cobrado en efectivo o tarjeta). Meterla obligaría a inventarle un método de pago que
  escondería en qué se cobró de verdad.
- **Cada ticket nuevo arranca en mostrador**, incluso después de cobrar uno de plataforma. Es un tap
  extra por pedido de plataforma, a cambio de que sea imposible cobrar precio de Uber en mostrador
  por inercia.
- **El margen por default es 35% para las tres.** Se carga por migración. La pantalla para
  configurarlo queda **fuera de alcance**: el dueño la pidió para después.
- **Un pedido de plataforma es a domicilio.** El reparto lo hace la plataforma, así que el costo de
  envío del negocio no aplica a estas ventas.
- **El método de pago en efectivo de una plataforma es dinero real del cajón.** No es un truco de
  reporte: el repartidor entrega billetes que se cuentan al cerrar.
- **El precio manual es un precio final**, no un ajuste sobre el margen: lo que se captura es lo que
  se cobra.
- **No hay conciliación automática** contra lo que la plataforma deposita. El corte solo separa lo
  cobrado por cada una; comparar contra el depósito es trabajo manual, fuera de alcance.
- **No hay comisión de plataforma modelada.** El margen del 35% es cómo el negocio absorbe la
  comisión en el precio publicado, no un cálculo de la comisión real.
- **El selector de plataforma es una preferencia de la venta en curso**, no del dispositivo: cada
  ticket nuevo arranca en mostrador.

## Out of Scope

- Integración con las APIs de Uber Eats, DiDi o Rappi (aceptar pedidos automáticamente).
- Pantalla de configuración del margen por plataforma.
- Alta de plataformas nuevas desde la interfaz.
- Conciliación de depósitos de las plataformas contra las ventas registradas.
- Reportes de rentabilidad por plataforma (margen real después de comisión).
- Precios por plataforma para combos con estructura propia distinta de la de mostrador.
