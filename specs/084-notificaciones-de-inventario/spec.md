# Feature Specification: Notificaciones de inventario

**Feature Branch**: `084-notificaciones-de-inventario`

**Created**: 2026-11-14

**Status**: Aprobado (maquetas v2 y «Para el spec» confirmados por el dueño el 2026-11-14)

**Input**: Issue #291 (lo subió el dueño el 2026-11-14 con sus decisiones del 13-nov). Relacionados:
#65 (más de un almacén), #271, #277, #289, #290, #293 (menú en dos niveles, otra sesión).

## Contexto

Hoy el aviso de que algo se acabó vive en un chat de WhatsApp. Quien opera avisa cuando el producto
ya no está y quien compra se entera horas después, cuando el proveedor ya cerró. Un viernes, entre
las 20:32 y las 21:42, se acabaron el queso crema, los platos de unicel, los empaques para llevar,
las bolsas grandes y las servitoallas; la siguiente ida a la Central era al día siguiente. Varios de
esos artículos ni siquiera existen como insumo en el POS.

La existencia del sistema no basta para avisar sola: muchos insumos tienen existencia negativa
porque los conteos recientes no están cargados, y casi ninguno tiene mínimo definido. Una alerta
automática sobre esos datos daría falsos positivos y se ignoraría. Por eso la alerta por mínimo es
**opt-in por insumo** y el aviso manual «Se acabó» es la vía confiable desde el primer día.

## Decisiones del dueño (2026-11-13, no se rediscuten)

| Tema | Decisión |
|---|---|
| Entrega | Una sola, con el trabajo ordenado por dependencias |
| Destinatarios por omisión | Gerente y admin, **por permiso** y no por nombre de rol. Pantalla para agregar correos extra, quitar a los de omisión y elegir por tipo de aviso |
| Alerta por insumo | Casilla «Enviar alerta de stock bajo» en la tarjeta, apagada por omisión, en una sección «Alertas» |
| Mínimo | Por empresa por omisión; cada sucursal puede definir sus reglas; queda abierta la opción de inventarios independientes por sucursal |
| Mínimo en 0 | Significa «sin mínimo definido» |
| Aviso al encender | Si el insumo ya está bajo, el aviso sale al instante (por estado, no solo por cruce) |
| «Se acabó» | Deja la existencia en 0 con un ajuste automático y dispara los avisos. Solo para insumos y productos existentes |
| «No lo encuentro» | Sí, sin tocar existencias |
| «Leída» | Por persona |
| Correo | Ventana de 5 minutos con un solo correo que junta todo lo nuevo. Solo HTML, con plantillas profesionales |
| Lista de compra | Con cantidad sugerida y el porqué; 14 días de cobertura por omisión |
| Resolución mínima | 1280×854 (constitución 1.29.0) |

## Para el spec (aprobado por el dueño el 2026-11-14)

| Pregunta | Decisión |
|---|---|
| Nombre de la acción y del permiso | Acción «Se acabó» con permiso nuevo `inventory.report_out` («Reportar que algo se acabó o no se encuentra»). Lo traen por omisión los cuatro roles sugeridos, igual que «Se echó a perder» (`inventory.write_off`): quien vende es quien ve que se acabó |
| Idempotencia en la pantalla | No hay una llave HTTP reutilizable fuera del MCP (que tiene la suya, ligada al token). Se sigue el patrón de `orders.client_uuid`: la pantalla manda una clave por toque, se guarda en el propio reporte con índice único por empresa, y repetirla devuelve el resultado del primero. El índice único del aviso abierto cubre el doble toque con claves distintas |
| Roles sugeridos con conteo y merma | Medido en el código: `inventory.count`, `inventory.configure` e `inventory.view` solo los traen admin y gerente; `inventory.write_off` los cuatro. Por eso el permiso nuevo es aparte: el cajero reporta «Se acabó» sin poder ajustar existencias ni ver el almacén |
| Correo de las personas | `users` no tiene columna de correo: solo `recovery_email`, opcional, que hoy usa la recuperación de contraseña. Se usa ese mismo correo para los avisos (en pantalla, «Correo») en vez de agregar otro; la pantalla de destinatarios dice quién no lo tiene y no lo edita (cambiar el correo de recuperación de otra persona sería poder entrar a su cuenta) |
| Destinatarios por omisión | Quien tiene `inventory.configure` (hoy admin y gerente) y correo. Un rol propio sin ese permiso no recibe nada |
| Choque con el spec 070 | En el código y en la base todo se llama **notificaciones** (`notifications`), nunca `alerts` ni `alert`: ese paquete ya es de los avisos de error del servidor. En pantalla, la sección de la tarjeta se llama «Alertas» (decisión del dueño) y el botón de la campana «Avisos» |

