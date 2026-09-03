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

// rangoDePrueba: el día del reloj fijo de los tests, que es la fecha de negocio con la que se
// siembran las ventas.
func rangoDePrueba() domain.Range {
	d := domain.BusinessDate(fixedNow, nil)
	return domain.Range{From: d, To: d}
}

func filtroDePrueba() domain.SalesFilter {
	return domain.SalesFilter{Range: rangoDePrueba(), Sort: "fecha", Dir: "desc", Limit: 20}
}

// Las consultas nuevas tienen que ser usables POR EL ROL DE APP, no solo por el owner.
//
// El grant de 0024 fue `on all tables in schema public`, que es puntual: no hay default privileges.
// Este test es invisible en dev —la API local sirve como owner— y en producción es la diferencia
// entre una pantalla que abre y un 42501 en el primer request.
func TestLaPantallaDeVentasEsUsablePorElRolDeApp(t *testing.T) {
	owner := newTestStore(t)
	appSt := appRoleStore(t)
	ctx := context.Background()

	cajero := makeUser(t, owner, "cajero_ventas", "cajero")
	prod := makeProduct(t, owner, "Café ventas", decimal.RequireFromString("50"), false)
	efectivo := paymentMethodID(t, owner, "Efectivo")
	abrirCajaPrincipal(t, owner, cajero)
	venderPara(t, owner, cajero, prod, efectivo, "50")

	tenantCtx, release, err := appSt.AcquireTenant(ctx, defaultCompanyID)
	if err != nil {
		t.Fatalf("AcquireTenant: %v", err)
	}
	defer release()

	svc := app.NewSalesService(appSt, clock)
	page, err := svc.List(tenantCtx, filtroDePrueba())
	if err != nil {
		t.Fatalf("listar bajo el rol de app: %v (¿falta un grant?)", err)
	}
	if len(page.Items) == 0 {
		t.Fatal("el rol de app debe ver su propia venta")
	}
	if _, err := svc.Summary(tenantCtx, filtroDePrueba()); err != nil {
		t.Fatalf("resumen bajo el rol de app: %v (¿falta un grant?)", err)
	}
}

// El resumen de una empresa no incluye ni un peso de la otra.
//
// Un agregado es la forma más silenciosa de fugar entre tenants: no devuelve filas ajenas que se
// vean, devuelve un número más grande. Nadie lo nota hasta que el dueño compara con su caja.
func TestElResumenDeVentasNoMezclaEmpresas(t *testing.T) {
	owner := newTestStore(t)
	appSt := appRoleStore(t)
	ctx := context.Background()

	otra := makeCompany(t, owner, "otra-ventas")
	cajero := makeUser(t, owner, "cajero_iso_ventas", "cajero")
	prod := makeProduct(t, owner, "Alitas ventas", decimal.RequireFromString("200"), false)
	efectivo := paymentMethodID(t, owner, "Efectivo")
	abrirCajaPrincipal(t, owner, cajero)
	venderPara(t, owner, cajero, prod, efectivo, "200")

	tenantCtx, release, err := appSt.AcquireTenant(ctx, otra)
	if err != nil {
		t.Fatalf("AcquireTenant: %v", err)
	}
	defer release()

	svc := app.NewSalesService(appSt, clock)
	sum, err := svc.Summary(tenantCtx, filtroDePrueba())
	if err != nil {
		t.Fatalf("resumen de la otra empresa: %v", err)
	}
	if sum.Count != 0 || !sum.Total.IsZero() {
		t.Fatalf("la otra empresa no vendió nada y su resumen dice %d ventas por %s", sum.Count, sum.Total)
	}
	page, err := svc.List(tenantCtx, filtroDePrueba())
	if err != nil {
		t.Fatalf("lista de la otra empresa: %v", err)
	}
	if len(page.Items) != 0 || page.Total != 0 {
		t.Fatalf("la otra empresa no debe ver ventas ajenas, vio %d de %d", len(page.Items), page.Total)
	}
}

