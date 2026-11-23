//go:build integration

package integration

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
	"uuid"

	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
)

// CONTEO, MERMA, «SE ACABÓ», MÍNIMOS, SALUD Y LUGARES POR ALMACÉN (spec 094, historia 6). Cada
// escenario arranca con leche en Local (10) y en Bodega (20) y comprueba que lo que se hace en un
// almacén no toca al otro.

type whCountKit struct {
	st            *store.Store
	svc           *app.BackofficeService
	ctx           context.Context
	user          int64
	local, bodega int64
	leche         int64
}

func newWhCountKit(t *testing.T, name string) *whCountKit {
	t.Helper()
	st := newTestStore(t)
	k := &whCountKit{st: st}
	k.local, k.bodega = comoEnLaSegundaSubida(t, st, defaultCompanyID)
	k.user = makeUser(t, st, "cuenta_"+name, "gerente")
	k.leche = ingredientIn(t, st, defaultCompanyID, "Leche "+name, "ml")
	adjustIn(t, st, k.local, "ingrediente", k.leche, "10")
	adjustIn(t, st, k.bodega, "ingrediente", k.leche, "20")
	k.svc, k.ctx = asTenant(t, defaultCompanyID)
	return k
}

func (k *whCountKit) onHand(t *testing.T, wh int64) decimal.Decimal {
	t.Helper()
	return onHandIn(t, k.st, wh, "ingredient_id", k.leche)
}

func (k *whCountKit) auditWarehouse(t *testing.T, action string, entity int64) string {
	t.Helper()
	var wh string
	if err := k.st.Pool.QueryRow(context.Background(), `select coalesce(after->>'warehouseId', '') from audit_events
		where action = $1 and entity_id = $2`, action, entity).Scan(&wh); err != nil {
		t.Fatalf("la bitácora de %s %d: %v", action, entity, err)
	}
	return wh
}

// Contar Bodega con diferencia y cerrar ajusta Bodega y deja Local como estaba; cada almacén tiene su
// conteo abierto, y un segundo en el mismo almacén choca.
func TestABodegaCountLeavesLocalUnchanged(t *testing.T) {
	t.Parallel()
	k := newWhCountKit(t, "conteo_bodega")
	local, err := k.svc.OpenCount(k.ctx, domain.WarehouseSel{}, k.user)
	if err != nil {
		t.Fatal(err)
	}
	c, err := k.svc.OpenCount(k.ctx, domain.WarehouseSel{ID: k.bodega}, k.user)
	if err != nil {
		t.Fatalf("con un conteo abierto en Local, abrir el de Bodega = %v; cada almacén tiene el suyo", err)
	}
	if c.WarehouseID != k.bodega || c.WarehouseName != "Bodega" || local.WarehouseID != k.local {
		t.Fatalf("los conteos dicen almacén %d (%q) y %d; quiere Bodega y Local", c.WarehouseID, c.WarehouseName, local.WarehouseID)
	}
	if _, err := k.svc.OpenCount(k.ctx, domain.WarehouseSel{ID: k.bodega}, k.user); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("un segundo conteo en Bodega = %v; quiere 409", err)
	}
	cur, err := k.svc.CurrentCount(k.ctx, domain.WarehouseSel{ID: k.bodega})
	if err != nil || cur == nil || cur.ID != c.ID {
		t.Fatalf("el conteo abierto de Bodega = %+v, %v; quiere %d", cur, err, c.ID)
	}
	if err := k.svc.CaptureCount(k.ctx, c.ID, k.leche, decimal.NewFromInt(17), k.user); err != nil {
		t.Fatal(err)
	}
	out, err := k.svc.CloseCount(k.ctx, c.ID, k.user)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Adjusted) != 1 || !out.Adjusted[0].System.Equal(decimal.NewFromInt(20)) {
		t.Fatalf("el cierre de Bodega comparó contra %+v; quiere lo de Bodega (20)", out.Adjusted)
	}
	if v := k.onHand(t, k.bodega); !v.Equal(decimal.NewFromInt(17)) {
		t.Fatalf("Bodega quedó en %s; quiere 17 (lo contado)", v)
	}
	if v := k.onHand(t, k.local); !v.Equal(decimal.NewFromInt(10)) {
		t.Fatalf("contar Bodega movió Local a %s; quiere 10", v)
	}
	if got := k.auditWarehouse(t, string(domain.AuditStockCounted), c.ID); got != itoa(int(k.bodega)) {
		t.Fatalf("la bitácora del conteo dice almacén %q; quiere %d", got, k.bodega)
	}
	last, err := k.svc.LastClosedCount(k.ctx, domain.WarehouseSel{ID: k.bodega})
	if err != nil || last == nil || last.ID != c.ID {
		t.Fatalf("el último conteo cerrado de Bodega = %+v, %v", last, err)
	}
	if last, err := k.svc.LastClosedCount(k.ctx, domain.WarehouseSel{}); err != nil || last != nil {
		t.Fatalf("Local no ha cerrado conteo y dice %+v, %v", last, err)
	}
}

