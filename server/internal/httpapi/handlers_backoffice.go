package httpapi

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/logging"
)

func (h *Handlers) PaymentMethods(w http.ResponseWriter, r *http.Request) {
	items, err := h.backoffice.PaymentMethods(r.Context())
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, map[string]any{"items": items})
}

// PATCH /payment-methods/{id}  {autoDeclare} — a nivel negocio, solo admin/gerente (gateado
// en el router). Marca si el método se declara solo (= esperado) al cerrar caja.
func (h *Handlers) UpdatePaymentMethod(w http.ResponseWriter, r *http.Request) {
	// bitSize 16: payment_methods.id es smallint; ParseInt (a diferencia de Atoi) rechaza lo
	// que no entra en int16 en vez de truncar/wrap y actualizar el método equivocado.
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 16)
	if err != nil {
		Error(w, err)
		return
	}
	var body struct {
		AutoDeclare bool `json:"autoDeclare"`
	}
	if err := Decode(r, &body); err != nil {
		Error(w, err)
		return
	}
	pm, err := h.backoffice.SetPaymentMethodAutoDeclare(r.Context(), int(id), body.AutoDeclare)
	if err != nil {
		Error(w, err)
		return
	}
	// Config con impacto directo en la reconciliación de caja: evento de seguridad para
	// auditoría (quién la cambió, sobre qué método, a qué valor).
	u, _ := userFrom(r.Context())
	logging.SecurityEvent(r.Context(), "payment_method_auto_declare_changed", "method_id", pm.ID, "auto_declare", pm.AutoDeclare, "user_id", u.ID)
	JSON(w, http.StatusOK, pm)
}

// ---- Cajas + cortes ----

// GET /cash-registers — cajas activas + estado (id de sesión abierta). Para pickers y la vista de caja.
func (h *Handlers) CashRegisters(w http.ResponseWriter, r *http.Request) {
	items, err := h.backoffice.CashRegisters(r.Context())
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, map[string]any{"items": items})
}

// GET /cash-registers/all — todas las cajas (incl. inactivas), para la gestión.
func (h *Handlers) AllCashRegisters(w http.ResponseWriter, r *http.Request) {
	items, err := h.backoffice.AllCashRegisters(r.Context())
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h *Handlers) CreateCashRegister(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name string `json:"name"`
	}
	if err := Decode(r, &body); err != nil {
		Error(w, err)
		return
	}
	v, err := h.backoffice.CreateCashRegister(r.Context(), body.Name)
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusCreated, v)
}

func (h *Handlers) UpdateCashRegister(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		Error(w, domain.ErrValidation)
		return
	}
	var body struct {
		Name     string `json:"name"`
		IsActive bool   `json:"isActive"`
	}
	if err := Decode(r, &body); err != nil {
		Error(w, err)
		return
	}
	v, err := h.backoffice.UpdateCashRegister(r.Context(), id, body.Name, body.IsActive)
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, v)
}

func (h *Handlers) OpenCashSession(w http.ResponseWriter, r *http.Request) {
	var body struct {
		RegisterID  int64           `json:"registerId"`
		OpeningCash decimal.Decimal `json:"openingCash"`
	}
	if err := Decode(r, &body); err != nil {
		Error(w, err)
		return
	}
	u, _ := userFrom(r.Context())
	sess, err := h.backoffice.OpenSession(r.Context(), body.RegisterID, body.OpeningCash, u.ID)
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusCreated, sess)
}

// GET /cash-sessions/current?registerId= — sesión abierta de una caja (null si está cerrada).
func (h *Handlers) CurrentCashSession(w http.ResponseWriter, r *http.Request) {
	registerID, err := strconv.ParseInt(r.URL.Query().Get("registerId"), 10, 64)
	if err != nil {
		Error(w, domain.ErrValidation)
		return
	}
	sess, err := h.backoffice.CurrentByRegister(r.Context(), registerID)
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, sess) // null si esa caja no está abierta
}

func (h *Handlers) CloseCashSession(w http.ResponseWriter, r *http.Request) {
	var body struct {
		RegisterID int64                      `json:"registerId"`
		Declared   map[string]decimal.Decimal `json:"declared"` // methodId(string) → contado
		Notes      string                     `json:"notes"`
	}
	if err := Decode(r, &body); err != nil {
		Error(w, err)
		return
	}
	declared := map[int]decimal.Decimal{}
	for k, v := range body.Declared {
		if id, err := strconv.Atoi(k); err == nil {
			declared[id] = v
		}
	}
	u, _ := userFrom(r.Context())
	sess, err := h.backoffice.CloseSession(r.Context(), body.RegisterID, u.ID, declared, body.Notes)
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, sess)
}

