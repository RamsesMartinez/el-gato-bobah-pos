//go:build integration

package integration

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"sync"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
)

// COMPRAS QUE SUMAN AL ALMACÉN (spec 038, historia 1). Medido en la copia de producción del
// 14-oct: todos los gastos «recibidos» y ni un renglón de insumo; ninguna compra sumaba.

type purchaseKit struct {
	st       *store.Store
	svc      *app.BackofficeService
	ctx      context.Context
	user     int64
	category int64
	azucar   int64 // en gramos
	bulto    int64 // empaque «Bulto = 25 kg» del azúcar
	product  int64 // lleva 20 g de azúcar (costo por receta)
}

func dd(s string) decimal.Decimal { return decimal.RequireFromString(s) }

func newPurchaseKit(t *testing.T) *purchaseKit {
	t.Helper()
	st := newTestStore(t)
	ctx := context.Background()
	k := &purchaseKit{st: st}
	k.user = makeUser(t, st, "comprador", "admin")
	if err := st.Pool.QueryRow(ctx, `insert into expense_categories (company_id, name, financial_group)
		values ($1, 'Compras de insumos', 'operacional') returning id`, defaultCompanyID).Scan(&k.category); err != nil {
		t.Fatal(err)
	}
	k.azucar = ingredientIn(t, st, defaultCompanyID, "Azúcar compra", "g")
	if err := st.Pool.QueryRow(ctx, `insert into ingredient_presentations (company_id, ingredient_id, name, qty_in_base)
		values ($1, $2, 'Bulto', 25000) returning id`, defaultCompanyID, k.azucar).Scan(&k.bulto); err != nil {
		t.Fatal(err)
	}
	k.product = makeProduct(t, st, "Agua de azúcar", dd("30"), false)
	recipe := makeRecipe(t, st, map[int64]string{k.azucar: "20"})
	if _, err := st.Pool.Exec(ctx, `update products set recipe_id = $2, cost_source = 'receta' where id = $1`, k.product, recipe); err != nil {
		t.Fatal(err)
	}
	k.svc, k.ctx = asTenant(t, defaultCompanyID)
	return k
}

// bultos arma un gasto de N bultos de azúcar que llegan completos.
func (k *purchaseKit) bultos(n, amount string) app.ExpenseInput {
	q := dd(n)
	return app.ExpenseInput{
		ReceivedAt: "2026-07-20", CategoryID: k.category, Amount: dd(amount), Status: domain.ExpensePendiente,
		Description: "Abarrotes", UserID: k.user,
		Items: []app.ExpenseItemInput{{
			ItemType: "ingrediente", IngredientID: &k.azucar, Description: "Azúcar", Quantity: q,
			PresentationID: &k.bulto, QtyReceived: &q, Amount: dd(amount),
		}},
	}
}

func (k *purchaseKit) onHand(t *testing.T) decimal.Decimal {
	t.Helper()
	return onHandIngredient(t, k.st, k.azucar)
}

func (k *purchaseKit) ingredientCost(t *testing.T) decimal.Decimal {
	t.Helper()
	var c decimal.Decimal
	if err := k.st.Pool.QueryRow(context.Background(), `select current_cost from ingredients where id = $1`, k.azucar).Scan(&c); err != nil {
		t.Fatal(err)
	}
	return c
}

