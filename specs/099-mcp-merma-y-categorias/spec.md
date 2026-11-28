# Feature Specification: Merma esperada y categorías del menú desde el MCP

**Feature Branch**: `099-mcp-merma-y-categorias`

**Created**: 2026-11-28

**Status**: Draft

**Input**: Solicitudes de la sesión de finanzas (buzón pos-finanzas, 25 y 27-nov-2026; respuesta del POS el 28-nov), parte del issue #289. Dos capacidades nuevas del servidor MCP: (1) cambiar la merma esperada de un insumo con motivo y vista previa; (2) dar de alta y editar categorías del menú (nombre, activa o inactiva, padre).

## User Scenarios & Testing *(mandatory)*

Quien usa estas herramientas es un agente que trabaja para el negocio (hoy, la sesión de finanzas) con un token del MCP; detrás siempre hay una persona que decide. Por eso cada escritura tiene vista previa por omisión y solo se aplica cuando se pide explícitamente.

### User Story 1 - Cambiar la merma esperada de un insumo (Priority: P1)

Finanzas detecta que un insumo se pierde más (o menos) de lo que el sistema supone al costear y quiere ajustar su porcentaje de merma esperada sin abrir la pantalla. Pide la vista previa, ve el porcentaje que tiene hoy y el nuevo, cómo cambia el costo efectivo del insumo y qué recetas y productos se encarecen o abaratan, y lo aplica con un motivo que queda en la bitácora.

**Why this priority**: es lo que pidió el dueño a través de finanzas y hoy no tiene otro camino que la pantalla; afecta directamente el costo de las recetas y los reportes de margen.

**Independent Test**: con un insumo que lleva dos recetas, pedir la vista previa de 7 % → 8 %, comprobar que no cambió nada; aplicarla con una clave nueva y comprobar el porcentaje guardado, el costo recalculado de los productos y el renglón de bitácora con el motivo.

**Acceptance Scenarios**:

1. **Given** un insumo con merma esperada de 7 % y costo por unidad base conocido, **When** el agente pide la vista previa con 8 %, **Then** recibe el porcentaje actual (7 %), el nuevo (8 %), el costo efectivo antes y después con la fórmula costo × (1 + %/100), y la lista de recetas y productos que usan el insumo, sin que se guarde nada.
2. **Given** la misma vista previa, **When** el agente aplica con `dry_run: false`, una clave de idempotencia y un motivo, **Then** el porcentaje queda en 8 %, el costo de los productos que lo usan se recalcula, y la bitácora registra el cambio con el valor anterior, el nuevo y el motivo.
3. **Given** un insumo sin merma capturada (nulo), **When** el agente pide la vista previa, **Then** el porcentaje actual se informa como «sin captura» (distinto de 0 %) y se dice de dónde sale hoy la merma que se aplica (la del producto o la general).
4. **Given** un insumo con 5 %, **When** el agente manda quitar la captura (volver a «sin captura»), **Then** la vista previa dice que pasa a heredar la del producto o la general, y al aplicar el insumo queda sin captura, no en 0 %.
5. **Given** un insumo con 5 %, **When** el agente manda 0, **Then** queda capturado en 0 % (no hereda).
6. **Given** una aplicación ya hecha, **When** el agente repite la misma llamada con la misma clave, **Then** recibe el mismo resultado y no se duplica el cambio ni el renglón de bitácora.

---

### User Story 2 - Apagar o editar una categoría del menú (Priority: P1)

El dueño pidió apagar una categoría que ya no se usa (todos sus productos están inactivos). Finanzas, desde el MCP, pide la vista previa de desactivarla, ve que no tiene productos activos ni subcategorías activas, y la aplica. La pantalla de venta deja de mostrarla. Del mismo modo puede renombrar una categoría, moverla bajo otra o volver a activarla.

**Why this priority**: es el caso real que está esperando (una categoría vacía en la pantalla de venta) y hoy el MCP no tiene cómo hacerlo.

**Independent Test**: con una categoría cuyos productos están todos inactivos, desactivarla desde el MCP y comprobar que ya no aparece en el menú de la pantalla de venta ni en la lista de categorías activas, y que la bitácora registra quién y cuándo.

**Acceptance Scenarios**:

1. **Given** una categoría sin productos activos y sin subcategorías activas, **When** el agente pide desactivarla, **Then** la vista previa dice cuántos productos inactivos conserva y que dejará de salir en la venta; al aplicar, deja de aparecer en el menú.
2. **Given** una categoría con productos activos, **When** el agente pide desactivarla, **Then** se rechaza diciendo cuántos productos activos tiene y qué hacer (moverlos a otra categoría o desactivarlos primero).
3. **Given** una categoría raíz con subcategorías activas, **When** el agente pide desactivarla, **Then** se rechaza nombrando las subcategorías activas.
4. **Given** una categoría inactiva, **When** el agente la reactiva, **Then** vuelve a salir en la venta; si su padre está inactivo, se rechaza diciendo que primero se active el padre.
5. **Given** una categoría, **When** el agente la renombra con un nombre que ya usa otra categoría bajo el mismo padre, **Then** se rechaza diciendo cuál categoría ya tiene ese nombre.
6. **Given** una categoría sin subcategorías, **When** el agente la mueve bajo una categoría raíz activa, **Then** queda como subcategoría de esa raíz.

