//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"uuid"

	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
)

// liveKit es draftsKit con el servicio de la fila de cuentas.
type liveKit struct {
	*draftsKit
	accounts *app.AccountsService
	efectivo int16
	product  int64
}

func newLiveKit(t *testing.T) *liveKit {
	t.Helper()
	k := &liveKit{draftsKit: newDraftsKit(t)}
	k.accounts = app.NewAccountsService(k.appSt, k.orders)
	k.efectivo = paymentMethodID(t, k.st, "Efectivo")
	k.product = makeProduct(t, k.st, "Café de la fila", pesos("100"), false)
	return k
}

// order crea un pedido de $100 (o de `qty` cafés), lo cobra con `paid` y lo entrega si se pide.
func (k *liveKit) order(t *testing.T, qty, paid string, deliver bool) *app.OrderView {
	t.Helper()
	o, err := k.orders.Create(k.ctx, app.CreateOrderCmd{ClientUUID: uuid.New(), ServiceType: "mostrador", OpenedBy: k.user,
		Lines: []domain.OrderLineInput{{ProductID: k.product, Qty: pesos(qty)}}})
	if err != nil {
		t.Fatal(err)
	}
	if !pesos(paid).IsZero() {
		if _, err := k.orders.Charge(k.ctx, app.ChargeCmd{OrderID: o.ID, MethodID: k.efectivo, Amount: pesos(paid), ActorID: k.user}); err != nil {
			t.Fatal(err)
		}
	}
	if deliver {
		if err := k.orders.DeliverAll(k.ctx, o.ID); err != nil {
			t.Fatal(err)
		}
	}
	return o
}

func (k *liveKit) ageOrder(t *testing.T, id int64, days int) {
	t.Helper()
	if _, err := k.st.Pool.Exec(context.Background(), `update orders set business_date = $2 where id = $1`,
		id, fixedNow.AddDate(0, 0, -days)); err != nil {
		t.Fatal(err)
	}
}

func (k *liveKit) live(t *testing.T, ctx context.Context, olderDebts bool) (*app.LiveAccounts, map[string]app.AccountItem) {
	t.Helper()
	res, err := k.accounts.Live(ctx, olderDebts)
	if err != nil {
		t.Fatalf("Live: %v", err)
	}
	byKey := map[string]app.AccountItem{}
	for _, it := range res.Items {
		byKey[it.Key] = it
	}
	return res, byKey
}

func orderKey(id int64) string     { return "o:" + itoa(int(id)) }
func draftKey(id uuid.UUID) string { return "d:" + id.String() }

