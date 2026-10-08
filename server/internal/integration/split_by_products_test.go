//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"testing"

	"uuid"

	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
)

// splitTable arma un pedido de mostrador con un renglón de una pieza por cada precio, bajo el rol
// de la aplicación, y devuelve el servicio con su contexto de empresa.
type splitTable struct {
	st      *store.Store
	svc     *app.OrdersService
	ctx     context.Context
	cashier int64
	cash    int16
	order   *app.OrderView
}

func newSplitTable(t *testing.T, st *store.Store, suffix string, prices ...string) *splitTable {
	t.Helper()
	ctx := context.Background()
	cashier := makeUser(t, st, "cajero_"+suffix, "cajero")
	abrirCajaPrincipal(t, st, cashier)
	appSt := appRoleStore(t)
	tctx, release, err := appSt.AcquireTenant(ctx, defaultCompanyID)
	if err != nil {
		t.Fatalf("AcquireTenant: %v", err)
	}
	t.Cleanup(release)
	svc := app.NewOrdersService(appSt, clock)
	lines := make([]domain.OrderLineInput, len(prices))
	for i, p := range prices {
		lines[i] = domain.OrderLineInput{ProductID: makeProduct(t, st, fmt.Sprintf("Producto %s %d", suffix, i+1), pesos(p), true), Qty: pesos("1")}
	}
	ord, err := svc.Create(tctx, app.CreateOrderCmd{ClientUUID: uuid.New(), ServiceType: "mostrador", OpenedBy: cashier, Lines: lines})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	return &splitTable{st: st, svc: svc, ctx: tctx, cashier: cashier, cash: paymentMethodID(t, st, "Efectivo"), order: ord}
}

// pay cobra por productos los renglones de las posiciones dadas, una pieza de cada uno.
func (s *splitTable) pay(t *testing.T, positions ...int) *app.ChargeResult {
	t.Helper()
	sel := make([]domain.SelectedPieces, len(positions))
	for i, p := range positions {
		sel[i] = domain.SelectedPieces{LineID: s.order.Lines[p].ID, Qty: pesos("1")}
	}
	res, err := s.svc.Charge(s.ctx, app.ChargeCmd{OrderID: s.order.ID, MethodID: s.cash, Lines: sel, ActorID: s.cashier, ClientUUID: uuid.New()})
	if err != nil {
		t.Fatalf("cobrar %v: %v", positions, err)
	}
	return res
}

func (s *splitTable) payRest(t *testing.T) *app.ChargeResult {
	t.Helper()
	res, err := s.svc.Charge(s.ctx, app.ChargeCmd{OrderID: s.order.ID, MethodID: s.cash, AllRemaining: true, ActorID: s.cashier, ClientUUID: uuid.New()})
	if err != nil {
		t.Fatalf("«Todo lo que falta»: %v", err)
	}
	return res
}

// coverage devuelve, leído como dueño, la suma de lo cubierto por pago: piezas y dinero.
func coverageOf(t *testing.T, st *store.Store, paymentID int64) (decimal.Decimal, decimal.Decimal) {
	t.Helper()
	var qty, amount decimal.Decimal
	if err := st.Pool.QueryRow(context.Background(),
		`select coalesce(sum(qty), 0), coalesce(sum(amount), 0) from order_payment_lines where order_payment_id = $1`, paymentID).
		Scan(&qty, &amount); err != nil {
		t.Fatal(err)
	}
	return qty, amount
}

