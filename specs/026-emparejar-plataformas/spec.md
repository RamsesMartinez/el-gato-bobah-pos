# Feature Specification: Emparejar la tienda de plataforma conectada

**Feature Branch**: `026-emparejar-plataformas`

**Created**: 2026-10-02

**Status**: Draft

**Input**: Rediseño de la pantalla que liga lo publicado en una tienda de plataforma (Uber) con el
catálogo del POS, una vez conectada. Diseño B elegido por el dueño (2026-09-30): lista y panel lado
a lado, a pantalla completa, con «Por revisar» en dos modos (uno por uno y en lote). Emparejamiento
flexible y precio que pone la plataforma. Decisiones en
[docs/emparejamiento-de-plataformas.md](../../docs/emparejamiento-de-plataformas.md).

## Contexto

Con la tienda de Uber conectada, el POS lee el menú publicado (65 platillos y 25 opciones en la
tienda real) y hay que decir a qué producto del POS corresponde cada uno. Hoy se hace uno por uno,
en una pantalla que el dueño calificó de inservible, y que tiene cuatro defectos medidos el
2026-09-28:

1. No deja ligar un producto del POS a varios platillos de Uber, aunque la base sí lo permite.
2. La fila de decisión se sale de la pantalla de 1024 px.
3. Las opciones (extras) de Uber no se pueden emparejar desde la pantalla.
4. El precio de Uber sale sin formato ($82.8).

Mientras un platillo no esté emparejado, el pedido que llega de Uber entra sin producto del POS:
no descuenta inventario y los reportes por producto no lo ven.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Ver de un vistazo cómo va el emparejamiento (Priority: P1)

Quien administra abre la tienda conectada y ve toda la lista de platillos y opciones de Uber,
separada en «Sin pareja», «Por revisar» y «Listos», con cuántos hay en cada una, y un buscador.
Sabe cuánto falta sin recorrer nada.

**Why this priority**: sin el universo a la vista no se puede trabajar en orden ni saber cuándo se
terminó. Es la base de todo lo demás.

**Independent Test**: con un menú de 65 platillos y 25 opciones, la pantalla muestra los tres grupos
con su conteo, la suma da 90, y buscar «chai» deja solo lo que coincide.

**Acceptance Scenarios**:

1. **Given** una tienda conectada con su menú leído, **When** se abre su pantalla, **Then** se ven
   los tres grupos con su conteo y la lista del grupo elegido, sin desplazamiento horizontal a
   1024×600.
2. **Given** la lista, **When** se escribe en el buscador, **Then** quedan solo los platillos y
   opciones cuyo nombre en Uber o en el POS coincide.
3. **Given** la pantalla, **When** se mira cualquier precio de Uber, **Then** sale con formato de
   moneda y la marca «lo pone Uber».

---

### User Story 2 - Revisar las propuestas, una por una o en lote (Priority: P1)

El sistema propone parejas por parecido de nombre; ninguna se da por buena sin que una persona la
vea. En «Por revisar» se elige el modo:

- **Uno por uno**: una propuesta en grande, con los dos nombres y los dos precios. «Es el mismo» la
  confirma y pasa sola a la siguiente; «Es otro…» abre el buscador; «Saltar» la deja para después;
  «Deshacer» revierte la última confirmación.
- **En lote**: la lista con casillas, todas marcadas, los dos nombres a la vista, y un solo
  «Confirmar N». Desmarcar una la deja en «Sin pareja».

**Why this priority**: es donde se va casi todo el tiempo. Las propuestas son la mayoría del menú y
hoy cuestan varios toques cada una.

**Independent Test**: con 12 propuestas, confirmarlas en lote toma dos toques y deja 12 parejas
confirmadas con quién y cuándo; en uno por uno, 12 toques de «Es el mismo» dan el mismo resultado.

**Acceptance Scenarios**:

1. **Given** 12 propuestas, **When** se confirma en lote con 11 marcadas, **Then** quedan 11 listas
   y la desmarcada pasa a «Sin pareja».
