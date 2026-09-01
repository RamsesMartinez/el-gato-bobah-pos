package app

import (
	"context"
	"errors"
	"fmt"
	"slices"
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
	// CompanyID identifica al negocio; solo se usa para barajar los nombres de folio, de modo que
	// dos locales de la misma cadena no canten "Tigre" a la misma hora.
	CompanyID int64
	// FolioName es el nombre que la pantalla ya le puso a la cuenta. Viene del cliente para que el
	// operador vea el mismo nombre desde que agrega el primer producto hasta que imprime el
	// ticket; el servidor lo sanea y resuelve los choques, así que proponerlo no es decidirlo.
	// Vacío = que lo reparta el servidor (clientes de API, tests).
	FolioName string
}

type OrderView struct {
	// Agregados: los ids de los renglones que ACABAN de entrar, para que la estación imprima la
	// comanda del agregado sin volver a preguntar cuáles eran. Vacío en cualquier otra respuesta.
	//
	// Viaja en la respuesta y no se deduce comparando contra lo que la pantalla tenía: dos
	// estaciones pueden estar agregando al mismo pedido, y la diferencia contra el estado local
	// incluiría lo que agregó la otra — cocina prepararía dos veces lo que el compañero ya mandó.
	Agregados []int64 `json:"agregados,omitempty"`
	ID        int64   `json:"id"`
	Number    int     `json:"number"`
	// FolioName es el nombre con el que se canta el pedido ("Tigre"). Vacío en los pedidos
	// anteriores a que existieran: a esos no se les inventa uno, porque el ticket que se imprimió
	// en su día llevaba solo el número.
	FolioName    string          `json:"folioName"`
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
	ID          int64           `json:"id"`
	ProductName string          `json:"productName"`
	Quantity    decimal.Decimal `json:"quantity"`
	// Delivered: cuánto de este renglón ya se le dio al cliente. Es cantidad y no un booleano
	// porque la comida sale por tandas: de cinco alitas salen tres y dos siguen en la freidora.
	Delivered decimal.Decimal `json:"delivered"`
	Cancelled bool            `json:"cancelled"`
	UnitPrice decimal.Decimal `json:"unitPrice"`
	LineTotal decimal.Decimal `json:"lineTotal"`
	Notes     string          `json:"notes,omitempty"`
	Modifiers []OrderModView  `json:"modifiers,omitempty"`
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
	// Crear un pedido YA COBRADO se rechaza: era el camino corto que se saltaba la cocina por
	// completo, y por ser el corto era el que se usaba. Confirmar y cobrar son dos momentos, y el
	// segundo entra por Charge.
	//
	// La barrera vive AQUÍ y no en la pantalla. Esconder el botón deja el endpoint abierto a
	// cualquiera con una petición a mano, y el front es espejo del backend, nunca la barrera.
	if len(cmd.Payments) > 0 {
		return nil, domain.ErrCobroFueraDeLugar
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

	// Si algo del pedido necesita prepararse. Lo dice el CATÁLOGO, no la pantalla: preguntárselo al
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

	// El pedido SIEMPRE nace abierto. Nacía entregado cuando no había nada que preparar y la venta
	// quedaba saldada, pero eso dependía de que crear y cobrar fueran una sola llamada — y esa vía
	// se cerró para que cocina vea todo. La regla vive ahora en Charge, que es donde ocurre.
	var orderID int64
	err = s.store.WithTx(ctx, func(q *db.Queries) error {
		num, err := q.NextDailyNumber(ctx, bizDate)
		if err != nil {
			return err
		}
		folio, err := resolverFolio(ctx, q, cmd, bizDate, int(num))
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
			FolioName:          strPtr(folio),
			Status:             db.OrderStatusAbierta,
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
				NaceEntregada:  false,
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
		// El pedido nace con todos sus renglones ya en cocina: la comanda del confirmado sale con el
		// pedido completo. Sin marcarlos, el primer agregado sacaría otra vez el pedido entero y
		// cocina prepararía dos veces lo que ya tenía en la plancha.
		if err := q.MarcarTodoElPedidoEnviadoACocina(ctx, ord.ID); err != nil {
			return err
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
		// Un id que no existe es NO ENCONTRADO, no un error interno: el `pgx.ErrNoRows` crudo no lo
		// reconoce `httpapi.Error` y sale como 500, que dice "el servidor se rompió" y manda a
		// revisar logs por un pedido que simplemente no está.
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
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
		FolioName: derefStr(o.FolioName),
		ID:        o.ID, Number: int(o.DailyNumber), Status: string(o.Status),
		ServiceType: string(o.ServiceType), CustomerName: o.CustomerName, Notes: o.Notes,
		Subtotal: o.Subtotal, DeliveryFee: o.DeliveryFee, Total: o.Total, Currency: domain.Currency(o.Currency),
		Paid:     paid.GreaterThanOrEqual(o.Total) && o.Total.IsPositive(),
		OpenedAt: o.OpenedAt,
	}
	for _, l := range lines {
		view.Lines = append(view.Lines, OrderLineView{
			ID: l.ID, ProductName: l.ProductName, Quantity: l.Quantity,
			Delivered: l.DeliveredQty, Cancelled: l.CancelledAt.Valid,
			UnitPrice: l.UnitPrice, LineTotal: l.LineTotal,
			Notes: derefStr(l.Notes), Modifiers: modsByLine[l.ID],
		})
	}
	return view, nil
}

type BoardOrder struct {
	ID          int64  `json:"id"`
	Number      int    `json:"number"`
	FolioName   string `json:"folioName"`
	Status      string `json:"status"`
	ServiceType string `json:"serviceType"`
	// DeliveryPlatformID deja que el tablero ofrezca solo los métodos con los que ese pedido se
	// puede cobrar. Sin él, cobrar un pedido de Uber con el efectivo del mostrador hace que el
	// sistema espere en el cajón billetes que la plataforma pagó por transferencia.
	DeliveryPlatformID *int16          `json:"deliveryPlatformId"`
	CustomerName       *string         `json:"customerName"`
	Total              decimal.Decimal `json:"total"`
	Currency           domain.Currency `json:"currency"`
	Paid               bool            `json:"paid"`
	// Outstanding es lo que falta por cobrar. Viaja aparte de Paid porque un pedido puede estar
	// ABONADO —el cliente dejó algo al pedir y termina al recoger—, y en ese caso derivar el
	// pendiente del total, como hacía la pantalla, cobra de más y descuadra el aviso del tablero.
	Outstanding decimal.Decimal `json:"outstanding"`
	OpenedAt    time.Time       `json:"openedAt"`
	// EnPreparacion: si a este pedido todavía se le puede AGREGAR. Viaja como dato y no se deduce
	// del estado en la pantalla, para que la regla no quede implementada en dos lados y se separen.
	EnPreparacion bool `json:"enPreparacion"`
	// Renglones vivos, para que el chip diga de un vistazo qué tan grande es el pedido sin traerse
	// la lista entera de cada uno.
	Renglones int `json:"renglones"`
	// Los renglones vivos con lo que falta de cada uno. El tablero los pinta desplegados: lo que
	// falta por entregar ES lo que el operador vino a leer, no algo que deba destapar con un tap.
	// Vacío en las entregadas, que ya no tienen nada pendiente.
	Lines []BoardLine `json:"lines"`
}

// BoardLine es un renglón visto desde el tablero. No trae precio: entregar no mueve dinero, y en
// una pantalla de 600 px una columna que no se usa le quita renglones a la que sí.
type BoardLine struct {
	ID        int64           `json:"id"`
	Name      string          `json:"name"`
	Qty       decimal.Decimal `json:"qty"`
	Delivered decimal.Decimal `json:"delivered"`
	// Notes y Modifiers son lo que vuelve utilizable el tablero en una cocina: "Alitas" y "Alitas
	// BBQ sin cebolla" son platillos distintos.
	Notes     string   `json:"notes,omitempty"`
	Modifiers []string `json:"modifiers,omitempty"`
}

// Board devuelve las órdenes activas (abierta/lista) para el tablero.
func (s *OrdersService) Board(ctx context.Context) ([]BoardOrder, error) {
	rows, err := s.store.QC(ctx).ListActiveOrders(ctx)
	if err != nil {
		return nil, err
	}
	porPedido, err := s.lineasDelTablero(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]BoardOrder, 0, len(rows))
	for _, r := range rows {
		out = append(out, BoardOrder{
			ID: r.ID, Number: int(r.DailyNumber), FolioName: derefStr(r.FolioName),
			Status:      string(r.Status),
			ServiceType: string(r.ServiceType), DeliveryPlatformID: r.DeliveryPlatformID,
			CustomerName: r.CustomerName,
			Total:        r.Total, Currency: domain.Currency(r.Currency),
			Paid:        r.Paid.GreaterThanOrEqual(r.Total) && r.Total.IsPositive(),
			Outstanding: domain.PorCobrar(r.Total, r.Paid),
			OpenedAt:    r.OpenedAt,
			Lines:       porPedido[r.ID],
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
			ID: r.ID, Number: int(r.DailyNumber), FolioName: derefStr(r.FolioName),
			Status:      string(r.Status),
			ServiceType: string(r.ServiceType), DeliveryPlatformID: r.DeliveryPlatformID,
			CustomerName: r.CustomerName,
			Total:        r.Total, Currency: domain.Currency(r.Currency),
			Paid:        r.Paid.GreaterThanOrEqual(r.Total) && r.Total.IsPositive(),
			Outstanding: domain.PorCobrar(r.Total, r.Paid),
			OpenedAt:    r.OpenedAt,
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
		// Cancelar repone el stock de TODAS las líneas, así que un pedido del que ya salió comida
		// no se puede cancelar: reponer lo que el cliente se llevó le inventaría al almacén
		// existencias que no están. Lo que queda por hacer se cancela renglón a renglón; lo que ya
		// se entregó se reembolsa.
		lineas, err := lineasDeEntrega(ctx, q, id)
		if err != nil {
			return err
		}
		if domain.HayEntregaParcial(lineas) {
			return domain.ErrCancelarConEntregas
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

	// Los renglones que entran en ESTA llamada. Es lo que la comanda del agregado imprime, y por eso
	// se recogen aquí y no se deducen después comparando contra lo que la pantalla tenía: dos
	// estaciones pueden estar agregando al mismo pedido, y esa diferencia incluiría lo que agregó la
	// otra — cocina prepararía dos veces lo que el compañero ya mandó.
	var agregados []int64

	err = s.store.WithTx(ctx, func(q *db.Queries) error {
		// Bloquea el pedido: dos capturas simultáneas sobre la misma cuenta recalcularían el total
		// sobre el estado viejo y uno de los dos agregados desaparecería del importe.
		o, err := q.GetOrderForUpdate(ctx, orderID)
		if err != nil {
			return err
		}
		// Y se REVALIDA el estado sobre la fila ya bloqueada, no solo sobre la lectura de arriba.
		//
		// Entre las dos cabe que la otra estación entregue o cancele el pedido, y con la barra de
		// pedidos en curso ese hueco dejó de ser teórico: el chip sigue en pantalla hasta el
		// siguiente refresco, y ahora un cobro que salda un pedido sin preparación lo entrega SOLO,
		// que es el caso más común del mostrador. Sin esto entra un renglón sobre un pedido ya
		// entregado y `RecalcOrderTotals` le sube el total — mover dinero que ya se contó, que es
		// justo lo que el comentario de esta función dice que nunca debe pasar.
		if !domain.PuedeRecibirLineas(string(o.Status)) {
			return fmt.Errorf("%w: el pedido #%d ya está %s y no admite más renglones",
				domain.ErrConflict, ord.DailyNumber, o.Status)
		}
		agregados = agregados[:0]
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
			agregados = append(agregados, lineID)
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
		// Marcados como salidos a cocina EN LA MISMA transacción que los inserta: si se marcaran
		// después y la petición muriera en medio, quedarían renglones que la comanda del agregado ya
		// no volvería a considerar y cocina nunca sabría de ellos.
		if err := q.MarcarRenglonesEnviadosACocina(ctx, db.MarcarRenglonesEnviadosACocinaParams{
			OrderID: orderID, Ids: agregados,
		}); err != nil {
			return err
		}
		return q.RecalcOrderTotals(ctx, orderID)
	})
	if err != nil {
		return nil, err
	}

	vista, err := s.load(ctx, orderID)
	if err != nil {
		return nil, err
	}
	vista.Agregados = agregados
	return vista, nil
}

// lineasDeEntrega traduce los renglones del pedido a lo que el dominio necesita para razonar sobre
// su entrega. Bloquea las filas: de esto cuelga el cierre automático del pedido, y dos personas
// marcando renglones a la vez podrían dejarlo abierto con todo entregado.
func lineasDeEntrega(ctx context.Context, q *db.Queries, orderID int64) ([]domain.LineaEntrega, error) {
	rows, err := q.ListLinesForDelivery(ctx, orderID)
	if err != nil {
		return nil, err
	}
	out := make([]domain.LineaEntrega, 0, len(rows))
	for _, r := range rows {
		out = append(out, domain.LineaEntrega{
			ID:        r.ID,
			Cantidad:  r.Quantity,
			Entregado: r.DeliveredQty,
			Cancelada: r.CancelledAt.Valid,
		})
	}
	return out, nil
}

// DeliverLine registra que se le dio al cliente `cantidad` de un renglón.
//
// Existe con cantidad —y no como un "listo/no listo"— porque en un pedido grande la comida sale
// por tandas: de cinco alitas salen tres y las otras dos siguen en la freidora. Con un booleano el
// operador tendría que elegir entre mentir y olvidar lo que sí entregó.
func (s *OrdersService) DeliverLine(ctx context.Context, orderID, lineID int64, cantidad decimal.Decimal) error {
	return s.store.WithTx(ctx, func(q *db.Queries) error {
		o, err := q.GetOrderForUpdate(ctx, orderID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return domain.ErrNotFound
			}
			return err
		}
		if !domain.PuedeRecibirLineas(string(o.Status)) {
			return fmt.Errorf("%w: este pedido ya está cerrado", domain.ErrConflict)
		}

		lineas, err := lineasDeEntrega(ctx, q, orderID)
		if err != nil {
			return err
		}
		i := slices.IndexFunc(lineas, func(l domain.LineaEntrega) bool { return l.ID == lineID })
		if i < 0 {
			return domain.ErrNotFound
		}
		if err := domain.ValidarEntrega(lineas[i], cantidad); err != nil {
			return err
		}

		// La base repite el tope que el dominio ya validó: entre leer y escribir cabe otra
		// transacción entregando lo mismo. Si no tocó ninguna fila, eso fue lo que pasó.
		n, err := q.DeliverOrderLine(ctx, db.DeliverOrderLineParams{
			LineID: lineID, OrderID: orderID, Cantidad: cantidad,
		})
		if err != nil {
			return err
		}
		if n == 0 {
			return fmt.Errorf("%w: alguien más entregó ese producto mientras tanto", domain.ErrConflict)
		}

		lineas[i].Entregado = lineas[i].Entregado.Add(cantidad)
		return cerrarSiYaSeEntregoTodo(ctx, q, orderID, lineas)
	})
}

// DeliverAll marca el pedido completo como entregado, que es el caso común y el de un solo tap.
// Marca también sus renglones: si quedaran en desacuerdo, la pantalla mostraría comida pendiente
// de un pedido ya cerrado y nadie sabría cuál de los dos datos creer.
func (s *OrdersService) DeliverAll(ctx context.Context, orderID int64) error {
	return s.store.WithTx(ctx, func(q *db.Queries) error {
		o, err := q.GetOrderForUpdate(ctx, orderID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return domain.ErrNotFound
			}
			return err
		}
		if !domain.CanTransition(string(o.Status), domain.StatusEntregada) {
			return domain.ErrConflict
		}
		if err := q.DeliverAllOrderLines(ctx, orderID); err != nil {
			return err
		}
		return q.SetOrderStatus(ctx, db.SetOrderStatusParams{ID: orderID, Status: db.OrderStatusEntregada})
	})
}

// cerrarSiYaSeEntregoTodo cierra el pedido cuando ya no le falta nada por entregar.
//
// Lo hace el servidor y no el operador: obligarlo a marcar el último renglón y además el pedido
// es pedirle dos veces lo mismo, y la segunda es la que se olvida — el pedido se quedaría abierto
// toda la tarde con la comida ya entregada, y el cierre de caja lo reclamaría al final del turno.
func cerrarSiYaSeEntregoTodo(ctx context.Context, q *db.Queries, orderID int64, lineas []domain.LineaEntrega) error {
	if !domain.TodoEntregado(lineas) {
		return nil
	}
	return q.SetOrderStatus(ctx, db.SetOrderStatusParams{ID: orderID, Status: db.OrderStatusEntregada})
}

// resolverFolio decide con qué nombre se canta el pedido.
//
// Gana el que propuso la pantalla, porque es el que el operador lleva viendo desde que abrió la
// cuenta y el que ya le dijo al cliente; el servidor solo lo sanea y le agrega la vuelta si otro
// pedido del día se le adelantó. Sin propuesta —clientes de API, tests— reparte el suyo.
func resolverFolio(ctx context.Context, q *db.Queries, cmd CreateOrderCmd, bizDate pgtype.Date, num int) (string, error) {
	propuesto := domain.SanitizarFolio(cmd.FolioName)
	if propuesto == "" {
		return domain.NombreDeFolio(cmd.CompanyID, bizDate.Time, num), nil
	}
	usados, err := q.FolioNamesUsedToday(ctx, bizDate)
	if err != nil {
		return "", err
	}
	nombres := make([]string, 0, len(usados))
	for _, u := range usados {
		if u != nil {
			nombres = append(nombres, *u)
		}
	}
	// Con las 100 vueltas agotadas del mismo animal cae al nombre del servidor, que sale del folio
	// numérico y por lo tanto no puede chocar.
	if libre := domain.SiguienteFolioLibre(propuesto, nombres); libre != "" {
		return libre, nil
	}
	return domain.NombreDeFolio(cmd.CompanyID, bizDate.Time, num), nil
}

// lineasDelTablero trae los renglones de TODOS los pedidos activos en dos consultas y los agrupa.
//
// Dos consultas y no una por tarjeta: el tablero se refresca solo cada diez segundos, así que
// pedir el detalle pedido por pedido serían N peticiones cada diez segundos en la pantalla que la
// cocina deja abierta todo el turno.
func (s *OrdersService) lineasDelTablero(ctx context.Context) (map[int64][]BoardLine, error) {
	q := s.store.QC(ctx)
	filas, err := q.ListLinesOfActiveOrders(ctx)
	if err != nil {
		return nil, err
	}
	mods, err := q.ListModifiersOfActiveOrders(ctx)
	if err != nil {
		return nil, err
	}
	porLinea := map[int64][]string{}
	for _, m := range mods {
		nombre := m.OptionName
		// La cantidad solo se dice cuando repite: "Ranch" y no "1× Ranch", que es ruido en una
		// tarjeta que se lee de un vistazo.
		if m.Quantity > 1 {
			nombre = fmt.Sprintf("%d× %s", m.Quantity, m.OptionName)
		}
		porLinea[m.OrderLineID] = append(porLinea[m.OrderLineID], nombre)
	}
	porPedido := map[int64][]BoardLine{}
	for _, l := range filas {
		porPedido[l.OrderID] = append(porPedido[l.OrderID], BoardLine{
			ID: l.ID, Name: l.ProductName, Qty: l.Quantity, Delivered: l.DeliveredQty,
			Notes: derefStr(l.Notes), Modifiers: porLinea[l.ID],
		})
	}
	return porPedido, nil
}

// ChargeCmd es un cobro sobre un pedido que ya existe.
type ChargeCmd struct {
	OrderID   int64
	MethodID  int16
	Amount    decimal.Decimal
	Tip       decimal.Decimal
	Reference *string
	ActorID   int64
}

// Charge cobra un pedido que se mandó a cocina sin cobrar.
//
// Existía el hueco al revés: el tablero marcaba "POR COBRAR" y no había forma de saldarlo — el
// único lugar del sistema que registraba un pago de pedido era la creación. El operador veía la
// deuda y su única salida era levantar un pedido nuevo con los mismos productos, que descuenta el
// inventario dos veces y reporta una venta que no ocurrió.
//
// El pago entra en el turno ABIERTO AHORA, no en el del pedido: el dinero cae en el cajón de hoy,
// y meterlo en un arqueo ya firmado dejaría ese turno cuadrando contra efectivo que no estaba.
func (s *OrdersService) Charge(ctx context.Context, cmd ChargeCmd) error {
	if !domain.ValidMoney(domain.Round2(cmd.Amount), false) || !domain.ValidMoney(domain.Round2(cmd.Tip), true) {
		return domain.ErrValidation
	}
	sess, err := s.store.QC(ctx).GetOpenPrimarySession(ctx)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrNoOpenRegister
		}
		return err
	}
	return s.store.WithTx(ctx, func(q *db.Queries) error {
		// FOR UPDATE: entre leer lo cobrado y escribir el pago cabe otro cajero haciendo lo mismo,
		// y sin el lock los dos verían el pedido a cero y registrarían el total completo cada uno.
		o, err := q.GetOrderForUpdate(ctx, cmd.OrderID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return domain.ErrNotFound
			}
			return err
		}
		pagado, err := q.SumOrderPayments(ctx, cmd.OrderID)
		if err != nil {
			return err
		}
		if err := domain.ValidarCobro(string(o.Status), o.Total, pagado, cmd.Amount); err != nil {
			return err
		}
		// El método se resuelve BAJO RLS y contra la plataforma del pedido, igual que al crearlo:
		// un método de otra empresa entraría por la llave foránea (sus chequeos saltan RLS) y el
		// pago desaparecería del corte, dejando un faltante por el monto exacto sin explicación.
		m, err := q.GetPaymentMethod(ctx, cmd.MethodID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return domain.ErrNotFound
			}
			return err
		}
		if !domain.MetodoCorrespondeALaPlataforma(m.DeliveryPlatformID, o.DeliveryPlatformID) {
			return domain.ErrPaymentMethodPlatform
		}
		if err := q.CreateOrderPayment(ctx, db.CreateOrderPaymentParams{
			OrderID:           cmd.OrderID,
			PaymentMethodID:   cmd.MethodID,
			Amount:            domain.Round2(cmd.Amount),
			TipAmount:         domain.Round2(cmd.Tip),
			RegisterSessionID: &sess.ID,
			ReceivedBy:        &cmd.ActorID,
			Reference:         cmd.Reference,
		}); err != nil {
			return err
		}

		// El pedido que no pasa por cocina y queda SALDADO se cierra aquí mismo.
		//
		// Antes nacía entregado, porque crear y cobrar eran una sola llamada. Al separarlos —que es
		// lo que obliga a que cocina vea todo— esa regla se quedó sin dónde correr, y una embotellada
		// del mostrador se habría quedado abierta para siempre en la barra: el operador tendría que
		// entregarla a mano en la venta más frecuente del día. La regla no se perdió, se movió al
		// momento en que ahora ocurre.
		//
		// Las dos condiciones siguen siendo necesarias: cerrar algo que cocina tiene que preparar lo
		// borraría del tablero antes de hacerlo, y cerrar algo sin saldar escondería el faltante
		// hasta el corte.
		if !domain.PagosCubren(pagado.Add(domain.Round2(cmd.Amount)), o.Total) {
			return nil
		}
		necesita, err := q.PedidoNecesitaPreparacion(ctx, cmd.OrderID)
		if err != nil {
			return err
		}
		if necesita || !domain.CanTransition(string(o.Status), domain.StatusEntregada) {
			return nil
		}
		if err := q.DeliverAllOrderLines(ctx, cmd.OrderID); err != nil {
			return err
		}
		return q.SetOrderStatus(ctx, db.SetOrderStatusParams{
			ID: cmd.OrderID, Status: db.OrderStatusEntregada,
		})
	})
}

