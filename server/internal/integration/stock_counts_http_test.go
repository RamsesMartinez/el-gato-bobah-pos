//go:build integration

package integration

import (
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/auth"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/cache"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/config"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/httpapi"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/realtime"
	"net/http"
	"strconv"
	"strings"
	"testing"
)

// Conteo por el router real (spec 038, historia 3).
func TestStockCountsHTTP(t *testing.T) {
	t.Parallel()
	r, st, token := newStockAPI(t)
	admin, cajero := token("admin_conteo", "admin"), token("cajero_conteo", "cajero")
	azucar := ingredientIn(t, st, defaultCompanyID, "Azúcar conteo http", "g")
	json := "application/json"

	if w := do(t, r, http.MethodGet, "/api/v1/stock/counts/last-closed", admin, nil, ""); w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"closedAt":null`) {
		t.Errorf("sin conteos, último cerrado = %d %s; quiere closedAt null", w.Code, w.Body.String())
	}
	if w := do(t, r, http.MethodGet, "/api/v1/stock/counts/last-closed", cajero, nil, ""); w.Code != http.StatusForbidden {
		t.Errorf("cajero lee el último conteo = %d; quiere 403", w.Code)
	}
	if w := do(t, r, http.MethodGet, "/api/v1/stock/counts/current", admin, nil, ""); w.Code != http.StatusNoContent {
		t.Errorf("sin conteo abierto = %d; quiere 204", w.Code)
	}
	if w := do(t, r, http.MethodPost, "/api/v1/stock/counts", cajero, nil, ""); w.Code != http.StatusForbidden {
		t.Errorf("cajero abre un conteo = %d; quiere 403", w.Code)
	}
	w := do(t, r, http.MethodPost, "/api/v1/stock/counts", admin, nil, "")
	if w.Code != http.StatusCreated || !strings.Contains(w.Body.String(), `"presentations":[]`) {
		t.Fatalf("abrir = %d %s; quiere 201 con arreglos, no null", w.Code, w.Body.String())
	}
	if w := do(t, r, http.MethodPost, "/api/v1/stock/counts", admin, nil, ""); w.Code != http.StatusConflict {
		t.Errorf("segundo conteo = %d; quiere 409", w.Code)
	}
	var id int64
	if err := st.Pool.QueryRow(t.Context(), `select id from stock_counts where status = 'open' order by id desc limit 1`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	line := "/api/v1/stock/counts/" + strconv.FormatInt(id, 10) + "/lines/" + strconv.FormatInt(azucar, 10)
	for _, bad := range []string{`{"countedQty":"-1"}`, `{"countedQty":"1e20"}`, `{"countedQty":"abc"}`} {
		if w := do(t, r, http.MethodPut, line, admin, []byte(bad), json); w.Code != http.StatusBadRequest {
			t.Errorf("%s = %d; quiere 400", bad, w.Code)
		}
	}
	if w := do(t, r, http.MethodPut, line, cajero, []byte(`{"countedQty":"1"}`), json); w.Code != http.StatusForbidden {
		t.Errorf("cajero captura = %d; quiere 403", w.Code)
	}
	if w := do(t, r, http.MethodPut, line, admin, []byte(`{"countedQty":"3000"}`), json); w.Code != http.StatusNoContent {
		t.Fatalf("capturar = %d %s", w.Code, w.Body.String())
	}
	place := "/api/v1/stock/ingredients/" + strconv.FormatInt(azucar, 10) + "/place"
	// Lugares (spec 040): la lista llega como arreglo aunque esté vacía, un nombre de 31 letras es 400,
	// uno repetido es 409 con el que ya existe, y asignar es de quien cuenta.
	if w := do(t, r, http.MethodGet, "/api/v1/stock/count-places", admin, nil, ""); w.Code != http.StatusOK || !strings.HasPrefix(strings.TrimSpace(w.Body.String()), "[") {
		t.Errorf("lista de lugares = %d %s; quiere 200 con un arreglo", w.Code, w.Body.String())
	}
	if w := do(t, r, http.MethodPost, "/api/v1/stock/count-places", admin, []byte(`{"name":"`+strings.Repeat("x", 31)+`"}`), json); w.Code != http.StatusBadRequest {
		t.Errorf("lugar de 31 = %d; quiere 400", w.Code)
	}
	w = do(t, r, http.MethodPost, "/api/v1/stock/count-places", admin, []byte(`{"name":"Refri del http"}`), json)
	if w.Code != http.StatusCreated && w.Code != http.StatusConflict {
		t.Fatalf("alta de lugar = %d %s", w.Code, w.Body.String())
	}
	if w := do(t, r, http.MethodPost, "/api/v1/stock/count-places", admin, []byte(`{"name":" REFRI del HTTP "}`), json); w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), `"existingId"`) {
		t.Errorf("lugar repetido = %d %s; quiere 409 con existingId", w.Code, w.Body.String())
	}
	if w := do(t, r, http.MethodPut, place, cajero, []byte(`{"placeId":null}`), json); w.Code != http.StatusForbidden {
		t.Errorf("cajero cambia el lugar = %d; quiere 403", w.Code)
	}
	w = do(t, r, http.MethodPost, "/api/v1/stock/counts/"+strconv.FormatInt(id, 10)+"/close", admin, nil, "")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"adjusted":[`) || !strings.Contains(w.Body.String(), `"notCounted":[`) {
		t.Fatalf("cerrar = %d %s", w.Code, w.Body.String())
	}
}

// El lugar se pone al contar (FR-015f, decisión del dueño del 14-oct): lo decide `inventory.count`,
// no `inventory.configure`. Un rol que cuenta sin configurar el almacén lo puede poner; uno que
// configura sin contar, no.
func TestTheCountPlaceIsSetByWhoCounts(t *testing.T) {
	t.Parallel()
	only := func(p domain.Permission) httpapi.PermissionResolver {
		return func(domain.Role) []domain.Permission { return []domain.Permission{p} }
	}
	for _, c := range []struct {
		name   string
		perms  httpapi.PermissionResolver
		denied bool
	}{
		{"quien cuenta sí", only(domain.PermInventoryCount), false},
		{"quien solo configura el almacén no", only(domain.PermInventoryConfigure), true},
	} {
		t.Run(c.name, func(t *testing.T) {
			st := newTestStore(t)
			jm := auth.NewManager("secreto-de-pruebas-suficientemente-largo-para-el-manager", nil)
			h := httpapi.NewHandlers(httpapi.Deps{
				JWT: jm, Backoffice: app.NewBackofficeService(st, nil), MenuCache: cache.NewMenuCache(""),
				Broker: realtime.NewBroker(), Permissions: c.perms,
			})
			r := httpapi.Router(config.Config{}, jm, h, st)
			id := makeUser(t, st, "lugar_"+itoa(len(c.name)), "cajero")
			tok, err := jm.Issue(domain.User{ID: id, CompanyID: defaultCompanyID, Name: "x", Role: domain.RoleCajero})
			if err != nil {
				t.Fatal(err)
			}
			ing := ingredientIn(t, st, defaultCompanyID, "Lugar "+c.name, "g")
			w := do(t, r, http.MethodPut, "/api/v1/stock/ingredients/"+strconv.FormatInt(ing, 10)+"/place", tok, []byte(`{"place":"Refri"}`), "application/json")
			if c.denied && w.Code != http.StatusForbidden {
				t.Errorf("= %d; quiere 403", w.Code)
			}
			if !c.denied && w.Code != http.StatusNoContent {
				t.Errorf("= %d %s; quiere 204", w.Code, w.Body.String())
			}
		})
	}
}
