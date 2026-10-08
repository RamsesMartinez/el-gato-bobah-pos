package app

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store/db"
)

// RecipeRow es un renglón de Catálogo › Recetas.
type RecipeRow struct {
	ID     int64  `json:"id"`
	Name   string `json:"name"`
	Group  string `json:"group"` // categoría del producto o grupo del extra
	Status string `json:"status"`
	// Mode dice qué es la receta, para el resumen: "items" (lleva insumos), "combo", "product"
	// (un extra que es otro producto), "own" (se descuenta solo) o "" (sin receta).
	Mode         string          `json:"mode"`
	Summary      string          `json:"summary"`
	Lines        int             `json:"lines"`
	SoldPerMonth decimal.Decimal `json:"soldPerMonth"`
}

// RecipeCounts son cuántas recetas hay en cada estado.
type RecipeCounts struct {
	Pending int `json:"pending"`
	Review  int `json:"review"`
	Done    int `json:"done"`
}

// RecipeTotals son los conteos de cada lista sin búsqueda ni categoría: las insignias de las
// pestañas y el avance.
type RecipeTotals struct {
	Product RecipeCounts `json:"product"`
	Extra   RecipeCounts `json:"extra"`
	Prep    RecipeCounts `json:"prep"`
}

// RecipePage es una página de la lista con sus conteos.
type RecipePage struct {
	Items []RecipeRow `json:"items"`
	Total int         `json:"total"`
	// Counts siguen la búsqueda y la categoría: son los números de los filtros de estado.
	Counts RecipeCounts `json:"counts"`
	Totals RecipeTotals `json:"totals"`
}

// salesWindow es cuánto atrás se cuentan las ventas para «más vendidos primero».
const salesWindow = 30 * 24 * time.Hour

// ListRecipes devuelve una página de la lista de recetas.
func (s *AdminService) ListRecipes(ctx context.Context, f domain.RecipeFilter, now time.Time) (RecipePage, error) {
	if err := f.Validate(); err != nil {
		return RecipePage{}, err
	}
	q := s.store.QC(ctx)
	page := RecipePage{Items: []RecipeRow{}}
	since := pgtype.Date{Time: now.Add(-salesWindow), Valid: true}
	status, sort, lim, off := string(f.Status), string(f.Sort), int32(f.Limit), int32(f.Offset)

	switch f.Kind {
	case domain.RecipeKindProduct:
		rows, err := q.ListProductRecipes(ctx, db.ListProductRecipesParams{
			Since: since, CategoryID: f.Category, Q: f.Query, Status: status, Sort: sort, Lim: lim, Off: off,
		})
		if err != nil {
			return page, err
		}
		for _, r := range rows {
			row := RecipeRow{ID: r.ID, Name: r.Name, Group: r.Category, Status: r.Status, Summary: r.Summary, Lines: int(r.Lines), SoldPerMonth: r.Sold}
			switch {
			case r.TrackStock:
				row.Mode = "own"
			case r.Type == string(db.ProductTypeCombo):
				row.Mode, row.Summary = "combo", r.ComboSummary
			case r.Lines > 0:
				row.Mode = "items"
			}
			page.Items = append(page.Items, row)
			page.Total = int(r.Total)
		}
	case domain.RecipeKindExtra:
		rows, err := q.ListExtraRecipes(ctx, db.ListExtraRecipesParams{
			Since: since, CategoryID: f.Category, Q: f.Query, Status: status, Sort: sort, Lim: lim, Off: off,
		})
		if err != nil {
			return page, err
		}
		for _, r := range rows {
			row := RecipeRow{ID: r.ID, Name: r.Name, Group: r.Category, Status: r.Status, Summary: r.Summary, Lines: int(r.Lines), SoldPerMonth: r.Sold}
			switch {
			case r.LinkedName != "":
				row.Mode, row.Summary = "product", r.LinkedName
			case r.Lines > 0:
				row.Mode = "items"
			}
			page.Items = append(page.Items, row)
			page.Total = int(r.Total)
		}
	case domain.RecipeKindPrep:
		rows, err := q.ListPrepRecipes(ctx, db.ListPrepRecipesParams{Q: f.Query, Status: status, Lim: lim, Off: off})
		if err != nil {
			return page, err
		}
		for _, r := range rows {
			page.Items = append(page.Items, RecipeRow{ID: r.ID, Name: r.Name, Status: r.Status, Mode: "items", Summary: r.Summary, Lines: int(r.Lines), SoldPerMonth: decimal.Zero})
			page.Total = int(r.Total)
		}
	}

	var err error
	if page.Counts, err = recipeCounts(ctx, q, f.Kind, f.Category, f.Query); err != nil {
		return page, err
	}
	if page.Totals.Product, err = recipeCounts(ctx, q, domain.RecipeKindProduct, 0, ""); err != nil {
		return page, err
	}
	if page.Totals.Extra, err = recipeCounts(ctx, q, domain.RecipeKindExtra, 0, ""); err != nil {
		return page, err
	}
	if page.Totals.Prep, err = recipeCounts(ctx, q, domain.RecipeKindPrep, 0, ""); err != nil {
		return page, err
	}
	return page, nil
}

