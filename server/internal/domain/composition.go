package domain

import (
	"fmt"

	"github.com/shopspring/decimal"
)

// Qué sale del almacén cuando se vende algo (spec 028).
//
// Es el mismo grafo que recorre el costeo (CostGraph), con una diferencia de fondo: aquí la
// cantidad es dinero del inventario y va en decimal, no en float64. Un paquete se expande con la
// misma regla que lo costea —el producto por omisión de cada hueco por max(min_select, 1)— para que
// lo que el almacén dice que salió y lo que el costeo dice que costó hablen de la misma cosa.

// ErrCompositionCycle y ErrPackageInPackage son de validación: responden 422 como cualquier
// ErrValidation, y se distinguen para el mensaje.
var (
	ErrCompositionCycle   = fmt.Errorf("%w: la composición se contiene a sí misma", ErrValidation)
	ErrPackageInPackage   = fmt.Errorf("%w: un paquete no puede llevar otro paquete", ErrValidation)
	ErrRecipeUnitMismatch = fmt.Errorf("%w: la unidad no es del tipo del insumo", ErrValidation)
)

// CompositionItem es un renglón capturado: insumo, cantidad y la unidad en que se capturó. Los
// tipos de unidad (masa, volumen, pieza) los llena quien carga los datos.
type CompositionItem struct {
	IngredientID       int64
	Quantity           decimal.Decimal
	UnitID             int16
	UnitKind           string
	IngredientUnitKind string
}

// CompositionInput es lo que se captura de un producto o un extra: insumos, o el producto del
// catálogo que es. Vacía quita la composición.
type CompositionInput struct {
	Items           []CompositionItem
	LinkedProductID *int64
}

// Validate rechaza lo que el almacén no podría descontar bien: cantidades fuera de rango, un insumo
// dos veces (la receta lo guarda una vez por insumo) y una unidad que no convierte a la del insumo:
// gramos de algo que se lleva en litros descontarían un número sin sentido.
func (c CompositionInput) Validate() error {
	if c.LinkedProductID != nil && len(c.Items) > 0 {
		return fmt.Errorf("%w: lleva insumos o es un producto, no las dos cosas", ErrValidation)
	}
	seen := map[int64]bool{}
	for _, it := range c.Items {
		if !it.Quantity.IsPositive() || !ValidQty(it.Quantity, MaxStockQty, false) {
			return fmt.Errorf("%w: cantidad inválida (insumo %d)", ErrValidation, it.IngredientID)
		}
		if seen[it.IngredientID] {
			return fmt.Errorf("%w: el insumo %d va dos veces", ErrValidation, it.IngredientID)
		}
		seen[it.IngredientID] = true
		if it.UnitKind != it.IngredientUnitKind {
			return fmt.Errorf("%w (insumo %d)", ErrRecipeUnitMismatch, it.IngredientID)
		}
	}
	return nil
}

// maxCompositionDepth corta la descomposición de insumos preparados. La captura rechaza los
// ciclos, pero un dato viejo no pasó por ella, y una venta no puede colgarse por eso.
const maxCompositionDepth = 8

// StockIngredient es un insumo; si es preparado, se descuenta en lo que lo compone.
type StockIngredient struct {
	ID       int64
	IsPrep   bool
	RecipeID *int64
	YieldQty decimal.Decimal
}

// RecipeLine es un renglón de receta en unidad base.
type RecipeLine struct {
	IngredientID int64
	QtyBase      decimal.Decimal
}

// StockProduct es lo mínimo de un producto para saber qué descuenta.
type StockProduct struct {
	ID         int64
	IsCombo    bool
	TrackStock bool
	RecipeID   *int64
}

// StockOption es un extra: lleva receta o es otro producto.
type StockOption struct {
	ID              int64
	RecipeID        *int64
	LinkedProductID *int64
}

// StockGraph es la composición de una empresa, cargada para expandir ventas.
type StockGraph struct {
	Ingredients   map[int64]StockIngredient
	Recipes       map[int64][]RecipeLine
	Products      map[int64]StockProduct
	ComboDefaults map[int64][]ComboDefault
	Options       map[int64]StockOption
}

// SaleOption es un extra elegido; Qty es por unidad del renglón.
type SaleOption struct {
	OptionID int64
	Qty      decimal.Decimal
}

// SaleLine es un renglón vendido.
type SaleLine struct {
	ProductID int64
	Qty       decimal.Decimal
	Options   []SaleOption
}

// StockDelta es una cantidad que sale del almacén, positiva, con su origen: el extra que la
// originó o el paquete del que salió el componente.
type StockDelta struct {
	ItemType     string // "ingrediente" | "producto", como stock_movements.item_type
	IngredientID *int64
	ProductID    *int64
	Qty          decimal.Decimal
	OptionID     *int64
	ComponentOf  *int64
}

// SaleComponent es un producto vendido dentro de un paquete, para el historial.
type SaleComponent struct {
	ProductID int64
	Qty       decimal.Decimal
}

// StockQty redondea una cantidad de almacén a 4 decimales sin dejar en cero una positiva: un
// gramo de algo vendido muchas veces no puede desaparecer del libro.
func StockQty(v decimal.Decimal) decimal.Decimal {
	r := Round4(v)
	if v.IsPositive() && !r.IsPositive() {
		return decimal.New(1, -4)
	}
	return r
}

