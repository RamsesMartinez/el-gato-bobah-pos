package domain

import (
	"math"

	"github.com/shopspring/decimal"
)

// Motor de costeo: roll-up recursivo ingrediente → prep → receta → producto/opción/combo,
// con merma esperada (artículo → producto → general, spec 096) y empaque incluidos, memoización y guard de ciclos.
//
// Todos los costos son "por unidad base" para ingredientes y "por unidad de venta"
// para productos/opciones. Se usa float64: es un estimado de costo con tolerancia de
// centavos, no el total cobrado (ese sale de la columna price).

type CostIngredient struct {
	ID       int64
	IsPrep   bool
	RecipeID *int64
	YieldQty float64
	// ExpectedWastePct es la merma esperada del artículo: nil = sin captura (cae al producto o al
	// general), 0 = capturado en cero (spec 096).
	ExpectedWastePct *float64
	CurrentCost      float64 // para materia prima (raw): costo por unidad base ya conocido
	// IsPackaging: vaso, tapa, popote. Solo recibe su propio %: un empaque no se pierde al preparar.
	IsPackaging bool
}

type CostRecipeItem struct {
	IngredientID int64
	QtyBase      float64 // cantidad ya normalizada a unidad base (quantity * to_base)
}

type CostProduct struct {
	ID         int64
	Type       string // 'simple' | 'combo'
	CostSource string // 'manual' | 'compra' | 'receta'
	ManualCost float64
	RecipeID   *int64
	// ExpectedWastePct reemplaza al general en los renglones de su receta; nil = usa el general.
	ExpectedWastePct *float64
}

type ComboDefault struct {
	ProductID int64
	MinSelect int
}

type CostOption struct {
	ID              int64
	RecipeID        *int64
	LinkedProductID *int64
}

type CostGraph struct {
	ingredients   map[int64]CostIngredient
	recipes       map[int64][]CostRecipeItem
	products      map[int64]CostProduct
	comboDefaults map[int64][]ComboDefault
	options       map[int64]CostOption
	general       *float64

	ingCache  map[int64]float64
	recCache  map[int64]float64
	prodCache map[int64]float64
}

// NewCostGraph builds the graph from loaded rows.
func NewCostGraph(
	ingredients map[int64]CostIngredient,
	recipes map[int64][]CostRecipeItem,
	products map[int64]CostProduct,
	comboDefaults map[int64][]ComboDefault,
	options map[int64]CostOption,
	general *float64,
) *CostGraph {
	return &CostGraph{
		ingredients:   ingredients,
		recipes:       recipes,
		products:      products,
		comboDefaults: comboDefaults,
		options:       options,
		general:       general,
		ingCache:      map[int64]float64{},
		recCache:      map[int64]float64{},
		prodCache:     map[int64]float64{},
	}
}

// IngredientCost returns the cost per base unit of an ingredient.
func (g *CostGraph) IngredientCost(id int64) float64 {
	return g.ingredientCost(id, map[int64]bool{})
}

func (g *CostGraph) ingredientCost(id int64, visiting map[int64]bool) float64 {
	if v, ok := g.ingCache[id]; ok {
		return v
	}
	ing, ok := g.ingredients[id]
	if !ok || visiting[id] {
		return 0 // desconocido o ciclo → 0 (guard)
	}
	if !ing.IsPrep || ing.RecipeID == nil || ing.YieldQty <= 0 {
		g.ingCache[id] = ing.CurrentCost
		return ing.CurrentCost
	}
	visiting[id] = true
	cost := g.prepRecipeCost(*ing.RecipeID, visiting) / ing.YieldQty
	delete(visiting, id)
	g.ingCache[id] = cost
	return cost
}

// wasteFallback es lo que recibe un renglón sin % propio: el del producto y el general en la receta
// de un producto, solo el general en la de un extra, y nada dentro de un preparado (R2). Un empaque
// nunca lo recibe: solo lleva el suyo (FR-004).
type wasteFallback struct{ product, general *float64 }

// prepRecipeCost es el costo de la receta de un preparado: sin fallback, así que no depende de quién
// lo use y se puede guardar en el caché (el costo del preparado es uno solo por artículo).
func (g *CostGraph) prepRecipeCost(recipeID int64, visiting map[int64]bool) float64 {
	if v, ok := g.recCache[recipeID]; ok {
		return v
	}
	total := g.recipeCost(recipeID, visiting, wasteFallback{})
	g.recCache[recipeID] = total
	return total
}

