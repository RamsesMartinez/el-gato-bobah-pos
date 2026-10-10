//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
)

func principalRegister(t *testing.T, st *store.Store) int64 {
	t.Helper()
	var id int64
	if err := st.Pool.QueryRow(context.Background(),
		`select id from cash_registers where is_primary and is_active limit 1`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

// LA PROPINA NO ES VENTA NI GASTO, Y SALE DEL CAJÓN UNA SOLA VEZ.
//
// Hoy la propina entregada se captura como gasto: sube los gastos, baja el resultado, y el corte la
// cuenta en dos lados. Este test falla nombrando el concepto que se duplicó.
func TestTipIsNeitherSaleNorExpenseAndLeavesDrawerOnce(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()
	orders := app.NewOrdersService(st, clock)
	back := app.NewBackofficeService(st, clock)
	tips := app.NewTipsService(st, clock)

	cajero := makeUser(t, st, "cajero_propinas", "cajero")
	ana := makeUser(t, st, "ana_propinas", "cajero")
	beto := makeUser(t, st, "beto_propinas", "cajero")
	carla := makeUser(t, st, "carla_propinas", "cajero")
	abrirCajaPrincipal(t, st, cajero)
	reg := principalRegister(t, st)

	cobrarConPropina(t, ctx, st, orders, "Propina efectivo", "100", "30", cajero, paymentMethodID(t, st, "Efectivo"))
	cobrarConPropina(t, ctx, st, orders, "Propina tarjeta", "200", "50.50", cajero, paymentMethodID(t, st, "Tarjeta débito"))

	antes, err := back.CurrentByRegister(ctx, reg)
	if err != nil {
		t.Fatal(err)
	}
	pend, err := tips.Pending(ctx, reg)
	if err != nil {
		t.Fatal(err)
	}
	if !pend.Total.Equal(dec("80.5")) {
		t.Fatalf("pendiente = %s, quería 80.50", pend.Total)
	}
	// La hoja de reparto ofrece a las personas activas del negocio; quien cobra no es admin y no
	// puede leer la lista de usuarios, así que viaja aquí, solo con id y nombre.
	var vista bool
	for _, p := range pend.People {
		vista = vista || p.ID == ana
	}
	if !vista {
		t.Fatalf("la lista de personas no trae a quien recibe: %+v", pend.People)
	}

	// Parejo entre tres: 26 pesos enteros a cada uno; sobran 2.50 que siguen pendientes.
	out, err := tips.Payout(ctx, reg, app.TipPayoutCmd{
		Mode: "parejo", ActorID: cajero,
		Recipients: []app.TipRecipient{{UserID: ana}, {UserID: beto}, {UserID: carla}},
	})
	if err != nil {
		t.Fatalf("repartir: %v", err)
	}
	if len(out) != 3 || !out[0].Amount.Equal(dec("26")) {
		t.Fatalf("reparto = %+v, quería 3 de 26", out)
	}
	pend, _ = tips.Pending(ctx, reg)
	if !pend.Total.Equal(dec("2.5")) {
		t.Fatalf("el sobrante de centavos y pesos debe seguir pendiente: %s", pend.Total)
	}

	despues, err := back.CurrentByRegister(ctx, reg)
	if err != nil {
		t.Fatal(err)
	}
	if len(despues.Expenses) != len(antes.Expenses) {
		t.Fatal("PROPINA DUPLICADA COMO GASTO: la entrega creó un gasto")
	}
	ventas := func(v *app.SessionView) decimal.Decimal {
		t := decimal.Zero
		for _, m := range v.Breakdown.Ingresos {
			for _, it := range m.Items {
				if it.Concept == "Ventas" {
					t = t.Add(it.Amount)
				}
			}
		}
		return t
	}
	if !ventas(despues).Equal(ventas(antes)) {
		t.Fatalf("PROPINA DUPLICADA COMO VENTA: ventas %s → %s", ventas(antes), ventas(despues))
	}
	for _, b := range despues.Breakdown.Egresos {
		if b.Concept != "Propinas entregadas" {
			t.Fatalf("PROPINA DUPLICADA COMO %q en egresos: %+v", b.Concept, despues.Breakdown.Egresos)
		}
	}
	// El efectivo esperado baja exactamente lo entregado (78), incluida la propina de tarjeta
	// pagada con billetes del cajón.
	var efAntes, efDespues decimal.Decimal
	for _, m := range antes.Totals {
		if m.Kind == "efectivo" {
			efAntes = *m.Expected
		}
	}
	for _, m := range despues.Totals {
		if m.Kind == "efectivo" {
			efDespues = *m.Expected
		}
	}
	if !efAntes.Sub(efDespues).Equal(dec("78")) {
		t.Fatalf("PROPINA DESCONTADA DEL CAJÓN %s veces: el esperado bajó %s y se entregaron 78",
			efAntes.Sub(efDespues).Div(dec("78")), efAntes.Sub(efDespues))
	}
	if !despues.TipsPaidOut.Equal(dec("78")) || !despues.CardTipsPaidInCash.Equal(dec("48")) {
		t.Fatalf("corte: entregado %s (quería 78), tarjeta pagada en efectivo %s (quería 48)",
			despues.TipsPaidOut, despues.CardTipsPaidInCash)
	}
}

// Centavos, personas repetidas y excesos se rechazan en el servidor (EB-43, EB-44, EB-46, EB-06).
func TestTipPayoutRejectsCentsDuplicatesExcessAndForeignRecipient(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()
	orders := app.NewOrdersService(st, clock)
	tips := app.NewTipsService(st, clock)
	cajero := makeUser(t, st, "cajero_prop_rech", "cajero")
	abrirCajaPrincipal(t, st, cajero)
	reg := principalRegister(t, st)
	cobrarConPropina(t, ctx, st, orders, "Propina rech", "100", "10", cajero, paymentMethodID(t, st, "Efectivo"))

	otra := makeCompany(t, st, "otra-propinas")
	ajeno := makeUserIn(t, st, otra, "ajeno_prop", "cajero")
	inactivo := makeUser(t, st, "inactivo_prop", "cajero")
	if _, err := st.Pool.Exec(ctx, `update users set is_active = false where id = $1`, inactivo); err != nil {
		t.Fatal(err)
	}

	for name, c := range map[string]struct {
		cmd  app.TipPayoutCmd
		want error
	}{
		"centavos":   {app.TipPayoutCmd{Mode: "ajustado", Recipients: []app.TipRecipient{{UserID: cajero, Amount: new(dec("5.50"))}}}, domain.ErrWholePesosOnly},
		"repetida":   {app.TipPayoutCmd{Mode: "parejo", Recipients: []app.TipRecipient{{UserID: cajero}, {UserID: cajero}}}, domain.ErrDuplicateRecipient},
		"exceso":     {app.TipPayoutCmd{Mode: "ajustado", Recipients: []app.TipRecipient{{UserID: cajero, Amount: new(dec("11"))}}}, domain.ErrTipExceedsPending},
		"ajena":      {app.TipPayoutCmd{Mode: "parejo", Recipients: []app.TipRecipient{{UserID: ajeno}}}, domain.ErrValidation},
		"inactiva":   {app.TipPayoutCmd{Mode: "parejo", Recipients: []app.TipRecipient{{UserID: inactivo}}}, domain.ErrValidation},
		"no alcanza": {app.TipPayoutCmd{Mode: "parejo", Recipients: []app.TipRecipient{{UserID: cajero}, {UserID: ajeno + 999999}}}, domain.ErrValidation},
		"modo":       {app.TipPayoutCmd{Mode: "otro", Recipients: []app.TipRecipient{{UserID: cajero}}}, domain.ErrValidation},
	} {
		c.cmd.ActorID = cajero
		if _, err := tips.Payout(ctx, reg, c.cmd); !errors.Is(err, c.want) {
			t.Errorf("%s: err = %v, quería %v", name, err, c.want)
		}
	}
	var n int
	_ = st.Pool.QueryRow(ctx, `select count(*) from register_cash_movements where kind = 'propina'`).Scan(&n)
	if n != 0 {
		t.Fatalf("un reparto rechazado dejó %d movimientos", n)
	}
}

// UN CIERRE QUE GANA LA CARRERA NO DEJA ENTREGAR LO QUE YA HEREDÓ EL SIGUIENTE TURNO.
//
// La entrega leía el turno abierto antes del candado. Si el cierre confirmaba en medio, la entrega
// seguía sobre el turno cerrado y el mismo dinero se entregaba y además se heredaba.
func TestTipPayoutLosingRaceToCloseIsRejected(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()
	orders := app.NewOrdersService(st, clock)
	tips := app.NewTipsService(st, clock)
	cajero := makeUser(t, st, "cajero_prop_carrera", "cajero")
	sess := abrirCajaPrincipal(t, st, cajero)
	reg := principalRegister(t, st)
	cobrarConPropina(t, ctx, st, orders, "Propina carrera", "100", "20", cajero, paymentMethodID(t, st, "Efectivo"))

	// Otra tableta tiene el turno bloqueado mientras lo cierra.
	tx, err := st.Pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `select id from register_sessions where id = $1 for update`, sess); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		_, err := tips.Payout(ctx, reg, app.TipPayoutCmd{Mode: "parejo", ActorID: cajero,
			Recipients: []app.TipRecipient{{UserID: cajero}}})
		done <- err
	}()
	time.Sleep(300 * time.Millisecond) // la entrega ya leyó el turno abierto y espera el candado
	if _, err := tx.Exec(ctx, `update register_sessions set status = 'cerrada', tips_carried_over = 20 where id = $1`, sess); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err := <-done; !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("entregar sobre un turno que se cerró en medio: err = %v, quería ErrConflict", err)
	}
}

