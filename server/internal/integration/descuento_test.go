//go:build integration

package integration

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/shopspring/decimal"
	"uuid"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/auth"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/config"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/httpapi"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/realtime"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store/db"
)

func pesos(s string) decimal.Decimal { return decimal.RequireFromString(s) }

// pedidoDe crea un pedido SIN cobrar con un solo producto del precio que se pida.
func pedidoConDescuento(t *testing.T, st *store.Store, svc *app.OrdersService, sufijo, precio string,
	cmd app.CreateOrderCmd) (*app.OrderView, int64) {
	t.Helper()
	cajero := makeUser(t, st, "cajero_"+sufijo, "cajero")
	prod := makeProduct(t, st, "Crepa "+sufijo, pesos(precio), false)
	abrirCajaPrincipal(t, st, cajero)

	cmd.ClientUUID = uuid.New()
	if cmd.ServiceType == "" {
		cmd.ServiceType = "mostrador"
	}
	cmd.OpenedBy = cajero
	cmd.Lines = []domain.OrderLineInput{{ProductID: prod, Qty: pesos("1")}}

	ord, err := svc.Create(context.Background(), cmd)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	return ord, cajero
}

// LA MIGRACIÓN, sobre DOS empresas. Con una sola, todo camino "por cada otra empresa" es un no-op y
// la migración pasa verde para romper en producción.
//
// Lo que se prueba no es que las columnas existan —eso lo diría el compilador— sino que un pedido
// ordinario, el de todos los días, siga naciendo sin descuento y SIN autor. Si el default cambiara
// o el insert estampara un autor por costumbre, cada venta normal quedaría marcada como descontada.
func TestLosPedidosNacenSinDescuentoYSinRastro(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	svc := app.NewOrdersService(st, clock)

	otra := makeCompany(t, st, "empresa-descuentos")

	ord, _ := pedidoConDescuento(t, st, svc, "sin_descuento", "120", app.CreateOrderCmd{})
	if !ord.Discount.IsZero() {
		t.Fatalf("un pedido sin descuento nació con %s", ord.Discount)
	}

	for _, empresa := range []int64{defaultCompanyID, otra} {
		var descuentos, conAutor int
		if err := st.Pool.QueryRow(ctx,
			`select count(*), count(discount_set_by) from orders where company_id = $1 and discount_total > 0`,
			empresa).Scan(&descuentos, &conAutor); err != nil {
			t.Fatalf("contar descuentos de la empresa %d: %v", empresa, err)
		}
		if descuentos != conAutor {
			t.Fatalf("empresa %d: %d pedidos con descuento y solo %d con autor — "+
				"el rastro dejó de ser obligatorio y no hay a quién responsabilizar por ese dinero",
				empresa, descuentos, conAutor)
		}
	}
}

// El check es lo que convierte el rastro en una garantía del ESQUEMA y no en una promesa de la
// aplicación: cualquier ruta nueva que escriba el monto sin el actor falla aquí, ruidosamente, en
// vez de dejar dinero descontado sin nadie detrás.
func TestNoSePuedeDescontarSinDecirQuienFue(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	svc := app.NewOrdersService(st, clock)

	ord, _ := pedidoConDescuento(t, st, svc, "sin_autor", "300", app.CreateOrderCmd{})

	_, err := st.Pool.Exec(ctx,
		`update orders set discount_total = 50 where id = $1`, ord.ID)
	if err == nil {
		t.Fatal("se guardó un descuento de $50 SIN autor: el check orders_descuento_con_rastro no " +
			"está, y el único control contra el abuso de esta feature es justamente ese rastro")
	}
}

