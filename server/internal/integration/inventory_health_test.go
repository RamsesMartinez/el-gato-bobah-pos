//go:build integration

package integration

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"strings"
	"testing"
	"time"
	"uuid"

	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
)

// SALUD DEL INVENTARIO (spec 037): qué hace que las existencias no cuadren.

// julio es el periodo de las pruebas: el reloj de los tests vende el 20-jul-2026.
func julio() domain.HealthFilter {
	return domain.HealthFilter{
		Range: domain.Range{From: time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC), To: time.Date(2026, 7, 31, 0, 0, 0, 0, time.UTC)},
		Limit: domain.HealthPageSize,
	}
}

// healthKit es un negocio chico con cada forma en que una venta resta o no resta.
type healthKit struct {
	st                          *store.Store
	cashier                     int64
	shot, cafe, frappe, te      int64 // productos
	conReceta, sinReceta, hielo int64 // extras del frappé
	combo, compA, compB         int64
	milk, azucar                int64 // insumos
	orders                      *app.OrdersService
}

func setCost(t *testing.T, st *store.Store, table string, id int64, cost string) {
	t.Helper()
	if _, err := st.Pool.Exec(context.Background(), "update "+table+" set current_cost = $2 where id = $1", id, cost); err != nil {
		t.Fatal(err)
	}
}

func confirmNone(t *testing.T, st *store.Store, table string, id int64) {
	t.Helper()
	if _, err := st.Pool.Exec(context.Background(),
		"update "+table+" set composition_status = 'confirmed', composition_confirmed_at = now(), recipe_id = null where id = $1", id); err != nil {
		t.Fatal(err)
	}
}

func withRecipe(t *testing.T, st *store.Store, table string, id int64, items map[int64]string) {
	t.Helper()
	r := makeRecipe(t, st, items)
	if _, err := st.Pool.Exec(context.Background(),
		"update "+table+" set recipe_id = $2, composition_status = 'confirmed', composition_confirmed_at = now() where id = $1", id, r); err != nil {
		t.Fatal(err)
	}
}

func newHealthKit(t *testing.T) *healthKit {
	t.Helper()
	st := newTestStore(t)
	ctx := context.Background()
	k := &healthKit{st: st, orders: app.NewOrdersService(st, clock)}
	k.cashier = makeUser(t, st, "cajero_salud", "cajero")
	abrirCajaPrincipal(t, st, k.cashier)

	k.milk = makeIngredient(t, st, "Leche salud")
	k.azucar = makeIngredient(t, st, "Azúcar salud")

	// Shot: nadie ha dicho qué resta. Se vende y no resta nada.
	k.shot = makeProduct(t, st, "Shot salud", decimal.RequireFromString("30"), false)
	setCost(t, st, "products", k.shot, "12.50")
	// Café: tiene receta. Se vende y resta leche.
	k.cafe = makeProduct(t, st, "Café salud", decimal.RequireFromString("40"), false)
	withRecipe(t, st, "products", k.cafe, map[int64]string{k.milk: "200"})
	// Frappé: confirmado sin receta, porque resta por su sabor (el extra obligatorio).
	k.frappe = makeProduct(t, st, "Frappé salud", decimal.RequireFromString("60"), false)
	confirmNone(t, st, "products", k.frappe)
	k.conReceta = opcionConTope(t, st, "Sabor salud", "Sabor con receta", decimal.Zero, 1, k.frappe)
	withRecipe(t, st, "modifier_options", k.conReceta, map[int64]string{k.azucar: "20"})
	k.sinReceta = opcionConTope(t, st, "Sabor salud", "Sabor sin receta", decimal.Zero, 1, k.frappe)
	setCost(t, st, "modifier_options", k.sinReceta, "4")
	k.hielo = opcionConTope(t, st, "Sabor salud", "Sin hielo", decimal.Zero, 1, k.frappe)
	confirmNone(t, st, "modifier_options", k.hielo)
	// Té: confirmado «No gasta insumos» a propósito. No es un problema aunque no reste.
	k.te = makeProduct(t, st, "Té salud", decimal.RequireFromString("25"), false)
	confirmNone(t, st, "products", k.te)

	// Paquete de dos piezas: A resta, B no.
	k.compA = makeProduct(t, st, "Pieza A salud", decimal.RequireFromString("20"), false)
	withRecipe(t, st, "products", k.compA, map[int64]string{k.milk: "100"})
	k.compB = makeProduct(t, st, "Pieza B salud", decimal.RequireFromString("20"), false)
	k.combo = makeProduct(t, st, "Paquete salud", decimal.RequireFromString("35"), false)
	if _, err := st.Pool.Exec(ctx, `update products set type = 'combo', composition_status = 'confirmed', composition_confirmed_at = now() where id = $1`, k.combo); err != nil {
		t.Fatal(err)
	}
	for i, p := range []int64{k.compA, k.compB} {
		var slot int64
		if err := st.Pool.QueryRow(ctx, `insert into combo_slots (combo_id, name, position) values ($1, $2, $3) returning id`,
			k.combo, "Hueco "+itoa(i), i).Scan(&slot); err != nil {
			t.Fatal(err)
		}
		if _, err := st.Pool.Exec(ctx, `insert into combo_slot_products (slot_id, product_id, is_default) values ($1, $2, true)`, slot, p); err != nil {
			t.Fatal(err)
		}
	}
	return k
}

