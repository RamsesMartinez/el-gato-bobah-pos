//go:build integration

package integration

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
)

// Anular un gasto capturado por error, aunque esté pagado (#216). El caso real: una propina
// registrada como gasto. Apagar «Incluir en el arqueo» sacaba la salida del cajón, pero el gasto
// seguía pagado y contando en los reportes.

type gastoAnulado struct {
	status string
	reason *string
	by     *int64
	at     *time.Time
}

func (g arqueoPorHora) anulado(t *testing.T, id int64) gastoAnulado {
	t.Helper()
	var a gastoAnulado
	if err := g.st.Pool.QueryRow(context.Background(), `select status::text, cancel_reason, cancelled_by, cancelled_at from expenses where id = $1`, id).
		Scan(&a.status, &a.reason, &a.by, &a.at); err != nil {
		t.Fatal(err)
	}
	return a
}

func (g arqueoPorHora) pagosDe(t *testing.T, id int64) int {
	t.Helper()
	var n int
	if err := g.st.Pool.QueryRow(context.Background(), `select count(*) from expense_payments where expense_id = $1`, id).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// PAGADO, CON SU PAGO EN UN CORTE ABIERTO: se anula, el pago sale del corte con su salida en la misma
// transacción, el pago no se borra, y la bitácora guarda quién y por qué.
func TestAnularUnGastoPagadoEnCorteAbierto(t *testing.T) {
	t.Parallel()
	g := nuevoArqueoPorHora(t)
	ctx := context.Background()
	sess := g.turno(t, g.primar, time.Now().Add(-time.Hour))
	id, err := g.back.CreateExpense(ctx, g.gasto(g.primar, hoyDelNegocio(), incluir(true), false, "120"))
	if err != nil {
		t.Fatal(err)
	}
	pay := g.pago(t, id).id
	if g.neto(t, sess) != "-120.00" {
		t.Fatalf("la propina no salió del cajón al registrarla: neto %s", g.neto(t, sess))
	}

	if err := g.back.CancelExpense(ctx, id, "  era una propina  ", g.admin, true); err != nil {
		t.Fatalf("anular un gasto pagado en un corte abierto: %v", err)
	}
	a := g.anulado(t, id)
	if a.status != "cancelada" || a.reason == nil || *a.reason != "era una propina" || a.by == nil || *a.by != g.admin || a.at == nil {
		t.Fatalf("el gasto quedó %+v; quería cancelada, con motivo recortado, quién y cuándo", a)
	}
	if p := g.pago(t, id); p.session != nil || p.include {
		t.Fatalf("el pago se quedó en el corte abierto: %+v", p)
	}
	if g.salidas(t, pay) != 0 || g.neto(t, sess) != "0.00" {
		t.Fatalf("la salida de la propina sigue en el corte: %d salidas, neto %s", g.salidas(t, pay), g.neto(t, sess))
	}
	if g.pagosDe(t, id) != 1 {
		t.Fatal("anular borró el pago: es borrado lógico, la fila se queda")
	}
	var reason, sessionBefore string
	if err := g.st.Pool.QueryRow(ctx, `select after->>'reason', coalesce(before->'payments'->0->>'sessionId', '')
		from audit_events where action = 'expense.cancelled' and entity_id = $1`, id).Scan(&reason, &sessionBefore); err != nil {
		t.Fatalf("la anulación no dejó su evento en la bitácora: %v", err)
	}
	if reason != "era una propina" || sessionBefore != itoa(int(sess)) {
		t.Fatalf("el evento dice motivo %q y corte previo %q; quería el motivo y el corte %d", reason, sessionBefore, sess)
	}
}

// CON UN PAGO EN UN CORTE CERRADO: no se reescribe un corte contado; se rechaza diciendo que eso es una
// devolución (#175), y nada cambia.
func TestAnularUnGastoConPagoEnCorteCerradoSeNiega(t *testing.T) {
	t.Parallel()
	g := nuevoArqueoPorHora(t)
	ctx := context.Background()
	sess := g.turno(t, g.primar, time.Now().Add(-time.Hour))
	id, err := g.back.CreateExpense(ctx, g.gasto(g.primar, hoyDelNegocio(), incluir(true), false, "80"))
	if err != nil {
		t.Fatal(err)
	}
	pay := g.pago(t, id).id
	g.cerrar(t, sess, time.Now())

	err = g.back.CancelExpense(ctx, id, "por error", g.admin, true)
	if !errors.Is(err, domain.ErrExpenseInClosedCount) || !strings.Contains(err.Error(), "devolución") {
		t.Fatalf("anular con un pago en corte cerrado = %v; quería EXPENSE_IN_CLOSED_COUNT diciendo que es una devolución", err)
	}
	if a := g.anulado(t, id); a.status != "pagada" {
		t.Fatalf("el rechazo dejó el gasto %q", a.status)
	}
	if p := g.pago(t, id); p.session == nil || *p.session != sess || g.salidas(t, pay) != 1 {
		t.Fatal("el rechazo movió el pago o su salida de un corte cerrado")
	}
	if g.eventos(t, "expense.cancelled") != 0 {
		t.Fatal("un rechazo dejó evento de anulación")
	}
}

// PENDIENTE DE CORTE: un pago incluido que espera la siguiente apertura deja de esperarla; abrir la
// caja después no descuenta un gasto que no existe.
func TestAnularUnGastoConPagoQueEsperaCorte(t *testing.T) {
	t.Parallel()
	g := nuevoArqueoPorHora(t)
	ctx := context.Background()
	g.cerrar(t, g.turno(t, g.primar, time.Now().Add(-6*time.Hour)), time.Now().Add(-5*time.Hour))
	id, err := g.back.CreateExpense(ctx, g.gasto(g.primar, hoyDelNegocio(), incluir(true), true, "200"))
	if err != nil {
		t.Fatal(err)
	}
	if p := g.pago(t, id); p.session != nil || !p.include {
		t.Fatalf("el pago debía quedar esperando corte: %+v", p)
	}
	if err := g.back.CancelExpense(ctx, id, "capturado dos veces", g.admin, true); err != nil {
		t.Fatalf("anular con un pago que espera corte: %v", err)
	}
	if p := g.pago(t, id); p.session != nil || p.include {
		t.Fatalf("el pago anulado sigue esperando corte: %+v", p)
	}
	vista, err := g.back.OpenSession(ctx, g.primar, aperturaAMano(dec("500")), g.admin)
	if err != nil {
		t.Fatal(err)
	}
	if g.neto(t, vista.ID) != "0.00" {
		t.Fatalf("abrir la caja descontó el gasto anulado: neto %s", g.neto(t, vista.ID))
	}
}

// DOBLE ANULACIÓN Y SIN MOTIVO: la segunda es un 409 que no deja otro evento; sin motivo es 400 y nada
// cambia.
func TestAnularDosVecesOSinMotivo(t *testing.T) {
	t.Parallel()
	g := nuevoArqueoPorHora(t)
	ctx := context.Background()
	id, err := g.back.CreateExpense(ctx, g.gasto(g.primar, hoyDelNegocio(), incluir(false), false, "50"))
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range []string{"", "   "} {
		if err := g.back.CancelExpense(ctx, id, r, g.admin, true); !errors.Is(err, domain.ErrValidation) {
			t.Fatalf("anular con motivo %q = %v; quería ErrValidation", r, err)
		}
	}
	if a := g.anulado(t, id); a.status != "pagada" {
		t.Fatalf("anular sin motivo dejó el gasto %q", a.status)
	}
	if err := g.back.CancelExpense(ctx, id, "era una propina", g.admin, true); err != nil {
		t.Fatal(err)
	}
	if err := g.back.CancelExpense(ctx, id, "otra vez", g.admin, true); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("anular dos veces = %v; quería ErrConflict", err)
	}
	if a := g.anulado(t, id); a.reason == nil || *a.reason != "era una propina" {
		t.Fatal("la segunda anulación reescribió el motivo de la primera")
	}
	if g.eventos(t, "expense.cancelled") != 1 {
		t.Fatalf("la segunda anulación dejó otro evento: %d", g.eventos(t, "expense.cancelled"))
	}
}

