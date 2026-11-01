package app

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store/db"
)

// «Por recibir» (spec 038, historia 1b): lo pagado que no ha llegado se recibe por partes, de más
// con su importe, o se cierra con «ya no va a llegar». Todo pasa por applyStockLines: el almacén se
// mueve por la diferencia contra lo que el renglón ya sumó.

// PendingReceipt es un renglón que falta por llegar. Las cantidades van en la unidad del renglón
// (el empaque o la unidad); `PackQtyInBase` y `BaseUnitCode` dicen cuánto es en lo que se guarda.
type PendingReceipt struct {
	ExpenseID        int64            `json:"expenseId"`
	ExpenseDate      string           `json:"expenseDate"`
	Supplier         *string          `json:"supplier"`
	ExpenseAmount    decimal.Decimal  `json:"expenseAmount"`
	Paid             bool             `json:"paid"`
	ItemID           int64            `json:"itemId"`
	ItemName         string           `json:"itemName"`
	Quantity         decimal.Decimal  `json:"quantity"`
	Received         decimal.Decimal  `json:"received"`
	NotComing        decimal.Decimal  `json:"notComing"`
	Missing          decimal.Decimal  `json:"missing"`
	Applied          decimal.Decimal  `json:"applied"`
	PackQtyInBase    *decimal.Decimal `json:"packQtyInBase"`
	PresentationName *string          `json:"presentationName"`
	UnitCode         *string          `json:"unitCode"`
	BaseUnitCode     *string          `json:"baseUnitCode"`
}

// PendingReceipts lista lo que falta por llegar. Siempre un arreglo, aunque vacío.
func (s *BackofficeService) PendingReceipts(ctx context.Context) ([]PendingReceipt, error) {
	rows, err := s.store.QC(ctx).PendingReceipts(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]PendingReceipt, len(rows))
	for i, r := range rows {
		out[i] = PendingReceipt{
			ExpenseID: r.ExpenseID, ExpenseDate: r.ExpenseDate.Time.Format(dateFmt), Supplier: r.Supplier,
			ExpenseAmount: r.ExpenseAmount, Paid: r.Paid, ItemID: r.ItemID, ItemName: r.ItemName,
			Quantity: r.Quantity, Received: r.Received, NotComing: r.NotComingQty, Missing: r.Missing,
			Applied: r.AppliedQtyBase, PackQtyInBase: r.PackQtyInBase, PresentationName: r.PresentationName,
			UnitCode: r.UnitCode, BaseUnitCode: r.BaseUnitCode,
		}
	}
	return out, nil
}

// ReceiveItemInput es lo que llegó de un renglón, en su unidad. SeenApplied es lo aplicado que vio
// la pantalla; ExtraAmount es obligatorio si llegó más de lo que faltaba.
type ReceiveItemInput struct {
	Qty         decimal.Decimal
	SeenApplied decimal.Decimal
	ExtraAmount *decimal.Decimal
}

// lockedLine abre la transacción de una escritura sobre un renglón: el gasto, sus insumos y el
// renglón, en ese orden y sin esperar a una venta.
func (s *BackofficeService) lockedLine(ctx context.Context, q *db.Queries, expenseID, itemID int64) (db.LockStockLineRow, error) {
	exp, err := q.LockExpenseForStock(ctx, expenseID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return db.LockStockLineRow{}, domain.ErrNotFound
		}
		return db.LockStockLineRow{}, err
	}
	if exp.Status == db.ExpenseStatusCancelada {
		return db.LockStockLineRow{}, fmt.Errorf("%w: la compra está anulada", domain.ErrConflict)
	}
	rows, err := q.ListStockLines(ctx, expenseID)
	if err != nil {
		return db.LockStockLineRow{}, err
	}
	var ids []int64
	for _, r := range rows {
		if r.IngredientID != nil {
			ids = append(ids, *r.IngredientID)
		}
	}
	if err := lockIngredients(ctx, q, ids); err != nil {
		return db.LockStockLineRow{}, err
	}
	line, err := q.LockStockLine(ctx, db.LockStockLineParams{ID: itemID, ExpenseID: expenseID})
	if errors.Is(err, pgx.ErrNoRows) {
		return line, domain.ErrNotFound
	}
	return line, err
}

func missingOf(l db.LockStockLineRow) decimal.Decimal {
	got := decimal.Zero
	if l.QtyReceived != nil {
		got = *l.QtyReceived
	}
	m := l.Quantity.Sub(got).Sub(l.NotComingQty)
	if m.IsNegative() {
		return decimal.Zero
	}
	return m
}

