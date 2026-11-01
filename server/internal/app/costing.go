package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/shopspring/decimal"

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
	return s.store.WithTx(ctx, func(q *db.Queries) error {
		if err := q.LockCompositionEdits(ctx); err != nil {
			return err
		}
		return recomputeCosts(ctx, q)
	})
}

// RecomputeCompanies recalcula cada empresa en su propia conexión y bajo su RLS: al arrancar deja al
// día lo que quedó mal antes de que guardar una receta o corregir un insumo recalculara (issue #145),
// y lo que cualquier script haya dejado después. Una empresa que falla no detiene a las demás.
func (s *CostingService) RecomputeCompanies(ctx context.Context, companyIDs []int64) error {
	var errs []error
	for _, id := range companyIDs {
		tctx, release, err := s.store.AcquireTenant(ctx, id)
		if err == nil {
			err = s.RecomputeAll(tctx)
			release()
		}
		if err != nil {
			errs = append(errs, fmt.Errorf("empresa %d: %w", id, err))
		}
	}
	return errors.Join(errs...)
}

// recomputeCosts es RecomputeAll dentro de una transacción ajena: quien cambia algo de lo que sale un
// costo (una compra, la receta de un producto, de un extra o de un preparado, el costo o la unidad de
// un insumo) recalcula en la MISMA transacción, para que nadie lea un costo hecho con lo de antes.
//
// Necesita el candado de captura de la empresa (LockCompositionEdits): cada recálculo lee el catálogo
// entero, y dos a la vez escribían cada uno su resultado sin el cambio del otro, que aún no se
// confirmaba. El orden es siempre candado y después filas: quien no tiene filas tomadas lo pide al
// empezar su transacción y aquí ya lo tiene. Quien llega con filas tomadas (una compra, con el
// almacén del insumo) no lo espera: si está ocupado responde «vuelve a intentar», como cualquier otro
// candado de la compra. Esperarlo así se cruzaba con una captura que necesita esas filas.
func recomputeCosts(ctx context.Context, q *db.Queries) error {
	locked, err := q.TryLockCompositionEdits(ctx)
	if err != nil {
		return err
	}
	if !locked {
		return fmt.Errorf("%w: se están recalculando los costos; vuelve a intentar en un momento", domain.ErrConflict)
	}
	g, ings, prods, opts, err := loadGraph(ctx, q)
	if err != nil {
		return err
	}
	var batch costBatch
	for _, ing := range ings {
		if ing.IsPrep { // la materia prima no se deriva: su costo es el de compra o el corregido
			batch.add(ctx, "ingredient", ing.ID, g.IngredientCost(ing.ID), domain.MaxIngredientCost)
		}
	}
	if err := q.SetPrepIngredientCosts(ctx, db.SetPrepIngredientCostsParams{Ids: batch.ids, Costs: batch.costs}); err != nil {
		return err
	}
	batch = costBatch{}
	for _, p := range prods {
		batch.add(ctx, "product", p.ID, g.ProductCost(p.ID), domain.MaxProductCost)
	}
	if err := q.SetProductCosts(ctx, db.SetProductCostsParams{Ids: batch.ids, Costs: batch.costs}); err != nil {
		return err
	}
	batch = costBatch{}
	for _, o := range opts {
		batch.add(ctx, "option", o.ID, g.OptionCost(o.ID), domain.MaxOptionCost)
	}
	return q.SetOptionCosts(ctx, db.SetOptionCostsParams{Ids: batch.ids, Costs: batch.costs})
}

// costBatch junta los costos de una tabla para escribirlos en una sentencia. Uno que no cabe en su
// columna se deja como estaba y se avisa en el log: tumbar el lote dejaría a toda la empresa sin
// poder guardar recetas por una sola mal capturada.
type costBatch struct {
	ids   []int64
	costs []string
}

func (b *costBatch) add(ctx context.Context, kind string, id int64, cost, limit float64) {
	d, ok := domain.StorableCost(cost, limit)
	if !ok {
		slog.WarnContext(ctx, "cost_out_of_range", "kind", kind, "id", id, "cost", cost)
		return
	}
	b.ids = append(b.ids, id)
	b.costs = append(b.costs, d.String())
}

// ProductCost devuelve el costo calculado de un producto (sin persistir), para /costing.
func (s *CostingService) ProductCost(ctx context.Context, productID int64) (decimal.Decimal, error) {
	g, _, _, _, err := loadGraph(ctx, s.store.QC(ctx))
	if err != nil {
		return decimal.Zero, err
	}
	return decimal.NewFromFloat(g.ProductCost(productID)).Round(4), nil
}

func loadGraph(ctx context.Context, q *db.Queries) (
	*domain.CostGraph,
	[]db.ListIngredientsForCostingRow,
	[]db.ListProductsForCostingRow,
	[]db.ListModifierOptionsForCostingRow,
	error,
) {
	ingRows, err := q.ListIngredientsForCosting(ctx)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	itemRows, err := q.ListRecipeItemsForCosting(ctx)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	prodRows, err := q.ListProductsForCosting(ctx)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	comboRows, err := q.ListComboSlotDefaultsForCosting(ctx)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	optRows, err := q.ListModifierOptionsForCosting(ctx)
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