// CON MERCANCÍA RECIBIDA Y PAGADO: lo que sumó al almacén se revierte con movimientos nuevos (la regla
// de anular de la 038), sin borrar ninguno.
func TestAnularUnGastoPagadoConMercanciaLaRevierte(t *testing.T) {
	t.Parallel()
	k := newPurchaseKit(t)
	var card int16
	if err := k.st.Pool.QueryRow(context.Background(), `select id from payment_methods where company_id = $1 and not affects_cash_drawer
		and is_active and delivery_platform_id is null order by id limit 1`, defaultCompanyID).Scan(&card); err != nil {
		t.Fatal(err)
	}
	in := k.bultos("1", "500")
	in.Status = domain.ExpensePagada
	in.Payments = []app.ExpensePaymentInput{{MethodID: card, Amount: dd("500"), PaidOn: "2026-07-20"}}
	id, err := k.svc.CreateExpense(k.ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	if got := k.onHand(t); got.IsZero() {
		t.Fatal("la compra no sumó al almacén")
	}
	if err := k.svc.CancelExpense(k.ctx, id, "no era de este local", k.user, true); err != nil {
		t.Fatalf("anular un gasto pagado con mercancía: %v", err)
	}
	if got := k.onHand(t); !got.IsZero() {
		t.Fatalf("anulado, quedan %s g en el almacén", got)
	}
	var n int
	_ = k.st.Pool.QueryRow(context.Background(), `select count(*) from stock_movements where expense_id = $1`, id).Scan(&n)
	if n != 2 {
		t.Errorf("movimientos = %d; quiere 2 (la entrada y su reversión, ninguno borrado)", n)
	}
}

// POR PERMISO, NO POR ROL: sin `expenses.cancel` es 403 y nada cambia; un cajero tampoco puede. Con el
// permiso, el detalle dice quién, cuándo y por qué.
func TestAnularUnGastoExigeElPermiso(t *testing.T) {
	t.Parallel()
	g := nuevoArqueoPorHora(t)
	ctx := context.Background()
	id, err := g.back.CreateExpense(ctx, g.gasto(g.primar, hoyDelNegocio(), incluir(false), false, "70"))
	if err != nil {
		t.Fatal(err)
	}
	sinAnular := func(r domain.Role) []domain.Permission {
		var out []domain.Permission
		for _, p := range domain.PermissionsFor(r) {
			if p != domain.PermExpensesCancel {
				out = append(out, p)
			}
		}
		return out
	}
	path := "/api/v1/expenses/" + itoa(int(id)) + "/cancel"
	body := []byte(`{"reason":"era una propina"}`)
	cajero := makeUser(t, g.st, "cajero_216", "cajero")
	for _, c := range []struct {
		name  string
		perms func(domain.Role) []domain.Permission
		user  domain.User
	}{
		{"admin sin el permiso", sinAnular, domain.User{ID: g.admin, CompanyID: defaultCompanyID, Name: "a", Role: domain.RoleAdmin}},
		{"cajero", nil, domain.User{ID: cajero, CompanyID: defaultCompanyID, Name: "c", Role: domain.RoleCajero}},
	} {
		t.Run(c.name, func(t *testing.T) {
			r, jm := routerDeArqueo(t, g.st, c.perms)
			tok, err := jm.Issue(c.user)
			if err != nil {
				t.Fatal(err)
			}
			if w := do(t, r, http.MethodPost, path, tok, body, "application/json"); w.Code != http.StatusForbidden {
				t.Fatalf("anular = %d (%s); quería 403", w.Code, w.Body.String())
			}
		})
	}
	if a := g.anulado(t, id); a.status != "pagada" {
		t.Fatal("se anuló sin el permiso")
	}
	r, jm := routerDeArqueo(t, g.st, nil)
	tok, _ := jm.Issue(domain.User{ID: g.admin, CompanyID: defaultCompanyID, Name: "a", Role: domain.RoleGerente})
	if w := do(t, r, http.MethodPost, path, tok, []byte(`{"reason":""}`), "application/json"); w.Code != http.StatusBadRequest {
		t.Fatalf("anular sin motivo por HTTP = %d; quería 400", w.Code)
	}
	if w := do(t, r, http.MethodPost, path, tok, body, "application/json"); w.Code != http.StatusNoContent {
		t.Fatalf("anular con el permiso = %d: %s", w.Code, w.Body.String())
	}
	w := do(t, r, http.MethodGet, "/api/v1/expenses/"+itoa(int(id)), tok, nil, "")
	for _, want := range []string{`"cancelReason":"era una propina"`, `"cancelledBy":"`, `"cancelledAt":"`} {
		if !strings.Contains(w.Body.String(), want) {
			t.Fatalf("el detalle del gasto anulado no trae %s: %s", want, w.Body.String())
		}
	}
	if w := do(t, r, http.MethodPost, path, tok, body, "application/json"); w.Code != http.StatusConflict {
		t.Fatalf("anular dos veces por HTTP = %d; quería 409", w.Code)
	}
	cerrado := g.turno(t, g.primar, time.Now().Add(-time.Hour))
	otro, err := g.back.CreateExpense(ctx, g.gasto(g.primar, hoyDelNegocio(), incluir(true), false, "30"))
	if err != nil {
		t.Fatal(err)
	}
	g.cerrar(t, cerrado, time.Now())
	w = do(t, r, http.MethodPost, "/api/v1/expenses/"+itoa(int(otro))+"/cancel", tok, body, "application/json")
	if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "EXPENSE_IN_CLOSED_COUNT") || !strings.Contains(w.Body.String(), "devolución") {
		t.Fatalf("anular con corte cerrado por HTTP = %d %s; quería 409 EXPENSE_IN_CLOSED_COUNT", w.Code, w.Body.String())
	}
}

