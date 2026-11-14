# Feature Specification: Rediseño del módulo Caja

**Feature Branch**: `082-rediseno-de-caja`

**Created**: 2026-11-13

**Status**: Draft

**Input**: Issue #290 (lo subió el dueño el 2026-11-13). Relacionados: #251 (cierre sin decir qué
falta), #175 (devoluciones de gasto), #289 (correcciones que hoy se hacen con SQL).

## Contexto

Hacer un corte en Caja es confuso y la pantalla puede dar cifras que se contradicen. El caso que lo
disparó: un gasto en efectivo salió de la Caja B, quien cerró creyó que había salido de la A y contó
mal la B. El corte de la B quedó con la conciliación diciendo «Efectivo: dif. $0», el arqueo
diciendo «contado ≠ esperado» y el renglón Total con una diferencia aunque sistema y declarado
coincidían. Al abrir el turno siguiente el cajero contó bien, eligió «el conteo anterior estaba mal»
y explicó qué pasó, pero **esa nota no aparece en ninguna pantalla ni la devuelve el MCP**, y el
corte viejo sigue con la diferencia sin explicación y sin forma de corregirse con rastro.

La auditoría del issue (21 hallazgos con evidencia `archivo:línea`) califica de 2 sobre 5 la
claridad del cierre, la separación de componentes y la consistencia de datos, y de 1.5 el uso de
pantallas grandes: Caja tiene un tope de 920 px y en un monitor 2K deja vacío ~65 % de la pantalla.

## Entregas

Un solo spec, **cuatro entregas que se suben solas y en este orden**. Cada una deja el sistema
completo y coherente sin la siguiente.

| Fase | Qué entrega | Historias | Esquema |
| --- | --- | --- | --- |
| 1. Lectura y claridad | Una sola cifra de diferencia, vocabulario y formato únicos, el detalle del corte completo (quién, fondo, retiro, motivo de apertura) y el atajo de tema | US1, US2, US6 | Sin cambios |
| 2. Partir sin cambiar comportamiento | Submódulos en pantalla y servidor; URL propia de cada caja, corte y de Administrar; ajustes de caja juntos | US7 | Sin cambios |
| 3. Correcciones con rastro | Corregir un corte cerrado sin reescribir lo firmado | US3 | Una tabla nueva, aditiva |
| 4. Cierre por pasos, X/Z y pantallas grandes | Asistente de cierre, Corte X contra Corte Z, Operar contra Cerrar, y disposición por ancho hasta 2560 px | US4, US5 | Una configuración nueva (tolerancia), aditiva |

## Decisiones pendientes del dueño (se deciden en la vuelta de maquetas)

Son las cuatro «Dudas para el spec» del issue. Cada una lleva la recomendación con la que se
escribió este spec; si el dueño decide otra cosa, se cambia aquí y en `plan.md` antes de seguir.

| # | Duda | Recomendación | Por qué |
| --- | --- | --- | --- |
| P1 | ¿Tolerancia de diferencia y quién aprueba fuera de ella? | **Tolerancia por empresa en pesos, configurada por quien tiene configurar caja, $0 por omisión. Sin PIN y sin bloquear el cierre.** Un corte que la pasa queda «por explicar» en el Histórico hasta que alguien deja la explicación: quien cierra viendo el esperado la escribe en el último paso; con arqueo ciego, la deja después quien tiene ajustar el arqueo (una nota, US3), o la deja sola la apertura siguiente con su motivo | **Consecuencia que no se ve**: con arqueo ciego quien cierra no conoce la diferencia, así que no puede explicarla, y pedirle un motivo «porque pasó la tolerancia» le revela que hay diferencia y le deja recontar hasta cuadrar, que es justo lo que el arqueo ciego impide. Un PIN de encargado detiene la caja cuando el encargado no está, contra la regla de que quien opera no decide en el momento. $0 por omisión para que nada pase en silencio |
| P2 | ¿El motivo y la nota de apertura los ve el cajero o solo quien tiene configurar caja? | **Los ve todo el que ve el corte.** Lo que sigue oculto por el arqueo ciego son los montos (esperado y diferencia) | El motivo no revela cuánto se esperaba: quien abrió ya supo que su conteo no coincidía, porque el sistema le pidió el motivo. Ocultarlo al cajero que lo escribió no protege nada y le impide ver que su explicación quedó guardada |
| P3 | ¿Reabrir el último turno, o solo ajustes? | **Solo ajustes.** No se reabre un corte | Reabrir reescribe lo firmado (conteo, totales, fondo, retiro y la asistencia ligada al cierre) y vuelve a mover el esperado de la apertura siguiente. Un ajuste resuelve el caso del issue sin tocar ninguna cifra firmada, y reabrir se puede agregar después al mismo costo |
| P4 | ¿La diferencia va a una cuenta «Sobrantes y faltantes» en reportes? | **No como cuenta contable.** El Histórico suma la diferencia de los cortes del periodo filtrado, ya con sus correcciones, en un renglón propio que no es venta ni gasto | Una cuenta contable es una feature de reportes con su propio diseño. La suma del periodo resuelve la pregunta de hoy (¿cuánto faltó este mes?) y no se cuenta como ingreso, como pide el principio III |