// POST /cash-sessions/transfer — traspaso de efectivo entre dos cajas abiertas.
func (h *Handlers) CashTransfer(w http.ResponseWriter, r *http.Request) {
	var body struct {
		FromRegisterID int64           `json:"fromRegisterId"`
		ToRegisterID   int64           `json:"toRegisterId"`
		Amount         decimal.Decimal `json:"amount"`
		Note           string          `json:"note"`
	}
	if err := Decode(r, &body); err != nil {
		Error(w, err)
		return
	}
	u, _ := userFrom(r.Context())
	id, err := h.backoffice.Transfer(r.Context(), body.FromRegisterID, body.ToRegisterID, body.Amount, body.Note, u.ID)
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusCreated, map[string]any{"id": id})
}

// GET /cash-status — ¿hay alguna caja abierta? Ligero, para el aviso del POS (cualquier rol).
func (h *Handlers) CashStatus(w http.ResponseWriter, r *http.Request) {
	open, err := h.backoffice.HasAnyOpenSession(r.Context())
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, map[string]bool{"open": open})
}

// GET /cash-sessions — histórico de cortes (últimos N).
func (h *Handlers) CashHistory(w http.ResponseWriter, r *http.Request) {
	limit := 50
	if q := r.URL.Query().Get("limit"); q != "" {
		if n, err := strconv.Atoi(q); err == nil && n > 0 && n <= 200 {
			limit = n
		}
	}
	rows, err := h.backoffice.SessionHistory(r.Context(), int32(limit))
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, map[string]any{"items": rows})
}

// GET /cash-sessions/{id} — detalle de un corte (totales guardados + movimientos).
func (h *Handlers) CashSessionDetail(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		Error(w, domain.ErrValidation)
		return
	}
	view, err := h.backoffice.SessionDetail(r.Context(), id)
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, view)
}

// POST /cash-sessions/movements — registra entrada/salida de efectivo en la sesión abierta de una caja.
func (h *Handlers) CreateCashMovement(w http.ResponseWriter, r *http.Request) {
	var body struct {
		RegisterID int64           `json:"registerId"`
		Kind       string          `json:"kind"`
		Amount     decimal.Decimal `json:"amount"`
		Concept    string          `json:"concept"`
	}
	if err := Decode(r, &body); err != nil {
		Error(w, err)
		return
	}
	u, _ := userFrom(r.Context())
	sess, err := h.backoffice.RecordCashMovement(r.Context(), body.RegisterID, body.Kind, body.Amount, body.Concept, u.ID)
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusCreated, sess)
}

// ---- Categorías de gasto ----

func (h *Handlers) ExpenseCategories(w http.ResponseWriter, r *http.Request) {
	items, err := h.backoffice.ExpenseCategories(r.Context())
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h *Handlers) CreateExpenseCategory(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name           string `json:"name"`
		FinancialGroup string `json:"financialGroup"`
	}
	if err := Decode(r, &body); err != nil {
		Error(w, err)
		return
	}
	v, err := h.backoffice.CreateExpenseCategory(r.Context(), body.Name, body.FinancialGroup)
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusCreated, v)
}

func (h *Handlers) UpdateExpenseCategory(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		Error(w, domain.ErrValidation)
		return
	}
	var body struct {
		Name           string `json:"name"`
		FinancialGroup string `json:"financialGroup"`
		IsActive       bool   `json:"isActive"`
	}
	if err := Decode(r, &body); err != nil {
		Error(w, err)
		return
	}
	v, err := h.backoffice.UpdateExpenseCategory(r.Context(), id, body.Name, body.FinancialGroup, body.IsActive)
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, v)
}

// ---- Proveedores ----

func (h *Handlers) Suppliers(w http.ResponseWriter, r *http.Request) {
	items, err := h.backoffice.Suppliers(r.Context())
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h *Handlers) CreateSupplier(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name  string  `json:"name"`
		Phone *string `json:"phone"`
		Notes *string `json:"notes"`
	}
	if err := Decode(r, &body); err != nil {
		Error(w, err)
		return
	}
	v, err := h.backoffice.CreateSupplier(r.Context(), body.Name, body.Phone, body.Notes)
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusCreated, v)
}

