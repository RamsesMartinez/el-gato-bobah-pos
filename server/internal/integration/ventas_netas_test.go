//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"
	"uuid"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store/db"
)

// Spec 029: el Total de Ventas es lo que se puede facturar, y el detalle y la API dicen la verdad.
// Los cuatro hallazgos de la prueba por API en el ambiente de pruebas están aquí, cada uno contra
// el caso exacto que se midió.

// EL DETALLE DE UN PEDIDO DICE CUÁNTO SE DEVOLVIÓ.
//
// `GET /orders/:id` respondía `"refund":"0"` siempre: `load` nunca llenaba el campo, solo el tablero
// y las entregadas. Medido con un pedido devuelto completo por $100. Es el otro extremo del tope de
// una devolución, así que una pantalla que lo leyera ofrecería devolver dos veces lo mismo.
func TestOrderDetailCarriesTheRefundedAmount(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()
	orders := app.NewOrdersService(st, clock)
	cajero := makeUser(t, st, "cajero_029_detalle", "gerente")
	efectivo := paymentMethodID(t, st, "Efectivo")
	abrirCajaPrincipal(t, st, cajero)

	ord := pedidoCobradoParcial(t, ctx, st, orders, "029-detalle", "100", "100", cajero, efectivo, true)
	refund(t, ctx, orders, ord, nil, "100", cajero)

	v, err := orders.Detail(ctx, ord)
	if err != nil {
		t.Fatalf("Detail: %v", err)
	}
	if !v.Refund.Equal(decimal.RequireFromString("100")) {
		t.Fatalf("refund del detalle = %s, quiere 100: el detalle decía 0 de un pedido devuelto completo", v.Refund)
	}
}

// DEVOLVER UN PRODUCTO YA DEVUELTO DICE QUE NO QUEDA NADA.
//
// Sin monto se pide «lo que queda», que es $0, y el rechazo decía «el monto a devolver no es una
// cantidad de dinero». Igual tras devolver la cuenta entera.
func TestRefundingAFullyRefundedLineSaysNothingIsLeft(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()
	orders := app.NewOrdersService(st, clock)
	cajero := makeUser(t, st, "cajero_029_renglon", "gerente")
	efectivo := paymentMethodID(t, st, "Efectivo")
	abrirCajaPrincipal(t, st, cajero)

	ord, lines := twoLineOrder(t, ctx, st, orders, "029r", "60", "40", cajero)
	charge(t, ctx, orders, ord, efectivo, "100", cajero)
	refundLine(t, ctx, orders, ord, lines[0], "60", cajero)

	for _, caso := range []struct {
		nombre string
		antes  func()
		quiere error
	}{
		{"renglón ya devuelto", func() {}, domain.ErrNothingLeftOnLine},
		// Con el pedido entero devuelto lo cierto es eso, no algo del producto.
		{"tras devolver la cuenta entera", func() { refund(t, ctx, orders, ord, nil, "40", cajero) }, domain.ErrNothingLeftToRefund},
	} {
		caso.antes()
		monto, err := orders.PorDevolver(ctx, ord, &lines[0])
		if err != nil {
			t.Fatalf("%s: PorDevolver: %v", caso.nombre, err)
		}
		err = orders.Devolver(ctx, app.DevolucionCmd{OrderID: ord, LineID: &lines[0], Monto: monto, Motivo: "prueba", ActorID: cajero})
		if !errors.Is(err, caso.quiere) || err.Error() != caso.quiere.Error() {
			t.Fatalf("%s: err = %v, quiere %v", caso.nombre, err, caso.quiere)
		}
	}

	// Un producto de un pedido SIN cobros: nada se devolvió nunca, así que «ya no queda nada por
	// devolver de ese producto» sería falso. Dice que no se ha cobrado, como antes.
	sinCobrar, lineas := twoLineOrder(t, ctx, st, orders, "029s", "60", "40", cajero)
	monto, err := orders.PorDevolver(ctx, sinCobrar, &lineas[0])
	if err != nil {
		t.Fatal(err)
	}
	err = orders.Devolver(ctx, app.DevolucionCmd{OrderID: sinCobrar, LineID: &lineas[0], Monto: monto, Motivo: "prueba", ActorID: cajero})
	if !errors.Is(err, domain.ErrSinCobrosQueDevolver) {
		t.Fatalf("producto de un pedido sin cobros: err = %v, quiere ErrSinCobrosQueDevolver", err)
	}
}

