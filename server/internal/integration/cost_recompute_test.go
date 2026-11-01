//go:build integration

package integration

import (
	"context"
	"errors"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
)

// EL COSTO VIGENTE SIGUE A LA RECETA Y A SUS INSUMOS (issue #145).
//
// Guardar la receta de un extra, confirmarla o corregir el costo de un insumo dejaba el costo
// guardado como estaba: un extra nuevo quedaba en $0 y uno editado con el costo de su receta
// anterior. El margen por producto y los reportes de costo salían de ese número.

// costCatalog es un catálogo chico en el que cada costo guardado ya cuadra con su receta: lo
// que cambie después solo lo puede mover la acción que se prueba.
//
//	leche     insumo        $0.02/g
//	té        insumo        $0.10/g
//	base      preparado     500 g leche + 100 g té, rinde 1000 g → $0.02/g
//	frappé    producto      200 g de base                        → $4
//	paquete   paquete       2 frappés                            → $8
//	extraLeche   extra      100 g leche                          → $2
//	extraFrappé  extra      ligado al frappé                     → $4
//	nuevo        extra      sin receta                           → $0
type costCatalog struct {
	company                        int64
	leche, te, base                int64
	frappe, paquete                int64
	extraLeche, extraFrappe, nuevo int64
	admin                          int64
}

func seedCostCatalog(t *testing.T, st *store.Store, company int64, suffix string) costCatalog {
	t.Helper()
	ctx := context.Background()
	c := costCatalog{company: company}
	tx, err := st.Pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	// El dueño sella cada insert con la empresa del GUC: aquí, la del catálogo.
	if _, err := tx.Exec(ctx, `select set_config('app.company_id', $1, true)`, strconv.FormatInt(company, 10)); err != nil {
		t.Fatal(err)
	}
	one := func(dst *int64, sql string, args ...any) {
		t.Helper()
		if err := tx.QueryRow(ctx, sql, args...).Scan(dst); err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
	}
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := tx.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
	}
	insumo := func(dst *int64, name, cost string) {
		one(dst, `insert into ingredients (name, base_unit_id, current_cost)
			select $1, id, $2::numeric from units where code = 'g' returning id`, name+suffix, cost)
	}
	receta := func(items ...any) int64 {
		var rid int64
		one(&rid, `insert into recipes default values returning id`)
		for i := 0; i < len(items); i += 2 {
			exec(`insert into recipe_items (recipe_id, ingredient_id, quantity, unit_id)
				select $1, $2, $3::numeric, id from units where code = 'g'`, rid, items[i], items[i+1])
		}
		return rid
	}
	insumo(&c.leche, "Leche costo", "0.02")
	insumo(&c.te, "Té costo", "0.10")
	baseRecipe := receta(c.leche, "500", c.te, "100")
	one(&c.base, `insert into ingredients (name, base_unit_id, current_cost, is_prep, recipe_id, yield_qty)
		select $1, id, 0.02, true, $2, 1000 from units where code = 'g' returning id`, "Base costo"+suffix, baseRecipe)

	var cat int64
	one(&cat, `insert into categories (name) values ($1) returning id`, "Bebidas costo"+suffix)
	one(&c.frappe, `insert into products (name, category_id, price, recipe_id, cost_source, current_cost)
		values ($1, $2, 60, $3, 'receta', 4) returning id`, "Frappé costo"+suffix, cat, receta(c.base, "200"))
	one(&c.paquete, `insert into products (name, category_id, price, type, current_cost)
		values ($1, $2, 100, 'combo', 8) returning id`, "Paquete costo"+suffix, cat)
	var slot int64
	one(&slot, `insert into combo_slots (combo_id, name, min_select, max_select) values ($1, 'Frappés', 2, 2) returning id`, c.paquete)
	exec(`insert into combo_slot_products (slot_id, product_id, is_default) values ($1, $2, true)`, slot, c.frappe)

	var grupo int64
	one(&grupo, `insert into modifier_groups (name) values ($1) returning id`, "Extras costo"+suffix)
	one(&c.extraLeche, `insert into modifier_options (group_id, name, price_delta, recipe_id, current_cost, composition_status, composition_confirmed_at)
		values ($1, 'Extra leche', 10, $2, 2, 'confirmed', now()) returning id`, grupo, receta(c.leche, "100"))
	one(&c.extraFrappe, `insert into modifier_options (group_id, name, price_delta, linked_product_id, current_cost, composition_status, composition_confirmed_at)
		values ($1, 'Extra frappé', 30, $2, 4, 'confirmed', now()) returning id`, grupo, c.frappe)
	one(&c.nuevo, `insert into modifier_options (group_id, name, price_delta) values ($1, 'Extra té', 10) returning id`, grupo)
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	c.admin = makeUserIn(t, st, company, "admin_costo"+suffix, "admin")
	return c
}

