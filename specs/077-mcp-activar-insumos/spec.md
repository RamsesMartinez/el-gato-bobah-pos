# Feature Specification: Desactivar y reactivar insumos desde el servidor MCP

**Feature Branch**: `077-mcp-activar-insumos`

**Created**: 2026-11-12

**Status**: Draft

**Input**: Issue #277. Seguimiento de la 076 (`specs/076-mcp-insumos`), que dejó archivar fuera a
propósito. Quien administra el inventario por MCP no puede dejar de ofrecer un insumo descontinuado:
`ingredient_update` cambia nombre, categoría, ubicación, mínimo y otros nombres, pero no si está
activo, y hoy hay que ir a la pantalla.

## Contexto

La 072 abrió la puerta para agentes (tokens personales, alcance y permiso leídos en cada llamada,
vista previa por omisión, clave de idempotencia, bitácora por llamada); la 073, la 074 y la 076 le
agregaron compras, empaques e insumos. Cada herramienta despacha a la misma ruta que usa la pantalla
y hereda sus reglas. Esta feature agrega una acción a una herramienta que ya existe, sin reglas
propias: desactivar («archivar», en la pantalla) y reactivar un insumo son las dos acciones de la
hoja «Archivar» de la ficha, y su vista previa es la misma revisión que esa hoja muestra antes de
archivar.

En la pantalla, la palabra es **archivar**; en el issue y en la herramienta, **desactivar**
(`active: false`). Son lo mismo.

## Respuesta a la exclusión «archivar y reactivar» de la 076

| Exclusión | Dónde | Respuesta |
| --- | --- | --- |
| Archivar y reactivar insumos | 076, Assumptions («siguientes pasos posibles») y la pista de `NAME_TAKEN` («Reactivar un archivado es en la pantalla») | Dejar de ofrecer un insumo descontinuado es puntual, como renombrarlo: aparece al capturar una compra o al revisar el almacén, uno a la vez. Entra solo eso, sobre las dos rutas de la hoja «Archivar» y con su misma revisión previa. Unir, cambiar unidad, costo, densidad y composición siguen fuera |

## Decisiones tomadas en este spec (recomendación, sin preguntar)

