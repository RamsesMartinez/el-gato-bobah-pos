package app

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
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
}

func (s *BackofficeService) PaymentMethods(ctx context.Context) ([]PaymentMethodView, error) {
	rows, err := s.store.QC(ctx).ListPaymentMethods(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]PaymentMethodView, len(rows))
	for i, r := range rows {
		out[i] = PaymentMethodView{ID: int(r.ID), Name: r.Name, Kind: string(r.Kind), AffectsCashDrawer: r.AffectsCashDrawer, AutoDeclare: r.AutoDeclare}
	}
	return out, nil
}

// SetPaymentMethodAutoDeclare marca a nivel negocio si un método de pago se declara solo al
// cerrar caja (declarado = esperado, sin captura del cajero) o requiere conteo manual.
func (s *BackofficeService) SetPaymentMethodAutoDeclare(ctx context.Context, methodID int, auto bool) (PaymentMethodView, error) {
	current, err := s.store.QC(ctx).GetPaymentMethod(ctx, int16(methodID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return PaymentMethodView{}, domain.ErrNotFound
		}
		return PaymentMethodView{}, err
	}
	// Efectivo (affects_cash_drawer) es justo el método que exige conteo físico: auto-declararlo
	// dejaría el corte de caja sin forma de detectar un faltante de efectivo.
	if auto && current.AffectsCashDrawer {
		return PaymentMethodView{}, domain.ErrValidation
	}
	row, err := s.store.QC(ctx).UpdatePaymentMethodAutoDeclare(ctx, db.UpdatePaymentMethodAutoDeclareParams{
		ID: int16(methodID), AutoDeclare: auto,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return PaymentMethodView{}, domain.ErrNotFound
		}
		return PaymentMethodView{}, err
	}
	return PaymentMethodView{ID: int(row.ID), Name: row.Name, Kind: string(row.Kind), AffectsCashDrawer: row.AffectsCashDrawer, AutoDeclare: row.AutoDeclare}, nil
}

// ---- Cortes de caja ----

