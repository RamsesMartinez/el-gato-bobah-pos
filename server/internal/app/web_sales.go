package app

import (
	"context"

	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store/db"
)

// WebSalesService es la regla única de la venta en web (spec 092): qué productos se ofrecen, a qué
// precio y cuáles están agotados.
type WebSalesService struct{ store *store.Store }

func NewWebSalesService(s *store.Store) *WebSalesService { return &WebSalesService{store: s} }

// WebOfferItem es un producto que se vende en la web, con lo que el menú público pinta (spec 097).
type WebOfferItem struct {
	ProductID   int64           `json:"productId"`
	Price       decimal.Decimal `json:"price"`
	SoldOut     bool            `json:"soldOut"`
	Name        string          `json:"name"`
	Description *string         `json:"description"`
	ImageURL    *string         `json:"imageUrl"`
	CategoryID  int64           `json:"categoryId"`
}

// Offer lista lo que se vende en la web con su precio efectivo y si está agotado. Un agotado se
// marca, no se oculta (decisión del dueño, #326).
func (s *WebSalesService) Offer(ctx context.Context) ([]WebOfferItem, error) {
	return offer(ctx, s.store.QC(ctx))
}

// offer es la regla única sobre unas Queries dadas: el menú público la corre dentro de la
// transacción en la que fijó la empresa que resolvió por nombre, sin sesión.
func offer(ctx context.Context, q *db.Queries) ([]WebOfferItem, error) {
	rows, err := q.WebOfferProducts(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]WebOfferItem, 0, len(rows))
	var graph *domain.StockGraph
	var levels domain.StockLevels
	for _, r := range rows {
		item := WebOfferItem{ProductID: r.ID, Price: domain.ChannelPrice(r.Price, r.WebPrice),
			Name: r.Name, Description: r.Description, ImageURL: r.ImageUrl, CategoryID: r.CategoryID}
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

// Summary cuenta con la misma base que Offer, en una sola sentencia (WebSalesCounts).
func (s *WebSalesService) Summary(ctx context.Context) (WebSalesSummary, error) {
	r, err := s.store.QC(ctx).WebSalesCounts(ctx)
	if err != nil {
		return WebSalesSummary{}, err
	}
	return WebSalesSummary{OnWeb: int(r.OnWeb), WithWebPrice: int(r.WithWebPrice), Total: int(r.Total)}, nil
}
