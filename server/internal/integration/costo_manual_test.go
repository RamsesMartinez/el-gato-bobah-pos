//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"

	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
)

// Costo capturado a mano desde «Editar producto» (2026-10-09). Antes no había forma de ponerle
// costo a un producto sin receta: el margen de la lista salía igual al precio.

type costoDeProducto struct {
	source  string
	manual  *decimal.Decimal
	current decimal.Decimal
}

func leerCosto(t *testing.T, st *store.Store, id int64) costoDeProducto {
	t.Helper()
	var c costoDeProducto
	if err := st.Pool.QueryRow(context.Background(),
		`select cost_source::text, manual_cost, current_cost from products where id = $1`, id).
		Scan(&c.source, &c.manual, &c.current); err != nil {
		t.Fatal(err)
	}
	return c
}

func costoManual(s string) *domain.ProductCostChange {
	v := decimal.RequireFromString(s)
	c, err := domain.NewProductCostChange(domain.CostSourceManual, &v)
	if err != nil {
		panic(err)
	}
	return &c
}

// productoConReceta crea un producto costeado por receta: 200 g de un insumo a $0.05/g = $10.
func productoConReceta(t *testing.T, st *store.Store, name string) int64 {
	t.Helper()
	ctx := context.Background()
	prod := makeProduct(t, st, name, decimal.RequireFromString("60"), false)
	var ing, rec int64
	if err := st.Pool.QueryRow(ctx, `
		insert into ingredients (company_id, name, base_unit_id, current_cost)
		select $1, $2, id, 0.05 from units where code = 'g' returning id`,
		defaultCompanyID, "Insumo "+name).Scan(&ing); err != nil {
		t.Fatal(err)
	}
	if err := st.Pool.QueryRow(ctx, `insert into recipes (company_id) values ($1) returning id`, defaultCompanyID).Scan(&rec); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Pool.Exec(ctx, `
		insert into recipe_items (company_id, recipe_id, ingredient_id, quantity, unit_id)
		select $1, $2, $3, 200, id from units where code = 'g'`, defaultCompanyID, rec, ing); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Pool.Exec(ctx,
		`update products set recipe_id = $2, cost_source = 'receta', current_cost = 10 where id = $1`, prod, rec); err != nil {
		t.Fatal(err)
	}
	return prod
}

func TestGuardarUnCostoManualLoDejaEnElMargen(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()
	admin := app.NewAdminService(st)
	prod := makeProduct(t, st, "Papas costo manual", decimal.RequireFromString("40"), false)

	if err := admin.UpdateProduct(ctx, app.UpdateProductInput{
		ID: prod, Name: "Papas costo manual", Price: decimal.RequireFromString("40"), Active: true,
		Cost: costoManual("23.404"),
	}); err != nil {
		t.Fatalf("guardar costo: %v", err)
	}
	c := leerCosto(t, st, prod)
	if c.source != "manual" || c.manual == nil || !c.manual.Equal(decimal.RequireFromString("23.40")) ||
		!c.current.Equal(decimal.RequireFromString("23.40")) {
		t.Fatalf("costo guardado = %+v, quiere manual 23.40 en manual_cost y current_cost", c)
	}

	// La lista (de donde sale la columna Margen) lo ve con su origen.
	page, err := admin.ListProducts(ctx, "", "Papas costo manual", 0, "", "", "", "", 0, 0)
	if err != nil || len(page.Items) != 1 {
		t.Fatalf("lista: %v %+v", err, page)
	}
	it := page.Items[0]
	if it.CostSource != "manual" || !it.CurrentCost.Equal(decimal.RequireFromString("23.40")) || it.HasRecipe {
		t.Fatalf("vista = %+v", it)
	}
}

// Sin costo en la petición no se toca: un cliente que no conoce el campo no puede borrar costos.
func TestSinCostoEnLaPeticionElCostoNoCambia(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	admin := app.NewAdminService(st)
	prod := productoConReceta(t, st, "Frappé quieto")

	if err := admin.UpdateProduct(context.Background(), app.UpdateProductInput{
		ID: prod, Name: "Frappé quieto", Price: decimal.RequireFromString("65"), Active: true,
	}); err != nil {
		t.Fatal(err)
	}
	if c := leerCosto(t, st, prod); c.source != "receta" || !c.current.Equal(decimal.RequireFromString("10")) {
		t.Fatalf("el costo cambió sin pedirlo: %+v", c)
	}
}

