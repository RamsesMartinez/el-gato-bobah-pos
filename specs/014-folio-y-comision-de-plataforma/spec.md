# Feature Specification: El folio con el que llegó el pedido, y lo que la plataforma se quedó

**Feature Branch**: `014-folio-y-comision-de-plataforma` (pendiente de crear; el spec nació en `develop`)

**Created**: 2026-09-06

**Status**: Draft

**Input**: User description: "Cada peso de plataforma deja tres rastros: con qué folio llegó, cuánto se quedó la plataforma y quién pagó el descuento."

## Contexto medido *(no se re-deriva)*

Lo que sigue está medido contra documentos reales y vive en `docs/`. Ningún artefacto de esta
feature debe volver a calcularlo:

| Hecho | Dónde está |
|---|---|
| Comisiones reales: Uber Eats 30.00%, DiDi Food 30.00%, Rappi 20.00% | [plataformas-digitales.md](../../docs/plataformas-digitales.md) |
| Con 35% de sobreprecio, Uber y DiDi devuelven $88.02 por cada $100 de precio de mostrador | idem |
| Las tres plataformas separan contablemente **quién financia** una promoción, y en Rappi eso cambia por campaña | idem §4-bis |
| En DiDi la comisión se calcula sobre el precio **sin promoción** (medido en 13 pedidos); en Uber la base bajo promoción **sigue sin resolverse** | idem §2-bis |
| Cero registros de comisión de plataforma en 1,129 gastos y 871 movimientos de dos años y medio de FUDO | [respaldo-fudo.md](../../docs/respaldo-fudo.md) §8 |
| El acceso por API está cerrado para este negocio; la captura manual es el único camino hoy | [apis-de-plataformas.md](../../docs/apis-de-plataformas.md) §1 |
| Qué decisiones de esquema son **irrecuperables** si se toman mal | idem §7.2 |

**El dato que fija la urgencia**: el reporte de pagos de Rappi expone 3 meses de historia y el de
Uber 31 días. Un folio que no se capture ahora no se recupera después.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - El pedido de plataforma nace con su folio (Priority: P1)

Quien captura un pedido que llegó por Uber Eats, DiDi Food o Rappi tiene enfrente, en la tablet de
la plataforma, el identificador con el que esa plataforma lo nombra. Hoy ese identificador se pierde
en el momento en que la persona voltea a capturar el pedido en el POS. Con esta historia, el POS se
lo pide ahí mismo, mientras lo está viendo.

**Why this priority**: es lo único de esta feature que **no se puede recuperar después**. Sin folio,
un depósito solo se puede conciliar por monto y fecha, y eso empata mal en cuanto hay dos pedidos del
mismo importe. Todo lo demás de esta feature sigue estando en los documentos de pago, que el negocio
conserva; el folio no.

**Independent Test**: se captura un pedido de plataforma con su folio, se cierra la sesión y se busca
ese pedido por el folio del documento de pago. Entrega valor sola: aunque nada más de esta feature se
construya, el folio queda guardado y la conciliación manual se vuelve posible.

**Acceptance Scenarios**:

1. **Given** que se está capturando un pedido con la lista de precios de una plataforma activa,
   **When** el operador escribe el identificador que muestra la plataforma y manda el pedido,
   **Then** el pedido queda guardado con ese identificador tal cual, sin cambiarle mayúsculas, guiones
   ni longitud.
2. **Given** un pedido de plataforma en captura, **When** el operador manda el pedido sin haber
   escrito el identificador, **Then** el sistema le pide el dato antes de mandarlo, y ofrece una única
   salida explícita para continuar sin él.
3. **Given** que el operador toma la salida explícita, **When** el pedido se manda, **Then** el pedido
   queda guardado sin folio y **aparece listado como pendiente**, de modo que la falta sea visible y
   no se confunda con un descuido.
4. **Given** un pedido de mostrador (sin plataforma), **When** se captura, **Then** el campo de folio
   **no existe** en la pantalla y no ocupa alto.
5. **Given** un identificador que ya está en otro pedido de la misma plataforma, **When** el operador
   lo escribe, **Then** el sistema lo rechaza **diciendo cuál pedido ya lo tiene**, no con un aviso
   genérico de duplicado.
6. **Given** un identificador escrito con espacios al principio o al final (típico al pegarlo),
   **When** se guarda, **Then** se guardan solo los espacios de los extremos recortados y nada más:
   ninguna otra transformación.

---

### User Story 2 - Ver qué pedidos quedaron sin folio, y completarlos (Priority: P2)

