# Feature Specification: La caja exige la asistencia de quien la abre

**Feature Branch**: `046-caja-y-asistencia`

**Created**: 2026-10-30

**Status**: Draft

**Input**: Petición del dueño (2026-10-30): ligar la caja con la asistencia de la spec 033.

1. No se puede abrir caja si quien la abre no tiene una entrada registrada vigente.
2. No se puede cerrar caja si la persona que la abrió no ha marcado su salida (por ahora solo ella,
   no todo el personal).

Hoy las dos cosas viven separadas: se puede abrir la caja sin haber checado y cerrarla con quien la
abrió todavía «trabajando» según la asistencia. El turno de caja y el turno de la persona dejan de
coincidir, y el reporte de horas no sirve para responder quién estaba a cargo del dinero.

## Clarifications

### Session 2026-10-30

- Q: Ana abrió la caja y se fue sin marcar su salida; quien cierra no tiene permiso y no hay
  gerente. ¿Qué pasa? → A: Alguien con permiso puede cerrar la caja aunque quien la abrió no haya
  marcado su salida. Es un cierre autorizado: no exige registrar la salida de esa persona; queda en
  la bitácora quién autorizó y el motivo. Sin el permiso no se cierra, y se le dice que lo pida a
  quien sí lo tenga.
- Q: ¿El cierre autorizado deja la entrada de quien abrió como «sin salida»? → A: Sí. Sin eso la
  entrada se queda abierta, y si la persona vuelve a checar antes de 16 horas la pantalla de
  Asistencia le registra una salida en vez de una entrada y le suma horas que no trabajó. Con la
  marca, ese día sale «sin salida» en el reporte hasta que alguien con permiso lo corrija, igual que
  una entrada de más de 16 horas. La marca va en la bitácora junto con el cierre.
- Q: ¿Quien tiene el permiso puede autorizar el cierre de la caja que él mismo abrió sin haber
  marcado su salida? → A: No; si está ahí, marca su salida con su PIN en la misma hoja. La única
  excepción es una entrada de más de 16 horas, que ya no se puede cerrar con PIN.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Abrir caja pide haber marcado la entrada (Priority: P1)

Quien llega a abrir la caja toca «Abrir caja». Si ya marcó su entrada, la apertura sigue como hoy.
Si no la marcó, la hoja de apertura se lo dice antes de contar el fondo y le ofrece marcarla ahí
mismo con su PIN, sin ir a la pantalla de Asistencia; en cuanto la marca, sigue con la apertura.

**Why this priority**: es la mitad de la regla que asegura que todo turno de caja tiene detrás a una
persona presente según la asistencia. Sin ella, la regla del cierre no tiene de qué colgarse.

**Independent Test**: con la asistencia encendida, una persona sin entrada intenta abrir la caja,
marca su entrada desde la misma hoja y la abre; la apertura y la entrada quedan registradas a su
nombre.

**Acceptance Scenarios**:

1. **Given** la asistencia encendida y una persona que lleva asistencia con entrada marcada hace
   2 horas, **When** abre la caja, **Then** la caja se abre como hoy, sin pasos nuevos.
2. **Given** la asistencia encendida y una persona sin entrada abierta, **When** toca «Abrir caja»,
   **Then** la hoja le dice «Para abrir la caja, primero marca tu entrada» antes de pedirle contar
   el fondo, y le ofrece «Marcar entrada».
3. **Given** el caso anterior, **When** marca su entrada con su PIN desde la hoja, **Then** la
   entrada queda registrada como si hubiera checado en la pantalla de Asistencia y la hoja sigue con
   la apertura sin volver a empezar.
4. **Given** una persona cuya única entrada abierta tiene más de 16 horas, **When** intenta abrir
   la caja, **Then** esa entrada no cuenta: se le pide marcar entrada, y al marcarla la vieja queda
   «sin salida» igual que hoy en la pantalla de Asistencia.
5. **Given** una tableta con una versión anterior de la pantalla, o una petición hecha a mano,
   **When** se intenta abrir la caja sin entrada vigente, **Then** el servidor la rechaza con un
   mensaje para quien opera; la caja no se abre.

---

### User Story 2 - Cerrar caja pide que quien la abrió haya marcado su salida (Priority: P1)

Al cierre, antes de contar el cajón, la hoja revisa si quien abrió la caja ya marcó su salida. Si
sí, el cierre sigue como hoy. Si no y esa persona está ahí, la hoja le ofrece marcar su salida con
su PIN sin salir de la hoja, y el cierre continúa.

**Why this priority**: es la otra mitad de la petición. Garantiza que el turno de caja no termina
con su responsable todavía «trabajando» en el reporte de horas.

