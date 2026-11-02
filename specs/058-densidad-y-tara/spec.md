# Feature Specification: Densidad y tara de los artículos del almacén

**Feature Branch**: `058-densidad-y-tara`

**Created**: 2026-11-02

**Status**: Draft

**Input**: Issue #117 con su ampliación. Tres cosas en una sola spec (decisión del dueño,
2026-11-02): la densidad de un artículo (para cambiar su unidad entre peso y líquido y para que una
compra en la otra clase de unidad se convierta sola), la tara de cada envase, y el conteo por peso
bruto.

**Evidencia** (hechos del mundo, sin números del negocio): la leche condensada se compra por lata o
por peso y se usa en ml; los siropes y las salsas llegan en galones etiquetados por peso y se miden
en ml; el equipo cuenta pesando el envase con el producto adentro. Hoy:

- Cambiar un artículo de g a ml (o al revés) se rechaza: «no se puede sin la densidad». El camino
  que queda es archivarlo y crear otro, y se pierde la historia de compras y costos.
- Una compra en kg de un artículo que se guarda en ml se rechaza: «la unidad de compra no es
  compatible».
- El conteo pide la cantidad neta; quien cuenta pesa el envase lleno y resta de cabeza.

**Principios fijos** (no se negocian en esta spec):

- **Nunca se asume 1 g = 1 ml.** Sin densidad capturada, lo que hoy se rechaza se sigue rechazando.
- **La densidad es del artículo y la pone una persona**, con una pesada o escrita a mano. No hay
  tabla de densidades genéricas.
- **Rehacer la pesada aplica de aquí en adelante** (decisión del dueño, 2026-11-02): lo ya convertido
  —compras ya sumadas al almacén, cambios de unidad ya hechos, conteos ya capturados— no se
  recalcula. La diferencia la ajusta un conteo.
- Todo queda en la bitácora: quién puso qué densidad o qué tara, cómo, y cuándo.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Capturar la densidad de un artículo con una pesada (Priority: P1)

Quien configura el almacén abre «Salsa de chocolate» (se guarda en ml) en Almacén › Artículos y
toca «Densidad». La hoja pide tres datos: cuánto pesa un envase lleno (g), cuánto pesa vacío (g) y
cuánto contiene según la etiqueta (ml). Con 1,250 g lleno, 150 g vacío y 1,000 ml, la hoja dice
«1.10 g por ml» antes de guardar. Al guardar, la ficha del artículo muestra «Densidad: 1.10 g/ml ·
por pesada · Ana, 2 nov». Si alguien ya conoce el valor, cambia a «Escribir el valor» y captura
1.10 directamente.

**Why this priority**: las otras tres historias dependen de que el artículo tenga densidad. Sin
esta, ninguna conversión entre peso y líquido es posible.

**Independent Test**: en un artículo en ml sin densidad, capturar la pesada 1,250 / 150 / 1,000 y
guardar. La ficha muestra 1.1 g/ml, el método y quién la capturó, y la bitácora tiene el evento con
los tres datos de la pesada.

**Acceptance Scenarios**:

1. **Given** un artículo en ml o en g sin densidad, **When** se captura la pesada lleno 1,250 g,
   vacío 150 g, contenido 1,000 ml, **Then** la hoja muestra 1.10 g/ml antes de guardar, y al
   guardar el artículo queda con densidad 1.1 por pesada, con quién y cuándo.
2. **Given** la hoja de densidad, **When** se elige «Escribir el valor» y se captura 0.92, **Then**
   el artículo queda con 0.92 g/ml marcado como escrito a mano.
3. **Given** un artículo nuevo que nadie puede pesar, cuya etiqueta dice «peso neto 4.3 kg ·
   contenido 3.785 L», **When** en «Escribir el valor» se elige «Con la etiqueta» y se capturan
   4,300 g y 3,785 ml, **Then** la hoja muestra 1.14 g por ml y el artículo queda con 1.1361
   marcado como «de la etiqueta».
