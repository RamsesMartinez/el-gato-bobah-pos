//go:build integration

package integration

import (
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
)

// UN OFFSET QUE NO CABE EN LA CONSULTA NO PUEDE CONTESTAR OTRA PÁGINA.
//
// Las tres listas de administración leían `offset` sin tope y lo convertían a int32: con
// offset=4294967296 el valor envuelve a 0 y la API contestaba 200 con la PRIMERA página, sin nada
// en la respuesta que lo delatara. Gastos y el catálogo por proveedor tenían el mismo defecto con
// `page × pageSize`. Va por el router real porque lo que se rompía era la frontera, no el servicio.
func TestUnOffsetQueDesbordaNoContestaLaPrimeraPagina(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	jm := auth.NewManager("secreto-de-pruebas-suficientemente-largo-para-el-manager", nil)
	h := httpapi.NewHandlers(httpapi.Deps{
		JWT:        jm,
		Admin:      app.NewAdminService(st),
		Backoffice: app.NewBackofficeService(st, nil),
		MenuCache:  cache.NewMenuCache(""),
		Broker:     realtime.NewBroker(),
	})
	r := httpapi.Router(config.Config{}, jm, h, st)
	id := makeUser(t, st, "admin_offset", "admin")
	tok, err := jm.Issue(domain.User{ID: id, CompanyID: defaultCompanyID, Name: "admin_offset", Role: domain.RoleAdmin})
	if err != nil {
		t.Fatal(err)
	}
	makeProduct(t, st, "Producto offset", decimal.RequireFromString("50"), false)

	malas := []string{
		// 2^32 envuelve a 0 en int32: la primera página.
		"/api/v1/admin/products?limit=1&offset=4294967296",
		"/api/v1/admin/modifier-options?limit=1&offset=4294967296",
		"/api/v1/admin/groups?limit=1&offset=4294967296",
		// Un offset negativo o que no es número se aplanaba a 0 en silencio.
		"/api/v1/admin/products?offset=-1",
		"/api/v1/admin/groups?offset=abc",
		// Un limit fuera de 0..100 se recortaba a 100 sin decirlo.
		"/api/v1/admin/products?limit=101",
		"/api/v1/admin/modifier-options?limit=-1",
		"/api/v1/admin/groups?limit=abc",
		// Una categoría que no es número contestaba todas las categorías.
		"/api/v1/admin/products?categoryId=abc",
		"/api/v1/admin/products?categoryId=-3",
		// page × pageSize = 4,294,967,300 envuelve a 4.
		"/api/v1/expenses?page=214748365&pageSize=20",
		"/api/v1/supplier-items?page=214748365&pageSize=20",
		"/api/v1/expenses?pageSize=500",
		"/api/v1/supplier-items?page=abc",
	}
	for _, path := range malas {
		if w := do(t, r, http.MethodGet, path, tok, nil, ""); w.Code != http.StatusBadRequest {
			t.Errorf("%s = %d (%.120s); quiere 400: contestar con otra página es peor que un error", path, w.Code, w.Body.String())
		}
	}

	// Lo que el front manda hoy sigue funcionando.
	buenas := []string{
		"/api/v1/admin/products?status=all&limit=0&offset=0",
		"/api/v1/admin/products?limit=25&offset=25&categoryId=0",
		"/api/v1/admin/products",
		"/api/v1/admin/modifier-options?status=all&limit=0&offset=0",
		"/api/v1/admin/groups?status=act&limit=25&offset=0",
		"/api/v1/expenses?page=0&pageSize=20",
		"/api/v1/expenses",
		"/api/v1/supplier-items?page=1&pageSize=30",
		"/api/v1/supplier-items",
	}
	for _, path := range buenas {
		if w := do(t, r, http.MethodGet, path, tok, nil, ""); w.Code != http.StatusOK {
			t.Errorf("%s = %d (%.200s); quiere 200", path, w.Code, w.Body.String())
		}
	}
}