El dueño abre la pantalla de Ventas y puede quedarse solo con los pedidos de plataforma a los que
les falta el folio, para completarlos con el documento de pago en la mano.

**Why this priority**: sin esto, un folio con salida de escape es un campo opcional, y un campo
opcional que nadie llena no resuelve nada. Esta historia es la que convierte "se puede omitir" en
"se puede omitir y después se ve".

**Independent Test**: se capturan tres pedidos de plataforma, uno sin folio; se filtra la lista y
aparece exactamente ese; se le escribe el folio y desaparece del filtro. Entrega valor sola sobre lo
que la US-1 ya guardó.

**Acceptance Scenarios**:

1. **Given** pedidos de plataforma con y sin folio, **When** se aplica el filtro de pendientes,
   **Then** la lista muestra únicamente los que no tienen folio.
2. **Given** ese filtro aplicado, **When** se lee el resumen de la misma pantalla, **Then** el resumen
   describe **el mismo conjunto** que la lista, sin excepción.
3. **Given** un pedido ya cobrado y sin folio, **When** el dueño le escribe el folio, **Then** el
   folio queda guardado y **ninguna cifra de venta, de corte o de arqueo cambia**.
4. **Given** un pedido de un arqueo ya cerrado, **When** se le escribe el folio, **Then** el total
   cerrado de ese arqueo sigue siendo exactamente el mismo.
5. **Given** un filtro de pendientes con un valor que el sistema no conoce, **When** llega,
   **Then** se rechaza como parámetro inválido; nunca cae en silencio a "todos".

---

### User Story 3 - Registrar lo que la plataforma se quedó (Priority: P3)

Cuando llega el documento de pago de la plataforma —días después de la venta— el dueño abre el
pedido y anota lo que ese documento dice: cuánto reportó la plataforma como venta, cuánto cobró de
comisión, cuánto del descuento lo puso la plataforma y cuánto lo puso el restaurante, qué se retuvo
y con qué referencia de depósito llegó.

**Why this priority**: es lo que hace visible una pérdida que lleva años siendo invisible, pero **es
recuperable**: mientras el pedido tenga folio, ese dato se puede capturar el mes que viene sin
perderlo. Por eso va después de las dos anteriores.

**Independent Test**: con un documento de pago real de una plataforma, se registra la liquidación de
un pedido y el sistema puede decir cuánto vendió ese pedido, cuánto se quedó la plataforma y cuánto
llegó al banco — tres cifras distintas que hoy no existen.

**Acceptance Scenarios**:

1. **Given** un pedido de plataforma cobrado, **When** el dueño registra su liquidación desde el
   documento, **Then** quedan guardados el importe que la plataforma reporta, la comisión (monto y
   tasa), la parte del descuento que financió la plataforma, la parte que financió el restaurante,
   las retenciones atribuidas al pedido, el neto y la referencia del depósito.
2. **Given** un pedido con liquidación registrada, **When** se registra otra vez desde un documento
   corregido, **Then** la liquidación se **reemplaza**, no se duplica.
3. **Given** una liquidación registrada, **When** se consulta cualquier total de ventas del sistema,
   **Then** ese total es idéntico al de antes de registrarla: **la comisión no se resta de ninguna
   venta**.
4. **Given** un pedido cuya promoción la financió el restaurante por completo, **When** su neto
   resulta negativo, **Then** se acepta y se muestra como negativo — es lo que de verdad pasó.
5. **Given** una comisión negativa, una tasa fuera de rango o un importe absurdo, **When** se intenta
   guardar, **Then** se rechaza como entrada inválida, no como error del servidor.
6. **Given** una parte del descuento financiada por la plataforma mayor que el descuento total,
   **When** se intenta guardar, **Then** se rechaza.
7. **Given** un pedido sin liquidación registrada, **When** se consulta, **Then** se distingue con
   claridad de uno cuya liquidación dice cero: "todavía no se sabe" y "la plataforma no cobró nada"
   no son lo mismo.
8. **Given** un periodo con liquidaciones registradas, **When** se lee su resumen, **Then** cada
   cifra declara qué incluye y qué excluye.

---

### Edge Cases

Enumerados **antes** de escribir código, como pide el principio IV. Cada uno debe dejar un test que
falle nombrando el concepto que se rompió.

**Del folio**

- **Cadena vacía**: un folio de texto vacío **no es lo mismo que no tener folio**. Guardar `""` haría
  que dos pedidos sin folio chocaran contra la unicidad, o peor, que pasaran. Se rechaza: o hay
  identificador o no hay campo.
