# Feature Specification: Nombres alternativos en productos y artículos

**Feature Branch**: `063-nombres-alternativos`

**Created**: 2026-11-02

**Status**: Draft

**Input**: Issue #221. Un mismo producto o artículo del almacén se nombra de varias formas: «Ice
Coffee», «café frío» o «café helado»; un insumo por su marca o por su nombre genérico. Hoy los
buscadores comparan solo contra el nombre, así que quien busca con otra palabra no lo encuentra. La
salida que se ha usado es meter el otro nombre entre paréntesis («Ice Coffe (Café Frío)»): ensucia
el ticket y la comanda, y ya dejó productos duplicados con nombres parecidos.

**Decisiones del dueño** (fijas, no se vuelven a preguntar):

- Quien puede editar el producto o el artículo puede editar sus nombres alternativos. No hay permiso
  nuevo.
- Hasta 10 nombres alternativos por producto o por artículo.
- La búsqueda es por coincidencia parcial y no distingue mayúsculas ni acentos. No tolera errores de
  dedo.
- Los nombres que hoy llevan el otro nombre entre paréntesis no se migran solos; la limpieza del
  catálogo va aparte.

**Fuera de alcance**: opciones de modificador (extras) y enviar los alternativos a las plataformas de
reparto.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Encontrar un producto en Vender por otro nombre (Priority: P1)

Quien cobra en el mostrador escribe «café frío» en el buscador de Vender. Aparece «Ice Coffee» con
una etiqueta que dice el alternativo que coincidió («café frío»). Lo toca, se agrega a la cuenta
como «Ice Coffee», y así sale en el ticket y en la comanda.

**Why this priority**: es donde el nombre equivocado cuesta tiempo con un cliente enfrente, y es la
razón por la que hoy se ensucian los nombres con paréntesis.

**Independent Test**: dar a «Ice Coffee» el alternativo «Café frío», buscar «cafe fri» en Vender,
ver el producto con «café frío» señalado, venderlo e imprimir: ticket y comanda dicen «Ice Coffee»
y no mencionan el alternativo.

**Acceptance Scenarios**:

1. **Given** «Ice Coffee» con el alternativo «Café frío», **When** en Vender se escribe «café frío»,
   «CAFE FRIO» o «fri», **Then** aparece «Ice Coffee» con la indicación del alternativo que
   coincidió.
2. **Given** el mismo producto, **When** se escribe «ice», **Then** aparece «Ice Coffee» sin
   indicación de alternativo (coincidió por su nombre).
3. **Given** el producto vendido desde una búsqueda por alternativo, **When** se imprime el ticket y
   la comanda, **Then** los dos dicen «Ice Coffee» y ninguno menciona «Café frío».
4. **Given** una búsqueda con un error de dedo («cafe fiio»), **When** se escribe, **Then** no
   aparece: no hay tolerancia a errores.
5. **Given** un producto que coincide por nombre y otro que coincide solo por alternativo, **When**
   se busca, **Then** los que coinciden por nombre van primero, como hoy van antes que los que
   coinciden por código.

---

### User Story 2 - Capturar los nombres alternativos de un producto (Priority: P1)

Quien configura el menú abre «Ice Coffee» en Productos y en «Otros nombres» agrega «Café frío» y
«Café helado». Al guardar, quedan ligados al producto. Si escribe uno que ya es el nombre de otro
producto, o un alternativo de otro producto, la pantalla dice con cuál choca y no lo guarda.

**Why this priority**: sin captura no hay qué buscar; va junto con la US1.

**Independent Test**: agregar dos alternativos a un producto, recargar y verlos; intentar agregar el
nombre de otro producto y ver el aviso que lo nombra.

**Acceptance Scenarios**:

1. **Given** un producto sin alternativos, **When** se agregan «Café frío» y «Café helado» y se
   guarda, **Then** el producto los conserva en ese orden, también después de recargar.
2. **Given** otro producto llamado «Frappé de café», **When** se intenta agregar «frappe de cafe»
   como alternativo de «Ice Coffee», **Then** no se guarda y el aviso dice que ya es el nombre de
   «Frappé de café».
3. **Given** «Latte» con el alternativo «Café con leche», **When** se intenta agregar «café  con
   leche» (otro espaciado, sin acento) a «Capuchino», **Then** no se guarda y el aviso dice que ya
   es otro nombre de «Latte».