type MethodTotal struct {
	MethodID    int             `json:"methodId"`
	Name        string          `json:"name"`
	Expected    decimal.Decimal `json:"expected"` // incluye ventas + propinas (+ fondo/neto en efectivo)
	Declared    decimal.Decimal `json:"declared"`
	Difference  decimal.Decimal `json:"difference"`
	Tips        decimal.Decimal `json:"tips"` // propinas del método (parte del esperado; línea aparte en el resumen)
	AutoDeclare bool            `json:"autoDeclare"`
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

	out := CorteBreakdown{Ingresos: []CorteMethodBreakdown{}, Egresos: []CorteBucket{}}
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
func (s *BackofficeService) OpenSession(ctx context.Context, registerID int64, openingCash decimal.Decimal, userID int64) (*SessionView, error) {
	// allowZero: abrir con cajón vacío es válido. Rechaza negativos (la columna no tiene
	// check) e importes absurdos antes de que desborden el numeric(10,2).
	if !domain.ValidMoney(domain.Round2(openingCash), true) {
		return nil, domain.ErrValidation
	}
	reg, err := s.activeRegister(ctx, registerID)
	if err != nil {
		return nil, err
	}
	if _, err := s.store.QC(ctx).GetOpenSessionByRegister(ctx, registerID); err == nil {
		return nil, domain.ErrConflict // esa caja ya está abierta
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}
	sess, err := s.store.QC(ctx).OpenSession(ctx, db.OpenSessionParams{
		BusinessDate: pgtype.Date{Time: s.businessDate(ctx), Valid: true},
		OpeningCash:  domain.Round2(openingCash),
		OpenedBy:     userID,
		RegisterID:   registerID,
	})
	if err != nil {
		return nil, err
	}
	return s.sessionWithExpected(ctx, sess, reg)
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
	return s.sessionWithExpected(ctx, sess, reg)
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
	rows, err := s.store.QC(ctx).ExpectedByMethodSince(ctx, sess.OpenedAt)
	if err != nil {
		return nil, err
	}
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
	// Slices no-nil: en JSON van como [] (no null), así el front no revienta con .length/.map.
	view := &SessionView{
		ID: sess.ID, RegisterID: reg.ID, RegisterName: reg.Name, IsPrimary: reg.IsPrimary,
		Status: string(sess.Status), OpeningCash: sess.OpeningCash,
		Currency: domain.Currency(sess.Currency), OpenedAt: sess.OpenedAt, NetMovements: domain.Round2(net),
		Totals: []MethodTotal{}, Movements: []CashMovementView{}, Expenses: exps,
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
		view.Totals = append(view.Totals, MethodTotal{MethodID: int(r.PaymentMethodID), Name: r.Name, Expected: expected, Tips: tips, AutoDeclare: r.AutoDeclare})
		methods = append(methods, methodExpected{name: r.Name, expected: expected, tips: tips, duenoDelFondo: r.Kind == db.PaymentKindEfectivo})
	}
	view.Breakdown = corteBreakdown(sess.OpeningCash, methods, moves)
	for _, m := range moves {
		view.Movements = append(view.Movements, CashMovementView{
			ID: m.ID, Kind: m.Kind, Amount: m.Amount, Concept: m.Concept, CreatedAt: m.CreatedAt, UserName: m.UserName, TransferID: m.TransferID, ExpenseID: m.ExpenseID,
		})
	}
	return view, nil
}

// businessDate: el día de negocio de AHORA, en la zona del local. Si la zona no se puede leer cae a
// UTC en vez de fallar: abrir caja no se detiene por un ajuste mal escrito, y el peor caso es la
// fecha corrida que ya se tenía antes de que esto existiera.
func (s *BackofficeService) businessDate(ctx context.Context) time.Time {
	tz, err := s.store.QC(ctx).GetBusinessTimezone(ctx)
	if err != nil {
		return domain.BusinessDate(s.now(), time.UTC)
	}
	return domain.BusinessDate(s.now(), domain.LoadBusinessLocation(tz))
}

// CloseSession cierra la sesión abierta de una caja, guarda esperado vs declarado por método.
func (s *BackofficeService) CloseSession(ctx context.Context, registerID int64, userID int64, declared map[int]decimal.Decimal, notes string) (*SessionView, error) {
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
	view, err := s.sessionWithExpected(ctx, sess, reg)
	if err != nil {
		return nil, err
	}
	err = s.store.WithTx(ctx, func(q *db.Queries) error {
		for i := range view.Totals {
			t := &view.Totals[i]
			t.Declared = domain.Round2(domain.ResolveDeclared(t.AutoDeclare, t.Expected, declared[t.MethodID]))
			t.Difference = domain.Round2(t.Declared.Sub(t.Expected))
			if err := q.SaveSessionTotal(ctx, db.SaveSessionTotalParams{
				SessionID: sess.ID, PaymentMethodID: int16(t.MethodID),
				Expected: t.Expected, Declared: t.Declared, Tips: t.Tips,
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
	view := &SessionDetailView{
		ID: sess.ID, RegisterName: sess.RegisterName, Status: string(sess.Status), OpeningCash: sess.OpeningCash,
		Currency: domain.Currency(sess.Currency), OpenedAt: sess.OpenedAt, ClosedAt: tsPtr(sess.ClosedAt),
		OpenedByName: sess.OpenedByName, ClosedByName: sess.ClosedByName, Notes: sess.Notes,
		Totals: []MethodTotal{}, Movements: []CashMovementView{}, Expenses: exps, // no-nil → [] en JSON
	}
	methods := make([]methodExpected, 0, len(totals))
	for _, t := range totals {
		view.Totals = append(view.Totals, MethodTotal{
			MethodID: int(t.PaymentMethodID), Name: t.Name,
			Expected: t.Expected, Declared: t.Declared, Difference: t.Difference, Tips: t.Tips,
		})
		methods = append(methods, methodExpected{name: t.Name, expected: t.Expected, tips: t.Tips, duenoDelFondo: t.Kind == db.PaymentKindEfectivo})
	}
	view.Breakdown = corteBreakdown(sess.OpeningCash, methods, moves)
	for _, m := range moves {
		view.Movements = append(view.Movements, CashMovementView{
			ID: m.ID, Kind: m.Kind, Amount: m.Amount, Concept: m.Concept, CreatedAt: m.CreatedAt, UserName: m.UserName, TransferID: m.TransferID, ExpenseID: m.ExpenseID,
		})
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
	_, err := s.store.QC(ctx).GetOpenPrimarySession(ctx)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
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
func (s *BackofficeService) SalesByMethod(ctx context.Context, since time.Time) ([]db.SalesByMethodRow, error) {
	return s.store.QC(ctx).SalesByMethod(ctx, since)
}
func (s *BackofficeService) ProductMargins(ctx context.Context, since time.Time, limit int32) ([]db.ProductMarginsRow, error) {
	return s.store.QC(ctx).ProductMargins(ctx, db.ProductMarginsParams{OpenedAt: since, Limit: limit})
}

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
