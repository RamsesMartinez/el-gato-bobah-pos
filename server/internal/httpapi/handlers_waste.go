package httpapi

import (
	"net/http"
	"slices"

	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/realtime"
)

// Lo que se echó a perder (spec 039). Handlers finos: la regla vive en app y domain.

// GET /waste/reasons — los motivos que se ofrecen, los más usados primero.
func (h *Handlers) WasteReasons(w http.ResponseWriter, r *http.Request) {
	v, err := h.backoffice.WasteReasons(r.Context())
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, v)
}

// POST /waste/reasons — `{ name, description }`; repetido → 409 con `details.existingId`.
func (h *Handlers) CreateWasteReason(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	if err := Decode(r, &body); err != nil {
		Error(w, err)
		return
	}
	u, _ := userFrom(r.Context())
	id, err := h.backoffice.CreateWasteReason(r.Context(), body.Name, body.Description, u.ID)
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusCreated, map[string]int64{"id": id})
}

// GET /waste/items — los artículos, productos y recientes que ofrece la captura.
func (h *Handlers) WasteItems(w http.ResponseWriter, r *http.Request) {
	v, err := h.backoffice.WasteItemsForCapture(r.Context())
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, v)
}

// POST /waste — registra lo que se echó a perder.
func (h *Handlers) RecordWaste(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ReasonID       int64           `json:"reasonId"`
		IngredientID   *int64          `json:"ingredientId"`
		ProductID      *int64          `json:"productId"`
		Qty            decimal.Decimal `json:"qty"`
		UnitID         *int16          `json:"unitId"`
		PresentationID *int64          `json:"presentationId"`
		Note           *string         `json:"note"`
	}
	if err := Decode(r, &body); err != nil {
		Error(w, err)
		return
	}
	u, _ := userFrom(r.Context())
	res, err := h.backoffice.RecordWaste(r.Context(), app.RecordWasteCmd{
		ReasonID: body.ReasonID, IngredientID: body.IngredientID, ProductID: body.ProductID, Qty: body.Qty,
		UnitID: body.UnitID, PresentationID: body.PresentationID, Note: body.Note, Actor: u.ID,
	})
	if err != nil {
		Error(w, err)
		return
	}
	if res.Status == string(domain.WastePending) {
		h.wastePendingChanged(r)
	}
	JSON(w, http.StatusCreated, res)
}

// GET /waste/today — lo registrado hoy en la sucursal.
func (h *Handlers) WasteToday(w http.ResponseWriter, r *http.Request) {
	v, err := h.backoffice.WasteToday(r.Context())
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, v)
}

// GET /waste/pending — las que esperan aprobación en la sucursal.
func (h *Handlers) PendingWaste(w http.ResponseWriter, r *http.Request) {
	v, err := h.backoffice.PendingWaste(r.Context())
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, v)
}

// POST /waste/{id}/approve y /reject.
func (h *Handlers) ApproveWaste(w http.ResponseWriter, r *http.Request) { h.resolveWaste(w, r, true) }

func (h *Handlers) RejectWaste(w http.ResponseWriter, r *http.Request) { h.resolveWaste(w, r, false) }

func (h *Handlers) resolveWaste(w http.ResponseWriter, r *http.Request, approve bool) {
	id, err := idParam(r, "id")
	if err != nil {
		Error(w, err)
		return
	}
	u, _ := userFrom(r.Context())
	if approve {
		err = h.backoffice.ApproveWaste(r.Context(), id, u.ID)
	} else {
		err = h.backoffice.RejectWaste(r.Context(), id, u.ID)
	}
	if err != nil {
		Error(w, err)
		return
	}
	h.wastePendingChanged(r)
	w.WriteHeader(http.StatusNoContent)
}

// POST /waste/{id}/undo — la deshace quien la registró, o quien configura el almacén.
func (h *Handlers) UndoWaste(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r, "id")
	if err != nil {
		Error(w, err)
		return
	}
	u, _ := userFrom(r.Context())
	canConfigure := slices.Contains(h.permissionsOf(u.Role), domain.PermInventoryConfigure)
	if err := h.backoffice.UndoWaste(r.Context(), id, u.ID, canConfigure); err != nil {
		Error(w, err)
		return
	}
	h.wastePendingChanged(r)
	w.WriteHeader(http.StatusNoContent)
}

// PUT /settings/waste-threshold — `{ threshold: "100" | null }`.
func (h *Handlers) SetWasteThreshold(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Threshold *string `json:"threshold"`
	}
	if err := Decode(r, &body); err != nil {
		Error(w, err)
		return
	}
	u, _ := userFrom(r.Context())
	if err := h.backoffice.SetWasteThreshold(r.Context(), body.Threshold, u.ID); err != nil {
		Error(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// wastePendingChanged avisa a las tabletas que cambió lo que espera aprobación, para que la franja
// y el número del riel se actualicen sin recargar.
func (h *Handlers) wastePendingChanged(r *http.Request) {
	u, _ := userFrom(r.Context())
	h.broker.Publish(u.CompanyID, realtime.Event{Type: "waste.pending_changed"})
}
