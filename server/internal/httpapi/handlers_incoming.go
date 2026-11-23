package httpapi

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/realtime"
)

// Pedidos entrantes (spec 093, contracts/incoming-orders.md). Handlers finos: la regla de qué está
// pendiente y quién gana una carrera vive en el servicio.

// IncomingOrders lista lo que espera decisión, de todos los orígenes.
func (h *Handlers) IncomingOrders(w http.ResponseWriter, r *http.Request) {
	pedidos, err := h.incomingOrders.Pending(r.Context())
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, map[string]any{"orders": pedidos})
}

// incomingTarget lee el origen y el id de la ruta. Un origen desconocido o un id que no es un
// entero positivo se rechazan: nunca caen a «la plataforma» ni a un id cualquiera.
func incomingTarget(r *http.Request) (domain.IncomingSource, int64, error) {
	source, err := domain.ParseIncomingSource(chi.URLParam(r, "source"))
	if err != nil {
		return "", 0, err
	}
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		return "", 0, domain.ErrValidation
	}
	return source, id, nil
}

// AcceptIncomingOrder acepta un pedido entrante en su origen y lo mete al POS.
func (h *Handlers) AcceptIncomingOrder(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	source, id, err := incomingTarget(r)
	if err != nil {
		Error(w, err)
		return
	}
	pedido, err := h.incomingOrders.Accept(r.Context(), source, id, u.ID)
	if err != nil {
		Error(w, err)
		return
	}
	h.broker.Publish(u.CompanyID, realtime.Event{Type: "order.created", Data: pedido})
	h.publishIncomingDecided(u.CompanyID, id)
	JSON(w, http.StatusOK, pedido)
}

// RejectIncomingOrder rechaza un pedido entrante con el motivo que eligió quien opera.
func (h *Handlers) RejectIncomingOrder(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	source, id, err := incomingTarget(r)
	if err != nil {
		Error(w, err)
		return
	}
	var body struct {
		Reason string `json:"reason"`
	}
	// Un cuerpo ilegible es un motivo ausente: 400, nunca «otro motivo».
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		Error(w, domain.ErrValidation)
		return
	}
	if err := h.incomingOrders.Reject(r.Context(), source, id, u.ID, body.Reason); err != nil {
		Error(w, err)
		return
	}
	h.publishIncomingDecided(u.CompanyID, id)
	w.WriteHeader(http.StatusNoContent)
}

// publishIncomingDecided avisa que un pedido entrante dejó de estar pendiente: sin esto, las demás
// tabletas siguen sonando por un pedido que ya se atendió. Va también el evento viejo de la spec 021
// para las tabletas con el front anterior en caché; se retira en una subida posterior.
func (h *Handlers) publishIncomingDecided(company, id int64) {
	h.broker.Publish(company, realtime.Event{Type: "incoming.changed", Data: map[string]any{}})
	h.broker.Publish(company, realtime.Event{Type: "platform.order.decided", Data: map[string]any{"id": id}})
}
