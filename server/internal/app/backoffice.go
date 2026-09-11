package app

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
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
	IsActive          bool   `json:"isActive"`
	// DeliveryPlatformID: a qué plataforma pertenece, o nil si no es de plataforma. Es lo que deja
	// al POS ofrecer solo los dos métodos de la plataforma activa sin compararlos por nombre.
	DeliveryPlatformID *int16 `json:"deliveryPlatformId"`
}

func (s *BackofficeService) PaymentMethods(ctx context.Context) ([]PaymentMethodView, error) {
	rows, err := s.store.QC(ctx).ListPaymentMethods(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]PaymentMethodView, len(rows))
	for i, r := range rows {
		out[i] = PaymentMethodView{ID: int(r.ID), Name: r.Name, Kind: string(r.Kind), AffectsCashDrawer: r.AffectsCashDrawer, AutoDeclare: r.AutoDeclare, DeliveryPlatformID: r.DeliveryPlatformID}
	}
	return out, nil
}

// MetodoDePagoCmd: los tres interruptores de un método, cada uno OPCIONAL.
//
// Punteros y no booleanos, y la diferencia es dinero: con un `bool` pelado un campo ausente y un
// `false` explícito son indistinguibles, así que un PATCH que solo quería desactivar el método le
// habría apagado de paso el del cajón y sacado su dinero del arqueo.
type MetodoDePagoCmd struct {
	AutoDeclare       *bool
	IsActive          *bool
	AffectsCashDrawer *bool
}

// UpdatePaymentMethod cambia los interruptores de un método de cobro.
//
// Dos reglas que la base no puede expresar y que este método hace cumplir:
//
//   - El EFECTIVO DEL MOSTRADOR no sale del cajón. Los billetes que el cliente pone en el mostrador
//     están ahí por definición, y apagarlo dejaría el fondo de apertura y los movimientos de caja
//     fuera del esperado: el arqueo se compararía contra una cifra que no incluye el dinero con el
//     que abrió el turno.
//   - Un método de cajón no se AUTO-DECLARA. Es la regla que ya existía: el servidor declararía lo
//     que él mismo espera y el corte perdería la capacidad de detectar un faltante de efectivo.
//
// Las dos se evalúan contra el estado RESULTANTE —lo que viene en el cmd, o lo que ya había— y no
// contra lo que llegó: encender el auto-declarar y apagar el cajón en el mismo PATCH no puede
// colarse por el orden en que se miren los campos.
func (s *BackofficeService) UpdatePaymentMethod(ctx context.Context, methodID int, cmd MetodoDePagoCmd) (PaymentMethodView, error) {
	// TODO EN UNA TRANSACCIÓN, CON EL RENGLÓN TOMADO. Leer, validar el estado resultante y escribir
	// son tres pasos, y sin lock dos PATCH simultáneos —uno que enciende «automático», otro «va al
	// cajón»— validan cada uno contra el estado viejo y dejan escrita la combinación que este
	// código considera imposible.
	var vista PaymentMethodView
	err := s.store.WithTx(ctx, func(q *db.Queries) error {
		actual, err := q.LockPaymentMethod(ctx, int16(methodID))
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return domain.ErrNotFound
			}
			return err
		}

		tocaElCajon := actual.AffectsCashDrawer
		if cmd.AffectsCashDrawer != nil {
			tocaElCajon = *cmd.AffectsCashDrawer
		}
		autoDeclara := actual.AutoDeclare
		if cmd.AutoDeclare != nil {
			autoDeclara = *cmd.AutoDeclare
		}
		if err := domain.FlagDeCajonValido(actual.Kind == db.PaymentKindEfectivo, tocaElCajon); err != nil {
			return err
		}
		if autoDeclara && tocaElCajon {
			return fmt.Errorf("%w: un método cuyo dinero se cuenta en el cajón no se puede auto-declarar",
				domain.ErrValidation)
		}
		// Y el interruptor del cajón no se mueve con dinero de ese método ya dentro del turno
		// abierto (FR-017): movería el esperado en billetes que están físicamente en el cajón.
		if tocaElCajon != actual.AffectsCashDrawer {
			cobrado, err := q.CollectedByMethodInOpenSessions(ctx, int16(methodID))
			if err != nil {
				return err
			}
			if err := domain.CambioDeCajonPermitido(true, cobrado); err != nil {
				return fmt.Errorf("%w (%s ya cobró %s)", err, actual.Name, cobrado)
			}
		}

		row, err := q.UpdatePaymentMethodFlags(ctx, db.UpdatePaymentMethodFlagsParams{
			ID: int16(methodID), AutoDeclare: cmd.AutoDeclare, IsActive: cmd.IsActive,
			AffectsCashDrawer: cmd.AffectsCashDrawer,
		})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return domain.ErrNotFound
			}
			return err
		}
		vista = PaymentMethodView{
			ID: int(row.ID), Name: row.Name, Kind: string(row.Kind),
			AffectsCashDrawer: row.AffectsCashDrawer, AutoDeclare: row.AutoDeclare, IsActive: row.IsActive,
		}
		return nil
	})
	if err != nil {
		return PaymentMethodView{}, err
	}
	return vista, nil
}

// ---- Cortes de caja ----

// maxVentasDeCorte acota la lista de ventas que viaja dentro del detalle de un corte.
//
// Es el mismo tope que la frontera aplica a cualquier lista sin paginar (`httpapi.MaxListLimit`), y
// se repite aquí en vez de importarlo para no invertir el layering: `app` no depende de `httpapi`.
// Si uno se mueve, el otro también.
//
// Recortar no es esconder: el detalle viaja con la cuenta REAL para que la pantalla pueda decir
// cuántas hay en total.
const maxVentasDeCorte = 200

type MethodTotal struct {
	MethodID int    `json:"methodId"`
	Name     string `json:"name"`
	// Kind viaja para que la pantalla sepa cuál de los métodos es el del CAJÓN —el que se cuenta por
	// denominaciones— sin compararlo por nombre. Los métodos de plataforma en efectivo también tocan
	// el cajón, así que `affectsCashDrawer` no sirve para distinguirlo; es el mismo `kind` con el que
	// el corte decide de quién es el fondo (ver sessionWithExpected).
	Kind string `json:"kind"`
	// Expected viaja en NULL con el arqueo ciego encendido y el turno abierto: lo que la pantalla no
	// debe mostrar no se le manda. Ocultarlo en el cliente lo dejaría legible en la respuesta, y el
	// control dejaría de serlo.
	Expected    *decimal.Decimal `json:"expected"` // incluye ventas + propinas (+ fondo/neto en efectivo)
	Declared    decimal.Decimal  `json:"declared"`
	Difference  decimal.Decimal  `json:"difference"`
	Tips        decimal.Decimal  `json:"tips"` // propinas del método (parte del esperado; línea aparte en el resumen)
	AutoDeclare bool             `json:"autoDeclare"`
	// RequiresEntry: si el cierre exige una cifra capturada para este método.
	//
	// Lo decide el SERVIDOR y no la pantalla, y esa es toda su razón de existir. La pantalla lo
	// deducía de si el esperado era cero, y con el arqueo ciego el esperado viaja en null: ahí
	// `Number(null) === 0` concluía que ningún método esperaba dinero, la guardia del cierre se
	// apagaba y el botón quedaba habilitado con la pantalla en blanco. Es el faltante inventado de
	// $1,662 por la puerta de atrás.
	//
	// Falso para los que se autodeclaran, falso para los que forman el cajón —su dinero se declara
	// UNA vez, contándolo— y verdadero para el resto que esperaba dinero.
	RequiresEntry bool `json:"requiresEntry"`
}

// ArqueoDelCajonView: una cifra esperada, un conteo, una diferencia (spec 015).
//
// `Expected` en nil significa dos cosas distintas según el turno, y las dos son legítimas: en un
// corte anterior a esta feature no se guardó, y con el arqueo ciego encendido no se manda a la
// pantalla. `Counted` y `Difference` en nil mientras el turno esté abierto: todavía no se cuenta.
type ArqueoDelCajonView struct {
	Expected   *decimal.Decimal `json:"expected"`
	Counted    *decimal.Decimal `json:"counted"`
	Difference *decimal.Decimal `json:"difference"`
	// MethodIDs: qué métodos forman este cajón. Viaja para que la pantalla no vuelva a decidirlo por
	// su cuenta — dos derivaciones de la misma regla son dos pantallas que pueden diferir, y aquí
	// diferir es pedirle al operador una cifra por dinero que ya contó.
	MethodIDs []int `json:"methodIds"`
	// RequiresCount: si el cierre exige contar este cajón. Por lo mismo que `RequiresEntry`, lo
	// decide el servidor: con el arqueo ciego la pantalla no puede deducirlo del esperado.
	RequiresCount bool `json:"requiresCount"`
}

type CashMovementView struct {
	ID         int64           `json:"id"`
	Kind       string          `json:"kind"` // entrada | salida
	Amount     decimal.Decimal `json:"amount"`
	Concept    string          `json:"concept"`
	CreatedAt  time.Time       `json:"createdAt"`
	UserName   string          `json:"userName"`
	TransferID *int64          `json:"transferId"` // no-nil si el movimiento es una pierna de un traspaso
	ExpenseID  *int64          `json:"expenseId"`  // no-nil si es la salida de un gasto (va en la sección Gastos)
}

// CashExpenseView es un PAGO de gasto atribuido a un corte (efectivo o no), para la sección
// "Gastos" del resumen.
//
// Es el pago y no el gasto porque desde 0029 la atribución al corte vive en cada pago: un gasto
// liquidado con tarjeta un día y efectivo otro toca dos cortes, y cada uno debe ver solo su
// parte. Amount es el importe del PAGO, no el del gasto completo.
type CashExpenseView struct {
	ID            int64           `json:"id"` // id del pago
	ExpenseID     int64           `json:"expenseId"`
	Category      string          `json:"category"`
	Supplier      *string         `json:"supplier"`
	PaymentMethod string          `json:"paymentMethod"`
	Amount        decimal.Decimal `json:"amount"`
	Currency      domain.Currency `json:"currency"`
	Status        string          `json:"status"`
}

