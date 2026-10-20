# Feature Specification: Gestión de artículos del almacén

**Feature Branch**: `042-gestion-de-insumos`

**Created**: 2026-10-19

**Status**: Aprobada por el dueño (2026-10-19)

**Input**: Issue #113. Quien administra el almacén no tiene un camino directo para crear, editar,
archivar ni entender los empaques de sus artículos. Relacionados, sin pisarlos: #100 (buscador en
Artículos, que esta spec absorbe), #111 (gastos, cajas y renglones excluidos), #112 (equivalencias
de compra por marca o código de barras) y #65 (varios almacenes por sucursal).

## Contexto

Verificado en el código el 2026-10-19:

- Almacén › Artículos solo muestra la tabla. No tiene botón de alta, buscador ni filtros. Un
  artículo solo nace dentro de otra tarea: un gasto o una receta.
- En el gasto («Buscar o crear») y en la receta («Agregar insumo»), el artículo **se crea al tocar
  la unidad**, antes de guardar. Si se cancela el gasto o la receta, el artículo queda creado.
  «Crear una nueva» de la receta sí espera a «Guardar», pero si falla el guardado de la receta, el
  artículo también queda creado.
- No hay forma de cambiar el nombre, la categoría, el mínimo, el costo ni la unidad base. Tampoco
  se puede archivar: la columna que lo marca existe, pero nada la escribe.
- El costo solo cambia al recibir una compra o al recalcular un preparado. No hay captura a mano ni
  una pantalla de historial. Cada entrada de compra sí guarda su costo unitario en el movimiento.
- «Los que ya usas» sugiere el **nombre** de cualquier empaque de la empresa, sin fijarse en la
  unidad del artículo. Por eso «Bolsa de 15 kg» aparece en un artículo que se mide en litros.
- En el conteo, el teclado ofrece solo el primer empaque del artículo, en orden alfabético; los
  demás no aparecen. La unidad suelta se ve pequeña y no hay aviso al capturar una cifra muy
  distinta de lo esperado.
- Crear un artículo pide rol de administrador o gerente, no un permiso. Crear, editar o archivar un
  artículo o un empaque no deja ningún rastro en la bitácora.

Lo que ya existe y se reutiliza, sin duplicarlo: los empaques (`ingredient_presentations`, con su
cantidad en la unidad base, su marca de aproximado y su estado activo), los lugares para contar
(spec 040), el estado activo del artículo, el costo de cada compra en sus movimientos y la regla de
nombres repetidos de la spec 037.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Encontrar un artículo en la lista (Priority: P1)

Quien administra el almacén escribe parte del nombre o filtra la lista: activos o archivados, por
lugar, sin costo y con existencia negativa.

**Why this priority**: con cientos de artículos, sin buscador no se llega a ninguna de las otras
historias. Es lo que pedía #100.

**Independent Test**: con 300 artículos, escribir «hie» deja solo los que contienen «hie», sin
distinguir mayúsculas ni acentos.

**Acceptance Scenarios**:

1. **Given** la lista de artículos, **When** se escribe «hie», **Then** solo quedan los que tienen
   «hie» en el nombre o en el nombre de alguno de sus empaques.
2. **Given** el filtro «Sin costo», **Then** solo quedan los artículos activos con costo cero o sin
   costo.
3. **Given** el filtro «Archivados», **Then** se ven solo los archivados, y ningún otro filtro los
   muestra.
4. **Given** el filtro de un lugar, **Then** solo quedan los artículos de ese lugar.

---

### User Story 2 - Crear un artículo sin abrir un gasto ni una receta (Priority: P1)

Desde Almacén › Artículos, «Nuevo artículo» abre una hoja con nombre y en qué se mide (peso,
líquido, longitud o piezas). Son opcionales el lugar, el mínimo, el costo inicial y los empaques.
Nada se guarda hasta tocar «Guardar».

**Why this priority**: es la observación principal del issue y el arreglo de los artículos que
quedan creados sin querer.

**Independent Test**: crear «Leche entera» en líquido desde la lista; aparece en la lista sin que
exista un gasto. Abrir la hoja y cancelarla no crea nada.

**Acceptance Scenarios**:

1. **Given** la lista, **When** se toca «Nuevo artículo», se captura nombre y unidad y se guarda,
   **Then** el artículo existe y aparece en la lista.