func recipeCounts(ctx context.Context, q *db.Queries, kind domain.RecipeKind, category int64, query string) (RecipeCounts, error) {
	type row struct {
		status string
		n      int32
	}
	var rows []row
	switch kind {
	case domain.RecipeKindProduct:
		rs, err := q.CountProductRecipes(ctx, db.CountProductRecipesParams{CategoryID: category, Q: query})
		if err != nil {
			return RecipeCounts{}, err
		}
		for _, r := range rs {
			rows = append(rows, row{r.Status, r.N})
		}
	case domain.RecipeKindExtra:
		rs, err := q.CountExtraRecipes(ctx, db.CountExtraRecipesParams{CategoryID: category, Q: query})
		if err != nil {
			return RecipeCounts{}, err
		}
		for _, r := range rs {
			rows = append(rows, row{r.Status, r.N})
		}
	case domain.RecipeKindPrep:
		rs, err := q.CountPrepRecipes(ctx, query)
		if err != nil {
			return RecipeCounts{}, err
		}
		for _, r := range rs {
			rows = append(rows, row{r.Status, r.N})
		}
	}
	var c RecipeCounts
	for _, r := range rows {
		switch domain.RecipeStatus(r.status) {
		case domain.RecipeStatusPending:
			c.Pending = int(r.n)
		case domain.RecipeStatusReview:
			c.Review = int(r.n)
		case domain.RecipeStatusDone:
			c.Done = int(r.n)
		}
	}
	return c, nil
}

// ConfirmRecipes confirma de una vez las recetas estimadas de la lista que se ve. Solo las
// estimadas: una pendiente confirmada sin receta diría «no gasta insumos», y eso lo decide una
// persona abriéndola.
func (s *AdminService) ConfirmRecipes(ctx context.Context, kind domain.RecipeKind, ids []int64, actor int64) (int64, error) {
	if err := domain.ValidConfirmIDs(ids); err != nil {
		return 0, err
	}
	q := s.store.QC(ctx)
	switch kind {
	case domain.RecipeKindProduct:
		return q.ConfirmEstimatedProducts(ctx, db.ConfirmEstimatedProductsParams{Ids: ids, Actor: &actor})
	case domain.RecipeKindExtra:
		return q.ConfirmEstimatedOptions(ctx, db.ConfirmEstimatedOptionsParams{Ids: ids, Actor: &actor})
	case domain.RecipeKindPrep:
		return q.ConfirmEstimatedIngredients(ctx, db.ConfirmEstimatedIngredientsParams{Ids: ids, Actor: &actor})
	}
	return 0, domain.ErrValidation
}
