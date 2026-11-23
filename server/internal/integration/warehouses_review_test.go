//go:build integration

package integration

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"uuid"

	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/auth"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/cache"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/config"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/httpapi"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/realtime"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
)

// LO QUE ENCONTRARON LA REVISIÓN DE CÓDIGO Y LA PRUEBA CON ALGUIEN NUEVO EN LAS ENTREGAS 4 Y 5 DE LA
// SPEC 094. Cada prueba nombra el defecto que atrapa.

// Las alertas (encendidas, al mínimo, al acabarse, franja) son del artículo; el mínimo es de cada
// almacén. Guardar el mínimo de Bodega con los interruptores apagados apagaba el aviso de Local.
func TestSavingABodegaMinimumKeepsTheArticleSwitches(t *testing.T) {
	t.Parallel()
	k := newWhCountKit(t, "interruptores_bodega")
	ns, _, nctx := asNotifications(t, defaultCompanyID)
	tres := decimal.NewFromInt(3)
	if _, err := ns.ConfigureLowStock(nctx, "ingrediente", k.leche, app.LowStockInput{Enabled: true, OnMin: true, OnOut: true, Banner: true,
		MinStock: &tres}, k.user, false); err != nil {
		t.Fatal(err)
	}
	diez := decimal.NewFromInt(10)
	if _, err := ns.ConfigureLowStock(nctx, "ingrediente", k.leche, app.LowStockInput{
		BranchMinStock: app.Field[decimal.Decimal]{Set: true, Value: &diez}, Warehouse: domain.WarehouseSel{ID: k.bodega}}, k.user, false); err != nil {
		t.Fatal(err)
	}
	local, err := ns.LowStockConfig(nctx, "ingrediente", k.leche, domain.WarehouseSel{})
	if err != nil {
		t.Fatal(err)
	}
	if !local.Enabled || !local.OnMin || !local.OnOut || !local.Banner || local.MinStock == nil || !local.MinStock.Equal(tres) {
		t.Fatalf("guardar el mínimo de Bodega cambió las alertas del artículo: %+v; quiere encendidas y mínimo 3", local)
	}
	bodega, err := ns.LowStockConfig(nctx, "ingrediente", k.leche, domain.WarehouseSel{ID: k.bodega})
	if err != nil || bodega.BranchMin == nil || !bodega.BranchMin.Equal(diez) {
		t.Fatalf("el mínimo de Bodega = %+v, %v; quiere 10", bodega.BranchMin, err)
	}
	// Quitarlo lo quita (null explícito).
	if _, err := ns.ConfigureLowStock(nctx, "ingrediente", k.leche, app.LowStockInput{
		BranchMinStock: app.Field[decimal.Decimal]{Set: true}, Warehouse: domain.WarehouseSel{ID: k.bodega}}, k.user, false); err != nil {
		t.Fatal(err)
	}
	if bodega, err = ns.LowStockConfig(nctx, "ingrediente", k.leche, domain.WarehouseSel{ID: k.bodega}); err != nil || bodega.BranchMin != nil {
		t.Fatalf("quitar el mínimo de Bodega lo dejó en %v (%v)", bodega.BranchMin, err)
	}
}

// Nombrar el almacén de venta por su id es lo mismo que no nombrarlo: el lote escribía un mínimo
// propio del almacén y dejaba el del artículo, y la ficha divergía en silencio.
func TestTheSalesWarehouseByIDIsTheSalesWarehouse(t *testing.T) {
	t.Parallel()
	k := newWhCountKit(t, "venta_por_id")
	ns, _, nctx := asNotifications(t, defaultCompanyID)
	siete := decimal.NewFromInt(7)
	if _, err := ns.ConfigureBatch(nctx, []app.LowStockBatchItem{{ItemType: "ingrediente", ID: k.leche, Enabled: true, MinStock: &siete}},
		domain.WarehouseSel{ID: k.local}, k.user, false); err != nil {
		t.Fatal(err)
	}
	cfg, err := ns.LowStockConfig(nctx, "ingrediente", k.leche, domain.WarehouseSel{})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.MinStock == nil || !cfg.MinStock.Equal(siete) || cfg.BranchMin != nil {
		t.Fatalf("el lote con el id de Local dejó mínimo del artículo %v y propio %v; quiere 7 y ninguno", cfg.MinStock, cfg.BranchMin)
	}
}

