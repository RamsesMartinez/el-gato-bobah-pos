# Feature Specification: Escrituras del servidor MCP — compras, conteos, cancelar pedidos, abrir y cerrar caja

**Feature Branch**: `073-mcp-escrituras`

**Created**: 2026-11-10

**Status**: Draft

**Input**: Issue #153, segunda parte. Encargo del dueño (2026-11-10): las cuatro escrituras que la
primera parte (spec 072, en producción) dejó fuera y que el dueño sí pidió — recibir compras,
conteos de almacén, cancelar pedidos, abrir y cerrar caja — con las mismas reglas que la pantalla y
las de la puerta para agentes de la 072.

## Contexto

La 072 dejó una puerta para agentes: un servidor MCP con tokens personales, alcance del token y
permiso del rol leídos en cada llamada, vista previa por omisión, clave de idempotencia y un evento
de bitácora por llamada. Cada herramienta despacha a la misma ruta que usa la pantalla, así que
hereda sus reglas. Nada de eso se reinventa aquí: esta feature agrega herramientas sobre la misma
puerta.

El motivo original del issue fue que en octubre varias compras confirmadas no se capturaron. La 072
resolvió el gasto pero no la mercancía: el gasto entraba sin tocar el almacén ni el costo.

## Medición (respaldo de producción del 2026-11-10, últimos 30 a 60 días)

Por la regla de `AGENTS.md` §1 aquí va el **orden**, no las cifras del negocio.

- **Compras con mercancía**: alrededor de uno de cada cuatro gastos lleva mercancía al almacén. De
  las que traen fecha de documento, **dos de cada tres se capturaron uno o más días después** de la
  fecha del ticket: es el síntoma del issue, ahora medido sobre la mercancía y no solo el gasto.
- **Compras por recibir**: hay pocas o ninguna pendiente a la vez; el caso normal es volver de la
  tienda con las cosas y capturar una sola vez, ya recibida.
- **Conteos**: se cierran en uno de cada cuatro días, a veces dos el mismo día. La mayoría son
  conteos chicos (de un puñado de artículos); el más grande, de unas decenas.
- **Cancelar pedidos**: raro (un puñado al mes); ninguno con devolución de dinero en el periodo.
- **Abrir y cerrar caja**: todos los días, en dos cajas.
- **Arqueo ciego**: **encendido** en el negocio. Con él encendido, la ruta del turno abierto —la
  misma que despacha la herramienta `cash_session_current`— ya viaja **sin** el esperado ni el
  desglose de ventas. La premisa del encargo («la lectura del turno abierto por MCP ya devuelve el
  esperado») es cierta solo con el arqueo ciego apagado; ver la decisión D4.

## Respuesta a cada objeción de la 072

La tabla «Qué herramientas entran y cuáles no» de `specs/072-servidor-mcp/spec.md` dejó fuera estas
cuatro escrituras con una razón cada una. Se responde por escrito:

