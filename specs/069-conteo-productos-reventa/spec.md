# Feature Specification: Conteo con productos de existencia propia, y quitarles el contador

**Feature Branch**: `069-conteo-productos-reventa`

**Created**: 2026-11-08

**Status**: Draft

**Input**: Issue #252 «Conteo de inventario: incluir productos con existencia propia (reventa) y permitir quitarles el contador».

## Contexto

Un producto con **existencia propia** es uno que el sistema cuenta por sí mismo, en piezas, sin
receta: una lata o una botella que se vende tal cual. Desde la spec 037 la pantalla ya no ofrece
crear productos así (los revendidos nuevos se modelan como receta de un artículo por pieza), pero
los que ya existían siguen contando y bajando con cada venta. Medido en la copia de producción del
2026-10-22, en la empresa real: **93 productos activos con existencia propia, 34 en negativo, 14
con grupos de opciones**.

Dos huecos:

1. **No se pueden contar.** El conteo de inventario solo lista artículos del almacén. Si el equipo
   cuenta 15 latas y el sistema dice −2 porque nunca se registró la compra, no hay dónde capturar
   «son 15». La cifra queda mal, el aviso de «comprar» no sirve y el valor del inventario no cuadra.
2. **No se les puede quitar el contador cuando lo que resta son sus opciones.** Un producto con un
   grupo de sabores donde cada sabor resta su propio artículo no necesita contarse a sí mismo. Hoy
   la única salida que no exige insumos se llama «No se cuenta — No se resta nada», que es falso
   para ese producto (sus sabores sí restan); quien opera elige «Se prepara aquí», la pantalla le
   pide al menos un insumo y el contador propio sigue vivo y bajando.

## Decisiones del dueño (issue #252)

- Mismo permiso que el conteo de artículos.
- Los productos sin existencia propia (los que restan por receta, paquete u opciones) **no**
  aparecen en el conteo.
- Nada fijo de un negocio: aplica a cualquier empresa.

## Decisiones tomadas en este spec (recomendación, sin preguntar)

| Duda | Decisión | Por qué |
| --- | --- | --- |
| ¿Se cuentan piezas sueltas o fracciones? | Solo piezas enteras, cero incluido («no hay») | Una lata partida no se vende; un decimal ahí es un dedazo |
| ¿Dónde aparecen en la lista del conteo? | Al final, agrupados por su categoría del menú, con un aviso arriba de cuántos hay (maqueta C, elegida por el dueño); sin «lugar» propio en esta entrega | Dar lugar a un producto se puede agregar después al mismo costo (principio VI) |
| ¿Aplican el máximo y el aviso de cifra rara del conteo? | Sí, los de piezas de la empresa | Una cifra de 1500 latas es el mismo dedazo que 1500 piezas de un artículo |
| ¿El reporte de lo que se perdió incluye productos? | No en esta entrega, pero el cierre guarda el costo de cada producto contado y el último movimiento que vio | Ese costo al cerrar no se puede reconstruir después (principio VIII); el reporte sí se puede agregar luego |
| Producto que deja de tener existencia propia con el conteo abierto | Su cifra se ignora al cerrar y la vista previa lo dice | Ajustarlo revive un contador que alguien acaba de apagar a propósito |
| ¿Cuándo se ofrece «Lo restan sus opciones»? | Cuando el producto tiene al menos un grupo de opciones con alguna opción que resta algo del almacén, y entonces **ocupa el lugar de «No se cuenta»** (maqueta B, elegida por el dueño); el servidor lo vuelve a revisar al guardar | Elegirla en un producto cuyas opciones no restan nada vendería sin descontar; «No se resta nada» sería falso en ese producto |
| ¿Y si ningún grupo que resta es obligatorio? | Se permite, avisando en la hoja que una venta sin elegir opción no resta nada | Es configuración legítima (un extra opcional) y el aviso evita la sorpresa |
| ¿Adónde va la existencia del producto al salir a «Lo restan sus opciones»? | Se cierra en cero con el movimiento de cambio de modo que ya existe; la hoja recomienda contar los artículos de las opciones | No hay forma de saber cuántas de las latas son de cada sabor: repartirla sería inventar |

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Contar los productos que se venden tal cual (Priority: P1)

