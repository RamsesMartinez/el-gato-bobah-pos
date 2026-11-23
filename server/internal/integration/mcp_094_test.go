//go:build integration

package integration

import (
	"context"
	"testing"
)

// MCP POR ALMACÉN (spec 094): una compra del agente entra al almacén que dice cada renglón; sin él, al
// de venta. Un almacén de otra empresa no existe para el token.
func TestMCPPurchaseGoesToTheWarehouseOfEachLine(t *testing.T) {
	t.Parallel()
	w := newMCPWorld(t)
	f := mcpSeed(t, w)
	ctx := context.Background()
	grams := unitIDOf(t, w, "g")
	tok := w.token(t, w.company, w.admin, "write:purchases")
	_, bodega := comoEnLaSegundaSubida(t, w.owner, w.company)
	ing := ingredientIn(t, w.owner, w.company, "Harina a la bodega MCP", "g")

	purchaseLine(t, w, tok, f, "compra-bodega-0001",
		map[string]any{"name": "HARINA 1KG", "ingredient_id": ing, "unit_id": grams, "quantity": "1000", "amount": "30.00",
			"warehouse_id": bodega}, nil)
	if v := onHandIn(t, w.owner, bodega, "ingredient_id", ing); !v.Equal(dd("1000")) {
		t.Fatalf("la bodega quedó con %s; quiere 1000 g", v)
	}

	var foreign int64
	if err := w.owner.Pool.QueryRow(ctx, `select id from warehouses where company_id = $1`, w.other).Scan(&foreign); err != nil {
		t.Fatal(err)
	}
	args := map[string]any{"category_id": f.category, "amount": "30.00", "lines": []any{map[string]any{
		"name": "HARINA 1KG", "ingredient_id": ing, "unit_id": grams, "quantity": "1000", "amount": "30.00", "warehouse_id": foreign}}}
	if res := callTool(t, w.router, tok, "purchase_create", applyArgs(args, "compra-ajena-0001")); !res.isError {
		t.Fatal("una compra al almacén de otra empresa entró")
	}
}
