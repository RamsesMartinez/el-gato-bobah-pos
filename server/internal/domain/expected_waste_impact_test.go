package domain

import (
	"math"
	"strconv"
	"strings"
	"testing"
)

func pctOf(v float64) *float64 { return new(v) }

// impactGraph: el insumo 1 (2 $/u, 10 %) entra directo en el producto 20, en el preparado 10 (que
// usa el producto 21), en un producto de costo manual (22), en un combo (24) y en dos extras; el
// insumo 2 entra solo en el producto 23; el 3 no entra en nada. Merma general 5 %.
func impactGraph() *CostGraph {
	rec100, rec200, rec201, rec202 := int64(100), int64(200), int64(201), int64(202)
	return NewCostGraph(
		map[int64]CostIngredient{
			1:  {ID: 1, CurrentCost: 2, ExpectedWastePct: pctOf(10)},
			2:  {ID: 2, CurrentCost: 1},
			3:  {ID: 3, CurrentCost: 9},
			10: {ID: 10, IsPrep: true, RecipeID: &rec100, YieldQty: 5},
		},
		map[int64][]CostRecipeItem{
			100: {{IngredientID: 1, QtyBase: 5}},
			200: {{IngredientID: 1, QtyBase: 1}},
			201: {{IngredientID: 10, QtyBase: 1}},
			202: {{IngredientID: 2, QtyBase: 1}},
		},
		map[int64]CostProduct{
			20: {ID: 20, Type: "simple", CostSource: "receta", RecipeID: &rec200},
			21: {ID: 21, Type: "simple", CostSource: "receta", RecipeID: &rec201},
			22: {ID: 22, Type: "simple", CostSource: "manual", ManualCost: 7, RecipeID: &rec200},
			23: {ID: 23, Type: "simple", CostSource: "receta", RecipeID: &rec202},
			24: {ID: 24, Type: "combo"},
		},
		map[int64][]ComboDefault{24: {{ProductID: 20, MinSelect: 2}}},
		map[int64]CostOption{
			30: {ID: 30, RecipeID: &rec200},
			31: {ID: 31, LinkedProductID: new(int64(20))},
		},
		pctOf(5),
	)
}

func near(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

func impactByKey(xs []CostImpact) map[string]CostImpact {
	m := map[string]CostImpact{}
	for _, x := range xs {
		m[x.Kind+":"+strconv.FormatInt(x.ID, 10)] = x
	}
	return m
}

func TestExpectedWasteImpact(t *testing.T) {
	cases := []struct {
		name string
		pct  *float64
		want map[string][2]float64 // antes, después
	}{
		{"10 % → 20 %", pctOf(20), map[string][2]float64{
			"prep:10":    {2.2, 2.4},
			"product:20": {2.2, 2.4},
			// El preparado sin % propio recibe la general en la receta del producto: 2.2 × 1.05.
			"product:21": {2.31, 2.52},
			"product:22": {7, 7},
			"product:24": {4.4, 4.8},
			"option:30":  {2.2, 2.4},
			"option:31":  {2.2, 2.4},
		}},
		// Sin captura, el producto hereda la general (5 %); dentro del preparado no hay herencia.
		{"10 % → sin captura", nil, map[string][2]float64{
			"prep:10":    {2.2, 2},
			"product:20": {2.2, 2.1},
			"product:21": {2.31, 2.1},
			"product:22": {7, 7},
			"product:24": {4.4, 4.2},
			"option:30":  {2.2, 2.1},
			"option:31":  {2.2, 2.1},
		}},
		// 0 es una decisión: no hereda la general (si heredara, saldría 2.1 como el nulo).
		{"10 % → 0 %", pctOf(0), map[string][2]float64{
			"prep:10":    {2.2, 2},
			"product:20": {2.2, 2},
			"product:21": {2.31, 2.1},
			"product:22": {7, 7},
			"product:24": {4.4, 4},
			"option:30":  {2.2, 2},
			"option:31":  {2.2, 2},
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			g := impactGraph()
			got := impactByKey(ExpectedWasteImpact(g, g.WithIngredientWaste(1, c.pct), 1))
			if len(got) != len(c.want) {
				t.Errorf("usos = %v; quiere %d (el 23 no usa el insumo y no va)", got, len(c.want))
			}
			for k, w := range c.want {
				x, ok := got[k]
				if !ok {
					t.Errorf("falta %s en los usos", k)
					continue
				}
				if !near(x.Before, w[0]) || !near(x.After, w[1]) {
					t.Errorf("%s = %v → %v, quiere %v → %v", k, x.Before, x.After, w[0], w[1])
				}
			}
			if !got["product:22"].Manual {
				t.Error("el producto de costo manual no se marcó: el agente creería que su costo debía cambiar")
			}
		})
	}
}

func TestWithIngredientWasteLeavesTheOriginalAlone(t *testing.T) {
	g := impactGraph()
	before := g.ProductCost(20) // llena el caché del original
	changed := g.WithIngredientWaste(1, pctOf(50))
	if near(changed.ProductCost(20), before) {
		t.Fatal("la copia reutilizó el caché del original: la vista previa diría que nada cambia")
	}
	if !near(g.ProductCost(20), before) {
		t.Fatal("la simulación alteró el grafo original")
	}
}

func TestExpectedWasteImpactWithoutUsesIsEmptyNotNil(t *testing.T) {
	g := impactGraph()
	got := ExpectedWasteImpact(g, g.WithIngredientWaste(3, pctOf(8)), 3)
	if got == nil || len(got) != 0 {
		t.Fatalf("= %#v; quiere un arreglo vacío (en JSON, [] y no null)", got)
	}
}

func TestNormalizeExpectedWasteReason(t *testing.T) {
	cases := []struct {
		in, want string
		bad      bool
	}{
		{"", "", false},    // la pantalla no manda motivo
		{"   ", "", false}, // puros espacios = sin motivo, no un motivo en blanco en la bitácora
		{"  rota poco ", "rota poco", false},
		{strings.Repeat("ñ", 200), strings.Repeat("ñ", 200), false}, // cuenta letras, no bytes
		{strings.Repeat("ñ", 201), "", true},
	}
	for _, c := range cases {
		got, err := NormalizeExpectedWasteReason(c.in)
		if (err != nil) != c.bad || got != c.want {
			t.Errorf("%q = %q, %v", c.in, got, err)
		}
	}
}