func (h *Handlers) UpdateSupplier(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		Error(w, domain.ErrValidation)
		return
	}
	var body struct {
		Name     string  `json:"name"`
		Phone    *string `json:"phone"`
		Notes    *string `json:"notes"`
		IsActive bool    `json:"isActive"`
	}
	if err := Decode(r, &body); err != nil {
		Error(w, err)
		return
	}
	v, err := h.backoffice.UpdateSupplier(r.Context(), id, body.Name, body.Phone, body.Notes, body.IsActive)
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, v)
}

// ---- Gastos ----

func (h *Handlers) ListExpenses(w http.ResponseWriter, r *http.Request) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 0 {
		page = 0
	}
	pageSize := 20
	if n, err := strconv.Atoi(r.URL.Query().Get("pageSize")); err == nil && n >= 1 && n <= 100 {
		pageSize = n
	}
	// pendingReceipt: la bandeja de "mercancía por llegar" (un pedido pagado que aún no entra).
	pending := r.URL.Query().Get("pendingReceipt") == "true"
	// Orden por columna: solo valores conocidos (lo demás → fecha desc, el default del query).
	sort := r.URL.Query().Get("sort")
	switch sort {
	case "date", "status", "category", "supplier", "description", "amount":
	default:
		sort = ""
	}
	dir := r.URL.Query().Get("dir")
	if dir != "desc" {
		dir = "asc"
	}
	items, total, err := h.backoffice.ListExpenses(r.Context(), r.URL.Query().Get("status"), pending, sort, dir, int32(pageSize), int32(page*pageSize))
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, map[string]any{"items": items, "total": total, "page": page, "pageSize": pageSize})
}

func (h *Handlers) ExpenseDetail(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		Error(w, domain.ErrValidation)
		return
	}
	v, err := h.backoffice.ExpenseDetail(r.Context(), id)
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, v)
}

// expenseItemBody es una línea de mercancía del gasto tal como llega del formulario.
type expenseItemBody struct {
	ItemType      string           `json:"itemType"` // "" = línea no inventariable
	IngredientID  *int64           `json:"ingredientId"`
	ProductID     *int64           `json:"productId"`
	Description   string           `json:"description"`
	Quantity      decimal.Decimal  `json:"quantity"`
	UnitID        *int16           `json:"unitId"`
	QtyReceived   *decimal.Decimal `json:"qtyReceived"`
	Amount        decimal.Decimal  `json:"amount"`
	PackQtyInBase *decimal.Decimal `json:"packQtyInBase"`
	RawCode       string           `json:"rawCode"`
	RawName       string           `json:"rawName"`
	// Personal: venía en el ticket pero no es del local. No se guarda como línea del gasto.
	Personal bool `json:"personal"`
}

// expensePaymentBody es un pago. registerId no-nil = entra al arqueo de esa caja; para métodos
// que mueven el cajón el servicio lo EXIGE.
type expensePaymentBody struct {
	MethodID   int16           `json:"methodId"`
	Amount     decimal.Decimal `json:"amount"`
	PaidOn     string          `json:"paidOn"`
	RegisterID *int64          `json:"registerId"`
	Reference  string          `json:"reference"`
}

func toItemInputs(in []expenseItemBody) []app.ExpenseItemInput {
	out := make([]app.ExpenseItemInput, len(in))
	for i, it := range in {
		out[i] = app.ExpenseItemInput{
			ItemType: it.ItemType, IngredientID: it.IngredientID, ProductID: it.ProductID,
			Description: it.Description, Quantity: it.Quantity, UnitID: it.UnitID,
			QtyReceived: it.QtyReceived, Amount: it.Amount, PackQtyInBase: it.PackQtyInBase,
			RawCode: it.RawCode, RawName: it.RawName, Personal: it.Personal,
		}
	}
	return out
}

func toPaymentInputs(in []expensePaymentBody) []app.ExpensePaymentInput {
	out := make([]app.ExpensePaymentInput, len(in))
	for i, p := range in {
		out[i] = app.ExpensePaymentInput{
			MethodID: p.MethodID, Amount: p.Amount, PaidOn: p.PaidOn,
			RegisterID: p.RegisterID, Reference: p.Reference,
		}
	}
	return out
}

