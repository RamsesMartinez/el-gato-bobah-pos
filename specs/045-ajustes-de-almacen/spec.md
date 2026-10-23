# Feature Specification: Ajustes de Almacén tras la prueba de la 044

**Feature Branch**: `045-ajustes-de-almacen`

**Created**: 2026-10-22

**Status**: Draft

**Input**: User description: "issue #127: pendientes de Almacén tras la prueba de la 044"

Origen: la prueba con usuario sin contexto de la spec 044 (2026-10-21) dejó cinco hallazgos fuera de
su alcance (specs/044-ajustes-de-articulos-y-conteo/plan.md › «Al construir»). Esta feature los
cierra.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - La misma existencia se lee igual en todas las pantallas (Priority: P1)

Quien revisa el almacén ve un artículo en Existencias y luego en Artículos. Hoy el mismo artículo
sale «-3000 ml» en una y «-3 L» en la otra, y «1 pieza» contra «1 piezas». La persona duda de cuál
cifra es la buena o cree que son dos artículos distintos.

**Why this priority**: es una cifra de inventario que parece contradecirse; resta confianza en todo
el almacén y lleva a conteos o compras equivocadas.

**Independent Test**: abrir un artículo con existencia negativa en mililitros y uno en piezas, y
comparar lo que muestran Existencias, Artículos y la ficha del artículo.

**Acceptance Scenarios**:

1. **Given** un artículo en mililitros con existencia de -3000 ml, **When** se ve en Existencias, en
   Artículos y en su ficha, **Then** las tres muestran la misma cifra con la misma unidad.
2. **Given** un artículo en piezas con existencia de 1, **When** se ve en cualquier pantalla del
   almacén, **Then** dice «1 pieza»; con 2 o 0, «piezas».
3. **Given** cantidades de 0.5 kg, 1 500 g o 2 L, **When** se muestran, **Then** todas las pantallas
   eligen la misma unidad legible para la misma cantidad.

---

### User Story 2 - «Revisar con un conteo» lleva al conteo (Priority: P2)

En la ficha de un artículo con una cifra dudosa aparece «Revisar con un conteo», pero es texto: la
persona tiene que ir a la pestaña Conteo y buscar el artículo a mano.

**Why this priority**: es la acción que el propio sistema sugiere; si no se puede tocar, el operador
la ignora.

**Independent Test**: desde la ficha, tocar «Revisar con un conteo» y llegar a capturar ese artículo.

**Acceptance Scenarios**:

1. **Given** un conteo abierto que incluye el artículo, **When** se toca «Revisar con un conteo»,
   **Then** se abre el conteo con ese artículo listo para capturar.
2. **Given** que no hay conteo abierto, **When** se toca, **Then** se llega a la pestaña Conteo, donde
   se puede empezar uno, con el artículo ya buscado.
3. **Given** un usuario sin permiso para contar, **When** ve la ficha, **Then** no se le ofrece la
   acción como algo que pueda tocar.

---

### User Story 3 - El buscador del conteo encuentra lo que se buscó (Priority: P2)

Al buscar «apio» en el conteo sale también «Tapioca», porque coincide en medio de la palabra.

**Why this priority**: en un conteo de decenas de artículos, un resultado de más es un toque en el
renglón equivocado y una cifra capturada en otro artículo.

**Independent Test**: buscar «apio» con «Apio» y «Tapioca» en el catálogo.

**Acceptance Scenarios**:

1. **Given** «Apio» y «Tapioca», **When** se busca «apio», **Then** sale «Apio» y no «Tapioca».
2. **Given** «Leche de almendra», **When** se busca «almen», **Then** sale (coincide al inicio de una
   palabra).
3. **Given** «Azúcar», **When** se busca «azucar» o «AZÚ», **Then** sale (sin distinguir acentos ni
   mayúsculas).

---

### User Story 4 - La barra lateral no corta el primer renglón (Priority: P3)

Con la página desplazada hacia abajo, el primer elemento de la barra lateral («Pedidos») queda medio
cortado arriba.

**Why this priority**: es cosmético, pero deja una opción de navegación medio oculta en la tableta.

**Independent Test**: a 1024×600, bajar una página larga y mirar la barra lateral.