// Descomposición del corte en ingresos/egresos → método → concepto, para el resumen jerárquico
// (estilo arqueo). Explica cómo el sistema llegó a cada esperado; los movimientos/gastos
// itemizados quedan como drill-down (Movements/Expenses).
type CorteBucket struct {
	Concept string          `json:"concept"`
	Amount  decimal.Decimal `json:"amount"`
}
type CorteMethodBreakdown struct {
	Method string          `json:"method"`
	Total  decimal.Decimal `json:"total"`
	Items  []CorteBucket   `json:"items"` // Ventas, Entradas, Traspasos recibidos, (Propinas)
}
type CorteBreakdown struct {
	Ingresos      []CorteMethodBreakdown `json:"ingresos"` // por método
	IngresosTotal decimal.Decimal        `json:"ingresosTotal"`
	Egresos       []CorteBucket          `json:"egresos"` // salidas de efectivo (Gastos, Salidas, Traspasos)
	EgresosTotal  decimal.Decimal        `json:"egresosTotal"`
	// Plataformas: lo que entró por cada plataforma, sumando sus DOS métodos (en línea y efectivo).
	// Por método solo se ve la mitad de cada una, y quien cierra caja a las once de la noche
	// termina sumando dos renglones a mano. Es además el número contra el que se concilia el
	// depósito que la plataforma manda después. Vacío cuando no hubo ventas de plataforma.
	Plataformas []CortePlatformSubtotal `json:"plataformas"`
}

// CortePlatformSubtotal: cuánto entró por una plataforma en el turno.
type CortePlatformSubtotal struct {
	Platform string          `json:"platform"`
	Total    decimal.Decimal `json:"total"`
}

// methodExpected: entrada para corteBreakdown (nombre, esperado del sistema, propinas y si es el
// método dueño del fondo de caja).
type methodExpected struct {
	name     string
	expected decimal.Decimal // ventas + propinas (+ fondo/neto en efectivo)
	tips     decimal.Decimal
	// duenoDelFondo: el ÚNICO método al que pertenecen el fondo de apertura y los movimientos de
	// caja, que son un solo montón de billetes. NO es lo mismo que `affects_cash_drawer`: el
	// efectivo que entrega un repartidor de Uber también se cuenta en el arqueo, pero el fondo no
	// es suyo. Mientras el efectivo del mostrador fue el único método de cajón las dos ideas
	// coincidían; desde 0037 hay cuatro y confundirlas reportaba el fondo una vez por método.
	duenoDelFondo bool
	// plataforma: nombre de la plataforma a la que pertenece el método, vacío si es de mostrador.
	// Es lo que permite subtotalizar sin comparar nombres de método.
	plataforma string
}

// corteBreakdown descompone un corte en ingresos (por método → concepto) y egresos de efectivo.
// ventas se derivan del esperado restando propinas y (en efectivo) fondo + neto de movimientos, así
// una caja secundaria (esperado = fondo + neto) da 0 ventas sola. Propinas se listan aparte.
func corteBreakdown(opening decimal.Decimal, methods []methodExpected, moves []db.ListCashMovementsRow) CorteBreakdown {
	var entradas, traspasosIn, salidas, traspasosOut, gastos, net decimal.Decimal
	for _, m := range moves {
		if m.Kind == domain.CashEntrada {
			net = net.Add(m.Amount)
		} else {
			net = net.Sub(m.Amount)
		}
		switch {
		case m.ExpenseID != nil: // gasto en efectivo (siempre salida)
			gastos = gastos.Add(m.Amount)
		case m.TransferID != nil:
			if m.Kind == domain.CashEntrada {
				traspasosIn = traspasosIn.Add(m.Amount)
			} else {
				traspasosOut = traspasosOut.Add(m.Amount)
			}
		case m.Kind == domain.CashEntrada:
			entradas = entradas.Add(m.Amount)
		default:
			salidas = salidas.Add(m.Amount)
		}
	}

	out := CorteBreakdown{Ingresos: []CorteMethodBreakdown{}, Egresos: []CorteBucket{}, Plataformas: []CortePlatformSubtotal{}}
	// Subtotal por plataforma. El orden es el de los métodos —que viene de sort_key— y no
	// alfabético: así el corte lista las plataformas en el mismo orden en que aparecen sus métodos
	// arriba, y los dos bloques se leen juntos sin buscar.
	porPlataforma := map[string]decimal.Decimal{}
	ordenPlataformas := []string{}
	for _, me := range methods {
		ventas := me.expected.Sub(me.tips)
		if me.duenoDelFondo {
			ventas = ventas.Sub(opening).Sub(net)
		}
		ventas = domain.Round2(ventas)
		items := []CorteBucket{}
		total := decimal.Zero
		add := func(concept string, amt decimal.Decimal) {
			if amt.IsPositive() {
				items = append(items, CorteBucket{Concept: concept, Amount: amt})
				total = total.Add(amt)
			}
		}
		add("Ventas", ventas)
		add("Propinas", domain.Round2(me.tips))
		if me.duenoDelFondo {
			add("Entradas", domain.Round2(entradas))
			add("Traspasos recibidos", domain.Round2(traspasosIn))
		}
		if len(items) > 0 {
			out.Ingresos = append(out.Ingresos, CorteMethodBreakdown{Method: me.name, Total: domain.Round2(total), Items: items})
			out.IngresosTotal = out.IngresosTotal.Add(total)
		}
		if me.plataforma != "" && total.IsPositive() {
			if _, visto := porPlataforma[me.plataforma]; !visto {
				ordenPlataformas = append(ordenPlataformas, me.plataforma)
			}
			porPlataforma[me.plataforma] = porPlataforma[me.plataforma].Add(total)
		}
	}
	// Solo las que vendieron: un renglón en $0 por cada plataforma configurada llena el corte de
	// ruido justo donde se está buscando un descuadre.
	for _, nombre := range ordenPlataformas {
		out.Plataformas = append(out.Plataformas, CortePlatformSubtotal{Platform: nombre, Total: domain.Round2(porPlataforma[nombre])})
	}
	for _, b := range []CorteBucket{
		{Concept: "Gastos", Amount: domain.Round2(gastos)},
		{Concept: "Salidas de efectivo", Amount: domain.Round2(salidas)},
		{Concept: "Traspasos enviados", Amount: domain.Round2(traspasosOut)},
	} {
		if b.Amount.IsPositive() {
			out.Egresos = append(out.Egresos, b)
			out.EgresosTotal = out.EgresosTotal.Add(b.Amount)
		}
	}
	out.IngresosTotal = domain.Round2(out.IngresosTotal)
	out.EgresosTotal = domain.Round2(out.EgresosTotal)
	return out
}

type SessionView struct {
	ID           int64              `json:"id"`
	RegisterID   int64              `json:"registerId"`
	RegisterName string             `json:"registerName"`
	IsPrimary    bool               `json:"isPrimary"` // la caja primaria recibe las ventas del POS
	Status       string             `json:"status"`
	OpeningCash  decimal.Decimal    `json:"openingCash"`
	Currency     domain.Currency    `json:"currency"`
	OpenedAt     time.Time          `json:"openedAt"`
	NetMovements decimal.Decimal    `json:"netMovements"` // entradas − salidas de efectivo
	Totals       []MethodTotal      `json:"totals"`
	Movements    []CashMovementView `json:"movements"`
	Expenses     []CashExpenseView  `json:"expenses"`
	Breakdown    CorteBreakdown     `json:"breakdown"`
	// Pending son los pedidos del turno que todavía no se terminan de entregar. Salen del MISMO
	// predicado que bloquea el cierre, no de una consulta parecida: si la pantalla y la guardia se
	// derivaran por separado, una de las dos mentiría y quien la lee no tendría cómo saber cuál.
	//
	// Se listan aunque ya estén cobrados: cobrado y entregado son cosas distintas, y lo que impide
	// cerrar es la comida que no ha salido, no el dinero.
	Pending []PendingOrder `json:"pending"`
	// Cashiers: cuánto cobró cada persona en el turno. Dos estaciones cobran contra el MISMO
	// cajón —partirlo en dos daría dos arqueos contando el mismo dinero—, así que la
	// responsabilidad se rastrea por quien cobró y no por el mueble.
	Cashiers []CashierTotal `json:"cashiers"`
	// Uncollected es la venta del turno que ningún pago cubre, y UncollectedCount en cuántos
	// pedidos está. Es la hermana de Pending: aquélla dice qué comida no ha salido, ésta qué dinero
	// no entró.
	//
	// Existe porque sin ella el hueco es invisible. Medido el 8 de septiembre de 2026: un turno
	// cerró con los diez métodos en diferencia $0.00 mientras cinco pedidos entregados por $554.00
	// no tenían un solo pago. El arqueo cuadraba por construcción —solo compara pagos contra
	// declarado— y la venta faltante solo se veía restando dos cifras de dos pantallas distintas.
	//
	// NO bloquea el cierre: entregar sin cobrar es una decisión legítima del negocio (se fio, se
	// cobró por fuera). Lo que no puede pasar es que el arqueo no la nombre.
	Uncollected      decimal.Decimal `json:"uncollected"`
	UncollectedCount int             `json:"uncollectedCount"`
	// Counts: lo que se contó al abrir. Va en la MISMA forma que en el detalle del corte y sale del
	// mismo lugar: dos derivaciones del mismo desglose son dos pantallas que pueden no coincidir.
	Counts *ConteosDelTurno `json:"counts"`
	// Drawer: el arqueo del cajón físico. Nil = este turno no tiene arqueo de efectivo (un corte
	// anterior a la spec 015, o una caja que no maneja efectivo).
	Drawer *ArqueoDelCajonView `json:"drawer"`
}

// CashierTotal es lo que cobró una persona en el turno. El efectivo va aparte de lo demás porque
// es lo único que está físicamente en el cajón: una diferencia de arqueo solo puede venir de ahí.
type CashierTotal struct {
	Name     string          `json:"name"`
	Cash     decimal.Decimal `json:"cash"`
	Other    decimal.Decimal `json:"other"`
	Payments int             `json:"payments"`
}

// PendingOrder es un pedido del turno que sigue sin entregarse.
type PendingOrder struct {
	Number int    `json:"number"`
	Name   string `json:"name"`
}

// CashRegisterView es una caja del catálogo. OpenSessionID no-nil = tiene una sesión abierta.
type CashRegisterView struct {
	ID            int64  `json:"id"`
	Name          string `json:"name"`
	IsPrimary     bool   `json:"isPrimary"`
	IsActive      bool   `json:"isActive"`
	OpenSessionID *int64 `json:"openSessionId"`
}

