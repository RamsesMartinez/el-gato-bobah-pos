package app

import (
	"context"

	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
)

// WebSalesService es la regla única de la venta en web (spec 092): qué productos se ofrecen, a qué
// precio y cuáles están agotados.
type WebSalesService struct{ store *store.Store }

func NewWebSalesService(s *store.Store) *WebSalesService { return &WebSalesService{store: s} }

// WebOfferItem es un producto que se vende en la web.
type WebOfferItem struct {
	ProductID int64           `json:"productId"`
	Price     decimal.Decimal `json:"price"`
	SoldOut   bool            `json:"soldOut"`
}

// Offer lista lo que se vende en la web con su precio efectivo y si está agotado. Un agotado se
// marca, no se oculta (decisión del dueño, #326).
func (s *WebSalesService) Offer(ctx context.Context) ([]WebOfferItem, error) {
	q := s.store.QC(ctx)
	rows, err := q.WebOfferProducts(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]WebOfferItem, 0, len(rows))
	var graph *domain.StockGraph
	var levels domain.StockLevels
	for _, r := range rows {
		item := WebOfferItem{ProductID: r.ID, Price: domain.ChannelPrice(r.Price, r.WebPrice)}
		if !r.AllowOversell {
			if graph == nil {
				g, err := loadStockGraph(ctx, q)
				if err != nil {
					return nil, err
				}
				graph = &g
				if levels, err = salesStock(ctx, q); err != nil {
					return nil, store.DomainError(err)
				}
			}
			item.SoldOut = domain.SoldOut(*graph, levels, r.ID)
		}
		out = append(out, item)
	}
	return out, nil
}

// WebSalesSummary es lo que la sección «Venta en web» de Ajustes cuenta (FR-025).
type WebSalesSummary struct {
	OnWeb        int `json:"onWeb"`
	WithWebPrice int `json:"withWebPrice"`
	Total        int `json:"total"`
}

// Summary cuenta sobre la misma oferta: si el resumen tuviera su propia consulta, podría decir algo
// distinto de lo que la web muestra.
func (s *WebSalesService) Summary(ctx context.Context) (WebSalesSummary, error) {
	q := s.store.QC(ctx)
	rows, err := q.WebOfferProducts(ctx)
	if err != nil {
		return WebSalesSummary{}, err
	}
	total, err := q.ActiveCatalogCount(ctx)
	if err != nil {
		return WebSalesSummary{}, err
	}
	sum := WebSalesSummary{OnWeb: len(rows), Total: int(total)}
	for _, r := range rows {
		if r.WebPrice != nil {
			sum.WithWebPrice++
		}
	}
	return sum, nil
}
