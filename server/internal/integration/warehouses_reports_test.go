//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store/db"
)

// MOVIMIENTOS Y VALOR POR ALMACÉN (spec 094, historia 7).

// Los movimientos de Bodega son solo los de Bodega, y la pata de un traspaso dice a qué almacén fue.
func TestMovementsByWarehouseShowTheTransferCounterpart(t *testing.T) {
	t.Parallel()
	k := newTransferKit(t)
	if _, err := k.svc.CreateTransfer(k.ctx, k.in(k.bodega, k.local, "6"), k.user); err != nil {
		t.Fatal(err)
	}
	bodega, err := k.svc.StockMovements(k.ctx, domain.WarehouseSel{ID: k.bodega}, 50)
	if err != nil {
		t.Fatal(err)
	}
	var leg *string
	for _, m := range bodega {
		if m.WarehouseID != k.bodega || m.WarehouseName != "Bodega" {
			t.Fatalf("los movimientos de Bodega traen uno de %q", m.WarehouseName)
		}
		if m.MovementType == "traspaso" {
			leg = m.CounterpartWarehouseName
			if m.TransferID == nil || m.UnitCode != "ml" || !m.Quantity.Equal(decimal.NewFromInt(-6)) {
				t.Fatalf("la pata de Bodega = %+v; quiere −6 ml ligada a su traspaso", m)
			}
		}
	}
	if leg == nil || *leg != "Local" {
		t.Fatalf("la pata que salió de Bodega dice contraparte %v; quiere Local", leg)
	}
	sales, err := k.svc.StockMovements(k.ctx, domain.WarehouseSel{}, 50)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range sales {
		if m.WarehouseID != k.local {
			t.Fatalf("sin almacén salió un movimiento de %q; quiere solo el de venta", m.WarehouseName)
		}
		if m.MovementType == "traspaso" && (m.CounterpartWarehouseName == nil || *m.CounterpartWarehouseName != "Bodega") {
			t.Fatalf("la pata que entró a Local dice contraparte %v; quiere Bodega", m.CounterpartWarehouseName)
		}
	}
	all, err := k.svc.StockMovements(k.ctx, domain.WarehouseSel{All: true}, 50)
	if err != nil || len(all) != len(sales)+len(bodega) {
		t.Fatalf("todos = %d movimientos (%v); quiere %d + %d", len(all), err, len(sales), len(bodega))
	}
}

// El valor de cada almacén suma el total, y un negativo no resta valor: Local 10 ml a $0.025 vale
// $0.25 y Bodega en −3 vale 0, así que los dos juntos valen $0.25 y no 7 × $0.025.
func TestStockValueDoesNotSubtractNegatives(t *testing.T) {
	t.Parallel()
	k := newTransferKit(t)
	adjustIn(t, k.st, k.bodega, "ingrediente", k.leche, "-23")
	value := func(sel domain.WarehouseSel) decimal.Decimal {
		t.Helper()
		rows, err := k.svc.StockLevels(k.ctx, sel)
		if err != nil {
			t.Fatal(err)
		}
		for _, r := range rows {
			if r.IngredientID != nil && *r.IngredientID == k.leche {
				return r.Value
			}
		}
		t.Fatalf("la leche no sale en %+v", sel)
		return decimal.Zero
	}
	local := value(domain.WarehouseSel{})
	bodega := value(domain.WarehouseSel{ID: k.bodega})
	all := value(domain.WarehouseSel{All: true})
	if !local.Equal(decimal.RequireFromString("0.25")) {
		t.Fatalf("el valor de Local es %s; quiere 0.25", local)
	}
	if !bodega.IsZero() {
		t.Fatalf("Bodega en −3 vale %s; un negativo no es valor que se tenga", bodega)
	}
	if !all.Equal(local.Add(bodega)) {
		t.Fatalf("todos los almacenes valen %s; quiere la suma de cada uno (%s): el negativo de Bodega restó valor", all, local.Add(bodega))
	}
}

// Un traspaso no es consumo: lo que sale de Local hacia Bodega no cuenta en lo que se usa para
// sugerir compras, solo la venta.
func TestATransferIsNotConsumption(t *testing.T) {
	t.Parallel()
	k := newTransferKit(t)
	ctx := context.Background()
	if _, err := k.st.Pool.Exec(ctx, `update ingredients set low_stock_enabled = true, min_stock = 1000 where id = $1`, k.leche); err != nil {
		t.Fatal(err)
	}
	if _, err := k.st.Pool.Exec(ctx, `insert into stock_movements (warehouse_id, item_type, ingredient_id, movement_type, quantity)
		values ($1, 'ingrediente', $2, 'venta', -2)`, k.local, k.leche); err != nil {
		t.Fatal(err)
	}
	if _, err := k.svc.CreateTransfer(k.ctx, k.in(k.local, k.bodega, "6"), k.user); err != nil {
		t.Fatal(err)
	}
	rows, err := k.st.Q.ShoppingListItems(ctx, db.ShoppingListItemsParams{Since: time.Now().Add(-24 * time.Hour), WarehouseID: k.local})
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rows {
		if r.ID == k.leche {
			if !r.Used.Equal(decimal.NewFromInt(2)) {
				t.Fatalf("el consumo de Local es %s; quiere 2 (la venta): el traspaso se contó como consumo", r.Used)
			}
			return
		}
	}
	t.Fatal("la leche bajo su mínimo no salió en la lista de compra")
}
