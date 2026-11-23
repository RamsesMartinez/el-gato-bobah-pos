//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
)

// El motivo de una corrección de costo (spec 090): la pantalla sigue corrigiendo sin motivo; con
// motivo, queda en el historial y en la bitácora. Contra la API completa servida como gatobobah_app.
func TestCostCorrectionKeepsItsReason(t *testing.T) {
	t.Parallel()
	w := newMCPWorld(t)
	jwt := adminJWT(t, w)
	ing := ingredientIn(t, w.owner, w.company, "Chile seco con motivo", "g")
	post := func(body string) int {
		t.Helper()
		return do(t, w.router, http.MethodPost, "/api/v1/stock/articles/"+itoa(int(ing))+"/cost", jwt, []byte(body), "application/json").Code
	}

	if code := post(`{"cost":"0.5"}`); code != http.StatusNoContent {
		t.Fatalf("la pantalla sin motivo: %d", code)
	}
	if code := post(`{"cost":"0.6","reason":"  difiere de la última compra  "}`); code != http.StatusNoContent {
		t.Fatalf("con motivo: %d", code)
	}
	for _, bad := range []string{`{"cost":"0.7","reason":"   "}`, `{"cost":"0.7","reason":"` + strings.Repeat("x", 201) + `"}`} {
		if code := post(bad); code != http.StatusBadRequest {
			t.Fatalf("%.40s: %d, esperaba 400", bad, code)
		}
	}

	rec := do(t, w.router, http.MethodGet, "/api/v1/stock/articles/"+itoa(int(ing))+"/cost-history", jwt, nil, "")
	var page struct {
		Items []struct {
			Kind   string  `json:"kind"`
			Reason *string `json:"reason"`
		} `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &page); err != nil || len(page.Items) < 2 {
		t.Fatalf("historial: %d %s", rec.Code, rec.Body.String())
	}
	if r := page.Items[0].Reason; r == nil || *r != "difiere de la última compra" {
		t.Fatalf("la corrección con motivo: %v (recortado)", r)
	}
	if page.Items[1].Reason != nil {
		t.Fatalf("la corrección sin motivo salió con %q", *page.Items[1].Reason)
	}
	if n := scalar(t, w, `select count(*) from audit_events where action = 'ingredient.cost_corrected' and entity_id = $1
		and after->>'reason' = 'difiere de la última compra'`, ing); n != 1 {
		t.Fatalf("eventos de bitácora con el motivo: %d", n)
	}

	inTheThreeCases(t, w.company, w.other, func(t *testing.T, st *store.Store, ctx context.Context) {
		page, err := app.NewBackofficeService(st, nil).ArticleCostHistory(ctx, ing, nil)
		if err == nil && len(page.Items) > 0 {
			t.Fatalf("se leyó el historial (y sus motivos) de otra empresa: %d renglones", len(page.Items))
		}
	})
}
