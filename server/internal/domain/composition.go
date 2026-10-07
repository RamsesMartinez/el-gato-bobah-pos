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

// PackageComponent es un producto dentro de un paquete, con cuántas piezas lleva.
type PackageComponent struct {
	ProductID int64
	Qty       int
}

// MaxPackageQty acota cuántas piezas de un producto lleva un paquete: el hueco se guarda en un
// smallint, y un paquete de mil crepas es un dedazo, no un paquete.
const MaxPackageQty = 99

// CompositionInput es lo que se captura de un producto o un extra: insumos, el producto del
// catálogo que es, o los productos que lleva un paquete. Vacía quita la composición.
type CompositionInput struct {
	Items           []CompositionItem
	LinkedProductID *int64
	Components      []PackageComponent
	// Yield es cuánto rinde la receta de un insumo preparado, en la unidad base del insumo.
	Yield *decimal.Decimal
}

// Validate rechaza lo que el almacén no podría descontar bien: dos formas de composición a la vez,
// cantidades fuera de rango —juzgadas ya redondeadas, que es como se guardan: 0.00003 se vuelve 0—,
// un insumo o un producto dos veces y una unidad que no convierte a la del insumo: gramos de algo
// que se lleva en litros descontarían un número sin sentido.
func (c CompositionInput) Validate() error {
	kinds := 0
	for _, has := range []bool{len(c.Items) > 0, c.LinkedProductID != nil, len(c.Components) > 0} {
		if has {
			kinds++
		}
	}
	if kinds > 1 {
		return fmt.Errorf("%w: lleva insumos, es un producto o es un paquete; solo una de las tres", ErrValidation)
	}
	// Un insumo preparado sin rendimiento no se puede descontar en proporción: rinde 1000 ml con
	// 500 g de azúcar, y sin el 1000 no se sabe cuánta azúcar lleva un ml.
	if c.Yield != nil {
		if len(c.Items) == 0 {
			return fmt.Errorf("%w: el rendimiento va con los insumos que lo componen", ErrValidation)
		}
		if !Round4(*c.Yield).IsPositive() || !ValidQty(*c.Yield, MaxStockQty, false) {
			return fmt.Errorf("%w: rendimiento inválido", ErrValidation)
		}
	}
	seen := map[int64]bool{}
	for _, it := range c.Items {
		if !Round4(it.Quantity).IsPositive() || !ValidQty(it.Quantity, MaxStockQty, false) {
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
	seenProduct := map[int64]bool{}
	for _, pc := range c.Components {
		if pc.Qty < 1 || pc.Qty > MaxPackageQty {
			return fmt.Errorf("%w: cantidad inválida (producto %d)", ErrValidation, pc.ProductID)
		}
		if seenProduct[pc.ProductID] {
			return fmt.Errorf("%w: el producto %d va dos veces", ErrValidation, pc.ProductID)
		}
		seenProduct[pc.ProductID] = true
	}
	return nil
}

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
	comps []SaleComponent
	// memo y visiting son de bases: lo ya descompuesto, y lo que se está descomponiendo ahora (un
	// insumo que reaparece en su propio camino es un ciclo).
	memo     map[int64][]baseLine
	visiting map[int64]bool
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
	e := &expansion{g: g, sums: map[deltaKey]decimal.Decimal{}, memo: map[int64][]baseLine{}, visiting: map[int64]bool{}}
	if p, ok := g.Products[line.ProductID]; ok {
		e.product(p, line.Qty, 0)
	}
	for _, o := range line.Options {
		opt, ok := g.Options[o.OptionID]
		if !ok {
			continue
		}
		qty := line.Qty.Mul(o.Qty).Round(internalScale)
		switch {
		case opt.LinkedProductID != nil:
			if lp, ok := g.Products[*opt.LinkedProductID]; ok {
				e.product(lp, qty, opt.ID)
			}
		case opt.RecipeID != nil:
			e.recipe(*opt.RecipeID, qty, opt.ID, 0)
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
	return out, e.comps
}

// product descuenta un producto vendido, suelto o como extra. Un paquete descuenta lo que lleva —el
// producto por omisión de cada hueco por max(min_select, 1), la regla del costeo— y anota cada
// componente para el historial.
func (e *expansion) product(p StockProduct, qty decimal.Decimal, option int64) {
	if !p.IsCombo {
		e.single(p, qty, option, 0)
		return
	}
	for _, c := range e.g.ComboDefaults[p.ID] {
		cq := qty.Mul(decimal.NewFromInt(int64(max(c.MinSelect, 1)))).Round(internalScale)
		e.comps = append(e.comps, SaleComponent{ProductID: c.ProductID, Qty: cq})
		// Un paquete dentro de un paquete no se expande: la captura lo prohíbe y un dato viejo no
		// debe multiplicar el descuento.
		if cp, ok := e.g.Products[c.ProductID]; ok && !cp.IsCombo {
			e.single(cp, cq, option, p.ID)
		}
	}
}

func (e *expansion) single(p StockProduct, qty decimal.Decimal, option, component int64) {
	switch {
	case p.TrackStock:
		e.add("producto", p.ID, qty, option, component)
	case p.RecipeID != nil:
		e.recipe(*p.RecipeID, qty, option, component)
	}
}

// internalScale acota los decimales de cada paso. La base entrega recetas con 6 decimales y cada
// nivel los suma: sin acotar, un insumo preparado dentro de otro pasaba el tope de escala de
// ValidQty y la venta tronaba. 12 decimales sobran para redondear al final a 4.
const internalScale = 12

func (e *expansion) recipe(recipeID int64, qty decimal.Decimal, option, component int64) {
	for _, it := range e.g.Recipes[recipeID] {
		lineQty := it.QtyBase.Mul(qty).Round(internalScale)
		for _, b := range e.bases(it.IngredientID) {
			e.add("ingrediente", b.ingredientID, lineQty.Mul(b.perUnit).Round(internalScale), option, component)
		}
	}
}

// baseLine es cuánto de un insumo que se compra lleva UNA unidad de otro insumo.
type baseLine struct {
	ingredientID int64
	perUnit      decimal.Decimal
}

// bases descompone un insumo en lo que se compra, por unidad. Uno preparado se descompone en lo que
// lo compone en proporción a su rendimiento; si no se puede —sin rendimiento, o un ciclo de datos
// viejos— se descuenta él mismo: mejor un insumo preparado en negativo que una venta que no
// descuenta nada.
//
// Recuerda cada insumo ya descompuesto. Sin eso el recorrido seguía cada camino del árbol, y unas
// capas de insumos que se llevan entre sí multiplicaban los caminos hasta tardar minutos por renglón.
// Así es lineal en insumos y renglones de receta.
func (e *expansion) bases(id int64) []baseLine {
	if b, ok := e.memo[id]; ok {
		return b
	}
	in := e.g.Ingredients[id]
	if !in.IsPrep || in.RecipeID == nil || !in.YieldQty.IsPositive() || e.visiting[id] {
		return []baseLine{{ingredientID: id, perUnit: decimal.NewFromInt(1)}}
	}
	e.visiting[id] = true
	var out []baseLine
	pos := map[int64]int{}
	for _, it := range e.g.Recipes[*in.RecipeID] {
		factor := it.QtyBase.Div(in.YieldQty).Round(internalScale)
		for _, b := range e.bases(it.IngredientID) {
			q := factor.Mul(b.perUnit).Round(internalScale)
			if i, ok := pos[b.ingredientID]; ok {
				out[i].perUnit = out[i].perUnit.Add(q)
				continue
			}
			pos[b.ingredientID] = len(out)
			out = append(out, baseLine{ingredientID: b.ingredientID, perUnit: q})
		}
	}
	delete(e.visiting, id)
	e.memo[id] = out
	return out
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
