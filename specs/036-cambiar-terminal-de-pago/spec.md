# Feature Specification: Cambiar la terminal de un pago ya cobrado

**Feature Branch**: `036-cambiar-terminal-de-pago`

**Created**: 2026-10-11

**Status**: Draft

**Input**: «Admin y gerente pueden cambiar la terminal de tarjeta de un pago ya cobrado, con motivo obligatorio, y cada cambio queda en la bitácora de auditoría. El detalle de venta muestra la terminal usada en cada pago. Al cobrar con tarjeta ya no hay terminal predeterminada: quien cobra la elige siempre.»

## Contexto

Un débito se cobró en una terminal y quedó registrado en otra. El arqueo por terminal del turno no habría cuadrado en ninguna de las dos, y la única forma de corregirlo fue editar la base a mano, sin rastro dentro del sistema. El error nace al cobrar: la terminal llegaba puesta (la última que usó esa persona) y se cobró sin mirarla.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Corregir la terminal de un pago (Priority: P1)

Un gerente revisa el turno, ve que un pago con tarjeta quedó en la terminal equivocada, abre el detalle de esa venta, toca «Cambiar terminal» en ese pago, elige la terminal correcta, escribe el motivo y confirma. Los totales por terminal del turno se mueven con el pago.

**Why this priority**: es el caso que hoy obliga a tocar la base a mano y deja el arqueo por terminal sin cuadrar.

**Independent Test**: cobrar un pedido con tarjeta en la terminal A, cambiar el pago a la terminal B como gerente y comprobar que el total de A baja, el de B sube por el mismo monto y que el pago muestra B.

**Acceptance Scenarios**:

1. **Given** un pago con tarjeta con tarjeta en Terminal A en un turno abierto, **When** un gerente lo cambia a Terminal B con el motivo «se cobró en Terminal B», **Then** el pago muestra Terminal B, el total esperado de Terminal A baja $250 y el de Terminal B sube $250.
2. **Given** el mismo pago, **When** el gerente intenta confirmar sin motivo, **Then** el sistema no aplica el cambio y pide el motivo.
3. **Given** un cajero o un mesero, **When** abre el detalle de la venta, **Then** no ve la acción «Cambiar terminal», y si la pide de otra forma el sistema la rechaza.
4. **Given** un pago en efectivo, transferencia o de plataforma, **When** un gerente abre el detalle, **Then** ese pago no ofrece cambiar terminal.

---

### User Story 2 - Cada cambio queda en la bitácora (Priority: P1)

Quien audita el turno ve en la bitácora de auditoría quién cambió la terminal de qué pago, de cuál a cuál, cuándo, desde qué aparato y por qué.

**Why this priority**: mover dinero entre terminales sin rastro es justo lo que la bitácora existe para impedir; sin esto la corrección es igual de opaca que editar la base.

**Independent Test**: hacer un cambio y buscarlo en la bitácora con el autor, el pago, la terminal anterior, la nueva y el motivo.

**Acceptance Scenarios**:

1. **Given** un cambio de terminal aplicado, **When** se consulta la bitácora, **Then** aparece un evento con autor, pedido, pago, monto, terminal anterior, terminal nueva y motivo.
2. **Given** que el evento de bitácora no se puede guardar, **When** se intenta el cambio, **Then** el cambio no se aplica.

---

### User Story 3 - El detalle de venta muestra la terminal (Priority: P2)

Al abrir una venta en Ventas, cada pago con tarjeta dice en qué terminal se cobró.

**Why this priority**: sin verlo, nadie detecta el error que la historia 1 corrige.

**Independent Test**: abrir una venta cobrada con tarjeta y leer la terminal en el renglón del pago.

**Acceptance Scenarios**:

1. **Given** una venta con dos pagos con tarjeta en terminales distintas, **When** se abre su detalle, **Then** cada pago muestra su terminal.
2. **Given** un pago con tarjeta anterior a que existieran las terminales (sin terminal), **When** se abre el detalle, **Then** el pago dice «Sin terminal» y un admin o gerente puede asignársela con el mismo flujo.

---

### User Story 4 - Sin terminal predeterminada al cobrar (Priority: P2)

Al cobrar con tarjeta, el selector de terminal llega vacío y el cobro no se puede confirmar hasta elegir una.

**Why this priority**: ataca la causa del error en vez de solo la corrección.

**Independent Test**: cobrar dos veces seguidas con tarjeta en la misma tableta y comprobar que la segunda vez la terminal no llega elegida.

