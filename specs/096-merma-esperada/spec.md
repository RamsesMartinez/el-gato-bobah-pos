# Feature Specification: Merma esperada, conteo contra ventas y rediseño del inventario

**Feature Branch**: `096-merma-esperada`

**Created**: 2026-11-23

**Status**: Draft

**Input**: Issue #226 (cuerpo del 2026-11-04, ampliación del 2026-11-16, merma real del 2026-11-23 y
decisión del dueño del 2026-11-23: todo en un spec, dos subidas). Relacionados: #146 (caducidad por
lote) y #65 (varios almacenes, spec 094).

## Contexto

Hoy la merma esperada existe solo **por artículo** y solo se llenó al importar de FUDO: ninguna
pantalla la deja capturar, y 443 de 763 artículos activos están en 0 %. Quien quiere cubrir lo que se
pierde al preparar (derrame, sobrante al medir) redondea cantidades receta por receta. Y como el
artículo guarda un número y no una decisión, un 0 % no distingue «aquí no se pierde nada» de «nadie
lo ha capturado».

El conteo tampoco dice cuánto de la diferencia es merma: el reporte de faltante (spec 039) parte lo
perdido en registrado y sin explicar, pero no lo compara con lo que las ventas descontaron. La
evidencia del dueño: la mayor parte de la diferencia observada **no** es merma, sino consumo sin
receta o venta sin capturar. Y las pantallas de inventario y conteo tardan en cargar y piden
capacitación para un conteo.

### Entregas

| # | Entrega | Cuándo sube | Lo que el operador ve al terminar |
| --- | --- | --- | --- |
| S1 | Costos | En cuanto esté lista | % general en Ajustes, % por producto, % por artículo con «sin captura» distinto de 0 %, de dónde sale el costo de cada producto con lo que sumó la merma, y la merma real de 90 días junto al % |
| S2 | Inventario y conteo | Cuando el #65 esté completo en producción, encima de sus pantallas con selector de almacén | Reporte conteo vs ventas por almacén, opción de que la merma esperada descuente inventario, inventario y conteo rediseñados con vista sencilla y avanzada, y carga medida contra una meta |

S1 solo toca costo y márgenes; no mueve existencias. S2 depende de que el almacén exista en
conteos, mermas y movimientos (spec 094, entregas E2 a E5).

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Un porcentaje general cubre lo que se pierde al preparar (Priority: P1) — S1

Quien configura el menú abre Ajustes, captura «Merma al preparar: 5 %» y el costo de todo producto
con receta sube 5 %, sin tocar receta por receta.

**Why this priority**: es el pedido original y el que más trabajo ahorra: hoy la alternativa es
redondear cientos de recetas a mano.

**Independent Test**: con el general vacío, un producto con receta cuesta $20.00; capturar 5 % lo
deja en $21.00; vaciarlo lo regresa a $20.00.

**Acceptance Scenarios**:

1. **Given** el general vacío (como arranca), **When** se sube esta entrega, **Then** ningún costo
   cambia.
2. **Given** 5 % general y un producto sin otro porcentaje cuyos artículos no tienen uno propio,
   **When** se recalcula, **Then** su costo sube 5 %.
3. **Given** 5 % general, **When** se vacía, **Then** los costos vuelven a lo de antes.
4. **Given** un producto con costo capturado a mano, **When** se captura el general, **Then** su
   costo no cambia (la merma aplica a lo que sale de una receta).
5. **Given** quien no tiene el permiso de configurar el menú, **When** intenta cambiar el general,
   **Then** se niega.

---

### User Story 2 - Un producto con su propio porcentaje (Priority: P1) — S1

En la ficha de un producto que derrama más (un frappé que se sirve hasta el borde), quien configura
el menú captura 10 %. Ese producto usa 10 % en lugar del general.

**Why this priority**: sin esto, el general castiga a los productos que casi no pierden o se queda
corto con los que pierden mucho.

**Independent Test**: con 5 % general y 10 % en el producto, el costo sube 10 %, no 15 %.

