package app

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store/db"
)

// Devolver dinero: la operación que el sistema no tenía.
//
// Tenía dos que decían que una venta no ocurrió —cancelar y reembolsar— y ninguna miraba lo que el
// cliente ya había pagado. Cancelar un pedido cobrado lo sacaba de los reportes y dejaba los cobros
// en la base, con el arqueo esperando ese dinero en el cajón; reembolsar uno entregado sin cobrar
// anotaba como pérdida un ingreso que nunca ocurrió.

// DevolucionCmd: qué se devuelve y de qué pedido.
//
// `LineID` nulo = contra la cuenta entera. Con renglón, el tope es lo cobrado de ESE renglón: sin esa
// cota, devolver tres veces un platillo de $60 en un pedido de $500 pasa sin que nada lo frene.
type DevolucionCmd struct {
	OrderID int64
	LineID  *int64
	Monto   decimal.Decimal
	Motivo  string
	ActorID int64
}

// CancelacionCmd: cancelar un pedido, resolviendo su dinero si lo tiene.
type CancelacionCmd struct {
	OrderID int64
	Motivo  string
	ActorID int64
	// Devolver: el cajero confirma que el dinero se le regresa al cliente. Sin esto, un pedido con
	// cobros NO se cancela — es el agujero que esta feature cierra.
	Devolver bool
}

// Devolver registra una devolución y, si el dinero salió del cajón, su movimiento de caja.
//
// Todo en UNA transacción: un pedido marcado como devuelto sin su salida de caja, o al revés, es
// justo el descuadre que esto viene a eliminar.
func (s *OrdersService) Devolver(ctx context.Context, cmd DevolucionCmd) error {
	motivo, err := domain.MotivoValido(cmd.Motivo)
	if err != nil {
		return err
	}
	return s.store.WithTx(ctx, func(q *db.Queries) error {
		return s.devolverEnTx(ctx, q, cmd, motivo)
	})
}

// devolverEnTx es el cuerpo compartido por Devolver y por CancelarConDevolucion: cancelar con
// devolución tiene que ir en la MISMA transacción que la cancelación, y dos copias de esta lógica
// serían dos formas distintas de mover el mismo dinero.
func (s *OrdersService) devolverEnTx(ctx context.Context, q *db.Queries, cmd DevolucionCmd, motivo string) error {
	// El pedido BLOQUEADO antes de leer lo cobrado y lo devuelto, igual que cobrar, cancelar y
	// devolver un pago. Sin el candado, dos devoluciones a la vez —dos tabletas, un doble toque con
	// la red lenta— leían «nada devuelto» las dos y pasaban las dos el tope (spec 031, D3). Dentro de
	// CancelarConDevolucion el pedido ya está bloqueado y volver a pedirlo es un no-op.
	o, err := q.GetOrderForUpdate(ctx, cmd.OrderID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrNotFound
		}
		return err
	}
	if o.Status == db.OrderStatusReembolsada {
		return domain.ErrRefundOnRefundedOrder
	}
	tope, err := s.refundable(ctx, q, cmd.OrderID, cmd.LineID)
	if err != nil {
		return err
	}
	// El renglón se mira solo si el PEDIDO todavía tiene algo devolvible: sin cobros, o con todo
	// devuelto, lo cierto es eso y lo dice ValidarDevolucion; decir «de ese producto ya no queda
	// nada» sería falso de un producto que nunca se devolvió.
	if cmd.LineID != nil && domain.MontoDevolvible(tope.paid, tope.refundedTotal).IsPositive() {
		if err := domain.ValidateLineRefund(cmd.Monto, tope.remaining); err != nil {
			return err
		}
	}
	if err := domain.ValidarDevolucion(cmd.Monto, tope.paid, tope.refundedTotal); err != nil {
		return err
	}
	entradas := tope.entries

	// El dinero sale por donde entró, y sale del cajón lo que estaba en el cajón. Devolver en
	// efectivo lo que entró por tarjeta saca de la caja dinero que nunca estuvo ahí, y el arqueo
	// cierra con un faltante inventado; no registrar la salida del efectivo de una app hace lo
	// mismo con el signo contrario.
	if err := s.recordRefundParts(ctx, q, cmd.OrderID, cmd.LineID, domain.RepartirDevolucion(entradas, cmd.Monto), motivo, cmd.ActorID); err != nil {
		return err
	}

	// `orders.refund_amount` pasa a ser la SUMA del libro, no un número que se escribe aparte:
	// `RefundsByDay` ya lo lee y dos verdades sobre el mismo dinero es lo que el principio III
	// prohíbe.
	return q.RecalcOrderRefundAmount(ctx, cmd.OrderID)
}