// recipeCost suma la receta con el % de cada renglón según la precedencia. No usa caché: la misma
// receta cuesta distinto con otro fallback (R3); quien la llama guarda el resultado.
func (g *CostGraph) recipeCost(recipeID int64, visiting map[int64]bool, fb wasteFallback) float64 {
	var total float64
	for _, line := range g.recipeLines(recipeID, visiting, fb) {
		total += line.Base + line.Waste
	}
	return total
}

// costLine es un renglón de receta ya costeado, sin redondear.
type costLine struct {
	IngredientID int64
	QtyBase      float64
	UnitCost     float64
	Base         float64
	Pct          float64
	Source       WasteSource
	Waste        float64
}

func (g *CostGraph) recipeLines(recipeID int64, visiting map[int64]bool, fb wasteFallback) []costLine {
	items := g.recipes[recipeID]
	lines := make([]costLine, 0, len(items))
	for _, item := range items {
		ing := g.ingredients[item.IngredientID]
		unit := g.ingredientCost(item.IngredientID, visiting)
		fbp, fbg := fb.product, fb.general
		if ing.IsPackaging { // decisión del dueño (2026-11-23): el general y el del producto no tocan empaques
			fbp, fbg = nil, nil
		}
		pct, src := ResolveExpectedWaste(ing.ExpectedWastePct, fbp, fbg)
		base := item.QtyBase * unit
		lines = append(lines, costLine{
			IngredientID: item.IngredientID, QtyBase: item.QtyBase, UnitCost: unit,
			Base: base, Pct: pct, Source: src, Waste: base * pct / 100,
		})
	}
	return lines
}

// RecipeCost returns the total cost of a recipe as a preparation's (only its articles' own waste).
func (g *CostGraph) RecipeCost(recipeID int64) float64 {
	return g.prepRecipeCost(recipeID, map[int64]bool{})
}

// ProductCost returns the cost per unit of a product (simple or combo).
func (g *CostGraph) ProductCost(id int64) float64 {
	return g.productCost(id, map[int64]bool{})
}

func (g *CostGraph) productCost(id int64, visiting map[int64]bool) float64 {
	if v, ok := g.prodCache[id]; ok {
		return v
	}
	p, ok := g.products[id]
	if !ok || visiting[id] {
		return 0
	}
	visiting[id] = true
	var cost float64
	switch {
	case p.Type == "combo":
		for _, d := range g.comboDefaults[id] {
			cost += g.productCost(d.ProductID, visiting) * float64(max(d.MinSelect, 1))
		}
	case p.CostSource == "receta" && p.RecipeID != nil:
		// mapa de visita fresco para ingredientes: los IDs de producto e ingrediente
		// comparten espacio numérico y colisionarían en el guard.
		cost = g.recipeCost(*p.RecipeID, map[int64]bool{}, wasteFallback{product: p.ExpectedWastePct, general: g.general})
	default:
		cost = p.ManualCost
	}
	delete(visiting, id)
	g.prodCache[id] = cost
	return cost
}

// OptionCost returns the cost of a modifier option.
func (g *CostGraph) OptionCost(id int64) float64 {
	o, ok := g.options[id]
	if !ok {
		return 0
	}
	switch {
	case o.RecipeID != nil:
		// Un extra no es un producto: artículo → general.
		return g.recipeCost(*o.RecipeID, map[int64]bool{}, wasteFallback{general: g.general})
	case o.LinkedProductID != nil:
		return g.ProductCost(*o.LinkedProductID)
	}
	return 0
}

// Topes de las columnas current_cost: products y modifier_options son numeric(12,4), ingredients
// numeric(12,6). Un valor que llega al tope no se puede guardar.
const (
	MaxProductCost    = 1e8
	MaxOptionCost     = 1e8
	MaxIngredientCost = 1e6
)