**Acceptance Scenarios**:

1. **Given** una persona que ya cobró con Terminal B, **When** abre otro cobro con tarjeta, **Then** ninguna terminal viene elegida.
2. **Given** un cobro con tarjeta sin terminal elegida, **When** se intenta confirmar, **Then** el sistema no cobra y señala el selector.
3. **Given** que la empresa tiene una sola terminal activa, **When** se cobra con tarjeta, **Then** esa terminal sí viene elegida (no hay nada que decidir).

### Edge Cases

- **Turno ya cerrado con arqueo por terminal capturado**: el cambio se permite; el arqueo capturado no se reescribe (es lo que se contó), pero el corte de ese turno muestra que hubo un cambio posterior y la diferencia recalculada con la terminal nueva.
- **Terminal destino inactiva o de otra empresa**: se rechaza.
- **Misma terminal que la actual**: se rechaza sin escribir bitácora.
- **Pago devuelto**: ya no es un pago; no se puede cambiar.
- **Venta cancelada**: sus pagos tampoco se pueden cambiar.
- **Dos personas cambian el mismo pago a la vez**: se aplica uno y el otro recibe un aviso de que el pago ya cambió; nunca queda una bitácora que no coincida con el estado final.
- **Motivo solo con espacios o muy largo**: se rechaza el vacío; se acota la longitud.
- **El nombre de la terminal cambió después del cobro**: el pago muestra la terminal con la que quedó, y tras el cambio, el nombre vigente de la nueva.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: El sistema MUST permitir cambiar la terminal de un pago con tarjeta ya cobrado a otra terminal activa de la misma empresa.
- **FR-002**: Solo quien tenga el permiso de corregir pagos (hoy admin y gerente) MUST poder hacerlo; el servidor rechaza a cualquier otro aunque la pantalla no le muestre la acción.
- **FR-003**: El cambio MUST exigir un motivo no vacío, de 3 a 200 caracteres.
- **FR-004**: Cada cambio MUST registrar en la bitácora de auditoría autor, pedido, pago, monto, terminal anterior, terminal nueva, motivo y origen del request, en la misma operación: si la bitácora falla, el cambio no se aplica.
- **FR-005**: Los totales por terminal del turno del pago (esperado del arqueo y reportes) MUST reflejar la terminal nueva.
- **FR-006**: El arqueo ya capturado de un turno cerrado MUST conservarse; el corte de ese turno MUST indicar los cambios posteriores y la diferencia recalculada.
- **FR-007**: El detalle de venta MUST mostrar la terminal de cada pago con tarjeta, o «Sin terminal».
- **FR-008**: Al cobrar con tarjeta, la terminal MUST llegar sin elegir cuando la empresa tiene más de una terminal activa, y el cobro MUST rechazarse sin terminal.
- **FR-009**: El cambio MUST rechazarse si el pago no es con tarjeta, fue devuelto, la venta está cancelada, o la terminal destino es la misma, está inactiva o no es de la empresa.
- **FR-010**: El cambio MUST ser seguro ante dos cambios simultáneos del mismo pago.

### Key Entities

- **Pago**: monto, método, terminal con que se cobró (y su nombre en ese momento), turno y venta.
- **Terminal**: terminal de tarjeta de la empresa, activa o no.
- **Evento de bitácora**: quién, cuándo, desde dónde, qué cambió (antes y después) y por qué.
- **Arqueo por terminal**: lo esperado y lo declarado por terminal al cerrar un turno.

## Success Criteria *(mandatory)*

- **SC-001**: Un gerente corrige la terminal de un pago en menos de 30 segundos desde el detalle de la venta, sin tocar la base.
- **SC-002**: El 100% de los cambios de terminal tienen su evento en la bitácora con motivo; cero cambios sin evento.
- **SC-003**: Tras corregir, la suma por terminal del turno coincide con lo que cada terminal reporta.
- **SC-004**: Ningún cobro con tarjeta se registra sin terminal cuando hay más de una activa.

## Assumptions

- La bitácora de auditoría que hoy está en `develop` sube a producción antes o junto con esta función.
- «Admin y gerente» se resuelve por permiso, no por nombre de rol (constitución, principio VIII).
- Solo se cambia la terminal; el monto y el método no se tocan (cambiar método es otra corrección, fuera de alcance).
- Se permite corregir pagos de cualquier fecha; el límite lo da el permiso y la bitácora, no la antigüedad.
- La terminal recordada por persona deja de usarse para preseleccionar; si algo más la usa, se conserva para eso.
