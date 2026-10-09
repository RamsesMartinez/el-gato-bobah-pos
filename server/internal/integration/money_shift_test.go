//go:build integration

package integration

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
)

// El corte de la spec 031: que espere exactamente el dinero que el turno movió, por cada medio.

func dec(s string) decimal.Decimal { return decimal.RequireFromString(s) }

func totalOf(t *testing.T, totals []app.MethodTotal, name string) decimal.Decimal {
	t.Helper()
	for _, m := range totals {
		if m.Name == name {
			if m.Expected == nil {
				t.Fatalf("%q sin esperado", name)
			}
			return *m.Expected
		}
	}
	t.Fatalf("%q no está en el corte", name)
	return decimal.Zero
}

func conceptOf(b app.CorteBreakdown, method, concept string) (decimal.Decimal, bool) {
	for _, m := range b.Ingresos {
		if m.Method != method {
			continue
		}
		for _, it := range m.Items {
			if it.Concept == concept {
				return it.Amount, true
			}
		}
	}
	return decimal.Zero, false
}

// closeBySQL cierra el turno abierto sin pasar por el arqueo: el caso que se prueba es lo que
// ocurre DESPUÉS, sin turno.
func closeBySQL(t *testing.T, st *store.Store, session int64) {
	t.Helper()
	if _, err := st.Pool.Exec(context.Background(),
		`update register_sessions set status = 'cerrada', closed_at = now() where id = $1`, session); err != nil {
		t.Fatal(err)
	}
}

// D6: UNA DEVOLUCIÓN POR TARJETA BAJA EL ESPERADO DE LA TARJETA EN SU TURNO.
//
// Con «declarar automático», el corte firmaba como recibidos $300 que la terminal ya le había
// regresado al cliente; sin él, marcaba un faltante de $300 que nadie podía explicar.
func TestACardRefundLowersTheShiftExpected(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()
	orders := app.NewOrdersService(st, clock)
	back := app.NewBackofficeService(st, clock)

	cajero := makeUser(t, st, "cajero_d6", "gerente")
	tarjeta := paymentMethodID(t, st, "Tarjeta débito")
	principal := registerID(t, st, "Caja principal")
	abrirCajaPrincipal(t, st, cajero)

	ord := pedidoCobradoParcial(t, ctx, st, orders, "d6", "300", "300", cajero, tarjeta, true)
	if err := orders.CancelarConDevolucion(ctx, app.CancelacionCmd{OrderID: ord, Motivo: "no llegó", ActorID: cajero, Devolver: true}); err != nil {
		t.Fatal(err)
	}

	view, err := back.CurrentByRegister(ctx, principal)
	if err != nil {
		t.Fatal(err)
	}
	if got := totalOf(t, view.Totals, "Tarjeta débito"); !got.IsZero() {
		t.Fatalf("el esperado de tarjeta es %s con el cobro devuelto en el mismo turno, quiere 0", got)
	}
	if len(view.Refunds) != 1 || !view.Refunds[0].Amount.Equal(dec("300")) || view.Refunds[0].FromDrawer {
		t.Fatalf("devoluciones del turno = %+v, quiere una de 300 por tarjeta, fuera del cajón", view.Refunds)
	}
	if got, ok := conceptOf(view.Breakdown, "Tarjeta débito", "Devoluciones"); !ok || !got.Equal(dec("-300")) {
		t.Fatalf("el desglose de tarjeta dice Devoluciones = %s (%v), quiere -300", got, ok)
	}
}

