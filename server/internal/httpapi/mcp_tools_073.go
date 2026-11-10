package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
)

// Las herramientas de la spec 073: compras con su mercancía, conteos, cancelar pedidos, abrir y
// cerrar caja. Igual que las de la 072, cada una manda el MISMO cuerpo que la pantalla a la MISMA
// ruta; lo propio de esta capa es la vista previa, los ids explícitos y las cifras dictadas.

//nolint:gosec // G101: patrones de rutas; gosec confunde palabras con credenciales.
const (
	pArticles         = "/api/v1/articles"
	pArticleSuggest   = "/api/v1/articles/suggest"
	pUnits            = "/api/v1/units"
	pPresentations    = "/api/v1/stock/presentations"
	pPendingReceipts  = "/api/v1/stock/pending-receipts"
	pPurchasePreview  = "/api/v1/expenses/purchase-preview"
	pExpenseReceive   = "/api/v1/expenses/{id}/receive"
	pBusinessSettings = "/api/v1/business-settings"
	pCountCurrent     = "/api/v1/stock/counts/current"
	pCounts           = "/api/v1/stock/counts"
	pCountLine        = "/api/v1/stock/counts/{id}/lines/{ingredientId}"
	pCountProductLine = "/api/v1/stock/counts/{id}/product-lines/{productId}"
	pCountPreview     = "/api/v1/stock/counts/{id}/preview"
	pCountClose       = "/api/v1/stock/counts/{id}/close"
	pCountDiscard     = "/api/v1/stock/counts/{id}/discard"
	pOrderCancel      = "/api/v1/orders/{id}/cancel"
	pDenominations    = "/api/v1/cash/denominations"
	pCashAttendance   = "/api/v1/cash-sessions/attendance"
	pCashOpen         = "/api/v1/cash-sessions/"
	pCashClose        = "/api/v1/cash-sessions/close"
	pPendingExpenses  = "/api/v1/cash-sessions/pending-expenses"
)

// maxCountBatch acota el lote de conteo: un conteo grande se dicta en varias llamadas, y una sola no
// puede despachar cientos de peticiones contra el límite de una.
const maxCountBatch = 50

// maxPurchaseLines es el mismo techo que la captura de gastos (maxExpenseItems en app).
const maxPurchaseLines = 300

func init() {
	for name, routes := range map[string][]mcpRoute{
		"articles_search":       {get(pArticles)},
		"units_list":            {get(pUnits)},
		"presentations_list":    {get(pPresentations)},
		"pending_receipts_list": {get(pPendingReceipts)},
		"purchase_create": {{http.MethodPost, pExpenses}, {http.MethodPost, pPurchasePreview}, get(pArticleSuggest),
			get(pPresentations), get(pBusinessSettings), get(pExpense), get(pCashCurrent)},
		"purchase_receive":        {{http.MethodPost, pExpenseReceive}, get(pExpense)},
		"stock_count_current":     {get(pCountCurrent)},
		"stock_count_open":        {{http.MethodPost, pCounts}, get(pCountCurrent)},
		"stock_count_capture":     {{http.MethodPut, pCountLine}, {http.MethodPut, pCountProductLine}, get(pCountCurrent)},
		"stock_count_close":       {{http.MethodPost, pCountClose}, get(pCountPreview)},
		"stock_count_discard":     {{http.MethodPost, pCountDiscard}, get(pCountCurrent)},
		"order_cancel":            {{http.MethodPost, pOrderCancel}, get(pOrder)},
		"cash_denominations_list": {get(pDenominations)},
		"cash_session_attendance": {get(pCashAttendance)},
		"cash_session_open":       {{http.MethodPost, pCashOpen}, get(pCashAttendance), get(pCashCurrent), get(pPendingExpenses)},
		"cash_session_close":      {{http.MethodPost, pCashClose}, get(pCashCurrent), get(pCashAttendance)},
	} {
		mcpToolRoutes[name] = routes
	}
	for k, v := range map[string]string{
		"purchase_receive": "pending_receipts_list", "stock_count_capture": "stock_count_current",
		"stock_count_close": "stock_count_current", "stock_count_discard": "stock_count_current",
		"order_cancel": "sales_list", "cash_session_open": "cash_registers_list",
		"cash_session_close": "cash_registers_list", "cash_session_attendance": "cash_registers_list",
	} {
		mcpListTool[k] = v
	}
	for k := range map[string]bool{"counted_cash": true, "opening_cash": true, "float_left": true} {
		moneyFields[k] = true
	}
	quantityFields["counted_qty"] = true
	moneyMapFields["declared"] = true
	moneyMapFields["terminal_counts"] = true
	// Aquí y no en el init de mcp_tools.go: el esquema de cada herramienta se arma al registrarla, y
	// tiene que ver ya los campos de dinero de arriba.
	register073()
}

// ---- Argumentos ----

type searchArgs struct {
	Query string `json:"query" jsonschema:"parte del nombre del insumo o producto"`
}

