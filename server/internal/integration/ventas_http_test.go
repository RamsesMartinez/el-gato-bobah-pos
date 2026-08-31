//go:build integration

package integration

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/auth"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/config"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/httpapi"
)

// La pantalla de Ventas POR EL ROUTER real.
//
// Los tests de servicio ya cubren las cifras; lo que solo se ve aquí es el cableado: que la ruta
// quedó dentro del grupo con RequireAuth y RequireRole, y que un parámetro inválido sale como 4xx
// en vez de caer a un default. Mover una ruta fuera de su grupo no rompe ningún test de servicio.
func TestRutasDeVentasPorElRouter(t *testing.T) {
	st := newTestStore(t)
	jm := auth.NewManager("secreto-de-pruebas-suficientemente-largo-para-el-manager", nil)
	h := httpapi.NewHandlers(httpapi.Deps{JWT: jm, Sales: app.NewSalesService(st, clock)})
	r := httpapi.Router(config.Config{}, jm, h, st)

	token := func(username, role string) string {
		id := makeUser(t, st, username, role)
		tok, err := jm.Issue(domain.User{ID: id, CompanyID: defaultCompanyID, Name: username, Role: domain.Role(role)})
		if err != nil {
			t.Fatalf("Issue(%s): %v", username, err)
		}
		return tok
	}
	gerente := token("gerente_ventas_http", "gerente")

	t.Run("sin token es 401", func(t *testing.T) {
		if w := do(t, r, http.MethodGet, "/api/v1/sales?preset=hoy", "", nil, ""); w.Code != http.StatusUnauthorized {
			t.Fatalf("sin token = %d, quiere 401", w.Code)
		}
	})

	// El cajero tiene su tablero de pedidos: esta pantalla es el histórico con el dinero del
	// negocio a la vista, y su gate es el que la separa.
	t.Run("el cajero no entra", func(t *testing.T) {
		cajero := token("cajero_ventas_http", "cajero")
		if w := do(t, r, http.MethodGet, "/api/v1/sales?preset=hoy", cajero, nil, ""); w.Code != http.StatusForbidden {
			t.Fatalf("cajero = %d, quiere 403", w.Code)
		}
	})

	t.Run("el gerente recibe la lista y el resumen", func(t *testing.T) {
		w := do(t, r, http.MethodGet, "/api/v1/sales?preset=hoy", gerente, nil, "")
		if w.Code != http.StatusOK {
			t.Fatalf("lista = %d: %s", w.Code, w.Body.String())
		}
		var page app.SalesPage
		if err := json.Unmarshal(w.Body.Bytes(), &page); err != nil {
			t.Fatalf("respuesta de la lista: %v (%s)", err, w.Body.String())
		}
		if page.Range.From == "" {
			t.Fatal("la respuesta debe decir qué rango está mirando")
		}

		w = do(t, r, http.MethodGet, "/api/v1/sales/summary?preset=hoy", gerente, nil, "")
		if w.Code != http.StatusOK {
			t.Fatalf("resumen = %d: %s", w.Code, w.Body.String())
		}
	})

	// Lo que la constitución llama "nunca cae a un default en silencio", por la ruta real: un
	// parámetro presente y desconocido se rechaza, no se ignora.
	t.Run("un parámetro inválido es 4xx y no un default", func(t *testing.T) {
		casos := []string{
			"?preset=el-mes-pasado-pero-solo-martes",
			"?preset=rango&from=2026-08-30&to=2026-08-01", // invertido
			"?preset=rango&from=2020-01-01&to=2026-08-30", // años
			"?preset=hoy&status=pagando",
			"?preset=hoy&serviceType=mesas",
			"?preset=hoy&sort=total_o_lo_que_sea",
			// Go dejó de aceptar el punto y coma como separador y url.Query() lo descarta EN
			// SILENCIO junto con el resto de los filtros: sin parsearlo a mano, la pantalla
			// contestaría los defaults como si nadie hubiera pedido nada.
			"?preset=hoy&sort=total;drop+table",
			"?preset=hoy&dir=arriba",
			"?preset=hoy&pageSize=100000",
			"?preset=hoy&from=2026-13-45",
		}
		for _, c := range casos {
			w := do(t, r, http.MethodGet, "/api/v1/sales"+c, gerente, nil, "")
			if w.Code < 400 || w.Code >= 500 {
				t.Fatalf("%s = %d, quiere 4xx: %s", c, w.Code, w.Body.String())
			}
		}
	})
}
