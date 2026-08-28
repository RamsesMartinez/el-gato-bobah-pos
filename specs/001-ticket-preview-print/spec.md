# Feature Specification: Visualizador e impresión del ticket de venta

**Feature Branch**: `001-ticket-preview-print`

**Created**: 2026-08-27

**Status**: Draft

**Input**: User description: "Visualizador de ticket de venta con impresión desde el navegador, datos del negocio y logo configurable. Vista previa que muestra exactamente lo que va a salir impreso (80mm) y desde la cual se manda a imprimir, disponible al cerrar un pedido en el POS y para reimprimir desde el tablero de órdenes. El encabezado lleva los datos del negocio y un logo subible desde el panel, con el ícono del Gato Bobah por default. Debe funcionar igual en local y desplegado en web, sin instalar nada en la máquina del operador."

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Ver el ticket y mandarlo a imprimir al cerrar el pedido (Priority: P1)

El cajero cierra un pedido y, antes de que salga papel, ve en pantalla el ticket tal como va a
imprimirse: el encabezado del negocio con su logo, las líneas del pedido con sus modificadores, el
desglose y el total. Confirma que es el pedido correcto y toca imprimir. Si se dio cuenta de que se
equivocó, cierra la vista sin gastar papel.

**Why this priority**: Es el caso de todos los días y el que hoy está a ciegas. El operador no puede
saber qué salió impreso hasta que lo tiene en la mano, y un ticket equivocado ya consumió papel y
tiempo del cliente que está esperando en el mostrador. Entregado solo, ya es un producto usable.

**Independent Test**: Cerrar un pedido con al menos dos líneas y un modificador, verificar que la
vista previa muestre los mismos datos que el papel, y comparar el papel impreso contra la pantalla.

**Acceptance Scenarios**:

1. **Given** un pedido recién cerrado con líneas y modificadores, **When** el operador abre la vista
   previa, **Then** ve el ticket completo (encabezado, líneas, modificadores, subtotal, envío si
   aplica, total y estado de pago) sin recortes.
2. **Given** la vista previa abierta, **When** el operador toca imprimir, **Then** el ticket sale por
   la impresora y lo impreso coincide campo por campo con lo que estaba en pantalla.
3. **Given** la vista previa abierta, **When** el operador la cierra sin imprimir, **Then** no se
   consume papel y regresa a la pantalla anterior con el pedido intacto.
4. **Given** un pedido a domicilio con costo de envío, **When** se abre la vista previa, **Then** el
   ticket muestra subtotal y envío desglosados antes del total.
5. **Given** la vista previa abierta, **When** el operador toca imprimir dos veces seguidas,
   **Then** sale un solo ticket.

---

### User Story 2 - Reimprimir el ticket de un pedido ya cerrado (Priority: P2)

El cliente pide su ticket después de que se cerró el pedido, o el papel salió cortado a la mitad. El
operador busca el pedido en el tablero de órdenes, abre la misma vista previa y lo vuelve a
imprimir. El ticket reimpreso queda marcado como reimpresión para que no pueda pasar por un
comprobante distinto del original.

**Why this priority**: Hoy no existe forma de recuperar un ticket: si el papel se atoró, se perdió.
Es frecuente, pero menos que el flujo de cierre, y depende de que la vista previa ya exista.

**Independent Test**: Cerrar un pedido, salir del POS, entrar al tablero, abrir el pedido y
reimprimir; comparar contra el ticket original.

**Acceptance Scenarios**:

1. **Given** un pedido cerrado visible en el tablero, **When** el operador pide su ticket, **Then**
   se abre la misma vista previa con los datos de ese pedido.
2. **Given** la vista previa de una reimpresión, **When** el operador imprime, **Then** el papel sale
   marcado visiblemente como reimpresión.
3. **Given** un pedido cancelado o reembolsado, **When** el operador abre su ticket, **Then** el
   estado del pedido se ve en el ticket y no se presenta como una venta cobrada.

---

### User Story 3 - Configurar los datos del negocio y el logo del ticket (Priority: P3)

El administrador entra a los ajustes del negocio, captura el nombre comercial, la dirección, el
teléfono, un texto para el encabezado y otro para el pie, y sube el logo que quiere en los tickets. A partir de ese
momento todos los tickets salen con esos datos. Si nunca sube un logo, los tickets salen con el
ícono del Gato Bobah.

**Why this priority**: Tiene un default que funciona desde el primer día, así que el negocio puede
operar sin esto. Es lo que hace que el ticket deje de estar escrito en el código.

