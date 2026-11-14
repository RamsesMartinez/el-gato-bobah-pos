# Gastos y sus pagos

Notas de mecánica de esta área: lo que no se ve en el código ni en la pantalla y ya costó un defecto. Se leen **antes** de tocar pagos de gasto en efectivo, arqueo por hora, renglones excluidos, editar y anular gastos, lo que salió de la caja. Vienen de `AGENTS.md` (se movieron el 2026-11-07 porque las instrucciones que carga cada sesión pasaron de 150 mil caracteres); los principios siguen en la constitución.

- **Un pago de gasto en efectivo ya no exige turno** (spec 047, 0099). Tres cosas que no se ven:
  - **La caja y el corte son columnas distintas**: `expense_payments.register_id` dice de qué caja
    salió y `register_session_id` a qué corte entra. Entra solo si la caja «lleva arqueo», tiene turno
    y el pago es del día de ese turno; lo decide `domain.ExpensePaymentTarget`, y un pago fuera de
    corte de una caja que se cuenta exige `outsideCount` (si no, 409 `EXPENSE_OUTSIDE_COUNT`).
  - **Un trigger deriva `register_id` de la sesión**: la API vieja, durante la subida, no lo manda.
  - **El Down de la 0099 se niega** en cuanto hay un pago con caja y sin corte: perdería la caja.
    Desde ahí, el rollback es restaurar el respaldo.
- **Renglones excluidos y caja que vende** (spec 049, 0101). Tres cosas que no se ven:
  - **Un excluido (`expense_items.excluded`) se guarda y no cuenta en ningún lado**: ni existencias
    (`PlanStockLines` le pone objetivo cero), ni costo, ni «Por recibir», ni «recibir todo», ni el
    historial de precios, ni la suma contra el total. Una consulta nueva sobre `expense_items` que
    hable de mercancía o de dinero filtra `not excluded`. Lo «personal» de la lectura es lo mismo.
  - **«Corregir lo que llegó» no manda los excluidos y eso NO los quita**: `SaveStockLines` los
    conserva. La marca se cambia solo con `PATCH /expenses/{id}/items/{itemId}`, que corrige
    existencias por diferencia.
  - **`is_primary` dice qué caja vende HOY**, y se mueve con `POST /cash-registers/{id}/sells` (las
    dos cerradas). Nada que hable de un turno pasado puede leerla: `CerroLaCajaPrincipal` ya pregunta
    por «el último turno con pedidos». La lista de cada uso está en specs/049/research.md.
- **El arqueo por hora de los gastos** (spec 048, 0100). Tres cosas que no se ven en la pantalla:
  - **Un pago en efectivo tiene tres estados**: en un corte (`register_session_id`), pendiente
    (`include_in_count` sin corte) o fuera. La regla vive en `domain.ExpensePaymentTarget`, contra
    `paid_at` (`domain.PaymentTime`: la hora del reloj si es de hoy, la medianoche de su día si no).
    Sin `includeInCount` en el cuerpo rigen las reglas de la 047: una tableta vieja nunca deja un
    pendiente.
  - **Un pendiente entra a un corte SOLO por `sweepPendingExpenses`** (`app/expenses.go`), y solo al
    abrir: la apertura los incluye sola desde la hora del primero (sin permiso) y lo que queda antes
    de la apertura pasa a «sin incluir»; excluirlos o elegir la hora exige `cash.adjust_count`. Un camino nuevo
    que pegue pagos a un corte lo llama, no lo copia; el `where register_session_id is null` es lo
    que impide pegarlo dos veces.
  - **Cada salida de caja de un gasto lleva su pago** (`register_cash_movements.expense_payment_id`,
    única por pago en las salidas). Una salida nueva sin él no truena, pero desmarcar ese pago
    después responde 409 porque no encuentra qué borrar. Los candados van caja → turno → pago, el
    orden de la apertura y el cierre; tomar el pago primero los cruza.
