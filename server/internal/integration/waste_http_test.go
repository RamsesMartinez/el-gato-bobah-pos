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
)

// La merma por el router real (spec 039): quién puede qué, cantidades absurdas y arreglos vacíos.
func TestWasteHTTP(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	jm := auth.NewManager("secreto-de-pruebas-suficientemente-largo-para-el-manager", nil)
	h := httpapi.NewHandlers(httpapi.Deps{
		JWT: jm, Backoffice: app.NewBackofficeService(st, clock), Settings: app.NewSettingsService(st, "pepper-de-prueba"),
		MenuCache: cache.NewMenuCache(""), Broker: realtime.NewBroker(),
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
	cajero, mesero, gerente := token("cajero_merma_http", "cajero"), token("mesero_merma_http", "mesero"), token("gerente_merma_http", "gerente")
	ing := pieceIngredient(t, st, "Insumo de la merma http")
	adjust(t, st, "ingrediente", ing, "20")
	caduco := reasonNamed(t, st, defaultCompanyID, "Caducó")
	json := "application/json"
	body := func(q string) []byte {
		return []byte(`{"reasonId":` + strconv.FormatInt(caduco, 10) + `,"ingredientId":` + strconv.FormatInt(ing, 10) + `,"qty":"` + q + `"}`)
	}

	t.Run("cajero y mesero registran; no aprueban ni ven por aprobar", func(t *testing.T) {
		for _, tok := range []string{cajero, mesero} {
			if w := do(t, r, http.MethodPost, "/api/v1/waste", tok, body("1"), json); w.Code != http.StatusCreated {
				t.Fatalf("registrar = %d %s", w.Code, w.Body.String())
			}
			if w := do(t, r, http.MethodGet, "/api/v1/waste/reasons", tok, nil, ""); w.Code != http.StatusOK {
				t.Fatalf("motivos = %d", w.Code)
			}
			for _, path := range []string{"/api/v1/waste/pending"} {
				if w := do(t, r, http.MethodGet, path, tok, nil, ""); w.Code != http.StatusForbidden {
					t.Errorf("%s = %d; quiere 403", path, w.Code)
				}
			}
			if w := do(t, r, http.MethodPut, "/api/v1/settings/waste-threshold", tok, []byte(`{"threshold":"100"}`), json); w.Code != http.StatusForbidden {
				t.Errorf("tope = %d; quiere 403", w.Code)
			}
		}
	})

	t.Run("cantidades absurdas y notas largas son 400", func(t *testing.T) {
		for _, q := range []string{"0", "-1", "1e9", "abc"} {
			if w := do(t, r, http.MethodPost, "/api/v1/waste", cajero, body(q), json); w.Code != http.StatusBadRequest {
				t.Errorf("cantidad %q = %d; quiere 400", q, w.Code)
			}
		}
		long := []byte(`{"reasonId":` + strconv.FormatInt(caduco, 10) + `,"ingredientId":` + strconv.FormatInt(ing, 10) + `,"qty":"1","note":"` + strings.Repeat("a", 201) + `"}`)
		if w := do(t, r, http.MethodPost, "/api/v1/waste", cajero, long, json); w.Code != http.StatusBadRequest {
			t.Errorf("nota larga = %d; quiere 400", w.Code)
		}
	})

	t.Run("la captura trae sus artículos aunque el cajero no entre al catálogo", func(t *testing.T) {
		w := do(t, r, http.MethodGet, "/api/v1/waste/items", cajero, nil, "")
		if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "Insumo de la merma http") ||
			strings.Contains(w.Body.String(), `"packs":null`) || strings.Contains(w.Body.String(), `"recent":null`) {
			t.Fatalf("= %d %s", w.Code, w.Body.String())
		}
	})

	t.Run("sin pendientes la lista es [] y no null", func(t *testing.T) {
		w := do(t, r, http.MethodGet, "/api/v1/waste/pending", gerente, nil, "")
		if w.Code != http.StatusOK || strings.TrimSpace(w.Body.String()) != "[]" {
			t.Fatalf("= %d %s; quiere 200 []", w.Code, w.Body.String())
		}
	})

	t.Run("un motivo repetido responde 409 con el que existe", func(t *testing.T) {
		w := do(t, r, http.MethodPost, "/api/v1/waste/reasons", cajero, []byte(`{"name":" caduco ","description":"otra"}`), json)
		if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), `"existingId":`+strconv.FormatInt(caduco, 10)) {
			t.Fatalf("= %d %s; quiere 409 con existingId", w.Code, w.Body.String())
		}
	})
}
