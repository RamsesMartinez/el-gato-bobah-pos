# Research: Defectos de dinero (031)

Cada decisión dice qué defecto resuelve (D1–D18 del spec), qué se eligió y qué se descartó.

## R1 — Lo ya devuelto por medio (D1)

- **Decisión**: `SumOrderPaymentsByMethod` trae, además de lo cobrado por medio, lo devuelto por ese
  medio (pre-agregado aparte: `order_payments` y `order_refunds` son dos 1:N de `orders`).
  `domain.RepartirDevolucion` reparte contra `cobrado − devuelto` de cada medio.
- **Alternativa descartada**: restar en Go con otra consulta por medio. Dos lecturas sobre el mismo
  dinero y una regla fuera del dominio.

## R2 — Candado del pedido al devolver (D3)

- **Decisión**: `devolverEnTx` toma `GetOrderForUpdate` antes de leer nada. `CancelarConDevolucion`,
  `VoidPayment` y `Charge` ya lo toman: con esto las cuatro operaciones de dinero de un pedido se
  serializan sobre la misma fila. Repetir el candado dentro de la misma transacción es un no-op.
- **Descartado**: `serializable` (reintentos en toda la app) y una restricción en la base que tope la
  suma (un trigger de suma por pedido es lógica de negocio escondida y no da el mensaje correcto).

## R3 — Tope de un renglón (D4)

- **Decisión**: `domain.MontoDevolvibleDeRenglon(cobrado, devueltoTotal, importeRenglon,
  devueltoRenglon) = min(queda del pedido, importe − devuelto del renglón)`. El renglón se lee con
  `order_id` en el `where` y vivo (`cancelled_at is null`); si no, `ErrNotFound`. Importe =
  `line_total` (DD-5).

## R4 — Pedido «reembolsado» (D18)

- **Decisión**: `devolverEnTx` rechaza `reembolsada` con `ErrConflict` («ese pedido ya devolvió su
  dinero»). Con el candado de R2 el estado se lee bloqueado. `cancelada` sigue pasando por el tope
  normal: un cancelado con devolución completa ya no tiene nada que devolver.

## R5 — Devolver un pago con devoluciones (D2)

- **Decisión**: `domain.VoidKeepsRefunds(pagadoDelMedioSinEstePago, devueltoDelMedio)`: rechaza si el
  segundo supera al primero, con `ErrPaymentHasRefunds`. Por medio y no "cualquier devolución"
  (DD-4): una devolución por tarjeta no impide devolver el pago en efectivo.

## R6 — La devolución tiene turno y día (D6, D7, decisiones del dueño)

- **Decisión**: migración **0080** agrega a `order_refunds`: `register_session_id` (llave compuesta
  `(company_id, register_session_id)` contra `register_sessions(company_id, id)` como 0061: los
  chequeos de FK saltan RLS), `business_date` (día de negocio por el reloj de la app, igual que
  `orders.business_date`) y `tip_amount` (D9). Backfill: el turno de las de efectivo sale de su
  salida de caja (con `m.company_id = r.company_id`); el día, de `created_at` con la misma expresión
  de zona que 0062. Las de tarjeta hechas durante un turno que **sigue abierto** al migrar se ligan a
  ese turno (`created_at >= opened_at`, misma empresa, caja principal): es único por caja, así que
  aquí la ventana sí es exacta, y sin esto ese turno cerraría sin restarlas. Las de turnos ya cerrados
  quedan sin turno a propósito: su corte ya guardó su cifra.
- FK `on delete no action` (no `restrict`: el borrado de una empresa cae en cascada por las dos
  tablas en el mismo statement). `lock_timeout = '3s'` como 0060.
- Down: se niega si hay devoluciones de solo propina (`amount = 0`), porque el check viejo no las
  admite y borrar `tip_amount` perdería ese dinero.
- Un esperado puede quedar negativo (devolución en B de un pago de A): es la decisión del dueño, y el
  arqueo y la pantalla lo toleran con su prueba.
- **Esperado**: `ExpectedByMethodForSession` devuelve `refunded` = devoluciones del turno por ese
  medio **sin salida de caja** (lo que salió del cajón ya baja por `NetCashMovements`; restarlo dos
  veces sería el doble conteo). `sessionWithExpected` resta `refunded` del esperado.
- Toda devolución lee el turno con `LockOpenPrimarySession` (`for share`), también la de tarjeta: así
  un cierre concurrente la espera o ella lo ve cerrado (FR-009).
- **Sin turno**: efectivo → `ErrNoOpenRegisterForCashRefund` (DD-3). Otro medio → se guarda sin turno
  y `ClaimOrphanRefunds` lo pasa al turno que se abre, solo si se hizo después del último cierre de
  esa caja (así no barre las devoluciones viejas sin turno).
- **Descartado**: ventana de tiempo para decidir el turno (el código ya la rechazó por frágil con dos
  cajas); rechazar también tarjeta sin turno (dejaría sin cancelar un pedido de Uber de madrugada).

## R7 — Pagos de plataforma sin turno (D5)

- **Decisión**: `ClaimPlatformOrder` se acompaña de `ClaimPlatformOrderPayments` (mismo `order_id`,
  solo `register_session_id is null`). Sin migración de datos: para lo histórico, R10 trata un pago
  sin turno como del turno del pedido.

## R8 — Cierre con candado (D8)