## User Scenarios & Testing *(mandatory)*

### User Story 1 - El corte dice una sola diferencia, rotulada y sin contradicciones (Priority: P1) — Fase 1

Quien revisa un corte cerrado (en la lista, en el detalle o en el diálogo «Caja cerrada») ve la
misma diferencia en los tres lugares, partida en «Cajón (arqueo)», «Otros métodos (conciliación)» y
«Total». En la conciliación el renglón de efectivo no dice «$0»: dice «ver arqueo». Toda tabla usa
las mismas palabras en el mismo orden: Esperado · Contado · Diferencia. La diferencia se lee con
palabra, signo, ícono y color: «Faltante −$120.00 ▼», «Sobrante +$35.50 ▲», «Cuadra ✓». Los montos
llevan siempre dos decimales y se alinean a la derecha.

**Why this priority**: es el defecto que hizo cerrar mal la Caja B. Una pantalla que se contradice
no se puede auditar.

**Independent Test**: con un corte cerrado donde el cajón quedó corto y la tarjeta cuadró, comparar
lista, detalle y diálogo de cierre: los tres dan la misma cifra y el mismo rótulo.

**Acceptance Scenarios**:

1. **Given** un corte cerrado con el cajón $120 corto y los demás métodos cuadrados, **When** se ve
   en la lista del Histórico, en su detalle y en el diálogo «Caja cerrada», **Then** los tres dicen
   «Cajón (arqueo): Faltante −$120.00», «Otros métodos (conciliación): Cuadra», «Total: Faltante
   −$120.00».
2. **Given** ese mismo corte, **When** se ve la tabla de conciliación, **Then** el renglón de
   efectivo dice «ver arqueo» y no muestra «$0».
3. **Given** cualquier tabla de Caja con montos, **When** se pinta, **Then** las columnas se llaman
   Esperado, Contado y Diferencia, en ese orden, y $1,200 se escribe «$1,200.00».
4. **Given** un sobrante, **When** se pinta en el cierre, **Then** dice «Sobrante» con «+», un ícono
   y su color, y no aparece dentro de un recuadro que lo marque como error.
5. **Given** el arqueo ciego (quien opera no tiene configurar caja), **When** ve un corte, **Then**
   no ve el esperado ni la diferencia en ninguna de las tres pantallas, igual que hoy.

---

### User Story 2 - El detalle del corte cuenta toda la historia del turno (Priority: P1) — Fase 1

El detalle de un corte muestra quién abrió y quién cerró, el fondo que se dejó, el retiro y quién se
lo llevó, y el motivo y la nota de apertura cuando el conteo de apertura no coincidió con el cierre
anterior. El corte anterior, el que estuvo mal, enlaza al turno que lo explicó: «Diferencia
explicada al abrir el turno #N: el cierre se contó mal — <nota>». El MCP devuelve lo mismo.

**Why this priority**: la nota del cajero existe en la base y nadie la puede leer. Es la mitad del
caso que disparó el issue.

**Independent Test**: abrir un turno con un conteo distinto al cierre anterior y un motivo con
nota; ver el detalle de los dos cortes y la lectura del MCP.

**Acceptance Scenarios**:

1. **Given** un turno abierto con motivo «El cierre se contó mal» y nota, **When** se ve su detalle,
   **Then** aparecen el motivo y la nota.
