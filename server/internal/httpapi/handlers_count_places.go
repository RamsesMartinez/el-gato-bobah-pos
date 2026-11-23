package httpapi

import (
	"fmt"
	"net/http"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
)

// GET /stock/count-places[?warehouseId=] — los lugares del almacén (el de venta sin él), en el
// orden del recorrido.
func (h *Handlers) ListCountPlaces(w http.ResponseWriter, r *http.Request) {
	sel, err := warehouseFromQuery(r, false)
	if err != nil {
		Error(w, err)
		return
	}
	out, err := h.backoffice.ListCountPlaces(r.Context(), sel)
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, out)
}

// POST /stock/count-places — `{ name, warehouseId? }`: al final del recorrido del almacén. 409 con el
// que ya existe en ese almacén.
func (h *Handlers) CreateCountPlace(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name        string `json:"name"`
		WarehouseID *int64 `json:"warehouseId"`
	}
	if err := DecodeStrict(r, &body); err != nil {
		Error(w, err)
		return
	}
	sel, err := domain.WarehouseFromBody(body.WarehouseID)
	if err != nil {
		Error(w, err)
		return
	}
	id, err := h.backoffice.CreateCountPlace(r.Context(), sel, body.Name)
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusCreated, map[string]int64{"id": id})
}

// PUT /stock/count-places/{id} — `{ name }`.
func (h *Handlers) RenameCountPlace(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r, "id")
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
	if err := h.backoffice.RenameCountPlace(r.Context(), id, body.Name); err != nil {
		Error(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// POST /stock/count-places/{id}/move — `{ toPosition }`: el ordinal entre los activos (0 = primero).
func (h *Handlers) MoveCountPlace(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r, "id")
	if err != nil {
		Error(w, err)
		return
	}
	var body struct {
		ToPosition *int `json:"toPosition"`
	}
	if err := Decode(r, &body); err != nil {
		Error(w, err)
		return
	}
	// Ausente no es «al principio»: un cliente viejo que manda otra llave movería el lugar sin querer.
	if body.ToPosition == nil {
		Error(w, fmt.Errorf("%w: falta a qué posición se mueve", domain.ErrValidation))
		return
	}
	if err := h.backoffice.MoveCountPlace(r.Context(), id, *body.ToPosition); err != nil {
		Error(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// POST /stock/count-places/{id}/archive — `{ moveTo }`: a dónde pasan sus artículos (null = sin
// lugar). Sin la llave y con artículos, 409 con cuántos tiene.
func (h *Handlers) ArchiveCountPlace(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r, "id")
	if err != nil {
		Error(w, err)
		return
	}
	var body map[string]*int64
	if err := Decode(r, &body); err != nil {
		Error(w, err)
		return
	}
	dest, decided := body["moveTo"]
	if decided {
		err = h.backoffice.ArchiveCountPlaceMoving(r.Context(), id, dest)
	} else {
		err = h.backoffice.ArchiveCountPlace(r.Context(), id)
	}
	if err != nil {
		Error(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
