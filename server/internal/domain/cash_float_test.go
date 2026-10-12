package domain

import (
	"errors"
	"testing"

	"github.com/shopspring/decimal"
)

func decp(s string) *decimal.Decimal { v := decimal.RequireFromString(s); return &v }

func TestWithdrawal(t *testing.T) {
	cases := []struct {
		name          string
		counted, left string
		want          string
		wantErr       bool
	}{
		{"lo contado menos el fondo", "5300", "1000", "4300", false},
		{"todo se queda: retiro cero", "5300", "5300", "0", false},
		{"fondo cero: todo es retiro", "5300", "0", "5300", false},
		{"redondea a centavos", "100.005", "0", "100.01", false},
		{"un fondo mayor a lo contado sería un retiro negativo", "800", "1000", "", true},
		{"fondo negativo", "800", "-1", "", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := Withdrawal(*decp(c.counted), *decp(c.left))
			if c.wantErr {
				if !errors.Is(err, ErrValidation) {
					t.Fatalf("esperaba ErrValidation, obtuvo %v (retiro %s)", err, got)
				}
				return
			}
			if err != nil || !got.Equal(*decp(c.want)) {
				t.Fatalf("Withdrawal(%s, %s) = %s, %v; quería %s", c.counted, c.left, got, err, c.want)
			}
		})
	}
}

func TestResolveFloatLeft(t *testing.T) {
	cases := []struct {
		name          string
		sent, fixed   *decimal.Decimal
		counted, want string
		wantErr       bool
	}{
		{"lo que manda quien cierra manda", decp("800"), decp("1000"), "5300", "800", false},
		{"sin cifra usa el fijo", nil, decp("1000"), "5300", "1000", false},
		// Una tableta con el front viejo no manda floatLeft: el fijo no puede rebotarle el cierre.
		{"el fijo mayor a lo contado se recorta a lo contado", nil, decp("1000"), "800", "800", false},
		{"sin fijo ni cifra se deja todo lo contado", nil, nil, "5300", "5300", false},
		{"cero enviado es cero, no ausente", decp("0"), decp("1000"), "5300", "0", false},
		{"una cifra explícita mayor a lo contado se rechaza", decp("6000"), decp("1000"), "5300", "", true},
		{"una cifra negativa se rechaza", decp("-5"), nil, "5300", "", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := ResolveFloatLeft(c.sent, c.fixed, *decp(c.counted))
			if c.wantErr {
				if !errors.Is(err, ErrValidation) {
					t.Fatalf("esperaba ErrValidation, obtuvo %v (%s)", err, got)
				}
				return
			}
			if err != nil || !got.Equal(*decp(c.want)) {
				t.Fatalf("got %s, %v; quería %s", got, err, c.want)
			}
		})
	}
}
