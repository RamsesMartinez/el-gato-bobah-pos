//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"
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

// La bitácora de una compra solo decía a qué almacén iba cada renglón al recibirlo: una compra
// capturada sin que llegara nada (o que llegó completa al crearla) no dejaba rastro de su destino, y
// «¿quién mandó esto a Bodega?» no tenía respuesta. Ahora el alta lo registra por renglón.
func TestCreatingAPurchaseAuditsTheWarehouseOfEachLine(t *testing.T) {
	t.Parallel()
	k := newPurchaseKit(t)
	local, bodega := comoEnLaSegundaSubida(t, k.st, defaultCompanyID)
	in := k.bultos("2", "1000")
	zero := dd("0")
	in.Items[0].QtyReceived = &zero // pendiente: nada llega al crearla
	in.Items[0].WarehouseID = &bodega
	otro := in.Items[0]
	otro.Description, otro.WarehouseID, otro.QtyReceived = "Azúcar del local", nil, nil
	in.Items = append(in.Items, otro, app.ExpenseItemInput{Description: "Bolsa", Quantity: dd("1"), Amount: dd("10")})
	in.Items[1].Amount, in.Amount = dd("500"), dd("1510")
	id, err := k.svc.CreateExpense(k.ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	var raw string
	if err := k.st.Pool.QueryRow(context.Background(), `select coalesce(after->'lines', 'null')::text from audit_events
		where action = 'expense.created' and entity_id = $1`, id).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var lines []struct {
		Position    int    `json:"position"`
		Description string `json:"description"`
		WarehouseID *int64 `json:"warehouseId"`
	}
	if err := json.Unmarshal([]byte(raw), &lines); err != nil {
		t.Fatalf("la bitácora del alta no trae los renglones: %s (%v)", raw, err)
	}
	want := map[int]int64{0: bodega, 1: local}
	if len(lines) != len(want) {
		t.Fatalf("la bitácora del alta trae %d renglones con almacén: %s; quiere los 2 del almacén", len(lines), raw)
	}
	for _, l := range lines {
		if l.WarehouseID == nil || *l.WarehouseID != want[l.Position] {
			t.Fatalf("el renglón %d (%s) dice almacén %v; quiere %d (%s)", l.Position, l.Description, l.WarehouseID, want[l.Position], raw)
		}
	}
}

// openOutIn abre un «se acabó» de un insumo nuevo en un almacén dado y fija su hora en la base.
func (k *mailerKit) openOutIn(t *testing.T, warehouse int64, name string, at time.Time) {
	t.Helper()
	ing := pieceIngredient(t, k.st, name)
	so, ctx := asStockOut(t, defaultCompanyID)
	if _, _, err := so.ReportOut(ctx, app.ReportOutCmd{ItemType: "ingrediente", ID: ing, ClientKey: uuid.New(), Actor: k.user,
		Warehouse: domain.WarehouseSel{ID: warehouse}}); err != nil {
		t.Fatal(err)
	}
	if _, err := k.st.Pool.Exec(context.Background(),
		`update notifications set opened_at = $2 where ingredient_id = $1 and status = 'open'`, ing, at); err != nil {
		t.Fatal(err)
	}
}

// itemLine es la tarjeta del correo de un artículo: desde su nombre hasta la tarjeta siguiente.
func itemLine(t *testing.T, html, name string) string {
	t.Helper()
	i := strings.Index(html, name)
	if i < 0 {
		t.Fatalf("el correo no trae %s", name)
	}
	rest := html[i:]
	if j := strings.Index(rest, "border-left:4px"); j > 0 {
		rest = rest[:j]
	}
	return rest
}

// Con Local y Bodega, un correo que decía «Leche: se acabó» no decía DÓNDE: quien compra iba a
// surtir el local cuando lo vacío era la bodega. Con dos o más almacenes activos cada aviso nombra
// el suyo, en el correo por bloque y en el resumen de la mañana.
func TestStockEmailsNameTheWarehouseWithTwoOrMore(t *testing.T) {
	t.Parallel()
	k := newMailerKit(t)
	ctx := context.Background()
	_, bodega := comoEnLaSegundaSubida(t, k.st, defaultCompanyID)
	k.openAt(t, "Vasos cierre", k.block.Add(time.Minute))
	k.openOutIn(t, bodega, "Popotes cierre", k.block.Add(2*time.Minute))
	if err := k.mailer(k.block.Add(6*time.Minute)).SendWindows(ctx, defaultCompanyID); err != nil {
		t.Fatal(err)
	}
	if k.box.count() != 1 {
		t.Fatalf("salieron %d correos; quiere 1", k.box.count())
	}
	html := k.box.sent[0].html
	if l := itemLine(t, html, "Vasos cierre"); !strings.Contains(l, "En Local") {
		t.Fatalf("el aviso de Local no dice su almacén: %s", l)
	}
	if l := itemLine(t, html, "Popotes cierre"); !strings.Contains(l, "En Bodega") {
		t.Fatalf("el aviso de Bodega no dice su almacén: %s", l)
	}
	morning := time.Date(2026, 11, 15, 14, 5, 0, 0, time.UTC)
	if err := k.mailer(morning).SendMorning(ctx, defaultCompanyID); err != nil {
		t.Fatal(err)
	}
	if k.box.count() != 2 {
		t.Fatalf("el resumen de la mañana no salió: %d correos", k.box.count())
	}
	html = k.box.sent[1].html
	if l := itemLine(t, html, "Vasos cierre"); !strings.Contains(l, "En Local") {
		t.Fatalf("el resumen no dice el almacén del aviso de Local: %s", l)
	}
	if l := itemLine(t, html, "Popotes cierre"); !strings.Contains(l, "En Bodega") {
		t.Fatalf("el resumen no dice el almacén del aviso de Bodega: %s", l)
	}
}

// Con un solo almacén el texto queda como antes: nombrar «Local» en cada renglón es ruido para el
// negocio que nunca dio de alta otro.
func TestStockEmailsWithOneWarehouseDoNotNameIt(t *testing.T) {
	t.Parallel()
	k := newMailerKit(t)
	ctx := context.Background()
	k.openAt(t, "Tapas cierre", k.block.Add(time.Minute))
	if err := k.mailer(k.block.Add(6*time.Minute)).SendWindows(ctx, defaultCompanyID); err != nil {
		t.Fatal(err)
	}
	morning := time.Date(2026, 11, 15, 14, 5, 0, 0, time.UTC)
	if err := k.mailer(morning).SendMorning(ctx, defaultCompanyID); err != nil {
		t.Fatal(err)
	}
	if k.box.count() != 2 {
		t.Fatalf("salieron %d correos; quiere el del bloque y el de la mañana", k.box.count())
	}
	for _, m := range k.box.sent {
		if l := itemLine(t, m.html, "Tapas cierre"); strings.Contains(l, "En Local") {
			t.Fatalf("con un solo almacén el correo «%s» lo nombra: %s", m.subject, l)
		}
	}
}
