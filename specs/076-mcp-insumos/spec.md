# Feature Specification: Crear y editar insumos desde el servidor MCP, y empaques por insumo

**Feature Branch**: `076-mcp-insumos`

**Created**: 2026-11-11

**Status**: Draft

**Input**: Issue #271. Seguimiento de #268 (empaques por MCP, ya en producción). Al cargar compras
reales por MCP el 11-nov salieron dos huecos: el agente puede crear y editar el empaque pero no el
insumo al que pertenece, y la lista de empaques no se puede pedir de un solo insumo.

## Contexto

La 072 abrió la puerta para agentes (tokens personales, alcance y permiso leídos en cada llamada,
vista previa por omisión, clave de idempotencia, bitácora por llamada), la 073 le agregó compras con
mercancía y la 074 empaques de compra y la corrección de gastos. Cada herramienta despacha a la misma
ruta que usa la pantalla y hereda sus reglas. Esta feature agrega dos escrituras y un filtro sobre
esa misma puerta, sin reglas propias.

Los dos casos reales del issue:

- **Renombrar**: el insumo 749 se llama «Jamón de pierna» y se compra jamón de pavo (Bafar) y de
  pierna (Kir, Member's Mark). Debe llamarse «Jamón», con las marcas en los empaques. Hoy solo se
  puede en la pantalla.
- **Alta**: cuando el ticket trae un insumo que no existe, el agente no puede darlo de alta y la
  captura de la compra se corta para ir a la tableta.

Y un tercero, de lectura: revisar los empaques de un insumo obliga a leer los de todos (más de 30
hoy) y filtrar a mano.

## Respuesta a la exclusión «configurar insumos» de la 072

| Exclusión | Dónde | Respuesta para este caso acotado |
| --- | --- | --- |
| **Configurar insumos**: «la configuración de insumos llega en ráfagas y es de pantalla» | 072, tabla «Qué herramientas entran», Inventario (y el punto 9: «Alta y corrección de insumos (costo, densidad, empaques): en ráfagas de configuración») | Lo que llega en ráfagas es la configuración pesada: cambiar la unidad base, corregir costos, densidades, composición, unir duplicados, archivar. Eso sigue fuera. Dar de alta **un** insumo no llega en ráfaga: aparece uno a la vez, con el ticket enfrente, en medio de una compra; mandarlo a la pantalla corta la captura justo cuando se sabe qué es. Renombrar y ajustar la ficha (categoría, ubicación, mínimo, otros nombres) es igual de puntual. Entra solo eso: el alta con los campos de la pantalla de alta y la edición de los campos que la ficha edita en su autoguardado y en «otros nombres» |
| **Dar de alta artículos desde la compra** | 072 (mercancía) y 073 (D3, FR-005) | La 073 exigió que el agente no eligiera el insumo: aquí tampoco elige. Crear un insumo es una escritura aparte con su propia vista previa, y esa vista previa pone primero los insumos parecidos que ya existen, con la misma búsqueda que la hoja «Nuevo artículo» |
| **Unir insumos** | 072, Inventario | Sigue fuera. El riesgo que unir resuelve —duplicados— es justo el que esta feature puede crear; por eso la vista previa del alta avisa antes, y unir sigue en la pantalla, que muestra lo que se mueve |

## Decisiones tomadas en este spec (recomendación, sin preguntar)

| # | Duda | Decisión | Por qué |
| --- | --- | --- | --- |
| D1 | ¿A qué alcance van las dos escrituras? | **Almacén** (`write:inventory`), con el permiso de configurar el almacén | Un insumo es configuración del almacén, igual que sus empaques (074, D1). Menú y precios es lo que se vende; Compras es lo que toca dinero y almacén a la vez al capturar |
| D2 | ¿Alcances nuevos o migración? | **Ninguno** | Cabe en un alcance que existe. **Consecuencia que no se ve**: ensancha los tokens ya emitidos. Después de la subida, un token con almacén puede crear y editar insumos si el rol de su dueño tiene el permiso de configurar el almacén. No es más de lo que esa persona ya puede en la pantalla. Medido en el respaldo de producción del 2026-11-11 03:20: el único token de la empresa («Token Gestion Claude», del administrador) ya trae almacén, así que cubre las herramientas nuevas sin regenerarlo; se comprueba con `whoami` contra la API local. Se dice en el texto del alcance en Integraciones y en el comentario final del issue |
| D3 | ¿Cómo se avisa del duplicado al dar de alta? | La vista previa pone **primero**, antes que el resumen de lo que se va a crear, «quizá ya existe» con cada insumo parecido (nombre e id). Los parecidos salen de la búsqueda de la hoja «Nuevo artículo» (nombres parecidos) **más** la búsqueda de la lista de Almacén por el nombre (que encuentra también por otros nombres). Aplicar no se bloquea por un parecido | Es el comportamiento de la pantalla: pregunta «¿Es alguno de estos?» y deja «Crear de todos modos». El nombre exactamente repetido (activo o archivado, por nombre o por otro nombre) lo rechaza la ruta al aplicar y la herramienta lo dice con el id del que ya existe. Comparar nombres en la capa del agente sería una segunda regla |
| D4 | ¿Qué campos lleva el alta? | Los de la hoja «Nuevo artículo» —nombre, unidad base, existencia inicial, costo inicial, ubicación y mínimo— más la categoría, que la ruta acepta y la ficha pide justo después | La categoría se edita en la ficha en el mismo minuto; dejarla fuera obligaría a una segunda escritura por cada alta. Cantidades y costo en la unidad base, como viajan a la ruta |
| D5 | ¿Existencia inicial al dar de alta antes de una compra? | Se permite (la pantalla la tiene) y la vista previa avisa: si el insumo se crea para capturar una compra, la existencia y el costo llegan con la compra y no van en el alta | Mandarlos en los dos lados duplica la existencia. No se prohíbe: la pantalla no lo prohíbe y hay altas sin compra (un insumo que ya estaba en el almacén) |
| D6 | ¿`ingredient_update` exige mandar todo? | No: cambia solo lo que el agente manda. Para quitar categoría, ubicación o mínimo se pide explícito (`clear_*`); los otros nombres se mandan como **lista completa** y una lista vacía los quita todos | Es lo que hace la ficha: autoguarda un campo a la vez, y la ruta de otros nombres reemplaza la lista completa. Un valor ausente nunca borra nada |
| D7 | ¿Nombre y otros nombres en una llamada? | Sí: primero el nombre y luego los otros nombres, en ese orden | Es el caso del issue: «Jamón de pierna» pasa a «Jamón» y puede quedarse como otro nombre. Al revés, la ruta descarta un otro nombre igual al nombre vigente. Si el segundo paso falla, el primero queda aplicado y el error lo dice |
| D8 | ¿Qué avisa la vista previa de la edición? | El antes y después de cada campo; si cambia el nombre, los insumos parecidos al nombre nuevo (las dos búsquedas de D3, sin el propio); por cada otro nombre nuevo, los parecidos que ya usa la ficha | El choque exacto lo rechaza la ruta al aplicar y nombra al dueño del nombre |
| D9 | ¿`presentations_list` con `ingredient_id`? | Opcional; con él lee la ruta de empaques de un insumo, sin él responde igual que hoy | Es la ruta que ya usa la ficha. Un insumo de otra empresa responde vacío o «no existe», como la ruta |

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Dar de alta el insumo que trae el ticket (Priority: P1)

El dueño dicta una compra y un renglón es «Queso cotija», que el almacén no tiene. El agente pide la
vista previa de `ingredient_create` con «Queso cotija» en gramos: la vista previa dice primero
«quizá ya existe: Queso Cotija rallado (id …)». El dueño dice que no es el mismo; el agente aplica,
crea el empaque con `presentation_create` y captura la compra con el insumo nuevo.

**Why this priority**: Es lo que hoy corta la captura de una compra; sin esto el resto de la compra
por MCP se detiene en el primer insumo nuevo.

**Independent Test**: Contra la API local con el respaldo restaurado: vista previa de un nombre que
se parece a uno existente (lo señala); alta de un nombre nuevo; empaque y vista previa de compra con
él.

**Acceptance Scenarios**:

1. **Given** un insumo que ya existe con un nombre parecido, **When** se pide la vista previa del
   alta, **Then** lo primero que dice es «quizá ya existe» con su nombre e id, y no se crea nada.
2. **Given** un insumo que ya existe con un otro nombre igual o parecido al pedido, **When** se pide
   la vista previa, **Then** también lo señala.
3. **Given** la vista previa, **When** se aplica con una clave nueva, **Then** el insumo queda creado
   con su unidad base y los campos mandados, la bitácora registra la llamada con el después, y
   repetir la llamada con la misma clave no crea otro.
4. **Given** un nombre que ya tiene otro insumo (activo o archivado, por nombre o por otro nombre),
   **When** se aplica, **Then** se rechaza con el id del que ya existe y no se crea nada.
5. **Given** una unidad que no es base (kilo, litro), **When** se aplica, **Then** se rechaza como en
   la pantalla: lo que llega en kilos se captura como empaque.
6. **Given** el insumo recién creado, **When** se crea su empaque y se pide la vista previa de una
   compra con él, **Then** la compra lo acepta como cualquier otro insumo.

---

### User Story 2 - Renombrar un insumo y ajustar su ficha (Priority: P1)

«El 749 se llama Jamón de pierna; ponle Jamón y deja Jamón de pierna y Jamón de pavo como otros
nombres». El agente pide la vista previa: antes y después del nombre y de los otros nombres, y los
insumos parecidos a «Jamón» si los hay. Aplica, y el insumo se llama «Jamón» con sus otros nombres;
sus existencias, costo y empaques no cambian.

**Why this priority**: Es el caso real del issue y hoy solo se puede en la tableta.

**Independent Test**: Renombrar 749 en la base restaurada, con sus otros nombres; comprobar que el
buscador de compras lo encuentra por los tres nombres y que nada más de la ficha cambió.

**Acceptance Scenarios**:

1. **Given** un insumo, **When** se pide la vista previa de cambiarle el nombre, **Then** dice el
   antes y el después y los insumos con nombre parecido al nuevo (sin el propio), y no cambia nada.
2. **Given** la vista previa, **When** se aplica, **Then** cambia el nombre y nada más de la ficha;
   la bitácora registra antes y después.
3. **Given** nombre nuevo y otros nombres en la misma llamada, **When** se aplica, **Then** quedan los
   dos, y el nombre viejo puede quedar como otro nombre.
4. **Given** un nombre nuevo que ya usa otro insumo (por nombre o por otro nombre), **When** se
   aplica, **Then** se rechaza nombrando al otro insumo y no cambia nada.
5. **Given** que solo se manda la categoría (o la ubicación, o el mínimo), **When** se aplica,
   **Then** cambia eso y se conserva lo demás; `clear_*` quita el valor.
6. **Given** que no se mandan los otros nombres, **When** se aplica, **Then** se conservan; una
   lista vacía los quita todos.

---

### User Story 3 - Empaques de un solo insumo (Priority: P2)

«¿Qué empaques tiene el jamón?» El agente pide `presentations_list` con `ingredient_id: 749` y
recibe solo los de ese insumo, con su marca.

**Why this priority**: Es lectura y ahorra leer y filtrar más de 30 renglones; acompaña a las dos
anteriores.

**Independent Test**: Comparar la respuesta con y sin el parámetro en la base restaurada.

**Acceptance Scenarios**:

1. **Given** un insumo con empaques, **When** se pide la lista con su id, **Then** trae solo los de
   ese insumo.
2. **Given** que no se manda el parámetro, **When** se pide la lista, **Then** responde igual que
   antes de esta feature.

### Edge Cases

- **El parecido es el mismo insumo**: la vista previa lo dice; el agente pregunta a la persona y no
  aplica. Aplicar de todos modos crea el insumo (como «Crear de todos modos»).
- **Nombre exactamente repetido de un insumo archivado**: la ruta lo rechaza diciendo que está
  archivado; reactivarlo queda en la pantalla (fuera de alcance).
- **Alta con existencia inicial y después una compra del mismo**: la vista previa del alta avisa que
  la compra trae la existencia; si se mandan las dos, el almacén queda con las dos.
- **Ficha cambiada entre la lectura y la escritura** (alguien la editó en la tableta): la ruta
  rechaza por versión vieja; la herramienta dice que se vuelva a intentar y no cambia nada.
- **El nombre se aplicó y los otros nombres fallaron** (un otro nombre ya es de otro insumo): el
  nombre queda cambiado y el error lo dice y nombra al dueño del otro nombre; repetir la llamada con
  la lista corregida termina el trabajo.
- **Más de 10 otros nombres, uno de más de 60 letras, o un nombre vacío**: lo que rechaza la ruta.
- **Insumo de otra empresa, unido a otro o archivado**: lo que responde la ruta («no existe», o la
  causa).
- **Categoría o ubicación de otra empresa**: la ruta lo rechaza.
- **`ingredient_update` sin nada que cambiar**: rechazado antes de despachar.
- **`clear_category` junto con `category_id`** (o ubicación, o mínimo): rechazado como argumento.
- **Token con el alcance y rol sin el permiso**, o al revés: la herramienta no se ofrece y, si se
  llama, se niega nombrando lo que falta.

## Requirements *(mandatory)*

### Functional Requirements

**Alta de insumos**

- **FR-001**: Una herramienta MUST dar de alta un insumo con los campos de la hoja «Nuevo artículo»
  (nombre, unidad base, existencia inicial, costo inicial, ubicación, mínimo) más la categoría, con
  las reglas y efectos de esa misma alta.
- **FR-002**: Su vista previa MUST poner primero, antes del resumen del alta, «quizá ya existe» con
  el nombre y el id de cada insumo parecido, tomados de la búsqueda de parecidos de la hoja «Nuevo
  artículo» y de la búsqueda por nombre de la lista de Almacén (que incluye otros nombres). Sin
  parecidos, lo dice.
- **FR-003**: Aplicar MUST NO bloquearse por un parecido; el nombre exactamente repetido lo rechaza
  la ruta y la herramienta MUST decir el id del que ya existe y si está archivado.
- **FR-004**: La vista previa MUST avisar que la existencia y el costo de un insumo que se crea para
  una compra llegan con la compra.

**Edición de insumos**

- **FR-005**: Una herramienta MUST editar nombre, categoría, ubicación y mínimo de un insumo y su
  lista de otros nombres, cambiando solo lo que se manda; quitar categoría, ubicación o mínimo MUST
  pedirse de forma explícita, y la lista de otros nombres MUST mandarse completa (vacía los quita).
- **FR-006**: Si en la misma llamada cambian el nombre y los otros nombres, MUST aplicarse primero el
  nombre. Si el segundo paso falla, el error MUST decir que el nombre ya quedó cambiado.
- **FR-007**: Su vista previa MUST mostrar el antes y el después de cada campo que cambia; si cambia
  el nombre, los insumos parecidos al nombre nuevo (sin el propio); y por cada otro nombre nuevo, los
  parecidos que la ficha ya ofrece.
- **FR-008**: La edición MUST usar la versión de la ficha leída en la misma llamada; si la ficha
  cambió entre la lectura y la escritura, se rechaza sin cambiar nada y se dice que se vuelva a
  intentar.

**Empaques por insumo**

- **FR-009**: La lista de empaques MUST aceptar un insumo opcional y, con él, devolver solo los de
  ese insumo; sin él, MUST responder igual que hoy.

**Alcances y comunes (heredadas de la 072, 073 y 074)**

- **FR-010**: Las dos escrituras MUST pertenecer al alcance de almacén y exigir el permiso de
  configurar el almacén; la lista de empaques sigue en consultar con el permiso de recibir mercancía.
  Cada una se ofrece solo si el token y el rol los tienen en ese momento.
- **FR-011**: Toda escritura nueva MUST tener vista previa por omisión, clave de idempotencia al
  aplicar y un evento de bitácora por llamada con antes y después.
- **FR-012**: Cantidades y costo como texto decimal en la unidad base; la empresa siempre del token.
- **FR-013**: El texto del alcance de almacén en Integraciones MUST decir que abarca insumos.

### Key Entities

- **Insumo** (ya existe): nombre, unidad base, categoría, ubicación, mínimo, otros nombres,
  existencia y costo.
- **Empaque de compra** (ya existe): ningún cambio.
- Nada nuevo en la base.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: El caso de aceptación corre de punta a punta contra la API local con el respaldo
  restaurado y un cliente MCP real: (a) alta de un insumo que ya existe con otro nombre → la vista
  previa lo señala; (b) alta de uno nuevo, su empaque y una compra en vista previa con él; (c)
  renombrar 749 a «Jamón» con sus otros nombres; (d) empaques de 749 solos.
- **SC-002**: Pruebas automáticas que fallan si: la vista previa del alta no pone primero un
  duplicado parecido, un nombre repetido se crea, una edición borra lo que no se mandó, el nombre y
  los otros nombres se aplican en otro orden, un token sin el alcance o un rol sin el permiso de
  configurar usan cualquiera de las dos escrituras, o la lista filtrada trae empaques de otro insumo.
- **SC-003**: Repetir cada escritura nueva diez veces con la misma clave deja un solo efecto.
- **SC-004**: Un token de otra empresa no ve ni toca insumos ni empaques de ésta.
- **SC-005**: Dar de alta un insumo nuevo a media compra toma un mensaje y una confirmación desde
  Claude (estimado), contra cortar la captura e ir a la tableta.

## Assumptions

- El nombre, la unidad y si es o no el mismo que un parecido los decide la persona; el agente no
  infiere.
- **Fuera de alcance, como siguientes pasos posibles**: cambiar la unidad base (el issue lo excluye),
  archivar y reactivar, unir insumos, corregir costo, densidad y composición.
- Los insumos preparados (con receta) no se dan de alta por aquí: su alta es la de recetas.
