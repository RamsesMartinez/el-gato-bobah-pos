//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/auth"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
)

// Las escrituras de la spec 074 por la puerta para agentes: empaques de compra y corregir un gasto
// guardado. Igual que las de la 073, se comparan con lo que deja la ruta de la pantalla.

// userWithPerms crea una persona de la empresa A con un rol propio que tiene SOLO esos permisos, y le
// pone la contraseña con la que se crean los tokens de prueba.
func userWithPerms(t *testing.T, w mcpWorld, name string, perms ...domain.Permission) int64 {
	t.Helper()
	ctx := context.Background()
	var role, uid int64
	if err := w.owner.Pool.QueryRow(ctx, `insert into roles (company_id, name) values ($1, $2) returning id`,
		w.company, name).Scan(&role); err != nil {
		t.Fatal(err)
	}
	for _, p := range perms {
		if _, err := w.owner.Pool.Exec(ctx, `insert into role_permission_overrides (company_id, role_id, permission_id, granted)
			select $1, $2, id, true from permissions where code = $3`, w.company, role, string(p)); err != nil {
			t.Fatal(err)
		}
	}
	h, err := auth.HashSecret(tokenPassword)
	if err != nil {
		t.Fatal(err)
	}
	if err := w.owner.Pool.QueryRow(ctx, `insert into users (company_id, name, username, role_id, password_hash)
		values ($1, $2::text, $2::text, $3, $4) returning id`, w.company, name, role, h).Scan(&uid); err != nil {
		t.Fatal(err)
	}
	return uid
}

// unitIDOf es el id de una unidad por su código, como entero de JSON.
func unitIDOf(t *testing.T, w mcpWorld, code string) int64 {
	t.Helper()
	return scalar(t, w, `select id::bigint from units where code = $1`, code)
}

func presentationRow(t *testing.T, w mcpWorld, id int64) (name string, qty decimal.Decimal, brand *string, active bool, unit *int16) {
	t.Helper()
	if err := w.owner.Pool.QueryRow(context.Background(), `select name, qty_in_base, brand, active, purchase_unit_id
		from ingredient_presentations where id = $1`, id).Scan(&name, &qty, &brand, &active, &unit); err != nil {
		t.Fatal(err)
	}
	return
}

// resultID saca result.id de una escritura aplicada.
func resultID(t *testing.T, res rpcResult) int64 {
	t.Helper()
	r, _ := res.data["result"].(map[string]any)
	id, _ := r["id"].(float64)
	if id == 0 {
		t.Fatalf("sin id en el resultado: %s", res.text)
	}
	return int64(id)
}

// ---- US2: empaques ----

