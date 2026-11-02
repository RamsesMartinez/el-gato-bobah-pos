# Feature Specification: Un insumo con varias marcas (entrega 1)

**Feature Branch**: `056-unir-insumos`

**Created**: 2026-11-01

**Status**: Draft

**Input**: Issue #185, primera entrega: unir insumos duplicados con vista previa, marca opcional en la
presentación y aviso de posible duplicado al crear un insumo. La regla de costo con varias marcas
(ajuste de la empresa) y comparar marcas en el historial de costos van a un issue nuevo.

**Evidencia**: en la empresa real hay más de 15 grupos de insumos que son lo mismo con otra marca o
con otro nombre: leche, concentrados de sabor, salsas del mismo sabor de dos proveedores, tapas y
vasos de la misma medida, agua mineral, aceite, y duplicados del mismo producto escritos distinto.
Cada receta apunta a uno solo, así que la venta descuenta de la marca que quedó en la receta: una
existencia baja sin control y la otra crece aunque se esté usando.

**Principios fijos** (no se negocian en esta spec):

- Unir lo confirma siempre una persona, después de ver qué va a pasar. Nunca es automático.
- Nada se borra: los insumos de origen se archivan con su historia, y el traspaso de existencia es
  un movimiento más, ligado a la unión.
- Unir no reescribe ventas, compras ni conteos pasados: el inventario se corrige de aquí en adelante
  (decisión del dueño, 2026-10-13).
- Todo queda en la bitácora: quién unió qué en qué, y cuándo.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Unir dos marcas del mismo insumo (Priority: P1)

Quien configura el almacén abre «Leche entera» en Almacén › Artículos, elige «Unir otros
aquí» y escoge «Leche entera Marca B». Antes de confirmar ve qué pasa: cuánto queda en cada sucursal
antes y después, qué recetas dejan de usar la Marca B para usar «Leche entera», qué presentaciones
llegan y con qué costo queda. Al confirmar, la Marca B queda archivada como «unida a Leche entera»,
y desde ese momento toda venta que la usaba descuenta de «Leche entera».

**Why this priority**: es el problema del issue. Sin esto la existencia de una marca baja sin control
mientras la otra crece, y ningún conteo cuadra.

**Independent Test**: con dos insumos en g, A con 500 g y B con 300 g en la misma sucursal, y un
producto cuya receta lleva 100 g de B: unir B en A. La vista previa dice «A: 500 g → 800 g»,
«B: 300 g → 0 g» y «Receta de <producto>: pasa a A». Al confirmar, vender el producto deja A en
700 g y B no se mueve.

**Acceptance Scenarios**:

1. **Given** A y B en la misma unidad base, **When** se une B en A, **Then** la existencia de cada
   sucursal de B se suma a la de A, B queda en cero y archivada, y la vista previa lo mostró antes
   con el antes y el después por sucursal.
2. **Given** un producto, un extra y un insumo preparado cuyas recetas llevan B, **When** se une B en
   A, **Then** las tres recetas llevan A con la misma cantidad y ninguna receta, activa o no, apunta
   a B.
3. **Given** una receta que ya lleva A y B (por ejemplo 100 ml de cada uno), **When** se une B en A,
   **Then** la receta lleva un solo renglón de A con la suma (200 ml), y la vista previa lo dice.
4. **Given** B unido en A, **When** se vende un producto cuya receta llevaba B, **Then** el
   movimiento de venta es de A.
5. **Given** B unido en A, **When** se abre un conteo, **Then** A aparece con lo que había en A más
   lo que había en B, y B no aparece.
6. **Given** varios insumos de origen (B, C y D), **When** se unen en A en una sola operación, **Then**
   la vista previa los muestra juntos y al confirmar los tres quedan archivados como unidos a A.
7. **Given** una unión confirmada, **When** se consulta la bitácora del destino o de un origen,
   **Then** aparece quién unió, cuándo, qué insumos y cuánto se traspasó por sucursal.

