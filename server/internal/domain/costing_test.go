package domain

import (
	"math"
	"testing"
)

func p(v int64) *int64 { return &v }

func almost(a, b float64) bool { return math.Abs(a-b) < 0.0001 }

func TestCosting(t *testing.T) {
	// Ingredientes:
	//  1 harina raw: $0.05/g, merma 0
	//  2 leche raw:  $0.02/ml, merma 10%
	//  3 vaso raw (empaque): $2/pieza, merma 0
	//  10 "Mezcla Crepas" prep: receta 100 (harina 200g + leche 300ml), yield 500 (base units), merma 0
	ings := map[int64]CostIngredient{
		1:  {ID: 1, CurrentCost: 0.05},
		2:  {ID: 2, CurrentCost: 0.02, ExpectedWastePct: pct(10)},
		3:  {ID: 3, CurrentCost: 2.0},
		10: {ID: 10, IsPrep: true, RecipeID: p(100), YieldQty: 500},
	}
	recipes := map[int64][]CostRecipeItem{
		// receta del prep Mezcla Crepas
		100: {{IngredientID: 1, QtyBase: 200}, {IngredientID: 2, QtyBase: 300}},
		// receta del producto Crepa: 250 de mezcla + 1 vaso
		200: {{IngredientID: 10, QtyBase: 250}, {IngredientID: 3, QtyBase: 1}},
	}

	// harina: 200*0.05 = 10 ; leche: 300*0.02*1.10 = 6.6 ; total receta 100 = 16.6
	// prep costo por base = 16.6/500 = 0.0332
	g := NewCostGraph(ings, recipes, nil, nil, nil, nil)

	if got := g.RecipeCost(100); !almost(got, 16.6) {
		t.Fatalf("recipeCost(100)=%v want 16.6", got)
	}
	if got := g.IngredientCost(10); !almost(got, 0.0332) {
		t.Fatalf("prep ingredientCost(10)=%v want 0.0332", got)
	}

	// producto Crepa (receta 200): mezcla 250*0.0332 = 8.3 ; vaso 1*2 = 2 ; total 10.3
	products := map[int64]CostProduct{
		1: {ID: 1, Type: "simple", CostSource: "receta", RecipeID: p(200)},
		2: {ID: 2, Type: "simple", CostSource: "manual", ManualCost: 5.5},
	}
	g2 := NewCostGraph(ings, recipes, products, nil, nil, nil)
	if got := g2.ProductCost(1); !almost(got, 10.3) {
		t.Fatalf("productCost(1)=%v want 10.3", got)
	}
	if got := g2.ProductCost(2); !almost(got, 5.5) {
		t.Fatalf("manual productCost(2)=%v want 5.5", got)
	}

	// combo: 1x producto1 (10.3) + 1x producto2 (5.5) = 15.8
	comboProducts := map[int64]CostProduct{
		1: products[1], 2: products[2],
		9: {ID: 9, Type: "combo"},
	}
	comboDefaults := map[int64][]ComboDefault{
		9: {{ProductID: 1, MinSelect: 1}, {ProductID: 2, MinSelect: 1}},
	}
	g3 := NewCostGraph(ings, recipes, comboProducts, comboDefaults, nil, nil)
	if got := g3.ProductCost(9); !almost(got, 15.8) {
		t.Fatalf("comboCost(9)=%v want 15.8", got)
	}

	// opción con receta comparte el motor
	opts := map[int64]CostOption{7: {ID: 7, RecipeID: p(100)}}
	g4 := NewCostGraph(ings, recipes, nil, nil, opts, nil)
	if got := g4.OptionCost(7); !almost(got, 16.6) {
		t.Fatalf("optionCost(7)=%v want 16.6", got)
	}
}

func TestCostingCycleGuard(t *testing.T) {
	// prep 1 depende de prep 2 y viceversa (ciclo) → no debe colgarse
	ings := map[int64]CostIngredient{
		1: {ID: 1, IsPrep: true, RecipeID: p(1), YieldQty: 1},
		2: {ID: 2, IsPrep: true, RecipeID: p(2), YieldQty: 1},
	}
	recipes := map[int64][]CostRecipeItem{
		1: {{IngredientID: 2, QtyBase: 1}},
		2: {{IngredientID: 1, QtyBase: 1}},
	}
	g := NewCostGraph(ings, recipes, nil, nil, nil, nil)
	_ = g.IngredientCost(1) // debe terminar sin deadlock/stack overflow
}