// SessionDetailView es una sesión (normalmente ya cerrada) con sus totales guardados y
// movimientos, para el histórico. Difiere de SessionView en que los totales vienen de
// register_session_totals (snapshot al cerrar), no del cálculo en vivo.
type SessionDetailView struct {
	ID           int64              `json:"id"`
	RegisterName string             `json:"registerName"`
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
	Expenses     []CashExpenseView  `json:"expenses"`
	Breakdown    CorteBreakdown     `json:"breakdown"`

	// Las ventas que este corte cobró. Viven aquí y no en un filtro de la pantalla de Ventas: ahí
	// convivirían con el filtro de fechas y bastaría elegir un rango que no toque el corte para
	// llegar a una pantalla vacía sin explicación.
	Sales []SessionSaleView `json:"sales"`
	// Cuántas hay EN TOTAL, no cuántas se mandaron. Un recorte silencioso se lee como "esto es todo".
	SalesCount int `json:"salesCount"`
	SalesShown int `json:"salesShown"`
	// Suma de las ventas que dejaron ingreso: sin canceladas, sin reembolsadas y sin propinas. La
	// pantalla declara las tres exclusiones.
	SalesTotal decimal.Decimal `json:"salesTotal"`
	// Uncollected es la parte de SalesTotal que ningún pago cubre. Sin ella las dos cifras de esta
	// misma pantalla —lo vendido y lo esperado por método— no cuadran y nadie puede saber cuál de
	// las dos miente: es el corolario del principio III, y aquí importa más que en el turno abierto
	// porque ESTA es la pantalla que alguien audita cuando ya nadie se acuerda del turno.
	Uncollected      decimal.Decimal `json:"uncollected"`
	UncollectedCount int             `json:"uncollectedCount"`
	// Counts: el desglose de los dos arqueos del turno. Es lo que convierte un faltante en algo
	// investigable — "faltan dos billetes de $500" en vez de "faltan $1,000".
	Counts *ConteosDelTurno `json:"counts"`
	// Drawer: el arqueo del cajón físico. Nil = este turno no tiene arqueo de efectivo (un corte
	// anterior a la spec 015, o una caja que no maneja efectivo).
	Drawer *ArqueoDelCajonView `json:"drawer"`
}

type SessionSaleView struct {
	ID          int64           `json:"id"`
	DailyNumber int             `json:"dailyNumber"`
	FolioName   *string         `json:"folioName"`
	OpenedAt    time.Time       `json:"openedAt"`
	Status      string          `json:"status"`
	ServiceType string          `json:"serviceType"`
	Total       decimal.Decimal `json:"total"`
	Refund      decimal.Decimal `json:"refund"`
}

type SessionHistoryRow struct {
	ID              int64           `json:"id"`
	RegisterName    string          `json:"registerName"`
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

// CashRegisters lista las cajas activas + el id de su sesión abierta (para pickers y la vista de caja).
func (s *BackofficeService) CashRegisters(ctx context.Context) ([]CashRegisterView, error) {
	rows, err := s.store.QC(ctx).ListCashRegisters(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]CashRegisterView, len(rows))
	for i, r := range rows {
		out[i] = CashRegisterView{ID: r.ID, Name: r.Name, IsPrimary: r.IsPrimary, IsActive: r.IsActive, OpenSessionID: r.OpenSessionID}
	}
	return out, nil
}

// AllCashRegisters lista TODAS las cajas (incl. inactivas) para la gestión; sin estado de sesión.
func (s *BackofficeService) AllCashRegisters(ctx context.Context) ([]CashRegisterView, error) {
	rows, err := s.store.QC(ctx).ListAllCashRegisters(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]CashRegisterView, len(rows))
	for i, r := range rows {
		out[i] = CashRegisterView{ID: r.ID, Name: r.Name, IsPrimary: r.IsPrimary, IsActive: r.IsActive}
	}
	return out, nil
}

func (s *BackofficeService) CreateCashRegister(ctx context.Context, name string) (CashRegisterView, error) {
	if name == "" {
		return CashRegisterView{}, domain.ErrValidation
	}
	r, err := s.store.QC(ctx).CreateCashRegister(ctx, name)
	if err != nil {
		return CashRegisterView{}, err
	}
	return CashRegisterView{ID: r.ID, Name: r.Name, IsPrimary: r.IsPrimary, IsActive: r.IsActive}, nil
}

func (s *BackofficeService) UpdateCashRegister(ctx context.Context, id int64, name string, isActive bool) (CashRegisterView, error) {
	if name == "" {
		return CashRegisterView{}, domain.ErrValidation
	}
	reg, err := s.store.QC(ctx).GetCashRegister(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return CashRegisterView{}, domain.ErrNotFound
		}
		return CashRegisterView{}, err
	}
	// La caja primaria recibe las ventas del POS: desactivarla dejaría al punto de venta sin
	// dónde cuadrar el efectivo. Renombrarla sí se permite.
	if reg.IsPrimary && !isActive {
		return CashRegisterView{}, domain.ErrValidation
	}
	r, err := s.store.QC(ctx).UpdateCashRegister(ctx, db.UpdateCashRegisterParams{ID: id, Name: name, IsActive: isActive})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return CashRegisterView{}, domain.ErrNotFound
		}
		return CashRegisterView{}, err
	}
	return CashRegisterView{ID: r.ID, Name: r.Name, IsPrimary: r.IsPrimary, IsActive: r.IsActive}, nil
}

// activeRegister carga una caja y exige que exista y esté activa (una caja inactiva no opera).
func (s *BackofficeService) activeRegister(ctx context.Context, registerID int64) (db.GetCashRegisterRow, error) {
	reg, err := s.store.QC(ctx).GetCashRegister(ctx, registerID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return reg, domain.ErrNotFound
		}
		return reg, err
	}
	if !reg.IsActive {
		return reg, domain.ErrValidation
	}
	return reg, nil
}

// OpenSession abre una sesión (corte) para una caja concreta. Falla si esa caja ya tiene una
// sesión abierta (respaldado por el índice único one_open_session_per_register).
// DenominationView: una pieza que se puede contar.
type DenominationView struct {
	ID     int64           `json:"id"`
	Value  decimal.Decimal `json:"value"`
	IsCoin bool            `json:"isCoin"`
}

// Denominations devuelve el catálogo de una moneda, de mayor a menor.
//
// Solo las activas: una denominación retirada de circulación no vuelve a ofrecerse, pero sigue
// existiendo para los arqueos que la usaron.
func (s *BackofficeService) Denominations(ctx context.Context, moneda domain.Currency) ([]DenominationView, error) {
	filas, err := s.store.QC(ctx).ListDenominations(ctx, string(moneda))
	if err != nil {
		return nil, err
	}
	out := make([]DenominationView, 0, len(filas))
	for _, f := range filas {
		out = append(out, DenominationView{ID: f.ID, Value: f.Value, IsCoin: f.IsCoin})
	}
	return out, nil
}

// ConteoLineView: un renglón del desglose, como lo lee quien compara contra su cajón.
//
// `Subtotal` viaja calculado desde la base aunque sea derivable: lo lee un humano contando billetes,
// y si además lo multiplica la pantalla hay dos multiplicaciones del mismo dato que pueden diferir.
type ConteoLineView struct {
	Value    decimal.Decimal `json:"value"`
	IsCoin   bool            `json:"isCoin"`
	Pieces   int             `json:"pieces"`
	Subtotal decimal.Decimal `json:"subtotal"`
}

// ConteoView: el arqueo de efectivo de un momento del turno.
//
// `ManualReason` no nulo significa que NO se contó por denominaciones y por qué; en ese caso `Lines`
// viene vacío. Es la mitad de FR-016 que hace auditable un arqueo sin piezas.
type ConteoView struct {
	Total decimal.Decimal `json:"total"`
	// Expected y Difference solo existen en el CIERRE, y solo desde la spec 015: en la apertura no
	// hay nada que esperar, y un corte cerrado antes de la 0067 no los guardó. Nil en los dos casos,
	// que la pantalla trata igual — muestra el total como siempre y no inventa una diferencia.
	Expected     *decimal.Decimal `json:"expected"`
	Difference   *decimal.Decimal `json:"difference"`
	ManualReason *string          `json:"manualReason"`
	Lines        []ConteoLineView `json:"lines"`
}

// ConteosDelTurno: lo que se contó al abrir y al cerrar.
//
// Un momento en nil = ese arqueo no se contó, que es el caso de TODOS los cortes anteriores a esta
// funcionalidad (FR-008). Nil no se rellena con un conteo en cero: eso afirmaría "conté el cajón y
// estaba vacío", que es un hecho distinto de "nadie contó".
type ConteosDelTurno struct {
	Apertura *ConteoView `json:"apertura"`
	Cierre   *ConteoView `json:"cierre"`
}

// conteosDelTurno lee el desglose de los dos momentos, para el turno abierto y para el corte
// cerrado.
//
// UNA SOLA DERIVACIÓN para las dos vistas, a propósito: el turno abierto muestra lo que se contó al
// abrir y el detalle del corte muestra los dos, y sacarlo por caminos distintos es de donde salen
// dos pantallas que no coinciden sin forma de saber cuál miente.
func (s *BackofficeService) conteosDelTurno(ctx context.Context, sessionID int64) (*ConteosDelTurno, error) {
	out := &ConteosDelTurno{}
	for _, momento := range []db.CashCountMoment{db.CashCountMomentApertura, db.CashCountMomentCierre} {
		conteo, err := s.store.QC(ctx).GetCashCount(ctx, db.GetCashCountParams{SessionID: sessionID, Moment: momento})
		if err != nil {
			// Sin conteo de ese momento no hay nada que mostrar, y no es un error: es lo normal en
			// los cortes que cerraron antes de esta funcionalidad.
			if errors.Is(err, pgx.ErrNoRows) {
				continue
			}
			return nil, err
		}
		vista := &ConteoView{
			Total: conteo.Total, Expected: conteo.Expected, Difference: conteo.Difference,
			ManualReason: conteo.ManualReason, Lines: []ConteoLineView{},
		}
		filas, err := s.store.QC(ctx).ListCashCountLines(ctx, conteo.ID)
		if err != nil {
			return nil, err
		}
		for _, f := range filas {
			vista.Lines = append(vista.Lines, ConteoLineView{
				Value: f.Value, IsCoin: f.IsCoin, Pieces: int(f.Pieces), Subtotal: f.Subtotal,
			})
		}
		if momento == db.CashCountMomentApertura {
			out.Apertura = vista
		} else {
			out.Cierre = vista
		}
	}
	if out.Apertura == nil && out.Cierre == nil {
		return nil, nil // un corte sin ningún conteo: la pantalla lo muestra como siempre
	}
	return out, nil
}

// esperadoDe: el esperado de un método como valor.
//
// El campo es puntero porque con el arqueo ciego viaja en null, pero el CIERRE nunca ve un null:
// nulifica solo el camino de lectura, y aquí un cero por si acaso es más barato que un 500 por un
// puntero que alguien deje pasar el día que eso cambie.
func esperadoDe(t MethodTotal) decimal.Decimal {
	if t.Expected == nil {
		return decimal.Zero
	}
	return *t.Expected
}

// ocultarLoEsperado borra del view lo que el arqueo ciego no debe mostrar.
//
// Se aplica en los caminos de LECTURA de un turno abierto y jamás en el del cierre, que necesita
// las cifras para calcular lo que guarda. Y borra el esperado de TODOS los métodos, no solo del
// efectivo: ver el de la tarjeta permite el mismo acomodo.
func ocultarLoEsperado(totals []MethodTotal, drawer *ArqueoDelCajonView) {
	for i := range totals {
		totals[i].Expected = nil
	}
	if drawer != nil {
		drawer.Expected = nil
		drawer.Difference = nil
	}
}