- **Solo espacios**: mismo caso que el anterior después de recortar los extremos.
- **Duplicado por dedazo**: es el caso normal, no la excepción. El mensaje nombra el pedido que ya lo
  tiene.
- **El mismo folio en dos empresas distintas**: permitido. La unicidad es por empresa y por
  plataforma, nunca global.
- **Cambiar la plataforma de un pedido que ya tiene folio**: un folio de Uber colgando de Rappi es
  basura silenciosa. Al cambiar de plataforma se decide explícitamente qué pasa con el folio.
- **Un folio muy largo o con caracteres raros**: se guarda tal cual, con un tope de longitud de
  cordura. Uber es un identificador de 36 caracteres, DiDi de 19 dígitos, Rappi de 10.
- **Pedido cancelado que ya tenía folio**: el folio se conserva. La plataforma también lo canceló y su
  documento lo trae; borrarlo destruye justo el rastro que sirve para explicar la cancelación.
- **Pedido de mostrador con folio**: imposible por construcción, no por validación en la pantalla.

**De la liquidación**

- **Documento corregido**: la plataforma re-emite. La segunda captura reemplaza a la primera y deja
  rastro de cuándo se capturó.
- **Neto negativo**: real y aceptado (promoción financiada por el restaurante).
- **Comisión mayor que la venta**: posible con promoción; se acepta. Comisión **negativa**: se rechaza.
- **Un pedido del POS que el documento de la plataforma no trae**: no se inventa liquidación. Queda
  sin liquidar y eso se ve.
- **Un pedido del documento que no existe en el POS**: fuera de alcance aquí (lo resuelve la feature de
  conciliación), pero el spec lo nombra para que no se descubra tarde: hoy pasa con los 19 pedidos de
  plataforma de agosto de 2026 que viven solo en el respaldo de FUDO.
- **Redondeo**: cada importe se redondea en la frontera y se valida antes de tocar una columna de
  dinero, como manda el principio III.

**De la pantalla**

- **La lista y el resumen**: si el filtro de pendientes toca uno y no el otro, la pantalla miente y
  quien la lee no tiene forma de saber cuál mitad. Ya costó un turno con $4,500 de faltante
  inexplicable.
- **Alto de pantalla**: el campo de folio aparece dentro del bloque que el selector de plataforma ya
  ocupa cuando hay una lista activa, y **solo cuando la hay**. El spec exige contar cuántos renglones
  de contenido quedan debajo, medido a 1024×600.
- **Teclado en tableta**: el campo es de texto y hay que teclearlo con el dedo. El teclado del sistema
  tapa parte de la pantalla al abrirse; el campo tiene que seguir visible mientras se escribe.

## Requirements *(mandatory)*

### Functional Requirements

**El folio**

- **FR-001**: El sistema MUST guardar, por cada pedido de plataforma, el identificador con el que esa
  plataforma nombra al pedido, como **texto libre**, opcional, y **sin transformarlo** más allá de
  recortar los espacios de los extremos.
- **FR-002**: El sistema MUST rechazar un identificador vacío o compuesto solo de espacios: la
  ausencia de folio se representa como ausencia, nunca como texto vacío.
- **FR-003**: El sistema MUST impedir que dos pedidos **de la misma empresa y la misma plataforma**
  compartan identificador, y MUST permitir que dos empresas distintas usen el mismo.
- **FR-004**: Al rechazar un identificador repetido, el sistema MUST decir **cuál pedido ya lo tiene**.
- **FR-005**: El sistema MUST pedir el identificador al capturar un pedido de plataforma, y MUST
  ofrecer **una** salida explícita para mandar el pedido sin él.
- **FR-006**: El sistema MUST permitir escribir o corregir el identificador de un pedido ya cobrado,
  incluido uno de un arqueo cerrado, **sin alterar ninguna cifra de venta, de corte ni de arqueo**.
- **FR-007**: El campo de identificador MUST NOT existir en la captura de un pedido que no es de
  plataforma.

**La visibilidad**

- **FR-008**: La pantalla de Ventas MUST poder filtrarse a los pedidos de plataforma sin identificador.
- **FR-009**: Cuando ese filtro está aplicado, la lista y el resumen de esa pantalla MUST derivarse del
  **mismo predicado**.
- **FR-010**: Un valor de filtro que el sistema no reconoce MUST rechazarse como inválido; MUST NOT
  caer en silencio a un default.
- **FR-011**: La pantalla de Ventas MUST poder mostrar el identificador de cada pedido de plataforma.

**La liquidación**

