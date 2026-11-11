package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
)

// Las herramientas del servidor MCP. Cada una nombra las rutas que puede despachar
// (mcpToolRoutes): el despachador rechaza cualquier otra, y TestEveryToolMatchesItsRouteGate
// comprueba que existen en el router con el permiso que el catálogo de dominio declara.

//nolint:gosec // G101: son patrones de rutas de esta API; gosec confunde «token» y «waste» con credenciales.
const (
	pSalesSummary   = "/api/v1/sales/summary"
	pSales          = "/api/v1/sales"
	pOrder          = "/api/v1/orders/{id}"
	pProductsSold   = "/api/v1/reports/products-sold"
	pAttendance     = "/api/v1/admin/attendance/report"
	pCashRegisters  = "/api/v1/cash-registers"
	pCashConcepts   = "/api/v1/cash-concepts"
	pCashCurrent    = "/api/v1/cash-sessions/current"
	pCashHistory    = "/api/v1/cash-sessions/"
	pCashMovement   = "/api/v1/cash-sessions/movements"
	pExpenses       = "/api/v1/expenses/"
	pExpense        = "/api/v1/expenses/{id}"
	pExpensePay     = "/api/v1/expenses/{id}/pay"
	pExpenseCancel  = "/api/v1/expenses/{id}/cancel"
	pExpenseCats    = "/api/v1/expense-categories"
	pSuppliers      = "/api/v1/suppliers"
	pPaymentMethods = "/api/v1/payment-methods"
	pStockLevels    = "/api/v1/stock/levels"
	pStockMoves     = "/api/v1/stock/movements"
	pArticle        = "/api/v1/stock/articles/{id}"
	pWasteReasons   = "/api/v1/waste/reasons"
	pWaste          = "/api/v1/waste/"
	pProducts       = "/api/v1/admin/products/"
	pProduct        = "/api/v1/admin/products/{id}"
	pOptions        = "/api/v1/admin/modifier-options/"
	pOption         = "/api/v1/admin/modifier-options/{id}"
	pGroupOptions   = "/api/v1/admin/groups/{id}/options"
	pPosMenu        = "/api/v1/pos/menu"
	pPriceProduct   = "/api/v1/platform-prices/product"
	pPriceOption    = "/api/v1/platform-prices/modifier-option"
	pAudit          = "/api/v1/admin/audit"
)

func get(p string) mcpRoute { return mcpRoute{http.MethodGet, p} }

// mcpToolRoutes es la lista blanca del despachador: la primera ruta de cada herramienta es la que
// la atiende (y la que da su permiso); las demás son las lecturas de su vista previa.
var mcpToolRoutes = map[string][]mcpRoute{
	"whoami":                  nil,
	"sales_summary":           {get(pSalesSummary)},
	"sales_list":              {get(pSales)},
	"order_get":               {get(pOrder)},
	"report_products_sold":    {get(pProductsSold)},
	"attendance_report":       {get(pAttendance)},
	"cash_registers_list":     {get(pCashRegisters)},
	"cash_concepts_list":      {get(pCashConcepts)},
	"cash_session_current":    {get(pCashCurrent)},
	"cash_sessions_history":   {get(pCashHistory)},
	"cash_movement_create":    {{http.MethodPost, pCashMovement}, get(pCashCurrent), get(pCashConcepts)},
	"expenses_list":           {get(pExpenses)},
	"expense_get":             {get(pExpense)},
	"expense_categories_list": {get(pExpenseCats)},
	"suppliers_list":          {get(pSuppliers)},
	"payment_methods_list":    {get(pPaymentMethods)},
	"expense_create": {{http.MethodPost, pExpenses}, get(pExpenseCats), get(pSuppliers), get(pPaymentMethods),
		get(pCashRegisters), get(pCashCurrent), get(pExpense)},
	"expense_pay":            {{http.MethodPost, pExpensePay}, get(pExpense), get(pPaymentMethods)},
	"expense_cancel":         {{http.MethodPost, pExpenseCancel}, get(pExpense)},
	"supplier_create":        {{http.MethodPost, pSuppliers}, get(pSuppliers)},
	"stock_levels":           {get(pStockLevels)},
	"stock_movements":        {get(pStockMoves)},
	"waste_reasons_list":     {get(pWasteReasons)},
	"waste_record":           {{http.MethodPost, pWaste}, get(pArticle), get(pWasteReasons)},
	"stock_adjust":           {{http.MethodPost, pStockMoves}, get(pArticle)},
	"products_list":          {get(pProducts)},
	"modifier_options_list":  {get(pOptions)},
	"platform_prices_list":   {get(pPosMenu)},
	"product_update":         {{http.MethodPatch, pProduct}, get(pProducts)},
	"modifier_option_update": {{http.MethodPatch, pOption}, get(pOptions), get(pGroupOptions)},
	"platform_price_set":     {{http.MethodPut, pPriceProduct}, {http.MethodPut, pPriceOption}, get(pPosMenu)},
	"platform_price_delete": {{http.MethodDelete, pPriceProduct}, {http.MethodDelete, pPriceOption},
		get(pPosMenu)},
	"audit_list": {get(pAudit)},
}

