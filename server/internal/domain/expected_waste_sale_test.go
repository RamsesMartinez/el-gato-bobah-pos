package domain

import (
	"testing"

	"github.com/shopspring/decimal"
)

func wp(s string) *decimal.Decimal { v := decimal.RequireFromString(s); return &v }

// La merma esperada que descuenta inventario (spec 096, subida 2) sigue la MISMA regla que el costo:
// artículo → producto → general en la receta del producto, artículo → general en la de un extra, solo
// el % del artículo dentro de un preparado y un empaque solo con el suyo. Lo que sale es la diferencia
// entre lo que la venta gasta con merma y sin ella, con el mismo origen que la venta.
func TestExpectedWasteOfASale(t *testing.T) {
	g := sampleGraph()
	g.Ingredients[5] = StockIngredient{ID: 5, IsPackaging: true}
	g.Recipes[10] = append(g.Recipes[10], RecipeLine{IngredientID: 5, QtyBase: d("1")}) // vaso
	general := wp("5")

	cases := []struct {
		name  string
		setup func(g *StockGraph)
		line  SaleLine
		want  map[key]string
	}{
		{"5 % general: leche 400 → 20; jarabe 60 → 63 (azúcar y agua 1.5 cada una); el vaso nada",
			func(g *StockGraph) {}, SaleLine{ProductID: 100, Qty: d("2")},
			map[key]string{{kind: "ingrediente", id: 1}: "20", {kind: "ingrediente", id: 2}: "1.5", {kind: "ingrediente", id: 3}: "1.5"}},
		{"10 % del producto reemplaza al general",
			func(g *StockGraph) { p := g.Products[100]; p.ExpectedWastePct = wp("10"); g.Products[100] = p },
			SaleLine{ProductID: 100, Qty: d("2")},
			map[key]string{{kind: "ingrediente", id: 1}: "40", {kind: "ingrediente", id: 2}: "3", {kind: "ingrediente", id: 3}: "3"}},
		{"leche en 0 % capturado no merma; el jarabe sí",
			func(g *StockGraph) { in := g.Ingredients[1]; in.ExpectedWastePct = wp("0"); g.Ingredients[1] = in },
			SaleLine{ProductID: 100, Qty: d("2")},
			map[key]string{{kind: "ingrediente", id: 2}: "1.5", {kind: "ingrediente", id: 3}: "1.5"}},
		{"el azúcar con su 10 % dentro del jarabe: 63 ml → 31.5 × 1.1 = 34.65, merma 4.65",
			func(g *StockGraph) { in := g.Ingredients[2]; in.ExpectedWastePct = wp("10"); g.Ingredients[2] = in },
			SaleLine{ProductID: 100, Qty: d("2")},
			map[key]string{{kind: "ingrediente", id: 1}: "20", {kind: "ingrediente", id: 2}: "4.65", {kind: "ingrediente", id: 3}: "1.5"}},
		{"el vaso con su propio 10 % sí merma",
			func(g *StockGraph) { in := g.Ingredients[5]; in.ExpectedWastePct = wp("10"); g.Ingredients[5] = in },
			SaleLine{ProductID: 100, Qty: d("2")},
			map[key]string{{kind: "ingrediente", id: 1}: "20", {kind: "ingrediente", id: 2}: "1.5", {kind: "ingrediente", id: 3}: "1.5", {kind: "ingrediente", id: 5}: "0.2"}},
		{"un producto con existencia propia no merma",
			func(g *StockGraph) {}, SaleLine{ProductID: 300, Qty: d("3")}, map[key]string{}},
		{"un extra con receta usa el general, con su origen",
			func(g *StockGraph) {}, SaleLine{ProductID: 600, Qty: d("2"), Options: []SaleOption{{OptionID: 7, Qty: d("1")}}},
			map[key]string{{kind: "ingrediente", id: 6, option: 7}: "5"}},
		{"un paquete: cada producto con su %, con el paquete como origen",
			func(g *StockGraph) {}, SaleLine{ProductID: 400, Qty: d("1")},
			map[key]string{{kind: "ingrediente", id: 1, of: 400}: "20", {kind: "ingrediente", id: 2, of: 400}: "0.75", {kind: "ingrediente", id: 3, of: 400}: "0.75"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			g := sampleGraph()
			g.Ingredients[5] = StockIngredient{ID: 5, IsPackaging: true}
			g.Recipes[10] = append(g.Recipes[10], RecipeLine{IngredientID: 5, QtyBase: d("1")})
			g.General = general
			c.setup(&g)
			got := byKey(g.ExpectedWasteOfSale(c.line))
			if len(got) != len(c.want) {
				t.Fatalf("merma = %v; quiere %v", got, c.want)
			}
			for k, w := range c.want {
				if !got[k].Equal(d(w)) {
					t.Fatalf("%+v = %s; quiere %s (todo: %v)", k, got[k], w, got)
				}
			}
		})
	}
	// Sin merma general ni %: nada que descontar.
	if ds := sampleGraph().ExpectedWasteOfSale(SaleLine{ProductID: 100, Qty: d("2")}); len(ds) != 0 {
		t.Fatalf("sin ningún %% salió merma: %v", byKey(ds))
	}
	_ = g
}

// El inventario y el costo aplican la misma merma: si una regla se mueve y la otra no, el almacén
// descuenta un % y el costo cobra otro (el «hermano que no se movió» de la constitución IV).
func TestInventoryWasteRuleMatchesTheCostRule(t *testing.T) {
	vals := []*float64{nil, pct(0), pct(7.5)}
	dec := func(f *float64) *decimal.Decimal {
		if f == nil {
			return nil
		}
		return new(decimal.NewFromFloat(*f))
	}
	for _, art := range vals {
		for _, prod := range vals {
			for _, gen := range vals {
				for _, pack := range []bool{false, true} {
					fp, fg := prod, gen
					if pack {
						fp, fg = nil, nil
					}
					want, _ := ResolveExpectedWaste(art, fp, fg)
					got := resolveWasteDec(StockIngredient{ExpectedWastePct: dec(art), IsPackaging: pack}, wasteFallbackDec{product: dec(prod), general: dec(gen)})
					if !got.Equal(decimal.NewFromFloat(want)) {
						t.Fatalf("artículo %v producto %v general %v empaque %v: inventario %s, costo %v", art, prod, gen, pack, got, want)
					}
				}
			}
		}
	}
}
