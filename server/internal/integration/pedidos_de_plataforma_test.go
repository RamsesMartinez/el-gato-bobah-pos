//go:build integration

package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/auth"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/config"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/httpapi"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
)

// decisorFalso sustituye a la plataforma. No hay red: lo que se prueba es el servicio.
type decisorFalso struct {
	detalle []byte
	falla   error
}

func (d *decisorFalso) TraerDetalleDePedido(ctx context.Context, liga string) ([]byte, error) {
	if d.falla != nil {
		return nil, d.falla
	}
	return d.detalle, nil
}
func (d *decisorFalso) AceptarPedido(ctx context.Context, pedidoID, ref string) error { return nil }
func (d *decisorFalso) RechazarPedido(ctx context.Context, pedidoID, motivo, expl string) error {
	return nil
}
func (d *decisorFalso) ClaseDeFallo(err error) domain.ClaseDeFallo {
	return domain.FalloRespuestaInvalida
}

const detalleDeUnPedido = `{
  "id":"ped-1","display_id":"K4T2","placed_at":"2026-09-17T18:04:00Z","type":"DELIVERY",
  "eater":{"first_name":"Ana"},
  "payment":{"charges":{"total":{"amount":34200}}},
  "cart":{"items":[{"id":"crepa","title":"Crepa de Nutella","quantity":2,
                    "price":{"unit_price":{"amount":12900}}}]}}`

// tiendaConLlave deja una empresa lista para recibir avisos: su plataforma, su conexión y su llave
// de firma.
func tiendaConLlave(t *testing.T, st *store.Store, empresa int64, tienda, llave string) int64 {
	t.Helper()
	ctx := context.Background()
	plataforma := platformID(t, st, empresa, "Uber Eats")
	var conexion int64
	if err := st.Pool.QueryRow(ctx, `
		insert into platform_connections (delivery_platform_id, external_store_id, label, company_id)
		values ($1, $2, $3, $4) returning id`,
		plataforma, tienda, "Sucursal "+tienda, empresa).Scan(&conexion); err != nil {
		t.Fatalf("crear la conexión: %v", err)
	}
	if _, err := st.Pool.Exec(ctx, `
		insert into platform_webhook_keys (delivery_platform_id, key_primary, company_id)
		values ($1, $2, $3)`, plataforma, llave, empresa); err != nil {
		t.Fatalf("guardar la llave: %v", err)
	}
	// Los métodos de cobro de la plataforma, que en producción los siembra CrearConexion. Se
	// replican aquí porque este helper inserta la conexión directo, sin pasar por el servicio.
	if _, err := st.Pool.Exec(ctx, `
		insert into payment_methods (company_id, name, kind, delivery_platform_id, is_cash,
		                             affects_cash_drawer, is_active, sort_key, auto_declare)
		values ($1, 'Uber Eats en línea', 'plataforma', $2, false, false, true, 400, false),
		       ($1, 'Uber Eats efectivo', 'plataforma', $2, true, true, true, 410, false)
		on conflict (company_id, name) do nothing`, empresa, plataforma); err != nil {
		t.Fatalf("sembrar los métodos de la plataforma: %v", err)
	}
	return conexion
}

func avisoDePedido(eventID, tienda string) []byte {
	cuerpo, _ := json.Marshal(map[string]any{
		"event_type":    "orders.notification",
		"event_id":      eventID,
		"meta":          map[string]any{"user_id": tienda, "resource_id": "ped-1"},
		"resource_href": "https://test-api.uber.com/v1/eats/order/ped-1",
	})
	return cuerpo
}

func servicioDePedidos(st *store.Store) *app.PedidosDePlataformaService {
	return app.NewPedidosDePlataformaService(st,
		map[string]app.DecisorDePedidos{"Uber Eats": &decisorFalso{detalle: []byte(detalleDeUnPedido)}},
		"sandbox", clock)
}

