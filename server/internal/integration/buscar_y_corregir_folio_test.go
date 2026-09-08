//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"testing"
	"time"
	"uuid"

	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/auth"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/config"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/httpapi"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
)

// La US-2: encontrar un pedido por su folio, ver los que no lo tienen, y completarlos.

func filtroBase(hoy time.Time) domain.SalesFilter {
	d := domain.BusinessDate(hoy, time.UTC)
	return domain.SalesFilter{
		Range: domain.Range{From: d, To: d},
		Sort:  "fecha", Dir: "desc", Limit: 50,
	}
}

// tresPedidosDePlataforma siembra dos con folio y uno sin, más uno de mostrador. Es el escenario
// mínimo en el que el filtro y la búsqueda pueden equivocarse de conjunto.
func tresPedidosDePlataforma(t *testing.T, ctx context.Context, svc *app.OrdersService, cajero, prod int64, uber int16) (conFolio, sinFolio int64) {
	t.Helper()
	folio := "UBER-BUSCAR-001"
	a, err := svc.Create(ctx, app.CreateOrderCmd{
		ClientUUID: uuid.New(), ServiceType: "domicilio", DeliveryPlatformID: &uber,
		OpenedBy: cajero, PlatformOrderRef: &folio,
		Lines: []domain.OrderLineInput{{ProductID: prod, Qty: decimal.RequireFromString("1")}},
	})
	if err != nil {
		t.Fatalf("pedido con folio: %v", err)
	}
	b, err := svc.Create(ctx, app.CreateOrderCmd{
		ClientUUID: uuid.New(), ServiceType: "domicilio", DeliveryPlatformID: &uber,
		OpenedBy: cajero,
		Lines:    []domain.OrderLineInput{{ProductID: prod, Qty: decimal.RequireFromString("1")}},
	})
	if err != nil {
		t.Fatalf("pedido sin folio: %v", err)
	}
	if _, err := svc.Create(ctx, app.CreateOrderCmd{
		ClientUUID: uuid.New(), ServiceType: "mostrador", OpenedBy: cajero,
		Lines: []domain.OrderLineInput{{ProductID: prod, Qty: decimal.RequireFromString("1")}},
	}); err != nil {
		t.Fatalf("pedido de mostrador: %v", err)
	}
	return a.ID, b.ID
}

func TestBuscarPegandoElFolioDelDocumentoDevuelveEsePedido(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	orders := app.NewOrdersService(st, clock)
	sales := app.NewSalesService(st, clock)

	cajero := makeUser(t, st, "cajero_buscar", "cajero")
	prod := makeProduct(t, st, "Crepa buscar", decimal.RequireFromString("100"), false)
	uber := platformID(t, st, defaultCompanyID, "Uber Eats")
	abrirCajaPrincipal(t, st, cajero)
	conFolio, _ := tresPedidosDePlataforma(t, ctx, orders, cajero, prod, uber)

	f := filtroBase(fixedNow)
	f.Folio = "UBER-BUSCAR-001"
	pagina, err := sales.List(ctx, f)
	if err != nil {
		t.Fatalf("List buscando: %v", err)
	}
	if len(pagina.Items) != 1 || pagina.Items[0].ID != conFolio {
		t.Fatalf("pegar el folio del documento devolvió %d pedidos: es lo que SC-002 pide resolver "+
			"en un solo paso, sin comparar montos ni fechas", len(pagina.Items))
	}

	// El resumen describe EL MISMO conjunto, y por construcción: sale de la misma fila.
	resumen, err := sales.Summary(ctx, f)
	if err != nil {
		t.Fatalf("Summary buscando: %v", err)
	}
	if resumen.Count != 1 {
		t.Fatalf("la lista trae 1 pedido y el resumen cuenta %d: quien lo lee no tiene forma de "+
			"saber cuál de los dos miente", resumen.Count)
	}
}

