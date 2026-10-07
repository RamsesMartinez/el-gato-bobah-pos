# Feature Specification: El almacén descuenta lo que de verdad se vendió

**Feature Branch**: `028-almacen-de-extras-y-plataformas`

**Created**: 2026-10-07

**Status**: Draft

**Input**: Decisiones del dueño del 2026-10-03 y 2026-10-07 ([docs/emparejamiento-de-plataformas.md](../../docs/emparejamiento-de-plataformas.md) §6):
que el almacén descuente los extras, los paquetes y los pedidos de Uber, con historial por producto,
ingrediente e insumo; que haya dónde capturar qué lleva cada cosa, con estimados de FUDO para no
empezar de cero. Tamaños y variantes que heredan quedan para la siguiente especificación.

## Contexto

Hoy el almacén solo descuenta el producto principal de cada venta de mostrador. Medido el
2026-10-07 sobre el catálogo real:

- **Ninguna opción activa tiene receta ni producto ligado.** La leche deslactosada o la perla extra
  que se cobran como extra no descuentan nada.
- **Los pedidos de plataforma aceptados no descuentan nada**, ni el platillo ni sus opciones.
- **Un combo no descuenta nada**: no tiene receta propia, y sus componentes no se recorren.
- **No existe pantalla para capturar recetas.** Las que hay vinieron de la importación de FUDO, y
  solo una parte de los productos tiene una.
- **FUDO tiene más de lo que se importó**: recetas de muchos más productos, el producto detrás de
  cada opción, insumos hechos de otros insumos (sub-recetas) y productos hechos de otros productos
  (paquetes). Vive fuera del repositorio (`~/gatobobah-datos/references/csv/`).

Con existencias que no bajan, el almacén no sirve para saber qué comprar ni cuánto costó lo vendido.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Capturar qué lleva cada producto y cada extra (Priority: P1)

Quien administra abre un producto o un extra y ve qué lleva: los insumos con su cantidad y unidad,
o el producto del catálogo que es (la Coca de un combo es la Coca que se vende sola). Puede
agregar, cambiar y quitar renglones. Lo que vino estimado de FUDO se ve marcado como **estimado**
hasta que alguien lo confirma.

**Why this priority**: sin esto nada de lo demás descuenta, porque no hay qué descontar.

**Independent Test**: capturar que «Leche deslactosada» lleva 250 ml de leche deslactosada; vender
un frappé con ese extra y ver bajar la leche.

**Acceptance Scenarios**:

1. **Given** un extra sin composición, **When** se le capturan dos insumos con cantidad, **Then**
   queda guardado y la próxima venta los descuenta.
2. **Given** un extra que es un producto del catálogo, **When** se liga a ese producto, **Then** la
   venta descuenta lo que ese producto lleva.
3. **Given** una composición estimada de FUDO, **When** se abre, **Then** dice que es estimada; al
   confirmarla deja de decirlo y guarda quién y cuándo.
4. **Given** la lista de productos y extras, **When** se filtra por «sin composición» o «estimado»,
   **Then** se ve lo que falta capturar o revisar.

---

### User Story 2 - El mostrador descuenta los extras (Priority: P1)

Al vender en mostrador, además del producto, cada extra elegido descuenta su composición,
multiplicada por la cantidad del renglón y la del extra.

**Why this priority**: es el defecto que motivó la especificación; los extras son una parte grande
de lo que se vende.

**Independent Test**: un frappé ×2 con «Perla extra» ×1 descuenta la receta del frappé dos veces y
la de la perla dos veces.

**Acceptance Scenarios**:

1. **Given** un extra con receta, **When** se vende en un renglón de cantidad 2, **Then** su receta
   se descuenta dos veces.
2. **Given** un extra ligado a un producto con existencias propias, **When** se vende, **Then** baja
   la existencia de ese producto.
3. **Given** un extra sin composición, **When** se vende, **Then** no descuenta nada y la venta no
   falla.
4. **Given** una venta cancelada antes de prepararse, **When** se cancela, **Then** sus extras
   regresan al almacén igual que el producto.

---

### User Story 3 - Los pedidos de plataforma descuentan (Priority: P1)