func (h *Handlers) CreateExpense(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ExpenseDate string               `json:"expenseDate"`
		ReceivedAt  string               `json:"receivedAt"`
		CategoryID  int64                `json:"categoryId"`
		SupplierID  *int64               `json:"supplierId"`
		Amount      decimal.Decimal      `json:"amount"`
		Description string               `json:"description"`
		Status      string               `json:"status"`
		Items       []expenseItemBody    `json:"items"`
		Payments    []expensePaymentBody `json:"payments"`
		DocKind     string               `json:"docKind"`
		DocFolio    string               `json:"docFolio"`
		DocRaw      json.RawMessage      `json:"docRaw"`
	}
	if err := Decode(r, &body); err != nil {
		Error(w, err)
		return
	}
	u, _ := userFrom(r.Context())
	id, err := h.backoffice.CreateExpense(r.Context(), app.ExpenseInput{
		ExpenseDate: body.ExpenseDate, ReceivedAt: body.ReceivedAt,
		CategoryID: body.CategoryID, SupplierID: body.SupplierID, Amount: body.Amount,
		Description: body.Description, Status: body.Status,
		Items: toItemInputs(body.Items), Payments: toPaymentInputs(body.Payments),
		DocKind: body.DocKind, DocFolio: body.DocFolio, DocRaw: body.DocRaw,
		UserID: u.ID,
	})
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusCreated, map[string]any{"id": id})
}

// PayExpense agrega UN pago al gasto (el "+1 nuevo pago"). Si con él los pagos cubren el
// importe, el gasto pasa a pagado; si no, sigue pendiente con un abono registrado.
func (h *Handlers) PayExpense(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		Error(w, domain.ErrValidation)
		return
	}
	var body expensePaymentBody
	if err := Decode(r, &body); err != nil {
		Error(w, err)
		return
	}
	u, _ := userFrom(r.Context())
	in := toPaymentInputs([]expensePaymentBody{body})[0]
	if err := h.backoffice.AddExpensePayment(r.Context(), id, in, u.ID); err != nil {
		Error(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ReceiveExpense marca la mercancía como recibida y genera los movimientos de almacén.
func (h *Handlers) ReceiveExpense(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		Error(w, domain.ErrValidation)
		return
	}
	var body struct {
		ReceivedAt string                     `json:"receivedAt"`
		Received   map[string]decimal.Decimal `json:"received"` // itemId → cantidad que llegó
	}
	if err := Decode(r, &body); err != nil {
		Error(w, err)
		return
	}
	u, _ := userFrom(r.Context())
	// Las cantidades recibidas se fijan antes de consumir: un renglón que no llegó va en 0 y no
	// genera movimiento.
	if len(body.Received) > 0 {
		got := make(map[int64]decimal.Decimal, len(body.Received))
		for k, v := range body.Received {
			itemID, err := strconv.ParseInt(k, 10, 64)
			if err != nil {
				Error(w, domain.ErrValidation)
				return
			}
			got[itemID] = v
		}
		if err := h.backoffice.SetItemsReceived(r.Context(), id, got); err != nil {
			Error(w, err)
			return
		}
	}
	if err := h.backoffice.ReceiveExpense(r.Context(), id, body.ReceivedAt, u.ID); err != nil {
		Error(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handlers) CancelExpense(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		Error(w, domain.ErrValidation)
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
	if err := h.backoffice.CancelExpense(r.Context(), id, body.Reason, u.ID); err != nil {
		Error(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---- Almacén ----

func (h *Handlers) StockLevels(w http.ResponseWriter, r *http.Request) {
	items, err := h.backoffice.StockLevels(r.Context())
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h *Handlers) StockMovements(w http.ResponseWriter, r *http.Request) {
	items, err := h.backoffice.StockMovements(r.Context(), queryLimit(r, 100))
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h *Handlers) CreateStockMovement(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ItemType     string          `json:"itemType"` // ingrediente | producto
		IngredientID *int64          `json:"ingredientId"`
		ProductID    *int64          `json:"productId"`
		MovementType string          `json:"movementType"` // ajuste | compra | merma
		Quantity     decimal.Decimal `json:"quantity"`
		Reason       string          `json:"reason"`
	}
	if err := Decode(r, &body); err != nil {
		Error(w, err)
		return
	}
	u, _ := userFrom(r.Context())
	if err := h.backoffice.RecordMovement(r.Context(), body.ItemType, body.IngredientID, body.ProductID, body.MovementType, body.Quantity, body.Reason, u.ID); err != nil {
		Error(w, err)
		return
	}
	w.WriteHeader(http.StatusCreated)
}

// ---- Reportes ----

func (h *Handlers) ReportSales(w http.ResponseWriter, r *http.Request) {
	to := parseDate(r.URL.Query().Get("to"), time.Now())
	from := parseDate(r.URL.Query().Get("from"), to.AddDate(0, 0, -30))
	rows, err := h.backoffice.SalesByDay(r.Context(), from, to)
	if err != nil {
		Error(w, err)
		return
	}
	methods, err := h.backoffice.SalesByMethod(r.Context(), from)
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, map[string]any{"byDay": rows, "byMethod": methods})
}

// GET /reports/tips?from=&to= — propinas por empleado (para repartir) y por día.
func (h *Handlers) ReportTips(w http.ResponseWriter, r *http.Request) {
	to := parseDate(r.URL.Query().Get("to"), time.Now())
	from := parseDate(r.URL.Query().Get("from"), to.AddDate(0, 0, -30))
	byEmployee, err := h.backoffice.TipsByEmployee(r.Context(), from, to)
	if err != nil {
		Error(w, err)
		return
	}
	byDay, err := h.backoffice.TipsByDay(r.Context(), from, to)
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, map[string]any{"byEmployee": byEmployee, "byDay": byDay})
}

func (h *Handlers) ReportMargins(w http.ResponseWriter, r *http.Request) {
	since := parseDate(r.URL.Query().Get("since"), time.Now().AddDate(0, 0, -30))
	rows, err := h.backoffice.ProductMargins(r.Context(), since, queryLimit(r, 50))
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, map[string]any{"items": rows})
}

// ---- helpers ----

func parseDate(s string, fallback time.Time) time.Time {
	if s == "" {
		return fallback
	}
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return fallback
	}
	return t
}

func queryLimit(r *http.Request, def int32) int32 {
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return int32(n)
		}
	}
	return def
}