func TestLaBusquedaNoEncuentraPorNumeroNiPorNombreInterno(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	orders := app.NewOrdersService(st, clock)
	sales := app.NewSalesService(st, clock)

	cajero := makeUser(t, st, "cajero_confusion", "cajero")
	prod := makeProduct(t, st, "Waffle confusion", decimal.RequireFromString("100"), false)
	uber := platformID(t, st, defaultCompanyID, "Uber Eats")
	abrirCajaPrincipal(t, st, cajero)
	conFolio, _ := tresPedidosDePlataforma(t, ctx, orders, cajero, prod, uber)

	var numero int32
	var nombre *string
	if err := st.Pool.QueryRow(ctx,
		`select daily_number, folio_name from orders where id = $1`, conFolio).Scan(&numero, &nombre); err != nil {
		t.Fatalf("leer el pedido: %v", err)
	}

	// "Folio" ya significa dos cosas en esta pantalla: el número del turno y el nombre cantable. El
	// buscador es SOLO del folio de plataforma; mezclarlos haría que teclear 187 devolviera el
	// pedido #187 y además cualquier folio de Rappi que contenga 187.
	for _, q := range []string{fmt.Sprint(numero), derefONada(nombre)} {
		if q == "" {
			continue
		}
		f := filtroBase(fixedNow)
		f.Folio = q
		pagina, err := sales.List(ctx, f)
		if err != nil {
			t.Fatalf("List buscando %q: %v", q, err)
		}
		if len(pagina.Items) != 0 {
			t.Fatalf("buscar %q devolvió %d pedidos: el buscador es del folio de PLATAFORMA, no del "+
				"número ni del nombre del turno", q, len(pagina.Items))
		}
	}
}

func TestUnFolioQueNadieCapturoDevuelveVacioYNoUnError(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	sales := app.NewSalesService(st, clock)

	f := filtroBase(fixedNow)
	f.Folio = "RAPPI-QUE-NO-EXISTE"
	pagina, err := sales.List(ctx, f)
	if err != nil {
		t.Fatalf("buscar un folio no capturado devolvió error: es una respuesta legítima —ese "+
			"renglón del documento todavía no se registró—, no un fallo: %v", err)
	}
	if len(pagina.Items) != 0 {
		t.Fatalf("devolvió %d pedidos para un folio inexistente", len(pagina.Items))
	}
}

func TestLaListaYElResumenDescribenElMismoConjunto(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	orders := app.NewOrdersService(st, clock)
	sales := app.NewSalesService(st, clock)

	cajero := makeUser(t, st, "cajero_conjunto", "cajero")
	prod := makeProduct(t, st, "Malteada conjunto", decimal.RequireFromString("100"), false)
	uber := platformID(t, st, defaultCompanyID, "Uber Eats")
	abrirCajaPrincipal(t, st, cajero)
	_, sinFolio := tresPedidosDePlataforma(t, ctx, orders, cajero, prod, uber)

	f := filtroBase(fixedNow)
	f.FolioPlataforma = "pendiente"

	pagina, err := sales.List(ctx, f)
	if err != nil {
		t.Fatalf("List pendientes: %v", err)
	}
	if len(pagina.Items) != 1 || pagina.Items[0].ID != sinFolio {
		t.Fatalf("el filtro de pendientes trajo %d pedidos y debía traer solo el de plataforma sin "+
			"folio; el de mostrador y el que ya tiene folio no son pendientes", len(pagina.Items))
	}

	resumen, err := sales.Summary(ctx, f)
	if err != nil {
		t.Fatalf("Summary pendientes: %v", err)
	}
	// Si el filtro se le olvidó a UNA de las cinco consultas, esto es lo que lo dice. Ya costó un
	// turno con $4,500 de faltante sin explicación.
	if int64(resumen.Count) != pagina.Total {
		t.Fatalf("la lista dice %d ventas y el resumen dice %d: una de las cinco consultas se quedó "+
			"sin el filtro, y quien lee la pantalla no tiene forma de saber cuál mitad miente",
			pagina.Total, resumen.Count)
	}
	// Y el total de dinero también sale del mismo conjunto.
	if !resumen.Total.Equal(pagina.Items[0].Total) {
		t.Fatalf("el resumen suma %s y el único pedido de la lista vale %s", resumen.Total, pagina.Items[0].Total)
	}
}

