//go:build integration

package integration

import (
	"context"
	"testing"

	"uuid"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store/db"
)

// splitBillFingerprint resume, leído como dueño, lo que las consultas de dividir la cuenta podrían
// tocar de un pedido: sus pagos, su cobertura, su bitácora, lo que se pasó y a dónde fue.
func splitBillFingerprint(t *testing.T, st *store.Store, orderID int64) string {
	t.Helper()
	var s string
	if err := st.Pool.QueryRow(context.Background(), `
		select o.status::text || '|' || coalesce(o.merged_into_order_id::text, '-') || '|' ||
		       (select count(*) || ':' || coalesce(sum(amount), 0) from order_payments where order_id = o.id) || '|' ||
		       (select count(*) from order_payment_lines pl join order_lines ol on ol.id = pl.order_line_id where ol.order_id = o.id) || '|' ||
		       (select count(*) from order_payment_voids where order_id = o.id) || '|' ||
		       (select count(*) from order_line_move_batches where from_order_id = o.id) || '|' ||
		       (select string_agg(ol.id || ':' || ol.order_id, ',' order by ol.id) from order_lines ol where ol.order_id = o.id) || '|' ||
		       (select count(*) || ':' || coalesce(sum(quantity), 0) from stock_movements where order_id = o.id)
		  from orders o where o.id = $1`, orderID).Scan(&s); err != nil {
		t.Fatal(err)
	}
	return s
}

