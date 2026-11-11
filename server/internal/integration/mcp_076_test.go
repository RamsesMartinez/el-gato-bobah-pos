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

// Las herramientas de la spec 076 por la puerta para agentes: dar de alta y editar insumos, y los
// empaques de un solo insumo. Igual que las de la 073 y la 074, se comparan con lo que deja la ruta de
// la pantalla; las reglas (nombre repetido, unidad base, versión de la ficha) las decide la ruta.

type ingredientRow struct {
	name     string
	unit     string
	category *int64
	place    *int64
	min      *decimal.Decimal
	alt      []string
	active   bool
}

func ingredientNow(t *testing.T, w mcpWorld, id int64) ingredientRow {
	t.Helper()
	var r ingredientRow
	if err := w.owner.Pool.QueryRow(context.Background(), `select i.name, u.code, i.category_id, i.count_place_id,
		i.min_stock, i.alt_names, i.is_active from ingredients i join units u on u.id = i.base_unit_id where i.id = $1`, id).
		Scan(&r.name, &r.unit, &r.category, &r.place, &r.min, &r.alt, &r.active); err != nil {
		t.Fatal(err)
	}
	return r
}

func ingredientCategoryIn(t *testing.T, w mcpWorld, company int64, name string) int64 {
	t.Helper()
	return scalarAs(t, w, company, `insert into ingredient_categories (company_id, name) values ($1, $2) returning id`, name)
}

func countPlaceIn(t *testing.T, w mcpWorld, company int64, name string) int64 {
	t.Helper()
	return scalarAs(t, w, company, `insert into count_places (company_id, name, position) values ($1, $2, 0) returning id`, name)
}

func scalarAs(t *testing.T, w mcpWorld, company int64, sql, name string) int64 {
	t.Helper()
	var id int64
	if err := w.owner.Pool.QueryRow(context.Background(), sql, company, name).Scan(&id); err != nil {
		t.Fatalf("%s: %v", sql, err)
	}
	return id
}

// before dice si `a` aparece antes que `b` en el texto que lee el agente.
func before(text, a, b string) bool {
	i, j := strings.Index(text, a), strings.Index(text, b)
	return i >= 0 && j >= 0 && i < j
}

// ---- US1: dar de alta ----