// mcpListTool es la lectura con la que se busca el id que una herramienta pide (para el 404).
var mcpListTool = map[string]string{
	"order_get": "sales_list", "expense_get": "expenses_list", "expense_pay": "expenses_list",
	"expense_cancel": "expenses_list", "cash_session_current": "cash_registers_list",
	"cash_movement_create": "cash_registers_list", "waste_record": "stock_levels", "stock_adjust": "stock_levels",
	"product_update": "products_list", "modifier_option_update": "modifier_options_list",
	"platform_price_set": "platform_prices_list", "platform_price_delete": "platform_prices_list",
}

// mcpMaxSavedResult es lo más que se guarda de la respuesta de una escritura para repetirla; el check
// de la tabla corta en 256 KB.
const mcpMaxSavedResult = 200 << 10

// toolError es un error que el agente lee: causa y cómo corregirlo. status es el de la API cuando
// lo hubo, para que la bitácora distinga un permiso negado de un argumento malo.
type toolError struct {
	msg    string
	status int
}

func (e toolError) Error() string { return e.msg }

func toolErr(format string, a ...any) error { return toolError{msg: fmt.Sprintf(format, a...)} }

// ---- Argumentos comunes ----

type rangeArgs struct {
	Preset string `json:"preset,omitempty" jsonschema:"hoy, ayer, semana, mes, 30d, o rango con from y to"`
	From   string `json:"from,omitempty" jsonschema:"primer día, AAAA-MM-DD (con preset rango)"`
	To     string `json:"to,omitempty" jsonschema:"último día, AAAA-MM-DD (con preset rango)"`
}

func (r rangeArgs) query() url.Values {
	q := url.Values{}
	for k, v := range map[string]string{"preset": r.Preset, "from": r.From, "to": r.To} {
		if v != "" {
			q.Set(k, v)
		}
	}
	return q
}

type pageArgs struct {
	Page     int `json:"page,omitempty" jsonschema:"página, desde 1"`
	PageSize int `json:"page_size,omitempty" jsonschema:"renglones por página"`
}

// apiPage traduce la página desde 1 del agente a la desde 0 de la API, con tope.
func (p pageArgs) apiPage(def, maxSize int) (page, size int, err error) {
	page, size = p.Page, p.PageSize
	if page == 0 {
		page = 1
	}
	if size == 0 {
		size = def
	}
	if page < 1 || size < 1 || size > maxSize {
		return 0, 0, toolErr("page va desde 1 y page_size de 1 a %d. Corrige los argumentos.", maxSize)
	}
	return page - 1, size, nil
}

// writeCommon va en toda escritura. DryRun es puntero: ausente o null es vista previa, y solo un
// false explícito aplica. Con un bool, el cero aplicaría sin que nadie lo pidiera.
type writeCommon struct {
	DryRun         *bool  `json:"dry_run,omitempty" jsonschema:"true o ausente: vista previa sin cambiar nada; false: aplica"`
	IdempotencyKey string `json:"idempotency_key,omitempty" jsonschema:"obligatoria al aplicar: 8 a 100 caracteres A-Z a-z 0-9 _ . : -, nueva por cada escritura distinta"`
}

func (w writeCommon) common() writeCommon { return w }

type writeArgs interface{ common() writeCommon }

// ---- Esquemas: dinero y fechas con forma ----

var (
	moneyPattern    = `^-?[0-9]{1,9}(\.[0-9]{1,2})?$`
	quantityPattern = `^-?[0-9]{1,9}(\.[0-9]{1,4})?$`
	datePattern     = `^[0-9]{4}-[0-9]{2}-[0-9]{2}$`
	moneyFields     = map[string]bool{"amount": true, "price": true, "price_delta": true}
	quantityFields  = map[string]bool{"qty": true, "quantity": true}
	dateFields      = map[string]bool{"from": true, "to": true, "expense_date": true, "paid_on": true,
		"document_date": true, "received_on": true}
	// moneyMapFields son mapas {id: monto}: el patrón va en cada valor.
	moneyMapFields = map[string]bool{}
)

// schemaFor infiere el esquema del struct y les pone forma al dinero, las cantidades y las fechas:
// un número en `amount` se rechaza antes de llegar a la herramienta (dinero como texto, FR-016).
func schemaFor[T any]() *jsonschema.Schema {
	s, err := jsonschema.For[T](nil)
	if err != nil {
		panic(err)
	}
	var walk func(*jsonschema.Schema)
	walk = func(s *jsonschema.Schema) {
		if s == nil {
			return
		}
		for name, p := range s.Properties {
			switch {
			case moneyFields[name]:
				p.Pattern = moneyPattern
			case quantityFields[name]:
				p.Pattern = quantityPattern
			case dateFields[name]:
				p.Pattern = datePattern
			case moneyMapFields[name] && p.AdditionalProperties != nil:
				p.AdditionalProperties.Pattern = moneyPattern
			}
			walk(p)
			walk(p.Items)
		}
	}
	walk(s)
	return s
}