// BAJO EL ROL DE LA APLICACIÓN, que es como corre producción: el resto de este archivo conecta como
// owner y salta RLS. Un grant que falte no se ve en dev —la API de desarrollo también entra como
// owner— y aparece en producción como 42501 en el primer descuento del día.
func TestElDescuentoSeEscribeBajoElRolDeLaAplicacion(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	svc := app.NewOrdersService(st, clock)

	ord, cajero := pedidoConDescuento(t, st, svc, "rol_app", "400", app.CreateOrderCmd{})

	appSt := appRoleStore(t)
	if err := appSt.WithTenant(ctx, defaultCompanyID, func(q *db.Queries) error {
		return q.SetOrderDiscount(ctx, db.SetOrderDiscountParams{
			ID: ord.ID, Descuento: pesos("100"), Quien: cajero,
		})
	}); err != nil {
		t.Fatalf("escribir el descuento como gatobobah_app: %v — en producción ningún descuento entraría", err)
	}

	tras, err := svc.Detail(ctx, ord.ID)
	if err != nil {
		t.Fatalf("Detail: %v", err)
	}
	if !tras.Total.Equal(pesos("300")) {
		t.Fatalf("total tras descontar $100 de $400: %s", tras.Total)
	}
}

// El porcentaje se resuelve contra el subtotal DEL SERVIDOR. Si se resolviera contra uno que mande
// el cliente, un 20 % podría descontar cualquier cantidad: sería creerle al cliente una cifra de
// dinero, que es justo lo que BuildOrder no hace con los precios.
func TestElServidorResuelveElPorcentajeContraSuPropioSubtotal(t *testing.T) {
	st := newTestStore(t)
	svc := app.NewOrdersService(st, clock)

	veinte := pesos("20")
	ord, _ := pedidoConDescuento(t, st, svc, "porcentaje", "385", app.CreateOrderCmd{DiscountPercent: &veinte})

	if !ord.Discount.Equal(pesos("77")) {
		t.Fatalf("20%% de $385 son $77 y se guardó %s", ord.Discount)
	}
	if !ord.Total.Equal(pesos("308")) {
		t.Fatalf("total: quería 308, obtuve %s", ord.Total)
	}
	// Lo que queda guardado es el MONTO, no la fórmula: de eso depende que el descuento no crezca
	// solo cuando alguien le agregue algo al pedido.
	var guardado decimal.Decimal
	if err := st.Pool.QueryRow(context.Background(),
		`select discount_total from orders where id = $1`, ord.ID).Scan(&guardado); err != nil {
		t.Fatalf("leer el descuento: %v", err)
	}
	if !guardado.Equal(pesos("77")) {
		t.Fatalf("en la base quedó %s", guardado)
	}
}

