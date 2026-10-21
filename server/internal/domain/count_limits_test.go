package domain

import (
	"errors"
	"testing"

	"github.com/shopspring/decimal"
)

// Los defaults son los valores fijos de antes (MaxStockQty y los topes del aviso): una empresa que
// nunca los toca cuenta igual que ayer, y la API anterior durante la subida coincide con la nueva.
func TestDefaultCountLimits(t *testing.T) {
	d := DefaultCountLimits()
	want := map[string][2]string{
		"masa": {"1000000", "50000"}, "volumen": {"1000000", "50000"},
		"longitud": {"1000000", "5000"}, "pieza": {"1000000", "5000"},
	}
	for kind, w := range want {
		p, ok := d.ForKind(kind)
		if !ok || !p.Max.Equal(dec(w[0])) || !p.Confirm.Equal(dec(w[1])) {
			t.Errorf("%s = %+v %v; quiere máximo %s y confirmar %s", kind, p, ok, w[0], w[1])
		}
	}
	if _, ok := d.ForKind("otra"); ok {
		t.Error("una clase desconocida devolvió límites")
	}
	if err := d.Validate(); err != nil {
		t.Fatalf("los defaults no pasan su propia validación: %v", err)
	}
}

func TestCountLimitsValidate(t *testing.T) {
	set := func(f func(*CountLimits)) CountLimits { l := DefaultCountLimits(); f(&l); return l }
	cases := []struct {
		name string
		l    CountLimits
		ok   bool
	}{
		{"subir el máximo sobre el default se acepta", set(func(l *CountLimits) { l.Mass.Max = dec("2000000") }), true},
		{"bajar el máximo se acepta", set(func(l *CountLimits) { l.Piece.Max = dec("6000") }), true},
		{"máximo en el techo se acepta", set(func(l *CountLimits) { l.Volume.Max = MaxCountLimit }), true},
		{"máximo cero", set(func(l *CountLimits) { l.Mass.Max = decimal.Zero }), false},
		{"confirmar cero", set(func(l *CountLimits) { l.Length.Confirm = decimal.Zero }), false},
		{"confirmar negativo", set(func(l *CountLimits) { l.Piece.Confirm = dec("-5") }), false},
		{"máximo igual a confirmar", set(func(l *CountLimits) { l.Mass.Max = dec("50000") }), false},
		{"máximo menor que confirmar", set(func(l *CountLimits) { l.Volume.Confirm = dec("2000000") }), false},
		{"sobre el techo", set(func(l *CountLimits) { l.Length.Max = MaxCountLimit.Add(decimal.NewFromInt(1)) }), false},
		{"escala absurda", set(func(l *CountLimits) { l.Mass.Max = decimal.RequireFromString("1e-9999") }), false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := c.l.Validate()
			if (err == nil) != c.ok {
				t.Fatalf("Validate() = %v; quiere ok=%v", err, c.ok)
			}
			if err != nil && !errors.Is(err, ErrValidation) {
				t.Fatalf("= %v; quiere ErrValidation", err)
			}
		})
	}
	// El porqué va en el mensaje: quien configura sabe qué corregir.
	err := set(func(l *CountLimits) { l.Mass.Max = dec("50000") }).Validate()
	if err == nil || err.Error() != "datos inválidos: en peso, el máximo tiene que ser mayor que la cifra para confirmar" {
		t.Fatalf("mensaje = %v", err)
	}
}

func TestCountLimitsMaxMessage(t *testing.T) {
	cases := []struct{ kind, max, want string }{
		{"masa", "1000000", "Máximo 1,000 kg por conteo"},
		{"volumen", "1000000", "Máximo 1,000 L por conteo"},
		{"longitud", "1000000", "Máximo 10,000 m por conteo"},
		{"pieza", "1000000", "Máximo 1,000,000 piezas por conteo"},
		{"masa", "2500", "Máximo 2.5 kg por conteo"},
		{"pieza", "1", "Máximo 1 pieza por conteo"},
	}
	for _, c := range cases {
		if got := CountMaxMessage(c.kind, dec(c.max)); got != c.want {
			t.Errorf("CountMaxMessage(%s, %s) = %q; quiere %q", c.kind, c.max, got, c.want)
		}
	}
}

// La cifra del conteo se compara en unidad base contra el máximo de la clase.
func TestCheckCountMax(t *testing.T) {
	l := DefaultCountLimits()
	if err := l.CheckCountMax("masa", dec("1000000")); err != nil {
		t.Fatalf("justo el máximo = %v", err)
	}
	err := l.CheckCountMax("masa", dec("1500000"))
	if !errors.Is(err, ErrValidation) || err.Error() != "datos inválidos: Máximo 1,000 kg por conteo" {
		t.Fatalf("1,500 kg = %v", err)
	}
	if err := l.CheckCountMax("pieza", decimal.Zero); err != nil {
		t.Fatalf("contar 0 = %v", err)
	}
	if err := l.CheckCountMax("pieza", dec("-1")); !errors.Is(err, ErrValidation) {
		t.Fatalf("negativo = %v", err)
	}
}
