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

// LO QUE ENCONTRÓ LA REVISIÓN DE CÓDIGO DE LA 042 (2026-10-19). Cada prueba nombra el defecto.

// La llave de ingredients.category_id no lleva la empresa y una FK salta RLS: un artículo podía
// ligarse a la categoría de otra empresa, y un id inexistente daba 500.
func TestAnArticleCannotTakeACategoryOfAnotherCompany(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	co := makeCompany(t, st, "categoria-ajena")
	user := makeUserIn(t, st, co, "cat_ajena", "admin")
	var foreign int64
	if err := st.Pool.QueryRow(context.Background(), `insert into ingredient_categories (company_id, name) values ($1, 'Lácteos de otra') returning id`, defaultCompanyID).Scan(&foreign); err != nil {
		t.Fatal(err)
	}
	svc, ctx := asWaste(t, co)
	ctx = app.WithActorUser(ctx, user)
	g := gramos(t, st)
	if _, err := svc.CreateArticle(ctx, app.ArticleCreate{Name: "Queso ajeno", BaseUnitID: g, CategoryID: &foreign}); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("crear con la categoría de otra empresa = %v; quiere ErrValidation", err)
	}
	missing := int64(99999999)
	if _, err := svc.CreateArticle(ctx, app.ArticleCreate{Name: "Queso fantasma", BaseUnitID: g, CategoryID: &missing}); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("crear con una categoría que no existe = %v; quiere ErrValidation, no 500", err)
	}
	id, err := svc.CreateArticle(ctx, app.ArticleCreate{Name: "Queso propio", BaseUnitID: g})
	if err != nil {
		t.Fatal(err)
	}
	a, _ := svc.GetArticle(ctx, id)
	if _, err := svc.UpdateArticle(ctx, id, app.ArticleUpdate{ExpectedUpdatedAt: a.UpdatedAt, Name: new("Queso propio"), CategoryID: app.Field[int64]{Set: true, Value: &foreign}}); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("editar con la categoría de otra empresa = %v; quiere ErrValidation", err)
	}
}

// Una equivalencia grande o una corrección a mano desbordaban la columna (500); una corrección
// negativa dejaba existencia negativa en el reemplazo; un costo convertido podía quedar en 0.
func TestUnitChangeRejectsWhatDoesNotFit(t *testing.T) {
	t.Parallel()
	k := newPurchaseKit(t)
	ctx := app.WithActorUser(k.ctx, k.user)
	g := gramos(t, k.st)
	pz := unitID(t, k.st, "pieza")

	big := ingredientIn(t, k.st, defaultCompanyID, "Grande a gramos", "pieza")
	stockAdjust(t, k.st, defaultCompanyID, big, "20000")
	if err := k.svc.ChangeUnit(ctx, big, app.UnitChangeRequest{BaseUnitID: g, Factor: qty("1000000")}); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("20,000 piezas × 1,000,000 = %v; quiere ErrValidation, no 500", err)
	}
	// El reemplazo (el azúcar tiene una compra) con correcciones imposibles.
	if _, err := k.svc.CreateExpense(ctx, k.bultos("1", "500")); err != nil {
		t.Fatal(err)
	}
	var branch int64
	if err := k.st.Pool.QueryRow(context.Background(), `select branch_id from stock_levels where ingredient_id = $1`, k.azucar).Scan(&branch); err != nil {
		t.Fatal(err)
	}
	stockKey := "stock:" + itoa(int(branch))
	for name, ov := range map[string]map[string]string{
		"existencia negativa":  {stockKey: "-50"},
		"existencia enorme":    {stockKey: "1e12"},
		"una llave que no hay": {"recipe:999999": "1"},
	} {
		overrides := map[string]decimal.Decimal{}
		for k2, v := range ov {
			overrides[k2] = qty(v)
		}
		if err := k.svc.ChangeUnit(ctx, k.azucar, app.UnitChangeRequest{BaseUnitID: pz, Factor: qty("0.0002"), Overrides: overrides}); !errors.Is(err, domain.ErrValidation) {
			t.Errorf("%s = %v; quiere ErrValidation", name, err)
		}
	}
	cheap := ingredientIn(t, k.st, defaultCompanyID, "Clavo barato", "pieza")
	if _, err := k.st.Pool.Exec(context.Background(), `update ingredients set current_cost = 0.001 where id = $1`, cheap); err != nil {
		t.Fatal(err)
	}
	if err := k.svc.ChangeUnit(ctx, cheap, app.UnitChangeRequest{BaseUnitID: g, Factor: qty("5000")}); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("$0.001 la pieza entre 5,000 g queda en 0 = %v; quiere ErrValidation", err)
	}
}

