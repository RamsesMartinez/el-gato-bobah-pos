# Feature Specification: Caja, propinas y cobro con tarjeta

**Feature Branch**: `032-caja-propinas-tarjeta`

**Created**: 2026-10-09

**Status**: Draft

**Input**: Decisiones del dueño del 2026-10-09, Bloque 1 «Caja y propinas» (puntos 1–7) y Bloque 2
«Cobro con tarjeta» (puntos 8–11). Las decisiones son finales; este spec las traduce a requisitos
verificables y enumera sus bordes. Se citan por número de punto; los datos del negocio que las
motivaron no entran al repositorio.

## Decisiones del dueño del 2026-10-09 (posteriores a las maquetas)

- **D-A** «Entregar propina» es la opción C: reparto entre una o varias personas, parejo o ajustado.
- **D-B** Los centavos no se reparten. El reparto parejo entrega pesos enteros por persona; lo que no
  alcanza un peso entero por persona (incluidos $0.01–$0.99) se queda como propina pendiente para el
  siguiente reparto. Los montos ajustados también son pesos enteros.
- **D-C** El correo diario va a las direcciones capturadas en un campo nuevo de Configuración del
  negocio (una o varias, validadas).

## Decisiones del dueño del 2026-10-10

- **D-D** La apertura a ciegas se compara contra el **fondo que se dejó al cerrar**, no contra todo
  lo contado: al cerrar se registra cuánto se queda de fondo para el siguiente turno (el resto se
  retira), y la apertura pide motivo solo si lo contado difiere de ese fondo. Reemplaza la
  comparación contra el conteo de cierre de FR-014/FR-015.
- **D-E** La terminal por omisión de cada usuario es la última que usó al cobrar; no hay pantalla
  para fijarla aparte.
- Bordes nuevos: **EB-48** fondo que se deja mayor a lo contado (se rechaza); **EB-49** cierre sin
  fondo capturado en pantalla (no deja cerrar; un campo vacío no es cero); **EB-50** cierres de antes
  de la decisión sin fondo registrado (se compara contra lo contado).

## Casos de borde enumerados ANTES del diseño *(constitución IV)*

Formas de fallar, agrupadas por las cuatro familias de la constitución. Cada una tiene un requisito
(FR) y, en `tasks.md`, un test que se escribe antes del código.

### Dinero clasificado una sola vez (principio III)

- **EB-01** Una propina contada como venta: el total de ventas del día sube por dinero del personal.
- **EB-02** Una propina entregada contada como gasto: el reporte de gastos sube y el resultado baja
  por dinero que nunca fue del negocio.
- **EB-03** La propina se resta dos veces del cajón: una al cobrarla (no entra como venta) y otra al
  entregarla.
- **EB-04** La propina de tarjeta pagada con efectivo del cajón no se descuenta del efectivo
  esperado: el arqueo sale con faltante igual a la propina.
- **EB-05** La propina de efectivo se entrega y además se declara como «pendiente» al cerrar:
  aparece en dos renglones del corte.
- **EB-06** Se entrega más propina de la pendiente (por método o en total): el pendiente queda
  negativo y el cajón pierde dinero del negocio sin que nadie lo vea.
- **EB-07** Una propina de un cobro que luego se devuelve o se anula: el pendiente sigue contándola.
- **EB-08** Una propina ya entregada cuyo cobro se devuelve: el pendiente queda negativo.
- **EB-09** Los gastos históricos que en realidad eran propinas (punto 1) siguen en el reporte de
  gastos después de migrarlos, o se cuentan en los dos lados.
- **EB-10** Un traspaso entre cajas sale como gasto en el reporte de gastos (punto 7).

### Arqueo y cierre

- **EB-11** El cajero cierra con propinas pendientes y no se le pregunta qué hacer con ellas.
- **EB-12** Elige «se quedan en caja»: el siguiente turno no las ve como pendientes y nadie las
  entrega jamás; o las ve y además las cuenta como fondo del negocio.
- **EB-13** El conteo de cierre mezcla efectivo del negocio y propinas: la diferencia del arqueo
  esconde la propina pendiente.
- **EB-14** Dos cajas abiertas a la vez (puerta VIII): la propina se entrega desde una caja distinta
  a la que la cobró y el pendiente se calcula por la caja equivocada.

### Movimientos y su corrección

- **EB-15** «Corregir» una salida edita la original: el corte ya impreso deja de cuadrar.
- **EB-16** Se corrige dos veces la misma salida, o se corrige un reverso: dos reversos de un mismo
  peso.