## User Scenarios & Testing *(mandatory)*

### User Story 1 - El cajero avisa que algo se acabó y el gerente se entera en minutos (Priority: P1)

A media operación se acaba el queso crema. El cajero abre «Se acabó» desde el menú de inventario,
escribe «queso» (o toca el mosaico de lo más reportado), confirma con un toque y sigue vendiendo. La
existencia del insumo queda en 0, en la campana de quienes reciben avisos aparece «Se acabó: Queso
crema» y, a más tardar en 5 minutos, al gerente le llega un correo con eso y con todo lo demás que
se haya acabado en esa ventana.

**Why this priority**: es la vía confiable desde el primer día y la que reemplaza al chat; no
depende de que la existencia ni los mínimos estén bien cargados.

**Independent Test**: con un cajero que solo tiene el permiso nuevo, reportar «Se acabó» de un
insumo con existencia 3, ver la existencia en 0, la notificación en la campana del gerente y el
correo en el buzón local; repetir el toque y ver que no hay un segundo aviso.

**Acceptance Scenarios**:

1. **Given** un insumo con existencia 3, **When** el cajero lo reporta como «Se acabó», **Then** la
   existencia queda en 0 por un ajuste con motivo fijo «se acabó», se abre una notificación y queda
   en la bitácora quién lo reportó.
2. **Given** un reporte recién hecho, **When** el cajero toca «Deshacer» dentro de la ventana, **Then**
   la existencia vuelve a 3 y la notificación queda descartada con el motivo «se deshizo».
3. **Given** un reporte hecho, **When** se registró una venta o un conteo de ese insumo después,
   **Then** «Deshacer» no procede y la pantalla dice por qué.
4. **Given** un insumo ya reportado y abierto, **When** otra persona lo reporta, **Then** se suma su
   confirmación al mismo aviso («también lo confirmó X») y no sale otro correo.
5. **Given** un cajero sin permiso de ajustar existencias, **When** reporta «Se acabó», **Then** puede
   hacerlo; **And When** intenta un ajuste de existencia con cantidad, **Then** se le niega.
6. **Given** una persona que ya reportó 20 veces en la última hora, **When** reporta otra, **Then** el
   sistema se lo impide con un aviso en pantalla.

---

### User Story 2 - «No lo encuentro»: lo que no existe en el POS también se avisa (Priority: P1)

Se acabaron las servitoallas, pero no existen como insumo. El cajero escribe «servitoallas», no
aparece nada y toca la última fila «No lo encuentro». El texto se guarda y llega como aviso al
gerente por campana y correo, sin tocar ninguna existencia. Desde el aviso, el gerente lo da de alta
(con el nombre ya escrito), lo marca como otro nombre de un insumo existente o lo descarta.

**Why this priority**: varios artículos del caso real no existían en el POS; sin esto el chat sigue
siendo necesario.

**Independent Test**: reportar un texto sin coincidencias y ver el aviso con ese texto; ninguna
existencia cambia; elegir «Es otro nombre de…» y ver que la búsqueda siguiente lo encuentra.

**Acceptance Scenarios**:

1. **Given** una búsqueda sin coincidencias, **When** el cajero toca «No lo encuentro», **Then** se
   guarda el texto y se abre un aviso que solo reciben quienes configuran el inventario.
2. **Given** un aviso «No lo encuentro», **When** el gerente elige «Dar de alta», **Then** se abre el
   alta de insumo con el nombre ya escrito, con la pregunta de alertas, y al guardar el aviso queda
   resuelto.
3. **Given** un aviso «No lo encuentro», **When** el gerente elige «Es otro nombre de…» y un insumo,
   **Then** el texto se agrega a los otros nombres de ese insumo y el aviso queda resuelto.
4. **Given** un aviso «No lo encuentro», **When** el gerente lo descarta, **Then** queda descartado
   con su motivo y nada más cambia.

---

### User Story 3 - El gerente enciende la alerta de stock bajo de un insumo (Priority: P1)

