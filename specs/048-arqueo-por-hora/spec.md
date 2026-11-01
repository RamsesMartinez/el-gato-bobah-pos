# Feature Specification: Arqueo por hora de los gastos

**Feature Branch**: `048-arqueo-por-hora`

**Created**: 2026-11-01

**Status**: Draft

**Input**: Lo que queda del issue #111 sobre cajas y cortes, encima de lo que subió la 047
(«lleva arqueo» por caja, efectivo sin turno, fechas del negocio): la casilla «Incluir en el arqueo»
por pago, cortes que juntan los gastos por hora, el aviso con Registrar / Registrar sin incluir / Cancelar, la red de seguridad al
abrir y mover la hora de apertura y la casilla con permiso y bitácora. Menos es más: el menor cambio
de pantalla posible.

**Punto de partida (047)**: un pago en efectivo entra al corte abierto de su caja si es del día del
turno; si no, solo se guarda fuera de todo corte tras confirmarlo. Lo que falta es el estado de en
medio: **un gasto que sí debe salir en un corte que todavía no existe**.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Casilla «Incluir en el arqueo» (Priority: P1)

Al capturar un pago en efectivo de una caja que lleva arqueo, quien captura ve la casilla «Incluir
en el arqueo», marcada por omisión. Desmarcarla registra el pago fuera de todo corte sin preguntar.

**Why this priority**: es la decisión explícita que hoy solo existe como respuesta a un aviso.

**Independent Test**: con la caja abierta, un gasto de hoy con la casilla desmarcada no cambia el
esperado del corte.

**Acceptance Scenarios**:

1. **Given** un pago en efectivo de una caja con arqueo, **Then** se ve la casilla, marcada.
2. **Given** tarjeta, transferencia o una caja sin arqueo, **Then** la casilla no aparece.
3. **Given** caja abierta y pago del día con la casilla desmarcada, **When** se guarda, **Then** no
   entra al corte y no aparece ningún aviso.

---

### User Story 2 - Un gasto sin corte entra al siguiente que lo cubra (Priority: P1)

Con la caja cerrada y la casilla marcada, aparece el aviso: «Este gasto no saldrá en ningún corte,
salvo que el siguiente corte se abra con una hora de apertura que lo incluya.», con **Registrar** y **Cancelar**
y, aparte, **Registrar sin incluir en el arqueo**. «Registrar» lo deja pendiente. Al abrir el
siguiente corte, la apertura lista los «gastos sin corte desde el último cierre» y ofrece, con un
toque, incluirlos abriendo desde la hora del primero; así ese corte lo descuenta del esperado.

**Why this priority**: es el caso del gasto de madrugada o del dinero que salió con la caja
cerrada: hoy queda fuera del arqueo aunque sí salió de ese cajón.

**Independent Test**: caja cerrada, gasto de $200 «Registrar»; al abrir, encender «Incluirlos (desde las
HH:MM)» y abrir; el esperado del corte es $200 menor; y al cerrarlo el corte lista el gasto.

**Acceptance Scenarios**:

1. **Given** caja con arqueo cerrada y casilla marcada, **When** se guarda, **Then** aparece el
   aviso; «Cancelar» no guarda; «Registrar sin incluir» lo deja fuera para
   siempre; «Registrar» lo deja pendiente de corte.
2. **Given** gastos pendientes de esa caja desde su último cierre, **When** se va a abrir, **Then** la
   apertura los lista con hora e importe y ofrece incluirlos abriendo desde la hora del primero.
3. **Given** el corte se abre desde esa hora, **Then** los pendientes con hora dentro del corte se
   descuentan de su esperado en ese momento, y también los que se registren pendientes después con hora
   dentro del corte se descuentan al cerrarlo.
4. **Given** un gasto pendiente con hora anterior al último cierre de su caja, **Then** no puede entrar
   a ningún corte: el aviso lo dice y solo ofrece «Registrar sin incluir en el arqueo» y «Cancelar».
5. **Given** se abre sin incluirlos (sin mover la hora, o desde una hora posterior a ellos),
   **Then** los pendientes con hora anterior a la apertura quedan «sin incluir», como si se hubiera
   elegido así al registrarlos, dejan de listarse y queda un evento en la bitácora por cada uno.

---

### User Story 3 - Mover la hora de apertura (Priority: P2)

Quien tiene permiso puede abrir un corte con una hora anterior a la actual, nunca antes del cierre
del corte anterior de esa caja. Queda en bitácora quién y desde qué hora.