// StorableCost redondea un costo calculado a 4 decimales y dice si cabe en una columna de tope limit.
// NaN e infinito no caben (convertirlos a decimal es un panic); el tope se mide ya redondeado.
func StorableCost(x, limit float64) (decimal.Decimal, bool) {
	if math.IsNaN(x) || math.IsInf(x, 0) || math.Abs(x) >= limit {
		return decimal.Zero, false
	}
	d := decimal.NewFromFloat(x).Round(4)
	if d.Abs().GreaterThanOrEqual(decimal.NewFromFloat(limit)) {
		return decimal.Zero, false
	}
	return d, true
}

// BreakdownLine es un renglón del desglose del costo de un producto (spec 096): cuánto lleva, cuánto
// cuesta sin merma, qué % se le aplicó, de dónde vino y cuántos pesos sumó. Un preparado ya trae
// dentro la merma de sus propios artículos; aquí solo suma la que se le aplica al usarlo.
type BreakdownLine struct {
	IngredientID int64
	IsPrep       bool
	IsPackaging  bool
	QtyBase      float64
	UnitCost     float64
	Base         float64
	Pct          float64
	Source       WasteSource
	Waste        float64
}

// BreakdownComponent es un producto dentro de un combo, con su cantidad.
type BreakdownComponent struct {
	ProductID int64
	Qty       int
	Cost      float64
	Waste     float64
}

// CostBreakdown es de dónde sale el costo de un producto. Los importes van redondeados a 4 decimales
// (el redondeo de current_cost) y Waste es la suma de los renglones ya redondeados, así que la suma
// que ve quien opera cuadra con el total por construcción (SC-003). Por eso Total puede diferir de
// current_cost (redondeo de la suma sin redondear) en hasta 0.00005 por renglón: a propósito, y por
// debajo del centavo que muestra la pantalla. Lines y Components nunca son nil.
type CostBreakdown struct {
	Source     string
	Base       float64
	Waste      float64
	Total      float64
	Lines      []BreakdownLine
	Components []BreakdownComponent
}

// General es la merma general con que se armó el grafo; nil = sin merma general.
func (g *CostGraph) General() *float64 { return g.general }

func round4(x float64) float64 { return math.Round(x*1e4) / 1e4 }

// ProductBreakdown arma el desglose del costo de un producto con la misma precedencia que ProductCost.
func (g *CostGraph) ProductBreakdown(id int64) CostBreakdown {
	return g.productBreakdown(id, map[int64]bool{})
}

func (g *CostGraph) productBreakdown(id int64, visiting map[int64]bool) CostBreakdown {
	b := CostBreakdown{Lines: []BreakdownLine{}, Components: []BreakdownComponent{}}
	p, ok := g.products[id]
	if !ok || visiting[id] {
		return b
	}
	visiting[id] = true
	defer delete(visiting, id)
	switch {
	case p.Type == "combo":
		b.Source = "combo"
		for _, d := range g.comboDefaults[id] {
			qty := max(d.MinSelect, 1)
			child := g.productBreakdown(d.ProductID, visiting)
			c := BreakdownComponent{ProductID: d.ProductID, Qty: qty,
				Cost: round4(child.Total * float64(qty)), Waste: round4(child.Waste * float64(qty))}
			b.Components = append(b.Components, c)
			b.Total += c.Cost
			b.Waste += c.Waste
		}
		b.Base = b.Total - b.Waste
	case p.CostSource == "receta" && p.RecipeID != nil:
		b.Source = "receta"
		fb := wasteFallback{product: p.ExpectedWastePct, general: g.general}
		for _, l := range g.recipeLines(*p.RecipeID, map[int64]bool{}, fb) {
			line := BreakdownLine{
				IngredientID: l.IngredientID, IsPrep: g.ingredients[l.IngredientID].IsPrep,
				IsPackaging: g.ingredients[l.IngredientID].IsPackaging,
				QtyBase:     l.QtyBase, UnitCost: l.UnitCost, Base: round4(l.Base), Pct: l.Pct, Source: l.Source,
				Waste: round4(l.Waste),
			}
			b.Lines = append(b.Lines, line)
			b.Base += line.Base
			b.Waste += line.Waste
		}
		b.Total = b.Base + b.Waste
	default:
		b.Source = p.CostSource
		b.Base, b.Total = round4(p.ManualCost), round4(p.ManualCost)
	}
	b.Base, b.Waste, b.Total = round4(b.Base), round4(b.Waste), round4(b.Total)
	return b
}
