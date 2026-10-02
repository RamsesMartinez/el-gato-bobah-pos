# Feature Specification: Sucursales en la base de datos

**Feature Branch**: `025-sucursales`

**Created**: 2026-10-01

**Status**: Draft

**Input**: User description: "Sucursales en la base de datos. Toda empresa nace con una sucursal y puede tener N (otra sucursal, una cocina oculta). No se ve en pantalla todavía: es estructura. Cruza la puerta «Más de una sucursal» del principio VIII. Motivo inmediato: cada tienda de plataforma pertenece a una sucursal, y el precio de plataforma se va a guardar por platillo de la plataforma, que vive en esa tienda. Debe decidir qué cuelga de sucursal hoy y qué se queda por empresa, cómo se rellenan los datos existentes, RLS por empresa sin cambios y que nada exija elegir sucursal en la pantalla mientras haya una sola."

## Contexto

El producto se va a vender a otros negocios y a cadenas, y el dueño quiere poder abrir otra
sucursal o una cocina oculta. Hoy la base solo conoce **empresas**: todo lo que pasa en un negocio
se le atribuye a la empresa entera. Decisión del dueño (2026-09-30, en
[docs/emparejamiento-de-plataformas.md](../../docs/emparejamiento-de-plataformas.md) §1): las
sucursales entran **en la base desde ahora, aunque no se vean**, y antes que el rediseño de la
pantalla de emparejar con Uber, porque una tienda de Uber pertenece a una sucursal.

**Por qué hoy y no después.** Mientras cada empresa tenga una sola sucursal, atribuir a ella lo
existente es trivial. El costo aparece el día que exista la segunda: desde ese momento, todo lo que
no haya guardado su sucursal queda ambiguo para siempre. Además, varias reglas del código asumen
«la» caja principal de la empresa; con dos sucursales, cada una tiene la suya.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Toda empresa tiene al menos una sucursal (Priority: P1)

Quien vende el sistema da de alta una empresa nueva, y esa empresa nace con su sucursal principal,
sin pasos extra. Las empresas que ya existen amanecen con la suya, y todo su pasado (pedidos,
cajas, turnos) queda atribuido a ella. Quien opera el POS no nota ningún cambio.

**Why this priority**: es la base de todo lo demás; sin ella no hay a qué colgar una tienda ni una
caja.

**Independent Test**: crear una empresa nueva y comprobar que tiene exactamente una sucursal
principal; aplicar el cambio sobre un respaldo real con dos empresas y comprobar que cada una tiene
la suya y que ningún dato quedó sin sucursal.

**Acceptance Scenarios**:

1. **Given** una empresa existente con pedidos, cajas y turnos, **When** se aplica el cambio,
   **Then** la empresa tiene una sucursal principal y todos sus pedidos, cajas y turnos le
   pertenecen.
2. **Given** la consola de plataforma, **When** se da de alta una empresa, **Then** nace con una
   sucursal principal, sin que nadie la capture.
3. **Given** una empresa con una sola sucursal, **When** alguien vende, cobra, abre o cierra caja,
   **Then** ninguna pantalla le pide elegir sucursal y todo funciona igual que antes.

---

### User Story 2 - Cada tienda de plataforma pertenece a una sucursal (Priority: P1)

Al conectar una tienda de Uber, queda ligada a una sucursal. Con una sola sucursal se liga sola.
Los pedidos que llegan de esa tienda quedan en esa sucursal.

**Why this priority**: es el motivo inmediato. El rediseño de emparejar guarda el precio por
platillo de la tienda, y la tienda tiene que saber de qué sucursal es.

**Independent Test**: conectar una tienda en una empresa con una sucursal y verificar que queda en
ella; recibir un pedido de esa tienda y verificar que el pedido queda en la misma sucursal.

**Acceptance Scenarios**:

1. **Given** una empresa con una sola sucursal, **When** se conecta una tienda de plataforma,
   **Then** la tienda queda en esa sucursal sin preguntar.
2. **Given** una tienda conectada antes de este cambio, **When** se aplica, **Then** queda en la
   sucursal principal de su empresa.
3. **Given** un pedido que llega de una tienda, **When** se registra, **Then** queda en la sucursal
   de esa tienda, no en otra.

---

### User Story 3 - Cajas, turnos y pedidos saben de qué sucursal son (Priority: P2)

Cada caja pertenece a una sucursal, y la caja principal es la principal **de su sucursal**, no de
la empresa. Cada pedido guarda en qué sucursal se hizo. El día que exista una segunda sucursal,
abrir caja allá no choca con la caja abierta de la primera, y un reporte puede separar lo que
vendió cada una.