En la tarjeta del insumo hay una sección «Alertas». El gerente marca «Enviar alerta de stock bajo»,
captura el mínimo, elige si avisar al llegar al mínimo y/o al agotarse, si mostrar la franja en
pantalla y, opcionalmente, los días de cobertura para la lista de compra. Al guardar, una
confirmación dice qué se actualizará y qué pasará de inmediato («Hoy quedan 2, así que se enviará un
aviso al guardar»). Al crear un insumo, o al abrir por primera vez una tarjeta que nunca lo ha
preguntado, el sistema pregunta «¿Quieres recibir alertas de stock bajo de este insumo?» (por
omisión, No).

**Why this priority**: es la alerta automática; sin la casilla no hay aviso por mínimo.

**Independent Test**: encender la alerta en un insumo que ya está bajo el mínimo y ver el aviso al
instante; apagarla y ver el aviso cerrado; nada de eso pasa en un insumo sin la casilla.

**Acceptance Scenarios**:

1. **Given** un insumo sin la casilla y con existencia bajo su mínimo, **When** se vende, **Then** no
   se abre ningún aviso por mínimo.
2. **Given** un insumo con existencia 2, **When** se enciende la alerta con mínimo 5, **Then** la
   confirmación lo anuncia y al guardar se abre el aviso.
3. **Given** un aviso abierto, **When** se sube el mínimo de nuevo por debajo de la existencia, se
   apaga la alerta, se archiva o se une el insumo, **Then** el aviso se cierra con el motivo.
4. **Given** mínimo 0 o vacío, **When** la existencia llega a 0, **Then** solo avisa el evento «se
   agotó» si está marcado; el de «llegó al mínimo» no aplica.
5. **Given** existencia negativa o sin conteo reciente, **When** se evalúa, **Then** el aviso dice
   «pedir conteo», no «agotado».
6. **Given** un cambio de alertas guardado, **Then** queda en la bitácora con la persona y lo que
   cambió.
7. **Given** un producto de reventa con inventario propio, **Then** su tarjeta tiene la misma sección.

---

### User Story 4 - La detección no depende de nadie y nunca estorba una venta (Priority: P1)

Cada venta, compra recibida, conteo cerrado, merma o ajuste que deja a un insumo con alerta en o
bajo su mínimo abre **un** aviso para esa sucursal, insumo y tipo, aunque dos ventas crucen el mínimo
a la vez. Cuando la existencia vuelve sobre el mínimo, por cualquier movimiento, el aviso se cierra
solo; si vuelve a bajar, se abre otro. Si crear el aviso falla, la venta o la compra se guardan igual.

**Why this priority**: sin esto la alerta por mínimo no existe; y una venta que falla por un aviso es
peor que no tener aviso.

**Independent Test**: dos ventas concurrentes que cruzan el mismo mínimo dejan una sola notificación;
una compra que devuelve la existencia sobre el mínimo la cierra; forzar una falla del aviso y ver la
venta guardada.

**Acceptance Scenarios**:

1. **Given** existencia 6 y mínimo 5, **When** dos ventas de 1 confirman al mismo tiempo, **Then**
   hay exactamente una notificación abierta.
2. **Given** un aviso abierto, **When** se recibe una compra que deja la existencia sobre el mínimo,
   **Then** el aviso se resuelve con el motivo «llegó compra».
3. **Given** que crear el aviso falla, **When** se guarda la venta, **Then** la venta queda guardada.
4. **Given** una unión de insumos o un cambio de unidad, **Then** sus movimientos de traspaso no abren
   ni cierran avisos por sí mismos; la unión fusiona la casilla del origen con la del destino y cierra
   los avisos del origen.
5. **Given** un conteo que confirma la misma cifra, **When** se cierra, **Then** se evalúa igual.

---

### User Story 5 - La campana y la franja (Priority: P2)

Al pie del riel lateral hay una campana con el número de avisos no leídos **de esa persona**. Al
tocarla se abre un panel con los avisos abiertos primero y los resueltos o descartados después, cada
uno con su motivo de cierre. Leer un aviso lo marca como leído solo para quien lo leyó. Cuando un
insumo con «Mostrar franja» se acaba, aparece arriba una franja que no bloquea, una vez por aviso y
persona, que espera si hay una hoja de captura abierta y se quita sola a los 90 segundos; el aviso
sigue en la campana.

