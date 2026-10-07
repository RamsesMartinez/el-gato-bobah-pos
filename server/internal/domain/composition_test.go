package domain

import (
	"errors"
	"testing"

	"github.com/shopspring/decimal"
)

func ip(v int64) *int64 { return &v }

// Un catálogo chico con todo lo que la expansión tiene que distinguir.
func sampleGraph() StockGraph {
	return StockGraph{
		Ingredients: map[int64]StockIngredient{
			1: {ID: 1},                                                      // leche (se compra)
			2: {ID: 2},                                                      // azúcar
			3: {ID: 3},                                                      // agua
			4: {ID: 4, IsPrep: true, RecipeID: ip(40), YieldQty: d("1000")}, // jarabe: 1000 ml por receta
			6: {ID: 6},                                                      // perla
		},
		Recipes: map[int64][]RecipeLine{
			10: {{IngredientID: 1, QtyBase: d("200")}, {IngredientID: 4, QtyBase: d("30")}},  // frappé
			20: {{IngredientID: 1, QtyBase: d("100")}},                                       // crepa
			30: {{IngredientID: 6, QtyBase: d("50")}},                                        // extra perla
			40: {{IngredientID: 2, QtyBase: d("500")}, {IngredientID: 3, QtyBase: d("500")}}, // jarabe
		},
		Products: map[int64]StockProduct{
			100: {ID: 100, RecipeID: ip(10)}, // frappé
			200: {ID: 200, RecipeID: ip(20)}, // crepa
			300: {ID: 300, TrackStock: true}, // refresco
			400: {ID: 400, IsCombo: true},    // paquete frappé + crepa
			600: {ID: 600},                   // sin composición
		},
		ComboDefaults: map[int64][]ComboDefault{
			400: {{ProductID: 100, MinSelect: 1}, {ProductID: 200, MinSelect: 2}},
		},
		Options: map[int64]StockOption{
			7: {ID: 7, RecipeID: ip(30)},         // perla extra
			8: {ID: 8, LinkedProductID: ip(300)}, // refresco del combo
			9: {ID: 9},                           // extra sin composición
		},
	}
}

type key struct {
	kind   string
	id     int64
	option int64
	of     int64
}

func byKey(ds []StockDelta) map[key]decimal.Decimal {
	m := map[key]decimal.Decimal{}
	for _, x := range ds {
		k := key{kind: x.ItemType}
		if x.IngredientID != nil {
			k.id = *x.IngredientID
		}
		if x.ProductID != nil {
			k.id = *x.ProductID
		}
		if x.OptionID != nil {
			k.option = *x.OptionID
		}
		if x.ComponentOf != nil {
			k.of = *x.ComponentOf
		}
		m[k] = m[k].Add(x.Qty)
	}
	return m
}

func TestExpandSaleRecipeAndPrepIngredient(t *testing.T) {
	ds, _ := sampleGraph().ExpandSale(SaleLine{ProductID: 100, Qty: d("2")})
	got := byKey(ds)
	if !got[key{kind: "ingrediente", id: 1}].Equal(d("400")) {
		t.Fatalf("el frappé ×2 descuenta 400 de leche: %v", got)
	}
	// 30 ml de jarabe ×2 = 60 ml; el jarabe rinde 1000 ml con 500 de azúcar y 500 de agua.
	if !got[key{kind: "ingrediente", id: 2}].Equal(d("30")) || !got[key{kind: "ingrediente", id: 3}].Equal(d("30")) {
		t.Fatalf("el jarabe preparado se descuenta en lo que lo compone, en proporción: %v", got)
	}
	if _, ok := got[key{kind: "ingrediente", id: 4}]; ok {
		t.Fatalf("un insumo preparado sin existencias propias no se descuenta él mismo")
	}
}

func TestExpandSaleTrackStockProduct(t *testing.T) {
	ds, _ := sampleGraph().ExpandSale(SaleLine{ProductID: 300, Qty: d("3")})
	if got := byKey(ds); !got[key{kind: "producto", id: 300}].Equal(d("3")) || len(got) != 1 {
		t.Fatalf("un producto con existencias propias se descuenta él mismo: %v", got)
	}
}