- **EB-17** Se corrige una salida de un turno ya cerrado: el reverso cae en el turno abierto y mueve
  su arqueo, o cae en el cerrado y lo reescribe.
- **EB-18** Una salida sin concepto se guarda por un camino viejo (otra pantalla, la API directa).
- **EB-19** Un concepto archivado o fusionado sigue apareciendo, o al fusionar se pierden las
  salidas del duplicado.
- **EB-20** Dos personas crean el mismo concepto a la vez, o con mayúsculas/espacios distintos.

### Día del gasto

- **EB-21** Una salida capturada después de medianoche con el turno abierto del día anterior cae en
  el día de hoy (punto 5: manda el día del turno).
- **EB-22** La fecha del documento (ticket del proveedor) mueve el día del gasto.

### Apertura a ciegas

- **EB-23** La pantalla de apertura deja ver el cierre anterior (en la red, en el texto, en un
  placeholder): el conteo deja de ser ciego.
- **EB-24** Campo de denominación vacío leído como cero que «cuadra» por accidente, o como cero que
  dispara diferencia (familia «valor vacío»).
- **EB-25** Recargar la tableta a media apertura evade el motivo (familia «estado que no sobrevive»).
- **EB-26** Primer turno de una caja sin cierre anterior: no hay contra qué comparar y no debe pedir
  motivo.
- **EB-27** Diferencia de centavos por redondeo dispara un motivo falso.

### Tarjeta

- **EB-28** Terminal de otra sucursal ofrecida al cobrar.
- **EB-29** La terminal por omisión del usuario se archivó: se cobra con una terminal inexistente o
  la pantalla truena.
- **EB-30** Negocio sin terminales activas: no se puede cobrar con tarjeta y nadie sabe por qué.
- **EB-31** Pago con tarjeta sin terminal o sin débito/crédito por una ruta vieja (cobro dividido,
  cobro idempotente reintentado, cobro de plataforma).
- **EB-32** Arqueo por terminal: una terminal sin cobros no se pide; una terminal con cobros
  archivada a media jornada sí se tiene que pedir.
- **EB-33** El modo de arqueo cambia con la caja abierta: el cierre usa un modo distinto del que vio
  el cajero.
- **EB-34** Devolución con tarjeta sin folio: se registra la devolución y el dinero nunca se regresa
  en la terminal.
- **EB-35** Devolución de un cobro con tarjeta anterior a esta feature (sin terminal registrada).
- **EB-36** Folio de devolución con espacios o vacío tras recortar.

### Correo diario

- **EB-37** El correo sale dos veces (reintento, dos cierres el mismo día, dos cajas).
- **EB-38** El correo lleva datos de otra empresa (RLS en un trabajo sin request).
- **EB-39** El correo falla y bloquea el cierre de caja.
- **EB-40** Sin correo del dueño configurado: falla en silencio.

### Camino nuevo que se salta el control viejo

- **EB-41** «Entregar propina» sin rol o sin caja abierta; entrega a una persona de otra empresa o
  inactiva.
- **EB-43** (D-B) Reparto parejo que no da un peso entero a cada persona (pendiente $2.50 entre 3):
  no se entrega nada y se avisa; nunca se entregan centavos ni se pierde el sobrante.
- **EB-44** (D-B) Monto ajustado con centavos (`$40.50`) rechazado en servidor, no solo en pantalla.
- **EB-45** (D-B) El sobrante de centavos se queda pendiente al cerrar: la pregunta del cierre no
  debe exigir decisión por un pendiente que no se puede entregar (menor a $1), pero sí heredarlo.
- **EB-46** (D-A) La misma persona dos veces en un reparto, o un reparto vacío.
- **EB-47** (D-C) Correo mal formado, lista vacía, duplicados o más de 10 direcciones.
- **EB-42** Montos absurdos (NaN, negativos, por encima de `MaxMoney`) en entrega, conteo por
  denominación, total de terminal.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Entregar propinas desde la caja (Priority: P1)

Quien opera la caja ve cuánto hay de propina pendiente de entregar, elige a quién y cuánto, y la
entrega con efectivo del cajón. Aplica igual a propinas cobradas en efectivo que a las cobradas con
tarjeta o SPEI (puntos 1 y 2).

**Why this priority**: Hoy las propinas se registran como gastos y descuadran ventas, gastos y
arqueo. Es el dinero que más confunde el corte.