// recordRefundParts escribe en el libro cada parte de una devolución, con su turno y su día, y la
// salida de caja de lo que estaba en el cajón. Lo comparten devolver y cancelar con devolución.
//
// El turno es el de AHORA, no el del pedido (decisión del dueño, spec 031): una devolución cuenta
// en el turno y el día en que se hizo. Se lee con candado compartido también cuando nada sale del
// cajón, para que un cierre simultáneo la espere o ella lo vea cerrado.
func (s *OrdersService) recordRefundParts(ctx context.Context, q *db.Queries, orderID int64, lineID *int64,
	partes []domain.ParteDeDevolucion, motivo string, actor int64,
) error {
	var turno *int64
	sess, err := q.LockOpenPrimarySession(ctx)
	switch {
	case err == nil:
		turno = &sess.ID
	case !errors.Is(err, pgx.ErrNoRows):
		return err
	}
	dia := pgtype.Date{Time: domain.BusinessDate(s.now(), s.location(ctx)), Valid: true}
	for _, parte := range partes {
		var movimiento *int64
		if parte.SaleDelCajon {
			// Sin turno NO se devuelve efectivo: la salida no quedaría en ningún arqueo, y la
			// apertura siguiente contaría su fondo ya sin esos billetes sin que nadie supiera por qué
			// (D7). Lo que no toca el cajón sí se registra: lo reclama el turno que se abra.
			if turno == nil {
				return domain.ErrCashRefundNeedsOpenRegister
			}
			mov, err := q.InsertCashMovement(ctx, db.InsertCashMovementParams{
				SessionID: *turno,
				Kind:      "salida",
				Amount:    domain.Round2(parte.Monto.Add(parte.Tip)),
				Concept:   fmt.Sprintf("Devolución: %s", motivo),
				UserID:    actor,
			})
			if err != nil {
				return err
			}
			movimiento = &mov.ID
		}
		if _, err := q.InsertOrderRefund(ctx, db.InsertOrderRefundParams{
			OrderID:           orderID,
			OrderLineID:       lineID,
			PaymentMethodID:   parte.MetodoID,
			Amount:            domain.Round2(parte.Monto),
			TipAmount:         domain.Round2(parte.Tip),
			Reason:            motivo,
			RefundedBy:        actor,
			CashMovementID:    movimiento,
			RegisterSessionID: turno,
			BusinessDate:      dia,
		}); err != nil {
			return err
		}
	}
	return nil
}

// cobradoPorMetodo traduce lo que la base sabe al tipo del dominio, que es donde vive la regla.
func (s *OrdersService) cobradoPorMetodo(ctx context.Context, q *db.Queries, orderID int64) ([]domain.CobradoPorMetodo, error) {
	filas, err := q.SumOrderPaymentsByMethod(ctx, orderID)
	if err != nil {
		return nil, err
	}
	entradas := make([]domain.CobradoPorMetodo, 0, len(filas))
	for _, f := range filas {
		entradas = append(entradas, domain.CobradoPorMetodo{
			MetodoID:    f.MethodID,
			Nombre:      f.Name,
			TocaElCajon: f.TocaElCajon,
			Activo:      f.IsActive,
			Monto:       f.Cobrado,
			Refunded:    f.Refunded,
			Tip:         f.Tip,

			TipRefunded: f.TipRefunded,
		})
	}
	return entradas, nil
}

// refundTop es lo que una devolución puede sacar de un pedido, y de dónde.
type refundTop struct {
	entries       []domain.CobradoPorMetodo
	paid          decimal.Decimal
	refundedTotal decimal.Decimal
	// remaining: lo que todavía se puede devolver; con renglón, el menor entre el pedido y el renglón.
	remaining decimal.Decimal
}

