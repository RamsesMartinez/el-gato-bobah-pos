//go:build integration

package integration

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
	"uuid"

	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
)

// QUÉ RESTA CADA PRODUCTO (spec 037, historia 3): salir de la existencia propia sin perder ni
// inventar latas, con bitácora, y nada de esto alcanza a otra empresa.

func pieceIngredient(t *testing.T, st *store.Store, name string) int64 {
	t.Helper()
	var id int64
	if err := st.Pool.QueryRow(context.Background(), `
		insert into ingredients (name, base_unit_id) select $1, id from units where code = 'pieza' returning id`,
		name).Scan(&id); err != nil {
		t.Fatalf("insumo %s: %v", name, err)
	}
	return id
}

func adjust(t *testing.T, st *store.Store, itemType string, id int64, qty string) {
	t.Helper()
	col := "product_id"
	if itemType == "ingrediente" {
		col = "ingredient_id"
	}
	if _, err := st.Pool.Exec(context.Background(),
		"insert into stock_movements (item_type, "+col+", movement_type, quantity) values ($1::stock_item_type, $2, 'ajuste', $3::numeric)",
		itemType, id, qty); err != nil {
		t.Fatal(err)
	}
}

func pieces(t *testing.T, ing int64, qty string, st *store.Store) []app.CompositionInputItem {
	return []app.CompositionInputItem{{IngredientID: ing, Quantity: decimal.RequireFromString(qty), UnitID: unitID(t, st, "pieza")}}
}

func trackStock(t *testing.T, st *store.Store, product int64) bool {
	t.Helper()
	var v bool
	if err := st.Pool.QueryRow(context.Background(), `select track_stock from products where id = $1`, product).Scan(&v); err != nil {
		t.Fatal(err)
	}
	return v
}