Quien hace el conteo abre «Conteo» en Almacén y, además de los artículos, ve los productos que
llevan su propia existencia (la lata de refresco, el agua embotellada). Los cuenta igual que un
artículo, en piezas, sin ver lo que el sistema tiene. En la vista previa del cierre ve, para cada
producto contado, lo que el sistema tenía, lo contado y la diferencia. Al cerrar, la existencia del
producto queda en lo contado (menos lo vendido mientras se contaba) y queda un ajuste ligado a ese
conteo.

**Why this priority**: es el hueco que deja cifras negativas y avisos de compra falsos hoy, en 34
productos de la empresa real.

**Independent Test**: con un producto de existencia propia en −2, abrir un conteo, capturar 15,
cerrar, y ver la existencia en 15 y un ajuste de +17 ligado al conteo.

**Acceptance Scenarios**:

1. **Given** un producto con existencia propia en −2, **When** se abre un conteo, se capturan 15 y
   se cierra, **Then** su existencia queda en 15 y hay un movimiento de ajuste de +17 ligado a ese
   conteo.
2. **Given** un conteo abierto, **When** se mira la lista, **Then** aparecen los productos activos
   con existencia propia, distinguibles de los artículos, y **no** aparecen los que restan por
   receta, paquete u opciones.
3. **Given** un producto capturado en 15 con el sistema en 10, **When** se venden 2 antes de cerrar,
   **Then** al cerrar queda en 13 (el ajuste es +5, contra lo que había al capturar).
4. **Given** un producto que no se contó, **When** se cierra, **Then** su existencia no cambia y la
   vista previa lo lista como «sin contar».
5. **Given** un producto capturado, **When** se quita la cifra («dejar sin contar»), **Then** vuelve
   a «sin contar» y no se ajusta.
6. **Given** un conteo abierto, **When** se recarga la pantalla o se cambia de tableta, **Then** las
   cifras de productos ya capturadas siguen ahí.
7. **Given** una cifra de producto con decimales o negativa, **When** se captura, **Then** se rechaza
   con un mensaje claro y no se guarda.

---

### User Story 2 - Quitarle el contador a un producto que restan sus opciones (Priority: P2)

Quien configura el menú abre la hoja de «Qué resta» de un producto que hoy cuenta su propia
existencia y que tiene un grupo de sabores donde cada sabor resta su artículo. Elige «Lo restan sus
opciones», sin agregar insumos. Al guardar, el contador propio del producto se cierra con el
movimiento de cambio de modo y, desde ese momento, vender el producto solo baja el artículo del
sabor elegido.

**Why this priority**: evita que el contador del producto siga bajando en paralelo a los artículos
de los sabores; es menos frecuente que el conteo (14 productos con opciones).

**Independent Test**: con un producto de existencia propia y un grupo obligatorio de sabores que
restan cada uno su artículo, guardarlo como «Lo restan sus opciones» sin insumos, vender uno con un
sabor y ver que solo baja el artículo de ese sabor.

**Acceptance Scenarios**:

1. **Given** un producto con existencia propia y un grupo obligatorio de sabores donde cada sabor
   resta su artículo, **When** se guarda como «Lo restan sus opciones» sin insumos, **Then** se
   guarda, el producto deja de contar existencia propia y su contador se cierra en cero con un
   movimiento de cambio de modo.
2. **Given** ese producto ya cambiado, **When** se vende uno con el sabor «Fresa», **Then** baja solo
   el artículo de Fresa y el producto no deja ningún movimiento propio.
