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

	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store/db"
)

// terminalKit arma el caso real: una sucursal con Terminal A y Terminal B (más Terminal C, archivada),
// un turno abierto y un débito de $250 cobrado en Terminal A.
type terminalKit struct {
	st                      *store.Store
	ctx                     context.Context
	orders                  *app.OrdersService
	gerente, cajero         int64
	session                 int64
	termA, termB, archivada int64
	order, payment          int64
}

func newTerminalKit(t *testing.T, slug string) *terminalKit {
	t.Helper()
	st := newTestStore(t)
	ctx := context.Background()
	k := &terminalKit{st: st, orders: app.NewOrdersService(st, clock)}
	terms := app.NewTerminalsService(st)
	branch := branchOfPrincipal(t, st)
	k.gerente = makeUser(t, st, slug+"_gerente", "gerente")
	k.cajero = makeUser(t, st, slug+"_cajero", "cajero")
	k.ctx = app.WithActorUser(ctx, k.gerente)
	k.session = abrirCajaPrincipal(t, st, k.cajero)
	for _, x := range []struct {
		name string
		id   *int64
	}{{"Terminal A", &k.termA}, {"Terminal B", &k.termB}, {"Terminal C", &k.archivada}} {
		v, err := terms.Create(ctx, branch, x.name)
		if err != nil {
			t.Fatal(err)
		}
		*x.id = v.ID
	}
	if err := terms.Archive(ctx, k.archivada); err != nil {
		t.Fatal(err)
	}
	k.order, k.payment = k.cobrar(t, "Débito capturado mal", "250", "Tarjeta débito", &k.termA)
	return k
}

func (k *terminalKit) cobrar(t *testing.T, producto, monto, metodo string, terminal *int64) (order, payment int64) {
	t.Helper()
	prod := makeProduct(t, k.st, producto, dec(monto), false)
	order = crearPedidoSimple(t, k.ctx, k.orders, prod, k.cajero)
	res, err := k.orders.Charge(k.ctx, app.ChargeCmd{OrderID: order, MethodID: paymentMethodID(t, k.st, metodo),
		Amount: dec(monto), ActorID: k.cajero, TerminalID: terminal})
	if err != nil {
		t.Fatalf("cobrar %s: %v", producto, err)
	}
	return order, res.PaymentID
}

func (k *terminalKit) change(from *int64, to int64, reason string) (*app.OrderView, error) {
	return k.orders.ChangePaymentTerminal(k.ctx, app.ChangeTerminalCmd{OrderID: k.order, PaymentID: k.payment,
		ActorID: k.gerente, FromTerminalID: from, TerminalID: to, Reason: reason})
}

func collectedBy(t *testing.T, st *store.Store, session int64) map[int64]decimal.Decimal {
	t.Helper()
	rows, err := db.New(st.Pool).TerminalCollectedForSession(context.Background(), session)
	if err != nil {
		t.Fatal(err)
	}
	out := map[int64]decimal.Decimal{}
	for _, r := range rows {
		out[r.TerminalID] = r.Collected
	}
	return out
}

func terminalEvents(t *testing.T, st *store.Store, orderID int64) []map[string]any {
	t.Helper()
	rows, err := st.Pool.Query(context.Background(), `
		select json_build_object('actor', actor_user_id, 'entityType', entity_type, 'entityId', entity_id,
		       'branch', branch_id, 'before', before, 'after', after)
		  from audit_events where order_id = $1 and action = 'payment.terminal_changed' order by id`, orderID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []map[string]any
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			t.Fatal(err)
		}
		m := map[string]any{}
		if err := json.Unmarshal(raw, &m); err != nil {
			t.Fatal(err)
		}
		out = append(out, m)
	}
	return out
}

