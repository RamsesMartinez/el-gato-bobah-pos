# Feature Specification: Gastos en efectivo sin turno abierto

**Feature Branch**: `047-gastos-sin-turno`

**Created**: 2026-10-31

**Status**: Draft

**Input**: Issue #111 (gastos con cajas configurables, «incluir en el arqueo», registrar gasto sin
turno abierto) y tres hallazgos de producción del 2026-10-31, durante el cierre de octubre:

1. Efectivo pagado por un tercero (la mamá de Abi) con la caja principal ya cerrada y «Caja externa»
   sin turno nunca abierto: el formulario dice «Abre una caja para pagar en efectivo» y no deja
   guardar.
2. En «Nuevo gasto», «Fecha del pago» y «Día del gasto» salen con la fecha UTC: a las 23:30 del
   31-oct (hora del negocio) proponen el 1-nov. Por eso la freidora (gasto 82) quedó pagada el 1-nov.
3. Hay que poder capturar gastos pasados sin tocar cortes cerrados y sin pedir caja abierta.

**Alcance decidido** (2026-10-31, con urgencia del cierre de octubre y «menos es más»): este spec
cubre los tres puntos con el menor cambio de pantalla posible. Del issue #111 entra solo lo que esos
puntos necesitan: el ajuste «lleva arqueo» por caja y que registrar un gasto ya no dependa de un turno
abierto. Lo demás del issue sigue abierto en #111 (ver *Fuera de alcance*). Lo evaluó un revisor con
puntajes: esta opción 8.5/10 contra 5.5 (agregar cortes por hora) y 4 (todo el issue).

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Las fechas del gasto son las del negocio (Priority: P1)

Quien captura un gasto de noche ve como fecha propuesta el día del negocio, no el de otro huso.

**Why this priority**: es un defecto que hoy ya puso un gasto en el mes equivocado; el cierre de
octubre depende de que cada gasto caiga en su día.

**Independent Test**: con el reloj en 31-oct 23:30 hora del negocio, abrir «Nuevo gasto» y ver
31-oct en la fecha del ticket, la del día del gasto y la del pago.

**Acceptance Scenarios**:

1. **Given** son las 23:30 del 31-oct en la zona del negocio (05:30 del 1-nov UTC), **When** se abre
   «Nuevo gasto», **Then** todas las fechas propuestas dicen 31-oct.
2. **Given** el mismo momento, **When** se agrega un pago o se usa el pago rápido, **Then** su fecha
   propuesta es 31-oct.
3. **Given** son las 00:30 del 1-nov en la zona del negocio, **When** se abre «Nuevo gasto»,
   **Then** las fechas dicen 1-nov.

---

### User Story 2 - Efectivo de una caja que no lleva arqueo (Priority: P1)

Alguien de fuera del negocio paga en efectivo una compra. Quien captura elige la caja que representa
ese dinero («Caja externa»), que está configurada como «no lleva arqueo», y guarda sin abrir ningún
turno. Ningún corte ni ninguna caja cambia.

**Why this priority**: es el caso que hoy no se puede capturar y bloquea el cierre de octubre.

**Independent Test**: con todas las cajas cerradas, capturar un gasto en efectivo en «Caja externa»
(sin arqueo); se guarda, y el esperado de todos los cortes, abiertos y cerrados, queda igual.

**Acceptance Scenarios**:

1. **Given** una caja configurada «no lleva arqueo» y sin turno, **When** se captura un gasto en
   efectivo en ella, **Then** se guarda sin pedir turno ni mostrar avisos, y queda registrado en qué
   caja se pagó.
2. **Given** ese gasto, **When** se consulta cualquier corte, **Then** no aparece en su efectivo
   esperado ni en su lista de gastos.
3. **Given** quien administra, **When** abre los ajustes de una caja, **Then** puede marcar o
   desmarcar «Lleva arqueo»; las cajas existentes empiezan con «Lleva arqueo» marcado.
4. **Given** una caja «no lleva arqueo», **When** se intenta abrirle un turno, **Then** se rechaza
   con un mensaje claro: esa caja no se cuenta.

---

### User Story 3 - Efectivo de una caja con arqueo cerrada, o con fecha pasada (Priority: P1)

Quien captura un gasto en efectivo de una caja que sí lleva arqueo, pero que no tiene turno abierto
(o el pago es de un día anterior), puede registrarlo fuera de cualquier corte. Antes de guardar, una
ventana le avisa que ese gasto no saldrá en ningún corte, y elige registrarlo así o cancelar.

**Why this priority**: es la captura de gastos pasados del cierre de octubre; sin esto, cada gasto
viejo obliga a abrir un turno que descuadra la caja de hoy.

**Independent Test**: con la caja principal cerrada, capturar un gasto en efectivo con fecha del
15-oct en la caja principal; aparece el aviso, se confirma «Registrar sin incluir en el arqueo», y
ningún corte cambia.

**Acceptance Scenarios**:

1. **Given** una caja con arqueo y sin turno abierto, **When** se guarda un gasto en efectivo en
   ella, **Then** aparece una ventana que nombra la caja («Caja principal no tiene turno abierto»,
   o «El pago es de otro día» si la fecha no es la del turno) y dice «Este gasto no entrará a ningún
   corte.», con «Registrar sin incluir en el arqueo» y «Cancelar».
