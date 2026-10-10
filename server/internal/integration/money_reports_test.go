//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"uuid"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store/db"
)

// Los reportes de la spec 031: que digan lo mismo que la venta, y que cada peso cuente en su día.

func clockAt(t time.Time) func() time.Time { return func() time.Time { return t } }

// D10: UTILIDAD POR PRODUCTO NO CUENTA LO QUITADO Y RESTA EL DESCUENTO.
//
// Dos frappés de $60, se quita uno y se descuentan $20: la venta es $40 y Utilidad decía 2 piezas
// y $120.
func TestProductMarginsSkipRemovedLinesAndSubtractTheDiscount(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()
	orders := app.NewOrdersService(st, clock)
	back := app.NewBackofficeService(st, clock)

	cajero := makeUser(t, st, "cajero_d10", "gerente")
	abrirCajaPrincipal(t, st, cajero)
	prod := makeProduct(t, st, "Frappé d10", dec("60"), false)
	ord, err := orders.Create(ctx, app.CreateOrderCmd{
		ClientUUID: uuid.New(), ServiceType: "mostrador", OpenedBy: cajero,
		Lines: []domain.OrderLineInput{{ProductID: prod, Qty: dec("2")}},
	})
	if err != nil {
		t.Fatal(err)
	}
	var line int64
	if err := st.Pool.QueryRow(ctx, `select id from order_lines where order_id = $1`, ord.ID).Scan(&line); err != nil {
		t.Fatal(err)
	}
	one := dec("1")
	if _, err := orders.RemovePieces(ctx, ord.ID, line, cajero, "se arrepintió", &one); err != nil {
		t.Fatal(err)
	}
	amount := dec("20")
	if _, err := orders.SetDiscount(ctx, app.SetDiscountCmd{OrderID: ord.ID, Amount: &amount, Actor: cajero}); err != nil {
		t.Fatal(err)
	}

	hoy := domain.BusinessDate(fixedNow, domain.LoadBusinessLocation(domain.DefaultTimezone))
	rows, err := back.ProductMargins(ctx, hoy, hoy, 50)
	if err != nil {
		t.Fatal(err)
	}
	var got *db.ProductMarginsRow
	for i := range rows {
		if rows[i].ProductName == "Frappé d10" {
			got = &rows[i]
		}
	}
	if got == nil {
		t.Fatal("el frappé no aparece en Utilidad por producto")
	}
	if !got.Qty.Equal(dec("1")) || !got.Revenue.Equal(dec("40")) {
		t.Fatalf("Utilidad dice %s piezas y %s de ingreso, quiere 1 y 40: lo quitado no se vendió y el descuento sí se dio",
			got.Qty, got.Revenue)
	}
}