| Objeción de la 072 | Respuesta |
| --- | --- |
| **Pedidos**: «un pedido se levanta y se cobra con el cliente enfrente y el dinero en el cajón; un agente no tiene ni lo uno ni lo otro. Cancelar y devolver mueven dinero del corte» | Crear, cobrar, entregar y devolver siguen fuera: ésos sí exigen al cliente y al cajón. **Cancelar** no: lo decide quien administra (el pedido de prueba, el que se capturó dos veces, el que el cliente ya no quiso) y hoy exige pararse frente a la tableta. Mueve dinero del corte solo si el pedido tiene cobros; en ese caso la herramienta exige lo mismo que la pantalla —confirmar que el dinero se regresa y, si fue con tarjeta, el folio de la terminal— y la vista previa dice cuánto y por qué método sale del corte antes de aplicar. El efecto en el corte es el de la pantalla porque es la misma ruta |
| **Caja**: «abrir y cerrar exige contar el cajón» | Sigue exigiéndolo: **contar lo hace una persona frente al cajón**; el agente solo transcribe lo que esa persona dicta, igual que la tableta transcribe lo que se teclea. Lo que la 072 no resolvía era el arqueo ciego, y se resuelve en D4: ninguna herramienta muestra el esperado del turno mientras el negocio cuente a ciegas, y cerrar exige declarar explícitamente cada cifra. La asistencia aplica igual: abrir exige la entrada de quien abre, cerrar la salida de quien abrió (o el cierre autorizado con motivo y permiso) |
| **Inventario**: «el conteo se hace con el insumo en la mano y cerrar reemplaza lo que el sistema creía tener» | El insumo sigue en la mano de la persona que cuenta; el agente captura lo que dicta. Que cerrar reemplaza lo que el sistema cree tener es justo por qué la vista previa del cierre muestra, artículo por artículo, lo que el sistema tiene, lo contado y el ajuste, **antes** de aplicar. Mientras se captura, el conteo sigue a ciegas: capturar no devuelve lo que el sistema tiene |
| **Inventario**: «recibir compras» fuera | Es el motivo original del issue. Ver la siguiente fila |
| **Gastos**: «la mercancía exige escoger insumo y empaque, que se decide mejor en pantalla» | El agente **no escoge**: la vista previa propone candidatos (por nombre y por lo aprendido de ese proveedor) y los empaques del insumo, y aplicar **rechaza** un renglón sin insumo y sin empaque o unidad explícitos. Quien decide es la persona que lee la vista previa; el agente solo pasa los ids que ella confirmó. La vista previa dice además cómo cambia el costo vigente de cada insumo, que en pantalla no se ve hasta después de guardar |

## Decisiones tomadas en este spec (recomendación, sin preguntar)