4. **Given** un producto con 10 alternativos, **When** se intenta agregar el undécimo, **Then** la
   pantalla no lo deja y dice que el máximo es 10; el servidor rechaza también una lista de 11.
5. **Given** un alternativo igual al nombre del propio producto, o dos alternativos iguales entre sí
   en la misma lista, **When** se guarda, **Then** se rechaza diciendo cuál está repetido.
6. **Given** un alternativo vacío o de puros espacios, **When** se guarda, **Then** no se guarda
   como alternativo; los espacios de más alrededor y en medio se recortan.
7. **Given** «Ice Coffee» con el alternativo «Café frío», **When** alguien renombra otro producto a
   «Cafe frio» o da de alta uno con ese nombre, **Then** se rechaza diciendo que ya es otro nombre
   de «Ice Coffee».
8. **Given** «Café frío de olla» y la ficha de «Ice Coffee», **When** se escribe «Café frío olla»
   como alternativo, **Then** antes de agregarlo la pantalla avisa que se parece a «Café frío de
   olla», y aun así deja agregarlo y guardarlo.
9. **Given** una persona sin permiso de editar productos, **When** intenta guardar alternativos (por
   pantalla o directo al servidor), **Then** se le niega igual que cualquier otra edición del
   producto.

---

### User Story 3 - Encontrar un producto en Productos y en recetas por otro nombre (Priority: P2)

En la lista de Productos, buscar «café frío» trae «Ice Coffee» señalando el alternativo. Lo mismo
al armar la receta de un combo y elegir un producto que lo forma.

**Why this priority**: la administración del catálogo sufre el mismo problema, pero no con un
cliente enfrente.

**Independent Test**: con el alternativo capturado, buscar en Productos y en el selector de
productos de una receta.

**Acceptance Scenarios**:

1. **Given** «Ice Coffee» con «Café frío», **When** en Productos se busca «cafe frio», **Then**
   aparece con el alternativo señalado, y el conteo de resultados lo incluye.
2. **Given** lo mismo, **When** se elige un producto para la receta de un combo y se escribe «café
   frío», **Then** aparece «Ice Coffee» con el alternativo señalado.

---

### User Story 4 - Nombres alternativos en los artículos del almacén (Priority: P2)

Quien lleva el almacén abre «Leche entera» en Almacén › Artículos y agrega «Leche Lala» y «Leche de
vaca». Desde ese momento, buscar «lala» en Artículos, en el conteo, en un renglón de un gasto o al
agregar un insumo a una receta encuentra «Leche entera» y dice que coincidió con «Leche Lala».

**Why this priority**: mismo problema en el almacén, donde un insumo se nombra por su marca.

**Independent Test**: capturar dos alternativos a un artículo y buscarlo por uno de ellos en
Artículos, Conteo, captura de gastos y receta.

**Acceptance Scenarios**:

1. **Given** «Leche entera» con «Leche Lala», **When** se busca «lala» en Artículos, en el conteo,
   al elegir el artículo de un renglón de gasto o al agregar un insumo a una receta, **Then**
   aparece «Leche entera» con el alternativo señalado.
2. **Given** otro artículo llamado «Leche Lala», **When** se intenta agregar «leche lala» como
   alternativo de «Leche entera», **Then** no se guarda y el aviso dice que ya es el nombre de ese
   artículo.
3. **Given** «Leche entera» con «Leche Lala», **When** al capturar un gasto o una receta alguien
   intenta crear un artículo nuevo llamado «Leche lala», **Then** no se crea otro: se le ofrece
   «Leche entera», igual que hoy se ofrece el artículo que ya existe con ese mismo nombre.
4. **Given** «Leche entera» con «Leche Lala», **When** se renombra otro artículo a «leche lala»,
   **Then** se rechaza diciendo con cuál choca.
5. **Given** un artículo con 10 alternativos, **When** se intenta el undécimo, **Then** se rechaza
   en pantalla y en el servidor.
6. **Given** un producto «Leche» y un artículo «Leche entera» con el alternativo «Leche», **When**
   se guarda, **Then** se acepta: productos y artículos son catálogos distintos.

---

### Edge Cases

- **Choque con algo archivado o inactivo**: un producto inactivo o un artículo archivado sigue
  reservando su nombre y sus alternativos (como hoy reserva su nombre). El aviso lo dice («… de
  «X», archivado») para que quien captura sepa dónde buscar.