2. **Given** el corte anterior a ese turno, **When** se ve su detalle, **Then** dice que la
   diferencia se explicó al abrir el turno siguiente, con el motivo, la nota y un enlace a ese turno.
3. **Given** un corte cerrado con fondo y retiro, **When** se ve su detalle, **Then** dice cuánto
   fondo se dejó y cuánto se retiró, y quién abrió y quién cerró.
4. **Given** un agente que lee el histórico de cortes por el MCP, **When** pide un corte con motivo
   de apertura, **Then** la respuesta trae el motivo y la nota.

---

### User Story 3 - Corregir un corte cerrado sin reescribirlo (Priority: P2) — Fase 3

Quien tiene ajustar el arqueo abre un corte cerrado y registra una corrección: el conteo del cajón
era otro, el fondo que se dejó era otro, o solo una nota que explica la diferencia. El corte
original no cambia. El reporte muestra «contado $X, corregido a $Y el <fecha> por <quién>: <motivo>»
y la diferencia corregida. Una corrección equivocada se deshace con otra que la revierte, nunca
borrándola.

**Why this priority**: hoy la única forma de corregir un corte es SQL a mano (#289), sin rastro.

**Independent Test**: corregir el conteo de un corte cerrado; ver que el detalle muestra el original
y el corregido, que la lista usa la diferencia corregida y que la bitácora lo registra.

**Acceptance Scenarios**:

1. **Given** un corte cerrado con el cajón contado en $1,000, **When** se corrige a $1,120 con
   motivo y nota, **Then** el detalle muestra «Contado $1,000.00, corregido a $1,120.00 el <fecha>
   por <quién>: <motivo>», la diferencia corregida, y el conteo firmado sigue diciendo $1,000.00.
2. **Given** esa corrección, **When** se revierte, **Then** el corte vuelve a mostrar $1,000.00 y
   las dos correcciones quedan a la vista, una revirtiendo a la otra.
3. **Given** una corrección ya revertida, **When** se intenta revertir otra vez, **Then** se rechaza.
4. **Given** quien solo opera la caja, **When** ve el corte, **Then** no tiene la acción de corregir,
   y la petición directa al servidor se rechaza.
5. **Given** una corrección de otra empresa, **When** se lee el corte, **Then** no aparece.
6. **Given** una nota sin cambio de cifras, **When** se registra, **Then** aparece en el corte y no
   mueve ninguna diferencia.

---

### User Story 4 - Cerrar caja por pasos, con todo lo que falta a la vista (Priority: P2) — Fase 4

Cerrar es un asistente de cuatro pasos con una barra fija que dice dónde se está: (1) pendientes que
bloquean, cada uno con su acción (cuentas abiertas, asistencia, gastos sin caja); (2) conteo del
efectivo por denominación, a ciegas por omisión; (3) otros métodos y terminales; (4) diferencia,
motivo si pasa la tolerancia, fondo que se deja y confirmación. El botón de cerrar nunca está gris
sin decir por qué: la lista de lo que falta y el botón salen de la misma regla. El aviso de
asistencia avisa pero no esconde el conteo. Lo capturado se guarda como borrador y sobrevive a una
recarga. Durante el turno, «Operar» (movimientos, gastos, traspasos, propinas) y «Cerrar» son vistas
distintas, y el **Corte X** (consulta del turno abierto, sin efectos) se separa del **Corte Z**
(cierre).

**Why this priority**: la pantalla de hoy es una columna de ~20 secciones y el botón se deshabilita
por 11 condiciones nombrando solo 4 (la misma clase de defecto que #251).

**Independent Test**: con una cuenta abierta y sin capturar el cajón, entrar a cerrar: el paso 1
nombra la cuenta con su acción, y el botón final dice qué falta.

**Acceptance Scenarios**:

1. **Given** cualquiera de las condiciones que hoy deshabilitan el cierre, **When** se llega al
   último paso, **Then** la lista de pendientes la nombra con su acción, y no existe una condición
   que deshabilite el botón sin aparecer en la lista.
2. **Given** quien abrió la caja sin marcar salida, **When** se entra a cerrar, **Then** el aviso de
   asistencia aparece y el conteo sigue disponible.
3. **Given** un conteo a medias, **When** se recarga la página o la tableta se suspende, **Then** al
   volver el conteo sigue ahí.
4. **Given** quien cierra ve el esperado y la diferencia pasa la tolerancia, **When** llega al último
   paso, **Then** se le ofrece dejar la explicación; si la deja, el corte no queda «por explicar».
   **Given** arqueo ciego, **When** se cierra con cualquier diferencia, **Then** no se pide motivo ni
   se insinúa que haya diferencia, y el corte queda «por explicar» si pasó la tolerancia.
5. **Given** un turno abierto, **When** se pide el Corte X, **Then** se ven las cifras del turno y
   nada cambia: no cierra, no firma ni guarda conteos.
6. **Given** la apertura con un conteo distinto al cierre anterior, **When** se abre, **Then** la
   secuencia conteo → motivo → confirmación muestra en qué paso va.

---

### User Story 5 - Caja usa la pantalla que tiene, de la tableta al 2K (Priority: P2) — Fase 4

En tableta (menos de 840 px) Caja es una columna y el detalle va en hoja, como hoy. En laptop
(1024 px o más) el turno va en dos columnas: Operar a la izquierda, cierre o resumen a la derecha, y
el Histórico en lista y detalle lado a lado. En PC (1440 px o más) tres columnas: cajas, turno y
detalle o movimientos, con tablas sin scroll interno de cinco renglones. En 2K (2560 px) las cajas
abiertas van lado a lado, cada una con su turno, y el Histórico muestra lista, detalle y movimientos
del corte a la vez. Lo que se limita es el largo de las líneas de texto, no el ancho del contenedor.

**Why this priority**: el 2K se usa a diario y hoy deja vacío ~65 % de la pantalla. En laptop y PC
el Histórico recorta nombres, pierde columnas y saca los botones «Ver» por la orilla.

**Independent Test**: capturas a 768, 1024×600, 1366, 1920 y 2560 px del turno abierto y del
Histórico con un corte abierto.

**Acceptance Scenarios**:

1. **Given** 1024×600, **When** se ve el turno abierto, **Then** cabe sin scroll horizontal y todo
   control mide al menos 48 px de alto.
2. **Given** 1366 px, **When** se abre un corte del Histórico, **Then** lista y detalle van lado a
   lado y la lista conserva nombre de la caja, Abrió, Cerró y «Ver» completos.
3. **Given** 1920 px, **When** se ve Caja, **Then** el contenido ocupa al menos 85 % del ancho útil.
4. **Given** 2560 px con dos cajas abiertas, **When** se ve Caja, **Then** las dos van lado a lado
   con su turno, y ningún párrafo pasa de un largo de línea legible.

---

### User Story 6 - Cambiar entre claro y oscuro de un toque (Priority: P3) — Fase 1

Un botón sol/luna en la franja fija del menú, abajo a la izquierda, justo arriba del usuario y la
versión, alterna entre claro y oscuro. «Como el sistema» se recupera desde Interfaz. La preferencia
se guarda por dispositivo. En oscuro, el aviso «Falta entregar» se lee, y sobrante y faltante
mantienen su contraste en los dos temas y las 8 paletas.

**Why this priority**: el tema oscuro ya existe; lo que falta es llegar a él sin buscar la página.

**Independent Test**: tocar el botón en una tableta y comprobar que otra tableta con la misma cuenta
no cambia.

**Acceptance Scenarios**:

1. **Given** el tema en «Como el sistema» con el sistema en claro, **When** se toca el botón,
   **Then** pasa a oscuro; un segundo toque, a claro.
2. **Given** una pantalla de 768 px de alto con el menú desplazado, **When** se busca el botón,
   **Then** está a la vista sin desplazar.
3. **Given** el tema oscuro, **When** se ve el aviso «Falta entregar», **Then** su texto contrasta
   con el fondo al menos 4.5:1.
4. **Given** cualquier tema y paleta, **When** se ve un monto de sobrante o faltante, **Then** su
   contraste es de al menos 7:1.

---

### User Story 7 - Cada caja, corte y ajuste tiene su dirección (Priority: P3) — Fase 2

Cada caja, cada corte del Histórico y Administrar tienen su propia dirección: se puede compartir
«corte #N» y una recarga regresa a donde se estaba. Los ajustes de caja que hoy viven en
Administración (conceptos, terminales, fondo fijo) se juntan con Caja → Administrar. Por dentro,
pantalla y servidor se parten en piezas por responsabilidad sin cambiar lo que hace ninguna.

**Why this priority**: es lo que hace posible las fases 3 y 4 sin tocar un archivo de 2,038 líneas;
para quien opera, la ganancia visible son las direcciones y una sola página de ajustes.

**Independent Test**: la red de pruebas de caja (servidor y e2e) en verde antes y después, y abrir
la dirección de un corte en otra pestaña.

**Acceptance Scenarios**:

1. **Given** la dirección de un corte, **When** se abre en otra pestaña, **Then** muestra ese corte.
2. **Given** Caja en una pestaña distinta de «Cajas», **When** se recarga, **Then** sigue en esa
   pestaña.
3. **Given** quien configura caja, **When** entra a Caja → Administrar, **Then** encuentra cajas,
   conceptos, terminales y fondo fijo; la dirección vieja de los ajustes lleva ahí.
4. **Given** la red de pruebas de caja, **When** corre antes y después de partir, **Then** pasa igual.

### Edge Cases

- **Corte sin arqueo de cajón** (caja que no «lleva arqueo»): la parte «Cajón (arqueo)» no aparece y
  el total es solo la conciliación; nunca «$0» que parezca un arqueo cuadrado.
- **Corte con arqueo automático por terminal**: no se firmó nada por terminal; la conciliación dice
  lo que hay sin inventar diferencias.
- **Corte anterior a la spec 015** (efectivo con declarado distinto del esperado en la conciliación):
  la cifra única no suma dos veces el efectivo.
- **El turno siguiente explica un corte de otra caja**: el enlace va al corte anterior de la misma
  caja, nunca al de otra.
- **Varias correcciones sobre el mismo corte**: la vigente es la última no revertida de cada tipo; las
  anteriores se ven en orden.
- **Corregir un corte mientras la caja tiene un turno abierto**: la corrección no cambia lo que el
  turno abierto espera en el cajón (eso lo fijó su conteo de apertura).
- **Revertir dos veces la misma corrección** desde dos tabletas a la vez: solo una gana; la otra
  recibe un rechazo claro.
- **Borrador del cierre de un turno que ya cerró otra tableta**: al volver, el borrador se descarta y
  se dice que la caja ya cerró.
- **Tolerancia borrada a media captura** (campo vacío): vacío no es $0 en silencio; se rechaza o se
  conserva la anterior.
- **Dirección de un corte de otra empresa o inexistente**: «no encontrado», sin revelar existencia.
- **Rotar la tableta o cambiar el ancho a medio cierre**: el paso y lo capturado se conservan.
- **Versión anterior de la API corriendo durante la subida de la fase 3**: sigue cerrando y leyendo
  cortes sin conocer la tabla nueva.

## Requirements *(mandatory)*

### Functional Requirements

**Fase 1 — Lectura y claridad**

- **FR-001**: La diferencia de un corte MUST calcularse con una sola regla, usada por la lista del
  Histórico, el detalle y el diálogo de cierre, que la parte en «Cajón (arqueo)», «Otros métodos
  (conciliación)» y «Total», donde Total = Cajón + Otros métodos.
- **FR-002**: En la conciliación, el método que toca el cajón MUST mostrarse como «ver arqueo» en
  lugar de una diferencia, cuando la caja lleva arqueo.
- **FR-003**: Toda tabla de Caja MUST usar Esperado · Contado · Diferencia, en ese orden.
- **FR-004**: Toda diferencia MUST escribirse con palabra (Faltante, Sobrante, Cuadra), signo (− con
  el signo menos tipográfico, +), ícono y color, con colores distinguibles con daltonismo y contraste
  de al menos 7:1 en el monto.
- **FR-005**: Todo monto de Caja MUST ir con dos decimales, cifras de ancho fijo y alineado a la
  derecha.
- **FR-006**: El detalle de un corte MUST mostrar quién abrió y quién cerró, el fondo que se dejó, el
  fondo fijo vigente al cierre, el retiro, y el motivo y la nota de apertura.
- **FR-007**: El detalle de un corte cuya diferencia se explicó al abrir el turno siguiente de la
  misma caja MUST mostrar esa explicación con un enlace al turno.
- **FR-008**: El arqueo ciego MUST conservarse: el esperado y la diferencia solo los ve quien tiene
  configurar caja; el motivo y la nota de apertura los ve todo el que ve el corte (P2).
- **FR-009**: La lectura del histórico de cortes por MCP MUST traer el motivo y la nota de apertura.
- **FR-010**: Un aviso de Caja MUST NOT mandar a «corregir en Caja» estando en Caja; dice la acción.
- **FR-011**: El estado de una venta MUST mostrarse con su rótulo en español, nunca el valor crudo.
- **FR-012**: El menú MUST tener un botón de tema de un toque en su franja fija, arriba del usuario y
  la versión, que alterna claro y oscuro y guarda la preferencia por dispositivo.
- **FR-013**: Todo aviso de Caja MUST leerse en los dos temas (contraste de texto de al menos 4.5:1).
- **FR-014**: MUST existir una prueba que fije que el efectivo de la conciliación cuadra por
  construcción y que la diferencia del cajón vive solo en el arqueo, de modo que no se cuente dos
  veces ni se pierda.

**Fase 2 — Partir sin cambiar comportamiento**

- **FR-015**: La pantalla de Caja MUST partirse en submódulos (turno, arqueo, conciliación,
  movimientos, ajustes, reporte) con un contenedor delgado, reusando las reglas que ya existen
  (cierre, fondo que se deja, propinas).
- **FR-016**: El servicio de caja del servidor MUST partirse por responsabilidad (turno, conteo,
  movimientos, reporte) y sus consultas en archivos por tema, sin cambiar ninguna respuesta.
- **FR-017**: Cada caja, cada corte y Administrar MUST tener dirección propia, y una recarga MUST
  volver a la misma vista.
- **FR-018**: Los ajustes de caja (cajas, conceptos, terminales, fondo fijo) MUST vivir juntos en
  Caja → Administrar; la dirección vieja MUST llevar ahí.
- **FR-019**: Las devoluciones y los pagos de gasto MUST seguir siendo el hecho que origina su salida
  de caja; ninguna pieza nueva los vuelve a capturar como movimiento.
- **FR-020**: La red de pruebas de caja (servidor e integración de caja, dinero del turno y «lo que
  salió de la caja», y las e2e de caja) MUST pasar igual antes y después.

**Fase 3 — Correcciones con rastro**

- **FR-021**: Quien tiene ajustar el arqueo MUST poder registrar sobre un corte cerrado una corrección
  de conteo del cajón, una corrección del fondo que se dejó, o una nota, cada una con motivo y nota.
- **FR-022**: Una corrección MUST guardar el valor anterior y el nuevo, quién y cuándo, y MUST NOT
  modificar el conteo, los totales ni el fondo firmados.
- **FR-023**: Una corrección MUST poder revertirse con otra que la nombra; cada corrección se revierte
  a lo más una vez, y ninguna se edita ni se borra.
- **FR-024**: La lista, el detalle y la suma del periodo MUST usar la diferencia corregida, mostrando
  junto a ella el valor original y la corrección.
- **FR-025**: Cada corrección y cada reverso MUST quedar en la bitácora de caja.
- **FR-026**: Las correcciones MUST aislarse por empresa como el resto de los datos de caja.
- **FR-027**: La versión anterior de la API MUST seguir funcionando durante la subida.
- **FR-028**: El Histórico MUST mostrar la suma de diferencias del periodo filtrado, ya corregidas,
  en un renglón que no es venta ni gasto (P4).

**Fase 4 — Cierre por pasos, X/Z y pantallas grandes**

- **FR-029**: El cierre MUST ser un asistente de cuatro pasos (pendientes, cajón, otros métodos,
  diferencia y confirmación) con barra de pasos fija.
- **FR-030**: La lista de pendientes y la habilitación del botón de cerrar MUST salir de la misma
  regla: toda condición que impide cerrar aparece en la lista con su acción.
- **FR-031**: El aviso de asistencia MUST NOT esconder el conteo.
- **FR-032**: Lo capturado en el cierre MUST guardarse como borrador en el dispositivo y sobrevivir a
  una recarga; si la caja ya cerró, se descarta avisando.
- **FR-033**: Cada empresa MUST poder fijar una tolerancia de diferencia en pesos (configurar caja;
  $0 por omisión). Un corte cuya diferencia la pasa y no tiene explicación (nota de corrección o
  motivo de la apertura siguiente) MUST verse «por explicar» en el Histórico. Quien cierra viendo el
  esperado MUST poder dejar la explicación en el último paso; el arqueo ciego MUST NOT revelar a
  quien cierra si pasó la tolerancia (P1).
- **FR-034**: Durante el turno, Operar y Cerrar MUST ser vistas distintas, y el Corte X MUST mostrar
  las cifras del turno sin efecto alguno.
- **FR-035**: La apertura con conteo distinto MUST mostrar en qué paso va (conteo, motivo,
  confirmación).
- **FR-036**: Caja MUST NOT tener tope de ancho de contenedor y MUST disponerse por ancho: una
  columna bajo 840 px, dos desde 1024 px, tres desde 1440 px, y cajas lado a lado en 2560 px, con el
  largo de línea de texto limitado.
- **FR-037**: Todo control de Caja MUST medir al menos 48 px de alto y el conteo MUST usar un teclado
  numérico grande.
- **FR-038**: La disposición MUST verificarse en 768, 1024×600, 1366, 1920 y 2560 px.

### Key Entities

- **Corte (turno de caja)**: ya existe. Quién abrió y cerró, conteo de apertura y su motivo,
  totales por método, arqueo del cajón, fondo que se dejó y retiro. Lo firmado no cambia.
- **Diferencia del corte**: no se guarda; se calcula con una sola regla a partir del arqueo, la
  conciliación y las correcciones vigentes.
- **Corrección de corte** (nueva, fase 3): pertenece a un corte cerrado. Tipo (conteo del cajón,
  fondo, nota), valor anterior y nuevo, motivo y nota, quién y cuándo, y la corrección que revierte.
  Solo se agregan; nunca se editan ni se borran.
- **Tolerancia de diferencia** (nueva, fase 4): un monto por empresa.
- **Preferencia de tema**: por dispositivo, ya existe.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: En el caso del issue (cajón corto, tarjeta cuadrada), las tres pantallas que muestran
  la diferencia dan la misma cifra: 3 de 3, contra 1 de 3 hoy.
- **SC-002**: 0 cortes cuya explicación de apertura no se pueda leer en pantalla ni por MCP.
- **SC-003**: 0 correcciones de corte hechas por SQL después de la fase 3: toda corrección deja su
  rastro en la bitácora.
- **SC-004**: 0 condiciones que deshabiliten el cierre sin aparecer en la lista de pendientes, contra
  7 de 11 sin nombrar hoy.
- **SC-005**: Una persona nueva cierra una caja con un pendiente y una diferencia sin pedir ayuda, a
  1024×600, en la prueba con usuario nuevo.
- **SC-006**: En 1920 px el contenido de Caja ocupa al menos 85 % del ancho útil (hoy ~47 %); en 2560
  px, al menos 85 % (hoy ~35 %).
- **SC-007**: Cambiar de tema cuesta 1 toque, contra 3 hoy (menú, Interfaz, tema).
- **SC-008**: La red de pruebas de caja pasa igual antes y después de la fase 2.

## Assumptions

- La regla única de diferencia se apoya en lo que ya guarda la spec 015: el declarado del método que
  toca el cajón es igual al esperado y la diferencia del cajón vive en el arqueo. Los cortes viejos
  donde eso no se cumplía se tratan sin duplicar.
- Corregir un corte usa el permiso existente de ajustar el arqueo, que ya cubre «lo que mueve el
  esperado de un corte»; no se crea un permiso nuevo.
- El borrador del cierre vive en el dispositivo, como el ticket del POS; no se sincroniza entre
  tabletas.
- La tolerancia es una sola por empresa, no por caja ni por sucursal (se puede agregar después al
  mismo costo).
- La preferencia de tema ya se guarda por dispositivo; el botón la cambia, no crea otra.

## Fuera de alcance

- Conciliar contra el lote del banco por terminal (otro issue).
- Editar arqueos firmados: revierte la spec 015.
- Reabrir un corte (P3, recomendado no).
- Una cuenta contable de sobrantes y faltantes (P4, recomendado no).
- Devoluciones de gasto (#175).