func TestMCPPresentations(t *testing.T) {
	t.Parallel()
	w := newMCPWorld(t)
	ctx := context.Background()
	harina := ingredientIn(t, w.owner, w.company, "Harina pastel MCP", "g")
	grams := unitIDOf(t, w, "g")
	tok := w.token(t, w.company, w.admin, "write:inventory")
	create := func(extra map[string]any) map[string]any {
		a := map[string]any{"ingredient_id": harina, "name": "Bolsa", "qty": "375", "unit_id": grams, "brand": "Betty Crocker"}
		for k, v := range extra {
			a[k] = v
		}
		return a
	}
	count := func() int64 {
		return scalar(t, w, `select count(*) from ingredient_presentations where ingredient_id = $1`, harina)
	}

	t.Run("la vista previa lista los empaques del insumo y no crea nada", func(t *testing.T) {
		res := callTool(t, w.router, tok, "presentation_create", create(nil))
		if res.isError || res.data["preview"] != true {
			t.Fatalf("vista previa: %s", res.text)
		}
		if _, ok := res.data["before"]; !ok {
			t.Fatalf("la vista previa no trae los empaques actuales: %s", res.text)
		}
		if count() != 0 {
			t.Fatal("la vista previa creó el empaque")
		}
	})

	var bolsa int64
	t.Run("diez veces la misma clave: un empaque con su marca, y la bitácora con antes y después", func(t *testing.T) {
		args := applyArgs(create(nil), "empaque-betty-0001")
		var res rpcResult
		for i := range 10 {
			if res = callTool(t, w.router, tok, "presentation_create", args); res.isError {
				t.Fatalf("intento %d: %s", i+1, res.text)
			}
		}
		if n := count(); n != 1 {
			t.Fatalf("hay %d empaques", n)
		}
		bolsa = resultID(t, res)
		name, qty, brand, _, _ := presentationRow(t, w, bolsa)
		if name != "Bolsa" || !qty.Equal(decimal.NewFromInt(375)) || brand == nil || *brand != "Betty Crocker" {
			t.Fatalf("empaque = %s %s %v", name, qty, brand)
		}
		var after string
		if err := w.owner.Pool.QueryRow(ctx, `select after::text from audit_events where action = 'mcp.tool_called'
			and after->>'tool' = 'presentation_create' and after->>'mode' = 'apply' and after ? 'entityAfter'
			order by id limit 1`).Scan(&after); err != nil {
			t.Fatalf("sin evento de la llamada aplicada: %v", err)
		}
		if !strings.Contains(after, "Betty Crocker") {
			t.Fatalf("el después del evento no trae el empaque: %s", after)
		}
	})

	t.Run("409: la misma marca y nombre en el mismo insumo", func(t *testing.T) {
		res := callTool(t, w.router, tok, "presentation_create", applyArgs(create(map[string]any{"brand": "betty crocker"}), "empaque-dup-0001"))
		if !res.isError || !strings.Contains(strings.ToLower(res.text), "ya hay un empaque") {
			t.Fatalf("marca repetida: %s", res.text)
		}
		if count() != 1 {
			t.Fatal("se creó el duplicado")
		}
	})

	t.Run("400: una marca de 41 letras", func(t *testing.T) {
		res := callTool(t, w.router, tok, "presentation_create", applyArgs(create(map[string]any{"name": "Caja",
			"brand": strings.Repeat("x", 41)}), "empaque-larga-01"))
		if !res.isError || count() != 1 {
			t.Fatalf("marca larga: %s", res.text)
		}
		if !strings.Contains(res.text, "40") && !strings.Contains(strings.ToLower(res.text), "marca") {
			t.Fatalf("el rechazo no dice qué corregir: %s", res.text)
		}
	})

	t.Run("editar solo la marca conserva nombre y contenido", func(t *testing.T) {
		res := callTool(t, w.router, tok, "presentation_update", applyArgs(map[string]any{"presentation_id": bolsa,
			"brand": "Betty Crocker Pro"}, "empaque-upd-0001"))
		if res.isError {
			t.Fatalf("editar marca: %s", res.text)
		}
		name, qty, brand, active, _ := presentationRow(t, w, bolsa)
		if name != "Bolsa" || !qty.Equal(decimal.NewFromInt(375)) || brand == nil || *brand != "Betty Crocker Pro" || !active {
			t.Fatalf("después de editar la marca: %s %s %v %v", name, qty, brand, active)
		}
	})

	t.Run("desactivar conserva la marca; brand vacío la quita", func(t *testing.T) {
		if res := callTool(t, w.router, tok, "presentation_update", applyArgs(map[string]any{"presentation_id": bolsa,
			"active": false}, "empaque-off-0001")); res.isError {
			t.Fatalf("desactivar: %s", res.text)
		}
		_, _, brand, active, _ := presentationRow(t, w, bolsa)
		if active || brand == nil {
			t.Fatalf("desactivar: activo=%v marca=%v", active, brand)
		}
		if res := callTool(t, w.router, tok, "presentation_update", applyArgs(map[string]any{"presentation_id": bolsa,
			"brand": "", "active": true}, "empaque-sinmarca1")); res.isError {
			t.Fatalf("quitar marca: %s", res.text)
		}
		if _, _, brand, _, _ := presentationRow(t, w, bolsa); brand != nil {
			t.Fatalf("brand vacío no quitó la marca: %v", *brand)
		}
	})

	t.Run("editar el nombre de un aproximado conserva su unidad de compra", func(t *testing.T) {
		huevo := ingredientIn(t, w.owner, w.company, "Huevo MCP", "pieza")
		kilo := unitIDOf(t, w, "kg")
		res := callTool(t, w.router, tok, "presentation_create", applyArgs(map[string]any{"ingredient_id": huevo, "name": "Kilo suelto",
			"qty": "16", "unit_id": kilo, "approximate": true}, "empaque-aprox-01"))
		if res.isError {
			t.Fatalf("crear aproximado: %s", res.text)
		}
		id := resultID(t, res)
		if res := callTool(t, w.router, tok, "presentation_update", applyArgs(map[string]any{"presentation_id": id,
			"name": "Kilo a granel"}, "empaque-aprox-02")); res.isError {
			t.Fatalf("renombrar aproximado: %s", res.text)
		}
		name, qty, _, _, unit := presentationRow(t, w, id)
		if name != "Kilo a granel" || !qty.Equal(decimal.NewFromInt(16)) || unit == nil || int64(*unit) != kilo {
			t.Fatalf("aproximado después de renombrar: %s %s unidad %v", name, qty, unit)
		}
	})

	t.Run("sin el alcance de almacén no se ofrece ninguna de las dos", func(t *testing.T) {
		cat := w.token(t, w.company, w.admin, "write:catalog")
		names := listTools(t, w.router, cat)
		if contains(names, "presentation_create") || contains(names, "presentation_update") {
			t.Fatalf("un token de menú ve empaques: %v", names)
		}
		res := callTool(t, w.router, cat, "presentation_create", applyArgs(create(map[string]any{"name": "Costal"}), "empaque-scope-01"))
		if !res.isError && len(res.rpcErr) == 0 {
			t.Fatalf("un token sin almacén creó un empaque: %s", res.result)
		}
	})

	t.Run("un rol sin configurar el almacén no las ve aunque el token tenga el alcance", func(t *testing.T) {
		uid := userWithPerms(t, w, "almacenista-mcp", domain.PermInventoryView, domain.PermInventoryReceive, domain.PermInventoryCount)
		tk := w.token(t, w.company, uid, "write:inventory")
		names := listTools(t, w.router, tk)
		if contains(names, "presentation_create") || contains(names, "presentation_update") {
			t.Fatalf("un rol sin inventory.configure ve empaques: %v", names)
		}
		res := callTool(t, w.router, tk, "presentation_update", applyArgs(map[string]any{"presentation_id": bolsa, "brand": "X"}, "empaque-perm-01"))
		if !res.isError && len(res.rpcErr) == 0 {
			t.Fatalf("un rol sin permiso editó un empaque: %s", res.result)
		}
	})

	t.Run("un token de otra empresa no edita este empaque", func(t *testing.T) {
		ajeno := w.token(t, w.other, w.otherAdm, "write:inventory")
		res := callTool(t, w.router, ajeno, "presentation_update", applyArgs(map[string]any{"presentation_id": bolsa,
			"brand": "Ajena"}, "empaque-ajeno-01"))
		if !res.isError || !strings.Contains(res.text, "No existe") {
			t.Fatalf("otra empresa: %s", res.text)
		}
		if _, _, brand, _, _ := presentationRow(t, w, bolsa); brand != nil {
			t.Fatalf("otra empresa le puso marca: %v", *brand)
		}
	})
}

