package app

import (
	"context"
	"errors"
	"fmt"
	"slices"
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
// BackofficeService es la operación de backoffice. Lo de caja vive en sus propios servicios
// (CashShiftService, CashMovementService, CashReportService) y llega aquí incrustado: los handlers y
// las pruebas siguen llamando backoffice.CloseSession, y el código de cada parte está en su archivo.
type BackofficeService struct {
	cashDeps
	*CashShiftService
	*CashMovementService
	*CashReportService
}

func NewBackofficeService(s *store.Store, now func() time.Time) *BackofficeService {
	if now == nil {
		now = time.Now
	}
	deps := cashDeps{store: s, now: now}
	return &BackofficeService{
		cashDeps:            deps,
		CashShiftService:    &CashShiftService{deps},
		CashMovementService: &CashMovementService{deps},
		CashReportService:   &CashReportService{deps},
	}
}

// ---- Medios de pago ----

type PaymentMethodView struct {
	ID                int    `json:"id"`
	Name              string `json:"name"`
	Kind              string `json:"kind"`
	AffectsCashDrawer bool   `json:"affectsCashDrawer"`
	AutoDeclare       bool   `json:"autoDeclare"`
	IsActive          bool   `json:"isActive"`
	// IsCash: este método se cobra en billetes. Es propiedad del método y no configuración: lo que
	// el negocio decide es si ese efectivo llega a su cajón, no si el cliente pagó en efectivo.
	// La pantalla lo usa para no ofrecer interruptores que el servidor va a rechazar.
	IsCash bool `json:"isCash"`
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
		out[i] = PaymentMethodView{ID: int(r.ID), Name: r.Name, Kind: string(r.Kind), IsActive: true,
			AffectsCashDrawer: r.AffectsCashDrawer, AutoDeclare: r.AutoDeclare, DeliveryPlatformID: r.DeliveryPlatformID}
	}
	return out, nil
}

