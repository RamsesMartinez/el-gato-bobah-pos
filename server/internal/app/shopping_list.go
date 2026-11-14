package app

import (
	"context"
	"time"

	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store/db"
)

func decimalFrom(s string) (decimal.Decimal, error) { return decimal.NewFromString(s) }

// La lista de compra (spec 084, historia 8). Una sola consulta para la pantalla y el correo de la
// mañana; cuánto comprar lo decide domain.SuggestPurchase.

// consumptionWindow es cuánto atrás se mide el consumo, y measuredDays cuántos días con venta hacen
// falta dentro de esa ventana para creerle.
const (
	consumptionWindow = 14
	measuredDays      = 7
)

// ShoppingLine es un renglón de la lista de compra.
type ShoppingLine struct {
	ItemType     string                     `json:"itemType"`
	ID           int64                      `json:"id"`
	Name         string                     `json:"name"`
	Unit         string                     `json:"unit"`
	OnHand       decimal.Decimal            `json:"onHand"`
	Min          *decimal.Decimal           `json:"min"`
	DailyUse     *decimal.Decimal           `json:"dailyUse"`
	CoverageDays int                        `json:"coverageDays"`
	Supplier     string                     `json:"supplier"`
	State        string                     `json:"state"` // out | low | count
	Suggested    *domain.PurchaseSuggestion `json:"suggested"`
	Why          string                     `json:"why"`
}

// ShoppingList arma la lista de la sucursal de la sesión. Nunca nil.
func (s *NotificationsService) ShoppingList(ctx context.Context) ([]ShoppingLine, error) {
	return shoppingListIn(ctx, s.store.QC(ctx))
}

// shoppingListIn es la lista dentro de una transacción ajena (el correo de la mañana).
func shoppingListIn(ctx context.Context, q *db.Queries) ([]ShoppingLine, error) {
	now, err := q.DBNow(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := q.ShoppingListItems(ctx, now.Add(-consumptionWindow*24*time.Hour))
	if err != nil {
		return nil, err
	}
	var ings []int64
	for _, r := range rows {
		if r.ItemType == "ingrediente" {
			ings = append(ings, r.ID)
		}
	}
	packs := map[int64][]domain.Pack{}
	if len(ings) > 0 {
		ps, err := q.PacksWithLastCost(ctx, ings)
		if err != nil {
			return nil, err
		}
		for _, p := range ps {
			pk := domain.Pack{Name: p.Name, QtyInBase: p.QtyInBase}
			if !p.UnitCost.IsNegative() {
				c := p.UnitCost
				pk.UnitCost = &c
			}
			packs[p.IngredientID] = append(packs[p.IngredientID], pk)
		}
	}
	out := make([]ShoppingLine, 0, len(rows))
	for _, r := range rows {
		l := ShoppingLine{ItemType: string(r.ItemType), ID: r.ID, Name: r.Name, Unit: r.UnitCode, OnHand: r.OnHand,
			CoverageDays: int(r.CoverageDays), Supplier: r.Supplier, State: "low"}
		if l.CoverageDays == 0 {
			l.CoverageDays = domain.DefaultCoverageDays
		}
		if r.MinStock.IsPositive() {
			m := r.MinStock
			l.Min = &m
		}
		measured := r.UsedDays >= measuredDays && r.Used.IsPositive()
		if r.Used.IsPositive() {
			d := r.Used.Div(decimal.NewFromInt(consumptionWindow))
			l.DailyUse = &d
		}
		advice := domain.SuggestPurchase(domain.PurchaseInput{DailyUse: l.DailyUse, Measured: measured, OnHand: r.OnHand,
			Unit: r.UnitCode, CoverageDays: l.CoverageDays, Packs: packs[r.ID]})
		l.Suggested, l.Why = advice.Suggested, advice.Why
		switch {
		case advice.NeedsCount:
			l.State = "count"
		case r.OutOpen:
			l.State = "out"
		}
		out = append(out, l)
	}
	return out, nil
}
