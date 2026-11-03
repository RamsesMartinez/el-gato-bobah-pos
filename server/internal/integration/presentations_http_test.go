//go:build integration

package integration

import (
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/auth"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/cache"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/config"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/httpapi"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/realtime"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
)

// newStockAPI arma el router real con el servicio de almacén.
func newStockAPI(t *testing.T) (http.Handler, *store.Store, func(username, role string) string) {
	t.Helper()
	st := newTestStore(t)
	jm := auth.NewManager("secreto-de-pruebas-suficientemente-largo-para-el-manager", nil)
	h := httpapi.NewHandlers(httpapi.Deps{
		JWT:        jm,
		Backoffice: app.NewBackofficeService(st, nil),
		MenuCache:  cache.NewMenuCache(""),
		Broker:     realtime.NewBroker(),
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

func TestPresentationsHTTP(t *testing.T) {
	t.Parallel()
	r, st, token := newStockAPI(t)
	admin := token("admin_empaques", "admin")
	cajero := token("cajero_empaques", "cajero")
	leche := ingredientIn(t, st, defaultCompanyID, "Leche http", "ml")
	vaso := ingredientIn(t, st, defaultCompanyID, "Vaso http", "pieza")
	path := "/api/v1/stock/ingredients/" + strconv.FormatInt(leche, 10) + "/presentations"
	litro := strconv.Itoa(int(unitID(t, st, "l")))

	// Una empresa sin empaques recibe [] y no null.
	w := do(t, r, http.MethodGet, "/api/v1/stock/presentations", admin, nil, "")
	if w.Code != http.StatusOK || strings.TrimSpace(w.Body.String()) != "[]" {
		t.Fatalf("lista vacía = %d %s; quiere 200 []", w.Code, w.Body.String())
	}

	body := `{"name":"Caja","qty":"12","unitId":` + litro + `}`
	if w := do(t, r, http.MethodPost, path, cajero, []byte(body), "application/json"); w.Code != http.StatusForbidden {
		t.Fatalf("cajero crea un empaque = %d; quiere 403", w.Code)
	}
	for _, bad := range []string{
		`{"name":"Caja","qty":"0","unitId":` + litro + `}`,
		`{"name":"Caja","qty":"-3","unitId":` + litro + `}`,
		`{"name":"","qty":"12","unitId":` + litro + `}`,
		`{"name":"Caja","qty":"12"}`,
	} {
		if w := do(t, r, http.MethodPost, path, admin, []byte(bad), "application/json"); w.Code != http.StatusBadRequest {
			t.Errorf("%s = %d; quiere 400", bad, w.Code)
		}
	}
	vasoPath := "/api/v1/stock/ingredients/" + strconv.FormatInt(vaso, 10) + "/presentations"
	pieza := strconv.Itoa(int(unitID(t, st, "pieza")))
	if w := do(t, r, http.MethodPost, vasoPath, admin, []byte(`{"name":"Bolsa","qty":"12.5","unitId":`+pieza+`}`), "application/json"); w.Code != http.StatusBadRequest {
		t.Errorf("piezas con decimales = %d; quiere 400", w.Code)
	}
	if w := do(t, r, http.MethodPost, path, admin, []byte(body), "application/json"); w.Code != http.StatusCreated {
		t.Fatalf("admin crea «Caja = 12 L» = %d %s", w.Code, w.Body.String())
	}
	if w := do(t, r, http.MethodPost, path, admin, []byte(body), "application/json"); w.Code != http.StatusConflict {
		t.Fatalf("nombre repetido = %d; quiere 409", w.Code)
	}
	if w := do(t, r, http.MethodGet, "/api/v1/stock/presentations", cajero, nil, ""); w.Code != http.StatusForbidden {
		t.Fatalf("cajero lista empaques = %d; quiere 403", w.Code)
	}
}
