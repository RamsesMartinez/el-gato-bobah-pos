package app

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store/db"
)

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
			c := domain.Round2(g.IngredientCost(ing.ID))
			if err := q.UpdateIngredientCost(ctx, db.UpdateIngredientCostParams{ID: ing.ID, CurrentCost: c}); err != nil {
				return err
			}
		}
		for _, p := range prods {
			c := domain.Round2(g.ProductCost(p.ID))
			if err := q.UpdateProductCost(ctx, db.UpdateProductCostParams{ID: p.ID, CurrentCost: c}); err != nil {
				return err
			}
		}
		for _, o := range opts {
			c := domain.Round2(g.OptionCost(o.ID))
			if err := q.UpdateOptionCost(ctx, db.UpdateOptionCostParams{ID: o.ID, CurrentCost: c}); err != nil {
				return err
			}
		}
		return nil
	})
}

// ProductCost devuelve el costo calculado de un producto (sin persistir), para /costing.
func (s *CostingService) ProductCost(ctx context.Context, productID int64) (float64, error) {
	g, _, _, _, err := s.loadGraph(ctx)
	if err != nil {
		return 0, err
	}
	return domain.Round2(g.ProductCost(productID)), nil
}

func (s *CostingService) loadGraph(ctx context.Context) (
	*domain.CostGraph,
	[]db.ListIngredientsForCostingRow,
	[]db.ListProductsForCostingRow,
	[]db.ListModifierOptionsForCostingRow,
	error,
) {
	ingRows, err := s.store.Q.ListIngredientsForCosting(ctx)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	itemRows, err := s.store.Q.ListRecipeItemsForCosting(ctx)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	prodRows, err := s.store.Q.ListProductsForCosting(ctx)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	comboRows, err := s.store.Q.ListComboSlotDefaultsForCosting(ctx)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	optRows, err := s.store.Q.ListModifierOptionsForCosting(ctx)
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
			WastePct:    r.WastePct,
			CurrentCost: r.CurrentCost,
		}
	}
	recipes := map[int64][]domain.CostRecipeItem{}
	for _, r := range itemRows {
		recipes[r.RecipeID] = append(recipes[r.RecipeID], domain.CostRecipeItem{
			IngredientID: r.IngredientID, QtyBase: r.QtyBase,
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

// numF convierte un pgtype.Numeric (nullable) a float64 (0 si es null).
func numF(n pgtype.Numeric) float64 {
	if !n.Valid {
		return 0
	}
	f, err := n.Float64Value()
	if err != nil || !f.Valid {
		return 0
	}
	return f.Float64
}
