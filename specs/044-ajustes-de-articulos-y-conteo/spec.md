# Feature Specification: Ajustes de artículos y conteo tras la prueba en tableta

**Feature Branch**: `044-ajustes-de-articulos-y-conteo`

**Created**: 2026-10-21

**Status**: Decisiones del dueño tomadas (2026-10-21); pendiente de plan

**Input**: Issue #124. Hallazgos de la prueba guiada del dueño en el ambiente de pruebas con la 042 y la
043 desplegadas (2026-10-21), con sus decisiones. Seguimiento de las dos specs.

## Contexto

Lo que el dueño encontró al recorrer las pantallas en el ambiente de pruebas:

1. En la ficha del artículo, «‹ Artículos» no se lee como «regresar».
2. En la lista, «· Revisar» junto a una existencia negativa es muy chico.
3. La gráfica de costos de la ficha se ve poco profesional.
4. En el conteo, una cifra que pasa el tope dice «datos inválidos» sin decir por qué.
5. El tope por movimiento es 1,000,000 en la unidad base del artículo (1,000 kg en un artículo en
   gramos). El dueño decide mantenerlo.
6. El teclado del conteo solo acepta el teclado de la pantalla, no el teclado físico.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Regresar a la lista sin dudar (Priority: P1)

**Why this priority**: es el primer toque al salir de cada ficha.

**Independent Test**: abrir la ficha de un artículo; el botón para volver dice «Volver a Artículos» y se
reconoce como regresar sin leerlo dos veces.

**Acceptance Scenarios**:

1. **Given** la ficha abierta, **Then** arriba a la izquierda hay un botón con una flecha y el texto
   «Volver a Artículos», con borde, de 44 px o más.
2. **Given** ese botón, **When** se toca, **Then** hace lo mismo que hoy: guarda el campo con el foco,
   espera lo que se esté guardando y regresa (o pregunta «¿Descartar cambios?» si algo quedó sin
   guardar).

---

### User Story 2 - Ver que una existencia negativa hay que revisarla (Priority: P1)

**Why this priority**: el aviso existe pero no se lee.

**Independent Test**: en la lista, un artículo con existencia negativa muestra «Revisar» legible a la
primera, y la fila sigue midiendo 44 px.

**Acceptance Scenarios**:

1. **Given** un artículo con existencia negativa, **Then** junto a la cifra se ve una etiqueta «Revisar»
   del mismo tamaño de letra que la cifra, en rojo sobre fondo rojo suave.
2. **Given** la lista a 1024×600, **Then** siguen cabiendo 6 renglones completos.

---

### User Story 3 - Una gráfica de costos que se vea profesional (Priority: P2)

**Why this priority**: es lo primero que se ve en la columna derecha de la ficha.

**Independent Test**: abrir la ficha de un artículo con varias compras; la gráfica se ve cuidada y se
lee a 1024×600 de lado y de pie.

**Acceptance Scenarios**:

1. **Given** un artículo con varias compras, **Then** la gráfica muestra la línea del costo por unidad
   suavizada, con un área de degradado debajo, una guía tenue, el último punto destacado con su cifra
   y las correcciones a mano como punto hueco.
2. **Given** la gráfica, **When** se toca un punto, **Then** se ve su fecha y su costo, sin abrir otra
   hoja.
3. **Given** un artículo con una sola compra o ninguna, **Then** se comporta como hoy: con una, un solo
   punto con su cifra; con ninguna, no se dibuja.
4. **Given** la gráfica, **Then** está hecha con el dibujo propio del POS, sin librería de gráficas
   (decisión del dueño: librería solo si esto no basta).

---

### User Story 4 - Saber por qué una cifra del conteo no se guarda (Priority: P1)

**Why this priority**: «datos inválidos» no dice qué corregir.

**Independent Test**: en el conteo de un artículo en gramos, escribir 1,500 kg y tocar «Guardar»: el
aviso dice «Máximo 1,000 kg por conteo».

**Acceptance Scenarios**:

1. **Given** un artículo en peso, **When** se captura más del tope, **Then** el aviso dice «Máximo
   1,000 kg por conteo» y no se guarda.
