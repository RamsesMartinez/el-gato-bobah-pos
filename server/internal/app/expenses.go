package app

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store/db"
)

// Gastos con detalle de mercancía y pagos múltiples.
//
// Un gasto tiene tres partes que se mueven en tiempos distintos, y de ahí el diseño:
//   - ENCABEZADO: importe, categoría, proveedor, fecha del documento.
//   - MERCANCÍA (expense_items): qué se compró. Toca el almacén al RECIBIR, no al capturar
//     — una cosa es la fecha en que se pide y otra cuándo entra la mercancía.
//   - PAGOS (expense_payments): un ticket real cobra con más de un medio.

// ---- Vistas ----

type ExpenseView struct {
	ID             int64           `json:"id"`
	ExpenseDate    string          `json:"expenseDate"` // YYYY-MM-DD, fecha del documento
	ReceivedAt     *string         `json:"receivedAt"`  // null = mercancía no recibida
	Status         string          `json:"status"`
	Category       string          `json:"category"`
	FinancialGroup string          `json:"financialGroup"`
	Supplier       *string         `json:"supplier"`
	Amount         decimal.Decimal `json:"amount"`
	Currency       domain.Currency `json:"currency"`
	Description    *string         `json:"description"`
	DocKind        *string         `json:"docKind"`
	DocFolio       *string         `json:"docFolio"`
	PaymentMethod  *string         `json:"paymentMethod"` // "Tarjeta + Efectivo" si hubo varios
	PaidAt         *time.Time      `json:"paidAt"`
	CreatedBy      *string         `json:"createdBy"`
	ItemCount      int64           `json:"itemCount"`
}

type ExpenseItemView struct {
	ID            int64            `json:"id"`
	ItemType      *string          `json:"itemType"` // null = línea no inventariable
	IngredientID  *int64           `json:"ingredientId"`
	ProductID     *int64           `json:"productId"`
	ItemName      *string          `json:"itemName"`
	Description   string           `json:"description"`
	Quantity      decimal.Decimal  `json:"quantity"`
	UnitCode      *string          `json:"unitCode"`
	QtyReceived   *decimal.Decimal `json:"qtyReceived"`
	UnitCost      decimal.Decimal  `json:"unitCost"`
	Amount        decimal.Decimal  `json:"amount"`
	PackQtyInBase *decimal.Decimal `json:"packQtyInBase"`
}

type ExpensePaymentView struct {
	ID          int64           `json:"id"`
	MethodID    int16           `json:"methodId"`
	Method      string          `json:"method"`
	Amount      decimal.Decimal `json:"amount"`
	PaidOn      string          `json:"paidOn"`
	InCashCount bool            `json:"inCashCount"` // atribuido a un corte
	Reference   *string         `json:"reference"`
	AffectsCash bool            `json:"affectsCashDrawer"`
}

// ExpenseDetailView es el gasto completo, para la pantalla de detalle/edición.
type ExpenseDetailView struct {
	ExpenseView
	Items    []ExpenseItemView    `json:"items"`
	Payments []ExpensePaymentView `json:"payments"`
	// Paid: suma de pagos. Se calcula en vez de guardarse para que no pueda desincronizarse
	// del detalle.
	Paid decimal.Decimal `json:"paid"`
}

// ---- Entradas ----

type ExpenseItemInput struct {
	ItemType      string // "" (no inventariable) | ingrediente | producto
	IngredientID  *int64
	ProductID     *int64
	Description   string
	Quantity      decimal.Decimal
	UnitID        *int16
	QtyReceived   *decimal.Decimal // nil = no recibido aún
	Amount        decimal.Decimal
	PackQtyInBase *decimal.Decimal
	// RawCode/RawName: el texto del documento del proveedor. Se guardan aparte del Description
	// porque son la LLAVE con la que se aprende el mapeo (ver learnSupplierItems).
	RawCode string
	RawName string
	// Personal: el renglón no es del local (venía en el mismo ticket pero es de la casa). No se
	// guarda como línea del gasto —su importe no es gasto del negocio— pero SÍ se aprende, para
	// que la próxima compra a ese proveedor lo marque solo.
	Personal bool
}