// Un costo que no cabe en su columna tumbaba el recálculo entero, y con él cualquier captura de
// receta de la empresa (issue #145). NaN e Inf hacían panic al convertirlos a decimal.
func TestStorableCost(t *testing.T) {
	cases := []struct {
		name  string
		x     float64
		limit float64
		want  string
		ok    bool
	}{
		{"redondea a 4 decimales", 2.123456, MaxProductCost, "2.1235", true},
		{"cero", 0, MaxProductCost, "0", true},
		{"justo debajo del tope", 99999999.9999, MaxProductCost, "99999999.9999", true},
		{"redondear lo lleva al tope", 99999999.99996, MaxProductCost, "", false},
		{"no cabe en un producto", 1.8e9, MaxProductCost, "", false},
		{"cabe en un producto pero no en un insumo", 2e6, MaxIngredientCost, "", false},
		{"insumo dentro del tope", 90000.01, MaxIngredientCost, "90000.01", true},
		{"NaN", math.NaN(), MaxProductCost, "", false},
		{"infinito", math.Inf(1), MaxProductCost, "", false},
		{"negativo enorme", -1.8e9, MaxProductCost, "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := StorableCost(c.x, c.limit)
			if ok != c.ok || (ok && got.String() != c.want) {
				t.Fatalf("StorableCost(%v) = %s, %v; quiere %s, %v", c.x, got, ok, c.want, c.ok)
			}
		})
	}
}

// La merma general y la del producto (spec 096). Receta del frappé: leche 200 ml a $0.025 ($5.00),
// fresa 100 g a $0.06 ($6.00), y 50 ml de jarabe, un preparado (azúcar 100 g a $0.02 por 100 ml de
// rendimiento) = $1.00. Sin ninguna merma el frappé cuesta $12.00.
func expectedWasteFixture(fresa, azucar, jarabe *float64) (map[int64]CostIngredient, map[int64][]CostRecipeItem) {
	ings := map[int64]CostIngredient{
		1:  {ID: 1, CurrentCost: 0.025},
		2:  {ID: 2, CurrentCost: 0.06, ExpectedWastePct: fresa},
		3:  {ID: 3, CurrentCost: 0.02, ExpectedWastePct: azucar},
		10: {ID: 10, IsPrep: true, RecipeID: p(100), YieldQty: 100, ExpectedWastePct: jarabe},
	}
	recipes := map[int64][]CostRecipeItem{
		100: {{IngredientID: 3, QtyBase: 100}},
		200: {{IngredientID: 1, QtyBase: 200}, {IngredientID: 2, QtyBase: 100}, {IngredientID: 10, QtyBase: 50}},
	}
	return ings, recipes
}

