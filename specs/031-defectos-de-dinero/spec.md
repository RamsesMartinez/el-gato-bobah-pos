# Feature Specification: Defectos de dinero de la auditoría

**Feature Branch**: `031-defectos-de-dinero`

**Created**: 2026-10-08

**Status**: Draft

**Input**: Corregir los defectos de dinero que encontró una auditoría de cuatro revisores (34 hallazgos,
18 distintos): devoluciones, devolver un pago, cortes de caja y reportes.

## Contexto

Una auditoría recorrió los caminos por donde entra y sale dinero del POS. Varios hallazgos describen
el mismo defecto visto desde áreas distintas; deduplicados quedan 18. En este spec se nombran D1–D18
y cada requisito cita el suyo. Lo que la auditoría marcó como «no es defecto» (D15) también se
resuelve aquí: se deja escrito con la prueba que lo demuestra.

Dos decisiones del dueño, ya tomadas, gobiernan todo el spec:

- **Una devolución cuenta en el periodo en que se devolvió**, no en el de la venta.
- **El dinero se clasifica por la hora y el turno de CADA pago, y cada devolución por los suyos**, no
  por el turno del pedido.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Una devolución nunca saca más dinero del que entró, ni por otro medio (Priority: P1)

El gerente devuelve una parte de un pedido y, más tarde, otra parte o el resto (o cancela el pedido
con devolución). Cada peso sale por el medio por el que entró, y lo ya devuelto por un medio no
vuelve a salir por él. Dos devoluciones del mismo pedido que llegan a la vez (dos tabletas, un doble
toque) no pasan las dos el tope. Una devolución contra un platillo se topa con lo que vale ese
platillo, además de con lo que queda del pedido.

**Why this priority**: Es dinero que sale físicamente del cajón. Hoy una segunda devolución parcial
vuelve a sacar en efectivo lo que ya se devolvió en efectivo (D1), dos devoluciones simultáneas
devuelven el doble (D3), y una devolución contra un platillo de $60 puede devolver el pedido entero
y repetirse por cada platillo (D4).

**Independent Test**: Pedido de $100 cobrado $40 en efectivo y $60 con tarjeta; devolver $40 y
luego $60. El segundo reparto sale todo por tarjeta y no hay una segunda salida de caja.

**Acceptance Scenarios**:

1. **Given** un pedido cobrado $40 en efectivo y luego $60 con tarjeta, con $40 ya devueltos en
   efectivo, **When** se devuelven los $60 restantes (o se cancela con devolución), **Then** salen $60
   por tarjeta, $0 en efectivo y no hay otra salida de caja (D1).
2. **Given** un pedido de $400 cobrado, **When** llegan a la vez dos «Devolver todo», **Then** uno se
   registra y el otro rebota con «no puedes devolver más de lo que se cobró» (D3).
3. **Given** una devolución y una cancelación con devolución del mismo pedido a la vez, **When**
   terminan las dos, **Then** lo devuelto no supera lo cobrado (D3).
4. **Given** un pedido de $500 cobrado con renglones A ($60) y B ($40), **When** se devuelve contra A
   sin monto, **Then** se devuelven $60, no $500; y contra A otra vez, $0 disponibles (D4).
5. **Given** un pedido al que ya se le devolvió la cuenta entera, **When** se intenta devolver contra
   un renglón, **Then** rebota: no queda nada (D4).
6. **Given** un renglón que no es de ese pedido, **When** se intenta devolver contra él, **Then**
   rebota como «no encontrado» (D4).
7. **Given** un pedido marcado «reembolsado» por el flujo anterior a las devoluciones parciales,
   **When** se intenta devolver, **Then** rebota: ese pedido ya devolvió su dinero (D18).

---

### User Story 2 - Devolver un pago no regresa dinero que ya se devolvió (Priority: P1)

El cajero devuelve un pago desde la hoja de cobro. Si el pedido ya tuvo una devolución por ese medio
y quitar el pago dejaría lo devuelto sin un cobro detrás, la acción se rechaza con un mensaje que lo
dice.

**Why this priority**: Hoy el mismo dinero sale dos veces, el pedido vuelve a deber y el corte
marca un sobrante falso (D2).

