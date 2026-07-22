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
	u, _ := userFrom(ctx)
	if b, ok := h.menuCache.Get(ctx, u.CompanyID); ok {
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
	h.menuCache.Set(ctx, u.CompanyID, b)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write(b)
}

// GET /pos/popular — IDs de producto más vendidos (read model aparte, TTL corto).
// Redis primero; si falla, lo calcula desde Postgres y lo cachea 5 min.
func (h *Handlers) PosPopular(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	u, _ := userFrom(ctx)
	if b, ok := h.menuCache.GetPopular(ctx, u.CompanyID); ok {
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
	h.menuCache.SetPopular(ctx, u.CompanyID, b)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write(b)
}

// GET /pos/modifier-defaults — opciones de modificador más probables por contexto
// (producto→grupo→[optionId rankeadas]). El POS las usa para preseleccionar.
func (h *Handlers) ModifierDefaults(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	defaults, err := h.suggest.Defaults(r.Context(), u.CompanyID)
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, defaults)
}

// POST /admin/reload — recarga el estado en caché sin reiniciar el proceso: menú,
// popularidad y el memo del recomendador. Útil tras editar la BD por fuera
// (migraciones / reorg SQL), donde el flujo normal del admin no invalida la caché.
// OJO: recarga ESTADO, no CÓDIGO — cambios de lógica siguen requiriendo recompilar.
func (h *Handlers) AdminReload(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	u, _ := userFrom(ctx)
	h.menuCache.Invalidate(ctx, u.CompanyID)
	h.menuCache.InvalidatePopular(ctx, u.CompanyID)
	h.suggest.Invalidate(u.CompanyID)
	JSON(w, http.StatusOK, map[string]any{"reloaded": []string{"menu", "popular", "recommendations"}})
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