// UN AVISO NO CAE EN LA EMPRESA EQUIVOCADA, NI CON LA MISMA TIENDA REGISTRADA EN LAS DOS.
//
// Es el caso que decidió el diseño. La primera propuesta fue hacer global el único de
// (plataforma, tienda) para poder resolver la empresa sin conocerla; se descartó porque en el
// ambiente de pruebas las plataformas reparten tiendas de demostración COMPARTIDAS, y la segunda
// empresa que quisiera probar chocaría contra el único.
//
// Lo que resuelve la ambigüedad es LA FIRMA: la empresa la determina quién pudo firmar el cuerpo,
// no un dato que cualquiera escribe adentro. Este test es lo único que lo comprueba, y NO se puede
// hacer con un unitario: exige dos empresas y Postgres real.
func TestUnAvisoNoCaeEnLaEmpresaEquivocada(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	const tiendaCompartida = "tienda-demo-de-la-plataforma"
	const llaveA = "llave-de-la-empresa-a-larga"
	const llaveB = "llave-de-la-empresa-b-larga"

	a := makeCompany(t, st, "empresa-a")
	b := makeCompany(t, st, "empresa-b")
	conexionA := tiendaConLlave(t, st, a, tiendaCompartida, llaveA)
	conexionB := tiendaConLlave(t, st, b, tiendaCompartida, llaveB)

	// SE FIRMA CON LA LLAVE DE **B**, LA SEGUNDA REGISTRADA, a propósito: si el servicio se
	// quedara con la primera fila que devuelve la consulta —que es el error fácil— este caso lo
	// atrapa y el inverso no. Firmar con la de A lo dejaría pasar por el orden de las filas.
	svc := servicioDePedidos(st)
	cuerpo := avisoDePedido("evt-de-b", tiendaCompartida)
	err := svc.RecibirAviso(ctx, "Uber Eats", app.AvisoEntrante{
		Crudo: cuerpo, Firma: domain.FirmarParaPrueba(cuerpo, llaveB), Ambiente: "sandbox",
	})
	if err != nil {
		t.Fatalf("un aviso firmado con la llave de B no entró: %v", err)
	}

	contar := func(conexion int64) int {
		var n int
		if err := st.Pool.QueryRow(ctx,
			`select count(*) from platform_incoming_orders where connection_id = $1`, conexion).Scan(&n); err != nil {
			t.Fatalf("contar pedidos: %v", err)
		}
		return n
	}
	if got := contar(conexionB); got != 1 {
		t.Fatalf("la empresa B, que es quien firmó, tiene %d pedidos y debería tener 1", got)
	}
	if got := contar(conexionA); got != 0 {
		t.Fatalf("EL PEDIDO CAYÓ EN LA EMPRESA A: tiene %d pedidos y el aviso lo firmó B. "+
			"La tienda es la misma en las dos; lo único que las distingue es quién pudo firmar, "+
			"y quedarse con la primera fila de la consulta es exactamente el error que esto atrapa", got)
	}
}

// UNA FIRMA QUE NO CUADRA NO ESCRIBE NADA, y el error es el MISMO que cuando la tienda no existe:
// distinguirlos le diría a quien prueba a ciegas cuándo va acertando el id de una tienda real.
func TestUnAvisoSinFirmaValidaNoEscribeNada(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	empresa := makeCompany(t, st, "empresa-firma")
	tiendaConLlave(t, st, empresa, "tienda-1", "la-llave-buena-y-larga-aqui")
	svc := servicioDePedidos(st)

	cuerpo := avisoDePedido("evt-malo", "tienda-1")
	casos := []struct {
		nombre string
		firma  string
	}{
		{"sin firma", ""},
		{"firma inventada", "00ff"},
		{"firmado con otra llave", domain.FirmarParaPrueba(cuerpo, "otra-llave-cualquiera-larga")},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			err := svc.RecibirAviso(ctx, "Uber Eats", app.AvisoEntrante{
				Crudo: cuerpo, Firma: c.firma, Ambiente: "sandbox",
			})
			if !errors.Is(err, domain.ErrFirmaInvalida) {
				t.Fatalf("se esperaba ErrFirmaInvalida y llegó: %v", err)
			}
		})
	}

	// Y una tienda que no conocemos: MISMO error, para no delatar cuáles existen.
	cuerpoAjeno := avisoDePedido("evt-ajeno", "tienda-que-no-es-nuestra")
	err := svc.RecibirAviso(ctx, "Uber Eats", app.AvisoEntrante{
		Crudo: cuerpoAjeno, Firma: domain.FirmarParaPrueba(cuerpoAjeno, "la-llave-buena-y-larga-aqui"),
		Ambiente: "sandbox",
	})
	if !errors.Is(err, domain.ErrFirmaInvalida) {
		t.Fatalf("una tienda desconocida dio un error distinto y eso delata cuáles existen: %v", err)
	}

	var avisos int
	if err := st.Pool.QueryRow(ctx, `select count(*) from platform_webhook_events`).Scan(&avisos); err != nil {
		t.Fatal(err)
	}
	if avisos != 0 {
		t.Fatalf("se escribieron %d avisos que no estaban autenticados", avisos)
	}
}

