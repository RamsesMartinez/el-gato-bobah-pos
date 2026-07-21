package httpapi

import (
	"net/http"
	"strconv"
	"time"

	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
)

func (h *Handlers) PaymentMethods(w http.ResponseWriter, r *http.Request) {
	items, err := h.backoffice.PaymentMethods(r.Context())
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, map[string]any{"items": items})
}

// ---- Cortes de caja ----

func (h *Handlers) OpenCashSession(w http.ResponseWriter, r *http.Request) {
	var body struct {
		OpeningCash decimal.Decimal `json:"openingCash"`
	}
	if err := Decode(r, &body); err != nil {
		Error(w, err)
		return
	}
	u, _ := userFrom(r.Context())
	sess, err := h.backoffice.OpenSession(r.Context(), body.OpeningCash, u.ID)
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusCreated, sess)
}

func (h *Handlers) CurrentCashSession(w http.ResponseWriter, r *http.Request) {
	sess, err := h.backoffice.Current(r.Context())
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, sess) // null si no hay caja abierta
}

func (h *Handlers) CloseCashSession(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Declared map[string]decimal.Decimal `json:"declared"` // methodId(string) → contado
		Notes    string                     `json:"notes"`
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
	sess, err := h.backoffice.CloseSession(r.Context(), u.ID, declared, body.Notes)
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, sess)
}

// ---- Gastos ----

func (h *Handlers) ExpenseCategories(w http.ResponseWriter, r *http.Request) {
	items, err := h.backoffice.ExpenseCategories(r.Context())
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h *Handlers) ListExpenses(w http.ResponseWriter, r *http.Request) {
	items, err := h.backoffice.ListExpenses(r.Context(), queryLimit(r, 100))
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h *Handlers) CreateExpense(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Date        string          `json:"date"`
		CategoryID  int64           `json:"categoryId"`
		SupplierID  *int64          `json:"supplierId"`
		Amount      decimal.Decimal `json:"amount"`
		MethodID    *int16          `json:"methodId"`
		Description string          `json:"description"`
	}
	if err := Decode(r, &body); err != nil {
		Error(w, err)
		return
	}
	u, _ := userFrom(r.Context())
	id, err := h.backoffice.CreateExpense(r.Context(), app.ExpenseInput{
		Date:        parseDate(body.Date, time.Now()),
		CategoryID:  body.CategoryID,
		SupplierID:  body.SupplierID,
		Amount:      body.Amount,
		MethodID:    body.MethodID,
		Description: body.Description,
		UserID:      u.ID,
	})
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusCreated, map[string]any{"id": id})
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
