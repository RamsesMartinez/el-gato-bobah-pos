//go:build integration

package integration

import (
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/auth"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/cache"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/config"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/httpapi"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/realtime"
)

// Las rutas de alternativos por el router real (spec 063): permiso, entradas absurdas y el 409 con
// el nombre que chocó.
func TestAltNamesHTTP(t *testing.T) {
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
	token := func(username, role string) string {
		id := makeUser(t, st, username, role)
		tok, err := jm.Issue(domain.User{ID: id, CompanyID: defaultCompanyID, Name: username, Role: domain.Role(role)})
		if err != nil {
			t.Fatal(err)
		}
		return tok
	}
	gerente, cajero := token("gerente_alt", "gerente"), token("cajero_alt", "cajero")
	price := decimal.RequireFromString("60")
	ice := makeProduct(t, st, "Ice Coffee http", price, false)
	makeProduct(t, st, "Latte http", price, false)
	leche := ingredientIn(t, st, defaultCompanyID, "Leche entera http", "ml")
	ingredientIn(t, st, defaultCompanyID, "Leche Lala http", "ml")
	json := "application/json"
	eleven := `{"altNames":["a1","a2","a3","a4","a5","a6","a7","a8","a9","a10","a11"]}`
	long := `{"altNames":["` + strings.Repeat("a", domain.MaxAltName+1) + `"]}`

	// La ficha del artículo manda su versión (STALE si cambió); la del producto no lleva.
	withVersion := func(article bool, body string) []byte {
		if !article || !strings.HasPrefix(body, `{"altNames":`) {
			return []byte(body)
		}
		v := articleVersion(t, st, leche).Format(time.RFC3339Nano)
		return []byte(strings.TrimSuffix(body, "}") + `,"expectedUpdatedAt":"` + v + `"}`)
	}
	for _, c := range []struct {
		base, clash, owner string
		article            bool
	}{
		{"/api/v1/admin/products/" + strconv.FormatInt(ice, 10), "latte HTTP", "Latte http", false},
		{"/api/v1/stock/articles/" + strconv.FormatInt(leche, 10), "leche lala http", "Leche Lala http", true},
	} {
		put := c.base + "/alt-names"
		if w := do(t, r, http.MethodPut, put, cajero, []byte(`{"altNames":["Café frío"]}`), json); w.Code != http.StatusForbidden {
			t.Errorf("%s: cajero = %d; quiere 403", put, w.Code)
		}
		if w := do(t, r, http.MethodGet, put+"/similar?name=caf", cajero, nil, ""); w.Code != http.StatusForbidden {
			t.Errorf("%s/similar: cajero = %d; quiere 403", put, w.Code)
		}
		// Sin la lista no es «vaciarla» (un {} borraba todos), y un NUL no llega a Postgres (500).
		for _, bad := range []string{eleven, long, `{"altNames":["x"],"otro":1}`, `{}`, `{"altNames":null}`, `{"altNames":["caf\u0000e"]}`} {
			if w := do(t, r, http.MethodPut, put, gerente, withVersion(c.article, bad), json); w.Code != http.StatusBadRequest {
				t.Errorf("%s %.40s = %d; quiere 400", put, bad, w.Code)
			}
		}
		w := do(t, r, http.MethodPut, put, gerente, withVersion(c.article, `{"altNames":[" Café  frío "]}`), json)
		if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"altNames":["Café frío"]`) {
			t.Fatalf("%s gerente = %d %s", put, w.Code, w.Body.String())
		}
		w = do(t, r, http.MethodPut, put, gerente, withVersion(c.article, `{"altNames":[]}`), json)
		if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"altNames":[]`) {
			t.Fatalf("quitar todos = %d %s; quiero altNames:[] en el JSON crudo", w.Code, w.Body.String())
		}
		w = do(t, r, http.MethodPut, put, gerente, withVersion(c.article, `{"altNames":["`+c.clash+`"]}`), json)
		if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "ALT_NAME_TAKEN") ||
			!strings.Contains(w.Body.String(), c.owner) || !strings.Contains(w.Body.String(), `"name":"`+c.clash+`"`) {
			t.Errorf("%s choque = %d %s", put, w.Code, w.Body.String())
		}
		if c.article {
			stale := `{"altNames":["x"],"expectedUpdatedAt":"2020-01-01T00:00:00Z"}`
			if w := do(t, r, http.MethodPut, put, gerente, []byte(stale), json); w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "STALE") {
				t.Errorf("con otra versión = %d %s; quiere 409 STALE", w.Code, w.Body.String())
			}
			if w := do(t, r, http.MethodPut, put, gerente, []byte(`{"altNames":["x"]}`), json); w.Code != http.StatusBadRequest {
				t.Errorf("sin versión = %d; quiere 400", w.Code)
			}
		}
		if w := do(t, r, http.MethodGet, put+"/similar?name=", gerente, nil, ""); w.Code != http.StatusBadRequest {
			t.Errorf("%s/similar sin nombre = %d; quiere 400", put, w.Code)
		}
		if w := do(t, r, http.MethodGet, put+"/similar?name=x", gerente, nil, ""); w.Code != http.StatusOK || strings.TrimSpace(w.Body.String()) != "[]" {
			t.Errorf("%s/similar sin parecidos = %d %s; quiere []", put, w.Code, w.Body.String())
		}
	}
}
