//go:build integration

package integration

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/auth"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
)

const testPIN = "4821"

// clockingEmployeeIn crea a alguien con PIN en `company` y le activa la asistencia a la empresa.
func clockingEmployeeIn(t *testing.T, st *store.Store, company int64, username string) int64 {
	t.Helper()
	id := makeUserIn(t, st, company, username, "cajero")
	hash, err := auth.HashSecret(testPIN)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if _, err := st.Pool.Exec(ctx, `update users set pin_hash = $2 where id = $1`, id, hash); err != nil {
		t.Fatal(err)
	}
	enableAttendance(t, st, company, true)
	return id
}

// enableAttendance prende o apaga la asistencia de `company`. Una empresa recién creada no tiene
// fila de ajustes (se crea al guardar el primero), así que va como upsert.
func enableAttendance(t *testing.T, st *store.Store, company int64, on bool) {
	t.Helper()
	if _, err := st.Pool.Exec(context.Background(), `
		insert into business_settings (company_id, business_name, attendance_enabled) values ($1, 'Prueba', $2)
		on conflict (company_id) do update set attendance_enabled = excluded.attendance_enabled`, company, on); err != nil {
		t.Fatalf("activar asistencia: %v", err)
	}
}

// attendanceAs devuelve el servicio bajo el rol de la app y la sesión de `company`, con el reloj en `now`.
func attendanceAs(t *testing.T, company int64, now *time.Time) (*app.AttendanceService, context.Context) {
	t.Helper()
	st := appRoleStore(t)
	ctx, release, err := st.AcquireTenant(context.Background(), company)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(release)
	return app.NewAttendanceService(st, func() time.Time { return *now }), ctx
}

func TestClockInAndOutLeavesTheEntryAndItsEvent(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	co := makeCompany(t, st, "checar-ok")
	ana := clockingEmployeeIn(t, st, co, "ana-checa")
	now := time.Date(2026, 10, 10, 15, 2, 0, 0, time.UTC) // 9:02 en Ciudad de México
	svc, ctx := attendanceAs(t, co, &now)

	in, err := svc.Clock(ctx, ana, testPIN, domain.ClockIn)
	if err != nil {
		t.Fatalf("entrada: %v", err)
	}
	if in.Kind != domain.ClockIn || !in.At.Equal(now) {
		t.Fatalf("entrada = %+v", in)
	}
	_, people, _ := svc.Roster(ctx)
	if len(people) != 1 || people[0].OpenSince == nil {
		t.Fatalf("tras entrar, el roster no la muestra dentro: %+v", people)
	}

	now = now.Add(8*time.Hour + 10*time.Minute)
	out, err := svc.Clock(ctx, ana, testPIN, domain.ClockOut)
	if err != nil {
		t.Fatalf("salida: %v", err)
	}
	if out.WorkedMinutes == nil || *out.WorkedMinutes != 490 {
		t.Fatalf("trabajó %v minutos, quiere 490", out.WorkedMinutes)
	}

	var status, businessDay string
	var branch *int64
	if err := st.Pool.QueryRow(context.Background(),
		`select status, business_date::text, branch_id from attendance_entries where user_id = $1`, ana).
		Scan(&status, &businessDay, &branch); err != nil {
		t.Fatal(err)
	}
	if status != "closed" || businessDay != "2026-10-10" || branch == nil {
		t.Fatalf("entrada guardada: status=%s fecha=%s sucursal=%v", status, businessDay, branch)
	}
	var eventCount int
	if err := st.Pool.QueryRow(context.Background(),
		`select count(*) from audit_events where actor_user_id = $1 and action in ('attendance.clock_in', 'attendance.clock_out')`, ana).
		Scan(&eventCount); err != nil {
		t.Fatal(err)
	}
	if eventCount != 2 {
		t.Fatalf("hay %d eventos de checar, quiere 2", eventCount)
	}
}

