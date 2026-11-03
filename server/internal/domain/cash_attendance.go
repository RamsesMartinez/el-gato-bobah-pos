package domain

import (
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode/utf8"
)

// La caja exige la asistencia de quien la abre (spec 046). Las dos funciones de aquí son el ÚNICO
// predicado: el GET que pinta la hoja, abrir y cerrar las usan, así que lo que la pantalla muestra
// es lo que el servidor exige.

var (
	// ErrCashClockInRequired: quien abre no tiene una entrada vigente.
	ErrCashClockInRequired = fmt.Errorf("%w: Para abrir la caja, primero marca tu entrada", ErrConflict)
	// ErrCashOpenerClockedIn: quien abrió la caja sigue con su entrada abierta. Viaja como
	// CashOpenerClockedIn, que pone el nombre en el mensaje.
	ErrCashOpenerClockedIn = fmt.Errorf("%w: quien abrió esta caja no ha marcado su salida", ErrConflict)
	// ErrCashClockOutWithPIN: quien abrió quiere autorizarse su propio cierre con una entrada de hoy.
	// Si está frente a la tableta, su salida es un hecho que puede marcar; autorizarse dejaría sus
	// horas «sin salida» por gusto.
	ErrCashClockOutWithPIN = fmt.Errorf("%w: Marca tu salida con tu PIN para cerrar la caja", ErrConflict)
)

// CashOpenerClockedIn es ErrCashOpenerClockedIn con el nombre de quien abrió. Es un tipo y no un
// `fmt.Errorf("%w…")`: el mensaje sale tal cual en la respuesta y el sentinel lo prefijaría.
type CashOpenerClockedIn struct{ Name string }

func (e CashOpenerClockedIn) Error() string {
	if e.Name == "" {
		return "Quien abrió esta caja no ha marcado su salida"
	}
	return e.Name + " abrió esta caja y no ha marcado su salida"
}

func (e CashOpenerClockedIn) Unwrap() error { return ErrCashOpenerClockedIn }

// CashAttendance es lo que se sabe de la asistencia de una persona al abrir o cerrar caja.
type CashAttendance struct {
	// Enabled: la asistencia del negocio está encendida.
	Enabled bool
	// Tracks: la persona tiene «Lleva asistencia».
	Tracks bool
	// Active: la persona no fue dada de baja. No cambia la regla; solo decide si la hoja ofrece su
	// PIN, que checar rechaza para una persona inactiva.
	Active bool
	// OpenSince: su entrada abierta, si tiene.
	OpenSince *time.Time
}

func (a CashAttendance) applies() bool { return a.Enabled && a.Tracks }

// CashGate es lo que la hoja necesita para decidir qué ofrecer.
type CashGate struct {
	// Applies: la regla aplica a esta persona.
	Applies bool
	// Ready: se puede confirmar sin hacer nada más.
	Ready bool
	// Stale: la entrada abierta tiene más de 16 h.
	Stale bool
	// IsSelf: la persona es quien está en sesión.
	IsSelf bool
	// CanAuthorize: quien está en sesión puede cerrar sin la salida de quien abrió.
	CanAuthorize bool
}

// CashOpenAttendance dice si quien está en sesión puede abrir caja. Una entrada de más de 16 h no
// cuenta: checar la daría por «sin salida» y abriría otra, así que no es el turno de hoy.
func CashOpenAttendance(a CashAttendance, now time.Time) CashGate {
	g := CashGate{Applies: a.applies(), Ready: true, IsSelf: true}
	if !g.Applies {
		return g
	}
	g.Stale = a.OpenSince != nil && IsStaleOpen(*a.OpenSince, now)
	g.Ready = a.OpenSince != nil && !g.Stale
	return g
}

// RequireCashOpenAttendance es la puerta de la apertura: ErrCashClockInRequired si no está lista.
func RequireCashOpenAttendance(a CashAttendance, now time.Time) error {
	if !CashOpenAttendance(a, now).Ready {
		return ErrCashClockInRequired
	}
	return nil
}

// CashCloseInput es lo que se sabe al cerrar: quién abrió, cómo está su asistencia, quién cierra y
// si pidió el cierre autorizado.
type CashCloseInput struct {
	Opener     CashAttendance
	OpenerID   int64
	OpenerName string
	CloserID   int64
	Perms      []Permission
	// AuthorizedReason: el motivo del cierre autorizado; nil si no se pidió.
	AuthorizedReason *string
}

// CashCloseDecision es cómo se cierra. Authorized: la entrada de quien abrió queda «sin salida» y
// se registra quién lo autorizó y por qué.
type CashCloseDecision struct {
	Authorized bool
	Reason     string
}

// CashCloseGate es el estado del cierre para la hoja. Cualquier entrada abierta bloquea, también
// una de más de 16 h: el cierre no puede dejar a quien abrió con su turno sin terminar.
func CashCloseGate(in CashCloseInput, now time.Time) CashGate {
	g := CashGate{Applies: in.Opener.applies(), Ready: true, IsSelf: in.OpenerID == in.CloserID}
	if !g.Applies || in.Opener.OpenSince == nil {
		return g
	}
	g.Ready = false
	g.Stale = IsStaleOpen(*in.Opener.OpenSince, now)
	g.CanAuthorize = slices.Contains(in.Perms, PermCashCloseWithoutClockOut) && (!g.IsSelf || g.Stale)
	return g
}

// CashCloseAttendance decide si se puede cerrar. Sin necesidad, el cierre autorizado se ignora: no
// deja evento ni marca nada. El permiso se revisa antes que la regla del cierre propio: quien no lo
// tiene recibe el mismo 403 sea cual sea la antigüedad de la entrada, en vez de un 409 que sí
// depende de ella.
func CashCloseAttendance(in CashCloseInput, now time.Time) (CashCloseDecision, error) {
	g := CashCloseGate(in, now)
	if g.Ready {
		return CashCloseDecision{}, nil
	}
	if in.AuthorizedReason == nil {
		return CashCloseDecision{}, CashOpenerClockedIn{Name: in.OpenerName}
	}
	if !slices.Contains(in.Perms, PermCashCloseWithoutClockOut) {
		return CashCloseDecision{}, fmt.Errorf("%w: %s", ErrForbidden, PermissionDeniedMessage(PermCashCloseWithoutClockOut))
	}
	if g.IsSelf && !g.Stale {
		return CashCloseDecision{}, ErrCashClockOutWithPIN
	}
	r := strings.TrimSpace(*in.AuthorizedReason)
	if n := utf8.RuneCountInString(r); n < 3 || n > 200 {
		return CashCloseDecision{}, fmt.Errorf("%w: escribe por qué se cierra sin la salida", ErrValidation)
	}
	return CashCloseDecision{Authorized: true, Reason: r}, nil
}
