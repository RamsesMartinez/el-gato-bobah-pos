# Feature Specification: Roles y permisos por empresa — entrega 1: el cajero ve los pedidos entregados

**Feature Branch**: `068-roles-por-empresa`

**Created**: 2026-11-06

**Status**: Draft

**Input**: Issue #61 «Roles y permisos por empresa (modelo híbrido)», incluida su sección «Caso que entra en este issue: el cajero ve los pedidos entregados (2026-11-06)».

## Corte en entregas

El issue #61 no cabe en una sola subida, y una de sus mitades cruza una puerta del principio VIII
(roles guardados en la base), cuyo diseño decide el dueño antes de construir. Se parte así:

| Entrega | Qué lleva | Dónde |
| --- | --- | --- |
| **1 — esta spec** | El permiso por capacidad para ver los pedidos entregados del día y reimprimir su ticket; cajero, gerente y admin lo traen de inicio; la pantalla de Pedidos y el servidor preguntan por permiso, no por rol, para esa lista y para devolver dinero de un pedido entregado | Esta spec, rama `068-roles-por-empresa` |
| **2 — issue nuevo enlazado al #61** | Roles por empresa en la base (sugeridos precargados + propios), pantalla para administrarlos, migrar los `RequireRole` y los gates por nombre de rol que quedan, y decidir qué pasa con `users.role` | Issue nuevo; el #61 se cierra solo cuando las dos estén entregadas |

**Por qué este corte**: la entrega 1 resuelve un hueco que hoy obliga a sacar a una persona de su
puesto para reimprimir un ticket, y no necesita guardar nada nuevo: los permisos de los roles
sugeridos todavía viven en el mapa fijo del producto, y agregar ahí un permiso lo reparte a todas
las empresas, nuevas y existentes, en la misma subida. La entrega 2 sí guarda roles como dato, y
esa forma (tabla, qué pasa con el rol de cada usuario, cómo se migra) es una puerta que el dueño
decide antes de construir.

### Lo que la entrega 2 ya tiene decidido (dueño, 2026-10-13)

- Un rol no se borra: se inactiva. Solo se borra definitivamente si no tiene ningún usuario asignado.
- Los roles de sistema (los que pone el producto) no se borran; solo los propios de la empresa.
- Fuera de alcance: permisos por pantalla o por botón; uno por capacidad.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - La cajera reimprime el ticket de un pedido ya entregado (Priority: P1)

Una cajera atiende el mostrador. Un pedido de plataforma lo capturó y lo entregó otra persona desde
su usuario, en el mismo momento. El repartidor o el cliente pide el ticket. La cajera abre Pedidos,
ve el pedido en la lista de entregados del día y reimprime su ticket, sin llamar a nadie.

**Why this priority**: es el caso que pasó el 2026-11-06 y el único que esta entrega viene a
resolver. Hoy obliga a que una administradora deje lo que hace y entre con su usuario.

**Independent Test**: con un usuario de rol cajero, abrir Pedidos con al menos un pedido entregado
hoy por otro usuario; aparece en la lista de entregados y «Ticket» abre la reimpresión.

**Acceptance Scenarios**:

1. **Given** una cajera con sesión y un pedido entregado hoy que capturó otro usuario, **When** abre Pedidos, **Then** la sección de entregados del día aparece y el pedido está en ella.
2. **Given** esa misma lista, **When** toca «Ticket» en el pedido, **Then** se abre la reimpresión del ticket de ese pedido.
3. **Given** esa misma lista, **When** la cajera la mira, **Then** no aparece la acción «Devolver» en ningún pedido, aunque el pedido tenga dinero cobrado.
4. **Given** una cajera, **When** pide la lista de entregados directamente al servidor, **Then** la recibe; **When** intenta devolver dinero de un pedido entregado, **Then** el servidor lo rechaza.

---

### User Story 2 - Gerente y admin siguen igual (Priority: P1)

Una gerente o una administradora abre Pedidos: ve los entregados del día, reimprime y puede
devolver dinero, exactamente como hoy.

**Why this priority**: separar «ver» de «devolver» no puede quitarle nada a quien ya lo tenía.

**Independent Test**: con usuarios gerente y admin, la lista y el botón «Devolver» aparecen y la
devolución se registra.

**Acceptance Scenarios**:

1. **Given** una gerente con un pedido entregado y cobrado, **When** abre Pedidos, **Then** ve el pedido, «Ticket» y «Devolver».
2. **Given** una administradora, **When** devuelve dinero de un pedido entregado, **Then** el servidor lo acepta como hoy.

---

### User Story 3 - El mesero no lo trae por omisión (Priority: P2)

Un usuario de rol mesero abre Pedidos: no ve la sección de entregados, y el servidor no le entrega
la lista.

**Why this priority**: el permiso se da a los roles que cobran en el mostrador; el mesero no lo
pidió nadie y repartirlo de más es más difícil de quitar después.