---

### User Story 3 - Dar de alta una categoría del menú (Priority: P2)

Al preparar productos nuevos, el agente necesita una categoría que no existe. Pide la vista previa del alta (con nombre y, si aplica, padre), ve las categorías de nombre parecido que ya existen para no duplicar, y la crea.

**Why this priority**: completa la capacidad (hoy `product_create` exige una categoría existente), pero no hay un caso real esperando.

**Independent Test**: crear una subcategoría bajo una raíz existente y comprobar que aparece en la lista de categorías y que un producto puede darse de alta en ella.

**Acceptance Scenarios**:

1. **Given** una raíz activa, **When** el agente crea «Tés» bajo ella, **Then** la vista previa pone primero las categorías de nombre parecido y, al aplicar, la categoría existe, activa, bajo esa raíz.
2. **Given** ya existe «Tés» bajo esa raíz, **When** el agente crea otra «tés», **Then** se rechaza (el nombre no distingue mayúsculas) diciendo el id de la existente.
3. **Given** el id de una categoría de otra empresa como padre, **When** el agente pide crear, **Then** se rechaza como si no existiera, sin revelar nada de la otra empresa.

---

### Edge Cases

- **Nulo contra cero en la merma**: «sin captura» y «capturado en 0» son estados distintos en la entrada, en la vista previa y en la bitácora. Mandar el campo vacío o ausente no puede confundirse con quitar la captura: quitarla se pide de forma explícita.
- **Merma fuera de rango**: un porcentaje negativo, de 100 o más, con más de dos decimales o que no es número se rechaza antes de la vista previa con el rango permitido (0 a menos de 100, dos decimales).
- **Mismo porcentaje**: aplicar el mismo valor que ya tiene no cambia nada ni deja renglón de bitácora; la vista previa lo dice.
- **Costo efectivo cuando el insumo no tiene merma propia**: el costo efectivo del insumo depende del producto que lo usa (hereda la del producto o la general); la vista previa lo muestra por producto y no inventa un único número.
- **Insumo sin costo conocido**: la vista previa lo dice; el porcentaje se puede guardar igual.
- **Insumo inactivo o de otra empresa**: el de otra empresa no existe para el token; el inactivo se puede ajustar (su costo sigue sirviendo a recetas viejas) y la vista previa avisa que está inactivo.
- **Padre de otra empresa**: se rechaza como inexistente, al crear y al mover.
- **Ciclos y profundidad**: la pantalla de venta muestra solo dos niveles (raíz y subcategoría). Una categoría no puede ser su propio padre, el padre debe ser una raíz, y una categoría que tiene subcategorías no puede volverse subcategoría. Con eso no hay ciclos posibles.
- **Padre inactivo**: una categoría activa no se mueve ni se crea bajo un padre inactivo.
- **Desactivar con productos activos o subcategorías activas**: se rechaza con la cifra y la salida (ver US2).
- **Nombre vacío, con solo espacios o demasiado largo**: se rechaza. Los espacios de los extremos se quitan.
- **Editar sin cambiar nada**: se rechaza diciendo que no hay nada que cambiar (no se finge un guardado).
- **Desactivar una categoría que ya está inactiva** o reactivar una activa: no cambia nada ni deja bitácora; la vista previa lo dice.
- **Categorías inactivas en la lista**: para poder reactivar, el agente necesita ver las inactivas; la lista de categorías permite pedirlas.
- **Repetición con la misma clave y argumentos distintos**: se rechaza (comportamiento común de las escrituras del MCP).
- **Token sin el permiso o sin el alcance de escritura**: se rechaza igual que hoy con las demás herramientas de catálogo.

## Requirements *(mandatory)*

### Functional Requirements

**Merma esperada de un insumo**

- **FR-001**: El MCP MUST permitir cambiar la merma esperada de un insumo a un porcentaje de 0 a menos de 100, con hasta dos decimales, o quitar la captura (dejarlo «sin captura»), indicando un motivo obligatorio de 1 a 200 letras.
- **FR-002**: La herramienta MUST distinguir en su entrada, su vista previa, su respuesta y la bitácora entre «sin captura» y «capturado en 0 %».
- **FR-003**: La vista previa MUST mostrar: porcentaje actual y nuevo (o «sin captura»), de dónde sale la merma que se aplica cuando no hay captura (producto o general), el costo por unidad base, el costo efectivo antes y después (costo × (1 + %/100)), y las recetas y productos que usan el insumo con su costo antes y después.
- **FR-004**: La fórmula de costo efectivo y el orden de herencia (insumo → producto → general) MUST quedar como están hoy.
- **FR-005**: Aplicar MUST recalcular el costo de los productos, extras y preparados que usan el insumo, igual que cuando se cambia desde la pantalla.
- **FR-006**: El cambio MUST quedar en la bitácora existente de merma esperada del insumo con valor anterior, valor nuevo, motivo, quién (la persona dueña del token) y que vino del MCP.
- **FR-007**: La herramienta MUST exigir el mismo permiso que la pantalla para cambiar la merma de un artículo.

