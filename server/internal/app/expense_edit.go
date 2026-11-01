package app

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store/db"
)

// Editar un gasto ya guardado (spec 053). Todo pasa por runExpenseEdit: hace el trabajo dentro de
// una transacción, lee el estado de antes y el de después, y con la vista previa revierte (R1). Así
// lo que se previsualiza es, por construcción, lo que se confirma; la huella (R2) detecta que algo
// cambió entre las dos.

// errPreviewRollback revierte la transacción de una vista previa. No sale del servicio.
var errPreviewRollback = errors.New("vista previa: se revierte")

// EditLineInput es un renglón que se agrega a un gasto guardado.
type EditLineInput struct {
	ItemType       string
	IngredientID   *int64
	ProductID      *int64
	Description    string
	Quantity       decimal.Decimal
	PresentationID *int64
	UnitID         *int16
	Amount         decimal.Decimal
	IsGift         bool
	// ArrivedOn: «Ya llegó» ese día (AAAA-MM-DD); vacío = queda por recibir.
	ArrivedOn string
}

// EditHeaderInput son los campos del encabezado que cambian; nil = no cambia.
type EditHeaderInput struct {
	// SupplierSet distingue «quitar el proveedor» (true con SupplierID nil) de «no cambia».
	SupplierSet bool
	SupplierID  *int64
	CategoryID  *int64
	Description *string
	ExpenseDate *string
}

// EditInput es una edición de un gasto: encabezado, renglones agregados y renglones quitados.
type EditInput struct {
	Header      *EditHeaderInput
	Add         []EditLineInput
	Remove      []int64
	Decisions   map[string]string
	Fingerprint string
}

// NeedsReceive: agregar algo que «ya llegó» mueve el almacén y exige también el permiso de recibir
// (R8). El handler lo verifica con el mismo resolvedor que el router.
func (in EditInput) NeedsReceive() bool {
	for _, a := range in.Add {
		if a.ArrivedOn != "" {
			return true
		}
	}
	return false
}

// DeliveryLineInput es lo que llegó de un renglón en una entrega, en su unidad.
type DeliveryLineInput struct {
	ItemID      int64
	Qty         decimal.Decimal
	On          string // avanzado: otra fecha para este renglón
	Shortfall   string // pending | adjust; vacío = la preselección
	ExtraAmount *decimal.Decimal
}

// DeliveryInput es una entrega: una fecha y lo que llegó de cada renglón.
type DeliveryInput struct {
	On          string
	Lines       []DeliveryLineInput
	Decisions   map[string]string
	Fingerprint string
}

// ---- Vista previa ----

// EditPreview son los efectos de una acción, «antes → después» (contracts/edit-expense.md). Todo
// arreglo se entrega aunque esté vacío.
type EditPreview struct {
	Header      []FieldChange  `json:"header"`
	Lines       []LineEffect   `json:"lines"`
	Inventory   []StockEffect  `json:"inventory"`
	Money       MoneyEffect    `json:"money"`
	Costs       []CostEffect   `json:"costs"`
	Decisions   []EditDecision `json:"decisions"`
	Fingerprint string         `json:"fingerprint"`
}

// FieldChange es un campo del encabezado que cambia.
type FieldChange struct {
	Field  string  `json:"field"`
	Before *string `json:"before"`
	After  *string `json:"after"`
}

// LineEffect es lo que le pasa a un renglón: added, removed, received o adjusted.
type LineEffect struct {
	Kind         string           `json:"kind"`
	ItemID       *int64           `json:"itemId"`
	Name         string           `json:"name"`
	Qty          *decimal.Decimal `json:"qty"`
	Amount       *decimal.Decimal `json:"amount"`
	On           *string          `json:"on"`
	MissingAfter *decimal.Decimal `json:"missingAfter"`
	AmountBefore *decimal.Decimal `json:"amountBefore"`
	AmountAfter  *decimal.Decimal `json:"amountAfter"`
}

// StockEffect es lo guardado de un artículo en la sucursal, antes y después.
type StockEffect struct {
	IngredientID  *int64          `json:"ingredientId"`
	ProductID     *int64          `json:"productId"`
	Name          string          `json:"name"`
	BaseUnit      string          `json:"baseUnit"`
	Before        decimal.Decimal `json:"before"`
	After         decimal.Decimal `json:"after"`
	NegativeAfter bool            `json:"negativeAfter"`
	DecisionKey   *string         `json:"decisionKey"`
}

// MoneyEffect es el importe y lo pagado, antes y después. Due = lo que falta pagar; Overpaid = lo
// pagado de más. Los pagos no cambian nunca: Paid es uno solo.
type MoneyEffect struct {
	AmountBefore decimal.Decimal `json:"amountBefore"`
	AmountAfter  decimal.Decimal `json:"amountAfter"`
	Paid         decimal.Decimal `json:"paid"`
	StatusBefore string          `json:"statusBefore"`
	StatusAfter  string          `json:"statusAfter"`
	Due          decimal.Decimal `json:"due"`
	Overpaid     decimal.Decimal `json:"overpaid"`
	DecisionKey  *string         `json:"decisionKey"`
}

