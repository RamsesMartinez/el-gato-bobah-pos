package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"uuid"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
)

// Las herramientas de pedidos de la spec 090: capturar una cuenta, mandarla a cocina, entregarla,
// descontarla y cobrarla; y corregir un cobro (anular, cambiar de terminal, reembolsar). Igual que las
// de la 072–084, cada una manda el MISMO cuerpo que la pantalla a la MISMA ruta: el precio, el cobro y
// cada rechazo los decide el servicio. Lo propio de esta capa es la vista previa y los ids derivados de
// la clave, que hacen inocuo un reintento aunque el registro de idempotencia se pierda.

//nolint:gosec // G101: patrones de rutas; gosec confunde palabras con credenciales.
const (
	pDrafts          = "/api/v1/pos/drafts"
	pDraftQuote      = "/api/v1/pos/drafts/quote"
	pDraft           = "/api/v1/pos/drafts/{id}"
	pDraftSend       = "/api/v1/pos/drafts/{id}/send"
	pDraftDiscard    = "/api/v1/pos/drafts/{id}/discard"
	pOrderDeliver    = "/api/v1/orders/{id}/deliver"
	pOrderDiscount   = "/api/v1/orders/{id}/discount"
	pOrderPay        = "/api/v1/orders/{id}/pay"
	pOrderQuote      = "/api/v1/orders/{id}/quote"
	pPaymentVoid     = "/api/v1/orders/{id}/payments/{paymentId}/void"
	pPaymentTerminal = "/api/v1/orders/{id}/payments/{paymentId}/terminal"
	pOrderRefund     = "/api/v1/orders/{id}/refund"
	pOrderRefundInfo = "/api/v1/orders/{id}/refund-info"
	pCardTerminals   = "/api/v1/card-terminals"
)

func init() {
	for name, routes := range map[string][]mcpRoute{
		"order_draft_create":         {{http.MethodPost, pDrafts}, {http.MethodPost, pDraftQuote}},
		"order_draft_send":           {{http.MethodPost, pDraftSend}, get(pDraft)},
		"order_draft_discard":        {{http.MethodPost, pDraftDiscard}, get(pDraft)},
		"order_deliver":              {{http.MethodPost, pOrderDeliver}, get(pOrder)},
		"order_discount_set":         {{http.MethodPut, pOrderDiscount}, get(pOrder)},
		"order_pay":                  {{http.MethodPost, pOrderPay}, {http.MethodPost, pOrderQuote}, get(pOrder), get(pPaymentMethods)},
		"order_payment_void":         {{http.MethodPost, pPaymentVoid}, get(pOrder)},
		"order_payment_terminal_set": {{http.MethodPost, pPaymentTerminal}, get(pOrder), get(pCardTerminals)},
		"order_refund":               {{http.MethodPost, pOrderRefund}, get(pOrderRefundInfo), get(pOrder)},
		"card_terminals_list":        {get(pCardTerminals)},
	} {
		mcpToolRoutes[name] = routes
	}
	for _, name := range []string{"order_deliver", "order_discount_set", "order_pay", "order_payment_void",
		"order_payment_terminal_set", "order_refund"} {
		mcpListTool[name] = "sales_list"
	}
	for k := range map[string]bool{"delivery_fee": true, "discount_amount": true, "tip": true} {
		moneyFields[k] = true
	}
	register090Orders()
}

// ---- Argumentos ----

type draftModifierArg struct {
	OptionID int64  `json:"option_id" jsonschema:"extra (de modifier_options_list)"`
	Qty      int    `json:"qty,omitempty" jsonschema:"cuántos de ese extra; vacío = 1"`
	Portion  string `json:"portion,omitempty" jsonschema:"porción, si el extra la pide (como en la pantalla)"`
}

type draftLineArg struct {
	ProductID int64              `json:"product_id" jsonschema:"producto (de products_list)"`
	Qty       string             `json:"qty" jsonschema:"cantidad, texto decimal (\"1\")"`
	Modifiers []draftModifierArg `json:"modifiers,omitempty" jsonschema:"extras del renglón"`
	Notes     string             `json:"notes,omitempty" jsonschema:"nota para cocina"`
}