// AllPaymentMethods: los métodos INCLUIDOS LOS APAGADOS, para la pantalla de ajustes.
//
// Separada de `PaymentMethods` por la misma razón que `AllCashRegisters` lo está de
// `CashRegisters`: la lista con la que se cobra no puede traer los apagados, y la lista con la que
// se configuran tiene que traerlos o apagar uno lo borra de la pantalla que tiene su interruptor.
func (s *BackofficeService) AllPaymentMethods(ctx context.Context) ([]PaymentMethodView, error) {
	rows, err := s.store.QC(ctx).ListAllPaymentMethods(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]PaymentMethodView, len(rows))
	for i, r := range rows {
		out[i] = PaymentMethodView{ID: int(r.ID), Name: r.Name, Kind: string(r.Kind),
			IsCash: r.IsCash, AffectsCashDrawer: r.AffectsCashDrawer, AutoDeclare: r.AutoDeclare,
			IsActive: r.IsActive, DeliveryPlatformID: r.DeliveryPlatformID}
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
		if err := domain.FlagsDelMetodoValidos(actual.IsCash,
			actual.Kind == db.PaymentKindEfectivo, tocaElCajon, autoDeclara); err != nil {
			return fmt.Errorf("%w (%s)", err, actual.Name)
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
			ID: int(row.ID), Name: row.Name, Kind: string(row.Kind), IsCash: actual.IsCash,
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
	Expected   *decimal.Decimal `json:"expected"` // incluye ventas + propinas (+ fondo/neto en efectivo)
	Declared   decimal.Decimal  `json:"declared"`
	Difference decimal.Decimal  `json:"difference"`
	// Tips: propinas del método NETAS de la propina devuelta en el turno (spec 029), con la misma
	// regla que el reporte de propinas. El esperado sigue llevando la bruta —el dinero entró— y la
	// devuelta baja por su devolución; por eso el cierre guarda `grossTips`, no esta.
	Tips        decimal.Decimal `json:"tips"`
	grossTips   decimal.Decimal
	AutoDeclare bool `json:"autoDeclare"`
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
	// IsRefund: la salida de caja de una devolución (spec 029). El desglose la cuenta en
	// «Devoluciones» y la tabla la nombra igual, no como una salida más.
	IsRefund bool `json:"isRefund"`
	// ReversesID: es el reverso de esa salida; Reversed: esta salida ya se corrigió (spec 032).
	ReversesID *int64 `json:"reversesId"`
	Reversed   bool   `json:"reversed"`
	// RecipientName: a quién se le dio (propina entregada o retiro al cierre); nulo en lo demás.
	RecipientName *string `json:"recipientName"`
}

// WithdrawalView es el retiro al cierre (spec 035): lo contado menos el fondo que se dejó, y quién
// se lo llevó.
type WithdrawalView struct {
	Amount   decimal.Decimal `json:"amount"`
	UserName string          `json:"userName"`
}

// ocultarElFondoDejado quita de un corte cerrado todo lo que dice cuánto se quedó en el cajón: el
// fondo, el fijo contra el que se comparó (casi siempre es el mismo número) y el retiro, en sus
// tres lugares. Lo contado del último cierre lo quita ocultarElArqueoDelUltimoCierre (R8).
func ocultarElFondoDejado(view *SessionDetailView) {
	view.FloatLeft, view.FixedFloatAtClose, view.Withdrawal = nil, nil, nil
	movs := view.Movements[:0]
	for _, m := range view.Movements {
		if m.Kind != domain.CashRetiro {
			movs = append(movs, m)
		}
	}
	view.Movements = movs
	egresos := []CorteBucket{}
	for _, e := range view.Breakdown.Egresos {
		if e.Concept == conceptoRetiro {
			view.Breakdown.EgresosTotal = domain.Round2(view.Breakdown.EgresosTotal.Sub(e.Amount))
			continue
		}
		egresos = append(egresos, e)
	}
	view.Breakdown.Egresos = egresos
}

// ocultarElArqueoDelUltimoCierre quita del último cierre de una caja todo lo que da lo contado: el
// conteo de cierre, el arqueo del cajón, el esperado y la diferencia de cada método (en los del
// cajón lo declarado ES el esperado) y los ingresos, que sumados al fondo y al neto dan el esperado.
// Sin retiro, lo contado es el fondo dejado, o sea el esperado de la siguiente apertura (R8). La
// apertura no se toca: es lo que el propio cajero contó. Se va el renglón entero y no se pone en
// cero: un $0 se lee como «no hubo», y ArqueoHidden le dice a la pantalla que hay algo oculto.
func ocultarElArqueoDelUltimoCierre(view *SessionDetailView) {
	view.ArqueoHidden = true
	if view.Counts != nil {
		view.Counts.Cierre = nil
	}
	view.Drawer = nil
	view.Totals = []MethodTotal{}
	view.Breakdown.Ingresos = []CorteMethodBreakdown{}
	view.Breakdown.IngresosTotal = decimal.Zero
	view.Breakdown.Plataformas = []CortePlatformSubtotal{}
}

// conceptoNoEsDelNegocio es el renglón de egresos de lo que salió del cajón con un gasto y no era
// del negocio (spec 050).
const conceptoNoEsDelNegocio = "No es del negocio"

// conceptoRetiro es el renglón del retiro en los egresos del corte.
const conceptoRetiro = "Retiro al cierre"

// withdrawalOf saca el retiro de los movimientos del turno; nil si no hubo.
func withdrawalOf(moves []db.ListCashMovementsRow) *WithdrawalView {
	for _, m := range moves {
		if m.Kind == domain.CashRetiro {
			w := &WithdrawalView{Amount: m.Amount}
			if m.RecipientName != nil {
				w.UserName = *m.RecipientName
			}
			return w
		}
	}
	return nil
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
	// DrawerExtra: lo que salió del cajón además del importe del pago (spec 050).
	DrawerExtra decimal.Decimal `json:"drawerExtra"`
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
	// Note explica un medio en negativo (spec 029): sin ella el cajero busca un faltante que no
	// existe. Vacía en cualquier otro caso.
	Note string `json:"note,omitempty"`
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
	// earlier: cobros del turno de pedidos de OTRO turno; ya están en `expected` (spec 031, D12).
	earlier decimal.Decimal
	// refunded: devuelto en el turno sin sacarlo del cajón; ya se RESTÓ de `expected` (D6).
	refunded decimal.Decimal
	// refunds: todo lo devuelto en el turno por este medio, del cajón o no, con su propina. Es lo que
	// se PRESENTA en «Devoluciones» (spec 029); no mueve el esperado.
	refunds domain.MethodRefunds
}

// corteBreakdown descompone un corte en ingresos (por método → concepto) y egresos de efectivo.
// ventas se derivan del esperado restando propinas y (en efectivo) fondo + neto de movimientos, así
// una caja secundaria (esperado = fondo + neto) da 0 ventas sola. Propinas se listan aparte.
func corteBreakdown(opening decimal.Decimal, methods []methodExpected, moves []db.ListCashMovementsRow) CorteBreakdown {
	var entradas, traspasosIn, salidas, traspasosOut, gastos, noNegocio, propinas, retiro, net decimal.Decimal
	for _, m := range moves {
		// El retiro al cierre se registra DESPUÉS de contar: ni es parte del esperado ni salida del
		// negocio. Restarlo del neto inflaba «Ventas» por el monto del retiro (spec 035).
		if m.Kind == domain.CashRetiro {
			retiro = retiro.Add(m.Amount)
			continue
		}
		if m.Kind == domain.CashEntrada || m.Kind == domain.CashReverso {
			net = net.Add(m.Amount)
		} else {
			net = net.Sub(m.Amount)
		}
		switch {
		case m.Kind == domain.CashReverso:
			// Corrige una salida a mano (domain.CanReverse): la descuenta de las salidas.
			salidas = salidas.Sub(m.Amount)
		case m.Kind == domain.CashPropina:
			// Dinero del personal que sale del cajón: ni gasto ni salida del negocio (spec 032).
			propinas = propinas.Add(m.Amount)
		case m.IsRefund:
			// La salida de caja de una devolución se presenta en «Devoluciones» de su medio, como la
			// de tarjeta (spec 029). Sigue en el neto: el esperado no cambia.
		case m.ExpenseID != nil: // gasto en efectivo (siempre salida)
			// La parte del ticket que no es del negocio salió del cajón pero no es gasto (spec 050):
			// contarla en «Gastos» reportaría un gasto mayor que el registrado.
			gastos = gastos.Add(m.Amount.Sub(m.DrawerExtra))
			noNegocio = noNegocio.Add(m.DrawerExtra)
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
		// Lo que no es venta de ESTE turno sale de «Ventas» y se nombra: la devolución ya se restó del
		// esperado y vuelve a sumarse aquí para mostrarse aparte, en negativo.
		ventas = domain.Round2(ventas.Add(me.refunded).Sub(me.earlier))
		items := []CorteBucket{}
		total := decimal.Zero
		add := func(concept string, amt decimal.Decimal) {
			if !amt.IsZero() {
				items = append(items, CorteBucket{Concept: concept, Amount: amt})
				total = total.Add(amt)
			}
		}
		add("Ventas", ventas)
		add("Cobros de otros turnos", domain.Round2(me.earlier))
		// Propina neta y venta devuelta por separado, salga o no del cajón: la misma clasificación
		// que Ventas y que el reporte de propinas (spec 029).
		add("Propinas", domain.Round2(me.tips.Sub(me.refunds.Tips())))
		add("Devoluciones", me.refunds.Sale().Neg())
		if me.duenoDelFondo {
			add("Entradas", domain.Round2(entradas))
			add("Traspasos recibidos", domain.Round2(traspasosIn))
		}
		if len(items) > 0 {
			out.Ingresos = append(out.Ingresos, CorteMethodBreakdown{Method: me.name, Total: domain.Round2(total), Items: items,
				Note: domain.NegativeMethodNote(domain.Round2(total))})
			out.IngresosTotal = out.IngresosTotal.Add(total)
		}
		if me.plataforma != "" && !total.IsZero() {
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
		{Concept: conceptoNoEsDelNegocio, Amount: domain.Round2(noNegocio)},
		{Concept: "Salidas de efectivo", Amount: domain.Round2(salidas)},
		{Concept: "Traspasos enviados", Amount: domain.Round2(traspasosOut)},
		{Concept: "Propinas entregadas", Amount: domain.Round2(propinas)},
		{Concept: conceptoRetiro, Amount: domain.Round2(retiro)},
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
	// corteMethods: los esperados con los que se armó el desglose. El cierre los reusa para rehacerlo
	// con el retiro, que nace después de armar la vista.
	corteMethods []methodExpected
	// Arqueo de tarjeta (spec 032): el modo del turno, las terminales que hay que declarar al cerrar
	// y, cerrado, lo declarado contra lo cobrado por terminal.
	CardCountMode string `json:"cardCountMode"`
	// Salidas a mano sin concepto del turno (las de antes de los conceptos): aviso del cierre.
	CashOutsWithoutConcept int                 `json:"cashOutsWithoutConcept"`
	TerminalsToCount       []TerminalToCount   `json:"terminalsToCount"`
	TerminalCounts         []TerminalCountView `json:"terminalCounts"`
	// Propinas (spec 032): por entregar, entregadas en el turno y cuánto de lo entregado era
	// propina de tarjeta u otro medio pagada con efectivo del cajón.
	TipsPending        *TipsPendingView   `json:"tipsPending"`
	TipsPaidOut        decimal.Decimal    `json:"tipsPaidOut"`
	CardTipsPaidInCash decimal.Decimal    `json:"cardTipsPaidInCash"`
	ID                 int64              `json:"id"`
	RegisterID         int64              `json:"registerId"`
	RegisterName       string             `json:"registerName"`
	IsPrimary          bool               `json:"isPrimary"` // la caja primaria recibe las ventas del POS
	Status             string             `json:"status"`
	OpeningCash        decimal.Decimal    `json:"openingCash"`
	Currency           domain.Currency    `json:"currency"`
	OpenedAt           time.Time          `json:"openedAt"`
	NetMovements       decimal.Decimal    `json:"netMovements"` // entradas − salidas de efectivo
	Totals             []MethodTotal      `json:"totals"`
	Movements          []CashMovementView `json:"movements"`
	Expenses           []CashExpenseView  `json:"expenses"`
	Breakdown          CorteBreakdown     `json:"breakdown"`
	// Blind: este turno se está contando a ciegas, así que la vista viene SIN el desglose de la
	// venta ni lo cobrado por cada cajero.
	//
	// Viaja como bandera en vez de dejar que la pantalla lo deduzca de que las listas vengan
	// vacías: un turno sin ventas también las trae vacías, y la pantalla diría «Sin ingresos» —que
	// es cierto en un caso y mentira en el otro— sobre la misma respuesta.
	Blind bool `json:"blind"`
	// Pending son los pedidos del turno que todavía no se terminan de entregar. Salen del MISMO
	// predicado que bloquea el cierre, no de una consulta parecida: si la pantalla y la guardia se
	// derivaran por separado, una de las dos mentiría y quien la lee no tendría cómo saber cuál.
	//
	// Se listan aunque ya estén cobrados: cobrado y entregado son cosas distintas, y lo que impide
	// cerrar es la comida que no ha salido, no el dinero.
	Pending []PendingOrder `json:"pending"`
	// Owing son los pedidos de mostrador entregados que todavía deben, de cualquier día. Bloquean el
	// cierre de la caja principal (no hay fiados: decisión del dueño, 2026-10-09) y salen del MISMO
	// predicado que la guardia. Vacío en una caja secundaria, que no vende. Siempre arreglo.
	Owing []domain.OwingOrder `json:"owing"`
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
	// WrittenOff: lo dado por perdido de los pedidos del turno («cancelar lo que falta», 2026-10-09).
	// No está en Uncollected ni en el esperado: vendido = cobrado + sin cobrar + perdido.
	WrittenOff decimal.Decimal `json:"writtenOff"`
	// Counts: lo que se contó al abrir. Va en la MISMA forma que en el detalle del corte y sale del
	// mismo lugar: dos derivaciones del mismo desglose son dos pantallas que pueden no coincidir.
	Counts *ConteosDelTurno `json:"counts"`
	// Drawer: el arqueo del cajón físico. Nil = este turno no tiene arqueo de efectivo (un corte
	// anterior a la spec 015, o una caja que no maneja efectivo).
	Drawer *ArqueoDelCajonView `json:"drawer"`
	// VoidedPayments: los pagos devueltos en el turno (spec 027). Lista aparte y no una salida: el
	// pago devuelto ya no está en order_payments, así que el esperado por método no cambia de
	// fórmula. Siempre arreglo.
	VoidedPayments []VoidedPaymentView `json:"voidedPayments"`
	// LiveAccounts son las cuentas vivas que NO bloquean el cierre (spec 030, D-10): las que se están
	// capturando y las entregadas que deben, de cualquier día. Se listan para abrirlas o descartarlas
	// antes de cerrar; las que bloquean siguen en Pending, con su guardia sin cambio. Siempre arreglo.
	LiveAccounts []AccountItem `json:"liveAccounts"`
	// Refunds: el dinero que se le devolvió al cliente en este turno (spec 031). No es lo mismo que
	// un pago devuelto: el pago sigue en pie y la devolución resta aparte. Las que no salieron del
	// cajón ya bajaron el esperado de su medio; las de efectivo, por su salida de caja. Siempre
	// arreglo.
	Refunds []RefundView `json:"refunds"`
	// Fondo fijo y retiro (spec 035). FixedFloat va solo en el turno abierto: es lo que la hoja de
	// cierre pone en el campo. FloatLeft, FixedFloatAtClose y Withdrawal salen del cierre; nulos
	// mientras el turno está abierto y Withdrawal nulo si no hubo retiro.
	FixedFloat        *decimal.Decimal `json:"fixedFloat"`
	FloatLeft         *decimal.Decimal `json:"floatLeft"`
	FixedFloatAtClose *decimal.Decimal `json:"fixedFloatAtClose"`
	Withdrawal        *WithdrawalView  `json:"withdrawal"`
	// Difference: la diferencia del corte en sus tres partes (#290). Solo en la respuesta del cierre;
	// un turno abierto no tiene diferencia porque nada se ha contado.
	Difference *domain.CutDiff `json:"difference"`
}

// RefundView es una devolución del turno, para la lista del corte.
type RefundView struct {
	Method     string          `json:"method"`
	Amount     decimal.Decimal `json:"amount"`
	Tip        decimal.Decimal `json:"tip"`
	OrderFolio string          `json:"orderFolio"`
	// FromDrawer: salió del cajón (tiene salida de caja). La pantalla no lo suma: lo dice.
	FromDrawer bool      `json:"fromDrawer"`
	RefundedBy string    `json:"refundedBy"`
	RefundedAt time.Time `json:"refundedAt"`
	Reason     string    `json:"reason"`
}

// VoidedPaymentView es un pago devuelto en el turno, para la lista del corte.
type VoidedPaymentView struct {
	Method     string          `json:"method"`
	Amount     decimal.Decimal `json:"amount"`
	Tip        decimal.Decimal `json:"tip"`
	OrderFolio string          `json:"orderFolio"`
	VoidedBy   string          `json:"voidedBy"`
	VoidedAt   time.Time       `json:"voidedAt"`
	Reason     string          `json:"reason"`
}

// CashierTotal es lo que cobró una persona en el turno. El efectivo va aparte de lo demás porque
// es lo único que está físicamente en el cajón: una diferencia de arqueo solo puede venir de ahí.
type CashierTotal struct {
	Name     string          `json:"name"`
	Cash     decimal.Decimal `json:"cash"`
	Other    decimal.Decimal `json:"other"`
	Payments int             `json:"payments"`
}

// PendingOrder es un pedido del turno que sigue sin entregarse. El id viaja para que el cierre
// ofrezca «Abrir» esa cuenta en el POS en vez de mandar a buscarla por nombre, y el total para que
// diga de cuánto es sin abrirla.
type PendingOrder struct {
	ID     int64           `json:"id"`
	Number int             `json:"number"`
	Name   string          `json:"name"`
	Total  decimal.Decimal `json:"total"`
}

// CashRegisterView es una caja del catálogo. OpenSessionID no-nil = tiene una sesión abierta.
type CashRegisterView struct {
	ID            int64  `json:"id"`
	Name          string `json:"name"`
	IsPrimary     bool   `json:"isPrimary"`
	IsActive      bool   `json:"isActive"`
	CountsCash    bool   `json:"countsCash"`
	OpenSessionID *int64 `json:"openSessionId"`
}

// CashRegisterConfigView es una caja para la gestión: incluye el fondo fijo (spec 035), que la lista
// del cajero NO lleva porque la apertura se cuenta a ciegas.
type CashRegisterConfigView struct {
	ID         int64            `json:"id"`
	Name       string           `json:"name"`
	IsPrimary  bool             `json:"isPrimary"`
	IsActive   bool             `json:"isActive"`
	CountsCash bool             `json:"countsCash"`
	FixedFloat *decimal.Decimal `json:"fixedFloat"`
}

// SessionDetailView es una sesión (normalmente ya cerrada) con sus totales guardados y
// movimientos, para el histórico. Difiere de SessionView en que los totales vienen de
// register_session_totals (snapshot al cerrar), no del cálculo en vivo.
type SessionDetailView struct {
	// Arqueo de tarjeta y propinas del turno (spec 032), lo mismo que muestra el turno abierto. En
	// uno cerrado salen de lo que guardó el cierre: el conteo por terminal y la propina que se quedó
	// en caja (tips_carried_over).
	CardCountMode      string              `json:"cardCountMode"`
	TerminalCounts     []TerminalCountView `json:"terminalCounts"`
	TipsPaidOut        decimal.Decimal     `json:"tipsPaidOut"`
	CardTipsPaidInCash decimal.Decimal     `json:"cardTipsPaidInCash"`
	TipsCarriedOver    decimal.Decimal     `json:"tipsCarriedOver"`
	ID                 int64               `json:"id"`
	RegisterName       string              `json:"registerName"`
	Status             string              `json:"status"`
	OpeningCash        decimal.Decimal     `json:"openingCash"`
	Currency           domain.Currency     `json:"currency"`
	OpenedAt           time.Time           `json:"openedAt"`
	ClosedAt           *time.Time          `json:"closedAt"`
	OpenedByName       string              `json:"openedByName"`
	ClosedByName       *string             `json:"closedByName"`
	Notes              *string             `json:"notes"`
	Totals             []MethodTotal       `json:"totals"`
	Movements          []CashMovementView  `json:"movements"`
	Expenses           []CashExpenseView   `json:"expenses"`
	Breakdown          CorteBreakdown      `json:"breakdown"`
	// ArqueoHidden: ver SessionHistoryRow.ArqueoHidden. Con él en true, totales, arqueo, conteo de
	// cierre e ingresos vienen vacíos porque se ocultaron, no porque no hubo.
	ArqueoHidden bool `json:"arqueoHidden"`

	// Las ventas que este corte cobró. Viven aquí y no en un filtro de la pantalla de Ventas: ahí
	// convivirían con el filtro de fechas y bastaría elegir un rango que no toque el corte para
	// llegar a una pantalla vacía sin explicación.
	Sales []SessionSaleView `json:"sales"`
	// VoidedPayments: ver SessionView.VoidedPayments.
	VoidedPayments []VoidedPaymentView `json:"voidedPayments"`
	// Refunds: ver SessionView.Refunds.
	Refunds []RefundView `json:"refunds"`
	// Cuántas hay EN TOTAL, no cuántas se mandaron. Un recorte silencioso se lee como "esto es todo".
	SalesCount int `json:"salesCount"`
	SalesShown int `json:"salesShown"`
	// Lo que se dejó de fondo, contra qué fijo, y el retiro (spec 035). Nulos en cierres de antes.
	FloatLeft         *decimal.Decimal `json:"floatLeft"`
	FixedFloatAtClose *decimal.Decimal `json:"fixedFloatAtClose"`
	Withdrawal        *WithdrawalView  `json:"withdrawal"`
	// Suma de las ventas que dejaron ingreso: sin canceladas, sin reembolsadas y sin propinas. La
	// pantalla declara las tres exclusiones.
	SalesTotal decimal.Decimal `json:"salesTotal"`
	// Uncollected es la parte de SalesTotal que ningún pago cubre. Sin ella las dos cifras de esta
	// misma pantalla —lo vendido y lo esperado por método— no cuadran y nadie puede saber cuál de
	// las dos miente: es el corolario del principio III, y aquí importa más que en el turno abierto
	// porque ESTA es la pantalla que alguien audita cuando ya nadie se acuerda del turno.
	Uncollected      decimal.Decimal `json:"uncollected"`
	UncollectedCount int             `json:"uncollectedCount"`
	// WrittenOff: lo dado por perdido de los pedidos del turno («cancelar lo que falta», 2026-10-09).
	// No está en Uncollected ni en el esperado: vendido = cobrado + sin cobrar + perdido.
	WrittenOff decimal.Decimal `json:"writtenOff"`
	// Counts: el desglose de los dos arqueos del turno. Es lo que convierte un faltante en algo
	// investigable — "faltan dos billetes de $500" en vez de "faltan $1,000".
	Counts *ConteosDelTurno `json:"counts"`
	// Drawer: el arqueo del cajón físico. Nil = este turno no tiene arqueo de efectivo (un corte
	// anterior a la spec 015, o una caja que no maneja efectivo).
	Drawer *ArqueoDelCajonView `json:"drawer"`
	// Difference: la diferencia del corte en sus tres partes (#290), con la misma función que la
	// lista y el cierre. Nil en un turno abierto y cuando el arqueo está oculto (ArqueoHidden).
	Difference *domain.CutDiff `json:"difference"`
	// Lo mismo que la fila del Histórico (#290), más el corte contra el que comparó esta apertura.
	OpeningReason     *string          `json:"openingReason"`
	OpeningReasonNote *string          `json:"openingReasonNote"`
	ExplainedByNext   *NextOpeningView `json:"explainedByNext"`
	PreviousSessionID *int64           `json:"previousSessionId"`
}

// NextOpeningView es la apertura del turno siguiente que explica la diferencia de un corte.
//
// La nota va como `reasonNote` y no `note`: el MCP borra las llaves `note`/`notes` por ser texto de
// terceros, y ésta la escribe el personal del negocio; el agente la necesita (FR-009).
type NextOpeningView struct {
	SessionID  int64   `json:"sessionId"`
	Reason     string  `json:"reason"`
	ReasonNote *string `json:"reasonNote"`
}

// explanationOf devuelve la apertura del turno siguiente como explicación de este corte, o nil si no
// abrió con motivo o si el corte cuadró: decir «diferencia explicada» junto a «Cuadra» afirma una
// diferencia que no hubo (el siguiente pudo abrir con «cambié el fondo» por otra razón).
func explanationOf(diff *domain.CutDiff, id int64, reason, note *string) *NextOpeningView {
	if id == 0 || reason == nil || diff == nil || diff.Total.IsZero() {
		return nil
	}
	return &NextOpeningView{SessionID: id, Reason: *reason, ReasonNote: note}
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
	ID           int64           `json:"id"`
	RegisterName string          `json:"registerName"`
	Status       string          `json:"status"`
	OpeningCash  decimal.Decimal `json:"openingCash"`
	Currency     domain.Currency `json:"currency"`
	OpenedAt     time.Time       `json:"openedAt"`
	ClosedAt     *time.Time      `json:"closedAt"`
	OpenedByName string          `json:"openedByName"`
	ClosedByName *string         `json:"closedByName"`
	// TotalDifference: nil cuando el arqueo está oculto (ArqueoHidden). Es Difference.Total; se
	// conserva mientras haya clientes que lo lean (la PWA anterior durante la subida y el MCP).
	TotalDifference *decimal.Decimal `json:"totalDifference"`
	// Difference: la diferencia en sus tres partes (#290). Nil en un turno abierto y con el arqueo
	// oculto.
	Difference *domain.CutDiff `json:"difference"`
	// OpeningReason y OpeningReasonNote: por qué la apertura de este turno no coincidió con el cierre
	// anterior (spec 032). Los ve todo el que ve el corte: no dicen cuánto se esperaba (#290, P2).
	OpeningReason     *string `json:"openingReason"`
	OpeningReasonNote *string `json:"openingReasonNote"`
	// ExplainedByNext: el turno siguiente de la misma caja abrió con motivo, y ese motivo explica la
	// diferencia de este corte. Se oculta junto con el arqueo: su existencia dice que hubo diferencia.
	ExplainedByNext *NextOpeningView `json:"explainedByNext"`
	Notes           *string          `json:"notes"`
	// ArqueoHidden: con arqueo ciego, el último cierre de una caja que no se ha vuelto a abrir no le
	// muestra su arqueo a quien no configura la caja: lo contado es el esperado de la siguiente
	// apertura (spec 035, R8). Viaja para que la pantalla diga «oculto» y no pinte $0.
	ArqueoHidden bool `json:"arqueoHidden"`
}

// CashRegisters lista las cajas activas + el id de su sesión abierta (para pickers y la vista de caja).
func (s *BackofficeService) CashRegisters(ctx context.Context) ([]CashRegisterView, error) {
	rows, err := s.store.QC(ctx).ListCashRegisters(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]CashRegisterView, len(rows))
	for i, r := range rows {
		out[i] = CashRegisterView{ID: r.ID, Name: r.Name, IsPrimary: r.IsPrimary, IsActive: r.IsActive, CountsCash: r.CountsCash, OpenSessionID: r.OpenSessionID}
	}
	return out, nil
}

// AllCashRegisters lista TODAS las cajas (incl. inactivas) para la gestión; sin estado de sesión.
func (s *BackofficeService) AllCashRegisters(ctx context.Context) ([]CashRegisterConfigView, error) {
	rows, err := s.store.QC(ctx).ListAllCashRegisters(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]CashRegisterConfigView, len(rows))
	for i, r := range rows {
		out[i] = CashRegisterConfigView{ID: r.ID, Name: r.Name, IsPrimary: r.IsPrimary, IsActive: r.IsActive, CountsCash: r.CountsCash, FixedFloat: r.FixedFloat}
	}
	return out, nil
}

// SetFixedFloat fija (o quita, con nil) lo que se queda en el cajón al cerrar esta caja. Cambia lo
// que proponen los cierres siguientes, nunca los que ya se hicieron (spec 035).
func (s *BackofficeService) SetFixedFloat(ctx context.Context, id int64, fixed *decimal.Decimal) (CashRegisterConfigView, error) {
	if fixed != nil {
		v := domain.Round2(*fixed)
		if !domain.ValidMoney(v, true) {
			return CashRegisterConfigView{}, fmt.Errorf("%w: el fondo fijo no es una cantidad válida", domain.ErrValidation)
		}
		fixed = &v
	}
	r, err := s.store.QC(ctx).SetRegisterFixedFloat(ctx, db.SetRegisterFixedFloatParams{ID: id, FixedFloat: fixed})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return CashRegisterConfigView{}, domain.ErrNotFound
		}
		return CashRegisterConfigView{}, err
	}
	return CashRegisterConfigView{ID: r.ID, Name: r.Name, IsPrimary: r.IsPrimary, IsActive: r.IsActive, CountsCash: r.CountsCash, FixedFloat: r.FixedFloat}, nil
}

