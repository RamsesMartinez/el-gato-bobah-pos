package domain

import (
	"errors"
	"testing"

	"github.com/shopspring/decimal"
)

// La frontera: las plataformas entregan el precio como ENTERO EN CENTAVOS y el POS trabaja en
// pesos. La conversión vive en un solo lugar a propósito — hacerla en dos permite que a cada uno le
// toque un redondeo distinto y que la comparación reporte diferencias de un centavo que no existen.
func TestPesosDeCentavos(t *testing.T) {
	casos := []struct {
		nombre   string
		centavos int64
		quiere   string
		falla    bool
	}{
		{nombre: "el caso de todos los días", centavos: 13000, quiere: "130"},
		{nombre: "con centavos que no son cero", centavos: 3475, quiere: "34.75"},
		{nombre: "gratis es un precio válido", centavos: 0, quiere: "0"},
		{nombre: "un peso", centavos: 100, quiere: "1"},
		{nombre: "el tope exacto", centavos: 1_000_000_000, quiere: "10000000"},
		// Los tres que importan: un precio absurdo de una API ajena tiene que salir como error de
		// validación, no como un número raro que después se guarda en una columna numeric.
		{nombre: "un centavo arriba del tope", centavos: 1_000_000_001, falla: true},
		{nombre: "negativo", centavos: -1, falla: true},
		{nombre: "el máximo de int64", centavos: 1<<63 - 1, falla: true},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			got, err := PesosDeCentavos(c.centavos)
			if c.falla {
				if err == nil {
					t.Fatalf("%d centavos debería rechazarse, dio %s", c.centavos, got)
				}
				if !errors.Is(err, ErrValidation) {
					t.Errorf("debe ser ErrValidation para que el handler responda 422, no 500: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("%d centavos: %v", c.centavos, err)
			}
			if !got.Equal(decimal.RequireFromString(c.quiere)) {
				t.Errorf("%d centavos dio %s, quería %s", c.centavos, got, c.quiere)
			}
		})
	}
}

// El resultado siempre cabe en numeric(10,2): dos decimales exactos, sin arrastre binario.
func TestPesosDeCentavosSiempreTieneDosDecimales(t *testing.T) {
	for _, c := range []int64{1, 7, 99, 101, 12345, 999999} {
		got, err := PesosDeCentavos(c)
		if err != nil {
			t.Fatal(err)
		}
		if got.Exponent() < -2 {
			t.Errorf("%d centavos dio %s, con más de dos decimales", c, got)
		}
	}
}
