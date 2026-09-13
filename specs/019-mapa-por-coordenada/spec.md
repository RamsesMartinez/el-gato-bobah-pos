# Feature Specification: Dónde cae el dedo — el mapa de calor por coordenada

**Feature Branch**: `019-mapa-por-coordenada`

**Created**: 2026-09-12

**Status**: Draft

**Input**: Ver *Origen* al final.

> **Por qué 019 y no 018**: el número 018 está reservado para las **acciones de soporte** desde que
> se escribió la spec 016, y ya está citado en [016/spec.md](../016-consola-de-plataforma/spec.md),
> en su plan, en sus tareas y en [docs/security-owasp.md](../../docs/security-owasp.md). Renumerar
> rompería cuatro referencias por un número.

## Contexto

La [spec 017](../017-mapa-de-calor-de-uso/spec.md) ya dice **qué** pantallas se usan y **qué**
acciones se disparan. Esta responde la otra mitad: **dónde, dentro de una pantalla, cae el dedo**.

La diferencia importa para decidir. Saber que la pantalla de Caja se abre cuarenta veces al día no
dice si el botón de cerrar turno está donde la mano lo alcanza, ni si la mitad inferior la ve
alguien alguna vez. En una tableta de 1024×600 cada renglón que sobra es contenido que no cabe
—el presupuesto está medido y peleado renglón por renglón en
[docs/presupuesto-de-pantalla-1024x600.md](../../docs/presupuesto-de-pantalla-1024x600.md)— y hoy
para mover, agrandar o borrar un control se sigue suponiendo.

Es la mitad que la 017 aplazó **a propósito** (su US3), con la puerta abierta y medida: el modelo de
aquella no cierra ésta, y crear lo que ésta necesita no obliga a migrar nada de lo escrito.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Ver en qué zonas de una pantalla cae el dedo (Priority: P1)

El operador de plataforma entra a la consola (spec 016), elige una pantalla del POS y un periodo, y
ve una **rejilla con la proporción de esa pantalla**, sombreada según cuántos toques cayeron en cada
zona. Con eso decide qué mover, qué agrandar y qué quitar, mirando en vez de suponiendo.

**Why this priority**: es la feature. Sin esto no hay nada.

**Independent Test**: tras un día de uso en una pantalla instrumentada, el mapa muestra al menos una
zona con conteo mayor que cero, y la zona que corresponde al control más usado es la más oscura.

**Acceptance Scenarios**:

1. **Given** un día de operación normal, **When** el operador abre el mapa de una pantalla,
   **Then** ve la rejilla completa con cada zona sombreada según su conteo, y el número a la vista.
2. **Given** una zona donde nadie tocó nunca, **When** se mira el mapa, **Then** aparece en cero y
   se distingue a simple vista de las que sí se tocaron.
3. **Given** dos pantallas instrumentadas, **When** el operador cambia de una a otra, **Then** el
   mapa cambia de forma si la proporción de las pantallas es distinta.
4. **Given** un periodo sin toques registrados, **When** se abre el mapa, **Then** lo dice con
   palabras y **no** pinta una rejilla vacía que parezca una falla.

---

### User Story 2 - Nunca una captura, nunca la persona (Priority: P1)

El mapa se pinta **solo**: una rejilla con la proporción de la pantalla y nada debajo. El sistema
**nunca** captura, transmite ni guarda una imagen de la pantalla de un cliente. Y como en la 017, se
guarda el **rol** y jamás la persona; si el rol identifica por eliminación, tampoco se guarda el rol.

**Why this priority**: P1 y no P2 porque no es una capa que se agregue encima. La tentación de
pintar las manchas sobre una captura real —que se entiende mejor— es exactamente la decisión que no
se puede deshacer: una captura de la pantalla de un cliente lleva **nombres de clientes, contenido
de pedidos e importes**. Si se capturó una vez, se capturó.

**Independent Test**: con el sistema en uso, ninguna petición que salga de la tableta contiene una
imagen, y ninguna consulta sobre los datos guardados permite reconstruir la pantalla que alguien
estaba viendo ni quién la tocó.

**Acceptance Scenarios**:

1. **Given** el POS en uso, **When** se inspecciona todo lo que sale de la tableta hacia el
   servidor, **Then** no hay ninguna imagen ni nada que describa el contenido de la pantalla.
2. **Given** los datos guardados, **When** alguien los consulta por cualquier vía, **Then** no
   existe forma de saber qué tocó un empleado concreto ni cuándo.
3. **Given** un rol con un solo empleado activo en esa empresa, **When** se guardan sus toques,
   **Then** quedan sin rol.

---

### User Story 3 - Que el dedo no se entere (Priority: P1)

Quien opera no nota que sus toques se cuentan: ni un toque más lento, ni un botón que espera, ni un
error cuando se cae el wifi. Si la medición no se puede entregar, se pierde en silencio.

