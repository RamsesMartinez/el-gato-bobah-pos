//go:build integration

package integration

import (
	"context"
	"testing"

	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
)

// UNA COMPRA VIEJA NO PISA EL COSTO DE UNA MÁS RECIENTE (#187). Con «último precio», el costo
// vigente lo pone la compra que LLEGÓ más recientemente, no la última que se capturó: una compra del
// día 5 capturada después de una del día 31 bajaba el costo al precio del día 5, y las recetas y el
// margen quedaban con un precio viejo sin que nada fallara.

func costOf(t *testing.T, st *store.Store, ing int64) decimal.Decimal {
	t.Helper()
	var c decimal.Decimal
	if err := st.Pool.QueryRow(context.Background(), `select current_cost from ingredients where id = $1`, ing).Scan(&c); err != nil {
		t.Fatal(err)
	}
	return c
}

func TestAPurchaseThatArrivedEarlierDoesNotOverwriteTheCostOfANewerOne(t *testing.T) {
	t.Parallel()
	k := newPurchaseKit(t)
	// Llegó el 20 de julio: $1,000 por 50,000 g = $0.02.
	if _, err := k.svc.CreateExpense(k.ctx, k.bultos("2", "1000")); err != nil {
		t.Fatal(err)
	}
	// Se captura después una compra olvidada que llegó el 5 de julio: $750 por 50,000 g = $0.015.
	old := k.bultos("2", "750")
	old.ReceivedAt = "2026-07-05"
	if _, err := k.svc.CreateExpense(k.ctx, old); err != nil {
		t.Fatal(err)
	}
	if c := k.ingredientCost(t); !c.Equal(dd("0.02")) {
		t.Fatalf("costo del azúcar = %s; quiere 0.02: la compra del 5-jul se capturó después pero llegó antes que la del 20-jul", c)
	}
	var pc decimal.Decimal
	if err := k.st.Pool.QueryRow(context.Background(), `select current_cost from products where id = $1`, k.product).Scan(&pc); err != nil {
		t.Fatal(err)
	}
	if !pc.Equal(dd("0.4")) {
		t.Errorf("costo del producto = %s; quiere 0.40 (la receta tomó el precio viejo)", pc)
	}
	// El mismo día que la más reciente: la que se captura después sí manda.
	same := k.bultos("2", "1100")
	if _, err := k.svc.CreateExpense(k.ctx, same); err != nil {
		t.Fatal(err)
	}
	if c := k.ingredientCost(t); !c.Equal(dd("0.022")) {
		t.Errorf("costo tras otra compra del 20-jul = %s; quiere 0.022 (mismo día de llegada: manda la última capturada)", c)
	}
	// Una más reciente siempre manda.
	newer := k.bultos("2", "1200")
	newer.ReceivedAt = "2026-07-25"
	if _, err := k.svc.CreateExpense(k.ctx, newer); err != nil {
		t.Fatal(err)
	}
	if c := k.ingredientCost(t); !c.Equal(dd("0.024")) {
		t.Errorf("costo tras la compra del 25-jul = %s; quiere 0.024", c)
	}
}

// Una compra cancelada no cuenta como «la más reciente»: si contara, una compra anulada del 31
// impediría para siempre que una compra real del 20 fijara el costo.
func TestACancelledNewerPurchaseDoesNotHoldTheCost(t *testing.T) {
	t.Parallel()
	k := newPurchaseKit(t)
	newer := k.bultos("2", "1000")
	newer.ReceivedAt = "2026-07-31"
	id, err := k.svc.CreateExpense(k.ctx, newer)
	if err != nil {
		t.Fatal(err)
	}
	if err := k.svc.CancelExpense(k.ctx, id, "prueba", k.user); err != nil {
		t.Fatalf("anular: %v", err)
	}
	old := k.bultos("2", "750")
	old.ReceivedAt = "2026-07-05"
	if _, err := k.svc.CreateExpense(k.ctx, old); err != nil {
		t.Fatal(err)
	}
	if c := k.ingredientCost(t); !c.Equal(dd("0.015")) {
		t.Fatalf("costo = %s; quiere 0.015: la compra del 31-jul está anulada y no debe detener el costo", c)
	}
}