3. **Given** un artículo con densidad 1.10, **When** se rehace la pesada y da 1.08, **Then** el
   artículo queda con 1.08, la bitácora guarda el valor anterior y el nuevo, y nada de lo ya
   convertido cambia (ver US2 escenario 4).
4. **Given** la pesada, **When** el vacío pesa lo mismo o más que el lleno, o el contenido es cero,
   **Then** la hoja no deja guardar y dice qué dato revisar.
5. **Given** una pesada o un valor que da una densidad fuera de lo posible para un alimento (menos
   de 0.30 o más de 3.00 g/ml), **When** se intenta guardar, **Then** se rechaza con «revisa la
   pesada: da X g/ml».
6. **Given** un artículo con densidad, **When** se quita la densidad, **Then** el artículo queda
   sin ella y vuelven los rechazos de hoy; lo ya convertido no cambia. Si hay una compra del
   artículo en la otra clase de unidad que todavía no entra al almacén, no se deja quitar: «hay una
   compra por recibir en kg que la necesita».
7. **Given** un artículo que se cuenta en piezas (o en cm), **When** se abre su ficha, **Then** no se
   ofrece densidad: solo aplica a artículos en g o en ml.

---

### User Story 2 - Una compra en peso de un artículo en ml se convierte sola (Priority: P1)

Al capturar un gasto, el renglón «Salsa de chocolate · 4.3 kg» (el galón viene etiquetado por peso)
hoy se rechaza. Con la densidad del artículo en 1.10, el renglón se acepta y dice cómo se convirtió:
«4,300 g ÷ 1.10 g/ml = 3,909 ml». Al recibirlo, entran 3,909 ml al almacén y el costo por ml sale
del importe entre 3,909. Si el artículo no tiene densidad, el renglón sigue sin aceptarse, pero el
aviso trae el atajo «Capturar la pesada», que abre la hoja de densidad de ese artículo y, al
guardarla, regresa al gasto con el renglón ya convertido.

**Why this priority**: es el hueco que más estorba al capturar compras reales: hoy obliga a hacer la
cuenta a mano y capturar en ml algo que el ticket dice en kg.

**Independent Test**: artículo en ml con densidad 1.10; capturar un gasto con 4.3 kg de él y
recibirlo. La existencia sube 3,909.0909 ml (redondeo de 4 decimales), el renglón muestra la
conversión con la densidad usada, y el costo por ml es el importe entre esa cantidad.

**Acceptance Scenarios**:

1. **Given** un artículo en ml con densidad 1.10, **When** se captura un renglón de 4.3 kg, **Then**
   el renglón se acepta y muestra «4,300 g ÷ 1.10 = 3,909 ml» antes de guardar.
2. **Given** un artículo en g con densidad 0.92, **When** se captura un renglón de 2 L, **Then** se
   convierte a 1,840 g y lo muestra «2,000 ml × 0.92 = 1,840 g».
3. **Given** un artículo en ml sin densidad, **When** se captura un renglón en kg, **Then** el
   renglón no se acepta como hoy, y el aviso ofrece «Capturar la pesada»; al guardar la pesada, el
   renglón se convierte sin volver a capturarlo.
4. **Given** un renglón ya recibido con densidad 1.10, **When** se rehace la pesada a 1.08, **Then**
   la existencia que ya sumó ese renglón no cambia, y el renglón sigue diciendo que se convirtió con
   1.10. Si después se corrige lo recibido de ese mismo renglón (llegó menos), la corrección usa
   también 1.10.
5. **Given** un renglón capturado pero todavía no recibido, **When** se cambia la densidad antes de
   recibirlo, **Then** al recibirlo se usa la densidad vigente en ese momento, y desde ahí queda fija
   para ese renglón.
6. **Given** un renglón comprado en una presentación del artículo («Galón 3,800 ml») o en piezas,
   **When** se captura, **Then** no usa densidad: se convierte como hoy.