func (k *healthKit) sell(t *testing.T, lines ...domain.OrderLineInput) *app.OrderView {
	t.Helper()
	o, err := k.orders.Create(context.Background(), app.CreateOrderCmd{
		ClientUUID: uuid.New(), ServiceType: "mostrador", OpenedBy: k.cashier, Lines: lines,
	})
	if err != nil {
		t.Fatalf("vender: %v", err)
	}
	return o
}

func line(p int64, qty string, mods ...int64) domain.OrderLineInput {
	l := domain.OrderLineInput{ProductID: p, Qty: decimal.RequireFromString(qty)}
	for _, m := range mods {
		l.Modifiers = append(l.Modifiers, domain.OrderModInput{OptionID: m, Qty: 1})
	}
	return l
}

func reportAsApp(t *testing.T, st *store.Store, f domain.HealthFilter) app.InventoryHealthView {
	t.Helper()
	as := appRoleStore(t)
	ctx, release, err := as.AcquireTenant(context.Background(), defaultCompanyID)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	v, err := app.NewInventoryHealthService(as).Report(ctx, f)
	if err != nil {
		t.Fatalf("Report: %v", err)
	}
	return v
}

func unsoldOf(v app.InventoryHealthView, kind string, id int64) *app.HealthUnsoldItem {
	for i := range v.UnsoldDeduction.Items {
		if it := &v.UnsoldDeduction.Items[i]; it.Kind == kind && it.ID == id {
			return it
		}
	}
	return nil
}

