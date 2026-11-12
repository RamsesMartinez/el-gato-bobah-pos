//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
)

// Spec 077: desactivar y reactivar un insumo por la puerta para agentes. La herramienta despacha las
// rutas de la hoja «Archivar» de la ficha; las reglas (existencia sin confirmar, receta activa, conteo
// abierto, nombre tomado, insumo unido) las decide el servicio y aquí se comprueba que el agente las ve
// y no se las salta.

func attentionOf(res rpcResult) []string {
	raw, _ := res.data["attention"].([]any)
	out := make([]string, len(raw))
	for i, a := range raw {
		out[i], _ = a.(string)
	}
	return out
}

func onHandOf(t *testing.T, w mcpWorld, ing int64) decimal.Decimal {
	t.Helper()
	var q decimal.Decimal
	if err := w.owner.Pool.QueryRow(context.Background(), `select coalesce(sum(on_hand), 0) from stock_levels where ingredient_id = $1`, ing).Scan(&q); err != nil {
		t.Fatal(err)
	}
	return q
}

func archiveEvents(t *testing.T, w mcpWorld, action string, ing int64) int64 {
	t.Helper()
	return scalar(t, w, `select count(*) from audit_events where action = $1 and entity_id = $2`, action, ing)
}

// ---- US1: desactivar ----