func (s *BackofficeService) CreateCashRegister(ctx context.Context, name string) (CashRegisterView, error) {
	if name == "" {
		return CashRegisterView{}, domain.ErrValidation
	}
	r, err := s.store.QC(ctx).CreateCashRegister(ctx, name)
	if err != nil {
		return CashRegisterView{}, err
	}
	return CashRegisterView{ID: r.ID, Name: r.Name, IsPrimary: r.IsPrimary, IsActive: r.IsActive, CountsCash: true}, nil
}

// UpdateCashRegister renombra, activa o cambia «lleva arqueo». countsCash nil = no se toca.
func (s *BackofficeService) UpdateCashRegister(ctx context.Context, id int64, name string, isActive bool, countsCash *bool) (CashRegisterView, error) {
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
	var r db.UpdateCashRegisterRow
	err = s.store.WithTx(ctx, func(q *db.Queries) error {
		// Quitar el arqueo, con la caja tomada: una apertura simultánea espera y se decide con lo que
		// quedó, en vez de dejar un turno abierto en una caja que ya no se cuenta.
		if countsCash != nil && !*countsCash {
			locked, err := q.GetCashRegisterForUpdate(ctx, id)
			if err != nil {
				return err
			}
			if locked.IsPrimary {
				return domain.ErrPrimaryRegisterCounts
			}
			if _, err := q.GetOpenSessionByRegister(ctx, id); err == nil {
				return domain.ErrRegisterHasOpenShift
			} else if !errors.Is(err, pgx.ErrNoRows) {
				return err
			}
		}
		var err error
		r, err = q.UpdateCashRegister(ctx, db.UpdateCashRegisterParams{ID: id, Name: name, IsActive: isActive, CountsCash: countsCash})
		return err
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return CashRegisterView{}, domain.ErrNotFound
		}
		return CashRegisterView{}, err
	}
	return CashRegisterView{ID: r.ID, Name: r.Name, IsPrimary: r.IsPrimary, IsActive: r.IsActive, CountsCash: r.CountsCash}, nil
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
//
// NULIFICAR `expected` NO BASTA, y creer que sí es lo que convierte este control en un adorno. La
// misma respuesta llevaba el desglose por método y lo que cobró cada cajero, y la pantalla los
// pinta ARRIBA de la tabla del cierre: fondo 500 + neto 0 + Ventas del mostrador 200 + Ventas de
// Didi efectivo 135 = 835, que era exactamente el esperado oculto. Quien cuenta no necesitaba las
// herramientas del navegador, solo sumar cuatro renglones contiguos.
//
// Por eso se va TODO lo que descompone la venta del turno: los ingresos por método, los subtotales
// por plataforma y lo cobrado por cada cajero. Lo que se queda es lo que el propio operador
// registró y ya conoce —el fondo con el que abrió, sus movimientos de efectivo y las salidas—:
// esconderlo no agregaría protección y sí le quitaría la pantalla con la que trabaja.
func ocultarLoEsperado(totals []MethodTotal, drawer *ArqueoDelCajonView,
	desglose *CorteBreakdown, cajeros []CashierTotal) {
	for i := range totals {
		totals[i].Expected = nil
	}
	if drawer != nil {
		drawer.Expected = nil
		drawer.Difference = nil
	}
	desglose.Ingresos = []CorteMethodBreakdown{}
	desglose.IngresosTotal = decimal.Zero
	desglose.Plataformas = []CortePlatformSubtotal{}
	for i := range cajeros {
		cajeros[i].Cash = decimal.Zero
		cajeros[i].Other = decimal.Zero
	}
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
	// Reason y ReasonNote: por qué lo contado no coincide con el cierre anterior (spec 032).
	Reason     string
	ReasonNote string
	// OpenedAt: abrir desde una hora anterior (spec 048). nil = la del primer gasto sin corte, o
	// ahora si no hay. El permiso lo revisa el handler; aquí se valida la hora.
	OpenedAt *time.Time
	// ExcludePending: abrir «ahora» dejando sin incluir los gastos sin corte (R13). Exige permiso.
	ExcludePending bool
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
	case strings.Contains(pgErr.ConstraintName, "register_cash_movements_one_withdrawal"):
		// Segunda barrera: el candado del cierre ya impide dos cierres del mismo turno.
		return fmt.Errorf("%w: este turno ya registró su retiro; recarga para ver el cierre",
			domain.ErrConflict)
	case strings.Contains(pgErr.ConstraintName, "session_cash_count_lines_unicas"):
		return fmt.Errorf("%w: llegó la misma denominación dos veces en el conteo",
			domain.ErrValidation)
	}
	return err
}