// EL MISMO AVISO CINCO VECES ES UN PEDIDO. La plataforma reintenta lo que no se le confirma y puede
// entregar dos veces lo mismo. Un duplicado hace que la cocina prepare dos veces, y no truena por
// ningún lado: se descubre cuando el cliente reclama.
func TestElMismoAvisoCincoVecesEsUnPedido(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	empresa := makeCompany(t, st, "empresa-dedup")
	conexion := tiendaConLlave(t, st, empresa, "tienda-dedup", "llave-para-deduplicar-larga")
	svc := servicioDePedidos(st)

	cuerpo := avisoDePedido("evt-repetido", "tienda-dedup")
	firma := domain.FirmarParaPrueba(cuerpo, "llave-para-deduplicar-larga")
	for i := 0; i < 5; i++ {
		if err := svc.RecibirAviso(ctx, "Uber Eats", app.AvisoEntrante{
			Crudo: cuerpo, Firma: firma, Ambiente: "sandbox",
		}); err != nil {
			t.Fatalf("entrega %d: %v", i+1, err)
		}
	}

	var pedidos, renglones int
	if err := st.Pool.QueryRow(ctx,
		`select count(*) from platform_incoming_orders where connection_id = $1`, conexion).Scan(&pedidos); err != nil {
		t.Fatal(err)
	}
	if err := st.Pool.QueryRow(ctx, `select count(*) from platform_incoming_order_lines`).Scan(&renglones); err != nil {
		t.Fatal(err)
	}
	if pedidos != 1 {
		t.Fatalf("cinco entregas del MISMO aviso dejaron %d pedidos: la cocina prepararía %d veces", pedidos, pedidos)
	}
	if renglones != 1 {
		t.Fatalf("cinco entregas dejaron %d renglones y el pedido tiene 1", renglones)
	}
}

// UN AVISO DE OTRO AMBIENTE NO SE PROCESA. Mezclar un pedido real con datos de prueba es el tipo de
// error que nadie detecta hasta que alguien cobra algo que no existió.
func TestUnAvisoDeProduccionNoEntraAlAmbienteDePruebas(t *testing.T) {
	st := newTestStore(t)
	empresa := makeCompany(t, st, "empresa-ambiente")
	tiendaConLlave(t, st, empresa, "tienda-amb", "llave-del-ambiente-de-pruebas")
	svc := servicioDePedidos(st)

	cuerpo := avisoDePedido("evt-prod", "tienda-amb")
	err := svc.RecibirAviso(context.Background(), "Uber Eats", app.AvisoEntrante{
		Crudo: cuerpo, Firma: domain.FirmarParaPrueba(cuerpo, "llave-del-ambiente-de-pruebas"),
		Ambiente: "production",
	})
	if !errors.Is(err, domain.ErrAmbienteEquivocado) {
		t.Fatalf("un aviso de producción entró al ambiente de pruebas: %v", err)
	}
}