// Anular no alcanza el gasto de otra empresa, en los tres casos donde RLS ya falló.
func TestAnularNoAlcanzaElGastoDeOtraEmpresa(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()
	coB := makeCompany(t, st, "anular-216-b")
	userB := makeUserIn(t, st, coB, "admin_216_anular", "admin")
	var catB, expB int64
	var cashB int16
	_ = st.Pool.QueryRow(ctx, `insert into expense_categories (company_id, name, financial_group) values ($1, 'B', 'operacional') returning id`, coB).Scan(&catB)
	_ = st.Pool.QueryRow(ctx, `insert into expenses (company_id, category_id, amount, expense_date, created_by, status, paid_at, paid_by)
		values ($1, $2, 10, current_date, $3, 'pagada', now(), $3) returning id`, coB, catB, userB).Scan(&expB)
	_ = st.Pool.QueryRow(ctx, `select id from payment_methods where company_id = $1 and affects_cash_drawer limit 1`, coB).Scan(&cashB)
	if _, err := st.Pool.Exec(ctx, `insert into expense_payments (company_id, expense_id, payment_method_id, amount, paid_on, paid_by)
		values ($1, $2, $3, 10, current_date, $4)`, coB, expB, cashB, userB); err != nil {
		t.Fatal(err)
	}
	inTheThreeCases(t, coB, defaultCompanyID, func(t *testing.T, ast *store.Store, actx context.Context) {
		if err := app.NewBackofficeService(ast, time.Now).CancelExpense(actx, expB, "ajeno", userB, true); err == nil {
			t.Fatal("se anuló un gasto de otra empresa")
		}
		var status string
		if err := st.Pool.QueryRow(ctx, `select status::text from expenses where id = $1`, expB).Scan(&status); err != nil || status != "pagada" {
			t.Fatalf("el gasto de otra empresa quedó %q (%v)", status, err)
		}
	})
}

