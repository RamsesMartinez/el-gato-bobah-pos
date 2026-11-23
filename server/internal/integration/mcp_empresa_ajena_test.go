//go:build integration

package integration

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
)

// UN ID DE OTRA EMPRESA (#335). Con un token de la empresa B, cada herramienta que recibe ids se
// llama con los de la empresa A. Antes, `stock_adjust` decía en la vista previa que todo estaba bien
// y al aplicar respondía 500 «Error interno» sin causa —la FK compuesta frenaba el insert— y dejaba la
// llave de idempotencia en «unknown». Lo que se espera, en vista previa y al aplicar: un rechazo que
// nombra qué no existe, con el mismo texto que un id que no existe en ninguna empresa (no revela si
// existe en otra), y ninguna llave «unknown».
func TestMCPForeignCompanyIDsAreRejectedByName(t *testing.T) {
	t.Parallel()
	w := newMCPWorld(t)
	f := mcpSeed(t, w)
	ctx := context.Background()
	q := func(sql string, args ...any) int64 {
		t.Helper()
		var id int64
		if err := w.owner.Pool.QueryRow(ctx, sql, args...).Scan(&id); err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
		return id
	}
	// Todo lo de abajo es de la empresa A.
	prod := makeProduct(t, w.owner, "Producto de A 335", decimal.NewFromInt(50), true)
	cat := q(`select category_id from products where id = $1`, prod)
	ing := ingredientIn(t, w.owner, w.company, "Insumo de A 335", "g")
	pres := q(`insert into ingredient_presentations (company_id, ingredient_id, name, qty_in_base, tare_g)
		values ($1, $2, 'Bolsa 335', 1000, 25) returning id`, w.company, ing)
	group := q(`insert into modifier_groups (company_id, name) values ($1, 'Grupo de A 335') returning id`, w.company)
	opt := q(`insert into modifier_options (company_id, group_id, name) values ($1, $2, 'Extra de A 335') returning id`, w.company, group)
	sup := q(`insert into suppliers (company_id, name) values ($1, 'Proveedor de A 335') returning id`, w.company)
	reason := q(`insert into waste_reasons (company_id, name, description) values ($1, 'Motivo de A 335', 'se cayó') returning id`, w.company)
	count := q(`insert into stock_counts (company_id, branch_id, opened_by, status, opened_at)
		select $1, id, $2, 'open', now() from branches where company_id = $1 order by id limit 1 returning id`, w.company, w.admin)
	expense := q(`insert into expenses (company_id, category_id, amount, status, expense_date, created_by)
		values ($1, $2, 10, 'pendiente', current_date, $3) returning id`, w.company, f.category, w.admin)
	platform := q(`select id from delivery_platforms where company_id = $1 order by id limit 1`, w.company)
	warehouse := q(`select id from warehouses where company_id = $1 and is_sales limit 1`, w.company)
	order := crearPedidoSimple(t, tenantCtx(t, w.app, w.company, w.admin), app.NewOrdersService(w.app, nil), prod, w.admin)

	// Un token por cada pocas herramientas: el tope es de 120 llamadas por token y por minuto.
	newToken := func() string {
		return w.token(t, w.other, w.otherAdm, "read", "write:catalog", "write:inventory", "write:expenses",
			"write:purchases", "write:cash", "write:orders")
	}
	// B también necesita lo suyo para que el único id ajeno sea el que se prueba.
	bCat := q(`insert into expense_categories (company_id, name, financial_group) values ($1, 'Cat B 335', 'operacional') returning id`, w.other)
	bMethod := q(`select id from payment_methods where company_id = $1 and not affects_cash_drawer and is_active order by id limit 1`, w.other)
	bProd := q(`insert into products (company_id, name, category_id, price) values ($1, 'Producto de B 335',
		(select id from categories where company_id = $1 order by id limit 1), 10) returning id`, w.other)
	bIng := ingredientIn(t, w.owner, w.other, "Insumo de B 335", "g")
	gram := q(`select id from units where code = 'g'`)
	bReason := q(`insert into waste_reasons (company_id, name, description) values ($1, 'Motivo de B 335', 'se cayó') returning id`, w.other)
	ingCat := q(`insert into ingredient_categories (company_id, name) values ($1, 'Lácteos de A 335') returning id`, w.company)

	const missing = int64(987654321) // no existe en ninguna empresa
	type tc struct {
		tool string
		what string // lo que el rechazo debe nombrar
		args func(id int64) map[string]any
		id   int64
	}
	cases := []tc{
		{"stock_adjust", "producto", func(id int64) map[string]any {
			return map[string]any{"product_id": id, "quantity": "5", "reason": "conteo 335"}
		}, prod},
		{"stock_adjust", "insumo", func(id int64) map[string]any {
			return map[string]any{"ingredient_id": id, "quantity": "5", "reason": "conteo 335"}
		}, ing},
		{"stock_adjust", "almacén", func(id int64) map[string]any {
			return map[string]any{"ingredient_id": bIng, "quantity": "5", "reason": "conteo 335", "warehouse_id": id}
		}, warehouse},
		{"waste_record", "producto", func(id int64) map[string]any {
			return map[string]any{"reason_id": bReason, "product_id": id, "qty": "1"}
		}, prod},
		{"waste_record", "motivo", func(id int64) map[string]any {
			return map[string]any{"reason_id": id, "ingredient_id": bIng, "qty": "1"}
		}, reason},
		{"product_update", "producto", func(id int64) map[string]any {
			return map[string]any{"product_id": id, "price": "60"}
		}, prod},
		{"product_update", "categoría", func(id int64) map[string]any {
			return map[string]any{"product_id": bProd, "category_id": id}
		}, cat},
		{"modifier_option_update", "extra", func(id int64) map[string]any {
			return map[string]any{"option_id": id, "price_delta": "5"}
		}, opt},
		{"modifier_option_create", "grupo", func(id int64) map[string]any {
			return map[string]any{"group_id": id, "name": "Extra 335", "price_delta": "1"}
		}, group},
		{"modifier_group_update", "grupo", func(id int64) map[string]any {
			return map[string]any{"group_id": id, "name": "Robado 335"}
		}, group},
		{"product_group_link", "grupo", func(id int64) map[string]any {
			return map[string]any{"product_id": bProd, "group_id": id}
		}, group},
		{"product_create", "categoría", func(id int64) map[string]any {
			return map[string]any{"name": "Nuevo 335", "category_id": id, "price": "10"}
		}, cat},
		{"platform_price_set", "producto", func(id int64) map[string]any {
			return map[string]any{"platform_id": q(`select id from delivery_platforms where company_id = $1 order by id limit 1`, w.other),
				"product_id": id, "price": "70"}
		}, prod},
		{"presentation_create", "insumo", func(id int64) map[string]any {
			return map[string]any{"ingredient_id": id, "name": "Caja 335", "qty": "10", "unit_id": gram}
		}, ing},
		{"presentation_update", "empaque", func(id int64) map[string]any {
			return map[string]any{"presentation_id": id, "name": "Robada 335"}
		}, pres},
		{"ingredient_update", "insumo", func(id int64) map[string]any {
			return map[string]any{"ingredient_id": id, "name": "Robado 335"}
		}, ing},
		{"ingredient_cost_set", "insumo", func(id int64) map[string]any {
			return map[string]any{"ingredient_id": id, "cost": "3", "reason": "335"}
		}, ing},
		{"low_stock_update", "insumo", func(id int64) map[string]any {
			return map[string]any{"items": []any{map[string]any{"item_type": "ingrediente", "id": id, "enabled": true, "min_stock": "1"}}}
		}, ing},
		{"stock_count_capture", "conteo", func(id int64) map[string]any {
			return map[string]any{"count_id": id, "lines": []any{map[string]any{"ingredient_id": bIng, "counted_qty": "1"}}}
		}, count},
		{"stock_count_discard", "conteo", func(id int64) map[string]any { return map[string]any{"count_id": id} }, count},
		{"expense_create", "proveedor", func(id int64) map[string]any {
			return map[string]any{"category_id": bCat, "amount": "10", "supplier_id": id}
		}, sup},
		{"expense_create", "categoría", func(id int64) map[string]any {
			return map[string]any{"category_id": id, "amount": "10"}
		}, f.category},
		{"expense_pay", "gasto", func(id int64) map[string]any {
			return map[string]any{"expense_id": id, "method_id": bMethod, "amount": "10"}
		}, expense},
		{"expense_cancel", "gasto", func(id int64) map[string]any {
			return map[string]any{"expense_id": id, "reason": "335"}
		}, expense},
		{"purchase_create", "insumo", func(id int64) map[string]any {
			return map[string]any{"category_id": bCat, "amount": "10", "pending": true,
				"lines": []any{map[string]any{"name": "Insumo 335", "ingredient_id": id, "unit_id": gram, "quantity": "1", "amount": "10"}}}
		}, ing},
		{"cash_movement_create", "caja", func(id int64) map[string]any {
			return map[string]any{"register_id": id, "kind": "entrada", "amount": "10", "concept": "335"}
		}, f.register},
		{"order_cancel", "pedido", func(id int64) map[string]any {
			return map[string]any{"order_id": id, "reason": "335"}
		}, order},
		{"order_pay", "pedido", func(id int64) map[string]any {
			return map[string]any{"order_id": id, "method_id": bMethod, "all_remaining": true}
		}, order},
		{"order_deliver", "pedido", func(id int64) map[string]any { return map[string]any{"order_id": id} }, order},
		{"order_draft_create", "producto", func(id int64) map[string]any {
			return map[string]any{"service_type": "mostrador", "lines": []any{map[string]any{"product_id": id, "qty": "1"}}}
		}, prod},
		{"ingredient_create", "categoría", func(id int64) map[string]any {
			return map[string]any{"name": "Insumo nuevo 335", "base_unit_id": gram, "category_id": id}
		}, ingCat},
		{"recipe_set", "insumo", func(id int64) map[string]any {
			return map[string]any{"mode": "items", "product_id": bProd,
				"items": []any{map[string]any{"ingredient_id": id, "quantity": "1", "unit_id": gram}}}
		}, ing},
	}
	_ = platform

	report := []string{}
	var ajeno string
	for i, c := range cases {
		if i%20 == 0 {
			ajeno = newToken()
		}
		t.Run(fmt.Sprintf("%s con %s ajeno", c.tool, c.what), func(t *testing.T) {
			for _, mode := range []string{"vista previa", "aplicar"} {
				for _, id := range []int64{c.id, missing} {
					args := c.args(id)
					if mode == "aplicar" {
						args = applyArgs(args, fmt.Sprintf("ajena-%d-%d-%s", i, id, map[bool]string{true: "p", false: "a"}[mode == "vista previa"]))
					}
					res := callTool(t, w.router, ajeno, c.tool, args)
					txt := res.text
					report = append(report, fmt.Sprintf("%s | %s | %s | id=%d | isError=%v | %.160s", c.tool, c.what, mode, id, res.isError, txt))
					if !res.isError {
						t.Errorf("%s (id %d): no se rechazó: %.300s", mode, id, txt)
						continue
					}
					if strings.Contains(txt, "Error interno") {
						t.Errorf("%s (id %d): 500 sin causa: %s", mode, id, txt)
					}
					want := fmt.Sprintf("%d no existe en esta empresa", id)
					if !strings.Contains(txt, want) || !strings.Contains(strings.ToLower(txt), c.what) {
						t.Errorf("%s (id %d): el rechazo no dice «%s %s»: %s", mode, id, c.what, want, txt)
					}
				}
			}
		})
	}
	if n := scalar(t, w, `select count(*) from mcp_idempotency_keys where company_id = $1 and status = 'unknown'`, w.other); n != 0 {
		t.Errorf("quedaron %d llaves «unknown» por escrituras que no aplicaron nada", n)
	}
	t.Log("\n" + strings.Join(report, "\n"))
}