func TestExpectedWastePrecedenceInCost(t *testing.T) {
	frappe := func(prod *float64) map[int64]CostProduct {
		return map[int64]CostProduct{1: {ID: 1, Type: "simple", CostSource: "receta", RecipeID: p(200), ExpectedWastePct: prod}}
	}
	cases := []struct {
		name                  string
		fresa, azucar, jarabe *float64
		prod, general         *float64
		want                  float64
	}{
		{"sin nada capturado no cambia", nil, nil, nil, nil, nil, 12},
		// SC-002: 5 % general sube exactamente 5 %. Dentro del jarabe el general NO aplica: si aplicara,
		// el azúcar subiría 5 % y luego el jarabe otro 5 % (compuesto).
		{"5 % general sube 5 %", nil, nil, nil, nil, pct(5), 12.6},
		{"10 % del producto reemplaza al general, no se suma", nil, nil, nil, pct(10), pct(5), 13.2},
		{"0 % del producto con general 5 %: sin merma", nil, nil, nil, pct(0), pct(5), 12},
		// fresa 15 %: 6 × 1.15 = 6.9; leche y jarabe con el 10 % del producto: 5.5 + 1.1 = 6.6.
		{"el artículo con su % lo conserva dentro del producto", pct(15), nil, nil, pct(10), pct(5), 13.5},
		// fresa 0 capturado: 6; el resto al 5 % general: 5.25 + 1.05.
		{"0 % capturado en el artículo no suma el general", pct(0), nil, nil, nil, pct(5), 12.3},
		// La merma propia del azúcar sí entra al jarabe (es de ese artículo): 2 × 1.1 / 100 × 50 = 1.1.
		{"la merma propia de un artículo dentro del preparado aplica", nil, pct(10), nil, nil, nil, 12.1},
		// El jarabe capturado en 20 % como artículo: 1 × 1.2.
		{"el preparado con su propio % lo usa en el producto", nil, nil, pct(20), nil, pct(5), 5.25 + 6.3 + 1.2},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ings, recipes := expectedWasteFixture(c.fresa, c.azucar, c.jarabe)
			g := NewCostGraph(ings, recipes, frappe(c.prod), nil, nil, c.general)
			if got := g.ProductCost(1); !almost(got, c.want) {
				t.Fatalf("ProductCost = %v; quiere %v", got, c.want)
			}
		})
	}
}

func TestExpectedWasteOptionsCombosAndManual(t *testing.T) {
	ings, recipes := expectedWasteFixture(nil, nil, nil)
	prods := map[int64]CostProduct{
		1: {ID: 1, Type: "simple", CostSource: "receta", RecipeID: p(200), ExpectedWastePct: pct(10)},
		2: {ID: 2, Type: "simple", CostSource: "manual", ManualCost: 8, ExpectedWastePct: pct(10)},
		9: {ID: 9, Type: "combo"},
	}
	combos := map[int64][]ComboDefault{9: {{ProductID: 1, MinSelect: 1}, {ProductID: 2, MinSelect: 1}}}
	opts := map[int64]CostOption{7: {ID: 7, RecipeID: p(200)}, 8: {ID: 8, LinkedProductID: p(1)}}
	g := NewCostGraph(ings, recipes, prods, combos, opts, pct(5))
	if got := g.ProductCost(2); !almost(got, 8) {
		t.Fatalf("un costo a mano cambió con la merma: %v", got)
	}
	if got := g.ProductCost(9); !almost(got, 13.2+8) {
		t.Fatalf("el combo no suma sus productos cada uno con su %%: %v", got)
	}
	// Un extra con receta no es un producto: artículo → general.
	if got := g.OptionCost(7); !almost(got, 12.6) {
		t.Fatalf("el extra no usó el general: %v", got)
	}
	if got := g.OptionCost(8); !almost(got, 13.2) {
		t.Fatalf("el extra que es otro producto no usó el %% de ese producto: %v", got)
	}
}

