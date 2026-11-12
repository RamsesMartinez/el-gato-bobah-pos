//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
)

// Lo que hace un insumo archivado (spec 077, FR-008): deja de salir en el conteo, en las compras y en
// la lista de existencias (la de faltantes), y conserva su historial. Todo por las rutas de la
// pantalla, que es lo que el MCP despacha.

type suggestion struct {
	Source string `json:"source"`
	ItemID int64  `json:"itemId"`
}

func suggestionsFor(t *testing.T, r http.Handler, tok string, supplier int64, raw string) []suggestion {
	t.Helper()
	q := url.Values{"rawName": {raw}}
	if supplier != 0 {
		q.Set("supplierId", strconv.FormatInt(supplier, 10))
	}
	w := do(t, r, http.MethodGet, "/api/v1/articles/suggest?"+q.Encode(), tok, nil, "")
	if w.Code != http.StatusOK {
		t.Fatalf("sugerencias = %d %s", w.Code, w.Body.String())
	}
	var env struct {
		Items []suggestion `json:"items"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	return env.Items
}

func suggests(items []suggestion, id int64) (bool, string) {
	for _, s := range items {
		if s.ItemID == id {
			return true, s.Source
		}
	}
	return false, ""
}

type levelRow struct {
	ItemName string          `json:"item_name"`
	IsActive *bool           `json:"is_active"`
	OnHand   decimal.Decimal `json:"on_hand"`
}

func stockLevelsOf(t *testing.T, r http.Handler, tok string) []levelRow {
	t.Helper()
	w := do(t, r, http.MethodGet, "/api/v1/stock/levels", tok, nil, "")
	if w.Code != http.StatusOK {
		t.Fatalf("existencias = %d %s", w.Code, w.Body.String())
	}
	var env struct {
		Items []levelRow `json:"items"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	return env.Items
}

func levelNamed(rows []levelRow, name string) *levelRow {
	for i := range rows {
		if rows[i].ItemName == name {
			return &rows[i]
		}
	}
	return nil
}

func TestArchivedIngredientLeavesCountPurchasesAndShortages(t *testing.T) {
	t.Parallel()
	r, st, token := newStockAPI(t)
	admin := token("admin_archivado", "admin")
	ctx := context.Background()

	const archName = "Jarabe lavanda archivo"
	arch := ingredientIn(t, st, defaultCompanyID, archName, "ml")
	fresco := ingredientIn(t, st, defaultCompanyID, "Jarabe lavanda vigente", "ml")
	if _, err := st.Pool.Exec(ctx, `update ingredients set min_stock = 10 where id = $1`, arch); err != nil {
		t.Fatal(err)
	}
	// Una existencia que fue y volvió a cero: hay renglón en existencias (bajo su mínimo) y movimientos.
	stockAdjust(t, st, defaultCompanyID, arch, "3")
	stockAdjust(t, st, defaultCompanyID, arch, "-3")
	var supA, supC int64
	for name, dst := range map[string]*int64{"Proveedor A archivo": &supA, "Proveedor C archivo": &supC} {
		if err := st.Pool.QueryRow(ctx, `insert into suppliers (company_id, name) values ($1, $2) returning id`, defaultCompanyID, name).Scan(dst); err != nil {
			t.Fatal(err)
		}
	}
	const raw = "JARABE LAVANDA ARCHIVO 1L"
	if _, err := st.Pool.Exec(ctx, `insert into supplier_items (company_id, supplier_id, raw_name, norm_name, status, item_type, ingredient_id)
		values ($1, $2, $3, $4, 'mapeado', 'ingrediente', $5)`, defaultCompanyID, supA, raw, domain.SupplierItemKey("", raw), arch); err != nil {
		t.Fatal(err)
	}
	// Un producto de reventa archivado sigue en existencias, marcado (spec D7): desactivar productos no
	// es de esta feature.
	refresco := makeProduct(t, st, "Refresco archivado 077", decimal.NewFromInt(20), true)
	if _, err := st.Pool.Exec(ctx, `insert into stock_movements (company_id, item_type, product_id, movement_type, quantity)
		values ($1, 'producto', $2, 'ajuste', 5)`, defaultCompanyID, refresco); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Pool.Exec(ctx, `update products set is_active = false where id = $1`, refresco); err != nil {
		t.Fatal(err)
	}

	path := "/api/v1/stock/articles/" + strconv.FormatInt(arch, 10)
	if w := do(t, r, http.MethodPost, path+"/archive", admin, nil, ""); w.Code != http.StatusNoContent {
		t.Fatalf("archivar = %d %s", w.Code, w.Body.String())
	}

	t.Run("no sale en el conteo", func(t *testing.T) {
		w := do(t, r, http.MethodPost, "/api/v1/stock/counts", admin, nil, "")
		if w.Code != http.StatusCreated {
			t.Fatalf("abrir conteo = %d %s", w.Code, w.Body.String())
		}
		var view app.CountView
		if err := json.Unmarshal(w.Body.Bytes(), &view); err != nil {
			t.Fatal(err)
		}
		for _, it := range view.Items {
			if it.IngredientID == arch {
				t.Fatal("el conteo ofrece el insumo archivado")
			}
		}
		w = do(t, r, http.MethodPost, "/api/v1/stock/counts/"+strconv.FormatInt(view.ID, 10)+"/close", admin, nil, "")
		if w.Code != http.StatusOK || strings.Contains(w.Body.String(), archName) {
			t.Fatalf("cerrar el conteo = %d; «sin contar» no debe nombrar al archivado: %s", w.Code, w.Body.String())
		}
	})

	t.Run("no sale en el buscador de compras ni en la sugerencia por nombre", func(t *testing.T) {
		w := do(t, r, http.MethodGet, "/api/v1/articles?q="+url.QueryEscape("lavanda archivo"), admin, nil, "")
		if w.Code != http.StatusOK || strings.Contains(w.Body.String(), archName) {
			t.Fatalf("buscador = %d %s", w.Code, w.Body.String())
		}
		if ok, _ := suggests(suggestionsFor(t, r, admin, 0, "jarabe lavanda archivo"), arch); ok {
			t.Fatal("la sugerencia por nombre propone el archivado")
		}
	})

	t.Run("lo aprendido con el proveedor ya no lo propone; sale el activo parecido como sugerencia", func(t *testing.T) {
		items := suggestionsFor(t, r, admin, supA, raw)
		if ok, src := suggests(items, arch); ok {
			t.Fatalf("el renglón aprendido sigue proponiendo el archivado (%s): %+v", src, items)
		}
		if ok, src := suggests(items, fresco); !ok || src == "aprendido" {
			t.Fatalf("el activo parecido no sale como sugerencia (ok=%v, %s): %+v", ok, src, items)
		}
	})

	t.Run("lo aprendido con otro proveedor tampoco lo propone", func(t *testing.T) {
		if ok, _ := suggests(suggestionsFor(t, r, admin, supC, raw), arch); ok {
			t.Fatal("el renglón de otro proveedor propone el archivado")
		}
	})

	t.Run("no sale en existencias; el producto archivado sí, marcado, y cada renglón dice si está activo", func(t *testing.T) {
		rows := stockLevelsOf(t, r, admin)
		if levelNamed(rows, archName) != nil {
			t.Fatal("existencias lista el insumo archivado (y lo marcaría «bajo»)")
		}
		p := levelNamed(rows, "Refresco archivado 077")
		if p == nil || p.IsActive == nil || *p.IsActive {
			t.Fatalf("el producto archivado: %+v", p)
		}
		for _, row := range rows {
			if row.IsActive == nil {
				t.Fatalf("renglón sin is_active: %+v", row)
			}
		}
	})

	t.Run("su historial sigue legible", func(t *testing.T) {
		if w := do(t, r, http.MethodGet, "/api/v1/stock/movements", admin, nil, ""); w.Code != http.StatusOK || strings.Count(w.Body.String(), archName) < 2 {
			t.Fatalf("movimientos = %d; quiere los dos del archivado: %s", w.Code, w.Body.String())
		}
		if w := do(t, r, http.MethodGet, path+"/cost-history", admin, nil, ""); w.Code != http.StatusOK {
			t.Fatalf("historial de costo = %d %s", w.Code, w.Body.String())
		}
		if w := do(t, r, http.MethodGet, path+"/audit", admin, nil, ""); w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "ingredient.archived") {
			t.Fatalf("bitácora = %d %s", w.Code, w.Body.String())
		}
	})

	t.Run("reactivarlo lo devuelve a compras y existencias", func(t *testing.T) {
		if w := do(t, r, http.MethodPost, path+"/reactivate", admin, nil, ""); w.Code != http.StatusNoContent {
			t.Fatalf("reactivar = %d %s", w.Code, w.Body.String())
		}
		if ok, src := suggests(suggestionsFor(t, r, admin, supA, raw), arch); !ok || src != "aprendido" {
			t.Fatalf("reactivado, lo aprendido no vuelve (ok=%v, %s)", ok, src)
		}
		if row := levelNamed(stockLevelsOf(t, r, admin), archName); row == nil || row.IsActive == nil || !*row.IsActive {
			t.Fatalf("reactivado, existencias: %+v", row)
		}
	})
}

