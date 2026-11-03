package domain

import (
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

// Errores de asistencia (spec 033).
var (
	// ErrAttendanceDisabled: el negocio no tiene la asistencia activa.
	ErrAttendanceDisabled = errors.New("la asistencia no está activa en este negocio")
	// ErrAttendanceStateChanged: la pantalla esperaba registrar entrada y ya hay una abierta, o al
	// revés. Pasa cuando otra tableta checó a la misma persona; registrar lo contrario de lo que
	// dice el botón sería peor que pedir que vuelva a intentar.
	ErrAttendanceStateChanged = fmt.Errorf("%w: el registro de esta persona cambió, vuelve a intentarlo", ErrConflict)
	// ErrAttendanceNotTracked: la persona no lleva asistencia («Lleva asistencia» apagada).
	ErrAttendanceNotTracked = fmt.Errorf("%w: a esta persona no se le registra asistencia", ErrValidation)
)

// ClockKind es entrada o salida.
type ClockKind string

const (
	ClockIn  ClockKind = "in"
	ClockOut ClockKind = "out"
)

// StaleOpenAfter es cuánto puede durar una entrada abierta antes de darla por «sin salida». Sin
// este tope, olvidar checar la salida le suma a alguien veinte horas al checar al día siguiente.
const StaleOpenAfter = 16 * time.Hour

// IsStaleOpen dice si una entrada abierta ya no puede ser el turno de hoy.
func IsStaleOpen(openSince, now time.Time) bool {
	return now.Sub(openSince) > StaleOpenAfter
}

// ClockDecision es lo que hay que hacer al checar.
type ClockDecision struct {
	Kind ClockKind
	// CloseStale: hay una entrada abierta vieja que queda «sin salida» antes de abrir la nueva.
	CloseStale bool
}

// NextClock decide si checar es entrada o salida. Lo decide el servidor; `expected` es lo que
// mostraba la pantalla, y si ya no corresponde se rechaza con ErrAttendanceStateChanged.
func NextClock(openSince *time.Time, now time.Time, expected ClockKind) (ClockDecision, error) {
	d := ClockDecision{Kind: ClockIn}
	if openSince != nil {
		if IsStaleOpen(*openSince, now) {
			d.CloseStale = true
		} else {
			d.Kind = ClockOut
		}
	}
	if expected != ClockIn && expected != ClockOut {
		return ClockDecision{}, fmt.Errorf("%w: falta decir si es entrada o salida", ErrValidation)
	}
	if expected != d.Kind {
		return ClockDecision{}, ErrAttendanceStateChanged
	}
	return d, nil
}

// BusinessDateOf es el día de calendario de un instante en la zona del local. Se guarda con la
// entrada: así un turno que cruza la medianoche cuenta al día en que empezó, y cambiar la zona
// después no mueve de día turnos ya cerrados.
func BusinessDateOf(t time.Time, loc *time.Location) time.Time {
	l := t.In(loc)
	return time.Date(l.Year(), l.Month(), l.Day(), 0, 0, 0, 0, time.UTC)
}

// Shift es un turno de un día, en minutos desde la medianoche. End < Start termina al día siguiente.
type Shift struct{ Start, End int }

// ScheduleVersion es un horario semanal completo, vigente desde una fecha. Un día sin turnos en la
// versión vigente es libre: guardar día por día con vigencia propia «resucitaba» el sábado que se
// dejó de trabajar.
type ScheduleVersion struct {
	EffectiveFrom time.Time
	Days          map[time.Weekday][]Shift
}

// ScheduleDay es un turno como se captura.
type ScheduleDay struct{ Weekday, ShiftNo, Start, End int }

// MaxShiftsPerDay es cuántos turnos se capturan por día: dos, el horario partido.
const MaxShiftsPerDay = 2

// ValidateSchedule revisa un horario capturado: días 0–6, hasta dos turnos por día, horas de 15 en
// 15 minutos dentro del día, y un turno no empieza y termina a la misma hora.
func ValidateSchedule(days []ScheduleDay) error {
	seen := map[[2]int]bool{}
	for _, d := range days {
		switch {
		case d.Weekday < 0 || d.Weekday > 6:
			return fmt.Errorf("%w: día de la semana inválido", ErrValidation)
		case d.ShiftNo < 1 || d.ShiftNo > MaxShiftsPerDay:
			return fmt.Errorf("%w: un día lleva a lo más dos turnos", ErrValidation)
		case d.Start < 0 || d.Start >= 24*60 || d.End < 0 || d.End >= 24*60 || d.Start%15 != 0 || d.End%15 != 0:
			return fmt.Errorf("%w: las horas van de 15 en 15 minutos", ErrValidation)
		case d.Start == d.End:
			return fmt.Errorf("%w: un turno no puede empezar y terminar a la misma hora", ErrValidation)
		case seen[[2]int{d.Weekday, d.ShiftNo}]:
			return fmt.Errorf("%w: turno repetido", ErrValidation)
		}
		seen[[2]int{d.Weekday, d.ShiftNo}] = true
	}
	return nil
}

// AttendanceSettings es cómo mide asistencia un negocio.
type AttendanceSettings struct {
	Measure           bool
	GraceIn, GraceOut int
}

// DayEntry es una entrada del día para el cálculo.
type DayEntry struct {
	In     time.Time
	Out    *time.Time
	Status string
}

// DaySummary es el día de una persona en el reporte. Retardo y extra solo existen con la medición
// encendida y un horario: sin eso son nil, nunca un cero inventado.
type DaySummary struct {
	Date            time.Time
	WorkedMinutes   int
	LateMinutes     *int
	OvertimeMinutes *int
	MissingOut      bool
	NoSchedule      bool
}

// versionFor es la versión de horario vigente en `date`: la de mayor vigencia que no sea posterior.
func versionFor(date time.Time, versions []ScheduleVersion) *ScheduleVersion {
	var best *ScheduleVersion
	for i := range versions {
		v := &versions[i]
		if v.EffectiveFrom.After(date) {
			continue
		}
		if best == nil || v.EffectiveFrom.After(best.EffectiveFrom) {
			best = v
		}
	}
	return best
}

// AttendanceDay calcula el día de una persona. Lo trabajado es la suma de las entradas cerradas;
// una sin salida no suma. Con la medición encendida, el retardo se mide entre la primera entrada y
// el inicio del primer turno, y el extra entre la última salida y el fin del último; pasando la
// tolerancia se cuentan completos. Salir antes no descuenta. En un día libre de la versión vigente,
// todo lo trabajado es extra.
func AttendanceDay(date time.Time, entries []DayEntry, versions []ScheduleVersion, s AttendanceSettings, loc *time.Location) DaySummary {
	out := DaySummary{Date: date}
	var closed []DayEntry
	for _, e := range entries {
		switch {
		case e.Status == "missing_out":
			out.MissingOut = true
		case e.Out != nil:
			closed = append(closed, e)
			out.WorkedMinutes += int(e.Out.Sub(e.In).Minutes())
		}
	}
	if !s.Measure {
		return out
	}
	v := versionFor(date, versions)
	if v == nil {
		out.NoSchedule = true
		return out
	}
	if len(closed) == 0 {
		return out
	}
	sort.Slice(closed, func(i, j int) bool { return closed[i].In.Before(closed[j].In) })
	shifts := v.Days[date.Weekday()]
	if len(shifts) == 0 {
		zero, all := 0, out.WorkedMinutes
		out.LateMinutes, out.OvertimeMinutes = &zero, &all
		return out
	}
	midnight := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, loc)
	start := midnight.Add(time.Duration(shifts[0].Start) * time.Minute)
	last := shifts[len(shifts)-1]
	end := midnight.Add(time.Duration(last.End) * time.Minute)
	if last.End <= last.Start {
		end = end.AddDate(0, 0, 1)
	}
	late := pastGrace(int(closed[0].In.Sub(start).Minutes()), s.GraceIn)
	lastOut := *closed[0].Out
	for _, e := range closed {
		if e.Out.After(lastOut) {
			lastOut = *e.Out
		}
	}
	extra := pastGrace(int(lastOut.Sub(end).Minutes()), s.GraceOut)
	out.LateMinutes, out.OvertimeMinutes = &late, &extra
	return out
}