**Independent Test**: Cobrar dos pedidos con propina (uno efectivo, uno tarjeta), entregar ambas, y
verificar que ventas y gastos no cambian y que el efectivo esperado bajó solo por la propina de
tarjeta.

**Acceptance Scenarios**:

1. **Given** una caja abierta con propinas pendientes, **When** se toca «Entregar propina», **Then**
   se muestra el pendiente desglosado por método y la lista de personas activas del local.
2. **Given** se entrega una propina, **Then** queda un movimiento de caja tipo propina, ligado a los
   cobros que la generaron y a quien la recibe, y el pendiente baja en ese monto.
3. **Given** se intenta entregar más que el pendiente, **Then** se rechaza con un mensaje claro.
4. **Given** una propina de tarjeta entregada en efectivo, **Then** el efectivo esperado del cajón
   baja en ese monto y el reporte la nombra «propina de tarjeta pagada en efectivo».
5. **Given** cualquier entrega, **Then** ni ventas ni gastos cambian.

---

### User Story 2 - Cierre que separa negocio y propinas (Priority: P1)

Al cerrar, si queda propina pendiente, se pregunta «¿se entrega ahora o se queda en caja?». El
arqueo separa el efectivo del negocio de las propinas por entregar, y el corte muestra propinas
cobradas por método, entregadas y pendientes (puntos 1 y 7).

**Why this priority**: Sin esto la entrega de propinas no cierra el ciclo y el faltante se esconde.

**Independent Test**: Cerrar con pendiente eligiendo cada opción y verificar el corte y el siguiente
turno.

**Acceptance Scenarios**:

1. **Given** pendiente > 0 al cerrar, **When** se elige «entregar ahora», **Then** se abre el mismo
   flujo de entrega y al terminar se continúa con el cierre.
2. **When** se elige «se queda en caja», **Then** el corte la muestra como propina por entregar,
   fuera del efectivo del negocio, y el siguiente turno de esa caja la hereda como pendiente.
3. **Given** el corte, **Then** muestra propinas cobradas por método, entregadas, pendientes y la
   propina de tarjeta pagada en efectivo, cada cifra una sola vez.

---

### User Story 3 - Salidas con concepto y corrección por reverso (Priority: P2)

Toda salida lleva un concepto de una lista (que se amplía en la misma pantalla); la salida mal
capturada se corrige con «Corregir», que crea un reverso y una salida nueva (puntos 3 y 4). El día
del gasto es el del turno abierto (punto 5).

**Independent Test**: Capturar una salida, corregirla y verificar que el corte cuadra y la original
sigue visible con su reverso.

**Acceptance Scenarios**:

1. **Given** el concepto no existe, **When** se escribe y se agrega, **Then** queda en la lista y la
   salida se guarda con él, sin salir de la pantalla.
2. **Given** una salida, **When** se toca «Corregir», **Then** se crean su reverso ligado y una
   salida nueva; la original no cambia.
3. **Given** una salida ya corregida o un reverso, **Then** «Corregir» no está disponible.
4. **Given** un turno abierto de ayer pasada la medianoche, **Then** el día del gasto es el del turno.
5. **Given** Configuración del negocio, **Then** se editan, archivan y fusionan conceptos; al
   fusionar, las salidas del duplicado pasan al concepto que queda.

---

### User Story 4 - Apertura contada a ciegas (Priority: P2)

El fondo se cuenta por denominación sin ver el cierre anterior; si difiere, se pide motivo de una
lista más texto (punto 6).

**Acceptance Scenarios**:

1. **Given** la apertura, **Then** no se muestra el monto del cierre anterior en ningún lugar.
2. **Given** lo contado difiere del cierre anterior, **Then** no se abre sin motivo.
3. **Given** el primer turno de la caja, **Then** no se pide motivo.

---

### User Story 5 - Cobro con tarjeta: terminal y tipo (Priority: P1)

Al cobrar con tarjeta se registra terminal y débito/crédito; la terminal por omisión del usuario
llega preseleccionada y el caso común cuesta un toque (punto 8).

**Acceptance Scenarios**:

1. **Given** un usuario con terminal por omisión, **When** elige tarjeta, **Then** basta tocar
   «Débito» o «Crédito».
2. **Given** su terminal por omisión archivada, **Then** se pide elegir terminal, sin error.
3. **Given** Configuración, **Then** se agregan, renombran y archivan terminales por sucursal; un
   negocio nuevo nace con una.