// Un reparto con cientos de personas tendría el turno bloqueado (y la caja sin cobrar) mientras
// dura: se acota en la frontera.
func TestTipPayoutCapsRecipients(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	tips := app.NewTipsService(st, clock)
	cajero := makeUser(t, st, "cajero_prop_tope", "cajero")
	abrirCajaPrincipal(t, st, cajero)
	muchas := make([]app.TipRecipient, 51)
	for i := range muchas {
		muchas[i] = app.TipRecipient{UserID: int64(i + 1)}
	}
	if _, err := tips.Payout(context.Background(), principalRegister(t, st),
		app.TipPayoutCmd{Mode: "parejo", ActorID: cajero, Recipients: muchas}); !errors.Is(err, domain.ErrTooManyRecipients) {
		t.Fatalf("51 personas: err = %v, quería ErrTooManyRecipients", err)
	}
}

// Lo devuelto deja de ser propina, aunque se devuelva después de entregar: el pendiente nunca queda
// negativo (EB-07, EB-08).
func TestRefundedTipLeavesPendingAndNeverGoesNegative(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()
	tips := app.NewTipsService(st, clock)
	cajero := makeUser(t, st, "cajero_prop_dev", "cajero")
	abrirCajaPrincipal(t, st, cajero)
	reg := principalRegister(t, st)
	orders := app.NewOrdersService(st, clock)
	cobrarConPropina(t, ctx, st, orders, "Propina dev", "100", "20", cajero, paymentMethodID(t, st, "Efectivo"))

	if _, err := tips.Payout(ctx, reg, app.TipPayoutCmd{Mode: "ajustado", ActorID: cajero,
		Recipients: []app.TipRecipient{{UserID: cajero, Amount: new(dec("15"))}}}); err != nil {
		t.Fatal(err)
	}
	// Se devuelve toda la propina del pedido (escrito directo: la regla bajo prueba es el pendiente).
	if _, err := st.Pool.Exec(ctx, `insert into order_refunds (order_id, payment_method_id, amount, tip_amount, reason, refunded_by)
		select op.order_id, op.payment_method_id, 0, 20, 'prueba', $1 from order_payments op where op.tip_amount > 0`, cajero); err != nil {
		t.Fatal(err)
	}
	pend, err := tips.Pending(ctx, reg)
	if err != nil {
		t.Fatal(err)
	}
	if !pend.Total.IsZero() {
		t.Fatalf("pendiente = %s tras devolver la propina; quería 0, nunca negativo", pend.Total)
	}
}

