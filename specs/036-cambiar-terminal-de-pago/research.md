# Research: Cambiar la terminal de un pago ya cobrado

## R1. ¿Hace falta migración?
- **Decision**: No.
- **Rationale**: `order_payments.card_terminal_id/card_terminal_name` existen desde 0088; `audit_events` desde 0089 (en `develop`, sube junto). La lista blanca de acciones vive en `domain`, sin `check` en la tabla.
- **Alternatives**: tabla `payment_terminal_changes` propia — duplica la bitácora.

## R2. Concurrencia
- **Decision**: `GetOrderForUpdate` del pedido del pago antes de leer y escribir, igual que devolver un pago.
- **Rationale**: serializa contra cobros, devoluciones y otro cambio del mismo pago; la bitácora queda en el orden real.

## R3. Qué terminales son válidas como destino
- **Decision**: activas de la sucursal de la caja del turno del pago.
- **Rationale**: es la regla de `resolveTerminal` al cobrar; la FK compuesta no basta porque los chequeos de FK saltan RLS y no conocen sucursal.

## R4. Turno cerrado
- **Decision**: se permite. `session_terminal_counts` no se reescribe (es lo declarado y lo esperado al cerrar); el corte compara su `expected` contra `TerminalCollectedForSession` actual y avisa la diferencia.
- **Alternatives**: bloquear turnos cerrados — deja sin salida el caso en que finanzas detecta el error días después; reescribir `expected` — borra lo que se vio al cerrar.

## R5. Quitar la terminal por omisión
- **Decision**: `resolveTerminal` deja de leer la preferencia del usuario; el cobro deja de escribirla. La hoja quita `porOmision`. Con una sola terminal activa sigue eligiéndose sola.
- **Rationale**: el error real nació de la terminal recordada. Un cliente viejo en caché manda `terminalId` siempre que tuviera una efectiva, así que no rebota durante el despliegue; el que no tuviera una ya rebotaba antes.
- **Pendiente**: las rutas para fijar o leer la terminal por omisión quedan sin consumidor; se quitan en el mismo cambio si nada más las usa (se verifica en tasks).

## R6. Refunds atribuidos por terminal
- Las devoluciones con tarjeta se atribuyen a la terminal del último pago con tarjeta del pedido; al cambiar ese pago, su devolución se mueve con él. Es lo correcto: se devolvió en la terminal donde se cobró.
