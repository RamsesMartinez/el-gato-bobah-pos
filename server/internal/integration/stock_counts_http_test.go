//go:build integration

package integration

import (
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
	if w := do(t, r, http.MethodPut, place, admin, []byte(`{"place":"`+strings.Repeat("x", 31)+`"}`), json); w.Code != http.StatusBadRequest {
		t.Errorf("lugar de 31 = %d; quiere 400", w.Code)
	}
	if w := do(t, r, http.MethodPut, place, cajero, []byte(`{"place":"Refri"}`), json); w.Code != http.StatusForbidden {
		t.Errorf("cajero cambia el lugar = %d; quiere 403", w.Code)
	}
	w = do(t, r, http.MethodPost, "/api/v1/stock/counts/"+strconv.FormatInt(id, 10)+"/close", admin, nil, "")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"adjusted":[`) || !strings.Contains(w.Body.String(), `"notCounted":[`) {
		t.Fatalf("cerrar = %d %s", w.Code, w.Body.String())
	}
}
