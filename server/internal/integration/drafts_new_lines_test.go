//go:build integration

package integration

import (
	"context"
	"errors"
	"sync"
	"testing"

	"uuid"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
)

// newOf abre (o reusa) lo «Nuevo» de un pedido.
func (k *liveKit) newOf(t *testing.T, ctx context.Context, orderID int64, lines ...app.DraftLineCmd) (*app.DraftView, error) {
	t.Helper()
	v, _, err := k.drafts.Create(ctx, app.CreateDraftCmd{ID: uuid.New(), OrderID: &orderID, Lines: lines, Actor: k.user})
	return v, err
}

// AGREGAR DESPUÉS DE COCINA MANDA SOLO LO NUEVO (US3, D-3, casos 6 y 7).
func TestNewLinesOfASentOrder(t *testing.T) {
	t.Parallel()
	k := newLiveKit(t)
	abrirCajaPrincipal(t, k.st, k.user)
	ord := k.order(t, "3", "0", false)

	nuevo, err := k.newOf(t, k.ctx, ord.ID, addOf(k.product, "1"))
	if err != nil {
		t.Fatal(err)
	}
	if nuevo.OrderID == nil || *nuevo.OrderID != ord.ID || nuevo.FolioName != nil {
		t.Fatalf("lo «Nuevo» = %+v: apunta a su pedido y no tiene nombre propio", nuevo)
	}

	t.Run("una segunda tableta recibe la misma «Nuevo» con su renglón sumado", func(t *testing.T) {
		other, err := k.newOf(t, k.tenant(t, k.company), ord.ID, addOf(k.product, "1"))
		if err != nil {
			t.Fatal(err)
		}
		if other.ID != nuevo.ID || !lineQty(t, other, k.product).Equal(pesos("2")) {
			t.Fatalf("id %v (quería %v), qty %s: dos «Nuevo» del mismo pedido son dos cuentas para una mesa",
				other.ID, nuevo.ID, lineQty(t, other, k.product))
		}
	})

	t.Run("cocina no lo recibe hasta mandarlo", func(t *testing.T) {
		board, err := k.orders.Board(k.ctx)
		if err != nil {
			t.Fatal(err)
		}
		for _, b := range board {
			if b.ID == ord.ID && len(b.Lines) != 1 {
				t.Fatalf("el tablero ya tiene %d renglones: lo «Nuevo» no ha ido a cocina", len(b.Lines))
			}
		}
	})

	t.Run("mandarlo agrega solo lo nuevo y la comanda lo lleva", func(t *testing.T) {
		res, err := k.drafts.Send(k.ctx, nuevo.ID, k.user)
		if err != nil {
			t.Fatal(err)
		}
		if res.Created || res.Order.ID != ord.ID || len(res.PrintLineIDs) != 1 {
			t.Fatalf("created=%v pedido=%d printLineIds=%v", res.Created, res.Order.ID, res.PrintLineIDs)
		}
		if !res.Order.Total.Equal(pesos("500")) {
			t.Fatalf("total = %s, quería 500 (3 + 2 cafés)", res.Order.Total)
		}
		var pending int
		if err := k.st.Pool.QueryRow(context.Background(),
			`select count(*) from order_lines where order_id = $1 and enviado_a_cocina_at is null`, ord.ID).Scan(&pending); err != nil {
			t.Fatal(err)
		}
		if pending != 0 {
			t.Fatalf("%d renglones quedaron sin marcar en cocina", pending)
		}
		if got := draftStatus(t, k.st, nuevo.ID); got != domain.DraftSent {
			t.Fatalf("lo «Nuevo» quedó %s tras mandarlo: la fila lo seguiría ofreciendo como pendiente", got)
		}
		if _, err := k.drafts.AddLine(k.ctx, nuevo.ID, addOf(k.product, "1")); !errors.Is(err, domain.ErrDraftAlreadySent) {
			t.Fatalf("agregar a lo «Nuevo» ya mandado = %v, quería DRAFT_SENT", err)
		}
		again, err := k.drafts.Send(k.ctx, nuevo.ID, k.user)
		if err != nil || len(again.PrintLineIDs) != 0 {
			t.Fatalf("reintento: %v, printLineIds %v", err, again.PrintLineIDs)
		}
	})
}

