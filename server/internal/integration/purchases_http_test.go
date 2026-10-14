//go:build integration

package integration

import (
	"context"
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

// Compras por el router real (spec 038): permisos, parámetros malformados y arreglos vacíos.
func TestPurchasesHTTP(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	jm := auth.NewManager("secreto-de-pruebas-suficientemente-largo-para-el-manager", nil)
	h := httpapi.NewHandlers(httpapi.Deps{
		JWT: jm, Backoffice: app.NewBackofficeService(st, nil), Settings: app.NewSettingsService(st, "pepper-de-prueba"),
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
	admin, cajero := token("admin_compras", "admin"), token("cajero_compras", "cajero")
	var category, expense int64
	ctx := context.Background()
	if err := st.Pool.QueryRow(ctx, `insert into expense_categories (company_id, name, financial_group)
		values ($1, 'Compras http', 'operacional') returning id`, defaultCompanyID).Scan(&category); err != nil {
		t.Fatal(err)
	}
	if err := st.Pool.QueryRow(ctx, `insert into expenses (company_id, expense_date, category_id, amount, status, created_by)
		values ($1, current_date, $2, 100, 'pendiente', (select min(id) from users)) returning id`, defaultCompanyID, category).Scan(&expense); err != nil {
		t.Fatal(err)
	}
	base := "/api/v1/expenses/" + strconv.FormatInt(expense, 10)
	json := "application/json"

	for _, c := range []struct{ method, path, body string }{
		{http.MethodPost, base + "/receive", `{}`},
		{http.MethodPut, base + "/stock-lines", `{"lines":[]}`},
		{http.MethodPut, "/api/v1/settings/inventory", `{"costingMethod":"weighted","stockItemLabel":"insumo"}`},
	} {
		if w := do(t, r, c.method, c.path, cajero, []byte(c.body), json); w.Code != http.StatusForbidden {
			t.Errorf("cajero %s %s = %d; quiere 403", c.method, c.path, w.Code)
		}
	}

	for _, bad := range []string{
		`{"costingMethod":"fifo","stockItemLabel":"insumo"}`,
		`{"costingMethod":"weighted","stockItemLabel":"   "}`,
		`{"costingMethod":"weighted","stockItemLabel":"veintiuna letras exac"}`,
	} {
		if w := do(t, r, http.MethodPut, "/api/v1/settings/inventory", admin, []byte(bad), json); w.Code != http.StatusBadRequest {
			t.Errorf("%s = %d; quiere 400", bad, w.Code)
		}
	}
	w := do(t, r, http.MethodPut, "/api/v1/settings/inventory", admin, []byte(`{"costingMethod":"weighted","stockItemLabel":"insumo"}`), json)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"costingMethod":"weighted"`) || !strings.Contains(w.Body.String(), `"stockItemLabel":"insumo"`) {
		t.Fatalf("guardar costeo = %d %s", w.Code, w.Body.String())
	}
	var events int
	if err := st.Pool.QueryRow(ctx, `select count(*) from audit_events where action = 'settings.costing_method'`).Scan(&events); err != nil || events != 1 {
		t.Errorf("eventos settings.costing_method = %d (%v); quiere 1", events, err)
	}
	if w := do(t, r, http.MethodGet, "/api/v1/business-settings", cajero, nil, ""); !strings.Contains(w.Body.String(), `"costingMethod":"weighted"`) {
		t.Errorf("GET /business-settings no trae el método: %s", w.Body.String())
	}

	if w := do(t, r, http.MethodGet, "/api/v1/expenses?pending=quiza", admin, nil, ""); w.Code != http.StatusBadRequest {
		t.Errorf("pending=quiza = %d; quiere 400 (no «todos» en silencio)", w.Code)
	}

	// Por recibir: arreglo vacío, permisos y reglas de la frontera (spec 038, historia 1b).
	if w := do(t, r, http.MethodGet, "/api/v1/stock/pending-receipts", admin, nil, ""); w.Code != http.StatusOK || strings.TrimSpace(w.Body.String()) != "[]" {
		t.Fatalf("por recibir vacío = %d %s; quiere 200 []", w.Code, w.Body.String())
	}
	if w := do(t, r, http.MethodGet, "/api/v1/stock/pending-receipts", cajero, nil, ""); w.Code != http.StatusForbidden {
		t.Errorf("cajero ve por recibir = %d; quiere 403", w.Code)
	}
	azucar := ingredientIn(t, st, defaultCompanyID, "Azúcar http", "g")
	gramo := strconv.Itoa(int(unitID(t, st, "g")))
	ing := strconv.FormatInt(azucar, 10)
	item := `{"itemType":"ingrediente","ingredientId":` + ing + `,"description":"Azúcar","quantity":"1000","unitId":` + gramo
	for name, c := range map[string]struct {
		tok, body string
		code      int
	}{
		"cajero agrega":         {cajero, item + `,"amount":"20"}`, http.StatusForbidden},
		"regalo con importe":    {admin, item + `,"amount":"20","isGift":true}`, http.StatusBadRequest},
		"pagado sin importe":    {admin, item + `,"amount":"0"}`, http.StatusBadRequest},
		"cantidad negativa":     {admin, `{"itemType":"ingrediente","ingredientId":` + ing + `,"description":"Azúcar","quantity":"-1","unitId":` + gramo + `,"amount":"20"}`, http.StatusBadRequest},
		"llegó algo más pagado": {admin, item + `,"amount":"20"}`, http.StatusCreated},
	} {
		if w := do(t, r, http.MethodPost, base+"/items", c.tok, []byte(c.body), json); w.Code != c.code {
			t.Errorf("%s = %d %s; quiere %d", name, w.Code, w.Body.String(), c.code)
		}
	}
	var itemID int64
	if err := st.Pool.QueryRow(ctx, `select id from expense_items where expense_id = $1`, expense).Scan(&itemID); err != nil {
		t.Fatal(err)
	}
	itemPath := base + "/items/" + strconv.FormatInt(itemID, 10)
	if w := do(t, r, http.MethodPost, itemPath+"/receive", cajero, []byte(`{"qty":"1","seenApplied":"1000"}`), json); w.Code != http.StatusForbidden {
		t.Errorf("cajero recibe = %d; quiere 403", w.Code)
	}
	// Ya llegó todo: recibir de más sin importe se rechaza en vez de adivinar el precio.
	if w := do(t, r, http.MethodPost, itemPath+"/receive", admin, []byte(`{"qty":"10","seenApplied":"1000"}`), json); w.Code != http.StatusBadRequest {
		t.Errorf("de más sin importe = %d; quiere 400", w.Code)
	}
	if w := do(t, r, http.MethodPost, itemPath+"/not-coming", admin, []byte(`{"qty":"1"}`), json); w.Code != http.StatusBadRequest {
		t.Errorf("«ya no va a llegar» sin nada pendiente = %d; quiere 400", w.Code)
	}

	// Sin renglones de almacén la respuesta trae arreglos, no null.
	w = do(t, r, http.MethodPut, base+"/stock-lines", admin, []byte(`{"lines":[]}`), json)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"items":[]`) || !strings.Contains(w.Body.String(), `"payments":[]`) {
		t.Fatalf("stock-lines vacío = %d %s; quiere 200 con items:[] y payments:[]", w.Code, w.Body.String())
	}
}