- **Un artículo unido a otro** (origen de una unión de duplicados) no reserva nada: después de unir
  «Leche Lala» en «Leche entera», «Leche Lala» se puede capturar como alternativo de «Leche
  entera».
- **Texto escrito sin tocar «Agregar»**: al guardar la ficha (o salir de la del artículo) se agrega;
  si choca, la ficha no se cierra y muestra el aviso.
- **Dos personas a la vez**: dos tabletas que guardan el mismo alternativo en dos productos
  distintos al mismo tiempo: gana una y la otra recibe el aviso de choque; nunca quedan las dos.
- **Otra empresa**: un alternativo de otra empresa no choca, no se ve en ningún buscador y no se
  nombra en ningún aviso.
- **Espacio duro o tabulador** dentro de un alternativo: cuenta como un espacio al comparar (lo
  mismo que ya pasa con nombres de insumos y motivos de merma).
- **Una tableta con la versión anterior de la pantalla** durante una subida: edita el producto o el
  artículo sin tocar sus alternativos; nunca los borra por no mandarlos.
- **Duplicar un producto**: la copia no hereda los alternativos (chocarían con el original).
- **Unir artículos**: el destino conserva sus alternativos; los del origen se quedan con el origen
  archivado (pasarlos al destino queda para otra feature).
- **Cambiar la unidad de un artículo que ya tiene compras** (se crea otro y el viejo se archiva): los
  alternativos pasan al artículo nuevo; el archivado se queda sin ellos.
- **La búsqueda coincide con el nombre y con un alternativo a la vez**: se muestra como coincidencia
  por nombre, sin señalar alternativo.
- **Varios alternativos coinciden**: se señala el primero en el orden capturado.
- **La pantalla de venta tenía el menú en memoria**: los alternativos nuevos llegan con la siguiente
  revisión del menú, igual que un cambio de nombre o de precio.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: Cada producto y cada artículo del almacén MUST poder tener de 0 a 10 nombres
  alternativos, en el orden en que se capturaron.
- **FR-002**: Un nombre alternativo MUST tener de 1 a 60 letras (el largo del nombre de un
  artículo); se guarda sin espacios de más (recortados y juntados). Uno vacío o de puros espacios se
  descarta sin error.
- **FR-003**: Comparar nombres para decidir si chocan MUST ignorar mayúsculas, acentos (á é í ó ú
  ü ñ) y espacios de más, con la misma regla que ya decide que dos insumos se llaman igual.
- **FR-004**: Un alternativo de un producto MUST NOT coincidir (FR-003) con el nombre principal de
  ningún producto de la empresa —incluido el suyo— ni con un alternativo de otro producto de la
  empresa, ni repetirse en la misma lista. Lo mismo para artículos entre artículos. Productos y
  artículos no chocan entre sí.
- **FR-005**: El nombre principal de un producto o artículo, al darlo de alta o renombrarlo, MUST NOT
  coincidir (FR-003) con un alternativo de otro de su catálogo.
- **FR-006**: Un choque MUST rechazarse sin guardar nada de la lista y avisar qué nombre chocó y con
  qué producto o artículo (y si está inactivo o archivado). Nunca nombra algo de otra empresa.
- **FR-007**: Dos guardados simultáneos que crearían un choque MUST resultar en uno guardado y otro
  rechazado con el aviso de FR-006.
- **FR-008**: Editar los alternativos MUST requerir el mismo permiso que editar el producto o el
  artículo, verificado en el servidor.
- **FR-009**: Una edición del producto o del artículo que no mande sus alternativos MUST dejarlos
  como estaban.
- **FR-010**: Los buscadores de Vender, Productos, Artículos, Conteo, captura de gastos (elegir el
  artículo de un renglón) y recetas (elegir un insumo o un producto) MUST encontrar un producto o
  artículo cuando la búsqueda coincide con su nombre principal o con cualquiera de sus alternativos,
  con la misma forma de coincidir que cada buscador ya usa para el nombre (parcial, o por inicio de
  palabra en el conteo). Ninguno de los seis distingue mayúsculas ni acentos, tampoco en el nombre
  principal: el buscador de Productos, que hoy sí distingue acentos, deja de hacerlo.