func TestMCPPurchaseSuggestsCreatingThePresentation(t *testing.T) {
	t.Parallel()
	w := newMCPWorld(t)
	f := mcpSeed(t, w)
	harina := ingredientIn(t, w.owner, w.company, "Harina sugerida MCP", "g")
	if _, err := w.owner.Pool.Exec(context.Background(), `insert into ingredient_presentations (company_id, ingredient_id, name, qty_in_base, brand)
		values ($1, $2, 'Costal', 25000, 'Selecta')`, w.company, harina); err != nil {
		t.Fatal(err)
	}
	grams := unitIDOf(t, w, "g")
	tok := w.token(t, w.company, w.admin, "write:purchases")
	preview := func(line map[string]any) rpcResult {
		l := map[string]any{"name": "HARINA BETTY CROCKER 375G", "ingredient_id": harina, "quantity": "1", "amount": "42.00"}
		for k, v := range line {
			l[k] = v
		}
		res := callTool(t, w.router, tok, "purchase_create", map[string]any{"category_id": f.category, "amount": "42.00",
			"lines": []any{l}})
		if res.isError {
			t.Fatalf("vista previa: %s", res.text)
		}
		return res
	}

	t.Run("sin empaque: los del insumo con su marca y cómo crear el que falta", func(t *testing.T) {
		res := preview(nil)
		for _, want := range []string{"Selecta", "create_presentation", "presentation_create", `"ingredient_id":` + strconv.FormatInt(harina, 10),
			`"unit_id":` + strconv.FormatInt(grams, 10), "brand", "qty"} {
			if !strings.Contains(compact(res.text), want) {
				t.Errorf("falta %q en la vista previa: %s", want, res.text)
			}
		}
	})

	t.Run("con unidad y sin empaque: avisa que entra sin marca ni empaque", func(t *testing.T) {
		res := preview(map[string]any{"unit_id": grams, "quantity": "375"})
		if !strings.Contains(res.text, "sin marca ni empaque") || !strings.Contains(res.text, "create_presentation") {
			t.Fatalf("sin aviso de la unidad: %s", res.text)
		}
	})
}

// compact quita espacios de un JSON para buscar pares llave:valor sin depender del formato.
func compact(s string) string {
	return strings.Join(strings.Fields(s), "")
}

