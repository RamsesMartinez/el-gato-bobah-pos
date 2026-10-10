//go:build integration

package integration

import (
	"context"
	"testing"

	"github.com/shopspring/decimal"
	"uuid"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
)

// UNA VENTA REEMBOLSADA NO PUEDE SALIR EN DOS TILES DE LA MISMA PANTALLA.
//
// El resumen de Ventas pinta "Total", "Reembolsadas" y el desglose por medio de pago uno al lado del
// otro. `SalesTotalsByMethod` no filtraba por estado, así que los $500 de una venta devuelta salían
// en "Reembolsadas" Y en "Tarjeta", mientras "Total" —que sí las excluye— los ignoraba. Tres
// renglones hermanos con el mismo dinero contado de tres maneras.
//
// Es EL HERMANO QUE NO SE MOVIÓ: el mismo defecto se corrigió en `SalesByMethod` de reports.sql y la
// copia de sales.sql se quedó como estaba. El test que debía atraparlo cancelaba una venta SIN
// pagos, así que la aserción pasaba sin tocar el caso.
func TestElDesgloseDeMetodosNoCuentaLoReembolsado(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()
	ordenes := app.NewOrdersService(st, clock)

	cajero := makeUser(t, st, "cajero_dup_metodo", "cajero")
	efectivo := paymentMethodID(t, st, "Efectivo")
	abrirCajaPrincipal(t, st, cajero)

	buena := makeProduct(t, st, "Buena dup", decimal.RequireFromString("100"), false)
	mala := makeProduct(t, st, "Devuelta dup", decimal.RequireFromString("500"), false)
	// Sin paso por cocina el pedido se cierra solo al quedar saldado, que es lo que `Refund` exige:
	// solo se devuelve lo que ya se entregó.
	if _, err := st.Pool.Exec(ctx,
		"update products set needs_prep = false where id in ($1, $2)", buena, mala); err != nil {
		t.Fatalf("quitar needs_prep: %v", err)
	}

	if _, err := crearYCobrar(t, ctx, ordenes, app.CreateOrderCmd{
		ClientUUID: uuid.New(), ServiceType: "mostrador", OpenedBy: cajero,
		Lines:    []domain.OrderLineInput{{ProductID: buena, Qty: decimal.RequireFromString("1")}},
		Payments: []app.PaymentInput{{MethodID: efectivo, Amount: decimal.RequireFromString("100")}},
	}); err != nil {
		t.Fatalf("venta buena: %v", err)
	}

	// La que se devuelve: se cobra de verdad —ahí está el defecto— y luego se reembolsa.
	devuelta, err := crearYCobrar(t, ctx, ordenes, app.CreateOrderCmd{
		ClientUUID: uuid.New(), ServiceType: "mostrador", OpenedBy: cajero,
		Lines:    []domain.OrderLineInput{{ProductID: mala, Qty: decimal.RequireFromString("1")}},
		Payments: []app.PaymentInput{{MethodID: efectivo, Amount: decimal.RequireFromString("500")}},
	})
	if err != nil {
		t.Fatalf("venta a devolver: %v", err)
	}
	if err := ordenes.Refund(ctx, devuelta.ID, cajero, "el cliente la devolvió"); err != nil {
		t.Fatalf("Refund: %v", err)
	}

	sum, err := app.NewSalesService(st, clock).Summary(ctx, filtroDePrueba())
	if err != nil {
		t.Fatalf("resumen: %v", err)
	}

	// El flujo viejo de `Refund` no deja filas en order_refunds: sus cobros no entran al desglose,
	// así que tampoco hay devolución que el tile pueda describir. Desde el 2026-10-09 el tile sale de
	// las devoluciones que el desglose restó, no del estado del pedido.
	if !sum.Refunded.Amount.IsZero() {
		t.Fatalf("devoluciones = %s, quiere 0: el desglose no restó nada de este pedido", sum.Refunded.Amount)
	}
	cobrado := decimal.Zero
	for _, m := range sum.ByMethod {
		cobrado = cobrado.Add(m.Total)
	}
	if !cobrado.Equal(decimal.RequireFromString("100")) {
		t.Fatalf("los métodos suman %s y el total de la pantalla es %s: los %s de diferencia son la "+
			"venta reembolsada, que el tile de reembolsos ya cuenta — el mismo peso en dos renglones",
			cobrado, sum.Total, cobrado.Sub(sum.Total))
	}
}

