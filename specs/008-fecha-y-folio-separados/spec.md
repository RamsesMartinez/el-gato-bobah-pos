# Feature Specification: La fecha la da el reloj, el folio lo da el turno

**Feature Branch**: `008-fecha-y-folio-separados`

**Created**: 2026-09-04

**Status**: Draft

**Input**: Ver *Origen* al final.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Ver hoy lo que se vendió hoy (Priority: P1)

Quien administra el negocio abre la pantalla de Ventas y ve las ventas del día, sin importar
cuándo se abrió la caja ni si alguien olvidó cerrarla.

Hoy no ocurre: la venta se archiva con la fecha del turno abierto, y un turno que nadie cierra
sigue estampando su fecha días después. Medido el 2026-09-04 en el ambiente de pruebas: 158
pedidos y $6,664 acumulados entre el 31-ago y el 4-sep, todos archivados como 31 de agosto. La
pantalla de Ventas en "hoy" devolvía cero filas y quien la leyó entendió que no se había vendido
nada.

**Why this priority**: Es el defecto que se reportó y el que hace que una pantalla mienta sobre
dinero. Sin esto, ninguna cifra por fecha —Ventas ni Reportes— es confiable después del primer
turno que se queda abierto.

**Independent Test**: Con un turno abierto de una fecha anterior, registrar una venta y verificar
que aparece en Ventas del día de hoy y no en el día del turno. Se prueba solo, sin nada de las
demás historias.

**Acceptance Scenarios**:

1. **Given** un turno abierto desde hace cuatro días, **When** se registra una venta hoy,
   **Then** la venta queda archivada con la fecha de hoy en la zona horaria del negocio y aparece
   en Ventas del día de hoy.
2. **Given** un turno que abrió a las 23:00 y sigue abierto, **When** se registra una venta a las
   00:30, **Then** la venta queda archivada con la fecha del nuevo día, no con la del anterior.
3. **Given** un negocio con zona `America/Mexico_City` y una venta a las 19:00 locales, **When**
   se consulta Ventas de ese día, **Then** la venta aparece en ese día y no en el siguiente.
4. **Given** un negocio sin configuración guardada, **When** se registra una venta, **Then** la
   fecha se calcula con la zona por defecto del producto y nunca con UTC ni con la zona del
   dispositivo.

---

### User Story 2 - El folio no se parte a medianoche (Priority: P2)

Quien atiende canta el número y el nombre del pedido. Esa numeración corre dentro del turno, así
el turno cruce la medianoche, y no depende de en qué día calendario cayó la venta.

**Why this priority**: Al soltar la fecha del turno (US1), el folio quedaría colgado de una fecha
que ahora cambia a medianoche, y un turno nocturno sacaría dos tickets con el mismo número la
misma noche. Es un defecto que este repositorio ya vivió y que la herencia de fecha vino a tapar.
Separar los dos caminos es lo que permite arreglar la fecha sin reabrirlo.

**Independent Test**: Con un turno abierto que cruza la medianoche, registrar ventas antes y
después de las 00:00 y verificar que la numeración sigue corrida y ningún nombre se repite entre
pedidos vivos.

**Acceptance Scenarios**:

1. **Given** un turno abierto con pedidos numerados hasta el 12, **When** pasa la medianoche y se
   registra otra venta, **Then** el pedido recibe el número 13 y no el 1.
2. **Given** un turno abierto, **When** se registran dos ventas simultáneas, **Then** cada una
   recibe un número distinto y ningún número se repite dentro del turno.
3. **Given** un turno abierto donde ya se cantó un nombre, **When** se registra otra venta,
   **Then** recibe un nombre distinto de los que ya están vivos en ese turno.
4. **Given** el turno se cierra y se abre otro el mismo día, **When** se registra una venta,
   **Then** la numeración reinicia en 1 sin colisionar con ningún pedido vivo, porque cerrar un
   turno exige que no queden pedidos pendientes.

---

### User Story 3 - Las ventas de un corte, dentro de su corte (Priority: P3)

Quien revisa un corte de caja abre su detalle y ve, junto al resumen y a lo declarado por método,
la lista de ventas que ese corte cobró.

**Why this priority**: Al dejar de coincidir "el día de la venta" con "el turno que la cobró",
hace falta un lugar donde ver qué ventas responden por el dinero de un arqueo. Es también la
alternativa deliberada a poner un filtro por corte en la pantalla de Ventas, donde convivir con el
filtro de fechas dejaría llegar a una pantalla vacía sin explicación.

**Independent Test**: Abrir el detalle de un corte cerrado y verificar que lista sus ventas, con
la cuenta y el total, y que ninguna venta de otro corte aparece ahí.

**Acceptance Scenarios**:

1. **Given** un corte cerrado con ventas, **When** se abre su detalle, **Then** se ve la lista de
   sus ventas con folio, hora, estado y total, y un encabezado con cuántas son y cuánto suman.
2. **Given** un corte sin ninguna venta, **When** se abre su detalle, **Then** la sección lo dice
   con una frase y no muestra una tabla vacía.
