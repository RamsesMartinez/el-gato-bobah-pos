package httpapi

import (
	"fmt"
	"net/http"
	"strconv"
	"uuid"

	"github.com/go-chi/chi/v5"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/logging"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/realtime"
	"github.com/shopspring/decimal"
)

type createOrderBody struct {
	ClientUUID         string          `json:"clientUuid"`
	ServiceType        string          `json:"serviceType"`
	DeliveryPlatformID *int16          `json:"deliveryPlatformId"`
	CustomerName       *string         `json:"customerName"`
	Notes              *string         `json:"notes"`
	DeliveryFee        decimal.Decimal `json:"deliveryFee"`
	// folioName: el nombre que la pantalla ya le puso a la cuenta. El servidor lo sanea y resuelve
	// los choques del día, así que proponerlo no es decidirlo.
	FolioName string `json:"folioName"`
	Lines     []struct {
		ProductID int64           `json:"productId"`
		Qty       decimal.Decimal `json:"qty"`
		Notes     string          `json:"notes"`
		Modifiers []struct {
			OptionID int64  `json:"optionId"`
			Qty      int    `json:"qty"`
			Portion  string `json:"portion"`
		} `json:"modifiers"`
	} `json:"lines"`
	// payments: 0..N métodos (pago dividido). El front manda una línea por método.
	Payments []struct {
		MethodID  int16           `json:"methodId"`
		Amount    decimal.Decimal `json:"amount"`
		Tip       decimal.Decimal `json:"tip"`
		Reference *string         `json:"reference"`
	} `json:"payments"`
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
		DeliveryFee:        body.DeliveryFee,
		CompanyID:          u.CompanyID,
		FolioName:          body.FolioName,
	}
	for _, l := range body.Lines {
		line := domain.OrderLineInput{ProductID: l.ProductID, Qty: l.Qty, Notes: l.Notes}
		for _, m := range l.Modifiers {
			line.Modifiers = append(line.Modifiers, domain.OrderModInput{OptionID: m.OptionID, Qty: m.Qty, Portion: m.Portion})
		}
		cmd.Lines = append(cmd.Lines, line)
	}
	for _, p := range body.Payments {
		cmd.Payments = append(cmd.Payments, app.PaymentInput{
			MethodID: p.MethodID, Amount: p.Amount, Tip: p.Tip, Reference: p.Reference,
		})
	}

	order, err := h.orders.Create(r.Context(), cmd)
	if err != nil {
		Error(w, err)
		return
	}
	h.suggest.Invalidate(u.CompanyID) // el pedido nuevo debe reflejarse en las recomendaciones al instante
	h.broker.Publish(u.CompanyID, realtime.Event{Type: "order.created", Data: order})
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
	u, _ := userFrom(r.Context())
	h.broker.Publish(u.CompanyID, realtime.Event{Type: "order.updated", Data: map[string]any{"id": id, "status": body.Status}})
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
	h.broker.Publish(u.CompanyID, realtime.Event{Type: "order.updated", Data: map[string]any{"id": id, "status": "cancelada"}})
	w.WriteHeader(http.StatusNoContent)
}

// GET /orders/delivered — entregadas del día (superficie de reembolso, admin/gerente).
func (h *Handlers) DeliveredOrders(w http.ResponseWriter, r *http.Request) {
	items, err := h.orders.DeliveredToday(r.Context())
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, map[string]any{"items": items})
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
	h.broker.Publish(u.CompanyID, realtime.Event{Type: "order.updated", Data: map[string]any{"id": id, "status": "reembolsada"}})
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

type addLinesBody struct {
	Lines []struct {
		ProductID int64           `json:"productId"`
		Qty       decimal.Decimal `json:"qty"`
		Notes     string          `json:"notes"`
		Modifiers []struct {
			OptionID int64 `json:"optionId"`
			Qty      int   `json:"qty"`
		} `json:"modifiers"`
	} `json:"lines"`
}

