# Feature Specification: Un solo arqueo de cajón, y qué entra a él se configura

**Feature Branch**: `015-un-solo-arqueo-de-cajon`

**Created**: 2026-09-10

**Status**: Draft

**Input**: Ver *Origen* al final.

## Contexto

El cajón es **un solo montón de billetes**. El corte, en cambio, pide una cifra declarada por cada
método de pago que toca ese cajón: además del «Efectivo» del mostrador están «Didi efectivo», «Uber
Eats efectivo» y «Rappi efectivo», y los tres cobran dinero que termina en el mismo cajón.

Pedirle a una persona que parta un montón de billetes por canal de venta es pedirle que invente un
reparto. Y el sistema guarda ese reparto inventado como si fuera un hecho.

**Medido en el ambiente de pruebas, turno 3:**

| Método | Esperado | Declarado | Diferencia |
|---|---|---|---|
| Efectivo | 5,221.60 | 5,221.60 | 0.00 |
| Didi efectivo | 64.80 | 0.00 | **−64.80** |

Ese faltante es una de dos cosas y **nadie puede saber cuál**: dinero que no llegó, o dinero que sí
está en el cajón y se contó dentro de los 5,221.60.

Con el conteo por denominaciones ([spec 003](../003-arqueo-denominaciones/spec.md)) el problema
cambió de signo y dejó de depender del criterio del operador: la hoja cuenta el cajón **completo** y
ese total va íntegro al método «Efectivo», así que el dinero de plataforma queda declarado dos veces
—una en el conteo y otra al teclear la cifra de la app— y el corte reporta un **sobrante que no
existe**. Es la misma clase de defecto que el fondo de caja sumado a cada método que tocaba el
cajón, que le inventó $4,500 de faltante a un turno.

## User Scenarios & Testing *(mandatory)*

### User Story 1 — El arqueo del cajón es una sola cifra (Priority: P1)

Quien cierra el turno cuenta el cajón una vez y el sistema compara ese conteo contra **una** cifra
esperada: el fondo con el que abrió, más todo lo que se cobró en efectivo y llegó a ese cajón —venga
del mostrador o de una app—, más las entradas y menos las salidas. La diferencia del arqueo es una
sola. De qué canal vino cada peso lo sigue diciendo el corte, pero como **reporte**, no como algo
que alguien capture.

**Why this priority**: es la corrección de un defecto que hoy está en operación y que el conteo por
denominaciones volvió estructural. Mientras no exista, todo turno con efectivo de plataforma reporta
una diferencia falsa, y una diferencia falsa manda a buscar dinero que está en el cajón.

**Independent Test**: cerrar un turno con una venta en efectivo de mostrador y otra en efectivo de
una app, contar el cajón con lo que suman las dos, y verificar que la diferencia es $0 y que no hay
un faltante y un sobrante cancelándose entre dos métodos.

**Acceptance Scenarios**:

1. **Given** un turno con $500 de fondo, $200 cobrados en efectivo de mostrador y $100 en efectivo
   de una app cuyo dinero llega al cajón, **When** el operador cuenta $800, **Then** la diferencia
   del arqueo es **$0** y es una sola cifra.
2. **Given** el mismo turno, **When** el operador cuenta $750, **Then** hay **un** faltante de $50 —
   no un faltante en un método y un sobrante en otro.
3. **Given** ese turno cerrado, **When** alguien lo revisa después, **Then** puede ver cuánto de ese
   efectivo vino del mostrador y cuánto de cada app.
4. **Given** un método de plataforma cuyo efectivo **no** llega al cajón, **When** se cierra el
   turno, **Then** su dinero no entra al esperado del cajón y no se le pide contarlo.
5. **Given** los métodos que no tocan el cajón (tarjeta, transferencia, plataforma en línea),
   **When** se cierra el turno, **Then** siguen declarándose cada uno con su propia cifra.

---

### User Story 2 — Configurar los métodos de pago (Priority: P2)

