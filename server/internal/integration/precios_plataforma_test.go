//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"
	"uuid"

	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store/db"
)

// Una venta por plataforma se valúa con la lista de ESA plataforma, y el servidor la recalcula: el
// precio que mande el cliente se ignora, igual que en mostrador.
func TestVentaPorPlataformaUsaSuLista(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	ordersSvc := app.NewOrdersService(st, clock)

	cajero := makeUser(t, st, "cajero_plat", "cajero")
	prod := makeProduct(t, st, "Boneless", decimal.RequireFromString("100"), false)
	// El método de la plataforma, no el efectivo de mostrador: un pedido de Uber cobrado con el
	// efectivo del cajón deja al turno esperando billetes que Uber pagó por transferencia.
	uberEfectivo := paymentMethodID(t, st, "Uber Eats efectivo")
	uber := platformID(t, st, defaultCompanyID, "Uber Eats")
	abrirCajaPrincipal(t, st, cajero)

	// Uber trae 35% sembrado por la migración: 100 → 135.
	ord, err := crearYCobrar(t, ctx, ordersSvc, app.CreateOrderCmd{
		ClientUUID:         uuid.New(),
		ServiceType:        "domicilio",
		DeliveryPlatformID: &uber,
		OpenedBy:           cajero,
		Lines:              []domain.OrderLineInput{{ProductID: prod, Qty: decimal.RequireFromString("1")}},
		Payments:           []app.PaymentInput{{MethodID: uberEfectivo, Amount: decimal.RequireFromString("135")}},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if !ord.Total.Equal(decimal.RequireFromString("135")) {
		t.Fatalf("la venta por Uber debe cobrar 135 (100 + 35%%), cobró %s", ord.Total)
	}
	if !ord.Lines[0].UnitPrice.Equal(decimal.RequireFromString("135")) {
		t.Fatalf("el unitario guardado debe ser el de la lista: %s", ord.Lines[0].UnitPrice)
	}
}

// El precio capturado a mano gana sobre el calculado, y PERSISTE: la siguiente venta en esa
// plataforma ya entra con él. Es lo que convierte corregir un precio en trabajo de una sola vez.
func TestPrecioManualGanaYPersiste(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	ordersSvc := app.NewOrdersService(st, clock)

	cajero := makeUser(t, st, "cajero_manual", "cajero")
	prod := makeProduct(t, st, "Alitas", decimal.RequireFromString("100"), false)
	efectivo := paymentMethodID(t, st, "Efectivo")
	rappiEnLinea := paymentMethodID(t, st, "Rappi en línea")
	rappi := platformID(t, st, defaultCompanyID, "Rappi")
	abrirCajaPrincipal(t, st, cajero)

	if err := st.Q.UpsertProductPlatformPrice(ctx, db.UpsertProductPlatformPriceParams{
		ProductID: prod, PlatformID: rappi, Price: decimal.RequireFromString("149"), UpdatedBy: cajero,
	}); err != nil {
		t.Fatalf("capturar el precio: %v", err)
	}

	ord, err := crearYCobrar(t, ctx, ordersSvc, app.CreateOrderCmd{
		ClientUUID:         uuid.New(),
		ServiceType:        "domicilio",
		DeliveryPlatformID: &rappi,
		OpenedBy:           cajero,
		Lines:              []domain.OrderLineInput{{ProductID: prod, Qty: decimal.RequireFromString("1")}},
		Payments:           []app.PaymentInput{{MethodID: rappiEnLinea, Amount: decimal.RequireFromString("149")}},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if !ord.Total.Equal(decimal.RequireFromString("149")) {
		t.Fatalf("el precio capturado debe ganar sobre el calculado (135): cobró %s", ord.Total)
	}

	// Y no contamina: en mostrador el mismo producto sigue en su base.
	enMostrador, err := crearYCobrar(t, ctx, ordersSvc, app.CreateOrderCmd{
		ClientUUID:  uuid.New(),
		ServiceType: "mostrador",
		OpenedBy:    cajero,
		Lines:       []domain.OrderLineInput{{ProductID: prod, Qty: decimal.RequireFromString("1")}},
		Payments:    []app.PaymentInput{{MethodID: efectivo, Amount: decimal.RequireFromString("100")}},
	})
	if err != nil {
		t.Fatalf("Create mostrador: %v", err)
	}
	if !enMostrador.Total.Equal(decimal.RequireFromString("100")) {
		t.Fatalf("mostrador debe cobrar el base: cobró %s", enMostrador.Total)
	}
}

// Una plataforma que no es de esta empresa se rechaza con 422. NUNCA se cae a margen 0: eso
// cobraría precio de mostrador en Uber, con el ticket bien impreso, y el descuadre aparecería
// semanas después al conciliar el depósito.
func TestPlataformaAjenaSeRechaza(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	appSt := appRoleStore(t)
	ordersSvc := app.NewOrdersService(appSt, clock)

	otra := makeCompany(t, st, "otra-plataforma")
	cajero := makeUser(t, st, "cajero_ajena", "cajero")
	prod := makeProduct(t, st, "Café", decimal.RequireFromString("50"), false)
	efectivo := paymentMethodID(t, st, "Efectivo")
	abrirCajaPrincipal(t, st, cajero)
	ajena := platformID(t, st, otra, "Uber Eats")

	tenantCtx, release, err := appSt.AcquireTenant(ctx, defaultCompanyID)
	if err != nil {
		t.Fatalf("AcquireTenant: %v", err)
	}
	defer release()

	_, err = crearYCobrar(t, tenantCtx, ordersSvc, app.CreateOrderCmd{
		ClientUUID:         uuid.New(),
		ServiceType:        "domicilio",
		DeliveryPlatformID: &ajena,
		OpenedBy:           cajero,
		Lines:              []domain.OrderLineInput{{ProductID: prod, Qty: decimal.RequireFromString("1")}},
		Payments:           []app.PaymentInput{{MethodID: efectivo, Amount: decimal.RequireFromString("50")}},
	})
	if !errors.Is(err, domain.ErrPlatformNotFound) {
		t.Fatalf("una plataforma ajena debe rechazarse, fue %v", err)
	}
}

// El reparto lo cobra la plataforma: el costo de envío del negocio se fuerza a 0 aunque el cliente
// mande otra cosa. Sin esto, cada pedido de Uber saldría con $20 de más.
func TestPedidoDePlataformaNoCobraEnvio(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	ordersSvc := app.NewOrdersService(st, clock)

	cajero := makeUser(t, st, "cajero_envio", "cajero")
	prod := makeProduct(t, st, "Pizza", decimal.RequireFromString("200"), false)
	didiEnLinea := paymentMethodID(t, st, "Didi en línea")
	didi := platformID(t, st, defaultCompanyID, "Didi")
	abrirCajaPrincipal(t, st, cajero)

	ord, err := crearYCobrar(t, ctx, ordersSvc, app.CreateOrderCmd{
		ClientUUID:         uuid.New(),
		ServiceType:        "domicilio",
		DeliveryPlatformID: &didi,
		DeliveryFee:        decimal.RequireFromString("20"), // el cliente lo manda; se ignora
		OpenedBy:           cajero,
		Lines:              []domain.OrderLineInput{{ProductID: prod, Qty: decimal.RequireFromString("1")}},
		Payments:           []app.PaymentInput{{MethodID: didiEnLinea, Amount: decimal.RequireFromString("270")}},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if !ord.DeliveryFee.IsZero() {
		t.Fatalf("un pedido de plataforma no cobra envío del negocio, cobró %s", ord.DeliveryFee)
	}
	if !ord.Total.Equal(decimal.RequireFromString("270")) {
		t.Fatalf("total = %s, quería 270 (200 + 35%%)", ord.Total)
	}
}

// El menú trae todo lo que el POS necesita para pintar cualquier lista sin volver a pedir nada:
// cambiar de plataforma tiene que ser instantáneo. Y "Propio" NO se ofrece — es reparto del propio
// negocio, sin comisión que absorber ni método de pago propio.
func TestElMenuTraeLasListasDePrecios(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	menu := app.NewMenuService(st, clock)

	cajero := makeUser(t, st, "cajero_menu", "cajero")
	prod := makeProduct(t, st, "Boneless", decimal.RequireFromString("100"), false)
	uber := platformID(t, st, defaultCompanyID, "Uber Eats")
	if err := st.Q.UpsertProductPlatformPrice(ctx, db.UpsertProductPlatformPriceParams{
		ProductID: prod, PlatformID: uber, Price: decimal.RequireFromString("149"), UpdatedBy: cajero,
	}); err != nil {
		t.Fatalf("capturar el precio: %v", err)
	}

	doc, err := menu.Build(ctx)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	nombres := map[string]bool{}
	for _, p := range doc.Platforms {
		nombres[p.Name] = true
		if p.Name != "Propio" && !p.MarkupPct.Equal(decimal.RequireFromString("35")) {
			t.Fatalf("%s debe traer su margen de 35%%, trajo %s", p.Name, p.MarkupPct)
		}
	}
	if nombres["Propio"] {
		t.Fatal("\"Propio\" no debe ofrecerse como lista de precios")
	}
	for _, esperada := range []string{"Didi", "Uber Eats", "Rappi"} {
		if !nombres[esperada] {
			t.Fatalf("falta la plataforma %s en el menú", esperada)
		}
	}

	// Solo las excepciones: el producto con precio capturado, y nada más.
	if got := doc.PlatformPrices[uber][prod]; !got.Equal(decimal.RequireFromString("149")) {
		t.Fatalf("el precio capturado debe venir en el menú, vino %s", got)
	}
}

// Capturar un precio desde la pantalla de venta: persiste, no contamina las otras listas, y se
// puede quitar para volver al calculado. Sin el borrado, un precio equivocado pero plausible
// —$14.90 donde iban $149.00— pasa todas las validaciones y se cobra así para siempre, porque el
// check `price > 0` cierra el idioma "pon 0 para limpiar".
func TestCapturarYQuitarUnPrecioDePlataforma(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	svc := app.NewPlatformPricesService(st)

	admin := makeUser(t, st, "admin_precios", "admin")
	prod := makeProduct(t, st, "Boneless", decimal.RequireFromString("100"), false)
	uber := platformID(t, st, defaultCompanyID, "Uber Eats")
	rappi := platformID(t, st, defaultCompanyID, "Rappi")

	if err := svc.SetProductPrice(ctx, prod, uber, decimal.RequireFromString("149"), admin); err != nil {
		t.Fatalf("capturar: %v", err)
	}

	var precio decimal.Decimal
	if err := st.Pool.QueryRow(ctx,
		`select price from product_platform_prices where product_id=$1 and platform_id=$2`, prod, uber).Scan(&precio); err != nil {
		t.Fatalf("leer el precio: %v", err)
	}
	if !precio.Equal(decimal.RequireFromString("149")) {
		t.Fatalf("precio guardado = %s", precio)
	}

	// No contamina la otra plataforma.
	var n int
	if err := st.Pool.QueryRow(ctx,
		`select count(*) from product_platform_prices where product_id=$1 and platform_id=$2`, prod, rappi).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatal("capturar en Uber no debe crear precio en Rappi")
	}

	// Quitarlo devuelve el producto al calculado.
	if borro, err := svc.DeleteProductPrice(ctx, prod, uber); err != nil || !borro {
		t.Fatalf("quitar: borro=%v err=%v", borro, err)
	}
	if err := st.Pool.QueryRow(ctx,
		`select count(*) from product_platform_prices where product_id=$1 and platform_id=$2`, prod, uber).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatal("el precio capturado debe desaparecer")
	}
	// Borrar lo que no existe deja el mundo como se pidió: no es un error, pero tampoco un borrado.
	if borro, err := svc.DeleteProductPrice(ctx, prod, uber); err != nil || borro {
		t.Fatalf("borrar lo inexistente: borro=%v err=%v", borro, err)
	}
}

// Un precio absurdo se rechaza en la frontera, como 4xx y no como un check violado de Postgres
// convertido en 500.
func TestPrecioDePlataformaInvalidoSeRechaza(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	svc := app.NewPlatformPricesService(st)

	admin := makeUser(t, st, "admin_invalido", "admin")
	prod := makeProduct(t, st, "Café", decimal.RequireFromString("50"), false)
	uber := platformID(t, st, defaultCompanyID, "Uber Eats")

	for _, malo := range []string{"0", "-10"} {
		if err := svc.SetProductPrice(ctx, prod, uber, decimal.RequireFromString(malo), admin); !errors.Is(err, domain.ErrValidation) {
			t.Fatalf("un precio de %s debe rechazarse como validación, fue %v", malo, err)
		}
	}

	// Un extra SÍ puede costar 0 ("sin cebolla"), pero no negativo.
	opt := optionID(t, st, defaultCompanyID)
	if err := svc.SetOptionDelta(ctx, opt, uber, decimal.Zero, admin); err != nil {
		t.Fatalf("un extra sin costo es válido: %v", err)
	}
	if err := svc.SetOptionDelta(ctx, opt, uber, decimal.RequireFromString("-1"), admin); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("un delta negativo debe rechazarse, fue %v", err)
	}
}

// Un cajero de la empresa A NO puede escribir un precio sobre el producto de la empresa B.
//
// La llave foránea no alcanza: sus chequeos saltan RLS por diseño, así que la fila entraba con
// company_id = A y ocupaba la PK global (product_id, platform_id). El daño no era solo escribir
// donde no debe: a partir de ahí B ya no podía capturar SU precio para ese producto —el upsert caía
// en ON CONFLICT DO UPDATE y chocaba con la política, saliendo como 500— ni borrar la fila
// intrusa, porque su DELETE bajo RLS no la ve. Irreparable desde el producto, y con los ids
// seriales se podía recorrer el catálogo ajeno completo.
func TestNoSePuedeEscribirElPrecioDeOtraEmpresa(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	appSt := appRoleStore(t)
	svc := app.NewPlatformPricesService(appSt)

	otra := makeCompany(t, st, "otra-escritura")
	cajero := makeUser(t, st, "cajero_squat", "cajero")
	uber := platformID(t, st, defaultCompanyID, "Uber Eats")

	// Un producto de la OTRA empresa.
	var ajeno int64
	if err := st.Pool.QueryRow(ctx,
		`insert into products (company_id, category_id, name, price)
		 select $1, c.id, 'Producto ajeno', 100 from categories c where c.company_id = $1 limit 1
		 returning id`, otra).Scan(&ajeno); err != nil {
		// La otra empresa no tiene categorías: se crea una.
		var cat int64
		if err := st.Pool.QueryRow(ctx,
			`insert into categories (company_id, name) values ($1, 'Ajena') returning id`, otra).Scan(&cat); err != nil {
			t.Fatalf("categoría ajena: %v", err)
		}
		if err := st.Pool.QueryRow(ctx,
			`insert into products (company_id, category_id, name, price) values ($1, $2, 'Producto ajeno', 100) returning id`,
			otra, cat).Scan(&ajeno); err != nil {
			t.Fatalf("producto ajeno: %v", err)
		}
	}

	tenantCtx, release, err := appSt.AcquireTenant(ctx, defaultCompanyID)
	if err != nil {
		t.Fatalf("AcquireTenant: %v", err)
	}
	defer release()

	err = svc.SetProductPrice(tenantCtx, ajeno, uber, decimal.RequireFromString("1"), cajero)
	if err == nil {
		t.Fatal("escribir el precio de un producto ajeno debe rechazarse")
	}
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("debe salir como no encontrado (mismo mensaje que un id inexistente), fue %v", err)
	}

	var n int
	if err := st.Pool.QueryRow(ctx, `select count(*) from product_platform_prices where product_id=$1`, ajeno).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("no debe quedar ninguna fila sobre el producto ajeno, hay %d", n)
	}
}

// Un id que no existe y uno que es de otra empresa deben responder IGUAL. Si "no existe" diera 500
// y "es ajeno" diera 200, recorrer los ids devolvería el censo de catálogo de todos los negocios.
func TestUnIdInexistenteYUnoAjenoRespondenIgual(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	appSt := appRoleStore(t)
	svc := app.NewPlatformPricesService(appSt)

	cajero := makeUser(t, st, "cajero_oraculo", "cajero")
	uber := platformID(t, st, defaultCompanyID, "Uber Eats")

	tenantCtx, release, err := appSt.AcquireTenant(ctx, defaultCompanyID)
	if err != nil {
		t.Fatal(err)
	}
	defer release()

	if err := svc.SetProductPrice(tenantCtx, 999999, uber, decimal.RequireFromString("50"), cajero); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("un id inexistente debe dar no encontrado, no 500: %v", err)
	}
	if err := svc.SetOptionDelta(tenantCtx, 999999, uber, decimal.RequireFromString("5"), cajero); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("una opción inexistente debe dar no encontrado, no 500: %v", err)
	}
}
