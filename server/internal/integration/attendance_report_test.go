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

func TestTheAttendanceReportMeasuresLatenessOnlyWhenTheBusinessAsksFor(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	co := makeCompany(t, st, "report-measure")
	ana := clockingEmployeeIn(t, st, co, "ana-report")
	manager := makeUserIn(t, st, co, "gerente-report", "gerente")
	// Lunes 12 de octubre de 2026, 9:10 a 17:40 en Ciudad de México.
	now := time.Date(2026, 10, 12, 15, 10, 0, 0, time.UTC)
	svc, ctx := attendanceAs(t, co, &now)
	ctx = app.WithActorUser(ctx, manager)
	if _, err := svc.Clock(ctx, ana, testPIN, domain.ClockIn); err != nil {
		t.Fatal(err)
	}
	now = now.Add(8*time.Hour + 30*time.Minute)
	if _, err := svc.Clock(ctx, ana, testPIN, domain.ClockOut); err != nil {
		t.Fatal(err)
	}
	monday := time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC)

	rep, err := svc.Report(ctx, monday, monday, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.People) != 1 || rep.People[0].WorkedMinutes != 510 || rep.People[0].LateMinutes != nil {
		t.Fatalf("con la medición apagada solo van horas: %+v", rep.People)
	}

	if err := svc.SetSettings(ctx, manager, app.AttendanceSettingsView{Enabled: true, MeasureSchedule: true, GraceInMin: 61}); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("una tolerancia de 61 minutos se aceptó: %v", err)
	}
	if err := svc.SetSettings(ctx, manager, app.AttendanceSettingsView{Enabled: true, MeasureSchedule: true}); err != nil {
		t.Fatal(err)
	}
	if err := svc.SetSchedule(ctx, manager, ana, monday.AddDate(0, 0, -7), []app.ScheduleShiftView{
		{Weekday: 1, ShiftNo: 1, Start: 9 * 60, End: 17 * 60}}); err != nil {
		t.Fatalf("horario: %v", err)
	}
	if err := svc.SetSchedule(ctx, manager, ana, monday.AddDate(0, 0, -7), nil); err == nil {
		t.Fatal("dos versiones con la misma vigencia se aceptaron")
	}
	rep, err = svc.Report(ctx, monday, monday, &ana)
	if err != nil {
		t.Fatal(err)
	}
	d := rep.People[0].Days[0]
	if d.LateMinutes == nil || *d.LateMinutes != 10 || d.OvertimeMinutes == nil || *d.OvertimeMinutes != 40 {
		t.Fatalf("día medido = %+v; quiere 10 de retardo y 40 extra", d)
	}

	var actions []string
	rows, _ := st.Pool.Query(context.Background(), `select action from audit_events where actor_user_id = $1 order by id`, manager)
	for rows.Next() {
		var a string
		_ = rows.Scan(&a)
		actions = append(actions, a)
	}
	rows.Close()
	if len(actions) != 2 || actions[0] != "attendance.settings_changed" || actions[1] != "attendance.schedule_set" {
		t.Fatalf("bitácora del gerente = %v", actions)
	}
}

func TestTurningOffTracksAttendanceRemovesThePersonFromTheRoster(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	co := makeCompany(t, st, "report-tracks")
	ana := clockingEmployeeIn(t, st, co, "ana-tracks")
	admin := makeUserIn(t, st, co, "admin-tracks", "admin")
	now := time.Date(2026, 10, 12, 15, 0, 0, 0, time.UTC)
	svc, ctx := attendanceAs(t, co, &now)
	if err := svc.SetTracksAttendance(ctx, admin, ana, false); err != nil {
		t.Fatal(err)
	}
	_, people, _ := svc.Roster(ctx)
	for _, p := range people {
		if p.UserID == ana {
			t.Fatal("sigue en la lista para checar")
		}
	}
	var before, after string
	if err := st.Pool.QueryRow(context.Background(), `select before->>'tracksAttendance', after->>'tracksAttendance'
		from audit_events where action = 'user.tracks_attendance_changed' and subject_user_id = $1`, ana).Scan(&before, &after); err != nil {
		t.Fatal(err)
	}
	if before != "true" || after != "false" {
		t.Fatalf("antes → después = %s → %s", before, after)
	}
}

func TestTheAttendanceReportIsNotReadableFromAnotherCompany(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	owner := makeCompany(t, st, "report-owner")
	other := makeCompany(t, st, "report-other")
	ana := clockingEmployeeIn(t, st, owner, "ana-iso")
	now := time.Date(2026, 10, 12, 15, 0, 0, 0, time.UTC)
	svc, ctx := attendanceAs(t, owner, &now)
	if _, err := svc.Clock(ctx, ana, testPIN, domain.ClockIn); err != nil {
		t.Fatal(err)
	}
	day := time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC)
	inTheThreeCases(t, owner, other, func(t *testing.T, s *store.Store, ctx context.Context) {
		a := app.NewAttendanceService(s, func() time.Time { return now })
		if rep, err := a.Report(ctx, day, day, nil); err == nil && len(rep.People) > 0 {
			t.Fatalf("el reporte trae %d personas de otra empresa", len(rep.People))
		}
		if v, err := a.Schedules(ctx, ana); err == nil && len(v) > 0 {
			t.Fatal("se leyó el horario de otra empresa")
		}
	})
}
