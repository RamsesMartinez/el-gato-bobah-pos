# Feature Specification: Empaques de compra y edición de compras guardadas desde el servidor MCP

**Feature Branch**: `074-mcp-presentaciones`

**Created**: 2026-11-11

**Status**: Draft

**Input**: Issue #268. Encargo del dueño (2026-11-11), al capturar una compra real por MCP: una marca
nueva de un insumo que ya existe no tiene empaque, el agente no puede crearlo y el renglón termina
cargado en la unidad base, sin marca ni empaque. Y una compra ya guardada no se puede corregir por
MCP: solo anularla o ir a la pantalla.

## Contexto

La 072 abrió la puerta para agentes (tokens personales, alcance y permiso leídos en cada llamada,
vista previa por omisión, clave de idempotencia, bitácora por llamada) y la 073 le agregó compras con
mercancía, conteos, cancelar pedidos y caja. Cada herramienta despacha a la misma ruta que usa la
pantalla y hereda sus reglas. Esta feature agrega tres herramientas sobre esa misma puerta, sin
reglas propias.

El hueco que se vio: `purchase_create` exige por renglón un empaque o una unidad, y su vista previa
solo propone empaques que ya existen. Cuando el empaque no existe, el único camino del agente es la
unidad base: la compra entra al almacén con la cantidad correcta pero sin marca ni empaque, y la
siguiente compra de esa marca vuelve a caer en lo mismo. Corregirlo después exige la pantalla.

## Respuesta a cada exclusión de la 072 y la 073

| Exclusión | Dónde | Respuesta para este caso acotado |
| --- | --- | --- |
| **Configurar insumos**: «la configuración de insumos llega en ráfagas y es de pantalla» | 072, tabla «Qué herramientas entran», Inventario | Lo que llega en ráfagas es dar de alta insumos, unidades, recetas, densidades y unir duplicados: eso sigue fuera. Un **empaque de compra** no llega en ráfaga: aparece uno a la vez, cuando el ticket trae una marca o un tamaño que no se había comprado, y es justo en ese momento —con el ticket enfrente— cuando se sabe su contenido. Mandarlo a la pantalla es lo que produce el renglón en gramos. Entra solo crear y editar empaques de un insumo que ya existe; crear insumos queda fuera (siguiente paso posible) |
| **Dar de alta artículos desde la compra** | 072 (mercancía) y 073 (D3, FR-005) | Sigue fuera para insumos. Para empaques, la 073 exigió que el agente no eligiera: aquí tampoco elige. La vista previa de la compra **sugiere** crear el empaque y con qué datos, pero el contenido y la marca los confirma la persona y crear el empaque es una escritura aparte, con su propia vista previa |
| **Editar un gasto ya guardado**: «poco frecuente y sensible (mueve importes ya cortados)» | 072, tabla, Gastos | Sigue siendo poco frecuente, y por eso mismo hoy obliga a ir a la tableta por una corrección de un renglón. Lo sensible se resuelve con el protocolo que la pantalla ya tiene (spec 053): la vista previa dice cómo cambian existencias, costo vigente, importe y estado; confirmar repite la huella de esa vista previa y se rechaza si algo cambió; las decisiones (no mover ante un conteo posterior, pagar o dejar la diferencia) se mandan explícitas. **Los pagos nunca se tocan** desde una edición —tampoco en la pantalla—, así que un corte ya cerrado no se reescribe |
| **Editar compras ya guardadas** | 073, Assumptions | Mismo argumento: es la misma ruta. Cambiar el insumo o el empaque de un renglón es quitarlo y agregarlo en la misma edición, porque la ruta no tiene «cambiar un renglón» y la pantalla hace lo mismo |

## Decisiones tomadas en este spec (recomendación, sin preguntar)

