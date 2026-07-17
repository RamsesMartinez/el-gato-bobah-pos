package domain

// Motor de costeo: roll-up recursivo ingrediente → prep → receta → producto/opción/combo,
// con merma (%) y empaque incluidos, memoización y guard de ciclos.
//
// Todos los costos son "por unidad base" para ingredientes y "por unidad de venta"
// para productos/opciones. Se usa float64: es un estimado de costo con tolerancia de
// centavos, no el total cobrado (ese sale de la columna price).

type CostIngredient struct {
	ID          int64
	IsPrep      bool
	RecipeID    *int64
	YieldQty    float64
	WastePct    float64
	CurrentCost float64 // para materia prima (raw): costo por unidad base ya conocido
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
) *CostGraph {
	return &CostGraph{
		ingredients:   ingredients,
		recipes:       recipes,
		products:      products,
		comboDefaults: comboDefaults,
		options:       options,
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
	cost := g.recipeCost(*ing.RecipeID, visiting) / ing.YieldQty
	delete(visiting, id)
	g.ingCache[id] = cost
	return cost
}

func (g *CostGraph) recipeCost(recipeID int64, visiting map[int64]bool) float64 {
	if v, ok := g.recCache[recipeID]; ok {
		return v
	}
	var total float64
	for _, item := range g.recipes[recipeID] {
		ing := g.ingredients[item.IngredientID]
		total += item.QtyBase * g.ingredientCost(item.IngredientID, visiting) * (1 + ing.WastePct/100)
	}
	// no cachear si estamos dentro de una recursión con visiting activo evitaría reuso;
	// el visiting solo protege ciclos, así que cachear el resultado final es correcto.
	g.recCache[recipeID] = total
	return total
}

// RecipeCost returns the total cost of a recipe.
func (g *CostGraph) RecipeCost(recipeID int64) float64 {
	return g.recipeCost(recipeID, map[int64]bool{})
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
		cost = g.recipeCost(*p.RecipeID, map[int64]bool{})
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
		return g.RecipeCost(*o.RecipeID)
	case o.LinkedProductID != nil:
		return g.ProductCost(*o.LinkedProductID)
	}
	return 0
}