// movementViews: los movimientos del turno como los ve la pantalla. Una sola función para el turno
// abierto, la respuesta del cierre y el corte reabierto, que antes los armaban cada uno a su modo.
func movementViews(moves []db.ListCashMovementsRow) []CashMovementView {
	out := make([]CashMovementView, 0, len(moves))
	for _, m := range moves {
		out = append(out, CashMovementView{
			ID: m.ID, Kind: m.Kind, Amount: m.Amount, Concept: m.Concept, CreatedAt: m.CreatedAt, UserName: m.UserName, TransferID: m.TransferID, ExpenseID: m.ExpenseID,
			IsRefund: m.IsRefund, ReversesID: m.ReversesID, Reversed: m.Reversed, RecipientName: m.RecipientName,
		})
	}
	return out
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
	// Propinas: qué se hace con la propina pendiente (spec 032). Obligatoria si queda al menos un
	// peso; «quedan_en_caja» la hereda el siguiente turno de esta caja.
	Propinas string
	// FloatLeft: cuánto se deja de fondo para el siguiente turno; el resto se retira (decisión del
	// 2026-10-10). Ausente = se deja todo lo contado. No puede pasar de lo contado.
	FloatLeft *decimal.Decimal
	// FixedFloatShown: el fondo fijo que la hoja de cierre mostró (spec 035). Es el snapshot contra
	// el que se compara lo que se dejó; ausente = el fijo vigente al cerrar.
	FixedFloatShown *decimal.Decimal
	// WithdrawalUserID: quién se lleva el retiro. Ausente = quien cierra (y lo que manda una tableta
	// con el front anterior).
	WithdrawalUserID *int64
	// TerminalCounts: terminal → total de su corte, con arqueo por terminal (spec 032, punto 9).
	TerminalCounts map[int64]decimal.Decimal
	// Perms: los permisos de quien cierra. Solo los pide el cierre autorizado.
	Perms []domain.Permission
	// AuthorizedReason: cerrar sin la salida de quien abrió, y por qué (spec 046). Ausente = cierre
	// normal; se ignora si quien abrió ya no tiene entrada abierta.
	AuthorizedReason *string
}