func TestMCPIngredientDeactivate(t *testing.T) {
	t.Parallel()
	w := newMCPWorld(t)
	ctx := context.Background()
	tok := w.token(t, w.company, w.admin, "write:inventory")
	off := func(id int64, extra map[string]any) map[string]any {
		a := map[string]any{"ingredient_id": id, "active": false}
		for k, v := range extra {
			a[k] = v
		}
		return a
	}

	conStock := ingredientIn(t, w.owner, w.company, "Colorante 077", "ml")
	stockAdjust(t, w.owner, w.company, conStock, "120")
	t.Run("con existencia: la vista previa la pone primero y pide merma o ajuste, sin cambiar nada", func(t *testing.T) {
		res := callTool(t, w.router, tok, "ingredient_update", off(conStock, nil))
		if res.isError || res.data["preview"] != true {
			t.Fatalf("vista previa: %s", res.text)
		}
		att := attentionOf(res)
		if len(att) == 0 || !strings.Contains(att[0], "120") || !strings.Contains(att[0], "merma") || !strings.Contains(att[0], "ajuste") {
			t.Fatalf("el primer aviso no es la existencia con merma o ajuste: %v", att)
		}
		if !ingredientNow(t, w, conStock).active {
			t.Fatal("la vista previa lo desactivó")
		}
	})
	t.Run("sin confirmar se rechaza con la pista; confirmado se desactiva con la existencia intacta", func(t *testing.T) {
		res := callTool(t, w.router, tok, "ingredient_update", applyArgs(off(conStock, nil), "desactiva-stock-01"))
		if !res.isError || !strings.Contains(res.text, "confirm_on_hand: true") {
			t.Fatalf("sin confirmar: %s", res.text)
		}
		if !ingredientNow(t, w, conStock).active {
			t.Fatal("el rechazo lo desactivó")
		}
		if res := callTool(t, w.router, tok, "ingredient_update", applyArgs(off(conStock, map[string]any{"confirm_on_hand": true}), "desactiva-stock-02")); res.isError {
			t.Fatalf("confirmado: %s", res.text)
		}
		if ingredientNow(t, w, conStock).active || !onHandOf(t, w, conStock).Equal(decimal.NewFromInt(120)) {
			t.Fatalf("confirmado: activo=%v existencia=%s", ingredientNow(t, w, conStock).active, onHandOf(t, w, conStock))
		}
	})

	negativo := ingredientIn(t, w.owner, w.company, "Hielo 077", "g")
	stockAdjust(t, w.owner, w.company, negativo, "-5")
	t.Run("la existencia negativa también cuenta", func(t *testing.T) {
		res := callTool(t, w.router, tok, "ingredient_update", off(negativo, nil))
		if att := attentionOf(res); len(att) == 0 || !strings.Contains(att[0], "-5") {
			t.Fatalf("existencia negativa: %s", res.text)
		}
		if res := callTool(t, w.router, tok, "ingredient_update", applyArgs(off(negativo, nil), "desactiva-neg-001")); !res.isError {
			t.Fatalf("desactivó con existencia negativa sin confirmar: %s", res.text)
		}
	})

	azucar := ingredientIn(t, w.owner, w.company, "Azúcar 077", "g")
	agua := makeProduct(t, w.owner, "Agua de azúcar 077", decimal.NewFromInt(25), false)
	withRecipe(t, w.owner, "products", agua, map[int64]string{azucar: "10"})
	t.Run("con receta activa: la vista previa la nombra y aplicar se rechaza sin tocar la receta", func(t *testing.T) {
		res := callTool(t, w.router, tok, "ingredient_update", off(azucar, nil))
		want := "Agua de azúcar 077 (producto, id " + itoa(int(agua)) + ")"
		if !strings.Contains(strings.Join(attentionOf(res), "\n"), want) {
			t.Fatalf("no nombra la receta %q: %s", want, res.text)
		}
		res = callTool(t, w.router, tok, "ingredient_update", applyArgs(off(azucar, map[string]any{"confirm_on_hand": true}), "desactiva-receta-1"))
		if !res.isError || !strings.Contains(res.text, "pantalla") {
			t.Fatalf("con receta: %s", res.text)
		}
		if !ingredientNow(t, w, azucar).active {
			t.Fatal("se desactivó con receta activa")
		}
		if n := scalar(t, w, `select count(*) from products where id = $1 and is_active and recipe_id is not null`, agua); n != 1 {
			t.Fatal("el rechazo tocó el producto o su receta")
		}
	})

	contado := ingredientIn(t, w.owner, w.company, "Canela 077", "g")
	pendiente := ingredientIn(t, w.owner, w.company, "Vainilla 077", "g")
	var cat, exp int64
	if err := w.owner.Pool.QueryRow(ctx, `insert into expense_categories (company_id, name, financial_group) values ($1, 'Compras 077', 'operacional') returning id`, w.company).Scan(&cat); err != nil {
		t.Fatal(err)
	}
	if err := w.owner.Pool.QueryRow(ctx, `insert into expenses (company_id, category_id, amount, expense_date, created_by) values ($1, $2, 50, current_date, $3) returning id`,
		w.company, cat, w.admin).Scan(&exp); err != nil {
		t.Fatal(err)
	}
	if _, err := w.owner.Pool.Exec(ctx, `insert into expense_items (company_id, expense_id, item_type, ingredient_id, description, quantity, unit_id, amount)
		select $1, $2, 'ingrediente', $3, 'Vainilla', 100, id, 50 from units where code = 'g'`, w.company, exp, pendiente); err != nil {
		t.Fatal(err)
	}
	t.Run("conteo abierto que lo capturó: aviso y rechazo; compra por recibir: nota", func(t *testing.T) {
		if res := callTool(t, w.router, tok, "stock_count_open", applyArgs(nil, "conteo-077-abre")); res.isError {
			t.Fatalf("abrir conteo: %s", res.text)
		}
		count := scalar(t, w, `select id from stock_counts where status = 'open' and company_id = $1`, w.company)
		if res := callTool(t, w.router, tok, "stock_count_capture", applyArgs(map[string]any{"count_id": count, "lines": []any{
			map[string]any{"ingredient_id": contado, "counted_qty": "0"}}}, "conteo-077-captura")); res.isError {
			t.Fatalf("capturar: %s", res.text)
		}
		res := callTool(t, w.router, tok, "ingredient_update", off(contado, nil))
		if !strings.Contains(strings.Join(attentionOf(res), "\n"), "conteo") {
			t.Fatalf("no avisa del conteo abierto: %s", res.text)
		}
		if res := callTool(t, w.router, tok, "ingredient_update", applyArgs(off(contado, nil), "desactiva-conteo-1")); !res.isError || !ingredientNow(t, w, contado).active {
			t.Fatalf("desactivó con un conteo abierto que lo capturó: %s", res.text)
		}
		if res := callTool(t, w.router, tok, "stock_count_discard", applyArgs(map[string]any{"count_id": count}, "conteo-077-descarta")); res.isError {
			t.Fatalf("descartar: %s", res.text)
		}
		res = callTool(t, w.router, tok, "ingredient_update", off(pendiente, nil))
		if !strings.Contains(res.text, "por recibir") {
			t.Fatalf("no avisa de la compra por recibir: %s", res.text)
		}
	})

	suelto := ingredientIn(t, w.owner, w.company, "Anís 077", "g")
	t.Run("sin nada: diez veces la misma clave dejan un solo archivado, con bitácora", func(t *testing.T) {
		args := applyArgs(off(suelto, nil), "desactiva-suelto-1")
		for i := range 10 {
			if res := callTool(t, w.router, tok, "ingredient_update", args); res.isError {
				t.Fatalf("intento %d: %s", i+1, res.text)
			}
		}
		if ingredientNow(t, w, suelto).active {
			t.Fatal("sigue activo")
		}
		if n := archiveEvents(t, w, "ingredient.archived", suelto); n != 1 {
			t.Fatalf("eventos de archivar: %d", n)
		}
		var after string
		if err := w.owner.Pool.QueryRow(ctx, `select after::text from audit_events where action = 'mcp.tool_called'
			and after->>'tool' = 'ingredient_update' and after->>'mode' = 'apply' and after ? 'entityAfter'
			and after::text like '%Anís 077%' order by id limit 1`).Scan(&after); err != nil {
			t.Fatalf("sin evento de la llamada aplicada: %v", err)
		}
		if !strings.Contains(after, `\"isActive\": false`) && !strings.Contains(after, `"isActive": false`) && !strings.Contains(after, `isActive\":false`) {
			t.Fatalf("el después de la llamada no dice que quedó inactivo: %s", after)
		}
	})
	t.Run("ya inactivo: la vista previa lo dice y aplicar no cambia nada", func(t *testing.T) {
		res := callTool(t, w.router, tok, "ingredient_update", off(suelto, nil))
		if res.isError || !strings.Contains(res.text, "ya está inactivo") {
			t.Fatalf("ya inactivo: %s", res.text)
		}
		if res := callTool(t, w.router, tok, "ingredient_update", applyArgs(off(suelto, nil), "desactiva-suelto-2")); res.isError {
			t.Fatalf("aplicar sobre inactivo: %s", res.text)
		}
		if n := archiveEvents(t, w, "ingredient.archived", suelto); n != 1 {
			t.Fatalf("eventos de archivar: %d", n)
		}
	})

	t.Run("active va solo y confirm_on_hand solo con active: false", func(t *testing.T) {
		cases := []map[string]any{
			off(azucar, map[string]any{"name": "Otro 077"}),
			off(azucar, map[string]any{"alt_names": []string{}}),
			off(azucar, map[string]any{"min_stock": "5"}),
			{"ingredient_id": azucar, "active": true, "confirm_on_hand": true},
			{"ingredient_id": azucar, "confirm_on_hand": true},
		}
		for i, a := range cases {
			res := callTool(t, w.router, tok, "ingredient_update", applyArgs(a, "desactiva-args-"+itoa(i)+"xx"))
			if !res.isError {
				t.Errorf("%v: no se rechazó: %s", a, res.text)
			}
		}
		if r := ingredientNow(t, w, azucar); r.name != "Azúcar 077" || r.min != nil || !r.active {
			t.Fatalf("un rechazo cambió algo: %+v", r)
		}
	})

	t.Run("sin alcance, sin permiso de configurar, o de otra empresa: nada cambia", func(t *testing.T) {
		victima := ingredientIn(t, w.owner, w.company, "Clavo 077", "g")
		catalogo := w.token(t, w.company, w.admin, "write:catalog")
		sinConf := w.token(t, w.company, userWithPerms(t, w, "sin-configurar-077", domain.PermInventoryView, domain.PermInventoryReceive), "write:inventory")
		ajeno := w.token(t, w.other, w.otherAdm, "write:inventory")
		for name, tk := range map[string]string{"sin alcance": catalogo, "sin configurar": sinConf, "otra empresa": ajeno} {
			res := callTool(t, w.router, tk, "ingredient_update", applyArgs(off(victima, nil), "desactiva-gate-01"))
			if !res.isError && len(res.rpcErr) == 0 {
				t.Errorf("%s desactivó: %s", name, res.result)
			}
			// Sin el alcance la herramienta no se registra para ese token (072): llamarla es «unknown tool».
			if name == "sin alcance" && contains(listTools(t, w.router, tk), "ingredient_update") {
				t.Errorf("un token sin write:inventory ve ingredient_update")
			}
			if name == "otra empresa" {
				prev := callTool(t, w.router, tk, "ingredient_update", off(victima, nil))
				if strings.Contains(prev.text, "Clavo 077") {
					t.Errorf("otra empresa ve la ficha en la vista previa: %s", prev.text)
				}
			}
		}
		if contains(listTools(t, w.router, sinConf), "ingredient_update") {
			t.Fatal("un rol sin inventory.configure ve ingredient_update")
		}
		if !ingredientNow(t, w, victima).active {
			t.Fatal("un rechazo lo desactivó")
		}
	})

	t.Run("configurando sin ver el almacén: la vista previa dice que no pudo revisar", func(t *testing.T) {
		sinVer := w.token(t, w.company, userWithPerms(t, w, "sin-ver-077", domain.PermInventoryConfigure), "write:inventory")
		res := callTool(t, w.router, sinVer, "ingredient_update", off(azucar, nil))
		if !strings.Contains(res.text, "No se pudo") {
			t.Fatalf("calla la revisión que no pudo leer: %s", res.text)
		}
	})
}

