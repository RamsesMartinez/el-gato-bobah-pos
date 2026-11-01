package app

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store/db"
)

// Compras que suman al almacén (spec 038).
//
// Lo que un renglón ya sumó vive en `applied_qty_base` y es la suma de sus movimientos. Toda
// escritura —recibir, corregir, quitar, anular— calcula lo que el renglón DEBERÍA haber sumado,
// inserta solo la diferencia y deja el historial intacto: nunca se edita ni se borra un movimiento.

// StockLineInput es un renglón de almacén tal como lo deja la pantalla de «Corregir lo que llegó».
type StockLineInput struct {
	// ID nulo: renglón nuevo. Con ID, SeenApplied es lo aplicado que vio la pantalla; si ya no
	// coincide, otra tableta corrigió primero y se responde 409 en vez de pisarla.
	ID          *int64
	SeenApplied *decimal.Decimal

	ItemType     string // ingrediente | producto
	IngredientID *int64
	ProductID    *int64
	Description  string
	// Quantity: lo pedido. Received: lo que llegó (0 = llega después). NotComing: lo que ya no va
	// a llegar. Las tres en la unidad del renglón (el empaque o la unidad elegida).
	Quantity       decimal.Decimal
	Received       decimal.Decimal
	NotComing      decimal.Decimal
	PresentationID *int64
	UnitID         *int16
	Amount         decimal.Decimal
	IsGift         bool
	// Excluded solo cuenta en un renglón nuevo; uno que ya existe conserva su marca, que se cambia
	// con SetItemExcluded (R5).
	Excluded bool
}

// lineUnit resuelve el empaque elegido: el renglón se guarda en «piezas de compra» con el contenido
// del empaque COPIADO, así que editar el empaque después no reescribe esta compra (FR-009).
func lineUnit(ctx context.Context, q *db.Queries, ingredientID *int64, presentationID *int64, unitID *int16, pack *decimal.Decimal) (*int16, *decimal.Decimal, error) {
	if presentationID == nil {
		return unitID, pack, nil
	}
	p, err := q.GetPresentationForLine(ctx, *presentationID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil, fmt.Errorf("%w: ese empaque ya no existe o ya no llega así", domain.ErrValidation)
		}
		return nil, nil, err
	}
	if ingredientID == nil || p.IngredientID != *ingredientID {
		return nil, nil, fmt.Errorf("%w: el empaque es de otro insumo", domain.ErrValidation)
	}
	piece, err := q.PieceUnitID(ctx)
	if err != nil {
		return nil, nil, err
	}
	qty := p.QtyInBase
	return &piece, &qty, nil
}

// checkLineMoney: cada renglón lleva importe mayor que cero, salvo un regalo, que va en 0 (FR-004a,
// FR-007d). El check de la 0092 lo repite como defensa.
func checkLineMoney(amount decimal.Decimal, gift bool) error {
	if gift {
		if !amount.IsZero() {
			return fmt.Errorf("%w: un regalo va con importe 0", domain.ErrValidation)
		}
		return nil
	}
	if !amount.IsPositive() || !domain.ValidMoney(amount, false) {
		return fmt.Errorf("%w: cada renglón lleva su importe", domain.ErrValidation)
	}
	return nil
}

// checkLinesFitTotal: la suma de los renglones no pasa del total pagado; lo que falta queda como
// «otros» del gasto (FR-004a).
func checkLinesFitTotal(lines []decimal.Decimal, total decimal.Decimal) error {
	sum := decimal.Zero
	for _, a := range lines {
		sum = sum.Add(a)
	}
	if domain.Round2(sum).GreaterThan(domain.Round2(total)) {
		return fmt.Errorf("%w: los renglones suman $%s y el total pagado es $%s", domain.ErrValidation, domain.Round2(sum), domain.Round2(total))
	}
	return nil
}