El dueño o el gerente activa y desactiva métodos de cobro, y define si el efectivo de cada uno entra
al cajón. Eso último existe porque el reparto de un pedido de app pagado en efectivo lo hace **a
veces** gente del local —y el dinero regresa al cajón— y a veces el repartidor de la plataforma, que
se lo lleva y lo descuenta del depósito. Depende de la plataforma, y hoy cambiarlo exige a un
desarrollador.

**Why this priority**: sin esto, el esperado del cajón de US1 sale de una configuración que nadie
puede corregir. Va en P2 y no en P1 porque US1 ya es correcta con la configuración actual del
negocio; esto es lo que la vuelve correcta para el siguiente cliente, o para el día en que una
plataforma cambie de forma de operar.

**Independent Test**: apagar un método, verificar que deja de ofrecerse al cobrar, y volver a
prenderlo. Cambiar el interruptor de cajón de una plataforma y ver el esperado del cajón moverse en
consecuencia.

**Acceptance Scenarios**:

1. **Given** un método activo, **When** el gerente lo desactiva, **Then** deja de ofrecerse para
   cobrar pedidos nuevos.
2. **Given** un método que ya cobró dinero en el turno abierto, **When** se desactiva, **Then** el
   arqueo **sigue esperando** ese dinero: apagarlo no lo desaparece.
3. **Given** un corte ya cerrado, **When** después se cambia cualquiera de los dos interruptores,
   **Then** las cifras de ese corte no cambian.
4. **Given** un método de plataforma en efectivo, **When** el gerente marca que su efectivo **no**
   llega al cajón, **Then** el esperado del cajón baja en lo cobrado con él y ese dinero pasa a
   conciliarse con la liquidación de la plataforma.
5. **Given** un cambio en cualquiera de los dos interruptores, **When** se consulta la bitácora del
   servidor, **Then** aparece quién lo cambió y cuándo.

---

### User Story 3 — Arqueo ciego (Priority: P3)

El negocio decide si quien cuenta el cajón ve lo que el sistema espera. Con el arqueo ciego
encendido, el operador cuenta sin ver el esperado ni la diferencia, y la diferencia aparece **después
de confirmar** el cierre.

**Why this priority**: es un control contra que alguien acomode lo que declara para que cuadre. No
desbloquea la operación diaria y el negocio de hoy opera con una sola persona de confianza, pero es
la clase de ajuste que se pide cuando entra personal nuevo o un segundo local.

**Independent Test**: encender el interruptor, abrir el cierre y verificar que no aparece ninguna
cifra esperada ni diferencia hasta confirmar; apagarlo y verificar que vuelve a verse antes.

**Acceptance Scenarios**:

1. **Given** el arqueo ciego apagado, **When** el operador captura el conteo, **Then** ve la
   diferencia antes de confirmar (el comportamiento de hoy).
2. **Given** el arqueo ciego encendido, **When** el operador captura el conteo, **Then** no ve el
   esperado ni la diferencia en ningún punto antes de confirmar.
3. **Given** el arqueo ciego encendido, **When** el operador confirma el cierre, **Then** la
   diferencia se muestra.
4. **Given** el arqueo ciego encendido, **When** el operador cuenta por denominaciones, **Then**
   sigue viendo el total de lo que él contó: eso es lo que capturó, no lo que el sistema espera.

**Esta historia enmienda un requisito de la 003.** FR-005 de
[003](../003-arqueo-denominaciones/spec.md) exige que la diferencia se vea **antes** de confirmar, y
está implementado así. Las dos reglas protegen cosas distintas —FR-005 al operador honesto de su
propio error de suma; el arqueo ciego al negocio de quien no lo es— así que se vuelve configuración
del negocio en vez de decisión de producto. La enmienda se escribe en la 003; no se corrige por
debajo.

### Edge Cases