// CostEffect es el costo por unidad base de un insumo, antes y después.
type CostEffect struct {
	IngredientID int64           `json:"ingredientId"`
	Name         string          `json:"name"`
	Before       decimal.Decimal `json:"before"`
	After        decimal.Decimal `json:"after"`
}

// EditDecision es una pregunta de la vista previa: Chosen nil = falta decidir.
type EditDecision struct {
	Key       string   `json:"key"`
	Kind      string   `json:"kind"`
	ItemID    *int64   `json:"itemId"`
	CountID   *int64   `json:"countId"`
	CountedOn *string  `json:"countedOn"`
	Options   []string `json:"options"`
	Chosen    *string  `json:"chosen"`
}

// EditResult es lo que responde una edición: la vista previa y, al confirmar, el gasto como quedó.
type EditResult struct {
	Preview EditPreview        `json:"preview"`
	Expense *ExpenseDetailView `json:"expense"`
}

// editRun es el estado de una edición dentro de su transacción.
type editRun struct {
	q        *db.Queries
	exp      db.LockExpenseForEditRow
	userID   int64
	loc      *time.Location
	today    time.Time
	now      time.Time
	removed  decimal.Decimal // importe de los renglones quitados (FR-009)
	adjusted decimal.Decimal // lo que bajó un renglón ajustado a lo recibido
	extra    decimal.Decimal // lo pagado por lo que llegó de más
	moves    []editMove
	ings     map[int64]bool
	prods    map[int64]bool
	lines    []LineEffect
	header   []FieldChange
	pending  []EditDecision // decisiones que no son de conteo ni de dinero (lo que falta)
	audit    map[string]any
}

// editMove es un movimiento de la edición que un conteo posterior pudo haber contado ya.
type editMove struct {
	IngredientID int64
	Qty          decimal.Decimal
	Day          time.Time
	RegisteredAt time.Time
	Description  string
}

func (r *editRun) touch(ing, prod *int64) {
	if ing != nil {
		r.ings[*ing] = true
	}
	if prod != nil {
		r.prods[*prod] = true
	}
}

func sortedKeys(m map[int64]bool) []int64 {
	out := make([]int64, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}

func dayStr(t time.Time) string { return t.Format(dateFmt) }

// stockSnapshot es lo guardado en la sucursal de cada artículo tocado.
type stockKey struct{ ing, prod int64 }

func readStock(ctx context.Context, q *db.Queries, ings, prods []int64) (map[stockKey]decimal.Decimal, error) {
	out := map[stockKey]decimal.Decimal{}
	if len(ings)+len(prods) == 0 {
		return out, nil
	}
	rows, err := q.StockOnHandFor(ctx, db.StockOnHandForParams{IngredientIds: ings, ProductIds: prods})
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		out[stockKey{derefID(r.IngredientID), derefID(r.ProductID)}] = r.OnHand
	}
	return out, nil
}

func readCosts(ctx context.Context, q *db.Queries, ings []int64) (map[int64]db.IngredientCostsForRow, error) {
	out := map[int64]db.IngredientCostsForRow{}
	if len(ings) == 0 {
		return out, nil
	}
	rows, err := q.IngredientCostsFor(ctx, ings)
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		out[r.ID] = r
	}
	return out, nil
}

// runExpenseEdit corre `work` dentro de la transacción de una edición y arma la vista previa. Con
// `preview` revierte todo; al confirmar exige la huella de la última vista previa y las decisiones.
// `ings`/`prods` son los artículos de los renglones nuevos: se toman junto con los del gasto antes de
// leer el «antes» y de escribir nada.
func (s *BackofficeService) runExpenseEdit(ctx context.Context, id, userID int64, preview bool,
	decisions map[string]string, fingerprint string, ings, prods []int64,
	work func(ctx context.Context, r *editRun) error) (EditResult, error) {
	loc := s.businessLocation(ctx)
	now := s.now()
	var out EditPreview
	err := s.store.WithTx(ctx, func(q *db.Queries) error {
		exp, err := q.LockExpenseForEdit(ctx, id)
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrNotFound
		}
		if err != nil {
			return err
		}
		if exp.Status == db.ExpenseStatusCancelada {
			return fmt.Errorf("%w: el gasto está anulado", domain.ErrConflict)
		}
		r := &editRun{q: q, exp: exp, userID: userID, loc: loc, today: domain.BusinessDate(now, loc), now: now,
			ings: map[int64]bool{}, prods: map[int64]bool{}, lines: []LineEffect{}, header: []FieldChange{},
			pending: []EditDecision{}, audit: map[string]any{}}
		current, err := q.ListStockLines(ctx, id)
		if err != nil {
			return err
		}
		for _, l := range current {
			r.touch(l.IngredientID, l.ProductID)
		}
		for i := range ings {
			r.touch(&ings[i], nil)
		}
		for i := range prods {
			r.touch(nil, &prods[i])
		}
		// Los insumos se toman en orden y sin esperar a una venta, ANTES de escribir (spec 038).
		if err := lockIngredients(ctx, q, sortedKeys(r.ings)); err != nil {
			return err
		}
		stockBefore, err := readStock(ctx, q, sortedKeys(r.ings), sortedKeys(r.prods))
		if err != nil {
			return err
		}
		costsBefore, err := readCosts(ctx, q, sortedKeys(r.ings))
		if err != nil {
			return err
		}
		if err := work(ctx, r); err != nil {
			return err
		}
		return s.finishEdit(ctx, r, stockBefore, costsBefore, preview, decisions, fingerprint, &out)
	})
	if err != nil && !errors.Is(err, errPreviewRollback) {
		return EditResult{}, err
	}
	res := EditResult{Preview: out}
	if !preview {
		detail, err := s.ExpenseDetail(ctx, id)
		if err != nil {
			return EditResult{}, err
		}
		res.Expense = &detail
	}
	return res, nil
}

