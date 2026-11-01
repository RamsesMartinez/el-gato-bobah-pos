# Feature Specification: Editar un gasto ya guardado (entrega 1)

**Feature Branch**: `053-editar-gasto`

**Created**: 2026-11-01

**Status**: Draft

**Input**: Issue #167, primera entrega: agregar y borrar renglones, recepción en varias entregas con
su fecha, encabezado editable y una vista previa de los efectos antes de confirmar. Las devoluciones
y la cancelación de un gasto con pagos y mercancía recibida quedan para una segunda entrega.

**Evidencia** (gasto real, 1-nov-2026): una compra de $67.50 de tapas Traveler 16 oz (50 piezas) y
fajillas de cartón (10 piezas), pagada en efectivo, se guardó como «solo el gasto», sin mercancía.
Las existencias quedaron en −5 tapas y 0 fajillas, y el detalle del gasto no ofrecía ninguna acción:
sin renglones no aparece ni «Corregir lo que llegó».

**Principios fijos del issue** (no se negocian en esta spec):

- El inventario y la caja son libros de movimientos: no se borra nada; toda corrección es un
  contra-movimiento ligado al original y al gasto.
- Un pago ya registrado nunca se modifica; una diferencia entra como pago nuevo (o, en la entrega 2,
  como devolución).
- Todo cambio deja bitácora: quién, cuándo y qué.
- Cada acción muestra una vista previa de sus efectos antes de confirmar.
- Permisos: por omisión el rol admin; el detalle fino lo define #61.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Agregar la mercancía a un gasto que se guardó sin ella (Priority: P1)

Quien administra abre un gasto guardado «solo el gasto» y le agrega los renglones de lo que se
compró. Lo que agrega ocupa primero el importe que el gasto ya tenía sin desglosar, así que el
importe y el pago no cambian si los renglones suman lo mismo. Al recibirlos, las existencias suben.

**Why this priority**: es el caso del 1-nov y hoy no tiene salida: la compra existe, el dinero salió,
y el almacén dice que hay menos de lo que hay.

**Independent Test**: con el gasto del 1-nov restaurado en local ($67.50, pagado, sin renglones),
agregar «Tapas 50 piezas $50.00» y «Fajillas 10 piezas $17.50» y recibirlos: tapas pasan de −5 a 45 y
fajillas de 0 a 10; el importe sigue en $67.50, el pago sigue siendo uno de $67.50 y el gasto sigue
pagado.

**Acceptance Scenarios**:

1. **Given** un gasto de $67.50 pagado y sin renglones, **When** se agregan dos renglones que suman
   $67.50 sin marcarlos como recibidos, **Then** la vista previa dice «Importe $67.50 → $67.50», no
   muestra ningún movimiento de inventario, y al confirmar los renglones aparecen «por recibir».
2. **Given** esos dos renglones por recibir, **When** se reciben completos con la fecha de hoy,
   **Then** la vista previa muestra cada artículo «−5 → 45» y «0 → 10» en verde, y al confirmar las
   existencias quedan así y el pago no cambia.
3. **Given** un gasto sin renglones, **When** al agregar un renglón se marca «Ya llegó» con una fecha,
   **Then** en una sola confirmación el renglón queda agregado y recibido con esa fecha.
4. **Given** un gasto de $67.50 pagado, **When** se agregan renglones que suman $80.00, **Then** la
   vista previa muestra «Importe $67.50 → $80.00» en ámbar con el aviso «Falta pagar $12.50» y la
   acción «Registrar pago»; confirmar sin pagar deja el gasto pendiente con saldo de $12.50.

---

### User Story 2 - Recibir en varias entregas, cada una con su fecha (Priority: P1)

El proveedor entrega parte de un pedido hoy y el resto otro día. Quien recibe registra cada entrega
con su fecha y ve, por renglón, cuánto falta y qué entregas llevan registradas. Si llega menos de lo
pedido, decide si lo demás sigue pendiente o si el importe se ajusta a lo recibido.

**Why this priority**: sin la fecha de cada entrega, el almacén registra la mercancía el día en que
alguien se acordó de capturarla, y la segunda entrega no tiene dónde quedar.