// ---- US1: corregir una compra ----

// purchaseInGrams captura por MCP una compra recibida de un renglón y devuelve el gasto y el renglón.
func purchaseLine(t *testing.T, w mcpWorld, tok string, f mcpFixtures, key string, line map[string]any, extra map[string]any) (int64, int64) {
	t.Helper()
	args := map[string]any{"category_id": f.category, "amount": line["amount"], "lines": []any{line}}
	for k, v := range extra {
		args[k] = v
	}
	res := callTool(t, w.router, tok, "purchase_create", applyArgs(args, key))
	if res.isError {
		t.Fatalf("compra %s: %s", key, res.text)
	}
	exp := resultID(t, res)
	item := scalar(t, w, `select id from expense_items where expense_id = $1 and voided_at is null`, exp)
	return exp, item
}

func currentCost(t *testing.T, w mcpWorld, ing int64) decimal.Decimal {
	t.Helper()
	var c decimal.Decimal
	if err := w.owner.Pool.QueryRow(context.Background(), `select current_cost from ingredients where id = $1`, ing).Scan(&c); err != nil {
		t.Fatal(err)
	}
	return c
}

func editPreview(t *testing.T, w mcpWorld, tok string, args map[string]any) (fingerprint string, res rpcResult) {
	t.Helper()
	res = callTool(t, w.router, tok, "expense_edit", args)
	if res.isError || res.data["preview"] != true {
		t.Fatalf("vista previa de la edición: %s", res.text)
	}
	var body struct {
		Resolved struct {
			Preview struct {
				Fingerprint string `json:"fingerprint"`
			} `json:"preview"`
		} `json:"resolved"`
	}
	b, _ := json.Marshal(res.data)
	_ = json.Unmarshal(b, &body)
	if body.Resolved.Preview.Fingerprint == "" {
		t.Fatalf("la vista previa no trae fingerprint: %s", res.text)
	}
	return body.Resolved.Preview.Fingerprint, res
}

func withArgs(base map[string]any, extra map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range base {
		out[k] = v
	}
	for k, v := range extra {
		out[k] = v
	}
	return out
}