2. **Given** el modo uno por uno, **When** se toca «Es el mismo», **Then** la pareja queda confirmada
   y aparece la siguiente propuesta sin otro toque.
3. **Given** una confirmación recién hecha, **When** se toca «Deshacer», **Then** vuelve a ser
   propuesta.
4. **Given** cualquier modo, **When** se confirma, **Then** la pareja guarda quién la confirmó y
   cuándo.

---

### User Story 3 - Emparejar lo que no tiene propuesta, sin las limitaciones de hoy (Priority: P1)

Para un platillo sin propuesta, se busca el producto del POS con resultados ordenados por parecido.
Se puede ligar:

- uno a uno (el mismo producto en los dos lados);
- **varios platillos de Uber al mismo producto del POS** (cada crepa de Uber a «Arma tu Crepa»);
- un platillo de Uber con otra composición a un **producto base** del POS.

Las opciones de Uber (extras) se emparejan igual, contra las opciones del POS, en la misma vista.

**Why this priority**: es el defecto 1 y el 3; sin esto el emparejamiento de El Gato Bobah no se
puede terminar nunca.

**Independent Test**: ligar tres crepas de Uber al mismo producto del POS deja tres parejas listas,
y el producto sigue apareciendo en el buscador para la cuarta.

**Acceptance Scenarios**:

1. **Given** un producto del POS ya ligado a un platillo de Uber, **When** se busca para otro
   platillo, **Then** aparece en los resultados, con la marca de cuántos platillos de Uber ya tiene.
2. **Given** una opción de Uber, **When** se busca, **Then** los resultados son opciones del POS, no
   productos.
3. **Given** el buscador, **When** se escribe, **Then** los resultados más parecidos van primero.

---

### User Story 4 - Corregir y decidir lo que no tiene pareja (Priority: P2)

Desde «Listos» se toca cualquier pareja y se puede cambiar de producto o quitarla. Para lo que no
debe tener pareja hay dos decisiones que se guardan:

- **«Solo existe en Uber»**: el platillo no tiene equivalente en el POS, a propósito.
- **«No se vende en Uber»**: para la vista inversa, un producto del POS que no se ofrece en Uber.

Con eso, «Sin pareja» llega a cero cuando se terminó, en lugar de mezclar lo pendiente con lo
decidido.

**Why this priority**: sin estas decisiones el trabajo nunca se ve terminado, pero el valor
principal ya llega con las historias 1 a 3.

**Independent Test**: marcar un platillo como «Solo existe en Uber» lo saca de «Sin pareja», y se
puede revertir.

**Acceptance Scenarios**:

1. **Given** una pareja confirmada, **When** se cambia de producto, **Then** la nueva queda
   confirmada por quien la cambió y la anterior desaparece.
2. **Given** un platillo marcado «Solo existe en Uber», **When** se vuelve a leer el menú,
   **Then** sigue marcado.
3. **Given** que un platillo desaparece de Uber, **When** se vuelve a leer, **Then** su pareja no se
   borra (sigue sirviendo si vuelve).

---

### User Story 5 - El precio de Uber manda y se ve que manda (Priority: P2)

Con la tienda conectada, el precio de cada platillo emparejado es el que publica Uber y se actualiza
en cada lectura del menú. En la tienda hay un aviso fijo: «Los precios de Uber los pone Uber. Para
cambiar uno, cámbialo en Uber.» Tras cada lectura se dice cuántos precios cambiaron y cuáles. En el
catálogo, el precio de Uber de un producto emparejado se ve bloqueado con «Lo pone Uber · se
actualizó hoy a las 10:42»; el de una plataforma no conectada sigue editable.

**Why this priority**: es regla del dueño (constitución, 2026-09-28) y quita de la pantalla la tarea
de «precio distinto», que deja de ser decisión de nadie.

**Independent Test**: cambiar en la copia leída el precio de un platillo emparejado y volver a leer:
el precio del POS queda igual al de Uber, el aviso lo cuenta y el catálogo lo muestra bloqueado.

**Acceptance Scenarios**:

1. **Given** un platillo emparejado cuyo precio cambió en Uber, **When** se lee el menú, **Then** el
   POS guarda el precio nuevo y el aviso lo lista.
2. **Given** un producto del POS ligado a tres platillos de Uber con precios distintos, **When** se
   lee el menú, **Then** cada platillo conserva su propio precio; ninguno se pierde ni se promedia.
3. **Given** el catálogo, **When** se abre un producto emparejado, **Then** su precio de Uber no se
   puede editar y dice que lo pone Uber.

---

### User Story 6 - Un platillo de Uber sin pareja entra igual y sale en la comanda (Priority: P1)

Llega un pedido de Uber con un platillo que el POS no tiene emparejado. Se acepta como cualquier
otro y el renglón queda ligado a un producto genérico del negocio, «Platillo de plataforma sin
pareja», con el detalle de lo que mandó Uber: el nombre del platillo, sus opciones y su precio.
Sale en la comanda de cocina como un renglón normal, y nadie decide nada al aceptarlo.

**Why this priority**: hoy ese renglón entra sin producto (decisión del dueño, 2026-10-03: no
esperar al spec del almacén). Sin producto no sigue el camino de un renglón normal en reportes y
comanda.

**Independent Test**: aceptar un pedido con un platillo sin pareja y ver el renglón ligado al genérico,
con el nombre y las opciones de Uber, en la comanda impresa.

**Acceptance Scenarios**:

1. **Given** un platillo sin pareja en un pedido, **When** se acepta, **Then** el renglón queda en el
   producto genérico con el nombre de Uber como nombre del renglón y las opciones como nota.
2. **Given** el producto genérico, **When** se abre la pantalla de vender, **Then** no aparece: no se
   vende a mano.
3. **Given** un negocio nuevo, **When** se da de alta, **Then** ya tiene su producto genérico.
4. **Given** el renglón genérico, **When** se cuenta el almacén, **Then** no descuenta nada (no se
   sabe qué lleva) y queda contado como venta de plataforma.

---

### Edge Cases

- **Menú vacío o sin leer**: la pantalla dice que hay que leer el menú, con el botón para hacerlo,
  no una lista vacía.
- **Producto del POS desactivado o borrado** con parejas: la pareja se conserva (la base no deja
  borrarlo) y la lista lo marca para corregir.
- **Dos personas confirmando a la vez** la misma propuesta: gana una y la otra ve el estado
  actualizado, sin error confuso.
- **Opción de Uber igual de nombre a un producto del POS**: el buscador de una opción solo ofrece
  opciones; nunca liga una opción con un producto.
- **Lectura del menú que falla a medias**: no se tocan precios; se conserva la última lectura buena.
- **Dos sucursales**: cada tienda es de una sucursal; la pantalla trabaja una tienda a la vez y el
  precio es de esa tienda.
- **Tableta vertical**: la lista y el panel se apilan; ningún botón queda fuera de la pantalla.
- **Escribir en Uber**: ningún botón de esta pantalla manda nada a Uber.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: La pantalla de la tienda conectada MUST mostrar todos los platillos y opciones de la
  última lectura en tres grupos (Sin pareja, Por revisar, Listos) con su conteo, y un buscador.
- **FR-002**: Ninguna pareja propuesta MUST darse por confirmada sin que una persona la vea; toda
  confirmación guarda quién y cuándo.
- **FR-003**: «Por revisar» MUST ofrecer los dos modos, uno por uno y en lote, con un selector.
- **FR-004**: El modo uno por uno MUST avanzar solo tras confirmar y permitir deshacer la última.
- **FR-005**: El modo en lote MUST confirmar todas las marcadas con una sola acción.
- **FR-006**: El sistema MUST permitir ligar varios platillos de Uber a un mismo producto del POS.
- **FR-007**: Las opciones de Uber MUST poder emparejarse con opciones del POS en la misma vista.
- **FR-008**: El buscador MUST ordenar los candidatos por parecido de nombre.
- **FR-009**: Cualquier pareja confirmada MUST poder cambiarse o quitarse desde la lista.
- **FR-010**: El sistema MUST guardar las decisiones «Solo existe en Uber» y «No se vende en Uber»,
  y conservarlas entre lecturas del menú.
