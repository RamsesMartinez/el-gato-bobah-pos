package domain

import "github.com/shopspring/decimal"

// ExpectedWasteOfSale es lo que la merma esperada saca del almacén por un renglón vendido, cuando la
// empresa enciende «la merma esperada también descuenta del almacén» (spec 096, subida 2). Sigue la
// misma regla que el costo (ResolveExpectedWaste): artículo → producto → general en la receta de un
// producto, artículo → general en la de un extra, solo el % de cada artículo dentro de un preparado, y
// un empaque solo con el suyo. Lo que sale es, por insumo y origen, lo que la venta gasta con merma
// menos lo que gasta sin ella: la parte de la venta la sigue registrando ExpandSale.
func (g StockGraph) ExpectedWasteOfSale(line SaleLine) []StockDelta {
	w := &wasteWalk{g: g, plain: map[deltaKey]decimal.Decimal{}, with: map[deltaKey]decimal.Decimal{}, visiting: map[int64]bool{}}
	if p, ok := g.Products[line.ProductID]; ok {
		w.product(p, line.Qty, 0)
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
				w.product(lp, qty, opt.ID)
			}
		case opt.RecipeID != nil:
			// Un extra no es un producto: artículo → general.
			w.recipe(*opt.RecipeID, qty, qty, opt.ID, 0, wasteFallbackDec{general: g.General})
		}
	}
	out := []StockDelta{}
	for _, k := range w.order {
		extra := Round4(w.with[k].Sub(w.plain[k]))
		if !extra.IsPositive() {
			continue
		}
		id := k.id
		d := StockDelta{ItemType: k.itemType, IngredientID: &id, Qty: extra}
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
	return out
}

type wasteFallbackDec struct{ product, general *decimal.Decimal }

// wasteWalk recorre la venta como ExpandSale, llevando dos cantidades por insumo: sin merma y con
// ella. La diferencia es lo que descuenta la merma esperada.
type wasteWalk struct {
	g        StockGraph
	order    []deltaKey
	plain    map[deltaKey]decimal.Decimal
	with     map[deltaKey]decimal.Decimal
	visiting map[int64]bool
}

func (w *wasteWalk) product(p StockProduct, qty decimal.Decimal, option int64) {
	if !p.IsCombo {
		w.single(p, qty, option, 0)
		return
	}
	for _, c := range w.g.ComboDefaults[p.ID] {
		cq := qty.Mul(decimal.NewFromInt(int64(max(c.MinSelect, 1)))).Round(internalScale)
		if cp, ok := w.g.Products[c.ProductID]; ok && !cp.IsCombo {
			w.single(cp, cq, option, p.ID)
		}
	}
}

// single: un producto con existencia propia sale entero por la venta; la merma es de lo que se prepara.
func (w *wasteWalk) single(p StockProduct, qty decimal.Decimal, option, component int64) {
	if p.TrackStock || p.RecipeID == nil {
		return
	}
	w.recipe(*p.RecipeID, qty, qty, option, component, wasteFallbackDec{product: p.ExpectedWastePct, general: w.g.General})
}

func (w *wasteWalk) recipe(recipeID int64, plainQty, withQty decimal.Decimal, option, component int64, fb wasteFallbackDec) {
	for _, it := range w.g.Recipes[recipeID] {
		in := w.g.Ingredients[it.IngredientID]
		pct := resolveWasteDec(in, fb)
		lp := it.QtyBase.Mul(plainQty).Round(internalScale)
		lw := it.QtyBase.Mul(withQty).Mul(decimal.NewFromInt(1).Add(pct.Div(decimal.NewFromInt(100)))).Round(internalScale)
		w.ingredient(it.IngredientID, lp, lw, option, component)
	}
}

// ingredient descompone un preparado en lo que lo compone, con su rendimiento y SIN fallback: dentro
// del preparado solo manda el % de cada artículo (R2). Lo demás se suma tal cual.
func (w *wasteWalk) ingredient(id int64, lp, lw decimal.Decimal, option, component int64) {
	in := w.g.Ingredients[id]
	if in.IsPrep && in.RecipeID != nil && in.YieldQty.IsPositive() && !w.visiting[id] {
		w.visiting[id] = true
		w.recipe(*in.RecipeID, lp.Div(in.YieldQty).Round(internalScale), lw.Div(in.YieldQty).Round(internalScale), option, component, wasteFallbackDec{})
		delete(w.visiting, id)
		return
	}
	k := deltaKey{"ingrediente", id, option, component}
	if _, ok := w.plain[k]; !ok {
		w.order = append(w.order, k)
	}
	w.plain[k] = w.plain[k].Add(lp)
	w.with[k] = w.with[k].Add(lw)
}

// resolveWasteDec es ResolveExpectedWaste con la regla de los empaques, en decimal.
func resolveWasteDec(in StockIngredient, fb wasteFallbackDec) decimal.Decimal {
	if in.ExpectedWastePct != nil {
		return *in.ExpectedWastePct
	}
	if in.IsPackaging {
		return decimal.Zero
	}
	if fb.product != nil {
		return *fb.product
	}
	if fb.general != nil {
		return *fb.general
	}
	return decimal.Zero
}