**Independent Test**: Pedido de $100 en efectivo, devolver $40, luego devolver el pago de $100:
rechazado.

**Acceptance Scenarios**:

1. **Given** un pedido de $100 cobrado en efectivo con $40 devueltos, **When** se devuelve ese pago,
   **Then** se rechaza y el pago sigue vivo (D2).
2. **Given** un pedido con un pago en efectivo y otro con tarjeta, con una devolución solo por
   tarjeta, **When** se devuelve el pago en efectivo, **Then** se permite: la devolución por tarjeta
   sigue cubierta por el pago con tarjeta (D2).

---

### User Story 3 - El corte espera exactamente el dinero que el turno movió (Priority: P1)

Quien cierra caja ve, por cada medio, lo que entró menos lo que se devolvió en ESE turno, con las
devoluciones listadas. Nada de lo que confirma mientras se cierra queda fuera del esperado firmado.
Los pagos de plataforma aceptados sin turno entran al turno que se abre después. Un cobro hecho en
un turno distinto del que abrió el pedido se explica en los dos cortes.

**Why this priority**: Con «declarar automático», el corte hoy firma como recibido dinero que se
regresó por tarjeta (D6); hay pagos que no entran a ningún corte (D5); hay salidas de efectivo sin
rastro (D7); un cobro concurrente al cierre queda fuera del esperado guardado (D8); y un cobro
tardío descuadra dos cortes (D12).

**Independent Test**: Pedido de $300 con tarjeta, cancelado con devolución en el mismo turno: el
esperado de tarjeta es $0, la devolución aparece listada, y el declarado automático es $0.

**Acceptance Scenarios**:

1. **Given** un pago de $300 con tarjeta y su devolución en el mismo turno, **When** se cierra,
   **Then** el esperado de tarjeta es $0 y la devolución aparece en el corte (D6).
2. **Given** un pago con tarjeta en el turno A y su devolución en el turno B, **When** se cierran,
   **Then** A espera $300 y B espera −$300 en tarjeta, cada uno con lo suyo (D6, decisión del dueño).
3. **Given** que no hay turno principal abierto, **When** se intenta devolver en efectivo, **Then**
   se rechaza con «Abre la caja para devolver efectivo» y no se registra nada (D7).
4. **Given** que no hay turno abierto, **When** se devuelve algo pagado con tarjeta o por plataforma,
   **Then** se registra y entra al siguiente turno que se abra en esa sucursal (D7).
5. **Given** un pedido de plataforma aceptado sin turno, **When** se abre turno, **Then** su pago
   entra al esperado de ese turno junto con su venta (D5).
6. **Given** un cobro que confirma mientras se cierra el turno, **When** el cierre termina, **Then** el
   cobro está dentro del esperado guardado o rebota por turno cerrado; nunca queda en un turno cerrado
   fuera de su esperado. Lo mismo para un movimiento de caja y un traspaso (D8).
7. **Given** un pedido del turno A entregado sin cobrar y cobrado en el turno B, **When** se ven los
   dos cortes, **Then** A sigue mostrando $554 sin cobrar y B muestra «Cobros de otros turnos»
   $554; el esperado de cada uno cuadra con sus renglones (D12).

---

### User Story 4 - Cancelar con devolución regresa todo lo que pagó el cliente (Priority: P2)

Cancelar un pedido cobrado con devolución regresa la cuenta y la propina. La propina sale del cajón
(o de la tarjeta) junto con la cuenta y no queda en ningún reparto.

**Why this priority**: Hoy la propina se queda en el esperado del cajón y nadie la reparte (D9).

**Independent Test**: Pedido de $100 + $10 de propina en efectivo, cancelado con devolución: salida
de caja de $110, esperado del cajón vuelve al fondo.

**Acceptance Scenarios**:

1. **Given** $100 + $10 de propina en efectivo, **When** se cancela con devolución, **Then** sale
   $110 del cajón, el libro registra $100 de devolución y $10 de propina devuelta, y la venta
   devuelta del pedido es $100 (D9).
