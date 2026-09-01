# Feature Specification: Bloqueo por inactividad y cambio de operador por PIN

**Feature Branch**: `004-bloqueo-y-cambio-por-pin`

**Created**: 2026-09-01

**Status**: Listo para plan

**Input**: Que la tableta se bloquee sola, que quien vaya a cobrar se identifique con su PIN, y que una sesión no dure más que un turno — para que "quién cobró" signifique algo cuando dos estaciones comparten un cajón.

## User Scenarios & Testing *(mandatory)*

### User Story 1 — Cobrar en la estación que esté libre (Priority: P1)

Hay dos estaciones en el mostrador y un solo cajón. Quien está en la primera se quedó guardando un pedido, así que el siguiente cliente se atiende en la segunda. Quien va a cobrar toca su nombre en la pantalla, teclea su PIN, y cobra. La venta queda a su nombre.

**Why this priority**: Es el motivo de existir de todo esto. Sin identificarse por venta, dos personas cobrando contra el mismo cajón hacen que el desglose por cajero del arqueo mienta — atribuye todo a quien dejó la tableta abierta. Con esto, el desglose que ya muestra el arqueo empieza a significar algo.

**Independent Test**: Con dos personas y una tableta, cobrar una venta con cada una identificándose por PIN, y verificar que el arqueo las separa correctamente.

**Acceptance Scenarios**:

1. **Given** la tableta está bloqueada, **When** una persona toca su nombre y teclea su PIN correcto, **Then** queda como operador activo y puede cobrar.
2. **Given** una persona está activa, **When** cobra una venta, **Then** esa venta queda registrada a su nombre y aparece bajo ella en el arqueo.
3. **Given** una persona teclea un PIN incorrecto, **When** lo intenta varias veces, **Then** el sistema la frena como frena un login fallido, sin decirle si el nombre existe o si solo falló el PIN.

---

### User Story 2 — La tableta se bloquea sola (Priority: P1)

Nadie toca la tableta por unos minutos. Se bloquea y muestra la lista de nombres. Nada de lo capturado se pierde: las cuentas abiertas siguen ahí. Quien vuelva —la misma persona u otra— se identifica y sigue.

**Why this priority**: Sin esto, la historia 1 no aguanta un turno real. Una tableta que queda activa como Ana mientras Ana está en la bodega hace que todo lo que cobre alguien más quede a nombre de Ana, y la responsabilidad se evapora justo cuando se necesita.

**Independent Test**: Dejar la tableta sin tocar el tiempo configurado y verificar que se bloquea, que las cuentas abiertas siguen intactas al desbloquear, y que desbloquear con otro PIN cambia de operador.

**Acceptance Scenarios**:

1. **Given** una tableta activa con una cuenta a medio capturar, **When** pasa el tiempo de inactividad, **Then** se bloquea y la cuenta sigue completa al desbloquear.
2. **Given** una tableta bloqueada, **When** la desbloquea una persona distinta a la que estaba, **Then** el operador activo cambia y las ventas siguientes quedan a nombre de la nueva.
3. **Given** el operador está a media captura, **When** toca la pantalla, **Then** el reloj de inactividad se reinicia y no se bloquea encima de él.

---

### User Story 3 — La sesión no dura más que un turno (Priority: P2)

Una tableta encendida el viernes no sigue autenticada el lunes. Pasadas las horas del turno, la sesión caduca de verdad y hace falta entrar con usuario y contraseña, no solo un PIN.

**Why this priority**: Es la red de seguridad de las dos anteriores. El bloqueo por inactividad protege los minutos; esto protege los días. Hoy una sesión dura 30 días y un usuario puede tener varias vivas a la vez, así que una tableta olvidada es una credencial abierta durante un mes.

**Independent Test**: Con una sesión iniciada, adelantar el reloj más allá del límite y verificar que la tableta exige credenciales completas y no acepta solo el PIN.

**Acceptance Scenarios**:

1. **Given** una sesión iniciada hace más del límite, **When** alguien intenta usar la tableta, **Then** se le pide usuario y contraseña, no un PIN.
2. **Given** una sesión dentro del límite, **When** la tableta lleva rato bloqueada, **Then** basta el PIN para volver.

---

### User Story 4 — Un negocio elige identificarse solo con el PIN (Priority: P3)

Un negocio con mucha rotación decide que tocar el nombre antes del PIN le sobra. Lo cambia en la configuración: a partir de ahí la pantalla de bloqueo solo pide el PIN y el sistema deduce quién es. Para poder activarlo, los PINs de ese negocio deben ser suficientemente largos y no repetirse entre personas.