2. **Given** la hoja abierta con datos, **When** se cancela, **Then** no se crea ningún artículo.
3. **Given** un gasto en captura, **When** se crea un artículo desde «Buscar o crear» y después se
   cancela el gasto, **Then** el artículo no existe.
4. **Given** una receta en captura, **When** se agrega un insumo nuevo y después se cancela la
   receta, o su guardado falla, **Then** el artículo no existe.
5. **Given** que ya existe «Leche entera», **When** se intenta crear «leche  entera», **Then** no
   se crea otro y se ofrece el que existe.

---

### User Story 3 - Ver y editar la ficha (Priority: P1)

Tocar un artículo abre su ficha: nombre, categoría, lugar, mínimo, costo, empaques y si se compra
hecho o se prepara. Todo se edita ahí.

**Why this priority**: hoy un nombre mal escrito o un mínimo equivocado no tiene arreglo.

**Independent Test**: cambiar el nombre y el mínimo de un artículo; la lista, el conteo y la receta
muestran el nombre nuevo.

**Acceptance Scenarios**:

1. **Given** la ficha de un artículo, **When** se cambia su nombre, **Then** las recetas, el conteo
   y los reportes muestran el nombre nuevo, y las ventas pasadas no cambian.
2. **Given** la ficha, **When** se cambia el nombre al de otro artículo que ya existe, **Then** no
   se guarda y se dice cuál.
3. **Given** un artículo, **When** se corrige su costo a mano, **Then** el costo nuevo cuenta desde
   ese momento y queda en su historial como «corrección».
4. **Given** un artículo con compras, **When** se abre su historial de costos, **Then** se ve cada
   compra con su fecha, su costo por unidad y su proveedor, y cada corrección con quién la hizo.

---

### User Story 4 - Archivar y reactivar (Priority: P2)

Archivar quita el artículo de la captura de gastos, recetas y conteo, sin borrar su historia.

**Why this priority**: libera la lista de artículos que ya no se usan. Depende de la ficha (US3).

**Independent Test**: archivar un artículo sin existencias ni recetas; deja de ofrecerse en el gasto
y sigue apareciendo en los movimientos pasados.

**Acceptance Scenarios**:

1. **Given** un artículo con existencia distinta de cero o usado en recetas activas, **When** se
   toca «Archivar», **Then** se ve cuánto queda y qué recetas lo usan antes de confirmar.
2. **Given** un artículo usado en una receta activa, **When** se confirma, **Then** no se archiva
   hasta quitarlo de esas recetas.
3. **Given** un artículo archivado, **When** se toca «Reactivar», **Then** vuelve a ofrecerse.
4. **Given** un conteo abierto que ya capturó el artículo, **When** se intenta archivar, **Then**
   no se archiva hasta cerrar o descartar el conteo.

---

### User Story 5 - Cambiar la unidad base (Priority: P2)

Un artículo dado de alta en piezas que en realidad se pesa se cambia a peso.

**Why this priority**: hoy obliga a duplicar el artículo. Menos frecuente que editar.

**Independent Test**: un artículo sin movimientos pasa de piezas a gramos y conserva sus recetas.

**Acceptance Scenarios**:

1. **Given** un artículo sin compras, ventas ni mermas (los ajustes de conteo no cuentan), **When**
   se cambia de piezas a peso con la equivalencia «1 pieza = 250 g», **Then** conserva su nombre, su
   lugar y sus recetas; su existencia, sus ajustes de conteo, sus recetas y sus empaques se
   convierten con esa equivalencia, y cada cantidad se puede revisar antes de confirmar.
2. **Given** un artículo con movimientos, **When** se intenta cambiar su unidad, **Then** se ofrece
   «Crear uno nuevo en la otra unidad y archivar este», con la existencia actual convertida al
   nuevo según una equivalencia que se captura.
3. **Given** ese reemplazo, **When** se confirma, **Then** las recetas activas pasan al nuevo con su
   cantidad convertida por la misma equivalencia (revisable una por una antes de confirmar), y la
   historia del viejo queda intacta.
4. **Given** un artículo cuyo único movimiento es el ajuste de su primer conteo, **When** se cambia
   su unidad, **Then** el cambio es directo, no un reemplazo.

---

### User Story 6 - Empaques que se entienden (Priority: P2)