func TestMCPExpenseEditFixesAPurchase(t *testing.T) {
	t.Parallel()
	w := newMCPWorld(t)
	f := mcpSeed(t, w)
	ctx := context.Background()
	grams := unitIDOf(t, w, "g")
	tok := w.token(t, w.company, w.admin, "write:purchases", "write:inventory", "write:expenses")

	// Dos insumos gemelos: el de referencia se compra bien desde el principio; el otro, en gramos y
	// corregido después. Al final tienen que quedar iguales.
	ref := ingredientIn(t, w.owner, w.company, "Harina referencia MCP", "g")
	fixed := ingredientIn(t, w.owner, w.company, "Harina corregida MCP", "g")
	var refBag int64
	if err := w.owner.Pool.QueryRow(ctx, `insert into ingredient_presentations (company_id, ingredient_id, name, qty_in_base, brand)
		values ($1, $2, 'Bolsa', 375, 'Betty Crocker') returning id`, w.company, ref).Scan(&refBag); err != nil {
		t.Fatal(err)
	}
	purchaseLine(t, w, tok, f, "ref-compra-0001",
		map[string]any{"name": "HARINA BETTY CROCKER 375G", "ingredient_id": ref, "presentation_id": refBag, "quantity": "1", "amount": "42.00"}, nil)
	exp, gramsLine := purchaseLine(t, w, tok, f, "mal-compra-0001",
		map[string]any{"name": "HARINA BETTY CROCKER 375G", "ingredient_id": fixed, "unit_id": grams, "quantity": "375", "amount": "42.00"}, nil)
	var arrived string
	if err := w.owner.Pool.QueryRow(ctx, `select effective_on::text from stock_movements where expense_item_id = $1 limit 1`, gramsLine).Scan(&arrived); err != nil {
		t.Fatal(err)
	}

	res := callTool(t, w.router, tok, "presentation_create", applyArgs(map[string]any{"ingredient_id": fixed, "name": "Bolsa",
		"qty": "375", "unit_id": grams, "brand": "Betty Crocker"}, "empaque-fix-0001"))
	if res.isError {
		t.Fatalf("crear empaque: %s", res.text)
	}
	bag := resultID(t, res)

	edit := map[string]any{"expense_id": exp, "remove": []any{gramsLine}, "add": []any{map[string]any{
		"name": "HARINA BETTY CROCKER 375G", "ingredient_id": fixed, "presentation_id": bag, "quantity": "1",
		"amount": "42.00", "arrived_on": arrived}}}

	stockBefore := onHandIngredient(t, w.owner, fixed)
	fp, prev := editPreview(t, w, tok, edit)
	t.Run("la vista previa dice existencias, costo e importe y no cambia nada", func(t *testing.T) {
		for _, want := range []string{"inventory", "costs", "money", "amountBefore", "statusAfter"} {
			if !strings.Contains(prev.text, want) {
				t.Errorf("falta %s en la vista previa: %s", want, prev.text)
			}
		}
		if n := scalar(t, w, `select count(*) from expense_items where expense_id = $1 and voided_at is null and presentation_id = $2`, exp, bag); n != 0 {
			t.Fatal("la vista previa agregó el renglón")
		}
		if !onHandIngredient(t, w.owner, fixed).Equal(stockBefore) {
			t.Fatal("la vista previa movió el almacén")
		}
	})

	t.Run("aplicar sin fingerprint se rechaza antes de despachar", func(t *testing.T) {
		res := callTool(t, w.router, tok, "expense_edit", applyArgs(edit, "editar-sin-fp-01"))
		if !res.isError || !strings.Contains(res.text, "fingerprint") {
			t.Fatalf("sin huella: %s", res.text)
		}
	})

	t.Run("diez veces la misma clave: una edición, y queda como la compra bien capturada", func(t *testing.T) {
		args := applyArgs(withArgs(edit, map[string]any{"fingerprint": fp}), "editar-harina-01")
		for i := range 10 {
			if res := callTool(t, w.router, tok, "expense_edit", args); res.isError {
				t.Fatalf("intento %d: %s", i+1, res.text)
			}
		}
		if n := scalar(t, w, `select count(*) from expense_items where expense_id = $1 and voided_at is null`, exp); n != 1 {
			t.Fatalf("la compra tiene %d renglones vivos", n)
		}
		var pres *int64
		var brand *string
		if err := w.owner.Pool.QueryRow(ctx, `select presentation_id, presentation_brand from expense_items
			where expense_id = $1 and voided_at is null`, exp).Scan(&pres, &brand); err != nil {
			t.Fatal(err)
		}
		if pres == nil || *pres != bag || brand == nil || *brand != "Betty Crocker" {
			t.Fatalf("el renglón quedó con empaque %v y marca %v", pres, brand)
		}
		if a, b := onHandIngredient(t, w.owner, fixed), onHandIngredient(t, w.owner, ref); !a.Equal(b) {
			t.Fatalf("existencia corregida %s, de referencia %s", a, b)
		}
		if a, b := currentCost(t, w, fixed), currentCost(t, w, ref); !a.Equal(b) {
			t.Fatalf("costo corregido %s, de referencia %s", a, b)
		}
		var amount decimal.Decimal
		var status string
		if err := w.owner.Pool.QueryRow(ctx, `select amount, status::text from expenses where id = $1`, exp).Scan(&amount, &status); err != nil {
			t.Fatal(err)
		}
		if !amount.Equal(decimal.RequireFromString("42.00")) {
			t.Fatalf("el importe cambió a %s", amount)
		}
		var device string
		_ = w.owner.Pool.QueryRow(ctx, `select coalesce(device::text, '') from audit_events where action = 'expense.edited'
			and entity_id = $1 order by id desc limit 1`, exp).Scan(&device)
		if !strings.Contains(device, `"agent": "mcp"`) {
			t.Fatalf("el evento de la edición no dice que vino del agente: %s", device)
		}
	})

	t.Run("huella vencida: se rechaza, nada cambia, y la misma clave sirve con la huella nueva", func(t *testing.T) {
		other, _ := purchaseLine(t, w, tok, f, "mal-compra-0002",
			map[string]any{"name": "HARINA OTRA", "ingredient_id": fixed, "unit_id": grams, "quantity": "100", "amount": "10.00"}, nil)
		e := map[string]any{"expense_id": other, "description": "corregida por MCP"}
		old, _ := editPreview(t, w, tok, e)
		// Alguien cambia el gasto entre la vista previa y la aplicación.
		if _, err := w.owner.Pool.Exec(ctx, `update expenses set amount = amount + 1 where id = $1`, other); err != nil {
			t.Fatal(err)
		}
		res := callTool(t, w.router, tok, "expense_edit", applyArgs(withArgs(e, map[string]any{"fingerprint": old}), "editar-vencida-01"))
		if !res.isError || !strings.Contains(res.text, "vista previa") {
			t.Fatalf("huella vencida: %s", res.text)
		}
		if n := scalar(t, w, `select count(*) from expenses where id = $1 and description is null`, other); n != 1 {
			t.Fatal("una huella vencida cambió algo")
		}
		fresh, _ := editPreview(t, w, tok, e)
		if res := callTool(t, w.router, tok, "expense_edit", applyArgs(withArgs(e, map[string]any{"fingerprint": fresh}), "editar-vencida-01")); res.isError {
			t.Fatalf("la misma clave con la huella nueva: %s", res.text)
		}
	})

	t.Run("decisión faltante: se rechaza nombrándola y con keep aplica", func(t *testing.T) {
		back := time.Now().AddDate(0, 0, -3).Format(time.DateOnly)
		other, line := purchaseLine(t, w, tok, f, "vieja-compra-001",
			map[string]any{"name": "HARINA VIEJA", "ingredient_id": fixed, "unit_id": grams, "quantity": "50", "amount": "5.00"},
			map[string]any{"document_date": back, "received_on": back})
		var countID int64
		if err := w.owner.Pool.QueryRow(ctx, `insert into stock_counts (company_id, branch_id, opened_by, status, closed_by, closed_at, opened_at)
			select $1, id, $2, 'closed', $2, now() - interval '1 hour', now() - interval '1 hour' from branches where company_id = $1 order by id limit 1 returning id`,
			w.company, w.admin).Scan(&countID); err != nil {
			t.Fatal(err)
		}
		if _, err := w.owner.Pool.Exec(ctx, `insert into stock_count_lines (count_id, company_id, ingredient_id, counted_qty, system_qty, captured_by, captured_at)
			values ($1, $2, $3, 0, 0, $4, now() - interval '1 hour')`, countID, w.company, fixed, w.admin); err != nil {
			t.Fatal(err)
		}
		// Se reemplaza el renglón (quitarlo solo dejaría el gasto en $0, que la ruta rechaza); lo que llega
		// hoy nunca pregunta, lo que se quita de hace tres días sí.
		e := map[string]any{"expense_id": other, "remove": []any{line}, "add": []any{map[string]any{"name": "HARINA VIEJA",
			"ingredient_id": fixed, "unit_id": grams, "quantity": "50", "amount": "5.00", "arrived_on": arrived}}}
		fp, prev := editPreview(t, w, tok, e)
		key := "count:" + strconv.FormatInt(fixed, 10)
		if !strings.Contains(prev.text, key) {
			t.Fatalf("la vista previa no pide la decisión del conteo: %s", prev.text)
		}
		res := callTool(t, w.router, tok, "expense_edit", applyArgs(withArgs(e, map[string]any{"fingerprint": fp}), "editar-decision-1"))
		if !res.isError || !strings.Contains(res.text, "decisions") {
			t.Fatalf("sin decisión: %s", res.text)
		}
		if n := scalar(t, w, `select count(*) from expense_items where id = $1 and voided_at is null`, line); n != 1 {
			t.Fatal("aplicó sin la decisión")
		}
		e["decisions"] = map[string]any{key: "keep"}
		fp, _ = editPreview(t, w, tok, e)
		if res := callTool(t, w.router, tok, "expense_edit", applyArgs(withArgs(e, map[string]any{"fingerprint": fp}), "editar-decision-2")); res.isError {
			t.Fatalf("con keep: %s", res.text)
		}
	})
}

