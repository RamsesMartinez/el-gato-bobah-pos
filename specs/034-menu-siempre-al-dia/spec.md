# Feature Specification: El menú de la tableta siempre al día

**Feature Branch**: `034-menu-siempre-al-dia`

**Created**: 2026-10-10

**Status**: Draft

**Input**: Aprobado por el dueño el 2026-10-11 (opciones B + D + F) tras el incidente del 2026-10-10.

## Contexto

El 2026-10-10 el catálogo de producción se corrigió con un script directo a la base: un grupo de
extras se desligó de un producto y se ligó otro. La tableta siguió ofreciendo el grupo viejo porque
el menú se guardaba hasta 24 horas y solo se refrescaba cuando el cambio entraba por las pantallas
de administración. El servidor, que sí veía el catálogo nuevo, rechazó cada intento de venta de
ese producto con «X no es un extra de Y». El personal no pudo vender ese producto hasta que alguien
borró el menú guardado a mano.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - El catálogo cambiado por cualquier camino llega a la tableta (Priority: P1)

Quien administra cambia el catálogo —por la pantalla de administración, por un script o por una
migración— y la siguiente vez que la tableta pide el menú recibe el catálogo nuevo, sin que nadie
tenga que borrar nada a mano.

**Why this priority**: Es la causa del incidente. Sin esto las otras dos historias solo maquillan
un menú viejo.

**Independent Test**: Cambiar un producto, un grupo de extras o un precio de plataforma con una
sentencia directa a la base y pedir el menú: la respuesta refleja el cambio.

**Acceptance Scenarios**:

1. **Given** un menú ya guardado, **When** se desliga un grupo de extras de un producto con un
   script directo, **Then** el siguiente menú que se pide ya no ofrece ese grupo en ese producto.
2. **Given** dos empresas, **When** cambia el catálogo de una, **Then** el menú guardado de la otra
   no se invalida ni cambia.
3. **Given** un script que cambia productos de dos empresas en una sola sentencia y sin decir de
   qué empresa es, **When** termina, **Then** el menú de las dos queda al día.
4. **Given** una venta en curso, **When** se cobra un pedido, **Then** cobrar no se vuelve más lento
   ni espera a que termine un cambio de catálogo de otra persona.

---

### User Story 2 - La tableta se entera sola y barato (Priority: P2)

La tableta comprueba si el menú cambió cuando vuelve a primer plano y cada minuto. Si no cambió, la
comprobación no trae el catálogo completo de nuevo.

**Why this priority**: Aunque el servidor esté al día, la tableta guardaba su copia cinco minutos y
no siempre recibe el aviso en vivo (red intermitente, tableta suspendida).

**Independent Test**: Con la pantalla de venta abierta, cambiar el catálogo y esperar un minuto o
traer la pantalla al frente: el producto ya muestra lo nuevo. Sin cambios, la comprobación responde
«sin cambios» sin cuerpo.

**Acceptance Scenarios**:

1. **Given** la pantalla de venta abierta, **When** pasa un minuto sin cambios de catálogo,
   **Then** la tableta comprueba y el servidor responde «sin cambios» sin mandar el menú.
2. **Given** la tableta suspendida, **When** vuelve a primer plano, **Then** comprueba el menú en
   ese momento.
3. **Given** que se cae la red, **When** falla la comprobación, **Then** la tableta sigue mostrando
   el último menú que tenía y nada de lo que hoy funciona sin red deja de funcionar.

---

### User Story 3 - Si aun así se vende con un menú viejo, se corrige sin drama (Priority: P3)

Si el operador elige un extra o un producto que el servidor ya no reconoce para ese producto, la
tableta trae el menú nuevo, dice «El menú se actualizó; elige de nuevo», vuelve a abrir la hoja de
extras del producto con lo nuevo y conserva el resto de la cuenta.

**Why this priority**: Es la red de seguridad para la ventana entre el cambio y la siguiente
comprobación; sin ella el operador ve un error rojo que no puede accionar.

**Independent Test**: Con un menú viejo forzado en la tableta, agregar el producto con el extra
desligado: aparece el aviso neutro, se reabre la hoja con los extras actuales y los demás renglones
siguen ahí.

**Acceptance Scenarios**:

1. **Given** una cuenta con dos productos, **When** se agrega un tercero con un extra que ya no
   pertenece, **Then** se ve «El menú se actualizó; elige de nuevo», la hoja de extras se reabre
   con los extras actuales y los dos productos anteriores siguen en la cuenta.
2. **Given** un producto que ya no se vende, **When** se agrega, **Then** se ve el mismo aviso
   neutro, el menú se recarga (el producto desaparece) y no se abre ninguna hoja.
3. **Given** cualquier otro rechazo (sin caja, red caída, cuenta cerrada), **Then** el
   comportamiento es el de hoy.