// audited cuenta los eventos de bitácora de una acción sobre un gasto.
func (k *purchaseKit) audited(t *testing.T, action string, expense int64) int {
	t.Helper()
	var n int
	if err := k.st.Pool.QueryRow(context.Background(), `select count(*) from audit_events
		where action = $1 and entity_type = 'expense' and entity_id = $2`, action, expense).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func (k *purchaseKit) itemOf(t *testing.T, expense int64) (id int64, applied decimal.Decimal) {
	t.Helper()
	if err := k.st.Pool.QueryRow(context.Background(), `select id, applied_qty_base from expense_items
		where expense_id = $1 and voided_at is null order by id limit 1`, expense).Scan(&id, &applied); err != nil {
		t.Fatal(err)
	}
	return id, applied
}

// Defecto encontrado al planear (research R0): la pantalla mandaba «recibido» aunque el gasto no
// trajera mercancía, el servidor lo guardaba, y ese gasto ya no podía recibir nada después.
func TestAnExpenseWithoutGoodsIsNotMarkedReceived(t *testing.T) {
	t.Parallel()
	k := newPurchaseKit(t)
	id, err := k.svc.CreateExpense(k.ctx, app.ExpenseInput{
		ReceivedAt: "2026-07-20", CategoryID: k.category, Amount: dd("640"), Status: domain.ExpensePendiente,
		Description: "Gas", UserID: k.user,
		Items: []app.ExpenseItemInput{{Description: "Gas LP", Quantity: dd("1"), Amount: dd("640")}},
	})
	if err != nil {
		t.Fatal(err)
	}
	var received *time.Time
	if err := k.st.Pool.QueryRow(context.Background(), `select received_at from expenses where id = $1`, id).Scan(&received); err != nil {
		t.Fatal(err)
	}
	if received != nil {
		t.Fatal("un gasto sin mercancía quedó marcado como recibido: después ya no podría recibir lo que llegue")
	}
}

// Segundo defecto (research R0): el alta rápida tomaba la unidad de compra (kg) como unidad del
// insumo, y lo guardado —que se cuenta en gramos— se leería 1,000 veces mal.
func TestAnIngredientIsCreatedOnlyInTheBaseUnitOfItsKind(t *testing.T) {
	t.Parallel()
	k := newPurchaseKit(t)
	if _, err := k.svc.CreateIngredient(k.ctx, app.IngredientInput{Name: "Harina kg", BaseUnitID: unitID(t, k.st, "kg")}); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("insumo en kg = %v, quiere ErrValidation", err)
	}
	for _, code := range []string{"g", "ml", "pieza", "cm"} {
		if _, err := k.svc.CreateIngredient(k.ctx, app.IngredientInput{Name: "Insumo en " + code, BaseUnitID: unitID(t, k.st, code)}); err != nil {
			t.Errorf("insumo en %s: %v", code, err)
		}
	}
}

func TestAPurchaseInPackagesAddsToStockAndSetsTheCost(t *testing.T) {
	t.Parallel()
	k := newPurchaseKit(t)
	id, err := k.svc.CreateExpense(k.ctx, k.bultos("2", "1000"))
	if err != nil {
		t.Fatalf("compra de 2 bultos: %v", err)
	}
	if got := k.onHand(t); !got.Equal(dd("50000")) {
		t.Fatalf("lo guardado = %s g; quiere 50000 (2 bultos de 25 kg)", got)
	}
	item, applied := k.itemOf(t, id)
	if !applied.Equal(dd("50000")) {
		t.Errorf("applied_qty_base = %s; quiere 50000", applied)
	}
	var movItem, movUser *int64
	if err := k.st.Pool.QueryRow(context.Background(), `select expense_item_id, user_id from stock_movements
		where expense_id = $1 and movement_type = 'compra'`, id).Scan(&movItem, &movUser); err != nil {
		t.Fatalf("el movimiento de la compra: %v", err)
	}
	if movItem == nil || *movItem != item || movUser == nil || *movUser != k.user {
		t.Errorf("el movimiento no dice su renglón o quién (SC-003): renglón %v, usuario %v", movItem, movUser)
	}
	// Última compra: $1,000 / 50,000 g = $0.02 por gramo; el producto con 20 g pasa a costar $0.40.
	if c := k.ingredientCost(t); !c.Equal(dd("0.02")) {
		t.Errorf("costo del azúcar = %s; quiere 0.02 por gramo", c)
	}
	var pc decimal.Decimal
	if err := k.st.Pool.QueryRow(context.Background(), `select current_cost from products where id = $1`, k.product).Scan(&pc); err != nil {
		t.Fatal(err)
	}
	if !pc.Equal(dd("0.4")) {
		t.Errorf("costo del producto = %s; quiere 0.40 (el costo del insumo no se propagó)", pc)
	}
	if n := k.audited(t, "expense.received", id); n != 1 {
		t.Errorf("eventos expense.received = %d; quiere 1 (lo que entró al almacén deja rastro)", n)
	}
}