// EL CASO REAL. Un débito se capturó en Terminal A y se cobró en Terminal B; hubo que corregirlo a
// mano en la base. Corregirlo desde el sistema mueve los $250 de una terminal a otra en el corte del
// turno, y el pago guarda el nombre que dice la base, no el que mandó la pantalla.
func TestACardPaymentMovesToTheTerminalWhereItWasCharged(t *testing.T) {
	t.Parallel()
	k := newTerminalKit(t, "once_oct")
	if c := collectedBy(t, k.st, k.session); !c[k.termA].Equal(dec("250")) || !c[k.termB].IsZero() {
		t.Fatalf("antes de corregir: %v", c)
	}
	if _, err := k.st.Pool.Exec(k.ctx, `update card_terminals set name = 'Terminal B renombrada' where id = $1`, k.termB); err != nil {
		t.Fatal(err)
	}
	v, err := k.change(&k.termA, k.termB, "  Se cobró en otra terminal  ")
	if err != nil {
		t.Fatalf("cambiar terminal: %v", err)
	}
	c := collectedBy(t, k.st, k.session)
	if !c[k.termA].IsZero() || !c[k.termB].Equal(dec("250")) {
		t.Fatalf("los $250 no se movieron a Terminal B: %v", c)
	}
	id, name := terminalDelPago(t, k.st, k.order)
	if id == nil || *id != k.termB || name == nil || *name != "Terminal B renombrada" {
		t.Fatalf("el pago quedó en %v %v; quiere Terminal B con el nombre de la base", id, name)
	}
	if len(v.Payments) != 1 || v.Payments[0].TerminalID == nil || *v.Payments[0].TerminalID != k.termB ||
		v.Payments[0].TerminalName == nil || *v.Payments[0].TerminalName != "Terminal B renombrada" {
		t.Fatalf("la respuesta no trae la terminal nueva: %+v", v.Payments)
	}
}

// Cada rechazo deja el pago como estaba y sin evento: un evento sin cambio es un rastro falso.
func TestChangingTheTerminalRejectsWhatItCannotDo(t *testing.T) {
	t.Parallel()
	k := newTerminalKit(t, "rechazos")
	cases := []struct {
		name   string
		from   *int64
		to     int64
		reason string
		want   error
	}{
		{"sin motivo", &k.termA, k.termB, "  ", domain.ErrValidation},
		{"destino archivado", &k.termA, k.archivada, "Se cobró en otra terminal", domain.ErrValidation},
		{"destino igual", &k.termA, k.termA, "Se cobró en otra terminal", domain.ErrValidation},
		{"destino que no existe", &k.termA, 999999, "Se cobró en otra terminal", domain.ErrValidation},
	}
	for _, c := range cases {
		if _, err := k.change(c.from, c.to, c.reason); !errors.Is(err, c.want) {
			t.Errorf("%s: err = %v, quiere %v", c.name, err, c.want)
		}
	}

	// Pago en efectivo.
	cashOrder, cashPayment := k.cobrar(t, "Efectivo", "50", "Efectivo", nil)
	_, err := k.orders.ChangePaymentTerminal(k.ctx, app.ChangeTerminalCmd{OrderID: cashOrder, PaymentID: cashPayment,
		ActorID: k.gerente, TerminalID: k.termB, Reason: "Se cobró en otra terminal"})
	if !errors.Is(err, domain.ErrValidation) {
		t.Errorf("efectivo: err = %v, quiere validación", err)
	}
	// El pago de OTRO pedido por la ruta de éste: no se toca, aunque exista.
	_, err = k.orders.ChangePaymentTerminal(k.ctx, app.ChangeTerminalCmd{OrderID: k.order, PaymentID: cashPayment,
		ActorID: k.gerente, TerminalID: k.termB, Reason: "Se cobró en otra terminal"})
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("pago de otro pedido: err = %v, quiere no encontrado", err)
	}
	// Destino de otra sucursal: existe, está activa, pero no es de la caja del pago.
	var otraSucursal int64
	if err := k.st.Pool.QueryRow(k.ctx, `insert into branches (company_id, name, branch_number, code)
		select company_id, 'Sucursal Norte', 2, 'NTE' from branches where id = $1 returning id`, branchOfPrincipal(t, k.st)).Scan(&otraSucursal); err != nil {
		t.Fatal(err)
	}
	ajena, err := app.NewTerminalsService(k.st).Create(k.ctx, otraSucursal, "Terminal de otra sucursal")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := k.change(&k.termA, ajena.ID, "Se cobró en otra terminal"); !errors.Is(err, domain.ErrValidation) {
		t.Errorf("terminal de otra sucursal: err = %v, quiere validación", err)
	}
	// Pedido cancelado.
	if _, err := k.st.Pool.Exec(k.ctx, `update orders set status = 'cancelada', cancelled_at = now(), cancelled_by = $2, cancel_reason = 'prueba'
		  where id = $1`, k.order, k.gerente); err != nil {
		t.Fatal(err)
	}
	if _, err := k.change(&k.termA, k.termB, "Se cobró en otra terminal"); !errors.Is(err, domain.ErrConflict) {
		t.Errorf("pedido cancelado: err = %v, quiere conflicto", err)
	}

	if id, _ := terminalDelPago(t, k.st, k.order); id == nil || *id != k.termA {
		t.Fatalf("un rechazo movió el pago a %v", id)
	}
	if evs := terminalEvents(t, k.st, k.order); len(evs) != 0 {
		t.Fatalf("un rechazo dejó %d eventos", len(evs))
	}
}

