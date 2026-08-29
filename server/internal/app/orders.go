package app

import (
	"context"
	"errors"
	"time"
	"uuid"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store/db"
)

type OrdersService struct {
	store *store.Store
	now   func() time.Time
}

func NewOrdersService(s *store.Store, now func() time.Time) *OrdersService {
	if now == nil {
		now = time.Now
	}
	return &OrdersService{store: s, now: now}
}

type PaymentInput struct {
	MethodID  int16
	Amount    decimal.Decimal
	Tip       decimal.Decimal
	Reference *string
}

type CreateOrderCmd struct {
	ClientUUID         uuid.UUID
	ServiceType        string
	DeliveryPlatformID *int16
	CustomerName       *string
	Notes              *string
	OpenedBy           int64
	DeliveryFee        decimal.Decimal // capturado en el cobro; solo aplica a domicilio
	Lines              []domain.OrderLineInput
	// Payments: 0..N líneas de pago (pago dividido). Vacío = enviar a cocina sin cobrar.
	// La orden queda "pagada" cuando la suma de amounts cubre el total (ver load()).
	Payments []PaymentInput
}

type OrderView struct {
	ID           int64           `json:"id"`
	Number       int             `json:"number"`
	Status       string          `json:"status"`
	ServiceType  string          `json:"serviceType"`
	CustomerName *string         `json:"customerName"`
	Notes        *string         `json:"notes"`
	Subtotal     decimal.Decimal `json:"subtotal"`
	DeliveryFee  decimal.Decimal `json:"deliveryFee"`
	Total        decimal.Decimal `json:"total"`
	Currency     domain.Currency `json:"currency"`
	Paid         bool            `json:"paid"`
	OpenedAt     time.Time       `json:"openedAt"`
	Lines        []OrderLineView `json:"lines"`
}

type OrderLineView struct {
	ProductName string          `json:"productName"`
	Quantity    decimal.Decimal `json:"quantity"`
	UnitPrice   decimal.Decimal `json:"unitPrice"`
	LineTotal   decimal.Decimal `json:"lineTotal"`
	Notes       string          `json:"notes,omitempty"`
	Modifiers   []OrderModView  `json:"modifiers,omitempty"`
}

type OrderModView struct {
	Name       string          `json:"name"`
	Quantity   int             `json:"quantity"`
	PriceDelta decimal.Decimal `json:"priceDelta"`
}

