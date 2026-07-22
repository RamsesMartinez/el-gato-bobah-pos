package app

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store/db"
)

// BackofficeService agrupa caja, gastos, almacén y reportes (todo lo no-POS del MVP).
type BackofficeService struct {
	store *store.Store
	now   func() time.Time
}

func NewBackofficeService(s *store.Store, now func() time.Time) *BackofficeService {
	if now == nil {
		now = time.Now
	}
	return &BackofficeService{store: s, now: now}
}

// ---- Medios de pago ----

type PaymentMethodView struct {
	ID                int    `json:"id"`
	Name              string `json:"name"`
	Kind              string `json:"kind"`
	AffectsCashDrawer bool   `json:"affectsCashDrawer"`
	AutoDeclare       bool   `json:"autoDeclare"`
}

func (s *BackofficeService) PaymentMethods(ctx context.Context) ([]PaymentMethodView, error) {
	rows, err := s.store.Q.ListPaymentMethods(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]PaymentMethodView, len(rows))
	for i, r := range rows {
		out[i] = PaymentMethodView{ID: int(r.ID), Name: r.Name, Kind: string(r.Kind), AffectsCashDrawer: r.AffectsCashDrawer, AutoDeclare: r.AutoDeclare}
	}
	return out, nil
}

// SetPaymentMethodAutoDeclare marca a nivel negocio si un método de pago se declara solo al
// cerrar caja (declarado = esperado, sin captura del cajero) o requiere conteo manual.
func (s *BackofficeService) SetPaymentMethodAutoDeclare(ctx context.Context, methodID int, auto bool) (PaymentMethodView, error) {
	current, err := s.store.Q.GetPaymentMethod(ctx, int16(methodID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return PaymentMethodView{}, domain.ErrNotFound
		}
		return PaymentMethodView{}, err
	}
	// Efectivo (affects_cash_drawer) es justo el método que exige conteo físico: auto-declararlo
	// dejaría el corte de caja sin forma de detectar un faltante de efectivo.
	if auto && current.AffectsCashDrawer {
		return PaymentMethodView{}, domain.ErrValidation
	}
	row, err := s.store.Q.UpdatePaymentMethodAutoDeclare(ctx, db.UpdatePaymentMethodAutoDeclareParams{
		ID: int16(methodID), AutoDeclare: auto,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return PaymentMethodView{}, domain.ErrNotFound
		}
		return PaymentMethodView{}, err
	}
	return PaymentMethodView{ID: int(row.ID), Name: row.Name, Kind: string(row.Kind), AffectsCashDrawer: row.AffectsCashDrawer, AutoDeclare: row.AutoDeclare}, nil
}

// ---- Cortes de caja ----

type MethodTotal struct {
	MethodID    int             `json:"methodId"`
	Name        string          `json:"name"`
	Expected    decimal.Decimal `json:"expected"`
	Declared    decimal.Decimal `json:"declared"`
	Difference  decimal.Decimal `json:"difference"`
	AutoDeclare bool            `json:"autoDeclare"`
}

type CashMovementView struct {
	ID        int64           `json:"id"`
	Kind      string          `json:"kind"` // entrada | salida
	Amount    decimal.Decimal `json:"amount"`
	Concept   string          `json:"concept"`
	CreatedAt time.Time       `json:"createdAt"`
	UserName  string          `json:"userName"`
}

type SessionView struct {
	ID           int64              `json:"id"`
	Status       string             `json:"status"`
	OpeningCash  decimal.Decimal    `json:"openingCash"`
	Currency     domain.Currency    `json:"currency"`
	OpenedAt     time.Time          `json:"openedAt"`
	NetMovements decimal.Decimal    `json:"netMovements"` // entradas − salidas de efectivo
	Totals       []MethodTotal      `json:"totals"`
	Movements    []CashMovementView `json:"movements"`
}