// LA FILA TIENE TODA CUENTA VIVA, CON SU ESTADO (US1, FR-009, casos 1–4 del lienzo).
func TestLiveAccountsStates(t *testing.T) {
	t.Parallel()
	k := newLiveKit(t)
	abrirCajaPrincipal(t, k.st, k.user)

	capturing := k.newDraft(t, addOf(k.product, "5")) // $500 que nadie debe todavía
	inKitchen := k.order(t, "1", "0", false)
	paidInKitchen := k.order(t, "1", "100", false)
	partlyPaid := k.order(t, "1", "40", false)
	if err := k.orders.SetStatus(k.ctx, partlyPaid.ID, domain.StatusLista); err != nil {
		t.Fatal(err)
	}
	owesToday := k.order(t, "1", "30", true)
	owesTenDays := k.order(t, "1", "0", true)
	k.ageOrder(t, owesTenDays.ID, 10)
	owesHundredDays := k.order(t, "1", "0", true)
	k.ageOrder(t, owesHundredDays.ID, 100)
	closed := k.order(t, "1", "100", true)
	cancelled := k.order(t, "1", "0", false)
	if err := k.orders.Cancel(k.ctx, cancelled.ID, k.user, "prueba"); err != nil {
		t.Fatal(err)
	}
	uber := platformID(t, k.st, defaultCompanyID, "Uber Eats")
	platform, err := k.orders.Create(k.ctx, app.CreateOrderCmd{ClientUUID: uuid.New(), ServiceType: "domicilio", OpenedBy: k.user,
		DeliveryPlatformID: &uber, Lines: []domain.OrderLineInput{{ProductID: k.product, Qty: pesos("1")}}})
	if err != nil {
		t.Fatal(err)
	}

	res, byKey := k.live(t, k.ctx, false)
	cases := []struct {
		name, key, state, group, outstanding string
		present                              bool
	}{
		{"capturando", draftKey(capturing.ID), domain.AccountCapturing, domain.GroupCapturing, "500", true},
		{"en cocina", orderKey(inKitchen.ID), domain.AccountInKitchen, domain.GroupInKitchen, "100", true},
		{"pagada en cocina (caso 2)", orderKey(paidInKitchen.ID), domain.AccountPaidInKitchen, domain.GroupInKitchen, "0", true},
		{"pago parcial, lista", orderKey(partlyPaid.ID), domain.AccountPartlyPaid, domain.GroupInKitchen, "60", true},
		{"entregada que debe, de hoy", orderKey(owesToday.ID), domain.AccountDeliveredOwes, domain.GroupDeliveredOwes, "70", true},
		{"entregada que debe, de hace 10 días (caso 3)", orderKey(owesTenDays.ID), domain.AccountDeliveredOwes, domain.GroupPreviousDays, "100", true},
		{"de plataforma", orderKey(platform.ID), domain.AccountInKitchen, domain.GroupInKitchen, platform.Total.String(), true},
		{"pagada y entregada (caso 4: cerrada)", orderKey(closed.ID), "", "", "", false},
		{"cancelada", orderKey(cancelled.ID), "", "", "", false},
		{"entregada que debe de hace 100 días: solo con olderDebts", orderKey(owesHundredDays.ID), "", "", "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			it, ok := byKey[c.key]
			if ok != c.present {
				t.Fatalf("presente = %v, quería %v", ok, c.present)
			}
			if !ok {
				return
			}
			if it.State != c.state || it.Group != c.group || !it.Outstanding.Equal(pesos(c.outstanding)) {
				t.Fatalf("state=%s group=%s outstanding=%s, quería %s %s %s", it.State, it.Group, it.Outstanding, c.state, c.group, c.outstanding)
			}
		})
	}
	if it := byKey[orderKey(partlyPaid.ID)]; !it.KitchenReady {
		t.Error("el pedido en `lista` no dice kitchenReady")
	}
	if it := byKey[draftKey(capturing.ID)]; it.Kind != "draft" || it.Number != nil || it.BusinessDate != nil || !it.Paid.IsZero() || it.LineCount != 1 {
		t.Errorf("la cuenta en captura = %+v", it)
	}

	t.Run("el total que se debe no cuenta la cuenta en captura", func(t *testing.T) {
		want := decimal.Zero
		for _, it := range res.Items {
			if it.Kind == "order" {
				want = want.Add(it.Outstanding)
			}
		}
		if !res.Outstanding.Equal(want) {
			t.Fatalf("outstanding = %s, la suma de los pedidos es %s: la cuenta en captura se contó como deuda", res.Outstanding, want)
		}
		if expected := pesos("330").Add(platform.Total); !want.Equal(expected) {
			t.Fatalf("la suma de los pedidos es %s, quería %s (100+60+70+100+plataforma)", want, expected)
		}
	})

	t.Run("con olderDebts aparece la deuda de hace 100 días", func(t *testing.T) {
		_, all := k.live(t, k.ctx, true)
		it, ok := all[orderKey(owesHundredDays.ID)]
		if !ok || it.Group != domain.GroupPreviousDays {
			t.Fatalf("presente=%v group=%s", ok, it.Group)
		}
	})

	t.Run("ordenadas por antigüedad", func(t *testing.T) {
		for i := 1; i < len(res.Items); i++ {
			if res.Items[i].OpenedAt.Before(res.Items[i-1].OpenedAt) {
				t.Fatalf("el %d abrió antes que el %d", i, i-1)
			}
		}
	})
}

func TestLiveAccountsPendingNew(t *testing.T) {
	t.Parallel()
	k := newLiveKit(t)
	abrirCajaPrincipal(t, k.st, k.user)
	inKitchen := k.order(t, "1", "0", false)
	nuevo, _, err := k.drafts.Create(k.ctx, app.CreateDraftCmd{ID: uuid.New(), OrderID: &inKitchen.ID,
		Lines: []app.DraftLineCmd{addOf(k.product, "1"), {OpID: uuid.New(), ProductID: k.product, Qty: pesos("1"), Notes: "tibio"}}, Actor: k.user})
	if err != nil {
		t.Fatal(err)
	}
	_, byKey := k.live(t, k.ctx, false)
	it := byKey[orderKey(inKitchen.ID)]
	if it.PendingDraftID == nil || *it.PendingDraftID != nuevo.ID || it.PendingCount != 2 {
		t.Fatalf("pendingDraftId=%v pendingCount=%d: la fila no dice que hay algo sin mandar", it.PendingDraftID, it.PendingCount)
	}
	if _, ok := byKey[draftKey(nuevo.ID)]; ok {
		t.Fatal("lo «Nuevo» de un pedido salió como cuenta aparte: dos fichas para la misma mesa")
	}

	t.Run("pedido que se cerró con su «Nuevo» viva sigue en la fila (R-9)", func(t *testing.T) {
		if _, err := k.orders.Charge(k.ctx, app.ChargeCmd{OrderID: inKitchen.ID, MethodID: k.efectivo, Amount: pesos("100"), ActorID: k.user}); err != nil {
			t.Fatal(err)
		}
		if err := k.orders.DeliverAll(k.ctx, inKitchen.ID); err != nil {
			t.Fatal(err)
		}
		_, byKey := k.live(t, k.ctx, false)
		it, ok := byKey[orderKey(inKitchen.ID)]
		if !ok || !it.ClosedWithPending || it.PendingDraftID == nil {
			t.Fatalf("presente=%v %+v: lo capturado quedaría en una cuenta que nadie ve", ok, it)
		}
	})
}

