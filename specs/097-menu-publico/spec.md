# Feature Specification: Menú público v1 con enlace y QR

**Feature Branch**: `097-menu-publico`

**Created**: 2026-11-23

**Status**: Draft

**Input**: Issue #316 con sus comentarios (plan por etapas y decisiones del dueño del 2026-11-23):
cada negocio publica su menú en `menu.elgatobobah.com/<nombre>`, con un nombre propio editable que
no es el slug del login y que al cambiar sigue llevando al nuevo; un QR estable; se comparte por
WhatsApp; rendimiento medido; una sola plantilla separada en plantilla + datos + estilo de marca.

## Contexto

Es la etapa 2 del menú en línea. Las piezas de la etapa 1 ya están en producción y este menú solo
las **lee**:

| Pieza | De dónde viene | Qué aporta al menú |
| --- | --- | --- |
| Dónde se vende cada producto y su precio web | #326 (spec 092) | Qué productos salen (los que tienen «Web» encendido) y a qué precio (el web, o el de mostrador si no hay) |
| «Agotado» | #326, función única de existencia del lugar de venta (su interior lo cambió la spec 094) | Qué producto sale marcado como agotado. El menú no calcula existencia por su cuenta |
| Fotos y familia de comida | #325 (spec 091) | Foto de cada producto y categoría; la ilustración de la familia cuando no hay foto |
| Descripción para el cliente | Pestaña «Venta en web» del producto (spec 092) | El texto bajo el nombre |
| Sección «Venta en web» de Ajustes | spec 092 | Donde viven el nombre, el enlace y el QR |

Lo que **no** existe hoy y este spec define: el nombre público del menú, el historial de nombres
para que lo viejo siga llevando a lo nuevo, el código estable del QR y el color de marca del menú
(la paleta del POS se elige por tableta, no por negocio).

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Un cliente abre el menú en su celular (Priority: P1)

Un cliente toca el enlace que el negocio le mandó por WhatsApp (o escanea el QR de la mesa) y ve el
menú del negocio: el nombre del negocio arriba, las categorías en orden con un acceso rápido para
saltar entre ellas, y cada producto con su foto (o la ilustración de su familia), nombre,
descripción y precio. Lo que está agotado se ve marcado como «Agotado». No necesita instalar nada ni
crear cuenta.

**Why this priority**: es el producto: sin esto no hay nada que compartir.

**Independent Test**: con un negocio que tiene productos en web, abrir su dirección pública en un
celular simulado y comprobar categorías, productos, precios, fotos, ilustraciones y «Agotado».

**Acceptance Scenarios**:

1. **Given** un producto con «Web» encendido y sin precio web, **When** el cliente abre el menú,
   **Then** lo ve con el precio de mostrador.
2. **Given** un producto con precio web propio, **When** el cliente abre el menú, **Then** ve el
   precio web y no el de mostrador.
3. **Given** un producto con «Web» apagado, **When** el cliente abre el menú, **Then** no aparece.
4. **Given** un producto con «vender sin existencia» apagado y sin existencia en el lugar de venta,
   **When** el cliente abre el menú, **Then** el producto aparece con la marca «Agotado» y sin
   ninguna cantidad.
5. **Given** un producto sin foto en una categoría con familia «Café», **When** se ve en el menú,
   **Then** muestra la ilustración de café; y sin familia, la genérica. Nunca un hueco.
6. **Given** una categoría cuyos productos no se venden en web, **When** se abre el menú, **Then**
   esa categoría no aparece.
7. **Given** un negocio sin ningún producto en web, **When** se abre su menú, **Then** ve el nombre
   del negocio y un mensaje de que el menú todavía no tiene productos.

---

### User Story 2 - La dueña comparte su menú por enlace y QR (Priority: P1)

La dueña de una fonda entra a Ajustes › Venta en web y ve la dirección de su menú, un botón para
copiarla, uno para abrirla y uno para compartirla; ve el QR y lo descarga como imagen (PNG), como
vector (SVG) o como tarjeta lista para imprimir con el nombre del negocio. Pega el enlace en
WhatsApp y el mensaje muestra la vista previa con el nombre del negocio, una foto y una descripción.

