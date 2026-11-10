package app

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store/db"
)

// PurchasePreview es lo que dejaría una compra con mercancía, sin guardarla (spec 073, R2).
type PurchasePreview struct {
	CostingMethod string                `json:"costingMethod"`
	Lines         []PurchasePreviewLine `json:"lines"`
	Costs         []PurchasePreviewCost `json:"costs"`
}

// PurchasePreviewLine es un renglón de almacén: cuánto entra en la unidad base y a cuánto sale.
type PurchasePreviewLine struct {
	Position     int32           `json:"position"`
	IngredientID *int64          `json:"ingredientId"`
	ProductID    *int64          `json:"productId"`
	Name         string          `json:"name"`
	BaseQty      decimal.Decimal `json:"baseQty"`
	UnitCost     decimal.Decimal `json:"unitCost"`
}

// PurchasePreviewCost es el costo vigente de un insumo antes de la compra y el que deja la compra
// COMPLETA: con el mismo insumo en dos renglones, un costo por renglón engañaría (revisión A3).
type PurchasePreviewCost struct {
	IngredientID        int64           `json:"ingredientId"`
	Name                string          `json:"name"`
	BaseUnitCode        string          `json:"baseUnitCode"`
	CostBefore          decimal.Decimal `json:"costBefore"`
	CostAfter           decimal.Decimal `json:"costAfter"`
	KeptByNewerPurchase bool            `json:"keptByNewerPurchase"`
}

// PreviewPurchase corre la captura del gasto completa y la revierte.
//
// Es la MISMA función que guarda (prepareExpense + createExpenseIn): la regla de costeo, la de las
// compras capturadas tarde, la conversión con empaque y densidad y cada validación son las de la
// compra de verdad, no una copia que se quede atrás. Tres cosas que no se ven:
//   - Los constraints diferidos (la guarda de insumos unidos, 0104) corren en el commit, y una
//     transacción revertida nunca llega ahí: se fuerzan antes de revertir.
//   - Toma los mismos candados que la compra (insumos y existencias sin esperar, la caja).
//     lock_timeout de 2 s acota lo que la vista previa espera; una venta que llega después la espera
//     a ella mientras dure la transacción, que es corta y la ruta limita por usuario.
//   - No da de alta artículos: devolvería el id de una fila que se revierte.
func (s *BackofficeService) PreviewPurchase(ctx context.Context, in ExpenseInput) (PurchasePreview, error) {
	for _, it := range in.Items {
		if it.NewArticle != nil {
			return PurchasePreview{}, fmt.Errorf("%w: la vista previa no da de alta artículos; búscalo o créalo antes", domain.ErrValidation)
		}
	}
	prep, err := s.prepareExpense(ctx, in)
	if err != nil {
		return PurchasePreview{}, err
	}
	out := PurchasePreview{Lines: []PurchasePreviewLine{}, Costs: []PurchasePreviewCost{}}
	err = s.store.WithTx(ctx, func(q *db.Queries) error {
		if err := q.SetPreviewLockTimeout(ctx); err != nil {
			return err
		}
		var ids []int64
		for _, it := range in.Items {
			if it.IngredientID != nil && !slices.Contains(ids, *it.IngredientID) {
				ids = append(ids, *it.IngredientID)
			}
		}
		slices.Sort(ids)
		before, err := q.PurchasePreviewArticles(ctx, ids)
		if err != nil {
			return err
		}
		method, err := q.CostingMethod(ctx)
		if err != nil {
			return err
		}
		out.CostingMethod = method
		day := prep.received
		if !prep.hasReceived {
			day = s.businessDate(ctx)
		}
		kept := map[int64]bool{}
		if domain.CostingMethod(method) == domain.CostingLast {
			for _, id := range ids {
				newest, err := q.NewestPurchaseArrival(ctx, id)
				if err != nil {
					return err
				}
				kept[id] = prep.hasReceived && newest.Valid && !domain.LastPriceApplies(day, &newest.Time)
			}
		}

		id, err := s.createExpenseIn(ctx, q, prep)
		if err != nil {
			return err
		}
		if err := q.CheckDeferredConstraintsNow(ctx); err != nil {
			return store.DomainError(err)
		}
		rows, err := q.ListStockLines(ctx, id)
		if err != nil {
			return err
		}
		states := stockStates(rows)
		for i, r := range rows {
			if r.Excluded {
				continue
			}
			base, err := domain.BaseQty(r.Quantity, states[i].Unit)
			if err != nil {
				return err
			}
			out.Lines = append(out.Lines, PurchasePreviewLine{Position: r.Position, IngredientID: r.IngredientID,
				ProductID: r.ProductID, Name: r.Description, BaseQty: base, UnitCost: domain.UnitCost(r.Amount, base)})
		}
		after, err := q.PurchasePreviewArticles(ctx, ids)
		if err != nil {
			return err
		}
		costAfter := map[int64]decimal.Decimal{}
		for _, a := range after {
			costAfter[a.ID] = a.CurrentCost
		}
		for _, b := range before {
			out.Costs = append(out.Costs, PurchasePreviewCost{IngredientID: b.ID, Name: b.Name, BaseUnitCode: b.BaseUnitCode,
				CostBefore: b.CurrentCost, CostAfter: costAfter[b.ID], KeptByNewerPurchase: kept[b.ID]})
		}
		return errPreviewRollback
	})
	if err != nil && !errors.Is(err, errPreviewRollback) {
		return PurchasePreview{}, err
	}
	return out, nil
}