---

### User Story 2 - Lo que se compraba de la otra marca sigue entrando al insumo (Priority: P1)

Después de unir, las presentaciones, los formatos de compra y lo que la lectura de tickets ya
aprendió de cada proveedor (el renglón «LECHE MARCA B 1L» de tal proveedor es este insumo) pasan al
destino. Una compra nueva de la Marca B entra a la existencia de «Leche entera».

**Why this priority**: sin esto, la siguiente compra de la marca que se unió vuelve a caer en el
insumo archivado o en «sin mapear», y el problema regresa en una semana.

**Independent Test**: con B unido en A y una presentación de B «Caja 12 × 1 L», capturar una compra
con esa presentación: la existencia de A sube 12,000 ml; con un ticket leído del proveedor que antes
se ligaba a B, el renglón sale ligado a A.

**Acceptance Scenarios**:

1. **Given** una presentación «Caja 12 × 1 L» de B, **When** se une B en A, **Then** la presentación
   es de A, conserva su contenido y lleva la marca (la que tenía o, si no tenía, el nombre de B).
2. **Given** B unido en A, **When** se captura una compra con la presentación que venía de B,
   **Then** la existencia de A sube lo que trae la presentación.
3. **Given** un renglón de ticket de un proveedor que estaba ligado a B, **When** se une B en A y
   llega otro ticket con ese renglón, **Then** se liga a A.
4. **Given** A y B con una presentación que se llama igual, tiene la misma marca y trae lo mismo,
   **When** se unen, **Then** queda una sola en A (la de B se archiva) y la vista previa lo dice.

---

### User Story 3 - Unir insumos que se miden distinto (Priority: P2)

«Tapa 16 oz» se dio de alta en piezas y «Tapas 16 oz (bolsa)» en gramos. Para unirlas, quien
configura escribe la equivalencia («1 pieza = 4 g», o al revés según cuál es el destino), y la vista
previa muestra cada cantidad convertida antes de confirmar.

**Why this priority**: existe en los datos reales, pero es menos frecuente que dos marcas medidas
igual; sin la equivalencia explícita la unión se niega, que es lo seguro.

**Independent Test**: unir B (en g, 400 g) en A (en piezas, 10 piezas) sin equivalencia: se rechaza
pidiéndola. Con «1 g de B = 0.25 piezas de A»: la vista previa dice «A: 10 → 110 piezas» y la receta
que llevaba 4 g de B pasa a llevar 1 pieza de A.

**Acceptance Scenarios**:

1. **Given** origen y destino con la misma unidad base, **When** se pide la vista previa, **Then** no
   se pide equivalencia.
2. **Given** origen y destino con unidad base distinta, **When** se pide la vista previa sin
   equivalencia, **Then** la pantalla pide la equivalencia y no deja confirmar.
3. **Given** una equivalencia escrita, **When** se ve la vista previa, **Then** cada existencia,
   cantidad de receta y contenido de presentación aparece convertido, y lo que quedaría en cero o no
   cabe se señala y bloquea la confirmación.

---

### User Story 4 - Marca en la presentación (Priority: P2)

En las presentaciones de un insumo, quien configura escribe opcionalmente la marca o el proveedor
(«Marca A», «Costco»). Así, en el mismo insumo se distinguen «Caja 12 × 1 L · Marca A» y
«Caja 8 × 1.5 L · Marca B», y al capturar una compra se elige la correcta.

**Why this priority**: es lo que permite tener un solo insumo con varias marcas sin perder de cuál
se compró; la comparación de costos por marca (entrega 2) se apoya en esto.

**Independent Test**: agregar a «Leche entera» dos presentaciones llamadas «Caja» con marcas
distintas: las dos se guardan; en el selector de presentación de una compra aparecen «Caja · Marca A»
y «Caja · Marca B».

**Acceptance Scenarios**:

1. **Given** una presentación sin marca, **When** se le escribe una marca y se guarda, **Then** se ve
   junto al nombre en la lista de presentaciones y en el selector de la compra.
2. **Given** dos presentaciones del mismo insumo con el mismo nombre, **When** tienen marca distinta,
   **Then** las dos pueden estar activas; **When** tienen la misma marca (o ninguna), **Then** la
   segunda se rechaza como hoy.
3. **Given** una presentación con marca, **When** se borra la marca, **Then** queda sin marca, como
   antes de esta entrega.

---

### User Story 5 - Aviso de posible duplicado al crear un insumo (Priority: P3)

Al guardar un insumo nuevo, si ya hay uno con un nombre parecido («Leche entera Lala 1L» contra
«Leche entera», o «Lehce entera»), aparece «¿Es alguno de estos?» con los parecidos, «Abrir» en cada
uno y «Crear de todos modos». Sin parecidos, guardar crea como hoy. El aviso no impide crear: hay
insumos parecidos que sí son distintos. Va al guardar y no mientras se escribe porque el teclado de
la tableta ocupa casi la mitad del alto (maqueta elegida, opción B).

**Why this priority**: evita que el problema vuelva a crecer, pero no arregla lo que ya existe.

**Independent Test**: con «Leche entera» activo, guardar «leche entera lala» en la hoja de alta
abre «¿Es alguno de estos?» con «Leche entera»; guardar «Azúcar» crea sin preguntar; «Crear de todos
modos» crea «leche entera lala».

**Acceptance Scenarios**:

1. **Given** un insumo activo cuyo nombre contiene el que se guarda, o al revés, **When** se toca
   «Guardar», **Then** aparece el aviso con el existente y no se crea nada todavía.
2. **Given** un nombre con una letra cambiada o dos letras invertidas respecto de uno activo, **When**
   se toca «Guardar», **Then** aparece el aviso.
3. **Given** el aviso, **When** se toca el insumo sugerido, **Then** se abre su ficha y no se crea
   nada.
4. **Given** el aviso, **When** se guarda de todos modos, **Then** el insumo se crea.
5. **Given** un nombre idéntico sin mayúsculas, acentos ni espacios, **When** se intenta guardar,
   **Then** se rechaza como hoy.

### User Story 6 - Ver los posibles duplicados agrupados (Priority: P2)

En Almacén › Artículos, «Posibles duplicados» muestra los grupos de insumos activos con nombre
parecido («Leche entera · Leche entera Marca B», «Vaso 16 oz · Vaso 16 oz (caja)»). Cada grupo se
revisa: se elige cuál se queda y se llega a la misma vista previa de la unión. Un grupo que no es lo
mismo se marca «No son lo mismo» y deja de proponerse.

**Why this priority**: con más de 15 grupos, encontrarlos uno por uno desde cada ficha es lo que
hace que la limpieza no se termine. Decisión del dueño al elegir las maquetas (A + C).

**Independent Test**: con «Leche entera» y «Leche entera Marca B» activos, la pantalla muestra ese
grupo; «Revisar» → elegir «Leche entera» → vista previa igual a la de la historia 1; tras unir, el
grupo desaparece. Marcar «No son lo mismo» en «Vaso 12 oz · Vaso 16 oz» lo quita y no vuelve.

**Acceptance Scenarios**:

1. **Given** insumos activos con nombres parecidos (la regla de FR-017), **When** se abre «Posibles
   duplicados», **Then** aparecen agrupados (si A se parece a B y B a C, es un grupo de tres).
2. **Given** un grupo, **When** se elige cuál se queda, **Then** se abre la vista previa de unir los
   demás en ése, con la posibilidad de quitar alguno antes de confirmar.
3. **Given** un grupo marcado «No son lo mismo», **When** se vuelve a abrir la pantalla, **Then** ese
   par ya no se propone; si el grupo tenía tres y solo un par se descartó, los otros siguen.
