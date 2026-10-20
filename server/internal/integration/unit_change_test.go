//go:build integration

package integration

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
)

// CAMBIAR EN QUÉ SE MIDE UN ARTÍCULO (spec 042, US5, FR-009).

func sumOf(t *testing.T, st *store.Store, ing int64) (movs, level decimal.Decimal) {
	t.Helper()
	var m, l string
	if err := st.Pool.QueryRow(context.Background(), `
		select coalesce((select sum(quantity) from stock_movements where ingredient_id = $1), 0)::text,
		       coalesce((select sum(on_hand) from stock_levels where ingredient_id = $1), 0)::text`, ing).Scan(&m, &l); err != nil {
		t.Fatal(err)
	}
	return qty(m), qty(l)
}

// Solo con ajustes, el cambio es directo: el mismo artículo, con su existencia, sus ajustes, sus
// recetas, sus empaques, su mínimo y su costo convertidos, y los originales en la bitácora.
func TestChangingTheUnitDirectly(t *testing.T) {
	t.Parallel()
	k := newPurchaseKit(t)
	ctx := app.WithActorUser(k.ctx, k.user)
	svc := k.svc
	ctxOwner := context.Background()
	// Un artículo en piezas que se pesa: 1 pieza = 150 g, para que la conversión no sea exacta.
	limon := ingredientIn(t, k.st, defaultCompanyID, "Limón unidad", "pieza")
	for range 30 {
		stockAdjust(t, k.st, defaultCompanyID, limon, "1")
	}
	if _, err := k.st.Pool.Exec(ctxOwner, `update ingredients set min_stock = 10, current_cost = 3 where id = $1`, limon); err != nil {
		t.Fatal(err)
	}
	if _, err := k.st.Pool.Exec(ctxOwner, `insert into ingredient_presentations (company_id, ingredient_id, name, qty_in_base) values ($1, $2, 'Malla', 20)`, defaultCompanyID, limon); err != nil {
		t.Fatal(err)
	}
	p := makeProduct(t, k.st, "Limonada unidad", qty("30"), false)
	var recipe int64
	if err := k.st.Pool.QueryRow(ctxOwner, `insert into recipes default values returning id`).Scan(&recipe); err != nil {
		t.Fatal(err)
	}
	if _, err := k.st.Pool.Exec(ctxOwner, `insert into recipe_items (recipe_id, ingredient_id, quantity, unit_id)
		select $1, $2, 2, id from units where code = 'pieza'`, recipe, limon); err != nil {
		t.Fatal(err)
	}
	if _, err := k.st.Pool.Exec(ctxOwner, `update products set recipe_id = $2 where id = $1`, p, recipe); err != nil {
		t.Fatal(err)
	}
	g := gramos(t, k.st)
	factor := qty("150")

	prev, err := svc.PreviewUnitChange(ctx, limon, app.UnitChangeRequest{BaseUnitID: g, Factor: factor})
	if err != nil {
		t.Fatal(err)
	}
	if prev.Path != domain.UnitPathDirect {
		t.Fatalf("camino = %s; con solo ajustes debe ser directo", prev.Path)
	}
	if err := svc.ChangeUnit(ctx, limon, app.UnitChangeRequest{BaseUnitID: g, Factor: factor}); err != nil {
		t.Fatal(err)
	}

	t.Run("la existencia queda igual a la suma de sus movimientos, aunque la conversión no sea exacta", func(t *testing.T) {
		movs, level := sumOf(t, k.st, limon)
		if !movs.Equal(level) || !level.Equal(qty("4500")) {
			t.Fatalf("movimientos %s, existencia %s; quiere las dos en 4500 g", movs, level)
		}
	})
	t.Run("receta, empaque, mínimo y costo convertidos", func(t *testing.T) {
		var rq, pq, min, cost string
		var unit int16
		if err := k.st.Pool.QueryRow(ctxOwner, `select quantity::text, unit_id from recipe_items where recipe_id = $1`, recipe).Scan(&rq, &unit); err != nil {
			t.Fatal(err)
		}
		if err := k.st.Pool.QueryRow(ctxOwner, `select qty_in_base::text from ingredient_presentations where ingredient_id = $1`, limon).Scan(&pq); err != nil {
			t.Fatal(err)
		}
		if err := k.st.Pool.QueryRow(ctxOwner, `select min_stock::text, current_cost::text from ingredients where id = $1`, limon).Scan(&min, &cost); err != nil {
			t.Fatal(err)
		}
		if !qty(rq).Equal(qty("300")) || unit != g || !qty(pq).Equal(qty("3000")) || !qty(min).Equal(qty("1500")) || !qty(cost).Equal(qty("0.02")) {
			t.Fatalf("receta %s (unidad %d), empaque %s, mínimo %s, costo %s", rq, unit, pq, min, cost)
		}
	})
	t.Run("la bitácora guarda los originales: un evento principal y sus detalles con el mismo lote", func(t *testing.T) {
		if n := auditCount(t, k.st, "ingredient.unit_changed", limon); n != 1 {
			t.Fatalf("principales = %d", n)
		}
		var details int
		var batches int
		if err := k.st.Pool.QueryRow(ctxOwner, `
			select count(*), count(distinct after->>'batchId') from audit_events
			 where action = 'ingredient.unit_changed.detail' and entity_id = $1`, limon).Scan(&details, &batches); err != nil {
			t.Fatal(err)
		}
		if details < 3 || batches != 1 {
			t.Fatalf("detalles = %d en %d lotes; quiere ajustes, recetas y empaques en un solo lote", details, batches)
		}
	})
}

