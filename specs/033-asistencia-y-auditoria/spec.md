# Feature Specification: Asistencia del personal y auditoría de acciones

**Feature Branch**: `033-asistencia-y-auditoria`

**Created**: 2026-10-10

**Status**: Draft

**Input**: Decisiones 18 y 19 del dueño (2026-10-09), que resuelven las mejoras #27 (asistencia) y
#22 (auditoría) del documento de mejoras del 2026-10-05:

- **18. Asistencia:** pantalla dedicada de asistencia en el POS; cada empleado pone su usuario y su
  PIN para registrar entrada y salida. Cada registro guarda, solo visible para admin/gerente: IP,
  dispositivo y si vino de una tableta del local registrada o de otro aparato (cada tableta del POS
  se da de alta con un nombre, p. ej. «Tableta barra»). Un registro desde un dispositivo no
  registrado se marca para revisión. Banco de horas: cada persona tiene su horario; el reporte da
  horas trabajadas, retardos y minutos extra acumulados.
- **19. Auditoría:** cada acción sobre pedidos, pagos, devoluciones, pérdidas, caja, salidas,
  precios y asistencia guarda quién, cuándo, IP, dispositivo y antes → después. Hay historial por
  pedido y uno general, solo para admin/gerente.

La parte de aparatos registrados y revisión de la decisión 18 la reemplazó el dueño el 2026-10-10
por una clave que genera cada navegador (ver Clarificaciones).

Hoy la entrada y salida se avisa en un grupo de WhatsApp, y un caso como el cobro de $218 del
14 de septiembre no se puede reconstruir porque no queda registro de quién creó, cobró o canceló.

## Clarifications

### Session 2026-10-10

- Q: ¿Cuántos minutos de tolerancia antes de contar retardo y extra? → A: Configurable por empresa. Cada negocio decide si mide retardos y minutos extra y con cuánta tolerancia (entrada y salida por separado). Por omisión está apagado: el reporte da solo horas trabajadas y la bitácora de entradas y salidas. En El Gato Bobah se deja apagado, porque abrir y cerrar el local toma minutos antes de checar y después de checar.
- Q: ¿Un gerente puede corregir o aprobar sus propios registros? → A: Dos permisos separados: «corregir y revisar asistencia de otros» y «corregir y revisar la propia». Hoy el admin tiene los dos y el gerente solo el primero; cuando cada empresa configure sus roles, decide quién tiene cuál. Toda corrección queda en la bitácora.
- Q: ¿Cómo se excluye a quien no se le mide asistencia (p. ej. el dueño)? → A: Casilla «Lleva asistencia» por usuario, prendida por omisión; apagada, la persona no aparece en la pantalla de checar ni en el reporte.
- Q: ¿Desde qué aparato se puede abrir la pantalla de Asistencia? → A: La asistencia se activa o desactiva por empresa. Con ella activa, se checa desde cualquier aparato con sesión del negocio abierta; lo que no venga de un aparato registrado sale «Por revisar». Admin y gerente ven de cada registro el aparato, sistema y navegador. Un aparato se registra por navegador: en una Surface con dos navegadores, cada uno se da de alta por separado.
- Q: ¿Se obtienen datos del hardware además del navegador? → A: Se guarda como descripción sistema operativo, navegador, modelo (cuando el aparato lo informa, p. ej. Android) y tamaño de pantalla. El navegador no entrega número de serie ni identificador de hardware, así que esos datos describen el aparato pero no lo identifican.
- Q: ¿Se mantiene el alta de aparatos y la revisión de registros «Por revisar»? → A: No; **reemplaza la parte de aparatos registrados de la decisión 18 y las respuestas anteriores que la mencionan**. No hay alta ni baja de aparatos, ni marca «Por revisar», ni aceptar/rechazar. En su lugar, cada navegador genera su propia clave la primera vez que abre el POS y la manda en cada acción; cada registro y cada evento guardan IP, descripción del aparato y esa clave. Sirve para auditar («aparato a3f9»), no como prueba: se renueva si se borran los datos del navegador y el aparato la genera, así que puede falsificarse. Ponerle nombre a una clave se puede agregar después sin perder lo guardado.
- Q: Si alguien olvidó checar la entrada, ¿el gerente puede capturar el registro completo? → A: Sí. Captura entrada y salida con motivo obligatorio; el registro queda marcado «capturado a mano», va a la bitácora y pide los mismos permisos que corregir (sobre otros / sobre lo propio).
- Q: ¿El empleado puede ver sus propias horas trabajadas? → A: Solo la confirmación al checar («Entrada 9:02», «Trabajaste 8 h 10 min»). El historial y el reporte los ve quien tiene permiso.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Checar entrada y salida en la tableta (Priority: P1)

