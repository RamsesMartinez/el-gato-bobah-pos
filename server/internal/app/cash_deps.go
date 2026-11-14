package app

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/logging"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store/db"
	"github.com/shopspring/decimal"
)

// cashDeps es lo que comparten los servicios de caja: la base, el reloj y las lecturas que usan
// más de uno (el turno con su esperado, los conteos, el arqueo ciego). BackofficeService lo incrusta,
// así que sus otros métodos siguen viéndolas (spec 082, fase 2).
type cashDeps struct {
	store *store.Store
	now   func() time.Time
}

// vistaDelTurnoAbierto: la vista que ve una PANTALLA, ya filtrada por el arqueo ciego.
//
// Existe para que el filtrado no sea responsabilidad de cada llamador. Lo era, y de los cuatro
// caminos que devuelven esta vista dos se lo saltaban: registrar un movimiento de caja —abierto a
// rol cajero— devolvía el esperado completo, así que una entrada de un centavo alcanzaba para
// leerlo. El único que usa la vista cruda es el cierre, que necesita las cifras para calcular lo
// que guarda, y eso se ve en que llama a `sessionWithExpected` a propósito.
func (s *cashDeps) vistaDelTurnoAbierto(ctx context.Context, sess db.RegisterSession, reg db.GetCashRegisterRow) (*SessionView, error) {
	view, err := s.sessionWithExpected(ctx, sess, reg, true)
	if err != nil {
		return nil, err
	}
	if s.arqueoCiego(ctx) {
		ocultarLoEsperado(view.Totals, view.Drawer, &view.Breakdown, view.Cashiers)
		view.Blind = true
	}
	return view, nil
}

