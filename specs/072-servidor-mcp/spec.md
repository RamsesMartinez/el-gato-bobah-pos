# Feature Specification: Servidor MCP del POS para operar el negocio desde Claude

**Feature Branch**: `072-servidor-mcp`

**Created**: 2026-11-10

**Status**: Draft

**Input**: Issue #153 y encargo del dueño (2026-11-10): «que las sesiones de Claude del dueño
operen el negocio (consultas, gastos, inventario, pedidos, caja, precios) sin pantalla ni SQL
directo, que salta las validaciones».

## Contexto

Hoy, para registrar o consultar lo del día a día hay dos caminos: la pantalla del POS en la tableta,
o SQL directo contra la base de producción. El primero cuesta toques y estar frente a la tableta; en
octubre varias compras confirmadas no se capturaron por esa fricción. El segundo salta todas las
validaciones del sistema (permisos, reglas de caja, bitácora, redondeo del dinero) y es justo lo que
la constitución prohíbe.

Lo que se construye es una **puerta para agentes**: un servidor MCP que expone operaciones del POS
como herramientas, con la misma identidad, los mismos permisos y las mismas reglas que la pantalla.
Un agente no puede hacer nada que la persona dueña del token no pueda hacer desde la tableta.

## Medición (respaldo de producción del 2026-11-10, últimos 30 días)

Se midió sobre la bitácora de auditoría y las tablas de pedidos, gastos, pagos de gasto, movimientos
de caja, movimientos de almacén, conteos, mermas y precios por plataforma. Por la regla de
`AGENTS.md` §1 aquí va el **orden**, no las cifras.

**Frecuencia, de mayor a menor** (por número de eventos y por días distintos en que ocurre):

1. Ventas en mostrador: crear pedido, entregar, cobrar — todos los días. Los pedidos de plataforma
   son una fracción pequeña.
2. Movimientos de almacén por venta (automáticos, no los captura nadie).
3. Entrada y salida de asistencia — casi todos los días.
4. Abrir y cerrar caja — casi todos los días.
5. **Gastos**: capturados dos de cada tres días; la mayoría pagados en efectivo, después tarjeta y
   transferencia. La mitad se captura el mismo día del documento y una parte llega con uno o más
   días de retraso: es el síntoma de la fricción del issue.
6. **Salidas de efectivo de la caja**: dos de cada tres días.
7. Cambios de precio (producto y extra) y de cómo se descuenta un producto: en ráfagas, pocos días
   con muchos cambios cada uno.
8. Ajustes de almacén por conteo: un día de cada tres o cuatro; conteos cerrados en la misma
   proporción.
9. Alta y corrección de insumos (costo, densidad, empaques): en ráfagas de configuración.
10. Mermas: pocos días, varias por día.
11. Precios por plataforma capturados a mano: pocos días.
12. Edición, anulación y pago posterior de gastos; entradas de efectivo; propinas repartidas;
    cancelaciones de pedido: ocasionales.

**Costo en pantalla** (estimado, por recorrido del flujo en la tableta, no medido con cronómetro):
capturar un gasto con su pago es el flujo más largo de los frecuentes (abrir Gastos, nuevo,
proveedor, categoría, importe, fechas, método, caja, guardar: del orden de 10 toques más teclado);
una salida de caja y un cambio de precio son de 5 a 7; consultar ventas o existencias de un rango
son de 3 a 5 más filtros. Contar el almacén es el más largo de todos, pero ocurre con el insumo en
la mano.

## Qué herramientas entran y cuáles no, por área

La vara para decidir: **entra lo que se puede hacer sin estar físicamente frente a la caja, el
cajón o el almacén, y que hoy cuesta abrir la pantalla**. Queda fuera lo que exige presencia (contar
dinero o insumos, el PIN de quien llega, el cliente frente al mostrador) y lo que reparte poder
(usuarios, roles, credenciales de plataforma).