func TestExpandSaleExtrasMultiplyByLineAndOptionQuantity(t *testing.T) {
	ds, _ := sampleGraph().ExpandSale(SaleLine{ProductID: 600, Qty: d("3"), Options: []SaleOption{
		{OptionID: 7, Qty: d("2")},
		{OptionID: 8, Qty: d("1")},
		{OptionID: 9, Qty: d("1")},
	}})
	got := byKey(ds)
	if !got[key{kind: "ingrediente", id: 6, option: 7}].Equal(d("300")) {
		t.Fatalf("perla extra ×2 en un renglón ×3 = 6 veces 50: %v", got)
	}
	if !got[key{kind: "producto", id: 300, option: 8}].Equal(d("3")) {
		t.Fatalf("el extra ligado a un producto descuenta ese producto, con el extra como origen: %v", got)
	}
	if len(got) != 2 {
		t.Fatalf("el producto sin composición y el extra sin composición no descuentan: %v", got)
	}
}

func TestExpandSalePackageUsesTheCostingRule(t *testing.T) {
	ds, comps := sampleGraph().ExpandSale(SaleLine{ProductID: 400, Qty: d("2")})
	got := byKey(ds)
	// Como el costeo: cada hueco por max(min_select, 1). Frappé ×1 y crepa ×2, por 2 paquetes.
	if !got[key{kind: "ingrediente", id: 1, of: 400}].Equal(d("800")) {
		t.Fatalf("leche: 2 frappés (400) + 4 crepas (400) = 800, con el paquete como origen: %v", got)
	}
	if len(comps) != 2 || !comps[0].Qty.Equal(d("2")) || comps[1].ProductID != 200 || !comps[1].Qty.Equal(d("4")) {
		t.Fatalf("los componentes vendidos se copian para el historial: %+v", comps)
	}
}

func TestExpandSaleCutsOldCycles(t *testing.T) {
	g := sampleGraph()
	g.Ingredients[4] = StockIngredient{ID: 4, IsPrep: true, RecipeID: ip(41), YieldQty: d("1")}
	g.Recipes[41] = []RecipeLine{{IngredientID: 4, QtyBase: d("1")}} // el jarabe se contiene a sí mismo
	ds, _ := g.ExpandSale(SaleLine{ProductID: 100, Qty: d("1")})
	if len(ds) == 0 {
		t.Fatalf("un ciclo en datos viejos no cuelga la venta ni la deja sin descontar lo demás")
	}
}

func TestStockQtyNeverRoundsAPositiveToZero(t *testing.T) {
	if got := StockQty(d("0.00001")); !got.Equal(d("0.0001")) {
		t.Fatalf("una cantidad positiva no se pierde al redondear: %s", got)
	}
	if got := StockQty(d("2.123456")); !got.Equal(d("2.1235")) {
		t.Fatalf("redondeo a 4 decimales: %s", got)
	}
}

func TestValidateComposition(t *testing.T) {
	g := sampleGraph()
	if err := g.ValidatePackage(400, []int64{100, 400}); !errors.Is(err, ErrPackageInPackage) {
		t.Fatalf("un paquete no puede contenerse: %v", err)
	}
	if err := g.ValidatePackage(401, []int64{100, 400}); !errors.Is(err, ErrPackageInPackage) {
		t.Fatalf("un paquete no puede contener otro paquete: %v", err)
	}
	if err := g.ValidatePackage(401, []int64{100, 200}); err != nil {
		t.Fatalf("un paquete de productos sueltos es válido: %v", err)
	}
	// El jarabe (4) no puede llevar algo que lo lleva a él.
	g.Ingredients[7] = StockIngredient{ID: 7, IsPrep: true, RecipeID: ip(70), YieldQty: d("1")}
	g.Recipes[70] = []RecipeLine{{IngredientID: 4, QtyBase: d("1")}}
	if err := g.ValidatePrepIngredient(4, []int64{7}); !errors.Is(err, ErrCompositionCycle) {
		t.Fatalf("una composición circular indirecta se rechaza: %v", err)
	}
	if err := g.ValidatePrepIngredient(4, []int64{4}); !errors.Is(err, ErrCompositionCycle) {
		t.Fatalf("una composición circular directa se rechaza: %v", err)
	}
	if err := g.ValidatePrepIngredient(4, []int64{2, 3}); err != nil {
		t.Fatalf("lo normal es válido: %v", err)
	}
}

