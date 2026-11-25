//go:build integration

package integration

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"uuid"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
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

// An order that never had a payment keeps its delivered lines: voiding them would erase a sale.
func TestADeliveredLineOfAnOrderWithoutPaymentsIsNotVoided(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()
	svc := app.NewOrdersService(st, clock)

	cashier := makeUser(t, st, "cashier_void_nopay", "cajero")
	abrirCajaPrincipal(t, st, cashier)

	product := makeProduct(t, st, "Producto sin cobro", pesos("80"), false)
	ord, err := svc.Create(ctx, app.CreateOrderCmd{
		ClientUUID: uuid.New(), ServiceType: "mostrador", OpenedBy: cashier,
		Lines: []domain.OrderLineInput{{ProductID: product, Qty: pesos("1")}},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := svc.DeliverAll(ctx, ord.ID); err != nil {
		t.Fatalf("DeliverAll: %v", err)
	}
	if _, err := svc.VoidUnpaidLine(ctx, ord.ID, ord.Lines[0].ID, cashier, "prueba"); !errors.Is(err, domain.ErrOrderWithoutPayments) {
		t.Fatalf("an order without payments should be rejected with ErrOrderWithoutPayments, got %v", err)
	}
}

// builds a paid order that also has a delivered line with no payment, ready to be voided.
func paidOrderWithUnpaidDeliveredLine(t *testing.T, st *store.Store, svc *app.OrdersService, suffix string) (orderID, unpaidLine, cashier int64) {
	t.Helper()
	ctx := context.Background()
	cashier = makeUser(t, st, "cashier_void_"+suffix, "cajero")
	cash := paymentMethodID(t, st, "Efectivo")
	abrirCajaPrincipal(t, st, cashier)

	paid := makeProduct(t, st, "Producto cobrado "+suffix, pesos("80"), false)
	added := makeProduct(t, st, "Producto agregado "+suffix, pesos("69"), false)
	ord, err := svc.Create(ctx, app.CreateOrderCmd{
		ClientUUID: uuid.New(), ServiceType: "mostrador", OpenedBy: cashier,
		Lines: []domain.OrderLineInput{{ProductID: paid, Qty: pesos("1")}},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := svc.Charge(ctx, app.ChargeCmd{OrderID: ord.ID, MethodID: cash, Amount: pesos("80"), ActorID: cashier}); err != nil {
		t.Fatalf("charge: %v", err)
	}
	ord, err = svc.AddLines(ctx, ord.ID, []domain.OrderLineInput{{ProductID: added, Qty: pesos("1")}}, cashier, uuid.New())
	if err != nil {
		t.Fatalf("AddLines: %v", err)
	}
	if err := svc.DeliverAll(ctx, ord.ID); err != nil {
		t.Fatalf("DeliverAll: %v", err)
	}
	for _, l := range ord.Lines {
		if l.ProductName == "Producto agregado "+suffix {
			unpaidLine = l.ID
		}
	}
	if unpaidLine == 0 {
		t.Fatalf("added line not found in %+v", ord.Lines)
	}
	return ord.ID, unpaidLine, cashier
}

// A closed shift already went through the cash count: the order does not change anymore.
func TestAUnpaidLineOfAClosedShiftIsNotVoided(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	svc := app.NewOrdersService(st, clock)
	orderID, line, cashier := paidOrderWithUnpaidDeliveredLine(t, st, svc, "closed")

	if _, err := st.Pool.Exec(context.Background(),
		`update register_sessions set status = 'cerrada' where id = (select register_session_id from orders where id = $1)`, orderID); err != nil {
		t.Fatalf("close the shift: %v", err)
	}
	if _, err := svc.VoidUnpaidLine(context.Background(), orderID, line, cashier, "prueba"); !errors.Is(err, domain.ErrOrderFromClosedShift) {
		t.Fatalf("a closed shift should be rejected with ErrOrderFromClosedShift, got %v", err)
	}
}

// The cash count adds the written-off amount: voiding after a write-off would keep reporting a loss
// the order no longer has.
func TestAUnpaidLineOfAnOrderWithAWriteOffIsNotVoided(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	svc := app.NewOrdersService(st, clock)
	orderID, line, cashier := paidOrderWithUnpaidDeliveredLine(t, st, svc, "writeoff")

	if err := svc.WriteOff(context.Background(), app.WriteOffCmd{OrderID: orderID, Motivo: "no pagó", ActorID: cashier}); err != nil {
		t.Fatalf("WriteOff: %v", err)
	}
	if _, err := svc.VoidUnpaidLine(context.Background(), orderID, line, cashier, "prueba"); !errors.Is(err, domain.ErrOrderWithWriteOff) {
		t.Fatalf("a previous write-off should be rejected with ErrOrderWithWriteOff, got %v", err)
	}
}

// The route asks for orders.cancel: a cashier gets 403 and nothing changes.
func TestACashierCannotVoidAnUnpaidLineThroughTheRoute(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	svc := app.NewOrdersService(st, clock)
	orderID, line, cashier := paidOrderWithUnpaidDeliveredLine(t, st, svc, "route")

	r, jm := routerDePedidos(t, st)
	tok, err := jm.Issue(domain.User{ID: cashier, CompanyID: defaultCompanyID, Name: "Cajero", Role: domain.RoleCajero})
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	path := "/api/v1/orders/" + itoa(int(orderID)) + "/lines/" + itoa(int(line)) + "/void-unpaid"
	w := do(t, r, http.MethodPost, path, tok, []byte(`{"reason":"prueba"}`), "application/json")
	if w.Code != http.StatusForbidden {
		t.Fatalf("a cashier should get 403, got %d: %s", w.Code, w.Body.String())
	}
	var cancelled bool
	if err := st.Pool.QueryRow(context.Background(), `select cancelled_at is not null from order_lines where id = $1`, line).Scan(&cancelled); err != nil {
		t.Fatalf("read the line: %v", err)
	}
	if cancelled {
		t.Fatal("the line must stay as it was")
	}
}
