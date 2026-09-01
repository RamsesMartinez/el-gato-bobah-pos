# Feature Specification: Conteo de efectivo por denominaciones

**Feature Branch**: `003-arqueo-denominaciones`

**Created**: 2026-08-31

**Status**: Listo para plan

**Input**: Conteo de efectivo por denominaciones en la apertura y el cierre de caja, para que el operador no tenga que sumar el cajón a mano.

## User Scenarios & Testing *(mandatory)*

### User Story 1 — Contar el fondo al abrir la caja (Priority: P1)

Quien abre el turno vacía el cajón, separa el dinero por denominación y captura cuántas piezas hay de cada una: seis monedas de $10, tres billetes de $50, y así. El sistema suma y muestra el fondo. El operador confirma y la caja queda abierta con esa cifra.

**Why this priority**: Es la mitad más simple del problema —una sola cifra, sin esperado contra el cual comparar— y ya entrega el valor completo: se acabó la libreta. Si solo se implementa esto, abrir la caja deja de depender de una suma manual, y la pantalla de cierre sigue funcionando como hoy.

**Independent Test**: Abrir un turno capturando piezas y verificar que el fondo registrado es la suma exacta de piezas × valor, sin que el operador escriba ningún total.

**Acceptance Scenarios**:

1. **Given** la caja está cerrada, **When** el operador captura 6 monedas de $10 y 3 billetes de $50, **Then** el sistema muestra un fondo de $210 y abre el turno con esa cifra.
2. **Given** el operador está capturando el conteo, **When** corrige una cantidad, **Then** el total se actualiza sin que tenga que recalcular nada.
3. **Given** el operador no captura ninguna pieza, **When** intenta abrir, **Then** el sistema abre con fondo de $0 sin tratarlo como error — una caja puede arrancar vacía.

---

### User Story 2 — Contar el efectivo al cerrar la caja (Priority: P1)

Al cerrar el turno, el operador cuenta el cajón por denominaciones igual que al abrir. El sistema suma y usa esa cifra como el efectivo declarado, la compara contra el esperado y muestra la diferencia. Los demás métodos de pago —tarjeta, transferencia, plataformas— se siguen declarando con una sola cifra.

**Why this priority**: Es donde el error cuesta dinero. Un faltante inexplicable nace casi siempre de una suma mal hecha, no de dinero perdido; quitar la suma manual quita la causa. Va en P1 junto a la apertura porque el operador que aprende a contar por denominaciones al abrir espera lo mismo al cerrar, y tener la mitad es más confuso que no tener nada.

**Independent Test**: Cerrar un turno con ventas conocidas, capturar piezas que sumen exactamente lo esperado, y verificar que la diferencia queda en cero sin haber escrito ningún total.

**Acceptance Scenarios**:

1. **Given** el turno espera $1,340 en efectivo, **When** el operador captura piezas que suman $1,340, **Then** la diferencia mostrada es $0.
2. **Given** el operador captura piezas que suman $1,290, **When** revisa el resumen antes de cerrar, **Then** ve un faltante de $50 **antes** de confirmar el cierre, no después.
3. **Given** hay métodos que se declaran solos, **When** el operador llega al cierre, **Then** solo se le pide contar el efectivo; los demás no le piden nada.

---

### User Story 3 — Explicar una diferencia después del cierre (Priority: P2)

Cuando un corte cerró con faltante, quien lo revisa al día siguiente abre el arqueo y ve el desglose que se capturó: cuántas piezas de cada denominación declaró el operador. Con eso puede distinguir un error de conteo —faltan justo dos billetes de $500— de dinero que de verdad no está.

**Why this priority**: No desbloquea la operación diaria, pero es lo que convierte un faltante en algo investigable. Sin el desglose guardado, un corte con diferencia es un número sin historia, que es exactamente el problema que dejó el turno con $1,662 sin explicación.

**Independent Test**: Cerrar un turno con un faltante deliberado, volver a abrir ese corte y verificar que muestra el desglose capturado, no solo el total.