**Independent Test**: la persona que abrió marca su salida desde la hoja de cierre y la caja se
cierra; en el reporte de asistencia su salida aparece a esa hora.

**Acceptance Scenarios**:

1. **Given** una caja abierta por Ana, que ya marcó su salida, **When** cualquiera con permiso de
   operar la caja la cierra, **Then** el cierre sigue como hoy.
2. **Given** una caja abierta por Ana, con su entrada todavía abierta, **When** se toca «Cerrar
   caja», **Then** la hoja dice «Ana abrió esta caja y no ha marcado su salida» antes de pedir el
   conteo, y ofrece «Marcar salida de Ana».
3. **Given** el caso anterior, **When** Ana teclea su PIN en la hoja, **Then** su salida queda
   registrada igual que en la pantalla de Asistencia y el cierre continúa sin volver a empezar.
4. **Given** una caja abierta por Ana, que marcó su salida a mediodía y volvió a marcar entrada en
   la tarde, **When** se intenta cerrar, **Then** se le pide la salida de la entrada de la tarde:
   lo que cuenta es que al cierre no tenga ninguna entrada abierta.
5. **Given** una tableta con una versión anterior de la pantalla, o una petición hecha a mano,
   **When** se intenta cerrar sin que quien abrió haya marcado su salida, **Then** el servidor lo
   rechaza; la caja sigue abierta y nada del cierre queda a medias.

---

### User Story 3 - Cierre autorizado cuando quien abrió se fue sin marcar su salida (Priority: P2)

Ana abrió la caja, se fue y olvidó marcar su salida; nadie puede teclear su PIN. Quien tiene el
permiso de autorizar ese cierre lo cierra desde la misma hoja: toca «Cerrar sin la salida de Ana»,
escribe el motivo y sigue con el cierre normal. No se inventa una hora de salida: la entrada de Ana
queda «sin salida», y la bitácora guarda quién autorizó, de quién era la salida pendiente y el
motivo. Quien no tiene el permiso no
puede cerrar en ese caso, y la hoja le dice que se lo pida a alguien que sí lo tenga.

**Why this priority**: sin este camino, un olvido deja la caja sin poder cerrarse; pero ocurre
menos que el caso normal y depende de que existan las dos historias anteriores.

**Independent Test**: con Ana fuera y su entrada abierta, un gerente cierra la caja con motivo; el
turno queda cerrado, la entrada de Ana aparece «sin salida» en el reporte y la bitácora muestra el cierre autorizado
con el gerente como autor y el motivo.

**Acceptance Scenarios**:

1. **Given** quien cierra tiene el permiso de autorizar el cierre sin la salida de quien abrió,
   **When** toca «Cerrar sin la salida de Ana» y escribe el motivo, **Then** el cierre sigue como
   hoy y la bitácora del cierre guarda quién autorizó, a quién se le pasó por alto la salida y el
   motivo.
2. **Given** el caso anterior sin motivo, **When** intenta seguir, **Then** se le pide el motivo y
   la caja no se cierra.
3. **Given** quien cierra sin ese permiso y Ana fuera, **When** toca «Cerrar caja», **Then** la hoja
   dice «Ana abrió esta caja y no ha marcado su salida. Pide a alguien con permiso que cierre la
   caja» y no ofrece seguir; el servidor rechaza el cierre aunque la petición llegue.
4. **Given** una entrada de Ana con más de 16 horas, **When** se intenta cerrar, **Then** no se
   ofrece «Marcar salida de Ana» con PIN (marcar ahí registraría una entrada nueva); solo el cierre
   autorizado.
5. **Given** un cierre autorizado, **When** después se revisa la asistencia de Ana, **Then** esa
   entrada aparece «sin salida» y no suma horas hasta que alguien con permiso capture su salida por
   el camino de la spec 033.
6. **Given** un cierre autorizado y Ana volviendo al día siguiente antes de que pasen 16 horas desde
   su entrada, **When** checa en la pantalla de Asistencia, **Then** se le registra una entrada
   nueva, no una salida.
7. **Given** la caja la abrió la misma persona que tiene el permiso y su entrada tiene 16 horas o
   menos, **When** intenta cerrar sin haber marcado su salida, **Then** no se le ofrece el cierre
   autorizado: se le pide marcar su salida con su PIN en la hoja, y el servidor rechaza el cierre
   autorizado aunque la petición llegue.
8. **Given** la misma persona con el permiso y una entrada de más de 16 horas, **When** cierra,
   **Then** puede usar el cierre autorizado con motivo, y su entrada queda «sin salida».

---

### User Story 4 - Negocios y personas sin asistencia (Priority: P1)

Un negocio con la asistencia apagada, o una persona con «Lleva asistencia» apagado (el dueño, por
ejemplo), abre y cierra caja exactamente como hoy.

