//go:build integration

package integration

import (
	"context"
	"strconv"
	"strings"
	"testing"

	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/auth"
)

// fixtures de la empresa A para las herramientas.
type mcpFixtures struct {
	category, card, cash, register, session, concept int64
}

func mcpSeed(t *testing.T, w mcpWorld) mcpFixtures {
	t.Helper()
	ctx := context.Background()
	var f mcpFixtures
	q := func(sql string, dst *int64, args ...any) {
		t.Helper()
		if err := w.owner.Pool.QueryRow(ctx, sql, args...).Scan(dst); err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
	}
	q(`insert into expense_categories (company_id, name, financial_group) values ($1, 'Insumos MCP', 'operacional') returning id`, &f.category, w.company)
	q(`select id from payment_methods where company_id = $1 and not affects_cash_drawer and is_active order by id limit 1`, &f.card, w.company)
	q(`select id from payment_methods where company_id = $1 and affects_cash_drawer and is_active order by id limit 1`, &f.cash, w.company)
	q(`select id from cash_registers where company_id = $1 and is_primary and is_active limit 1`, &f.register, w.company)
	f.session = openMCPShift(t, w.owner.Pool, w.company, w.admin, f.register)
	q(`insert into cash_concepts (company_id, name, expense_category_id) values ($1, 'Gas MCP', $2) returning id`, &f.concept, w.company, f.category)
	return f
}

