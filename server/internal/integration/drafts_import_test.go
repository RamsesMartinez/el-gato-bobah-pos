//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"

	"uuid"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
)

// LAS PESTAÑAS DE LA VERSIÓN ANTERIOR SUBEN AL SERVIDOR (D-12, FR-018).
//
// Un deploy no puede tirar lo que alguien estaba capturando. Y una pestaña que ya se había mandado a
// cocina —la red se cayó después de que el servidor confirmó— no puede volver como cuenta: mandarla
// otra vez sacaría la comida dos veces.
func TestImportOldTabs(t *testing.T) {
	k := newDraftsKit(t)
	abrirCajaPrincipal(t, k.st, k.user)
	cafe := makeProduct(t, k.st, "Café de la pestaña", pesos("30"), false)
	taro := makeProduct(t, k.st, "Taro de la pestaña", pesos("55"), false)

	tabA, tabB := uuid.New(), uuid.New()
	opA1, opA2, opB := uuid.New(), uuid.New(), uuid.New()
	accounts := []app.ImportAccount{
		{ID: tabA, FolioName: "Persa", Lines: []app.DraftLineCmd{
			{OpID: opA1, ProductID: cafe, Qty: pesos("2")}, {OpID: opA2, ProductID: taro, Qty: pesos("1"), Notes: "sin hielo"}}},
		{ID: tabB, FolioName: "Bombay", Lines: []app.DraftLineCmd{{OpID: opB, ProductID: taro, Qty: pesos("1")}}},
		{ID: uuid.New(), FolioName: "Siamés"}, // pestaña vacía
	}
	res, err := k.drafts.Import(k.ctx, accounts, k.user)
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	want := []string{"created", "created", "skipped_empty"}
	if len(res) != len(want) {
		t.Fatalf("%d resultados para %d pestañas", len(res), len(want))
	}
	for i, r := range res {
		if r.ID != accounts[i].ID || r.Outcome != want[i] {
			t.Fatalf("resultado %d = %+v, quería %s", i, r, want[i])
		}
	}
	a, err := k.drafts.Get(k.ctx, tabA)
	if err != nil {
		t.Fatal(err)
	}
	if *a.FolioName != "Persa" || len(a.Lines) != 2 || !lineQty(t, a, cafe).Equal(pesos("2")) {
		t.Fatalf("la pestaña subió como %v con %d renglones", a.FolioName, len(a.Lines))
	}

	t.Run("reintentar el mismo import no duplica", func(t *testing.T) {
		res, err := k.drafts.Import(k.ctx, accounts, k.user)
		if err != nil {
			t.Fatal(err)
		}
		if res[0].Outcome != "exists" || res[1].Outcome != "exists" {
			t.Fatalf("reintento = %+v, quería exists", res)
		}
		again, _ := k.drafts.Get(k.ctx, tabA)
		if len(again.Lines) != 2 || !lineQty(t, again, cafe).Equal(pesos("2")) {
			t.Fatal("el reintento duplicó renglones")
		}
	})
}

func TestImportOfATabDiscardedElsewhere(t *testing.T) {
	k := newDraftsKit(t)
	cafe := makeProduct(t, k.st, "Café descartado en otra", pesos("30"), false)
	tab := app.ImportAccount{ID: uuid.New(), Lines: []app.DraftLineCmd{addOf(cafe, "1")}}
	if _, err := k.drafts.Import(k.ctx, []app.ImportAccount{tab}, k.user); err != nil {
		t.Fatal(err)
	}
	if _, err := k.st.Pool.Exec(context.Background(), `update order_drafts set status = 'descartada', discarded_at = now(),
		discard_reason = 'manual' where id = $1`, tab.ID); err != nil {
		t.Fatal(err)
	}
	// La tableta vieja no alcanzó a borrar su copia y vuelve a subir: no se resucita ni tumba la subida.
	res, err := k.drafts.Import(k.ctx, []app.ImportAccount{tab}, k.user)
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if len(res) != 1 || res[0].Outcome != "exists" || draftStatus(t, k.st, tab.ID) != domain.DraftDiscarded {
		t.Fatalf("= %+v; la cuenta quedó %s", res, draftStatus(t, k.st, tab.ID))
	}
}