type ExpensePaymentInput struct {
	MethodID int16
	Amount   decimal.Decimal
	PaidOn   string // YYYY-MM-DD; vacío = hoy
	// RegisterID: la caja a cuyo corte se atribuye el pago. Para métodos que afectan el cajón
	// es OBLIGATORIA (ver validatePayment): efectivo que sale sin movimiento de caja descuadra
	// el corte, y eso no puede quedar a criterio de quien captura.
	RegisterID *int64
	Reference  string
}

type ExpenseInput struct {
	ExpenseDate string // YYYY-MM-DD; vacío = hoy
	ReceivedAt  string // YYYY-MM-DD; vacío = aún no recibido
	CategoryID  int64
	SupplierID  *int64
	Amount      decimal.Decimal
	Description string
	Status      string // pendiente | pagada
	Items       []ExpenseItemInput
	Payments    []ExpensePaymentInput
	DocKind     string
	DocFolio    string
	DocRaw      json.RawMessage // extracción cruda del documento, si vino de uno
	UserID      int64
}

// maxExpenseItems acota el detalle: el ticket más largo que hemos visto trae ~30 renglones.
// El techo evita que un payload absurdo genere miles de inserts en una tx.
const maxExpenseItems = 300

// ---- Alta ----

// CreateExpense registra el gasto, su mercancía y sus pagos en UNA transacción, y si viene con
// fecha de recepción también genera los movimientos de almacén.
//
// Todo o nada a propósito: un gasto guardado a medias (con líneas pero sin el pago, o con el
// pago sin la salida de caja) descuadra la contabilidad y nadie se entera hasta el corte.
func (s *BackofficeService) CreateExpense(ctx context.Context, in ExpenseInput) (int64, error) {
	amount := domain.Round2(in.Amount)
	if !domain.ValidMoney(amount, false) || in.CategoryID == 0 {
		return 0, domain.ErrValidation
	}
	if in.Status != domain.ExpensePendiente && in.Status != domain.ExpensePagada {
		return 0, domain.ErrValidation // no se crea directo como cancelada
	}
	if len(in.Items) > maxExpenseItems {
		return 0, domain.ErrValidation
	}
	docDate, err := s.parseDayOrToday(in.ExpenseDate)
	if err != nil {
		return 0, err
	}
	received, hasReceived, err := s.parseOptionalDay(in.ReceivedAt)
	if err != nil {
		return 0, err
	}

	// Los pagos se validan ANTES de abrir la transacción: resolver la caja y el método son
	// lecturas, y así un pago inválido no deja una tx abierta a medias.
	payments, err := s.validatePayments(ctx, in.Payments, docDate)
	if err != nil {
		return 0, err
	}
	if in.Status == domain.ExpensePagada {
		if err := checkPaymentsCover(payments, amount); err != nil {
			return 0, err
		}
	}

	params := db.CreateExpenseParams{
		ExpenseDate: pgtype.Date{Time: docDate, Valid: true},
		CategoryID:  in.CategoryID, SupplierID: in.SupplierID, Amount: amount,
		Description: strPtr(in.Description), CreatedBy: in.UserID,
		Status:   db.ExpenseStatus(in.Status),
		DocKind:  strPtr(in.DocKind),
		DocFolio: strPtr(in.DocFolio),
		DocRaw:   in.DocRaw,
	}
	if hasReceived {
		params.ReceivedAt = pgtype.Date{Time: received, Valid: true}
	}
	if in.Status == domain.ExpensePagada {
		params.PaidAt = pgtype.Timestamptz{Time: s.now(), Valid: true}
		params.PaidBy = &in.UserID
	}

	var id int64
	err = s.store.WithTx(ctx, func(q *db.Queries) error {
		var err error
		if id, err = q.CreateExpense(ctx, params); err != nil {
			return err
		}
		if err := s.insertItems(ctx, q, id, in.Items); err != nil {
			return err
		}
		if err := s.insertPayments(ctx, q, id, in.Description, in.UserID, payments); err != nil {
			return err
		}
		// Si ya llegó la mercancía, se descuenta en la misma tx: el caso normal es volver de la
		// tienda con las cosas y capturar una sola vez.
		if hasReceived {
			if err := s.depleteReceived(ctx, q, id, in.UserID); err != nil {
				return err
			}
		}
		return s.learnSupplierItems(ctx, q, in.SupplierID, in.Items)
	})
	return id, err
}

