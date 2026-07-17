package httpapi

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/realtime"
)

// GET /admin/products
func (h *Handlers) AdminListProducts(w http.ResponseWriter, r *http.Request) {
	items, err := h.admin.ListProducts(r.Context())
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, map[string]any{"items": items})
}

// PATCH /admin/products/{id}
func (h *Handlers) AdminUpdateProduct(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		Error(w, err)
		return
	}
	var body struct {
		Name           string  `json:"name"`
		Price          float64 `json:"price"`
		Favorite       bool    `json:"favorite"`
		Active         bool    `json:"active"`
		AvailableFrom  *string `json:"availableFrom"`
		AvailableUntil *string `json:"availableUntil"`
	}
	if err := Decode(r, &body); err != nil {
		Error(w, err)
		return
	}
	if err := h.admin.UpdateProduct(r.Context(), app.UpdateProductInput{
		ID: id, Name: body.Name, Price: body.Price, Favorite: body.Favorite, Active: body.Active,
		AvailableFrom: body.AvailableFrom, AvailableUntil: body.AvailableUntil,
	}); err != nil {
		Error(w, err)
		return
	}
	// el catálogo cambió: invalidar cache del menú y avisar a las tablets
	h.menuCache.Invalidate(r.Context())
	h.broker.Publish(realtime.Event{Type: "menu.updated"})
	w.WriteHeader(http.StatusNoContent)
}
