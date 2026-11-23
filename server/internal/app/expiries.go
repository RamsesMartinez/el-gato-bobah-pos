package app

import (
	"context"
	"slices"
	"time"

	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
)

// Caducidades (spec 098, historia 3): los lotes por almacén con su proyección contra el consumo.

// ExpiryLot es un lote como lo pinta la vista.
type ExpiryLot struct {
	IngredientID     int64            `json:"ingredientId"`
	Name             string           `json:"name"`
	UnitCode         string           `json:"unitCode"`
	WarehouseID      int64            `json:"warehouseId"`
	WarehouseName    string           `json:"warehouseName"`
	ExpiresOn        string           `json:"expiresOn"`
	Qty              decimal.Decimal  `json:"qty"`
	DaysLeft         int              `json:"daysLeft"`
	Expired          bool             `json:"expired"`
	DailyUse         *decimal.Decimal `json:"dailyUse"`
	NoRecentUse      bool             `json:"noRecentUse"`
	FinishesInDays   *int             `json:"finishesInDays"`
	LeftoverAtExpiry *decimal.Decimal `json:"leftoverAtExpiry"`
	PendingWaste     decimal.Decimal  `json:"pendingWaste"`
}

// ExpiryCounts cuenta lotes por plazo; cada lote cae en uno solo.
type ExpiryCounts struct {
	Expired  int `json:"expired"`
	Within7  int `json:"within7"`
	Within30 int `json:"within30"`
	Within60 int `json:"within60"`
}

// ExpiriesView es la vista: hoy del negocio, cuántos por plazo y los lotes (nunca nil).
type ExpiriesView struct {
	Today  string       `json:"today"`
	Counts ExpiryCounts `json:"counts"`
	Lots   []ExpiryLot  `json:"lots"`
}

// ExpirySummary es lo que dice el aviso de Existencias.
type ExpirySummary struct {
	Expired int `json:"expired"`
	Within7 int `json:"within7"`
}

// Expiries lista los lotes del almacén pedido (todos con sel.All) del que caduca antes al que caduca
// después. La proyección se hace con toda la sucursal: Bodega alimenta a Local, así que lo que se usa
// antes de un lote incluye lo sin fecha y los lotes que caducan antes en cualquier almacén.
func (s *BackofficeService) Expiries(ctx context.Context, sel domain.WarehouseSel) (ExpiriesView, error) {
	q := s.store.QC(ctx)
	ids, err := warehouseIDs(ctx, q, sel)
	if err != nil {
		return ExpiriesView{}, err
	}
	todayDate, err := q.BusinessToday(ctx)
	if err != nil {
		return ExpiriesView{}, err
	}
	today := todayDate.Time
	rows, err := q.BranchLots(ctx)
	if err != nil {
		return ExpiriesView{}, err
	}
	var ingIDs []int64
	for _, r := range rows {
		if !slices.Contains(ingIDs, r.IngredientID) {
			ingIDs = append(ingIDs, r.IngredientID)
		}
	}
	undated := map[int64]decimal.Decimal{}
	daily := map[int64]decimal.Decimal{}
	if len(ingIDs) > 0 {
		us, err := q.BranchUndated(ctx, ingIDs)
		if err != nil {
			return ExpiriesView{}, err
		}
		for _, u := range us {
			undated[u.IngredientID] = u.Undated
		}
		ds, err := q.BranchDailyUse(ctx, ingIDs)
		if err != nil {
			return ExpiriesView{}, err
		}
		for _, d := range ds {
			daily[d.IngredientID] = domain.Round4(d.Daily)
		}
	}
	// La proyección de cada artículo, con todos sus lotes de la sucursal.
	byIng := map[int64][]int{}
	for i, r := range rows {
		byIng[r.IngredientID] = append(byIng[r.IngredientID], i)
	}
	proj := make([]domain.LotProjection, len(rows))
	for ing, idx := range byIng {
		lots := make([]domain.LotStock, len(idx))
		for j, i := range idx {
			lots[j] = domain.LotStock{ExpiresOn: rows[i].ExpiresOn.Time, Qty: rows[i].Qty}
		}
		var use *decimal.Decimal
		if d, ok := daily[ing]; ok && d.IsPositive() {
			use = &d
		}
		for j, p := range domain.ProjectLots(undated[ing], lots, use, today) {
			proj[idx[j]] = p
		}
	}
	out := ExpiriesView{Today: today.Format(time.DateOnly), Lots: []ExpiryLot{}}
	for i, r := range rows {
		if !slices.Contains(ids, r.WarehouseID) {
			continue
		}
		left := domain.DaysLeft(r.ExpiresOn.Time, today)
		lot := ExpiryLot{IngredientID: r.IngredientID, Name: r.Name, UnitCode: r.UnitCode, WarehouseID: r.WarehouseID,
			WarehouseName: r.WarehouseName, ExpiresOn: r.ExpiresOn.Time.Format(time.DateOnly), Qty: r.Qty, DaysLeft: left,
			Expired: proj[i].Expired, FinishesInDays: proj[i].FinishesInDays, LeftoverAtExpiry: proj[i].LeftoverAtExpiry,
			PendingWaste: r.PendingWaste}
		if d, ok := daily[r.IngredientID]; ok && d.IsPositive() {
			lot.DailyUse = &d
		} else {
			lot.NoRecentUse = true
		}
		switch {
		case left < 0:
			out.Counts.Expired++
		case left <= 7:
			out.Counts.Within7++
		case left <= 30:
			out.Counts.Within30++
		case left <= 60:
			out.Counts.Within60++
		}
		out.Lots = append(out.Lots, lot)
	}
	return out, nil
}

// ExpirySummary cuenta lo caducado y lo que caduca en 7 días del almacén, para el aviso de Existencias.
func (s *BackofficeService) ExpirySummary(ctx context.Context, sel domain.WarehouseSel) (ExpirySummary, error) {
	v, err := s.Expiries(ctx, sel)
	if err != nil {
		return ExpirySummary{}, err
	}
	return ExpirySummary{Expired: v.Counts.Expired, Within7: v.Counts.Within7}, nil
}
