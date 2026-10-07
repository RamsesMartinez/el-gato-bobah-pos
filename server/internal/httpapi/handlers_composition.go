package httpapi

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
)

// Composición de productos y extras (spec 028): qué descuenta del almacén cada uno al venderse.

func (h *Handlers) GetProductComposition(w http.ResponseWriter, r *http.Request) {
	h.getComposition(app.CompositionOfProduct)(w, r)
}

func (h *Handlers) PutProductComposition(w http.ResponseWriter, r *http.Request) {
	h.putComposition(app.CompositionOfProduct)(w, r)
}

func (h *Handlers) ConfirmProductComposition(w http.ResponseWriter, r *http.Request) {
	h.confirmComposition(app.CompositionOfProduct)(w, r)
}

func (h *Handlers) GetOptionComposition(w http.ResponseWriter, r *http.Request) {
	h.getComposition(app.CompositionOfOption)(w, r)
}

func (h *Handlers) PutOptionComposition(w http.ResponseWriter, r *http.Request) {
	h.putComposition(app.CompositionOfOption)(w, r)
}

func (h *Handlers) ConfirmOptionComposition(w http.ResponseWriter, r *http.Request) {
	h.confirmComposition(app.CompositionOfOption)(w, r)
}

func compositionTarget(r *http.Request, kind app.CompositionKind) (app.CompositionKind, int64, error) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		return kind, 0, domain.ErrValidation
	}
	return kind, id, nil
}

func (h *Handlers) getComposition(kind app.CompositionKind) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		kind, id, err := compositionTarget(r, kind)
		if err != nil {
			Error(w, err)
			return
		}
		v, err := h.admin.Composition(r.Context(), kind, id)
		if err != nil {
			Error(w, err)
			return
		}
		JSON(w, http.StatusOK, v)
	}
}

func (h *Handlers) putComposition(kind app.CompositionKind) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		kind, id, err := compositionTarget(r, kind)
		if err != nil {
			Error(w, err)
			return
		}
		var body struct {
			Items           []app.CompositionInputItem `json:"items"`
			LinkedProductID *int64                     `json:"linkedProductId"`
		}
		if err := Decode(r, &body); err != nil {
			Error(w, err)
			return
		}
		u, _ := userFrom(r.Context())
		if err := h.admin.SaveComposition(r.Context(), kind, id, body.Items, body.LinkedProductID, u.ID); err != nil {
			Error(w, err)
			return
		}
		v, err := h.admin.Composition(r.Context(), kind, id)
		if err != nil {
			Error(w, err)
			return
		}
		JSON(w, http.StatusOK, v)
	}
}

func (h *Handlers) confirmComposition(kind app.CompositionKind) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		kind, id, err := compositionTarget(r, kind)
		if err != nil {
			Error(w, err)
			return
		}
		u, _ := userFrom(r.Context())
		if err := h.admin.ConfirmComposition(r.Context(), kind, id, u.ID); err != nil {
			Error(w, err)
			return
		}
		v, err := h.admin.Composition(r.Context(), kind, id)
		if err != nil {
			Error(w, err)
			return
		}
		JSON(w, http.StatusOK, v)
	}
}

// compositionFilter acepta solo los valores conocidos: un filtro mal escrito que se vuelve «todos»
// muestra una lista que parece completa y no lo es.
func compositionFilter(v string) (string, error) {
	switch v {
	case "", "none", "estimated":
		return v, nil
	}
	return "", domain.ErrValidation
}
