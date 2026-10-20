package httpapi

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
)

// Gestión de artículos del almacén (spec 042).

// GET /stock/articles?q=&status=active|archived&placeId=|none&noCost=1&negative=1 — la lista de
// Almacén › Artículos. Un valor presente y malformado se rechaza; nunca cae a un default en silencio.
func (h *Handlers) ListArticles(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := app.ArticleFilter{Q: q.Get("q")}
	switch q.Get("status") {
	case "", "active":
	case "archived":
		f.Archived = true
	default:
		Error(w, fmt.Errorf("%w: estado desconocido", domain.ErrValidation))
		return
	}
	switch p := q.Get("placeId"); p {
	case "":
	case "none":
		f.NoPlace = true
	default:
		id, err := strconv.ParseInt(p, 10, 64)
		if err != nil || id <= 0 {
			Error(w, fmt.Errorf("%w: lugar inválido", domain.ErrValidation))
			return
		}
		f.PlaceID = &id
	}
	var err error
	if f.NoCost, err = flagParam(q.Get("noCost")); err != nil {
		Error(w, err)
		return
	}
	if f.Negative, err = flagParam(q.Get("negative")); err != nil {
		Error(w, err)
		return
	}
	out, err := h.backoffice.ListArticles(r.Context(), f)
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, out)
}

// flagParam lee un filtro de sí o no: ausente es no; «1» es sí; cualquier otra cosa es un error.
func flagParam(v string) (bool, error) {
	switch v {
	case "", "0":
		return false, nil
	case "1":
		return true, nil
	}
	return false, fmt.Errorf("%w: filtro inválido", domain.ErrValidation)
}

// POST /stock/articles — «Nuevo artículo» (FR-002). 409 NAME_TAKEN con el que ya existe.
func (h *Handlers) CreateArticle(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name         string           `json:"name"`
		BaseUnitID   int16            `json:"baseUnitId"`
		CategoryID   *int64           `json:"categoryId"`
		PlaceID      *int64           `json:"placeId"`
		MinStock     *decimal.Decimal `json:"minStock"`
		InitialCost  *decimal.Decimal `json:"initialCost"`
		InitialStock *decimal.Decimal `json:"initialStock"`
	}
	if err := Decode(r, &body); err != nil {
		Error(w, err)
		return
	}
	id, err := h.backoffice.CreateArticle(r.Context(), app.ArticleCreate{
		Name: body.Name, BaseUnitID: body.BaseUnitID, CategoryID: body.CategoryID, PlaceID: body.PlaceID,
		MinStock: body.MinStock, InitialCost: body.InitialCost, InitialStock: body.InitialStock,
	})
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusCreated, map[string]int64{"id": id})
}

// GET /stock/articles/{id} — la ficha.
func (h *Handlers) GetArticle(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r, "id")
	if err != nil {
		Error(w, err)
		return
	}
	out, err := h.backoffice.GetArticle(r.Context(), id)
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, out)
}

// PATCH /stock/articles/{id} — editar la ficha con la versión que se vio (409 STALE si cambió).
func (h *Handlers) UpdateArticle(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r, "id")
	if err != nil {
		Error(w, err)
		return
	}
	var body struct {
		ExpectedUpdatedAt time.Time        `json:"expectedUpdatedAt"`
		Name              string           `json:"name"`
		CategoryID        *int64           `json:"categoryId"`
		PlaceID           *int64           `json:"placeId"`
		MinStock          *decimal.Decimal `json:"minStock"`
	}
	if err := Decode(r, &body); err != nil {
		Error(w, err)
		return
	}
	if body.ExpectedUpdatedAt.IsZero() {
		Error(w, fmt.Errorf("%w: falta la versión de la ficha", domain.ErrValidation))
		return
	}
	if err := h.backoffice.UpdateArticle(r.Context(), id, app.ArticleUpdate{
		ExpectedUpdatedAt: body.ExpectedUpdatedAt, Name: body.Name, CategoryID: body.CategoryID,
		PlaceID: body.PlaceID, MinStock: body.MinStock,
	}); err != nil {
		Error(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// POST /stock/articles/{id}/cost — `{ cost }`: corrección a mano del costo por unidad base.
func (h *Handlers) CorrectArticleCost(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r, "id")
	if err != nil {
		Error(w, err)
		return
	}
	var body struct {
		Cost decimal.Decimal `json:"cost"`
	}
	if err := Decode(r, &body); err != nil {
		Error(w, err)
		return
	}
	if err := h.backoffice.CorrectArticleCost(r.Context(), id, body.Cost); err != nil {
		Error(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// beforeParam lee el cursor de «Ver más»; ausente es la primera página, ilegible es 400.
func beforeParam(r *http.Request) (*app.HistoryCursor, error) {
	v := r.URL.Query().Get("before")
	if v == "" {
		return nil, nil
	}
	return app.DecodeHistoryCursor(v)
}

// GET /stock/articles/{id}/cost-history?before= — el historial de costos, de 20 en 20.
func (h *Handlers) ArticleCostHistory(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r, "id")
	if err != nil {
		Error(w, err)
		return
	}
	before, err := beforeParam(r)
	if err != nil {
		Error(w, err)
		return
	}
	out, err := h.backoffice.ArticleCostHistory(r.Context(), id, before)
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, out)
}

// GET /stock/articles/{id}/audit?before= — la bitácora del artículo y de sus empaques.
func (h *Handlers) ArticleAudit(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r, "id")
	if err != nil {
		Error(w, err)
		return
	}
	before, err := beforeParam(r)
	if err != nil {
		Error(w, err)
		return
	}
	out, err := h.backoffice.ArticleAudit(r.Context(), id, before)
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, out)
}

// GET /stock/articles/{id}/archive-check — qué queda y qué lo usa, antes de archivar.
func (h *Handlers) ArticleArchiveCheck(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r, "id")
	if err != nil {
		Error(w, err)
		return
	}
	out, err := h.backoffice.ArticleArchiveCheck(r.Context(), id)
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, out)
}

// POST /stock/articles/{id}/archive — 409 IN_USE con receta activa o conteo abierto.
func (h *Handlers) ArchiveArticle(w http.ResponseWriter, r *http.Request) {
	h.articleAction(w, r, h.backoffice.ArchiveArticle)
}

// POST /stock/articles/{id}/reactivate
func (h *Handlers) ReactivateArticle(w http.ResponseWriter, r *http.Request) {
	h.articleAction(w, r, h.backoffice.ReactivateArticle)
}

func (h *Handlers) articleAction(w http.ResponseWriter, r *http.Request, act func(context.Context, int64) error) {
	id, err := idParam(r, "id")
	if err != nil {
		Error(w, err)
		return
	}
	if err := act(r.Context(), id); err != nil {
		Error(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// GET /stock/ingredient-categories
func (h *Handlers) ArticleCategories(w http.ResponseWriter, r *http.Request) {
	out, err := h.backoffice.ArticleCategories(r.Context())
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, out)
}

// POST /stock/ingredient-categories — `{ name }`.
func (h *Handlers) CreateArticleCategory(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name string `json:"name"`
	}
	if err := Decode(r, &body); err != nil {
		Error(w, err)
		return
	}
	out, err := h.backoffice.CreateArticleCategory(r.Context(), body.Name)
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusCreated, out)
}

// GET /stock/presentation-suggestions?unitKind= — empaques de artículos que se miden igual.
func (h *Handlers) PresentationSuggestions(w http.ResponseWriter, r *http.Request) {
	out, err := h.backoffice.PresentationSuggestions(r.Context(), r.URL.Query().Get("unitKind"))
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, out)
}