// ---- Registro ----

type toolReg func(s *mcpSession, srv *mcp.Server)

var mcpToolRegs = map[string]toolReg{}

func (s *mcpSession) register(srv *mcp.Server) {
	for _, t := range domain.MCPTools() {
		if !s.allows(t) {
			continue
		}
		if reg, ok := mcpToolRegs[t.Name]; ok {
			reg(s, srv)
		}
	}
}

func toolDef(name string, schema *jsonschema.Schema) *mcp.Tool {
	t, _ := domain.MCPToolByName(name)
	destructive := t.Write
	return &mcp.Tool{Name: name, Description: t.Description, InputSchema: schema,
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: !t.Write, DestructiveHint: &destructive}}
}

// limit cuenta la llamada contra el tope del token. Falla cerrado: sin Redis no hay a quién dejar
// fuera del mostrador, y quedarse sin tope cuesta la base.
func (s *mcpSession) limit(ctx context.Context) error {
	if s.h.mcpTokens.recordAndOver(ctx, strconv.FormatInt(s.principal.TokenID, 10), mcpMaxTokenCalls) {
		slog.WarnContext(ctx, "mcp_rate_limited", "token_id", s.principal.TokenID)
		return toolErr("Demasiadas llamadas con este token. Espera un minuto y vuelve a intentar.")
	}
	return nil
}

// record deja el evento de la llamada. Sin cancelación: si el cliente cortó después de aplicar, el
// rastro igual queda.
func (s *mcpSession) record(ctx context.Context, c app.MCPCall) {
	ctx = context.WithoutCancel(ctx)
	if err := s.h.apiTokens.RecordCall(app.WithOrigin(ctx, s.origin), s.principal, c); err != nil {
		slog.ErrorContext(ctx, "mcp_audit_failed", "tool", c.Tool, "error", err.Error())
	}
}

// fetch despacha una lectura y devuelve el JSON, o un error de herramienta con causa y corrección.
func (s *mcpSession) fetch(ctx context.Context, tool, pattern string, params map[string]int64, q url.Values) (json.RawMessage, int, error) {
	status, body, err := s.dispatch(ctx, tool, http.MethodGet, pattern, params, q, nil)
	if err != nil {
		slog.ErrorContext(ctx, "mcp_dispatch_failed", "tool", tool, "pattern", pattern, "error", err.Error())
		return nil, 0, toolErr("Error interno del servidor MCP. Avísale al administrador.")
	}
	if status >= 400 {
		return nil, status, toolError{msg: toolHint(status, body, mcpListTool[tool]), status: status}
	}
	return body, status, nil
}

func argsJSON(v any) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}

// readTool registra una lectura.
func readTool[In any](name string, run func(s *mcpSession, ctx context.Context, in In) (any, error)) {
	schema := schemaFor[In]()
	mcpToolRegs[name] = func(s *mcpSession, srv *mcp.Server) {
		mcp.AddTool(srv, toolDef(name, schema), func(ctx context.Context, _ *mcp.CallToolRequest, in In) (*mcp.CallToolResult, any, error) {
			if err := s.limit(ctx); err != nil {
				return nil, nil, err
			}
			out, err := run(s, ctx, in)
			status := http.StatusOK
			var te toolError
			if errors.As(err, &te) {
				status = http.StatusBadRequest
				if te.status != 0 {
					status = te.status
				}
			}
			s.record(ctx, app.MCPCall{Tool: name, Mode: "read", Status: status, Args: argsJSON(in)})
			if err != nil {
				return nil, nil, err
			}
			return nil, out, nil
		})
	}
}

// simpleRead es una lectura que solo pasa la respuesta de una ruta, sin el texto que escribe un
// tercero y con la página contada desde 1, como se la pidió el agente.
func simpleRead[In any](name, pattern string, build func(in In) (map[string]int64, url.Values, error)) {
	readTool(name, func(s *mcpSession, ctx context.Context, in In) (any, error) {
		params, q, err := build(in)
		if err != nil {
			return nil, err
		}
		body, _, err := s.fetch(ctx, name, pattern, params, q)
		if err != nil {
			return nil, err
		}
		return forAgent(body), nil
	})
}

// forAgent prepara una respuesta de la API para el agente: quita el texto de terceros y, si la
// respuesta trae página, la cuenta desde 1 (la API cuenta desde 0 y el agente pide desde 1; con la
// de la API, pedir «la siguiente» repetía la primera).
func forAgent(body json.RawMessage) any {
	v := withoutCustomerText(body)
	if m, ok := v.(map[string]any); ok {
		if p, ok := m["page"].(float64); ok {
			m["page"] = p + 1
		}
	}
	return v
}

// asObject envuelve un arreglo: el contenido estructurado de MCP es un objeto.
func asObject(body json.RawMessage) any {
	t := strings.TrimSpace(string(body))
	if t == "" || t == "null" {
		return map[string]any{"result": nil}
	}
	if t[0] != '{' {
		return map[string]json.RawMessage{"items": body}
	}
	return body
}