- **Editar un gasto ya guardado** (spec 053, 0103). Encabezado, agregar y quitar renglones
  (`POST /expenses/{id}/edit`, permiso `expenses.edit`, solo admin) y entregas con su fecha
  (`POST /expenses/{id}/deliveries`, `inventory.receive`). Seis cosas que no se ven:
  - **La vista previa es la misma transacción, revertida** (`runExpenseEdit`): con `?preview=1` corre
    todo y devuelve `errPreviewRollback`; confirmar repite la `fingerprint` de la última vista previa
    (409 `EXPENSE_EDIT_STALE` si algo cambió) y exige sus decisiones (400 `DECISION_REQUIRED`). Una
    cifra de la vista previa que dependa de algo volátil (un id recién insertado) rompe la huella: la
    vista previa revertida consume otro id que la confirmación. **La huella amarra además los
    renglones del gasto de antes de editar** (id, anulado y lo aplicado; `expenseLinesMark`): una
    edición sin efecto en dinero, almacén ni costo —un regalo que queda por recibir— daba la misma
    huella antes y después de aplicarse, y un reintento la duplicaba (revisión de la 074,
    `TestTheFingerprintDoesNotReapplyAnEditWithoutEffects`).
  - **`stock_movements.effective_on` es el día de la entrega**, no el de la captura. La llena un
    trigger en cada compra o movimiento de un gasto (`business_day_of`, con la zona de la EMPRESA de la
    fila), así que la API vieja también deja fecha; los reportes por periodo siguen con `created_at`.
    El Down de la 0103 se niega en cuanto hay una fecha capturada distinta del día de su captura.
    **Un turno sembrado en una prueba va con `business_day_of(empresa, now())`, nunca con
    `current_date`**: ése es el día de la zona de la sesión de Postgres (UTC en CI), y de 18:00 a
    24:00 del centro ya es mañana. El gasto toma el día del turno y la entrega el del negocio, queda
    «entregado antes de comprarse» y editarlo truena con `domain.ValidDeliveryDate`. Detuvo un release
    con la prueba en rojo solo de noche (`TestMCPSeedShiftIsOnTheBusinessDay`). La API abre el turno
    con la zona de la empresa (`OpenSession`), así que en el producto no pasa por la hora.
  - **Una entrega puede ser anterior a la fecha del gasto; futura, nunca** (FR-015 enmendado,
    `domain.ValidDeliveryDate`). La fecha del gasto la fija el turno cuando hay caja abierta, y la
    mercancía llega antes en la operación real. Antes crear la aceptaba y editar la rechazaba: la
    compra ya no se podía corregir (arreglado; `TestAPurchaseThatCouldBeCreatedCanBeEdited`). Crear,
    «Llegó todo», entregar y agregar un renglón que ya llegó pasan por la misma función; un camino
    nuevo que fije `received_at` o `effective_on` también.
  - **«No mover» ante un conteo posterior es un movimiento más, no la ausencia de uno**: el del renglón
    se registra igual (lo aplicado sigue siendo la suma de sus movimientos) y otro `ajuste` contrario
    lleva `stock_count_id`, `expense_id`, SIN `expense_item_id` y la sucursal del conteo explícita.
    «Conteo posterior» = renglón de un conteo cerrado de la sucursal capturado después de
    `least(registro, fin del día de la entrega)`; recibir hoy nunca pregunta.
  - **El importe se mueve con `domain.ExpenseAmountAfterEdit`**: lo agregado ocupa primero lo que el
    importe tenía sin desglosar; borrar lo baja por su renglón. El estado se recalcula solo si cambió
    el importe y una `pagada` sin ningún pago no se degrada. Los pagos nunca se tocan.
  - **Anular un gasto capturado por error** (#216, permiso `expenses.cancel`, motivo obligatorio)
    vale pendiente o pagado. Es borrado lógico: el gasto queda `cancelada` y sus pagos se quedan
    como fila, sin contar en ningún arqueo (`domain.CancelPaymentPlan`): el de un corte **abierto**
    sale de él con su salida de caja, y el que espera corte deja de esperarlo, en la misma
    transacción. Los candados van caja → turno → gasto, como la casilla del arqueo. **Un pago en un
    corte cerrado lo impide** (409 `EXPENSE_IN_CLOSED_COUNT`): eso ya es una devolución (#175), no se
    reescribe un corte contado. La mercancía recibida se revierte con la regla de la 038.
- **Lo que salió de la caja** (spec 050, 0102). Un pago en efectivo de un gasto con renglones
  excluidos guarda en `expense_payments.drawer_extra` lo que salió del cajón además de su importe.
  Tres cosas que no se ven:
  - **El monto de la salida de caja de un pago lo pone la base**, no Go: un trigger en
    `register_cash_movements` lo fija en `amount + drawer_extra` del pago, sea cual sea el monto
    que mande el INSERT. Un camino nuevo que registre la salida de un pago no suma nada.
  - **El tope es por gasto** (lo excluido vivo, y si el ticket traía total, tampoco más de
    `document_total − amount`; menos lo que otros pagos ya usaron) y se valida con
    el gasto tomado (`LockExpenseForStock`). Volver a incluir un renglón, o corregir renglones, que
    dejaría lo excluido por debajo de lo usado responde 400 `DRAWER_EXTRA_TOO_HIGH`.
  - **El corte la presenta en «No es del negocio», no en «Gastos»** (`ListCashMovements` trae
    `drawer_extra`). Sumarla a Gastos reportaría un gasto mayor que el registrado.
