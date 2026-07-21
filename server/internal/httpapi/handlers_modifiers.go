package httpapi

import (
	"context"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/realtime"
)

// menuChanged: invalida la caché del menú y avisa a las tablets tras un cambio de catálogo.
func (h *Handlers) menuChanged(ctx context.Context) {
	h.menuCache.Invalidate(ctx)
	h.broker.Publish(realtime.Event{Type: "menu.updated"})
}

func urlID(r *http.Request, key string) (int64, error) {
	return strconv.ParseInt(chi.URLParam(r, key), 10, 64)
}

// --- Grupos (catálogo) -----------------------------------------------------

// GET /admin/groups?status=&search=&sort=&dir=&limit=&offset=
func (h *Handlers) AdminListGroups(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	status := q.Get("status")
	if status != "act" && status != "inact" {
		status = ""
	}
	sort := q.Get("sort") // name | options | products
	if sort != "options" && sort != "products" {
		sort = "name"
	}
	dir := q.Get("dir")
	if dir != "desc" {
		dir = "asc"
	}
	limit := clampInt(atoiOr(q.Get("limit"), 25), 0, 100)
	offset := atoiOr(q.Get("offset"), 0)
	if offset < 0 {
		offset = 0
	}
	page, err := h.admin.ListGroups(r.Context(), status, q.Get("search"), sort, dir, int32(limit), int32(offset))
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, page)
}

// POST /admin/groups  {name, defaultMin, defaultMax}
func (h *Handlers) AdminCreateGroup(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name       string `json:"name"`
		DefaultMin int    `json:"defaultMin"`
		DefaultMax int    `json:"defaultMax"`
	}
	if err := Decode(r, &body); err != nil {
		Error(w, err)
		return
	}
	if body.DefaultMax == 0 {
		body.DefaultMax = 1
	}
	id, err := h.admin.CreateGroup(r.Context(), body.Name, body.DefaultMin, body.DefaultMax)
	if err != nil {
		Error(w, err)
		return
	}
	h.menuChanged(r.Context())
	JSON(w, http.StatusCreated, map[string]any{"id": id})
}

// PATCH /admin/groups/{id}  {name, active, defaultMin, defaultMax}
func (h *Handlers) AdminUpdateGroup(w http.ResponseWriter, r *http.Request) {
	id, err := urlID(r, "id")
	if err != nil {
		Error(w, err)
		return
	}
	var body struct {
		Name       string `json:"name"`
		Active     bool   `json:"active"`
		DefaultMin int    `json:"defaultMin"`
		DefaultMax int    `json:"defaultMax"`
	}
	if err := Decode(r, &body); err != nil {
		Error(w, err)
		return
	}
	if body.DefaultMax == 0 {
		body.DefaultMax = 1
	}
	if err := h.admin.UpdateGroup(r.Context(), id, body.Name, body.Active, body.DefaultMin, body.DefaultMax); err != nil {
		Error(w, err)
		return
	}
	h.menuChanged(r.Context())
	w.WriteHeader(http.StatusNoContent)
}

// GET /admin/groups/{id}/options
func (h *Handlers) AdminGroupOptions(w http.ResponseWriter, r *http.Request) {
	id, err := urlID(r, "id")
	if err != nil {
		Error(w, err)
		return
	}
	items, err := h.admin.GroupOptions(r.Context(), id)
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, map[string]any{"items": items})
}

// POST /admin/groups/{id}/options  {name, priceDelta, maxPerLine}
func (h *Handlers) AdminCreateOption(w http.ResponseWriter, r *http.Request) {
	id, err := urlID(r, "id")
	if err != nil {
		Error(w, err)
		return
	}
	var body struct {
		Name       string          `json:"name"`
		PriceDelta decimal.Decimal `json:"priceDelta"`
		MaxPerLine int             `json:"maxPerLine"`
	}
	if err := Decode(r, &body); err != nil {
		Error(w, err)
		return
	}
	if body.MaxPerLine == 0 {
		body.MaxPerLine = 1
	}
	optID, err := h.admin.CreateOption(r.Context(), id, body.Name, body.PriceDelta, body.MaxPerLine)
	if err != nil {
		Error(w, err)
		return
	}
	h.menuChanged(r.Context())
	JSON(w, http.StatusCreated, map[string]any{"id": optID})
}

// POST /admin/groups/{id}/options/reorder  {ids:[...]} — fija el orden de las opciones.
func (h *Handlers) AdminReorderOptions(w http.ResponseWriter, r *http.Request) {
	id, err := urlID(r, "id")
	if err != nil {
		Error(w, err)
		return
	}
	var body struct {
		IDs []int64 `json:"ids"`
	}
	if err := Decode(r, &body); err != nil {
		Error(w, err)
		return
	}
	if err := h.admin.ReorderOptions(r.Context(), id, body.IDs); err != nil {
		Error(w, err)
		return
	}
	h.menuChanged(r.Context())
	w.WriteHeader(http.StatusNoContent)
}

// GET /admin/groups/{id}/products — productos que usan el grupo ("usado en N productos").
func (h *Handlers) AdminGroupProducts(w http.ResponseWriter, r *http.Request) {
	id, err := urlID(r, "id")
	if err != nil {
		Error(w, err)
		return
	}
	items, err := h.admin.GroupProducts(r.Context(), id)
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, map[string]any{"items": items})
}

// --- Producto ↔ grupos -----------------------------------------------------

// GET /admin/products/{id}/groups
func (h *Handlers) AdminProductGroups(w http.ResponseWriter, r *http.Request) {
	id, err := urlID(r, "id")
	if err != nil {
		Error(w, err)
		return
	}
	items, err := h.admin.ProductGroups(r.Context(), id)
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, map[string]any{"items": items})
}

// POST /admin/products/{id}/groups  {groupId, title, minSelect, maxSelect, position} — asigna o actualiza.
func (h *Handlers) AdminAttachProductGroup(w http.ResponseWriter, r *http.Request) {
	id, err := urlID(r, "id")
	if err != nil {
		Error(w, err)
		return
	}
	var body struct {
		GroupID   int64  `json:"groupId"`
		Title     string `json:"title"`
		Override  bool   `json:"override"` // false = hereda el default del grupo
		MinSelect int    `json:"minSelect"`
		MaxSelect int    `json:"maxSelect"`
		Position  int    `json:"position"`
	}
	if err := Decode(r, &body); err != nil {
		Error(w, err)
		return
	}
	if err := h.admin.AttachGroup(r.Context(), app.AttachGroupInput{
		ProductID: id, GroupID: body.GroupID, Title: body.Title, Override: body.Override,
		MinSelect: body.MinSelect, MaxSelect: body.MaxSelect, Position: body.Position,
	}); err != nil {
		Error(w, err)
		return
	}
	h.menuChanged(r.Context())
	w.WriteHeader(http.StatusNoContent)
}

// DELETE /admin/products/{id}/groups/{groupId}
func (h *Handlers) AdminDetachProductGroup(w http.ResponseWriter, r *http.Request) {
	id, err := urlID(r, "id")
	if err != nil {
		Error(w, err)
		return
	}
	groupID, err := urlID(r, "groupId")
	if err != nil {
		Error(w, err)
		return
	}
	if err := h.admin.DetachGroup(r.Context(), id, groupID); err != nil {
		Error(w, err)
		return
	}
	h.menuChanged(r.Context())
	w.WriteHeader(http.StatusNoContent)
}
