//go:build integration

package integration

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/auth"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/cache"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/config"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/httpapi"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/realtime"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
)

// Las rutas de precio por el ROUTER real, no por el servicio.
//
// Los tests de servicio ya cubren la regla; lo que solo se ve aquí es el cableado: que la ruta
// quedó dentro del grupo con RequireRole y con el tope por usuario, que el cuerpo se decodifica al
// tipo correcto, y que el error del dominio sale con el status que el front espera. Mover una ruta
// fuera de su grupo no rompe ningún test de servicio.
func newPreciosAPI(t *testing.T) (http.Handler, *store.Store, func(username, role string) string) {
	t.Helper()
	st := newTestStore(t)
	jm := auth.NewManager("secreto-de-pruebas-suficientemente-largo-para-el-manager", nil)
	h := httpapi.NewHandlers(httpapi.Deps{
		JWT:            jm,
		PlatformPrices: app.NewPlatformPricesService(st),
		MenuCache:      cache.NewMenuCache(""),
		Broker:         realtime.NewBroker(),
	})
	r := httpapi.Router(config.Config{}, jm, h, st)

	token := func(username, role string) string {
		id := makeUser(t, st, username, role)
		tok, err := jm.Issue(domain.User{ID: id, CompanyID: defaultCompanyID, Name: username, Role: domain.Role(role)})
		if err != nil {
			t.Fatalf("Issue(%s): %v", username, err)
		}
		return tok
	}
	return r, st, token
}