func (s *OrdersService) Create(ctx context.Context, cmd CreateOrderCmd) (*OrderView, error) {
	if cmd.ClientUUID == uuid.Nil() {
		return nil, domain.ErrValidation
	}
	if !validServiceType(cmd.ServiceType) {
		return nil, domain.ErrValidation
	}
	// A04: acota cada línea de pago/propina antes de la tx. Un monto sobre el tope desbordaría
	// el numeric(10,2) (→ 500); una propina negativa violaría el check de la columna.
	// allowZero en la propina (0 es válido), no en el monto (una línea de pago cobra algo).
	for _, p := range cmd.Payments {
		if !domain.ValidMoney(domain.Round2(p.Amount), false) ||
			!domain.ValidMoney(domain.Round2(p.Tip), true) {
			return nil, domain.ErrValidation
		}
	}

	// idempotencia: si ya existe una orden con ese client_uuid, devolverla
	if id, err := s.store.QC(ctx).GetOrderIDByClientUUID(ctx, cmd.ClientUUID); err == nil {
		return s.load(ctx, id)
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}

	// Sin turno abierto no se cobra. Va ANTES de armar el pedido para no gastar consultas en algo
	// que se va a rechazar, y después de la idempotencia para que un reintento de una venta que ya
	// entró siga devolviéndola aunque entretanto se haya cerrado la caja.
	sess, err := s.store.QC(ctx).GetOpenPrimarySession(ctx)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNoOpenRegister
		}
		return nil, err
	}

	// cargar catálogo priceado (autoritativo)
	prodIDs, optIDs := collectIDs(cmd.Lines)
	prodRows, err := s.store.QC(ctx).GetPricedProducts(ctx, prodIDs)
	if err != nil {
		return nil, err
	}
	optRows, err := s.store.QC(ctx).GetPricedOptions(ctx, optIDs)
	if err != nil {
		return nil, err
	}
	products := map[int64]domain.PricedProduct{}
	for _, p := range prodRows {
		products[p.ID] = domain.PricedProduct{ID: p.ID, Name: p.Name, Price: p.Price, Cost: p.CurrentCost, Active: p.IsActive}
	}
	options := map[int64]domain.PricedOption{}
	for _, o := range optRows {
		options[o.ID] = domain.PricedOption{ID: o.ID, Name: o.Name, PriceDelta: o.PriceDelta, Cost: o.CurrentCost, GroupTitle: o.GroupTitle}
	}

	built, err := domain.BuildOrder(cmd.Lines, products, options)
	if err != nil {
		return nil, err
	}
	// El costo de envío solo aplica a domicilio; para el resto queda en 0 aunque el cliente lo mande.
	built, err = domain.ApplyDeliveryFee(built, cmd.DeliveryFee, cmd.ServiceType == "domicilio")
	if err != nil {
		return nil, err
	}

	// datos para depleción de stock (lectura antes de la tx)
	qtyByProduct := map[int64]decimal.Decimal{}
	for _, l := range built.Lines {
		qtyByProduct[l.ProductID] = qtyByProduct[l.ProductID].Add(l.Qty)
	}
	depletion, err := s.loadDepletion(ctx, prodIDs)
	if err != nil {
		return nil, err
	}

	bizDate := pgtype.Date{Time: s.now(), Valid: true}
	var orderID int64
	err = s.store.WithTx(ctx, func(q *db.Queries) error {
		num, err := q.NextDailyNumber(ctx, bizDate)
		if err != nil {
			return err
		}
		ord, err := q.CreateOrder(ctx, db.CreateOrderParams{
			ClientUuid:         cmd.ClientUUID,
			BusinessDate:       bizDate,
			DailyNumber:        num,
			ServiceType:        db.ServiceType(cmd.ServiceType),
			DeliveryPlatformID: cmd.DeliveryPlatformID,
			CustomerName:       cmd.CustomerName,
			Notes:              cmd.Notes,
			RegisterSessionID:  &sess.ID,
			OpenedBy:           cmd.OpenedBy,
			Subtotal:           built.Subtotal,
			Total:              built.Total,
			DeliveryFee:        built.DeliveryFee,
		})
		if err != nil {
			return err
		}
		orderID = ord.ID
		for _, l := range built.Lines {
			lineID, err := q.CreateOrderLine(ctx, db.CreateOrderLineParams{
				OrderID:        ord.ID,
				ProductID:      l.ProductID,
				ProductName:    l.ProductName,
				Quantity:       l.Qty,
				UnitPrice:      l.UnitPrice,
				ModifiersTotal: l.ModifiersTotal,
				UnitCost:       l.UnitCost,
				LineTotal:      l.LineTotal,
				Notes:          strPtr(l.Notes),
			})
			if err != nil {
				return err
			}
			for _, m := range l.Modifiers {
				if err := q.CreateOrderLineModifier(ctx, db.CreateOrderLineModifierParams{
					OrderLineID:      lineID,
					ModifierOptionID: m.OptionID,
					GroupTitle:       m.GroupTitle,
					OptionName:       m.OptionName,
					Quantity:         int16(m.Qty),
					PriceDelta:       m.PriceDelta,
					UnitCost:         m.UnitCost,
				}); err != nil {
					return err
				}
			}
		}
		// Pago dividido: una fila order_payments por método. La suma de amounts determina si
		// la orden queda pagada (load() la compara con el total).
		for _, p := range cmd.Payments {
			if !p.Amount.IsPositive() {
				continue // línea vacía: se ignora (no crea filas de $0)
			}
			if err := q.CreateOrderPayment(ctx, db.CreateOrderPaymentParams{
				OrderID:           ord.ID,
				PaymentMethodID:   p.MethodID,
				Amount:            domain.Round2(p.Amount),
				TipAmount:         domain.Round2(p.Tip),
				RegisterSessionID: &sess.ID,
				ReceivedBy:        &cmd.OpenedBy,
				Reference:         p.Reference,
			}); err != nil {
				return err
			}
		}

		// depleción de stock: producto con stock directo → descuenta el producto;
		// con receta → descuenta cada ingrediente × cantidad vendida (el trigger
		// mantiene stock_levels). Negativos permitidos (verdad contable).
		reason := "venta"
		for pid, qty := range qtyByProduct {
			if depletion.trackStock[pid] {
				if err := insertDepletion(ctx, q, movementIngredientOrProduct(ord.ID, cmd.OpenedBy, reason, "producto", nil, &pid, qty.Neg())); err != nil {
					return err
				}
				continue
			}
			for _, it := range depletion.recipe[pid] {
				ingID := it.ingredientID
				if err := insertDepletion(ctx, q, movementIngredientOrProduct(ord.ID, cmd.OpenedBy, reason, "ingrediente", &ingID, nil, it.qtyBase.Mul(qty).Neg())); err != nil {
					return err
				}
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return s.load(ctx, orderID)
}

func (s *OrdersService) load(ctx context.Context, id int64) (*OrderView, error) {
	o, err := s.store.QC(ctx).GetOrder(ctx, id)
	if err != nil {
		return nil, err
	}
	lines, err := s.store.QC(ctx).ListOrderLines(ctx, id)
	if err != nil {
		return nil, err
	}
	mods, err := s.store.QC(ctx).ListOrderLineModifiers(ctx, id)
	if err != nil {
		return nil, err
	}
	pays, err := s.store.QC(ctx).ListOrderPayments(ctx, id)
	if err != nil {
		return nil, err
	}
	modsByLine := map[int64][]OrderModView{}
	for _, m := range mods {
		modsByLine[m.OrderLineID] = append(modsByLine[m.OrderLineID], OrderModView{
			Name: m.OptionName, Quantity: int(m.Quantity), PriceDelta: m.PriceDelta,
		})
	}
	var paid decimal.Decimal
	for _, pmt := range pays {
		paid = paid.Add(pmt.Amount)
	}
	view := &OrderView{
		ID: o.ID, Number: int(o.DailyNumber), Status: string(o.Status),
		ServiceType: string(o.ServiceType), CustomerName: o.CustomerName, Notes: o.Notes,
		Subtotal: o.Subtotal, DeliveryFee: o.DeliveryFee, Total: o.Total, Currency: domain.Currency(o.Currency),
		Paid:     paid.GreaterThanOrEqual(o.Total) && o.Total.IsPositive(),
		OpenedAt: o.OpenedAt,
	}
	for _, l := range lines {
		view.Lines = append(view.Lines, OrderLineView{
			ProductName: l.ProductName, Quantity: l.Quantity, UnitPrice: l.UnitPrice,
			LineTotal: l.LineTotal, Notes: derefStr(l.Notes), Modifiers: modsByLine[l.ID],
		})
	}
	return view, nil
}

type BoardOrder struct {
	ID           int64           `json:"id"`
	Number       int             `json:"number"`
	Status       string          `json:"status"`
	ServiceType  string          `json:"serviceType"`
	CustomerName *string         `json:"customerName"`
	Total        decimal.Decimal `json:"total"`
	Currency     domain.Currency `json:"currency"`
	Paid         bool            `json:"paid"`
	OpenedAt     time.Time       `json:"openedAt"`
}

// Board devuelve las órdenes activas (abierta/lista) para el tablero.
func (s *OrdersService) Board(ctx context.Context) ([]BoardOrder, error) {
	rows, err := s.store.QC(ctx).ListActiveOrders(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]BoardOrder, 0, len(rows))
	for _, r := range rows {
		out = append(out, BoardOrder{
			ID: r.ID, Number: int(r.DailyNumber), Status: string(r.Status),
			ServiceType: string(r.ServiceType), CustomerName: r.CustomerName,
			Total: r.Total, Currency: domain.Currency(r.Currency),
			Paid: r.Paid.GreaterThanOrEqual(r.Total) && r.Total.IsPositive(), OpenedAt: r.OpenedAt,
		})
	}
	return out, nil
}

// DeliveredToday lista las órdenes entregadas del día (para la sección de reembolsos).
func (s *OrdersService) DeliveredToday(ctx context.Context) ([]BoardOrder, error) {
	rows, err := s.store.QC(ctx).ListDeliveredToday(ctx, pgtype.Date{Time: s.now(), Valid: true})
	if err != nil {
		return nil, err
	}
	out := make([]BoardOrder, 0, len(rows))
	for _, r := range rows {
		out = append(out, BoardOrder{
			ID: r.ID, Number: int(r.DailyNumber), Status: string(r.Status),
			ServiceType: string(r.ServiceType), CustomerName: r.CustomerName,
			Total: r.Total, Currency: domain.Currency(r.Currency),
			Paid: r.Paid.GreaterThanOrEqual(r.Total) && r.Total.IsPositive(), OpenedAt: r.OpenedAt,
		})
	}
	return out, nil
}

// Detail carga una orden completa.
func (s *OrdersService) Detail(ctx context.Context, id int64) (*OrderView, error) {
	return s.load(ctx, id)
}

// SetStatus avanza el estado de una orden (lista / entregada), respetando la
// máquina de estados: no retrocede ni toca órdenes terminales.
func (s *OrdersService) SetStatus(ctx context.Context, id int64, status string) error {
	if status != domain.StatusLista && status != domain.StatusEntregada {
		return domain.ErrValidation
	}
	return s.store.WithTx(ctx, func(q *db.Queries) error {
		o, err := q.GetOrder(ctx, id)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return domain.ErrNotFound
			}
			return err
		}
		// mismo estado = no-op idempotente: un doble-tap en el tablero no debe dar error
		// (mantra: nunca hacer que el operador deshaga para rehacer).
		if string(o.Status) == status {
			return nil
		}
		if !domain.CanTransition(string(o.Status), status) {
			return domain.ErrConflict
		}
		return q.SetOrderStatus(ctx, db.SetOrderStatusParams{ID: id, Status: db.OrderStatus(status)})
	})
}

