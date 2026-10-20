//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"testing"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
)

// GESTIÓN DE ARTÍCULOS DEL ALMACÉN (spec 042).

// stockAdjust deja un ajuste en el artículo como owner: el trigger de existencias suma la cantidad.
func stockAdjust(t *testing.T, st *store.Store, company, ingredient int64, qty string) {
	t.Helper()
	if _, err := st.Pool.Exec(context.Background(), `
		insert into stock_movements (company_id, item_type, ingredient_id, movement_type, quantity)
		values ($1, 'ingrediente', $2, 'ajuste', $3::numeric)`, company, ingredient, qty); err != nil {
		t.Fatalf("ajuste de %d: %v", ingredient, err)
	}
}

func articleNames(t *testing.T, ctx context.Context, svc *app.BackofficeService, f app.ArticleFilter) []string {
	t.Helper()
	rows, err := svc.ListArticles(ctx, f)
	if err != nil {
		t.Fatal(err)
	}
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = r.Name
	}
	return out
}

// US1: encontrar un artículo entre cientos sin bajar a mano (FR-001). El buscador ignora mayúsculas y
// acentos y también mira los empaques; los filtros funcionan solos y juntos.
func TestFindingAnArticle(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	co := makeCompany(t, st, "articulos-buscar")
	svc, ctx := asWaste(t, co)

	hielo := ingredientIn(t, st, co, "Hielo en bolsa", "g")
	leche := ingredientIn(t, st, co, "Leche entera", "ml")
	vasos := ingredientIn(t, st, co, "Vasos de 16 oz", "pieza")
	azucar := ingredientIn(t, st, co, "Azúcar", "g")
	cajeta := ingredientIn(t, st, co, "Cajeta", "ml")
	ctxOwner := context.Background()
	if _, err := st.Pool.Exec(ctxOwner, `update ingredients set current_cost = 0.006 where id = any($1)`, []int64{hielo, vasos, azucar}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Pool.Exec(ctxOwner, `update ingredients set is_active = false where id = $1`, cajeta); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Pool.Exec(ctxOwner, `insert into ingredient_presentations (company_id, ingredient_id, name, qty_in_base) values ($1, $2, 'Garrafa', 4000)`, co, leche); err != nil {
		t.Fatal(err)
	}
	var refri int64
	if err := st.Pool.QueryRow(ctxOwner, `insert into count_places (company_id, name, position) values ($1, 'Refri', 0) returning id`, co).Scan(&refri); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Pool.Exec(ctxOwner, `update ingredients set count_place_id = $2 where id = any($1)`, []int64{hielo, leche}, refri); err != nil {
		t.Fatal(err)
	}
	stockAdjust(t, st, co, azucar, "-250")
	stockAdjust(t, st, co, hielo, "27000")

	for _, c := range []struct {
		name string
		f    app.ArticleFilter
		want []string
	}{
		{"sin filtro: solo los activos, por nombre", app.ArticleFilter{}, []string{"Azúcar", "Hielo en bolsa", "Leche entera", "Vasos de 16 oz"}},
		{"«hie» encuentra el hielo", app.ArticleFilter{Q: "hie"}, []string{"Hielo en bolsa"}},
		{"«AZUCAR» sin acento encuentra «Azúcar»", app.ArticleFilter{Q: "AZUCAR"}, []string{"Azúcar"}},
		{"el nombre de un empaque también encuentra", app.ArticleFilter{Q: "garrafa"}, []string{"Leche entera"}},
		{"archivados: solo los archivados", app.ArticleFilter{Archived: true}, []string{"Cajeta"}},
		{"por lugar", app.ArticleFilter{PlaceID: &refri}, []string{"Hielo en bolsa", "Leche entera"}},
		{"sin lugar", app.ArticleFilter{NoPlace: true}, []string{"Azúcar", "Vasos de 16 oz"}},
		{"sin costo", app.ArticleFilter{NoCost: true}, []string{"Leche entera"}},
		{"existencia negativa", app.ArticleFilter{Negative: true}, []string{"Azúcar"}},
		{"filtros juntos: en el refri y sin costo", app.ArticleFilter{PlaceID: &refri, NoCost: true}, []string{"Leche entera"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := articleNames(t, ctx, svc, c.f); !slices.Equal(got, c.want) {
				t.Fatalf("= %v, quiere %v", got, c.want)
			}
		})
	}

	t.Run("cada renglón trae su existencia, su lugar y sus empaques", func(t *testing.T) {
		rows, err := svc.ListArticles(ctx, app.ArticleFilter{Q: "leche"})
		if err != nil || len(rows) != 1 {
			t.Fatalf("%v %v", rows, err)
		}
		r := rows[0]
		if r.CountPlace == nil || *r.CountPlace != "Refri" || len(r.Presentations) != 1 || r.Presentations[0].Name != "Garrafa" || r.BaseUnitKind != "volumen" {
			t.Fatalf("renglón = %+v", r)
		}
	})

	t.Run("la lista de una empresa no ve los artículos de otra, en los tres casos", func(t *testing.T) {
		inTheThreeCases(t, co, defaultCompanyID, func(t *testing.T, st *store.Store, ctx context.Context) {
			rows, err := app.NewBackofficeService(st, clock).ListArticles(ctx, app.ArticleFilter{Q: "hielo en bolsa"})
			// Sin empresa, la sucursal no se resuelve y la consulta se niega entera: no filtra nada.
			if errors.Is(err, domain.ErrBranchAmbiguous) {
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			for _, r := range rows {
				if r.ID == hielo {
					t.Fatalf("se vio el artículo %d de otra empresa", hielo)
				}
			}
		})
	})
}

// La lista se lee con `inventory.view`: el administrador y el gerente sí; el cajero, no (FR-013). Una
// empresa sin artículos recibe [] y no null.
func TestArticlesHTTPNeedsInventoryView(t *testing.T) {
	t.Parallel()
	r, _, token := newStockAPI(t)
	for _, c := range []struct {
		role string
		want int
	}{{"admin", http.StatusOK}, {"gerente", http.StatusOK}, {"cajero", http.StatusForbidden}, {"mesero", http.StatusForbidden}} {
		w := do(t, r, http.MethodGet, "/api/v1/stock/articles", token("art_"+c.role, c.role), nil, "")
		if w.Code != c.want {
			t.Errorf("%s = %d, quiere %d", c.role, w.Code, c.want)
		}
		if c.want == http.StatusOK {
			var raw json.RawMessage
			if err := json.Unmarshal(w.Body.Bytes(), &raw); err != nil || string(raw) == "null" {
				t.Errorf("%s: cuerpo %q; quiere un arreglo", c.role, w.Body.String())
			}
		}
	}
	admin := token("art_admin_mal", "admin")
	if w := do(t, r, http.MethodGet, "/api/v1/stock/articles?status=borrados", admin, nil, ""); w.Code != http.StatusBadRequest {
		t.Errorf("un estado desconocido = %d, quiere 400 (no se cae a «activos» en silencio)", w.Code)
	}
	if w := do(t, r, http.MethodGet, "/api/v1/stock/articles?placeId=abc", admin, nil, ""); w.Code != http.StatusBadRequest {
		t.Errorf("un lugar ilegible = %d, quiere 400", w.Code)
	}
}