// finishEdit aplica las decisiones de conteo, recalcula importe y estado, arma la vista previa con su
// huella y escribe la bitácora. Con `preview` devuelve errPreviewRollback, que revierte todo.
func (s *BackofficeService) finishEdit(ctx context.Context, r *editRun, stockBefore map[stockKey]decimal.Decimal,
	costsBefore map[int64]db.IngredientCostsForRow, preview bool, decisions map[string]string, fingerprint string,
	out *EditPreview) error {
	q := r.q
	missing := false
	decided := []EditDecision{}

	// Conteo posterior (R5, R6): por insumo, la decisión de quien edita. Sin decisión, la vista
	// previa la pide y lo que se ve es «ajustar» (el movimiento ya está registrado).
	countDecision := map[int64]*EditDecision{}
	for _, m := range r.moves {
		cutoff := domain.CountPosteriorCutoff(m.RegisteredAt, m.Day, r.loc)
		cnt, err := q.LatestPosteriorCount(ctx, db.LatestPosteriorCountParams{IngredientID: m.IngredientID, Cutoff: cutoff})
		if errors.Is(err, pgx.ErrNoRows) {
			continue
		}
		if err != nil {
			return err
		}
		key := "count:" + strconv.FormatInt(m.IngredientID, 10)
		dec, ok := countDecision[m.IngredientID]
		if !ok {
			countedOn := dayStr(domain.BusinessDate(cnt.CapturedAt, r.loc))
			cid := cnt.ID
			dec = &EditDecision{Key: key, Kind: "count", CountID: &cid, CountedOn: &countedOn,
				Options: []string{string(domain.CountAdjust), string(domain.CountKeep)}}
			if v, ok := decisions[key]; ok {
				choice, err := domain.ParseCountDecision(v)
				if err != nil {
					return err
				}
				c := string(choice)
				dec.Chosen = &c
			} else {
				missing = true
			}
			countDecision[m.IngredientID] = dec
		}
		if dec.Chosen != nil && *dec.Chosen == string(domain.CountKeep) {
			ing, exp, uid := m.IngredientID, r.exp.ID, r.userID
			cid := cnt.ID
			reason := fmt.Sprintf("ya contado en el conteo %d: %s", cnt.ID, m.Description)
			if err := q.InsertCompensationMovement(ctx, db.InsertCompensationMovementParams{
				IngredientID: &ing, Quantity: m.Qty.Neg(), ExpenseID: &exp, StockCountID: &cid,
				UserID: &uid, Reason: &reason, BranchID: cnt.BranchID,
				EffectiveOn: pgtype.Date{Time: m.Day, Valid: true},
			}); err != nil {
				return err
			}
		}
	}
	for _, ing := range sortedKeys(r.ings) {
		if d, ok := countDecision[ing]; ok {
			decided = append(decided, *d)
		}
	}

	// Importe y estado (FR-009, FR-011). Los pagos no se tocan.
	items, err := q.ListExpenseItems(ctx, r.exp.ID)
	if err != nil {
		return err
	}
	live := decimal.Zero
	for _, a := range includedAmounts(items) {
		live = live.Add(a)
	}
	amount, err := domain.ExpenseAmountAfterEdit(r.exp.Amount, r.removed, r.adjusted, r.extra, live)
	if err != nil {
		return err
	}
	changed := !amount.Equal(r.exp.Amount)
	statusBefore := string(r.exp.Status)
	status := domain.PaymentStatusAfterEdit(statusBefore, changed, r.exp.Paid, amount, r.exp.PaymentCount > 0)
	if changed || status != statusBefore {
		if err := q.SetExpenseAmountStatus(ctx, db.SetExpenseAmountStatusParams{
			Amount: amount, Status: db.ExpenseStatus(status), UserID: r.userID, ID: r.exp.ID,
		}); err != nil {
			return err
		}
	}
	// Un importe nuevo mueve el tope de lo que salió de la caja (spec 050).
	if err := excludedStillCovers(ctx, q, r.exp.ID); err != nil {
		return err
	}
	money := MoneyEffect{AmountBefore: r.exp.Amount, AmountAfter: amount, Paid: r.exp.Paid,
		StatusBefore: statusBefore, StatusAfter: status, Due: decimal.Zero, Overpaid: decimal.Zero}
	if diff := amount.Sub(r.exp.Paid); diff.IsPositive() {
		money.Due = diff
	} else if diff.IsNegative() && r.exp.PaymentCount > 0 {
		money.Overpaid = diff.Neg()
	}
	if money.Due.IsPositive() && r.exp.PaymentCount > 0 && changed {
		key := "money"
		money.DecisionKey = &key
		dec := EditDecision{Key: key, Kind: "money", Options: []string{string(domain.MoneyPay), string(domain.MoneyLeave)}}
		if v, ok := decisions[key]; ok {
			choice, err := domain.ParseMoneyDecision(v)
			if err != nil {
				return err
			}
			c := string(choice)
			dec.Chosen = &c
		} else {
			missing = true
		}
		decided = append(decided, dec)
	}
	decided = append(decided, r.pending...)

	// El después.
	stockAfter, err := readStock(ctx, q, sortedKeys(r.ings), sortedKeys(r.prods))
	if err != nil {
		return err
	}
	costsAfter, err := readCosts(ctx, q, sortedKeys(r.ings))
	if err != nil {
		return err
	}
	names := map[stockKey][2]string{}
	for _, it := range items {
		k := stockKey{derefID(it.IngredientID), derefID(it.ProductID)}
		name := it.Description
		if n := firstName(it.IngredientName, it.ProductName); n != nil {
			name = *n
		}
		unit := ""
		if it.BaseUnitCode != nil {
			unit = *it.BaseUnitCode
		} else if it.UnitCode != nil {
			unit = *it.UnitCode
		}
		names[k] = [2]string{name, unit}
	}
	inv := []StockEffect{}
	for _, ing := range sortedKeys(r.ings) {
		inv = appendStockEffect(inv, stockKey{ing, 0}, stockBefore, stockAfter, names, countDecision[ing])
	}
	for _, p := range sortedKeys(r.prods) {
		inv = appendStockEffect(inv, stockKey{0, p}, stockBefore, stockAfter, names, nil)
	}
	costs := []CostEffect{}
	movedIng := map[int64]bool{}
	for _, m := range r.moves {
		movedIng[m.IngredientID] = true
	}
	for _, ing := range sortedKeys(r.ings) {
		b, a := costsBefore[ing], costsAfter[ing]
		if !movedIng[ing] && b.CurrentCost.Equal(a.CurrentCost) {
			continue
		}
		costs = append(costs, CostEffect{IngredientID: ing, Name: a.Name, Before: b.CurrentCost, After: a.CurrentCost})
	}

	*out = EditPreview{Header: r.header, Lines: r.lines, Inventory: inv, Money: money, Costs: costs, Decisions: decided}
	out.Fingerprint = domain.PreviewFingerprint(out)

	r.audit["amountBefore"], r.audit["amountAfter"] = r.exp.Amount, amount
	r.audit["statusBefore"], r.audit["statusAfter"] = statusBefore, status
	if len(decided) > 0 {
		r.audit["decisions"] = decided
	}
	if err := auditEntity(ctx, q, r.auditAction(), "expense", r.exp.ID, nil, &r.userID, nil, r.audit); err != nil {
		return err
	}
	if preview {
		return errPreviewRollback
	}
	if fingerprint != out.Fingerprint {
		return domain.ErrExpenseEditStale
	}
	if missing {
		return domain.ErrDecisionRequired
	}
	return nil
}

