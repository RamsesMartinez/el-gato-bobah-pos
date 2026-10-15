# Feature Specification: Merma y faltante sin explicar

**Feature Branch**: `039-merma-y-cortesias`

**Created**: 2026-10-15

**Status**: Draft

**Input**: Issue #66 «Merma y cortesías: registro con motivo y faltante sin explicar después del
conteo», decidido por el dueño el 14-oct-2026 como feature aparte del 038; y las decisiones del
dueño del 9-oct (bloque 8, puntos 29, 30, 34 y 35), fuera del repositorio.

## Contexto

Hoy lo que se pierde en el local —se cayó, caducó, salió mal en cocina, se lo tomó el personal— no
se registra. Solo aparece como diferencia cuando se cuenta (038), mezclado con todo lo demás, así
que nadie sabe si un faltante es desperdicio normal, porciones mal servidas, una receta mal
configurada o robo.

Esta feature hace dos cosas: registrar la merma **cuando pasa**, con su motivo, y después de cada
conteo separar la diferencia en **lo que ya se sabía** (la merma registrada) y **lo que nadie
explicó**.

## Clarifications

### Session 2026-10-15

- Q: ¿Las cortesías de promoción (producto a $0 en el pedido, ligado a una promo con vigencia y etiqueta) entran en esta feature? → A: No. Van en su propia feature: tocan el pedido, el ticket, Ventas y el cobro. Esta queda en merma y faltante sin explicar.
- Q: ¿Una merma de costo alto espera aprobación antes de restar? → A: Sí, como se decidió el 9-oct (punto 34): arriba de un tope en pesos que configura cada empresa, la merma queda pendiente y no resta hasta que alguien con permiso de aprobar la aprueba.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Registrar lo que se echó a perder (Priority: P1)

Quien está en el local ve que un insumo se echó a perder (la leche se cortó, se cayó una bolsa de
perlas) o que un producto preparado no se pudo vender (un frappé salió mal). Desde la tableta,
elige el motivo, el insumo o el producto y la cantidad, y guarda. El almacén resta en ese momento y
queda quién, cuándo y por qué.

**Why this priority**: sin el registro no hay nada que separar después del conteo; es la mitad que
hoy no existe. Sola ya da valor: lo guardado deja de bajar «sin razón».

**Independent Test**: registrar una merma de un insumo con 10 guardados, por 2, con motivo
«Caducó»: lo guardado queda en 8, y la merma aparece con su motivo, quién y cuándo.

**Acceptance Scenarios**:

1. **Given** un insumo con 10 guardados, **When** alguien con permiso registra 2 con el motivo
   «Caducó», **Then** quedan 8 y el registro dice el motivo, quién y cuándo.
2. **Given** un insumo que llega en «Caja · 12 L», **When** se registra «1 caja», **Then** resta
   12 L, igual que en compras y conteo.
3. **Given** un producto con receta, **When** se registra que se echó a perder 1, **Then** resta lo
   mismo que restaría venderlo, y no cuenta como venta.
4. **Given** que el motivo no está en la lista, **When** se escribe uno nuevo con su descripción,
   **Then** se guarda y queda como opción la próxima vez.
5. **Given** una merma registrada hoy por error, **When** quien la registró la deshace, **Then**
   lo guardado vuelve a como estaba y el registro queda como deshecho, no borrado.
6. **Given** una empresa con tope de aprobación, **When** se registra una merma que cuesta más que
   el tope, **Then** queda «por aprobar», no resta, y quien registró ve que espera aprobación.
7. **Given** una merma por aprobar, **When** alguien con permiso la aprueba, **Then** resta en ese
   momento y queda quién aprobó; **When** la rechaza, **Then** no resta y queda quién rechazó.

---

### User Story 2 - Ver el faltante sin explicar después de un conteo (Priority: P2)

Al cerrar un conteo, quien administra ve, por insumo, lo que se perdió desde el conteo anterior
partido en dos: lo **registrado como merma** (ya se había restado de lo que el sistema tenía) y el
**faltante sin explicar**, que es lo que el conteo encontró de menos. En cantidad y en pesos.

**Why this priority**: es lo que el dueño quiere saber (¿desperdicio normal o algo más?). Depende
de que exista la merma (US1) y de que haya dos conteos.

**Independent Test**: con dos conteos cerrados de un insumo y 3 de merma registrada entre ellos
(que ya restaron), si el segundo conteo encontró 2 menos de lo que el sistema tenía, el reporte dice
5 perdidos: 3 registrados y 2 sin explicar, en pesos al costo del cierre.