// La base entrega las cantidades de receta con 6 decimales (numeric(20,6)) y el rendimiento con 4.
// Multiplicar y dividir eso por cada nivel acumulaba decimales hasta pasar el tope de escala de
// ValidQty, y la venta de un té con jarabe tronaba con «datos inválidos». Las pruebas de arriba
// usaban cantidades sin decimales y no lo veían.
func TestExpandSaleKeepsTheScaleOfDatabaseQuantities(t *testing.T) {
	g := StockGraph{
		Ingredients: map[int64]StockIngredient{
			1: {ID: 1}, 2: {ID: 2},
			3: {ID: 3, IsPrep: true, RecipeID: ip(20), YieldQty: d("1000.0000")},
			4: {ID: 4, IsPrep: true, RecipeID: ip(30), YieldQty: d("3.0000")},
		},
		Recipes: map[int64][]RecipeLine{
			10: {{IngredientID: 4, QtyBase: d("30.000000")}},
			20: {{IngredientID: 1, QtyBase: d("500.000000")}, {IngredientID: 2, QtyBase: d("500.000000")}},
			30: {{IngredientID: 3, QtyBase: d("7.000000")}},
		},
		Products: map[int64]StockProduct{100: {ID: 100, RecipeID: ip(10)}},
	}
	ds, _ := g.ExpandSale(SaleLine{ProductID: 100, Qty: d("2.00")})
	if len(ds) != 2 {
		t.Fatalf("dos insumos comprados: %+v", ds)
	}
	for _, x := range ds {
		if !ValidQty(x.Qty.Neg(), MaxStockQty, true) {
			t.Fatalf("la cantidad %s (exponente %d) no pasa la validación de la frontera", x.Qty, x.Qty.Exponent())
		}
		// 2 × 30 / 3 × 7 / 1000 × 500 = 70
		if !x.Qty.Equal(d("70")) {
			t.Fatalf("cantidad: quería 70, salió %s", x.Qty)
		}
	}
}

func TestValidateCompositionInput(t *testing.T) {
	gramo, litro := "masa", "volumen"
	item := func(ing int64, qty string, kind string) CompositionItem {
		return CompositionItem{IngredientID: ing, Quantity: d(qty), UnitKind: kind, IngredientUnitKind: gramo}
	}
	cases := []struct {
		name string
		in   CompositionInput
		err  error
	}{
		{"insumos", CompositionInput{Items: []CompositionItem{item(1, "200", gramo), item(2, "0.5", gramo)}}, nil},
		{"producto ligado", CompositionInput{LinkedProductID: ip(9)}, nil},
		{"vacía borra", CompositionInput{}, nil},
		{"las dos cosas", CompositionInput{LinkedProductID: ip(9), Items: []CompositionItem{item(1, "1", gramo)}}, ErrValidation},
		{"cantidad cero", CompositionInput{Items: []CompositionItem{item(1, "0", gramo)}}, ErrValidation},
		{"cantidad negativa", CompositionInput{Items: []CompositionItem{item(1, "-1", gramo)}}, ErrValidation},
		{"cantidad absurda", CompositionInput{Items: []CompositionItem{item(1, "100000000", gramo)}}, ErrValidation},
		{"insumo repetido", CompositionInput{Items: []CompositionItem{item(1, "1", gramo), item(1, "2", gramo)}}, ErrValidation},
		{"unidad de otro tipo", CompositionInput{Items: []CompositionItem{item(1, "1", litro)}}, ErrRecipeUnitMismatch},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := c.in.Validate()
			if c.err == nil && err != nil || c.err != nil && !errors.Is(err, c.err) {
				t.Fatalf("Validate() = %v, quería %v", err, c.err)
			}
		})
	}
}