| # | Duda | Decisión | Por qué |
| --- | --- | --- | --- |
| D1 | ¿Alcances nuevos? | Dos: **compras** (`write:purchases`) y **pedidos** (`write:orders`). Los conteos van con **almacén** (`write:inventory`) y abrir y cerrar caja con **caja** (`write:cash`) | Lo sugirió el dueño. Compras toca dinero y almacén a la vez, y cancelar un pedido puede devolver dinero: ninguna cabe en un alcance que ya existe sin agrandarlo de más. **Consecuencia que no se ve**: un token ya emitido con almacén puede, después de la subida, abrir, capturar y cerrar conteos, y uno con caja puede abrir y cerrar caja. No es más poder del que ya tenía su dueño en la pantalla (sigue limitado por el permiso del rol), y el ajuste de existencia que ese token ya podía hacer mueve el almacén igual que un conteo; se dice en la pantalla de Integraciones (texto de cada alcance) y en el reporte de la subida |
| D2 | ¿Recibir compras crea la compra o solo recibe una pendiente? | Las dos: **capturar una compra con su mercancía** (el caso del issue: ya llegó, ya se pagó o no) y **recibir una compra pendiente** (lo que llegó de lo que estaba «Por recibir») | Lo primero es lo que faltó en octubre; lo segundo es la otra mitad de «recibir» en la pantalla y cuesta una herramienta más sobre la misma ruta |
| D3 | ¿Quién propone el insumo y cuánto cambia el costo? | El servidor: los candidatos salen de la sugerencia que ya usa la captura de gastos (lo aprendido por proveedor y por nombre), y el costo nuevo de una consulta nueva **de solo lectura** que usa la misma regla con que la compra cambia el costo (último precio o promedio ponderado, según el negocio) | Calcular el costo en la capa del agente sería una segunda copia de la regla: «el hermano que no se movió». La vista previa dice que es un cálculo con el estado de ese momento; aplicar lo vuelve a decidir |
| D4 | **Arqueo ciego**: si un agente puede cerrar, ¿conoce la cifra antes de capturar lo contado? | **Se conserva igual que en la pantalla, sin camino nuevo, y se refuerza en el cierre.** (a) El ocultamiento vive en la ruta del turno abierto, que es la que despachan la lectura del turno y las vistas previas de caja: con el arqueo ciego encendido ninguna herramienta devuelve el esperado ni el desglose de la venta del turno. (b) La vista previa de cerrar caja solo lee rutas ya filtradas: dice **qué** hay que declarar (el cajón, cada método, cada terminal, si hay propina por decidir), nunca **cuánto** se espera. (c) Aplicar el cierre exige **cada cifra explícita**: el cajón contado y cada método y terminal que pide declaración; uno que falta se rechaza nombrándolo, en vez de cerrar con cero o con lo que el sistema espera. (d) La diferencia se ve **después** de cerrar, igual que en la pantalla. (e) Con el arqueo ciego apagado, el negocio decidió que quien cuenta vea el esperado: por MCP también, y no aplica | Mismo principio que la 072: la herramienta no puede más ni menos que la pantalla. El riesgo propio del agente —que «ayude» llenando lo que el sistema espera— lo cierra (c). **Lo que no se cierra, y se acepta por escrito**: quien tiene el permiso de ver ventas o de configurar la caja puede reconstruir el esperado desde el resumen de ventas o el histórico, igual que en la pantalla de Ventas; el arqueo ciego protege la cifra de quien cuenta sin esos permisos, y por MCP sigue igual |
| D5 | ¿El conteo por MCP también es a ciegas? | Sí, igual que la pantalla: capturar no devuelve lo que el sistema tiene; la referencia se ve solo en la vista previa del cierre. Una cifra fuera de lo razonable vuelve como «confirma esta cifra» sin decir contra qué | Es la regla de la pantalla (spec 038 y 042). Igual que en caja, quien tiene permiso de ver existencias puede consultarlas en otra herramienta, como en la pantalla; la bitácora del cierre ya cuenta los renglones que cambiaron después de ver las diferencias |
| D6 | ¿Capturar renglones de conteo uno por uno? | En lotes de hasta 50 renglones por llamada, aplicados en orden; si uno falla, se detiene ahí y la respuesta dice cuáles quedaron y cuál falló. Repetir el lote con la misma clave vuelve a fijar las mismas cifras sin duplicar nada | Un conteo de decenas de artículos uno por uno serían decenas de llamadas con vista previa cada una. Capturar una cifra es fijarla, no sumarla: repetirla no cambia nada |
| D7 | ¿Cancelar un pedido cobrado? | Sí, con las reglas de la pantalla: `refund: true` explícito y el folio de la terminal si hubo tarjeta. Sin `refund`, el pedido cobrado se rechaza con la misma causa que en la pantalla | Es lo que la pantalla exige y lo que el dueño pidió: «mismas reglas» |
| D8 | ¿Quién queda como quien abrió o cerró la caja? | La persona dueña del token, siempre. Si contó otra persona, va en las notas del cierre | La 072 fijó que un token es de una persona real y que la bitácora no puede decir que hizo algo alguien que no lo hizo. La asistencia se revisa sobre esa persona |
| D9 | ¿Se puede descartar un conteo por MCP? | Sí, con vista previa: no toca el almacén | La alternativa deja un conteo abierto que bloquea abrir otro hasta que alguien vaya a la tableta |

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Capturar una compra con su mercancía desde Claude (Priority: P1)

El dueño regresa del proveedor con el ticket y dice «registra la compra de Costco: 2 bolsas de
azúcar de 5 kg a $210 cada una y 3 litros de leche a $28, pagado con tarjeta». El agente pide la vista
previa: para cada renglón el servidor propone insumos candidatos y sus empaques, y para los que ya
tienen insumo y empaque dice cuánto entra al almacén en la unidad del insumo, a cuánto sale la
unidad y cómo cambia el costo vigente. El dueño confirma «el azúcar es Azúcar estándar, bolsa de
5 kg»; el agente repite la vista previa con esos ids y aplica. La compra aparece en Gastos con su
mercancía recibida, las existencias suben y el costo del insumo cambia como dijo la vista previa.

