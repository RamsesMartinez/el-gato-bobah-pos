# Feature Specification: Alerta continua de pedido entrante y sección «Por aceptar»

**Feature Branch**: `093-alerta-de-pedido-entrante`

**Created**: 2026-11-23

**Status**: Draft

**Input**: Issue #327. Alerta continua de pedido entrante en cualquier pantalla y sección «Por
aceptar» en Pedidos, como pieza común para cualquier origen de pedidos que haya que aceptar: hoy los
pedidos de plataforma (spec 021), después el pedido del menú público (#328) y otras plataformas.

## Contexto: lo que ya existe

La spec 021 dejó los pedidos de plataforma por aceptar: el servidor los guarda como pendientes,
avisa a las tabletas por el canal de eventos y los lista; el POS (solo la pantalla Vender) pinta una
franja con el más urgente y un botón Aceptar, y una hoja con la lista. Lo que falta, y es lo que
pide esta feature:

- **No suena.** La franja solo se ve, y solo en Vender: en Pedidos, Caja o Almacén un pedido entra
  sin que nadie se entere.
- **No se puede rechazar desde el POS.** El servidor sabe decirle a la plataforma «no», pero no hay
  forma de pedírselo desde la pantalla.
- **No hay permiso.** Cualquier sesión ve y acepta; no se puede quitar a un rol.
- **Está atado a la plataforma.** El pedido web (#328) necesita la misma alerta y la misma sección,
  y hoy tendría que copiarlas.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Un pedido nuevo suena y se ve en cualquier pantalla (Priority: P1)

Quien atiende el mostrador está en cualquier pantalla del POS (Vender, Pedidos, Caja, Almacén,
Gastos…). Entra un pedido que hay que aceptar. La tableta empieza a sonar y aparece un indicador con
el número de pedidos por aceptar. Sigue sonando, sin tiempo límite, hasta que alguien lo acepte o lo
rechace, o hasta que el origen lo cancele. Tocar el indicador lleva a la sección «Por aceptar».

**Why this priority**: es el valor entero de la feature. Un pedido que nadie oye es un cliente que
espera y, en plataforma, un pedido que se cancela solo y una mala reseña.

**Independent Test**: con un pedido de plataforma pendiente creado en la base local, la tableta en
la pantalla de Almacén suena y muestra «1 por aceptar»; al aceptarlo, deja de sonar.

**Acceptance Scenarios**:

1. **Given** una sesión abierta en cualquier pantalla y el sonido desbloqueado, **When** entra un
   pedido por aceptar, **Then** en menos de 5 s suena de forma continua y se ve el indicador con el
   número de pendientes.
2. **Given** dos pedidos pendientes sonando, **When** se acepta uno, **Then** sigue sonando y el
   indicador baja a 1; **When** se decide el segundo, **Then** deja de sonar y el indicador desaparece.
3. **Given** el indicador visible, **When** se toca, **Then** abre Pedidos con la sección «Por
   aceptar» a la vista.
4. **Given** una hoja o diálogo abierto (cobro, modificadores, ticket), **When** entra un pedido,
   **Then** el indicador queda visible encima y el sonido empieza igual.

---

### User Story 2 - Aceptar o rechazar desde «Por aceptar» (Priority: P1)

En Pedidos, la sección «Por aceptar» muestra cada pedido pendiente: de dónde viene (la plataforma o
la web), su folio, el nombre del cliente y los datos de contacto que el origen dé, si es para llevar
o a domicilio, qué pidieron con sus opciones, el total y, si el origen tiene plazo, cuánto queda. Cada
pedido tiene Aceptar y Rechazar grandes. Rechazar pide elegir un motivo de una lista corta.

**Why this priority**: sin una forma cómoda de decidir, la alerta solo genera ruido; rechazar es la
otra mitad que hoy no existe en pantalla.

**Independent Test**: con dos pedidos pendientes, aceptar uno lo manda a cocina con su comanda y lo
quita de la sección; rechazar el otro con el motivo «Sin existencia de un producto» lo quita de la
sección y queda registrado como rechazado con ese motivo.

**Acceptance Scenarios**:

1. **Given** un pedido pendiente, **When** se toca Aceptar, **Then** el origen confirma, el pedido
   pasa a cocina como cualquier otro, sale su comanda en la tableta que aceptó y desaparece de la
   sección en todas las tabletas.
2. **Given** un pedido pendiente, **When** se toca Rechazar, **Then** se pide un motivo; **When** se
   elige y se confirma, **Then** el origen recibe el rechazo con ese motivo, el pedido desaparece de
   la sección y no entra a cocina ni al corte.
3. **Given** que el origen no confirma la aceptación o el rechazo, **When** se toca el botón,
   **Then** el pedido sigue pendiente y sonando, y se dice en palabras del operador qué hacer.
4. **Given** un renglón sin pareja en el catálogo, **When** se ve en la sección, **Then** se señala
   sin impedir aceptar (el cliente ya pagó en la plataforma).
5. **Given** ningún pedido pendiente, **When** se abre Pedidos, **Then** la sección no ocupa alto.

---

### User Story 3 - La alerta sobrevive a recargas, red caída y tableta dormida (Priority: P1)

La alerta no depende de que la tableta haya escuchado el aviso en el momento exacto. Si se recarga
la página, se cae el wifi, la tableta se suspende o la API cambia de copia durante una subida, al
volver la tableta pregunta al servidor qué está pendiente y vuelve a sonar.

**Why this priority**: es el criterio del issue («recargar o perder la red no apaga la alerta») y el
fallo más caro: un pedido que nadie oye y que nadie sabe que se perdió.

**Independent Test**: con la red de la tableta cortada, se crea un pedido pendiente; al reconectar,
en menos de 5 s suena.

**Acceptance Scenarios**:

1. **Given** un pedido pendiente sonando, **When** se recarga la página, **Then** al terminar de
   cargar con la sesión abierta vuelve a sonar (o pide desbloquear el sonido) y el indicador
   muestra el pendiente.
2. **Given** la red caída, **When** entra un pedido, **Then** al volver la red suena en menos de 5 s.
3. **Given** un pedido pendiente sonando y la red caída, **When** la red sigue caída, **Then** la
   alerta sigue sonando: perder la red no la apaga.
4. **Given** la tableta suspendida con un pedido entrando, **When** se despierta, **Then** en menos
   de 5 s suena.
5. **Given** un pedido que se aceptó en otra tableta mientras ésta estaba sin red, **When** vuelve la
   red, **Then** deja de sonar.

---

### User Story 4 - Varias tabletas (Priority: P1)

Con dos o más tabletas con sesión abierta, un pedido suena en todas. Al aceptarlo o rechazarlo en
una, deja de sonar en las demás. Si dos tocan al mismo tiempo, solo una gana; la otra lo ve
resuelto sin mensaje de error alarmante.

**Why this priority**: el local opera con más de una tableta; si aceptar en una no apaga la otra, el
ruido sigue y alguien intenta aceptar un pedido ya aceptado.

**Independent Test**: dos navegadores con sesión abierta; aceptar en uno apaga el sonido del otro en
menos de 5 s.

**Acceptance Scenarios**:

1. **Given** dos tabletas sonando por el mismo pedido, **When** una lo acepta, **Then** la otra deja
   de sonar en menos de 5 s y el pedido sale de su sección.
2. **Given** dos tabletas que tocan Aceptar al mismo tiempo, **When** el servidor resuelve, **Then**
   el pedido se acepta una sola vez (una sola comanda, un solo cobro) y la tableta que perdió dice
   «Otra tableta ya lo atendió».

---

### User Story 5 - El navegador bloquea el sonido (Priority: P2)

Los navegadores no dejan sonar una página hasta que la persona la toca. Al abrir o recargar el POS,
la primera interacción de la sesión desbloquea el sonido. Mientras el sonido esté bloqueado y haya
pedidos pendientes, se ve un aviso claro que dice que toque para activar el sonido.

**Why this priority**: sin esto, una recarga en hora pico deja la tableta muda sin que nadie lo
note; pero el indicador visual sigue funcionando, así que no es P1.

**Independent Test**: abrir el POS sin tocar nada, crear un pedido pendiente: se ve el aviso de
activar sonido; al tocarlo, suena.

**Acceptance Scenarios**:

1. **Given** el POS recién cargado sin ninguna interacción, **When** entra un pedido, **Then** se ve
   el indicador y un aviso «Toca para activar el sonido».
2. **Given** ese aviso, **When** se toca cualquier parte, **Then** empieza a sonar y el aviso se va.
3. **Given** que la persona ya tocó la pantalla en esta carga, **When** entra un pedido, **Then**
   suena sin pedir nada.

---

### User Story 6 - Quién recibe y decide (Priority: P2)

Aceptar y rechazar pedidos entrantes es un permiso propio, con nombre estable. Lo tienen por
herencia todos los roles sugeridos, así que por omisión todos pueden. Una empresa puede quitárselo a
un rol; a quien no lo tiene no le suena la alerta, no ve el indicador ni la sección, y el servidor
le niega aceptar, rechazar y listar.

**Why this priority**: el issue lo pide y es la barrera real, pero el comportamiento por omisión no
cambia para nadie.

**Independent Test**: quitar el permiso a «Mesero»; con un pedido pendiente, la sesión de un mesero
no suena ni ve el indicador, y aceptar por la API responde 403.

**Acceptance Scenarios**:

1. **Given** un rol sugerido sin cambios, **When** entra un pedido, **Then** a sus usuarios les suena.
2. **Given** un rol al que la empresa le quitó el permiso, **When** entra un pedido, **Then** a sus
   usuarios no les suena, no ven el indicador ni la sección, y el servidor rechaza sus intentos.
3. **Given** un rol propio de la empresa que ya existía antes de esta feature, **When** se sube,
   **Then** conserva la capacidad que tenía (ver y aceptar pedidos de plataforma).

### User Story 7 - Ticket a la mano y cerrados aparte (Priority: P3)

En Pedidos, quien atiende imprime el ticket de un pedido en curso con un toque, y consulta los ya
entregados del día en su propia pestaña, sin que esa lista empuje el tablero hacia abajo.

**Why this priority**: lo pidió el dueño al ver la maqueta; es de uso diario pero no depende de la
alerta.

**Independent Test**: en Pedidos, tocar la impresora de una tarjeta abre su ticket; tocar «Cerrados»
muestra todos los entregados de hoy con su botón de ticket.

**Acceptance Scenarios**:

1. **Given** un pedido en curso, **When** se toca la impresora junto a su total, **Then** se abre su
   ticket, listo para imprimir.
2. **Given** 10 pedidos entregados hoy, **When** se abre «Cerrados», **Then** se ven los 10, cada uno
   con su ticket y, si hay qué devolver y el permiso, Devolver.
3. **Given** la pestaña «En curso», **When** se compara con la pantalla actual, **Then** el tablero
   muestra el mismo número de tarjetas a la vista.

### Edge Cases

- **Sonido bloqueado por el navegador**: el indicador visual funciona igual; se ve el aviso para
  activar sonido; la primera interacción lo desbloquea (US5).
- **Recarga**: lo pendiente se vuelve a pedir al cargar; la alerta se reanuda (US3-1).
- **Red caída y reconectada**: la alerta ya sonando no se apaga sin red; al reconectar se vuelve a
  pedir lo pendiente y la alerta refleja lo que dice el servidor (US3-2, 3, 5).
- **Tableta suspendida**: al volver a primer plano se vuelve a pedir lo pendiente (US3-4).
- **Subida con cambio de copia de la API**: equivale a una reconexión; el aviso emitido por la copia
  vieja no se pierde porque al reconectar se pregunta.
- **Varias tabletas**: suena en todas; decidir en una apaga las demás; dos decisiones simultáneas
  resuelven una sola vez (US4).
- **El origen no confirma** (plataforma caída o sin credenciales): el pedido sigue pendiente y sigue
  sonando; el mensaje dice qué hacer (resolverlo desde la aplicación de la plataforma).
- **El origen cancela el pedido o vence su plazo**: deja de estar pendiente en el servidor y la
  alerta se apaga en todas las tabletas. La alerta no tiene plazo propio; solo termina cuando el
  pedido deja de estar pendiente.
- **Pantalla de bloqueo por PIN**: la sesión sigue abierta, así que la alerta sigue sonando; decidir
  exige desbloquear.
- **Sin sesión (pantalla de inicio de sesión)**: no suena.
- **Muchos pendientes a la vez** (hora pico, 3 o más): un solo sonido, no uno por pedido; el
  indicador muestra el número; la sección los ordena del que vence primero al último.
- **Pedido sin datos de cliente** (la plataforma no los da): se muestra el folio, nunca un hueco.
- **Usuario sin el permiso**: no suena ni ve nada aunque otra tableta de la misma empresa sí.
- **Rechazar sin elegir motivo**: no se puede confirmar; el motivo es obligatorio.

## Requirements *(mandatory)*

### Functional Requirements

**Módulo común de pedidos entrantes**

- **FR-001**: El sistema MUST tratar «pedido entrante por aceptar» como un concepto común con un
  origen (hoy: plataforma de reparto; después: menú público y otras plataformas), de modo que
  agregar un origen nuevo no exija cambiar la alerta, el indicador ni la sección «Por aceptar».
- **FR-002**: El servidor MUST ser el único que decide qué está pendiente. La pantalla solo pinta y
  suena lo que el servidor dice que está pendiente.
- **FR-003**: Cada pedido entrante MUST exponer, en un formato igual para todos los orígenes: origen
  y su nombre visible, folio que ve el cliente, nombre y datos de contacto del cliente cuando el
  origen los da, para llevar o a domicilio, renglones con sus opciones y si tienen pareja en el
  catálogo, total, cuándo llegó y, si el origen lo tiene, el plazo para decidir.
- **FR-004**: El sistema MUST permitir aceptar un pedido entrante: el origen confirma primero y solo
  entonces el pedido entra a cocina y al corte, como ya hace la spec 021 para plataformas.
- **FR-005**: El sistema MUST permitir rechazar un pedido entrante con un motivo elegido de una lista
  cerrada, expresada en palabras de quien opera, que corresponde a un motivo que el origen acepta. Si
  el origen no confirma el rechazo, el pedido sigue pendiente.
- **FR-006**: Aceptar y rechazar MUST resolverse una sola vez aunque dos tabletas lo intenten a la
  vez; quien pierde recibe un resultado que la pantalla presenta como «otra tableta ya lo atendió».
- **FR-007**: Todo cambio de un pedido entrante (llegó, se aceptó, se rechazó, el origen lo canceló o
  venció) MUST avisarse en vivo a las tabletas de la empresa por el canal de eventos existente.
- **FR-008**: Aceptar y rechazar MUST dejar rastro de quién decidió, cuándo y, al rechazar, el motivo.

**Alerta**

- **FR-009**: Con sesión abierta y el permiso, mientras haya al menos un pedido pendiente, la tableta
  MUST sonar de forma continua, en cualquier pantalla, incluida la de bloqueo por PIN.
- **FR-010**: La alerta MUST NOT tener tiempo límite propio: termina solo cuando el servidor deja de
  considerar pendiente al último pedido.
- **FR-011**: Varios pendientes MUST sonar como una sola alerta, no superpuestas.
- **FR-012**: La primera interacción de la persona con la página MUST desbloquear el sonido. Mientras
  esté bloqueado y haya pendientes, MUST verse un aviso para activarlo con un toque.
- **FR-013**: El tono MUST poder cambiarse después sin tocar la lógica de la alerta (hoy basta uno).

**Indicador y sección**

- **FR-014**: En todas las pantallas de la sesión MUST verse un indicador con el número de pedidos
  por aceptar mientras haya al menos uno, por encima de hojas y diálogos, que al tocarse lleva a la
  sección «Por aceptar» de Pedidos.
- **FR-015**: Pedidos MUST tener una columna «Por aceptar» a la izquierda del tablero, solo cuando
  hay pendientes, con cada pedido y sus botones Aceptar y Rechazar de al menos 44 px, separados lo
  suficiente para no tocar uno por otro.
- **FR-016**: La columna MUST ordenar los pedidos por urgencia (el plazo más cercano primero; los
  que no tienen plazo, por antigüedad).
- **FR-017**: Al aceptar, la comanda MUST imprimirse solo en la tableta que aceptó, con el ajuste de
  impresión del negocio.
- **FR-027**: Cada pedido entrante MUST mostrar su origen de forma reconocible de un vistazo (un
  ícono propio por origen junto al nombre del cliente), y, si el origen no tiene plazo, hace cuánto
  llegó en lugar de cuánto queda.
- **FR-028**: Con varios pendientes, el más urgente MUST verse completo y los demás plegados con su
  total, su tiempo y sus botones, abriéndose con un toque.
- **FR-029** (pedido por el dueño al ver la maqueta): Pedidos MUST separar en pestañas «En curso» y
  «Cerrados» (los entregados del día, completos), sin quitarle alto al tablero.
- **FR-030** (ídem): Cada tarjeta de pedido en curso MUST ofrecer su ticket a un toque, sin pasar por
  el menú de la tarjeta.
- **FR-018**: El indicador y la columna MUST sustituir a la franja y a la hoja actuales de la
  pantalla Vender, sin dejar dos avisos del mismo pedido en pantalla.

**Resistencia**

- **FR-019**: Al cargar la página, al reconectar el canal de eventos y al volver la tableta a primer
  plano, la pantalla MUST volver a pedir al servidor lo pendiente.
- **FR-020**: Además del aviso en vivo, la pantalla MUST volver a pedir lo pendiente periódicamente,
  cada 15 s como máximo, como red de abajo para un aviso perdido sin que el canal se haya caído
  (el criterio de 5 s lo cumplen el aviso en vivo, la reconexión y el regreso a primer plano).
- **FR-026**: Un pedido de plataforma MUST dejar de estar pendiente cuando la plataforma avisa que lo
  canceló, o cuando pasa su plazo para decidir (la plataforma lo cancela sola). Sin esto, la alerta
  continua sonaría para siempre por un pedido muerto.
- **FR-021**: Sin red, la alerta de un pedido ya conocido MUST seguir sonando; nunca se apaga por un
  error de red.

**Permiso**

- **FR-022**: MUST existir un permiso nuevo con nombre estable en inglés, `orders.accept_incoming`,
  que cubre ver, aceptar y rechazar pedidos entrantes, y que el servidor exige en esas tres acciones.
- **FR-023**: El permiso MUST llegar por herencia a los cuatro roles sugeridos de toda empresa.
- **FR-024**: Los roles propios que ya existen al subir MUST conservar la capacidad que tenían (hoy
  cualquier sesión ve y acepta), sin que eso convierta al permiso en heredable para roles propios
  futuros.
- **FR-025**: Sin el permiso, la sesión MUST NOT sonar, ni ver el indicador ni la columna.

### Key Entities

- **Pedido entrante**: un pedido que llegó de un origen externo y espera que el negocio lo acepte o
  lo rechace. Tiene origen, folio, cliente, tipo de servicio, renglones, total, cuándo llegó, plazo
  opcional y estado (pendiente, aceptado, rechazado, cancelado por el origen, vencido). Al aceptarse
  se liga al pedido del POS que creó.
- **Origen**: de dónde viene un pedido entrante (plataforma de reparto; después menú público). Sabe
  confirmar una aceptación y un rechazo con su motivo, y puede tener plazo o no.
- **Motivo de rechazo**: lista cerrada en palabras de quien opera, cada uno con su equivalente en el
  origen.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Con la tableta en cualquier pantalla y el sonido desbloqueado, un pedido nuevo suena en
  menos de 5 s desde que el servidor lo registra, y sigue sonando hasta que alguien lo decide.
- **SC-002**: Con dos tabletas, decidir un pedido en una apaga la alerta en la otra en menos de 5 s.
- **SC-003**: Tras recargar, reconectar la red o despertar la tableta, un pedido pendiente vuelve a
  sonar (o a pedir activar el sonido) en menos de 5 s.
- **SC-004**: En 20 intentos de aceptar a la vez desde dos tabletas, cada pedido queda aceptado una
  sola vez: una comanda y un cobro.
- **SC-005**: Una persona nueva de una fonda, sin explicación, encuentra y acepta un pedido entrante
  desde una pantalla distinta de Pedidos en 2 toques, y rechaza otro con motivo en 4 toques o menos.
- **SC-006**: Sin pedidos pendientes, ninguna pantalla pierde alto por esta feature.

## Assumptions

- Hoy el único origen real son las plataformas de la spec 021; el pedido web (#328) llega después y
  se conecta al mismo módulo. Esta feature no construye el pedido web.
- Uber está apagado en producción; la feature se prueba en local con pedidos de plataforma
  pendientes sembrados en la base, y la confirmación del origen se simula en desarrollo (el plan
  define cómo, sin que la simulación pueda encenderse en producción).
- El tono es uno generado por la propia página, sin archivo de audio de terceros; cambiarlo es otro
  issue.
- La lista de motivos de rechazo es la que admite hoy la plataforma, reducida a los que tienen
  sentido para quien opera; el pedido web definirá los suyos al llegar.
- El volumen lo controla el equipo; no hay botón de silenciar: el issue pide que suene hasta decidir.
- El canal de eventos actual se reutiliza; no se agrega otro mecanismo de notificación (sin
  notificaciones del sistema operativo ni push).

## Out of Scope

- El pedido desde el menú público (#328) y cualquier pantalla del cliente final.
- Elegir o subir un tono propio, volumen por tableta.
- Aceptar automáticamente.
- Notificaciones fuera del navegador (push, sistema operativo, WhatsApp).