4. **Given** quien no tiene permiso de configurar el almacén, **Then** no ve la pantalla.

---

### Edge Cases

- **Venta o compra en curso sobre el origen**: si en el mismo instante una venta, una compra, una
  merma o un ajuste está moviendo la existencia del origen o del destino, la unión responde «vuelve
  a intentar» sin esperar; nunca traspasa una existencia vieja. Una venta que llega después de la
  unión descuenta del destino.
- **Conteo abierto**: si un conteo abierto ya capturó el origen o el destino, la unión se niega hasta
  cerrarlo o descartarlo (su cifra quedaría hablando de un insumo que ya no existe).
- **Compras por recibir y mermas pendientes del origen**: la unión se niega diciendo cuántas hay; al
  recibirse después entrarían a un insumo archivado.
- **Existencia negativa del origen**: se traspasa tal cual; el destino queda con la suma, aunque sea
  menor que antes. La vista previa avisa si el destino queda en negativo.
- **Varias sucursales**: el traspaso es sucursal por sucursal; lo de una sucursal nunca pasa a otra.
- **Insumo preparado**: uno con receta propia (preparado) no puede ser origen ni destino en esta
  entrega; las recetas de preparados que **usan** el origen sí se mueven.
- **El mismo insumo como origen y destino**, o un origen que ya está archivado o ya fue unido: se
  rechaza.
- **Un origen ya archivado por otra razón**: no se ofrece; se reactiva antes si se quiere unir.
- **Cancelar un pedido vendido antes de la unión**: lo que se repone entra al destino, no al origen
  archivado; la mercancía vuelve al anaquel que ahora es uno solo.
- **Cualquier movimiento que llegue después a un origen unido** (una pantalla abierta desde antes, la
  versión anterior del sistema durante una subida) entra al destino, convertido si hubo equivalencia.
  Lo que no es un movimiento (una receta, una compra o una presentación nueva con el origen) se
  rechaza diciendo a qué insumo se unió.
- **Cambiar la unidad de un destino con insumos unidos**: no se permite en esta entrega; el historial
  de sus orígenes dejaría de leerse en la unidad correcta.
- **Costo**: el destino queda con el costo de la compra más reciente entre destino y orígenes (la
  regla de hoy, «último precio»); si ninguno tiene compras, conserva el suyo, o el de un origen si el
  suyo es cero. Los productos y extras que lo llevan se recostean. La vista previa muestra el costo
  antes y después.
- **Volver a activar un origen unido**: no se ofrece; su ficha dice a qué insumo se unió y lleva a
  él. Deshacer una unión no está en esta entrega.
- **Mínimo de existencia**: el destino conserva el suyo; si no tenía, toma el mayor de los orígenes
  (convertido).
- **Algo cambió entre la vista previa y la confirmación** (otra venta, otra compra, otra persona
  editó una receta): la confirmación se niega y la pantalla vuelve a mostrar la vista previa con lo de
  ahora.
- **Nombre del origen**: el archivado conserva su nombre; sigue ocupándolo, así que no se puede dar de
  alta otro igual (como cualquier archivado).

## Requirements *(mandatory)*

### Functional Requirements

**Unir insumos**

- **FR-001**: Quien tiene permiso de configurar el almacén DEBE poder elegir, desde la ficha de un
  insumo activo, uno o varios insumos activos de la misma empresa para unirlos en él. Sin ese permiso
  la acción no se ve y el servidor la rechaza.
- **FR-002**: Antes de confirmar, el sistema DEBE mostrar una vista previa calculada por el servidor
  con: existencia por sucursal del destino y de cada origen, antes y después; cada receta (producto,
  extra o preparado) que cambia de insumo, con su cantidad antes y después; cada presentación que
  llega al destino, se archiva o choca; los artículos de proveedor que se ligan al destino; y el costo
  del destino antes y después.
