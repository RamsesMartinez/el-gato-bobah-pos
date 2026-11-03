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

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/auth"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/cache"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/config"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/httpapi"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/realtime"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
)

// cashWorld es la caja principal de la empresa de pruebas con la asistencia encendida, y los dos
// servicios bajo el rol de la aplicación con el MISMO reloj: la regla compara la entrada que puso
// checar contra la hora con la que decide la caja.
type cashWorld struct {
	st   *store.Store // owner, para sembrar y revisar
	app  *store.Store // gatobobah_app
	back *app.BackofficeService
	att  *app.AttendanceService
	ctx  context.Context
	now  time.Time
	reg  int64
}

func newCashWorld(t *testing.T) *cashWorld {
	t.Helper()
	st := newTestStore(t)
	enableAttendance(t, st, defaultCompanyID, true)
	w := &cashWorld{st: st, app: appRoleStore(t), now: time.Date(2026, 10, 30, 15, 0, 0, 0, time.UTC),
		reg: registerID(t, st, "Caja principal")}
	clockFn := func() time.Time { return w.now }
	w.back = app.NewBackofficeService(w.app, clockFn)
	w.att = app.NewAttendanceService(w.app, clockFn)
	w.ctx = w.tenant(t)
	return w
}

// tenant es otra sesión de la empresa de pruebas: su propia conexión, como otra tableta.
func (w *cashWorld) tenant(t *testing.T) context.Context {
	t.Helper()
	ctx, release, err := w.app.AcquireTenant(context.Background(), defaultCompanyID)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(release)
	return ctx
}

// person crea a alguien con PIN y «Lleva asistencia» (el default de la columna).
func (w *cashWorld) person(t *testing.T, username, role string) int64 {
	t.Helper()
	id := makeUserIn(t, w.st, defaultCompanyID, username, role)
	hash, err := auth.HashSecret(testPIN)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.st.Pool.Exec(context.Background(), `update users set pin_hash = $2, tracks_attendance = true where id = $1`, id, hash); err != nil {
		t.Fatal(err)
	}
	return id
}

func (w *cashWorld) clock(t *testing.T, id int64, kind domain.ClockKind) {
	t.Helper()
	if kind == domain.ClockOut {
		// Una salida al mismo instante que la entrada la rechaza el esquema.
		w.now = w.now.Add(time.Minute)
	}
	if _, err := w.att.Clock(w.ctx, id, testPIN, kind); err != nil {
		t.Fatalf("checar %s de %d: %v", kind, id, err)
	}
}

func (w *cashWorld) state(t *testing.T, ctx context.Context, user int64, perms []domain.Permission) *app.CashAttendanceView {
	t.Helper()
	v, err := w.back.CashAttendanceState(ctx, w.reg, user, perms)
	if err != nil {
		t.Fatalf("estado de asistencia: %v", err)
	}
	return v
}

func (w *cashWorld) openAs(ctx context.Context, user int64) error {
	_, err := w.back.OpenSession(ctx, w.reg, aperturaAMano(dec("500")), user)
	return err
}

func (w *cashWorld) closeAs(ctx context.Context, user int64, perms []domain.Permission, reason *string) error {
	total := dec("500")
	_, err := w.back.CloseSession(ctx, w.reg, user, app.CierreCmd{Total: &total, Motivo: "Conté a mano",
		Perms: perms, AuthorizedReason: reason})
	return err
}

// openAndCheck abre y comprueba que el GET dijo lo mismo antes (B18): ready ⇔ abrir no rechaza por
// asistencia.
func (w *cashWorld) openAndCheck(t *testing.T, user int64) error {
	t.Helper()
	v := w.state(t, w.ctx, user, domain.PermissionsFor(domain.RoleCajero))
	if v.Stage != "open" || v.Person.ID != user {
		t.Fatalf("con la caja cerrada el estado es %+v; quiere stage=open y la persona de la sesión", v)
	}
	err := w.openAs(w.ctx, user)
	if v.Ready != !errors.Is(err, domain.ErrCashClockInRequired) {
		t.Fatalf("el GET dijo ready=%v y abrir respondió %v", v.Ready, err)
	}
	return err
}

// closeAndCheck cierra sin autorizar y comprueba que el GET dijo lo mismo antes (B18).
func (w *cashWorld) closeAndCheck(t *testing.T, user int64) error {
	t.Helper()
	perms := domain.PermissionsFor(domain.RoleCajero)
	v := w.state(t, w.ctx, user, perms)
	if v.Stage != "close" {
		t.Fatalf("con la caja abierta el estado es %+v; quiere stage=close", v)
	}
	err := w.closeAs(w.ctx, user, perms, nil)
	if v.Ready != !errors.Is(err, domain.ErrCashOpenerClockedIn) {
		t.Fatalf("el GET dijo ready=%v y cerrar respondió %v", v.Ready, err)
	}
	return err
}

