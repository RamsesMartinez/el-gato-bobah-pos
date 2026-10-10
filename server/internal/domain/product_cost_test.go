package domain

import (
	"errors"
	"testing"

	"github.com/shopspring/decimal"
)

func TestNewProductCostChange(t *testing.T) {
	d := func(s string) *decimal.Decimal { v := decimal.RequireFromString(s); return &v }
	cases := []struct {
		name    string
		source  string
		amount  *decimal.Decimal
		wantErr bool
		want    string
	}{
		{"manual se redondea a centavos", "manual", d("12.345"), false, "12.35"},
		{"manual en cero es válido (producto sin costo)", "manual", d("0"), false, "0"},
		{"manual sin monto: un campo vacío no es costo cero", "manual", nil, true, ""},
		{"manual negativo", "manual", d("-1"), true, ""},
		{"manual absurdo por encima del tope", "manual", d("1e12"), true, ""},
		{"manual con exponente que quema CPU", "manual", d("1e9999"), true, ""},
		{"receta sin monto", "receta", nil, false, ""},
		{"receta con monto: el costo de receta no se captura", "receta", d("10"), true, ""},
		{"compra no se elige desde esta pantalla", "compra", d("10"), true, ""},
		{"origen vacío", "", d("10"), true, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := NewProductCostChange(c.source, c.amount)
			if c.wantErr {
				if !errors.Is(err, ErrValidation) {
					t.Fatalf("quería ErrValidation, fue %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got.Source != c.source {
				t.Fatalf("origen = %q", got.Source)
			}
			if c.want != "" && !got.Amount.Equal(decimal.RequireFromString(c.want)) {
				t.Fatalf("monto = %s, quiere %s", got.Amount, c.want)
			}
		})
	}
}