// SessionDetailView es una sesión (normalmente ya cerrada) con sus totales guardados y
// movimientos, para el histórico. Difiere de SessionView en que los totales vienen de
// register_session_totals (snapshot al cerrar), no del cálculo en vivo.
type SessionDetailView struct {
	ID           int64              `json:"id"`
	Status       string             `json:"status"`
	OpeningCash  decimal.Decimal    `json:"openingCash"`
	Currency     domain.Currency    `json:"currency"`
	OpenedAt     time.Time          `json:"openedAt"`
	ClosedAt     *time.Time         `json:"closedAt"`
	OpenedByName string             `json:"openedByName"`
	ClosedByName *string            `json:"closedByName"`
	Notes        *string            `json:"notes"`
	Totals       []MethodTotal      `json:"totals"`
	Movements    []CashMovementView `json:"movements"`
}

type SessionHistoryRow struct {
	ID              int64           `json:"id"`
	Status          string          `json:"status"`
	OpeningCash     decimal.Decimal `json:"openingCash"`
	Currency        domain.Currency `json:"currency"`
	OpenedAt        time.Time       `json:"openedAt"`
	ClosedAt        *time.Time      `json:"closedAt"`
	OpenedByName    string          `json:"openedByName"`
	ClosedByName    *string         `json:"closedByName"`
	TotalDifference decimal.Decimal `json:"totalDifference"`
	Notes           *string         `json:"notes"`
}

func (s *BackofficeService) OpenSession(ctx context.Context, openingCash decimal.Decimal, userID int64) (*SessionView, error) {
	// allowZero: abrir con cajón vacío es válido. Rechaza negativos (la columna no tiene
	// check) e importes absurdos antes de que desborden el numeric(10,2).
	if !domain.ValidMoney(domain.Round2(openingCash), true) {
		return nil, domain.ErrValidation
	}
	if _, err := s.store.Q.GetOpenSession(ctx); err == nil {
		return nil, domain.ErrConflict // ya hay una caja abierta
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}
	sess, err := s.store.Q.OpenSession(ctx, db.OpenSessionParams{
		BusinessDate: pgtype.Date{Time: s.now(), Valid: true},
		OpeningCash:  domain.Round2(openingCash),
		OpenedBy:     userID,
	})
	if err != nil {
		return nil, err
	}
	return s.sessionWithExpected(ctx, sess)
}

// Current devuelve la caja abierta con sus esperados en vivo, o nil si no hay.
func (s *BackofficeService) Current(ctx context.Context) (*SessionView, error) {
	sess, err := s.store.Q.GetOpenSession(ctx)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return s.sessionWithExpected(ctx, sess)
}

func (s *BackofficeService) sessionWithExpected(ctx context.Context, sess db.RegisterSession) (*SessionView, error) {
	rows, err := s.store.Q.ExpectedByMethodSince(ctx, sess.OpenedAt)
	if err != nil {
		return nil, err
	}
	// Neto de efectivo movido (entradas − salidas): entra en el esperado del cajón, junto al fondo.
	net, err := s.store.Q.NetCashMovements(ctx, sess.ID)
	if err != nil {
		return nil, err
	}
	moves, err := s.store.Q.ListCashMovements(ctx, sess.ID)
	if err != nil {
		return nil, err
	}
	// Slices no-nil: en JSON van como [] (no null), así el front no revienta con .length/.map.
	view := &SessionView{
		ID: sess.ID, Status: string(sess.Status), OpeningCash: sess.OpeningCash,
		Currency: domain.Currency(sess.Currency), OpenedAt: sess.OpenedAt, NetMovements: domain.Round2(net),
		Totals: []MethodTotal{}, Movements: []CashMovementView{},
	}
	for _, r := range rows {
		expected := r.Expected
		if r.AffectsCashDrawer {
			expected = expected.Add(sess.OpeningCash).Add(net) // fondo + neto de movimientos
		}
		view.Totals = append(view.Totals, MethodTotal{MethodID: int(r.PaymentMethodID), Name: r.Name, Expected: domain.Round2(expected), AutoDeclare: r.AutoDeclare})
	}
	for _, m := range moves {
		view.Movements = append(view.Movements, CashMovementView{
			ID: m.ID, Kind: m.Kind, Amount: m.Amount, Concept: m.Concept, CreatedAt: m.CreatedAt, UserName: m.UserName,
		})
	}
	return view, nil
}