- **Se desactiva un método a media jornada, con ventas ya cobradas.** El arqueo tiene que seguir
  esperando ese dinero. Quitarlo del corte porque el método está apagado hace desaparecer dinero que
  sí entró, y hoy el corte solo considera los métodos activos: es el camino que ya existe, no uno
  hipotético.
- **Se cambia el interruptor de cajón a media jornada, después de que el operador contó.** El
  esperado del cajón se movería bajo sus pies. *Resuelto el 2026-09-10 por FR-017*: se rechaza el
  cambio mientras ese método ya haya cobrado en el turno abierto. Ni al turno abierto ni al
  siguiente — no se aplica, se pide cerrar el turno primero.
- **Un corte histórico con un método que hoy está desactivado.** Se sigue mostrando: sus cifras
  quedaron guardadas y borrarlo de la vista reescribiría el pasado.
- **Se desactiva el único método de efectivo.** *Resuelto el 2026-09-10, después de medirlo.* Deja
  de ofrecerse para cobrar, como cualquier otro, **pero su renglón no se cae del arqueo**: es el
  único dueño del fondo de apertura y de los movimientos de caja, y sin él el fondo no tiene dónde
  vivir. Medido: apagarlo con $500 de fondo dejaba al cajón esperando $0 con los billetes adentro y
  el corte cerraba con $500 de sobrante. El cierre sigue pidiendo contar mientras el cajón espere
  algo — si no espera nada, no obliga, que es el mismo criterio de cualquier método sin movimiento.
  Lo cubre `TestDesactivarElEfectivoNoBorraElFondoDelArqueo`.
- **Arqueo ciego con el conteo por denominaciones.** El desglose que el operador captura no revela
  lo esperado, así que puede seguir viéndose; lo que se oculta es la comparación.
- **Se enciende el arqueo ciego con un cierre a medio capturar.** No puede dejar la pantalla en un
  estado donde ya se vio la diferencia y ahora se oculta.
- **Dos personas cambian el mismo interruptor a la vez.** Gana uno y el otro tiene que enterarse, no
  quedarse con la pantalla diciendo lo contrario de lo que quedó.
- **Un turno sin efectivo de ninguna clase.** No hay cajón que contar y el cierre no debe exigirlo.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: El arqueo de efectivo MUST comparar el conteo del cajón contra **una sola** cifra
  esperada: fondo de apertura + lo cobrado con cada método cuyo efectivo llega al cajón + entradas −
  salidas.
- **FR-002**: El cierre MUST NOT pedir una cifra declarada por cada método cuyo efectivo llega al
  cajón. Lo que el operador declara del cajón es **una** cifra: la que contó.
- **FR-003**: La diferencia del arqueo de efectivo MUST ser una sola. El sistema MUST NOT producir
  un faltante en un método y un sobrante en otro cuando los dos comparten el mismo cajón.
- **FR-004**: El corte MUST seguir informando cuánto se cobró en efectivo por canal —mostrador y
  cada plataforma— como reporte derivado de las ventas.
- **FR-005**: Los métodos cuyo dinero **no** está en el cajón MUST seguir declarándose cada uno con
  su propia cifra.
- **FR-006**: Un usuario con rol de administración MUST poder activar y desactivar un método de
  cobro.
- **FR-007**: Un usuario con rol de administración MUST poder definir, por método, si su efectivo
  llega al cajón.
- **FR-008**: Desactivar un método MUST NOT quitar del arqueo el dinero ya cobrado con él, ni
  cambiar las cifras de un corte ya cerrado.
