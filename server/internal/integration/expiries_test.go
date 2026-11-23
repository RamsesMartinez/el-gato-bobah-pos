//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
)

// LA VISTA DE CADUCIDADES (spec 098, historia 3): lotes por almacén, consumo de 30 días y proyección.
func TestExpiriesListsLotsWithTheirProjection(t *testing.T) {
	t.Parallel()
	k := newLotKit(t)
	ctx := context.Background()
	today := k.today(t)
	k.move(t, k.bodega, "compra", "3000", day(today, 20)) // 3 kg a 20 días
	k.move(t, k.local, "compra", "500", day(today, -2))   // caducado en Local
	// 100 g al día de ventas netas en 30 días: 3200 vendidos y 200 cancelados. El traspaso no cuenta.
	k.move(t, k.local, "ajuste", "4000", "")
	k.move(t, k.local, "venta", "-3200", "")
	k.move(t, k.local, "cancelacion", "200", "")
	if _, err := k.st.Pool.Exec(ctx, `update stock_movements set created_at = now() - interval '40 days'
		where ingredient_id = $1 and movement_type = 'ajuste'`, k.leche); err != nil {
		t.Fatal(err)
	}
	svc, sctx := asTenant(t, defaultCompanyID)

	all, err := svc.Expiries(sctx, domain.WarehouseSel{All: true})
	if err != nil {
		t.Fatal(err)
	}
	if all.Counts.Expired != 1 || all.Counts.Within30 != 1 || len(all.Lots) != 2 {
		t.Fatalf("todos = %+v", all)
	}
	// El caducado va primero (del que caduca antes al que caduca después).
	if !all.Lots[0].Expired || all.Lots[0].WarehouseName != "Local" {
		t.Fatalf("primer lote = %+v", all.Lots[0])
	}
	b := all.Lots[1]
	if b.DailyUse == nil || !b.DailyUse.Equal(decimal.NewFromInt(100)) {
		t.Fatalf("consumo diario = %v, quería 100 (ventas netas de cancelaciones, sin traspasos)", b.DailyUse)
	}
	// Lo sin fecha de la sucursal (4000 − 3200 + 200 = 1000, en Local) se usa antes: 10 días, y luego
	// el lote: del día 10 al 20 se lleva 1100 de 3000; quedan ~1900.
	if b.LeftoverAtExpiry == nil || !b.LeftoverAtExpiry.Equal(decimal.NewFromInt(1900)) {
		t.Fatalf("sobrante = %v, quería 1900", b.LeftoverAtExpiry)
	}

	bodega, err := svc.Expiries(sctx, domain.WarehouseSel{ID: k.bodega})
	if err != nil || len(bodega.Lots) != 1 || bodega.Lots[0].WarehouseName != "Bodega" {
		t.Fatalf("Bodega = %+v (%v)", bodega, err)
	}
	if _, err := svc.Expiries(sctx, domain.WarehouseSel{ID: 999999}); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("almacén ajeno = %v, quería 404", err)
	}
	sum, err := svc.ExpirySummary(sctx, domain.WarehouseSel{ID: k.local})
	if err != nil || sum.Expired != 1 || sum.Within7 != 0 {
		t.Fatalf("resumen de Local = %+v (%v)", sum, err)
	}
}

// Una merma por aprobar de un lote se ve en ese lote.
func TestExpiriesShowsPendingWasteOfALot(t *testing.T) {
	t.Parallel()
	k := newLotKit(t)
	d := day(k.today(t), -1)
	k.move(t, k.local, "compra", "500", d)
	if _, err := k.st.Pool.Exec(context.Background(), `insert into waste_records
		(branch_id, warehouse_id, item_type, ingredient_id, quantity, entered_qty, is_staff_meal, status, recorded_by, reason_id, lot_expires_on)
		select w.branch_id, w.id, 'ingrediente', $2, 200, 200, false, 'pending', $3,
		       (select id from waste_reasons order by id limit 1), $4::date
		  from warehouses w where w.id = $1`, k.local, k.leche, makeUser(t, k.st, "merma_lotes", "cajero"), d); err != nil {
		t.Fatal(err)
	}
	svc, sctx := asTenant(t, defaultCompanyID)
	v, err := svc.Expiries(sctx, domain.WarehouseSel{All: true})
	if err != nil || len(v.Lots) != 1 || !v.Lots[0].PendingWaste.Equal(decimal.NewFromInt(200)) {
		t.Fatalf("lotes = %+v (%v), quería 200 pendientes de merma", v.Lots, err)
	}
}

// Aislada en los tres casos: ninguna sesión ajena ve los lotes de la dueña.
func TestExpiriesStayIsolated(t *testing.T) {
	t.Parallel()
	k := newLotKit(t)
	other := makeCompany(t, k.st, "ajena-caducidades")
	k.move(t, k.local, "compra", "100", day(k.today(t), 3))
	inTheThreeCases(t, defaultCompanyID, other, func(t *testing.T, st *store.Store, ctx context.Context) {
		v, err := app.NewBackofficeService(st, clock).Expiries(ctx, domain.WarehouseSel{All: true})
		if err == nil && len(v.Lots) > 0 {
			t.Fatalf("se ven %d lotes de otra empresa", len(v.Lots))
		}
	})
	_ = time.Now
}
