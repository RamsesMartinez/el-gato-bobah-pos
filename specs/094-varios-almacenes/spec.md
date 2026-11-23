# Feature Specification: Varios almacenes por sucursal

**Feature Branch**: `094-varios-almacenes`

**Created**: 2026-11-23

**Status**: Draft

**Input**: Issue #65 — requerimiento del 2026-10-18 y actualización del 2026-11-23. Un negocio guarda
mercancía suya en más de un lugar físico (el local y una bodega aparte) y necesita saber cuánto hay
en cada uno, moverla entre ellos y que las ventas descuenten solo del almacén desde el que se vende.

## Contexto

Hoy cada sucursal tiene **un solo** montón de existencias: las ventas descuentan de él, las compras
entran a él, el conteo lo corrige y la merma lo baja. Los «lugares» del conteo solo ordenan el
recorrido; no separan existencias. Cuando la leche está en la bodega y no en el local, el sistema la
cuenta como disponible para vender y el conteo del local la encuentra «de menos».

Esta es la puerta del principio VIII «Más de un almacén por sucursal». Se cruza con una sola regla:
**todo lo que hoy guarda existencias de la sucursal pasa a guardarlas de un almacén**, y lo existente
se asigna al almacén principal de su sucursal sin que cambie ningún total.

### Entregas

El alcance no cabe en una subida. Va en este orden, y cada entrega sube a producción por su cuenta:

| # | Entrega | Lo que el operador ve al terminar |
| --- | --- | --- |
| E1 | Almacenes y almacén en cada registro | Negocio › Inventario › Almacenes: el principal («Local») y alta de otros. Todo lo existente queda en el principal; ningún total cambia |
| E2 | Ventas | Las ventas (mostrador, plataformas, pedido web) descuentan del almacén de venta; Existencias con selector de almacén y «Todos» |
| E3 | Compras con destino | Cada renglón de compra elige a qué almacén entra |
| E4 | Traspasos | Mover mercancía entre almacenes de la misma sucursal |
| E5 | Conteo y merma | Contar un almacén; la merma y el «se acabó» dicen de qué almacén; mínimos por almacén |
| E6 | Reportes | Existencias y valor por almacén y total; Movimientos con su almacén y su traspaso |

La columna de almacén entra **opcional** en E1 (llenada con el principal) y pasa a **obligatoria** en
una subida posterior, cuando ninguna versión en producción escriba sin almacén.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Dar de alta la bodega sin que cambie nada de lo que hay (Priority: P1)

Quien administra el almacén abre Almacenes, ve que su sucursal ya tiene un almacén principal
(«Local», el de venta) con todo lo que existía, y da de alta «Bodega», que nace vacía.

**Why this priority**: es la base de todo lo demás y la única entrega que toca datos existentes. Si
el reparto inicial cambia un total, el negocio pierde la confianza en su inventario.

**Independent Test**: sobre una copia de producción con dos empresas, después de la migración cada
existencia, movimiento, conteo, merma, mínimo y renglón de compra tiene almacén, y la suma por
sucursal y artículo es idéntica a la de antes.

**Acceptance Scenarios**:

1. **Given** una sucursal sin almacenes, **When** se sube esta entrega, **Then** la sucursal tiene un
   almacén principal llamado «Local» y todo lo existente está en él.
2. **Given** la copia de producción, **When** corre la migración, **Then** ningún movimiento,
   existencia, conteo, merma, mínimo ni renglón de compra queda sin almacén y los totales por
   sucursal y artículo no cambian.
3. **Given** «Local», **When** se da de alta «Bodega», **Then** existe con existencias en cero y no es
   el principal.
4. **Given** el principal, **When** alguien intenta archivarlo, **Then** se niega.
5. **Given** «Bodega» con 3 kg de azúcar, **When** alguien intenta archivarla, **Then** se niega
   diciendo que tiene existencias.
6. **Given** la versión anterior de la API corriendo durante la subida, **When** registra una venta,
   compra, conteo o merma, **Then** el registro queda en el almacén principal y no falla.

---

### User Story 2 - La venta descuenta solo del almacén de venta (Priority: P1)

El cajero vende un frappé con leche. La leche sale del almacén de venta, aunque haya más en la
bodega. Si el local queda en negativo y la bodega tiene, el sistema sugiere traspasar.