type purchaseLineArg struct {
	Name           string `json:"name" jsonschema:"lo que dice el ticket del renglón"`
	IngredientID   *int64 `json:"ingredient_id,omitempty" jsonschema:"insumo (de articles_search o de los candidatos de la vista previa); o product_id"`
	ProductID      *int64 `json:"product_id,omitempty" jsonschema:"producto de reventa; o ingredient_id"`
	PresentationID *int64 `json:"presentation_id,omitempty" jsonschema:"empaque del insumo (de presentations_list); o unit_id"`
	UnitID         *int16 `json:"unit_id,omitempty" jsonschema:"unidad en que se compró (de units_list); o presentation_id"`
	Quantity       string `json:"quantity" jsonschema:"cuántos empaques o cuánto en la unidad, texto decimal"`
	Amount         string `json:"amount" jsonschema:"importe del renglón, texto decimal"`
	IsGift         bool   `json:"is_gift,omitempty" jsonschema:"true si vino de regalo (importe 0)"`
}

type purchaseCreateArgs struct {
	writeCommon
	CategoryID   int64             `json:"category_id" jsonschema:"id de la categoría (de expense_categories_list)"`
	Amount       string            `json:"amount" jsonschema:"total de la compra en pesos, texto decimal"`
	SupplierID   *int64            `json:"supplier_id,omitempty" jsonschema:"id del proveedor (de suppliers_list)"`
	DocumentDate string            `json:"document_date,omitempty" jsonschema:"fecha del ticket, AAAA-MM-DD"`
	DocFolio     string            `json:"doc_folio,omitempty" jsonschema:"folio del ticket o factura"`
	Description  string            `json:"description,omitempty" jsonschema:"qué se compró, en una línea"`
	ReceivedOn   string            `json:"received_on,omitempty" jsonschema:"día en que llegó, AAAA-MM-DD; vacío = hoy"`
	Pending      bool              `json:"pending,omitempty" jsonschema:"true: todavía no llega; queda por recibir"`
	Payments     []paymentArg      `json:"payments,omitempty" jsonschema:"pagos (hasta 10); sin pagos queda pendiente de pago"`
	Lines        []purchaseLineArg `json:"lines" jsonschema:"renglones de mercancía (1 a 300)"`
}

type receivedArg struct {
	ItemID int64  `json:"item_id" jsonschema:"renglón de la compra (de pending_receipts_list)"`
	Qty    string `json:"qty" jsonschema:"cuánto llegó, en la unidad del renglón; 0 = no llegó"`
}

type purchaseReceiveArgs struct {
	writeCommon
	ExpenseID  int64         `json:"expense_id" jsonschema:"la compra (de pending_receipts_list)"`
	Received   []receivedArg `json:"received,omitempty" jsonschema:"lo que llegó por renglón; ausente = llegó todo"`
	ReceivedOn string        `json:"received_on,omitempty" jsonschema:"día en que llegó, AAAA-MM-DD; vacío = hoy"`
}

type countOpenArgs struct {
	writeCommon
}

type countLineArg struct {
	IngredientID *int64 `json:"ingredient_id,omitempty" jsonschema:"insumo; o product_id"`
	ProductID    *int64 `json:"product_id,omitempty" jsonschema:"producto con existencia propia (piezas enteras); o ingredient_id"`
	CountedQty   string `json:"counted_qty" jsonschema:"lo que se contó, en la unidad base del artículo, como lo dictaron"`
	Confirmed    bool   `json:"confirmed,omitempty" jsonschema:"true si se confirmó una cifra fuera de lo de costumbre"`
}

type countCaptureArgs struct {
	writeCommon
	CountID int64          `json:"count_id" jsonschema:"el conteo abierto (de stock_count_current)"`
	Lines   []countLineArg `json:"lines" jsonschema:"de 1 a 50 renglones dictados"`
}

type countIDArgs struct {
	writeCommon
	CountID int64 `json:"count_id" jsonschema:"el conteo abierto (de stock_count_current)"`
}

type orderCancelArgs struct {
	writeCommon
	OrderID   int64  `json:"order_id" jsonschema:"id del pedido (de sales_list)"`
	Reason    string `json:"reason" jsonschema:"por qué se cancela"`
	Refund    bool   `json:"refund,omitempty" jsonschema:"obligatorio true si el pedido tiene cobros: el dinero se devuelve"`
	CardFolio string `json:"card_folio,omitempty" jsonschema:"folio de la terminal si se devuelve algo cobrado con tarjeta"`
}

type piecesArg struct {
	DenominationID int64 `json:"denomination_id" jsonschema:"billete o moneda (de cash_denominations_list)"`
	Pieces         int   `json:"pieces" jsonschema:"cuántas piezas"`
}

func piecesBody(in []piecesArg) []map[string]any {
	out := make([]map[string]any, 0, len(in))
	for _, p := range in {
		out = append(out, map[string]any{"denominationId": p.DenominationID, "pieces": p.Pieces})
	}
	return out
}

