package domain

import (
	"errors"
	"testing"
	"time"
)

func dia(s string) time.Time {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}
	return t
}

// Sin fechas, la pantalla pide los últimos 30 días contando hoy: 30 y no 31, porque «Últimos 30
// días» que trae 31 reporta un día de ventas de más sin que nadie lo note.
func TestDefaultHealthRangeIsThirtyDaysIncludingToday(t *testing.T) {
	r := DefaultHealthRange(dia("2026-10-13"))
	if !r.From.Equal(dia("2026-09-14")) || !r.To.Equal(dia("2026-10-13")) {
		t.Fatalf("DefaultHealthRange = %s..%s, quiere 2026-09-14..2026-10-13",
			r.From.Format("2006-01-02"), r.To.Format("2006-01-02"))
	}
}

// Cada caso es una forma en que la pantalla contestaría algo que nadie pidió si no se rechazara.
func TestHealthFilterValidate(t *testing.T) {
	ok := HealthFilter{Range: Range{From: dia("2026-10-05"), To: dia("2026-10-13")}, Limit: HealthPageSize}
	since := dia("2026-09-01")
	cases := []struct {
		name string
		mod  func(*HealthFilter)
		ok   bool
	}{
		{"el filtro de la pantalla", func(*HealthFilter) {}, true},
		{"un solo día", func(f *HealthFilter) { f.Range.From = f.Range.To }, true},
		{"una sección conocida con su página", func(f *HealthFilter) { f.Section = SectionUnsold; f.Offset = 25 }, true},
		{"sin compras desde una fecha", func(f *HealthFilter) { f.Section = SectionNoPurchases; f.NoPurchasesSince = &since }, true},
		{"rango invertido", func(f *HealthFilter) { f.Range.From, f.Range.To = f.Range.To, f.Range.From }, false},
		{"más de 366 días", func(f *HealthFilter) { f.Range.From = dia("2025-10-12") }, false},
		{"366 días justos", func(f *HealthFilter) { f.Range.From = dia("2025-10-13") }, true},
		{"sección inventada", func(f *HealthFilter) { f.Section = "todo" }, false},
		{"página de cero", func(f *HealthFilter) { f.Limit = 0 }, false},
		{"página enorme", func(f *HealthFilter) { f.Limit = MaxHealthPage + 1 }, false},
		{"desplazamiento negativo", func(f *HealthFilter) { f.Offset = -1 }, false},
		{"fecha sin año", func(f *HealthFilter) { f.Range.From = time.Time{} }, false},
		{"sin compras desde una fecha vacía", func(f *HealthFilter) { d := time.Time{}; f.NoPurchasesSince = &d }, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := ok
			c.mod(&f)
			err := f.Validate()
			if c.ok && err != nil {
				t.Fatalf("Validate() = %v, quiere nil", err)
			}
			if !c.ok && !errors.Is(err, ErrValidation) {
				t.Fatalf("Validate() = %v, quiere ErrValidation: la pantalla contestaría algo que nadie pidió", err)
			}
		})
	}
}
