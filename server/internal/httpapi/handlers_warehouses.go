package httpapi

import (
	"fmt"
	"net/http"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
)

// boolQuery lee un interruptor de la URL: ausente es falso; presente tiene que ser 1/0 o true/false.
// Un valor que no se entiende se rechaza en vez de leerse como falso (constitución V).
func boolQuery(r *http.Request, name string) (bool, error) {
	switch r.URL.Query().Get(name) {
	case "":
		return false, nil
	case "1", "true":
		return true, nil
	case "0", "false":
		return false, nil
	}
	return false, fmt.Errorf("%w: %s inválido", domain.ErrValidation, name)
}

// GET /stock/warehouses — los almacenes de la sucursal; `?includeArchived=1` suma los archivados.
func (h *Handlers) ListWarehouses(w http.ResponseWriter, r *http.Request) {
	include, err := boolQuery(r, "includeArchived")
	if err != nil {
		Error(w, err)
		return
	}
	out, err := h.backoffice.ListWarehouses(r.Context(), include)
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, map[string]any{"items": out})
}

// POST /stock/warehouses — `{ name }`. 409 WAREHOUSE_NAME_TAKEN si ya hay uno activo con ese nombre.
func (h *Handlers) CreateWarehouse(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name string `json:"name"`
	}
	if err := Decode(r, &body); err != nil {
		Error(w, err)
		return
	}
	id, err := h.backoffice.CreateWarehouse(r.Context(), body.Name)
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusCreated, map[string]int64{"id": id})
}

// PATCH /stock/warehouses/{id} — `{ name }` y/o `{ isSales: true }`. Quitar la marca de venta no se
// pide: se le da a otro. Sin ninguno de los dos es 400, no un éxito que no hizo nada.
func (h *Handlers) RenameWarehouse(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r, "id")
	if err != nil {
		Error(w, err)
		return
	}
	var body struct {
		Name    *string `json:"name"`
		IsSales *bool   `json:"isSales"`
	}
	if err := Decode(r, &body); err != nil {
		Error(w, err)
		return
	}
	if body.Name == nil && body.IsSales == nil || body.IsSales != nil && !*body.IsSales {
		Error(w, fmt.Errorf("%w: manda el nombre o isSales: true", domain.ErrValidation))
		return
	}
	if err := h.backoffice.UpdateWarehouse(r.Context(), id, body.Name, body.IsSales != nil); err != nil {
		Error(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// POST /stock/warehouses/{id}/archive — 409 si es el de venta, tiene existencias o un conteo abierto.
func (h *Handlers) ArchiveWarehouse(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r, "id")
	if err != nil {
		Error(w, err)
		return
	}
	if err := h.backoffice.ArchiveWarehouse(r.Context(), id); err != nil {
		Error(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// POST /stock/warehouses/{id}/restore.
func (h *Handlers) RestoreWarehouse(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r, "id")
	if err != nil {
		Error(w, err)
		return
	}
	if err := h.backoffice.RestoreWarehouse(r.Context(), id); err != nil {
		Error(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