func TestLiveAccountsSweepsBeforeListing(t *testing.T) {
	t.Parallel()
	k := newLiveKit(t)
	old := k.newDraft(t, addOf(k.product, "1"))
	if _, err := k.st.Pool.Exec(context.Background(), `update order_drafts set updated_at = now() - interval '13 hours' where id = $1`, old.ID); err != nil {
		t.Fatal(err)
	}
	_, byKey := k.live(t, k.ctx, false)
	if _, ok := byKey[draftKey(old.ID)]; ok {
		t.Fatal("la cuenta de 13 h sigue en la fila")
	}
	if got := draftStatus(t, k.st, old.ID); got != domain.DraftDiscarded {
		t.Fatalf("la cuenta de 13 h quedó %s", got)
	}
}

func TestLiveAccountsInTheThreeCases(t *testing.T) {
	t.Parallel()
	k := newLiveKit(t)
	abrirCajaPrincipal(t, k.st, k.user)
	other := makeCompany(t, k.st, "otra-fila")
	d := k.newDraft(t, addOf(k.product, "1"))
	o := k.order(t, "1", "0", false)
	inTheThreeCases(t, k.company, other, func(t *testing.T, st *store.Store, ctx context.Context) {
		svc := app.NewAccountsService(st, app.NewOrdersService(st, clock))
		res, err := svc.Live(ctx, true)
		if err != nil {
			return // sin empresa puede negarse; lo que no puede es mostrar algo de la dueña
		}
		for _, it := range res.Items {
			if it.Key == draftKey(d.ID) || it.Key == orderKey(o.ID) {
				t.Fatalf("la fila de otra empresa trae %s", it.Key)
			}
		}
		if !res.Outstanding.IsZero() {
			t.Fatalf("outstanding = %s fuera de la empresa", res.Outstanding)
		}
	})
	if got := draftStatus(t, k.st, d.ID); got != domain.DraftCapturing {
		t.Fatalf("un barrido de otra empresa tocó la cuenta de la dueña: %s", got)
	}
}

// EL TURNO QUE CRUZA LA MEDIANOCHE NO PUEDE VACIAR LA FILA (venía de pedidos_en_curso_test.go).
//
// El servidor corre en UTC y el local cierra a las 22:00 hora de México: la medianoche UTC cae a las
// 18:00 locales. La barra filtraba por la fecha del servidor y se vaciaba TODAS las noches en plena
// hora pico. Lo que sigue en cocina se ve sin filtro de fecha.
func TestLiveAccountsSurviveMidnight(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()
	ayer := app.NewOrdersService(st, clock)
	hoy := app.NewOrdersService(st, func() time.Time { return fixedNow.Add(26 * time.Hour) })
	cajero := makeUser(t, st, "cajero_medianoche", "cajero")
	prod := makeProduct(t, st, "Café medianoche", pesos("100"), false)
	abrirCajaPrincipal(t, st, cajero)
	ord, err := ayer.Create(ctx, app.CreateOrderCmd{ClientUUID: uuid.New(), ServiceType: "mostrador", OpenedBy: cajero,
		Lines: []domain.OrderLineInput{{ProductID: prod, Qty: pesos("1")}}})
	if err != nil {
		t.Fatal(err)
	}
	res, err := app.NewAccountsService(st, hoy).Live(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range res.Items {
		if it.OrderID != nil && *it.OrderID == ord.ID {
			return
		}
	}
	t.Error("el pedido del turno abierto desapareció de la fila al cruzar la medianoche")
}