**Why this priority**: es como se entera quien está en el local; el correo cubre a quien no está.

**Independent Test**: dos personas ven el mismo aviso; una lo lee y solo su contador baja; la franja
no aparece con una hoja abierta y sí al cerrarla; desaparece a los 90 s.

**Acceptance Scenarios**:

1. **Given** dos personas con acceso a los avisos, **When** una abre uno, **Then** solo su contador
   baja.
2. **Given** un aviso resuelto, **Then** el panel lo muestra como resuelto con quién o qué lo cerró.
3. **Given** una hoja de captura abierta, **When** llega un «Se acabó» con franja, **Then** la franja
   espera a que la hoja se cierre.
4. **Given** una franja visible, **When** pasan 90 s, **Then** se quita sola y no vuelve para esa
   persona.
5. **Given** un aviso nuevo, **Then** la campana se actualiza sin recargar la página.

---

### User Story 6 - Un correo que se lee de un vistazo, cada 5 minutos como máximo (Priority: P2)

El gerente recibe un solo correo por ventana de 5 minutos con todo lo nuevo (lo que se acabó, lo
que llegó al mínimo, lo que no se encontró), con la marca del negocio y una tabla legible en el
teléfono. Si en la ventana no pasó nada, no hay correo. El mismo insumo no vuelve a mandar correo en
12 horas aunque se cierre y reabra su aviso. Cada mañana (08:00 por omisión, hora del negocio) llega
un resumen con los avisos abiertos y la lista de compra.

**Why this priority**: es lo que llega a quien compra cuando no está en el local.

**Independent Test**: tres avisos en la misma ventana producen un correo; ninguno en la siguiente
ventana sin avisos; el mismo insumo reabierto en 2 horas no vuelve a enviar; el resumen sale una
vez aunque el servidor se reinicie.

**Acceptance Scenarios**:

1. **Given** tres avisos en 5 minutos, **Then** sale un solo correo con los tres.
2. **Given** un aviso que se abre y se resuelve en la misma operación, **Then** no sale correo.
3. **Given** un insumo con correo enviado hace 2 horas, **When** su aviso se reabre, **Then** aparece
   en la campana pero no sale otro correo hasta cumplir 12 horas.
4. **Given** el resumen de la mañana ya enviado hoy, **When** el servidor se reinicia, **Then** no se
   envía otra vez.
5. **Given** un correo generado, **Then** pesa menos de 102 KB y lleva los colores y el logo del
   negocio.

---

### User Story 7 - Destinatarios por permiso, con correos extra (Priority: P2)

En la pantalla de destinatarios, quien configura el inventario ve quiénes reciben por omisión (las
personas cuyo rol puede configurar el inventario y tienen correo), puede quitar a alguno, agregar
correos que no son de ninguna persona del sistema (el del dueño de la tienda, el de compras) y elegir
por tipo de aviso quién recibe qué. Un rol propio sin el permiso no recibe nada.

**Why this priority**: sin destinatarios correctos el correo no sirve; y la lista del cierre de caja,
que lleva cifras de caja, no se mezcla con ésta.

**Independent Test**: quitar al gerente y ver que deja de recibir; agregar un correo extra y ver que
recibe; un rol propio sin el permiso no recibe nada.

**Acceptance Scenarios**:

1. **Given** un gerente con correo, **Then** aparece como destinatario por omisión.
2. **Given** un correo extra agregado, **Then** recibe aunque no tenga usuario.
3. **Given** un destinatario por omisión quitado, **Then** deja de recibir.
4. **Given** una persona sin correo, **Then** la pantalla lo dice y no recibe.
5. **Given** un cambio de destinatarios, **Then** queda en la bitácora con quién lo hizo.

---

### User Story 8 - Lista de compra que explica cada cantidad (Priority: P3)

Una pantalla «Lista de compra» (y la misma lista en el correo de la mañana) junta los insumos con
aviso abierto o bajo su mínimo, con existencia, mínimo, consumo diario medido, lugar de compra si
está definido, cantidad sugerida y el porqué en una línea: «se usan ~2 kg al día, quedan 1 kg,
objetivo 14 días».

**Why this priority**: convierte el aviso en una compra; depende de que los avisos existan.

**Independent Test**: un insumo con consumo medido sugiere una cantidad redondeada a empaques
enteros con su explicación; uno sin consumo medido lo dice y no sugiere cantidad.

