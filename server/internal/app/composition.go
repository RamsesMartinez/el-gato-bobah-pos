package app

import (
	"context"

	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store/db"
)

// loadStockGraph carga la composición de la empresa de la sesión (RLS) para expandir ventas.
//
// ponytail: carga el catálogo completo en cada venta —cinco consultas de unos cientos de filas—.
// Si el catálogo crece a miles o la venta se vuelve lenta, se carga solo lo alcanzable desde los
// productos y opciones del pedido.
func loadStockGraph(ctx context.Context, q *db.Queries) (domain.StockGraph, error) {
	g := domain.StockGraph{
		Ingredients:   map[int64]domain.StockIngredient{},
		Recipes:       map[int64][]domain.RecipeLine{},
		Products:      map[int64]domain.StockProduct{},
		ComboDefaults: map[int64][]domain.ComboDefault{},
		Options:       map[int64]domain.StockOption{},
	}
	ings, err := q.ListIngredientsForCosting(ctx)
	if err != nil {
		return g, err
	}
	for _, r := range ings {
		in := domain.StockIngredient{ID: r.ID, IsPrep: r.IsPrep, RecipeID: r.RecipeID}
		if r.YieldQty != nil {
			in.YieldQty = *r.YieldQty
		}
		g.Ingredients[r.ID] = in
	}
	items, err := q.ListRecipeItemsForCosting(ctx)
	if err != nil {
		return g, err
	}
	for _, r := range items {
		g.Recipes[r.RecipeID] = append(g.Recipes[r.RecipeID], domain.RecipeLine{IngredientID: r.IngredientID, QtyBase: r.QtyBase})
	}
	prods, err := q.ListProductsForStock(ctx)
	if err != nil {
		return g, err
	}
	for _, r := range prods {
		g.Products[r.ID] = domain.StockProduct{ID: r.ID, IsCombo: r.Type == db.ProductTypeCombo, TrackStock: r.TrackStock, RecipeID: r.RecipeID}
	}
	slots, err := q.ListComboSlotDefaultsForCosting(ctx)
	if err != nil {
		return g, err
	}
	for _, r := range slots {
		g.ComboDefaults[r.ComboID] = append(g.ComboDefaults[r.ComboID], domain.ComboDefault{ProductID: r.ProductID, MinSelect: int(r.MinSelect)})
	}
	opts, err := q.ListModifierOptionsForCosting(ctx)
	if err != nil {
		return g, err
	}
	for _, r := range opts {
		g.Options[r.ID] = domain.StockOption{ID: r.ID, RecipeID: r.RecipeID, LinkedProductID: r.LinkedProductID}
	}
	return g, nil
}

// saleLineOf arma lo que la expansión necesita de un renglón ya valuado.
func saleLineOf(l domain.BuiltLine) domain.SaleLine {
	sl := domain.SaleLine{ProductID: l.ProductID, Qty: l.Qty}
	for _, m := range l.Modifiers {
		sl.Options = append(sl.Options, domain.SaleOption{OptionID: m.OptionID, Qty: decimal.NewFromInt(int64(m.Qty))})
	}
	return sl
}