// UN PEDIDO DE PLATAFORMA DE MOSTRADOR ES UN ERROR DE CAPTURA, NO DEL SERVIDOR.
//
// La base lo rechaza con `orders_servicio_de_plataforma` (23514) y ese error subía crudo: 500.
func TestAPlatformOrderAtTheCounterIsAValidationError(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()
	orders := app.NewOrdersService(st, clock)
	cajero := makeUser(t, st, "cajero_029_plataforma", "gerente")
	abrirCajaPrincipal(t, st, cajero)
	uber := platformID(t, st, defaultCompanyID, "Uber Eats")
	prod := makeProduct(t, st, "Crepa 029", decimal.RequireFromString("135"), false)

	_, err := orders.Create(ctx, app.CreateOrderCmd{
		ClientUUID: uuid.New(), ServiceType: "mostrador", DeliveryPlatformID: &uber, OpenedBy: cajero,
		Lines: []domain.OrderLineInput{{ProductID: prod, Qty: decimal.RequireFromString("1")}},
	})
	if !errors.Is(err, domain.ErrPlatformOrderAtCounter) {
		t.Fatalf("err = %v, quiere ErrPlatformOrderAtCounter (un 4xx, no el check de la base)", err)
	}
}

// MEDIO CENTAVO NO SALDA NADA.
//
// $0.005 se redondeaba a $0.01 y entraba como pago.
func TestChargingLessThanACentIsRejected(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()
	orders := app.NewOrdersService(st, clock)
	cajero := makeUser(t, st, "cajero_029_centavo", "gerente")
	efectivo := paymentMethodID(t, st, "Efectivo")
	abrirCajaPrincipal(t, st, cajero)
	prod := makeProduct(t, st, "Frappe 029", decimal.RequireFromString("100"), false)
	ord := crearPedidoSimple(t, ctx, orders, prod, cajero)

	_, err := orders.Charge(ctx, app.ChargeCmd{OrderID: ord, MethodID: efectivo, Amount: decimal.RequireFromString("0.005"), ActorID: cajero})
	if !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("cobrar 0.005: err = %v, quiere validación", err)
	}
	v, err := orders.Detail(ctx, ord)
	if err != nil {
		t.Fatal(err)
	}
	if !v.Outstanding.Equal(decimal.RequireFromString("100")) {
		t.Fatalf("por cobrar = %s, quiere 100: el medio centavo entró como pago", v.Outstanding)
	}
}

// filtroDelDia: la pantalla de Ventas mirando un rango de días, sin otros filtros.
func filtroDelDia(desde, hasta time.Time) domain.SalesFilter {
	return domain.SalesFilter{Range: domain.Range{From: desde, To: hasta}, Sort: "fecha", Dir: "desc", Limit: 50}
}

