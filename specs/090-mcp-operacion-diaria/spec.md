# Feature Specification: MCP para la operación diaria (paquete 1 del #289)

**Feature Branch**: `090-mcp-operacion-diaria`

**Created**: 2026-11-22

**Status**: Draft

**Input**: Paquete 1 del #289 (con el barrido del 2026-11-22): que el servicio de operación diaria
—un agente que trabaja con un token del MCP— deje de abrir la pantalla web para capturar pedidos,
corregir cobros, cambiar costos, recetas, modificadores y productos. Incluye los tres defectos
reportados en el issue.

## Contexto

Hoy el agente de operación usa la API REST de la pantalla (con una sesión web) o un navegador
automatizado para lo que el MCP no cubre. Eso cuesta sesiones que caducan, selectores frágiles y,
sobre todo, escrituras **sin** vista previa, sin clave de idempotencia y sin el rastro de «lo hizo
el token X». Dos de los defectos de abajo salieron justo de ese camino: un `PATCH` parcial que borró
dos precios y una herramienta que negó la existencia de una opción que sí existe.

Cada herramienta nueva es un envoltorio de una operación que la pantalla **ya** hace: misma regla,
mismo permiso, mismos rechazos. Lo propio del MCP es la vista previa, la clave de idempotencia, el
alcance del token y la bitácora.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Los tres defectos reportados dejan de pasar (Priority: P1)

Quien configura el menú (por la pantalla o por el MCP) cambia el nombre de un extra sin tocar su
precio; el agente cambia una opción de un grupo inactivo sin que le digan que no existe; y quien
revisa los productos ve cuáles tienen ligado un grupo apagado, que en la pantalla de venta no
aparece.

**Why this priority**: son defectos que ya costaron dinero (dos precios en cero) y confusión en el
mostrador («la opción no existe»). Se pueden subir antes que lo demás.

**Independent Test**: un cambio de solo nombre deja el precio y el tope como estaban; la
herramienta de extras encuentra una opción de un grupo inactivo; la lista de productos marca el
producto con grupo inactivo, y archivar un grupo ligado avisa a cuántos productos les deja de salir.

**Acceptance Scenarios**:

1. **Given** un extra con precio $15 y tope 2, **When** se manda un cambio con solo el nombre,
   **Then** el nombre cambia y el precio sigue en $15 y el tope en 2.
2. **Given** un cambio con solo el precio o solo el tope, **When** se aplica, **Then** cambia solo
   ese campo.
3. **Given** una opción activa de un grupo inactivo, **When** el agente la edita por el MCP,
   **Then** la encuentra, la edita y la vista previa avisa que su grupo está inactivo y por eso no
   sale en la pantalla de venta.
4. **Given** un producto con un grupo inactivo ligado, **When** alguien abre la lista de productos,
   **Then** el renglón del producto dice que tiene un grupo inactivo.
5. **Given** un grupo ligado a 3 productos, **When** alguien lo archiva, **Then** el aviso dice que
   deja de salir en esos 3 productos (y ofrece deshacer, como hoy).

---

### User Story 2 - Capturar un pedido completo sin la pantalla (Priority: P1)

El agente captura un pedido de plataforma que el equipo no registró: arma la cuenta (tipo de
servicio, plataforma, folio de la plataforma, cliente, renglones con sus extras), ve en la vista
previa el total con los precios de esa plataforma —y, por renglón, si el precio es el propio de la
plataforma o el precio base más el margen de la plataforma—, la manda a cocina, la entrega, le pone
el descuento de la promoción y la cobra con el método de esa plataforma.

**Why this priority**: es lo que más se hace hoy por REST con la sesión web, y cada paso escribe
dinero.

**Independent Test**: con la base restaurada, un pedido de plataforma pasa por los seis pasos por
MCP y queda entregado y cobrado, con el mismo total que la pantalla habría cobrado.

**Acceptance Scenarios**:

1. **Given** renglones válidos y una plataforma, **When** el agente pide la vista previa de la
   cuenta, **Then** recibe el subtotal, el descuento, el total y, por renglón y por extra, el precio
   unitario y de dónde salió (propio de la plataforma, o base × (1 + margen)), sin que se cree nada.