// Cada clase con piezas: masa↔pieza y volumen↔pieza. Entre masa y volumen no se ofrece.
func TestEachUnitClassChange(t *testing.T) {
	t.Parallel()
	k := newPurchaseKit(t)
	ctx := app.WithActorUser(k.ctx, k.user)
	for _, c := range []struct {
		from, to string
		factor   string
		ok       bool
	}{
		{"g", "pieza", "0.004", true},
		{"pieza", "g", "250", true},
		{"ml", "pieza", "0.001", true},
		{"pieza", "ml", "1000", true},
		{"g", "ml", "1", false},
	} {
		t.Run(c.from+"→"+c.to, func(t *testing.T) {
			ing := ingredientIn(t, k.st, defaultCompanyID, "Clase "+c.from+" a "+c.to, c.from)
			stockAdjust(t, k.st, defaultCompanyID, ing, "1000")
			err := k.svc.ChangeUnit(ctx, ing, app.UnitChangeRequest{BaseUnitID: unitID(t, k.st, c.to), Factor: qty(c.factor)})
			if c.ok && err != nil {
				t.Fatalf("= %v", err)
			}
			if !c.ok && !errors.Is(err, domain.ErrValidation) {
				t.Fatalf("= %v; quiere ErrValidation", err)
			}
		})
	}
}

// Un empaque que no cuadra con la unidad nueva se archiva, nunca se borra, con su original en la
// bitácora; y no se puede reactivar mientras siga sin cuadrar.
func TestAPresentationThatNoLongerFitsIsArchived(t *testing.T) {
	t.Parallel()
	k := newPurchaseKit(t)
	ctx := app.WithActorUser(k.ctx, k.user)
	harina := ingredientIn(t, k.st, defaultCompanyID, "Harina a piezas", "g")
	var half int64
	if err := k.st.Pool.QueryRow(context.Background(), `insert into ingredient_presentations (company_id, ingredient_id, name, qty_in_base)
		values ($1, $2, 'Bolsita', 2500) returning id`, defaultCompanyID, harina).Scan(&half); err != nil {
		t.Fatal(err)
	}
	// 1 pieza = 5,000 g: la bolsita de 2,500 g sería media pieza.
	if err := k.svc.ChangeUnit(ctx, harina, app.UnitChangeRequest{BaseUnitID: unitID(t, k.st, "pieza"), Factor: qty("0.0002")}); err != nil {
		t.Fatal(err)
	}
	var active bool
	var n int
	if err := k.st.Pool.QueryRow(context.Background(), `select active, (select count(*) from ingredient_presentations where id = $1) from ingredient_presentations where id = $1`, half).Scan(&active, &n); err != nil {
		t.Fatal(err)
	}
	if active || n != 1 {
		t.Fatalf("activo %v, filas %d; quiere archivado y conservado", active, n)
	}
	var orig string
	if err := k.st.Pool.QueryRow(context.Background(), `
		select coalesce(before::text, '') from audit_events
		 where action = 'ingredient.unit_changed.detail' and entity_id = $1 and before::text like '%Bolsita%'`, harina).Scan(&orig); err != nil {
		t.Fatalf("la bitácora no guarda el empaque original: %v", err)
	}
	on := true
	if _, err := k.svc.UpdatePresentation(ctx, half, app.PresentationInput{Name: "Bolsita", Qty: qty("0.5"), UnitID: unitID(t, k.st, "pieza"), Active: &on}); err == nil {
		t.Fatal("se reactivó un empaque de media pieza")
	}
}