**Why this priority**: compartir es para lo que existe el menú; sin enlace ni QR, nadie llega.

**Independent Test**: desde Ajustes, copiar el enlace y descargar el QR en los tres formatos;
escanear el QR descargado y comprobar que abre el menú; pedir la página como lo hace el rastreador
de WhatsApp y comprobar que trae título, imagen y descripción sin ejecutar código.

**Acceptance Scenarios**:

1. **Given** un negocio con nombre de menú, **When** abre Venta en web, **Then** ve la dirección
   completa, el QR y los botones Copiar, Abrir, Compartir y Descargar.
2. **Given** el QR descargado en PNG, **When** se escanea con un celular, **Then** abre el menú del
   negocio.
3. **Given** la dirección del menú, **When** la pide un rastreador de vista previa que no ejecuta
   código, **Then** recibe el título (nombre del negocio), una descripción y la foto de un producto
   (o la imagen por omisión si ninguno tiene foto).
4. **Given** quien no tiene el permiso de configurar el negocio, **When** abre Ajustes, **Then** no
   puede cambiar el nombre (el servidor lo rechaza).

---

### User Story 3 - Cambiar el nombre del menú sin romper lo ya compartido (Priority: P2)

La dueña cambia el nombre de su menú (por ejemplo, de `fonda-dona-lupe` a `donalupe`). La nueva
dirección funciona al momento; la dirección anterior —la que ya está en mensajes de WhatsApp y
volantes— sigue llevando al menú nuevo; y el QR que ya imprimió sigue funcionando sin reimprimirse.

**Why this priority**: sin esto, cambiar el nombre rompe cada QR impreso y cada enlace compartido.

**Independent Test**: cambiar el nombre y abrir las tres direcciones (nueva, anterior y la del QR).

**Acceptance Scenarios**:

1. **Given** un menú llamado `a`, **When** la dueña lo cambia a `b`, **Then** `/b` muestra el menú y
   `/a` lleva a `/b`.
2. **Given** el menú pasó de `a` a `b` y luego a `c`, **When** alguien abre `/a` o `/b`, **Then**
   llega a `/c` en un solo salto.
3. **Given** el menú pasó de `a` a `b`, **When** la dueña regresa a `a`, **Then** `/a` muestra el
   menú y `/b` lleva a `/a`, sin ciclos.
4. **Given** el nombre `a` fue de otro negocio alguna vez, **When** la dueña intenta usarlo, **Then**
   el sistema le dice que ese nombre no está disponible.
5. **Given** un nombre con mayúsculas, acentos o espacios («Doña Lupe»), **When** la dueña lo
   escribe, **Then** ve al momento cómo queda la dirección (`dona-lupe`) y eso es lo que se guarda.
6. **Given** un nombre reservado (por ejemplo `admin`, `api`, `q`, `www`), **When** lo intenta,
   **Then** el sistema le dice que no está disponible.
7. **Given** el QR impreso, **When** se escanea después de cualquier cambio de nombre, **Then** abre
   el menú con el nombre vigente.

---

### User Story 4 - El menú se ve con los colores del negocio y es cómodo en cualquier pantalla (Priority: P2)

La dueña elige el color de su menú en Venta en web, de la misma lista de paletas que ya ofrece la
pantalla Interfaz del POS. El menú se ve con ese color en celular, tableta y computadora: en celular
una columna; en pantallas más anchas, más columnas y el acceso a categorías siempre a la vista.

**Why this priority**: la marca hace que el menú se sienta del negocio; la comodidad en pantallas
grandes importa para quien lo abre en la computadora, pero el grueso entra por celular.

**Independent Test**: cambiar el color y abrir el menú a 360, 768 y 1280 px de ancho.

**Acceptance Scenarios**:

1. **Given** la paleta «verde» elegida, **When** se abre el menú, **Then** los acentos (encabezado,
   categoría activa, precio) usan esa paleta.
2. **Given** un celular de 360 px de ancho, **When** se abre el menú, **Then** no hay desplazamiento
   horizontal y cada control tocable mide al menos 44 px.