// CloseSession cierra la caja abierta, guarda esperado vs declarado por método.
func (s *BackofficeService) CloseSession(ctx context.Context, userID int64, declared map[int]decimal.Decimal, notes string) (*SessionView, error) {
	// allowZero: un método puede cerrar en 0 (sin ventas). Rechaza negativos y absurdos.
	for _, d := range declared {
		if !domain.ValidMoney(domain.Round2(d), true) {
			return nil, domain.ErrValidation
		}
	}
	sess, err := s.store.Q.GetOpenSession(ctx)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	view, err := s.sessionWithExpected(ctx, sess)
	if err != nil {
		return nil, err
	}
	err = s.store.WithTx(ctx, func(q *db.Queries) error {
		for i := range view.Totals {
			t := &view.Totals[i]
			t.Declared = domain.Round2(domain.ResolveDeclared(t.AutoDeclare, t.Expected, declared[t.MethodID]))
			t.Difference = domain.Round2(t.Declared.Sub(t.Expected))
			if err := q.SaveSessionTotal(ctx, db.SaveSessionTotalParams{
				SessionID: sess.ID, PaymentMethodID: int16(t.MethodID),
				Expected: t.Expected, Declared: t.Declared,
			}); err != nil {
				return err
			}
		}
		var n *string
		if notes != "" {
			n = &notes
		}
		return q.CloseSession(ctx, db.CloseSessionParams{ID: sess.ID, ClosedBy: &userID, Notes: n})
	})
	if err != nil {
		return nil, err
	}
	view.Status = "cerrada"
	return view, nil
}

// RecordCashMovement registra una entrada/salida de efectivo del cajón en la caja abierta.
// El neto (entradas − salidas) entra al efectivo esperado al cerrar (ver sessionWithExpected).
func (s *BackofficeService) RecordCashMovement(ctx context.Context, kind string, amount decimal.Decimal, concept string, userID int64) (*SessionView, error) {
	if kind != "entrada" && kind != "salida" {
		return nil, domain.ErrValidation
	}
	amt := domain.Round2(amount)
	if !domain.ValidMoney(amt, false) || concept == "" { // monto > 0 y con concepto
		return nil, domain.ErrValidation
	}
	sess, err := s.store.Q.GetOpenSession(ctx)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound // sin caja abierta no hay dónde registrar
		}
		return nil, err
	}
	if _, err := s.store.Q.InsertCashMovement(ctx, db.InsertCashMovementParams{
		SessionID: sess.ID, Kind: kind, Amount: amt, Concept: concept, UserID: userID,
	}); err != nil {
		return nil, err
	}
	return s.sessionWithExpected(ctx, sess)
}

// SessionHistory lista los últimos cortes (abiertos y cerrados) para el histórico.
func (s *BackofficeService) SessionHistory(ctx context.Context, limit int32) ([]SessionHistoryRow, error) {
	rows, err := s.store.Q.ListSessions(ctx, limit)
	if err != nil {
		return nil, err
	}
	out := make([]SessionHistoryRow, len(rows))
	for i, r := range rows {
		out[i] = SessionHistoryRow{
			ID: r.ID, Status: string(r.Status), OpeningCash: r.OpeningCash,
			Currency: domain.Currency(r.Currency), OpenedAt: r.OpenedAt, ClosedAt: tsPtr(r.ClosedAt),
			OpenedByName: r.OpenedByName, ClosedByName: r.ClosedByName,
			TotalDifference: r.TotalDifference, Notes: r.Notes,
		}
	}
	return out, nil
}

