package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
)

// categoryPatchBody es el PATCH de una categoría ya leído: la familia (la pantalla, spec 091) o el
// nombre, el estado y el padre (el MCP, spec 099). No se mezclan: van a servicios distintos y en
// transacciones distintas, y mezclarlos podía guardar la mitad.
type categoryPatchBody struct {
	HasFamily bool
	Family    string
	Patch     app.CategoryPatch
}

// decodeCategoryPatch distingue «no mandé el padre» de «quítale el padre» (`"parentId": null`).
func decodeCategoryPatch(r *http.Request) (categoryPatchBody, error) {
	var body struct {
		FoodFamily json.RawMessage `json:"foodFamily"`
		Name       *string         `json:"name"`
		IsActive   *bool           `json:"isActive"`
		ParentID   json.RawMessage `json:"parentId"`
	}
	var out categoryPatchBody
	if err := DecodeStrict(r, &body); err != nil {
		return out, err
	}
	tree := body.Name != nil || body.IsActive != nil || len(body.ParentID) > 0
	if len(body.FoodFamily) > 0 {
		if tree {
			return out, fmt.Errorf("%w: la familia se cambia sola, sin nombre, estado ni padre", domain.ErrValidation)
		}
		var family *string
		if err := json.Unmarshal(body.FoodFamily, &family); err != nil {
			return out, fmt.Errorf("%w: foodFamily debe ser texto o null", domain.ErrValidation)
		}
		out.HasFamily = true
		if family != nil {
			out.Family = *family
		}
		return out, nil
	}
	if !tree {
		return out, fmt.Errorf("%w: no hay nada que cambiar (name, isActive, parentId o foodFamily)", domain.ErrValidation)
	}
	out.Patch = app.CategoryPatch{Name: body.Name, Active: body.IsActive}
	if len(body.ParentID) > 0 {
		out.Patch.SetParent = true
		if string(body.ParentID) != "null" {
			var id int64
			if err := json.Unmarshal(body.ParentID, &id); err != nil || id <= 0 {
				return out, fmt.Errorf("%w: parentId es el id de una categoría o null", domain.ErrValidation)
			}
			out.Patch.ParentID = &id
		}
	}
	return out, nil
}

func decodeCategoryCreate(r *http.Request) (string, *int64, error) {
	var body struct {
		Name     string `json:"name"`
		ParentID *int64 `json:"parentId"`
	}
	if err := DecodeStrict(r, &body); err != nil {
		return "", nil, err
	}
	return body.Name, body.ParentID, nil
}

// PATCH /admin/categories/{id} — menu.configure. La familia (`{"foodFamily":"coffee"}` o null para
// volver a la genérica), o nombre, estado y padre. Sin nada que cambiar se rechaza: un PATCH vacío
// que respondiera 200 haría creer que se guardó algo.
func (h *Handlers) PatchCategory(w http.ResponseWriter, r *http.Request) {
	u, ok := userFrom(r.Context())
	if !ok {
		Error(w, domain.ErrUnauthorized)
		return
	}
	id, err := entityID(r)
	if err != nil {
		Error(w, err)
		return
	}
	body, err := decodeCategoryPatch(r)
	if err != nil {
		Error(w, err)
		return
	}
	if body.HasFamily {
		if err := h.media.SetCategoryFamily(r.Context(), id, body.Family, u.ID); err != nil {
			Error(w, err)
			return
		}
		JSON(w, http.StatusOK, map[string]any{"id": id, "foodFamily": body.Family})
		return
	}
	row, err := h.admin.UpdateCategory(r.Context(), id, body.Patch, u.ID)
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, row)
}

// POST /admin/categories — menu.configure. Alta de una categoría (el MCP, spec 099).
func (h *Handlers) CreateCategory(w http.ResponseWriter, r *http.Request) {
	u, ok := userFrom(r.Context())
	if !ok {
		Error(w, domain.ErrUnauthorized)
		return
	}
	name, parent, err := decodeCategoryCreate(r)
	if err != nil {
		Error(w, err)
		return
	}
	row, err := h.admin.CreateCategory(r.Context(), name, parent, u.ID)
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusCreated, row)
}
