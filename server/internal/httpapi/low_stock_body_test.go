package httpapi

import (
	"encoding/json"
	"testing"
)

// Quitar el mínimo propio de un almacén manda branchMinStock: null. Si null se confunde con ausente,
// el servidor «no cambia» el mínimo, la confirmación dice que se quitó y la ficha sigue avisando al
// llegar a él (prueba con alguien nuevo, spec 094: la Bodega se quedó con 10 L).
func TestLowStockBodyTellsNullFromAbsent(t *testing.T) {
	cases := []struct {
		name     string
		body     string
		set, nil bool
	}{
		{"ausente no toca el mínimo", `{}`, false, true},
		{"null lo quita", `{"branchMinStock":null}`, true, true},
		{"un número lo fija", `{"branchMinStock":"10"}`, true, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var b lowStockBody
			if err := json.Unmarshal([]byte(c.body), &b); err != nil {
				t.Fatal(err)
			}
			in, err := b.input()
			if err != nil {
				t.Fatal(err)
			}
			if in.BranchMinStock.Set != c.set || (in.BranchMinStock.Value == nil) != c.nil {
				t.Fatalf("%s → Set=%v Value=%v; quiere Set=%v sin valor=%v", c.body, in.BranchMinStock.Set, in.BranchMinStock.Value, c.set, c.nil)
			}
		})
	}
}
