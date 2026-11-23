# Feature Specification: Vender — canales compactos, más cuentas a la vista y qué le falta a cada una

**Feature Branch**: `089-vender-cuentas-en-curso`

**Created**: 2026-11-23

**Status**: Draft

**Input**: Issue #315 (pedido del dueño del 2026-11-15 y nota del 2026-11-23). Rediseño de la parte
de arriba de Vender: los canales quitan mucho alto, se ven pocas cuentas en curso, y una cuenta no
dice qué le falta para quitarse de la fila.

## Contexto: lo que hay hoy (medido)

Medido el 2026-11-23 contra el `make start` del worktree, con un respaldo de producción de ese día,
a 1024×600 y a 1280×854:

| Pieza | 1024×600 | 1280×854 |
| --- | --- | --- |
| Renglón de canales (Mostrador, DiDi, Rappi, Uber Eats) | y 12–56 | y 12–56 |
| Renglón de cuentas + buscar + precios + editar | y 72–116 | y 72–116 |
| Categorías | y 130–182 | y 130–182 |
| Los productos empiezan en | y 196 (33% del alto) | y 196 |
| Fichas de cuenta visibles, ticket cerrado | 4 | 4 |
| Fichas de cuenta visibles, ticket abierto | 2 | 2–3 |

- Al elegir una plataforma, el bloque de canales crece dos renglones de texto («Cobrando con precios
  de…» y «Mantén presionado un producto para corregir su precio»): unos 40 px más.
- Las fichas miden 112 px de ancho y dicen el nombre, el estado y una cifra: «En cocina $120»,
  «Pagada · en cocina pagada», «Pago parcial falta $40», «Entregada · debe $40». Lo que no cabe se
  cuenta en «+N», que abre la lista completa.
- **Una cuenta pagada que no se ha marcado entregada sigue en la fila**, y la ficha no dice por qué
  ni ofrece cómo quitarla. Entregar solo se puede desde la pantalla de Pedidos. Se acumulan.
- Las franjas del shell aparecen arriba de cualquier pantalla: la de pedidos por aceptar (spec 093)
  es una fila de 52 px que le quita ese alto a Vender mientras hay pedidos; el aviso de agotado
  (spec 084) flota encima, 60 px al centro, durante 90 segundos, y tapa lo que quede debajo.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Cada cuenta dice qué le falta para quitarse (Priority: P1)

Quien atiende ve la fila de cuentas en curso. Cada cuenta dice con palabras de mostrador qué le falta
para cerrarse: **cobrar** (con cuánto), **entregar**, o **cobrar y entregar**. Una cuenta que todavía
se está capturando dice que falta enviarla. Alguien que nunca usó el sistema entiende, sin que se lo
expliquen, por qué una cuenta pagada sigue ahí.

**Why this priority**: es la queja de fondo. Las cuentas «pagadas» se acumulan porque nadie entiende
que falta entregarlas, y el operador pierde de vista las que sí deben dinero.

**Independent Test**: con cuentas en los cinco estados vivos en la base local, cada ficha muestra el
texto de lo que le falta correspondiente a su estado, y una persona sin contexto, preguntada «¿qué
hay que hacer con esta cuenta para que se quite?», contesta bien en todas.

**Acceptance Scenarios**:

1. **Given** un pedido enviado a cocina sin pagar, **When** se ve su cuenta, **Then** dice que falta
   cobrar (con el monto) y entregar.
2. **Given** un pedido pagado completo que no se ha entregado, **When** se ve su cuenta, **Then**
   dice que solo falta entregar, sin monto.
3. **Given** un pedido con pago parcial sin entregar, **When** se ve su cuenta, **Then** dice que
   falta cobrar lo que resta (con el monto) y entregar.
4. **Given** un pedido entregado que debe, **When** se ve su cuenta, **Then** dice que solo falta
   cobrar, con el monto.
5. **Given** una cuenta en captura, **When** se ve, **Then** dice que no se ha enviado, con su total.
6. **Given** un pedido cerrado con algo «nuevo» capturándose, **When** se ve, **Then** dice que lo
   nuevo no se ha enviado; no dice que falte cobrar ni entregar el pedido ya cerrado.

---

### User Story 2 - Resolver desde la cuenta lo que le falta (Priority: P1)

Desde la cuenta, sin ir a otra pantalla, quien atiende la entrega o la cobra. Entregar una cuenta
pagada la quita de la fila; cobrar una entregada que debía, también.