// En el cambio directo la existencia es la suma de su historia: una corrección de existencia se
// ignoraba en silencio. Ahora se rechaza.
func TestDirectUnitChangeRejectsAStockOverride(t *testing.T) {
	t.Parallel()
	k := newPurchaseKit(t)
	ctx := app.WithActorUser(k.ctx, k.user)
	lim := ingredientIn(t, k.st, defaultCompanyID, "Limón sin compras", "pieza")
	stockAdjust(t, k.st, defaultCompanyID, lim, "3")
	var branch int64
	if err := k.st.Pool.QueryRow(context.Background(), `select branch_id from stock_levels where ingredient_id = $1`, lim).Scan(&branch); err != nil {
		t.Fatal(err)
	}
	err := k.svc.ChangeUnit(ctx, lim, app.UnitChangeRequest{BaseUnitID: gramos(t, k.st), Factor: qty("100"),
		Overrides: map[string]decimal.Decimal{"stock:" + itoa(int(branch)): qty("500")}})
	if !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("= %v; quiere ErrValidation", err)
	}
}

// El reemplazo dejaba las recetas de productos inactivos en el artículo archivado: al reactivar el
// producto, vendía descontando de un artículo sin existencia.
func TestReplacementMovesInactiveRecipesToo(t *testing.T) {
	t.Parallel()
	k := newPurchaseKit(t)
	ctx := app.WithActorUser(k.ctx, k.user)
	if _, err := k.svc.CreateExpense(ctx, k.bultos("1", "500")); err != nil {
		t.Fatal(err)
	}
	if _, err := k.st.Pool.Exec(context.Background(), `update products set is_active = false where id = $1`, k.product); err != nil {
		t.Fatal(err)
	}
	if err := k.svc.ChangeUnit(ctx, k.azucar, app.UnitChangeRequest{BaseUnitID: unitID(t, k.st, "pieza"), Factor: qty("0.0002")}); err != nil {
		t.Fatal(err)
	}
	var ing int64
	if err := k.st.Pool.QueryRow(context.Background(), `select ri.ingredient_id from recipe_items ri join products p on p.recipe_id = ri.recipe_id where p.id = $1`, k.product).Scan(&ing); err != nil {
		t.Fatal(err)
	}
	if ing == k.azucar {
		t.Fatal("la receta del producto inactivo se quedó en el artículo archivado")
	}
}

// Reemplazar dos veces la misma unidad chocaba con el nombre «X (antes en kg)» que ya existía: 500.
func TestReplacingTheSameUnitTwiceDoesNotCollide(t *testing.T) {
	t.Parallel()
	k := newPurchaseKit(t)
	ctx := app.WithActorUser(k.ctx, k.user)
	g, pz := gramos(t, k.st), unitID(t, k.st, "pieza")
	current := ingredientIn(t, k.st, defaultCompanyID, "Vaivén", "g")
	for i, to := range []int16{pz, g, pz} {
		// Una compra por vuelta, para que cada cambio vaya por el reemplazo.
		if _, err := k.st.Pool.Exec(context.Background(), `insert into stock_movements (company_id, item_type, ingredient_id, movement_type, quantity, unit_cost)
			values ($1, 'ingrediente', $2, 'compra', 10, 1)`, defaultCompanyID, current); err != nil {
			t.Fatal(err)
		}
		factor := qty("0.01")
		if to == g {
			factor = qty("100")
		}
		if err := k.svc.ChangeUnit(ctx, current, app.UnitChangeRequest{BaseUnitID: to, Factor: factor}); err != nil {
			t.Fatalf("vuelta %d: %v", i+1, err)
		}
		if err := k.st.Pool.QueryRow(context.Background(), `select new_ingredient_id from ingredient_replacements where old_ingredient_id = $1`, current).Scan(&current); err != nil {
			t.Fatal(err)
		}
	}
}