func modeEvents(t *testing.T, st *store.Store, product int64) int {
	t.Helper()
	var n int
	if err := st.Pool.QueryRow(context.Background(), `
		select count(*) from audit_events where action = 'product.consumption_mode' and entity_id = $1`, product).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func asApp(t *testing.T, company int64) (*app.AdminService, context.Context) {
	t.Helper()
	as := appRoleStore(t)
	ctx, release, err := as.AcquireTenant(context.Background(), company)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(release)
	return app.NewAdminService(as), ctx
}

func TestLeavingOwnStockMovesTheStockOnce(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	admin := makeUser(t, st, "admin_modo", "admin")
	svc, ctx := asApp(t, defaultCompanyID)

	t.Run("12 latas pasan al insumo nuevo, con su nota y su evento", func(t *testing.T) {
		lata := makeProduct(t, st, "Lata modo", decimal.RequireFromString("30"), true)
		adjust(t, st, "producto", lata, "12")
		ing := pieceIngredient(t, st, "Lata modo insumo")
		if err := svc.SaveComposition(ctx, app.CompositionOfProduct, lata,
			app.CompositionRequest{Mode: "items", Items: pieces(t, ing, "1", st)}, admin); err != nil {
			t.Fatal(err)
		}
		if got := onHand(t, st, lata); !got.IsZero() {
			t.Errorf("el producto quedó en %s, quiere 0", got)
		}
		if got := onHandIngredient(t, st, ing); !got.Equal(decimal.NewFromInt(12)) {
			t.Errorf("el insumo quedó en %s, quiere 12", got)
		}
		if trackStock(t, st, lata) {
			t.Error("el producto sigue contando su propia existencia")
		}
		var notes int
		if err := st.Pool.QueryRow(ctx, `
			select count(distinct note) from stock_movements
			 where movement_type = 'ajuste' and reason = 'cambio de modo' and (product_id = $1 or ingredient_id = $2)`, lata, ing).Scan(&notes); err != nil {
			t.Fatal(err)
		}
		if notes != 1 {
			t.Errorf("los dos ajustes van ligados por la misma nota: hay %d notas distintas", notes)
		}
		if modeEvents(t, st, lata) != 1 {
			t.Errorf("el cambio de modo deja un evento en la bitácora: hay %d", modeEvents(t, st, lata))
		}
	})

	t.Run("Menú › Recetas dice cuánto resta por venta y cuánto hay del insumo", func(t *testing.T) {
		lata := makeProduct(t, st, "Lata lista modo", decimal.RequireFromString("30"), true)
		adjust(t, st, "producto", lata, "7")
		ing := pieceIngredient(t, st, "Lata lista modo insumo")
		if err := svc.SaveComposition(ctx, app.CompositionOfProduct, lata,
			app.CompositionRequest{Mode: "items", Items: pieces(t, ing, "1", st)}, admin); err != nil {
			t.Fatal(err)
		}
		page, err := svc.ListRecipes(ctx, domain.RecipeFilter{Kind: domain.RecipeKindProduct, Status: domain.RecipeStatusDone,
			Sort: domain.RecipeSortAZ, Query: "Lata lista modo", Limit: 25}, fixedNow)
		if err != nil {
			t.Fatal(err)
		}
		var row *app.RecipeRow
		for i := range page.Items {
			if page.Items[i].ID == lata {
				row = &page.Items[i]
			}
		}
		if row == nil || row.PerSale == nil || row.OnHand == nil {
			t.Fatalf("la receta de un insumo trae cuánto resta y cuánto hay: %+v", row)
		}
		if !row.PerSale.Equal(decimal.NewFromInt(1)) || !row.OnHand.Equal(decimal.NewFromInt(7)) {
			t.Errorf("resta %s por venta y hay %s, quiere 1 y 7", row.PerSale, row.OnHand)
		}
	})

	t.Run("un paquete con piezas que se compran hechas resta cada insumo al venderse", func(t *testing.T) {
		cajero := makeUser(t, st, "cajero_modo", "cajero")
		abrirCajaPrincipal(t, st, cajero)
		a := makeProduct(t, st, "Mochi modo", decimal.RequireFromString("20"), false)
		b := makeProduct(t, st, "Soda modo", decimal.RequireFromString("20"), false)
		ia, ib := pieceIngredient(t, st, "Mochi modo insumo"), pieceIngredient(t, st, "Soda modo insumo")
		for p, i := range map[int64]int64{a: ia, b: ib} {
			if err := svc.SaveComposition(ctx, app.CompositionOfProduct, p, app.CompositionRequest{Mode: "items", Items: pieces(t, i, "1", st)}, admin); err != nil {
				t.Fatal(err)
			}
		}
		combo := makeProduct(t, st, "Paquete modo", decimal.RequireFromString("35"), false)
		if err := svc.SaveComposition(ctx, app.CompositionOfProduct, combo, app.CompositionRequest{Mode: "package",
			Components: []app.CompositionComponent{{ProductID: a, Quantity: 2}, {ProductID: b, Quantity: 1}}}, admin); err != nil {
			t.Fatal(err)
		}
		k := &healthKit{st: st, cashier: cajero, orders: app.NewOrdersService(st, clock)}
		k.sell(t, line(combo, "1"))
		if got := onHandIngredient(t, st, ia); !got.Equal(decimal.NewFromInt(-2)) {
			t.Errorf("el paquete lleva 2 mochis: el insumo quedó en %s", got)
		}
		if got := onHandIngredient(t, st, ib); !got.Equal(decimal.NewFromInt(-1)) {
			t.Errorf("el paquete lleva 1 soda: el insumo quedó en %s", got)
		}
	})

	t.Run("pasar una receta a «No se cuenta» la deja guardada y deja su evento", func(t *testing.T) {
		p := makeProduct(t, st, "Agua caliente modo", decimal.RequireFromString("10"), false)
		ing := pieceIngredient(t, st, "Vaso modo")
		if err := svc.SaveComposition(ctx, app.CompositionOfProduct, p, app.CompositionRequest{Mode: "items", Items: pieces(t, ing, "1", st)}, admin); err != nil {
			t.Fatal(err)
		}
		var recipe int64
		if err := st.Pool.QueryRow(ctx, `select recipe_id from products where id = $1`, p).Scan(&recipe); err != nil {
			t.Fatal(err)
		}
		before := modeEvents(t, st, p)
		if err := svc.SaveComposition(ctx, app.CompositionOfProduct, p, app.CompositionRequest{Mode: "nothing"}, admin); err != nil {
			t.Fatal(err)
		}
		var exists bool
		if err := st.Pool.QueryRow(ctx, `select exists (select 1 from recipe_items where recipe_id = $1)`, recipe).Scan(&exists); err != nil {
			t.Fatal(err)
		}
		if !exists {
			t.Error("la receta anterior se borró: el historial de qué restaba se pierde")
		}
		var status string
		if err := st.Pool.QueryRow(ctx, `select coalesce(composition_status, '') from products where id = $1`, p).Scan(&status); err != nil {
			t.Fatal(err)
		}
		if status != "confirmed" {
			t.Errorf("«No se cuenta» queda confirmado de una vez: estado %q", status)
		}
		if modeEvents(t, st, p) != before+1 {
			t.Error("pasar de receta a «No se cuenta» no dejó evento")
		}
	})

	t.Run("«Se prepara aquí» o «Es un combo» sin contenido no se guardan", func(t *testing.T) {
		p := makeProduct(t, st, "Vacío modo", decimal.RequireFromString("10"), false)
		for _, mode := range []string{"items", "package"} {
			err := svc.SaveComposition(ctx, app.CompositionOfProduct, p, app.CompositionRequest{Mode: mode}, admin)
			if !errors.Is(err, domain.ErrValidation) {
				t.Errorf("modo %q vacío = %v, quiere ErrValidation", mode, err)
			}
		}
		if err := svc.SaveComposition(ctx, app.CompositionOfProduct, p, app.CompositionRequest{Mode: "todo"}, admin); !errors.Is(err, domain.ErrValidation) {
			t.Errorf("un modo inventado se rechaza: %v", err)
		}
	})

	t.Run("el insumo ya tenía existencia: queda la cifra elegida", func(t *testing.T) {
		lata := makeProduct(t, st, "Coca modo", decimal.RequireFromString("30"), true)
		adjust(t, st, "producto", lata, "12")
		ing := pieceIngredient(t, st, "Coca modo insumo")
		adjust(t, st, "ingrediente", ing, "20")
		err := svc.SaveComposition(ctx, app.CompositionOfProduct, lata, app.CompositionRequest{Mode: "items", Items: pieces(t, ing, "1", st)}, admin)
		if !errors.Is(err, domain.ErrValidation) {
			t.Fatalf("sin decir cuántas hay de verdad = %v, quiere ErrValidation", err)
		}
		d := func(s string) *decimal.Decimal { v := decimal.RequireFromString(s); return &v }
		if err := svc.SaveComposition(ctx, app.CompositionOfProduct, lata, app.CompositionRequest{Mode: "items", Items: pieces(t, ing, "1", st),
			ActualStock: d("25"), SeenProductOnHand: d("12"), SeenIngredientOnHand: d("20")}, admin); err != nil {
			t.Fatal(err)
		}
		if got := onHandIngredient(t, st, ing); !got.Equal(decimal.NewFromInt(25)) {
			t.Errorf("el insumo quedó en %s, quiere 25 (la cifra elegida, no 32)", got)
		}
		if got := onHand(t, st, lata); !got.IsZero() {
			t.Errorf("el producto quedó en %s, quiere 0", got)
		}
	})

	inTheThreeCases(t, defaultCompanyID, makeCompany(t, st, "otra-modo"), func(t *testing.T, as *store.Store, c context.Context) {
		lata := makeProduct(t, st, "Lata ajena "+t.Name(), decimal.RequireFromString("30"), true)
		adjust(t, st, "producto", lata, "5")
		ing := pieceIngredient(t, st, "Insumo ajeno "+t.Name())
		_ = app.NewAdminService(as).SaveComposition(c, app.CompositionOfProduct, lata,
			app.CompositionRequest{Mode: "items", Items: pieces(t, ing, "1", st)}, admin)
		if !trackStock(t, st, lata) || !onHand(t, st, lata).Equal(decimal.NewFromInt(5)) {
			t.Fatal("una sesión de otra empresa (o sin empresa) movió la existencia de la dueña")
		}
	})
}

// Una venta entre mostrar las cifras y guardar: lo que se contestó fue sobre números viejos.
func TestModeChangeRejectsStaleFigures(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	admin := makeUser(t, st, "admin_modo_viejo", "admin")
	cajero := makeUser(t, st, "cajero_modo_viejo", "cajero")
	abrirCajaPrincipal(t, st, cajero)
	svc, ctx := asApp(t, defaultCompanyID)
	lata := makeProduct(t, st, "Lata vieja", decimal.RequireFromString("30"), true)
	adjust(t, st, "producto", lata, "12")
	ing := pieceIngredient(t, st, "Lata vieja insumo")
	adjust(t, st, "ingrediente", ing, "20")

	// La pantalla mostró 12 y 20; antes de guardar se vende una.
	k := &healthKit{st: st, cashier: cajero, orders: app.NewOrdersService(st, clock)}
	k.sell(t, line(lata, "1"))
	d := func(s string) *decimal.Decimal { v := decimal.RequireFromString(s); return &v }
	err := svc.SaveComposition(ctx, app.CompositionOfProduct, lata, app.CompositionRequest{Mode: "items", Items: pieces(t, ing, "1", st),
		ActualStock: d("32"), SeenProductOnHand: d("12"), SeenIngredientOnHand: d("20")}, admin)
	if !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("guardar sobre cifras viejas = %v, quiere ErrConflict", err)
	}
	if got := onHand(t, st, lata); !got.Equal(decimal.NewFromInt(11)) {
		t.Errorf("el rechazo movió la existencia del producto: %s, quiere 11", got)
	}
	if got := onHandIngredient(t, st, ing); !got.Equal(decimal.NewFromInt(20)) {
		t.Errorf("el rechazo inventó existencia en el insumo: %s, quiere 20", got)
	}
	if !trackStock(t, st, lata) {
		t.Error("el rechazo apagó la existencia propia")
	}

	// Ventas en paralelo con la conversión: el producto nunca queda distinto de cero ni el insumo con
	// existencia inventada. Lo que la venta resta, o lo resta el producto antes, o el insumo después.
	lata2 := makeProduct(t, st, "Lata paralela", decimal.RequireFromString("30"), true)
	adjust(t, st, "producto", lata2, "50")
	ing2 := pieceIngredient(t, st, "Lata paralela insumo")
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = k.orders.Create(context.Background(), app.CreateOrderCmd{
				ClientUUID: uuid.New(), ServiceType: "mostrador", OpenedBy: cajero, Lines: []domain.OrderLineInput{line(lata2, "1")},
			})
		}()
	}
	if err := svc.SaveComposition(ctx, app.CompositionOfProduct, lata2, app.CompositionRequest{Mode: "items", Items: pieces(t, ing2, "1", st)}, admin); err != nil {
		t.Fatal(err)
	}
	wg.Wait()
	total := onHand(t, st, lata2).Add(onHandIngredient(t, st, ing2))
	var sold decimal.Decimal
	if err := st.Pool.QueryRow(ctx, `
		select coalesce(sum(-quantity), 0) from stock_movements where movement_type = 'venta' and (product_id = $1 or ingredient_id = $2)`,
		lata2, ing2).Scan(&sold); err != nil {
		t.Fatal(err)
	}
	if !total.Add(sold).Equal(decimal.NewFromInt(50)) {
		t.Errorf("50 latas, %s vendidas: entre producto e insumo quedan %s; se perdieron o se inventaron", sold, total)
	}
}