func TestMCPIngredientCreate(t *testing.T) {
	t.Parallel()
	w := newMCPWorld(t)
	ctx := context.Background()
	grams := unitIDOf(t, w, "g")
	tok := w.token(t, w.company, w.admin, "write:inventory")
	jamon := ingredientIn(t, w.owner, w.company, "Jamón de pierna MCP", "g")
	pavo := ingredientIn(t, w.owner, w.company, "Embutido de ave MCP", "g")
	if _, err := w.owner.Pool.Exec(ctx, `update ingredients set alt_names = '{"Pechuga de pavo MCP"}' where id = $1`, pavo); err != nil {
		t.Fatal(err)
	}
	count := func(name string) int64 {
		return scalar(t, w, `select count(*) from ingredients where name = $1`, name)
	}

	t.Run("la vista previa pone primero el insumo parecido y no crea nada", func(t *testing.T) {
		res := callTool(t, w.router, tok, "ingredient_create", map[string]any{"name": "Jamón de piena MCP", "base_unit_id": grams})
		if res.isError || res.data["preview"] != true {
			t.Fatalf("vista previa: %s", res.text)
		}
		want := "Quizá ya existe: Jamón de pierna MCP (id " + itoa(int(jamon)) + ")"
		if !strings.Contains(res.text, want) {
			t.Fatalf("no dice %q: %s", want, res.text)
		}
		for _, later := range []string{`"before"`, `"would_send"`} {
			if !before(res.text, want, later) {
				t.Fatalf("el aviso no va antes de %s: %s", later, res.text)
			}
		}
		if count("Jamón de piena MCP") != 0 {
			t.Fatal("la vista previa creó el insumo")
		}
	})

	t.Run("también lo señala si el parecido es un otro nombre", func(t *testing.T) {
		res := callTool(t, w.router, tok, "ingredient_create", map[string]any{"name": "Pechuga de pavo MCP", "base_unit_id": grams})
		if !strings.Contains(res.text, "Quizá ya existe: Embutido de ave MCP (id "+itoa(int(pavo))+")") {
			t.Fatalf("no señala el otro nombre: %s", res.text)
		}
	})

	t.Run("sin parecidos lo dice, y avisa que la compra trae la existencia", func(t *testing.T) {
		res := callTool(t, w.router, tok, "ingredient_create", map[string]any{"name": "Zzyzx raro MCP", "base_unit_id": grams,
			"initial_stock": "500"})
		if res.isError || strings.Contains(res.text, "Quizá ya existe") || !strings.Contains(res.text, "No hay insumos parecidos") {
			t.Fatalf("sin parecidos: %s", res.text)
		}
		if !strings.Contains(res.text, "llegan con la compra") {
			t.Fatalf("sin aviso de la existencia inicial: %s", res.text)
		}
	})

	var nuevo int64
	cat := ingredientCategoryIn(t, w, w.company, "Lácteos MCP")
	place := countPlaceIn(t, w, w.company, "Refri MCP")
	t.Run("diez veces la misma clave: un insumo con sus campos y su bitácora", func(t *testing.T) {
		args := applyArgs(map[string]any{"name": "Queso cotija MCP", "base_unit_id": grams, "category_id": cat,
			"place_id": place, "min_stock": "250", "initial_cost": "0.18"}, "insumo-cotija-0001")
		var res rpcResult
		for i := range 10 {
			if res = callTool(t, w.router, tok, "ingredient_create", args); res.isError {
				t.Fatalf("intento %d: %s", i+1, res.text)
			}
		}
		if n := count("Queso cotija MCP"); n != 1 {
			t.Fatalf("hay %d insumos", n)
		}
		nuevo = resultID(t, res)
		r := ingredientNow(t, w, nuevo)
		if r.unit != "g" || r.category == nil || *r.category != cat || r.place == nil || *r.place != place ||
			r.min == nil || !r.min.Equal(decimal.NewFromInt(250)) {
			t.Fatalf("insumo = %+v", r)
		}
		if n := scalar(t, w, `select count(*) from audit_events where action = 'ingredient.created' and entity_id = $1`, nuevo); n != 1 {
			t.Fatalf("eventos de alta: %d", n)
		}
		var after string
		if err := w.owner.Pool.QueryRow(ctx, `select after::text from audit_events where action = 'mcp.tool_called'
			and after->>'tool' = 'ingredient_create' and after->>'mode' = 'apply' and after ? 'entityAfter'
			order by id limit 1`).Scan(&after); err != nil {
			t.Fatalf("sin evento de la llamada aplicada: %v", err)
		}
		if !strings.Contains(after, "Queso cotija MCP") {
			t.Fatalf("el después del evento no trae el insumo: %s", after)
		}
	})

	t.Run("nombre repetido: se rechaza con el id del que ya existe", func(t *testing.T) {
		res := callTool(t, w.router, tok, "ingredient_create", applyArgs(map[string]any{"name": "jamon de pierna mcp",
			"base_unit_id": grams}, "insumo-dup-00001"))
		if !res.isError || !strings.Contains(res.text, "id "+itoa(int(jamon))) {
			t.Fatalf("repetido: %s", res.text)
		}
		archivado := ingredientIn(t, w.owner, w.company, "Crema vieja MCP", "g")
		if _, err := w.owner.Pool.Exec(ctx, `update ingredients set is_active = false where id = $1`, archivado); err != nil {
			t.Fatal(err)
		}
		res = callTool(t, w.router, tok, "ingredient_create", applyArgs(map[string]any{"name": "Crema vieja MCP",
			"base_unit_id": grams}, "insumo-dup-00002"))
		if !res.isError || !strings.Contains(res.text, "id "+itoa(int(archivado))+", archivado") {
			t.Fatalf("repetido archivado: %s", res.text)
		}
		if count("jamon de pierna mcp") != 0 {
			t.Fatal("se creó el repetido")
		}
	})

	t.Run("en kilos se rechaza como en la pantalla", func(t *testing.T) {
		res := callTool(t, w.router, tok, "ingredient_create", applyArgs(map[string]any{"name": "Azúcar MCP",
			"base_unit_id": unitIDOf(t, w, "kg")}, "insumo-kilo-0001"))
		if !res.isError || !strings.Contains(res.text, "empaque") || count("Azúcar MCP") != 0 {
			t.Fatalf("kilo: %s", res.text)
		}
	})

	t.Run("el insumo nuevo sirve para su empaque y para la compra", func(t *testing.T) {
		res := callTool(t, w.router, tok, "presentation_create", applyArgs(map[string]any{"ingredient_id": nuevo,
			"name": "Bolsa", "qty": "500", "unit_id": grams, "brand": "Cotija MCP"}, "insumo-empaque-01"))
		if res.isError {
			t.Fatalf("empaque: %s", res.text)
		}
		bolsa := resultID(t, res)
		f := mcpSeed(t, w)
		compras := w.token(t, w.company, w.admin, "write:purchases")
		res = callTool(t, w.router, compras, "purchase_create", map[string]any{"category_id": f.category, "amount": "95.00",
			"lines": []any{map[string]any{"name": "QUESO COTIJA 500G", "ingredient_id": nuevo, "presentation_id": bolsa,
				"quantity": "1", "amount": "95.00"}}})
		if res.isError || strings.Contains(res.text, "No existe") {
			t.Fatalf("compra en vista previa con el insumo nuevo: %s", res.text)
		}
	})

	t.Run("sin el alcance de almacén no se ofrece", func(t *testing.T) {
		gastos := w.token(t, w.company, w.admin, "write:expenses")
		if contains(listTools(t, w.router, gastos), "ingredient_create") {
			t.Fatal("un token sin almacén ve ingredient_create")
		}
		res := callTool(t, w.router, gastos, "ingredient_create", applyArgs(map[string]any{"name": "Sin alcance MCP",
			"base_unit_id": grams}, "insumo-scope-001"))
		if (!res.isError && len(res.rpcErr) == 0) || count("Sin alcance MCP") != 0 {
			t.Fatalf("un token sin almacén creó un insumo: %s", res.result)
		}
	})

	t.Run("un rol sin configurar el almacén no lo ve y la ruta lo niega", func(t *testing.T) {
		uid := userWithPerms(t, w, "almacenista-076", domain.PermInventoryView, domain.PermInventoryReceive, domain.PermInventoryCount)
		tk := w.token(t, w.company, uid, "write:inventory")
		if contains(listTools(t, w.router, tk), "ingredient_create") {
			t.Fatal("un rol sin inventory.configure ve ingredient_create")
		}
		res := callTool(t, w.router, tk, "ingredient_create", applyArgs(map[string]any{"name": "Sin permiso MCP",
			"base_unit_id": grams}, "insumo-perm-0001"))
		if (!res.isError && len(res.rpcErr) == 0) || count("Sin permiso MCP") != 0 {
			t.Fatalf("un rol sin permiso creó un insumo: %s", res.text)
		}
	})

	t.Run("un rol que configura sin ver el almacén: la vista previa dice que no pudo buscar por otros nombres", func(t *testing.T) {
		uid := userWithPerms(t, w, "configura-sin-ver-076", domain.PermInventoryConfigure)
		tk := w.token(t, w.company, uid, "write:inventory")
		res := callTool(t, w.router, tk, "ingredient_create", map[string]any{"name": "Pechuga de pavo MCP", "base_unit_id": grams})
		if res.isError || !strings.Contains(res.text, "No se pudo revisar") {
			t.Fatalf("sin aviso de la búsqueda que falló: %s", res.text)
		}
	})

	t.Run("otra empresa no ve los parecidos de ésta ni el id de su insumo, y puede usar el mismo nombre", func(t *testing.T) {
		ajeno := w.token(t, w.other, w.otherAdm, "write:inventory")
		res := callTool(t, w.router, ajeno, "ingredient_create", map[string]any{"name": "Jamón de pierna MCP", "base_unit_id": grams})
		if res.isError || strings.Contains(res.text, "Quizá ya existe") || strings.Contains(res.text, "id "+itoa(int(jamon))) {
			t.Fatalf("vista previa de otra empresa: %s", res.text)
		}
		res = callTool(t, w.router, ajeno, "ingredient_create", applyArgs(map[string]any{"name": "Jamón de pierna MCP",
			"base_unit_id": grams}, "insumo-otra-0001"))
		if res.isError || strings.Contains(res.text, "id "+itoa(int(jamon))) {
			t.Fatalf("alta de otra empresa con el mismo nombre: %s", res.text)
		}
		if n := scalar(t, w, `select count(*) from ingredients where name = 'Jamón de pierna MCP' and company_id = $1`, w.other); n != 1 {
			t.Fatalf("otra empresa tiene %d", n)
		}
	})

	t.Run("un token de otra empresa no usa la categoría de ésta", func(t *testing.T) {
		ajeno := w.token(t, w.other, w.otherAdm, "write:inventory")
		res := callTool(t, w.router, ajeno, "ingredient_create", applyArgs(map[string]any{"name": "Ajeno MCP",
			"base_unit_id": grams, "category_id": cat}, "insumo-ajeno-001"))
		if !res.isError || count("Ajeno MCP") != 0 {
			t.Fatalf("otra empresa creó con la categoría de ésta: %s", res.text)
		}
	})
}