**Why this priority**: Es el motivo original del issue.

**Independent Test**: Con un token con alcance de compras de una persona que puede capturar gastos,
capturar una compra de dos renglones en efectivo de la caja abierta; verificar existencias, costo
vigente, salida de caja y bitácora.

**Acceptance Scenarios**:

1. **Given** una compra cuyos renglones traen solo el nombre del ticket, **When** se pide la vista
   previa, **Then** cada renglón trae candidatos de insumo (los aprendidos de ese proveedor primero)
   y no se guarda nada.
2. **Given** un renglón con insumo y empaque explícitos, **When** se pide la vista previa, **Then**
   dice cuánto entra en la unidad del insumo, el costo por unidad de esta compra, el costo vigente de
   hoy y el que quedaría.
3. **Given** que se pide aplicar con un renglón sin insumo o sin empaque ni unidad, **When** se
   ejecuta, **Then** se rechaza nombrando el renglón y qué falta, sin guardar nada.
4. **Given** una compra aplicada, **When** se repite con la misma clave y los mismos argumentos,
   **Then** devuelve el mismo resultado y el almacén no vuelve a subir.
5. **Given** una compra pendiente de recibir (capturada en la pantalla sin la mercancía), **When** el
   agente aplica «llegó» con lo que llegó de cada renglón, **Then** el almacén sube por esa cantidad y
   la compra deja de estar pendiente, igual que en la pantalla.

---

### User Story 2 - Cerrar la caja dictando el arqueo (Priority: P1)

Al final del día, quien cerró el local dice a Claude «cierra la caja principal: en el cajón hay
$3,420, la terminal marcó $1,180, la propina se queda en la caja». El agente pide la vista previa:
sabe qué hay que declarar (el cajón, la terminal, la propina por decidir) y si la asistencia de
quien abrió lo permite, pero no cuánto se espera. Aplica con las cifras dictadas y recibe el corte
con su diferencia, como la pantalla después de cerrar.

**Why this priority**: Abrir y cerrar caja ocurre todos los días en dos cajas, y es la escritura con
la regla más delicada (el arqueo ciego).

**Independent Test**: Con el arqueo ciego encendido, abrir una caja por MCP, cobrar un pedido en la
pantalla, cerrar por MCP; comprobar que ni la lectura del turno ni la vista previa del cierre traen
el esperado, que un cierre sin la cifra de un método se rechaza nombrándolo, y que el corte queda
igual que uno cerrado en la pantalla.

**Acceptance Scenarios**:

1. **Given** el arqueo ciego encendido y un turno abierto con ventas, **When** el agente lee el turno
   o pide la vista previa del cierre, **Then** ninguna respuesta trae el esperado, la diferencia ni
   el desglose de la venta del turno.
2. **Given** la vista previa del cierre, **When** se lee, **Then** dice qué hay que declarar (cajón,
   métodos, terminales, propina por decidir) y el estado de asistencia de quien abrió.
3. **Given** un cierre al que le falta la cifra de un método o terminal que pide declaración,
   **When** se aplica, **Then** se rechaza nombrando lo que falta y el turno sigue abierto.
4. **Given** que quien abrió no ha marcado su salida, **When** se aplica el cierre sin cierre
   autorizado, **Then** ocurre lo mismo que en la pantalla (se rechaza con la causa); con cierre
   autorizado, motivo y el permiso, se cierra y queda el evento de la pantalla.
5. **Given** un cierre aplicado, **When** se lee la respuesta, **Then** trae la diferencia del corte,
   como la pantalla después de cerrar.
6. **Given** el arqueo ciego apagado, **When** se lee el turno, **Then** trae el esperado, igual que
   la pantalla.
7. **Given** una caja cerrada y una persona con su entrada marcada, **When** el agente abre la caja
   con el fondo contado (por denominaciones o total con motivo), **Then** se abre con las reglas de
   la pantalla; sin entrada marcada se rechaza con la causa de la pantalla.

