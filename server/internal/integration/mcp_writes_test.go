//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
)

// Las escrituras de la spec 073 por la puerta para agentes, contra la API completa servida como
// gatobobah_app. Cada una se compara con lo que deja la pantalla, no con una idea de lo que debería.

func applyArgs(args map[string]any, key string) map[string]any {
	out := map[string]any{"dry_run": false, "idempotency_key": key}
	for k, v := range args {
		out[k] = v
	}
	return out
}

func scalar(t *testing.T, w mcpWorld, sql string, args ...any) int64 {
	t.Helper()
	var n int64
	if err := w.owner.Pool.QueryRow(context.Background(), sql, args...).Scan(&n); err != nil {
		t.Fatalf("%s: %v", sql, err)
	}
	return n
}

// nonNullKey busca, a cualquier profundidad, una llave con valor no nulo. Así se prueba el arqueo
// ciego sobre la respuesta entera y no sobre el renglón que uno se acordó de revisar.
func nonNullKey(v any, key string) bool {
	switch x := v.(type) {
	case map[string]any:
		for k, c := range x {
			if k == key && c != nil {
				return true
			}
			if nonNullKey(c, key) {
				return true
			}
		}
	case []any:
		for _, c := range x {
			if nonNullKey(c, key) {
				return true
			}
		}
	}
	return false
}

func asJSON(v any) any {
	b, _ := json.Marshal(v)
	var out any
	_ = json.Unmarshal(b, &out)
	return out
}

// ---- US1: compras ----