func TestMCPExpenseEditGates(t *testing.T) {
	t.Parallel()
	w := newMCPWorld(t)
	f := mcpSeed(t, w)
	grams := unitIDOf(t, w, "g")
	admin := w.token(t, w.company, w.admin, "write:purchases", "write:expenses")
	azucar := ingredientIn(t, w.owner, w.company, "Azúcar gates MCP", "g")
	exp, line := purchaseLine(t, w, admin, f, "gates-compra-001",
		map[string]any{"name": "AZUCAR", "ingredient_id": azucar, "unit_id": grams, "quantity": "1000", "amount": "30.00"}, nil)
	remove := map[string]any{"expense_id": exp, "remove": []any{line}}

	t.Run("un token de compras no la ve", func(t *testing.T) {
		if contains(listTools(t, w.router, w.token(t, w.company, w.admin, "write:purchases")), "expense_edit") {
			t.Fatal("un token sin gastos ve expense_edit")
		}
	})

	t.Run("un rol sin editar gastos no la ve aunque el token tenga el alcance", func(t *testing.T) {
		uid := userWithPerms(t, w, "capturista-mcp", domain.PermExpensesCapture)
		tk := w.token(t, w.company, uid, "write:expenses")
		if contains(listTools(t, w.router, tk), "expense_edit") {
			t.Fatal("un rol sin expenses.edit ve expense_edit")
		}
		res := callTool(t, w.router, tk, "expense_edit", applyArgs(withArgs(remove, map[string]any{"fingerprint": "x"}), "gates-perm-0001"))
		if !res.isError && len(res.rpcErr) == 0 {
			t.Fatalf("un rol sin permiso editó: %s", res.result)
		}
	})

	t.Run("con editar y sin recibir, quitar lo que ya sumó se rechaza nombrando el permiso", func(t *testing.T) {
		uid := userWithPerms(t, w, "editor-mcp", domain.PermExpensesCapture, domain.PermExpensesEdit)
		tk := w.token(t, w.company, uid, "write:expenses")
		res := callTool(t, w.router, tk, "expense_edit", remove)
		if !res.isError {
			// La ruta corre la edición también en la vista previa: el rechazo puede llegar ahí o al aplicar.
			fp, _ := editPreview(t, w, tk, remove)
			res = callTool(t, w.router, tk, "expense_edit", applyArgs(withArgs(remove, map[string]any{"fingerprint": fp}), "gates-recv-0001"))
		}
		if !res.isError || !strings.Contains(res.text, "lo que llega al almacén") {
			t.Fatalf("sin permiso de recibir: %s", res.text)
		}
		if n := scalar(t, w, `select count(*) from expense_items where id = $1 and voided_at is null`, line); n != 1 {
			t.Fatal("quitó el renglón sin permiso de recibir")
		}
	})

	t.Run("un token de otra empresa no edita este gasto", func(t *testing.T) {
		ajeno := w.token(t, w.other, w.otherAdm, "write:expenses")
		res := callTool(t, w.router, ajeno, "expense_edit", remove)
		if !res.isError || !strings.Contains(res.text, "No existe") {
			t.Fatalf("otra empresa: %s", res.text)
		}
	})

	t.Run("arrived_on con forma inválida se rechaza en el esquema", func(t *testing.T) {
		res := callTool(t, w.router, admin, "expense_edit", map[string]any{"expense_id": exp, "add": []any{map[string]any{
			"name": "AZUCAR", "ingredient_id": azucar, "unit_id": grams, "quantity": "1", "amount": "1.00", "arrived_on": "11/11/2026"}}})
		if !res.isError && len(res.rpcErr) == 0 {
			t.Fatalf("una fecha mal escrita pasó: %s", res.text)
		}
	})

	t.Run("sin arrived_on el renglón agregado queda por recibir y la vista previa lo dice", func(t *testing.T) {
		before := onHandIngredient(t, w.owner, azucar)
		add := map[string]any{"expense_id": exp, "add": []any{map[string]any{"name": "AZUCAR EXTRA", "ingredient_id": azucar,
			"unit_id": grams, "quantity": "500", "amount": "0", "is_gift": true}}}
		fp, prev := editPreview(t, w, admin, add)
		if !strings.Contains(prev.text, "por recibir") {
			t.Fatalf("la vista previa no avisa que queda por recibir: %s", prev.text)
		}
		if res := callTool(t, w.router, admin, "expense_edit", applyArgs(withArgs(add, map[string]any{"fingerprint": fp}), "gates-pend-0001")); res.isError {
			t.Fatalf("agregar por recibir: %s", res.text)
		}
		if !onHandIngredient(t, w.owner, azucar).Equal(before) {
			t.Fatal("un renglón por recibir sumó al almacén")
		}
	})
}

