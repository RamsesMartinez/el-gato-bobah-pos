package httpapi

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
)

// Empaques de compra por insumo (spec 038).

// idParam lee un id de la ruta; uno mal formado es un 400, no un 500.
func idParam(r *http.Request, name string) (int64, error) {
	id, err := strconv.ParseInt(chi.URLParam(r, name), 10, 64)
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("%w: %s inválido", domain.ErrValidation, name)
	}
	return id, nil
}

type presentationBody struct {
	Name        string          `json:"name"`
	Qty         decimal.Decimal `json:"qty"`
	UnitID      int16           `json:"unitId"`
	Approximate bool            `json:"approximate"`
	Active      *bool           `json:"active"`
}

func (b presentationBody) input() app.PresentationInput {
	return app.PresentationInput{Name: b.Name, Qty: b.Qty, UnitID: b.UnitID, Approximate: b.Approximate, Active: b.Active}
}

// GET /stock/presentations — todos los empaques de la empresa (la columna «Cómo llega»).
func (h *Handlers) ListPresentations(w http.ResponseWriter, r *http.Request) {
	list, err := h.backoffice.Presentations(r.Context(), nil)
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, list)
}

// GET /stock/ingredients/{id}/presentations
func (h *Handlers) ListIngredientPresentations(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r, "id")
	if err != nil {
		Error(w, err)
		return
	}
	list, err := h.backoffice.Presentations(r.Context(), &id)
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, list)
}

// POST /stock/ingredients/{id}/presentations
func (h *Handlers) CreatePresentation(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r, "id")
	if err != nil {
		Error(w, err)
		return
	}
	var body presentationBody
	if err := Decode(r, &body); err != nil {
		Error(w, err)
		return
	}
	v, err := h.backoffice.CreatePresentation(r.Context(), id, body.input())
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusCreated, v)
}

// PUT /stock/presentations/{id} — editar o desactivar («Ya no llega así»).
func (h *Handlers) UpdatePresentation(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r, "id")
	if err != nil {
		Error(w, err)
		return
	}
	var body presentationBody
	if err := Decode(r, &body); err != nil {
		Error(w, err)
		return
	}
	v, err := h.backoffice.UpdatePresentation(r.Context(), id, body.input())
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, v)
}