// ---- US2: editar ----

func TestMCPIngredientUpdate(t *testing.T) {
	t.Parallel()
	w := newMCPWorld(t)
	ctx := context.Background()
	tok := w.token(t, w.company, w.admin, "write:inventory")
	jamon := ingredientIn(t, w.owner, w.company, "Jamón de pierna 076", "g")
	cat := ingredientCategoryIn(t, w, w.company, "Carnes frías 076")
	place := countPlaceIn(t, w, w.company, "Refri 076")
	if _, err := w.owner.Pool.Exec(ctx, `update ingredients set category_id = $2, count_place_id = $3, min_stock = 100 where id = $1`,
		jamon, cat, place); err != nil {
		t.Fatal(err)
	}
	otro := ingredientIn(t, w.owner, w.company, "Jamón 076 de pavo", "g")
	if _, err := w.owner.Pool.Exec(ctx, `update ingredients set alt_names = '{"Jamón ibérico 076"}' where id = $1`, otro); err != nil {
		t.Fatal(err)
	}
	upd := func(args map[string]any) map[string]any {
		a := map[string]any{"ingredient_id": jamon}
		for k, v := range args {
			a[k] = v
		}
		return a
	}

	t.Run("la vista previa dice el antes y el después y los parecidos al nombre nuevo, sin cambiar nada", func(t *testing.T) {
		res := callTool(t, w.router, tok, "ingredient_update", upd(map[string]any{"name": "Jamón 076",
			"alt_names": []string{"Jamón de pierna 076", "Jamón york 076"}}))
		if res.isError || res.data["preview"] != true {
			t.Fatalf("vista previa: %s", res.text)
		}
		for _, want := range []string{"Jamón de pierna 076", "Jamón 076", "Quizá ya existe: Jamón 076 de pavo (id " + itoa(int(otro)) + ")", "Jamón york 076"} {
			if !strings.Contains(res.text, want) {
				t.Errorf("falta %q: %s", want, res.text)
			}
		}
		if strings.Contains(res.text, "Quizá ya existe: Jamón de pierna 076") {
			t.Fatalf("se señala a sí mismo: %s", res.text)
		}
		if r := ingredientNow(t, w, jamon); r.name != "Jamón de pierna 076" || len(r.alt) != 0 {
			t.Fatalf("la vista previa cambió el insumo: %+v", r)
		}
	})

	t.Run("renombrar y dejar el nombre viejo como otro nombre, diez veces la misma clave", func(t *testing.T) {
		args := applyArgs(upd(map[string]any{"name": "Jamón 076", "alt_names": []string{"Jamón de pierna 076", "Jamón de pavo 076"}}),
			"insumo-renombra-01")
		for i := range 10 {
			if res := callTool(t, w.router, tok, "ingredient_update", args); res.isError {
				t.Fatalf("intento %d: %s", i+1, res.text)
			}
		}
		r := ingredientNow(t, w, jamon)
		if r.name != "Jamón 076" || strings.Join(r.alt, "|") != "Jamón de pierna 076|Jamón de pavo 076" {
			t.Fatalf("después de renombrar: %+v", r)
		}
		if r.category == nil || *r.category != cat || r.place == nil || *r.place != place || r.min == nil || !r.min.Equal(decimal.NewFromInt(100)) {
			t.Fatalf("renombrar tocó lo demás: %+v", r)
		}
		var n int64
		if err := w.owner.Pool.QueryRow(ctx, `select count(*) from audit_events where action = 'ingredient.updated' and entity_id = $1`,
			jamon).Scan(&n); err != nil || n != 2 {
			t.Fatalf("eventos de edición: %d %v (uno del nombre, uno de los otros nombres)", n, err)
		}
	})

	t.Run("cambiar solo el mínimo conserva categoría, ubicación y otros nombres", func(t *testing.T) {
		if res := callTool(t, w.router, tok, "ingredient_update", applyArgs(upd(map[string]any{"min_stock": "300"}), "insumo-minimo-01")); res.isError {
			t.Fatalf("mínimo: %s", res.text)
		}
		r := ingredientNow(t, w, jamon)
		if !r.min.Equal(decimal.NewFromInt(300)) || r.category == nil || r.place == nil || len(r.alt) != 2 || r.name != "Jamón 076" {
			t.Fatalf("después del mínimo: %+v", r)
		}
	})

	t.Run("clear_* quita y alt_names vacío los quita todos", func(t *testing.T) {
		if res := callTool(t, w.router, tok, "ingredient_update", applyArgs(upd(map[string]any{"clear_category": true,
			"clear_place": true, "clear_min_stock": true, "alt_names": []string{}}), "insumo-limpia-01")); res.isError {
			t.Fatalf("limpiar: %s", res.text)
		}
		r := ingredientNow(t, w, jamon)
		if r.category != nil || r.place != nil || r.min != nil || len(r.alt) != 0 || r.name != "Jamón 076" {
			t.Fatalf("después de limpiar: %+v", r)
		}
	})

	t.Run("argumentos sin nada que cambiar o contradictorios se rechazan antes de despachar", func(t *testing.T) {
		for _, a := range []map[string]any{{}, {"category_id": cat, "clear_category": true}} {
			res := callTool(t, w.router, tok, "ingredient_update", applyArgs(upd(a), "insumo-args-0001"))
			if !res.isError {
				t.Fatalf("%v: %s", a, res.text)
			}
		}
	})

	t.Run("un nombre que ya usa otro insumo se rechaza con su id y nada cambia", func(t *testing.T) {
		res := callTool(t, w.router, tok, "ingredient_update", applyArgs(upd(map[string]any{"name": "Jamón ibérico 076"}), "insumo-choca-001"))
		if !res.isError || !strings.Contains(res.text, "id "+itoa(int(otro))) {
			t.Fatalf("choque: %s", res.text)
		}
		if ingredientNow(t, w, jamon).name != "Jamón 076" {
			t.Fatal("el choque cambió el nombre")
		}
	})

	t.Run("nombre aplicado y otro nombre ajeno: el error dice que el nombre ya quedó y de quién es el otro", func(t *testing.T) {
		res := callTool(t, w.router, tok, "ingredient_update", applyArgs(upd(map[string]any{"name": "Jamón cocido 076",
			"alt_names": []string{"Jamón 076 de pavo"}}), "insumo-mitad-0001"))
		if !res.isError || !strings.Contains(res.text, "ya quedó") || !strings.Contains(res.text, "id "+itoa(int(otro))) {
			t.Fatalf("segundo paso: %s", res.text)
		}
		if r := ingredientNow(t, w, jamon); r.name != "Jamón cocido 076" || len(r.alt) != 0 {
			t.Fatalf("después del segundo paso fallido: %+v", r)
		}
	})

	t.Run("sin alcance, sin permiso de configurar, configurando sin ver, o de otra empresa: nada cambia", func(t *testing.T) {
		catalogo := w.token(t, w.company, w.admin, "write:catalog")
		sinConf := w.token(t, w.company, userWithPerms(t, w, "sin-configurar-076", domain.PermInventoryView, domain.PermInventoryReceive), "write:inventory")
		sinVer := w.token(t, w.company, userWithPerms(t, w, "sin-ver-076", domain.PermInventoryConfigure), "write:inventory")
		ajeno := w.token(t, w.other, w.otherAdm, "write:inventory")
		for name, tk := range map[string]string{"sin alcance": catalogo, "sin configurar": sinConf, "sin ver": sinVer, "otra empresa": ajeno} {
			res := callTool(t, w.router, tk, "ingredient_update", applyArgs(upd(map[string]any{"name": "Robado 076"}), "insumo-gate-0001"))
			if !res.isError && len(res.rpcErr) == 0 {
				t.Errorf("%s editó: %s", name, res.result)
			}
			if name == "sin ver" && !strings.Contains(strings.ToLower(res.text), "ver") {
				t.Errorf("sin ver: el rechazo no nombra el permiso de ver el almacén: %s", res.text)
			}
		}
		propio := ingredientIn(t, w.owner, w.other, "Insumo ajeno 076", "g")
		for _, a := range []map[string]any{{"category_id": cat}, {"place_id": place}} {
			a["ingredient_id"] = propio
			if res := callTool(t, w.router, ajeno, "ingredient_update", applyArgs(a, "insumo-ajeno-cat1")); !res.isError {
				t.Errorf("otra empresa editó su insumo con %v de ésta: %s", a, res.text)
			}
		}
		if r := ingredientNow(t, w, propio); r.category != nil || r.place != nil {
			t.Fatalf("otra empresa quedó con categoría o ubicación de ésta: %+v", r)
		}
		if contains(listTools(t, w.router, sinConf), "ingredient_update") {
			t.Fatal("un rol sin inventory.configure ve ingredient_update")
		}
		if ingredientNow(t, w, jamon).name != "Jamón cocido 076" {
			t.Fatal("un rechazo cambió el nombre")
		}
	})
}

