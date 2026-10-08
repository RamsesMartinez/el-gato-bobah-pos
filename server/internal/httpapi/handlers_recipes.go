package httpapi

import (
	"net/http"
	"strconv"
	"time"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
)

// Catálogo › Recetas (spec 028).

// GET /admin/recipes?kind=&status=&sort=&q=&category=&limit=&offset=
//
// Todo valor desconocido llega a domain.RecipeFilter.Validate y se rechaza; solo el AUSENTE toma
// su valor por omisión.
func (h *Handlers) ListRecipes(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := domain.RecipeFilter{
		Kind:   domain.RecipeKind(orDefault(q.Get("kind"), string(domain.RecipeKindProduct))),
		Status: domain.RecipeStatus(orDefault(q.Get("status"), string(domain.RecipeStatusPending))),
		Sort:   domain.RecipeSort(orDefault(q.Get("sort"), string(domain.RecipeSortSales))),
		Query:  q.Get("q"),
		Limit:  25,
	}
	for name, dst := range map[string]*int{"limit": &f.Limit, "offset": &f.Offset} {
		if v := q.Get(name); v != "" {
			n, err := strconv.Atoi(v)
			if err != nil {
				Error(w, domain.ErrValidation)
				return
			}
			*dst = n
		}
	}
	if v := q.Get("category"); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			Error(w, domain.ErrValidation)
			return
		}
		f.Category = n
	}
	page, err := h.admin.ListRecipes(r.Context(), f, time.Now())
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, page)
}

// POST /admin/recipes/confirm {kind, ids} — confirma las estimadas que la pantalla tiene a la vista.
func (h *Handlers) ConfirmRecipes(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Kind domain.RecipeKind `json:"kind"`
		IDs  []int64           `json:"ids"`
	}
	if err := Decode(r, &body); err != nil {
		Error(w, err)
		return
	}
	u, _ := userFrom(r.Context())
	n, err := h.admin.ConfirmRecipes(r.Context(), body.Kind, body.IDs, u.ID)
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, map[string]any{"confirmed": n})
}

func orDefault(v, def string) string {
	if v == "" {
		return def
	}
	return v
}