2. **Given** un producto sin precio propio en esa plataforma, **When** se pide la vista previa,
   **Then** el renglón lo marca y lo dice en una nota, para que el agente lo compare con la app de
   la plataforma antes de enviar.
3. **Given** la cuenta creada, **When** se envía, **Then** nace el pedido con el total que dijo la
   vista previa; enviarla otra vez no duplica el pedido.
4. **Given** una cuenta abierta por error, **When** se descarta, **Then** desaparece sin crear
   pedido.
5. **Given** un pedido enviado, **When** se entrega, se le pone descuento y se cobra con «todo lo
   que falta», **Then** queda entregado y saldado; un reintento con la misma clave no cobra dos
   veces.
6. **Given** un pedido de plataforma, **When** se intenta cobrar con un método que no es el de su
   plataforma, **Then** se rechaza igual que en la pantalla.

---

### User Story 3 - Corregir un cobro ya hecho (Priority: P1)

El agente corrige una venta cobrada con la terminal equivocada, anula un pago capturado con el
método equivocado y lo vuelve a cobrar con el bueno, o reembolsa un pedido entregado; antes de
reembolsar ve cuánto se devolvería, en qué terminal y si pedirá folio.

**Why this priority**: pasa cada semana (venta en la terminal equivocada del 11-oct) y hoy solo se
hace por la pantalla.

**Independent Test**: anular un pago lo saca de los pagos y deja ese monto por cobrar; cambiar la
terminal la mueve en el corte; reembolsar saca el dinero del corte, con la vista previa igual a lo
que la pantalla mostraría.

**Acceptance Scenarios**:

1. **Given** un pago de un turno abierto, **When** se anula con motivo, **Then** sale de los pagos,
   queda en la bitácora y su monto vuelve a quedar por cobrar.
2. **Given** un pago con tarjeta, **When** se cambia de terminal con motivo, **Then** el corte lo
   cuenta en la terminal nueva.
3. **Given** un pedido entregado, **When** se pide la vista previa del reembolso, **Then** se ve
   cuánto queda por devolver, en qué terminal y si pedirá folio; al aplicar sin folio cuando se
   pide, se rechaza como en la pantalla.
4. **Given** un usuario sin el permiso de anular pagos, **When** el agente intenta cualquiera de las
   tres, **Then** la herramienta ni se ofrece, y si se llama, se niega.

---

### User Story 4 - Corregir el costo de un insumo con su motivo (Priority: P1)

El agente compara costos contra la última compra y corrige los que difieren, dejando el motivo.
Antes de aplicar ve el costo actual, el nuevo, las últimas entradas del historial y las recetas que
usan el insumo.

**Why this priority**: es el caso que disparó el barrido (6 de 23 ramen) y hoy no hay forma por MCP
sin meter mercancía ni crear un gasto.

**Independent Test**: tras aplicar, el historial del insumo muestra la corrección con el costo
anterior, el nuevo, quién y el motivo; los productos que lo llevan recalculan su costo.

**Acceptance Scenarios**:

1. **Given** un insumo con costo $10/g, **When** se pide la vista previa con $12 y un motivo,
   **Then** se ven el costo actual, el nuevo, el historial reciente y las recetas que lo usan.
2. **Given** la llamada sin motivo, **When** se intenta aplicar, **Then** se rechaza diciendo que
   falta el motivo.
3. **Given** un insumo preparado, **When** se intenta corregir su costo, **Then** se rechaza como en
   la pantalla (su costo sale de su receta).
4. **Given** una corrección hecha desde la pantalla sin motivo, **When** se lee el historial,
   **Then** sale igual que antes, sin motivo.

---

### User Story 5 - Recetas de productos y extras (Priority: P1)

El agente lee y guarda lo que resta del almacén un producto o un extra: insumos con cantidad y
unidad, el producto que es (un extra), los productos que lleva (un paquete) o «no se cuenta». Por
omisión **no** copia la receta a los extras que se llaman igual en otros grupos; solo lo hace si lo
pide explícitamente. Si el producto tiene existencia propia y el cambio la deja, la vista previa
dice cuánta existencia tiene el producto y el insumo, y qué cifra hace falta mandar.

**Why this priority**: recetas nuevas cada semana (11-oct, 30-oct, 3-nov) y la trampa de «También
en…» que copió recetas sin querer.

**Independent Test**: guardar una receta por MCP deja la misma composición que la pantalla y no
toca los extras homónimos salvo que se pida.