**Independent Test**: un gasto con un renglón de 5 cajas; recibir 3 con fecha de ayer y «Sigue
pendiente»; después recibir 2 con fecha de hoy. El renglón queda completo, su historial muestra las
dos entregas con sus fechas y el gasto pasa a «recibido» con la fecha de la última.

**Acceptance Scenarios**:

1. **Given** un renglón de 5 cajas sin recibir, **When** se registra una entrega de 3 con fecha de
   ayer, **Then** la vista previa preselecciona «Sigue pendiente de entrega» (se compra por pieza o
   empaque), y al confirmar el renglón muestra «Faltan 2» y el gasto sigue «por recibir».
2. **Given** ese renglón con 2 pendientes, **When** se registra otra entrega de 2 con la fecha de hoy,
   **Then** el renglón queda completo, su historial lista «ayer: 3 cajas» y «hoy: 2 cajas», y el
   gasto queda recibido con la fecha de hoy.
3. **Given** un renglón de 10 kg, **When** llegan 8.5 kg, **Then** la vista previa preselecciona
   «Ajustar el importe a lo recibido» y muestra el importe del renglón y el del gasto bajando en
   proporción; quien recibe puede cambiar a «Sigue pendiente».
4. **Given** la lista de gastos, **When** un gasto tiene algo por recibir, **Then** su renglón ofrece
   el atajo «Recibir», que abre la recepción de ese gasto sin pasar por el detalle.
5. **Given** una entrega con fecha anterior a un conteo cerrado que ya incluyó ese artículo, **Then**
   la vista previa pide decidir «Ajustar el inventario» o «No mover» en el renglón de ese artículo, y
   «Confirmar» no se habilita hasta decidir.

---

### User Story 3 - Borrar un renglón o sustituir un producto (Priority: P2)

Quien administra borra un renglón que no se compró, o que el proveedor cambió por otro. Si el
renglón no había llegado, no se mueve el inventario. Si ya había llegado, sale con un
contra-movimiento fechado el día en que llegó; si un conteo posterior ya contó ese artículo, quien
borra decide si el inventario se ajusta o se queda como lo dejó el conteo.

**Why this priority**: es la otra mitad de corregir un gasto, pero hoy existe un camino parcial
(«Corregir lo que llegó» permite quitar renglones), así que el hueco es menor que el de la historia 1.

**Independent Test**: en un gasto con tres renglones (uno sin recibir, uno recibido sin conteo
posterior, uno recibido con conteo posterior), borrar los tres: el primero no crea movimientos, el
segundo crea un contra-movimiento con su fecha de llegada, y el tercero pide la decisión y deja
registrada la que se tomó.

**Acceptance Scenarios**:

1. **Given** un renglón sin recibir, **When** se borra, **Then** la vista previa no muestra nada en
   Inventario, muestra el importe bajando por el del renglón, y al confirmar no hay ningún movimiento
   de inventario nuevo.
2. **Given** un renglón que llegó el 10 de octubre sin conteos posteriores de ese artículo, **When**
   se borra, **Then** se registra un contra-movimiento por lo que llegó, ligado al renglón y al gasto,
   con fecha 10 de octubre, y las existencias bajan.
3. **Given** un renglón que llegó el 10 de octubre y un conteo cerrado el 20 que incluyó el artículo,
   **When** se borra, **Then** la vista previa pide «Ajustar el inventario» (las existencias bajan) o
   «No mover» (las existencias se quedan como las dejó el conteo); la decisión queda en la bitácora y
   en los movimientos del artículo.
4. **Given** un renglón sin recibir que el proveedor sustituyó, **When** en la misma edición se borra
   y se agrega el producto nuevo con el mismo importe, **Then** la vista previa muestra el importe sin
   cambio y una sola confirmación aplica las dos cosas.
5. **Given** un borrado que dejaría existencias negativas, **Then** la vista previa lo marca en ámbar
   («quedarán −3») y permite confirmar.

---

### User Story 4 - Corregir el encabezado (Priority: P2)

Proveedor, categoría, comentario y fecha del gasto se corrigen desde el detalle, con bitácora.

**Why this priority**: hoy cada uno de estos errores se arregla por SQL; son baratos de corregir y no
mueven inventario ni dinero.