// D6 + decisión del dueño: LA DEVOLUCIÓN VA AL TURNO EN QUE OCURRE, NO AL DEL COBRO.
func TestARefundInALaterShiftBelongsToThatShift(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()
	orders := app.NewOrdersService(st, clock)
	back := app.NewBackofficeService(st, clock)

	cajero := makeUser(t, st, "cajero_d6b", "gerente")
	tarjeta := paymentMethodID(t, st, "Tarjeta débito")
	principal := registerID(t, st, "Caja principal")
	a, err := back.OpenSession(ctx, principal, aperturaAMano(decimal.Zero), cajero)
	if err != nil {
		t.Fatal(err)
	}
	ord := pedidoCobradoParcial(t, ctx, st, orders, "d6b", "300", "300", cajero, tarjeta, false)
	if _, err := back.CloseSession(ctx, principal, cajero, cierreDelCajonAMano(t, st, nil)); err != nil {
		t.Fatalf("cerrar A: %v", err)
	}
	if _, err := back.OpenSession(ctx, principal, aperturaAMano(decimal.Zero), cajero); err != nil {
		t.Fatal(err)
	}
	refund(t, ctx, orders, ord, nil, "300", cajero)

	b, err := back.CurrentByRegister(ctx, principal)
	if err != nil {
		t.Fatal(err)
	}
	if got := totalOf(t, b.Totals, "Tarjeta débito"); !got.Equal(dec("-300")) {
		t.Fatalf("el turno B espera %s de tarjeta, quiere -300: ahí se regresó el dinero", got)
	}
	detA, err := back.SessionDetail(ctx, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got := totalOf(t, detA.Totals, "Tarjeta débito"); !got.Equal(dec("300")) {
		t.Fatalf("el turno A ya firmado cambió a %s", got)
	}
	// Un esperado negativo no rompe el cierre.
	if _, err := back.CloseSession(ctx, principal, cajero, cierreDelCajonAMano(t, st, nil)); err != nil {
		t.Fatalf("cerrar B con esperado negativo: %v", err)
	}
}

// D7: SIN TURNO NO SE DEVUELVE EFECTIVO, PORQUE NO QUEDARÍA EN NINGÚN ARQUEO.
func TestACashRefundWithoutAnOpenShiftIsRejected(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()
	orders := app.NewOrdersService(st, clock)

	cajero := makeUser(t, st, "cajero_d7", "gerente")
	efectivo := paymentMethodID(t, st, "Efectivo")
	sess := abrirCajaPrincipal(t, st, cajero)
	ord := pedidoCobradoParcial(t, ctx, st, orders, "d7", "120", "120", cajero, efectivo, false)
	closeBySQL(t, st, sess)

	err := orders.Devolver(ctx, app.DevolucionCmd{OrderID: ord, Monto: dec("120"), Motivo: "ayer", ActorID: cajero})
	if !errors.Is(err, domain.ErrCashRefundNeedsOpenRegister) {
		t.Fatalf("devolver efectivo sin turno: err = %v, quiere ErrCashRefundNeedsOpenRegister", err)
	}
	if total := refundedTotal(t, st, ord); !total.IsZero() {
		t.Fatalf("quedó registrada una devolución de %s sin salida de caja", total)
	}
}

// D7: LA DE TARJETA SIN TURNO SE REGISTRA Y ENTRA AL SIGUIENTE TURNO; LA DE ANTES DEL ÚLTIMO
// CIERRE, NO.
func TestACardRefundWithoutAShiftJoinsTheNextOne(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()
	orders := app.NewOrdersService(st, clock)
	back := app.NewBackofficeService(st, clock)

	cajero := makeUser(t, st, "cajero_d7b", "gerente")
	tarjeta := paymentMethodID(t, st, "Tarjeta débito")
	principal := registerID(t, st, "Caja principal")
	sess := abrirCajaPrincipal(t, st, cajero)
	viejo := pedidoCobradoParcial(t, ctx, st, orders, "d7b_viejo", "50", "50", cajero, tarjeta, false)
	ord := pedidoCobradoParcial(t, ctx, st, orders, "d7b", "80", "80", cajero, tarjeta, false)

	// Una devolución sin turno de ANTES del último cierre: no es del turno que viene.
	if _, err := st.Pool.Exec(ctx, `
		insert into order_refunds (order_id, payment_method_id, amount, reason, refunded_by, created_at)
		values ($1, $2, 50, 'vieja', $3, now() - interval '2 days')`, viejo, tarjeta, cajero); err != nil {
		t.Fatal(err)
	}
	closeBySQL(t, st, sess)
	refund(t, ctx, orders, ord, nil, "80", cajero)

	nuevo, err := back.OpenSession(ctx, principal, aperturaAMano(decimal.Zero), cajero)
	if err != nil {
		t.Fatal(err)
	}
	if got := totalOf(t, nuevo.Totals, "Tarjeta débito"); !got.Equal(dec("-80")) {
		t.Fatalf("el turno nuevo espera %s de tarjeta, quiere -80: la devolución sin turno entra aquí", got)
	}
	var vieja *int64
	if err := st.Pool.QueryRow(ctx, `select register_session_id from order_refunds where order_id = $1`, viejo).Scan(&vieja); err != nil {
		t.Fatal(err)
	}
	if vieja != nil {
		t.Fatalf("la devolución de hace dos días entró al turno %d", *vieja)
	}
}

// D5: EL PAGO DE UN PEDIDO DE PLATAFORMA ACEPTADO SIN TURNO ENTRA AL TURNO QUE LO RECLAMA.
//
// Al abrir turno se reclamaba el pedido y no su pago: la venta salía en el corte y su dinero en
// ninguno, y «Uber efectivo» aparecía como sobrante del cajón.
func TestAnOrphanPlatformPaymentJoinsTheShiftThatClaimsIt(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()

	const llave = "llave-para-el-pago-huerfano"
	empresa := makeCompany(t, st, "empresa-pago-huerfano")
	usuario := makeUserIn(t, st, empresa, "cajera-pago-huerfano", "cajero")
	tiendaConLlave(t, st, empresa, "tienda-pago-h", llave)
	ctxT, soltar, err := st.AcquireTenant(ctx, empresa)
	if err != nil {
		t.Fatal(err)
	}
	defer soltar()

	svc := app.NewPedidosDePlataformaService(st,
		fixedClients{deciders: map[string]app.DecisorDePedidos{"Uber Eats": &decisorFalso{detalle: []byte(detalleDeUnPedido)}}}, signingKeyCipher,
		"sandbox", clock)
	cuerpo := avisoDePedido("evt-pago-huerfano", "tienda-pago-h")
	if _, err := svc.RecibirAviso(ctx, "Uber Eats", app.AvisoEntrante{
		Crudo: cuerpo, Firma: domain.FirmarParaPrueba(cuerpo, llave), Ambiente: "sandbox",
	}); err != nil {
		t.Fatal(err)
	}
	pend, err := svc.Pendientes(ctxT)
	if err != nil || len(pend) != 1 {
		t.Fatalf("pendientes: %v %d", err, len(pend))
	}
	acc, err := svc.Aceptar(ctxT, pend[0].ID, usuario)
	if err != nil {
		t.Fatal(err)
	}

	var principal int64
	if err := st.Pool.QueryRow(ctxT,
		`insert into cash_registers (company_id, name, is_primary) values ($1, 'Caja', true) returning id`, empresa).Scan(&principal); err != nil {
		t.Fatal(err)
	}
	vista, err := app.NewBackofficeService(st, clock).OpenSession(ctxT, principal, app.AperturaCmd{}, usuario)
	if err != nil {
		t.Fatal(err)
	}
	var sesion *int64
	var monto decimal.Decimal
	var metodo string
	if err := st.Pool.QueryRow(ctx, `select p.register_session_id, p.amount, pm.name from order_payments p
		join payment_methods pm on pm.id = p.payment_method_id where p.order_id = $1`, acc.ID).Scan(&sesion, &monto, &metodo); err != nil {
		t.Fatal(err)
	}
	if sesion == nil || *sesion != vista.ID {
		t.Fatalf("el pago quedó en el turno %v, quiere %d: la venta está en ese corte y su dinero no", sesion, vista.ID)
	}
	if got := totalOf(t, vista.Totals, metodo); !got.Equal(monto) {
		t.Fatalf("«%s» espera %s en el turno que reclamó el pedido, quiere %s", metodo, got, monto)
	}
}

// D12: UN PEDIDO DE UN TURNO COBRADO EN OTRO SE EXPLICA EN LOS DOS CORTES.
//
// El turno A cerraba con $554 vendidos sin cobrar. Cobrados en B, al reabrir A su «sin cobrar» se
// recalculaba en $0 —A quedaba con una venta que ningún método explicaba— y B esperaba $554 en
// efectivo sin una venta suya que los justificara.
func TestAnOrderChargedInALaterShiftIsExplainedInBoth(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()
	orders := app.NewOrdersService(st, clock)
	back := app.NewBackofficeService(st, clock)

	cajero := makeUser(t, st, "cajero_d12", "gerente")
	efectivo := paymentMethodID(t, st, "Efectivo")
	principal := registerID(t, st, "Caja principal")
	a, err := back.OpenSession(ctx, principal, aperturaAMano(decimal.Zero), cajero)
	if err != nil {
		t.Fatal(err)
	}
	prod := makeProduct(t, st, "Fiado d12", dec("554"), false)
	ord := crearPedidoSimple(t, ctx, orders, prod, cajero)
	if _, err := st.Pool.Exec(ctx, `update orders set status = 'entregada', completed_at = now(), service_type = 'domicilio',
		delivery_platform_id = (select min(id) from delivery_platforms) where id = $1`, ord); err != nil {
		t.Fatal(err)
	}
	if _, err := back.CloseSession(ctx, principal, cajero, cierreDelCajonAMano(t, st, nil)); err != nil {
		t.Fatalf("cerrar A: %v", err)
	}
	// Un fiado de ANTES de la regla (2026-10-09): un entregado de mostrador que debe ya no deja
	// cerrar, así que el turno A se cierra con el pedido disfrazado de plataforma y se le regresa
	// su forma después. Es el dato viejo que este test cuida.
	if _, err := st.Pool.Exec(ctx, `update orders set service_type = 'mostrador', delivery_platform_id = null where id = $1`, ord); err != nil {
		t.Fatal(err)
	}
	if _, err := back.OpenSession(ctx, principal, aperturaAMano(decimal.Zero), cajero); err != nil {
		t.Fatal(err)
	}
	charge(t, ctx, orders, ord, efectivo, "554", cajero)

	detA, err := back.SessionDetail(ctx, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !detA.Uncollected.Equal(dec("554")) {
		t.Fatalf("A dice sin cobrar %s después de que B cobró, quiere 554: lo firmado no cambia", detA.Uncollected)
	}
	b, err := back.CurrentByRegister(ctx, principal)
	if err != nil {
		t.Fatal(err)
	}
	if got, ok := conceptOf(b.Breakdown, "Efectivo", "Cobros de otros turnos"); !ok || !got.Equal(dec("554")) {
		t.Fatalf("B explica %s (%v) de cobros de otros turnos, quiere 554", got, ok)
	}
	if got, ok := conceptOf(b.Breakdown, "Efectivo", "Ventas"); ok && !got.IsZero() {
		t.Fatalf("B cuenta %s como ventas suyas", got)
	}
}

// D8: LO QUE CONFIRMA MIENTRAS SE CIERRA QUEDA DENTRO DEL ESPERADO FIRMADO O REBOTA.
//
// El cierre calculaba el esperado fuera de su transacción. Un cobro que confirmaba entre esa lectura
// y el cierre quedaba en el turno cerrado sin entrar a su esperado: sobrante firmado, y el pago ya
// no se podía devolver porque «es de un turno cerrado».
func TestWhatCommitsDuringTheCloseIsInsideTheSignedExpected(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()
	orders := app.NewOrdersService(st, clock)
	back := app.NewBackofficeService(st, clock)

	cajero := makeUser(t, st, "cajero_d8", "gerente")
	efectivo := paymentMethodID(t, st, "Efectivo")
	principal := registerID(t, st, "Caja principal")
	prod := makeProduct(t, st, "Cierre d8", dec("100"), false)

	for intento := 0; intento < 15; intento++ {
		sess, err := back.OpenSession(ctx, principal, aperturaAMano(decimal.Zero), cajero)
		if err != nil {
			t.Fatal(err)
		}
		// Entregado sin cobrar: no bloquea el cierre, y el cobro «por monto» entra aunque el turno del
		// pedido sea el que se está cerrando.
		ord := crearPedidoSimple(t, ctx, orders, prod, cajero)
		if _, err := st.Pool.Exec(ctx, `update orders set status = 'entregada', completed_at = now() where id = $1`, ord); err != nil {
			t.Fatal(err)
		}
		manual := decimal.Zero
		errs := concurrently(3, func(i int) error {
			switch i {
			case 0:
				_, err := back.CloseSession(ctx, principal, cajero, app.CierreCmd{Total: &manual, Motivo: "prueba de carrera"})
				return err
			case 1:
				_, err := orders.Charge(ctx, app.ChargeCmd{OrderID: ord, MethodID: efectivo, Amount: dec("100"), ActorID: cajero})
				return err
			default:
				_, err := back.RecordCashMovement(ctx, principal, domain.CashEntrada, dec("7"), "cambio", cajero)
				return err
			}
		})
		if errs[0] != nil {
			// Si el cierre rebota no hay nada firmado que revisar. Se cierra para el siguiente.
			if _, err := st.Pool.Exec(ctx, `update register_sessions set status = 'cerrada', closed_at = now() where id = $1`, sess.ID); err != nil {
				t.Fatal(err)
			}
			continue
		}
		var stored, live decimal.Decimal
		if err := st.Pool.QueryRow(ctx, `select expected from register_session_totals where session_id = $1 and payment_method_id = $2`,
			sess.ID, efectivo).Scan(&stored); err != nil {
			t.Fatal(err)
		}
		if err := st.Pool.QueryRow(ctx, `
			select coalesce((select sum(amount + tip_amount) from order_payments where register_session_id = $1 and payment_method_id = $2), 0)
			     + coalesce((select sum(case when kind = 'entrada' then amount else -amount end) from register_cash_movements where session_id = $1), 0)`,
			sess.ID, efectivo).Scan(&live); err != nil {
			t.Fatal(err)
		}
		if !stored.Equal(live) {
			t.Fatalf("intento %d: el corte firmó %s de efectivo y el turno cerrado tiene %s (cobro: %v, movimiento: %v)",
				intento, stored, live, errs[1], errs[2])
		}
		for i, err := range errs[1:] {
			if err != nil && !errors.Is(err, domain.ErrNoOpenRegister) && !errors.Is(err, domain.ErrNotFound) &&
				!strings.Contains(err.Error(), "abierta") {
				t.Fatalf("intento %d: la operación %d rebotó con %v, quiere «no hay caja abierta»", intento, i+1, err)
			}
		}
	}
}

// LAS CONSULTAS NUEVAS DEL CORTE NO ALCANZAN LAS DEVOLUCIONES NI LOS PAGOS DE OTRA EMPRESA, en los
// tres casos donde RLS ya falló: otra empresa, conexión reciclada y sin empresa.
func TestTheShiftRefundQueriesAreIsolated(t *testing.T) {
	t.Parallel()
	owner := newTestStore(t)
	ctx := context.Background()
	orders := app.NewOrdersService(owner, clock)
	other := makeCompany(t, owner, "otra-devoluciones-031")

	cajero := makeUser(t, owner, "cajero_aisla_031", "gerente")
	tarjeta := paymentMethodID(t, owner, "Tarjeta débito")
	sess := abrirCajaPrincipal(t, owner, cajero)
	ord := pedidoCobradoParcial(t, ctx, owner, orders, "aisla_031", "90", "90", cajero, tarjeta, false)
	refund(t, ctx, orders, ord, nil, "90", cajero)
	// Una huérfana de la dueña, para ver que el reclamo de otra empresa no la toca.
	if _, err := owner.Pool.Exec(ctx, `update order_refunds set register_session_id = null where order_id = $1`, ord); err != nil {
		t.Fatal(err)
	}

	inTheThreeCases(t, defaultCompanyID, other, func(t *testing.T, st *store.Store, ctx context.Context) {
		q := st.QC(ctx)
		if rows, err := q.ListSessionRefunds(ctx, &sess); err == nil && len(rows) != 0 {
			t.Fatalf("se vieron %d devoluciones de la dueña", len(rows))
		}
		if rows, err := q.ExpectedByMethodForSession(ctx, sess); err == nil {
			for _, r := range rows {
				if !r.Refunded.IsZero() || !r.Expected.IsZero() {
					t.Fatalf("el esperado de otra empresa vio dinero de la dueña: %+v", r)
				}
			}
		}
		_ = q.ClaimOrphanRefunds(ctx, sess)
	})
	var claimed *int64
	if err := owner.Pool.QueryRow(ctx, `select register_session_id from order_refunds where order_id = $1`, ord).Scan(&claimed); err != nil {
		t.Fatal(err)
	}
	if claimed != nil {
		t.Fatalf("una sesión ajena reclamó la devolución de la dueña al turno %d", *claimed)
	}
}

// D7 (revisión): UNA CAJA SECUNDARIA QUE ABRE PRIMERO NO SE QUEDA CON LA DEVOLUCIÓN HUÉRFANA.
//
// La secundaria no vende y su esperado ignora las devoluciones: si la reclamaba, la devolución no
// restaba de ningún corte y la principal cerraba con sobrante.
func TestASecondaryRegisterDoesNotClaimAnOrphanRefund(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()
	orders := app.NewOrdersService(st, clock)
	back := app.NewBackofficeService(st, clock)

	cajero := makeUser(t, st, "cajero_d7c", "gerente")
	tarjeta := paymentMethodID(t, st, "Tarjeta débito")
	principal := registerID(t, st, "Caja principal")
	sess := abrirCajaPrincipal(t, st, cajero)
	ord := pedidoCobradoParcial(t, ctx, st, orders, "d7c", "70", "70", cajero, tarjeta, false)
	closeBySQL(t, st, sess)
	refund(t, ctx, orders, ord, nil, "70", cajero)

	secundaria, err := back.CreateCashRegister(ctx, "Barra d7c")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := back.OpenSession(ctx, secundaria.ID, aperturaAMano(decimal.Zero), cajero); err != nil {
		t.Fatal(err)
	}
	nueva, err := back.OpenSession(ctx, principal, aperturaAMano(decimal.Zero), cajero)
	if err != nil {
		t.Fatal(err)
	}
	if got := totalOf(t, nueva.Totals, "Tarjeta débito"); !got.Equal(dec("-70")) {
		t.Fatalf("la principal espera %s de tarjeta, quiere -70: la secundaria se quedó con la devolución", got)
	}
}

// D8 (revisión): ACEPTAR UN PEDIDO DE PLATAFORMA MIENTRAS SE CIERRA EL TURNO.
//
// Aceptar leía el turno abierto sin candado: un cierre que firmaba entre esa lectura y el pago
// dejaba el pago en un turno cerrado y fuera de su esperado. Ahora o entra al esperado firmado, o
// queda sin turno y lo reclama la apertura siguiente.
func TestAcceptingAPlatformOrderDuringTheCloseStaysInsideTheSignedExpected(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()

	const llave = "llave-para-cerrar-y-aceptar"
	empresa := makeCompany(t, st, "empresa-cierre-aceptar")
	usuario := makeUserIn(t, st, empresa, "cajera-cierre-aceptar", "gerente")
	tiendaConLlave(t, st, empresa, "tienda-cierre-a", llave)
	ctxT, soltar, err := st.AcquireTenant(ctx, empresa)
	if err != nil {
		t.Fatal(err)
	}
	defer soltar()
	var principal int64
	if err := st.Pool.QueryRow(ctx,
		`insert into cash_registers (company_id, name, is_primary) values ($1, 'Caja', true) returning id`, empresa).Scan(&principal); err != nil {
		t.Fatal(err)
	}
	back := app.NewBackofficeService(st, clock)

	for intento := 0; intento < 10; intento++ {
		folio := "ped-cierre-" + itoa(intento)
		detalle := strings.Replace(detalleDeUnPedido, `"id":"ped-1"`, `"id":"`+folio+`"`, 1)
		svc := app.NewPedidosDePlataformaService(st,
			fixedClients{deciders: map[string]app.DecisorDePedidos{"Uber Eats": &decisorFalso{detalle: []byte(detalle)}}}, signingKeyCipher,
			"sandbox", clock)
		cuerpo := avisoDePedido("evt-cierre-"+itoa(intento), "tienda-cierre-a")
		if _, err := svc.RecibirAviso(ctx, "Uber Eats", app.AvisoEntrante{
			Crudo: cuerpo, Firma: domain.FirmarParaPrueba(cuerpo, llave), Ambiente: "sandbox",
		}); err != nil {
			t.Fatal(err)
		}
		pend, err := svc.Pendientes(ctxT)
		if err != nil || len(pend) != 1 {
			t.Fatalf("pendientes: %v %d", err, len(pend))
		}
		sess, err := back.OpenSession(ctxT, principal, app.AperturaCmd{}, usuario)
		if err != nil {
			t.Fatal(err)
		}
		// Cada goroutine con su propia conexión de empresa: compartir una no es lo que pasa en producción.
		concurrently(2, func(i int) error {
			c, rel, err := st.AcquireTenant(ctx, empresa)
			if err != nil {
				return err
			}
			defer rel()
			if i == 0 {
				_, err := back.CloseSession(c, principal, usuario, app.CierreCmd{})
				return err
			}
			_, err = svc.Aceptar(c, pend[0].ID, usuario)
			return err
		})
		var status string
		if err := st.Pool.QueryRow(ctx, `select status from register_sessions where id = $1`, sess.ID).Scan(&status); err != nil {
			t.Fatal(err)
		}
		if status != "cerrada" {
			if _, err := st.Pool.Exec(ctx, `update register_sessions set status = 'cerrada', closed_at = now() where id = $1`, sess.ID); err != nil {
				t.Fatal(err)
			}
			continue
		}
		var stored, live decimal.Decimal
		if err := st.Pool.QueryRow(ctx, `
			select coalesce((select sum(expected) from register_session_totals where session_id = $1), 0),
			       coalesce((select sum(amount) from order_payments where register_session_id = $1), 0)`, sess.ID).Scan(&stored, &live); err != nil {
			t.Fatal(err)
		}
		if !stored.Equal(live) {
			t.Fatalf("intento %d: el corte firmó %s y el turno cerrado tiene %s en pagos", intento, stored, live)
		}
	}
}