**Acceptance Scenarios**:

1. **Given** 5 % general y 10 % en el producto, **When** se recalcula, **Then** usa 10 %, no 15 %.
2. **Given** 0 % capturado en el producto, **When** hay 5 % general, **Then** ese producto no suma
   merma.
3. **Given** un producto con 10 %, **When** se le quita el porcentaje, **Then** vuelve a usar el
   general.

---

### User Story 3 - Un artículo con su propio porcentaje se respeta, también en 0 % (Priority: P1) — S1

En la ficha de un artículo (la fresa, que se limpia y se tira el rabo), quien configura el menú
captura 15 %. Dentro de cualquier producto, la fresa usa 15 %; el resto de los artículos de ese
producto usa el del producto o el general. Un artículo que no se pierde (una lata) se captura en 0 % a
propósito y nunca suma merma.

**Why this priority**: es la regla de precedencia del pedido, y el 0 % capturado es lo único que
distingue «no se pierde» de «nadie lo ha revisado».

**Independent Test**: un producto con fresa (15 %), leche (sin captura) y lata (0 % capturado), con
10 % en el producto: la fresa suma 15 %, la leche 10 % y la lata nada.

**Acceptance Scenarios**:

1. **Given** fresa con 15 % y el producto con 10 %, **When** se recalcula, **Then** la fresa suma 15 %
   dentro de ese producto.
2. **Given** lata con 0 % capturado y 5 % general, **When** se recalcula, **Then** la lata no suma.
3. **Given** un artículo sin captura, **When** se ve su ficha, **Then** dice «Sin capturar» y no
   «0 %».
4. **Given** un artículo con 0 % capturado, **When** se borra su porcentaje, **Then** vuelve a «Sin
   capturar» y sigue la precedencia.
5. **Given** los artículos que hoy tienen un porcentaje mayor a 0 (importado), **When** se sube esta
   entrega, **Then** lo conservan y cuentan como capturados; los que están en 0 quedan «Sin capturar».

---

### User Story 4 - Ver de dónde sale el costo y cuánto sumó la merma (Priority: P1) — S1

Quien revisa márgenes abre un producto y ve su costo partido: cada artículo de la receta con su
cantidad, su costo, el % que se le aplicó, de dónde vino (artículo, producto o general) y cuántos
pesos sumó; abajo, el costo sin merma, lo que sumó la merma y el costo final.

**Why this priority**: con tres fuentes posibles, un costo sin explicación no se puede auditar; el
pedido lo exige.

**Independent Test**: el producto del ejemplo anterior muestra tres renglones con «Artículo · 15 %»,
«Producto · 10 %» y «Artículo · 0 %», y la suma de lo que sumó cada renglón es la merma total.

**Acceptance Scenarios**:

1. **Given** un producto con receta, **When** se abre su costo, **Then** cada renglón dice su %, de
   dónde vino y cuánto sumó, y el total de merma es la suma de los renglones.
2. **Given** un renglón que es un preparado (jarabe hecho en casa), **When** se ve, **Then** su costo
   ya incluye la merma de sus propios artículos y el renglón muestra solo la merma aplicada al
   usarlo.
3. **Given** un producto sin receta (costo a mano o de compra), **When** se abre su costo, **Then**
   dice de dónde sale el costo y que la merma no aplica.
4. **Given** un combo, **When** se abre su costo, **Then** se ve la suma de sus productos, cada uno
   con lo que sumó su merma.

---

### User Story 5 - La merma real de 90 días junto al porcentaje (Priority: P2) — S1

Junto al % esperado de cada artículo se ve cuánto se ha mermado de verdad en los últimos 90 días:
lo registrado como merma entre lo que se consumió por ventas, por almacén. Si la real es mucho mayor
que la esperada, quien configura decide subir el % o cambiar de presentación. Solo informa: no cambia
el costo.

