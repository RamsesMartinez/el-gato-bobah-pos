# Feature Specification: Importe que salió de la caja

**Feature Branch**: `050-salio-de-la-caja`

**Created**: 2026-11-01

**Status**: Draft

**Input**: Lo último del issue #111: la «propuesta por defecto» del ticket mixto pagado de una caja que
lleva arqueo («importe que salió de la caja»), y tres detalles que encontró la prueba con usuario
nuevo de la spec 049 (interruptores chicos en Caja › Administrar, «Excluir» del detalle sin texto ni
confirmación, botón de pasar las ventas con otro color). Menos es más.

**Decisión registrada**: «varias cajas vendiendo a la vez» NO entra. Cruza la puerta «Más de una caja
vendiendo» del principio VIII y obliga a decidir cómo elige su caja cada pedido; un evaluador
independiente la puntuó 8/8/2 (valor/riesgo/tamaño) contra 6/2/8 de la opción que ya subió la 049. Es
decisión del dueño y queda en el issue.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - El efectivo que de verdad salió del cajón (Priority: P1)

Un ticket de $500 trae $120 que no son del negocio (excluidos). El gasto es de $380, pero del cajón
salieron $500. Al pagar en efectivo de una caja con arqueo, quien captura ve «Salió de la caja», con
$380 por omisión, y puede escribir hasta $500. La diferencia ($120) sale del esperado del corte junto
con el gasto, con el motivo «Parte que no es del negocio».

**Independent Test**: con la caja abierta, el ticket anterior con «Salió de la caja» $500: el gasto
queda en $380 y el esperado del corte baja $500.

**Acceptance Scenarios**:

1. **Given** un pago en efectivo de una caja con arqueo en un gasto con renglones excluidos, **Then**
   aparece «Salió de la caja» con el importe del pago.
2. **Given** se escribe más que el importe y hasta el importe más lo excluido, **When** se guarda,
   **Then** el gasto conserva su total y el corte descuenta lo que salió, con el motivo.
3. **Given** se escribe más de lo permitido o menos que el importe, **Then** se rechaza.
4. **Given** el pago queda pendiente de corte, **When** entra al siguiente corte, **Then** entra con lo
   que salió, no solo con el importe.
5. **Given** sin excluidos, tarjeta o caja sin arqueo, **Then** el campo no aparece.

---

### User Story 2 - Tres detalles de pantalla (Priority: P2)

1. Los interruptores de Caja › Administrar se tocan con el dedo a la primera (44 px).
2. En el detalle del gasto, «Excluir» dice «Excluir» y, si el renglón ya entró al almacén, pide
   confirmar porque mueve existencias.
3. «Pasar las ventas a…» usa el color de las demás acciones principales.

### Edge Cases

- Desmarcar «En el arqueo» de un pago con diferencia: se quita del corte todo lo que salió.
- Un gasto sin documento leído: el tope es el importe más la suma de excluidos.
- Pagos ya guardados: salió de la caja = su importe.
- Volver a incluir un renglón excluido mientras un pago lo usa como diferencia: se rechaza.
- El corte muestra la diferencia en «No es del negocio», no como gasto.

## Requirements *(mandatory)*

- **FR-001**: Un pago en efectivo de una caja con arqueo MUST poder guardar cuánto salió del cajón,
  entre su importe y su importe más lo excluido del gasto que no hayan usado ya otros pagos (el tope es por gasto).
- **FR-002**: Todo lo que salió MUST descontarse del corte al que entra el pago (al guardar o al
  entrar como pendiente), una sola vez, y quitarse completo si se desmarca.
- **FR-003**: El total del gasto MUST no cambiar por esto.
- **FR-004**: Los tres detalles de pantalla de la US2.

## Success Criteria *(mandatory)*

- **SC-001**: El esperado de un corte cuadra con el efectivo real de un ticket mixto.
- **SC-002**: Un ticket sin excluidos no muestra nada nuevo.

## Fuera de alcance

- Varias cajas vendiendo a la vez (ver «Decisión registrada»).
