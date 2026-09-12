# Feature Specification: Un mapa de calor de uso, para dejar de suponer qué se usa

**Feature Branch**: `017-mapa-de-calor-de-uso`

**Created**: 2026-09-11

**Status**: Draft

**Input**: Ver *Origen* al final.

## Contexto

Hoy no existe **una sola cifra** sobre qué se usa del sistema y qué no. Cada decisión de qué mejorar
se toma a ciegas: se supone qué pantallas importan, se supone qué acciones cuestan trabajo y se
supone qué quedó sin usar por nadie.

Pasó esta misma semana. La pantalla de modificadores llevaba un subtítulo de dos renglones que
—palabras del dueño— «nadie ve realmente». Se descubrió **mirándola**, no midiéndola, y solo porque
alguien pasó por ahí. En una tableta de 1024×600 cada renglón que sobra es contenido que no cabe, y
el presupuesto de esa pantalla ya está medido y peleado renglón por renglón
([docs/presupuesto-de-pantalla-1024x600.md](../../docs/presupuesto-de-pantalla-1024x600.md)).
Suponer sale caro.

Lo que esta feature entrega no es un tablero bonito: es **evidencia para decidir**. Qué pantalla
abre la gente cien veces al día y cuál no abre nunca, y qué acciones dentro de ellas se disparan de
verdad.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Ver qué pantallas y qué acciones se usan (Priority: P1)

El **operador de plataforma** entra a la consola (spec 016) y ve, en un mapa de calor, las pantallas
del sistema ordenadas por cuánto se abren, y dentro de cada una las acciones con nombre que se
dispararon (cobrar, cerrar caja, contar efectivo, editar producto, traspaso). Elige el periodo y
compara, y puede verlo unificado o por empresa.

**NO es una pantalla del dueño del negocio.** Es investigación del producto, no un reporte del
local: vive detrás del login de plataforma y ningún usuario de una empresa la ve.

**Why this priority**: Es la feature. Sin esto no hay nada; con esto ya se puede decidir qué
recortar y qué arreglar, que es lo único que se pidió.

**Independent Test**: Con el sistema en uso durante un día, la pantalla muestra al menos una
pantalla con conteo mayor que cero, y ese conteo sube al volver a abrirla.

**Acceptance Scenarios**:

1. **Given** un día de operación normal, **When** el dueño abre el mapa de uso, **Then** ve las
   pantallas ordenadas de más a menos usada, con su conteo y su intensidad de color.
2. **Given** una pantalla que nadie abrió en el periodo, **When** se mira el mapa, **Then** aparece
   con cero y se distingue a simple vista de las que sí se usaron.
3. **Given** una pantalla seleccionada, **When** el dueño la despliega, **Then** ve las acciones
   con nombre que ocurrieron dentro y cuántas veces.
4. **Given** un periodo sin datos —el sistema recién instalado—, **When** se abre el mapa,
   **Then** dice que todavía no hay uso registrado, sin pintar un mapa vacío que parezca roto.

---

### User Story 2 - Por rol, nunca por persona (Priority: P1)

El registro distingue si quien usó el sistema era cajero, gerente o administrador, y **no guarda
quién fue**. El dueño puede ver que un rol usa el sistema distinto sin que eso señale a nadie.

**Why this priority**: Va en P1 junto con US1 y no después, porque **no es una capa que se agregue
encima**: si el primer evento se escribe con la identidad de la persona, ya se registró, y quitarlo
después no borra lo que se guardó. La decisión se toma antes de la primera fila o no se toma.

**Independent Test**: Con varios usuarios de roles distintos usando el sistema, ninguna consulta
sobre los datos de uso —ni desde la aplicación ni desde la base— permite reconstruir qué hizo un
empleado en particular.

**Acceptance Scenarios**:

1. **Given** un cajero y un gerente usando el sistema el mismo día, **When** se mira el mapa,
   **Then** se puede separar el uso por rol.
2. **Given** los datos de uso, **When** alguien los consulta por cualquier vía, **Then** no existe
   forma de saber qué hizo un empleado concreto.
3. **Given** un rol con un solo empleado en la empresa, **When** se mira su uso, **Then** el sistema
   no ofrece ese corte si con él la persona queda identificada por eliminación.

---

### User Story 3 - Que el sistema no se entere de que está midiendo (Priority: P1)

Quien opera no nota que hay medición: ni un toque más lento, ni un botón que espera, ni un error
cuando se cae el wifi. Si la medición no se puede registrar, **se pierde en silencio**.

**Why this priority**: P1 porque es la condición para que la feature sea aceptable. El carrito del
POS vive en el navegador y hoy una caída de red **no detiene la captura**; una feature de analítica
que vuelva la operación dependiente de la red sería un retroceso para quien cobra, y la constitución
lo prohíbe en sus *Restricciones del producto*.

**Independent Test**: Con la red del local caída, el operador arma una cuenta y cobra sin ver un
solo error ni una demora atribuible a la medición.

**Acceptance Scenarios**:

1. **Given** el servidor sin responder, **When** el operador usa el POS, **Then** todo funciona
   igual y no aparece ningún aviso sobre el registro de uso.
2. **Given** una medición que no se pudo entregar, **When** pasa el tiempo, **Then** se descarta sin
   avisar y sin reintentar indefinidamente.
3. **Given** un toque del operador, **When** se mide, **Then** la medición nunca ocurre antes de la
   acción: primero se cobra, después se registra.

---

### User Story 4 - Que quepa en el disco del negocio (Priority: P2)

El dueño no tiene que pensar en esto nunca: los datos de uso se agregan y se recortan solos, y no
crecen hasta llenar el disco que comparte con la base del negocio.

**Why this priority**: P2 porque no se ve, pero sin ello la feature se vuelve un problema en unos
meses. La VM es una **e2-micro con 1 GB de RAM y 20 GB de disco**, medida el 2026-09-11 con 602 MB
de pico bajo carga y 367 MB libres.

**Independent Test**: Simulando el uso de un año, el espacio ocupado por los datos de uso se
mantiene por debajo de un techo declarado y verificable.

**Acceptance Scenarios**:

1. **Given** meses de uso acumulado, **When** se mide el espacio, **Then** está por debajo del techo
   declarado.
2. **Given** datos más viejos que el periodo de conservación, **When** corre el recorte, **Then**
   desaparecen sin intervención de nadie.

---

### Edge Cases

- **El sistema recién instalado, sin un solo dato.** La pantalla lo dice; no pinta un mapa en cero
  que se lea como una falla.
- **Un rol con un solo empleado.** Cortar por rol lo identifica por eliminación. Hay que decidir qué
  hace el sistema ahí, y está resuelto en FR-009.
- **Una pantalla que se abre y se cierra al instante** (un toque errado en el menú). Cuenta igual y
  eso está bien: también dice algo — que el menú confunde.
- **El operador deja la tableta abierta en una pantalla toda la noche.** Un conteo de aperturas no
  se distorsiona; un conteo de *tiempo* sí. Por eso se mide **cuántas veces**, no cuánto rato.
- **Dos tabletas con la misma cuenta.** Hoy dos estaciones comparten usuario
  (constitución, puerta *«Saber de quién es un pedido»*). Como no se guarda la persona, no estorba;
  y el día que se distingan estaciones, el rol sigue siendo el corte correcto.
- **Recargar la pantalla** no puede contar como una apertura nueva si el operador solo apretó F5
  por costumbre. Se decide qué cuenta como apertura y se escribe.
- **El reloj de la tableta mal puesto.** La fecha del dato la pone el servidor, como ya hace la
  venta (spec 008).
- **Datos de uso de otra empresa.** No se cruzan: RLS, como todo lo demás.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: El sistema MUST registrar cuántas veces se abre cada pantalla y cuántas veces se
  dispara cada acción con nombre.
- **FR-002**: El registro MUST guardar el **rol** de quien lo hizo (cajero, gerente, administrador)
  y MUST NOT guardar la identidad de la persona ni nada de lo que permita deducirla.
- **FR-003**: El registro MUST NOT contener datos personales, nombres de cliente, contenido de
  pedidos ni importes.
- **FR-004**: El registro MUST NOT bloquear, demorar ni hacer fallar ninguna acción del operador. Si
  no se puede entregar, **se descarta**.
- **FR-005**: El sistema MUST seguir funcionando sin red exactamente igual que hoy. Esta feature
  MUST NOT introducir ninguna dependencia nueva de la red en un camino que hoy no la tenga.
- **FR-006**: El operador de plataforma MUST poder ver un mapa de calor con las pantallas ordenadas por uso y las
  acciones de cada una.
- **FR-007**: El mapa de calor MUST estar construido sin librerías de gráficas de terceros.

  No es preferencia técnica ni ahorro de licencias: es que cada dependencia trae su calendario de
  versiones y sus conflictos, y en este repo además un CVE bloquea el merge. Ya es principio de la
  constitución —*«Sin dependencia nueva para lo que hace la stdlib»*—, y aquí aplica solo: una
  escala de color y unas barras son CSS, y escribirlas cuesta menos que integrar una librería.

- **FR-008**: El operador de plataforma MUST poder elegir el periodo que mira, y ver el mapa
  unificado o acotado a una empresa.
- **FR-009**: El sistema MUST NOT ofrecer un corte por rol cuando ese corte identifique a una
  persona por eliminación.

  Es la mitad que hace real a FR-002. Decir «el rol gerente hizo estas 40 acciones» en una empresa
  con **un** gerente es decir su nombre. Sin esto, la promesa de no guardar la persona se rompe en
  la pantalla.

- **FR-010**: Los datos de uso MUST agregarse de forma que su volumen no crezca sin límite, y el
  sistema MUST recortar lo más viejo sin intervención humana.
- **FR-011**: El spec MUST declarar el techo de espacio y el periodo de conservación, y ambos MUST
  ser verificables.
