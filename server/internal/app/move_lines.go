package app

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"uuid"

	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store/db"
)

// MoveLinesCmd pasa piezas de un pedido a otro abierto o a uno nuevo (ToOrderID nil).
type MoveLinesCmd struct {
	// ClientUUID identifica el lote: el reenvío del mismo «Pasar» no pasa dos veces. Obligatorio.
	ClientUUID  uuid.UUID
	FromOrderID int64
	ToOrderID   *int64
	Lines       []domain.SelectedPieces
	ActorID     int64
}

// errChooseWhatToMove es la selección vacía o malformada de «Pasar»: pasar no es pagar, y el texto
// de cobrar («Elige qué productos paga») ahí no dice nada.
var errChooseWhatToMove = fmt.Errorf("%w: Elige qué productos pasar", domain.ErrValidation)

// MoveLinesResult son los dos pedidos como quedaron.
type MoveLinesResult struct {
	From *OrderView `json:"from"`
	To   *OrderView `json:"to"`
}

// MoveLines pasa productos de un pedido a otro: los renglones SE MUEVEN, no se cancelan ni se
// recapturan.
//
// Es la salida de los casos D y E del incidente del 2026-10-04: se capturó en la cuenta equivocada,
// o alguien quiere su propio pedido. Ese día se resolvió quitando renglones con un motivo falso y
// recapturándolos, y eso dejó 18 cancelaciones que no ocurrieron y el inventario descontado dos
// veces. Aquí lo ya enviado a cocina conserva su estado (no hay comanda nueva), lo entregado viaja
// entregado, y el consumo viaja con su renglón: las existencias no se mueven.
//
// El pedido nuevo hereda del origen su turno, su día, su servicio y quién lo abrió, y no pide «la
// caja abierta»: eso cerraría la puerta de más de una caja. Quien pasó los productos queda en el
// lote. Pasar todo a un pedido existente junta el origen con él; pasar todo a uno nuevo no hace nada
// que el pedido no sea ya, y se rechaza.
func (s *OrdersService) MoveLines(ctx context.Context, cmd MoveLinesCmd) (*MoveLinesResult, error) {
	if cmd.ClientUUID == uuid.Nil() || len(cmd.Lines) == 0 {
		return nil, errChooseWhatToMove
	}
	for _, l := range cmd.Lines {
		if !domain.ValidPieces(l.Qty) {
			return nil, errChooseWhatToMove
		}
	}
	var toID int64
	err := s.store.WithTx(ctx, func(q *db.Queries) error {
		from, to, err := lockMoveOrders(ctx, q, cmd.FromOrderID, cmd.ToOrderID)
		if err != nil {
			return err
		}
		// El lote se consulta bajo los candados: un reenvío concurrente espera al primero y aquí ya
		// lo ve.
		if b, err := q.GetLineMoveBatch(ctx, cmd.ClientUUID); err == nil {
			if b.FromOrderID != cmd.FromOrderID || (cmd.ToOrderID != nil && *cmd.ToOrderID != b.ToOrderID) {
				return domain.ErrMoveKeyMismatch
			}
			moved, err := q.ListMovedLinesOfBatch(ctx, cmd.ClientUUID)
			if err != nil {
				return err
			}
			done := make([]domain.SelectedPieces, len(moved))
			for i, m := range moved {
				done[i] = domain.SelectedPieces{LineID: m.LineID, Qty: m.Qty}
			}
			if domain.CoverageKey(done) != domain.CoverageKey(cmd.Lines) {
				return domain.ErrMoveKeyMismatch
			}
			toID = b.ToOrderID
			return nil
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		if err := canGiveLines(from); err != nil {
			return err
		}
		if to != nil {
			if err := canReceiveLines(from, *to); err != nil {
				return err
			}
		}
		plan, err := planMove(ctx, q, from, cmd.Lines)
		if err != nil {
			return err
		}
		if plan.everything {
			if to == nil {
				return domain.ErrAlreadyItsOwnOrder
			}
			if from.DeliveryFee.IsPositive() {
				return domain.ErrMergeWithShipping
			}
		}
		if to == nil {
			if toID, err = newOrderLike(ctx, q, from, cmd.ClientUUID); err != nil {
				return err
			}
		} else {
			toID = to.ID
		}
		if _, err := q.CreateLineMoveBatch(ctx, db.CreateLineMoveBatchParams{
			ClientUuid: cmd.ClientUUID, FromOrderID: from.ID, ToOrderID: toID, MovedBy: cmd.ActorID,
		}); err != nil {
			return err
		}
		for _, p := range plan.pieces {
			if err := moveOne(ctx, q, p, from.ID, toID, cmd.ClientUUID, cmd.ActorID); err != nil {
				return err
			}
		}
		for _, id := range []int64{from.ID, toID} {
			if err := q.RecalcOrderTotals(ctx, id); err != nil {
				return err
			}
		}
		// No mueve dinero: el origen no puede quedar con más pagado que su total.
		if err := plan.guard.keepsPayments(ctx, q, from.ID); err != nil {
			return err
		}
		// El destino: si estaba entregado y recibió algo pendiente se REABRE, como al agregarle; el
		// tablero solo lista abiertos y listos, y entregado escondería esa comida. Si todo lo que
		// tiene ya salió, se cierra solo.
		lineas, err := lineasDeEntrega(ctx, q, toID)
		if err != nil {
			return err
		}
		if to != nil && domain.ReabreAlAgregar(string(to.Status)) && !domain.TodoEntregado(lineas) {
			if err := q.SetOrderStatus(ctx, db.SetOrderStatusParams{ID: toID, Status: db.OrderStatusAbierta}); err != nil {
				return err
			}
		} else if err := cerrarSiYaSeEntregoTodo(ctx, q, toID, lineas); err != nil {
			return err
		}
		if plan.everything {
			// Sin reponer: el consumo viajó con los productos.
			actor := cmd.ActorID
			n, err := q.MarkOrderMerged(ctx, db.MarkOrderMergedParams{ActorID: &actor, IntoOrderID: &toID, ID: from.ID})
			if err != nil {
				return err
			}
			if n != 1 {
				return fmt.Errorf("%w: Ese pedido ya se cerró", domain.ErrConflict)
			}
			return nil
		}
		lineas, err = lineasDeEntrega(ctx, q, from.ID)
		if err != nil {
			return err
		}
		return cerrarSiYaSeEntregoTodo(ctx, q, from.ID, lineas)
	})
	if err != nil {
		return nil, err
	}
	fromView, err := s.load(ctx, cmd.FromOrderID)
	if err != nil {
		return nil, err
	}
	toView, err := s.load(ctx, toID)
	if err != nil {
		return nil, err
	}
	return &MoveLinesResult{From: fromView, To: toView}, nil
}

// lockMoveOrders bloquea origen y destino en orden ascendente de id: en cualquier otro orden, dos
// «Pasar» cruzados entre los mismos pedidos se interbloquean.
func lockMoveOrders(ctx context.Context, q *db.Queries, fromID int64, toID *int64) (db.GetOrderForMoveRow, *db.GetOrderForMoveRow, error) {
	ids := []int64{fromID}
	if toID != nil && *toID != fromID {
		ids = append(ids, *toID)
	}
	slices.Sort(ids)
	rows := map[int64]db.GetOrderForMoveRow{}
	for _, id := range ids {
		r, err := q.GetOrderForMove(ctx, id)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				if id == fromID {
					return db.GetOrderForMoveRow{}, nil, fmt.Errorf("%w: Ese pedido no existe", domain.ErrNotFound)
				}
				return db.GetOrderForMoveRow{}, nil, domain.ErrMoveTargetClosed
			}
			return db.GetOrderForMoveRow{}, nil, err
		}
		rows[id] = r
	}
	from := rows[fromID]
	if toID == nil {
		return from, nil, nil
	}
	if *toID == fromID {
		return from, nil, domain.ErrMoveTargetClosed
	}
	to := rows[*toID]
	return from, &to, nil
}