// UN DETALLE QUE NO SE PUDO TRAER NO SE CONFIRMA, y eso es lo que hace que la plataforma reintente.
// Confirmar lo que falló pierde el pedido en silencio: el cliente espera comida que nadie hace.
func TestUnDetalleQueNoSeTraeNoSeConfirma(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	empresa := makeCompany(t, st, "empresa-sin-detalle")
	tiendaConLlave(t, st, empresa, "tienda-sd", "llave-sin-detalle-suficiente")
	svc := app.NewPedidosDePlataformaService(st,
		map[string]app.DecisorDePedidos{"Uber Eats": &decisorFalso{falla: errors.New("la plataforma no contestó")}},
		"sandbox", clock)

	cuerpo := avisoDePedido("evt-sin-detalle", "tienda-sd")
	err := svc.RecibirAviso(ctx, "Uber Eats", app.AvisoEntrante{
		Crudo: cuerpo, Firma: domain.FirmarParaPrueba(cuerpo, "llave-sin-detalle-suficiente"),
		Ambiente: "sandbox",
	})
	if err == nil {
		t.Fatal("se confirmó un aviso cuyo detalle no se pudo traer: la plataforma no lo reintentaría " +
			"y el pedido se pierde sin que nada falle")
	}

	// El aviso SÍ queda registrado, con su clase de fallo y sin el mensaje crudo del error.
	var desenlace, clase *string
	if err := st.Pool.QueryRow(ctx,
		`select outcome, failure_kind from platform_webhook_events where event_id = $1`,
		"evt-sin-detalle").Scan(&desenlace, &clase); err != nil {
		t.Fatalf("el aviso no quedó registrado: %v", err)
	}
	if desenlace == nil || *desenlace != "fallido" {
		t.Fatalf("desenlace = %v, se esperaba fallido", desenlace)
	}
	if clase == nil {
		t.Fatal("no se registró la clase del fallo")
	}
	var pedidos int
	if err := st.Pool.QueryRow(ctx, `select count(*) from platform_incoming_orders`).Scan(&pedidos); err != nil {
		t.Fatal(err)
	}
	if pedidos != 0 {
		t.Fatalf("quedaron %d pedidos de un detalle que nunca llegó", pedidos)
	}
}

// LA PUERTA PÚBLICA: qué contesta y qué NO dice.
//
// Es la primera ruta del negocio sin sesión de usuario, así que lo que se prueba aquí no es solo
// que funcione: es que no se le pueda sacar información por tanteo y que un fallo haga reintentar
// a la plataforma en vez de perder el pedido.
func TestLaPuertaPublicaDelWebhook(t *testing.T) {
	st := newTestStore(t)
	const llave = "la-llave-de-la-puerta-publica"
	empresa := makeCompany(t, st, "empresa-puerta")
	tiendaConLlave(t, st, empresa, "tienda-puerta", llave)

	jm := auth.NewManager(secretoDelNegocioEnPruebas, nil)
	h := httpapi.NewHandlers(httpapi.Deps{
		Cfg: config.Config{}, JWT: jm,
		PedidosPlataforma: servicioDePedidos(st),
	})
	r := httpapi.Router(config.Config{}, jm, h, st)

	pegar := func(cuerpo []byte, firma, ambiente string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/uber-eats", bytes.NewReader(cuerpo))
		if firma != "" {
			req.Header.Set("X-Uber-Signature", firma)
		}
		if ambiente != "" {
			req.Header.Set("X-Environment", ambiente)
		}
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w
	}

	bueno := avisoDePedido("evt-puerta", "tienda-puerta")

	t.Run("un aviso firmado entra y contesta 200 con cuerpo vacío", func(t *testing.T) {
		w := pegar(bueno, domain.FirmarParaPrueba(bueno, llave), "sandbox")
		if w.Code != http.StatusOK {
			t.Fatalf("code = %d: %s", w.Code, w.Body.String())
		}
		// La plataforma espera el cuerpo VACÍO. Cualquier otra cosa la hace reintentar.
		if w.Body.Len() != 0 {
			t.Fatalf("contestó con cuerpo %q y debe ir vacío", w.Body.String())
		}
	})

	// EL MISMO CUERPO PARA TODOS LOS FALLOS DE AUTENTICACIÓN. Si difieren, quien prueba a ciegas
	// sabe cuándo acertó el identificador de una tienda real — y eso es lo único que hace falta
	// adivinar para intentarlo.
	t.Run("nada delata si la tienda existe", func(t *testing.T) {
		ajeno := avisoDePedido("evt-ajeno-puerta", "tienda-que-no-existe")
		sinFirma := pegar(bueno, "", "sandbox")
		malaFirma := pegar(bueno, "00ff", "sandbox")
		tiendaAjena := pegar(ajeno, domain.FirmarParaPrueba(ajeno, llave), "sandbox")

		for _, w := range []*httptest.ResponseRecorder{sinFirma, malaFirma, tiendaAjena} {
			if w.Code != http.StatusUnauthorized {
				t.Fatalf("code = %d, se esperaba 401: %s", w.Code, w.Body.String())
			}
		}
		if sinFirma.Body.String() != malaFirma.Body.String() ||
			malaFirma.Body.String() != tiendaAjena.Body.String() {
			t.Fatalf("las tres respuestas difieren y eso se puede usar para tantear:\n  sin firma: %s\n  mala: %s\n  ajena: %s",
				sinFirma.Body.String(), malaFirma.Body.String(), tiendaAjena.Body.String())
		}
	})

	t.Run("un cuerpo enorme se rechaza por tamaño", func(t *testing.T) {
		gordo := append([]byte(`{"relleno":"`), bytes.Repeat([]byte("x"), 128<<10)...)
		gordo = append(gordo, []byte(`"}`)...)
		w := pegar(gordo, domain.FirmarParaPrueba(gordo, llave), "sandbox")
		if w.Code != http.StatusRequestEntityTooLarge {
			t.Fatalf("code = %d, se esperaba 413", w.Code)
		}
	})

	t.Run("una plataforma que no conocemos no existe como ruta", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/rappi", bytes.NewReader(bueno))
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusNotFound {
			t.Fatalf("code = %d, se esperaba 404", w.Code)
		}
	})

	// UN FALLO CONTESTA 5xx Y NO 200, y es la diferencia entre que la plataforma reintente y que el
	// pedido se pierda en silencio.
	t.Run("un detalle que no se pudo traer contesta 5xx para que reintenten", func(t *testing.T) {
		hRoto := httpapi.NewHandlers(httpapi.Deps{
			Cfg: config.Config{}, JWT: jm,
			PedidosPlataforma: app.NewPedidosDePlataformaService(st,
				map[string]app.DecisorDePedidos{"Uber Eats": &decisorFalso{falla: errors.New("caída")}},
				"sandbox", clock),
		})
		rRoto := httpapi.Router(config.Config{}, jm, hRoto, st)
		cuerpo := avisoDePedido("evt-roto-puerta", "tienda-puerta")
		req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/uber-eats", bytes.NewReader(cuerpo))
		req.Header.Set("X-Uber-Signature", domain.FirmarParaPrueba(cuerpo, llave))
		req.Header.Set("X-Environment", "sandbox")
		w := httptest.NewRecorder()
		rRoto.ServeHTTP(w, req)
		if w.Code < 500 {
			t.Fatalf("code = %d: un fallo tiene que contestar 5xx o la plataforma no reintenta y el pedido se pierde", w.Code)
		}
	})
}