- **FR-012**: El sistema MUST guardar, por pedido de plataforma, lo que el documento de pago dice:
  importe reportado por la plataforma, comisión en monto y en tasa, descuento financiado por la
  plataforma, descuento financiado por el restaurante, retenciones atribuidas al pedido, neto y
  referencia del depósito.
- **FR-013**: Esos valores MUST guardarse como **copia del documento** (snapshot), no calcularse a
  partir de un porcentaje configurado.
- **FR-014**: Un pedido MUST tener a lo más una liquidación; registrarla de nuevo la reemplaza y deja
  rastro de cuándo se capturó y de qué documento salió.
- **FR-015**: El sistema MUST distinguir "sin liquidación registrada" de "liquidación registrada en
  cero".
- **FR-016**: Ninguna cifra de la liquidación MUST sumarse ni restarse a un total de ventas, a un
  corte de caja ni a un arqueo.
- **FR-017**: Toda cifra agregada que combine ventas y liquidaciones MUST declarar qué incluye y qué
  excluye, y esa clasificación MUST dejar un test que falle **nombrando el concepto que se duplicó**.
- **FR-018**: El sistema MUST rechazar como entrada inválida —no como error del servidor— una comisión
  negativa, una tasa fuera de rango, un descuento financiado por la plataforma mayor que el descuento
  total, o cualquier importe fuera de las cotas de dinero del sistema. MUST aceptar un neto negativo.

**Alcance y permisos**

- **FR-019**: Capturar y corregir el identificador MUST estar permitido a quien captura pedidos.
  Registrar una liquidación MUST requerir el rol de administración: es dinero que no pasó por la caja.
- **FR-020**: La autorización MUST resolverse en el servidor. La pantalla es espejo, nunca la barrera.
- **FR-021**: Esta feature MUST NOT calcular ni proponer precios de venta por plataforma.
- **FR-022**: Esta feature MUST NOT leer archivos de reporte de plataforma, ni conectarse a ninguna
  API, ni importar el histórico del sistema anterior.

### Key Entities

- **Identificador de plataforma del pedido**: atributo del pedido. Texto libre, opcional, único por
  empresa y plataforma. Es la llave con la que un pedido del POS se encuentra en el documento de pago
  de la plataforma. Se guarda **completo**: el código corto de cinco caracteres que ve el personal se
  deriva del identificador completo, pero del corto no se reconstruye el completo.
- **Liquidación del pedido de plataforma**: uno a uno con el pedido, creada **después** de la venta y
  por otro camino. Su existencia es lo que distingue un pedido cuyo dinero ya se conoce de uno cuyo
  dinero sigue siendo provisional; por eso el estado del dinero **no es una columna**, es la presencia
  o ausencia de esta fila. Guarda importe reportado, comisión (monto y tasa), descuento por origen de
  financiamiento, retenciones, neto, referencia del depósito y de qué documento salió.
- **Documento de pago de la plataforma**: se referencia por su identificador, **no se modela aquí**.
  Modelarlo es la feature de conciliación.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: De los pedidos de plataforma capturados después de la entrega, el 100% tiene
  identificador **o** aparece en la lista de pendientes. Cero pedidos en los que sea imposible saber
  si falta.
- **SC-002**: Teniendo a la vista un documento de pago, cualquier renglón se localiza en el POS en un
  solo paso, sin comparar montos ni fechas.
- **SC-003**: Capturar un pedido de plataforma cuesta **un toque más** que hoy, más lo que tarde
  teclear el identificador. Ninguna acción existente pierde toques ni cambia de lugar.
- **SC-004**: Registrar la liquidación de un pedido desde el documento en la mano toma menos de 30
  segundos.
- **SC-005**: Antes y después de esta feature, los totales de ventas, los cortes de caja y los arqueos
  ya cerrados devuelven **exactamente los mismos importes**, verificado contra una copia de los datos
  de producción.
- **SC-006**: Para un periodo con documentos capturados, el sistema responde tres cifras distintas y
  no confundibles: cuánto se vendió, cuánto se quedó la plataforma y cuánto llegó al banco.
- **SC-007**: A 1024×600, la pantalla de captura de un pedido de plataforma sigue mostrando al menos
  la misma cantidad de producto que hoy menos un renglón, medido y declarado.
- **SC-008**: Ninguna consulta de la pantalla de Ventas se vuelve más lenta de forma perceptible al
  agregar el filtro y la columna nuevos, medido sobre el volumen real de producción.

## Assumptions

Decisiones tomadas al escribir el spec, con su porqué. Cada una es reversible salvo donde se dice.

