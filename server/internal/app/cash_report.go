package app

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store/db"
	"github.com/shopspring/decimal"
)

// CashReportService lee los cortes: el Histórico y el detalle (spec 082, fase 2). No escribe.
type CashReportService struct{ cashDeps }

// SessionHistory devuelve una página del histórico de cortes (abiertos y cerrados), el más reciente
// primero, y cuántos hay con el mismo filtro.
func (s *CashReportService) SessionHistory(ctx context.Context, f domain.SessionHistoryFilter) ([]SessionHistoryRow, int64, error) {
	return s.SessionHistoryFor(ctx, f, true)
}

// SessionHistoryFor es SessionHistory para quien la pide: sin el permiso de configurar caja y con
// arqueo ciego, la diferencia del último cierre de cada caja no reabierta no viaja (ver R8).
func (s *CashReportService) SessionHistoryFor(ctx context.Context, f domain.SessionHistoryFilter, canSeeArqueo bool) ([]SessionHistoryRow, int64, error) {
	if err := f.Validate(); err != nil {
		return nil, 0, err
	}
	day := func(t *time.Time) pgtype.Date {
		if t == nil {
			return pgtype.Date{}
		}
		return pgtype.Date{Time: *t, Valid: true}
	}
	q := s.store.QC(ctx)
	desde, hasta := day(f.From), day(f.To)
	total, err := q.CountSessions(ctx, db.CountSessionsParams{Desde: desde, Hasta: hasta})
	if err != nil {
		return nil, 0, err
	}
	rows, err := q.ListSessions(ctx, db.ListSessionsParams{Desde: desde, Hasta: hasta, Lim: f.Limit, Off: f.Offset})
	if err != nil {
		return nil, 0, err
	}
	ciego := !canSeeArqueo && s.arqueoCiego(ctx)
	correcciones, err := adjustmentsBySession(ctx, q, rows)
	if err != nil {
		return nil, 0, err
	}
	tolerancia, err := q.GetCashTolerance(ctx)
	if err != nil {
		return nil, 0, err
	}
	out := make([]SessionHistoryRow, len(rows))
	for i, r := range rows {
		out[i] = SessionHistoryRow{
			ID: r.ID, RegisterName: r.RegisterName, Status: string(r.Status), OpeningCash: r.OpeningCash,
			Currency: domain.Currency(r.Currency), OpenedAt: r.OpenedAt, ClosedAt: tsPtr(r.ClosedAt),
			OpenedByName: r.OpenedByName, ClosedByName: r.ClosedByName, Notes: r.Notes,
			OpeningReason: r.OpeningReason, OpeningReasonNote: r.OpeningReasonNote,
		}
		if r.Status != db.SessionStatusCerrada {
			// Compatibilidad: antes de #290 un turno abierto traía aquí la suma parcial y nunca null.
			// La PWA anterior y el MCP lo formatean como número durante la subida.
			parcial := domain.Round2(r.OffDrawerDifference.Add(r.OnDrawerDifference))
			out[i].TotalDifference = &parcial
			continue
		}
		if ciego && r.IsLatestOfRegister {
			out[i].ArqueoHidden = true
			continue
		}
		diff, corregido := historyRowDifference(r, correcciones[r.ID])
		out[i].Difference, out[i].TotalDifference, out[i].Adjusted = &diff, &diff.Total, corregido
		out[i].ExplainedByNext = explanationOf(&diff, r.NextID, r.NextOpeningReason, r.NextOpeningReasonNote)
		out[i].NeedsExplanation = needsExplanation(canSeeArqueo, &diff, tolerancia, correcciones[r.ID], out[i].ExplainedByNext)
	}
	return out, total, nil
}

