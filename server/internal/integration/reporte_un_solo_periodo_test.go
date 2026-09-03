//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"uuid"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store/db"
)

// LAS DOS TABLAS DE LA PANTALLA DE REPORTES RESPONDEN EL MISMO PERIODO.
//
// "Venta por día" y "Por medio de pago" se leen una junto a otra, y la segunda se usa para cuadrar
// la primera. SalesByMethod filtraba por op.created_at >= $1 —sin cota superior y sobre el instante
// del cobro, no sobre el día de negocio—, así que al elegir un día mostraba ese día arriba y "de ese
// día en adelante" abajo. Quien lo lee no tiene forma de saber cuál de las dos cifras es la del
// periodo que pidió: es la forma de mentir que nombra el principio III, la lista y el resumen de una
// misma pantalla derivados de predicados distintos.
//
// De integración y no unitario a propósito: el defecto vive en el where de la consulta y no hay
// función a la que llamar para verlo.
func TestElReporteDeVentasNoMezclaDosPeriodos(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	ordenes := app.NewOrdersService(st, clock)
	backoffice := app.NewBackofficeService(st, clock)

	hoy := domain.BusinessDate(fixedNow, domain.LoadBusinessLocation(domain.DefaultTimezone))
	efectivo := paymentMethodID(t, st, "Efectivo")
	cajero := makeUser(t, st, "cajero_reporte_periodo", "cajero")
	abrirCajaPrincipal(t, st, cajero)

	ventaCobrada(t, ctx, st, ordenes, "hoy_periodo", "100", cajero, efectivo)
	// La segunda venta se mueve a OTRO día de negocio. Es lo que separa las dos consultas: con la
	// cota superior puesta, el reporte de hoy no la ve.
	otroDia := ventaCobrada(t, ctx, st, ordenes, "otro_dia_periodo", "700", cajero, efectivo)
	moverAlDia(t, st, otroDia, hoy.AddDate(0, 0, 5))

	porDia, err := backoffice.SalesByDay(ctx, hoy, hoy)
	if err != nil {
		t.Fatalf("SalesByDay: %v", err)
	}
	porMetodo, err := backoffice.SalesByMethod(ctx, hoy, hoy)
	if err != nil {
		t.Fatalf("SalesByMethod: %v", err)
	}

	venta := sumaDeDias(porDia)
	cobrado := sumaDeMetodos(porMetodo)
	if !venta.Equal(cobrado) {
		t.Fatalf("la venta del día es %s y los métodos suman %s: las dos tablas de la misma pantalla "+
			"están contestando periodos distintos", venta, cobrado)
	}
	if !venta.Equal(decimal.RequireFromString("100")) {
		t.Fatalf("la venta del día es %s, quiere 100: se coló la venta de otro día", venta)
	}
}

// UNA VENTA REEMBOLSADA NO ES INGRESO, Y NO PUEDE SERLO EN UNA TABLA Y NO EN LA OTRA.
//
// SalesByDay excluye canceladas y reembolsadas; SalesByMethod no las excluía. El cobro de una venta
// devuelta seguía sumando abajo mientras el total de arriba no lo contaba: el mismo peso clasificado
// de dos maneras en la misma pantalla, que es cómo se reporta dinero que el negocio no tuvo.
func TestUnaVentaReembolsadaNoSumaEnLosMetodosDePago(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	ordenes := app.NewOrdersService(st, clock)
	backoffice := app.NewBackofficeService(st, clock)

	hoy := domain.BusinessDate(fixedNow, domain.LoadBusinessLocation(domain.DefaultTimezone))
	efectivo := paymentMethodID(t, st, "Efectivo")
	cajero := makeUser(t, st, "cajero_reporte_reembolso", "cajero")
	abrirCajaPrincipal(t, st, cajero)

	ventaCobrada(t, ctx, st, ordenes, "buena_reembolso", "100", cajero, efectivo)
	devuelta := ventaCobrada(t, ctx, st, ordenes, "mala_reembolso", "450", cajero, efectivo)
	if err := ordenes.Refund(ctx, devuelta, cajero, "el cliente la devolvió"); err != nil {
		t.Fatalf("Refund: %v", err)
	}

	porDia, err := backoffice.SalesByDay(ctx, hoy, hoy)
	if err != nil {
		t.Fatalf("SalesByDay: %v", err)
	}
	porMetodo, err := backoffice.SalesByMethod(ctx, hoy, hoy)
	if err != nil {
		t.Fatalf("SalesByMethod: %v", err)
	}

	venta := sumaDeDias(porDia)
	cobrado := sumaDeMetodos(porMetodo)
	if !cobrado.Equal(venta) {
		t.Fatalf("los métodos suman %s y la venta del día %s: la diferencia de %s es la venta "+
			"reembolsada, que una tabla cuenta y la otra no", cobrado, venta, cobrado.Sub(venta))
	}
	if !venta.Equal(decimal.RequireFromString("100")) {
		t.Fatalf("la venta del día es %s, quiere 100", venta)
	}
}