**Acceptance Scenarios**:

1. **Given** consumo de 2/día, existencia 1, cobertura 14, **Then** sugiere 27 redondeado hacia arriba
   al empaque de menor costo por unidad, con el porqué.
2. **Given** un insumo sin consumo medido o sin receta, **Then** dice «sin consumo medido» y no
   sugiere cantidad.
3. **Given** un «Se acabó», **Then** su ajuste no cuenta como consumo.

### Edge Cases

- Dos tabletas reportan «Se acabó» del mismo insumo en el mismo segundo: un solo ajuste a 0 y un solo
  aviso; la segunda suma su confirmación.
- El doble toque manda la misma petición dos veces: la segunda devuelve el resultado de la primera.
- «Se acabó» de un insumo con existencia 0 o negativa: no inserta ajuste (no hay qué restar) pero sí
  abre el aviso.
- «Deshacer» después de que llegó una compra o un conteo: se niega.
- La existencia vuelve sobre el mínimo y vuelve a bajar en la misma ventana de 5 minutos: dos avisos
  en la campana, uno solo en el correo por el enfriamiento.
- La empresa tiene dos sucursales: los avisos son por sucursal; el mínimo por omisión es el de la
  empresa y una sucursal puede tener el suyo.
- Insumo archivado o unido: sus avisos abiertos se cierran; un movimiento que llega al origen
  redirigido no abre avisos en el origen.
- Un rol propio sin el permiso de configurar inventario: no recibe correo ni ve los avisos de «No lo
  encuentro».
- La persona no tiene correo: no recibe y la pantalla de destinatarios lo dice.
- El servidor de correo no responde: el aviso queda pendiente y se reintenta en la siguiente ventana;
  la venta nunca espera al correo.
- El servidor se reinicia a las 08:00: el resumen sale una vez.
- La empresa cambia de zona horaria: el resumen usa la zona vigente del negocio.
- La pantalla anterior (sin esta feature) sigue mandando ajustes y ventas durante la subida: todo
  sigue funcionando y los avisos se calculan igual.

## Requirements *(mandatory)*

### Functional Requirements

**Configuración de alertas por insumo**

- **FR-001**: Cada insumo y cada producto de reventa con inventario propio MUST tener una
  configuración de alertas apagada por omisión: casilla, mínimo, eventos («llegue al mínimo», «se
  acabe»), «mostrar franja» y días de cobertura (opcional, 14 por omisión).
- **FR-002**: El mínimo MUST valer por empresa por omisión y MUST poder sobrescribirse por sucursal.
  Mínimo 0 o vacío = sin mínimo definido.
- **FR-003**: Al crear un insumo (incluido el alta desde «No lo encuentro») o al abrir por primera vez
  una tarjeta que nunca lo preguntó, el sistema MUST preguntar si se quieren alertas; por omisión No,
  y la respuesta se recuerda para no volver a preguntar.
- **FR-004**: Al guardar un cambio de alertas, el sistema MUST mostrar una confirmación que diga qué se
  actualizará y qué pasará de inmediato (si se abrirá o cerrará un aviso al guardar), calculada con la
  existencia de ese momento.
- **FR-005**: Encender o apagar alertas MUST poderse hacer uno por uno o por lote, desde pantalla y
  desde el MCP (con vista previa e `idempotency_key`).
- **FR-006**: Todo cambio de configuración de alertas y de destinatarios MUST quedar en la bitácora
  con la persona.

**Detección**

- **FR-007**: El sistema MUST evaluar el estado de cada insumo con alerta tras cada movimiento de
  existencia (venta, compra, conteo, merma, ajuste) y MUST evaluarlo también al encender, apagar o
  cambiar el mínimo, archivar, unir y cerrar un conteo.
- **FR-008**: Un aviso por mínimo MUST abrirse por estado (en o bajo el mínimo y sin aviso abierto),
  no solo por cruce.
- **FR-009**: Como máximo MUST existir un aviso abierto por sucursal, insumo y tipo, incluso con
  transacciones concurrentes.
- **FR-010**: Un aviso MUST resolverse solo cuando la existencia vuelve sobre el mínimo (o sobre 0
  para «se acabó») por cualquier movimiento, guardando el motivo y quién o qué lo resolvió.
- **FR-011**: Existencia negativa o sin conteo reciente (30 días) MUST etiquetarse «pedir conteo», no
  «agotado».