- **FR-011**: Con la tienda conectada, el precio de cada platillo emparejado MUST ser el de Uber,
  guardado **por platillo de la tienda**, y actualizarse en cada lectura buena del menú.
- **FR-012**: La pantalla y el catálogo MUST decir que ese precio lo pone Uber y cuándo se actualizó;
  el precio de una plataforma no conectada MUST seguir editable.
- **FR-013**: Tras cada lectura, el sistema MUST decir cuántos precios cambiaron y cuáles.
- **FR-014**: Todo precio MUST mostrarse con formato de moneda.
- **FR-015**: Todos los controles MUST caber en 1024×600 sin desplazamiento horizontal, medir al
  menos 44 px de alto y no usar selectores nativos.
- **FR-016**: Nada en esta pantalla MUST escribir en Uber.
- **FR-017**: Un platillo u opción de Uber MUST ligarse a una sola cosa del POS (un producto o una
  opción). Varios platillos de Uber MAY ligarse al mismo producto del POS; en ese caso, quien
  configura MUST elegir en esta pantalla cuál de ellos da el precio para la captura a mano, y la
  pantalla no deja terminar la pareja sin esa elección. Al operar, el sistema MUST aplicar esa
  elección sin preguntar (constitución, 2026-10-03).
- **FR-018**: Una opción de Uber MUST poder ligarse a una opción de modificador del POS, no solo a un
  producto (hoy el emparejamiento solo guarda productos).
- **FR-019**: Aceptar un pedido de Uber MUST seguir siendo un solo toque y nunca pedir una decisión
  al operador: lo que no tenga pareja entra igual y sale a cocina.

- **FR-020**: Todo renglón de un pedido de plataforma sin pareja MUST ligarse al producto genérico
  de su empresa, con el nombre de Uber como nombre del renglón y sus opciones y precio en la nota.
- **FR-021**: Toda empresa MUST tener su producto genérico desde que nace; MUST salir en la comanda
  de cocina y MUST NOT aparecer en la pantalla de vender.

### Key Entities

- **Platillo de la tienda**: lo que Uber publica en una tienda (nombre, precio, disponible), de la
  última lectura. Pertenece a una tienda, que pertenece a una sucursal.
- **Pareja**: liga un platillo u opción de la tienda con un producto u opción del POS. Estado:
  propuesta o confirmada (quién, cuándo). Un platillo tiene a lo más una pareja; un producto puede
  tener varias, y entonces una de ellas es la que da el precio de captura a mano.
- **Decisión sin pareja**: «solo existe en Uber» (sobre un platillo) o «no se vende en Uber» (sobre
  un producto del POS), con quién y cuándo.
- **Precio de la tienda**: el precio que Uber publica para un platillo emparejado, con la hora de la
  lectura que lo trajo.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Emparejar el menú real (65 platillos, 25 opciones) toma menos de 15 minutos, contra la
  sesión completa que toma hoy.
- **SC-002**: Confirmar N propuestas en lote cuesta 2 toques, sin importar N.
- **SC-003**: Al terminar, «Sin pareja» llega a 0.
- **SC-004**: Después de una lectura, el 100% de los precios de Uber de platillos emparejados es
  igual en el POS que en Uber.
- **SC-005**: Ningún control queda fuera de la pantalla a 1024×600, horizontal ni vertical.

## Assumptions

- La plataforma es Uber; DiDi y Rappi no tienen API conectada todavía, pero el diseño no asume Uber
  en los nombres de las tablas.
- El producto genérico («OTRO») entró a este spec por decisión del dueño (2026-10-03, historia 6).
- Insumos, tamaños y variantes que heredan quedan fuera: es otro spec.
- La lectura del menú y la conexión de la tienda ya existen (spec 020); la conexión y las llaves
  salen de la vista de trabajo diario y quedan en su propia sección.
- Depende de sucursales (spec 025): la tienda ya pertenece a una sucursal.