// raiseTotal sube el total de la compra por lo que llegó de más o «Llegó algo más» pagado. Cada
// importe se valida solo, así que el total que resulta se valida aquí, antes de escribir: sin esto,
// varios importes válidos llevaban la columna a desbordarse (500).
func raiseTotal(ctx context.Context, q *db.Queries, expenseID int64, extra decimal.Decimal) error {
	if !extra.IsPositive() {
		return nil
	}
	exp, err := q.LockExpenseForStock(ctx, expenseID)
	if err != nil {
		return err
	}
	if !domain.ValidMoney(exp.Amount.Add(extra), false) {
		return fmt.Errorf("%w: el total de la compra pasaría del máximo", domain.ErrValidation)
	}
	return q.RaiseExpenseAmount(ctx, db.RaiseExpenseAmountParams{ID: expenseID, Extra: extra})
}

// ReceiveItem suma lo que llegó de un renglón. Si llegó más de lo que faltaba, lo de más sube lo
// pedido, el importe del renglón y el total de la compra por `ExtraAmount`.
func (s *BackofficeService) ReceiveItem(ctx context.Context, expenseID, itemID int64, in ReceiveItemInput, userID int64) error {
	qty := domain.Round4(in.Qty)
	if !qty.IsPositive() || !domain.ValidQty(qty, domain.MaxStockQty, false) {
		return domain.ErrValidation
	}
	day, err := s.parseDayOrToday(ctx, "")
	if err != nil {
		return err
	}
	return s.store.WithTx(ctx, func(q *db.Queries) error {
		line, err := s.lockedLine(ctx, q, expenseID, itemID)
		if err != nil {
			return err
		}
		if !domain.ValidStockQtyOrZero(in.SeenApplied) {
			return domain.ErrValidation
		}
		if !domain.Round4(in.SeenApplied).Equal(line.AppliedQtyBase) {
			return fmt.Errorf("%w: alguien más ya recibió en esta compra; vuelve a abrirla", domain.ErrConflict)
		}
		missing := missingOf(line)
		quantity, amount := line.Quantity, line.Amount
		extra := decimal.Zero
		if over := qty.Sub(missing); over.IsPositive() {
			if line.IsGift {
				quantity = quantity.Add(over) // un regalo de más sigue siendo regalo
			} else {
				if in.ExtraAmount == nil {
					return fmt.Errorf("%w: llegó más de lo que faltaba; ¿cuánto se pagó por lo de más?", domain.ErrValidation)
				}
				extra = domain.Round2(*in.ExtraAmount)
				if !extra.IsPositive() || !domain.ValidMoney(extra, false) {
					return domain.ErrValidation
				}
				quantity, amount = quantity.Add(over), amount.Add(extra)
			}
		}
		got := qty
		if line.QtyReceived != nil {
			got = got.Add(*line.QtyReceived)
		}
		if err := raiseTotal(ctx, q, expenseID, extra); err != nil {
			return err
		}
		if !domain.ValidLineUnitCost(domain.UnitCost(amount, quantity)) {
			return fmt.Errorf("%w: el importe por unidad es demasiado grande", domain.ErrValidation)
		}
		if err := q.SetLineArrival(ctx, db.SetLineArrivalParams{
			ID: itemID, QtyReceived: &got, Quantity: quantity, Amount: amount, UnitCost: domain.UnitCost(amount, quantity),
		}); err != nil {
			return err
		}
		if _, err := s.applyStockLines(ctx, q, expenseID, userID, &day); err != nil {
			return err
		}
		return auditEntity(ctx, q, domain.AuditExpenseReceived, "expense", expenseID, nil, &userID, nil,
			map[string]any{"itemId": itemID, "qty": qty, "extraAmount": extra})
	})
}

