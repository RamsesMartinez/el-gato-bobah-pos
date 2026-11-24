package domain

import (
	"errors"
	"testing"
	"time"

	"github.com/shopspring/decimal"
)

func day(s string) time.Time {
	t, err := time.Parse(time.DateOnly, s)
	if err != nil {
		panic(err)
	}
	return t
}

func TestParseExpiry(t *testing.T) {
	today := day("2026-11-23")
	cases := []struct {
		in   string
		ok   bool
		want string
	}{
		{"2026-12-10", true, "2026-12-10"},
		{"2026-11-23", true, "2026-11-23"},
		// Lo que ya caducó se acepta: el conteo registra «esto ya caducó».
		{"2026-11-01", true, "2026-11-01"},
		{"2036-11-23", true, "2036-11-23"},
		{"2036-11-24", false, ""},
		{"1999-12-31", false, ""},
		{"", false, ""},
		{"10/12/2026", false, ""},
		{"2026-02-30", false, ""},
	}
	for _, c := range cases {
		got, err := ParseExpiry(c.in, today)
		if c.ok != (err == nil) {
			t.Errorf("ParseExpiry(%q) error = %v, quería ok=%v", c.in, err, c.ok)
			continue
		}
		if err != nil && !errors.Is(err, ErrValidation) {
			t.Errorf("ParseExpiry(%q) = %v, quería ErrValidation", c.in, err)
		}
		if c.ok && got.Format(time.DateOnly) != c.want {
			t.Errorf("ParseExpiry(%q) = %s", c.in, got.Format(time.DateOnly))
		}
	}
}

func TestValidLotBreakdown(t *testing.T) {
	lot := func(d, q string) LotQty { return LotQty{ExpiresOn: day(d), Qty: dec(q)} }
	cases := []struct {
		name    string
		counted string
		lots    []LotQty
		err     error
	}{
		{"sin desglose", "5", nil, nil},
		{"suma igual a lo contado", "5", []LotQty{lot("2026-12-10", "3"), lot("2026-11-20", "2")}, nil},
		{"suma menor: el resto es sin fecha", "5", []LotQty{lot("2026-12-10", "3")}, nil},
		{"suma mayor que lo contado", "5", []LotQty{lot("2026-12-10", "3"), lot("2026-12-11", "2.5")}, ErrLotsExceedCounted},
		{"contado en cero con fechas", "0", []LotQty{lot("2026-12-10", "1")}, ErrLotsExceedCounted},
		{"cantidad cero", "5", []LotQty{lot("2026-12-10", "0")}, ErrValidation},
		{"cantidad negativa", "5", []LotQty{lot("2026-12-10", "-1")}, ErrValidation},
		// Dos renglones con la misma fecha son el mismo lote: la pantalla los junta antes de mandar.
		{"fecha repetida", "5", []LotQty{lot("2026-12-10", "1"), lot("2026-12-10", "1")}, ErrValidation},
	}
	for _, c := range cases {
		err := ValidLotBreakdown(dec(c.counted), c.lots)
		if c.err == nil && err != nil {
			t.Errorf("%s: %v", c.name, err)
		}
		if c.err != nil && !errors.Is(err, c.err) {
			t.Errorf("%s: error = %v, quería %v", c.name, err, c.err)
		}
	}
	// Un desglose sin tope inflaría la transacción de captura (una fecha por día de diez años).
	many := make([]LotQty, MaxCountLotsPerItem+1)
	for i := range many {
		many[i] = LotQty{ExpiresOn: day("2026-12-01").AddDate(0, 0, i), Qty: dec("1")}
	}
	if err := ValidLotBreakdown(dec("1000"), many); !errors.Is(err, ErrValidation) {
		t.Fatalf("%d fechas = %v, quería validación", len(many), err)
	}
	if err := ValidLotBreakdown(dec("1000"), many[:MaxCountLotsPerItem]); err != nil {
		t.Fatalf("%d fechas = %v", MaxCountLotsPerItem, err)
	}
	// ErrLotsExceedCounted también es de validación: la frontera responde 400.
	if !errors.Is(ErrLotsExceedCounted, ErrValidation) {
		t.Fatal("ErrLotsExceedCounted debe envolver ErrValidation")
	}
}