func countExpenses(t *testing.T, w mcpWorld, desc string) int {
	t.Helper()
	var n int
	if err := w.owner.Pool.QueryRow(context.Background(), `select count(*) from expenses where description = $1`, desc).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// US1: capturar un gasto con tarjeta y uno en efectivo desde un agente.
func TestMCPExpenses(t *testing.T) {
	t.Parallel()
	w := newMCPWorld(t)
	f := mcpSeed(t, w)
	tok := w.token(t, w.company, w.admin, "write:expenses")
	ctx := context.Background()

	t.Run("vista previa no guarda nada y resuelve nombres", func(t *testing.T) {
		res := callTool(t, w.router, tok, "expense_create", map[string]any{"category_id": f.category, "amount": "120.00",
			"description": "Vista previa MCP", "payments": []map[string]any{{"method_id": f.card, "amount": "120.00"}}})
		if res.isError || res.data["preview"] != true {
			t.Fatalf("vista previa: %s", res.text)
		}
		if !strings.Contains(res.text, "Insumos MCP") {
			t.Fatalf("la vista previa no nombra la categoría: %s", res.text)
		}
		if countExpenses(t, w, "Vista previa MCP") != 0 {
			t.Fatal("la vista previa guardó el gasto")
		}
	})

	t.Run("con tarjeta, diez veces la misma clave: un solo gasto", func(t *testing.T) {
		args := map[string]any{"category_id": f.category, "amount": "845.50", "description": "Súper MCP tarjeta",
			"payments": []map[string]any{{"method_id": f.card, "amount": "845.50"}}, "dry_run": false,
			"idempotency_key": "gasto-tarjeta-0001"}
		for i := range 10 {
			res := callTool(t, w.router, tok, "expense_create", args)
			if res.isError || res.data["applied"] != true {
				t.Fatalf("intento %d: %s", i+1, res.text)
			}
		}
		if n := countExpenses(t, w, "Súper MCP tarjeta"); n != 1 {
			t.Fatalf("hay %d gastos", n)
		}
		args["amount"] = "900.00"
		if res := callTool(t, w.router, tok, "expense_create", args); !res.isError || !strings.Contains(res.text, "otros argumentos") {
			t.Fatalf("la misma clave con otro importe: %s", res.text)
		}
	})

	t.Run("en efectivo: la salida aparece en el turno abierto", func(t *testing.T) {
		res := callTool(t, w.router, tok, "expense_create", map[string]any{"category_id": f.category, "amount": "200.00",
			"description": "Gas MCP efectivo", "payments": []map[string]any{{"method_id": f.cash, "amount": "200.00", "register_id": f.register}},
			"dry_run": false, "idempotency_key": "gasto-efectivo-0001"})
		if res.isError {
			t.Fatalf("efectivo: %s", res.text)
		}
		var n int
		_ = w.owner.Pool.QueryRow(ctx, `select count(*) from expense_payments p join expenses e on e.id = p.expense_id
			where e.description = 'Gas MCP efectivo' and p.register_session_id = $1`, f.session).Scan(&n)
		if n != 1 {
			t.Fatalf("el pago en efectivo no quedó en el turno abierto (%d)", n)
		}
	})

	t.Run("bitácora: la llamada y el gasto dicen que vino del agente, con la IP del cliente", func(t *testing.T) {
		var device, ip string
		if err := w.owner.Pool.QueryRow(ctx, `select coalesce(device::text, ''), coalesce(host(ip), '') from audit_events
			where action = 'expense.created' order by id desc limit 1`).Scan(&device, &ip); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(device, `"agent": "mcp"`) || !strings.Contains(device, `"tool": "expense_create"`) {
			t.Fatalf("el evento del gasto no dice que vino del agente: %s", device)
		}
		if ip != "192.0.2.1" { // la RemoteAddr de httptest: la de la petición de afuera
			t.Fatalf("IP del evento: %q", ip)
		}
		var calls int
		_ = w.owner.Pool.QueryRow(ctx, `select count(*) from audit_events where action = 'mcp.tool_called'
			and after->>'tool' = 'expense_create' and after->>'mode' = 'apply'`).Scan(&calls)
		if calls < 11 {
			t.Fatalf("faltan eventos mcp.tool_called de las aplicaciones: %d", calls)
		}
	})

	t.Run("más de 10 pagos se rechaza sin despachar lecturas", func(t *testing.T) {
		pays := make([]map[string]any, 11)
		for i := range pays {
			pays[i] = map[string]any{"method_id": f.card, "amount": "1.00"}
		}
		res := callTool(t, w.router, tok, "expense_create", map[string]any{"category_id": f.category, "amount": "11.00", "payments": pays})
		if !res.isError || !strings.Contains(res.text, "10 pagos") {
			t.Fatalf("11 pagos: %s", res.text)
		}
	})

	t.Run("dos aplicaciones a la vez con la misma clave dejan un solo gasto", func(t *testing.T) {
		args := map[string]any{"category_id": f.category, "amount": "33.00", "description": "Carrera MCP",
			"payments": []map[string]any{{"method_id": f.card, "amount": "33.00"}}, "dry_run": false, "idempotency_key": "carrera-0001"}
		done := make(chan struct{}, 4)
		for range 4 {
			// done con defer: si callTool falla, su t.Fatalf termina la goroutine y sin defer la prueba
			// se quedaba esperando hasta el timeout en vez de fallar con su causa.
			go func() { defer func() { done <- struct{}{} }(); callTool(t, w.router, tok, "expense_create", args) }()
		}
		for range 4 {
			<-done
		}
		if n := countExpenses(t, w, "Carrera MCP"); n != 1 {
			t.Fatalf("hay %d gastos", n)
		}
	})

	t.Run("un token de lectura no escribe", func(t *testing.T) {
		read := w.token(t, w.company, w.admin)
		res := callTool(t, w.router, read, "expense_create", map[string]any{"category_id": f.category, "amount": "1.00",
			"description": "No debe existir", "dry_run": false, "idempotency_key": "lectura-no-escribe"})
		if !res.isError && len(res.rpcErr) == 0 {
			t.Fatal("un token de lectura aplicó un gasto")
		}
		if countExpenses(t, w, "No debe existir") != 0 {
			t.Fatal("se guardó")
		}
	})

	t.Run("un gerente sin expenses.cancel no anula, aunque el token tenga el alcance", func(t *testing.T) {
		ger := makeUser(t, w.owner, "gerente-mcp-gastos", "gerente")
		h, _ := auth.HashSecret(tokenPassword)
		_, _ = w.owner.Pool.Exec(ctx, `update users set password_hash = $1 where id = $2`, h, ger)
		var id int64
		_ = w.owner.Pool.QueryRow(ctx, `select id from expenses where description = 'Súper MCP tarjeta'`).Scan(&id)
		// El gerente sí trae expenses.cancel; se lo quita la empresa a su rol.
		if _, err := w.owner.Pool.Exec(ctx, `insert into role_permission_overrides (company_id, role_id, permission_id, granted)
			select $1, u.role_id, p.id, false from users u, permissions p where u.id = $2 and p.code = 'expenses.cancel'`, w.company, ger); err != nil {
			t.Fatal(err)
		}
		tg := w.token(t, w.company, ger, "write:expenses")
		res := callTool(t, w.router, tg, "expense_cancel", map[string]any{"expense_id": id, "reason": "prueba", "dry_run": false,
			"idempotency_key": "anular-sin-permiso"})
		if !res.isError && len(res.rpcErr) == 0 {
			t.Fatal("se anuló sin el permiso")
		}
	})
}

// US3: cambiar precios. Solo cambia lo pedido.
func TestMCPCatalog(t *testing.T) {
	t.Parallel()
	w := newMCPWorld(t)
	tok := w.token(t, w.company, w.admin, "write:catalog")
	ctx := context.Background()
	product := makeProduct(t, w.owner, "Taro MCP", decimal.RequireFromString("65"), false)
	if _, err := w.owner.Pool.Exec(ctx, `update products set available_from = current_date, is_favorite = true where id = $1`, product); err != nil {
		t.Fatal(err)
	}
	option := optionID(t, w.owner, w.company, product)
	if _, err := w.owner.Pool.Exec(ctx, `update modifier_options set max_per_line = 3 where id = $1`, option); err != nil {
		t.Fatal(err)
	}

	t.Run("precio del producto: nada más cambia", func(t *testing.T) {
		before := productRow(t, w, product)
		res := callTool(t, w.router, tok, "product_update", map[string]any{"product_id": product, "price": "75.00",
			"dry_run": false, "idempotency_key": "precio-taro-0001"})
		if res.isError {
			t.Fatalf("precio: %s", res.text)
		}
		after := productRow(t, w, product)
		if after["price"] != "75.00" {
			t.Fatalf("precio después: %v", after["price"])
		}
		for _, k := range []string{"name", "category_id", "available_from", "is_favorite", "is_active", "needs_prep", "cost_source"} {
			if before[k] != after[k] {
				t.Errorf("%s cambió: %v → %v", k, before[k], after[k])
			}
		}
	})

	t.Run("precio del extra: el tope por renglón no regresa a 1", func(t *testing.T) {
		res := callTool(t, w.router, tok, "modifier_option_update", map[string]any{"option_id": option, "price_delta": "25.00",
			"dry_run": false, "idempotency_key": "extra-0001"})
		if res.isError {
			t.Fatalf("extra: %s", res.text)
		}
		var max int
		var delta string
		_ = w.owner.Pool.QueryRow(ctx, `select max_per_line, price_delta::text from modifier_options where id = $1`, option).Scan(&max, &delta)
		if max != 3 || delta != "25.00" {
			t.Fatalf("extra después: tope %d, precio %s", max, delta)
		}
	})

	t.Run("el rol pierde el permiso entre dos llamadas", func(t *testing.T) {
		if res := callTool(t, w.router, tok, "products_list", map[string]any{"search": "Taro MCP"}); res.isError {
			t.Fatalf("antes: %s", res.text)
		}
		if _, err := w.owner.Pool.Exec(ctx, `insert into role_permission_overrides (company_id, role_id, permission_id, granted)
			select $1, u.role_id, p.id, false from users u, permissions p where u.id = $2 and p.code = 'menu.configure'`, w.company, w.admin); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			_, _ = w.owner.Pool.Exec(ctx, `delete from role_permission_overrides where permission_id = (select id from permissions where code = 'menu.configure')`)
		})
		res := callTool(t, w.router, tok, "product_update", map[string]any{"product_id": product, "price": "80.00",
			"dry_run": false, "idempotency_key": "precio-taro-0002"})
		if !res.isError && len(res.rpcErr) == 0 {
			t.Fatal("cambió el precio sin el permiso")
		}
		if productRow(t, w, product)["price"] != "75.00" {
			t.Fatal("el precio cambió")
		}
	})

	t.Run("precio por plataforma: poner y quitar", func(t *testing.T) {
		pl := platformID(t, w.owner, w.company, "Rappi")
		res := callTool(t, w.router, tok, "platform_price_set", map[string]any{"platform_id": pl, "product_id": product,
			"price": "89.00", "dry_run": false, "idempotency_key": "rappi-taro-0001"})
		if res.isError {
			t.Fatalf("poner: %s", res.text)
		}
		var price string
		_ = w.owner.Pool.QueryRow(ctx, `select price::text from product_platform_prices where product_id = $1 and platform_id = $2`, product, pl).Scan(&price)
		if price != "89.00" {
			t.Fatalf("precio en plataforma: %q", price)
		}
		res = callTool(t, w.router, tok, "platform_price_delete", map[string]any{"platform_id": pl, "product_id": product,
			"dry_run": false, "idempotency_key": "rappi-taro-0002"})
		if res.isError {
			t.Fatalf("quitar: %s", res.text)
		}
		var n int
		_ = w.owner.Pool.QueryRow(ctx, `select count(*) from product_platform_prices where product_id = $1 and platform_id = $2`, product, pl).Scan(&n)
		if n != 0 {
			t.Fatal("no se quitó")
		}
	})
}

