//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"testing"

	"uuid"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
)

// LA MISMA PARTE NO SE COBRA DOS VECES, Y LAS PARTES SOBREVIVEN A RECARGAR LA TABLETA.
//
// La llave del cobro cubre el reenvío de la misma llamada, pero no a quien rota la llave al cambiar
// de intención: «cobrar 2 de 3» dos veces cobraría a la misma persona dos veces. Y las partes viven
// en el servidor, no en la memoria de la hoja: tras recargar, la parte que falta sigue siendo la que
// falta y la última absorbe el centavo.
func TestTheSameSplitPartCannotBeChargedTwiceAndSurvivesAReload(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	s := newSplitTable(t, st, "partes", "100")
	charge := func(part int) (*app.ChargeResult, error) {
		return s.svc.Charge(s.ctx, app.ChargeCmd{OrderID: s.order.ID, MethodID: s.cash, ActorID: s.cashier, ClientUUID: uuid.New(),
			Split: &app.ChargeSplit{Part: part, Of: 3}})
	}
	first, err := charge(1)
	if err != nil || !first.Amount.Equal(pesos("33.33")) {
		t.Fatalf("parte 1 = %v %v, quiere 33.33", first, err)
	}
	if _, err := charge(1); !errors.Is(err, domain.ErrSplitPartAlreadyCharged) {
		t.Fatalf("parte 1 otra vez = %v, quiere «Esa parte ya se cobró»", err)
	}
	// «Recargar» es volver a leer el pedido: las partes cobradas salen del servidor.
	v, err := s.svc.Detail(s.ctx, s.order.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(v.Payments) != 1 || v.Payments[0].Split == nil || v.Payments[0].Split.Part != 1 || v.Payments[0].Split.Of != 3 {
		t.Fatalf("tras recargar los pagos dicen %+v; quiere la parte 1 de 3", v.Payments)
	}
	third, err := charge(3)
	if err != nil || !third.Amount.Equal(pesos("33.33")) {
		t.Fatalf("parte 3 = %v %v, quiere 33.33", third, err)
	}
	second, err := charge(2)
	if err != nil || !second.Amount.Equal(pesos("33.34")) || !second.Paid {
		t.Fatalf("la última parte = %v %v, quiere 33.34 y el pedido saldado", second, err)
	}
}

// EL CONTRATO DE /pay Y /quote CON PARTES.
func TestPayBySplitContract(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()
	r, token := ordersAPI(t, st, nil)
	cashier, tok := token("cajero_partes_http", "cajero")
	abrirCajaPrincipal(t, st, cashier)
	cash := paymentMethodID(t, st, "Efectivo")
	frappe := makeProduct(t, st, "Frappé por partes", pesos("50"), true)
	svc := app.NewOrdersService(st, clock)
	newOrder := func(platform *int16) *app.OrderView {
		t.Helper()
		cmd := app.CreateOrderCmd{ClientUUID: uuid.New(), ServiceType: "mostrador", OpenedBy: cashier,
			Lines: []domain.OrderLineInput{{ProductID: frappe, Qty: pesos("2")}}}
		if platform != nil {
			cmd.ServiceType, cmd.DeliveryPlatformID = "domicilio", platform
		}
		o, err := svc.Create(ctx, cmd)
		if err != nil {
			t.Fatal(err)
		}
		return o
	}
	post := func(t *testing.T, id int64, path string, body map[string]any) (int, map[string]any) {
		t.Helper()
		b, _ := json.Marshal(body)
		w := do(t, r, http.MethodPost, "/api/v1/orders/"+strconv.FormatInt(id, 10)+path, tok, b, "application/json")
		var m map[string]any
		_ = json.Unmarshal(w.Body.Bytes(), &m)
		return w.Code, m
	}
	msg := func(m map[string]any) string {
		e, _ := m["error"].(map[string]any)
		s, _ := e["message"].(string)
		return s
	}
	split := map[string]any{"part": 1, "of": 3}

	t.Run("las partes no se mezclan con otra forma", func(t *testing.T) {
		o := newOrder(nil)
		for _, body := range []map[string]any{
			{"methodId": cash, "split": split, "amount": "10"},
			{"methodId": cash, "split": split, "allRemaining": true},
			{"methodId": cash, "split": split, "lines": []map[string]any{{"lineId": o.Lines[0].ID, "qty": "1"}}},
		} {
			if code, m := post(t, o.ID, "/pay", body); code != http.StatusBadRequest || msg(m) != "Elige una sola forma de cobrar" {
				t.Errorf("%v = %d %q", body, code, msg(m))
			}
		}
	})

	t.Run("la cotización de una parte es lo que /pay cobra", func(t *testing.T) {
		o := newOrder(nil)
		_, q := post(t, o.ID, "/quote", map[string]any{"split": split})
		code, p := post(t, o.ID, "/pay", map[string]any{"methodId": cash, "split": split})
		if code != http.StatusOK || q["amount"] == nil || !pesos(p["amount"].(string)).Equal(pesos(q["amount"].(string))) {
			t.Fatalf("quote %v, pay %d %v", q, code, p)
		}
		if lines, ok := q["lines"].([]any); !ok || len(lines) != 0 {
			t.Fatalf("una parte no cubre productos: lines = %#v", q["lines"])
		}
	})

	t.Run("plataforma y turno cerrado", func(t *testing.T) {
		didi := platformID(t, st, defaultCompanyID, "Didi")
		p := newOrder(&didi)
		if code, m := post(t, p.ID, "/pay", map[string]any{"methodId": cash, "split": split}); code != http.StatusConflict || msg(m) != "Los pedidos de plataforma no se dividen" {
			t.Errorf("plataforma = %d %q", code, msg(m))
		}
		c := newOrder(nil)
		if _, err := st.Pool.Exec(ctx, `update register_sessions set status = 'cerrada', closed_at = now() where status = 'abierta'`); err != nil {
			t.Fatal(err)
		}
		abrirCajaPrincipal(t, st, cashier)
		if code, m := post(t, c.ID, "/pay", map[string]any{"methodId": cash, "split": split}); code != http.StatusConflict || msg(m) != "Ese pedido es de un turno cerrado; no se divide" {
			t.Errorf("turno cerrado = %d %q", code, msg(m))
		}
	})
}