// Revisión de seguridad del #216: sacar un pago del arqueo cambia el esperado del cajón, igual que la
// casilla del arqueo. Un rol con `expenses.cancel` y sin `cash.adjust_count` no puede hacerlo por esta
// puerta; un gasto cuyo pago no está en ningún arqueo sí lo anula.
func TestAnularUnPagoDelArqueoExigeAjustarElArqueo(t *testing.T) {
	t.Parallel()
	g := nuevoArqueoPorHora(t)
	ctx := context.Background()
	sess := g.turno(t, g.primar, time.Now().Add(-time.Hour))
	enCorte, err := g.back.CreateExpense(ctx, g.gasto(g.primar, hoyDelNegocio(), incluir(true), false, "500"))
	if err != nil {
		t.Fatal(err)
	}
	fuera, err := g.back.CreateExpense(ctx, g.gasto(g.primar, hoyDelNegocio(), incluir(false), false, "20"))
	if err != nil {
		t.Fatal(err)
	}
	soloAnular := func(domain.Role) []domain.Permission { return []domain.Permission{domain.PermExpensesCancel} }
	r, jm := routerDeArqueo(t, g.st, soloAnular)
	tok, err := jm.Issue(domain.User{ID: g.admin, CompanyID: defaultCompanyID, Name: "contador", Role: domain.RoleGerente})
	if err != nil {
		t.Fatal(err)
	}
	body := []byte(`{"reason":"error"}`)
	w := do(t, r, http.MethodPost, "/api/v1/expenses/"+itoa(int(enCorte))+"/cancel", tok, body, "application/json")
	if w.Code != http.StatusForbidden || !strings.Contains(w.Body.String(), "arqueo") {
		t.Fatalf("anular un pago del arqueo sin cash.adjust_count = %d %s; quería 403", w.Code, w.Body.String())
	}
	if g.anulado(t, enCorte).status != "pagada" || g.neto(t, sess) != "-500.00" {
		t.Fatalf("el 403 movió algo: estado %q, neto %s", g.anulado(t, enCorte).status, g.neto(t, sess))
	}
	if g.salidas(t, g.pago(t, enCorte).id) != 1 {
		t.Fatal("el 403 borró la salida del cajón")
	}
	if w := do(t, r, http.MethodPost, "/api/v1/expenses/"+itoa(int(fuera))+"/cancel", tok, body, "application/json"); w.Code != http.StatusNoContent {
		t.Fatalf("anular un gasto fuera del arqueo solo con expenses.cancel = %d %s", w.Code, w.Body.String())
	}
}