// ---- Recepción ----

// ReceiveExpense marca la mercancía como recibida en la fecha dada y genera los movimientos de
// almacén de las líneas que llegaron.
//
// Idempotente por el guard `received_at is null` de la query: un doble-tap no puede duplicar el
// inventario. Las cantidades recibidas (qty_received) se fijan antes con SetItemsReceived —
// aquí solo se consume lo que llegó.
func (s *BackofficeService) ReceiveExpense(ctx context.Context, id int64, receivedAt string, userID int64) error {
	date, err := s.parseDayOrToday(receivedAt)
	if err != nil {
		return err
	}
	if _, err := s.getExpense(ctx, id); err != nil {
		return err
	}
	return s.store.WithTx(ctx, func(q *db.Queries) error {
		n, err := q.MarkExpenseReceived(ctx, db.MarkExpenseReceivedParams{
			ID: id, ReceivedAt: pgtype.Date{Time: date, Valid: true},
		})
		if err != nil {
			return err
		}
		if n == 0 {
			return domain.ErrConflict // ya estaba recibido
		}
		return s.depleteReceived(ctx, q, id, userID)
	})
}

// SetItemsReceived fija cuánto llegó de cada línea. 0 = no llegó (el renglón "No disponible" de
// un pedido); distinto de lo pedido = entrega parcial o peso ajustado.
func (s *BackofficeService) SetItemsReceived(ctx context.Context, expenseID int64, got map[int64]decimal.Decimal) error {
	exp, err := s.getExpense(ctx, expenseID)
	if err != nil {
		return err
	}
	// Después de recibido las cantidades ya generaron movimientos de almacén; cambiarlas dejaría
	// el ledger mintiendo. La corrección es un ajuste de inventario, no una edición del gasto.
	if exp.ReceivedAt.Valid {
		return domain.ErrConflict
	}
	return s.store.WithTx(ctx, func(q *db.Queries) error {
		for itemID, qty := range got {
			v := domain.Round4(qty)
			if v.IsNegative() || v.GreaterThan(domain.MaxStockQty) {
				return domain.ErrValidation
			}
			n, err := q.SetExpenseItemReceived(ctx, db.SetExpenseItemReceivedParams{
				ID: itemID, ExpenseID: expenseID, QtyReceived: &v,
			})
			if err != nil {
				return err
			}
			if n == 0 {
				return domain.ErrNotFound // la línea no es de este gasto
			}
		}
		return nil
	})
}

