# Research: 049
- Excluido reusa la marca «personal» existente: un solo control, sin pantalla nueva.
- «Habilita ventas»: opción A del evaluador (una caja vende, cambiable).

## R1: cada `is_primary` de server/queries/ y server/internal/app/ (T002)

«Hoy» = dónde cae la venta de ahora; se queda. «Pasado» = qué caja vendió un turno ya cerrado; deja
de leer `is_primary`.

| Lugar | Pregunta | Clase |
|---|---|---|
| cash.sql `ListCashRegisters…`, `GetCashRegister`, `LockCashRegister…`, `UpdateCashRegister`, `ListOpenSessions` | Mostrar y ordenar cajas | Hoy |
| cash.sql `GetOpenPrimarySession`, `GetOpenPrimarySessionOfBranch`, `LockOpenPrimarySession` (orders, drafts, devolucion, expenses, pedidos de plataforma, backoffice `EstadoDeCaja`) | Turno al que cae una venta/devolución/gasto ahora | Hoy |
| cash.sql `AbrioElTurnoPrincipal` | Desde cuándo se ve un entregado en modo turno (turno abierto) | Hoy |
| cash.sql `CerroLaCajaPrincipal` (orders `desdeCuandoSeVenLosEntregados`) | Último cierre de la caja que vendía | **Pasado → «último cierre de un turno con pedidos»** |
| cash.sql `ClaimOrphanRefunds` (c.is_primary al abrir) | Quién reclama lo huérfano al abrir | Hoy |
| backoffice `OpenSession` (`reg.IsPrimary` reclama pedidos de plataforma y devoluciones) | Al abrir | Hoy |
| backoffice `sessionWithExpected` / `owingOrders` / `liveAccountsForClosing` / `CloseSession` | Solo turnos ABIERTOS (vista en vivo y cierre); mover exige las dos cajas cerradas, así que la caja no cambia de papel con el turno abierto | Hoy |
| backoffice `liveDetail` | Solo para turnos abiertos; el detalle cerrado se lee de lo guardado | Hoy |
| backoffice `UpdateCashRegister` (no desactivar la principal, la principal lleva arqueo) | Configuración actual | Hoy |