2. **Given** una devolución parcial (sin cancelar), **When** se registra, **Then** la propina no se
   toca (D9).

---

### User Story 5 - Los reportes dicen lo mismo que la venta (Priority: P2)

Utilidad por producto, Ventas y los renglones cancelados cuadran con lo vendido.

**Acceptance Scenarios**:

1. **Given** un pedido con 2 frappés de $60 a los que se quitó 1 y se aplicó $20 de descuento,
   **When** se ve Utilidad por producto, **Then** dice 1 pieza e ingreso $40 (el renglón menos su
   parte del descuento), no 2 piezas y $120 (D10).
2. **Given** un pedido al que se le quitaron 3 frappés y luego se cerró sin productos, **When** se ve
   Ventas, **Then** «Renglones cancelados» los sigue contando (D14).
3. **Given** un pedido de un turno cerrado cobrado «por monto» días después, **When** se ve Ventas por
   método, **Then** el cobro aparece en el día en que se cobró (D12, decisión del dueño).
4. **Given** un cobro de septiembre devuelto en octubre, **When** se ve Ventas por método de cada mes,
   **Then** septiembre conserva el cobro y octubre muestra la devolución (decisión del dueño).
5. **Given** el detalle de un corte con el cajón $200 corto, **When** se ve la tabla de conciliación,
   **Then** «Total Dif.» dice −$200, igual que el histórico (D13).
6. **Given** Ventas filtrada por tipo de venta, **When** se ve la pantalla, **Then** el recuadro de
   plataformas no aparece (D15, ya protegido: se deja la prueba).

---

### User Story 6 - El inventario y los centavos no se inventan (Priority: P3)

1. **Given** un frappé ya enviado a cocina, **When** se cancela el pedido completo, **Then** sus
   insumos NO vuelven al almacén, igual que si se quitara el renglón (D11).
2. **Given** un pedido de $100, **When** se cobran $99.99, **Then** el pedido sigue debiendo $0.01
   (D16).
3. **Given** un renglón de 1 pieza a $45.55, **When** se parte a la mitad, **Then** las dos partes
   suman $45.55 (D17).

### Edge Cases

- Devolución por un medio que ya se desactivó: se permite (el dinero que entró tiene que poder salir).
- Pedido con varios pagos del mismo medio: lo devuelto se resta por medio, no por pago.
- Devolución que queda en $0.00 después de redondear: rechazada como hoy.
- Turno de otra sucursal abierto: no cuenta como turno para devolver efectivo en esta.
- Cancelar con devolución cuando un medio ya está devuelto completo pero tiene propina: se devuelve
  solo la propina por ese medio.
- Un pago viejo (anterior a la vinculación turno–pago) sin turno: se trata como del turno del pedido
  al calcular lo que falta por cobrar.
- Devoluciones registradas antes de este cambio: las de efectivo quedan ligadas a su turno por su
  salida de caja; las de tarjeta quedan sin turno (los cortes ya cerrados guardaron su cifra).

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001 (D1)**: El reparto de una devolución por medio de pago MUST descontar, por cada medio, lo
  ya devuelto por ese medio.
- **FR-002 (D3)**: Devolver MUST serializarse contra cualquier otra operación de dinero del mismo
  pedido (devolver, cancelar, cobrar, devolver un pago); el tope se valida con el pedido bloqueado.
- **FR-003 (D4)**: Una devolución contra un renglón MUST toparse con el menor de: lo que queda por
  devolver del pedido y lo que vale el renglón menos lo ya devuelto contra él. El renglón MUST ser de
  ese pedido y estar vivo.
- **FR-004 (D18)**: Un pedido en estado «reembolsado» MUST rechazar nuevas devoluciones.
- **FR-005 (D2)**: Devolver un pago MUST rechazarse si, sin él, lo cobrado por su medio quedaría por
  debajo de lo devuelto por ese medio.
- **FR-006 (D6)**: Cada devolución MUST quedar ligada al turno en que se hizo, y el esperado de cada
  medio que no va al cajón MUST restar las devoluciones de ese turno; el corte MUST listarlas.
