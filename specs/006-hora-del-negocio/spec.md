# Feature Specification: La hora del negocio manda

**Feature Branch**: `006-hora-del-negocio`

**Created**: 2026-09-01

**Status**: Draft

**Input**: Ver [Contexto medido](#contexto-medido). Las cuatro decisiones de diseño las tomó el dueño
antes de este spec y aquí se implementan, no se reabren.

## Contexto medido

| Qué | Cuánto |
| --- | --- |
| Lugares del frontend que formatean fecha u hora | **11** |
| De ésos, que usan la zona del negocio | **0** |
| Zona configurada del negocio | `America/Mexico_City`, desde la migración 0038 |
| Diferencia con el reloj del servidor | 6 horas (el servidor corre en UTC) |

El negocio ya tiene su zona horaria guardada y validada, y el backend la usa para decidir de qué día
es una venta o un corte. Ninguna pantalla la usa: los once dicen la hora del **navegador de esa
tableta**. Dos Surface con la hora del sistema distinta muestran horas distintas del mismo pedido, y
el ticket que se le entrega al cliente lleva la hora de la tableta, no la del local.

Del mismo desfase sale un segundo problema. "Entregados hoy" se llena con los pedidos cuyo día
coincide con **el día del servidor**, que en UTC cambia a las 18:00 de México. La lista se vacía a
media hora pico, con los pedidos entregados y el turno todavía abierto.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Todas las pantallas dicen la hora del local (Priority: P1)

Quien opera ve la misma hora en las dos tabletas y en el papel, sin importar cómo esté configurado
el reloj de cada máquina.

**Why this priority**: Es la causa raíz. Mientras cada pantalla diga la hora de su propia tableta, no
hay forma de que dos personas hablen del mismo pedido, y el ticket que sale del negocio miente sobre
cuándo se hizo la venta.

**Independent Test**: Cambiar la hora del sistema de una tableta a otra zona y comprobar que todas
las pantallas y el ticket siguen mostrando la hora del local.

**Acceptance Scenarios**:

1. **Given** una tableta con el reloj en otra zona horaria, **When** se abre cualquier pantalla que
   muestre una hora, **Then** la hora es la del negocio, no la del sistema.
2. **Given** un pedido, **When** se imprime su ticket, **Then** la hora impresa es la del negocio.
3. **Given** la aplicación recién abierta y los ajustes todavía cargando, **When** se pinta la
   primera hora, **Then** no aparece una hora que después se corrija sola.
4. **Given** un negocio sin ajustes guardados, **When** se pinta una hora, **Then** se usa la zona
   por default del producto, nunca la del navegador.

---

### User Story 2 - Los pedidos activos no desaparecen (Priority: P1)

Un pedido que sigue abierto se ve en la pantalla **siempre**, sin importar de qué día sea, hasta que
alguien lo cierre.

**Why this priority**: Es el mecanismo que limpia el rezago. Hoy hay pedidos abiertos desde julio que
nadie ve, y por lo tanto nadie cierra. Mostrarlos obliga a resolverlos; al resolverlos salen solos.

**Independent Test**: Dejar un pedido abierto de una fecha anterior y comprobar que aparece en la
pantalla y que, al entregarlo, pasa a la lista de entregados.

**Acceptance Scenarios**:

1. **Given** un pedido abierto de hace dos meses, **When** se abre el punto de venta, **Then**
   aparece entre los pedidos en curso.
2. **Given** ese pedido, **When** se entrega, **Then** sale de los pedidos en curso y aparece en los
   entregados.
3. **Given** un turno que cruza la medianoche, **When** cambia el día, **Then** los pedidos abiertos
   siguen viéndose.

---

### User Story 3 - "Entregados hoy" se vacía cuando el negocio dice (Priority: P2)

La lista de entregados se limpia a la medianoche del local. El dueño puede cambiar ese momento por
el inicio del turno o por el cierre de caja.

**Why this priority**: Sin esto la lista se vacía a las 18:00 y con los pedidos del día todavía
frescos. Es P2 porque la US2 ya devuelve lo importante —lo que falta por entregar— y esto ordena lo
que ya se entregó.

**Independent Test**: Entregar un pedido, adelantar el reloj más allá del corte configurado, y
comprobar que la lista se vació; con el corte sin cumplirse, que sigue ahí.

**Acceptance Scenarios**:

1. **Given** un pedido entregado hoy y el corte en la medianoche, **When** son las 23:00 del local,
   **Then** sigue en la lista.
2. **Given** ese mismo pedido, **When** pasa la medianoche del local, **Then** ya no está.
3. **Given** el corte configurado por cierre de caja, **When** el turno se cierra, **Then** la lista
   se vacía aunque no haya cambiado el día.
4. **Given** un negocio que nunca tocó el ajuste, **When** se consulta, **Then** el corte es la
   medianoche.

---

### User Story 4 - Cambiar la zona no asusta a nadie (Priority: P3)

Cuando el dueño cambia la zona horaria del negocio, el sistema le dice qué va a pasar antes de
guardarlo.

**Why this priority**: Es un cambio raro pero de efecto visible y global: todas las horas se mueven
de golpe. Sin aviso, se lee como que los datos se corrompieron.

**Independent Test**: Cambiar la zona y comprobar que el aviso aparece antes de guardar y que las
cifras de los cortes anteriores no cambiaron.

**Acceptance Scenarios**:

1. **Given** la pantalla de negocio, **When** se elige otra zona, **Then** se explica que las horas
   mostradas van a cambiar y que las ventas ya registradas no se mueven de día.
2. **Given** un corte cerrado antes del cambio, **When** se consulta después, **Then** sus cifras son
   idénticas.

---

### Edge Cases

Las formas de fallar, enumeradas antes de escribir nada.

#### El valor vacío que significa algo

- **La zona todavía no ha cargado.** Si se pinta con la del navegador y luego se corrige, el
  operador ve la hora saltar y deja de confiar en ella. La primera hora que se muestra ya tiene que
  ser la correcta, aunque eso signifique no mostrar nada por un instante.
- **Un negocio sin fila de ajustes.** Cae a la zona por default del producto, nunca a la del
  navegador: el fallback tiene que ser el mismo en todas las tabletas o el problema vuelve por otra
  puerta.

#### El estado que no sobrevive

- **La tableta se queda abierta cruzando la medianoche.** La lista de entregados tiene que vaciarse
  sin que nadie recargue, y los pedidos en curso tienen que seguir ahí.
- **El horario de verano.** Una zona con cambio de horario mueve la medianoche una hora dos veces al
  año. El corte tiene que caer en la medianoche real de ese día, no a un número fijo de horas del
  anterior.

#### El camino nuevo que se salta el control viejo

- **La zona guardada deja de ser válida.** Una zona que el navegador no reconoce no puede tumbar la
  pantalla ni caer a UTC en silencio: se usa el default y se avisa a quien pueda arreglarlo.
- **El ticket impreso y la comanda.** Son las dos salidas de papel y las dos formatean hora. Si solo
  se arregla la pantalla, el papel sigue mintiendo — y es el que se lleva el cliente.
- **Los pedidos activos sin filtro de fecha.** Al quitar el límite, entra todo el histórico abierto.
  Un pedido de julio que nadie cierre se queda en la pantalla para siempre y con el tiempo la barra
  deja de ser usable.

#### El hermano que no se movió

- **El corte de caja y el resumen de ventas** ya usan la zona del negocio, del lado del servidor. Al
  aplicarla en la pantalla hay que comprobar que no queda aplicada **dos veces** — una hora corrida
  seis horas de más es peor que una hora corrida seis de menos, porque parece plausible.
- **La barra de pedidos en curso** se ató a la fecha del turno abierto en un arreglo anterior. Esta
  feature la reemplaza por "sin filtro de fecha", así que ese arreglo se retira, no se acumula.

## Requirements *(mandatory)*

### Functional Requirements

#### La hora que se muestra

- **FR-001**: Toda fecha y hora que el sistema muestre a una persona MUST estar expresada en la zona
  horaria configurada del negocio.
- **FR-002**: Esto MUST incluir el ticket del cliente y la comanda de cocina.
- **FR-003**: El sistema MUST NOT mostrar una hora derivada del reloj o la zona del dispositivo.
- **FR-004**: Mientras la zona del negocio no se conozca, el sistema MUST NOT mostrar una hora que
  después se corrija sola.
- **FR-005**: Sin ajustes guardados, el sistema MUST usar la zona por default del producto.
- **FR-006**: Si la zona guardada no se puede aplicar, el sistema MUST usar la zona por default,
  seguir funcionando, y dejar constancia para quien pueda corregirla.
- **FR-007**: La conversión de zona MUST aplicarse en un solo lugar del sistema, de modo que una
  pantalla nueva la herede sin tener que acordarse.

#### Qué se ve en pantalla

- **FR-008**: Los pedidos que siguen en curso MUST mostrarse sin importar de qué día sean.
- **FR-009**: Un pedido MUST salir de esa lista solo al entregarse o cancelarse.
- **FR-010**: El sistema MUST distinguir a la vista un pedido en curso de un día anterior, para que
  el rezago se note en vez de confundirse con el trabajo de hoy.
- **FR-011**: Los pedidos entregados MUST mostrarse hasta el momento de corte configurado.
- **FR-012**: El momento de corte MUST poder ser: la medianoche del negocio, el inicio del turno, o
  el cierre de caja.
- **FR-013**: Un negocio que nunca tocó el ajuste MUST tener el corte en la medianoche.
- **FR-014**: Las listas MUST reflejar el corte sin que nadie recargue la pantalla.

#### Lo que no puede cambiar

- **FR-015**: El día al que pertenece una venta MUST NOT cambiar por esta feature.
- **FR-016**: Las cifras de los cortes ya cerrados MUST quedar idénticas.
- **FR-017**: Cambiar la zona del negocio MUST avisar que las horas mostradas van a cambiar y que las
  ventas ya registradas no se mueven de día.

### Key Entities

- **Zona horaria del negocio**: nombre IANA, uno por empresa. Ya existe. Decide cómo se muestra toda
  hora y de qué día es una venta.
- **Momento de corte de la vista**: cuándo se vacía la lista de entregados. Medianoche del negocio,
  inicio del turno, o cierre de caja. No tiene nada que ver con el día de la venta.
- **Pedido en curso**: el que no se ha entregado ni cancelado. Ya no depende de una fecha.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Con dos dispositivos configurados en zonas distintas, la hora que muestran del mismo
  pedido es idéntica.
- **SC-002**: El ticket impreso muestra la misma hora que la pantalla, en cualquier dispositivo.
- **SC-003**: Ninguna hora visible cambia de valor después de mostrarse por primera vez.
- **SC-004**: Con el reloj del sistema en cualquier momento del día, la lista de entregados conserva
  los pedidos del día del negocio hasta el corte configurado.
- **SC-005**: Un pedido abierto de cualquier antigüedad aparece en la pantalla, y se distingue de los
  del día.
- **SC-006**: Las cifras de un corte cerrado antes de la feature son idénticas después.
- **SC-007**: Ninguna pantalla nueva tiene que acordarse de aplicar la zona: se hereda.

## Assumptions

- **El desfase que se ataca es el del navegador, no el del servidor.** El backend ya resuelve el día
  de una venta con la zona del negocio; lo que falta es la presentación y las dos ventanas de
  pantalla.
- La lista corta de zonas de México que ya ofrece la pantalla de negocio se conserva. Un negocio
  fuera de esa lista se configura igual.
- **Los tres modos de corte pueden coincidir en la práctica.** En un negocio que abre a las 16:00 y
  cierra a las 22:00, "medianoche" y "cierre de caja" caen a horas distintas pero el efecto visible
  es el mismo la mayoría de los días. Se construyen los tres porque el producto se vende a negocios
  con horarios distintos, no porque este local los necesite; el plan tiene que decir cuál es el
  costo real de los dos que hoy no se usan.
- **No se toca el histórico.** Ninguna migración de datos, ningún recálculo de `business_date`.
- El aviso al cambiar de zona es informativo, no una confirmación con doble paso: cambiar la zona no
  destruye nada.

## Out of Scope

- **Zonas horarias por sucursal o por usuario.** Una por empresa alcanza; cuando exista la segunda
  sucursal se decide entonces.
- **Recalcular el día de ventas históricas.** Si un negocio tuvo la zona mal configurada durante
  meses, corregirla no reescribe sus cortes — eso es una migración de datos con su propia decisión.
- **Un horario de apertura configurable** que corra el "día del negocio" (tipo "el día empieza a las
  5am"). El corte de esta feature es solo de VISTA; el día de la venta lo sigue decidiendo el turno.
