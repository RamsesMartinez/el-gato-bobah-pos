# Feature Specification: El descuento que se le hace a un pedido

**Feature Branch**: `022-descuento-en-el-pedido`

**Created**: 2026-09-19

**Status**: Draft

**Input**: User description: "Cuando llegan pedidos de plataformas digitales, en unas ofrezco promos y debo agregar también un descuento sobre el total."

## Contexto *(no se re-deriva)*

| Hecho | Dónde está |
|---|---|
| `orders.discount_total` **ya existe** y siempre vale cero desde [0007](../../server/migrations/0007_orders.sql) | La columna está, la feature no — principio VIII, tabla de puertas |
| Todo reporte de dinero agregado suma `orders.total`, nunca `orders.subtotal` | [reports.sql](../../server/queries/reports.sql), [cash.sql](../../server/queries/cash.sql) §corte, [settlements.sql](../../server/queries/settlements.sql), [sales.sql](../../server/queries/sales.sql) |
| El precio de plataforma del POS es una **copia que va detrás** de lo que la plataforma ya cobró | `AGENTS.md` §2 |
| El costo de envío ya está **dentro** de `orders.total` | Principio III |

**La consecuencia de la segunda fila fija el alcance**: si el descuento se resta dentro de `total`,
el corte de caja, las ventas, los reportes y la liquidación de plataforma **siguen diciendo la
verdad sin tocarlos**. Lo que sí hay que tocar es cada pantalla que muestra el **desglose** —
ticket, precuenta, detalle de venta —, porque ahí un total que no cuadra con la suma de los
renglones es un total que el cliente no puede verificar.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Capturar el descuento con el pedido (Priority: P1)

Quien captura un pedido con promoción escribe el descuento en la misma pantalla en la que lo está
armando, como monto en pesos o como porcentaje, y el total baja ahí mismo, antes de mandarlo.

**Why this priority**: es la feature. Sin esto, un pedido de plataforma con promoción se captura con
un total que **nunca fue el que se cobró**, y ese pedido ya entró a la venta del día con una cifra
inventada. Lo demás de este spec hace que el descuento se vea; esto lo hace existir.

**Independent Test**: se captura un pedido de $385 con $50 de descuento, se manda y se cobra con el
método de su plataforma. El pedido queda con total $335 y la venta del día sube $335, no $385.
Entrega valor sola: aunque nada más se construya, el POS ya cuadra con lo que la plataforma cobró.

**Acceptance Scenarios**:

1. **Given** una cuenta en captura con productos, **When** el operador escribe un descuento de
   $50.00 y manda el pedido, **Then** el pedido queda con subtotal $385.00, descuento $50.00 y total
   $335.00, y el servidor **recalcula** esas tres cifras sin creerle el total al cliente.
2. **Given** la misma cuenta, **When** el operador elige porcentaje y escribe 20, **Then** el
   descuento guardado es un **monto en pesos** ($77.00) y no un porcentaje: lo que el negocio dejó de
   cobrar es una cantidad de dinero, no una fórmula que se recalcule después.
3. **Given** una cuenta con descuento capturado, **When** el operador borra el campo para
   reescribirlo, **Then** el pedido queda **sin descuento** — el campo vacío es ausencia, no cero por
   ciento ni un descuento de $0.00 registrado.
4. **Given** una cuenta de $385.00, **When** el operador escribe un descuento de $400.00, **Then** el
   sistema lo rechaza diciendo cuánto es el máximo, y **no** manda el pedido con total negativo.
5. **Given** un pedido a domicilio propio con $35.00 de envío y $50.00 de descuento sobre $385.00 de
   productos, **When** se manda, **Then** el total es $370.00: el descuento se aplica **sobre los
   productos** y el envío se suma después, sin descontarse.
6. **Given** un pedido de cualquier tipo — mostrador, para llevar, domicilio propio o plataforma —,
   **When** se captura, **Then** el campo de descuento está disponible: la promoción de plataforma es
   el caso que apura, no el único.

---

### User Story 2 - El descuento se ve donde se lee el dinero (Priority: P1)

El renglón del descuento aparece en el ticket del cliente, en la precuenta, en la hoja de cobro y en
el detalle de la venta — con su propio nombre y su propio monto.

**Why this priority**: es P1 y no P2 porque un total rebajado **sin decir por qué** es peor que no
tener descuentos. El cliente suma los renglones del ticket, le da $385 y el papel dice $335: lo
normal es que reclame, y quien atiende no tiene con qué explicarlo. Adentro pasa lo mismo — quien
revisa el corte ve un pedido que cobró menos de lo que vendió y no tiene dónde leer la razón.

**Independent Test**: se imprime el ticket de un pedido con descuento y se verifica que trae los tres
renglones (subtotal, descuento, total) y que subtotal − descuento + envío = total.

**Acceptance Scenarios**:

1. **Given** un pedido con descuento, **When** se imprime su ticket, **Then** el papel muestra
   subtotal, descuento y total como renglones distintos, y las tres cifras cierran.