// storedCost lee el costo vigente como lo guarda la base (lo que leen el margen y los reportes).
func storedCost(t *testing.T, st *store.Store, table string, id int64) decimal.Decimal {
	t.Helper()
	var v *decimal.Decimal
	sql := map[string]string{
		"ingredients":      `select current_cost from ingredients where id = $1`,
		"products":         `select current_cost from products where id = $1`,
		"modifier_options": `select current_cost from modifier_options where id = $1`,
	}[table]
	if err := st.Pool.QueryRow(context.Background(), sql, id).Scan(&v); err != nil {
		t.Fatalf("leer costo de %s %d: %v", table, id, err)
	}
	if v == nil {
		return decimal.Zero
	}
	return *v
}

func wantCost(t *testing.T, st *store.Store, what, table string, id int64, want string) {
	t.Helper()
	if got := storedCost(t, st, table, id); !got.Equal(decimal.RequireFromString(want)) {
		t.Errorf("%s: el costo guardado es %s y su receta dice %s", what, got, want)
	}
}

// tenantCtx entrega el contexto de la empresa bajo el rol de la API, con la persona que actúa.
func tenantCtx(t *testing.T, st *store.Store, company, actor int64) context.Context {
	t.Helper()
	ctx, release, err := st.AcquireTenant(context.Background(), company)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(release)
	return app.WithActorUser(ctx, actor)
}

func inGrams(t *testing.T, st *store.Store, ing int64, qty string) app.CompositionInputItem {
	return app.CompositionInputItem{IngredientID: ing, Quantity: decimal.RequireFromString(qty), UnitID: unitID(t, st, "g")}
}

func TestANewOptionWithARecipeGetsItsCost(t *testing.T) {
	t.Parallel()
	owner := newTestStore(t)
	k := seedCostCatalog(t, owner, defaultCompanyID, "")
	st := appRoleStore(t)
	ctx := tenantCtx(t, st, k.company, k.admin)

	if err := app.NewAdminService(st).SaveComposition(ctx, app.CompositionOfOption, k.nuevo,
		app.CompositionRequest{Items: []app.CompositionInputItem{inGrams(t, owner, k.te, "50")}}, k.admin); err != nil {
		t.Fatal(err)
	}
	wantCost(t, owner, "extra nuevo con 50 g de té", "modifier_options", k.nuevo, "5")
}

func TestEditingAnOptionRecipeMovesItsCost(t *testing.T) {
	t.Parallel()
	owner := newTestStore(t)
	k := seedCostCatalog(t, owner, defaultCompanyID, "")
	st := appRoleStore(t)
	ctx := tenantCtx(t, st, k.company, k.admin)
	svc := app.NewAdminService(st)

	if err := svc.SaveComposition(ctx, app.CompositionOfOption, k.extraLeche,
		app.CompositionRequest{Items: []app.CompositionInputItem{inGrams(t, owner, k.leche, "150")}}, k.admin); err != nil {
		t.Fatal(err)
	}
	wantCost(t, owner, "extra de leche con 150 g", "modifier_options", k.extraLeche, "3")

	// Ligarlo a un producto también lo costea con el producto.
	if err := svc.SaveComposition(ctx, app.CompositionOfOption, k.extraLeche,
		app.CompositionRequest{LinkedProductID: &k.frappe}, k.admin); err != nil {
		t.Fatal(err)
	}
	wantCost(t, owner, "extra de leche ligado al frappé", "modifier_options", k.extraLeche, "4")
}