**Acceptance Scenarios**:

1. **Given** un producto con receta, **When** se lee, **Then** se ven sus renglones, su estado
   (estimada o confirmada), si tiene existencia propia y los extras con el mismo nombre.
2. **Given** una receta nueva para un extra con homónimos, **When** se aplica sin
   `also_in_same_name`, **Then** solo cambia ese extra; con `also_in_same_name: true`, cambian
   también los homónimos, y la vista previa los nombra.
3. **Given** un producto con existencia propia que pasa a receta y el insumo ya tiene existencia,
   **When** se pide la vista previa sin decir cuántas hay de verdad, **Then** la vista previa dice
   la existencia del producto y del insumo y que falta `actual_stock`; aplicar sin ella se rechaza
   con ese mismo mensaje.
4. **Given** que otra persona guardó la receta entre la vista previa y el aplicar, **When** se
   aplica, **Then** se rechaza diciendo que la receta cambió y que se vuelva a pedir la vista previa.

---

### User Story 6 - Grupos de modificadores, opciones y su liga con productos (Priority: P1)

El agente crea un grupo («Leche (bebida 16 oz)», obligatorio), lo edita (nombre, mínimo y máximo
por omisión, activo), crea y edita opciones (nombre, lo que suma al precio, tope por renglón,
activa), y liga o desliga grupos a productos con su título, mínimo, máximo y orden. Puede leer qué
grupos tiene un producto, con cuáles están inactivos.

**Why this priority**: altas y archivos de opciones cada semana (11-oct: 6 archivadas, 5 altas, 1
reactivada; 3-nov: grupo nuevo obligatorio).

**Independent Test**: un grupo creado y ligado por MCP aparece en la pantalla de venta del producto
con su mínimo y máximo; los rechazos (nombre repetido en el grupo, mínimo mayor que máximo) son los
de la pantalla.

**Acceptance Scenarios**:

1. **Given** un nombre de opción que ya existe en el grupo, **When** se intenta crear, **Then** se
   rechaza como en la pantalla.
2. **Given** mínimo mayor que máximo, **When** se intenta crear o ligar el grupo, **Then** se
   rechaza.
3. **Given** un grupo inactivo, **When** se pide la vista previa de ligarlo a un producto, **Then**
   avisa que no saldrá en la pantalla de venta mientras esté inactivo.
4. **Given** un grupo ligado a productos, **When** se pide la vista previa de desactivarlo,
   **Then** nombra los productos en los que deja de salir.

---

### User Story 7 - Dar de alta y editar productos (Priority: P1)

El agente da de alta un producto (nombre, categoría, precio, costo manual, si lleva existencia
propia, favorito) y edita nombre, categoría, precio, costo manual o «de receta», favorito y si
está activo. Solo cambia lo que manda.

**Why this priority**: alta de producto genérico para plataformas y de reventa; hoy `product_update`
no cambia el nombre.

**Independent Test**: un producto creado por MCP aparece en la lista con lo que se mandó; editar
solo el nombre deja intactos precio, categoría, costo, horario y favorito.

**Acceptance Scenarios**:

1. **Given** un nombre que ya existe, **When** se crea, **Then** se rechaza como en la pantalla, y
   la vista previa nombra antes los productos parecidos.
2. **Given** una categoría de otra empresa o inexistente, **When** se crea o se mueve, **Then** se
   rechaza.
3. **Given** solo `name`, **When** se aplica la edición, **Then** lo demás queda como estaba.

### Edge Cases

- **Campo ausente contra campo vacío**: en toda edición, un campo que no se manda no cambia. Un
  precio `"0"` sí cambia a cero (es un valor). Mandar un nombre vacío se rechaza, no se ignora.
- **Reintento con la misma clave**: devuelve lo guardado sin repetir (cobro, envío, reembolso).
  Misma clave con otros argumentos: se rechaza.
- **Cuenta enviada dos veces** (otra tableta la envió): enviar de nuevo devuelve el mismo pedido.
- **Cobro sin caja abierta, o con el método de otra plataforma**: lo rechaza la operación de la
  pantalla, y el agente recibe la causa y qué hacer.