// Lo que la venta de verdad dejó en el almacén decide, no la configuración: un producto cuyo sabor
// obligatorio resta no es problema, y el sabor que no resta sí.
func TestUnsoldDeductionIsMeasuredOnWhatTheSaleLeft(t *testing.T) {
	t.Parallel()
	k := newHealthKit(t)
	ctx := context.Background()

	k.sell(t, line(k.shot, "2"), line(k.cafe, "1"), line(k.te, "1"))
	k.sell(t, line(k.frappe, "1", k.conReceta), line(k.frappe, "3", k.sinReceta, k.hielo))
	k.sell(t, line(k.combo, "1"))
	// Un renglón cancelado y un pedido cancelado no son ventas.
	cancelado := k.sell(t, line(k.shot, "5"))
	if _, err := k.st.Pool.Exec(ctx, `update order_lines set cancelled_at = now() where order_id = $1`, cancelado.ID); err != nil {
		t.Fatal(err)
	}
	pedidoCancelado := k.sell(t, line(k.shot, "7"))
	if _, err := k.st.Pool.Exec(ctx, `update orders set status = 'cancelada', cancelled_at = now(), cancelled_by = $2, cancel_reason = 'prueba'
		 where id = $1`, pedidoCancelado.ID, k.cashier); err != nil {
		t.Fatal(err)
	}
	// Un pedido de plataforma con un platillo sin pareja: va en su propio contador.
	sinPareja := k.sell(t, line(k.shot, "1"))
	if _, err := k.st.Pool.Exec(ctx, `
		update order_lines set product_id = (select id from products where system_kind = 'platform_unpaired' and company_id = $2)
		 where order_id = $1`, sinPareja.ID, defaultCompanyID); err != nil {
		t.Fatal(err)
	}
	// Una venta por Uber: las piezas se separan por canal.
	porUber := k.sell(t, line(k.shot, "1"))
	if _, err := k.st.Pool.Exec(ctx, `
		update orders set service_type = 'domicilio', delivery_platform_id = (select id from delivery_platforms where name = 'Uber Eats' and company_id = $2)
		 where id = $1`, porUber.ID, defaultCompanyID); err != nil {
		t.Fatal(err)
	}

	v := reportAsApp(t, k.st, julio())

	shot := unsoldOf(v, "product", k.shot)
	if shot == nil {
		t.Fatalf("el Shot se vendió sin restar y no aparece: %+v", v.UnsoldDeduction.Items)
	}
	if !shot.Pieces.Equal(decimal.NewFromInt(3)) {
		t.Errorf("piezas del Shot = %s, quiere 3 (2 de mostrador y 1 de Uber; lo cancelado no cuenta)", shot.Pieces)
	}
	porCanal := map[string]string{}
	for _, u := range shot.Units {
		porCanal[u.Channel] = u.Units.String()
	}
	if porCanal["Mostrador"] != "2" || porCanal["Uber Eats"] != "1" || len(porCanal) != 2 {
		t.Errorf("piezas del Shot por canal = %v, quiere Mostrador 2 y Uber Eats 1", porCanal)
	}
	if shot.EstimatedCost == nil || !shot.EstimatedCost.Equal(decimal.RequireFromString("37.5")) {
		t.Errorf("costo estimado del Shot = %s, quiere 3 × 12.50 = 37.50", shot.EstimatedCost)
	}
	if shot.Status != string(domain.ModeUndecided) {
		t.Errorf("estado del Shot = %q, quiere %q", shot.Status, domain.ModeUndecided)
	}
	if !strings.Contains(shot.FixAt, "open=product:"+itoa(int(k.shot))) {
		t.Errorf("el Shot lleva a %q, quiere su receta", shot.FixAt)
	}

	if it := unsoldOf(v, "extra", k.sinReceta); it == nil || !it.Pieces.Equal(decimal.NewFromInt(3)) {
		t.Errorf("el sabor que no resta se vendió 3 veces y aparece así: %+v", it)
	}
	if it := unsoldOf(v, "product", k.compB); it == nil || !it.Pieces.Equal(decimal.NewFromInt(1)) {
		t.Errorf("la pieza B del paquete no resta y aparece así: %+v", it)
	}
	for name, it := range map[string]*app.HealthUnsoldItem{
		"el café (tiene receta)":                      unsoldOf(v, "product", k.cafe),
		"el frappé (resta por su sabor)":              unsoldOf(v, "product", k.frappe),
		"el té (confirmado «No gasta insumos»)":       unsoldOf(v, "product", k.te),
		"el sabor con receta":                         unsoldOf(v, "extra", k.conReceta),
		"«Sin hielo» (confirmado «No gasta insumos»)": unsoldOf(v, "extra", k.hielo),
		"la pieza A (resta)":                          unsoldOf(v, "product", k.compA),
		"el paquete mismo (se mide por sus piezas)":   unsoldOf(v, "product", k.combo),
	} {
		if it != nil {
			t.Errorf("%s aparece como vendido sin restar: %+v", name, it)
		}
	}
	if v.UnsoldDeduction.UnpairedPlatformLines != 1 {
		t.Errorf("renglones sin pareja = %d, quiere 1, aparte de los productos del catálogo", v.UnsoldDeduction.UnpairedPlatformLines)
	}
	if v.UnsoldDeduction.Total != len(v.UnsoldDeduction.Items) || v.UnsoldDeduction.Total != 3 {
		t.Errorf("total de «Se vende sin restar» = %d con %d renglones, quiere 3 (Shot, sabor sin receta, pieza B)",
			v.UnsoldDeduction.Total, len(v.UnsoldDeduction.Items))
	}
	if v.UnsoldDeduction.Items[0].ID != k.shot {
		t.Errorf("la lista va por costo estimado: el Shot (37.50) va primero, va %+v", v.UnsoldDeduction.Items[0])
	}
	if v.SalesInPeriod == 0 {
		t.Error("hubo ventas en el periodo y salesInPeriod dice 0")
	}

	// Un periodo sin ventas no es «todo sano»: no hay renglones, pero lo demás sigue.
	vacio := julio()
	vacio.Range = domain.Range{From: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), To: time.Date(2026, 1, 31, 0, 0, 0, 0, time.UTC)}
	if v := reportAsApp(t, k.st, vacio); v.SalesInPeriod != 0 || len(v.UnsoldDeduction.Items) != 0 {
		t.Errorf("enero no tuvo ventas: salesInPeriod = %d, renglones = %d", v.SalesInPeriod, len(v.UnsoldDeduction.Items))
	}
}