// LOS ARREGLOS SALEN COMO `[]` Y NUNCA COMO `null`, Y SE PRUEBA SOBRE EL JSON CRUDO.
//
// Un slice nil de Go se serializa como `null`. La pantalla hace `.filter()` al pintar cada tarjeta,
// así que un `null` la tumba entera **con la API respondiendo 200 y sin una sola línea de error en
// el log**. Ya pasó con la pantalla de pedidos de producción el 2026-09-13.
//
// El test va sobre los BYTES y no sobre una estructura de Go: deserializar borra justo la
// diferencia entre `null` y `[]`, que es lo único que este caso vigila.
func TestLosPedidosPendientesNuncaTraenNullEnSusArreglos(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	const llave = "llave-para-los-pendientes-ok"
	empresa := makeCompany(t, st, "empresa-pendientes")
	tiendaConLlave(t, st, empresa, "tienda-pend", llave)
	svc := servicioDePedidos(st)

	// Un pedido SIN opciones: es el caso que produce el nil, porque el renglón no tiene hijos.
	sinOpciones := `{"id":"ped-sin-op","display_id":"Z9","placed_at":"2026-09-17T18:04:00Z",
	  "type":"DELIVERY","eater":{"first_name":"Beto"},
	  "payment":{"charges":{"total":{"amount":4500}}},
	  "cart":{"items":[{"id":"cafe","title":"Café","quantity":1,
	                    "price":{"unit_price":{"amount":4500}}}]}}`
	svcSinOpciones := app.NewPedidosDePlataformaService(st,
		map[string]app.DecisorDePedidos{"Uber Eats": &decisorFalso{detalle: []byte(sinOpciones)}},
		"sandbox", clock)
	cuerpo := avisoDePedido("evt-sin-opciones", "tienda-pend")
	if err := svcSinOpciones.RecibirAviso(ctx, "Uber Eats", app.AvisoEntrante{
		Crudo: cuerpo, Firma: domain.FirmarParaPrueba(cuerpo, llave), Ambiente: "sandbox",
	}); err != nil {
		t.Fatalf("recibir: %v", err)
	}

	ctxT, soltar, err := st.AcquireTenant(ctx, empresa)
	if err != nil {
		t.Fatalf("tomar el tenant: %v", err)
	}
	defer soltar()
	pendientes, err := svc.Pendientes(ctxT)
	if err != nil {
		t.Fatalf("listar pendientes: %v", err)
	}
	crudo, err := json.Marshal(pendientes)
	if err != nil {
		t.Fatal(err)
	}
	texto := string(crudo)
	if strings.Contains(texto, `"lines":null`) {
		t.Fatalf("`lines` salió como null y eso tumba la pantalla entera con la API en 200: %s", texto)
	}
	if strings.Contains(texto, `"options":null`) {
		t.Fatalf("`options` salió como null: %s", texto)
	}
	if !strings.Contains(texto, `"options":[]`) {
		t.Fatalf("un renglón sin opciones debería traer `options: []`: %s", texto)
	}

	// Y sin ningún pendiente, la lista entera también es `[]`.
	vacio := app.NewPedidosDePlataformaService(newTestStore(t), nil, "sandbox", clock)
	st2 := newTestStore(t)
	otra := makeCompany(t, st2, "empresa-vacia")
	ctx2, soltar2, err := st2.AcquireTenant(ctx, otra)
	if err != nil {
		t.Fatal(err)
	}
	defer soltar2()
	_ = vacio
	svc2 := app.NewPedidosDePlataformaService(st2, nil, "sandbox", clock)
	lista, err := svc2.Pendientes(ctx2)
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := json.Marshal(lista); string(b) != "[]" {
		t.Fatalf("sin pendientes la lista debería ser [] y fue %s", b)
	}
}