- **FR-003**: Al confirmar, el sistema DEBE, en una sola operación que se aplica completa o no se
  aplica: traspasar la existencia de cada origen al destino por sucursal; cambiar al destino toda
  receta que llevaba un origen (juntando en un renglón la cantidad si la receta ya llevaba el
  destino); pasar al destino las presentaciones activas, los formatos de compra y los artículos de proveedor
  del origen; archivar cada origen ligado a la unión; recostear lo que cambió; y registrar el evento
  en la bitácora.
- **FR-004**: Después de unir, ninguna receta (de producto activo o inactivo, extra o preparado) DEBE
  apuntar a un insumo de origen.
- **FR-005**: El traspaso de existencia DEBE quedar como movimientos (salida del origen, entrada al
  destino) ligados a la unión, por sucursal; la historia previa del origen (ventas, compras, mermas,
  conteos) no se modifica.
- **FR-006**: El historial de costos y la bitácora del destino DEBEN incluir los de sus orígenes,
  cada renglón identificado con el insumo del que viene; los costos de un origen medido distinto se
  muestran convertidos a la unidad del destino.
- **FR-007**: Si origen y destino tienen unidad base distinta, el sistema DEBE exigir una equivalencia
  explícita (cuánto del destino vale una unidad del origen) y aplicarla a existencias, recetas,
  presentaciones, formatos de compra y artículos de proveedor; sin ella, la vista previa no deja
  confirmar. Con la misma unidad base no se pide.
- **FR-008**: La unión DEBE negarse, con un mensaje que diga qué falta resolver, si: algún origen o el
  destino está en un conteo abierto; un origen tiene compras por recibir o mermas pendientes; un
  origen o el destino es un insumo preparado; un origen está archivado o ya fue unido; el destino
  está entre los orígenes; o alguna cantidad convertida queda en cero o no cabe.
- **FR-009**: Una venta, compra, merma o ajuste en curso sobre un origen o el destino NO DEBE perder ni
  duplicar existencia: la unión espera a que no haya nada en curso o responde «vuelve a intentar».
- **FR-010**: Si algo de lo que la vista previa mostró cambió antes de confirmar, la confirmación DEBE
  negarse y la pantalla DEBE volver a pedir la vista previa.
- **FR-011**: Un insumo unido DEBE verse archivado como «unido a <destino>» y NO DEBE poder
  reactivarse en esta entrega.
- **FR-020**: Todo movimiento de existencia que llegue a un insumo ya unido DEBE aplicarse al destino
  (convertido con la equivalencia, siguiendo la cadena si el destino también se unió después), y todo
  registro nuevo que no sea un movimiento y apunte a un insumo unido DEBE rechazarse nombrando el
  destino.
- **FR-012**: El costo del destino DEBE quedar con el de la compra más reciente entre destino y
  orígenes (convertido si hubo equivalencia); sin compras, el del destino, o el de un origen si el del
  destino es cero.

**Marca en la presentación**

- **FR-013**: Cada presentación DEBE admitir una marca o proveedor opcional, de hasta 40 caracteres.
- **FR-014**: La marca DEBE verse junto al nombre de la presentación donde se lista y donde se elige
  al capturar una compra.
- **FR-015**: Dos presentaciones activas del mismo insumo PUEDEN llamarse igual si su marca es
  distinta; con el mismo nombre y la misma marca (o ambas sin marca) se rechaza, sin distinguir
  mayúsculas, acentos ni espacios.
- **FR-016**: Al unir, una presentación de origen sin marca DEBE llegar al destino con el nombre del
  insumo de origen como marca; una con marca la conserva. Si choca con una del destino con mismo
  nombre, marca y contenido, la del origen se archiva; si choca con contenido distinto, la vista
  previa lo señala y no deja confirmar.

**Aviso de posible duplicado**