func diaDe(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

// EL TOTAL DE VENTAS ES LO QUE SE PUEDE FACTURAR (spec 029, decisión del dueño).
//
// Medido en el ambiente de pruebas: el Total sumaba completo un pedido ya devuelto y $360 de dos
// pedidos sin cobrar. El número con el que se factura estaba inflado por los dos lados.
func TestSalesTotalIsCollectedNetOfRefunds(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()
	orders := app.NewOrdersService(st, clock)
	sales := app.NewSalesService(st, clock)
	cajero := makeUser(t, st, "cajero_029_total", "gerente")
	efectivo := paymentMethodID(t, st, "Efectivo")
	tarjeta := paymentMethodID(t, st, "Tarjeta débito")
	abrirCajaPrincipal(t, st, cajero)

	pedidoCobradoParcial(t, ctx, st, orders, "029-cobrado", "100", "100", cajero, efectivo, false)
	devuelto := pedidoCobradoParcial(t, ctx, st, orders, "029-devuelto", "92", "92", cajero, tarjeta, false)
	refund(t, ctx, orders, devuelto, nil, "92", cajero)
	crearPedidoSimple(t, ctx, orders, makeProduct(t, st, "029 abierto", decimal.RequireFromString("180"), false), cajero)

	// Cancelado con devolución que tenía propina: la propina devuelta no entra al Total (la propina
	// nunca entra) y se nombra aparte en su medio.
	conPropina := crearPedidoSimple(t, ctx, orders, makeProduct(t, st, "029 propina", decimal.RequireFromString("50"), false), cajero)
	if _, err := orders.Charge(ctx, app.ChargeCmd{OrderID: conPropina, MethodID: efectivo,
		Amount: decimal.RequireFromString("50"), Tip: decimal.RequireFromString("5"), ActorID: cajero}); err != nil {
		t.Fatalf("Charge con propina: %v", err)
	}
	if err := orders.CancelarConDevolucion(ctx, app.CancelacionCmd{OrderID: conPropina, Motivo: "prueba", ActorID: cajero, Devolver: true}); err != nil {
		t.Fatalf("CancelarConDevolucion: %v", err)
	}

	hoy := diaDe(domain.BusinessDate(fixedNow, sales.Location(ctx)))
	r, err := sales.Summary(ctx, filtroDelDia(hoy, hoy))
	if err != nil {
		t.Fatalf("Summary: %v", err)
	}
	if !r.Total.Equal(decimal.RequireFromString("100")) {
		t.Fatalf("Total = %s, quiere 100: se coló lo devuelto, lo abierto o la propina", r.Total)
	}
	suma := decimal.Zero
	var propinaDevuelta decimal.Decimal
	for _, m := range r.ByMethod {
		suma = suma.Add(m.Total)
		if m.MethodID == efectivo {
			propinaDevuelta = m.TipRefunds
		}
	}
	if !suma.Equal(r.Total) {
		t.Fatalf("Σ medios = %s y Total = %s: la pantalla no cuadra con lo que pinta debajo", suma, r.Total)
	}
	if !propinaDevuelta.Equal(decimal.RequireFromString("5")) {
		t.Fatalf("propina devuelta del efectivo = %s, quiere 5", propinaDevuelta)
	}
	if r.Pending.Count != 1 || !r.Pending.Amount.Equal(decimal.RequireFromString("180")) {
		t.Fatalf("por cobrar = %+v, quiere 1 pedido por 180", r.Pending)
	}
}

// UNA DEVOLUCIÓN PEGA EN EL MES EN QUE SE DEVOLVIÓ (decisión del dueño). El mes cerrado no cambia.
func TestARefundInOctoberDoesNotChangeSeptember(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()
	junio := time.Date(2026, 6, 30, 18, 0, 0, 0, time.UTC)
	ordersJunio := app.NewOrdersService(st, clockAt(junio))
	orders := app.NewOrdersService(st, clock)
	sales := app.NewSalesService(st, clock)
	cajero := makeUser(t, st, "cajero_029_meses", "gerente")
	tarjeta := paymentMethodID(t, st, "Tarjeta débito")
	abrirCajaPrincipal(t, st, cajero)

	ord := crearPedidoSimple(t, ctx, ordersJunio, makeProduct(t, st, "029 meses", decimal.RequireFromString("300"), false), cajero)
	charge(t, ctx, ordersJunio, ord, tarjeta, "300", cajero)
	refund(t, ctx, orders, ord, nil, "120", cajero)

	loc := sales.Location(ctx)
	diaJunio := diaDe(domain.BusinessDate(junio, loc))
	diaJulio := diaDe(domain.BusinessDate(fixedNow, loc))
	enJunio, err := sales.Summary(ctx, filtroDelDia(time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC), diaJunio))
	if err != nil {
		t.Fatal(err)
	}
	enJulio, err := sales.Summary(ctx, filtroDelDia(time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC), diaJulio))
	if err != nil {
		t.Fatal(err)
	}
	if !enJunio.Total.Equal(decimal.RequireFromString("300")) {
		t.Fatalf("junio = %s, quiere 300: la devolución de julio reescribió junio", enJunio.Total)
	}
	if !enJulio.Total.Equal(decimal.RequireFromString("-120")) {
		t.Fatalf("julio = %s, quiere -120: la devolución pega en el mes en que se devolvió", enJulio.Total)
	}
}

