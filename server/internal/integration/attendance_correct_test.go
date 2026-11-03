//go:build integration

package integration

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
)

var (
	gerentePerms = domain.PermissionsFor(domain.RoleGerente)
	adminPerms   = domain.PermissionsFor(domain.RoleAdmin)
)

func TestCompletingAForgottenClockOutKeepsTheOriginalAndLeavesAnEvent(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	co := makeCompany(t, st, "correct-out")
	ana := clockingEmployeeIn(t, st, co, "ana-correct")
	manager := makeUserIn(t, st, co, "gerente-correct", "gerente")
	now := time.Date(2026, 10, 12, 15, 0, 0, 0, time.UTC)
	svc, ctx := attendanceAs(t, co, &now)
	if _, err := svc.Clock(ctx, ana, testPIN, domain.ClockIn); err != nil {
		t.Fatal(err)
	}
	now = now.Add(24 * time.Hour)
	if _, err := svc.Clock(ctx, ana, testPIN, domain.ClockIn); err != nil { // la de ayer queda sin salida
		t.Fatal(err)
	}
	var forgotten int64
	if err := st.Pool.QueryRow(context.Background(), `select id from attendance_entries where user_id = $1 and status = 'missing_out'`, ana).Scan(&forgotten); err != nil {
		t.Fatal(err)
	}
	clockOut := time.Date(2026, 10, 12, 23, 0, 0, 0, time.UTC)
	if err := svc.Correct(ctx, manager, gerentePerms, app.CorrectCmd{EntryID: forgotten, Field: "clock_out_at", At: clockOut, Reason: "ok"}); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("una corrección sin motivo suficiente se aceptó: %v", err)
	}
	if err := svc.Correct(ctx, manager, gerentePerms, app.CorrectCmd{EntryID: forgotten, Field: "clock_out_at", At: clockOut, Reason: "Olvidó checar"}); err != nil {
		t.Fatalf("corregir: %v", err)
	}
	var status string
	var corrected bool
	var before *time.Time
	if err := st.Pool.QueryRow(context.Background(), `select e.status, e.corrected, c.before from attendance_entries e
		join attendance_corrections c on c.entry_id = e.id where e.id = $1`, forgotten).Scan(&status, &corrected, &before); err != nil {
		t.Fatal(err)
	}
	if status != "closed" || !corrected || before != nil {
		t.Fatalf("status=%s corregido=%v antes=%v", status, corrected, before)
	}
	var subject int64
	if err := st.Pool.QueryRow(context.Background(), `select subject_user_id from audit_events where action = 'attendance.corrected'`).Scan(&subject); err != nil || subject != ana {
		t.Fatalf("el evento no nombra a la persona corregida: %d %v", subject, err)
	}
}

// Corregir lo propio pide su propio permiso: el gerente no se corrige a sí mismo; el admin sí.
func TestAManagerCannotCorrectTheirOwnAttendance(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	co := makeCompany(t, st, "correct-own")
	manager := clockingEmployeeIn(t, st, co, "gerente-own")
	admin := clockingEmployeeIn(t, st, co, "admin-own")
	now := time.Date(2026, 10, 12, 15, 0, 0, 0, time.UTC)
	svc, ctx := attendanceAs(t, co, &now)
	in, out := time.Date(2026, 10, 10, 15, 0, 0, 0, time.UTC), time.Date(2026, 10, 10, 23, 0, 0, 0, time.UTC)
	if _, err := svc.ManualEntry(ctx, manager, gerentePerms, app.ManualEntryCmd{UserID: manager, In: in, Out: out, Reason: "Olvidé checar"}); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("el gerente se capturó a sí mismo: %v", err)
	}
	if _, err := svc.ManualEntry(ctx, admin, adminPerms, app.ManualEntryCmd{UserID: admin, In: in, Out: out, Reason: "Olvidé checar"}); err != nil {
		t.Fatalf("el admin no pudo capturarse: %v", err)
	}
	var source, reason string
	if err := st.Pool.QueryRow(context.Background(), `select e.source, ev.after->>'reason' from attendance_entries e
		join audit_events ev on ev.entity_id = e.id and ev.action = 'attendance.manual_entry'
		where e.user_id = $1`, admin).Scan(&source, &reason); err != nil {
		t.Fatalf("la captura a mano no dejó registro ni evento: %v", err)
	}
	if source != "manual" || reason != "Olvidé checar" {
		t.Fatalf("captura a mano = %s / %q", source, reason)
	}
	if _, err := svc.ManualEntry(ctx, manager, gerentePerms, app.ManualEntryCmd{UserID: admin, In: in.Add(time.Hour), Out: out, Reason: "Doble"}); !errors.Is(err, domain.ErrAttendanceOverlap) {
		t.Fatalf("una captura encimada se aceptó: %v", err)
	}
}