// historyRowDifference es la diferencia de un renglón del Histórico con domain.CutDifference, ya con
// el conteo corregido si hay una corrección vigente. La lista trae las diferencias sumadas por lado
// del cajón: la regla no necesita los renglones, solo saber de qué lado cae cada peso.
func historyRowDifference(r db.ListSessionsRow, adjs []domain.Adjustment) (domain.CutDiff, bool) {
	methods := []domain.MethodDiff{
		{Declared: r.OffDrawerDifference},
		{Declared: r.OnDrawerDifference, TouchesDrawer: true},
	}
	var drawer *domain.DrawerCount
	if r.DrawerCounted != nil {
		drawer = &domain.DrawerCount{Expected: r.DrawerExpected, Counted: *r.DrawerCounted}
	}
	eff := effectiveOf(adjs)
	if drawer != nil && eff != nil && eff.DrawerCounted != nil {
		drawer.Counted = *eff.DrawerCounted
	}
	return domain.CutDifference(methods, drawer), eff != nil
}

// adjustmentsBySession trae las correcciones de los cortes cerrados de una página, en una consulta.
func adjustmentsBySession(ctx context.Context, q *db.Queries, rows []db.ListSessionsRow) (map[int64][]domain.Adjustment, error) {
	ids := make([]int64, 0, len(rows))
	for _, r := range rows {
		if r.Status == db.SessionStatusCerrada {
			ids = append(ids, r.ID)
		}
	}
	out := map[int64][]domain.Adjustment{}
	if len(ids) == 0 {
		return out, nil
	}
	adjs, err := q.ListAdjustmentsForSessions(ctx, ids)
	if err != nil {
		return nil, err
	}
	for _, a := range adjs {
		out[a.SessionID] = append(out[a.SessionID], domain.Adjustment{ID: a.ID, Kind: a.Kind, After: a.AfterAmount, ReversesID: a.ReversesID})
	}
	return out, nil
}

// SessionPeriodDifference suma los sobrantes y faltantes de TODOS los cortes cerrados del filtro, ya
// corregidos (#290, P4): no es venta ni gasto, y no depende de la página que se esté viendo. Con
// arqueo ciego, quien no configura la caja no la recibe: incluiría el último cierre de cada caja.
//
// ponytail: lee todos los cortes del filtro en una consulta; son pocos al día y el Histórico casi
// siempre se filtra por fechas. Si un filtro abierto se vuelve lento, el siguiente paso es sumar en
// SQL con las mismas piezas.
func (s *CashReportService) SessionPeriodDifference(ctx context.Context, f domain.SessionHistoryFilter, canSeeArqueo bool) (*decimal.Decimal, error) {
	if err := f.Validate(); err != nil {
		return nil, err
	}
	if !canSeeArqueo && s.arqueoCiego(ctx) {
		return nil, nil
	}
	day := func(t *time.Time) pgtype.Date {
		if t == nil {
			return pgtype.Date{}
		}
		return pgtype.Date{Time: *t, Valid: true}
	}
	q := s.store.QC(ctx)
	desde, hasta := day(f.From), day(f.To)
	total, err := q.CountSessions(ctx, db.CountSessionsParams{Desde: desde, Hasta: hasta})
	if err != nil {
		return nil, err
	}
	rows, err := q.ListSessions(ctx, db.ListSessionsParams{Desde: desde, Hasta: hasta, Lim: int32(total), Off: 0})
	if err != nil {
		return nil, err
	}
	correcciones, err := adjustmentsBySession(ctx, q, rows)
	if err != nil {
		return nil, err
	}
	suma := decimal.Zero
	for _, r := range rows {
		if r.Status != db.SessionStatusCerrada {
			continue
		}
		diff, _ := historyRowDifference(r, correcciones[r.ID])
		suma = suma.Add(diff.Total)
	}
	suma = domain.Round2(suma)
	return &suma, nil
}

// SessionDetail devuelve una sesión con sus totales GUARDADOS (snapshot al cerrar) y movimientos,
// completa: es la vista de quien configura la caja.
func (s *CashReportService) SessionDetail(ctx context.Context, id int64) (*SessionDetailView, error) {
	return s.SessionDetailFor(ctx, id, true)
}

