package domain

import (
	"errors"
	"testing"
	"time"
)

func TestIsPending(t *testing.T) {
	ahora := time.Date(2026, 11, 23, 19, 0, 0, 0, time.UTC)
	futuro, pasado, justo := ahora.Add(time.Minute), ahora.Add(-time.Minute), ahora
	cases := []struct {
		name   string
		state  string
		plazo  *time.Time
		espera bool
	}{
		{"pendiente con plazo futuro", IncomingStatePending, &futuro, true},
		{"pendiente sin plazo (un origen que no lo tiene)", IncomingStatePending, nil, true},
		// El defecto que esto atrapa: la plataforma ya canceló sola y la tableta seguiría sonando.
		{"pendiente vencido", IncomingStatePending, &pasado, false},
		{"pendiente justo en el plazo ya no se decide", IncomingStatePending, &justo, false},
		{"aceptado", "aceptado", &futuro, false},
		{"rechazado", "rechazado", &futuro, false},
		{"cancelado", "cancelado", nil, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := IsPending(c.state, c.plazo, ahora); got != c.espera {
				t.Fatalf("IsPending = %v, se esperaba %v", got, c.espera)
			}
		})
	}
}

func TestSortByUrgency(t *testing.T) {
	base := time.Date(2026, 11, 23, 19, 0, 0, 0, time.UTC)
	at := func(m int) *time.Time { v := base.Add(time.Duration(m) * time.Minute); return &v }
	in := []IncomingOrder{
		{ID: 1, Source: IncomingSourcePlatform, PlacedAt: at(-1)}, // sin plazo, más nuevo
		{ID: 2, Source: IncomingSourcePlatform, DecideBefore: at(10), PlacedAt: at(-2)},
		{ID: 3, Source: IncomingSourcePlatform, PlacedAt: at(-5)}, // sin plazo, más viejo
		{ID: 4, Source: IncomingSourcePlatform, DecideBefore: at(3), PlacedAt: at(-8)},
		{ID: 5, Source: IncomingSourcePlatform, DecideBefore: at(3), PlacedAt: at(-8)}, // empate: decide el id
	}
	SortByUrgency(in)
	got := []int64{}
	for _, o := range in {
		got = append(got, o.ID)
	}
	want := []int64{4, 5, 2, 3, 1}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("orden = %v; se esperaba %v (con plazo el más cercano primero, sin plazo el más viejo)", got, want)
		}
	}
}

func TestParseIncomingSource(t *testing.T) {
	if s, err := ParseIncomingSource("platform"); err != nil || s != IncomingSourcePlatform {
		t.Fatalf("platform: %v %v", s, err)
	}
	for _, raw := range []string{"", "web", "PLATFORM", "uber"} {
		if _, err := ParseIncomingSource(raw); !errors.Is(err, ErrValidation) {
			t.Fatalf("%q debía rechazarse como ErrValidation, dio %v", raw, err)
		}
	}
}

func TestParseIncomingRejectReason(t *testing.T) {
	plataforma := map[string]string{
		"closed": "STORE_CLOSED", "out_of_stock": "ITEM_AVAILABILITY", "too_busy": "CAPACITY",
		"address": "ADDRESS", "other": "OTHER",
	}
	for code, want := range plataforma {
		r, err := ParseIncomingRejectReason(code)
		if err != nil {
			t.Fatalf("%s: %v", code, err)
		}
		if got := r.PlatformCode(); got != want || !MotivoDeRechazoValido(got) {
			t.Fatalf("%s se traduce a %q; se esperaba %q y que la plataforma lo admita", code, got, want)
		}
		if r.Label() == "" {
			t.Fatalf("%s no tiene texto para quien opera", code)
		}
	}
	// Nunca cae a «other»: mandaría al cliente una explicación que nadie eligió.
	for _, raw := range []string{"", "OTHER", "STORE_CLOSED", "cerrado"} {
		if _, err := ParseIncomingRejectReason(raw); !errors.Is(err, ErrValidation) {
			t.Fatalf("%q debía rechazarse como ErrValidation, dio %v", raw, err)
		}
	}
}
