//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
)

// gastoSinTurno arma lo que comparten las pruebas de la spec 047: una categoría, el método en
// efectivo de la empresa por omisión y un administrador.
type gastoSinTurno struct {
	st     *store.Store
	back   *app.BackofficeService
	admin  int64
	cat    int64
	cash   int16
	card   int16
	primar int64
}

func nuevoGastoSinTurno(t *testing.T, now time.Time) gastoSinTurno {
	t.Helper()
	st := newTestStore(t)
	ctx := context.Background()
	g := gastoSinTurno{st: st, back: app.NewBackofficeService(st, clockAt(now)), primar: principalRegister(t, st)}
	g.admin = makeUser(t, st, "admin_047_"+now.Format("150405"), "admin")
	if err := st.Pool.QueryRow(ctx, `insert into expense_categories (name, financial_group) values ('Gasto 047', 'operacional') returning id`).Scan(&g.cat); err != nil {
		t.Fatal(err)
	}
	if err := st.Pool.QueryRow(ctx, `select id from payment_methods where company_id = $1 and affects_cash_drawer and is_active order by id limit 1`, defaultCompanyID).Scan(&g.cash); err != nil {
		t.Fatalf("método en efectivo: %v", err)
	}
	if err := st.Pool.QueryRow(ctx, `select id from payment_methods where company_id = $1 and not affects_cash_drawer and is_active and delivery_platform_id is null order by id limit 1`, defaultCompanyID).Scan(&g.card); err != nil {
		t.Fatalf("método sin cajón: %v", err)
	}
	return g
}