// La misma llave de «se acabó» con otro almacén no es un reintento: devolvía el reporte de Local como
// si se hubiera hecho en Bodega. Y un almacén ajeno con una llave ya usada respondía el reporte viejo
// en vez de 404.
func TestAStockOutKeyIsTiedToItsWarehouse(t *testing.T) {
	t.Parallel()
	k := newWhCountKit(t, "llave_almacen")
	so, ctx := asStockOut(t, defaultCompanyID)
	key := uuid.New()
	if _, _, err := so.ReportOut(ctx, app.ReportOutCmd{ItemType: "ingrediente", ID: k.leche, ClientKey: key, Actor: k.user}); err != nil {
		t.Fatal(err)
	}
	_, _, err := so.ReportOut(ctx, app.ReportOutCmd{ItemType: "ingrediente", ID: k.leche, ClientKey: key, Actor: k.user,
		Warehouse: domain.WarehouseSel{ID: k.bodega}})
	if !errors.Is(err, domain.ErrClientKeyReused) {
		t.Fatalf("misma llave en Bodega = %v; quiere ErrClientKeyReused", err)
	}
	if v := k.onHand(t, k.bodega); !v.Equal(decimal.NewFromInt(20)) {
		t.Fatalf("Bodega quedó en %s; quiere 20 intacta", v)
	}
	_, _, err = so.ReportOut(ctx, app.ReportOutCmd{ItemType: "ingrediente", ID: k.leche, ClientKey: key, Actor: k.user,
		Warehouse: domain.WarehouseSel{ID: 999_999_999}})
	if !errors.Is(err, domain.ErrWarehouseNotFound) {
		t.Fatalf("misma llave con un almacén que no existe = %v; quiere 404", err)
	}
}

// Archivar un lugar mandando sus artículos a un lugar de otro almacén los sacaba del recorrido del
// suyo y los metía en uno que nunca los cuenta.
func TestArchivingAPlaceMovesItsArticlesWithinItsWarehouse(t *testing.T) {
	t.Parallel()
	k := newWhCountKit(t, "archivar_lugar")
	anaquel, err := k.svc.CreateCountPlace(k.ctx, domain.WarehouseSel{ID: k.bodega}, "Anaquel")
	if err != nil {
		t.Fatal(err)
	}
	refri, err := k.svc.CreateCountPlace(k.ctx, domain.WarehouseSel{}, "Refri")
	if err != nil {
		t.Fatal(err)
	}
	if err := k.svc.SetCountPlace(k.ctx, k.leche, &anaquel); err != nil {
		t.Fatal(err)
	}
	if err := k.svc.ArchiveCountPlaceMoving(k.ctx, anaquel, &refri); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("archivar el Anaquel de Bodega mandando todo al Refri de Local = %v; quiere ErrValidation", err)
	}
	var place *int64
	if err := k.st.Pool.QueryRow(context.Background(), `select count_place_id from ingredients where id = $1`, k.leche).Scan(&place); err != nil {
		t.Fatal(err)
	}
	if place == nil || *place != anaquel {
		t.Fatalf("la leche quedó en el lugar %v; quiere seguir en el Anaquel", place)
	}
}

