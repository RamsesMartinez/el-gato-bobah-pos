//go:build integration

package integration

import (
	"context"
	"testing"

	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/fudoimport"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
)

// LA CARGA DE FUDO LLENA SOLO LO QUE FALTA, Y SOLO EN SU EMPRESA (spec 028).
//
// Corre como owner, que salta RLS: si una consulta olvida la empresa, el catálogo de la otra —que
// aquí tiene los mismos nombres a propósito— recibe las recetas. Y lo capturado a mano no se pisa.
func TestFudoCompositionsFillOnlyWhatIsMissingInTheirCompany(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()
	other := makeCompany(t, st, "empresa-otra-fudo")

	type ids struct{ drink, captured, extra, milk, sugar, syrup int64 }
	seed := func(company int64) ids {
		var x ids
		var cat, group int64
		q := func(sql string, dst *int64, args ...any) {
			t.Helper()
			if err := st.Pool.QueryRow(ctx, sql, args...).Scan(dst); err != nil {
				t.Fatalf("%s: %v", sql, err)
			}
		}
		q(`insert into categories (company_id, name) values ($1, 'Cat FUDO') returning id`, &cat, company)
		q(`insert into products (company_id, name, category_id, price) values ($1, 'Bebida Prueba FUDO', $2, 50) returning id`, &x.drink, company, cat)
		q(`insert into products (company_id, name, category_id, price) values ($1, 'Capturada Prueba FUDO', $2, 50) returning id`, &x.captured, company, cat)
		var unused int64
		q(`insert into products (company_id, name, category_id, price) values ($1, 'Sin Insumo Prueba FUDO', $2, 50) returning id`, &unused, company, cat)
		q(`insert into modifier_groups (company_id, name) values ($1, 'Grupo FUDO') returning id`, &group, company)
		q(`insert into modifier_options (company_id, group_id, name, price_delta) values ($1, $2, 'Extra Prueba FUDO', 5) returning id`, &x.extra, company, group)
		for _, in := range []struct {
			name, unit string
			dst        *int64
		}{{"Leche Prueba FUDO", "ml", &x.milk}, {"Azucar Prueba FUDO", "g", &x.sugar}, {"Jarabe Prueba FUDO", "ml", &x.syrup}} {
			q(`insert into ingredients (company_id, name, base_unit_id) select $1, $2, id from units where code = $3 returning id`, in.dst, company, in.name, in.unit)
		}
		return x
	}
	a, b := seed(defaultCompanyID), seed(other)

	// Lo capturado a mano: tiene receta propia y está confirmado.
	var manual int64
	if err := st.Pool.QueryRow(ctx, `insert into recipes default values returning id`).Scan(&manual); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Pool.Exec(ctx, `
		update products set recipe_id = $1, composition_status = 'confirmed', composition_confirmed_at = now()
		 where id = $2`, manual, a.captured); err != nil {
		t.Fatal(err)
	}

	src, err := fudoimport.ReadSources("../fudoimport/testdata")
	if err != nil {
		t.Fatal(err)
	}
	run := func() (fudoimport.Applied, fudoimport.Plan) {
		t.Helper()
		tx, err := st.Pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = tx.Rollback(ctx) }()
		cat, err := fudoimport.LoadCatalog(ctx, tx, defaultCompanyID)
		if err != nil {
			t.Fatal(err)
		}
		plan := fudoimport.PlanCompositions(src, cat)
		applied, err := fudoimport.Apply(ctx, tx, defaultCompanyID, plan)
		if err != nil {
			t.Fatal(err)
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		return applied, plan
	}
	applied, plan := run()

	if applied != (fudoimport.Applied{Products: 1, Options: 1, Preps: 1}) {
		t.Fatalf("se esperaba la bebida, el extra y el jarabe: %+v (reporte %+v)", applied, plan.Report)
	}
	if len(plan.Report.MissingIngredients) != 1 || len(plan.Report.Packages) != 1 {
		t.Fatalf("el insumo inexistente y el paquete se reportan: %+v", plan.Report)
	}

	status := func(table string, id int64) string {
		var s *string
		if err := st.Pool.QueryRow(ctx, `select composition_status from `+table+` where id = $1`, id).Scan(&s); err != nil {
			t.Fatal(err)
		}
		if s == nil {
			return ""
		}
		return *s
	}
	if status("products", a.drink) != "estimated" || status("modifier_options", a.extra) != "estimated" || status("ingredients", a.syrup) != "estimated" {
		t.Fatal("lo cargado queda estimado")
	}
	var milk decimal.Decimal
	if err := st.Pool.QueryRow(ctx, `
		select ri.quantity from products p join recipe_items ri on ri.recipe_id = p.recipe_id where p.id = $1`, a.drink).Scan(&milk); err != nil {
		t.Fatal(err)
	}
	if !milk.Equal(decimal.RequireFromString("250")) {
		t.Fatalf("0.25 L de leche son 250 ml: %s", milk)
	}
	var manualAfter *int64
	if err := st.Pool.QueryRow(ctx, `select recipe_id from products where id = $1`, a.captured).Scan(&manualAfter); err != nil {
		t.Fatal(err)
	}
	if manualAfter == nil || *manualAfter != manual || status("products", a.captured) != "confirmed" {
		t.Fatal("lo capturado a mano se pisó")
	}
	for _, c := range []struct {
		table string
		id    int64
	}{{"products", b.drink}, {"modifier_options", b.extra}, {"ingredients", b.syrup}} {
		if status(c.table, c.id) != "" {
			t.Fatalf("la otra empresa recibió composición en %s %d", c.table, c.id)
		}
	}

	if again, _ := run(); again != (fudoimport.Applied{}) {
		t.Fatalf("correrla dos veces no escribe nada la segunda: %+v", again)
	}
	var _ *store.Store = st
}
