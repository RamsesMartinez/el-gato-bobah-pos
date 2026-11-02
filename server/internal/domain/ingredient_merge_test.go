package domain

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"
)

// md y mdp son propios de este archivo: los de purchaseqty_test.go los está cambiando otra rama.
func md(s string) decimal.Decimal { return decimal.RequireFromString(s) }
func mdp(s string) *decimal.Decimal {
	d := md(s)
	return &d
}

func TestValidMergeRequest(t *testing.T) {
	many := make([]int64, MaxMergeSources+1)
	for i := range many {
		many[i] = int64(i + 10)
	}
	cases := []struct {
		name    string
		target  int64
		sources []int64
		ok      bool
	}{
		{"uno", 1, []int64{2}, true},
		{"varios", 1, []int64{2, 3, 4}, true},
		{"sin orígenes no hay nada que unir", 1, nil, false},
		{"el destino entre los orígenes se uniría consigo mismo", 1, []int64{2, 1}, false},
		{"un origen repetido traspasaría su existencia dos veces", 1, []int64{2, 2}, false},
		{"un id inválido", 1, []int64{0}, false},
		{"más de los que caben en una revisión", 1, many, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := ValidMergeRequest(c.target, c.sources)
			if (err == nil) != c.ok {
				t.Fatalf("ValidMergeRequest(%d, %v) = %v; ok=%v", c.target, c.sources, err, c.ok)
			}
			if err != nil && !errors.Is(err, ErrValidation) {
				t.Fatalf("debe ser 400: %v", err)
			}
		})
	}
}

func TestMergeFactor(t *testing.T) {
	cases := []struct {
		name     string
		sameUnit bool
		given    *decimal.Decimal
		want     string
		ok       bool
	}{
		{"misma unidad sin equivalencia vale 1", true, nil, "1", true},
		{"misma unidad con 1 explícito", true, mdp("1"), "1", true},
		{"misma unidad con otra equivalencia cambiaría cantidades sin razón", true, mdp("2"), "", false},
		{"unidad distinta exige la equivalencia", false, nil, "", false},
		{"unidad distinta con equivalencia", false, mdp("0.25"), "0.25", true},
		{"equivalencia en cero", false, mdp("0"), "", false},
		{"equivalencia negativa", false, mdp("-3"), "", false},
		{"equivalencia que no cabe", false, mdp("100000000000"), "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := MergeFactor(c.sameUnit, c.given)
			if (err == nil) != c.ok {
				t.Fatalf("MergeFactor = %v, %v; ok=%v", got, err, c.ok)
			}
			if c.ok && !got.Equal(md(c.want)) {
				t.Fatalf("factor %s; quiere %s", got, c.want)
			}
			if !c.ok && !errors.Is(err, ErrValidation) {
				t.Fatalf("debe ser 400: %v", err)
			}
		})
	}
}

func TestPlanMergedRecipe(t *testing.T) {
	const ml, liter, targetBase int16 = 2, 3, 2
	cases := []struct {
		name     string
		src      MergeRecipeLine
		existing *MergeRecipeLine
		factor   string
		want     MergedRecipeLine
		ok       bool
	}{
		{"misma unidad: cambia de insumo y conserva cantidad y unidad", MergeRecipeLine{md("0.18"), md("1000"), liter}, nil, "1",
			MergedRecipeLine{Qty: md("0.18"), UnitID: liter}, true},
		{"la receta ya llevaba el destino: se suman en su unidad base", MergeRecipeLine{md("120"), md("1"), ml}, &MergeRecipeLine{md("0.12"), md("1000"), liter}, "1",
			MergedRecipeLine{Qty: md("240"), UnitID: targetBase, Sum: true}, true},
		{"con equivalencia se convierte a la unidad base del destino", MergeRecipeLine{md("4"), md("1"), ml}, nil, "0.25",
			MergedRecipeLine{Qty: md("1"), UnitID: targetBase}, true},
		{"con equivalencia y la receta ya lo llevaba", MergeRecipeLine{md("4"), md("1"), ml}, &MergeRecipeLine{md("2"), md("1"), targetBase}, "0.25",
			MergedRecipeLine{Qty: md("3"), UnitID: targetBase, Sum: true}, true},
		{"una cantidad que la equivalencia deja en cero no se guarda", MergeRecipeLine{md("0.0001"), md("1"), ml}, nil, "0.001",
			MergedRecipeLine{}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := PlanMergedRecipe(c.src, c.existing, md(c.factor), targetBase)
			if (err == nil) != c.ok {
				t.Fatalf("PlanMergedRecipe = %+v, %v; ok=%v", got, err, c.ok)
			}
			if c.ok && (!got.Qty.Equal(c.want.Qty) || got.UnitID != c.want.UnitID || got.Sum != c.want.Sum) {
				t.Fatalf("%+v; quiere %+v", got, c.want)
			}
		})
	}
}