// auditAction: una entrega es «llegó mercancía»; lo demás, «gasto editado».
func (r *editRun) auditAction() domain.AuditAction {
	if v, ok := r.audit["delivery"].(bool); ok && v {
		return domain.AuditExpenseReceived
	}
	return domain.AuditExpenseEdited
}

func appendStockEffect(inv []StockEffect, k stockKey, before, after map[stockKey]decimal.Decimal,
	names map[stockKey][2]string, dec *EditDecision) []StockEffect {
	b, a := before[k], after[k]
	if b.Equal(a) && dec == nil {
		return inv
	}
	e := StockEffect{Name: names[k][0], BaseUnit: names[k][1], Before: b, After: a, NegativeAfter: a.IsNegative()}
	if k.ing != 0 {
		ing := k.ing
		e.IngredientID = &ing
	} else {
		p := k.prod
		e.ProductID = &p
	}
	if dec != nil {
		key := dec.Key
		e.DecisionKey = &key
	}
	return append(inv, e)
}

// maxExpenseComment es el largo del comentario de un gasto: una cota para que un texto absurdo sea
// un 400 y no un renglón de megas.
const maxExpenseComment = 1000

// parseDay lee un día AAAA-MM-DD; vacío = hoy del negocio.
func (r *editRun) parseDay(v string) (time.Time, error) {
	if v == "" {
		return r.today, nil
	}
	d, err := parseDate(&v)
	if err != nil {
		return time.Time{}, err
	}
	return d.Time, nil
}