**Why this priority**: es la razón del issue: saber cuánto hay en el local, donde se vende.

**Independent Test**: con «Bodega» en 20 L y «Local» en 0, vender un frappé deja «Local» en negativo,
«Bodega» igual, y la existencia muestra la sugerencia de traspaso.

**Acceptance Scenarios**:

1. **Given** «Bodega» 20 L y «Local» 0 L, **When** se vende un frappé con leche, **Then** baja «Local»
   (queda en negativo), «Bodega» no cambia y se sugiere traspasar desde «Bodega».
2. **Given** un pedido de plataforma o del menú en línea, **When** se acepta, **Then** descuenta del
   almacén de venta igual que el mostrador.
3. **Given** un pedido cancelado que repone, **When** se cancela, **Then** repone al almacén del que
   salió, aunque después se haya cambiado cuál es el de venta.
4. **Given** un producto con existencia propia, **When** se pregunta si hay existencia para venderlo
   en un lugar de venta, **Then** la respuesta mide el almacén de venta, no la suma de todos.

---

### User Story 3 - Ver las existencias de cada almacén (Priority: P1)

Quien revisa el inventario abre Existencias y ve por omisión el almacén de venta; puede elegir otro
almacén o «Todos», que suma.

**Why this priority**: sin esto el dato existe pero nadie lo ve; va en la misma entrega que ventas.

**Independent Test**: con dos almacenes con cifras distintas, el selector muestra cada uno y «Todos»
muestra la suma.

**Acceptance Scenarios**:

1. **Given** dos almacenes, **When** se abre Existencias, **Then** muestra el de venta y un selector.
2. **Given** «Todos», **When** se elige, **Then** cada artículo muestra la suma de sus almacenes.
3. **Given** una sucursal con un solo almacén, **When** se abre Existencias, **Then** no aparece el
   selector (la pantalla queda como hoy).

---

### User Story 4 - Repartir una compra entre almacenes (Priority: P2)

Quien recibe una compra elige, por renglón, a qué almacén entra. Por omisión el principal; la
pantalla recuerda el último usado.

**Why this priority**: es la entrada de mercancía a la bodega; sin ella la bodega solo se llena por
traspaso.

**Independent Test**: un ticket de 8 renglones con 2 a «Bodega» y 1 excluido sube «Local» por 5 y
«Bodega» por 2; el excluido no entra a ninguno.

**Acceptance Scenarios**:

1. **Given** un ticket con 8 renglones, 2 destinados a «Bodega» y 1 excluido, **When** se guarda,
   **Then** «Local» sube por 5 renglones, «Bodega» por 2 y el excluido no entra a ninguno.
2. **Given** un renglón ya recibido en «Local», **When** se le cambia el destino a «Bodega», **Then**
   sale de «Local» lo que sumó y entra a «Bodega» (por diferencia, sin editar movimientos).
3. **Given** la última compra con destino «Bodega», **When** se abre otra, **Then** los renglones
   nuevos proponen «Bodega».

---

### User Story 5 - Traspasar de la bodega al local (Priority: P2)

Quien opera el almacén registra un traspaso: origen, destino, renglones (artículo y cantidad, en
unidad base o en una presentación) y nota. En pocos toques desde la tableta o el teléfono.

**Why this priority**: es lo que permite que el local tenga lo que vende sin descontar «de cualquier
lado».

**Independent Test**: «Local» 10 L y «Bodega» 20 L; traspasar 6 L de «Bodega» a «Local» deja 16 y 14
con dos movimientos ligados.

**Acceptance Scenarios**:

1. **Given** «Local» 10 L y «Bodega» 20 L, **When** se traspasan 6 L de «Bodega» a «Local», **Then**
   quedan 16 y 14 y existen dos movimientos ligados al mismo traspaso.
2. **Given** un traspaso mayor a lo que hay en el origen, **When** se manda, **Then** avisa y pide
   confirmar; confirmado, el origen queda en negativo.
3. **Given** un traspaso enviado dos veces por un reintento de red, **When** llega el segundo,
   **Then** no se duplica.
4. **Given** un traspaso guardado, **When** se quiere corregir, **Then** no se edita: se registra otro
   en sentido contrario.