**Why this priority**: decir qué falta sin dejar resolverlo obliga a cambiar de pantalla en la hora
pico. Entregar es un toque que hoy vive solo en Pedidos.

**Independent Test**: con un pedido pagado sin entregar, se marca entregado desde Vender y la cuenta
desaparece de la fila en la tableta y en otra tableta; con uno entregado que debe, se cobra desde
Vender y desaparece.

**Acceptance Scenarios**:

1. **Given** una cuenta a la que solo le falta entregar, **When** quien atiende toca entregar,
   **Then** el pedido queda entregado y la cuenta sale de la fila.
2. **Given** una cuenta a la que le falta cobrar y entregar, **When** quien atiende toca entregar,
   **Then** el pedido queda entregado y la cuenta sigue en la fila diciendo que solo falta cobrar.
3. **Given** una cuenta a la que le falta cobrar, **When** quien atiende toca cobrar, **Then** se
   abre el cobro de esa cuenta (el mismo de siempre), y al saldarla, si ya estaba entregada, sale de
   la fila.
4. **Given** una cuenta en captura (todavía no es pedido), **When** se ve, **Then** no ofrece ni
   cobrar ni entregar desde la fila: se abre y se envía o se cobra desde el ticket, como hoy.
5. **Given** que el servidor rechaza la entrega (otra tableta la canceló), **When** se toca
   entregar, **Then** se ve un mensaje para quien opera, sin detalles técnicos, y la fila se
   actualiza con el estado real.

---

### User Story 3 - Más cuentas a la vista y canales que ocupan menos (Priority: P2)

La parte de arriba de Vender gasta menos alto en los canales y deja ver más cuentas en curso a la
vez, también con el ticket abierto y también cuando aparecen las franjas del shell. El canal de la
cuenta sigue a un toque de 44 px y siempre se ve con cuál se está cobrando.

**Why this priority**: es lo que el dueño ve primero, pero sin la historia 1 ver más cuentas solo
muestra más cuentas que no se entienden.

**Independent Test**: medición con navegador a 1024×600 y 1280×854, ticket abierto y cerrado, con y
sin las dos franjas: los productos empiezan más arriba que hoy (o igual con más cuentas visibles,
según la opción elegida en las maquetas) y caben más cuentas que hoy.

**Acceptance Scenarios**:

1. **Given** Vender a 1024×600 con el ticket abierto y 10 cuentas vivas, **When** se mide, **Then**
   se ven más cuentas completas que las 2 de hoy, y las que no caben se siguen pudiendo abrir.
2. **Given** una cuenta de plataforma elegida, **When** se ve la parte de arriba, **Then** el canal
   elegido se distingue de reojo (color) y la parte de arriba no crece dos renglones como hoy.
3. **Given** las franjas de pedidos por aceptar y de agotados abiertas, **When** se mide, **Then**
   Vender sigue mostrando al menos una fila completa de productos y la cuenta activa.
4. **Given** cualquier control nuevo o movido, **When** se mide, **Then** mide al menos 44 px de
   alto y de ancho tappable.

---

### Edge Cases

- **Muchas cuentas** (20 o más): las que no caben siguen alcanzables en un toque; la lista completa
  dice lo mismo que la fila sobre qué le falta a cada una.
- **El valor vacío que significa algo**: una cuenta de $0 entregada está cerrada (no «debe $0»); un
  centavo de redondeo no deja «falta cobrar $0.01» — ya lo decide el servidor (`PedidoSaldado`).
- **El estado que no sobrevive**: tras entregar desde Vender, una recarga (F5) muestra lo mismo; no
  hay estado optimista que el servidor no confirme.
- **Otra tableta cambió la cuenta** (la cobró, la entregó, la canceló) mientras se veía: la fila se
  actualiza por el canal de eventos y entregar algo ya entregado no rompe ni duplica.
- **Doble toque en entregar**: no manda dos veces ni muestra error por la segunda.
- **Cuenta en captura de otra tableta**: no ofrece entregar ni cobrar (no es pedido todavía).
- **Pedido de plataforma**: el cobro solo acepta el método de su plataforma (regla existente); la
  cuenta dice lo que le falta igual que una de mostrador.
- **Sin conexión**: entregar exige servidor; sin red el control no hace como que funcionó.
- **Permisos**: entregar y cobrar no tienen permiso propio en el servidor —los puede quien vende,
  igual que en Pedidos—; la fila no inventa una barrera que el servidor no tiene.