func productRow(t *testing.T, w mcpWorld, id int64) map[string]any {
	t.Helper()
	row := map[string]any{}
	var name, price, from, costSource string
	var cat int64
	var fav, active, prep bool
	if err := w.owner.Pool.QueryRow(context.Background(), `select name, price::text, category_id, coalesce(available_from::text, ''),
		is_favorite, is_active, needs_prep, cost_source::text from products where id = $1`, id).Scan(&name, &price, &cat, &from, &fav, &active, &prep, &costSource); err != nil {
		t.Fatal(err)
	}
	row["name"], row["price"], row["category_id"], row["available_from"] = name, price, cat, from
	row["is_favorite"], row["is_active"], row["needs_prep"], row["cost_source"] = fav, active, prep, costSource
	return row
}

// US4: consultas; otra empresa no ve nada.
func TestMCPReads(t *testing.T) {
	t.Parallel()
	w := newMCPWorld(t)
	f := mcpSeed(t, w)
	tok := w.token(t, w.company, w.admin, "write:expenses")
	res := callTool(t, w.router, tok, "expense_create", map[string]any{"category_id": f.category, "amount": "50.00",
		"description": "Gasto de A", "dry_run": false, "idempotency_key": "gasto-a-0001"})
	if res.isError {
		t.Fatal(res.text)
	}
	var expense int64
	_ = w.owner.Pool.QueryRow(context.Background(), `select id from expenses where description = 'Gasto de A'`).Scan(&expense)

	t.Run("token de otra empresa no ve el gasto de A", func(t *testing.T) {
		other := w.token(t, w.other, w.otherAdm)
		res := callTool(t, w.router, other, "expense_get", map[string]any{"expense_id": expense})
		if !res.isError || !strings.Contains(res.text, "No existe") {
			t.Fatalf("otra empresa: %s", res.text)
		}
		list := callTool(t, w.router, other, "expenses_list", map[string]any{})
		if strings.Contains(list.text, "Gasto de A") {
			t.Fatal("la lista de otra empresa trae el gasto de A")
		}
		// La bitácora guarda el estado que dio la API (404), no un 400 genérico: quien audita tiene
		// que distinguir un intento sobre algo ajeno de un argumento mal escrito.
		var status string
		if err := w.owner.Pool.QueryRow(context.Background(), `select after->>'status' from audit_events
			where action = 'mcp.tool_called' and company_id = $1 and after->>'tool' = 'expense_get'
			order by id desc limit 1`, w.other).Scan(&status); err != nil {
			t.Fatal(err)
		}
		if status != "404" {
			t.Fatalf("la bitácora guardó %q en vez del 404 de la API", status)
		}
	})

	t.Run("un rango mal escrito se rechaza con la causa", func(t *testing.T) {
		res := callTool(t, w.router, tok, "sales_summary", map[string]any{"preset": "rango", "from": "2026-13-40", "to": "2026-11-10"})
		if !res.isError || !strings.Contains(res.text, "Revisa los argumentos") {
			t.Fatalf("rango malo: %s", res.text)
		}
	})

	t.Run("páginas con tope", func(t *testing.T) {
		res := callTool(t, w.router, tok, "expenses_list", map[string]any{"page_size": 5000})
		if !res.isError {
			t.Fatalf("page_size enorme pasó: %s", res.text)
		}
	})

	t.Run("el resumen no acepta un filtro que no aplica", func(t *testing.T) {
		res := callTool(t, w.router, tok, "sales_summary", map[string]any{"preset": "mes", "status": "cancelada"})
		if !res.isError {
			t.Fatalf("status en el resumen pasó: %s", res.text)
		}
	})

	t.Run("la primera página es la 1", func(t *testing.T) {
		res := callTool(t, w.router, tok, "expenses_list", map[string]any{"page_size": 1})
		if res.isError || res.data["page"] != float64(1) {
			t.Fatalf("página: %v %s", res.data["page"], res.text)
		}
	})

	t.Run("la vista previa dice si la categoría no existe", func(t *testing.T) {
		res := callTool(t, w.router, tok, "expense_create", map[string]any{"category_id": 999999, "amount": "1.00"})
		if res.isError || !strings.Contains(res.text, "No existe la categoría") {
			t.Fatalf("vista previa: %s", res.text)
		}
	})

	t.Run("whoami dice el token y sus alcances", func(t *testing.T) {
		res := callTool(t, w.router, tok, "whoami", map[string]any{})
		if res.isError || !strings.Contains(res.text, "write:expenses") || !strings.Contains(res.text, "Claude de prueba") {
			t.Fatalf("whoami: %s", res.text)
		}
	})
}

