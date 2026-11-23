package domain

import (
	"errors"
	"testing"

	"github.com/shopspring/decimal"
)

func TestChannelPrice(t *testing.T) {
	base := d("35")
	if got := ChannelPrice(base, nil); !got.Equal(base) {
		t.Fatalf("sin precio propio debe heredar el de mostrador, fue %s", got)
	}
	own := d("39")
	if got := ChannelPrice(base, &own); !got.Equal(own) {
		t.Fatalf("con precio propio debe usarlo, fue %s", got)
	}
}

func TestValidChannelPrice(t *testing.T) {
	if err := ValidChannelPrice(nil); err != nil {
		t.Fatalf("vacío es heredar, no un error: %v", err)
	}
	ok := []string{"0.01", "39", "39.50"}
	for _, s := range ok {
		v := d(s)
		if err := ValidChannelPrice(&v); err != nil {
			t.Fatalf("%s debió aceptarse: %v", s, err)
		}
	}
	bad := map[string]decimal.Decimal{
		"cero (regalar no se captura por accidente)": d("0"),
		"negativo":       d("-1"),
		"tres decimales": d("39.555"),
		"más del tope":   MaxMoney.Add(d("1")),
	}
	for name, v := range bad {
		v := v
		if err := ValidChannelPrice(&v); !errors.Is(err, ErrValidation) {
			t.Fatalf("%s debió rechazarse con ErrValidation, fue %v", name, err)
		}
	}
}

func TestMeasuresStock(t *testing.T) {
	cases := []struct {
		name  string
		facts ProductModeFacts
		want  bool
	}{
		{"existencia propia", ProductModeFacts{TrackStock: true}, true},
		{"receta", ProductModeFacts{HasRecipeItems: true}, true},
		{"paquete", ProductModeFacts{IsPackage: true}, true},
		{"confirmado que no gasta", ProductModeFacts{CompositionStatus: "confirmed"}, false},
		{"lo restan sus opciones", ProductModeFacts{CompositionStatus: "confirmed", OptionsDeduct: true}, false},
		{"sin decidir", ProductModeFacts{}, false},
	}
	for _, c := range cases {
		if got := MeasuresStock(c.facts); got != c.want {
			t.Fatalf("%s: MeasuresStock = %v, se esperaba %v", c.name, got, c.want)
		}
	}
}