- **FR-007 (D7)**: Una devolución que saca dinero del cajón MUST rechazarse sin turno principal
  abierto en la sucursal. Una que no lo saca MUST registrarse y entrar al siguiente turno que se abra
  en la sucursal.
- **FR-008 (D5)**: Al abrir turno, los pagos de los pedidos de plataforma que ese turno reclama MUST
  quedar en ese turno.
- **FR-009 (D8)**: Cerrar turno MUST calcular y guardar el esperado con el turno bloqueado; cobrar,
  devolver, devolver un pago, registrar un movimiento y traspasar MUST esperar al cierre o rebotar
  por turno cerrado.
- **FR-010 (D12)**: Lo que falta por cobrar de un turno MUST considerar solo los pagos de ese turno
  (o sin turno), y el corte MUST mostrar aparte lo cobrado en el turno de pedidos de otros turnos.
- **FR-011 (D12, decisión del dueño)**: Ventas por método y el reporte de cobros por método MUST
  clasificar cada pago por el día en que se cobró y restar cada devolución en el día en que se
  devolvió.
- **FR-012 (D9)**: Cancelar con devolución MUST devolver también la propina, por el medio por el que
  entró, registrándola aparte del monto de la venta.
- **FR-013 (D10)**: Utilidad por producto MUST excluir renglones quitados y repartir el descuento del
  pedido entre sus renglones en proporción a su importe.
- **FR-014 (D14)**: «Renglones cancelados» MUST contar lo quitado de un pedido aunque el pedido se
  haya cancelado después.
- **FR-015 (D11)**: Cancelar un pedido completo MUST reponer el inventario con la misma regla que
  quitar un renglón: lo que se prepara y ya salió a cocina no vuelve.
- **FR-016 (D16)**: Un pedido MUST darse por saldado solo cuando lo cobrado cubre el total al centavo.
- **FR-017 (D17)**: Partir un renglón MUST conservar la suma de los importes al centavo.
- **FR-018 (D13)**: «Total Dif.» del detalle del corte MUST incluir la diferencia del cajón.
- **FR-019 (D15)**: Ventas MUST seguir ocultando el recuadro de plataformas con cualquier filtro de la
  tabla, con una prueba que lo vigile.

### Key Entities

- **Devolución**: dinero que sale hacia el cliente; pedido, renglón opcional, medio, monto de venta,
  propina devuelta, turno en que ocurrió, salida de caja si salió del cajón.
- **Pago**: dinero que entra; medio, monto, propina, turno en que entró.
- **Turno (corte)**: lo que esperaba cada medio y lo que se declaró; se firma al cerrar.

## Decisiones por defecto, revisables por el dueño

**Aprobadas tal cual por el dueño el 2026-10-09** (DD-1 a DD-10): dejan de ser revisables y son
decisiones tomadas.

| # | Decisión | Consecuencia |
|---|---|---|
| DD-1 | Un pago de plataforma aceptado sin turno entra al turno que reclama su pedido al abrirse (el de su sucursal y su día). | La venta y su dinero quedan en el mismo corte. Si el repartidor entregó el efectivo antes de abrir y se contó en el fondo, aparecería como sobrante; hoy no hay evidencia de que pase. |
| DD-2 | Cancelar con devolución regresa también la propina. | El cajón sale con lo que el cliente dio; la propina no aparece en ningún reparto (ya era así en los reportes). Una devolución parcial no toca la propina. |
| DD-3 | Sin turno principal abierto, devolver efectivo se rechaza; devolver tarjeta o plataforma se registra y entra al siguiente turno de la sucursal. | Para devolver efectivo hay que abrir caja primero. Cancelar un pedido de Uber de madrugada sigue funcionando. |
| DD-4 | Devolver un pago se rechaza solo si deja una devolución sin cobro detrás por su medio. | Un pedido con devolución por tarjeta todavía puede devolver su pago en efectivo. |
| DD-5 | El tope de una devolución contra un renglón es el importe del renglón (precio × piezas con extras), sin prorratear el descuento. | Con descuento, un renglón puede devolver hasta su precio de lista; el tope del pedido sigue mandando. |
| DD-6 | Ventas por método incluye los cobros de pedidos cancelados que tienen devolución registrada, y resta la devolución en su día. Los cancelados sin devolución (anteriores a las devoluciones parciales) siguen fuera. | Un mes cerrado ya no cambia porque un pedido se cancele después. |
| DD-7 | Utilidad por producto no incluye el envío. | Su ingreso suma lo vendido menos envíos, no el total de Ventas. |
| DD-8 | Se quita la tolerancia de un centavo para dar un pedido por saldado. | Un pedido viejo cobrado $0.01 abajo vuelve a mostrarse con $0.01 por cobrar si sigue en la ventana visible. |
| DD-9 | El día de un pago o una devolución es su fecha en la zona del negocio. | Un turno que cruza la medianoche reparte sus cobros en dos días en Ventas, igual que ya reparte sus ventas. |
| DD-10 | La comisión de una venta que la plataforma canceló sigue fuera del resumen de plataformas (D15 b). | Es una exclusión documentada; no hay un caso medido de comisión cobrada sobre un pedido cancelado. |