---

### User Story 3 - Contar el almacén dictando (Priority: P2)

Quien está en el almacén dicta «leche 6 litros, azúcar 2 bolsas y media, vasos 340» y el agente abre
el conteo, captura los renglones en lote y, al terminar, pide la vista previa del cierre: por cada
artículo contado ve lo que el sistema tenía, lo contado y el ajuste. Al confirmar, aplica el cierre.

**Why this priority**: Un día de cada cuatro, y cerrar reemplaza lo que el sistema creía tener.

**Independent Test**: Abrir, capturar tres renglones (uno de producto con existencia propia), pedir
la vista previa del cierre y cerrar; comparar con el mismo conteo hecho en la pantalla.

**Acceptance Scenarios**:

1. **Given** que no hay conteo abierto, **When** se abre, **Then** queda abierto igual que desde la
   pantalla; con uno abierto, se rechaza y se dice cuál.
2. **Given** un lote de renglones, **When** se capturan, **Then** la respuesta dice qué quedó
   capturado y no trae lo que el sistema tiene.
3. **Given** una cifra fuera de lo razonable, **When** se captura, **Then** el lote se detiene en ese
   renglón y pide confirmarla (`confirmed: true`) sin decir contra qué se comparó.
4. **Given** un conteo con renglones capturados, **When** se pide la vista previa del cierre, **Then**
   trae por artículo lo que el sistema tenía, lo contado y el ajuste, y lo no contado se dice aparte.
5. **Given** el cierre aplicado, **When** se revisan existencias y bitácora, **Then** son las mismas
   que un cierre en la pantalla.

---

### User Story 4 - Cancelar un pedido desde Claude (Priority: P3)

El dueño dice «cancela el pedido 1234, se capturó dos veces». La vista previa dice el estado del
pedido, qué cobros tiene y, si los tiene, cuánto sale del corte y por qué método. Al confirmar se
cancela con el mismo efecto que en la pantalla (inventario que se repone, corte, aviso a las
tabletas).

**Why this priority**: Raro, pero hoy exige ir a la tableta.

**Independent Test**: Cancelar un pedido sin cobros y uno cobrado en efectivo con `refund: true`;
comparar el corte y el almacén con los mismos casos en la pantalla.

**Acceptance Scenarios**:

1. **Given** un pedido sin cobros, **When** se cancela con motivo, **Then** queda cancelado y la
   barra de las tabletas se actualiza.
2. **Given** un pedido cobrado, **When** se cancela sin `refund: true`, **Then** se rechaza con la
   causa de la pantalla.
3. **Given** un pedido cobrado con tarjeta, **When** se cancela con `refund: true` sin folio,
   **Then** se rechaza pidiendo el folio de la terminal.
4. **Given** un token sin el alcance de pedidos o una persona sin el permiso de cancelar, **When**
   intenta cancelar, **Then** se niega nombrando lo que falta.

### Edge Cases

- **Insumo unido o archivado entre la vista previa y la aplicación**: la aplicación falla con la
  causa de la pantalla (insumo unido nombra el destino); la vista previa no garantiza el resultado.
- **Compra que pone «último precio» con otra compra más reciente del insumo**: la vista previa dice
  que el costo no cambia por esa razón (la regla del costo con compras capturadas tarde).
- **Compra con fecha de recepción anterior al último cierre de caja / pago fuera del arqueo**: mismas
  correcciones que el gasto de la 072.
- **Compra con más de 300 renglones o un lote de conteo con más de 50**: rechazado como argumento.
- **Una venta tomando el insumo mientras se aplica la compra**: «vuelve a intentar», igual que la
  pantalla, sin aplicar nada (la clave queda libre).
- **Cierre de caja con pedidos sin terminar o entregados que deben**: rechazado con la causa de la
  pantalla.