// sessionWithExpected arma la vista en vivo de una sesión. La caja PRIMARIA recibe las ventas del
// POS (esperado por método = suma de order_payments desde la apertura); una caja SECUNDARIA no
// vende: solo maneja efectivo (fondo + neto de entradas/salidas y traspasos), así que su único
// esperado es el del método que toca cajón.
func (s *cashDeps) sessionWithExpected(ctx context.Context, sess db.RegisterSession, reg db.GetCashRegisterRow, withLiveAccounts bool) (*SessionView, error) {
	rows, err := s.store.QC(ctx).ExpectedByMethodForSession(ctx, sess.ID)
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
		// El fijo que la hoja de cierre pone en el campo (spec 035).
		FixedFloat: reg.FixedFloat,
	}
	if view.WrittenOff, err = s.store.QC(ctx).SessionWrittenOff(ctx, &sess.ID); err != nil {
		return nil, err
	}
	if view.VoidedPayments, err = s.voidedPayments(ctx, sess.ID); err != nil {
		return nil, err
	}
	view.LiveAccounts = []AccountItem{}
	if view.Owing, err = s.owingOrders(ctx, reg.IsPrimary); err != nil {
		return nil, err
	}
	if withLiveAccounts {
		if view.LiveAccounts, err = s.liveAccountsForClosing(ctx, reg.IsPrimary); err != nil {
			return nil, err
		}
	}
	if view.Refunds, err = s.sessionRefunds(ctx, sess.ID); err != nil {
		return nil, err
	}
	methods := []methodExpected{}
	for _, r := range rows {
		// Caja secundaria: los métodos no-efectivo no aplican (no vende por ellos) → se omiten.
		if !reg.IsPrimary && !r.AffectsCashDrawer {
			continue
		}
		ventas, tips, earlier, refunded := r.Expected, r.Tips, r.Earlier, r.Refunded
		if !reg.IsPrimary {
			// secundaria: sin ventas, propinas ni devoluciones
			ventas, tips, earlier, refunded = decimal.Zero, decimal.Zero, decimal.Zero, decimal.Zero
		}
		// ventas + propinas: ambas son dinero recibido en el corte. Menos lo que se le devolvió al
		// cliente por este medio sin pasar por el cajón (spec 031, D6): con «declarar automático» el
		// corte firmaba como recibido dinero que la terminal ya había regresado.
		expected := ventas.Add(tips).Sub(refunded)
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
		devuelto := domain.MethodRefunds{}
		if reg.IsPrimary {
			devuelto = refundsOf(r.Refunded, r.RefundedTips, r.DrawerRefunded, r.DrawerRefundedTips)
		}
		// Con arqueo por terminal la tarjeta se declara una vez, por terminal (spec 032): el método
		// toma lo esperado y la diferencia sale en el conteo de cada terminal.
		autoDeclara := r.AutoDeclare || (r.Kind == db.PaymentKindTarjeta && domain.CardCountMode(sess.CardCountMode) == domain.CardCountPerTerminal)
		view.Totals = append(view.Totals, MethodTotal{MethodID: int(r.PaymentMethodID), Name: r.Name,
			Kind: string(r.Kind), Expected: &expected, Tips: domain.Round2(tips.Sub(devuelto.Tips())), grossTips: tips,
			AutoDeclare: autoDeclara,
			// Un método cuyo dinero está en el cajón NO pide cifra: su dinero se declara una vez,
			// contándolo. Los demás piden la suya si esperaban algo.
			RequiresEntry: !autoDeclara && !r.AffectsCashDrawer && !expected.IsZero()})
		delCorte = append(delCorte, domain.MetodoDelCorte{
			ID: int(r.PaymentMethodID), Esperado: expected, TocaElCajon: r.AffectsCashDrawer,
		})
		methods = append(methods, methodExpected{
			name: r.Name, expected: expected, tips: tips,
			duenoDelFondo: r.Kind == db.PaymentKindEfectivo,
			plataforma:    r.PlatformName,
			earlier:       earlier, refunded: refunded, refunds: devuelto,
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
	fuentes, err := tipSources(ctx, s.store.QC(ctx), sess.ID)
	if err != nil {
		return nil, err
	}
	pend := pendingView(fuentes)
	if pend.People, err = tipPeople(ctx, s.store.QC(ctx)); err != nil {
		return nil, err
	}
	view.TipsPending = &pend
	entregado, err := s.store.QC(ctx).TipPayoutTotalsForSession(ctx, sess.ID)
	if err != nil {
		return nil, err
	}
	view.TipsPaidOut, view.CardTipsPaidInCash = entregado.PaidOut, entregado.NonCashPaidInCash
	view.CardCountMode = sess.CardCountMode
	sinConcepto, err := s.store.QC(ctx).CashOutsWithoutConceptInSession(ctx, sess.ID)
	if err != nil {
		return nil, err
	}
	view.CashOutsWithoutConcept = int(sinConcepto)
	view.TerminalsToCount = []TerminalToCount{}
	cobrado, err := s.store.QC(ctx).TerminalCollectedForSession(ctx, sess.ID)
	if err != nil {
		return nil, err
	}
	por := map[int64]decimal.Decimal{}
	for _, c := range cobrado {
		por[c.TerminalID] = c.Collected
	}
	for _, id := range domain.TerminalsToDeclare(domain.CardCountMode(sess.CardCountMode), por) {
		for _, c := range cobrado {
			if c.TerminalID == id {
				view.TerminalsToCount = append(view.TerminalsToCount, TerminalToCount{TerminalID: id, Name: c.Name})
			}
		}
	}
	if view.TerminalCounts, err = s.terminalCounts(ctx, sess.ID); err != nil {
		return nil, err
	}
	view.Movements = movementViews(moves)
	view.corteMethods = methods
	return view, nil
}

// closeAttendance aplica la regla de la salida de quien abrió dentro de la transacción del cierre.
// Con el cierre autorizado, la entrada queda «sin salida» y el evento dice quién lo autorizó y por
// qué, con el mismo `q`: si el cierre falla después, nada de esto queda.
func (s *cashDeps) closeAttendance(ctx context.Context, q *db.Queries, sess db.RegisterSession, userID int64, cmd CierreCmd) error {
	opener, err := cashAttendanceOf(ctx, q, sess.OpenedBy)
	if err != nil {
		return err
	}
	decision, err := domain.CashCloseAttendance(domain.CashCloseInput{Opener: opener.Attendance,
		OpenerID: sess.OpenedBy, OpenerName: opener.Name, CloserID: userID, Perms: cmd.Perms,
		AuthorizedReason: cmd.AuthorizedReason}, s.now())
	if err != nil || !decision.Authorized {
		return err
	}
	// Exactamente una fila. Con 0, otro cierre autorizado (de otra caja de la misma persona) la marcó
	// entre la lectura y este update: el `for share` no choca consigo mismo. No queda evento de algo
	// que no pasó, y se pide volver a intentar: al releer, la entrada ya no está abierta y cierra.
	n, err := q.MarkAttendanceMissingOut(ctx, opener.EntryID)
	if err != nil {
		return err
	}
	if n != 1 {
		return domain.ErrAttendanceStateChanged
	}
	branch, err := q.GetRegisterSessionBranch(ctx, sess.ID)
	if err != nil {
		return err
	}
	actor, subject := userID, sess.OpenedBy
	return recordAudit(ctx, q, domain.AuditEvent{Action: "cash.closed_without_opener_clock_out",
		ActorKind: domain.ActorUser, ActorUserID: &actor, SubjectUserID: &subject,
		EntityType: "register_session", EntityID: sess.ID, BranchID: &branch,
		After: map[string]any{"entryId": opener.EntryID, "reason": decision.Reason}})
}

// conteosDelTurno lee el desglose de los dos momentos, para el turno abierto y para el corte
// cerrado.
//
// UNA SOLA DERIVACIÓN para las dos vistas, a propósito: el turno abierto muestra lo que se contó al
// abrir y el detalle del corte muestra los dos, y sacarlo por caminos distintos es de donde salen
// dos pantallas que no coinciden sin forma de saber cuál miente.
func (s *cashDeps) conteosDelTurno(ctx context.Context, sessionID int64) (*ConteosDelTurno, error) {
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

// guardarConteo escribe el conteo y sus renglones DENTRO de la transacción que le pasen.
//
// Los renglones con cero piezas no se escriben: "no hay" y "no se capturó" son lo mismo en un
// arqueo (FR-009), y el `check (pieces > 0)` del esquema está para que eso no dependa de que esta
// función se acuerde.
func (s *cashDeps) guardarConteo(ctx context.Context, q *db.Queries, sessionID int64,
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

// saveTerminalCounts: con arqueo por terminal, cada terminal que cobró en el turno tiene que
// traer el total de su corte; con automático no se pide nada. El modo es el del turno (copiado al
// abrir), no el actual de la sucursal.
func (s *cashDeps) saveTerminalCounts(ctx context.Context, q *db.Queries, sess db.RegisterSession, userID int64, declared map[int64]decimal.Decimal) error {
	cobrado, err := q.TerminalCollectedForSession(ctx, sess.ID)
	if err != nil {
		return err
	}
	por := map[int64]decimal.Decimal{}
	for _, c := range cobrado {
		por[c.TerminalID] = c.Collected
	}
	for _, id := range domain.TerminalsToDeclare(domain.CardCountMode(sess.CardCountMode), por) {
		d, ok := declared[id]
		if !ok {
			return domain.ErrTerminalCountRequired
		}
		d = domain.Round2(d)
		if !domain.ValidMoney(d, true) {
			return domain.ErrValidation
		}
		var name string
		for _, c := range cobrado {
			if c.TerminalID == id {
				name = c.Name
			}
		}
		if err := q.InsertSessionTerminalCount(ctx, db.InsertSessionTerminalCountParams{SessionID: sess.ID, CardTerminalID: id,
			TerminalName: name, Expected: por[id], Declared: d, CreatedBy: userID}); err != nil {
			return err
		}
	}
	return nil
}

func (s *cashDeps) terminalCounts(ctx context.Context, sessionID int64) ([]TerminalCountView, error) {
	q := s.store.QC(ctx)
	rows, err := q.SessionTerminalCounts(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	out := make([]TerminalCountView, 0, len(rows))
	// Sin arqueo guardado (turno abierto, o arqueo automático) no hay nada firmado contra qué avisar.
	if len(rows) == 0 {
		return out, nil
	}
	cobrado, err := q.TerminalCollectedForSession(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	now := map[int64]decimal.Decimal{}
	for _, c := range cobrado {
		now[c.TerminalID] = c.Collected
	}
	seen := map[int64]bool{}
	for _, r := range rows {
		seen[r.CardTerminalID] = true
		en := now[r.CardTerminalID]
		out = append(out, TerminalCountView{TerminalID: r.CardTerminalID, Name: r.TerminalName, Expected: r.Expected,
			Declared: r.Declared, Difference: r.Difference, ExpectedNow: en, DifferenceNow: domain.Round2(r.Declared.Sub(en))})
	}
	// La UNIÓN con lo cobrado hoy: la terminal que recibió un pago corregido pudo no tener renglón al
	// cerrar (el caso real). Sin renglón, al cerrar se esperaba $0 y no se declaró nada.
	for _, c := range cobrado {
		if !seen[c.TerminalID] {
			out = append(out, TerminalCountView{TerminalID: c.TerminalID, Name: c.Name, Expected: decimal.Zero,
				Declared: decimal.Zero, Difference: decimal.Zero, ExpectedNow: c.Collected, DifferenceNow: domain.Round2(c.Collected.Neg())})
		}
	}
	slices.SortFunc(out, func(a, b TerminalCountView) int { return cmp.Compare(a.TerminalID, b.TerminalID) })
	return out, nil
}

// openSessionOrConflict devuelve la sesión abierta de una caja, o ErrConflict si está cerrada
// (un traspaso exige ambas cajas abiertas).
func (s *cashDeps) openSessionOrConflict(ctx context.Context, registerID int64) (db.RegisterSession, error) {
	sess, err := s.store.QC(ctx).GetOpenSessionByRegister(ctx, registerID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return sess, domain.ErrConflict
		}
		return sess, err
	}
	return sess, nil
}

// sinPedidosPendientes falla nombrando los folios que faltan por terminar.
//
// Los folios van en el mensaje a propósito: un "hay pedidos sin terminar" a secas manda al operador
// a recorrer el tablero comparando, justo cuando está cerrando y con prisa.
func (s *cashDeps) sinPedidosPendientes(ctx context.Context, sessionID int64) error {
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
func (s *cashDeps) pedidosSinEntregar(ctx context.Context, sessionID int64) ([]PendingOrder, error) {
	filas, err := s.store.QC(ctx).OpenOrdersInSession(ctx, &sessionID)
	if err != nil {
		return nil, err
	}
	out := make([]PendingOrder, 0, len(filas))
	for _, f := range filas {
		out = append(out, PendingOrder{ID: f.ID, Number: int(f.DailyNumber), Name: derefStr(f.FolioName), Total: f.Total})
	}
	return out, nil
}

// liveAccountsForClosing son las cuentas vivas que el cierre lista sin bloquearse por ellas. Del
// MISMO servicio que la fila del POS con `olderDebts` (un fiado de hace meses también se ve al cerrar),
// filtrado a los grupos que no bloquean: lo que está en cocina ya va en Pending. Solo la caja
// principal: las secundarias no venden.
func (s *cashDeps) liveAccountsForClosing(ctx context.Context, isPrimary bool) ([]AccountItem, error) {
	out := []AccountItem{}
	if !isPrimary {
		return out, nil
	}
	live, err := NewAccountsService(s.store, NewOrdersService(s.store, s.now)).Live(ctx, true)
	if err != nil {
		return nil, err
	}
	for _, it := range live.Items {
		if !domain.ListedAtClosing(it.State, it.PendingCount) {
			continue
		}
		switch it.Group {
		case domain.GroupCapturing, domain.GroupDeliveredOwes, domain.GroupPreviousDays:
			out = append(out, it)
		}
	}
	return out, nil
}

// liveDetail pone en el detalle de un turno abierto lo que Cajas calcula en vivo: totales por
// medio, desglose y arqueo. Lo demás del detalle (ventas, movimientos, devoluciones) ya se lee en
// vivo. El ocultamiento del arqueo ciego lo aplica SessionDetail después, sobre esto mismo.
func (s *cashDeps) liveDetail(ctx context.Context, sess db.GetSessionRow, view *SessionDetailView) error {
	reg, err := s.store.QC(ctx).GetCashRegister(ctx, sess.RegisterID)
	if err != nil {
		return err
	}
	vivo, err := s.sessionWithExpected(ctx, db.RegisterSession{
		ID: sess.ID, BusinessDate: sess.BusinessDate, Status: sess.Status, OpeningCash: sess.OpeningCash,
		OpenedBy: sess.OpenedBy, OpenedAt: sess.OpenedAt, ClosedBy: sess.ClosedBy, ClosedAt: sess.ClosedAt,
		Notes: sess.Notes, Currency: sess.Currency, RegisterID: sess.RegisterID,
	}, reg, false)
	if err != nil {
		return err
	}
	view.Totals, view.Breakdown, view.Drawer = vivo.Totals, vivo.Breakdown, vivo.Drawer
	return nil
}

// sessionSales lee las ventas de un corte junto con su conteo y su ingreso.
//
// La lista y el resumen salen de dos consultas con el MISMO where a propósito: si divergen, una de
// las dos miente y quien lee la pantalla no tiene forma de saber cuál.
//
// El tope es el mismo `MaxListLimit` del resto de las listas. Se devuelve la cuenta REAL aparte para
// que la pantalla pueda decir cuántas hay: recortar en silencio se lee como "esto es todo".
func (s *cashDeps) sessionSales(ctx context.Context, id int64) ([]SessionSaleView, int, decimal.Decimal, error) {
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

// cobradoPorCajero: cuánto cobró cada persona en el turno.
//
// Es la respuesta a "dos estaciones, un solo cajón". Crear una caja por Surface daría dos arqueos
// contando el mismo dinero físico —dos cifras inventadas—, así que la caja sigue siendo una y lo
// que se separa es quién cobró. El dato existía desde el principio en received_by y solo lo usaba
// el reparto de propinas.
func (s *cashDeps) cobradoPorCajero(ctx context.Context, sessionID int64) ([]CashierTotal, error) {
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

// voidedPayments lista los pagos devueltos de un turno. Slice no-nil → [] en JSON.
func (s *cashDeps) voidedPayments(ctx context.Context, sessionID int64) ([]VoidedPaymentView, error) {
	rows, err := s.store.QC(ctx).ListSessionPaymentVoids(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	out := make([]VoidedPaymentView, 0, len(rows))
	for _, v := range rows {
		folio := fmt.Sprintf("#%d", v.DailyNumber)
		if v.FolioName != "" {
			folio = fmt.Sprintf("%s #%d", v.FolioName, v.DailyNumber)
		}
		out = append(out, VoidedPaymentView{
			Method: v.MethodName, Amount: v.Amount, Tip: v.TipAmount, OrderFolio: folio,
			VoidedBy: v.VoidedBy, VoidedAt: v.VoidedAt, Reason: v.Reason,
		})
	}
	return out, nil
}

// sessionRefunds lista las devoluciones de un turno. Slice no-nil → [] en JSON.
func (s *cashDeps) sessionRefunds(ctx context.Context, sessionID int64) ([]RefundView, error) {
	rows, err := s.store.QC(ctx).ListSessionRefunds(ctx, &sessionID)
	if err != nil {
		return nil, err
	}
	out := make([]RefundView, 0, len(rows))
	for _, r := range rows {
		folio := fmt.Sprintf("#%d", r.DailyNumber)
		if r.FolioName != "" {
			folio = fmt.Sprintf("%s #%d", r.FolioName, r.DailyNumber)
		}
		out = append(out, RefundView{
			Method: r.MethodName, Amount: r.Amount, Tip: r.TipAmount, OrderFolio: folio, FromDrawer: r.FromDrawer,
			RefundedBy: r.RefundedBy, RefundedAt: r.CreatedAt, Reason: r.Reason,
		})
	}
	return out, nil
}

// sessionExpenses lista los gastos atribuidos a un corte para la sección "Gastos" del resumen.
// Slice no-nil → en JSON va como [] (el front no revienta con .map).
func (s *cashDeps) sessionExpenses(ctx context.Context, sessionID int64) ([]CashExpenseView, error) {
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
			DrawerExtra: e.DrawerExtra,
		})
	}
	return out, nil
}

// arqueoCiego dice si este negocio cuenta a ciegas.
//
// Un fallo al leer los ajustes NO enciende el control: devolver "ciego" ante un error escondería
// las cifras por un hiccup de la base, y el operador no tendría cómo saber por qué. El default es
// el comportamiento de la spec 003, que es el que está probado.
//
// Pero ese fail-open se REGISTRA, por lo mismo que el del limitador cuando Redis se cae: un
// control antifraude que se apaga solo y en silencio no se distingue de uno que nunca estuvo
// encendido, y quien audita el corte no tiene cómo saber cuál de los dos vio.
func (s *cashDeps) arqueoCiego(ctx context.Context) bool {
	ajustes, err := s.store.QC(ctx).GetBusinessSettings(ctx)
	if err != nil {
		logging.SecurityEvent(ctx, "blind_count_unavailable", "error", err.Error())
		return false
	}
	return ajustes.BlindCashCount
}

// owingOrders lista los pedidos entregados que deben. Es la fuente ÚNICA de esa lista: la usa la
// vista del cierre y la usa la guardia que impide cerrar, igual que pedidosSinEntregar.
func (s *cashDeps) owingOrders(ctx context.Context, isPrimary bool) ([]domain.OwingOrder, error) {
	out := []domain.OwingOrder{}
	if !isPrimary {
		return out, nil
	}
	filas, err := s.store.QC(ctx).OwingDeliveredOrders(ctx)
	if err != nil {
		return nil, err
	}
	for _, f := range filas {
		o := domain.OwingOrder{ID: f.ID, Number: int(f.DailyNumber), Name: derefStr(f.FolioName), Total: f.Total, Paid: f.Paid, WrittenOff: f.WrittenOffAmount}
		if o.Outstanding().IsPositive() {
			out = append(out, o)
		}
	}
	return out, nil
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
func (s *cashDeps) conteoDelCajon(ctx context.Context, hayCajon bool, cmd CierreCmd) (*conteoResuelto, error) {
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

// piezasConSuValor cambia ids por valores leyendo el catálogo, y de paso hace cumplir FR-011.
//
// La moneda importa: contar dólares en un turno en pesos daría un total sin significado, y el
// arqueo del turno entero se compararía después contra esa cifra. Un id que no existe también se
// rechaza — es una pantalla desincronizada del catálogo, no un cero.
func (s *cashDeps) piezasConSuValor(ctx context.Context, capturadas []PiezaCapturada, moneda string) ([]domain.PiezaContada, error) {
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

// activeRegister carga una caja y exige que exista y esté activa (una caja inactiva no opera).
func (s *cashDeps) activeRegister(ctx context.Context, registerID int64) (db.GetCashRegisterRow, error) {
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
func (s *cashDeps) businessDate(ctx context.Context) time.Time {
	tz, err := s.store.QC(ctx).GetBusinessTimezone(ctx)
	if err != nil {
		tz = domain.DefaultTimezone
	}
	return domain.BusinessDate(s.now(), domain.LoadBusinessLocation(tz))
}
