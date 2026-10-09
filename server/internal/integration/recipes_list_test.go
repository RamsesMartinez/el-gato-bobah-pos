//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"
	"uuid"

	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
)

// LA LISTA DE RECETAS: lo que falta, lo que vino de FUDO y lo listo, con lo más vendido primero.
func TestRecipeListUnderTheAppRole(t *testing.T) {
	t.Parallel()
	owner := newTestStore(t)
	ctx := context.Background()
	cashier := makeUser(t, owner, "cajero_recetas", "cajero")
	admin := makeUser(t, owner, "admin_recetas", "admin")
	abrirCajaPrincipal(t, owner, cashier)

	milk := makeIngredient(t, owner, "Leche recetas")
	tapioca := makeIngredient(t, owner, "Tapióca recetas")
	var drinks, hot int64
	if err := owner.Pool.QueryRow(ctx, `insert into categories (name) values ('Bebidas recetas') returning id`).Scan(&drinks); err != nil {
		t.Fatal(err)
	}
	if err := owner.Pool.QueryRow(ctx, `insert into categories (name, parent_id) values ('Calientes recetas', $1) returning id`, drinks).Scan(&hot); err != nil {
		t.Fatal(err)
	}
	product := func(name string, cat int64, track bool) int64 {
		var id int64
		if err := owner.Pool.QueryRow(ctx, `insert into products (name, category_id, price, track_stock) values ($1, $2, 50, $3) returning id`,
			name, cat, track).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	latte := product("Latte recetas", hot, false)
	cafe := product("Café recetas", hot, false)
	mocha := product("Moka recetas", drinks, false)
	soda := product("Refresco recetas", drinks, true)
	hidden := product("Archivado recetas", drinks, false)
	if _, err := owner.Pool.Exec(ctx, `update products set is_active = false where id = $1`, hidden); err != nil {
		t.Fatal(err)
	}
	if _, err := owner.Pool.Exec(ctx, `update products set recipe_id = $1, composition_status = 'estimated' where id = $2`,
		makeRecipe(t, owner, map[int64]string{milk: "200", tapioca: "45"}), latte); err != nil {
		t.Fatal(err)
	}
	if _, err := owner.Pool.Exec(ctx, `update products set composition_status = 'confirmed', composition_confirmed_at = now() where id = $1`, soda); err != nil {
		t.Fatal(err)
	}

	// Ventas: el café se vende más que la moka, para el orden.
	orders := app.NewOrdersService(owner, clock)
	for _, sale := range []struct {
		id  int64
		qty string
	}{{cafe, "3"}, {mocha, "1"}, {latte, "2"}} {
		if _, err := orders.Create(ctx, app.CreateOrderCmd{
			ClientUUID: uuid.New(), ServiceType: "mostrador", OpenedBy: cashier,
			Lines: []domain.OrderLineInput{{ProductID: sale.id, Qty: decimal.RequireFromString(sale.qty)}},
		}); err != nil {
			t.Fatal(err)
		}
	}

	st := appRoleStore(t)
	tctx, release, err := st.AcquireTenant(ctx, defaultCompanyID)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	svc := app.NewAdminService(st)
	list := func(f domain.RecipeFilter) app.RecipePage {
		t.Helper()
		if f.Limit == 0 {
			f.Limit = 25
		}
		if f.Sort == "" {
			f.Sort = domain.RecipeSortSales
		}
		if f.Kind == "" {
			f.Kind = domain.RecipeKindProduct
		}
		page, err := svc.ListRecipes(tctx, f, fixedNow)
		if err != nil {
			t.Fatal(err)
		}
		return page
	}
	names := func(p app.RecipePage) []string {
		var out []string
		for _, r := range p.Items {
			out = append(out, r.Name)
		}
		return out
	}

	t.Run("pendientes, lo más vendido primero, sin archivados", func(t *testing.T) {
		got := names(list(domain.RecipeFilter{Status: domain.RecipeStatusPending, Query: "recetas"}))
		if len(got) != 2 || got[0] != "Café recetas" || got[1] != "Moka recetas" {
			t.Fatalf("pendientes: %v", got)
		}
	})
	t.Run("lo de FUDO está por revisar, con su resumen y sus ventas", func(t *testing.T) {
		p := list(domain.RecipeFilter{Status: domain.RecipeStatusReview, Query: "recetas"})
		if len(p.Items) != 1 || p.Items[0].Name != "Latte recetas" {
			t.Fatalf("por revisar: %v", names(p))
		}
		r := p.Items[0]
		// makeRecipe recorre un mapa: el orden de los insumos sale al azar.
		if (r.Summary != "200 g Leche recetas · 45 g Tapióca recetas" && r.Summary != "45 g Tapióca recetas · 200 g Leche recetas") ||
			r.Lines != 2 || !r.SoldPerMonth.Equal(decimal.RequireFromString("2")) {
			t.Fatalf("renglón: %+v", r)
		}
	})
	// FUDO guarda en kilos: «0.2 kg Leche» se lee peor que «200 g Leche», y la hoja ya lo muestra así.
	t.Run("el resumen dice gramos lo que no llega a un kilo", func(t *testing.T) {
		if _, err := owner.Pool.Exec(ctx, `
			update recipe_items set quantity = case when ingredient_id = $1 then 0.2 else 1.5 end,
			       unit_id = (select id from units where code = 'kg')
			 where recipe_id = (select recipe_id from products where id = $2)`, milk, latte); err != nil {
			t.Fatal(err)
		}
		r := list(domain.RecipeFilter{Status: domain.RecipeStatusReview, Query: "recetas"}).Items[0]
		if r.Summary != "200 g Leche recetas · 1.5 kg Tapióca recetas" && r.Summary != "1.5 kg Tapióca recetas · 200 g Leche recetas" {
			t.Fatalf("resumen: %q", r.Summary)
		}
	})
	t.Run("lo que se descuenta solo está listo", func(t *testing.T) {
		got := names(list(domain.RecipeFilter{Status: domain.RecipeStatusDone, Query: "recetas"}))
		if len(got) != 1 || got[0] != "Refresco recetas" {
			t.Fatalf("listas: %v", got)
		}
	})
	t.Run("buscar sin acentos, por nombre o por un insumo", func(t *testing.T) {
		if got := names(list(domain.RecipeFilter{Status: domain.RecipeStatusPending, Query: "cafe"})); len(got) != 1 || got[0] != "Café recetas" {
			t.Fatalf("«cafe» encuentra el Café: %v", got)
		}
		if got := names(list(domain.RecipeFilter{Status: domain.RecipeStatusReview, Query: "tapioca"})); len(got) != 1 || got[0] != "Latte recetas" {
			t.Fatalf("«tapioca» encuentra la receta que la lleva: %v", got)
		}
	})
	t.Run("la categoría incluye sus subcategorías", func(t *testing.T) {
		if got := names(list(domain.RecipeFilter{Status: domain.RecipeStatusPending, Category: hot})); len(got) != 1 || got[0] != "Café recetas" {
			t.Fatalf("subcategoría: %v", got)
		}
		if got := names(list(domain.RecipeFilter{Status: domain.RecipeStatusPending, Category: drinks, Query: "recetas"})); len(got) != 2 {
			t.Fatalf("la categoría padre incluye la hija: %v", got)
		}
	})
	t.Run("de la A a la Z y por páginas, con el total", func(t *testing.T) {
		p := list(domain.RecipeFilter{Status: domain.RecipeStatusPending, Sort: domain.RecipeSortAZ, Query: "recetas", Limit: 1})
		if len(p.Items) != 1 || p.Items[0].Name != "Café recetas" || p.Total != 2 {
			t.Fatalf("primera página: %v total %d", names(p), p.Total)
		}
		p = list(domain.RecipeFilter{Status: domain.RecipeStatusPending, Sort: domain.RecipeSortAZ, Query: "recetas", Limit: 1, Offset: 1})
		if len(p.Items) != 1 || p.Items[0].Name != "Moka recetas" {
			t.Fatalf("segunda página: %v", names(p))
		}
	})
	t.Run("los conteos por estado siguen la búsqueda; los de cada lista, no", func(t *testing.T) {
		p := list(domain.RecipeFilter{Status: domain.RecipeStatusPending, Query: "recetas"})
		if p.Counts.Pending != 2 || p.Counts.Review != 1 || p.Counts.Done != 1 {
			t.Fatalf("conteos de la búsqueda: %+v", p.Counts)
		}
		if p.Totals.Product.Pending < 2 || p.Totals.Product.Done < 1 {
			t.Fatalf("totales de productos: %+v", p.Totals)
		}
	})
	t.Run("confirmar las que se ven confirma solo las estimadas indicadas", func(t *testing.T) {
		n, err := svc.ConfirmRecipes(tctx, domain.RecipeKindProduct, []int64{latte, cafe}, admin)
		if err != nil {
			t.Fatal(err)
		}
		if n != 1 {
			t.Fatalf("solo el Latte estaba estimado: confirmó %d", n)
		}
		if got := names(list(domain.RecipeFilter{Status: domain.RecipeStatusPending, Query: "recetas"})); len(got) != 2 {
			t.Fatalf("el Café sigue pendiente, no «no gasta insumos»: %v", got)
		}
	})

	// Un extra y un preparado de esta empresa, para que la otra tenga algo que no debe ver.
	extra := opcionConTope(t, owner, "Toppings aislamiento recetas", "Perla aislamiento", decimal.RequireFromString("10"), 3)
	if _, err := owner.Pool.Exec(ctx, `update ingredients set is_prep = true, composition_status = 'estimated', recipe_id = $2, yield_qty = 1000 where id = $1`,
		milk, makeRecipe(t, owner, map[int64]string{tapioca: "100"})); err != nil {
		t.Fatal(err)
	}
	if _, err := owner.Pool.Exec(ctx, `update modifier_options set composition_status = 'estimated' where id = $1`, extra); err != nil {
		t.Fatal(err)
	}
	other := makeCompany(t, owner, "empresa-otra-recetas")
	inTheThreeCases(t, defaultCompanyID, other, func(t *testing.T, st *store.Store, ctx context.Context) {
		svc := app.NewAdminService(st)
		page, err := svc.ListRecipes(ctx, domain.RecipeFilter{Kind: domain.RecipeKindProduct, Status: domain.RecipeStatusPending, Sort: domain.RecipeSortSales, Limit: 25}, fixedNow)
		if err == nil {
			for _, r := range page.Items {
				if r.ID == cafe || r.ID == mocha {
					t.Fatalf("se vio la receta de otra empresa: %s", r.Name)
				}
			}
		}
		if n, _ := svc.ConfirmRecipes(ctx, domain.RecipeKindProduct, []int64{latte}, admin); n != 0 {
			t.Fatal("se confirmó la receta de otra empresa")
		}
		// Extras y preparados tienen su propio SQL: cada uno se prueba, no se infiere del de productos.
		for _, k := range []domain.RecipeKind{domain.RecipeKindExtra, domain.RecipeKindPrep} {
			for _, status := range []domain.RecipeStatus{domain.RecipeStatusPending, domain.RecipeStatusReview, domain.RecipeStatusDone} {
				page, err := svc.ListRecipes(ctx, domain.RecipeFilter{Kind: k, Status: status, Sort: domain.RecipeSortAZ, Limit: 100}, fixedNow)
				if err != nil {
					continue
				}
				for _, r := range page.Items {
					if r.ID == extra || r.ID == milk {
						t.Fatalf("%s %s: se vio %s, de otra empresa", k, status, r.Name)
					}
				}
			}
		}
		if n, _ := svc.ConfirmRecipes(ctx, domain.RecipeKindExtra, []int64{extra}, admin); n != 0 {
			t.Fatal("se confirmó un extra de otra empresa")
		}
		if n, _ := svc.ConfirmRecipes(ctx, domain.RecipeKindPrep, []int64{milk}, admin); n != 0 {
			t.Fatal("se confirmó un preparado de otra empresa")
		}
	})

	if _, err := svc.ListRecipes(tctx, domain.RecipeFilter{Kind: "x", Status: domain.RecipeStatusPending, Sort: domain.RecipeSortSales, Limit: 25}, fixedNow); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("un filtro desconocido se rechaza: %v", err)
	}
}