// El reemplazo convertía los formatos de compra viejos pero los dejaba en el artículo archivado.
func TestReplacementMovesPurchaseFormats(t *testing.T) {
	t.Parallel()
	k := newPurchaseKit(t)
	ctx := app.WithActorUser(k.ctx, k.user)
	if _, err := k.svc.CreateExpense(ctx, k.bultos("1", "500")); err != nil {
		t.Fatal(err)
	}
	if _, err := k.st.Pool.Exec(context.Background(), `insert into ingredient_purchase_formats (company_id, ingredient_id, name, qty_in_base) values ($1, $2, 'Costal', 50000)`, defaultCompanyID, k.azucar); err != nil {
		t.Fatal(err)
	}
	if err := k.svc.ChangeUnit(ctx, k.azucar, app.UnitChangeRequest{BaseUnitID: unitID(t, k.st, "pieza"), Factor: qty("0.0002")}); err != nil {
		t.Fatal(err)
	}
	var owner int64
	var q string
	if err := k.st.Pool.QueryRow(context.Background(), `select ingredient_id, qty_in_base::text from ingredient_purchase_formats where name = 'Costal'`).Scan(&owner, &q); err != nil {
		t.Fatal(err)
	}
	if owner == k.azucar || !qty(q).Equal(qty("10")) {
		t.Fatalf("el costal quedó en %d con %s; quiere el nuevo con 10 piezas", owner, q)
	}
}

// Una existencia o un costo inicial minúsculo se redondeaba a 0 después de validar: 500 al insertar.
func TestATinyInitialStockOrCostIsNotAServerError(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	co := makeCompany(t, st, "inicial-minusculo")
	user := makeUserIn(t, st, co, "minusculo", "admin")
	svc, ctx := asWaste(t, co)
	ctx = app.WithActorUser(ctx, user)
	tiny := qty("0.00001")
	if _, err := svc.CreateArticle(ctx, app.ArticleCreate{Name: "Pizca", BaseUnitID: gramos(t, st), InitialStock: &tiny, InitialCost: new(qty("0.0000001"))}); err != nil {
		t.Fatalf("= %v; quiere que se ignore lo que redondea a cero, no un 500", err)
	}
}