// Cancel cancela una orden (razón obligatoria) y repone el stock descontado.
// Idempotente: si ya está cancelada (o entregada) rechaza con ErrConflict, así un
// doble-tap no duplica los movimientos de reposición.
// ponytail: el guard se lee dentro de la tx con GetOrder (no FOR UPDATE); cubre el
// caso real (doble-tap secuencial). Si dos cancels concurren, añade SELECT ... FOR
// UPDATE en una query dedicada cuando exista sqlc en el toolchain.
func (s *OrdersService) Cancel(ctx context.Context, id int64, actor int64, reason string) error {
	if reason == "" {
		return domain.ErrValidation
	}
	return s.store.WithTx(ctx, func(q *db.Queries) error {
		o, err := q.GetOrder(ctx, id)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return domain.ErrNotFound
			}
			return err
		}
		if !domain.CanTransition(string(o.Status), domain.StatusCancelada) {
			return domain.ErrConflict
		}
		if err := q.CancelOrder(ctx, db.CancelOrderParams{ID: id, CancelledBy: &actor, CancelReason: &reason}); err != nil {
			return err
		}
		return q.RestockCancelledOrder(ctx, db.RestockCancelledOrderParams{Oid: &id, ActorID: &actor})
	})
}

// Refund reembolsa una orden YA entregada (razón obligatoria). A diferencia de Cancel, NO
// repone stock: la mercancía se hizo y se entregó, así que su costo ya consumido ES la
// pérdida de inventario; solo se revierte el ingreso marcando la orden 'reembolsada' con el
// monto devuelto (el total). Idempotente: si no está entregada (ya reembolsada, cancelada,
// abierta…) rechaza con ErrConflict, así un doble-tap no reembolsa dos veces.
func (s *OrdersService) Refund(ctx context.Context, id, actor int64, reason string) error {
	if reason == "" {
		return domain.ErrValidation
	}
	return s.store.WithTx(ctx, func(q *db.Queries) error {
		o, err := q.GetOrder(ctx, id)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return domain.ErrNotFound
			}
			return err
		}
		if !domain.CanRefund(string(o.Status)) {
			return domain.ErrConflict
		}
		return q.RefundOrder(ctx, db.RefundOrderParams{
			ID: id, RefundedBy: &actor, RefundReason: &reason, RefundAmount: o.Total,
		})
	})
}