// refundable calcula el tope de una devolución. Lo comparten Devolver —con el pedido bloqueado— y
// PorDevolver, que contesta «cuánto queda» sin monto: dos reglas distintas para la misma cifra son
// dos respuestas distintas a la misma pregunta.
func (s *OrdersService) refundable(ctx context.Context, q *db.Queries, orderID int64, lineID *int64) (refundTop, error) {
	entradas, err := s.cobradoPorMetodo(ctx, q, orderID)
	if err != nil {
		return refundTop{}, err
	}
	var t refundTop
	t.entries = entradas
	for _, e := range entradas {
		t.paid = t.paid.Add(e.Monto)
	}
	devuelto, err := q.SumOrderRefunds(ctx, db.SumOrderRefundsParams{OrderID: orderID, LineID: lineID})
	if err != nil {
		return refundTop{}, err
	}
	t.refundedTotal = devuelto.DevueltoTotal
	t.remaining = domain.MontoDevolvible(t.paid, t.refundedTotal)
	if lineID != nil {
		importe, err := q.GetOrderLineForRefund(ctx, db.GetOrderLineForRefundParams{LineID: *lineID, OrderID: orderID})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return refundTop{}, fmt.Errorf("%w: ese producto no es de este pedido", domain.ErrNotFound)
			}
			return refundTop{}, err
		}
		t.remaining = domain.LineRefundable(t.paid, t.refundedTotal, importe, devuelto.DevueltoDelRenglon)
	}
	return t, nil
}

// CancelarConDevolucion cancela un pedido resolviendo su dinero en la MISMA transacción.
//
// `Cancel` ignoraba los cobros: respondía 204, la venta salía de los reportes, los renglones de
// `order_payments` se quedaban intactos y el arqueo seguía esperando ese dinero en el cajón. Si se
// le devolvía al cliente, el corte cerraba con un faltante que ningún renglón explicaba; si no, el
// negocio se quedaba con dinero que no aparecía en ninguna venta.
//
// Una transacción y no dos llamadas: en el hueco entre "cancelado" y "devuelto" vive exactamente el
// descuadre que esto elimina.
func (s *OrdersService) CancelarConDevolucion(ctx context.Context, cmd CancelacionCmd) error {
	motivo, err := domain.MotivoValido(cmd.Motivo)
	if err != nil {
		return err
	}
	return s.store.WithTx(ctx, func(q *db.Queries) error {
		// Con candado, como CancelarRenglon: la reposición neta lee lo que ya repuso un renglón, y
		// una cancelación de renglón concurrente no debe colarse entre esa lectura y la escritura.
		o, err := q.GetOrderForUpdate(ctx, cmd.OrderID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return domain.ErrNotFound
			}
			return err
		}
		if !domain.CanTransition(string(o.Status), domain.StatusCancelada) {
			return domain.ErrConflict
		}
		lineas, err := lineasDeEntrega(ctx, q, cmd.OrderID)
		if err != nil {
			return err
		}
		if domain.HayEntregaParcial(lineas) {
			return domain.ErrCancelarConEntregas
		}

		entradas, err := s.cobradoPorMetodo(ctx, q, cmd.OrderID)
		if err != nil {
			return err
		}
		// Lo que queda por devolver de CADA medio, cuenta y propina: cancelar dice que la venta no
		// ocurrió, así que el cliente recibe lo que dio (spec 031, D9). Lo ya devuelto antes no se
		// devuelve otra vez.
		partes := domain.SplitCancellationRefund(entradas)
		if len(entradas) > 0 && !cmd.Devolver {
			return domain.ErrCancelarSinDevolver
		}
		if len(partes) > 0 {
			if err := s.recordRefundParts(ctx, q, cmd.OrderID, nil, partes, motivo, cmd.ActorID); err != nil {
				return err
			}
			if err := q.RecalcOrderRefundAmount(ctx, cmd.OrderID); err != nil {
				return err
			}
		}

		if err := q.CancelOrder(ctx, db.CancelOrderParams{
			ID: cmd.OrderID, CancelledBy: &cmd.ActorID, CancelReason: &motivo,
		}); err != nil {
			return err
		}
		// Cada renglón vivo con la MISMA regla que quitarlo: lo que se prepara y ya salió a cocina se
		// consumió y no vuelve (spec 031, D11). Lo que no consta de qué renglón salió —anterior a
		// 0060— se repone por pedido, como siempre.
		renglones, err := q.ListLinesToSplit(ctx, cmd.OrderID)
		if err != nil {
			return err
		}
		for _, l := range renglones {
			if !domain.ReponeInventario(l.NeedsPrep, nullTime(l.EnviadoACocinaAt)) {
				continue
			}
			lineID := l.ID
			if err := q.RestockCancelledLine(ctx, db.RestockCancelledLineParams{LineID: &lineID, ActorID: &cmd.ActorID}); err != nil {
				return err
			}
		}
		return q.RestockCancelledOrder(ctx, db.RestockCancelledOrderParams{Oid: &cmd.OrderID, ActorID: &cmd.ActorID})
	})
}