// El historial contaba una compra recibida en dos entregas como dos, y seguía mostrando renglones
// anulados y gastos cancelados como precios; con eso la tendencia salía con una sola compra.
func TestCostHistoryCountsEachPurchaseOnce(t *testing.T) {
	t.Parallel()
	k := newPurchaseKit(t)
	ctx := app.WithActorUser(k.ctx, k.user)
	one := qty("1")
	in := k.bultos("2", "1000")
	in.Items[0].QtyReceived = &one
	exp, err := k.svc.CreateExpense(ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	item, applied := k.itemOf(t, exp)
	if err := k.svc.ReceiveItem(ctx, exp, item, app.ReceiveItemInput{Qty: one, SeenApplied: applied}, k.user); err != nil {
		t.Fatal(err)
	}
	page, err := k.svc.ArticleCostHistory(ctx, k.azucar, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.TrendPct != nil {
		t.Fatalf("una compra en dos entregas = %d renglones, tendencia %v; quiere 1 y sin tendencia", len(page.Items), page.TrendPct)
	}
	// Un renglón anulado y un gasto cancelado no son precios.
	exp2, err := k.svc.CreateExpense(ctx, k.bultos("1", "900"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := k.st.Pool.Exec(context.Background(), `update expense_items set voided_at = now() where expense_id = $1`, exp2); err != nil {
		t.Fatal(err)
	}
	exp3, err := k.svc.CreateExpense(ctx, k.bultos("1", "800"))
	if err != nil {
		t.Fatal(err)
	}
	if err := k.svc.CancelExpense(ctx, exp3, "Se pidió dos veces", k.user); err != nil {
		t.Fatal(err)
	}
	page, err = k.svc.ArticleCostHistory(ctx, k.azucar, nil)
	if err != nil || len(page.Items) != 1 {
		t.Fatalf("con un anulado y un cancelado = %d renglones %v; quiere 1", len(page.Items), err)
	}
}

// El conteo a ciegas se acepta con su riesgo (decisión del dueño, 2026-10-19), pero cada aviso
// queda en la bitácora: quien prueba cifras para adivinar lo esperado deja rastro.
func TestEachCountWarningIsAudited(t *testing.T) {
	t.Parallel()
	k := newCountKit(t)
	c, err := k.svc.OpenCount(k.ctx, k.user)
	if err != nil {
		t.Fatal(err)
	}
	ctx := app.WithActorUser(k.ctx, k.user)
	if _, err := k.svc.CaptureCountChecked(ctx, c.ID, k.azucar, app.CountCapture{Qty: qty("740000"), Confirmable: true}, k.user); !errors.Is(err, domain.ErrCountNeedsConfirm) {
		t.Fatalf("= %v", err)
	}
	var n int
	if err := k.st.Pool.QueryRow(context.Background(), `select count(*) from audit_events where action = 'stock.count_confirm_asked' and entity_id = $1`, k.azucar).Scan(&n); err != nil || n != 1 {
		t.Fatalf("avisos en la bitácora = %d; quiere 1", n)
	}
}

// Lo barato de la revisión: archivar no espera detrás de una venta, la vista previa revisa que esté
// activo, y «%» en el buscador es un signo, no un comodín.
func TestReviewSmallFixes(t *testing.T) {
	t.Parallel()
	k := newPurchaseKit(t)
	ctx := app.WithActorUser(k.ctx, k.user)
	t.Run("archivar con una venta en curso no espera", func(t *testing.T) {
		art := ingredientIn(t, k.st, defaultCompanyID, "Archivar ocupado", "pieza")
		tx, err := k.st.Pool.Begin(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = tx.Rollback(context.Background()) }()
		if _, err := tx.Exec(context.Background(), `select 1 from ingredients where id = $1 for update`, art); err != nil {
			t.Fatal(err)
		}
		if err := k.svc.ArchiveArticle(ctx, art); !errors.Is(err, domain.ErrConflict) {
			t.Fatalf("= %v; quiere un conflicto inmediato", err)
		}
	})
	t.Run("la vista previa de un archivado se niega como el cambio", func(t *testing.T) {
		art := ingredientIn(t, k.st, defaultCompanyID, "Vista archivada", "pieza")
		if _, err := k.st.Pool.Exec(context.Background(), `update ingredients set is_active = false where id = $1`, art); err != nil {
			t.Fatal(err)
		}
		if _, err := k.svc.PreviewUnitChange(ctx, art, app.UnitChangeRequest{BaseUnitID: gramos(t, k.st), Factor: qty("10")}); !errors.Is(err, domain.ErrConflict) {
			t.Fatalf("= %v", err)
		}
	})
	t.Run("«%» no es comodín", func(t *testing.T) {
		rows, err := k.svc.ListArticles(ctx, app.ArticleFilter{Q: "%"})
		if err != nil || len(rows) != 0 {
			t.Fatalf("«%%» encontró %d artículos %v; quiere 0", len(rows), err)
		}
	})
}