// US5: caja e inventario.
func TestMCPCashAndInventory(t *testing.T) {
	t.Parallel()
	w := newMCPWorld(t)
	f := mcpSeed(t, w)
	ctx := context.Background()
	tok := w.token(t, w.company, w.admin, "write:cash", "write:inventory")

	t.Run("una salida sin concepto remite a la lista de conceptos", func(t *testing.T) {
		res := callTool(t, w.router, tok, "cash_movement_create", map[string]any{"register_id": f.register, "kind": "salida", "amount": "100.00"})
		if !res.isError || !strings.Contains(res.text, "cash_concepts_list") {
			t.Fatalf("salida sin concepto: %s", res.text)
		}
	})
	t.Run("salida con concepto y entrada con texto quedan en el turno", func(t *testing.T) {
		for i, args := range []map[string]any{
			{"register_id": f.register, "kind": "salida", "amount": "100.00", "concept_id": f.concept},
			{"register_id": f.register, "kind": "entrada", "amount": "50.00", "concept": "Cambio del banco"},
		} {
			args["dry_run"] = false
			args["idempotency_key"] = "movimiento-000" + strconv.Itoa(i)
			if res := callTool(t, w.router, tok, "cash_movement_create", args); res.isError {
				t.Fatalf("%v: %s", args, res.text)
			}
		}
		var n int
		_ = w.owner.Pool.QueryRow(ctx, `select count(*) from register_cash_movements where session_id = $1`, f.session).Scan(&n)
		if n < 2 {
			t.Fatalf("movimientos en el turno: %d", n)
		}
	})
	t.Run("ajuste de existencia con antes y después", func(t *testing.T) {
		ing := ingredientIn(t, w.owner, w.company, "Leche MCP", "ml")
		res := callTool(t, w.router, tok, "stock_adjust", map[string]any{"ingredient_id": ing, "quantity": "500", "reason": "conteo",
			"dry_run": false, "idempotency_key": "ajuste-leche-01"})
		if res.isError || res.data["before"] == nil || res.data["after"] == nil {
			t.Fatalf("ajuste: %s", res.text)
		}
		var n int
		_ = w.owner.Pool.QueryRow(ctx, `select count(*) from stock_movements where ingredient_id = $1 and movement_type = 'ajuste'`, ing).Scan(&n)
		if n != 1 {
			t.Fatalf("movimientos de ajuste: %d", n)
		}
	})
}