**Independent Test**: Cambiar el nombre y subir un logo, cerrar un pedido nuevo y verificar que el
ticket salga con los datos nuevos sin reiniciar ni volver a desplegar el sistema.

**Acceptance Scenarios**:

1. **Given** un negocio sin logo configurado, **When** se imprime cualquier ticket, **Then** el
   encabezado usa el logo por default del Gato Bobah.
2. **Given** el administrador en ajustes, **When** sube una imagen válida como logo, **Then** el
   siguiente ticket la usa, sin reiniciar el sistema.
3. **Given** el administrador en ajustes, **When** intenta subir un archivo que no es una imagen
   permitida o que excede el tamaño máximo, **Then** el sistema lo rechaza con un mensaje que dice
   qué se aceptaba, y el logo anterior queda intacto.
4. **Given** un usuario sin permiso de administración, **When** intenta cambiar el logo o los datos
   del negocio, **Then** el sistema lo rechaza aunque la opción no se le muestre en pantalla, y el intento queda registrado como evento de seguridad.
5. **Given** un logo ya subido, **When** el sistema se vuelve a desplegar, **Then** el logo sigue
   ahí.
6. **Given** el administrador en ajustes, **When** quita el logo subido, **Then** los tickets
   regresan al logo por default.

---

### User Story 4 - Imprimir solo al cerrar la venta (Priority: P2)

En la hora pico el cajero cobra y entrega; un toque de más por pedido son minutos de fila al final
del día. El administrador enciende la impresión automática y, a partir de ahí, cerrar un pedido saca
el papel sin que nadie toque nada. El ticket sigue estando disponible para verlo o reimprimirlo.

**Why this priority**: La vara de UX del POS es minimizar taps, y éste es el toque que más se repite
en el día. No es P1 porque el sistema es usable sin él y porque depende de que el equipo esté
configurado para imprimir sin diálogo.

**Independent Test**: Encender la opción, cerrar un pedido y ver salir el papel sin tocar nada;
apagarla y comprobar que vuelve a pedir el toque.

**Acceptance Scenarios**:

1. **Given** la impresión automática encendida, **When** el operador cierra un pedido, **Then** el
   ticket sale sin ninguna interacción adicional.
2. **Given** la impresión automática encendida, **When** el ticket ya salió, **Then** el operador
   puede abrirlo y volver a imprimirlo a mano.
3. **Given** la impresión automática apagada, **When** el operador cierra un pedido, **Then** no
   sale papel hasta que él lo pide.
4. **Given** la impresión automática encendida y la impresora apagada o sin papel, **When** se
   cierra un pedido, **Then** el pedido queda registrado igual y el operador puede reimprimir
   después: una falla de impresión nunca pierde la venta.

---

### Edge Cases

- **Pedido sin líneas**: no debería existir, pero si llega uno, la vista previa muestra el
  encabezado y los totales en cero sin romperse.
- **Ticket muy largo** (muchas líneas y modificadores): la vista previa permite recorrerlo completo;
  el papel es continuo, así que no hay corte por página.
- **Nombres largos** de producto, modificador o cliente: se acomodan en el ancho del ticket sin
  desbordarse ni empujar la columna de importes.
- **Datos con caracteres especiales o intentos de inyección** en nombre de cliente, producto,
  modificador o notas: se muestran como texto plano tanto en pantalla como en papel.
- **Acentos y ñ**: se ven correctamente en pantalla y en papel.
- **Logo con transparencia, a color o muy grande**: el ticket lo muestra dentro del ancho del
  encabezado sin deformarlo; el resultado monocromo en papel es propio de la impresora térmica.
- **Sin impresora disponible o apagada**: el operador recibe el comportamiento normal de su equipo;
  el sistema no se bloquea ni pierde el pedido.
- **El navegador pregunta antes de imprimir**: es un comportamiento válido, no un error. La ausencia
  del diálogo depende de cómo se lanzó el navegador (ver Assumptions).
- **Reimpresión de un pedido muy viejo**: si el pedido existe, su ticket se puede reimprimir.
- **Dos operadores imprimiendo el mismo pedido a la vez**: cada uno obtiene su copia; ninguno
  bloquea al otro.

## Requirements *(mandatory)*

### Functional Requirements

#### Vista previa e impresión

- **FR-001**: El sistema MUST mostrar una vista previa del ticket de venta que represente el mismo
  contenido que se enviará a la impresora, sin diferencias de datos entre pantalla y papel.