- **FR-009**: Un método desactivado MUST NOT ofrecerse para cobrar pedidos nuevos.
- **FR-017**: Cambiar si el efectivo de un método llega al cajón MUST rechazarse mientras ese método
  ya haya cobrado en el turno abierto, con un mensaje que diga qué hacer.

  **Encontrado midiendo, el 2026-09-10, y en rojo antes del arreglo.** Hasta esta feature nadie
  podía mover ese interruptor desde la aplicación, así que el camino no existía. Apagarlo a media
  jornada bajaba el esperado del cajón de $835 a $700: $135 de billetes que la app ya había traído
  y que siguen físicamente en el cajón dejaban de pedirse, y el corte cerraba con un **sobrante
  fantasma** que nadie audita porque un sobrante no duele. Es la misma falla que FR-008 cierra para
  «activo», entrando por el otro interruptor.

  Se rechaza en vez de recordar el valor que tenía cada cobro porque **los cortes cerrados ya
  conservan su forma** —cada renglón guarda su propio `affects_cash_drawer` desde esta feature— y
  la única ventana ambigua que quedaba era el turno abierto. Cerrarla cuesta una validación en vez
  de una migración, y no cierra la puerta a guardar el dato por cobro el día que haga falta.
- **FR-018**: Devolver dinero de un método cuyo efectivo llega al cajón MUST registrar la salida de
  caja, aunque el método sea de una plataforma.

  Enmienda escrita a D2 de la [spec 007](../007-devolver-el-dinero/spec.md), que decidió que la
  devolución sale del cajón "cuando el cobro fue en efectivo" y dejó fuera a las plataformas porque
  "ese dinero nunca estuvo en la caja". Era cierto cuando se escribió. Con esta feature quien decide
  es el interruptor «va al cajón», no el tipo del método: devolverle $135 a un cliente de Didi que
  pagó en efectivo sacaba los billetes del cajón sin registrar la salida, y el corte cerraba con un
  faltante de $135 —el defecto que la 007 vino a cerrar, entrando por la otra puerta.
- **FR-010**: Todo cambio a cualquiera de los dos interruptores MUST quedar registrado en la
  **bitácora del servidor** con quién y cuándo: es configuración con impacto directo en la
  reconciliación del dinero. No se construye una pantalla de auditoría — prometer una pantalla que
  no se va a construir es peor que no prometerla.
- **FR-011**: El negocio MUST poder encender y apagar el arqueo ciego.
- **FR-012**: Con el arqueo ciego encendido, la pantalla de cierre MUST NOT mostrar lo esperado ni
  la diferencia antes de confirmar.
- **FR-013**: Con el arqueo ciego encendido, la diferencia MUST mostrarse después de confirmar el
  cierre.
- **FR-014**: Con el arqueo ciego encendido, el operador MUST poder seguir viendo el total de lo que
  él mismo capturó.
- **FR-015**: La enmienda a FR-005 de la spec 003 MUST quedar escrita en esa spec, nombrando que el
  requisito pasa a depender de un ajuste del negocio.
- **FR-016**: El cierre MUST permanecer bloqueado hasta que todo método que exige captura la tenga,
  **con y sin arqueo ciego**, y un campo vacío MUST NOT tratarse como cero.

  **Esta regla es anterior a esta feature y nunca se había escrito en un spec.** Vive en el
  comentario de `cierreDeCaja.ts` y en su test desde el corte que registró $1,662 de faltante que no
  era dinero perdido: nadie capturó el efectivo y el campo vacío se guardó como cero. Se escribe aquí
  porque la 015 le cambia el mecanismo —de deducirlo del esperado a preguntárselo al servidor— y una
  regla no escrita se pierde justo cuando su mecanismo cambia. Sin ella, el arqueo ciego habilita el
  botón de cerrar con la pantalla en blanco.

### Key Entities

- **Método de pago**: cómo se cobra. Ya existe. Gana dos interruptores operables desde la
  aplicación: si está activo, y si su efectivo llega al cajón. Cada plataforma tiene su propio
  método de efectivo, así que "depende de la plataforma" se configura ahí sin inventar un concepto
  nuevo.