// LAS CONSULTAS DE DIVIDIR LA CUENTA, AISLADAS EN LOS TRES CASOS.
//
// Cobertura, bitácora de pagos devueltos, lotes de «Pasar», piezas pagadas por renglón y los pagos
// del pedido y del turno: ninguna, desde otra empresa, con una conexión reciclada o sin empresa,
// lee ni cambia nada de la dueña.
func TestSplitBillQueriesStayIsolated(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()
	other := makeCompany(t, st, "ajena-dividir")
	cajero := makeUser(t, st, "cajero_aislado_dividir", "cajero")
	session := abrirCajaPrincipal(t, st, cajero)
	efectivo := paymentMethodID(t, st, "Efectivo")
	frappe := makeProduct(t, st, "Frappé dividido aislado", pesos("50"), true)

	appSt := appRoleStore(t)
	tctx, release, err := appSt.AcquireTenant(ctx, defaultCompanyID)
	if err != nil {
		t.Fatalf("AcquireTenant: %v", err)
	}
	svc := app.NewOrdersService(appSt, clock)
	newOrder := func() *app.OrderView {
		o, err := svc.Create(tctx, app.CreateOrderCmd{
			ClientUUID: uuid.New(), ServiceType: "mostrador", OpenedBy: cajero,
			Lines: []domain.OrderLineInput{{ProductID: frappe, Qty: pesos("2")}},
		})
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		return o
	}
	ord, dest := newOrder(), newOrder()
	line := ord.Lines[0].ID
	if _, err := svc.Charge(tctx, app.ChargeCmd{OrderID: ord.ID, MethodID: efectivo, Amount: pesos("30"), ActorID: cajero}); err != nil {
		t.Fatalf("Charge: %v", err)
	}
	release()

	var payment int64
	if err := st.Pool.QueryRow(ctx, `select id from order_payments where order_id = $1`, ord.ID).Scan(&payment); err != nil {
		t.Fatal(err)
	}
	payKey, voidKey, moveKey := uuid.New(), uuid.New(), uuid.New()
	if _, err := st.Pool.Exec(ctx, `update order_payments set client_uuid = $2, payment_number = 1, split_part = 1, split_of = 2 where id = $1`, payment, payKey); err != nil {
		t.Fatal(err)
	}
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{`insert into order_payment_lines (company_id, order_payment_id, order_line_id, qty, amount) values ($1, $2, $3, 1, 30)`,
			[]any{defaultCompanyID, payment, line}},
		{`insert into order_payment_voids (company_id, order_id, original_payment_id, payment_number, payment_method_id, amount,
			tip_amount, register_session_id, paid_at, client_uuid, voided_by, reason)
			values ($1, $2, -1, 2, $3, 10, 0, $4, now(), $5, $6, 'prueba')`, []any{defaultCompanyID, ord.ID, efectivo, session, voidKey, cajero}},
		{`insert into order_line_move_batches (company_id, client_uuid, from_order_id, to_order_id, moved_by) values ($1, $2, $3, $4, $5)`,
			[]any{defaultCompanyID, moveKey, ord.ID, dest.ID, cajero}},
		{`insert into order_line_moves (company_id, client_uuid, order_line_id, qty) values ($1, $2, $3, 1)`,
			[]any{defaultCompanyID, moveKey, line}},
	} {
		if _, err := st.Pool.Exec(ctx, q.sql, q.args...); err != nil {
			t.Fatalf("sembrar: %v", err)
		}
	}
	before := splitBillFingerprint(t, st, ord.ID)

	inTheThreeCases(t, defaultCompanyID, other, func(t *testing.T, st2 *store.Store, c context.Context) {
		q := st2.QC(c)
		actor := cajero
		two := int16(2)
		// Cada lectura dice si VIO algo de la dueña; un error de la base también es no ver.
		reads := []struct {
			name string
			saw  func() bool
		}{
			{"ListLinesForSelection", func() bool { r, err := q.ListLinesForSelection(c, ord.ID); return err == nil && len(r) > 0 }},
			{"CountOrderPaymentsForNumber", func() bool { n, err := q.CountOrderPaymentsForNumber(c, ord.ID); return err == nil && n > 0 }},
			{"ListChargedSplitParts", func() bool {
				r, err := q.ListChargedSplitParts(c, db.ListChargedSplitPartsParams{OrderID: ord.ID, SplitOf: &two})
				return err == nil && len(r) > 0
			}},
			{"GetOrderPaymentShapeByClientUUID", func() bool { _, err := q.GetOrderPaymentShapeByClientUUID(c, &payKey); return err == nil }},
			{"GetPaymentVoidByClientUUID", func() bool { _, err := q.GetPaymentVoidByClientUUID(c, &voidKey); return err == nil }},
			{"ListOrderPaymentsForView", func() bool { r, err := q.ListOrderPaymentsForView(c, ord.ID); return err == nil && len(r) > 0 }},
			{"ListOrderPaymentCoverage", func() bool { r, err := q.ListOrderPaymentCoverage(c, ord.ID); return err == nil && len(r) > 0 }},
			{"ListOrderPaymentVoids", func() bool { r, err := q.ListOrderPaymentVoids(c, ord.ID); return err == nil && len(r) > 0 }},
			{"ListPaidQtyForOrders", func() bool { r, err := q.ListPaidQtyForOrders(c, []int64{ord.ID}); return err == nil && len(r) > 0 }},
			{"GetOrderPaymentForVoid", func() bool { _, err := q.GetOrderPaymentForVoid(c, payment); return err == nil }},
			{"GetLineMoveBatch", func() bool { _, err := q.GetLineMoveBatch(c, moveKey); return err == nil }},
			{"ListSessionPaymentVoids", func() bool { r, err := q.ListSessionPaymentVoids(c, session); return err == nil && len(r) > 0 }},
			{"Detail con pagos", func() bool {
				v, err := app.NewOrdersService(st2, clock).Detail(c, ord.ID)
				return err == nil && v != nil
			}},
		}
		for _, r := range reads {
			if r.saw() {
				t.Errorf("%s vio datos de la dueña", r.name)
			}
		}
		// Las escrituras se juzgan por la huella: no deben alcanzar las filas de la dueña.
		writes := []struct {
			name string
			run  func()
		}{
			{"DeleteOrderPayment", func() { _ = q.DeleteOrderPayment(c, payment) }},
			{"MoveOrderLineToOrder", func() { _ = q.MoveOrderLineToOrder(c, db.MoveOrderLineToOrderParams{ToOrderID: dest.ID, ID: line}) }},
			{"MoveLineStockMovements", func() {
				_ = q.MoveLineStockMovements(c, db.MoveLineStockMovementsParams{ToOrderID: &dest.ID, LineID: &line})
			}},
			{"MarkOrderMerged", func() {
				_, _ = q.MarkOrderMerged(c, db.MarkOrderMergedParams{ActorID: &actor, IntoOrderID: &dest.ID, ID: ord.ID})
			}},
		}
		for _, w := range writes {
			w.run()
			if after := splitBillFingerprint(t, st, ord.ID); after != before {
				t.Fatalf("%s cambió el pedido de la dueña:\nantes   %s\ndespués %s", w.name, before, after)
			}
		}
	})
}