// arqueoCiego dice si este negocio cuenta a ciegas.
//
// Un fallo al leer los ajustes NO enciende el control: devolver "ciego" ante un error escondería
// las cifras por un hiccup de la base, y el operador no tendría cómo saber por qué. El default es
// el comportamiento de la spec 003, que es el que está probado.
func (s *BackofficeService) arqueoCiego(ctx context.Context) bool {
	ajustes, err := s.store.QC(ctx).GetBusinessSettings(ctx)
	if err != nil {
		return false
	}
	return ajustes.BlindCashCount
}

// arqueoEnVivo arma el arqueo del cajón de un turno ABIERTO: el esperado se calcula, y lo contado
// todavía no existe.
//
// Devuelve nil si ningún método de este turno pone dinero en el cajón: una caja secundaria que no
// maneja efectivo no tiene arqueo que mostrar, y un arqueo en cero afirmaría que el cajón debía
// estar vacío.
func arqueoEnVivo(metodos []domain.MetodoDelCorte, conteos *ConteosDelTurno) (*ArqueoDelCajonView, error) {
	ids := domain.MetodosDelCajon(metodos)
	if len(ids) == 0 {
		return nil, nil
	}
	esperado, err := domain.EsperadoDelCajon(metodos)
	if err != nil {
		return nil, err
	}
	arqueo := &ArqueoDelCajonView{Expected: &esperado, MethodIDs: ids}
	// Exige conteo mientras haya dinero que contar y nadie lo haya contado. Un cajón que no espera
	// nada no obliga: es el mismo criterio con el que un método sin movimiento no pide cifra.
	yaContado := conteos != nil && conteos.Cierre != nil
	arqueo.RequiresCount = !esperado.IsZero() && !yaContado
	if yaContado {
		arqueo.Counted = &conteos.Cierre.Total
		if conteos.Cierre.Difference != nil {
			arqueo.Difference = conteos.Cierre.Difference
		}
	}
	return arqueo, nil
}

// arqueoGuardado arma el arqueo de un corte CERRADO desde lo que se guardó, sin recalcular nada.
//
// Aquí está la diferencia que importa con `arqueoEnVivo`: el esperado sale de la fila del conteo y
// los métodos, del snapshot de cada renglón. Recalcularlos leería el catálogo de hoy, y entonces un
// corte cerrado se reagruparía —o cambiaría de cifra— según cuándo alguien lo abra.
//
// Nil cuando no hay conteo de cierre: es el caso de todos los cortes anteriores a la spec 015.
func arqueoGuardado(metodos []domain.MetodoDelCorte, conteos *ConteosDelTurno) *ArqueoDelCajonView {
	if conteos == nil || conteos.Cierre == nil {
		return nil
	}
	cierre := conteos.Cierre
	arqueo := &ArqueoDelCajonView{
		Counted:   &cierre.Total,
		MethodIDs: domain.MetodosDelCajon(metodos),
	}
	if cierre.Expected != nil {
		arqueo.Expected = cierre.Expected
		arqueo.Difference = cierre.Difference
	}
	return arqueo
}

// PiezaCapturada: cuántas piezas de una denominación contó el operador.
//
// Viaja con el ID y no con el valor: el valor lo resuelve el servidor leyendo el catálogo, que es lo
// que hace que un total mandado por el cliente no pueda influir en nada (FR-003).
type PiezaCapturada struct {
	DenominationID int64
	Pieces         int
}

// AperturaCmd: cómo se declara el fondo al abrir la caja.
//
// Los dos caminos de FR-014 son EXCLUYENTES: o se cuentan piezas, o se captura el total con un
// motivo. Ninguno de los dos es un cajón vacío, que es válido. Quien decide es `domain`.
type AperturaCmd struct {
	Piezas []PiezaCapturada
	Total  *decimal.Decimal
	Motivo string
}

func (s *BackofficeService) OpenSession(ctx context.Context, registerID int64, cmd AperturaCmd, userID int64) (*SessionView, error) {
	reg, err := s.activeRegister(ctx, registerID)
	if err != nil {
		return nil, err
	}
	if _, err := s.store.QC(ctx).GetOpenSessionByRegister(ctx, registerID); err == nil {
		return nil, domain.ErrConflict // esa caja ya está abierta
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}
	// El total sale de las PIEZAS y del catálogo, nunca de lo que mande el cliente: es la misma
	// regla que `BuildOrder` con los precios. Se resuelve antes de abrir la transacción porque leer
	// el catálogo no necesita estar dentro de ella.
	// La moneda del turno es la de la columna, que hoy siempre toma su default: no hay forma de
	// elegir otra al abrir. Cuando la haya, ESTA línea es la que cambia — y el test de FR-011 ya
	// cubre que una denominación de otra moneda se rechaza.
	piezas, err := s.piezasConSuValor(ctx, cmd.Piezas, string(domain.DefaultCurrency))
	if err != nil {
		return nil, err
	}
	total, err := domain.TotalDeclarado(piezas, cmd.Total, cmd.Motivo)
	if err != nil {
		return nil, err
	}

	var sess db.RegisterSession
	// UNA SOLA TRANSACCIÓN. Son tres escrituras —la sesión, el conteo y sus renglones— y si la
	// segunda o la tercera fallan después de que la primera comprometió, queda una sesión ABIERTA
	// SIN CONTEO Y SIN MOTIVO, que es justo lo que FR-016 prohíbe. Peor: la caja queda bloqueada,
	// porque `one_open_session_per_register` no deja abrir otra hasta resolver la huérfana a mano.
	err = s.store.WithTx(ctx, func(q *db.Queries) error {
		abierta, err := q.OpenSession(ctx, db.OpenSessionParams{
			BusinessDate: pgtype.Date{Time: s.businessDate(ctx), Valid: true},
			OpeningCash:  total,
			OpenedBy:     userID,
			RegisterID:   registerID,
		})
		if err != nil {
			return err
		}
		sess = abierta
		// Sin esperado: al abrir no hay nada que esperar — el fondo ES lo que se contó. El check del
		// esquema lo exige nulo justo aquí.
		return s.guardarConteo(ctx, q, abierta.ID, db.CashCountMomentApertura, total, nil,
			cmd.Piezas, motivoDelCamino(cmd.Total, cmd.Motivo), userID)
	})
	if err != nil {
		// El pre-check de arriba NO es atómico: dos tabletas pueden pasarlo las dos y la segunda
		// choca aquí con `one_open_session_per_register`. La fila secuencial ya devuelve
		// ErrConflict; esto hace que la carrera termine igual, y no en un 500.
		return nil, traduceConflictoDeCaja(err, db.CashCountMomentApertura)
	}
	return s.sessionWithExpected(ctx, sess, reg)
}

// piezasConSuValor cambia ids por valores leyendo el catálogo, y de paso hace cumplir FR-011.
//
// La moneda importa: contar dólares en un turno en pesos daría un total sin significado, y el
// arqueo del turno entero se compararía después contra esa cifra. Un id que no existe también se
// rechaza — es una pantalla desincronizada del catálogo, no un cero.
func (s *BackofficeService) piezasConSuValor(ctx context.Context, capturadas []PiezaCapturada, moneda string) ([]domain.PiezaContada, error) {
	if len(capturadas) == 0 {
		return nil, nil
	}
	catalogo, err := s.store.QC(ctx).ListDenominations(ctx, moneda)
	if err != nil {
		return nil, err
	}
	valorDe := make(map[int64]decimal.Decimal, len(catalogo))
	for _, d := range catalogo {
		valorDe[d.ID] = d.Value
	}
	out := make([]domain.PiezaContada, 0, len(capturadas))
	// LA MISMA DENOMINACIÓN NO PUEDE VENIR DOS VECES, y se rechaza aquí porque es el único lugar que
	// ve los ids: `domain.TotalDelConteo` recibe valores, así que SUMA los dos renglones sin poder
	// saber que son el mismo billete, y lo único que quedaba impidiendo un fondo inflado era la
	// unique del esquema — que salta cuando ya se escribió la sesión, o sea como 500.
	vistas := make(map[int64]bool, len(capturadas))
	for _, p := range capturadas {
		valor, ok := valorDe[p.DenominationID]
		if !ok {
			return nil, fmt.Errorf("%w: la denominación %d no es de la moneda del turno (%s)",
				domain.ErrValidation, p.DenominationID, moneda)
		}
		if vistas[p.DenominationID] {
			return nil, fmt.Errorf("%w: la denominación de $%s llegó dos veces en el conteo",
				domain.ErrValidation, valor)
		}
		vistas[p.DenominationID] = true
		out = append(out, domain.PiezaContada{Valor: valor, Piezas: p.Pieces})
	}
	return out, nil
}

// guardarConteo escribe el conteo y sus renglones DENTRO de la transacción que le pasen.
//
// Los renglones con cero piezas no se escriben: "no hay" y "no se capturó" son lo mismo en un
// arqueo (FR-009), y el `check (pieces > 0)` del esquema está para que eso no dependa de que esta
// función se acuerde.
func (s *BackofficeService) guardarConteo(ctx context.Context, q *db.Queries, sessionID int64,
	momento db.CashCountMoment, total decimal.Decimal, esperado *decimal.Decimal,
	piezas []PiezaCapturada, motivo *string, userID int64) error {
	conteo, err := q.SaveCashCount(ctx, db.SaveCashCountParams{
		SessionID: sessionID, Moment: momento, Total: total, Expected: esperado,
		ManualReason: motivo, CreatedBy: userID,
	})
	if err != nil {
		return traduceConflictoDeCaja(err, momento)
	}
	for _, p := range piezas {
		if p.Pieces <= 0 {
			continue
		}
		if err := q.SaveCashCountLine(ctx, db.SaveCashCountLineParams{
			CountID: conteo.ID, DenominationID: p.DenominationID, Pieces: int32(p.Pieces),
		}); err != nil {
			return traduceConflictoDeCaja(err, momento)
		}
	}
	return nil
}

// motivoDelCamino devuelve el motivo a guardar, o nil si el efectivo se contó por denominaciones.
//
// Nulo significa "se contó" en el esquema, así que la señal es el CAMINO y no el texto: se limpia
// con `domain.MotivoLimpio`, la misma regla con la que el dominio lo aprobó, porque guardar algo
// distinto de lo que se validó es cómo se cuela un motivo que la pantalla ve vacío.
func motivoDelCamino(aMano *decimal.Decimal, motivo string) *string {
	if aMano == nil {
		return nil
	}
	m := domain.MotivoLimpio(motivo)
	return &m
}