// #335: la revisión de referencias del MCP lee doce tablas de empresa. Bajo el rol de la aplicación,
// en los tres casos donde RLS ya falló, un id de la dueña sale como inexistente; con la sesión de la
// dueña sale como suyo, con su estado.
func TestFindReferencesIsIsolatedInTheThreeCases(t *testing.T) {
	t.Parallel()
	w := newMCPWorld(t)
	prod := makeProduct(t, w.owner, "Producto 335 aislado", decimal.NewFromInt(10), false)
	if _, err := w.owner.Pool.Exec(context.Background(), `update products set is_active = false where id = $1`, prod); err != nil {
		t.Fatal(err)
	}
	ing := ingredientIn(t, w.owner, w.company, "Insumo 335 aislado", "g")
	refs := []domain.Ref{{Kind: domain.RefProduct, ID: prod}, {Kind: domain.RefIngredient, ID: ing}}

	t.Run("la dueña los ve, con el producto inactivo", func(t *testing.T) {
		got, err := app.NewAPITokensService(w.app).FindReferences(tenantCtx(t, w.app, w.company, w.admin), refs)
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 2 || !got[0].Found || got[0].Active || got[0].Name != "Producto 335 aislado" || !got[1].Found {
			t.Fatalf("la dueña: %+v", got)
		}
	})
	inTheThreeCases(t, w.company, w.other, func(t *testing.T, st *store.Store, ctx context.Context) {
		got, err := app.NewAPITokensService(st).FindReferences(ctx, refs)
		if err != nil {
			// Sin empresa, fallar cerrado también es aislar.
			return
		}
		for _, r := range got {
			if r.Found {
				t.Fatalf("vio %s %d de la otra empresa", r.Kind, r.ID)
			}
		}
	})
}