// lockErr traduce el «no esperar» de los bloqueos: una venta tiene tomado ese insumo y la compra se
// reintenta, en vez de esperar y arriesgar un ciclo que Postgres resolvería cancelando una.
func lockErr(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "55P03" {
		return fmt.Errorf("%w: hay una venta en curso con uno de esos insumos; vuelve a intentar en un momento", domain.ErrConflict)
	}
	return err
}

// lockIngredients toma sin esperar los insumos que una escritura de compra va a tocar. Va primero
// en la transacción: después, la llave foránea de cada renglón y el recálculo del costo ya los
// tienen y nada espera.
func lockIngredients(ctx context.Context, q *db.Queries, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	got, err := q.LockIngredientsNowait(ctx, ids)
	if err != nil {
		return lockErr(err)
	}
	// RLS esconde el insumo de otra empresa en vez de rechazarlo: si falta alguno de los pedidos, el
	// id no es de esta empresa (o no existe), y se dice antes de escribir nada.
	want := map[int64]struct{}{}
	for _, id := range ids {
		want[id] = struct{}{}
	}
	if len(got) != len(want) {
		return fmt.Errorf("%w: el insumo no existe", domain.ErrNotFound)
	}
	// Sus existencias también, sin esperar: anular, quitar un renglón o cambiarle el artículo restan
	// antes de applyStockLines, y sin esto esperaban a una venta en vez de responder 409.
	if _, err := q.LockIngredientStockNowait(ctx, ids); err != nil {
		return lockErr(err)
	}
	return nil
}

func derefID(p *int64) int64 {
	if p == nil {
		return 0
	}
	return *p
}

// stockStates traduce los renglones guardados al estado que planea domain.PlanStockLines.
func stockStates(rows []db.ListStockLinesRow) []domain.StockLineState {
	out := make([]domain.StockLineState, 0, len(rows))
	for _, r := range rows {
		received := decimal.Zero
		if r.QtyReceived != nil {
			received = *r.QtyReceived
		}
		out = append(out, domain.StockLineState{
			ID: r.ID, IngredientID: derefID(r.IngredientID),
			Unit: domain.PurchaseUnit{
				Kind: string(r.BuyKind), ToBase: r.BuyToBase, BaseKind: string(r.BaseKind), PackQtyInBase: r.PackQtyInBase,
			},
			Ordered: r.Quantity, Received: received, NotComing: r.NotComingQty, Applied: r.AppliedQtyBase,
			Excluded: r.Excluded,
		})
	}
	return out
}