---

### User Story 6 - Arqueo de tarjeta por sucursal (Priority: P2)

Cada sucursal elige arqueo automático o por terminal (punto 9).

**Acceptance Scenarios**:

1. **Given** modo automático, **Then** la tarjeta se declara igual a lo esperado.
2. **Given** modo por terminal, **Then** al cerrar se escribe el total del corte de cada terminal con
   cobros y la diferencia sale en el corte.

---

### User Story 7 - Devolución con tarjeta con folio (Priority: P2)

La devolución de un cobro con tarjeta dice en qué terminal hacerla y no termina sin el folio que
imprime la terminal; se guarda quién lo capturó (punto 10).

---

### User Story 8 - Avisos y correo diario (Priority: P3)

Avisos de «salidas sin concepto» y «propinas sin entregar» en el cierre y en Ventas del día; correo
diario al dueño con el resumen del cierre; reportes de gastos sin propinas ni traspasos (punto 7).

---

### User Story 9 - «Cobrar» como acción principal al enviar a cocina (Priority: P3)

En mostrador, «Cobrar» pasa a ser la acción principal al enviar a cocina (punto 11). Sin campo de
hora.

## Requirements *(mandatory)*

### Functional Requirements

**Propinas**

- **FR-001** El sistema MUST calcular la propina pendiente de una caja como propinas cobradas
  − propinas devueltas − propinas entregadas, por método, más lo heredado del turno anterior de la
  misma caja (EB-07, EB-08, EB-12, EB-14).
- **FR-002** «Entregar propina» MUST registrar un movimiento de caja de tipo propina con: persona que
  la recibe (usuario activo de la empresa), monto, método de origen, cobros que la generaron, quién
  la entregó y el turno (EB-41).
- **FR-003** MUST rechazar una entrega mayor al pendiente (EB-06). El método de origen lo decide
  el servidor: primero lo heredado, luego lo cobrado en efectivo y después los demás métodos, en
  orden de cobro.
- **FR-003a** (D-A) Una entrega MAY repartirse entre varias personas, parejo o ajustado; queda un
  movimiento por persona. Sin personas o con una persona repetida se rechaza (EB-46).
- **FR-003b** (D-B) Toda entrega es en pesos enteros. Parejo = `floor(pendiente / n)` pesos por
  persona; el sobrante sigue pendiente. Un monto con centavos se rechaza (EB-43, EB-44).
- **FR-004** Una propina entregada MUST NOT contar en ventas ni en gastos (EB-01, EB-02).
- **FR-005** El efectivo esperado del cajón MUST bajar por propinas entregadas en efectivo,
  cualquiera que sea el método con que se cobraron, y la de efectivo MUST haber entrado una sola vez
  (EB-03, EB-04).
- **FR-006** Al cerrar con pendiente de al menos $1 MUST preguntar «¿se entrega ahora o se queda en caja?».
- **FR-007** El arqueo de cierre MUST separar efectivo del negocio y propinas por entregar (EB-13).
- **FR-008** Los gastos históricos que eran propinas (punto 1) MUST migrarse al tipo propina por una
  corrección de datos con lista aprobada, respaldo y rollback, y dejar de salir en gastos (EB-09).

**Salidas**

- **FR-009** Toda salida MUST llevar concepto, en todos los caminos de escritura (EB-18).
- **FR-010** Los conceptos MUST ser un catálogo por empresa, único sin distinguir mayúsculas ni
  espacios extremos, con alta en línea, edición, archivo y fusión; cada uno opcionalmente ligado a
  categoría de gasto y proveedor recurrente (EB-19, EB-20).
- **FR-011** «Corregir» MUST crear un reverso ligado y una salida nueva; nada se edita ni se borra;
  una salida se revierte a lo más una vez y un reverso no se revierte (EB-15, EB-16).
- **FR-012** El reverso de una salida de un turno cerrado MUST caer en el turno abierto actual y
  nombrar el turno de origen; el corte cerrado no se reescribe (EB-17).
- **FR-013** El día del gasto MUST ser el del turno abierto; sin turno, hoy y editable. La fecha del
  documento se guarda aparte y no lo mueve (EB-21, EB-22).

**Apertura**

- **FR-014** La apertura MUST contarse por denominación sin exponer el cierre anterior en la
  respuesta del servidor ni en la pantalla (EB-23).