- **Pago de un turno ya cerrado**: anular se rechaza como en la pantalla.
- **Identificadores de cuenta**: el agente solo puede mandar el id de cuenta que le dio el servidor
  (un UUID); cualquier otro texto se rechaza antes de despachar.
- **Precio de un extra o producto cuyo grupo o producto se desactiva con la cuenta abierta**: la
  vista previa lo marca como no disponible y no suma, igual que la pantalla.
- **Receta que cambia mientras se previsualiza**: aplicar compara contra la versión vista y se
  niega si cambió.
- **Opción de un grupo inactivo**: se puede editar; su vista previa avisa del grupo.
- **Token sin el alcance de escritura**: la herramienta no se ofrece; llamada directa se niega.
- **Costo con más de 6 decimales o negativo**: se rechaza en la frontera.
- **Motivo de costo de más de 200 letras**: se rechaza.

## Requirements *(mandatory)*

### Functional Requirements

**Defectos**

- **FR-001**: Una edición de un extra (por pantalla o por MCP) MUST cambiar solo los campos que
  trae; nombre, lo que suma al precio y tope por renglón se conservan si no vienen.
- **FR-002**: La edición de un extra por MCP MUST encontrar la opción aunque su grupo esté inactivo,
  y su vista previa MUST avisar que el grupo está inactivo.
- **FR-003**: La lista de productos de la pantalla MUST marcar el producto que tiene al menos un
  grupo inactivo ligado.
- **FR-004**: Archivar un grupo ligado a productos MUST avisar a cuántos productos les deja de salir.

**Pedidos**

- **FR-005**: `order_draft_create` MUST abrir una cuenta con cabecera (tipo de servicio, plataforma,
  folio de plataforma, cliente, costo de envío, descuento) y renglones (producto, cantidad, extras
  con cantidad y porción, notas), con la misma validación que la pantalla.
- **FR-006**: La vista previa de `order_draft_create` MUST mostrar el total calculado por el
  servidor con la lista de precios de la cuenta, sin crear nada, y por renglón y extra el precio
  unitario y su origen: propio de la plataforma, base × (1 + margen), o el del mostrador.
- **FR-007**: `order_draft_send` y `order_draft_discard` MUST enviar o descartar la cuenta como la
  pantalla; enviar es idempotente.
- **FR-008**: `order_deliver`, `order_discount_set` y `order_pay` MUST entregar, poner o quitar el
  descuento (monto o porcentaje) y cobrar (un monto o todo lo que falta, con método, propina,
  referencia y terminal) como la pantalla. La vista previa del cobro MUST mostrar cuánto cobraría
  según el servidor.
- **FR-009**: Un cobro aplicado MUST quedar protegido contra el reenvío: dos llamadas con la misma
  clave cobran una vez.

**Correcciones de cobro**

- **FR-010**: `order_payment_void` MUST anular un pago con motivo obligatorio.
- **FR-011**: `order_payment_terminal_set` MUST cambiar la terminal de un pago con motivo
  obligatorio.
- **FR-012**: `order_refund` MUST reembolsar (todo o un monto, de la cuenta o de un renglón) con
  motivo y folio cuando se pide; su vista previa MUST traer lo que la pantalla muestra antes de
  reembolsar (cuánto queda por devolver, terminal y si pide folio).
- **FR-013**: Cambiar el método de un pago MUST hacerse anulando y volviendo a cobrar, como en la
  pantalla; no hay herramienta propia.

**Costo, recetas, modificadores y productos**

- **FR-014**: `ingredient_cost_set` MUST corregir el costo por unidad base de un insumo con motivo
  obligatorio; el motivo MUST quedar en el historial de costos y en la bitácora. La pantalla sigue
  pudiendo corregir sin motivo.
- **FR-015**: La vista previa de `ingredient_cost_set` MUST mostrar el costo actual, el nuevo, las
  últimas entradas del historial y las recetas que usan el insumo.
- **FR-016**: `recipe_get` MUST leer la composición de un producto o un extra.
- **FR-017**: `recipe_set` MUST guardarla con la misma regla que la pantalla, comparando contra la
  versión que se leyó; `also_in_same_name` MUST ser falso por omisión.
- **FR-018**: Con existencia propia, la vista previa de `recipe_set` MUST decir la existencia del
  producto y del insumo y qué cifra falta; el rechazo al aplicar MUST decir lo mismo.
