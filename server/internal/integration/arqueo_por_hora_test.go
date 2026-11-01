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
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/auth"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/cache"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/config"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/httpapi"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/realtime"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
)

// arqueoPorHora es gastoSinTurno con el reloj de verdad: las horas de apertura y cierre las pone la
// base con now(), y la del pago el servicio; con un reloj fijo hablarían de días distintos.
type arqueoPorHora struct{ gastoSinTurno }

func nuevoArqueoPorHora(t *testing.T) arqueoPorHora {
	t.Helper()
	g := nuevoGastoSinTurno(t, time.Now())
	g.back = app.NewBackofficeService(g.st, time.Now)
	return arqueoPorHora{g}
}

func hoyDelNegocio() string {
	return domain.BusinessDate(time.Now(), domain.LoadBusinessLocation("America/Mexico_City")).Format("2006-01-02")
}

func incluir(v bool) *bool { return &v }

// gasto es un gasto pagado de `amount` en efectivo desde `reg`.
func (g arqueoPorHora) gasto(reg int64, paidOn string, include *bool, confirm bool, amount string) app.ExpenseInput {
	return app.ExpenseInput{
		CategoryID: g.cat, Amount: dec(amount), Status: domain.ExpensePagada, UserID: g.admin, Description: "Gasto 048",
		Payments: []app.ExpensePaymentInput{{MethodID: g.cash, Amount: dec(amount), PaidOn: paidOn, RegisterID: &reg,
			IncludeInCount: include, OutsideCount: confirm}},
	}
}