// LOS EXTRAS: su grupo, sus ventas como extra y los que se llaman igual en otro grupo.
func TestRecipeListOfExtras(t *testing.T) {
	t.Parallel()
	owner := newTestStore(t)
	ctx := context.Background()
	cashier := makeUser(t, owner, "cajero_extras_rec", "cajero")
	abrirCajaPrincipal(t, owner, cashier)
	tea := makeProduct(t, owner, "Té extras recetas", decimal.RequireFromString("40"), false)
	topping := opcionConTope(t, owner, "Toppings recetas", "Perla recetas", decimal.RequireFromString("10"), 3, tea)
	twin := opcionConTope(t, owner, "Toppings frappé recetas", "Perla recetas", decimal.RequireFromString("10"), 3)
	if _, err := app.NewOrdersService(owner, clock).Create(ctx, app.CreateOrderCmd{
		ClientUUID: uuid.New(), ServiceType: "mostrador", OpenedBy: cashier,
		Lines: []domain.OrderLineInput{{ProductID: tea, Qty: decimal.RequireFromString("2"), Modifiers: []domain.OrderModInput{{OptionID: topping, Qty: 2}}}},
	}); err != nil {
		t.Fatal(err)
	}

	st := appRoleStore(t)
	tctx, release, err := st.AcquireTenant(ctx, defaultCompanyID)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	svc := app.NewAdminService(st)
	page, err := svc.ListRecipes(tctx, domain.RecipeFilter{Kind: domain.RecipeKindExtra, Status: domain.RecipeStatusPending, Sort: domain.RecipeSortSales, Query: "perla", Limit: 25}, fixedNow)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 2 || page.Items[0].ID != topping || page.Items[0].Group != "Toppings recetas" || !page.Items[0].SoldPerMonth.Equal(decimal.RequireFromString("4")) {
		t.Fatalf("2 tés con 2 perlas cada uno = 4 perlas, la del grupo vendido primero: %+v", page.Items)
	}
	v, err := svc.Composition(tctx, app.CompositionOfOption, topping)
	if err != nil {
		t.Fatal(err)
	}
	if len(v.SameName) != 1 || v.SameName[0].ID != twin || v.SameName[0].Group != "Toppings frappé recetas" {
		t.Fatalf("el extra que se llama igual en otro grupo: %+v", v.SameName)
	}
}