| # | Duda | Decisión | Por qué |
| --- | --- | --- | --- |
| D1 | Existencia distinta de 0 al desactivar: ¿lo bloquea el servicio o solo lo advierte la vista previa? | **El servicio ya lo bloquea sin confirmación, y no cambia.** La herramienta acepta `confirm_on_hand`, espejo de la casilla «Sí, archivarlo con esa existencia» de la pantalla. La vista previa pone primero el aviso con la cantidad por sucursal y pide registrar merma (`waste_record`) o ajuste (`stock_adjust`) antes; `confirm_on_hand: true` solo si la persona decide archivarlo con esa existencia | Es la decisión del issue («pide merma o ajuste antes») sin una segunda regla: pantalla y MCP pasan por la misma regla del servicio (#203: archivar no ajusta la existencia; se queda y vuelve igual al reactivar). Prohibir la confirmación por MCP haría al agente más estricto que la pantalla con la misma persona detrás |
| D2 | Recetas activas que lo usan | La vista previa nombra cada una (producto, extra o preparado, con su id) y dice que hay que quitarlo de ellas en la pantalla. Aplicar lo rechaza el servicio. **Nada se desactiva en cascada** | Es lo que hace la hoja: una receta activa impide archivar. Lo mismo un conteo abierto que ya lo capturó. Las compras por recibir no lo impiden: la vista previa avisa que sumarán aunque esté inactivo |
| D3 | ¿`active` junto con otros cambios en la misma llamada? | **No**: `active` va solo; `confirm_on_hand` solo con `active: false`. Se rechaza como argumento antes de despachar | Son tres rutas distintas (ficha, otros nombres, archivar); combinarlas deja estados a medias que la 076 ya tuvo que explicar con `PARTIAL`. Dos llamadas cuestan un mensaje |
| D4 | Ya está en el estado pedido | La vista previa lo dice; aplicar no cambia nada y responde como aplicado | El servicio ya responde sin tocar nada en ese caso; un error haría reintentar algo que ya está |
| D5 | ¿Alcance o permiso nuevo? | **Ninguno**: almacén (`write:inventory`) y configurar el almacén, como el resto de `ingredient_update`. Sin migración | **Consecuencia que no se ve**: ensancha los tokens ya emitidos con almacén. Después de la subida, un token con almacén puede desactivar y reactivar insumos si el rol de su dueño tiene configurar el almacén; no es más de lo que esa persona ya puede en la pantalla. Se dice en el comentario final del issue |
| D6 | La pista de nombre repetido decía «Reactivar un archivado es en la pantalla» | Ahora dice que se reactiva con `ingredient_update` y `active: true` | Si no, el agente manda a la tableta lo que ya puede hacer |
| D7 | ¿La lista de existencias muestra los inactivos? | **No**, ni en la pantalla ni por MCP: omite los insumos inactivos y cada renglón dice si está activo. Los productos inactivos siguen saliendo, marcados | La pestaña Existencias es la lista de faltantes (marca «bajo» lo que está en o bajo su mínimo): un insumo descontinuado ahí es un faltante falso. Su existencia sigue a la vista en Almacén › Archivados y en la revisión previa. Desactivar productos no es de esta feature |
| D8 | ¿Cómo encuentra el agente un insumo inactivo para reactivarlo? | `articles_search` acepta `include_inactive: true`: agrega los insumos archivados que coinciden, marcados `active: false`, tomados de la lista Almacén › Archivados (pide ver el almacén). Sin el parámetro responde igual que hoy, y cada renglón dice `active: true` | La búsqueda de hoy es la del buscador de compras y gastos, que a propósito no ofrece archivados; cambiarla sacaría archivados en la captura de compras |

## Qué hace hoy un insumo archivado (verificado con pruebas antes del código)

Se verifica con pruebas, antes de escribir código, contra lo que pide el issue («deja de salir en
conteos, compras y listas de faltantes, y conserva su historial»). Lo que ya cumple no se toca; lo
que no, se arregla en el servicio y la pantalla lo hereda.

| Lugar | Hipótesis por lectura del código | Si falla |
| --- | --- | --- |
| Conteo (lista de qué contar y «sin contar») | Ya no sale | — |
| Compras: buscador de artículos | Ya no sale | — |
| Compras: sugerencias por lo aprendido del proveedor (exacta y de otros proveedores) | **Sí sale**: un renglón ya mapeado a un insumo archivado lo vuelve a proponer, y el exacto se autollena | Se arregla en la consulta: lo aprendido que apunta a un insumo archivado no se propone |
| Compras: sugerencias por nombre parecido | Ya no sale | — |
| Faltantes (pestaña Existencias, marca «bajo») | **Sí sale** | Se arregla en la consulta (D7) |
| Historial: movimientos, historial de costo, bitácora del insumo | Se conserva | — |

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Desactivar un insumo descontinuado (Priority: P1)

«Ya no compramos el jarabe de lavanda; quítalo». El agente pide la vista previa de
`ingredient_update` con `active: false`. Si tiene existencia, lo primero que dice es cuánto queda en
cada sucursal y que conviene registrar la merma o el ajuste antes; si una receta activa lo usa, la
nombra y dice que hay que quitarlo de ella en la pantalla. Sin nada de eso, aplica, y el insumo deja
de salir en conteos, compras y faltantes con su historial intacto.

**Why this priority**: Es el problema del issue: hoy obliga a ir a la tableta.

**Independent Test**: Contra la API local con el respaldo restaurado: vista previa de uno con
existencia, de uno con receta activa, y desactivar uno sin nada de eso.

**Acceptance Scenarios**:

1. **Given** un insumo con existencia distinta de 0 (también negativa), **When** se pide la vista
   previa con `active: false`, **Then** lo primero que dice es la existencia por sucursal y que se
   registre merma o ajuste antes, y no cambia nada.
2. **Given** ese insumo, **When** se aplica sin `confirm_on_hand`, **Then** se rechaza diciendo que
   tiene existencia y nada cambia; **When** se aplica con `confirm_on_hand: true`, **Then** queda
   inactivo con su existencia intacta.
3. **Given** un insumo que usa una receta activa, **When** se pide la vista previa, **Then** nombra
   cada receta con su tipo e id; **When** se aplica, **Then** se rechaza y ni el insumo ni la receta
   ni su producto cambian.
4. **Given** un insumo sin existencia ni recetas, **When** se aplica con una clave nueva, **Then**
   queda inactivo, la bitácora registra la llamada con antes y después y el evento de archivar, y
   repetir la llamada con la misma clave no hace nada más.
5. **Given** el insumo ya inactivo, **When** se pide la vista previa o se aplica, **Then** dice que
   ya está inactivo y no cambia nada.
6. **Given** el insumo inactivo, **Then** ya no sale en la lista del conteo, ni en el buscador ni en
   las sugerencias de compra (tampoco por lo aprendido del proveedor), ni en la lista de existencias;
   sus movimientos, su historial de costo y su bitácora siguen legibles.

---

### User Story 2 - Reactivar un insumo (Priority: P1)

«El jarabe de lavanda regresó». El agente lo busca con `articles_search` e `include_inactive: true`,
lo encuentra marcado inactivo y pide `ingredient_update` con `active: true`: vuelve tal como estaba,
con la existencia que tenía.

**Why this priority**: El issue lo pide con el mismo parámetro; sin esto, desactivar por error
obliga a ir a la tableta.

**Independent Test**: Reactivar el insumo de la historia 1 en la base restaurada y comprobar que
vuelve a salir en conteo, compras y existencias.

**Acceptance Scenarios**:

1. **Given** un insumo inactivo, **When** se pide la vista previa con `active: true`, **Then** dice
   que vuelve a ofrecerse tal como estaba, con su existencia, y no cambia nada.
2. **Given** la vista previa, **When** se aplica, **Then** queda activo y vuelve a salir en conteo,
   compras y existencias.
3. **Given** que otro insumo activo tomó su nombre mientras estaba inactivo, **When** se aplica,
   **Then** se rechaza nombrando al otro (id) y no cambia nada.
4. **Given** un insumo que se unió a otro, **When** se aplica, **Then** se rechaza diciendo a cuál se
   unió.
5. **Given** un insumo ya activo, **When** se pide `active: true`, **Then** dice que ya está activo
   y no cambia nada.

---

### User Story 3 - Saber si un insumo está activo al buscarlo o al ver existencias (Priority: P2)

El agente que busca o lista existencias necesita saber qué está activo para no proponer un insumo
descontinuado, y para encontrar uno inactivo que hay que reactivar.

**Why this priority**: Es lectura y acompaña a las dos anteriores.

**Independent Test**: Comparar `articles_search` con y sin `include_inactive` y `stock_levels`
antes y después de desactivar un insumo.

**Acceptance Scenarios**:

1. **Given** una búsqueda sin `include_inactive`, **Then** responde los mismos renglones que hoy, cada
   uno con `active: true`.
2. **Given** `include_inactive: true`, **Then** agrega los insumos inactivos que coinciden por nombre
   u otro nombre, marcados `active: false`, sin repetir ninguno.
3. **Given** un rol que captura compras pero no ve el almacén, **When** pide `include_inactive`,
   **Then** se le dice que hace falta ver el almacén, en vez de responder como si no hubiera
   inactivos.
4. **Given** `stock_levels`, **Then** cada renglón dice si está activo y los insumos inactivos no
   salen (igual que en la pantalla).

### Edge Cases

- **Existencia negativa**: cuenta como existencia distinta de 0 (la regla del servicio).
- **Existencia que llega entre la vista previa y aplicar**: el servicio lo rechaza al aplicar; el
  error dice que tiene existencia y que se repita la vista previa.
- **Conteo abierto que ya lo capturó**: la vista previa lo dice; aplicar se rechaza.
- **Compras por recibir**: no impiden desactivar; la vista previa avisa que sumarán aunque esté
  inactivo.
- **Varias recetas activas**: se nombran todas.
- **Un insumo en uso por una venta en curso** (el candado del servicio no espera): se rechaza con
  «vuelve a intentar».
- **`active` junto con `name`, `category_id`, `place_id`, `min_stock`, sus `clear_*` o `alt_names`**:
  rechazo de argumentos.
- **`confirm_on_hand` sin `active: false`**: rechazo de argumentos.
- **Insumo de otra empresa**: «no existe», como la ruta.
- **Token sin almacén, o rol sin configurar el almacén**: la herramienta no se ofrece y, si se llama,
  se niega nombrando lo que falta; nada cambia.
- **Rol que configura el almacén pero no lo ve**: la revisión previa pide ver el almacén; la vista
  previa dice que no pudo revisar existencia ni recetas en vez de callarlo. Aplicar lo decide la ruta.
- **Un renglón de compra ya mapeado a un insumo que después se desactivó**: deja de proponerse y el
  renglón se trata como uno sin aprender (se vuelve a elegir).

## Requirements *(mandatory)*

### Functional Requirements

**Desactivar y reactivar**

- **FR-001**: `ingredient_update` MUST aceptar `active` (verdadero o falso) y aplicarlo con las mismas
  acciones y reglas que la hoja «Archivar» de la ficha, sin reglas propias.
- **FR-002**: `active` MUST ir solo: con cualquier otro cambio de la ficha o de los otros nombres se
  rechaza antes de despachar. `confirm_on_hand` MUST aceptarse solo con `active: false`.
- **FR-003**: La vista previa de `active: false` MUST poner primero, antes de lo demás: la existencia
  por sucursal si es distinta de 0, con la indicación de registrar merma o ajuste antes y de que solo
  se confirma si la persona decide archivarlo con esa existencia; cada receta activa que lo usa (tipo,
  nombre e id) con la indicación de quitarlo de ella en la pantalla; y el conteo abierto que ya lo
  capturó. Las compras por recibir van como aviso.
- **FR-004**: Aplicar `active: false` con existencia y sin `confirm_on_hand` MUST rechazarse (regla
  del servicio, la misma de la pantalla); con receta activa o conteo abierto, MUST rechazarse siempre.
  Ningún producto, extra ni receta cambia.
- **FR-005**: La vista previa de `active: true` MUST decir que vuelve tal como estaba, con su
  existencia; aplicar MUST rechazar, con la causa, el nombre tomado por otro activo y el insumo unido
  a otro.
- **FR-006**: Si el insumo ya está en el estado pedido, la vista previa MUST decirlo y aplicar MUST NO
  cambiar nada.
- **FR-007**: La pista de nombre repetido MUST decir que un archivado se reactiva con
  `ingredient_update` y `active: true`.

**Lo que deja de ofrecerse**

- **FR-008**: Un insumo inactivo MUST NO salir en la lista del conteo, en el buscador de compras, en
  ninguna sugerencia de compra (incluidas las aprendidas del proveedor) ni en la lista de existencias;
  y su historial (movimientos, historial de costo, bitácora) MUST seguir legible. Lo que ya cumple se
  fija con prueba; lo que no, se corrige en el servicio y la pantalla lo hereda.

**Lecturas**

- **FR-009**: `articles_search` MUST decir en cada renglón si está activo; sin `include_inactive`
  MUST responder los mismos renglones que hoy; con `include_inactive: true` MUST agregar los insumos
  inactivos que coinciden, sin repetir, y decir que hace falta ver el almacén si el rol no lo tiene.
- **FR-010**: `stock_levels` MUST decir en cada renglón si está activo y omitir los insumos
  inactivos, igual que la pantalla.

**Comunes (heredadas de la 072–076)**

- **FR-011**: La acción MUST pertenecer al alcance de almacén y exigir configurar el almacén, leídos
  los dos en cada llamada; la herramienta se ofrece solo si el token y el rol los tienen.
- **FR-012**: Vista previa por omisión, clave de idempotencia al aplicar y un evento de bitácora por
  llamada con antes y después (que incluye si está activo). La empresa, siempre la del token.

### Key Entities

- **Insumo** (ya existe): si está activo; su existencia por sucursal, las recetas que lo usan y su
  historial no cambian al desactivarlo.
- **Renglón aprendido de un proveedor** (ya existe): ningún cambio de datos; deja de proponerse
  mientras su insumo esté inactivo.
- Nada nuevo en la base.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: El caso de aceptación corre de punta a punta contra la API local con el respaldo de
  producción restaurado y un cliente MCP real: (a) desactivar uno con existencia distinta de 0 → la
  vista previa lo advierte; (b) uno usado en una receta activa → la vista previa nombra la receta;
  (c) desactivar uno sin existencia y comprobar que ya no sale en conteo, compras ni faltantes y que
  su historial sigue; (d) reactivarlo con `active: true`; (e) token sin almacén y rol sin configurar
  el almacén → rechazo.
- **SC-002**: Pruebas automáticas que fallan si: la vista previa no pone primero la existencia o no
  nombra una receta activa; se desactiva con existencia sin confirmar, o con una receta activa; se
  combina `active` con otro cambio; reactivar no devuelve el insumo a conteo, compras y existencias;
  un token sin almacén o un rol sin configurar el almacén desactivan; un insumo inactivo sale en
  conteo, compras, sugerencias aprendidas o existencias; su historial deja de leerse.
- **SC-003**: Repetir la misma escritura diez veces con la misma clave deja un solo efecto.
- **SC-004**: Un token de otra empresa no ve ni cambia el estado de un insumo de ésta.
- **SC-005**: Desactivar un insumo descontinuado toma un mensaje y una confirmación desde Claude
  (estimado), contra ir a la tableta.

## Assumptions

- Si un insumo con existencia se archiva así o primero se registra la merma, lo decide la persona;
  el agente no infiere.
- **Fuera de alcance**: borrar insumos; recalcular inventario pasado; desactivar en cascada los
  productos o extras que lo usan; quitar el insumo de una receta por MCP; rechazar en el servicio una
  compra que traiga a mano el id de un insumo inactivo (ni la pantalla ni el MCP lo ofrecen después de
  esta feature: queda como residuo, no como regla); desactivar productos.
- No hay pantalla nueva ni cambio de disposición: la pestaña Existencias deja de listar insumos
  inactivos, y su tabla vacía dice «Sin existencias que mostrar» en vez de «Sin movimientos aún»
  (que deja de ser cierto cuando el único insumo con existencia se desactiva).