// POR COBRAR NO ALCANZA PEDIDOS DE OTRA EMPRESA.
func TestSalesPendingIsIsolated(t *testing.T) {
	t.Parallel()
	owner := newTestStore(t)
	ctx := context.Background()
	orders := app.NewOrdersService(owner, clock)
	other := makeCompany(t, owner, "otra-029-pendiente")
	cajero := makeUser(t, owner, "cajero_029_aislado", "gerente")
	abrirCajaPrincipal(t, owner, cajero)
	crearPedidoSimple(t, ctx, orders, makeProduct(t, owner, "029 aislado", decimal.RequireFromString("80"), false), cajero)
	dia := pgtype.Date{Time: domain.BusinessDate(fixedNow, nil), Valid: true}

	inTheThreeCases(t, defaultCompanyID, other, func(t *testing.T, st *store.Store, ctx context.Context) {
		q := st.QC(ctx)
		if r, err := q.SalesPending(ctx, db.SalesPendingParams{Desde: dia, Hasta: dia}); err == nil && r.Pedidos != 0 {
			t.Fatalf("SalesPending vio %d pedidos de la dueña", r.Pedidos)
		}
		if r, err := q.SalesPendingSinFolio(ctx, db.SalesPendingSinFolioParams{Desde: dia, Hasta: dia}); err == nil && r.Pedidos != 0 {
			t.Fatalf("SalesPendingSinFolio vio %d pedidos de la dueña", r.Pedidos)
		}
	})
}

// UNA DEVOLUCIÓN SE VE EN LA LISTA SIN ABRIR EL PEDIDO (US2).
//
// La lista trae lo cobrado y cuándo fue la última devolución. Un pedido sin devoluciones manda
// `null`, no una fecha cero: el JSON crudo es lo que lee la pantalla.
func TestTheSalesListCarriesPaidAndLastRefund(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()
	orders := app.NewOrdersService(st, clock)
	sales := app.NewSalesService(st, clock)
	cajero := makeUser(t, st, "cajero_029_lista", "gerente")
	efectivo := paymentMethodID(t, st, "Efectivo")
	abrirCajaPrincipal(t, st, cajero)

	devuelto := pedidoCobradoParcial(t, ctx, st, orders, "029-lista-dev", "92", "92", cajero, efectivo, false)
	refund(t, ctx, orders, devuelto, nil, "30.67", cajero)
	debe := pedidoCobradoParcial(t, ctx, st, orders, "029-lista-debe", "100", "40", cajero, efectivo, true)

	hoy := diaDe(domain.BusinessDate(fixedNow, sales.Location(ctx)))
	page, err := sales.List(ctx, filtroDelDia(hoy, hoy))
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	vistos := 0
	for _, r := range page.Items {
		switch r.ID {
		case devuelto:
			vistos++
			if r.LastRefundAt == nil || !r.Refund.Equal(decimal.RequireFromString("30.67")) || !r.Paid.Equal(decimal.RequireFromString("92")) {
				t.Fatalf("pedido devuelto: refund %s, paid %s, lastRefundAt %v", r.Refund, r.Paid, r.LastRefundAt)
			}
		case debe:
			vistos++
			if !r.Paid.Equal(decimal.RequireFromString("40")) {
				t.Fatalf("pedido con saldo: paid = %s, quiere 40", r.Paid)
			}
			crudo, err := json.Marshal(r)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(crudo), `"lastRefundAt":null`) {
				t.Fatalf("sin devoluciones el JSON debe decir null: %s", crudo)
			}
		}
	}
	if vistos != 2 {
		t.Fatalf("la lista trajo %d de los 2 pedidos", vistos)
	}
}