**Independent Test**: cambiar proveedor, categoría, comentario y fecha de un gasto; el detalle
muestra lo nuevo y la bitácora muestra el valor anterior y el nuevo de cada campo, con quién y cuándo.

**Acceptance Scenarios**:

1. **Given** un gasto guardado, **When** se cambia el proveedor y la categoría, **Then** la vista
   previa (compacta: no hay avisos) lista «Proveedor A → B» y «Categoría X → Y», y al confirmar quedan
   en el gasto y en la bitácora.
2. **Given** un gasto del 3 de octubre, **When** se cambia su fecha al 31 de octubre, **Then** solo
   cambia la fecha del gasto: las fechas de llegada y de los pagos no se mueven.
3. **Given** una fecha futura, **Then** se rechaza.

---

### User Story 5 - Ver los efectos antes de confirmar (Priority: P1, transversal)

Toda acción de esta spec (agregar, borrar, recibir, editar encabezado) pasa por la misma vista previa
antes de escribir nada: qué cambia en Inventario, en Dinero y en Costos, cada cosa como «antes →
después», con las decisiones dentro de la línea a la que afectan.

**Why this priority**: es lo que hace seguras a las otras cuatro historias; sin ella, un borrado de un
renglón recibido mueve existencias que nadie vio venir.

**Independent Test**: preparar una edición con un renglón agregado, uno borrado ya recibido con
conteo posterior y un importe que sube con pagos; la vista previa muestra los tres grupos, el aviso de
dinero con su acción y la decisión dentro del renglón del artículo contado; «Confirmar» se habilita
solo al decidir; lo que se confirma es exactamente lo que se vio.

**Acceptance Scenarios**:

1. **Given** una acción sin avisos ni decisiones, **Then** la vista previa es compacta: una línea por
   efecto y «Confirmar» habilitado.
2. **Given** una acción con una decisión pendiente, **Then** «Confirmar» está deshabilitado hasta que
   se resuelva, y la decisión se ve dentro de la línea afectada, no en un diálogo aparte.
3. **Given** que entre la vista previa y la confirmación otra persona cambió el mismo gasto o vendió
   el mismo artículo de modo que el resultado sería distinto, **When** se confirma, **Then** no se
   escribe nada y se muestra la vista previa actualizada para volver a confirmar.
4. **Given** una pantalla de 1024×600, **Then** la vista previa y sus decisiones se leen y se tocan sin
   scroll horizontal, con controles de al menos 44 px.

### Edge Cases

- **Gasto anulado**: no se edita, no se recibe, no se le agregan renglones.
- **Anular un gasto con un pago ya dentro de un corte**: no se permite en esta entrega, aunque una
  edición lo haya dejado pendiente: el corte seguiría contando la salida de un gasto anulado (anular
  con devolución es la entrega 2). Un pago que todavía espera corte no lo impide (spec 050).
- **Renglón excluido** («no es del local», spec 049): no se borra desde esta edición ni cuenta para
  el importe; sigue con su propio botón «Incluir».
- **Renglón «otros» (sin artículo de inventario)**: se agrega y se borra igual, sin efectos de
  inventario.
- **Regalo** (importe 0): se agrega y se recibe; no cambia el importe ni el costo del insumo.
- **Llega más de lo que faltaba**: se mantiene la regla actual: pide cuánto se pagó por lo de más y
  sube el importe.
- **Entrega con fecha futura o anterior a la fecha del gasto**: se rechaza.
- **Renglón con entregas parciales que se borra**: el contra-movimiento revierte cada entrega con su
  propia fecha; la decisión por conteo posterior se toma por artículo.
- **Gasto con importe sin desglosar y renglones**: agregar consume lo sin desglosar; borrar un
  renglón no convierte su importe en «sin desglosar»: lo resta del importe.
- **Importe que quedaría en $0** (borrar todos los renglones de un gasto sin importe sin desglosar):
  se rechaza; quitar el gasto entero es anularlo (entrega 2 para los pagados).
- **Importe que baja de lo pagado**: el aviso «Pagado de más $X» queda en el detalle; en esta entrega
  no hay acción de devolución.