// TerminalCountView es el arqueo de una terminal al cierre (spec 032, punto 9).
type TerminalCountView struct {
	TerminalID int64           `json:"terminalId"`
	Name       string          `json:"name"`
	Expected   decimal.Decimal `json:"expected"`
	Declared   decimal.Decimal `json:"declared"`
	Difference decimal.Decimal `json:"difference"`
	// ExpectedNow y DifferenceNow: lo que se espera HOY de la terminal en ese turno y su diferencia
	// contra lo declarado. Difieren de Expected y Difference cuando alguien cambió la terminal de un
	// pago después del arqueo (spec 036); lo firmado no se reescribe.
	ExpectedNow   decimal.Decimal `json:"expectedNow"`
	DifferenceNow decimal.Decimal `json:"differenceNow"`
}

// TerminalToCount es una terminal cuyo total se pide al cerrar.
type TerminalToCount struct {
	TerminalID int64  `json:"terminalId"`
	Name       string `json:"name"`
}

// conteoResuelto: con qué total se declaró el cajón y con qué motivo (nil = se contó por piezas).
type conteoResuelto struct {
	total  decimal.Decimal
	motivo *string
}

// RecordCashMovement registra una entrada/salida de efectivo del cajón en la sesión abierta de una
// caja. El neto (entradas − salidas) entra al efectivo esperado al cerrar (ver sessionWithExpected).
// CashMovementCmd es una entrada o salida de efectivo capturada a mano. Una salida lleva concepto
// del catálogo (spec 032, punto 3); una entrada, texto libre.
type CashMovementCmd struct {
	Kind      string          `json:"kind"`
	Amount    decimal.Decimal `json:"amount"`
	Concept   string          `json:"concept"`
	ConceptID *int64          `json:"conceptId"`
	UserID    int64           `json:"-"`
}