func TestProjectLots(t *testing.T) {
	today := day("2026-11-23")
	lot := func(d, q string) LotStock { return LotStock{ExpiresOn: day(d), Qty: dec(q)} }
	use := func(s string) *decimal.Decimal { v := dec(s); return &v }
	type want struct {
		leftover string // "" = se termina a tiempo
		finishes int    // -1 = no aplica
		expired  bool
	}
	cases := []struct {
		name    string
		undated string
		lots    []LotStock
		daily   *decimal.Decimal
		want    []want
	}{
		{
			// El ejemplo del spec: 3 kg que caducan en 20 días con 100 g al día.
			name: "no se termina antes de caducar", undated: "0", daily: use("0.1"),
			lots: []LotStock{lot("2026-12-13", "3")},
			want: []want{{leftover: "0.9", finishes: -1}},
		},
		{
			name: "se termina a tiempo", undated: "0", daily: use("1"),
			lots: []LotStock{lot("2026-12-13", "3")},
			want: []want{{finishes: 3}},
		},
		{
			// Lo sin fecha se usa primero: retrasa el lote.
			name: "lo sin fecha va antes", undated: "2", daily: use("1"),
			lots: []LotStock{lot("2026-11-26", "3")},
			want: []want{{leftover: "1", finishes: -1}},
		},
		{
			// Lo que sobra de un lote al caducar no se sigue usando: el siguiente empieza ese día.
			name: "un lote que caduca con sobrante no retrasa al siguiente", undated: "0", daily: use("1"),
			lots: []LotStock{lot("2026-11-24", "5"), lot("2026-11-30", "3")},
			want: []want{{leftover: "3", finishes: -1}, {finishes: 5}},
		},
		{
			name: "dos lotes en orden", undated: "0", daily: use("1"),
			lots: []LotStock{lot("2026-11-25", "2"), lot("2026-11-27", "4")},
			want: []want{{finishes: 2}, {leftover: "1", finishes: -1}},
		},
		{
			name: "sin consumo reciente", undated: "0", daily: nil,
			lots: []LotStock{lot("2026-12-13", "3")},
			want: []want{{leftover: "3", finishes: -1}},
		},
		{
			name: "consumo cero es sin consumo", undated: "0", daily: use("0"),
			lots: []LotStock{lot("2026-12-13", "3")},
			want: []want{{leftover: "3", finishes: -1}},
		},
		{
			// Un caducado no se usa ni retrasa a nadie.
			name: "caducado", undated: "0", daily: use("1"),
			lots: []LotStock{lot("2026-11-20", "2"), lot("2026-11-25", "2")},
			want: []want{{leftover: "2", finishes: -1, expired: true}, {finishes: 2}},
		},
		{
			// Caduca hoy: todavía se usa hoy (un día).
			name: "caduca hoy", undated: "0", daily: use("1"),
			lots: []LotStock{lot("2026-11-23", "3")},
			want: []want{{leftover: "2", finishes: -1}},
		},
		{
			// Lo sin fecha negativo no adelanta nada: es un faltante, no existencia.
			name: "sin fecha negativo", undated: "-4", daily: use("1"),
			lots: []LotStock{lot("2026-11-25", "2")},
			want: []want{{finishes: 2}},
		},
	}
	for _, c := range cases {
		got := ProjectLots(dec(c.undated), c.lots, c.daily, today)
		if len(got) != len(c.want) {
			t.Fatalf("%s: %d proyecciones, quería %d", c.name, len(got), len(c.want))
		}
		for i, w := range c.want {
			g := got[i]
			if g.Expired != w.expired {
				t.Errorf("%s[%d]: expired = %v", c.name, i, g.Expired)
			}
			if w.leftover == "" {
				if g.LeftoverAtExpiry != nil {
					t.Errorf("%s[%d]: dice que sobran %s y se termina a tiempo", c.name, i, g.LeftoverAtExpiry)
				}
			} else if g.LeftoverAtExpiry == nil || !g.LeftoverAtExpiry.Equal(dec(w.leftover)) {
				t.Errorf("%s[%d]: sobrante = %v, quería %s", c.name, i, g.LeftoverAtExpiry, w.leftover)
			}
			if w.finishes < 0 {
				if g.FinishesInDays != nil {
					t.Errorf("%s[%d]: dice que se termina en %d días", c.name, i, *g.FinishesInDays)
				}
			} else if g.FinishesInDays == nil || *g.FinishesInDays != w.finishes {
				t.Errorf("%s[%d]: se termina en %v, quería %d", c.name, i, g.FinishesInDays, w.finishes)
			}
		}
	}
}

func TestDaysLeft(t *testing.T) {
	if d := DaysLeft(day("2026-12-03"), day("2026-11-23")); d != 10 {
		t.Fatalf("DaysLeft = %d", d)
	}
	if d := DaysLeft(day("2026-11-20"), day("2026-11-23")); d != -3 {
		t.Fatalf("DaysLeft caducado = %d", d)
	}
}
