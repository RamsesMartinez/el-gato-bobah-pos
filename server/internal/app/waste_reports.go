package app

import (
	"context"
	"time"

	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store/db"
)

// LossItem es el faltante de un artículo en un conteo cerrado (spec 039, historia 2).
type LossItem struct {
	IngredientID int64            `json:"ingredientId"`
	Name         string           `json:"name"`
	UnitCode     string           `json:"unitCode"`
	HasPrevious  bool             `json:"hasPrevious"`
	Registered   decimal.Decimal  `json:"registered"`
	Unexplained  decimal.Decimal  `json:"unexplained"`
	Surplus      decimal.Decimal  `json:"surplus"`
	Lost         decimal.Decimal  `json:"lost"`
	UnitCost     *decimal.Decimal `json:"unitCost"`
	LostCost     *decimal.Decimal `json:"lostCost"`
	UnexplCost   *decimal.Decimal `json:"unexplainedCost"`
}

// LossTotals suma en pesos lo que tiene costo; lo que no, se cuenta aparte.
type LossTotals struct {
	Lost        decimal.Decimal `json:"lost"`
	Registered  decimal.Decimal `json:"registered"`
	Unexplained decimal.Decimal `json:"unexplained"`
	WithoutCost int             `json:"withoutCost"`
}

// CountLossView es el faltante de un conteo cerrado. `FirstCount`: ningún artículo tenía un conteo
// anterior comparable, así que no hay periodo y no se inventan cifras.
type CountLossView struct {
	FirstCount bool       `json:"firstCount"`
	Items      []LossItem `json:"items"`
	Totals     LossTotals `json:"totals"`
}

// CountLoss parte lo perdido de cada artículo contado entre lo registrado como merma y lo que el
// conteo encontró de menos. Los pesos usan el costo guardado al cerrar, así que el reporte de un
// conteo cerrado no cambia cuando cambia un costo.
func (s *BackofficeService) CountLoss(ctx context.Context, countID int64) (CountLossView, error) {
	rows, err := s.store.QC(ctx).CountLossLines(ctx, countID)
	if err != nil {
		return CountLossView{}, err
	}
	out := CountLossView{FirstCount: true, Items: make([]LossItem, 0, len(rows))}
	for _, r := range rows {
		it := LossItem{IngredientID: r.IngredientID, Name: r.Name, UnitCode: r.BaseUnitCode, HasPrevious: r.HasPrevious}
		registered := decimal.Zero
		if r.HasPrevious {
			out.FirstCount = false
			registered = r.Registered
		}
		split := domain.SplitLoss(registered, r.SystemQty, r.CountedQty)
		it.Registered, it.Unexplained, it.Surplus, it.Lost = split.Registered, split.Unexplained, split.Surplus, split.Lost
		if r.UnitCost != nil && r.UnitCost.IsPositive() {
			uc := *r.UnitCost
			lost, unexpl := domain.Round2(split.Lost.Mul(uc)), domain.Round2(split.Unexplained.Mul(uc))
			it.UnitCost, it.LostCost, it.UnexplCost = &uc, &lost, &unexpl
			if r.HasPrevious {
				out.Totals.Lost = out.Totals.Lost.Add(lost)
				out.Totals.Registered = out.Totals.Registered.Add(domain.Round2(split.Registered.Mul(uc)))
			}
			out.Totals.Unexplained = out.Totals.Unexplained.Add(unexpl)
		} else if split.Lost.IsPositive() {
			out.Totals.WithoutCost++
		}
		out.Items = append(out.Items, it)
	}
	return out, nil
}

// coverPendingWaste cierra como «la cubrió el conteo» las mermas por aprobar de la sucursal que el
// conteo vio: registradas antes de que capturara alguno de los artículos que restarían. El conteo
// ya reflejó esa pérdida; aprobarlas después la restaría dos veces. Una registrada después de la
// captura no la vio y sigue por aprobar (revisión de arquitectura).
func coverPendingWaste(ctx context.Context, q *db.Queries, countID, branch int64) error {
	pending, err := q.PendingWasteToCover(ctx, branch)
	if err != nil || len(pending) == 0 {
		return err
	}
	lines, err := q.CountedLinesSince(ctx, countID)
	if err != nil {
		return err
	}
	captured := make(map[int64]time.Time, len(lines))
	for _, l := range lines {
		captured[l.IngredientID] = l.CapturedAt
	}
	var graph *domain.StockGraph
	for _, p := range pending {
		var ings []int64
		if p.IngredientID != nil {
			ings = []int64{*p.IngredientID}
		} else if p.ProductID != nil {
			if graph == nil {
				g, err := loadStockGraph(ctx, q)
				if err != nil {
					return err
				}
				graph = &g
			}
			deltas, _ := graph.ExpandSale(domain.SaleLine{ProductID: *p.ProductID, Qty: p.Quantity})
			for _, d := range deltas {
				if d.IngredientID != nil {
					ings = append(ings, *d.IngredientID)
				}
			}
		}
		seen := false
		for _, id := range ings {
			if at, ok := captured[id]; ok && !p.RecordedAt.After(at) {
				seen = true
				break
			}
		}
		if !seen {
			continue
		}
		cid := countID
		if _, err := q.ResolveWasteRecord(ctx, db.ResolveWasteRecordParams{
			Status: string(domain.WasteCovered), CoveredByCountID: &cid, ID: p.ID, FromStatus: string(domain.WastePending),
		}); err != nil {
			return err
		}
	}
	return nil
}