// cashMovementConcept resuelve el texto del movimiento: el nombre del concepto (copiado, para que
// renombrarlo no reescriba el corte) en una salida, el texto libre en una entrada.
func cashMovementConcept(ctx context.Context, q *db.Queries, cmd CashMovementCmd) (string, error) {
	if cmd.Kind != domain.CashSalida {
		if cmd.Concept == "" {
			return "", domain.ErrValidation
		}
		return cmd.Concept, nil
	}
	if cmd.ConceptID == nil {
		return "", domain.ErrConceptRequired
	}
	c, err := q.GetCashConcept(ctx, *cmd.ConceptID)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && c.ArchivedAt.Valid) {
		return "", domain.ErrConceptRequired
	}
	if err != nil {
		return "", err
	}
	return c.Name, nil
}

// cutDifferenceOf arma la diferencia de un corte cerrado con domain.CutDifference desde lo que ya
// trae la vista. Nil si falta un esperado (arqueo ciego): sin esperado no hay diferencia que dar.
func cutDifferenceOf(totals []MethodTotal, drawer *ArqueoDelCajonView) *domain.CutDiff {
	enElCajon := map[int]bool{}
	var conteo *domain.DrawerCount
	if drawer != nil {
		for _, id := range drawer.MethodIDs {
			enElCajon[id] = true
		}
		if drawer.Counted != nil {
			conteo = &domain.DrawerCount{Expected: drawer.Expected, Counted: *drawer.Counted}
		}
	}
	methods := make([]domain.MethodDiff, 0, len(totals))
	for _, t := range totals {
		if t.Expected == nil {
			return nil
		}
		methods = append(methods, domain.MethodDiff{Expected: *t.Expected, Declared: t.Declared, TouchesDrawer: enElCajon[t.MethodID]})
	}
	diff := domain.CutDifference(methods, conteo)
	return &diff
}