- **Propina pendiente sin decidir**: rechazado pidiendo la decisión.
- **Dos llamadas de cierre a la vez sobre la misma caja**: una cierra, la otra recibe que ya no hay
  turno abierto.
- **Lote de conteo que falla a la mitad**: los renglones anteriores quedaron capturados; repetir con
  la misma clave vuelve a fijar las mismas cifras sin duplicar (capturar fija, no suma).
- **Conteo cerrado o descartado en la tableta mientras el agente captura**: la captura se rechaza
  con la causa.
- **Pedido ya cancelado, entregado o de otra empresa**: lo que responde la pantalla; el de otra
  empresa «no existe».
- **Token con alcance que el rol no tiene**: igual que en la 072, se niega con el permiso que falta.

## Requirements *(mandatory)*

### Functional Requirements

**Alcances**

- **FR-001**: Los tokens MUST admitir dos alcances nuevos: compras y pedidos. Los alcances existentes
  no cambian de nombre ni de significado.
- **FR-002**: La pantalla de Integraciones MUST ofrecer los dos alcances nuevos al crear un token y
  describir en una línea qué permite cada alcance, incluido que almacén abarca conteos y caja abarca
  abrir y cerrar.

**Compras**

- **FR-003**: Una herramienta MUST capturar una compra con su mercancía (proveedor, categoría, total,
  fecha del documento, folio, renglones y pagos opcionales), con las mismas reglas y efectos que la
  captura de un gasto con mercancía en la pantalla: almacén, costo vigente, salida de caja si se paga
  en efectivo, aprendizaje por proveedor y bitácora.
- **FR-004**: La vista previa de una compra MUST proponer candidatos de insumo para cada renglón que
  no trae uno (una vez por nombre y hasta 20 nombres por llamada; los demás lo dicen y se piden en la
  siguiente vista previa), y para cada renglón con insumo y empaque o unidad decir la cantidad que entra en la
  unidad del insumo, el costo por unidad de esta compra, el costo vigente actual y el que quedaría,
  con la regla de costeo del negocio.
- **FR-005**: Aplicar una compra MUST rechazar todo renglón sin insumo (o producto de reventa) y sin
  empaque o unidad explícitos; el agente nunca elige por su cuenta.
- **FR-006**: Una herramienta MUST recibir una compra pendiente indicando lo que llegó de cada renglón,
  con las reglas de la pantalla («Por recibir»), y una lectura MUST listar las compras pendientes.

**Conteos**

- **FR-007**: Herramientas MUST abrir un conteo, capturar renglones en lotes (insumos y productos con
  existencia propia), pedir el cierre con vista previa de diferencias, cerrar y descartar, con las
  reglas de la pantalla.
- **FR-008**: Capturar MUST seguir a ciegas: ninguna respuesta de captura ni de lectura del conteo
  abierto trae lo que el sistema tiene; la cifra fuera de lo razonable pide confirmación sin decir la
  referencia.
- **FR-009**: La vista previa del cierre MUST mostrar por artículo contado lo que el sistema tenía, lo
  contado y el ajuste, y aparte lo no contado (que no se toca).

**Pedidos**

- **FR-010**: Una herramienta MUST cancelar un pedido con motivo, con las mismas reglas y efectos que
  la pantalla (devolución confirmada y folio de terminal si hubo cobros, inventario que se repone,
  corte, aviso a las tabletas). Crear, cobrar, entregar y devolver siguen fuera.
- **FR-011**: La vista previa de cancelar MUST decir el estado del pedido, sus cobros y, si los hay,
  qué sale del corte y por qué método.

**Caja**

- **FR-012**: Herramientas MUST abrir y cerrar caja con las reglas de la pantalla, incluida la
  asistencia (entrada de quien abre; salida de quien abrió o cierre autorizado con motivo y permiso),
  el conteo del cajón (por denominaciones o total con motivo), el arqueo por terminal, la decisión de
  la propina, el fondo que se deja y quién se lleva el retiro.