**Categorías del menú**

- **FR-010**: El MCP MUST permitir dar de alta una categoría con nombre y, opcionalmente, una categoría raíz como padre. Nace activa.
- **FR-011**: El MCP MUST permitir editar una categoría: nombre, activa o inactiva, y padre (moverla bajo una raíz o volverla raíz). Solo cambia lo que se manda.
- **FR-012**: El nombre MUST ser único bajo el mismo padre dentro de la empresa, sin distinguir mayúsculas; el rechazo nombra la categoría que ya lo tiene.
- **FR-013**: El padre MUST ser una categoría raíz de la misma empresa; una categoría con subcategorías no puede tener padre; una categoría no puede ser su propio padre.
- **FR-014**: Desactivar una categoría MUST rechazarse si tiene productos activos (diciendo cuántos) o subcategorías activas (nombrándolas).
- **FR-015**: Reactivar una subcategoría, o crear o mover una categoría activa, bajo un padre inactivo MUST rechazarse.
- **FR-016**: La vista previa del alta MUST poner primero las categorías de nombre parecido que ya existen (activas e inactivas). La de la edición MUST mostrar el estado actual, el nuevo, cuántos productos activos e inactivos tiene y sus subcategorías.
- **FR-017**: La lista de categorías del MCP MUST poder incluir las inactivas, marcadas como tales, para poder reactivarlas.
- **FR-018**: Todo cambio de categoría MUST reflejarse en el menú de la pantalla de venta y del menú público en la siguiente lectura, como cualquier cambio de catálogo.
- **FR-019**: El alta y cada edición de categoría MUST dejar renglón en la bitácora con el estado anterior y el nuevo, quién y que vino del MCP.
- **FR-020**: Las herramientas de categoría MUST exigir el permiso de configurar el menú, el mismo que las demás escrituras de catálogo.

**Comunes**

- **FR-030**: Las tres herramientas MUST comportarse como las demás escrituras del MCP: vista previa por omisión, aplicar solo con `dry_run: false` y una clave de idempotencia, misma respuesta al repetir con la misma clave, y rechazo si la clave se reusa con otros argumentos.
- **FR-031**: Un id de otra empresa (insumo, categoría o padre) MUST rechazarse como inexistente, en la vista previa y al aplicar, sin revelar nada de la otra empresa; igual con una conexión reciclada o sin empresa.
- **FR-032**: Los mensajes de rechazo MUST decirle al agente qué hacer para corregir (qué herramienta usar o qué dato cambiar), en español.

### Key Entities

- **Insumo (artículo)**: su porcentaje de merma esperada, nulo o de 0 a menos de 100; su costo por unidad base; las recetas que lo usan.
- **Categoría del menú**: nombre, padre opcional (solo raíces pueden ser padre), activa o inactiva; agrupa productos.
- **Renglón de bitácora**: acción, entidad, valor anterior y nuevo, motivo cuando aplica, persona y origen.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Finanzas puede cambiar la merma esperada de un insumo y apagar una categoría sin abrir la pantalla, cada uno en dos llamadas (vista previa y aplicar).
- **SC-002**: En el 100 % de los casos de prueba, la vista previa no cambia ningún dato y el resultado aplicado coincide con lo que la vista previa anunció.
- **SC-003**: Ningún caso de prueba con un id de otra empresa, conexión reciclada o sin empresa lee ni escribe datos ajenos.
- **SC-004**: Ningún cambio desde el MCP deja una categoría activa con padre inactivo, una subcategoría de tercer nivel o un ciclo.
- **SC-005**: Todo cambio aplicado aparece en la bitácora con su valor anterior y nuevo; ninguna repetición con la misma clave deja un segundo renglón.

## Assumptions

- El orden de las categorías, su color, su foto y su familia no se editan aquí (la familia y la foto ya tienen herramienta; el orden es preferencia de cada pantalla).
- No se borra ninguna categoría: se desactiva. Un producto inactivo puede quedarse en una categoría inactiva.
- La merma esperada del producto y la general no entran en este alcance; solo la del insumo.
- La pantalla no cambia: lo que se agrega al servidor para el MCP queda disponible para una pantalla futura, pero no se construye.
- Fuera de alcance: el #392 (qué muestra la pantalla de venta con categorías vacías o solo con subcategorías) y el resto del #289.
- Desactivar una categoría no toca los menús ya publicados en las plataformas de reparto; esos se publican aparte.