Al llegar, el empleado abre la pantalla de Asistencia en la tableta del local, elige su nombre,
teclea su PIN y toca «Entrada». Al irse hace lo mismo con «Salida». La pantalla le confirma la hora
registrada. No necesita iniciar sesión en el POS ni interrumpir a quien está cobrando.

**Why this priority**: Sin el registro no hay banco de horas, ni reparto de propinas por turno, ni
base para nómina. Sustituye al grupo de WhatsApp desde el primer día.

**Independent Test**: Con dos empleados dados de alta, cada uno registra entrada y salida en una
tableta; el gerente ve los cuatro registros con su hora.

**Acceptance Scenarios**:

1. **Given** un empleado activo sin entrada abierta, **When** pone su usuario y PIN correcto y toca
   «Entrada», **Then** queda registrada la entrada con la hora del servidor y la pantalla muestra
   «Entrada registrada, 9:02».
2. **Given** un empleado con entrada abierta, **When** registra «Salida», **Then** se cierra su
   turno y la pantalla muestra cuánto trabajó.
3. **Given** un empleado con entrada abierta, **When** intenta registrar otra «Entrada», **Then** el
   sistema no duplica: le indica que ya tiene entrada desde esa hora y le ofrece «Salida».
4. **Given** un PIN incorrecto, **When** se intenta varias veces seguidas, **Then** el sistema
   bloquea temporalmente a ese usuario igual que en el desbloqueo con PIN.
5. **Given** la pantalla de Asistencia abierta en una tableta con la caja en uso, **When** alguien
   checa, **Then** la sesión de quien cobra no cambia.

---

### User Story 2 - Saber desde dónde se checó (Priority: P1)

Cada registro de asistencia guarda la IP, la descripción del aparato (sistema, navegador, modelo y
pantalla) y la clave del aparato. Admin y gerente lo ven en el reporte y pueden notar a simple vista
un registro hecho desde otra red (otra IP) o desde un aparato distinto al de siempre (otra clave).
Solo admin y gerente ven esos datos.

**Why this priority**: Sin esto, checar desde casa es indistinguible de checar en el local, y el
registro no vale como respaldo de nómina.

**Independent Test**: Checar desde la tableta y desde otro navegador; el gerente ve en los dos
registros IP, descripción y claves distintas.

**Acceptance Scenarios**:

1. **Given** un navegador que abre el POS por primera vez, **When** se hace cualquier acción,
   **Then** el navegador ya tiene su clave y la manda; las siguientes acciones llevan la misma.
2. **Given** dos tabletas iguales, **When** cada una registra una entrada, **Then** los registros
   tienen la misma descripción pero claves distintas.
3. **Given** un empleado sin rol de admin/gerente, **When** checa, **Then** solo ve la
   confirmación de su registro; no ve IP, descripción, clave del aparato ni registros anteriores.
4. **Given** un navegador al que se le borraron los datos, **When** vuelve a usarse, **Then** genera
   una clave nueva y aparece como un aparato distinto.

---

### User Story 3 - Historial de un pedido (Priority: P1)

Desde un pedido, admin o gerente abre su historial: quién lo creó, qué renglones se agregaron o
quitaron, quién cobró, con qué método, terminal y referencia, quién canceló o devolvió, cada uno con
fecha, hora, dispositivo e IP, y el valor antes → después de lo que cambió.

**Why this priority**: Es el caso que motivó la auditoría (los $218 del 14-sep). Reconstruir un
pedido discutido es la pregunta más frecuente.

**Independent Test**: Crear un pedido, agregarle un renglón, cobrarlo en dos pagos, devolver uno;
el historial muestra los cinco eventos en orden con su autor.

**Acceptance Scenarios**:

1. **Given** un pedido con varias acciones, **When** el gerente abre su historial, **Then** ve cada
   acción en orden cronológico con quién, cuándo, dispositivo, IP y antes → después.
2. **Given** un pedido que se pasó a otro (unir cuentas), **When** se consulta cualquiera de los
   dos, **Then** el historial muestra el traslado y enlaza al otro pedido.
3. **Given** un pedido de plataforma aceptado automáticamente, **When** se consulta, **Then** el
   autor aparece como la plataforma, no como una persona.
4. **Given** un cajero, **When** abre un pedido, **Then** no ve la opción de historial, y si la
   pide directamente se le niega.

---

### User Story 4 - Bitácora general de acciones (Priority: P2)