// ---- Catálogo de artículos (insumos) ----

func (h *Handlers) Units(w http.ResponseWriter, r *http.Request) {
	items, err := h.backoffice.Units(r.Context())
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h *Handlers) ListIngredients(w http.ResponseWriter, r *http.Request) {
	items, err := h.backoffice.Ingredients(r.Context(), r.URL.Query().Get("onlyActive") == "true")
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h *Handlers) CreateIngredient(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name        string           `json:"name"`
		BaseUnitID  int16            `json:"baseUnitId"`
		CategoryID  *int64           `json:"categoryId"`
		MinStock    *decimal.Decimal `json:"minStock"`
		TrackStock  *bool            `json:"trackStock"`
		IsPackaging *bool            `json:"isPackaging"`
	}
	if err := Decode(r, &body); err != nil {
		Error(w, err)
		return
	}
	v, err := h.backoffice.CreateIngredient(r.Context(), app.IngredientInput{
		Name: body.Name, BaseUnitID: body.BaseUnitID, CategoryID: body.CategoryID,
		MinStock: body.MinStock, TrackStock: body.TrackStock, IsPackaging: body.IsPackaging,
	})
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusCreated, v)
}

// SearchArticles alimenta el picker de artículo del gasto (ingredientes + productos con stock).
func (h *Handlers) SearchArticles(w http.ResponseWriter, r *http.Request) {
	items, err := h.backoffice.SearchArticles(r.Context(), r.URL.Query().Get("q"))
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, map[string]any{"items": items})
}

// SuggestArticles corre la cascada de mapeo para un renglón de documento. Solo SUGIERE: nada se
// aplica sin que el operador confirme.
func (h *Handlers) SuggestArticles(w http.ResponseWriter, r *http.Request) {
	supplierID, _ := strconv.ParseInt(r.URL.Query().Get("supplierId"), 10, 64)
	items, err := h.backoffice.SuggestForLine(r.Context(), supplierID,
		r.URL.Query().Get("rawCode"), r.URL.Query().Get("rawName"))
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, map[string]any{"items": items})
}

// ---- Catálogo aprendido por proveedor (revisión de mapeos) ----

func (h *Handlers) SupplierItems(w http.ResponseWriter, r *http.Request) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 0 {
		page = 0
	}
	pageSize := 30
	if n, err := strconv.Atoi(r.URL.Query().Get("pageSize")); err == nil && n >= 1 && n <= 100 {
		pageSize = n
	}
	supplierID, _ := strconv.ParseInt(r.URL.Query().Get("supplierId"), 10, 64)
	items, total, err := h.backoffice.SupplierItems(r.Context(), r.URL.Query().Get("status"),
		supplierID, int32(pageSize), int32(page*pageSize))
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, map[string]any{"items": items, "total": total, "page": page, "pageSize": pageSize})
}

// ForgetSupplierItem deshace un mapeo aprendido: la próxima compra vuelve a sugerir desde cero.
func (h *Handlers) ForgetSupplierItem(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		Error(w, domain.ErrValidation)
		return
	}
	if err := h.backoffice.ForgetSupplierItem(r.Context(), id); err != nil {
		Error(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