// MarkNotComing cierra una parte de lo que falta de un renglón: no suma ni cambia el total. Si ya
// no queda nada pendiente en la compra, queda recibida.
func (s *BackofficeService) MarkNotComing(ctx context.Context, expenseID, itemID int64, qty decimal.Decimal, userID int64) error {
	qty = domain.Round4(qty)
	if !qty.IsPositive() || !domain.ValidQty(qty, domain.MaxStockQty, false) {
		return domain.ErrValidation
	}
	day, err := s.parseDayOrToday(ctx, "")
	if err != nil {
		return err
	}
	return s.store.WithTx(ctx, func(q *db.Queries) error {
		line, err := s.lockedLine(ctx, q, expenseID, itemID)
		if err != nil {
			return err
		}
		if qty.GreaterThan(missingOf(line)) {
			return fmt.Errorf("%w: no faltan tantos", domain.ErrValidation)
		}
		if err := q.AddNotComing(ctx, db.AddNotComingParams{ID: itemID, Qty: qty}); err != nil {
			return err
		}
		if _, err := s.applyStockLines(ctx, q, expenseID, userID, &day); err != nil {
			return err
		}
		return auditEntity(ctx, q, domain.AuditExpenseStockCorrected, "expense", expenseID, nil, &userID, nil,
			map[string]any{"itemId": itemID, "notComing": qty})
	})
}

// ArrivedItemInput es «Llegó algo más»: un renglón que no venía en la compra y ya llegó.
type ArrivedItemInput struct {
	ItemType       string
	IngredientID   *int64
	ProductID      *int64
	Description    string
	Quantity       decimal.Decimal
	PresentationID *int64
	UnitID         *int16
	Amount         decimal.Decimal
	IsGift         bool
}

// AddArrivedItem agrega lo que llegó de más a una compra. Pagado, sube el total por su importe; un
// regalo va en 0, no cambia el total ni el costo.
func (s *BackofficeService) AddArrivedItem(ctx context.Context, expenseID int64, in ArrivedItemInput, userID int64) error {
	amount := domain.Round2(in.Amount)
	if err := checkLineMoney(amount, in.IsGift); err != nil {
		return err
	}
	qty := domain.Round4(in.Quantity)
	if !qty.IsPositive() || !domain.ValidQty(qty, domain.MaxStockQty, false) || in.Description == "" {
		return domain.ErrValidation
	}
	itemType, err := itemTypeParam(ExpenseItemInput{ItemType: in.ItemType, IngredientID: in.IngredientID, ProductID: in.ProductID})
	if err != nil || itemType == nil {
		return domain.ErrValidation
	}
	day, err := s.parseDayOrToday(ctx, "")
	if err != nil {
		return err
	}
	return s.store.WithTx(ctx, func(q *db.Queries) error {
		exp, err := q.LockExpenseForStock(ctx, expenseID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return domain.ErrNotFound
			}
			return err
		}
		if exp.Status == db.ExpenseStatusCancelada {
			return fmt.Errorf("%w: la compra está anulada", domain.ErrConflict)
		}
		rows, err := q.ListStockLines(ctx, expenseID)
		if err != nil {
			return err
		}
		var ids []int64
		for _, r := range rows {
			if r.IngredientID != nil {
				ids = append(ids, *r.IngredientID)
			}
		}
		if in.IngredientID != nil {
			ids = append(ids, *in.IngredientID)
		}
		if err := lockIngredients(ctx, q, ids); err != nil {
			return err
		}
		unit, pack, err := lineUnit(ctx, q, in.IngredientID, in.PresentationID, in.UnitID, nil)
		if err != nil {
			return err
		}
		if unit == nil {
			return domain.ErrValidation
		}
		if err := raiseTotal(ctx, q, expenseID, amount); err != nil {
			return err
		}
		if !domain.ValidLineUnitCost(domain.UnitCost(amount, qty)) {
			return fmt.Errorf("%w: el importe por unidad es demasiado grande", domain.ErrValidation)
		}
		pos, err := q.NextItemPosition(ctx, expenseID)
		if err != nil {
			return err
		}
		if _, err := q.CreateExpenseItem(ctx, db.CreateExpenseItemParams{
			ExpenseID: expenseID, ItemType: itemType, IngredientID: in.IngredientID, ProductID: in.ProductID,
			Description: in.Description, Quantity: qty, UnitID: unit, QtyReceived: &qty,
			UnitCost: domain.UnitCost(amount, qty), Amount: amount, PackQtyInBase: pack,
			Position: pos, PresentationID: in.PresentationID, IsGift: in.IsGift,
		}); err != nil {
			return err
		}
		if _, err := s.applyStockLines(ctx, q, expenseID, userID, &day); err != nil {
			return err
		}
		return auditEntity(ctx, q, domain.AuditExpenseReceived, "expense", expenseID, nil, &userID, nil,
			map[string]any{"added": in.Description, "amount": amount, "gift": in.IsGift})
	})
}