**Acceptance Scenarios**:

1. **Given** un conteo cerrado con un conteo anterior del mismo insumo, **When** se abre el
   reporte, **Then** cada insumo contado muestra lo perdido en el periodo, cuánto de eso se
   registró como merma y cuánto quedó sin explicar, en cantidad y en pesos.
2. **Given** que el conteo encontró de más (sobraba), **When** se ve el reporte, **Then** lo dice
   como «sobraba», no como un faltante negativo.
3. **Given** el primer conteo de la sucursal, **When** se abre el reporte, **Then** dice que no hay
   periodo con qué comparar, sin inventar cifras.
4. **Given** un insumo sin costo, **When** aparece en el reporte, **Then** muestra «sin costo» y no
   $0.

---

### User Story 3 - Merma por motivo y los motivos de la empresa (Priority: P3)

Quien administra ve cuánto se perdió en un periodo, por motivo (con «consumo del personal»
aparte), en cantidad y en pesos; y edita, archiva o junta los motivos de su empresa.

**Why this priority**: sirve para decidir (cambiar de proveedor, capacitar, ajustar porciones),
pero el dato ya queda guardado desde US1; el reporte se puede agregar después al mismo costo.

**Independent Test**: con mermas de dos motivos en la semana, el reporte las suma por motivo y el
total coincide con la suma de los registros no deshechos.

**Acceptance Scenarios**:

1. **Given** mermas de la semana con motivos «Caducó» y «Consumo del personal», **When** se abre
   el reporte de la semana, **Then** cada motivo tiene su total y el consumo del personal sale en
   su propio renglón.
2. **Given** dos motivos que significan lo mismo, **When** se juntan, **Then** el reporte los suma
   bajo el que se conserva, el otro deja de ofrecerse, y cada registro sigue diciendo su motivo.
3. **Given** un motivo archivado, **When** se registra una merma nueva, **Then** ya no se ofrece,
   pero las mermas viejas siguen diciendo su motivo.

### Edge Cases

- **Cantidad vacía, cero o negativa**: no se guarda; se pide una cantidad mayor que cero. Una
  cantidad absurda se rechaza con el mismo tope que compras y conteo.
- **Más de lo que hay guardado**: se acepta (lo guardado puede ser negativo, heredado), y el
  registro lo resta igual; el conteo lo corrige.
- **Producto sin receta ni nada que restar**: no se puede registrar como merma; la pantalla dice
  que capture su receta o registre los insumos.
- **Insumo preparado** (crema batida, perlas cocidas): resta el preparado, no los insumos con que se
  hizo.
- **Merma mientras hay un conteo abierto**: si el insumo ya se capturó en el conteo, la merma se
  resta de lo contado al cerrar, como lo vendido mientras se cuenta (038); si no se ha capturado,
  la cifra que se cuente ya la incluye.
- **De qué periodo es una merma**: la registrada antes de capturar el insumo en un conteo cuenta
  para el periodo que ese conteo cierra; la posterior, para el siguiente.
- **Deshacer**: solo el mismo día y solo si ese insumo no se ha contado después; pasado eso, la
  corrección la hace el siguiente conteo. Deshacer devuelve exactamente lo que se restó.
- **Dos tabletas registran lo mismo a la vez**: son dos mermas; cada una resta la suya.
- **Una venta en curso del mismo insumo**: registrar no espera a la venta ni la hace esperar.
- **Dos sucursales sin selector**: igual que compras y conteo, la merma no adivina la sucursal.
- **Motivo nuevo sin descripción, o con un nombre que ya existe** (sin distinguir mayúsculas,
  acentos ni espacios): no se crea; con el nombre repetido se ofrece el que ya existe.
- **Juntar un motivo con otro de otra empresa**: imposible; los motivos son de cada empresa.
- **Sin tope configurado**: ninguna merma pide aprobación.
- **Insumo sin costo con tope configurado**: no se sabe cuánto vale, así que queda por aprobar.
- **Merma por aprobar cuando se cierra un conteo que contó su insumo**: el conteo ya reflejó la
  pérdida; aprobarla después la restaría dos veces. Queda cerrada como «la cubrió el conteo» y ya no se puede aprobar.
- **Aprobar la propia merma**: se permite solo a quien tiene el permiso de aprobar; queda en la
  bitácora igual que cualquier aprobación.
