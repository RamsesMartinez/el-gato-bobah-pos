package domain

import (
	"math"
	"testing"

	"github.com/shopspring/decimal"
)

// Los topes de grupos, ligas y extras (revisión de la 090): las columnas son smallint e integer, y un
// int que no cabe da la vuelta en silencio (65536 → 0, un grupo donde no se puede elegir nada) o
// revienta un check como 500. Un precio fuera de numeric(10,2) también salía como 500.
func TestModifierLimits(t *testing.T) {
	cases := []struct {
		name     string
		min, max int
		ok       bool
	}{
		{"opcional de 1", 0, 1, true},
		{"obligatorio de 1", 1, 1, true},
		{"máximo de smallint", 0, math.MaxInt16, true},
		{"máximo que da la vuelta a 0", 0, 65536, false},
		{"máximo que da la vuelta a negativo", 0, 40000, false},
		{"mínimo mayor que máximo", 3, 1, false},
		{"máximo cero", 0, 0, false},
		{"mínimo negativo", -1, 1, false},
	}
	for _, c := range cases {
		if got := ValidSelectRange(c.min, c.max); got != c.ok {
			t.Errorf("%s (%d, %d): %v", c.name, c.min, c.max, got)
		}
	}
	for n, ok := range map[int]bool{1: true, math.MaxInt16: true, 0: false, 65537: false, 40000: false} {
		if ValidMaxPerLine(n) != ok {
			t.Errorf("tope por renglón %d: esperaba %v", n, ok)
		}
	}
	for n, ok := range map[int]bool{0: true, 7: true, math.MaxInt32: true, math.MaxInt32 + 2: false, -1: false} {
		if ValidPosition(n) != ok {
			t.Errorf("orden %d: esperaba %v", n, ok)
		}
	}
	for s, ok := range map[string]bool{"0": true, "15.50": true, "-5": true, "99999999999": false, "-99999999999": false, "0.001": true} {
		if ValidPriceDelta(decimal.RequireFromString(s)) != ok {
			t.Errorf("precio de extra %s: esperaba %v", s, ok)
		}
	}
}