// DOS GERENTES A LA VEZ. Los dos vieron Terminal A; uno lo pasa a Terminal B y el otro a Terminal C. Sin la
// comparación contra lo que vio cada pantalla, el segundo pisaría al primero y la bitácora diría que
// alguien cambió «Terminal A → Terminal C» cuando el pago ya estaba en Terminal B.
func TestTwoSimultaneousChangesOnlyOneWins(t *testing.T) {
	t.Parallel()
	k := newTerminalKit(t, "simultaneos")
	tercera, err := app.NewTerminalsService(k.st).Create(k.ctx, branchOfPrincipal(t, k.st), "Clip")
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i, to := range []int64{k.termB, tercera.ID} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, errs[i] = k.change(&k.termA, to, "Se cobró en otra terminal")
		}()
	}
	wg.Wait()
	ok, conflict := 0, 0
	for _, e := range errs {
		switch {
		case e == nil:
			ok++
		case errors.Is(e, domain.ErrPaymentChanged):
			conflict++
		default:
			t.Fatalf("error inesperado: %v", e)
		}
	}
	if ok != 1 || conflict != 1 {
		t.Fatalf("ganaron %d y chocaron %d; quiere 1 y 1", ok, conflict)
	}
	if evs := terminalEvents(t, k.st, k.order); len(evs) != 1 {
		t.Fatalf("la bitácora tiene %d eventos; quiere 1", len(evs))
	}
}

// EL RASTRO. El evento dice quién, qué pago, de qué terminal a cuál, cuánto, en qué turno y sucursal,
// y por qué. Y si el evento no se puede guardar, el cambio no se aplica: un cambio de dinero sin
// rastro es justo lo que la bitácora vino a impedir.
func TestTheTerminalChangeLeavesItsEventOrDoesNotHappen(t *testing.T) {
	t.Parallel()
	k := newTerminalKit(t, "rastro")
	if _, err := k.change(&k.termA, k.termB, "Se cobró en otra terminal"); err != nil {
		t.Fatal(err)
	}
	evs := terminalEvents(t, k.st, k.order)
	if len(evs) != 1 {
		t.Fatalf("eventos = %d; quiere 1", len(evs))
	}
	e := evs[0]
	num := func(v any) int64 { f, _ := v.(float64); return int64(f) }
	before, _ := e["before"].(map[string]any)
	after, _ := e["after"].(map[string]any)
	if num(e["actor"]) != k.gerente || e["entityType"] != "order_payment" || num(e["entityId"]) != k.payment ||
		num(e["branch"]) != branchOfPrincipal(t, k.st) {
		t.Errorf("autor, entidad o sucursal del evento: %v", e)
	}
	if num(before["terminalId"]) != k.termA || before["terminalName"] != "Terminal A" ||
		num(after["terminalId"]) != k.termB || after["terminalName"] != "Terminal B" {
		t.Errorf("antes/después de la terminal: %v → %v", before, after)
	}
	for _, side := range []map[string]any{before, after} {
		if side["amount"] != "250" && side["amount"] != "250.00" || num(side["sessionId"]) != k.session ||
			num(side["branchId"]) != branchOfPrincipal(t, k.st) {
			t.Errorf("monto, turno o sucursal: %v", side)
		}
	}
	if after["reason"] != "Se cobró en otra terminal" {
		t.Errorf("motivo = %v", after["reason"])
	}

	// La bitácora se niega a guardar el siguiente evento: el cambio tampoco se aplica.
	if _, err := k.st.Pool.Exec(k.ctx, `
		create function no_terminal_events() returns trigger language plpgsql as $$
		begin raise exception 'bitácora caída'; end $$;
		create trigger no_terminal_events before insert on audit_events
		for each row when (new.action = 'payment.terminal_changed') execute function no_terminal_events();`); err != nil {
		t.Fatal(err)
	}
	if _, err := k.change(&k.termB, k.termA, "Se cobró en otra terminal"); err == nil {
		t.Fatal("el cambio pasó con la bitácora caída")
	}
	if id, _ := terminalDelPago(t, k.st, k.order); id == nil || *id != k.termB {
		t.Fatalf("con la bitácora caída el pago quedó en %v; quiere Terminal B", id)
	}
}