- **Importe que sube de lo pagado**: el gasto vuelve a «pendiente» con su saldo; «Registrar pago»
  usa el flujo de pagos que ya existe (la caja abierta de hoy, con las reglas de corte de las specs 047
  y 048; una caja cerrada no se toca).
- **Un gasto pendiente sin pagos** (a crédito): el importe cambia sin aviso de dinero más allá del
  «antes → después».
- **Venta simultánea del mismo insumo**: la confirmación no espera a la venta; responde «vuelve a
  intentar» como ya hace la recepción.
- **Conteo abierto del artículo** al momento de recibir o borrar: no cuenta como conteo posterior; el
  conteo abierto ya ajusta contra lo que había al capturar (spec 038).
- **Costo del insumo**: recibir actualiza el costo con el método de la empresa (regla que ya existe);
  borrar un renglón recibido no restaura el costo anterior, y la vista previa lo dice en el grupo
  Costos («el costo se queda en $X»).
- **Gastos guardados antes de esta spec**: sus movimientos de compra no traen fecha de entrega; para
  ellos la fecha de llegada es el día en que se registró el movimiento.

## Requirements *(mandatory)*

### Functional Requirements

**Encabezado**

- **FR-001**: Quien tiene el permiso de editar gastos MUST poder cambiar proveedor (o quitarlo),
  categoría, comentario y fecha de un gasto no anulado.
- **FR-002**: Cambiar la fecha del gasto MUST cambiar solo esa fecha; las fechas de llegada y de los
  pagos no se mueven. La fecha no puede ser futura.

**Renglones**

- **FR-003**: Quien tiene el permiso de editar gastos MUST poder agregar renglones (artículo de
  inventario con su cantidad y empaque o unidad, o renglón sin artículo; con importe, o regalo en 0) a
  un gasto no anulado, tenga o no renglones.
- **FR-004**: Un renglón agregado MAY marcarse «Ya llegó» con su fecha de llegada; si no, queda por
  recibir.
- **FR-005**: Quien tiene el permiso de editar gastos MUST poder borrar renglones no excluidos. Los
  renglones no se editan: corregir uno es borrarlo y agregar otro.
- **FR-006**: Borrar un renglón MUST conservarlo marcado como quitado (no se elimina) y MUST NOT crear
  movimientos de inventario si no había llegado nada de él.
- **FR-007**: Borrar un renglón que llegó (total o parcialmente) MUST crear, por cada entrega, un
  contra-movimiento por la cantidad que esa entrega sumó, ligado al renglón y al gasto y fechado el
  día de esa entrega.
- **FR-008**: Una sola edición MAY combinar varios renglones agregados y borrados; se previsualiza y
  se confirma como una sola operación (todo o nada).

**Importe y pagos**

- **FR-009**: El importe resultante de una edición o una entrega MUST ser el mayor entre (a) el
  importe anterior menos los importes de los renglones borrados y de lo ajustado a lo recibido, más
  lo pagado por lo que llegó de más (FR-021), y (b) la suma de los importes de los renglones vivos no
  excluidos. Así, lo agregado ocupa primero lo que el importe tenía sin desglosar, y solo lo que
  excede lo sube.
- **FR-010**: Los pagos ya registrados MUST NOT modificarse por ninguna acción de esta spec.
- **FR-011**: Cuando una acción cambia el importe, el estado de pago MUST recalcularse: lo pagado
  menor que el importe deja el gasto pendiente con su saldo; lo pagado igual o mayor lo deja pagado,
  aunque antes estuviera pendiente. Excepción: un gasto pagado que no tiene ningún pago registrado
  (de antes de que existieran los pagos) MUST quedarse pagado. Si la acción no cambia el importe, el
  estado no se toca.
- **FR-012**: Si después de la acción hay pagos y lo pagado es menor que el importe, la vista previa
  MUST mostrar el aviso «Falta pagar $X» con dos salidas: «Registrar pago» (abre el pago que ya existe
  con el saldo puesto) o «Dejar pendiente»; el detalle MUST mostrar el saldo.
- **FR-013**: Si lo pagado supera al importe, la vista previa y el detalle MUST mostrar «Pagado de
  más $X»; en esta entrega no hay acción de devolución.

**Recepción**

