//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
)

// closedSessions siembra n cortes cerrados de la caja principal, uno por día desde `first`, y
// devuelve sus ids del más viejo al más nuevo.
func closedSessions(t *testing.T, st *store.Store, by int64, first time.Time, n int) []int64 {
	t.Helper()
	ctx := context.Background()
	var regID int64
	if err := st.Pool.QueryRow(ctx, `select id from cash_registers where company_id = $1 and is_primary limit 1`,
		defaultCompanyID).Scan(&regID); err != nil {
		t.Fatalf("caja principal: %v", err)
	}
	ids := make([]int64, n)
	for i := range n {
		day := first.AddDate(0, 0, i)
		if err := st.Pool.QueryRow(ctx, `
			insert into register_sessions (company_id, register_id, business_date, opening_cash, opened_by, opened_at,
			                               status, closed_by, closed_at)
			values ($1, $2, $3, 0, $4, $5::timestamptz, 'cerrada', $4, $5::timestamptz + interval '8 hours') returning id`,
			defaultCompanyID, regID, day, by, day.Add(9*time.Hour)).Scan(&ids[i]); err != nil {
			t.Fatalf("corte %d: %v", i, err)
		}
	}
	return ids
}

// EL HISTÓRICO DE CAJA SE PAGINA: un corte más viejo que los 50 recientes se tiene que poder abrir.
//
// La auditoría lo encontró: GET /cash-sessions devolvía los 50 más recientes sin total ni páginas,
// así que el corte 51 no existía para la pantalla. Lista y conteo salen del mismo `where`.
func TestCashHistoryReachesEveryCut(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	r, _, token := draftsAPI(t, st)
	gerente, tok := token("gerente_historico", "gerente")
	first := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	ids := closedSessions(t, st, gerente, first, 55)

	type page struct {
		Items []struct {
			ID int64 `json:"id"`
		} `json:"items"`
		Total    int `json:"total"`
		Page     int `json:"page"`
		PageSize int `json:"pageSize"`
	}
	get := func(t *testing.T, query string) page {
		t.Helper()
		w := do(t, r, http.MethodGet, "/api/v1/cash-sessions"+query, tok, nil, "")
		if w.Code != http.StatusOK {
			t.Fatalf("GET %s = %d: %s", query, w.Code, w.Body)
		}
		var p page
		if err := json.Unmarshal(w.Body.Bytes(), &p); err != nil {
			t.Fatal(err)
		}
		return p
	}

	t.Run("la última página trae el corte más viejo y el total los cuenta todos", func(t *testing.T) {
		p := get(t, "?page=2&pageSize=20")
		if p.Total != 55 || len(p.Items) != 15 {
			t.Fatalf("página 3 de 20: %d filas de un total de %d, quería 15 de 55", len(p.Items), p.Total)
		}
		if got := p.Items[len(p.Items)-1].ID; got != ids[0] {
			t.Fatalf("el último renglón es el corte %d y el más viejo es el %d", got, ids[0])
		}
	})

	t.Run("el rango por día acota lista y total con el mismo predicado", func(t *testing.T) {
		p := get(t, "?from=2026-08-01&to=2026-08-03")
		if p.Total != 3 || len(p.Items) != 3 {
			t.Fatalf("del 1 al 3 de agosto: %d filas, total %d; quería 3 y 3", len(p.Items), p.Total)
		}
		if p.Items[0].ID != ids[2] || p.Items[2].ID != ids[0] {
			t.Fatalf("orden %v: el más reciente primero", p.Items)
		}
	})

	for _, bad := range []string{"?page=-1", "?pageSize=0", "?pageSize=1000", "?page=abc",
		"?from=2026-09-02&to=2026-09-01", "?from=01/08/2026", "?to=ayer"} {
		t.Run("rechaza "+bad, func(t *testing.T) {
			if w := do(t, r, http.MethodGet, "/api/v1/cash-sessions"+bad, tok, nil, ""); w.Code != http.StatusBadRequest {
				t.Fatalf("= %d — un parámetro malformado no cae a «los más recientes»", w.Code)
			}
		})
	}
}

func TestCashHistoryInTheThreeCases(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	gerente := makeUser(t, st, "gerente_historico_ajeno", "gerente")
	closedSessions(t, st, gerente, time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC), 3)
	other := makeCompany(t, st, "otra-historico")
	inTheThreeCases(t, defaultCompanyID, other, func(t *testing.T, st *store.Store, ctx context.Context) {
		rows, total, err := app.NewBackofficeService(st, clock).SessionHistory(ctx, domain.SessionHistoryFilter{Limit: 20})
		if err == nil && (len(rows) != 0 || total != 0) {
			t.Fatalf("se ven %d cortes (total %d) de otra empresa", len(rows), total)
		}
	})
}