**Why this priority**: P1 por la misma razón que en la 017, y aquí con más filo: esto se engancha al
**toque mismo**, no al final de una acción. Si algo puede demorar un toque, es esto.

**Independent Test**: con la red del local caída, el operador arma una cuenta y cobra sin ver un
solo error ni una demora atribuible a la medición.

**Acceptance Scenarios**:

1. **Given** el servidor sin responder, **When** el operador usa el POS, **Then** todo funciona
   igual y no aparece ningún aviso.
2. **Given** un toque del operador, **When** se registra, **Then** el registro ocurre **después** de
   que la interfaz respondió al toque, nunca antes.
3. **Given** una ráfaga de toques —el operador captura veinte productos seguidos—, **Then** no se
   dispara una petición por toque.

---

### User Story 4 - Que quepa, sabiendo lo que ya costó (Priority: P1)

El volumen de esta medición no crece con la cantidad de toques. Miles de toques por turno suben
contadores; no crean filas.

**Why this priority**: P1 y no P2 —al revés que en la 017— porque aquí el riesgo ya se midió en
carne propia. En aquella se propuso un renglón por evento y la auditoría adversarial lo tumbó por
dos caminos: **la marca de tiempo por evento se cruza con el cierre de turno y deshace el anonimato
entero**, y el volumen al tope del limitador daban **4 GB en dos semanas** para una sola cuenta. Un
mapa por coordenada es ese mismo caso con más fuerza: son miles de toques por turno, no decenas.

**Independent Test**: simulando un trimestre de uso al tope de lo que el limitador permite, el
espacio ocupado se mantiene por debajo de un techo declarado y verificable.

**Acceptance Scenarios**:

1. **Given** meses de toques acumulados, **When** se mide el espacio, **Then** está por debajo del
   techo declarado.
2. **Given** una ráfaga de toques en la misma zona, **When** se guardan, **Then** el número de
   filas no cambia.
3. **Given** datos más viejos que el periodo de conservación, **When** corre el recorte, **Then**
   desaparecen sin intervención de nadie.

---

### Edge Cases

- **La pantalla que se desplaza.** El POS tiene pantallas con más contenido del que cabe —medido:
  `/caja` tiene 1,978 px de contenido en 600 px visibles—. Un toque "a la mitad de la pantalla" no
  cae sobre el mismo control si el operador desplazó antes. Está resuelto en FR-004: se mide **dónde
  cae el dedo sobre el vidrio**, no sobre qué elemento cae, y el spec dice qué significa eso.
- **Arrastrar no es tocar.** Desplazar la lista con el dedo empieza con un contacto y termina en
  otro lugar. Contarlo como toque llenaría el mapa de rastros de desplazamiento en vez de
  intenciones.
- **Tabletas de tamaños distintos.** Un píxel no significa lo mismo en dos pantallas, así que la
  posición se guarda **relativa** al área visible (FR-005).
- **La tableta en horizontal y en vertical.** Cambiar la orientación cambia la proporción; mezclar
  las dos en un solo mapa pinta zonas que no existieron.
- **Un toque fuera de toda zona medible** (la barra del sistema operativo, un menú del navegador) no
  llega a la aplicación y por lo tanto no se mide. No es un hueco: es el límite de lo que la
  aplicación puede ver.
- **Dos tabletas con la misma cuenta.** Como en la 017: no se guarda la persona, así que no estorba.
- **El sistema recién instalado.** El mapa lo dice; no pinta una rejilla en cero que se lea como una
  falla.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: El sistema MUST registrar en qué **zona** de una pantalla instrumentada cae cada toque
  del operador.
- **FR-002**: El registro MUST guardar el **rol** de quien tocó y MUST NOT guardar la identidad de
  la persona ni nada de lo que permita deducirla.
- **FR-003**: El registro MUST NOT permitir saber **cuándo** ocurrió un toque con más precisión que
  el día. Un instante por toque, cruzado con la operación del negocio, identifica a la persona
  aunque no se guarde su nombre.
- **FR-004**: La posición MUST medirse respecto del **área visible** de la pantalla —dónde cae el
  dedo sobre el vidrio—, no respecto del contenido desplazado.

  Es una decisión con consecuencia: en una pantalla que se desplaza, una misma zona corresponde a
  contenidos distintos en momentos distintos. Lo que el mapa responde es *«qué parte del vidrio usa
  la mano»*, que es lo que decide dónde poner un control; *«qué control se toca»* ya lo responde la
  017 con las acciones con nombre.

- **FR-005**: La posición MUST guardarse **relativa** al área visible y no en píxeles, para que dos
  tabletas de tamaños distintos sean comparables.
- **FR-006**: El sistema MUST distinguir un **toque** de un **arrastre** y MUST NOT contar el
  segundo.