**Why this priority**: la regla no puede romper la operación de quien no usa asistencia; un negocio
nuevo la tiene apagada por omisión.

**Independent Test**: con la asistencia apagada, abrir y cerrar la caja sin ninguna entrada; con la
asistencia encendida, lo mismo con una persona que no lleva asistencia.

**Acceptance Scenarios**:

1. **Given** la asistencia apagada en el negocio, **When** cualquiera abre o cierra la caja,
   **Then** no se pide entrada ni salida.
2. **Given** la asistencia encendida y una persona con «Lleva asistencia» apagado, **When** abre la
   caja, **Then** no se le pide entrada; y al cierre no se pide su salida.
3. **Given** una caja abierta con la asistencia apagada, **When** se enciende antes del cierre,
   **Then** al cierre se aplica la regla según el estado de ese momento: si quien abrió tiene una
   entrada abierta, se pide su salida; si no tiene ninguna, se cierra.

---

### Edge Cases

- **Dos tabletas a la vez**: Ana marca su salida en la pantalla de Asistencia mientras otra tableta
  muestra la hoja de cierre pidiéndola. Al confirmar, la hoja ve que ya está y sigue; no registra
  una segunda salida ni falla con un error genérico.
- **Ana marca entrada otra vez después de que la hoja de cierre la vio sin entrada abierta**: el
  servidor decide con el estado al confirmar el cierre, no con lo que mostró la hoja.
- **La tableta sigue con la sesión de Ana, que ya marcó su salida, y Beto va a abrir**: la
  apertura se revisa contra la persona de la sesión (Ana) y se rechaza; Beto entra con su PIN y
  abre a su nombre.
- **Cajas secundarias** (barra): la regla aplica igual a cada caja, cada una con quien la abrió.
- **Varias cajas abiertas por la misma persona**: marcar su salida libera el cierre de todas; no se
  pide una salida por caja.
- **La persona que abrió fue dada de baja** antes del cierre: su entrada abierta sigue pidiendo
  salida; solo se resuelve con el cierre autorizado (no tiene PIN útil).
- **Se apaga «Lleva asistencia» a quien abrió mientras la caja está abierta**: al cierre la regla
  ya no aplica a esa persona.
- **PIN equivocado desde la hoja**: cuenta para el mismo bloqueo por intentos que la pantalla de
  Asistencia y el desbloqueo con PIN; la hoja no se convierte en otra puerta para probar PINs.
- **Cancelar la hoja después de marcar la entrada o la salida**: lo marcado se queda (es un hecho de
  asistencia, no parte de la apertura o el cierre); la caja sigue como estaba.
- **Dos personas con permiso autorizan el mismo cierre a la vez**: se cierra una sola vez; la
  segunda ve la caja ya cerrada, como hoy con dos cierres simultáneos.
- **Ana marca su salida mientras alguien autoriza el cierre sin ella**: si al confirmar Ana ya no
  tiene entrada abierta, el cierre es normal y no queda registrado como autorizado.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: Con la asistencia del negocio encendida y quien abre con «Lleva asistencia» prendido,
  el sistema MUST rechazar la apertura de cualquier caja si esa persona no tiene una entrada
  abierta de 16 horas o menos al momento de confirmar la apertura.
- **FR-002**: Con la asistencia encendida y quien abrió la caja con «Lleva asistencia» prendido, el
  sistema MUST rechazar el cierre si esa persona tiene cualquier entrada abierta (de la antigüedad
  que sea) al momento de confirmar el cierre.
- **FR-003**: Con la asistencia del negocio apagada, o con «Lleva asistencia» apagado para la
  persona correspondiente, el sistema MUST abrir y cerrar la caja sin revisar asistencia. El estado
  que cuenta es el del momento de confirmar, no el que había al abrir.
- **FR-004**: La regla MUST decidirla el servidor en cada apertura y cada cierre, sin importar qué
  pantalla o versión mande la petición. La revisión del cierre MUST ocurrir dentro de la misma
  operación que cierra, para que una entrada marcada en otra tableta a la vez no se cuele.
- **FR-005**: Las hojas de apertura y de cierre MUST mostrar el estado de asistencia que aplica
  antes de pedir el conteo del dinero, para que nadie cuente el cajón y luego sea rechazado.
- **FR-006**: La hoja de apertura MUST permitir a quien abre marcar su entrada con su PIN sin salir
  de la hoja, y la hoja de cierre MUST permitir a quien abrió marcar su salida con su PIN sin salir
  de la hoja, cuando esa entrada tenga 16 horas o menos. Ese registro MUST ser idéntico al de la
  pantalla de Asistencia (misma hora del servidor, mismo aparato e IP, misma bitácora) y compartir
  su bloqueo por PIN equivocado.