// ---- US3: empaques de un insumo ----

func TestMCPPresentationsListByIngredient(t *testing.T) {
	t.Parallel()
	w := newMCPWorld(t)
	ctx := context.Background()
	a := ingredientIn(t, w.owner, w.company, "Leche 076", "ml")
	b := ingredientIn(t, w.owner, w.company, "Café 076", "g")
	for _, p := range []struct {
		ing  int64
		name string
	}{{a, "Caja litro 076"}, {a, "Galón 076"}, {b, "Bolsa kilo 076"}} {
		if _, err := w.owner.Pool.Exec(ctx, `insert into ingredient_presentations (company_id, ingredient_id, name, qty_in_base)
			values ($1, $2, $3, 1000)`, w.company, p.ing, p.name); err != nil {
			t.Fatal(err)
		}
	}
	tok := w.token(t, w.company, w.admin)
	names := func(res rpcResult) []string {
		t.Helper()
		if res.isError {
			t.Fatalf("lista: %s", res.text)
		}
		var out []string
		var walk func(v any)
		walk = func(v any) {
			switch x := v.(type) {
			case map[string]any:
				if n, ok := x["name"].(string); ok && strings.HasSuffix(n, " 076") {
					out = append(out, n)
				}
				for _, y := range x {
					walk(y)
				}
			case []any:
				for _, y := range x {
					walk(y)
				}
			}
		}
		var v any
		_ = json.Unmarshal(res.result, &v)
		walk(v)
		return out
	}
	t.Run("con ingredient_id solo trae los de ese insumo", func(t *testing.T) {
		got := names(callTool(t, w.router, tok, "presentations_list", map[string]any{"ingredient_id": a}))
		if len(got) == 0 || contains(got, "Bolsa kilo 076") || !contains(got, "Galón 076") {
			t.Fatalf("filtrado: %v", got)
		}
	})
	t.Run("sin el parámetro trae los de todos, como antes", func(t *testing.T) {
		got := names(callTool(t, w.router, tok, "presentations_list", map[string]any{}))
		if !contains(got, "Bolsa kilo 076") || !contains(got, "Galón 076") {
			t.Fatalf("sin filtro: %v", got)
		}
	})
	t.Run("ingredient_id presente y no positivo se rechaza", func(t *testing.T) {
		res := callTool(t, w.router, tok, "presentations_list", map[string]any{"ingredient_id": 0})
		if !res.isError {
			t.Fatalf("cero: %s", res.text)
		}
	})
	t.Run("un token de otra empresa no ve los empaques de ésta", func(t *testing.T) {
		ajeno := w.token(t, w.other, w.otherAdm)
		got := names(callTool(t, w.router, ajeno, "presentations_list", map[string]any{"ingredient_id": a}))
		if len(got) != 0 {
			t.Fatalf("otra empresa ve: %v", got)
		}
	})
}
