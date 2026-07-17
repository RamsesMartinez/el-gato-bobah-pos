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
		1:  {ID: 1, CurrentCost: 0.05, WastePct: 0},
		2:  {ID: 2, CurrentCost: 0.02, WastePct: 10},
		3:  {ID: 3, CurrentCost: 2.0, WastePct: 0},
		10: {ID: 10, IsPrep: true, RecipeID: p(100), YieldQty: 500, WastePct: 0},
	}
	recipes := map[int64][]CostRecipeItem{
		// receta del prep Mezcla Crepas
		100: {{IngredientID: 1, QtyBase: 200}, {IngredientID: 2, QtyBase: 300}},
		// receta del producto Crepa: 250 de mezcla + 1 vaso
		200: {{IngredientID: 10, QtyBase: 250}, {IngredientID: 3, QtyBase: 1}},
	}

	// harina: 200*0.05 = 10 ; leche: 300*0.02*1.10 = 6.6 ; total receta 100 = 16.6
	// prep costo por base = 16.6/500 = 0.0332
	g := NewCostGraph(ings, recipes, nil, nil, nil)

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
	g2 := NewCostGraph(ings, recipes, products, nil, nil)
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
	g3 := NewCostGraph(ings, recipes, comboProducts, comboDefaults, nil)
	if got := g3.ProductCost(9); !almost(got, 15.8) {
		t.Fatalf("comboCost(9)=%v want 15.8", got)
	}

	// opción con receta comparte el motor
	opts := map[int64]CostOption{7: {ID: 7, RecipeID: p(100)}}
	g4 := NewCostGraph(ings, recipes, nil, nil, opts)
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
	g := NewCostGraph(ings, recipes, nil, nil, nil)
	_ = g.IngredientCost(1) // debe terminar sin deadlock/stack overflow
}