| # | Duda | Decisión | Por qué |
| --- | --- | --- | --- |
| D1 | ¿A qué alcance van las herramientas de empaque? | **Almacén** (`write:inventory`) | Un empaque de compra es configuración del almacén: dice cuánto entra cuando llega una bolsa. **Menú y precios** (`write:catalog`) es lo que se vende y a cuánto; no tiene que ver con lo que se compra |
| D2 | ¿Y la edición de gastos? | **Gastos** (`write:expenses`) | Es el alcance del permiso de editar gastos. Compras (`write:purchases`) se descartó: la edición vale también para un gasto sin mercancía, y el alcance de compras existe para lo que toca dinero y almacén a la vez al **capturar** |
| D3 | ¿Alcances nuevos? | **Ninguno**, y por tanto ninguna migración | Las dos herramientas caben en alcances que ya existen. **Consecuencia que no se ve**: ensancha los tokens ya emitidos. Después de la subida, un token con almacén puede crear y editar empaques si el rol de su dueño tiene el permiso de configurar el almacén, y uno con gastos puede editar gastos si el rol tiene el permiso de editarlos. No es más de lo que esa persona ya puede en la pantalla. Medido en el respaldo del 2026-11-11: el único token vivo («Gestión Claude», del administrador) ya trae los dos alcances, así que cubre las herramientas nuevas sin regenerarlo; se comprueba con `whoami` contra la API local. Se dice en el texto de cada alcance en Integraciones y en el comentario final del issue |
| D4 | ¿Cuándo sugiere la compra crear un empaque? | En todo renglón con insumo y **sin empaque**: si trae unidad, la vista previa avisa que entra en esa unidad sin marca ni empaque; si no trae ninguno de los dos, lista los empaques del insumo. En los dos casos dice que, si llegó en un empaque que no está en la lista, se crea con `presentation_create` y da los argumentos sugeridos (insumo, unidad del insumo, el texto del ticket como nombre de partida), dejando **marca y contenido para que los confirme la persona** | Adivinar «375 g» o «Betty Crocker» del texto del ticket sería una regla nueva en la capa del agente y podría equivocarse con el contenido, que es lo que mueve el almacén. Lo que el servidor sí sabe —el insumo y su unidad— va lleno; lo demás se pregunta |
| D5 | ¿`presentation_update` exige mandar todo? | No: parte del empaque como está hoy y cambia solo lo que el agente manda | La ruta reemplaza el registro completo; mandar un campo vacío por omisión borraría la marca o cambiaría el contenido sin que nadie lo pidiera. Es el mismo patrón de `product_update` |
| D6 | ¿Cómo se pide el «cambiar un renglón»? | Con `remove` (los renglones que se quitan) y `add` (los que entran) en la **misma** llamada; la herramienta no tiene un tercer verbo | La ruta y la pantalla no tienen otro; un verbo propio de la capa del agente sería la regla copiada que el encargo prohíbe. En la misma edición la vista previa muestra el efecto neto en existencias y costo |
| D7 | ¿Cómo convive la huella de la ruta con la clave de idempotencia? | La vista previa devuelve la huella y las decisiones pendientes; aplicar exige mandar la huella (`fingerprint`) y las decisiones, **más** la clave. Si algo cambió entre la vista previa y la aplicación, se rechaza con «vuelve a pedir la vista previa» y la clave queda libre | La huella protege contra el estado que cambió; la clave, contra el reintento que duplicaría. Son dos riesgos distintos y ninguno cubre al otro |

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Corregir una compra que entró en la unidad base (Priority: P1)

El dueño capturó por Claude una compra en la que un renglón de harina entró en gramos porque la marca
nueva no tenía empaque. Dice «la harina era Betty Crocker de 375 g; créale el empaque y corrige la
compra». El agente crea el empaque con vista previa y confirmación, pide la vista previa de la
edición —quitar el renglón en gramos y agregar el mismo con el empaque nuevo— y ve que las
existencias y el costo vigente quedan igual (la misma cantidad entró de otra forma) y el importe no
cambia. Aplica, y la compra queda con su marca y su empaque, como si se hubiera capturado bien.

**Why this priority**: Es el caso del encargo y combina las tres herramientas.

**Independent Test**: Con el respaldo restaurado y un cliente MCP real contra la API local: crear la
compra en gramos, crear el empaque, editar la compra; comparar existencias, costo vigente, importe y
empaque del renglón con los de una compra capturada bien desde el principio.

**Acceptance Scenarios**:

1. **Given** una compra guardada con un renglón de un insumo en la unidad base, **When** el agente
   pide la vista previa de quitar ese renglón y agregar el mismo con un empaque, **Then** la vista
   previa dice cómo quedan existencias, costo vigente, importe y estado, trae la huella y no cambia
   nada.
2. **Given** esa vista previa, **When** se aplica con su huella y una clave nueva, **Then** el
   renglón queda con el empaque y su marca, la existencia y el costo vigente quedan como los de una
   compra capturada así desde el principio, y la bitácora registra la edición con el token.
3. **Given** que algo cambió en la compra o en el almacén después de la vista previa, **When** se
   aplica con la huella vieja, **Then** se rechaza diciendo que se vuelva a pedir la vista previa, y
   nada cambia.
4. **Given** una edición que pide una decisión (un conteo cerrado después de la llegada, o un
   importe que sube en una compra con pagos), **When** se aplica sin la decisión, **Then** se rechaza
   nombrando qué decisión falta y sus opciones.
5. **Given** la misma aplicación repetida con la misma clave, **When** se repite, **Then** devuelve
   el mismo resultado y la edición ocurre una vez.

---

### User Story 2 - Crear el empaque de una marca nueva al capturar la compra (Priority: P1)