**Why this priority**: Ahorra un tap por desbloqueo, que en un turno con muchos relevos suma. Va en P3 porque el ahorro es chico y el riesgo no: un PIN mal tecleado que coincida con el de otra persona atribuye la venta a quien no fue, **en silencio**. Solo vale la pena para quien de verdad lo pida.

**Independent Test**: Activar el ajuste en un negocio con PINs válidos y verificar que la pantalla ya no pide elegir nombre; intentar activarlo en un negocio con PINs cortos o repetidos y verificar que el sistema lo impide diciendo cuáles.

**Acceptance Scenarios**:

1. **Given** un negocio con el ajuste apagado, **When** alguien desbloquea, **Then** elige su nombre y luego teclea el PIN.
2. **Given** un negocio con PINs largos y únicos, **When** se enciende el ajuste, **Then** la pantalla de bloqueo pide solo el PIN.
3. **Given** un negocio con dos personas con el mismo PIN, **When** se intenta encender el ajuste, **Then** el sistema lo rechaza nombrando a quiénes hay que cambiarles el PIN.
4. **Given** el ajuste encendido, **When** alguien intenta ponerse un PIN igual al de otra persona, **Then** el sistema lo rechaza.

---

### Por qué FR-007 obliga a recapturar

Se descubrió al implementar, no al diseñar: el PIN se guarda con un algoritmo que **saliniza**, así
que de lo guardado no se puede leer el largo, ni saber si dos personas tienen el mismo, ni averiguar
de quién es uno. Las tres cosas son justo lo que el modo de solo-PIN necesita.

La versión anterior del spec pedía "verificar que los PINs actuales cumplan", y eso **no se puede
hacer** contra lo que ya está guardado. Pedir que se recapturen es lo que pone el texto del PIN
enfrente una vez, que es el único momento en que se puede validar largo y unicidad.

### Edge Cases

- **Se bloquea con el cliente enfrente y una cuenta a medias.** Lo capturado no puede perderse; si se pierde, el operador aprende a no dejar que se bloquee y desactiva la protección de facto.
- **Una persona sin PIN configurado.** Hoy 2 de 8 usuarios activos no tienen. No pueden quedar encerrados fuera del sistema por una funcionalidad que no eligieron.
- **Se enciende el modo y nadie ha recapturado su PIN todavía.** Nadie puede desbloquear con PIN
  hasta hacerlo; todos entran con usuario y contraseña, que sigue funcionando (FR-012).
- **La única persona con acceso olvidó su PIN** a media noche. Tiene que existir una salida que no dependa de que alguien más esté presente.
- **Encender el modo de solo-PIN en un negocio cuyos PINs no cumplen.** Los 6 usuarios con PIN hoy los tienen de 4 dígitos y sin garantía de ser únicos: el cambio no puede dejar el negocio en un estado donde dos personas se desbloqueen la una a la otra.
- **Sesión caducada a medio cobro.** El operador no puede quedarse con el dinero en la mano y la pantalla muerta.
- **Un PIN tecleado muchas veces mal.** Tiene que frenarse como se frena un login, sin revelar si el error fue el nombre o el PIN.
- **La tableta se bloquea mientras imprime un ticket.** La impresión no depende de quién esté activo.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: El sistema MUST bloquear la pantalla tras un periodo de inactividad configurable, sin cerrar la sesión.
- **FR-002**: El bloqueo MUST conservar intacto todo lo capturado: cuentas abiertas, renglones y notas.
- **FR-003**: Desbloquear MUST establecer quién es el operador activo, y las ventas cobradas después MUST quedar a nombre de esa persona.
- **FR-004**: Por default, desbloquear MUST pedir elegir a la persona y después su PIN.
- **FR-005**: El sistema MUST permitir, por negocio, que desbloquear pida SOLO el PIN y deduzca la persona.
- **FR-006**: El modo de solo-PIN MUST exigir PINs de al menos 6 dígitos y ÚNICOS entre las personas activas del negocio.
- **FR-007**: Activar el modo de solo-PIN MUST obligar a que cada persona vuelva a capturar su PIN.
  Los PINs anteriores dejan de servir en el momento del cambio.
- **FR-008**: Con el modo de solo-PIN activo, el sistema MUST rechazar un PIN nuevo que coincida con
  el de otra persona activa del negocio, sin decir de quién es.