// writeSpec describe una escritura: a qué ruta va y cómo se lee el estado antes y después.
type writeSpec[In writeArgs] struct {
	name string
	// route arma la escritura. Un error aquí es de argumentos y no llega a despachar nada.
	route func(s *mcpSession, ctx context.Context, in In) (method, pattern string, params map[string]int64, q url.Values, body any, err error)
	// state lee lo que la escritura va a cambiar (antes) y lo que cambió (después, con la respuesta).
	state func(s *mcpSession, ctx context.Context, in In, result json.RawMessage) any
	// preview agrega a la vista previa lo que conviene saber antes de aplicar (nombres resueltos,
	// avisos). Opcional.
	preview func(s *mcpSession, ctx context.Context, in In) (any, []string)
	// strictPreview reemplaza a preview cuando la vista previa ES la operación (la edición de un gasto
	// corre entera y se revierte): lo que la ruta rechaza ahí se le dice al agente como error, no como
	// nota de una vista previa «exitosa». Opcional.
	strictPreview func(s *mcpSession, ctx context.Context, in In) (any, []string, error)
	// run reemplaza al despacho único cuando la escritura son varias peticiones (el lote de conteo).
	// Despacha en orden y se detiene en la primera que falla; su respuesta y su estado se tratan como
	// los de un despacho. Opcional.
	run func(s *mcpSession, ctx context.Context, in In) (status int, resp []byte, err error)
	// guard revisa, contra el estado de AHORA, algo que la ruta no exige y esta capa sí (las cifras
	// dictadas del cierre). Corre después de la clave de idempotencia: una repetición de algo que ya
	// se aplicó devuelve lo guardado aunque el estado ya no pase la revisión (la caja ya cerró).
	// Opcional.
	guard func(s *mcpSession, ctx context.Context, in In) error
}

func writeTool[In writeArgs](spec writeSpec[In]) {
	schema := schemaFor[In]()
	name := spec.name
	mcpToolRegs[name] = func(s *mcpSession, srv *mcp.Server) {
		mcp.AddTool(srv, toolDef(name, schema), func(ctx context.Context, _ *mcp.CallToolRequest, in In) (*mcp.CallToolResult, any, error) {
			if err := s.limit(ctx); err != nil {
				return nil, nil, err
			}
			out, err := apply(s, ctx, spec, in)
			if err != nil {
				return nil, nil, err
			}
			return nil, out, nil
		})
	}
}

// canonicalArgs son los argumentos sin dry_run ni la clave, con las llaves ordenadas: la huella de
// una escritura no depende de cómo los ordenó el agente.
func canonicalArgs(in any) []byte {
	var m map[string]any
	_ = json.Unmarshal(argsJSON(in), &m)
	delete(m, "dry_run")
	delete(m, "idempotency_key")
	b, _ := json.Marshal(m)
	return b
}