// Existencias de Bodega mostraban el mínimo del artículo (el de Local) y marcaban «bajo» con él; el
// mínimo de un almacén que no es el de venta es solo el suyo. En «Todos» no hay un mínimo que valga.
// Y Local nombrado por su id trae de dónde traer igual que sin nombrarlo.
func TestStockLevelsShowTheMinimumOfTheWarehouse(t *testing.T) {
	t.Parallel()
	k := newTransferKit(t)
	ns, _, nctx := asNotifications(t, defaultCompanyID)
	tres, veinticinco := decimal.NewFromInt(3), decimal.NewFromInt(25)
	if _, err := ns.ConfigureLowStock(nctx, "ingrediente", k.leche, app.LowStockInput{Enabled: true, OnMin: true, MinStock: &tres}, 1, false); err != nil {
		t.Fatal(err)
	}
	if _, err := ns.ConfigureLowStock(nctx, "ingrediente", k.leche, app.LowStockInput{
		BranchMinStock: app.Field[decimal.Decimal]{Set: true, Value: &veinticinco}, Warehouse: domain.WarehouseSel{ID: k.bodega}}, 1, false); err != nil {
		t.Fatal(err)
	}
	adjustIn(t, k.st, k.local, "ingrediente", k.leche, "-15")
	row := func(sel domain.WarehouseSel) app.StockLevelView {
		t.Helper()
		rows, err := k.svc.StockLevels(k.ctx, sel)
		if err != nil {
			t.Fatal(err)
		}
		for _, r := range rows {
			if r.IngredientID != nil && *r.IngredientID == k.leche {
				return r
			}
		}
		t.Fatalf("la leche no sale en %+v", sel)
		return app.StockLevelView{}
	}
	min := func(r app.StockLevelView) string {
		if r.MinStock == nil {
			return "ninguno"
		}
		return r.MinStock.String()
	}
	if r := row(domain.WarehouseSel{}); min(r) != "3" || len(r.TransferFrom) != 1 {
		t.Fatalf("Local: mínimo %s y %d de dónde traer; quiere 3 y Bodega", min(r), len(r.TransferFrom))
	}
	if r := row(domain.WarehouseSel{ID: k.local}); min(r) != "3" || len(r.TransferFrom) != 1 {
		t.Fatalf("Local por su id: mínimo %s y %d de dónde traer; quiere lo mismo que sin nombrarlo", min(r), len(r.TransferFrom))
	}
	if r := row(domain.WarehouseSel{ID: k.bodega}); min(r) != "25" {
		t.Fatalf("Bodega: mínimo %s; quiere 25, el suyo", min(r))
	}
	if r := row(domain.WarehouseSel{All: true}); min(r) != "ninguno" {
		t.Fatalf("Todos: mínimo %s; los almacenes sumados no tienen un mínimo", min(r))
	}
	// Valor por almacén: Local en −5 no vale nada y Bodega 20 ml a $0.025 vale 0.50; juntos, 0.50.
	vals, err := k.svc.StockValueByWarehouse(k.ctx)
	if err != nil {
		t.Fatal(err)
	}
	got := map[int64]string{}
	for _, v := range vals {
		got[v.WarehouseID] = v.Value.StringFixed(2)
	}
	if got[k.local] != "0.00" || got[k.bodega] != "0.50" {
		t.Fatalf("el valor por almacén = %v; quiere Local 0.00 y Bodega 0.50", got)
	}
}

// Por el router real: un campo de almacén mal escrito en el cuerpo (warehouse_id en vez de
// warehouseId) restaba del almacén de venta sin avisar; un ?warehouseId= vacío leía el de venta; y
// abrir un conteo con un cuerpo vacío sin largo declarado (chunked) daba 400.
func TestWarehouseInputsAtTheBoundary(t *testing.T) {
	t.Parallel()
	k := newWhCountKit(t, "frontera_almacen")
	jm := auth.NewManager("secreto-de-pruebas-suficientemente-largo-para-el-manager", nil)
	h := httpapi.NewHandlers(httpapi.Deps{JWT: jm, Admin: app.NewAdminService(k.st), Backoffice: app.NewBackofficeService(k.st, nil),
		Notifications: app.NewNotificationsService(k.st, clock), MenuCache: cache.NewMenuCache(""), Broker: realtime.NewBroker()})
	r := httpapi.Router(config.Config{}, jm, h, k.st)
	tok, err := jm.Issue(domain.User{ID: k.user, CompanyID: defaultCompanyID, Name: "gerente", Role: "gerente"})
	if err != nil {
		t.Fatal(err)
	}
	var reason int64
	if err := k.st.Pool.QueryRow(context.Background(), `select id from waste_reasons where company_id = $1 and archived_at is null and not is_staff_meal limit 1`, defaultCompanyID).Scan(&reason); err != nil {
		t.Fatal(err)
	}
	ct := "application/json"
	waste := fmt.Sprintf(`{"reasonId":%d,"ingredientId":%d,"qty":"1","warehouse_id":%d}`, reason, k.leche, k.bodega)
	if w := do(t, r, http.MethodPost, "/api/v1/waste", tok, []byte(waste), ct); w.Code != http.StatusBadRequest {
		t.Errorf("merma con warehouse_id = %d %s; quiere 400", w.Code, w.Body.String())
	}
	if v := k.onHand(t, k.local); !v.Equal(decimal.NewFromInt(10)) {
		t.Errorf("la merma mal escrita restó de Local: quedó en %s", v)
	}
	if w := do(t, r, http.MethodPost, "/api/v1/stock/count-places", tok, []byte(`{"name":"Anaquel","warehouse_id":1}`), ct); w.Code != http.StatusBadRequest {
		t.Errorf("lugar con warehouse_id = %d; quiere 400", w.Code)
	}
	if w := do(t, r, http.MethodGet, "/api/v1/stock/movements?warehouseId=", tok, nil, ""); w.Code != http.StatusBadRequest {
		t.Errorf("movimientos con ?warehouseId= vacío = %d; quiere 400", w.Code)
	}
	// El 400 dice qué campo espera: «datos inválidos» a secas no dice que el almacén va en warehouseId.
	if w := do(t, r, http.MethodPost, "/api/v1/stock/counts", tok, []byte(`{"warehouse_id":1}`), ct); w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "warehouseId") {
		t.Errorf("abrir conteo con warehouse_id = %d %s; quiere 400 que nombre warehouseId", w.Code, w.Body.String())
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/stock/counts", io.NopCloser(strings.NewReader("")))
	req.ContentLength = -1
	req.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code >= 300 {
		t.Errorf("abrir conteo con cuerpo vacío sin largo = %d %s; quiere que abra el de venta", w.Code, w.Body.String())
	}
}

