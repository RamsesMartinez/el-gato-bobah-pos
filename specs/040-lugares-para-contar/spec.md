# Feature Specification: Lugares para contar

**Feature Branch**: `040-lugares-para-contar`

**Created**: 2026-10-16

**Status**: Draft

**Input**: Decisión del dueño del 2026-10-16. En Almacén › Artículos, el «Lugar para contar» (spec 038)
se escribe a mano en un campo de texto. Por eso «Refri» y «refri» quedan como dos lugares distintos y
el conteo los separa. Se cambia por un catálogo de lugares de cada empresa.

## Contexto

El lugar de un artículo solo sirve para que el conteo vaya en el orden en que se camina el local:
primero el refri, luego la barra, luego la bodega. Hoy cada artículo guarda su lugar como texto
libre. Un lugar mal escrito parte el recorrido en dos, y nadie puede renombrar un lugar para todos
sus artículos a la vez ni decidir en qué orden se recorren.

El lugar **no** es un almacén: no guarda existencias propias ni permite traspasos. Más de un almacén
por sucursal es otra feature (issue #65).

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Elegir el lugar de un artículo de una lista (Priority: P1)

Quien administra el almacén abre un artículo y elige su lugar de la lista de lugares de la empresa,
en una hoja con renglones grandes. Si el lugar no existe, lo crea ahí mismo. También puede dejar el
artículo sin lugar.

**Why this priority**: es lo que el dueño vio mal. Sola, ya evita que un lugar mal escrito parta el
recorrido.

**Independent Test**: con los lugares «Refri» y «Barra», asignar «Refri» a un artículo; escribir
«refri » como lugar nuevo ofrece el «Refri» que ya existe y no crea otro.

**Acceptance Scenarios**:

1. **Given** una empresa con los lugares «Refri» y «Barra», **When** se abre el lugar de un
   artículo, **Then** se ven los dos en una hoja con renglones de 44 px o más, sin selector nativo.
2. **Given** un artículo sin lugar, **When** se elige «Refri», **Then** el artículo queda en
   «Refri».
3. **Given** que el lugar no está en la lista, **When** se escribe uno nuevo, **Then** se crea en
   la lista y queda elegido.
4. **Given** que se escribe «refri » y ya existe «Refri», **When** se intenta crear, **Then** no
   se crea otro y queda elegido «Refri».
5. **Given** un artículo con lugar, **When** se elige «Sin lugar», **Then** el artículo queda sin
   lugar.

---

### User Story 2 - Administrar los lugares y su orden (Priority: P2)

Quien administra el almacén ve, en Negocio › Inventario, la lista de lugares de la empresa. Puede
agregar, renombrar, cambiar el orden (el orden es el recorrido del conteo) y archivar.

**Why this priority**: sin el orden, el recorrido sigue saliendo alfabético. Depende de que exista
la lista (US1).

**Independent Test**: renombrar «Refri» a «Refrigerador» cambia el lugar de todos sus artículos;
arrastrar «Bodega» antes de «Refri» hace que el conteo empiece por «Bodega».

**Acceptance Scenarios**:

1. **Given** «Refri» con 12 artículos, **When** se renombra a «Refrigerador», **Then** los 12
   artículos dicen «Refrigerador».
2. **Given** los lugares en orden Refri, Barra y Bodega, **When** se arrastra «Bodega» al primer
   lugar, **Then** el siguiente conteo empieza por los artículos de «Bodega».
3. **Given** un lugar con artículos, **When** se archiva, **Then** se pregunta a qué lugar pasan sus
   artículos, con «Sin lugar» como opción, y deja de ofrecerse al elegir lugar.
4. **Given** un lugar sin artículos, **When** se archiva, **Then** se archiva sin preguntar.
5. **Given** que se renombra un lugar con el nombre de otro que ya existe (sin distinguir
   mayúsculas, acentos ni espacios), **Then** no se guarda y se dice que ya existe.

---

### User Story 3 - Los lugares escritos a mano pasan a la lista (Priority: P1)

Los lugares que ya se escribieron a mano pasan a la lista de su empresa sin perder ninguno. Los que
solo difieren en mayúsculas, acentos o espacios se juntan en uno solo.

**Why this priority**: sin esto, al estrenar la lista los artículos que ya tenían lugar lo
perderían. Es parte del cambio, no una mejora posterior.

**Independent Test**: una empresa con artículos en «Refri», «refri » y «Barra» termina con dos
lugares, «Refri» y «Barra», y cada artículo conserva el suyo.

**Acceptance Scenarios**:

1. **Given** artículos con «Refri», «refri » y «Barra», **When** se hace el cambio, **Then** la
   empresa tiene dos lugares y cada artículo apunta al que corresponde.
2. **Given** dos empresas que escribieron «Refri», **When** se hace el cambio, **Then** cada una
   tiene su propio «Refri».
3. **Given** un artículo sin lugar, **When** se hace el cambio, **Then** sigue sin lugar.

### Edge Cases

- **Nombre vacío o de más de 30 letras**: no se guarda.
- **Nombre repetido**, sin distinguir mayúsculas, acentos ni espacios: no se crea otro. Al crear
  desde el artículo, queda elegido el que ya existe.
- **Cambiar lugares con un conteo abierto**: el conteo abierto toma el orden nuevo al volver a
  cargarse; lo ya capturado no se pierde, porque lo capturado es del artículo y no del lugar.
- **Un lugar archivado** no se ofrece al elegir y no aparece en el recorrido. Sus artículos ya se
  movieron al archivarlo.
- **Dos personas renombran o reordenan a la vez**: gana la última, sin duplicar ni perder lugares.
- **Lugar de otra empresa**: no se puede elegir ni ver.
- **Artículos sin lugar**: en el conteo van al final, como hoy.
- **Reordenar**: se mantiene presionada la agarradera del lugar y se arrastra (decisión del dueño, maqueta C). Desplazar la lista o tocar el renglón no mueve nada.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: Cada empresa MUST tener su propia lista de lugares, con nombre, orden y si está
  archivado. Una empresa nueva arranca sin lugares: ningún lugar viene fijo en el producto.
- **FR-002**: El nombre de un lugar MUST tener de 1 a 30 letras y no repetirse en la empresa sin
  distinguir mayúsculas, acentos ni espacios.
- **FR-003**: En el artículo, el lugar MUST elegirse de la lista en una hoja inferior con buscador.
  La hoja MUST ofrecer «Sin lugar» y permitir crear uno nuevo ahí mismo escribiendo su nombre. Nunca un campo
  de texto libre ni un selector nativo.
- **FR-004**: Crear un lugar con un nombre que ya existe MUST elegir el que existe en vez de crear
  otro, y decirlo en pantalla («Ya tenías «Refri»; quedó elegido»).
- **FR-005**: Quien administra el almacén MUST poder, en Negocio › Inventario, agregar, renombrar,
  reordenar arrastrando y archivar lugares. Arrastrar es la única forma de reordenar (decisión del
  dueño, 2026-10-16); archivar un lugar sin artículos pide confirmación de un paso.
- **FR-006**: Renombrar un lugar MUST cambiar el lugar que muestran todos sus artículos.
- **FR-007**: Archivar un lugar con artículos MUST pedir a qué lugar activo pasan, o «Sin lugar».
  Mover y archivar son una sola operación e incluyen los artículos inactivos. Un lugar archivado no
  se ofrece, y ningún artículo puede quedar ligado a uno, aunque otra persona asigne ese lugar en el
  mismo momento.
- **FR-008**: El conteo MUST recorrer los artículos por el orden de su lugar y, dentro de cada
  lugar, por nombre. Los artículos sin lugar van al final.
- **FR-009**: Los lugares escritos a mano MUST pasar a la lista de su empresa sin que ningún
  artículo pierda su lugar. Los que solo difieren en mayúsculas, acentos o espacios se juntan, y el
  orden inicial es alfabético.
- **FR-010**: Asignar el lugar de un artículo y crear uno nuevo desde ahí MUST pedir el permiso que
  hoy pide el lugar (contar el almacén); renombrar, reordenar y archivar, el de configurar el
  almacén. Verificados en el servidor, nunca por nombre de rol.
- **FR-011**: Los textos de la pantalla MUST usar palabras de quien opera («Lugar», «Sin lugar»,
  «Crear…», «Mantén presionada ≡ para mover un lugar»), nunca «catálogo», «almacén» ni nombres internos.

### Key Entities

- **Lugar**: pertenece a una empresa. Tiene nombre, posición en el recorrido y si está archivado.
- **Artículo** (ya existe): ahora apunta a un lugar de su empresa, o a ninguno, en lugar de guardar
  el texto.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Elegir el lugar de un artículo que ya está en la lista toma 2 toques: abrir la hoja y
  tocar el lugar.
- **SC-002**: Después del cambio, ningún artículo que tenía lugar queda sin él, y ninguna empresa
  tiene dos lugares que solo difieran en mayúsculas, acentos o espacios.
- **SC-003**: Renombrar un lugar actualiza el 100% de sus artículos en una sola acción.
- **SC-004**: El siguiente conteo recorre los lugares en el orden que fijó la empresa.
- **SC-005**: Una persona nueva asigna un lugar y crea uno nuevo sin ayuda, en la prueba con
  usuario sin contexto.

## Assumptions

- Archivar con artículos pide a dónde moverlos, con «Sin lugar» como opción. Así ningún artículo
  queda apuntando a un lugar que ya no se ofrece. Es el valor por omisión elegido; el dueño puede
  cambiarlo en `/speckit-clarify`.
- Un lugar no se borra, se archiva, igual que los motivos de merma.
- El lugar es de la empresa y no de la sucursal, igual que el artículo. Cuando haya varios almacenes
  por sucursal (#65), el lugar seguirá solo ordenando el recorrido.
- Arrastrar alcanza para las decenas de lugares de un local de comida.
- Fuera de alcance: existencias por lugar, traspasos entre lugares y más de un almacén por sucursal.