- **FR-012**: Una falla al crear o enviar un aviso MUST NOT impedir guardar una venta, una compra, un
  conteo o una merma.
- **FR-013**: Los movimientos de una unión de insumos o de un cambio de unidad, y los insumos
  inactivos, MUST NOT abrir avisos. Al unir, la casilla se fusiona con OR y los avisos del origen se
  cierran.

**«Se acabó» y «No lo encuentro»**

- **FR-014**: MUST existir una pantalla «Se acabó» en el módulo de Inventario, junto a «Se echó a
  perder», visible por el permiso nuevo, con búsqueda grande que encuentra por nombre y otros
  nombres sin distinguir acentos ni plural, mosaicos de lo más reportado y lo reciente, confirmación
  con un segundo toque sobre el mismo artículo, y «Deshacer» visible 90 s.
- **FR-015**: «Se acabó» MUST dejar la existencia del artículo en 0 con un ajuste de motivo fijo, MUST
  NOT aceptar cantidad, MUST abrir el aviso y auditar, todo en una sola operación, y el ajuste MUST
  NOT contar como consumo.
- **FR-016**: «Se acabó» MUST ser idempotente: repetir la misma petición devuelve el mismo resultado,
  y un segundo reporte del mismo artículo con aviso abierto suma una confirmación sin otro correo.
- **FR-017**: «Deshacer» MUST reponer solo lo que restó el ajuste y MUST negarse si hubo un
  movimiento o un conteo posterior del artículo.
- **FR-018**: «Se acabó» MUST funcionar aunque el artículo no tenga alerta ni mínimo (avisa por campana
  y correo; la franja solo si el artículo la tiene).
- **FR-019**: Un aviso «se acabó» MUST resolverse con una compra recibida que deje existencia sobre 0
  (o sobre el mínimo si lo tiene), un conteo cerrado que incluya el artículo, o «ya llegó».
- **FR-020**: «No lo encuentro» MUST guardar el texto escrito, abrir un aviso dirigido solo a quien
  configura el inventario y MUST NOT tocar ninguna existencia. Desde el aviso: dar de alta (con el
  nombre escrito), agregarlo como otro nombre de un artículo existente, o descartar.
- **FR-021**: Reportar MUST tener un tope de 20 reportes por hora y persona, con aviso en pantalla.
- **FR-022**: Un permiso nuevo MUST permitir reportar «Se acabó» y «No lo encuentro» sin permitir
  ajustar existencias.

**Centro de notificaciones**

- **FR-023**: Los avisos MUST guardarse en una sola lista genérica por empresa con tipo, sucursal,
  artículo, estado (abierta, resuelta, descartada), existencia y mínimo vistos al abrir, motivo,
  persona y hora de cierre; lista para otros tipos después.
- **FR-024**: Lo leído MUST ser por persona; el contador de la campana es de no leídos de esa persona.
- **FR-025**: La campana MUST vivir fija al pie del riel lateral, con un panel que muestra abiertas
  primero y el estado con su motivo; MUST actualizarse en vivo.
- **FR-026**: La franja superior MUST ser no bloqueante, aparecer una vez por aviso y persona solo
  para artículos con «mostrar franja», esperar si hay una hoja de captura abierta y quitarse sola a
  los 90 s.

**Correo**

- **FR-027**: MUST salir como máximo un correo por ventana de 5 minutos por empresa con todo lo nuevo,
  y ninguno si no hay nada; MUST salir después de confirmar la operación, y un aviso que se abre y se
  resuelve antes de la ventana no se envía.
- **FR-028**: El mismo artículo MUST NOT mandar correo más de una vez en 12 horas.
- **FR-029**: MUST salir un resumen de la mañana a una hora configurable (08:00 por omisión, hora del
  negocio) con los avisos abiertos y la lista de compra, una sola vez por día y empresa aunque el
  servidor se reinicie.
- **FR-030**: Los correos MUST ser solo HTML, con plantilla profesional de la marca, menores de
  102 KB, legibles en teléfono.
- **FR-031**: Los destinatarios por omisión MUST ser las personas activas cuyo rol puede configurar el
  inventario y tienen correo; MUST poderse quitar a cualquiera, agregar correos extra sin usuario y
  elegir por tipo de aviso. Esta lista MUST ser propia, separada de la del cierre de caja.

