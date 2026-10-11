package domain

import (
	"errors"
	"strings"
	"testing"
	"time"
)

var cdmx = LoadBusinessLocation("America/Mexico_City")

func at(s string) time.Time {
	t, err := time.ParseInLocation("2006-01-02 15:04", s, cdmx)
	if err != nil {
		panic(err)
	}
	return t
}

// El servidor decide si es entrada o salida; la pantalla manda lo que ESPERA. Si otra tableta ya
// checó a esa persona, lo esperado ya no corresponde y se rechaza en vez de registrar lo contrario
// de lo que dice el botón.
func TestNextClockDecidesInOrOutAndRejectsAStaleScreen(t *testing.T) {
	open := at("2026-10-10 09:02")
	cases := []struct {
		name       string
		open       *time.Time
		now        time.Time
		expected   ClockKind
		want       ClockKind
		closeStale bool
		err        bool
	}{
		{"sin entrada: entra", nil, at("2026-10-10 09:00"), ClockIn, ClockIn, false, false},
		{"con entrada: sale", &open, at("2026-10-10 17:00"), ClockOut, ClockOut, false, false},
		{"la pantalla cree que no ha entrado", &open, at("2026-10-10 17:00"), ClockIn, "", false, true},
		{"la pantalla cree que ya entró", nil, at("2026-10-10 09:00"), ClockOut, "", false, true},
		{"entrada de hace 16 h o menos: sale", &open, at("2026-10-11 01:02"), ClockOut, ClockOut, false, false},
		// Olvidó la salida de ayer: no se le suman 24 horas. La vieja queda «sin salida» para que la
		// corrija el gerente, y hoy entra de nuevo.
		{"entrada de hace más de 16 h: la cierra sin salida y entra", &open, at("2026-10-11 09:00"), ClockIn, ClockIn, true, false},
		{"entrada vieja y la pantalla pide salida", &open, at("2026-10-11 09:00"), ClockOut, "", false, true},
		{"lo esperado vacío", nil, at("2026-10-10 09:00"), "", "", false, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			d, err := NextClock(c.open, c.now, c.expected)
			if c.err {
				if err == nil {
					t.Fatalf("aceptó %+v", d)
				}
				if c.expected != "" && !errors.Is(err, ErrAttendanceStateChanged) {
					t.Fatalf("error = %v, quiere ErrAttendanceStateChanged", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if d.Kind != c.want || d.CloseStale != c.closeStale {
				t.Fatalf("= %+v, quiere %s closeStale=%v", d, c.want, c.closeStale)
			}
		})
	}
}

// El turno cuenta al día de la ENTRADA: una salida a la 1:00 no crea un día nuevo.
func TestBusinessDateOfIsTheDayOfTheEntry(t *testing.T) {
	cases := map[string]string{
		"2026-10-10 23:50": "2026-10-10",
		"2026-10-11 00:10": "2026-10-11",
		"2026-10-10 09:00": "2026-10-10",
	}
	for in, want := range cases {
		if got := BusinessDateOf(at(in), cdmx).Format("2006-01-02"); got != want {
			t.Errorf("BusinessDateOf(%s) = %s, quiere %s", in, got, want)
		}
	}
	// Un instante en UTC se lleva a la zona del local antes de decidir el día.
	utc := time.Date(2026, 10, 11, 3, 0, 0, 0, time.UTC) // 21:00 del 10 en Ciudad de México
	if got := BusinessDateOf(utc, cdmx).Format("2006-01-02"); got != "2026-10-10" {
		t.Errorf("BusinessDateOf(03:00 UTC) = %s, quiere 2026-10-10", got)
	}
}

func hm(h, m int) int { return h*60 + m }

func iptr(v int) *int { return &v }

func timePtr(v time.Time) *time.Time { return &v }

// El banco de horas, día por día. Cada caso es un borde que ya se discutió en el spec.
func TestAttendanceDay(t *testing.T) {
	monday := time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC) // un lunes
	nineToFive := ScheduleVersion{EffectiveFrom: monday.AddDate(0, 0, -30),
		Days: map[time.Weekday][]Shift{time.Monday: {{Start: hm(9, 0), End: hm(17, 0)}}, time.Saturday: {{Start: hm(9, 0), End: hm(17, 0)}}}}
	noSaturday := ScheduleVersion{EffectiveFrom: monday.AddDate(0, 0, -1),
		Days: map[time.Weekday][]Shift{time.Monday: {{Start: hm(9, 0), End: hm(17, 0)}}}}
	nightShift := ScheduleVersion{EffectiveFrom: monday.AddDate(0, 0, -30),
		Days: map[time.Weekday][]Shift{time.Monday: {{Start: hm(22, 0), End: hm(6, 0)}}}}
	measured := AttendanceSettings{Measure: true}
	withGrace := AttendanceSettings{Measure: true, GraceIn: 10, GraceOut: 10}
	entryOf := func(in, out string) DayEntry {
		e := DayEntry{In: at(in), Status: "closed"}
		if out != "" {
			e.Out = timePtr(at(out))
		}
		return e
	}

	cases := []struct {
		name                   string
		date                   time.Time
		entries                []DayEntry
		versions               []ScheduleVersion
		settings               AttendanceSettings
		worked                 int
		late, overtime         *int
		missingOut, noSchedule bool
	}{
		{"medición apagada: solo horas", monday, []DayEntry{entryOf("2026-10-12 09:10", "2026-10-12 17:40")},
			[]ScheduleVersion{nineToFive}, AttendanceSettings{}, 510, nil, nil, false, false},
		{"sin tolerancia: 10 de retardo, 40 extra", monday, []DayEntry{entryOf("2026-10-12 09:10", "2026-10-12 17:40")},
			[]ScheduleVersion{nineToFive}, measured, 510, iptr(10), iptr(40), false, false},
		{"dentro de la tolerancia no cuenta", monday, []DayEntry{entryOf("2026-10-12 09:08", "2026-10-12 17:05")},
			[]ScheduleVersion{nineToFive}, withGrace, 477, iptr(0), iptr(0), false, false},
		{"pasando la tolerancia cuenta completo", monday, []DayEntry{entryOf("2026-10-12 09:12", "2026-10-12 17:15")},
			[]ScheduleVersion{nineToFive}, withGrace, 483, iptr(12), iptr(15), false, false},
		{"salir antes no descuenta", monday, []DayEntry{entryOf("2026-10-12 09:00", "2026-10-12 15:00")},
			[]ScheduleVersion{nineToFive}, measured, 360, iptr(0), iptr(0), false, false},
		{"sin horario capturado", monday, []DayEntry{entryOf("2026-10-12 09:00", "2026-10-12 17:00")},
			nil, measured, 480, nil, nil, false, true},
		{"sin salida no suma", monday, []DayEntry{{In: at("2026-10-12 09:00"), Status: "missing_out"}},
			[]ScheduleVersion{nineToFive}, measured, 0, nil, nil, true, false},
		{"turno nocturno que cruza la medianoche", monday, []DayEntry{entryOf("2026-10-12 22:05", "2026-10-13 06:20")},
			[]ScheduleVersion{nightShift}, measured, 495, iptr(5), iptr(20), false, false},
		{"dos tramos el mismo día", monday, []DayEntry{entryOf("2026-10-12 09:00", "2026-10-12 13:00"), entryOf("2026-10-12 16:00", "2026-10-12 20:00")},
			[]ScheduleVersion{nineToFive}, measured, 480, iptr(0), iptr(180), false, false},
		// La versión nueva quitó el sábado: ese día ya no tiene horario, y lo trabajado es extra.
		{"versión que quitó el sábado", monday.AddDate(0, 0, 5), []DayEntry{entryOf("2026-10-17 09:30", "2026-10-17 12:30")},
			[]ScheduleVersion{nineToFive, noSaturday}, measured, 180, iptr(0), iptr(180), false, false},
		{"con la versión vieja el sábado sí tenía horario", monday.AddDate(0, 0, -2), []DayEntry{entryOf("2026-10-10 09:30", "2026-10-10 17:00")},
			[]ScheduleVersion{nineToFive, noSaturday}, measured, 450, iptr(30), iptr(0), false, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := AttendanceDay(c.date, c.entries, c.versions, c.settings, cdmx)
			if got.WorkedMinutes != c.worked || got.MissingOut != c.missingOut || got.NoSchedule != c.noSchedule {
				t.Fatalf("= %+v; quiere trabajado %d, sinSalida %v, sinHorario %v", got, c.worked, c.missingOut, c.noSchedule)
			}
			if !eqPtr(got.LateMinutes, c.late) || !eqPtr(got.OvertimeMinutes, c.overtime) {
				t.Fatalf("retardo %v extra %v; quiere %v y %v", deref(got.LateMinutes), deref(got.OvertimeMinutes), deref(c.late), deref(c.overtime))
			}
		})
	}
}