// El conteo de Bodega listaba todos los artículos del negocio como «sin lugar en este almacén» (520
// en la base restaurada) aunque la Bodega tuviera dos. Ahora cada renglón dice si es de este almacén
// (tiene existencia, lugar o ya se capturó aquí); los demás siguen en la lista para buscarlos, porque
// el primer conteo de una Bodega nueva es justo el que carga lo que tiene. «Sin contar» solo cuenta
// los de este almacén.
func TestABodegaCountMarksWhatBelongsToIt(t *testing.T) {
	t.Parallel()
	k := newWhCountKit(t, "conteo_propio")
	soloLocal := ingredientIn(t, k.st, defaultCompanyID, "Cajeta conteo_propio", "g")
	adjustIn(t, k.st, k.local, "ingrediente", soloLocal, "5")
	enLugar := ingredientIn(t, k.st, defaultCompanyID, "Harina conteo_propio", "g")
	anaquel, err := k.svc.CreateCountPlace(k.ctx, domain.WarehouseSel{ID: k.bodega}, "Anaquel")
	if err != nil {
		t.Fatal(err)
	}
	if err := k.svc.SetCountPlace(k.ctx, enLugar, &anaquel); err != nil {
		t.Fatal(err)
	}
	c, err := k.svc.OpenCount(k.ctx, domain.WarehouseSel{ID: k.bodega}, k.user)
	if err != nil {
		t.Fatal(err)
	}
	in := map[int64]bool{}
	for _, it := range c.Items {
		in[it.IngredientID] = it.InWarehouse
	}
	if !in[k.leche] || !in[enLugar] || in[soloLocal] {
		t.Fatalf("de Bodega: leche %v, harina con lugar ahí %v, cajeta solo de Local %v; quiere true, true, false", in[k.leche], in[enLugar], in[soloLocal])
	}
	// Capturar uno que no es de Bodega (se encontró buscando) lo vuelve de Bodega.
	if err := k.svc.CaptureCount(k.ctx, c.ID, soloLocal, decimal.NewFromInt(2), k.user); err != nil {
		t.Fatal(err)
	}
	cur, err := k.svc.CurrentCount(k.ctx, domain.WarehouseSel{ID: k.bodega})
	if err != nil || cur == nil {
		t.Fatal(err)
	}
	for _, it := range cur.Items {
		if it.IngredientID == soloLocal && !it.InWarehouse {
			t.Fatal("la cajeta capturada en Bodega no quedó como de Bodega")
		}
	}
	prev, err := k.svc.PreviewCount(k.ctx, c.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range prev.NotCounted {
		if n.IngredientID != k.leche && n.IngredientID != enLugar {
			var bodegaHas bool
			if err := k.st.Pool.QueryRow(context.Background(), `select exists (select 1 from stock_levels where warehouse_id = $1 and ingredient_id = $2 and on_hand <> 0)`,
				k.bodega, n.IngredientID).Scan(&bodegaHas); err != nil {
				t.Fatal(err)
			}
			if !bodegaHas {
				t.Fatalf("«sin contar» en Bodega trae %q, que no es de Bodega", n.Name)
			}
		}
	}
	// En el de venta, todo es de este almacén: ahí se cuenta todo lo que se vende.
	if err := k.svc.DiscardCount(k.ctx, c.ID, k.user); err != nil {
		t.Fatal(err)
	}
	cl, err := k.svc.OpenCount(k.ctx, domain.WarehouseSel{}, k.user)
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range cl.Items {
		if !it.InWarehouse {
			t.Fatalf("en el conteo de Local, %q salió como de otro almacén", it.Name)
		}
	}
}

// La lista de «se acabó» de hoy no decía de qué almacén era cada reporte, aunque la consulta ya lo
// traía.
func TestTodaysStockOutsNameTheirWarehouse(t *testing.T) {
	t.Parallel()
	k := newWhCountKit(t, "hoy_almacen")
	so, ctx := asStockOut(t, defaultCompanyID)
	if _, _, err := so.ReportOut(ctx, app.ReportOutCmd{ItemType: "ingrediente", ID: k.leche, ClientKey: uuid.New(), Actor: k.user,
		Warehouse: domain.WarehouseSel{ID: k.bodega}}); err != nil {
		t.Fatal(err)
	}
	sug, err := so.Suggestions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range sug.Today {
		if r.Name == "Leche hoy_almacen" {
			if r.WarehouseName != "Bodega" {
				t.Fatalf("el reporte de hoy dice almacén %q; quiere Bodega", r.WarehouseName)
			}
			return
		}
	}
	t.Fatal("el reporte no sale en los de hoy")
}

// Un traspaso con un almacén que después se archivó decía «← Bodega» igual que uno con la Bodega
// nueva del mismo nombre: no se distinguía de cuál venía.
func TestAMovementSaysWhenItsCounterpartIsArchived(t *testing.T) {
	t.Parallel()
	k := newTransferKit(t)
	if _, err := k.svc.CreateTransfer(k.ctx, k.in(k.bodega, k.local, "20"), k.user); err != nil {
		t.Fatal(err)
	}
	if err := k.svc.ArchiveWarehouse(k.ctx, k.bodega); err != nil {
		t.Fatal(err)
	}
	ms, err := k.svc.StockMovements(k.ctx, domain.WarehouseSel{}, 50)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range ms {
		if m.MovementType == "traspaso" {
			if !m.CounterpartWarehouseArchived {
				t.Fatalf("la pata que entró a Local viene de una Bodega archivada y no lo dice: %+v", m)
			}
			return
		}
	}
	t.Fatal("el traspaso no sale en los movimientos de Local")
}

// Las lecturas y escrituras por almacén que no tenían su prueba en los tres casos: con el id de la
// bodega de otra empresa, ninguna alcanza nada. Desde la empresa ajena, con un artículo suyo, lo único
// que puede fallar es el almacén: tiene que ser 404, no otro error que tape un hueco.
func TestWarehouseReadsAndWritesAreIsolatedInTheThreeCases(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	owner := makeCompany(t, st, "almacen-lecturas-duena")
	other := makeCompany(t, st, "almacen-lecturas-ajena")
	svc, ctx := asWaste(t, owner)
	bodega, err := svc.CreateWarehouse(ctx, "Bodega de la dueña")
	if err != nil {
		t.Fatal(err)
	}
	otherIng := ingredientIn(t, st, other, "Leche ajena lecturas", "ml")
	inTheThreeCases(t, owner, other, func(t *testing.T, as *store.Store, ctx context.Context) {
		fromOther := strings.Contains(t.Name(), "otra")
		mustFail := func(what string, err error) {
			t.Helper()
			if err == nil {
				t.Fatalf("%s en la bodega de otra empresa no falló", what)
			}
			if fromOther && !errors.Is(err, domain.ErrNotFound) {
				t.Fatalf("%s en la bodega de otra empresa = %v; quiere 404 por el almacén", what, err)
			}
		}
		sel := domain.WarehouseSel{ID: bodega}
		ns := app.NewNotificationsService(as, clock)
		bo := app.NewBackofficeService(as, clock)
		so := app.NewStockOutService(as, clock)
		_, err := ns.ShoppingList(ctx, sel)
		mustFail("la lista de compra", err)
		_, err = ns.ListLowStock(ctx, sel)
		mustFail("las alertas por lote", err)
		_, err = ns.LowStockConfig(ctx, "ingrediente", otherIng, sel)
		mustFail("la ficha de alertas", err)
		uno := decimal.NewFromInt(1)
		_, err = ns.ConfigureLowStock(ctx, "ingrediente", otherIng, app.LowStockInput{
			BranchMinStock: app.Field[decimal.Decimal]{Set: true, Value: &uno}, Warehouse: sel}, 1, false)
		mustFail("fijar un mínimo", err)
		_, err = ns.ConfigureBatch(ctx, []app.LowStockBatchItem{{ItemType: "ingrediente", ID: otherIng, Enabled: true, MinStock: &uno}}, sel, 1, false)
		mustFail("el lote de mínimos", err)
		_, _, err = so.ReportOut(ctx, app.ReportOutCmd{ItemType: "ingrediente", ID: otherIng, ClientKey: uuid.New(), Actor: 1, Warehouse: sel})
		mustFail("«se acabó»", err)
		_, err = bo.LastClosedCount(ctx, sel)
		mustFail("el último conteo", err)
		if rows, err := bo.StockLevels(ctx, domain.WarehouseSel{All: true}); err == nil {
			for _, r := range rows {
				if r.IngredientID != nil && *r.IngredientID != otherIng && fromOther {
					var owned int64
					if err := st.Pool.QueryRow(context.Background(), `select company_id from ingredients where id = $1`, *r.IngredientID).Scan(&owned); err == nil && owned == owner {
						t.Fatalf("«Todos» trajo un artículo de la dueña: %s", r.ItemName)
					}
				}
			}
		}
	})
}

// Quien registra merma o «se acabó» sin ver el inventario (un cajero) no podía leer la lista de
// almacenes, así que su pantalla no le ofrecía elegir y todo caía en el de venta. Ahora lee los
// nombres; cuántos artículos tiene cada uno y los archivados siguen siendo de quien ve el inventario.
func TestACashierCanListWarehouseNamesOnly(t *testing.T) {
	t.Parallel()
	k := newWhCountKit(t, "cajero_almacenes")
	jm := auth.NewManager("secreto-de-pruebas-suficientemente-largo-para-el-manager", nil)
	h := httpapi.NewHandlers(httpapi.Deps{JWT: jm, Admin: app.NewAdminService(k.st), Backoffice: app.NewBackofficeService(k.st, nil),
		MenuCache: cache.NewMenuCache(""), Broker: realtime.NewBroker()})
	r := httpapi.Router(config.Config{}, jm, h, k.st)
	cajero := makeUser(t, k.st, "cajero_almacenes", "cajero")
	tok, err := jm.Issue(domain.User{ID: cajero, CompanyID: defaultCompanyID, Name: "cajero", Role: "cajero"})
	if err != nil {
		t.Fatal(err)
	}
	w := do(t, r, http.MethodGet, "/api/v1/stock/warehouses/names", tok, nil, "")
	if w.Code != http.StatusOK {
		t.Fatalf("el cajero pide los nombres de almacén = %d %s; quiere 200", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "itemsWithStock") || !strings.Contains(w.Body.String(), `"Bodega"`) {
		t.Fatalf("los nombres para el cajero = %s; quiere la Bodega y nada de existencias", w.Body.String())
	}
	if w := do(t, r, http.MethodGet, "/api/v1/stock/warehouses", tok, nil, ""); w.Code != http.StatusForbidden {
		t.Fatalf("el cajero pide la lista con existencias = %d; quiere 403", w.Code)
	}
}

// Los nombres de almacén (los lee hasta un cajero) tenían que salir de una consulta que no cuenta
// existencias, y aislada como cualquier lectura de empresa: desde la ajena, con una conexión
// reciclada o sin empresa, la Bodega de la dueña no aparece.
func TestWarehouseNamesAreIsolatedInTheThreeCases(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	owner := makeCompany(t, st, "almacen-nombres-duena")
	other := makeCompany(t, st, "almacen-nombres-ajena")
	svc, ctx := asWaste(t, owner)
	bodega, err := svc.CreateWarehouse(ctx, "Bodega de nombres")
	if err != nil {
		t.Fatal(err)
	}
	mine, err := svc.WarehouseNames(ctx)
	if err != nil || len(mine) != 2 || !mine[0].IsSales || mine[1].ID != bodega || mine[1].Name != "Bodega de nombres" {
		t.Fatalf("los nombres de la dueña = %+v (%v); quiere el de venta y la Bodega", mine, err)
	}
	inTheThreeCases(t, owner, other, func(t *testing.T, as *store.Store, ctx context.Context) {
		ws, err := app.NewBackofficeService(as, clock).WarehouseNames(ctx)
		if err != nil {
			return
		}
		for _, w := range ws {
			if w.ID == bodega {
				t.Fatalf("se ve el nombre de la bodega de otra empresa: %+v", w)
			}
		}
	})
}
