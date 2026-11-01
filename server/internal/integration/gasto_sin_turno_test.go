//go:build integration

package integration

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/auth"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/cache"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/config"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/httpapi"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/realtime"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store/db"
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

// QUITAR EL ARQUEO ES POR PERMISO (FR-010, principio V). El control vive en el handler y no en el
// router: sin esta prueba, un refactor lo pierde en silencio y un rol sin el permiso deja una caja
// fuera de todo corte.
func TestLlevaArqueoExigeElPermisoDeConfigurarCaja(t *testing.T) {
	t.Parallel()
	sinConfigurar := func(r domain.Role) []domain.Permission {
		var out []domain.Permission
		for _, p := range domain.PermissionsFor(r) {
			if p != domain.PermCashConfigure {
				out = append(out, p)
			}
		}
		return out
	}
	for _, c := range []struct {
		name   string
		perms  httpapi.PermissionResolver
		body   string
		denied bool
	}{
		{"un gerente sin el permiso no apaga el arqueo", sinConfigurar, `{"name":"Sec","isActive":true,"countsCash":false}`, true},
		{"ni lo prende", sinConfigurar, `{"name":"Sec","isActive":true,"countsCash":true}`, true},
		{"sin tocar el arqueo, renombrar sigue igual", sinConfigurar, `{"name":"Sec 2","isActive":true}`, false},
		{"con el permiso sí", nil, `{"name":"Sec","isActive":true,"countsCash":false}`, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			st := newTestStore(t)
			jm := auth.NewManager("secreto-de-pruebas-suficientemente-largo-para-el-manager", nil)
			h := httpapi.NewHandlers(httpapi.Deps{JWT: jm, Backoffice: app.NewBackofficeService(st, clock), MenuCache: cache.NewMenuCache(""),
				Broker: realtime.NewBroker(), Permissions: c.perms})
			r := httpapi.Router(config.Config{}, jm, h, st)
			id := makeUser(t, st, "gerente_arqueo", "gerente")
			tok, err := jm.Issue(domain.User{ID: id, CompanyID: defaultCompanyID, Name: "g", Role: domain.RoleGerente})
			if err != nil {
				t.Fatal(err)
			}
			var sec int64
			if err := st.Pool.QueryRow(context.Background(), `insert into cash_registers (name) values ('Sec') returning id`).Scan(&sec); err != nil {
				t.Fatal(err)
			}
			w := do(t, r, http.MethodPatch, "/api/v1/cash-registers/"+itoa(int(sec)), tok, []byte(c.body), "application/json")
			if c.denied && w.Code != http.StatusForbidden {
				t.Fatalf("PATCH = %d; quería 403", w.Code)
			}
			if !c.denied && w.Code != http.StatusOK {
				t.Fatalf("PATCH = %d: %s", w.Code, w.Body.String())
			}
			var counts bool
			_ = st.Pool.QueryRow(context.Background(), `select counts_cash from cash_registers where id = $1`, sec).Scan(&counts)
			if c.denied && !counts {
				t.Fatal("el arqueo se apagó sin el permiso")
			}
		})
	}
}

// Un pago no puede apuntar a la caja de otra empresa aunque el servicio se equivoque: la FK
// compuesta es la barrera real, porque la FK salta RLS (R8).
func TestLaFKNoDejaPagarDeLaCajaDeOtraEmpresa(t *testing.T) {
	t.Parallel()
	g := nuevoGastoSinTurno(t, fixedNow)
	st := g.st
	ctx := context.Background()
	company := makeCompany(t, st, "fk-caja-047")
	var ajena int64
	if err := st.Pool.QueryRow(ctx, `insert into cash_registers (company_id, name) values ($1, 'Ajena') returning id`, company).Scan(&ajena); err != nil {
		t.Fatal(err)
	}
	appSt := appRoleStore(t)
	actx, release, err := appSt.AcquireTenant(ctx, defaultCompanyID)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	var exp int64
	if err := g.st.Pool.QueryRow(ctx, `insert into expenses (category_id, amount, expense_date, created_by) values ($1, 10, current_date, $2) returning id`, g.cat, g.admin).Scan(&exp); err != nil {
		t.Fatal(err)
	}
	if _, err := appSt.QC(actx).CreateExpensePayment(actx, db.CreateExpensePaymentParams{
		ExpenseID: exp, PaymentMethodID: g.cash, Amount: dec("10"), PaidOn: pgtype.Date{Time: fixedNow, Valid: true},
		RegisterID: &ajena, PaidBy: g.admin,
	}); err == nil {
		t.Fatal("un pago de la empresa por omisión quedó apuntando a la caja de otra empresa")
	}
}