**Acceptance Scenarios**:

1. **Given** una página larga desplazada hasta abajo, **When** se mira la barra lateral, **Then**
   «Pedidos» se ve completo y se puede tocar.

---

### User Story 5 - El tema oscuro sigue al del sistema (Priority: P3)

Pedir modo oscuro al sistema o al navegador no cambia nada; solo cambia desde Interfaz › Tema.

**Why this priority**: quien trabaja de noche espera que la tableta respete lo que ya eligió en el
sistema.

**Independent Test**: con el sistema en oscuro y sin haber elegido tema en el POS, abrir la app.

**Acceptance Scenarios**:

1. **Given** que nunca se eligió tema en Interfaz › Tema, **When** el sistema está en oscuro,
   **Then** el POS abre en oscuro; y cambia si el sistema cambia con la app abierta.
2. **Given** que se eligió «Claro» u «Oscuro» en Interfaz › Tema, **When** el sistema cambia,
   **Then** se respeta lo elegido en el POS.
3. **Given** Interfaz › Tema, **When** se abre, **Then** ofrece también «Como el sistema» y es la
   opción por omisión.

### Edge Cases

- Cantidad cero, negativa, con decimales largos (0.3333 L) o enorme: la misma forma en todas las
  pantallas.
- Unidad sin plural propio (kg, ml, L) contra unidad con plural (pieza, bolsa, empaque de la empresa).
- Búsqueda vacía, de una letra, con espacios de más o con varias palabras («leche alm»).
- «Revisar con un conteo» sobre un artículo archivado o un conteo que se cierra mientras se navega.
- Preferencia de tema guardada antes de esta feature (claro u oscuro explícito): se respeta tal cual.
- Barra lateral más alta que la pantalla de 600 px.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: Toda pantalla del almacén que muestre una existencia o cantidad de un artículo MUST usar
  una sola regla para elegir la unidad mostrada y su redondeo.
- **FR-002**: El nombre de la unidad MUST concordar en número con la cantidad (singular solo para 1).
- **FR-003**: «Revisar con un conteo» MUST ser una acción tocable que lleva al conteo del artículo,
  abierto si existe, o a la pestaña Conteo con el artículo buscado si no.
- **FR-004**: La acción de FR-003 MUST ofrecerse solo a quien tiene permiso de contar.
- **FR-005**: El buscador del conteo MUST coincidir solo al inicio de una palabra del nombre, sin
  distinguir mayúsculas ni acentos; con varias palabras, cada una debe coincidir al inicio de alguna.
- **FR-006**: La barra lateral MUST mostrar completo su primer elemento sin importar el desplazamiento
  de la página.
- **FR-007**: El tema MUST ofrecer «Como el sistema», «Claro» y «Oscuro», con «Como el sistema» por
  omisión, y seguir cambios del sistema en vivo mientras esté elegido.
- **FR-008**: Una elección explícita de tema guardada antes de esta feature MUST conservarse.

### Key Entities

- **Preferencia de tema**: por aparato; valores sistema, claro, oscuro.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Para cualquier artículo, las cifras de existencia en Existencias, Artículos y la ficha
  son idénticas en texto (0 diferencias en una revisión de todos los artículos de la base restaurada).
- **SC-002**: Desde la ficha se llega a capturar el artículo en un conteo abierto en 1 toque.
- **SC-003**: Buscar «apio» en el conteo da 0 resultados que no empiecen palabra con «apio».
- **SC-004**: A 1024×600, con cualquier página desplazada, el primer elemento de la barra lateral se ve
  completo y mide al menos 44 px tocables.
- **SC-005**: Con el sistema en oscuro y sin elección previa, el POS abre en oscuro sin tocar nada.

## Assumptions

- La regla de unidad que gana es la de Artículos (convierte a la unidad legible: 3000 ml → 3 L); se
  ajusta en el plan si alguna pantalla necesita la unidad base.
- El buscador de FR-005 aplica al conteo; si otros buscadores del almacén comparten el componente, se
  alinean con la misma regla.
- La preferencia de tema sigue guardándose por aparato, como hoy.
- No hay cambios de esquema ni de servidor esperados, salvo que la unidad mostrada venga ya formateada
  de la API.