- **Dos personas aprueban o rechazan la misma a la vez**: una gana; la otra ve que ya se resolvió.

## Requirements *(mandatory)*

### Functional Requirements

**Registrar (US1)**

- **FR-001**: Quien tiene el permiso de registrar merma MUST poder registrar desde la tableta una
  merma de un insumo o de un producto, con cantidad, motivo, y opcionalmente una nota.
- **FR-002**: Registrar MUST restar de lo guardado en ese momento, con un movimiento de merma ligado
  al registro, en la sucursal de quien registra.
- **FR-003**: La cantidad de un insumo MUST poder capturarse en sus empaques o en la unidad que se
  usa (como en compras y conteo), y guardarse en la unidad del insumo.
- **FR-004**: La merma de un producto MUST restar lo mismo que restaría venderlo (su receta, su
  paquete o su existencia propia) y MUST NOT contar como venta ni aparecer en Ventas.
- **FR-005**: Cada registro MUST guardar quién, cuándo, el motivo, la cantidad y el costo de ese
  momento (por unidad), para que el pasado no cambie cuando cambia el costo.
- **FR-006**: Los motivos MUST ser de cada empresa. Una empresa nueva arranca con una lista sugerida
  (caducó, se cayó, mal preparado, prueba de receta, consumo del personal), cada uno con su
  descripción, que la empresa puede cambiar.
- **FR-007**: Si el motivo no está, MUST poder crearse ahí mismo con nombre y descripción
  obligatoria; un nombre que ya existe (sin distinguir mayúsculas, acentos ni espacios) MUST NOT
  crearse dos veces.
- **FR-008**: Los motivos más usados de la empresa MUST ofrecerse como botones grandes, y el resto
  en una hoja con buscador; nunca un selector nativo.
- **FR-009**: Quien registró una merma, o quien tenga el permiso de administrar el almacén, MUST
  poder deshacerla el mismo día mientras ese insumo no se haya contado después; deshacer devuelve
  lo restado con un movimiento propio y deja el registro marcado como deshecho, nunca borrado. Una
  merma por aprobar se deshace sin devolver nada (no había restado).

**Aprobación (US1)**

- **FR-010**: Cada empresa MUST poder configurar un tope en pesos para aprobar mermas; sin tope,
  ninguna pide aprobación. El valor de cada negocio es configuración, no código.
- **FR-011**: Una merma cuyo costo (cantidad por costo de ese momento) pase el tope, o cuyo insumo no
  tenga costo, MUST quedar por aprobar y MUST NOT restar hasta que se apruebe.
- **FR-012**: Quien tiene el permiso de aprobar mermas MUST ver las que esperan aprobación de su
  sucursal y poder aprobarlas (resta en ese momento) o rechazarlas (no resta), quedando quién y
  cuándo. Una merma resuelta no se vuelve a resolver.
- **FR-013**: Una merma por aprobar MUST cerrarse como «la cubrió el conteo», y no poder aprobarse
  después, cuando se cierra un conteo que contó alguno de los insumos que restaría **y la merma se
  registró antes de que ese insumo se capturara**. Una registrada después de la captura no la vio
  el conteo y sigue por aprobar. Un conteo descartado no cubre nada.

**Faltante sin explicar (US2)**

- **FR-014**: Al cerrar un conteo, el sistema MUST ofrecer, por cada insumo contado que tenga un
  conteo cerrado anterior, la merma registrada en el periodo, el faltante sin explicar (lo que
  el conteo encontró de menos contra lo que el sistema tenía al capturarlo) y el total perdido (la
  suma de los dos), en cantidad y en pesos al costo de cuando se cerró el conteo. La merma registrada ya restó de lo que
  el sistema tenía: restarla otra vez del faltante del conteo la contaría dos veces.
- **FR-015**: El periodo de un insumo MUST ir de lo que vio el conteo anterior al capturarlo a lo
  que vio este conteo al capturarlo, contado por el orden de los movimientos del almacén y no por la
  hora: una merma que confirmó después de la captura queda en el periodo siguiente aunque haya
  empezado antes. Cuenta la merma que RESTÓ (al registrarse o al aprobarse); la deshecha, la
  rechazada y la que cubrió el conteo no cuentan.
- **FR-016**: Lo que sobraba MUST decirse como «sobraba»; un insumo sin costo MUST decir «sin costo»;
  el primer conteo MUST decir que no hay con qué comparar.