func TestWeightedCostAndGifts(t *testing.T) {
	t.Parallel()
	k := newPurchaseKit(t)
	if _, err := k.st.Pool.Exec(context.Background(), `update business_settings set costing_method = 'weighted' where company_id = $1`, defaultCompanyID); err != nil {
		t.Fatal(err)
	}
	if _, err := k.svc.CreateExpense(k.ctx, k.bultos("1", "500")); err != nil { // 25 kg a 0.02
		t.Fatal(err)
	}
	if _, err := k.svc.CreateExpense(k.ctx, k.bultos("1", "1000")); err != nil { // 25 kg a 0.04
		t.Fatal(err)
	}
	if c := k.ingredientCost(t); !c.Equal(dd("0.03")) {
		t.Fatalf("promedio ponderado = %s; quiere 0.03", c)
	}
	// Un regalo suma a lo guardado y no mueve el costo.
	gift := k.bultos("1", "0")
	gift.Amount = dd("10")
	gift.Items[0].IsGift = true
	gift.Items = append(gift.Items, app.ExpenseItemInput{Description: "Envío", Quantity: dd("1"), Amount: dd("10")})
	if _, err := k.svc.CreateExpense(k.ctx, gift); err != nil {
		t.Fatalf("regalo: %v", err)
	}
	if c := k.ingredientCost(t); !c.Equal(dd("0.03")) {
		t.Errorf("el regalo movió el costo a %s", c)
	}
	if got := k.onHand(t); !got.Equal(dd("75000")) {
		t.Errorf("lo guardado = %s; quiere 75000 con el regalo", got)
	}
}

// Las reglas de dinero las valida el servidor (FR-004a/b), no solo la pantalla.
func TestPurchaseMoneyRules(t *testing.T) {
	t.Parallel()
	k := newPurchaseKit(t)
	zero := k.bultos("1", "0")
	zero.Amount = dd("100")
	over := k.bultos("1", "500")
	over.Amount = dd("400")
	noCat := k.bultos("1", "500")
	noCat.CategoryID = 0
	otherIng := k.bultos("1", "500")
	leche := ingredientIn(t, k.st, defaultCompanyID, "Leche compra", "ml")
	otherIng.Items[0].IngredientID = &leche // el bulto es del azúcar
	for name, in := range map[string]app.ExpenseInput{
		"renglón de mercancía en 0 sin ser regalo": zero,
		"renglones que pasan del total":            over,
		"sin categoría":                            noCat,
		"empaque de otro insumo":                   otherIng,
	} {
		if _, err := k.svc.CreateExpense(k.ctx, in); !errors.Is(err, domain.ErrValidation) {
			t.Errorf("%s = %v, quiere ErrValidation", name, err)
		}
	}
	if got := k.onHand(t); !got.IsZero() {
		t.Errorf("un gasto rechazado sumó %s g", got)
	}
}

