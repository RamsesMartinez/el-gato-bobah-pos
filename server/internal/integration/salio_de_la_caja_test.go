//go:build integration

package integration

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
)

// LO QUE SALIÓ DE LA CAJA (spec 050). Un ticket de $500 trae $120 que no son del negocio: el gasto
// es de $380, pero del cajón salieron $500. La diferencia sale del corte con el gasto, una sola vez,
// y el corte la presenta aparte de «Gastos».

// mixto es un gasto de $380 con un renglón excluido de $120, pagado en efectivo desde `reg` con
// `extra` de diferencia. Sin pagos si `amount` es vacío (queda pendiente).
func (g arqueoPorHora) mixto(reg int64, include *bool, confirm bool, amount, extra string) app.ExpenseInput {
	in := app.ExpenseInput{
		CategoryID: g.cat, Amount: dec("380"), Status: domain.ExpensePendiente, UserID: g.admin, Description: "Ticket mixto",
		Items: []app.ExpenseItemInput{
			{Description: "Papel y bolsas", Quantity: dec("1"), Amount: dec("380")},
			{Description: "Despensa de la casa", Quantity: dec("1"), Amount: dec("120"), Excluded: true},
		},
	}
	if amount != "" {
		in.Payments = []app.ExpensePaymentInput{{MethodID: g.cash, Amount: dec(amount), DrawerExtra: dec(extra),
			PaidOn: hoyDelNegocio(), RegisterID: &reg, IncludeInCount: include, OutsideCount: confirm}}
		if dec(amount).Equal(dec("380")) {
			in.Status = domain.ExpensePagada
		}
	}
	return in
}

// egreso es el renglón `concept` de los egresos del corte abierto de `reg`; "" si no está.
func (g arqueoPorHora) egreso(t *testing.T, reg int64, concept string) string {
	t.Helper()
	v, err := g.back.CurrentByRegister(context.Background(), reg)
	if err != nil || v == nil {
		t.Fatalf("corte abierto: %v", err)
	}
	for _, b := range v.Breakdown.Egresos {
		if b.Concept == concept {
			return b.Amount.StringFixed(2)
		}
	}
	return ""
}

// EL ESPERADO BAJA LO QUE SALIÓ, EL GASTO CONSERVA SU TOTAL, Y LA DIFERENCIA NO SE CUENTA COMO GASTO.
func TestLoQueNoEsDelNegocioSaleDelCorteUnaSolaVez(t *testing.T) {
	t.Parallel()
	g := nuevoArqueoPorHora(t)
	ctx := context.Background()
	sess := g.turno(t, g.primar, time.Now().Add(-time.Hour))
	id, err := g.back.CreateExpense(ctx, g.mixto(g.primar, incluir(true), false, "380", "120"))
	if err != nil {
		t.Fatalf("guardar con salió de la caja $500: %v", err)
	}
	if n := g.neto(t, sess); n != "-500.00" {
		t.Fatalf("el esperado bajó %s; quería −500: la parte que no es del negocio también salió del cajón", n)
	}
	pay := g.pago(t, id).id
	if n := g.salidas(t, pay); n != 1 {
		t.Fatalf("el pago dejó %d salidas; la diferencia va en la misma, no en otra", n)
	}
	var total string
	if err := g.st.Pool.QueryRow(ctx, `select amount::text from expenses where id = $1`, id).Scan(&total); err != nil || total != "380.00" {
		t.Fatalf("el gasto quedó en %s (%v); su total no cambia", total, err)
	}
	if got := g.egreso(t, g.primar, "Gastos"); got != "380.00" {
		t.Fatalf("«Gastos» del corte = %s; quería 380: la parte que no es del negocio no es gasto", got)
	}
	if got := g.egreso(t, g.primar, "No es del negocio"); got != "120.00" {
		t.Fatalf("«No es del negocio» del corte = %q; quería 120", got)
	}
	v, _ := g.back.CurrentByRegister(ctx, g.primar)
	if len(v.Expenses) != 1 || !v.Expenses[0].DrawerExtra.Equal(dec("120")) {
		t.Fatalf("los gastos del corte no traen la diferencia: %+v", v.Expenses)
	}
	det, err := g.back.ExpenseDetail(ctx, id)
	if err != nil || len(det.Payments) != 1 || !det.Payments[0].DrawerExtra.Equal(dec("120")) {
		t.Fatalf("el detalle no trae la diferencia del pago: %+v, %v", det.Payments, err)
	}

	// Desmarcar sube todo lo que salió; volver a marcar lo baja otra vez, completo.
	if err := g.back.SetPaymentInCount(ctx, id, pay, false, g.admin); err != nil {
		t.Fatal(err)
	}
	if n := g.neto(t, sess); n != "0.00" {
		t.Fatalf("desmarcar dejó el neto en %s; quería 0", n)
	}
	if err := g.back.SetPaymentInCount(ctx, id, pay, true, g.admin); err != nil {
		t.Fatal(err)
	}
	if n := g.neto(t, sess); n != "-500.00" {
		t.Fatalf("volver a marcar dejó el neto en %s; quería −500", n)
	}
}