**Acceptance Scenarios**:

1. **Given** un corte cerrado con desglose, **When** se consulta ese corte, **Then** muestra cuántas piezas de cada denominación se declararon.
2. **Given** un corte cerrado ANTES de esta funcionalidad, **When** se consulta, **Then** muestra su total como siempre y no inventa un desglose que nadie capturó.

---

### Edge Cases

- **Una denominación que el negocio no maneja.** Las monedas de 50¢ existen pero un local que redondea nunca las ve. Capturar un cero en ellas todos los días es ruido; ocultarlas y que un día aparezca una es peor.
- **Un conteo que no cuadra con el esperado.** Es el caso normal, no un error: el sistema muestra la diferencia y deja cerrar. Bloquear el cierre por un faltante dejaría al operador sin salida y con la caja abierta toda la noche.
- **El operador corrige un dígito a media captura.** El total tiene que seguirlo sin que él vuelva a sumar; si el total se congela o se recalcula tarde, el operador desconfía y saca la calculadora otra vez, que es justo lo que esto viene a quitar.
- **Un billete de $1000 en un cajón de $400.** El conteo permite capturarlo; quien decide si es un error es el humano al ver la diferencia.
- **La caja se abre en una moneda distinta a la del catálogo de denominaciones.** Hoy todo es MXN, pero el sistema acepta USD y las denominaciones de una no sirven para la otra.
- **Dos personas contando a la vez** en la misma caja: el arqueo es de un turno y un turno tiene un solo cierre, pero la pantalla no debe permitir que un segundo cierre pise el conteo del primero.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: El sistema MUST permitir capturar, al abrir la caja, cuántas piezas hay de cada denominación de efectivo, y calcular el fondo a partir de ellas.
- **FR-002**: El sistema MUST permitir capturar, al cerrar la caja, cuántas piezas hay de cada denominación, y usar esa suma como el efectivo declarado.
- **FR-003**: El sistema MUST calcular el total en el servidor a partir de las PIEZAS. Un total que mande la pantalla se ignora.
- **FR-004**: El sistema MUST mostrar el total acumulado mientras se captura, actualizado en cada cambio, para que el operador nunca tenga que sumar.
- **FR-005**: El sistema MUST mostrar la diferencia contra el esperado ANTES de confirmar el cierre, no solo después.
- **FR-006**: El conteo por denominaciones MUST aplicar únicamente a los métodos de pago en efectivo. Los demás se siguen declarando con una sola cifra.
- **FR-007**: El sistema MUST guardar el desglose capturado —piezas por denominación— junto al arqueo, y mostrarlo al consultar ese corte.
- **FR-008**: El sistema MUST seguir mostrando correctamente los cortes cerrados antes de esta funcionalidad, sin desglose y sin inventarles uno.
- **FR-009**: El sistema MUST rechazar una cantidad de piezas que no sea un entero mayor o igual a cero.
- **FR-010**: El sistema MUST rechazar un conteo cuyo total exceda los topes de dinero del sistema, con un error accionable y no un fallo interno.
- **FR-011**: El catálogo de denominaciones MUST depender de la moneda del turno; las denominaciones de una moneda no se ofrecen para otra.
- **FR-012**: El sistema MUST permitir abrir o cerrar con cero piezas de una denominación sin obligar a capturarla.
- **FR-013**: La captura MUST caber en una pantalla de ~1024×600 sin empujar fuera de vista el resumen del corte.
- **FR-014**: El sistema MUST ofrecer dos caminos EXCLUYENTES para declarar el efectivo: contar por
  denominaciones, o escribir el total directamente. Escribir el total directamente MUST exigir un
  motivo, que se guarda con el arqueo.
- **FR-015**: Los dos caminos MUST NO poder coexistir en un mismo arqueo. Elegir uno descarta lo
  capturado en el otro, y la pantalla MUST advertirlo antes de descartarlo. Así nunca hay dos cifras
  de efectivo compitiendo, que era el caso donde no había forma de saber cuál era la buena.