type cashOpenArgs struct {
	writeCommon
	RegisterID        int64       `json:"register_id" jsonschema:"id de la caja (de cash_registers_list)"`
	Counts            []piecesArg `json:"counts,omitempty" jsonschema:"el fondo contado por billetes y monedas; o opening_cash"`
	OpeningCash       string      `json:"opening_cash,omitempty" jsonschema:"el fondo contado como total, texto decimal; exige manual_reason"`
	ManualReason      string      `json:"manual_reason,omitempty" jsonschema:"por qué se captura el total y no por piezas"`
	OpeningReason     string      `json:"opening_reason,omitempty" jsonschema:"si no coincide con el cierre anterior: last_count_wrong, float_changed, unrecorded_withdrawal u other"`
	OpeningReasonNote string      `json:"opening_reason_note,omitempty" jsonschema:"obligatorio con other"`
}

type cashCloseArgs struct {
	writeCommon
	RegisterID            int64             `json:"register_id" jsonschema:"id de la caja (de cash_registers_list)"`
	Counts                []piecesArg       `json:"counts,omitempty" jsonschema:"el cajón contado por billetes y monedas; o counted_cash"`
	CountedCash           string            `json:"counted_cash,omitempty" jsonschema:"el cajón contado como total, texto decimal; exige manual_reason"`
	ManualReason          string            `json:"manual_reason,omitempty" jsonschema:"por qué se captura el total y no por piezas"`
	Declared              map[string]string `json:"declared,omitempty" jsonschema:"lo contado de cada método que no es del cajón: {id del método: monto}"`
	TerminalCounts        map[string]string `json:"terminal_counts,omitempty" jsonschema:"el total que marcó cada terminal: {id de la terminal: monto}"`
	TipsDecision          string            `json:"tips_decision,omitempty" jsonschema:"si hay propina pendiente: entregar o quedan_en_caja"`
	FloatLeft             string            `json:"float_left,omitempty" jsonschema:"el fondo que se queda para el siguiente turno, texto decimal; vacío = el fondo fijo"`
	WithdrawalUserID      *int64            `json:"withdrawal_user_id,omitempty" jsonschema:"quién se lleva el retiro; vacío = quien cierra"`
	Notes                 string            `json:"notes,omitempty" jsonschema:"notas del cierre (p. ej. quién contó, si no es quien cierra)"`
	AuthorizedCloseReason string            `json:"authorized_close_reason,omitempty" jsonschema:"cerrar sin la salida de quien abrió, con motivo (pide el permiso)"`
}

// ---- Lectura del turno para cerrar ----

// closeView es lo que el cierre lee del turno abierto. Viene de la MISMA ruta que la pantalla, ya
// filtrada por el arqueo ciego: aquí no hay un esperado que leer aunque se quisiera.
type closeView struct {
	Totals []struct {
		MethodID    int    `json:"methodId"`
		Name        string `json:"name"`
		AutoDeclare bool   `json:"autoDeclare"`
	} `json:"totals"`
	Drawer *struct {
		MethodIDs     []int `json:"methodIds"`
		RequiresCount bool  `json:"requiresCount"`
	} `json:"drawer"`
	TerminalsToCount []struct {
		TerminalID int64  `json:"terminalId"`
		Name       string `json:"name"`
	} `json:"terminalsToCount"`
	TipsPending *struct {
		Total decimal.Decimal `json:"total"`
	} `json:"tipsPending"`
}

func (v closeView) requirements() domain.CloseRequirements {
	inDrawer := map[int]bool{}
	var req domain.CloseRequirements
	if v.Drawer != nil {
		req.DrawerNeedsCount = v.Drawer.RequiresCount
		for _, id := range v.Drawer.MethodIDs {
			inDrawer[id] = true
		}
	}
	for _, t := range v.Totals {
		req.Methods = append(req.Methods, domain.CloseMethod{ID: t.MethodID, Name: t.Name,
			InDrawer: inDrawer[t.MethodID], AutoDeclare: t.AutoDeclare})
	}
	for _, t := range v.TerminalsToCount {
		req.Terminals = append(req.Terminals, domain.CloseTerminal{ID: t.TerminalID, Name: t.Name})
	}
	return req
}

func (s *mcpSession) openShift(ctx context.Context, tool string, register int64) (*closeView, error) {
	b, _, err := s.fetch(ctx, tool, pCashCurrent, nil, url.Values{"registerId": {strconv.FormatInt(register, 10)}})
	if err != nil {
		return nil, err
	}
	if t := strings.TrimSpace(string(b)); t == "" || t == "null" {
		return nil, nil
	}
	var v closeView
	if err := json.Unmarshal(b, &v); err != nil {
		return nil, toolErr("No se pudo leer el turno de la caja. Vuelve a intentar.")
	}
	return &v, nil
}