- **FR-014**: Quien tiene el permiso de recibir mercancía MUST poder registrar una entrega de uno o
  varios renglones con una fecha (hoy por omisión) y, en opciones avanzadas, una fecha distinta por
  renglón.
- **FR-015**: La fecha de una entrega MUST NOT ser futura ni anterior a la fecha del gasto.
- **FR-016**: Cada entrega MUST quedar registrada como movimientos de compra del renglón con su
  fecha, y el detalle del gasto MUST mostrar por renglón lo pedido, lo recibido, lo pendiente y el
  historial de entregas (fecha y cantidad, incluidas las correcciones).
- **FR-017**: Si una entrega trae menos de lo que falta, quien recibe MUST elegir «Sigue pendiente de
  entrega» o «Ajustar el importe a lo recibido». La opción viene preseleccionada: pendiente si el
  renglón se compra por pieza o por empaque; ajustar si se compra por peso, volumen o longitud.
- **FR-018**: «Ajustar el importe a lo recibido» MUST cerrar lo que falta del renglón, bajar el
  importe del renglón en proporción a lo que no llegó y bajar el importe del gasto por esa diferencia
  (con los avisos de FR-012 y FR-013).
- **FR-019**: El gasto MUST quedar «recibido» con la fecha de la última entrega cuando ya no queda
  nada pendiente en ningún renglón vivo no excluido.
- **FR-020**: La lista de gastos MUST ofrecer el atajo «Recibir» en cada gasto con algo por recibir,
  a quien tiene el permiso de recibir.
- **FR-021**: Lo que llegue de más que lo que faltaba MUST seguir la regla que ya existe: pide el
  importe de lo de más y sube lo pedido, el importe del renglón y el del gasto.

**Conteo posterior y existencias negativas**

- **FR-022**: Cuando un movimiento de esta spec (contra-movimiento de un borrado o entrada de una
  entrega) lleva una fecha anterior a un conteo cerrado que ya contó ese artículo, la vista previa
  MUST pedir en la línea de ese artículo «Ajustar el inventario» o «No mover», y la confirmación MUST
  rechazarse si falta esa decisión.
- **FR-023**: «Ajustar el inventario» MUST registrar el movimiento y mover las existencias. «No
  mover» MUST registrar el mismo movimiento ligado al renglón más un movimiento de compensación por la
  cantidad contraria, ligado al conteo que lo motivó, de modo que las existencias quedan como las dejó
  el conteo y el libro muestra por qué.
- **FR-024**: Las existencias negativas que resulten MUST permitirse y la vista previa MUST marcarlas
  como aviso.

**Vista previa**

- **FR-025**: Toda acción de esta spec MUST mostrar antes de escribir una vista previa calculada por
  el servidor con la misma regla que aplica al confirmar, agrupada en Inventario (existencias por
  artículo, antes → después), Dinero (importe, pagado, saldo o pagado de más, antes → después) y
  Costos (costo por unidad de cada insumo, antes → después, o «sin cambio»), más los cambios de
  encabezado.
- **FR-026**: Cada línea MUST distinguir por color: rojo lo que baja, verde lo que sube, ámbar un
  aviso; y no depender solo del color (signo o texto).
- **FR-027**: Las decisiones (FR-012, FR-017, FR-022) MUST aparecer dentro de la línea afectada y solo
  cuando hay conflicto; «Confirmar» MUST estar deshabilitado mientras falte alguna.
- **FR-028**: Sin avisos ni decisiones, la vista previa MUST mostrarse en versión compacta.
- **FR-029**: Si al confirmar el estado ya no es el que se previsualizó (otra edición, otra entrega o
  una venta del mismo artículo), MUST NOT escribirse nada y MUST mostrarse la vista previa nueva.

**Bitácora y permisos**

- **FR-030**: Toda acción confirmada de esta spec MUST quedar en la bitácora de auditoría con quién,
  cuándo, el gasto, y el antes y después de lo que cambió (campos del encabezado, renglones agregados
  y borrados, entregas con su fecha, importe, decisiones de conteo).