El dueño dicta una compra con «Harina Betty Crocker 375 g». La vista previa de la compra encuentra el
insumo, pero no hay empaque de esa marca: dice que se puede crear con `presentation_create` y con qué
datos. El agente pregunta marca y contenido, crea el empaque y repite la vista previa de la compra
con él.

**Why this priority**: Evita el problema en el origen; sin esto la historia 1 se repite cada compra.

**Independent Test**: Vista previa de una compra con un renglón de insumo sin empaque: comprobar la
sugerencia; crear el empaque; capturar la compra con él.

**Acceptance Scenarios**:

1. **Given** un renglón con insumo y sin empaque, **When** se pide la vista previa de la compra,
   **Then** el renglón trae los empaques activos del insumo (con su marca) y la sugerencia de crear
   uno con `presentation_create`, con insumo y unidad llenos y marca y contenido por confirmar.
2. **Given** un renglón con insumo y unidad (sin empaque), **When** se pide la vista previa, **Then**
   avisa que entrará en esa unidad sin marca ni empaque, con la misma sugerencia.
3. **Given** un empaque nuevo con marca, **When** se crea, **Then** queda en el insumo y la siguiente
   vista previa de compra lo lista.
4. **Given** un empaque con la misma marca y nombre que otro del mismo insumo, **When** se aplica,
   **Then** se rechaza como en la pantalla (ya existe) y no se crea nada.
5. **Given** una marca de más de 40 letras, **When** se aplica, **Then** se rechaza como argumento
   inválido y no se crea nada.
6. **Given** un empaque existente, **When** se edita solo su marca (o se desactiva), **Then** cambia
   eso y nada más; el contenido y el nombre se conservan.

---

### User Story 3 - Corregir el encabezado o los renglones de un gasto (Priority: P2)

«El gasto 812 era de Costco, no de Walmart, y le faltó un renglón de servilletas». El agente pide la
vista previa (cambio de proveedor, renglón agregado, importe y estado de antes y después) y aplica.

**Why this priority**: Misma herramienta que la historia 1, otros casos de la misma ruta.

**Independent Test**: Editar el proveedor y agregar un renglón sin mercancía a un gasto pagado;
comprobar que el importe sube, que se pide la decisión de la diferencia y que los pagos no cambian.

**Acceptance Scenarios**:

1. **Given** un gasto, **When** se pide la vista previa de cambiar proveedor, categoría, fecha o
   comentario, **Then** dice el antes y el después de cada campo.
2. **Given** un gasto anulado, **When** se intenta editar, **Then** se rechaza con la causa.
3. **Given** quitar un renglón que ya sumó al almacén, **When** la persona no tiene el permiso de
   recibir mercancía, **Then** se rechaza nombrando ese permiso.

### Edge Cases

- **Huella de una vista previa vieja**: rechazo con «vuelve a pedir la vista previa»; la clave queda
  libre para el siguiente intento.
- **Decisión con un valor que no es de sus opciones**: lo que responde la ruta (argumento inválido).
- **Aplicar sin huella**: se rechaza antes de despachar, diciendo que primero va la vista previa.
- **Un renglón que se quita y no es de esa compra, o ya se quitó**: «no existe», como la ruta.
- **Más de 300 renglones entre agregados y quitados**: rechazado como argumento.
- **Editar sin nada que cambiar**: rechazado por la ruta.
- **Empaque de un insumo de otra empresa o de un insumo unido**: lo que responde la ruta («no
  existe», o el destino de la unión).
- **Empaque con contenido cero o negativo, o unidad de otra clase** (litros en un insumo en gramos):
  lo que responde la ruta.
- **Editar un empaque que ya se usó en compras**: las compras guardadas conservan la marca de cuando
  se compraron, como en la pantalla.
- **Token con el alcance y rol sin el permiso**, o al revés: la herramienta no se ofrece y, si se
  llama, se niega nombrando lo que falta.

## Requirements *(mandatory)*

### Functional Requirements

**Empaques**

- **FR-001**: Una herramienta MUST crear un empaque de compra de un insumo existente (nombre,
  contenido, unidad, marca opcional, aproximado opcional) con las reglas y efectos de la pantalla de
  empaques.
- **FR-002**: Una herramienta MUST editar un empaque existente cambiando solo los campos que se
  mandan (nombre, contenido, unidad, marca, aproximado, activo); lo no mandado se conserva. Mandar la
  marca vacía la quita.
- **FR-003**: Las dos MUST rechazar sin cambiar nada lo que la pantalla rechaza, en particular la
  marca repetida en el mismo insumo (ya existe) y la marca de más de 40 letras (argumento inválido),
  con la causa y cómo corregirla.
