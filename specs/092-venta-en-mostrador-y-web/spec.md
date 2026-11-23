# Feature Specification: Dónde se vende cada producto (mostrador y web), vender sin existencia, precio web y Configuración con menú lateral

**Feature Branch**: `092-venta-en-mostrador-y-web`

**Created**: 2026-11-23

**Status**: Draft

**Input**: Issue #326 (parte del menú en línea, #316), con las decisiones del dueño del 2026-11-23:
dónde se vende cada producto (mostrador, web o los dos), vender sin existencia, precio web propio
en una sección «Venta en web» del mismo producto, y la pantalla de configuración del negocio
reorganizada con un menú lateral por secciones.

## Contexto

Hoy un producto se vende en mostrador y, con su precio propio, en cada plataforma de reparto. El
menú público (#316, etapa 2) agrega un lugar de venta nuevo: **la web**. El negocio necesita decidir
por producto dónde se ofrece, qué pasa cuando se acaba y si en la web cuesta distinto.

Lo que ya existe y se mide antes de diseñar (respaldo de producción del 2026-11-23, restaurado en
local):

| Hecho | Medido |
| --- | --- |
| Catálogo de lugares de venta por empresa (`pos`, `qr`, `online`) | Existe desde la 0004, con sus 3 filas en las dos empresas. Una empresa creada desde la consola **no** recibe esas filas |
| Visibilidad de un producto por lugar de venta | Existe desde la 0004, con la regla «sin fila = visible». **0 filas** en producción: hoy todo producto se ve en todos lados |
| La pantalla de venta ya respeta «oculto en mostrador» | Sí: el menú del POS excluye un producto con fila «oculto» para `pos` |
| `products.allow_oversell` («vender sin existencia») | Existe desde la 0004 con default `true`, pero **ninguna regla de venta lo lee** |
| Valores de `allow_oversell` en El Gato Bobah | 178 de 540 productos en `false`; 23 de ellos cuentan su propia existencia. Vienen de la columna «Vender sin stock» del export de FUDO, importada tal cual y nunca aplicada en este sistema |

**¿`allow_oversell` sirve tal cual?** La columna sí: significa exactamente lo que el dueño pidió
(la llenó la columna «Vender sin stock» de FUDO). **Sus valores actuales no**: aplicarlos como están
haría que, el día de la subida, 23 productos que hoy se venden sin problema salgan «Agotado» en el
mostrador por existencias que el negocio no ha conciliado desde FUDO. Por eso la subida los deja
todos en encendido —que es justo como se comportan hoy— y a partir de ahí cada producto se apaga a
propósito. No se crea otra columna.

## Cómo lo resuelven otros sistemas

Ver la sección [Investigación de mercado](#investigación-de-mercado) al final. Lo que se tomó:
casillas por lugar de venta en el mismo producto (Square, Toast, Shopify), «Agotado» visible y sin
poder pedirse en vez de ocultar (Toast, Square Online), y precio en línea que hereda el de mostrador
mientras no se capture otro (la casilla «Usar precio original» de Fudo).

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Decidir dónde se vende cada producto (Priority: P1)

Quien configura el menú abre un producto y, en la pestaña «Disponibilidad» (o en «Opciones avanzadas»
al darlo de alta), ve dos casillas grandes: «Mostrador» y «Web», las dos encendidas. Apaga «Mostrador» en un producto que solo quiere ofrecer
por la web (o al revés) y guarda.

**Why this priority**: es la base de todo lo demás: el menú público (#316) solo puede mostrar lo que
el negocio decidió vender ahí, y sin esto el mostrador tampoco puede retirar un producto sin
archivarlo.

**Independent Test**: crear un producto sin tocar nada y comprobar que sale en Vender y que queda
marcado para la web; apagar «Mostrador» y comprobar que desaparece de Vender sin dejar de existir en
el catálogo.

**Acceptance Scenarios**:

1. **Given** un producto recién creado sin tocar opciones avanzadas, **When** se abre Vender,
   **Then** el producto aparece, y en su pestaña «Disponibilidad» las casillas «Mostrador» y «Web»
   están encendidas.
2. **Given** cualquier producto que ya existía antes de esta subida, **When** se abre su ficha,
   **Then** las dos casillas están encendidas y Vender lo sigue mostrando como antes.
3. **Given** un producto con «Mostrador» apagado, **When** se abre Vender, **Then** no aparece, ni
   en su categoría ni en la búsqueda, y sigue en la lista de productos del menú.
4. **Given** un producto con «Web» apagado, **When** el sistema arma la lista de lo que se vende en
   la web, **Then** el producto no está en ella.
5. **Given** un producto con las dos casillas apagadas, **When** se guarda, **Then** se guarda y la
   lista de productos lo marca como «No se vende en ningún lado», para que no se confunda con un
   error.

---

### User Story 2 - Que un producto se agote solo (Priority: P1)

Quien configura el menú apaga «Vender sin existencia» en un producto que descuenta del almacén (por
ejemplo, un refresco, que descuenta una lata por pieza). Cuando la existencia llega a cero, el producto sale
«Agotado» en Vender —se ve, pero no se puede agregar a la cuenta— y lo mismo en la web.

**Why this priority**: es lo que evita prometerle a un cliente algo que ya no hay, en el mostrador
hoy y en la web en cuanto exista el menú público.

**Independent Test**: con un producto cuya existencia está en cero y «Vender sin existencia»
apagado, intentar venderlo en Vender: la tarjeta dice «Agotado» y el servidor rechaza la venta
aunque la pantalla esté desactualizada.

**Acceptance Scenarios**:

1. **Given** un producto recién creado, **When** se abre su ficha, **Then** «Vender sin existencia»
   está encendido.
2. **Given** un producto con «Vender sin existencia» apagado y sin existencia para uno en la
   sucursal, **When** se abre Vender, **Then** la tarjeta se ve atenuada con la
   etiqueta «Agotado» y tocarla no agrega nada.
3. **Given** ese mismo producto, **When** una tableta con la pantalla desactualizada envía a cocina
   o cobra una cuenta en captura que lo incluye (o lo agrega a un pedido abierto), **Then** el
   servidor la rechaza diciendo qué producto se agotó, y la pantalla lo marca como agotado. Un pedido
   ya creado no se vuelve a revisar al cobrarlo: su existencia se descontó al crearlo.
4. **Given** un producto con existencia 1 y «Vender sin existencia» apagado, **When** alguien pide
   2, **Then** se rechaza diciendo que solo queda 1.
5. **Given** un producto agotado, **When** se registra una compra o un conteo que le devuelve
   existencia, **Then** deja de salir «Agotado» en la siguiente revisión del menú (a lo más un
   minuto) sin recargar la pantalla.
6. **Given** un pedido que llega de una plataforma de reparto con un producto agotado, **When** se
   recibe, **Then** se acepta igual y sale a cocina (la plataforma ya lo vendió; ver constitución,
   «quien opera nunca decide en el momento»).
7. **Given** un producto con existencia en cero y «Vender sin existencia» encendido, **When** se
   vende, **Then** se vende como hoy (la existencia queda negativa).

---

### User Story 3 - Precio y datos para la web en el mismo producto (Priority: P2)

Quien configura el menú abre un producto (a pantalla completa) y entra a la pestaña «Venta en web». Ahí ve si el producto
se vende en la web, el precio web —vacío, mostrando el de mostrador como el que se usará— y la
descripción para el cliente. Escribe un precio propio para la web y guarda; el precio de mostrador
no cambia.

**Why this priority**: el precio web es opcional y no lo consume nadie hasta el menú público; lo
que sí urge es que el dato quede guardado en el lugar correcto y no en el selector de plataformas.

**Independent Test**: capturar un precio web distinto, guardar, y comprobar que Vender sigue
cobrando el de mostrador y que la lista de la web usa el capturado; borrar el precio web y comprobar
que la web vuelve al de mostrador.

**Acceptance Scenarios**:

1. **Given** un producto sin precio web, **When** se abre «Venta en web», **Then** el campo está
   vacío y dice que se usará el precio de mostrador, con la cifra.
2. **Given** un precio web capturado, **When** se guarda, **Then** el precio de mostrador, el de
   cada plataforma y lo que cobra Vender no cambian.
3. **Given** un precio web capturado, **When** se borra el campo y se guarda, **Then** la web vuelve
   a usar el de mostrador (vacío es «hereda», nunca «cero»).
4. **Given** un precio web de 0, **When** se guarda, **Then** se rechaza: un producto gratis en la
   web no se captura por accidente (para regalar está la puerta de promociones).
5. **Given** un precio web absurdo (negativo, más de dos decimales, más del tope de dinero), **When**
   se guarda, **Then** se rechaza con un mensaje claro y no se guarda nada.
6. **Given** que se cambia el precio de mostrador de un producto sin precio web, **When** se mira la
   web, **Then** el precio web sigue al de mostrador.

---

### User Story 4 - Configuración ordenada por secciones (Priority: P2)

Quien administra el negocio entra a Ajustes → Negocio y ve, a la izquierda, un menú con las
secciones (Negocio, Venta en mostrador, Venta en web, Caja, Inventario, Personal); toca una y a la
derecha ve solo sus ajustes. Ya no tiene que bajar por una sola página larga. Si prefiere, cambia la
vista a íconos o a índice con el selector de la barra de título.

**Why this priority**: la pantalla ya junta once bloques y el menú público va a sumar más (enlace,
QR, marca). Reordenarla ahora cuesta poco; hacerlo cuando tenga veinte cuesta más.

**Independent Test**: abrir la pantalla a 1280×854 y llegar a cualquier ajuste que hoy existe con a
lo más dos toques desde que abre, sin desplazarse por secciones ajenas.

**Acceptance Scenarios**:

1. **Given** quien tiene permiso de configurar el negocio, **When** abre la pantalla, **Then** ve el
   menú de secciones y la primera abierta.
2. **Given** cualquier ajuste que existía antes, **When** se busca, **Then** está en alguna sección
   y funciona igual que antes (no se pierde ni se duplica ninguno).
3. **Given** quien no tiene un permiso de una sección (p. ej. configurar asistencia), **When** abre
   la pantalla, **Then** esa sección no aparece en el menú.
4. **Given** que se recarga la página o se comparte el enlace, **When** abre, **Then** abre en la
   misma sección.
5. **Given** la sección «Venta en web», **When** se abre, **Then** dice cuántos productos se venden
   en la web y cuántos tienen precio web propio, con un acceso al menú para cambiarlos.
6. **Given** que cambia a la vista de íconos o de índice, **When** recarga, **Then** abre en la
   misma vista, y el selector está en el mismo lugar en las tres.

### Edge Cases

- **Vacío que significa algo**: precio web vacío es «hereda el de mostrador», nunca cero. Un precio
  web de 0 se rechaza.
- **Fila ausente**: un producto sin ningún registro de lugar de venta se vende en los dos. Esa es la
  regla para los 1,043 productos que ya existen y para toda empresa nueva; nunca se interpreta como
  «no se vende».
- **Empresa nueva creada desde la consola**: hoy no recibe su catálogo de lugares de venta; con esta
  feature lo recibe al crearse, y la subida se lo da a las que no lo tengan.
- **Existencia por sucursal**: «Agotado» se calcula con la existencia de la sucursal donde se vende,
  no la suma de la empresa.
- **Qué es «la existencia» de un producto**: lo que el producto descuenta al venderse. Con
  existencia propia, la suya; con receta, la de cada insumo de su receta base (lo que se compra hecho,
  como un refresco, es receta de un insumo por pieza); un paquete, la de sus piezas. Sale «Agotado»
  cuando con lo que hay no alcanza para **uno**. Los extras que el cliente elige no cuentan: dependen
  de la elección, no del producto.
- **Producto que no descuenta nada** (sin receta, «no gasta insumos» o lo restan sus opciones): no
  hay existencia que medir; la casilla «Vender sin existencia» no aparece para él y nunca se agota.
- **Pantalla desactualizada**: la tarjeta puede decir disponible y el producto ya estar agotado; el
  servidor es quien decide al cobrar.
- **Dos tabletas venden la última pieza a la vez**: solo una venta pasa; la otra recibe «se agotó».
- **Pedido de plataforma**: nunca se rechaza por existencia ni por estar apagado para mostrador.
- **Un producto apagado para mostrador mientras está en una cuenta**: si el pedido ya se creó (ya
  salió a cocina), se cobra como siempre; si la cuenta sigue en captura sin enviar, al enviarla el
  servidor responde que el menú cambió y la tableta recarga y pide elegir de nuevo (el mismo camino
  que un producto archivado).
- **Las dos casillas apagadas**: se permite, y la lista de productos lo dice.
- **La API vieja durante la subida**: la versión anterior sigue creando productos y cobrando
  mientras la nueva arranca; nada de lo que agrega esta feature le cambia el contrato.
- **Duplicar un producto**: el duplicado hereda sus lugares de venta y su precio web.

## Requirements *(mandatory)*

### Functional Requirements

**Dónde se vende**

- **FR-001**: Todo producto MUST tener, por cada lugar de venta (hoy Mostrador y Web), si se ofrece
  o no. La ausencia de decisión MUST significar «se ofrece».
- **FR-002**: Todo producto existente y todo producto nuevo, en toda empresa, MUST quedar
  ofreciéndose en Mostrador y Web sin que nadie lo marque.
- **FR-003**: Las casillas «Mostrador» y «Web», encendidas por omisión y de al menos 44 px de alto,
  MUST estar en el alta (dentro de «Opciones avanzadas», cerrado, para no estorbar el alta normal) y
  en la edición (pestaña «Disponibilidad» de la ficha a pantalla completa; ver FR-028).
- **FR-004**: Un producto apagado para Mostrador MUST no aparecer en Vender (categorías, búsqueda,
  favoritos y «Top»), y el servidor MUST rechazar agregarlo a un pedido de mostrador.
- **FR-005**: El sistema MUST ofrecer una regla única que diga qué productos se venden en la web y
  a qué precio, para que el menú público (#316) la use sin volver a decidirla.
- **FR-006**: La lista de productos del menú MUST distinguir los que no se venden en ningún lado.
- **FR-007**: La forma de guardar el lugar de venta MUST admitir después más lugares (cada
  plataforma, otra sucursal) sin migrar las decisiones ya guardadas.
- **FR-008**: Toda empresa MUST tener su catálogo de lugares de venta: las que ya existen lo
  reciben en la subida si les falta, y las nuevas al crearse.

**Vender sin existencia**

- **FR-009**: Todo producto MUST tener «Vender sin existencia», encendido por omisión al crearse.
- **FR-010**: Al subir esta feature, todo producto existente MUST quedar con «Vender sin
  existencia» encendido (es como se comporta hoy), para que ningún producto amanezca agotado.
- **FR-011**: La casilla MUST mostrarse solo en productos que descuentan algo del almacén al
  venderse (existencia propia, receta o paquete).
- **FR-012**: Un producto con «Vender sin existencia» apagado cuya existencia en la sucursal (la
  suya, la de los insumos de su receta base o la de las piezas del paquete) no alcanza para uno MUST
  salir «Agotado» en Vender (visible, atenuado, sin poder agregarse) y
  marcarse como agotado en la regla de la web (FR-005). No se oculta.
- **FR-013**: El servidor MUST rechazar un pedido de mostrador cuando lo que descuenta la cantidad
  pedida de un producto con «Vender sin existencia» apagado supera la existencia en la sucursal, nombrando el
  producto y lo que queda; dos ventas simultáneas de la última pieza MUST dejar pasar solo una.
- **FR-014**: Los pedidos de plataforma MUST aceptarse sin importar existencia ni lugar de venta.
- **FR-015**: Vender MUST enterarse de un producto agotado o repuesto en a lo más un minuto, sin
  recargar, y al instante cuando el servidor rechaza una venta por existencia.

**Venta en web del producto**

- **FR-016**: La ficha del producto MUST tener una pestaña «Venta en web» que junte: si se vende en
  la web (el mismo dato que la casilla de FR-003), el precio web y la descripción para el cliente.
- **FR-017**: El precio web MUST ser opcional, con una casilla «Usar el precio de mostrador ($N)»
  encendida por omisión; encendida (o el campo vacío) MUST significar «el de mostrador», y la pestaña
  MUST mostrar la cifra que se usará.
- **FR-018**: El precio web MUST validarse como cualquier precio (mayor que cero, dos decimales,
  tope de dinero) y MUST no cambiar el precio de mostrador ni los de plataforma.
- **FR-019**: El precio web MUST capturarse en la ficha del producto, no en el selector de
  plataformas de reparto.
- **FR-020**: Duplicar un producto MUST copiar sus lugares de venta y su precio web.

**Ficha del producto**

- **FR-028**: La edición de un producto MUST abrirse a pantalla completa, con un menú lateral de
  pestañas por tema: **General** (nombre, precio, costo, categoría, otros nombres, favorito, activo,
  pasa por cocina), **Disponibilidad** (Mostrador y Web, temporada), **Venta en web**, **Receta e
  inventario** (receta, vender sin existencia, alertas) y **Extras** (grupos modificadores). Todo lo
  que hoy tiene la ficha MUST quedar en exactamente una pestaña; a 1280×854 y a 1024×600 ninguna
  pestaña MUST obligar a deslizar con un producto típico (hasta 4 grupos de extras).
- **FR-029**: «Guardar» MUST guardar lo de todas las pestañas en una sola acción (salvo Extras, que
  se guarda al momento como hoy y lo confirma), cambiar de pestaña MUST no perder lo capturado, y
  «Guardar» MUST poder tocarse siempre: si algo no es válido, la ficha MUST ir a la pestaña del error
  y marcar el campo.
- **FR-030**: Con «Usar el precio de mostrador» apagado, el precio web MUST ser obligatorio. Apagar
  la web MUST conservar el precio web y la descripción ya capturados.

**Configuración**

- **FR-021**: La pantalla de configuración del negocio MUST organizarse por secciones: Negocio,
  Venta en mostrador, Venta en web, Caja, Inventario y Personal, con tres vistas que se cambian con
  un selector fijo en la barra de título (siempre en el mismo lugar): **A** menú lateral con
  nombres (por omisión), **B** menú lateral de íconos e **C** índice de secciones. La tableta
  recuerda la vista elegida.
- **FR-022**: Todo ajuste que hoy está en esa pantalla MUST quedar en exactamente una sección, con
  el mismo comportamiento.
- **FR-023**: Una sección cuyo contenido el usuario no puede ver MUST no aparecer en el menú.
- **FR-024**: La sección abierta MUST sobrevivir a una recarga y poder enlazarse.
- **FR-025**: La sección «Venta en web» MUST mostrar cuántos productos se venden en la web y cuántos
  tienen precio web propio, con un acceso a la lista de productos.

**Permisos y rastro**

- **FR-026**: Cambiar lugares de venta, «Vender sin existencia» y precio web MUST exigir el mismo
  permiso que hoy edita un producto (`menu.configure`); la configuración, el de hoy de esa pantalla.
- **FR-027**: Toda regla nueva MUST respetar el aislamiento entre empresas en los tres casos (otra
  empresa, conexión reciclada, sin empresa).

### Key Entities

- **Lugar de venta**: dónde se le ofrece un producto a un cliente. Hoy: Mostrador y Web. Pertenece a
  la empresa. Mañana puede ser cada plataforma o cada sucursal.
- **Decisión de un producto en un lugar de venta**: si se ofrece ahí y, opcionalmente, a qué precio.
  Sin decisión guardada: se ofrece, al precio de mostrador.
- **Vender sin existencia**: atributo del producto, igual para todos los lugares de venta.
- **Existencia de un producto**: lo que la sucursal tiene de lo que el producto descuenta al
  venderse (él mismo, los insumos de su receta base o las piezas de su paquete).

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Tras la subida, el 100% de los productos existentes siguen apareciendo en Vender y
  ninguno sale «Agotado» (medido sobre la base restaurada de producción).
- **SC-002**: Un producto creado sin tocar opciones avanzadas queda vendiéndose en Mostrador y Web
  en el 100% de los casos.
- **SC-003**: Un producto agotado no se puede cobrar en mostrador, ni desde una pantalla
  desactualizada (0 ventas aceptadas por encima de la existencia con la opción apagada).
- **SC-004**: Quien configura llega a cualquier ajuste de la pantalla de configuración en a lo más
  dos toques desde que abre, a 1280×854.
- **SC-007**: En la ficha del producto, ninguna pestaña obliga a deslizar a 1024×600 con un producto
  típico (medido en la maqueta elegida: la más alta, 464 px de 464 a la vista).
- **SC-005**: Una persona nueva, de una fonda, logra sin ayuda: apagar un producto para la web,
  ponerle precio web y hacer que otro se agote solo; en la prueba con alguien sin contexto.
- **SC-006**: Un producto repuesto deja de salir «Agotado» en a lo más un minuto.

## Fuera de alcance

- El menú público en sí (página, enlace, QR, marca): #316.
- Cupo de piezas por lugar de venta (decisión del dueño).
- Agotar según los extras elegidos (p. ej. «no hay leche de almendra»): se puede agregar después al
  mismo costo (no exige ningún dato que hoy no se guarde).
- Lugar de venta por categoría (la tabla existe desde la 0004 y no se usa; no se toca).
- Activar por paquete: hoy es para todos (#333).
- Precio o visibilidad por sucursal.

## Assumptions

- «Web» es el lugar de venta del menú público, y el enlace y el QR apuntan al mismo menú: un solo
  lugar de venta «Web», no uno por QR.
- La descripción para el cliente es la descripción que el producto ya tiene en el catálogo (hoy no
  se muestra en ninguna pantalla).
- Las secciones de la configuración son las del issue menos «Avisos» y «Apariencia»: no existen
  ajustes de empresa de esos temas hoy (Interfaz es por tableta y vive en su propia pantalla); se
  agregan cuando haya qué poner.
- La revisión del menú que ya hace Vender cada minuto es suficiente para enterarse de un agotado.

## Investigación de mercado

Investigado el 2026-11-23 en la ayuda oficial de cada producto (y la comunidad donde se dice).
Lo no confirmado se marca así; ningún dato se infirió.

### Dónde se vende cada producto

| Sistema | Cómo | Por omisión |
| --- | --- | --- |
| Square | En el artículo, «Locations and channels» → lista de casillas de canales en línea; aparte, «Discoverable via direct link» | Configurable en *Item defaults* (y lo creado desde el POS no siempre lo toma, según la comunidad) |
| Toast | «Channel visibility» por menú, grupo, artículo y modificador: POS, kiosco, pedido en línea, socios; ocultar al padre oculta al hijo | No confirmado para artículos |
| Loyverse | Una casilla «The item is available for sale» (solo POS); no tiene tienda en línea propia | No confirmado |
| Shopify | Sección «Publishing» del producto: canales de venta (Online Store, Point of Sale…) | No confirmado (la comunidad dice que nace en borrador) |
| Fudo | El catálogo de la tienda en línea es un **menú aparte**: se agregan productos y cada uno lleva un interruptor de visibilidad | No confirmado |

### Vender sin existencia

| Sistema | Qué hace |
| --- | --- |
| Square | El POS deja quedar en negativo; en línea no deja comprar. «Sold out» se ve gris y tachado |
| Toast | Existencia en 0 bloquea («86») en POS y en línea; en línea **sigue visible, marcado y sin poder pedirse** |
| Loyverse | Global: solo avisa al cajero, que puede seguir |
| Shopify | «Continue selling when out of stock» por producto; no aplica al POS, que siempre vende |
| Fudo | Control de stock por producto o ingrediente; vender en cero y cómo se ve el agotado: no confirmado |

### Precio distinto en línea

| Sistema | Qué hace |
| --- | --- |
| Square | Solo un «Online sale price» en el mismo artículo |
| Toast | Menú en línea aparte con precio propio, en otra pantalla |
| Loyverse | No hay precio por canal (sí por tienda) |
| Shopify | Listas de precios por mercado, en otra pantalla; lo no capturado hereda del padre |
| Fudo | En el menú de la tienda, casilla **«Usar precio original»**: al desmarcarla se captura otro; o listas de precios por canal |

### Configuración

Toast usa menú lateral con un submenú «Settings» para lo poco usado; Shopify, ~18 secciones planas;
Loyverse junta interruptores en «Features»; Fudo separa «Administración» de «Tienda Online», que
trae su propio submenú lateral.

### Lo que se decidió con esto

- **Por producto y con casillas**, como Square, Toast y Shopify; nada por categoría (ninguno usa
  solo la categoría).
- **«Agotado» visible y sin poder pedirse**, como Toast y Square en línea. A diferencia de Square y
  Shopify, el mostrador también lo respeta, porque el dueño pidió que aplique igual en los dos; y a
  diferencia de Toast, solo si el negocio apaga «Vender sin existencia» en ese producto.
- **Precio web en el mismo producto, vacío = el de mostrador**, como la casilla de Fudo, pero sin
  menú aparte: el dueño pidió capturarlo en el producto, no en otra pantalla.
- **Configuración con menú lateral por áreas**, como Toast y la Tienda Online de Fudo.

Fuentes: squareup.com/help (artículos 7982, 8289, 8495, 7963), support.toasttab.com («Setting
Ordering Visibility in the Menu Builder», «86 an Item», «Show or Hide Menu Items for Toast Online
Ordering»), doc.toasttab.com, help.loyverse.com («How add items», «Negative stock alerts»),
help.shopify.com («Selling when out of stock», Markets), soporte.fu.do («Crea tu menú para tienda
online», «Control de stock», «Listas de precios»).
