package app

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
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

// ArticleCreate es lo que se captura en «Nuevo artículo». Lo opcional va en nil.
type ArticleCreate struct {
	Name         string
	BaseUnitID   int16
	CategoryID   *int64
	PlaceID      *int64
	MinStock     *decimal.Decimal
	InitialCost  *decimal.Decimal
	InitialStock *decimal.Decimal
}

// NewArticle es un artículo que nace dentro de otra captura (un gasto o una receta): solo nombre y
// en qué se mide. Se crea en la misma transacción que guarda esa captura (FR-003).
type NewArticle struct {
	Name       string `json:"name"`
	BaseUnitID int16  `json:"baseUnitId"`
}

// CreateArticle da de alta un artículo desde la lista, sin gasto ni receta (FR-002).
func (s *BackofficeService) CreateArticle(ctx context.Context, in ArticleCreate) (int64, error) {
	var id int64
	err := s.store.WithTx(ctx, func(q *db.Queries) error {
		var err error
		id, err = createArticleTx(ctx, q, in)
		return err
	})
	return id, err
}

// createArticleTx es la única alta de artículos: la lista, el gasto y la receta pasan por aquí, con
// la misma regla de nombres repetidos, el mismo candado y el mismo evento de bitácora.
func createArticleTx(ctx context.Context, q *db.Queries, in ArticleCreate) (int64, error) {
	in.Name = domain.CleanArticleName(in.Name)
	if err := domain.ValidateArticle(domain.ArticleInput{Name: in.Name, MinStock: in.MinStock, Cost: in.InitialCost, InitialStock: in.InitialStock}); err != nil {
		return 0, err
	}
	u, err := q.GetUnit(ctx, in.BaseUnitID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, fmt.Errorf("%w: unidad desconocida", domain.ErrValidation)
		}
		return 0, err
	}
	// Se cuenta en la unidad base de su clase (g, ml, cm, pieza); lo que llega en kg o L es un empaque.
	if !u.ToBase.Equal(decimal.NewFromInt(1)) {
		return 0, fmt.Errorf("%w: el artículo se mide en g, ml, cm o piezas; lo que llega en kg o L se captura como empaque", domain.ErrValidation)
	}
	if err := q.LockIngredientNames(ctx); err != nil {
		return 0, err
	}
	switch found, err := q.FindArticleByName(ctx, in.Name); {
	case err == nil:
		return 0, domain.ArticleNameTaken{ID: found.ID, Archived: !found.IsActive}
	case !errors.Is(err, pgx.ErrNoRows):
		return 0, err
	}
	var minStock *decimal.Decimal
	if in.MinStock != nil {
		v := domain.Round4(*in.MinStock)
		minStock = &v
	}
	row, err := q.CreateIngredient(ctx, db.CreateIngredientParams{
		Name: in.Name, BaseUnitID: in.BaseUnitID, CategoryID: in.CategoryID, MinStock: minStock,
	})
	if err != nil {
		return 0, err
	}
	id := row.ID
	if in.PlaceID != nil {
		if err := activePlace(ctx, q, *in.PlaceID); err != nil {
			return 0, err
		}
		if _, err := q.SetCountPlace(ctx, db.SetCountPlaceParams{ID: id, PlaceID: in.PlaceID}); err != nil {
			return 0, err
		}
	}
	actor, hasActor := ActorUserFrom(ctx)
	if in.InitialCost != nil && in.InitialCost.IsPositive() {
		if !hasActor {
			return 0, fmt.Errorf("%w: corregir un costo necesita a la persona que lo hace", domain.ErrForbidden)
		}
		cost := in.InitialCost.Round(6)
		prev, err := q.SetArticleCostManual(ctx, db.SetArticleCostManualParams{ID: id, Cost: cost})
		if err != nil {
			return 0, err
		}
		if err := q.InsertCostCorrection(ctx, db.InsertCostCorrectionParams{IngredientID: id, PreviousCost: prev, NewCost: cost, UserID: actor}); err != nil {
			return 0, err
		}
	}
	if in.InitialStock != nil && in.InitialStock.IsPositive() {
		var by *int64
		if hasActor {
			by = &actor
		}
		if err := q.InsertInitialStock(ctx, db.InsertInitialStockParams{IngredientID: &id, Quantity: domain.Round4(*in.InitialStock), UserID: by}); err != nil {
			return 0, err
		}
	}
	return id, recordAudit(ctx, q, domain.AuditEvent{
		Action: domain.AuditIngredientCreated, EntityType: "ingredient", EntityID: id,
		After: map[string]any{"name": in.Name, "baseUnit": u.Code, "initialStock": in.InitialStock, "initialCost": in.InitialCost},
	})
}

// ensureArticleTx es el alta de un artículo que nace dentro de un gasto o una receta: si ya existe uno
// activo con ese nombre se usa ése, como hacía la pantalla al recibir el 409. Uno archivado no se
// reactiva en silencio: se responde el conflicto para que se decida en la lista.
func ensureArticleTx(ctx context.Context, q *db.Queries, na NewArticle) (int64, error) {
	id, err := createArticleTx(ctx, q, ArticleCreate{Name: na.Name, BaseUnitID: na.BaseUnitID})
	var taken domain.ArticleNameTaken
	if errors.As(err, &taken) && !taken.Archived {
		return taken.ID, nil
	}
	return id, err
}