**Independent Test**: un cajero sin permiso no ve «Incluirlos (desde las HH:MM)»; un gerente sí, y el
evento aparece en la bitácora.

**Acceptance Scenarios**:

1. **Given** quien no tiene el permiso, **Then** la apertura solo abre «ahora» y el servidor rechaza
   una hora anterior.
2. **Given** una hora anterior al último cierre o futura, **Then** se rechaza.
3. **Given** se abre desde una hora anterior, **Then** queda un evento de bitácora con la hora.

---

### User Story 4 - Cambiar la casilla de un gasto ya registrado (Priority: P3)

En el detalle del gasto, quien tiene permiso puede cambiar «Incluir en el arqueo» de un pago en
efectivo mientras su corte no esté cerrado. Queda en bitácora.

**Acceptance Scenarios**:

1. **Given** un pago incluido en un corte abierto, **When** se desmarca, **Then** sale del esperado
   de ese corte.
2. **Given** un pago fuera de corte de una caja abierta con hora dentro del corte, **When** se marca,
   **Then** entra al esperado.
3. **Given** un pago incluido en un corte cerrado, **Then** la casilla no se puede cambiar.
4. **Given** quien no tiene el permiso, **Then** no la puede cambiar.

---

### Edge Cases

- **Hora del pago**: la del servidor al capturar si la fecha es la del día; la medianoche de su día
  si la fecha es otra (no hay hora real de un pago capturado días después). Así un gasto de días
  pasados casi siempre queda antes del último cierre y no puede entrar a un corte, que es lo correcto.
- **Dos tabletas abren la misma caja**: una sola apertura gana (como hoy); los pendientes entran a
  ese corte una sola vez.
- **Un pendiente se registra mientras el corte está abierto con hora dentro de él**: entra en ese
  momento, como cualquier pago del día.
- **Archivar una caja con pendientes**: se permite; los pendientes quedan fuera de corte.
- **Pagos ya guardados por la 047**: los confirmados fuera de corte quedan «sin incluir»; nada se
  recalcula.
- **Tabletas con la versión anterior** durante la subida: siguen mandando la confirmación de la 047,
  que se interpreta como «sin incluir», igual que antes.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: Cada pago en efectivo de una caja con arqueo MUST guardar si se incluye en el arqueo
  (por omisión sí) y la hora del pago.
- **FR-002**: Un pago incluido entra al corte de su caja cuya ventana (apertura a cierre) contiene su
  hora; mientras no exista ese corte queda «pendiente de corte».
- **FR-003**: Al abrir y al cerrar un corte, el sistema MUST descontar de su esperado los pendientes
  de esa caja con hora dentro de la ventana, una sola vez.
- **FR-004**: Un pago incluido que no entra a un corte al guardarse MUST pedir confirmación que
  ofrece Registrar, Registrar sin incluir y Cancelar; si su hora es anterior al último cierre de su caja,
  solo Registrar sin incluir / Cancelar.
- **FR-005**: La apertura MUST listar los pendientes de esa caja desde su último cierre y ofrecer abrir
  desde la hora del primero.
- **FR-006**: Abrir con una hora anterior MUST exigir permiso, rechazar horas futuras o anteriores al
  último cierre de esa caja, y dejar evento de bitácora.
- **FR-007**: Cambiar la casilla de un pago MUST exigir permiso, rechazarse si su corte está cerrado,
  ajustar el esperado del corte abierto y dejar evento de bitácora.
- **FR-008**: Los controles nuevos preguntan por permiso, no por rol.

### Key Entities

- **Pago de gasto**: gana «se incluye en el arqueo» y la hora del pago.

## Success Criteria *(mandatory)*

- **SC-001**: Un gasto pagado con la caja cerrada sale en el esperado del siguiente corte con un solo
  toque al abrir.
- **SC-002**: Ningún gasto se descuenta dos veces ni de un corte cuya ventana no contiene su hora.
- **SC-003**: Ningún corte cerrado cambia.
- **SC-004**: El caso normal (caja abierta, pago de hoy) sigue sin pasos extra: la casilla ya está
  marcada.

## Assumptions

- El permiso nuevo («ajustar el arqueo») lo tienen admin y gerente, como configurar caja.
- La hora de apertura se mueve al abrir; mover la de un corte ya abierto queda fuera.

## Fuera de alcance

- Renglones excluidos de una compra, «importe que salió de la caja» y «habilita ventas» por caja: van
  en la spec 049.