| Área (orden de frecuencia) | Entra | Queda fuera | Por qué |
| --- | --- | --- | --- |
| Ventas y pedidos | Leer: resumen de ventas de un rango, lista de ventas, un pedido con sus renglones, productos vendidos | Crear, cobrar, entregar, cancelar o devolver pedidos | Un pedido se levanta y se cobra con el cliente enfrente y el dinero en el cajón; un agente no tiene ni lo uno ni lo otro. Cancelar y devolver mueven dinero del corte |
| Asistencia | Leer: reporte de asistencia | Marcar entrada y salida, corregir asistencia | Marcar exige el PIN de quien llega, en persona. Corregir mueve horas pagadas y ocurre poco |
| Caja y cierre | Leer: cajas, conceptos de salida, turno abierto de una caja y su esperado, historial de cortes. Escribir: salida y entrada de efectivo | Abrir y cerrar caja, repartir propinas, traspasos, corregir salidas | Abrir y cerrar exige contar el cajón. La salida de efectivo es frecuente y se olvida; registrarla al momento desde el teléfono es el caso |
| **Gastos y sus pagos** | Leer: gastos, un gasto, categorías, proveedores, métodos de pago. Escribir: capturar gasto (con o sin pago), agregar un pago, anular, dar de alta proveedor | Editar un gasto ya guardado, mercancía del gasto al almacén, leer un ticket por foto | Es el criterio de aceptación del issue. Editar es poco frecuente y sensible (mueve importes ya cortados); la mercancía exige escoger insumo y empaque, que se decide mejor en pantalla |
| Inventario | Leer: existencias, movimientos, motivos de merma. Escribir: registrar merma, ajuste de existencia | Conteos (abrir, capturar, cerrar), recibir compras, configurar insumos, unir insumos | El conteo se hace con el insumo en la mano y cerrar reemplaza lo que el sistema creía tener. La configuración de insumos llega en ráfagas y es de pantalla |
| Catálogo y precios | Leer: productos, extras. Escribir: precio y activo de un producto, precio y activo de un extra, precio por plataforma (poner y quitar) | Alta de productos y grupos, recetas, emparejamiento con plataformas, credenciales | Cambiar precios es lo que ocurre en ráfaga y el criterio de aceptación pide uno. Lo demás es configuración pesada o reparte acceso a terceros |
| Reportes | Leer: resumen de ventas, productos vendidos | Exportar CSV/XLSX | La exportación es el issue #154 |
| Personal, roles y bitácora | Leer: bitácora de auditoría | Usuarios, roles, permisos, PIN y contraseñas | Un token que pudiera crear usuarios o darse permisos sería una escalera hacia arriba |

## Decisiones tomadas en este spec (recomendación, sin preguntar)

| Duda | Decisión | Por qué |
| --- | --- | --- |
| ¿De quién es un token? | De la persona que lo crea, siempre. Nadie crea un token a nombre de otro | «Token personal de un usuario real»: si un admin pudiera crear tokens ajenos, la bitácora diría que hizo algo alguien que no lo hizo |
| ¿Quién crea tokens? | Quien tiene el permiso nuevo «Administrar integraciones»; por omisión, solo el administrador | Es una llave que opera el negocio sin pantalla |
| ¿Quién los revoca? | Cualquiera con «Administrar integraciones» de la misma empresa, el de cualquier persona | Quitar el acceso tiene que poder hacerlo quien lo detecta. Si a alguien le quitan el permiso, sus tokens no se apagan solos (siguen limitados por sus otros permisos): los revoca quien administra |
| Expiración | Obligatoria: 30, 90 (por omisión) o 180 días | Un token sin fecha es una llave que nadie recuerda que existe |
| Alcances | `read` siempre incluido; escritura por área: gastos, caja, inventario, catálogo | Lectura por defecto, escritura por área (encargo del dueño) |
| ¿Una escritura sin vista previa? | La vista previa es lo que pasa **si no se dice lo contrario**: aplicar exige pedirlo explícitamente | Así un agente que se equivoca de argumentos ve el resultado antes de que exista |
| Idempotencia | Toda escritura que se aplica exige una clave; la misma clave con los mismos argumentos devuelve el resultado anterior sin repetir; con argumentos distintos, error | Un agente reintenta cuando se corta la red; dos gastos iguales son dinero que no salió |
| ¿Se registran las lecturas en la bitácora? | Sí, cada llamada, lectura o escritura | El encargo dice «cada llamada»; leer ventas también es acceso a datos del negocio |
| Límite de llamadas | Por token, del mismo orden que el de la pantalla para escrituras sensibles | Un agente en bucle no debe poder vaciar la base a lecturas ni duplicar escrituras |
| Dinero en las herramientas | Se acepta y se devuelve como texto decimal con dos decimales; nunca como número de punto flotante | Issue #153, «Dinero como texto» |
| Fechas | Días del negocio en la zona del negocio (`America/Mexico_City`), en formato `AAAA-MM-DD` | Lo mismo que la pantalla de Ventas |

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Capturar un gasto desde Claude (Priority: P1)