Al aceptar un pedido de Uber, el platillo emparejado descuenta lo que lleva su producto, y cada
opción emparejada descuenta lo que lleva su opción del POS. Lo que no tiene pareja no descuenta y
queda contado.

**Why this priority**: con la integración de Uber, una parte creciente de las ventas entra por ahí y
hoy no toca el almacén.

**Independent Test**: aceptar un pedido con un platillo y una opción emparejados y ver los dos
descuentos atribuidos a ese pedido.

**Acceptance Scenarios**:

1. **Given** un pedido aceptado con un platillo emparejado, **When** se acepta, **Then** se descuenta
   la composición del producto por la cantidad pedida.
2. **Given** una opción del pedido emparejada a una opción del POS, **When** se acepta, **Then** se
   descuenta la composición de esa opción.
3. **Given** un renglón sin pareja (va al producto genérico), **When** se acepta, **Then** no
   descuenta y el reporte del almacén lo cuenta como vendido sin descontar.
4. **Given** aceptar el pedido, **When** se descuenta, **Then** nadie decide nada: es el mismo toque
   de siempre.

---

### User Story 4 - Paquetes con nombre propio (Priority: P2)

Un producto puede ser un paquete de otros productos (frappé + crepa, crepa + papas), con cantidades.
Venderlo descuenta lo de cada componente, y el historial cuenta cada componente como vendido.

**Why this priority**: el negocio vende paquetes y quiere saber cuántas crepas salieron en total,
sueltas o en paquete; pero llega después de que los extras y Uber descuenten.

**Independent Test**: vender un paquete «Frappé + crepa» descuenta la receta del frappé y la de la
crepa, y el reporte por producto suma una crepa.

**Acceptance Scenarios**:

1. **Given** un paquete con dos componentes, **When** se vende, **Then** se descuenta la composición
   de cada componente.
2. **Given** el historial por producto, **When** se consulta, **Then** un componente vendido dentro
   de un paquete cuenta, distinguido de la venta suelta.
3. **Given** un paquete que contiene otro paquete, **When** se captura, **Then** se rechaza: un
   paquete solo contiene productos sueltos.

---

### User Story 5 - Insumos hechos de otros insumos (Priority: P2)

Un insumo puede prepararse con otros (un jarabe con azúcar y agua). Al descontarlo se descuenta lo
que lo compone, hasta los insumos que se compran.

**Why this priority**: sin esto, los insumos que se preparan en el local nunca bajan los que se
compran.

**Independent Test**: un frappé que lleva 30 ml de jarabe descuenta el azúcar y el agua del jarabe
en proporción.

**Acceptance Scenarios**:

1. **Given** un insumo compuesto, **When** se descuenta, **Then** se descuentan sus componentes en
   proporción.
2. **Given** una composición que se contiene a sí misma (directa o indirectamente), **When** se
   captura, **Then** se rechaza.

---

### User Story 6 - Promociones de plataforma (Priority: P3)

En un 2x1 o un producto de regalo de Uber, lo regalado descuenta del almacén y no suma ingreso, y
queda escrito quién financió la promoción (el negocio o la plataforma).

**Why this priority**: lo pidió el dueño, pero depende de confirmar con un pedido real cómo manda
Uber las promociones (hoy solo hay documentación).

**Independent Test**: un pedido de Uber con una promoción de producto gratis descuenta dos productos
y suma el ingreso de uno.

**Acceptance Scenarios**:

1. **Given** un pedido con un producto de regalo, **When** se acepta, **Then** el regalo descuenta
   almacén y no suma al total.
2. **Given** la promoción, **When** se registra, **Then** dice quién la financió.

---

### Edge Cases

- **Cantidades de extras**: un extra pedido ×2 en un renglón ×3 descuenta 6 veces.
- **Unidades distintas**: la receta dice 250 ml y el insumo se lleva en litros; la conversión usa la
  unidad del insumo y nunca redondea a cero una cantidad positiva.
- **Composición cambiada después de vender**: lo ya vendido no se recalcula; el movimiento guardado
  es el de ese momento.