func (g gastoSinTurno) caja(t *testing.T, name string, counts bool) int64 {
	t.Helper()
	var id int64
	if err := g.st.Pool.QueryRow(context.Background(), `insert into cash_registers (name, counts_cash) values ($1, $2) returning id`, name, counts).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func (g gastoSinTurno) pagar(reg int64, paidOn string, outside bool) app.ExpenseInput {
	return app.ExpenseInput{
		CategoryID: g.cat, Amount: dec("150"), Status: domain.ExpensePagada, UserID: g.admin,
		Payments: []app.ExpensePaymentInput{{MethodID: g.cash, Amount: dec("150"), PaidOn: paidOn, RegisterID: &reg, OutsideCount: outside}},
	}
}

type pagoGuardado struct {
	register *int64
	session  *int64
}

func (g gastoSinTurno) pagoDe(t *testing.T, expense int64) pagoGuardado {
	t.Helper()
	var p pagoGuardado
	if err := g.st.Pool.QueryRow(context.Background(), `select register_id, register_session_id from expense_payments where expense_id = $1`, expense).Scan(&p.register, &p.session); err != nil {
		t.Fatal(err)
	}
	return p
}

// movimientosDe cuenta las salidas de caja de un turno: es de donde sale el esperado en efectivo.
func (g gastoSinTurno) movimientosDe(t *testing.T, sess int64) int {
	t.Helper()
	var n int
	if err := g.st.Pool.QueryRow(context.Background(), `select count(*) from register_cash_movements where session_id = $1`, sess).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// EL EFECTIVO DE UN TERCERO, CON TODO CERRADO (spec 047, punto 1 del 2026-10-31). Una caja que no
// lleva arqueo recibe el gasto sin turno, sin preguntar y sin mover ninguna caja.
func TestUnGastoEnCajaSinArqueoNoPideTurno(t *testing.T) {
	t.Parallel()
	g := nuevoGastoSinTurno(t, fixedNow)
	ext := g.caja(t, "Caja externa", false)
	id, err := g.back.CreateExpense(context.Background(), g.pagar(ext, "", false))
	if err != nil {
		t.Fatalf("el gasto en una caja sin arqueo no se pudo guardar: %v", err)
	}
	if p := g.pagoDe(t, id); p.register == nil || *p.register != ext || p.session != nil {
		t.Fatalf("el pago quedó con caja %v y corte %v; quería la caja externa y ningún corte", p.register, p.session)
	}
}

// UN GASTO FUERA DE CORTE NO ENTRA A NINGÚN CORTE (FR-007, SC-003). El efectivo de otro día, o de
// una caja cerrada, no sale del cajón que hoy se cuenta: ni en la lista de gastos del turno ni en su
// esperado. Y sin confirmar, no se guarda nada.
func TestUnGastoFueraDeCorteNoEntraANingunCorte(t *testing.T) {
	t.Parallel()
	g := nuevoGastoSinTurno(t, fixedNow)
	ctx := context.Background()
	sess := abrirCajaPrincipal(t, g.st, g.admin)
	antes := g.movimientosDe(t, sess)

	ayer := fixedNow.AddDate(0, 0, -1).Format("2006-01-02")
	var gastos int
	_ = g.st.Pool.QueryRow(ctx, `select count(*) from expenses`).Scan(&gastos)
	if _, err := g.back.CreateExpense(ctx, g.pagar(g.primar, ayer, false)); !errors.Is(err, domain.ErrExpenseOutsideCount) {
		t.Fatalf("un pago de ayer con el turno abierto sin confirmar dio %v; quería ErrExpenseOutsideCount", err)
	}
	var despues int
	_ = g.st.Pool.QueryRow(ctx, `select count(*) from expenses`).Scan(&despues)
	if despues != gastos {
		t.Fatal("el gasto sin confirmar se guardó igual")
	}

	id, err := g.back.CreateExpense(ctx, g.pagar(g.primar, ayer, true))
	if err != nil {
		t.Fatal(err)
	}
	if p := g.pagoDe(t, id); p.session != nil || p.register == nil || *p.register != g.primar {
		t.Fatalf("el pago de ayer quedó con corte %v y caja %v; quería ningún corte y la caja principal", p.session, p.register)
	}
	if g.movimientosDe(t, sess) != antes {
		t.Fatal("un gasto fuera de corte movió el efectivo esperado del turno abierto: se descuenta dinero que no salió de ese cajón")
	}
	rows, err := g.st.Q.ListExpensesBySession(ctx, &sess)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rows {
		if r.ExpenseID == id {
			t.Fatal("el gasto fuera de corte aparece en la lista de gastos del turno")
		}
	}
}

// El caso normal no cambia (SC-004): turno abierto y pago del día del turno, sin confirmar.
func TestUnGastoDelDiaDelTurnoEntraAlCorteSinPreguntar(t *testing.T) {
	t.Parallel()
	g := nuevoGastoSinTurno(t, fixedNow)
	sess := abrirCajaPrincipal(t, g.st, g.admin)
	id, err := g.back.CreateExpense(context.Background(), g.pagar(g.primar, fixedNow.Format("2006-01-02"), false))
	if err != nil {
		t.Fatalf("el caso normal ahora pide algo: %v", err)
	}
	if p := g.pagoDe(t, id); p.session == nil || *p.session != sess {
		t.Fatalf("el pago del día del turno quedó con corte %v; quería %d", p.session, sess)
	}
	if g.movimientosDe(t, sess) != 1 {
		t.Fatal("el pago del día del turno no salió del cajón")
	}
}

// Caja con arqueo y sin turno: pregunta, por los dos caminos (gasto nuevo y «+1 pago»).
func TestUnGastoEnCajaCerradaPideConfirmacionPorLosDosCaminos(t *testing.T) {
	t.Parallel()
	g := nuevoGastoSinTurno(t, fixedNow)
	ctx := context.Background()
	if _, err := g.back.CreateExpense(ctx, g.pagar(g.primar, "", false)); !errors.Is(err, domain.ErrExpenseOutsideCount) {
		t.Fatalf("gasto nuevo en caja cerrada: %v; quería ErrExpenseOutsideCount", err)
	}
	pend, err := g.back.CreateExpense(ctx, app.ExpenseInput{CategoryID: g.cat, Amount: dec("80"), Status: domain.ExpensePendiente, UserID: g.admin})
	if err != nil {
		t.Fatal(err)
	}
	reg := g.primar
	in := app.ExpensePaymentInput{MethodID: g.cash, Amount: dec("80"), RegisterID: &reg}
	if err := g.back.AddExpensePayment(ctx, pend, in, g.admin); !errors.Is(err, domain.ErrExpenseOutsideCount) {
		t.Fatalf("«+1 pago» en caja cerrada: %v; quería ErrExpenseOutsideCount", err)
	}
	in.OutsideCount = true
	if err := g.back.AddExpensePayment(ctx, pend, in, g.admin); err != nil {
		t.Fatalf("«+1 pago» confirmado: %v", err)
	}
}

// LAS FECHAS DEL GASTO SON LAS DEL NEGOCIO (punto 2 del 2026-10-31). A las 23:30 del 31-oct en la
// Ciudad de México ya es 1-nov en UTC: así quedó la freidora pagada el día equivocado.
func TestElDiaDelGastoSinFechaEsElDelNegocio(t *testing.T) {
	t.Parallel()
	g := nuevoGastoSinTurno(t, time.Date(2026, 11, 1, 5, 30, 0, 0, time.UTC))
	ext := g.caja(t, "Caja externa", false)
	id, err := g.back.CreateExpense(context.Background(), g.pagar(ext, "", false))
	if err != nil {
		t.Fatal(err)
	}
	var dia, pago time.Time
	if err := g.st.Pool.QueryRow(context.Background(), `select e.expense_date, p.paid_on from expenses e join expense_payments p on p.expense_id = e.id where e.id = $1`, id).Scan(&dia, &pago); err != nil {
		t.Fatal(err)
	}
	if dia.Format("2006-01-02") != "2026-10-31" || pago.Format("2006-01-02") != "2026-10-31" {
		t.Fatalf("día del gasto %s y del pago %s; quería 2026-10-31 (hora del negocio), no el día UTC",
			dia.Format("2006-01-02"), pago.Format("2006-01-02"))
	}
}

// «Lleva arqueo»: la principal siempre, nunca con turno abierto, y renombrar no lo toca. Una caja sin
// arqueo no abre turno.
func TestLlevaArqueoSusGuardas(t *testing.T) {
	t.Parallel()
	g := nuevoGastoSinTurno(t, fixedNow)
	ctx := context.Background()
	no := false
	if _, err := g.back.UpdateCashRegister(ctx, g.primar, "Principal", true, &no); !errors.Is(err, domain.ErrPrimaryRegisterCounts) {
		t.Fatalf("quitar el arqueo a la principal: %v", err)
	}
	sec := g.caja(t, "Secundaria", true)
	if _, err := g.st.Pool.Exec(ctx, `insert into register_sessions (business_date, opening_cash, opened_by, register_id) values ($1, 0, $2, $3)`, fixedNow, g.admin, sec); err != nil {
		t.Fatal(err)
	}
	if _, err := g.back.UpdateCashRegister(ctx, sec, "Secundaria", true, &no); !errors.Is(err, domain.ErrRegisterHasOpenShift) {
		t.Fatalf("quitar el arqueo con turno abierto: %v", err)
	}
	ext := g.caja(t, "Externa", false)
	if v, err := g.back.UpdateCashRegister(ctx, ext, "Externa renombrada", true, nil); err != nil || v.CountsCash {
		t.Fatalf("renombrar le cambió el arqueo: %v, %+v", err, v)
	}
	if _, err := g.back.OpenSession(ctx, ext, app.AperturaCmd{}, g.admin); !errors.Is(err, domain.ErrRegisterNotCounted) {
		t.Fatalf("abrir turno en una caja sin arqueo: %v; quería ErrRegisterNotCounted", err)
	}
}

// Una caja de otra empresa no se puede usar para pagar, bajo el rol de la app en los tres casos.
func TestUnPagoNoUsaLaCajaDeOtraEmpresa(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	company := makeCompany(t, st, "caja-ajena-047")
	var register int64
	if err := st.Pool.QueryRow(context.Background(), `insert into cash_registers (company_id, name) values ($1, 'Ajena') returning id`, company).Scan(&register); err != nil {
		t.Fatal(err)
	}
	inTheThreeCases(t, company, defaultCompanyID, func(t *testing.T, st *store.Store, ctx context.Context) {
		if _, err := st.QC(ctx).GetCashRegister(ctx, register); err == nil {
			t.Fatal("se leyó la caja de otra empresa: un pago podría apuntarle")
		}
	})
}