// SessionDetail devuelve una sesión con sus totales GUARDADOS (snapshot al cerrar) y movimientos.
func (s *BackofficeService) SessionDetail(ctx context.Context, id int64) (*SessionDetailView, error) {
	sess, err := s.store.Q.GetSession(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	totals, err := s.store.Q.ListSessionTotals(ctx, id)
	if err != nil {
		return nil, err
	}
	moves, err := s.store.Q.ListCashMovements(ctx, id)
	if err != nil {
		return nil, err
	}
	view := &SessionDetailView{
		ID: sess.ID, Status: string(sess.Status), OpeningCash: sess.OpeningCash,
		Currency: domain.Currency(sess.Currency), OpenedAt: sess.OpenedAt, ClosedAt: tsPtr(sess.ClosedAt),
		OpenedByName: sess.OpenedByName, ClosedByName: sess.ClosedByName, Notes: sess.Notes,
		Totals: []MethodTotal{}, Movements: []CashMovementView{}, // no-nil → [] en JSON
	}
	for _, t := range totals {
		view.Totals = append(view.Totals, MethodTotal{
			MethodID: int(t.PaymentMethodID), Name: t.Name,
			Expected: t.Expected, Declared: t.Declared, Difference: t.Difference,
		})
	}
	for _, m := range moves {
		view.Movements = append(view.Movements, CashMovementView{
			ID: m.ID, Kind: m.Kind, Amount: m.Amount, Concept: m.Concept, CreatedAt: m.CreatedAt, UserName: m.UserName,
		})
	}
	return view, nil
}

// HasOpenSession: chequeo ligero (¿hay caja abierta?) para el aviso del POS, sin calcular
// esperados. Disponible a cualquier rol autenticado: saber si hay caja no es dato sensible.
func (s *BackofficeService) HasOpenSession(ctx context.Context) (bool, error) {
	if _, err := s.store.Q.GetOpenSession(ctx); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// tsPtr convierte un timestamptz anulable de pgx a *time.Time (nil si NULL).
func tsPtr(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	return &t.Time
}

// ---- Gastos ----

type ExpenseView struct {
	ID             int64           `json:"id"`
	ExpenseDate    string          `json:"expenseDate"` // YYYY-MM-DD
	Status         string          `json:"status"`
	Category       string          `json:"category"`
	FinancialGroup string          `json:"financialGroup"`
	Supplier       *string         `json:"supplier"`
	Amount         decimal.Decimal `json:"amount"`
	Currency       domain.Currency `json:"currency"`
	Description    *string         `json:"description"`
	PaymentMethod  *string         `json:"paymentMethod"`
	PaidAt         *time.Time      `json:"paidAt"`
	CreatedBy      *string         `json:"createdBy"`
}

type ExpenseInput struct {
	CategoryID  int64
	SupplierID  *int64
	Amount      decimal.Decimal
	Description string
	Status      string // pendiente | pagada
	MethodID    *int16 // requerido si status == pagada
	UserID      int64
}

// CreateExpense registra un gasto pendiente o directamente pagado. Si es pagado en efectivo y
// hay caja abierta, genera la salida de efectivo en el mismo tx (el corte cuadra solo).
func (s *BackofficeService) CreateExpense(ctx context.Context, in ExpenseInput) (int64, error) {
	amount := domain.Round2(in.Amount)
	// Monto ya redondeado (sub-centavo/absurdo → 400, no 500), categoría y estado válidos.
	if !domain.ValidMoney(amount, false) || in.CategoryID == 0 {
		return 0, domain.ErrValidation
	}
	if in.Status != domain.ExpensePendiente && in.Status != domain.ExpensePagada {
		return 0, domain.ErrValidation // no se crea directo como cancelada
	}
	var desc *string
	if in.Description != "" {
		desc = &in.Description
	}

	params := db.CreateExpenseParams{
		ExpenseDate: pgtype.Date{Time: s.now(), Valid: true},
		CategoryID:  in.CategoryID, SupplierID: in.SupplierID, Amount: amount,
		Description: desc, CreatedBy: in.UserID, Status: db.ExpenseStatus(in.Status),
	}
	var sessionID *int64
	var isCash bool
	if in.Status == domain.ExpensePagada {
		if in.MethodID == nil {
			return 0, domain.ErrValidation // pagar exige método
		}
		sid, cash, err := s.resolveCashPayment(ctx, *in.MethodID)
		if err != nil {
			return 0, err
		}
		sessionID, isCash = sid, cash
		params.PaymentMethodID = in.MethodID
		params.RegisterSessionID = sessionID
		params.PaidAt = pgtype.Timestamptz{Time: s.now(), Valid: true}
		params.PaidBy = &in.UserID
	}

	var id int64
	err := s.store.WithTx(ctx, func(q *db.Queries) error {
		var err error
		if id, err = q.CreateExpense(ctx, params); err != nil {
			return err
		}
		if isCash && sessionID != nil { // salida de efectivo ligada al gasto
			return q.InsertExpenseCashMovement(ctx, db.InsertExpenseCashMovementParams{
				SessionID: *sessionID, Amount: amount, Concept: expenseConcept(in.Description), ExpenseID: &id, UserID: in.UserID,
			})
		}
		return nil
	})
	return id, err
}

// PayExpense marca una pendiente como pagada; en efectivo con caja abierta registra la salida.
func (s *BackofficeService) PayExpense(ctx context.Context, id int64, methodID int16, userID int64) error {
	exp, err := s.store.Q.GetExpense(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrNotFound
		}
		return err
	}
	if !domain.CanPayExpense(string(exp.Status)) {
		return domain.ErrConflict // ya pagada/cancelada
	}
	sessionID, isCash, err := s.resolveCashPayment(ctx, methodID)
	if err != nil {
		return err
	}
	return s.store.WithTx(ctx, func(q *db.Queries) error {
		n, err := q.PayExpense(ctx, db.PayExpenseParams{ID: id, PaymentMethodID: &methodID, RegisterSessionID: sessionID, PaidBy: &userID})
		if err != nil {
			return err
		}
		if n == 0 {
			return domain.ErrConflict // carrera: alguien la cambió entre el GET y el UPDATE
		}
		if isCash && sessionID != nil {
			return q.InsertExpenseCashMovement(ctx, db.InsertExpenseCashMovementParams{
				SessionID: *sessionID, Amount: exp.Amount, Concept: expenseConcept(derefStr(exp.Description)), ExpenseID: &id, UserID: userID,
			})
		}
		return nil
	})
}

// CancelExpense anula una pendiente (una pagada es terminal — ver domain.CanCancelExpense).
func (s *BackofficeService) CancelExpense(ctx context.Context, id int64, reason string, userID int64) error {
	exp, err := s.store.Q.GetExpense(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrNotFound
		}
		return err
	}
	if !domain.CanCancelExpense(string(exp.Status)) {
		return domain.ErrConflict
	}
	var r *string
	if reason != "" {
		r = &reason
	}
	n, err := s.store.Q.CancelExpense(ctx, db.CancelExpenseParams{ID: id, CancelledBy: &userID, CancelReason: r})
	if err != nil {
		return err
	}
	if n == 0 {
		return domain.ErrConflict
	}
	return nil
}

