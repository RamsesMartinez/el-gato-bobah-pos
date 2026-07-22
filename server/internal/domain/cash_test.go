package domain

import (
	"testing"

	"github.com/shopspring/decimal"
)

func TestValidCashKind(t *testing.T) {
	for _, k := range []string{CashEntrada, CashSalida} {
		if !ValidCashKind(k) {
			t.Errorf("ValidCashKind(%q) = false, want true", k)
		}
	}
	for _, k := range []string{"", "deposito", "Entrada", "salidas"} {
		if ValidCashKind(k) {
			t.Errorf("ValidCashKind(%q) = true, want false", k)
		}
	}
}

func TestValidTransfer(t *testing.T) {
	ok := decimal.NewFromInt(100)
	cases := []struct {
		name     string
		from, to int64
		amount   decimal.Decimal
		want     bool
	}{
		{"ok", 1, 2, ok, true},
		{"same register", 1, 1, ok, false},
		{"zero amount", 1, 2, decimal.Zero, false},
		{"negative amount", 1, 2, decimal.NewFromInt(-5), false},
		{"over cap", 1, 2, MaxMoney.Add(decimal.NewFromInt(1)), false},
		{"bad from id", 0, 2, ok, false},
		{"bad to id", 1, 0, ok, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := ValidTransfer(c.from, c.to, c.amount); got != c.want {
				t.Errorf("ValidTransfer(%d,%d,%s) = %v, want %v", c.from, c.to, c.amount, got, c.want)
			}
		})
	}
}

func TestResolveDeclared(t *testing.T) {
	expected := decimal.NewFromInt(500)
	clientDeclared := decimal.NewFromInt(320)

	cases := []struct {
		name        string
		autoDeclare bool
		want        decimal.Decimal
	}{
		{"auto-declare ignores client value, uses expected", true, expected},
		{"manual keeps client value", false, clientDeclared},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := ResolveDeclared(c.autoDeclare, expected, clientDeclared)
			if !got.Equal(c.want) {
				t.Errorf("ResolveDeclared(%v, %s, %s) = %s, want %s", c.autoDeclare, expected, clientDeclared, got, c.want)
			}
		})
	}
}
