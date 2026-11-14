//go:build integration

package integration

import (
	"context"
	"testing"
	"uuid"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
)

// LA LISTA DE COMPRA (spec 084, historia 8): sale de lo que se acabó o está bajo su mínimo, mide el
// consumo solo con ventas (un «se acabó» no es consumo) y explica cada cantidad.
func TestShoppingListExcludesOutReportsFromConsumption(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()
	ana := makeUser(t, st, "ana_084s", "cajero")
	ing := pieceIngredient(t, st, "Vaso lista 084")
	adjust(t, st, "ingrediente", ing, "500")
	// Ventas de 10 piezas en 10 días distintos: 100 en 14 días, ~7.14 al día.
	for i := 1; i <= 10; i++ {
		if _, err := st.Pool.Exec(ctx, `insert into stock_movements (item_type, ingredient_id, movement_type, quantity, created_at)
			values ('ingrediente', $1, 'venta', -10, now() - make_interval(days => $2))`, ing, i); err != nil {
			t.Fatal(err)
		}
	}
	alertOn(t, st, ing, "1000", true, true)
	out, outCtx := asStockOut(t, defaultCompanyID)
	if _, _, err := out.ReportOut(outCtx, app.ReportOutCmd{ItemType: "ingrediente", ID: ing, ClientKey: uuid.New(), Actor: ana}); err != nil {
		t.Fatal(err)
	}
	sinVentas := pieceIngredient(t, st, "Sin receta lista 084")
	alertOn(t, st, sinVentas, "5", true, false)
	adjust(t, st, "ingrediente", sinVentas, "2")
	sinAlerta := pieceIngredient(t, st, "Sin alerta lista 084")
	adjust(t, st, "ingrediente", sinAlerta, "1")

	svc, _, nctx := asNotifications(t, defaultCompanyID)
	lines, err := svc.ShoppingList(nctx)
	if err != nil {
		t.Fatal(err)
	}
	byID := map[int64]app.ShoppingLine{}
	for _, l := range lines {
		byID[l.ID] = l
	}
	v, ok := byID[ing]
	if !ok {
		t.Fatalf("lo que se acabó no está en la lista: %+v", lines)
	}
	if v.DailyUse == nil || v.DailyUse.Round(2).String() != "7.14" {
		t.Fatalf("consumo diario %v; el «se acabó» (−300) no cuenta, solo las ventas", v.DailyUse)
	}
	if v.Suggested == nil || v.Why == "" || v.State != "out" {
		t.Fatalf("%+v", v)
	}
	w, ok := byID[sinVentas]
	if !ok || w.Suggested != nil || w.Why != "sin consumo medido: no está en ninguna receta o casi no se ha vendido" {
		t.Fatalf("sin ventas: %+v (%v)", w, ok)
	}
	if _, ok := byID[sinAlerta]; ok {
		t.Fatal("un insumo sin alerta ni aviso entró a la lista")
	}
}
