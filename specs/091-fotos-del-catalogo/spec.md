# Feature Specification: Fotos de productos y categorías

**Feature Branch**: `091-fotos-del-catalogo`

**Created**: 2026-11-23

**Status**: Draft

**Input**: Issue #325, primera etapa del menú en línea (#316). La decisión de almacenamiento está en
los comentarios del #316: Cloudflare R2 con dominio propio de medios.

## Contexto

Cada producto y cada categoría tiene, desde la construcción inicial, un lugar para su foto, pero
ninguna pantalla ni herramienta lo llena. El catálogo de fotos del negocio vive en otra computadora
y hoy no hay forma de subirlo. Sin fotos, el menú público del #316 no sirve: es lo primero que ve
un cliente.

Esta etapa deja tres caminos para llenar las fotos —la pantalla del catálogo, el MCP en lote y una
ilustración por omisión que se elige por familia de comida— y garantiza que **ningún producto se ve
con un hueco** donde va su foto.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Subir la foto de un producto desde el catálogo (Priority: P1)

Quien administra el menú abre un producto en el catálogo, toca «Agregar foto», elige una imagen de
la tableta o la computadora, ve cómo va a quedar (recortada en cuadrado), ajusta el encuadre si hace
falta y la guarda. La foto aparece en la lista de productos. Puede reemplazarla por otra o quitarla.

**Why this priority**: es el camino más corto para que un negocio tenga fotos, y el que usará
cualquier cliente nuevo que no tenga una computadora con el catálogo.

**Independent Test**: con un producto sin foto, subir una imagen desde la pantalla; el producto
muestra la foto en la lista del catálogo y en el editor. Reemplazarla y quitarla.

**Acceptance Scenarios**:

1. **Given** un producto sin foto, **When** quien tiene permiso de configurar el menú toca
   «Agregar foto» y elige una imagen, **Then** ve una vista previa cuadrada antes de guardar y
   puede mover el encuadre o cancelar sin que cambie nada.
2. **Given** la vista previa, **When** toca «Guardar foto», **Then** la foto queda guardada y la
   lista de productos la muestra sin recargar la pantalla.
3. **Given** un producto con foto, **When** sube otra, **Then** se ve la nueva y la anterior deja
   de existir en el almacenamiento.
4. **Given** un producto con foto, **When** toca «Quitar foto» y confirma, **Then** el producto
   vuelve a mostrar la ilustración de su categoría y el archivo se borra del almacenamiento.
5. **Given** un archivo que no es imagen, una imagen demasiado grande o demasiado chica, **When**
   intenta subirla, **Then** ve un mensaje que dice qué hacer («Elige una foto de al menos 400 px»)
   y el producto queda como estaba.
6. **Given** una persona sin permiso de configurar el menú, **When** abre el catálogo, **Then** no
   ve las acciones de foto, y si llama al servidor directo, el servidor lo rechaza.

---

### User Story 2 - Un producto sin foto nunca muestra un hueco (Priority: P1)

Cada categoría elige una **familia de comida** (bebida fría, café, té o bubble tea, frappé o
malteada, soda, ramen o sopa, snack, alitas o boneless, crepa o postre, desayuno, platillo). Un
producto sin foto muestra la foto de su categoría; si su categoría tampoco tiene foto, la
ilustración de la familia de su categoría; y una categoría sin familia, la ilustración genérica.

**Why this priority**: el día que se sube esta etapa casi ningún producto tiene foto. Sin esto, el
catálogo y el menú público se ven rotos hasta que alguien suba cientos de fotos.

**Independent Test**: elegir «Café» como familia de una categoría sin foto; sus productos sin foto
muestran la ilustración de café. Quitar la familia: muestran la genérica.

**Acceptance Scenarios**:

1. **Given** una categoría con familia «Café» y sin foto, **When** se ve uno de sus productos sin
   foto, **Then** muestra la ilustración de café.
2. **Given** una categoría con foto, **When** se ve uno de sus productos sin foto, **Then** muestra
   la foto de la categoría.
3. **Given** una categoría sin familia, **When** se ve uno de sus productos sin foto, **Then**
   muestra la ilustración genérica de platillo.
4. **Given** otro negocio de comida (una fonda) con categorías de nombres propios («Guisados»,
   «Aguas frescas»), **When** configura sus categorías, **Then** elige para cada una la familia que
   le queda, sin que el sistema dependa de cómo se llaman.
5. **Given** una lista de 100 productos sin foto, **When** se abre, **Then** las ilustraciones no
   cuestan una descarga por producto.

---

### User Story 3 - Subir el catálogo de fotos en lote por el MCP (Priority: P2)

El agente que trabaja en la computadora que tiene el catálogo de fotos sube, producto por producto,
la foto que corresponde, con vista previa antes de escribir, y puede quitarla. Lo mismo para las
categorías.

**Why this priority**: el negocio tiene ya un catálogo de fotos; subirlo a mano en la tableta,
foto por foto, es lo que esta herramienta evita. Depende de que el almacenamiento (US1) exista.

**Independent Test**: con un token del MCP con alcance de catálogo, mandar la foto de un producto
con vista previa (no cambia nada y dice qué haría) y luego sin ella; la foto aparece en la pantalla.