type deltaKey struct {
	itemType  string
	id        int64
	option    int64
	component int64
}

type expansion struct {
	g     StockGraph
	order []deltaKey
	sums  map[deltaKey]decimal.Decimal
}

func (e *expansion) add(itemType string, id int64, qty decimal.Decimal, option, component int64) {
	k := deltaKey{itemType, id, option, component}
	if _, ok := e.sums[k]; !ok {
		e.order = append(e.order, k)
	}
	e.sums[k] = e.sums[k].Add(qty)
}

// ExpandSale dice qué sale del almacén por un renglón, agrupado por insumo o producto y origen, y
// qué productos se vendieron dentro si es un paquete. Lo que no tiene composición no descuenta.
func (g StockGraph) ExpandSale(line SaleLine) ([]StockDelta, []SaleComponent) {
	e := &expansion{g: g, sums: map[deltaKey]decimal.Decimal{}}
	var comps []SaleComponent
	if p, ok := g.Products[line.ProductID]; ok && p.IsCombo {
		for _, c := range g.ComboDefaults[p.ID] {
			qty := line.Qty.Mul(decimal.NewFromInt(int64(max(c.MinSelect, 1))))
			comps = append(comps, SaleComponent{ProductID: c.ProductID, Qty: qty})
			// Un paquete dentro de un paquete no se expande: la captura lo prohíbe y un dato viejo
			// no debe multiplicar el descuento.
			if cp, ok := g.Products[c.ProductID]; ok && !cp.IsCombo {
				e.product(cp, qty, 0, p.ID)
			}
		}
	} else if ok {
		e.product(p, line.Qty, 0, 0)
	}
	for _, o := range line.Options {
		opt, ok := g.Options[o.OptionID]
		if !ok {
			continue
		}
		qty := line.Qty.Mul(o.Qty)
		switch {
		case opt.LinkedProductID != nil:
			if lp, ok := g.Products[*opt.LinkedProductID]; ok && !lp.IsCombo {
				e.product(lp, qty, opt.ID, 0)
			}
		case opt.RecipeID != nil:
			e.recipe(*opt.RecipeID, qty, opt.ID, 0, 0)
		}
	}

	out := make([]StockDelta, 0, len(e.order))
	for _, k := range e.order {
		d := StockDelta{ItemType: k.itemType, Qty: StockQty(e.sums[k])}
		id := k.id
		if k.itemType == "producto" {
			d.ProductID = &id
		} else {
			d.IngredientID = &id
		}
		if k.option != 0 {
			o := k.option
			d.OptionID = &o
		}
		if k.component != 0 {
			c := k.component
			d.ComponentOf = &c
		}
		out = append(out, d)
	}
	return out, comps
}

func (e *expansion) product(p StockProduct, qty decimal.Decimal, option, component int64) {
	switch {
	case p.TrackStock:
		e.add("producto", p.ID, qty, option, component)
	case p.RecipeID != nil:
		e.recipe(*p.RecipeID, qty, option, component, 0)
	}
}

func (e *expansion) recipe(recipeID int64, qty decimal.Decimal, option, component int64, depth int) {
	for _, it := range e.g.Recipes[recipeID] {
		e.ingredient(it.IngredientID, it.QtyBase.Mul(qty), option, component, depth)
	}
}

// ingredient descuenta un insumo; uno preparado se descuenta en lo que lo compone, en proporción a
// su rendimiento. Si no se puede descomponer —sin rendimiento, o un ciclo de datos viejos— se
// descuenta él mismo: mejor un insumo preparado en negativo que una venta que no descuenta nada.
func (e *expansion) ingredient(id int64, qty decimal.Decimal, option, component int64, depth int) {
	in := e.g.Ingredients[id]
	if !in.IsPrep || in.RecipeID == nil || !in.YieldQty.IsPositive() || depth >= maxCompositionDepth {
		e.add("ingrediente", id, qty, option, component)
		return
	}
	e.recipe(*in.RecipeID, qty.Div(in.YieldQty), option, component, depth+1)
}

// ValidatePackage rechaza un paquete que se contiene o que lleva otro paquete.
func (g StockGraph) ValidatePackage(packageID int64, components []int64) error {
	for _, c := range components {
		if c == packageID || g.Products[c].IsCombo {
			return fmt.Errorf("%w (producto %d)", ErrPackageInPackage, c)
		}
	}
	return nil
}

// ValidatePrepIngredient rechaza componer un insumo con algo que, directa o indirectamente, lo
// lleva a él.
func (g StockGraph) ValidatePrepIngredient(ingredientID int64, components []int64) error {
	for _, c := range components {
		if g.reaches(c, ingredientID, map[int64]bool{}) {
			return fmt.Errorf("%w (insumo %d)", ErrCompositionCycle, c)
		}
	}
	return nil
}

func (g StockGraph) reaches(from, target int64, seen map[int64]bool) bool {
	if from == target {
		return true
	}
	if seen[from] {
		return false
	}
	seen[from] = true
	in := g.Ingredients[from]
	if !in.IsPrep || in.RecipeID == nil {
		return false
	}
	for _, it := range g.Recipes[*in.RecipeID] {
		if g.reaches(it.IngredientID, target, seen) {
			return true
		}
	}
	return false
}
