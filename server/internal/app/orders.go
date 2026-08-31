package app

import (
	"context"
	"errors"
	"fmt"
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
	// Delivered: el pedido se cobró y se entregó en el mismo acto, así que nace ENTREGADO y no
	// pasa por el tablero. Es el refresco de mostrador: nada va a cocina y el cliente se va con él
	// en la mano. Exige que los pagos cubran el total — entregar sin cobrar sería regalar comida
	// sin dejar rastro, porque el pedido nace terminado y no vuelve a aparecer en ninguna pantalla
	// operativa.
	Delivered bool
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

	// La lista de precios de la venta, ANTES que los métodos de pago: si la plataforma no es de
	// esta empresa hay que decir eso y no "el método no corresponde a la plataforma", que compara
	// contra una plataforma que no existe y manda al operador a revisar lo que no es.
	//
	// Sin plataforma es la de mostrador (margen 0, sin excepciones): el caso de todos los días, que
	// no toca la base ni una vez más.
	lista, err := s.listaDePrecios(ctx, cmd.DeliveryPlatformID)
	if err != nil {
		return nil, err
	}

	// Cada método de pago se resuelve BAJO RLS. La llave foránea no alcanza: los chequeos de
	// integridad referencial de Postgres saltan RLS por diseño, así que el id de otra empresa
	// entraría sin protestar. El daño sería silencioso — el corte hace join con payment_methods
	// bajo RLS, así que ese pago no saldría en ningún renglón, desaparecería del reporte de ventas,
	// y el cajero encontraría un faltante por el monto exacto sin nada que lo explique.
	//
	// Se deduplica porque un pago dividido repite métodos y no vale la pena consultar dos veces.
	vistos := map[int16]bool{}
	for _, p := range cmd.Payments {
		if vistos[p.MethodID] {
			continue
		}
		vistos[p.MethodID] = true
		m, err := s.store.QC(ctx).GetPaymentMethod(ctx, p.MethodID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil, domain.ErrNotFound
			}
			return nil, err
		}
		if !domain.MetodoCorrespondeALaPlataforma(m.DeliveryPlatformID, cmd.DeliveryPlatformID) {
			return nil, domain.ErrPaymentMethodPlatform
		}
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
		products[p.ID] = domain.PricedProduct{
			ID: p.ID, Name: p.Name, Cost: p.CurrentCost, Active: p.IsActive,
			// El costo NO lleva margen: el margen es de precio de VENTA. Vender por Uber consume
			// exactamente el mismo inventario, y el margen extra es lo que se va en comisión.
			Price: domain.PlatformPrice(p.Price, lista.margen, lista.producto[p.ID]),
		}
	}
	options := map[int64]domain.PricedOption{}
	for _, o := range optRows {
		options[o.ID] = domain.PricedOption{
			ID: o.ID, Name: o.Name, Cost: o.CurrentCost, GroupTitle: o.GroupTitle,
			MaxPerLine: int(o.MaxPerLine),
			PriceDelta: domain.PlatformPrice(o.PriceDelta, lista.margen, lista.opcion[o.ID]),
		}
	}

	built, err := domain.BuildOrder(cmd.Lines, products, options)
	if err != nil {
		return nil, err
	}
	// Entregar en el acto exige que la venta quede saldada. Se valida aquí, contra el total que el
	// SERVIDOR calculó, y no en la pantalla: el pedido nace terminado y ya no aparece en ninguna
	// pantalla operativa, así que un faltante solo se vería hasta el corte.
	if cmd.Delivered && !domain.PagosCubren(cmd.pagado(), built.Total.Add(built.DeliveryFee)) {
		return nil, fmt.Errorf("%w: no se puede entregar un pedido que no se cobró completo", domain.ErrValidation)
	}
	// El costo de envío solo aplica a domicilio Y sin plataforma: el reparto de Uber/DiDi/Rappi lo
	// cobra la plataforma, así que sumarle el envío del negocio le carga $20 de más a cada pedido.
	cobraEnvio := cmd.ServiceType == "domicilio" && cmd.DeliveryPlatformID == nil
	built, err = domain.ApplyDeliveryFee(built, cmd.DeliveryFee, cobraEnvio)
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

	// La fecha de negocio la HEREDA del turno, no la recalcula. Dos razones:
	//
	// 1. El turno ya la resolvió en la zona del local, así que la venta no vuelve a consultarla.
	// 2. Un turno que cruza la medianoche (abre 11pm, cierra 3am) numera corrido en vez de
	//    partirse: recalcular por reloj reiniciaba el folio a mitad del turno y dejaba dos
	//    tickets #1 en la misma noche.
	bizDate := sess.BusinessDate
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
			Status:             estadoInicial(cmd.Delivered),
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

