//go:build integration

package integration

import (
	"context"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/auth"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/config"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/httpapi"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/realtime"
)

// LAS RUTAS DE PEDIDOS ENTRANTES (spec 093, contracts/incoming-orders.md): permiso propio, fronteras
// que rechazan en vez de adivinar, arreglos que nunca salen como `null`, y el aviso que apaga la
// alerta de las demás tabletas.
func TestIncomingOrdersHTTP(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	appSt := appRoleStore(t)
	jm := auth.NewManager(secretoDelNegocioEnPruebas, nil)
	broker := realtime.NewBroker()
	empresa := makeCompany(t, st, "entrantes-http")
	conexion := tiendaConLlave(t, st, empresa, "tienda-http", "llave-http-suficientemente-larga")
	plataforma := app.NewPedidosDePlataformaService(appSt,
		fixedClients{deciders: map[string]app.DecisorDePedidos{"Uber Eats": &decisorFalso{}}}, signingKeyCipher,
		"sandbox", clock)
	h := httpapi.NewHandlers(httpapi.Deps{JWT: jm, Broker: broker,
		Orders: app.NewOrdersService(appSt, clock), PedidosPlataforma: plataforma,
		IncomingOrders: app.NewIncomingOrdersService(plataforma)})
	r := httpapi.Router(config.Config{}, jm, h, appSt)

	token := func(username, role string) string {
		id := makeUserIn(t, st, empresa, username, role)
		tok, err := jm.Issue(domain.User{ID: id, CompanyID: empresa, Name: username, Role: domain.Role(role)})
		if err != nil {
			t.Fatal(err)
		}
		return tok
	}
	cajero := token("cajero_entrantes", "cajero")
	mesero := token("mesero_entrantes", "mesero")
	// La empresa le quita el permiso al mesero.
	if _, err := st.Pool.Exec(context.Background(), `
		insert into role_permission_overrides (company_id, role_id, permission_id, granted)
		select $1, r.id, p.id, false from roles r, permissions p
		 where r.company_id = $1 and r.system_role_id = 4 and p.code = 'orders.accept_incoming'`, empresa); err != nil {
		t.Fatal(err)
	}
	aceptar := pendienteDirecto(t, st, empresa, conexion, "H-1", fixedNow.Add(5*time.Minute))
	rechazar := pendienteDirecto(t, st, empresa, conexion, "H-2", fixedNow.Add(6*time.Minute))
	ruta := func(id int64, accion string) string {
		return "/api/v1/orders/incoming/platform/" + strconv.FormatInt(id, 10) + "/" + accion
	}
	const json = "application/json"

	t.Run("lista: orders, lines y options son arreglos, nunca null", func(t *testing.T) {
		w := do(t, r, http.MethodGet, "/api/v1/orders/incoming", cajero, nil, "")
		if w.Code != http.StatusOK {
			t.Fatalf("= %d: %s", w.Code, w.Body)
		}
		m := rawJSON(t, w.Body.Bytes())
		orders := mustArray(t, m, "orders")
		if len(orders) != 2 {
			t.Fatalf("pendientes = %d", len(orders))
		}
		for _, o := range orders {
			mustArray(t, o.(map[string]any), "lines")
		}
	})

	t.Run("sin el permiso: 403 en las rutas nuevas y en las viejas", func(t *testing.T) {
		for _, c := range []struct{ metodo, ruta, cuerpo string }{
			{http.MethodGet, "/api/v1/orders/incoming", ""},
			{http.MethodPost, ruta(aceptar, "accept"), "{}"},
			{http.MethodPost, ruta(rechazar, "reject"), `{"reason":"closed"}`},
			{http.MethodGet, "/api/v1/orders/platform/pending", ""},
			{http.MethodPost, "/api/v1/orders/platform/" + strconv.FormatInt(aceptar, 10) + "/accept", "{}"},
		} {
			if w := do(t, r, c.metodo, c.ruta, mesero, []byte(c.cuerpo), json); w.Code != http.StatusForbidden {
				t.Errorf("%s %s = %d, quería 403", c.metodo, c.ruta, w.Code)
			}
		}
	})

	t.Run("fronteras: origen, id y motivo inválidos son 400", func(t *testing.T) {
		for _, c := range []struct{ ruta, cuerpo string }{
			{"/api/v1/orders/incoming/web/" + strconv.FormatInt(aceptar, 10) + "/accept", "{}"},
			{"/api/v1/orders/incoming/platform/0/accept", "{}"},
			{"/api/v1/orders/incoming/platform/abc/accept", "{}"},
			{ruta(rechazar, "reject"), `{}`},
			{ruta(rechazar, "reject"), `{"reason":"STORE_CLOSED"}`},
			{ruta(rechazar, "reject"), `no es json`},
		} {
			if w := do(t, r, http.MethodPost, c.ruta, cajero, []byte(c.cuerpo), json); w.Code != http.StatusBadRequest {
				t.Errorf("%s %s = %d, quería 400", c.ruta, c.cuerpo, w.Code)
			}
		}
	})

	eventos, soltar := broker.Subscribe(empresa)
	defer soltar()
	espera := func(tipo string) {
		t.Helper()
		limite := time.After(2 * time.Second)
		for {
			select {
			case ev := <-eventos:
				if ev.Type == tipo {
					return
				}
			case <-limite:
				t.Fatalf("no llegó %q a las tabletas: las demás seguirían sonando", tipo)
			}
		}
	}

	t.Run("aceptar avisa a las demás tabletas", func(t *testing.T) {
		w := do(t, r, http.MethodPost, ruta(aceptar, "accept"), cajero, []byte("{}"), json)
		if w.Code != http.StatusOK {
			t.Fatalf("= %d: %s", w.Code, w.Body)
		}
		espera("incoming.changed")
		if w := do(t, r, http.MethodPost, ruta(aceptar, "accept"), cajero, []byte("{}"), json); w.Code != http.StatusConflict {
			t.Fatalf("aceptar dos veces = %d, quería 409", w.Code)
		}
	})

	t.Run("rechazar avisa a las demás tabletas", func(t *testing.T) {
		w := do(t, r, http.MethodPost, ruta(rechazar, "reject"), cajero, []byte(`{"reason":"too_busy"}`), json)
		if w.Code != http.StatusNoContent {
			t.Fatalf("= %d: %s", w.Code, w.Body)
		}
		espera("incoming.changed")
	})
}