// La merma de Bodega resta de Bodega y la bitácora dice de dónde.
func TestWasteInBodegaLowersBodega(t *testing.T) {
	t.Parallel()
	k := newWhCountKit(t, "merma_bodega")
	caduco := reasonNamed(t, k.st, defaultCompanyID, "Caducó")
	res, err := k.svc.RecordWaste(k.ctx, app.RecordWasteCmd{ReasonID: caduco, IngredientID: &k.leche, Qty: decimal.NewFromInt(4),
		Actor: k.user, Warehouse: domain.WarehouseSel{ID: k.bodega}})
	if err != nil {
		t.Fatal(err)
	}
	if v := k.onHand(t, k.bodega); !v.Equal(decimal.NewFromInt(16)) {
		t.Fatalf("Bodega quedó en %s; quiere 16", v)
	}
	if v := k.onHand(t, k.local); !v.Equal(decimal.NewFromInt(10)) {
		t.Fatalf("la merma de Bodega restó de Local: %s", v)
	}
	if got := k.auditWarehouse(t, string(domain.AuditWasteRecorded), res.ID); got != itoa(int(k.bodega)) {
		t.Fatalf("la bitácora de la merma dice almacén %q; quiere %d", got, k.bodega)
	}
	// Deshacerla devuelve a Bodega.
	if err := k.svc.UndoWaste(k.ctx, res.ID, k.user, true); err != nil {
		t.Fatal(err)
	}
	if v := k.onHand(t, k.bodega); !v.Equal(decimal.NewFromInt(20)) {
		t.Fatalf("deshacer la merma dejó Bodega en %s; quiere 20", v)
	}
}

// «Se acabó» en Bodega deja Bodega en 0 y el aviso es de Bodega; Local no se toca.
func TestStockOutLandsInTheRequestedWarehouse(t *testing.T) {
	t.Parallel()
	k := newWhCountKit(t, "acabo_bodega")
	so, ctx := asStockOut(t, defaultCompanyID)
	res, _, err := so.ReportOut(ctx, app.ReportOutCmd{ItemType: "ingrediente", ID: k.leche, ClientKey: uuid.New(), Actor: k.user,
		Warehouse: domain.WarehouseSel{ID: k.bodega}})
	if err != nil {
		t.Fatal(err)
	}
	if v := k.onHand(t, k.bodega); !v.IsZero() {
		t.Fatalf("«se acabó» en Bodega la dejó en %s; quiere 0", v)
	}
	if v := k.onHand(t, k.local); !v.Equal(decimal.NewFromInt(10)) {
		t.Fatalf("«se acabó» en Bodega movió Local a %s", v)
	}
	var wh int64
	if err := k.st.Pool.QueryRow(context.Background(), `select warehouse_id from notifications where id = $1`, res.NotificationID).Scan(&wh); err != nil || wh != k.bodega {
		t.Fatalf("el aviso quedó en el almacén %d (%v); quiere Bodega", wh, err)
	}
	// Deshacerlo repone Bodega, no Local.
	if err := so.UndoOut(ctx, res.ID, k.user, true); err != nil {
		t.Fatal(err)
	}
	if v := k.onHand(t, k.bodega); !v.Equal(decimal.NewFromInt(20)) {
		t.Fatalf("deshacer repuso Bodega a %s; quiere 20", v)
	}
}