// LA MESA DEL INCIDENTE DEL 2026-10-04 SE DIVIDE SIN CANCELAR NADA.
//
// Tres personas, once productos: una paga uno, otra cuatro y la última «Todo lo que falta». Ese día
// se resolvió quitando renglones con un motivo falso y recapturándolos: 18 cancelaciones que no
// ocurrieron, el inventario descontado dos veces y un pedido que no se podía cerrar.
func TestTheIncidentTableSplitsWithoutCancellingAnything(t *testing.T) {
	st := newTestStore(t)
	s := newSplitTable(t, st, "incidente", "110", "65", "65", "85", "95", "40", "40", "75", "120", "55", "57")
	products := make([]int64, len(s.order.Lines))
	before := make([]decimal.Decimal, len(s.order.Lines))
	for i, l := range s.order.Lines {
		if err := st.Pool.QueryRow(context.Background(), `select product_id from order_lines where id = $1`, l.ID).Scan(&products[i]); err != nil {
			t.Fatal(err)
		}
		before[i] = existencias(t, st, products[i])
	}

	first := s.pay(t, 0)
	second := s.pay(t, 1, 2, 3, 4)
	last := s.payRest(t)

	if !first.Amount.Equal(pesos("110")) || !second.Amount.Equal(pesos("310")) {
		t.Fatalf("pagos de %s y %s, quiere 110 y 310: cada quien paga lo suyo", first.Amount, second.Amount)
	}
	if got := first.Amount.Add(second.Amount).Add(last.Amount); !got.Equal(s.order.Total) {
		t.Fatalf("los tres pagos suman %s y el pedido vale %s", got, s.order.Total)
	}
	if !last.Paid || !last.Outstanding.IsZero() {
		t.Fatalf("tras «Todo lo que falta» el pedido debe quedar saldado: %+v", last)
	}
	for i, r := range []*app.ChargeResult{first, second, last} {
		if r.Number != i+1 || r.PaymentID == 0 {
			t.Errorf("pago %d: número %d, id %d", i+1, r.Number, r.PaymentID)
		}
	}
	for i, r := range []*app.ChargeResult{first, second, last} {
		qty, amount := coverageOf(t, st, r.PaymentID)
		if want := []string{"1", "4", "6"}[i]; !qty.Equal(pesos(want)) || !amount.Equal(r.Amount) {
			t.Errorf("pago %d cubre %s piezas por %s; quiere %s piezas por %s", i+1, qty, amount, want, r.Amount)
		}
	}

	var cancelled, restocks int
	if err := st.Pool.QueryRow(context.Background(), `
		select (select count(*) from order_lines where order_id = $1 and cancelled_at is not null),
		       (select count(*) from stock_movements where order_id = $1 and movement_type <> 'venta')`, s.order.ID).
		Scan(&cancelled, &restocks); err != nil {
		t.Fatal(err)
	}
	if cancelled != 0 || restocks != 0 {
		t.Fatalf("%d productos cancelados y %d movimientos que no son venta: dividir no cancela nada", cancelled, restocks)
	}
	for i, p := range products {
		if got := existencias(t, st, p); !got.Equal(before[i]) {
			t.Errorf("producto %d: existencias %s → %s; se vendió una vez al crear el pedido y cobrar no las mueve", i+1, before[i], got)
		}
	}
}

// «TODO LO QUE FALTA» SIN PIEZAS POR CUBRIR COBRA EL SALDO.
//
// En operación aparece al devolver un pago por monto: todas las piezas están cubiertas y aún se
// debe dinero. Rechazarlo como «selección vacía» dejaría el pedido sin forma de saldarse.
func TestAllRemainingWithNothingUncoveredChargesTheBalance(t *testing.T) {
	st := newTestStore(t)
	s := newSplitTable(t, st, "saldo_sin_piezas", "50", "50")
	ctx := context.Background()
	var session int64
	if err := st.Pool.QueryRow(ctx, `select id from register_sessions where status = 'abierta'`).Scan(&session); err != nil {
		t.Fatal(err)
	}
	var pay int64
	if err := st.Pool.QueryRow(ctx, `
		insert into order_payments (company_id, order_id, payment_method_id, amount, tip_amount, register_session_id, payment_number)
		values ($1, $2, $3, 60, 0, $4, 1) returning id`, defaultCompanyID, s.order.ID, s.cash, session).Scan(&pay); err != nil {
		t.Fatal(err)
	}
	for _, l := range s.order.Lines {
		if _, err := st.Pool.Exec(ctx, `insert into order_payment_lines (company_id, order_payment_id, order_line_id, qty, amount) values ($1, $2, $3, 1, 30)`,
			defaultCompanyID, pay, l.ID); err != nil {
			t.Fatal(err)
		}
	}
	res := s.payRest(t)
	if !res.Amount.Equal(pesos("40")) || !res.Paid {
		t.Fatalf("cobró %s (saldado %v), quiere los 40 que faltan", res.Amount, res.Paid)
	}
	if qty, _ := coverageOf(t, st, res.PaymentID); !qty.IsZero() {
		t.Fatalf("cubrió %s piezas: ya no había ninguna sin cubrir", qty)
	}
}

