//go:build integration

package integration

import (
	"strings"
	"testing"
)

// LA HERRAMIENTA DE TRASPASO (spec 094): la vista previa nombra el origen y el destino y no mueve nada;
// aplicar con clave mueve una sola vez aunque se repita; si el origen no alcanza, se pide confirmar.
func TestMCPStockTransfer(t *testing.T) {
	t.Parallel()
	w := newMCPWorld(t)
	local, bodega := comoEnLaSegundaSubida(t, w.owner, w.company)
	tok := w.token(t, w.company, w.admin, "write:inventory")
	ing := ingredientIn(t, w.owner, w.company, "Azúcar MCP 094", "g")
	adjustIn(t, w.owner, bodega, "ingrediente", ing, "20")
	args := func(qty string) map[string]any {
		return map[string]any{"from_warehouse_id": bodega, "to_warehouse_id": local,
			"lines": []map[string]any{{"ingredient_id": ing, "quantity": qty}}}
	}
	moved := func() int64 {
		return scalar(t, w, `select count(*) from stock_movements where ingredient_id = $1 and movement_type = 'traspaso'`, ing)
	}

	t.Run("la vista previa nombra origen y destino y no mueve", func(t *testing.T) {
		res := callTool(t, w.router, tok, "stock_transfer", args("6"))
		if res.isError || res.data["preview"] != true || !strings.Contains(res.text, "Sale de «Bodega» y entra a «Local»") {
			t.Fatalf("vista previa: %s", res.text)
		}
		if moved() != 0 {
			t.Fatal("la vista previa movió mercancía")
		}
	})

	t.Run("aplicar con clave mueve una vez, y repetirla no vuelve a mover", func(t *testing.T) {
		apply := args("6")
		apply["dry_run"], apply["idempotency_key"] = false, "traspaso-094-uno"
		if res := callTool(t, w.router, tok, "stock_transfer", apply); res.isError {
			t.Fatalf("aplicar: %s", res.text)
		}
		if res := callTool(t, w.router, tok, "stock_transfer", apply); res.isError {
			t.Fatalf("repetir: %s", res.text)
		}
		if n := moved(); n != 2 {
			t.Fatalf("repetir la clave dejó %d movimientos de traspaso; quiere 2 (una salida y una entrada)", n)
		}
	})

	t.Run("más de lo que hay en el origen pide confirmar", func(t *testing.T) {
		apply := args("500")
		apply["dry_run"], apply["idempotency_key"] = false, "traspaso-094-dos"
		res := callTool(t, w.router, tok, "stock_transfer", apply)
		if !res.isError || !strings.Contains(res.text, "confirm_negative: true") || !strings.Contains(res.text, "hay 14") {
			t.Fatalf("traspasar 500 de 14 = %s; quiere que pida confirm_negative con lo que hay", res.text)
		}
		if n := moved(); n != 2 {
			t.Fatalf("sin confirmar se movió: %d movimientos", n)
		}
	})

	t.Run("la lista trae el traspaso", func(t *testing.T) {
		res := callTool(t, w.router, tok, "stock_transfers_list", map[string]any{})
		if res.isError || !strings.Contains(res.text, "Azúcar MCP 094") {
			t.Fatalf("lista: %s", res.text)
		}
	})
}
