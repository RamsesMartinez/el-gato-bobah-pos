//go:build integration

package integration

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"uuid"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/auth"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/cache"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/config"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/httpapi"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/realtime"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
)

// CIERRE DEL #65 (spec 094): lo que quedó midiendo por sucursal cuando ya había varios almacenes.

// La búsqueda de «se acabó» marcaba «ya reportado» por sucursal: un artículo reportado en Bodega
// salía como reportado en Local, y quien veía que se acabó en el local no lo podía avisar.
func TestStockOutSearchMarksReportedPerWarehouse(t *testing.T) {
	t.Parallel()
	k := newWhCountKit(t, "busca_almacen")
	so, ctx := asStockOut(t, defaultCompanyID)
	if _, _, err := so.ReportOut(ctx, app.ReportOutCmd{ItemType: "ingrediente", ID: k.leche, ClientKey: uuid.New(), Actor: k.user,
		Warehouse: domain.WarehouseSel{ID: k.bodega}}); err != nil {
		t.Fatal(err)
	}
	openIn := func(sel domain.WarehouseSel) bool {
		t.Helper()
		items, err := so.Search(ctx, "Leche busca_almacen", sel)
		if err != nil {
			t.Fatal(err)
		}
		for _, it := range items {
			if it.ID == k.leche {
				return it.OpenOut
			}
		}
		t.Fatal("la búsqueda no trae la leche")
		return false
	}
	if openIn(domain.WarehouseSel{}) {
		t.Fatal("la leche reportada en Bodega sale como «ya reportado» en Local (el de venta)")
	}
	if openIn(domain.WarehouseSel{ID: k.local}) {
		t.Fatal("la leche reportada en Bodega sale como «ya reportado» en Local por su id")
	}
	if !openIn(domain.WarehouseSel{ID: k.bodega}) {
		t.Fatal("la leche reportada en Bodega no sale como «ya reportado» en Bodega")
	}
	if _, err := so.Search(ctx, "Leche", domain.WarehouseSel{ID: 999_999_999}); !errors.Is(err, domain.ErrWarehouseNotFound) {
		t.Fatalf("buscar en un almacén que no existe = %v; quiere 404", err)
	}
}

// Por el router: ?warehouseId= vacío o malformado es 400 (nunca el de venta en silencio), uno ajeno
// es 404, y sin el parámetro busca en el de venta.
func TestStockOutSearchWarehouseAtTheBoundary(t *testing.T) {
	t.Parallel()
	k := newWhCountKit(t, "busca_frontera")
	jm := auth.NewManager("secreto-de-pruebas-suficientemente-largo-para-el-manager", nil)
	h := httpapi.NewHandlers(httpapi.Deps{JWT: jm, Admin: app.NewAdminService(k.st), Backoffice: app.NewBackofficeService(k.st, nil),
		StockOut: app.NewStockOutService(k.st, clock), MenuCache: cache.NewMenuCache(""), Broker: realtime.NewBroker()})
	r := httpapi.Router(config.Config{}, jm, h, k.st)
	tok, err := jm.Issue(domain.User{ID: k.user, CompanyID: defaultCompanyID, Name: "gerente", Role: "gerente"})
	if err != nil {
		t.Fatal(err)
	}
	base := "/api/v1/stock/out-reports/search?q=Leche"
	for _, c := range []struct {
		query string
		want  int
	}{
		{"", http.StatusOK},
		{"&warehouseId=", http.StatusBadRequest},
		{"&warehouseId=abc", http.StatusBadRequest},
		{"&warehouseId=0", http.StatusBadRequest},
		{"&warehouseId=all", http.StatusBadRequest},
		{"&warehouseId=999999999", http.StatusNotFound},
		{fmt.Sprintf("&warehouseId=%d", k.bodega), http.StatusOK},
	} {
		if w := do(t, r, http.MethodGet, base+c.query, tok, nil, ""); w.Code != c.want {
			t.Errorf("buscar con %q = %d %s; quiere %d", c.query, w.Code, w.Body.String(), c.want)
		}
	}
}

// Buscar con el almacén de otra empresa no alcanza nada en ninguno de los tres casos; desde la
// ajena, el único error posible es el 404 del almacén.
func TestStockOutSearchIsIsolatedInTheThreeCases(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	owner := makeCompany(t, st, "busca-almacen-duena")
	other := makeCompany(t, st, "busca-almacen-ajena")
	svc, ctx := asWaste(t, owner)
	bodega, err := svc.CreateWarehouse(ctx, "Bodega de búsqueda")
	if err != nil {
		t.Fatal(err)
	}
	ingredientIn(t, st, other, "Leche ajena búsqueda", "ml")
	inTheThreeCases(t, owner, other, func(t *testing.T, as *store.Store, ctx context.Context) {
		_, err := app.NewStockOutService(as, clock).Search(ctx, "Leche", domain.WarehouseSel{ID: bodega})
		if err == nil {
			t.Fatal("se buscó en la bodega de otra empresa")
		}
		if strings.Contains(t.Name(), "otra") && !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("buscar en la bodega de otra empresa = %v; quiere 404 por el almacén", err)
		}
	})
}
