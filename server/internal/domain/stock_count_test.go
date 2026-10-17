package domain

import (
	"testing"

	"github.com/shopspring/decimal"
)

func TestPlanCountClose(t *testing.T) {
	d := decimal.RequireFromString
	tests := []struct {
		name  string
		lines []CountLine
		want  map[int64]string
	}{
		// El caso heredado: el sistema dice -1,200 g y se contaron 3,000.
		{"un negativo heredado sube a lo contado", []CountLine{{IngredientID: 1, Counted: d("3000"), System: d("-1200")}}, map[int64]string{1: "4200"}},
		{"sobraban: baja", []CountLine{{IngredientID: 1, Counted: d("500"), System: d("800")}}, map[int64]string{1: "-300"}},
		{"cero contado es «no hay», no «no se contó»", []CountLine{{IngredientID: 1, Counted: d("0"), System: d("250")}}, map[int64]string{1: "-250"}},
		{"sin diferencia no deja movimiento", []CountLine{{IngredientID: 1, Counted: d("12.5"), System: d("12.5")}}, map[int64]string{}},
		// Recapturar usa la lectura nueva: lo que importa es la última pareja contado/sistema.
		{"contra lo que había al capturar, no al cerrar", []CountLine{{IngredientID: 1, Counted: d("1000"), System: d("1100")}}, map[int64]string{1: "-100"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := PlanCountClose(tt.lines)
			if len(got) != len(tt.want) {
				t.Fatalf("ajustes = %v; quiere %v", got, tt.want)
			}
			for _, a := range got {
				if !a.Delta.Equal(d(tt.want[a.IngredientID])) {
					t.Errorf("insumo %d: ajuste %s; quiere %s", a.IngredientID, a.Delta, tt.want[a.IngredientID])
				}
			}
		})
	}
	t.Run("en orden de insumo", func(t *testing.T) {
		got := PlanCountClose([]CountLine{{IngredientID: 9, Counted: d("1")}, {IngredientID: 2, Counted: d("1")}})
		if got[0].IngredientID != 2 {
			t.Errorf("primero el insumo %d; quiere 2", got[0].IngredientID)
		}
	})
}

func TestValidStockQtyOrZero(t *testing.T) {
	// «0e9999» es cero, pero su exponente no cabe en una columna numeric: llegaba hasta la base y
	// respondía 500 en vez de 400 (revisión de seguridad del 038). 9999 es lo más que el JSON deja
	// pasar (`decimal.MaxDecodeExponent`).
	for q, ok := range map[string]bool{"0": true, "3000": true, "-1": false, "1e20": false, "0e9999": false, "1e-9999": false} {
		if ValidStockQtyOrZero(decimal.RequireFromString(q)) != ok {
			t.Errorf("%s: quiere %v", q, ok)
		}
	}
}
