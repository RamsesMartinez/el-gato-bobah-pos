# Caja y cobro

Notas de mecánica de esta área: lo que no se ve en el código ni en la pantalla y ya costó un defecto. Se leen **antes** de tocar dividir la cuenta, terminales de tarjeta, fondo fijo y retiro, abrir y cerrar caja con asistencia. Vienen de `AGENTS.md` (se movieron el 2026-11-07 porque las instrucciones que carga cada sesión pasaron de 150 mil caracteres); los principios siguen en la constitución.

- **Dividir la cuenta** (spec 027). Cinco cosas que cuestan caro si se olvidan:
  - **Un pago devuelto SALE de `order_payments`** a la bitácora `order_payment_voids`, con su
    cobertura en `covered` y el número que lleva su ticket. Por eso las ~32 consultas que suman pagos
    no filtran devueltos: no hay devueltos ahí. Una consulta nueva que lea la bitácora como si fueran
    pagos cuenta el dinero dos veces.
  - **Qué cubrió cada pago vive en `order_payment_lines`**, y toda lectura que decide si una pieza
    está pagada va bajo el `FOR UPDATE` del pedido (`GetOrderForCharge`, `GetOrderForUpdate`): es lo
    que impide que dos tabletas cobren la misma pieza. Quitar un producto o lo que falta pasa por
    **una sola** validación (`paymentGuard` en `devolucion.go`); un camino nuevo que quite
    renglones la llama, no la copia.
  - **El monto lo calcula el servidor**: `/quote` y `/pay` usan la misma función (`chargeAmount` →
    `domain.SelectionAmount` / `SplitPartAmount`). La hoja no suma ni reparte; pide la cotización.
  - **Pasar productos mueve el renglón y sus movimientos por `order_line_id`**, nunca por
    `order_id`, y partir un renglón inserta pares de movimientos «renglón partido» (el trigger de
    existencias es solo de insert) y reparte `order_line_components`. Pasar todo a otro pedido deja
    el origen `cancelada` con `merged_into_order_id`: lista, conteo y resumen de Ventas y las ventas
    del turno lo excluyen con la misma línea, y no es una cancelación.
  - **Los controles nuevos preguntan por permiso** (`domain.Permission`, `RequirePermission`,
    `can()` en el front), no por nombre de rol: los roles serán de cada empresa (constitución,
    principio VIII). La migración 0079 se niega a bajar en cuanto hay un cobro nuevo; tras
    desplegarla, el rollback es restaurar el respaldo.
- **Terminales de tarjeta** (spec 032, corrección en la 036). Tres cosas que no se ven:
  - **Con dos o más terminales activas, la terminal se elige en CADA cobro; nada llega marcado.**
    Antes llegaba puesta la última con la que cobró el usuario, y así un débito quedó registrado en
    una terminal cuando se pasó en otra. Es una excepción decidida por el dueño a la
    regla de la constitución «quien opera no decide en el momento»: la terminal es un hecho físico
    (dónde se pasó la tarjeta), no algo que la configuración pueda saber. Con una sola activa, va
    sola. En cuenta dividida se limpia tras cada cobro. No la vuelvas a recordar por usuario, en el
    servidor (`resolveTerminal`) ni en la hoja (`terminalEfectiva`).
  - **Corregir la terminal de un pago ya cobrado** es `POST /orders/{id}/payments/{paymentId}/terminal`
    (permiso `payments.change_terminal`) y deja `payment.terminal_changed` en la bitácora en la misma
    transacción. Manda `fromTerminalId`, la terminal que vio la pantalla: si ya no es la del pago,
    responde 409 en vez de pisar otro cambio.
  - **El arqueo de un turno cerrado no se reescribe.** El corte compara al pintarse lo firmado contra
    lo esperado hoy (`expectedNow`), sobre la unión de los renglones guardados y lo cobrado por
    terminal, y avisa por terminal. Solo hay aviso donde hubo arqueo por terminal: con arqueo
    automático no se firmó nada por terminal. Límite que ya existía: una devolución con tarjeta se
    atribuye a la terminal del último pago con tarjeta de su pedido, así que cambiar ese pago mueve
    también la devolución (que puede ser de otro turno), y si la terminal nueva no cobró nada en el
    turno de la devolución, la devolución no aparece en la lista por terminal de ese turno.
- **Fondo fijo y retiro al cierre** (spec 035). Dos cosas que no se ven:
  - **El retiro (`register_cash_movements.kind = 'retiro'`) no es una salida ni entra al neto del
    cajón**: se registra DESPUÉS de contar. Toda suma de movimientos nombra cada tipo con su signo
    (`NetCashMovements`, `corteBreakdown`); una suma «todo lo que no es entrada resta» lo cuenta y
    el corte cerrado reporta ventas infladas por el monto del retiro.
  - **El rollback de la API es solo hacia adelante** en cuanto existe un retiro: la API anterior lo
    resta como salida. El fondo fijo no viaja en `GET /cash-registers` (la apertura es a ciegas).