// refundsOf arma lo devuelto por un medio desde las cuatro columnas del esperado del turno.
func refundsOf(offDrawer, offDrawerTips, drawer, drawerTips decimal.Decimal) domain.MethodRefunds {
	return domain.MethodRefunds{OffDrawer: offDrawer, OffDrawerTips: offDrawerTips, Drawer: drawer, DrawerTips: drawerTips}
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
		Desde: pgtype.Date{Time: from, Valid: true},
		Hasta: pgtype.Date{Time: to, Valid: true},
	})
}

// ProductsSold son las unidades vendidas por producto, sueltas y dentro de paquetes.
func (s *BackofficeService) ProductsSold(ctx context.Context, from, to time.Time, limit int32) ([]db.ProductsSoldRow, error) {
	return s.store.QC(ctx).ProductsSold(ctx, db.ProductsSoldParams{
		FromDate: pgtype.Date{Time: from, Valid: true},
		ToDate:   pgtype.Date{Time: to, Valid: true},
		RowLimit: limit,
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

// MoveSales pasa la venta de la caja que vende en la sucursal a `targetID` (spec 049, R6). Las dos
// cajas se toman en orden de id y se valida bajo candado: abrir turno toma la caja con el mismo
// FOR UPDATE, así que no se cuela un turno entre la validación y el cambio.
func (s *BackofficeService) MoveSales(ctx context.Context, targetID, userID int64) error {
	return s.store.WithTx(ctx, func(q *db.Queries) error {
		rows, err := q.LockRegistersForSalesMove(ctx, targetID)
		if err != nil {
			return err
		}
		var current, target *db.LockRegistersForSalesMoveRow
		ids := make([]int64, 0, len(rows))
		for i := range rows {
			ids = append(ids, rows[i].ID)
			if rows[i].IsPrimary {
				current = &rows[i]
			}
			if rows[i].ID == targetID {
				target = &rows[i]
			}
		}
		if target == nil {
			return domain.ErrNotFound
		}
		if current == nil {
			return fmt.Errorf("%w: esta sucursal no tiene caja que venda", domain.ErrConflict)
		}
		open, err := q.RegistersWithOpenShift(ctx, ids)
		if err != nil {
			return err
		}
		reg := func(r *db.LockRegistersForSalesMoveRow) domain.SellingRegister {
			return domain.SellingRegister{ID: r.ID, BranchID: r.BranchID, Active: r.IsActive, HasOpenShift: slices.Contains(open, r.ID)}
		}
		changes, err := domain.CanMoveSales(reg(current), reg(target))
		if err != nil || !changes {
			return err
		}
		if err := q.SetRegisterSells(ctx, db.SetRegisterSellsParams{ID: current.ID, Sells: false}); err != nil {
			return err
		}
		if err := q.SetRegisterSells(ctx, db.SetRegisterSellsParams{ID: target.ID, Sells: true}); err != nil {
			return err
		}
		return auditEntity(ctx, q, domain.AuditCashSellingRegisterChanged, "cash_register", target.ID, &target.BranchID, &userID,
			map[string]any{"registerId": current.ID}, map[string]any{"registerId": target.ID})
	})
}
