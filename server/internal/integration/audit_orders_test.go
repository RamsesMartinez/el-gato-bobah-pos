//go:build integration

package integration

import (
	"context"
	"strings"
	"testing"
	"uuid"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
)

// auditRow es lo que el test lee de audit_events.
type auditRow struct {
	action, actorKind string
	actor, order      *int64
	branch            *int64
	after             string
}

func orderEvents(t *testing.T, st *store.Store, orderID int64) []auditRow {
	t.Helper()
	rows, err := st.Pool.Query(context.Background(), `
		select action, actor_kind, actor_user_id, order_id, branch_id, coalesce(after::text, '')
		  from audit_events where order_id = $1 order by id`, orderID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []auditRow
	for rows.Next() {
		var e auditRow
		if err := rows.Scan(&e.action, &e.actorKind, &e.actor, &e.order, &e.branch, &e.after); err != nil {
			t.Fatal(err)
		}
		out = append(out, e)
	}
	return out
}

func actionsOf(evs []auditRow) []string {
	out := make([]string, len(evs))
	for i, e := range evs {
		out[i] = e.action
	}
	return out
}

// EL CASO DE LOS $218. Un pedido que se cobra dos veces, se devuelve un pago y se entrega tiene que
// poder reconstruirse completo: quién hizo cada cosa, en qué orden, con qué método y cuánto.
func TestAnOrderHistoryTellsEverythingThatHappened(t *testing.T) {
	t.Parallel()
	k := newLiveKit(t)
	abrirCajaPrincipal(t, k.st, k.user)
	// Como en un request: la persona autenticada viaja en el contexto (lo pone RequireAuth).
	k.ctx = app.WithActorUser(k.ctx, k.user)
	o := k.order(t, "1", "0", false)

	if _, err := k.orders.AddLines(k.ctx, o.ID, []domain.OrderLineInput{{ProductID: k.product, Qty: pesos("1")}}, k.user, uuid.New()); err != nil {
		t.Fatalf("AddLines: %v", err)
	}
	if _, err := k.orders.SetDiscount(k.ctx, app.SetDiscountCmd{OrderID: o.ID, Amount: ptrDec(pesos("10")), Actor: k.user}); err != nil {
		t.Fatalf("SetDiscount: %v", err)
	}
	detail, err := k.orders.Detail(k.ctx, o.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := k.orders.RemovePieces(k.ctx, o.ID, detail.Lines[1].ID, k.user, "Se capturó de más", nil); err != nil {
		t.Fatalf("RemovePieces: %v", err)
	}
	// Total: 100 − 10 = 90. Se cobra dos veces la mitad y una se devuelve.
	first, err := k.orders.Charge(k.ctx, app.ChargeCmd{OrderID: o.ID, MethodID: k.efectivo, Amount: pesos("45"), ActorID: k.user, ClientUUID: uuid.New()})
	if err != nil {
		t.Fatalf("Charge: %v", err)
	}
	if _, err := k.orders.VoidPayment(k.ctx, o.ID, first.PaymentID, k.user, "Se cobró dos veces"); err != nil {
		t.Fatalf("VoidPayment: %v", err)
	}
	if _, err := k.orders.Charge(k.ctx, app.ChargeCmd{OrderID: o.ID, MethodID: k.efectivo, Amount: pesos("90"), ActorID: k.user, ClientUUID: uuid.New()}); err != nil {
		t.Fatalf("Charge: %v", err)
	}
	if err := k.orders.DeliverAll(k.ctx, o.ID); err != nil {
		t.Fatalf("DeliverAll: %v", err)
	}
	if err := k.orders.Devolver(k.ctx, app.DevolucionCmd{OrderID: o.ID, Monto: pesos("10"), Motivo: "Llegó frío", ActorID: k.user}); err != nil {
		t.Fatalf("Devolver: %v", err)
	}

	evs := orderEvents(t, k.st, o.ID)
	want := []string{"order.created", "order.lines_added", "order.discount_set", "order.line_removed",
		"payment.charged", "payment.voided", "payment.charged", "order.delivered", "payment.refunded"}
	if strings.Join(actionsOf(evs), " ") != strings.Join(want, " ") {
		t.Fatalf("historial = %v\n           quiere %v", actionsOf(evs), want)
	}
	for _, e := range evs {
		if e.actorKind != "user" || e.actor == nil || e.branch == nil {
			t.Errorf("%s sin autor o sin sucursal: %+v", e.action, e)
		}
	}
	if !strings.Contains(evs[4].after, `"methodId"`) || !strings.Contains(evs[4].after, `"amount"`) {
		t.Errorf("el cobro no dice con qué método ni cuánto: %s", evs[4].after)
	}
}

func TestMovingLinesShowsInBothOrdersHistory(t *testing.T) {
	t.Parallel()
	k := newLiveKit(t)
	abrirCajaPrincipal(t, k.st, k.user)
	// Como en un request: la persona autenticada viaja en el contexto (lo pone RequireAuth).
	k.ctx = app.WithActorUser(k.ctx, k.user)
	origin := k.order(t, "2", "0", false)
	target := k.order(t, "1", "0", false)
	detail, err := k.orders.Detail(k.ctx, origin.ID)
	if err != nil {
		t.Fatal(err)
	}
	cmd := app.MoveLinesCmd{ClientUUID: uuid.New(), FromOrderID: origin.ID, ToOrderID: &target.ID, ActorID: k.user,
		Lines: []domain.SelectedPieces{{LineID: detail.Lines[0].ID, Qty: pesos("1")}}}
	if _, err := k.orders.MoveLines(k.ctx, cmd); err != nil {
		t.Fatalf("MoveLines: %v", err)
	}
	// El mismo lote reenviado (tableta que reintenta) no duplica el evento.
	if _, err := k.orders.MoveLines(k.ctx, cmd); err != nil {
		t.Fatalf("reenvío: %v", err)
	}
	for _, id := range []int64{origin.ID, target.ID} {
		n := 0
		for _, e := range orderEvents(t, k.st, id) {
			if e.action == "order.lines_moved" {
				n++
			}
		}
		if n != 1 {
			t.Errorf("el pedido %d tiene %d eventos de traslado, quiere 1", id, n)
		}
	}
}

func TestCancellingAndWritingOffAreAudited(t *testing.T) {
	t.Parallel()
	k := newLiveKit(t)
	abrirCajaPrincipal(t, k.st, k.user)
	// Como en un request: la persona autenticada viaja en el contexto (lo pone RequireAuth).
	k.ctx = app.WithActorUser(k.ctx, k.user)
	cancelled := k.order(t, "1", "0", false)
	if err := k.orders.Cancel(k.ctx, cancelled.ID, k.user, "El cliente se fue"); err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	writtenOff := k.order(t, "1", "40", true)
	if err := k.orders.WriteOff(k.ctx, app.WriteOffCmd{OrderID: writtenOff.ID, Motivo: "Se fue sin pagar", ActorID: k.user}); err != nil {
		t.Fatalf("WriteOff: %v", err)
	}
	if got := actionsOf(orderEvents(t, k.st, cancelled.ID)); got[len(got)-1] != "order.cancelled" {
		t.Errorf("cancelar no dejó evento: %v", got)
	}
	// Cancelar devolviendo dice CUÁNTO y por qué medio: «se devolvió: sí» no contesta a dónde fue el
	// dinero.
	charged := k.order(t, "1", "100", false)
	if err := k.orders.CancelarConDevolucion(k.ctx, app.CancelacionCmd{OrderID: charged.ID, Motivo: "Se equivocó de pedido", ActorID: k.user, Devolver: true}); err != nil {
		t.Fatalf("cancelar devolviendo: %v", err)
	}
	var refunded string
	if err := k.st.Pool.QueryRow(context.Background(), `select after->'refunds'->0->>'amount' from audit_events
		where action = 'order.cancelled' and order_id = $1`, charged.ID).Scan(&refunded); err != nil || refunded != "100" {
		t.Fatalf("el evento de cancelar no dice cuánto se devolvió: %q %v", refunded, err)
	}
	if got := actionsOf(orderEvents(t, k.st, writtenOff.ID)); got[len(got)-1] != "stock.written_off" {
		t.Errorf("dar por perdido no dejó evento: %v", got)
	}
}

// Un cobro que se rechaza no deja evento: la acción y su evento se guardan juntos o ninguno.
func TestARejectedChargeLeavesNoEvent(t *testing.T) {
	t.Parallel()
	k := newLiveKit(t)
	abrirCajaPrincipal(t, k.st, k.user)
	// Como en un request: la persona autenticada viaja en el contexto (lo pone RequireAuth).
	k.ctx = app.WithActorUser(k.ctx, k.user)
	o := k.order(t, "1", "0", false)
	if _, err := k.orders.Charge(k.ctx, app.ChargeCmd{OrderID: o.ID, MethodID: k.efectivo, Amount: pesos("500"), ActorID: k.user, ClientUUID: uuid.New()}); err == nil {
		t.Fatal("se aceptó cobrar $500 de un pedido de $100")
	}
	for _, a := range actionsOf(orderEvents(t, k.st, o.ID)) {
		if a == "payment.charged" {
			t.Fatal("un cobro rechazado dejó evento en la bitácora")
		}
	}
}

func TestCorrectingThePlatformRefIsAudited(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	makeUser(t, st, "quien-folio", "cajero")
	platform := plataformaDe(t, st, defaultCompanyID)
	orderID := nuevoPedidoDePlataforma(t, st, defaultCompanyID, platform, 9100)
	who := quienSea(t, st, defaultCompanyID)
	orders := app.NewOrdersService(st, clock)
	if _, err := orders.SetPlatformRef(context.Background(), orderID, "UBER-1", who); err != nil {
		t.Fatalf("SetPlatformRef: %v", err)
	}
	evs := orderEvents(t, st, orderID)
	if len(evs) != 1 || evs[0].action != "order.platform_ref_set" || !strings.Contains(evs[0].after, "UBER-1") {
		t.Fatalf("eventos = %+v", evs)
	}
}

// El historial de un pedido no se lee desde otra empresa, ni con una conexión reciclada, ni sin
// empresa: trae IP y aparato de personas de otro negocio.
func TestOrderHistoryIsNotReadableFromAnotherCompany(t *testing.T) {
	t.Parallel()
	k := newLiveKit(t)
	abrirCajaPrincipal(t, k.st, k.user)
	o := k.order(t, "1", "0", false)
	otherCompany := makeCompany(t, k.st, "historial-otra")
	own, err := app.NewAuditService(k.appSt).OrderHistory(k.ctx, o.ID)
	if err != nil || len(own) == 0 {
		t.Fatalf("la dueña no ve su historial: %v %v", own, err)
	}
	inTheThreeCases(t, defaultCompanyID, otherCompany, func(t *testing.T, s *store.Store, ctx context.Context) {
		evs, err := app.NewAuditService(s).OrderHistory(ctx, o.ID)
		if err == nil && len(evs) > 0 {
			t.Fatalf("se leyeron %d eventos de un pedido de otra empresa", len(evs))
		}
		if err == nil && evs == nil {
			t.Fatal("el historial vacío salió como null y no como []")
		}
	})
}