func TestMCPPurchases(t *testing.T) {
	t.Parallel()
	w := newMCPWorld(t)
	f := mcpSeed(t, w)
	ctx := context.Background()
	azucar := ingredientIn(t, w.owner, w.company, "Azúcar MCP", "g")
	var bolsa int64
	if err := w.owner.Pool.QueryRow(ctx, `insert into ingredient_presentations (company_id, ingredient_id, name, qty_in_base)
		values ($1, $2, 'Bolsa 5 kg', 5000) returning id`, w.company, azucar).Scan(&bolsa); err != nil {
		t.Fatal(err)
	}
	tok := w.token(t, w.company, w.admin, "write:purchases")
	onHand := func() decimal.Decimal { return onHandIngredient(t, w.owner, azucar) }
	purchases := func(folio string) int64 {
		return scalar(t, w, `select count(*) from expenses where doc_folio = $1`, folio)
	}
	line := func(extra map[string]any) map[string]any {
		l := map[string]any{"name": "AZUCAR STD 5KG", "quantity": "2", "amount": "420.00"}
		for k, v := range extra {
			l[k] = v
		}
		return l
	}

	t.Run("vista previa con solo el nombre del ticket: candidatos y nada guardado", func(t *testing.T) {
		res := callTool(t, w.router, tok, "purchase_create", map[string]any{"category_id": f.category, "amount": "420.00",
			"doc_folio": "MCP-PREV", "lines": []any{line(nil)}})
		if res.isError || res.data["preview"] != true {
			t.Fatalf("vista previa: %s", res.text)
		}
		if !strings.Contains(res.text, "candidates") || !strings.Contains(res.text, "todavía no tiene insumo") {
			t.Fatalf("sin candidatos ni aviso: %s", res.text)
		}
		if purchases("MCP-PREV") != 0 {
			t.Fatal("la vista previa guardó la compra")
		}
	})

	t.Run("con insumo y empaque dice cómo cambia el costo", func(t *testing.T) {
		res := callTool(t, w.router, tok, "purchase_create", map[string]any{"category_id": f.category, "amount": "420.00",
			"doc_folio": "MCP-PREV", "lines": []any{line(map[string]any{"ingredient_id": azucar, "presentation_id": bolsa})}})
		if res.isError || !strings.Contains(res.text, "costAfter") || !strings.Contains(res.text, "0.042") {
			t.Fatalf("costo de la vista previa ($420 / 10 000 g = 0.042): %s", res.text)
		}
	})

	t.Run("aplicar un renglón sin empaque se rechaza nombrándolo", func(t *testing.T) {
		res := callTool(t, w.router, tok, "purchase_create", applyArgs(map[string]any{"category_id": f.category, "amount": "420.00",
			"doc_folio": "MCP-SIN-ID", "lines": []any{line(map[string]any{"ingredient_id": azucar})}}, "compra-sin-empaque-1"))
		if !res.isError || !strings.Contains(res.text, "renglón 1") || !strings.Contains(res.text, "presentation_id") {
			t.Fatalf("renglón incompleto: %s", res.text)
		}
		if purchases("MCP-SIN-ID") != 0 {
			t.Fatal("se guardó una compra con un renglón sin empaque")
		}
	})

	t.Run("diez veces la misma clave: una compra y el almacén sube una vez", func(t *testing.T) {
		before := onHand()
		args := applyArgs(map[string]any{"category_id": f.category, "amount": "420.00", "doc_folio": "MCP-COMPRA",
			"payments": []any{map[string]any{"method_id": f.cash, "amount": "420.00", "register_id": f.register}},
			"lines":    []any{line(map[string]any{"ingredient_id": azucar, "presentation_id": bolsa})}}, "compra-azucar-0001")
		for i := range 10 {
			if res := callTool(t, w.router, tok, "purchase_create", args); res.isError {
				t.Fatalf("intento %d: %s", i+1, res.text)
			}
		}
		if n := purchases("MCP-COMPRA"); n != 1 {
			t.Fatalf("hay %d compras", n)
		}
		if got := onHand().Sub(before); !got.Equal(decimal.NewFromInt(10000)) {
			t.Fatalf("el almacén subió %s g, esperaba 10000", got)
		}
		if n := scalar(t, w, `select count(*) from expense_payments p join expenses e on e.id = p.expense_id
			where e.doc_folio = 'MCP-COMPRA' and p.register_session_id = $1`, f.session); n != 1 {
			t.Fatalf("el pago en efectivo no quedó en el turno (%d)", n)
		}
		var device string
		_ = w.owner.Pool.QueryRow(ctx, `select coalesce(device::text, '') from audit_events a join expenses e on e.id = a.entity_id
			where a.action = 'expense.created' and e.doc_folio = 'MCP-COMPRA'`).Scan(&device)
		if !strings.Contains(device, `"agent": "mcp"`) {
			t.Fatalf("el evento no dice que vino del agente: %s", device)
		}
	})

	t.Run("una compra pendiente se recibe después", func(t *testing.T) {
		args := applyArgs(map[string]any{"category_id": f.category, "amount": "210.00", "doc_folio": "MCP-PEND", "pending": true,
			"lines": []any{line(map[string]any{"ingredient_id": azucar, "presentation_id": bolsa, "quantity": "1", "amount": "210.00"})}},
			"compra-pendiente-1")
		if res := callTool(t, w.router, tok, "purchase_create", args); res.isError {
			t.Fatalf("pendiente: %s", res.text)
		}
		before := onHand()
		if !onHand().Equal(before) {
			t.Fatal("una compra pendiente sumó al almacén")
		}
		id := scalar(t, w, `select id from expenses where doc_folio = 'MCP-PEND'`)
		res := callTool(t, w.router, tok, "purchase_receive", applyArgs(map[string]any{"expense_id": id}, "recibir-pend-1"))
		if res.isError {
			t.Fatalf("recibir: %s", res.text)
		}
		if got := onHand().Sub(before); !got.Equal(decimal.NewFromInt(5000)) {
			t.Fatalf("recibir sumó %s g", got)
		}
	})

	t.Run("sin el alcance de compras no hay compras", func(t *testing.T) {
		gastos := w.token(t, w.company, w.admin, "write:expenses")
		if contains(listTools(t, w.router, gastos), "purchase_create") {
			t.Fatal("un token de gastos ve purchase_create")
		}
	})

	t.Run("un token de otra empresa no recibe una compra ajena", func(t *testing.T) {
		ajeno := w.token(t, w.other, w.otherAdm, "write:purchases")
		id := scalar(t, w, `select id from expenses where doc_folio = 'MCP-COMPRA'`)
		res := callTool(t, w.router, ajeno, "purchase_receive", applyArgs(map[string]any{"expense_id": id}, "recibir-ajeno-1"))
		if !res.isError || !strings.Contains(res.text, "No existe") {
			t.Fatalf("otra empresa: %s", res.text)
		}
	})
}

// ---- US2: caja ----