- **Ajuste del negocio**: dónde vive el interruptor del arqueo ciego. Es por empresa.
- **Arqueo del cajón**: la comparación entre lo contado y lo esperado de un turno. Deja de ser una
  comparación por método para el efectivo y pasa a ser una sola.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Un turno con efectivo de mostrador **y** de plataforma se cierra con **una** sola
  diferencia, y esa diferencia es $0 cuando el cajón trae exactamente lo esperado.
- **SC-002**: Ningún corte nuevo reporta simultáneamente un faltante y un sobrante entre métodos que
  comparten el cajón.
- **SC-003**: El dueño cambia si el efectivo de una plataforma llega al cajón, y si un método se
  ofrece o no, **sin ayuda de un desarrollador**.
- **SC-004**: Con el arqueo ciego encendido, **ninguna pantalla del cierre** —ni la respuesta que las
  alimenta— muestra el esperado del cajón antes de confirmar.
- **SC-005**: Los cortes cerrados antes de este cambio se siguen consultando **con las mismas
  cifras**.
- **SC-006**: El cierre de un turno con varios métodos de efectivo no toma más pasos que hoy — no se
  cambia una captura inventada por dos pantallas más.

## Assumptions

- **Cada plataforma ya tiene su propio método de efectivo**, así que configurar "por plataforma" es
  configurar por método.
- **El efectivo que se lleva el repartidor de la app** se concilia con la liquidación de la
  plataforma ([014](../014-folio-y-comision-de-plataforma/spec.md)), que ya registra lo que la
  plataforma cobró y depositó. Este spec no cambia esa conciliación.
- **El arqueo ciego aplica a todo lo que el operador declara en el cierre**, no solo al efectivo:
  ver el esperado de la tarjeta permite el mismo acomodo.
- **La diferencia, con arqueo ciego, se muestra a quien cierra** al confirmar — el mismo diálogo de
  hoy. No se restringe a un rol: el control es que no la vea *antes*, no que no la vea nunca.
- **El interruptor del arqueo ciego es por empresa**, como el resto de los ajustes del negocio.
- **Un gerente o un administrador pueden derivar el esperado desde otra pantalla**, y se acepta. La
  pantalla de Ventas está abierta a esos dos roles y muestra la venta en efectivo del día, así que
  quien tiene ese acceso puede estimar lo que el cajón debería tener antes de contarlo. El control
  apunta a quien está en el mostrador —rol cajero, que no alcanza Ventas—; sostener que nadie puede
  verlo dejaría un criterio de éxito imposible de probar. Cerrar esa vía exigiría bloquear una
  pantalla legítima en su uso normal, y seguiría siendo evadible con los datos de ayer.
- Un cambio de configuración aplica **al siguiente cierre**, no a uno a medio capturar.

## Out of Scope

- Recontar o corregir un arqueo ya firmado. Un arqueo no se edita ni se borra desde la aplicación, y
  eso no cambia aquí.
- Contar el cajón por sobres o buckets separados. El efectivo de plataforma o llega al cajón o no
  llega; no hay un tercer lugar que contar aparte.
- Cambiar cómo se concilia el depósito de una plataforma.
- Un método de pago nuevo, o cambiar el tipo (`kind`) de uno existente.
- Restringir por rol quién ve la diferencia después de cerrar.
- **Que el que cuenta no sea el que ve.** Es el control de verdad —el segundo par de ojos que la 003
  también dejó fuera— y es lo único que haría el arqueo ciego efectivo contra un gerente. No se
  construye hoy y **no se cierra**: se agrega después decidiendo quién ve qué, sin tocar un dato ni
  partir una fila.

## Origen

Salió de una pregunta del dueño sobre el conteo de efectivo: si hay «Didi efectivo» o «Rappi
efectivo» y ese dinero entra al mismo cajón, cómo lo resuelven otros negocios. La respuesta de la
industria —un cajón, un conteo, el canal como reporte— es US1. Al revisarlo aparecieron las otras
dos: que los interruptores existen en la base y nadie puede cambiarlos, y que el arqueo ciego, que
el dueño pidió, contradice un requisito de la 003.