- **Producto con existencias propias y además receta**: no se permite (ya lo impide el esquema).
- **Existencias negativas**: se permiten, como hoy; el almacén dice lo que falta, no bloquea la venta.
- **Devolución de un pedido de plataforma**: descuenta igual que hoy una devolución de mostrador.
- **Opción con receta y producto ligado a la vez**: no se permite (ya lo impide el esquema).
- **Estimado de FUDO para algo que ya tiene composición capturada**: no se pisa; gana lo capturado.
- **Dos opciones distintas que llevan el mismo insumo** (ranch de cortesía y ranch extra): cada una
  tiene su composición y las dos bajan el mismo insumo.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: Quien administra MUST poder capturar la composición de un producto o de una opción:
  insumos con cantidad y unidad, o el producto del catálogo que es.
- **FR-002**: Toda composición MUST guardar si es estimada o confirmada, y quién la confirmó y cuándo.
- **FR-003**: El sistema MUST ofrecer la carga inicial de estimados desde los archivos de FUDO, sin
  pisar lo ya capturado. Un estimado MUST descontar desde que se carga, marcado como estimado
  (decisión del dueño, 2026-10-07: un número aproximado sirve más que ninguno para saber qué
  comprar); la lista «por confirmar» dice qué revisar.
- **FR-004**: La venta de mostrador MUST descontar la composición de cada extra elegido, por la
  cantidad del extra y la del renglón.
- **FR-005**: Aceptar un pedido de plataforma MUST descontar la composición del producto emparejado
  y de cada opción emparejada; lo sin pareja MUST NOT descontar y MUST quedar contado.
- **FR-006**: Un producto MUST poder ser un paquete de otros productos con cantidades; venderlo MUST
  descontar cada componente, y un paquete MUST NOT contener otro paquete.
- **FR-007**: Un insumo MUST poder componerse de otros insumos; descontarlo MUST descontar sus
  componentes en proporción, y una composición circular MUST rechazarse al capturarse.
- **FR-008**: Todo movimiento de almacén de una venta MUST quedar atribuido a su pedido, su renglón y,
  si aplica, al extra o componente que lo originó.
- **FR-009**: El historial por producto MUST contar los componentes vendidos dentro de un paquete,
  distinguidos de la venta suelta.
- **FR-010**: Cancelar o devolver MUST regresar lo descontado por extras y componentes igual que lo
  del producto.
- **FR-011**: Nada de esto MUST pedir una decisión a quien opera al vender o aceptar.
- **FR-012**: La pantalla de composición MUST cumplir la restricción de tableta (44 px, sin
  selectores nativos, 1024×600).
- **FR-013**: (P3) Un producto regalado por una promoción de plataforma MUST descontar almacén, MUST
  NOT sumar ingreso y MUST registrar quién la financió.

### Key Entities

- **Composición**: qué lleva un producto o una opción. Renglones de insumo con cantidad y unidad, o
  un producto del catálogo. Estado: estimada o confirmada (quién, cuándo).
- **Paquete**: un producto cuyos renglones son otros productos con cantidad.
- **Insumo compuesto**: un insumo con su propia composición de insumos.
- **Movimiento de almacén**: lo descontado o devuelto, atribuido a pedido, renglón y origen (producto,
  extra o componente).

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Después de vender, las existencias de los insumos de extras y de componentes de
  paquetes bajan en la cantidad exacta de su composición (0 diferencias en las pruebas).
- **SC-002**: El 100% de los pedidos de plataforma aceptados con renglones emparejados genera sus
  movimientos de almacén.
- **SC-003**: Tras la carga de FUDO, quien administra ve en una lista qué falta capturar o confirmar,
  y la lista baja conforme confirma.
- **SC-004**: Vender y aceptar pedidos lleva el mismo número de toques que hoy.

## Assumptions

- Tamaños por producto, variantes que heredan de un «padre» y el ticket que dice lo agregado van en
  la siguiente especificación.
- Las opciones repetidas en grupos distintos («ranch de cortesía» y «ranch extra») no se fusionan
  aquí: cada una lleva su composición y comparten el insumo. La depuración de duplicados va con las
  variantes, ensayada sobre un respaldo de producción.
- Lo vendido antes de esta entrega no se descuenta hacia atrás.
- Las promociones (historia 6) se activan cuando un pedido real confirme cómo las manda Uber.
- Los archivos de FUDO no entran al repositorio; la carga los lee de fuera, como `make fudo-import`.