// depleteReceived inserta un movimiento 'compra' por cada línea inventariable que llegó.
// El trigger de stock_movements actualiza stock_levels (0009), así que aquí no se toca el nivel.
func (s *BackofficeService) depleteReceived(ctx context.Context, q *db.Queries, expenseID, userID int64) error {
	rows, err := q.ItemsToDeplete(ctx, expenseID)
	if err != nil {
		return err
	}
	for _, r := range rows {
		if r.QtyReceived == nil {
			continue
		}
		baseQty, err := domain.BaseQty(*r.QtyReceived, domain.PurchaseUnit{
			Kind:          string(r.BuyKind),
			ToBase:        r.BuyToBase,
			BaseKind:      string(r.BaseKind),
			PackQtyInBase: r.PackQtyInBase,
		})
		if err != nil {
			// Falta el formato del proveedor o la unidad no es compatible: se devuelve tal cual
			// (envuelve ErrValidation → 422 con mensaje accionable) en vez de meter al almacén
			// una cantidad inventada.
			return err
		}
		if !domain.ValidQty(baseQty, domain.MaxStockQty, false) {
			return domain.ErrValidation
		}
		cost := domain.UnitCost(r.Amount, baseQty)
		reason := "compra: " + r.Description
		if err := q.InsertPurchaseMovement(ctx, db.InsertPurchaseMovementParams{
			ItemType:     *r.ItemType,
			IngredientID: r.IngredientID,
			ProductID:    r.ProductID,
			Quantity:     baseQty,
			UnitCost:     &cost,
			ExpenseID:    &expenseID,
			UserID:       &userID,
			Reason:       &reason,
		}); err != nil {
			return err
		}
	}
	return nil
}

// ---- Pagos ----

// resolvedPayment es un pago ya validado contra la caja y el método: lleva resuelto lo que la
// transacción necesita para no volver a consultar.
type resolvedPayment struct {
	in        ExpensePaymentInput
	paidOn    time.Time
	sessionID *int64 // no-nil = entra al arqueo de ese corte
	isCash    bool
}

// AddExpensePayment registra un pago adicional sobre un gasto (el "+1 nuevo pago"), y si con él
// los pagos cubren el importe, marca el gasto como pagado.
func (s *BackofficeService) AddExpensePayment(ctx context.Context, expenseID int64, in ExpensePaymentInput, userID int64) error {
	exp, err := s.getExpense(ctx, expenseID)
	if err != nil {
		return err
	}
	if !domain.CanPayExpense(string(exp.Status)) {
		return domain.ErrConflict // ya pagada o cancelada
	}
	payments, err := s.validatePayments(ctx, []ExpensePaymentInput{in}, exp.ExpenseDate.Time)
	if err != nil {
		return err
	}
	return s.store.WithTx(ctx, func(q *db.Queries) error {
		if err := s.insertPayments(ctx, q, expenseID, derefStr(exp.Description), userID, payments); err != nil {
			return err
		}
		paid, err := q.SumExpensePayments(ctx, expenseID)
		if err != nil {
			return err
		}
		// Un abono parcial deja el gasto pendiente; solo cubrir el importe lo cierra. Así el
		// pago partido (tarjeta + efectivo) funciona sin un estado "parcial" extra.
		if paid.LessThan(exp.Amount) {
			return nil
		}
		n, err := q.MarkExpensePaid(ctx, db.MarkExpensePaidParams{ID: expenseID, PaidBy: &userID})
		if err != nil {
			return err
		}
		if n == 0 {
			return domain.ErrConflict // carrera entre el GET y el UPDATE
		}
		return nil
	})
}

// validatePayments resuelve cada pago contra su método y su caja. Es la frontera donde se
// aplica la regla que protege el corte.
func (s *BackofficeService) validatePayments(ctx context.Context, in []ExpensePaymentInput, fallbackDate time.Time) ([]resolvedPayment, error) {
	out := make([]resolvedPayment, 0, len(in))
	for _, p := range in {
		amount := domain.Round2(p.Amount)
		if !domain.ValidMoney(amount, false) {
			return nil, domain.ErrValidation
		}
		m, err := s.store.QC(ctx).GetPaymentMethod(ctx, p.MethodID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil, domain.ErrValidation // método inexistente
			}
			return nil, err
		}
		paidOn := fallbackDate
		if p.PaidOn != "" {
			d, err := parseDate(&p.PaidOn)
			if err != nil {
				return nil, err
			}
			paidOn = d.Time
		}
		r := resolvedPayment{in: p, paidOn: paidOn, isCash: m.AffectsCashDrawer}
		// Para un método que mueve el cajón el arqueo NO es opcional: el efectivo salió de una
		// caja concreta y el corte tiene que verlo. Para los demás (transferencia, tarjeta) sí
		// es decisión del operador atribuirlo o no a un corte.
		if m.AffectsCashDrawer && p.RegisterID == nil {
			return nil, domain.ErrPaymentNeedsRegister
		}
		if p.RegisterID != nil {
			sess, err := s.store.QC(ctx).GetOpenSessionByRegister(ctx, *p.RegisterID)
			if err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					return nil, domain.ErrConflict // la caja elegida no está abierta
				}
				return nil, err
			}
			r.sessionID = &sess.ID
		}
		r.in.Amount = amount
		out = append(out, r)
	}
	return out, nil
}