// CancelarRenglon quita todas las piezas pendientes de UN renglón de un pedido vivo.
//
// Existía la columna y no la operación: ninguna consulta escribía `order_lines.cancelled_at`,
// mientras el error de cancelar un pedido con entregas parciales mandaba al operador a "cancela los
// que falten" — que no se podía hacer desde ningún lado. La única salida practicable era marcar como
// entregado lo que seguía en la plancha.
//
// Devuelve si repuso el inventario, porque la pantalla tiene que poder decirlo: cancelar algo que ya
// salió a cocina baja el total pero NO devuelve el insumo, y callarlo descuadra el almacén sin que
// nadie sepa por qué.
func (s *OrdersService) CancelarRenglon(ctx context.Context, orderID, lineID, actor int64, motivo string) (repuso bool, err error) {
	return s.RemovePieces(ctx, orderID, lineID, actor, motivo, nil)
}

// RemovePieces quita `qty` piezas pendientes de un renglón; nil quita todas las pendientes.
//
// Quitar menos de las que tiene el renglón lo PARTE: las piezas quitadas se van a un renglón nuevo
// que se cancela, con su parte del inventario, y lo demás se queda. Antes el bote quitaba el renglón
// entero: de dos frappés no había forma de quitar uno.
func (s *OrdersService) RemovePieces(ctx context.Context, orderID, lineID, actor int64, motivo string, qty *decimal.Decimal) (repuso bool, err error) {
	razon, err := domain.MotivoValido(motivo)
	if err != nil {
		return false, err
	}
	if qty != nil && !domain.ValidPieces(*qty) {
		return false, fmt.Errorf("%w: Esa cantidad de piezas no se puede quitar", domain.ErrValidation)
	}
	err = s.store.WithTx(ctx, func(q *db.Queries) error {
		// Pedido y luego renglones, en el MISMO orden que DeliverLine. Tomando solo el renglón, una
		// entrega y una cancelación simultáneas se cruzaban: la entrega tenía el pedido y esperaba
		// este renglón, y esto tenía el renglón y esperaba al pedido en RecalcOrderTotals —
		// interbloqueo, y Postgres mataba una de las dos (500). Y sin serializarse, cada una veía al
		// otro renglón pendiente y ninguna cerraba el pedido.
		if _, err := q.GetOrderForUpdate(ctx, orderID); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return domain.ErrNotFound
			}
			return err
		}
		l, err := q.GetOrderLineForCancel(ctx, db.GetOrderLineForCancelParams{ID: lineID, OrderID: orderID})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return domain.ErrNotFound
			}
			return err
		}
		if l.CancelledAt.Valid {
			// Ya estaba cancelado: no-op idempotente. Un doble tap no puede reponer dos veces el
			// mismo insumo, que es inventar existencias.
			return nil
		}
		// Lo entregado se queda: solo se quitan piezas pendientes, y un renglón ya entregado entero
		// no tiene ninguna.
		pending := l.Quantity.Sub(l.DeliveredQty)
		if !pending.IsPositive() {
			return domain.ErrRenglonYaEntregado
		}
		if err := domain.PuedeCancelarRenglon(string(l.OrderStatus), pending, decimal.Zero); err != nil {
			return err
		}
		k := pending
		if qty != nil {
			k = *qty
		}
		if k.GreaterThan(pending) {
			return domain.ErrTooManyPieces
		}
		guard, err := loadPaymentGuard(ctx, q, orderID)
		if err != nil {
			return err
		}
		if err := guard.canRemove(lineID, l.Quantity, l.DeliveredQty, k); err != nil {
			return err
		}
		removed := lineID
		if k.LessThan(l.Quantity) {
			rows, err := q.ListLinesToSplit(ctx, orderID)
			if err != nil {
				return err
			}
			i := slices.IndexFunc(rows, func(r db.ListLinesToSplitRow) bool { return r.ID == lineID })
			if i < 0 {
				return domain.ErrNotFound
			}
			if removed, err = splitOrderLine(ctx, q, rows[i], k, guard.covered[lineID], orderID, orderID, actor); err != nil {
				return err
			}
		}
		if err := q.CancelOrderLine(ctx, db.CancelOrderLineParams{
			ID: removed, CancelledBy: &actor, CancelReason: &razon,
		}); err != nil {
			return err
		}
		// La regla la decide el dominio con lo que la base ya sabe, no el cajero.
		repuso = domain.ReponeInventario(l.NeedsPrep, nullTime(l.EnviadoACocinaAt))
		if repuso {
			if err := q.RestockCancelledLine(ctx, db.RestockCancelledLineParams{
				LineID: &removed, ActorID: &actor,
			}); err != nil {
				return err
			}
		}
		// El total baja igual, haya repuesto o no: el cliente no paga lo que se canceló.
		if err := q.RecalcOrderTotals(ctx, orderID); err != nil {
			return err
		}
		if err := guard.keepsPayments(ctx, q, orderID); err != nil {
			return err
		}
		// Cancelar lo último que faltaba también termina el pedido, igual que entregarlo. Sin esto
		// un pedido con todo lo vivo entregado se quedaba abierto para siempre: el tablero ya no
		// ofrecía entregarlo, cancelarlo completo rebota porque soltó comida, y bloqueaba el corte.
		lineas, err := lineasDeEntrega(ctx, q, orderID)
		if err != nil {
			return err
		}
		return cerrarSiYaSeEntregoTodo(ctx, q, orderID, lineas)
	})
	return repuso, err
}

