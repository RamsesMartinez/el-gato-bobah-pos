//go:build integration

package integration

import (
	"context"
	"errors"
	"strconv"
	"testing"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
)

// CONTEO CONTRA VENTAS (spec 096, subida 2) con el servicio real: abrir, capturar y cerrar conteos,
// vender y mermar entre ellos. Un artículo con 100 piezas contadas compra 50, vende 120 y se merman 3;
// el sistema queda en 27 y se cuentan 22: diferencia −5, −4.2 % de lo vendido.
func TestCountVsSalesWithTheRealCount(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	actor := makeUser(t, st, "gerente_conteo_ventas", "gerente")
	svc, ctx := asWaste(t, defaultCompanyID)
	caduco := reasonNamed(t, st, defaultCompanyID, "Caducó")
	ing := pieceIngredient(t, st, "Insumo conteo ventas")
	adjust(t, st, "ingrediente", ing, "100")
	move := func(kind, q string) {
		t.Helper()
		if _, err := st.Pool.Exec(context.Background(), `insert into stock_movements (item_type, ingredient_id, movement_type, quantity)
			values ('ingrediente', $1, $2::stock_movement_type, $3::numeric)`, ing, kind, q); err != nil {
			t.Fatal(err)
		}
	}

	first := countAndClose(t, svc, ctx, ing, "100", actor)
	if v, err := svc.CountVsSales(ctx, first); err != nil || !v.FirstCount || v.Items == nil {
		t.Fatalf("primer conteo = %+v, %v; quiere «sin con qué comparar» y un arreglo", v, err)
	}

	move("compra", "50")
	move("venta", "-120")
	if _, err := svc.RecordWaste(ctx, app.RecordWasteCmd{ReasonID: caduco, IngredientID: &ing, Qty: qty("3"), Actor: actor}); err != nil {
		t.Fatal(err)
	}
	second := countAndClose(t, svc, ctx, ing, "22", actor)
	v, err := svc.CountVsSales(ctx, second)
	if err != nil || len(v.Items) != 1 {
		t.Fatalf("= %+v, %v", v, err)
	}
	it := v.Items[0]
	if !it.Initial.Equal(qty("100")) || !it.Entries.Equal(qty("50")) || !it.Sold.Equal(qty("120")) || !it.Registered.Equal(qty("3")) ||
		!it.System.Equal(qty("27")) || !it.Diff.Equal(qty("-5")) || it.Pct == nil || !it.Pct.Equal(qty("-4.2")) {
		t.Fatalf("renglón = %+v", it)
	}

	// Tres conteos seguidos con 3 % de faltante sobre lo vendido: sugiere 3 %.
	last := second
	for range 3 {
		move("venta", "-100")
		var sys float64
		_ = st.Pool.QueryRow(context.Background(), `select on_hand from stock_levels where ingredient_id = $1`, ing).Scan(&sys)
		if sys < 3 {
			adjust(t, st, "ingrediente", ing, "200")
			_ = st.Pool.QueryRow(context.Background(), `select on_hand from stock_levels where ingredient_id = $1`, ing).Scan(&sys)
		}
		last = countAndClose(t, svc, ctx, ing, strconv.FormatFloat(sys-3, 'f', -1, 64), actor)
	}
	v, err = svc.CountVsSales(ctx, last)
	if err != nil || len(v.Items) != 1 || v.Items[0].Group != "suggest" || v.Items[0].SuggestedPct == nil || !v.Items[0].SuggestedPct.Equal(qty("3")) {
		t.Fatalf("tras tres conteos estables al 3 %% = %+v, %v; quiere sugerir 3", v.Items, err)
	}

	inTheThreeCases(t, defaultCompanyID, makeCompany(t, st, "otra-conteo-ventas"), func(t *testing.T, as *store.Store, c context.Context) {
		v, err := app.NewBackofficeService(as, clock).CountVsSales(c, last)
		if err == nil && len(v.Items) > 0 {
			t.Fatal("se vio el conteo contra ventas de otra empresa")
		}
	})
}

// Lo esperado del conteo (vista avanzada) solo lo da su ruta con inventory.configure (la referencia de
// permisos lo fija: cajero y mesero no la tienen) y nunca cruza empresas.
func TestCountExpectedIsOnlyForTheWarehouseAndIsolated(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	actor := makeUser(t, st, "gerente_esperado", "gerente")
	svc, ctx := asWaste(t, defaultCompanyID)
	ing := pieceIngredient(t, st, "Insumo esperado")
	adjust(t, st, "ingrediente", ing, "40")
	c, err := svc.OpenCount(ctx, domain.WarehouseSel{}, actor)
	if err != nil {
		t.Fatal(err)
	}
	got, err := svc.CountExpected(ctx, c.ID)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, e := range got {
		if e.IngredientID == ing && e.Expected.Equal(qty("40")) {
			found = true
		}
	}
	if !found {
		t.Fatalf("lo esperado no trae las 40 piezas: %+v", got)
	}
	// Revisión de código: un conteo que no existe respondía 200 vacío (y el reporte, «primer
	// conteo»); un conteo abierto no tiene comparación contra ventas todavía.
	if _, err := svc.CountExpected(ctx, 999999999); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("lo esperado de un conteo que no existe = %v; quiere ErrNotFound", err)
	}
	if _, err := svc.CountVsSales(ctx, 999999999); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("el reporte de un conteo que no existe = %v; quiere ErrNotFound", err)
	}
	if _, err := svc.CountVsSales(ctx, c.ID); !errors.Is(err, domain.ErrConflict) {
		t.Errorf("el reporte de un conteo abierto = %v; quiere ErrConflict", err)
	}
	inTheThreeCases(t, defaultCompanyID, makeCompany(t, st, "otra-esperado"), func(t *testing.T, as *store.Store, cc context.Context) {
		v, err := app.NewBackofficeService(as, clock).CountExpected(cc, c.ID)
		if err == nil && len(v) > 0 {
			t.Fatal("se vio lo esperado del conteo de otra empresa")
		}
	})
}