- **FR-011**: Cuando un resultado coincidió solo por un alternativo, el renglón MUST mostrar cuál
  (el primero que coincide, en el orden capturado). Cuando coincide por el nombre principal, no se
  señala alternativo.
- **FR-012**: En Vender, los que coinciden por nombre principal MUST ir antes que los que coinciden
  solo por alternativo, y éstos antes que los que coinciden solo por código.
- **FR-013**: Ticket, comanda, reportes, exportaciones y lo que se envía a plataformas de reparto
  MUST seguir usando solo el nombre principal; los alternativos no aparecen ahí.
- **FR-014**: Crear un artículo desde la captura de un gasto o una receta con un nombre que coincide
  (FR-003) con un alternativo de un artículo existente MUST ofrecer ese artículo en vez de crear
  otro, como hoy pasa con un nombre igual.
- **FR-015**: Los alternativos de una empresa MUST quedar aislados de las demás en lectura, búsqueda
  y validación de choques, probado en los tres casos: otra empresa, conexión reciclada y sin
  empresa.
- **FR-016**: Duplicar un producto MUST NOT copiar sus alternativos.
- **FR-017**: La pantalla de edición MUST mostrar los alternativos del producto o artículo y
  permitir agregar, quitar y guardarlos; con 10 no ofrece agregar otro.
- **FR-018**: El cambio de esquema MUST convivir con la versión anterior de la API corriendo durante
  la subida (la versión anterior sigue creando y editando productos y artículos sin error).
- **FR-019**: Cuando un cambio de unidad reemplaza un artículo por otro, los alternativos MUST pasar
  al nuevo y dejar de reservarse en el archivado.
- **FR-020**: Mientras se escribe un nombre alternativo, la pantalla MUST avisar —sin impedir
  guardarlo— si hay otros productos (o artículos, en su catálogo) con un nombre principal o
  alternativo parecido, nombrando hasta 5. «Parecido» es la misma regla con la que hoy se ofrecen
  insumos parecidos al unir duplicados. Un nombre idéntico (FR-003) no es aviso: es el bloqueo de
  FR-004.
- **FR-021**: Un artículo que es origen de una unión de duplicados MUST NOT contar para el choque
  de FR-004, ni con su nombre ni con sus alternativos.

### Key Entities

- **Nombre alternativo**: otro nombre con el que se conoce a un producto o a un artículo del
  almacén. Pertenece a uno solo; tiene un orden dentro de su lista; es único en su catálogo y
  empresa bajo la comparación de FR-003 junto con los nombres principales.
- **Producto** y **Artículo del almacén**: ya existen; ganan su lista de nombres alternativos. Su
  nombre principal sigue siendo el único que viaja a ticket, comanda, reportes y plataformas.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: En los seis buscadores nombrados, buscar por cualquier alternativo capturado encuentra
  el producto o artículo en el 100% de los casos probados, y el renglón dice qué alternativo
  coincidió.
- **SC-002**: Ningún ticket ni comanda impreso después del cambio contiene un nombre alternativo.
- **SC-003**: Ningún par de productos (o de artículos) de la misma empresa queda con nombres que
  coinciden bajo FR-003 por un alternativo, aun con dos guardados simultáneos.
- **SC-004**: Una persona sin contexto, a 1024×600, captura dos alternativos a un producto y lo
  encuentra en Vender por uno de ellos sin ayuda, en menos de 1 minuto.
- **SC-005**: Un alternativo capturado aparece en la búsqueda de Vender de otra tableta en menos de
  2 minutos, sin recargar a mano.

## Assumptions

- «Quien puede editar el producto» es quien hoy puede guardar la ficha de un producto en Productos;
  «quien puede editar el artículo» es quien hoy puede editar su ficha en Almacén › Artículos.
- El largo máximo de un alternativo es 60 letras en los dos catálogos: el de un artículo; un
  producto no tiene tope de nombre, y 60 cabe en el renglón de cualquier buscador.
- Los buscadores que ya miran otras cosas además del nombre (código en Vender, empaque y marca en
  Artículos y Conteo) las siguen mirando; los alternativos se suman.
- Otros buscadores que listan productos o artículos fuera de los seis nombrados (por ejemplo
  emparejar con la plataforma o unir artículos) no cambian en esta feature.
- Los productos de sistema (el genérico de plataforma) no llevan alternativos.