- **FR-002**: El sistema MUST generar el contenido del ticket a partir de una sola definición, de
  modo que sea imposible que la vista previa y lo impreso diverjan.
- **FR-003**: Los usuarios MUST poder mandar a imprimir desde la propia vista previa, sin pasos
  intermedios.
- **FR-004**: Los usuarios MUST poder cerrar la vista previa sin imprimir, sin efectos secundarios
  sobre el pedido.
- **FR-005**: El sistema MUST evitar que un toque repetido en imprimir produzca más de un ticket por
  acción del operador.
- **FR-006**: El sistema MUST ofrecer la vista previa al cerrar un pedido en el punto de venta, en
  lugar de imprimir a ciegas.
- **FR-007**: El sistema MUST ofrecer la vista previa para cualquier pedido listado en el tablero de
  órdenes, incluidos los ya entregados.
- **FR-008**: El sistema MUST marcar en el papel los tickets que son una reimpresión, distinguibles
  del ticket original.
- **FR-009**: El sistema MUST funcionar de forma idéntica en la instalación local y en la desplegada
  en web, sin requerir que el operador instale extensiones, complementos ni aplicaciones.
- **FR-010**: El sistema MUST presentar la vista previa legible y operable en una pantalla de 7
  pulgadas, con la acción de imprimir alcanzable sin desplazarse.

#### Contenido del ticket

- **FR-011**: El ticket MUST incluir un encabezado con el logo del negocio y sus datos de
  identificación y contacto.
- **FR-012**: El ticket MUST incluir el número de pedido, la fecha y hora, el tipo de servicio, el
  nombre del cliente cuando exista, las líneas con cantidad, descripción, modificadores e importe,
  el desglose de envío cuando aplique, el total y el estado de pago.
- **FR-013**: El sistema MUST tratar como texto plano cualquier dato capturado por una persona
  (nombres de cliente, producto, modificador, notas y datos del negocio) al mostrarlo o imprimirlo.
- **FR-014**: El sistema MUST expresar los importes con la misma precisión y redondeo que el pedido
  registrado, sin recalcularlos en el ticket.

#### Datos del negocio y logo

- **FR-015**: El sistema MUST permitir configurar los datos del negocio que aparecen en el ticket.
- **FR-016**: El sistema MUST permitir subir una imagen como logo del ticket y quitarla para volver
  al default.
- **FR-017**: El sistema MUST usar un logo por default cuando el negocio no ha subido ninguno.
- **FR-018**: El sistema MUST rechazar archivos de logo cuyo tipo o tamaño estén fuera de lo
  permitido, indicando qué se aceptaba, y conservar el logo anterior.
- **FR-019**: El sistema MUST verificar en el servidor que quien cambia los datos del negocio o el
  logo tiene permiso para hacerlo, independientemente de lo que muestre la interfaz.
- **FR-020**: El sistema MUST conservar el logo y los datos del negocio a través de reinicios y
  nuevos despliegues.
- **FR-021**: El sistema MUST reflejar un cambio de logo o de datos del negocio en los tickets
  siguientes sin reiniciar ni volver a desplegar.
- **FR-022**: El sistema MUST registrar como evento de seguridad los intentos rechazados de cambiar
  los datos del negocio o el logo por falta de permiso.
- **FR-023**: El sistema MUST permitir configurar un texto libre que se imprime **arriba** del
  detalle del pedido y otro **abajo**, ambos opcionales, **de varias líneas** y con los saltos de
  línea respetados en el papel. El de abajo debe alcanzar para el aviso de que el ticket no es
  comprobante fiscal junto con los datos para pedir factura.
- **FR-027**: Los usuarios MUST poder imprimir un **ticket de prueba** desde la configuración, para
  verificar en papel cómo quedó el logo y los textos sin registrar una venta.
- **FR-028**: El sistema MUST marcar en el papel los tickets de prueba, distinguibles de una venta
  real.
- **FR-024**: El sistema MUST permitir activar que el ticket se imprima solo al cerrar una venta,
  sin que el operador toque nada.
- **FR-025**: Con la impresión automática activada, el sistema MUST seguir permitiendo abrir el
  ticket y volver a imprimirlo a mano.
- **FR-026**: El sistema MUST tratar los textos superior e inferior como texto plano, igual que
  cualquier otro dato capturado por una persona.

### Key Entities

- **Ajustes del negocio**: los datos que identifican al negocio en el ticket — nombre comercial,
  dirección, teléfono y leyenda de pie. Existe uno por empresa. Hoy esta entidad ya guarda el costo
  de envío.
