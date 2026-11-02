//go:build integration

package integration

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"testing"
	"time"
	"uuid"

	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store/db"
)

// mergeKit arma dos marcas de leche (en gramos, para usar makeRecipe) y un producto que vende la B.
type mergeKit struct {
	st      *store.Store
	svc     *app.BackofficeService
	ctx     context.Context
	admin   int64
	a, b    int64 // a es el destino
	product int64
}

func newMergeKit(t *testing.T) *mergeKit {
	t.Helper()
	st := newTestStore(t)
	k := &mergeKit{st: st}
	k.admin = makeUser(t, st, "unir_admin", "admin")
	k.a = ingredientIn(t, st, defaultCompanyID, "Leche unir", "g")
	k.b = ingredientIn(t, st, defaultCompanyID, "Leche unir marca B", "g")
	adjust(t, st, "ingrediente", k.a, "500")
	adjust(t, st, "ingrediente", k.b, "300")
	k.product = makeProduct(t, st, "Frappé unir", qty("60"), false)
	sinPreparacion(t, st, k.product)
	recipe := makeRecipe(t, st, map[int64]string{k.b: "100"})
	if _, err := st.Pool.Exec(context.Background(), `update products set recipe_id = $2, cost_source = 'receta' where id = $1`, k.product, recipe); err != nil {
		t.Fatal(err)
	}
	svc, ctx := asTenant(t, defaultCompanyID)
	k.svc, k.ctx = svc, app.WithActorUser(ctx, k.admin)
	return k
}

func (k *mergeKit) req(sources ...int64) app.MergeRequest {
	r := app.MergeRequest{}
	for _, s := range sources {
		r.Sources = append(r.Sources, app.MergeSourceInput{ID: s})
	}
	return r
}

// merge pide la vista previa y confirma con su huella, como la pantalla.
func (k *mergeKit) merge(t *testing.T, target int64, r app.MergeRequest) app.MergePreview {
	t.Helper()
	prev, err := k.svc.MergeIngredients(k.ctx, target, r, true)
	if err != nil {
		t.Fatalf("vista previa: %v", err)
	}
	r.Fingerprint = prev.Fingerprint
	got, err := k.svc.MergeIngredients(k.ctx, target, r, false)
	if err != nil {
		t.Fatalf("unir: %v", err)
	}
	return got
}