**Why this priority**: con una sola sucursal no cambia nada visible, pero es el hecho que después
ya no se puede reconstruir.

**Independent Test**: en una base de prueba, crear una segunda sucursal con su propia caja
principal; abrir turno en las dos a la vez; vender en cada una y verificar que cada pedido queda en
la suya.

**Acceptance Scenarios**:

1. **Given** dos sucursales de la misma empresa, **When** cada una abre su caja principal,
   **Then** las dos quedan abiertas a la vez sin error.
2. **Given** dos sucursales, **When** se intenta marcar dos cajas principales en la misma sucursal,
   **Then** se rechaza.
3. **Given** un pedido hecho en una caja, **When** se registra, **Then** queda en la sucursal de esa
   caja.

---

### Edge Cases

- **Empresa sin sucursal**: no debe poder existir. Si una empresa llegara a quedar sin ninguna,
  el sistema lo detecta (prueba) en vez de vender sin sucursal.
- **Sucursal de otra empresa**: una caja, tienda o pedido nunca puede quedar ligado a una sucursal
  de otra empresa, aunque alguien envíe ese dato a mano. Los chequeos de llave foránea saltan el
  aislamiento por empresa, así que la barrera tiene que ser explícita y probada.
- **Aislamiento en los tres casos**: las sucursales de una empresa no son visibles desde otra, ni
  con una conexión reciclada, ni sin empresa en la sesión.
- **Borrar una sucursal con historia**: no se permite; una sucursal con pedidos o cajas se
  desactiva, no se borra.
- **La única sucursal**: no se puede desactivar ni borrar la última sucursal activa de una empresa.
- **Respaldo restaurado**: un respaldo anterior a este cambio, restaurado y migrado, termina en el
  mismo estado que producción (una sucursal por empresa, todo atribuido).
- **Pedido sin caja** (por ejemplo, uno que llega de plataforma): toma la sucursal de su tienda, no
  la de una caja.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: Toda empresa MUST tener al menos una sucursal, y exactamente una marcada como
  principal.
- **FR-002**: Al crear una empresa, el sistema MUST crear su sucursal principal en la misma
  operación; no puede existir un momento con empresa y sin sucursal.
- **FR-003**: Al aplicar el cambio, cada empresa existente MUST recibir su sucursal principal, y
  todas sus cajas, turnos, tiendas de plataforma y pedidos existentes MUST quedar atribuidos a ella.
- **FR-004**: Una sucursal MUST tener número consecutivo dentro de su empresa, código corto único
  dentro de la empresa e inmutable, nombre, estado (activa o no) y marca de matriz (exactamente una
  por empresa). MUST poder guardar dirección, teléfono, código postal y zona horaria; la zona
  horaria, si falta, es la de la empresa. La matriz nace con número 1, el nombre de la empresa y un
  código derivado de ella.
- **FR-005**: Cada caja MUST pertenecer a una sucursal; la regla de «una sola caja principal» MUST
  aplicar por sucursal, no por empresa.
- **FR-006**: Cada tienda de plataforma conectada MUST pertenecer a una sucursal. Con una sola
  sucursal activa, MUST asignarse sola.
- **FR-007**: Cada pedido MUST guardar la sucursal donde se hizo: la de su caja, o la de su tienda
  si vino de una plataforma.
- **FR-007b**: Las existencias y los movimientos de inventario MUST guardar su sucursal; los
  gastos MAY guardarla.
- **FR-007c**: Cuando el sistema necesite «la sucursal» y no haya selector, MUST usar la única
  sucursal activa; si hubiera más de una, MUST rechazar la operación con un error claro en vez de
  escoger una en silencio.
- **FR-008**: Ninguna pantalla MUST pedir elegir sucursal mientras la empresa tenga una sola
  activa. Esta entrega no agrega pantallas.
- **FR-009**: Una caja, tienda o pedido MUST NOT poder ligarse a una sucursal de otra empresa.
- **FR-010**: Las sucursales MUST quedar aisladas por empresa igual que el resto de los datos, y
  probadas en los tres casos (otra empresa, conexión reciclada, sin empresa).
- **FR-011**: Una sucursal con historia MUST NOT poder borrarse; la última sucursal activa de una
  empresa MUST NOT poder desactivarse.
- **FR-012**: Lo que hoy se queda por empresa (ver Alcance) MUST quedar documentado como
  pendiente, con lo que costaría pasarlo a sucursal, para que no se tome por resuelto.