func (s *BackofficeService) insertPayments(ctx context.Context, q *db.Queries, expenseID int64, desc string, userID int64, payments []resolvedPayment) error {
	for _, p := range payments {
		if _, err := q.CreateExpensePayment(ctx, db.CreateExpensePaymentParams{
			ExpenseID:         expenseID,
			PaymentMethodID:   p.in.MethodID,
			Amount:            p.in.Amount,
			PaidOn:            pgtype.Date{Time: p.paidOn, Valid: true},
			RegisterSessionID: p.sessionID,
			Reference:         strPtr(p.in.Reference),
			PaidBy:            userID,
		}); err != nil {
			return err
		}
		// La salida del cajón va en la MISMA tx que el pago: si una falla no queda dinero
		// fantasma en ningún lado.
		if p.isCash && p.sessionID != nil {
			if err := q.InsertExpenseCashMovement(ctx, db.InsertExpenseCashMovementParams{
				SessionID: *p.sessionID, Amount: p.in.Amount,
				Concept: expenseConcept(desc), ExpenseID: &expenseID, UserID: userID,
			}); err != nil {
				return err
			}
		}
	}
	return nil
}

// checkPaymentsCover exige que los pagos cubran el importe antes de dar el gasto por pagado.
// Sin esto un gasto quedaría "pagado" con menos dinero registrado del que salió.
func checkPaymentsCover(payments []resolvedPayment, amount decimal.Decimal) error {
	var sum decimal.Decimal
	for _, p := range payments {
		sum = sum.Add(p.in.Amount)
	}
	if domain.Round2(sum).LessThan(amount) {
		return domain.ErrPaymentsBelowAmount
	}
	return nil
}

// learnedStatus decide con qué estado se recuerda un renglón del proveedor para la próxima compra.
//
// 'personal' gana sobre el artículo elegido: si el operador dice que el renglón es de la casa, no
// importa a qué estuvo mapeado antes — volver a proponer ese artículo sería reintroducir el error
// que acaba de corregir.
//
// Sin artículo se recuerda como 'ignorado' y no como 'pendiente': el operador ya lo revisó y
// decidió que no es inventariable (bolsa, envío, IVA), así que no vuelve a la cola de revisión.
func learnedStatus(it ExpenseItemInput) (string, error) {
	if it.Personal {
		return domain.SupplierItemPersonal, nil
	}
	itemType, err := itemTypeParam(it)
	if err != nil {
		return "", err
	}
	if itemType != nil {
		return domain.SupplierItemMapeado, nil
	}
	return domain.SupplierItemIgnorado, nil
}

// ---- Líneas ----

