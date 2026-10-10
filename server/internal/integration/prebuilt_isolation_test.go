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

// orderFingerprint resume, leído como dueño, todo lo que quitar, partir o reponer podría tocar de un
// pedido: sus renglones, sus modificadores, sus movimientos y su estado.
func orderFingerprint(t *testing.T, st *store.Store, orderID int64) string {
	t.Helper()
	var s string
	if err := st.Pool.QueryRow(context.Background(), `
		select o.status::text || '|' ||
		       (select string_agg(ol.id || ':' || ol.quantity || ':' || ol.delivered_qty || ':' || (ol.cancelled_at is null), ',' order by ol.id)
		          from order_lines ol where ol.order_id = o.id) || '|' ||
		       (select count(*) from order_line_modifiers m join order_lines ol on ol.id = m.order_line_id where ol.order_id = o.id) || '|' ||
		       (select count(*) || ':' || coalesce(sum(quantity), 0) from stock_movements where order_id = o.id)
		  from orders o where o.id = $1`, orderID).Scan(&s); err != nil {
		t.Fatal(err)
	}
	return s
}

// LO QUE SE CONSTRUYÓ ANTES DE TENER EL HELPER DE LOS TRES CASOS, AISLADO EN LOS TRES.
//
// Quitar lo que falta, partir un renglón y reponer al cancelar nacieron antes de que existiera
// `inTheThreeCases`. Cada una, llamada desde otra empresa, con una conexión reciclada o sin empresa,
// no alcanza ni cambia nada del pedido de la dueña.
func TestPrebuiltRemovalAndSplitStayIsolated(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()
	other := makeCompany(t, st, "ajena-quitar")
	cajero := makeUser(t, st, "cajero_aislado_quitar", "cajero")
	abrirCajaPrincipal(t, st, cajero)
	frappe := makeProduct(t, st, "Frappé aislado", pesos("50"), true)
	extra := optionID(t, st, defaultCompanyID, frappe)

	appSt := appRoleStore(t)
	tctx, release, err := appSt.AcquireTenant(ctx, defaultCompanyID)
	if err != nil {
		t.Fatalf("AcquireTenant: %v", err)
	}
	svc := app.NewOrdersService(appSt, clock)
	ord, err := svc.Create(tctx, app.CreateOrderCmd{
		ClientUUID: uuid.New(), ServiceType: "mostrador", OpenedBy: cajero,
		Lines: []domain.OrderLineInput{{ProductID: frappe, Qty: pesos("3"),
			Modifiers: []domain.OrderModInput{{OptionID: extra, Qty: 1}}}},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	line := ord.Lines[0].ID
	if err := svc.DeliverLine(tctx, ord.ID, line, pesos("1")); err != nil {
		t.Fatalf("entregar una pieza: %v", err)
	}
	release()
	before := orderFingerprint(t, st, ord.ID)

	inTheThreeCases(t, defaultCompanyID, other, func(t *testing.T, st2 *store.Store, c context.Context) {
		q := st2.QC(c)
		actor := cajero
		oid := ord.ID
		// Cada llamada dice si ALCANZÓ algo de la dueña. Un error de la base (sin empresa, sin
		// permiso) también es no alcanzar.
		calls := []struct {
			name    string
			reached func() bool
		}{
			{"CancelPending", func() bool {
				_, err := app.NewOrdersService(st2, clock).CancelPending(c, ord.ID, cajero, "Ya no lo quiere")
				return err == nil
			}},
			{"CancelarRenglon", func() bool {
				_, err := app.NewOrdersService(st2, clock).CancelarRenglon(c, ord.ID, line, cajero, "Ya no lo quiere")
				return err == nil
			}},
			{"GetOrderLineForCancel", func() bool {
				_, err := q.GetOrderLineForCancel(c, db.GetOrderLineForCancelParams{ID: line, OrderID: ord.ID})
				return err == nil
			}},
			{"ListLinesToSplit", func() bool {
				rows, err := q.ListLinesToSplit(c, ord.ID)
				return err == nil && len(rows) > 0
			}},
			{"ListLineSaleMovements", func() bool {
				rows, err := q.ListLineSaleMovements(c, &line)
				return err == nil && len(rows) > 0
			}},
			{"SplitOffOrderLine", func() bool {
				_, err := q.SplitOffOrderLine(c, db.SplitOffOrderLineParams{OrderID: ord.ID, Quantity: pesos("1"),
					LineTotal: pesos("50"), DeliveredQty: pesos("0"), LineID: line})
				return err == nil
			}},
			// Las que no devuelven filas se juzgan por la huella del pedido.
			{"CopyOrderLineModifiers", func() bool {
				_ = q.CopyOrderLineModifiers(c, db.CopyOrderLineModifiersParams{NewLineID: line, LineID: line})
				return false
			}},
			{"ShrinkOrderLine", func() bool {
				_ = q.ShrinkOrderLine(c, db.ShrinkOrderLineParams{Quantity: pesos("1"), DeliveredQty: pesos("1"), LineTotal: pesos("50"), ID: line})
				return false
			}},
			{"RestockCancelledOrder", func() bool {
				_ = q.RestockCancelledOrder(c, db.RestockCancelledOrderParams{ActorID: &actor, Oid: &oid})
				return false
			}},
		}
		for _, call := range calls {
			if call.reached() {
				t.Errorf("%s alcanzó el pedido de la dueña", call.name)
			}
			if after := orderFingerprint(t, st, ord.ID); after != before {
				t.Fatalf("%s cambió el pedido de la dueña:\nantes   %s\ndespués %s", call.name, before, after)
			}
		}
	})
}
