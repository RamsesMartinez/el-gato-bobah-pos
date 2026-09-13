//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"uuid"

	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/auth"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/config"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/httpapi"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
)

// routerDePedidos monta el router real con el servicio de pedidos: lo que se prueba aquí es el
// JSON que sale por la ruta, no lo que devuelve el servicio.
func routerDePedidos(t *testing.T, st *store.Store) (http.Handler, *auth.Manager) {
	t.Helper()
	jm := auth.NewManager(secretoDelNegocioEnPruebas, nil)
	h := httpapi.NewHandlers(httpapi.Deps{JWT: jm, Orders: app.NewOrdersService(st, clock)})
	return httpapi.Router(config.Config{}, jm, h, st), jm
}

// UN ARREGLO VACÍO NO ES UN NULO, Y LA DIFERENCIA TUMBÓ LA PANTALLA DE PEDIDOS EN PRODUCCIÓN.
//
// El 2026-09-13, el pedido «Azul Ruso» quedó abierto con su única línea cancelada. La consulta del
// tablero filtra `cancelled_at is null`, así que ese pedido no tenía entrada en el mapa de
// renglones; `porPedido[id]` devolvió el cero de Go —un slice **nil**— y `json.Marshal` escribe un
// slice nil como `null`, no como `[]`.
//
// La pantalla hace `o.lines.filter(...)` al pintar cada tarjeta, así que reventó con
// `Cannot read properties of null (reading 'filter')` y el mostrador se quedó viendo «La pantalla no
// se pudo mostrar» — **con el servidor respondiendo 200**. Por eso no había un solo 5xx en el log:
// no falló nada, se entregó un contrato distinto del prometido.
//
// Este test fija el contrato en la frontera HTTP y no en el servicio: lo que importa no es el valor
// de Go, es el JSON que sale.
func TestElTableroNuncaMandaRenglonesNulos(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	r, jm := routerDePedidos(t, st)

	svc := app.NewOrdersService(st, clock)
	cajero := makeUser(t, st, "cajero_sin_renglones", "cajero")
	cafe := makeProduct(t, st, "Café que se cancela", decimal.RequireFromString("100"), false)
	abrirCajaPrincipal(t, st, cajero)

	ord, err := crearYCobrar(t, ctx, svc, app.CreateOrderCmd{
		ClientUUID: uuid.New(), ServiceType: "mostrador", OpenedBy: cajero,
		Lines: []domain.OrderLineInput{{ProductID: cafe, Qty: decimal.RequireFromString("1")}},
	})
	if err != nil {
		t.Fatalf("crear el pedido: %v", err)
	}

	// Se cancela su ÚNICA línea. El pedido sigue abierto, en cero y sin nada que entregar: es el
	// estado exacto que produjo la caída.
	if _, err := svc.CancelarRenglon(ctx, ord.ID, ord.Lines[0].ID, cajero, "se equivocó"); err != nil {
		t.Fatalf("cancelar el renglón: %v", err)
	}

	tok, err := jm.Issue(domain.User{ID: cajero, CompanyID: defaultCompanyID, Name: "Cajero", Role: domain.RoleCajero})
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}

	// Se mira el JSON CRUDO. Deserializar a una estructura de Go borraría la diferencia entre
	// `null` y `[]` —las dos llegan como slice nil— que es justo la que rompió la pantalla.
	w := do(t, r, http.MethodGet, "/api/v1/orders", tok, nil, "")
	if w.Code != http.StatusOK {
		t.Fatalf("el tablero = %d: %s", w.Code, w.Body.String())
	}
	revisarRenglonesNoNulos(t, w.Body.Bytes(), "/orders")
}

// Y LAS ENTREGADAS, que nunca llevaron renglones: su constructor ni siquiera asigna el campo, así
// que TODAS salían con `lines: null`. Hoy no tumba nada solo porque la pantalla de entregadas no
// los toca — la misma bomba, armada en otro lado.
func TestLasEntregadasTampocoMandanRenglonesNulos(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	r, jm := routerDePedidos(t, st)

	svc := app.NewOrdersService(st, clock)
	gerente := makeUser(t, st, "gerente_entregadas_nulas", "gerente")
	cafe := makeProduct(t, st, "Café entregado", decimal.RequireFromString("100"), false)
	abrirCajaPrincipal(t, st, gerente)

	ord, err := crearYCobrar(t, ctx, svc, app.CreateOrderCmd{
		ClientUUID: uuid.New(), ServiceType: "mostrador", OpenedBy: gerente,
		Lines:    []domain.OrderLineInput{{ProductID: cafe, Qty: decimal.RequireFromString("1")}},
		Payments: []app.PaymentInput{{MethodID: paymentMethodID(t, st, "Efectivo"), Amount: decimal.RequireFromString("100")}},
	})
	if err != nil {
		t.Fatalf("crear el pedido: %v", err)
	}
	if err := svc.SetStatus(ctx, ord.ID, domain.StatusEntregada); err != nil {
		t.Fatalf("entregar: %v", err)
	}

	tok, err := jm.Issue(domain.User{ID: gerente, CompanyID: defaultCompanyID, Name: "Gerente", Role: domain.RoleGerente})
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	w := do(t, r, http.MethodGet, "/api/v1/orders/delivered", tok, nil, "")
	if w.Code != http.StatusOK {
		t.Fatalf("las entregadas = %d: %s", w.Code, w.Body.String())
	}
	revisarRenglonesNoNulos(t, w.Body.Bytes(), "/orders/delivered")
}

// revisarRenglonesNoNulos exige que TODO pedido del cuerpo traiga `lines` como arreglo.
//
// Se comprueba sobre el JSON sin tipar a propósito: la pantalla lo recibe así, y es ahí donde
// `null` y `[]` dejan de ser lo mismo.
func revisarRenglonesNoNulos(t *testing.T, cuerpo []byte, ruta string) {
	t.Helper()
	var payload struct {
		Items []map[string]json.RawMessage `json:"items"`
	}
	if err := json.Unmarshal(cuerpo, &payload); err != nil {
		t.Fatalf("respuesta de %s: %v (%s)", ruta, err, string(cuerpo))
	}
	if len(payload.Items) == 0 {
		t.Fatalf("%s devolvió cero pedidos: el test pasaría en verde sin comprobar nada", ruta)
	}
	for _, item := range payload.Items {
		crudo, ok := item["lines"]
		if !ok {
			t.Fatalf("%s: un pedido llegó SIN el campo `lines`; la pantalla lo recorre al pintar", ruta)
		}
		if string(crudo) == "null" {
			t.Fatalf("%s: un pedido llegó con `lines: null`. La pantalla hace `o.lines.filter(...)` al pintar cada tarjeta, así que esto la tumba entera con el servidor respondiendo 200 — es el defecto del 2026-09-13", ruta)
		}
	}
}