func negativeOf(v app.InventoryHealthView, ing int64) *app.HealthNegative {
	for i := range v.NegativeStock.Items {
		if v.NegativeStock.Items[i].IngredientID == ing {
			return &v.NegativeStock.Items[i]
		}
	}
	return nil
}

// Lo que no depende de las ventas: negativos, insumos sin compras, recetas con un error, paquetes
// con un hueco de varios productos y recetas pendientes.
func TestHealthSectionsThatDoNotDependOnSales(t *testing.T) {
	t.Parallel()
	k := newHealthKit(t)
	ctx := context.Background()
	st := k.st

	// Vender el café deja la leche en negativo: las compras todavía no entran al almacén.
	k.sell(t, line(k.cafe, "1"))
	v := reportAsApp(t, st, julio())
	n := negativeOf(v, k.milk)
	if n == nil || !n.OnHand.Equal(decimal.NewFromInt(-200)) {
		t.Fatalf("la leche quedó en -200 y aparece así: %+v", n)
	}
	if n.UsedBy != 2 {
		t.Errorf("la leche la usan el café y la pieza A: usedBy = %d", n.UsedBy)
	}
	if !v.NegativeStock.Collapsed {
		t.Error("sin ninguna compra registrada, la sección de negativos abre resumida")
	}
	if v.MultipleBranches || n.Branch != nil {
		t.Errorf("con una sola sucursal no se muestra la sucursal: %+v", n)
	}

	// «Sin compras»: todo insumo usado por una receta activa, mientras no entre una compra.
	if v.NoPurchases.Total != 2 || v.NoPurchases.Of != 2 {
		t.Errorf("sin compras = %d de %d, quiere 2 de 2 (leche y azúcar)", v.NoPurchases.Total, v.NoPurchases.Of)
	}
	if len(v.NoPurchases.Items) != 0 {
		t.Errorf("sin pedir la sección, la lista de «sin compras» viene vacía (la pantalla la pide al desplegar): %d", len(v.NoPurchases.Items))
	}
	if _, err := st.Pool.Exec(ctx, `
		insert into stock_movements (item_type, ingredient_id, movement_type, quantity, created_at)
		values ('ingrediente', $1, 'compra', 1000, '2026-06-15')`, k.azucar); err != nil {
		t.Fatal(err)
	}
	v = reportAsApp(t, st, julio())
	if v.NegativeStock.Collapsed {
		t.Error("con una compra registrada, la sección de negativos ya no abre resumida")
	}
	if v.NoPurchases.Total != 1 {
		t.Errorf("con una compra de azúcar, sin compras = %d, quiere 1 (la leche)", v.NoPurchases.Total)
	}
	desdeJulio := julio()
	since := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	desdeJulio.NoPurchasesSince = &since
	desdeJulio.Section = domain.SectionNoPurchases
	if v := reportAsApp(t, st, desdeJulio); v.NoPurchases.Total != 2 || len(v.NoPurchases.Items) != 2 {
		t.Errorf("la compra de azúcar fue en junio: sin compras desde julio = %d (%d renglones), quiere 2",
			v.NoPurchases.Total, len(v.NoPurchases.Items))
	}

	// Recetas con un error: unidad de otro tipo, insumo archivado, insumo duplicado.
	archivado := makeIngredient(t, st, "Canela salud")
	duplicado := makeIngredient(t, st, "AZUCAR SALUD")
	malo := makeProduct(t, st, "Malteada salud", decimal.RequireFromString("50"), false)
	withRecipe(t, st, "products", malo, map[int64]string{archivado: "5", duplicado: "10"})
	if _, err := st.Pool.Exec(ctx, `update ingredients set is_active = false where id = $1`, archivado); err != nil {
		t.Fatal(err)
	}
	// La unidad no puede desalinearse al capturar (lo impide un trigger), pero sí si después cambia
	// la unidad base del insumo: una receta en gramos de algo que ahora se lleva en mililitros.
	jarabe := makeIngredient(t, st, "Jarabe salud")
	limonada := makeProduct(t, st, "Limonada salud", decimal.RequireFromString("30"), false)
	withRecipe(t, st, "products", limonada, map[int64]string{jarabe: "50"})
	if _, err := st.Pool.Exec(ctx, `update ingredients set base_unit_id = (select id from units where code = 'ml') where id = $1`, jarabe); err != nil {
		t.Fatal(err)
	}
	v = reportAsApp(t, st, julio())
	problems := map[string]bool{}
	for _, m := range v.MalformedRecipes.Items {
		problems[m.OwnerName+"|"+m.Problem] = true
	}
	for _, want := range []string{"Malteada salud|archived_ingredient", "Malteada salud|duplicate_ingredient", "Limonada salud|unit_mismatch"} {
		if !problems[want] {
			t.Errorf("falta el error de receta %q en %v", want, problems)
		}
	}
	if v.MalformedRecipes.Total != len(v.MalformedRecipes.Items) {
		t.Errorf("total de recetas con un error = %d con %d renglones", v.MalformedRecipes.Total, len(v.MalformedRecipes.Items))
	}

	// Un paquete con un hueco de dos productos siempre resta el de fábrica.
	var slot int64
	if err := st.Pool.QueryRow(ctx, `select id from combo_slots where combo_id = $1 order by position limit 1`, k.combo).Scan(&slot); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Pool.Exec(ctx, `insert into combo_slot_products (slot_id, product_id) values ($1, $2)`, slot, k.compB); err != nil {
		t.Fatal(err)
	}
	v = reportAsApp(t, st, julio())
	if len(v.ComboChoices.Items) != 1 || v.ComboChoices.Items[0].ProductID != k.combo || v.ComboChoices.Items[0].Options != 2 {
		t.Errorf("el paquete con un hueco de dos productos se señala: %+v", v.ComboChoices.Items)
	}

	// Recetas pendientes: los mismos números que las pestañas de Menú › Recetas.
	as := appRoleStore(t)
	tctx, release, err := as.AcquireTenant(ctx, defaultCompanyID)
	if err != nil {
		t.Fatal(err)
	}
	page, err := app.NewAdminService(as).ListRecipes(tctx, domain.RecipeFilter{
		Kind: domain.RecipeKindProduct, Status: domain.RecipeStatusPending, Sort: domain.RecipeSortAZ, Limit: 25}, fixedNow)
	release()
	if err != nil {
		t.Fatal(err)
	}
	pr := v.PendingRecipes
	if pr.Undecided.Products != page.Totals.Product.Pending || pr.Review.Products != page.Totals.Product.Review ||
		pr.Undecided.Extras != page.Totals.Extra.Pending || pr.Review.Extras != page.Totals.Extra.Review {
		t.Errorf("recetas pendientes %+v no coinciden con las pestañas de Recetas %+v", pr, page.Totals)
	}
	if pr.Undecided.Products == 0 || !strings.Contains(pr.Undecided.FixAt, "status=pending") || !strings.Contains(pr.Review.FixAt, "status=review") {
		t.Errorf("el Shot y la pieza B nadie los ha decidido, aunque B no se vendió suelta: %+v", pr)
	}

	// Con dos sucursales, la sucursal se muestra.
	if _, err := st.Pool.Exec(ctx, `insert into branches (company_id, branch_number, code, name) values ($1, 2, 'SALUD2', 'Sucursal dos salud')`, defaultCompanyID); err != nil {
		t.Fatal(err)
	}
	if v := reportAsApp(t, st, julio()); !v.MultipleBranches || negativeOf(v, k.milk) == nil || negativeOf(v, k.milk).Branch == nil {
		t.Errorf("con dos sucursales el negativo dice de cuál es: %+v", negativeOf(v, k.milk))
	}
}