func TestMCPCashCloseKeepsTheBlindCount(t *testing.T) {
	t.Parallel()
	w := newMCPWorld(t)
	f := mcpSeed(t, w)
	ctx := context.Background()
	setBlind := func(on bool) {
		if _, err := w.owner.Pool.Exec(ctx, `insert into business_settings (company_id, business_name, blind_cash_count)
			values ($1, 'Prueba', $2) on conflict (company_id) do update set blind_cash_count = excluded.blind_cash_count`, w.company, on); err != nil {
			t.Fatal(err)
		}
	}
	setBlind(true)
	tok := w.token(t, w.company, w.admin, "write:cash")
	// Una entrada de efectivo: el turno ya tiene un esperado que no es el fondo.
	if res := callTool(t, w.router, tok, "cash_movement_create", applyArgs(map[string]any{"register_id": f.register,
		"kind": "entrada", "amount": "50.00", "concept": "Cambio"}, "entrada-ciego-1")); res.isError {
		t.Fatal(res.text)
	}
	reg := map[string]any{"register_id": f.register}

	t.Run("ninguna herramienta de caja trae el esperado de un turno abierto", func(t *testing.T) {
		for _, c := range []struct {
			tool string
			args map[string]any
		}{
			{"cash_session_current", reg},
			{"cash_session_close", map[string]any{"register_id": f.register}},
			{"cash_movement_create", map[string]any{"register_id": f.register, "kind": "entrada", "amount": "1.00", "concept": "x"}},
		} {
			res := callTool(t, w.router, tok, c.tool, c.args)
			if res.isError {
				t.Fatalf("%s: %s", c.tool, res.text)
			}
			data := asJSON(res.data)
			for _, k := range []string{"expected", "difference", "ingresos", "plataformas"} {
				if k == "ingresos" || k == "plataformas" {
					if strings.Contains(res.text, `"`+k+`":[{`) {
						t.Errorf("%s trae %s con renglones: %s", c.tool, k, res.text)
					}
					continue
				}
				if k == "difference" {
					// Los totales por método traen la diferencia en cero mientras el turno está abierto; lo
					// que no puede viajar es la del cajón, que sale del esperado.
					if strings.Contains(res.text, `"drawer":{`) && !strings.Contains(res.text, `"difference":null`) {
						t.Errorf("%s trae la diferencia del cajón: %s", c.tool, res.text)
					}
					continue
				}
				if nonNullKey(data, k) {
					t.Errorf("%s trae %s con el arqueo ciego encendido: %s", c.tool, k, res.text)
				}
			}
		}
	})

	t.Run("la vista previa dice qué declarar, no cuánto", func(t *testing.T) {
		res := callTool(t, w.router, tok, "cash_session_close", map[string]any{"register_id": f.register})
		if res.isError || !strings.Contains(res.text, "to_declare") || !strings.Contains(res.text, `"drawer":true`) {
			t.Fatalf("vista previa: %s", res.text)
		}
	})

	var card string
	_ = w.owner.Pool.QueryRow(ctx, `select name from payment_methods where id = $1`, f.card).Scan(&card)
	// Lo que la vista previa pide declarar, como lo leería el agente: «Nombre (id N)».
	prev := callTool(t, w.router, tok, "cash_session_close", map[string]any{"register_id": f.register})
	all := map[string]any{}
	for _, m := range regexp.MustCompile(`\(id (\d+)\)`).FindAllStringSubmatch(prev.text, -1) {
		all[m[1]] = "0.00"
	}
	if _, ok := all[strconv.FormatInt(f.card, 10)]; !ok {
		t.Fatalf("la vista previa no pide la tarjeta: %s", prev.text)
	}
	allBut := func(skip int64) map[string]any {
		out := map[string]any{}
		for k, v := range all {
			if k != strconv.FormatInt(skip, 10) {
				out[k] = v
			}
		}
		return out
	}

	t.Run("sin la cifra de un método se rechaza nombrándolo y el turno sigue abierto", func(t *testing.T) {
		res := callTool(t, w.router, tok, "cash_session_close", applyArgs(map[string]any{"register_id": f.register,
			"counted_cash": "1050.00", "manual_reason": "contado a mano", "declared": allBut(f.card)}, "cierre-incompleto-1"))
		if !res.isError || !strings.Contains(res.text, card) || !strings.Contains(res.text, "nunca lo llenes") {
			t.Fatalf("cierre sin la tarjeta: %s", res.text)
		}
		if n := scalar(t, w, `select count(*) from register_sessions where id = $1 and status = 'abierta'`, f.session); n != 1 {
			t.Fatal("el turno se cerró sin una cifra")
		}
		res = callTool(t, w.router, tok, "cash_session_close", applyArgs(map[string]any{"register_id": f.register,
			"declared": all}, "cierre-sin-cajon-1"))
		if !res.isError || !strings.Contains(res.text, "cajón") {
			t.Fatalf("cierre sin el cajón: %s", res.text)
		}
	})

	t.Run("con todo dictado cierra, diez veces la misma clave es un cierre, y la diferencia llega después", func(t *testing.T) {
		args := applyArgs(map[string]any{"register_id": f.register, "counted_cash": "1040.00", "manual_reason": "contado a mano",
			"declared": all, "notes": "contó la cajera"}, "cierre-completo-1")
		var first rpcResult
		for i := range 10 {
			res := callTool(t, w.router, tok, "cash_session_close", args)
			if res.isError {
				t.Fatalf("intento %d: %s", i+1, res.text)
			}
			if i == 0 {
				first = res
			}
		}
		if n := scalar(t, w, `select count(*) from register_sessions where id = $1 and status = 'cerrada' and closed_by = $2`,
			f.session, w.admin); n != 1 {
			t.Fatal("el turno no quedó cerrado por la persona del token")
		}
		if !strings.Contains(first.text, "difference") {
			t.Fatalf("la respuesta del cierre no trae la diferencia: %s", first.text)
		}
	})

	t.Run("abrir pide la entrada de quien abre, como la pantalla", func(t *testing.T) {
		enableAttendance(t, w.owner, w.company, true)
		res := callTool(t, w.router, tok, "cash_session_open", applyArgs(map[string]any{"register_id": f.register,
			"opening_cash": "1040.00", "manual_reason": "contado a mano"}, "abrir-sin-entrada-1"))
		if !res.isError || !strings.Contains(res.text, "entrada") {
			t.Fatalf("abrir sin entrada: %s", res.text)
		}
		enableAttendance(t, w.owner, w.company, false)
	})

	t.Run("abrir con el fondo contado, diez veces la misma clave es un turno", func(t *testing.T) {
		args := applyArgs(map[string]any{"register_id": f.register, "opening_cash": "1040.00", "manual_reason": "contado a mano"}, "abrir-fondo-1")
		for i := range 10 {
			if res := callTool(t, w.router, tok, "cash_session_open", args); res.isError {
				t.Fatalf("intento %d: %s", i+1, res.text)
			}
		}
		if n := scalar(t, w, `select count(*) from register_sessions where register_id = $1 and status = 'abierta' and opened_by = $2`,
			f.register, w.admin); n != 1 {
			t.Fatalf("turnos abiertos: %d", n)
		}
	})

	t.Run("con el arqueo ciego apagado el esperado sí viaja, igual que en la pantalla", func(t *testing.T) {
		setBlind(false)
		res := callTool(t, w.router, tok, "cash_session_current", reg)
		if res.isError || !nonNullKey(asJSON(res.data), "expected") {
			t.Fatalf("sin arqueo ciego: %s", res.text)
		}
	})

	t.Run("otra empresa no cierra esta caja", func(t *testing.T) {
		ajeno := w.token(t, w.other, w.otherAdm, "write:cash")
		res := callTool(t, w.router, ajeno, "cash_session_close", applyArgs(map[string]any{"register_id": f.register,
			"counted_cash": "1.00", "manual_reason": "x"}, "cierre-ajeno-1"))
		if !res.isError {
			t.Fatalf("otra empresa cerró la caja: %s", res.text)
		}
	})
}