// ---- Editar: encabezado, agregar y quitar renglones ----

// EditExpense corrige un gasto ya guardado (spec 053, historias 1, 3 y 4). Con `preview` devuelve los
// efectos sin escribir nada.
func (s *BackofficeService) EditExpense(ctx context.Context, id int64, in EditInput, userID int64, preview bool) (EditResult, error) {
	if in.Header == nil && len(in.Add) == 0 && len(in.Remove) == 0 {
		return EditResult{}, fmt.Errorf("%w: no hay nada que cambiar", domain.ErrValidation)
	}
	if len(in.Add)+len(in.Remove) > maxExpenseItems {
		return EditResult{}, domain.ErrValidation
	}
	var ings, prods []int64
	for _, a := range in.Add {
		if a.IngredientID != nil {
			ings = append(ings, *a.IngredientID)
		}
		if a.ProductID != nil {
			prods = append(prods, *a.ProductID)
		}
	}
	return s.runExpenseEdit(ctx, id, userID, preview, in.Decisions, in.Fingerprint, ings, prods,
		func(ctx context.Context, r *editRun) error {
			if in.Header != nil {
				if err := r.editHeader(ctx, *in.Header); err != nil {
					return err
				}
			}
			if err := r.removeLines(ctx, in.Remove); err != nil {
				return err
			}
			perLine, err := r.addLines(ctx, in.Add)
			if err != nil {
				return err
			}
			moves, err := s.applyStockLinesOn(ctx, r.q, r.exp.ID, r.userID, nil, perLine)
			if err != nil {
				return err
			}
			r.recordMoves(moves, r.now)
			return nil
		})
}

func (r *editRun) recordMoves(moves []appliedMove, at time.Time) {
	for _, m := range moves {
		if m.IngredientID != nil {
			r.moves = append(r.moves, editMove{IngredientID: *m.IngredientID, Qty: m.Qty, Day: m.Day,
				RegisteredAt: at, Description: m.Description})
		}
	}
}

func (r *editRun) editHeader(ctx context.Context, h EditHeaderInput) error {
	q := r.q
	exp := r.exp
	nameOfSupplier := func(id *int64) (*string, error) {
		if id == nil {
			return nil, nil
		}
		n, err := q.SupplierName(ctx, *id)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("%w: el proveedor no existe", domain.ErrValidation)
		}
		return &n, err
	}
	nameOfCategory := func(id int64) (*string, error) {
		n, err := q.ExpenseCategoryName(ctx, id)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("%w: la categoría no existe", domain.ErrValidation)
		}
		return &n, err
	}
	supplier, category, desc, date := exp.SupplierID, exp.CategoryID, exp.Description, exp.ExpenseDate.Time
	changes := map[string]any{}
	if h.SupplierSet && derefID(h.SupplierID) != derefID(exp.SupplierID) {
		before, err := nameOfSupplier(exp.SupplierID)
		if err != nil {
			return err
		}
		after, err := nameOfSupplier(h.SupplierID)
		if err != nil {
			return err
		}
		supplier = h.SupplierID
		r.header = append(r.header, FieldChange{Field: "supplier", Before: before, After: after})
		changes["supplier"] = map[string]any{"before": exp.SupplierID, "after": h.SupplierID}
	}
	if h.CategoryID != nil && *h.CategoryID != exp.CategoryID {
		before, err := nameOfCategory(exp.CategoryID)
		if err != nil {
			return err
		}
		after, err := nameOfCategory(*h.CategoryID)
		if err != nil {
			return err
		}
		category = *h.CategoryID
		r.header = append(r.header, FieldChange{Field: "category", Before: before, After: after})
		changes["category"] = map[string]any{"before": exp.CategoryID, "after": *h.CategoryID}
	}
	if h.Description != nil {
		v := *h.Description
		if len([]rune(v)) > maxExpenseComment {
			return fmt.Errorf("%w: el comentario es demasiado largo", domain.ErrValidation)
		}
		var after *string
		if v != "" {
			after = &v
		}
		if derefStr(after) != derefStr(exp.Description) {
			desc = after
			r.header = append(r.header, FieldChange{Field: "description", Before: exp.Description, After: after})
			changes["description"] = map[string]any{"before": exp.Description, "after": after}
		}
	}
	if h.ExpenseDate != nil {
		d, err := r.parseDay(*h.ExpenseDate)
		if err != nil {
			return err
		}
		if d.After(r.today) {
			return fmt.Errorf("%w: la fecha del gasto no puede ser de un día que no ha pasado", domain.ErrValidation)
		}
		if !d.Equal(date) {
			b, a := dayStr(date), dayStr(d)
			r.header = append(r.header, FieldChange{Field: "expenseDate", Before: &b, After: &a})
			changes["expenseDate"] = map[string]any{"before": b, "after": a}
			date = d
		}
	}
	if len(changes) == 0 {
		return nil
	}
	r.audit["header"] = changes
	r.exp.ExpenseDate = pgtype.Date{Time: date, Valid: true}
	return q.UpdateExpenseHeader(ctx, db.UpdateExpenseHeaderParams{
		SupplierID: supplier, CategoryID: category, Description: desc,
		ExpenseDate: pgtype.Date{Time: date, Valid: true}, ID: exp.ID,
	})
}