- **FR-015** Si lo contado difiere del cierre anterior de la misma caja, MUST exigir motivo (lista +
  texto); el servidor lo valida, no solo la pantalla (EB-25, EB-26, EB-27). Se elimina la nota libre.

**Tarjeta**

- **FR-016** Las terminales MUST ser un catálogo por sucursal (agregar, renombrar, archivar); un
  negocio nuevo nace con una (EB-30).
- **FR-017** Cada usuario MAY tener terminal por omisión; si está archivada o es de otra sucursal se
  ignora (EB-28, EB-29).
- **FR-018** Todo pago con tarjeta nuevo MUST guardar terminal y débito/crédito, en todos los caminos
  de cobro del POS (EB-31). Los pagos de plataforma no llevan terminal.
- **FR-019** Cada sucursal MUST tener modo de arqueo de tarjeta: automático o por terminal; el
  cierre usa el modo vigente al abrir el turno (EB-33). El Gato Bobah arranca por terminal.
- **FR-020** En modo por terminal, MUST pedir el total de cada terminal con cobros en el turno,
  incluidas las archivadas, y mostrar la diferencia en el corte (EB-32).
- **FR-021** Devolver un cobro con tarjeta MUST mostrar la terminal y exigir folio no vacío; se
  guarda quién lo capturó. Para cobros sin terminal registrada, se pide el folio sin terminal
  (EB-34, EB-35, EB-36).

**Reportes y avisos**

- **FR-022** El corte MUST mostrar propinas cobradas por método, entregadas, pendientes y de tarjeta
  pagadas en efectivo; salidas por concepto y proveedor; y salidas sin concepto (las históricas).
- **FR-023** Avisos de salidas sin concepto y propinas sin entregar MUST aparecer en el cierre y en
  Ventas del día.
- **FR-024a** (D-C) Configuración del negocio MUST tener un campo de correos para el resumen
  diario: uno o varios, cada uno validado, sin duplicados, a lo más 10 (EB-47).
- **FR-024** Un correo diario a esas direcciones con el resumen del cierre MUST enviarse una vez por empresa y
  día de negocio, sin bloquear el cierre si falla, y dejar evento en el log si no hay destinatario
  (EB-37 a EB-40).
- **FR-025** Los reportes de gastos MUST excluir propinas y traspasos y mostrarlos aparte (EB-10).
- **FR-026** Todos los montos MUST validarse con los topes de dinero existentes (EB-42).
- **FR-027** En mostrador, «Cobrar» MUST ser la acción principal al enviar a cocina (punto 11).

### Key Entities

- **Movimiento de caja**: ahora con tipo (venta-ajeno, salida, entrada, propina, traspaso, reverso),
  concepto, persona que recibe, método de origen y movimiento revertido.
- **Concepto de salida**: catálogo por empresa con categoría y proveedor opcionales, archivable y
  fusionable.
- **Terminal**: catálogo por sucursal, archivable.
- **Ajuste de sucursal**: modo de arqueo de tarjeta.
- **Conteo de apertura**: por denominación, con motivo si difiere.
- **Total de terminal al cierre**: declarado por terminal y turno.
- **Envío de resumen diario**: uno por empresa y día.

## Success Criteria *(mandatory)*

- **SC-001** En un día con propinas por todos los métodos, ventas y gastos del corte no incluyen un
  solo peso de propina (verificable con un test que falla nombrando el concepto duplicado).
- **SC-002** El caso común de cobro con tarjeta cuesta un toque adicional a elegir el método.
- **SC-003** Entregar la propina pendiente completa a una persona cuesta a lo más 3 toques desde la
  caja.
- **SC-004** Ninguna salida nueva queda sin concepto.
- **SC-005** Corregir una salida deja el corte cuadrado y la original visible.
- **SC-006** El correo diario llega una vez por día de negocio.

## Assumptions

- «Persona que recibe» es un usuario de la empresa; no se crea un catálogo aparte de empleados.
- El reparto lo decide quien entrega (parejo o ajustado); el sistema no reparte por horas trabajadas.
- El modo de arqueo y las terminales cuelgan de la sucursal existente; con una sola sucursal todo
  funciona igual.
- El correo usa el mailer existente; el destinatario sale del campo de D-C.
- La corrección de los gastos 45 y 46 (punto 5) es corrección de datos fuera de esta feature.
- Las listas de motivos de apertura arrancan con un catálogo fijo en código (YAGNI); si se piden
  editables, se agregan después al mismo costo.