// paymentGuard es la ÚNICA validación de pagos al quitar productos, la de quitar uno y la de quitar
// lo que falta. Una copia en cada camino es como nace el camino nuevo que se salta el control viejo.
type paymentGuard struct {
	// covered son las piezas de cada renglón vivo que cubren pagos vivos.
	covered map[int64]decimal.Decimal
	paid    decimal.Decimal
}

func loadPaymentGuard(ctx context.Context, q *db.Queries, orderID int64) (paymentGuard, error) {
	rows, err := q.ListLinesForSelection(ctx, orderID)
	if err != nil {
		return paymentGuard{}, err
	}
	g := paymentGuard{covered: map[int64]decimal.Decimal{}}
	for _, r := range rows {
		g.covered[r.ID] = r.CoveredQty
	}
	sums, err := q.SumOrderPayments(ctx, orderID)
	if err != nil {
		return paymentGuard{}, err
	}
	g.paid = sums.Pagado
	return g, nil
}

// canRemove rechaza quitar k piezas de un renglón si alguna está pagada: lo pagado se queda en el
// renglón, así que solo salen las que ningún pago cubre.
func (g paymentGuard) canRemove(lineID int64, qty, delivered, k decimal.Decimal) error {
	if k.GreaterThan(domain.MovablePieces(domain.LinePieces{Qty: qty, Delivered: delivered, Covered: g.covered[lineID]})) {
		return domain.ErrPieceAlreadyPaidToRemove
	}
	return nil
}

// keepsPayments rechaza, ya recalculado el total, dejarlo por debajo de lo cobrado.
func (g paymentGuard) keepsPayments(ctx context.Context, q *db.Queries, orderID int64) error {
	o, err := q.GetOrder(ctx, orderID)
	if err != nil {
		return err
	}
	return domain.RemovalKeepsPayments(o.Total, g.paid)
}

// VoidResult es lo que queda del pedido después de devolver un pago.
type VoidResult struct {
	Outstanding decimal.Decimal `json:"outstanding"`
	Paid        bool            `json:"paid"`
}