// removeLines quita renglones: lo que no llegó no mueve nada; lo que llegó sale con un
// contra-movimiento por cada día de entrega, con esa fecha (R7).
func (r *editRun) removeLines(ctx context.Context, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	items, err := r.q.ListExpenseItems(ctx, r.exp.ID)
	if err != nil {
		return err
	}
	byID := make(map[int64]db.ListExpenseItemsRow, len(items))
	for _, it := range items {
		byID[it.ID] = it
	}
	seen := map[int64]bool{}
	removed := []map[string]any{}
	for _, id := range ids {
		it, ok := byID[id]
		if !ok || it.VoidedAt.Valid || seen[id] {
			return domain.ErrNotFound
		}
		seen[id] = true
		if it.Excluded {
			return fmt.Errorf("%w: un renglón que no es del negocio no se quita; vuelve a incluirlo primero", domain.ErrValidation)
		}
		if it.ItemType != nil && !it.AppliedQtyBase.IsZero() {
			if err := r.reverseDeliveries(ctx, it); err != nil {
				return err
			}
		}
		if _, err := r.q.VoidStockLine(ctx, db.VoidStockLineParams{ID: id, ExpenseID: r.exp.ID}); err != nil {
			return err
		}
		r.removed = r.removed.Add(it.Amount)
		name := it.Description
		if n := firstName(it.IngredientName, it.ProductName); n != nil {
			name = *n
		}
		itemID, amount := it.ID, it.Amount
		r.lines = append(r.lines, LineEffect{Kind: "removed", ItemID: &itemID, Name: name, Amount: &amount})
		removed = append(removed, map[string]any{"itemId": it.ID, "description": it.Description, "amount": it.Amount,
			"applied": it.AppliedQtyBase})
	}
	r.audit["removed"] = removed
	return nil
}

func (r *editRun) reverseDeliveries(ctx context.Context, it db.ListExpenseItemsRow) error {
	itemID := it.ID
	rows, err := r.q.LineDeliveryDays(ctx, &itemID)
	if err != nil {
		return err
	}
	days := make([]domain.DeliveryDay, 0, len(rows))
	for _, d := range rows {
		day := r.today
		if d.Day.Valid {
			day = d.Day.Time
		}
		days = append(days, domain.DeliveryDay{Day: day, Qty: d.Qty, RegisteredAt: d.RegisteredAt})
	}
	plan := domain.PlanDeliveryReversal(days)
	// Lo aplicado es la suma de los movimientos del renglón; si un renglón viejo trae lo aplicado sin
	// sus movimientos ligados, lo que falte sale con fecha de hoy para que el renglón quede en cero.
	left := it.AppliedQtyBase
	for _, p := range plan {
		left = left.Add(p.Qty)
	}
	if !left.IsZero() {
		plan = append(plan, domain.DeliveryDay{Day: r.today, Qty: left.Neg(), RegisteredAt: r.now})
	}
	expenseID, uid := r.exp.ID, r.userID
	reason := "renglón quitado: " + it.Description
	for _, p := range plan {
		if err := r.q.InsertLineMovement(ctx, db.InsertLineMovementParams{
			ItemType: *it.ItemType, IngredientID: it.IngredientID, ProductID: it.ProductID,
			Quantity: p.Qty, ExpenseID: &expenseID, ExpenseItemID: &itemID, UserID: &uid, Reason: &reason,
			EffectiveOn: pgtype.Date{Time: p.Day, Valid: true},
		}); err != nil {
			return err
		}
		if err := r.q.AddAppliedQty(ctx, db.AddAppliedQtyParams{ID: itemID, Delta: p.Qty}); err != nil {
			return err
		}
		if it.IngredientID != nil {
			r.moves = append(r.moves, editMove{IngredientID: *it.IngredientID, Qty: p.Qty, Day: p.Day,
				RegisteredAt: p.RegisteredAt, Description: it.Description})
		}
	}
	return nil
}