El dueño, desde una sesión de Claude, dice «registra la compra de hoy en el súper, $845.50 en
efectivo de la caja principal, categoría insumos». El agente consulta proveedores, categorías,
métodos de pago y cajas, arma la captura, muestra la vista previa (qué gasto, cuánto, de qué caja
sale el efectivo) y, al confirmar el dueño, la aplica. El gasto aparece en la pantalla de Gastos, la
salida de efectivo en el turno abierto de esa caja, y la bitácora dice quién, con qué token y desde
qué herramienta.

**Why this priority**: Es la fricción que originó el issue y el primer renglón del criterio de
aceptación.

**Independent Test**: Con un token de alcance `read` + gastos, capturar un gasto con tarjeta y uno
en efectivo; verificar en la pantalla de Gastos y en el turno de caja, y en la bitácora.

**Acceptance Scenarios**:

1. **Given** un token con alcance de gastos de una persona con permiso de capturar gastos, **When**
   el agente pide capturar un gasto sin pedir aplicarlo, **Then** recibe la vista previa con el
   gasto resuelto (proveedor, categoría, método, caja por nombre) y no se guarda nada.
2. **Given** la misma llamada pidiendo aplicarla con una clave nueva, **When** se ejecuta, **Then**
   el gasto queda guardado con las mismas reglas que la pantalla (el pago en efectivo genera su
   salida de caja) y la respuesta trae su identificador.
3. **Given** que la red se cortó y el agente repite la llamada con la misma clave y los mismos
   argumentos, **When** se ejecuta, **Then** devuelve el mismo resultado y no hay un segundo gasto.
4. **Given** un pago en efectivo sin caja abierta, **When** se aplica, **Then** el error dice que la
   caja está cerrada y que se abra o se elija otra caja, y no se guarda nada.

---

### User Story 2 - Crear y revocar tokens (Priority: P1)

El administrador entra a una pantalla nueva, «Integraciones», crea un token para sí mismo con nombre
(«Claude de la laptop»), alcances y vigencia, y lo ve **una sola vez** junto con el comando exacto
para conectarlo desde Claude Code. Después ve la lista de tokens de la empresa (de quién, alcances,
cuándo vence, último uso) y puede revocar cualquiera.

**Why this priority**: Sin token no hay puerta; sin revocación, un token filtrado no tiene salida.

**Independent Test**: Crear un token en la pantalla, conectarlo, revocarlo y comprobar que la
siguiente llamada se rechaza.

**Acceptance Scenarios**:

1. **Given** un usuario con «Administrar integraciones», **When** crea un token, **Then** lo ve
   completo una vez, con el comando para conectarlo, y después la lista solo muestra su inicio.
2. **Given** un usuario sin ese permiso, **When** intenta abrir la pantalla o crear un token por la
   API, **Then** se le niega con el mensaje de permiso, y la opción no aparece en el menú.
3. **Given** un token revocado o vencido, **When** el agente lo usa, **Then** la llamada se rechaza
   como no autenticada, sin decir si el token existió.
4. **Given** que se da de baja a la persona dueña del token, **When** el agente lo usa, **Then** se
   rechaza igual que uno revocado.

---

### User Story 3 - Cambiar precios desde Claude (Priority: P2)

El dueño dice «sube el Taro grande a $75 y quita el precio especial de Rappi del Matcha». El agente
busca los productos, muestra antes y después, y al confirmar aplica. Las tabletas ven el menú nuevo
igual que cuando se cambia desde la pantalla.

**Why this priority**: Tercer renglón del criterio de aceptación; los cambios de precio llegan en
ráfagas.

**Independent Test**: Con alcance de catálogo, cambiar el precio de un producto y verificarlo en la
pantalla de productos y en el menú del POS.

**Acceptance Scenarios**:

1. **Given** un token con alcance de catálogo de una persona con permiso de configurar el menú,
   **When** pide cambiar el precio con vista previa, **Then** ve el precio actual y el nuevo.
2. **Given** la misma persona sin el permiso de configurar el menú (se lo quitaron ayer al rol),
   **When** aplica, **Then** se niega aunque el token tenga el alcance: puede lo que den a la vez
   alcance y permiso, leídos en cada llamada.

---

### User Story 4 - Consultar el negocio (Priority: P2)

El dueño pregunta «¿cuánto vendimos ayer por plataforma?», «¿qué insumos están en negativo?»,
«¿qué salió de la caja hoy?». El agente responde con herramientas de solo lectura, con salidas
paginadas y compactas.

**Why this priority**: Es el uso más frecuente esperado y el de menor riesgo.

**Independent Test**: Con un token de solo lectura, consultar ventas de un rango, existencias y el
turno abierto; y comprobar que cualquier escritura se niega.