// Corregir una receta no reescribe las ventas pasadas (criterio del dueño del 13-oct).
func TestSavingARecipeDoesNotRewritePastSales(t *testing.T) {
	t.Parallel()
	k := newHealthKit(t)
	ctx := context.Background()
	k.sell(t, line(k.cafe, "2"), line(k.shot, "1"))
	ventas := func() string {
		var s string
		if err := k.st.Pool.QueryRow(ctx, `
			select coalesce(string_agg(id || ':' || quantity, ',' order by id), '') from stock_movements where movement_type = 'venta'`).Scan(&s); err != nil {
			t.Fatal(err)
		}
		return s
	}
	antes := ventas()
	admin := makeUser(t, k.st, "admin_salud", "admin")
	g := unitID(t, k.st, "g")
	if err := app.NewAdminService(k.st).SaveComposition(ctx, app.CompositionOfProduct, k.shot,
		app.CompositionRequest{Items: []app.CompositionInputItem{{IngredientID: k.milk, Quantity: decimal.RequireFromString("30"), UnitID: g}}}, admin); err != nil {
		t.Fatal(err)
	}
	if err := app.NewAdminService(k.st).SaveComposition(ctx, app.CompositionOfProduct, k.cafe,
		app.CompositionRequest{Items: []app.CompositionInputItem{{IngredientID: k.milk, Quantity: decimal.RequireFromString("250"), UnitID: g}}}, admin); err != nil {
		t.Fatal(err)
	}
	if despues := ventas(); despues != antes {
		t.Fatalf("guardar una receta reescribió las ventas pasadas:\nantes   %s\ndespués %s", antes, despues)
	}
	v := reportAsApp(t, k.st, julio())
	if unsoldOf(v, "product", k.shot) == nil {
		t.Error("el Shot se vendió antes de tener receta: esa venta sigue sin restar y la pantalla lo dice")
	}
}