// listaDePrecios: el margen de la plataforma y sus excepciones capturadas. Sin plataforma devuelve
// la lista de mostrador — margen 0 y sin excepciones — para que el camino de todos los días no
// gaste consultas.
type listaDePrecios struct {
	margen   decimal.Decimal
	producto map[int64]*decimal.Decimal
	opcion   map[int64]*decimal.Decimal
}

func (s *OrdersService) listaDePrecios(ctx context.Context, platformID *int16) (listaDePrecios, error) {
	lista := listaDePrecios{
		producto: map[int64]*decimal.Decimal{},
		opcion:   map[int64]*decimal.Decimal{},
	}
	if platformID == nil {
		return lista, nil
	}
	// BAJO RLS y con rechazo explícito: los chequeos de llave foránea de Postgres saltan RLS, así
	// que un id de otra empresa pasaría el insert. Si aquí se cayera a margen 0, la venta se
	// cobraría a precio de mostrador en Uber con el ticket bien impreso, y el descuadre aparecería
	// semanas después al conciliar el depósito.
	plat, err := s.store.QC(ctx).GetPlatformByID(ctx, *platformID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return lista, domain.ErrPlatformNotFound
		}
		return lista, err
	}
	lista.margen = plat.PriceMarkupPct

	precios, err := s.store.QC(ctx).GetProductPlatformPrices(ctx, *platformID)
	if err != nil {
		return lista, err
	}
	for _, p := range precios {
		precio := p.Price
		lista.producto[p.ProductID] = &precio
	}
	deltas, err := s.store.QC(ctx).GetOptionPlatformPrices(ctx, *platformID)
	if err != nil {
		return lista, err
	}
	for _, d := range deltas {
		delta := d.PriceDelta
		lista.opcion[d.OptionID] = &delta
	}
	return lista, nil
}

// pagado suma lo que traen las líneas de pago del comando.
func (c CreateOrderCmd) pagado() decimal.Decimal {
	total := decimal.Zero
	for _, p := range c.Payments {
		if p.Amount.IsPositive() {
			total = total.Add(p.Amount)
		}
	}
	return total
}

// estadoInicial: entregado en el acto, o abierto para que pase por el tablero.
func estadoInicial(entregado bool) db.OrderStatus {
	if entregado {
		return db.OrderStatus(domain.StatusEntregada)
	}
	return db.OrderStatus(domain.StatusAbierta)
}