**Lista de compra**

- **FR-032**: La lista MUST salir de una sola consulta para la pantalla y el correo, con artículo,
  existencia, mínimo, consumo diario medido, lugar de compra si existe, cantidad sugerida y su
  explicación.
- **FR-033**: Cantidad sugerida = consumo diario × días de cobertura − existencia, redondeada hacia
  arriba a empaques enteros del de menor costo por unidad. Sin consumo medido, MUST decirlo y no
  sugerir.

**Pantalla y plataforma**

- **FR-034**: Todo MUST caber sin desborde a 1280×854 y verse bien en 2K, con controles de 44 px y sin
  selectores nativos.
- **FR-035**: Todo dato nuevo MUST estar aislado por empresa y la subida MUST tolerar a la versión
  anterior corriendo.

### Key Entities

- **Configuración de alertas del artículo**: por artículo (insumo o producto) y empresa, con
  sobrescritura por sucursal: activa, mínimo, eventos, franja, días de cobertura, si ya se preguntó.
- **Notificación**: aviso de la empresa con tipo («stock bajo», «se acabó», «no lo encuentro»),
  sucursal, artículo o texto, estado, cifras vistas al abrir, quién la abrió, confirmaciones, motivo
  y hora de cierre, y si ya salió en un correo.
- **Lectura**: qué persona leyó qué notificación (y si ya vio su franja).
- **Destinatario**: persona o correo extra, por tipo de aviso, incluido o quitado.
- **Envío**: registro de cada correo enviado (ventana o resumen de la mañana), para no repetir.
- **Ajuste «se acabó»**: movimiento de existencia con motivo fijo, clave de idempotencia y su
  deshacer.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Desde que el cajero toca «Se acabó» hasta que el aviso aparece en la campana de otra
  persona pasan menos de 5 segundos, y el correo sale en menos de 5 minutos.
- **SC-002**: Reportar «Se acabó» de un artículo de los mosaicos cuesta 2 toques; uno que hay que buscar, escribir parte del nombre y 2 toques.
- **SC-003**: Dos operaciones simultáneas que cruzan el mismo mínimo producen exactamente un aviso en
  el 100 % de las corridas de prueba.
- **SC-004**: Ninguna venta ni compra falla por culpa de un aviso, incluso forzando la falla.
- **SC-005**: El tiempo extra que agrega la detección a guardar un pedido grande se mide y se reporta;
  el objetivo es que quien cobra no lo note (menos de 50 ms en un pedido de 30 renglones).
- **SC-006**: Ningún correo pesa 102 KB o más; nunca salen dos correos de la misma empresa en la misma
  ventana de 5 minutos.
- **SC-007**: Cada cantidad sugerida de la lista de compra trae su explicación en una línea.
- **SC-008**: Todas las pantallas nuevas caben a 1280×854 sin desborde horizontal.

## Assumptions

- Hay conexión en el local (constitución): la campana y la franja exigen red.
- La hora del negocio es la zona configurada de la empresa; si no hay, la del centro de México.
- «Sin conteo reciente» no existe hoy en «Salud del inventario» (solo existencia negativa): aquí se
  define como «ningún conteo cerrado que incluya el artículo en los últimos 30 días».
- «Consumo diario medido» no existe hoy: sale de las salidas por venta (menos las cancelaciones) de
  los últimos 14 días entre 14. Los ajustes «se acabó», mermas, uniones y cambios de unidad no
  cuentan. Con menos de 7 días con movimiento se dice «sin consumo medido».
- El empaque para redondear es el activo de menor costo por unidad base según su última compra; sin
  compras, el más chico activo; sin empaques, la unidad base.
- «Lugar de compra» es el proveedor de la última compra del artículo (el campo de proveedor del
  insumo existe pero nadie lo llena).
- Un producto «Se compra hecho» guarda su existencia en su insumo: sus alertas son las del insumo. La
  sección «Alertas» en la ficha de producto aplica a los de existencia propia.
- El menú en dos niveles (#293) va en otra sesión: aquí «Se acabó» entra como un renglón más del menú
  actual, debajo de «Se echó a perder».
- Fuera de alcance: ocultar productos en plataformas, texto plano en correos, pedidos automáticos,
  corregir recetas, cargar alias o unir duplicados, push y WhatsApp, y migrar los correos existentes a
  las plantillas nuevas.
