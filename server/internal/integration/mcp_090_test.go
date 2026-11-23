//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
)

// Las herramientas de la spec 090 por la puerta para agentes, contra la API completa servida como
// gatobobah_app. Cada una se compara con lo que deja la ruta de la pantalla: los rechazos los decide
// la ruta, y la herramienta solo tiene que hacérselos llegar al agente.

func applied(t *testing.T, res rpcResult) map[string]any {
	t.Helper()
	if res.isError || res.data["applied"] != true {
		t.Fatalf("no se aplicó: %s", res.text)
	}
	m, _ := res.data["result"].(map[string]any)
	return m
}

func previewed(t *testing.T, res rpcResult) {
	t.Helper()
	if res.isError || res.data["preview"] != true {
		t.Fatalf("vista previa: %s", res.text)
	}
}

func refused(t *testing.T, res rpcResult, want string) {
	t.Helper()
	if !res.isError || !strings.Contains(res.text, want) {
		t.Fatalf("esperaba un rechazo que dijera %q: %s", want, res.text)
	}
}

// tokenForRole crea un usuario con un rol propio de solo esos permisos y su token.
func tokenForRole(t *testing.T, w mcpWorld, name string, perms []string, scopes ...string) string {
	t.Helper()
	ctx := context.Background()
	var role, uid int64
	if err := w.owner.Pool.QueryRow(ctx, `insert into roles (company_id, name) values ($1, $2) returning id`, w.company, name).Scan(&role); err != nil {
		t.Fatal(err)
	}
	for _, p := range perms {
		if _, err := w.owner.Pool.Exec(ctx, `insert into role_permission_overrides (company_id, role_id, permission_id, granted)
			select $1, $2, id, true from permissions where code = $3`, w.company, role, p); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.owner.Pool.QueryRow(ctx, `insert into users (company_id, name, username, role_id, password_hash)
		select $1, $2::text, $2::text, $3, password_hash from users where id = $4 returning id`, w.company, name, role, w.admin).Scan(&uid); err != nil {
		t.Fatal(err)
	}
	return w.token(t, w.company, uid, scopes...)
}

// ---- US1: la herramienta de extras encuentra la opción de un grupo inactivo ----

func TestOptionUpdateFindsOptionOfInactiveGroup(t *testing.T) {
	t.Parallel()
	w := newMCPWorld(t)
	tok := w.token(t, w.company, w.admin, "write:catalog")
	_, opt := groupWithOption(t, w, "Grupo dormido", false)

	prev := callTool(t, w.router, tok, "modifier_option_update", map[string]any{"option_id": opt, "name": "Pago con efectivo"})
	previewed(t, prev)
	if !strings.Contains(prev.text, "inactivo") {
		t.Fatalf("la vista previa no avisa que el grupo está inactivo: %s", prev.text)
	}
	applied(t, callTool(t, w.router, tok, "modifier_option_update",
		applyArgs(map[string]any{"option_id": opt, "name": "Pago con efectivo", "max_per_line": 4}, "opcion-dormida-1")))
	if got := optionNowRow(t, w, opt); got.name != "Pago con efectivo" || got.max != 4 || !got.price.Equal(decimal.NewFromInt(15)) {
		t.Fatalf("opción: %+v (el precio se queda en 15)", got)
	}
	refused(t, callTool(t, w.router, tok, "modifier_option_update", map[string]any{"option_id": opt}), "Manda")
	ajeno := w.token(t, w.other, w.otherAdm, "write:catalog")
	refused(t, callTool(t, w.router, ajeno, "modifier_option_update",
		applyArgs(map[string]any{"option_id": opt, "name": "x"}, "opcion-ajena-1")), "No existe")
}

// ---- US2: un pedido de plataforma de punta a punta ----

type orderWorld struct {
	w         mcpWorld
	f         mcpFixtures
	tok       string
	uber      int16
	uberPay   int16
	propio    int64 // con precio propio en Uber: 150
	sinPropio int64 // 80 × 1.35 = 108
	orders    *app.OrdersService
	octx      context.Context
}

func newOrderWorld(t *testing.T) orderWorld {
	t.Helper()
	w := newMCPWorld(t)
	o := orderWorld{w: w, f: mcpSeed(t, w), tok: w.token(t, w.company, w.admin, "write:orders")}
	ctx := context.Background()
	o.uber = platformID(t, w.owner, w.company, "Uber Eats")
	if err := w.owner.Pool.QueryRow(ctx, `select id from payment_methods where company_id = $1 and delivery_platform_id = $2`,
		w.company, o.uber).Scan(&o.uberPay); err != nil {
		t.Fatal(err)
	}
	if _, err := w.owner.Pool.Exec(ctx, `update delivery_platforms set price_markup_pct = 35 where id = $1`, o.uber); err != nil {
		t.Fatal(err)
	}
	o.propio = makeProduct(t, w.owner, "Ramen MCP propio", decimal.NewFromInt(100), false)
	o.sinPropio = makeProduct(t, w.owner, "Ramen MCP sin propio", decimal.NewFromInt(80), false)
	if _, err := w.owner.Pool.Exec(ctx, `insert into product_platform_prices (company_id, product_id, platform_id, price, updated_by)
		values ($1, $2, $3, 150, $4)`, w.company, o.propio, o.uber, w.admin); err != nil {
		t.Fatal(err)
	}
	o.orders = app.NewOrdersService(w.app, nil)
	o.octx = tenantCtx(t, w.app, w.company, w.admin)
	return o
}

func (o orderWorld) draftArgs(ref string) map[string]any {
	return map[string]any{"service_type": "domicilio", "platform_id": o.uber, "platform_order_ref": ref,
		"customer_name": "Cliente de prueba", "lines": []map[string]any{
			{"product_id": o.propio, "qty": "1"}, {"product_id": o.sinPropio, "qty": "2"}}}
}

func orderStatus(t *testing.T, w mcpWorld, id int64) (status string, paid bool) {
	t.Helper()
	if err := w.owner.Pool.QueryRow(context.Background(), `select status::text,
		coalesce((select sum(amount) from order_payments where order_id = o.id), 0) >= o.total
		from orders o where id = $1`, id).Scan(&status, &paid); err != nil {
		t.Fatal(err)
	}
	return status, paid
}

func TestMCPPlatformOrderEndToEnd(t *testing.T) {
	t.Parallel()
	o := newOrderWorld(t)
	w := o.w
	drafts := func() int64 { return scalar(t, w, `select count(*) from order_drafts`) }

	prev := callTool(t, w.router, o.tok, "order_draft_create", o.draftArgs("UB-MCP-1"))
	previewed(t, prev)
	// 150 + 2 × 108 = 366, con el origen de cada precio y la nota del que no tiene precio propio.
	for _, want := range []string{`"total":"366"`, `"priceSource":"platform"`, `"priceSource":"markup"`, "Ramen MCP sin propio"} {
		if !strings.Contains(prev.text, want) {
			t.Fatalf("la vista previa no dice %s: %s", want, prev.text)
		}
	}
	if drafts() != 0 {
		t.Fatal("la vista previa creó una cuenta")
	}
	refused(t, callTool(t, w.router, o.tok, "order_draft_create", map[string]any{"lines": []map[string]any{
		{"product_id": o.propio, "qty": "0"}}}), "")

	args := applyArgs(o.draftArgs("UB-MCP-1"), "cuenta-uber-0001")
	draft := applied(t, callTool(t, w.router, o.tok, "order_draft_create", args))
	draftID, _ := draft["id"].(string)
	again := callTool(t, w.router, o.tok, "order_draft_create", args)
	if again.isError || again.data["replayed"] != true || drafts() != 1 {
		t.Fatalf("misma clave: %s (cuentas %d)", again.text, drafts())
	}
	refused(t, callTool(t, w.router, o.tok, "order_draft_send", applyArgs(map[string]any{"draft_id": "../../me/pin"}, "envio-malo-1")), "draft_id")

	sent := applied(t, callTool(t, w.router, o.tok, "order_draft_send", applyArgs(map[string]any{"draft_id": draftID}, "envio-uber-0001")))
	order, _ := sent["order"].(map[string]any)
	orderID := int64(order["id"].(float64))
	if total := order["total"]; total != "366" {
		t.Fatalf("el pedido nació con %v; la vista previa dijo 366", total)
	}
	applied(t, callTool(t, w.router, o.tok, "order_draft_send", applyArgs(map[string]any{"draft_id": draftID}, "envio-uber-0002")))
	if n := scalar(t, w, `select count(*) from orders where platform_order_ref = 'UB-MCP-1'`); n != 1 {
		t.Fatalf("enviar dos veces dejó %d pedidos", n)
	}

	applied(t, callTool(t, w.router, o.tok, "order_deliver", applyArgs(map[string]any{"order_id": orderID}, "entrega-uber-0001")))
	refused(t, callTool(t, w.router, o.tok, "order_discount_set", map[string]any{"order_id": orderID}), "amount")
	applied(t, callTool(t, w.router, o.tok, "order_discount_set", applyArgs(map[string]any{"order_id": orderID, "amount": "16"}, "desc-uber-0001")))

	// Un pedido de plataforma se cobra entero por monto, como en la pantalla: «todo lo que falta» es de
	// la cuenta dividida, y la vista previa lo dice con lo que falta.
	q := callTool(t, w.router, o.tok, "order_pay", map[string]any{"order_id": orderID, "method_id": o.uberPay, "all_remaining": true})
	previewed(t, q)
	if !strings.Contains(q.text, `"outstanding":"350"`) || !strings.Contains(q.text, "manda amount") {
		t.Fatalf("la vista previa no guía a cobrar por monto los 350: %s", q.text)
	}
	refused(t, callTool(t, w.router, o.tok, "order_pay", applyArgs(map[string]any{"order_id": orderID, "method_id": o.f.cash,
		"amount": "350"}, "cobro-metodo-malo")), "")
	pay := applyArgs(map[string]any{"order_id": orderID, "method_id": o.uberPay, "amount": "350"}, "cobro-uber-0001")
	for range 3 {
		applied(t, callTool(t, w.router, o.tok, "order_pay", pay))
	}
	if n := scalar(t, w, `select count(*) from order_payments where order_id = $1`, orderID); n != 1 {
		t.Fatalf("tres llamadas con la misma clave dejaron %d pagos", n)
	}
	if st, paid := orderStatus(t, w, orderID); st != "entregada" || !paid {
		t.Fatalf("pedido %s, pagado %v", st, paid)
	}
	if n := scalar(t, w, `select count(*) from audit_events where action = 'mcp.tool_called' and after->>'tool' = 'order_pay'
		and after->>'mode' = 'apply'`); n == 0 {
		t.Fatal("el cobro no quedó en la bitácora del MCP")
	}
}

func TestMCPDraftDiscard(t *testing.T) {
	t.Parallel()
	o := newOrderWorld(t)
	draft := applied(t, callTool(t, o.w.router, o.tok, "order_draft_create", applyArgs(map[string]any{
		"lines": []map[string]any{{"product_id": o.propio, "qty": "1"}}}, "cuenta-descartar-1")))
	id, _ := draft["id"].(string)
	previewed(t, callTool(t, o.w.router, o.tok, "order_draft_discard", map[string]any{"draft_id": id}))
	applied(t, callTool(t, o.w.router, o.tok, "order_draft_discard", applyArgs(map[string]any{"draft_id": id}, "descartar-1")))
	if n := scalar(t, o.w, `select count(*) from order_drafts where id = $1 and status = 'open'`, id); n != 0 {
		t.Fatal("la cuenta sigue abierta")
	}
	if contains(listTools(t, o.w.router, o.w.token(t, o.w.company, o.w.admin, "write:cash")), "order_draft_create") {
		t.Fatal("un token sin write:orders ve order_draft_create")
	}
}

// ---- US3: corregir un cobro ----

func TestMCPPaymentCorrections(t *testing.T) {
	t.Parallel()
	o := newOrderWorld(t)
	w := o.w
	prod := makeProduct(t, w.owner, "Taro corregible", decimal.NewFromInt(70), false)

	t.Run("anular un pago con motivo deja el monto por cobrar", func(t *testing.T) {
		id := crearPedidoSimple(t, o.octx, o.orders, prod, w.admin)
		pid := charge(t, o.octx, o.orders, id, int16(o.f.cash), "70", w.admin)
		refused(t, callTool(t, w.router, o.tok, "order_payment_void", applyArgs(map[string]any{"order_id": id, "payment_id": pid}, "anular-sin-motivo")), "reason")
		applied(t, callTool(t, w.router, o.tok, "order_payment_void", applyArgs(map[string]any{"order_id": id, "payment_id": pid,
			"reason": "se cobró con otro método"}, "anular-1")))
		if n := scalar(t, w, `select count(*) from order_payment_voids where original_payment_id = $1`, pid); n != 1 {
			t.Fatalf("anulaciones: %d", n)
		}
		// Cambiar de método = anular y volver a cobrar.
		applied(t, callTool(t, w.router, o.tok, "order_pay", applyArgs(map[string]any{"order_id": id, "method_id": o.f.card,
			"all_remaining": true}, "recobro-1")))
		if _, paid := orderStatus(t, w, id); !paid {
			t.Fatal("el recobro no saldó el pedido")
		}
	})

	t.Run("cambiar la terminal de un pago con tarjeta", func(t *testing.T) {
		terms := app.NewTerminalsService(w.app)
		tctx := tenantCtx(t, w.app, w.company, w.admin)
		branch := scalar(t, w, `select min(id) from branches where company_id = $1`, w.company)
		a, err := terms.Create(tctx, branch, "Terminal MCP A")
		if err != nil {
			t.Fatal(err)
		}
		b, err := terms.Create(tctx, branch, "Terminal MCP B")
		if err != nil {
			t.Fatal(err)
		}
		id := crearPedidoSimple(t, o.octx, o.orders, prod, w.admin)
		res, err := o.orders.Charge(o.octx, app.ChargeCmd{OrderID: id, MethodID: int16(o.f.card), Amount: decimal.NewFromInt(70),
			ActorID: w.admin, TerminalID: &a.ID})
		if err != nil {
			t.Fatal(err)
		}
		list := callTool(t, w.router, w.token(t, w.company, w.admin), "card_terminals_list", map[string]any{})
		if list.isError || !strings.Contains(list.text, "Terminal MCP B") {
			t.Fatalf("terminales: %s", list.text)
		}
		applied(t, callTool(t, w.router, o.tok, "order_payment_terminal_set", applyArgs(map[string]any{"order_id": id,
			"payment_id": res.PaymentID, "terminal_id": b.ID, "from_terminal_id": a.ID, "reason": "se cobró en la otra"}, "terminal-1")))
		if n := scalar(t, w, `select count(*) from order_payments where id = $1 and card_terminal_id = $2`, res.PaymentID, b.ID); n != 1 {
			t.Fatal("el pago no quedó en la terminal B")
		}
	})

	t.Run("reembolsar: la vista previa dice cuánto, y aplicar lo saca del corte", func(t *testing.T) {
		id := crearPedidoSimple(t, o.octx, o.orders, prod, w.admin)
		charge(t, o.octx, o.orders, id, int16(o.f.cash), "70", w.admin)
		if err := o.orders.DeliverAll(o.octx, id); err != nil {
			t.Fatal(err)
		}
		prev := callTool(t, w.router, o.tok, "order_refund", map[string]any{"order_id": id, "reason": "llegó frío"})
		previewed(t, prev)
		if !strings.Contains(prev.text, `"pending":"70"`) || !strings.Contains(prev.text, `"needsFolio":false`) {
			t.Fatalf("vista previa del reembolso: %s", prev.text)
		}
		refused(t, callTool(t, w.router, o.tok, "order_refund", applyArgs(map[string]any{"order_id": id}, "reembolso-sin-motivo")), "reason")
		applied(t, callTool(t, w.router, o.tok, "order_refund", applyArgs(map[string]any{"order_id": id, "reason": "llegó frío"}, "reembolso-1")))
		if got := refundedTotal(t, w.owner, id); !got.Equal(decimal.NewFromInt(70)) {
			t.Fatalf("devuelto %s", got)
		}
	})

	t.Run("sin payments.void ni payments.change_terminal no se ofrecen", func(t *testing.T) {
		tok := tokenForRole(t, w, "solo-cobra", []string{"cash.operate"}, "write:orders")
		tools := listTools(t, w.router, tok)
		for _, name := range []string{"order_payment_void", "order_refund", "order_payment_terminal_set"} {
			if contains(tools, name) {
				t.Errorf("%s se ofrece sin su permiso", name)
			}
		}
		if !contains(tools, "order_pay") {
			t.Error("order_pay no se ofrece y cobrar no pide permiso en la pantalla")
		}
	})
}

// ---- US4: costo con motivo ----

func TestMCPIngredientCostSet(t *testing.T) {
	t.Parallel()
	w := newMCPWorld(t)
	tok := w.token(t, w.company, w.admin, "write:inventory")
	ing := ingredientIn(t, w.owner, w.company, "Hielo MCP", "g")

	prev := callTool(t, w.router, tok, "ingredient_cost_set", map[string]any{"ingredient_id": ing, "cost": "0.012", "reason": "difiere de la compra"})
	previewed(t, prev)
	if !strings.Contains(prev.text, "Hielo MCP") {
		t.Fatalf("la vista previa no trae la ficha: %s", prev.text)
	}
	refused(t, callTool(t, w.router, tok, "ingredient_cost_set", applyArgs(map[string]any{"ingredient_id": ing, "cost": "0.012"}, "costo-sin-motivo")), "reason")
	applied(t, callTool(t, w.router, tok, "ingredient_cost_set", applyArgs(map[string]any{"ingredient_id": ing, "cost": "0.012",
		"reason": "difiere de la compra"}, "costo-hielo-1")))
	if n := scalar(t, w, `select count(*) from ingredient_cost_corrections where ingredient_id = $1 and reason = 'difiere de la compra'`, ing); n != 1 {
		t.Fatalf("correcciones con motivo: %d", n)
	}
	sinPermiso := tokenForRole(t, w, "solo-ve-almacen", []string{"inventory.view"}, "write:inventory")
	if contains(listTools(t, w.router, sinPermiso), "ingredient_cost_set") {
		t.Fatal("ingredient_cost_set se ofrece sin inventory.configure")
	}
}

// ---- US5: recetas ----

func TestMCPRecipes(t *testing.T) {
	t.Parallel()
	w := newMCPWorld(t)
	ctx := context.Background()
	tok := w.token(t, w.company, w.admin, "write:catalog")
	grams := unitIDOf(t, w, "g")
	leche := ingredientIn(t, w.owner, w.company, "Leche MCP receta", "g")
	_, opt := groupWithOption(t, w, "Leches A", true)
	_, twin := groupWithOption(t, w, "Leches B", true)
	if _, err := w.owner.Pool.Exec(ctx, `update modifier_options set name = 'Leche de avena MCP' where id = any($1)`, []int64{opt, twin}); err != nil {
		t.Fatal(err)
	}

	got := callTool(t, w.router, tok, "recipe_get", map[string]any{"option_id": opt})
	if got.isError || !strings.Contains(got.text, `"sameName"`) || !strings.Contains(got.text, "Leches B") {
		t.Fatalf("recipe_get: %s", got.text)
	}
	set := map[string]any{"option_id": opt, "mode": "items", "based_on": "",
		"items": []map[string]any{{"ingredient_id": leche, "quantity": "200", "unit_id": grams}}}
	prev := callTool(t, w.router, tok, "recipe_set", set)
	previewed(t, prev)
	applied(t, callTool(t, w.router, tok, "recipe_set", applyArgs(set, "receta-avena-1")))
	recipeOf := func(id int64) int64 {
		return scalar(t, w, `select count(*) from modifier_options where id = $1 and recipe_id is not null`, id)
	}
	if recipeOf(opt) != 1 || recipeOf(twin) != 0 {
		t.Fatal("sin also_in_same_name la receta se copió al homónimo, o no se guardó")
	}
	refused(t, callTool(t, w.router, tok, "recipe_set", applyArgs(set, "receta-avena-vieja")), "recipe_get")

	fresh := callTool(t, w.router, tok, "recipe_get", map[string]any{"option_id": opt})
	var v struct {
		Stamp string `json:"stamp"`
	}
	b, _ := json.Marshal(fresh.data)
	_ = json.Unmarshal(b, &v)
	set["based_on"], set["also_in_same_name"] = v.Stamp, true
	set["items"] = []map[string]any{{"ingredient_id": leche, "quantity": "250", "unit_id": grams}}
	refused(t, callTool(t, w.router, tok, "recipe_set", applyArgs(set, "receta-avena-sin-also")), "also_based_on")
	prev2 := callTool(t, w.router, tok, "recipe_set", set)
	previewed(t, prev2)
	resolved, _ := prev2.data["resolved"].(map[string]any)
	also, _ := resolved["also_based_on"].(map[string]any)
	if len(also) != 1 {
		t.Fatalf("la vista previa no trae also_based_on del homónimo: %s", prev2.text)
	}
	set["also_based_on"] = also

	// Alguien le guarda receta al homónimo entre la vista previa y el aplicar: no se pisa.
	if _, err := w.owner.Pool.Exec(ctx, `update modifier_options set composition_confirmed_at = now() where id = $1`, twin); err != nil {
		t.Fatal(err)
	}
	refused(t, callTool(t, w.router, tok, "recipe_set", applyArgs(set, "receta-avena-homonimo-viejo")), "recipe_get")
	if recipeOf(twin) != 0 {
		t.Fatal("se pisó la receta que alguien le guardó al homónimo")
	}
	if _, err := w.owner.Pool.Exec(ctx, `update modifier_options set composition_confirmed_at = null where id = $1`, twin); err != nil {
		t.Fatal(err)
	}
	applied(t, callTool(t, w.router, tok, "recipe_set", applyArgs(set, "receta-avena-2")))
	if recipeOf(twin) != 1 {
		t.Fatal("con also_in_same_name el homónimo no recibió la receta")
	}
}

// ---- US6 y US7: grupos, opciones, ligas y productos ----

func TestMCPCatalogEditing(t *testing.T) {
	t.Parallel()
	w := newMCPWorld(t)
	tok := w.token(t, w.company, w.admin, "write:catalog")
	call := func(name string, args map[string]any, key string) map[string]any {
		t.Helper()
		return applied(t, callTool(t, w.router, tok, name, applyArgs(args, key)))
	}
	id := func(m map[string]any) int64 { return int64(m["id"].(float64)) }

	g := id(call("modifier_group_create", map[string]any{"name": "Leche (bebida 16 oz) MCP", "default_min": 1, "default_max": 1}, "grupo-1-catalogo"))
	refused(t, callTool(t, w.router, tok, "modifier_group_create", applyArgs(map[string]any{"name": "Leche (bebida 16 oz) MCP"}, "grupo-repetido")), "")
	o1 := id(call("modifier_option_create", map[string]any{"group_id": g, "name": "Entera MCP"}, "op-1-catalogo"))
	call("modifier_option_create", map[string]any{"group_id": g, "name": "Deslactosada MCP", "price_delta": "5"}, "op-2-catalogo")
	refused(t, callTool(t, w.router, tok, "modifier_option_create", applyArgs(map[string]any{"group_id": g, "name": "Entera MCP"}, "op-repetida-catalogo")), "")
	_ = o1

	cats := callTool(t, w.router, tok, "categories_list", map[string]any{})
	if cats.isError {
		t.Fatalf("categorías: %s", cats.text)
	}
	cat := scalar(t, w, `select min(id) from categories where company_id = $1`, w.company)
	prev := callTool(t, w.router, tok, "product_create", map[string]any{"name": "Latte MCP", "category_id": cat, "price": "65"})
	previewed(t, prev)
	p := id(call("product_create", map[string]any{"name": "Latte MCP", "category_id": cat, "price": "65", "cost": "18.50"}, "prod-1-catalogo"))
	refused(t, callTool(t, w.router, tok, "product_create", applyArgs(map[string]any{"name": "Latte MCP", "category_id": cat, "price": "65"}, "prod-repetido")), "")

	refused(t, callTool(t, w.router, tok, "product_group_link", applyArgs(map[string]any{"product_id": p, "group_id": g,
		"min_select": 3, "max_select": 1}, "liga-mala-catalogo")), "")
	call("product_group_link", map[string]any{"product_id": p, "group_id": g, "position": 1}, "liga-1-catalogo")
	groups := callTool(t, w.router, tok, "product_groups_get", map[string]any{"product_id": p})
	if groups.isError || !strings.Contains(groups.text, "Leche (bebida 16 oz) MCP") || !strings.Contains(groups.text, `"minSelect":1`) {
		t.Fatalf("grupos del producto (hereda mínimo 1): %s", groups.text)
	}

	off := callTool(t, w.router, tok, "modifier_group_update", map[string]any{"group_id": g, "active": false})
	previewed(t, off)
	if !strings.Contains(off.text, "Latte MCP") || !strings.Contains(off.text, "1 producto activo:") {
		t.Fatalf("desactivar no nombra el producto donde deja de salir: %s", off.text)
	}
	call("modifier_group_update", map[string]any{"group_id": g, "active": false}, "grupo-off-catalogo")
	if n := scalar(t, w, `select count(*) from modifier_groups where id = $1 and not is_active and name = 'Leche (bebida 16 oz) MCP'
		and default_min_select = 1`, g); n != 1 {
		t.Fatal("desactivar cambió el nombre o el mínimo")
	}
	call("product_group_unlink", map[string]any{"product_id": p, "group_id": g}, "desliga-1-catalogo")

	call("product_update", map[string]any{"product_id": p, "name": "Latte grande MCP"}, "prod-nombre")
	var name, price, cost string
	var catNow int64
	if err := w.owner.Pool.QueryRow(context.Background(), `select name, price::text, manual_cost::text, category_id from products where id = $1`, p).
		Scan(&name, &price, &cost, &catNow); err != nil {
		t.Fatal(err)
	}
	if name != "Latte grande MCP" || decimal.RequireFromString(price).Cmp(decimal.NewFromInt(65)) != 0 ||
		decimal.RequireFromString(cost).Cmp(decimal.RequireFromString("18.50")) != 0 || catNow != cat {
		t.Fatalf("solo el nombre cambió lo demás: %s %s %s %d", name, price, cost, catNow)
	}
}

// Los ids derivados de la clave son la segunda red (revisión de seguridad de la 090): si el registro
// de idempotencia se pierde, la misma clave manda el mismo id de cuenta y el mismo clientUuid, y la
// ruta reconoce el reintento. Sin esta prueba, el registro tapaba la red y nunca se ejercitaba.
func TestMCPDerivedIDsSurviveALostIdempotencyRecord(t *testing.T) {
	t.Parallel()
	o := newOrderWorld(t)
	w := o.w
	forget := func() {
		t.Helper()
		if _, err := w.owner.Pool.Exec(context.Background(), `delete from mcp_idempotency_keys`); err != nil {
			t.Fatal(err)
		}
	}
	args := applyArgs(map[string]any{"lines": []map[string]any{{"product_id": o.propio, "qty": "1"}}}, "cuenta-perdida-0001")
	draft := applied(t, callTool(t, w.router, o.tok, "order_draft_create", args))
	forget()
	again := applied(t, callTool(t, w.router, o.tok, "order_draft_create", args))
	if draft["id"] != again["id"] || scalar(t, w, `select count(*) from order_drafts`) != 1 {
		t.Fatalf("sin registro, la misma clave abrió otra cuenta: %v y %v", draft["id"], again["id"])
	}
	sent := applied(t, callTool(t, w.router, o.tok, "order_draft_send", applyArgs(map[string]any{"draft_id": draft["id"]}, "envio-perdido-0001")))
	orderID := int64(sent["order"].(map[string]any)["id"].(float64))

	pay := applyArgs(map[string]any{"order_id": orderID, "method_id": o.f.cash, "amount": "50"}, "cobro-perdido-0001")
	applied(t, callTool(t, w.router, o.tok, "order_pay", pay))
	forget()
	applied(t, callTool(t, w.router, o.tok, "order_pay", pay))
	if n := scalar(t, w, `select count(*) from order_payments where order_id = $1`, orderID); n != 1 {
		t.Fatalf("sin registro, la misma clave cobró %d veces", n)
	}

	other := crearPedidoSimple(t, o.octx, o.orders, o.propio, w.admin)
	forget()
	res := callTool(t, w.router, o.tok, "order_pay", applyArgs(map[string]any{"order_id": other, "method_id": o.f.cash, "amount": "50"}, "cobro-perdido-0001"))
	if !res.isError || scalar(t, w, `select count(*) from order_payments where order_id = $1`, other) != 0 {
		t.Fatalf("la misma clave cobró otro pedido: %s", res.text)
	}
}

// La barrera del permiso es la ruta, no que la herramienta se esconda: con un token sin payments.void la
// ruta de reembolso responde 403 y nada cambia.
func TestRefundRouteRefusesWithoutPermission(t *testing.T) {
	t.Parallel()
	o := newOrderWorld(t)
	w := o.w
	prod := makeProduct(t, w.owner, "Taro sin permiso", decimal.NewFromInt(70), false)
	id := crearPedidoSimple(t, o.octx, o.orders, prod, w.admin)
	charge(t, o.octx, o.orders, id, int16(o.f.cash), "70", w.admin)
	if err := o.orders.DeliverAll(o.octx, id); err != nil {
		t.Fatal(err)
	}
	uid := makeUser(t, w.owner, "solo-cobra-403", "mesero")
	jwt, err := w.jm.Issue(domain.User{ID: uid, CompanyID: w.company, Name: "solo cobra", Role: domain.RoleMesero})
	if err != nil {
		t.Fatal(err)
	}
	if !forbiddenByPermission(t, w.router, "POST", "/api/v1/orders/"+itoa(int(id))+"/refund", jwt) {
		t.Fatal("la ruta de reembolso no exige payments.void")
	}
	if got := refundedTotal(t, w.owner, id); !got.IsZero() {
		t.Fatalf("se devolvió %s sin permiso", got)
	}
}
