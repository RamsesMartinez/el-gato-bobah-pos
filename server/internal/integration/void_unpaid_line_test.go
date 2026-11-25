//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"

	"uuid"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
)

// A PRODUCT ADDED BY MISTAKE TO AN ALREADY-PAID ORDER CAN BE REMOVED EVEN IF IT SHOWS AS DELIVERED.
//
// Delivering the whole order marks the new line as delivered although it was never served from that
// order. Cancelling it is rejected (already delivered) and a refund does not apply (no money was
// taken for it), so the order kept a pending balance forever.
func TestADeliveredLineWithNoPaymentCanBeVoided(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()
	svc := app.NewOrdersService(st, clock)

	cashier := makeUser(t, st, "cashier_void_unpaid", "cajero")
	cash := paymentMethodID(t, st, "Efectivo")
	abrirCajaPrincipal(t, st, cashier)

	paidProduct := makeProduct(t, st, "Producto cobrado", pesos("80"), false)
	addedProduct := makeProduct(t, st, "Producto agregado por error", pesos("69"), false)

	ord, err := svc.Create(ctx, app.CreateOrderCmd{
		ClientUUID: uuid.New(), ServiceType: "mostrador", OpenedBy: cashier,
		Lines: []domain.OrderLineInput{{ProductID: paidProduct, Qty: pesos("1")}},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := svc.Charge(ctx, app.ChargeCmd{OrderID: ord.ID, MethodID: cash, Amount: pesos("80"), ActorID: cashier}); err != nil {
		t.Fatalf("charge: %v", err)
	}
	ord, err = svc.AddLines(ctx, ord.ID, []domain.OrderLineInput{{ProductID: addedProduct, Qty: pesos("1")}}, cashier, uuid.New())
	if err != nil {
		t.Fatalf("AddLines: %v", err)
	}
	if err := svc.DeliverAll(ctx, ord.ID); err != nil {
		t.Fatalf("DeliverAll: %v", err)
	}
	var paidLine, addedLine int64
	for _, l := range ord.Lines {
		if l.ProductName == "Producto agregado por error" {
			addedLine = l.ID
		} else {
			paidLine = l.ID
		}
	}
	if addedLine == 0 || paidLine == 0 {
		t.Fatalf("both lines expected in %+v", ord.Lines)
	}

	// The starting point is the bug: the regular cancel rejects a delivered line.
	if _, err := svc.CancelarRenglon(ctx, ord.ID, addedLine, cashier, "agregado por error"); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("regular cancel should be a conflict, got %v", err)
	}

	// Paid by amount leaves no covered pieces, so what stops voiding the wrong line is the other
	// barrier: the total cannot drop below what was collected.
	if _, err := svc.VoidUnpaidLine(ctx, ord.ID, paidLine, cashier, "prueba"); !errors.Is(err, domain.ErrOrderWouldBeOverpaid) {
		t.Fatalf("voiding the paid line should be rejected as overpaid, got %v", err)
	}
	if _, err := svc.VoidUnpaidLine(ctx, ord.ID, addedLine, cashier, "  "); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("a missing reason should be a validation error, got %v", err)
	}

	if _, err := svc.VoidUnpaidLine(ctx, ord.ID, addedLine, cashier, "agregado por error; se cobró en otro pedido"); err != nil {
		t.Fatalf("VoidUnpaidLine: %v", err)
	}
	// A double tap is a no-op.
	if _, err := svc.VoidUnpaidLine(ctx, ord.ID, addedLine, cashier, "agregado por error; se cobró en otro pedido"); err != nil {
		t.Fatalf("second call should be a no-op, got %v", err)
	}

	det, err := svc.Detail(ctx, ord.ID)
	if err != nil {
		t.Fatalf("Detail: %v", err)
	}
	var cancelled bool
	for _, l := range det.Lines {
		if l.ID == addedLine {
			cancelled = l.Cancelled
		}
	}
	if !cancelled {
		t.Fatal("the line should be cancelled")
	}
	if !det.Total.Equal(pesos("80")) || !det.Outstanding.IsZero() || !det.Paid {
		t.Fatalf("total=%s outstanding=%s paid=%v: want 80, nothing outstanding and paid", det.Total, det.Outstanding, det.Paid)
	}
	var events int
	if err := st.Pool.QueryRow(ctx, `select count(*) from audit_events where order_id = $1 and action = 'order.line_removed' and after->>'unpaid' = 'true'`, ord.ID).Scan(&events); err != nil {
		t.Fatalf("read audit: %v", err)
	}
	if events != 1 {
		t.Fatalf("want ONE audit event, got %d", events)
	}
}

// A line covered by a payment (charged by products) is not voided: the payment goes back first.
func TestALineCoveredByAPaymentIsNotVoided(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()
	svc := app.NewOrdersService(st, clock)

	cashier := makeUser(t, st, "cashier_void_covered", "cajero")
	cash := paymentMethodID(t, st, "Efectivo")
	abrirCajaPrincipal(t, st, cashier)

	first := makeProduct(t, st, "Producto uno", pesos("80"), false)
	second := makeProduct(t, st, "Producto dos", pesos("69"), false)
	ord, err := svc.Create(ctx, app.CreateOrderCmd{
		ClientUUID: uuid.New(), ServiceType: "mostrador", OpenedBy: cashier,
		Lines: []domain.OrderLineInput{{ProductID: first, Qty: pesos("1")}, {ProductID: second, Qty: pesos("1")}},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := svc.Charge(ctx, app.ChargeCmd{
		OrderID: ord.ID, MethodID: cash, ActorID: cashier,
		Lines: []domain.SelectedPieces{{LineID: ord.Lines[0].ID, Qty: pesos("1")}},
	}); err != nil {
		t.Fatalf("charge the first product: %v", err)
	}
	if err := svc.DeliverAll(ctx, ord.ID); err != nil {
		t.Fatalf("DeliverAll: %v", err)
	}
	if _, err := svc.VoidUnpaidLine(ctx, ord.ID, ord.Lines[0].ID, cashier, "prueba"); !errors.Is(err, domain.ErrPieceAlreadyPaid) {
		t.Fatalf("a covered line should be rejected with ErrPieceAlreadyPaid, got %v", err)
	}
}