- **FR-012**: Los datos de uso MUST estar aislados por empresa.
- **FR-013**: El modelo del evento MUST admitir que más adelante se le agreguen **coordenadas del
  toque** sin rehacer el modelo ni migrar los datos ya escritos.

  Es el principio VIII aplicado: el mapa por coordenada no se construye hoy, pero la decisión que lo
  impediría —un modelo que solo sepa contar pantallas— no se toma tampoco.

- **FR-014**: La pantalla del mapa MUST leerse en una tableta de 1024×600.
- **FR-015**: Con cero datos, la pantalla MUST decirlo y MUST NOT pintar un mapa vacío.

### Key Entities

- **Evento de uso**: qué ocurrió (una pantalla que se abrió o una acción con nombre), de qué rol,
  de qué empresa y cuándo. Nace con espacio para llevar coordenadas después (FR-013).
- **Agregado de uso**: el conteo por empresa, rol, pantalla, acción y día, que es lo que la pantalla
  lee y lo que hace que el volumen no crezca sin límite.
- **Catálogo de acciones con nombre**: la lista de acciones que vale la pena contar. Que sea una
  lista y no «cualquier clic» es lo que mantiene el volumen acotado y los datos legibles.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Después de un día de operación, el operador de plataforma puede nombrar **la pantalla más usada y la
  menos usada** del sistema mirando una sola pantalla, sin pedirle nada a nadie.
- **SC-002**: **Ninguna** consulta sobre los datos de uso, por ninguna vía, permite reconstruir qué
  hizo un empleado identificado.
- **SC-003**: Con el servidor caído, cobrar un pedido toma **los mismos pasos y el mismo tiempo**
  que con el servidor arriba.
- **SC-004**: Simulado un año de uso, el espacio ocupado se mantiene **por debajo del techo
  declarado**.
- **SC-005**: La pantalla del mapa se lee completa a 1024×600 **sin desplazamiento horizontal**.
- **SC-006**: Agregar coordenadas del toque más adelante **no requiere migrar** los datos ya
  escritos.
- **SC-007**: El mapa se pinta **sin agregar una sola dependencia** al proyecto.

## Assumptions

- **Lo que se cuenta son veces, no tiempo.** Un conteo de aperturas no se distorsiona porque alguien
  deje la tableta abierta; uno de permanencia sí.
- **Las acciones que se cuentan son una lista con nombre**, no cualquier toque. Eso acota el volumen
  y hace los datos legibles; contar «todos los clics» es justo lo que US3 del mapa por coordenada
  deja para después.
- **Quien mira esto es el operador de plataforma**, desde la consola de la spec 016. No es
  información del negocio sino del producto, y por eso ningún usuario de una empresa la ve.
- **La fecha la pone el servidor**, como ya hace la venta desde la spec 008 — el reloj de la tableta
  no es confiable.
- **Periodo de conservación por defecto**: se propone conservar el detalle por días recientes y el
  agregado por más tiempo; el plan fija los números y FR-011 exige que queden escritos.

## Dependencias

Depende de la **spec 016 (la consola de plataforma)**: el mapa vive ahí dentro, detrás de su login
y de su subdominio. La recolección del dato es independiente y puede construirse antes; lo que no
se puede es pintar el mapa sin la consola.

## Out of Scope

- **El mapa de calor por coordenada del toque.** Es US3 del origen y queda para después, con su
  puerta abierta en FR-013. Hoy no se construye: son miles de filas por turno sobre una e2-micro.
- **Medir a una persona.** No es un recorte de alcance: es una decisión, y está en FR-002.
- **Medir tiempo de permanencia en pantalla.** Ver *Assumptions*.
- **Enviar estos datos a cualquier servicio externo de analítica.** Ninguno: los datos son del
  negocio y viven en su base.
- **Un tablero configurable.** Una pantalla que contesta la pregunta, no un constructor de reportes.
- **La consola en sí** —identidad de plataforma, login separado, subdominio—: es la spec 016.

## Origen

Pedido por el dueño el 2026-09-11, a raíz de borrar un subtítulo de `/catalogo/opciones` que «nadie
ve realmente»: *«necesito que agregues un mapa de calor, hecho en casa, nada de librerías para no
pagar licencias… así podremos ver en dónde se mueve más el usuario e ir recabando información para
mejorar el sistema»*.

Dos decisiones tomadas por él antes de escribir este spec, y que lo condicionan entero:

| Pregunta | Respuesta |
|---|---|
| ¿Qué mide? | **Las dos**: hoy pantallas y acciones; el toque por coordenada queda como puerta abierta |
| ¿Distingue quién? | **Por rol, sin identidad** |

Y una precisión suya, ese mismo día, sobre el «sin librerías»: *«me refiero a que desde un inicio
quiero tecnologías in house… muchas veces me puedo llegar a topar con problemas de versiones»*. La
razón no es el costo de una licencia sino el costo de depender: versiones, conflictos y CVE. Es el
principio VI de la constitución, que ya lo dice.