// canGiveLines decide si un pedido puede dar productos.
func canGiveLines(o db.GetOrderForMoveRow) error {
	switch {
	case o.DeliveryPlatformID != nil:
		return domain.ErrPlatformOrderNotSplittable
	case o.SessionStatus != string(db.SessionStatusAbierta):
		return domain.ErrOrderFromClosedShiftToMove
	case !domain.PuedeRecibirLineas(string(o.Status)):
		return fmt.Errorf("%w: Ese pedido ya se cerró", domain.ErrConflict)
	case o.DiscountTotal.IsPositive():
		return domain.ErrMoveWithDiscount
	}
	return nil
}

// canReceiveLines decide si un pedido existente puede recibir los productos del origen: abierto o
// listo, del mismo turno y del mismo día, sin plataforma y sin descuento.
func canReceiveLines(from, to db.GetOrderForMoveRow) error {
	switch {
	case to.DeliveryPlatformID != nil:
		return domain.ErrPlatformOrderNotSplittable
	case !domain.PuedeRecibirLineas(string(to.Status)):
		return domain.ErrMoveTargetClosed
	case to.RegisterSessionID == nil || from.RegisterSessionID == nil || *to.RegisterSessionID != *from.RegisterSessionID ||
		!to.BusinessDate.Time.Equal(from.BusinessDate.Time):
		return domain.ErrMoveTargetOtherShift
	case to.DiscountTotal.IsPositive():
		return domain.ErrMoveWithDiscount
	}
	return nil
}

// movePlan son las piezas a pasar ya validadas, y si con ellas el origen se queda sin productos.
type movePlan struct {
	pieces     []movePiece
	everything bool
	guard      paymentGuard
}

