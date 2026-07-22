//go:build integration

package integration

import (
	"context"
	"testing"

	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
)

// El alta de productos (gerente/admin) más el orden por columna y el filtro por categoría de
// AdminListProducts: cubre de punta a punta el order-by con CASE (fácil de romper con un cast) y
// que el filtro por categoría incluye subcategorías.
func TestAdminCreateListSortAndCategoryFilter(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	admin := app.NewAdminService(st)

	var bebidas, calientes, postres int64
	if err := st.Pool.QueryRow(ctx, `insert into categories (name) values ('Bebidas') returning id`).Scan(&bebidas); err != nil {
		t.Fatalf("cat Bebidas: %v", err)
	}
	// 'Calientes' cuelga de 'Bebidas' (subcategoría): filtrar por la raíz debe incluir sus productos.
	if err := st.Pool.QueryRow(ctx, `insert into categories (name, parent_id) values ('Calientes', $1) returning id`, bebidas).Scan(&calientes); err != nil {
		t.Fatalf("cat Calientes: %v", err)
	}
	if err := st.Pool.QueryRow(ctx, `insert into categories (name) values ('Postres') returning id`).Scan(&postres); err != nil {
		t.Fatalf("cat Postres: %v", err)
	}

	mk := func(name string, cat int64, price string) {
		if _, err := admin.CreateProduct(ctx, name, cat, decimal.RequireFromString(price), false, false); err != nil {
			t.Fatalf("CreateProduct(%s): %v", name, err)
		}
	}
	mk("Latte", calientes, "50") // subcategoría de Bebidas
	mk("Espresso", calientes, "35")
	mk("Flan", postres, "60")

	// Orden por precio ascendente (ejercita el CASE numérico del order-by).
	page, err := admin.ListProducts(ctx, "", "", 0, "", "price", "asc", 25, 0)
	if err != nil {
		t.Fatalf("ListProducts price asc: %v", err)
	}
	got := []string{}
	for _, p := range page.Items {
		got = append(got, p.Name)
	}
	want := []string{"Espresso", "Latte", "Flan"}
	if len(got) != 3 || got[0] != want[0] || got[1] != want[1] || got[2] != want[2] {
		t.Fatalf("orden por precio asc = %v, want %v", got, want)
	}

	// Filtro por la categoría raíz 'Bebidas' → incluye los de la subcategoría 'Calientes' (2), no el Flan.
	beb, err := admin.ListProducts(ctx, "", "", bebidas, "", "name", "asc", 25, 0)
	if err != nil {
		t.Fatalf("ListProducts filtro categoría: %v", err)
	}
	if beb.Total != 2 {
		t.Fatalf("filtro por 'Bebidas' (incl. subcategoría) devolvió total=%d, want 2", beb.Total)
	}
	for _, p := range beb.Items {
		if p.Name == "Flan" {
			t.Fatalf("el filtro por 'Bebidas' no debe incluir 'Flan' (categoría Postres)")
		}
	}
}