// ---- US3: encabezado y renglones sin mercancía ----

func TestMCPExpenseEditHeader(t *testing.T) {
	t.Parallel()
	w := newMCPWorld(t)
	f := mcpSeed(t, w)
	ctx := context.Background()
	tok := w.token(t, w.company, w.admin, "write:expenses")
	var walmart, costco int64
	for name, dst := range map[string]*int64{"Walmart MCP 074": &walmart, "Costco MCP 074": &costco} {
		if err := w.owner.Pool.QueryRow(ctx, `insert into suppliers (company_id, name) values ($1, $2) returning id`, w.company, name).Scan(dst); err != nil {
			t.Fatal(err)
		}
	}
	res := callTool(t, w.router, tok, "expense_create", applyArgs(map[string]any{"category_id": f.category, "amount": "100.00",
		"supplier_id": walmart, "description": "Gasto encabezado 074",
		"payments": []any{map[string]any{"method_id": f.card, "amount": "100.00"}}}, "gasto-header-0001"))
	if res.isError {
		t.Fatalf("gasto: %s", res.text)
	}
	exp := resultID(t, res)

	t.Run("cambiar proveedor muestra antes y después y lo aplica", func(t *testing.T) {
		e := map[string]any{"expense_id": exp, "supplier_id": costco}
		fp, prev := editPreview(t, w, tok, e)
		if !strings.Contains(prev.text, "Walmart MCP 074") || !strings.Contains(prev.text, "Costco MCP 074") {
			t.Fatalf("la vista previa no dice el antes y el después: %s", prev.text)
		}
		if res := callTool(t, w.router, tok, "expense_edit", applyArgs(withArgs(e, map[string]any{"fingerprint": fp}), "header-prov-0001")); res.isError {
			t.Fatalf("cambiar proveedor: %s", res.text)
		}
		if got := scalar(t, w, `select coalesce(supplier_id, 0) from expenses where id = $1`, exp); got != costco {
			t.Fatalf("proveedor = %d", got)
		}
	})

	t.Run("supplier_id y clear_supplier juntos se rechazan; clear_supplier lo quita", func(t *testing.T) {
		res := callTool(t, w.router, tok, "expense_edit", map[string]any{"expense_id": exp, "supplier_id": walmart, "clear_supplier": true})
		if !res.isError {
			t.Fatalf("los dos a la vez: %s", res.text)
		}
		e := map[string]any{"expense_id": exp, "clear_supplier": true}
		fp, _ := editPreview(t, w, tok, e)
		if res := callTool(t, w.router, tok, "expense_edit", applyArgs(withArgs(e, map[string]any{"fingerprint": fp}), "header-clear-001")); res.isError {
			t.Fatalf("quitar proveedor: %s", res.text)
		}
		if got := scalar(t, w, `select coalesce(supplier_id, 0) from expenses where id = $1`, exp); got != 0 {
			t.Fatalf("el proveedor sigue: %d", got)
		}
	})

	t.Run("un renglón sin mercancía en una compra pagada y desglosada pide la decisión del dinero y no toca los pagos", func(t *testing.T) {
		buyer := w.token(t, w.company, w.admin, "write:purchases")
		leche := ingredientIn(t, w.owner, w.company, "Leche encabezado MCP", "ml")
		paid, _ := purchaseLine(t, w, buyer, f, "header-compra-001",
			map[string]any{"name": "LECHE", "ingredient_id": leche, "unit_id": unitIDOf(t, w, "ml"), "quantity": "1000", "amount": "30.00"},
			map[string]any{"payments": []any{map[string]any{"method_id": f.card, "amount": "30.00"}}})
		pays := func() int64 { return scalar(t, w, `select count(*) from expense_payments where expense_id = $1`, paid) }
		e := map[string]any{"expense_id": paid, "add": []any{map[string]any{"name": "Servilletas", "quantity": "1", "amount": "15.00"}}}
		fp, prev := editPreview(t, w, tok, e)
		if !strings.Contains(prev.text, "Falta decidir money") {
			t.Fatalf("no pide la decisión del dinero: %s", prev.text)
		}
		res := callTool(t, w.router, tok, "expense_edit", applyArgs(withArgs(e, map[string]any{"fingerprint": fp}), "header-money-001"))
		if !res.isError || !strings.Contains(res.text, "decisions") {
			t.Fatalf("sin decidir el dinero: %s", res.text)
		}
		e["decisions"] = map[string]any{"money": "leave"}
		fp, _ = editPreview(t, w, tok, e)
		if res := callTool(t, w.router, tok, "expense_edit", applyArgs(withArgs(e, map[string]any{"fingerprint": fp}), "header-money-002")); res.isError {
			t.Fatalf("con leave: %s", res.text)
		}
		if got := scalar(t, w, `select (amount * 100)::bigint from expenses where id = $1`, paid); got != 4500 {
			t.Fatalf("importe = %d centavos", got)
		}
		if pays() != 1 {
			t.Fatal("la edición tocó los pagos")
		}
	})

	t.Run("un gasto anulado no se edita", func(t *testing.T) {
		if res := callTool(t, w.router, tok, "expense_cancel", applyArgs(map[string]any{"expense_id": exp, "reason": "prueba"}, "header-cancel-01")); res.isError {
			t.Fatalf("anular: %s", res.text)
		}
		res := callTool(t, w.router, tok, "expense_edit", map[string]any{"expense_id": exp, "description": "otra"})
		if !res.isError || !strings.Contains(res.text, "anulado") {
			t.Fatalf("editar un anulado: %s", res.text)
		}
	})
}