type draftCreateArgs struct {
	writeCommon
	ServiceType      string         `json:"service_type,omitempty" jsonschema:"mostrador, para_llevar o domicilio; vacío = mostrador"`
	PlatformID       *int16         `json:"platform_id,omitempty" jsonschema:"plataforma de reparto (de platform_prices_list); cobra con sus precios"`
	PlatformOrderRef string         `json:"platform_order_ref,omitempty" jsonschema:"folio del pedido en la plataforma"`
	CustomerName     string         `json:"customer_name,omitempty" jsonschema:"nombre del cliente"`
	DeliveryFee      *string        `json:"delivery_fee,omitempty" jsonschema:"costo de envío (domicilio sin plataforma), texto decimal"`
	DiscountAmount   *string        `json:"discount_amount,omitempty" jsonschema:"descuento en pesos; o discount_percent"`
	DiscountPercent  *string        `json:"discount_percent,omitempty" jsonschema:"descuento en porcentaje; o discount_amount"`
	FolioName        string         `json:"folio_name,omitempty" jsonschema:"nombre con el que se canta el pedido; vacío = el que toque"`
	Lines            []draftLineArg `json:"lines" jsonschema:"renglones (1 a 200)"`
}

type draftIDArgs struct {
	writeCommon
	DraftID string `json:"draft_id" jsonschema:"id de la cuenta que devolvió order_draft_create (UUID)"`
}

type orderWriteArgs struct {
	writeCommon
	OrderID int64 `json:"order_id" jsonschema:"id del pedido (de sales_list o de order_draft_send)"`
}

type discountArgs struct {
	writeCommon
	OrderID int64   `json:"order_id" jsonschema:"id del pedido"`
	Amount  *string `json:"amount,omitempty" jsonschema:"descuento en pesos, texto decimal; o percent o clear"`
	Percent *string `json:"percent,omitempty" jsonschema:"descuento en porcentaje, texto decimal; o amount o clear"`
	Clear   bool    `json:"clear,omitempty" jsonschema:"true quita el descuento"`
}

type payArgs struct {
	writeCommon
	OrderID      int64   `json:"order_id" jsonschema:"id del pedido"`
	MethodID     int16   `json:"method_id" jsonschema:"método de pago (de payment_methods_list); un pedido de plataforma, el de su plataforma"`
	AllRemaining bool    `json:"all_remaining,omitempty" jsonschema:"true cobra todo lo que falta; o amount"`
	Amount       *string `json:"amount,omitempty" jsonschema:"monto a cobrar, texto decimal; o all_remaining"`
	Tip          *string `json:"tip,omitempty" jsonschema:"propina, texto decimal"`
	Reference    string  `json:"reference,omitempty" jsonschema:"referencia (folio de la terminal, últimos dígitos)"`
	TerminalID   *int64  `json:"terminal_id,omitempty" jsonschema:"terminal de un cobro con tarjeta (de card_terminals_list)"`
}

type voidArgs struct {
	writeCommon
	OrderID   int64  `json:"order_id" jsonschema:"id del pedido"`
	PaymentID int64  `json:"payment_id" jsonschema:"id del pago (de order_get)"`
	Reason    string `json:"reason" jsonschema:"por qué se anula"`
}

type terminalArgs struct {
	writeCommon
	OrderID        int64  `json:"order_id" jsonschema:"id del pedido"`
	PaymentID      int64  `json:"payment_id" jsonschema:"id del pago con tarjeta (de order_get)"`
	TerminalID     int64  `json:"terminal_id" jsonschema:"terminal correcta (de card_terminals_list)"`
	FromTerminalID *int64 `json:"from_terminal_id,omitempty" jsonschema:"terminal que tiene hoy, como la ve order_get"`
	Reason         string `json:"reason" jsonschema:"por qué se corrige"`
}

