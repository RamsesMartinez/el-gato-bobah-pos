# Feature Specification: Salud del inventario

**Feature Branch**: `037-salud-del-inventario`
**Created**: 2026-10-13
**Status**: Draft
**Input**: Requerimiento de finanzas «inventario flexible y salud del inventario» (13-oct-2026) y la
auditoría de inventario del mismo día. Decisión del dueño (13-oct): el requerimiento se separa en
dos. Este spec es el 037; las compras que entran al almacén, las presentaciones de compra y el conteo
inicial van en el 038 (issue #60); los alérgenos, en el issue #59.

## Contexto

La auditoría del 13-oct, hecha a mano con consultas sobre un respaldo de producción, encontró que el
almacén no dice la verdad y que nadie lo veía:

- 31 productos activos se venden sin descontar nada del almacén: no tienen receta, ni existencia
  propia, ni piezas de paquete.
- 126 insumos están en negativo, porque ninguna compra entra al almacén (eso lo resuelve el 038).
- Un extra que representa un producto revendido («Coca Cola 355ml» en un grupo de refrescos) está
  activo sin receta ni producto ligado: si se vende dentro de un combo, no descuenta nada.
- La existencia propia de un producto no se puede cambiar desde ninguna pantalla: solo la fijan la
  importación inicial y los scripts.

Este spec convierte esa auditoría en algo que cualquier empresa puede ver y corregir sola, sin pedir
consultas a quien programa. El producto se vende a otras empresas: nada de esto puede depender de
nombres, marcas o reglas de un local.

## Clarifications

### Session 2026-10-13

- Q: Cuando un producto con existencia propia se convierte en receta de 1 pieza de un insumo nuevo, ¿qué pasa con su existencia? → A: Pasa al insumo nuevo con un movimiento que lo dice; el producto queda en cero.
- Q: Mientras ninguna compra entra al almacén (hasta el 038), ¿cómo se muestra «Insumos sin compras registradas», que listaría más de 100 insumos? → A: Un renglón resumen («118 de 130 insumos sin compras registradas») que con un toque se despliega en la lista completa; la exportación siempre trae la lista completa.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Ver qué está mal en el inventario, en una sola pantalla (Priority: P1)

Quien administra el negocio abre «Salud del inventario» y ve, agrupado por tipo de problema, todo lo
que hace que las existencias no cuadren: lo que se vende sin descontar, los insumos en negativo, los
insumos sin compras registradas, las recetas mal formadas y las recetas pendientes de confirmar.
Cada renglón dice qué es, cuánto pesa (unidades vendidas por canal en el periodo, existencia
actual) y lleva directo a donde se corrige.

**Why this priority**: Es lo que hoy costó una auditoría a mano. Sin verlo, nadie sabe qué
corregir, y el 038 (compras) no sirve si las ventas no descuentan.

**Independent Test**: Sobre una copia del respaldo de producción del 13-oct, la pantalla lista los
31 productos sin descuento y los 126 insumos en negativo de la auditoría, con las mismas unidades
vendidas por canal.

**Acceptance Scenarios**:

1. **Given** un producto activo sin receta, sin existencia propia y sin piezas de paquete que se vendió en el periodo, **When** se abre «Salud del inventario», **Then** aparece en «Se vende sin descontar» con sus unidades vendidas por canal (mostrador y cada plataforma).
2. **Given** un producto que no descuenta por sí mismo pero cuyo extra obligatorio sí descuenta (por ejemplo un tamaño o sabor con receta), **When** se abre la pantalla, **Then** no aparece como problema.
3. **Given** un extra activo sin receta ni producto ligado, que no está confirmado como «No gasta insumos», y se vendió en el periodo, **When** se abre la pantalla, **Then** aparece en «Se vende sin descontar» con sus unidades.
4. **Given** un insumo con existencia negativa, **When** se abre la pantalla, **Then** aparece en «Insumos en negativo» con su existencia y las recetas que lo usan.
5. **Given** una receta con un renglón en cantidad cero, con unidad de otro tipo que la del insumo, o que apunta a un insumo archivado o duplicado, **When** se abre la pantalla, **Then** aparece en «Recetas mal formadas» con el renglón señalado.
6. **Given** recetas «Por revisar», **When** se abre la pantalla, **Then** se ve cuántas son y un toque lleva a Menú › Recetas filtrado en «Por revisar».
7. **Given** una empresa sin ningún problema, **When** se abre la pantalla, **Then** dice que el inventario está sano, sin listas vacías que parezcan error.

---

### User Story 2 - Exportar la salud del inventario (Priority: P2)

Quien administra exporta lo que ve a un archivo que abre en una hoja de cálculo, para cruzarlo con
las compras o mandarlo a quien lleva las finanzas.

**Why this priority**: Hoy finanzas recibe estas listas a mano. Exportar quita ese paso, pero la
pantalla ya entrega el valor sin él.

**Independent Test**: Exportar con el periodo del 5 al 13-oct y obtener un archivo con un renglón
por problema, con las mismas cifras que la pantalla.

**Acceptance Scenarios**:

1. **Given** la pantalla con un periodo elegido, **When** se exporta, **Then** el archivo trae cada problema con su tipo, el producto, extra o insumo, su estado, sus unidades por canal y su existencia, iguales a lo que muestra la pantalla.
2. **Given** un nombre con acentos o comas, **When** se abre el archivo en una hoja de cálculo, **Then** se lee igual que en la pantalla.

---

### User Story 3 - Decidir cómo descuenta cada producto, desde la pantalla (Priority: P1)

Quien configura el menú elige, para cada producto, cómo descuenta del almacén: **sin control**
(«No gasta insumos»), **existencia propia** (cuenta el producto mismo por pieza) o **receta con
insumos**. Un producto que se compra hecho se configura como receta de uno o más insumos con
cantidad editable, de 1 pieza por omisión, para que sirva en combos y en variaciones (con un extra,
sin empaque, otra presentación).

**Why this priority**: Es lo que corrige la mayoría de lo que la Historia 1 encuentra. Hoy la
existencia propia solo se cambia con scripts, y un producto revendido no tiene forma guiada de
volverse receta.

**Independent Test**: Tomar un producto que se compra hecho y no descuenta, convertirlo en receta de
1 pieza de un insumo nuevo con su nombre, vender uno y ver que el insumo baja 1 y que el producto
sale de «Se vende sin descontar».

**Acceptance Scenarios**:

1. **Given** un producto revendido sin receta, **When** se elige «receta con insumos» y «es un producto que se compra hecho», **Then** el sistema ofrece crear el insumo con el nombre del producto, por pieza, con 1 pieza en la receta; la cantidad y el insumo se pueden cambiar antes de guardar.
2. **Given** un producto con existencia propia y 12 piezas registradas, **When** se convierte en receta de 1 pieza de un insumo nuevo, **Then** las 12 piezas pasan al insumo con un movimiento que dice de dónde vienen, el producto queda en cero, y el historial de ambos se conserva.
3. **Given** un producto con receta, **When** se cambia a «existencia propia», **Then** la receta se conserva sin usarse (no se borra) y el cambio queda registrado con quién y cuándo.
4. **Given** un combo cuyos componentes son productos revendidos configurados como receta, **When** se vende el combo, **Then** baja cada insumo de cada componente.
5. **Given** un usuario sin permiso de configurar el menú, **When** intenta cambiar el modo de un producto, **Then** el sistema lo rechaza aunque la petición no pase por la pantalla.

---

### User Story 4 - Un extra que es un producto no puede quedar sin ligar (Priority: P2)

Un extra que representa un producto (el refresco de un combo, una lata que se agrega) debe
descontar: se liga a un producto o lleva su receta. Mientras no esté configurado, se sigue vendiendo
normal y el sistema lo marca en «Salud del inventario» para que alguien lo corrija (decisión del
dueño, 13-oct: no frenar ventas; hoy hay unos 200 extras pendientes).

**Why this priority**: Es el agujero por el que los refrescos de combo no descuentan. Es más chico
que la Historia 3 porque hoy afecta a pocos extras.

**Independent Test**: Crear un extra en un grupo de refrescos sin receta ni producto ligado y ver que
el extra se vende normal y, en cuanto se vende, aparece en «Salud del inventario» con sus unidades.

**Acceptance Scenarios**:

1. **Given** un extra sin receta, sin producto ligado y sin confirmar «No gasta insumos», **When** se vende, **Then** la venta pasa sin preguntas ni avisos a quien opera, y el extra aparece en «Salud del inventario» con sus unidades vendidas.
2. **Given** un extra confirmado «No gasta insumos» (como «Sin hielo»), **When** se vende, **Then** no se considera problema.

---

### Edge Cases

Formas de fallar, pensadas antes del diseño (constitución IV):

- **El valor vacío que significa algo:**
  - Un producto «sin control» confirmado a propósito («No gasta insumos») no es un problema; uno sin decidir sí. La pantalla no puede confundirlos.
  - Un periodo sin ventas no es «todo sano»: se dice que no hubo ventas, y los problemas que no dependen de ventas (negativos, recetas mal formadas) se siguen mostrando.
- **El camino nuevo que se salta el control viejo:**
  - Un producto que no descuenta por sí mismo pero cuyo extra obligatorio sí descuenta no se marca. Pero si ese extra obligatorio tiene una opción que no descuenta, esa opción sí se marca.
  - Un pedido de plataforma que entra con un platillo sin pareja (producto genérico) descuenta nada: aparece como su propio renglón, no se mezcla con los productos del catálogo.
  - El componente de un paquete que no descuenta hace que el paquete no descuente esa parte: se marca el componente.
- **El hermano que no se movió:**
  - Cambiar de modo deja datos del modo anterior (existencia propia sin uso, una receta guardada). Ninguno puede seguir descontando ni desaparecer sin rastro.
  - Un insumo duplicado (mismo nombre, otro id) se marca aunque cada copia esté bien usada por separado.
- **El estado que no sobrevive:**
  - La exportación y la pantalla salen del mismo cálculo y el mismo periodo: si divergen, una de las dos miente.
- **Escala:**
  - Con cientos de extras pendientes, la lista se pagina o se agrupa; no se vuelve inmanejable en una tableta de 600 px de alto.
- **Otra empresa:** la pantalla de una empresa nunca muestra productos, insumos ni ventas de otra.

## Requirements *(mandatory)*

### Functional Requirements

**Salud del inventario (Historia 1 y 2)**

- **FR-001**: El sistema MUST mostrar una pantalla «Salud del inventario» con cinco secciones: se vende sin descontar; insumos en negativo; insumos sin compras registradas; recetas mal formadas; recetas pendientes de confirmar.
- **FR-002**: «Se vende sin descontar» MUST listar cada producto y cada extra activos que en el periodo elegido se vendieron y no dejaron ningún movimiento de almacén, con sus unidades vendidas separadas por canal (mostrador y cada plataforma de reparto).
- **FR-003**: Un producto MUST NOT marcarse si lo que se vende con él descuenta: su receta, su existencia propia, las piezas de su paquete o un extra obligatorio con receta o producto ligado.
- **FR-004**: Un producto o extra confirmado como «No gasta insumos» MUST NOT marcarse como problema; uno que nunca se decidió, sí.
- **FR-005**: «Insumos en negativo» MUST listar cada insumo activo con existencia menor que cero, con su existencia y las recetas activas que lo usan.
- **FR-006**: «Insumos sin compras registradas» MUST mostrar primero un renglón resumen («N de M insumos sin compras registradas») y, con un toque, desplegar la lista de cada insumo activo usado por alguna receta activa sin ninguna entrada de compra desde una fecha elegible. La exportación MUST traer siempre la lista completa.
- **FR-007**: «Recetas mal formadas» MUST listar cada renglón de receta activa con cantidad cero o menor, con unidad de un tipo distinto al del insumo, o que apunta a un insumo archivado o a uno duplicado (mismo nombre, sin distinguir mayúsculas ni acentos, que otro insumo activo).
- **FR-008**: «Recetas pendientes de confirmar» MUST mostrar cuántos productos y extras están «Por revisar» y llevar a Menú › Recetas con ese filtro.
- **FR-009**: Cada renglón MUST llevar a la pantalla donde se corrige (la receta del producto o extra, el insumo).
- **FR-010**: El periodo MUST poder elegirse; un periodo mal formado se rechaza con un mensaje, nunca se reemplaza en silencio por otro (constitución V).
- **FR-011**: La pantalla MUST exportarse a un archivo que abre en una hoja de cálculo, con las mismas cifras y el mismo periodo que se ven.
- **FR-012**: La pantalla MUST verla solo quien tiene permiso de administrar el menú o el almacén, y mostrar solo datos de su empresa.

**Modo de consumo (Historia 3)**

- **FR-013**: Cada producto MUST tener un modo de consumo visible y configurable desde la pantalla: sin control («No gasta insumos»), existencia propia o receta con insumos. Los paquetes siguen descontando por sus componentes.
- **FR-014**: Al elegir «receta con insumos» para un producto que se compra hecho, el sistema MUST ofrecer crear el insumo con el nombre del producto, por pieza, y una receta de 1 pieza; insumo y cantidad editables antes de guardar.
- **FR-015**: Cambiar de modo MUST conservar el historial de movimientos y dejar registro de quién lo cambió y cuándo. Al convertir un producto con existencia propia en receta de **un solo** insumo (nuevo o existente), su existencia MUST pasar a ese insumo en su equivalencia (cada pieza de producto = la cantidad de la receta) con movimientos ligados que digan de dónde viene, y el producto quedar en cero. Si la receta lleva varios insumos, no hay equivalencia única: la existencia del producto se cierra en cero con un ajuste que dice el motivo.
- **FR-016**: Cambiar el modo MUST requerir el permiso de configurar el menú, verificado en el servidor.

**Extras que son productos (Historia 4)**

- **FR-017**: Un extra activo sin receta, sin producto ligado y sin confirmar «No gasta insumos» MUST poder venderse sin bloqueo ni aviso a quien opera, y MUST aparecer en «Salud del inventario» si se vendió en el periodo (decisión del dueño, 13-oct).

**Principio de producto**

- **FR-018**: Ningún nombre, marca, producto o regla de un negocio en particular MUST aparecer en el código ni en las migraciones de esquema; todo lo que distingue a un negocio es dato configurable.

### Key Entities

- **Problema de inventario**: algo que hace que las existencias no cuadren. Tiene un tipo (una de las cinco secciones), a qué apunta (producto, extra, insumo o renglón de receta), su peso (unidades vendidas por canal o existencia) y a dónde se va a corregir. Se calcula; no se guarda.
- **Modo de consumo**: cómo descuenta un producto al venderse (sin control, existencia propia, receta). Es una propiedad del producto con historial de cambios.
- **Extra que es producto**: un extra que, al venderse, debe descontar como un producto (ligado o con receta).

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Sobre la copia del respaldo de producción del 13-oct, la pantalla marca los 31 productos sin descuento y los 126 insumos en negativo que encontró la auditoría, con las mismas unidades vendidas por canal.
- **SC-002**: Quien administra encuentra qué corregir y llega a la pantalla donde se corrige en 2 toques o menos desde «Salud del inventario».
- **SC-003**: Un cliente nuevo configura sin soporte técnico un producto revendido (receta de 1 pieza), un combo que lo incluye y una variación (con un extra), y los tres descuentan bien en la primera venta.
- **SC-004**: Después de corregir con la pantalla los productos marcados, vender cada uno una vez deja un movimiento de almacén por cada venta (cero ventas sin descontar en ese grupo).
- **SC-005**: La exportación coincide renglón por renglón con la pantalla para el mismo periodo.
- **SC-006**: Ninguna referencia a marcas ni productos de un negocio aparece en el código ni en las migraciones de esquema de esta feature.

## Assumptions

- **Fuera de alcance, en el 038 (issue #60):** que las compras entren al almacén, las presentaciones de compra y el conteo inicial. Mientras no exista, «Insumos sin compras registradas» marcará casi todos los insumos: es correcto, y es lo que el 038 corrige.
- **Fuera de alcance:** alérgenos (issue #59), multi-almacén y traspasos, mermas y cortesías, kardex y valor del inventario.
- **Periodo por omisión** de «Se vende sin descontar»: los últimos 30 días.
- **Exportación** en un formato de texto separado por comas que abren las hojas de cálculo comunes; no PDF.
- **Permisos:** los mismos roles que hoy administran el menú y el almacén (administrador y gerente).
- **«Unidades vendidas»** cuenta renglones no cancelados de pedidos no cancelados ni reembolsados, con la fecha del día de negocio.
- **Los combos ya descuentan** por sus componentes y los extras por su receta o producto ligado (spec 028): este spec no cambia esas reglas, solo las hace visibles y configurables.
- **Convertir los 7 productos revendidos** de la auditoría se puede hacer antes con el script preparado para finanzas; la Historia 3 es el camino para que cualquier empresa lo haga sola.