// Lo que no es del negocio sale del corte, pero no de «Gastos»: si lo contara ahí, el resumen
// reportaría $500 de gasto por un gasto de $380.
func TestLoQueNoEsDelNegocioNoSeCuentaComoGasto(t *testing.T) {
	t.Parallel()
	g := nuevoArqueoPorHora(t)
	ctx := context.Background()
	g.turno(t, g.primar, time.Now().Add(-time.Hour))
	if _, err := g.back.CreateExpense(ctx, g.mixto(g.primar, incluir(true), false, "380", "0")); err != nil {
		t.Fatal(err)
	}
	if got := g.egreso(t, g.primar, "No es del negocio"); got != "" {
		t.Fatalf("sin diferencia el corte muestra «No es del negocio» = %s", got)
	}
	if got := g.egreso(t, g.primar, "Gastos"); got != "380.00" {
		t.Fatalf("«Gastos» = %s; quería 380", got)
	}
}

// UN PENDIENTE ENTRA AL CORTE CON TODO LO QUE SALIÓ (US1, escenario 4).
func TestUnPendienteConDiferenciaEntraCompleto(t *testing.T) {
	t.Parallel()
	g := nuevoArqueoPorHora(t)
	ctx := context.Background()
	g.cerrar(t, g.turno(t, g.primar, time.Now().Add(-6*time.Hour)), time.Now().Add(-5*time.Hour))
	if _, err := g.back.CreateExpense(ctx, g.mixto(g.primar, incluir(true), true, "380", "120")); err != nil {
		t.Fatal(err)
	}
	pend, err := g.back.PendingExpenses(ctx, g.primar)
	if err != nil || len(pend) != 1 || !pend[0].DrawerExtra.Equal(dec("120")) {
		t.Fatalf("la lista de pendientes no trae la diferencia: %+v, %v", pend, err)
	}
	desde := time.Now().Add(-3 * time.Hour)
	cmd := aperturaAMano(dec("500"))
	cmd.OpenedAt = &desde
	vista, err := g.back.OpenSession(ctx, g.primar, cmd, g.admin)
	if err != nil {
		t.Fatal(err)
	}
	if n := g.neto(t, vista.ID); n != "-500.00" {
		t.Fatalf("el pendiente entró con neto %s; quería −500, no solo el importe", n)
	}
}