// POST /orders/{id}/lines — agrega renglones a un pedido en curso.
//
// Ruta propia y no un PATCH del pedido: lo que se manda es un DELTA —lo que el cliente pidió de
// más—, no el pedido completo. Un PATCH invitaría a mandar la lista entera, y entonces el servidor
// tendría que adivinar qué renglón es nuevo y cuál ya estaba para no volver a descontar su stock.
func (h *Handlers) AddOrderLines(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		Error(w, domain.ErrValidation)
		return
	}
	var body addLinesBody
	if err := Decode(r, &body); err != nil {
		Error(w, err)
		return
	}
	u, _ := userFrom(r.Context())

	lines := make([]domain.OrderLineInput, 0, len(body.Lines))
	for _, l := range body.Lines {
		in := domain.OrderLineInput{ProductID: l.ProductID, Qty: l.Qty, Notes: l.Notes}
		for _, m := range l.Modifiers {
			in.Modifiers = append(in.Modifiers, domain.OrderModInput{OptionID: m.OptionID, Qty: m.Qty})
		}
		lines = append(lines, in)
	}

	order, err := h.orders.AddLines(r.Context(), id, lines, u.ID)
	if err != nil {
		Error(w, err)
		return
	}
	// El tablero tiene que enterarse: el pedido cambió de total y de contenido, y quien cocina lo
	// está mirando.
	h.broker.Publish(u.CompanyID, realtime.Event{Type: "order.updated", Data: map[string]any{"id": id}})
	JSON(w, http.StatusOK, order)
}

type deliverLineBody struct {
	Qty decimal.Decimal `json:"qty"`
}

// POST /orders/{id}/lines/{lineId}/deliver
func (h *Handlers) DeliverOrderLine(w http.ResponseWriter, r *http.Request) {
	orderID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		Error(w, domain.ErrValidation)
		return
	}
	lineID, err := strconv.ParseInt(chi.URLParam(r, "lineId"), 10, 64)
	if err != nil {
		Error(w, domain.ErrValidation)
		return
	}
	var body deliverLineBody
	if err := Decode(r, &body); err != nil {
		Error(w, err)
		return
	}
	// La cantidad lleva el mismo tope que una línea de venta: entregar cierra comida contra un
	// renglón, y un valor absurdo aquí llega a una columna numeric igual que en el cobro. Sin esto
	// un NaN o un 1e300 saldrían como 500 en vez de 400.
	if !domain.ValidQty(body.Qty, domain.MaxOrderQty, false) {
		Error(w, fmt.Errorf("%w: la cantidad a entregar no es válida", domain.ErrValidation))
		return
	}
	if err := h.orders.DeliverLine(r.Context(), orderID, lineID, body.Qty); err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// POST /orders/{id}/deliver
func (h *Handlers) DeliverOrder(w http.ResponseWriter, r *http.Request) {
	orderID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		Error(w, domain.ErrValidation)
		return
	}
	if err := h.orders.DeliverAll(r.Context(), orderID); err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// GET /pos/folio-names
//
// Sin caché HTTP a propósito: la lista cambia con el binario, y un max-age dejaría tabletas
// proponiendo un animal que el despliegue ya quitó. El cliente la guarda por sesión, que es
// exactamente lo que dura un binario para una tableta encendida.
func (h *Handlers) FolioNames(w http.ResponseWriter, r *http.Request) {
	JSON(w, http.StatusOK, map[string][]string{"items": domain.FolioNames()})
}

type chargeOrderBody struct {
	MethodID  int16           `json:"methodId"`
	Amount    decimal.Decimal `json:"amount"`
	Tip       decimal.Decimal `json:"tip"`
	Reference *string         `json:"reference"`
}

// POST /orders/{id}/pay
//
// Cobra un pedido que se mandó a cocina sin cobrar. El tablero lo marcaba "POR COBRAR" y no había
// con qué saldarlo: el único lugar que registraba un pago de pedido era la creación.
func (h *Handlers) ChargeOrder(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		Error(w, domain.ErrValidation)
		return
	}
	var body chargeOrderBody
	if err := Decode(r, &body); err != nil {
		Error(w, err)
		return
	}
	u, _ := userFrom(r.Context())
	if err := h.orders.Charge(r.Context(), app.ChargeCmd{
		OrderID: id, MethodID: body.MethodID, Amount: body.Amount, Tip: body.Tip,
		Reference: body.Reference, ActorID: u.ID,
	}); err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// GET /orders/open — la barra de pedidos en curso del POS.
//
// El total pendiente viaja junto a la lista y NO se suma en la pantalla: si cada lado lo calculara
// por su cuenta, un cambio en el predicado dejaría la cifra del encabezado diciendo una cosa y la
// lista otra, y quien la lee no tiene forma de saber cuál miente.
func (h *Handlers) OpenOrders(w http.ResponseWriter, r *http.Request) {
	items, pendiente, err := h.orders.Open(r.Context())
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, map[string]any{"items": items, "outstanding": pendiente})
}
