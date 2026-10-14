package domain

import (
	"errors"
	"testing"
)

// Salir de la existencia propia (spec 037): qué ajustes se insertan. Cada caso es una forma de perder
// o inventar existencias si se calcula mal.
func TestPlanModeChange(t *testing.T) {
	const branch, product, ingredient = int64(1), int64(10), int64(20)
	ing := ingredient
	single := func(qty string) ModeTarget {
		return ModeTarget{Kind: TargetItems, SingleIngredient: &ing, QtyPerSale: dec(qty)}
	}
	cases := []struct {
		name    string
		in      ModeChangeInput
		want    []StockAdjustment
		turnOff bool
		err     error
	}{
		{
			name: "sin existencia propia no hay nada que mover",
			in:   ModeChangeInput{From: ModeUndecided, ProductID: product, Target: single("1")},
		},
		{
			name: "a un insumo sin existencia: las 12 latas pasan al insumo",
			in: ModeChangeInput{From: ModeOwnStock, ProductID: product, Target: single("1"),
				Levels: []BranchLevels{{BranchID: branch, Product: dec("12")}}},
			want: []StockAdjustment{
				{BranchID: branch, ItemType: "producto", ID: product, Qty: dec("-12")},
				{BranchID: branch, ItemType: "ingrediente", ID: ingredient, Qty: dec("12")},
			},
			turnOff: true,
		},
		{
			name: "un six: cada pieza de producto son 6 del insumo",
			in: ModeChangeInput{From: ModeOwnStock, ProductID: product, Target: single("6"),
				Levels: []BranchLevels{{BranchID: branch, Product: dec("2")}}},
			want: []StockAdjustment{
				{BranchID: branch, ItemType: "producto", ID: product, Qty: dec("-2")},
				{BranchID: branch, ItemType: "ingrediente", ID: ingredient, Qty: dec("12")},
			},
			turnOff: true,
		},
		{
			name: "existencia negativa también se traslada, no se pierde",
			in: ModeChangeInput{From: ModeOwnStock, ProductID: product, Target: single("1"),
				Levels: []BranchLevels{{BranchID: branch, Product: dec("-3")}}},
			want: []StockAdjustment{
				{BranchID: branch, ItemType: "producto", ID: product, Qty: dec("3")},
				{BranchID: branch, ItemType: "ingrediente", ID: ingredient, Qty: dec("-3")},
			},
			turnOff: true,
		},
		{
			name: "el insumo ya tenía existencia y no se dijo cuántas hay de verdad",
			in: ModeChangeInput{From: ModeOwnStock, ProductID: product, Target: single("1"),
				Levels: []BranchLevels{{BranchID: branch, Product: dec("12"), Ingredient: dec("20")}}},
			err: ErrValidation,
		},
		{
			name: "el insumo ya tenía existencia: queda la cifra elegida, no la suma a ciegas",
			in: ModeChangeInput{From: ModeOwnStock, ProductID: product, Target: single("1"),
				Levels:      []BranchLevels{{BranchID: branch, Product: dec("12"), Ingredient: dec("20")}},
				ActualStock: decp("32"), SeenProductOnHand: decp("12"), SeenIngredientOnHand: decp("20")},
			want: []StockAdjustment{
				{BranchID: branch, ItemType: "producto", ID: product, Qty: dec("-12")},
				{BranchID: branch, ItemType: "ingrediente", ID: ingredient, Qty: dec("12")},
			},
			turnOff: true,
		},
		{
			name: "la cifra elegida igual a la del insumo: el producto va a cero y el insumo no se mueve",
			in: ModeChangeInput{From: ModeOwnStock, ProductID: product, Target: single("1"),
				Levels:      []BranchLevels{{BranchID: branch, Product: dec("12"), Ingredient: dec("20")}},
				ActualStock: decp("20"), SeenProductOnHand: decp("12"), SeenIngredientOnHand: decp("20")},
			want:    []StockAdjustment{{BranchID: branch, ItemType: "producto", ID: product, Qty: dec("-12")}},
			turnOff: true,
		},
		{
			name: "una venta entre mostrar y guardar: lo que se vio ya no es lo que hay",
			in: ModeChangeInput{From: ModeOwnStock, ProductID: product, Target: single("1"),
				Levels:      []BranchLevels{{BranchID: branch, Product: dec("11"), Ingredient: dec("20")}},
				ActualStock: decp("32"), SeenProductOnHand: decp("12"), SeenIngredientOnHand: decp("20")},
			err: ErrConflict,
		},
		{
			name: "cifra negativa",
			in: ModeChangeInput{From: ModeOwnStock, ProductID: product, Target: single("1"),
				Levels:      []BranchLevels{{BranchID: branch, Product: dec("12"), Ingredient: dec("20")}},
				ActualStock: decp("-1"), SeenProductOnHand: decp("12"), SeenIngredientOnHand: decp("20")},
			err: ErrValidation,
		},
		{
			name: "dos sucursales con existencia en los dos: se resuelve con un conteo",
			in: ModeChangeInput{From: ModeOwnStock, ProductID: product, Target: single("1"),
				Levels: []BranchLevels{
					{BranchID: 1, Product: dec("5"), Ingredient: dec("2")},
					{BranchID: 2, Product: dec("3"), Ingredient: dec("1")},
				},
				ActualStock: decp("10"), SeenProductOnHand: decp("8"), SeenIngredientOnHand: decp("3")},
			err: ErrConflict,
		},
		{
			name: "dos sucursales sin existencia del insumo: cada una traslada lo suyo",
			in: ModeChangeInput{From: ModeOwnStock, ProductID: product, Target: single("1"),
				Levels: []BranchLevels{{BranchID: 1, Product: dec("5")}, {BranchID: 2, Product: dec("3")}}},
			want: []StockAdjustment{
				{BranchID: 1, ItemType: "producto", ID: product, Qty: dec("-5")},
				{BranchID: 1, ItemType: "ingrediente", ID: ingredient, Qty: dec("5")},
				{BranchID: 2, ItemType: "producto", ID: product, Qty: dec("-3")},
				{BranchID: 2, ItemType: "ingrediente", ID: ingredient, Qty: dec("3")},
			},
			turnOff: true,
		},
		{
			name: "a varios insumos, un paquete o «No se cuenta»: la existencia se cierra en cero",
			in: ModeChangeInput{From: ModeOwnStock, ProductID: product, Target: ModeTarget{Kind: TargetNothing},
				Levels: []BranchLevels{{BranchID: branch, Product: dec("7")}}},
			want:    []StockAdjustment{{BranchID: branch, ItemType: "producto", ID: product, Qty: dec("-7")}},
			turnOff: true,
		},
		{
			name: "existencia propia en cero: se apaga sin ajustes",
			in: ModeChangeInput{From: ModeOwnStock, ProductID: product, Target: single("1"),
				Levels: []BranchLevels{{BranchID: branch}}},
			turnOff: true,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := PlanModeChange(c.in)
			if c.err != nil {
				if !errors.Is(err, c.err) {
					t.Fatalf("PlanModeChange = %v, quiere %v", err, c.err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got.TurnOffTrackStock != c.turnOff {
				t.Errorf("TurnOffTrackStock = %v, quiere %v", got.TurnOffTrackStock, c.turnOff)
			}
			if len(got.Adjustments) != len(c.want) {
				t.Fatalf("ajustes = %+v, quiere %+v", got.Adjustments, c.want)
			}
			for i, a := range got.Adjustments {
				w := c.want[i]
				if a.BranchID != w.BranchID || a.ItemType != w.ItemType || a.ID != w.ID || !a.Qty.Equal(w.Qty) {
					t.Errorf("ajuste %d = %+v, quiere %+v", i, a, w)
				}
			}
		})
	}
}

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