// addLines agrega renglones; los que «ya llegaron» devuelven su día para applyStockLinesOn.
func (r *editRun) addLines(ctx context.Context, lines []EditLineInput) (map[int64]time.Time, error) {
	perLine := map[int64]time.Time{}
	added := []map[string]any{}
	for _, l := range lines {
		amount := domain.Round2(l.Amount)
		qty := domain.Round4(l.Quantity)
		if l.Description == "" || !qty.IsPositive() || !domain.ValidQty(qty, domain.MaxStockQty, false) {
			return nil, domain.ErrValidation
		}
		if err := checkLineMoney(amount, l.IsGift); err != nil {
			return nil, err
		}
		itemType, err := itemTypeParam(ExpenseItemInput{ItemType: l.ItemType, IngredientID: l.IngredientID, ProductID: l.ProductID})
		if err != nil {
			return nil, err
		}
		var unit *int16
		var pack *decimal.Decimal
		if itemType != nil {
			if unit, pack, err = lineUnit(ctx, r.q, l.IngredientID, l.PresentationID, l.UnitID, nil); err != nil {
				return nil, err
			}
			if unit == nil {
				return nil, domain.ErrValidation
			}
		} else if l.IsGift || l.PresentationID != nil || l.ArrivedOn != "" {
			// Un renglón sin artículo no se regala ni «llega»: no hay nada que guardar.
			return nil, domain.ErrValidation
		}
		if !domain.ValidLineUnitCost(domain.UnitCost(amount, qty)) {
			return nil, fmt.Errorf("%w: el importe por unidad es demasiado grande", domain.ErrValidation)
		}
		var recv *decimal.Decimal
		var on *string
		var arrived *time.Time
		if l.ArrivedOn != "" {
			day, err := r.parseDay(l.ArrivedOn)
			if err != nil {
				return nil, err
			}
			if err := domain.ValidDeliveryDate(day, r.exp.ExpenseDate.Time, r.today); err != nil {
				return nil, err
			}
			recv = &qty
			s := dayStr(day)
			on, arrived = &s, &day
		}
		pos, err := r.q.NextItemPosition(ctx, r.exp.ID)
		if err != nil {
			return nil, err
		}
		newID, err := r.q.CreateExpenseItem(ctx, db.CreateExpenseItemParams{
			ExpenseID: r.exp.ID, ItemType: itemType, IngredientID: l.IngredientID, ProductID: l.ProductID,
			Description: l.Description, Quantity: qty, UnitID: unit, QtyReceived: recv,
			UnitCost: domain.UnitCost(amount, qty), Amount: amount, PackQtyInBase: pack,
			Position: pos, PresentationID: l.PresentationID, IsGift: l.IsGift,
		})
		if err != nil {
			return nil, err
		}
		if arrived != nil {
			perLine[newID] = *arrived
		}
		// Sin id: la vista previa revertida consume uno distinto del de la confirmación, y la huella
		// no coincidiría nunca.
		r.lines = append(r.lines, LineEffect{Kind: "added", Name: l.Description, Qty: &qty, Amount: &amount, On: on})
		added = append(added, map[string]any{"itemId": newID, "description": l.Description, "quantity": qty,
			"amount": amount, "gift": l.IsGift, "arrivedOn": on})
	}
	if len(added) > 0 {
		r.audit["added"] = added
	}
	return perLine, nil
}

// ---- Entregas ----

// DeliverExpense registra una entrega de uno o varios renglones con su fecha (spec 053, historia 2).
// Con `preview` devuelve los efectos sin escribir nada.
func (s *BackofficeService) DeliverExpense(ctx context.Context, id int64, in DeliveryInput, userID int64, preview bool) (EditResult, error) {
	if len(in.Lines) == 0 || len(in.Lines) > maxExpenseItems {
		return EditResult{}, fmt.Errorf("%w: no llegó nada", domain.ErrValidation)
	}
	seen := map[int64]bool{}
	for _, l := range in.Lines {
		if seen[l.ItemID] {
			return EditResult{}, fmt.Errorf("%w: un renglón viene dos veces", domain.ErrValidation)
		}
		seen[l.ItemID] = true
		if q := domain.Round4(l.Qty); !q.IsPositive() || !domain.ValidQty(q, domain.MaxStockQty, false) {
			return EditResult{}, domain.ErrValidation
		}
		if l.Shortfall != "" {
			if _, err := domain.ParseShortfall(l.Shortfall); err != nil {
				return EditResult{}, err
			}
		}
	}
	return s.runExpenseEdit(ctx, id, userID, preview, in.Decisions, in.Fingerprint, nil, nil,
		func(ctx context.Context, r *editRun) error {
			def, err := r.parseDay(in.On)
			if err != nil {
				return err
			}
			stock, err := r.q.ListStockLines(ctx, r.exp.ID)
			if err != nil {
				return err
			}
			byID := make(map[int64]db.ListStockLinesRow, len(stock))
			for _, l := range stock {
				byID[l.ID] = l
			}
			perLine := map[int64]time.Time{}
			got := []map[string]any{}
			for _, l := range in.Lines {
				day := def
				if l.On != "" {
					if day, err = r.parseDay(l.On); err != nil {
						return err
					}
				}
				if err := domain.ValidDeliveryDate(day, r.exp.ExpenseDate.Time, r.today); err != nil {
					return err
				}
				info, ok := byID[l.ItemID]
				if !ok || info.Excluded {
					return domain.ErrNotFound
				}
				detail, err := r.deliverLine(ctx, l, info, day)
				if err != nil {
					return err
				}
				perLine[l.ItemID] = day
				got = append(got, detail)
			}
			moves, err := s.applyStockLinesOn(ctx, r.q, r.exp.ID, r.userID, &def, perLine)
			if err != nil {
				return err
			}
			r.recordMoves(moves, r.now)
			r.audit["delivery"] = true
			r.audit["lines"] = got
			return nil
		})
}