// AddLines agrega renglones a un pedido que sigue en curso.
//
// Es el caso de todos los días: la libreta vuelve de la mesa con "la 3 pidió dos más". Sin esto, la
// única salida era abrir un segundo pedido —dos folios y dos tickets para el mismo cliente, y el
// corte contando dos ventas donde hubo una— o cancelar y rehacer.
//
// Tres cuidados, cada uno por un fallo distinto:
//
//   - Solo a pedidos en curso. Uno entregado ya tiene su venta en el corte y su ticket en manos del
//     cliente; cambiarle el total después es mover dinero que ya se contó.
//   - El stock se descuenta SOLO de lo nuevo. Recalcularlo sobre el pedido entero volvería a
//     descontar lo que ya se descontó, y el inventario se iría al piso sin explicación.
//   - El total se recalcula desde los renglones GUARDADOS, en la base. Rearmarlo desde el comando
//     obligaría a re-precisar lo viejo con la lista de precios de hoy, y un pedido de ayer
//     cambiaría de precio por agregarle un café.
func (s *OrdersService) AddLines(ctx context.Context, orderID int64, lines []domain.OrderLineInput, actor int64) (*OrderView, error) {
	if len(lines) == 0 {
		return nil, fmt.Errorf("%w: no hay nada que agregar", domain.ErrValidation)
	}

	ord, err := s.store.QC(ctx).GetOrder(ctx, orderID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	if !domain.PuedeRecibirLineas(string(ord.Status)) {
		return nil, fmt.Errorf("%w: el pedido #%d ya está %s y no admite más renglones",
			domain.ErrConflict, ord.DailyNumber, ord.Status)
	}

	// La lista de precios es la del PEDIDO, no la de la pantalla: un agregado a un pedido de Uber se
	// cobra con los precios de Uber aunque quien captura tenga el mostrador seleccionado.
	lista, err := s.listaDePrecios(ctx, ord.DeliveryPlatformID)
	if err != nil {
		return nil, err
	}

	prodIDs, optIDs := collectIDs(lines)
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
		products[p.ID] = domain.PricedProduct{
			ID: p.ID, Name: p.Name, Cost: p.CurrentCost, Active: p.IsActive,
			Price: domain.PlatformPrice(p.Price, lista.margen, lista.producto[p.ID]),
		}
	}
	options := map[int64]domain.PricedOption{}
	for _, o := range optRows {
		options[o.ID] = domain.PricedOption{
			ID: o.ID, Name: o.Name, Cost: o.CurrentCost, GroupTitle: o.GroupTitle,
			MaxPerLine: int(o.MaxPerLine),
			PriceDelta: domain.PlatformPrice(o.PriceDelta, lista.margen, lista.opcion[o.ID]),
		}
	}

	// Se valúa SOLO lo nuevo: es lo que se va a insertar y lo único de lo que se descuenta stock.
	built, err := domain.BuildOrder(lines, products, options)
	if err != nil {
		return nil, err
	}

	qtyByProduct := map[int64]decimal.Decimal{}
	for _, l := range built.Lines {
		qtyByProduct[l.ProductID] = qtyByProduct[l.ProductID].Add(l.Qty)
	}
	depletion, err := s.loadDepletion(ctx, prodIDs)
	if err != nil {
		return nil, err
	}

	err = s.store.WithTx(ctx, func(q *db.Queries) error {
		// Bloquea el pedido: dos capturas simultáneas sobre la misma cuenta recalcularían el total
		// sobre el estado viejo y uno de los dos agregados desaparecería del importe.
		if _, err := q.GetOrderForUpdate(ctx, orderID); err != nil {
			return err
		}
		for _, l := range built.Lines {
			lineID, err := q.CreateOrderLine(ctx, db.CreateOrderLineParams{
				OrderID:        orderID,
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
		for pid, qty := range qtyByProduct {
			if depletion.trackStock[pid] {
				if err := insertDepletion(ctx, q, movementIngredientOrProduct(orderID, actor, "venta", "producto", nil, &pid, qty.Neg())); err != nil {
					return err
				}
				continue
			}
			for _, it := range depletion.recipe[pid] {
				ingID := it.ingredientID
				if err := insertDepletion(ctx, q, movementIngredientOrProduct(orderID, actor, "venta", "ingrediente", &ingID, nil, it.qtyBase.Mul(qty).Neg())); err != nil {
					return err
				}
			}
		}
		return q.RecalcOrderTotals(ctx, orderID)
	})
	if err != nil {
		return nil, err
	}

	return s.load(ctx, orderID)
}