// pastGrace devuelve los minutos completos si pasan la tolerancia, o cero.
func pastGrace(minutes, grace int) int {
	if minutes <= grace || minutes <= 0 {
		return 0
	}
	return minutes
}

// ValidateCorrectionReason exige el motivo de una corrección o captura a mano: de 3 a 200
// caracteres. Sin motivo, la corrección es un número que cambió sin explicación.
func ValidateCorrectionReason(reason string) (string, error) {
	r := strings.TrimSpace(reason)
	if n := utf8.RuneCountInString(r); n < 3 || n > 200 {
		return "", fmt.Errorf("%w: escribe el motivo de la corrección", ErrValidation)
	}
	return r, nil
}

// CanCorrectAny dice si `actor` tiene algún permiso de corrección. Se pregunta ANTES de leer el
// registro: sin esto, un 404 o un 403 según exista el id sirve para enumerar registros.
func CanCorrectAny(perms []Permission) error {
	if slices.Contains(perms, PermAttendanceCorrectOthers) || slices.Contains(perms, PermAttendanceCorrectOwn) {
		return nil
	}
	return fmt.Errorf("%w: %s", ErrForbidden, PermissionDeniedMessage(PermAttendanceCorrectOthers))
}

// CanCorrect decide si `actor` puede corregir la asistencia de `subject`. Lo propio pide su propio
// permiso.
func CanCorrect(actor, subject int64, perms []Permission) error {
	need := PermAttendanceCorrectOthers
	if actor == subject {
		need = PermAttendanceCorrectOwn
	}
	if !slices.Contains(perms, need) {
		return fmt.Errorf("%w: %s", ErrForbidden, PermissionDeniedMessage(need))
	}
	return nil
}

// Interval es un registro de asistencia para revisar solapes.
type Interval struct {
	ID  int64
	In  time.Time
	Out *time.Time
}

// ErrAttendanceOverlap: el registro se encima con otro de la misma persona.
var ErrAttendanceOverlap = fmt.Errorf("%w: ese horario se encima con otro registro de esta persona", ErrConflict)

// Overlaps dice si [in, out) se encima con algún registro de `existing` distinto de `skipID`. Un
// registro abierto (sin salida) cuenta como si no terminara.
func Overlaps(existing []Interval, skipID int64, in time.Time, out *time.Time) bool {
	for _, e := range existing {
		if e.ID == skipID && skipID != 0 {
			continue
		}
		startsBeforeOtherEnds := e.Out == nil || in.Before(*e.Out)
		endsAfterOtherStarts := out == nil || out.After(e.In)
		if startsBeforeOtherEnds && endsAfterOtherStarts {
			return true
		}
	}
	return false
}
