# Feature Specification: Ventas netas y devoluciones a la vista

**Feature Branch**: `029-ventas-netas`

**Created**: 2026-10-08

**Status**: Draft

**Input**: Pedido del dueño (2026-10-08): el «Total» de Ventas tiene que ser lo que se puede facturar
—solo lo cobrado, neto de devoluciones, por el día de cada pago y de cada devolución—, una
devolución se tiene que ver a primera vista, y los hallazgos de la revisión visual de Caja,
Histórico, Ventas y Reportes, más cinco de la prueba por API en el ambiente de pruebas.

## Contexto

Las decisiones del dueño viven en [docs/criterios-de-ventas.md](../../docs/criterios-de-ventas.md):

- Una devolución cuenta **en el mes en que se devolvió**.
- El «Total» de Ventas es **solo lo cobrado, neto de devoluciones**; un pedido abierto o con saldo no
  suma hasta que se cobra.

La spec 031 ya dejó cada pago y cada devolución con su día y su turno, y el desglose por medio de
Ventas ya cuenta por esos días. Lo que falta es que el Total lo use, que la pantalla lo diga, y que
el corte, Ventas y Reportes clasifiquen cada peso igual.

**Fuera de alcance, a propósito:**

- La pantalla de Vender, el tablero de Pedidos, «Pedidos en curso» y la hoja de cobro: otra rama los
  está rehaciendo. Lo que de este spec necesita esas pantallas queda anotado en *Pendiente para el
  tablero* y no se construye aquí.
- En qué día cuentan los **productos** vendidos: pendiente del dueño. Los conteos de productos
  quedan como están.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - El Total de Ventas es lo que se puede facturar (Priority: P1)

El dueño abre Ventas en un periodo y el Total dice cuánto dinero entró, menos lo que se devolvió,
contando cada cobro en el día en que se cobró y cada devolución en el día en que se devolvió. Lo
que falta por cobrar se ve aparte, rotulado como tal, y no se suma. La suma de los medios de pago
es igual al Total.

**Why this priority**: es el número con el que se factura. Medido en el ambiente de pruebas: el Total
sumaba completos dos pedidos ya devueltos y $360 de dos pedidos sin cobrar.

**Independent Test**: un día con un pedido cobrado $100, otro cobrado $92 y devuelto completo, y uno
abierto de $180 sin cobrar: Total $100; «Por cobrar $180 · 1 pedido» aparte.

**Acceptance Scenarios**:

1. **Given** un pedido de $92 cobrado y devuelto completo el mismo día, **When** se ve Ventas de ese
   día, **Then** el Total no lo suma.
2. **Given** un pedido abierto de $180 sin cobrar, **When** se ve Ventas, **Then** el Total no lo suma
   y aparece «Por cobrar» con $180 y 1 pedido, rotulado como fuera del total.
3. **Given** un pedido cobrado en septiembre y devuelto en octubre, **When** se ven los dos meses,
   **Then** septiembre conserva el cobro en su Total y octubre resta la devolución.
4. **Given** cualquier periodo, **When** se ve Ventas, **Then** el Total es igual a la suma de los
   medios de pago mostrados, y cada medio dice que ya tiene restado lo devuelto.
5. **Given** un pedido del día anterior cobrado hoy, **When** se ve Ventas de hoy, **Then** el cobro
   suma hoy.
6. **Given** la propina devuelta al cancelar, **When** se ve Ventas, **Then** no entra al Total (la
   propina nunca entra) y el medio la nombra aparte como propina devuelta.

---

### User Story 2 - Una devolución se reconoce a primera vista en Ventas (Priority: P1)

En la lista de Ventas cada pedido dice la fecha y la hora, y uno con devolución lleva una marca con
el monto devuelto y cuándo se devolvió, sin que cambie el total del pedido de su renglón. Un pedido
con saldo dice cuánto falta.

**Why this priority**: identificar una devolución hoy obliga a abrir pedido por pedido.

**Acceptance Scenarios**:

1. **Given** un pedido de $92 con $30.67 devueltos el 8 oct a las 09:08, **When** se ve en la lista,
   **Then** el renglón dice «Devuelto $30.67 · 8 oct 09:08» y su Total sigue en $92.
2. **Given** un rango de una semana, **When** se ve la lista, **Then** cada renglón dice el día y la
   hora en que se abrió.
3. **Given** un pedido entregado sin cobrar, **When** se ve en la lista, **Then** dice «Por cobrar» y
   el monto.