// SessionDetailFor es SessionDetail para quien la pide. Con arqueo ciego, quien no configura la
// caja no ve en un turno cerrado el fondo que se dejó ni el retiro (spec 035, R8): el fondo dejado
// —o lo contado menos el retiro— es la cifra contra la que se compara la siguiente apertura.
func (s *CashReportService) SessionDetailFor(ctx context.Context, id int64, canSeeFloat bool) (*SessionDetailView, error) {
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
	if view.WrittenOff, err = s.store.QC(ctx).SessionWrittenOff(ctx, &sess.ID); err != nil {
		return nil, err
	}
	// Lo que no es venta del turno se lee en vivo y no del snapshot: los cobros y devoluciones de un
	// turno cerrado ya no cambian (cobrar y devolver exigen el turno abierto), y así el corte cerrado
	// los nombra igual que el abierto. El esperado sí sale del snapshot, que es lo que se firmó.
	vivos, err := s.store.QC(ctx).ExpectedByMethodForSession(ctx, id)
	if err != nil {
		return nil, err
	}
	porMetodo := make(map[int16]db.ExpectedByMethodForSessionRow, len(vivos))
	for _, v := range vivos {
		porMetodo[v.PaymentMethodID] = v
	}
	methods := make([]methodExpected, 0, len(totals))
	// Desde el SNAPSHOT de cada renglón, no del catálogo: así es como el corte cerrado conserva la
	// forma con la que se firmó, aunque el interruptor del método cambie después.
	delCorte := make([]domain.MetodoDelCorte, 0, len(totals))
	for _, t := range totals {
		v := porMetodo[t.PaymentMethodID]
		devuelto := refundsOf(v.Refunded, v.RefundedTips, v.DrawerRefunded, v.DrawerRefundedTips)
		view.Totals = append(view.Totals, MethodTotal{
			MethodID: int(t.PaymentMethodID), Name: t.Name, Kind: string(t.Kind),
			Expected: &t.Expected, Declared: t.Declared, Difference: t.Difference,
			Tips: domain.Round2(t.Tips.Sub(devuelto.Tips())), grossTips: t.Tips,
		})
		delCorte = append(delCorte, domain.MetodoDelCorte{
			ID: int(t.PaymentMethodID), Esperado: t.Expected, TocaElCajon: t.AffectsCashDrawer,
		})
		methods = append(methods, methodExpected{
			name: t.Name, expected: t.Expected, tips: t.Tips,
			duenoDelFondo: t.Kind == db.PaymentKindEfectivo,
			plataforma:    t.PlatformName,
			earlier:       v.Earlier, refunded: v.Refunded, refunds: devuelto,
		})
	}
	conteos, err := s.conteosDelTurno(ctx, sess.ID)
	if err != nil {
		return nil, err
	}
	view.Counts = conteos
	view.Drawer = arqueoGuardado(delCorte, conteos)
	if view.VoidedPayments, err = s.voidedPayments(ctx, sess.ID); err != nil {
		return nil, err
	}
	if view.Refunds, err = s.sessionRefunds(ctx, sess.ID); err != nil {
		return nil, err
	}
	view.Breakdown = corteBreakdown(sess.OpeningCash, methods, moves)
	// Las mismas fuentes que el cierre y que el turno abierto: el conteo por terminal guardado, las
	// entregas del turno y lo que el cierre dejó en caja.
	view.CardCountMode, view.TipsCarriedOver = sess.CardCountMode, sess.TipsCarriedOver
	if view.TerminalCounts, err = s.terminalCounts(ctx, sess.ID); err != nil {
		return nil, err
	}
	entregado, err := s.store.QC(ctx).TipPayoutTotalsForSession(ctx, sess.ID)
	if err != nil {
		return nil, err
	}
	view.TipsPaidOut, view.CardTipsPaidInCash = entregado.PaidOut, entregado.NonCashPaidInCash
	// Un turno ABIERTO no tiene totales guardados: se calculan en vivo con la misma función que
	// Cajas (spec 029). Leía el snapshot vacío y el Histórico decía «Sin ingresos» mientras Cajas
	// decía $630.
	if sess.Status == db.SessionStatusAbierta {
		if err := s.liveDetail(ctx, sess, view); err != nil {
			return nil, err
		}
	}
	view.Movements = movementViews(moves)
	view.FloatLeft, view.FixedFloatAtClose, view.Withdrawal = sess.FloatLeft, sess.FixedFloatAtClose, withdrawalOf(moves)
	if !canSeeFloat && string(sess.Status) == "cerrada" && s.arqueoCiego(ctx) {
		ocultarElFondoDejado(view)
		ultimo, err := s.store.QC(ctx).IsLatestSessionOfRegister(ctx, db.IsLatestSessionOfRegisterParams{
			RegisterID: sess.RegisterID, SessionID: sess.ID})
		if err != nil {
			return nil, err
		}
		if ultimo {
			ocultarElArqueoDelUltimoCierre(view)
		}
	}
	// El mismo ocultamiento que en el turno abierto, y aquí no es redundante: este endpoint
	// acepta el id del turno ABIERTO y está abierto a rol cajero, así que nulificar solo el otro
	// camino dejaría la cifra a un request de distancia.
	if string(sess.Status) == "abierta" && s.arqueoCiego(ctx) {
		ocultarLoEsperado(view.Totals, view.Drawer, &view.Breakdown, nil)
	}
	view.OpeningReason, view.OpeningReasonNote = sess.OpeningReason, sess.OpeningReasonNote
	vecinos, err := s.store.QC(ctx).GetSessionNeighbors(ctx, sess.ID)
	if err != nil {
		return nil, err
	}
	if vecinos.PreviousID != 0 {
		view.PreviousSessionID = &vecinos.PreviousID
	}
	if sess.Status == db.SessionStatusCerrada && !view.ArqueoHidden {
		adjRows, err := s.store.QC(ctx).ListSessionAdjustments(ctx, sess.ID)
		if err != nil {
			return nil, err
		}
		view.Adjustments = adjustmentViews(adjRows)
		view.Effective = effectiveOf(adjustmentsOf(adjRows))
		view.Difference = cutDifferenceOf(view.Totals, withCorrectedCount(view.Drawer, view.Effective))
		view.ExplainedByNext = explanationOf(view.Difference, vecinos.NextID, vecinos.NextOpeningReason, vecinos.NextOpeningReasonNote)
		tolerancia, err := s.store.QC(ctx).GetCashTolerance(ctx)
		if err != nil {
			return nil, err
		}
		view.NeedsExplanation = needsExplanation(canSeeFloat, view.Difference, tolerancia, adjustmentsOf(adjRows), view.ExplainedByNext)
	}
	// Siempre arreglo: un turno abierto o un arqueo oculto no tienen correcciones a la vista, y un
	// `null` donde la pantalla espera una lista la tumba sin un solo error en el servidor.
	if view.Adjustments == nil {
		view.Adjustments = []SessionAdjustmentView{}
	}
	return view, nil
}

// SessionSalesPage devuelve una página de las ventas de un corte.
//
// Existe porque el detalle trae hasta `maxVentasDeCorte` y un corte más grande no se podía recorrer
// completo desde ninguna parte: la pantalla decía cuántas había —no mentía— pero el resto era
// inalcanzable, y un arqueo que no se puede auditar entero no se puede auditar.
//
// Endpoint aparte y no un parámetro del detalle: pedir la página 3 no debería recalcular el arqueo,
// los gastos y lo cobrado por persona.
func (s *CashReportService) SessionSalesPage(ctx context.Context, id int64, limit, offset int32) ([]SessionSaleView, int, decimal.Decimal, error) {
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