type depletionData struct {
	trackStock map[int64]bool
	recipe     map[int64][]recipeDelta
}
type recipeDelta struct {
	ingredientID int64
	qtyBase      decimal.Decimal
}

func (s *OrdersService) loadDepletion(ctx context.Context, prodIDs []int64) (depletionData, error) {
	d := depletionData{trackStock: map[int64]bool{}, recipe: map[int64][]recipeDelta{}}
	tracked, err := s.store.QC(ctx).GetTrackStockProductIDs(ctx, prodIDs)
	if err != nil {
		return d, err
	}
	for _, id := range tracked {
		d.trackStock[id] = true
	}
	rows, err := s.store.QC(ctx).GetRecipeDepletion(ctx, prodIDs)
	if err != nil {
		return d, err
	}
	for _, r := range rows {
		d.recipe[r.ProductID] = append(d.recipe[r.ProductID], recipeDelta{ingredientID: r.IngredientID, qtyBase: r.QtyBase})
	}
	return d, nil
}

func movementIngredientOrProduct(orderID, userID int64, reason, itemType string, ingID, prodID *int64, qty decimal.Decimal) db.InsertStockMovementParams {
	return db.InsertStockMovementParams{
		ItemType:     db.StockItemType(itemType),
		IngredientID: ingID,
		ProductID:    prodID,
		MovementType: db.StockMovementType("venta"),
		Quantity:     qty, // insertDepletion redondea (4dp) y valida
		OrderID:      &orderID,
		UserID:       &userID,
		Reason:       &reason,
	}
}