2. **Given** un artículo en líquido, **Then** dice «Máximo 1,000 L por conteo»; en longitud, «Máximo
   10,000 m por conteo»; en piezas, «Máximo 1,000,000 piezas por conteo».
3. **Given** el aviso, **Then** se ve antes de mandar nada, y si la cifra llega al servidor por otro
   camino, el servidor responde con el mismo texto.
4. **Given** el tope, **Then** sigue siendo 1,000,000 en la unidad base (decisión del dueño).

---

### User Story 5 - Contar con el teclado físico (Priority: P2)

**Why this priority**: con un teclado conectado, obligar a tocar la pantalla cuesta tiempo.

**Independent Test**: en el teclado del conteo, escribir 2.5 con el teclado físico y oprimir Enter:
se guarda igual que con la pantalla.

**Acceptance Scenarios**:

1. **Given** el teclado del conteo abierto, **When** se oprimen números, punto o coma, **Then** se
   escriben en la casilla que tiene el foco, igual que con la pantalla.
2. **Given** lo capturado, **When** se oprime Retroceso, **Then** borra el último dígito.
3. **When** se oprime Enter, **Then** hace lo mismo que «Guardar».
4. **When** se oprime Tab, **Then** pasa a la siguiente casilla, como «Siguiente casilla».
5. **Given** el teclado de la pantalla, **Then** sigue funcionando igual.

### Edge Cases

- **Coma decimal** del teclado físico: cuenta como punto.
- **Una tecla que no es número** (letras): se ignora.
- **Enter con «Guardar» apagado** (nada capturado): no hace nada.
- **Enter mientras se muestra el aviso de cifra absurda**: no confirma; se confirma tocando el botón.
- **Volver con un guardado en curso**: igual que hoy (043).
- **Gráfica de pie (< 800 px)**: ocupa el ancho de la columna y se lee.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: La ficha MUST tener un botón «Volver a Artículos» con flecha, con borde y de 44 px o más,
  que haga lo mismo que el «‹ Artículos» de hoy.
- **FR-002**: La lista MUST mostrar «Revisar» junto a una existencia negativa con el mismo tamaño de
  letra que la cifra, como etiqueta roja; la fila MUST seguir midiendo 44 px.
- **FR-003**: La gráfica de costos MUST rediseñarse con el dibujo propio del POS: línea suavizada, área
  con degradado, guía tenue, último punto destacado con su cifra, correcciones como punto hueco, y
  fecha y costo al tocar un punto. Sin librería de gráficas.
- **FR-004**: El conteo MUST rechazar una cifra que pase el tope con «Máximo N <unidad> por conteo»,
  en la unidad grande del artículo (1,000 kg, 1,000 L, 10,000 m, 1,000,000 piezas), antes de mandarla.
- **FR-005**: El servidor MUST responder 400 con ese mismo texto cuando la cifra contada pase el tope.
- **FR-006**: El tope por movimiento MUST seguir siendo 1,000,000 en la unidad base.
- **FR-007**: El teclado del conteo MUST aceptar el teclado físico: números, punto y coma como punto,
  Retroceso, Enter como «Guardar» y Tab como «Siguiente casilla». Otras teclas se ignoran.
- **FR-008**: Todo control tocable MUST medir 44 px o más, y ningún selector es nativo.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: En la prueba con usuario sin contexto, la persona regresa de la ficha a la lista sin dudar.
- **SC-002**: La lista sigue mostrando 6 renglones completos a 1024×600.
- **SC-003**: El dueño aprueba la gráfica en la prueba local; si no, se abre la opción de librería.
- **SC-004**: Ninguna cifra fuera del tope muestra «datos inválidos» en el conteo.
- **SC-005**: Un conteo completo se puede capturar sin tocar la pantalla, con el teclado físico.

## Assumptions

- **El tope de longitud** sale del mismo 1,000,000 en la unidad base: en centímetros son 10,000 m.
- **Tab** se toma como «Siguiente casilla» porque es lo que hace en cualquier formulario; no estaba en
  la decisión del dueño.
- **La prueba la hace el dueño en local** (decisión del 2026-10-21): no se mergea ni se sube a develop
  hasta su visto bueno; el PR queda en borrador.
- **Fuera de alcance**: cambiar el tope, una librería de gráficas (salvo que el SVG no baste) y el resto
  de pantallas del almacén.