// La entregada que debe recibe y vuelve a cocina (US3 AS4, caso 7).
func TestNewLinesReopenADeliveredOrderThatOwes(t *testing.T) {
	t.Parallel()
	k := newLiveKit(t)
	abrirCajaPrincipal(t, k.st, k.user)
	ord := k.order(t, "1", "0", true)
	nuevo, err := k.newOf(t, k.ctx, ord.ID, addOf(k.product, "1"))
	if err != nil {
		t.Fatal(err)
	}
	res, err := k.drafts.Send(k.ctx, nuevo.ID, k.user)
	if err != nil {
		t.Fatal(err)
	}
	if res.Order.Status != domain.StatusAbierta {
		t.Fatalf("status = %s: lo nuevo no llegaría a cocina", res.Order.Status)
	}
}

func TestClosedOrderReceivesNothing(t *testing.T) {
	t.Parallel()
	k := newLiveKit(t)
	abrirCajaPrincipal(t, k.st, k.user)
	ord := k.order(t, "1", "100", true)
	if _, err := k.newOf(t, k.ctx, ord.ID, addOf(k.product, "1")); !errors.Is(err, domain.ErrOrderClosed) {
		t.Fatalf("abrir «Nuevo» de un pedido cerrado = %v, quería ORDER_CLOSED", err)
	}
	if _, err := k.orders.AddLines(k.ctx, ord.ID, []domain.OrderLineInput{{ProductID: k.product, Qty: pesos("1")}}, k.user, uuid.New()); !errors.Is(err, domain.ErrOrderClosed) {
		t.Fatalf("POST /orders/{id}/lines a un pedido cerrado = %v, quería ORDER_CLOSED", err)
	}
}