3. **Given** una computadora de 1280 px, **When** se abre el menú, **Then** los productos se
   acomodan en varias columnas y no en una sola tira estrecha.

---

### User Story 5 - El menú carga rápido en un celular de gama baja (Priority: P1)

Un cliente con un celular económico y datos móviles abre el menú y lo ve casi de inmediato, sin
esperar a que cargue todo: las fotos de más abajo llegan conforme baja.

**Why this priority**: un menú que tarda se cierra antes de verse; lo pidió el dueño desde el
principio.

**Independent Test**: medir la página con el perfil móvil de la herramienta de auditoría de páginas
web (celular de gama media-baja, red 4G lenta) contra un menú con el catálogo real.

**Acceptance Scenarios**:

1. **Given** el menú de El Gato Bobah con su catálogo real, **When** se mide con el perfil móvil
   (4G lenta, celular de gama baja), **Then** el elemento principal aparece en menos de 2.5 s.
2. **Given** un catálogo que cambia (precio, foto, producto apagado para web, un producto que se
   agota), **When** pasan como máximo 2 minutos, **Then** el menú público lo refleja.

---

### Edge Cases

- **Nombre vacío o de un solo carácter**: se rechaza; mínimo 3, máximo 40 caracteres.
- **Dos negocios piden el mismo nombre a la vez**: uno lo obtiene y al otro se le dice que no está
  disponible; nunca quedan dos negocios con el mismo nombre.
- **Un nombre que fue de otro negocio**: no se puede reusar nunca (si se pudiera, el QR impreso de
  un negocio llevaría al menú de otro).
- **Regresar a un nombre propio anterior**: se permite; el que se deja pasa a redirigir.
- **Cambiar de nombre muchas veces**: un negocio guarda hasta 20 nombres anteriores; con 20, solo
  puede volver a uno de ellos (los nombres nunca se liberan y sin tope se podrían acaparar).
- **Dirección con mayúsculas o diagonal final** (`/Dona-Lupe/`): lleva a la forma en minúsculas.
- **Dos personas guardan el nombre a la vez** en el mismo negocio: se aplican en orden; ninguna ve
  un «no disponible» falso.
- **Nombre con guiones al principio, al final o dobles** («-lupe», «lupe--fonda»): se normaliza a
  guiones sencillos internos.
- **Dirección que no existe** (`/no-existe`): página de «Este menú no existe» con estado de no
  encontrado, sin decir qué negocios sí existen.
- **Empresa inactiva**: su menú responde como no encontrado.
- **El servidor del POS no responde**: el menú se sigue mostrando con la última copia buena (hasta
  24 h); si nunca hubo copia, una página de «No pudimos cargar el menú, intenta de nuevo».
- **Producto en temporada terminada o inactivo**: no sale, igual que en Vender.
- **Producto agotado**: sale con «Agotado»; nunca se muestra cuánto queda.
- **Producto con precio web pero «Web» apagado**: no sale.
- **Descripción larga**: se corta visualmente a dos renglones en la tarjeta (el texto completo no
  se pierde: se ve al tocar la tarjeta).
- **Producto sin categoría activa** o categoría inactiva: no sale.
- **Más de un negocio con datos en la misma base**: el menú de uno nunca muestra productos, precios
  ni nombres del otro.
- **Alguien prueba direcciones al azar para descubrir negocios o tumbar el servidor**: límite de
  peticiones por dirección IP; las direcciones inexistentes también se guardan en caché un rato.
- **Foto que no carga** (medios caídos): se ve la ilustración de la familia en su lugar.
- **Precio con centavos**: se muestra «$45.50»; sin centavos, «$45».

## Requirements *(mandatory)*

### Functional Requirements

**Nombre y dirección**

- **FR-001**: Cada negocio MUST tener un nombre de menú propio, distinto del identificador con el que
  se inicia sesión, que forma su dirección `<base>/<nombre>`. La base es un solo valor de
  configuración (hoy `https://menu.elgatobobah.com`).
- **FR-002**: El nombre MUST tener de 3 a 40 caracteres, solo letras minúsculas sin acento, números y
  guiones internos; lo que escribe la dueña se normaliza (minúsculas, sin acentos, espacios a guión)
  y la pantalla muestra al momento cómo quedará.
