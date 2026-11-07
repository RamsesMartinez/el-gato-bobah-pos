package httpapi

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
)

// Roles por empresa (spec 068). Handlers finos: las reglas viven en domain y app.

func roleIDParam(r *http.Request) (int64, error) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		return 0, domain.ErrValidation
	}
	return id, nil
}

// GET /permissions
func (h *Handlers) PermissionCatalog(w http.ResponseWriter, _ *http.Request) {
	JSON(w, http.StatusOK, map[string]any{"items": h.roles.Catalog()})
}

// GET /roles
func (h *Handlers) ListRoles(w http.ResponseWriter, r *http.Request) {
	items, err := h.roles.List(r.Context())
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, map[string]any{"items": items})
}

// POST /roles  {name, permissions}
func (h *Handlers) CreateRole(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name        string              `json:"name"`
		Permissions []domain.Permission `json:"permissions"`
	}
	if err := Decode(r, &body); err != nil {
		Error(w, err)
		return
	}
	id, err := h.roles.Create(r.Context(), h.actorPerms(r), body.Name, body.Permissions)
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusCreated, map[string]any{"id": id})
}

// PATCH /roles/{id}  {version, name?, add?, remove?}
func (h *Handlers) UpdateRole(w http.ResponseWriter, r *http.Request) {
	id, err := roleIDParam(r)
	if err != nil {
		Error(w, err)
		return
	}
	var body struct {
		Version int32               `json:"version"`
		Name    *string             `json:"name"`
		Add     []domain.Permission `json:"add"`
		Remove  []domain.Permission `json:"remove"`
	}
	if err := Decode(r, &body); err != nil {
		Error(w, err)
		return
	}
	if err := h.roles.Update(r.Context(), app.UpdateRoleCmd{Actor: h.actorPerms(r), ID: id, Version: body.Version, Name: body.Name,
		Add: body.Add, Remove: body.Remove}); err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, map[string]any{"id": id})
}

func (h *Handlers) roleVersionAction(w http.ResponseWriter, r *http.Request, do func(id int64, version int32) error) {
	id, err := roleIDParam(r)
	if err != nil {
		Error(w, err)
		return
	}
	var body struct {
		Version int32 `json:"version"`
	}
	if err := Decode(r, &body); err != nil {
		Error(w, err)
		return
	}
	if err := do(id, body.Version); err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, map[string]any{"id": id})
}

// POST /roles/{id}/reset  {version}
func (h *Handlers) ResetRole(w http.ResponseWriter, r *http.Request) {
	h.roleVersionAction(w, r, func(id int64, v int32) error { return h.roles.Reset(r.Context(), h.actorPerms(r), id, v) })
}

// POST /roles/{id}/deactivate  {version}
func (h *Handlers) DeactivateRole(w http.ResponseWriter, r *http.Request) {
	h.roleVersionAction(w, r, func(id int64, v int32) error { return h.roles.SetActive(r.Context(), h.actorPerms(r), id, v, false) })
}

// POST /roles/{id}/reactivate  {version}
func (h *Handlers) ReactivateRole(w http.ResponseWriter, r *http.Request) {
	h.roleVersionAction(w, r, func(id int64, v int32) error { return h.roles.SetActive(r.Context(), h.actorPerms(r), id, v, true) })
}

// DELETE /roles/{id}
func (h *Handlers) DeleteRole(w http.ResponseWriter, r *http.Request) {
	id, err := roleIDParam(r)
	if err != nil {
		Error(w, err)
		return
	}
	if err := h.roles.Delete(r.Context(), h.actorPerms(r), id); err != nil {
		Error(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