- **FR-004**: La vista previa de crear o editar un empaque MUST mostrar el insumo y sus empaques
  actuales, para que se vea un posible duplicado antes de aplicar.

**Sugerencia en la compra**

- **FR-005**: La vista previa de `purchase_create` MUST, para cada renglón con insumo y sin empaque,
  listar los empaques activos del insumo con su marca y sugerir crear uno con `presentation_create`,
  con los argumentos que el servidor conoce (insumo y su unidad, el texto del ticket como nombre de
  partida) y marca y contenido marcados como por confirmar con la persona.
- **FR-006**: Si el renglón trae unidad y no empaque, la vista previa MUST avisar que entrará en esa
  unidad sin marca ni empaque.

**Editar un gasto**

- **FR-007**: Una herramienta MUST editar un gasto o compra guardada —encabezado (proveedor,
  categoría, fecha, comentario), renglones agregados y renglones quitados— con las reglas y efectos
  de la edición de la pantalla.
- **FR-008**: Su vista previa MUST decir, en el antes y el después, cómo cambian los campos del
  encabezado, cada renglón, las existencias de cada artículo tocado, el costo vigente de cada insumo,
  el importe, lo pagado y el estado del gasto; las decisiones pendientes con sus opciones; y la huella
  que aplicar debe repetir.
- **FR-009**: Aplicar MUST exigir la huella de la última vista previa y las decisiones que pidió,
  además de la clave de idempotencia. Una huella vencida o una decisión faltante se rechazan sin
  cambiar nada, diciendo cómo corregir.
- **FR-010**: Cambiar el insumo o el empaque de un renglón MUST hacerse quitando y agregando en la
  misma edición; la herramienta lo dice en su descripción.
- **FR-011**: Mover el almacén desde una edición (agregar algo que ya llegó, quitar algo que ya sumó)
  MUST exigir además el permiso de recibir mercancía, como en la pantalla.
- **FR-012**: Los pagos del gasto MUST NO cambiar por una edición.

**Alcances y comunes (heredadas de la 072 y la 073)**

- **FR-013**: Crear y editar empaques MUST pertenecer al alcance de almacén y exigir el permiso de
  configurar el almacén; editar gastos MUST pertenecer al alcance de gastos y exigir el permiso de
  editar gastos. Cada una se ofrece solo si el token y el rol los tienen en ese momento.
- **FR-014**: Toda escritura nueva MUST tener vista previa por omisión, clave de idempotencia al
  aplicar y un evento de bitácora por llamada con antes y después.
- **FR-015**: Dinero y cantidades como texto decimal; fechas como días del negocio; la empresa
  siempre del token.
- **FR-016**: El texto de los alcances de almacén y gastos en Integraciones MUST decir que abarcan
  empaques de compra y corregir gastos, respectivamente.

### Key Entities

- **Empaque de compra** (ya existe): insumo, nombre, contenido en la unidad del insumo, marca
  opcional, aproximado, activo.
- **Edición de un gasto** (ya existe, spec 053): cambios de encabezado, renglones agregados y
  quitados, decisiones y huella. No se guarda la vista previa.
- Nada nuevo en la base.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: El caso del encargo corre de punta a punta contra la API local con el respaldo
  restaurado y un cliente MCP real: compra en la unidad base → empaque creado → compra editada; al
  final, existencias, costo vigente e importe son iguales a los de la misma compra capturada con el
  empaque desde el principio, y el renglón trae empaque y marca.
- **SC-002**: Pruebas automáticas que fallan si: la marca repetida no se rechaza, la marca de 41
  letras no se rechaza, una huella vencida aplica algo, una decisión faltante aplica algo, un token
  sin el alcance o un rol sin el permiso usan cualquiera de las tres herramientas.
- **SC-003**: Repetir cada escritura nueva diez veces con la misma clave deja un solo efecto (un
  empaque, una edición).
- **SC-004**: Un token de otra empresa no ve ni toca empaques ni gastos de ésta.
- **SC-005**: Corregir un renglón de una compra desde Claude toma un mensaje y dos confirmaciones
  (estimado), contra ir a la tableta, abrir el gasto y rehacer el renglón.

## Assumptions

- La marca y el contenido de un empaque los dicta o confirma la persona con el ticket enfrente; el
  agente no los infiere.
- Crear insumos (`ingredient_create`) queda fuera; es el siguiente paso posible si el caso aparece
  con un insumo que no existe.
- Anular gastos ya existe (`expense_cancel`) y no cambia.
- Entregas parciales de un renglón por recibir (la otra ruta de la 053) quedan fuera: ya existe
  `purchase_receive` para lo que llega.
- Excluir o volver a incluir un renglón «que no es del negocio» (spec 050) queda fuera: es otra
  ruta y no se pidió.