// Open lista los pedidos que el punto de venta tiene que seguir viendo: la barra de en curso.
//
// Es la UNIÓN de dos conjuntos, y confundirlos pierde uno de los dos. Los que siguen en cocina son
// a los que el cliente le pide algo más —incluidos los YA COBRADOS—, y los que deben dinero
// incluyen el ENTREGADO sin cobrar, que es el caro porque el cliente ya se fue. Quedarse solo con
// los impagos borraría el primero; solo con los no terminados, el segundo.
//
// Se llamaba Unpaid, y el nombre mentía en cuanto la lista dejó de ser solo de impagos.
func (s *OrdersService) Open(ctx context.Context) ([]BoardOrder, decimal.Decimal, error) {
	// La fecha sale del TURNO abierto, no del reloj del servidor.
	//
	// El pedido hereda la fecha de negocio del turno —así una noche que abre a las 4pm y cierra a
	// las 10pm numera corrido en vez de partirse a medianoche—, y filtrar por el reloj hacía que en
	// cuanto el día cambiara los dos dejaran de coincidir: todos los pedidos en curso desaparecían
	// de la pantalla, vivos y sin forma de llegar a ellos. El servidor corre en UTC y el local
	// cierra a las 22:00 de México, así que la medianoche UTC cae a las 18:00 locales: se vaciaba
	// todas las noches, en plena hora pico.
	//
	// Sin turno abierto se cae al día del servidor: no hay pedidos que mostrar, pero la pantalla
	// tiene que poder abrirse sin error.
	fecha := pgtype.Date{Time: s.now(), Valid: true}
	if sess, err := s.store.QC(ctx).GetOpenPrimarySession(ctx); err == nil {
		fecha = sess.BusinessDate
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return nil, decimal.Zero, err
	}

	rows, err := s.store.QC(ctx).ListOpenOrders(ctx, fecha)
	if err != nil {
		return nil, decimal.Zero, err
	}
	// El total pendiente se suma AQUÍ y no en el handler: sumar dinero de varias filas es agregación,
	// no formateo, y del mismo predicado que la lista para que el encabezado y el detalle no puedan
	// decir cosas distintas.
	pendiente := decimal.Zero
	out := make([]BoardOrder, 0, len(rows))
	for _, r := range rows {
		out = append(out, BoardOrder{
			ID: r.ID, Number: int(r.DailyNumber), FolioName: derefStr(r.FolioName),
			Status:      string(r.Status),
			ServiceType: string(r.ServiceType), DeliveryPlatformID: r.DeliveryPlatformID,
			CustomerName: r.CustomerName,
			Total:        r.Total, Currency: domain.Currency(r.Currency),
			Paid:          false,
			Outstanding:   domain.PorCobrar(r.Total, r.Paid),
			OpenedAt:      r.OpenedAt,
			EnPreparacion: r.EnPreparacion,
			Renglones:     int(r.Renglones),
		})
		pendiente = pendiente.Add(out[len(out)-1].Outstanding)
	}
	return out, domain.Round2(pendiente), nil
}