// Un pago que se cobró sin terminal (antes de que existieran) se corrige igual; la pantalla dice que
// no tenía ninguna y el servidor lo confirma.
func TestAPaymentWithoutTerminalGetsOne(t *testing.T) {
	t.Parallel()
	k := newTerminalKit(t, "sin_terminal")
	if _, err := k.st.Pool.Exec(k.ctx, `update order_payments set card_terminal_id = null, card_terminal_name = null where id = $1`, k.payment); err != nil {
		t.Fatal(err)
	}
	if _, err := k.change(&k.termA, k.termB, "Se cobró en otra terminal"); !errors.Is(err, domain.ErrPaymentChanged) {
		t.Fatalf("la pantalla creía Terminal A: err = %v", err)
	}
	if _, err := k.change(nil, k.termB, "Se cobró en otra terminal"); err != nil {
		t.Fatal(err)
	}
	if id, _ := terminalDelPago(t, k.st, k.order); id == nil || *id != k.termB {
		t.Fatalf("quedó en %v", id)
	}
}

// La lectura del pago va bajo RLS: un pago de otra empresa, o sin empresa en la sesión, no existe.
func TestChangingTheTerminalIsIsolatedInTheThreeCases(t *testing.T) {
	t.Parallel()
	k := newTerminalKit(t, "aislamiento")
	otra := makeCompany(t, k.st, "otra-cambio-terminal")
	inTheThreeCases(t, defaultCompanyID, otra, func(t *testing.T, as *store.Store, ctx context.Context) {
		_, err := app.NewOrdersService(as, clock).ChangePaymentTerminal(ctx, app.ChangeTerminalCmd{OrderID: k.order,
			PaymentID: k.payment, ActorID: k.gerente, FromTerminalID: &k.termA, TerminalID: k.termB, Reason: "Se cobró en otra terminal"})
		if err == nil {
			t.Fatal("se cambió el pago de otra empresa")
		}
	})
	if id, _ := terminalDelPago(t, k.st, k.order); id == nil || *id != k.termA {
		t.Fatalf("el pago se movió a %v", id)
	}
}