4. **Given** el detalle de un pedido devuelto, **When** se abre, **Then** dice cuánto se devolvió.

---

### User Story 3 - El corte y Ventas clasifican cada peso igual (Priority: P1)

El corte presenta las devoluciones igual sin importar el medio, separa la propina devuelta de la
venta devuelta igual que Ventas, y un esperado negativo dice por qué. El Histórico de un turno
abierto dice lo mismo que Cajas.

**Acceptance Scenarios**:

1. **Given** un turno abierto con $630 de ingresos, **When** se abre su detalle desde Histórico,
   **Then** dice los mismos ingresos que Cajas, no «Sin ingresos».
2. **Given** una devolución en efectivo y otra con tarjeta en el mismo turno, **When** se ve el
   corte, **Then** las dos aparecen como «Devoluciones» dentro del medio por el que salieron, y
   ninguna aparece en «Salidas de efectivo».
3. **Given** un pedido de $100 + $10 de propina en efectivo cancelado con devolución, **When** se ve
   el corte, **Then** la propina del efectivo es $0 (neta de la devuelta), «Devoluciones» es −$100,
   y el esperado no cambia respecto a hoy.
4. **Given** un turno que solo devolvió con tarjeta una venta cobrada en otro turno, **When** se ve el
   corte, **Then** el renglón de tarjeta en negativo trae una nota que dice que se devolvió dinero
   de ventas cobradas en otro turno.
5. **Given** Ventas y el corte del mismo día con una cancelación con devolución que tenía propina,
   **When** se comparan, **Then** lo devuelto de la venta coincide en los dos, y la propina devuelta
   se nombra aparte en los dos.

---

### User Story 4 - El detalle de un pedido y la API dicen la verdad (Priority: P2)

1. **Given** un pedido devuelto $100, **When** se consulta su detalle, **Then** dice «devuelto $100»,
   no $0.
2. **Given** un renglón ya devuelto completo (o un pedido ya devuelto entero), **When** se pide
   devolver contra ese renglón sin monto, **Then** se rechaza diciendo que de ese producto ya no
   queda nada por devolver.
3. **Given** un pedido de plataforma, **When** se intenta crear «en mostrador», **Then** se rechaza
   con un mensaje para quien opera (validación, no error del servidor).
4. **Given** un pedido de $100, **When** se intenta cobrar $0.005, **Then** se rechaza como monto
   menor a un centavo y el pedido sigue debiendo $100.

---

### User Story 5 - Las pantallas se leen sin ambigüedad en la tableta (Priority: P2)

1. Reportes «Por medio de pago» dice que ya tiene restadas las devoluciones.
2. Todo control tocable de Caja, Histórico y sus pestañas mide al menos 44 px.
3. Los montos con centavos se muestran con dos decimales ($2,165.20, no $2,165.2).
4. El concepto de un movimiento de efectivo no se corta a 220 px cuando hay ancho.
5. La lista de devoluciones del corte no tiene scroll propio dentro de una página que ya hace
   scroll.
6. El conteo del pie de Ventas y el recuadro «Ventas» dicen qué cuentan (con y sin canceladas).
7. La tabla de Ventas deja ver al menos cuatro renglones a 1024×600 con el resumen arriba.
8. «Propinas por día» muestra la fecha en formato local («8 oct»), no «2026-10-08».
9. En Utilidad por producto, un producto sin costo capturado dice «sin costo capturado» y su venta no
   se suma como margen.

### Edge Cases

- Un medio que en el periodo solo tuvo devoluciones sale con Total negativo; la suma de medios sigue
  igual al Total.
- Un pedido cancelado sin devolución registrada (anterior a las devoluciones parciales) no suma ni
  resta.
- Un pedido con saldo y devolución parcial: aparece en «Por cobrar» con lo que falta; su cobro y su
  devolución cuentan en sus días.
- Buscar un folio de plataforma: el resumen sale del mismo pedido; el Total es lo cobrado de ese
  pedido menos lo devuelto.
- Turno abierto con arqueo a ciegas: el detalle desde Histórico sigue ocultando lo esperado.
- Devolución de un pago de plataforma en efectivo: sale del cajón y se presenta en el medio por el
  que salió.
- Un renglón de Utilidad con parte de las piezas costeadas: el margen solo cuenta las costeadas y el
  renglón dice cuánto se vendió sin costo.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: El Total de Ventas MUST ser la suma, por medio, de lo cobrado en el periodo (por el día
  de cada pago) menos lo devuelto de la venta (por el día de cada devolución), sin propinas y sin
  pedidos sin cobrar. MUST ser igual a la suma de los medios que muestra la pantalla.