// Cada peso se clasifica UNA sola vez, sobre datos reales.
//
// La propina es dinero del personal; la cancelada es ingreso que no ocurrió. Que el total las
// excluya es la diferencia entre reportar la venta del día y reportar un número inventado. El test
// falla nombrando qué concepto se coló, no "esperaba X obtuve Y".
func TestElResumenDeVentasClasificaCadaPesoUnaVez(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	cajero := makeUser(t, st, "cajero_clasif", "cajero")
	prod := makeProduct(t, st, "Boneless clasif", decimal.RequireFromString("100"), false)
	efectivo := paymentMethodID(t, st, "Efectivo")
	abrirCajaPrincipal(t, st, cajero)
	ordersSvc := app.NewOrdersService(st, clock)

	// Dos ventas buenas de $100, una con $15 de propina.
	if _, err := crearYCobrar(t, ctx, ordersSvc, app.CreateOrderCmd{
		ClientUUID: uuid.New(), ServiceType: "mostrador", OpenedBy: cajero,
		Lines:    []domain.OrderLineInput{{ProductID: prod, Qty: decimal.RequireFromString("1")}},
		Payments: []app.PaymentInput{{MethodID: efectivo, Amount: decimal.RequireFromString("100"), Tip: decimal.RequireFromString("15")}},
	}); err != nil {
		t.Fatalf("venta con propina: %v", err)
	}
	venderPara(t, st, cajero, prod, efectivo, "100")

	// Y una cancelada, que NO es ingreso.
	cancelada, err := crearYCobrar(t, ctx, ordersSvc, app.CreateOrderCmd{
		ClientUUID: uuid.New(), ServiceType: "mostrador", OpenedBy: cajero,
		Lines: []domain.OrderLineInput{{ProductID: prod, Qty: decimal.RequireFromString("1")}},
	})
	if err != nil {
		t.Fatalf("venta a cancelar: %v", err)
	}
	if err := ordersSvc.Cancel(ctx, cancelada.ID, cajero, "prueba"); err != nil {
		t.Fatalf("cancelar: %v", err)
	}

	sum, err := app.NewSalesService(st, clock).Summary(ctx, filtroDePrueba())
	if err != nil {
		t.Fatalf("resumen: %v", err)
	}

	if !sum.Total.Equal(decimal.RequireFromString("200")) {
		t.Fatalf("total = %s, quiere 200: se coló la cancelada o la propina", sum.Total)
	}
	if !sum.Tips.Equal(decimal.RequireFromString("15")) {
		t.Fatalf("propinas = %s, quiere 15", sum.Tips)
	}
	if sum.Cancelled.Count != 1 || !sum.Cancelled.Amount.Equal(decimal.RequireFromString("100")) {
		t.Fatalf("canceladas = %+v, quiere 1 por 100", sum.Cancelled)
	}
	if sum.Count != 2 {
		t.Fatalf("conteo = %d, quiere 2: la cancelada no es una venta", sum.Count)
	}

	// El desglose por método cuenta lo COBRADO, propina incluida como campo aparte.
	if len(sum.ByMethod) != 1 || sum.ByMethod[0].Method != "Efectivo" {
		t.Fatalf("desglose por método = %+v, quiere un solo renglón de Efectivo", sum.ByMethod)
	}
	if !sum.ByMethod[0].Total.Equal(decimal.RequireFromString("200")) {
		t.Fatalf("cobrado en efectivo = %s, quiere 200", sum.ByMethod[0].Total)
	}
}

// La tabla y el resumen se derivan del mismo rango: si divergieran, uno de los dos miente y el
// operador no tiene forma de saber cuál.
func TestLaTablaYElResumenCuadran(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	cajero := makeUser(t, st, "cajero_cuadre", "cajero")
	prod := makeProduct(t, st, "Papas cuadre", decimal.RequireFromString("40"), false)
	efectivo := paymentMethodID(t, st, "Efectivo")
	abrirCajaPrincipal(t, st, cajero)
	for i := 0; i < 3; i++ {
		venderPara(t, st, cajero, prod, efectivo, "40")
	}

	svc := app.NewSalesService(st, clock)
	page, err := svc.List(ctx, filtroDePrueba())
	if err != nil {
		t.Fatalf("lista: %v", err)
	}
	sum, err := svc.Summary(ctx, filtroDePrueba())
	if err != nil {
		t.Fatalf("resumen: %v", err)
	}

	suma := decimal.Zero
	for _, it := range page.Items {
		suma = suma.Add(it.Total)
	}
	if !suma.Equal(sum.Total) {
		t.Fatalf("la tabla suma %s y el resumen dice %s", suma, sum.Total)
	}
	if int(page.Total) != sum.Count {
		t.Fatalf("la tabla trae %d filas y el resumen cuenta %d ventas", page.Total, sum.Count)
	}
	// Y el rango que reportan las dos respuestas es el mismo, que es lo que permite verlo en
	// pantalla en vez de descubrirlo como un descuadre sin causa.
	if page.Range != sum.Range {
		t.Fatalf("rangos distintos: lista %+v, resumen %+v", page.Range, sum.Range)
	}
}

