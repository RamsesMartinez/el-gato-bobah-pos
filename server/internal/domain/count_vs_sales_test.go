package domain

import (
	"testing"

	"github.com/shopspring/decimal"
)

// Conteo contra ventas (spec 096, subida 2): por artículo, cuánto vendió el sistema contra lo que se
// contó. La diferencia es contado − lo que el sistema tenía, y su % es sobre lo vendido.
func TestCountVsSalesRow(t *testing.T) {
	r := CountVsSales(CountVsSalesInput{
		HasPrevious: true, Initial: d("10"), Entries: d("5"), Sold: d("12"), ExpectedWaste: d("0"),
		Registered: d("0.3"), TransfersOut: d("0"), System: d("2.7"), Counted: d("2.2"),
	})
	if !r.Diff.Equal(d("-0.5")) || r.Pct == nil || !r.Pct.Equal(d("-4.2")) {
		t.Fatalf("diferencia %s y %% %v; quiere -0.5 y -4.2", r.Diff, r.Pct)
	}
	if sinVentas := CountVsSales(CountVsSalesInput{HasPrevious: true, System: d("5"), Counted: d("4")}); sinVentas.Pct != nil {
		t.Fatalf("sin ventas no hay %% sobre lo vendido: %v", sinVentas.Pct)
	}
}

// Sugerir un % solo con datos estables: tres conteos con faltante entre 2 y 4 % y que no varíe más
// de 1.5 puntos. Más de 10 % o errático no es merma: se revisa la receta o la captura.
func TestSuggestExpectedWaste(t *testing.T) {
	p := func(s string) *decimal.Decimal { v := d(s); return &v }
	cases := []struct {
		name    string
		pcts    []*decimal.Decimal
		suggest string
		review  bool
	}{
		{"estable 3 %: sugiere 3", []*decimal.Decimal{p("-3.1"), p("-2.8"), p("-3.3")}, "3", false},
		{"redondea a medio punto", []*decimal.Decimal{p("-2.3"), p("-2.4"), p("-2.6")}, "2.5", false},
		{"solo dos conteos: no sugiere", []*decimal.Decimal{p("-3"), p("-3")}, "", false},
		{"un conteo sin ventas en medio: no sugiere", []*decimal.Decimal{p("-3"), nil, p("-3")}, "", false},
		{"errático: revisar", []*decimal.Decimal{p("-1"), p("-4"), p("-6")}, "", true},
		{"más de 10 %: revisar", []*decimal.Decimal{p("-12")}, "", true},
		{"sobrante, no merma: no sugiere", []*decimal.Decimal{p("3"), p("2.5"), p("3")}, "", false},
		{"chica y estable fuera del rango: no sugiere", []*decimal.Decimal{p("-1"), p("-1.2"), p("-0.9")}, "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s, review := SuggestExpectedWaste(c.pcts)
			if review != c.review || (c.suggest == "") != (s == nil) || (s != nil && !s.Equal(d(c.suggest))) {
				t.Fatalf("SuggestExpectedWaste = %v, %v; quiere %q, %v", s, review, c.suggest, c.review)
			}
		})
	}
}
