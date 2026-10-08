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
)

// LA VISTA DEL PEDIDO TRAE SUS PAGOS, SIEMPRE COMO ARREGLO, CON SU NÚMERO ESTABLE.
//
// Se mira el JSON CRUDO: deserializar a un tipo de Go borra la diferencia entre `null` y `[]`, y un
// `null` tumba la hoja de cobro al primer `.map()` sin un solo error en el servidor (AGENTS.md §1).
// El número de cada pago es el que lleva su ticket impreso: un pago devuelto conserva el suyo y sale
// tachado, y los pagos anteriores a la migración se numeran por hora contando los devueltos.
func TestOrderViewCarriesItsPaymentsAsArrays(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	r, token := ordersAPI(t, st, nil)
	cajero, tok := token("cajero_vista_pagos", "cajero")
	session := abrirCajaPrincipal(t, st, cajero)
	efectivo := paymentMethodID(t, st, "Efectivo")
	frappe := makeProduct(t, st, "Frappé de la vista", pesos("50"), true)
	svc := app.NewOrdersService(st, clock)

	newOrder := func() *app.OrderView {
		o, err := svc.Create(ctx, app.CreateOrderCmd{
			ClientUUID: uuid.New(), ServiceType: "mostrador", OpenedBy: cajero,
			Lines: []domain.OrderLineInput{{ProductID: frappe, Qty: pesos("3")}},
		})
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		return o
	}
	get := func(t *testing.T, id int64) map[string]any {
		t.Helper()
		w := do(t, r, http.MethodGet, "/api/v1/orders/"+strconv.FormatInt(id, 10), tok, nil, "")
		if w.Code != http.StatusOK {
			t.Fatalf("GET = %d: %s", w.Code, w.Body.String())
		}
		var m map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &m); err != nil {
			t.Fatal(err)
		}
		return m
	}
	arr := func(t *testing.T, m map[string]any, k string) []any {
		t.Helper()
		v, ok := m[k].([]any)
		if !ok {
			t.Fatalf("%s = %#v, quiere un arreglo (nunca null ni ausente)", k, m[k])
		}
		return v
	}

	t.Run("sin pagos", func(t *testing.T) {
		m := get(t, newOrder().ID)
		if len(arr(t, m, "payments")) != 0 {
			t.Fatalf("payments = %v, quiere []", m["payments"])
		}
		if v, ok := m["mergedIntoOrderId"]; !ok || v != nil {
			t.Fatalf("mergedIntoOrderId = %#v (presente: %v), quiere null presente", v, ok)
		}
		arr(t, m, "lines")
	})

	t.Run("un pago viejo, uno devuelto y uno nuevo con productos", func(t *testing.T) {
		ord := newOrder()
		line := ord.Lines[0].ID
		// El viejo: sin número, como los que ya existen en producción.
		if _, err := svc.Charge(ctx, app.ChargeCmd{OrderID: ord.ID, MethodID: efectivo, Amount: pesos("20"), ActorID: cajero}); err != nil {
			t.Fatalf("Charge: %v", err)
		}
		if _, err := st.Pool.Exec(ctx, `update order_payments set payment_number = null, created_at = now() - interval '2 minutes' where order_id = $1`, ord.ID); err != nil {
			t.Fatal(err)
		}
		// El devuelto: en la bitácora con el número 2.
		if _, err := st.Pool.Exec(ctx, `
			insert into order_payment_voids (company_id, order_id, original_payment_id, payment_number, payment_method_id, amount,
			  tip_amount, register_session_id, paid_at, voided_by, reason)
			values ($1, $2, -10, 2, $3, 15, 0, $4, now() - interval '1 minute', $5, 'Se le cobró a otra persona')`,
			defaultCompanyID, ord.ID, efectivo, session, cajero); err != nil {
			t.Fatal(err)
		}
		// El nuevo: número 3 y una pieza del frappé.
		var pay int64
		if err := st.Pool.QueryRow(ctx, `
			insert into order_payments (company_id, order_id, payment_method_id, amount, tip_amount, register_session_id, received_by, payment_number)
			values ($1, $2, $3, 50, 0, $4, $5, 3) returning id`, defaultCompanyID, ord.ID, efectivo, session, cajero).Scan(&pay); err != nil {
			t.Fatal(err)
		}
		if _, err := st.Pool.Exec(ctx, `insert into order_payment_lines (company_id, order_payment_id, order_line_id, qty, amount)
			values ($1, $2, $3, 1, 50)`, defaultCompanyID, pay, line); err != nil {
			t.Fatal(err)
		}

		pays := arr(t, get(t, ord.ID), "payments")
		if len(pays) != 3 {
			t.Fatalf("payments = %v, quiere 3 (el viejo, el devuelto y el nuevo)", pays)
		}
		type want struct {
			number float64
			voided bool
			amount string
			lines  int
		}
		for i, w := range []want{{1, false, "20", 0}, {2, true, "15", 0}, {3, false, "50", 1}} {
			p := pays[i].(map[string]any)
			lines, ok := p["lines"].([]any)
			if !ok {
				t.Fatalf("pago %d: lines = %#v, quiere arreglo", i, p["lines"])
			}
			if p["number"] != w.number || p["voided"] != w.voided || !pesos(p["amount"].(string)).Equal(pesos(w.amount)) || len(lines) != w.lines {
				t.Errorf("pago %d = número %v, devuelto %v, monto %v, %d productos; quiere %+v", i, p["number"], p["voided"], p["amount"], len(lines), w)
			}
		}
		if p := pays[1].(map[string]any); p["voidReason"] != "Se le cobró a otra persona" || p["voidedAt"] == nil {
			t.Errorf("el devuelto no dice por qué ni cuándo: %v", p)
		}
		if p := pays[2].(map[string]any); p["methodName"] != "Efectivo" {
			t.Errorf("methodName = %v", p["methodName"])
		}
	})
}
