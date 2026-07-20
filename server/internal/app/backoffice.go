package app

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

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
}

func (s *BackofficeService) PaymentMethods(ctx context.Context) ([]PaymentMethodView, error) {
	rows, err := s.store.Q.ListPaymentMethods(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]PaymentMethodView, len(rows))
	for i, r := range rows {
		out[i] = PaymentMethodView{ID: int(r.ID), Name: r.Name, Kind: string(r.Kind), AffectsCashDrawer: r.AffectsCashDrawer}
	}
	return out, nil
}

// ---- Cortes de caja ----

type MethodTotal struct {
	MethodID   int     `json:"methodId"`
	Name       string  `json:"name"`
	Expected   float64 `json:"expected"`
	Declared   float64 `json:"declared"`
	Difference float64 `json:"difference"`
}

type SessionView struct {
	ID          int64         `json:"id"`
	Status      string        `json:"status"`
	OpeningCash float64       `json:"openingCash"`
	OpenedAt    time.Time     `json:"openedAt"`
	Totals      []MethodTotal `json:"totals"`
}

func (s *BackofficeService) OpenSession(ctx context.Context, openingCash float64, userID int64) (*SessionView, error) {
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
	view := &SessionView{ID: sess.ID, Status: string(sess.Status), OpeningCash: sess.OpeningCash, OpenedAt: sess.OpenedAt}
	for _, r := range rows {
		expected := r.Expected
		if r.AffectsCashDrawer {
			expected += sess.OpeningCash
		}
		view.Totals = append(view.Totals, MethodTotal{MethodID: int(r.PaymentMethodID), Name: r.Name, Expected: domain.Round2(expected)})
	}
	return view, nil
}

// CloseSession cierra la caja abierta, guarda esperado vs declarado por método.
func (s *BackofficeService) CloseSession(ctx context.Context, userID int64, declared map[int]float64, notes string) (*SessionView, error) {
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
			t.Declared = domain.Round2(declared[t.MethodID])
			t.Difference = domain.Round2(t.Declared - t.Expected)
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

// ---- Gastos ----

type ExpenseInput struct {
	Date        time.Time
	CategoryID  int64
	SupplierID  *int64
	Amount      float64
	MethodID    *int16
	Description string
	UserID      int64
}

func (s *BackofficeService) CreateExpense(ctx context.Context, in ExpenseInput) (int64, error) {
	// Valida el monto ya redondeado: así un sub-centavo (0.004→0.00) y un valor absurdo
	// (Inf, sobre el tope del numeric(10,2)) se rechazan como 400 en vez de 500.
	amount := domain.Round2(in.Amount)
	if !domain.ValidMoney(amount, false) || in.CategoryID == 0 {
		return 0, domain.ErrValidation
	}
	var desc *string
	if in.Description != "" {
		desc = &in.Description
	}
	return s.store.Q.CreateExpense(ctx, db.CreateExpenseParams{
		ExpenseDate:     pgtype.Date{Time: in.Date, Valid: true},
		CategoryID:      in.CategoryID,
		SupplierID:      in.SupplierID,
		Amount:          amount,
		PaymentMethodID: in.MethodID,
		Description:     desc,
		CreatedBy:       in.UserID,
	})
}

func (s *BackofficeService) ListExpenses(ctx context.Context, limit int32) ([]db.ListExpensesRow, error) {
	return s.store.Q.ListExpenses(ctx, limit)
}
func (s *BackofficeService) ExpenseCategories(ctx context.Context) ([]db.ListExpenseCategoriesRow, error) {
	return s.store.Q.ListExpenseCategories(ctx)
}

// ---- Almacén ----

func (s *BackofficeService) StockLevels(ctx context.Context) ([]db.ListStockLevelsRow, error) {
	return s.store.Q.ListStockLevels(ctx)
}
func (s *BackofficeService) StockMovements(ctx context.Context, limit int32) ([]db.ListStockMovementsRow, error) {
	return s.store.Q.ListStockMovements(ctx, limit)
}

// RecordMovement registra un ajuste/compra/merma manual sobre un ingrediente o producto.
func (s *BackofficeService) RecordMovement(ctx context.Context, itemType string, ingID, prodID *int64, mtype string, qty float64, reason string, userID int64) error {
	// allowNegative: el delta puede restar (merma/ajuste). Round4 porque la columna es
	// numeric(14,4) (base units g/ml); Round2 rechazaría ajustes válidos de sub-centésima.
	// Rechaza 0, Inf/NaN y valores que desbordarían la columna.
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