func TestClockRejectsWhatShouldNotHappen(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	co := makeCompany(t, st, "checar-no")
	ana := clockingEmployeeIn(t, st, co, "ana-no")
	noPIN := makeUserIn(t, st, co, "sin-pin", "cajero")
	ownerUser := clockingEmployeeIn(t, st, co, "dueno-no")
	if _, err := st.Pool.Exec(context.Background(), `update users set tracks_attendance = false where id = $1`, ownerUser); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 10, 10, 15, 0, 0, 0, time.UTC)
	svc, ctx := attendanceAs(t, co, &now)

	if _, err := svc.Clock(ctx, ana, "0000", domain.ClockIn); !errors.Is(err, domain.ErrInvalidCredentials) {
		t.Errorf("PIN equivocado: %v", err)
	}
	if _, err := svc.Clock(ctx, noPIN, testPIN, domain.ClockIn); !errors.Is(err, domain.ErrInvalidCredentials) {
		t.Errorf("sin PIN: %v", err)
	}
	if _, err := svc.Clock(ctx, ownerUser, testPIN, domain.ClockIn); !errors.Is(err, domain.ErrAttendanceNotTracked) {
		t.Errorf("sin «Lleva asistencia»: %v", err)
	}
	if _, err := svc.Clock(ctx, ana, testPIN, domain.ClockOut); !errors.Is(err, domain.ErrAttendanceStateChanged) {
		t.Errorf("salida sin entrada: %v", err)
	}
	_, people, _ := svc.Roster(ctx)
	for _, p := range people {
		if p.UserID == ownerUser {
			t.Error("quien no lleva asistencia aparece para checar")
		}
		if p.UserID == noPIN && p.HasPin {
			t.Error("quien no tiene PIN aparece con PIN")
		}
	}

	enableAttendance(t, st, co, false)
	if _, err := svc.Clock(ctx, ana, testPIN, domain.ClockIn); !errors.Is(err, domain.ErrAttendanceDisabled) {
		t.Errorf("con la asistencia apagada: %v", err)
	}
	if enabled, people, err := svc.Roster(ctx); err != nil || enabled || people == nil || len(people) != 0 {
		t.Errorf("roster apagado = %v %v %v; quiere apagado y lista vacía (no null)", enabled, people, err)
	}
}

// Olvidó la salida de ayer: hoy entra de nuevo, y lo de ayer queda «sin salida» para que lo corrija
// el gerente, sin sumarle veinticuatro horas.
func TestAForgottenEntryBecomesMissingOutAndAddsNoDay(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	co := makeCompany(t, st, "checar-olvido")
	ana := clockingEmployeeIn(t, st, co, "ana-olvido")
	now := time.Date(2026, 10, 10, 15, 0, 0, 0, time.UTC)
	svc, ctx := attendanceAs(t, co, &now)
	if _, err := svc.Clock(ctx, ana, testPIN, domain.ClockIn); err != nil {
		t.Fatal(err)
	}
	now = now.Add(24 * time.Hour)
	if _, people, _ := svc.Roster(ctx); people[0].OpenSince != nil {
		t.Fatal("una entrada de ayer sigue mostrándose como «dentro»: la pantalla ofrecería Salida")
	}
	if _, err := svc.Clock(ctx, ana, testPIN, domain.ClockIn); err != nil {
		t.Fatalf("entrar al día siguiente: %v", err)
	}
	var statuses []string
	rows, _ := st.Pool.Query(context.Background(), `select status from attendance_entries where user_id = $1 order by clock_in_at`, ana)
	for rows.Next() {
		var s string
		_ = rows.Scan(&s)
		statuses = append(statuses, s)
	}
	rows.Close()
	if len(statuses) != 2 || statuses[0] != "missing_out" || statuses[1] != "open" {
		t.Fatalf("estados = %v, quiere [missing_out open]", statuses)
	}
	var closedID *int64
	if err := st.Pool.QueryRow(context.Background(), `select (after->>'closedAsMissingOut')::bigint from audit_events
		where action = 'attendance.clock_in' order by id desc limit 1`).Scan(&closedID); err != nil || closedID == nil {
		t.Fatalf("la entrada que quedó sin salida no dejó rastro en la bitácora: %v %v", closedID, err)
	}
}

// Dos tabletas checan a la misma persona al mismo tiempo: queda una sola entrada.
func TestTwoTabletsAtOnceLeaveASingleEntry(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	co := makeCompany(t, st, "checar-carrera")
	ana := clockingEmployeeIn(t, st, co, "ana-carrera")
	now := time.Date(2026, 10, 10, 15, 0, 0, 0, time.UTC)
	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i := range 2 {
		svc, ctx := attendanceAs(t, co, &now)
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, errs[i] = svc.Clock(ctx, ana, testPIN, domain.ClockIn)
		}()
	}
	wg.Wait()
	var n int
	if err := st.Pool.QueryRow(context.Background(), `select count(*) from attendance_entries where user_id = $1`, ana).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("hay %d entradas tras dos checadas simultáneas (errores: %v)", n, errs)
	}
	if (errs[0] == nil) == (errs[1] == nil) {
		t.Fatalf("una de las dos debía rechazarse: %v", errs)
	}
}

