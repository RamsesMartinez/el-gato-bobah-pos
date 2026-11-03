package domain

import (
	"errors"
	"strings"
	"testing"
	"time"
)

// Abrir caja exige una entrada vigente de quien abre (spec 046). Las 16 h exactas siguen vigentes:
// `IsStaleOpen` usa `>`, y la apertura y checar tienen que estar de acuerdo en qué es «vieja», o
// la hoja pediría marcar una entrada que checar trataría como salida.
func TestCashOpenAttendance(t *testing.T) {
	now := at("2026-10-30 09:00")
	since := func(d time.Duration) *time.Time { v := now.Add(-d); return &v }
	cases := []struct {
		name                         string
		a                            CashAttendance
		applies, ready, stale, isErr bool
	}{
		{"asistencia apagada", CashAttendance{Enabled: false, Tracks: true, Active: true}, false, true, false, false},
		{"no lleva asistencia", CashAttendance{Enabled: true, Tracks: false, Active: true}, false, true, false, false},
		{"sin entrada", CashAttendance{Enabled: true, Tracks: true, Active: true}, true, false, false, true},
		{"entrada de 2 h", CashAttendance{Enabled: true, Tracks: true, Active: true, OpenSince: since(2 * time.Hour)}, true, true, false, false},
		{"entrada de 16 h exactas sigue vigente (B1)", CashAttendance{Enabled: true, Tracks: true, Active: true, OpenSince: since(StaleOpenAfter)}, true, true, false, false},
		{"entrada de 16 h y un minuto ya venció (B2)", CashAttendance{Enabled: true, Tracks: true, Active: true, OpenSince: since(StaleOpenAfter + time.Minute)}, true, false, true, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			g := CashOpenAttendance(c.a, now)
			if g.Applies != c.applies || g.Ready != c.ready || g.Stale != c.stale {
				t.Fatalf("gate = %+v; quiere applies=%v ready=%v stale=%v", g, c.applies, c.ready, c.stale)
			}
			if !g.IsSelf {
				t.Fatal("al abrir la persona es quien está en sesión: isSelf tiene que ser true")
			}
			if g.CanAuthorize {
				t.Fatal("abrir no tiene camino autorizado: canAuthorize tiene que ser false")
			}
			err := RequireCashOpenAttendance(c.a, now)
			if c.isErr != (err != nil) {
				t.Fatalf("RequireCashOpenAttendance = %v; quiere error=%v", err, c.isErr)
			}
			if err != nil && (!errors.Is(err, ErrCashClockInRequired) || !errors.Is(err, ErrConflict)) {
				t.Fatalf("error = %v; quiere ErrCashClockInRequired envuelto de ErrConflict", err)
			}
			// El mismo predicado: lo que la hoja muestra es lo que el servidor exige (corolario del III).
			if g.Ready != (err == nil) {
				t.Fatalf("la hoja dice ready=%v y abrir responde %v", g.Ready, err)
			}
		})
	}
}