- **Ticket abierto y cerrado, y modo angosto** (pantallas sin panel lateral): las tres disposiciones
  siguen funcionando.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: Cada cuenta de la fila y de la lista completa MUST decir qué le falta para cerrarse,
  derivado SOLO del estado que manda el servidor: en captura → «sin enviar» con total; en cocina sin
  pagar → cobrar (monto) y entregar; pagada sin entregar → entregar; pago parcial → cobrar (lo que
  falta) y entregar; entregada que debe → cobrar (monto); cerrada con algo nuevo → lo nuevo sin
  enviar.
- **FR-002**: El texto de cada estado MUST vivir en un solo lugar del front y usarse igual en la fila,
  la lista completa y el cierre de caja; ninguna pantalla recalcula el estado.
- **FR-003**: Quien vende MUST poder marcar entregado, desde Vender, un pedido al
  que le falta entregar, con el mismo endpoint y permiso que la pantalla de Pedidos.
- **FR-004**: Quien vende MUST poder abrir el cobro de una cuenta a la que le falta
  cobrar desde la cuenta misma, con la hoja de cobro existente.
- **FR-005**: Tras entregar o cobrar, la fila MUST reflejar el estado que confirma el servidor, en
  esta tableta y en las demás.
- **FR-006**: Un error del servidor al entregar MUST mostrarse en lenguaje de quien opera y dejar la
  fila con el estado real.
- **FR-007**: Los canales MUST ocupar menos alto que hoy (44 px de renglón más 12 px arriba, más ~40
  px de texto con plataforma) manteniendo cada canal a un toque de al menos 44 px y el canal activo
  distinguible por color.
- **FR-008**: Vender MUST mostrar más cuentas completas a la vez que hoy a 1024×600 con el ticket
  abierto (hoy 2) y cerrado (hoy 4); las que no caben MUST seguir alcanzables en un toque.
- **FR-009**: La disposición MUST caber a 1024×600 con la franja de entrantes abierta (52 px) y el
  aviso de agotado encima (60 px),
  dejando visibles la cuenta activa y al menos una fila completa de productos; y MUST revisarse
  también a 1280×854.
- **FR-010**: Todo control tappable nuevo o movido MUST medir al menos 44 px; sin `<select>` nativo.
- **FR-011**: La disposición final MUST salir de una maqueta elegida por el dueño entre 2 o 3
  opciones medidas con componentes reales contra la pantalla actual.
- **FR-012**: El texto en pantalla MUST ser para quien opera: sin nombres de estados internos, sin
  justificar decisiones.

### Key Entities

- **Cuenta viva**: un pedido no cerrado o una cuenta en captura, con su estado (decidido por el
  servidor), total, pagado y lo que falta. No cambia su forma en esta feature.
- **Lo que le falta**: la traducción del estado a acciones de mostrador (enviar, cobrar, entregar).
  Es nombre, no regla: no se guarda.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Una persona sin contexto (de una fonda) identifica correctamente qué le falta a cada
  cuenta en los cinco estados vivos, sin explicación previa.
- **SC-002**: Entregar una cuenta pagada desde Vender toma un toque desde que se ve la cuenta (más una
  confirmación, si el plan la exige), sin cambiar de pantalla.
- **SC-003**: A 1024×600 con el ticket abierto se ven al menos el doble de cuentas completas que hoy
  (hoy 2), o la opción elegida en las maquetas declara su cifra y supera la actual.
- **SC-004**: Con una plataforma elegida, la parte de arriba no crece respecto de Mostrador.
- **SC-005**: Con las dos franjas del shell abiertas a 1024×600, la cuenta activa y una fila completa
  de productos siguen visibles.

## Assumptions

- No cambia el servidor: el estado ya lo decide `domain.AccountState` y entregar ya existe
  (`POST /orders/:id/deliver`, sin permiso propio: mismo gate que cobrar y que crear un pedido). Si el plan encuentra que falta algo del
  servidor, lo agrega con su test.
- Entregar desde la cuenta es «entregar todo»; la entrega por renglón sigue en Pedidos.
- Entregar no tiene deshacer (entregada es terminal), igual que en Pedidos, donde es un toque. El
  plan decide si en Vender va con confirmación.
- El presupuesto de diseño de esta feature es 1024×600 por pedido expreso del dueño en el encargo;
  la constitución fija 1280×854 como mínimo y la pantalla se revisa también ahí.
- Sin migración.