// traduceConflictoDeCaja convierte los 23505 que el CLIENTE puede provocar en un error que dice qué
// pasó, en vez del "el servidor se rompió" que sale de dejar subir el error crudo de Postgres.
//
// Ninguno de los tres es raro:
//   - dos conteos del mismo momento: las dos tabletas comparten cuenta, así que dos personas pueden
//     abrir el cierre y confirmar las dos;
//   - la misma caja abierta dos veces: la misma carrera, en la apertura. El pre-check de
//     `GetOpenSessionByRegister` es read-then-insert y las dos pasan;
//   - la misma denominación dos veces: una pantalla desincronizada. `piezasConSuValor` ya lo rechaza
//     antes de escribir, pero esto queda como respaldo para cualquier camino que se agregue después.
func traduceConflictoDeCaja(err error, momento db.CashCountMoment) error {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23505" {
		return err
	}
	switch {
	case strings.Contains(pgErr.ConstraintName, "session_cash_counts_un_momento"):
		return fmt.Errorf("%w: este turno ya tiene un conteo de %s guardado; recarga para ver el que quedó",
			domain.ErrConflict, momento)
	case strings.Contains(pgErr.ConstraintName, "one_open_session_per_register"):
		return fmt.Errorf("%w: esa caja ya está abierta; recarga para ver el turno que quedó",
			domain.ErrConflict)
	case strings.Contains(pgErr.ConstraintName, "session_cash_count_lines_unicas"):
		return fmt.Errorf("%w: llegó la misma denominación dos veces en el conteo",
			domain.ErrValidation)
	}
	return err
}

