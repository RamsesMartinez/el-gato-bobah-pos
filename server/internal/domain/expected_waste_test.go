package domain

import (
	"errors"
	"testing"

	"github.com/shopspring/decimal"
)

func pct(v float64) *float64 { return new(v) }

// La precedencia del #226: artículo capturado → producto → general → nada. El 0 capturado del
// artículo es una decisión («aquí no se pierde nada») y NO cae al producto ni al general: si cayera,
// una lata capturada en 0 a propósito sumaría el 5 % general.
func TestResolveExpectedWaste(t *testing.T) {
	cases := []struct {
		name                      string
		article, product, general *float64
		want                      float64
		src                       WasteSource
	}{
		{"artículo capturado gana al producto y al general", pct(15), pct(10), pct(5), 15, WasteFromArticle},
		{"0 capturado en el artículo no cae al general", pct(0), nil, pct(5), 0, WasteFromArticle},
		{"0 capturado en el artículo no cae al producto", pct(0), pct(10), pct(5), 0, WasteFromArticle},
		{"sin captura cae al producto", nil, pct(10), pct(5), 10, WasteFromProduct},
		{"0 del producto no cae al general", nil, pct(0), pct(5), 0, WasteFromProduct},
		{"sin artículo ni producto, el general", nil, nil, pct(5), 5, WasteFromGeneral},
		{"nada capturado, nada suma", nil, nil, nil, 0, WasteNone},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, src := ResolveExpectedWaste(c.article, c.product, c.general)
			if got != c.want || src != c.src {
				t.Fatalf("ResolveExpectedWaste = %v, %s; quiere %v, %s", got, src, c.want, c.src)
			}
		})
	}
}

// Un % vacío es «sin captura», no 0; y uno fuera de rango se rechaza como 400 antes de tocar la
// columna numeric(5,2), que con 100 o más truena como 500.
func TestValidExpectedWastePct(t *testing.T) {
	d := func(s string) *decimal.Decimal { v := decimal.RequireFromString(s); return &v }
	cases := []struct {
		name string
		v    *decimal.Decimal
		ok   bool
	}{
		{"vacío es válido (sin captura)", nil, true},
		{"cero es válido", d("0"), true},
		{"dos decimales", d("2.75"), true},
		{"justo debajo de 100", d("99.99"), true},
		{"100 duplicaría el costo y no cabe", d("100"), false},
		{"negativo", d("-0.01"), false},
		{"tres decimales no caben en la columna", d("2.755"), false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := ValidExpectedWastePct(c.v)
			if (err == nil) != c.ok {
				t.Fatalf("ValidExpectedWastePct = %v; quiere ok=%v", err, c.ok)
			}
			if err != nil && !errors.Is(err, ErrValidation) {
				t.Fatalf("el error %v no es ErrValidation: saldría como 500", err)
			}
		})
	}
}

// La merma real (spec 096): lo mermado entre lo consumido por ventas. Sin ventas no hay cociente: un
// «infinito %» espantaría a quien lo lee por un artículo que solo se tiró.
func TestRealWasteRatio(t *testing.T) {
	cases := []struct {
		name             string
		wasted, consumed float64
		want             *float64
		state            RealWasteState
	}{
		{"0.5 de 10 es 5 %", 0.5, 10, pct(5), RealWasteRatio},
		{"redondea a un decimal", 1, 3, pct(33.3), RealWasteRatio},
		{"sin merma con ventas es 0 %", 0, 10, pct(0), RealWasteRatio},
		{"merma sin ventas", 0.4, 0, nil, RealWasteNoSales},
		{"cancelaciones de más dejan el consumo negativo: sin ventas", 0.4, -1, nil, RealWasteNoSales},
		{"nada de nada", 0, 0, nil, RealWasteNoData},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, state := RealWaste(c.wasted, c.consumed)
			if state != c.state || (c.want == nil) != (got == nil) || (got != nil && *got != *c.want) {
				t.Fatalf("RealWaste = %v, %s; quiere %v, %s", got, state, c.want, c.state)
			}
		})
	}
}

// Una unión no reescribe lo pasado: las ventas y mermas del origen siguen con su id y su unidad. Para
// la merma real del destino se suman con su factor, siguiendo la cadena como el trigger de la 0104.
func TestMergeChain(t *testing.T) {
	edges := []MergeEdge{
		{Old: 2, New: 1, Factor: 1000}, // «Leche 1 L» (litros) se unió a «Leche» (ml)
		{Old: 3, New: 2, Factor: 2},    // y antes, «Leche 2 L» se había unido a «Leche 1 L»
		{Old: 9, New: 8, Factor: 5},    // otra cadena que no toca
	}
	got := MergeChain(1, edges)
	want := map[int64]float64{1: 1, 2: 1000, 3: 2000}
	if len(got) != len(want) {
		t.Fatalf("MergeChain = %v; quiere %v", got, want)
	}
	for id, f := range want {
		if !almost(got[id], f) {
			t.Fatalf("factor de %d = %v; quiere %v", id, got[id], f)
		}
	}
	// Un ciclo (no debería existir) no cuelga.
	loop := MergeChain(1, []MergeEdge{{Old: 2, New: 1, Factor: 1}, {Old: 1, New: 2, Factor: 1}})
	if len(loop) != 2 {
		t.Fatalf("ciclo = %v", loop)
	}
}
