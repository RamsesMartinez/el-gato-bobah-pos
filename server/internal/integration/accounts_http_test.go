//go:build integration

package integration

import (
	"net/http"
	"testing"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/auth"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/config"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/httpapi"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/realtime"
)

// GET /pos/accounts EN JSON CRUDO (contracts/api.md).
func TestLiveAccountsHTTP(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	appSt := appRoleStore(t)
	jm := auth.NewManager(secretoDelNegocioEnPruebas, nil)
	orders := app.NewOrdersService(appSt, clock)
	h := httpapi.NewHandlers(httpapi.Deps{JWT: jm, Orders: orders, Drafts: app.NewDraftsService(appSt, orders),
		Accounts: app.NewAccountsService(appSt, orders), Broker: realtime.NewBroker()})
	r := httpapi.Router(config.Config{}, jm, h, appSt)
	id := makeUser(t, st, "cajero_fila_http", "cajero")
	tok, err := jm.Issue(domain.User{ID: id, CompanyID: defaultCompanyID, Name: "c", Role: domain.RoleCajero})
	if err != nil {
		t.Fatal(err)
	}

	t.Run("sin cuentas: items es [] y no null", func(t *testing.T) {
		w := do(t, r, http.MethodGet, "/api/v1/pos/accounts", tok, nil, "")
		if w.Code != http.StatusOK {
			t.Fatalf("= %d: %s", w.Code, w.Body)
		}
		m := rawJSON(t, w.Body.Bytes())
		mustArray(t, m, "items")
		if m["outstanding"] != "0" || m["serverTime"] == nil {
			t.Fatalf("cuerpo = %s", w.Body)
		}
	})

	t.Run("olderDebts=true se acepta", func(t *testing.T) {
		if w := do(t, r, http.MethodGet, "/api/v1/pos/accounts?olderDebts=true", tok, nil, ""); w.Code != http.StatusOK {
			t.Fatalf("= %d", w.Code)
		}
	})

	t.Run("olderDebts con otro valor es 400, nunca la ventana por omisión", func(t *testing.T) {
		for _, q := range []string{"maybe", "1", "false", "", "TRUE"} {
			if w := do(t, r, http.MethodGet, "/api/v1/pos/accounts?olderDebts="+q, tok, nil, ""); w.Code != http.StatusBadRequest {
				t.Errorf("olderDebts=%q = %d, quería 400", q, w.Code)
			}
		}
	})

	t.Run("GET /orders/open ya no lista nada", func(t *testing.T) {
		// La ruta se borró con la barra de «Pedidos por cobrar». `open` lo atrapa `/orders/{id}` y
		// rebota como id inválido: lo que importa es que ningún cliente viejo reciba una lista.
		w := do(t, r, http.MethodGet, "/api/v1/orders/open", tok, nil, "")
		if w.Code == http.StatusOK {
			t.Fatalf("GET /orders/open = 200: %s", w.Body)
		}
	})
}
