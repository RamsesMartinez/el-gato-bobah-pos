# Feature Specification: Roles y permisos por empresa (modelo híbrido)

**Feature Branch**: `068-roles-por-empresa`

**Created**: 2026-11-06 (entrega 1) · ampliado el 2026-11-06 (entregas 2 y 3)

**Status**: Entregas 1 y 2 en producción · entrega 3 en subida

**Input**: Issue #61 «Roles y permisos por empresa (modelo híbrido)» y su seguimiento #232, con las decisiones del dueño del 2026-10-13 y del 2026-11-06.

## Corte en entregas

| Entrega | Qué lleva | Estado |
| --- | --- | --- |
| **1** | Permiso `orders.delivered_view`: la cajera ve los entregados del día y reimprime su ticket; entregados y devolución preguntan por permiso | En producción desde el 2026-11-06 (PR #234, #235) |
| **2** | Roles por empresa guardados en la base, pantalla para administrarlos, Empleados asigna cualquier rol activo, todo control que hoy pregunta por el nombre del rol pasa a un permiso por capacidad, y el permiso de cada petición se decide contra la base | Esta especificación |
| **3** | Quitar el rol fijo del usuario cuando ninguna versión de la API en producción lo lea (el tipo de cuatro valores queda solo para la medición de uso) | Subida posterior a la 2; mismo spec |

**Por qué tres subidas**: la API anterior y la nueva conviven durante cada subida (constitución IV).
La entrega 2 agrega el rol por empresa y deja de leer el rol fijo; borrarlo en la misma subida
dejaría a la API anterior preguntando por una columna que ya no existe. La entrega 3 la borra
cuando ya nadie la lee.

### Entrega 1 (cerrada)

Una cajera no pudo reimprimir el ticket de un pedido que entregó otro usuario (2026-11-06). Se
agregó el permiso por capacidad `orders.delivered_view` para admin, gerente y cajero; devolver dinero
de un pedido entregado pasó a `payments.void`. Detalle en el historial de este archivo y en #234.

## Decisiones del dueño

**2026-10-13**
- Un rol no se borra: se inactiva. Solo se borra definitivamente un rol propio sin ningún usuario.
- Los roles sugeridos (los del producto) no se borran.

**2026-11-06** (puerta de roles del principio VIII)
1. Los roles sugeridos —admin, gerente, cajero y mesero— existen siempre en todas las empresas,
   nuevas y existentes. La empresa los puede inactivar, extender o crear roles propios.
2. Base normalizada: un catálogo universal de permisos y de roles sugeridos (el mismo identificador
   en todas las empresas) y, por empresa, sus roles con llave compuesta empresa + rol; ninguna
   referencia entre filas de una empresa puede apuntar a otra.
3. **Herencia**: lo que puede un rol sugerido en una empresa = lo que el producto le da + lo que la
   empresa le agregó − lo que le quitó. Un permiso nuevo del producto llega solo a los sugeridos de
   todas las empresas, salvo que la empresa lo haya quitado a propósito. Los roles propios no
   reciben nada solos.
4. En un rol sugerido la empresa puede agregar y quitar permisos, salvo que el admin nunca pierde la
   administración de roles.
5. Lo que hoy se limita por rol pasa a un permiso por capacidad, reutilizando los que existen.

Fuera de alcance: permisos por pantalla o por botón.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Nada cambia para quien ya opera (Priority: P1)

El día de la subida, cada persona de cada empresa puede exactamente lo mismo que el día anterior:
la cajera abre caja y cobra, el mesero toma pedidos, la gerente ve ventas y reportes, la
administradora da de alta empleados. Ninguna pantalla aparece ni desaparece.

**Why this priority**: es un negocio operando; una subida que le quita a alguien una pantalla a
media jornada detiene el mostrador.

**Independent Test**: para cada rol sugerido, la lista de lo que puede hacer antes y después de la
subida es idéntica, ruta por ruta y pantalla por pantalla.

**Acceptance Scenarios**:

1. **Given** una empresa existente con usuarios de los cuatro roles, **When** se sube la versión nueva, **Then** cada usuario tiene asignado el rol sugerido equivalente al que tenía y puede exactamente lo mismo.
2. **Given** cualquier ruta que hoy permite o niega por rol, **When** la pide un usuario de cada rol sugerido, **Then** la respuesta (permitido o negado) es la misma que antes.
3. **Given** el menú lateral y los controles que hoy dependen del rol, **When** entra cada rol sugerido, **Then** ve las mismas pantallas y los mismos controles.
4. **Given** una empresa creada después de la subida, **When** se crea, **Then** nace con los cuatro roles sugeridos, activos, y su primer admin con el rol admin.

---

### User Story 2 - La administradora ajusta lo que puede un rol (Priority: P1)

La administradora de una fonda quiere que su cajero pueda ver los reportes pero no capturar precios
por plataforma. Entra a la administración de roles, abre «Cajero», ve los permisos agrupados por
área con los que trae marcados, agrega «Ver reportes», quita «Capturar precios por plataforma» y
guarda. Desde ese momento, sin que el cajero vuelva a entrar, el cajero ve Reportes y ya no puede
capturar precios.

**Why this priority**: es lo que el modelo híbrido le vende a cada cliente: decidir sin pedir un
cambio de código.

**Independent Test**: editar los permisos de un rol sugerido y comprobar que la siguiente petición
de un usuario con ese rol se permite o se niega según lo nuevo.

**Acceptance Scenarios**:

1. **Given** el rol sugerido «Cajero», **When** la administradora le agrega un permiso y le quita otro, **Then** lo que puede ese rol es lo que el producto le da, más lo agregado, menos lo quitado.
2. **Given** un cajero con sesión abierta, **When** se le quita a su rol un permiso, **Then** su siguiente petición que lo pida se niega, sin esperar a que renueve su sesión.
3. **Given** el rol sugerido «Cajero» con permisos editados, **When** la administradora pide «volver a lo sugerido», **Then** el rol vuelve a tener exactamente lo que el producto le da.
4. **Given** el rol «Admin», **When** alguien intenta quitarle la administración de roles, **Then** se rechaza con un mensaje claro.
5. **Given** el producto agrega en una subida futura un permiso nuevo al rol «Cajero», **When** la empresa no había quitado ese permiso, **Then** su cajero lo recibe sin que nadie haga nada; **When** la empresa lo había quitado, **Then** no lo recibe.

---

### User Story 3 - La empresa crea un rol propio y se lo asigna a alguien (Priority: P1)

Una taquería tiene «Encargado de turno»: cobra, abre y cierra caja, ve ventas del día pero no
reportes ni empleados. La administradora crea el rol «Encargado de turno», marca sus permisos,
guarda, y en Empleados se lo asigna a una persona. Esa persona entra y ve justo eso.

**Why this priority**: sin roles propios, el modelo híbrido no existe.

**Independent Test**: crear un rol propio con un conjunto de permisos, asignarlo a un usuario y
comprobar ruta por ruta que puede exactamente esos.

**Acceptance Scenarios**:

1. **Given** la administración de roles, **When** la administradora crea un rol con nombre y permisos, **Then** queda activo y aparece al asignar rol en Empleados.
2. **Given** un nombre de rol que ya existe en la empresa (sin distinguir mayúsculas, acentos ni espacios, y contando los sugeridos), **When** intenta crearlo o renombrar otro con él, **Then** se rechaza diciendo que ya existe.
3. **Given** un usuario con un rol propio, **When** entra, **Then** el menú y los controles muestran lo que su rol permite, y el servidor niega lo demás.
4. **Given** el producto agrega un permiso nuevo, **Then** los roles propios no lo reciben.

---

### User Story 4 - Inactivar, reactivar y borrar roles (Priority: P2)

La empresa deja de usar «Mesero». Lo inactiva: ya no aparece al asignar rol. Si todavía hay
empleados activos con ese rol, el sistema no la deja inactivarlo y le dice cuántos son. Un rol propio
que nunca se asignó se puede borrar; uno con usuarios (activos o dados de baja), no: solo se inactiva.

**Why this priority**: es la regla del dueño del 2026-10-13; sin ella, borrar un rol dejaría a sus
usuarios sin rol.

**Independent Test**: intentar inactivar con usuarios activos (rechazado), inactivar sin ellos
(aceptado), borrar un propio sin usuarios (aceptado) y uno con usuarios (rechazado).

**Acceptance Scenarios**:

1. **Given** un rol con empleados activos, **When** se intenta inactivar, **Then** se rechaza diciendo cuántos empleados lo tienen.
2. **Given** un rol sin empleados activos, **When** se inactiva, **Then** deja de ofrecerse al asignar rol y puede reactivarse después con sus mismos permisos.
3. **Given** un rol sugerido, **When** se intenta borrar, **Then** no se ofrece la acción y el servidor la rechaza.
4. **Given** un rol propio que ningún usuario tiene ni tuvo asignado, **When** se borra, **Then** desaparece.
5. **Given** un rol propio asignado a un usuario dado de baja, **When** se intenta borrar, **Then** se rechaza y se ofrece inactivarlo.
6. **Given** el rol «Admin», **When** se intenta inactivar, **Then** se rechaza: una empresa no puede quedarse sin quien administre roles.

---

### User Story 5 - Queda registro de quién cambió qué (Priority: P2)

La dueña ve en la bitácora que el martes la gerente le dio a «Cajero» el permiso de devolver
dinero: quién, cuándo, desde qué aparato, qué agregó y qué quitó.

**Why this priority**: dar permisos de dinero es una decisión de confianza; sin rastro, un abuso no
se puede reconstruir.

**Independent Test**: cada acción sobre roles y cada cambio de rol de un empleado deja un evento con
el detalle de lo que cambió.

**Acceptance Scenarios**:

1. **Given** cualquier alta, edición de permisos, cambio de nombre, inactivación, reactivación o borrado de un rol, **When** se guarda, **Then** queda un evento con quién, cuándo y qué cambió.
2. **Given** un cambio del rol asignado a un empleado, **When** se guarda, **Then** queda un evento con el rol anterior y el nuevo.

---

### Edge Cases

- **Subida con las dos versiones de la API corriendo**: la anterior sigue leyendo el rol fijo y la
  nueva el rol por empresa. Un empleado dado de alta o cambiado de rol por cualquiera de las dos
  queda con los dos datos coherentes mientras existan ambos.
- **Sesión emitida por la API anterior**: no trae el rol por empresa; la API nueva lo resuelve por el
  usuario y la petición funciona igual.
- **Usuario con rol inactivo** (no debería pasar: no se puede inactivar con usuarios activos, pero un
  usuario dado de baja que se reactiva sí puede caer ahí): no puede hacer nada que pida permiso; al
  reactivarlo se pide elegirle un rol activo.
- **Empleado dado de baja**: su rol no cuenta como «en uso» para inactivar, pero sí para borrar.
- **Permiso que deja de existir en el producto**: las adiciones y quitas que lo nombraban dejan de
  contar; no rompen la pantalla ni el cálculo.
- **El último usuario activo con administración de roles**: no se le puede cambiar el rol a uno sin
  esa capacidad ni darlo de baja; una empresa nunca queda sin quien corrija los roles.
- **Una empresa sin roles por un respaldo viejo o un alta a mano**: los sugeridos se crean solos al
  arrancar la API o al dar de alta la empresa; nunca se adivina un rol por empresa al vuelo.
- **Dos pantallas editando el mismo rol**: la segunda en guardar recibe un aviso de que el rol cambió
  en vez de pisar la primera.
- **Nombre vacío, de más de 40 caracteres o igual a otro**: se rechaza con un mensaje claro.
- **Una empresa nunca ve ni toca los roles de otra**, ni puede asignarle a su empleado un rol de otra.
- **Medición de uso**: con roles propios, la medición sigue agrupando por los cuatro sugeridos; lo
  hecho por un rol propio se guarda sin rol, como hoy se guarda lo de un rol con una sola persona.

## Requirements *(mandatory)*

### Functional Requirements

**Catálogo y roles**
- **FR-001**: El producto mantiene un catálogo universal de permisos (nombre estable en inglés,
  etiqueta y área) y de los cuatro roles sugeridos con lo que cada uno trae; el identificador de cada
  permiso y de cada rol sugerido es el mismo en todas las empresas.
- **FR-002**: Toda empresa, existente o nueva, tiene siempre sus cuatro roles sugeridos; la subida los
  crea para las existentes y el alta de empresa para las nuevas.
- **FR-003**: Lo que puede un rol sugerido en una empresa es lo que el producto le da, más lo que la
  empresa le agregó, menos lo que le quitó. Lo que puede un rol propio es solo lo que la empresa le
  dio.
- **FR-004**: Un permiso nuevo del producto asignado a un rol sugerido lo reciben ese rol en todas
  las empresas, salvo las que lo hayan quitado; ningún rol propio lo recibe.
- **FR-005**: La empresa puede crear roles propios con nombre y cualquier conjunto de permisos del
  catálogo; renombrarlos; agregar y quitar permisos a cualquier rol; y devolver un rol sugerido a lo
  que el producto le da.
- **FR-006**: El rol admin no puede perder la administración de roles ni inactivarse.
- **FR-007**: Un rol con empleados activos no se puede inactivar; un rol inactivo no se ofrece al
  asignar y se puede reactivar con sus permisos.
- **FR-008**: Solo se borra un rol propio que ningún usuario (activo o dado de baja) tiene asignado;
  los sugeridos nunca.
- **FR-009**: El nombre de un rol es único en la empresa sin distinguir mayúsculas, acentos ni
  espacios, incluidos los nombres de los sugeridos; de 1 a 40 caracteres.
- **FR-010**: Guardar un rol rechaza el cambio si el rol cambió desde que la pantalla lo leyó.

**Empleados**
- **FR-011**: Empleados ofrece, al dar de alta o editar, cualquier rol activo de la empresa, y
  muestra el nombre del rol de cada empleado.
- **FR-012**: No se puede dejar a la empresa sin ningún usuario activo con la administración de
  roles (por cambio de rol ni por baja).
- **FR-013**: Una empresa solo puede asignar a sus empleados roles propios de ella.

**Autorización**
- **FR-014**: Todo control del servidor y de la pantalla que hoy pregunta por el nombre del rol pasa a
  preguntar por un permiso por capacidad; ningún control pregunta por el nombre de un rol.
- **FR-015**: La asignación inicial de permisos a los cuatro sugeridos reproduce exactamente lo que
  cada rol puede hacer hoy, ruta por ruta y pantalla por pantalla.
- **FR-016**: El servidor decide cada petición con los permisos vigentes del rol del usuario en ese
  momento: un cambio de permisos o de rol aplica a la siguiente petición, sin renovar sesión.
- **FR-017**: Un usuario sin rol activo no puede nada que pida permiso.
- **FR-018**: La sesión entrega a la pantalla el nombre del rol y la lista de permisos vigentes, y la
  pantalla la vuelve a pedir al volver al frente o al recibir un rechazo por permiso, para no ofrecer
  lo que ya no puede.
- **FR-019**: La medición de uso agrupa por los cuatro roles sugeridos; lo de un rol propio se guarda
  sin rol. Ninguna pantalla medida depende del nombre del rol para aceptarse: depende del permiso.

**Bitácora**
- **FR-020**: Crear, renombrar, cambiar permisos, inactivar, reactivar, devolver a lo sugerido y
  borrar un rol, y cambiar el rol de un empleado, dejan un evento en la bitácora con quién, cuándo y
  el detalle de lo que cambió, en la misma operación que el cambio.

**Entrega 3**
- **FR-021**: En la subida siguiente a la entrega 2, el rol fijo del usuario deja de existir en la
  base; ninguna consulta, prueba ni pantalla lo nombra. El tipo de cuatro valores sigue existiendo
  solo como la dimensión «rol sugerido» de la medición de uso.

### Key Entities

- **Permiso**: capacidad del producto con nombre estable en inglés, etiqueta en español y área
  (Ventas, Caja, Almacén…). Universal.
- **Rol sugerido**: uno de los cuatro del producto (admin, gerente, cajero, mesero) con los permisos
  que el producto le da. Universal.
- **Rol de la empresa**: un rol que existe en una empresa; o es la instancia de un rol sugerido o es
  propio. Tiene nombre (los sugeridos usan el del producto), si está activo, y la empresa a la que
  pertenece.
- **Ajuste de permiso**: lo que la empresa agregó o quitó a uno de sus roles, permiso por permiso.
- **Usuario**: pertenece a una empresa y tiene asignado un rol de esa misma empresa.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: El día de la subida, 0 diferencias entre lo que podía y lo que puede cada rol
  sugerido, medido ruta por ruta y pantalla por pantalla.
- **SC-002**: Una administradora crea un rol propio con 5 permisos y lo asigna a un empleado en
  menos de 2 minutos, en una tableta de 1024×600, sin ayuda.
- **SC-003**: Quitar un permiso a un rol niega la siguiente petición de un usuario con sesión
  abierta que lo pida; no hay ventana en la que lo siga pudiendo.
- **SC-004**: 0 controles, en servidor o pantalla, que pregunten por el nombre de un rol al terminar
  la entrega 2.
- **SC-005**: El 100% de los cambios de roles y de rol asignado aparecen en la bitácora con quién y
  qué cambió.
- **SC-006**: Ninguna empresa ve, asigna ni edita un rol de otra, probado con dos empresas reales.

## Assumptions

- Los nombres de los permisos nuevos (uno por capacidad) los decide el plan siguiendo la forma de
  los que ya existen; quedan fijos en cuanto se guardan como dato.
- El nombre de un rol sugerido es el del producto y no lo renombra la empresa; si quiere otro nombre,
  inactiva el sugerido y crea uno propio.
- La pantalla de roles vive dentro de la administración de Empleados (mismo permiso de entrada que
  administrar roles), y se diseña con maquetas a 1024×600.
- La consola de plataforma no administra roles de las empresas.
- La medición de uso guardada antes de la subida no se reescribe.