// today es el día del negocio, en su zona: la de los ajustes de la empresa.
func (s *mcpSession) today(ctx context.Context, tool string) string {
	tz := domain.DefaultTimezone
	if b, _, err := s.fetch(ctx, tool, pBusinessSettings, nil, nil); err == nil {
		var st struct {
			Timezone string `json:"timezone"`
		}
		if json.Unmarshal(b, &st) == nil && st.Timezone != "" {
			tz = st.Timezone
		}
	}
	return domain.BusinessDate(time.Now(), domain.LoadBusinessLocation(tz)).Format(time.DateOnly)
}

// ---- Compras ----

func purchaseLineComplete(l purchaseLineArg) bool {
	article := (l.IngredientID != nil) != (l.ProductID != nil)
	unit := (l.PresentationID != nil) != (l.UnitID != nil)
	return article && unit
}

func purchaseItems(lines []purchaseLineArg) []map[string]any {
	items := make([]map[string]any, 0, len(lines))
	for _, l := range lines {
		it := map[string]any{"description": strings.TrimSpace(l.Name), "rawName": strings.TrimSpace(l.Name),
			"quantity": l.Quantity, "amount": l.Amount, "isGift": l.IsGift,
			"unitId": l.UnitID, "presentationId": l.PresentationID}
		switch {
		case !purchaseLineComplete(l):
			// Incompleto solo puede llegar a la vista previa: como renglón sin almacén, que es lo que
			// la captura haría con él. Aplicar lo rechaza antes.
			it["itemType"] = ""
			it["unitId"], it["presentationId"] = nil, nil
		case l.IngredientID != nil:
			it["itemType"], it["ingredientId"] = "ingrediente", *l.IngredientID
		default:
			it["itemType"], it["productId"] = "producto", *l.ProductID
		}
		items = append(items, it)
	}
	return items
}

func purchaseBody(s *mcpSession, ctx context.Context, in purchaseCreateArgs) (map[string]any, error) {
	if err := positive("category_id", in.CategoryID); err != nil {
		return nil, err
	}
	if len(in.Lines) == 0 || len(in.Lines) > maxPurchaseLines {
		return nil, toolErr("Una compra lleva de 1 a %d renglones de mercancía; un gasto sin mercancía va con expense_create.", maxPurchaseLines)
	}
	if len(in.Payments) > maxExpensePayments {
		return nil, toolErr("Una compra lleva a lo más %d pagos.", maxExpensePayments)
	}
	for i, l := range in.Lines {
		if strings.TrimSpace(l.Name) == "" {
			return nil, toolErr("El renglón %d necesita name: lo que dice el ticket.", i+1)
		}
	}
	total, err := decimal.NewFromString(in.Amount)
	if err != nil {
		return nil, toolErr("amount es un importe en pesos, como \"845.50\".")
	}
	status := "pendiente"
	paid := decimal.Zero
	payments := make([]map[string]any, 0, len(in.Payments))
	for _, p := range in.Payments {
		a, err := decimal.NewFromString(p.Amount)
		if err != nil {
			return nil, toolErr("El importe de cada pago es texto decimal, como \"845.50\".")
		}
		paid = paid.Add(a)
		payments = append(payments, p.body())
	}
	if len(payments) > 0 && paid.GreaterThanOrEqual(total) {
		status = "pagada"
	}
	body := map[string]any{"expenseDate": in.DocumentDate, "categoryId": in.CategoryID, "supplierId": in.SupplierID,
		"amount": in.Amount, "description": in.Description, "status": status, "payments": payments,
		"docFolio": in.DocFolio, "items": purchaseItems(in.Lines)}
	if !in.Pending {
		day := in.ReceivedOn
		if day == "" {
			day = s.today(ctx, "purchase_create")
		}
		body["receivedAt"] = day
	}
	return body, nil
}