// Corregir por diferencia (R2): nunca se duplica ni se borra un movimiento (SC-004).
func TestCorrectingWhatArrivedByDifference(t *testing.T) {
	t.Parallel()
	k := newPurchaseKit(t)
	id, err := k.svc.CreateExpense(k.ctx, k.bultos("3", "1500"))
	if err != nil {
		t.Fatal(err)
	}
	item, applied := k.itemOf(t, id)
	two := app.StockLineInput{ID: &item, SeenApplied: &applied, ItemType: "ingrediente", IngredientID: &k.azucar,
		Quantity: dd("3"), Received: dd("2"), PresentationID: &k.bulto, Amount: dd("1500"), Description: "Azúcar"}
	if _, err := k.svc.SaveStockLines(k.ctx, id, []app.StockLineInput{two}, k.user); err != nil {
		t.Fatalf("corregir a 2 bultos: %v", err)
	}
	if got := k.onHand(t); !got.Equal(dd("50000")) {
		t.Fatalf("tras corregir = %s; quiere 50000", got)
	}
	if n := k.audited(t, "expense.stock_corrected", id); n != 1 {
		t.Errorf("eventos expense.stock_corrected = %d; quiere 1", n)
	}
	// La misma corrección con lo aplicado viejo: otra tableta ya la había hecho → 409.
	if _, err := k.svc.SaveStockLines(k.ctx, id, []app.StockLineInput{two}, k.user); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("corrección con lo aplicado viejo = %v, quiere ErrConflict", err)
	}
	// Con lo aplicado de hoy y lo mismo deseado: diferencia cero, ningún movimiento nuevo.
	_, applied = k.itemOf(t, id)
	two.SeenApplied = &applied
	var before int
	_ = k.st.Pool.QueryRow(context.Background(), `select count(*) from stock_movements where expense_id = $1`, id).Scan(&before)
	if _, err := k.svc.SaveStockLines(k.ctx, id, []app.StockLineInput{two}, k.user); err != nil {
		t.Fatal(err)
	}
	var after int
	_ = k.st.Pool.QueryRow(context.Background(), `select count(*) from stock_movements where expense_id = $1`, id).Scan(&after)
	if after != before {
		t.Fatalf("reenviar lo mismo insertó %d movimientos", after-before)
	}
	// Quitar el renglón: se revierte y se marca, no se borra.
	if _, err := k.svc.SaveStockLines(k.ctx, id, nil, k.user); err != nil {
		t.Fatalf("quitar el renglón: %v", err)
	}
	if got := k.onHand(t); !got.IsZero() {
		t.Fatalf("tras quitar el renglón quedan %s g", got)
	}
	var voided *time.Time
	if err := k.st.Pool.QueryRow(context.Background(), `select voided_at from expense_items where id = $1`, item).Scan(&voided); err != nil {
		t.Fatal(err)
	}
	if voided == nil {
		t.Error("el renglón quitado no quedó marcado")
	}
}

func TestCancellingAReceivedPurchaseReversesWithoutDeleting(t *testing.T) {
	t.Parallel()
	k := newPurchaseKit(t)
	id, err := k.svc.CreateExpense(k.ctx, k.bultos("1", "500"))
	if err != nil {
		t.Fatal(err)
	}
	if err := k.svc.CancelExpense(k.ctx, id, "no era de aquí", k.user); err != nil {
		t.Fatalf("anular una compra recibida: %v", err)
	}
	if got := k.onHand(t); !got.IsZero() {
		t.Fatalf("anulada, quedan %s g", got)
	}
	var n int
	_ = k.st.Pool.QueryRow(context.Background(), `select count(*) from stock_movements where expense_id = $1`, id).Scan(&n)
	if n != 2 {
		t.Errorf("movimientos = %d; quiere 2 (la entrada y su reversión, ninguno borrado)", n)
	}
}

