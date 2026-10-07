package fudoimport

import (
	"slices"
	"strings"
	"testing"

	"github.com/shopspring/decimal"
)

// Nombres inventados: ningún dato del negocio entra al repositorio (AGENTS.md §1).

func dec(s string) decimal.Decimal { return decimal.RequireFromString(s) }

func sampleCatalog() Catalog {
	return Catalog{
		Units: map[string]Unit{
			"g": {ID: 1, Kind: "masa", ToBase: dec("1")}, "kg": {ID: 2, Kind: "masa", ToBase: dec("1000")},
			"ml": {ID: 3, Kind: "volumen", ToBase: dec("1")}, "l": {ID: 4, Kind: "volumen", ToBase: dec("1000")},
			"pieza": {ID: 5, Kind: "pieza", ToBase: dec("1")},
		},
		Products: []Product{
			{ID: 10, Name: "Bebida Uno"},
			{ID: 11, Name: "Bebida Capturada", HasComposition: true},
			{ID: 12, Name: "Refresco Lata", TrackStock: true, HasComposition: true},
			{ID: 13, Name: "Postre Dos"},
			{ID: 14, Name: "Repetido"}, {ID: 15, Name: "REPETIDO "},
			{ID: 16, Name: "Platillo Raro"},
		},
		Options: []Option{
			{ID: 20, Name: "Extra Crema"}, {ID: 21, Name: "Extra Crema"}, // el mismo extra en dos grupos
			{ID: 22, Name: "Refresco Lata"},
			{ID: 23, Name: "Extra Capturado", HasComposition: true},
			{ID: 24, Name: "Sin Hielo"},
		},
		Ingredients: []Ingredient{
			{ID: 30, Name: "Leche", BaseKind: "volumen"},
			{ID: 31, Name: "Azúcar", BaseKind: "masa"},
			{ID: 32, Name: "Crema", BaseKind: "volumen"},
			{ID: 33, Name: "Jarabe Casero", BaseKind: "volumen"},
			{ID: 34, Name: "Base Ya Preparada", BaseKind: "masa", HasComposition: true},
		},
	}
}

func sampleSources() Sources {
	r := func(kv ...string) Row {
		m := Row{}
		for i := 0; i < len(kv); i += 2 {
			m[kv[i]] = kv[i+1]
		}
		return m
	}
	rec := func(p, ing, qty, unit string) Row {
		return r("Producto", p, "Ingrediente", ing, "Cantidad", qty, "Unidad", unit)
	}
	return Sources{
		Recipes: []Row{
			rec("BEBIDA UNO", "Leche", "0.25", "L"),
			rec("BEBIDA UNO", "Azúcar", "10", "g"),
			rec("BEBIDA UNO", "Leche", "50", "ml"), // repetido: se suma en la unidad base
			rec("Bebida Capturada", "Leche", "1", "L"),
			rec("Postre Dos", "Insumo Que No Existe", "1", "kg"),
			rec("Postre Dos", "Azúcar", "5", "g"),
			rec("Platillo Raro", "Azúcar", "1", "L"), // unidad de otro tipo
			rec("Repetido", "Azúcar", "1", "g"),
			rec("Extra Crema", "Crema", "30", "ml"),
			rec("Extra Capturado", "Crema", "30", "ml"),
		},
		SubIngredients: []Row{
			r("Ingrediente", "Jarabe Casero", "Subingrediente", "Azúcar", "Cantidad", "0.5", "Unidad", "kg"),
			r("Ingrediente", "Base Ya Preparada", "Subingrediente", "Azúcar", "Cantidad", "1", "Unidad", "kg"),
		},
		IngredientUnits: map[string]string{"Jarabe Casero": "L", "Base Ya Preparada": "kg"},
		SubProducts:     []Row{r("Producto", "Paquete Fiesta", "Subproducto", "Bebida Uno", "Cantidad", "2", "Unidad", "unid.")},
	}
}

