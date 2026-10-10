//go:build integration

package integration

import (
	"context"
	"testing"

	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
)

// Spec 029: el corte presenta las devoluciones igual sin importar el medio, separa la propina
// devuelta de la venta devuelta igual que Ventas, y el Histórico de un turno abierto dice lo mismo
// que Cajas.

func conceptoDe(t *testing.T, b app.CorteBreakdown, metodo, concepto string) decimal.Decimal {
	t.Helper()
	for _, m := range b.Ingresos {
		if m.Method != metodo {
			continue
		}
		for _, it := range m.Items {
			if it.Concept == concepto {
				return it.Amount
			}
		}
	}
	return decimal.Zero
}

func egresoDe(b app.CorteBreakdown, concepto string) decimal.Decimal {
	for _, e := range b.Egresos {
		if e.Concept == concepto {
			return e.Amount
		}
	}
	return decimal.Zero
}

// LAS DEVOLUCIONES DEL CORTE VAN EN UN SOLO LUGAR, Y LA PROPINA DEVUELTA NO ES VENTA DEVUELTA.
//
// Medido en el ambiente de pruebas: la devolución en efectivo salía en «Salidas de efectivo» y la de
// tarjeta en «Devoluciones» dentro de Ingresos; la propina devuelta se contaba dentro de lo devuelto
// en Caja y fuera en Ventas, y la propina del efectivo seguía diciendo $10 con $10 devueltos.
func TestTheCutShowsEveryRefundTheSameWay(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	// Este caso no es del arqueo por terminal (spec 032): la sucursal arquea la tarjeta en automático.
	sinArqueoPorTerminal(t, st)
	ctx := context.Background()
	backoffice := app.NewBackofficeService(st, clock)
	orders := app.NewOrdersService(st, clock)
	sales := app.NewSalesService(st, clock)
	cajero := makeUser(t, st, "cajero_029_corte", "gerente")
	efectivo := paymentMethodID(t, st, "Efectivo")
	tarjeta := paymentMethodID(t, st, "Tarjeta débito")
	principal := registerID(t, st, "Caja principal")
	if _, err := backoffice.OpenSession(ctx, principal, aperturaAMano(decimal.RequireFromString("500")), cajero); err != nil {
		t.Fatalf("abrir la caja: %v", err)
	}

	cobrarConPropina := func(nombre, precio, propina string, metodo int16) int64 {
		ord := crearPedidoSimple(t, ctx, orders, makeProduct(t, st, nombre, decimal.RequireFromString(precio), false), cajero)
		if _, err := orders.Charge(ctx, app.ChargeCmd{OrderID: ord, MethodID: metodo, Amount: decimal.RequireFromString(precio),
			Tip: decimal.RequireFromString(propina), ActorID: cajero}); err != nil {
			t.Fatalf("Charge %s: %v", nombre, err)
		}
		return ord
	}
	cancelarDevolviendo := func(ord int64) {
		if err := orders.CancelarConDevolucion(ctx, app.CancelacionCmd{OrderID: ord, Motivo: "prueba", ActorID: cajero, Devolver: true}); err != nil {
			t.Fatalf("CancelarConDevolucion: %v", err)
		}
	}
	cancelarDevolviendo(cobrarConPropina("029 efectivo propina", "100", "10", efectivo)) // sale $110 del cajón
	cobrarConPropina("029 efectivo venta", "200", "0", efectivo)
	parcial := cobrarConPropina("029 tarjeta parcial", "35", "0", tarjeta)
	cancelarDevolviendo(cobrarConPropina("029 tarjeta propina", "40", "4", tarjeta)) // $44 por la terminal
	refund(t, ctx, orders, parcial, nil, "35", cajero)

	v, err := backoffice.CurrentByRegister(ctx, principal)
	if err != nil {
		t.Fatalf("CurrentByRegister: %v", err)
	}
	b := v.Breakdown

	if s := egresoDe(b, "Salidas de efectivo"); !s.IsZero() {
		t.Fatalf("«Salidas de efectivo» = %s: la devolución en efectivo se presentó como salida y la de tarjeta como devolución", s)
	}
	if d := conceptoDe(t, b, "Efectivo", "Devoluciones"); !d.Equal(decimal.RequireFromString("-100")) {
		t.Fatalf("Devoluciones del efectivo = %s, quiere -100 (la venta; la propina va aparte)", d)
	}
	if d := conceptoDe(t, b, "Tarjeta débito", "Devoluciones"); !d.Equal(decimal.RequireFromString("-75")) {
		t.Fatalf("Devoluciones de tarjeta = %s, quiere -75: la propina devuelta se contó como venta devuelta", d)
	}
	for _, m := range v.Totals {
		if (m.MethodID == int(efectivo) || m.MethodID == int(tarjeta)) && !m.Tips.IsZero() {
			t.Fatalf("propina de %s = %s, quiere 0: se devolvió y el corte la seguía contando", m.Name, m.Tips)
		}
	}

	// El desglose explica el esperado al centavo: fondo + ingresos − egresos = Σ esperados.
	esperado := decimal.Zero
	for _, m := range v.Totals {
		if m.Expected != nil {
			esperado = esperado.Add(*m.Expected)
		}
	}
	explicado := v.OpeningCash.Add(b.IngresosTotal).Sub(b.EgresosTotal)
	if !explicado.Equal(esperado) {
		t.Fatalf("fondo + ingresos − egresos = %s y los esperados suman %s: el desglose dejó de explicar el corte "+
			"(una devolución quedó en Devoluciones y en Salidas, o en ninguno)", explicado, esperado)
	}
	for _, m := range v.Totals {
		if m.MethodID == int(efectivo) && !m.Expected.Equal(decimal.RequireFromString("700")) {
			t.Fatalf("esperado del efectivo = %s, quiere 700 (500 + 300 + 10 − 110): presentar no mueve el esperado", m.Expected)
		}
	}

	// La misma cifra devuelta que Ventas.
	hoy := diaDe(domain.BusinessDate(fixedNow, sales.Location(ctx)))
	r, err := sales.Summary(ctx, filtroDelDia(hoy, hoy))
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range r.ByMethod {
		corte := conceptoDe(t, b, m.Method, "Devoluciones").Neg()
		if !corte.Equal(m.Refunds) {
			t.Fatalf("%s: Ventas dice %s devuelto y el corte %s", m.Method, m.Refunds, corte)
		}
	}

	entregarPendientes(t, st)
	// CERRADO dice lo mismo. El cierre guarda la propina BRUTA —el desglose del corte cerrado resta
	// la devuelta al leerlo—; guardar la neta la restaría dos veces.
	if _, err := backoffice.CloseSession(ctx, principal, cajero, cierreDelCajonAMano(t, st,
		map[int]decimal.Decimal{int(efectivo): decimal.RequireFromString("700")})); err != nil {
		t.Fatalf("CloseSession: %v", err)
	}
	cerrado, err := backoffice.SessionDetail(ctx, v.ID)
	if err != nil {
		t.Fatal(err)
	}
	if d := conceptoDe(t, cerrado.Breakdown, "Efectivo", "Devoluciones"); !d.Equal(decimal.RequireFromString("-100")) {
		t.Fatalf("cerrado: Devoluciones del efectivo = %s, quiere -100", d)
	}
	if !cerrado.Breakdown.IngresosTotal.Equal(b.IngresosTotal) || !cerrado.Breakdown.EgresosTotal.Equal(b.EgresosTotal) {
		t.Fatalf("cerrado: ingresos %s / egresos %s; abierto decía %s / %s",
			cerrado.Breakdown.IngresosTotal, cerrado.Breakdown.EgresosTotal, b.IngresosTotal, b.EgresosTotal)
	}
	for _, m := range cerrado.Totals {
		if m.MethodID == int(efectivo) && !m.Tips.IsZero() {
			t.Fatalf("cerrado: propina del efectivo = %s, quiere 0", m.Tips)
		}
	}
}