// LA UTILIDAD POR PRODUCTO MIRA EL MISMO PERIODO QUE EL RESTO DE LA PANTALLA.
//
// ProductMargins filtraba por o.opened_at >= $1: sin cota superior —así que con un filtro de fechas
// encima habría seguido contestando "de esa fecha a hoy"— y sobre un instante en UTC en vez del día
// de negocio con el que el local cuadra su caja.
func TestLaUtilidadPorProductoRespetaElRango(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	ordenes := app.NewOrdersService(st, clock)
	backoffice := app.NewBackofficeService(st, clock)

	hoy := domain.BusinessDate(fixedNow, domain.LoadBusinessLocation(domain.DefaultTimezone))
	efectivo := paymentMethodID(t, st, "Efectivo")
	cajero := makeUser(t, st, "cajero_reporte_margen", "cajero")
	abrirCajaPrincipal(t, st, cajero)

	ventaCobrada(t, ctx, st, ordenes, "margen_hoy", "100", cajero, efectivo)
	futura := ventaCobrada(t, ctx, st, ordenes, "margen_despues", "900", cajero, efectivo)
	moverAlDia(t, st, futura, hoy.AddDate(0, 0, 5))

	filas, err := backoffice.ProductMargins(ctx, hoy, hoy, 50)
	if err != nil {
		t.Fatalf("ProductMargins: %v", err)
	}
	total := decimal.Zero
	for _, f := range filas {
		total = total.Add(f.Revenue)
	}
	if !total.Equal(decimal.RequireFromString("100")) {
		t.Fatalf("la utilidad del día suma %s de venta, quiere 100: se coló un producto de otro día", total)
	}
}

// ---- helpers ----

// ventaCobrada deja una venta cerrada y cobrada por el monto pedido. El producto NO pasa por cocina
// para que el pedido se cierre solo al quedar saldado, como una embotellada del mostrador.
func ventaCobrada(t *testing.T, ctx context.Context, st *store.Store, svc *app.OrdersService,
	sufijo, monto string, cajero int64, metodo int16,
) int64 {
	t.Helper()
	prod := makeProduct(t, st, "Reporte "+sufijo, decimal.RequireFromString(monto), false)
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
		OrderID: ord.ID, MethodID: metodo, Amount: decimal.RequireFromString(monto), ActorID: cajero,
	}); err != nil {
		t.Fatalf("Charge(%s): %v", sufijo, err)
	}
	return ord.ID
}

// moverAlDia reescribe el día de negocio de una venta. Es lo único que se toca a mano: el turno
// abierto fecha igual a todos sus pedidos, y separar dos periodos exigiría abrir y cerrar un segundo
// turno solo para eso.
func moverAlDia(t *testing.T, st *store.Store, orderID int64, dia time.Time) {
	t.Helper()
	if _, err := st.Pool.Exec(context.Background(),
		"update orders set business_date = $2 where id = $1", orderID, dia); err != nil {
		t.Fatalf("mover business_date: %v", err)
	}
}

func sumaDeDias(filas []db.SalesByDayRow) decimal.Decimal {
	total := decimal.Zero
	for _, f := range filas {
		total = total.Add(f.Revenue)
	}
	return total
}

func sumaDeMetodos(filas []db.SalesByMethodRow) decimal.Decimal {
	total := decimal.Zero
	for _, f := range filas {
		total = total.Add(f.Total)
	}
	return total
}
