# Feature Specification: El aviso de versión nueva no tapa la pantalla

**Feature Branch**: `064-aviso-de-version`

**Created**: 2026-11-05

**Status**: Draft

**Input**: Issue #102. El aviso «Nueva versión disponible / Actualizar» aparece flotando arriba al
centro de cualquier pantalla y no se cierra solo. En las pruebas sin contexto de la 040
(2026-10-17), a 1024×600, quedó encima de «Agregar lugar» en Negocio y lo tapó. Es un aviso de toda
la app, no de una pantalla: hay que decidir dónde vive sin quitarle alto a lo que se está usando ni
tapar ningún botón. Hoy existe además un punto verde de 6 px sobre la versión, en el pie del riel
lateral, que casi nadie ve.

**Decisiones de la sesión** (por delegación del dueño, 2026-11-05):

- El aviso de versión nueva deja de flotar sobre el contenido. Vive en un lugar fijo del marco de la
  app que no le quita alto a la pantalla de trabajo; la forma exacta la decide la maqueta.
- El aviso de «Instalar app» queda **fuera de alcance**: se puede cerrar, en una tableta ya
  instalada no aparece nunca y ya tiene su botón fijo en Configuración. Si una prueba lo ve tapando
  algo, se abre otro issue.
- Los demás avisos flotantes (errores, confirmaciones) no cambian: son de una acción que la persona
  acaba de hacer y se cierran solos.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - El aviso no tapa lo que se está usando (Priority: P1)

Quien opera está en cualquier pantalla —Vender, Pedidos, Negocio, Caja— cuando sale una versión
nueva. El aviso aparece en su lugar fijo del marco, y todos los botones de la pantalla siguen
visibles y se pueden tocar a la primera, sin cerrar ni mover nada.

**Why this priority**: es el defecto del issue. Un botón tapado en una tableta es un toque perdido
o, peor, un toque que cae en «Actualizar» y recarga a media captura.

**Independent Test**: a 1024×600, abrir Negocio (donde está «Agregar lugar») y Vender, provocar el
aviso de versión nueva y comprobar que ningún control de la pantalla queda debajo del aviso y que el
área de trabajo conserva exactamente el mismo alto que sin aviso.

**Acceptance Scenarios**:

1. **Given** Negocio abierto a 1024×600, **When** llega una versión nueva, **Then** «Agregar lugar»
   y todo control de la pantalla siguen visibles y tocables, sin nada encima.
2. **Given** Vender abierto con una cuenta en captura, **When** llega una versión nueva, **Then**
   la fila de cuentas, los canales, el buscador y el pie de cobro siguen visibles y tocables.
3. **Given** cualquier pantalla, **When** aparece el aviso, **Then** el área de trabajo mide el
   mismo alto que antes del aviso: nada se recorre hacia abajo.

---

### User Story 2 - El aviso se ve y se aplica con un toque (Priority: P1)

Quien opera nota que hay una versión nueva sin buscarla, y la aplica con un solo toque cuando le
conviene (entre pedidos). El aviso se queda hasta que se aplica; no se pierde por tocar en otro lado
ni por cambiar de pantalla.

**Why this priority**: mover el aviso a un rincón invisible arreglaría el issue y rompería lo que el
aviso viene a hacer. Hoy el único aviso persistente es un punto de 6 px que nadie ve.

**Independent Test**: provocar el aviso, cambiar de pantalla dos veces y comprobar que sigue en su
lugar; tocarlo una vez y comprobar que la app se actualiza.

**Acceptance Scenarios**:

1. **Given** una versión nueva esperando, **When** quien opera cambia de pantalla, **Then** el aviso
   sigue visible en el mismo lugar.
2. **Given** el aviso visible, **When** se toca su control de actualizar (al menos 44 px de alto),
   **Then** la app se actualiza con ese único toque.
3. **Given** una cuenta en captura en Vender, **When** se aplica la actualización, **Then** la cuenta
   sigue ahí después de recargar (como hoy).
4. **Given** que no hay versión nueva, **When** se usa la app, **Then** no se ve ningún aviso ni
   lugar reservado vacío que ocupe espacio de trabajo.

---

### Edge Cases

- **Riel con muchas opciones**: con el rol de administrador el riel ya hace scroll a 600 px. El
  aviso no puede empujar fuera de la vista la opción de la pantalla abierta ni el botón «Salir».
- **Pantallas fuera del marco**: el inicio de sesión no tiene riel; ahí el aviso ocupa el lugar
  de su logo (una versión que no deja entrar se arregla pasando a la siguiente). Recuperar y
  restablecer contraseña no lo muestran: vuelven al inicio de sesión. La pantalla de bloqueo por PIN
  tapa el riel; al desbloquear, el aviso sigue ahí.
- **La pantalla se cae con versión nueva esperando**: «Reiniciar» vuelve a cargar la versión rota
  (el service worker viejo sigue atendiendo mientras el nuevo espera). La barrera ofrece además
  «Actualizar a la versión nueva». Antes lo cubría el toast, que vivía fuera de la barrera.
- **Varias pestañas**: cada pestaña muestra su aviso; aplicar en una recarga las demás una sola vez.
  Si otra pestaña ya la aplicó y ésta no recargó, «Actualizar» recarga en vez de quedarse puesto sin
  hacer nada.
- **Doble toque** sobre «Actualizar»: no debe provocar dos recargas (ya lo impide la marca por
  pestaña).
- **Diálogo u hoja abierta**: el aviso queda detrás de la capa, como el resto del marco; no compite
  con la hoja que se está usando.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: El aviso de versión nueva MUST mostrarse en un lugar fijo del marco de la app, fuera
  del área de trabajo de la pantalla, y nunca encima de un control.
- **FR-002**: Mostrar el aviso MUST NOT reducir ni desplazar el alto del área de trabajo.
- **FR-003**: El aviso MUST seguir visible desde que se detecta la versión nueva hasta que se
  aplica, en todas las pantallas del marco.
- **FR-004**: Aplicar la versión MUST requerir un solo toque sobre un control de al menos 44 px de
  alto.
- **FR-005**: El aviso flotante de versión nueva MUST dejar de existir; no hay dos avisos del mismo
  hecho.
- **FR-006**: Sin versión nueva, el marco MUST verse y medir igual que hoy.
- **FR-007**: El aviso MUST NOT sacar de la vista la opción de la pantalla abierta ni el botón
  «Salir» del riel a 1024×600.
- **FR-008**: El texto del aviso MUST ser para quien opera (qué pasa y qué tocar), sin nombrar
  piezas técnicas.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: A 1024×600, en Vender, Pedidos, Negocio y Caja, con el aviso visible, cero controles
  de la pantalla quedan cubiertos por el aviso.
- **SC-002**: El alto del área de trabajo con y sin aviso es el mismo, al píxel, en esas cuatro
  pantallas.
- **SC-003**: Una persona sin contexto, puesta en Vender con una versión nueva esperando, encuentra
  y aplica la actualización a la primera, sin ayuda.
- **SC-004**: Aplicar la actualización toma exactamente un toque.

## Assumptions

- El mecanismo que detecta la versión nueva y recarga una sola vez por pestaña se queda como está;
  esto cambia solo dónde y cómo se muestra el aviso.
- La cuenta en captura sobrevive a la recarga porque vive en el aparato (comportamiento actual).
- No hay cambio de servidor, ni de base, ni de permisos: cualquier persona que use la app puede
  aplicar la versión nueva, como hoy.
- La verificación es con Playwright a 1024×600 en local, simulando la llegada de la versión nueva.
