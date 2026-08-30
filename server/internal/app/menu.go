package app

import (
	"context"
	"time"

	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
)

// MenuDoc es el documento denormalizado que consume el POS en un solo request.
type MenuDoc struct {
	Version    int64          `json:"version"`
	Categories []MenuCategory `json:"categories"`
	Products   []MenuProduct  `json:"products"`
	// Todo lo que el POS necesita para pintar CUALQUIER lista de precios sin volver a pedir nada:
	// cambiar de plataforma tiene que ser instantáneo, y una llamada por cambio haría lento justo
	// el momento que esta feature vino a acelerar.
	//
	// La llave del caché sigue siendo pos:menu:<companyID>, sin la plataforma: con ella serían
	// cuatro entradas y cuatro invalidaciones para el mismo catálogo.
	Platforms []MenuPlatform `json:"platforms"`
	// Solo las EXCEPCIONES, por plataforma. Un producto ausente usa base × (1 + margen).
	PlatformPrices    map[int16]map[int64]decimal.Decimal `json:"platformPrices"`
	PlatformModPrices map[int16]map[int64]decimal.Decimal `json:"platformModPrices"`
}

// MenuPlatform: una lista de precios que el operador puede elegir. "Propio" NO se incluye — es
// reparto del propio negocio, sin comisión que absorber ni depósito que conciliar, y no tiene
// método de pago propio: se vende como domicilio a precio base.
type MenuPlatform struct {
	ID        int16           `json:"id"`
	Name      string          `json:"name"`
	MarkupPct decimal.Decimal `json:"markupPct"`
}

type MenuCategory struct {
	ID       int64   `json:"id"`
	Name     string  `json:"name"`
	ParentID *int64  `json:"parentId"`
	SortKey  float64 `json:"sortKey"`
	Color    *string `json:"color"`
	ImageURL *string `json:"imageUrl"`
}

type MenuProduct struct {
	ID          int64           `json:"id"`
	Name        string          `json:"name"`
	Description *string         `json:"description"`
	Price       decimal.Decimal `json:"price"`
	Cost        decimal.Decimal `json:"cost"`
	Margin      decimal.Decimal `json:"margin"`
	CategoryID  int64           `json:"categoryId"`
	Type        string          `json:"type"`
	Favorite    bool            `json:"favorite"`
	ImageURL    *string         `json:"imageUrl"`
	TrackStock  bool            `json:"trackStock"`
	Groups      []MenuGroup     `json:"groups"`
}

type MenuGroup struct {
	ID      int64        `json:"id"`
	Title   string       `json:"title"`
	Min     int          `json:"min"`
	Max     int          `json:"max"`
	Options []MenuOption `json:"options"`
}

type MenuOption struct {
	ID         int64           `json:"id"`
	Name       string          `json:"name"`
	PriceDelta decimal.Decimal `json:"priceDelta"`
	MaxPerLine int             `json:"maxPerLine"`
	Favorite   bool            `json:"favorite"`
}

type MenuService struct {
	store *store.Store
	now   func() time.Time
}

func NewMenuService(s *store.Store, now func() time.Time) *MenuService {
	if now == nil {
		now = time.Now
	}
	return &MenuService{store: s, now: now}
}