func registerPurchases() {
	simpleRead("articles_search", pArticles, func(in searchArgs) (map[string]int64, url.Values, error) {
		if strings.TrimSpace(in.Query) == "" {
			return nil, nil, toolErr("query es obligatorio: parte del nombre.")
		}
		return nil, url.Values{"q": {in.Query}}, nil
	})
	simpleRead("units_list", pUnits, func(noArgs) (map[string]int64, url.Values, error) { return nil, nil, nil })
	simpleRead("presentations_list", pPresentations, func(noArgs) (map[string]int64, url.Values, error) { return nil, nil, nil })
	simpleRead("pending_receipts_list", pPendingReceipts, func(noArgs) (map[string]int64, url.Values, error) { return nil, nil, nil })

	writeTool(writeSpec[purchaseCreateArgs]{
		name: "purchase_create",
		route: func(s *mcpSession, ctx context.Context, in purchaseCreateArgs) (string, string, map[string]int64, url.Values, any, error) {
			applying := in.DryRun != nil && !*in.DryRun
			if applying {
				// El agente nunca elige insumo ni empaque: aplicar exige que la persona los haya
				// confirmado en la vista previa (FR-005).
				for i, l := range in.Lines {
					if !purchaseLineComplete(l) {
						return "", "", nil, nil, nil, toolErr("El renglón %d («%s») necesita exactamente uno de ingredient_id o "+
							"product_id y uno de presentation_id o unit_id. Pide la vista previa: propone candidatos y empaques.",
							i+1, strings.TrimSpace(l.Name))
					}
				}
			}
			body, err := purchaseBody(s, ctx, in)
			if err != nil {
				return "", "", nil, nil, nil, err
			}
			return http.MethodPost, pExpenses, nil, nil, body, nil
		},
		state: func(s *mcpSession, ctx context.Context, _ purchaseCreateArgs, result json.RawMessage) any {
			var r struct {
				ID int64 `json:"id"`
			}
			if result == nil || json.Unmarshal(result, &r) != nil || r.ID == 0 {
				return nil
			}
			return s.soft(ctx, "purchase_create", pExpense, ids(r.ID), nil)
		},
		preview: func(s *mcpSession, ctx context.Context, in purchaseCreateArgs) (any, []string) {
			const tool = "purchase_create"
			var notes []string
			body, err := purchaseBody(s, ctx, in)
			if err != nil {
				return nil, nil
			}
			var presentations json.RawMessage
			presOK := false
			lines := make([]map[string]any, 0, len(in.Lines))
			for i, l := range in.Lines {
				row := map[string]any{"line": i + 1, "name": l.Name, "complete": purchaseLineComplete(l)}
				if l.IngredientID == nil && l.ProductID == nil {
					q := url.Values{"rawName": {l.Name}}
					if in.SupplierID != nil {
						q.Set("supplierId", strconv.FormatInt(*in.SupplierID, 10))
					}
					row["candidates"] = s.soft(ctx, tool, pArticleSuggest, nil, q)
				}
				if l.IngredientID != nil && l.PresentationID == nil && l.UnitID == nil {
					if !presOK {
						presentations, _, err = s.fetch(ctx, tool, pPresentations, nil, nil)
						presOK = err == nil
						if err != nil {
							notes = append(notes, "No se pudieron leer los empaques: "+err.Error())
						}
					}
					if presOK {
						row["presentations"] = presentationsOf(presentations, *l.IngredientID)
					}
				}
				lines = append(lines, row)
			}
			out := map[string]any{"lines": lines}
			status, resp, err := s.dispatch(ctx, tool, http.MethodPost, pPurchasePreview, nil, nil, body)
			switch {
			case err != nil:
				notes = append(notes, "No se pudo calcular el costo. Vuelve a intentar.")
			case status >= 400:
				notes = append(notes, "Al aplicarla así se rechazaría: "+toolHint(status, resp, ""))
			default:
				out["cost"] = json.RawMessage(resp)
			}
			for i, l := range in.Lines {
				if !purchaseLineComplete(l) {
					notes = append(notes, fmt.Sprintf("El renglón %d todavía no tiene insumo y empaque (o unidad): confírmalos "+
						"con quien compró antes de aplicar.", i+1))
				}
			}
			notes = append(notes, "El costo nuevo es el de este momento; al aplicar se vuelve a calcular.")
			return out, notes
		},
	})

	writeTool(writeSpec[purchaseReceiveArgs]{
		name: "purchase_receive",
		route: func(_ *mcpSession, _ context.Context, in purchaseReceiveArgs) (string, string, map[string]int64, url.Values, any, error) {
			if err := positive("expense_id", in.ExpenseID); err != nil {
				return "", "", nil, nil, nil, err
			}
			body := map[string]any{"receivedAt": in.ReceivedOn}
			if len(in.Received) > 0 {
				got := map[string]string{}
				for _, r := range in.Received {
					if r.ItemID <= 0 || strings.TrimSpace(r.Qty) == "" {
						return "", "", nil, nil, nil, toolErr("Cada renglón de received lleva item_id y qty.")
					}
					got[strconv.FormatInt(r.ItemID, 10)] = r.Qty
				}
				body["received"] = got
			}
			return http.MethodPost, pExpenseReceive, ids(in.ExpenseID), nil, body, nil
		},
		state: func(s *mcpSession, ctx context.Context, in purchaseReceiveArgs, _ json.RawMessage) any {
			return s.soft(ctx, "purchase_receive", pExpense, ids(in.ExpenseID), nil)
		},
	})
}

// presentationsOf deja los empaques activos de un insumo.
func presentationsOf(body json.RawMessage, ingredient int64) []map[string]any {
	var all []map[string]any
	if json.Unmarshal(body, &all) != nil {
		var env struct {
			Items []map[string]any `json:"items"`
		}
		_ = json.Unmarshal(body, &env)
		all = env.Items
	}
	out := []map[string]any{}
	for _, p := range all {
		if id, ok := p["ingredientId"].(float64); ok && int64(id) == ingredient {
			if active, ok := p["active"].(bool); ok && !active {
				continue
			}
			out = append(out, map[string]any{"id": p["id"], "name": p["name"], "qtyInBase": p["qtyInBase"],
				"baseUnitCode": p["baseUnitCode"]})
		}
	}
	return out
}

// ---- Conteos ----

