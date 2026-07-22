package httpapi

import (
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
	items, total, err := h.backoffice.ListExpenses(r.Context(), r.URL.Query().Get("status"), int32(pageSize), int32(page*pageSize))
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, map[string]any{"items": items, "total": total, "page": page, "pageSize": pageSize})
}

func (h *Handlers) CreateExpense(w http.ResponseWriter, r *http.Request) {
	var body struct {
		CategoryID  int64           `json:"categoryId"`
		SupplierID  *int64          `json:"supplierId"`
		Amount      decimal.Decimal `json:"amount"`
		Description string          `json:"description"`
		Status      string          `json:"status"`
		MethodID    *int16          `json:"methodId"`
		RegisterID  *int64          `json:"registerId"`
	}
	if err := Decode(r, &body); err != nil {
		Error(w, err)
		return
	}
	u, _ := userFrom(r.Context())
	id, err := h.backoffice.CreateExpense(r.Context(), app.ExpenseInput{
		CategoryID: body.CategoryID, SupplierID: body.SupplierID, Amount: body.Amount,
		Description: body.Description, Status: body.Status, MethodID: body.MethodID, RegisterID: body.RegisterID, UserID: u.ID,
	})
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusCreated, map[string]any{"id": id})
}

func (h *Handlers) PayExpense(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		Error(w, domain.ErrValidation)
		return
	}
	var body struct {
		MethodID   int16 `json:"methodId"`
		RegisterID int64 `json:"registerId"`
	}
	if err := Decode(r, &body); err != nil {
		Error(w, err)
		return
	}
	u, _ := userFrom(r.Context())
	if err := h.backoffice.PayExpense(r.Context(), id, body.MethodID, body.RegisterID, u.ID); err != nil {
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