// UNA DEVOLUCIÓN PARCIAL DE HOY SALE EN EL TILE DE DEVOLUCIONES, NO SOLO EN EL DESGLOSE.
//
// `GET /sales/summary?preset=hoy` devolvía `refunded` {0, 0} mientras `byMethod` ya restaba la
// devolución: el tile solo contaba pedidos del periodo en estado «reembolsada», y una devolución
// parcial deja el pedido entregado. La lista y el resumen dejaban de describir el mismo dinero.
func TestAPartialRefundTodayShowsInTheRefundedTile(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()
	ordenes := app.NewOrdersService(st, clock)

	cajero := makeUser(t, st, "cajero_tile_dev", "cajero")
	efectivo := paymentMethodID(t, st, "Efectivo")
	abrirCajaPrincipal(t, st, cajero)

	ord := pedidoCobradoParcial(t, ctx, st, ordenes, "tile_dev", "200", "200", cajero, efectivo, false)
	if err := ordenes.Devolver(ctx, app.DevolucionCmd{CardFolio: "F-1",
		OrderID: ord, Monto: decimal.RequireFromString("30"),
		Motivo: "faltó un topping", ActorID: cajero,
	}); err != nil {
		t.Fatalf("devolver 30: %v", err)
	}

	sum, err := app.NewSalesService(st, clock).Summary(ctx, filtroDePrueba())
	if err != nil {
		t.Fatalf("resumen: %v", err)
	}
	restado := decimal.Zero
	for _, m := range sum.ByMethod {
		restado = restado.Add(m.Refunds)
	}
	if !restado.Equal(decimal.RequireFromString("30")) {
		t.Fatalf("el desglose restó %s, quiere 30", restado)
	}
	if sum.Refunded.Count != 1 || !sum.Refunded.Amount.Equal(restado) {
		t.Fatalf("tile de devoluciones = %+v y el desglose restó %s: el resumen no describe el mismo dinero",
			sum.Refunded, restado)
	}
}

// LAS PROPINAS SE REPARTEN POR PERSONA, NO POR NOMBRE.
//
// El reporte agrupaba por `u.name`. Dos empleados que se llamen igual —"Ana" y "Ana"— salían en un
// solo renglón con la suma de los dos, y no hay forma de repartir un renglón así: quien lo lee no
// sabe cuánto le toca a cada una. Es el único reporte que existe para entregar dinero.
func TestLasPropinasNoSeFusionanPorHomonimia(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()
	ordenes := app.NewOrdersService(st, clock)
	backoffice := app.NewBackofficeService(st, clock)

	hoy := domain.BusinessDate(fixedNow, domain.LoadBusinessLocation(domain.DefaultTimezone))
	efectivo := paymentMethodID(t, st, "Efectivo")
	// Dos usuarios distintos con el MISMO nombre, que es lo que el reporte tiene que distinguir.
	unaAna := makeUser(t, st, "ana_turno_a", "cajero")
	otraAna := makeUser(t, st, "ana_turno_b", "cajero")
	renombrar(t, st, unaAna, "Ana")
	renombrar(t, st, otraAna, "Ana")
	abrirCajaPrincipal(t, st, unaAna)

	cobrarConPropina(t, ctx, st, ordenes, "prop_a", "100", "30", unaAna, efectivo)
	cobrarConPropina(t, ctx, st, ordenes, "prop_b", "100", "70", otraAna, efectivo)

	filas, err := backoffice.TipsByEmployee(ctx, hoy, hoy)
	if err != nil {
		t.Fatalf("TipsByEmployee: %v", err)
	}
	var anas int
	for _, f := range filas {
		if f.Employee == "Ana" {
			anas++
		}
	}
	if anas != 2 {
		t.Fatalf("las dos Anas salieron en %d renglón(es): con uno solo no hay forma de repartir "+
			"los $100 de propina entre las dos personas que los cobraron", anas)
	}
}

func renombrar(t *testing.T, st *store.Store, id int64, nombre string) {
	t.Helper()
	if _, err := st.Pool.Exec(context.Background(),
		"update users set name = $2 where id = $1", id, nombre); err != nil {
		t.Fatalf("renombrar usuario: %v", err)
	}
}

func cobrarConPropina(t *testing.T, ctx context.Context, st *store.Store, svc *app.OrdersService,
	sufijo, monto, propina string, cajero int64, metodo int16,
) {
	t.Helper()
	prod := makeProduct(t, st, "Propina "+sufijo, decimal.RequireFromString(monto), false)
	if _, err := st.Pool.Exec(ctx, "update products set needs_prep = false where id = $1", prod); err != nil {
		t.Fatalf("quitar needs_prep: %v", err)
	}
	ord, err := svc.Create(ctx, app.CreateOrderCmd{
		ClientUUID: uuid.New(), ServiceType: "mostrador", OpenedBy: cajero,
		Lines: []domain.OrderLineInput{{ProductID: prod, Qty: decimal.RequireFromString("1")}},
	})
	if err != nil {
		t.Fatalf("Create(%s): %v", sufijo, err)
	}
	if _, err := svc.Charge(ctx, app.ChargeCmd{
		OrderID: ord.ID, MethodID: metodo,
		Amount: decimal.RequireFromString(monto), Tip: decimal.RequireFromString(propina),
		ActorID: cajero,
	}); err != nil {
		t.Fatalf("Charge(%s): %v", sufijo, err)
	}
}