func registerCounts() {
	simpleRead("stock_count_current", pCountCurrent, func(noArgs) (map[string]int64, url.Values, error) { return nil, nil, nil })

	current := func(tool string) func(s *mcpSession, ctx context.Context, _ any) any {
		return func(s *mcpSession, ctx context.Context, _ any) any {
			return s.soft(ctx, tool, pCountCurrent, nil, nil)
		}
	}
	openState := current("stock_count_open")
	writeTool(writeSpec[countOpenArgs]{
		name: "stock_count_open",
		route: func(_ *mcpSession, _ context.Context, _ countOpenArgs) (string, string, map[string]int64, url.Values, any, error) {
			return http.MethodPost, pCounts, nil, nil, map[string]any{}, nil
		},
		state: func(s *mcpSession, ctx context.Context, in countOpenArgs, _ json.RawMessage) any {
			return openState(s, ctx, in)
		},
	})

	captureState := current("stock_count_capture")
	writeTool(writeSpec[countCaptureArgs]{
		name: "stock_count_capture",
		route: func(_ *mcpSession, _ context.Context, in countCaptureArgs) (string, string, map[string]int64, url.Values, any, error) {
			if err := positive("count_id", in.CountID); err != nil {
				return "", "", nil, nil, nil, err
			}
			if len(in.Lines) == 0 || len(in.Lines) > maxCountBatch {
				return "", "", nil, nil, nil, toolErr("Manda de 1 a %d renglones por llamada; un conteo grande va en varias.", maxCountBatch)
			}
			for i, l := range in.Lines {
				if err := oneOf(l.IngredientID, l.ProductID, "ingredient_id o product_id"); err != nil {
					return "", "", nil, nil, nil, toolErr("Renglón %d: %s", i+1, err.Error())
				}
				if strings.TrimSpace(l.CountedQty) == "" {
					return "", "", nil, nil, nil, toolErr("Renglón %d: falta counted_qty, lo que se contó.", i+1)
				}
			}
			return http.MethodPut, pCountLine, nil, nil, in.Lines, nil
		},
		state: func(s *mcpSession, ctx context.Context, in countCaptureArgs, result json.RawMessage) any {
			if result != nil {
				return nil // lo capturado ya va en el resultado; releer el conteo no agrega nada
			}
			return captureState(s, ctx, in)
		},
		run: func(s *mcpSession, ctx context.Context, in countCaptureArgs) (int, []byte, error) {
			captured := make([]map[string]any, 0, len(in.Lines))
			for i, l := range in.Lines {
				pattern, params, what := pCountLine, map[string]int64{"id": in.CountID}, "insumo"
				if l.IngredientID != nil {
					params["ingredientId"] = *l.IngredientID
				} else {
					pattern, what = pCountProductLine, "producto"
					params["productId"] = *l.ProductID
				}
				// confirmable: la cifra fuera de lo de costumbre vuelve como «confirma», sin decir contra
				// qué se comparó (el conteo sigue a ciegas).
				status, resp, err := s.dispatch(ctx, "stock_count_capture", http.MethodPut, pattern, params, nil,
					map[string]any{"countedQty": l.CountedQty, "confirmable": true, "confirmed": l.Confirmed})
				if err != nil || status >= 500 {
					return max(status, 500), resp, err
				}
				id := params["ingredientId"] + params["productId"]
				if status >= 400 {
					var env errorEnvelope
					_ = json.Unmarshal(resp, &env)
					env.Error.Message = fmt.Sprintf("Renglón %d (%s %d): %s. Quedaron capturados los %d anteriores",
						i+1, what, id, strings.TrimRight(env.Error.Message, "."), len(captured))
					b, _ := json.Marshal(env)
					return status, b, nil
				}
				captured = append(captured, map[string]any{"line": i + 1, "item": what, "id": id, "counted_qty": l.CountedQty})
			}
			b, _ := json.Marshal(map[string]any{"captured": captured})
			return http.StatusOK, b, nil
		},
	})

	writeTool(writeSpec[countIDArgs]{
		name: "stock_count_close",
		route: func(_ *mcpSession, _ context.Context, in countIDArgs) (string, string, map[string]int64, url.Values, any, error) {
			if err := positive("count_id", in.CountID); err != nil {
				return "", "", nil, nil, nil, err
			}
			return http.MethodPost, pCountClose, ids(in.CountID), nil, map[string]any{}, nil
		},
		// Las diferencias contra lo que el sistema tiene se ven aquí y solo aquí, antes de cerrar: es la
		// misma hoja que la pantalla muestra antes del botón (y queda marcada como vista).
		preview: func(s *mcpSession, ctx context.Context, in countIDArgs) (any, []string) {
			return s.soft(ctx, "stock_count_close", pCountPreview, ids(in.CountID), nil),
				[]string{"Cerrar reemplaza lo que el sistema tiene por lo contado en cada artículo de adjusted; lo no contado no se toca."}
		},
	})

	discardState := current("stock_count_discard")
	writeTool(writeSpec[countIDArgs]{
		name: "stock_count_discard",
		route: func(_ *mcpSession, _ context.Context, in countIDArgs) (string, string, map[string]int64, url.Values, any, error) {
			if err := positive("count_id", in.CountID); err != nil {
				return "", "", nil, nil, nil, err
			}
			return http.MethodPost, pCountDiscard, ids(in.CountID), nil, map[string]any{}, nil
		},
		state: func(s *mcpSession, ctx context.Context, in countIDArgs, _ json.RawMessage) any {
			return discardState(s, ctx, in)
		},
	})
}