func (s *BackofficeService) ListExpenses(ctx context.Context, status string, limit int32) ([]ExpenseView, error) {
	var st *db.ExpenseStatus
	if domain.ValidExpenseStatus(status) {
		s := db.ExpenseStatus(status)
		st = &s
	}
	rows, err := s.store.Q.ListExpenses(ctx, db.ListExpensesParams{Status: st, Lim: limit})
	if err != nil {
		return nil, err
	}
	out := make([]ExpenseView, len(rows))
	for i, r := range rows {
		out[i] = ExpenseView{
			ID: r.ID, ExpenseDate: r.ExpenseDate.Time.Format("2006-01-02"), Status: string(r.Status),
			Category: r.Category, FinancialGroup: string(r.FinancialGroup), Supplier: r.Supplier,
			Amount: r.Amount, Currency: domain.Currency(r.Currency), Description: r.Description,
			PaymentMethod: r.PaymentMethod, PaidAt: tsPtr(r.PaidAt), CreatedBy: r.CreatedByName,
		}
	}
	return out, nil
}

// resolveCashPayment: dado un método de pago, decide si toca el cajón (efectivo) y en qué caja.
// Efectivo sin caja abierta → se paga igual (petty cash), sin movimiento (sessionID nil).
func (s *BackofficeService) resolveCashPayment(ctx context.Context, methodID int16) (*int64, bool, error) {
	m, err := s.store.Q.GetPaymentMethod(ctx, methodID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, false, domain.ErrValidation // método inexistente
		}
		return nil, false, err
	}
	if !m.AffectsCashDrawer {
		return nil, false, nil
	}
	sess, err := s.store.Q.GetOpenSession(ctx)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, true, nil // efectivo pero sin caja abierta
		}
		return nil, true, err
	}
	return &sess.ID, true, nil
}