- **FR-002**: Ventas MUST mostrar aparte lo que falta por cobrar de los pedidos del periodo (monto y
  número de pedidos), rotulado como fuera del Total.
- **FR-003**: Cada medio de pago en Ventas MUST decir que lo devuelto ya está restado, y nombrar
  aparte la propina devuelta cuando la hubo.
- **FR-004**: Cada renglón de la lista de Ventas MUST mostrar día y hora; uno con devolución MUST
  mostrar el monto devuelto y el momento de la última devolución; uno con saldo MUST mostrar lo que
  falta por cobrar. El Total del renglón no cambia.
- **FR-005**: El detalle de un pedido MUST devolver el monto devuelto real.
- **FR-006**: El detalle de un turno abierto desde Histórico MUST calcular sus ingresos en vivo, igual
  que Cajas, respetando el arqueo a ciegas.
- **FR-007**: El corte MUST presentar toda devolución como «Devoluciones» dentro del medio por el que
  salió, sin importar si salió del cajón; las salidas de caja de una devolución MUST dejar de
  contarse en «Salidas de efectivo». El esperado de cada medio no cambia.
- **FR-008**: La propina del corte (por medio) MUST ser neta de la propina devuelta en el turno, y
  «Devoluciones» MUST ser solo la parte de la venta, igual que Ventas.
- **FR-009**: Un medio del corte cuyo total del turno es negativo MUST llevar una nota que lo explique.
- **FR-010**: Devolver contra un renglón (o un pedido) sin nada por devolver MUST rechazarse con un
  mensaje que diga que ya no queda nada por devolver.
- **FR-011**: Crear un pedido de plataforma en mostrador MUST rechazarse como validación antes de
  llegar a la base.
- **FR-012**: Cobrar un monto menor a un centavo MUST rechazarse.
- **FR-013**: Reportes «Por medio de pago» MUST decir que resta devoluciones.
- **FR-014**: Utilidad por producto MUST distinguir lo vendido sin costo capturado y no contarlo como
  margen.
- **FR-015**: Los hallazgos de presentación de la US5 (44 px, dos decimales, concepto, scroll
  anidado, conteos, renglones visibles, fecha local) MUST quedar resueltos en Caja, Histórico,
  Ventas y Reportes.

### Key Entities

- **Cobro neto de un periodo**: lo cobrado por medio en el periodo menos lo devuelto de la venta por
  medio en el periodo.
- **Por cobrar**: total menos lo cobrado de los pedidos vivos del periodo con saldo.

## Pendiente para el tablero (rama que rehace Pedidos)

No se construye aquí porque el tablero lo está rehaciendo otra rama:

- Mostrar fecha y hora y la marca de devolución en las tarjetas de Pedidos.
- Ocultar «Devolver» cuando ya se devolvió todo: la condición hoy es `total − por cobrar > 0`; debe
  ser `total − por cobrar − devuelto > 0`. El dato `refund` ya viaja en el tablero, en entregadas y,
  con este spec, en el detalle.

## Success Criteria *(mandatory)*

- **SC-001**: En cualquier periodo, Total de Ventas = suma de los medios mostrados, al centavo.
- **SC-002**: Registrar una devolución en un mes no cambia el Total del mes anterior.
- **SC-003**: Ventas y el corte de un mismo turno-día dicen la misma cifra devuelta de la venta.
- **SC-004**: A 1024×600 la tabla de Ventas muestra al menos 4 renglones con el resumen visible.
- **SC-005**: Cada hallazgo tiene una prueba que falla sin el arreglo, o se declara sin prueba
  automática y con captura.

## Assumptions

- Sin migraciones: todo sale de columnas que ya existen (día y turno de pagos y devoluciones,
  propina devuelta, salida de caja de la devolución).
- «Ventas» (conteo) sigue contando los pedidos del periodo no cancelados, y el promedio sigue siendo
  el ticket promedio de esos pedidos: son cifras de venta, no de dinero, y se rotulan así.
- Los cortes ya cerrados conservan su esperado firmado; la propina neta y las devoluciones se leen
  en vivo, como ya se leen los cobros de otros turnos.
- El formato de dos decimales aplica a toda cifra con centavos del sistema; una cifra entera sigue
  sin decimales ($45).