7. **Given** la edición de un gasto ya guardado (agregar un renglón en kg de un artículo en ml),
   **When** se ve la vista previa, **Then** la conversión con densidad aparece igual que en la
   captura.

---

### User Story 3 - Cambiar la unidad de un artículo entre peso y líquido (Priority: P2)

«Leche condensada» se dio de alta en g y se usa en ml. Hoy no se puede cambiar. Con densidad 1.30,
en «Cambiar en qué se mide» se elige ml y la equivalencia ya viene puesta: «1 g = 0.7692 ml (sale
de la densidad 1.30 g/ml)». La vista previa es la de siempre —existencias, recetas y empaques
convertidos, por el camino directo o por el reemplazo según su historia— y el costo por ml sale del
costo por g × 1.30. Sin densidad, la opción entre peso y líquido no se ofrece y la hoja dice
«primero captura la densidad», con el atajo a la hoja de densidad.

**Why this priority**: es el issue original, pero el caso se puede rodear hoy archivando y creando
otro; las compras (US2) y el conteo (US4) no tienen rodeo.

**Independent Test**: artículo en g con 1,300 g en existencia, costo $0.10 por g y densidad 1.30;
cambiarlo a ml. La existencia queda en 1,000 ml, el costo en $0.13 por ml, y la bitácora dice que
la equivalencia salió de la densidad 1.30.

**Acceptance Scenarios**:

1. **Given** un artículo en g con densidad 1.30, **When** se pide cambiarlo a ml, **Then** la
   equivalencia propuesta es 1 ÷ 1.30 ml por g, la vista previa la muestra con la densidad, y al
   confirmar existencias, recetas y empaques quedan convertidos con ella y el costo por ml es el
   costo por g × 1.30.
2. **Given** un artículo en ml con densidad 0.92, **When** se cambia a g, **Then** la equivalencia
   es 0.92 g por ml y el costo por g es el costo por ml ÷ 0.92.
3. **Given** un artículo en g sin densidad, **When** se abre el cambio de unidad, **Then** ml no se
   ofrece como destino y la hoja ofrece capturar la densidad primero.
4. **Given** un artículo con compras, ventas o mermas, **When** se cambia entre peso y líquido,
   **Then** aplica el reemplazo de siempre (uno nuevo en la otra unidad, el viejo archivado), y el
   artículo nuevo conserva la densidad.
5. **Given** un cambio de unidad ya hecho con densidad 1.30, **When** se rehace la pesada, **Then**
   lo convertido en ese cambio no se recalcula.
6. **Given** un artículo con empaques que tienen tara, **When** cambia entre peso y líquido,
   **Then** la tara de cada empaque se conserva (es un peso del envase, no del contenido).

---

### User Story 4 - Contar por peso bruto con la tara del envase (Priority: P2)

Cada empaque de un artículo puede guardar cuánto pesa vacío: «Botella 1 L · vacía pesa 120 g». En
el conteo, debajo de los empaques cerrados, aparece plegado «Pesar con empaque · Opcional»: quien
cuenta pone en la báscula los empaques abiertos con lo que traen (850 g) y dice cuántos pesó (2). El
sistema resta lo que pesan vacíos (2 × 120 g = 240 g), y si el artículo se guarda en ml convierte con
la densidad: «850 g − 240 g = 610 g ÷ 1.10 = 554.5 ml». Lo pesado se suma a lo contado en empaques
cerrados; lo abierto se pesa o se teclea como suelto, no las dos.

**Why this priority**: ahorra la cuenta de cabeza en cada conteo, pero hoy se puede contar en neto.

**Independent Test**: artículo en ml con densidad 1.10 y un empaque «Botella 1 L» con tara 120 g;
abrir un conteo y capturar 3 botellas cerradas más 850 g en bruto con 2 empaques. Queda contado
3,554.5455 ml, y la pantalla mostró la cuenta antes de guardar.

**Acceptance Scenarios**:

1. **Given** un empaque de un artículo en g o ml, **When** se edita, **Then** se puede capturar su
   tara en g (opcional) y quitarla; queda en la bitácora.
2. **Given** un artículo en g con un empaque con tara 50 g, **When** en el conteo se pesan 3 empaques
   con 1,000 g en bruto, **Then** se cuenta 850 g.
3. **Given** un artículo en ml con densidad 1.10 y un empaque con tara 120 g, **When** se pesan 2
   empaques con 850 g en bruto, **Then** se cuenta 554.5455 ml, y la hoja muestra la cuenta completa
   antes de guardar.
4. **Given** un artículo en ml con un empaque con tara pero SIN densidad, **When** se abre el conteo,
   **Then** «Pesar con empaque» no se ofrece para ese artículo.
5. **Given** un artículo cuyos empaques no tienen tara, **When** se abre el conteo, **Then** no
   aparece «Pesar con empaque» y la captura es como hoy.
6. **Given** un peso bruto menor que lo que pesan vacíos los empaques que se dijo pesar, **When** se intenta
   guardar, **Then** se rechaza: «pesa menos que N empaques vacíos; revisa el peso o cuántos
   empaques».
7. **Given** un peso bruto que da cero neto (empaques vacíos), **When** se guarda, **Then** se cuenta
   cero de lo pesado: es un «no hay» válido.
8. **Given** el conteo a ciegas, **When** se captura por peso bruto, **Then** sigue a ciegas: la
   cuenta usa la tara y la densidad, nunca lo que el sistema tiene; el aviso de cifra rara y el
   máximo de la empresa aplican al resultado en la unidad base, como hoy.
9. **Given** una cifra ya capturada por peso bruto, **When** después se cambia la tara o la
   densidad, **Then** lo ya capturado no cambia; recapturarlo usa los valores vigentes.

---

### Edge Cases

- **El vacío que significa algo**: borrar la densidad o la tara para reescribirla no la deja en
  cero; vacío es «sin densidad» o «sin tara», y cero nunca es un valor válido para ninguna de las
  dos: borrar el campo manda «sin densidad» o «sin tara», y un cero escrito se rechaza.
- **Recarga a media pesada**: la hoja de densidad no guarda nada hasta confirmar; recargar la
  descarta. No queda una densidad a medias.
- **Dos personas a la vez**: si dos editan la densidad del mismo artículo, gana la última que
  guarda, y la bitácora tiene las dos.
- **El camino nuevo que se salta el control viejo**: la conversión por densidad entra en todos los
  caminos que suman compras al almacén —capturar, recibir, corregir lo que llegó, editar un gasto,
  anular— y todos usan la misma regla y la misma densidad fija del renglón.
- **El hermano que no se movió**: la pantalla que hoy convierte lo comprado para mostrarlo (captura
  de gasto, por recibir, detalle) usa la misma regla que el servidor; si no, mostraría 4.3 kg como
  «no compatible» mientras el servidor lo acepta.
- **Unión de insumos**: un insumo unido a otro (spec 056) no se edita; la densidad del destino es la
  que aplica a lo que llegue después. Si el destino no tiene densidad y los insumos que se le unen
  tienen la misma, el destino la recibe; si tienen distintas, la vista previa de la unión lo dice y no
  se copia ninguna. Un empaque del destino sin «pesa vacío» recibe el del duplicado que se archiva.
- **Artículo preparado** (insumo que sale de una receta): admite densidad igual que cualquier otro en
  g o ml.
- **Recetas**: una receta sigue capturándose en la clase de unidad del artículo. Capturar «50 g» de
  un artículo en ml queda fuera de esta spec.
- **Mermas**: se siguen capturando en la clase de unidad del artículo; la merma por peso bruto queda
  fuera.
- **Densidad extrema**: el rango aceptado (0.30 a 3.00 g/ml) cubre aceites, jarabes, mieles, leches
  y salsas; algo fuera de él casi siempre es un dedazo en la pesada (gramos por kilos).