func eqPtr(a, b *int) bool { return (a == nil && b == nil) || (a != nil && b != nil && *a == *b) }

func deref(p *int) any {
	if p == nil {
		return nil
	}
	return *p
}

func TestValidateSchedule(t *testing.T) {
	ok := []ScheduleDay{{Weekday: 1, ShiftNo: 1, Start: hm(9, 0), End: hm(17, 0)}, {Weekday: 5, ShiftNo: 2, Start: hm(22, 0), End: hm(6, 0)}}
	if err := ValidateSchedule(ok); err != nil {
		t.Fatalf("un horario válido se rechazó: %v", err)
	}
	for name, bad := range map[string][]ScheduleDay{
		"día 7":            {{Weekday: 7, ShiftNo: 1, Start: hm(9, 0), End: hm(17, 0)}},
		"misma hora":       {{Weekday: 1, ShiftNo: 1, Start: hm(9, 0), End: hm(9, 0)}},
		"turno 3":          {{Weekday: 1, ShiftNo: 3, Start: hm(9, 0), End: hm(10, 0)}},
		"hora fuera":       {{Weekday: 1, ShiftNo: 1, Start: hm(24, 0), End: hm(10, 0)}},
		"turno repetido":   {{Weekday: 1, ShiftNo: 1, Start: hm(9, 0), End: hm(10, 0)}, {Weekday: 1, ShiftNo: 1, Start: hm(11, 0), End: hm(12, 0)}},
		"no en 15 minutos": {{Weekday: 1, ShiftNo: 1, Start: hm(9, 7), End: hm(10, 0)}},
	} {
		if err := ValidateSchedule(bad); !errors.Is(err, ErrValidation) {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestValidateCorrectionReason(t *testing.T) {
	for reason, ok := range map[string]bool{"": false, "  ": false, "ok": false, "Olvidó checar": true, strings.Repeat("x", 200): true, strings.Repeat("x", 201): false} {
		if _, err := ValidateCorrectionReason(reason); (err == nil) != ok {
			t.Errorf("%q: %v", reason, err)
		}
	}
	if r, _ := ValidateCorrectionReason("  Olvidó checar  "); r != "Olvidó checar" {
		t.Errorf("no recortó el motivo: %q", r)
	}
}

// Corregir lo propio es un permiso aparte: una corrección propia mueve horas pagadas sin que nadie
// más la vea.
func TestCanCorrect(t *testing.T) {
	others := []Permission{PermAttendanceCorrectOthers}
	both := []Permission{PermAttendanceCorrectOthers, PermAttendanceCorrectOwn}
	cases := []struct {
		name           string
		actor, subject int64
		perms          []Permission
		ok             bool
	}{
		{"gerente sobre otro", 1, 2, others, true},
		{"gerente sobre sí", 1, 1, others, false},
		{"admin sobre sí", 1, 1, both, true},
		{"sin permiso", 1, 2, nil, false},
		{"solo lo propio, sobre otro", 1, 2, []Permission{PermAttendanceCorrectOwn}, false},
	}
	for _, c := range cases {
		if err := CanCorrect(c.actor, c.subject, c.perms); (err == nil) != c.ok {
			t.Errorf("%s: %v", c.name, err)
		}
		if err := CanCorrect(c.actor, c.subject, c.perms); err != nil && !errors.Is(err, ErrForbidden) {
			t.Errorf("%s: no es ErrForbidden: %v", c.name, err)
		}
	}
}

// Un registro capturado o corregido no se encima con otro de la misma persona; una entrada abierta
// cuenta como si no terminara.
func TestOverlaps(t *testing.T) {
	nine, five := at("2026-10-12 09:00"), at("2026-10-12 17:00")
	existingEntries := []Interval{{ID: 1, In: nine, Out: &five}}
	if !Overlaps(existingEntries, 0, at("2026-10-12 16:00"), timePtr(at("2026-10-12 18:00"))) {
		t.Error("16–18 no se encima con 9–17")
	}
	if Overlaps(existingEntries, 0, five, timePtr(at("2026-10-12 20:00"))) {
		t.Error("17–20 empieza cuando termina el otro: no se encima")
	}
	if Overlaps(existingEntries, 1, at("2026-10-12 08:00"), &five) {
		t.Error("corregir el mismo registro no se encima consigo mismo")
	}
	openAt := []Interval{{ID: 2, In: at("2026-10-12 20:00")}}
	if !Overlaps(openAt, 0, at("2026-10-13 01:00"), timePtr(at("2026-10-13 02:00"))) {
		t.Error("una entrada abierta cubre todo lo que sigue")
	}
}