3. **Given** un producto cuyas opciones no restan nada (o sin grupos), **When** se abre la hoja,
   **Then** «Lo restan sus opciones» no se ofrece; y si se manda de todos modos, el servidor lo
   rechaza diciendo que ninguna de sus opciones resta del almacén.
4. **Given** un producto que no tenía existencia propia, **When** se guarda como «Lo restan sus
   opciones», **Then** se guarda igual (confirmado, sin receta) y no se inserta ningún movimiento.
5. **Given** un producto guardado así, **When** se ve Recetas, **Then** se lee «Lo restan sus
   opciones», no «No se cuenta».

---

### Edge Cases

- **Venta durante la captura**: el ajuste es contra lo que había al capturar; lo vendido después ya
  restó (igual que los artículos).
- **Venta durante el cierre**: si una venta tiene tomada la existencia de un producto contado, el
  cierre responde «vuelve a intentar» en vez de esperar o perder la venta.
- **Producto que deja de tener existencia propia con el conteo abierto** (cambio de modo, o pasa a
  paquete): su cifra se ignora al cerrar y la vista previa lo dice; no revive el contador.
- **Producto archivado con el conteo abierto**: igual que un artículo archivado; si ya tenía cifra,
  se ajusta al cerrar; si no, no aparece.
- **Un producto y un artículo con el mismo nombre** (la lata como producto y como artículo): los
  dos aparecen y se distinguen; la pantalla no los junta.
- **Dos sucursales**: el conteo es de su sucursal; el ajuste toca solo la existencia del producto en
  esa sucursal.
- **Otra empresa**: un producto de otra empresa no se puede capturar en este conteo (no existe para
  esta sesión).
- **La versión anterior de la API durante la subida**: puede cerrar un conteo abierto sin ajustar
  las cifras de productos (no las conoce); los artículos se ajustan normal. Se acepta: dura segundos
  y la cifra se vuelve a capturar en el siguiente conteo.
- **«Lo restan sus opciones» con el producto en negativo**: el contador se cierra en cero; no se
  pasa nada a los artículos de los sabores (no se sabe de cuál fue cada pieza).
- **Opciones que dejan de restar después**: el producto se sigue leyendo según lo que es hoy; si
  ninguna opción resta, se lee como «No se cuenta», y Salud del inventario lo hace visible.

## Requirements *(mandatory)*

### Functional Requirements

**Conteo**

- **FR-001**: El conteo MUST listar, además de los artículos, los productos activos con existencia
  propia de la empresa, en su sucursal, distinguibles de los artículos: al final, agrupados por su
  categoría del menú, con un aviso arriba de la lista de cuántos productos hay.
- **FR-002**: El conteo MUST NOT listar productos que restan por receta, paquete u opciones, ni
  productos sin decidir o «No se cuenta».
- **FR-003**: La captura de un producto MUST aceptar solo piezas enteras de cero en adelante, dentro
  del máximo de piezas de la empresa, con el mismo aviso de cifra rara que los artículos en piezas.
- **FR-004**: El conteo MUST seguir siendo a ciegas: lo que el sistema tiene de un producto no viaja
  a la pantalla mientras el conteo está abierto.
- **FR-005**: Cada cifra de producto MUST guardarse al capturarla con lo que el sistema tenía en ese
  momento, y sobrevivir a recargar y a cambiar de tableta. Recapturar reemplaza; quitarla la vuelve
  «sin contar».
- **FR-006**: La vista previa y el cierre MUST incluir los productos contados (sistema, contado,
  diferencia) y los no contados, con el mismo criterio que los artículos.
- **FR-007**: Al cerrar, cada producto contado con diferencia MUST recibir un movimiento de ajuste
  por la diferencia contra lo capturado, ligado al conteo, con quien cerró y la sucursal del conteo.
- **FR-008**: Al cerrar, MUST guardarse para cada producto contado su costo en ese momento y el
  último movimiento que vio la captura, aunque esta entrega no los muestre.