// VoidPayment devuelve un pago de un turno abierto: lo SACA de order_payments a la bitácora.
//
// Se saca y no se marca: las ~32 consultas que suman pagos —corte por método, propinas, ventas por
// método— quedan correctas sin tocarlas, porque para el cajón un pago devuelto en su mismo turno es
// un pago que no ocurrió. La bitácora guarda la copia completa, con el número que el ticket impreso
// lleva, y lo que cubrió vuelve a quedar por cobrar.
//
// Solo de un turno abierto: el de un turno cerrado ya se arqueó, y devolverlo aquí movería un corte
// firmado. Ese va por la devolución de siempre.
func (s *OrdersService) VoidPayment(ctx context.Context, orderID, paymentID, actor int64, reason string) (VoidResult, error) {
	why := domain.MotivoLimpio(reason)
	if why == "" {
		return VoidResult{}, fmt.Errorf("%w: Elige por qué se devuelve", domain.ErrValidation)
	}
	var res VoidResult
	err := s.store.WithTx(ctx, func(q *db.Queries) error {
		// El candado del pedido ANTES de leer el pago, el mismo de Charge. Sin él, un reintento del
		// cobro con la misma llave podía ver el pago, perderlo por este borrado y volver a
		// insertarlo: el pago revivía.
		o, err := q.GetOrderForUpdate(ctx, orderID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return fmt.Errorf("%w: Ese pedido no existe", domain.ErrNotFound)
			}
			return err
		}
		if o.Status == db.OrderStatusCancelada || o.Status == db.OrderStatusReembolsada {
			return domain.ErrOrderClosedForVoid
		}
		p, err := q.GetOrderPaymentForVoid(ctx, paymentID)
		if errors.Is(err, pgx.ErrNoRows) {
			if v, verr := q.GetPaymentVoidByOriginalID(ctx, paymentID); verr == nil && v == orderID {
				return domain.ErrPaymentAlreadyVoided
			}
			return fmt.Errorf("%w: Ese pago no es de este pedido", domain.ErrNotFound)
		}
		if err != nil {
			return err
		}
		if p.OrderID != orderID {
			return fmt.Errorf("%w: Ese pago no es de este pedido", domain.ErrNotFound)
		}
		if p.RegisterSessionID == nil {
			return domain.ErrPaymentFromClosedShift
		}
		// El turno se lee con candado compartido y no del join de arriba: un cierre que corre a la
		// vez termina antes y aquí se ve cerrado (el mismo hueco que cierra Charge al bloquear la caja).
		status, err := q.LockSessionStatusForShare(ctx, *p.RegisterSessionID)
		if err != nil {
			return err
		}
		if status != string(db.SessionStatusAbierta) {
			return domain.ErrPaymentFromClosedShift
		}
		// Lo devuelto por el medio de este pago tiene que seguir cubierto sin él. Si no, el cliente
		// recibe el pago completo ADEMÁS de lo ya devuelto y el pedido vuelve a deber (spec 031, D2).
		entradas, err := s.cobradoPorMetodo(ctx, q, orderID)
		if err != nil {
			return err
		}
		for _, e := range entradas {
			if e.MetodoID == p.PaymentMethodID {
				if err := domain.VoidKeepsRefunds(e.Monto.Sub(p.Amount), e.Refunded); err != nil {
					return err
				}
			}
		}
		// El número que la vista le daba, también si es de los viejos sin número.
		number := deref16(p.PaymentNumber)
		if number == 0 {
			views, err := paymentsOf(ctx, q, orderID)
			if err != nil {
				return err
			}
			for _, v := range views {
				if v.ID == p.ID && !v.Voided {
					number = int16(v.Number)
				}
			}
		}
		if err := q.CreateOrderPaymentVoid(ctx, db.CreateOrderPaymentVoidParams{
			OrderID: orderID, OriginalPaymentID: p.ID, PaymentNumber: number, PaymentMethodID: p.PaymentMethodID,
			Amount: p.Amount, TipAmount: p.TipAmount, Reference: p.Reference, RegisterSessionID: *p.RegisterSessionID,
			ReceivedBy: p.ReceivedBy, PaidAt: p.CreatedAt, ClientUuid: p.ClientUuid, SplitPart: p.SplitPart, SplitOf: p.SplitOf,
			Covered: p.Covered, VoidedBy: actor, Reason: why,
		}); err != nil {
			return err
		}
		if err := q.DeleteOrderPayment(ctx, p.ID); err != nil {
			return err
		}
		sums, err := q.SumOrderPayments(ctx, orderID)
		if err != nil {
			return err
		}
		res = VoidResult{Outstanding: domain.PorCobrar(o.Total, sums.Pagado), Paid: domain.PedidoSaldado(sums.Pagado, o.Total)}
		return nil
	})
	return res, err
}

// nullTime traduce el timestamptz opcional de pgx a lo que el dominio entiende.
func nullTime(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	return &t.Time
}

// PorDevolver dice cuánto queda por devolver de un pedido, o de uno de sus renglones.
//
// Lo usa la frontera cuando no se manda monto, que es el caso de todos los días: "devuélvele todo".
// Se calcula aquí y no en la pantalla porque el tope es lo COBRADO menos lo ya devuelto, y la
// pantalla no tiene esas dos cifras sin pedirlas — y si las pidiera, quedarían viejas entre la
// consulta y el toque.
func (s *OrdersService) PorDevolver(ctx context.Context, orderID int64, lineID *int64) (decimal.Decimal, error) {
	t, err := s.refundable(ctx, s.store.QC(ctx), orderID, lineID)
	if err != nil {
		return decimal.Zero, err
	}
	return t.remaining, nil
}