// El desglose de la pantalla de costos (spec 096, SC-003): cada renglón dice su %, de dónde vino y
// cuánto sumó, y el total de merma ES la suma de los renglones al redondeo con que se guarda.
func TestProductBreakdown(t *testing.T) {
	ings, recipes := expectedWasteFixture(pct(15), pct(10), nil)
	prods := map[int64]CostProduct{
		1: {ID: 1, Type: "simple", CostSource: "receta", RecipeID: p(200), ExpectedWastePct: pct(10)},
		2: {ID: 2, Type: "simple", CostSource: "manual", ManualCost: 8},
		9: {ID: 9, Type: "combo"},
	}
	combos := map[int64][]ComboDefault{9: {{ProductID: 1, MinSelect: 2}}}
	g := NewCostGraph(ings, recipes, prods, combos, nil, pct(5))

	b := g.ProductBreakdown(1)
	if b.Source != "receta" || len(b.Lines) != 3 || b.Components == nil {
		t.Fatalf("desglose = %+v", b)
	}
	byID := map[int64]BreakdownLine{}
	var suma float64
	for _, l := range b.Lines {
		byID[l.IngredientID] = l
		suma += l.Waste
	}
	if l := byID[2]; l.Pct != 15 || l.Source != WasteFromArticle || !almost(l.Waste, 0.9) {
		t.Fatalf("la fresa no dice 15 %% del artículo: %+v", l)
	}
	if l := byID[1]; l.Pct != 10 || l.Source != WasteFromProduct || !almost(l.Waste, 0.5) {
		t.Fatalf("la leche no dice 10 %% del producto: %+v", l)
	}
	// El jarabe ya trae dentro la merma del azúcar (10 %): cuesta 1.1 y solo suma su propia merma.
	if l := byID[10]; !l.IsPrep || !almost(l.Base, 1.1) || l.Source != WasteFromProduct || !almost(l.Waste, 0.11) {
		t.Fatalf("el preparado: %+v", l)
	}
	if !almost(b.Waste, suma) || !almost(b.Total, b.Base+b.Waste) || !almost(b.Total, g.ProductCost(1)) {
		t.Fatalf("totales que no cuadran: base %v merma %v (renglones %v) total %v costo %v", b.Base, b.Waste, suma, b.Total, g.ProductCost(1))
	}

	if m := g.ProductBreakdown(2); m.Source != "manual" || len(m.Lines) != 0 || m.Lines == nil || !almost(m.Total, 8) || m.Waste != 0 {
		t.Fatalf("un costo a mano no debe traer merma ni renglones nulos: %+v", m)
	}
	c := g.ProductBreakdown(9)
	if c.Source != "combo" || len(c.Components) != 1 || c.Components[0].Qty != 2 || !almost(c.Total, 2*b.Total) || !almost(c.Waste, 2*b.Waste) {
		t.Fatalf("el combo: %+v", c)
	}
}

// Un vaso no se «pierde al preparar» (decisión del dueño, 2026-11-23): el empaque no recibe ni el %
// del producto ni el general; solo el suyo si lo tiene capturado.
func TestPackagingGetsOnlyItsOwnExpectedWaste(t *testing.T) {
	ings, recipes := expectedWasteFixture(nil, nil, nil)
	ings[4] = CostIngredient{ID: 4, CurrentCost: 2, IsPackaging: true}
	recipes[200] = append(recipes[200], CostRecipeItem{IngredientID: 4, QtyBase: 1})
	prods := map[int64]CostProduct{1: {ID: 1, Type: "simple", CostSource: "receta", RecipeID: p(200)}}

	g := NewCostGraph(ings, recipes, prods, nil, nil, pct(5))
	// 12 de lo demás al 5 % = 12.6, más el vaso de 2 sin merma.
	if got := g.ProductCost(1); !almost(got, 14.6) {
		t.Fatalf("con 5 %% general el vaso sumó merma: costo %v, quiere 14.6", got)
	}
	for _, l := range g.ProductBreakdown(1).Lines {
		if l.IngredientID == 4 && (l.Pct != 0 || l.Source != WasteNone) {
			t.Fatalf("el desglose del vaso dice %v %% %s; quiere 0 sin merma", l.Pct, l.Source)
		}
	}

	ings[4] = CostIngredient{ID: 4, CurrentCost: 2, IsPackaging: true, ExpectedWastePct: pct(10)}
	g = NewCostGraph(ings, recipes, prods, nil, nil, pct(5))
	if got := g.ProductCost(1); !almost(got, 12.6+2.2) {
		t.Fatalf("el %% capturado en el propio empaque no se respetó: %v", got)
	}

	// El % propio del producto tampoco toca al vaso: 12 × 1.08 + 2.
	prods[1] = CostProduct{ID: 1, Type: "simple", CostSource: "receta", RecipeID: p(200), ExpectedWastePct: pct(8)}
	ings[4] = CostIngredient{ID: 4, CurrentCost: 2, IsPackaging: true}
	g = NewCostGraph(ings, recipes, prods, nil, map[int64]CostOption{7: {ID: 7, RecipeID: p(200)}}, pct(5))
	if got := g.ProductCost(1); !almost(got, 12*1.08+2) {
		t.Fatalf("con 8 %% del producto el vaso sumó merma: %v", got)
	}
	// Un extra con la misma receta (artículo → general) tampoco le suma al vaso: 12.6 + 2.
	if got := g.OptionCost(7); !almost(got, 14.6) {
		t.Fatalf("en un extra el vaso recibió el general: %v", got)
	}
}