- **Logo del ticket**: la imagen del encabezado, con su tipo y su tamaño. Opcional: cuando no
  existe, se usa el logo por default del sistema. Uno por empresa.
- **Ticket de venta**: la representación imprimible de un pedido en un momento dado. No es una
  entidad nueva que se guarde: se deriva del pedido y de los ajustes del negocio vigentes al
  imprimir.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Desde que el pedido queda cerrado, el operador ve el ticket y lo manda a imprimir en
  no más de dos toques.
- **SC-002**: En una comparación campo por campo entre la vista previa y el papel (encabezado,
  cada línea, cada modificador, desglose y total), el 100% de los campos coincide.
- **SC-003**: Reimprimir el ticket de un pedido ya cerrado toma no más de tres toques desde el
  tablero de órdenes.
- **SC-004**: Un cambio de logo o de datos del negocio aparece en el siguiente ticket impreso, sin
  reiniciar ni volver a desplegar el sistema.
- **SC-005**: El logo y los datos del negocio siguen presentes después de un despliegue nuevo del
  sistema.
- **SC-006**: El flujo completo (ver, imprimir, reimprimir) se ejecuta igual en la instalación local
  y en la desplegada, sin instalar nada adicional en el equipo del operador.
- **SC-007**: Un operador que no conoce la función logra imprimir un ticket a la primera, sin
  instrucciones, en una pantalla de 7 pulgadas y sin usar zoom.
- **SC-008**: Ningún dato capturado por una persona puede alterar la estructura del ticket ni
  ejecutar código, verificado con entradas de prueba hostiles en todos los campos libres.
- **SC-009**: Con la impresión automática encendida, cerrar una venta produce el ticket con **cero**
  toques adicionales del operador.

## Assumptions

- **La impresora ya está instalada y configurada en el equipo del operador**, como impresora del
  sistema. Instalarla, elegir el corte de papel y configurar el cajón de dinero queda fuera de esta
  feature: son ajustes del equipo, no del POS.
- **Que el navegador muestre o no su diálogo de impresión depende de cómo fue lanzado**, no de esta
  feature. Con el navegador configurado para impresión directa, el ticket sale sin preguntar; sin
  esa configuración, sale el diálogo estándar. Ambos casos se consideran correctos.
- **Los datos del negocio que salen en el ticket son**: nombre comercial, dirección, teléfono, un
  bloque de texto arriba del detalle y otro abajo. El de abajo viene **precargado** con el aviso de
  que el ticket no tiene valor fiscal y cómo pedir factura; se siembra solo donde no hay nada
  configurado, para no pisarle el texto a un negocio que ya puso el suyo. Se excluyen deliberadamente los datos fiscales (RFC, régimen, folios): un ticket
  de venta no es un comprobante fiscal, y la facturación es un problema aparte con sus propias
  reglas.
- **Un solo logo y un solo juego de datos por empresa.** No hay logos por sucursal ni por tipo de
  ticket.
- **El logo se ve monocromo en el papel** porque la impresora es térmica. La conversión la hace el
  equipo de impresión, no el POS; lo que el POS garantiza es que la imagen entre en el ancho del
  ticket.
- **La reimpresión no tiene límite de cantidad ni requiere autorización especial**, pero queda
  marcada en el papel. Si más adelante se necesita controlarla, el marcado ya deja la evidencia.
- **El pedido individual ya se puede consultar completo con sus líneas**, así que la reimpresión no
  necesita guardar una copia del ticket.
- **La impresión automática solo sirve si el navegador está configurado para imprimir sin
  diálogo.** Con el diálogo activo, cada venta abriría una ventana modal que alguien tiene que
  cerrar: sería peor que el toque que vino a quitar. La opción se puede encender igual —el sistema
  no adivina cómo se lanzó el navegador—, pero el equipo tiene que estar preparado.
- **Esta feature no imprime comandas de cocina.** El ticket de venta sale por la impresora del
  operador; la comanda es otro flujo, con otra impresora y otro disparo, y está fuera de alcance.

## Out of Scope

- Impresión por agente local, extensión de navegador o comandos ESC/POS crudos.
- Impresora de cocina, comandas y disparo automático al confirmar el pedido.
- Apertura del cajón de dinero.
- Cualquier cosa que requiera instalar software en el equipo del operador.
- Comprobantes fiscales (facturación electrónica).
- Personalización del diseño del ticket más allá del logo y los datos del negocio.