// EL TOPE: importe + excluidos; con tarjeta, sin incluir o sin excluidos no hay diferencia (R1, R5).
func TestLaDiferenciaTieneTope(t *testing.T) {
	t.Parallel()
	g := nuevoArqueoPorHora(t)
	ctx := context.Background()
	g.turno(t, g.primar, time.Now().Add(-time.Hour))
	if _, err := g.back.CreateExpense(ctx, g.mixto(g.primar, incluir(true), false, "380", "120.01")); !errors.Is(err, domain.ErrDrawerExtraTooHigh) {
		t.Fatalf("pasar de lo excluido: %v; quería DRAWER_EXTRA_TOO_HIGH", err)
	}
	if _, err := g.back.CreateExpense(ctx, g.mixto(g.primar, incluir(true), false, "380", "-1")); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("diferencia negativa: %v; quería 400", err)
	}
	card := g.mixto(g.primar, incluir(true), false, "380", "10")
	card.Payments[0].MethodID = g.card
	if _, err := g.back.CreateExpense(ctx, card); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("diferencia con tarjeta: %v; quería 400", err)
	}
	if _, err := g.back.CreateExpense(ctx, g.mixto(g.primar, incluir(false), false, "380", "10")); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("diferencia sin incluir en el arqueo: %v; quería 400", err)
	}
	ext := g.caja(t, "Externa 050", false)
	if _, err := g.back.CreateExpense(ctx, g.mixto(ext, nil, false, "380", "10")); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("diferencia en caja sin arqueo: %v; quería 400", err)
	}
	sin := g.mixto(g.primar, incluir(true), false, "380", "10")
	sin.Items = sin.Items[:1]
	if _, err := g.back.CreateExpense(ctx, sin); !errors.Is(err, domain.ErrDrawerExtraTooHigh) {
		t.Fatalf("diferencia sin excluidos: %v; quería DRAWER_EXTRA_TOO_HIGH", err)
	}
}

// DOS «+1 PAGO» A LA VEZ NO USAN DOS VECES LA MISMA PARTE (R1, B5).
func TestDosPagosNoUsanDosVecesLoExcluido(t *testing.T) {
	t.Parallel()
	g := nuevoArqueoPorHora(t)
	ctx := context.Background()
	sess := g.turno(t, g.primar, time.Now().Add(-time.Hour))
	in := g.mixto(g.primar, nil, false, "", "")
	// Lo del negocio por debajo del total: volver a incluir el excluido cabría en él (regla de la
	// 038), y lo que lo impide es solo lo ya usado como diferencia.
	in.Items[0].Amount = dec("200")
	id, err := g.back.CreateExpense(ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	pago := app.ExpensePaymentInput{MethodID: g.cash, Amount: dec("100"), DrawerExtra: dec("80"), PaidOn: hoyDelNegocio(),
		RegisterID: &g.primar, IncludeInCount: incluir(true)}
	errs := make([]error, 2)
	var wg sync.WaitGroup
	for i := range errs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs[i] = g.back.AddExpensePayment(ctx, id, pago, g.admin)
		}()
	}
	wg.Wait()
	ok, tope := 0, 0
	for _, e := range errs {
		switch {
		case e == nil:
			ok++
		case errors.Is(e, domain.ErrDrawerExtraTooHigh):
			tope++
		default:
			t.Fatalf("pago a la vez: %v", e)
		}
	}
	if ok != 1 || tope != 1 {
		t.Fatalf("dos pagos de $80 de diferencia sobre $120 excluidos: %d pasaron, %d topados; quería 1 y 1", ok, tope)
	}
	// Lo que queda (40) sí se puede usar.
	pago.DrawerExtra = dec("40")
	if err := g.back.AddExpensePayment(ctx, id, pago, g.admin); err != nil {
		t.Fatalf("usar lo que queda: %v", err)
	}
	if n := g.neto(t, sess); n != "-320.00" {
		t.Fatalf("neto %s; quería −320 (dos pagos de 100 + 80 + 40)", n)
	}

	// Volver a incluir el renglón excluido dejaría lo usado sin respaldo: se rechaza.
	var item int64
	if err := g.st.Pool.QueryRow(ctx, `select id from expense_items where expense_id = $1 and excluded`, id).Scan(&item); err != nil {
		t.Fatal(err)
	}
	if _, err := g.back.SetItemExcluded(ctx, id, item, false, g.admin); !errors.Is(err, domain.ErrDrawerExtraTooHigh) {
		t.Fatalf("incluir un renglón usado como diferencia: %v; quería DRAWER_EXTRA_TOO_HIGH", err)
	}
}