- **FR-007**: El registro MUST NOT bloquear, demorar ni hacer fallar ningún toque. Si no se puede
  entregar, **se descarta**.
- **FR-008**: El sistema MUST seguir funcionando sin red exactamente igual que hoy.
- **FR-009**: El sistema MUST NOT capturar, transmitir ni almacenar **ninguna imagen** de la
  pantalla, ni ningún texto de lo que la pantalla mostraba.
- **FR-010**: El operador de plataforma MUST poder ver la rejilla de una pantalla, elegir el periodo
  y la empresa, y ver el conteo de cada zona.
- **FR-011**: La rejilla MUST pintarse sin librerías de gráficas de terceros.
- **FR-012**: El volumen MUST NO crecer con la cantidad de toques: una ráfaga sobre la misma zona
  MUST NOT crear filas nuevas.
- **FR-013**: El spec MUST declarar el techo de espacio y el periodo de conservación, y ambos MUST
  ser verificables.
- **FR-014**: El sistema MUST NOT ofrecer un corte por rol cuando ese corte identifique a una
  persona por eliminación, con la misma regla que la 017.
- **FR-015**: Las pantallas instrumentadas MUST ser una **lista corta y explícita**, y agregar una
  MUST costar una línea.

  No se mide todo el sistema: se empieza donde el dedo está todo el día. Medir doce pantallas para
  mirar dos es volumen y ruido a cambio de nada.

- **FR-016**: Los mapas de dos **orientaciones o proporciones distintas** MUST NO mezclarse en una
  sola rejilla.
- **FR-017**: Con cero toques en el periodo, la pantalla MUST decirlo y MUST NOT pintar una rejilla
  vacía.
- **FR-018**: Los datos MUST estar aislados por empresa.

### Key Entities

- **Zona de una pantalla**: una celda de la rejilla, definida de forma relativa al área visible, con
  la proporción de la pantalla a la que pertenece.
- **Conteo de toques**: cuántos toques cayeron en una zona, de una pantalla, de una empresa, de un
  rol, en un día. **No hay entidad "toque"**: el toque individual no se guarda, y ésa es la
  decisión que hace posible cumplir FR-003 y FR-012 a la vez.
- **Catálogo de pantallas instrumentadas**: la lista corta de FR-015.

## Success Criteria *(mandatory)*

- **SC-001**: Después de un día de operación, el operador de plataforma puede nombrar **la zona más
  tocada y la menos tocada** de una pantalla instrumentada, sin preguntarle a nadie.
- **SC-002**: **Ninguna** consulta sobre los datos, por ninguna vía, permite reconstruir qué tocó
  una persona concreta ni a qué hora.
- **SC-003**: **Ninguna** petición que sale de la tableta contiene una imagen ni texto de la
  pantalla.
- **SC-004**: Con el servidor caído, capturar y cobrar un pedido toma **los mismos pasos y el mismo
  tiempo** que con el servidor arriba.
- **SC-005**: Simulado un trimestre de uso **al tope de lo que el limitador permite**, el espacio
  ocupado se mantiene por debajo del techo declarado.
- **SC-006**: Una ráfaga de mil toques en la misma zona **no cambia** el número de filas guardadas.
- **SC-007**: La rejilla se pinta **sin agregar una sola dependencia** al proyecto.
- **SC-008**: El paquete que descarga la tableta no crece más de **5 kB** respecto de la medición
  anterior al cambio.

## Assumptions

- **La rejilla es fija y gruesa.** Una cuadrícula de pocas decenas de celdas basta para decidir
  dónde va un control; una de miles sería un mapa de puntos con otro nombre, y traería de vuelta el
  problema de volumen que esta feature evita por diseño.
- **Se empieza por el POS.** Es donde el dedo está todo el día. Las demás pantallas se agregan
  cuando la 017 diga que vale la pena, que es exactamente para lo que sirve tenerla primero.
- **El día es el del negocio**, con su zona horaria, igual que la venta y que la 017.
- **La consola es el único lector.** Ninguna pantalla del negocio muestra esto: es investigación del
  producto, no un reporte del local.

## Out of scope

- **Pintar sobre una captura de la pantalla.** Prohibido por FR-009, y es la razón de US2.
- **Seguir el recorrido del dedo** (de dónde a dónde se mueve). Eso exige guardar secuencia, y la
  secuencia es tiempo: choca de frente con FR-003.
- **Mapas por tamaño de tableta.** Hoy todas las del local son iguales; cuando no lo sean, la
  proporción ya está guardada y el corte se puede agregar.
- **Medir la consola a sí misma.**

## Origen

Pedido por el dueño el 2026-09-12, al ver el mapa de la 017 funcionando y preguntar *«¿en dónde
quedaron los mapas de calor?»* — los de las manchas sobre la interfaz, que es lo que se había
imaginado desde el principio y que la 017 dejó aplazado a propósito.