Admin y gerente consultan una bitácora de todas las acciones auditadas —pedidos, pagos,
devoluciones, pérdidas, caja (apertura, cierre, arqueo), salidas de efectivo, cambios de precio y
asistencia—, filtrable por fecha, persona, tipo de acción y clave del aparato.

**Why this priority**: Responde preguntas que no empiezan por un pedido («¿quién cambió el precio
del frappé?», «¿qué hizo Abi el sábado?»). Depende de que las acciones ya se registren (US3).

**Independent Test**: Cambiar un precio y hacer una salida de efectivo; filtrar la bitácora por esa
persona y ver las dos acciones con su antes → después.

**Acceptance Scenarios**:

1. **Given** acciones de varios tipos, **When** el gerente filtra por persona y rango de fechas,
   **Then** ve solo las de esa persona en ese rango, paginadas y ordenadas por hora.
2. **Given** un cambio de precio de un producto, **When** se consulta, **Then** muestra el precio
   anterior y el nuevo.
3. **Given** un filtro con un valor inválido, **When** se envía, **Then** se rechaza con un mensaje,
   no se cae a un rango por omisión.

---

### User Story 5 - Banco de horas (Priority: P2)

El admin decide, para su negocio, si se miden retardos y minutos extra y con cuántos minutos de
tolerancia. Si lo enciende, captura el horario de cada persona (qué días y de qué hora a qué hora) y
el reporte de asistencia da, por persona y rango de fechas: horas trabajadas, retardos (entrada después del
horario) y minutos extra (salida después del horario, p. ej. por cerrar tarde con pedidos de
plataforma), con su acumulado.

**Why this priority**: Es lo que pide finanzas para nómina; necesita registros de US1 para servir.

**Independent Test**: Encender la medición sin tolerancia, capturar un horario de 9:00 a 17:00,
registrar entrada 9:10 y salida 17:40; el reporte muestra 10 min de retardo, 40 min extra y
8 h 30 min trabajadas.

**Acceptance Scenarios**:

0. **Given** una empresa con la medición apagada (por omisión), **When** se pide el reporte,
   **Then** muestra solo horas trabajadas por persona y día, sin columnas de retardo ni extra.
1. **Given** la medición encendida con 10 min de tolerancia, **When** alguien entra 9:08 con
   horario de 9:00, **Then** no cuenta retardo; si entra 9:12, cuenta 12 min.
2. **Given** la medición encendida, un horario y registros de una semana, **When** el gerente pide el reporte, **Then** ve
   por día y en total las horas trabajadas, retardos y minutos extra.
3. **Given** una persona sin horario capturado, **When** se pide el reporte, **Then** muestra sus
   horas trabajadas y dice «sin horario» en retardos y extra, sin inventarlos.
4. **Given** una entrada sin salida de un día anterior, **When** se pide el reporte, **Then** ese
   día aparece como «sin salida» y no suma horas hasta que el gerente lo corrija.
5. **Given** un horario que cambia a mitad de mes, **When** se pide el reporte del mes, **Then**
   cada día se mide contra el horario vigente ese día.

---

### User Story 6 - Corregir un registro de asistencia (Priority: P3)

Si alguien olvidó checar la salida, el gerente captura la hora correcta con un motivo. La
corrección queda auditada (antes → después, quién, motivo) y el registro original no se pierde.

**Why this priority**: Pasa en la práctica, pero el sistema es útil sin ella durante los primeros
días.

**Independent Test**: Corregir una salida faltante; el reporte suma las horas y la bitácora muestra
la corrección con su motivo.

**Acceptance Scenarios**:

1. **Given** una entrada sin salida, **When** el gerente captura la salida con motivo, **Then** el
   reporte la cuenta y el registro aparece como «corregido».
2. **Given** una corrección sin motivo, **When** se intenta guardar, **Then** se rechaza.
3. **Given** un gerente con permiso sobre otros pero no sobre lo propio, **When** intenta corregir su
   propio registro, **Then** se rechaza; el de otra persona sí lo puede corregir.
4. **Given** una persona que trabajó pero no checó nada ese día, **When** el gerente captura su
   entrada y salida con motivo, **Then** el registro aparece «capturado a mano» y suma horas.
5. **Given** el admin, con los dos permisos, **When** corrige su propio registro, **Then** se guarda
   y la bitácora lo muestra como autor y como persona afectada.

---

### Edge Cases

- **PIN débil o compartido:** el PIN es el mismo que desbloquea el POS; quien conoce el PIN de otro
  puede checar por él. Se mitiga con la IP, el aparato y la bitácora, no se resuelve.
- **Empleado sin PIN capturado:** no puede checar; la pantalla le dice que pida su PIN al gerente.
- **Usuario con «Lleva asistencia» apagada:** no aparece en la lista para checar ni en el reporte;
  sus registros anteriores se conservan.
