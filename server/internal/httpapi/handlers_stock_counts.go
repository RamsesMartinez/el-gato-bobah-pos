package httpapi

import (
	"net/http"

	"github.com/shopspring/decimal"
)

// Conteo de existencias (spec 038, historia 3). Las cifras van en la unidad base del insumo.

// GET /stock/counts/current — el conteo abierto de la sucursal, o 204 si no hay.
func (h *Handlers) CurrentCount(w http.ResponseWriter, r *http.Request) {
	v, err := h.backoffice.CurrentCount(r.Context())
	if err != nil {
		Error(w, err)
		return
	}
	if v == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	JSON(w, http.StatusOK, v)
}

// POST /stock/counts — abre un conteo; con uno abierto en la sucursal, 409.
func (h *Handlers) OpenCount(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	v, err := h.backoffice.OpenCount(r.Context(), u.ID)
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusCreated, v)
}

// GET /stock/counts/last-closed — `{ closedAt }` del último conteo cerrado, o null.
func (h *Handlers) LastClosedCount(w http.ResponseWriter, r *http.Request) {
	last, err := h.backoffice.LastClosedCount(r.Context())
	if err != nil {
		Error(w, err)
		return
	}
	if last == nil {
		JSON(w, http.StatusOK, map[string]any{"closedAt": nil, "id": nil})
		return
	}
	JSON(w, http.StatusOK, map[string]any{"closedAt": last.ClosedAt, "id": last.ID})
}

// PUT /stock/counts/{id}/lines/{ingredientId} — `{ countedQty }`.
func (h *Handlers) CaptureCount(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r, "id")
	if err != nil {
		Error(w, err)
		return
	}
	ing, err := idParam(r, "ingredientId")
	if err != nil {
		Error(w, err)
		return
	}
	var body struct {
		CountedQty decimal.Decimal `json:"countedQty"`
	}
	if err := Decode(r, &body); err != nil {
		Error(w, err)
		return
	}
	u, _ := userFrom(r.Context())
	if err := h.backoffice.CaptureCount(r.Context(), id, ing, body.CountedQty, u.ID); err != nil {
		Error(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// DELETE /stock/counts/{id}/lines/{ingredientId} — vuelve a «no contado».
func (h *Handlers) UncaptureCount(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r, "id")
	if err != nil {
		Error(w, err)
		return
	}
	ing, err := idParam(r, "ingredientId")
	if err != nil {
		Error(w, err)
		return
	}
	if err := h.backoffice.UncaptureCount(r.Context(), id, ing); err != nil {
		Error(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// GET /stock/counts/{id}/preview — lo que haría cerrar, sin escribir.
func (h *Handlers) PreviewCount(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r, "id")
	if err != nil {
		Error(w, err)
		return
	}
	v, err := h.backoffice.PreviewCount(r.Context(), id)
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, v)
}

// POST /stock/counts/{id}/close — ajusta por la diferencia y cierra.
func (h *Handlers) CloseCount(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r, "id")
	if err != nil {
		Error(w, err)
		return
	}
	u, _ := userFrom(r.Context())
	v, err := h.backoffice.CloseCount(r.Context(), id, u.ID)
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, v)
}

// POST /stock/counts/{id}/discard — sin tocar el almacén.
func (h *Handlers) DiscardCount(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r, "id")
	if err != nil {
		Error(w, err)
		return
	}
	u, _ := userFrom(r.Context())
	if err := h.backoffice.DiscardCount(r.Context(), id, u.ID); err != nil {
		Error(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// PUT /stock/ingredients/{id}/place — `{ placeId }`: un lugar de la lista (spec 040), o null.
func (h *Handlers) SetCountPlace(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r, "id")
	if err != nil {
		Error(w, err)
		return
	}
	var body struct {
		PlaceID *int64 `json:"placeId"`
	}
	if err := Decode(r, &body); err != nil {
		Error(w, err)
		return
	}
	if err := h.backoffice.SetCountPlace(r.Context(), id, body.PlaceID); err != nil {
		Error(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