// Pasar a manual y regresar a «de su receta» recalcula desde la receta, no deja el monto manual.
func TestRegresarALaRecetaRecalculaElCosto(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()
	admin := app.NewAdminService(st)
	prod := productoConReceta(t, st, "Frappé ida y vuelta")
	in := app.UpdateProductInput{ID: prod, Name: "Frappé ida y vuelta", Price: decimal.RequireFromString("60"), Active: true}

	in.Cost = costoManual("15")
	if err := admin.UpdateProduct(ctx, in); err != nil {
		t.Fatal(err)
	}
	if c := leerCosto(t, st, prod); c.source != "manual" || !c.current.Equal(decimal.RequireFromString("15")) {
		t.Fatalf("a manual: %+v", c)
	}

	receta, _ := domain.NewProductCostChange(domain.CostSourceRecipe, nil)
	in.Cost = &receta
	if err := admin.UpdateProduct(ctx, in); err != nil {
		t.Fatal(err)
	}
	if c := leerCosto(t, st, prod); c.source != "receta" || !c.current.Equal(decimal.RequireFromString("10")) {
		t.Fatalf("de regreso a receta el costo debe salir de la receta ($10): %+v", c)
	}
}

func TestSinRecetaNoSePuedeCostearPorReceta(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	prod := makeProduct(t, st, "Agua sin receta", decimal.RequireFromString("20"), false)
	receta, _ := domain.NewProductCostChange(domain.CostSourceRecipe, nil)

	err := app.NewAdminService(st).UpdateProduct(context.Background(), app.UpdateProductInput{
		ID: prod, Name: "Agua sin receta", Price: decimal.RequireFromString("20"), Active: true, Cost: &receta,
	})
	if !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("quería ErrValidation, fue %v", err)
	}
	if c := leerCosto(t, st, prod); c.source != "manual" {
		t.Fatalf("el origen cambió: %+v", c)
	}
}

// Un combo se costea con sus componentes; un costo manual ahí lo borraría el siguiente recálculo.
func TestUnComboNoAceptaCostoManual(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	prod := makeProduct(t, st, "Combo costo", decimal.RequireFromString("90"), false)
	if _, err := st.Pool.Exec(context.Background(), `update products set type = 'combo' where id = $1`, prod); err != nil {
		t.Fatal(err)
	}
	err := app.NewAdminService(st).UpdateProduct(context.Background(), app.UpdateProductInput{
		ID: prod, Name: "Combo costo", Price: decimal.RequireFromString("90"), Active: true, Cost: costoManual("30"),
	})
	if !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("quería ErrValidation, fue %v", err)
	}
}

func TestAltaYDuplicadoConCostoManual(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()
	admin := app.NewAdminService(st)
	cat := categoriaDeEmpresa(t, st, defaultCompanyID, "Cat costo alta")
	costo := decimal.RequireFromString("12.5")

	id, err := admin.CreateProduct(ctx, "Té costo alta", cat, decimal.RequireFromString("35"), false, false, &costo)
	if err != nil {
		t.Fatal(err)
	}
	if c := leerCosto(t, st, id); c.source != "manual" || !c.current.Equal(costo) {
		t.Fatalf("alta: %+v", c)
	}

	otro := decimal.RequireFromString("14")
	dup, err := admin.DuplicateProduct(ctx, id, "Té costo alta grande", &otro)
	if err != nil {
		t.Fatal(err)
	}
	if c := leerCosto(t, st, dup); !c.current.Equal(otro) {
		t.Fatalf("duplicado con costo propio: %+v", c)
	}
	if c := leerCosto(t, st, id); !c.current.Equal(costo) {
		t.Fatalf("el original no debe cambiar: %+v", c)
	}

	sinCosto, err := admin.DuplicateProduct(ctx, id, "Té costo alta copia", nil)
	if err != nil {
		t.Fatal(err)
	}
	if c := leerCosto(t, st, sinCosto); !c.current.Equal(costo) {
		t.Fatalf("sin costo el duplicado hereda el del original: %+v", c)
	}

	negativo := decimal.RequireFromString("-1")
	if _, err := admin.CreateProduct(ctx, "Té negativo", cat, decimal.RequireFromString("35"), false, false, &negativo); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("costo negativo: %v", err)
	}
}

// Otra empresa no puede ponerle costo a un producto ajeno ni verlo en su lista.
func TestElCostoManualNoCruzaEmpresas(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	otra := makeCompany(t, st, "otra-costo-manual")
	prod := productoConReceta(t, st, "Frappé ajeno costo")

	inTheThreeCases(t, defaultCompanyID, otra, func(t *testing.T, ast *store.Store, ctx context.Context) {
		admin := app.NewAdminService(ast)
		err := admin.UpdateProduct(ctx, app.UpdateProductInput{
			ID: prod, Name: "Frappé ajeno costo", Price: decimal.RequireFromString("60"), Active: true,
			Cost: costoManual("1"),
		})
		if err == nil {
			t.Fatal("guardar costo sobre un producto ajeno debió fallar")
		}
		if c := leerCosto(t, st, prod); c.source != "receta" || !c.current.Equal(decimal.RequireFromString("10")) {
			t.Fatalf("el costo del producto ajeno cambió: %+v", c)
		}
		page, err := admin.ListProducts(ctx, "", "Frappé ajeno costo", 0, "", "", "", "", 0, 0)
		if err == nil && len(page.Items) != 0 {
			t.Fatalf("la lista ve el producto ajeno: %+v", page.Items)
		}
	})
}