// applyStockLines lleva lo sumado de cada renglón vivo del gasto a lo que llegó, actualiza el
// costo de los insumos que recibieron algo y fija `received_at` (la fecha en que ya no quedó nada
// pendiente). `day` es la fecha de lo que llega hoy; nil = hoy.
//
// Corre con el gasto y sus insumos ya bloqueados (LockExpenseForStock, lockIngredients).
func (s *BackofficeService) applyStockLines(ctx context.Context, q *db.Queries, expenseID, userID int64, day *time.Time) (applied bool, err error) {
	rows, err := q.ListStockLines(ctx, expenseID)
	if err != nil {
		return false, err
	}
	moves, pending, err := domain.PlanStockLines(stockStates(rows))
	if err != nil {
		return false, err
	}
	byID := make(map[int64]db.ListStockLinesRow, len(rows))
	var ids []int64
	for _, r := range rows {
		byID[r.ID] = r
	}
	for _, m := range moves {
		if m.IngredientID != 0 {
			ids = append(ids, m.IngredientID)
		}
	}
	method := domain.CostingLast
	if len(ids) > 0 {
		if _, err := q.LockIngredientStockNowait(ctx, ids); err != nil {
			return false, lockErr(err)
		}
		m, err := q.CostingMethod(ctx)
		if err != nil {
			return false, err
		}
		if method, err = domain.ParseCostingMethod(m); err != nil {
			return false, err
		}
	}

	costChanged := false
	for _, m := range moves {
		r := byID[m.ID]
		if !domain.ValidQty(m.Delta.Abs(), domain.MaxStockQty, false) {
			return false, domain.ErrValidation
		}
		unitCost := domain.UnitCost(r.Amount, m.Ordered)
		if r.IngredientID != nil && m.Delta.IsPositive() && !r.IsGift {
			changed, err := s.applyPurchaseCost(ctx, q, *r.IngredientID, method, m.Delta, unitCost)
			if err != nil {
				return false, err
			}
			costChanged = costChanged || changed
		}
		reason := "compra: " + r.Description
		if m.Delta.IsNegative() {
			reason = "corrección de compra: " + r.Description
		}
		id := r.ID
		if err := q.InsertLineMovement(ctx, db.InsertLineMovementParams{
			ItemType: *r.ItemType, IngredientID: r.IngredientID, ProductID: r.ProductID,
			Quantity: m.Delta, UnitCost: &unitCost, ExpenseID: &expenseID, ExpenseItemID: &id,
			UserID: &userID, Reason: &reason,
		}); err != nil {
			return false, err
		}
		if err := q.AddAppliedQty(ctx, db.AddAppliedQtyParams{ID: r.ID, Delta: m.Delta}); err != nil {
			return false, err
		}
		applied = true
	}
	if costChanged {
		if err := recomputeCosts(ctx, q); err != nil {
			return false, err
		}
	}

	received := pgtype.Date{}
	if !pending {
		at := s.now()
		if day != nil {
			at = *day
		}
		received = pgtype.Date{Time: at, Valid: true}
	}
	return applied, q.SetExpenseReceivedAt(ctx, db.SetExpenseReceivedAtParams{ID: expenseID, ReceivedAt: received})
}

// applyPurchaseCost recalcula el costo de un insumo con el método de la empresa. El insumo se toma
// sin esperar: si lo tiene una venta, la compra se reintenta.
func (s *BackofficeService) applyPurchaseCost(ctx context.Context, q *db.Queries, ingredientID int64, method domain.CostingMethod, qty, unitCost decimal.Decimal) (bool, error) {
	cost, err := q.LockIngredientCostNowait(ctx, ingredientID)
	if err != nil {
		return false, lockErr(err)
	}
	onHand, err := q.IngredientOnHandTotal(ctx, &ingredientID)
	if err != nil {
		return false, err
	}
	next := domain.NextIngredientCost(method, onHand, cost, qty, unitCost, false)
	if next.Equal(cost) {
		return false, nil
	}
	return true, q.UpdateIngredientCost(ctx, db.UpdateIngredientCostParams{ID: ingredientID, CurrentCost: next})
}