- **FR-007**: Quien tenga el permiso de autorizar el cierre sin la salida de quien abrió MUST
  poder cerrar la caja aunque quien la abrió tenga una entrada abierta, con motivo obligatorio. El
  cierre autorizado MUST dejar en la bitácora, dentro de la misma operación que cierra, quién
  autorizó, de quién era la salida pendiente y el motivo, y MUST dejar en esa misma operación la
  entrada abierta de quien abrió como «sin salida», sin inventarle hora de salida.
- **FR-007a**: El cierre autorizado MUST NOT estar disponible para quien abrió la caja cuando su
  propia entrada tiene 16 horas o menos (marca su salida con su PIN); con una entrada de más de
  16 horas, sí.
- **FR-008**: Quien no tenga ese permiso MUST NOT poder cerrar la caja mientras quien la abrió no
  haya marcado su salida: la hoja no le ofrece seguir y le dice que lo pida a alguien con el
  permiso, y el servidor MUST rechazar el cierre aunque la petición llegue. El permiso es uno nuevo,
  de caja; por omisión lo tienen los roles sugeridos admin y gerente.
- **FR-009**: Los controles nuevos MUST preguntar por permiso, no por nombre de rol.
- **FR-010**: Los mensajes de rechazo MUST nombrar a la persona y decir qué hacer («Para abrir la
  caja, primero marca tu entrada», «Ana abrió esta caja y no ha marcado su salida»), sin términos
  técnicos, y caber en la hoja a 1024×600 junto con sus botones de al menos 44 px.
- **FR-011**: Marcar entrada o salida desde la hoja y luego cancelar la apertura o el cierre MUST
  dejar el registro de asistencia como quedó; la caja no cambia.
- **FR-012**: La regla del cierre MUST revisar solo a quien abrió la caja; la salida del resto del
  personal no se revisa (fuera de alcance).

### Key Entities

- **Turno de caja**: ya existe; guarda quién lo abrió. Es el dato del que cuelga la regla del
  cierre.
- **Registro de asistencia**: ya existe (spec 033); una entrada con o sin salida, con su
  corrección y su bitácora. La feature lee si hay una entrada abierta y su antigüedad, marca
  entradas y salidas por el camino que ya existe, y el cierre autorizado deja la entrada «sin
  salida», el mismo estado que hoy pone una entrada de más de 16 horas.
- **Cierre autorizado**: un evento de la bitácora de auditoría ligado al turno de caja: quién
  autorizó, de quién era la salida pendiente, el motivo y que su entrada quedó «sin salida».
- **Permiso de autorizar el cierre sin la salida de quien abrió**: una capacidad nueva de caja,
  asignable por empresa; por omisión la tienen admin y gerente.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Con la asistencia encendida, el 100 % de los turnos de caja abiertos desde la
  entrega tienen una entrada vigente de quien los abrió a la hora de abrir.
- **SC-002**: Con la asistencia encendida, todo turno de caja cerrado desde la entrega con quien lo
  abrió todavía con una entrada abierta es un cierre autorizado, con autor y motivo en la bitácora;
  ninguno se cierra así sin el permiso.
- **SC-003**: Quien olvidó marcar su entrada abre la caja con a lo más un paso más que hoy (su PIN
  en la misma hoja), sin ir a otra pantalla.
- **SC-004**: Un negocio con la asistencia apagada abre y cierra caja con exactamente los mismos
  pasos que antes de la entrega.
- **SC-005**: Ningún rechazo por asistencia llega después de que la persona contó el dinero.

## Assumptions

- **«Quien abre» es la persona de la sesión de la tableta al confirmar la apertura**, la misma que
  hoy queda como autora del turno. Un negocio que abre caja con una cuenta compartida que no es una
  persona tendría que apagarle «Lleva asistencia», y para esa cuenta la regla no aplica.
- **Quien cierra no necesita entrada vigente**: el dueño pidió revisar solo a quien abrió.
- **Las 16 horas son las de la spec 033** (una entrada abierta más vieja se da por «sin salida»);
  no se vuelven configurables en esta feature.
- **Sin datos nuevos de asistencia**: lo que la regla necesita ya se guarda; marcar usa el camino
  de la spec 033. Lo nuevo es el permiso y el evento de bitácora del cierre autorizado.
- **Los turnos ya abiertos al momento de la entrega** se cierran con la regla nueva (FR-002 se
  evalúa al cerrar), aunque se hayan abierto sin ella.
- **Fuera de alcance**: exigir la salida de todo el personal al cerrar; revisar asistencia en
  traspasos, salidas de efectivo u otras acciones de caja.