// #335: un 500 le llega a quien llamó con el id del error —el de su llamada, el mismo del correo— y
// una causa por categoría, nunca el texto del error. Si nada se confirmó, la llave queda en «failed»
// con ese error y reintentar con la MISMA llave aplica una vez; si pudo confirmar, queda en «unknown».
func TestMCPInternalErrorTellsTheCallerAndTheKeyTellsTheTruth(t *testing.T) {
	t.Parallel()
	w := newMCPWorld(t)
	ctx := context.Background()
	ing := ingredientIn(t, w.owner, w.company, "Insumo 335 del 500", "g")
	tok := w.token(t, w.company, w.admin, "write:inventory")
	exec := func(sql string) {
		t.Helper()
		if _, err := w.owner.Pool.Exec(ctx, sql); err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
	}
	keyStatus := func(key string) (status, result string) {
		t.Helper()
		_ = w.owner.Pool.QueryRow(ctx, `select status, coalesce(result::text, '') from mcp_idempotency_keys where idem_key = $1`, key).
			Scan(&status, &result)
		return status, result
	}
	movements := func() int64 {
		return scalar(t, w, `select count(*) from stock_movements where ingredient_id = $1`, ing)
	}
	call := func(key, rid string) rpcResult {
		args := applyArgs(map[string]any{"ingredient_id": ing, "quantity": "5", "reason": "falla 335"}, key)
		return mcpRPC(t, w.router, tok, "tools/call", map[string]any{"name": "stock_adjust", "arguments": args},
			"X-Request-Id", rid)
	}

	t.Run("falla antes de confirmar: failed, con id y causa, y la misma llave reintenta", func(t *testing.T) {
		exec(`create function falla_335() returns trigger language plpgsql as $$
			begin raise exception 'detalle-interno-335' using errcode = 'XX335'; end $$`)
		exec(`create trigger falla_335 before insert on stock_movements for each row
			when (new.reason = 'falla 335') execute function falla_335()`)
		res := call("falla-335-antes", "id-del-error-335a")
		if !res.isError || !strings.Contains(res.text, "id-del-error-335a") || !strings.Contains(res.text, "base de datos rechazó") ||
			!strings.Contains(res.text, "No se aplicó nada") {
			t.Fatalf("el 500 no dice id, causa y que no se aplicó: %s", res.text)
		}
		if strings.Contains(res.text, "detalle-interno-335") {
			t.Fatalf("el texto interno llegó a quien llamó: %s", res.text)
		}
		if st, result := keyStatus("falla-335-antes"); st != "failed" || !strings.Contains(result, "id-del-error-335a") {
			t.Fatalf("la llave quedó en %q con %s; quería failed con el error", st, result)
		}
		if movements() != 0 {
			t.Fatal("se escribió un movimiento")
		}
		exec(`drop trigger falla_335 on stock_movements`)
		if res := call("falla-335-antes", "id-del-error-335b"); res.isError || res.data["applied"] != true {
			t.Fatalf("reintentar con la misma llave: %s", res.text)
		}
		if res := call("falla-335-antes", "id-del-error-335c"); res.isError || res.data["replayed"] != true {
			t.Fatalf("la tercera vez debía repetir lo guardado: %s", res.text)
		}
		if n := movements(); n != 1 {
			t.Fatalf("%d movimientos; quería uno", n)
		}
	})

	t.Run("falla al confirmar: pudo haberse aplicado, unknown", func(t *testing.T) {
		exec(`create constraint trigger falla_335_al_confirmar after insert on stock_movements
			deferrable initially deferred for each row when (new.reason = 'falla 335') execute function falla_335()`)
		t.Cleanup(func() { exec(`drop trigger if exists falla_335_al_confirmar on stock_movements`) })
		res := call("falla-335-confirmar", "id-del-error-335d")
		if !res.isError || !strings.Contains(res.text, "id-del-error-335d") || !strings.Contains(res.text, "pudo haberse aplicado") {
			t.Fatalf("el 500 al confirmar: %s", res.text)
		}
		if st, _ := keyStatus("falla-335-confirmar"); st != "unknown" {
			t.Fatalf("la llave quedó en %q; quería unknown", st)
		}
	})
}