// SaveStockLines deja los renglones de almacén de un gasto como los pide la pantalla de «Corregir
// lo que llegó»: agrega, corrige y quita (se marca, no se borra), y el almacén se mueve solo por la
// diferencia. Mandar dos veces lo mismo no mueve nada.
func (s *BackofficeService) SaveStockLines(ctx context.Context, expenseID int64, lines []StockLineInput, userID int64) (ExpenseDetailView, error) {
	if len(lines) > maxExpenseItems {
		return ExpenseDetailView{}, domain.ErrValidation
	}
	today, err := s.parseDayOrToday(ctx, "")
	if err != nil {
		return ExpenseDetailView{}, err
	}
	err = s.store.WithTx(ctx, func(q *db.Queries) error {
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
		before, err := q.ListStockLines(ctx, expenseID)
		if err != nil {
			return err
		}
		current := make(map[int64]db.ListStockLinesRow, len(before))
		var ids []int64
		for _, r := range before {
			current[r.ID] = r
			if r.IngredientID != nil {
				ids = append(ids, *r.IngredientID)
			}
		}
		for _, l := range lines {
			if l.IngredientID != nil {
				ids = append(ids, *l.IngredientID)
			}
		}
		if err := lockIngredients(ctx, q, ids); err != nil {
			return err
		}
		kept := map[int64]bool{}
		amounts := []decimal.Decimal{}
		for i, l := range lines {
			if err := s.saveStockLine(ctx, q, expenseID, userID, i, l, current, kept); err != nil {
				return err
			}
			if l.ID == nil && !l.Excluded || l.ID != nil && !current[*l.ID].Excluded {
				amounts = append(amounts, domain.Round2(l.Amount))
			}
		}
		for id := range current {
			// La pantalla no manda los excluidos: no mandarlos no es quitarlos (R5).
			if kept[id] || current[id].Excluded {
				continue
			}
			// Lo que sumó se revierte antes de marcarlo: un renglón quitado deja de contar.
			if err := reverseLine(ctx, q, expenseID, userID, current[id], "renglón quitado: "); err != nil {
				return err
			}
			if _, err := q.VoidStockLine(ctx, db.VoidStockLineParams{ID: id, ExpenseID: expenseID}); err != nil {
				return err
			}
		}
		// Los renglones no inventariables también cuentan contra el total.
		others, err := q.ListExpenseItems(ctx, expenseID)
		if err != nil {
			return err
		}
		for _, o := range others {
			if o.ItemType == nil && !o.VoidedAt.Valid && !o.Excluded {
				amounts = append(amounts, o.Amount)
			}
		}
		if err := checkLinesFitTotal(amounts, exp.Amount); err != nil {
			return err
		}
		if err := excludedStillCovers(ctx, q, expenseID); err != nil {
			return err
		}
		if _, err := s.applyStockLines(ctx, q, expenseID, userID, &today); err != nil {
			return err
		}
		return auditEntity(ctx, q, domain.AuditExpenseStockCorrected, "expense", expenseID, nil, &userID, nil,
			map[string]any{"lines": len(lines)})
	})
	if err != nil {
		return ExpenseDetailView{}, err
	}
	return s.ExpenseDetail(ctx, expenseID)
}

// lineKeepsPresentation dice si un renglón que ya existe se reenvía con el mismo artículo y el mismo
// empaque, y devuelve lo guardado.
func lineKeepsPresentation(l StockLineInput, current map[int64]db.ListStockLinesRow) (db.ListStockLinesRow, bool) {
	if l.ID == nil || l.PresentationID == nil {
		return db.ListStockLinesRow{}, false
	}
	row, ok := current[*l.ID]
	if !ok || row.PresentationID == nil || *row.PresentationID != *l.PresentationID ||
		derefID(row.IngredientID) != derefID(l.IngredientID) {
		return db.ListStockLinesRow{}, false
	}
	return row, true
}