func (w *cashWorld) openSessions(t *testing.T) int {
	t.Helper()
	var n int
	if err := w.st.Pool.QueryRow(context.Background(),
		`select count(*) from register_sessions where register_id = $1 and status = 'abierta'`, w.reg).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func (w *cashWorld) entryStatus(t *testing.T, user int64) []string {
	t.Helper()
	rows, err := w.st.Pool.Query(context.Background(), `select status from attendance_entries where user_id = $1 order by id`, user)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var s string
		_ = rows.Scan(&s)
		out = append(out, s)
	}
	return out
}

func (w *cashWorld) authorizedEvents(t *testing.T) int {
	t.Helper()
	var n int
	if err := w.st.Pool.QueryRow(context.Background(),
		`select count(*) from audit_events where action = 'cash.closed_without_opener_clock_out'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// waitForLockWaiter espera a que alguna sesión de esta base esté detenida por un candado: así la
// prueba sabe que la otra tableta ya llegó al `for share` antes de soltar el `for update`.
func (w *cashWorld) waitForLockWaiter(t *testing.T) {
	t.Helper()
	for range 200 {
		var n int
		if err := w.st.Pool.QueryRow(context.Background(), `select count(*) from pg_stat_activity
			where datname = current_database() and wait_event_type = 'Lock'`).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n > 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("nadie quedó esperando el candado de la persona")
}

func reasonOf(s string) *string { return &s }

// --- El estado del GET (T007) ---

func TestCashAttendanceStateSaysWhoTheActionDependsOn(t *testing.T) {
	t.Parallel()
	w := newCashWorld(t)
	ana := w.person(t, "ana-estado", "cajero")
	gina := w.person(t, "gina-estado", "gerente")

	v := w.state(t, w.ctx, ana, domain.PermissionsFor(domain.RoleCajero))
	if v.Stage != "open" || v.Person.ID != ana || v.Person.Name != "Test ana-estado" || !v.Person.Active ||
		!v.Applies || v.Ready || !v.IsSelf || v.OpenSince != nil {
		t.Fatalf("caja cerrada, sin entrada: %+v", v)
	}
	w.clock(t, ana, domain.ClockIn)
	if err := w.openAndCheck(t, ana); err != nil {
		t.Fatal(err)
	}
	v = w.state(t, w.ctx, gina, domain.PermissionsFor(domain.RoleGerente))
	if v.Stage != "close" || v.Person.ID != ana || v.IsSelf || v.Ready || !v.CanAuthorize || v.OpenSince == nil {
		t.Fatalf("caja abierta por Ana, vista por Gina: %+v", v)
	}
	// El JSON promete `person` siempre y `openSince` solo si hay entrada.
	raw, _ := json.Marshal(v)
	var m map[string]any
	_ = json.Unmarshal(raw, &m)
	if _, ok := m["person"].(map[string]any); !ok {
		t.Fatalf("person no es objeto: %s", raw)
	}
}

// B17 y B23: desde otra empresa, con una conexión reciclada o sin empresa, la caja no existe — y
// nunca responde «no aplica», que abriría la puerta que la regla cierra.
func TestCashAttendanceStateIsIsolated(t *testing.T) {
	t.Parallel()
	w := newCashWorld(t)
	ana := w.person(t, "ana-aislada", "cajero")
	other := makeCompany(t, w.st, "caja-asistencia-otra")
	inTheThreeCases(t, defaultCompanyID, other, func(t *testing.T, st *store.Store, ctx context.Context) {
		back := app.NewBackofficeService(st, func() time.Time { return w.now })
		if v, err := back.CashAttendanceState(ctx, w.reg, ana, domain.AllPermissions()); !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("alcanzó la caja de otra empresa: %+v %v", v, err)
		}
	})
	// La caja sí es de la otra empresa, pero la persona no: tampoco «no aplica».
	var otherReg int64
	if err := w.st.Pool.QueryRow(context.Background(),
		`insert into cash_registers (company_id, name) values ($1, 'Caja de otra') returning id`, other).Scan(&otherReg); err != nil {
		t.Fatal(err)
	}
	ctx, release, err := w.app.AcquireTenant(context.Background(), other)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	if v, err := w.back.CashAttendanceState(ctx, otherReg, ana, domain.AllPermissions()); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("persona de otra empresa: %+v %v; quiere ErrNotFound", v, err)
	}
}

// --- Abrir (US1) ---

func TestOpeningRequiresAValidClockIn(t *testing.T) {
	t.Parallel()
	w := newCashWorld(t)
	ana := w.person(t, "ana-abre", "cajero")

	if err := w.openAndCheck(t, ana); !errors.Is(err, domain.ErrCashClockInRequired) {
		t.Fatalf("abrir sin entrada = %v; quiere CLOCK_IN_REQUIRED", err)
	}
	if n := w.openSessions(t); n != 0 {
		t.Fatalf("el rechazo dejó %d turnos abiertos", n)
	}
	w.clock(t, ana, domain.ClockIn)
	w.now = w.now.Add(2 * time.Hour)
	if err := w.openAndCheck(t, ana); err != nil {
		t.Fatalf("abrir con entrada de 2 h: %v", err)
	}
}

// US1-4 y B2: una entrada de más de 16 h no cuenta; checar desde la hoja la deja «sin salida» y
// abre otra, y con ésa sí se abre.
func TestOpeningWithAStaleClockInAsksForANewOne(t *testing.T) {
	t.Parallel()
	w := newCashWorld(t)
	ana := w.person(t, "ana-vieja", "cajero")
	w.clock(t, ana, domain.ClockIn)
	w.now = w.now.Add(domain.StaleOpenAfter + time.Minute)

	v := w.state(t, w.ctx, ana, nil)
	if v.Ready || !v.Stale {
		t.Fatalf("entrada de más de 16 h: %+v", v)
	}
	if err := w.openAndCheck(t, ana); !errors.Is(err, domain.ErrCashClockInRequired) {
		t.Fatalf("abrir con entrada vieja = %v", err)
	}
	w.clock(t, ana, domain.ClockIn)
	if got := w.entryStatus(t, ana); len(got) != 2 || got[0] != "missing_out" || got[1] != "open" {
		t.Fatalf("entradas = %v; quiere la vieja sin salida y una nueva abierta", got)
	}
	if err := w.openAndCheck(t, ana); err != nil {
		t.Fatalf("abrir tras checar de nuevo: %v", err)
	}
}

// B15: la tableta tiene la sesión de alguien que ya se fue.
func TestOpeningAfterClockingOutIsRejected(t *testing.T) {
	t.Parallel()
	w := newCashWorld(t)
	ana := w.person(t, "ana-salio", "cajero")
	w.clock(t, ana, domain.ClockIn)
	w.now = w.now.Add(8 * time.Hour)
	w.clock(t, ana, domain.ClockOut)
	if err := w.openAndCheck(t, ana); !errors.Is(err, domain.ErrCashClockInRequired) {
		t.Fatalf("abrir tras marcar salida = %v", err)
	}
}

// B4 al abrir: la salida de Ana confirma en otra tableta mientras su caja se abre. La caja espera
// el candado y decide con la salida ya hecha: sin el `for share`, leería la entrada todavía
// abierta y abriría.
func TestOpeningWaitsForAClockOutInAnotherTablet(t *testing.T) {
	t.Parallel()
	w := newCashWorld(t)
	ana := w.person(t, "ana-carrera", "cajero")
	w.clock(t, ana, domain.ClockIn)

	ctx := context.Background()
	tx, err := w.st.Pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `select id from users where id = $1 for update`, ana); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `update attendance_entries set status = 'closed', clock_out_at = $2
		where user_id = $1 and status = 'open'`, ana, w.now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	other := w.tenant(t)
	go func() { done <- w.openAs(other, ana) }()
	w.waitForLockWaiter(t)
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err := <-done; !errors.Is(err, domain.ErrCashClockInRequired) {
		t.Fatalf("abrir mientras otra tableta marcaba la salida = %v; quiere CLOCK_IN_REQUIRED", err)
	}
}

// B4 al revés: con la caja leyendo la asistencia de Ana (`for share`), checar espera.
func TestClockWaitsWhileTheCashReadsTheAttendance(t *testing.T) {
	t.Parallel()
	w := newCashWorld(t)
	ana := w.person(t, "ana-espera", "cajero")
	ctx := context.Background()
	tx, err := w.st.Pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `select id from users where id = $1 for share`, ana); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	other := w.tenant(t)
	go func() {
		_, err := w.att.Clock(other, ana, testPIN, domain.ClockIn)
		done <- err
	}()
	w.waitForLockWaiter(t)
	select {
	case err := <-done:
		t.Fatalf("checar no esperó a la caja: %v", err)
	default:
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatalf("checar tras soltar el candado: %v", err)
	}
}

// --- Cerrar (US2) ---

func TestClosingRequiresTheOpenersClockOut(t *testing.T) {
	t.Parallel()
	w := newCashWorld(t)
	ana := w.person(t, "ana-cierra", "cajero")
	w.clock(t, ana, domain.ClockIn)
	if err := w.openAs(w.ctx, ana); err != nil {
		t.Fatal(err)
	}
	w.now = w.now.Add(7 * time.Hour)
	err := w.closeAndCheck(t, ana)
	if !errors.Is(err, domain.ErrCashOpenerClockedIn) || err.Error() != "Test ana-cierra abrió esta caja y no ha marcado su salida" {
		t.Fatalf("cerrar con Ana dentro = %v", err)
	}
	if n := w.openSessions(t); n != 1 {
		t.Fatalf("el rechazo cerró el turno: %d abiertos", n)
	}
	w.clock(t, ana, domain.ClockOut)
	if err := w.closeAndCheck(t, ana); err != nil {
		t.Fatalf("cerrar tras la salida: %v", err)
	}
}

// US2-4: salió a mediodía y volvió en la tarde; su entrada de la tarde está abierta.
func TestClosingAfterASplitShiftStillAsksForTheLastClockOut(t *testing.T) {
	t.Parallel()
	w := newCashWorld(t)
	ana := w.person(t, "ana-partido", "cajero")
	w.clock(t, ana, domain.ClockIn)
	if err := w.openAs(w.ctx, ana); err != nil {
		t.Fatal(err)
	}
	w.now = w.now.Add(4 * time.Hour)
	w.clock(t, ana, domain.ClockOut)
	w.now = w.now.Add(time.Hour)
	w.clock(t, ana, domain.ClockIn)
	w.now = w.now.Add(4 * time.Hour)
	if err := w.closeAndCheck(t, ana); !errors.Is(err, domain.ErrCashOpenerClockedIn) {
		t.Fatalf("cerrar con la entrada de la tarde abierta = %v", err)
	}
}

// B3: una entrada de más de 16 h también bloquea, y el GET dice que no se puede marcar con PIN.
func TestClosingWithAStaleEntryOnlyAllowsAuthorization(t *testing.T) {
	t.Parallel()
	w := newCashWorld(t)
	ana := w.person(t, "ana-olvido", "cajero")
	gina := w.person(t, "gina-olvido", "gerente")
	w.clock(t, ana, domain.ClockIn)
	if err := w.openAs(w.ctx, ana); err != nil {
		t.Fatal(err)
	}
	w.now = w.now.Add(domain.StaleOpenAfter + time.Hour)
	if err := w.closeAndCheck(t, gina); !errors.Is(err, domain.ErrCashOpenerClockedIn) {
		t.Fatalf("cerrar con entrada vieja = %v", err)
	}
	if v := w.state(t, w.ctx, gina, domain.PermissionsFor(domain.RoleGerente)); !v.Stale || !v.CanAuthorize {
		t.Fatalf("estado con entrada vieja: %+v", v)
	}
}

// B5 y B6: lo que cuenta es cómo están las cosas al cerrar, no al abrir.
func TestClosingUsesTheRuleAsItIsWhenClosing(t *testing.T) {
	t.Parallel()
	t.Run("asistencia encendida con la caja abierta", func(t *testing.T) {
		w := newCashWorld(t)
		enableAttendance(t, w.st, defaultCompanyID, false)
		ana := w.person(t, "ana-b5", "cajero")
		if err := w.openAs(w.ctx, ana); err != nil {
			t.Fatal(err)
		}
		enableAttendance(t, w.st, defaultCompanyID, true)
		if err := w.closeAndCheck(t, ana); err != nil {
			t.Fatalf("sin entrada abierta cierra: %v", err)
		}
		if err := w.openAs(w.ctx, ana); !errors.Is(err, domain.ErrCashClockInRequired) {
			t.Fatalf("reabrir = %v", err)
		}
		w.clock(t, ana, domain.ClockIn)
		if err := w.openAs(w.ctx, ana); err != nil {
			t.Fatal(err)
		}
		if err := w.closeAndCheck(t, ana); !errors.Is(err, domain.ErrCashOpenerClockedIn) {
			t.Fatalf("con entrada abierta = %v", err)
		}
	})
	t.Run("«Lleva asistencia» apagado con la caja abierta", func(t *testing.T) {
		w := newCashWorld(t)
		ana := w.person(t, "ana-b6", "cajero")
		w.clock(t, ana, domain.ClockIn)
		if err := w.openAs(w.ctx, ana); err != nil {
			t.Fatal(err)
		}
		if _, err := w.st.Pool.Exec(context.Background(), `update users set tracks_attendance = false where id = $1`, ana); err != nil {
			t.Fatal(err)
		}
		if err := w.closeAndCheck(t, ana); err != nil {
			t.Fatalf("ya no lleva asistencia = %v", err)
		}
	})
}

// B7: quien abrió fue dada de baja con la entrada abierta. Sigue pidiendo su salida; el GET dice
// que no está activa para que la hoja no ofrezca un PIN que checar rechazaría.
func TestClosingAfterTheOpenerWasDeactivated(t *testing.T) {
	t.Parallel()
	w := newCashWorld(t)
	ana := w.person(t, "ana-baja", "cajero")
	gina := w.person(t, "gina-baja", "gerente")
	w.clock(t, ana, domain.ClockIn)
	if err := w.openAs(w.ctx, ana); err != nil {
		t.Fatal(err)
	}
	if _, err := w.st.Pool.Exec(context.Background(), `update users set is_active = false where id = $1`, ana); err != nil {
		t.Fatal(err)
	}
	if err := w.closeAndCheck(t, gina); !errors.Is(err, domain.ErrCashOpenerClockedIn) {
		t.Fatalf("cerrar = %v", err)
	}
	if v := w.state(t, w.ctx, gina, domain.PermissionsFor(domain.RoleGerente)); v.Person.Active || !v.CanAuthorize {
		t.Fatalf("estado: %+v", v)
	}
	if err := w.closeAs(w.ctx, gina, domain.PermissionsFor(domain.RoleGerente), reasonOf("Ya no trabaja aquí")); err != nil {
		t.Fatalf("cierre autorizado: %v", err)
	}
}

// B19 y FR-012: lo que se mira es la entrada de quien abrió, no la caja ni al resto del personal.
func TestOneClockOutReleasesEveryRegisterAndOthersDoNotBlock(t *testing.T) {
	t.Parallel()
	w := newCashWorld(t)
	ana := w.person(t, "ana-dos", "cajero")
	beto := w.person(t, "beto-dentro", "cajero")
	vault := registerID(t, w.st, "Caja fuerte")
	w.clock(t, ana, domain.ClockIn)
	w.clock(t, beto, domain.ClockIn)
	if err := w.openAs(w.ctx, ana); err != nil {
		t.Fatal(err)
	}
	if _, err := w.back.OpenSession(w.ctx, vault, aperturaAMano(dec("0")), ana); err != nil {
		t.Fatalf("abrir la segunda caja: %v", err)
	}
	w.clock(t, ana, domain.ClockOut)
	if err := w.closeAndCheck(t, ana); err != nil {
		t.Fatalf("Beto dentro no bloquea el cierre de Ana: %v", err)
	}
	total := dec("0")
	if _, err := w.back.CloseSession(w.ctx, vault, ana, app.CierreCmd{Total: &total, Motivo: "Conté a mano"}); err != nil {
		t.Fatalf("la misma salida libera la segunda caja: %v", err)
	}
}

// B4 al cerrar: la salida confirma en otra tableta mientras se cierra; la caja espera y cierra.
func TestClosingWaitsForAClockOutInAnotherTablet(t *testing.T) {
	t.Parallel()
	w := newCashWorld(t)
	ana := w.person(t, "ana-cierre-carrera", "cajero")
	w.clock(t, ana, domain.ClockIn)
	if err := w.openAs(w.ctx, ana); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	tx, err := w.st.Pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `select id from users where id = $1 for update`, ana); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `update attendance_entries set status = 'closed', clock_out_at = $2
		where user_id = $1 and status = 'open'`, ana, w.now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	other := w.tenant(t)
	go func() { done <- w.closeAs(other, ana, nil, nil) }()
	w.waitForLockWaiter(t)
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatalf("cerrar mientras otra tableta marcaba la salida = %v; quiere que cierre", err)
	}
}

// --- Cierre autorizado (US3) ---

func TestAuthorizedCloseLeavesTheEntryWithoutClockOutAndItsEvent(t *testing.T) {
	t.Parallel()
	w := newCashWorld(t)
	ana := w.person(t, "ana-se-fue", "cajero")
	gina := w.person(t, "gina-autoriza", "gerente")
	beto := w.person(t, "beto-cajero", "cajero")
	w.clock(t, ana, domain.ClockIn)
	if err := w.openAs(w.ctx, ana); err != nil {
		t.Fatal(err)
	}
	w.now = w.now.Add(8 * time.Hour)
	gerente := domain.PermissionsFor(domain.RoleGerente)

	// US3-2: sin motivo, 400 y nada cambia.
	if err := w.closeAs(w.ctx, gina, gerente, reasonOf("  ")); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("sin motivo = %v", err)
	}
	// US3-3 / FR-008: sin el permiso, 403.
	if err := w.closeAs(w.ctx, beto, domain.PermissionsFor(domain.RoleCajero), reasonOf("Se fue sin checar")); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("sin permiso = %v", err)
	}
	if got := w.entryStatus(t, ana); len(got) != 1 || got[0] != "open" || w.openSessions(t) != 1 || w.authorizedEvents(t) != 0 {
		t.Fatalf("un rechazo cambió algo: entradas %v, turnos %d, eventos %d", got, w.openSessions(t), w.authorizedEvents(t))
	}

	// US3-1.
	if err := w.closeAs(w.ctx, gina, gerente, reasonOf("Se fue sin checar")); err != nil {
		t.Fatalf("cierre autorizado: %v", err)
	}
	if got := w.entryStatus(t, ana); len(got) != 1 || got[0] != "missing_out" {
		t.Fatalf("entradas de Ana = %v; quiere sin salida", got)
	}
	var actor, subject int64
	var reason, entry string
	if err := w.st.Pool.QueryRow(context.Background(), `select actor_user_id, subject_user_id, after->>'reason', after->>'entryId'
		from audit_events where action = 'cash.closed_without_opener_clock_out'`).Scan(&actor, &subject, &reason, &entry); err != nil {
		t.Fatalf("evento: %v", err)
	}
	var entryID int64
	_ = w.st.Pool.QueryRow(context.Background(), `select id from attendance_entries where user_id = $1`, ana).Scan(&entryID)
	if actor != gina || subject != ana || reason != "Se fue sin checar" || entry != strconv.FormatInt(entryID, 10) {
		t.Fatalf("evento: actor %d sujeto %d motivo %q entrada %s", actor, subject, reason, entry)
	}

	// US3-6: al día siguiente, antes de 16 h desde aquella entrada, checar es una entrada nueva.
	w.now = w.now.Add(4 * time.Hour)
	res, err := w.att.Clock(w.ctx, ana, testPIN, domain.ClockIn)
	if err != nil || res.Kind != domain.ClockIn {
		t.Fatalf("checar al día siguiente: %+v %v", res, err)
	}
}

// US3-7 y US3-8 (B10): quien abrió no se autoriza su propio cierre con una entrada de hoy.
func TestAuthorizingYourOwnClose(t *testing.T) {
	t.Parallel()
	w := newCashWorld(t)
	gina := w.person(t, "gina-propia", "gerente")
	gerente := domain.PermissionsFor(domain.RoleGerente)
	w.clock(t, gina, domain.ClockIn)
	if err := w.openAs(w.ctx, gina); err != nil {
		t.Fatal(err)
	}
	w.now = w.now.Add(8 * time.Hour)
	if v := w.state(t, w.ctx, gina, gerente); !v.IsSelf || v.CanAuthorize {
		t.Fatalf("estado del cierre propio: %+v", v)
	}
	if err := w.closeAs(w.ctx, gina, gerente, reasonOf("Me voy")); !errors.Is(err, domain.ErrCashClockOutWithPIN) {
		t.Fatalf("cierre propio con entrada de hoy = %v", err)
	}
	w.now = w.now.Add(9 * time.Hour)
	if err := w.closeAs(w.ctx, gina, gerente, reasonOf("No marqué ayer")); err != nil {
		t.Fatalf("cierre propio con entrada de más de 16 h: %v", err)
	}
}

// B8: autorizar cuando ya no hace falta es un cierre normal, sin evento.
func TestAuthorizedCloseWhenNotNeededIsANormalClose(t *testing.T) {
	t.Parallel()
	w := newCashWorld(t)
	ana := w.person(t, "ana-b8", "cajero")
	gina := w.person(t, "gina-b8", "gerente")
	w.clock(t, ana, domain.ClockIn)
	if err := w.openAs(w.ctx, ana); err != nil {
		t.Fatal(err)
	}
	w.clock(t, ana, domain.ClockOut)
	if err := w.closeAs(w.ctx, gina, domain.PermissionsFor(domain.RoleGerente), reasonOf("Se fue sin checar")); err != nil {
		t.Fatal(err)
	}
	if n := w.authorizedEvents(t); n != 0 {
		t.Fatalf("%d eventos de cierre autorizado sin que hiciera falta", n)
	}
}

// B12: el cierre autorizado falla después de marcar «sin salida» (pedido pendiente): todo se
// deshace, la entrada sigue abierta y no hay evento.
func TestAFailedAuthorizedCloseLeavesNothing(t *testing.T) {
	t.Parallel()
	w := newCashWorld(t)
	ana := w.person(t, "ana-b12", "cajero")
	gina := w.person(t, "gina-b12", "gerente")
	w.clock(t, ana, domain.ClockIn)
	if err := w.openAs(w.ctx, ana); err != nil {
		t.Fatal(err)
	}
	if _, err := w.st.Pool.Exec(context.Background(), `insert into orders (client_uuid, business_date, daily_number, service_type, subtotal, total, opened_by, register_session_id)
		select gen_random_uuid(), business_date, 1, 'mostrador', 20, 20, $1, id from register_sessions where register_id = $2 and status = 'abierta'`, ana, w.reg); err != nil {
		t.Fatalf("pedido pendiente: %v", err)
	}
	if err := w.closeAs(w.ctx, gina, domain.PermissionsFor(domain.RoleGerente), reasonOf("Se fue sin checar")); err == nil || errors.Is(err, domain.ErrCashOpenerClockedIn) {
		t.Fatalf("cerrar con un pedido pendiente = %v; quiere el rechazo del pedido", err)
	}
	if got := w.entryStatus(t, ana); len(got) != 1 || got[0] != "open" || w.authorizedEvents(t) != 0 {
		t.Fatalf("quedó algo de un cierre que falló: entradas %v, eventos %d", got, w.authorizedEvents(t))
	}
}

// B21: la entrada se cerró en otra tableta antes de marcarla: no se inventa el evento.
func TestAuthorizedCloseAfterTheEntryWasClosedElsewhere(t *testing.T) {
	t.Parallel()
	w := newCashWorld(t)
	ana := w.person(t, "ana-b21", "cajero")
	gina := w.person(t, "gina-b21", "gerente")
	w.clock(t, ana, domain.ClockIn)
	if err := w.openAs(w.ctx, ana); err != nil {
		t.Fatal(err)
	}
	w.clock(t, ana, domain.ClockOut)
	// Lo que vio la hoja ya no es cierto: la autorización se ignora y cierra normal, sin evento.
	if err := w.closeAs(w.ctx, gina, domain.PermissionsFor(domain.RoleGerente), reasonOf("Se fue sin checar")); err != nil {
		t.Fatal(err)
	}
	if got := w.entryStatus(t, ana); len(got) != 1 || got[0] != "closed" || w.authorizedEvents(t) != 0 {
		t.Fatalf("entradas %v, eventos %d", got, w.authorizedEvents(t))
	}
}

// B21 en carrera: otro cierre autorizado de otra caja de Ana marcó su entrada «sin salida» después de
// que éste la leyera abierta. El update espera el renglón y no encuentra nada abierto: la respuesta
// pide volver a intentar, no «Ana no ha marcado su salida», que ya es falso.
func TestAuthorizedCloseThatLosesTheRaceAsksToRetry(t *testing.T) {
	t.Parallel()
	w := newCashWorld(t)
	ana := w.person(t, "ana-b21-carrera", "cajero")
	gina := w.person(t, "gina-b21-carrera", "gerente")
	w.clock(t, ana, domain.ClockIn)
	if err := w.openAs(w.ctx, ana); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	tx, err := w.st.Pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	// Lo que hace el otro cierre: marca la entrada y todavía no confirma.
	if _, err := tx.Exec(ctx, `update attendance_entries set status = 'missing_out' where user_id = $1 and status = 'open'`, ana); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	other := w.tenant(t)
	go func() {
		done <- w.closeAs(other, gina, domain.PermissionsFor(domain.RoleGerente), reasonOf("Se fue sin checar"))
	}()
	w.waitForLockWaiter(t)
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	err = <-done
	if !errors.Is(err, domain.ErrAttendanceStateChanged) || errors.Is(err, domain.ErrCashOpenerClockedIn) {
		t.Fatalf("el cierre que perdió la carrera = %v; quiere ATTENDANCE_STATE_CHANGED (vuelve a intentarlo)", err)
	}
	if n := w.authorizedEvents(t); n != 0 {
		t.Fatalf("%d eventos de un cierre que no ocurrió", n)
	}
	if n := w.openSessions(t); n != 1 {
		t.Fatalf("el turno se cerró: %d abiertos", n)
	}
}

// B19 con autorización: Ana abrió dos cajas y se fue; cerrar las dos deja UN evento autorizado.
func TestAuthorizedCloseOfTwoRegistersLeavesOneEvent(t *testing.T) {
	t.Parallel()
	w := newCashWorld(t)
	ana := w.person(t, "ana-b19", "cajero")
	gina := w.person(t, "gina-b19", "gerente")
	vault := registerID(t, w.st, "Caja fuerte")
	w.clock(t, ana, domain.ClockIn)
	if err := w.openAs(w.ctx, ana); err != nil {
		t.Fatal(err)
	}
	if _, err := w.back.OpenSession(w.ctx, vault, aperturaAMano(dec("0")), ana); err != nil {
		t.Fatal(err)
	}
	gerente := domain.PermissionsFor(domain.RoleGerente)
	if err := w.closeAs(w.ctx, gina, gerente, reasonOf("Se fue sin checar")); err != nil {
		t.Fatal(err)
	}
	total := dec("0")
	if _, err := w.back.CloseSession(w.ctx, vault, gina, app.CierreCmd{Total: &total, Motivo: "Conté a mano",
		Perms: gerente, AuthorizedReason: reasonOf("Se fue sin checar")}); err != nil {
		t.Fatalf("segunda caja: %v", err)
	}
	if n := w.authorizedEvents(t); n != 1 {
		t.Fatalf("%d eventos autorizados; quiere 1", n)
	}
}

// B13: dos personas autorizan el mismo cierre a la vez. Una cierra; la otra ve la caja cerrada.
func TestTwoAuthorizedClosesAtOnceLeaveOneEvent(t *testing.T) {
	t.Parallel()
	w := newCashWorld(t)
	ana := w.person(t, "ana-b13", "cajero")
	gina := w.person(t, "gina-b13", "gerente")
	admin := w.person(t, "admin-b13", "admin")
	w.clock(t, ana, domain.ClockIn)
	if err := w.openAs(w.ctx, ana); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i, who := range []int64{gina, admin} {
		ctx := w.tenant(t)
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs[i] = w.closeAs(ctx, who, domain.AllPermissions(), reasonOf("Se fue sin checar"))
		}()
	}
	wg.Wait()
	ok := 0
	for _, err := range errs {
		switch {
		case err == nil:
			ok++
		case !errors.Is(err, domain.ErrNotFound):
			t.Fatalf("el segundo cierre = %v; quiere que vea la caja cerrada", err)
		}
	}
	if ok != 1 || w.authorizedEvents(t) != 1 {
		t.Fatalf("cierres %d, eventos %d; quiere 1 y 1", ok, w.authorizedEvents(t))
	}
}

// --- Sin asistencia (US4) ---

func TestWithoutAttendanceNothingChanges(t *testing.T) {
	t.Parallel()
	t.Run("asistencia apagada", func(t *testing.T) {
		w := newCashWorld(t)
		enableAttendance(t, w.st, defaultCompanyID, false)
		ana := w.person(t, "ana-us4", "cajero")
		if v := w.state(t, w.ctx, ana, nil); v.Applies || !v.Ready {
			t.Fatalf("estado: %+v", v)
		}
		if err := w.openAndCheck(t, ana); err != nil {
			t.Fatal(err)
		}
		if err := w.closeAndCheck(t, ana); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("no lleva asistencia", func(t *testing.T) {
		w := newCashWorld(t)
		owner := w.person(t, "dueno-us4", "admin")
		if _, err := w.st.Pool.Exec(context.Background(), `update users set tracks_attendance = false where id = $1`, owner); err != nil {
			t.Fatal(err)
		}
		if v := w.state(t, w.ctx, owner, nil); v.Applies || !v.Ready {
			t.Fatalf("estado: %+v", v)
		}
		if err := w.openAndCheck(t, owner); err != nil {
			t.Fatal(err)
		}
		if err := w.closeAndCheck(t, owner); err != nil {
			t.Fatal(err)
		}
	})
}

// --- La frontera HTTP (T009, FR-008) ---

func TestCashAttendanceHTTP(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	enableAttendance(t, st, defaultCompanyID, true)
	jm := auth.NewManager("secreto-de-pruebas-suficientemente-largo-para-el-manager", nil)
	h := httpapi.NewHandlers(httpapi.Deps{JWT: jm, Backoffice: app.NewBackofficeService(st, nil),
		MenuCache: cache.NewMenuCache(""), Broker: realtime.NewBroker()})
	r := httpapi.Router(config.Config{}, jm, h, st)
	token := func(username, role string) (int64, string) {
		id := makeUser(t, st, username, role)
		tok, err := jm.Issue(domain.User{ID: id, CompanyID: defaultCompanyID, Name: username, Role: domain.Role(role)})
		if err != nil {
			t.Fatal(err)
		}
		return id, tok
	}
	ana, anaTok := token("ana-http", "cajero")
	_, betoTok := token("beto-http", "cajero")
	reg := registerID(t, st, "Caja principal")
	const path = "/api/v1/cash-sessions/attendance"

	for _, q := range []string{"", "?registerId=", "?registerId=abc", "?registerId=-3"} {
		if w := do(t, r, http.MethodGet, path+q, anaTok, nil, ""); w.Code != http.StatusBadRequest {
			t.Errorf("%q = %d; quiere 400", q, w.Code)
		}
	}
	if w := do(t, r, http.MethodGet, path+"?registerId=999999", anaTok, nil, ""); w.Code != http.StatusNotFound {
		t.Errorf("caja que no existe = %d; quiere 404", w.Code)
	}
	w := do(t, r, http.MethodGet, path+"?registerId="+strconv.FormatInt(reg, 10), anaTok, nil, "")
	if w.Code != http.StatusOK {
		t.Fatalf("GET = %d %s", w.Code, w.Body.String())
	}
	var v map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &v)
	if v["stage"] != "open" || v["applies"] != true || v["ready"] != false {
		t.Fatalf("GET = %s", w.Body.String())
	}

	// Abrir sin entrada: 409 con su código, desde el front anterior también.
	body := []byte(`{"registerId":` + strconv.FormatInt(reg, 10) + `,"openingCash":500,"manualReason":"a mano"}`)
	if w := do(t, r, http.MethodPost, "/api/v1/cash-sessions", anaTok, body, "application/json"); w.Code != http.StatusConflict ||
		!jsonHasCode(w.Body.Bytes(), "CLOCK_IN_REQUIRED") {
		t.Fatalf("abrir sin entrada = %d %s", w.Code, w.Body.String())
	}
	// Se abre a nombre de Ana, con su entrada puesta a mano.
	if _, err := st.Pool.Exec(context.Background(), `insert into attendance_entries (user_id, business_date, clock_in_at, status, source)
		values ($1, current_date, now(), 'open', 'clock')`, ana); err != nil {
		t.Fatal(err)
	}
	if w := do(t, r, http.MethodPost, "/api/v1/cash-sessions", anaTok, body, "application/json"); w.Code != http.StatusCreated {
		t.Fatalf("abrir con entrada = %d %s", w.Code, w.Body.String())
	}
	closeBody := []byte(`{"registerId":` + strconv.FormatInt(reg, 10) + `,"countedCash":500,"manualReason":"a mano","authorizedClose":{"reason":"Se fue sin checar"}}`)
	if w := do(t, r, http.MethodPost, "/api/v1/cash-sessions/close", betoTok, closeBody, "application/json"); w.Code != http.StatusForbidden {
		t.Fatalf("cierre autorizado de un cajero = %d %s; quiere 403", w.Code, w.Body.String())
	}
	plain := []byte(`{"registerId":` + strconv.FormatInt(reg, 10) + `,"countedCash":500,"manualReason":"a mano"}`)
	if w := do(t, r, http.MethodPost, "/api/v1/cash-sessions/close", betoTok, plain, "application/json"); w.Code != http.StatusConflict ||
		!jsonHasCode(w.Body.Bytes(), "OPENER_NOT_CLOCKED_OUT") {
		t.Fatalf("cerrar con Ana dentro = %d %s", w.Code, w.Body.String())
	}
	_, ginaTok := token("gina-http", "gerente")
	if w := do(t, r, http.MethodPost, "/api/v1/cash-sessions/close", ginaTok, closeBody, "application/json"); w.Code != http.StatusOK {
		t.Fatalf("cierre autorizado de la gerente = %d %s", w.Code, w.Body.String())
	}
}

func jsonHasCode(b []byte, code string) bool {
	var env struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	return json.Unmarshal(b, &env) == nil && env.Error.Code == code
}