// El hermano que no se movió: lo que no pasa por la hoja de receta sigue igual.
func TestOwnStockThatNobodyTouchedStillWorks(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	cajero := makeUser(t, st, "cajero_hermano", "cajero")
	abrirCajaPrincipal(t, st, cajero)
	lata := makeProduct(t, st, "Lata intacta", decimal.RequireFromString("30"), true)
	adjust(t, st, "producto", lata, "10")
	k := &healthKit{st: st, cashier: cajero, orders: app.NewOrdersService(st, clock)}
	k.sell(t, line(lata, "2"))
	if got := onHand(t, st, lata); !got.Equal(decimal.NewFromInt(8)) {
		t.Errorf("la existencia propia heredada dejó de restar: %s, quiere 8", got)
	}
	copia, err := app.NewAdminService(st).DuplicateProduct(context.Background(), lata, "Lata intacta copia", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !trackStock(t, st, copia) || !onHand(t, st, lata).Equal(decimal.NewFromInt(8)) {
		t.Error("duplicar un producto con existencia propia la cambió o movió existencias")
	}
}

// Nombres de insumo sin duplicar, sin distinguir mayúsculas ni acentos (caso borde «lo que entra en
// otro orden»): dos cuentas de las mismas latas se contarían dos veces.
func TestIngredientNamesAreUniqueIgnoringCaseAndAccents(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()
	pieceIngredient(t, st, "Lata Coca")
	bo := app.NewBackofficeService(st, clock)
	pieza := unitID(t, st, "pieza")
	for _, name := range []string{"lata coca", "LATA COCA", "Láta Cóca", "  Lata   Coca "} {
		if _, err := bo.CreateIngredient(ctx, app.IngredientInput{Name: name, BaseUnitID: pieza}); !errors.Is(err, domain.ErrConflict) {
			t.Errorf("alta de %q = %v, quiere ErrConflict", name, err)
		}
	}
	if _, err := bo.CreateIngredient(ctx, app.IngredientInput{Name: "Lata Coca Light", BaseUnitID: pieza}); err != nil {
		t.Errorf("otro nombre sí se da de alta: %v", err)
	}

	// Dos altas casi simultáneas que solo cambian mayúsculas: exactamente una gana.
	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i, name := range []string{"Jarabe paralelo", "jarabe paralelo"} {
		wg.Add(1)
		go func(i int, name string) {
			defer wg.Done()
			_, errs[i] = bo.CreateIngredient(ctx, app.IngredientInput{Name: name, BaseUnitID: pieza})
		}(i, name)
	}
	wg.Wait()
	ok := 0
	for _, e := range errs {
		if e == nil {
			ok++
		} else if !errors.Is(e, domain.ErrConflict) {
			t.Errorf("la que pierde responde %v, quiere ErrConflict", e)
		}
	}
	if ok != 1 {
		t.Errorf("de dos altas casi simultáneas ganaron %d, quiere exactamente 1", ok)
	}
}

// Convertir un producto mientras una venta tiene tomado el insumo destino (revisión de base de datos
// del 037): una venta bloquea en el orden de su ticket, así que «insumo y luego producto» contra
// «producto y luego insumo» era un bloqueo mutuo, y Postgres cancelaba una de las dos —a veces el
// cobro—. La conversión ahora no espera por el insumo: si está tomado, responde 409 y la venta sigue.
func TestModeChangeNeverDeadlocksWithASale(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	admin := makeUser(t, st, "admin_bloqueo", "admin")
	svc, ctx := asApp(t, defaultCompanyID)
	lata := makeProduct(t, st, "Lata bloqueo", decimal.RequireFromString("30"), true)
	adjust(t, st, "producto", lata, "12")
	ing := pieceIngredient(t, st, "Lata bloqueo insumo")
	adjust(t, st, "ingrediente", ing, "4")

	// Una «venta» a medias: tiene tomada la existencia del insumo y no ha terminado.
	tx, err := st.Pool.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	if _, err := tx.Exec(context.Background(), `select 1 from stock_levels where ingredient_id = $1 for update`, ing); err != nil {
		t.Fatal(err)
	}

	done := make(chan error, 1)
	go func() {
		done <- svc.SaveComposition(ctx, app.CompositionOfProduct, lata, app.CompositionRequest{Mode: "nothing"}, admin)
	}()
	// «No se cuenta» no toca el insumo: no tiene por qué esperar.
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("pasar a «No se cuenta» con el insumo tomado por otra venta: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("pasar a «No se cuenta» se quedó esperando a una venta que no le estorba")
	}

	lata2 := makeProduct(t, st, "Lata bloqueo dos", decimal.RequireFromString("30"), true)
	adjust(t, st, "producto", lata2, "12")
	go func() {
		done <- svc.SaveComposition(ctx, app.CompositionOfProduct, lata2, app.CompositionRequest{Mode: "items", Items: pieces(t, ing, "1", st),
			ActualStock:          func() *decimal.Decimal { v := decimal.RequireFromString("10"); return &v }(),
			SeenProductOnHand:    func() *decimal.Decimal { v := decimal.RequireFromString("12"); return &v }(),
			SeenIngredientOnHand: func() *decimal.Decimal { v := decimal.RequireFromString("4"); return &v }()}, admin)
	}()
	select {
	case err := <-done:
		if !errors.Is(err, domain.ErrConflict) {
			t.Fatalf("con el insumo tomado por una venta = %v, quiere ErrConflict de inmediato", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("la conversión esperó por el insumo: con una venta que espera por el producto, sería un bloqueo mutuo")
	}
	if !trackStock(t, st, lata2) || !onHand(t, st, lata2).Equal(decimal.NewFromInt(12)) {
		t.Error("el rechazo movió la existencia del producto")
	}
	_ = tx.Rollback(context.Background())
	// Ya sin la venta a medias, la misma conversión pasa.
	if err := svc.SaveComposition(ctx, app.CompositionOfProduct, lata2, app.CompositionRequest{Mode: "items", Items: pieces(t, ing, "1", st),
		ActualStock:          func() *decimal.Decimal { v := decimal.RequireFromString("10"); return &v }(),
		SeenProductOnHand:    func() *decimal.Decimal { v := decimal.RequireFromString("12"); return &v }(),
		SeenIngredientOnHand: func() *decimal.Decimal { v := decimal.RequireFromString("4"); return &v }()}, admin); err != nil {
		t.Fatalf("sin la venta a medias, la conversión pasa: %v", err)
	}
}

// Un guardado sin tarjeta (una pestaña vieja, un script) no puede dejar en cero la existencia propia
// sin que nadie eligiera «No se cuenta» (revisión de seguridad del 037).
func TestOwnStockNeedsAChosenCard(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	admin := makeUser(t, st, "admin_sin_tarjeta", "admin")
	svc, ctx := asApp(t, defaultCompanyID)
	lata := makeProduct(t, st, "Lata sin tarjeta", decimal.RequireFromString("30"), true)
	adjust(t, st, "producto", lata, "9")
	if err := svc.SaveComposition(ctx, app.CompositionOfProduct, lata, app.CompositionRequest{}, admin); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("guardar sin tarjeta un producto con existencia propia = %v, quiere ErrValidation", err)
	}
	if !trackStock(t, st, lata) || !onHand(t, st, lata).Equal(decimal.NewFromInt(9)) {
		t.Error("el guardado sin tarjeta movió la existencia")
	}
}

// Visto en CI (40P01): una venta toma el producto (la llave foránea del renglón) y DESPUÉS su
// existencia; guardar la receta bloqueaba la existencia y DESPUÉS el producto (cambiar recipe_id,
// que es único, choca con esa llave). Con la venta a la mitad, Postgres cancelaba una de las dos.
func TestSavingARecipeDuringASaleDoesNotDeadlock(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	admin := makeUser(t, st, "admin_sin_abrazo", "admin")
	svc, ctx := asApp(t, defaultCompanyID)
	lata := makeProduct(t, st, "Lata abrazo", decimal.RequireFromString("30"), true)
	adjust(t, st, "producto", lata, "10")
	ing := pieceIngredient(t, st, "Lata abrazo insumo")

	sale, err := st.Pool.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = sale.Rollback(context.Background()) }()
	if _, err := sale.Exec(context.Background(), `select 1 from products where id = $1 for key share`, lata); err != nil {
		t.Fatal(err)
	}

	saved := make(chan error, 1)
	go func() {
		saved <- svc.SaveComposition(ctx, app.CompositionOfProduct, lata, app.CompositionRequest{Mode: "items", Items: pieces(t, ing, "1", st)}, admin)
	}()
	deadline := time.Now().Add(5 * time.Second)
	for {
		var waiting int
		if err := st.Pool.QueryRow(context.Background(), `
			select count(*) from pg_stat_activity where datname = current_database() and wait_event_type = 'Lock'`).Scan(&waiting); err != nil {
			t.Fatal(err)
		}
		if waiting > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("guardar la receta nunca esperó a la venta")
		}
		time.Sleep(20 * time.Millisecond)
	}

	if _, err := sale.Exec(context.Background(), `
		insert into stock_movements (item_type, product_id, movement_type, quantity) values ('producto', $1, 'venta', -1)`, lata); err != nil {
		t.Fatalf("la venta a la mitad se canceló al guardar la receta: %v", err)
	}
	if err := sale.Commit(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := <-saved; err != nil {
		t.Fatalf("guardar la receta con una venta a la mitad = %v", err)
	}
	if got := onHand(t, st, lata).Add(onHandIngredient(t, st, ing)); !got.Equal(decimal.NewFromInt(9)) {
		t.Errorf("entre producto e insumo quedan %s, quiere 9", got)
	}
}