func (s *BackofficeService) saveStockLine(ctx context.Context, q *db.Queries, expenseID, userID int64, pos int, l StockLineInput, current map[int64]db.ListStockLinesRow, kept map[int64]bool) error {
	amount := domain.Round2(l.Amount)
	// Un excluido (nuevo, o uno guardado que se reenvía) es evidencia: su importe puede ser 0 y lo
	// que diga de recibido se ignora.
	excluded := l.Excluded && l.ID == nil || l.ID != nil && current[*l.ID].Excluded
	if excluded {
		if !domain.ValidMoney(amount, true) {
			return fmt.Errorf("%w: el importe no es válido", domain.ErrValidation)
		}
	} else if err := checkLineMoney(amount, l.IsGift); err != nil {
		return err
	}
	qty := domain.Round4(l.Quantity)
	received := domain.Round4(l.Received)
	notComing := domain.Round4(l.NotComing)
	if !qty.IsPositive() || !domain.ValidQty(qty, domain.MaxStockQty, false) ||
		!domain.ValidStockQtyOrZero(received) ||
		!domain.ValidStockQtyOrZero(notComing) || received.Add(notComing).GreaterThan(qty) || l.Description == "" {
		return domain.ErrValidation
	}
	itemType, err := itemTypeParam(ExpenseItemInput{ItemType: l.ItemType, IngredientID: l.IngredientID, ProductID: l.ProductID})
	if err != nil || itemType == nil {
		return domain.ErrValidation
	}
	if !domain.ValidLineUnitCost(domain.UnitCost(amount, qty)) {
		return fmt.Errorf("%w: el importe por unidad es demasiado grande", domain.ErrValidation)
	}
	var unit *int16
	var pack *decimal.Decimal
	if prev, ok := lineKeepsPresentation(l, current); ok {
		// El empaque no cambió: su contenido ya está copiado en el renglón. Resolverlo de nuevo contra
		// los empaques activos dejaba sin poder corregir una compra cuyo empaque ya se retiró.
		unit, pack = prev.UnitID, prev.PackQtyInBase
	} else if unit, pack, err = lineUnit(ctx, q, l.IngredientID, l.PresentationID, l.UnitID, nil); err != nil {
		return err
	}
	if unit == nil {
		return domain.ErrValidation
	}
	var recv *decimal.Decimal
	if received.IsPositive() && !excluded {
		recv = &received
	}
	if l.ID == nil {
		_, err := q.CreateExpenseItem(ctx, db.CreateExpenseItemParams{
			ExpenseID: expenseID, ItemType: itemType, IngredientID: l.IngredientID, ProductID: l.ProductID,
			Description: l.Description, Quantity: qty, UnitID: unit, QtyReceived: recv,
			UnitCost: domain.UnitCost(amount, qty), Amount: amount, PackQtyInBase: pack,
			Position: int32(pos), PresentationID: l.PresentationID, IsGift: l.IsGift, Excluded: l.Excluded,
		})
		return err
	}
	row, ok := current[*l.ID]
	if !ok {
		return domain.ErrNotFound
	}
	if l.SeenApplied == nil || !domain.ValidStockQtyOrZero(*l.SeenApplied) {
		return domain.ErrValidation
	}
	if !domain.Round4(*l.SeenApplied).Equal(row.AppliedQtyBase) {
		return fmt.Errorf("%w: alguien más corrigió esta compra; vuelve a abrirla", domain.ErrConflict)
	}
	kept[*l.ID] = true
	// Otro artículo: lo que el renglón sumó es del artículo viejo y se le regresa ahí. Comparar solo
	// en unidad base daría diferencia cero y dejaría la existencia en el insumo equivocado.
	if derefID(row.IngredientID) != derefID(l.IngredientID) || derefID(row.ProductID) != derefID(l.ProductID) {
		if err := reverseLine(ctx, q, expenseID, userID, row, "cambio de artículo: "); err != nil {
			return err
		}
	}
	n, err := q.UpdateStockLine(ctx, db.UpdateStockLineParams{
		ID: *l.ID, ExpenseID: expenseID, ItemType: itemType, IngredientID: l.IngredientID, ProductID: l.ProductID,
		Description: l.Description, Quantity: qty, UnitID: unit, QtyReceived: recv, Amount: amount,
		UnitCost: domain.UnitCost(amount, qty), PackQtyInBase: pack, PresentationID: l.PresentationID,
		IsGift: l.IsGift, NotComingQty: notComing,
	})
	if err == nil && n == 0 {
		return domain.ErrNotFound
	}
	return err
}

// reverseStockLines regresa a cero lo que sumaron los renglones de un gasto (al anularlo). No
// cambia el costo: una reversión no reconstruye el promedio (ponytail de NextIngredientCost).
func (s *BackofficeService) reverseStockLines(ctx context.Context, q *db.Queries, expenseID, userID int64) error {
	rows, err := q.ListStockLines(ctx, expenseID)
	if err != nil {
		return err
	}
	var ids []int64
	for _, r := range rows {
		if r.IngredientID != nil && !r.AppliedQtyBase.IsZero() {
			ids = append(ids, *r.IngredientID)
		}
	}
	if err := lockIngredients(ctx, q, ids); err != nil {
		return err
	}
	slices.SortStableFunc(rows, func(a, b db.ListStockLinesRow) int { return int(derefID(a.IngredientID) - derefID(b.IngredientID)) })
	for _, r := range rows {
		if err := reverseLine(ctx, q, expenseID, userID, r, "compra anulada: "); err != nil {
			return err
		}
	}
	return nil
}