func TestCorrectingAnIngredientCostMovesEverythingThatUsesIt(t *testing.T) {
	t.Parallel()
	owner := newTestStore(t)
	k := seedCostCatalog(t, owner, defaultCompanyID, "")
	st := appRoleStore(t)
	ctx := tenantCtx(t, st, k.company, k.admin)

	if err := app.NewBackofficeService(st, clock).CorrectArticleCost(ctx, k.leche, decimal.RequireFromString("0.04")); err != nil {
		t.Fatal(err)
	}
	// base = (500 × 0.04 + 100 × 0.10) / 1000 = 0.03; frappé = 200 × 0.03 = 6.
	wantCost(t, owner, "extra de leche (100 g)", "modifier_options", k.extraLeche, "4")
	wantCost(t, owner, "insumo preparado que lleva leche", "ingredients", k.base, "0.03")
	wantCost(t, owner, "producto hecho con el preparado", "products", k.frappe, "6")
	wantCost(t, owner, "paquete de dos frappés", "products", k.paquete, "12")
	wantCost(t, owner, "extra ligado al frappé", "modifier_options", k.extraFrappe, "6")
}

func TestSavingAPreparedIngredientMovesWhatUsesIt(t *testing.T) {
	t.Parallel()
	owner := newTestStore(t)
	k := seedCostCatalog(t, owner, defaultCompanyID, "")
	st := appRoleStore(t)
	ctx := tenantCtx(t, st, k.company, k.admin)
	rinde := decimal.RequireFromString("1000")

	if err := app.NewAdminService(st).SaveComposition(ctx, app.CompositionOfIngredient, k.base, app.CompositionRequest{
		Items: []app.CompositionInputItem{inGrams(t, owner, k.leche, "500"), inGrams(t, owner, k.te, "200")}, Yield: &rinde,
	}, k.admin); err != nil {
		t.Fatal(err)
	}
	// base = (10 + 20) / 1000 = 0.03.
	wantCost(t, owner, "preparado con más té", "ingredients", k.base, "0.03")
	wantCost(t, owner, "frappé", "products", k.frappe, "6")
	wantCost(t, owner, "paquete", "products", k.paquete, "12")
	wantCost(t, owner, "extra ligado al frappé", "modifier_options", k.extraFrappe, "6")
}

// Una receta estimada (la carga de FUDO) se confirma sin abrirla; confirmar también deja su costo al
// día, para que lo que se ve confirmado no traiga un costo de otra época.
func TestConfirmingAnEstimatedRecipeCostsIt(t *testing.T) {
	t.Parallel()
	owner := newTestStore(t)
	k := seedCostCatalog(t, owner, defaultCompanyID, "")
	if _, err := owner.Pool.Exec(context.Background(), `
		update modifier_options set composition_status = 'estimated', current_cost = 0 where id = any($1);
		`, []int64{k.extraLeche, k.extraFrappe}); err != nil {
		t.Fatal(err)
	}
	st := appRoleStore(t)
	ctx := tenantCtx(t, st, k.company, k.admin)
	svc := app.NewAdminService(st)

	if err := svc.ConfirmComposition(ctx, app.CompositionOfOption, k.extraLeche, k.admin); err != nil {
		t.Fatal(err)
	}
	wantCost(t, owner, "extra confirmado uno por uno", "modifier_options", k.extraLeche, "2")

	if _, err := svc.ConfirmRecipes(ctx, domain.RecipeKindExtra, []int64{k.extraFrappe}, k.admin); err != nil {
		t.Fatal(err)
	}
	wantCost(t, owner, "extra confirmado desde la lista", "modifier_options", k.extraFrappe, "4")
}

// El recálculo de una empresa no toca a otra: la lista que recorre es la que RLS le deja ver.
func TestCostRecomputeDoesNotCrossCompanies(t *testing.T) {
	t.Parallel()
	owner := newTestStore(t)
	k := seedCostCatalog(t, owner, defaultCompanyID, "")
	otra := makeCompany(t, owner, "otra-costo-vigente")
	b := seedCostCatalog(t, owner, otra, " B")
	// La otra empresa trae un costo viejo: si el recálculo de la primera lo corrigiera, la estaría
	// leyendo.
	if _, err := owner.Pool.Exec(context.Background(),
		`update modifier_options set current_cost = 0 where id = $1`, b.extraLeche); err != nil {
		t.Fatal(err)
	}

	st := appRoleStore(t)
	ctx := tenantCtx(t, st, k.company, k.admin)
	if err := app.NewBackofficeService(st, clock).CorrectArticleCost(ctx, k.leche, decimal.RequireFromString("0.04")); err != nil {
		t.Fatal(err)
	}
	wantCost(t, owner, "extra de la empresa que corrigió", "modifier_options", k.extraLeche, "4")
	wantCost(t, owner, "extra de la otra empresa", "modifier_options", b.extraLeche, "0")

	inTheThreeCases(t, k.company, otra, func(t *testing.T, ast *store.Store, ctx context.Context) {
		err := app.NewBackofficeService(ast, clock).CorrectArticleCost(app.WithActorUser(ctx, b.admin), k.leche, decimal.RequireFromString("1"))
		if err == nil {
			t.Fatal("corregir el insumo de otra empresa debió fallar")
		}
		wantCost(t, owner, "insumo ajeno", "ingredients", k.leche, "0.04")
		wantCost(t, owner, "extra ajeno", "modifier_options", k.extraLeche, "4")
	})
}

