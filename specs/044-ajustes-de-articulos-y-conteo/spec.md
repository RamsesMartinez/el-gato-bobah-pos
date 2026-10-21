# Feature Specification: Ajustes de artículos y conteo tras la prueba en tableta

**Feature Branch**: `044-ajustes-de-articulos-y-conteo`

**Created**: 2026-10-21

**Status**: Aprobada por el dueño (2026-10-21), con el cambio de alcance de #126; US7 y US8 agregadas el mismo día

**Input**: Issues #124 y #126 (este cierra con el mismo PR, por decisión del dueño). Hallazgos de la prueba guiada del dueño en el ambiente de pruebas con la 042 y la
043 desplegadas (2026-10-21), con sus decisiones. Seguimiento de las dos specs.

## Contexto

Lo que el dueño encontró al recorrer las pantallas en el ambiente de pruebas:

1. En la ficha del artículo, «‹ Artículos» no se lee como «regresar».
2. En la lista, «· Revisar» junto a una existencia negativa es muy chico.
3. La gráfica de costos de la ficha se ve poco profesional.
4. En el conteo, una cifra que pasa el tope dice «datos inválidos» sin decir por qué.
5. El tope del conteo es 1,000,000 en la unidad base del artículo (1,000 kg en un artículo en gramos)
   y los umbrales del aviso de cifra absurda (50 kg, 50 L, 50 m, 5,000 piezas) están fijos en el código.
   El dueño decide hacerlos configurables por empresa (#126), con esos valores como default.
6. El teclado del conteo solo acepta el teclado de la pantalla, no el teclado físico.
7. En tema oscuro, la lista de Almacén › Existencias no se ve (agregado por el dueño el 2026-10-21).

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
   10,000 m por conteo»; en piezas, «Máximo 1,000,000 piezas por conteo» (con los límites por default).
   Si la empresa cambió su límite, el mensaje dice el de la empresa.
3. **Given** el aviso, **Then** se ve antes de mandar nada, y si la cifra llega al servidor por otro
   camino, el servidor responde con el mismo texto.
4. **Given** una empresa que no ha cambiado su límite, **Then** el tope es 1,000,000 en la unidad base.

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

---

### User Story 6 - Ajustar los límites del conteo de la empresa (Priority: P2)

Quien configura el almacén cambia, por clase de unidad, el máximo que se puede registrar en un conteo y
la cifra a partir de la cual el conteo pide confirmar.

**Why this priority**: el producto es para negocios de comida de distinto volumen (#126).

**Independent Test**: en Configuración del negocio › Almacén, cambiar el máximo de peso a 2,000 kg;
en el conteo, 1,500 kg se guarda y 2,500 kg dice «Máximo 2,000 kg por conteo».

**Acceptance Scenarios**:

1. **Given** Configuración del negocio › Almacén, **Then** hay una sección «Límites del conteo» con, por
   peso, líquido, longitud y piezas, el máximo por conteo y la cifra que pide confirmar, en la unidad
   grande de cada clase (kg, L, m, piezas).
2. **Given** una empresa existente o nueva, **Then** arranca con 1,000 kg, 1,000 L, 10,000 m y 1,000,000
   piezas de máximo, y 50 kg, 50 L, 50 m y 5,000 piezas para pedir confirmar.
3. **Given** un cambio, **When** se guarda, **Then** queda en la bitácora con quién, cuándo, lo de antes
   y lo nuevo.
4. **Given** un máximo menor o igual que su cifra de confirmar, un cero, un negativo o una cifra fuera de
   lo que el sistema puede guardar, **Then** no se guarda y se dice por qué.
5. **Given** quien no tiene permiso de configurar el almacén, **Then** no ve la sección ni puede cambiar
   los límites por otro camino.
6. **Given** dos empresas, **Then** cada una ve y cambia solo los suyos.
7. **Given** el aviso de cifra absurda, **Then** usa la cifra de confirmar de la empresa cuando el artículo
   no tiene otra referencia, y sigue sin revelar ninguna cifra.

### User Story 7 - Ver la lista de existencias en tema oscuro (Priority: P1)

**Why this priority**: con tema oscuro la pantalla queda inservible.

**Independent Test**: con tema oscuro, abrir Almacén › Existencias a 1024×600: cada renglón se lee.

**Acceptance Scenarios**:

1. **Given** el tema oscuro, **Then** los renglones, encabezados y cifras de Existencias se leen con contraste
   de 4.5:1 o más (texto) y 3:1 o más (bordes y controles).
2. **Given** el tema claro, **Then** se ve igual que hoy.

### User Story 8 - Repaso visual de todo el almacén (Priority: P1)

**Why this priority**: el defecto de Existencias en oscuro apareció solo al usarlo; puede haber más.

**Independent Test**: un recorrido con Playwright en local, a 1024×600, en tema claro y oscuro, por cada
pestaña y hoja del almacén, con capturas; ninguna queda ilegible, cortada ni desbordada.

**Acceptance Scenarios**:

1. **Given** cada pestaña del almacén (Existencias, Movimientos, Artículos y su ficha, Por recibir, Conteo con
   su teclado, Se echó a perder, Salud) y Configuración del negocio › Almacén, **Then** en claro y en oscuro el texto
   cumple el contraste de US7, nada se sale del ancho y los controles miden 44 px o más.
2. **Given** un defecto encontrado, **Then** se corrige con colores del tema y deja su prueba.
3. **Given** el recorrido, **Then** las capturas de claro y oscuro quedan como evidencia para el dueño.

### User Story 9 - CI más corto (Priority: P2)

**Why this priority**: el dueño quiere iterar sin esperar; hoy un PR tarda ~5 min y el merge a develop ~3
(medido el 2026-10-21: integración 286 s, imagen de la API 102 s).

**Independent Test**: un PR que toca `server/` termina su CI en ~2 min y un merge a develop construye la
imagen en ~40 s, sin perder ninguna prueba.

**Acceptance Scenarios**:

1. **Given** un PR que toca `server/`, **Then** las pruebas de integración corren repartidas en 3 máquinas,
   cada una con su Postgres, y entre las 3 corren exactamente las mismas pruebas que hoy (ninguna se
   pierde ni se repite).
2. **Given** un merge a develop, **Then** la imagen de la API reusa la compilación en vez de recompilar el
   módulo entero.
3. **Given** las pruebas de integración, **Then** queda medido cuáles son las 10 más lentas y por qué, y
   se corrige lo que sea barato (correr en paralelo lo que hoy corre en serie sin necesidad).

### Edge Cases

- **Coma decimal** del teclado físico: cuenta como punto.
- **Una tecla que no es número** (letras): se ignora.
- **Enter con «Guardar» apagado** (nada capturado): no hace nada.
- **Enter mientras se muestra el aviso de cifra absurda**: no confirma; se confirma tocando el botón.
- **Volver con un guardado en curso**: igual que hoy (043).
- **Gráfica de pie (< 800 px)**: ocupa el ancho de la columna y se lee.
- **Bajar el máximo con un conteo abierto**: lo ya capturado no se toca; la regla nueva aplica a la
  siguiente captura.
- **Una tableta con la pantalla anterior** durante una subida: el servidor aplica el límite de la empresa;
  la pantalla vieja muestra su error genérico.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: La ficha MUST tener un botón «Volver a Artículos» con flecha, con borde y de 44 px o más,
  que haga lo mismo que el «‹ Artículos» de hoy.
- **FR-002**: La lista MUST mostrar «Revisar» junto a una existencia negativa con el mismo tamaño de
  letra que la cifra, como etiqueta roja; la fila MUST seguir midiendo 44 px.
- **FR-003**: La gráfica de costos MUST rediseñarse con el dibujo propio del POS: línea suavizada, área
  con degradado, guía tenue, último punto destacado con su cifra, correcciones como punto hueco, y
  fecha y costo al tocar un punto. Sin librería de gráficas.
- **FR-004**: El conteo MUST rechazar una cifra que pase el máximo de la empresa para la clase del
  artículo con «Máximo N <unidad> por conteo», en la unidad grande (kg, L, m, piezas), antes de mandarla.
- **FR-005**: El servidor MUST responder 400 con ese mismo texto cuando la cifra contada pase el máximo.
- **FR-006**: Cada empresa MUST tener, por clase de unidad, un máximo por conteo y una cifra para pedir
  confirmar. Default para empresas existentes (relleno al migrar) y nuevas: máximo 1,000,000 en la unidad
  base (1,000 kg, 1,000 L, 10,000 m, 1,000,000 piezas) y confirmar a partir de 50 kg, 50 L, 50 m y 5,000
  piezas.
- **FR-007**: El teclado del conteo MUST aceptar el teclado físico: números, punto y coma como punto,
  Retroceso, Enter como «Guardar» y Tab como «Siguiente casilla». Otras teclas se ignoran.
- **FR-008**: Todo control tocable MUST medir 44 px o más, y ningún selector es nativo.
- **FR-009**: Los límites MUST editarse en Configuración del negocio › Almacén con `inventory.configure`,
  validados en el servidor: positivos y el máximo mayor que la cifra de confirmar. El máximo puede subir o
  bajar respecto al default; el único techo es lo que el sistema puede guardar (1,000,000,000 en unidad base).
- **FR-010**: Cada cambio MUST dejar su evento en la bitácora dentro de su transacción, con lo de antes y
  lo nuevo.
- **FR-011**: Los límites MUST estar aislados por empresa (RLS), con su prueba en los tres casos.
- **FR-012**: El aviso de cifra absurda MUST usar la cifra de confirmar de la empresa en lugar del valor
  fijo de hoy; el resto de su regla no cambia.
- **FR-013**: La lista de Existencias MUST leerse en tema claro y oscuro (contraste de US7); sus colores salen
  de los tokens del tema, no de colores fijos.
- **FR-014**: Todas las pantallas del almacén MUST pasar el repaso visual de US8 en claro y oscuro a 1024×600,
  en local.
- **FR-015**: El CI MUST repartir las pruebas de integración en 3 máquinas sin perder ni repetir ninguna,
  con una verificación en el mismo CI que falle si el total repartido no coincide con la lista completa.
- **FR-016**: La imagen de la API MUST reusar la compilación de Go entre corridas de CI.
- **FR-017**: Las 10 pruebas de integración más lentas MUST quedar medidas y anotadas en el plan, con lo que
  se corrigió y lo que no.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: En la prueba con usuario sin contexto, la persona regresa de la ficha a la lista sin dudar.
- **SC-002**: La lista sigue mostrando 6 renglones completos a 1024×600.
- **SC-003**: El dueño aprueba la gráfica en la prueba local; si no, se abre la opción de librería.
- **SC-004**: Ninguna cifra fuera del tope muestra «datos inválidos» en el conteo.
- **SC-005**: Un conteo completo se puede capturar sin tocar la pantalla, con el teclado físico.
- **SC-006**: Cambiar el máximo de una empresa cambia lo que su conteo acepta y su mensaje, sin tocar a
  las demás empresas.
- **SC-007**: El CI de un PR que toca `server/` baja de ~5 min a ~2 min, y la imagen de la API de 102 s a
  ~40 s (objetivos estimados; se miden en el PR de la 044 y en su merge).

## Assumptions

- **El tope de longitud** sale del mismo 1,000,000 en la unidad base: en centímetros son 10,000 m.
- **Tab** se toma como «Siguiente casilla» porque es lo que hace en cualquier formulario; no estaba en
  la decisión del dueño.
- **La prueba la hace el dueño en local** (decisión del 2026-10-21): no se mergea ni se sube a develop
  hasta su visto bueno; el PR queda en borrador.
- **Los límites son por clase de unidad**, no un solo número en unidad base: «1,000,000 g» no se lee en
  pantalla, y los avisos ya eran por clase.
- **El máximo sube o baja** (decisión del dueño, 2026-10-21): no queda fijo en el código; el default es solo el valor inicial.
- **Solo el conteo** usa estos límites. Compras, recetas y ajustes siguen con el tope general del sistema.
- **Fuera de alcance**: una librería de gráficas (salvo que el SVG no baste), cambiar la lógica del aviso
  del conteo (solo su cifra de referencia) y cambios de disposición en el resto de pantallas del almacén.
  Sí entra: corregir lo que el repaso visual del almacén (US8) encuentre ilegible o roto en claro u oscuro.
- **Todo se prueba en local** (regla del repo desde la constitución 1.25.0, 2026-10-21): la 044 no se valida en el ambiente de
  pruebas; las pruebas de pantalla, el repaso visual y la prueba del dueño corren contra el `make start`
  del worktree (:3044). Lo que solo se ve desplegado (el build de producción, el CSP) queda sin verificar
  hasta que llegue a develop.