// apply es el camino común de toda escritura, con los tipos ya resueltos.
func apply[In writeArgs](s *mcpSession, ctx context.Context, spec writeSpec[In], in In) (any, error) {
	name := spec.name
	tool, _ := domain.MCPToolByName(name)
	if !domain.ScopeAllows(s.principal.Scopes, tool.Scope) {
		return nil, toolError{msg: domain.MissingScopeMessage(tool.Scope), status: http.StatusForbidden}
	}
	args := argsJSON(in)
	method, pattern, params, q, body, err := spec.route(s, ctx, in)
	if err != nil {
		s.record(ctx, app.MCPCall{Tool: name, Mode: "preview", Status: http.StatusBadRequest, Args: args})
		return nil, err
	}
	var before any
	if spec.state != nil {
		before = spec.state(s, ctx, in, nil)
	}
	c := in.common()
	if c.DryRun == nil || *c.DryRun {
		out := map[string]any{"preview": true, "before": before, "would_send": body,
			"how_to_apply": "Repite la llamada con dry_run: false y una idempotency_key nueva."}
		if spec.strictPreview != nil {
			extra, notes, perr := spec.strictPreview(s, ctx, in)
			if perr != nil {
				status := http.StatusBadRequest
				var te toolError
				if errors.As(perr, &te) && te.status != 0 {
					status = te.status
				}
				s.record(ctx, app.MCPCall{Tool: name, Mode: "preview", Status: status, Args: args})
				return nil, perr
			}
			out["resolved"] = extra
			if len(notes) > 0 {
				out["notes"] = notes
			}
		}
		if spec.preview != nil {
			extra, notes := spec.preview(s, ctx, in)
			if extra != nil {
				out["resolved"] = extra
			}
			if len(notes) > 0 {
				out["notes"] = notes
			}
		}
		s.record(ctx, app.MCPCall{Tool: name, Mode: "preview", Status: http.StatusOK, Args: args})
		return out, nil
	}
	if !domain.ValidIdempotencyKey(c.IdempotencyKey) {
		s.record(ctx, app.MCPCall{Tool: name, Mode: "apply", Status: http.StatusBadRequest, Args: args})
		return nil, toolErr("Para aplicar hace falta idempotency_key: de 8 a 100 caracteres (letras, números, _ . : -), una nueva por cada escritura distinta.")
	}
	ref := domain.IdempotencyKeyRef{CompanyID: s.principal.CompanyID, TokenID: s.principal.TokenID,
		Key: c.IdempotencyKey, Tool: name}
	bg := context.WithoutCancel(ctx)
	decision, saved, err := s.h.apiTokens.BeginIdempotent(bg, ref, domain.IdempotencyArgsHash(name, canonicalArgs(in)))
	if err != nil {
		return nil, toolErr("Error interno; no se aplicó nada. Vuelve a intentar.")
	}
	switch decision {
	case domain.IdemReplay:
		s.record(ctx, app.MCPCall{Tool: name, Mode: "apply", Status: http.StatusOK, Args: args})
		return map[string]any{"applied": true, "replayed": true, "result": json.RawMessage(saved)}, nil
	case domain.IdemProceed:
	default:
		s.record(ctx, app.MCPCall{Tool: name, Mode: "apply", Status: http.StatusConflict, Args: args})
		return nil, toolError{msg: domain.IdempotencyMessage(decision), status: http.StatusConflict}
	}
	if spec.guard != nil {
		if gerr := spec.guard(s, ctx, in); gerr != nil {
			if aerr := s.h.apiTokens.AbandonIdempotent(bg, ref); aerr != nil {
				slog.ErrorContext(ctx, "mcp_idempotency_abandon_failed", "tool", name, "error", aerr.Error())
			}
			status := http.StatusBadRequest
			var te toolError
			if errors.As(gerr, &te) && te.status != 0 {
				status = te.status
			}
			s.record(ctx, app.MCPCall{Tool: name, Mode: "apply", Status: status, Args: args})
			return nil, gerr
		}
	}

	var status int
	var resp []byte
	if spec.run != nil {
		status, resp, err = spec.run(s, ctx, in)
	} else {
		status, resp, err = s.dispatch(ctx, name, method, pattern, params, q, body)
	}
	switch {
	case err != nil || status >= 500:
		if err != nil {
			slog.ErrorContext(ctx, "mcp_dispatch_failed", "tool", name, "pattern", pattern, "error", err.Error())
		}
		if uerr := s.h.apiTokens.MarkIdempotentUnknown(bg, ref); uerr != nil {
			slog.ErrorContext(ctx, "mcp_idempotency_mark_failed", "tool", name, "error", uerr.Error())
		}
		s.record(ctx, app.MCPCall{Tool: name, Mode: "apply", Status: max(status, 500), Args: args})
		return nil, toolError{msg: domain.ToolErrorHint(500, "", ""), status: http.StatusInternalServerError}
	case status >= 400:
		if aerr := s.h.apiTokens.AbandonIdempotent(bg, ref); aerr != nil {
			slog.ErrorContext(ctx, "mcp_idempotency_abandon_failed", "tool", name, "error", aerr.Error())
		}
		s.record(ctx, app.MCPCall{Tool: name, Mode: "apply", Status: status, Args: args})
		return nil, toolError{msg: toolHint(status, resp, mcpListTool[name]), status: status}
	}
	result := json.RawMessage(resp)
	if len(strings.TrimSpace(string(resp))) == 0 {
		result = json.RawMessage(`{"ok":true}`)
	}
	var after any
	if spec.state != nil {
		after = spec.state(s, ctx, in, result)
	}
	stored := result
	if len(stored) > mcpMaxSavedResult {
		// El check de la tabla rechazaría guardarlo, y la clave se quedaría «en curso» para siempre.
		stored = json.RawMessage(`{"applied":true,"note":"respuesta demasiado grande para guardarla; consulta el estado con una lectura"}`)
	}
	if err := s.h.apiTokens.FinishIdempotent(bg, ref, stored); err != nil {
		// Ya se aplicó: la clave dice «pudo haberse aplicado» en vez de «en curso», para que un
		// reintento no espere 7 días ni, con otra clave, repita la escritura sin revisar.
		slog.ErrorContext(ctx, "mcp_idempotency_finish_failed", "tool", name, "error", err.Error())
		_ = s.h.apiTokens.MarkIdempotentUnknown(bg, ref)
	}
	s.record(ctx, app.MCPCall{Tool: name, Mode: "apply", Status: status, Args: args,
		EntityBefore: argsJSON(before), EntityAfter: argsJSON(after)})
	return map[string]any{"applied": true, "result": result, "before": before, "after": after}, nil
}

// soft lee una parte del estado para la vista previa: un 403 o un 404 se vuelven nota, no error,
// porque la vista previa no debe brincarse un permiso ni detener una escritura que sí se puede.
func (s *mcpSession) soft(ctx context.Context, tool, pattern string, params map[string]int64, q url.Values) any {
	body, _, err := s.fetch(ctx, tool, pattern, params, q)
	if err != nil {
		return map[string]string{"unavailable": err.Error()}
	}
	return body
}