// Lo que ya quedó mal antes del arreglo se corrige una vez al arrancar, empresa por empresa.
func TestBootRecomputeFixesEachCompany(t *testing.T) {
	t.Parallel()
	owner := newTestStore(t)
	k := seedCostCatalog(t, owner, defaultCompanyID, "")
	otra := makeCompany(t, owner, "otra-costo-arranque")
	b := seedCostCatalog(t, owner, otra, " B")
	for _, c := range []costCatalog{k, b} {
		if _, err := owner.Pool.Exec(context.Background(), `
			update modifier_options set current_cost = 0 where id = $1;`, c.extraLeche); err != nil {
			t.Fatal(err)
		}
		if _, err := owner.Pool.Exec(context.Background(), `
			update products set current_cost = 1 where id = $1;`, c.frappe); err != nil {
			t.Fatal(err)
		}
	}

	if err := app.NewCostingService(appRoleStore(t)).RecomputeCompanies(context.Background(), []int64{k.company, otra}); err != nil {
		t.Fatal(err)
	}
	for _, c := range []costCatalog{k, b} {
		wantCost(t, owner, "extra que quedó en cero", "modifier_options", c.extraLeche, "2")
		wantCost(t, owner, "frappé con costo viejo", "products", c.frappe, "4")
	}
}

// Dos personas corrigen el costo de dos insumos del mismo producto al mismo tiempo. Cada una
// recalcula el catálogo con su cambio y sin el de la otra (aún no confirmado): la segunda en
// escribir pisaba el costo del producto con uno que no lleva el cambio de la primera.
func TestTwoConcurrentCostCorrectionsDoNotOverwriteEachOther(t *testing.T) {
	t.Parallel()
	owner := newTestStore(t)
	k := seedCostCatalog(t, owner, defaultCompanyID, "")
	st := appRoleStore(t)
	bo := app.NewBackofficeService(st, clock)
	bg := context.Background()

	// Alguien tiene tomado el frappé: las dos correcciones llegan hasta ahí con su cálculo hecho.
	hold, err := owner.Pool.Begin(bg)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := hold.Exec(bg, `select 1 from products where id = $1 for update`, k.frappe); err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i, fix := range []struct {
		ing  int64
		cost string
	}{{k.leche, "0.04"}, {k.te, "0.20"}} {
		ctx := tenantCtx(t, st, k.company, k.admin)
		wg.Go(func() { errs[i] = bo.CorrectArticleCost(ctx, fix.ing, decimal.RequireFromString(fix.cost)) })
	}
	// Si la prueba se corta antes de soltar el frappé, las dos correcciones se quedarían esperando y
	// el cierre de los pools con ellas: se suelta y se esperan antes que nada.
	t.Cleanup(func() { _ = hold.Rollback(bg); wg.Wait() })
	waitForBlocked(t, owner, 2)
	if err := hold.Rollback(bg); err != nil {
		t.Fatal(err)
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	// base = (500 × 0.04 + 100 × 0.20) / 1000 = 0.04; frappé = 200 × 0.04 = 8.
	wantCost(t, owner, "preparado con los dos insumos corregidos", "ingredients", k.base, "0.04")
	wantCost(t, owner, "frappé con los dos insumos corregidos", "products", k.frappe, "8")
}

// waitForBlocked espera a que n sesiones de la base de la prueba estén esperando un candado. Por
// pg_locks y no por el wait_event de pg_stat_activity: el dueño no ve el de las sesiones de otro rol.
func waitForBlocked(t *testing.T, st *store.Store, n int) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if blocked(t, st) >= n {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("las %d sesiones nunca llegaron a esperarse", n)
}

func blocked(t *testing.T, st *store.Store) int {
	t.Helper()
	var waiting int
	if err := st.Pool.QueryRow(context.Background(), `
		select count(distinct l.pid) from pg_locks l
		  join pg_stat_activity a on a.pid = l.pid
		 where not l.granted and a.datname = current_database()`).Scan(&waiting); err != nil {
		t.Fatal(err)
	}
	return waiting
}

// holdCompositionLock toma, desde fuera, el candado de captura de la empresa: hace de una captura de
// receta que lo tiene mientras otra cosa llega. La prueba lo suelta al terminar pase lo que pase.
func holdCompositionLock(t *testing.T, st *store.Store, company int64) pgx.Tx {
	t.Helper()
	bg := context.Background()
	tx, err := st.Pool.Begin(bg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tx.Rollback(bg) })
	if _, err := tx.Exec(bg, `select pg_advisory_xact_lock(28028, $1::int)`, company); err != nil {
		t.Fatal(err)
	}
	return tx
}