// Anular un gasto pendiente cuyo pago con diferencia espera corte: ningún corte lo toma.
func TestAnularUnPendienteConDiferencia(t *testing.T) {
	t.Parallel()
	g := nuevoArqueoPorHora(t)
	ctx := context.Background()
	g.cerrar(t, g.turno(t, g.primar, time.Now().Add(-6*time.Hour)), time.Now().Add(-5*time.Hour))
	id, err := g.back.CreateExpense(ctx, g.mixto(g.primar, incluir(true), true, "100", "120"))
	if err != nil {
		t.Fatal(err)
	}
	if err := g.back.CancelExpense(ctx, id, "prueba", g.admin); err != nil {
		t.Fatalf("anular: %v", err)
	}
	desde := time.Now().Add(-3 * time.Hour)
	cmd := aperturaAMano(dec("500"))
	cmd.OpenedAt = &desde
	vista, err := g.back.OpenSession(ctx, g.primar, cmd, g.admin)
	if err != nil {
		t.Fatal(err)
	}
	if n := g.neto(t, vista.ID); n != "0.00" {
		t.Fatalf("el pago de un gasto anulado entró al corte: neto %s", n)
	}
}

// EL CONTRATO: `drawerExtra` viaja en el JSON del «+1 pago», pasar el tope responde 400 con su
// código, y el detalle lo devuelve.
func TestLaDiferenciaPorHTTP(t *testing.T) {
	t.Parallel()
	g := nuevoArqueoPorHora(t)
	ctx := context.Background()
	g.turno(t, g.primar, time.Now().Add(-time.Hour))
	id, err := g.back.CreateExpense(ctx, g.mixto(g.primar, nil, false, "", ""))
	if err != nil {
		t.Fatal(err)
	}
	r, jm := routerDeArqueo(t, g.st, nil)
	tok, _ := jm.Issue(domain.User{ID: g.admin, CompanyID: defaultCompanyID, Name: "a", Role: domain.RoleAdmin})
	cuerpo := func(extra string) []byte {
		return []byte(`{"methodId":` + itoa(int(g.cash)) + `,"amount":"380","registerId":` + itoa(int(g.primar)) +
			`,"includeInCount":true,"drawerExtra":"` + extra + `"}`)
	}
	url := "/api/v1/expenses/" + itoa(int(id)) + "/pay"
	if w := do(t, r, http.MethodPost, url, tok, cuerpo("121"), "application/json"); w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "DRAWER_EXTRA_TOO_HIGH") {
		t.Fatalf("pasar el tope = %d %s; quería 400 DRAWER_EXTRA_TOO_HIGH", w.Code, w.Body.String())
	}
	if w := do(t, r, http.MethodPost, url, tok, cuerpo("120"), "application/json"); w.Code >= 300 {
		t.Fatalf("pago con diferencia = %d %s", w.Code, w.Body.String())
	}
	w := do(t, r, http.MethodGet, "/api/v1/expenses/"+itoa(int(id)), tok, nil, "")
	if !strings.Contains(w.Body.String(), `"drawerExtra":"120"`) {
		t.Fatalf("el detalle no trae drawerExtra: %s", w.Body.String())
	}
}