// CON DESCUENTO, LOS PAGOS POR PRODUCTOS SUMAN EL TOTAL AL CENTAVO.
//
// El descuento se reparte en proporción a lo que paga cada quien y el último absorbe el centavo del
// redondeo. Si no, la suma de los pagos pasaría del total por el descuento o quedaría un centavo
// que nadie paga.
func TestDiscountedSplitAddsUpToTheTotal(t *testing.T) {
	st := newTestStore(t)
	s := newSplitTable(t, st, "descuento_partido", "300", "307", "300")
	fifty := pesos("50")
	if _, err := s.svc.SetDiscount(s.ctx, app.SetDiscountCmd{OrderID: s.order.ID, Amount: &fifty, Actor: s.cashier}); err != nil {
		t.Fatalf("SetDiscount: %v", err)
	}
	var total decimal.Decimal
	if err := st.Pool.QueryRow(context.Background(), `select total from orders where id = $1`, s.order.ID).Scan(&total); err != nil {
		t.Fatal(err)
	}
	if !total.Equal(pesos("857")) {
		t.Fatalf("total %s, quiere 857", total)
	}
	pays := []*app.ChargeResult{s.pay(t, 0), s.pay(t, 1), s.payRest(t)}
	sum := decimal.Zero
	for i, p := range pays {
		sum = sum.Add(p.Amount)
		if _, covered := coverageOf(t, st, p.PaymentID); !covered.Equal(p.Amount) {
			t.Errorf("pago %d: lo cubierto por producto suma %s y el pago es de %s", i+1, covered, p.Amount)
		}
	}
	if !sum.Equal(total) {
		t.Fatalf("los pagos suman %s y el total es %s: se %s %s del descuento", sum, total,
			map[bool]string{true: "duplicó", false: "perdió"}[sum.GreaterThan(total)], sum.Sub(total).Abs())
	}
}

// [H6] TRAS UN PAGO POR MONTO, «TODO LO QUE FALTA» PRORRATEA LO CUBIERTO.
//
// Cobra menos que el bruto de lo que cubre, porque parte ya entró sin elegir productos. Si cada
// renglón dijera su bruto, lo cubierto por producto sumaría más que el pago y el reporte por
// producto no cuadraría con el corte.
func TestAllRemainingAfterAnAmountPaymentProratesCoverage(t *testing.T) {
	st := newTestStore(t)
	m := newSplitTable(t, st, "monto_y_resto", "100", "100", "100")
	if _, err := m.svc.Charge(m.ctx, app.ChargeCmd{OrderID: m.order.ID, MethodID: m.cash, Amount: pesos("50"), ActorID: m.cashier}); err != nil {
		t.Fatalf("pago por monto: %v", err)
	}
	m.pay(t, 0)
	rest := m.payRest(t)
	if !rest.Amount.Equal(pesos("150")) {
		t.Fatalf("«Todo lo que falta» cobró %s, quiere 150", rest.Amount)
	}
	if _, covered := coverageOf(t, st, rest.PaymentID); !covered.Equal(rest.Amount) {
		t.Fatalf("lo cubierto por producto suma %s y el pago es de %s: el reporte por producto no cuadraría con el corte", covered, rest.Amount)
	}
}