// Una captura de receta tiene el candado de la empresa y su llave foránea necesita el insumo; la
// corrección de costo de ese insumo ya lo tenía tomado y esperaba el candado: se esperaban una a la
// otra y Postgres abortaba a una con «deadlock detected» (un 500 al guardar o al corregir).
func TestCorrectingACostWhileARecipeIsSavedDoesNotDeadlock(t *testing.T) {
	t.Parallel()
	owner := newTestStore(t)
	k := seedCostCatalog(t, owner, defaultCompanyID, "")
	st := appRoleStore(t)
	bo := app.NewBackofficeService(st, clock)
	recipe := holdCompositionLock(t, owner, k.company)

	ctx := tenantCtx(t, st, k.company, k.admin)
	var wg sync.WaitGroup
	var correctErr error
	wg.Go(func() { correctErr = bo.CorrectArticleCost(ctx, k.leche, decimal.RequireFromString("0.04")) })
	t.Cleanup(func() { _ = recipe.Rollback(context.Background()); wg.Wait() })
	waitForBlocked(t, owner, 1)

	// Lo que hace la llave foránea de un renglón de receta nuevo con ese insumo.
	bg := context.Background()
	if _, err := recipe.Exec(bg, `set local lock_timeout = '5s'`); err != nil {
		t.Fatal(err)
	}
	if _, err := recipe.Exec(bg, `select 1 from ingredients where id = $1 for key share`, k.leche); err != nil {
		t.Fatalf("la captura de receta no pudo tomar el insumo que la corrección tenía: %v", err)
	}
	if err := recipe.Commit(bg); err != nil {
		t.Fatal(err)
	}
	wg.Wait()
	if correctErr != nil {
		t.Fatalf("la corrección de costo falló: %v", correctErr)
	}
	wantCost(t, owner, "extra de leche", "modifier_options", k.extraLeche, "4")
}

// Una compra que cambia el costo de un insumo recalcula con el almacén del insumo ya tomado. Si el
// candado de la empresa está ocupado no lo espera: con las filas en la mano, esperar es justo lo que
// cruzaba los candados con una captura de receta que pasa existencias al insumo (spec 037).
func TestAPurchaseDoesNotWaitForTheCompositionLockHoldingStock(t *testing.T) {
	t.Parallel()
	k := newPurchaseKit(t)
	if _, err := k.svc.CreateExpense(k.ctx, k.bultos("1", "1000")); err != nil {
		t.Fatal(err)
	}
	recipe := holdCompositionLock(t, k.st, defaultCompanyID)

	var wg sync.WaitGroup
	var buyErr error
	wg.Go(func() { _, buyErr = k.svc.CreateExpense(k.ctx, k.bultos("1", "500")) })
	t.Cleanup(func() { _ = recipe.Rollback(context.Background()); wg.Wait() })
	waitForBlockedOrDone(t, k.st, 1, &wg)

	// Lo que hace la captura que pasa la existencia propia de un producto al insumo.
	bg := context.Background()
	if _, err := recipe.Exec(bg, `set local lock_timeout = '5s'`); err != nil {
		t.Fatal(err)
	}
	if _, err := recipe.Exec(bg, `select 1 from stock_levels where ingredient_id = $1 for update`, k.azucar); err != nil {
		t.Fatalf("la captura de receta no pudo tomar el almacén que la compra tenía: %v", err)
	}
	if err := recipe.Commit(bg); err != nil {
		t.Fatal(err)
	}
	wg.Wait()
	if !errors.Is(buyErr, domain.ErrConflict) {
		t.Fatalf("la compra debe pedir volver a intentar, no esperar: %v", buyErr)
	}
	if c := k.ingredientCost(t); !c.Equal(dd("0.04")) {
		t.Errorf("la compra que no se guardó movió el costo a %s", c)
	}
}