// Revisión de código del #187: un renglón corregido a «no llegó» ya no aportó nada; su movimiento
// positivo sigue en la tabla, pero no puede detener el costo como si fuera la compra más reciente.
func TestALineCorrectedToNotArrivedDoesNotHoldTheCost(t *testing.T) {
	t.Parallel()
	k := newPurchaseKit(t)
	newer := k.bultos("2", "1000")
	newer.ReceivedAt = "2026-07-31"
	id, err := k.svc.CreateExpense(k.ctx, newer)
	if err != nil {
		t.Fatal(err)
	}
	item, applied := k.itemOf(t, id)
	none := app.StockLineInput{ID: &item, SeenApplied: &applied, ItemType: "ingrediente", IngredientID: &k.azucar,
		Quantity: dd("2"), Received: dd("0"), PresentationID: &k.bulto, Amount: dd("1000"), Description: "Azúcar"}
	if _, err := k.svc.SaveStockLines(k.ctx, id, []app.StockLineInput{none}, k.user); err != nil {
		t.Fatal(err)
	}
	old := k.bultos("2", "750")
	old.ReceivedAt = "2026-07-05"
	if _, err := k.svc.CreateExpense(k.ctx, old); err != nil {
		t.Fatal(err)
	}
	if c := k.ingredientCost(t); !c.Equal(dd("0.015")) {
		t.Fatalf("costo = %s; quiere 0.015: la compra del 31-jul se corrigió a «no llegó» y no debe detener el costo", c)
	}
}

// Editar un gasto viejo y agregarle un renglón que «ya llegó» ese día: la vista previa dice que el
// costo no cambia porque hay una compra más reciente, y al confirmar no cambia.
func TestEditingAnOldExpenseKeepsTheCostOfANewerPurchase(t *testing.T) {
	t.Parallel()
	k := newEditKit(t)
	ten := dd("10")
	if _, err := k.svc.CreateExpense(k.ctx, app.ExpenseInput{
		ReceivedAt: k.day(-1), CategoryID: k.category, Amount: dd("25.00"), Status: domain.ExpensePendiente,
		Description: "Tapas", UserID: k.user,
		Items: []app.ExpenseItemInput{{ItemType: "ingrediente", IngredientID: &k.tapas, Description: "Tapas",
			Quantity: ten, UnitID: &k.pieza, QtyReceived: &ten, Amount: dd("25.00")}},
	}); err != nil {
		t.Fatal(err)
	}
	if c := costOf(t, k.st, k.tapas); !c.Equal(dd("2.5")) {
		t.Fatalf("costo de partida = %s; quiere 2.50", c)
	}
	id := k.paidWithoutGoods(t, "20.00", k.day(-5))
	in := app.EditInput{Add: []app.EditLineInput{k.line(k.tapas, "Tapas olvidadas", "10", "20.00", k.day(-5))}, CanReceive: true}
	pre, err := k.svc.EditExpense(k.ctx, id, in, k.user, true)
	if err != nil {
		t.Fatal(err)
	}
	var effect *app.CostEffect
	for i := range pre.Preview.Costs {
		if pre.Preview.Costs[i].IngredientID == k.tapas {
			effect = &pre.Preview.Costs[i]
		}
	}
	if effect == nil {
		t.Fatalf("la vista previa no dice qué pasa con el costo de las tapas: %+v", pre.Preview.Costs)
	}
	if !effect.After.Equal(dd("2.5")) {
		t.Errorf("la vista previa baja el costo a %s; quiere 2.50: la compra del día −1 es más reciente que la del día −5", effect.After)
	}
	if effect.NewerPurchaseOn == nil || *effect.NewerPurchaseOn != k.day(-1) {
		t.Errorf("la vista previa no nombra la compra más reciente: %v; quiere %s", effect.NewerPurchaseOn, k.day(-1))
	}
	k.confirmEdit(t, id, app.EditInput{Add: in.Add}, nil)
	if c := costOf(t, k.st, k.tapas); !c.Equal(dd("2.5")) {
		t.Fatalf("costo tras confirmar = %s; quiere 2.50", c)
	}
}