// ---- Pedidos ----

func registerOrders() {
	orderState := func(s *mcpSession, ctx context.Context, id int64) any {
		b, _, err := s.fetch(ctx, "order_cancel", pOrder, ids(id), nil)
		if err != nil {
			return map[string]string{"unavailable": err.Error()}
		}
		var o struct {
			ID     int64  `json:"id"`
			Status string `json:"status"`
			Total  any    `json:"total"`
		}
		_ = json.Unmarshal(b, &o)
		return o
	}
	writeTool(writeSpec[orderCancelArgs]{
		name: "order_cancel",
		route: func(_ *mcpSession, _ context.Context, in orderCancelArgs) (string, string, map[string]int64, url.Values, any, error) {
			if err := positive("order_id", in.OrderID); err != nil {
				return "", "", nil, nil, nil, err
			}
			if strings.TrimSpace(in.Reason) == "" {
				return "", "", nil, nil, nil, toolErr("reason es obligatorio: por qué se cancela el pedido.")
			}
			return http.MethodPost, pOrderCancel, ids(in.OrderID), nil,
				map[string]any{"reason": in.Reason, "devolver": in.Refund, "cardFolio": in.CardFolio}, nil
		},
		state: func(s *mcpSession, ctx context.Context, in orderCancelArgs, _ json.RawMessage) any {
			return orderState(s, ctx, in.OrderID)
		},
		preview: func(s *mcpSession, ctx context.Context, in orderCancelArgs) (any, []string) {
			b, _, err := s.fetch(ctx, "order_cancel", pOrder, ids(in.OrderID), nil)
			if err != nil {
				return nil, []string{err.Error()}
			}
			var o struct {
				Payments []struct {
					MethodName string          `json:"methodName"`
					MethodKind string          `json:"methodKind"`
					Amount     decimal.Decimal `json:"amount"`
					Voided     bool            `json:"voided"`
				} `json:"payments"`
			}
			_ = json.Unmarshal(b, &o)
			var pays []domain.CancelPayment
			for _, p := range o.Payments {
				if !p.Voided {
					pays = append(pays, domain.CancelPayment{Method: p.MethodName, Amount: p.Amount,
						IsCard: p.MethodKind == domain.PaymentKindCard})
				}
			}
			sum := domain.CancelSummary(pays)
			var notes []string
			if sum.NeedsRefund {
				notes = append(notes, "El pedido tiene cobros: cancelarlo devuelve ese dinero y sale del corte. Hace falta refund: true.")
			}
			if sum.NeedsCardFolio {
				notes = append(notes, "Hubo cobro con tarjeta: hace falta card_folio, el folio que imprime la terminal al devolver.")
			}
			return map[string]any{"refund": sum}, notes
		},
	})
}

// ---- Caja ----

