# Feature Specification: Fondo de caja fijo

**Feature Branch**: `035-fondo-fijo`
**Created**: 2026-10-11
**Status**: Draft
**Input**: "El fondo que se deja al cerrar la caja hoy se teclea en cada cierre («Se queda de fondo para el siguiente turno»). Que sea un fondo fijo configurado una vez en Caja, que el cierre lo traiga ya escrito y se pueda cambiar solo ese día, y que el retiro (lo contado menos el fondo) quede registrado como salida de caja con quién se lo llevó. Ya está en producción: respetar los cierres que guardaron un fondo tecleado."

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Configurar el fondo fijo una vez (Priority: P1)

Quien administra la caja entra a Ajustes › Caja y captura cuánto se queda de fondo al cerrar cada
turno (por ejemplo $1,000). Desde ese momento ningún cierre pide teclearlo.

**Why this priority**: Sin el valor configurado no hay nada que el cierre pueda traer escrito; es la
base de las otras dos historias.

**Independent Test**: Capturar $1,000 en Ajustes › Caja, salir, volver y ver $1,000; un usuario sin
permiso de configurar caja no puede cambiarlo.

**Acceptance Scenarios**:

1. **Given** una caja sin fondo fijo, **When** quien tiene permiso captura $1,000 y guarda, **Then** Ajustes › Caja muestra $1,000 y el siguiente cierre lo trae escrito.
2. **Given** un fondo fijo de $1,000, **When** se cambia a $1,500, **Then** los cierres ya hechos conservan lo que guardaron y solo los siguientes usan $1,500.
3. **Given** un usuario sin el permiso, **When** intenta cambiar el fondo fijo, **Then** el sistema lo rechaza aunque la petición no pase por la pantalla.

---

### User Story 2 - El cierre trae el fondo escrito y se puede cambiar solo ese día (Priority: P1)

Al cerrar el turno, el campo «Se queda de fondo para el siguiente turno» ya aparece con el fondo
fijo. Quien cierra cuenta el cajón y, si ese día se deja otra cantidad (por ejemplo falta cambio),
la cambia ahí; el cambio aplica solo a ese cierre y no toca el fondo configurado.

**Why this priority**: Es el toque que se quita en cada cierre y la fuente de errores de captura.

**Independent Test**: Con fondo fijo $1,000, cerrar un turno sin tocar el campo guarda $1,000; cerrar
otro cambiándolo a $800 guarda $800 y el siguiente cierre vuelve a traer $1,000.

**Acceptance Scenarios**:

1. **Given** fondo fijo $1,000, **When** se cierra sin tocar el campo, **Then** el cierre guarda $1,000 como fondo que se deja.
2. **Given** fondo fijo $1,000, **When** se cambia a $800 en un cierre, **Then** ese cierre guarda $800, se nota que fue distinto del fijo, y el fijo sigue en $1,000.
3. **Given** una caja sin fondo fijo configurado, **When** se cierra, **Then** el campo aparece vacío y se teclea como hoy.
4. **Given** lo contado es menor que el fondo fijo, **When** se abre el cierre, **Then** el sistema avisa antes de guardar que no alcanza para dejar el fondo (hoy ya avisa «Es más de lo contado en el cajón»).

---

### User Story 3 - El retiro queda registrado como salida con quién se lo llevó (Priority: P2)

Al cerrar, lo contado menos el fondo que se deja es el retiro. Ese retiro queda registrado como
salida de caja del turno, con quién se lo llevó, y aparece en el corte y en las salidas del turno.

**Why this priority**: Hoy el retiro es una resta implícita: el dinero sale del cajón sin rastro de
quién lo tiene.

**Independent Test**: Contado $5,300, fondo $1,000 → existe una salida de $4,300 ligada a ese cierre,
con la persona elegida; el faltante/sobrante del arqueo no cambia por registrarla.

**Acceptance Scenarios**:

1. **Given** contado $5,300 y fondo $1,000, **When** se cierra eligiendo a la persona que se lleva el dinero, **Then** queda una salida de $4,300 con esa persona, ligada al cierre.
2. **Given** quien cierra elige «No, todo se queda en el cajón», **When** cierra, **Then** el fondo que se deja es lo contado, no hay salida y no se pide persona.
3. **Given** retiro de $0 (contado igual al fondo), **When** se cierra, **Then** no se registra salida y no se pide persona.
4. **Given** retiro mayor que cero, **When** se intenta cerrar sin elegir quién se lo llevó, **Then** el cierre no se guarda y lo dice.
5. **Given** el retiro registrado, **When** se lee el corte del turno, **Then** el retiro aparece una sola vez y no altera ni el esperado ni la diferencia del arqueo.

---

### Edge Cases

