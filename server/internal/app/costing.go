package app

import (
	"context"

	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store/db"
)

// costDec convierte un costo calculado (el motor de costeo trabaja en float64: los costos
// son ESTIMACIONES analíticas de margen, no dinero facturado) a decimal para persistir en
// las columnas numeric de costo. Round4 = precisión de las columnas current_cost.
func costDec(x float64) decimal.Decimal { return decimal.NewFromFloat(x).Round(4) }

type CostingService struct {
	store *store.Store
}

func NewCostingService(s *store.Store) *CostingService { return &CostingService{store: s} }

// RecomputeAll recalcula el costo de todos los ingredientes-prep, productos y opciones
// desde las recetas, y persiste los caches current_cost en una sola transacción.
// Es idempotente y barato (~750 productos < 50ms), sirve tras importar/editar catálogo
// y como job nocturno de seguridad.
func (s *CostingService) RecomputeAll(ctx context.Context) error {
	g, ings, prods, opts, err := s.loadGraph(ctx)
	if err != nil {
		return err
	}

	return s.store.WithTx(ctx, func(q *db.Queries) error {
		for _, ing := range ings {
			if !ing.IsPrep {
				continue // materia prima: su costo no se deriva
			}
			c := costDec(g.IngredientCost(ing.ID))
			if err := q.UpdateIngredientCost(ctx, db.UpdateIngredientCostParams{ID: ing.ID, CurrentCost: c}); err != nil {
				return err
			}
		}
		for _, p := range prods {
			c := costDec(g.ProductCost(p.ID))
			if err := q.UpdateProductCost(ctx, db.UpdateProductCostParams{ID: p.ID, CurrentCost: c}); err != nil {
				return err
			}
		}
		for _, o := range opts {
			c := costDec(g.OptionCost(o.ID))
			if err := q.UpdateOptionCost(ctx, db.UpdateOptionCostParams{ID: o.ID, CurrentCost: c}); err != nil {
				return err
			}
		}
		return nil
	})
}

// ProductCost devuelve el costo calculado de un producto (sin persistir), para /costing.
func (s *CostingService) ProductCost(ctx context.Context, productID int64) (decimal.Decimal, error) {
	g, _, _, _, err := s.loadGraph(ctx)
	if err != nil {
		return decimal.Zero, err
	}
	return costDec(g.ProductCost(productID)), nil
}

func (s *CostingService) loadGraph(ctx context.Context) (
	*domain.CostGraph,
	[]db.ListIngredientsForCostingRow,
	[]db.ListProductsForCostingRow,
	[]db.ListModifierOptionsForCostingRow,
	error,
) {
	ingRows, err := s.store.QC(ctx).ListIngredientsForCosting(ctx)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	itemRows, err := s.store.QC(ctx).ListRecipeItemsForCosting(ctx)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	prodRows, err := s.store.QC(ctx).ListProductsForCosting(ctx)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	comboRows, err := s.store.QC(ctx).ListComboSlotDefaultsForCosting(ctx)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	optRows, err := s.store.QC(ctx).ListModifierOptionsForCosting(ctx)
	if err != nil {
		return nil, nil, nil, nil, err
	}

	ingredients := make(map[int64]domain.CostIngredient, len(ingRows))
	for _, r := range ingRows {
		ingredients[r.ID] = domain.CostIngredient{
			ID:          r.ID,
			IsPrep:      r.IsPrep,
			RecipeID:    r.RecipeID,
			YieldQty:    numF(r.YieldQty),
			WastePct:    r.WastePct.InexactFloat64(),
			CurrentCost: r.CurrentCost.InexactFloat64(),
		}
	}
	recipes := map[int64][]domain.CostRecipeItem{}
	for _, r := range itemRows {
		recipes[r.RecipeID] = append(recipes[r.RecipeID], domain.CostRecipeItem{
			IngredientID: r.IngredientID, QtyBase: r.QtyBase.InexactFloat64(),
		})
	}
	products := make(map[int64]domain.CostProduct, len(prodRows))
	for _, r := range prodRows {
		products[r.ID] = domain.CostProduct{
			ID:         r.ID,
			Type:       string(r.Type),
			CostSource: string(r.CostSource),
			ManualCost: numF(r.ManualCost),
			RecipeID:   r.RecipeID,
		}
	}
	comboDefaults := map[int64][]domain.ComboDefault{}
	for _, r := range comboRows {
		comboDefaults[r.ComboID] = append(comboDefaults[r.ComboID], domain.ComboDefault{
			ProductID: r.ProductID, MinSelect: int(r.MinSelect),
		})
	}
	options := make(map[int64]domain.CostOption, len(optRows))
	for _, r := range optRows {
		options[r.ID] = domain.CostOption{ID: r.ID, RecipeID: r.RecipeID, LinkedProductID: r.LinkedProductID}
	}

	g := domain.NewCostGraph(ingredients, recipes, products, comboDefaults, options)
	return g, ingRows, prodRows, optRows, nil
}

// numF convierte un numeric nullable a float64 (0 si es null). El motor de costeo trabaja en
// float64 a propósito (es un estimado con tolerancia de centavos, ver domain/costing.go); el
// dinero cobrado sí es decimal exacto.
func numF(n *decimal.Decimal) float64 {
	if n == nil {
		return 0
	}
	return n.InexactFloat64()
}
