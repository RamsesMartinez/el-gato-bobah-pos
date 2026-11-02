//go:build integration

package integration

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
)

// Posibles duplicados (spec 056, US6) y el aviso al dar de alta (US5).
func TestDuplicateGroupsAndDistinctPairs(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()
	admin := makeUser(t, st, "dup_admin", "admin")
	a := ingredientIn(t, st, defaultCompanyID, "Polvo chocolate", "g")
	b := ingredientIn(t, st, defaultCompanyID, "Polvo chocolate Monjita", "g")
	ingredientIn(t, st, defaultCompanyID, "Azúcar dup", "g")
	archived := ingredientIn(t, st, defaultCompanyID, "Polvo chocolate viejo", "g")
	if _, err := st.Pool.Exec(ctx, `update ingredients set is_active = false where id = $1`, archived); err != nil {
		t.Fatal(err)
	}
	svc, tctx := asTenant(t, defaultCompanyID)
	tctx = app.WithActorUser(tctx, admin)

	groups, err := svc.DuplicateGroups(tctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 1 || len(groups[0].Items) != 2 {
		t.Fatalf("grupos = %+v; quiere uno con los dos polvos activos (el archivado no se propone)", groups)
	}

	t.Run("no son lo mismo: el par ya no se propone, y marcarlo dos veces no truena", func(t *testing.T) {
		for range 2 {
			if err := svc.MarkDistinct(tctx, b, a); err != nil {
				t.Fatal(err)
			}
		}
		groups, err := svc.DuplicateGroups(tctx)
		if err != nil || len(groups) != 0 {
			t.Fatalf("grupos = %+v %v; el par descartado no vuelve", groups, err)
		}
		var low, high int64
		if err := st.Pool.QueryRow(ctx, `select low_ingredient_id, high_ingredient_id from ingredient_distinct_pairs`).Scan(&low, &high); err != nil {
			t.Fatal(err)
		}
		if low != min(a, b) || high != max(a, b) {
			t.Fatalf("par guardado (%d, %d); quiere ordenado", low, high)
		}
	})
	t.Run("un par consigo mismo es 400 y uno de otra empresa no existe", func(t *testing.T) {
		if err := svc.MarkDistinct(tctx, a, a); !errors.Is(err, domain.ErrValidation) {
			t.Fatalf("a con a = %v; quiere 400", err)
		}
		other := ingredientIn(t, st, makeCompany(t, st, "dup-otra"), "Polvo chocolate otra", "g")
		if err := svc.MarkDistinct(tctx, a, other); !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("con un insumo de otra empresa = %v; quiere 404", err)
		}
	})
	t.Run("el aviso al dar de alta", func(t *testing.T) {
		got, err := svc.SimilarArticles(tctx, "Polvo chocolat")
		if err != nil || len(got) == 0 {
			t.Fatalf("parecidos de «Polvo chocolat» = %+v %v", got, err)
		}
		none, err := svc.SimilarArticles(tctx, "Aceite")
		if err != nil || none == nil || len(none) != 0 {
			t.Fatalf("sin parecidos = %#v %v; quiere una lista vacía", none, err)
		}
		for _, bad := range []string{"", "   ", strings.Repeat("x", 81)} {
			if _, err := svc.SimilarArticles(tctx, bad); !errors.Is(err, domain.ErrValidation) {
				t.Fatalf("nombre %q = %v; quiere 400", bad, err)
			}
		}
	})
	t.Run("nada de esto se ve desde otra empresa (los tres casos)", func(t *testing.T) {
		inTheThreeCases(t, int64(defaultCompanyID), makeCompany(t, st, "dup-ajena"), func(t *testing.T, st *store.Store, ctx context.Context) {
			svc := app.NewBackofficeService(st, nil)
			groups, err := svc.DuplicateGroups(ctx)
			if err == nil && len(groups) > 0 {
				t.Fatalf("se ven grupos de otra empresa: %+v", groups)
			}
			got, err := svc.SimilarArticles(ctx, "Polvo chocolate")
			if err == nil && len(got) > 0 {
				t.Fatalf("se ven nombres de otra empresa: %+v", got)
			}
		})
	})
}

func TestDuplicatesHTTP(t *testing.T) {
	t.Parallel()
	r, st, token := newStockAPI(t)
	a := ingredientIn(t, st, defaultCompanyID, "Leche dup http", "g")
	b := ingredientIn(t, st, defaultCompanyID, "Leche dup http lala", "g")
	cajero := token("dup_cajero", "cajero")
	for _, p := range []string{"/api/v1/stock/articles/duplicate-groups", "/api/v1/stock/articles/similar?name=leche"} {
		if w := do(t, r, http.MethodGet, p, cajero, nil, ""); w.Code != http.StatusForbidden {
			t.Fatalf("cajero %s = %d; quiere 403", p, w.Code)
		}
	}
	admin := token("dup_admin_http", "admin")
	w := do(t, r, http.MethodGet, "/api/v1/stock/articles/similar?name=", admin, nil, "")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("sin nombre = %d; quiere 400", w.Code)
	}
	w = do(t, r, http.MethodGet, "/api/v1/stock/articles/similar?name=aceite", admin, nil, "")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"items":[]`) {
		t.Fatalf("sin parecidos = %d %s; quiere 200 con items: [] (no null)", w.Code, w.Body.String())
	}
	body := []byte(`{"a":` + strconv.FormatInt(a, 10) + `,"b":` + strconv.FormatInt(b, 10) + `}`)
	if w := do(t, r, http.MethodPost, "/api/v1/stock/articles/distinct-pairs", admin, body, "application/json"); w.Code != http.StatusNoContent {
		t.Fatalf("no son lo mismo = %d %s", w.Code, w.Body.String())
	}
	w = do(t, r, http.MethodGet, "/api/v1/stock/articles/duplicate-groups", admin, nil, "")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"groups":[]`) {
		t.Fatalf("grupos = %d %s; quiere groups: [] (no null)", w.Code, w.Body.String())
	}
}