- **FR-017**: El sistema MUST poder comparar dos PINs por igualdad y encontrar a quién pertenece uno,
  sin guardar el PIN de forma reversible.
- **FR-018**: Si falta el secreto que hace posible FR-017, el modo de solo-PIN MUST NO poder
  activarse. Nunca se activa en un estado donde no se pueda garantizar la unicidad.
- **FR-009**: Una sesión iniciada MUST caducar tras un periodo del orden de un turno, exigiendo credenciales completas y no solo un PIN.
- **FR-010**: Un intento fallido de desbloqueo MUST frenarse con el mismo control que frena un login fallido, y MUST NOT revelar si falló la persona o el PIN.
- **FR-011**: El sistema MUST ofrecer una salida cuando alguien olvida su PIN, que no dependa de que otra persona esté presente en el local.
- **FR-012**: Una persona sin PIN configurado MUST poder seguir entrando con sus credenciales completas.
- **FR-013**: Las cuentas abiertas MUST pertenecer a la estación y no a la persona: al cambiar de operador siguen ahí.
- **FR-014**: La pantalla de bloqueo MUST caber en ~1024×600 con controles de al menos 44 px, sin desplegables que pinte el sistema operativo.
- **FR-015**: El sistema MUST registrar los desbloqueos fallidos como evento de seguridad, sin guardar el PIN ni datos personales.
- **FR-016**: Los periodos de inactividad y de caducidad MUST ser configurables por negocio, con valores por default sensatos.

### Key Entities

- **Operador activo**: quién está usando la estación en este momento. Es lo que se le atribuye a una venta cobrada. Cambia al desbloquear.
- **Estación**: la tableta. Tiene su propia pantalla de bloqueo y sus propias cuentas abiertas; no pertenece a una persona.
- **PIN** *(ya existe)*: el secreto corto de una persona. Gana reglas de largo y de unicidad cuando el negocio activa el modo de solo-PIN.
- **Ajuste de identificación del negocio**: si desbloquear pide nombre y PIN, o solo PIN. Y cada cuánto se bloquea y cada cuánto caduca la sesión.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Cambiar de operador en una estación toma **menos de 5 segundos**, contra el login completo que toma bastante más.
- **SC-002**: **100%** de las ventas cobradas quedan atribuidas a la persona que estaba activa al cobrar.
- **SC-003**: Ninguna tableta permanece autenticada más allá del periodo de caducidad, medido sobre las sesiones vivas del sistema.
- **SC-004**: Al desbloquear, **nada de lo capturado se pierde** en ningún caso.
- **SC-005**: Un negocio no puede quedar en un estado donde dos personas se desbloqueen mutuamente por tener el mismo PIN.
- **SC-006**: Quien olvida su PIN recupera el acceso **sin esperar a que otra persona llegue al local**.

## Assumptions

- **6 dígitos y no 8** para el modo de solo-PIN. Con 6 dígitos y diez personas, la probabilidad de que un dedazo caiga en el PIN de otro es de 9 en un millón, contra 9 en diez mil con 4 dígitos. Ocho dígitos no compra nada útil sobre seis y sí garantiza que la gente lo escriba en un papel pegado a la tableta, que es peor que un PIN corto.
- El modo por default —elegir persona y luego PIN— **no necesita PINs únicos ni largos**: el nombre ya identifica, y el PIN solo prueba. Por eso el mínimo actual de 4 dígitos se mantiene ahí.
- El negocio tiene pocas personas activas (hoy 8), así que elegir el nombre cabe en una rejilla de una pantalla sin buscador.
- La atribución de una venta es de quien la COBRA, no de quien la capturó. Es lo coherente con el arqueo, donde la responsabilidad es sobre el dinero recibido.
- El cambio de operador por PIN ya existe en el servidor y ninguna pantalla lo usa; esta feature lo conecta, no lo inventa.
- Los periodos por default arrancan en algo del orden de minutos para el bloqueo y de un turno para la caducidad; el valor exacto se ajusta con el negocio y no cambia el diseño.

## Out of Scope

- Cajón de dinero por persona. La caja sigue siendo una y compartida; la responsabilidad se rastrea por quien cobró, que es lo que ya muestra el arqueo.
- Permisos distintos por operador dentro de la misma estación: los roles siguen funcionando como hoy.
- Reconocimiento biométrico o tarjetas de proximidad.
- Bloquear el acceso a pantallas de administración por inactividad con reglas distintas a las del punto de venta.