func expenseConcept(description string) string {
	if description != "" {
		return "Gasto: " + description
	}
	return "Gasto"
}

// ---- Proveedores ----

type SupplierView struct {
	ID       int64   `json:"id"`
	Name     string  `json:"name"`
	Phone    *string `json:"phone"`
	Notes    *string `json:"notes"`
	IsActive bool    `json:"isActive"`
}

func (s *BackofficeService) Suppliers(ctx context.Context) ([]SupplierView, error) {
	rows, err := s.store.Q.ListAllSuppliers(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]SupplierView, len(rows))
	for i, r := range rows {
		out[i] = SupplierView{ID: r.ID, Name: r.Name, Phone: r.Phone, Notes: r.Notes, IsActive: r.IsActive}
	}
	return out, nil
}

func (s *BackofficeService) CreateSupplier(ctx context.Context, name string, phone, notes *string) (SupplierView, error) {
	if name == "" {
		return SupplierView{}, domain.ErrValidation
	}
	r, err := s.store.Q.CreateSupplier(ctx, db.CreateSupplierParams{Name: name, Phone: phone, Notes: notes})
	if err != nil {
		return SupplierView{}, err
	}
	return SupplierView{ID: r.ID, Name: r.Name, Phone: r.Phone, Notes: r.Notes, IsActive: r.IsActive}, nil
}

func (s *BackofficeService) UpdateSupplier(ctx context.Context, id int64, name string, phone, notes *string, active bool) (SupplierView, error) {
	if name == "" {
		return SupplierView{}, domain.ErrValidation
	}
	r, err := s.store.Q.UpdateSupplier(ctx, db.UpdateSupplierParams{ID: id, Name: name, Phone: phone, Notes: notes, IsActive: active})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return SupplierView{}, domain.ErrNotFound
		}
		return SupplierView{}, err
	}
	return SupplierView{ID: r.ID, Name: r.Name, Phone: r.Phone, Notes: r.Notes, IsActive: r.IsActive}, nil
}

// ---- Categorías de gasto ----

type ExpenseCategoryView struct {
	ID             int64  `json:"id"`
	Name           string `json:"name"`
	FinancialGroup string `json:"financialGroup"`
	IsActive       bool   `json:"isActive"`
}

func validFinancialGroup(g string) bool {
	return g == "operacional" || g == "administrativo" || g == "otro"
}

func (s *BackofficeService) ExpenseCategories(ctx context.Context) ([]ExpenseCategoryView, error) {
	rows, err := s.store.Q.ListAllExpenseCategories(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]ExpenseCategoryView, len(rows))
	for i, r := range rows {
		out[i] = ExpenseCategoryView{ID: r.ID, Name: r.Name, FinancialGroup: string(r.FinancialGroup), IsActive: r.IsActive}
	}
	return out, nil
}