// ACEPTAR: EL PEDIDO ENTRA AL POS YA PAGADO POR LA PLATAFORMA.
//
// Es donde vive el dinero de esta feature, y lo que se vigila es el principio III: el total tiene
// que cuadrar al centavo con lo que cobró la plataforma, y el pedido NO puede aparecer como dinero
// por cobrar — la plataforma ya cobró, y pedirlo otra vez en el corte es un faltante inventado.
func TestAceptarDejaElPedidoEnElPOSYaPagado(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	const llave = "llave-para-aceptar-pedidos-ok"
	empresa := makeCompany(t, st, "empresa-acepta")
	usuario := makeUserIn(t, st, empresa, "cajera-acepta", "cajero")
	tiendaConLlave(t, st, empresa, "tienda-acepta", llave)
	svc := servicioDePedidos(st)

	cuerpo := avisoDePedido("evt-aceptar", "tienda-acepta")
	if err := svc.RecibirAviso(ctx, "Uber Eats", app.AvisoEntrante{
		Crudo: cuerpo, Firma: domain.FirmarParaPrueba(cuerpo, llave), Ambiente: "sandbox",
	}); err != nil {
		t.Fatalf("recibir: %v", err)
	}

	ctxT, soltar, err := st.AcquireTenant(ctx, empresa)
	if err != nil {
		t.Fatal(err)
	}
	defer soltar()

	pendientes, err := svc.Pendientes(ctxT)
	if err != nil || len(pendientes) != 1 {
		t.Fatalf("pendientes = %d, err = %v", len(pendientes), err)
	}

	vista, err := svc.Aceptar(ctxT, pendientes[0].ID, usuario)
	if err != nil {
		t.Fatalf("aceptar: %v", err)
	}

	// EL TOTAL CUADRA AL CENTAVO con lo que cobró la plataforma (34200 centavos = $342.00).
	var total, pagado string
	var estado, folio string
	var sesion *int64
	if err := st.Pool.QueryRow(ctx, `
		select o.total::text, o.status::text, o.platform_order_ref, o.register_session_id,
		       coalesce((select sum(amount) from order_payments where order_id = o.id), 0)::text
		  from orders o where o.id = $1`, vista.ID).Scan(&total, &estado, &folio, &sesion, &pagado); err != nil {
		t.Fatalf("leer el pedido creado: %v", err)
	}
	if total != "342.00" {
		t.Fatalf("total = %s, la plataforma cobró 342.00", total)
	}
	if pagado != "342.00" {
		t.Fatalf("pagado = %s: la plataforma YA cobró, y dejarlo por cobrar inventa un faltante en el corte", pagado)
	}
	if folio != "ped-1" {
		t.Fatalf("platform_order_ref = %q, se esperaba el folio de la plataforma", folio)
	}
	// SIN TURNO ABIERTO SE ACEPTA IGUAL: la cocina no espera a que alguien abra caja.
	if sesion != nil {
		t.Fatalf("register_session_id = %v y no había turno abierto", *sesion)
	}

	// El pedido entrante quedó marcado, con quién y cuándo.
	var estadoEntrante string
	var decidioPor *int64
	if err := st.Pool.QueryRow(ctx,
		`select state::text, decided_by from platform_incoming_orders where id = $1`,
		pendientes[0].ID).Scan(&estadoEntrante, &decidioPor); err != nil {
		t.Fatal(err)
	}
	if estadoEntrante != "aceptado" || decidioPor == nil || *decidioPor != usuario {
		t.Fatalf("el entrante quedó %q por %v", estadoEntrante, decidioPor)
	}

	// Y ya no está pendiente: la tableta deja de mostrarlo.
	if p, _ := svc.Pendientes(ctxT); len(p) != 0 {
		t.Fatalf("sigue pendiente después de aceptarlo: %d", len(p))
	}

	// ACEPTAR DOS VECES SE RECHAZA DICIÉNDOLO, no en silencio: dos tabletas pueden tocar el botón
	// al mismo tiempo y quien no gane tiene que enterarse.
	if _, err := svc.Aceptar(ctxT, pendientes[0].ID, usuario); !errors.Is(err, domain.ErrPedidoYaDecidido) {
		t.Fatalf("aceptar dos veces dio: %v", err)
	}
}