// CurrentByRegister devuelve la sesión abierta de una caja con sus esperados en vivo, o nil si
// esa caja está cerrada.
func (s *BackofficeService) CurrentByRegister(ctx context.Context, registerID int64) (*SessionView, error) {
	reg, err := s.store.QC(ctx).GetCashRegister(ctx, registerID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	sess, err := s.store.QC(ctx).GetOpenSessionByRegister(ctx, registerID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	view, err := s.sessionWithExpected(ctx, sess, reg)
	if err != nil {
		return nil, err
	}
	// ARQUEO CIEGO: quien va a contar no ve lo que el sistema espera. Se borra aquí, en el camino de
	// lectura, y nunca en el del cierre — ese necesita las cifras para calcular lo que guarda.
	if s.arqueoCiego(ctx) {
		ocultarLoEsperado(view.Totals, view.Drawer)
	}
	return view, nil
}

// sessionExpenses lista los gastos atribuidos a un corte para la sección "Gastos" del resumen.
// Slice no-nil → en JSON va como [] (el front no revienta con .map).
func (s *BackofficeService) sessionExpenses(ctx context.Context, sessionID int64) ([]CashExpenseView, error) {
	rows, err := s.store.QC(ctx).ListExpensesBySession(ctx, &sessionID)
	if err != nil {
		return nil, err
	}
	out := make([]CashExpenseView, 0, len(rows))
	for _, e := range rows {
		out = append(out, CashExpenseView{
			ID: e.ID, ExpenseID: e.ExpenseID, Category: e.Category, Supplier: e.Supplier,
			PaymentMethod: e.PaymentMethod,
			Amount:        e.Amount, Currency: domain.Currency(e.Currency), Status: string(e.Status),
		})
	}
	return out, nil
}

// sessionWithExpected arma la vista en vivo de una sesión. La caja PRIMARIA recibe las ventas del
// POS (esperado por método = suma de order_payments desde la apertura); una caja SECUNDARIA no
// vende: solo maneja efectivo (fondo + neto de entradas/salidas y traspasos), así que su único
// esperado es el del método que toca cajón.
func (s *BackofficeService) sessionWithExpected(ctx context.Context, sess db.RegisterSession, reg db.GetCashRegisterRow) (*SessionView, error) {
	rows, err := s.store.QC(ctx).ExpectedByMethodForSession(ctx, &sess.ID)
	if err != nil {
		return nil, err
	}
	// Lo que el dominio necesita para arquear el cajón: qué esperaba cada método y si su dinero está
	// ahí. Se arma en el mismo recorrido que los totales para no leer el catálogo dos veces.
	delCorte := make([]domain.MetodoDelCorte, 0, len(rows))
	// Neto de efectivo movido (entradas − salidas): entra en el esperado del cajón, junto al fondo.
	net, err := s.store.QC(ctx).NetCashMovements(ctx, sess.ID)
	if err != nil {
		return nil, err
	}
	moves, err := s.store.QC(ctx).ListCashMovements(ctx, sess.ID)
	if err != nil {
		return nil, err
	}
	exps, err := s.sessionExpenses(ctx, sess.ID)
	if err != nil {
		return nil, err
	}
	// Lo que falta por entregar del turno. Sale del mismo predicado que la guardia del cierre, así
	// que la pantalla del arqueo no puede decir "todo listo" mientras el botón rebota.
	pendientes, err := s.pedidosSinEntregar(ctx, sess.ID)
	if err != nil {
		return nil, err
	}
	porCajero, err := s.cobradoPorCajero(ctx, sess.ID)
	if err != nil {
		return nil, err
	}
	// Lo que se vendió y nadie pagó. Va junto a Pending y por la misma razón: que el operador lo
	// vea mientras cuenta el efectivo, no cuando alguien audite el corte tres semanas después.
	sinCobrar, err := s.store.QC(ctx).UncollectedInSession(ctx, &sess.ID)
	if err != nil {
		return nil, err
	}
	// Slices no-nil: en JSON van como [] (no null), así el front no revienta con .length/.map.
	view := &SessionView{
		Pending:  pendientes,
		Cashiers: porCajero,
		ID:       sess.ID, RegisterID: reg.ID, RegisterName: reg.Name, IsPrimary: reg.IsPrimary,
		Status: string(sess.Status), OpeningCash: sess.OpeningCash,
		Currency: domain.Currency(sess.Currency), OpenedAt: sess.OpenedAt, NetMovements: domain.Round2(net),
		Totals: []MethodTotal{}, Movements: []CashMovementView{}, Expenses: exps,
		Uncollected: domain.Round2(sinCobrar.Monto), UncollectedCount: int(sinCobrar.Pedidos),
	}
	methods := []methodExpected{}
	for _, r := range rows {
		// Caja secundaria: los métodos no-efectivo no aplican (no vende por ellos) → se omiten.
		if !reg.IsPrimary && !r.AffectsCashDrawer {
			continue
		}
		ventas, tips := r.Expected, r.Tips
		if !reg.IsPrimary {
			ventas, tips = decimal.Zero, decimal.Zero // secundaria: sin ventas ni propinas
		}
		expected := ventas.Add(tips) // ventas + propinas: ambas son dinero recibido en el corte
		// El fondo de apertura y el neto de movimientos son UN solo montón de billetes, así que se
		// suman a UN solo método: el efectivo del mostrador. Antes la condición era
		// `AffectsCashDrawer`, que funcionaba de casualidad mientras ese fuera el único método de
		// cajón; desde que cada plataforma tiene su variante en efectivo hay cuatro, y sumarlo a
		// cada uno reportaba el fondo tantas veces como métodos — un turno con $1,500 y cero
		// ventas salía con $4,500 de faltante que nadie podía explicar.
		//
		// `kind` y no el nombre: los métodos de plataforma en efectivo también tocan el cajón (su
		// dinero se cuenta), pero el fondo no es suyo.
		if r.Kind == db.PaymentKindEfectivo {
			expected = expected.Add(sess.OpeningCash).Add(net) // + fondo + neto de movimientos
		}
		expected, tips = domain.Round2(expected), domain.Round2(tips)
		view.Totals = append(view.Totals, MethodTotal{MethodID: int(r.PaymentMethodID), Name: r.Name,
			Kind: string(r.Kind), Expected: &expected, Tips: tips, AutoDeclare: r.AutoDeclare,
			// Un método cuyo dinero está en el cajón NO pide cifra: su dinero se declara una vez,
			// contándolo. Los demás piden la suya si esperaban algo.
			RequiresEntry: !r.AutoDeclare && !r.AffectsCashDrawer && !expected.IsZero()})
		delCorte = append(delCorte, domain.MetodoDelCorte{
			ID: int(r.PaymentMethodID), Esperado: expected, TocaElCajon: r.AffectsCashDrawer,
		})
		methods = append(methods, methodExpected{
			name: r.Name, expected: expected, tips: tips,
			duenoDelFondo: r.Kind == db.PaymentKindEfectivo,
			plataforma:    r.PlatformName,
		})
	}
	conteos, err := s.conteosDelTurno(ctx, sess.ID)
	if err != nil {
		return nil, err
	}
	view.Counts = conteos
	// El arqueo del cajón, calculado en vivo mientras el turno está abierto: es lo que la pantalla
	// del cierre necesita para decir cuánto debería haber antes de que alguien cuente.
	arqueo, err := arqueoEnVivo(delCorte, conteos)
	if err != nil {
		return nil, err
	}
	view.Drawer = arqueo
	view.Breakdown = corteBreakdown(sess.OpeningCash, methods, moves)
	for _, m := range moves {
		view.Movements = append(view.Movements, CashMovementView{
			ID: m.ID, Kind: m.Kind, Amount: m.Amount, Concept: m.Concept, CreatedAt: m.CreatedAt, UserName: m.UserName, TransferID: m.TransferID, ExpenseID: m.ExpenseID,
		})
	}
	return view, nil
}

// businessDate: el día de negocio de AHORA, en la zona del local. Si la zona no se puede leer cae a
// El default del producto en vez de fallar: abrir caja no se detiene por un ajuste que no se pudo
// leer. Pero el valor del fallback importa tanto como el hecho de tener uno.
//
// Caía a UTC, y eso corre la fecha SEIS HORAS sin avisar: un turno abierto después de las 18:00
// locales queda fechado al día siguiente y todo su dinero entra al arqueo equivocado. Ya pasó — los
// pedidos 61 y 62 de la cuenta de pruebas, del 29 de agosto a las 20:50, están fechados el 30.
//
// El producto se vende en México y nace en `America/Mexico_City`: ese es el fallback que se parece a
// la verdad. UTC no se parece a nada.
func (s *BackofficeService) businessDate(ctx context.Context) time.Time {
	tz, err := s.store.QC(ctx).GetBusinessTimezone(ctx)
	if err != nil {
		tz = domain.DefaultTimezone
	}
	return domain.BusinessDate(s.now(), domain.LoadBusinessLocation(tz))
}

// CloseSession cierra la sesión abierta de una caja, guarda esperado vs declarado por método.
// CierreCmd: cómo se declara el dinero al cerrar el turno.
//
// El EFECTIVO tiene los dos caminos excluyentes de FR-014 —contar piezas, o escribir el total con un
// motivo en `Declarado`— porque es el único que está físicamente en el cajón. Los demás métodos
// siguen mandando su cifra y nada más: no hay piezas que contar en una terminal de tarjeta.
type CierreCmd struct {
	// Declarado: methodId → lo contado, SOLO de los métodos cuyo dinero no está en el cajón. Un
	// método de cajón aquí se rechaza: su dinero se declara una vez, contándolo.
	Declarado map[int]decimal.Decimal
	Piezas    []PiezaCapturada
	// Total y Motivo son el camino manual del CAJÓN, igual que en la apertura: se usa cuando hay algo
	// que el catálogo de denominaciones no puede expresar, y el motivo es obligatorio (FR-014).
	Total  *decimal.Decimal
	Motivo string
	Notas  string
}

func (s *BackofficeService) CloseSession(ctx context.Context, registerID int64, userID int64, cmd CierreCmd) (*SessionView, error) {
	// Copia: el mapa es de quien llama y el efectivo se sustituye por el total del conteo. Mutar el
	// del handler dejaría el cuerpo del request diciendo algo que ya no es.
	declared := make(map[int]decimal.Decimal, len(cmd.Declarado))
	for k, v := range cmd.Declarado {
		declared[k] = v
	}
	// allowZero: un método puede cerrar en 0 (sin ventas). Rechaza negativos y absurdos.
	for _, d := range declared {
		if !domain.ValidMoney(domain.Round2(d), true) {
			return nil, domain.ErrValidation
		}
	}
	reg, err := s.store.QC(ctx).GetCashRegister(ctx, registerID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	sess, err := s.store.QC(ctx).GetOpenSessionByRegister(ctx, registerID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	// Ningún pedido sin terminar sobrevive al cierre. Va ANTES de calcular nada: si el turno se
	// cierra con pendientes, esos pedidos quedan colgados de un arqueo ya firmado y su venta cae en
	// un corte que nadie puede volver a cuadrar. Cerrar es además el momento en que el operador SÍ
	// puede resolverlos: está frente a la caja y el local está vacío.
	if err := s.sinPedidosPendientes(ctx, sess.ID); err != nil {
		return nil, err
	}

	view, err := s.sessionWithExpected(ctx, sess, reg)
	if err != nil {
		return nil, err
	}

	// EL CAJÓN SE DECLARA UNA SOLA VEZ, contándolo. Un método cuyo dinero está ahí no acepta una
	// cifra propia: son dos declaraciones del mismo dinero, y de ahí salía el turno con «Efectivo» en
	// diferencia $0.00 y «Didi efectivo» en −$64.80 sin forma de saber cuál era el faltante real.
	//
	// Se RECHAZA en vez de ignorarse. El caso realista no es un atacante sino una tableta con el
	// front viejo en caché —la aplicación es una PWA con service worker— mandando el cuerpo de antes,
	// y descartar su cifra en silencio deja al operador creyendo que declaró algo que no se guardó.
	delCajon := map[int]bool{}
	if view.Drawer != nil {
		for _, id := range view.Drawer.MethodIDs {
			delCajon[id] = true
		}
	}
	for _, t := range view.Totals {
		if delCajon[t.MethodID] {
			if _, vino := declared[t.MethodID]; vino {
				// La acción primero y sin explicar el mecanismo: quien lee esto está cerrando
				// una caja, no entendiendo el modelo. El nombre del método va porque es lo único
				// que hace el mensaje diagnosticable cuando alguien lo reporta.
				return nil, fmt.Errorf("%w: recarga la pantalla para cerrar: «%s» ahora se cuenta con el cajón",
					domain.ErrValidation, t.Name)
			}
		}
	}

	conteo, err := s.conteoDelCajon(ctx, view.Drawer != nil, cmd)
	if err != nil {
		return nil, err
	}
	// Y SIN CONTEO NO SE CIERRA UN CAJÓN QUE ESPERA DINERO. La pantalla ya bloquea el botón, pero la
	// guardia tiene que estar aquí: abajo cada método del cajón declara su esperado, así que un
	// cierre que llega sin conteo deja los cuatro renglones en diferencia cero y ninguna fila de la
	// cual sacar la real — el corte reportaría $0 con el cajón sin contar. Un cliente viejo en
	// caché, un envío que falla o un doble toque bastan para llegar aquí.
	if view.Drawer != nil && view.Drawer.RequiresCount && conteo == nil {
		return nil, domain.ErrCajonSinContar
	}
	// Cada método del cajón declara SU esperado, con conteo o sin él. No es un truco: es la única
	// forma de escribir "a este método nadie le declaró una cifra propia" en una columna `not null`,
	// y es lo que hace que ninguno reporte una diferencia que cancele la de otro. La diferencia del
	// cajón —la única que existe— vive en la fila del conteo.
	for _, t := range view.Totals {
		if delCajon[t.MethodID] {
			declared[t.MethodID] = esperadoDe(t)
		}
	}

	err = s.store.WithTx(ctx, func(q *db.Queries) error {
		for i := range view.Totals {
			t := &view.Totals[i]
			esperado := esperadoDe(*t)
			t.Declared = domain.Round2(domain.ResolveDeclared(t.AutoDeclare, esperado, declared[t.MethodID]))
			t.Difference = domain.Round2(t.Declared.Sub(esperado))
			if err := q.SaveSessionTotal(ctx, db.SaveSessionTotalParams{
				SessionID: sess.ID, PaymentMethodID: int16(t.MethodID),
				Expected: esperado, Declared: t.Declared, Tips: t.Tips,
				// El snapshot: con qué configuración se cerró este renglón. Leerlo del catálogo al
				// consultar el corte lo reagruparía según el interruptor de hoy.
				AffectsCashDrawer: delCajon[t.MethodID],
			}); err != nil {
				return err
			}
		}
		// DENTRO de la misma transacción que escribe los totales y cierra el turno. Si el conteo
		// fallara después de comprometerse el cierre, quedaría un arqueo firmado cuyo efectivo no se
		// puede reconstruir — y el turno ya cerrado, o sea sin forma de volver a intentarlo.
		if conteo != nil {
			// El esperado del cajón se GUARDA con el conteo: sale de `order_payments`, y una venta
			// cancelada o reembolsada mañana movería la cifra contra la que el operador firmó hoy.
			if err := s.guardarConteo(ctx, q, sess.ID, db.CashCountMomentCierre, conteo.total,
				view.Drawer.Expected, cmd.Piezas, conteo.motivo, userID); err != nil {
				return err
			}
		}
		var n *string
		if cmd.Notas != "" {
			n = &cmd.Notas
		}
		return q.CloseSession(ctx, db.CloseSessionParams{ID: sess.ID, ClosedBy: &userID, Notes: n})
	})
	if err != nil {
		return nil, traduceConflictoDeCaja(err, db.CashCountMomentCierre)
	}
	view.Status = "cerrada"
	if conteo != nil {
		// Se vuelve a leer el conteo ya guardado para que la diferencia que devuelve el cierre sea la
		// de la columna GENERADA y no una resta que este método haga aparte: dos restas del mismo
		// dinero son dos cifras que tarde o temprano difieren.
		conteos, err := s.conteosDelTurno(ctx, sess.ID)
		if err != nil {
			return nil, err
		}
		view.Counts = conteos
		if conteos != nil && conteos.Cierre != nil && view.Drawer != nil {
			view.Drawer.Counted = &conteos.Cierre.Total
			view.Drawer.Difference = conteos.Cierre.Difference
			view.Drawer.RequiresCount = false
		}
	}
	return view, nil
}

// conteoDelEfectivo resuelve los dos caminos de FR-014 para el método del CAJÓN, o devuelve nil si
// en este cierre no se declaró efectivo.
//
// Nil no es lo mismo que cero: una caja secundaria que no manejó efectivo cierra sin nada que
// contar, y guardar un conteo en cero afirmaría "conté el cajón y estaba vacío", que es un hecho
// distinto de "nadie contó". Un arqueo inventado se lee después como si fuera cierto.
//
// Quién decide es `domain.TotalDeclarado`, el MISMO que decide en la apertura: si los dos caminos
// llegan juntos, o si el total a mano viene sin motivo, el error sale de ahí. Repetir esa regla aquí
// sería tenerla en dos capas para que alguien mueva una sola.
func (s *BackofficeService) conteoDelCajon(ctx context.Context, hayCajon bool, cmd CierreCmd) (*conteoResuelto, error) {
	if !hayCajon {
		// Sin ningún método que ponga dinero en el cajón no hay nada que contar; si alguien mandó un
		// conteo, es una pantalla desincronizada y no un cierre que valga guardar a medias.
		if len(cmd.Piezas) > 0 || cmd.Total != nil {
			return nil, fmt.Errorf("%w: este turno no maneja efectivo", domain.ErrValidation)
		}
		return nil, nil
	}

	piezas, err := s.piezasConSuValor(ctx, cmd.Piezas, string(domain.DefaultCurrency))
	if err != nil {
		return nil, err
	}
	// Nada de nada: no se inventa un arqueo. Un conteo en cero afirmaría "conté el cajón y estaba
	// vacío", que es un hecho distinto de "nadie contó". Devolver nil aquí no deja pasar el cierre:
	// quien llama rechaza el nil cuando el cajón esperaba dinero.
	if len(piezas) == 0 && cmd.Total == nil {
		return nil, nil
	}
	total, err := domain.TotalDeclarado(piezas, cmd.Total, cmd.Motivo)
	if err != nil {
		return nil, err
	}
	return &conteoResuelto{total: total, motivo: motivoDelCamino(cmd.Total, cmd.Motivo)}, nil
}

// conteoResuelto: con qué total se declaró el cajón y con qué motivo (nil = se contó por piezas).
type conteoResuelto struct {
	total  decimal.Decimal
	motivo *string
}

// RecordCashMovement registra una entrada/salida de efectivo del cajón en la sesión abierta de una
// caja. El neto (entradas − salidas) entra al efectivo esperado al cerrar (ver sessionWithExpected).
func (s *BackofficeService) RecordCashMovement(ctx context.Context, registerID int64, kind string, amount decimal.Decimal, concept string, userID int64) (*SessionView, error) {
	if !domain.ValidCashKind(kind) {
		return nil, domain.ErrValidation
	}
	amt := domain.Round2(amount)
	if !domain.ValidMoney(amt, false) || concept == "" { // monto > 0 y con concepto
		return nil, domain.ErrValidation
	}
	reg, err := s.store.QC(ctx).GetCashRegister(ctx, registerID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	sess, err := s.store.QC(ctx).GetOpenSessionByRegister(ctx, registerID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound // sin caja abierta no hay dónde registrar
		}
		return nil, err
	}
	if _, err := s.store.QC(ctx).InsertCashMovement(ctx, db.InsertCashMovementParams{
		SessionID: sess.ID, Kind: kind, Amount: amt, Concept: concept, UserID: userID,
	}); err != nil {
		return nil, err
	}
	return s.sessionWithExpected(ctx, sess, reg)
}

// Transfer mueve efectivo de una caja abierta a otra: registra el traspaso y genera, en la MISMA
// tx, la salida en origen + la entrada en destino, ambas ligadas al traspaso → las dos cajas lo
// reflejan de forma atómica ("lo detectan"). Exige ambas cajas abiertas y misma moneda (un
// traspaso no convierte divisa). Devuelve el id del traspaso.
func (s *BackofficeService) Transfer(ctx context.Context, fromRegisterID, toRegisterID int64, amount decimal.Decimal, note string, userID int64) (int64, error) {
	amt := domain.Round2(amount)
	if !domain.ValidTransfer(fromRegisterID, toRegisterID, amt) {
		return 0, domain.ErrValidation
	}
	from, err := s.store.QC(ctx).GetCashRegister(ctx, fromRegisterID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, domain.ErrNotFound
		}
		return 0, err
	}
	to, err := s.store.QC(ctx).GetCashRegister(ctx, toRegisterID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, domain.ErrNotFound
		}
		return 0, err
	}
	fromSess, err := s.openSessionOrConflict(ctx, fromRegisterID)
	if err != nil {
		return 0, err
	}
	toSess, err := s.openSessionOrConflict(ctx, toRegisterID)
	if err != nil {
		return 0, err
	}
	if fromSess.Currency != toSess.Currency {
		return 0, domain.ErrValidation
	}
	var transferID int64
	err = s.store.WithTx(ctx, func(q *db.Queries) error {
		var e error
		transferID, e = q.CreateCashTransfer(ctx, db.CreateCashTransferParams{
			FromSessionID: fromSess.ID, ToSessionID: toSess.ID, Amount: amt, Note: strPtr(note), CreatedBy: userID,
		})
		if e != nil {
			return e
		}
		// Concepto con el nombre de la contraparte al momento del traspaso (snapshot de auditoría:
		// si luego renombran la caja, el histórico conserva cómo se llamaba).
		if e = q.InsertTransferMovement(ctx, db.InsertTransferMovementParams{
			SessionID: fromSess.ID, Kind: domain.CashSalida, Amount: amt,
			Concept: "Traspaso a " + to.Name, UserID: userID, TransferID: &transferID,
		}); e != nil {
			return e
		}
		return q.InsertTransferMovement(ctx, db.InsertTransferMovementParams{
			SessionID: toSess.ID, Kind: domain.CashEntrada, Amount: amt,
			Concept: "Traspaso desde " + from.Name, UserID: userID, TransferID: &transferID,
		})
	})
	if err != nil {
		return 0, err
	}
	return transferID, nil
}

// openSessionOrConflict devuelve la sesión abierta de una caja, o ErrConflict si está cerrada
// (un traspaso exige ambas cajas abiertas).
func (s *BackofficeService) openSessionOrConflict(ctx context.Context, registerID int64) (db.RegisterSession, error) {
	sess, err := s.store.QC(ctx).GetOpenSessionByRegister(ctx, registerID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return sess, domain.ErrConflict
		}
		return sess, err
	}
	return sess, nil
}

// SessionHistory lista los últimos cortes (abiertos y cerrados) para el histórico.
func (s *BackofficeService) SessionHistory(ctx context.Context, limit int32) ([]SessionHistoryRow, error) {
	rows, err := s.store.QC(ctx).ListSessions(ctx, limit)
	if err != nil {
		return nil, err
	}
	out := make([]SessionHistoryRow, len(rows))
	for i, r := range rows {
		out[i] = SessionHistoryRow{
			ID: r.ID, RegisterName: r.RegisterName, Status: string(r.Status), OpeningCash: r.OpeningCash,
			Currency: domain.Currency(r.Currency), OpenedAt: r.OpenedAt, ClosedAt: tsPtr(r.ClosedAt),
			OpenedByName: r.OpenedByName, ClosedByName: r.ClosedByName,
			TotalDifference: r.TotalDifference, Notes: r.Notes,
		}
	}
	return out, nil
}

// SessionDetail devuelve una sesión con sus totales GUARDADOS (snapshot al cerrar) y movimientos.
func (s *BackofficeService) SessionDetail(ctx context.Context, id int64) (*SessionDetailView, error) {
	sess, err := s.store.QC(ctx).GetSession(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	totals, err := s.store.QC(ctx).ListSessionTotals(ctx, id)
	if err != nil {
		return nil, err
	}
	moves, err := s.store.QC(ctx).ListCashMovements(ctx, id)
	if err != nil {
		return nil, err
	}
	exps, err := s.sessionExpenses(ctx, id)
	if err != nil {
		return nil, err
	}
	ventas, cuenta, ingreso, err := s.sessionSales(ctx, id)
	if err != nil {
		return nil, err
	}
	// Misma consulta que el turno abierto: una segunda derivación de la misma cifra es de donde
	// salen dos pantallas que no coinciden.
	sinCobrar, err := s.store.QC(ctx).UncollectedInSession(ctx, &id)
	if err != nil {
		return nil, err
	}
	view := &SessionDetailView{
		ID: sess.ID, RegisterName: sess.RegisterName, Status: string(sess.Status), OpeningCash: sess.OpeningCash,
		Currency: domain.Currency(sess.Currency), OpenedAt: sess.OpenedAt, ClosedAt: tsPtr(sess.ClosedAt),
		OpenedByName: sess.OpenedByName, ClosedByName: sess.ClosedByName, Notes: sess.Notes,
		Totals: []MethodTotal{}, Movements: []CashMovementView{}, Expenses: exps, // no-nil → [] en JSON
		Sales: ventas, SalesCount: cuenta, SalesShown: len(ventas), SalesTotal: ingreso,
		Uncollected: domain.Round2(sinCobrar.Monto), UncollectedCount: int(sinCobrar.Pedidos),
	}
	methods := make([]methodExpected, 0, len(totals))
	// Desde el SNAPSHOT de cada renglón, no del catálogo: así es como el corte cerrado conserva la
	// forma con la que se firmó, aunque el interruptor del método cambie después.
	delCorte := make([]domain.MetodoDelCorte, 0, len(totals))
	for _, t := range totals {
		view.Totals = append(view.Totals, MethodTotal{
			MethodID: int(t.PaymentMethodID), Name: t.Name, Kind: string(t.Kind),
			Expected: &t.Expected, Declared: t.Declared, Difference: t.Difference, Tips: t.Tips,
		})
		delCorte = append(delCorte, domain.MetodoDelCorte{
			ID: int(t.PaymentMethodID), Esperado: t.Expected, TocaElCajon: t.AffectsCashDrawer,
		})
		methods = append(methods, methodExpected{
			name: t.Name, expected: t.Expected, tips: t.Tips,
			duenoDelFondo: t.Kind == db.PaymentKindEfectivo,
			plataforma:    t.PlatformName,
		})
	}
	conteos, err := s.conteosDelTurno(ctx, sess.ID)
	if err != nil {
		return nil, err
	}
	view.Counts = conteos
	view.Drawer = arqueoGuardado(delCorte, conteos)
	view.Breakdown = corteBreakdown(sess.OpeningCash, methods, moves)
	for _, m := range moves {
		view.Movements = append(view.Movements, CashMovementView{
			ID: m.ID, Kind: m.Kind, Amount: m.Amount, Concept: m.Concept, CreatedAt: m.CreatedAt, UserName: m.UserName, TransferID: m.TransferID, ExpenseID: m.ExpenseID,
		})
	}
	// El mismo ocultamiento que en el turno abierto, y aquí no es redundante: este endpoint
	// acepta el id del turno ABIERTO y está abierto a rol cajero, así que nulificar solo el otro
	// camino dejaría la cifra a un request de distancia.
	if string(sess.Status) == "abierta" && s.arqueoCiego(ctx) {
		ocultarLoEsperado(view.Totals, view.Drawer)
	}
	return view, nil
}

// SellingRegisterOpen: ¿se puede cobrar ahora mismo? Contesta EXACTAMENTE la misma pregunta que
// hace OrdersService.Create antes de aceptar una venta (la caja PRINCIPAL con turno abierto), y no
// "¿hay alguna caja abierta?" como antes: con la caja fuerte abierta el POS decía que sí y el cobro
// tronaba hasta el final, con el ticket ya armado.
//
// Chequeo ligero, sin calcular esperados. Disponible a cualquier rol autenticado: saber si el
// negocio está operando no es dato sensible.
func (s *BackofficeService) SellingRegisterOpen(ctx context.Context) (bool, error) {
	estado, err := s.EstadoDeCaja(ctx)
	if err != nil {
		return false, err
	}
	return estado.Open, nil
}

// EstadoDeCajaView: si se puede cobrar, y desde cuándo está abierto el turno que recibe ventas.
type EstadoDeCajaView struct {
	Open bool `json:"open"`
	// OpenedAt y BusinessDate: nil sin turno abierto. La pantalla no los inventa.
	OpenedAt     *time.Time `json:"openedAt,omitempty"`
	BusinessDate *string    `json:"businessDate,omitempty"`
	// DeOtroDia lo decide el SERVIDOR. La zona del negocio vive aqui, y que cada tableta compare
	// fechas contra su propio reloj es la familia de defectos que esta feature viene a cerrar.
	DeOtroDia bool `json:"deOtroDia"`
}

// EstadoDeCaja contesta lo mismo que SellingRegisterOpen y además si el turno abierto ya no es de
// hoy.
//
// Va colgado del endpoint que el POS YA consulta para saber si puede cobrar: el aviso no cuesta un
// viaje más ni un estado nuevo que sincronizar. Y es un AVISO, nunca un bloqueo: un negocio en
// operación prefiere una fecha corrida a una caja parada, y convertir un descuido administrativo en
// un cobro imposible es peor que el descuido.
func (s *BackofficeService) EstadoDeCaja(ctx context.Context) (EstadoDeCajaView, error) {
	sess, err := s.store.QC(ctx).GetOpenPrimarySession(ctx)
	if errors.Is(err, pgx.ErrNoRows) {
		return EstadoDeCajaView{}, nil
	}
	if err != nil {
		return EstadoDeCajaView{}, err
	}
	tz, err := s.store.QC(ctx).GetBusinessTimezone(ctx)
	if err != nil {
		tz = domain.DefaultTimezone
	}
	zona := domain.LoadBusinessLocation(tz)
	fecha := sess.BusinessDate.Time.Format("2006-01-02")
	return EstadoDeCajaView{
		Open: true, OpenedAt: &sess.OpenedAt, BusinessDate: &fecha,
		DeOtroDia: domain.TurnoDeOtroDia(sess.OpenedAt, s.now(), zona),
	}, nil
}

// tsPtr convierte un timestamptz anulable de pgx a *time.Time (nil si NULL).
func tsPtr(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	return &t.Time
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
	rows, err := s.store.QC(ctx).ListAllSuppliers(ctx)
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
	r, err := s.store.QC(ctx).CreateSupplier(ctx, db.CreateSupplierParams{Name: name, Phone: phone, Notes: notes})
	if err != nil {
		return SupplierView{}, err
	}
	return SupplierView{ID: r.ID, Name: r.Name, Phone: r.Phone, Notes: r.Notes, IsActive: r.IsActive}, nil
}

func (s *BackofficeService) UpdateSupplier(ctx context.Context, id int64, name string, phone, notes *string, active bool) (SupplierView, error) {
	if name == "" {
		return SupplierView{}, domain.ErrValidation
	}
	r, err := s.store.QC(ctx).UpdateSupplier(ctx, db.UpdateSupplierParams{ID: id, Name: name, Phone: phone, Notes: notes, IsActive: active})
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
	rows, err := s.store.QC(ctx).ListAllExpenseCategories(ctx)
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
	r, err := s.store.QC(ctx).CreateExpenseCategory(ctx, db.CreateExpenseCategoryParams{Name: name, FinancialGroup: db.FinancialGroup(group)})
	if err != nil {
		return ExpenseCategoryView{}, err
	}
	return ExpenseCategoryView{ID: r.ID, Name: r.Name, FinancialGroup: string(r.FinancialGroup), IsActive: r.IsActive}, nil
}

func (s *BackofficeService) UpdateExpenseCategory(ctx context.Context, id int64, name, group string, active bool) (ExpenseCategoryView, error) {
	if name == "" || !validFinancialGroup(group) {
		return ExpenseCategoryView{}, domain.ErrValidation
	}
	r, err := s.store.QC(ctx).UpdateExpenseCategory(ctx, db.UpdateExpenseCategoryParams{ID: id, Name: name, FinancialGroup: db.FinancialGroup(group), IsActive: active})
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
	return s.store.QC(ctx).ListStockLevels(ctx)
}
func (s *BackofficeService) StockMovements(ctx context.Context, limit int32) ([]db.ListStockMovementsRow, error) {
	return s.store.QC(ctx).ListStockMovements(ctx, limit)
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
	return s.store.QC(ctx).InsertStockMovement(ctx, db.InsertStockMovementParams{
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
	return s.store.QC(ctx).SalesByDay(ctx, db.SalesByDayParams{
		BusinessDate:   pgtype.Date{Time: from, Valid: true},
		BusinessDate_2: pgtype.Date{Time: to, Valid: true},
	})
}
func (s *BackofficeService) SalesByMethod(ctx context.Context, from, to time.Time) ([]db.SalesByMethodRow, error) {
	return s.store.QC(ctx).SalesByMethod(ctx, db.SalesByMethodParams{
		BusinessDate:   pgtype.Date{Time: from, Valid: true},
		BusinessDate_2: pgtype.Date{Time: to, Valid: true},
	})
}
func (s *BackofficeService) ProductMargins(ctx context.Context, from, to time.Time, limit int32) ([]db.ProductMarginsRow, error) {
	return s.store.QC(ctx).ProductMargins(ctx, db.ProductMarginsParams{
		BusinessDate:   pgtype.Date{Time: from, Valid: true},
		BusinessDate_2: pgtype.Date{Time: to, Valid: true},
		Limit:          limit,
	})
}

// Location: la zona del NEGOCIO, para que el rango de un reporte se resuelva en el día del local.
//
// Mismo modo de falla y mismo fallback que `businessDate`: si la zona no se puede leer se cae al
// default del producto y no a UTC, porque UTC corre la fecha seis horas sin avisar y el reporte
// contestaría un periodo que nadie pidió después de las 18:00 locales.
func (s *BackofficeService) Location(ctx context.Context) *time.Location {
	tz, err := s.store.QC(ctx).GetBusinessTimezone(ctx)
	if err != nil {
		tz = domain.DefaultTimezone
	}
	return domain.LoadBusinessLocation(tz)
}

// Now expone el reloj del servicio para que el handler resuelva el preset con el mismo instante que
// usa el resto del backoffice (los tests lo fijan).
func (s *BackofficeService) Now() time.Time { return s.now() }

func (s *BackofficeService) TipsByEmployee(ctx context.Context, from, to time.Time) ([]db.TipsByEmployeeRow, error) {
	return s.store.QC(ctx).TipsByEmployee(ctx, db.TipsByEmployeeParams{
		BusinessDate: pgtype.Date{Time: from, Valid: true}, BusinessDate_2: pgtype.Date{Time: to, Valid: true},
	})
}

func (s *BackofficeService) TipsByDay(ctx context.Context, from, to time.Time) ([]db.TipsByDayRow, error) {
	return s.store.QC(ctx).TipsByDay(ctx, db.TipsByDayParams{
		BusinessDate: pgtype.Date{Time: from, Valid: true}, BusinessDate_2: pgtype.Date{Time: to, Valid: true},
	})
}

// sinPedidosPendientes falla nombrando los folios que faltan por terminar.
//
// Los folios van en el mensaje a propósito: un "hay pedidos sin terminar" a secas manda al operador
// a recorrer el tablero comparando, justo cuando está cerrando y con prisa.
func (s *BackofficeService) sinPedidosPendientes(ctx context.Context, sessionID int64) error {
	pendientes, err := s.pedidosSinEntregar(ctx, sessionID)
	if err != nil {
		return err
	}
	if len(pendientes) == 0 {
		return nil
	}
	partes := make([]string, 0, len(pendientes))
	for _, p := range pendientes {
		// El nombre primero porque es lo que se lee en el tablero y lo que se canta; el número va
		// entre paréntesis para quien busque por ticket. Los pedidos viejos no tienen nombre.
		num := "#" + strconv.Itoa(p.Number)
		if p.Name != "" {
			partes = append(partes, p.Name+" ("+num+")")
			continue
		}
		partes = append(partes, num)
	}
	return fmt.Errorf("%w: %s. Entrégalos o cancélalos antes de cerrar",
		domain.ErrOpenOrders, strings.Join(partes, ", "))
}

// pedidosSinEntregar lista los pedidos del turno que todavía no salen.
//
// Es la fuente ÚNICA de esa lista: la usa el resumen del arqueo y la usa la guardia que impide
// cerrar. Derivarlas por separado dejaría a la pantalla diciendo una cosa y al botón haciendo otra,
// y quien lo lee no tendría cómo saber cuál de las dos miente.
//
// Cobrado no entra en la cuenta: lo que impide cerrar es la comida que no ha salido, no el dinero.
func (s *BackofficeService) pedidosSinEntregar(ctx context.Context, sessionID int64) ([]PendingOrder, error) {
	filas, err := s.store.QC(ctx).OpenOrdersInSession(ctx, &sessionID)
	if err != nil {
		return nil, err
	}
	out := make([]PendingOrder, 0, len(filas))
	for _, f := range filas {
		out = append(out, PendingOrder{Number: int(f.DailyNumber), Name: derefStr(f.FolioName)})
	}
	return out, nil
}

// cobradoPorCajero: cuánto cobró cada persona en el turno.
//
// Es la respuesta a "dos estaciones, un solo cajón". Crear una caja por Surface daría dos arqueos
// contando el mismo dinero físico —dos cifras inventadas—, así que la caja sigue siendo una y lo
// que se separa es quién cobró. El dato existía desde el principio en received_by y solo lo usaba
// el reparto de propinas.
func (s *BackofficeService) cobradoPorCajero(ctx context.Context, sessionID int64) ([]CashierTotal, error) {
	filas, err := s.store.QC(ctx).SessionCashByCashier(ctx, &sessionID)
	if err != nil {
		return nil, err
	}
	out := make([]CashierTotal, 0, len(filas))
	for _, f := range filas {
		out = append(out, CashierTotal{
			Name: f.Cashier, Cash: f.Cash, Other: f.Other, Payments: int(f.Payments),
		})
	}
	return out, nil
}

// sessionSales lee las ventas de un corte junto con su conteo y su ingreso.
//
// La lista y el resumen salen de dos consultas con el MISMO where a propósito: si divergen, una de
// las dos miente y quien lee la pantalla no tiene forma de saber cuál.
//
// El tope es el mismo `MaxListLimit` del resto de las listas. Se devuelve la cuenta REAL aparte para
// que la pantalla pueda decir cuántas hay: recortar en silencio se lee como "esto es todo".
func (s *BackofficeService) sessionSales(ctx context.Context, id int64) ([]SessionSaleView, int, decimal.Decimal, error) {
	q := s.store.QC(ctx)
	rows, err := q.SessionSales(ctx, db.SessionSalesParams{RegisterSessionID: &id, Lim: maxVentasDeCorte})
	if err != nil {
		return nil, 0, decimal.Zero, err
	}
	resumen, err := q.CountSessionSales(ctx, &id)
	if err != nil {
		return nil, 0, decimal.Zero, err
	}
	ventas := make([]SessionSaleView, 0, len(rows)) // no-nil → [] en JSON, nunca null
	for _, r := range rows {
		ventas = append(ventas, SessionSaleView{
			ID: r.ID, DailyNumber: int(r.DailyNumber), FolioName: r.FolioName, OpenedAt: r.OpenedAt,
			Status: string(r.Status), ServiceType: string(r.ServiceType),
			Total: r.Total, Refund: r.RefundAmount,
		})
	}
	return ventas, int(resumen.Total), resumen.Ingreso, nil
}

// SessionSalesPage devuelve una página de las ventas de un corte.
//
// Existe porque el detalle trae hasta `maxVentasDeCorte` y un corte más grande no se podía recorrer
// completo desde ninguna parte: la pantalla decía cuántas había —no mentía— pero el resto era
// inalcanzable, y un arqueo que no se puede auditar entero no se puede auditar.
//
// Endpoint aparte y no un parámetro del detalle: pedir la página 3 no debería recalcular el arqueo,
// los gastos y lo cobrado por persona.
func (s *BackofficeService) SessionSalesPage(ctx context.Context, id int64, limit, offset int32) ([]SessionSaleView, int, decimal.Decimal, error) {
	q := s.store.QC(ctx)
	rows, err := q.SessionSales(ctx, db.SessionSalesParams{RegisterSessionID: &id, Lim: limit, Off: offset})
	if err != nil {
		return nil, 0, decimal.Zero, err
	}
	resumen, err := q.CountSessionSales(ctx, &id)
	if err != nil {
		return nil, 0, decimal.Zero, err
	}
	ventas := make([]SessionSaleView, 0, len(rows))
	for _, r := range rows {
		ventas = append(ventas, SessionSaleView{
			ID: r.ID, DailyNumber: int(r.DailyNumber), FolioName: r.FolioName, OpenedAt: r.OpenedAt,
			Status: string(r.Status), ServiceType: string(r.ServiceType),
			Total: r.Total, Refund: r.RefundAmount,
		})
	}
	return ventas, int(resumen.Total), resumen.Ingreso, nil
}