3. **Given** un corte con ventas canceladas o reembolsadas, **When** se abre su detalle,
   **Then** esas ventas aparecen con su estado y el total declara qué incluye y qué excluye.
4. **Given** un corte con más ventas de las que la pantalla trae, **When** se abre su detalle,
   **Then** el encabezado dice cuántas hay en total y cuántas se están mostrando.

---

### User Story 4 - Saber que la caja abierta ya no es de hoy (Priority: P2)

Quien opera ve un aviso cuando el turno que tiene abierto se abrió en una fecha anterior, con la
acción para cerrarlo.

**Why this priority**: Nada avisaba, y por eso el defecto duró cinco días sin que nadie lo notara.
Arreglar la fecha de la venta quita la consecuencia peor —la pantalla que miente— pero deja en pie
la causa: un turno olvidado sigue metiendo días de dinero en un solo arqueo.

**Independent Test**: Con un turno abierto de una fecha anterior, entrar al sistema y verificar que
el aviso aparece y que registrar una venta sigue funcionando.

**Acceptance Scenarios**:

1. **Given** un turno abierto desde ayer o antes, **When** se entra a la pantalla de venta o a la
   de caja, **Then** se ve un aviso que dice desde cuándo está abierto y ofrece ir a cerrarlo.
2. **Given** un turno abierto hoy, **When** se entra a esas pantallas, **Then** no hay aviso.
3. **Given** un turno abierto desde ayer, **When** se registra una venta, **Then** la venta se
   registra normalmente: el aviso nunca bloquea el cobro.

---

### Edge Cases

- **Cerrar y reabrir la caja el mismo día**: la numeración reinicia en 1. No colisiona con ningún
  pedido vivo porque cerrar un turno ya exige que no queden pedidos abiertos ni listos; ese
  requisito es lo que hace segura la decisión y por eso su prueba forma parte de esta feature. Dos
  tickets del mismo día pueden llevar el número 1: se distinguen por fecha, hora y nombre.
- **Un turno abierto al momento del cambio**: el ambiente de pruebas tiene un turno con 158
  pedidos numerados. Si el contador de folio pasa a contarse por turno sin arrastrar lo ya
  repartido, el siguiente pedido de ese turno recibiría el número 1 y chocaría con uno que ya
  existe. El cambio tiene que continuar la numeración desde el número más alto ya usado en cada
  turno abierto.
- **Ventas históricas archivadas con la fecha del turno**: quedarían con un significado distinto
  al de las nuevas. Medido: recalcularlas cambia **0 de 31 filas del negocio en operación** y 2 de
  61 de la cuenta de pruebas que vive en el mismo servidor.
- **Zona horaria que deja de ser válida**: el sistema cae a la zona por defecto del producto, no a
  UTC, y sigue cobrando. Ya está resuelto y esta feature no lo cambia.
- **El negocio cambia su zona horaria**: las ventas ya archivadas conservan su fecha; solo cambian
  las nuevas. Ninguna cifra de un día cerrado se mueve.
- **Zona con horario de verano**: `America/Tijuana` sigue cambiando de hora. El día se resuelve
  preguntándole a la zona, nunca restando 24 horas, para que el día del cambio no se desfase.
- **Dos ventas al mismo tiempo**: la numeración dentro del turno no puede repetir ni saltar. Hoy
  hay una garantía de que dos cobros simultáneos nunca reciben el mismo número; cambiar de qué
  depende el contador no puede debilitarla.
- **Sin turno abierto**: la venta se sigue rechazando como hoy. Esta feature no cambia esa regla.
- **Un corte con muchas ventas**: la pantalla no puede crecer sin límite en una tableta de
  1024x600. Si se muestra un subconjunto, tiene que decir cuántas hay en total: un recorte
  silencioso se lee como "esto es todo".

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: El sistema MUST archivar cada venta con el día de calendario en que ocurrió, medido
  en la zona horaria del negocio, y no con la fecha del turno de caja.
- **FR-002**: El sistema MUST resolver esa fecha con la zona configurada del negocio, cayendo a la
  zona por defecto del producto cuando no hay configuración o cuando la guardada dejó de ser
  válida; nunca a UTC ni a la zona del dispositivo.
- **FR-003**: El sistema MUST numerar el folio de una venta dentro del turno que la cobra, sin
  consultar la fecha de la venta.
- **FR-004**: El sistema MUST asignar el nombre que se canta dentro del mismo alcance que el
  número —el turno— para que folio y nombre respondan a una sola cosa.
- **FR-005**: El sistema MUST garantizar que dos ventas simultáneas del mismo turno reciben
  números distintos y consecutivos, sin repetir ni saltar.
- **FR-006**: El sistema MUST continuar la numeración de los turnos que ya estén abiertos al
  momento del cambio, a partir del número más alto ya repartido en cada uno.
- **FR-007**: El sistema MUST dejar las ventas históricas con el mismo significado de fecha que
  las nuevas, corrigiendo las que fueron archivadas con la fecha del turno.
