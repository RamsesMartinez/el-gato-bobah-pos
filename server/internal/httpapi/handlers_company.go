package httpapi

import (
	"net/http"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
)

// GET /company — la empresa del usuario autenticado (para mostrar nombre/slug en el panel).
func (h *Handlers) GetCompany(w http.ResponseWriter, r *http.Request) {
	u, ok := userFrom(r.Context())
	if !ok {
		Error(w, domain.ErrUnauthorized)
		return
	}
	co, err := h.company.Get(r.Context(), u.CompanyID)
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, co)
}

// PATCH /company  {name, slug} — solo admin/gerente (gateado en el router). Cambiar el slug
// afecta el login username@slug de todos los empleados de la empresa.
func (h *Handlers) UpdateCompany(w http.ResponseWriter, r *http.Request) {
	u, ok := userFrom(r.Context())
	if !ok {
		Error(w, domain.ErrUnauthorized)
		return
	}
	var body struct {
		Name string `json:"name"`
		Slug string `json:"slug"`
	}
	if err := Decode(r, &body); err != nil {
		Error(w, err)
		return
	}
	co, err := h.company.Update(r.Context(), u.CompanyID, body.Name, body.Slug)
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, co)
}