func recipeRowsOf(t *testing.T, st *store.Store, ing int64) int {
	t.Helper()
	var n int
	if err := st.Pool.QueryRow(context.Background(), `select count(*) from recipe_items where ingredient_id = $1`, ing).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func isActive(t *testing.T, st *store.Store, ing int64) bool {
	t.Helper()
	var a bool
	if err := st.Pool.QueryRow(context.Background(), `select is_active from ingredients where id = $1`, ing).Scan(&a); err != nil {
		t.Fatal(err)
	}
	return a
}

func TestMergeMovesEverythingAndLeavesNoRecipeOnTheSource(t *testing.T) {
	t.Parallel()
	k := newMergeKit(t)
	ctx := context.Background()
	c := ingredientIn(t, k.st, defaultCompanyID, "Leche unir marca C", "g")
	adjust(t, k.st, "ingrediente", c, "-20")
	// Una receta que ya lleva el destino y el origen, la de un producto inactivo y la de un preparado.
	both := makeProduct(t, k.st, "Latte unir", qty("50"), false)
	inactive := makeProduct(t, k.st, "Viejo unir", qty("40"), false)
	prep := ingredientIn(t, k.st, defaultCompanyID, "Base unir", "g")
	for prod, items := range map[int64]map[int64]string{both: {k.a: "100", k.b: "100"}, inactive: {k.b: "50"}} {
		if _, err := k.st.Pool.Exec(ctx, `update products set recipe_id = $2 where id = $1`, prod, makeRecipe(t, k.st, items)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := k.st.Pool.Exec(ctx, `update products set is_active = false where id = $1`, inactive); err != nil {
		t.Fatal(err)
	}
	if _, err := k.st.Pool.Exec(ctx, `update ingredients set is_prep = true, recipe_id = $2, yield_qty = 1000 where id = $1`,
		prep, makeRecipe(t, k.st, map[int64]string{c: "10"})); err != nil {
		t.Fatal(err)
	}
	if _, err := k.st.Pool.Exec(ctx, `update ingredients set min_stock = 400, current_cost = 0.03 where id = $1`, k.b); err != nil {
		t.Fatal(err)
	}
	if _, err := k.st.Pool.Exec(ctx, `update ingredients set current_cost = 0.02 where id = $1`, k.a); err != nil {
		t.Fatal(err)
	}

	prev, err := k.svc.MergeIngredients(k.ctx, k.a, k.req(k.b, c), true)
	if err != nil {
		t.Fatal(err)
	}
	t.Run("la vista previa dice cuánto queda y qué recetas pasan, y no cambia nada", func(t *testing.T) {
		after := map[int64]decimal.Decimal{}
		for _, s := range prev.Stock {
			after[s.IngredientID] = s.After
		}
		if !after[k.a].Equal(qty("780")) || !after[k.b].IsZero() || !after[c].IsZero() {
			t.Fatalf("existencias después = %v; quiere A 780, B 0, C 0", after)
		}
		if len(prev.Recipes) != 4 {
			t.Fatalf("recetas = %+v; quiere 4 (frappé, latte, inactivo, preparado)", prev.Recipes)
		}
		sums := 0
		for _, r := range prev.Recipes {
			if r.Merged {
				sums++
			}
		}
		if sums != 1 {
			t.Fatalf("solo el latte junta dos renglones: %+v", prev.Recipes)
		}
		if !isActive(t, k.st, k.b) || recipeRowsOf(t, k.st, k.b) != 3 {
			t.Fatal("la vista previa escribió: B ya no está activa o sus recetas se movieron")
		}
		var n int
		if err := k.st.Pool.QueryRow(ctx, `select count(*) from ingredient_replacements where kind = 'merge'`).Scan(&n); err != nil || n != 0 {
			t.Fatalf("la vista previa dejó %d uniones %v", n, err)
		}
	})

	r := k.req(k.b, c)
	r.Fingerprint = prev.Fingerprint
	if _, err := k.svc.MergeIngredients(k.ctx, k.a, r, false); err != nil {
		t.Fatal(err)
	}

	t.Run("los orígenes quedan archivados, en cero y sin recetas", func(t *testing.T) {
		for _, src := range []int64{k.b, c} {
			if isActive(t, k.st, src) || recipeRowsOf(t, k.st, src) != 0 {
				t.Fatalf("el origen %d sigue activo o con recetas", src)
			}
			if _, level := sumOf(t, k.st, src); !level.IsZero() {
				t.Fatalf("el origen %d quedó con %s", src, level)
			}
		}
	})
	t.Run("el destino tiene la suma, igual a sus movimientos, el mínimo del origen y su costo", func(t *testing.T) {
		movs, level := sumOf(t, k.st, k.a)
		if !level.Equal(qty("780")) || !movs.Equal(level) {
			t.Fatalf("A = %s (movimientos %s); quiere 780", level, movs)
		}
		var minStock, cost decimal.Decimal
		if err := k.st.Pool.QueryRow(ctx, `select min_stock, current_cost from ingredients where id = $1`, k.a).Scan(&minStock, &cost); err != nil {
			t.Fatal(err)
		}
		if !minStock.Equal(qty("400")) || !cost.Equal(qty("0.02")) {
			t.Fatalf("mínimo %s costo %s; quiere 400 y 0.02", minStock, cost)
		}
	})
	t.Run("el latte lleva un solo renglón con la suma", func(t *testing.T) {
		var n int
		var q decimal.Decimal
		if err := k.st.Pool.QueryRow(ctx, `select count(*), sum(ri.quantity * u.to_base) from recipe_items ri join units u on u.id = ri.unit_id
			join products p on p.recipe_id = ri.recipe_id where p.id = $1`, both).Scan(&n, &q); err != nil {
			t.Fatal(err)
		}
		if n != 1 || !q.Equal(qty("200")) {
			t.Fatalf("latte = %d renglones con %s; quiere 1 con 200", n, q)
		}
	})
	t.Run("la bitácora lo dice en el destino y en cada origen", func(t *testing.T) {
		var merged, into int
		if err := k.st.Pool.QueryRow(ctx, `select count(*) filter (where action = 'ingredient.merged' and entity_id = $1),
			count(*) filter (where action = 'ingredient.merged_into' and entity_id in ($2, $3))
			from audit_events where entity_type = 'ingredient'`, k.a, k.b, c).Scan(&merged, &into); err != nil {
			t.Fatal(err)
		}
		if merged != 1 || into != 2 {
			t.Fatalf("eventos merged=%d merged_into=%d; quiere 1 y 2", merged, into)
		}
	})
	t.Run("la bitácora del destino trae también la de sus orígenes", func(t *testing.T) {
		page, err := k.svc.ArticleAudit(k.ctx, k.a, nil)
		if err != nil {
			t.Fatal(err)
		}
		into := 0
		for _, e := range page.Items {
			if e.Action == "ingredient.merged_into" {
				into++
			}
		}
		if into != 2 {
			t.Fatalf("la bitácora del destino trae %d eventos de sus orígenes; quiere 2", into)
		}
	})
	t.Run("la ficha del origen dice a cuál se unió", func(t *testing.T) {
		got, err := k.svc.GetArticle(k.ctx, k.b)
		if err != nil || got.MergedInto == nil || got.MergedInto.ID != k.a {
			t.Fatalf("%+v %v", got.MergedInto, err)
		}
		dst, err := k.svc.GetArticle(k.ctx, k.a)
		if err != nil || dst.MergedSources != 2 {
			t.Fatalf("destino con %d orígenes %v", dst.MergedSources, err)
		}
	})
	t.Run("una venta después descuenta del destino", func(t *testing.T) {
		cashier := makeUser(t, k.st, "unir_cajero", "cajero")
		abrirCajaPrincipal(t, k.st, cashier)
		orders := app.NewOrdersService(k.st, clock)
		if _, err := orders.Create(ctx, app.CreateOrderCmd{ClientUUID: uuid.New(), ServiceType: "mostrador", OpenedBy: cashier,
			Lines: []domain.OrderLineInput{{ProductID: k.product, Qty: qty("1")}}}); err != nil {
			t.Fatal(err)
		}
		if got := onHandIngredient(t, k.st, k.a); !got.Equal(qty("680")) {
			t.Fatalf("A = %s; quiere 680", got)
		}
		if got := onHandIngredient(t, k.st, k.b); !got.IsZero() {
			t.Fatalf("B se movió: %s", got)
		}
	})
}

func TestMergeBlocks(t *testing.T) {
	t.Parallel()
	k := newMergeKit(t)
	ctx := context.Background()
	blocked := func(t *testing.T, err error, sentinel error) {
		t.Helper()
		if !errors.Is(err, sentinel) {
			t.Fatalf("quería %v; llegó %v", sentinel, err)
		}
	}
	t.Run("la solicitud mal armada es 400", func(t *testing.T) {
		_, err := k.svc.MergeIngredients(k.ctx, k.a, k.req(), true)
		blocked(t, err, domain.ErrValidation)
		_, err = k.svc.MergeIngredients(k.ctx, k.a, k.req(k.b, k.b), true)
		blocked(t, err, domain.ErrValidation)
		_, err = k.svc.MergeIngredients(k.ctx, k.a, k.req(k.a), true)
		blocked(t, err, domain.ErrValidation)
	})
	t.Run("un origen medido distinto sin equivalencia es 400 y con ella se puede", func(t *testing.T) {
		pz := ingredientIn(t, k.st, defaultCompanyID, "Tapa unir", "pieza")
		_, err := k.svc.MergeIngredients(k.ctx, k.a, k.req(pz), true)
		blocked(t, err, domain.ErrValidation)
		f := qty("4")
		if _, err := k.svc.MergeIngredients(k.ctx, k.a, app.MergeRequest{Sources: []app.MergeSourceInput{{ID: pz, Factor: &f}}}, true); err != nil {
			t.Fatalf("con equivalencia: %v", err)
		}
	})
	t.Run("un insumo de otra empresa no existe para ésta (RLS, gatobobah_app)", func(t *testing.T) {
		other := ingredientIn(t, k.st, makeCompany(t, k.st, "unir-otra"), "Leche otra empresa", "g")
		_, err := k.svc.MergeIngredients(k.ctx, k.a, k.req(other), true)
		blocked(t, err, domain.ErrNotFound)
	})
	t.Run("un conteo abierto que ya capturó el origen", func(t *testing.T) {
		var count int64
		if err := k.st.Pool.QueryRow(ctx, `insert into stock_counts (company_id, branch_id, status, opened_by)
			values ($1, (select id from branches where company_id = $1 limit 1), 'open', $2) returning id`, defaultCompanyID, k.admin).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if _, err := k.st.Pool.Exec(ctx, `insert into stock_count_lines (company_id, count_id, ingredient_id, counted_qty, system_qty, captured_by)
			values ($1, $2, $3, 1, 1, $4)`, defaultCompanyID, count, k.b, k.admin); err != nil {
			t.Fatal(err)
		}
		_, err := k.svc.MergeIngredients(k.ctx, k.a, k.req(k.b), true)
		blocked(t, err, domain.ErrMergeBlocked)
		if _, err := k.st.Pool.Exec(ctx, `delete from stock_count_lines where count_id = $1`, count); err != nil {
			t.Fatal(err)
		}
		if _, err := k.st.Pool.Exec(ctx, `delete from stock_counts where id = $1`, count); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("un preparado no se une", func(t *testing.T) {
		prep := ingredientIn(t, k.st, defaultCompanyID, "Jarabe unir", "g")
		if _, err := k.st.Pool.Exec(ctx, `update ingredients set is_prep = true, recipe_id = $2, yield_qty = 1000 where id = $1`,
			prep, makeRecipe(t, k.st, map[int64]string{k.a: "10"})); err != nil {
			t.Fatal(err)
		}
		_, err := k.svc.MergeIngredients(k.ctx, k.a, k.req(prep), true)
		blocked(t, err, domain.ErrMergeBlocked)
	})
	t.Run("un origen archivado no se une", func(t *testing.T) {
		old := ingredientIn(t, k.st, defaultCompanyID, "Leche archivada unir", "g")
		if _, err := k.st.Pool.Exec(ctx, `update ingredients set is_active = false where id = $1`, old); err != nil {
			t.Fatal(err)
		}
		_, err := k.svc.MergeIngredients(k.ctx, k.a, k.req(old), true)
		blocked(t, err, domain.ErrMergeBlocked)
	})
	t.Run("un origen unido no se reactiva y un destino con unidos no cambia de unidad", func(t *testing.T) {
		k.merge(t, k.a, k.req(k.b))
		err := k.svc.ReactivateArticle(k.ctx, k.b)
		blocked(t, err, domain.ErrIngredientMerged)
		_, err = k.svc.PreviewUnitChange(k.ctx, k.a, app.UnitChangeRequest{BaseUnitID: unitID(t, k.st, "pieza"), Factor: qty("0.01")})
		blocked(t, err, domain.ErrMergeBlocked)
	})
	t.Run("lo que la vista previa mostró cambió: la confirmación se niega", func(t *testing.T) {
		d := ingredientIn(t, k.st, defaultCompanyID, "Leche unir marca D", "g")
		adjust(t, k.st, "ingrediente", d, "10")
		prev, err := k.svc.MergeIngredients(k.ctx, k.a, k.req(d), true)
		if err != nil {
			t.Fatal(err)
		}
		adjust(t, k.st, "ingrediente", d, "5")
		r := k.req(d)
		r.Fingerprint = prev.Fingerprint
		_, err = k.svc.MergeIngredients(k.ctx, k.a, r, false)
		blocked(t, err, domain.ErrMergeStale)
		if !isActive(t, k.st, d) {
			t.Fatal("se unió con una vista previa vieja")
		}
	})
}

// LA CARRERA CON UNA VENTA (research R2). Dos órdenes de eventos: la venta ya tiene tomado el origen
// (la unión responde «vuelve a intentar»), o la unión lo tomó primero y la venta llega después (la
// venta truena al confirmar y su reintento entra al destino). En los dos, la existencia es la suma
// de los movimientos.
func TestASaleRacingTheMergeNeverLosesStock(t *testing.T) {
	t.Parallel()
	k := newMergeKit(t)
	ctx := context.Background()
	move := func(ing int64, q string) db.InsertStockMovementParams {
		return db.InsertStockMovementParams{ItemType: "ingrediente", IngredientID: &ing, MovementType: "venta", Quantity: qty(q)}
	}

	t.Run("la venta llegó primero: la unión no espera y pide volver a intentar", func(t *testing.T) {
		tx, err := k.st.Pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := tx.Exec(ctx, `insert into stock_movements (company_id, item_type, ingredient_id, movement_type, quantity)
			values ($1, 'ingrediente', $2, 'venta', -10)`, defaultCompanyID, k.b); err != nil {
			t.Fatal(err)
		}
		_, mergeErr := k.svc.MergeIngredients(k.ctx, k.a, k.req(k.b), true)
		_ = tx.Rollback(ctx)
		if !errors.Is(mergeErr, domain.ErrConflict) {
			t.Fatalf("la unión con una venta en curso debió responder 409; llegó %v", mergeErr)
		}
	})

	t.Run("la unión llegó primero: la venta que esperaba truena al confirmar y el reintento entra al destino", func(t *testing.T) {
		tx, err := k.st.Pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := tx.Exec(ctx, `select 1 from ingredients where id = $1 for update`, k.b); err != nil {
			t.Fatal(err)
		}
		if _, err := tx.Exec(ctx, `insert into ingredient_replacements (company_id, old_ingredient_id, new_ingredient_id, factor, user_id, kind)
			values ($1, $2, $3, 1, $4, 'merge')`, defaultCompanyID, k.b, k.a, k.admin); err != nil {
			t.Fatal(err)
		}
		saleErr := make(chan error, 1)
		go func() {
			saleErr <- k.st.WithTx(ctx, func(q *db.Queries) error {
				return q.InsertStockMovement(ctx, move(k.b, "-10"))
			})
		}()
		time.Sleep(300 * time.Millisecond) // la venta ya empezó su insert y espera la llave foránea
		if err := tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		if err := <-saleErr; !errors.Is(err, domain.ErrIngredientMerged) {
			t.Fatalf("la venta debió tronar como insumo unido (409); llegó %v", err)
		}
		if err := k.st.WithTx(ctx, func(q *db.Queries) error { return q.InsertStockMovement(ctx, move(k.b, "-10")) }); err != nil {
			t.Fatalf("el reintento: %v", err)
		}
		for _, ing := range []int64{k.a, k.b} {
			movs, level := sumOf(t, k.st, ing)
			if !movs.Equal(level) {
				t.Fatalf("insumo %d: existencia %s, movimientos %s", ing, level, movs)
			}
		}
		if got := onHandIngredient(t, k.st, k.a); !got.Equal(qty("490")) {
			t.Fatalf("A = %s; quiere 490 (el reintento restó del destino)", got)
		}
	})
}

// Cancelar un pedido vendido ANTES de unir repone donde ahora está la mercancía: el destino.
func TestALateMovementOnAMergedSourceGoesToTheTarget(t *testing.T) {
	t.Parallel()
	k := newMergeKit(t)
	ctx := context.Background()
	cashier := makeUser(t, k.st, "unir_cajero_c", "cajero")
	abrirCajaPrincipal(t, k.st, cashier)
	orders := app.NewOrdersService(k.st, clock)
	order, err := orders.Create(ctx, app.CreateOrderCmd{ClientUUID: uuid.New(), ServiceType: "mostrador", OpenedBy: cashier,
		Lines: []domain.OrderLineInput{{ProductID: k.product, Qty: qty("1")}}})
	if err != nil {
		t.Fatal(err)
	}
	k.merge(t, k.a, k.req(k.b)) // B tenía 200 tras la venta
	if err := orders.Cancel(ctx, order.ID, cashier, "se arrepintió"); err != nil {
		t.Fatal(err)
	}
	if got := onHandIngredient(t, k.st, k.b); !got.IsZero() {
		t.Fatalf("lo repuesto entró al origen archivado: B = %s", got)
	}
	if got := onHandIngredient(t, k.st, k.a); !got.Equal(qty("800")) {
		t.Fatalf("A = %s; quiere 500 + 200 del traspaso + 100 repuesto = 800", got)
	}
}

func TestMergeHTTPNeedsConfigure(t *testing.T) {
	t.Parallel()
	r, st, token := newStockAPI(t)
	a := strconv.FormatInt(ingredientIn(t, st, defaultCompanyID, "Leche http unir", "g"), 10)
	b := ingredientIn(t, st, defaultCompanyID, "Leche http unir B", "g")
	body := func(id int64) []byte { return []byte(`{"sources":[{"id":` + strconv.FormatInt(id, 10) + `}]}`) }
	cajero := token("unir_http_cajero", "cajero")
	if w := do(t, r, http.MethodPost, "/api/v1/stock/articles/"+a+"/merge?preview=1", cajero, body(b), "application/json"); w.Code != http.StatusForbidden {
		t.Fatalf("cajero = %d; quiere 403", w.Code)
	}
	admin := token("unir_http_admin", "admin")
	if w := do(t, r, http.MethodPost, "/api/v1/stock/articles/"+a+"/merge?preview=2", admin, body(b), "application/json"); w.Code != http.StatusBadRequest {
		t.Fatalf("preview=2 = %d; quiere 400", w.Code)
	}
	w := do(t, r, http.MethodPost, "/api/v1/stock/articles/"+a+"/merge?preview=1", admin, body(b), "application/json")
	if w.Code != http.StatusOK {
		t.Fatalf("vista previa = %d %s", w.Code, w.Body.String())
	}
	if w := do(t, r, http.MethodPost, "/api/v1/stock/articles/"+a+"/merge", admin, body(b), "application/json"); w.Code != http.StatusConflict {
		t.Fatalf("confirmar sin la huella de la vista previa = %d; quiere 409", w.Code)
	}
}

// Con dos sucursales, el traspaso es sucursal por sucursal: lo de una nunca pasa a la otra.
func TestMergeMovesStockBranchByBranch(t *testing.T) {
	t.Parallel()
	k := newMergeKit(t)
	ctx := context.Background()
	var main, second int64
	if err := k.st.Pool.QueryRow(ctx, `select id from branches where company_id = $1 order by id limit 1`, defaultCompanyID).Scan(&main); err != nil {
		t.Fatal(err)
	}
	if err := k.st.Pool.QueryRow(ctx, `insert into branches (company_id, branch_number, code, name) values ($1, 2, 'S2', 'Sucursal 2') returning id`,
		defaultCompanyID).Scan(&second); err != nil {
		t.Fatal(err)
	}
	if _, err := k.st.Pool.Exec(ctx, `insert into stock_movements (company_id, item_type, ingredient_id, movement_type, quantity, branch_id)
		values ($1, 'ingrediente', $2, 'ajuste', 70, $3)`, defaultCompanyID, k.b, second); err != nil {
		t.Fatal(err)
	}
	k.merge(t, k.a, k.req(k.b))
	level := func(ing, branch int64) decimal.Decimal {
		t.Helper()
		var q decimal.Decimal
		if err := k.st.Pool.QueryRow(ctx, `select coalesce(sum(on_hand), 0) from stock_levels where ingredient_id = $1 and branch_id = $2`, ing, branch).Scan(&q); err != nil {
			t.Fatal(err)
		}
		return q
	}
	if got := level(k.a, main); !got.Equal(qty("800")) {
		t.Fatalf("A en la principal = %s; quiere 500 + 300", got)
	}
	if got := level(k.a, second); !got.Equal(qty("70")) {
		t.Fatalf("A en la segunda = %s; quiere los 70 de B en esa sucursal", got)
	}
	if !level(k.b, main).IsZero() || !level(k.b, second).IsZero() {
		t.Fatal("B quedó con existencia en alguna sucursal")
	}
}

// El historial de costos del destino trae las compras de sus orígenes, con el nombre del origen y el
// costo en la unidad del destino (FR-006), y nada de eso se ve desde otra empresa (los tres casos).
func TestTheTargetCostHistoryShowsItsSources(t *testing.T) {
	t.Parallel()
	k := newMergeKit(t)
	ctx := context.Background()
	pz := ingredientIn(t, k.st, defaultCompanyID, "Leche en botella unir", "pieza")
	_, item := purchaseIn(t, k.st, defaultCompanyID, k.admin, pz)
	if _, err := k.st.Pool.Exec(ctx, `update expense_items set qty_received = quantity where id = $1`, item); err != nil {
		t.Fatal(err)
	}
	if _, err := k.st.Pool.Exec(ctx, `insert into stock_movements (company_id, item_type, ingredient_id, movement_type, quantity, unit_cost, expense_item_id)
		values ($1, 'ingrediente', $2, 'compra', 2, 30, $3)`, defaultCompanyID, pz, item); err != nil {
		t.Fatal(err)
	}
	f := qty("1000") // una botella = 1000 g del destino
	r := app.MergeRequest{Sources: []app.MergeSourceInput{{ID: pz, Factor: &f}}}
	k.merge(t, k.a, r)
	page, err := k.svc.ArticleCostHistory(k.ctx, k.a, nil)
	if err != nil {
		t.Fatal(err)
	}
	var found *app.CostHistoryItem
	for i := range page.Items {
		if page.Items[i].Kind == "purchase" {
			found = &page.Items[i]
		}
	}
	if found == nil || found.FromName == nil || *found.FromName != "Leche en botella unir" || !found.UnitCost.Equal(qty("0.03")) {
		t.Fatalf("historial del destino = %+v; quiere la compra de la botella, de $30 la pieza = $0.03 el gramo", page.Items)
	}
	owner := int64(defaultCompanyID)
	other := makeCompany(t, k.st, "unir-historial-otra")
	inTheThreeCases(t, owner, other, func(t *testing.T, st *store.Store, ctx context.Context) {
		svc := app.NewBackofficeService(st, nil)
		page, err := svc.ArticleCostHistory(ctx, k.a, nil)
		if err == nil && len(page.Items) > 0 {
			t.Fatalf("se ve el historial de otra empresa: %+v", page.Items)
		}
	})
}