func TestPlatformOrderReceivesNothing(t *testing.T) {
	t.Parallel()
	k := newLiveKit(t)
	abrirCajaPrincipal(t, k.st, k.user)
	uber := platformID(t, k.st, defaultCompanyID, "Uber Eats")
	ord, err := k.orders.Create(k.ctx, app.CreateOrderCmd{ClientUUID: uuid.New(), ServiceType: "domicilio", OpenedBy: k.user,
		DeliveryPlatformID: &uber, Lines: []domain.OrderLineInput{{ProductID: k.product, Qty: pesos("1")}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := k.newOf(t, k.ctx, ord.ID, addOf(k.product, "1")); !errors.Is(err, domain.ErrPlatformOrderNoLines) {
		t.Fatalf("= %v, quería PLATFORM_ORDER_NO_LINES", err)
	}
	if _, err := k.orders.AddLines(k.ctx, ord.ID, []domain.OrderLineInput{{ProductID: k.product, Qty: pesos("1")}}, k.user, uuid.New()); !errors.Is(err, domain.ErrPlatformOrderNoLines) {
		t.Fatalf("AddLines = %v", err)
	}
}

// El pedido se cerró (pagado y entregado) con su «Nuevo» viva: enviar se rechaza, la cuenta sigue viva
// y la fila la sigue mostrando (R-9).
func TestNewOfAnOrderThatClosedMeanwhile(t *testing.T) {
	t.Parallel()
	k := newLiveKit(t)
	abrirCajaPrincipal(t, k.st, k.user)
	ord := k.order(t, "1", "0", false)
	nuevo, err := k.newOf(t, k.ctx, ord.ID, addOf(k.product, "1"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := k.orders.Charge(k.ctx, app.ChargeCmd{OrderID: ord.ID, MethodID: k.efectivo, Amount: pesos("100"), ActorID: k.user}); err != nil {
		t.Fatal(err)
	}
	if err := k.orders.DeliverAll(k.ctx, ord.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := k.drafts.Send(k.ctx, nuevo.ID, k.user); !errors.Is(err, domain.ErrOrderClosed) {
		t.Fatalf("= %v, quería ORDER_CLOSED", err)
	}
	if got := draftStatus(t, k.st, nuevo.ID); got != domain.DraftCapturing {
		t.Fatalf("la «Nuevo» quedó %s", got)
	}
	_, byKey := k.live(t, k.ctx, false)
	if it, ok := byKey[orderKey(ord.ID)]; !ok || !it.ClosedWithPending {
		t.Fatalf("la fila no la muestra: %+v", it)
	}
}

func TestCancelledOrderDiscardsItsNew(t *testing.T) {
	t.Parallel()
	k := newLiveKit(t)
	abrirCajaPrincipal(t, k.st, k.user)
	ord := k.order(t, "1", "0", false)
	nuevo, err := k.newOf(t, k.ctx, ord.ID, addOf(k.product, "1"))
	if err != nil {
		t.Fatal(err)
	}
	if err := k.orders.Cancel(k.ctx, ord.ID, k.user, "se fueron"); err != nil {
		t.Fatal(err)
	}
	if _, err := k.drafts.Send(k.ctx, nuevo.ID, k.user); err == nil {
		t.Fatal("se mandó lo «Nuevo» de un pedido cancelado")
	}
	k.live(t, k.ctx, false) // el barrido corre al listar
	var status, reason string
	if err := k.st.Pool.QueryRow(context.Background(), `select status, coalesce(discard_reason, '') from order_drafts where id = $1`, nuevo.ID).
		Scan(&status, &reason); err != nil {
		t.Fatal(err)
	}
	if status != domain.DraftDiscarded || reason != domain.DiscardOrderClosed {
		t.Fatalf("la «Nuevo» de un pedido cancelado quedó %s/%s", status, reason)
	}
}

// Enviar y agregar a la vez: el renglón quedó en el pedido o el agregado se rechazó (y la tableta
// lo sabe); nunca se pierde en silencio.
func TestSendWhileAddingNeverLosesALine(t *testing.T) {
	t.Parallel()
	k := newLiveKit(t)
	abrirCajaPrincipal(t, k.st, k.user)
	leche := makeProduct(t, k.st, "Leche de la carrera", pesos("10"), false)
	ctxs := []context.Context{k.tenant(t, k.company), k.tenant(t, k.company)}
	for round := range 5 {
		ord := k.order(t, "1", "0", false)
		nuevo, err := k.newOf(t, k.ctx, ord.ID, addOf(k.product, "1"))
		if err != nil {
			t.Fatal(err)
		}
		var sendErr, addErr error
		var wg sync.WaitGroup
		wg.Add(2)
		go func() { defer wg.Done(); _, sendErr = k.drafts.Send(ctxs[0], nuevo.ID, k.user) }()
		go func() { defer wg.Done(); _, addErr = k.drafts.AddLine(ctxs[1], nuevo.ID, addOf(leche, "1")) }()
		wg.Wait()
		if sendErr != nil {
			t.Fatalf("vuelta %d: enviar falló: %v", round, sendErr)
		}
		var inOrder int
		if err := k.st.Pool.QueryRow(context.Background(), `select count(*) from order_lines where order_id = $1 and product_id = $2`,
			ord.ID, leche).Scan(&inOrder); err != nil {
			t.Fatal(err)
		}
		switch {
		case addErr == nil && inOrder == 0:
			// El agregado entró a la cuenta DESPUÉS del envío: tendría que haberse rechazado.
			t.Fatalf("vuelta %d: el agregado dijo que entró y no está en el pedido", round)
		case addErr != nil && !errors.Is(addErr, domain.ErrDraftAlreadySent):
			t.Fatalf("vuelta %d: el agregado falló con %v, quería DRAFT_SENT (la tableta abre otra «Nuevo»)", round, addErr)
		}
	}
}