// #335 (opcional del issue): cargar existencias de un producto inactivo avisa en la vista previa, porque
// stock_count_open no lo lista y confunde.
func TestMCPPreviewWarnsAboutAnInactiveProduct(t *testing.T) {
	t.Parallel()
	w := newMCPWorld(t)
	prod := makeProduct(t, w.owner, "Producto 335 apagado", decimal.NewFromInt(10), true)
	if _, err := w.owner.Pool.Exec(context.Background(), `update products set is_active = false where id = $1`, prod); err != nil {
		t.Fatal(err)
	}
	tok := w.token(t, w.company, w.admin, "write:inventory")
	res := callTool(t, w.router, tok, "stock_adjust", map[string]any{"product_id": prod, "quantity": "3", "reason": "llegó"})
	if res.isError || !strings.Contains(res.text, "inactivo") || !strings.Contains(res.text, "Producto 335 apagado") {
		t.Fatalf("la vista previa no avisa del producto inactivo: %s", res.text)
	}
}

// Revisión de código del #335: una herramienta de varios despachos que confirma el primero y recibe un
// 4xx en el segundo YA escribió. Su llave no puede quedar en «failed» —reintentar con la misma llave
// volvería a escribir la ficha—: queda en «unknown», con el mensaje de lo que sí quedó.
func TestMCPRejectionAfterAPartialWriteIsNotFailed(t *testing.T) {
	t.Parallel()
	w := newMCPWorld(t)
	ctx := context.Background()
	ing := ingredientIn(t, w.owner, w.company, "Queso 335", "g")
	otro := ingredientIn(t, w.owner, w.company, "Queso 335 de cabra", "g")
	if _, err := w.owner.Pool.Exec(ctx, `update ingredients set alt_names = '{"Queso fresco 335"}' where id = $1`, otro); err != nil {
		t.Fatal(err)
	}
	tok := w.token(t, w.company, w.admin, "write:inventory")
	res := callTool(t, w.router, tok, "ingredient_update", applyArgs(map[string]any{"ingredient_id": ing,
		"name": "Queso panela 335", "alt_names": []string{"Queso fresco 335"}}, "parcial-335-01"))
	if !res.isError || !strings.Contains(res.text, "ficha ya quedó cambiada") {
		t.Fatalf("se esperaba el rechazo de los otros nombres con la ficha guardada: %s", res.text)
	}
	var status string
	if err := w.owner.Pool.QueryRow(ctx, `select status from mcp_idempotency_keys where idem_key = 'parcial-335-01'`).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "unknown" {
		t.Fatalf("la llave quedó en %q tras escribir la ficha; un reintento la repetiría", status)
	}
}