// AL ABRIR TURNO, LOS PEDIDOS HUÉRFANOS SE RENUMERAN.
//
// `orders_folio_turno_key` es único por (company_id, register_session_id, daily_number). Mientras el
// pedido no tiene turno su número es 0 y no choca con nada, porque Postgres trata los NULL como
// distintos. Al asignarle el turno, ese 0 entra a competir con los folios reales — y DOS huérfanos
// chocan entre ellos. Un update en bloque habría reventado.
func TestAlAbrirTurnoLosPedidosHuerfanosSeRenumeran(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	const llave = "llave-para-los-huerfanos-ok"
	empresa := makeCompany(t, st, "empresa-huerfanos")
	usuario := makeUserIn(t, st, empresa, "cajera-huerfanos", "cajero")
	tiendaConLlave(t, st, empresa, "tienda-huerf", llave)

	ctxT, soltar, err := st.AcquireTenant(ctx, empresa)
	if err != nil {
		t.Fatal(err)
	}
	defer soltar()

	// DOS pedidos aceptados sin turno: es el caso que choca.
	for i, folio := range []string{"ped-h1", "ped-h2"} {
		detalle := strings.Replace(detalleDeUnPedido, `"id":"ped-1"`, `"id":"`+folio+`"`, 1)
		svc := app.NewPedidosDePlataformaService(st,
			map[string]app.DecisorDePedidos{"Uber Eats": &decisorFalso{detalle: []byte(detalle)}},
			"sandbox", clock)
		cuerpo := avisoDePedido("evt-huerfano-"+itoa(i), "tienda-huerf")
		if err := svc.RecibirAviso(ctx, "Uber Eats", app.AvisoEntrante{
			Crudo: cuerpo, Firma: domain.FirmarParaPrueba(cuerpo, llave), Ambiente: "sandbox",
		}); err != nil {
			t.Fatalf("recibir %s: %v", folio, err)
		}
		pend, err := svc.Pendientes(ctxT)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := svc.Aceptar(ctxT, pend[0].ID, usuario); err != nil {
			t.Fatalf("aceptar %s: %v", folio, err)
		}
	}

	// SE ABRE EL TURNO POR EL CAMINO REAL, no insertando la fila: lo que este caso prueba es que
	// `OpenSession` reclama los huérfanos, y un insert directo se saltaría justo eso.
	var principal int64
	if err := st.Pool.QueryRow(ctxT,
		`select id from cash_registers where is_primary and is_active limit 1`).Scan(&principal); err != nil {
		t.Fatalf("caja principal: %v", err)
	}
	vista, err := app.NewBackofficeService(st, clock).OpenSession(ctxT, principal, app.AperturaCmd{}, usuario)
	if err != nil {
		t.Fatalf("abrir turno: %v", err)
	}
	sesion := vista.ID

	filas, err := st.Pool.Query(ctx, `
		select daily_number, register_session_id from orders
		 where delivery_platform_id is not null and company_id = $1 order by id`, empresa)
	if err != nil {
		t.Fatal(err)
	}
	defer filas.Close()
	numeros := map[int32]bool{}
	for filas.Next() {
		var num int32
		var ses *int64
		if err := filas.Scan(&num, &ses); err != nil {
			t.Fatal(err)
		}
		if ses == nil || *ses != sesion {
			t.Fatalf("un pedido quedó sin reclamar por el turno: sesión = %v", ses)
		}
		if numeros[num] {
			t.Fatalf("DOS PEDIDOS CON EL MISMO FOLIO %d en el mismo turno: el único "+
				"orders_folio_turno_key no lo habría permitido, y un update en bloque revienta aquí", num)
		}
		numeros[num] = true
	}
	if len(numeros) != 2 {
		t.Fatalf("se reclamaron %d pedidos y deberían ser 2", len(numeros))
	}
}