// Mínimo de 5 en Local y ninguno en Bodega: al bajar los dos a 3, avisa Local y Bodega no. El aviso
// mide el almacén del movimiento, no siempre el de venta.
func TestALocalMinimumWarnsInLocalOnly(t *testing.T) {
	t.Parallel()
	k := newWhCountKit(t, "minimo_local")
	as := appRoleStore(t)
	nctx, release, err := as.AcquireTenant(context.Background(), defaultCompanyID)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(release)
	ns := app.NewNotificationsService(as, clock)
	cinco := decimal.NewFromInt(5)
	in := app.LowStockInput{Enabled: true, OnMin: true, OnOut: true,
		BranchMinStock: app.Field[decimal.Decimal]{Set: true, Value: &cinco}, Warehouse: domain.WarehouseSel{ID: k.local}}
	if _, err := ns.ConfigureLowStock(nctx, "ingrediente", k.leche, in, k.user, false); err != nil {
		t.Fatal(err)
	}
	adjustIn(t, k.st, k.local, "ingrediente", k.leche, "-7")
	adjustIn(t, k.st, k.bodega, "ingrediente", k.leche, "-17")
	rows, err := k.st.Pool.Query(context.Background(), `select w.name, n.kind from notifications n join warehouses w on w.id = n.warehouse_id
		where n.ingredient_id = $1 and n.status = 'open' order by w.name, n.kind`, k.leche)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for rows.Next() {
		var w, kind string
		if err := rows.Scan(&w, &kind); err != nil {
			t.Fatal(err)
		}
		got = append(got, w+":"+kind)
	}
	rows.Close()
	if len(got) != 1 || got[0] != "Local:stock_low" {
		t.Fatalf("los avisos abiertos son %v; quiere solo Local:stock_low (Bodega no tiene mínimo propio)", got)
	}
	cfg, err := ns.LowStockConfig(nctx, "ingrediente", k.leche, domain.WarehouseSel{ID: k.bodega})
	if err != nil || cfg.BranchMin != nil || cfg.WarehouseID != k.bodega || !cfg.OnHand.Equal(decimal.NewFromInt(3)) {
		t.Fatalf("la ficha de Bodega = %+v, %v; quiere sin mínimo propio y 3 en existencia", cfg, err)
	}
}

// Un negativo de Bodega sale en la salud con el nombre de Bodega; Local en positivo no lo compensa.
func TestHealthNamesTheWarehouseOfANegative(t *testing.T) {
	t.Parallel()
	k := newWhCountKit(t, "salud_bodega")
	adjustIn(t, k.st, k.bodega, "ingrediente", k.leche, "-23")
	if _, err := k.st.Pool.Exec(context.Background(), `update ingredients set track_stock = true where id = $1`, k.leche); err != nil {
		t.Fatal(err)
	}
	v := reportAsApp(t, k.st, domain.HealthFilter{Range: domain.DefaultHealthRange(time.Now()), Section: domain.SectionNegative, Limit: 50})
	found := false
	for _, n := range v.NegativeStock.Items {
		if n.IngredientID == k.leche {
			found = true
			if n.Warehouse != "Bodega" || n.WarehouseID != k.bodega || !n.OnHand.Equal(decimal.NewFromInt(-3)) {
				t.Fatalf("el negativo dice %+v; quiere Bodega en -3", n)
			}
		}
	}
	if !found {
		t.Fatal("Bodega en -3 no sale en la salud: Local en 10 lo escondió")
	}
}

