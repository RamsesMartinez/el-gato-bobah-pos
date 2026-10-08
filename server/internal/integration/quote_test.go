//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"testing"

	"uuid"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
)

// paymentRows cuenta, leído como dueño, lo que una cotización NO debe escribir.
func paymentRows(t *testing.T, st *store.Store) string {
	t.Helper()
	var s string
	if err := st.Pool.QueryRow(context.Background(), `
		select (select count(*) from orders) || ':' || (select count(*) from order_payments) || ':' ||
		       (select count(*) from order_payment_lines) || ':' || (select coalesce(sum(total), 0) from orders)`).Scan(&s); err != nil {
		t.Fatal(err)
	}
	return s
}

// LA COTIZACIÓN DICE LO QUE /pay VA A COBRAR, Y NO ESCRIBE NADA.
//
// La pantalla muestra el monto antes de cobrar sin calcularlo ella: si lo calculara, habría dos
// reglas de dinero y tarde o temprano dirían cosas distintas con el cliente enfrente.
func TestQuoteMatchesWhatPayChargesAndWritesNothing(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	r, token := ordersAPI(t, st, nil)
	cashier, tok := token("cajero_cotiza", "cajero")
	abrirCajaPrincipal(t, st, cashier)
	cash := paymentMethodID(t, st, "Efectivo")
	svc := app.NewOrdersService(st, clock)
	frappe := makeProduct(t, st, "Frappé cotizado", pesos("50"), true)
	soda := makeProduct(t, st, "Refresco cotizado", pesos("33.33"), true)
	newOrder := func(platform *int16) *app.OrderView {
		t.Helper()
		cmd := app.CreateOrderCmd{ClientUUID: uuid.New(), ServiceType: "mostrador", OpenedBy: cashier,
			Lines: []domain.OrderLineInput{{ProductID: frappe, Qty: pesos("3")}, {ProductID: soda, Qty: pesos("1")}}}
		if platform != nil {
			cmd.ServiceType, cmd.DeliveryPlatformID = "domicilio", platform
		}
		o, err := svc.Create(ctx, cmd)
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		return o
	}
	post := func(t *testing.T, orderID int64, path string, body map[string]any) (int, map[string]any) {
		t.Helper()
		b, _ := json.Marshal(body)
		w := do(t, r, http.MethodPost, "/api/v1/orders/"+strconv.FormatInt(orderID, 10)+path, tok, b, "application/json")
		var m map[string]any
		_ = json.Unmarshal(w.Body.Bytes(), &m)
		return w.Code, m
	}
	msg := func(m map[string]any) string {
		e, _ := m["error"].(map[string]any)
		s, _ := e["message"].(string)
		return s
	}

	t.Run("con productos y con todo lo que falta, igual que /pay y sin escribir", func(t *testing.T) {
		o := newOrder(nil)
		for _, body := range []map[string]any{
			{"lines": []map[string]any{{"lineId": o.Lines[0].ID, "qty": "1"}}},
			{"allRemaining": true},
		} {
			before := paymentRows(t, st)
			code, q := post(t, o.ID, "/quote", body)
			if code != http.StatusOK {
				t.Fatalf("quote %v = %d %v", body, code, q)
			}
			if after := paymentRows(t, st); after != before {
				t.Fatalf("la cotización escribió: %s → %s", before, after)
			}
			lines, ok := q["lines"].([]any)
			if !ok || q["amount"] == nil || q["outstandingAfter"] == nil {
				t.Fatalf("respuesta %v: quiere amount, lines (arreglo) y outstandingAfter", q)
			}
			payBody := map[string]any{"methodId": cash, "clientUuid": uuid.New().String()}
			for k, v := range body {
				payBody[k] = v
			}
			code, p := post(t, o.ID, "/pay", payBody)
			if code != http.StatusOK || !pesos(p["amount"].(string)).Equal(pesos(q["amount"].(string))) {
				t.Fatalf("/pay cobró %v y la cotización dijo %v (%d)", p["amount"], q["amount"], code)
			}
			if !pesos(p["outstanding"].(string)).Equal(pesos(q["outstandingAfter"].(string))) {
				t.Fatalf("tras cobrar falta %v y la cotización dijo %v", p["outstanding"], q["outstandingAfter"])
			}
			_ = lines
		}
	})

	t.Run("no pide método y las formas se excluyen", func(t *testing.T) {
		o := newOrder(nil)
		code, m := post(t, o.ID, "/quote", map[string]any{"lines": []map[string]any{{"lineId": o.Lines[0].ID, "qty": "1"}}, "allRemaining": true})
		if code != http.StatusBadRequest || msg(m) != "Elige una sola forma de cobrar" {
			t.Fatalf("= %d %q", code, msg(m))
		}
	})

	t.Run("los mismos rechazos que /pay", func(t *testing.T) {
		o := newOrder(nil)
		one := map[string]any{"lines": []map[string]any{{"lineId": o.Lines[0].ID, "qty": "3"}}}
		if code, m := post(t, o.ID, "/pay", map[string]any{"methodId": cash, "lines": one["lines"]}); code != http.StatusOK {
			t.Fatalf("cobrar: %d %v", code, m)
		}
		if code, m := post(t, o.ID, "/quote", one); code != http.StatusConflict || msg(m) != "Ese producto ya se pagó" {
			t.Errorf("pieza cubierta = %d %q", code, msg(m))
		}
		didi := platformID(t, st, defaultCompanyID, "Didi")
		p := newOrder(&didi)
		if code, m := post(t, p.ID, "/quote", map[string]any{"allRemaining": true}); code != http.StatusConflict || msg(m) != "Los pedidos de plataforma no se dividen" {
			t.Errorf("plataforma = %d %q", code, msg(m))
		}
		// Excede: tras un pago por monto, una selección parcial pasa de lo que falta.
		e := newOrder(nil)
		if _, err := svc.Charge(ctx, app.ChargeCmd{OrderID: e.ID, MethodID: cash, Amount: pesos("170"), ActorID: cashier}); err != nil {
			t.Fatal(err)
		}
		code, m := post(t, e.ID, "/quote", map[string]any{"lines": []map[string]any{{"lineId": e.Lines[0].ID, "qty": "1"}}})
		if code != http.StatusBadRequest || msg(m) != "Ya se cobraron $170.00 sin elegir productos. Esta selección pasa de lo que falta: usa «Todo lo que falta»" {
			t.Errorf("excede = %d %q", code, msg(m))
		}
		c := newOrder(nil)
		if _, err := st.Pool.Exec(ctx, `update register_sessions set status = 'cerrada', closed_at = now() where status = 'abierta'`); err != nil {
			t.Fatal(err)
		}
		abrirCajaPrincipal(t, st, cashier)
		if code, m := post(t, c.ID, "/quote", map[string]any{"allRemaining": true}); code != http.StatusConflict || msg(m) != "Ese pedido es de un turno cerrado; no se divide" {
			t.Errorf("turno cerrado = %d %q", code, msg(m))
		}
	})
}

// QUOTE, AISLADO EN LOS TRES CASOS.
func TestQuoteStaysIsolated(t *testing.T) {
	st := newTestStore(t)
	other := makeCompany(t, st, "ajena-cotiza")
	s := newSplitTable(t, st, "cotiza_aislado", "50", "50")
	inTheThreeCases(t, defaultCompanyID, other, func(t *testing.T, st2 *store.Store, c context.Context) {
		svc := app.NewOrdersService(st2, clock)
		if q, err := svc.Quote(c, app.QuoteCmd{OrderID: s.order.ID, AllRemaining: true}); err == nil {
			t.Fatalf("cotizó el pedido de la dueña: %+v", q)
		}
	})
}
