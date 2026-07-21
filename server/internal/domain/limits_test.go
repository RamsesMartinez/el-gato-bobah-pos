package domain

import (
	"testing"

	"github.com/shopspring/decimal"
)

// d es un helper de test para construir decimales exactos desde string.
func d(s string) decimal.Decimal { return decimal.RequireFromString(s) }

func TestValidMoney(t *testing.T) {
	cases := []struct {
		name      string
		v         decimal.Decimal
		allowZero bool
		want      bool
	}{
		{"monto normal", d("150.50"), false, true},
		{"un centavo", d("0.01"), false, true},
		{"cero rechazado sin allowZero", decimal.Zero, false, false},
		{"cero aceptado con allowZero", decimal.Zero, true, true},
		{"negativo", d("-1"), true, false},
		{"tope exacto", MaxMoney, false, true},
		{"sobre el tope (evita overflow numeric(10,2))", MaxMoney.Add(d("0.01")), false, false},
		// El punto de todo el refactor: 0.1 + 0.2 == 0.30 exacto (con float64 daría 0.30000000000000004).
		{"suma exacta sin drift", d("0.1").Add(d("0.2")), false, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := ValidMoney(c.v, c.allowZero); got != c.want {
				t.Fatalf("ValidMoney(%v, %v) = %v, want %v", c.v, c.allowZero, got, c.want)
			}
		})
	}
}

func TestValidQty(t *testing.T) {
	cases := []struct {
		name          string
		v, max        decimal.Decimal
		allowNegative bool
		want          bool
	}{
		{"venta normal", d("2"), MaxOrderQty, false, true},
		{"fraccionaria", d("1.5"), MaxOrderQty, false, true},
		{"cero nunca", decimal.Zero, MaxOrderQty, true, false},
		{"negativo en venta", d("-1"), MaxOrderQty, false, false},
		{"negativo en stock (merma)", d("-5"), MaxStockQty, true, true},
		{"tope de venta", MaxOrderQty, MaxOrderQty, false, true},
		{"sobre tope de venta (evita int16 wrap del modificador)", MaxOrderQty.Add(d("1")), MaxOrderQty, false, false},
		{"bajo el tope negativo de stock", MaxStockQty.Neg(), MaxStockQty, true, true},
		{"sobre el tope negativo", MaxStockQty.Neg().Sub(d("1")), MaxStockQty, true, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := ValidQty(c.v, c.max, c.allowNegative); got != c.want {
				t.Fatalf("ValidQty(%v, %v, %v) = %v, want %v", c.v, c.max, c.allowNegative, got, c.want)
			}
		})
	}
}