// El nombre de un lugar es único por almacén; al contar Bodega, un artículo con lugar de Local va al
// final marcado «sin lugar en este almacén».
func TestCountPlacesArePerWarehouse(t *testing.T) {
	t.Parallel()
	k := newWhCountKit(t, "lugares_bodega")
	refriLocal, err := k.svc.CreateCountPlace(k.ctx, domain.WarehouseSel{}, "Refri")
	if err != nil {
		t.Fatal(err)
	}
	refriBodega, err := k.svc.CreateCountPlace(k.ctx, domain.WarehouseSel{ID: k.bodega}, "Refri")
	if err != nil {
		t.Fatalf("«Refri» en Bodega con «Refri» en Local = %v; el nombre es único por almacén", err)
	}
	var exists domain.CountPlaceExists
	if _, err := k.svc.CreateCountPlace(k.ctx, domain.WarehouseSel{ID: k.bodega}, "refri"); !errors.As(err, &exists) || exists.ID != refriBodega {
		t.Fatalf("otro «refri» en Bodega = %v; quiere el que ya existe", err)
	}
	places, err := k.svc.ListCountPlaces(k.ctx, domain.WarehouseSel{ID: k.bodega})
	if err != nil || len(places) != 1 || places[0].ID != refriBodega {
		t.Fatalf("los lugares de Bodega = %+v, %v; quiere solo su Refri", places, err)
	}
	if err := k.svc.SetCountPlace(k.ctx, k.leche, &refriLocal); err != nil {
		t.Fatal(err)
	}
	c, err := k.svc.OpenCount(k.ctx, domain.WarehouseSel{ID: k.bodega}, k.user)
	if err != nil {
		t.Fatal(err)
	}
	for i, it := range c.Items {
		if it.IngredientID != k.leche {
			continue
		}
		if !it.PlaceElsewhere || it.Place != nil || it.PlaceID != nil {
			t.Fatalf("la leche en el conteo de Bodega = %+v; quiere «sin lugar en este almacén»", it)
		}
		for _, after := range c.Items[i+1:] {
			if after.Place != nil {
				t.Fatalf("después de un artículo sin lugar viene %q con lugar: los sin lugar van al final", *after.Place)
			}
		}
		return
	}
	t.Fatal("la leche no sale en el conteo de Bodega")
}

// Lo de un almacén de otra empresa no se alcanza: ni su conteo, ni sus lugares, ni sus movimientos,
// ni registrar merma ahí.
func TestWarehouseCountsAreIsolatedInTheThreeCases(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	owner := makeCompany(t, st, "conteo-almacen-duena")
	other := makeCompany(t, st, "conteo-almacen-ajena")
	svc, ctx := asWaste(t, owner)
	bodega, err := svc.CreateWarehouse(ctx, "Bodega de la dueña")
	if err != nil {
		t.Fatal(err)
	}
	gerente := makeUserIn(t, st, owner, "gerente_conteo_duena", "gerente")
	if _, err := svc.OpenCount(ctx, domain.WarehouseSel{ID: bodega}, gerente); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CreateCountPlace(ctx, domain.WarehouseSel{ID: bodega}, "Anaquel"); err != nil {
		t.Fatal(err)
	}
	// Un motivo y un insumo de la empresa ajena: así la merma solo puede fallar por el almacén.
	var reason int64
	if err := st.Pool.QueryRow(context.Background(), `select id from waste_reasons
		where company_id = $1 and archived_at is null and not is_staff_meal order by id limit 1`, other).Scan(&reason); err != nil {
		t.Fatal(err)
	}
	otherIng := ingredientIn(t, st, other, "Leche ajena conteo", "ml")
	inTheThreeCases(t, owner, other, func(t *testing.T, as *store.Store, ctx context.Context) {
		s := app.NewBackofficeService(as, clock)
		if c, err := s.CurrentCount(ctx, domain.WarehouseSel{ID: bodega}); err == nil && c != nil {
			t.Fatalf("se vio el conteo de la bodega de otra empresa: %d", c.ID)
		}
		if _, err := s.OpenCount(ctx, domain.WarehouseSel{ID: bodega}, 1); err == nil {
			t.Fatal("se abrió un conteo en la bodega de otra empresa")
		}
		if ps, err := s.ListCountPlaces(ctx, domain.WarehouseSel{ID: bodega}); err == nil && len(ps) > 0 {
			t.Fatalf("se vieron los lugares de la bodega de otra empresa: %+v", ps)
		}
		if ms, err := s.StockMovements(ctx, domain.WarehouseSel{ID: bodega}, 10); err == nil && len(ms) > 0 {
			t.Fatalf("se vieron movimientos de la bodega de otra empresa: %d", len(ms))
		}
		_, err := s.RecordWaste(ctx, app.RecordWasteCmd{ReasonID: reason, IngredientID: &otherIng, Qty: decimal.NewFromInt(1), Actor: 1,
			Warehouse: domain.WarehouseSel{ID: bodega}})
		if err == nil {
			t.Fatal("se registró merma en la bodega de otra empresa")
		}
		// Desde la empresa ajena, con su motivo y su insumo, lo único que falla es el almacén: 404.
		if strings.Contains(t.Name(), "otra") && !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("merma en la bodega de otra empresa = %v; quiere 404 por el almacén", err)
		}
	})
}