func (s *BackofficeService) insertItems(ctx context.Context, q *db.Queries, expenseID int64, items []ExpenseItemInput) error {
	for i, it := range items {
		// Un renglón de la casa no es una línea del gasto: se descarta aquí (ya se aprendió en
		// learnSupplierItems) para que no sume al importe ni toque el almacén.
		if it.Personal {
			continue
		}
		qty := domain.Round4(it.Quantity)
		if !domain.ValidQty(qty, domain.MaxStockQty, false) {
			return domain.ErrValidation
		}
		amount := domain.Round2(it.Amount)
		if !domain.ValidMoney(amount, true) { // 0 permitido: una cortesía o un renglón sin costo
			return domain.ErrValidation
		}
		if it.Description == "" {
			return domain.ErrValidation
		}
		itemType, err := itemTypeParam(it)
		if err != nil {
			return err
		}
		// Una línea inventariable exige unidad: sin ella no hay conversión a unidad de almacén
		// (el CHECK de la tabla lo repite como defensa en profundidad).
		if itemType != nil && it.UnitID == nil {
			return domain.ErrValidation
		}
		if it.QtyReceived != nil {
			v := domain.Round4(*it.QtyReceived)
			if v.IsNegative() || v.GreaterThan(domain.MaxStockQty) {
				return domain.ErrValidation
			}
			it.QtyReceived = &v
		}
		unitCost := domain.UnitCost(amount, qty)
		if _, err := q.CreateExpenseItem(ctx, db.CreateExpenseItemParams{
			ExpenseID: expenseID, ItemType: itemType,
			IngredientID: it.IngredientID, ProductID: it.ProductID,
			Description: it.Description, Quantity: qty, UnitID: it.UnitID,
			QtyReceived: it.QtyReceived, UnitCost: unitCost, Amount: amount,
			PackQtyInBase: it.PackQtyInBase, Position: int32(i),
		}); err != nil {
			return err
		}
	}
	return nil
}

// itemTypeParam valida la coherencia tipo↔referencia antes de que el CHECK de la tabla la
// rechace como 500. "" = línea no inventariable (bolsa, envío, IVA): suma al gasto y no toca
// el almacén, que es la mayoría de los renglones de un ticket de tienda mixta.
func itemTypeParam(it ExpenseItemInput) (*db.StockItemType, error) {
	switch it.ItemType {
	case "":
		if it.IngredientID != nil || it.ProductID != nil {
			return nil, domain.ErrValidation
		}
		return nil, nil
	case string(db.StockItemTypeIngrediente):
		if it.IngredientID == nil || it.ProductID != nil {
			return nil, domain.ErrValidation
		}
	case string(db.StockItemTypeProducto):
		if it.ProductID == nil || it.IngredientID != nil {
			return nil, domain.ErrValidation
		}
	default:
		return nil, domain.ErrValidation
	}
	t := db.StockItemType(it.ItemType)
	return &t, nil
}

// learnSupplierItems escribe el mapeo aprendido: cada línea con texto del documento y artículo
// asignado se guarda contra su proveedor, de modo que la próxima compra se autollene.
//
// Es el bucle de aprendizaje completo. Sin proveedor no hay nada que aprender (la llave es
// (proveedor, código|nombre)), y una línea sin texto original tampoco: se capturó a mano y no
// hay nada que reconocer la próxima vez.
func (s *BackofficeService) learnSupplierItems(ctx context.Context, q *db.Queries, supplierID *int64, items []ExpenseItemInput) error {
	if supplierID == nil {
		return nil
	}
	for _, it := range items {
		if it.RawName == "" {
			continue
		}
		itemType, err := itemTypeParam(it)
		if err != nil {
			return err
		}
		status, err := learnedStatus(it)
		if err != nil {
			return err
		}
		var cost *decimal.Decimal
		if c := domain.UnitCost(domain.Round2(it.Amount), domain.Round4(it.Quantity)); c.IsPositive() {
			cost = &c
		}
		if _, err := q.UpsertSupplierItem(ctx, db.UpsertSupplierItemParams{
			SupplierID:    *supplierID,
			RawCode:       strPtr(it.RawCode),
			RawName:       it.RawName,
			NormName:      domain.NormalizeItemName(it.RawName),
			Status:        status,
			ItemType:      itemType,
			IngredientID:  it.IngredientID,
			ProductID:     it.ProductID,
			PackQtyInBase: it.PackQtyInBase,
			UnitID:        it.UnitID,
			LastCost:      cost,
		}); err != nil {
			return err
		}
	}
	return nil
}