- **Tara de un empaque de un artículo en piezas o en cm**: no se ofrece; ahí no se cuenta por peso
  bruto (el conteo por peso de artículos en piezas ya existe y no cambia).
- **Peso bruto en kg**: se puede capturar en g o en kg, como lo suelto del conteo.
- **Nadie puede pesar** (pregunta del dueño, 2026-11-02): la densidad y lo que pesa vacío un empaque
  son opcionales. Sin densidad todo funciona como hoy (la compra se captura en la unidad del
  artículo o con un empaque); sin «pesa vacío», el conteo no ofrece «Pesar con empaque». Para la
  densidad hay además una salida sin báscula: el peso neto y el contenido que trae la etiqueta.

## Requirements *(mandatory)*

### Functional Requirements

**Densidad**

- **FR-001**: Un artículo cuya unidad base es g o ml puede tener una densidad en g/ml, opcional.
  Artículos en piezas o en cm no la tienen.
- **FR-002**: La densidad se captura por omisión con una pesada —envase lleno (g), envase vacío (g)
  y contenido (ml)— y la calcula el sistema como (lleno − vacío) ÷ contenido. Como opción, se
  escribe: el valor en g/ml, o el peso neto (g) y el contenido (ml) de la etiqueta, y la calcula el
  sistema como neto ÷ contenido. La hoja muestra el resultado antes de guardar.
- **FR-003**: El sistema rechaza una pesada con vacío ≥ lleno o contenido ≤ 0, y toda densidad fuera
  de 0.30–3.00 g/ml, diciendo qué revisar.
- **FR-004**: Guardar, rehacer o quitar la densidad deja en la bitácora el valor anterior, el nuevo,
  el método (pesada, etiqueta o escrita) y los datos de la pesada o de la etiqueta, con quién y cuándo. La ficha del
  artículo muestra el valor vigente, su método, quién y cuándo.
- **FR-005**: Cambiar o quitar la densidad no recalcula nada ya convertido (compras sumadas,
  cambios de unidad hechos, conteos capturados).
- **FR-006**: Solo quien puede configurar artículos del almacén captura densidad y tara.

**Compras**

- **FR-007**: Un renglón de compra en una unidad de masa de un artículo en ml (o de volumen de uno
  en g) se convierte con la densidad del artículo: g → ml dividiendo, ml → g multiplicando. Sin
  densidad se rechaza como hoy.
- **FR-008**: El renglón guarda la densidad con la que se convirtió la primera vez que sumó al
  almacén, y toda corrección posterior de ese renglón (recibir de más o de menos, quitar, anular,
  editar) usa esa misma densidad. Un renglón que todavía no suma nada usa la vigente.
- **FR-009**: La captura del gasto, la edición de un gasto, «por recibir» y el detalle muestran la
  conversión usada en palabras: cantidad comprada, operación, densidad y resultado en la unidad base.
- **FR-010**: El rechazo por falta de densidad ofrece un atajo a la hoja de densidad del artículo, y
  al guardarla el renglón se vuelve a evaluar sin volver a capturarlo.
- **FR-011**: Una compra en presentación del artículo o en piezas no usa densidad.

**Cambio de unidad**

- **FR-012**: Con densidad, el cambio de unidad entre g y ml se ofrece, con la equivalencia derivada
  de la densidad (g → ml: 1 ÷ densidad; ml → g: densidad), y sigue los mismos caminos (directo o
  reemplazo), vista previa y bitácora que los demás cambios de unidad. La bitácora registra la
  densidad usada.
- **FR-013**: Sin densidad, el destino de la otra clase no se ofrece y la hoja ofrece capturarla.
- **FR-014**: El artículo que queda después del cambio (el mismo o el de reemplazo) conserva la
  densidad, y los empaques conservan su tara.

**Tara y conteo por peso bruto**

- **FR-015**: Un empaque de un artículo en g o ml puede guardar su tara en g, opcional y mayor que
  cero, con bitácora.