La sección de empaques dice qué son con un ejemplo («¿Cómo lo compras? Por ejemplo: bolsa de
5 kg»), muestra siempre su equivalencia en la unidad del artículo y sugiere solo empaques de
artículos que se miden igual.

**Why this priority**: una persona nueva no entendió los empaques; la sugerencia equivocada invita
al error.

**Independent Test**: en un artículo en litros, las sugerencias no incluyen «Bolsa de 15 kg».

**Acceptance Scenarios**:

1. **Given** un artículo en litros, **When** se agrega un empaque, **Then** las sugerencias son
   solo de artículos en litros, cada una con su cantidad («Garrafa · 4 L»).
2. **Given** un empaque, **Then** se ve como «Bolsa · 5 kg» y no solo su nombre.
3. **Given** un empaque, **When** se edita su cantidad o se retira, **Then** las compras pasadas no
   cambian.

---

### User Story 7 - Contar sin equivocarse de unidad (Priority: P2)

En el conteo, la unidad se ve grande junto al número, se elige la unidad (g o kg, ml o L) y el
empaque cuando hay varios, y se avisa antes de guardar una cifra fuera de lo razonable.

**Why this priority**: 740 escrito en un artículo en kg guarda 740 kg y descuadra el inventario.

**Independent Test**: en un artículo en gramos con existencia 0 y un empaque de 15 kg, escribir 740
en kg pide confirmar; escribir 740 en g, no.

**Acceptance Scenarios**:

1. **Given** un artículo en gramos, **When** se abre el teclado, **Then** la unidad elegida es «g»
   y se puede cambiar a «kg» con un toque; al escribir 740 se ve «740 g» con la unidad del mismo
   tamaño que el número.
2. **Given** el hielo con bolsas de 5 kg y de 15 kg, **When** se captura, **Then** se puede elegir
   cualquiera de las dos, y buscar «bolsa» también lo encuentra.
3. **Given** un artículo con existencia 0 o negativa y un empaque de 15 kg, **When** se guarda
   740 kg, **Then** se pide confirmar mostrando «740 kg» en grande, porque pasa de 10 veces el
   empaque más grande.
4. **Given** un artículo con existencia positiva, **When** se guarda una cifra más de 10 veces mayor
   o menor que lo esperado, **Then** se pide confirmar. Ningún aviso revela la cifra de referencia:
   el conteo sigue siendo a ciegas.
5. **Given** un artículo con existencia positiva, **When** se cuenta 0, **Then** se guarda sin aviso.
6. **Given** un artículo en gramos con empaque de 1 kg, **When** se guarda 5 g, **Then** se pide
   confirmar mostrando «5 g» en grande.

### Edge Cases

- **Nombre repetido**, sin distinguir mayúsculas, acentos ni espacios: no se crea ni se renombra
  (regla de la spec 037), en las cuatro entradas: la lista, el gasto, la receta y el reemplazo.
- **Dos personas editan la misma ficha**: la segunda recibe «cambió mientras la veías» y vuelve a
  cargar; no pisa.
- **Archivar con una compra por recibir** de ese artículo: se avisa; lo pendiente se puede recibir
  después y lo pone de vuelta en existencia, sin reactivarlo.
- **Cambiar la unidad mientras se vende**: el cambio sin movimientos se niega si aparece una venta
  en ese momento (409), en vez de convertir a medias.
- **Empaque con la misma cantidad que la unidad** (bolsa de 1 kg): se permite.
- **Costo inicial o corregido negativo o absurdo**: 400, como cualquier dinero.
- **Nombre repetido de un artículo archivado**: el aviso ofrece «Reactivar» ese artículo en vez de
  crear otro.
- **Existencia inicial con dos sucursales y sin selector**: se niega con el aviso de sucursal
  ambigua, igual que un conteo; el artículo no se crea a medias.
- **Productos de reventa** (una lata, una botella): son artículos como cualquier otro, con su
  existencia, sus empaques y su costo; el producto que se vende los resta por su receta de 1 pieza.
- **Un preparado**: su costo lo calcula la receta y no se captura a mano; la ficha lo dice.
- **Artículo de otra empresa**: no se ve ni se edita.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: La lista de artículos MUST tener un buscador por nombre del artículo y de sus
  empaques, sin distinguir mayúsculas ni acentos, y filtros de activos o archivados, lugar, sin
  costo y existencia negativa.