// ---- Cancelación ----

// CancelExpense anula una pendiente (una pagada es terminal — ver domain.CanCancelExpense).
func (s *BackofficeService) CancelExpense(ctx context.Context, id int64, reason string, userID int64) error {
	exp, err := s.getExpense(ctx, id)
	if err != nil {
		return err
	}
	if !domain.CanCancelExpense(string(exp.Status)) {
		return domain.ErrConflict
	}
	// Una compra ya recibida movió el almacén: anular el gasto no puede devolver la mercancía
	// sola, y dejar el movimiento sin gasto rompe la trazabilidad del ledger.
	if exp.ReceivedAt.Valid {
		return domain.ErrConflict
	}
	n, err := s.store.QC(ctx).CancelExpense(ctx, db.CancelExpenseParams{
		ID: id, CancelledBy: &userID, CancelReason: strPtr(reason),
	})
	if err != nil {
		return err
	}
	if n == 0 {
		return domain.ErrConflict
	}
	return nil
}

// ---- Consultas ----

// ListExpenses devuelve una página de gastos + el total (para el paginador).
// pendingReceipt filtra los que aún no llegan al almacén (la bandeja de "por recibir").
// sort/dir ordenan por columna (ver ListExpenses en queries/expenses.sql).
func (s *BackofficeService) ListExpenses(ctx context.Context, status string, pendingReceipt bool, sort, dir string, limit, offset int32) ([]ExpenseView, int64, error) {
	var st *db.ExpenseStatus
	if domain.ValidExpenseStatus(status) {
		v := db.ExpenseStatus(status)
		st = &v
	}
	var pending *bool
	if pendingReceipt {
		pending = &pendingReceipt
	}
	total, err := s.store.QC(ctx).CountExpenses(ctx, db.CountExpensesParams{Status: st, PendingReceipt: pending})
	if err != nil {
		return nil, 0, err
	}
	rows, err := s.store.QC(ctx).ListExpenses(ctx, db.ListExpensesParams{
		Status: st, PendingReceipt: pending, Sort: sort, Dir: dir, Lim: limit, Off: offset,
	})
	if err != nil {
		return nil, 0, err
	}
	out := make([]ExpenseView, len(rows))
	for i, r := range rows {
		out[i] = ExpenseView{
			ID: r.ID, ExpenseDate: r.ExpenseDate.Time.Format(dateFmt),
			ReceivedAt: dateStr(r.ReceivedAt), Status: string(r.Status),
			Category: r.Category, FinancialGroup: string(r.FinancialGroup), Supplier: r.Supplier,
			Amount: r.Amount, Currency: domain.Currency(r.Currency), Description: r.Description,
			DocKind: r.DocKind, DocFolio: r.DocFolio,
			PaymentMethod: bytesPtr(r.PaymentMethod), PaidAt: tsPtr(r.PaidAt),
			CreatedBy: r.CreatedByName, ItemCount: r.ItemCount,
		}
	}
	return out, total, nil
}

