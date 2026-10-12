# Research: Fondo de caja fijo

## R1. Dónde vive el fondo fijo

- **Decisión**: columna `cash_registers.fixed_float numeric(10,2)` nulable, `check (>= 0)`.
- **Por qué**: supuesto confirmado (por caja). Nulo = «no configurado» y el cierre se comporta como hoy; `0` es un valor válido (dejar el cajón vacío) y no se confunde con «sin configurar».
- **Descartado**: ajuste por empresa (`company_settings`): una sucursal con dos cajas no podría tener fondos distintos.

## R2. Cómo se nota que el fondo de un cierre difirió del fijo

- **Decisión**: snapshot `register_sessions.fixed_float_at_close numeric(10,2)` nulable, escrito al cerrar con el fijo vigente. «Difirió» = `float_left <> fixed_float_at_close`, derivado al leer.
- **Por qué**: constitución VIII (snapshot). Si se cambia el fijo mañana, el cierre de hoy sigue diciendo contra qué se comparó. Cierres previos: nulo → no se muestra comparación (FR-009).
- **Descartado**: booleano `float_overridden`: pierde contra qué cifra se comparó.

## R3. El retiro: tipo de movimiento propio, no `salida` con concepto

- **Decisión**: nuevo `kind = 'retiro'` en `register_cash_movements`, con `recipient_user_id`/`recipient_name` (columnas que ya existen desde 0087 para propinas). Índice único `(session_id) where kind = 'retiro'`.
- **Por qué**:
  - `NetCashMovements` (`server/queries/cash.sql`) resta **todo** lo que no es `entrada`/`reverso`. Un retiro sumado ahí restaría del esperado del cajón el mismo dinero que acaba de contarse → el arqueo reportaría un sobrante igual al retiro (SC-004). La consulta se cambia a `kind in ('salida','propina')` explícito para resta y el retiro queda fuera; el test nombra el concepto duplicado (constitución III).
  - Las salidas por concepto (`daily_summary.sql`, `cash_concepts.sql`) filtran `kind = 'salida'`: el retiro queda fuera de «salidas a mano» sin tocar esas consultas.
  - El índice único es la idempotencia de FR-011 en la base, no en la pantalla.
- **Descartado**: concepto «Retiro al cierre» en `cash_concepts`: obligaba a excluirlo por nombre en cada consulta de salidas y en la captura manual; un renombre lo rompía en silencio.
- **Reverso**: `domain.CanReverse` solo admite `salida`; el retiro no se reversa (spec, edge case).

## R4. Compatibilidad durante la subida (con el negocio operando)

- **Decisión**: migración solo aditiva (dos columnas nulables, ampliar checks, índice parcial). El servidor nuevo acepta el cuerpo viejo: `floatLeft` ausente → usa `fixed_float` si existe, si no lo contado (como hoy). `retiroUserId` ausente con retiro > 0 → por omisión quien cierra (supuesto confirmado), así una tableta con el front viejo cierra sin error y el retiro queda registrado.
- **Por qué**: constitución, *Se sube a producción con el negocio operando*.
- **Número de migración**: **se toma el número libre siguiente al momento de fusionar**. Hoy develop llega a 0089 y la rama 034 también trae una 0089; ese choque lo resuelve la sesión del POS al fusionar. Esta rama no toca la 034. Mientras tanto el archivo se nombra con el siguiente libre sobre develop y se renumera al fusionar si hace falta.

## R5. Permiso

- **Decisión**: `domain.PermCashConfigure = "cash.configure"`, para admin y gerente en `PermissionsFor`; la ruta usa `RequirePermission`.
- **Por qué**: constitución VIII, roles por empresa: ningún control nuevo pregunta por nombre de rol.

## R6. Vacío no es cero

- **Decisión**: en la pantalla, campo vacío bloquea «Cerrar» con el mensaje «Escribe cuánto se queda». En el servidor, ausente = usar el fijo/lo contado (compatibilidad R4); un valor presente y malformado → 400 (principio V).

## R7. Ajustes tras la revisión de arquitectura (2026-10-12)

- **`corteBreakdown` (Go, `app/backoffice.go`) suma movimientos por su cuenta** y restaba todo lo que no fuera entrada/reverso: un retiro inflaba «Ventas» del corte cerrado por el monto del retiro. Se agrega `domain.CashRetiro`, un `case` propio que no toca `net` ni «Salidas de efectivo», y un renglón «Retiro al cierre» que el total de egresos nombra. Test sobre el corte cerrado: falla diciendo «el retiro se restó de las ventas».
- **El fijo nunca rebota un cierre**: cuando el fondo sale del fijo (cliente viejo sin `floatLeft`), `ResolveFloatLeft` usa `min(fijo, contado)`. Solo un `floatLeft` explícito mayor a lo contado es 400.
- **Apertura a ciegas**: `fixedFloat` NO viaja en `GET /cash-registers` (lo lee el cajero al abrir y el fijo es casi siempre el esperado de apertura). Viaja en la vista del turno abierto (hoja de cierre) y en `GET /cash-registers/all` (configuración, con permiso).
- **El retiro se ve antes de cerrar sin revelar el esperado**: es contado − fondo, dos cifras que captura el operador; no se compara contra el esperado ni entra a `netMovements` antes de cerrar.
- **Doble cierre**: la garantía es `LockSessionForClose`; el índice único es una segunda barrera y `traduceConflictoDeCaja` lo mapea a `ErrConflict`.
- **Rollback de la API**: una vez que existen retiros, volver a la API vieja infla las ventas de esos cortes (su `corteBreakdown` los resta). El rollback de esta feature es solo hacia adelante.

## R8. El historial no revela la próxima apertura (revisión de código, 2026-10-12)

- **Hallazgo**: con arqueo ciego, un cajero podía abrir en el historial el detalle del último cierre de su caja. Ahí estaban el fondo dejado y el retiro, y cualquiera de los dos da el esperado de la apertura siguiente: el fondo, o lo contado menos el retiro.
- **Decisión**: `SessionDetailFor(ctx, id, canSeeFloat)`. Sin `PermCashConfigure`, con arqueo ciego y en un turno cerrado, se quitan el fondo dejado, el fijo contra el que se comparó y el retiro, que aparecía en tres lugares: el campo, el movimiento y el renglón de egresos, cuyo total se ajusta. Quien configura la caja lo ve completo. Lo vigila `TestClosedShiftDetailDoesNotRevealNextOpeningToCashier`.
- **Lo que esto no cierra**:
  - Lo contado del cierre sigue a la vista en el detalle. En un cierre sin retiro («todo se queda» o retiro 0), lo contado es el fondo dejado, y por lo tanto el esperado de la apertura.
  - Pasaba igual antes de esta spec: todo cierre anterior a la decisión del 2026-10-10 dejaba lo contado como fondo.
  - Cerrarlo pide ocultar lo contado y lo declarado del cierre más reciente de cada caja. Es una decisión del dueño, fuera del alcance de la 035.