- **Caja y asistencia** (spec 046). Abrir caja pide una entrada de 16 h o menos de quien abre;
  cerrarla, que quien la abrió no tenga ninguna entrada abierta, salvo el cierre autorizado (permiso
  `cash.close_without_clock_out`, con motivo). Cinco cosas que no se ven:
  - **Una sola regla para tres puertas**: `domain.CashOpenAttendance` / `CashCloseGate` /
    `CashCloseAttendance`. El `GET /cash-sessions/attendance` que pinta la hoja, abrir y cerrar las
    usan; una pantalla que deduzca «ya marcó» por su cuenta dirá un día otra cosa que el servidor.
  - **La persona se toma `for share` (`LockUserAttendanceForCash`) dentro de la transacción**, y todas
    las lecturas de asistencia van con ese mismo `q`, nunca con `s.store.QC`. Choca con el `for update`
    de checar, así que una entrada o salida en otra tableta espera; `for update` frenaría además las
    ventas de esa persona (el `for key share` de `orders.opened_by`). Orden: turno → persona.
  - **El cierre autorizado deja la entrada `missing_out`** con `MarkAttendanceMissingOut`, que tiene
    que afectar exactamente una fila (con 0, 409 sin evento), y escribe
    `cash.closed_without_opener_clock_out` con sujeto = quien abrió, en la misma transacción: si el
    cierre falla después, ni la marca ni el evento quedan. No inventa una hora de salida.
  - **Marcar desde la caja reusa `POST /attendance/clock`** (`ClockPinSheet`): mismo límite, mismo
    bloqueo por PIN y mismos textos (`clockErrors.ts`). No hay un endpoint de checar propio de la caja.
  - **Una cuenta que comparten varias personas para abrir caja necesita «Lleva asistencia» apagado**,
    o nunca podrá abrir. Lo dice la ayuda del horario, no una regla. El grupo `/cash-sessions` sigue
    con `RequireRole` por exención firmada (plan de la 046): la siguiente feature que lo toque lo
    migra completo a permisos.
- **La diferencia de un corte** (#290, spec 082). Tres cosas que no se ven:
  - **Una sola regla, en `domain.CutDifference`**: la usan la lista del Histórico, el detalle y la
    respuesta del cierre. `ListSessions` ya no suma: trae las piezas (diferencia de los métodos
    fuera y dentro del cajón, esperado y contado del conteo de cierre) y Go las junta. Una pantalla o
    un reporte nuevo que sume diferencias por su cuenta vuelve a dar dos cifras del mismo corte.
  - **Con arqueo, los métodos del cajón no suman a la conciliación**: desde la 0067 su renglón guarda
    contado = esperado y su diferencia vive en `session_cash_counts`. La conciliación los pinta con
    «ver arqueo». Sin arqueo con esperado (cortes anteriores a la 0067) el efectivo declarado sí
    cuenta ahí.
  - **El motivo de apertura explica el corte ANTERIOR de su caja** (`explainedByNext`). Se busca con
    un `left join lateral` por `register_id` y no con una ventana `lead`: la ventana corre después
    del filtro y del `limit`, y la última fila de cada página perdía su explicación. La nota viaja
    como `reasonNote`: el MCP borra las llaves `note`/`notes` por ser texto de terceros.
- **Correcciones de un corte cerrado** (#290, spec 082, 0113). Cuatro cosas que no se ven:
  - **Lo firmado no se escribe nunca.** `session_cash_counts`, `register_session_totals` y
    `register_sessions.float_left` se quedan como al cerrar; la corrección vive en
    `cash_session_adjustments` y la diferencia corregida se calcula al leer (`withCorrectedCount`,
    `historyRowDifference`). Un reporte nuevo que lea el conteo directo reporta lo firmado, no lo
    corregido: que pase por la misma función.
  - **Un reverso solo revierte una corrección de SU corte y una vez**: la llave del reverso es
    `(company_id, session_id, reverses_id)` y hay un índice único sobre `reverses_id`; dos tabletas
    que revierten a la vez reciben 409 `ALREADY_REVERSED` la segunda. Revertir un reverso lo impide el
    servicio (`domain.CanReverseAdjustment`), no la base.
  - **Corregir el corte anterior no mueve el turno abierto de esa caja**: su esperado lo fijó su conteo
    de apertura. Corregir el fondo es un dato del corte, no un cambio de lo que se abrió.
  - **El correo del resumen diario sale al cerrar** y es una foto de lo firmado: no se reenvía al
    corregir. Tras desplegar la 0113, el rollback es hacia adelante en cuanto hay correcciones (su
    Down se niega).
- **Cierre por pasos, tolerancia y «Por explicar»** (#290, spec 082 fase 4, 0114). Cuatro cosas que
  no se ven:
  - **Lo que impide cerrar es una sola lista**: `pendientesDelCierre` (en
    `web/src/features/caja/cierreDeCaja.ts`). El botón final, el estado de cada paso y la lista «Para
    cerrar falta» salen de ella. Una condición nueva que apague el botón sin entrar a esa lista vuelve
    al defecto de #251 (nueve condiciones, cuatro nombradas). El servidor sigue siendo la barrera.
  - **El borrador del cierre vive en el aparato** (`localStorage`, `egb:cierre:<caja>:<sesión>`): una
    recarga no pierde lo capturado; si otra tableta cerró ese turno, el borrador se descarta y se
    avisa. No viaja a otra tableta.
  - **«Por explicar» se calcula con la tolerancia de HOY** (`domain.CutNeedsExplanation`): subir o
    bajar la tolerancia reclasifica cortes pasados. Un corte deja de estarlo con una nota vigente o
    con el motivo de la apertura siguiente. Solo lo ve quien configura la caja; con arqueo ciego no
    se pide explicación al cerrar (explicar una diferencia que no se ve es adivinarla), y el
    servidor responde 403 si se manda sin el permiso.
  - **La explicación del cierre se guarda como nota** (`cash_session_adjustments`, tipo `note`) en
    la misma transacción del cierre: si el cierre falla, no queda nota huérfana.