// Una venta tiene tomadas las existencias en el orden de su ticket; si la compra esperara por el
// insumo, un ciclo de esperas la cancelaría (el choque que el 037 resolvió con NOWAIT).
func TestAPurchaseNeverWaitsForAnIngredientHeldByASale(t *testing.T) {
	t.Parallel()
	k := newPurchaseKit(t)
	tx, err := k.st.Pool.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	if _, err := tx.Exec(context.Background(), `select 1 from ingredients where id = $1 for update`, k.azucar); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { _, err := k.svc.CreateExpense(k.ctx, k.bultos("1", "500")); done <- err }()
	select {
	case err := <-done:
		if !errors.Is(err, domain.ErrConflict) {
			t.Fatalf("con el insumo tomado = %v, quiere ErrConflict de inmediato", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("la compra esperó por el insumo")
	}
	if got := k.onHand(t); !got.IsZero() {
		t.Errorf("el rechazo sumó %s g", got)
	}
}

func TestAPurchaseWithTwoBranchesAndNoSelectorIsNotGuessed(t *testing.T) {
	t.Parallel()
	k := newPurchaseKit(t)
	addBranch(t, k.st, defaultCompanyID, "NORTE")
	if _, err := k.svc.CreateExpense(k.ctx, k.bultos("1", "500")); !errors.Is(err, domain.ErrBranchAmbiguous) {
		t.Fatalf("con dos sucursales = %v, quiere ErrBranchAmbiguous", err)
	}
}

// Las compras de una empresa no se ven ni se corrigen desde otra.
func TestPurchasesAreIsolated(t *testing.T) {
	t.Parallel()
	k := newPurchaseKit(t)
	id, err := k.svc.CreateExpense(k.ctx, k.bultos("1", "500"))
	if err != nil {
		t.Fatal(err)
	}
	item, applied := k.itemOf(t, id)
	other := makeCompany(t, k.st, "compras-otra")
	inTheThreeCases(t, defaultCompanyID, other, func(t *testing.T, as *store.Store, ctx context.Context) {
		svc := app.NewBackofficeService(as, nil)
		line := app.StockLineInput{ID: &item, SeenApplied: &applied, ItemType: "ingrediente", IngredientID: &k.azucar,
			Quantity: dd("1"), Received: dd("0"), Amount: dd("500"), Description: "Azúcar"}
		if _, err := svc.SaveStockLines(ctx, id, []app.StockLineInput{line}, k.user); err == nil {
			t.Fatal("corrigió una compra de otra empresa")
		}
		if err := svc.CancelExpense(ctx, id, "ajena", k.user); err == nil {
			t.Fatal("anuló una compra de otra empresa")
		}
	})
	if got := k.onHand(t); !got.Equal(dd("25000")) {
		t.Errorf("lo guardado cambió a %s desde otra empresa", got)
	}
}

// Regla 4: la unidad de un insumo no cambia después de crearlo. Hoy ninguna consulta la reescribe;
// si aparece una, lo guardado y las recetas se leerían en otra unidad sin un solo error.
func TestNoQueryRewritesTheBaseUnitOfAnIngredient(t *testing.T) {
	t.Parallel()
	files, err := filepath.Glob("../../queries/*.sql")
	if err != nil || len(files) == 0 {
		t.Fatalf("no encontré las consultas: %v", err)
	}
	stmt := regexp.MustCompile(`(?is)update\s+ingredients\b[^;]*;`)
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for _, u := range stmt.FindAllString(string(b), -1) {
			if regexp.MustCompile(`(?i)\bbase_unit_id\s*=`).MatchString(u) {
				t.Errorf("%s reescribe la unidad de un insumo:\n%s", filepath.Base(f), u)
			}
		}
	}
}

// Un renglón que no es de almacén (gas, envío) no toca nada, y un gasto heredado sin renglones
// queda como estaba: ni movimientos ni «recibido» (FR-007).
func TestLinesOutsideTheStockTouchNothing(t *testing.T) {
	t.Parallel()
	k := newPurchaseKit(t)
	id, err := k.svc.CreateExpense(k.ctx, app.ExpenseInput{
		CategoryID: k.category, Amount: dd("120"), Status: domain.ExpensePendiente, Description: "Envío", UserID: k.user,
		Items: []app.ExpenseItemInput{{Description: "Envío", Quantity: dd("1"), Amount: dd("120")}},
	})
	if err != nil {
		t.Fatal(err)
	}
	var n int
	_ = k.st.Pool.QueryRow(context.Background(), `select count(*) from stock_movements where expense_id = $1`, id).Scan(&n)
	if n != 0 {
		t.Errorf("un renglón que no es de almacén dejó %d movimientos", n)
	}
	// La bandeja «Por recibir» no lo ofrece: no trae nada que recibir.
	list, _, err := k.svc.ListExpenses(k.ctx, "", true, "date", "desc", 50, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range list {
		if e.ID == id {
			t.Error("un gasto sin mercancía apareció en «Por recibir»")
		}
	}
}

// «Llegó todo» sobre una compra capturada antes: suma lo que faltaba y la saca de «Por recibir».
// Un segundo toque no suma otra vez.
func TestReceivingEverythingLaterAddsOnce(t *testing.T) {
	t.Parallel()
	k := newPurchaseKit(t)
	in := k.bultos("2", "1000")
	in.ReceivedAt = ""
	in.Items[0].QtyReceived = nil
	id, err := k.svc.CreateExpense(k.ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	if got := k.onHand(t); !got.IsZero() {
		t.Fatalf("capturada sin llegar sumó %s g", got)
	}
	pending := func() bool {
		list, _, err := k.svc.ListExpenses(k.ctx, "", true, "date", "desc", 50, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range list {
			if e.ID == id {
				return true
			}
		}
		return false
	}
	if !pending() {
		t.Fatal("la compra sin llegar no aparece en «Por recibir»")
	}
	if err := k.svc.ReceiveExpense(k.ctx, id, "2026-07-21", k.user); err != nil {
		t.Fatalf("llegó todo: %v", err)
	}
	if got := k.onHand(t); !got.Equal(dd("50000")) {
		t.Fatalf("tras «Llegó todo» = %s; quiere 50000", got)
	}
	if pending() {
		t.Error("ya llegó todo y sigue en «Por recibir»")
	}
	if err := k.svc.ReceiveExpense(k.ctx, id, "", k.user); !errors.Is(err, domain.ErrConflict) {
		t.Errorf("segundo «Llegó todo» = %v; quiere ErrConflict", err)
	}
	if got := k.onHand(t); !got.Equal(dd("50000")) {
		t.Errorf("el segundo toque sumó: %s", got)
	}
}

// Dos tabletas corrigiendo la misma compra a la vez: el gasto bloqueado las forma, y la segunda ve
// que lo aplicado ya no es lo que leyó. Nunca suman las dos.
func TestTwoCorrectionsOfTheSameLineOneLoses(t *testing.T) {
	t.Parallel()
	k := newPurchaseKit(t)
	id, err := k.svc.CreateExpense(k.ctx, k.bultos("3", "1500"))
	if err != nil {
		t.Fatal(err)
	}
	item, applied := k.itemOf(t, id)
	line := app.StockLineInput{ID: &item, SeenApplied: &applied, ItemType: "ingrediente", IngredientID: &k.azucar,
		Quantity: dd("3"), Received: dd("1"), PresentationID: &k.bulto, Amount: dd("1500"), Description: "Azúcar"}
	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i := range errs {
		svc, ctx := asTenant(t, defaultCompanyID) // una conexión por tableta
		wg.Go(func() { _, errs[i] = svc.SaveStockLines(ctx, id, []app.StockLineInput{line}, k.user) })
	}
	wg.Wait()
	ok := 0
	for _, err := range errs {
		switch {
		case err == nil:
			ok++
		case !errors.Is(err, domain.ErrConflict):
			t.Fatalf("corrección simultánea = %v; quiere éxito o ErrConflict", err)
		}
	}
	if ok != 1 {
		t.Fatalf("correcciones aceptadas = %d; quiere 1", ok)
	}
	if got := k.onHand(t); !got.Equal(dd("25000")) {
		t.Errorf("lo guardado = %s; quiere 25000 (un bulto)", got)
	}
}

// Dos compras del mismo insumo a la vez con promedio ponderado: la segunda no espera ni promedia
// sobre el costo viejo; o entra después de la primera o se rechaza para reintentar.
func TestTwoWeightedPurchasesAtOnceNeverLoseOne(t *testing.T) {
	t.Parallel()
	k := newPurchaseKit(t)
	if _, err := k.st.Pool.Exec(context.Background(), `update business_settings set costing_method = 'weighted' where company_id = $1`, defaultCompanyID); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	errs := make([]error, 2)
	amounts := []string{"500", "1000"}
	for i := range errs {
		svc, ctx := asTenant(t, defaultCompanyID)
		wg.Go(func() { _, errs[i] = svc.CreateExpense(ctx, k.bultos("1", amounts[i])) })
	}
	wg.Wait()
	ok := 0
	for _, err := range errs {
		switch {
		case err == nil:
			ok++
		case !errors.Is(err, domain.ErrConflict):
			t.Fatalf("compra simultánea = %v; quiere éxito o ErrConflict", err)
		}
	}
	if got, want := k.onHand(t), dd("25000").Mul(decimal.NewFromInt(int64(ok))); !got.Equal(want) {
		t.Fatalf("lo guardado = %s con %d compras aceptadas; quiere %s", got, ok, want)
	}
	if ok == 2 {
		if c := k.ingredientCost(t); !c.Equal(dd("0.03")) {
			t.Errorf("con las dos compras el promedio = %s; quiere 0.03 (una promedió sobre el costo viejo)", c)
		}
	}
}