- **FR-013**: Con el arqueo ciego encendido, ninguna herramienta MUST devolver el esperado, la
  diferencia ni el desglose de la venta de un turno abierto; la vista previa del cierre dice qué
  declarar, nunca cuánto se espera.
- **FR-014**: Aplicar un cierre MUST exigir explícitamente cada cifra que la pantalla pide declarar
  (cajón, cada método y cada terminal que lo requieran); si falta alguna, se rechaza nombrándola.
- **FR-015**: La persona dueña del token MUST quedar como quien abre o cierra; las reglas de
  asistencia se revisan sobre ella.

**Comunes (heredadas de la 072)**

- **FR-016**: Cada herramienta nueva MUST declarar su alcance y el permiso del rol de la ruta que la
  atiende, y ofrecerse solo si el token y el rol los tienen en ese momento.
- **FR-017**: Toda escritura nueva MUST tener vista previa por omisión, clave de idempotencia al
  aplicar y un evento de bitácora por llamada con antes y después, como las de la 072.
- **FR-018**: Dinero como texto decimal con dos decimales, cantidades como texto decimal, fechas como
  días del negocio; la empresa siempre del token.
- **FR-019**: Los errores de las reglas de la pantalla (asistencia, propina, folio, cifra por
  confirmar, insumo ocupado o unido) MUST llegar al agente con la causa y cómo corregirla.

### Key Entities

- **Alcance de token**: se agregan «compras» y «pedidos» a los que ya existen.
- **Vista previa de costo de compra**: por renglón, cantidad en unidad del insumo, costo unitario de
  la compra, costo vigente y costo que quedaría. No se guarda.
- Las demás (compra, renglón de compra, conteo, turno de caja, pedido) ya existen y no cambian.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Contra la API local con un cliente MCP real: una compra con mercancía, un conteo
  abierto-capturado-cerrado, un pedido cancelado, una caja abierta y cerrada, cada una con su rastro
  en la bitácora y sin tocar la pantalla ni la base.
- **SC-002**: Con el arqueo ciego encendido, ninguna respuesta de una herramienta sobre un turno
  abierto contiene el esperado ni la diferencia (prueba automática que recorre las herramientas de
  caja).
- **SC-003**: Un cierre con una cifra faltante, una compra con un renglón sin insumo y un pedido
  cobrado sin devolución confirmada se rechazan sin cambiar nada (pruebas automáticas).
- **SC-004**: Repetir cada escritura nueva diez veces con la misma clave deja un solo efecto (una
  compra, un ajuste por artículo, una cancelación, un turno).
- **SC-005**: Por cada herramienta nueva, una persona sin el permiso o un token sin el alcance es
  rechazado; un token de otra empresa no ve ni toca nada.
- **SC-006**: Capturar una compra de tres renglones desde Claude toma menos que en la tableta
  (estimado: un mensaje y una confirmación contra del orden de 15 toques más teclado).

## Assumptions

- La persona dueña del token es la que está frente al cajón o al almacén, o dicta lo que alguien ahí
  contó; el sistema no puede saber cuál de las dos y lo dice en las notas quien cierra.
- Una compra capturada por MCP llega como la del caso normal en la pantalla: ya recibida, con su
  fecha de recepción (hoy por omisión). Quien quiera dejarla pendiente lo dice explícitamente.
- Los renglones de compra que no van al almacén (lo que no es insumo ni producto de reventa) quedan
  fuera de esta herramienta: el gasto sin mercancía sigue siendo `expense_create`.
- Pesar con empaque en el conteo (lo abierto pesado con su tara) queda fuera: por MCP se dicta la
  cantidad en la unidad del artículo o en piezas. Es lo que se dicta de viva voz.
- Traspasos entre cajas, repartir propinas, corregir salidas, editar compras ya guardadas y
  devolver pagos siguen fuera.
- El texto de terceros (notas de pedidos de plataforma) sigue sin viajar al agente, como en la 072.