func (s *BackofficeService) CreateExpenseCategory(ctx context.Context, name, group string) (ExpenseCategoryView, error) {
	if name == "" || !validFinancialGroup(group) {
		return ExpenseCategoryView{}, domain.ErrValidation
	}
	r, err := s.store.Q.CreateExpenseCategory(ctx, db.CreateExpenseCategoryParams{Name: name, FinancialGroup: db.FinancialGroup(group)})
	if err != nil {
		return ExpenseCategoryView{}, err
	}
	return ExpenseCategoryView{ID: r.ID, Name: r.Name, FinancialGroup: string(r.FinancialGroup), IsActive: r.IsActive}, nil
}

func (s *BackofficeService) UpdateExpenseCategory(ctx context.Context, id int64, name, group string, active bool) (ExpenseCategoryView, error) {
	if name == "" || !validFinancialGroup(group) {
		return ExpenseCategoryView{}, domain.ErrValidation
	}
	r, err := s.store.Q.UpdateExpenseCategory(ctx, db.UpdateExpenseCategoryParams{ID: id, Name: name, FinancialGroup: db.FinancialGroup(group), IsActive: active})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ExpenseCategoryView{}, domain.ErrNotFound
		}
		return ExpenseCategoryView{}, err
	}
	return ExpenseCategoryView{ID: r.ID, Name: r.Name, FinancialGroup: string(r.FinancialGroup), IsActive: r.IsActive}, nil
}

// ---- Almacén ----

func (s *BackofficeService) StockLevels(ctx context.Context) ([]db.ListStockLevelsRow, error) {
	return s.store.Q.ListStockLevels(ctx)
}
func (s *BackofficeService) StockMovements(ctx context.Context, limit int32) ([]db.ListStockMovementsRow, error) {
	return s.store.Q.ListStockMovements(ctx, limit)
}

// RecordMovement registra un ajuste/compra/merma manual sobre un ingrediente o producto.
func (s *BackofficeService) RecordMovement(ctx context.Context, itemType string, ingID, prodID *int64, mtype string, qty decimal.Decimal, reason string, userID int64) error {
	// allowNegative: el delta puede restar (merma/ajuste). Round4 porque la columna es
	// numeric(14,4) (base units g/ml); Round2 rechazaría ajustes válidos de sub-centésima.
	// Rechaza 0 y valores que desbordarían la columna.
	q := domain.Round4(qty)
	if !domain.ValidQty(q, domain.MaxStockQty, true) {
		return domain.ErrValidation
	}
	var r *string
	if reason != "" {
		r = &reason
	}
	return s.store.Q.InsertStockMovement(ctx, db.InsertStockMovementParams{
		ItemType:     db.StockItemType(itemType),
		IngredientID: ingID,
		ProductID:    prodID,
		MovementType: db.StockMovementType(mtype),
		Quantity:     q,
		UserID:       &userID,
		Reason:       r,
	})
}

// ---- Reportes ----

func (s *BackofficeService) SalesByDay(ctx context.Context, from, to time.Time) ([]db.SalesByDayRow, error) {
	return s.store.Q.SalesByDay(ctx, db.SalesByDayParams{
		BusinessDate:   pgtype.Date{Time: from, Valid: true},
		BusinessDate_2: pgtype.Date{Time: to, Valid: true},
	})
}
func (s *BackofficeService) SalesByMethod(ctx context.Context, since time.Time) ([]db.SalesByMethodRow, error) {
	return s.store.Q.SalesByMethod(ctx, since)
}
func (s *BackofficeService) ProductMargins(ctx context.Context, since time.Time, limit int32) ([]db.ProductMarginsRow, error) {
	return s.store.Q.ProductMargins(ctx, db.ProductMarginsParams{OpenedAt: since, Limit: limit})
}