5. **Given** origen y destino iguales, o de otra sucursal, **When** se manda, **Then** se rechaza.

---

### User Story 6 - Contar un almacén y registrar la merma donde ocurrió (Priority: P2)

Al empezar un conteo se elige el almacén (por omisión el de venta); al cerrar se corrige solo ese.
La merma, el consumo del personal, las cortesías y el «se acabó» dicen de qué almacén. Los mínimos y
la salud del inventario son por almacén.

**Why this priority**: sin conteo por almacén, el conteo del local «encuentra de menos» lo que está en
la bodega.

**Independent Test**: un conteo de «Bodega» con diferencias, al cerrar, no cambia «Local».

**Acceptance Scenarios**:

1. **Given** un conteo de «Bodega» con diferencias, **When** se cierra, **Then** «Local» no cambia.
2. **Given** un conteo abierto de «Local», **When** se intenta abrir otro de «Local», **Then** se
   niega como hoy; uno de «Bodega» sí se puede abrir.
3. **Given** una merma capturada desde el local, **When** se guarda, **Then** baja el almacén de
   venta salvo que se elija otro.
4. **Given** un mínimo de 5 L de leche en «Local» y ninguno en «Bodega», **When** «Local» baja de 5,
   **Then** el aviso dice que falta leche en «Local».
5. **Given** un «se acabó» reportado desde la tableta, **When** se guarda, **Then** queda en el
   almacén de venta.

---

### User Story 7 - Reportes y movimientos por almacén (Priority: P3)

Quien revisa el negocio ve existencias y valor por almacén y el total; Movimientos dice de qué
almacén es cada uno y, si vino de un traspaso, cuál.

**Why this priority**: es lectura sobre datos que ya existen tras las entregas anteriores.

**Independent Test**: con mercancía en dos almacenes, el valor por almacén suma el total, y un
movimiento de traspaso muestra su contraparte.

**Acceptance Scenarios**:

1. **Given** dos almacenes con mercancía, **When** se abre el reporte de existencias, **Then** muestra
   valor por almacén y el total, y la suma de los almacenes es el total.
2. **Given** un movimiento de traspaso, **When** se ve en Movimientos, **Then** dice su almacén y el
   traspaso que lo originó.
3. **Given** el costo de ventas de un periodo, **When** se calcula, **Then** sale de las salidas por
   venta (del almacén de venta); los traspasos no son costo.

---

### User Story 8 - El agente de operación trabaja por almacén (Priority: P2)

El agente que usa el MCP de inventario puede indicar el almacén en cada herramienta de existencias,
movimientos, ajustes, conteo, compras, merma y mínimos. Si no lo indica, usa el de venta, y la vista
previa dice a qué almacén toca.

**Why this priority**: el agente ya opera el inventario; si su herramienta supone un solo almacén,
vuelve a perder el almacén por ahí.

**Independent Test**: un ajuste sin almacén toca el de venta y la vista previa lo nombra; con el
almacén de otra empresa se rechaza.

**Acceptance Scenarios**:

1. **Given** una herramienta de inventario sin almacén, **When** se llama, **Then** usa el de venta y
   la vista previa lo nombra.
2. **Given** un almacén archivado o de otra empresa, **When** se manda, **Then** se rechaza como si no
   existiera.

### Edge Cases

- **Sucursal con un solo almacén**: toda pantalla se ve como hoy (sin selector), y todo cae al
  principal.
- **Almacén archivado con un conteo abierto**: archivar se niega mientras tenga un conteo abierto.
- **Cambiar cuál es el de venta**: lo vendido antes se queda en el almacén del que salió; las
  cancelaciones reponen al almacén del movimiento original, no al de venta de hoy.
- **Unir insumos**: las existencias se suman por almacén (el origen en «Bodega» suma al destino en
  «Bodega»).
- **Cambio de unidad de un artículo**: la existencia de cada almacén se convierte por separado.
- **Productos con existencia propia** (refrescos, latas): tienen existencia por almacén igual que los
  insumos.
- **Compra recibida con la versión anterior durante la subida**: entra al principal.
- **Traspaso con un artículo archivado o unido**: el archivado se rechaza; el unido se redirige al
  destino de la unión, como hoy cualquier movimiento.