- **FR-016**: Todo arqueo MUST quedar explicado: o tiene desglose por denominaciones, o tiene el
  motivo de por qué se capturó el total a mano. Nunca una cifra suelta sin ninguna de las dos.

### Key Entities

- **Denominación**: una pieza de dinero de un valor fijo en una moneda — moneda de $10 en MXN, billete de $50 en MXN. Tiene valor, moneda, si es moneda o billete, y un orden para presentarla.
- **Conteo de efectivo**: cuántas piezas de cada denominación se contaron en un momento del turno. Pertenece a un turno y a un momento (apertura o cierre). Su total es derivado, nunca capturado. Un turno puede no tener conteo si se declaró el total a mano; en ese caso tiene un motivo en su lugar.
- **Motivo de captura manual**: por qué este arqueo no se contó por denominaciones. Es lo que hace auditable un corte sin desglose.
- **Turno de caja** *(ya existe)*: gana la relación con sus dos conteos, el de apertura y el de cierre.
- **Total declarado por método** *(ya existe)*: para el efectivo pasa a alimentarse del conteo; para los demás métodos no cambia.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: El operador abre o cierra la caja **sin usar calculadora ni libreta** para sumar el efectivo.
- **SC-002**: El total mostrado durante la captura coincide **siempre** con la suma de piezas × valor; no existe un estado en que el operador vea un total desactualizado.
- **SC-003**: Contar y capturar un cajón típico (menos de 60 piezas) toma **menos de 2 minutos**.
- **SC-004**: Ante un corte con diferencia, quien lo revisa puede ver **cuántas piezas de cada denominación** se declararon —o, si se capturó el total a mano, **por qué**— sin pedirle nada al operador que lo cerró.
- **SC-007**: **Ningún** arqueo queda con una cifra de efectivo sin desglose y sin motivo.
- **SC-005**: Las diferencias de arqueo atribuibles a error de suma bajan a **cero**: una diferencia solo puede venir de dinero que no está o de una pieza mal contada, nunca de una suma.
- **SC-006**: Los cortes cerrados antes de esta funcionalidad se siguen consultando **sin cambios en sus cifras**.

## Assumptions

- Las denominaciones de MXN que se ofrecen son: monedas de 50¢, $1, $2, $5 y $10; billetes de $20, $50, $100, $200, $500 y $1000. Se dejan fuera las de 10¢ y 20¢ (prácticamente no circulan) y la moneda de $20 (existe pero es conmemorativa y rara en caja). Si aparece una, se captura en el campo de total.
- Hoy toda la operación es en MXN. USD existe en el sistema pero ningún negocio lo usa todavía, así que el catálogo arranca con MXN; lo que el spec exige es que agregar otra moneda no obligue a rehacer el modelo.
- El conteo lo hace la misma persona que abre o cierra el turno; no hay un flujo de "un segundo par de ojos" que valide el conteo.
- El esperado contra el que se compara el efectivo lo calcula el sistema como hoy; esta funcionalidad no cambia cómo se llega a esa cifra.
- El arqueo sigue siendo por turno y por caja, como ya lo es hoy: el negocio tiene varias cajas
  (principal, clip, externa) y cada una lleva su propio turno. El conteo por denominaciones aplica a
  cada caja que maneje efectivo, sin cambiar esa estructura.
- Esta funcionalidad no introduce conteos parciales a media jornada.
- El cierre sigue bloqueado por pedidos sin entregar, como hoy. Un faltante NO bloquea el cierre.

## Out of Scope

- Ligar el conteo con retiros parciales de efectivo a media jornada.
- Un flujo de doble verificación (que una segunda persona confirme el conteo).
- Denominaciones de monedas distintas a MXN. El modelo debe admitirlas; sembrarlas no es parte de esta entrega.
- Reconciliar automáticamente una diferencia contra los movimientos de caja.