// CON TOTAL DEL TICKET, LO QUE SALIÓ NO PASA DE ÉL: un ticket de $450 con $380 del negocio deja $70
// de diferencia, aunque los excluidos sumen $120.
func TestElTotalDelTicketTopaLaDiferencia(t *testing.T) {
	t.Parallel()
	g := nuevoArqueoPorHora(t)
	ctx := context.Background()
	g.turno(t, g.primar, time.Now().Add(-time.Hour))
	ticket := dec("450")
	in := g.mixto(g.primar, incluir(true), false, "380", "70.01")
	in.DocumentTotal = &ticket
	if _, err := g.back.CreateExpense(ctx, in); !errors.Is(err, domain.ErrDrawerExtraTooHigh) {
		t.Fatalf("pasar del total del ticket: %v; quería DRAWER_EXTRA_TOO_HIGH", err)
	}
	in.Payments[0].DrawerExtra = dec("70")
	if _, err := g.back.CreateExpense(ctx, in); err != nil {
		t.Fatalf("justo en el total del ticket: %v", err)
	}
}

// CORREGIR LOS RENGLONES NO DEJA LO USADO SIN RESPALDO (R2): bajar el importe de un excluido ya
// usado como diferencia se rechaza igual que volver a incluirlo.
func TestCorregirRenglonesRespetaLaDiferencia(t *testing.T) {
	t.Parallel()
	g := nuevoArqueoPorHora(t)
	ctx := context.Background()
	g.turno(t, g.primar, time.Now().Add(-time.Hour))
	ing := ingredientIn(t, g.st, defaultCompanyID, "Despensa 050", "g")
	unit := unitID(t, g.st, "g")
	in := g.mixto(g.primar, incluir(true), false, "380", "120")
	in.Items[1] = app.ExpenseItemInput{ItemType: "ingrediente", IngredientID: &ing, UnitID: &unit, Description: "Despensa de la casa",
		Quantity: dec("1000"), Amount: dec("120"), Excluded: true}
	id, err := g.back.CreateExpense(ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	var item int64
	if err := g.st.Pool.QueryRow(ctx, `select id from expense_items where expense_id = $1 and excluded`, id).Scan(&item); err != nil {
		t.Fatal(err)
	}
	zero := dec("0")
	_, err = g.back.SaveStockLines(ctx, id, []app.StockLineInput{{ID: &item, SeenApplied: &zero, ItemType: "ingrediente",
		IngredientID: &ing, Description: "Despensa de la casa", Quantity: dec("1000"), UnitID: &unit, Amount: dec("50")}}, g.admin)
	if !errors.Is(err, domain.ErrDrawerExtraTooHigh) {
		t.Fatalf("bajar el excluido por debajo de lo usado: %v; quería DRAWER_EXTRA_TOO_HIGH", err)
	}
}

// EL MONTO DE LA SALIDA LO PONE EL PAGO (R3): un INSERT que manda otro monto queda en importe +
// diferencia; sin diferencia, en el importe.
func TestLaSalidaDeUnPagoLaFijaLaBase(t *testing.T) {
	t.Parallel()
	g := nuevoArqueoPorHora(t)
	ctx := context.Background()
	sess := g.turno(t, g.primar, time.Now().Add(-time.Hour))
	for _, c := range []struct{ extra, want string }{{"0", "380.00"}, {"120", "500.00"}} {
		id, err := g.back.CreateExpense(ctx, g.mixto(g.primar, incluir(true), false, "380", c.extra))
		if err != nil {
			t.Fatal(err)
		}
		pay := g.pago(t, id).id
		if _, err := g.st.Pool.Exec(ctx, `delete from register_cash_movements where expense_payment_id = $1`, pay); err != nil {
			t.Fatal(err)
		}
		var amount string
		if err := g.st.Pool.QueryRow(ctx, `insert into register_cash_movements (session_id, kind, amount, concept, user_id, expense_payment_id)
			values ($1, 'salida', 9999, 'a mano', $2, $3) returning amount::text`, sess, g.admin, pay).Scan(&amount); err != nil {
			t.Fatal(err)
		}
		if amount != c.want {
			t.Fatalf("con diferencia %s la salida quedó en %s; quería %s: el monto del INSERT no manda", c.extra, amount, c.want)
		}
	}
}