- **Traspaso a un almacén archivado**: se rechaza.
- **Un lugar de conteo** pertenece a un almacén; al contar un almacén solo aparecen sus lugares.
- **Dos sucursales**: un almacén nunca se usa desde otra sucursal (ni en compra, traspaso, conteo ni
  merma).

## Requirements *(mandatory)*

### Functional Requirements

**Almacenes (E1)**

- **FR-001**: Cada sucursal MUST tener exactamente un almacén principal (el de venta), activo.
- **FR-002**: Quien tenga el permiso de administrar almacenes MUST poder dar de alta, renombrar,
  archivar y reactivar almacenes de su sucursal, y elegir cuál es el principal entre los activos
  (esto último desde la entrega E3: antes, las llaves de existencia por sucursal lo impiden; research R2).
- **FR-003**: El sistema MUST negar archivar el principal, un almacén con alguna existencia distinta
  de cero o uno con un conteo abierto.
- **FR-004**: Los nombres de almacén MUST ser únicos dentro de la sucursal sin distinguir mayúsculas,
  acentos ni espacios.
- **FR-005**: La migración MUST asignar todo lo existente (existencias, movimientos, conteos y sus
  renglones, mermas, mínimos, reportes de «se acabó», renglones de compra) al principal de su
  sucursal, sin cambiar ningún total por sucursal y artículo; los almacenes nuevos nacen vacíos.
- **FR-006**: Durante la subida, lo que escriba la versión anterior sin almacén MUST quedar en el
  principal de su sucursal sin fallar.
- **FR-007**: El almacén MUST pasar a ser obligatorio en todos esos registros en una subida posterior
  a la que deja de escribirlos sin almacén.
- **FR-008**: Se MUST registrar en la bitácora `warehouse.created`, `warehouse.updated` y
  `warehouse.archived`.

**Ventas (E2)**

- **FR-010**: Toda venta (mostrador, plataformas, menú en línea) MUST descontar del almacén de venta
  de su sucursal, y nunca de otro automáticamente.
- **FR-011**: Una cancelación que repone MUST reponer al almacén del movimiento que repone.
- **FR-012**: «¿Hay existencia de este producto en este lugar de venta?» MUST medir el almacén de
  venta; es una sola regla compartida con el #326.
- **FR-013**: Cuando el almacén de venta tiene una existencia negativa y otro almacén de la sucursal
  tiene de ese artículo, Existencias MUST sugerir el traspaso.
- **FR-014**: Existencias MUST mostrar por omisión el almacén de venta, un selector de almacén y la
  vista «Todos» que suma; con un solo almacén el selector no aparece.

**Compras (E3)**

- **FR-020**: Cada renglón de compra MUST llevar almacén de destino, por omisión el principal, y la
  pantalla MUST proponer el último usado por esa persona en ese equipo.
- **FR-021**: Un renglón excluido MUST no entrar a ningún almacén.
- **FR-022**: Cambiar el destino de un renglón ya recibido MUST mover la mercancía por diferencia
  (salida del almacén anterior y entrada al nuevo), sin editar movimientos.

**Traspasos (E4)**

- **FR-030**: Quien tenga el permiso de traspasar MUST poder registrar un traspaso entre dos almacenes
  activos distintos de la misma sucursal, con fecha, usuario, nota y renglones (artículo, cantidad y,
  opcional, presentación).
- **FR-031**: Un traspaso MUST generar por renglón una salida del origen y una entrada al destino
  ligadas al traspaso, valoradas al costo vigente del artículo.
- **FR-032**: Un traspaso MUST ser idempotente por un identificador que manda el cliente, y no MUST
  poder editarse ni borrarse; se corrige con otro.
- **FR-033**: Un traspaso que deja el origen en negativo MUST pedir confirmación explícita.
- **FR-034**: Se MUST registrar `transfer.created` en la bitácora.

**Conteo, merma y avisos (E5)**

- **FR-040**: Un conteo MUST ser de un almacén (por omisión el de venta) y al cerrar MUST corregir solo
  ese almacén. Puede haber un conteo abierto por almacén.
