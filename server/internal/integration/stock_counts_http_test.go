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
	for _, bad := range []string{`{"countedQty":"-1"}`, `{"countedQty":"1e20"}`, `{"countedQty":"abc"}`, `{}`, `{"countedQty":null}`} {
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
	// Sin toPosition el lugar se iba al primer puesto con 204: un parámetro ausente no cae a un default.
	for _, body := range []string{`{}`, `{"to":2}`} {
		if w := do(t, r, http.MethodPost, "/api/v1/stock/count-places/1/move", admin, []byte(body), json); w.Code != http.StatusBadRequest {
			t.Errorf("mover con %s = %d; quiere 400", body, w.Code)
		}
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
	only := func(p domain.Permission) func(domain.Role) []domain.Permission {
		return func(domain.Role) []domain.Permission { return []domain.Permission{p} }
	}
	for _, c := range []struct {
		name   string
		perms  func(domain.Role) []domain.Permission
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
				Broker: realtime.NewBroker(), Permissions: porRol(c.perms),
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

// Productos con existencia propia en el conteo por el router real (spec 069): mismo permiso que el
// de artículos, a ciegas, y arreglos aunque estén vacíos.
func TestStockCountProductsHTTP(t *testing.T) {
	t.Parallel()
	r, st, token := newStockAPI(t)
	admin, cajero := token("admin_conteo_prod", "admin"), token("cajero_conteo_prod", "cajero")
	lata := makeProduct(t, st, "Lata conteo http", dd("25"), true)
	adjust(t, st, "producto", lata, "-2")
	jsonCT := "application/json"

	w := do(t, r, http.MethodPost, "/api/v1/stock/counts", admin, nil, "")
	if w.Code != http.StatusCreated {
		t.Fatalf("abrir = %d %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if !strings.Contains(body, `"products":[`) || !strings.Contains(body, `"productId":`+strconv.FormatInt(lata, 10)) {
		t.Fatalf("el conteo no trae los productos: %s", body)
	}
	for _, leak := range []string{`"onHand"`, `"system"`, `"systemQty"`} {
		if strings.Contains(body, leak) {
			t.Errorf("el conteo abierto enseña lo que el sistema tiene (%s): deja de ser a ciegas", leak)
		}
	}
	var id int64
	if err := st.Pool.QueryRow(t.Context(), `select id from stock_counts where status = 'open' order by id desc limit 1`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	line := "/api/v1/stock/counts/" + strconv.FormatInt(id, 10) + "/product-lines/" + strconv.FormatInt(lata, 10)
	if w := do(t, r, http.MethodPut, line, cajero, []byte(`{"countedQty":"15"}`), jsonCT); w.Code != http.StatusForbidden {
		t.Errorf("cajero captura un producto = %d; quiere 403", w.Code)
	}
	// Un cuerpo sin cifra no es «no hay»: guardarlo como 0 dejaría el producto en 0 al cerrar.
	for _, missing := range []string{`{}`, `{"countedQty":null}`, `{"countedQty":"NaN"}`, `{"countedQty":"1e99999999999"}`, `{"countedQty":"1e30"}`, `{"countedQty":"0e9999"}`} {
		if w := do(t, r, http.MethodPut, line, admin, []byte(missing), jsonCT); w.Code != http.StatusBadRequest {
			t.Errorf("%s = %d; quiere 400", missing, w.Code)
		}
	}
	if w := do(t, r, http.MethodPut, line, admin, []byte(`{"countedQty":"2.5"}`), jsonCT); w.Code != http.StatusBadRequest {
		t.Errorf("2.5 latas = %d; quiere 400", w.Code)
	}
	if w := do(t, r, http.MethodPut, line, admin, []byte(`{"countedQty":"15"}`), jsonCT); w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"countedQty":"15"`) {
		t.Fatalf("capturar 15 = %d %s; quiere 200 con lo guardado", w.Code, w.Body.String())
	}
	if w := do(t, r, http.MethodDelete, line, cajero, nil, ""); w.Code != http.StatusForbidden {
		t.Errorf("cajero quita una cifra = %d; quiere 403", w.Code)
	}
	w = do(t, r, http.MethodGet, "/api/v1/stock/counts/"+strconv.FormatInt(id, 10)+"/preview", admin, nil, "")
	for _, k := range []string{`"adjustedProducts":[`, `"notCountedProducts":[`, `"skippedProducts":[`} {
		if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), k) {
			t.Errorf("vista previa sin %s como arreglo: %d %s", k, w.Code, w.Body.String())
		}
	}
	w = do(t, r, http.MethodPost, "/api/v1/stock/counts/"+strconv.FormatInt(id, 10)+"/close", admin, nil, "")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"delta":"17"`) {
		t.Fatalf("cerrar = %d %s; quiere el ajuste de +17", w.Code, w.Body.String())
	}
}