// ---- US3: conteos ----

func TestMCPCounts(t *testing.T) {
	t.Parallel()
	w := newMCPWorld(t)
	ctx := context.Background()
	leche := ingredientIn(t, w.owner, w.company, "Leche conteo MCP", "ml")
	vasos := ingredientIn(t, w.owner, w.company, "Vasos conteo MCP", "pieza")
	adjust(t, w.owner, "ingrediente", leche, "10")
	adjust(t, w.owner, "ingrediente", vasos, "340")
	tok := w.token(t, w.company, w.admin, "write:inventory")

	res := callTool(t, w.router, tok, "stock_count_open", applyArgs(nil, "abrir-conteo-1"))
	if res.isError {
		t.Fatalf("abrir: %s", res.text)
	}
	count := scalar(t, w, `select id from stock_counts where status = 'open' and company_id = $1`, w.company)

	t.Run("con uno abierto no se abre otro", func(t *testing.T) {
		if res := callTool(t, w.router, tok, "stock_count_open", applyArgs(nil, "abrir-conteo-2")); !res.isError {
			t.Fatalf("abrió dos: %s", res.text)
		}
	})

	t.Run("el lote se captura a ciegas y repetido no duplica", func(t *testing.T) {
		args := applyArgs(map[string]any{"count_id": count, "lines": []any{
			map[string]any{"ingredient_id": leche, "counted_qty": "6"},
			map[string]any{"ingredient_id": vasos, "counted_qty": "330"},
		}}, "captura-lote-1")
		for i := range 10 {
			res := callTool(t, w.router, tok, "stock_count_capture", args)
			if res.isError {
				t.Fatalf("intento %d: %s", i+1, res.text)
			}
			if strings.Contains(res.text, `"system"`) || strings.Contains(res.text, "340") {
				t.Fatalf("la captura dice lo que el sistema tiene: %s", res.text)
			}
		}
		if n := scalar(t, w, `select count(*) from stock_count_lines where count_id = $1`, count); n != 2 {
			t.Fatalf("renglones: %d", n)
		}
	})

	t.Run("una cifra fuera de lo de costumbre detiene el lote y pide confirmarla sin decir contra qué", func(t *testing.T) {
		res := callTool(t, w.router, tok, "stock_count_capture", applyArgs(map[string]any{"count_id": count, "lines": []any{
			map[string]any{"ingredient_id": leche, "counted_qty": "6"},
			map[string]any{"ingredient_id": vasos, "counted_qty": "900000"},
		}}, "captura-rara-1"))
		if !res.isError || !strings.Contains(res.text, "confirmed: true") || !strings.Contains(res.text, "Renglón 2") {
			t.Fatalf("cifra rara: %s", res.text)
		}
		if strings.Contains(res.text, "340") {
			t.Fatalf("el aviso dice la referencia: %s", res.text)
		}
		res = callTool(t, w.router, tok, "stock_count_capture", applyArgs(map[string]any{"count_id": count, "lines": []any{
			map[string]any{"ingredient_id": vasos, "counted_qty": "330", "confirmed": true},
		}}, "captura-confirmada-1"))
		if res.isError {
			t.Fatalf("confirmada: %s", res.text)
		}
	})

	t.Run("el mismo artículo dos veces en un lote se rechaza", func(t *testing.T) {
		res := callTool(t, w.router, tok, "stock_count_capture", map[string]any{"count_id": count, "lines": []any{
			map[string]any{"ingredient_id": leche, "counted_qty": "6"},
			map[string]any{"ingredient_id": leche, "counted_qty": "60"}}})
		if !res.isError || !strings.Contains(res.text, "ya viene en este lote") {
			t.Fatalf("artículo repetido: %s", res.text)
		}
	})

	t.Run("más de 50 renglones se rechaza", func(t *testing.T) {
		lines := make([]any, 51)
		for i := range lines {
			lines[i] = map[string]any{"ingredient_id": leche, "counted_qty": "1"}
		}
		if res := callTool(t, w.router, tok, "stock_count_capture", map[string]any{"count_id": count, "lines": lines}); !res.isError {
			t.Fatalf("51 renglones: %s", res.text)
		}
	})

	t.Run("la vista previa del cierre trae lo que el sistema tiene y el ajuste; cerrar lo aplica", func(t *testing.T) {
		res := callTool(t, w.router, tok, "stock_count_close", map[string]any{"count_id": count})
		if res.isError || !strings.Contains(res.text, `"system"`) || !strings.Contains(res.text, `"delta"`) {
			t.Fatalf("vista previa del cierre: %s", res.text)
		}
		if res := callTool(t, w.router, tok, "stock_count_close", applyArgs(map[string]any{"count_id": count}, "cerrar-conteo-1")); res.isError {
			t.Fatalf("cerrar: %s", res.text)
		}
		if got := onHandIngredient(t, w.owner, leche); !got.Equal(decimal.NewFromInt(6)) {
			t.Fatalf("la leche quedó en %s", got)
		}
	})

	t.Run("descartar no toca el almacén", func(t *testing.T) {
		if res := callTool(t, w.router, tok, "stock_count_open", applyArgs(nil, "abrir-conteo-3")); res.isError {
			t.Fatal(res.text)
		}
		other := scalar(t, w, `select id from stock_counts where status = 'open' and company_id = $1`, w.company)
		_ = callTool(t, w.router, tok, "stock_count_capture", applyArgs(map[string]any{"count_id": other, "lines": []any{
			map[string]any{"ingredient_id": leche, "counted_qty": "1"}}}, "captura-descartar-1"))
		if res := callTool(t, w.router, tok, "stock_count_discard", applyArgs(map[string]any{"count_id": other}, "descartar-1")); res.isError {
			t.Fatalf("descartar: %s", res.text)
		}
		if got := onHandIngredient(t, w.owner, leche); !got.Equal(decimal.NewFromInt(6)) {
			t.Fatalf("descartar movió la leche a %s", got)
		}
	})

	t.Run("otra empresa no captura en este conteo", func(t *testing.T) {
		if _, err := w.owner.Pool.Exec(ctx, `select 1`); err != nil {
			t.Fatal(err)
		}
		ajeno := w.token(t, w.other, w.otherAdm, "write:inventory")
		res := callTool(t, w.router, ajeno, "stock_count_capture", applyArgs(map[string]any{"count_id": count, "lines": []any{
			map[string]any{"ingredient_id": leche, "counted_qty": "1"}}}, "captura-ajena-1"))
		if !res.isError {
			t.Fatalf("otra empresa capturó: %s", res.text)
		}
	})
}