// Un pago con tarjeta atribuido a un corte abierto sale del corte sin tocar el cajón (no tiene salida),
// y uno con «salió también lo que no es del negocio» (spec 050) devuelve al esperado el importe completo
// de su salida; la bitácora lo dice para poder reconstruir el esperado.
func TestAnularSacaTarjetaYDiferenciaDelCorteAbierto(t *testing.T) {
	t.Parallel()
	g := nuevoArqueoPorHora(t)
	ctx := context.Background()
	sess := g.turno(t, g.primar, time.Now().Add(-time.Hour))
	id, err := g.back.CreateExpense(ctx, g.gasto(g.primar, hoyDelNegocio(), incluir(true), false, "100"))
	if err != nil {
		t.Fatal(err)
	}
	pay := g.pago(t, id).id
	// La diferencia que salió del cajón (spec 050): el trigger de la 0102 fija la salida en importe + extra.
	if _, err := g.st.Pool.Exec(ctx, `update expense_payments set drawer_extra = 40 where id = $1`, pay); err != nil {
		t.Fatal(err)
	}
	if _, err := g.st.Pool.Exec(ctx, `update register_cash_movements set amount = 140 where expense_payment_id = $1`, pay); err != nil {
		t.Fatal(err)
	}
	var tarjeta int64
	if err := g.st.Pool.QueryRow(ctx, `insert into expense_payments (expense_id, payment_method_id, amount, paid_on, register_session_id, paid_by)
		values ($1, $2, 5, current_date, $3, $4) returning id`, id, g.card, sess, g.admin).Scan(&tarjeta); err != nil {
		t.Fatal(err)
	}
	if g.neto(t, sess) != "-140.00" {
		t.Fatalf("preparación: neto %s; quería −140", g.neto(t, sess))
	}
	if err := g.back.CancelExpense(ctx, id, "por error", g.admin, true); err != nil {
		t.Fatalf("anular con tarjeta y diferencia en corte abierto: %v", err)
	}
	if g.neto(t, sess) != "0.00" {
		t.Fatalf("el corte abierto sigue con neto %s; la salida completa (importe + diferencia) debía volver", g.neto(t, sess))
	}
	var enCorte int
	_ = g.st.Pool.QueryRow(ctx, `select count(*) from expense_payments where expense_id = $1 and register_session_id is not null`, id).Scan(&enCorte)
	if enCorte != 0 {
		t.Fatalf("%d pagos siguen atribuidos al corte abierto (la tarjeta también sale de «Gastos» del corte)", enCorte)
	}
	var extra, removed string
	if err := g.st.Pool.QueryRow(ctx, `select p->>'drawerExtra', p->>'removedSalida' from audit_events,
		jsonb_array_elements(before->'payments') p where action = 'expense.cancelled' and entity_id = $1 and (p->>'paymentId')::bigint = $2`, id, pay).
		Scan(&extra, &removed); err != nil {
		t.Fatalf("el evento no trae el pago: %v", err)
	}
	if extra != "40" || removed != "true" {
		t.Fatalf("el evento dice drawerExtra %q y removedSalida %q; quería 40 y true", extra, removed)
	}
}

// LA CARRERA CONTRA EL CIERRE: si el cierre gana, la anulación lo ve cerrado y se niega; la salida se
// queda en el corte que ya se contó.
func TestAnularQueEsperaAlCierreSeNiega(t *testing.T) {
	t.Parallel()
	g := nuevoArqueoPorHora(t)
	ctx := context.Background()
	sess := g.turno(t, g.primar, time.Now().Add(-time.Hour))
	id, err := g.back.CreateExpense(ctx, g.gasto(g.primar, hoyDelNegocio(), incluir(true), false, "150"))
	if err != nil {
		t.Fatal(err)
	}
	pay := g.pago(t, id).id
	tx, err := g.st.Pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `select id from register_sessions where id = $1 for update`, sess); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- g.back.CancelExpense(ctx, id, "por error", g.admin, true) }()
	esperarBloqueo(t, g.st)
	if _, err := tx.Exec(ctx, `update register_sessions set status = 'cerrada', closed_at = now(), closed_by = $2 where id = $1`, sess, g.admin); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err := <-done; !errors.Is(err, domain.ErrExpenseInClosedCount) {
		t.Fatalf("la anulación que esperó al cierre dio %v; quería EXPENSE_IN_CLOSED_COUNT", err)
	}
	if g.salidas(t, pay) != 1 || g.anulado(t, id).status != "pagada" {
		t.Fatal("la anulación movió algo de un corte ya cerrado")
	}
}