- **Empleado desactivado:** no aparece en la lista y no puede checar.
- **Turno que cruza medianoche:** la salida a la 1:00 cierra la entrada del día anterior y las
  horas se asignan al día de la entrada.
- **Entrada abierta de hace más de 16 horas:** al checar «Entrada» de nuevo, la anterior queda como
  «sin salida» (para que el gerente la corrija) y se abre una nueva; no se le suman 20 horas.
- **Dos tabletas al mismo tiempo** registrando la entrada de la misma persona: solo una entrada
  queda abierta.
- **Un aparato con varios navegadores** (p. ej. la Surface): cada navegador tiene su propia clave y
  aparece como un aparato distinto.
- **Reloj de la tableta mal puesto:** la hora la pone el servidor; la de la tableta se ignora.
- **Sin red:** la asistencia exige conexión; la pantalla lo dice y no guarda nada localmente.
- **Datos del navegador borrados** (limpieza, reinstalación, ventana privada): se genera otra clave;
  lo anterior conserva la suya.
- **Clave ausente o malformada** (p. ej. un cliente que no es el POS): la acción se acepta y el
  registro queda sin clave; nunca se rechaza una venta por esto.
- **Dos aparatos del mismo modelo:** su descripción es idéntica; solo la clave los distingue.
- **Celular en el wifi del local:** tiene la misma IP que la tableta; lo delatan la descripción y la
  clave, y nadie lo marca automáticamente.
- **IP detrás del proxy:** se guarda la IP real del cliente, no la del proxy.
- **Una acción auditada que falla** (p. ej. el cobro se rechaza): no deja evento de éxito; la
  acción y su evento se guardan juntos o ninguno.
- **Dos empresas:** un gerente nunca ve asistencia ni bitácora de otra empresa.
- **Dos sucursales:** cada evento guarda la sucursal del objeto auditado (la del pedido, la de la
  caja). Un registro de asistencia guarda la sucursal solo si la empresa tiene una; con varias queda
  sin sucursal, sin adivinar, hasta que exista el selector de sucursal.

## Requirements *(mandatory)*

### Functional Requirements

**Asistencia**

- **FR-000**: Cada empresa MUST poder activar o desactivar la asistencia; desactivada, no aparece la
  pantalla ni el reporte, y los registros anteriores se conservan.
- **FR-001**: Con la asistencia activa, el POS MUST ofrecer una pantalla de Asistencia, accesible desde la tableta sin cambiar
  la sesión de quien está operando la caja.
- **FR-002**: El empleado MUST identificarse eligiendo su usuario y tecleando su PIN; un intento
  fallido cuenta para el mismo bloqueo por intentos que el desbloqueo con PIN.
- **FR-003**: El sistema MUST registrar entrada y salida con la hora del servidor, y no permitir dos
  entradas abiertas para la misma persona.
- **FR-004**: Cada registro MUST guardar la IP del cliente, una descripción del aparato (sistema
  operativo, navegador, modelo cuando lo informa y tamaño de pantalla) y la clave del aparato.
- **FR-005**: Cada navegador MUST generar su propia clave aleatoria la primera vez que abre el POS,
  guardarla y mandarla en cada acción. No hay alta, baja ni revisión de aparatos.
- **FR-006a**: El empleado MUST ver solo la confirmación de lo que acaba de registrar (hora y, en la
  salida, tiempo trabajado); no hay vista de historial propio.
- **FR-006**: IP, descripción y clave del aparato MUST ser visibles solo con permiso (hoy admin y
  gerente).
- **FR-008a**: Cada empresa MUST poder encender o apagar la medición de retardos y minutos extra, y
  fijar la tolerancia en minutos para la entrada y para la salida (0 a 60). Por omisión está
  apagada. Pasando la tolerancia, se cuentan todos los minutos desde la hora del horario.
- **FR-008**: Con la medición encendida, el admin MUST poder capturar el horario semanal de cada persona, con vigencia desde
  una fecha, sin reescribir el horario de días anteriores. Cada día admite hasta dos turnos; un turno
  cuya salida es menor que su entrada termina al día siguiente. El horario de otra persona se puede
  copiar como punto de partida, sin quedar ligado a ella.
- **FR-009**: El reporte de asistencia MUST dar, por persona, día y rango: horas trabajadas y, si la
  medición está encendida, retardos y minutos extra, con sus totales; los días sin salida se muestran aparte y no suman.
  Se puede ver por persona (por omisión), día por día o en cuadrícula de una semana.
- **FR-010**: Quien tenga permiso MUST poder corregir o completar un registro con motivo
  obligatorio, conservando el valor original.