// ---- US2: reactivar ----

func TestMCPIngredientReactivate(t *testing.T) {
	t.Parallel()
	w := newMCPWorld(t)
	ctx := context.Background()
	tok := w.token(t, w.company, w.admin, "write:inventory")
	on := func(id int64) map[string]any { return map[string]any{"ingredient_id": id, "active": true} }
	setActive := func(id int64, v bool) {
		if _, err := w.owner.Pool.Exec(ctx, `update ingredients set is_active = $2 where id = $1`, id, v); err != nil {
			t.Fatal(err)
		}
	}

	lavanda := ingredientIn(t, w.owner, w.company, "Lavanda 077", "ml")
	stockAdjust(t, w.owner, w.company, lavanda, "40")
	if res := callTool(t, w.router, tok, "ingredient_update", applyArgs(map[string]any{"ingredient_id": lavanda, "active": false,
		"confirm_on_hand": true}, "reactiva-prepara-1")); res.isError {
		t.Fatalf("desactivar: %s", res.text)
	}

	t.Run("la vista previa dice que vuelve tal como estaba, con su existencia", func(t *testing.T) {
		res := callTool(t, w.router, tok, "ingredient_update", on(lavanda))
		if res.isError || !strings.Contains(res.text, "tal como estaba") || !strings.Contains(res.text, "40") {
			t.Fatalf("vista previa: %s", res.text)
		}
		if ingredientNow(t, w, lavanda).active {
			t.Fatal("la vista previa lo reactivó")
		}
	})
	t.Run("aplicar lo reactiva y vuelve a salir en conteo, búsqueda y existencias", func(t *testing.T) {
		if res := callTool(t, w.router, tok, "ingredient_update", applyArgs(on(lavanda), "reactiva-lavanda-1")); res.isError {
			t.Fatalf("reactivar: %s", res.text)
		}
		if !ingredientNow(t, w, lavanda).active || archiveEvents(t, w, "ingredient.reactivated", lavanda) != 1 {
			t.Fatal("no quedó activo o sin su evento")
		}
		if res := callTool(t, w.router, tok, "articles_search", map[string]any{"query": "Lavanda 077"}); !strings.Contains(res.text, "Lavanda 077") {
			t.Fatalf("búsqueda: %s", res.text)
		}
		if res := callTool(t, w.router, tok, "stock_levels", map[string]any{"search": "Lavanda 077"}); !strings.Contains(res.text, "Lavanda 077") {
			t.Fatalf("existencias: %s", res.text)
		}
		if res := callTool(t, w.router, tok, "stock_count_current", map[string]any{}); res.isError {
			t.Fatalf("conteo: %s", res.text)
		}
	})
	t.Run("ya activo: lo dice y no cambia nada", func(t *testing.T) {
		res := callTool(t, w.router, tok, "ingredient_update", on(lavanda))
		if res.isError || !strings.Contains(res.text, "ya está activo") {
			t.Fatalf("ya activo: %s", res.text)
		}
		if res := callTool(t, w.router, tok, "ingredient_update", applyArgs(on(lavanda), "reactiva-lavanda-2")); res.isError {
			t.Fatalf("aplicar sobre activo: %s", res.text)
		}
		if archiveEvents(t, w, "ingredient.reactivated", lavanda) != 1 {
			t.Fatal("reactivar un activo dejó otro evento")
		}
	})
	t.Run("otro activo tomó su nombre: rechazo con el id del otro", func(t *testing.T) {
		viejo := ingredientIn(t, w.owner, w.company, "Café molido 077", "g")
		setActive(viejo, false)
		nuevo := ingredientIn(t, w.owner, w.company, "cafe molido 077", "g")
		res := callTool(t, w.router, tok, "ingredient_update", applyArgs(on(viejo), "reactiva-nombre-01"))
		if !res.isError || !strings.Contains(res.text, "id "+itoa(int(nuevo))) {
			t.Fatalf("nombre tomado: %s", res.text)
		}
		if ingredientNow(t, w, viejo).active {
			t.Fatal("se reactivó con el nombre tomado")
		}
	})
	t.Run("unido a otro: rechazo con el nombre del destino", func(t *testing.T) {
		origen := ingredientIn(t, w.owner, w.company, "Leche suelta 077", "ml")
		destino := ingredientIn(t, w.owner, w.company, "Leche entera 077", "ml")
		// La unión se registra con los dos activos (la base lo exige) y deja el origen archivado.
		if _, err := w.owner.Pool.Exec(ctx, `insert into ingredient_replacements (company_id, old_ingredient_id, new_ingredient_id, factor, user_id, kind)
			values ($1, $2, $3, 1, $4, 'merge')`, w.company, origen, destino, w.admin); err != nil {
			t.Fatal(err)
		}
		setActive(origen, false)
		res := callTool(t, w.router, tok, "ingredient_update", applyArgs(on(origen), "reactiva-unido-001"))
		if !res.isError || !strings.Contains(res.text, "Leche entera 077") {
			t.Fatalf("unido: %s", res.text)
		}
	})
}