// EL HISTÓRICO DE UN TURNO ABIERTO DICE LO MISMO QUE CAJAS.
//
// Medido: «Ingresos $0 · Sin ingresos» desde Histórico mientras Cajas decía $630. El detalle leía
// los totales guardados al cerrar, y un turno abierto no los tiene.
func TestTheHistoryOfAnOpenShiftIsLive(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()
	backoffice := app.NewBackofficeService(st, clock)
	orders := app.NewOrdersService(st, clock)
	cajero := makeUser(t, st, "cajero_029_historico", "gerente")
	efectivo := paymentMethodID(t, st, "Efectivo")
	principal := registerID(t, st, "Caja principal")
	sess, err := backoffice.OpenSession(ctx, principal, aperturaAMano(decimal.RequireFromString("100")), cajero)
	if err != nil {
		t.Fatalf("abrir la caja: %v", err)
	}
	pedidoCobradoParcial(t, ctx, st, orders, "029-historico", "630", "630", cajero, efectivo, false)

	vivo, err := backoffice.CurrentByRegister(ctx, principal)
	if err != nil {
		t.Fatal(err)
	}
	detalle, err := backoffice.SessionDetail(ctx, sess.ID)
	if err != nil {
		t.Fatalf("SessionDetail: %v", err)
	}
	if !detalle.Breakdown.IngresosTotal.Equal(vivo.Breakdown.IngresosTotal) || detalle.Breakdown.IngresosTotal.IsZero() {
		t.Fatalf("Histórico dice ingresos %s y Cajas %s", detalle.Breakdown.IngresosTotal, vivo.Breakdown.IngresosTotal)
	}
	if len(detalle.Totals) == 0 {
		t.Fatal("el detalle de un turno abierto llegó sin totales por medio")
	}
}

// UN MEDIO EN NEGATIVO DICE POR QUÉ.
//
// Un turno que solo devolvió con tarjeta una venta cobrada en otro turno sale en −$300 en tarjeta, y
// sin nota el cajero busca un faltante que no existe.
func TestANegativeMethodInTheCutExplainsItself(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()
	backoffice := app.NewBackofficeService(st, clock)
	orders := app.NewOrdersService(st, clock)
	cajero := makeUser(t, st, "cajero_029_negativo", "gerente")
	tarjeta := paymentMethodID(t, st, "Tarjeta débito")
	primero := abrirCajaPrincipal(t, st, cajero)
	ord := pedidoCobradoParcial(t, ctx, st, orders, "029-negativo", "300", "300", cajero, tarjeta, false)
	if _, err := st.Pool.Exec(ctx, `update register_sessions set status = 'cerrada', closed_at = now(), closed_by = $2 where id = $1`, primero, cajero); err != nil {
		t.Fatal(err)
	}
	segundo := abrirCajaPrincipal(t, st, cajero)
	refund(t, ctx, orders, ord, nil, "300", cajero)

	v, err := backoffice.SessionDetail(ctx, segundo)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range v.Breakdown.Ingresos {
		if m.Method == "Tarjeta débito" {
			if !m.Total.Equal(decimal.RequireFromString("-300")) || m.Note == "" {
				t.Fatalf("tarjeta = %s con nota %q; quiere -300 y la nota que lo explica", m.Total, m.Note)
			}
			return
		}
	}
	t.Fatal("la tarjeta no salió en el desglose del segundo turno")
}