// D14: LO QUITADO DE UN PEDIDO QUE DESPUÉS SE CANCELA SIGUE EN «RENGLONES CANCELADOS».
//
// Tres frappés quitados y el pedido cerrado sin productos: «Canceladas» decía $0 y «Renglones
// cancelados» 0 — los $180 desaparecían de un día pasado.
func TestRemovedLinesOfALaterCancelledOrderStillCount(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()
	orders := app.NewOrdersService(st, clock)

	cajero := makeUser(t, st, "cajero_d14", "gerente")
	abrirCajaPrincipal(t, st, cajero)
	prod := makeProduct(t, st, "Frappé d14", dec("60"), false)
	ord, err := orders.Create(ctx, app.CreateOrderCmd{
		ClientUUID: uuid.New(), ServiceType: "mostrador", OpenedBy: cajero,
		Lines: []domain.OrderLineInput{{ProductID: prod, Qty: dec("3")}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := orders.CancelPending(ctx, ord.ID, cajero, "se fue"); err != nil {
		t.Fatalf("quitar lo que falta: %v", err)
	}
	// Si quitar lo último no lo cerró, «Cerrar pedido» lo cancela sin productos.
	var still string
	if err := st.Pool.QueryRow(ctx, `select status from orders where id = $1`, ord.ID).Scan(&still); err != nil {
		t.Fatal(err)
	}
	if still != "cancelada" {
		if _, err := orders.CancelPending(ctx, ord.ID, cajero, ""); err != nil {
			t.Fatalf("cerrar el pedido sin productos: %v", err)
		}
	}
	var status string
	if err := st.Pool.QueryRow(ctx, `select status from orders where id = $1`, ord.ID).Scan(&status); err != nil || status != "cancelada" {
		t.Fatalf("el pedido quedó %q (%v), quiere cancelada", status, err)
	}

	sum, err := app.NewSalesService(st, clock).Summary(ctx, filtroDePrueba())
	if err != nil {
		t.Fatal(err)
	}
	if !sum.CancelledLines.Amount.Equal(dec("180")) {
		t.Fatalf("renglones cancelados = %s, quiere 180: lo quitado se canceló de verdad aunque el pedido se cerrara después",
			sum.CancelledLines.Amount)
	}
}

// D12 + decisiones del dueño: UN COBRO CUENTA EL DÍA EN QUE SE COBRÓ, Y UNA DEVOLUCIÓN EL DÍA EN
// QUE SE DEVOLVIÓ, en Ventas por método y en el reporte de cobros por método.
//
// Un pedido de un turno cerrado cobrado «por monto» dos días después entraba al corte de ese día
// pero Ventas lo ponía en el día viejo; y un cobro devuelto el mes siguiente desaparecía del mes en
// que entró.
func TestEachPaymentAndRefundCountsOnItsOwnDay(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()
	loc := domain.LoadBusinessLocation(domain.DefaultTimezone)
	day1 := fixedNow
	day3 := fixedNow.Add(48 * time.Hour)
	d1, d3 := domain.BusinessDate(day1, loc), domain.BusinessDate(day3, loc)
	ordersDay1 := app.NewOrdersService(st, clockAt(day1))
	ordersDay3 := app.NewOrdersService(st, clockAt(day3))

	cajero := makeUser(t, st, "cajero_d12r", "gerente")
	efectivo := paymentMethodID(t, st, "Efectivo")
	tarjeta := paymentMethodID(t, st, "Transferencia SPEI")
	abrirCajaPrincipal(t, st, cajero)

	// Fiado el día 1, cobrado el día 3.
	prod := makeProduct(t, st, "Fiado d12r", dec("450"), false)
	fiado := crearPedidoSimple(t, ctx, ordersDay1, prod, cajero)
	if _, err := st.Pool.Exec(ctx, `update orders set status = 'entregada', completed_at = now() where id = $1`, fiado); err != nil {
		t.Fatal(err)
	}
	charge(t, ctx, ordersDay3, fiado, efectivo, "450", cajero)

	// Cobrado el día 1 por transferencia y devuelto el día 3.
	otro := pedidoCobradoParcial(t, ctx, st, ordersDay1, "d12r", "300", "300", cajero, tarjeta, false)
	refund(t, ctx, ordersDay3, otro, nil, "300", cajero)

	back := app.NewBackofficeService(st, clock)
	byMethod := func(d time.Time) map[string]db.SalesByMethodRow {
		rows, err := back.SalesByMethod(ctx, d, d)
		if err != nil {
			t.Fatal(err)
		}
		out := map[string]db.SalesByMethodRow{}
		for _, r := range rows {
			out[r.Method] = r
		}
		return out
	}
	first, third := byMethod(d1), byMethod(d3)
	if r := first["Efectivo"]; !r.Total.IsZero() {
		t.Fatalf("el día 1 tiene %s en efectivo: el fiado se cobró el día 3", r.Total)
	}
	if r := third["Efectivo"]; !r.Total.Equal(dec("450")) {
		t.Fatalf("el día 3 tiene %s en efectivo, quiere 450: ese día entró el dinero", r.Total)
	}
	if r := first["Transferencia SPEI"]; !r.Total.Equal(dec("300")) {
		t.Fatalf("el día 1 tiene %s por transferencia, quiere 300: la devolución del día 3 no reescribe el día 1", r.Total)
	}
	if r, ok := third["Transferencia SPEI"]; !ok || !r.Total.Equal(dec("-300")) || !r.Refunds.Equal(dec("300")) {
		t.Fatalf("el día 3 por transferencia = %+v (%v), quiere total -300 con 300 devueltos", r, ok)
	}

	// La pantalla de Ventas dice lo mismo.
	summary := func(d time.Time) map[string]app.MethodTotals {
		f := filtroDePrueba()
		f.Range = domain.Range{From: d, To: d}
		s, err := app.NewSalesService(st, clockAt(day3)).Summary(ctx, f)
		if err != nil {
			t.Fatal(err)
		}
		out := map[string]app.MethodTotals{}
		for _, m := range s.ByMethod {
			out[m.Method] = m
		}
		return out
	}
	if m := summary(d3)["Transferencia SPEI"]; !m.Total.Equal(dec("-300")) || !m.Refunds.Equal(dec("300")) {
		t.Fatalf("Ventas del día 3 por transferencia = %+v, quiere total -300 con 300 devueltos", m)
	}
	if m := summary(d3)["Efectivo"]; !m.Total.Equal(dec("450")) {
		t.Fatalf("Ventas del día 3 en efectivo = %s, quiere 450", m.Total)
	}
}

// DD-6: UN CANCELADO ANTERIOR AL LIBRO DE DEVOLUCIONES, CON COBROS Y SIN DEVOLUCIÓN, SIGUE FUERA.
//
// Antes de 0060 cancelar no miraba los cobros. Esos pedidos tienen pagos y ninguna devolución en el
// libro (medido: cinco en la empresa real); contarlos ahora inflaría meses ya cerrados.
func TestALegacyCancelledOrderWithoutLedgerStaysOut(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()
	orders := app.NewOrdersService(st, clock)

	cajero := makeUser(t, st, "cajero_dd6", "gerente")
	efectivo := paymentMethodID(t, st, "Efectivo")
	abrirCajaPrincipal(t, st, cajero)
	ord := pedidoCobradoParcial(t, ctx, st, orders, "dd6", "159", "159", cajero, efectivo, true)
	if _, err := st.Pool.Exec(ctx, `update orders set status = 'cancelada', cancelled_at = now(), cancelled_by = opened_by, cancel_reason = 'antes de 0060' where id = $1`, ord); err != nil {
		t.Fatal(err)
	}
	rows, err := app.NewBackofficeService(st, clock).SalesByMethod(ctx, domain.BusinessDate(fixedNow, nil), domain.BusinessDate(fixedNow, nil))
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rows {
		if !r.Total.IsZero() {
			t.Fatalf("%s suma %s de un cancelado sin devolución registrada", r.Method, r.Total)
		}
	}
}

// LAS CONSULTAS DE REPORTE CAMBIADAS NO ALCANZAN DINERO DE OTRA EMPRESA.
func TestTheChangedReportQueriesAreIsolated(t *testing.T) {
	t.Parallel()
	owner := newTestStore(t)
	ctx := context.Background()
	orders := app.NewOrdersService(owner, clock)
	other := makeCompany(t, owner, "otra-reportes-031")
	cajero := makeUser(t, owner, "cajero_rep_031", "gerente")
	efectivo := paymentMethodID(t, owner, "Efectivo")
	abrirCajaPrincipal(t, owner, cajero)
	ord := pedidoCobradoParcial(t, ctx, owner, orders, "rep_031", "70", "70", cajero, efectivo, false)
	refund(t, ctx, orders, ord, nil, "10", cajero)
	d := domain.BusinessDate(fixedNow, nil)
	desde := pgtype.Date{Time: d, Valid: true}

	inTheThreeCases(t, defaultCompanyID, other, func(t *testing.T, st *store.Store, ctx context.Context) {
		q := st.QC(ctx)
		if rows, err := q.SalesByMethod(ctx, db.SalesByMethodParams{Desde: desde, Hasta: desde}); err == nil && len(rows) != 0 {
			t.Fatalf("SalesByMethod vio %d métodos de la dueña", len(rows))
		}
		if rows, err := q.SalesTotalsByMethod(ctx, db.SalesTotalsByMethodParams{Desde: desde, Hasta: desde}); err == nil && len(rows) != 0 {
			t.Fatalf("SalesTotalsByMethod vio %d métodos de la dueña", len(rows))
		}
		if rows, err := q.ProductMargins(ctx, db.ProductMarginsParams{BusinessDate: desde, BusinessDate_2: desde, Limit: 50}); err == nil && len(rows) != 0 {
			t.Fatalf("ProductMargins vio %d productos de la dueña", len(rows))
		}
		if r, err := q.SalesCancelledLines(ctx, db.SalesCancelledLinesParams{Desde: desde, Hasta: desde}); err == nil && r.Lineas != 0 {
			t.Fatalf("SalesCancelledLines vio %d renglones de la dueña", r.Lineas)
		}
	})
}

// UN PRODUCTO SIN COSTO CAPTURADO NO TIENE MARGEN = VENTA (spec 029).
//
// Utilidad por producto restaba un costo de $0 y mostraba como margen la venta entera: el producto
// sin costo capturado parecía el más rentable de la carta. Su venta va aparte y no suma al margen.
func TestProductMarginsDoNotCountUncostedSalesAsMargin(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()
	orders := app.NewOrdersService(st, clock)
	back := app.NewBackofficeService(st, clock)
	cajero := makeUser(t, st, "cajero_029_costo", "gerente")
	abrirCajaPrincipal(t, st, cajero)

	sinCosto := makeProduct(t, st, "Sin costo 029", dec("80"), false)
	conCosto := makeProduct(t, st, "Con costo 029", dec("50"), false)
	if _, err := st.Pool.Exec(ctx, `update products set current_cost = 20 where id = $1`, conCosto); err != nil {
		t.Fatal(err)
	}
	for _, p := range []int64{sinCosto, conCosto} {
		if _, err := orders.Create(ctx, app.CreateOrderCmd{
			ClientUUID: uuid.New(), ServiceType: "mostrador", OpenedBy: cajero,
			Lines: []domain.OrderLineInput{{ProductID: p, Qty: dec("1")}},
		}); err != nil {
			t.Fatal(err)
		}
	}

	hoy := domain.BusinessDate(fixedNow, domain.LoadBusinessLocation(domain.DefaultTimezone))
	rows, err := back.ProductMargins(ctx, hoy, hoy, 50)
	if err != nil {
		t.Fatal(err)
	}
	vistos := 0
	for _, r := range rows {
		switch r.ProductName {
		case "Sin costo 029":
			vistos++
			if !r.Margin.IsZero() || !r.UncostedRevenue.Equal(dec("80")) {
				t.Fatalf("sin costo: margen %s y venta sin costo %s; quiere 0 y 80", r.Margin, r.UncostedRevenue)
			}
		case "Con costo 029":
			vistos++
			if !r.Margin.Equal(dec("30")) || !r.UncostedRevenue.IsZero() {
				t.Fatalf("con costo: margen %s y venta sin costo %s; quiere 30 y 0", r.Margin, r.UncostedRevenue)
			}
		}
	}
	if vistos != 2 {
		t.Fatalf("Utilidad trajo %d de los 2 productos", vistos)
	}
}