func TestMergedCost(t *testing.T) {
	jan, feb, mar := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	at := func(t time.Time) *time.Time { return &t }
	cases := []struct {
		name  string
		cands []CostCandidate
		want  string
		from  int64
	}{
		{"la compra más reciente entre todos, aunque sea de un origen", []CostCandidate{
			{ID: 1, Factor: md("1"), Current: md("0.0230"), LastPurchaseAt: at(jan), LastPurchaseCost: mdp("0.0230")},
			{ID: 2, Factor: md("1"), Current: md("0.0215"), LastPurchaseAt: at(feb), LastPurchaseCost: mdp("0.0215")},
		}, "0.0215", 2},
		{"la de un origen medido distinto se convierte", []CostCandidate{
			{ID: 1, Factor: md("1"), Current: md("3"), LastPurchaseAt: at(jan), LastPurchaseCost: mdp("3")},
			{ID: 2, Factor: md("0.25"), Current: md("0.5"), LastPurchaseAt: at(mar), LastPurchaseCost: mdp("0.5")},
		}, "2", 2},
		{"sin compras conserva el del destino", []CostCandidate{
			{ID: 1, Factor: md("1"), Current: md("0.02")},
			{ID: 2, Factor: md("1"), Current: md("0.05")},
		}, "0.02", 1},
		{"sin compras y el destino en cero toma el de un origen", []CostCandidate{
			{ID: 1, Factor: md("1"), Current: md("0")},
			{ID: 2, Factor: md("1"), Current: md("0")},
			{ID: 3, Factor: md("2"), Current: md("0.08")},
		}, "0.04", 3},
		{"nadie tiene costo", []CostCandidate{{ID: 1, Factor: md("1"), Current: md("0")}, {ID: 2, Factor: md("1"), Current: md("0")}}, "0", 1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, from := MergedCost(c.cands)
			if !got.Equal(md(c.want)) || from != c.from {
				t.Fatalf("MergedCost = %s de %d; quiere %s de %d", got, from, c.want, c.from)
			}
		})
	}
}

func TestMergeMinStock(t *testing.T) {
	if got := MergeMinStock(mdp("500"), []MinStockSource{{Min: mdp("900"), Factor: md("1")}}); got == nil || !got.Equal(md("500")) {
		t.Fatalf("el destino conserva su mínimo: %v", got)
	}
	if got := MergeMinStock(nil, []MinStockSource{{Min: mdp("100"), Factor: md("1")}, {Min: mdp("40"), Factor: md("4")}, {Min: nil, Factor: md("1")}}); got == nil || !got.Equal(md("160")) {
		t.Fatalf("sin mínimo toma el mayor de los orígenes, convertido: %v", got)
	}
	if got := MergeMinStock(nil, []MinStockSource{{Min: nil, Factor: md("1")}}); got != nil {
		t.Fatalf("sin mínimos queda sin mínimo: %v", got)
	}
}

func TestPlanMergedPresentations(t *testing.T) {
	target := []PresentationKey{{Name: "Caja", Brand: "Marca A", QtyInBase: md("12000")}}
	cases := []struct {
		name   string
		src    MergePresentation
		action string
		brand  string
	}{
		{"sin marca llega con el nombre del origen como marca", MergePresentation{ID: 1, Name: "Caja", QtyInBase: md("12000"), Fits: true}, MergeMove, "Leche Marca B"},
		{"con marca la conserva", MergePresentation{ID: 2, Name: "Garrafa", Brand: "Lala", QtyInBase: md("4000"), Fits: true}, MergeMove, "Lala"},
		{"igual de nombre, marca y contenido: la del origen se archiva", MergePresentation{ID: 3, Name: " caja ", Brand: "marca a", QtyInBase: md("12000"), Fits: true}, MergeArchive, "marca a"},
		{"igual de nombre y marca pero trae distinto: no se adivina cuál vale", MergePresentation{ID: 4, Name: "Caja", Brand: "Marca A", QtyInBase: md("8000"), Fits: true}, MergeConflict, "Marca A"},
		{"la que deja de cuadrar con la unidad del destino se queda archivada en el origen", MergePresentation{ID: 5, Name: "Kilo", QtyInBase: md("4"), Fits: false}, MergeArchive, "Leche Marca B"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := PlanMergedPresentations(target, []MergePresentation{c.src}, "Leche Marca B")
			if len(got) != 1 || got[0].Action != c.action || got[0].Brand != c.brand {
				t.Fatalf("%+v; quiere %s con marca %q", got, c.action, c.brand)
			}
		})
	}
	t.Run("dos orígenes con la misma presentación: la segunda choca con la que ya se movió", func(t *testing.T) {
		got := PlanMergedPresentations(nil, []MergePresentation{
			{ID: 1, Name: "Caja", Brand: "Lala", QtyInBase: md("12000"), Fits: true},
			{ID: 2, Name: "Caja", Brand: "Lala", QtyInBase: md("12000"), Fits: true},
		}, "X")
		if got[0].Action != MergeMove || got[1].Action != MergeArchive {
			t.Fatalf("%+v", got)
		}
	})
	t.Run("un nombre de origen largo se recorta a lo que cabe en la marca", func(t *testing.T) {
		long := "Leche entera ultrapasteurizada deslactosada light marca de la casa"
		got := PlanMergedPresentations(nil, []MergePresentation{{ID: 1, Name: "Caja", QtyInBase: md("1"), Fits: true}}, long)
		if n := len([]rune(got[0].Brand)); n > MaxPresentationBrand || n == 0 {
			t.Fatalf("marca de %d letras: %q", n, got[0].Brand)
		}
	})
}

func TestCleanPresentationBrand(t *testing.T) {
	long := strings.Repeat("x", MaxPresentationBrand+1)
	cases := []struct {
		in   *string
		want *string
		ok   bool
	}{
		{nil, nil, true},
		{strPtr("  Marca   A "), strPtr("Marca A"), true},
		{strPtr("   "), nil, true},
		{strPtr(long), nil, false},
	}
	for _, c := range cases {
		got, err := CleanPresentationBrand(c.in)
		if (err == nil) != c.ok {
			t.Fatalf("CleanPresentationBrand(%v) = %v", c.in, err)
		}
		if (got == nil) != (c.want == nil) || (got != nil && *got != *c.want) {
			t.Fatalf("CleanPresentationBrand(%v) = %v; quiere %v", c.in, got, c.want)
		}
	}
}

func strPtr(s string) *string { return &s }