// POR HTTP. Solo con el permiso; un cuerpo o una ruta mal formados son 400; un pago sin terminal
// sale como `null`, no ausente, en el JSON del pedido.
func TestChangeTerminalOverHTTP(t *testing.T) {
	t.Parallel()
	k := newTerminalKit(t, "http_terminal")
	r, token := ordersAPI(t, k.st, nil)
	_, cashierTok := token("http_cajero_terminal", "cajero")
	_, managerTok := token("http_gerente_terminal", "gerente")
	path := "/api/v1/orders/" + strconv.FormatInt(k.order, 10) + "/payments/" + strconv.FormatInt(k.payment, 10) + "/terminal"
	body := []byte(`{"fromTerminalId":` + strconv.FormatInt(k.termA, 10) + `,"terminalId":` + strconv.FormatInt(k.termB, 10) + `,"reason":"Se cobró en otra terminal"}`)

	w := do(t, r, http.MethodPost, path, cashierTok, body, "application/json")
	if w.Code != http.StatusForbidden || !containsMessage(w.Body.Bytes(), "Tu usuario no puede cambiar la terminal de un pago") {
		t.Fatalf("cajero = %d %s", w.Code, w.Body.String())
	}
	for _, bad := range []struct{ path, body string }{
		{path, `{"terminalId":"termB"}`},
		{path, `no es json`},
		{"/api/v1/orders/x/payments/1/terminal", string(body)},
		{"/api/v1/orders/" + strconv.FormatInt(k.order, 10) + "/payments/y/terminal", string(body)},
	} {
		if w := do(t, r, http.MethodPost, bad.path, managerTok, []byte(bad.body), "application/json"); w.Code != http.StatusBadRequest {
			t.Errorf("%s %s = %d; quiere 400", bad.path, bad.body, w.Code)
		}
	}
	w = do(t, r, http.MethodPost, path, managerTok, body, "application/json")
	if w.Code != http.StatusOK {
		t.Fatalf("gerente = %d %s", w.Code, w.Body.String())
	}
	// El mismo cuerpo otra vez: la pantalla vio Terminal A y el pago ya está en Terminal B.
	if w := do(t, r, http.MethodPost, path, managerTok, body, "application/json"); w.Code != http.StatusConflict {
		t.Fatalf("repetido = %d; quiere 409", w.Code)
	}

	cashOrder, _ := k.cobrar(t, "Efectivo JSON", "40", "Efectivo", nil)
	w = do(t, r, http.MethodGet, "/api/v1/orders/"+strconv.FormatInt(cashOrder, 10), managerTok, nil, "")
	var raw struct {
		Payments []map[string]json.RawMessage `json:"payments"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &raw); err != nil || len(raw.Payments) != 1 {
		t.Fatalf("pedido = %d %s", w.Code, w.Body.String())
	}
	if string(raw.Payments[0]["methodKind"]) != `"efectivo"` {
		t.Errorf("methodKind = %s; la pantalla lo necesita para saber qué pago tiene terminal", raw.Payments[0]["methodKind"])
	}
	for _, k := range []string{"terminalId", "terminalName"} {
		if string(raw.Payments[0][k]) != "null" {
			t.Errorf("%s = %s; quiere null", k, raw.Payments[0][k])
		}
	}
}

func countOf(t *testing.T, v []app.TerminalCountView, id int64) app.TerminalCountView {
	t.Helper()
	for _, c := range v {
		if c.TerminalID == id {
			return c
		}
	}
	t.Fatalf("la terminal %d no sale en el arqueo: %+v", id, v)
	return app.TerminalCountView{}
}

// EL CORTE CERRADO NO SE REESCRIBE, PERO AVISA. El arqueo se firmó con el débito en la Terminal A, y Terminal B ni siquiera tenía renglón porque no había cobrado nada. Al corregir, lo firmado se
// queda como se firmó y el corte dice, terminal por terminal, cuánto se espera hoy y la diferencia
// contra lo declarado. Y si el pedido tuvo una devolución con tarjeta en OTRO turno cerrado, ese
// turno también lo ve: la devolución se atribuye a la terminal del pago, que es la que se movió.
func TestAClosedShiftKeepsItsCountAndShowsWhatChangedAfter(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()
	orders := app.NewOrdersService(st, clock)
	back := app.NewBackofficeService(st, clock)
	terms := app.NewTerminalsService(st)
	cajero := makeUser(t, st, "cajero_corte_cerrado", "cajero")
	gerente := makeUser(t, st, "gerente_corte_cerrado", "gerente")
	branch := branchOfPrincipal(t, st)
	reg := principalRegister(t, st)
	if err := terms.SetCardCountMode(ctx, branch, "per_terminal"); err != nil {
		t.Fatal(err)
	}
	termA, err := terms.Create(ctx, branch, "Terminal A")
	if err != nil {
		t.Fatal(err)
	}
	termB, err := terms.Create(ctx, branch, "Terminal B")
	if err != nil {
		t.Fatal(err)
	}
	debito := paymentMethodID(t, st, "Tarjeta débito")
	cero := dec("0")
	cobrar := func(nombre, monto string, terminal int64) (int64, int64) {
		prod := makeProduct(t, st, nombre, dec(monto), false)
		ord := crearPedidoSimple(t, ctx, orders, prod, cajero)
		res, err := orders.Charge(ctx, app.ChargeCmd{OrderID: ord, MethodID: debito, Amount: dec(monto), ActorID: cajero, TerminalID: &terminal})
		if err != nil {
			t.Fatal(err)
		}
		return ord, res.PaymentID
	}

	// Turno A: el débito de $250 en Terminal A; la terminal de verdad (Terminal B) no imprime nada de la A.
	turnoA, err := back.OpenSession(ctx, reg, app.AperturaCmd{Total: &cero, Motivo: "prueba"}, cajero)
	if err != nil {
		t.Fatal(err)
	}
	ord, pago := cobrar("Débito corte", "250", termA.ID)
	entregarPendientes(t, st)
	if _, err := back.CloseSession(ctx, reg, cajero, app.CierreCmd{Total: &cero, Motivo: "prueba",
		TerminalCounts: map[int64]decimal.Decimal{termA.ID: dec("0")}}); err != nil {
		t.Fatal(err)
	}

	// Turno B: Terminal A cobra $200 y se devuelven $100 del débito de A con tarjeta.
	turnoB, err := back.OpenSession(ctx, reg, app.AperturaCmd{Total: &cero, Motivo: "prueba"}, cajero)
	if err != nil {
		t.Fatal(err)
	}
	cobrar("Otro débito", "200", termA.ID)
	entregarPendientes(t, st)
	if err := orders.Devolver(ctx, app.DevolucionCmd{OrderID: ord, Monto: dec("100"), Motivo: "prueba", ActorID: cajero, CardFolio: "0042"}); err != nil {
		t.Fatal(err)
	}
	if _, err := back.CloseSession(ctx, reg, cajero, app.CierreCmd{Total: &cero, Motivo: "prueba",
		TerminalCounts: map[int64]decimal.Decimal{termA.ID: dec("100")}}); err != nil {
		t.Fatal(err)
	}

	if _, err := orders.ChangePaymentTerminal(app.WithActorUser(ctx, gerente), app.ChangeTerminalCmd{OrderID: ord, PaymentID: pago,
		ActorID: gerente, FromTerminalID: &termA.ID, TerminalID: termB.ID, Reason: "Se cobró en otra terminal"}); err != nil {
		t.Fatal(err)
	}

	a, err := back.SessionDetail(ctx, turnoA.ID)
	if err != nil {
		t.Fatal(err)
	}
	h := countOf(t, a.TerminalCounts, termA.ID)
	if !h.Expected.Equal(dec("250")) || !h.Difference.Equal(dec("-250")) || !h.ExpectedNow.IsZero() || !h.DifferenceNow.IsZero() {
		t.Errorf("turno A, Terminal A: lo firmado se reescribió o el aviso no cambió: %+v", h)
	}
	g := countOf(t, a.TerminalCounts, termB.ID)
	if !g.Expected.IsZero() || !g.Declared.IsZero() || !g.ExpectedNow.Equal(dec("250")) || !g.DifferenceNow.Equal(dec("-250")) {
		t.Errorf("turno A, Terminal B (sin renglón al cerrar): %+v", g)
	}

	b, err := back.SessionDetail(ctx, turnoB.ID)
	if err != nil {
		t.Fatal(err)
	}
	hb := countOf(t, b.TerminalCounts, termA.ID)
	if !hb.Expected.Equal(dec("100")) || !hb.ExpectedNow.Equal(dec("200")) || !hb.DifferenceNow.Equal(dec("-100")) {
		t.Errorf("turno B: la devolución se fue con el pago y el corte no lo dice: %+v", hb)
	}
}