- **FR-016**: En el conteo, un artículo con al menos un empaque activo con tara (y con densidad si
  se guarda en ml) ofrece, plegado y opcional, «Pesar con empaque»: peso bruto (g o kg), empaque y
  cuántos empaques. Lo contado por peso bruto es (bruto − tara × empaques), en g, convertido a ml con
  la densidad si la base es ml, y se suma a lo contado en empaques cerrados. Pesar y teclear suelto
  son excluyentes en la misma captura.
- **FR-017**: El servidor hace esa cuenta con la tara y la densidad vigentes al capturar; un bruto
  menor que la tara de los empaques se rechaza; un neto cero es válido.
- **FR-018**: La captura por peso bruto respeta el conteo a ciegas, el máximo de la empresa y el
  aviso de cifra rara, sobre el resultado en la unidad base.
- **FR-019**: La hoja del conteo muestra la cuenta completa antes de guardar («850 g − 2 × 120 g =
  610 g ÷ 1.10 = 554.5 ml») con la tara y la densidad que recibió; si al guardar ya no son las
  vigentes, el servidor no guarda y la hoja recarga el artículo. Lo contado que se muestra después
  es el que calculó el servidor.
- **FR-020**: El renglón del conteo guarda lo pesado: el bruto, cuántos empaques, la tara y la
  densidad usadas. Recapturar sin pesar lo borra.
- **FR-021**: La densidad no se puede quitar mientras haya una compra del artículo en la otra clase
  de unidad que todavía no entra al almacén.

### Key Entities *(include if feature involves data)*

- **Densidad del artículo**: g/ml, el método con que se obtuvo (pesada, etiqueta o escrita), los
  datos si los hubo (lleno, vacío y contenido; o neto y contenido), quién y cuándo. Una por artículo; su historia vive en la bitácora.
- **Tara del empaque**: g que pesa el envase vacío. Una por empaque, opcional.
- **Pesada del conteo**: bruto, empaques, tara y densidad usadas en un renglón de conteo capturado
  pesando; explica después de dónde salió la cifra contada.
- **Densidad usada en un renglón de compra**: copia fija del valor con que el renglón se convirtió la
  primera vez. Es lo que impide que rehacer la pesada reescriba lo que ya entró.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Una compra en kg de un artículo que se guarda en ml se captura sin hacer la cuenta a
  mano, en el mismo número de toques que una compra en la unidad del artículo cuando ya tiene
  densidad.
- **SC-002**: Capturar la densidad con la pesada toma menos de un minuto para alguien que no la
  había hecho antes, sin conocer el concepto de g/ml.
- **SC-003**: Rehacer la pesada no cambia en un solo gramo ni mililitro la existencia ni el costo de
  lo ya recibido (verificado comparando antes y después).
- **SC-004**: Un artículo en g con compras se pasa a ml sin perder su historia de compras ni de
  costos (queda ligada por el reemplazo), en una sola operación.
- **SC-005**: Contar un artículo por peso bruto toma una sola captura por grupo de empaques abiertos,
  sin cuentas de cabeza, y la cifra contada coincide con la cuenta a mano al cuarto decimal.
- **SC-006**: Ninguna conversión entre peso y líquido ocurre en un artículo sin densidad.

## Assumptions

- El rango de densidad aceptado es 0.30 a 3.00 g/ml; cubre los alimentos y bebidas de un negocio de
  comida. Un valor fuera de él se toma como error de captura.
- La densidad se guarda a 4 decimales y se muestra a 2.
- La pesada se hace con el envase original, cuyo contenido nominal en ml viene en la etiqueta.
- La tara se captura en g; el peso bruto en g o kg.
- El permiso para capturar densidad y tara es el mismo que para configurar artículos del almacén.
- La densidad no se pide al dar de alta un artículo: se captura en su ficha o desde el atajo.
- Recetas y mermas por peso de un artículo en ml (y al revés) quedan fuera de esta spec.