// El destino de una unión hereda la compra más reciente de sus orígenes: una compra vieja del destino
// no puede pisar el costo que puso la compra más reciente de una marca unida.
func TestAnOldPurchaseOfAMergeTargetRespectsItsSourcesNewerPurchase(t *testing.T) {
	t.Parallel()
	k := newMergeKit(t)
	var g int16
	if err := k.st.Pool.QueryRow(context.Background(), `select id from units where code = 'g'`).Scan(&g); err != nil {
		t.Fatal(err)
	}
	cat := categoryIn(t, k.st)
	buy := func(ing int64, day, amount string) {
		t.Helper()
		q := dd("1000")
		if _, err := k.svc.CreateExpense(k.ctx, app.ExpenseInput{
			ReceivedAt: day, CategoryID: cat, Amount: dd(amount), Status: domain.ExpensePendiente,
			Description: "Leche", UserID: k.admin,
			Items: []app.ExpenseItemInput{{ItemType: "ingrediente", IngredientID: &ing, Description: "Leche",
				Quantity: q, UnitID: &g, QtyReceived: &q, Amount: dd(amount)}},
		}); err != nil {
			t.Fatal(err)
		}
	}
	buy(k.b, "2026-07-31", "30") // $0.03 el gramo, la más reciente
	k.merge(t, k.a, k.req(k.b))
	if c := costOf(t, k.st, k.a); !c.Equal(dd("0.03")) {
		t.Fatalf("costo del destino tras unir = %s; quiere 0.03", c)
	}
	buy(k.a, "2026-07-05", "20")
	if c := costOf(t, k.st, k.a); !c.Equal(dd("0.03")) {
		t.Fatalf("costo del destino = %s; quiere 0.03: la compra del origen (31-jul) es más reciente que la del 5-jul", c)
	}
}

// Al unir, «la compra más reciente» también es la que llegó después, no la que se capturó después.
func TestMergeTakesTheCostOfTheLatestArrivalNotTheLatestCapture(t *testing.T) {
	t.Parallel()
	k := newMergeKit(t)
	var g int16
	if err := k.st.Pool.QueryRow(context.Background(), `select id from units where code = 'g'`).Scan(&g); err != nil {
		t.Fatal(err)
	}
	cat := categoryIn(t, k.st)
	buy := func(ing int64, day, amount string) {
		t.Helper()
		q := dd("1000")
		if _, err := k.svc.CreateExpense(k.ctx, app.ExpenseInput{
			ReceivedAt: day, CategoryID: cat, Amount: dd(amount), Status: domain.ExpensePendiente,
			Description: "Leche", UserID: k.admin,
			Items: []app.ExpenseItemInput{{ItemType: "ingrediente", IngredientID: &ing, Description: "Leche",
				Quantity: q, UnitID: &g, QtyReceived: &q, Amount: dd(amount)}},
		}); err != nil {
			t.Fatal(err)
		}
	}
	buy(k.b, "2026-07-31", "30") // capturada primero, llegó después
	buy(k.a, "2026-07-05", "20") // capturada después, llegó antes
	k.merge(t, k.a, k.req(k.b))
	if c := costOf(t, k.st, k.a); !c.Equal(dd("0.03")) {
		t.Fatalf("costo del destino tras unir = %s; quiere 0.03 (la compra del origen llegó el 31-jul; la del destino, el 5-jul)", c)
	}
}

// La consulta de la llegada más reciente lee compras de la empresa: bajo RLS, la de otra empresa no se
// ve en ninguno de los tres casos.
func TestNewestPurchaseArrivalIsIsolatedInTheThreeCases(t *testing.T) {
	t.Parallel()
	k := newPurchaseKit(t)
	if _, err := k.svc.CreateExpense(k.ctx, k.bultos("2", "1000")); err != nil {
		t.Fatal(err)
	}
	other := makeCompany(t, k.st, "llegada-mas-reciente-otra")
	inTheThreeCases(t, defaultCompanyID, other, func(t *testing.T, st *store.Store, ctx context.Context) {
		got, err := st.QC(ctx).NewestPurchaseArrival(ctx, k.azucar)
		if err == nil && got.Valid {
			t.Fatalf("se ve la llegada de una compra de otra empresa: %v", got.Time)
		}
	})
}

func categoryIn(t *testing.T, st *store.Store) int64 {
	t.Helper()
	var id int64
	if err := st.Pool.QueryRow(context.Background(), `insert into expense_categories (company_id, name, financial_group)
		values ($1, 'Compras ' || gen_random_uuid()::text, 'operacional') returning id`, defaultCompanyID).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}
