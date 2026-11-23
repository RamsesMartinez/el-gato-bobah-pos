//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
)

func lotQty(t *testing.T, k *mergeKit, ing int64, d string) decimal.Decimal {
	t.Helper()
	var v decimal.Decimal
	if err := k.st.Pool.QueryRow(context.Background(), `select coalesce(sum(qty), 0) from stock_lots
		where ingredient_id = $1 and expires_on = $2::date`, ing, d).Scan(&v); err != nil {
		t.Fatal(err)
	}
	return v
}

// Unir dos marcas pasa los lotes del origen al destino, por almacén y fecha, con la equivalencia; si
// el destino ya tenía esa fecha, se suman (spec 098, FR-061).
func TestMergingArticlesMovesTheirLots(t *testing.T) {
	t.Parallel()
	k := newMergeKit(t)
	ctx := context.Background()
	var today time.Time
	if err := k.st.Pool.QueryRow(ctx, `select business_today($1)`, defaultCompanyID).Scan(&today); err != nil {
		t.Fatal(err)
	}
	d1, d2 := today.AddDate(0, 0, 10).Format(time.DateOnly), today.AddDate(0, 0, 20).Format(time.DateOnly)
	for _, m := range []struct {
		ing int64
		qty string
		d   string
	}{{k.a, "100", d1}, {k.b, "50", d1}, {k.b, "80", d2}} {
		if _, err := k.st.Pool.Exec(ctx, `insert into stock_movements (item_type, ingredient_id, movement_type, quantity, expires_on)
			values ('ingrediente', $1, 'compra', $2::numeric, $3::date)`, m.ing, m.qty, m.d); err != nil {
			t.Fatal(err)
		}
	}
	k.merge(t, k.a, k.req(k.b))
	if v := lotQty(t, k, k.a, d1); !v.Equal(decimal.NewFromInt(150)) {
		t.Fatalf("lote del destino en %s = %s, quería 150 (100 suyos + 50 del origen)", d1, v)
	}
	if v := lotQty(t, k, k.a, d2); !v.Equal(decimal.NewFromInt(80)) {
		t.Fatalf("lote del destino en %s = %s, quería 80", d2, v)
	}
	if v := lotQty(t, k, k.b, d1).Add(lotQty(t, k, k.b, d2)); !v.IsZero() {
		t.Fatalf("el origen conserva %s en lotes", v)
	}
	// Lo sin fecha del destino sigue cuadrando: existencia − lotes.
	var undated decimal.Decimal
	if err := k.st.Pool.QueryRow(ctx, `select (select sum(on_hand) from stock_levels where ingredient_id = $1)
		- (select sum(qty) from stock_lots where ingredient_id = $1)`, k.a).Scan(&undated); err != nil {
		t.Fatal(err)
	}
	// 500 + 100 del destino y 300 + 130 del origen = 1030; 230 en lotes; 800 sin fecha.
	if !undated.Equal(decimal.NewFromInt(800)) {
		t.Fatalf("sin fecha del destino = %s, quería 800", undated)
	}
}

// Cambiar la unidad (directo) escala los lotes y lo que tomó cada movimiento.
func TestChangingTheUnitScalesTheLots(t *testing.T) {
	t.Parallel()
	k := newLotKit(t)
	ctx := context.Background()
	d := day(k.today(t), 15)
	// Un artículo sin compras ni ventas (cambio directo): sus lotes nacen de un conteo, aquí un ajuste fechado.
	k.move(t, k.local, "ajuste", "2000", d)
	k.move(t, k.local, "ajuste", "-500", "")
	svc, sctx := asTenant(t, defaultCompanyID)
	admin := makeUser(t, k.st, "unidad_lotes", "admin")
	if err := svc.ChangeUnit(app.WithActorUser(sctx, admin), k.leche, app.UnitChangeRequest{BaseUnitID: unitID(t, k.st, "pieza"), Factor: decimal.RequireFromString("0.002")}); err != nil {
		t.Fatal(err)
	}
	wantLots(t, k.lots(t, k.local), map[string]string{d: "3"}, "en piezas")
	var alloc decimal.Decimal
	if err := k.st.Pool.QueryRow(ctx, `select sum(a.qty) from stock_lot_allocations a join stock_lots l on l.id = a.lot_id
		where l.ingredient_id = $1`, k.leche).Scan(&alloc); err != nil || !alloc.Equal(decimal.NewFromInt(3)) {
		t.Fatalf("las asignaciones suman %s (%v), quería 3 en piezas", alloc, err)
	}
}