// Dos gerentes capturan a la vez el mismo turno de alguien sin registros ese día: solo uno entra.
// Un `for update` sobre las entradas no bloquea nada cuando no hay filas; el candado es la persona.
func TestTwoSimultaneousManualEntriesOfTheSameShiftLeaveOne(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	co := makeCompany(t, st, "correct-race")
	ana := clockingEmployeeIn(t, st, co, "ana-race")
	g1 := makeUserIn(t, st, co, "g1-race", "gerente")
	g2 := makeUserIn(t, st, co, "g2-race", "gerente")
	now := time.Date(2026, 10, 12, 15, 0, 0, 0, time.UTC)
	in, out := time.Date(2026, 10, 10, 15, 0, 0, 0, time.UTC), time.Date(2026, 10, 10, 23, 0, 0, 0, time.UTC)
	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i, g := range []int64{g1, g2} {
		svc, ctx := attendanceAs(t, co, &now)
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, errs[i] = svc.ManualEntry(ctx, g, gerentePerms, app.ManualEntryCmd{UserID: ana, In: in, Out: out, Reason: "No checó"})
		}()
	}
	wg.Wait()
	var n int
	if err := st.Pool.QueryRow(context.Background(), `select count(*) from attendance_entries where user_id = $1`, ana).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("quedaron %d registros del mismo turno (errores %v)", n, errs)
	}
}

// Sin ningún permiso de corrección, un id que existe y uno que no responden igual: si no, el 404
// contra el 403 sirve para enumerar los registros de la empresa.
func TestWithoutPermissionAnExistingAndAMissingEntryLookTheSame(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	co := makeCompany(t, st, "correct-enum")
	ana := clockingEmployeeIn(t, st, co, "ana-enum")
	cashier := makeUserIn(t, st, co, "cajero-enum", "cajero")
	now := time.Date(2026, 10, 12, 15, 0, 0, 0, time.UTC)
	svc, ctx := attendanceAs(t, co, &now)
	if _, err := svc.Clock(ctx, ana, testPIN, domain.ClockIn); err != nil {
		t.Fatal(err)
	}
	var entry int64
	if err := st.Pool.QueryRow(context.Background(), `select id from attendance_entries where user_id = $1`, ana).Scan(&entry); err != nil {
		t.Fatal(err)
	}
	perms := domain.PermissionsFor(domain.RoleCajero)
	for _, id := range []int64{entry, entry + 999} {
		err := svc.Correct(ctx, cashier, perms, app.CorrectCmd{EntryID: id, Field: "clock_out_at", At: now.Add(time.Hour), Reason: "Prueba"})
		if !errors.Is(err, domain.ErrForbidden) {
			t.Fatalf("registro %d sin permiso = %v, quiere ErrForbidden en los dos casos", id, err)
		}
	}
}

func TestACorrectionCannotReachAnotherCompany(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	owner := makeCompany(t, st, "correct-owner")
	other := makeCompany(t, st, "correct-other")
	ana := clockingEmployeeIn(t, st, owner, "ana-iso-c")
	now := time.Date(2026, 10, 12, 15, 0, 0, 0, time.UTC)
	svc, ctx := attendanceAs(t, owner, &now)
	if _, err := svc.Clock(ctx, ana, testPIN, domain.ClockIn); err != nil {
		t.Fatal(err)
	}
	var entry int64
	if err := st.Pool.QueryRow(context.Background(), `select id from attendance_entries where user_id = $1`, ana).Scan(&entry); err != nil {
		t.Fatal(err)
	}
	inTheThreeCases(t, owner, other, func(t *testing.T, s *store.Store, ctx context.Context) {
		a := app.NewAttendanceService(s, func() time.Time { return now })
		if err := a.Correct(ctx, 1, adminPerms, app.CorrectCmd{EntryID: entry, Field: "clock_out_at", At: now.Add(time.Hour), Reason: "Intento ajeno"}); err == nil {
			t.Fatal("se corrigió un registro de otra empresa")
		}
	})
}