- **Decisión**: `CloseSession` abre la transacción primero, bloquea la sesión `for update` con
  `status = 'abierta'` (`LockSessionForClose`), y dentro calcula pendientes y esperados con el `q` de
  la transacción. `sessionWithExpected` recibe el `*db.Queries`. `RecordCashMovement` y `Transfer`
  bloquean `for share` la(s) sesión(es) dentro de su transacción (`LockOpenSessionForShare`), en
  orden de id para no interbloquearse. Un cobro que espera el candado ve la sesión cerrada al
  reevaluar el `where` (READ COMMITTED) y rebota con «no hay caja abierta».

## R9 — Cancelar con devolución regresa la propina (D9, DD-2)

- **Decisión**: `CancelarConDevolucion` calcula por medio la propina cobrada menos la ya devuelta y la
  devuelve en el mismo renglón del libro (`tip_amount`), con la salida de caja por monto + propina.
  El `check (amount > 0)` pasa a `amount >= 0 and tip_amount >= 0 and amount + tip_amount > 0`.
  `refund_amount` del pedido sigue sumando solo `amount`: la propina no es ingreso.

## R10 — Cobros de otros turnos (D12)

- **Decisión**: `UncollectedInSession` resta solo los pagos de ese turno o sin turno: lo que falta
  de un turno cerrado deja de cambiar cuando otro turno cobra. `ExpectedByMethodForSession` y
  `SessionMoneyByMethod` (para cortes cerrados) dan `earlier` = cobros del turno de pedidos de otro
  turno, y el desglose los muestra como «Cobros de otros turnos» separados de «Ventas».

## R11 — El día de cada pago (D12, decisiones del dueño)

- **Decisión**: 0080 agrega `order_payments.business_date` nullable; los dos inserts
  (`CreateOrderPaymentNumbered`, `CreatePlatformOrderPayment`) la llenan con el reloj de la app.
  **0081** la rellena desde `o.business_date` (lo histórico queda en el mismo día que hoy; medido en
  el respaldo del 2026-10-05: 274 pagos). Sigue nullable para que un binario anterior en rollback
  pueda cobrar; las consultas usan `op.business_date between … or (op.business_date is null and
  o.business_date between …)`, que sí puede usar el índice `(company_id, business_date)`.
  Devoluciones: `coalesce(r.business_date, o.business_date)` (solo alcanza a lo que inserte un
  binario anterior).
- Cobros y devoluciones se agregan en **dos CTE por método** y se unen con `full outer join`: un
  método con devoluciones y sin cobros en el periodo también sale (unir las dos 1:N multiplicaría
  filas). El esperado del corte las lleva como subconsultas por método, no como otro join.
- Pedidos cancelados: se incluyen sus cobros **solo si tienen filas en `order_refunds`**, con el
  predicado literal en las dos gemelas. Medido en el respaldo: la empresa 2 tiene 5 cancelados
  anteriores a 0060 con $1,009 cobrados y sin libro; se quedan fuera, con su prueba.
- `SalesTotalsByMethod` (+ gemela) y `SalesByMethod`: cobros por día del pago **menos** devoluciones
  por día de la devolución, incluyendo cancelados que tienen devoluciones (DD-6). Se agrega
  `refunds` a cada fila.
- **Descartado**: `created_at at time zone` en la consulta: la app fecha con su reloj (los tests y la
  zona del negocio) y la base con el suyo; mezclar relojes rompe la regla de un solo día de negocio.
  Índice: `order_payments (company_id, business_date)`.

## R12 — Utilidad por producto (D10, DD-7)

- **Decisión**: `ol.cancelled_at is null` y `revenue = sum(line_total × (total − envío) / subtotal)`;
  margen sobre ese ingreso.

## R13 — Renglones cancelados (D14)

- **Decisión**: `SalesCancelledLines` (+ gemela) cuenta todo renglón cancelado del periodo sin
  importar el estado del pedido. No hay doble conteo: el total del pedido cancelado ya excluye lo
  quitado (`RecalcOrderTotals`).

## R14 — Inventario al cancelar (D11) — verificado abierto en develop

- La 027 hizo que `RestockCancelledOrder` se salte los renglones **quitados**, pero repone los vivos
  enviados a cocina. **Decisión**: `CancelarConDevolucion` repone por renglón vivo con
  `domain.ReponeInventario` (`RestockCancelledLine`, que ya revierte por renglón) y
  `RestockCancelledOrder` queda solo para movimientos sin renglón (anteriores a 0060).

## R15 — Centavo de tolerancia (D16, DD-8)

- **Decisión**: `PagosCubren` exige `pagado ≥ total`. Las divisiones (`SplitPartAmount`,
  `SelectionAmount`) ya cargan el residuo al último pago; la tolerancia ya no tiene a quién servir y
  `UncollectedInSession` ya es exacta.

## R16 — Partir un renglón (D17)

- **Decisión**: `domain.SplitLineTotal(total, unit, k)`: lo movido = `Round2(unit × k)`, lo que se
  queda = `total − movido`. `ListLinesToSplit` trae `line_total`.

## R17 — Total Dif. (D13)

- **Decisión**: `TotalsTable` recibe la diferencia del cajón y la suma al renglón Total, igual que
  `ListSessions`.

## R18 — Recuadro de plataformas (D15) — no se reproduce

- (a) `SalesPage` ya apaga la consulta con cualquier filtro (`tablaAcotada`); falta la prueba que lo
  vigile: se agrega. (b) Exclusión documentada en `settlements.sql` (DD-10). Sin cambio de código.
