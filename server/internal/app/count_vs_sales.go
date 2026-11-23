package app

import (
	"context"

	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store/db"
)

// CountVsSalesItem es un artículo del reporte conteo contra ventas (spec 096, subida 2).
type CountVsSalesItem struct {
	IngredientID  int64            `json:"ingredientId"`
	Name          string           `json:"name"`
	UnitCode      string           `json:"unitCode"`
	HasPrevious   bool             `json:"hasPrevious"`
	Initial       decimal.Decimal  `json:"initial"`
	Entries       decimal.Decimal  `json:"entries"`
	Sold          decimal.Decimal  `json:"sold"`
	ExpectedWaste decimal.Decimal  `json:"expectedWaste"`
	Registered    decimal.Decimal  `json:"registered"`
	TransfersOut  decimal.Decimal  `json:"transfersOut"`
	System        decimal.Decimal  `json:"system"`
	Counted       decimal.Decimal  `json:"counted"`
	Diff          decimal.Decimal  `json:"diff"`
	Pct           *decimal.Decimal `json:"pct"`
	// Group: «review» (revisar receta o captura), «suggest» (merma estable: usar SuggestedPct) o
	// «small» (diferencia chica). La maqueta C agrupa por esto.
	Group        string           `json:"group"`
	SuggestedPct *decimal.Decimal `json:"suggestedPct"`
}

// CountVsSalesView es el reporte de un conteo cerrado. FirstCount: ningún artículo tenía un conteo
// anterior en su almacén, así que no hay con qué comparar.
type CountVsSalesView struct {
	FirstCount bool               `json:"firstCount"`
	Items      []CountVsSalesItem `json:"items"`
}

// CountVsSales arma el reporte de un conteo cerrado y, con los dos conteos cerrados anteriores del
// mismo almacén, decide qué artículo tiene una merma estable (sugiere un %) y cuál hay que revisar.
// Solo lee; aplicar la sugerencia es el PUT de la merma del artículo, con su bitácora.
func (s *BackofficeService) CountVsSales(ctx context.Context, countID int64) (CountVsSalesView, error) {
	q := s.store.QC(ctx)
	rows, err := q.CountVsSalesLines(ctx, countID)
	if err != nil {
		return CountVsSalesView{}, err
	}
	prev, err := q.PreviousClosedCounts(ctx, countID)
	if err != nil {
		return CountVsSalesView{}, err
	}
	history := map[int64][]*decimal.Decimal{}
	for _, p := range prev {
		pr, err := q.CountVsSalesLines(ctx, p)
		if err != nil {
			return CountVsSalesView{}, err
		}
		for _, r := range pr {
			if r.HasPrevious {
				history[r.IngredientID] = append(history[r.IngredientID], countVsSalesRow(r).Pct)
			}
		}
	}
	out := CountVsSalesView{FirstCount: true, Items: make([]CountVsSalesItem, 0, len(rows))}
	for _, r := range rows {
		row := countVsSalesRow(r)
		it := CountVsSalesItem{
			IngredientID: r.IngredientID, Name: r.Name, UnitCode: r.BaseUnitCode, HasPrevious: r.HasPrevious,
			Initial: r.InitialQty, Entries: r.Entries, Sold: r.Sold, ExpectedWaste: r.ExpectedWaste,
			Registered: r.Registered, TransfersOut: r.TransfersOut, System: r.SystemQty, Counted: r.CountedQty,
			Diff: row.Diff, Pct: row.Pct, Group: "small",
		}
		if r.HasPrevious {
			out.FirstCount = false
			pcts := append([]*decimal.Decimal{row.Pct}, history[r.IngredientID]...)
			switch suggest, review := domain.SuggestExpectedWaste(pcts); {
			case review:
				it.Group = "review"
			case suggest != nil:
				it.Group, it.SuggestedPct = "suggest", suggest
			}
		}
		out.Items = append(out.Items, it)
	}
	return out, nil
}

func countVsSalesRow(r db.CountVsSalesLinesRow) domain.CountVsSalesRow {
	return domain.CountVsSales(domain.CountVsSalesInput{
		HasPrevious: r.HasPrevious, Initial: r.InitialQty, Entries: r.Entries, Sold: r.Sold,
		ExpectedWaste: r.ExpectedWaste, Registered: r.Registered, TransfersOut: r.TransfersOut,
		System: r.SystemQty, Counted: r.CountedQty,
	})
}