// reverseLine deja en cero lo aplicado de un renglón con un movimiento de signo contrario.
func reverseLine(ctx context.Context, q *db.Queries, expenseID, userID int64, r db.ListStockLinesRow, why string) error {
	if r.AppliedQtyBase.IsZero() {
		return nil
	}
	delta := r.AppliedQtyBase.Neg()
	reason := why + r.Description
	id := r.ID
	if err := q.InsertLineMovement(ctx, db.InsertLineMovementParams{
		ItemType: *r.ItemType, IngredientID: r.IngredientID, ProductID: r.ProductID,
		Quantity: delta, ExpenseID: &expenseID, ExpenseItemID: &id, UserID: &userID, Reason: &reason,
	}); err != nil {
		return err
	}
	return q.AddAppliedQty(ctx, db.AddAppliedQtyParams{ID: r.ID, Delta: delta})
}

// includedAmounts son los importes de los renglones vivos que cuentan contra el total: los
// excluidos no son gasto del negocio (spec 049).
func includedAmounts(items []db.ListExpenseItemsRow) []decimal.Decimal {
	out := []decimal.Decimal{}
	for _, it := range items {
		if !it.VoidedAt.Valid && !it.Excluded {
			out = append(out, it.Amount)
		}
	}
	return out
}

// SetItemExcluded excluye o vuelve a incluir un renglón de un gasto ya guardado (spec 049, R4).
// Lo que ya había entrado sale con una corrección; lo que vuelve a contar entra por diferencia, y
// la suma de los incluidos no puede pasar del total (la regla de la 038).
func (s *BackofficeService) SetItemExcluded(ctx context.Context, expenseID, itemID int64, excluded bool, userID int64) (ExpenseDetailView, error) {
	today, err := s.parseDayOrToday(ctx, "")
	if err != nil {
		return ExpenseDetailView{}, err
	}
	err = s.store.WithTx(ctx, func(q *db.Queries) error {
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
		item, err := q.LockExpenseItemForExclusion(ctx, db.LockExpenseItemForExclusionParams{ID: itemID, ExpenseID: expenseID})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return domain.ErrNotFound
			}
			return err
		}
		if item.Excluded == excluded {
			return nil
		}
		// Un excluido pudo guardarse en $0 sin ser regalo; incluido, ese importe ya no vale y la base
		// lo rechazaría como 500.
		if !excluded {
			if err := checkLineMoney(item.Amount, item.IsGift); err != nil {
				return err
			}
		}
		if item.IngredientID != nil {
			if err := lockIngredients(ctx, q, []int64{*item.IngredientID}); err != nil {
				return err
			}
		}
		if err := q.SetExpenseItemExcluded(ctx, db.SetExpenseItemExcludedParams{ID: itemID, Excluded: excluded}); err != nil {
			return err
		}
		items, err := q.ListExpenseItems(ctx, expenseID)
		if err != nil {
			return err
		}
		if err := checkLinesFitTotal(includedAmounts(items), exp.Amount); err != nil {
			return err
		}
		if err := excludedStillCovers(ctx, q, expenseID); err != nil {
			return err
		}
		if _, err := s.applyStockLines(ctx, q, expenseID, userID, &today); err != nil {
			return err
		}
		return auditEntity(ctx, q, domain.AuditExpenseItemExcluded, "expense", expenseID, nil, &userID,
			map[string]any{"itemId": itemID, "excluded": item.Excluded}, map[string]any{"itemId": itemID, "excluded": excluded})
	})
	if err != nil {
		return ExpenseDetailView{}, err
	}
	return s.ExpenseDetail(ctx, expenseID)
}
