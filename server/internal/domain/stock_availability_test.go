package domain

import (
	"errors"
	"testing"
)

// availabilityGraph: refresco (receta de una lata por pieza), frappé (receta de leche y café),
// botella con existencia propia, paquete de dos refrescos y un producto que no descuenta nada.
func availabilityGraph() StockGraph {
	soda, frappe, cafe := int64(100), int64(101), int64(102)
	return StockGraph{
		Ingredients: map[int64]StockIngredient{1: {ID: 1}, 2: {ID: 2}, 3: {ID: 3}},
		Recipes: map[int64][]RecipeLine{
			soda:   {{IngredientID: 1, QtyBase: d("1")}},
			frappe: {{IngredientID: 2, QtyBase: d("250")}, {IngredientID: 3, QtyBase: d("20")}},
			cafe:   {{IngredientID: 3, QtyBase: d("15")}},
		},
		Products: map[int64]StockProduct{
			10: {ID: 10, RecipeID: &soda},
			11: {ID: 11, RecipeID: &frappe},
			12: {ID: 12, TrackStock: true},
			13: {ID: 13, IsCombo: true},
			14: {ID: 14},
			15: {ID: 15, RecipeID: &cafe},
		},
		ComboDefaults: map[int64][]ComboDefault{13: {{ProductID: 10, MinSelect: 2}}},
	}
}

func lv(pairs ...any) StockLevels {
	out := StockLevels{}
	for i := 0; i < len(pairs); i += 2 {
		out[pairs[i].(StockKey)] = d(pairs[i+1].(string))
	}
	return out
}

var (
	lata    = StockKey{ItemType: "ingrediente", ID: 1}
	leche   = StockKey{ItemType: "ingrediente", ID: 2}
	cafeIng = StockKey{ItemType: "ingrediente", ID: 3}
	botella = StockKey{ItemType: "producto", ID: 12}
)

func TestSoldOut(t *testing.T) {
	g := availabilityGraph()
	cases := []struct {
		name    string
		product int64
		levels  StockLevels
		want    bool
	}{
		{"receta de una lata con latas", 10, lv(lata, "3"), false},
		{"receta de una lata sin latas", 10, lv(lata, "0"), true},
		{"existencia negativa cuenta como agotado", 10, lv(lata, "-2"), true},
		{"sin fila de existencia es cero", 10, lv(), true},
		{"receta de varios insumos: falta uno", 11, lv(leche, "1000", cafeIng, "10"), true},
		{"receta de varios insumos: alcanza para uno exacto", 11, lv(leche, "250", cafeIng, "20"), false},
		{"existencia propia en cero", 12, lv(botella, "0"), true},
		{"existencia propia con una", 12, lv(botella, "1"), false},
		{"paquete: no alcanzan las piezas", 13, lv(lata, "1"), true},
		{"paquete: alcanzan las piezas", 13, lv(lata, "2"), false},
		{"lo que no descuenta nada nunca se agota", 14, lv(), false},
		{"producto que no está en el grafo nunca se agota", 999, lv(), false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := SoldOut(g, c.levels, c.product); got != c.want {
				t.Fatalf("SoldOut = %v, se esperaba %v", got, c.want)
			}
		})
	}
}

func TestCheckStock(t *testing.T) {
	g := availabilityGraph()
	line := func(id int64, name, qty string) StockCheckLine {
		return StockCheckLine{ProductID: id, Name: name, Qty: d(qty)}
	}
	cases := []struct {
		name     string
		lines    []StockCheckLine
		levels   StockLevels
		wantName string // "" = pasa
		wantLeft string
	}{
		{"cantidad exacta a la existencia pasa", []StockCheckLine{line(10, "Refresco", "3")}, lv(lata, "3"), "", ""},
		{"una de más falla y dice cuántas quedan", []StockCheckLine{line(10, "Refresco", "4")}, lv(lata, "3"), "Refresco", "3"},
		{"sin fila no queda ninguna", []StockCheckLine{line(12, "Botella", "1")}, lv(), "Botella", "0"},
		{"dos renglones del mismo producto suman", []StockCheckLine{line(10, "Refresco", "2"), line(10, "Refresco", "2")}, lv(lata, "3"), "Refresco", "3"},
		{"dos productos que comparten insumo suman", []StockCheckLine{line(11, "Frappé", "1"), line(15, "Café", "1")}, lv(leche, "500", cafeIng, "30"), "Frappé", "1"},
		{"existencia negativa: no queda ninguna", []StockCheckLine{line(10, "Refresco", "1")}, lv(lata, "-1"), "Refresco", "0"},
		{"sin renglones pasa", nil, lv(), "", ""},
		{"lo que no descuenta pasa siempre", []StockCheckLine{line(14, "Agua del día", "50")}, lv(), "", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := CheckStock(g, c.levels, c.lines)
			if c.wantName == "" {
				if err != nil {
					t.Fatalf("se esperaba que pasara, falló con %v", err)
				}
				return
			}
			var oos OutOfStock
			if !errors.As(err, &oos) || !errors.Is(err, ErrOutOfStock) {
				t.Fatalf("se esperaba OutOfStock, fue %v", err)
			}
			if oos.Name != c.wantName || oos.Left.String() != c.wantLeft {
				t.Fatalf("se agotó %q con %s; se esperaba %q con %s", oos.Name, oos.Left, c.wantName, c.wantLeft)
			}
		})
	}
}

func TestStockNeedsListsWhatTheCheckLocks(t *testing.T) {
	g := availabilityGraph()
	keys := StockNeeds(g, []StockCheckLine{{ProductID: 11, Qty: d("2")}, {ProductID: 12, Qty: d("1")}, {ProductID: 14, Qty: d("1")}})
	want := map[StockKey]string{leche: "500", cafeIng: "40", botella: "1"}
	if len(keys) != len(want) {
		t.Fatalf("se esperaban %d renglones de almacén, fueron %d: %v", len(want), len(keys), keys)
	}
	for k, q := range want {
		if keys[k].String() != q {
			t.Fatalf("%v: se esperaba %s, fue %s", k, q, keys[k])
		}
	}
}
