package httpapi

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
)

// GET /pos/menu — documento denormalizado del catálogo. Redis primero; si falla,
// lo arma desde Postgres y lo cachea.
func (h *Handlers) PosMenu(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if b, ok := h.menuCache.Get(ctx); ok {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		_, _ = w.Write(b)
		return
	}
	doc, err := h.menu.Build(ctx)
	if err != nil {
		Error(w, err)
		return
	}
	b, err := json.Marshal(doc)
	if err != nil {
		Error(w, err)
		return
	}
	h.menuCache.Set(ctx, b)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write(b)
}

// GET /pos/popular — IDs de producto más vendidos (read model aparte, TTL corto).
// Redis primero; si falla, lo calcula desde Postgres y lo cachea 5 min.
func (h *Handlers) PosPopular(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if b, ok := h.menuCache.GetPopular(ctx); ok {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		_, _ = w.Write(b)
		return
	}
	ids, err := h.menu.Popular(ctx)
	if err != nil {
		Error(w, err)
		return
	}
	b, err := json.Marshal(map[string]any{"items": ids})
	if err != nil {
		Error(w, err)
		return
	}
	h.menuCache.SetPopular(ctx, b)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write(b)
}

// GET /pos/modifier-defaults — opciones de modificador más probables por contexto
// (producto→grupo→[optionId rankeadas]). El POS las usa para preseleccionar.
func (h *Handlers) ModifierDefaults(w http.ResponseWriter, r *http.Request) {
	defaults, err := h.suggest.Defaults(r.Context())
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, defaults)
}

// GET /products/{id}/costing — costo calculado del producto (desglose simple).
func (h *Handlers) ProductCosting(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		Error(w, err)
		return
	}
	cost, err := h.costing.ProductCost(r.Context(), id)
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, map[string]any{"productId": id, "cost": cost})
}
