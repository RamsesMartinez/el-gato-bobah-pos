package httpapi

import (
	"fmt"
	"net/http"
	"strconv"

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