// ---- Lecturas ----

type noArgs struct{}

func positive(name string, v int64) error {
	if v <= 0 {
		return toolErr("%s es obligatorio y mayor que cero.", name)
	}
	return nil
}

type orderArgs struct {
	OrderID int64 `json:"order_id" jsonschema:"id del pedido (de sales_list)"`
}

type salesArgs struct {
	rangeArgs
	pageArgs
	Status      string `json:"status,omitempty" jsonschema:"filtra por estado del pedido"`
	ServiceType string `json:"service_type,omitempty" jsonschema:"filtra por tipo de servicio"`
}

// summaryArgs: el resumen no filtra por estado ni pagina (el handler no lo aplica); anunciarlo en el
// esquema y no mandarlo haría que el agente leyera el total general como el de un estado.
type summaryArgs struct {
	rangeArgs
	ServiceType string `json:"service_type,omitempty" jsonschema:"filtra por tipo de servicio"`
}

type productsSoldArgs struct {
	rangeArgs
	Limit int `json:"limit,omitempty" jsonschema:"cuántos productos (por omisión 50)"`
}

type attendanceArgs struct {
	From   string `json:"from" jsonschema:"primer día, AAAA-MM-DD"`
	To     string `json:"to" jsonschema:"último día, AAAA-MM-DD"`
	UserID int64  `json:"user_id,omitempty" jsonschema:"solo esta persona"`
}

type registerArg struct {
	RegisterID int64 `json:"register_id" jsonschema:"id de la caja (de cash_registers_list)"`
}

type historyArgs struct {
	pageArgs
	From string `json:"from,omitempty" jsonschema:"desde, AAAA-MM-DD"`
	To   string `json:"to,omitempty" jsonschema:"hasta, AAAA-MM-DD"`
}

type expensesArgs struct {
	pageArgs
	Status  string `json:"status,omitempty" jsonschema:"pendiente, pagada o cancelada"`
	Pending *bool  `json:"pending_receipt,omitempty" jsonschema:"true: solo con mercancía por llegar"`
}

type expenseArg struct {
	ExpenseID int64 `json:"expense_id" jsonschema:"id del gasto (de expenses_list)"`
}

type limitArg struct {
	Limit int `json:"limit,omitempty" jsonschema:"cuántos (por omisión 50)"`
}

type stockLevelsArgs struct {
	pageArgs
	Search   string `json:"search,omitempty" jsonschema:"parte del nombre"`
	ItemType string `json:"item_type,omitempty" jsonschema:"ingrediente o producto"`
}

type catalogListArgs struct {
	pageArgs
	Search string `json:"search,omitempty" jsonschema:"parte del nombre"`
	Status string `json:"status,omitempty" jsonschema:"act (activos), inact (inactivos) o vacío (todos)"`
}

type platformPricesArgs struct {
	ProductID int64 `json:"product_id,omitempty" jsonschema:"solo este producto"`
	OptionID  int64 `json:"option_id,omitempty" jsonschema:"solo este extra"`
}

type auditArgs struct {
	From        string `json:"from" jsonschema:"primer día, AAAA-MM-DD"`
	To          string `json:"to" jsonschema:"último día, AAAA-MM-DD"`
	Action      string `json:"action,omitempty" jsonschema:"una acción, p. ej. expense.created"`
	Group       string `json:"group,omitempty" jsonschema:"money, cancel, price, cash o attendance"`
	ActorUserID int64  `json:"actor_user_id,omitempty" jsonschema:"solo lo que hizo esta persona"`
	Page        int    `json:"page,omitempty" jsonschema:"página, desde 1"`
}

func pageQuery(p pageArgs, def, maxSize int) (url.Values, error) {
	page, size, err := p.apiPage(def, maxSize)
	if err != nil {
		return nil, err
	}
	return url.Values{"page": {strconv.Itoa(page)}, "pageSize": {strconv.Itoa(size)}}, nil
}