// ---- US3: las lecturas dicen si está activo ----

func TestMCPArticlesSearchAndStockLevelsSayActive(t *testing.T) {
	t.Parallel()
	w := newMCPWorld(t)
	ctx := context.Background()
	tok := w.token(t, w.company, w.admin, "read")
	activo := ingredientIn(t, w.owner, w.company, "Matcha vigente 077", "g")
	archivado := ingredientIn(t, w.owner, w.company, "Matcha retirado 077", "g")
	stockAdjust(t, w.owner, w.company, activo, "7")
	stockAdjust(t, w.owner, w.company, archivado, "3")
	if _, err := w.owner.Pool.Exec(ctx, `update ingredients set is_active = false, alt_names = '{"Té verde molido 077"}' where id = $1`, archivado); err != nil {
		t.Fatal(err)
	}
	rows := func(res rpcResult) []map[string]any {
		t.Helper()
		var env struct {
			Items []map[string]any `json:"items"`
		}
		if err := json.Unmarshal([]byte(res.text), &env); err != nil {
			t.Fatalf("respuesta: %v %s", err, res.text)
		}
		return env.Items
	}
	idOf := func(r map[string]any) int64 { v, _ := r["id"].(float64); return int64(v) }

	t.Run("sin include_inactive: los mismos renglones, cada uno con active: true", func(t *testing.T) {
		res := callTool(t, w.router, tok, "articles_search", map[string]any{"query": "Matcha"})
		items := rows(res)
		if len(items) != 1 || idOf(items[0]) != activo || items[0]["active"] != true {
			t.Fatalf("búsqueda: %s", res.text)
		}
	})
	t.Run("con include_inactive: agrega el archivado, marcado, por nombre y por otro nombre, sin repetir", func(t *testing.T) {
		for _, q := range []string{"Matcha", "té verde molido"} {
			res := callTool(t, w.router, tok, "articles_search", map[string]any{"query": q, "include_inactive": true})
			seen := map[int64]int{}
			var arch map[string]any
			for _, r := range rows(res) {
				seen[idOf(r)]++
				if idOf(r) == archivado {
					arch = r
				}
			}
			if arch == nil || arch["active"] != false {
				t.Fatalf("%q: no trae el archivado marcado: %s", q, res.text)
			}
			for id, n := range seen {
				if n > 1 {
					t.Fatalf("%q: el id %d sale %d veces", q, id, n)
				}
			}
		}
	})
	t.Run("quien captura compras sin ver el almacén: se le dice, no se calla", func(t *testing.T) {
		capt := w.token(t, w.company, userWithPerms(t, w, "solo-compras-077", domain.PermExpensesCapture), "read")
		res := callTool(t, w.router, capt, "articles_search", map[string]any{"query": "Matcha", "include_inactive": true})
		if res.isError || !strings.Contains(res.text, "ver el almacén") {
			t.Fatalf("sin ver el almacén: %s", res.text)
		}
	})
	t.Run("stock_levels: is_active en cada renglón y sin el archivado", func(t *testing.T) {
		res := callTool(t, w.router, tok, "stock_levels", map[string]any{"search": "Matcha"})
		items := rows(res)
		if len(items) != 1 || items[0]["item_name"] != "Matcha vigente 077" || items[0]["is_active"] != true {
			t.Fatalf("existencias: %s", res.text)
		}
	})
}