// insertDepletion registra un movimiento de venta acotando el delta a numeric(14,4): un
// delta que redondea a 0 (depleción despreciable) se omite —un movimiento 0 viola el check
// de la columna—; uno fuera de rango (pedido abusivo: muchas líneas de un producto $0 que
// esquiva el tope del total) es 400, no un overflow del numeric → 500.
func insertDepletion(ctx context.Context, q *db.Queries, p db.InsertStockMovementParams) error {
	p.Quantity = domain.Round4(p.Quantity)
	if p.Quantity.IsZero() {
		return nil
	}
	if !domain.ValidQty(p.Quantity, domain.MaxStockQty, true) {
		return domain.ErrValidation
	}
	return q.InsertStockMovement(ctx, p)
}

func collectIDs(lines []domain.OrderLineInput) ([]int64, []int64) {
	pset := map[int64]bool{}
	oset := map[int64]bool{}
	for _, l := range lines {
		pset[l.ProductID] = true
		for _, m := range l.Modifiers {
			oset[m.OptionID] = true
		}
	}
	return keys(pset), keys(oset)
}

func keys(m map[int64]bool) []int64 {
	out := make([]int64, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func validServiceType(s string) bool {
	return s == "mostrador" || s == "para_llevar" || s == "domicilio"
}

func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
func derefStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