func TestPlanCompositions(t *testing.T) {
	p := PlanCompositions(sampleSources(), sampleCatalog())

	t.Run("un producto sin composición recibe su receta en la unidad base, sumando lo repetido", func(t *testing.T) {
		got := p.ProductRecipes[10]
		want := []Line{{IngredientID: 30, Qty: dec("300"), UnitID: 3}, {IngredientID: 31, Qty: dec("10"), UnitID: 1}}
		if !sameLines(got, want) {
			t.Fatalf("receta de Bebida Uno: %+v", got)
		}
	})
	t.Run("lo ya capturado no se pisa", func(t *testing.T) {
		if _, ok := p.ProductRecipes[11]; ok {
			t.Fatal("se pisó un producto con composición")
		}
		if _, ok := p.OptionRecipes[23]; ok {
			t.Fatal("se pisó un extra con composición")
		}
		if _, ok := p.PrepIngredients[34]; ok {
			t.Fatal("se pisó un insumo ya preparado")
		}
	})
	t.Run("un insumo que no existe deja fuera la receta entera y se reporta", func(t *testing.T) {
		if _, ok := p.ProductRecipes[13]; ok {
			t.Fatal("una receta a medias descontaría de menos")
		}
		if !hasReport(p.Report.MissingIngredients, "Insumo Que No Existe") {
			t.Fatalf("debe reportar el insumo: %v", p.Report.MissingIngredients)
		}
	})
	t.Run("una unidad de otro tipo se reporta", func(t *testing.T) {
		if _, ok := p.ProductRecipes[16]; ok || !hasReport(p.Report.UnitMismatch, "Platillo Raro") {
			t.Fatalf("unidad de otro tipo: %v", p.Report.UnitMismatch)
		}
	})
	t.Run("un nombre que empata con dos productos no se adivina", func(t *testing.T) {
		if _, ok := p.ProductRecipes[14]; ok {
			t.Fatal("se adivinó")
		}
		if _, ok := p.ProductRecipes[15]; ok {
			t.Fatal("se adivinó")
		}
		if !hasReport(p.Report.Ambiguous, "REPETIDO") {
			t.Fatalf("debe reportarse: %v", p.Report.Ambiguous)
		}
	})
	t.Run("un extra con receta en FUDO la recibe, en cada grupo donde está", func(t *testing.T) {
		for _, id := range []int64{20, 21} {
			if !sameLines(p.OptionRecipes[id], []Line{{IngredientID: 32, Qty: dec("30"), UnitID: 3}}) {
				t.Fatalf("extra %d: %+v", id, p.OptionRecipes[id])
			}
		}
	})
	t.Run("un extra que se llama como un producto es ese producto", func(t *testing.T) {
		if p.OptionLinks[22] != 12 {
			t.Fatalf("Refresco Lata debe ligarse al producto 12: %v", p.OptionLinks)
		}
	})
	t.Run("un extra sin datos no se toca", func(t *testing.T) {
		if _, ok := p.OptionRecipes[24]; ok {
			t.Fatal("Sin Hielo no tiene datos")
		}
		if _, ok := p.OptionLinks[24]; ok {
			t.Fatal("Sin Hielo no tiene datos")
		}
	})
	t.Run("un insumo compuesto rinde una unidad de FUDO en su unidad base", func(t *testing.T) {
		got, ok := p.PrepIngredients[33]
		if !ok || !got.Yield.Equal(dec("1000")) || !sameLines(got.Lines, []Line{{IngredientID: 31, Qty: dec("500"), UnitID: 1}}) {
			t.Fatalf("jarabe: %+v", got)
		}
	})
	t.Run("los paquetes no se convierten solos: se reportan", func(t *testing.T) {
		if !hasReport(p.Report.Packages, "Paquete Fiesta") {
			t.Fatalf("paquetes: %v", p.Report.Packages)
		}
	})
}

// UN INSUMO COMPUESTO CIRCULAR NO ENTRA, NI DIRECTO NI INDIRECTO.
//
// Solo se revisaba que un insumo no se contuviera a sí mismo. A lleva B y B lleva A pasaba, y la venta
// descontaba un número sin sentido hasta cortar por profundidad, sin que nadie se enterara.
func TestPlanRejectsCircularPrepIngredients(t *testing.T) {
	cat := sampleCatalog()
	cat.Ingredients = append(cat.Ingredients,
		Ingredient{ID: 40, Name: "Base A", BaseKind: "masa"},
		Ingredient{ID: 41, Name: "Base B", BaseKind: "masa"},
		// Ya compuesto en el POS: lleva la Base C, así que la Base C no puede llevarlo a él.
		Ingredient{ID: 42, Name: "Base Ya Hecha", BaseKind: "masa", HasComposition: true},
		Ingredient{ID: 43, Name: "Base C", BaseKind: "masa"},
	)
	cat.PrepComponents = map[int64][]int64{42: {43}}
	sub := func(owner, comp string) Row {
		return Row{"Ingrediente": owner, "Subingrediente": comp, "Cantidad": "1", "Unidad": "kg"}
	}
	src := Sources{
		SubIngredients:  []Row{sub("Base A", "Base B"), sub("Base B", "Base A"), sub("Base C", "Base Ya Hecha")},
		IngredientUnits: map[string]string{"Base A": "kg", "Base B": "kg", "Base C": "kg"},
	}
	p := PlanCompositions(src, cat)
	_, a := p.PrepIngredients[40]
	_, b := p.PrepIngredients[41]
	if a && b {
		t.Fatal("A lleva B y B lleva A: no pueden entrar los dos")
	}
	if _, ok := p.PrepIngredients[43]; ok {
		t.Fatal("la Base C llevaría a la Base Ya Hecha, que ya la lleva a ella")
	}
	if len(p.Report.Cycles) != 2 {
		t.Fatalf("los dos ciclos se reportan: %v", p.Report.Cycles)
	}
}

func sameLines(a, b []Line) bool {
	return slices.EqualFunc(a, b, func(x, y Line) bool {
		return x.IngredientID == y.IngredientID && x.Qty.Equal(y.Qty) && x.UnitID == y.UnitID
	})
}

func hasReport(items []string, needle string) bool {
	return slices.ContainsFunc(items, func(s string) bool { return strings.Contains(s, needle) })
}