func TestRutasDePrecioPorElRouter(t *testing.T) {
	r, st, token := newPreciosAPI(t)
	prod := makeProduct(t, st, "Boneless http", decimal.RequireFromString("100"), false)
	uber := platformID(t, st, defaultCompanyID, "Uber Eats")

	cajero := token("cajero_http", "cajero")
	cuerpo := func(precio string) []byte {
		return []byte(fmt.Sprintf(`{"productId":%d,"platformId":%d,"price":%s}`, prod, uber, precio))
	}

	t.Run("el cajero puede capturar: es la decisión de agilidad de la feature", func(t *testing.T) {
		w := do(t, r, http.MethodPut, "/api/v1/platform-prices/product", cajero, cuerpo("149"), "application/json")
		if w.Code != http.StatusOK {
			t.Fatalf("PUT = %d, quiere 200: %s", w.Code, w.Body.String())
		}
		var resp struct {
			Price decimal.Decimal `json:"price"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("respuesta: %v (%s)", err, w.Body.String())
		}
		if !resp.Price.Equal(decimal.RequireFromString("149")) {
			t.Fatalf("devolvió %s, quiere 149", resp.Price)
		}
	})

	// Sin token la ruta no existe para nadie. Va antes que el rol: si RequireAuth se cayera del
	// grupo, el 403 por rol seguiría saliendo y taparía el agujero.
	t.Run("sin token es 401", func(t *testing.T) {
		if w := do(t, r, http.MethodPut, "/api/v1/platform-prices/product", "", cuerpo("149"), "application/json"); w.Code != http.StatusUnauthorized {
			t.Fatalf("sin token = %d, quiere 401", w.Code)
		}
	})

	// Mesero es el único rol autenticado fuera del grupo: toma la orden pero no cobra ni toca
	// precios. Si la ruta se saliera del grupo con RequireRole, este es el que lo delata.
	t.Run("un rol que no cobra no captura precios", func(t *testing.T) {
		mesero := token("mesero_http", "mesero")
		if w := do(t, r, http.MethodPut, "/api/v1/platform-prices/product", mesero, cuerpo("149"), "application/json"); w.Code != http.StatusForbidden {
			t.Fatalf("mesero = %d, quiere 403", w.Code)
		}
	})

	// La bomba de la auditoría, por la ruta real: 47 bytes de cuerpo que quemaban 25 segundos de
	// CPU. Tiene que salir como 4xx inmediato, no colgarse ni caerse como 500.
	t.Run("un exponente absurdo se rechaza como 4xx y al instante", func(t *testing.T) {
		w := do(t, r, http.MethodPut, "/api/v1/platform-prices/product", cajero, cuerpo("1e100000000"), "application/json")
		if w.Code < 400 || w.Code >= 500 {
			t.Fatalf("1e100000000 = %d, quiere 4xx (no 5xx ni cuelgue)", w.Code)
		}
	})

	// El id de plataforma es smallint. Con un parse de 64 bits, platformId=65537 truncaba a 1 y
	// borraba el precio de OTRA plataforma respondiendo 204.
	t.Run("un platformId que no cabe en smallint no borra el de otra plataforma", func(t *testing.T) {
		ruta := fmt.Sprintf("/api/v1/platform-prices/product?productId=%d&platformId=%d", prod, 65536+int(uber))
		if w := do(t, r, http.MethodDelete, ruta, cajero, nil, ""); w.Code != http.StatusBadRequest && w.Code != http.StatusUnprocessableEntity {
			t.Fatalf("platformId fuera de rango = %d, quiere 4xx de validación", w.Code)
		}
		var n int
		if err := st.Pool.QueryRow(t.Context(),
			`select count(*) from product_platform_prices where product_id=$1 and platform_id=$2`, prod, uber).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != 1 {
			t.Fatal("el precio de la plataforma real no debió borrarse")
		}
	})

	t.Run("quitar el precio devuelve 204 y lo borra", func(t *testing.T) {
		ruta := fmt.Sprintf("/api/v1/platform-prices/product?productId=%d&platformId=%d", prod, uber)
		if w := do(t, r, http.MethodDelete, ruta, cajero, nil, ""); w.Code != http.StatusNoContent {
			t.Fatalf("DELETE = %d, quiere 204: %s", w.Code, w.Body.String())
		}
		var n int
		if err := st.Pool.QueryRow(t.Context(),
			`select count(*) from product_platform_prices where product_id=$1 and platform_id=$2`, prod, uber).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != 0 {
			t.Fatalf("quedaron %d filas", n)
		}
	})
}

// El tope por usuario, por la ruta real. Es lo único que impide que un bucle de escrituras haga
// refetch del menú en todas las tablets del local: cada una publica `menu.updated`.
func TestElTopeDeEscriturasDePrecioAplicaEnLaRuta(t *testing.T) {
	r, st, token := newPreciosAPI(t)
	prod := makeProduct(t, st, "Alitas tope", decimal.RequireFromString("100"), false)
	uber := platformID(t, st, defaultCompanyID, "Uber Eats")
	cajero := token("cajero_tope", "cajero")
	cuerpo := []byte(fmt.Sprintf(`{"productId":%d,"platformId":%d,"price":149}`, prod, uber))

	// El tope es 120 en 5 minutos; se pasa de largo para no depender del número exacto.
	frenado := false
	for i := 0; i < 130; i++ {
		w := do(t, r, http.MethodPut, "/api/v1/platform-prices/product", cajero, cuerpo, "application/json")
		if w.Code == http.StatusTooManyRequests {
			frenado = true
			if w.Header().Get("Retry-After") == "" {
				t.Fatal("un 429 sin Retry-After deja al cliente adivinando cuándo reintentar")
			}
			break
		}
		if w.Code != http.StatusOK {
			t.Fatalf("escritura %d = %d: %s", i, w.Code, w.Body.String())
		}
	}
	if !frenado {
		t.Fatal("130 escrituras seguidas del mismo usuario debieron toparse con el límite")
	}

	// Otro cajero, mismo momento: su contador arranca en cero. Un tope por IP lo habría frenado
	// también, y el local entero sale por una sola dirección.
	otro := token("cajero_tope_2", "cajero")
	if w := do(t, r, http.MethodPut, "/api/v1/platform-prices/product", otro, cuerpo, "application/json"); w.Code != http.StatusOK {
		t.Fatalf("el segundo cajero = %d, quiere 200: no debe pagar el tope del primero", w.Code)
	}
}