// AL CERRAR: con un peso o más pendiente se exige decidir; «se queda en caja» lo hereda el siguiente
// turno de LA MISMA caja y no otra (EB-11, EB-12, EB-14, EB-45).
func TestClosingAsksAboutTipsAndNextShiftOfSameRegisterInherits(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()
	orders := app.NewOrdersService(st, clock)
	back := app.NewBackofficeService(st, clock)
	tips := app.NewTipsService(st, clock)
	cajero := makeUser(t, st, "cajero_prop_cierre", "cajero")
	abrirCajaPrincipal(t, st, cajero)
	reg := principalRegister(t, st)
	cobrarConPropina(t, ctx, st, orders, "Propina cierre", "100", "12.40", cajero, paymentMethodID(t, st, "Efectivo"))
	entregarPendientes(t, st)

	total := dec("112.40")
	if _, err := back.CloseSession(ctx, reg, cajero, app.CierreCmd{Total: &total, Motivo: "prueba"}); !errors.Is(err, domain.ErrTipsDecisionNeeded) {
		t.Fatalf("cerrar con propina pendiente sin decidir: err = %v", err)
	}
	if _, err := back.CloseSession(ctx, reg, cajero, app.CierreCmd{Total: &total, Motivo: "prueba", Propinas: "quedan_en_caja"}); err != nil {
		t.Fatalf("cerrar dejando la propina en caja: %v", err)
	}
	abrirCajaPrincipal(t, st, cajero)
	pend, err := tips.Pending(ctx, reg)
	if err != nil {
		t.Fatal(err)
	}
	if !pend.Total.Equal(dec("12.4")) || !pend.Inherited.Equal(dec("12.4")) {
		t.Fatalf("el turno siguiente heredó %s (heredado %s); quería 12.40", pend.Total, pend.Inherited)
	}
	// Se entrega lo heredado: 12 pesos; los 0.40 siguen y el cierre ya no pregunta.
	if _, err := tips.Payout(ctx, reg, app.TipPayoutCmd{Mode: "parejo", ActorID: cajero,
		Recipients: []app.TipRecipient{{UserID: cajero}}}); err != nil {
		t.Fatal(err)
	}
	cero := dec("0")
	cerrado, err := back.CloseSession(ctx, reg, cajero, app.CierreCmd{Total: &cero, Motivo: "prueba"})
	if err != nil {
		t.Fatalf("un sobrante de centavos no debe exigir decisión: %v", err)
	}
	var carried decimal.Decimal
	_ = st.Pool.QueryRow(ctx, `select tips_carried_over from register_sessions where id = $1`, cerrado.ID).Scan(&carried)
	if !carried.Equal(dec("0.4")) {
		t.Fatalf("los centavos se perdieron al cerrar: heredado %s, quería 0.40", carried)
	}
}