// EL DINERO DEL TURNO. El descuento se clasifica UNA sola vez: es una reducción del ingreso dentro
// del total, no un renglón hermano que alguien pueda volver a restar.
//
// Falla nombrando el concepto duplicado, no "esperaba X obtuve Y": si el corte dijera $285 sería
// que el descuento se restó dos veces, y si dijera $385, que no se restó ninguna.
func TestElCorteCuentaElTotalRebajadoUnaSolaVez(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	svc := app.NewOrdersService(st, clock)
	backoffice := app.NewBackofficeService(st, clock)

	cajero := makeUser(t, st, "cajero_corte_desc", "cajero")
	prod := makeProduct(t, st, "Crepa del corte", pesos("385"), false)
	efectivo := paymentMethodID(t, st, "Efectivo")
	principal := registerID(t, st, "Caja principal")
	if _, err := backoffice.OpenSession(ctx, principal, app.AperturaCmd{}, cajero); err != nil {
		t.Fatalf("OpenSession: %v", err)
	}

	cincuenta := pesos("50")
	ord, err := crearYCobrar(t, ctx, svc, app.CreateOrderCmd{
		ClientUUID: uuid.New(), ServiceType: "mostrador", OpenedBy: cajero,
		DiscountAmount: &cincuenta,
		Lines:          []domain.OrderLineInput{{ProductID: prod, Qty: pesos("1")}},
		Payments:       []app.PaymentInput{{MethodID: efectivo, Amount: pesos("335")}},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if !ord.Total.Equal(pesos("335")) {
		t.Fatalf("total del pedido: %s", ord.Total)
	}

	turno, err := backoffice.CurrentByRegister(ctx, principal)
	if err != nil {
		t.Fatalf("CurrentByRegister: %v", err)
	}
	for _, tot := range turno.Totals {
		if tot.Name != "Efectivo" {
			continue
		}
		switch {
		case tot.Expected.Equal(pesos("335")):
			return // lo correcto: el ingreso es el total ya rebajado
		case tot.Expected.Equal(pesos("385")):
			t.Fatalf("el corte espera $385: el DESCUENTO no se restó del ingreso y la caja va a " +
				"cerrar con $50 de faltante que nadie va a poder explicar")
		case tot.Expected.Equal(pesos("285")):
			t.Fatalf("el corte espera $285: el DESCUENTO se restó DOS VECES —una en el total del " +
				"pedido y otra en el corte— y la caja va a cerrar con $50 de sobrante")
		default:
			t.Fatalf("el corte espera %s y el pedido cobró $335", tot.Expected)
		}
	}
	t.Fatal("el corte no trajo el renglón de Efectivo")
}

// Cancelar renglones mueve el subtotal y NO mueve el descuento: el descuento es lo que una persona
// decidió. Lo que se recorta es el total, con piso en cero — un total negativo devolvería dinero
// que nadie autorizó, y ningún reporte lo diría.
func TestCancelarUnRenglonNoDejaElTotalNegativo(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	svc := app.NewOrdersService(st, clock)

	cajero := makeUser(t, st, "cajero_cancelar_desc", "cajero")
	caro := makeProduct(t, st, "Crepa cara", pesos("300"), false)
	barato := makeProduct(t, st, "Agua", pesos("20"), false)
	abrirCajaPrincipal(t, st, cajero)

	cien := pesos("100")
	ord, err := svc.Create(ctx, app.CreateOrderCmd{
		ClientUUID: uuid.New(), ServiceType: "mostrador", OpenedBy: cajero,
		DiscountAmount: &cien,
		Lines: []domain.OrderLineInput{
			{ProductID: caro, Qty: pesos("1")},
			{ProductID: barato, Qty: pesos("1")},
		},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if !ord.Total.Equal(pesos("220")) { // 320 - 100
		t.Fatalf("total inicial: %s", ord.Total)
	}

	// Se cancela el caro: quedan $20 de subtotal contra $100 de descuento.
	if _, err := svc.CancelarRenglon(ctx, ord.ID, ord.Lines[0].ID, cajero, "el cliente cambió de opinión"); err != nil {
		t.Fatalf("CancelarRenglon: %v", err)
	}
	tras, err := svc.Detail(ctx, ord.ID)
	if err != nil {
		t.Fatalf("Detail: %v", err)
	}
	if tras.Total.IsNegative() {
		t.Fatalf("el total quedó en %s: se le va a COBRAR AL REVÉS al cliente", tras.Total)
	}
	if !tras.Total.IsZero() {
		t.Fatalf("subtotal $20 menos descuento $100 debe quedar en 0 y quedó en %s", tras.Total)
	}
	if !tras.Discount.Equal(pesos("100")) {
		t.Fatalf("el descuento registrado se recortó solo a %s: lo que se recorta es el total, "+
			"no lo que una persona decidió descontar", tras.Discount)
	}
}

// Agregar renglones NO recalcula el descuento, y por eso se guarda en pesos y no como porcentaje:
// si se recalculara, agregarle un café a la cuenta cambiaría el descuento sin que nadie lo pidiera
// y el ticket que el cliente ya tiene en la mano dejaría de cuadrar.
func TestAgregarRenglonesNoRecalculaElDescuento(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	svc := app.NewOrdersService(st, clock)

	cajero := makeUser(t, st, "cajero_agregar_desc", "cajero")
	prod := makeProduct(t, st, "Crepa que crece", pesos("100"), false)
	abrirCajaPrincipal(t, st, cajero)

	veinte := pesos("20")
	ord, err := svc.Create(ctx, app.CreateOrderCmd{
		ClientUUID: uuid.New(), ServiceType: "mostrador", OpenedBy: cajero,
		DiscountPercent: &veinte, // 20% de $100 = $20
		Lines:           []domain.OrderLineInput{{ProductID: prod, Qty: pesos("1")}},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if _, err := svc.AddLines(ctx, ord.ID,
		[]domain.OrderLineInput{{ProductID: prod, Qty: pesos("1")}}, cajero, uuid.New()); err != nil {
		t.Fatalf("AddLines: %v", err)
	}
	tras, err := svc.Detail(ctx, ord.ID)
	if err != nil {
		t.Fatalf("Detail: %v", err)
	}
	if !tras.Discount.Equal(pesos("20")) {
		t.Fatalf("el descuento se recalculó a %s al agregar un renglón: se guardó la fórmula y no "+
			"el monto, y el pedido cambió de descuento sin que nadie lo decidiera", tras.Discount)
	}
	if !tras.Total.Equal(pesos("180")) { // 200 - 20
		t.Fatalf("total: quería 180, obtuve %s", tras.Total)
	}
}

// Un pedido ya cobrado no admite cambio de descuento: movería el total contra pagos que ya se
// registraron, y contra un arqueo que quizá ya se firmó.
func TestUnPedidoCobradoNoAdmiteCambioDeDescuento(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	svc := app.NewOrdersService(st, clock)

	cajero := makeUser(t, st, "cajero_cobrado_desc", "cajero")
	prod := makeProduct(t, st, "Crepa cobrada", pesos("200"), false)
	efectivo := paymentMethodID(t, st, "Efectivo")
	abrirCajaPrincipal(t, st, cajero)

	ord, err := crearYCobrar(t, ctx, svc, app.CreateOrderCmd{
		ClientUUID: uuid.New(), ServiceType: "mostrador", OpenedBy: cajero,
		Lines:    []domain.OrderLineInput{{ProductID: prod, Qty: pesos("1")}},
		Payments: []app.PaymentInput{{MethodID: efectivo, Amount: pesos("200")}},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	cincuenta := pesos("50")
	_, err = svc.SetDiscount(ctx, app.SetDiscountCmd{OrderID: ord.ID, Amount: &cincuenta, Actor: cajero})
	if !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("se dejó descontar sobre un pedido YA COBRADO (err=%v): el total baja a $150 "+
			"contra un pago de $200 que ya entró al corte", err)
	}
}

// Un descuento que deja el total por debajo de lo ya abonado deja el pedido sobrepagado sin forma
// de devolver la diferencia. El pago parcial no es raro: el cliente deja algo al pedir y termina al
// recoger.
func TestElDescuentoNoPuedeDejarElTotalBajoLoYaAbonado(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	svc := app.NewOrdersService(st, clock)

	cajero := makeUser(t, st, "cajero_abonado_desc", "cajero")
	prod := makeProduct(t, st, "Crepa abonada", pesos("400"), false)
	efectivo := paymentMethodID(t, st, "Efectivo")
	abrirCajaPrincipal(t, st, cajero)

	ord, err := svc.Create(ctx, app.CreateOrderCmd{
		ClientUUID: uuid.New(), ServiceType: "mostrador", OpenedBy: cajero,
		Lines: []domain.OrderLineInput{{ProductID: prod, Qty: pesos("1")}},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := svc.Charge(ctx, app.ChargeCmd{
		OrderID: ord.ID, MethodID: efectivo, Amount: pesos("200"), ActorID: cajero,
	}); err != nil {
		t.Fatalf("Charge parcial: %v", err)
	}

	trescientos := pesos("300")
	_, err = svc.SetDiscount(ctx, app.SetDiscountCmd{OrderID: ord.ID, Amount: &trescientos, Actor: cajero})
	if !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("se dejó bajar el total a $100 con $200 ya abonados (err=%v): el pedido queda "+
			"sobrepagado y no hay por dónde devolver los $100", err)
	}
}

// Cualquiera que pueda capturar un pedido puede descontar: es una decisión, no un olvido (un pedido
// de plataforma con promoción llega a cualquier hora y esperar a alguien con rol detendría la
// captura). El test existe para que la decisión no se revierta sola en una refactorización.
func TestUnMeseroPuedeDescontar(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	svc := app.NewOrdersService(st, clock)

	mesero := makeUser(t, st, "mesero_desc", "mesero")
	prod := makeProduct(t, st, "Crepa del mesero", pesos("100"), false)
	abrirCajaPrincipal(t, st, mesero)

	diez := pesos("10")
	ord, err := svc.Create(ctx, app.CreateOrderCmd{
		ClientUUID: uuid.New(), ServiceType: "mostrador", OpenedBy: mesero,
		DiscountAmount: &diez,
		Lines:          []domain.OrderLineInput{{ProductID: prod, Qty: pesos("1")}},
	})
	if err != nil {
		t.Fatalf("un mesero no pudo capturar un descuento: %v", err)
	}
	if !ord.Total.Equal(pesos("90")) {
		t.Fatalf("total: %s", ord.Total)
	}
}

// --- El endpoint, por el ROUTER real ----------------------------------------------------------
//
// Los tests de arriba entran por el servicio y cubren la regla. Lo que SOLO se ve aquí es el
// cableado: que la ruta esté detrás de la autenticación, que siga alcanzable para el rol más común
// del local —que es la decisión de FR-012, no un descuido— y que tenga su tope por usuario. Mover
// la ruta fuera de su grupo no rompe ningún test de servicio, y este endpoint baja totales.

// El router de estos tests trae BROKER, a diferencia del de los folios: este handler avisa por SSE
// al cambiar el descuento, y sin broker el test revienta con un nil en vez de probar la ruta.
func nuevaAPIDeDescuento(t *testing.T) (http.Handler, *store.Store, func(username, role string, empresa int64) string) {
	t.Helper()
	st := newTestStore(t)
	jm := auth.NewManager("secreto-de-pruebas-suficientemente-largo-para-el-manager", nil)
	h := httpapi.NewHandlers(httpapi.Deps{
		JWT: jm, Orders: app.NewOrdersService(st, clock), Broker: realtime.NewBroker(),
	})
	r := httpapi.Router(config.Config{}, jm, h, st)

	token := func(username, role string, empresa int64) string {
		id := makeUserIn(t, st, empresa, username, role)
		tok, err := jm.Issue(domain.User{ID: id, CompanyID: empresa, Name: username, Role: domain.Role(role)})
		if err != nil {
			t.Fatalf("Issue(%s): %v", username, err)
		}
		return tok
	}
	return r, st, token
}

func putDescuento(t *testing.T, r http.Handler, tok string, id int64, cuerpo string) *httptest.ResponseRecorder {
	t.Helper()
	return do(t, r, http.MethodPut, fmt.Sprintf("/api/v1/orders/%d/discount", id), tok, []byte(cuerpo), "application/json")
}

func pedidoParaLaRuta(t *testing.T, st *store.Store, sufijo string) int64 {
	t.Helper()
	svc := app.NewOrdersService(st, clock)
	cajero := makeUser(t, st, "cajero_ruta_"+sufijo, "cajero")
	prod := makeProduct(t, st, "Crepa ruta "+sufijo, pesos("200"), false)
	abrirCajaPrincipal(t, st, cajero)
	ord, err := svc.Create(context.Background(), app.CreateOrderCmd{
		ClientUUID: uuid.New(), ServiceType: "mostrador", OpenedBy: cajero,
		Lines: []domain.OrderLineInput{{ProductID: prod, Qty: pesos("1")}},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	return ord.ID
}

func TestElEndpointDelDescuentoExigeAutenticacion(t *testing.T) {
	r, st, _ := nuevaAPIDeDescuento(t)
	id := pedidoParaLaRuta(t, st, "sin_token")

	if got := putDescuento(t, r, "", id, `{"discountAmount":50}`).Code; got != http.StatusUnauthorized {
		t.Fatalf("sin token se pudo descontar (status %d): la ruta quedó fuera de RequireAuth", got)
	}
}

// FR-012 por la RUTA, no solo por el servicio: descontar no pide rol a propósito, y un
// `RequireRole` agregado después dejaría al mostrador sin poder aplicar la promoción de una
// plataforma a media tarde.
func TestUnMeseroDescuentaPorLaRuta(t *testing.T) {
	r, st, token := nuevaAPIDeDescuento(t)
	id := pedidoParaLaRuta(t, st, "mesero")
	tok := token("mesero_ruta_desc", "mesero", defaultCompanyID)

	if got := putDescuento(t, r, tok, id, `{"discountAmount":50}`).Code; got != http.StatusOK {
		t.Fatalf("el mesero no pudo descontar (status %d)", got)
	}
}

func TestElDescuentoTieneTopePorUsuario(t *testing.T) {
	r, st, token := nuevaAPIDeDescuento(t)
	id := pedidoParaLaRuta(t, st, "rafaga")
	tok := token("cajero_rafaga_desc", "cajero", defaultCompanyID)

	visto429 := false
	for i := 0; i < 200; i++ {
		if putDescuento(t, r, tok, id, `{"discountAmount":1}`).Code == http.StatusTooManyRequests {
			visto429 = true
			break
		}
	}
	if !visto429 {
		t.Fatal("200 cambios de descuento seguidos del mismo usuario no toparon con el limitador: " +
			"la ruta quedó fuera del grupo con rateLimitUser, y cada vuelta escribe la fila del " +
			"pedido y avisa por SSE a todas las tabletas")
	}
}

// QUITAR UN DESCUENTO BORRA SU PROPIO RASTRO, y el evento es lo único que lo conserva.
//
// `discount_set_by` se sobrescribe en sitio y se limpia a NULL al quitar el descuento: la fila queda
// indistinguible de un pedido que nunca tuvo uno. Sin este evento, alguien puede descontar el 100 %,
// entregar la comida y quitar el descuento, y no queda una sola huella de que pasó.
func TestQuitarUnDescuentoDejaElEventoConElMontoAnterior(t *testing.T) {
	r, st, token := nuevaAPIDeDescuento(t)
	id := pedidoParaLaRuta(t, st, "evento")
	tok := token("cajero_evento_desc", "cajero", defaultCompanyID)

	var bitacora bytes.Buffer
	anterior := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&bitacora, &slog.HandlerOptions{Level: slog.LevelWarn})))
	defer slog.SetDefault(anterior)

	if got := putDescuento(t, r, tok, id, `{"discountAmount":80}`).Code; got != http.StatusOK {
		t.Fatalf("aplicar el descuento: status %d", got)
	}
	if got := putDescuento(t, r, tok, id, `{}`).Code; got != http.StatusOK {
		t.Fatalf("quitar el descuento: status %d", got)
	}

	log := bitacora.String()
	if !strings.Contains(log, "order_discount_set") {
		t.Fatal("no quedó ningún evento de seguridad: quitar el descuento dejó la fila igual que " +
			"la de un pedido que nunca tuvo uno, y el rastro es TODO el control de esta feature")
	}
	if !strings.Contains(log, `"descuento_anterior":"80.00"`) {
		t.Fatalf("el evento no dice cuánto se había descontado antes; sin eso no se reconstruye "+
			"qué se quitó. Bitácora: %s", log)
	}
}

// LOS CUATRO CHECKS DE LA MIGRACIÓN, cada uno con el estado imposible que cierra.
//
// Un check sin test es una línea de DDL que nadie nota si desaparece en el siguiente `alter table`:
// el esquema deja de proteger y ninguna prueba cambia de color.
func TestLosChecksDelDescuentoRechazanLoImposible(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	svc := app.NewOrdersService(st, clock)
	ord, cajero := pedidoConDescuento(t, st, svc, "checks", "300", app.CreateOrderCmd{})

	casos := []struct {
		nombre string
		sql    string
		daño   string
	}{
		{
			"descuento negativo", `update orders set discount_total = -10 where id = $1`,
			"un descuento negativo SUBE el total: se le cobraría de más al cliente y el ticket diría otra cosa",
		},
		{
			"total negativo", `update orders set total = -1 where id = $1`,
			"un total negativo es devolverle dinero a alguien sin que nadie lo haya autorizado",
		},
		{
			"rastro a medias",
			`update orders set discount_total = 50, discount_set_by = ` + fmt.Sprint(cajero) + `, discount_set_at = null where id = $1`,
			"un descuento con autor pero sin fecha: no se puede saber si fue antes o después del corte",
		},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			if _, err := st.Pool.Exec(ctx, c.sql, ord.ID); err == nil {
				t.Fatalf("Postgres lo aceptó y no debería: %s", c.daño)
			}
		})
	}
}