2. **Given** un pedido **sin** descuento, **When** se imprime su ticket, **Then** **no** aparece un
   renglón de descuento en $0.00: un renglón que siempre dice cero enseña a ignorarlo.
3. **Given** un pedido con descuento en la hoja de cobro, **When** el operador va a cobrar, **Then**
   lo que se ofrece cobrar es el total **ya rebajado**, y es el mismo número que la pantalla venía
   mostrando.
4. **Given** una venta con descuento en la pantalla de Ventas, **When** se abre su detalle, **Then**
   el descuento se lee ahí, junto al subtotal y al total.

---

### User Story 3 - Cambiar o quitar el descuento antes de cobrar (Priority: P2)

Mientras el pedido no se haya cobrado, quien lo atiende puede corregir el descuento o quitarlo, y
queda registrado quién lo dejó así y cuándo.

**Why this priority**: el descuento se teclea con el cliente enfrente y se teclea mal. Sin esta
historia, corregir un descuento obliga a cancelar el pedido y volver a capturarlo — que es
exactamente lo que la vara de UX del producto prohíbe (nunca deshacer para rehacer). Va después de
las dos P1 porque un descuento mal capturado, hoy, al menos se puede resolver cancelando.

**Independent Test**: se crea un pedido con $50 de descuento, se le cambia a $30, se cobra, y el
detalle de la venta muestra $30 y el nombre de quien lo aplicó.

**Acceptance Scenarios**:

1. **Given** un pedido abierto con descuento, **When** el operador lo cambia a otro monto, **Then**
   el total se recalcula y queda registrado quién lo aplicó y cuándo.
2. **Given** un pedido **ya cobrado por completo**, **When** se intenta cambiar su descuento,
   **Then** el sistema lo rechaza: cambiarlo movería el total contra pagos que ya se registraron.
3. **Given** un pedido con descuento al que se le **cancela una línea** y el subtotal queda por
   debajo del descuento, **Then** el sistema no deja el total negativo y el pedido queda en cero como
   piso.
4. **Given** un pedido con descuento al que se le **agregan líneas** después, **Then** el descuento
   sigue siendo el mismo monto en pesos que se capturó: no se recalcula solo, aunque se haya
   capturado como porcentaje.

---

### Edge Cases

Enumerados antes de escribir una línea, como exige el principio IV. Cada uno tiene su renglón en
Requirements:

- **El campo vacío.** Borrar el campo para reescribirlo es ausencia de descuento, no `0`. Es el
  defecto que ya costó una vez en este repo (el bloqueo por PIN que se apagaba a media captura).
- **El descuento mayor que el subtotal.** Se rechaza en la frontera, no se recorta en silencio: un
  descuento que el sistema "ajusta" solo deja al operador creyendo que cobró otra cosa.
- **El porcentaje que no divide exacto.** 33% de $100.05 no da centavos redondos; el monto guardado
  se redondea a 2 decimales y **manda el monto**, no el porcentaje.
- **El porcentaje absurdo.** 120%, −5, `NaN`, texto: se rechazan como entrada inválida.
- **El subtotal que cambia después.** Agregar o cancelar líneas mueve el subtotal y deja el descuento
  donde estaba; si el subtotal queda por debajo, el total es cero y nunca negativo.
- **El envío.** El descuento nunca come el costo de envío: son dos pesos distintos y el principio III
  exige que cada uno se clasifique una vez.
- **El pedido ya cobrado.** No admite cambio de descuento.
- **El pedido cancelado o reembolsado.** El descuento no se devuelve aparte ni se cuenta como pérdida
  adicional: el ingreso que se revierte es el total, que ya venía rebajado.
- **El doble tap.** Aplicar el mismo descuento dos veces no lo suma dos veces.

**Tres bordes que NO se vieron al escribir este spec y aparecieron después.** Se dejan aquí, que es
donde se pensaron demasiado tarde, porque un borde arreglado en silencio vuelve:

- **El pedido descontado al 100 % no quedaba saldado.** «Saldado» exigía un total positivo, y esa
  regla era defendible mientras llegar a cero pidiera que cada producto costara $0. Con el descuento
  se llega en un toque, y el pedido se quedaba con «Falta cobrar $0.00» en naranja para siempre: no
  había forma de cerrarlo, porque no había nada que cobrar. Lo encontró la revisión de código; el
  spec no lo había pensado.
- **El descuento que deja el total por debajo de lo YA ABONADO.** El pedido quedaría sobrepagado sin
  forma de devolver la diferencia. El pago parcial no es raro: el cliente deja algo al pedir y
  termina al recoger. Se rechaza nombrando cuánto hay abonado.