**Acceptance Scenarios**:

1. **Given** un token de solo lectura, **When** pide aplicar cualquier escritura, **Then** se niega
   nombrando el alcance que falta y cómo conseguirlo.
2. **Given** un rango de fechas mal escrito, **When** consulta ventas, **Then** el error dice cuál
   campo y en qué formato va, sin caer a un rango por omisión.

---

### User Story 5 - Caja e inventario desde Claude (Priority: P3)

Registrar una salida de efectivo («$200 de gas, caja principal»), una merma («se echó a perder 1 L
de leche») o un ajuste de existencia, con vista previa y la misma bitácora.

**Why this priority**: Frecuentes pero de menor fricción que el gasto.

**Independent Test**: Con alcances de caja e inventario, aplicar una salida y una merma y verlas en
el turno y en la lista de mermas.

**Acceptance Scenarios**:

1. **Given** una salida mayor que el efectivo esperado, **When** se aplica, **Then** ocurre lo
   mismo que en la pantalla (mismo rechazo o misma advertencia).
2. **Given** una merma que pasa el tope de la empresa, **When** se aplica, **Then** queda pendiente
   de aprobación igual que en la pantalla.

### Edge Cases

- **Token de otra empresa**: nunca ve ni toca datos de la empresa de este token, aunque pase el
  identificador de un gasto o producto ajeno: la respuesta es «no existe».
- **La empresa en un argumento**: ninguna herramienta acepta empresa ni sucursal como argumento que
  cambie de quién son los datos; la empresa sale del token.
- **Token con alcance que el rol ya no tiene**: el alcance no da permisos; la llamada se niega con el
  mensaje del permiso que falta.
- **Rol que pierde un permiso a media sesión**: la siguiente llamada ya lo ve (se lee en cada
  llamada, no al crear el token).
- **Clave de idempotencia repetida con argumentos distintos**: error que lo dice, sin aplicar nada.
- **Clave repetida mientras la primera sigue en curso o terminó en un estado desconocido**: error
  que pide revisar antes de reintentar, sin aplicar dos veces.
- **Vista previa de algo que ya no existe** (producto archivado entre la vista previa y la
  aplicación): la aplicación falla con la causa; la vista previa no garantiza el resultado.
- **Argumentos absurdos** (importe negativo, NaN, cantidades enormes, textos gigantes): rechazados
  como error de validación con la causa, no como error interno.
- **Ráfaga de llamadas de un token**: al pasar el límite, error que dice cuánto esperar.
- **Token mostrado una sola vez y perdido**: no se recupera; se revoca y se crea otro.
- **Último uso**: la lista de tokens dice cuándo se usó cada uno por última vez, para detectar uno
  que nadie reconoce.
- **Token pegado en un log o en un chat**: el sistema nunca lo escribe en sus registros; la
  bitácora guarda qué token por su identificador y nombre, no el valor.

## Requirements *(mandatory)*

### Functional Requirements

**Identidad y acceso**

- **FR-001**: El sistema MUST permitir a un usuario con el permiso «Administrar integraciones» crear
  tokens de acceso personal **para sí mismo**, con nombre, alcances y vigencia (30, 90 o 180 días).
- **FR-002**: El valor del token MUST mostrarse una sola vez al crearlo; el sistema solo guarda una
  huella irreversible y un prefijo visible para reconocerlo.
- **FR-003**: Todo token MUST incluir el alcance de lectura; los de escritura son por área: gastos,
  caja, inventario y catálogo.
- **FR-004**: Una llamada MUST rechazarse como no autenticada si el token no existe, está revocado,
  venció, o su dueño está dado de baja; el rechazo no distingue entre esos casos.
- **FR-005**: Lo que una llamada puede hacer MUST ser lo que permiten a la vez los alcances del token
  y los permisos del rol del dueño en ese momento, leídos de la base en cada llamada.
- **FR-006**: La empresa de cada llamada MUST salir del token; ninguna herramienta acepta un
  argumento que la cambie.
- **FR-007**: Cualquier usuario de la empresa con «Administrar integraciones» MUST poder revocar
  cualquier token de esa empresa; la revocación surte efecto en la siguiente llamada.
- **FR-008**: El sistema MUST limitar las llamadas por token y por minuto, y al pasarse responder con
  cuánto esperar.
- **FR-009**: El permiso «Administrar integraciones» MUST existir como capacidad nueva del catálogo,
  dada por omisión solo al rol administrador, y heredarse como cualquier permiso nuevo.

**Herramientas**