func init() {
	readTool("whoami", func(s *mcpSession, _ context.Context, _ noArgs) (any, error) {
		tools := []string{}
		for _, t := range domain.MCPTools() {
			if s.allows(t) {
				tools = append(tools, t.Name)
			}
		}
		return map[string]any{"user": s.user.Name, "token": s.principal.TokenName, "scopes": s.principal.Scopes,
			"permissions": nonNilPerms(s.perms), "tools": tools}, nil
	})
	simpleRead("sales_summary", pSalesSummary, func(in summaryArgs) (map[string]int64, url.Values, error) {
		q := in.query()
		if in.ServiceType != "" {
			q.Set("serviceType", in.ServiceType)
		}
		return nil, q, nil
	})
	simpleRead("sales_list", pSales, func(in salesArgs) (map[string]int64, url.Values, error) {
		q, err := pageQuery(in.pageArgs, 20, domain.MaxSalesPageSize)
		if err != nil {
			return nil, nil, err
		}
		for k, v := range in.query() {
			q[k] = v
		}
		if in.Status != "" {
			q.Set("status", in.Status)
		}
		if in.ServiceType != "" {
			q.Set("serviceType", in.ServiceType)
		}
		return nil, q, nil
	})
	readTool("order_get", func(s *mcpSession, ctx context.Context, in orderArgs) (any, error) {
		if err := positive("order_id", in.OrderID); err != nil {
			return nil, err
		}
		body, _, err := s.fetch(ctx, "order_get", pOrder, map[string]int64{"id": in.OrderID}, nil)
		if err != nil {
			return nil, err
		}
		return forAgent(body), nil
	})
	simpleRead("report_products_sold", pProductsSold, func(in productsSoldArgs) (map[string]int64, url.Values, error) {
		q := in.query()
		if in.Limit != 0 {
			q.Set("limit", strconv.Itoa(in.Limit))
		}
		return nil, q, nil
	})
	simpleRead("attendance_report", pAttendance, func(in attendanceArgs) (map[string]int64, url.Values, error) {
		q := url.Values{"from": {in.From}, "to": {in.To}}
		if in.UserID != 0 {
			q.Set("userId", strconv.FormatInt(in.UserID, 10))
		}
		return nil, q, nil
	})
	simpleRead("cash_registers_list", pCashRegisters, func(noArgs) (map[string]int64, url.Values, error) { return nil, nil, nil })
	simpleRead("cash_concepts_list", pCashConcepts, func(noArgs) (map[string]int64, url.Values, error) { return nil, nil, nil })
	simpleRead("cash_session_current", pCashCurrent, func(in registerArg) (map[string]int64, url.Values, error) {
		if err := positive("register_id", in.RegisterID); err != nil {
			return nil, nil, err
		}
		return nil, url.Values{"registerId": {strconv.FormatInt(in.RegisterID, 10)}}, nil
	})
	simpleRead("cash_sessions_history", pCashHistory, func(in historyArgs) (map[string]int64, url.Values, error) {
		q, err := pageQuery(in.pageArgs, 20, domain.MaxSalesPageSize)
		if err != nil {
			return nil, nil, err
		}
		if in.From != "" {
			q.Set("from", in.From)
		}
		if in.To != "" {
			q.Set("to", in.To)
		}
		return nil, q, nil
	})
	simpleRead("expenses_list", pExpenses, func(in expensesArgs) (map[string]int64, url.Values, error) {
		q, err := pageQuery(in.pageArgs, 20, 100)
		if err != nil {
			return nil, nil, err
		}
		if in.Status != "" {
			q.Set("status", in.Status)
		}
		if in.Pending != nil {
			q.Set("pending", strconv.FormatBool(*in.Pending))
		}
		return nil, q, nil
	})
	simpleRead("expense_get", pExpense, func(in expenseArg) (map[string]int64, url.Values, error) {
		if err := positive("expense_id", in.ExpenseID); err != nil {
			return nil, nil, err
		}
		return map[string]int64{"id": in.ExpenseID}, nil, nil
	})
	simpleRead("expense_categories_list", pExpenseCats, func(noArgs) (map[string]int64, url.Values, error) { return nil, nil, nil })
	simpleRead("suppliers_list", pSuppliers, func(noArgs) (map[string]int64, url.Values, error) { return nil, nil, nil })
	simpleRead("payment_methods_list", pPaymentMethods, func(noArgs) (map[string]int64, url.Values, error) { return nil, nil, nil })
	readTool("stock_levels", func(s *mcpSession, ctx context.Context, in stockLevelsArgs) (any, error) {
		page, size, err := in.apiPage(50, 200)
		if err != nil {
			return nil, err
		}
		body, _, err := s.fetch(ctx, "stock_levels", pStockLevels, nil, nil)
		if err != nil {
			return nil, err
		}
		var env struct {
			Items []map[string]any `json:"items"`
		}
		_ = json.Unmarshal(body, &env)
		search := strings.ToLower(strings.TrimSpace(in.Search))
		var hits []map[string]any
		for _, it := range env.Items {
			name, _ := it["item_name"].(string)
			typ, _ := it["item_type"].(string)
			if search != "" && !strings.Contains(strings.ToLower(name), search) {
				continue
			}
			if in.ItemType != "" && typ != in.ItemType {
				continue
			}
			hits = append(hits, it)
		}
		total := len(hits)
		from := min(page*size, total)
		to := min(from+size, total)
		items := hits[from:to]
		if items == nil {
			items = []map[string]any{}
		}
		return map[string]any{"items": items, "total": total, "page": page + 1, "page_size": size}, nil
	})
	simpleRead("stock_movements", pStockMoves, func(in limitArg) (map[string]int64, url.Values, error) {
		if in.Limit == 0 {
			return nil, nil, nil
		}
		return nil, url.Values{"limit": {strconv.Itoa(in.Limit)}}, nil
	})
	simpleRead("waste_reasons_list", pWasteReasons, func(noArgs) (map[string]int64, url.Values, error) { return nil, nil, nil })
	catalogQuery := func(in catalogListArgs) (map[string]int64, url.Values, error) {
		page, size, err := in.apiPage(25, 100)
		if err != nil {
			return nil, nil, err
		}
		q := url.Values{"limit": {strconv.Itoa(size)}, "offset": {strconv.Itoa(page * size)}}
		if in.Search != "" {
			q.Set("search", in.Search)
		}
		if in.Status != "" {
			if in.Status != "act" && in.Status != "inact" {
				return nil, nil, toolErr("status es act, inact o vacío.")
			}
			q.Set("status", in.Status)
		}
		return nil, q, nil
	}
	simpleRead("products_list", pProducts, catalogQuery)
	simpleRead("modifier_options_list", pOptions, catalogQuery)
	readTool("platform_prices_list", func(s *mcpSession, ctx context.Context, in platformPricesArgs) (any, error) {
		body, _, err := s.fetch(ctx, "platform_prices_list", pPosMenu, nil, nil)
		if err != nil {
			return nil, err
		}
		return projectPlatformPrices(body, in.ProductID, in.OptionID), nil
	})
	simpleRead("audit_list", pAudit, func(in auditArgs) (map[string]int64, url.Values, error) {
		q := url.Values{"from": {in.From}, "to": {in.To}}
		if in.Action != "" {
			q.Set("action", in.Action)
		}
		if in.Group != "" {
			q.Set("group", in.Group)
		}
		if in.ActorUserID != 0 {
			q.Set("actorUserId", strconv.FormatInt(in.ActorUserID, 10))
		}
		if in.Page > 1 {
			q.Set("page", strconv.Itoa(in.Page-1))
		} else if in.Page < 0 {
			return nil, nil, toolErr("page va desde 1.")
		}
		return nil, q, nil
	})
	registerWrites()
}

