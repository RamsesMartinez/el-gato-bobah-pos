//go:build integration

package integration

import (
	"strings"
	"testing"
)

// LA HERRAMIENTA DE ALERTAS POR LOTE (spec 084): la vista previa (dry_run ausente) dice qué avisos se
// abrirían y no guarda; aplicar pide idempotency_key y repetirla no vuelve a aplicar.
func TestMCPLowStockUpdate(t *testing.T) {
	t.Parallel()
	w := newMCPWorld(t)
	tok := w.token(t, w.company, w.admin, "write:inventory")
	ing := ingredientIn(t, w.owner, w.company, "Queso crema MCP 084", "g")
	adjust(t, w.owner, "ingrediente", ing, "200")
	args := map[string]any{"items": []map[string]any{{"item_type": "ingrediente", "id": ing, "enabled": true, "min_stock": "500"}}}
	enabled := func() int64 {
		return scalar(t, w, `select count(*) from ingredients where id = $1 and low_stock_enabled`, ing)
	}

	t.Run("la vista previa dice que se abrirá un aviso y no guarda", func(t *testing.T) {
		res := callTool(t, w.router, tok, "low_stock_update", args)
		if res.isError || res.data["preview"] != true || !strings.Contains(res.text, "stock_low") {
			t.Fatalf("vista previa: %s", res.text)
		}
		if enabled() != 0 {
			t.Fatal("la vista previa encendió la alerta")
		}
	})

	t.Run("aplicar con clave enciende, y repetir la clave no vuelve a aplicar", func(t *testing.T) {
		apply := map[string]any{"items": args["items"], "dry_run": false, "idempotency_key": "alertas-084-uno"}
		if res := callTool(t, w.router, tok, "low_stock_update", apply); res.isError {
			t.Fatalf("aplicar: %s", res.text)
		}
		if enabled() != 1 {
			t.Fatal("no se encendió")
		}
		if res := callTool(t, w.router, tok, "low_stock_update", apply); res.isError {
			t.Fatalf("repetir: %s", res.text)
		}
		if n := scalar(t, w, `select count(*) from audit_events where action = 'stock.low_stock_configured' and entity_id = $1`, ing); n != 1 {
			t.Fatalf("repetir la clave volvió a aplicar: %d eventos", n)
		}
	})

	t.Run("sin el alcance de almacén la herramienta no existe", func(t *testing.T) {
		otro := w.token(t, w.company, w.admin, "read")
		if res := callTool(t, w.router, otro, "low_stock_update", args); !res.isError && len(res.rpcErr) == 0 {
			t.Fatalf("sin alcance: %s", res.text)
		}
	})
}