- **Quitar un descuento borra su propio rastro.** `discount_set_by` se sobrescribe en sitio y se
  limpia al quitar el descuento, así que la fila queda idéntica a la de un pedido que nunca tuvo
  uno. El rastro es todo el control de esta feature (FR-012), así que cada cambio deja además un
  evento de seguridad con el monto anterior — y el nombre de quien descontó se lee en el detalle de
  la venta, no solo en la base.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: El sistema MUST permitir capturar un descuento sobre un pedido, expresado por el
  operador como **monto en pesos** o como **porcentaje**, a su elección.
- **FR-002**: El sistema MUST guardar el descuento **siempre como monto en pesos**. El porcentaje es
  una forma de teclear, no un dato que se conserve para recalcular.
- **FR-003**: El sistema MUST recalcular subtotal, descuento y total **en el servidor**. Un descuento
  o un total que mande el cliente se ignoran, igual que los precios.
- **FR-004**: El sistema MUST aplicar el descuento **sobre el subtotal de productos**, y sumar el
  costo de envío después, de modo que `total = subtotal − descuento + envío`.
- **FR-005**: El sistema MUST rechazar, como entrada inválida y no como un valor ajustado en
  silencio: un descuento mayor que el subtotal, negativo, no numérico, o un porcentaje fuera de
  0–100.
- **FR-006**: El sistema MUST tratar el campo vacío como **ausencia de descuento**, distinta de un
  descuento de cero.
- **FR-007**: El sistema MUST registrar **quién** aplicó el descuento y **cuándo**, y conservarlo con
  el pedido.
- **FR-008**: Los usuarios MUST poder cambiar o quitar el descuento de un pedido **mientras no esté
  cobrado por completo**; una vez cobrado, el sistema MUST rechazar el cambio.
- **FR-009**: El sistema MUST mantener el total en cero como piso cuando el subtotal baje por debajo
  del descuento ya capturado, sin dejarlo negativo.
- **FR-010**: El sistema MUST mostrar el descuento como renglón propio en el ticket impreso, la
  precuenta, la hoja de cobro y el detalle de la venta, **solo cuando exista**.
- **FR-011**: El descuento MUST estar disponible en cualquier tipo de pedido: mostrador, para llevar,
  domicilio propio y plataforma.
- **FR-012**: Cualquier operador con acceso a capturar pedidos MUST poder aplicar un descuento; el
  control es el rastro de FR-007, no un permiso aparte.

### Key Entities

- **Descuento del pedido**: el monto en pesos que el negocio dejó de cobrar en ese pedido, con quién
  lo aplicó y cuándo. Vive **con el pedido**, no como una entidad suelta: no hay catálogo de
  promociones ni campañas en este spec.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Un pedido con promoción se captura con su descuento sin salir de la pantalla de
  captura. Desde la vista por omisión de la tableta (1024×600, donde el panel del ticket arranca
  colapsado) cuesta **un toque para abrir el panel y otro para abrir el campo**, más escribir la
  cifra y elegir `$` o `%`. Ese toque de apertura está contado a propósito: el descuento no cabe en
  la píldora ni en la barra, que no tienen ancho libre.
- **SC-002**: Para todo pedido, la cuenta cierra: subtotal − descuento + envío = total, y ninguna
  pantalla muestra dos cifras distintas para lo mismo.
- **SC-003**: La venta del día, el corte de caja y el reporte de ventas reflejan el total ya
  rebajado, **sin que ninguna de esas cifras cambie de significado** para los pedidos sin descuento
  (que son todos los que ya existen).
- **SC-004**: Un cliente que suma los renglones de su ticket llega al total impreso.
- **SC-005**: Ningún pedido puede quedar con total negativo ni con un descuento mayor a lo que se
  vendió.

## Assumptions

- **Quién financió el descuento NO se registra.** El dueño lo decidió explícitamente el 2026-09-19,
  con el costo enfrente: la tabla de puertas del principio VIII nombra *Promociones de plataforma*, y
  `platform_settlements.discount_platform` ya distingue quién lo puso al capturar el documento de
  pago. La consecuencia aceptada: para un pedido individual, **saber si la promoción la pagó el
  negocio o la plataforma dependerá de tener a la mano el documento de pago**, que Uber expone 31 días
  y Rappi 3 meses. No se cierra la puerta —agregar el dato después es una columna más—, pero el hecho
  de los pedidos capturados mientras tanto no se recupera. Queda como exención del dueño, no como
  descuido del spec.
- **No hay catálogo de promociones.** Ni 2x1, ni producto de regalo, ni campañas con vigencia. El
  descuento es una cifra que el operador escribe, y esta feature no intenta reconstruir qué se
  regaló. Esa sigue siendo la puerta abierta que nombra el principio VIII.
- **El descuento es del pedido completo**, no por renglón. Descontar un producto en particular no
  está en este spec.
- **No hay tope configurable ni autorización por rol o PIN.** Cualquier operador puede aplicarlo y el
  rastro es lo que permite auditarlo después. Si el negocio detecta abuso, poner el tope es una
  feature nueva que este diseño no estorba.
- **Los pedidos que ya existen quedan con descuento cero**, que es lo que la columna ya dice hoy: no
  hay que rellenar historia.