// CancelPendingResult dice cuántos productos se quitaron y cuántos repusieron inventario.
type CancelPendingResult struct {
	Removed   int `json:"removed"`
	Restocked int `json:"restocked"`
}

// emptyOrderReason es el motivo fijo de cancelar un pedido que ya no tiene productos: no se le
// pregunta a nadie porque no hay nada que decidir.
const emptyOrderReason = "Sin productos"

// CancelPending quita de un pedido todo lo que falta por entregar, en una transacción.
//
// Es la salida de un pedido con algo entregado: cancelarlo completo rebota porque soltó comida, y
// quitar renglón por renglón un pedido de once productos es justo lo que no se hace con el cliente
// enfrente. Un renglón con entrega parcial se PARTE: lo pendiente se va a un renglón nuevo que se
// quita, y lo entregado se queda.
//
// Sobre un pedido sin productos vivos y sin pagos lo cancela con un motivo fijo; ahí el motivo que
// mande la pantalla se ignora («Cerrar pedido» no pregunta nada).
func (s *OrdersService) CancelPending(ctx context.Context, orderID, actor int64, reason string) (CancelPendingResult, error) {
	var res CancelPendingResult
	err := s.store.WithTx(ctx, func(q *db.Queries) error {
		// Pedido y luego renglones, en el mismo orden que CancelarRenglon y DeliverLine: en otro
		// orden, una entrega simultánea se interbloquea con esto.
		o, err := q.GetOrderForUpdate(ctx, orderID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return fmt.Errorf("%w: Ese pedido no existe", domain.ErrNotFound)
			}
			return err
		}
		if !domain.OrderNotVoided(string(o.Status)) {
			return fmt.Errorf("%w: Ese pedido ya se cerró", domain.ErrConflict)
		}
		lineas, err := lineasDeEntrega(ctx, q, orderID)
		if err != nil {
			return err
		}
		guard, err := loadPaymentGuard(ctx, q, orderID)
		if err != nil {
			return err
		}
		// Lo que queda por devolver, no lo cobrado en bruto: un pedido vacío con su pago ya devuelto
		// no tiene nada que devolver, y con el bruto se quedaba sin salida.
		tope, err := s.refundable(ctx, q, orderID, nil)
		if err != nil {
			return err
		}
		closeEmpty, err := domain.PlanCancelPending(lineas, domain.MontoDevolvible(tope.paid, tope.refundedTotal))
		if err != nil {
			return err
		}
		if closeEmpty {
			// Una entregada a la que se le quitó todo después: ya salió y ya contó como venta.
			if !domain.CanTransition(string(o.Status), domain.StatusCancelada) {
				return fmt.Errorf("%w: Ese pedido ya se cerró", domain.ErrConflict)
			}
			// Sin reponer: cada renglón ya resolvió su inventario al quitarse, y reponer aquí lo
			// haría dos veces.
			motivo := emptyOrderReason
			return q.CancelOrder(ctx, db.CancelOrderParams{ID: orderID, CancelledBy: &actor, CancelReason: &motivo})
		}

		razon, err := domain.ReasonToRemove(reason)
		if err != nil {
			return err
		}
		renglones, err := q.ListLinesToSplit(ctx, orderID)
		if err != nil {
			return err
		}
		for _, l := range renglones {
			pending := l.Quantity.Sub(l.DeliveredQty)
			if !pending.IsPositive() {
				continue
			}
			if err := guard.canRemove(l.ID, l.Quantity, l.DeliveredQty, pending); err != nil {
				return err
			}
			lineID := l.ID
			if l.DeliveredQty.IsPositive() {
				if lineID, err = splitOrderLine(ctx, q, l, pending, guard.covered[l.ID], orderID, orderID, actor); err != nil {
					return err
				}
			}
			if err := q.CancelOrderLine(ctx, db.CancelOrderLineParams{
				ID: lineID, CancelledBy: &actor, CancelReason: &razon,
			}); err != nil {
				return err
			}
			res.Removed++
			if domain.ReponeInventario(l.NeedsPrep, nullTime(l.EnviadoACocinaAt)) {
				if err := q.RestockCancelledLine(ctx, db.RestockCancelledLineParams{LineID: &lineID, ActorID: &actor}); err != nil {
					return err
				}
				res.Restocked++
			}
		}
		if err := q.RecalcOrderTotals(ctx, orderID); err != nil {
			return err
		}
		if err := guard.keepsPayments(ctx, q, orderID); err != nil {
			return err
		}
		lineas, err = lineasDeEntrega(ctx, q, orderID)
		if err != nil {
			return err
		}
		return cerrarSiYaSeEntregoTodo(ctx, q, orderID, lineas)
	})
	return res, err
}