// Ninguna sección alcanza nada de otra empresa, en los tres casos donde RLS ya falló.
func TestInventoryHealthIsIsolated(t *testing.T) {
	t.Parallel()
	k := newHealthKit(t)
	k.sell(t, line(k.shot, "1"), line(k.cafe, "1"))
	other := makeCompany(t, k.st, "otra-salud")
	inTheThreeCases(t, defaultCompanyID, other, func(t *testing.T, st *store.Store, ctx context.Context) {
		v, err := app.NewInventoryHealthService(st).Report(ctx, julio())
		if err != nil {
			return // sin empresa la consulta puede negarse; lo que no puede es devolver datos ajenos
		}
		if len(v.UnsoldDeduction.Items) != 0 || len(v.NegativeStock.Items) != 0 || v.NoPurchases.Of != 0 ||
			len(v.MalformedRecipes.Items) != 0 || v.PendingRecipes.Undecided.Products != 0 || v.SalesInPeriod != 0 {
			t.Fatalf("la salud del inventario de otra empresa muestra datos de la dueña: %+v", v)
		}
	})
}

// La exportación y la pantalla salen del mismo cálculo (SC-005): si divergen, una de las dos miente.
func TestInventoryHealthExportMatchesTheScreen(t *testing.T) {
	t.Parallel()
	k := newHealthKit(t)
	k.sell(t, line(k.shot, "2"), line(k.cafe, "1"), line(k.frappe, "3", k.sinReceta))
	// Un nombre que una hoja de cálculo leería como fórmula.
	formula := makeProduct(t, k.st, "=HYPERLINK(\"x\")", decimal.RequireFromString("10"), false)
	k.sell(t, line(formula, "1"))

	as := appRoleStore(t)
	ctx, release, err := as.AcquireTenant(context.Background(), defaultCompanyID)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	svc := app.NewInventoryHealthService(as)
	screen, err := svc.Report(ctx, julio())
	if err != nil {
		t.Fatal(err)
	}
	body, name, err := svc.CSV(ctx, julio())
	if err != nil {
		t.Fatal(err)
	}
	if name != "salud-del-inventario-2026-07-01-2026-07-31.csv" {
		t.Errorf("nombre del archivo = %q", name)
	}
	if !strings.HasPrefix(string(body), "\uFEFF") {
		t.Error("sin BOM, una hoja de cálculo lee mal los acentos")
	}
	rows, err := csv.NewReader(strings.NewReader(strings.TrimPrefix(string(body), "\uFEFF"))).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(rows[0], ",") != "seccion,tipo,id,nombre,grupo,estado,canal,piezas,costo_estimado,existencia,sucursal,problema" {
		t.Fatalf("encabezado = %v", rows[0])
	}
	count := map[string]int{}
	for _, r := range rows[1:] {
		count[r[0]]++
		if strings.HasPrefix(r[3], "=") || strings.HasPrefix(r[3], "+") || strings.HasPrefix(r[3], "@") {
			t.Errorf("la celda %q se leería como fórmula", r[3])
		}
	}
	// Un renglón por canal en lo vendido sin restar, uno por insumo en lo demás, y el resumen de pendientes.
	channels := 0
	for _, it := range screen.UnsoldDeduction.Items {
		channels += len(it.Units)
	}
	if count["unsold_deduction"] != channels+len(screen.ComboChoices.Items) {
		t.Errorf("la exportación y la pantalla divergen en «Se vende sin restar»: %d renglones contra %d canales en pantalla",
			count["unsold_deduction"], channels+len(screen.ComboChoices.Items))
	}
	if count["negative_stock"] != screen.NegativeStock.Total {
		t.Errorf("la exportación y la pantalla divergen en negativos: %d contra %d", count["negative_stock"], screen.NegativeStock.Total)
	}
	// La pantalla muestra «sin compras» resumido; el archivo trae la lista completa (FR-006).
	if count["no_purchases"] != screen.NoPurchases.Total {
		t.Errorf("la exportación trae %d insumos sin compras, la pantalla cuenta %d", count["no_purchases"], screen.NoPurchases.Total)
	}
	if count["malformed_recipe"] != screen.MalformedRecipes.Total {
		t.Errorf("la exportación y la pantalla divergen en recetas con un error: %d contra %d", count["malformed_recipe"], screen.MalformedRecipes.Total)
	}
	if count["pending_recipe"] != 4 {
		t.Errorf("recetas pendientes van como cuatro renglones de resumen: hay %d", count["pending_recipe"])
	}
}