- **FR-003**: El nombre MUST ser único entre todos los negocios, incluidos los nombres que cada
  negocio usó antes; un nombre usado alguna vez por un negocio solo lo puede volver a usar ese mismo
  negocio.
- **FR-004**: El sistema MUST rechazar una lista de nombres reservados (rutas propias del menú y
  palabras de sistema como `q`, `api`, `admin`, `www`, `menu`, `static`, `assets`, `ayuda`,
  `soporte`, `login`).
- **FR-005**: Al cambiar el nombre, toda dirección con un nombre anterior del negocio MUST llevar a
  la dirección vigente en un solo salto, con una redirección temporal (para que regresar a un nombre
  anterior no deje navegadores atrapados en un salto viejo).
- **FR-006**: Toda empresa, existente o nueva, MUST tener un nombre de menú desde el primer día,
  derivado del nombre del negocio (no del identificador de inicio de sesión) y desambiguado con un
  sufijo numérico si ya está tomado o reservado.
- **FR-007**: Cambiar el nombre y el color del menú MUST requerir el permiso de configurar el
  negocio, verificado en el servidor.

**QR y compartir**

- **FR-008**: Cada negocio MUST tener un código de QR estable, que no cambia aunque cambie el
  nombre; su dirección `<base>/q/<código>` lleva al menú con el nombre vigente. El código no revela
  el número interno del negocio y no se puede adivinar recorriendo números.
- **FR-009**: Venta en web MUST mostrar la dirección del menú, el QR y los botones Copiar, Abrir,
  Compartir (el menú de compartir del dispositivo, si existe; si no, copiar) y Descargar en PNG, SVG
  y tarjeta imprimible (QR, nombre del negocio y dirección legible).
- **FR-010**: La página del menú MUST traer en el HTML inicial —sin depender de que el navegador
  ejecute código— el título, una descripción y una imagen para la vista previa al compartir. La
  imagen es la foto de un producto del menú en su tamaño chico; sin fotos, una imagen por omisión
  del menú.

**Contenido del menú**

- **FR-011**: El menú MUST mostrar solo los productos que la regla de venta en web ya decide (activos,
  en temporada, con «Web» encendido), con el precio que esa misma regla calcula (web o el de
  mostrador). No puede existir una segunda regla de qué se vende en web ni de su precio.