// Las tres consultas que cambia la 077 no alcanzan nada de otra empresa en los tres casos donde RLS ya
// falló.
func TestArchivedIngredientQueriesAreIsolated(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()
	other := makeCompany(t, st, "ajena-077")
	ing := ingredientIn(t, st, defaultCompanyID, "Insumo propio 077", "g")
	stockAdjust(t, st, defaultCompanyID, ing, "4")
	var sup int64
	if err := st.Pool.QueryRow(ctx, `insert into suppliers (company_id, name) values ($1, 'Proveedor 077') returning id`, defaultCompanyID).Scan(&sup); err != nil {
		t.Fatal(err)
	}
	const raw = "INSUMO PROPIO 077"
	if _, err := st.Pool.Exec(ctx, `insert into supplier_items (company_id, supplier_id, raw_name, norm_name, status, item_type, ingredient_id)
		values ($1, $2, $3, $4, 'mapeado', 'ingrediente', $5)`, defaultCompanyID, sup, raw, domain.SupplierItemKey("", raw), ing); err != nil {
		t.Fatal(err)
	}
	inTheThreeCases(t, defaultCompanyID, other, func(t *testing.T, ast *store.Store, actx context.Context) {
		svc := app.NewBackofficeService(ast, nil)
		if rows, err := svc.StockLevels(actx); err == nil {
			for _, row := range rows {
				if row.ItemName == "Insumo propio 077" {
					t.Fatal("existencias de otra empresa")
				}
			}
		}
		if items, err := svc.SuggestForLine(actx, sup, "", raw); err == nil {
			for _, s := range items {
				if s.ItemID == ing {
					t.Fatalf("sugerencia de otra empresa (%s)", s.Source)
				}
			}
		}
		if items, err := svc.SuggestForLine(actx, 0, "", "insumo propio 077"); err == nil {
			for _, s := range items {
				if s.ItemID == ing {
					t.Fatalf("sugerencia por parecido de otra empresa (%s)", s.Source)
				}
			}
		}
	})
}