- **Cierres históricos con fondo tecleado**: se muestran con la cifra que guardaron, sin recalcular contra el fondo fijo, y no se les crea un retiro retroactivo.
- **Campo borrado en el cierre**: vacío no es $0. Si se borra, el cierre pide una cifra; $0 se escribe explícito (deja el cajón vacío y todo es retiro).
- **Fondo mayor que lo contado**: no se guarda un retiro negativo; el cierre avisa y exige corregir el fondo de ese día (máximo: lo contado).
- **Cambio del fondo fijo con un turno abierto**: el cierre de ese turno trae el valor vigente al abrir la hoja de cierre.
- **Doble envío del cierre (dos toques, red lenta, dos tabletas)**: un solo cierre y una sola salida de retiro.
- **Recarga o caída de red a media captura**: lo tecleado puede perderse, pero nunca queda un cierre sin su retiro ni un retiro sin cierre.
- **Persona que se lleva el retiro dada de baja después**: la salida conserva quién fue.
- **Corregir un retiro mal registrado**: el retiro nace con el cierre y el turno ya está cerrado; no se reversa ni se edita. Lo que salió mal se aclara en las notas del cierre (fuera de alcance corregirlo después).
- **El historial no adelanta la siguiente apertura**: con arqueo ciego, quien no configura la caja no ve en el último cierre de una caja que no se ha vuelto a abrir ni el fondo dejado, ni el retiro, ni lo contado, ni nada de donde salga (diferencia, esperado, ingresos). La pantalla dice «Arqueo oculto» y el histórico «Oculto», nunca $0. Al abrir la caja de nuevo, el corte se ve completo.
- **Varias cajas en la sucursal**: cada caja tiene su propio fondo fijo.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: El sistema DEBE permitir configurar un fondo fijo por caja en Ajustes › Caja, como cantidad en pesos ≥ $0, solo a quien tenga el permiso de configurar caja (validado en el servidor).
- **FR-002**: El cierre DEBE traer escrito el fondo fijo vigente en el campo «Se queda de fondo para el siguiente turno».
- **FR-003**: Quien cierra DEBE poder cambiar el fondo de ese cierre; el cambio aplica solo a ese cierre y no modifica el fondo fijo.
- **FR-004**: Cada cierre DEBE guardar el fondo que se dejó y si fue distinto del fijo vigente.
- **FR-005**: El sistema DEBE calcular el retiro como lo contado menos el fondo que se deja, en el servidor, y rechazar un retiro negativo.
- **FR-006**: Si el retiro es mayor que cero, el cierre DEBE registrar una salida de caja del turno por ese monto, con quién se lo llevó (persona del personal de la empresa) y ligada al cierre, en la misma operación que el cierre: o se guardan los dos o ninguno.
- **FR-007**: El cierre DEBE exigir quién se lleva el retiro cuando es mayor que cero, eligiéndolo de una lista táctil, no tecleándolo.
- **FR-007a**: El cierre DEBE ofrecer a la vista no retirar nada («todo se queda en el cajón»): el fondo que se deja es lo contado y no se registra retiro. Por omisión el cierre propone retirar.
- **FR-008**: El retiro NO DEBE contarse en el esperado del arqueo ni en el faltante/sobrante; el corte DEBE mostrarlo una sola vez y declarar que es el dinero retirado al cerrar.
- **FR-009**: Los cierres guardados antes de esta feature DEBEN conservar su fondo tecleado tal cual; no se les genera retiro.
- **FR-010**: La apertura del turno siguiente DEBE seguir comparándose contra el fondo que se dejó en el cierre anterior, no contra el fijo.
- **FR-011**: Un cierre enviado dos veces DEBE producir un solo cierre y una sola salida de retiro.
- **FR-012**: La pantalla de cierre y la de Ajustes › Caja DEBEN tener maqueta elegida por el dueño antes de construirse (constitución, *Quality gates*).

### Key Entities

- **Fondo fijo de la caja**: cantidad que se deja al cerrar; una por caja; la cambia quien configura.
- **Cierre de turno**: ya existe; guarda lo contado y el fondo que se dejó; se agrega si ese fondo difirió del fijo.
- **Retiro al cierre**: una salida de caja del turno, una por cierre, con monto y quién se lo llevó; no se reversa.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Un cierre con el fondo fijo configurado se completa sin teclear el fondo (0 toques en ese campo) en el caso normal.
- **SC-002**: El 100% de los cierres nuevos con retiro mayor que cero tienen su salida con persona; ninguno queda sin ella.
- **SC-003**: Los cierres previos muestran exactamente la misma cifra de fondo antes y después de la subida.
- **SC-004**: El faltante/sobrante de un turno es idéntico con y sin el retiro registrado.
- **SC-005**: La subida se hace con el negocio operando, sin cerrar cajas abiertas.

## Assumptions

- «Caja» es la caja registradora (cada una ya pertenece a una sucursal); el fondo fijo es por caja, no por empresa.
- El permiso para configurar el fondo fijo es el mismo que hoy configura Ajustes › Caja; se pregunta por permiso, no por nombre de rol.
- «Quién se lo llevó» se elige del personal activo de la empresa; por omisión aparece quien está cerrando.
- El retiro es su propio tipo de salida, rotulado «Retiro al cierre», y no se ofrece en la captura manual de salidas ni se suma con ellas.
- Sin fondo fijo configurado, el cierre se comporta como hoy (campo vacío), y el retiro se registra igual cuando hay fondo tecleado.
- Las cajas abiertas al subir cierran con la regla nueva; nada exige caja cerrada.