// El motivo con el que se abrió un turno —y la explicación que deja en el corte anterior— llegan al
// agente por el histórico de cortes (#290, FR-009). La nota la escribe el personal del negocio, así
// que no cae en el filtro de texto de terceros.
func TestMCPCashHistoryCarriesTheOpeningReason(t *testing.T) {
	t.Parallel()
	w := newMCPWorld(t)
	ctx := context.Background()
	back := app.NewBackofficeService(w.owner, clock)
	cajero := makeUser(t, w.owner, "cajero_mcp_motivo", "cajero")
	fuerte := registerID(t, w.owner, "Caja fuerte")
	quinientos, cuatrocientos := dec("500"), dec("400")
	if _, err := back.OpenSession(ctx, fuerte, aperturaAMano(quinientos), cajero); err != nil {
		t.Fatal(err)
	}
	if _, err := back.CloseSession(ctx, fuerte, cajero, app.CierreCmd{Total: &quinientos, Motivo: "prueba"}); err != nil {
		t.Fatal(err)
	}
	const nota = "Un gasto salió de esta caja y se contó en la otra"
	if _, err := back.OpenSession(ctx, fuerte, app.AperturaCmd{Total: &cuatrocientos, Motivo: "prueba",
		Reason: "last_count_wrong", ReasonNote: nota}, cajero); err != nil {
		t.Fatal(err)
	}
	res := callTool(t, w.router, w.token(t, w.company, w.admin), "cash_sessions_history", map[string]any{})
	if res.isError {
		t.Fatalf("cash_sessions_history: %s", res.text)
	}
	for _, quiero := range []string{`"openingReason":"last_count_wrong"`, `"openingReasonNote":"` + nota + `"`, `"reasonNote":"` + nota + `"`} {
		if !strings.Contains(strings.ReplaceAll(res.text, ": ", ":"), quiero) {
			t.Fatalf("el histórico por MCP no trae %s:\n%s", quiero, res.text)
		}
	}
}