// thirdPartyText son las llaves que traen texto que escribe alguien de afuera (el cliente, o la
// plataforma de reparto por él): un texto así no debe llegarle al agente como si fuera una
// instrucción. Los importes, folios y fechas sí pasan.
var thirdPartyText = []string{"customerName", "customer", "folioName", "notes", "note", "voidReason"}

// withoutCustomerText quita de cualquier profundidad las llaves de thirdPartyText.
func withoutCustomerText(body json.RawMessage) any {
	var v any
	if json.Unmarshal(body, &v) != nil {
		return asObject(body)
	}
	if _, isObj := v.(map[string]any); !isObj {
		v = map[string]any{"items": v}
	}
	var strip func(any)
	strip = func(x any) {
		switch t := x.(type) {
		case map[string]any:
			for _, k := range thirdPartyText {
				delete(t, k)
			}
			for _, c := range t {
				strip(c)
			}
		case []any:
			for _, c := range t {
				strip(c)
			}
		}
	}
	strip(v)
	return v
}

// projectPlatformPrices saca del menú solo las plataformas y las excepciones de precio.
func projectPlatformPrices(body json.RawMessage, productID, optionID int64) any {
	var doc struct {
		Platforms         []map[string]any                      `json:"platforms"`
		PlatformPrices    map[string]map[string]decimal.Decimal `json:"platformPrices"`
		PlatformModPrices map[string]map[string]decimal.Decimal `json:"platformModPrices"`
		PlatformSynced    map[string]map[string]any             `json:"platformSynced"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		return map[string]any{"unavailable": "No se pudo leer el menú. Vuelve a intentar."}
	}
	if doc.Platforms == nil {
		doc.Platforms = []map[string]any{}
	}
	filter := func(m map[string]map[string]decimal.Decimal, id int64) map[string]map[string]decimal.Decimal {
		out := map[string]map[string]decimal.Decimal{}
		for pl, prices := range m {
			out[pl] = map[string]decimal.Decimal{}
			for k, v := range prices {
				if id == 0 || k == strconv.FormatInt(id, 10) {
					out[pl][k] = v
				}
			}
		}
		return out
	}
	out := map[string]any{"platforms": doc.Platforms,
		"note": "Solo las excepciones: un producto sin precio propio usa precio base × (1 + markupPct). Los que están en synced los pone la plataforma conectada y no se editan."}
	if optionID == 0 {
		out["product_prices"] = filter(doc.PlatformPrices, productID)
	}
	if productID == 0 {
		out["option_prices"] = filter(doc.PlatformModPrices, optionID)
	}
	if doc.PlatformSynced != nil {
		out["synced"] = doc.PlatformSynced
	}
	return out
}

// MCPToolRoute es la ruta que atiende una herramienta (la primera de su lista blanca). La usa la
// prueba que comprueba, contra el router de verdad, que el permiso de cada herramienta es el de
// su ruta.
func MCPToolRoute(name string) (method, pattern string, ok bool) {
	routes := mcpToolRoutes[name]
	if len(routes) == 0 {
		return "", "", false
	}
	return routes[0].method, routes[0].pattern, true
}