// Build arma el documento del menú desde lecturas planas.
func (s *MenuService) Build(ctx context.Context) (*MenuDoc, error) {
	catRows, err := s.store.QC(ctx).MenuCategories(ctx)
	if err != nil {
		return nil, err
	}
	prodRows, err := s.store.QC(ctx).MenuProducts(ctx)
	if err != nil {
		return nil, err
	}
	groupRows, err := s.store.QC(ctx).MenuProductGroups(ctx)
	if err != nil {
		return nil, err
	}
	optRows, err := s.store.QC(ctx).MenuOptions(ctx)
	if err != nil {
		return nil, err
	}

	// slices vacías (no nil) → el JSON siempre es [] y el front nunca recibe null
	doc := &MenuDoc{
		Version:           s.now().UnixMilli(),
		Categories:        []MenuCategory{},
		Products:          []MenuProduct{},
		Platforms:         []MenuPlatform{},
		PlatformPrices:    map[int16]map[int64]decimal.Decimal{},
		PlatformModPrices: map[int16]map[int64]decimal.Decimal{},
	}

	platRows, err := s.store.QC(ctx).ListPlatformsWithMarkup(ctx)
	if err != nil {
		return nil, err
	}
	for _, pl := range platRows {
		doc.Platforms = append(doc.Platforms, MenuPlatform{ID: pl.ID, Name: pl.Name, MarkupPct: pl.PriceMarkupPct})
		precios, err := s.store.QC(ctx).GetProductPlatformPrices(ctx, pl.ID)
		if err != nil {
			return nil, err
		}
		if len(precios) > 0 {
			m := map[int64]decimal.Decimal{}
			for _, x := range precios {
				m[x.ProductID] = x.Price
			}
			doc.PlatformPrices[pl.ID] = m
		}
		deltas, err := s.store.QC(ctx).GetOptionPlatformPrices(ctx, pl.ID)
		if err != nil {
			return nil, err
		}
		if len(deltas) > 0 {
			m := map[int64]decimal.Decimal{}
			for _, x := range deltas {
				m[x.OptionID] = x.PriceDelta
			}
			doc.PlatformModPrices[pl.ID] = m
		}
	}
	for _, c := range catRows {
		doc.Categories = append(doc.Categories, MenuCategory{
			ID: c.ID, Name: c.Name, ParentID: c.ParentID,
			SortKey: c.SortKey.InexactFloat64(), Color: c.Color, ImageURL: c.ImageUrl,
		})
	}

	optionsByGroup := map[int64][]MenuOption{}
	for _, o := range optRows {
		optionsByGroup[o.GroupID] = append(optionsByGroup[o.GroupID], MenuOption{
			ID: o.ID, Name: o.Name, PriceDelta: o.PriceDelta, MaxPerLine: int(o.MaxPerLine),
			Favorite: o.IsFavorite,
		})
	}

	groupsByProduct := map[int64][]MenuGroup{}
	for _, g := range groupRows {
		opts := optionsByGroup[g.GroupID]
		if opts == nil {
			opts = []MenuOption{}
		}
		groupsByProduct[g.ProductID] = append(groupsByProduct[g.ProductID], MenuGroup{
			ID: g.GroupID, Title: g.Title, Min: int(g.MinSelect), Max: int(g.MaxSelect),
			Options: opts,
		})
	}

	for _, pr := range prodRows {
		groups := groupsByProduct[pr.ID]
		if groups == nil {
			groups = []MenuGroup{}
		}
		doc.Products = append(doc.Products, MenuProduct{
			ID: pr.ID, Name: pr.Name, Description: pr.Description,
			Price:      domain.Round2(pr.Price),
			Cost:       domain.Round2(pr.CurrentCost),
			Margin:     domain.Round2(pr.Price.Sub(pr.CurrentCost)),
			CategoryID: pr.CategoryID, Type: string(pr.Type),
			Favorite: pr.IsFavorite, ImageURL: pr.ImageUrl, TrackStock: pr.TrackStock,
			Groups: groups,
		})
	}
	return doc, nil
}

// Popular devuelve los IDs de producto más vendidos (ventana reciente), ordenados.
// Read model aparte del menú: se cachea con TTL corto para que el "Top" refresque
// en minutos sin reconstruir todo el catálogo. No filtra a productos vigentes —
// el front mapea id→producto y descarta los que ya no existen.
func (s *MenuService) Popular(ctx context.Context) ([]int64, error) {
	rows, err := s.store.QC(ctx).PopularProducts(ctx)
	if err != nil {
		return nil, err
	}
	ids := make([]int64, 0, len(rows))
	for _, r := range rows {
		ids = append(ids, r.ProductID)
	}
	return ids, nil
}
