package domain

import "testing"

// ModeOf tiene que decir lo mismo que ExpandSale hace al vender: si dice «receta» y la venta resta
// de la existencia propia, la pantalla miente sobre qué se descuenta.
func TestModeOf(t *testing.T) {
	cases := []struct {
		name string
		in   ProductModeFacts
		want ConsumptionMode
	}{
		{"existencia propia heredada", ProductModeFacts{TrackStock: true}, ModeOwnStock},
		// ExpandSale prefiere la existencia propia; una receta guardada al lado no se usa.
		{"existencia propia con receta guardada", ProductModeFacts{TrackStock: true, HasRecipeItems: true}, ModeOwnStock},
		{"receta con insumos", ProductModeFacts{HasRecipeItems: true, CompositionStatus: "estimated"}, ModeRecipe},
		{"paquete", ProductModeFacts{IsPackage: true, CompositionStatus: "confirmed"}, ModePackage},
		{"no gasta insumos, confirmado", ProductModeFacts{CompositionStatus: "confirmed"}, ModeNone},
		// Nunca decidido no es «no gasta insumos»: el primero es un problema y el segundo no (FR-004).
		{"nunca decidido", ProductModeFacts{}, ModeUndecided},
		{"estimado sin receta", ProductModeFacts{CompositionStatus: "estimated"}, ModeUndecided},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := ModeOf(c.in); got != c.want {
				t.Fatalf("ModeOf(%+v) = %q, quiere %q", c.in, got, c.want)
			}
		})
	}
}
