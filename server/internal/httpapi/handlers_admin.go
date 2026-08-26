package httpapi

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/realtime"
)

// GET /admin/products?status=&search=&limit=&offset=  (paginado en el backend)
func (h *Handlers) AdminListProducts(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	status := q.Get("status") // ""=todos | "act" | "inact"
	if status != "act" && status != "inact" {
		status = ""
	}
	limit := clampInt(atoiOr(q.Get("limit"), 25), 0, 100) // 0 = sin límite (POS modo edición)
	offset := max(atoiOr(q.Get("offset"), 0), 0)
	groups := q.Get("groups") // ""=todos | "none"=sin grupos | "some"=con grupos
	if groups != "none" && groups != "some" {
		groups = ""
	}
	categoryID := max(int64(atoiOr(q.Get("categoryId"), 0)), 0) // 0 = todas las categorías
	// Orden por columna: solo valores conocidos (lo demás → nombre asc, el default del query).
	sort := q.Get("sort")
	switch sort {
	case "name", "price", "cost", "margin", "category", "groups":
	default:
		sort = ""
	}
	dir := q.Get("dir")
	if dir != "desc" {
		dir = "asc"
	}
	page, err := h.admin.ListProducts(r.Context(), status, q.Get("search"), categoryID, groups, sort, dir, int32(limit), int32(offset))
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, page)
}

// GET /admin/categories — categorías activas para el filtro y el alta de productos.
func (h *Handlers) AdminCategories(w http.ResponseWriter, r *http.Request) {
	items, err := h.admin.Categories(r.Context())
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, map[string]any{"items": items})
}

// POST /admin/products — alta de producto (admin/gerente). Invalida la caché del menú.
func (h *Handlers) AdminCreateProduct(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name       string          `json:"name"`
		CategoryID int64           `json:"categoryId"`
		Price      decimal.Decimal `json:"price"`
		Favorite   bool            `json:"favorite"`
		TrackStock bool            `json:"trackStock"`
	}
	if err := Decode(r, &body); err != nil {
		Error(w, err)
		return
	}
	id, err := h.admin.CreateProduct(r.Context(), body.Name, body.CategoryID, body.Price, body.Favorite, body.TrackStock)
	if err != nil {
		Error(w, err)
		return
	}
	h.menuChanged(r.Context())
	JSON(w, http.StatusCreated, map[string]any{"id": id})
}

// POST /admin/products/{id}/duplicate — clona el producto con sus relaciones (admin/gerente).
func (h *Handlers) AdminDuplicateProduct(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		Error(w, err)
		return
	}
	var body struct {
		Name string `json:"name"`
	}
	if err := Decode(r, &body); err != nil {
		Error(w, err)
		return
	}
	newID, err := h.admin.DuplicateProduct(r.Context(), id, body.Name)
	if err != nil {
		Error(w, err)
		return
	}
	h.menuChanged(r.Context())
	JSON(w, http.StatusCreated, map[string]any{"id": newID})
}

func atoiOr(s string, def int) int {
	if n, err := strconv.Atoi(s); err == nil {
		return n
	}
	return def
}

func clampInt(n, lo, hi int) int {
	if n < lo {
		return lo
	}
	if n > hi {
		return hi
	}
	return n
}

// GET /admin/modifier-options?status=&search=&limit=&offset=  (paginado en el backend)
func (h *Handlers) AdminListModifierOptions(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	status := q.Get("status") // ""=todas | "act" | "inact"
	if status != "act" && status != "inact" {
		status = ""
	}
	limit := clampInt(atoiOr(q.Get("limit"), 25), 0, 100) // 0 = sin límite (el POS pide todas)
	offset := max(atoiOr(q.Get("offset"), 0), 0)
	page, err := h.admin.ListModifierOptions(r.Context(), status, q.Get("search"), int32(limit), int32(offset))
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, page)
}

// PATCH /admin/modifier-options/{id}  {favorite?, active?, name?, priceDelta?, maxPerLine?} — actualiza lo que venga.
func (h *Handlers) AdminUpdateOption(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		Error(w, err)
		return
	}
	var body struct {
		Favorite   *bool            `json:"favorite"`
		Active     *bool            `json:"active"`
		Name       *string          `json:"name"`
		PriceDelta *decimal.Decimal `json:"priceDelta"`
		MaxPerLine *int             `json:"maxPerLine"`
	}
	if err := Decode(r, &body); err != nil {
		Error(w, err)
		return
	}
	// edición de campos: nombre/precio/max llegan juntos desde el formulario
	if body.Name != nil {
		pd, mpl := decimal.Zero, 1
		if body.PriceDelta != nil {
			pd = *body.PriceDelta
		}
		if body.MaxPerLine != nil {
			mpl = *body.MaxPerLine
		}
		if err := h.admin.UpdateOptionFields(r.Context(), id, *body.Name, pd, mpl); err != nil {
			Error(w, err)
			return
		}
	}
	if body.Favorite != nil {
		if err := h.admin.SetOptionFavorite(r.Context(), id, *body.Favorite); err != nil {
			Error(w, err)
			return
		}
	}
	if body.Active != nil {
		if err := h.admin.SetOptionActive(r.Context(), id, *body.Active); err != nil {
			Error(w, err)
			return
		}
	}
	h.menuChanged(r.Context())
	w.WriteHeader(http.StatusNoContent)
}

// PATCH /admin/products/{id}
func (h *Handlers) AdminUpdateProduct(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		Error(w, err)
		return
	}
	var body struct {
		Name           string          `json:"name"`
		Price          decimal.Decimal `json:"price"`
		Favorite       bool            `json:"favorite"`
		Active         bool            `json:"active"`
		AvailableFrom  *string         `json:"availableFrom"`
		AvailableUntil *string         `json:"availableUntil"`
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
	// el catálogo cambió: invalidar cache del menú y avisar a las tablets (de esta empresa)
	u, _ := userFrom(r.Context())
	h.menuCache.Invalidate(r.Context(), u.CompanyID)
	h.broker.Publish(u.CompanyID, realtime.Event{Type: "menu.updated"})
	w.WriteHeader(http.StatusNoContent)
}