// Un extra que nadie configuró se vende sin bloqueo ni aviso a quien opera, y aparece en Salud
// (FR-017, decisión del dueño del 13-oct); uno confirmado «No gasta insumos» no es problema.
func TestUnconfiguredExtraSellsAndShowsUp(t *testing.T) {
	t.Parallel()
	k := newHealthKit(t)
	o, err := k.orders.Create(context.Background(), app.CreateOrderCmd{
		ClientUUID: uuid.New(), ServiceType: "mostrador", OpenedBy: k.cashier,
		Lines: []domain.OrderLineInput{line(k.frappe, "2", k.sinReceta, k.hielo)},
	})
	if err != nil {
		t.Fatalf("un extra sin configurar bloqueó la venta: %v", err)
	}
	if len(o.Lines) != 1 {
		t.Fatalf("la venta salió con %d renglones", len(o.Lines))
	}
	v := reportAsApp(t, k.st, julio())
	it := unsoldOf(v, "extra", k.sinReceta)
	if it == nil || it.Status != "extra" || !it.Pieces.Equal(decimal.NewFromInt(2)) {
		t.Errorf("el extra sin configurar aparece así: %+v", it)
	}
	if unsoldOf(v, "extra", k.hielo) != nil {
		t.Error("«Sin hielo», confirmado «No gasta insumos», aparece como problema")
	}
}