- **FR-019**: `modifier_group_create`, `modifier_group_update`, `modifier_option_create` y
  `modifier_option_update` MUST cubrir nombre, mínimo y máximo por omisión y activo (grupo); nombre,
  lo que suma al precio, tope por renglón y activa (opción). Solo cambia lo que se manda.
- **FR-020**: `product_groups_get`, `product_group_link` y `product_group_unlink` MUST leer, ligar
  (con título, mínimo, máximo y orden; sin mínimo y máximo hereda los del grupo) y desligar grupos
  de un producto.
- **FR-021**: `product_create` MUST dar de alta un producto (nombre, categoría, precio, costo manual
  opcional, existencia propia, favorito), con vista previa de los parecidos.
- **FR-022**: `product_update` MUST editar nombre, categoría, precio, costo (manual con monto, o de
  receta), favorito y activo, sin tocar lo que no se manda.
- **FR-023**: `categories_list` MUST listar las categorías con las que se da de alta o mueve un
  producto.

**Reglas comunes**

- **FR-024**: Toda herramienta de escritura MUST hacer vista previa por omisión, exigir clave de
  idempotencia al aplicar, pedir el mismo permiso que su pantalla y dejar su llamada en `audit_list`.
- **FR-025**: Toda herramienta MUST rechazar lo mismo que su pantalla; el rechazo MUST llegar al
  agente con la causa y la corrección.
- **FR-026**: Lo que queda fuera de este paquete MUST anotarse en el #289.

### Key Entities

- **Corrección de costo**: insumo, costo anterior, costo nuevo, quién, cuándo y —nuevo— motivo
  (opcional desde la pantalla, obligatorio desde el MCP).
- **Cuenta en captura**: la cuenta de la pantalla Vender antes de enviarse; la identifica un UUID.
- **Liga producto–grupo**: título, mínimo, máximo, si hereda del grupo, orden.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Las acciones del alcance (capturar y cobrar un pedido de plataforma, corregir un cobro,
  costo, receta, grupos, opciones y productos) se completan sin abrir la pantalla web ni la API REST
  con sesión: 0 de 9 flujos requieren navegador en la prueba de punta a punta.
- **SC-002**: Un pedido de prueba capturado por MCP cobra el mismo total que la vista previa dijo, al
  centavo.
- **SC-003**: Cada herramienta nueva tiene al menos una prueba de rechazo que coincide con la de su
  pantalla, y una de permiso.
- **SC-004**: Un cambio de solo nombre de un extra deja su precio y su tope intactos (prueba de
  regresión en verde, vista primero en rojo).
- **SC-005**: Ninguna llamada aplicada por MCP falta en `audit_list`.

## Assumptions

- **Duda 1 del issue**: herramienta aparte, `ingredient_cost_set`, con motivo; `ingredient_update`
  no gana campo de costo. Así el costo deja rastro con su motivo y la ficha no mezcla dos cosas.
- **Dudas 2 y 3 (empleados y nómina)**: fuera de este paquete; se deciden en el que los incluya.
- **Cambiar de método** se hace como la pantalla: anular y volver a cobrar. Una herramienta que hiciera
  las dos cosas no sería atómica y duplicaría la regla del cobro.
- **«Orden» del producto**: el catálogo no tiene orden propio de producto (el POS ordena por
  categoría y popularidad); el orden que sí existe es el de los grupos dentro de un producto, que va
  en `product_group_link`. Se anota en el issue.
- **Duplicar producto** queda para un paquete siguiente (el issue lo menciona en P1-4, el encargo de
  este paquete no).
- Archivar y reactivar grupos y opciones es `active` en sus herramientas de edición, no herramientas
  aparte: es el mismo campo y la misma ruta que la pantalla.
- El alcance del token: pedidos y cobros en `write:orders`; catálogo (grupos, opciones, productos,
  recetas) en `write:catalog`; costo en `write:inventory`.
- Fuera de este paquete y anotado en el #289: cancelar o mover renglones de un pedido vivo, folio de
  plataforma, aceptar o rechazar pedidos de plataforma, liquidaciones, merge/archivo/unidad de
  insumos, costeo y margen, avisos de inventario, correcciones de caja, propinas, asistencia,
  reportes de solo lectura, emparejamiento de plataformas, categorías y lugares, empleados y nómina.