- **FR-002**: La lista MUST tener «Nuevo artículo», que abre una hoja con nombre y en qué se mide
  (obligatorios) y lugar, mínimo, costo inicial, existencia inicial y empaques (opcionales). La
  existencia inicial entra como un ajuste de conteo. Todo se guarda en una sola operación al tocar
  «Guardar». Si el nombre ya existe en un artículo archivado, se ofrece «Reactivar».
- **FR-003**: Ninguna captura MUST dejar un artículo creado si se cancela o si su guardado falla. El
  artículo nuevo del gasto y de la receta se crea en la misma operación que guarda el gasto o la
  receta.
- **FR-004**: La ficha MUST permitir editar nombre, categoría, lugar, mínimo y empaques, con la
  regla de nombres repetidos de la spec 037.
- **FR-005**: La ficha MUST permitir corregir el costo de un artículo que se compra hecho. Un
  preparado no lo permite. El costo corregido se usa desde ese momento y no recalcula lo vendido.
- **FR-006**: La ficha MUST mostrar el historial de costos por compra: arriba, una línea de
  tendencia del costo por unidad; debajo, las últimas 20 compras recibidas y correcciones a mano,
  las más recientes primero, con «Ver más» para las anteriores. Cada compra lleva fecha, proveedor,
  empaque en que se compró y costo por unidad («$30 por Bolsa de 5 kg · $0.006/g»); cada corrección,
  quién la hizo. La gráfica y la lista MUST caber y leerse a 1024×600 sin desplazar la ficha de lado.
- **FR-007**: Archivar MUST mostrar antes la existencia y las recetas activas que usan el artículo.
  Se niega mientras una receta activa lo use o un conteo abierto lo tenga capturado. Reactivar lo
  devuelve tal como estaba.
- **FR-008**: Un artículo archivado MUST dejar de ofrecerse en el gasto, la receta y el conteo, y
  MUST seguir visible en sus movimientos, compras y reportes pasados.
- **FR-009**: Para decidir el camino, solo cuentan como movimientos las compras, ventas, mermas y
  cancelaciones; los ajustes de conteo no. Sin esos movimientos, la unidad base MUST poder
  cambiarse directo con una equivalencia capturada («1 pieza = 250 g»; entre kg y g no se pide):
  la existencia, los ajustes de conteo, las cantidades de las recetas y de los empaques se
  convierten con ella, y la bitácora guarda cada valor y unidad originales para poder revertir el
  cambio. Con movimientos, MUST ofrecerse el reemplazo: un artículo nuevo en la otra
  unidad, la existencia convertida por la equivalencia (una salida en el viejo y una entrada en el
  nuevo, ligadas), las recetas activas pasadas al nuevo con su cantidad convertida por la misma
  equivalencia, y el viejo archivado. En los dos caminos, cada cantidad convertida se muestra y se
  puede corregir antes de confirmar, y todo es una sola operación.
- **FR-010**: Los empaques MUST mostrarse siempre con su equivalencia en la unidad del artículo, y
  la sección MUST explicar qué son con un ejemplo.
- **FR-011**: Las sugerencias de empaque MUST ser solo de artículos con el mismo tipo de unidad, y
  cada una MUST llevar su cantidad.
- **FR-012**: El conteo MUST ofrecer todos los empaques activos y no aproximados del artículo, un
  selector visible de unidad suelta (g o kg, ml o L, cm o m) con la unidad base elegida al abrir.
  Un artículo en piezas con un empaque aproximado por peso («Kilo ≈ 400 pz») ofrece también g y kg
  para lo suelto; lo pesado se convierte a piezas con esa equivalencia, el total se muestra con «≈»
  y se guarda en piezas, sin tocar sus recetas. Sin ese empaque, en piezas no hay selector. MUST
  mostrar la unidad del mismo tamaño que el número y buscar también por nombre de empaque. Antes de guardar MUST pedir confirmación, mostrando la cifra con su unidad en
  grande, cuando la cifra pase la referencia del artículo, en este orden:
  1. Existencia esperada positiva: más de 10 veces mayor o menor que ella.
  2. Si no: el último conteo cerrado del artículo con cifra positiva, con la misma regla.
  3. Si no: más de 10 veces su empaque más grande.
  4. Si no tiene empaques: un tope por unidad (50 kg, 50 L, 50 m o 5,000 piezas).
  Contar 0 nunca dispara el aviso de «10 veces menor». Además MUST pedir confirmación cuando la
  cifra, mayor que 0, sea menor a 1/100 de su empaque más pequeño (5 g en un artículo con bolsa de
  1 kg: probablemente se quiso escribir kg). Ningún aviso muestra la cifra de referencia: el conteo
  sigue siendo a ciegas.
