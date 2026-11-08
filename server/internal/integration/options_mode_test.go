//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"
	"time"
	"uuid"

	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
)

// «LO RESTAN SUS OPCIONES» (spec 069, issue #252): un producto cuyas opciones ya restan deja de
// contar su propia existencia sin darle insumos, y al venderlo solo baja lo de la opción.

// pieceRecipe es una receta de N piezas de un insumo por pieza.
func pieceRecipe(t *testing.T, st *store.Store, ing int64, qty string) int64 {
	t.Helper()
	ctx := context.Background()
	var id int64
	if err := st.Pool.QueryRow(ctx, `insert into recipes default values returning id`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Pool.Exec(ctx, `insert into recipe_items (recipe_id, ingredient_id, quantity, unit_id)
		select $1, $2, $3::numeric, id from units where code = 'pieza'`, id, ing, qty); err != nil {
		t.Fatal(err)
	}
	return id
}

// flavorGroup liga al producto un grupo de opciones con min_select dado; cada opción resta 1 pieza
// de su insumo (nil = la opción no resta nada).
func flavorGroup(t *testing.T, st *store.Store, product int64, name string, minSelect int, options map[string]*int64) map[string]int64 {
	t.Helper()
	ctx := context.Background()
	var group int64
	if err := st.Pool.QueryRow(ctx, `insert into modifier_groups (company_id, name) values ($1, $2) returning id`,
		defaultCompanyID, name).Scan(&group); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Pool.Exec(ctx, `insert into product_modifier_groups (product_id, group_id, min_select, max_select)
		values ($1, $2, $3, 1)`, product, group, minSelect); err != nil {
		t.Fatal(err)
	}
	out := map[string]int64{}
	for optName, ing := range options {
		var recipe *int64
		if ing != nil {
			r := pieceRecipe(t, st, *ing, "1")
			recipe = &r
		}
		var id int64
		if err := st.Pool.QueryRow(ctx, `insert into modifier_options (company_id, group_id, name, recipe_id) values ($1, $2, $3, $4) returning id`,
			defaultCompanyID, group, optName, recipe).Scan(&id); err != nil {
			t.Fatal(err)
		}
		out[optName] = id
	}
	return out
}

type optionsKit struct {
	st           *store.Store
	admin        int64
	soda         int64 // existencia propia en 12, grupo obligatorio de sabores
	fresa, limon int64 // artículos por pieza
	opts         map[string]int64
	svc          *app.AdminService
	ctx          context.Context
}

func newOptionsKit(t *testing.T) *optionsKit {
	t.Helper()
	st := newTestStore(t)
	k := &optionsKit{st: st}
	k.admin = makeUser(t, st, "admin_opciones", "admin")
	k.soda = makeProduct(t, st, "Refresco de lata opciones", dd("25"), true)
	adjust(t, st, "producto", k.soda, "12")
	k.fresa = pieceIngredient(t, st, "Lata fresa opciones")
	k.limon = pieceIngredient(t, st, "Lata limón opciones")
	adjust(t, st, "ingrediente", k.fresa, "10")
	adjust(t, st, "ingrediente", k.limon, "10")
	k.opts = flavorGroup(t, st, k.soda, "Sabor opciones", 1, map[string]*int64{"Fresa": &k.fresa, "Limón": &k.limon})
	k.svc, k.ctx = asApp(t, defaultCompanyID)
	return k
}

// El criterio de aceptación del issue #252.
func TestAProductWhoseFlavorsDeductSellsOnlyTheFlavorArticle(t *testing.T) {
	t.Parallel()
	k := newOptionsKit(t)
	if err := k.svc.SaveComposition(k.ctx, app.CompositionOfProduct, k.soda, app.CompositionRequest{Mode: "options"}, k.admin); err != nil {
		t.Fatalf("marcar «Lo restan sus opciones» sin insumos: %v", err)
	}
	if trackStock(t, k.st, k.soda) {
		t.Fatal("el producto sigue contando su propia existencia")
	}
	cashier := makeUser(t, k.st, "cajero_opciones", "cajero")
	abrirCajaPrincipal(t, k.st, cashier)
	orders := app.NewOrdersService(k.st, clock)
	order, err := orders.Create(context.Background(), app.CreateOrderCmd{ClientUUID: uuid.New(), ServiceType: "mostrador", OpenedBy: cashier,
		Lines: []domain.OrderLineInput{{ProductID: k.soda, Qty: dd("1"), Modifiers: []domain.OrderModInput{{OptionID: k.opts["Fresa"], Qty: 1}}}}})
	if err != nil {
		t.Fatalf("vender con Fresa: %v", err)
	}
	type mov struct {
		itemType string
		item     int64
		qty      decimal.Decimal
	}
	rows, err := k.st.Pool.Query(context.Background(), `select item_type::text, coalesce(product_id, ingredient_id), quantity
		from stock_movements where order_id = $1 and movement_type = 'venta'`, order.ID)
	if err != nil {
		t.Fatal(err)
	}
	var got []mov
	for rows.Next() {
		var m mov
		if err := rows.Scan(&m.itemType, &m.item, &m.qty); err != nil {
			t.Fatal(err)
		}
		got = append(got, m)
	}
	rows.Close()
	if len(got) != 1 || got[0].itemType != "ingrediente" || got[0].item != k.fresa || !got[0].qty.Equal(dd("-1")) {
		t.Fatalf("movimientos de venta = %+v; quiere uno solo: −1 del artículo Fresa", got)
	}
	if got := onHand(t, k.st, k.soda); !got.IsZero() {
		t.Errorf("el refresco quedó en %s; su contador debió cerrarse en 0 y no volver a moverse", got)
	}
}

func TestOptionsModeClosesTheOwnCounterWithItsAudit(t *testing.T) {
	t.Parallel()
	k := newOptionsKit(t)
	north, _ := addBranch(t, k.st, defaultCompanyID, "NORTE")
	if _, err := k.st.Pool.Exec(context.Background(), `insert into stock_movements (item_type, product_id, movement_type, quantity, branch_id)
		values ('producto', $1, 'ajuste', 5, $2)`, k.soda, north); err != nil {
		t.Fatal(err)
	}
	before := modeEvents(t, k.st, k.soda)
	if err := k.svc.SaveComposition(k.ctx, app.CompositionOfProduct, k.soda, app.CompositionRequest{Mode: "options"}, k.admin); err != nil {
		t.Fatal(err)
	}
	var branches int
	if err := k.st.Pool.QueryRow(context.Background(), `select count(*) from stock_levels where product_id = $1 and on_hand <> 0`, k.soda).Scan(&branches); err != nil || branches != 0 {
		t.Fatalf("sucursales con existencia del producto = %d (%v); quiere 0 en las dos", branches, err)
	}
	var moves int
	if err := k.st.Pool.QueryRow(context.Background(), `select count(*) from stock_movements where product_id = $1 and reason = 'cambio de modo'`, k.soda).Scan(&moves); err != nil || moves != 2 {
		t.Errorf("movimientos de cambio de modo = %d (%v); quiere uno por sucursal", moves, err)
	}
	if modeEvents(t, k.st, k.soda) != before+1 {
		t.Error("el cambio no quedó en la bitácora")
	}
	var after string
	if err := k.st.Pool.QueryRow(context.Background(), `select after->>'mode' from audit_events
		where action = 'product.consumption_mode' and entity_id = $1 order by id desc limit 1`, k.soda).Scan(&after); err != nil || after != "options" {
		t.Errorf("la bitácora dice %q (%v); quiere options", after, err)
	}
}

func TestOptionsModeIsRejectedWhenNoOptionDeducts(t *testing.T) {
	t.Parallel()
	k := newOptionsKit(t)
	ctx := context.Background()
	plain := makeProduct(t, k.st, "Sin opciones que resten", dd("20"), true)
	flavorGroup(t, k.st, plain, "Hielo opciones", 1, map[string]*int64{"Sin hielo": nil})
	inactive := makeProduct(t, k.st, "Opción apagada", dd("20"), true)
	io := flavorGroup(t, k.st, inactive, "Sabor apagado", 1, map[string]*int64{"Fresa": &k.fresa})
	if _, err := k.st.Pool.Exec(ctx, `update modifier_options set is_active = false where id = $1`, io["Fresa"]); err != nil {
		t.Fatal(err)
	}
	unlinked := makeProduct(t, k.st, "Grupo apagado", dd("20"), true)
	flavorGroup(t, k.st, unlinked, "Sabor de grupo apagado", 1, map[string]*int64{"Fresa": &k.fresa})
	if _, err := k.st.Pool.Exec(ctx, `update modifier_groups set is_active = false where name = 'Sabor de grupo apagado'`); err != nil {
		t.Fatal(err)
	}
	nothingLinked := makeProduct(t, k.st, "Ligado a uno que no resta", dd("20"), true)
	nada := makeProduct(t, k.st, "Producto que no resta", dd("5"), false)
	nl := flavorGroup(t, k.st, nothingLinked, "Ligados opciones", 1, map[string]*int64{"Otro": nil})
	if _, err := k.st.Pool.Exec(ctx, `update modifier_options set linked_product_id = $1 where id = $2`, nada, nl["Otro"]); err != nil {
		t.Fatal(err)
	}
	for name, id := range map[string]int64{"sin opciones que resten": plain, "opción apagada": inactive, "grupo apagado": unlinked, "ligada a uno que no resta": nothingLinked} {
		err := k.svc.SaveComposition(k.ctx, app.CompositionOfProduct, id, app.CompositionRequest{Mode: "options"}, k.admin)
		if !errors.Is(err, domain.ErrValidation) {
			t.Errorf("%s: = %v; quiere ErrValidation", name, err)
		}
		if !trackStock(t, k.st, id) {
			t.Errorf("%s: se le apagó el contador aunque se rechazó", name)
		}
	}
	ing := pieceIngredient(t, k.st, "Insumo con opciones")
	if err := k.svc.SaveComposition(k.ctx, app.CompositionOfProduct, k.soda,
		app.CompositionRequest{Mode: "options", Items: pieces(t, ing, "1", k.st)}, k.admin); !errors.Is(err, domain.ErrValidation) {
		t.Errorf("«Lo restan sus opciones» con insumos = %v; quiere ErrValidation", err)
	}

	// Un producto sin existencia propia también puede quedar así: sin movimientos.
	noOwn := makeProduct(t, k.st, "Sin existencia propia opciones", dd("30"), false)
	flavorGroup(t, k.st, noOwn, "Sabor sin existencia", 1, map[string]*int64{"Fresa": &k.fresa})
	if err := k.svc.SaveComposition(k.ctx, app.CompositionOfProduct, noOwn, app.CompositionRequest{Mode: "options"}, k.admin); err != nil {
		t.Fatal(err)
	}
	var moves int
	if err := k.st.Pool.QueryRow(ctx, `select count(*) from stock_movements where product_id = $1`, noOwn).Scan(&moves); err != nil || moves != 0 {
		t.Errorf("movimientos = %d (%v); quiere 0", moves, err)
	}
}

func TestCompositionAndRecipesTellTheOptionsMode(t *testing.T) {
	t.Parallel()
	k := newOptionsKit(t)
	optional := makeProduct(t, k.st, "Con extra opcional", dd("30"), false)
	flavorGroup(t, k.st, optional, "Extra opcional", 0, map[string]*int64{"Fresa": &k.fresa})

	v, err := k.svc.Composition(k.ctx, app.CompositionOfProduct, k.soda)
	if err != nil {
		t.Fatal(err)
	}
	if !v.OptionsDeduct || !v.OptionsRequired || len(v.OptionGroups) != 1 || len(v.OptionGroups[0].Options) != 2 {
		t.Fatalf("composición = deduct %v, required %v, grupos %+v; quiere que sus sabores restan, obligatorio y dos opciones",
			v.OptionsDeduct, v.OptionsRequired, v.OptionGroups)
	}
	for _, o := range v.OptionGroups[0].Options {
		if o.Deducts == "" {
			t.Errorf("la opción %s no dice qué resta", o.Name)
		}
	}
	ov, err := k.svc.Composition(k.ctx, app.CompositionOfProduct, optional)
	if err != nil {
		t.Fatal(err)
	}
	if !ov.OptionsDeduct || ov.OptionsRequired {
		t.Errorf("con un grupo opcional: deduct %v, required %v; quiere true y false (para avisar)", ov.OptionsDeduct, ov.OptionsRequired)
	}

	if err := k.svc.SaveComposition(k.ctx, app.CompositionOfProduct, k.soda, app.CompositionRequest{Mode: "options"}, k.admin); err != nil {
		t.Fatal(err)
	}
	page, err := k.svc.ListRecipes(k.ctx, domain.RecipeFilter{Kind: domain.RecipeKindProduct, Status: domain.RecipeStatusDone,
		Sort: domain.RecipeSortAZ, Query: "Refresco de lata opciones", Limit: 20}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Items[0].Mode != "options" {
		t.Fatalf("Recetas = %+v; quiere el refresco con mode options", page.Items)
	}
}

func TestOptionsModeIsIsolated(t *testing.T) {
	t.Parallel()
	k := newOptionsKit(t)
	other := makeCompany(t, k.st, "opciones-otra")
	inTheThreeCases(t, defaultCompanyID, other, func(t *testing.T, as *store.Store, ctx context.Context) {
		svc := app.NewAdminService(as)
		if v, err := svc.Composition(ctx, app.CompositionOfProduct, k.soda); err == nil && (v.OptionsDeduct || len(v.OptionGroups) > 0) {
			t.Fatal("se ven las opciones de un producto de otra empresa")
		}
		if err := svc.SaveComposition(ctx, app.CompositionOfProduct, k.soda, app.CompositionRequest{Mode: "options"}, k.admin); err == nil {
			t.Fatal("cambió el modo de un producto de otra empresa")
		}
	})
	if !trackStock(t, k.st, k.soda) {
		t.Error("otra empresa le apagó el contador al producto")
	}
}

// «Una opción resta» en SQL (option_deducts, 0110) tiene que decir lo mismo que la venta real
// (domain.ExpandSale): si no, la hoja ofrecería «Lo restan sus opciones» para un producto que al
// venderse no baja nada. Lo encontró la revisión de la 0110: un paquete ligado contaba como «resta»
// aunque sus piezas por omisión no restaran nada. Se vende cada opción y se compara.
func TestOptionDeductsAgreesWithWhatASaleDeducts(t *testing.T) {
	t.Parallel()
	k := newOptionsKit(t)
	ctx := context.Background()
	st := k.st
	ownStock := makeProduct(t, st, "Ligado existencia propia", dd("10"), true)
	nothing := makeProduct(t, st, "Ligado que no resta", dd("10"), false)
	withRecipe := makeProduct(t, st, "Ligado con receta", dd("10"), false)
	if _, err := st.Pool.Exec(ctx, `update products set recipe_id = $1 where id = $2`, pieceRecipe(t, st, k.fresa, "1"), withRecipe); err != nil {
		t.Fatal(err)
	}
	combo := func(name string, def *int64) int64 {
		c := makeProduct(t, st, name, dd("50"), false)
		if _, err := st.Pool.Exec(ctx, `update products set type = 'combo' where id = $1`, c); err != nil {
			t.Fatal(err)
		}
		if def != nil {
			if _, err := st.Pool.Exec(ctx, `
				with s as (insert into combo_slots (combo_id, name, min_select, max_select) values ($1, 'hueco', 1, 1) returning id)
				insert into combo_slot_products (slot_id, product_id, is_default) select id, $2, true from s`, c, *def); err != nil {
				t.Fatal(err)
			}
		}
		return c
	}
	comboThatDeducts := combo("Paquete que resta", &ownStock)
	comboThatDoesNot := combo("Paquete que no resta", &nothing)
	comboEmpty := combo("Paquete sin piezas", nil)

	host := makeProduct(t, st, "Producto anfitrión de opciones", dd("30"), false)
	opts := flavorGroup(t, st, host, "Paridad opciones", 0, map[string]*int64{"Con receta": &k.fresa, "Sin nada": nil,
		"Receta vacía": nil, "Existencia propia": nil, "No resta": nil, "Con receta ligada": nil,
		"Paquete que resta": nil, "Paquete que no resta": nil, "Paquete vacío": nil})
	if _, err := st.Pool.Exec(ctx, `insert into recipes default values`); err != nil {
		t.Fatal(err)
	}
	links := map[string]int64{"Existencia propia": ownStock, "No resta": nothing, "Con receta ligada": withRecipe,
		"Paquete que resta": comboThatDeducts, "Paquete que no resta": comboThatDoesNot, "Paquete vacío": comboEmpty}
	for name, p := range links {
		if _, err := st.Pool.Exec(ctx, `update modifier_options set linked_product_id = $1 where id = $2`, p, opts[name]); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := st.Pool.Exec(ctx, `update modifier_options set recipe_id = (select max(id) from recipes) where id = $1`, opts["Receta vacía"]); err != nil {
		t.Fatal(err)
	}

	cashier := makeUser(t, st, "cajero_paridad", "cajero")
	abrirCajaPrincipal(t, st, cashier)
	orders := app.NewOrdersService(st, clock)
	for name, opt := range opts {
		order, err := orders.Create(ctx, app.CreateOrderCmd{ClientUUID: uuid.New(), ServiceType: "mostrador", OpenedBy: cashier,
			Lines: []domain.OrderLineInput{{ProductID: host, Qty: dd("1"), Modifiers: []domain.OrderModInput{{OptionID: opt, Qty: 1}}}}})
		if err != nil {
			t.Fatalf("%s: vender: %v", name, err)
		}
		var sold, says bool
		if err := st.Pool.QueryRow(ctx, `select exists (select 1 from stock_movements where order_id = $1 and modifier_option_id = $2 and movement_type = 'venta'),
		    option_deducts($2)`, order.ID, opt).Scan(&sold, &says); err != nil {
			t.Fatal(err)
		}
		if sold != says {
			t.Errorf("%s: la venta restó %v y option_deducts dice %v", name, sold, says)
		}
	}
}