- **FR-017**: El reporte MUST poder verse otra vez para cualquier conteo cerrado, con las mismas
  cifras (cantidades y costo se fijan al cerrar el conteo; un conteo de antes de esta feature dice «sin costo guardado»).

**Por motivo y motivos (US3)**

- **FR-018**: Quien tiene el permiso de ver el almacén MUST poder ver la merma de un periodo por
  motivo, en cantidad por insumo y en pesos al costo del registro, con «consumo del personal» en su
  propio renglón.
- **FR-019**: Quien administra el almacén MUST poder renombrar, cambiar la descripción, archivar y
  juntar motivos. Archivar deja de ofrecerlo sin cambiar lo registrado; juntar deja de ofrecer el
  motivo y el reporte suma sus registros bajo el que se conserva, pero cada registro conserva su
  motivo original (no se reescribe el pasado).

**Transversales**

- **FR-020**: Registrar, deshacer y administrar motivos MUST pedir permisos verificados en el
  servidor, no nombres de rol. Registrar merma lo pueden hacer por omisión todos los roles de la
  tableta: admin, gerente, cajero y mesero (decisión del 9-oct: cualquiera del local). La cocina
  registra con la sesión del rol que tenga su tableta; no se crea un rol de cocina. Aprobar mermas,
  administrar motivos y el tope: admin y gerente.
- **FR-021**: Registrar, aprobar, rechazar y deshacer una merma, y cambiar el tope, MUST dejar su
  evento en la bitácora de auditoría, en la misma operación.
- **FR-022**: Ninguna lista fija de un negocio en el código ni en migraciones: los motivos
  sugeridos son una semilla editable.
- **FR-023**: Los textos de la pantalla MUST usar palabras de quien opera. Todo lo de esta feature se
  llama «Se echó a perder» en pantalla (riel, captura, pestaña de Almacén, por aprobar); «merma»
  queda como nombre interno. Se dice «tienes», «faltaba», «sobraba», «sin explicar»; nunca
  «movimiento», «ajuste» ni «kardex».

### Key Entities

- **Motivo de merma**: de una empresa; nombre, descripción, si está archivado, y cuántas veces se
  ha usado (para ordenar los botones). Puede juntarse con otro.
- **Registro de merma**: de una sucursal; insumo o producto, cantidad (en la unidad del insumo, o
  piezas de producto), motivo, nota, costo por unidad de ese momento, quién y cuándo, y su estado:
  restó, por aprobar, rechazada, cubierta por el conteo o deshecha (con quién y cuándo de cada
  cambio).
- **Tope de aprobación**: de una empresa; un monto en pesos o ninguno.
- **Faltante sin explicar**: no se guarda; se calcula por insumo de un conteo cerrado contra el
  anterior y la merma del periodo.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Registrar una merma de un motivo frecuente y un insumo reciente toma 3 toques o menos
  desde la pantalla de inventario, sin contar teclear la cantidad.
- **SC-002**: Después de un conteo, el dueño ve en una sola pantalla cuánto del faltante estaba
  explicado y cuánto no, por insumo y en pesos.
- **SC-003**: Insumo por insumo, lo perdido en el periodo es exactamente la merma registrada más lo
  que el conteo encontró de menos; ninguna de las dos se cuenta dos veces.
- **SC-004**: Ninguna merma deshecha ni ninguna de otra sucursal o empresa aparece en los reportes.
- **SC-005**: Una persona nueva registra una merma sin ayuda en la prueba con usuarios sin contexto.

## Assumptions

- Por omisión una empresa nueva no tiene tope (ninguna merma pide aprobación); El Gato Bobah pone el
  suyo en su configuración.
- El dinero del faltante se fija con el costo al cerrar el conteo (ajuste de la revisión de
  arquitectura: con «costo de hoy» un reporte cerrado cambiaba solo); el de la merma por motivo, al
  costo de cuando se registró. Una merma de un insumo sin costo toma el costo vigente al aprobarse.
- En pantalla todo se llama «Se echó a perder» (el riel, la captura y la pestaña de Almacén); «merma»
  queda como nombre interno y en este documento.
- Un solo almacén por sucursal (más de uno es otra feature, issue #65); la merma es de la sucursal.
- El costo de lo que se tira de un producto se calcula con su receta al costo de ese momento.
- La merma de un producto no lleva empaques: se registra en piezas.
- Fuera de alcance: la cortesía de promoción (su propia feature), consumo teórico contra real (otro
  reporte), valor del inventario y traspasos.