- **FR-010c**: Quien tenga permiso MUST poder capturar a mano un registro completo (entrada y
  salida) con motivo obligatorio; queda marcado «capturado a mano» y no puede encimarse con otro
  registro de la misma persona.
- **FR-010a**: Corregir o capturar asistencia MUST pedir uno de dos permisos: sobre registros de
  otros, o sobre los propios. Hoy: admin los dos, gerente solo sobre otros.
- **FR-010b**: Cada usuario MUST tener la casilla «Lleva asistencia» (prendida por omisión); si está
  apagada, no aparece para checar ni en el reporte.

**Auditoría**

- **FR-011**: El sistema MUST guardar un evento por cada acción sobre: pedidos (crear, agregar o
  quitar renglones, pasar a otro pedido, entregar, cancelar), pagos (cobrar, devolver), pérdidas,
  caja (abrir, cerrar, arqueo), salidas de efectivo, propinas entregadas, cambios de precio
  (manuales y por plataforma) y asistencia (registros, correcciones y cambios de ajustes u horarios).
- **FR-012**: Cada evento MUST guardar quién (persona, o la plataforma/sistema si no hubo persona),
  cuándo, IP, descripción y clave del aparato, sucursal, el objeto afectado y el valor antes → después de lo que cambió.
- **FR-013**: El evento MUST guardarse en la misma operación que la acción: si la acción falla no
  queda evento, y si el evento no se puede guardar la acción no se aplica.
- **FR-014**: Los eventos MUST ser de solo agregar: nadie, ni admin, puede editarlos ni borrarlos
  desde el sistema.
- **FR-015**: Admin y gerente MUST poder ver el historial de un pedido desde el pedido.
- **FR-016**: Admin y gerente MUST poder consultar la bitácora general, filtrable por rango de
  fechas, persona, tipo de acción y clave del aparato, paginada; un filtro inválido se rechaza. La
  bitácora muestra cuántas acciones hubo de cada grupo (cobros y pedidos, cancelaciones y
  devoluciones, precios, caja, asistencia) y tocar un grupo filtra por él.
- **FR-017**: El acceso a historial, bitácora, reporte y correcciones MUST decidirse por permiso, no por
  nombre de rol (hoy: admin y gerente lo tienen).
- **FR-018**: Los eventos y la asistencia MUST guardarse indefinidamente mientras exista la empresa.
- **FR-019**: Las pantallas MUST cumplir la restricción de tableta: 1024×600, controles de al menos
  44 px y sin selector nativo.

### Key Entities

- **Registro de asistencia**: persona, entrada, salida, origen de cada una (IP, descripción y clave
  del aparato), y corrección con motivo y autor.
- **Ajuste de asistencia de la empresa**: medición encendida o apagada, tolerancia de entrada y de
  salida en minutos.
- **Horario**: por persona, los días de la semana con hora de entrada y salida, vigente desde una
  fecha.
- **Evento de auditoría**: tipo de acción, objeto afectado (p. ej. pedido y renglón), autor,
  momento, IP, descripción y clave del aparato, sucursal y el antes → después.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Un empleado registra su entrada en menos de 15 segundos desde que toca la tableta.
- **SC-002**: El grupo de WhatsApp de asistencia deja de usarse en las dos semanas siguientes al
  despliegue.
- **SC-003**: Un caso como el de los $218 del 14-sep se reconstruye (quién creó, cobró y con qué
  método) en menos de 2 minutos desde la pantalla del pedido.
- **SC-004**: El 100% de las acciones de la lista de FR-011 hechas tras el despliegue tienen su
  evento; ninguna acción de dinero queda sin autor.
- **SC-005**: El reporte semanal de horas (y, con la medición encendida, retardos y extra) de todo el personal sale sin cálculos a
  mano, y coincide al minuto con los registros.
- **SC-006**: Ningún empleado sin permiso puede ver IP, aparato ni la bitácora.

## Assumptions

- El PIN de asistencia es el mismo que ya tiene cada empleado para desbloquear el POS; no se crea
  otra credencial.
- Las acciones anteriores al despliegue no tienen historial; no se reconstruyen.
- La clave del aparato es una pista de auditoría, no una prueba: la genera el navegador y se
  renueva si se borran sus datos.
- Salir antes de la hora no descuenta del banco: el reporte lo muestra en horas trabajadas.
- El reparto de propinas por turno y la nómina usan estos datos después; no son parte de este spec.
- No se audita la consulta (quién vio qué), solo las acciones que cambian algo.
- El crecimiento estimado de la bitácora es de pocos MB al mes para un local.