// ---- US4: cancelar pedidos ----

func TestMCPOrderCancel(t *testing.T) {
	t.Parallel()
	w := newMCPWorld(t)
	f := mcpSeed(t, w)
	prod := makeProduct(t, w.owner, "Taro MCP", decimal.NewFromInt(70), false)
	orders := app.NewOrdersService(w.app, nil)
	octx := tenantCtx(t, w.app, w.company, w.admin)
	tok := w.token(t, w.company, w.admin, "write:orders")
	status := func(id int64) string {
		var s string
		_ = w.owner.Pool.QueryRow(context.Background(), `select status::text from orders where id = $1`, id).Scan(&s)
		return s
	}

	t.Run("sin cobros se cancela y el evento dice quién y con qué token", func(t *testing.T) {
		id := crearPedidoSimple(t, octx, orders, prod, w.admin)
		args := applyArgs(map[string]any{"order_id": id, "reason": "se capturó dos veces"}, "cancelar-sin-cobro-1")
		for i := range 10 {
			if res := callTool(t, w.router, tok, "order_cancel", args); res.isError {
				t.Fatalf("intento %d: %s", i+1, res.text)
			}
		}
		if status(id) != "cancelada" {
			t.Fatalf("estado %s", status(id))
		}
		if n := scalar(t, w, `select count(*) from audit_events where entity_type = 'order' and entity_id = $1
			and action like 'order.cancel%' and actor_user_id = $2 and device::text like '%"agent": "mcp"%'`, id, w.admin); n != 1 {
			t.Fatalf("eventos de cancelación del agente: %d", n)
		}
	})

	t.Run("cobrado en efectivo: sin refund se rechaza; con refund se devuelve una vez", func(t *testing.T) {
		id := crearPedidoSimple(t, octx, orders, prod, w.admin)
		charge(t, octx, orders, id, int16(f.cash), "70", w.admin)
		prev := callTool(t, w.router, tok, "order_cancel", map[string]any{"order_id": id, "reason": "no lo quiso"})
		if prev.isError || !strings.Contains(prev.text, "refund: true") || !strings.Contains(prev.text, `"needs_refund":true`) {
			t.Fatalf("vista previa: %s", prev.text)
		}
		res := callTool(t, w.router, tok, "order_cancel", applyArgs(map[string]any{"order_id": id, "reason": "no lo quiso"}, "cancelar-cobrado-0"))
		if !res.isError || status(id) == "cancelada" {
			t.Fatalf("cobrado sin refund: %s", res.text)
		}
		args := applyArgs(map[string]any{"order_id": id, "reason": "no lo quiso", "refund": true}, "cancelar-cobrado-1")
		for i := range 10 {
			if res := callTool(t, w.router, tok, "order_cancel", args); res.isError {
				t.Fatalf("intento %d: %s", i+1, res.text)
			}
		}
		if status(id) != "cancelada" {
			t.Fatalf("estado %s", status(id))
		}
		if got := refundedTotal(t, w.owner, id); !got.Equal(decimal.NewFromInt(70)) {
			t.Fatalf("devuelto: %s (diez llamadas deben devolver una vez)", got)
		}
	})

	t.Run("entregado y cobrado: la vista previa avisa que no se cancela", func(t *testing.T) {
		id := crearPedidoSimple(t, octx, orders, prod, w.admin)
		charge(t, octx, orders, id, int16(f.cash), "70", w.admin)
		if err := orders.DeliverAll(octx, id); err != nil {
			t.Fatal(err)
		}
		res := callTool(t, w.router, tok, "order_cancel", map[string]any{"order_id": id, "reason": "x"})
		if res.isError || !strings.Contains(res.text, "devolución") || strings.Contains(res.text, "Hace falta refund") {
			t.Fatalf("vista previa de un entregado cobrado: %s", res.text)
		}
		if res := callTool(t, w.router, tok, "order_cancel", applyArgs(map[string]any{"order_id": id, "reason": "x", "refund": true},
			"cancelar-entregado-1")); !res.isError || status(id) == "cancelada" {
			t.Fatalf("se canceló un entregado cobrado: %s", res.text)
		}
	})

	t.Run("cobrado con tarjeta pide el folio", func(t *testing.T) {
		id := crearPedidoSimple(t, octx, orders, prod, w.admin)
		charge(t, octx, orders, id, int16(f.card), "70", w.admin)
		res := callTool(t, w.router, tok, "order_cancel", applyArgs(map[string]any{"order_id": id, "reason": "error", "refund": true},
			"cancelar-tarjeta-1"))
		if !res.isError || !strings.Contains(res.text, "card_folio") {
			t.Fatalf("tarjeta sin folio: %s", res.text)
		}
	})

	t.Run("sin el alcance de pedidos no se ofrece", func(t *testing.T) {
		if contains(listTools(t, w.router, w.token(t, w.company, w.admin, "write:cash")), "order_cancel") {
			t.Fatal("un token de caja ve order_cancel")
		}
	})

	t.Run("el pedido de otra empresa no existe", func(t *testing.T) {
		id := crearPedidoSimple(t, octx, orders, prod, w.admin)
		ajeno := w.token(t, w.other, w.otherAdm, "write:orders")
		res := callTool(t, w.router, ajeno, "order_cancel", applyArgs(map[string]any{"order_id": id, "reason": "x"}, "cancelar-ajeno-1"))
		if !res.isError || !strings.Contains(res.text, "No existe") || status(id) == "cancelada" {
			t.Fatalf("otra empresa: %s", res.text)
		}
	})
}

// La vista previa de una compra corre la captura entera y toma sus candados: lleva su propio límite
// por usuario (auditoría de seguridad de la 073). Sin esta prueba, quitarlo no tronaba nada.
func TestPurchasePreviewIsRateLimited(t *testing.T) {
	t.Parallel()
	w := newMCPWorld(t)
	f := mcpSeed(t, w)
	jwt, err := w.jm.Issue(domain.User{ID: w.admin, CompanyID: w.company, Name: "admin", Role: domain.RoleAdmin})
	if err != nil {
		t.Fatal(err)
	}
	body := fmt.Sprintf(`{"categoryId":%d,"amount":"10.00","status":"pendiente"}`, f.category)
	var last int
	for i := range 31 {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/expenses/purchase-preview", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+jwt)
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		w.router.ServeHTTP(rec, req)
		last = rec.Code
		if i < 30 && rec.Code != http.StatusOK {
			t.Fatalf("la vista previa %d respondió %d: %s", i+1, rec.Code, rec.Body)
		}
	}
	if last != http.StatusTooManyRequests {
		t.Fatalf("la vista previa 31 en un minuto respondió %d, esperaba 429", last)
	}
}