- **FR-012**: «Agotado» MUST salir de la función única de existencia del lugar de venta (#326/#094);
  el menú no calcula existencias.
- **FR-013**: Los productos MUST agruparse por su categoría, en el orden de categorías y productos
  que el negocio ya definió; una categoría sin productos en web no aparece.
- **FR-014**: Cada producto MUST mostrar nombre, descripción para el cliente (si hay), precio, foto en
  tamaño adecuado a la pantalla (o la ilustración de la familia de su categoría; sin familia, la
  genérica) y la marca «Agotado» cuando aplique.
- **FR-015**: El menú MUST ofrecer un acceso a las categorías que permita saltar a cualquiera con un
  toque y que siga visible al bajar.
- **FR-016**: Tocar un producto MUST mostrar su foto grande y su descripción completa, y cerrarse con
  un toque; funciona también sin ejecutar código (sin ventana, la tarjeta ya muestra lo esencial).

**Marca y plantilla**

- **FR-017**: El color del menú MUST elegirse en Venta en web de la misma lista de paletas que
  ofrece Interfaz; por omisión, la paleta de marca del producto.
- **FR-018**: El menú MUST construirse con una plantilla que recibe por separado los datos del menú y
  el estilo de marca, de forma que agregar otra plantilla (#330) no cambie los datos ni el estilo.

**Seguridad y privacidad**

- **FR-019**: Lo que el menú público recibe y muestra MUST limitarse a: nombre del negocio, nombre y
  orden de categorías, nombre, descripción, precio, imagen, familia y si está agotado de cada
  producto, y el color de marca. Nunca costos, recetas, insumos, existencias numéricas, proveedores,
  identificador de inicio de sesión, usuarios ni ningún otro dato interno.
- **FR-020**: Las peticiones públicas MUST tener un límite por dirección IP; al pasarlo se responde
  «demasiadas peticiones» sin afectar al POS.
- **FR-021**: El menú de un negocio MUST mostrar solo datos de ese negocio, aunque otra empresa
  comparta la base.

**Rendimiento y frescura**

- **FR-022**: La página MUST funcionar completa sin ejecutar código en el navegador (el código, si
  lo hay, solo mejora), usar fuentes del sistema, cargar las fotos fuera de la primera pantalla solo
  al acercarse, y declarar el tamaño de cada imagen para que la página no salte.
- **FR-023**: El menú MUST guardarse en caché cerca del cliente y reflejar un cambio del catálogo o
  un agotado en máximo 2 minutos, sin que nadie tenga que limpiar nada a mano.
- **FR-024**: Si el servidor del POS no responde, el menú MUST seguir mostrándose con la última copia
  buena de hasta 24 h.

**Puertas**

- **FR-025**: La función lleva la clave estable `public_menu` (para activarla por paquete, #333);
  hoy está encendida para todas las empresas.

### Key Entities

- **Nombre de menú**: el nombre con el que un negocio aparece en la dirección pública. Un negocio
  tiene uno vigente y puede tener anteriores, que redirigen al vigente. Un nombre pertenece a un solo
  negocio para siempre.
- **Menú público del negocio**: la configuración del menú: código estable del QR y color de marca.
- **Vista pública del menú**: lo que el exterior puede saber de un negocio (FR-019). Se deriva del
  catálogo y de la regla de venta en web; no se guarda aparte.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: El elemento principal del menú de El Gato Bobah aparece en menos de 2.5 s con el perfil
  móvil de la auditoría (red 4G lenta, celular de gama baja), y la página no salta al cargar
  (desplazamiento acumulado < 0.1).
- **SC-002**: Una dueña sin contexto encuentra y comparte el enlace de su menú, y descarga su QR, en
  menos de 2 minutos desde Ajustes.
- **SC-003**: Un cliente sin contexto, en un celular, encuentra un producto de una categoría del
  final del menú y su precio en menos de 20 s.
- **SC-004**: El 100 % de las direcciones con un nombre anterior y del QR llegan al menú vigente en
  un solo salto, después de 2 cambios de nombre.
- **SC-005**: Ningún dato fuera de la lista de FR-019 aparece en la respuesta pública (verificado
  contra la respuesta completa, campo por campo).
- **SC-006**: Un cambio en el catálogo se ve en el menú público en 2 minutos o menos.

## Assumptions

- El nombre inicial se deriva del nombre del negocio, no del identificador de inicio de sesión:
  publicar el identificador de login ayudaría a quien intenta adivinar contraseñas. Si el derivado
  coincide con él por casualidad, la dueña puede cambiarlo.
- Las empresas de prueba que ya existen en producción también reciben menú; es inofensivo (solo
  muestra lo que su catálogo vende en web) y la función es para todas (FR-025).
- No hay interruptor de «publicar/despublicar» en v1: un negocio que no quiera salir apaga «Web» en
  sus productos (sale el menú vacío). Si hace falta, es una columna que se agrega después sin
  perder nada.
- El logo, la foto de portada, la tipografía y más plantillas son del #330; en v1 el encabezado es
  el nombre del negocio con el color de marca.
- Extras y modificadores no se muestran en v1 (llegan con los pedidos, #328).
- Un QR descargado como tarjeta imprimible se imprime desde el navegador (tamaño carta o media
  carta); no se genera PDF en el servidor.
- La vista previa en WhatsApp usa la foto chica del producto; que WhatsApp muestre ese formato de
  imagen se verifica al subir (queda «no verificado» hasta probarlo con un enlace real).

## Fuera de alcance

Pedidos desde el menú (#328), WhatsApp al negocio (#329), catálogo de plantillas, logo, portada y
tipografía (#330), dominio genérico, subdominio por negocio y dominio propio (#331), pago en línea
(#332), activar por paquete (#333, solo se deja la clave), extras y modificadores en el menú, varias
fotos por producto, precio o disponibilidad por sucursal.