// CON LA PRINCIPAL ABIERTA, UN GASTO PASADO DE OTRA CAJA CONSERVA SU DÍA. El día del turno manda
// solo si el pago entra a ese turno; antes se imponía el de la principal y se rechazaba el día elegido.
func TestUnGastoPasadoDeOtraCajaNoTomaElDiaDeLaPrincipal(t *testing.T) {
	t.Parallel()
	g := nuevoGastoSinTurno(t, fixedNow)
	abrirCajaPrincipal(t, g.st, g.admin)
	ext := g.caja(t, "Caja externa", false)
	in := g.pagar(ext, "2026-07-01", false)
	in.ExpenseDay = "2026-07-01"
	id, err := g.back.CreateExpense(context.Background(), in)
	if err != nil {
		t.Fatalf("el gasto pasado de otra caja se rechazó con la principal abierta: %v", err)
	}
	var dia time.Time
	_ = g.st.Pool.QueryRow(context.Background(), `select expense_date from expenses where id = $1`, id).Scan(&dia)
	if dia.Format("2006-01-02") != "2026-07-01" {
		t.Fatalf("día del gasto %s; quería el elegido 2026-07-01", dia.Format("2006-01-02"))
	}
}

// PAGAR HOY UN GASTO DE AYER, CON LA CAJA ABIERTA, ENTRA AL CORTE. Sin fecha de pago, el día es el
// del negocio, no el del gasto: si no, el efectivo sale del cajón de hoy y ningún corte lo ve.
func TestPagarHoyUnGastoDeAyerEntraAlCorte(t *testing.T) {
	t.Parallel()
	g := nuevoGastoSinTurno(t, fixedNow)
	ctx := context.Background()
	sess := abrirCajaPrincipal(t, g.st, g.admin)
	if _, err := g.st.Pool.Exec(ctx, `update register_sessions set status = 'cerrada', closed_at = now(), closed_by = $2 where id = $1`, sess, g.admin); err != nil {
		t.Fatal(err)
	}
	pend, err := g.back.CreateExpense(ctx, app.ExpenseInput{ExpenseDay: fixedNow.AddDate(0, 0, -1).Format("2006-01-02"),
		CategoryID: g.cat, Amount: dec("80"), Status: domain.ExpensePendiente, UserID: g.admin})
	if err != nil {
		t.Fatal(err)
	}
	hoy := abrirCajaPrincipal(t, g.st, g.admin)
	reg := g.primar
	if err := g.back.AddExpensePayment(ctx, pend, app.ExpensePaymentInput{MethodID: g.cash, Amount: dec("80"), RegisterID: &reg}, g.admin); err != nil {
		t.Fatalf("pagar hoy un gasto de ayer con la caja abierta: %v", err)
	}
	if g.movimientosDe(t, hoy) != 1 {
		t.Fatal("el pago de hoy no salió del cajón de hoy")
	}
}

// LA CARRERA: apagar el arqueo y abrir turno a la vez nunca dejan un turno abierto en una caja que
// no se cuenta (R2). Cada lado toma la caja con FOR UPDATE.
func TestApagarArqueoYAbrirTurnoALaVezNoDejaTurnoSinArqueo(t *testing.T) {
	t.Parallel()
	g := nuevoGastoSinTurno(t, fixedNow)
	ctx := context.Background()
	for i := 0; i < 10; i++ {
		sec := g.caja(t, "Carrera "+itoa(i), true)
		no := false
		done := make(chan struct{}, 2)
		go func() { _, _ = g.back.UpdateCashRegister(ctx, sec, "Carrera", true, &no); done <- struct{}{} }()
		go func() { _, _ = g.back.OpenSession(ctx, sec, app.AperturaCmd{}, g.admin); done <- struct{}{} }()
		<-done
		<-done
		var counts bool
		var abiertos int
		_ = g.st.Pool.QueryRow(ctx, `select counts_cash, (select count(*) from register_sessions where register_id = $1 and status = 'abierta') from cash_registers where id = $1`, sec).Scan(&counts, &abiertos)
		if !counts && abiertos > 0 {
			t.Fatalf("vuelta %d: la caja quedó sin arqueo con un turno abierto", i)
		}
	}
}
