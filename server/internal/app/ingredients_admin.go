package app

import (
	"context"
	"strings"

	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store/db"
)

// Gestión de artículos del almacén (spec 042): la lista con buscador y filtros, la ficha, editar,
// archivar, el historial de costos, la bitácora y el cambio de unidad.

// ArticleFilter son los filtros de la lista. Se combinan con «y».
type ArticleFilter struct {
	Q        string
	Archived bool
	PlaceID  *int64
	NoPlace  bool
	NoCost   bool
	Negative bool
}

// PresentationLabel es un empaque como se pinta en la lista: «Garrafa · 4 L».
type PresentationLabel struct {
	ID        int64           `json:"id"`
	Name      string          `json:"name"`
	QtyInBase decimal.Decimal `json:"qtyInBase"`
}

// ArticleRow es un renglón de la lista de artículos.
type ArticleRow struct {
	ID            int64               `json:"id"`
	Name          string              `json:"name"`
	IsActive      bool                `json:"isActive"`
	BaseUnitID    int16               `json:"baseUnitId"`
	BaseUnitCode  string              `json:"baseUnitCode"`
	BaseUnitKind  string              `json:"baseUnitKind"`
	CurrentCost   decimal.Decimal     `json:"currentCost"`
	MinStock      *decimal.Decimal    `json:"minStock"`
	IsPrep        bool                `json:"isPrep"`
	CategoryID    *int64              `json:"categoryId"`
	Category      *string             `json:"category"`
	CountPlaceID  *int64              `json:"countPlaceId"`
	CountPlace    *string             `json:"countPlace"`
	OnHand        decimal.Decimal     `json:"onHand"`
	Presentations []PresentationLabel `json:"presentations"`
}

// ListArticles es la lista de Almacén › Artículos. Los empaques de cada artículo se juntan aparte, en
// una sola lectura, en vez de una consulta por renglón.
func (s *BackofficeService) ListArticles(ctx context.Context, f ArticleFilter) ([]ArticleRow, error) {
	q := s.store.QC(ctx)
	var needle *string
	if t := strings.TrimSpace(f.Q); t != "" {
		needle = &t
	}
	rows, err := q.ListArticles(ctx, db.ListArticlesParams{
		Archived: f.Archived, Q: needle, PlaceID: f.PlaceID, NoPlace: f.NoPlace, NoCost: f.NoCost, Negative: f.Negative,
	})
	if err != nil {
		return nil, err
	}
	pres, err := q.ListPresentations(ctx, nil)
	if err != nil {
		return nil, err
	}
	byArticle := map[int64][]PresentationLabel{}
	for _, p := range pres {
		if p.Active && !p.IsApproximate {
			byArticle[p.IngredientID] = append(byArticle[p.IngredientID], PresentationLabel{ID: p.ID, Name: p.Name, QtyInBase: p.QtyInBase})
		}
	}
	out := make([]ArticleRow, len(rows))
	for i, r := range rows {
		ps := byArticle[r.ID]
		if ps == nil {
			// Un arreglo prometido sale como [] y no como null (AGENTS §1).
			ps = []PresentationLabel{}
		}
		out[i] = ArticleRow{
			ID: r.ID, Name: r.Name, IsActive: r.IsActive, BaseUnitID: r.BaseUnitID, BaseUnitCode: r.BaseUnitCode,
			BaseUnitKind: string(r.BaseUnitKind), CurrentCost: r.CurrentCost, MinStock: r.MinStock, IsPrep: r.IsPrep,
			CategoryID: r.CategoryID, Category: r.Category, CountPlaceID: r.CountPlaceID, CountPlace: r.CountPlace,
			OnHand: r.OnHand, Presentations: ps,
		}
	}
	return out, nil
}