**Why this priority**: es el dato que habría evitado el caso del insumo caducado (#146); sin él el %
se captura a ciegas.

**Independent Test**: con 10 kg vendidos y 0.5 kg mermados en el almacén de venta en 90 días, el
artículo muestra «Real: 5 %»; el costo del producto no cambia al aparecer el dato.

**Acceptance Scenarios**:

1. **Given** 10 kg consumidos por ventas y 0.5 kg mermados en 90 días en el almacén de venta, **When**
   se ve el artículo, **Then** dice 5 % real en ese almacén.
2. **Given** una merma registrada como comida del personal, una rechazada, una por aprobar o una
   deshecha, **When** se calcula la real, **Then** no cuenta.
3. **Given** un artículo con merma pero sin ventas en 90 días, **When** se ve, **Then** dice cuánto se
   mermó y «sin ventas», no un porcentaje infinito.
4. **Given** un artículo sin merma ni ventas en 90 días, **When** se ve, **Then** dice «Sin datos».
5. **Given** dos almacenes (local y bodega) con merma en los dos, **When** se ve el artículo, **Then**
   aparece la cifra de cada almacén.
6. **Given** la merma real, **When** cambia, **Then** el costo calculado no cambia.

---

### User Story 6 - Comparar el conteo con lo que las ventas descontaron (Priority: P1) — S2

Al cerrar el conteo de un almacén, quien revisa ve por artículo: lo que había en el conteo anterior,
lo que entró, lo que las ventas descontaron, lo registrado como merma, lo esperado, lo contado, la
diferencia y su % sobre lo vendido. Así separa la merma de lo que se consumió sin receta o sin venta.

**Why this priority**: es la herramienta para fijar el % con datos y para encontrar recetas mal
capturadas; sin ella el % esperado es una opinión.

**Independent Test**: un artículo con 10 kg al inicio, 5 kg comprados, 12 kg descontados por ventas,
0.3 kg de merma y 2.2 kg contados muestra diferencia de −0.5 kg y −4.2 % sobre lo vendido.

**Acceptance Scenarios**:

1. **Given** el ejemplo de arriba, **When** se abre el reporte, **Then** muestra esas cifras por
   renglón.
2. **Given** el reporte del almacén «Bodega», **When** se abre, **Then** solo cuenta movimientos de
   «Bodega»; los traspasos aparecen como entrada o salida, no como venta.
3. **Given** un artículo con diferencia estable de 2 a 4 % en los últimos tres conteos, **When** se ve
   el reporte, **Then** sugiere ese % para el artículo, y capturarlo es un toque explícito.
4. **Given** un artículo cuya diferencia cambia mucho de un conteo a otro o pasa de 10 %, **When** se
   ve, **Then** no sugiere %: lo marca para revisar receta o captura.
5. **Given** el primer conteo de un almacén, **When** se abre, **Then** dice que aún no hay con qué
   comparar.

---

### User Story 7 - Que la merma esperada también descuente inventario (Priority: P2) — S2

Quien configura el almacén activa «La merma esperada también descuenta del almacén». Desde ese
momento, cada venta descuenta del almacén de venta la cantidad de la receta más su merma esperada, y
la parte de merma se ve aparte en los movimientos.

**Why this priority**: con la opción apagada, el conteo encuentra «de menos» justo lo que el costo ya
previó; con ella, la existencia del sistema se acerca a lo físico.

**Independent Test**: con la opción activa y 10 % en la fresa, vender un producto con 100 g de fresa
descuenta 110 g: 100 por venta y 10 por merma esperada, en dos renglones.

**Acceptance Scenarios**:

1. **Given** la opción apagada (como arranca), **When** se vende, **Then** descuenta como hoy.
2. **Given** la opción activa, **When** se vende, **Then** descuenta receta + merma esperada, con la
   merma como movimiento propio.
3. **Given** una venta con la opción activa, **When** se cancela con reposición, **Then** repone
   también la merma esperada que descontó.
4. **Given** ventas anteriores a activarla, **When** se activa, **Then** no se recalculan ni descuentan
   nada atrás.
5. **Given** el reporte conteo vs ventas, **When** la opción está activa, **Then** la merma esperada
   descontada aparece en su columna, no dentro de lo vendido.

---

### User Story 8 - Contar sin capacitación en la vista sencilla (Priority: P1) — S2

Una persona nueva abre el conteo en la vista sencilla: elige el almacén, ve el recorrido por lugar,
un artículo a la vez o en lista corta, teclea lo que hay en la presentación que tiene en la mano, y
ve cuánto le falta. Quien administra cambia a la vista avanzada para ver lo esperado, las
diferencias, los lugares y las opciones, y puede regresar sin perder lo capturado.

**Why this priority**: es el criterio de aceptación de la ampliación: un conteo completo sin ayuda.

**Independent Test**: alguien de una fonda que nunca vio el sistema cuenta un almacén de 30
artículos en la vista sencilla sin preguntar nada; cambiar a avanzada y regresar conserva las 30
cifras.

**Acceptance Scenarios**:

1. **Given** la vista sencilla, **When** se cuenta, **Then** no se muestra la cantidad esperada
   (conteo a ciegas) y se ve el avance «N de M».
2. **Given** 12 artículos capturados en la sencilla, **When** se cambia a avanzada y se regresa,
   **Then** siguen los 12.
3. **Given** la tableta se recarga o se cae la red a media captura, **When** se vuelve, **Then** lo
   capturado sigue ahí.
4. **Given** la vista avanzada, **When** se abre, **Then** muestra esperado, diferencia, lugares y las
   acciones de configurar y cerrar.
5. **Given** quien solo tiene permiso de contar, **When** abre el conteo, **Then** entra en la vista
   sencilla y no ve lo esperado.

---

### User Story 9 - El inventario carga rápido (Priority: P2) — S2

Existencias y la lista del conteo cargan en menos tiempo que la meta, medida contra lo de hoy en el
equipo de mostrador y con el catálogo real.

**Why this priority**: es queja explícita del dueño; una pantalla lenta en mostrador se abandona.

**Independent Test**: con la base restaurada (≈ 800 artículos), la primera pantalla de Existencias y
del conteo se ve en menos de la meta, medida antes y después.

**Acceptance Scenarios**:

1. **Given** la base restaurada, **When** se abre Existencias, **Then** los primeros renglones se ven
   antes de la meta y el resto carga sin congelar la pantalla.
2. **Given** el conteo de un almacén, **When** se abre, **Then** la lista del primer lugar se ve antes
   de la meta.
3. **Given** el cambio de vista o de almacén, **When** se hace, **Then** no vuelve a bajar lo que ya
   tenía.

### Edge Cases

- **Prep dentro de prep**: un preparado usa otro preparado; la merma de cada nivel aplica con la
  merma propia del artículo, y el % del producto o el general solo en los renglones de la receta del
  producto, para que «5 % general» sea 5 % y no 5 % compuesto en cada nivel.
- **Extra (opción) con receta**: no es un producto, así que su precedencia es artículo → general.
  Una opción que es otro producto usa el costo de ese producto, con su %.
- **Porcentaje de 99.99 % o negativo**: se acepta de 0 a menos de 100, con dos decimales; fuera de
  eso se rechaza sin guardar nada. Vacío no es 0.
- **Producto de costo manual o de compra** con % capturado: se guarda, pero no aplica mientras el
  costo no salga de receta; la pantalla lo dice.
- **Ventas pasadas**: guardan el costo con el que se vendieron; ningún cambio de % las reescribe.
- **Dos personas cambian el % general a la vez**: gana la última, y la bitácora guarda las dos.
- **Merma real con consumo diminuto** (0.001 kg vendidos): se muestra el % redondeado y con el
  aviso de pocas ventas; con menos de una venta en el periodo, «sin ventas».
- **Artículo unido a otro** (spec de unir insumos): su merma y su consumo cuentan en el destino.
- **Cambio de unidad de un artículo**: el cambio crea un artículo nuevo y archiva el viejo; la merma
  real del nuevo cuenta desde el cambio (sus 90 días empiezan ahí) y no mezcla movimientos en otra
  unidad. Las uniones, en cambio, sí se siguen con su factor.
- **Opción «descontar merma» activa y artículo sin captura**: descuenta según la precedencia (puede
  ser el general); un 0 % capturado no descuenta.
- **Cambiar de vista a media captura** con un renglón a medio teclear: el renglón se guarda antes de
  cambiar.
- **Una sucursal con un solo almacén**: el reporte y el conteo no muestran selector, como en la
  spec 094.

## Requirements *(mandatory)*

### Functional Requirements

**Subida 1 — Costos**

- **FR-001**: La empresa MUST poder fijar un porcentaje general de merma esperada, vacío por omisión;
  vacío no suma nada.
- **FR-002**: Cada producto MUST poder tener un porcentaje propio opcional que reemplaza al general
  para los renglones de su receta.
- **FR-003**: Cada artículo MUST poder estar «sin captura» o tener un porcentaje capturado, incluido
  0 %; capturado reemplaza al del producto y al general en cualquier receta donde aparezca.
- **FR-004**: El % que se aplica a cada renglón de la receta de un producto MUST seguir: artículo
  capturado → producto → general → nada. En la receta de un extra: artículo → general. Dentro de un
  preparado: solo el del artículo.
- **FR-005**: Los porcentajes MUST aceptar de 0 a menos de 100 con dos decimales; vacío es «sin
  captura», nunca 0; un valor fuera de rango se rechaza.
- **FR-006**: Cambiar cualquier porcentaje MUST recalcular en ese momento los costos de productos,
  extras y preparados afectados, y no MUST cambiar el costo guardado de ventas pasadas.
- **FR-007**: La migración MUST dejar como capturados los artículos que hoy tienen un % mayor a 0 y
  «sin captura» los que están en 0; ningún costo cambia al subir.
- **FR-008**: Se MUST poder ver, por producto, el desglose del costo: por renglón, cantidad, costo
  sin merma, % aplicado, de dónde vino y pesos sumados; y en total, costo sin merma, merma y costo
  final. La suma de los renglones MUST ser el total.
- **FR-009**: Para un producto sin receta o un combo, el desglose MUST decir de dónde sale el costo.
- **FR-010**: Se MUST mostrar por artículo y por almacén la merma real de 90 días: merma aplicada que
  no es comida del personal ÷ consumo por ventas, con la cantidad mermada; «sin ventas» y «sin datos»
  cuando no hay divisor o no hay nada.
- **FR-011**: La merma real MUST verse junto al % esperado en el desglose del costo y en la ficha del
  artículo, y no MUST cambiar ningún costo.
- **FR-012**: Configurar los porcentajes MUST pedir el permiso de configurar el menú, igual que ver
  el desglose del costo. La merma real (consumo y mermas por almacén) MUST pedir además el permiso de
  ver el almacén: sin él, el desglose llega sin merma real.
- **FR-013**: Cada cambio de porcentaje (general, producto, artículo) MUST quedar en la bitácora con el
  valor anterior y el nuevo.

**Subida 2 — Inventario y conteo** (sobre las pantallas de la spec 094)

- **FR-020**: Al cerrar el conteo de un almacén se MUST poder ver por artículo: conteo anterior,
  entradas (compras y traspasos entrantes), salidas por venta, merma esperada descontada, merma
  registrada, traspasos salientes, esperado, contado, diferencia y % sobre lo vendido.
- **FR-021**: El reporte MUST sugerir un % para el artículo solo cuando su diferencia fue estable
  (entre 2 y 4 % con poca variación) en los últimos tres conteos, y MUST marcar «revisar receta o
  captura» cuando es mayor o errática; aplicarlo es un toque explícito que queda en la bitácora.
- **FR-022**: La empresa MUST poder activar que la merma esperada descuente inventario, apagado por
  omisión; activo, cada venta nueva descuenta del almacén de venta su merma esperada como movimiento
  propio, y la cancelación con reposición la repone.
- **FR-023**: Activar o apagar la opción no MUST tocar ventas pasadas ni existencias.
- **FR-024**: El conteo MUST tener una vista sencilla (solo contar, a ciegas, recorrido por lugar,
  avance visible, presentación a elegir) y una avanzada (esperado, diferencias, lugares, configurar,
  cerrar), intercambiables sin perder lo capturado, también tras recargar. Al teclear una cantidad
  menor a la mitad o mayor al doble del conteo anterior, la sencilla MUST pedir confirmarla sin
  mostrar la cifra esperada.
- **FR-025**: Quien solo puede contar MUST entrar siempre a la sencilla; la avanzada pide el permiso
  de configurar el almacén (`inventory.configure`).
- **FR-026**: Existencias MUST tener la misma separación: vista sencilla (qué hay y qué falta) y
  avanzada (valor, movimientos, mínimos, configuración).
- **FR-027**: Existencias y la lista del conteo MUST cargar su primera pantalla por debajo de la meta
  de SC-007, medida antes y después sobre la base restaurada.

### Key Entities

- **Merma general**: porcentaje de la empresa, opcional.
- **Merma del producto**: porcentaje opcional por producto.
- **Merma del artículo**: «sin captura» o porcentaje capturado (0 incluido).
- **Desglose del costo**: por producto, renglones con %, origen del % y pesos sumados; no se guarda,
  se calcula.
- **Merma real**: por artículo y almacén en 90 días; cantidad mermada, consumo por ventas y su
  cociente; no se guarda.
- **Reporte conteo vs ventas** (S2): por conteo cerrado de un almacén, un renglón por artículo.
- **Opción de descontar merma** (S2): ajuste de empresa; el movimiento de merma esperada es una clase
  de movimiento propia.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Al subir S1, 0 costos de producto, extra o preparado cambian (general vacío y los
  artículos conservan su %).
- **SC-002**: Con 5 % general, un producto sin otros porcentajes sube exactamente 5 % (al centavo).
- **SC-003**: En el desglose, la suma de lo que sumó cada renglón es igual al total de merma al
  centavo, en el 100 % de los productos con receta de la base restaurada.
- **SC-004**: Ninguna venta cerrada antes de un cambio de % cambia su costo.
- **SC-005**: Quien configura el menú captura el general y el % de un producto en 4 toques o menos
  cada uno, a 1024×600.
- **SC-006**: Una persona nueva de una fonda hace un conteo completo de 30 artículos sin ayuda en la
  vista sencilla (S2).
- **SC-007**: La primera pantalla de Existencias y del conteo se ve en 1 s o menos, y la lista
  completa en 2 s o menos, en el build de producción servido en local con la base restaurada, a
  1024×600 con CPU limitada a 4× (simulación del equipo de mostrador); se mide antes del cambio y se
  reporta contra esa cifra (S2).
- **SC-008**: Cambiar de vista sencilla a avanzada y de regreso conserva el 100 % de lo capturado
  (S2).

## Cómo lo resuelven otros POS de restaurante

Ver [research.md › Otros POS](research.md#otros-pos) para las fuentes. Lo que se adopta:

- Conteo por zona de guardado, en el orden del recorrido, en cualquier presentación (Toast,
  Restaurant365, Craftable, Square).
- Conteo a ciegas: lo esperado aparece solo en la revisión y para quien tiene permiso
  (Restaurant365; opción en Apicbase). Loyverse sí lo muestra; aquí no.
- Aviso al contar contra el conteo anterior («menos de la mitad», «más del doble»), sin revelar lo
  esperado (Restaurant365).
- Reporte real vs teórico con la merma registrada en su columna y la diferencia «no explicada» aparte,
  con un tope aceptable (Restaurant365, xtraCHEF).
- Merma esperada por ingrediente con una por omisión más general (xtraCHEF); aquí el nivel específico
  reemplaza, no se suma.
- Lo no contado no se toca (Apicbase, Lightspeed); no se copia el conteo de Square que pone en cero lo
  no contado.
- Ninguna ayuda publica tiempos de carga: la meta se mide contra la pantalla de hoy.

## Assumptions

- «Configurar el menú» es el permiso `menu.configure` que hoy ve y edita costos; no hay permiso nuevo
  en S1 (decisión del #226).
- La merma general y la del producto aplican solo a los renglones de la receta del producto (y la
  general a la de los extras); dentro de un preparado manda el % del artículo, como hoy. Así un 5 %
  general sube 5 % y no se compone por nivel.
- «Consumo por ventas» es lo que las ventas descontaron del almacén por receta, neto de
  cancelaciones; la merma esperada que S2 descuente no es consumo, ni los traspasos.
- La merma real se calcula al pedirla; con un año de movimientos no hace falta guardarla.
- S1 sube antes que el #65 complete sus entregas: la merma real usa el almacén que la spec 094 ya
  guarda en mermas y movimientos (con un solo almacén se ve una sola cifra).
- La sugerencia de % de S2 usa los tres últimos conteos cerrados del almacén; con menos, no sugiere.
- Fuera de alcance: caducidad por lote (#146), costo por almacén, recalcular ventas pasadas, merma
  esperada por presentación o por proveedor.

## Lo que encontró la prueba con alguien nuevo (2026-11-23, subida 1)

Dueña de una fonda sin capacitación, a 1024×600 en local con la base restaurada. Logró las tres
tareas (merma general en 6 toques, la del Capuccino en 10, «no se pierde nada» en los palillos en 9).
Toques con ratón sobre un contexto táctil: los eventos táctiles no se verificaron ahí (la e2e sí los
usa).

| Lo que se atoró | Qué se hizo |
| --- | --- |
| Tras «Guardar» en Costos no supo si quedó | Mensaje «Guardado. Los costos ya incluyen la merma.» |
| En el artículo buscó un botón Guardar: se guarda al salir del campo sin decirlo | Mensaje «Guardado» bajo el campo |
| «al servirlo» en una tarjeta que se llama «al preparar» | Dice «al prepararlo» |
| El campo vacío del producto mostraba el general en gris y parecía escrito | Sin texto de ejemplo; la ayuda debajo dice «Usa la general: N %» |
| «Merma real, últimos 90 días» sin explicación, y «Local» no se entendía | Encabezado «…por almacén» y un (i) con qué es y qué hacer con ella |
| Un 0 % con ventas se leía como «sin datos» | «0 % · sin merma registrada» |
| **La merma general también se suma a vasos, tapas y popotes** | **Pendiente de decisión del dueño** (ver abajo) |
| La lista de productos no dice que el costo trae merma | Sin cambio: el desglose está a un toque en la ficha |

**Decisión pendiente del dueño**: hoy la merma general aplica a todo renglón de la receta sin % propio,
empaques incluidos. Los artículos ya tienen la marca «es empaque»; excluirlos del general es un cambio
de una regla en `domain.ResolveExpectedWaste` y no toca datos. Mientras tanto, capturar 0 % en esos
artículos los deja fuera.

### Segunda vuelta, con toques táctiles reales (2026-11-23)

Otra persona nueva, navegador propio con `hasTouch` a 1024×600 y solo `tap()`. Logró las tres tareas
(8, 8 y 9 toques). Encontró un defecto: tras capturar 0 % en el artículo no apareció ningún aviso.
El campo estaba ligado a su valor como llave y el refresco de la ficha lo volvía a montar, borrando
«Guardado» y cerrando el (i); queda ligado al artículo, con su prueba. El (i) de la merma real ya no
dice «entre lo que se vendió» (se leía «en medio de»): dice «qué porcentaje de lo vendido».

Sin cambio, anotado: a 600 px de alto «Ajustes» queda fuera de la barra lateral y el chevron del pie
no la desplaza (es de la barra, no de esta spec); el almacén se llama «Local» y el lugar de conteo
«Almacén principal», nombres que el negocio puede cambiar.