- **FR-013**: El ticket impreso no cambia en esta entrega; los datos de la sucursal quedan
  disponibles para que mostrar nombre, código o dirección sea configuración, no esquema.

### Alcance: qué cuelga de sucursal y qué se queda por empresa

Decisión del dueño (2026-10-01): **todo lo que ocurre en un lugar es de la sucursal**; la empresa
se queda con lo que es legal o compartido. Es el modelo de SAP Business One, Square, Lightspeed y
Odoo (investigación con fuentes en [research.md](research.md)).

| De la sucursal | Por qué |
|---|---|
| Cajas, y por ellas turnos, conteos y movimientos de efectivo | La caja principal es una por sucursal |
| Pedidos | El hecho que no se recupera si no se guarda desde el principio |
| Tiendas de plataforma | Una tienda de Uber es un lugar; el precio por platillo vive en ella |
| Inventario (existencias y movimientos) | Las existencias están en un lugar físico |
| Datos del lugar: dirección, teléfono, código postal, zona horaria | Van en el ticket; el código postal es el «lugar de expedición» del CFDI de esa sucursal |
| Gastos (sucursal opcional) | Un gasto puede ser de un local o de toda la empresa |

| De la empresa | Por qué |
|---|---|
| RFC, razón social, régimen | Son de la persona fiscal, no del lugar |
| Catálogo maestro (productos, modificadores, categorías, recetas) | Compartido, como en todos los sistemas revisados. Disponibilidad y precio por sucursal se agregarán como excepciones sobre el maestro, sin tocar lo existente |
| Empleados y roles | Compartidos. Quién trabaja en qué sucursal se agrega después como asignación, sin tocar lo existente |
| Proveedores, credenciales de la app de plataformas | Son de la empresa |

**Matriz**: una sucursal por empresa lleva la marca de matriz. Es la de omisión mientras no haya
selector, la que nace con la empresa y la plantilla de las siguientes. La empresa sigue siendo la
persona fiscal.

**Cómo se identifica una sucursal**: un **número** consecutivo dentro de la empresa (1, 2, 3…), un
**código** corto (hasta 10 caracteres, único dentro de la empresa, que no cambia una vez creado
porque va en tickets y podría servir de serie fiscal) y un **nombre**. No hace falta slug: ninguna
dirección web la usa todavía.

**Folio de los pedidos**: no cambia en esta entrega. El número que lee cocina ya es por turno, y un
turno es de una caja, que es de una sucursal: con dos sucursales cada una numera por su lado sin
tocar nada. Un folio permanente por sucursal (tipo `CEN-000123`) se puede agregar después
calculándolo del historial, así que no es urgente.

### Key Entities

- **Sucursal**: un lugar donde la empresa vende (un local, una cocina oculta). Pertenece a una
  empresa; tiene número, código, nombre, si es la matriz, si está activa, y los datos del lugar.
- **Empresa**: sin cambios; ahora tiene una o más sucursales.
- **Caja**: pertenece a una sucursal; la principal lo es dentro de su sucursal.
- **Tienda de plataforma**: pertenece a una sucursal.
- **Pedido**: guarda su sucursal.
- **Existencia y movimiento de inventario**: guardan su sucursal.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Tras aplicar el cambio sobre un respaldo real con dos empresas, el 100% de las
  empresas tiene exactamente una sucursal principal y el 0% de cajas, tiendas y pedidos queda sin
  sucursal.
- **SC-002**: Quien opera el POS hace sus tareas de siempre (vender, cobrar, abrir y cerrar caja,
  recibir pedidos de plataforma) con el mismo número de toques que antes.
- **SC-003**: En una base de prueba con dos sucursales, las dos pueden tener su caja principal
  abierta a la vez y cada pedido queda en la sucursal correcta.
- **SC-004**: Ningún intento de ligar algo a una sucursal de otra empresa tiene éxito.
- **SC-005**: Las cifras de dinero (ventas, corte de caja) dan exactamente lo mismo antes y después
  del cambio sobre el mismo respaldo.

## Assumptions

- Esta entrega no agrega pantallas: dar de alta una segunda sucursal se hará después, en su propio
  cambio, cuando haya un caso real. Las pruebas la crean directo en la base.
- El aislamiento sigue siendo por empresa; dentro de una empresa, cualquier usuario ve todas sus
  sucursales (no hay permisos por sucursal todavía).
- Va sobre el trabajo de la integración con Uber (que ya tiene las tablas de tiendas y
  credenciales) y llega a producción junto con él.
- Las credenciales de la app de Uber se quedan **por empresa**: son de la cuenta de desarrollador,
  no de la tienda.