// Escribirle el folio a un pedido de un arqueo YA CERRADO no mueve ninguna cifra.
//
// Corre contra la base RESTAURADA de producción, no sobre datos sembrados: lo que se está afirmando
// es sobre el histórico real —pedidos viejos sin nombre de folio, sin sesión de caja, con la fecha
// corregida— y eso es justo lo que una siembra no produce.
func TestCorregirElFolioNoMueveNingunaCifra(t *testing.T) {
	st := restoredStore(t)
	migrarArriba(t, st.Pool)
	ctx := context.Background()

	// La fotografía: cada cifra agregada de dinero que el sistema sabe calcular.
	foto := func() map[string]string {
		m := map[string]string{}
		for nombre, sql := range map[string]string{
			"ventas totales":       `select coalesce(sum(total),0)::text from orders`,
			"ventas no canceladas": `select coalesce(sum(total),0)::text from orders where status not in ('cancelada','reembolsada')`,
			"cobrado":              `select coalesce(sum(amount),0)::text from order_payments`,
			"propinas":             `select coalesce(sum(tip_amount),0)::text from order_payments`,
			"envios":               `select coalesce(sum(delivery_fee),0)::text from orders`,
			"reembolsado":          `select coalesce(sum(refund_amount),0)::text from orders`,
			"arqueos declarados":   `select coalesce(sum(declared),0)::text from register_session_totals`,
			"arqueos esperados":    `select coalesce(sum(expected),0)::text from register_session_totals`,
			"propinas de arqueo":   `select coalesce(sum(tips),0)::text from register_session_totals`,
			"movimientos de caja":  `select coalesce(sum(amount),0)::text from register_cash_movements`,
			"pedidos":              `select count(*)::text from orders`,
			"pagos":                `select count(*)::text from order_payments`,
		} {
			var v string
			if err := st.Pool.QueryRow(ctx, sql).Scan(&v); err != nil {
				t.Fatalf("fotografiar %q: %v", nombre, err)
			}
			m[nombre] = v
		}
		return m
	}

	plataforma := plataformaDe(t, st, defaultCompanyID)
	pedido := nuevoPedidoDePlataforma(t, st, defaultCompanyID, plataforma, 9100)

	antes := foto()

	orders := app.NewOrdersService(st, clock)
	if _, err := orders.SetPlatformRef(ctx, pedido, "  UBER-CORREGIDO-1  ", quienSea(t, st, defaultCompanyID)); err != nil {
		t.Fatalf("SetPlatformRef: %v", err)
	}

	despues := foto()
	for nombre, v := range antes {
		if despues[nombre] != v {
			// El mensaje NOMBRA la cifra que se movió, no "esperaba X obtuve Y".
			t.Fatalf("escribir un folio movió %q: era %s y quedó en %s. Corregir un folio no puede "+
				"tocar dinero, y un arqueo cerrado menos", nombre, v, despues[nombre])
		}
	}
}

func quienSea(t *testing.T, st *store.Store, empresa int64) int64 {
	t.Helper()
	var id int64
	if err := st.Pool.QueryRow(context.Background(),
		`select id from users where company_id = $1 order by id limit 1`, empresa).Scan(&id); err != nil {
		t.Fatalf("buscar un usuario de la empresa %d: %v", empresa, err)
	}
	return id
}

func derefONada(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// --- El endpoint de corrección, por el ROUTER real -------------------------------------------
//
// Los tests de servicio ya cubren la regla. Lo que solo se ve aquí es el CABLEADO: que la ruta quedó
// con su gate de rol y con el tope por usuario, y que un pedido de otra empresa no se alcanza. Mover
// la ruta fuera de su grupo no rompe ningún test de servicio.

func nuevaAPIDeFolio(t *testing.T) (http.Handler, *store.Store, func(username, role string, empresa int64) string) {
	t.Helper()
	st := newTestStore(t)
	jm := auth.NewManager("secreto-de-pruebas-suficientemente-largo-para-el-manager", nil)
	h := httpapi.NewHandlers(httpapi.Deps{JWT: jm, Orders: app.NewOrdersService(st, clock)})
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

func patchFolio(t *testing.T, r http.Handler, tok string, id int64, folio string) int {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"platformOrderRef": folio})
	return do(t, r, http.MethodPatch, fmt.Sprintf("/api/v1/orders/%d/platform-ref", id), tok, body, "application/json").Code
}