// EL CONTRATO DE /pay CON PRODUCTOS.
func TestPayByProductsContract(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	r, token := ordersAPI(t, st, nil)
	cashier, tok := token("cajero_contrato_pago", "cajero")
	session := abrirCajaPrincipal(t, st, cashier)
	cash := paymentMethodID(t, st, "Efectivo")
	frappe := makeProduct(t, st, "Frappé del contrato", pesos("50"), true)
	svc := app.NewOrdersService(st, clock)
	newOrder := func(cmd app.CreateOrderCmd) *app.OrderView {
		t.Helper()
		if cmd.ServiceType == "" {
			cmd.ServiceType = "mostrador"
		}
		cmd.ClientUUID, cmd.OpenedBy = uuid.New(), cashier
		cmd.Lines = []domain.OrderLineInput{{ProductID: frappe, Qty: pesos("3")}}
		o, err := svc.Create(ctx, cmd)
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		return o
	}
	pay := func(t *testing.T, orderID int64, body map[string]any) (int, map[string]any) {
		t.Helper()
		if _, ok := body["methodId"]; !ok {
			body["methodId"] = cash
		}
		b, _ := json.Marshal(body)
		w := do(t, r, http.MethodPost, "/api/v1/orders/"+strconv.FormatInt(orderID, 10)+"/pay", tok, b, "application/json")
		var m map[string]any
		_ = json.Unmarshal(w.Body.Bytes(), &m)
		return w.Code, m
	}
	message := func(m map[string]any) string {
		if e, ok := m["error"].(map[string]any); ok {
			s, _ := e["message"].(string)
			return s
		}
		return ""
	}

	t.Run("las formas se excluyen", func(t *testing.T) {
		o := newOrder(app.CreateOrderCmd{})
		line := []map[string]any{{"lineId": o.Lines[0].ID, "qty": "1"}}
		for _, body := range []map[string]any{
			{"lines": line, "amount": "50"},
			{"lines": line, "allRemaining": true},
			{"allRemaining": true, "amount": "10"},
		} {
			code, m := pay(t, o.ID, body)
			if code != http.StatusBadRequest || message(m) != "Elige una sola forma de cobrar" {
				t.Errorf("%v = %d %q, quiere 400 «Elige una sola forma de cobrar»", body, code, message(m))
			}
		}
	})

	t.Run("la respuesta trae id, número y el monto cobrado", func(t *testing.T) {
		o := newOrder(app.CreateOrderCmd{})
		code, m := pay(t, o.ID, map[string]any{"clientUuid": uuid.New().String(), "lines": []map[string]any{{"lineId": o.Lines[0].ID, "qty": "2"}}})
		amount, _ := m["amount"].(string)
		if code != http.StatusOK || m["paymentId"] == nil || m["number"] != float64(1) || amount == "" || !pesos(amount).Equal(pesos("100")) {
			t.Fatalf("= %d %v; quiere 200 con paymentId, number 1 y amount 100", code, m)
		}
	})

	t.Run("la misma llave con otra selección", func(t *testing.T) {
		o := newOrder(app.CreateOrderCmd{})
		key := uuid.New().String()
		one := map[string]any{"clientUuid": key, "lines": []map[string]any{{"lineId": o.Lines[0].ID, "qty": "1"}}}
		if code, m := pay(t, o.ID, one); code != http.StatusOK {
			t.Fatalf("primer cobro = %d %v", code, m)
		}
		if code, m := pay(t, o.ID, map[string]any{"clientUuid": key, "lines": []map[string]any{{"lineId": o.Lines[0].ID, "qty": "1"}}}); code != http.StatusOK || m["yaEstaba"] != true {
			t.Fatalf("reenvío idéntico = %d %v, quiere 200 con yaEstaba", code, m)
		}
		code, m := pay(t, o.ID, map[string]any{"clientUuid": key, "lines": []map[string]any{{"lineId": o.Lines[0].ID, "qty": "2"}}})
		if code != http.StatusConflict || message(m) != "Ese cobro ya se hizo con otros productos. Vuelve a intentarlo" {
			t.Fatalf("misma llave, otra selección = %d %q", code, message(m))
		}
	})

	t.Run("el número cuenta vivos, devueltos y los viejos sin número", func(t *testing.T) {
		o := newOrder(app.CreateOrderCmd{})
		if _, err := svc.Charge(ctx, app.ChargeCmd{OrderID: o.ID, MethodID: cash, Amount: pesos("10"), ActorID: cashier}); err != nil {
			t.Fatal(err)
		}
		if _, err := st.Pool.Exec(ctx, `update order_payments set payment_number = null where order_id = $1`, o.ID); err != nil {
			t.Fatal(err)
		}
		if _, err := st.Pool.Exec(ctx, `
			insert into order_payment_voids (company_id, order_id, original_payment_id, payment_number, payment_method_id, amount,
			  tip_amount, register_session_id, paid_at, voided_by, reason)
			values ($1, $2, -20, 2, $3, 15, 0, $4, now(), $5, 'prueba')`, defaultCompanyID, o.ID, cash, session, cashier); err != nil {
			t.Fatal(err)
		}
		_, m := pay(t, o.ID, map[string]any{"lines": []map[string]any{{"lineId": o.Lines[0].ID, "qty": "1"}}})
		if m["number"] != float64(3) {
			t.Fatalf("número %v, quiere 3: el viejo sin número es el 1 y el devuelto el 2", m["number"])
		}
	})

	t.Run("un pedido de plataforma no se divide", func(t *testing.T) {
		didi := platformID(t, st, defaultCompanyID, "Didi")
		o := newOrder(app.CreateOrderCmd{ServiceType: "domicilio", DeliveryPlatformID: &didi})
		for _, body := range []map[string]any{
			{"lines": []map[string]any{{"lineId": o.Lines[0].ID, "qty": "1"}}},
			{"allRemaining": true},
		} {
			code, m := pay(t, o.ID, body)
			if code != http.StatusConflict || message(m) != "Los pedidos de plataforma no se dividen" {
				t.Errorf("%v = %d %q", body, code, message(m))
			}
		}
	})

	t.Run("un pedido de un turno cerrado no se divide", func(t *testing.T) {
		o := newOrder(app.CreateOrderCmd{})
		if _, err := st.Pool.Exec(ctx, `update register_sessions set status = 'cerrada', closed_at = now() where status = 'abierta'`); err != nil {
			t.Fatal(err)
		}
		abrirCajaPrincipal(t, st, cashier)
		code, m := pay(t, o.ID, map[string]any{"lines": []map[string]any{{"lineId": o.Lines[0].ID, "qty": "1"}}})
		if code != http.StatusConflict || message(m) != "Ese pedido es de un turno cerrado; no se divide" {
			t.Fatalf("= %d %q", code, message(m))
		}
	})
}