func TestImportDoesNotResendASentTab(t *testing.T) {
	k := newDraftsKit(t)
	abrirCajaPrincipal(t, k.st, k.user)
	cafe := makeProduct(t, k.st, "Café ya enviado", pesos("30"), false)

	// Pestaña 1: nació como pedido (client_uuid = id de la pestaña).
	sentTab := uuid.New()
	ord, err := k.orders.Create(k.ctx, app.CreateOrderCmd{ClientUUID: sentTab, ServiceType: "mostrador", OpenedBy: k.user,
		Lines: []domain.OrderLineInput{{ProductID: cafe, Qty: pesos("1")}}})
	if err != nil {
		t.Fatal(err)
	}
	// Pestaña 2: era un agregado a ese pedido que ya entró (order_line_batches.client_uuid).
	addTab := uuid.New()
	if _, err := k.orders.AddLines(k.ctx, ord.ID, []domain.OrderLineInput{{ProductID: cafe, Qty: pesos("1")}}, k.user, addTab); err != nil {
		t.Fatal(err)
	}
	res, err := k.drafts.Import(k.ctx, []app.ImportAccount{
		{ID: sentTab, FolioName: "Persa", Lines: []app.DraftLineCmd{addOf(cafe, "1")}},
		{ID: addTab, FolioName: "Bombay", Lines: []app.DraftLineCmd{addOf(cafe, "1")}},
	}, k.user)
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 2 {
		t.Fatalf("%d resultados para 2 pestañas: la tableta no sabría si borrar lo suyo", len(res))
	}
	for i, r := range res {
		if r.Outcome != "already_sent" || r.OrderID == nil || *r.OrderID != ord.ID || r.DraftID != nil {
			t.Fatalf("pestaña %d = %+v: ya se había mandado y volvería a cocina", i, r)
		}
	}
	var drafts int
	if err := k.st.Pool.QueryRow(context.Background(), `select count(*) from order_drafts`).Scan(&drafts); err != nil {
		t.Fatal(err)
	}
	if drafts != 0 {
		t.Fatalf("quedaron %d cuentas vivas de pestañas ya enviadas", drafts)
	}
}

func TestImportLimits(t *testing.T) {
	k := newDraftsKit(t)
	cafe := makeProduct(t, k.st, "Café del tope de pestañas", pesos("30"), false)
	many := make([]app.ImportAccount, 21)
	for i := range many {
		many[i] = app.ImportAccount{ID: uuid.New(), Lines: []app.DraftLineCmd{addOf(cafe, "1")}}
	}
	if _, err := k.drafts.Import(k.ctx, many, k.user); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("21 pestañas = %v, quería 400", err)
	}
	var n int
	if err := k.st.Pool.QueryRow(context.Background(), `select count(*) from order_drafts`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("el rechazo dejó %d cuentas a medias", n)
	}
}

func TestImportInTheThreeCases(t *testing.T) {
	k := newDraftsKit(t)
	abrirCajaPrincipal(t, k.st, k.user)
	other := makeCompany(t, k.st, "otra-import")
	otherUser := makeUserIn(t, k.st, other, "cajero_otra_import", "cajero")
	cafe := makeProduct(t, k.st, "Café importado", pesos("30"), false)
	// Una pestaña de la dueña que ya se mandó como pedido.
	sentTab := uuid.New()
	if _, err := k.orders.Create(k.ctx, app.CreateOrderCmd{ClientUUID: sentTab, ServiceType: "mostrador", OpenedBy: k.user,
		Lines: []domain.OrderLineInput{{ProductID: cafe, Qty: pesos("1")}}}); err != nil {
		t.Fatal(err)
	}
	inTheThreeCases(t, k.company, other, func(t *testing.T, st *store.Store, ctx context.Context) {
		svc := app.NewDraftsService(st, app.NewOrdersService(st, clock))
		res, err := svc.Import(ctx, []app.ImportAccount{{ID: sentTab, Lines: []app.DraftLineCmd{addOf(cafe, "1")}}}, otherUser)
		if err == nil && len(res) == 1 && res[0].OrderID != nil {
			t.Fatalf("la otra empresa vio el pedido de la dueña: %+v", res[0])
		}
	})
}