// venderPara siembra una venta cobrada. Vive aquí y no en el harness porque es lo que estos tests
// necesitan repetir; el resto de los archivos arma sus pedidos con lo suyo.
func venderPara(t *testing.T, st *store.Store, cajero, prod int64, metodo int16, monto string) {
	t.Helper()
	if _, err := crearYCobrar(t, context.Background(), app.NewOrdersService(st, clock), app.CreateOrderCmd{
		ClientUUID:  uuid.New(),
		ServiceType: "mostrador",
		OpenedBy:    cajero,
		Lines:       []domain.OrderLineInput{{ProductID: prod, Qty: decimal.RequireFromString("1")}},
		Payments:    []app.PaymentInput{{MethodID: metodo, Amount: decimal.RequireFromString(monto)}},
	}); err != nil {
		t.Fatalf("sembrar venta de %s: %v", monto, err)
	}
}

// El filtro de tipo de venta tiene que aplicar a TODAS las cifras del resumen, no solo a algunas.
//
// La propina salía de una subconsulta que filtraba por fecha y estado pero NO por tipo de venta:
// con la pantalla filtrada a domicilio, el tile de propinas sumaba también las de mostrador. Un
// número inflado junto a otros correctos es peor que uno mal parejo, porque invita a confiar en el
// resto. Lo mismo pasaba con las líneas canceladas, que ni recibían el filtro.
func TestElFiltroDeTipoDeVentaAplicaATodoElResumen(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	cajero := makeUser(t, st, "cajero_tipo", "cajero")
	prod := makeProduct(t, st, "Café tipo", decimal.RequireFromString("100"), false)
	efectivo := paymentMethodID(t, st, "Efectivo")
	abrirCajaPrincipal(t, st, cajero)
	ordersSvc := app.NewOrdersService(st, clock)

	venta := func(tipo string, propina string) {
		t.Helper()
		if _, err := crearYCobrar(t, ctx, ordersSvc, app.CreateOrderCmd{
			ClientUUID: uuid.New(), ServiceType: tipo, OpenedBy: cajero,
			Lines: []domain.OrderLineInput{{ProductID: prod, Qty: decimal.RequireFromString("1")}},
			Payments: []app.PaymentInput{{
				MethodID: efectivo, Amount: decimal.RequireFromString("100"),
				Tip: decimal.RequireFromString(propina),
			}},
		}); err != nil {
			t.Fatalf("venta %s: %v", tipo, err)
		}
	}
	venta("mostrador", "50")
	venta("para_llevar", "7")

	f := filtroDePrueba()
	f.ServiceType = "para_llevar"
	sum, err := app.NewSalesService(st, clock).Summary(ctx, f)
	if err != nil {
		t.Fatalf("resumen filtrado: %v", err)
	}

	if !sum.Total.Equal(decimal.RequireFromString("100")) {
		t.Fatalf("total = %s, quiere 100: el filtro de tipo no se aplicó a las ventas", sum.Total)
	}
	// 7 y no 57: la propina de mostrador no es de este filtro.
	if !sum.Tips.Equal(decimal.RequireFromString("7")) {
		t.Fatalf("propinas = %s, quiere 7: se colaron las de otro tipo de venta", sum.Tips)
	}
	// Y el desglose por método también respeta el filtro.
	if len(sum.ByMethod) != 1 || !sum.ByMethod[0].Total.Equal(decimal.RequireFromString("100")) {
		t.Fatalf("por método = %+v, quiere un renglón de 100", sum.ByMethod)
	}
}