// Pedir «Ver más» más allá del final no puede decir «0»: se leería como «todo sano» (revisión de
// backend del 037).
func TestAnEmptyPageKeepsTheRealTotal(t *testing.T) {
	t.Parallel()
	k := newHealthKit(t)
	k.sell(t, line(k.shot, "2"), line(k.frappe, "1", k.sinReceta))
	f := julio()
	f.Section = domain.SectionUnsold
	f.Offset = 1000
	v := reportAsApp(t, k.st, f)
	if len(v.UnsoldDeduction.Items) != 0 || v.UnsoldDeduction.Total != 2 {
		t.Errorf("página vacía: %d renglones, total %d; quiere 0 renglones y el total real, 2",
			len(v.UnsoldDeduction.Items), v.UnsoldDeduction.Total)
	}
	f.Section = domain.SectionMalformed
	if v := reportAsApp(t, k.st, f); v.MalformedRecipes.Total != 0 {
		t.Errorf("sin recetas con error, el total sigue en 0: %d", v.MalformedRecipes.Total)
	}
}

// Visto en el ambiente de pruebas (13-oct): los extras sin costo capturado salían con «$0 costo
// estimado», que se lee como «aquí no se pierde dinero», y por ordenar por costo se iban al fondo
// aunque se vendieran mucho. Sin costo se dice «sin costo» (null, celda vacía en el CSV) y esos
// renglones van después de los que tienen monto, por piezas vendidas.
func TestAnItemWithoutCostSaysSoInsteadOfZero(t *testing.T) {
	t.Parallel()
	k := newHealthKit(t)
	galleta := makeProduct(t, k.st, "Galleta sin costo salud", decimal.RequireFromString("10"), false)
	dulce := makeProduct(t, k.st, "Dulce sin costo salud", decimal.RequireFromString("10"), false)
	k.sell(t, line(galleta, "5"), line(dulce, "2"), line(k.shot, "1"), line(k.frappe, "1", k.sinReceta))

	v := reportAsApp(t, k.st, julio())
	var order []int64
	for _, it := range v.UnsoldDeduction.Items {
		order = append(order, it.ID)
	}
	want := []int64{k.shot, k.sinReceta, galleta, dulce}
	if len(order) != len(want) {
		t.Fatalf("renglones = %v, quiere %v", order, want)
	}
	for i := range want {
		if order[i] != want[i] {
			t.Fatalf("orden = %v, quiere primero los que tienen monto (Shot 12.50, sabor 4.00) y después los sin costo por piezas (galleta 5, dulce 2): %v", order, want)
		}
	}
	raw, err := json.Marshal(unsoldOf(v, "product", galleta))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"estimatedCost":null`) {
		t.Errorf("un producto sin costo capturado viaja como %s: «0» se lee como «no se pierde nada»", raw)
	}

	as := appRoleStore(t)
	ctx, release, err := as.AcquireTenant(context.Background(), defaultCompanyID)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	body, _, err := app.NewInventoryHealthService(as).CSV(ctx, julio())
	if err != nil {
		t.Fatal(err)
	}
	rows, err := csv.NewReader(strings.NewReader(strings.TrimPrefix(string(body), "\uFEFF"))).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rows[1:] {
		if r[3] == "Galleta sin costo salud" && r[8] != "" {
			t.Errorf("en el CSV la galleta sin costo dice costo_estimado %q; quiere la celda vacía", r[8])
		}
	}
}

// Una compra anulada deja su entrada y su reversión (spec 038): no es una compra. Contarla sacaría
// el insumo de «sin compras» y abriría completa la sección de negativos con nada comprado.
func TestACancelledPurchaseDoesNotCountAsBought(t *testing.T) {
	t.Parallel()
	k := newHealthKit(t)
	k.sell(t, line(k.cafe, "1"))
	if _, err := k.st.Pool.Exec(context.Background(), `
		insert into stock_movements (item_type, ingredient_id, movement_type, quantity, created_at)
		values ('ingrediente', $1, 'compra', 1000, '2026-07-02'), ('ingrediente', $1, 'compra', -1000, '2026-07-03')`, k.azucar); err != nil {
		t.Fatal(err)
	}
	v := reportAsApp(t, k.st, julio())
	if v.NoPurchases.Total != 2 {
		t.Errorf("sin compras = %d; quiere 2: la compra de azúcar se anuló", v.NoPurchases.Total)
	}
	if !v.NegativeStock.Collapsed {
		t.Error("con la única compra anulada, la sección de negativos sigue abriendo resumida")
	}
}