// splitOrderLine saca k piezas de un renglón del pedido fromOrderID a uno nuevo del pedido
// toOrderID, y devuelve su id.
//
// El inventario se parte INSERTANDO un par de movimientos «renglón partido» por cada venta del
// original —lo que se lleva el nuevo sale del original y entra al nuevo—, nunca editando las
// cantidades: el trigger de existencias solo corre al insertar, y cada par se anula, así que partir
// no mueve el almacén. Cada renglón queda con los movimientos de sus piezas, que es lo que
// RestockCancelledLine revierte.
func splitOrderLine(ctx context.Context, q *db.Queries, l db.ListLinesToSplitRow, k, covered decimal.Decimal, fromOrderID, toOrderID, actor int64) (int64, error) {
	parts, err := domain.SplitLine(domain.LinePieces{Qty: l.Quantity, Delivered: l.DeliveredQty, Covered: covered}, k)
	if err != nil {
		return 0, err
	}
	unit := l.UnitPrice.Add(l.ModifiersTotal)
	// Lo que se queda es el RESTO del importe, no otro redondeo: redondear las dos partes por su lado
	// inventaba un centavo (spec 031, D17).
	keepTotal, moveTotal := domain.SplitLineTotal(l.LineTotal, unit, parts.Move.Qty)
	newID, err := q.SplitOffOrderLine(ctx, db.SplitOffOrderLineParams{
		OrderID: toOrderID, LineID: l.ID, Quantity: parts.Move.Qty, DeliveredQty: parts.Move.Delivered,
		LineTotal: moveTotal,
	})
	if err != nil {
		return 0, err
	}
	if err := q.CopyOrderLineModifiers(ctx, db.CopyOrderLineModifiersParams{NewLineID: newID, LineID: l.ID}); err != nil {
		return 0, err
	}
	if err := q.ShrinkOrderLine(ctx, db.ShrinkOrderLineParams{
		ID: l.ID, Quantity: parts.Keep.Qty, DeliveredQty: parts.Keep.Delivered,
		LineTotal: keepTotal,
	}); err != nil {
		return 0, err
	}
	// Lo que lleva el paquete se reparte con la misma regla que los movimientos: sin esto el renglón
	// nuevo sale como un paquete vacío y el original sigue diciendo que lleva todo.
	comps, err := q.ListLineComponents(ctx, l.ID)
	if err != nil {
		return 0, err
	}
	for _, c := range comps {
		keep, move := domain.SplitMovement(c.Quantity, l.Quantity, k)
		if !move.IsPositive() {
			continue
		}
		if err := q.SetLineComponentQty(ctx, db.SetLineComponentQtyParams{ID: c.ID, Quantity: keep}); err != nil {
			return 0, err
		}
		if err := q.InsertOrderLineComponent(ctx, db.InsertOrderLineComponentParams{
			OrderLineID: newID, ProductID: c.ProductID, Quantity: move,
		}); err != nil {
			return 0, err
		}
	}
	origID := l.ID
	movs, err := q.ListLineSaleMovements(ctx, &origID)
	if err != nil {
		return 0, err
	}
	reason := splitLineReason
	for _, m := range movs {
		_, move := domain.SplitMovement(m.Quantity, l.Quantity, k)
		if move.IsZero() {
			continue
		}
		for _, half := range []struct {
			line, order int64
			qty         decimal.Decimal
		}{{origID, fromOrderID, move.Neg()}, {newID, toOrderID, move}} {
			if err := q.InsertStockMovement(ctx, db.InsertStockMovementParams{
				ItemType: m.ItemType, IngredientID: m.IngredientID, ProductID: m.ProductID,
				MovementType: db.StockMovementTypeVenta, Quantity: half.qty, UnitCost: m.UnitCost,
				OrderID: &half.order, OrderLineID: &half.line, UserID: &actor, Reason: &reason,
				ModifierOptionID: m.ModifierOptionID, ComponentOfProductID: m.ComponentOfProductID,
			}); err != nil {
				return 0, err
			}
		}
	}
	return newID, nil
}

// splitLineReason marca en el kárdex los pares de movimientos de un renglón partido, para que se
// distingan de una venta.
const splitLineReason = "renglón partido"