// El pedido de OTRA empresa no se alcanza, y lo único que lo cierra es RLS.
//
// Corre bajo `appRoleStore` y no por el router del harness a propósito: la API de los tests se
// conecta como OWNER, que salta RLS, así que ahí este caso pasaría en verde con la policy borrada.
// En producción la API usa APP_DATABASE_URL (rol `gatobobah_app`) y RequireAuth fija el tenant con
// AcquireTenant — que es exactamente lo que se reproduce aquí.
func TestElFolioDeOtraEmpresaNoSeAlcanza(t *testing.T) {
	ctx := context.Background()
	owner := newTestStore(t)
	otra := makeCompany(t, owner, "otra-folio-014")

	cajeroA := makeUser(t, owner, "cajero_a_014", "cajero")
	prod := makeProduct(t, owner, "Pan 014", decimal.RequireFromString("100"), false)
	uber := platformID(t, owner, defaultCompanyID, "Uber Eats")
	abrirCajaPrincipal(t, owner, cajeroA)

	ord, err := app.NewOrdersService(owner, clock).Create(ctx, app.CreateOrderCmd{
		ClientUUID: uuid.New(), ServiceType: "domicilio", DeliveryPlatformID: &uber, OpenedBy: cajeroA,
		Lines: []domain.OrderLineInput{{ProductID: prod, Qty: decimal.RequireFromString("1")}},
	})
	if err != nil {
		t.Fatalf("crear el pedido de la empresa A: %v", err)
	}
	cajeroB := makeUserIn(t, owner, otra, "cajero_b_014", "cajero")

	// El servicio, conectado como el rol de la app y con el tenant de la empresa B.
	st := appRoleStore(t)
	tenantCtx, release, err := st.AcquireTenant(ctx, otra)
	if err != nil {
		t.Fatalf("AcquireTenant de la empresa B: %v", err)
	}
	defer release()

	_, err = app.NewOrdersService(st, clock).SetPlatformRef(tenantCtx, ord.ID, "UBER-AJENO-1", cajeroB)
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("la empresa B alcanzó el pedido de la A (err=%v): tiene que ser NO ENCONTRADO, y no "+
			"un 'sin permisos' — decir que no tiene permiso confirmaría que ese pedido existe", err)
	}

	// Y no lo tocó.
	var folio *string
	if err := owner.Pool.QueryRow(ctx,
		`select platform_order_ref from orders where id = $1`, ord.ID).Scan(&folio); err != nil {
		t.Fatalf("releer el pedido: %v", err)
	}
	if folio != nil {
		t.Fatalf("el pedido de la empresa A quedó con folio %q escrito desde la B", *folio)
	}
}

func TestElFolioSeCorrigePorElRouterConSuGateYSuTope(t *testing.T) {
	ctx := context.Background()
	r, st, token := nuevaAPIDeFolio(t)

	cajero := makeUser(t, st, "cajero_router_014", "cajero")
	prod := makeProduct(t, st, "Café 014", decimal.RequireFromString("100"), false)
	uber := platformID(t, st, defaultCompanyID, "Uber Eats")
	abrirCajaPrincipal(t, st, cajero)
	orders := app.NewOrdersService(st, clock)
	ord, err := orders.Create(ctx, app.CreateOrderCmd{
		ClientUUID: uuid.New(), ServiceType: "domicilio", DeliveryPlatformID: &uber, OpenedBy: cajero,
		Lines: []domain.OrderLineInput{{ProductID: prod, Qty: decimal.RequireFromString("1")}},
	})
	if err != nil {
		t.Fatalf("crear: %v", err)
	}

	// El MESERO no corrige folios: no captura pedidos de plataforma.
	tokMesero := token("mesero_014", "mesero", defaultCompanyID)
	if got := patchFolio(t, r, tokMesero, ord.ID, "UBER-MESERO"); got != http.StatusForbidden {
		t.Fatalf("el mesero pudo corregir un folio (status %d)", got)
	}

	// El cajero sí: es quien captura el pedido, y esto es el mismo dato movido en el tiempo.
	tokCajero := token("cajero_tok_014", "cajero", defaultCompanyID)
	if got := patchFolio(t, r, tokCajero, ord.ID, "UBER-CAJERO-1"); got != http.StatusOK {
		t.Fatalf("el cajero no pudo corregir el folio (status %d)", got)
	}

	// Y el tope por usuario existe. Sin él, este endpoint es una escritura sin límite alcanzable
	// por el rol más común del local.
	visto429 := false
	for i := 0; i < 200; i++ {
		if patchFolio(t, r, tokCajero, ord.ID, fmt.Sprintf("UBER-RAFAGA-%d", i)) == http.StatusTooManyRequests {
			visto429 = true
			break
		}
	}
	if !visto429 {
		t.Fatal("200 correcciones seguidas del mismo usuario no toparon con el limitador: la ruta " +
			"quedó fuera del grupo con rateLimitUser")
	}
}
