package httpapi

import (
	"net/http"

	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
)

// GET /business-settings — cualquier autenticado (el cobro necesita el costo de envío por defecto).
func (h *Handlers) BusinessSettings(w http.ResponseWriter, r *http.Request) {
	bs, err := h.settings.Get(r.Context())
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, bs)
}

// PUT /business-settings — solo admin/gerente (gateado en el router). Actualiza el costo de envío.
func (h *Handlers) UpdateBusinessSettings(w http.ResponseWriter, r *http.Request) {
	var body struct {
		DeliveryFee decimal.Decimal `json:"deliveryFee"`
	}
	if err := Decode(r, &body); err != nil {
		Error(w, err)
		return
	}
	u, ok := userFrom(r.Context())
	if !ok {
		Error(w, domain.ErrUnauthorized)
		return
	}
	bs, err := h.settings.SetDeliveryFee(r.Context(), body.DeliveryFee, u.ID)
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, bs)
}