// Los bloqueos dicen qué falta cerrar, y una venta en curso no espera (E6, E9).
func TestUnitChangeBlocks(t *testing.T) {
	t.Parallel()
	k := newPurchaseKit(t)
	ctx := app.WithActorUser(k.ctx, k.user)
	g := gramos(t, k.st)
	pz := unitID(t, k.st, "pieza")

	t.Run("una compra por recibir lo detiene y dice cuál", func(t *testing.T) {
		fresa := ingredientIn(t, k.st, defaultCompanyID, "Fresa unidad", "g")
		q := qty("2")
		if _, err := k.svc.CreateExpense(ctx, app.ExpenseInput{CategoryID: k.category, Amount: qty("80"), Status: domain.ExpensePendiente, UserID: k.user,
			Items: []app.ExpenseItemInput{{ItemType: "ingrediente", IngredientID: &fresa, Description: "Fresa", Quantity: q, UnitID: &g, Amount: qty("80")}}}); err != nil {
			t.Fatal(err)
		}
		err := k.svc.ChangeUnit(ctx, fresa, app.UnitChangeRequest{BaseUnitID: pz, Factor: qty("0.01")})
		var pending domain.UnitChangePending
		if !errors.As(err, &pending) || pending.Receipts != 1 {
			t.Fatalf("= %v; quiere UnitChangePending con 1 compra por recibir", err)
		}
	})
	t.Run("una merma por aprobar lo detiene", func(t *testing.T) {
		kiwi := ingredientIn(t, k.st, defaultCompanyID, "Kiwi unidad", "pieza")
		reason := reasonNamed(t, k.st, defaultCompanyID, "Caducó")
		if _, err := k.st.Pool.Exec(context.Background(), `insert into waste_records (company_id, branch_id, item_type, ingredient_id, quantity, entered_qty, reason_id, is_staff_meal, status, recorded_by)
			select $1, b.id, 'ingrediente', $2, 3, 3, $3, false, 'pending', $4 from branches b where b.company_id = $1 limit 1`, defaultCompanyID, kiwi, reason, k.user); err != nil {
			t.Fatal(err)
		}
		err := k.svc.ChangeUnit(ctx, kiwi, app.UnitChangeRequest{BaseUnitID: g, Factor: qty("90")})
		var pending domain.UnitChangePending
		if !errors.As(err, &pending) || pending.Waste != 1 {
			t.Fatalf("= %v; quiere UnitChangePending con 1 merma", err)
		}
	})
	t.Run("un conteo abierto que lo capturó lo detiene", func(t *testing.T) {
		pera := ingredientIn(t, k.st, defaultCompanyID, "Pera unidad", "pieza")
		c, err := k.svc.OpenCount(ctx, k.user)
		if err != nil {
			t.Fatal(err)
		}
		if err := k.svc.CaptureCount(ctx, c.ID, pera, qty("4"), k.user); err != nil {
			t.Fatal(err)
		}
		if err := k.svc.ChangeUnit(ctx, pera, app.UnitChangeRequest{BaseUnitID: g, Factor: qty("180")}); !errors.Is(err, domain.ErrArticleInUse) {
			t.Fatalf("= %v; quiere ErrArticleInUse", err)
		}
		if err := k.svc.DiscardCount(ctx, c.ID, k.user); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("con una venta en curso no espera: 409 para reintentar", func(t *testing.T) {
		mango := ingredientIn(t, k.st, defaultCompanyID, "Mango unidad", "pieza")
		stockAdjust(t, k.st, defaultCompanyID, mango, "5")
		tx, err := k.st.Pool.Begin(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = tx.Rollback(context.Background()) }()
		if _, err := tx.Exec(context.Background(), `select 1 from stock_levels where ingredient_id = $1 for update`, mango); err != nil {
			t.Fatal(err)
		}
		var wg sync.WaitGroup
		wg.Add(1)
		var got error
		go func() {
			defer wg.Done()
			got = k.svc.ChangeUnit(ctx, mango, app.UnitChangeRequest{BaseUnitID: g, Factor: qty("200")})
		}()
		wg.Wait()
		if !errors.Is(got, domain.ErrConflict) {
			t.Fatalf("= %v; quiere un conflicto inmediato, no esperar", got)
		}
	})
}

// Con compras o ventas, se reemplaza: uno nuevo en la otra unidad, la existencia traspasada por
// sucursal con dos ajustes ligados, las recetas pasadas y el viejo archivado con su historia (E8).
func TestReplacingAnArticleWithPurchases(t *testing.T) {
	t.Parallel()
	k := newPurchaseKit(t)
	ctx := app.WithActorUser(k.ctx, k.user)
	if _, err := k.svc.CreateExpense(ctx, k.bultos("1", "500")); err != nil { // 25 kg a 0.02
		t.Fatal(err)
	}
	pz := unitID(t, k.st, "pieza")
	if _, err := k.st.Pool.Exec(context.Background(), `insert into ingredient_presentations (company_id, ingredient_id, name, qty_in_base) values ($1, $2, 'Bolsita', 2500)`, defaultCompanyID, k.azucar); err != nil {
		t.Fatal(err)
	}
	prev, err := k.svc.PreviewUnitChange(ctx, k.azucar, app.UnitChangeRequest{BaseUnitID: pz, Factor: qty("0.0002")})
	if err != nil || prev.Path != domain.UnitPathReplace {
		t.Fatalf("camino = %+v %v; con compras debe ser reemplazo", prev, err)
	}
	if err := k.svc.ChangeUnit(ctx, k.azucar, app.UnitChangeRequest{BaseUnitID: pz, Factor: qty("0.0002")}); err != nil {
		t.Fatal(err)
	}
	var newID int64
	if err := k.st.Pool.QueryRow(context.Background(), `select new_ingredient_id from ingredient_replacements where old_ingredient_id = $1`, k.azucar).Scan(&newID); err != nil {
		t.Fatal(err)
	}
	t.Run("el viejo queda archivado y conserva su compra", func(t *testing.T) {
		a, err := k.svc.GetArticle(ctx, k.azucar)
		if err != nil || a.IsActive {
			t.Fatalf("%+v %v", a, err)
		}
		var compras int
		if err := k.st.Pool.QueryRow(context.Background(), `select count(*) from stock_movements where ingredient_id = $1 and movement_type = 'compra'`, k.azucar).Scan(&compras); err != nil || compras != 1 {
			t.Fatalf("compras del viejo = %d %v", compras, err)
		}
		if _, level := sumOf(t, k.st, k.azucar); !level.IsZero() {
			t.Fatalf("existencia del viejo = %s; quiere 0", level)
		}
	})
	t.Run("el nuevo tiene la existencia convertida, ligada por el reemplazo, con su costo", func(t *testing.T) {
		n, err := k.svc.GetArticle(ctx, newID)
		if err != nil || !n.IsActive || n.BaseUnitCode != "pieza" || !n.OnHand.Equal(qty("5")) || !n.CurrentCost.Equal(qty("100")) {
			t.Fatalf("nuevo = %+v %v; quiere 5 piezas a $100", n, err)
		}
		var linked int
		if err := k.st.Pool.QueryRow(context.Background(), `select count(*) from stock_movements where replacement_id is not null and branch_id is not null and ingredient_id = any($1)`, []int64{k.azucar, newID}).Scan(&linked); err != nil || linked != 2 {
			t.Fatalf("movimientos ligados = %d %v; quiere la salida y la entrada", linked, err)
		}
	})
	t.Run("la receta activa pasó al nuevo con su cantidad convertida", func(t *testing.T) {
		var ing int64
		var q string
		if err := k.st.Pool.QueryRow(context.Background(), `select ri.ingredient_id, ri.quantity::text from recipe_items ri join products p on p.recipe_id = ri.recipe_id where p.id = $1`, k.product).Scan(&ing, &q); err != nil {
			t.Fatal(err)
		}
		if ing != newID || !qty(q).Equal(qty("0.004")) {
			t.Fatalf("receta → %d con %s; quiere el nuevo con 0.004 piezas (20 g)", ing, q)
		}
	})
	t.Run("el bulto pasó; la bolsita (media pieza) no, y quedó archivada en el viejo", func(t *testing.T) {
		var names []string
		rows, err := k.st.Pool.Query(context.Background(), `select name from ingredient_presentations where ingredient_id = $1 and active order by name`, newID)
		if err != nil {
			t.Fatal(err)
		}
		for rows.Next() {
			var n string
			_ = rows.Scan(&n)
			names = append(names, n)
		}
		rows.Close()
		if !slices.Equal(names, []string{"Bulto"}) {
			t.Fatalf("empaques del nuevo = %v", names)
		}
		var active bool
		if err := k.st.Pool.QueryRow(context.Background(), `select active from ingredient_presentations where ingredient_id = $1 and name like 'Bolsita%'`, k.azucar).Scan(&active); err != nil || active {
			t.Fatalf("bolsita activa %v %v", active, err)
		}
	})
	t.Run("deja ingredient.replaced y una corrección del costo traspasado", func(t *testing.T) {
		if n := auditCount(t, k.st, "ingredient.replaced", k.azucar); n != 1 {
			t.Fatalf("eventos = %d", n)
		}
		var corr int
		if err := k.st.Pool.QueryRow(context.Background(), `select count(*) from ingredient_cost_corrections where ingredient_id = $1`, newID).Scan(&corr); err != nil || corr != 1 {
			t.Fatalf("correcciones del nuevo = %d %v", corr, err)
		}
	})
}

// Toda tabla que apunta a un artículo está clasificada: o se convierte en el cambio de unidad, o lo
// bloquea, o no guarda cantidades. Una tabla nueva sin clasificar truena aquí.
func TestEveryTableThatPointsToAnArticleIsClassified(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	rows, err := st.Pool.Query(context.Background(), `
		select distinct c.conrelid::regclass::text from pg_constraint c
		 where c.contype = 'f' and c.confrelid = 'ingredients'::regclass order by 1`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var tbl string
		if err := rows.Scan(&tbl); err != nil {
			t.Fatal(err)
		}
		if _, ok := app.UnitChangeTables[tbl]; !ok {
			t.Errorf("la tabla %s apunta a artículos y el cambio de unidad no dice qué hace con ella", tbl)
		}
	}
}

func TestUnitChangeHTTPNeedsConfigure(t *testing.T) {
	t.Parallel()
	r, st, token := newStockAPI(t)
	id := strconv.FormatInt(ingredientIn(t, st, defaultCompanyID, "Unidad http", "pieza"), 10)
	cajero := token("unidad_cajero", "cajero")
	body := []byte(`{"baseUnitId":` + strconv.Itoa(int(gramos(t, st))) + `,"factor":"100"}`)
	for _, p := range []string{"/unit/preview", "/unit"} {
		if w := do(t, r, http.MethodPost, "/api/v1/stock/articles/"+id+p, cajero, body, "application/json"); w.Code != http.StatusForbidden {
			t.Errorf("cajero %s = %d, quiere 403", p, w.Code)
		}
	}
	admin := token("unidad_admin", "admin")
	if w := do(t, r, http.MethodPost, "/api/v1/stock/articles/"+id+"/unit", admin, body, "application/json"); w.Code != http.StatusNoContent {
		t.Fatalf("admin = %d %s", w.Code, w.Body.String())
	}
}

// Un preparado con movimientos no se reemplaza desde la ficha: su unidad cuelga del rendimiento de
// su receta. Respondía 400, como si la equivalencia estuviera mal escrita; es un conflicto con el
// estado del artículo, igual que corregir el costo de un preparado (FR-009).
func TestAPrepWithMovementsChangesUnitFromItsRecipe(t *testing.T) {
	t.Parallel()
	r, st, token := newStockAPI(t)
	base := ingredientIn(t, st, defaultCompanyID, "Azúcar para jarabe", "g")
	prep := ingredientIn(t, st, defaultCompanyID, "Jarabe con historia", "pieza")
	recipe := makeRecipe(t, st, map[int64]string{base: "10"})
	if _, err := st.Pool.Exec(context.Background(), `update ingredients set is_prep = true, recipe_id = $2, yield_qty = 1000 where id = $1`, prep, recipe); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Pool.Exec(context.Background(), `insert into stock_movements (company_id, item_type, ingredient_id, movement_type, quantity)
		values ($1, 'ingrediente', $2, 'produccion', 1000)`, defaultCompanyID, prep); err != nil {
		t.Fatal(err)
	}
	body := []byte(`{"baseUnitId":` + strconv.Itoa(int(gramos(t, st))) + `,"factor":"250"}`)
	w := do(t, r, http.MethodPost, "/api/v1/stock/articles/"+strconv.FormatInt(prep, 10)+"/unit", token("prep_admin", "admin"), body, "application/json")
	if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "cambia de unidad desde su receta") {
		t.Fatalf("= %d %s; quiere 409 «cambia de unidad desde su receta»", w.Code, w.Body.String())
	}
}