- **FR-017**: Al guardar en la hoja de alta de un insumo, el sistema DEBE mostrar, antes de crear,
  hasta 5 insumos activos de la empresa con nombre parecido: uno contiene al otro, o difieren en una o dos
  letras, sin distinguir mayúsculas, acentos, signos ni espacios.
- **FR-018**: El aviso NO DEBE impedir crear el insumo, y DEBE permitir abrir el existente en lugar de
  crear.
- **FR-019**: El rechazo de un nombre idéntico (sin mayúsculas, acentos ni espacios) DEBE seguir como
  hoy.

**Posibles duplicados**

- **FR-021**: Quien configura el almacén DEBE ver en Almacén › Artículos los grupos de insumos activos
  cuyos nombres se parecen (la regla de FR-017, agrupando por transitividad), sin los pares marcados
  «No son lo mismo».
- **FR-022**: Desde un grupo DEBE poder elegirse el insumo que se queda y llegar a la vista previa de
  unir los demás (FR-002), pudiendo quitar alguno antes de confirmar.
- **FR-023**: «No son lo mismo» DEBE guardarse por par de insumos y por empresa, con quién y cuándo, y
  ese par no DEBE volver a proponerse; no impide unirlos a mano desde la ficha. Durante unos segundos
  después de marcarlo DEBE poder deshacerse.

### Key Entities

- **Unión de insumos**: un hecho registrado: qué insumos de origen se unieron en qué destino, la
  equivalencia usada (1 si se miden igual), quién y cuándo. Liga los movimientos del traspaso y es lo
  que permite mostrar la historia del origen desde el destino.
- **Insumo** (existente): gana el estado «unido a otro», que es una forma de archivado.
- **Presentación** (existente): gana la marca opcional.
- **Par descartado**: dos insumos que alguien marcó «No son lo mismo», con quién y cuándo.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Unir dos insumos, de abrir la ficha a confirmar, toma menos de 1 minuto y no más de 6
  toques cuando se miden igual.
- **SC-002**: Después de cualquier unión, 0 recetas apuntan a un insumo unido.
- **SC-003**: La suma de existencias del destino después de unir es igual, por sucursal, a la suma de
  destino y orígenes antes (convertida si hubo equivalencia), sin diferencias de redondeo mayores a
  la precisión de la existencia.
- **SC-004**: Con ventas simultáneas sobre el origen durante la unión, la existencia final cuadra con
  la suma de movimientos en el 100% de las corridas de la prueba de concurrencia.
- **SC-005**: Una persona que no conoce la pantalla une dos marcas de leche con datos reales a
  1024×600 sin ayuda y explica qué va a pasar antes de confirmar.
- **SC-006**: Al dar de alta un insumo cuyo nombre es una variante de uno existente, el aviso aparece
  antes de guardar en los ejemplos del issue (marca agregada al nombre, una letra cambiada).

## Assumptions

- El permiso es el de configurar el almacén (`inventory.configure`), el mismo que archivar o cambiar
  la unidad de un insumo.
- Los productos con existencia propia (no insumos) no se unen en esta entrega.
- Los conteos cerrados, las ventas, las compras recibidas y las mermas ya registradas del origen se
  quedan en el origen; el destino las muestra en su historial (FR-006), no las reescribe.
- Deshacer una unión no está en esta entrega; lo que pasó queda completo en la bitácora y en los
  movimientos del traspaso.
- El aviso de duplicado vive en la hoja de alta de Almacén › Artículos y sale al guardar. El alta rápida desde un gasto
  o una receta ya parte de un buscador que muestra los existentes mientras se escribe.
- Fuera de esta entrega, al issue nuevo: regla de costo con varias marcas como ajuste de la empresa
  (último precio o promedio ponderado) y comparar marcas en el historial de costos. Las compras ya
  guardan con qué presentación se compró, y desde esta entrega también la marca que tenía en ese
  momento, así que editar la marca después no reescribe de qué marca fue cada compra.
- Fuera de alcance: sustitutos entre insumos distintos y unir automáticamente.
