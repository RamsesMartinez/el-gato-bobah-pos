//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"

	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
)

// CONTAR CON FECHAS CORRIGE LOS LOTES (spec 098, historia 5).

func (k *lotKit) capture(t *testing.T, svc *app.BackofficeService, ctx context.Context, count int64, qty string, user int64, lots ...app.CountLot) error {
	t.Helper()
	in := app.CountCapture{Qty: decimal.RequireFromString(qty)}
	if lots != nil {
		in.Lots = &lots
	}
	_, err := svc.CaptureCountChecked(ctx, count, k.leche, in, user)
	return err
}

// El escenario del spec: 5 contados, 3 al día +17 y 1 ya caducado; al cerrar, los lotes son los contados
// y 1 queda sin fecha. Un lote que el sistema tenía y no se contó queda en 0.
func TestCountingWithDatesSetsTheLots(t *testing.T) {
	t.Parallel()
	k := newLotKit(t)
	today := k.today(t)
	k.move(t, k.local, "compra", "9", day(today, 30)) // el sistema cree que hay 9 del +30
	svc, ctx := asTenant(t, defaultCompanyID)
	user := makeUser(t, k.st, "cuenta_fechas", "gerente")
	c, err := svc.OpenCount(ctx, domain.WarehouseSel{}, user)
	if err != nil {
		t.Fatal(err)
	}
	if err := k.capture(t, svc, ctx, c.ID, "5", user,
		app.CountLot{ExpiresOn: day(today, 17), Qty: decimal.NewFromInt(3)},
		app.CountLot{ExpiresOn: day(today, -3), Qty: decimal.NewFromInt(1)}); err != nil {
		t.Fatal(err)
	}
	// Lo que se lee del conteo trae el desglose y las fechas que ya conocía el sistema, sin cantidad.
	cur, err := svc.CurrentCount(ctx, domain.WarehouseSel{})
	if err != nil {
		t.Fatal(err)
	}
	var item *app.CountItem
	for i := range cur.Items {
		if cur.Items[i].IngredientID == k.leche {
			item = &cur.Items[i]
		}
	}
	if item == nil || len(item.Lots) != 2 || len(item.KnownDates) != 1 || item.KnownDates[0] != day(today, 30) {
		t.Fatalf("renglón del conteo = %+v", item)
	}
	if _, err := svc.CloseCount(ctx, c.ID, user); err != nil {
		t.Fatal(err)
	}
	wantLots(t, k.lots(t, k.local), map[string]string{day(today, 17): "3", day(today, -3): "1"}, "tras cerrar")
	if u := k.undated(t, k.local); !u.Equal(decimal.NewFromInt(1)) {
		t.Fatalf("sin fecha = %s, quería 1", u)
	}
}

// Lo vendido entre la captura y el cierre se resta del lote contado; lo que se repuso después, suma.
func TestASaleDuringTheCountComesOffTheCountedLot(t *testing.T) {
	t.Parallel()
	k := newLotKit(t)
	today := k.today(t)
	d := day(today, 10)
	k.move(t, k.local, "compra", "10", d)
	svc, ctx := asTenant(t, defaultCompanyID)
	user := makeUser(t, k.st, "cuenta_venta", "gerente")
	c, err := svc.OpenCount(ctx, domain.WarehouseSel{}, user)
	if err != nil {
		t.Fatal(err)
	}
	if err := k.capture(t, svc, ctx, c.ID, "8", user, app.CountLot{ExpiresOn: d, Qty: decimal.NewFromInt(8)}); err != nil {
		t.Fatal(err)
	}
	k.move(t, k.local, "venta", "-3", "")
	k.move(t, k.local, "merma", "1", "") // positiva sin origen: suma a sin fecha, no al lote
	if _, err := svc.CloseCount(ctx, c.ID, user); err != nil {
		t.Fatal(err)
	}
	wantLots(t, k.lots(t, k.local), map[string]string{d: "5"}, "8 contados − 3 vendidos después")
}

// Sin desglose, los lotes no se reescriben: la diferencia va por el orden de consumo.
func TestCountingWithoutDatesKeepsTheLots(t *testing.T) {
	t.Parallel()
	k := newLotKit(t)
	today := k.today(t)
	d := day(today, 10)
	k.move(t, k.local, "ajuste", "2", "")
	k.move(t, k.local, "compra", "10", d)
	svc, ctx := asTenant(t, defaultCompanyID)
	user := makeUser(t, k.st, "cuenta_sin", "gerente")
	c, err := svc.OpenCount(ctx, domain.WarehouseSel{}, user)
	if err != nil {
		t.Fatal(err)
	}
	if err := k.capture(t, svc, ctx, c.ID, "9", user); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CloseCount(ctx, c.ID, user); err != nil {
		t.Fatal(err)
	}
	// 12 → 9: faltan 3; salen 2 de sin fecha y 1 del lote.
	wantLots(t, k.lots(t, k.local), map[string]string{d: "9"}, "sin desglose")
}

// Un artículo con desglose y sin diferencia también fija sus lotes; un desglose que pasa lo contado
// se rechaza; `[]` lo borra.
func TestCountLotEdges(t *testing.T) {
	t.Parallel()
	k := newLotKit(t)
	today := k.today(t)
	k.move(t, k.local, "ajuste", "6", "")
	svc, ctx := asTenant(t, defaultCompanyID)
	user := makeUser(t, k.st, "cuenta_bordes", "gerente")
	c, err := svc.OpenCount(ctx, domain.WarehouseSel{}, user)
	if err != nil {
		t.Fatal(err)
	}
	if err := k.capture(t, svc, ctx, c.ID, "6", user, app.CountLot{ExpiresOn: day(today, 4), Qty: decimal.NewFromInt(7)}); !errors.Is(err, domain.ErrLotsExceedCounted) {
		t.Fatalf("fechas de más = %v", err)
	}
	if err := k.capture(t, svc, ctx, c.ID, "6", user, app.CountLot{ExpiresOn: day(today, 4), Qty: decimal.NewFromInt(2)}); err != nil {
		t.Fatal(err)
	}
	if err := k.capture(t, svc, ctx, c.ID, "6", user); err != nil { // sin `lots`: no cambia el desglose
		t.Fatal(err)
	}
	if _, err := svc.CloseCount(ctx, c.ID, user); err != nil {
		t.Fatal(err)
	}
	wantLots(t, k.lots(t, k.local), map[string]string{day(today, 4): "2"}, "sin diferencia pero con fechas")

	c2, err := svc.OpenCount(ctx, domain.WarehouseSel{}, user)
	if err != nil {
		t.Fatal(err)
	}
	if err := k.capture(t, svc, ctx, c2.ID, "6", user, app.CountLot{ExpiresOn: day(today, 4), Qty: decimal.NewFromInt(2)}); err != nil {
		t.Fatal(err)
	}
	if err := k.capture(t, svc, ctx, c2.ID, "6", user, []app.CountLot{}...); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := k.st.Pool.QueryRow(context.Background(), `select count(*) from stock_count_line_lots where count_id = $1`, c2.ID).Scan(&n); err != nil || n != 0 {
		t.Fatalf("`[]` dejó %d fechas (%v)", n, err)
	}
}