1. **El identificador se pide al capturar el pedido, no al cobrarlo.** Es el único momento en que el
   operador lo tiene enfrente en la tablet de la plataforma; al cobrar, esa pantalla ya se movió. Y
   pedirlo en la captura **no bloquea un cobro en hora pico**, que era el riesgo real.
2. **Es obligatorio con una salida explícita**, no opcional. Un campo opcional no se llena y la feature
   se vuelve decorativa; un campo obligatorio sin salida detiene la operación cuando el dato de verdad
   no está. La salida explícita deja la falta **registrada**, que es lo que la US-2 aprovecha.
3. **Vive dentro del bloque que el selector de plataforma ya ocupa** cuando hay una lista activa, y
   desaparece con él. Ese bloque ya existe y ya se muestra solo en pedidos de plataforma; meterlo ahí
   no abre una zona nueva de pantalla.
4. **La única normalización es recortar los espacios de los extremos.** Sin ella, un identificador
   pegado con un espacio se ve distinto de sí mismo y rompe la unicidad y el match. Cualquier otra
   transformación —mayúsculas, guiones, longitud— destruiría el valor que sirve para conciliar.
5. **La liquidación va en su propia entidad, no en columnas del pedido.** Tres razones: se crea en otro
   momento y por otro camino; su presencia **es** el estado del dinero, y con columnas nulas
   "provisional" y "capturado mal" se verían igual; y el pedido de mostrador —la gran mayoría— nunca
   la tendría.
6. **No se construye la entidad del depósito ni la del documento de pago.** La referencia del depósito
   se guarda como texto dentro de la liquidación porque **expira** (Rappi conserva 3 meses, Uber 31
   días); el resto de esa estructura se agrega después al mismo costo, y hoy no tiene quién la llene.
7. **No se estima ninguna comisión al capturar.** Un número estimado que se parece a uno medido
   termina sumándose a algo. La tasa configurada por plataforma es material para **proponer precios**,
   que es otra feature, no para afirmar lo que la plataforma cobró.
8. **El histórico de agosto de 2026 que vive en el respaldo de FUDO queda fuera.** Ese histórico va a
   una tabla propia y nunca a los pedidos, por tres restricciones duras del esquema; es su propia
   feature.
9. **La liquidación la captura quien administra, el identificador lo captura quien vende.** Son dos
   momentos, dos fuentes y dos niveles de riesgo distintos.
10. **La captura manual es el destino de hoy, no un puente.** El acceso por API está cerrado para este
    negocio y no depende del equipo; si algún día se abre, lo único que cambia es **quién llena** estos
    mismos campos.

## Puertas que este spec deja abiertas, y una que no cierra

Aplicando el principio VIII: lo que se puede agregar después al mismo costo no se construye hoy; lo
que después costaría una migración sobre datos vivos, se deja preparado.

| Puerta | Cómo queda |
|---|---|
| **Conciliar el depósito contra los pedidos que lo formaron** | **Se cruza**: el identificador y la referencia del depósito son exactamente lo que faltaba. Deja de ser puerta y pasa a ser feature |
| **Cuánto deja cada plataforma** | **Se cruza**: la liquidación por pedido es el registro que no existía |
| **Promociones de plataforma** | Queda abierta y mejor: el descuento se guarda **por origen de financiamiento**, que es el dato que Rappi cambia por campaña |
| **Precio sugerido por plataforma** | Queda abierta y alimentada: con liquidaciones reales, la siguiente feature puede proponer precio con la tasa **medida** en vez de la configurada |
| **Conectarse por API** | Queda abierta: los campos que llenaría un conector son los mismos que hoy llena una persona |
| **Lista de productos por plataforma** | Sin tocar. Este spec no asume que un producto del catálogo sea un producto de la plataforma |

**La que este spec NO cierra pero tampoco resuelve, dicha por su nombre**: un pedido de plataforma
sigue exigiendo un turno de caja abierto, porque el cobro cuelga de la sesión y el folio interno se
numera dentro del turno. Un pedido que entrara a las 23:50 sin turno abierto **no tendría dónde
aterrizar**. Hoy la captura manual lo esconde, porque siempre hay una persona capturando dentro de un
turno. Se declara aquí para que no se descubra el día que un conector empiece a meter pedidos solo:
resolverlo entonces sería una migración sobre datos vivos.

## Fuera de alcance

- Proponer o calcular precios de venta por plataforma.
- Leer, importar o interpretar archivos de reporte de las plataformas.
- Cualquier conexión con una API, webhook, autenticación o sincronización de menú.
- Importar el histórico del sistema anterior.
- Modelar el depósito o el documento de pago como entidades propias.
- Cambiar el sobreprecio configurado de las plataformas.