- **FR-031**: Editar encabezado y renglones MUST requerir un permiso propio, que por omisión tiene
  solo el rol admin. Recibir MUST seguir con el permiso de recibir mercancía que ya existe. Lo que
  desde la edición mueve el almacén —agregar un renglón «Ya llegó» o quitar uno que ya sumó— MUST
  exigir los dos permisos.

**Pagos nuevos** (de la prueba con usuario nuevo)

- **FR-032**: Un pago nuevo MUST NOT pasar de lo que falta pagar (importe menos lo pagado); el
  importe que se propone al pagar es ese saldo, no el total del gasto.
- **FR-033**: El detalle de un gasto pendiente con saldo MUST ofrecer «Registrar pago» por ese saldo.

### Key Entities

- **Gasto**: encabezado (proveedor, categoría, comentario, fecha), importe, estado de pago, fecha en
  que quedó recibido. El importe deja de ser fijo: lo mueve una edición según FR-009 y FR-018.
- **Renglón del gasto**: artículo o descripción, cantidad pedida, empaque o unidad, importe, cuánto
  llegó, cuánto ya no llegará, si fue quitado y cuándo. Ya existe.
- **Entrega**: no es una entidad guardada aparte. Es el conjunto de movimientos de compra de un
  renglón con la misma fecha de entrega; el historial sale de ahí.
- **Movimiento de inventario**: ya existe; gana la fecha de entrega (el día en que pasó, distinto del
  momento en que se capturó) y, en la compensación de «No mover», la liga al conteo.
- **Conteo cerrado**: ya existe; define «conteo posterior» cuando contó el artículo después de la
  fecha del movimiento.
- **Evento de bitácora**: ya existe; esta spec agrega las acciones de editar encabezado, agregar y
  borrar renglones, recibir con fecha y decidir ante un conteo.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: El gasto del 1-nov se corrige desde la tableta, sin SQL, en menos de 2 minutos: dos
  renglones agregados y recibidos, existencias de tapas y fajillas correctas, importe y pago intactos.
- **SC-002**: Cero acciones de esta spec escriben algo que no se vio en su vista previa: en las
  pruebas, lo confirmado coincide cifra por cifra con lo previsualizado, y un cambio de estado entre
  ambos rechaza la confirmación.
- **SC-003**: Cero pagos ya registrados cambian por una edición, en todas las pruebas.
- **SC-004**: El 100% de las acciones confirmadas aparece en la bitácora con quién, cuándo y qué.
- **SC-005**: Una persona sin contexto, a 1024×600, completa «agregar dos renglones y recibirlos» y
  «recibir 3 de 5 y luego el resto con otra fecha» sin ayuda.
- **SC-006**: Después de borrar un renglón recibido, la suma de los movimientos del renglón es cero y
  las existencias del artículo coinciden con su libro de movimientos.

## Assumptions

- **Entrega 2** (issue nuevo): devoluciones (fila negativa en Pagos, «Registrar devolución», cómo la
  muestra el corte), cancelar un gasto con pagos y mercancía recibida, mover juntas las fechas de
  llegada y pagos al cambiar la fecha del gasto y «fechas por separado». Fuera del issue entero: saldo
  a favor con el proveedor, tolerancias de peso, ventana de tiempo para editar y permisos finos (#61).
- «Corregir lo que llegó» (spec 038) y el botón «Incluir/Excluir» (spec 049) se quedan como están; la
  edición nueva convive con ellos. «Agregar un artículo que faltó» del detalle se reemplaza por
  «Agregar renglón» con «Ya llegó», que cubre el mismo caso con la regla de importe de FR-009.
- «Por recibir» (spec 038) sigue existiendo; su recepción gana la fecha de entrega.
- El costo del insumo al recibir sigue la regla que ya existe (último o promedio, spec 038); deshacer
  una entrada no reconstruye el costo anterior. La spec 052, en paralelo, recalcula costos de opciones
  e insumos; si cambia esa regla, la vista previa refleja la que quede.
- Permiso de recibir: el que ya existe (`inventory.receive`). Permiso de editar: nuevo, solo admin por
  omisión, sin pantalla para repartirlo (eso es #61).
- El negocio tiene conexión: la edición exige servidor (constitución, «El local tiene conexión»).
- La fecha de un gasto o de una entrega es un día del negocio (zona horaria de la empresa), no un
  instante.
