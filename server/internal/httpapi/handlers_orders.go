package httpapi

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/logging"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/realtime"
)

type createOrderBody struct {
	ClientUUID         string  `json:"clientUuid"`
	ServiceType        string  `json:"serviceType"`
	DeliveryPlatformID *int16  `json:"deliveryPlatformId"`
	CustomerName       *string `json:"customerName"`
	Notes              *string `json:"notes"`
	Lines              []struct {
		ProductID int64           `json:"productId"`
		Qty       decimal.Decimal `json:"qty"`
		Notes     string          `json:"notes"`
		Modifiers []struct {
			OptionID int64  `json:"optionId"`
			Qty      int    `json:"qty"`
			Portion  string `json:"portion"`
		} `json:"modifiers"`
	} `json:"lines"`
	Payment *struct {
		MethodID  int16           `json:"methodId"`
		Amount    decimal.Decimal `json:"amount"`
		Tip       decimal.Decimal `json:"tip"`
		Reference *string         `json:"reference"`
	} `json:"payment"`
}

// POST /orders
func (h *Handlers) CreateOrder(w http.ResponseWriter, r *http.Request) {
	var body createOrderBody
	if err := Decode(r, &body); err != nil {
		Error(w, err)
		return
	}
	cid, err := uuid.Parse(body.ClientUUID)
	if err != nil {
		Error(w, domain.ErrValidation)
		return
	}
	u, _ := userFrom(r.Context())

	cmd := app.CreateOrderCmd{
		ClientUUID:         cid,
		ServiceType:        body.ServiceType,
		DeliveryPlatformID: body.DeliveryPlatformID,
		CustomerName:       body.CustomerName,
		Notes:              body.Notes,
		OpenedBy:           u.ID,
	}
	for _, l := range body.Lines {
		line := domain.OrderLineInput{ProductID: l.ProductID, Qty: l.Qty, Notes: l.Notes}
		for _, m := range l.Modifiers {
			line.Modifiers = append(line.Modifiers, domain.OrderModInput{OptionID: m.OptionID, Qty: m.Qty, Portion: m.Portion})
		}
		cmd.Lines = append(cmd.Lines, line)
	}
	if body.Payment != nil {
		cmd.Payment = &app.PaymentInput{
			MethodID: body.Payment.MethodID, Amount: body.Payment.Amount,
			Tip: body.Payment.Tip, Reference: body.Payment.Reference,
		}
	}

	order, err := h.orders.Create(r.Context(), cmd)
	if err != nil {
		Error(w, err)
		return
	}
	h.suggest.Invalidate() // el pedido nuevo debe reflejarse en las recomendaciones al instante
	h.broker.Publish(realtime.Event{Type: "order.created", Data: order})
	JSON(w, http.StatusCreated, order)
}

// POST /orders/{id}/status  {status}
func (h *Handlers) SetOrderStatus(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		Error(w, err)
		return
	}
	var body struct {
		Status string `json:"status"`
	}
	if err := Decode(r, &body); err != nil {
		Error(w, err)
		return
	}
	if err := h.orders.SetStatus(r.Context(), id, body.Status); err != nil {
		Error(w, err)
		return
	}
	h.broker.Publish(realtime.Event{Type: "order.updated", Data: map[string]any{"id": id, "status": body.Status}})
	w.WriteHeader(http.StatusNoContent)
}

// POST /orders/{id}/cancel  {reason}
func (h *Handlers) CancelOrder(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		Error(w, err)
		return
	}
	var body struct {
		Reason string `json:"reason"`
	}
	if err := Decode(r, &body); err != nil {
		Error(w, err)
		return
	}
	u, _ := userFrom(r.Context())
	if err := h.orders.Cancel(r.Context(), id, u.ID, body.Reason); err != nil {
		Error(w, err)
		return
	}
	h.broker.Publish(realtime.Event{Type: "order.updated", Data: map[string]any{"id": id, "status": "cancelada"}})
	w.WriteHeader(http.StatusNoContent)
}

// POST /orders/{id}/refund  {reason}  — reembolsa una orden entregada (admin/gerente).
func (h *Handlers) RefundOrder(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		Error(w, err)
		return
	}
	var body struct {
		Reason string `json:"reason"`
	}
	if err := Decode(r, &body); err != nil {
		Error(w, err)
		return
	}
	u, _ := userFrom(r.Context())
	if err := h.orders.Refund(r.Context(), id, u.ID, body.Reason); err != nil {
		Error(w, err)
		return
	}
	// Salida de dinero: evento de seguridad para detección/auditoría (quién reembolsó qué).
	logging.SecurityEvent(r.Context(), "order_refund", "order_id", id, "user_id", u.ID)
	h.broker.Publish(realtime.Event{Type: "order.updated", Data: map[string]any{"id": id, "status": "reembolsada"}})
	w.WriteHeader(http.StatusNoContent)
}

// GET /orders (tablero de activas)
func (h *Handlers) ListOrders(w http.ResponseWriter, r *http.Request) {
	orders, err := h.orders.Board(r.Context())
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, map[string]any{"items": orders})
}

// GET /orders/{id}
func (h *Handlers) GetOrder(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		Error(w, err)
		return
	}
	order, err := h.orders.Detail(r.Context(), id)
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, order)
}