type refundArgs struct {
	writeCommon
	OrderID   int64   `json:"order_id" jsonschema:"id del pedido entregado"`
	Reason    string  `json:"reason" jsonschema:"por qué se reembolsa"`
	Amount    *string `json:"amount,omitempty" jsonschema:"cuánto devolver, texto decimal; vacío = todo lo que queda"`
	LineID    *int64  `json:"line_id,omitempty" jsonschema:"renglón contra el que se devuelve (de order_get); vacío = la cuenta"`
	CardFolio string  `json:"card_folio,omitempty" jsonschema:"folio que imprime la terminal al devolver con tarjeta"`
}

// ---- Ayudas ----

// derived es el id que una escritura manda a la ruta, el mismo en cada reintento con la misma clave.
func (s *mcpSession) derived(tool, key string, extra ...string) uuid.UUID {
	parts := append([]string{strconv.FormatInt(s.principal.CompanyID, 10), strconv.FormatInt(s.principal.TokenID, 10),
		key, tool}, extra...)
	return domain.DerivedUUID(parts...)
}

// fetchDraft lee una cuenta por su UUID.
func (s *mcpSession) fetchDraft(ctx context.Context, tool, raw string) (json.RawMessage, error) {
	id, err := parseDraftID(raw)
	if err != nil {
		return nil, err
	}
	status, body, err := s.dispatchDraft(ctx, tool, http.MethodGet, pDraft, id, nil)
	if err != nil {
		return nil, toolErr("Error interno del servidor MCP. Avísale al administrador.")
	}
	if status >= 400 {
		return nil, toolError{msg: toolHint(status, body, ""), status: status}
	}
	return body, nil
}

func requireReason(reason, what string) error {
	if strings.TrimSpace(reason) == "" {
		return toolErr("reason es obligatorio: por qué se %s.", what)
	}
	return nil
}

func (in draftCreateArgs) header() (map[string]any, error) {
	h := map[string]any{}
	if in.ServiceType != "" {
		h["serviceType"] = in.ServiceType
	}
	if in.PlatformID != nil {
		h["platformId"] = *in.PlatformID
	}
	if in.PlatformOrderRef != "" {
		h["platformOrderRef"] = in.PlatformOrderRef
	}
	if in.CustomerName != "" {
		h["customerName"] = in.CustomerName
	}
	if in.DeliveryFee != nil {
		h["deliveryFee"] = *in.DeliveryFee
	}
	switch {
	case in.DiscountAmount != nil && in.DiscountPercent != nil:
		return nil, toolErr("Manda discount_amount o discount_percent, no los dos.")
	case in.DiscountAmount != nil:
		h["discount"] = map[string]any{"amount": *in.DiscountAmount}
	case in.DiscountPercent != nil:
		h["discount"] = map[string]any{"percent": *in.DiscountPercent}
	}
	if len(h) == 0 {
		return nil, nil
	}
	return h, nil
}

// lines arma los renglones; opID da la llave de cada toque (derivada al aplicar, nueva en la vista
// previa, donde no se guarda nada).
func (in draftCreateArgs) lines(opID func(i int) uuid.UUID) []map[string]any {
	out := make([]map[string]any, len(in.Lines))
	for i, l := range in.Lines {
		mods := make([]map[string]any, len(l.Modifiers))
		for j, m := range l.Modifiers {
			qty := m.Qty
			if qty == 0 {
				qty = 1
			}
			mods[j] = map[string]any{"optionId": m.OptionID, "qty": qty, "portion": m.Portion}
		}
		out[i] = map[string]any{"opId": opID(i).String(), "productId": l.ProductID, "qty": l.Qty, "modifiers": mods, "notes": l.Notes}
	}
	return out
}