- **FR-008**: La corrección histórica MUST NOT alterar ninguna cifra de un arqueo ya cerrado, ni
  el turno al que pertenece una venta, ni su folio.
- **FR-009**: El detalle de un corte MUST mostrar las ventas que ese corte cobró, con folio, hora,
  estado y total de cada una.
- **FR-010**: El detalle de un corte MUST encabezar esa lista con cuántas ventas son y cuánto
  suman, declarando qué incluye y qué excluye ese total.
- **FR-011**: Si el detalle de un corte muestra menos ventas de las que existen, MUST decir
  cuántas hay en total.
- **FR-012**: El sistema MUST avisar a quien opera cuando el turno abierto de la caja que recibe
  ventas se abrió en una fecha anterior a hoy, y ofrecer la acción de cerrarlo.
- **FR-013**: Ese aviso MUST NOT impedir registrar ni cobrar una venta.
- **FR-014**: La pantalla de Ventas MUST NOT ganar un filtro por corte de caja: la relación entre
  una venta y su corte se consulta desde el corte.
- **FR-015**: El sistema MUST seguir exigiendo un turno abierto para registrar una venta.

### Key Entities

- **Venta**: tiene un día de negocio —el día de calendario en que ocurrió, en la zona del local— y
  un folio —número y nombre— que le da el turno. Los dos son independientes: ninguno se deriva del
  otro.
- **Turno de caja (corte)**: la sesión de una caja entre su apertura y su cierre. Es lo que agrupa
  el dinero de un arqueo y, desde ahora, también lo que numera los folios. Un turno puede abarcar
  varios días de calendario, y un día de calendario puede abarcar varios turnos.
- **Contador de folio**: lo que garantiza que dos ventas simultáneas no reciban el mismo número.
  Pasa a contarse por turno.
- **Zona horaria del negocio**: ya existe y ya se configura; es lo que convierte un instante en un
  día de calendario.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Una venta registrada hoy aparece en la pantalla de Ventas del día de hoy, sea cual
  sea la antigüedad del turno abierto. Verificable con un turno de cuatro días de antigüedad.
- **SC-002**: Cero ventas archivadas en un día distinto de aquel en que ocurrieron, medido sobre
  todo el histórico después de la corrección.
- **SC-003**: En un turno que cruza la medianoche, cero folios repetidos y cero saltos en la
  numeración.
- **SC-004**: Todas las cifras de los arqueos ya cerrados quedan idénticas antes y después del
  cambio, comparadas fila por fila sobre una copia de los datos reales.
- **SC-005**: Desde el detalle de un corte se puede saber qué ventas lo componen sin salir de esa
  pantalla y sin cambiar ningún filtro.
- **SC-006**: Un turno abierto de una fecha anterior es visible para quien opera desde la primera
  pantalla que abre, sin tener que ir a buscarlo.
- **SC-007**: Registrar una venta sigue siendo posible en todos los casos en que hoy lo es.

## Assumptions

- **La corrección histórica se aplica.** Se midió antes de decidirlo: cambia 0 de 31 filas del
  negocio en operación y 2 de 61 de la cuenta de pruebas alojada en el mismo servidor. Se aplica
  porque deja la fecha de una venta con un solo significado en todo el histórico; un dato que
  significa una cosa antes de cierto día y otra después obliga a cada quien que lo lee a saber la
  fecha del cambio. Se verifica contra una copia restaurada de los datos reales antes de aplicarse.
- **El folio reinicia si la caja se cierra y se reabre el mismo día.** Es consecuencia directa de
  numerar por turno, y es segura porque cerrar exige que no queden pedidos vivos. Se prefiere a la
  alternativa —una regla que mire la fecha para decidir si reiniciar— porque volvería a acoplar los
  dos caminos que esta feature separa.
- **El aviso de turno viejo compara contra el día de hoy en la zona del negocio**, no contra un
  número de horas transcurridas: un turno que abrió ayer a las 23:00 ya es de ayer aunque lleve una
  hora abierto.
- **El detalle del corte no gana paginación con controles.** El alto de una tableta de 1024x600 ya
  está repartido entre el resumen, los gastos, lo declarado por método y lo cobrado por persona; la
  lista de ventas convive con ellos y no los desplaza.
- Se reutiliza todo lo que ya existe: el cálculo del día de negocio en la zona del local, la
  resolución de la zona con su respaldo, el agrupamiento del arqueo por turno, y la pestaña de
  histórico de cortes con su detalle.

## Origen

Reportado por el dueño el 2026-09-04: "hice ventas hoy pero no se ven en la pantalla de ventas".
La investigación encontró la herencia de fecha sin techo. Decisiones del dueño en la misma
conversación:

1. Los pedidos dejan de heredar la fecha de la caja y toman la del reloj.
2. "Una buena código de programación debería tener ambos caminos separados e independientes":
   folio y fecha no se leen entre sí.
3. Nada de filtrar ventas por corte en la pantalla de Ventas; el histórico de cortes muestra las
   ventas de cada corte.