// Con dos sucursales, la entrada no adivina en cuál fue.
func TestWithTwoBranchesTheEntryDoesNotGuessABranch(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	co := makeCompany(t, st, "checar-dos-suc")
	ana := clockingEmployeeIn(t, st, co, "ana-dos-suc")
	if _, err := st.Pool.Exec(context.Background(),
		`insert into branches (company_id, branch_number, code, name) values ($1, 2, 'S2', 'Sucursal 2')`, co); err != nil {
		t.Fatalf("segunda sucursal: %v", err)
	}
	now := time.Date(2026, 10, 10, 15, 0, 0, 0, time.UTC)
	svc, ctx := attendanceAs(t, co, &now)
	if _, err := svc.Clock(ctx, ana, testPIN, domain.ClockIn); err != nil {
		t.Fatalf("checar con dos sucursales: %v", err)
	}
	var branch *int64
	if err := st.Pool.QueryRow(context.Background(), `select branch_id from attendance_entries where user_id = $1`, ana).Scan(&branch); err != nil {
		t.Fatal(err)
	}
	if branch != nil {
		t.Fatalf("la entrada tomó la sucursal %d con dos activas", *branch)
	}
}

func TestAttendanceIsNotVisibleNorClockableFromAnotherCompany(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ownerCompany := makeCompany(t, st, "checar-duena")
	otherCompany := makeCompany(t, st, "checar-otra")
	ana := clockingEmployeeIn(t, st, ownerCompany, "ana-duena")
	enableAttendance(t, st, otherCompany, true)
	now := time.Date(2026, 10, 10, 15, 0, 0, 0, time.UTC)
	inTheThreeCases(t, ownerCompany, otherCompany, func(t *testing.T, s *store.Store, ctx context.Context) {
		svc := app.NewAttendanceService(s, func() time.Time { return now })
		if _, err := svc.Clock(ctx, ana, testPIN, domain.ClockIn); err == nil {
			t.Fatal("checó a una persona de otra empresa")
		}
		_, people, err := svc.Roster(ctx)
		if err != nil {
			return // sin empresa no hay ajustes que leer: no ver nada es el resultado correcto
		}
		for _, p := range people {
			if p.UserID == ana {
				t.Fatal("el roster muestra a una persona de otra empresa")
			}
		}
	})
}

// La entrada y la salida guardan cada una su origen: alguien puede entrar en la tableta del local y
// salir desde su celular, y eso es justo lo que el gerente necesita ver. Una clave malformada no
// rechaza la checada: se guarda sin clave.
func TestClockInAndOutEachKeepTheirOrigin(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	co := makeCompany(t, st, "checar-origen")
	ana := clockingEmployeeIn(t, st, co, "ana-origen")
	now := time.Date(2026, 10, 10, 15, 0, 0, 0, time.UTC)
	svc, ctx := attendanceAs(t, co, &now)

	tablet := app.WithOrigin(ctx, domain.NormalizeOrigin("187.190.4.12", "0123456789abcdef0123456789abcdef", `{"browser":"Edge 141"}`))
	if _, err := svc.Clock(tablet, ana, testPIN, domain.ClockIn); err != nil {
		t.Fatal(err)
	}
	now = now.Add(8 * time.Hour)
	phone := app.WithOrigin(ctx, domain.NormalizeOrigin("201.141.22.9", "NO-ES-UNA-CLAVE", `{"browser":"Chrome 141","model":"SM-A145"}`))
	if _, err := svc.Clock(phone, ana, testPIN, domain.ClockOut); err != nil {
		t.Fatalf("una clave malformada no debe rechazar la salida: %v", err)
	}

	var inIP, outIP, inKey string
	var outKey *string
	var outDev string
	if err := st.Pool.QueryRow(context.Background(), `
		select host(in_ip), host(out_ip), in_device_key, out_device_key, out_device->>'model'
		  from attendance_entries where user_id = $1`, ana).Scan(&inIP, &outIP, &inKey, &outKey, &outDev); err != nil {
		t.Fatal(err)
	}
	if inIP != "187.190.4.12" || outIP != "201.141.22.9" || inKey != "0123456789abcdef0123456789abcdef" || outKey != nil || outDev != "SM-A145" {
		t.Fatalf("origen guardado: in=%s out=%s inKey=%s outKey=%v outModel=%s", inIP, outIP, inKey, outKey, outDev)
	}
	var evIP string
	if err := st.Pool.QueryRow(context.Background(),
		`select host(ip) from audit_events where action = 'attendance.clock_out' and actor_user_id = $1`, ana).Scan(&evIP); err != nil {
		t.Fatal(err)
	}
	if evIP != "201.141.22.9" {
		t.Fatalf("el evento de la salida guardó la IP %s", evIP)
	}
}