## Decisiones del dueño del 2026-10-09

| # | Decisión | Por qué / consecuencia |
|---|---|---|
| DO-1 | **Un producto cuenta como vendido el día en que su pedido quedó saldado**: el día del cobro que cubrió el total al centavo. El dinero sigue contando el día de cada cobro, y un producto devuelto resta el día de la devolución. | Antes «Productos vendidos» contaba el día en que se abrió el pedido e incluía pedidos sin cobrar (un fiado salía como vendido). Se corrigió `ProductsSold`. Una devolución **parcial de dinero no resta piezas**: resta el renglón cuyas devoluciones cubren su importe, o todos los del pedido cuando las devoluciones cubren el total. `ProductMargins` (utilidad por producto) **sigue** por día de negocio del pedido: no se movió. |
| DO-2 | **El resumen de Ventas cuenta como devoluciones las HECHAS en el periodo**, las mismas que el desglose por medio ya restó (constitución III). | `refunded` salía en cero con una devolución parcial de hoy porque solo miraba pedidos del periodo en estado «reembolsada». Un reembolso del flujo viejo (sin renglón en el libro de devoluciones) ya no aparece en ese tile: tampoco aparece su cobro en el desglose. |
| DO-3 | **Lo dado por perdido es un concepto propio** («cancelar lo que falta», opción A, 2026-10-09; spec 030 D-15). Lo pagado del pedido sigue siendo venta y dinero del cajón; el resto perdido no es cobro, ni devolución, ni pendiente. | Ventas lo reporta en su tile «Perdido» el día en que se dio por perdido; el corte lo nombra en su propia línea fuera de «Sin cobrar» y del esperado. Cada peso en un solo lugar (principio III), con `TestWriteOffIsCountedOnce` fallando por el concepto duplicado. |

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Ninguna secuencia de devoluciones, cancelaciones y devoluciones de pago de un pedido
  saca más dinero del que entró, en total ni por medio (cubierto por pruebas de los escenarios US1–US2).
- **SC-002**: En un turno con ventas, devoluciones por cada medio y cobros de pedidos de otros
  turnos, el esperado de cada medio es igual a la suma de sus renglones explicados en el corte.
- **SC-003**: Cada uno de los 18 defectos tiene una prueba que falla sin el arreglo y pasa con él, o
  una prueba que demuestra que ya no se reproduce.
- **SC-004**: Ventas por método de dos meses consecutivos no cambia el mes anterior al registrar una
  devolución en el mes siguiente.

## Assumptions

- La migración 0060 ya registra cada devolución con su medio; lo nuevo es ligarla a su turno y
  guardar la propina devuelta aparte.
- Los cortes ya cerrados conservan las cifras que se firmaron; no se recalculan.
- Lo que la 027 ya arregló (pagos devueltos fuera del esperado, quitar renglón sin reponer lo
  consumido) se respeta; D11 sigue abierto en develop: cancelar el pedido completo todavía repone lo
  enviado a cocina.
- Solo la tableta (1024×600) y la pantalla del corte cambian en el front: «Total Dif.», la lista de
  devoluciones del turno y el renglón «Cobros de otros turnos».