- **FR-010**: El sistema MUST exponer las herramientas de la tabla «Qué herramientas entran», cada
  una con su alcance y el permiso que ya exige la misma operación en la pantalla.
- **FR-011**: Cada herramienta MUST pasar por las mismas reglas, validaciones y efectos que la
  operación equivalente de la pantalla (bitácora del negocio, salida de caja de un gasto en
  efectivo, aviso de menú cambiado a las tabletas, eventos de seguridad).
- **FR-012**: La lista de herramientas que ve un agente MUST limitarse a las que su token y su rol
  permiten en ese momento.
- **FR-013**: Toda escritura MUST devolver una vista previa salvo que se pida aplicarla de forma
  explícita; la vista previa no cambia nada.
- **FR-014**: Toda escritura que se aplica MUST exigir una clave de idempotencia; la misma clave con
  los mismos argumentos devuelve el resultado anterior, con argumentos distintos se rechaza, y una
  clave cuya primera ejecución no terminó se rechaza pidiendo revisar.
- **FR-015**: Los errores MUST decir la causa y cómo corregirla, en español, sin detalles internos.
- **FR-016**: El dinero MUST entrar y salir como texto decimal; las fechas como días del negocio.
- **FR-017**: Las lecturas que devuelven listas MUST paginarse con un tope por página.

**Rastro**

- **FR-018**: Cada llamada MUST quedar en la bitácora de auditoría de la empresa con la persona, el
  token (identificador y nombre), la herramienta, si fue vista previa o aplicación, el resultado y,
  en las escrituras, el antes y el después.
- **FR-019**: Los eventos que la operación ya deja en la bitácora MUST indicar que vinieron por un
  agente y con qué token.
- **FR-020**: El valor de un token MUST no escribirse nunca en los registros del servidor ni en la
  bitácora; los argumentos de una llamada siguen la misma regla que el cuerpo de una petición de la
  pantalla (fuera de los registros salvo nivel de depuración o error interno). La creación y la
  revocación de un token quedan en la bitácora.

**Pantalla**

- **FR-021**: Una pantalla «Integraciones», visible solo con «Administrar integraciones», MUST
  permitir crear un token (nombre, alcances, vigencia), verlo una vez con el comando para conectarlo,
  listar los tokens de la empresa y revocarlos, con las reglas de tableta (controles de 44 px, sin
  selectores nativos, a 1024×600).

### Key Entities

- **Token de acceso personal**: pertenece a una persona y a su empresa; tiene nombre, prefijo visible,
  huella, alcances, fecha de creación, de vencimiento, de revocación (y quién) y de último uso.
- **Clave de idempotencia**: la usa un token para una herramienta; guarda la huella de los argumentos,
  el estado (en curso, terminada) y el resultado.
- **Llamada de herramienta** (en la bitácora existente): quién, token, herramienta, vista previa o
  aplicación, resultado, antes y después.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: El criterio de aceptación del issue se cumple de punta a punta contra la API local:
  un gasto con tarjeta, uno en efectivo y un cambio de precio capturados desde un cliente MCP real,
  cada uno con su rastro en la bitácora, sin tocar la pantalla ni la base.
- **SC-002**: Un token de solo lectura no logra ninguna escritura; uno revocado no entra; uno de otra
  empresa no ve nada — cada caso con su prueba automática.
- **SC-003**: Repetir una escritura con la misma clave diez veces deja exactamente un registro.
- **SC-004**: Crear un token y conectarlo desde Claude Code toma menos de dos minutos siguiendo solo
  lo que la pantalla muestra.
- **SC-005**: Ninguna herramienta permite algo que la misma persona no pueda hacer en la pantalla
  (prueba: por cada herramienta de escritura, un usuario sin el permiso es rechazado).

## Assumptions

- El cliente principal es Claude Code del dueño; cualquier cliente MCP que hable el transporte HTTP
  estándar sirve igual.
- La conexión es por internet hacia la misma API de producción; no hay red privada.
- La pantalla de Integraciones se usa poco (crear un token cada pocos meses), pero vive en tableta
  como todo el producto.
- La exportación a CSV/XLSX (#154) queda fuera.
- Lo que el agente lee (bitácora con IP y aparato, asistencia del personal) viaja al proveedor del
  modelo que usa el dueño. Es el mismo dato que el admin ve en pantalla; se acepta y se dice aquí.
- Un token no se apaga si se desactiva la empresa ni si a su dueño le resetean la contraseña: se
  revoca en Integraciones.
- Crear un token pide la contraseña actual de quien lo crea.
- Los nombres de herramienta van en inglés (identificadores), sus descripciones en español.