// deliverLine sube lo que llegó de un renglón. De más: sube lo pedido y el importe por lo que se
// pagó (regla de la 038). De menos: lo que falta sigue pendiente o el renglón se ajusta a lo recibido.
func (r *editRun) deliverLine(ctx context.Context, l DeliveryLineInput, info db.ListStockLinesRow, day time.Time) (map[string]any, error) {
	line, err := r.q.LockStockLine(ctx, db.LockStockLineParams{ID: l.ItemID, ExpenseID: r.exp.ID})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	qty := domain.Round4(l.Qty)
	missing := missingOf(line)
	quantity, amount := line.Quantity, line.Amount
	extra := decimal.Zero
	if over := qty.Sub(missing); over.IsPositive() {
		if line.IsGift {
			quantity = quantity.Add(over) // un regalo de más sigue siendo regalo
		} else {
			if l.ExtraAmount == nil {
				return nil, fmt.Errorf("%w: llegó más de lo que faltaba; ¿cuánto se pagó por lo de más?", domain.ErrValidation)
			}
			extra = domain.Round2(*l.ExtraAmount)
			if !extra.IsPositive() || !domain.ValidMoney(extra, false) {
				return nil, domain.ErrValidation
			}
			quantity, amount = quantity.Add(over), amount.Add(extra)
		}
	}
	received := qty
	if line.QtyReceived != nil {
		received = received.Add(*line.QtyReceived)
	}
	name := info.Description
	itemID := l.ItemID
	on := dayStr(day)
	effect := LineEffect{Kind: "received", ItemID: &itemID, Name: name, Qty: &qty, On: &on}
	detail := map[string]any{"itemId": l.ItemID, "qty": qty, "on": on, "extraAmount": extra}

	left := quantity.Sub(received).Sub(line.NotComingQty)
	if left.IsPositive() {
		choice := domain.ShortfallDefault(string(info.BuyKind), info.PresentationID != nil)
		if l.Shortfall != "" {
			choice, _ = domain.ParseShortfall(l.Shortfall) // ya validado
		}
		c := string(choice)
		r.pending = append(r.pending, EditDecision{Key: "shortfall:" + strconv.FormatInt(l.ItemID, 10), Kind: "shortfall",
			ItemID: &itemID, Options: []string{string(domain.ShortfallPending), string(domain.ShortfallAdjust)}, Chosen: &c})
		detail["shortfall"] = c
		if choice == domain.ShortfallAdjust {
			newQty, newAmount := domain.AdjustLineToReceived(quantity, received, amount)
			if line.IsGift {
				newAmount = decimal.Zero
			}
			if !newAmount.IsPositive() && !line.IsGift {
				return nil, fmt.Errorf("%w: el renglón quedaría en $0; quítalo en vez de ajustarlo", domain.ErrValidation)
			}
			if err := r.q.AdjustLineToReceived(ctx, db.AdjustLineToReceivedParams{
				Qty: &newQty, Amount: newAmount, UnitCost: domain.UnitCost(newAmount, newQty), ID: l.ItemID,
			}); err != nil {
				return nil, err
			}
			r.adjusted = r.adjusted.Add(amount.Sub(newAmount))
			r.extra = r.extra.Add(extra)
			before, after := amount, newAmount
			r.lines = append(r.lines, effect, LineEffect{Kind: "adjusted", ItemID: &itemID, Name: name,
				AmountBefore: &before, AmountAfter: &after})
			return detail, nil
		}
		effect.MissingAfter = &left
	}
	if !domain.ValidLineUnitCost(domain.UnitCost(amount, quantity)) {
		return nil, fmt.Errorf("%w: el importe por unidad es demasiado grande", domain.ErrValidation)
	}
	if err := r.q.SetLineArrival(ctx, db.SetLineArrivalParams{
		ID: l.ItemID, QtyReceived: &received, Quantity: quantity, Amount: amount, UnitCost: domain.UnitCost(amount, quantity),
	}); err != nil {
		return nil, err
	}
	r.extra = r.extra.Add(extra)
	r.lines = append(r.lines, effect)
	return detail, nil
}