func registerCash() {
	simpleRead("cash_denominations_list", pDenominations, func(noArgs) (map[string]int64, url.Values, error) { return nil, nil, nil })
	simpleRead("cash_session_attendance", pCashAttendance, func(in registerArg) (map[string]int64, url.Values, error) {
		if err := positive("register_id", in.RegisterID); err != nil {
			return nil, nil, err
		}
		return nil, url.Values{"registerId": {strconv.FormatInt(in.RegisterID, 10)}}, nil
	})
	reg := func(id int64) url.Values { return url.Values{"registerId": {strconv.FormatInt(id, 10)}} }

	writeTool(writeSpec[cashOpenArgs]{
		name: "cash_session_open",
		route: func(_ *mcpSession, _ context.Context, in cashOpenArgs) (string, string, map[string]int64, url.Values, any, error) {
			if err := positive("register_id", in.RegisterID); err != nil {
				return "", "", nil, nil, nil, err
			}
			if (len(in.Counts) > 0) == (in.OpeningCash != "") {
				return "", "", nil, nil, nil, toolErr("Manda el fondo contado: counts (billetes y monedas) u opening_cash con manual_reason, no los dos.")
			}
			body := map[string]any{"registerId": in.RegisterID, "openingReason": in.OpeningReason,
				"openingReasonNote": in.OpeningReasonNote}
			if len(in.Counts) > 0 {
				body["counts"] = piecesBody(in.Counts)
			} else {
				body["openingCash"], body["manualReason"] = in.OpeningCash, in.ManualReason
			}
			return http.MethodPost, pCashOpen, nil, nil, body, nil
		},
		state: func(s *mcpSession, ctx context.Context, in cashOpenArgs, _ json.RawMessage) any {
			return s.soft(ctx, "cash_session_open", pCashCurrent, nil, reg(in.RegisterID))
		},
		preview: func(s *mcpSession, ctx context.Context, in cashOpenArgs) (any, []string) {
			return map[string]any{
				"attendance":       s.soft(ctx, "cash_session_open", pCashAttendance, nil, reg(in.RegisterID)),
				"pending_expenses": s.soft(ctx, "cash_session_open", pPendingExpenses, nil, reg(in.RegisterID)),
			}, []string{"Quien abre es la persona de este token: la asistencia se revisa sobre ella."}
		},
	})

	writeTool(writeSpec[cashCloseArgs]{
		name: "cash_session_close",
		route: func(s *mcpSession, ctx context.Context, in cashCloseArgs) (string, string, map[string]int64, url.Values, any, error) {
			if err := positive("register_id", in.RegisterID); err != nil {
				return "", "", nil, nil, nil, err
			}
			if len(in.Counts) > 0 && in.CountedCash != "" {
				return "", "", nil, nil, nil, toolErr("Manda el cajón contado como counts o como counted_cash, no los dos.")
			}
			body := map[string]any{"registerId": in.RegisterID, "notes": in.Notes, "tipsDecision": in.TipsDecision,
				"withdrawalUserId": in.WithdrawalUserID}
			if in.Declared != nil {
				body["declared"] = in.Declared
			}
			if in.TerminalCounts != nil {
				body["terminalCounts"] = in.TerminalCounts
			}
			if len(in.Counts) > 0 {
				body["counts"] = piecesBody(in.Counts)
			} else if in.CountedCash != "" {
				body["countedCash"], body["manualReason"] = in.CountedCash, in.ManualReason
			}
			if in.FloatLeft != "" {
				body["floatLeft"] = in.FloatLeft
			}
			if strings.TrimSpace(in.AuthorizedCloseReason) != "" {
				body["authorizedClose"] = map[string]any{"reason": in.AuthorizedCloseReason}
			}
			return http.MethodPost, pCashClose, nil, nil, body, nil
		},
		guard: func(s *mcpSession, ctx context.Context, in cashCloseArgs) error {
			v, err := s.openShift(ctx, "cash_session_close", in.RegisterID)
			if err != nil {
				return err
			}
			if v == nil {
				return toolErr("Esa caja no tiene turno abierto. Revisa con cash_registers_list.")
			}
			missing := domain.MissingCloseDeclarations(v.requirements(),
				domain.DeclarationsFrom(len(in.Counts) > 0 || in.CountedCash != "", in.Declared, in.TerminalCounts))
			if len(missing) > 0 {
				return toolErr("Falta lo contado de: %s. Pídeselo a quien está frente a la caja; "+
					"nunca lo llenes con lo que el sistema espera.", strings.Join(missing, ", "))
			}
			return nil
		},
		// Antes y después se leen de la ruta del turno abierto, la que filtra el arqueo ciego. El
		// resultado del cierre trae la diferencia, como la pantalla después de cerrar.
		state: func(s *mcpSession, ctx context.Context, in cashCloseArgs, result json.RawMessage) any {
			if result != nil {
				return nil
			}
			return s.soft(ctx, "cash_session_close", pCashCurrent, nil, reg(in.RegisterID))
		},
		preview: func(s *mcpSession, ctx context.Context, in cashCloseArgs) (any, []string) {
			v, err := s.openShift(ctx, "cash_session_close", in.RegisterID)
			if err != nil {
				return nil, []string{err.Error()}
			}
			if v == nil {
				return nil, []string{"Esa caja no tiene turno abierto."}
			}
			req := v.requirements()
			methods := []string{}
			for _, m := range req.Methods {
				if !m.InDrawer && !m.AutoDeclare {
					methods = append(methods, fmt.Sprintf("%s (id %d)", m.Name, m.ID))
				}
			}
			terminals := []string{}
			for _, t := range req.Terminals {
				terminals = append(terminals, fmt.Sprintf("%s (id %d)", t.Name, t.ID))
			}
			toDeclare := map[string]any{"drawer": req.DrawerNeedsCount, "methods": methods, "terminals": terminals,
				"tips_decision_required": v.TipsPending != nil && v.TipsPending.Total.GreaterThanOrEqual(decimal.NewFromInt(1)),
				"attendance":             s.soft(ctx, "cash_session_close", pCashAttendance, nil, reg(in.RegisterID))}
			missing := domain.MissingCloseDeclarations(req,
				domain.DeclarationsFrom(len(in.Counts) > 0 || in.CountedCash != "", in.Declared, in.TerminalCounts))
			notes := []string{"Lo contado lo dicta quien está frente a la caja. Quien cierra es la persona de este token."}
			if len(missing) > 0 {
				notes = append(notes, "Para aplicar falta lo contado de: "+strings.Join(missing, ", ")+".")
			}
			return map[string]any{"to_declare": toDeclare}, notes
		},
	})
}

func register073() {
	registerPurchases()
	registerCounts()
	registerOrders()
	registerCash()
}
