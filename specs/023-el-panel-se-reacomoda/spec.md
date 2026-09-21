# Feature Specification: El panel del pedido se reacomoda

**Feature Branch**: `023-el-panel-se-reacomoda`

**Created**: 2026-09-20

**Status**: Draft

**Input**: User description: "Está súper saturada esa parte. Casi siempre dice Mostrador y el cliente casi no lo uso; ¿podríamos ahorrar espacio poniéndolo en otro lugar, por si algún otro negocio sí lo implementa?"

## El diseño ya está decidido, y se eligió mirándolo

Las variantes se dibujaron a tamaño real (360 × 600, el panel tal como cae en la tableta) y el dueño
eligió sobre los dibujos, no sobre una descripción:
[el canvas de reacomodos](https://claude.ai/artifact/VvfDZUESPiHcSmPZkpAa8F), tablero **V3 · Tipo y
un menú**.

Este spec no vuelve a discutir el diseño: dice qué tiene que ser cierto cuando esté construido.

## Contexto medido *(no se re-deriva)*

| Hecho | De dónde sale |
|---|---|
| El presupuesto real es 1024 × 600 y el panel se lleva ~32 % del ancho (~330 px) | Constitución, *Restricciones del producto* |
| La fila de tipo + cliente cuesta **52 px en todo pedido que no sea de plataforma** | Medido sobre `Ticket.tsx` en la revisión de la 022 |
| Con esa fila, en un domicilio caben ≈3.5 renglones de producto; sin ella, ≈4.5 | idem |
| A 1024×600 el panel **arranca colapsado** y la píldora es la vista por omisión | `POSPage.tsx`, `panelHidden` bajo `max-height: 720px` |
| Todo control tappable mide ≥44 px | Constitución |

**El dato que fija el alcance**: el nombre del cliente y el tipo de servicio son atributos que en
este negocio casi nunca cambian —siempre mostrador, casi nunca cliente— pero que **otro negocio sí
va a usar**. No se borran: se mueven a donde no cobren alto a quien no los usa.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - El panel devuelve el espacio que gastaba en lo que no se usa (Priority: P1)

Quien captura un pedido de mostrador —el de todos los días— ve más renglones de producto, porque el
tipo de servicio y el nombre del cliente dejaron de ocupar una fila entera debajo del total.

**Why this priority**: es la feature. Cada 52 px recuperados son medio renglón de producto más en
una pantalla donde el operador ya hace scroll con cuatro productos.

**Independent Test**: se captura un pedido de mostrador con cinco productos y se cuenta cuántos
renglones se ven sin hacer scroll, contra los que se veían antes.

**Acceptance Scenarios**:

1. **Given** un pedido de mostrador en captura, **When** se abre el panel, **Then** la zona de
   totales **no** tiene la fila de tipo + cliente, y los renglones de producto ganan ese alto.
2. **Given** ese mismo pedido, **When** se mira el encabezado del panel, **Then** ahí están el tipo
   de servicio y un menú, y el tipo dice cuál está activo sin tener que abrirlo.
3. **Given** un pedido de plataforma, **When** se abre el panel, **Then** el tipo **no** se puede
   cambiar —un pedido de plataforma es a domicilio por definición— y el encabezado no lo ofrece.

---

### User Story 2 - Lo que casi nadie usa vive en un menú, y sigue existiendo (Priority: P1)

El nombre del cliente, el descuento y vaciar el pedido se capturan desde un menú en el encabezado,
con renglones grandes.

**Why this priority**: es la otra mitad de la historia 1. Sin el menú, mover las cosas sería
quitarlas — y el nombre del cliente tiene que seguir existiendo para el negocio que sí lo use.

**Independent Test**: se abre el menú, se escribe un nombre de cliente y un descuento, y los dos
llegan al pedido y al ticket impreso.

**Acceptance Scenarios**:

1. **Given** el panel abierto, **When** se toca el menú, **Then** se ofrecen: nombre del cliente,
   descuento y vaciar el pedido, cada uno en un renglón de **al menos 48 px**.
2. **Given** el menú abierto, **When** se captura un nombre de cliente, **Then** ese nombre viaja
   con el pedido y sale en el ticket y en la comanda, igual que hoy.
3. **Given** un descuento aplicado desde el menú, **When** se cierra el menú, **Then** el descuento
   **se ve sin abrir nada**: el renglón bajo el total lo dice. Es dinero; esconderlo sería cobrar de
   menos sin decir por qué.
4. **Given** el menú abierto, **When** se toca fuera de él, **Then** se cierra sin aplicar nada.
5. **Given** «vaciar el pedido» en el menú, **When** se toca, **Then** sigue pidiendo confirmación
   como hoy: es destructivo y estar dentro de un menú no lo vuelve inofensivo.

---

### User Story 3 - Corregir el descuento de un pedido que ya se mandó (Priority: P1)

Quien va a cobrar ve el total, nota que el descuento quedó mal y lo corrige ahí mismo, sin cancelar
el pedido.

**Why this priority**: el descuento se teclea con el cliente enfrente y se teclea mal. Es la tarea
que la 022 dejó abierta a propósito (T031) y el momento en que el error se descubre es el cobro.

**Independent Test**: se crea un pedido con $50 de descuento, se abre la hoja de cobro, se corrige a
$30 y se cobra; el detalle de la venta muestra $30.

**Acceptance Scenarios**:

1. **Given** un pedido con descuento en la hoja de cobro, **When** se toca el control del renglón de
   descuento, **Then** se puede cambiar el monto o el porcentaje, y lo que se cobra se recalcula.
2. **Given** un pedido **ya cobrado por completo**, **When** se intenta corregir su descuento,
   **Then** el sistema lo rechaza diciendo por qué, y ninguna cifra se mueve.
3. **Given** un pedido **sin** descuento en la hoja de cobro, **When** se mira, **Then** no aparece
   un renglón de descuento en $0.00 — pero sí se puede agregar uno.

---

### Edge Cases

- **El encabezado en el pedido más largo.** Nombre de folio largo + tipo + menú tienen que caber en
  ~330 px sin empujar nada fuera. El nombre trunca; los controles no.
- **El menú abierto cuando el panel se colapsa.** A 1024×600 el panel puede ocultarse: el menú no
  puede quedar flotando sobre el catálogo.
- **El envío.** Solo aparece en domicilio propio y **se queda en la zona baja**: es un importe que
  suma al total y se lee junto a él, no un atributo que se esconda.
- **El pedido de plataforma.** No ofrece tipo de servicio ni envío; sí cliente y descuento.
- **Tocar el menú por error.** Abre una lista, no ejecuta nada: ninguna opción del menú mueve dinero
  por sí sola.
- **La hoja de cobro de un pedido sin descuento.** Agregar uno desde ahí también tiene que funcionar,
  no solo corregir el que ya estaba.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: El tipo de servicio MUST vivir en el encabezado del panel y decir cuál está activo sin
  abrir nada.
- **FR-002**: El nombre del cliente, el descuento y vaciar MUST vivir en un menú del encabezado, con
  renglones de al menos 48 px.
- **FR-003**: La zona de totales MUST quedar **sin controles tocables** salvo el campo de envío
  —cuando aplica— y los dos botones de acción.
- **FR-004**: Un descuento aplicado MUST verse en la zona de totales sin abrir el menú.
- **FR-005**: El sistema MUST seguir pidiendo confirmación para vaciar el pedido.
- **FR-006**: El nombre del cliente MUST seguir llegando al pedido, al ticket y a la comanda.
- **FR-007**: Los usuarios MUST poder corregir el descuento de un pedido ya creado desde la hoja de
  cobro, mientras no esté cobrado por completo.
- **FR-008**: Un pedido de plataforma MUST seguir sin ofrecer tipo de servicio ni costo de envío.
- **FR-009**: Ningún control nuevo MUST medir menos de 44 px de alto tappable.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: En un pedido de mostrador se ve **al menos un renglón de producto más** que antes del
  cambio, en la misma pantalla de 1024 × 600.
- **SC-002**: Capturar un pedido de mostrador sin cliente ni descuento —el caso de todos los días—
  no cuesta **ni un toque más** que antes.
- **SC-003**: Corregir un descuento mal capturado no obliga a cancelar el pedido.
- **SC-004**: Ninguna cifra de dinero cambia de significado: el total, el envío y el descuento
  siguen diciendo lo mismo que hoy.

## Assumptions

- **El tipo de servicio se alterna, no se elige de una lista.** Son dos valores (mostrador y
  domicilio) y una lista para dos opciones son dos toques donde basta uno.
- **El envío no se mueve.** Es dinero que suma al total y se lee junto a él.
- **No se toca la barra del POS ni la píldora.** El ancho de la barra ya está medido y no sobra;
  esta feature vive dentro del panel.
- **El menú es del panel, no de la aplicación.** No se reutiliza para acciones que no sean de este
  pedido.
