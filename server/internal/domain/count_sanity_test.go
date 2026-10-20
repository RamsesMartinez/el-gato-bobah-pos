package domain

import (
	"testing"

	"github.com/shopspring/decimal"
)

func decs(ss ...string) []decimal.Decimal {
	out := make([]decimal.Decimal, len(ss))
	for i, s := range ss {
		out[i] = dec(s)
	}
	return out
}

// El aviso del conteo nació de 740 escrito en un artículo en kg con existencia cero: con «10 veces lo
// esperado» y lo esperado en 0 nunca habría saltado. Por eso la referencia cae en cascada.
func TestCountNeedsConfirm(t *testing.T) {
	last := dec("12000")
	cases := []struct {
		name     string
		captured string
		ref      CountReference
		want     bool
	}{
		// 1. Esperado positivo.
		{"esperado 1 kg, 740 kg avisa", "740000", CountReference{Expected: dec("1000"), BaseKind: "masa"}, true},
		{"esperado 1 kg, 5 kg no avisa", "5000", CountReference{Expected: dec("1000"), BaseKind: "masa"}, false},
		{"esperado 1 kg, 50 g avisa por menor", "50", CountReference{Expected: dec("1000"), BaseKind: "masa"}, true},
		{"esperado 1 kg, contar 0 nunca avisa", "0", CountReference{Expected: dec("1000"), BaseKind: "masa"}, false},
		// 2. Sin esperado positivo: el último conteo cerrado.
		{"esperado negativo, último conteo 12 L, 1 L avisa", "1000", CountReference{Expected: dec("-1500"), LastCounted: &last, BaseKind: "volumen"}, true},
		{"esperado negativo, último conteo 12 L, 10 L no avisa", "10000", CountReference{Expected: dec("-1500"), LastCounted: &last, BaseKind: "volumen"}, false},
		{"esperado negativo, último conteo 12 L, 200 L avisa", "200000", CountReference{Expected: dec("-1500"), LastCounted: &last, BaseKind: "volumen"}, true},
		// 3. Sin esperado ni conteo: 10 × el empaque más grande.
		{"hielo en 0, bolsas de 5 y 15 kg, 740 kg avisa", "740000", CountReference{BaseKind: "masa", PackQtys: decs("5000", "15000")}, true},
		{"hielo en 0, 740 g no avisa", "740", CountReference{BaseKind: "masa", PackQtys: decs("5000", "15000")}, false},
		// Menos de 1/100 del empaque más chico.
		{"hielo en 0, 20 g avisa por muy poco", "20", CountReference{BaseKind: "masa", PackQtys: decs("5000", "15000")}, true},
		{"esperado 1 kg con bolsa de 1 kg, 5 g avisa por muy poco", "5", CountReference{Expected: dec("1000"), BaseKind: "masa", PackQtys: decs("1000")}, true},
		// 4. Sin empaques: el tope por unidad.
		{"vasos sin empaques, 6000 pz avisa", "6000", CountReference{BaseKind: "pieza"}, true},
		{"vasos sin empaques, 4800 pz no avisa", "4800", CountReference{BaseKind: "pieza"}, false},
		{"leche sin empaques, 60 L avisa", "60000", CountReference{BaseKind: "volumen"}, true},
		{"cinta sin empaques, 60 m avisa", "6000", CountReference{BaseKind: "longitud"}, true},
		{"cinta sin empaques, 40 m no avisa", "4000", CountReference{BaseKind: "longitud"}, false},
		{"un último conteo en 0 no sirve de referencia", "740000", CountReference{BaseKind: "masa", LastCounted: new(decimal.Zero), PackQtys: decs("15000")}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := CountNeedsConfirm(dec(c.captured), c.ref); got != c.want {
				t.Fatalf("CountNeedsConfirm(%s, %+v) = %v, quería %v", c.captured, c.ref, got, c.want)
			}
		})
	}
}