// Cambiar la unidad de un insumo convierte sus recetas; una cantidad corregida a mano en ese cambio
// cambia lo que cuesta lo que la lleva.
func TestChangingAnIngredientUnitRecomputesWhatUsesIt(t *testing.T) {
	t.Parallel()
	owner := newTestStore(t)
	k := seedCostCatalog(t, owner, defaultCompanyID, "")
	var item int64
	if err := owner.Pool.QueryRow(context.Background(), `select ri.id from recipe_items ri
		join modifier_options mo on mo.recipe_id = ri.recipe_id where mo.id = $1`, k.extraLeche).Scan(&item); err != nil {
		t.Fatal(err)
	}
	st := appRoleStore(t)
	ctx := tenantCtx(t, st, k.company, k.admin)
	// 1 pieza = 250 g: el costo pasa a $5 por pieza y los 100 g del extra a 0.4 pieza; se corrigen a 0.6.
	if err := app.NewBackofficeService(st, clock).ChangeUnit(ctx, k.leche, app.UnitChangeRequest{
		BaseUnitID: unitID(t, owner, "pieza"), Factor: decimal.RequireFromString("0.004"),
		Overrides: map[string]decimal.Decimal{"recipe:" + strconv.FormatInt(item, 10): decimal.RequireFromString("0.6")},
	}); err != nil {
		t.Fatal(err)
	}
	wantCost(t, owner, "extra con 0.6 pieza de leche", "modifier_options", k.extraLeche, "3")
	wantCost(t, owner, "preparado (sin corrección)", "ingredients", k.base, "0.02")
}

// Un costo que no cabe en su columna (una receta capturada con una cantidad absurda) no puede tumbar
// el recálculo de todo lo demás: antes de recalcular al guardar, solo fallaban las compras; ahora
// cualquier receta de la empresa dejaría de poder guardarse.
func TestACostThatDoesNotFitDoesNotBlockTheRest(t *testing.T) {
	t.Parallel()
	owner := newTestStore(t)
	k := seedCostCatalog(t, owner, defaultCompanyID, "")
	var huge int64
	if err := owner.Pool.QueryRow(context.Background(), `
		with r as (insert into recipes default values returning id),
		     i as (insert into recipe_items (recipe_id, ingredient_id, quantity, unit_id)
		           select r.id, $1, 2000, u.id from r, units u where u.code = 'g')
		insert into products (name, category_id, price, recipe_id, cost_source, current_cost)
		select 'Receta absurda', p.category_id, 10, r.id, 'receta', 7 from r, products p where p.id = $2
		returning id`, k.te, k.frappe).Scan(&huge); err != nil {
		t.Fatal(err)
	}
	// 2000 g × $900,000 = $1,800,000,000: no cabe en numeric(12,4).
	if _, err := owner.Pool.Exec(context.Background(), `update ingredients set current_cost = 900000 where id = $1`, k.te); err != nil {
		t.Fatal(err)
	}
	st := appRoleStore(t)
	ctx := tenantCtx(t, st, k.company, k.admin)
	if err := app.NewAdminService(st).SaveComposition(ctx, app.CompositionOfOption, k.extraLeche,
		app.CompositionRequest{Items: []app.CompositionInputItem{inGrams(t, owner, k.leche, "150")}}, k.admin); err != nil {
		t.Fatalf("guardar otra receta: %v", err)
	}
	wantCost(t, owner, "la receta que sí se guardó", "modifier_options", k.extraLeche, "3")
	wantCost(t, owner, "el producto cuyo costo no cabe se queda como estaba", "products", huge, "7")
}

// waitForBlockedOrDone es waitForBlocked para algo que, ya arreglado, no espera: basta con que
// termine.
func waitForBlockedOrDone(t *testing.T, st *store.Store, n int, wg *sync.WaitGroup) {
	t.Helper()
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case <-done:
			return
		default:
		}
		if blocked(t, st) >= n {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("ni terminó ni llegó a esperar")
}