// CHARGE CON PRODUCTOS, AISLADO EN LOS TRES CASOS. Empieza a leer order_payment_lines: desde otra
// empresa no cobra el pedido de la dueña ni ve su cobertura.
func TestChargeByProductsStaysIsolated(t *testing.T) {
	st := newTestStore(t)
	other := makeCompany(t, st, "ajena-cobro")
	s := newSplitTable(t, st, "cobro_aislado", "50", "50")
	s.pay(t, 0)
	before := splitBillFingerprint(t, st, s.order.ID)
	inTheThreeCases(t, defaultCompanyID, other, func(t *testing.T, st2 *store.Store, c context.Context) {
		svc := app.NewOrdersService(st2, clock)
		for _, cmd := range []app.ChargeCmd{
			{OrderID: s.order.ID, MethodID: s.cash, ActorID: s.cashier, Lines: []domain.SelectedPieces{{LineID: s.order.Lines[1].ID, Qty: pesos("1")}}},
			{OrderID: s.order.ID, MethodID: s.cash, ActorID: s.cashier, AllRemaining: true},
		} {
			if _, err := svc.Charge(c, cmd); err == nil {
				t.Errorf("cobró el pedido de la dueña: %+v", cmd)
			}
		}
		if after := splitBillFingerprint(t, st, s.order.ID); after != before {
			t.Fatalf("cambió el pedido de la dueña:\nantes   %s\ndespués %s", before, after)
		}
	})
}

// DOS TABLETAS COBRANDO LA MISMA PIEZA A LA VEZ: UNA PASA, LA OTRA NO.
//
// Sin el candado del pedido las dos leerían la pieza libre y la cobrarían, y el cliente pagaría dos
// veces el mismo frappé. Varias vueltas, porque una carrera que se gana por suerte en una vuelta
// no prueba nada.
func TestTheSamePieceCannotBePaidTwiceConcurrently(t *testing.T) {
	st := newTestStore(t)
	cashier := makeUser(t, st, "cajero_carrera_pieza", "cajero")
	abrirCajaPrincipal(t, st, cashier)
	cash := paymentMethodID(t, st, "Efectivo")
	frappe := makeProduct(t, st, "Frappé en carrera", pesos("50"), true)
	svc := app.NewOrdersService(st, clock)
	ctx := context.Background()
	for i := range 15 {
		o, err := svc.Create(ctx, app.CreateOrderCmd{ClientUUID: uuid.New(), ServiceType: "mostrador", OpenedBy: cashier,
			Lines: []domain.OrderLineInput{{ProductID: frappe, Qty: pesos("2")}}})
		if err != nil {
			t.Fatal(err)
		}
		var wg sync.WaitGroup
		errs := make([]error, 2)
		for g := range 2 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, errs[g] = svc.Charge(ctx, app.ChargeCmd{OrderID: o.ID, MethodID: cash, ActorID: cashier, ClientUUID: uuid.New(),
					Lines: []domain.SelectedPieces{{LineID: o.Lines[0].ID, Qty: pesos("2")}}})
			}()
		}
		wg.Wait()
		ok, paid := 0, 0
		for _, err := range errs {
			switch {
			case err == nil:
				ok++
			case errors.Is(err, domain.ErrPieceAlreadyPaid), errors.Is(err, domain.ErrPedidoYaPagado):
				paid++
			default:
				t.Fatalf("vuelta %d: error inesperado %v", i, err)
			}
		}
		var payments int
		if err := st.Pool.QueryRow(ctx, `select count(*) from order_payments where order_id = $1`, o.ID).Scan(&payments); err != nil {
			t.Fatal(err)
		}
		if ok != 1 || paid != 1 || payments != 1 {
			t.Fatalf("vuelta %d: %d cobros pasaron, %d rechazados, %d pagos en la base; quiere 1, 1 y 1", i, ok, paid, payments)
		}
	}
}