2. **Given** esa ventana, **When** se elige «Registrar sin incluir en el arqueo», **Then** el gasto
   queda registrado con su caja y fuera de todo corte; **When** se elige «Cancelar», **Then** no se
   guarda nada y el formulario sigue como estaba.
3. **Given** una caja con arqueo y con turno abierto, **When** se guarda un gasto en efectivo con
   fecha de pago igual al día del turno, **Then** se comporta como hoy: entra al corte abierto, sin ventana.
4. **Given** una caja con turno abierto, **When** el pago en efectivo tiene una fecha distinta del día del turno,
   **Then** aparece la misma ventana y el gasto no entra al corte abierto: ese efectivo no salió del
   cajón de hoy.
5. **Given** un corte ya cerrado que cubre la fecha del pago, **When** se registra el gasto pasado,
   **Then** el corte cerrado no cambia en nada.

---

### Edge Cases

- **El turno se cierra mientras se captura**: la pantalla mostraba la caja abierta y al guardar ya
  está cerrada. El servidor responde que el gasto no entraría a ningún corte y la pantalla muestra la
  misma ventana; nunca se guarda en silencio fuera del corte.
- **Fecha de pago futura**: se rechaza, igual que hoy.
- **Pago con tarjeta o transferencia**: no cambia; la caja sigue siendo opcional y no hay ventana.
- **Una caja pasa a «no lleva arqueo» con un turno abierto**: se rechaza el cambio hasta cerrar el
  turno, para no dejar un corte a medias.
- **La caja principal** (la que vende) no se puede marcar «no lleva arqueo»: sus ventas en efectivo
  necesitan corte.
- **Agregar un pago a un gasto existente**: las mismas reglas aplican; un pago ya ligado a un corte
  cerrado sigue sin poder moverse.
- **Gastos ya registrados**: no se recalculan; el gasto 82 se corrige a mano editando su fecha.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: Toda fecha que el formulario de gasto propone (ticket, día del gasto, pago, pago rápido,
  recepción) MUST calcularse en la zona horaria configurada del negocio, no en UTC.
- **FR-002**: Cada caja MUST tener el ajuste «Lleva arqueo» (por omisión sí), editable por quien
  administra las cajas. La caja principal MUST llevar arqueo.
- **FR-003**: Una caja sin arqueo MUST NOT poder abrir turno, y MUST NOT poder perder el arqueo
  mientras tiene un turno abierto.
- **FR-004**: Un pago de gasto en efectivo MUST seguir exigiendo una caja, pero MUST NOT exigir que
  esa caja tenga turno abierto.
- **FR-005**: Un pago en efectivo entra a un corte solo si su caja lleva arqueo, tiene turno abierto y
  la fecha del pago es el día de negocio de ese turno (un turno que cruza la medianoche sigue siendo
  de su día). En cualquier otro caso queda fuera de todo corte y
  guarda en qué caja se pagó.
- **FR-006**: Cuando un pago en efectivo de una caja con arqueo quedaría fuera de todo corte, el
  sistema MUST pedir confirmación explícita antes de guardarlo; sin ella, el servidor MUST rechazarlo
  con un error que la pantalla convierte en la ventana de aviso.
- **FR-007**: Un gasto registrado fuera de corte MUST NOT modificar ningún corte, abierto o cerrado,
  ni su efectivo esperado.
- **FR-008**: El selector de caja del pago MUST mostrar todas las cajas activas de la sucursal, no
  solo las que tienen turno abierto.
- **FR-009**: Los mensajes MUST ser para quien opera y sin términos técnicos.
- **FR-010**: Cambiar «Lleva arqueo» MUST exigir el permiso de configurar caja (el mismo del fondo
  fijo), no un nombre de rol.

### Key Entities

- **Caja**: gana el ajuste «Lleva arqueo».
- **Pago de gasto**: guarda la caja de donde salió el dinero aunque no pertenezca a ningún corte.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Un gasto en efectivo pagado por un tercero se captura con todas las cajas cerradas sin
  abrir ningún turno.
- **SC-002**: Entre las 18:00 y las 23:59 del negocio, el 100 % de las fechas propuestas en «Nuevo
  gasto» son del día del negocio.
- **SC-003**: Capturar un gasto de cualquier día de octubre deja sin cambio el esperado de todos los
  cortes existentes.
- **SC-004**: La captura de un gasto en el caso normal (caja abierta, pago con la fecha del día del turno) no agrega ningún
  paso ni ninguna ventana.

## Assumptions

- La zona del negocio es la configurada por empresa (hoy America/Mexico_City para El Gato Bobah).
- «Caja externa» ya existe en producción; el dueño la marca «no lleva arqueo» desde sus ajustes
  después de la subida.
- Quien ya puede registrar gastos puede registrarlos fuera de corte; no se agrega permiso nuevo.

## Fuera de alcance (sigue en el issue #111)

- «Habilita ventas» configurable por caja; todas las cajas iguales para quien opera.
- Casilla «Incluir en el arqueo» por pago y cortes que juntan gastos por hora al abrir y cerrar.
- Red de seguridad al abrir un corte («gastos sin corte desde el último cierre») y mover la hora de
  apertura hacia atrás.
- Renglones excluidos en una compra e «importe que salió de la caja».