**Acceptance Scenarios**:

1. **Given** un token con permiso de configurar el menú, **When** el agente pide poner la foto de
   un producto en modo vista previa, **Then** recibe qué producto se afectaría, si ya tenía foto y
   si la imagen es aceptable, sin que nada cambie.
2. **Given** la misma petición sin vista previa, **When** se aplica, **Then** la foto queda puesta
   igual que si se hubiera subido por la pantalla, y la bitácora dice que fue el token.
3. **Given** una imagen que excede el peso o las dimensiones permitidas, **When** el agente la
   manda, **Then** el rechazo dice el límite y el tamaño recibido.
4. **Given** la misma petición repetida con la misma clave de idempotencia, **When** llega dos
   veces, **Then** se aplica una sola vez.
5. **Given** el agente pide quitar la foto de una categoría, **When** se aplica, **Then** la
   categoría vuelve a mostrar la ilustración de su familia.
6. **Given** un token sin el permiso, **When** intenta poner una foto, **Then** se rechaza.

---

### User Story 4 - Subir la foto de una categoría (Priority: P2)

Igual que el producto, pero desde la categoría: subir, reemplazar y quitar su foto, y elegir su
familia de comida.

**Why this priority**: la foto de la categoría es la que se ve en el menú público como portada de
cada sección, y la que heredan sus productos sin foto.

**Independent Test**: subir la foto de una categoría; sus productos sin foto la muestran.

**Acceptance Scenarios**:

1. **Given** una categoría, **When** se le sube una foto, **Then** sus productos sin foto la
   muestran en el catálogo.
2. **Given** una categoría, **When** se le elige familia, **Then** la ilustración de esa familia
   aparece en cuanto la categoría no tiene foto.

### Edge Cases

- **Foto tomada de lado** (orientación guardada en la imagen): la vista previa y la foto guardada
  salen derechas.
- **Imagen enorme** (una foto de 48 megapíxeles, 15 MB): la tableta la reduce antes de mandarla; si
  aun así excede el límite, se rechaza con el límite en el mensaje.
- **Imagen diminuta** (un ícono de 64 px): se rechaza, porque en el menú público se vería borrosa.
- **Un archivo con extensión de imagen que no es imagen** (un ejecutable renombrado): el servidor lo
  rechaza revisando el contenido, no el nombre ni lo que dice el navegador.
- **Una imagen con contenido activo** (SVG con scripts): no se acepta; solo fotos.
- **Se cae la red a media subida**: el producto queda como estaba; no queda a medias con una foto
  que no existe.
- **El almacenamiento no responde**: el servidor contesta con un error claro y el producto queda
  como estaba; la venta no se detiene por esto.
- **Borrar el archivo anterior falla** después de guardar la foto nueva: la foto nueva queda
  puesta; el archivo huérfano se registra para limpiarlo, sin regresarle un error a quien subió.
- **Dos personas suben la foto del mismo producto casi a la vez**: gana la última, y ningún archivo
  queda referido por dos productos ni borrado mientras otro lo usa.
- **La misma imagen subida dos veces** al mismo producto: no se duplica el archivo.
- **Un producto se archiva o se borra**: su foto se conserva mientras el producto exista
  (archivado se puede reactivar); solo se borra al quitarla.
- **Otra empresa**: nunca ve, reemplaza ni borra la foto de un producto ajeno, aunque adivine su
  identificador o el nombre del archivo.
- **Producto cuya categoría cambia**: si no tiene foto, pasa a mostrar la de su categoría nueva.

## Requirements *(mandatory)*

### Functional Requirements

**Almacenamiento**

- **FR-001**: Las fotos se guardan en un almacenamiento de objetos externo con dominio propio de
  medios; ninguna foto se sirve desde la API ni desde el servidor del negocio.
- **FR-002**: Cada foto se guarda en dos tamaños (lado de 400 px y de 1200 px), en un formato
  ligero para web, generados en el dispositivo de quien sube antes de mandarlos.
- **FR-003**: El nombre de cada archivo se deriva de su contenido, de modo que un archivo nunca
  cambia una vez publicado y se puede guardar en caché por un año.
- **FR-004**: El archivo vive bajo la empresa dueña; una empresa no puede escribir ni borrar
  archivos de otra.
- **FR-005**: Al reemplazar o quitar una foto, los archivos anteriores se borran del
  almacenamiento. Un borrado que falla no revierte el cambio ya hecho y queda registrado.
- **FR-006**: La credencial del almacenamiento solo puede tocar el espacio de medios, viaja como
  secreto y la API se niega a arrancar en producción sin ella si la función está encendida.

**Validación en el servidor**

- **FR-007**: El servidor valida cada archivo por su contenido: solo el formato de foto ligero
  acordado, peso máximo por tamaño (400 px: hasta 100 KB; 1200 px: hasta 400 KB) y dimensiones
  exactas esperadas para cada tamaño (cuadrado, 400×400 y 1200×1200). Lo que no cumple se rechaza
  con un error de validación (nunca un 500) que dice el límite.
