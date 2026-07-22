package domain

import (
	"testing"

	"github.com/shopspring/decimal"
)

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