// markupNotes dice, por renglón y extra, qué precios no son los propios de la plataforma: el caso que
// el 16-nov cobró distinto de lo que publicaba la app.
func markupNotes(quote json.RawMessage) []string {
	var q struct {
		MarkupPct string `json:"markupPct"`
		Lines     []struct {
			ProductName string `json:"productName"`
			UnitPrice   string `json:"unitPrice"`
			BasePrice   string `json:"basePrice"`
			PriceSource string `json:"priceSource"`
			Modifiers   []struct {
				Name        string `json:"name"`
				PriceDelta  string `json:"priceDelta"`
				BasePrice   string `json:"basePrice"`
				PriceSource string `json:"priceSource"`
			} `json:"modifiers"`
		} `json:"lines"`
	}
	if json.Unmarshal(quote, &q) != nil {
		return nil
	}
	var notes []string
	for _, l := range q.Lines {
		if l.PriceSource == domain.PriceFromMarkup {
			notes = append(notes, fmt.Sprintf("%s no tiene precio propio en la plataforma: se cobra $%s (base $%s más %s%%). "+
				"Compáralo con lo que publica la app antes de enviar.", l.ProductName, l.UnitPrice, l.BasePrice, q.MarkupPct))
		}
		for _, m := range l.Modifiers {
			if m.PriceSource == domain.PriceFromMarkup {
				notes = append(notes, fmt.Sprintf("El extra %s de %s se cobra $%s (base $%s más %s%%).", m.Name, l.ProductName,
					m.PriceDelta, m.BasePrice, q.MarkupPct))
			}
		}
	}
	return notes
}

func orderState(tool string) func(*mcpSession, context.Context, int64) any {
	return func(s *mcpSession, ctx context.Context, id int64) any {
		return forAgent(asJSONOrNote(s.soft(ctx, tool, pOrder, ids(id), nil)))
	}
}

// asJSONOrNote deja pasar la nota de soft y vuelve JSON lo que sí se leyó.
func asJSONOrNote(v any) json.RawMessage {
	if b, ok := v.(json.RawMessage); ok {
		return b
	}
	b, _ := json.Marshal(v)
	return b
}

func orderAndPayment(order, payment int64) map[string]int64 {
	return map[string]int64{"id": order, "paymentId": payment}
}