4. **Given** una cuenta capturada con un extra cuyo grupo se cambió después (perlas viejas → nuevas),
   **When** se toca «Enviar a cocina» o «Enviar y cobrar», **Then** no se manda nada, no se abre el
   cobro, se ve el aviso neutro (no el rojo «No se pudo: X»), el menú se recarga, el renglón
   afectado queda señalado con «Elige de nuevo» y su hoja de extras se reabre con lo actual; los
   demás renglones siguen en la cuenta.
5. **Given** un producto de la cuenta que dejó de venderse, **When** se manda o se cobra, **Then**
   el mismo aviso neutro y el renglón queda señalado (y tachado) sin abrir hoja, para quitarlo.

### Edge Cases

- Un producto con fecha de fin de disponibilidad: al cambiar el día deja de aparecer aunque nadie
  toque el catálogo.
- Un cambio que no afecta ninguna fila (sentencia sin coincidencias) no cuenta como cambio.
- Un script que no declara de qué empresa es: cada empresa tocada queda al día, y solo ésas.
- Un despliegue que cambia la forma del menú: la tableta recibe la forma nueva aunque el catálogo
  no haya cambiado.
- La tableta sin red: se queda con su menú; la comprobación falla en silencio y se reintenta.
- El servicio de memoria rápida caído: el menú se arma desde la base y sigue al día.
- Rechazo al cambiar extras de un renglón ya capturado (no al agregar): mismo aviso neutro, el
  renglón queda como estaba.
- Varios toques rechazados seguidos por el mismo motivo: un solo aviso, no una pila de avisos.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: El sistema MUST llevar, por empresa, una versión del catálogo que cambia sola con
  cualquier alta, baja o cambio en lo que alimenta el menú de la tableta, sin importar el camino
  (pantalla, script directo, migración).
- **FR-002**: El menú guardado MUST quedar ligado a esa versión, de modo que un menú de una versión
  anterior nunca se sirva.
- **FR-003**: La versión de una empresa MUST NOT cambiar por cambios de otra empresa.
- **FR-004**: Los cambios de catálogo MUST NOT bloquear ni hacer esperar el cobro de pedidos.
- **FR-005**: La respuesta del menú MUST identificar su versión, y la tableta MUST poder preguntar
  «¿cambió desde esta versión?» y recibir «sin cambios» sin el menú completo.
- **FR-006**: La identidad del menú MUST cambiar también al cambiar el día (disponibilidad por
  fecha) y al cambiar la forma del documento en un despliegue.
- **FR-007**: La tableta MUST comprobar el menú al volver a primer plano y cada 60 segundos mientras
  está abierta.
- **FR-008**: Una comprobación fallida MUST conservar el menú que la tableta ya tenía.
- **FR-009**: Cuando el servidor rechaza un producto o extra porque ya no se vende o ya no
  pertenece al producto, MUST responder con un código estable que lo distinga de otros rechazos.
- **FR-010**: Ante ese código la tableta MUST recargar el menú, mostrar el aviso neutro «El menú se
  actualizó; elige de nuevo», reabrir la hoja de extras del producto si sigue a la venta y tiene
  extras, y conservar el resto de la cuenta.
- **FR-011**: El cambio de esquema MUST ser aditivo: la versión anterior del servidor sigue
  funcionando con él, porque se despliega con el negocio operando.
- **FR-013**: El mismo código MUST aplicar al mandar a cocina y al «Enviar y cobrar», y la respuesta
  MUST identificar el renglón de la cuenta rechazado; la tableta MUST señalarlo para elegirlo de
  nuevo, sin perder la cuenta y sin el aviso rojo. Cobrar un pedido ya enviado no revalida el menú
  (sus renglones ya son pedido) y queda fuera.
- **FR-012**: La documentación operativa MUST decir que un cambio de catálogo por script ya no
  necesita borrar el menú guardado a mano.

### Key Entities

- **Versión de catálogo**: un contador por empresa y el momento del último cambio.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Tras un cambio de catálogo por cualquier camino, una tableta abierta muestra el menú
  nuevo en 60 segundos o menos sin intervención de nadie.
- **SC-002**: Cero intervenciones manuales para refrescar el menú después de un script de catálogo.
- **SC-003**: Una comprobación sin cambios no transfiere el catálogo.
- **SC-004**: Ante un menú viejo, el operador termina de agregar el producto con un toque de
  reelección, sin perder renglones y sin ver un error rojo.

## Assumptions

- El aviso en vivo que ya existe para cambios hechos desde la pantalla se conserva; la
  comprobación periódica es la red para cuando ese aviso no llega.
- 60 segundos es la cadencia aprobada; no se hace configurable.
- Un recálculo de costos que reescribe productos cuenta como cambio de catálogo (el menú muestra
  costo y margen), aunque algún valor no cambie: el costo es un menú rearmado de más.