// AISLAMIENTO: el pendiente de propinas de una empresa no se ve desde otra, ni con conexión reciclada,
// ni sin empresa.
func TestTipPendingIsIsolatedInTheThreeCases(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()
	orders := app.NewOrdersService(st, clock)
	cajero := makeUser(t, st, "cajero_prop_rls", "cajero")
	abrirCajaPrincipal(t, st, cajero)
	reg := principalRegister(t, st)
	cobrarConPropina(t, ctx, st, orders, "Propina rls", "100", "10", cajero, paymentMethodID(t, st, "Efectivo"))
	otra := makeCompany(t, st, "otra-prop-rls")

	// La entrega se escribe como gatobobah_app: sin el grant de la tabla nueva, el primer reparto en
	// producción respondería 42501.
	app0 := appRoleStore(t)
	tctx, release, err := app0.AcquireTenant(ctx, defaultCompanyID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.NewTipsService(app0, clock).Payout(tctx, reg, app.TipPayoutCmd{Mode: "parejo", ActorID: cajero,
		Recipients: []app.TipRecipient{{UserID: cajero}}}); err != nil {
		t.Fatalf("repartir como gatobobah_app: %v", err)
	}
	release()
	if _, err := st.Pool.Exec(ctx, `update register_sessions set status = 'cerrada', tips_carried_over = 0 where status = 'abierta'`); err != nil {
		t.Fatal(err)
	}
	abrirCajaPrincipal(t, st, cajero)
	cobrarConPropina(t, ctx, st, orders, "rls2", "100", "10", cajero, paymentMethodID(t, st, "Efectivo"))

	inTheThreeCases(t, defaultCompanyID, otra, func(t *testing.T, as *store.Store, ctx context.Context) {
		p, err := app.NewTipsService(as, clock).Pending(ctx, reg)
		if err == nil && p.Total.IsPositive() {
			t.Fatalf("se vio propina de otra empresa: %s", p.Total)
		}
	})
}

// AVISOS (punto 7): la propina sin entregar y las salidas sin concepto del turno abierto se dicen
// en Ventas del día, no solo al cerrar.
func TestCashAlertsReportPendingTipsAndCashOutsWithoutConcept(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()
	orders := app.NewOrdersService(st, clock)
	back := app.NewBackofficeService(st, clock)
	cajero := makeUser(t, st, "cajero_avisos", "admin")
	sess := abrirCajaPrincipal(t, st, cajero)
	cobrarConPropina(t, ctx, st, orders, "avisos", "100", "15", cajero, paymentMethodID(t, st, "Efectivo"))
	// Una salida vieja, de antes de los conceptos.
	if _, err := st.Pool.Exec(ctx, `insert into register_cash_movements (session_id, kind, amount, concept, user_id) values ($1, 'salida', 10, 'algo', $2)`, sess, cajero); err != nil {
		t.Fatal(err)
	}
	a, err := back.CashAlerts(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !a.TipsPending.Equal(dec("15")) || a.CashOutsWithoutConcept != 1 {
		t.Fatalf("avisos = %+v; quería 15 de propina y 1 salida sin concepto", a)
	}
}

func TestCashAlertsIsolatedInTheThreeCases(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()
	orders := app.NewOrdersService(st, clock)
	cajero := makeUser(t, st, "cajero_avisos_rls", "admin")
	abrirCajaPrincipal(t, st, cajero)
	cobrarConPropina(t, ctx, st, orders, "avisos rls", "100", "15", cajero, paymentMethodID(t, st, "Efectivo"))
	otra := makeCompany(t, st, "otra-avisos")
	inTheThreeCases(t, defaultCompanyID, otra, func(t *testing.T, as *store.Store, ctx context.Context) {
		a, err := app.NewBackofficeService(as, clock).CashAlerts(ctx)
		if err == nil && a.TipsPending.IsPositive() {
			t.Fatalf("se vio propina de otra empresa: %s", a.TipsPending)
		}
	})
}