// turno abre por SQL un turno de `reg` con la hora de apertura dada.
func (g arqueoPorHora) turno(t *testing.T, reg int64, openedAt time.Time) int64 {
	t.Helper()
	var id int64
	if err := g.st.Pool.QueryRow(context.Background(), `insert into register_sessions (business_date, opening_cash, opened_by, register_id, opened_at)
		values ($1::date, 500, $2, $3, $4) returning id`, hoyDelNegocio(), g.admin, reg, openedAt).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func (g arqueoPorHora) cerrar(t *testing.T, sess int64, closedAt time.Time) {
	t.Helper()
	if _, err := g.st.Pool.Exec(context.Background(), `update register_sessions set status = 'cerrada', closed_at = $2, closed_by = $3 where id = $1`, sess, closedAt, g.admin); err != nil {
		t.Fatal(err)
	}
}

type pagoDelArqueo struct {
	id      int64
	session *int64
	include bool
}

func (g arqueoPorHora) pago(t *testing.T, expense int64) pagoDelArqueo {
	t.Helper()
	var p pagoDelArqueo
	if err := g.st.Pool.QueryRow(context.Background(), `select id, register_session_id, include_in_count from expense_payments where expense_id = $1`, expense).Scan(&p.id, &p.session, &p.include); err != nil {
		t.Fatal(err)
	}
	return p
}

// salidas cuenta las salidas de caja de un pago: más de una es un doble descuento.
func (g arqueoPorHora) salidas(t *testing.T, pay int64) int {
	t.Helper()
	var n int
	if err := g.st.Pool.QueryRow(context.Background(), `select count(*) from register_cash_movements where expense_payment_id = $1 and kind = 'salida'`, pay).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// neto es lo que los movimientos de un turno suman al esperado del cajón (entradas − salidas).
func (g arqueoPorHora) neto(t *testing.T, sess int64) string {
	t.Helper()
	var s string
	if err := g.st.Pool.QueryRow(context.Background(), `select coalesce(sum(case kind when 'entrada' then amount else -amount end), 0)::numeric(10,2)::text
		from register_cash_movements where session_id = $1`, sess).Scan(&s); err != nil {
		t.Fatal(err)
	}
	return s
}

func (g arqueoPorHora) eventos(t *testing.T, action string) int {
	t.Helper()
	var n int
	if err := g.st.Pool.QueryRow(context.Background(), `select count(*) from audit_events where action = $1`, action).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// esperarBloqueo espera a que alguna conexión quede esperando un candado: es la señal de que la
// otra mitad de la carrera ya llegó a la puerta.
func esperarBloqueo(t *testing.T, st *store.Store) {
	t.Helper()
	for i := 0; i < 100; i++ {
		var n int
		_ = st.Pool.QueryRow(context.Background(), `select count(*) from pg_stat_activity where datname = current_database() and wait_event_type = 'Lock'`).Scan(&n)
		if n > 0 {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("la otra mitad de la carrera nunca llegó al candado")
}

// ---- US1: la casilla ----

// DESMARCADA, CON LA CAJA ABIERTA, NO TOCA EL ESPERADO Y NO PREGUNTA (US1, escenario 3).
func TestUnPagoDesmarcadoNoEntraAlCorteAbierto(t *testing.T) {
	t.Parallel()
	g := nuevoArqueoPorHora(t)
	sess := g.turno(t, g.primar, time.Now().Add(-time.Hour))
	id, err := g.back.CreateExpense(context.Background(), g.gasto(g.primar, hoyDelNegocio(), incluir(false), false, "150"))
	if err != nil {
		t.Fatalf("un pago desmarcado preguntó o falló: %v", err)
	}
	if p := g.pago(t, id); p.session != nil || p.include {
		t.Fatalf("el pago desmarcado quedó con corte %v e incluido %v", p.session, p.include)
	}
	if n := g.neto(t, sess); n != "0.00" {
		t.Fatalf("el pago desmarcado movió el esperado del corte: neto %s", n)
	}
}

// Tarjeta y caja sin arqueo no se incluyen aunque la casilla venga marcada (US1, escenario 2).
func TestTarjetaYCajaSinArqueoNoSeIncluyen(t *testing.T) {
	t.Parallel()
	g := nuevoArqueoPorHora(t)
	ctx := context.Background()
	ext := g.caja(t, "Externa 048", false)
	id, err := g.back.CreateExpense(ctx, g.gasto(ext, "", incluir(true), false, "50"))
	if err != nil {
		t.Fatal(err)
	}
	if p := g.pago(t, id); p.include || p.session != nil {
		t.Fatalf("el efectivo de una caja sin arqueo quedó incluido %v con corte %v", p.include, p.session)
	}
	in := g.gasto(g.primar, "", incluir(true), false, "50")
	in.Payments[0].MethodID = g.card
	id, err = g.back.CreateExpense(ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	if p := g.pago(t, id); p.include {
		t.Fatal("una tarjeta con la caja cerrada quedó pendiente de corte: no salió de ningún cajón")
	}
}

// UN PAGO QUE ESPERA AL CIERRE DE SU CAJA TERMINA EN 409, NO PENDIENTE (R4). El cierre tiene el
// turno; el pago lo ve abierto, espera, y al soltarse lo encuentra cerrado.
func TestUnPagoQueEsperaAlCierreTerminaEn409(t *testing.T) {
	t.Parallel()
	g := nuevoArqueoPorHora(t)
	ctx := context.Background()
	sess := g.turno(t, g.primar, time.Now().Add(-time.Hour))
	tx, err := g.st.Pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `select id from register_sessions where id = $1 for update`, sess); err != nil {
		t.Fatal(err)
	}
	var gastos int
	_ = g.st.Pool.QueryRow(ctx, `select count(*) from expenses`).Scan(&gastos)
	done := make(chan error, 1)
	go func() {
		_, err := g.back.CreateExpense(ctx, g.gasto(g.primar, hoyDelNegocio(), incluir(true), false, "70"))
		done <- err
	}()
	esperarBloqueo(t, g.st)
	if _, err := tx.Exec(ctx, `update register_sessions set status = 'cerrada', closed_at = now(), closed_by = $2 where id = $1`, sess, g.admin); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err := <-done; !errors.Is(err, domain.ErrExpenseOutsideCount) {
		t.Fatalf("el pago que esperó al cierre dio %v; quería 409 EXPENSE_OUTSIDE_COUNT", err)
	}
	var despues int
	_ = g.st.Pool.QueryRow(ctx, `select count(*) from expenses`).Scan(&despues)
	if despues != gastos {
		t.Fatal("el pago que esperó al cierre se guardó: quedó pendiente sin que nadie lo confirmara")
	}
}

// ---- US2: pendientes y barrido ----

// Caja cerrada: pregunta; confirmado queda pendiente; antes del último cierre no puede entrar.
func TestUnGastoMarcadoConLaCajaCerradaQuedaPendiente(t *testing.T) {
	t.Parallel()
	g := nuevoArqueoPorHora(t)
	ctx := context.Background()
	g.cerrar(t, g.turno(t, g.primar, time.Now().Add(-5*time.Hour)), time.Now().Add(-4*time.Hour))

	if _, err := g.back.CreateExpense(ctx, g.gasto(g.primar, hoyDelNegocio(), incluir(true), false, "200")); !errors.Is(err, domain.ErrExpenseOutsideCount) {
		t.Fatalf("caja cerrada sin confirmar: %v; quería EXPENSE_OUTSIDE_COUNT", err)
	}
	id, err := g.back.CreateExpense(ctx, g.gasto(g.primar, hoyDelNegocio(), incluir(true), true, "200"))
	if err != nil {
		t.Fatal(err)
	}
	if p := g.pago(t, id); p.session != nil || !p.include {
		t.Fatalf("«Registrar» dejó el pago con corte %v e incluido %v; quería pendiente", p.session, p.include)
	}
	ayer := domain.BusinessDate(time.Now(), domain.LoadBusinessLocation("America/Mexico_City")).AddDate(0, 0, -1).Format("2006-01-02")
	if _, err := g.back.CreateExpense(ctx, g.gasto(g.primar, ayer, incluir(true), true, "200")); !errors.Is(err, domain.ErrExpenseBeforeLastClose) {
		t.Fatalf("un pago de ayer, antes del último cierre: %v; quería EXPENSE_BEFORE_LAST_CLOSE", err)
	}
	if _, err := g.back.CreateExpense(ctx, g.gasto(g.primar, ayer, incluir(false), false, "200")); err != nil {
		t.Fatalf("«Registrar sin incluir» de un pago de ayer: %v", err)
	}
}

// ABRIR DESDE LA HORA DEL PENDIENTE LO DESCUENTA DE ESE CORTE (US2, escenarios 2 y 3).
func TestAbrirDesdeUnaHoraAnteriorBarreLosPendientes(t *testing.T) {
	t.Parallel()
	g := nuevoArqueoPorHora(t)
	ctx := context.Background()
	g.cerrar(t, g.turno(t, g.primar, time.Now().Add(-6*time.Hour)), time.Now().Add(-5*time.Hour))
	id, err := g.back.CreateExpense(ctx, g.gasto(g.primar, hoyDelNegocio(), incluir(true), true, "200"))
	if err != nil {
		t.Fatal(err)
	}
	pend, err := g.back.PendingExpenses(ctx, g.primar)
	if err != nil || len(pend) != 1 || pend[0].ExpenseID != id {
		t.Fatalf("la apertura no lista el pendiente: %+v, %v", pend, err)
	}
	desde := time.Now().Add(-3 * time.Hour)
	cmd := aperturaAMano(dec("500"))
	cmd.OpenedAt = &desde
	vista, err := g.back.OpenSession(ctx, g.primar, cmd, g.admin)
	if err != nil {
		t.Fatalf("abrir desde una hora anterior: %v", err)
	}
	p := g.pago(t, id)
	if p.session == nil || *p.session != vista.ID {
		t.Fatalf("el pendiente quedó con corte %v; quería el que se abrió (%d)", p.session, vista.ID)
	}
	if n := g.neto(t, vista.ID); n != "-200.00" {
		t.Fatalf("el esperado del corte no bajó $200: neto %s", n)
	}
	if pend, _ := g.back.PendingExpenses(ctx, g.primar); len(pend) != 0 {
		t.Fatalf("el pendiente sigue listado tras entrar al corte: %+v", pend)
	}
	if g.eventos(t, "cash.opening_backdated") != 1 {
		t.Fatal("abrir desde una hora anterior no dejó su evento en la bitácora (audit_events)")
	}
}

// Abrir sin mover la hora no barre: el pendiente se queda pendiente (US2, escenario 5).
func TestAbrirAhoraNoBarreLosPendientesAnteriores(t *testing.T) {
	t.Parallel()
	g := nuevoArqueoPorHora(t)
	ctx := context.Background()
	g.cerrar(t, g.turno(t, g.primar, time.Now().Add(-6*time.Hour)), time.Now().Add(-5*time.Hour))
	id, err := g.back.CreateExpense(ctx, g.gasto(g.primar, hoyDelNegocio(), incluir(true), true, "200"))
	if err != nil {
		t.Fatal(err)
	}
	vista, err := g.back.OpenSession(ctx, g.primar, aperturaAMano(dec("500")), g.admin)
	if err != nil {
		t.Fatal(err)
	}
	if p := g.pago(t, id); p.session != nil || !p.include {
		t.Fatalf("abrir sin mover la hora cambió el pendiente: corte %v, incluido %v", p.session, p.include)
	}
	if n := g.neto(t, vista.ID); n != "0.00" {
		t.Fatalf("abrir ahora descontó un pendiente anterior a la apertura: neto %s", n)
	}
}

// EL CIERRE BARRE LO QUE QUEDÓ PENDIENTE CON HORA DENTRO DEL CORTE (US2, escenario 3).
func TestElCierreBarreLosPendientesDeSuVentana(t *testing.T) {
	t.Parallel()
	g := nuevoArqueoPorHora(t)
	ctx := context.Background()
	sess := g.turno(t, g.primar, time.Now().Add(-2*time.Hour))
	// Un pendiente dentro de la ventana: lo que deja una tableta que decidió con la caja cerrada
	// justo antes de que se abriera desde una hora anterior.
	var exp, pay int64
	if err := g.st.Pool.QueryRow(ctx, `insert into expenses (category_id, amount, expense_date, created_by, description) values ($1, 120, current_date, $2, 'Hielo') returning id`, g.cat, g.admin).Scan(&exp); err != nil {
		t.Fatal(err)
	}
	if err := g.st.Pool.QueryRow(ctx, `insert into expense_payments (expense_id, payment_method_id, amount, paid_on, register_id, paid_by, include_in_count, paid_at)
		values ($1, $2, 120, current_date, $3, $4, true, now() - interval '1 hour') returning id`, exp, g.cash, g.primar, g.admin).Scan(&pay); err != nil {
		t.Fatal(err)
	}
	if _, err := g.back.CloseSession(ctx, g.primar, g.admin, app.CierreCmd{Total: new(dec("380")), Motivo: "conteo de prueba"}); err != nil {
		t.Fatalf("cerrar: %v", err)
	}
	if g.salidas(t, pay) != 1 {
		t.Fatal("el cierre no descontó el pendiente que caía en su ventana")
	}
	var at *int64
	_ = g.st.Pool.QueryRow(ctx, `select register_session_id from expense_payments where id = $1`, pay).Scan(&at)
	if at == nil || *at != sess {
		t.Fatalf("el pendiente quedó con corte %v; quería el que se cerró (%d)", at, sess)
	}
	var esperado string
	if err := g.st.Pool.QueryRow(ctx, `select expected::text from session_cash_counts where session_id = $1 and moment = 'cierre'`, sess).Scan(&esperado); err != nil {
		t.Fatal(err)
	}
	if esperado != "380.00" {
		t.Fatalf("el esperado firmado del cierre es %s; quería 380.00 (500 de fondo − 120 del pendiente)", esperado)
	}
}

// UN PENDIENTE SE DESCUENTA UNA SOLA VEZ (constitución III): dos aperturas a la vez y el cierre
// que vuelve a barrer la misma ventana no lo pegan dos veces.
func TestUnPendienteSeDescuentaUnaSolaVez(t *testing.T) {
	t.Parallel()
	g := nuevoArqueoPorHora(t)
	ctx := context.Background()
	g.cerrar(t, g.turno(t, g.primar, time.Now().Add(-6*time.Hour)), time.Now().Add(-5*time.Hour))
	id, err := g.back.CreateExpense(ctx, g.gasto(g.primar, hoyDelNegocio(), incluir(true), true, "200"))
	if err != nil {
		t.Fatal(err)
	}
	pay := g.pago(t, id).id
	desde := time.Now().Add(-3 * time.Hour)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			cmd := aperturaAMano(dec("500"))
			cmd.OpenedAt = &desde
			_, _ = g.back.OpenSession(ctx, g.primar, cmd, g.admin)
		}()
	}
	wg.Wait()
	if n := g.salidas(t, pay); n != 1 {
		t.Fatalf("dos aperturas a la vez dejaron %d salidas del mismo pendiente: el gasto se descontó dos veces", n)
	}
	if _, err := g.back.CloseSession(ctx, g.primar, g.admin, app.CierreCmd{Total: new(dec("300")), Motivo: "conteo de prueba"}); err != nil {
		t.Fatalf("cerrar: %v", err)
	}
	if n := g.salidas(t, pay); n != 1 {
		t.Fatalf("el barrido del cierre volvió a descontar el pendiente: %d salidas", n)
	}
}

// Archivar una caja con pendientes se permite y los deja fuera, sin error (B5).
func TestArchivarUnaCajaConPendientes(t *testing.T) {
	t.Parallel()
	g := nuevoArqueoPorHora(t)
	ctx := context.Background()
	sec := g.caja(t, "Secundaria 048", true)
	id, err := g.back.CreateExpense(ctx, g.gasto(sec, hoyDelNegocio(), incluir(true), true, "90"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := g.back.UpdateCashRegister(ctx, sec, "Secundaria 048", false, nil); err != nil {
		t.Fatalf("archivar una caja con pendientes: %v", err)
	}
	if p := g.pago(t, id); p.session != nil {
		t.Fatal("archivar la caja le dio corte al pendiente")
	}
}

// La lista de pendientes de una caja no se ve desde otra empresa, en los tres casos donde RLS ya
// falló (R8).
func TestLosPendientesDeUnaCajaNoSeVenDesdeOtraEmpresa(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()
	coB := makeCompany(t, st, "pendientes-048-b")
	userB := makeUserIn(t, st, coB, "admin_048_b", "admin")
	var regB, catB, expB, cashB int64
	if err := st.Pool.QueryRow(ctx, `insert into cash_registers (company_id, name) values ($1, 'Caja B') returning id`, coB).Scan(&regB); err != nil {
		t.Fatal(err)
	}
	if err := st.Pool.QueryRow(ctx, `insert into expense_categories (company_id, name, financial_group) values ($1, 'B', 'operacional') returning id`, coB).Scan(&catB); err != nil {
		t.Fatal(err)
	}
	if err := st.Pool.QueryRow(ctx, `insert into expenses (company_id, category_id, amount, expense_date, created_by) values ($1, $2, 10, current_date, $3) returning id`, coB, catB, userB).Scan(&expB); err != nil {
		t.Fatal(err)
	}
	if err := st.Pool.QueryRow(ctx, `select id from payment_methods where company_id = $1 and affects_cash_drawer limit 1`, coB).Scan(&cashB); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Pool.Exec(ctx, `insert into expense_payments (company_id, expense_id, payment_method_id, amount, paid_on, register_id, paid_by, include_in_count)
		values ($1, $2, $3, 10, current_date, $4, $5, true)`, coB, expB, cashB, regB, userB); err != nil {
		t.Fatal(err)
	}
	own := appRoleStore(t)
	octx, release, err := own.AcquireTenant(ctx, coB)
	if err != nil {
		t.Fatal(err)
	}
	if items, err := app.NewBackofficeService(own, time.Now).PendingExpenses(octx, regB); err != nil || len(items) != 1 {
		t.Fatalf("la dueña no ve su pendiente: %+v, %v", items, err)
	}
	release()
	inTheThreeCases(t, coB, defaultCompanyID, func(t *testing.T, st *store.Store, ctx context.Context) {
		items, err := app.NewBackofficeService(st, time.Now).PendingExpenses(ctx, regB)
		if err == nil && len(items) > 0 {
			t.Fatalf("se leyeron los pendientes de la caja de otra empresa: %+v", items)
		}
	})
}

// ---- US3 y US4 por HTTP: permisos y validación ----

func routerDeArqueo(t *testing.T, st *store.Store, perms httpapi.PermissionResolver) (http.Handler, *auth.Manager) {
	t.Helper()
	jm := auth.NewManager("secreto-de-pruebas-suficientemente-largo-para-el-manager", nil)
	h := httpapi.NewHandlers(httpapi.Deps{JWT: jm, Backoffice: app.NewBackofficeService(st, time.Now), MenuCache: cache.NewMenuCache(""),
		Broker: realtime.NewBroker(), Permissions: perms})
	return httpapi.Router(config.Config{}, jm, h, st), jm
}

func sinAjustarArqueo(r domain.Role) []domain.Permission {
	var out []domain.Permission
	for _, p := range domain.PermissionsFor(r) {
		if p != domain.PermCashAdjustCount {
			out = append(out, p)
		}
	}
	return out
}

// MOVER LA HORA DE APERTURA ES POR PERMISO Y SE VALIDA EN EL SERVIDOR (US3).
func TestAbrirDesdeUnaHoraAnteriorExigePermisoYHoraValida(t *testing.T) {
	t.Parallel()
	g := nuevoArqueoPorHora(t)
	g.cerrar(t, g.turno(t, g.primar, time.Now().Add(-6*time.Hour)), time.Now().Add(-5*time.Hour))
	cuerpo := func(at time.Time) []byte {
		return []byte(`{"registerId":` + itoa(int(g.primar)) + `,"openingCash":500,"manualReason":"prueba","openedAt":"` + at.UTC().Format(time.RFC3339) + `"}`)
	}
	for _, c := range []struct {
		name  string
		perms httpapi.PermissionResolver
		at    time.Time
		want  int
	}{
		{"sin el permiso", sinAjustarArqueo, time.Now().Add(-time.Hour), http.StatusForbidden},
		{"hora futura", nil, time.Now().Add(time.Hour), http.StatusBadRequest},
		{"antes del último cierre", nil, time.Now().Add(-7 * time.Hour), http.StatusBadRequest},
	} {
		t.Run(c.name, func(t *testing.T) {
			r, jm := routerDeArqueo(t, g.st, c.perms)
			tok, err := jm.Issue(domain.User{ID: g.admin, CompanyID: defaultCompanyID, Name: "a", Role: domain.RoleAdmin})
			if err != nil {
				t.Fatal(err)
			}
			if w := do(t, r, http.MethodPost, "/api/v1/cash-sessions", tok, cuerpo(c.at), "application/json"); w.Code != c.want {
				t.Fatalf("abrir = %d (%s); quería %d", w.Code, w.Body.String(), c.want)
			}
		})
	}
	var abiertos int
	_ = g.st.Pool.QueryRow(context.Background(), `select count(*) from register_sessions where status = 'abierta'`).Scan(&abiertos)
	if abiertos != 0 {
		t.Fatal("una apertura rechazada dejó la caja abierta")
	}
	r, jm := routerDeArqueo(t, g.st, nil)
	tok, _ := jm.Issue(domain.User{ID: g.admin, CompanyID: defaultCompanyID, Name: "a", Role: domain.RoleAdmin})
	if w := do(t, r, http.MethodGet, "/api/v1/cash-sessions/pending-expenses", tok, nil, ""); w.Code != http.StatusBadRequest {
		t.Fatalf("pendientes sin caja = %d; quería 400", w.Code)
	}
	if w := do(t, r, http.MethodPost, "/api/v1/cash-sessions", tok, cuerpo(time.Now().Add(-time.Hour)), "application/json"); w.Code != http.StatusCreated {
		t.Fatalf("abrir desde una hora válida = %d: %s", w.Code, w.Body.String())
	}
	if g.eventos(t, "cash.opening_backdated") != 1 {
		t.Fatal("la apertura con hora anterior no quedó en la bitácora")
	}
}

// LA CASILLA DE UN PAGO YA REGISTRADO (US4).
func TestCambiarLaCasillaDeUnPagoRegistrado(t *testing.T) {
	t.Parallel()
	g := nuevoArqueoPorHora(t)
	ctx := context.Background()
	sess := g.turno(t, g.primar, time.Now().Add(-time.Hour))
	id, err := g.back.CreateExpense(ctx, g.gasto(g.primar, hoyDelNegocio(), incluir(true), false, "150"))
	if err != nil {
		t.Fatal(err)
	}
	pay := g.pago(t, id).id

	if err := g.back.SetPaymentInCount(ctx, id, pay, false, g.admin); err != nil {
		t.Fatalf("desmarcar en corte abierto: %v", err)
	}
	if p := g.pago(t, id); p.session != nil || p.include || g.salidas(t, pay) != 0 || g.neto(t, sess) != "0.00" {
		t.Fatalf("desmarcar no sacó el pago del corte: %+v, %d salidas, neto %s", p, g.salidas(t, pay), g.neto(t, sess))
	}
	if err := g.back.SetPaymentInCount(ctx, id, pay, true, g.admin); err != nil {
		t.Fatalf("volver a marcar en el mismo corte: %v", err)
	}
	if g.salidas(t, pay) != 1 || g.neto(t, sess) != "-150.00" {
		t.Fatalf("volver a marcar dejó %d salidas y neto %s; quería una y −150", g.salidas(t, pay), g.neto(t, sess))
	}
	if g.eventos(t, "expense.count_changed") != 2 {
		t.Fatal("cada cambio de la casilla deja un evento en la bitácora (audit_events)")
	}

	g.cerrar(t, sess, time.Now())
	if err := g.back.SetPaymentInCount(ctx, id, pay, false, g.admin); !errors.Is(err, domain.ErrPaymentInClosedCount) {
		t.Fatalf("desmarcar en corte cerrado: %v; quería PAYMENT_IN_CLOSED_COUNT", err)
	}
	ayer := domain.BusinessDate(time.Now(), domain.LoadBusinessLocation("America/Mexico_City")).AddDate(0, 0, -1).Format("2006-01-02")
	viejo, err := g.back.CreateExpense(ctx, g.gasto(g.primar, ayer, incluir(false), false, "40"))
	if err != nil {
		t.Fatal(err)
	}
	if err := g.back.SetPaymentInCount(ctx, viejo, g.pago(t, viejo).id, true, g.admin); !errors.Is(err, domain.ErrExpenseBeforeLastClose) {
		t.Fatalf("marcar un pago de antes del último cierre: %v; quería EXPENSE_BEFORE_LAST_CLOSE", err)
	}
}

// Marcar un pago que quedó fuera, con la caja abierta y hora dentro del corte, lo descuenta.
func TestMarcarUnPagoFueraConHoraEnElCorte(t *testing.T) {
	t.Parallel()
	g := nuevoArqueoPorHora(t)
	ctx := context.Background()
	sess := g.turno(t, g.primar, time.Now().Add(-time.Hour))
	id, err := g.back.CreateExpense(ctx, g.gasto(g.primar, hoyDelNegocio(), incluir(false), false, "60"))
	if err != nil {
		t.Fatal(err)
	}
	if err := g.back.SetPaymentInCount(ctx, id, g.pago(t, id).id, true, g.admin); err != nil {
		t.Fatal(err)
	}
	if p := g.pago(t, id); p.session == nil || *p.session != sess || g.neto(t, sess) != "-60.00" {
		t.Fatalf("marcar no metió el pago al corte abierto: %+v, neto %s", p, g.neto(t, sess))
	}
}

// LA CARRERA DE LA CASILLA CONTRA EL CIERRE (R3): el cierre gana y la casilla ya no cambia.
func TestLaCasillaQueEsperaAlCierreNoCambiaUnCorteCerrado(t *testing.T) {
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
	go func() { done <- g.back.SetPaymentInCount(ctx, id, pay, false, g.admin) }()
	esperarBloqueo(t, g.st)
	if _, err := tx.Exec(ctx, `update register_sessions set status = 'cerrada', closed_at = now(), closed_by = $2 where id = $1`, sess, g.admin); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err := <-done; !errors.Is(err, domain.ErrPaymentInClosedCount) {
		t.Fatalf("la casilla que esperó al cierre dio %v; quería PAYMENT_IN_CLOSED_COUNT", err)
	}
	if g.salidas(t, pay) != 1 {
		t.Fatal("la casilla le quitó la salida a un corte ya cerrado")
	}
}

// Cambiar la casilla es por permiso (US4, escenario 4).
func TestCambiarLaCasillaExigeElPermiso(t *testing.T) {
	t.Parallel()
	g := nuevoArqueoPorHora(t)
	ctx := context.Background()
	g.turno(t, g.primar, time.Now().Add(-time.Hour))
	id, err := g.back.CreateExpense(ctx, g.gasto(g.primar, hoyDelNegocio(), incluir(true), false, "150"))
	if err != nil {
		t.Fatal(err)
	}
	pay := g.pago(t, id).id
	r, jm := routerDeArqueo(t, g.st, sinAjustarArqueo)
	tok, err := jm.Issue(domain.User{ID: g.admin, CompanyID: defaultCompanyID, Name: "a", Role: domain.RoleGerente})
	if err != nil {
		t.Fatal(err)
	}
	path := "/api/v1/expenses/" + itoa(int(id)) + "/payments/" + itoa(int(pay))
	if w := do(t, r, http.MethodPatch, path, tok, []byte(`{"includeInCount":false}`), "application/json"); w.Code != http.StatusForbidden {
		t.Fatalf("PATCH sin permiso = %d; quería 403", w.Code)
	}
	if g.salidas(t, pay) != 1 {
		t.Fatal("la casilla cambió sin el permiso")
	}
	r, jm = routerDeArqueo(t, g.st, nil)
	tok, _ = jm.Issue(domain.User{ID: g.admin, CompanyID: defaultCompanyID, Name: "a", Role: domain.RoleGerente})
	w := do(t, r, http.MethodPatch, path, tok, []byte(`{"includeInCount":false}`), "application/json")
	if w.Code != http.StatusNoContent {
		t.Fatalf("PATCH con permiso = %d: %s", w.Code, w.Body.String())
	}
	w = do(t, r, http.MethodGet, "/api/v1/expenses/"+itoa(int(id)), tok, nil, "")
	if !strings.Contains(w.Body.String(), `"includeInCount":false`) || !strings.Contains(w.Body.String(), `"sessionClosed":false`) || !strings.Contains(w.Body.String(), `"paidAt"`) {
		t.Fatalf("el detalle del gasto no trae la casilla del pago: %s", w.Body.String())
	}
}