- **FR-013**: Crear, editar, archivar, reactivar, corregir el costo y cambiar la unidad MUST pedir
  permiso en el servidor, nunca por nombre de rol: ver la lista y la ficha, `inventory.view`;
  crear desde un gasto o una receta, el permiso de esa captura; crear y editar desde la lista,
  archivar, reactivar, corregir el costo, cambiar la unidad y editar empaques,
  `inventory.configure`. Los roles sugeridos MUST quedar así para que nadie pierda acceso:
  administrador y gerente reciben `inventory.configure` e `inventory.view`, que son los dos roles
  que hoy ven el almacén; cajero y mesero no reciben ninguno. Los roles viven en un mapa
  del código, no en la base: no hay migración de datos.
- **FR-014**: Cada una de esas acciones MUST escribir su evento en la bitácora dentro de su
  transacción: `ingredient.created`, `ingredient.updated` (con lo que cambió),
  `ingredient.archived`, `ingredient.reactivated`, `ingredient.cost_corrected`,
  `ingredient.unit_changed` (con la equivalencia y, por cada ajuste de conteo, receta y empaque
  convertido, su valor y unidad originales), `ingredient.replaced`, `presentation.created`, `presentation.updated`
  y `presentation.retired`.
- **FR-015**: Los textos MUST ser de quien opera («Artículo», «Empaque», «En qué se mide»,
  «Archivar»), sin nombres internos. Todo control táctil mide 44 px o más, y ningún selector es
  nativo.

### Key Entities

- **Artículo** (ya existe): nombre, categoría, unidad base, lugar, mínimo, costo actual, si está
  activo. Gana la corrección de costo y el reemplazo por otro artículo.
- **Empaque** (ya existe): nombre, cantidad en la unidad base, aproximado, activo. Sin cambios de
  forma.
- **Corrección de costo** (nueva): artículo, costo, quién, cuándo. Junto con los movimientos de
  compra forma el historial.
- **Reemplazo** (nuevo): artículo viejo, artículo nuevo, equivalencia y los dos movimientos
  ligados.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Encontrar un artículo entre 400 toma a lo más 3 toques más lo que se escribe.
- **SC-002**: Ninguna captura cancelada o fallida deja un artículo creado (lo cubre una prueba por
  camino: lista, gasto y las dos de la receta).
- **SC-003**: Las sugerencias de empaque de un artículo nunca incluyen uno de otro tipo de unidad.
- **SC-004**: Una persona nueva crea un artículo con un empaque y archiva otro sin ayuda, en la
  prueba con usuario sin contexto.
- **SC-005**: Cada acción de FR-014 deja exactamente un evento en la bitácora.

## Assumptions

- **Historial de costos** (decisión del dueño, 2026-10-19): por compra, con una línea de tendencia
  arriba y las últimas 20 con «Ver más» (FR-006). Sale de los movimientos de compra y de las
  correcciones; no agrega nada a lo pasado.
- **Permisos** (decisión del dueño, 2026-10-19): se reutiliza `inventory.configure`, que ya protege
  empaques y lugares, para todo lo que escribe, y se agrega `inventory.view` para leer. Cada
  corrección de costo, archivo, reactivación y cambio de unidad queda en la bitácora con quién y
  cuándo (FR-014).
- **Cambio de unidad** (decisión del dueño, 2026-10-19): sin movimientos, directo en el mismo
  artículo; con movimientos, reemplazo guiado y nunca conversión en el mismo artículo, para no
  reescribir movimientos pasados (regla del 2026-10-13: el pasado no se recalcula).
- Un artículo no se borra, se archiva.
- El umbral del aviso del conteo (10 veces) es constante, no configurable.
- El lugar sigue siendo solo el orden del conteo (spec 040), no un almacén.
- **Fuera de alcance**: marcas, proveedores y códigos de barras por artículo (#112); varios almacenes
  y traspasos entre ellos (#65); renglones excluidos y cajas (#111); caducidades.