- **FR-009**: Un producto que ya no tiene existencia propia al cerrar MUST NOT ajustarse, y la vista
  previa MUST decirlo.
- **FR-010**: Capturar, quitar, previsualizar y cerrar productos MUST exigir el mismo permiso que el
  conteo de artículos.

**Quitar el contador**

- **FR-011**: La hoja de «Qué resta» de un producto MUST ofrecer «Lo restan sus opciones» en el lugar
  de «No se cuenta» cuando el producto tiene al menos un grupo de opciones con alguna opción que
  resta algo del almacén; en los demás productos «No se cuenta» sigue igual.
- **FR-012**: Guardar «Lo restan sus opciones» MUST NOT exigir insumos, y el servidor MUST rechazarlo
  si ninguna opción del producto resta del almacén.
- **FR-013**: Si el producto tenía existencia propia, guardarlo así MUST cerrar su contador en cero
  en cada sucursal con el movimiento de cambio de modo existente y dejar el cambio en la bitácora.
- **FR-014**: Si ningún grupo que resta es obligatorio, la hoja MUST avisar que una venta sin elegir
  opción no resta nada.
- **FR-015**: Recetas MUST mostrar estos productos como «Lo restan sus opciones», distinto de «No
  se cuenta». Salud del inventario no los lista como problema: están confirmados y sus ventas dejan
  el movimiento del artículo de la opción (si una venta no restara nada, sale como hoy en «se vende
  sin restar»).
- **FR-016**: Aplica a cualquier empresa: nada en la regla depende de nombres de productos, grupos o
  categorías.
- **FR-017**: El resumen al cerrar MUST contar artículos y productos (corregidos y sin contar) con
  las mismas cifras que la vista previa.
- **FR-018**: Al elegir «Lo restan sus opciones» en un producto con existencia propia, la hoja MUST
  decir la cifra que tiene el sistema, que queda en cero al guardar, y que las piezas se cuentan por
  opción en el próximo conteo; el diálogo de confirmar lo repite.

### Key Entities

- **Cifra de producto en un conteo**: el producto, lo contado (piezas), lo que el sistema tenía al
  capturar, quién y cuándo; al cerrar, su costo y el último movimiento visto. Pertenece a un conteo
  de una sucursal y a una empresa.
- **Ajuste por conteo de un producto**: movimiento de almacén del producto, ligado al conteo.
- **Modo «Lo restan sus opciones»**: no se guarda como dato propio; se deduce de que el producto no
  tiene receta ni existencia propia, está confirmado, y alguna de sus opciones resta.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Un producto en −2 contado en 15 queda en 15 al cerrar, con un ajuste de +17 ligado al
  conteo (prueba automatizada).
- **SC-002**: Un producto con un grupo obligatorio de sabores pasa a «Lo restan sus opciones» sin
  insumos, y vender uno baja solo el artículo del sabor (prueba automatizada).
- **SC-003**: El 100 % de los productos activos con existencia propia de una empresa aparece en su
  conteo, y ninguno de los que restan por receta, paquete u opciones.
- **SC-004**: Una persona sin contexto encuentra y cuenta un producto en el conteo a 1024×600 sin
  ayuda, en la primera vuelta.
- **SC-005**: Ninguna cifra de producto de una empresa se puede leer, capturar ni ajustar desde otra.

## Assumptions

- La existencia propia de un producto se mide en piezas.
- La sucursal la resuelve la base como hoy; con dos sucursales y sin selector, el conteo responde
  igual que con artículos.
- Los productos no tienen «lugar» de conteo en esta entrega.
- Fuera de alcance: precio por pieza y por caja, variantes y unidades del giro (#64); recalcular el
  inventario pasado (lo corrige el conteo); el reporte de lo que se perdió para productos; volver a
  ofrecer la existencia propia como modo nuevo.