// Cerrar caja exige que quien la abrió no tenga ninguna entrada abierta, salvo un cierre autorizado.
// El orden de las reglas importa: el permiso se revisa antes que la antigüedad, para que quien no
// lo tiene no aprenda nada de la entrada de otra persona (B9).
func TestCashCloseAttendance(t *testing.T) {
	now := at("2026-10-30 22:00")
	since := func(d time.Duration) *time.Time { v := now.Add(-d); return &v }
	reason := func(s string) *string { return &s }
	const ana, gina = int64(12), int64(20)
	gerente := []Permission{PermCashCloseWithoutClockOut}
	inside := CashAttendance{Enabled: true, Tracks: true, Active: true, OpenSince: since(7 * time.Hour)}
	old := CashAttendance{Enabled: true, Tracks: true, Active: true, OpenSince: since(StaleOpenAfter + time.Minute)}
	exact := CashAttendance{Enabled: true, Tracks: true, Active: true, OpenSince: since(StaleOpenAfter)}

	cases := []struct {
		name string
		in   CashCloseInput
		// gate
		applies, ready, stale, isSelf, canAuthorize bool
		// decisión
		authorized bool
		err        error
	}{
		{name: "asistencia apagada", in: CashCloseInput{Opener: CashAttendance{Tracks: true, OpenSince: since(time.Hour)}, OpenerID: ana, CloserID: gina},
			ready: true},
		{name: "no lleva asistencia", in: CashCloseInput{Opener: CashAttendance{Enabled: true, OpenSince: since(time.Hour)}, OpenerID: ana, CloserID: gina},
			ready: true},
		{name: "ya marcó su salida", in: CashCloseInput{Opener: CashAttendance{Enabled: true, Tracks: true, Active: true}, OpenerID: ana, CloserID: gina},
			applies: true, ready: true},
		{name: "sigue dentro y nadie autoriza", in: CashCloseInput{Opener: inside, OpenerID: ana, CloserID: gina, OpenerName: "Ana", Perms: gerente},
			applies: true, canAuthorize: true, err: ErrCashOpenerClockedIn},
		{name: "entrada de más de 16 h también bloquea (B3)", in: CashCloseInput{Opener: old, OpenerID: ana, CloserID: gina, OpenerName: "Ana"},
			applies: true, stale: true, err: ErrCashOpenerClockedIn},
		{name: "autorizado con permiso y motivo", in: CashCloseInput{Opener: inside, OpenerID: ana, CloserID: gina, Perms: gerente, AuthorizedReason: reason("Se fue sin checar")},
			applies: true, canAuthorize: true, authorized: true},
		{name: "autorizado sin permiso, con entrada ajena vieja: 403 antes de mirar la antigüedad (B9)", in: CashCloseInput{Opener: old, OpenerID: ana, CloserID: gina, AuthorizedReason: reason("Se fue sin checar")},
			applies: true, stale: true, err: ErrForbidden},
		{name: "cierre propio con entrada de 16 h o menos: con PIN (B10)", in: CashCloseInput{Opener: exact, OpenerID: gina, CloserID: gina, Perms: gerente, AuthorizedReason: reason("Se fue sin checar")},
			applies: true, isSelf: true, err: ErrCashClockOutWithPIN},
		{name: "cierre propio con entrada de más de 16 h: permitido (B10)", in: CashCloseInput{Opener: old, OpenerID: gina, CloserID: gina, Perms: gerente, AuthorizedReason: reason("Se fue sin checar")},
			applies: true, stale: true, isSelf: true, canAuthorize: true, authorized: true},
		{name: "motivo de puros espacios (B11)", in: CashCloseInput{Opener: inside, OpenerID: ana, CloserID: gina, Perms: gerente, AuthorizedReason: reason("    ")},
			applies: true, canAuthorize: true, err: ErrValidation},
		{name: "motivo de 201 caracteres (B11)", in: CashCloseInput{Opener: inside, OpenerID: ana, CloserID: gina, Perms: gerente, AuthorizedReason: reason(strings.Repeat("a", 201))},
			applies: true, canAuthorize: true, err: ErrValidation},
		{name: "autorización cuando ya no hace falta se ignora (B8)", in: CashCloseInput{Opener: CashAttendance{Enabled: true, Tracks: true, Active: true}, OpenerID: ana, CloserID: gina, Perms: gerente, AuthorizedReason: reason("Se fue sin checar")},
			applies: true, ready: true},
		{name: "autorización con la regla apagada se ignora (B8)", in: CashCloseInput{Opener: CashAttendance{Tracks: true, OpenSince: since(time.Hour)}, OpenerID: ana, CloserID: gina, AuthorizedReason: reason("x")},
			ready: true},
		{name: "dada de baja con entrada abierta: sigue pidiendo salida (B7)", in: CashCloseInput{Opener: CashAttendance{Enabled: true, Tracks: true, OpenSince: since(time.Hour)}, OpenerID: ana, CloserID: gina, OpenerName: "Ana", Perms: gerente},
			applies: true, canAuthorize: true, err: ErrCashOpenerClockedIn},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			g := CashCloseGate(c.in, now)
			if g.Applies != c.applies || g.Ready != c.ready || g.Stale != c.stale || g.IsSelf != c.isSelf || g.CanAuthorize != c.canAuthorize {
				t.Fatalf("gate = %+v; quiere applies=%v ready=%v stale=%v isSelf=%v canAuthorize=%v",
					g, c.applies, c.ready, c.stale, c.isSelf, c.canAuthorize)
			}
			d, err := CashCloseAttendance(c.in, now)
			if c.err != nil {
				if !errors.Is(err, c.err) {
					t.Fatalf("error = %v; quiere %v", err, c.err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if d.Authorized != c.authorized {
				t.Fatalf("authorized = %v; quiere %v", d.Authorized, c.authorized)
			}
			if d.Authorized && d.Reason != strings.TrimSpace(*c.in.AuthorizedReason) {
				t.Fatalf("motivo = %q", d.Reason)
			}
			// Sin autorización, `ready` del GET es exactamente «cerrar no responde error».
			if c.in.AuthorizedReason == nil && !g.Ready {
				t.Fatal("ready=false y cerrar no rechazó")
			}
		})
	}
}

// El 409 nombra a quien abrió y llega a la pantalla sin el prefijo del sentinel: «Ana abrió esta
// caja…», no «conflicto: quien abrió…». Los otros dos son fijos.
func TestCashAttendanceErrorsSpeakToTheOperator(t *testing.T) {
	err := CashOpenerClockedIn{Name: "Ana"}
	if err.Error() != "Ana abrió esta caja y no ha marcado su salida" {
		t.Fatalf("mensaje = %q", err.Error())
	}
	if !errors.Is(err, ErrCashOpenerClockedIn) || !errors.Is(err, ErrConflict) {
		t.Fatal("tiene que seguir siendo ErrCashOpenerClockedIn y ErrConflict para el 409")
	}
	if !strings.HasSuffix(ErrCashClockInRequired.Error(), "Para abrir la caja, primero marca tu entrada") {
		t.Fatalf("mensaje = %q", ErrCashClockInRequired.Error())
	}
	if !strings.HasSuffix(ErrCashClockOutWithPIN.Error(), "Marca tu salida con tu PIN para cerrar la caja") {
		t.Fatalf("mensaje = %q", ErrCashClockOutWithPIN.Error())
	}
	_, err2 := CashCloseAttendance(CashCloseInput{Opener: CashAttendance{Enabled: true, Tracks: true,
		OpenSince: new(at("2026-10-30 09:00"))}, OpenerID: 1, CloserID: 2, OpenerName: "Ana"}, at("2026-10-30 10:00"))
	if err2 == nil || err2.Error() != "Ana abrió esta caja y no ha marcado su salida" {
		t.Fatalf("CashCloseAttendance = %v", err2)
	}
}