**Independent Test**: con un usuario mesero, la sección no aparece y la lista responde «sin permiso».

**Acceptance Scenarios**:

1. **Given** un mesero, **When** abre Pedidos, **Then** no aparece la sección de entregados del día.
2. **Given** un mesero, **When** pide la lista de entregados al servidor, **Then** recibe «Tu usuario no puede ver los pedidos entregados».

---

### Edge Cases

- **Sesión abierta antes de la subida**: la tableta conoce sus permisos desde que inició o renovó
  sesión. Una cajera con la sesión abierta antes de la subida ve la sección a más tardar en la
  siguiente renovación de su sesión (o al volver a entrar); no se pide que aparezca al instante.
- **Pantalla vieja durante la subida**: mientras una tableta corra la pantalla anterior, pregunta
  por rol y a la cajera no le muestra la sección: igual que hoy, nada se rompe. Con la API anterior
  y la pantalla nueva, la sesión no trae el permiso nuevo y la sección no aparece: tampoco se rompe.
- **Pedido entregado que todavía debe**: la cajera ve el aviso y el camino a cobrarlo en su cuenta,
  igual que hoy lo ven gerente y admin (el cobro ya está permitido a la cajera).
- **Pedido ya devuelto por completo**: la marca de devolución se sigue viendo; la cajera tampoco
  ve «Devolver», como hoy no lo ve nadie cuando ya no queda nada por devolver.
- **Empresa distinta**: la lista sigue mostrando solo los pedidos de la empresa de la sesión.
- **Rol desconocido o sesión sin lista de permisos**: no se muestra la sección (se niega por omisión).

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: Existe un permiso por capacidad, «ver los pedidos entregados del día y reimprimir su
  ticket», con nombre estable en inglés (`orders.delivered_view`), separado del permiso de devolver
  dinero.
- **FR-002**: Los roles sugeridos admin, gerente y cajero traen ese permiso de inicio, en toda
  empresa, nueva o existente, desde la subida y sin ningún paso manual. El mesero no lo trae.
- **FR-003**: El servidor entrega la lista de pedidos entregados del día a quien tiene ese permiso,
  y a nadie más; quien no lo tiene recibe un rechazo que dice qué no puede hacer, sin nombrar roles.
- **FR-004**: La pantalla de Pedidos muestra la sección de entregados del día si y solo si la sesión
  trae ese permiso; no pregunta por el nombre del rol.
- **FR-005**: Devolver dinero de un pedido entregado (y consultar con qué terminal se devolvería)
  se decide por el permiso de devolver dinero (`payments.void`, el mismo que ya decide devolver un
  pago), en la pantalla y en el servidor. Por omisión lo siguen teniendo solo admin y gerente.
- **FR-006**: La acción «Devolver» de la lista de entregados aparece solo con el permiso de devolver
  dinero, además de las condiciones que ya tiene (que quede algo por devolver).
- **FR-007**: El texto que ve quien no tiene el permiso no nombra roles: con roles propios por
  empresa, el rol que sí puede quizá no se llame igual.
- **FR-008**: Reimprimir el ticket desde la lista no exige más permiso que el de verla.

### Key Entities

- **Permiso**: capacidad con nombre estable en inglés que un rol trae o no. Hoy los permisos de cada
  rol sugerido salen de un mapa fijo del producto; en la entrega 2 pasarán a la base como semilla.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Una cajera reimprime el ticket de un pedido entregado hoy por otro usuario en 2 toques
  desde Pedidos (abrir la sección ya visible, «Ticket»), sin que nadie más entre con su usuario.
- **SC-002**: 0 cambios de lo que pueden hacer gerente y admin en Pedidos: siguen viendo la lista y
  devolviendo dinero.
- **SC-003**: Ni la cajera ni el mesero pueden devolver dinero de un pedido entregado, ni por
  pantalla ni pidiéndolo directo al servidor.
- **SC-004**: En la pantalla de Pedidos y en las tres rutas de entregados y devolución, ningún
  control pregunta por el nombre del rol.

## Assumptions

- «Devolver dinero» de un pedido entregado es la misma capacidad que «devolver un pago»
  (`payments.void`): las dos sacan dinero del corte hacia el cliente, y hoy las dos son de admin y
  gerente. Unirlas evita un segundo permiso para la misma capacidad, que es lo que pide la regla
  «uno por capacidad». Si en la entrega 2 el dueño las quiere separadas, se separa entonces sin
  cambiar ninguna pantalla.
- La reimpresión ya lee el pedido por un camino abierto a toda sesión; lo único que la impedía era
  no ver la lista.
- No hace falta migración de datos: los permisos de los roles sugeridos todavía no se guardan en la
  base.
- La entrega 2 queda en un issue nuevo enlazado al #61, con su diseño a decidir por el dueño.