type movePiece struct {
	line    db.ListLinesToSplitRow
	k       decimal.Decimal
	covered decimal.Decimal
}

// planMove valida la selección contra los renglones vivos del origen: que existan, que no tengan
// devolución, que se sepa su consumo, que no estén pagadas y que no mezclen lo entregado con lo
// pendiente.
func planMove(ctx context.Context, q *db.Queries, from db.GetOrderForMoveRow, sel []domain.SelectedPieces) (movePlan, error) {
	guard, err := loadPaymentGuard(ctx, q, from.ID)
	if err != nil {
		return movePlan{}, err
	}
	rows, err := q.ListLinesToSplit(ctx, from.ID)
	if err != nil {
		return movePlan{}, err
	}
	refunded, err := q.ListRefundedLinesOfOrder(ctx, from.ID)
	if err != nil {
		return movePlan{}, err
	}
	fromID := from.ID
	legacy, err := q.CountUnattributedSales(ctx, &fromID)
	if err != nil {
		return movePlan{}, err
	}
	want := map[int64]decimal.Decimal{}
	for _, s := range sel {
		want[s.LineID] = want[s.LineID].Add(s.Qty)
	}
	plan := movePlan{guard: guard, everything: true}
	for _, r := range rows {
		k, ok := want[r.ID]
		if !ok {
			plan.everything = false
			continue
		}
		delete(want, r.ID)
		if slices.Contains(refunded, r.ID) {
			return movePlan{}, domain.ErrMoveRefundedLine
		}
		if legacy > 0 {
			return movePlan{}, domain.ErrMoveLegacyLine
		}
		pieces := domain.LinePieces{Qty: r.Quantity, Delivered: r.DeliveredQty, Covered: guard.covered[r.ID]}
		if _, err := domain.SplitLine(pieces, k); err != nil {
			if errors.Is(err, domain.ErrPieceAlreadyPaid) {
				return movePlan{}, domain.ErrPieceAlreadyPaidToMove
			}
			return movePlan{}, err
		}
		if k.LessThan(r.Quantity) {
			plan.everything = false
		}
		plan.pieces = append(plan.pieces, movePiece{line: r, k: k, covered: guard.covered[r.ID]})
	}
	if len(want) > 0 {
		return movePlan{}, fmt.Errorf("%w: Ese producto ya no está en el pedido", domain.ErrNotFound)
	}
	return plan, nil
}

// moveOne pasa un renglón entero o, si son solo algunas piezas, lo parte y pasa la parte nueva.
func moveOne(ctx context.Context, q *db.Queries, p movePiece, fromID, toID int64, key uuid.UUID, actor int64) error {
	if p.k.LessThan(p.line.Quantity) {
		newID, err := splitOrderLine(ctx, q, p.line, p.k, p.covered, fromID, toID, actor)
		if err != nil {
			return err
		}
		orig := p.line.ID
		return q.CreateLineMove(ctx, db.CreateLineMoveParams{ClientUuid: key, OrderLineID: newID, SplitFromLineID: &orig, Qty: p.k})
	}
	if err := q.MoveOrderLineToOrder(ctx, db.MoveOrderLineToOrderParams{ToOrderID: toID, ID: p.line.ID}); err != nil {
		return err
	}
	lineID := p.line.ID
	if err := q.MoveLineStockMovements(ctx, db.MoveLineStockMovementsParams{ToOrderID: &toID, LineID: &lineID}); err != nil {
		return err
	}
	return q.CreateLineMove(ctx, db.CreateLineMoveParams{ClientUuid: key, OrderLineID: lineID, Qty: p.k})
}

// newOrderLike abre el pedido destino con el turno, el día, el servicio, la sucursal y quien abrió
// el origen, y un folio de ese turno. La llave del lote es la del pedido: un reenvío no abre otro.
func newOrderLike(ctx context.Context, q *db.Queries, from db.GetOrderForMoveRow, key uuid.UUID) (int64, error) {
	num, err := q.NextFolioNumber(ctx, *from.RegisterSessionID)
	if err != nil {
		return 0, err
	}
	name, err := resolverFolio(ctx, q, CreateOrderCmd{}, *from.RegisterSessionID)
	if err != nil {
		return 0, err
	}
	branch := from.BranchID
	o, err := q.CreateOrder(ctx, db.CreateOrderParams{
		ClientUuid: key, BusinessDate: from.BusinessDate, DailyNumber: num, ServiceType: from.ServiceType,
		RegisterSessionID: from.RegisterSessionID, OpenedBy: from.OpenedBy,
		Subtotal: decimal.Zero, Total: decimal.Zero, DeliveryFee: decimal.Zero,
		FolioName: strPtr(name), Status: db.OrderStatusAbierta,
		DiscountTotal: decimal.Zero, DiscountSetBy: from.OpenedBy, BranchID: &branch,
	})
	if err != nil {
		return 0, err
	}
	return o.ID, nil
}