- **FR-041**: Los lugares de conteo MUST pertenecer a un almacén; al contar, solo aparecen los del
  almacén contado.
  Cada artículo tiene un solo lugar (de cualquier almacén): asignarle uno de otro almacén lo pasa a
  ese recorrido, y la ficha lo confirma antes. El conteo de un almacén que no es el de venta enseña lo
  que tiene existencia o lugar ahí; lo demás se encuentra buscando.
- **FR-042**: La merma (incluidos consumo del personal y cortesías) MUST llevar almacén, por omisión el
  de venta.
- **FR-043**: Los mínimos, la lista de compra, el aviso de stock bajo y el «se acabó» MUST ser por
  almacén y decir de cuál.
  Encender o apagar las alertas (al mínimo, al acabarse, la franja) es del artículo y vale en todos
  sus almacenes; el mínimo es de cada almacén, y un almacén que no es el de venta solo avisa con su
  mínimo propio.
- **FR-044**: La salud del inventario MUST medir por almacén.
- **FR-045**: Unir insumos MUST sumar existencias por almacén.

**Reportes y movimientos (E6)**

- **FR-050**: El reporte de existencias MUST mostrar cantidad y valor por almacén y el total.
- **FR-051**: Movimientos MUST mostrar el almacén de cada movimiento y, si viene de un traspaso, cuál.
- **FR-052**: El costo de ventas MUST salir solo de las salidas por venta; los traspasos no cuentan.

**Permisos y agente**

- **FR-060**: Se MUST agregar los permisos «Administrar almacenes» y «Traspasar entre almacenes»,
  heredados por los roles sugeridos que hoy configuran o cuentan el almacén.
- **FR-061**: Las herramientas del MCP de inventario MUST aceptar un almacén opcional (por omisión el
  de venta) y su vista previa MUST nombrarlo; las que no lo reciban se documentan.
- **FR-062**: Un almacén de otra empresa o de otra sucursal MUST rechazarse como si no existiera, en
  todas las rutas.

### Key Entities

- **Almacén**: lugar físico de una sucursal donde hay mercancía del negocio. Nombre, sucursal, si es
  el de venta, activo o archivado.
- **Existencia**: cantidad de un artículo (insumo o producto con existencia propia) en un almacén.
- **Movimiento**: cambio en la existencia de un almacén; guarda su almacén y, si aplica, el traspaso.
- **Traspaso**: documento que mueve mercancía de un almacén a otro de la misma sucursal; origen,
  destino, fecha, usuario, nota, identificador del cliente y renglones.
- **Conteo / Merma / Mínimo / Aviso de «se acabó» / Renglón de compra**: cada uno ligado a un almacén.
- **Lugar de conteo**: orden del recorrido dentro de un almacén.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Tras la migración sobre una copia real con dos empresas, 0 registros sin almacén y 0
  diferencias en la existencia total por sucursal y artículo.
- **SC-002**: Ninguna petición de la versión anterior falla durante la subida de cada entrega.
- **SC-003**: Un traspaso de un artículo se registra en 6 toques o menos desde la pantalla.
- **SC-004**: En una sucursal con un solo almacén, ninguna pantalla agrega un control nuevo.
- **SC-005**: Una persona nueva de una fonda da de alta una bodega, traspasa y cuenta la bodega sin
  ayuda en la prueba a la resolución del equipo de mostrador.
- **SC-006**: Ninguna consulta de existencias devuelve almacenes de otra empresa (otra empresa,
  conexión reciclada y sin empresa).

## Assumptions

- Todos los almacenes de una sucursal comparten el catálogo de artículos; cambia solo la existencia.
- El costo vigente es del artículo, no del almacén: un traspaso no cambia el costo.
- El conteo de un almacén secundario es opcional; nada obliga a contar la bodega.
- El permiso de contar (`inventory.count`) sirve para cualquier almacén de la sucursal; permisos por
  almacén quedan fuera (agregarlos después cuesta una tabla que se llena de cero, sin hecho perdido).
- El almacén de venta es uno por sucursal; si un lugar de venta (la web) vendiera de otro almacén
  sería configuración de ese lugar de venta, no parte de este spec.
- «Recordar el último destino» es por equipo (la pantalla), no un dato del servidor.
- Fuera de alcance: traspasos entre sucursales, órdenes de compra, caducidad por lote, contabilidad.