// EL WEBHOOK BAJO EL ROL DE LA APLICACIÓN, que es como corre producción.
//
// Todo lo demás de este archivo usa `newTestStore`, que conecta como OWNER y por lo tanto SALTA
// RLS. En producción la API se conecta como `gatobobah_app` —`config.Validate` lo exige— y ahí las
// políticas sí aplican.
//
// `resolverTienda` corre por el pool SIN empresa fijada, a propósito: la empresa es el resultado.
// Pero la política `tenant_isolation` de `platform_connections` compara contra
// `current_setting('app.company_id', true)`, que sin tenant es NULL — y `company_id = NULL` nunca
// es verdadero. Bajo el rol de la aplicación, la consulta devuelve CERO filas.
//
// El síntoma no es un error: es un 401 a cada aviso. La plataforma reintenta siete veces, se rinde,
// cancela el pedido, y en el log solo queda «firma no autenticada». Nadie sospecha de RLS.
func TestElWebhookResuelveLaTiendaBajoElRolDeLaAplicacion(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	const llave = "llave-bajo-el-rol-de-la-app"
	empresa := makeCompany(t, st, "empresa-rol-app")
	tiendaConLlave(t, st, empresa, "tienda-rol-app", llave)

	// EL MISMO servicio, pero con el store del rol de aplicación: es el único cambio.
	appSt := appRoleStore(t)
	svc := app.NewPedidosDePlataformaService(appSt,
		map[string]app.DecisorDePedidos{"Uber Eats": &decisorFalso{detalle: []byte(detalleDeUnPedido)}},
		"sandbox", clock)

	cuerpo := avisoDePedido("evt-rol-app", "tienda-rol-app")
	err := svc.RecibirAviso(ctx, "Uber Eats", app.AvisoEntrante{
		Crudo: cuerpo, Firma: domain.FirmarParaPrueba(cuerpo, llave), Ambiente: "sandbox",
	})
	if errors.Is(err, domain.ErrFirmaInvalida) {
		t.Fatal("bajo el rol de la aplicación el aviso se rechazó como NO AUTENTICADO: " +
			"RLS bloqueó la consulta que resuelve la tienda, así que no hubo ninguna candidata " +
			"contra la cual verificar la firma. En producción NINGÚN pedido entraría, y el log " +
			"solo diría «firma no autenticada»")
	}
	if err != nil {
		t.Fatalf("recibir bajo el rol de la aplicación: %v", err)
	}
}