- **FR-008**: El servidor recalcula la huella de contenido; la que mande el cliente no se usa.
- **FR-009**: El tamaño de la petición completa tiene tope antes de leerla, para que un archivo
  gigante no agote la memoria del servidor.

**Pantalla**

- **FR-010**: En el editor de producto y en el de categoría hay un control de foto con botón táctil
  grande, vista previa cuadrada y encuadre (arrastrar y acercar), con «Guardar foto», «Cancelar»,
  «Reemplazar» y «Quitar foto». Nunca un selector de archivo sin vista previa.
- **FR-011**: La vista previa respeta la orientación de la foto.
- **FR-012**: Quitar una foto pide confirmación.
- **FR-013**: La lista de productos y la de categorías del catálogo muestran una miniatura de la
  foto o de la ilustración que le corresponde.
- **FR-014**: Solo quien tiene permiso de configurar el menú ve y usa las acciones de foto; el
  servidor aplica el mismo permiso.

**Ilustraciones por omisión**

- **FR-015**: Existe un juego fijo de familias de comida —bebida fría, café, té o bubble tea,
  frappé o malteada, soda, ramen o sopa, snack, alitas o boneless, crepa o postre, desayuno y
  platillo (genérica)— cada una con su ilustración ligera en los colores de la marca.
- **FR-016**: Cada categoría puede elegir una familia; sin elegir, usa la genérica. La elección no
  depende del nombre de la categoría.
- **FR-017**: Qué se muestra para un producto: su foto; si no tiene, la foto de su categoría; si
  tampoco, la ilustración de la familia de su categoría; si no tiene familia, la genérica. Para
  una categoría: su foto o la ilustración de su familia.
- **FR-018**: Las ilustraciones se cargan una sola vez para toda la pantalla, no una por producto.

**MCP**

- **FR-019**: El MCP ofrece poner y quitar la foto de un producto y de una categoría, con vista
  previa, clave de idempotencia, permiso de configurar el menú y registro en bitácora, igual que
  las demás escrituras del MCP.
- **FR-020**: El MCP acepta la foto ya preparada en los dos tamaños, con un tope de peso, y aplica
  las mismas validaciones del servidor que la pantalla. La forma en que viaja el archivo queda
  documentada con un ejemplo para la computadora que tiene el catálogo.
- **FR-021**: El MCP permite también elegir la familia de una categoría.

**Rendimiento**

- **FR-022**: Donde se muestra una foto se declara su tamaño para que la pantalla no salte al
  cargar, se pide el tamaño chico o el grande según el espacio, y las fotos fuera de la vista se
  cargan al acercarse.

**Rastro**

- **FR-023**: Poner, reemplazar y quitar una foto, y cambiar la familia, queda en la bitácora de
  auditoría con quién lo hizo.

### Key Entities

- **Foto de producto / de categoría**: la dirección pública de la foto grande; la chica se deriva
  de ella. Una por producto y una por categoría.
- **Familia de comida**: un valor de un conjunto fijo del producto (no de la empresa), con su
  ilustración. Una categoría tiene cero o una familia.
- **Archivo pendiente de borrar**: un archivo cuyo borrado falló, con su empresa, para limpiarlo
  después.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Una persona nueva, de una fonda, sube la foto de un producto desde la tableta en
  menos de 1 minuto y sin ayuda.
- **SC-002**: El 100 % de los productos del catálogo muestran una foto o una ilustración: cero
  huecos.
- **SC-003**: Las 11 ilustraciones juntas pesan menos de 40 KB y se descargan una sola vez.
- **SC-004**: La foto chica que se descarga para una miniatura pesa menos de 100 KB.
- **SC-005**: Ninguna petición de imagen llega a la API ni al servidor del negocio.
- **SC-006**: Reemplazar 50 fotos deja 0 archivos sin usar en el almacenamiento (salvo los que
  quedaron registrados como pendientes de borrar).
- **SC-007**: Un archivo que no es foto, o que excede peso o dimensiones, se rechaza el 100 % de
  las veces, por la pantalla y por el MCP.

## Assumptions

- **El mosaico de la pantalla de venta no cambia en esta etapa.** La foto se ve en el catálogo del
  POS (lista y editor) y la usará el menú público (#316). Meter fotos en los mosaicos de venta le
  quita renglones al operador y es una decisión de diseño aparte; los datos ya quedan listos.
- El permiso que gobierna las fotos es el mismo que configura el menú; no hay permiso nuevo.
- Los tamaños se generan en el dispositivo de quien sube, porque el servidor del negocio es chico y
  no debe gastar CPU en convertir imágenes. La computadora del catálogo prepara los dos tamaños con
  el script documentado.
- Las familias de comida son del producto, no de cada empresa: un conjunto fijo cubre a los
  negocios de comida; agregar una familia es un cambio de código, igual que agregar un permiso.
- Lectura pública: cualquiera que conozca la dirección de una foto la puede ver. Las fotos de un
  menú no son secretas.
- En local y en pruebas se usa un almacenamiento de objetos compatible que corre en la máquina, para
  no depender de la cuenta de producción.
- Fuera: varias fotos por producto, texto alternativo editable, galería, fotos de extras.
