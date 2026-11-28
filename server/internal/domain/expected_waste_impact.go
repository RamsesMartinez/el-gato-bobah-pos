package domain

import (
	"fmt"
	"maps"
	"slices"
	"strings"
	"unicode/utf8"
)

// CostImpact es lo que le pasaría al costo de algo que usa un insumo si cambia su merma esperada
// (spec 099). Kind es "prep" (costo por unidad base del preparado), "product" u "option". Manual
// marca un producto de costo manual: usa el insumo, pero su costo no se mueve.
type CostImpact struct {
	Kind   string
	ID     int64
	Before float64
	After  float64
	Manual bool
}

// WithIngredientWaste devuelve una copia del grafo con la merma de un insumo cambiada y los cachés
// vacíos: la vista previa compara los dos grafos sin escribir costos para luego deshacerlos.
func (g *CostGraph) WithIngredientWaste(id int64, pct *float64) *CostGraph {
	ings := maps.Clone(g.ingredients)
	if ing, ok := ings[id]; ok {
		ing.ExpectedWastePct = pct
		ings[id] = ing
	}
	return NewCostGraph(ings, g.recipes, g.products, g.comboDefaults, g.options, g.general)
}

// ExpectedWasteImpact enumera todo lo que usa el insumo —directo, a través de preparados, de un
// producto dentro de un combo o del producto que es un extra— con su costo en cada grafo. Siempre
// un arreglo (vacío si nadie lo usa), en orden: preparados, productos y extras, cada uno por id.
func ExpectedWasteImpact(before, after *CostGraph, ingredientID int64) []CostImpact {
	u := usage{g: before, target: ingredientID, recipes: map[int64]bool{}, products: map[int64]bool{}}
	out := []CostImpact{}
	for _, id := range slices.Sorted(maps.Keys(before.ingredients)) {
		ing := before.ingredients[id]
		if ing.IsPrep && ing.RecipeID != nil && id != ingredientID && u.recipe(*ing.RecipeID, map[int64]bool{}) {
			out = append(out, CostImpact{Kind: "prep", ID: id, Before: before.IngredientCost(id), After: after.IngredientCost(id)})
		}
	}
	for _, id := range slices.Sorted(maps.Keys(before.products)) {
		if u.product(id, map[int64]bool{}) {
			p := before.products[id]
			out = append(out, CostImpact{Kind: "product", ID: id, Before: before.ProductCost(id), After: after.ProductCost(id),
				Manual: p.Type != "combo" && p.CostSource != "receta"})
		}
	}
	for _, id := range slices.Sorted(maps.Keys(before.options)) {
		o := before.options[id]
		if (o.RecipeID != nil && u.recipe(*o.RecipeID, map[int64]bool{})) ||
			(o.LinkedProductID != nil && u.product(*o.LinkedProductID, map[int64]bool{})) {
			out = append(out, CostImpact{Kind: "option", ID: id, Before: before.OptionCost(id), After: after.OptionCost(id)})
		}
	}
	return out
}

// usage responde «¿esto usa el insumo?» con memo y guardia de ciclos (las recetas pueden tener uno
// mal capturado; el motor de costeo también se protege).
type usage struct {
	g        *CostGraph
	target   int64
	recipes  map[int64]bool
	products map[int64]bool
}

func (u usage) recipe(id int64, visiting map[int64]bool) bool {
	if v, ok := u.recipes[id]; ok {
		return v
	}
	if visiting[id] {
		return false
	}
	visiting[id] = true
	found := false
	for _, item := range u.g.recipes[id] {
		if item.IngredientID == u.target {
			found = true
			break
		}
		ing := u.g.ingredients[item.IngredientID]
		if ing.IsPrep && ing.RecipeID != nil && u.recipe(*ing.RecipeID, visiting) {
			found = true
			break
		}
	}
	u.recipes[id] = found
	return found
}

func (u usage) product(id int64, visiting map[int64]bool) bool {
	if v, ok := u.products[id]; ok {
		return v
	}
	if visiting[id] {
		return false
	}
	visiting[id] = true
	p := u.g.products[id]
	found := false
	if p.Type == "combo" {
		for _, d := range u.g.comboDefaults[id] {
			if u.product(d.ProductID, visiting) {
				found = true
				break
			}
		}
	} else if p.RecipeID != nil {
		found = u.recipe(*p.RecipeID, map[int64]bool{})
	}
	u.products[id] = found
	return found
}

// MaxExpectedWasteReasonLen es el tope del motivo de un cambio de merma esperada (el mismo de
// corregir un costo).
const MaxExpectedWasteReasonLen = 200

// NormalizeExpectedWasteReason quita los espacios de los extremos. Vacío es válido: la pantalla no
// manda motivo; el MCP lo exige en su propia capa.
func NormalizeExpectedWasteReason(reason string) (string, error) {
	r := strings.TrimSpace(reason)
	if utf8.RuneCountInString(r) > MaxExpectedWasteReasonLen {
		return "", fmt.Errorf("%w: el motivo lleva a lo más %d letras", ErrValidation, MaxExpectedWasteReasonLen)
	}
	return r, nil
}
