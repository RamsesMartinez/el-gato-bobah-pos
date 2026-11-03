//go:build integration

package integration

import (
	"net/http"
	"strconv"
	"strings"
	"testing"
)

// Densidad, tara y conteo pesando con empaque por el router real (spec 058).
func TestDensityAndGrossCountHTTP(t *testing.T) {
	t.Parallel()
	r, st, token := newStockAPI(t)
	admin, cajero := token("admin_densidad", "admin"), token("cajero_densidad", "cajero")
	salsa := ingredientIn(t, st, defaultCompanyID, "Salsa http densidad", "ml")
	json := "application/json"
	density := "/api/v1/stock/articles/" + strconv.FormatInt(salsa, 10) + "/density"

	if w := do(t, r, http.MethodPut, density, cajero, []byte(`{"method":"manual","density":"1.1"}`), json); w.Code != http.StatusForbidden {
		t.Errorf("cajero captura la densidad = %d; quiere 403", w.Code)
	}
	for _, bad := range []string{
		`{"method":"manual","density":"1.1","extra":1}`, // un campo que no se entiende no cae a un default
		`{"method":"manual"}`,
		`{"method":"weighing","fullG":"150","emptyG":"150","contentMl":"1000"}`,
		`{"method":"manual","density":"9"}`,
	} {
		if w := do(t, r, http.MethodPut, density, admin, []byte(bad), json); w.Code != http.StatusBadRequest {
			t.Errorf("%s = %d; quiere 400", bad, w.Code)
		}
	}
	w := do(t, r, http.MethodPut, density, admin, []byte(`{"method":"weighing","fullG":"1250","emptyG":"150","contentMl":"1000"}`), json)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"method":"weighing"`) {
		t.Fatalf("capturar por pesada = %d %s", w.Code, w.Body.String())
	}

	var bottle int64
	if err := st.Pool.QueryRow(t.Context(), `insert into ingredient_presentations (company_id, ingredient_id, name, qty_in_base, tare_g)
		values ($1, $2, 'Botella', 1000, 120) returning id`, defaultCompanyID, salsa).Scan(&bottle); err != nil {
		t.Fatal(err)
	}
	if w := do(t, r, http.MethodPost, "/api/v1/stock/counts", admin, nil, ""); w.Code != http.StatusCreated {
		t.Fatalf("abrir conteo = %d", w.Code)
	} else if !strings.Contains(w.Body.String(), `"grossWeighable":true`) || !strings.Contains(w.Body.String(), `"tareG":"120"`) {
		t.Errorf("el conteo no trae lo necesario para pesar con empaque: %s", w.Body.String())
	}
	var count int64
	if err := st.Pool.QueryRow(t.Context(), `select id from stock_counts where status = 'open' order by id desc limit 1`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	line := "/api/v1/stock/counts/" + strconv.FormatInt(count, 10) + "/lines/" + strconv.FormatInt(salsa, 10)
	g := strconv.Itoa(int(unitID(t, st, "g")))
	body := `{"countedQty":"3000","confirmable":true,"gross":{"weight":"850","weightUnitId":` + g + `,"presentationId":` +
		strconv.FormatInt(bottle, 10) + `,"containers":2,"seenTareG":"120","seenDensity":"1.1"}}`
	w = do(t, r, http.MethodPut, line, admin, []byte(body), json)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"countedQty":"3554.5455"`) || !strings.Contains(w.Body.String(), `"grossNet":"554.5455"`) {
		t.Fatalf("capturar pesando = %d %s", w.Code, w.Body.String())
	}
	stale := strings.Replace(body, `"seenTareG":"120"`, `"seenTareG":"100"`, 1)
	if w := do(t, r, http.MethodPut, line, admin, []byte(stale), json); w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "COUNT_INPUTS_CHANGED") {
		t.Errorf("con otra tara = %d %s; quiere 409 COUNT_INPUTS_CHANGED", w.Code, w.Body.String())
	}
}

// Revisión de seguridad: quitar la densidad y cambiar lo que pesa vacío un empaque también son de
// quien configura el almacén. Si mañana una ruta se mueve a un grupo más laxo, esto truena.
func TestOnlyWhoConfiguresTheStockChangesDensityOrTare(t *testing.T) {
	t.Parallel()
	r, st, token := newStockAPI(t)
	admin, cajero := token("admin_tara", "admin"), token("cajero_tara", "cajero")
	salsa := ingredientIn(t, st, defaultCompanyID, "Salsa http tara", "ml")
	density := "/api/v1/stock/articles/" + strconv.FormatInt(salsa, 10) + "/density"
	if w := do(t, r, http.MethodPut, density, admin, []byte(`{"method":"manual","density":"1.1"}`), "application/json"); w.Code != http.StatusOK {
		t.Fatalf("admin = %d", w.Code)
	}
	if w := do(t, r, http.MethodDelete, density, cajero, nil, ""); w.Code != http.StatusForbidden {
		t.Errorf("cajero quita la densidad = %d; quiere 403", w.Code)
	}
	var bottle int64
	if err := st.Pool.QueryRow(t.Context(), `insert into ingredient_presentations (company_id, ingredient_id, name, qty_in_base, tare_g)
		values ($1, $2, 'Botella', 1000, 120) returning id`, defaultCompanyID, salsa).Scan(&bottle); err != nil {
		t.Fatal(err)
	}
	ml := strconv.Itoa(int(unitID(t, st, "ml")))
	body := `{"name":"Botella","qty":"1000","unitId":` + ml + `,"approximate":false,"tareG":"50"}`
	if w := do(t, r, http.MethodPut, "/api/v1/stock/presentations/"+strconv.FormatInt(bottle, 10), cajero, []byte(body), "application/json"); w.Code != http.StatusForbidden {
		t.Errorf("cajero cambia lo que pesa vacío = %d; quiere 403", w.Code)
	}
	var tare string
	if err := st.Pool.QueryRow(t.Context(), `select tare_g::text from ingredient_presentations where id = $1`, bottle).Scan(&tare); err != nil {
		t.Fatal(err)
	}
	if tare != "120.00" {
		t.Fatalf("la tara quedó en %s", tare)
	}
}
