package domain

import (
	"math"
	"testing"
)

func TestValidMoney(t *testing.T) {
	cases := []struct {
		name      string
		v         float64
		allowZero bool
		want      bool
	}{
		{"monto normal", 150.50, false, true},
		{"un centavo", 0.01, false, true},
		{"cero rechazado sin allowZero", 0, false, false},
		{"cero aceptado con allowZero", 0, true, true},
		{"negativo", -1, true, false},
		{"tope exacto", MaxMoney, false, true},
		{"sobre el tope (evita overflow numeric(10,2))", MaxMoney + 0.01, false, false},
		{"NaN", math.NaN(), true, false},
		{"+Inf", math.Inf(1), true, false},
		{"-Inf", math.Inf(-1), true, false},
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
		v, max        float64
		allowNegative bool
		want          bool
	}{
		{"venta normal", 2, MaxOrderQty, false, true},
		{"fraccionaria", 1.5, MaxOrderQty, false, true},
		{"cero nunca", 0, MaxOrderQty, true, false},
		{"negativo en venta", -1, MaxOrderQty, false, false},
		{"negativo en stock (merma)", -5, MaxStockQty, true, true},
		{"tope de venta", MaxOrderQty, MaxOrderQty, false, true},
		{"sobre tope de venta (evita int16 wrap del modificador)", MaxOrderQty + 1, MaxOrderQty, false, false},
		{"bajo el tope negativo de stock", -MaxStockQty, MaxStockQty, true, true},
		{"sobre el tope negativo", -MaxStockQty - 1, MaxStockQty, true, false},
		{"NaN", math.NaN(), MaxOrderQty, false, false},
		{"Inf", math.Inf(1), MaxOrderQty, false, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := ValidQty(c.v, c.max, c.allowNegative); got != c.want {
				t.Fatalf("ValidQty(%v, %v, %v) = %v, want %v", c.v, c.max, c.allowNegative, got, c.want)
			}
		})
	}
}