func register090Orders() {
	writeTool(writeSpec[draftCreateArgs]{
		name: "order_draft_create",
		route: func(_ *mcpSession, _ context.Context, in draftCreateArgs) (string, string, map[string]int64, url.Values, any, error) {
			if len(in.Lines) == 0 || len(in.Lines) > domain.MaxDraftLines {
				return "", "", nil, nil, nil, toolErr("La cuenta lleva de 1 a %d renglones.", domain.MaxDraftLines)
			}
			h, err := in.header()
			if err != nil {
				return "", "", nil, nil, nil, err
			}
			body := map[string]any{"header": h, "lines": in.lines(func(int) uuid.UUID { return uuid.UUID{} }), "folioName": in.FolioName}
			return http.MethodPost, pDrafts, nil, nil, body, nil
		},
		strictPreview: func(s *mcpSession, ctx context.Context, in draftCreateArgs) (any, []string, error) {
			h, _ := in.header()
			status, resp, err := s.dispatch(ctx, "order_draft_create", http.MethodPost, pDraftQuote, nil, nil,
				map[string]any{"header": h, "lines": in.lines(func(int) uuid.UUID { return uuid.New() })})
			if err != nil {
				return nil, nil, toolErr("Error interno del servidor MCP. Avísale al administrador.")
			}
			if status >= 400 {
				return nil, nil, toolError{msg: toolHint(status, resp, "products_list"), status: status}
			}
			notes := markupNotes(resp)
			notes = append(notes, "Aplicar abre la cuenta; mándala a cocina con order_draft_send.")
			return json.RawMessage(resp), notes, nil
		},
		run: func(s *mcpSession, ctx context.Context, in draftCreateArgs) (int, []byte, error) {
			const tool = "order_draft_create"
			h, _ := in.header()
			key := in.IdempotencyKey
			body := map[string]any{"id": s.derived(tool, key).String(), "header": h, "folioName": in.FolioName,
				"lines": in.lines(func(i int) uuid.UUID { return s.derived(tool, key, "line", strconv.Itoa(i)) })}
			return s.dispatch(ctx, tool, http.MethodPost, pDrafts, nil, nil, body)
		},
	})

	draftRoute := func(pattern string) func(*mcpSession, context.Context, draftIDArgs) (string, string, map[string]int64, url.Values, any, error) {
		return func(_ *mcpSession, _ context.Context, in draftIDArgs) (string, string, map[string]int64, url.Values, any, error) {
			if _, err := parseDraftID(in.DraftID); err != nil {
				return "", "", nil, nil, nil, err
			}
			return http.MethodPost, pattern, nil, nil, nil, nil
		}
	}
	draftState := func(tool string) func(*mcpSession, context.Context, draftIDArgs, json.RawMessage) any {
		return func(s *mcpSession, ctx context.Context, in draftIDArgs, _ json.RawMessage) any {
			b, err := s.fetchDraft(ctx, tool, in.DraftID)
			if err != nil {
				return map[string]string{"unavailable": err.Error()}
			}
			return forAgent(b)
		}
	}
	writeTool(writeSpec[draftIDArgs]{
		name:  "order_draft_send",
		route: draftRoute(pDraftSend),
		state: draftState("order_draft_send"),
		run: func(s *mcpSession, ctx context.Context, in draftIDArgs) (int, []byte, error) {
			id, _ := parseDraftID(in.DraftID)
			return s.dispatchDraft(ctx, "order_draft_send", http.MethodPost, pDraftSend, id, nil)
		},
	})
	writeTool(writeSpec[draftIDArgs]{
		name:  "order_draft_discard",
		route: draftRoute(pDraftDiscard),
		state: draftState("order_draft_discard"),
		run: func(s *mcpSession, ctx context.Context, in draftIDArgs) (int, []byte, error) {
			const tool = "order_draft_discard"
			id, _ := parseDraftID(in.DraftID)
			// La versión se lee en la misma llamada: descartar la pide para no tirar un renglón que otra
			// tableta acaba de agregar. Si cambió en medio, la ruta responde 409.
			status, b, err := s.dispatchDraft(ctx, tool, http.MethodGet, pDraft, id, nil)
			if err != nil || status >= 400 {
				return status, b, err
			}
			var v struct {
				Version int32 `json:"version"`
			}
			_ = json.Unmarshal(b, &v)
			return s.dispatchDraft(ctx, tool, http.MethodPost, pDraftDiscard, id, map[string]any{"expectedVersion": v.Version})
		},
	})

	deliverState := orderState("order_deliver")
	writeTool(writeSpec[orderWriteArgs]{
		name: "order_deliver",
		route: func(_ *mcpSession, _ context.Context, in orderWriteArgs) (string, string, map[string]int64, url.Values, any, error) {
			if err := positive("order_id", in.OrderID); err != nil {
				return "", "", nil, nil, nil, err
			}
			return http.MethodPost, pOrderDeliver, ids(in.OrderID), nil, nil, nil
		},
		state: func(s *mcpSession, ctx context.Context, in orderWriteArgs, _ json.RawMessage) any {
			return deliverState(s, ctx, in.OrderID)
		},
	})

	discountState := orderState("order_discount_set")
	writeTool(writeSpec[discountArgs]{
		name: "order_discount_set",
		route: func(_ *mcpSession, _ context.Context, in discountArgs) (string, string, map[string]int64, url.Values, any, error) {
			if err := positive("order_id", in.OrderID); err != nil {
				return "", "", nil, nil, nil, err
			}
			n := 0
			for _, set := range []bool{in.Amount != nil, in.Percent != nil, in.Clear} {
				if set {
					n++
				}
			}
			if n != 1 {
				// Sin nada, la ruta borraría el descuento: dinero que cambia sin que nadie lo pidiera.
				return "", "", nil, nil, nil, toolErr("Manda exactamente uno de amount, percent o clear: true.")
			}
			return http.MethodPut, pOrderDiscount, ids(in.OrderID), nil,
				map[string]any{"discountAmount": in.Amount, "discountPercent": in.Percent}, nil
		},
		state: func(s *mcpSession, ctx context.Context, in discountArgs, _ json.RawMessage) any {
			return discountState(s, ctx, in.OrderID)
		},
	})

	payState := orderState("order_pay")
	writeTool(writeSpec[payArgs]{
		name: "order_pay",
		route: func(_ *mcpSession, _ context.Context, in payArgs) (string, string, map[string]int64, url.Values, any, error) {
			if err := positive("order_id", in.OrderID); err != nil {
				return "", "", nil, nil, nil, err
			}
			if in.MethodID <= 0 {
				return "", "", nil, nil, nil, toolErr("method_id es obligatorio. Búscalo con payment_methods_list.")
			}
			if in.AllRemaining == (in.Amount != nil) {
				return "", "", nil, nil, nil, toolErr("Manda all_remaining: true o amount, uno de los dos.")
			}
			return http.MethodPost, pOrderPay, ids(in.OrderID), nil, in.body(uuid.UUID{}), nil
		},
		state: func(s *mcpSession, ctx context.Context, in payArgs, _ json.RawMessage) any {
			return payState(s, ctx, in.OrderID)
		},
		preview: func(s *mcpSession, ctx context.Context, in payArgs) (any, []string) {
			const tool = "order_pay"
			out := map[string]any{}
			var notes []string
			if in.AllRemaining {
				status, resp, err := s.dispatch(ctx, tool, http.MethodPost, pOrderQuote, ids(in.OrderID), nil,
					map[string]any{"allRemaining": true})
				if err == nil && status < 400 {
					out["quote"] = json.RawMessage(resp)
				} else if err == nil {
					notes = append(notes, "No se pudo cotizar «todo lo que falta»: "+toolHint(status, resp, "sales_list"))
					if status == http.StatusConflict {
						notes = append(notes, "Un pedido de plataforma se cobra completo por monto: manda amount con lo que "+
							"falta (before.outstanding).")
					}
				}
			} else {
				notes = append(notes, "Se cobra el monto que mandaste; lo que falta del pedido viene en before.outstanding.")
			}
			if b, _, err := s.fetch(ctx, tool, pPaymentMethods, nil, nil); err == nil {
				if m := findByID(b, int64(in.MethodID)); m != nil {
					out["method"] = m
				} else {
					notes = append(notes, fmt.Sprintf("No existe el método %d; búscalo con payment_methods_list.", in.MethodID))
				}
			}
			return out, notes
		},
		run: func(s *mcpSession, ctx context.Context, in payArgs) (int, []byte, error) {
			const tool = "order_pay"
			return s.dispatch(ctx, tool, http.MethodPost, pOrderPay, ids(in.OrderID), nil,
				in.body(s.derived(tool, in.IdempotencyKey)))
		},
	})

	voidState := orderState("order_payment_void")
	writeTool(writeSpec[voidArgs]{
		name: "order_payment_void",
		route: func(_ *mcpSession, _ context.Context, in voidArgs) (string, string, map[string]int64, url.Values, any, error) {
			if err := positive("order_id", in.OrderID); err != nil {
				return "", "", nil, nil, nil, err
			}
			if err := positive("payment_id", in.PaymentID); err != nil {
				return "", "", nil, nil, nil, err
			}
			if err := requireReason(in.Reason, "anula el pago"); err != nil {
				return "", "", nil, nil, nil, err
			}
			return http.MethodPost, pPaymentVoid, orderAndPayment(in.OrderID, in.PaymentID), nil, map[string]any{"reason": in.Reason}, nil
		},
		state: func(s *mcpSession, ctx context.Context, in voidArgs, _ json.RawMessage) any {
			return voidState(s, ctx, in.OrderID)
		},
	})

	terminalState := orderState("order_payment_terminal_set")
	writeTool(writeSpec[terminalArgs]{
		name: "order_payment_terminal_set",
		route: func(_ *mcpSession, _ context.Context, in terminalArgs) (string, string, map[string]int64, url.Values, any, error) {
			if err := positive("order_id", in.OrderID); err != nil {
				return "", "", nil, nil, nil, err
			}
			if err := positive("payment_id", in.PaymentID); err != nil {
				return "", "", nil, nil, nil, err
			}
			if err := positive("terminal_id", in.TerminalID); err != nil {
				return "", "", nil, nil, nil, err
			}
			if err := requireReason(in.Reason, "corrige la terminal"); err != nil {
				return "", "", nil, nil, nil, err
			}
			return http.MethodPost, pPaymentTerminal, orderAndPayment(in.OrderID, in.PaymentID), nil,
				map[string]any{"fromTerminalId": in.FromTerminalID, "terminalId": in.TerminalID, "reason": in.Reason}, nil
		},
		state: func(s *mcpSession, ctx context.Context, in terminalArgs, _ json.RawMessage) any {
			return terminalState(s, ctx, in.OrderID)
		},
		preview: func(s *mcpSession, ctx context.Context, in terminalArgs) (any, []string) {
			b, _, err := s.fetch(ctx, "order_payment_terminal_set", pCardTerminals, nil, nil)
			if err != nil {
				return nil, []string{"No se pudieron leer las terminales: " + err.Error()}
			}
			if t := findByID(b, in.TerminalID); t != nil {
				return map[string]any{"terminal": t}, nil
			}
			return nil, []string{fmt.Sprintf("No existe la terminal %d; búscala con card_terminals_list.", in.TerminalID)}
		},
	})

	refundState := orderState("order_refund")
	writeTool(writeSpec[refundArgs]{
		name: "order_refund",
		route: func(_ *mcpSession, _ context.Context, in refundArgs) (string, string, map[string]int64, url.Values, any, error) {
			if err := positive("order_id", in.OrderID); err != nil {
				return "", "", nil, nil, nil, err
			}
			if err := requireReason(in.Reason, "reembolsa"); err != nil {
				return "", "", nil, nil, nil, err
			}
			return http.MethodPost, pOrderRefund, ids(in.OrderID), nil,
				map[string]any{"reason": in.Reason, "amount": in.Amount, "lineId": in.LineID, "cardFolio": in.CardFolio}, nil
		},
		state: func(s *mcpSession, ctx context.Context, in refundArgs, _ json.RawMessage) any {
			return refundState(s, ctx, in.OrderID)
		},
		preview: func(s *mcpSession, ctx context.Context, in refundArgs) (any, []string) {
			info := s.soft(ctx, "order_refund", pOrderRefundInfo, ids(in.OrderID), nil)
			var notes []string
			if in.LineID != nil {
				notes = append(notes, "pending es lo que queda de la cuenta entera; contra un renglón, el servidor topa al importe de ese renglón.")
			}
			var ri struct {
				NeedsFolio bool `json:"needsFolio"`
			}
			if b, ok := info.(json.RawMessage); ok && json.Unmarshal(b, &ri) == nil && ri.NeedsFolio && in.CardFolio == "" {
				notes = append(notes, "Se cobró con tarjeta: aplicar pide card_folio, el folio que imprime la terminal al devolver.")
			}
			return map[string]any{"refund_info": info}, notes
		},
	})

	simpleRead("card_terminals_list", pCardTerminals, func(noArgs) (map[string]int64, url.Values, error) { return nil, nil, nil })
}

func (in payArgs) body(client uuid.UUID) map[string]any {
	b := map[string]any{"methodId": in.MethodID, "allRemaining": in.AllRemaining, "terminalId": in.TerminalID}
	if in.Amount != nil {
		b["amount"] = *in.Amount
	}
	if in.Tip != nil {
		b["tip"] = *in.Tip
	}
	if in.Reference != "" {
		b["reference"] = in.Reference
	}
	if client != (uuid.UUID{}) {
		b["clientUuid"] = client.String()
	}
	return b
}
