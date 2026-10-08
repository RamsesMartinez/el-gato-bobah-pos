//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"sync"
	"testing"
	"time"

	"uuid"

	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
)

// drawerFor lee, de la vista del turno, lo esperado y las propinas de un método.
func drawerFor(t *testing.T, st *store.Store, session int64, method string) (expected, tips decimal.Decimal) {
	t.Helper()
	v, err := app.NewBackofficeService(st, clock).SessionDetail(context.Background(), session)
	if err != nil {
		t.Fatalf("SessionDetail: %v", err)
	}
	for _, m := range v.Totals {
		if m.Name == method {
			if m.Expected != nil {
				expected = *m.Expected
			}
			return expected, m.Tips
		}
	}
	return decimal.Zero, decimal.Zero
}

func openSession(t *testing.T, st *store.Store) int64 {
	t.Helper()
	var id int64
	if err := st.Pool.QueryRow(context.Background(), `select id from register_sessions where status = 'abierta'`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

// UN PAGO DEVUELTO CUENTA CERO VECES EN EL CAJÓN.
//
// Cobrado y devuelto en el mismo turno es, para el cajón, un pago que no ocurrió: el esperado por
// método, las propinas y lo pendiente quedan como si nunca hubiera entrado. Si reapareciera en
// cualquiera de los tres, el corte cerraría con un faltante o un sobrante por el monto exacto.
func TestAVoidedPaymentCountsZeroTimesInTheDrawer(t *testing.T) {
	st := newTestStore(t)
	s := newSplitTable(t, st, "devuelto_en_cajon", "50", "50")
	session := openSession(t, st)
	expBefore, tipsBefore := drawerFor(t, st, session, "Efectivo")
	res, err := s.svc.Charge(s.ctx, app.ChargeCmd{OrderID: s.order.ID, MethodID: s.cash, Tip: pesos("5"), ActorID: s.cashier,
		Lines: []domain.SelectedPieces{{LineID: s.order.Lines[0].ID, Qty: pesos("1")}}})
	if err != nil {
		t.Fatal(err)
	}
	void, err := s.svc.VoidPayment(s.ctx, s.order.ID, res.PaymentID, s.cashier, "Se le cobró a otra persona")
	if err != nil {
		t.Fatalf("VoidPayment: %v", err)
	}
	if !void.Outstanding.Equal(pesos("100")) || void.Paid {
		t.Fatalf("tras devolver: falta %s, saldado %v; quiere 100 y no saldado", void.Outstanding, void.Paid)
	}
	exp, tips := drawerFor(t, st, session, "Efectivo")
	if !exp.Equal(expBefore) {
		t.Errorf("el esperado en efectivo pasó de %s a %s: el pago devuelto reapareció en el corte por método", expBefore, exp)
	}
	if !tips.Equal(tipsBefore) {
		t.Errorf("las propinas pasaron de %s a %s: la propina del pago devuelto reapareció", tipsBefore, tips)
	}
	v, err := s.svc.Detail(s.ctx, s.order.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !v.Outstanding.Equal(pesos("100")) {
		t.Errorf("lo pendiente es %s: el pago devuelto reapareció como pagado", v.Outstanding)
	}
	// Sus productos vuelven a quedar por cobrar.
	if _, err := s.svc.Quote(s.ctx, app.QuoteCmd{OrderID: s.order.ID, Lines: []domain.SelectedPieces{{LineID: s.order.Lines[0].ID, Qty: pesos("1")}}}); err != nil {
		t.Errorf("el producto del pago devuelto no quedó por cobrar: %v", err)
	}
}

// UN PAGO DEVUELTO NO REVIVE POR SU LLAVE, NI EN SECUENCIA NI A LA VEZ QUE SE DEVUELVE.
//
// La tableta que no pintó la respuesta reenvía el cobro con la misma llave. Si eso pasa después de
// devolverlo, o mientras se devuelve, el pago no puede volver a entrar: sería cobrar justo lo que se
// acaba de devolver.
func TestAVoidedPaymentCannotBeRevivedByItsKey(t *testing.T) {
	st := newTestStore(t)
	s := newSplitTable(t, st, "llave_devuelta", "50", "50")
	charge := func(key uuid.UUID) (*app.ChargeResult, error) {
		return s.svc.Charge(s.ctx, app.ChargeCmd{OrderID: s.order.ID, MethodID: s.cash, ActorID: s.cashier, ClientUUID: key,
			Lines: []domain.SelectedPieces{{LineID: s.order.Lines[0].ID, Qty: pesos("1")}}})
	}
	key := uuid.New()
	res, err := charge(key)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.svc.VoidPayment(s.ctx, s.order.ID, res.PaymentID, s.cashier, "Se le cobró a otra persona"); err != nil {
		t.Fatal(err)
	}
	if _, err := charge(key); !errors.Is(err, domain.ErrPaymentVoidedKey) {
		t.Fatalf("reenviar la llave devuelta = %v; quiere «Ese pago ya se devolvió. Vuelve a cobrar»", err)
	}

	// A la vez: cada vuelta cobra, y luego devuelve y reenvía la llave en paralelo.
	appSt := appRoleStore(t)
	svc := app.NewOrdersService(appSt, clock)
	for i := range 10 {
		k := uuid.New()
		r, err := charge(k)
		if err != nil {
			t.Fatalf("vuelta %d: %v", i, err)
		}
		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			c, release, _ := appSt.AcquireTenant(context.Background(), defaultCompanyID)
			defer release()
			_, _ = svc.VoidPayment(c, s.order.ID, r.PaymentID, s.cashier, "Se le cobró a otra persona")
		}()
		go func() {
			defer wg.Done()
			c, release, _ := appSt.AcquireTenant(context.Background(), defaultCompanyID)
			defer release()
			_, _ = svc.Charge(c, app.ChargeCmd{OrderID: s.order.ID, MethodID: s.cash, ActorID: s.cashier, ClientUUID: k,
				Lines: []domain.SelectedPieces{{LineID: s.order.Lines[0].ID, Qty: pesos("1")}}})
		}()
		wg.Wait()
		var live, voided int
		if err := st.Pool.QueryRow(context.Background(), `
			select (select count(*) from order_payments where client_uuid = $1),
			       (select count(*) from order_payment_voids where client_uuid = $1)`, k).Scan(&live, &voided); err != nil {
			t.Fatal(err)
		}
		if live != 0 || voided != 1 {
			t.Fatalf("vuelta %d: %d pagos vivos y %d devueltos con la llave; quiere 0 y 1: el pago revivió", i, live, voided)
		}
	}
}

// UN PAGO SE DEVUELVE UNA SOLA VEZ, Y SOLO EN SU TURNO ABIERTO.
func TestAPaymentIsVoidedOnceAndOnlyInItsOpenShift(t *testing.T) {
	st := newTestStore(t)
	s := newSplitTable(t, st, "devolver_una_vez", "50", "50")
	res := s.pay(t, 0)
	if _, err := s.svc.VoidPayment(s.ctx, s.order.ID, res.PaymentID, s.cashier, "Se le cobró a otra persona"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.svc.VoidPayment(s.ctx, s.order.ID, res.PaymentID, s.cashier, "Se le cobró a otra persona"); !errors.Is(err, domain.ErrPaymentAlreadyVoided) {
		t.Fatalf("devolver dos veces = %v; quiere «Ese pago ya se devolvió»", err)
	}
	if _, err := s.svc.VoidPayment(s.ctx, s.order.ID, s.pay(t, 1).PaymentID, s.cashier, "   "); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("sin motivo = %v; quiere «Elige por qué se devuelve»", err)
	}

	// De un turno cerrado: su dinero ya se arqueó.
	closed := newSplitTable(t, st, "devolver_turno_cerrado", "50", "50")
	old := closed.pay(t, 0)
	if _, err := st.Pool.Exec(context.Background(), `update register_sessions set status = 'cerrada', closed_at = now() where status = 'abierta'`); err != nil {
		t.Fatal(err)
	}
	abrirCajaPrincipal(t, st, closed.cashier)
	if _, err := closed.svc.VoidPayment(closed.ctx, closed.order.ID, old.PaymentID, closed.cashier, "Se le cobró a otra persona"); !errors.Is(err, domain.ErrPaymentFromClosedShift) {
		t.Fatalf("pago de turno cerrado = %v; quiere ErrPaymentFromClosedShift", err)
	}
	// Sin turno: los pagos viejos que nacieron sin sesión.
	if _, err := st.Pool.Exec(context.Background(), `update order_payments set register_session_id = null where id = $1`, old.PaymentID); err != nil {
		t.Fatal(err)
	}
	if _, err := closed.svc.VoidPayment(closed.ctx, closed.order.ID, old.PaymentID, closed.cashier, "Se le cobró a otra persona"); !errors.Is(err, domain.ErrPaymentFromClosedShift) {
		t.Fatalf("pago sin turno = %v; quiere ErrPaymentFromClosedShift", err)
	}
}

// LOS NÚMEROS DE LOS PAGOS SOBREVIVEN A UNA DEVOLUCIÓN.
//
// Con un pago viejo sin número: la bitácora guarda el número que la vista le daba, y el pago
// siguiente no lo repite. El «Pago 2» impreso sigue siendo el 2 en pantalla.
func TestPaymentNumbersSurviveAVoid(t *testing.T) {
	st := newTestStore(t)
	s := newSplitTable(t, st, "numeros_devueltos", "50", "50", "50")
	first := s.pay(t, 0)
	if _, err := st.Pool.Exec(context.Background(), `update order_payments set payment_number = null where id = $1`, first.PaymentID); err != nil {
		t.Fatal(err)
	}
	second := s.pay(t, 1)
	if second.Number != 2 {
		t.Fatalf("el segundo pago es el %d", second.Number)
	}
	if _, err := s.svc.VoidPayment(s.ctx, s.order.ID, first.PaymentID, s.cashier, "Se le cobró a otra persona"); err != nil {
		t.Fatal(err)
	}
	var kept int
	if err := st.Pool.QueryRow(context.Background(), `select payment_number from order_payment_voids where original_payment_id = $1`, first.PaymentID).Scan(&kept); err != nil {
		t.Fatal(err)
	}
	if kept != 1 {
		t.Fatalf("la bitácora guardó el número %d; quiere 1, el que la vista le daba", kept)
	}
	if third := s.pay(t, 2); third.Number != 3 {
		t.Fatalf("el pago siguiente es el %d; quiere 3 (el 1 devuelto y el 2 siguen contando)", third.Number)
	}
	v, err := s.svc.Detail(s.ctx, s.order.ID)
	if err != nil {
		t.Fatal(err)
	}
	got := []string{}
	for _, p := range v.Payments {
		got = append(got, strconv.Itoa(p.Number)+map[bool]string{true: "d", false: ""}[p.Voided])
	}
	if len(got) != 3 || got[0] != "1d" || got[1] != "2" || got[2] != "3" {
		t.Fatalf("la vista numera %v; quiere [1d 2 3]", got)
	}
}

// DEVOLVER UN PAGO: PERMISO, AISLAMIENTO Y LA LISTA DEL CORTE.
func TestVoidPaymentPermissionIsolationAndDrawerList(t *testing.T) {
	st := newTestStore(t)
	r, token := ordersAPI(t, st, nil)
	_, cashierTok := token("http_cajero_devuelve", "cajero")
	_, managerTok := token("http_gerente_devuelve", "gerente")
	s := newSplitTable(t, st, "devolver_http", "50", "50")
	session := openSession(t, st)
	path := func(order, payment int64) string {
		return "/api/v1/orders/" + strconv.FormatInt(order, 10) + "/payments/" + strconv.FormatInt(payment, 10) + "/void"
	}
	body := []byte(`{"reason":"Se le cobró a otra persona"}`)

	paid := s.pay(t, 0)
	w := do(t, r, http.MethodPost, path(s.order.ID, paid.PaymentID), cashierTok, body, "application/json")
	if w.Code != http.StatusForbidden || !json.Valid(w.Body.Bytes()) || !containsMessage(w.Body.Bytes(), "Tu usuario no puede devolver pagos") {
		t.Fatalf("cajero = %d %s; quiere 403 «Tu usuario no puede devolver pagos»", w.Code, w.Body.String())
	}
	w = do(t, r, http.MethodPost, path(s.order.ID, paid.PaymentID), managerTok, body, "application/json")
	if w.Code != http.StatusOK {
		t.Fatalf("gerente = %d %s", w.Code, w.Body.String())
	}

	// La vista del turno lista los devueltos, siempre como arreglo, sin tocar el esperado.
	var raw map[string]any
	if err := json.Unmarshal(mustJSON(t, func() (any, error) {
		return app.NewBackofficeService(st, clock).SessionDetail(context.Background(), session)
	}), &raw); err != nil {
		t.Fatal(err)
	}
	list, ok := raw["voidedPayments"].([]any)
	if !ok || len(list) != 1 {
		t.Fatalf("voidedPayments = %#v; quiere un arreglo con el pago devuelto", raw["voidedPayments"])
	}
	item := list[0].(map[string]any)
	if item["method"] != "Efectivo" || item["reason"] != "Se le cobró a otra persona" || item["orderFolio"] == nil || item["voidedBy"] == nil {
		t.Fatalf("el devuelto dice %v", item)
	}

	other := makeCompany(t, st, "ajena-devolver")
	again := s.pay(t, 0)
	before := splitBillFingerprint(t, st, s.order.ID)
	inTheThreeCases(t, defaultCompanyID, other, func(t *testing.T, st2 *store.Store, c context.Context) {
		if _, err := app.NewOrdersService(st2, clock).VoidPayment(c, s.order.ID, again.PaymentID, s.cashier, "Se le cobró a otra persona"); err == nil {
			t.Fatal("devolvió un pago de la dueña")
		}
		if after := splitBillFingerprint(t, st, s.order.ID); after != before {
			t.Fatalf("cambió el pedido de la dueña:\nantes   %s\ndespués %s", before, after)
		}
		if v, err := app.NewBackofficeService(st2, clock).SessionDetail(c, session); err == nil && len(v.VoidedPayments) > 0 {
			t.Fatalf("vio %d pagos devueltos de la dueña", len(v.VoidedPayments))
		}
	})
}

func containsMessage(body []byte, want string) bool {
	var m map[string]map[string]any
	if json.Unmarshal(body, &m) != nil {
		return false
	}
	got, _ := m["error"]["message"].(string)
	return got == want
}

// mustJSON serializa lo que devuelve f como lo haría el handler.
func mustJSON(t *testing.T, f func() (any, error)) []byte {
	t.Helper()
	v, err := f()
	if err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// DEVOLVER NO SE CUELA MIENTRAS SE CIERRA EL TURNO.
//
// Si devolver lee «abierto», el corte confirma con ese pago en su esperado y luego la devolución lo
// borra, el corte cerrado espera un dinero que ya no figura en los pagos. El cierre bloquea el turno
// mientras corre; devolver tiene que esperarlo y ver que ya cerró.
func TestVoidingWaitsForAShiftThatIsClosing(t *testing.T) {
	st := newTestStore(t)
	s := newSplitTable(t, st, "devolver_mientras_cierra", "50", "50")
	paid := s.pay(t, 0)
	session := openSession(t, st)
	ctx := context.Background()

	closer, err := st.Pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = closer.Rollback(ctx) }()
	if _, err := closer.Exec(ctx, `select id from register_sessions where id = $1 for update`, session); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		_, err := s.svc.VoidPayment(s.ctx, s.order.ID, paid.PaymentID, s.cashier, "Se le cobró a otra persona")
		done <- err
	}()
	select {
	case err := <-done:
		t.Fatalf("devolver no esperó al cierre del turno (err=%v): el corte quedaría esperando un pago que ya no está", err)
	case <-time.After(300 * time.Millisecond):
	}
	if _, err := closer.Exec(ctx, `update register_sessions set status = 'cerrada', closed_at = now() where id = $1`, session); err != nil {
		t.Fatal(err)
	}
	if err := closer.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err := <-done; !errors.Is(err, domain.ErrPaymentFromClosedShift) {
		t.Fatalf("tras el cierre, devolver = %v; quiere «Ese pago es de un turno cerrado»", err)
	}
}

// EL PAGO DE OTRO PEDIDO DE LA MISMA EMPRESA NO SE DEVUELVE DESDE ÉSTE.
func TestVoidingAPaymentOfAnotherOrderIsRejected(t *testing.T) {
	st := newTestStore(t)
	a := newSplitTable(t, st, "devolver_ajeno_a", "50")
	b := newSplitTable(t, st, "devolver_ajeno_b", "50")
	paid := b.pay(t, 0)
	if _, err := a.svc.VoidPayment(a.ctx, a.order.ID, paid.PaymentID, a.cashier, "Se le cobró a otra persona"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("devolver el pago de otro pedido = %v; quiere «Ese pago no es de este pedido»", err)
	}
	var n int
	if err := st.Pool.QueryRow(context.Background(), `select count(*) from order_payments where id = $1`, paid.PaymentID).Scan(&n); err != nil || n != 1 {
		t.Fatalf("el pago del otro pedido desapareció (n=%d, err=%v)", n, err)
	}
}