// ExpenseDetail devuelve el gasto con su mercancía y sus pagos.
func (s *BackofficeService) ExpenseDetail(ctx context.Context, id int64) (ExpenseDetailView, error) {
	var out ExpenseDetailView
	exp, err := s.store.QC(ctx).GetExpenseView(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return out, domain.ErrNotFound
		}
		return out, err
	}
	items, err := s.store.QC(ctx).ListExpenseItems(ctx, id)
	if err != nil {
		return out, err
	}
	pays, err := s.store.QC(ctx).ListExpensePayments(ctx, id)
	if err != nil {
		return out, err
	}
	paid, err := s.store.QC(ctx).SumExpensePayments(ctx, id)
	if err != nil {
		return out, err
	}
	out.ExpenseView = ExpenseView{
		ID: exp.ID, ExpenseDate: exp.ExpenseDate.Time.Format(dateFmt),
		ReceivedAt: dateStr(exp.ReceivedAt), Status: string(exp.Status),
		Category: exp.Category, FinancialGroup: string(exp.FinancialGroup), Supplier: exp.Supplier,
		Amount: exp.Amount, Currency: domain.Currency(exp.Currency), Description: exp.Description,
		DocKind: exp.DocKind, DocFolio: exp.DocFolio, PaidAt: tsPtr(exp.PaidAt),
		CreatedBy: exp.CreatedByName,
	}
	out.Paid = paid
	out.Items = make([]ExpenseItemView, len(items))
	for i, r := range items {
		v := ExpenseItemView{
			ID: r.ID, IngredientID: r.IngredientID, ProductID: r.ProductID,
			ItemName: firstName(r.IngredientName, r.ProductName), Description: r.Description, Quantity: r.Quantity,
			UnitCode: r.UnitCode, QtyReceived: r.QtyReceived, UnitCost: r.UnitCost,
			Amount: r.Amount, PackQtyInBase: r.PackQtyInBase,
		}
		if r.ItemType != nil {
			t := string(*r.ItemType)
			v.ItemType = &t
		}
		out.Items[i] = v
	}
	out.Payments = make([]ExpensePaymentView, len(pays))
	for i, r := range pays {
		out.Payments[i] = ExpensePaymentView{
			ID: r.ID, MethodID: r.PaymentMethodID, Method: r.Method, Amount: r.Amount,
			PaidOn: r.PaidOn.Time.Format(dateFmt), InCashCount: r.RegisterSessionID != nil,
			Reference: r.Reference, AffectsCash: r.AffectsCashDrawer,
		}
	}
	return out, nil
}

func (s *BackofficeService) getExpense(ctx context.Context, id int64) (db.Expense, error) {
	exp, err := s.store.QC(ctx).GetExpense(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return exp, domain.ErrNotFound
		}
		return exp, err
	}
	return exp, nil
}

// ---- Helpers de fecha ----
// parseDate(*string) → pgtype.Date y dateStr viven en admin.go; aquí solo lo específico del gasto.

// parseDayOrToday: una fecha del formulario, cayendo a hoy cuando no viene. Las fechas del gasto
// son DÍAS (no instantes): la del documento y la de recepción las decide una persona mirando un
// papel, no el reloj del servidor.
func (s *BackofficeService) parseDayOrToday(v string) (time.Time, error) {
	if v == "" {
		return s.now(), nil
	}
	d, err := parseDate(&v)
	if err != nil {
		return time.Time{}, err
	}
	return d.Time, nil
}

// parseOptionalDay distingue "no viene" (válido: aún no se recibe) de "viene mal" (400).
func (s *BackofficeService) parseOptionalDay(v string) (time.Time, bool, error) {
	if v == "" {
		return time.Time{}, false, nil
	}
	t, err := s.parseDayOrToday(v)
	return t, err == nil, err
}

// firstName elige el nombre del artículo entre los dos posibles orígenes (ingrediente o
// producto). Los dos vienen de left joins y AMBOS son null en una línea no inventariable —
// resolverlo con un coalesce en SQL hacía que sqlc lo tipara no-nulable y reventaba al escanear.
func firstName(ingredient, product *string) *string {
	if ingredient != nil {
		return ingredient
	}
	return product
}

// bytesPtr convierte el string_agg de los métodos de pago (que sqlc tipa como []byte por venir
// de una subconsulta agregada) al *string de la vista.
func bytesPtr(b []byte) *string {
	if len(b) == 0 {
		return nil
	}
	s := string(b)
	return &s
}
