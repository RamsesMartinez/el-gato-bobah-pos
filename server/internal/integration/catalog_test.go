//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"

	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
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

// Duplicar un producto copia también sus relaciones (grupos de modificadores y canales) al nuevo
// producto, sin tocar el original.
func TestDuplicateProductClonesRelations(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	admin := app.NewAdminService(st)

	var catID int64
	if err := st.Pool.QueryRow(ctx, `insert into categories (name) values ('Bebidas') returning id`).Scan(&catID); err != nil {
		t.Fatalf("categoria: %v", err)
	}
	srcID, err := admin.CreateProduct(ctx, "Latte", catID, decimal.RequireFromString("50"), true, false)
	if err != nil {
		t.Fatalf("CreateProduct: %v", err)
	}
	// Relación 1: un grupo de modificadores ligado al producto de origen.
	var groupID int64
	if err := st.Pool.QueryRow(ctx, `insert into modifier_groups (name) values ('Tamaño') returning id`).Scan(&groupID); err != nil {
		t.Fatalf("modifier_group: %v", err)
	}
	if _, err := st.Pool.Exec(ctx,
		`insert into product_modifier_groups (product_id, group_id, min_select, max_select, position) values ($1,$2,1,1,0)`,
		srcID, groupID); err != nil {
		t.Fatalf("product_modifier_group: %v", err)
	}
	// Relación 2: visibilidad de canal (el canal 'pos' viene sembrado para la empresa 1).
	if _, err := st.Pool.Exec(ctx,
		`insert into product_channels (product_id, channel_id, visibility)
		 select $1, id, 'oculto' from channels where code = 'pos'`, srcID); err != nil {
		t.Fatalf("product_channel: %v", err)
	}

	newID, err := admin.DuplicateProduct(ctx, srcID, "Latte grande")
	if err != nil {
		t.Fatalf("DuplicateProduct: %v", err)
	}
	if newID == srcID {
		t.Fatal("el clon reutilizó el id del original")
	}

	count := func(q string, arg int64) int {
		var n int
		if err := st.Pool.QueryRow(ctx, q, arg).Scan(&n); err != nil {
			t.Fatalf("count %q: %v", q, err)
		}
		return n
	}
	if got := count(`select count(*) from product_modifier_groups where product_id = $1`, newID); got != 1 {
		t.Fatalf("grupos del clon = %d, want 1", got)
	}
	if got := count(`select count(*) from product_channels where product_id = $1`, newID); got != 1 {
		t.Fatalf("canales del clon = %d, want 1", got)
	}
	// El precio y la categoría se copiaron; el nombre es el nuevo.
	var name string
	var price decimal.Decimal
	var cat int64
	if err := st.Pool.QueryRow(ctx, `select name, price, category_id from products where id = $1`, newID).Scan(&name, &price, &cat); err != nil {
		t.Fatalf("leer clon: %v", err)
	}
	if name != "Latte grande" || !price.Equal(decimal.RequireFromString("50")) || cat != catID {
		t.Fatalf("clon = (%s, %s, cat %d), want (Latte grande, 50, cat %d)", name, price, cat, catID)
	}
}

// Crear o duplicar con un nombre ya existente (case-insensitive, por el citext) → ErrDuplicateName
// (que es un ErrConflict → 409), nunca un 500.
func TestProductDuplicateNameRejected(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	admin := app.NewAdminService(st)

	var catID int64
	if err := st.Pool.QueryRow(ctx, `insert into categories (name) values ('Cafés') returning id`).Scan(&catID); err != nil {
		t.Fatalf("categoria: %v", err)
	}
	if _, err := admin.CreateProduct(ctx, "Café", catID, decimal.RequireFromString("30"), false, false); err != nil {
		t.Fatalf("CreateProduct: %v", err)
	}

	// Alta con el mismo nombre (distinto case) → duplicado.
	_, err := admin.CreateProduct(ctx, "café", catID, decimal.RequireFromString("40"), false, false)
	if !errors.Is(err, domain.ErrDuplicateName) || !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("CreateProduct nombre duplicado = %v, want ErrDuplicateName/ErrConflict", err)
	}

	// Duplicar hacia un nombre ya usado → mismo rechazo.
	src, err := admin.CreateProduct(ctx, "Chai", catID, decimal.RequireFromString("45"), false, false)
	if err != nil {
		t.Fatalf("CreateProduct Chai: %v", err)
	}
	if _, err := admin.DuplicateProduct(ctx, src, "Café"); !errors.Is(err, domain.ErrDuplicateName) {
		t.Fatalf("DuplicateProduct a nombre existente = %v, want ErrDuplicateName", err)
	}
}
