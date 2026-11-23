//go:build integration

package integration

import (
	"context"
	"encoding/json"
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

// LA LISTA DE COMPRA DE TODOS LOS ALMACENES (prueba con alguien nuevo, spec 094). La pantalla pedía
// la lista sin almacén y solo veía el de venta: la campana mandaba a la lista desde un «se acabó» de
// Bodega y la lista salía vacía. Y la lista de Bodega aplicaba el mínimo del artículo, que es del
// almacén de venta: con Bodega en 0, salían ahí todos los artículos con mínimo.

type shoppingAllKit struct {
	st                      *store.Store
	local, bodega           int64
	arizona, leche, popotes int64
	user                    int64
}

// newShoppingAllKit: Arizona se acabó en Bodega (reportado a mano); la leche tiene mínimo de artículo
// 100 y hay 500 en Local, nada en Bodega y Bodega sin mínimo propio; los popotes tienen mínimo propio
// de 50 en Bodega y hay 10 ahí.
func newShoppingAllKit(t *testing.T) *shoppingAllKit {
	t.Helper()
	st := newTestStore(t)
	k := &shoppingAllKit{st: st}
	k.local, k.bodega = comoEnLaSegundaSubida(t, st, defaultCompanyID)
	k.user = makeUser(t, st, "compra_todos", "gerente")
	k.arizona = pieceIngredient(t, st, "Arizona todos")
	adjustIn(t, st, k.local, "ingrediente", k.arizona, "12")
	adjustIn(t, st, k.bodega, "ingrediente", k.arizona, "3")
	k.leche = pieceIngredient(t, st, "Leche todos")
	alertOn(t, st, k.leche, "100", true, true)
	adjustIn(t, st, k.local, "ingrediente", k.leche, "500")
	k.popotes = pieceIngredient(t, st, "Popotes todos")
	alertOn(t, st, k.popotes, "", true, false)
	adjustIn(t, st, k.bodega, "ingrediente", k.popotes, "10")
	if _, err := st.Pool.Exec(context.Background(), `
		insert into branch_stock_minimums (branch_id, warehouse_id, item_type, ingredient_id, min_stock, updated_by)
		select w.branch_id, w.id, 'ingrediente', $2, 50, $3 from warehouses w where w.id = $1`, k.bodega, k.popotes, k.user); err != nil {
		t.Fatal(err)
	}
	_, so, ctx := asNotifications(t, defaultCompanyID)
	if _, _, err := so.ReportOut(ctx, app.ReportOutCmd{ItemType: "ingrediente", ID: k.arizona, ClientKey: uuid.New(), Actor: k.user,
		Warehouse: domain.WarehouseSel{ID: k.bodega}}); err != nil {
		t.Fatal(err)
	}
	return k
}

// linesOf dice en qué almacenes sale un insumo en la lista.
func linesOf(lines []app.ShoppingLine, ing int64) map[int64]app.ShoppingLine {
	out := map[int64]app.ShoppingLine{}
	for _, l := range lines {
		if l.ItemType == "ingrediente" && l.ID == ing {
			out[l.WarehouseID] = l
		}
	}
	return out
}

func TestTheShoppingListOfAllWarehousesNamesEach(t *testing.T) {
	t.Parallel()
	k := newShoppingAllKit(t)
	ns, _, ctx := asNotifications(t, defaultCompanyID)

	all, err := ns.ShoppingList(ctx, domain.WarehouseSel{All: true})
	if err != nil {
		t.Fatal(err)
	}
	az := linesOf(all, k.arizona)
	if l, ok := az[k.bodega]; !ok || l.WarehouseName != "Bodega" || l.State != "out" {
		t.Fatalf("en «Todos» el «se acabó» de Arizona en Bodega = %+v; quiere un renglón de Bodega, «se acabó»", az)
	}
	if _, ok := az[k.local]; ok {
		t.Fatal("Arizona sale en Local, donde hay 12 y nadie dijo que se acabó")
	}
	if l, ok := linesOf(all, k.popotes)[k.bodega]; !ok || l.Min == nil || l.Min.String() != "50" {
		t.Fatalf("los popotes bajo el mínimo propio de Bodega no salen con ese mínimo: %+v", linesOf(all, k.popotes))
	}
	if got := linesOf(all, k.leche); len(got) != 0 {
		t.Fatalf("la leche sale en la lista en los almacenes %v: Bodega no tiene mínimo propio y el del artículo es del almacén de venta", keys(got))
	}

	sales, err := ns.ShoppingList(ctx, domain.WarehouseSel{})
	if err != nil {
		t.Fatal(err)
	}
	for _, l := range sales {
		if l.WarehouseID != k.local {
			t.Fatalf("sin almacén la lista trae un renglón del almacén %d; sigue siendo la del de venta", l.WarehouseID)
		}
	}
}

func keys(m map[int64]app.ShoppingLine) []int64 {
	out := []int64{}
	for k := range m {
		out = append(out, k)
	}
	return out
}

// «Todos» por la frontera: ?warehouseId=all es 200 y trae el renglón de Bodega; vacío o malformado
// sigue siendo 400.
func TestShoppingListAllAtTheBoundary(t *testing.T) {
	t.Parallel()
	k := newShoppingAllKit(t)
	jm := auth.NewManager("secreto-de-pruebas-suficientemente-largo-para-el-manager", nil)
	h := httpapi.NewHandlers(httpapi.Deps{JWT: jm, Admin: app.NewAdminService(k.st), Backoffice: app.NewBackofficeService(k.st, nil),
		Notifications: app.NewNotificationsService(k.st, clock), MenuCache: cache.NewMenuCache(""), Broker: realtime.NewBroker()})
	r := httpapi.Router(config.Config{}, jm, h, k.st)
	tok, err := jm.Issue(domain.User{ID: k.user, CompanyID: defaultCompanyID, Name: "gerente", Role: "gerente"})
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		query string
		want  int
	}{
		{"", http.StatusOK},
		{"?warehouseId=all", http.StatusOK},
		{"?warehouseId=", http.StatusBadRequest},
		{"?warehouseId=todos", http.StatusBadRequest},
	} {
		w := do(t, r, http.MethodGet, "/api/v1/stock/shopping-list"+c.query, tok, nil, "")
		if w.Code != c.want {
			t.Errorf("lista de compra con %q = %d %s; quiere %d", c.query, w.Code, w.Body.String(), c.want)
			continue
		}
		if c.query != "?warehouseId=all" {
			continue
		}
		var body struct {
			Items []app.ShoppingLine `json:"items"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if _, ok := linesOf(body.Items, k.arizona)[k.bodega]; !ok {
			t.Fatalf("?warehouseId=all no trae el «se acabó» de Bodega: %s", w.Body.String())
		}
	}
}

// «Enviarme por correo» mandaba solo la lista del de venta: el «se acabó» de Bodega no llegaba. Con
// dos o más almacenes, cada renglón dice de cuál.
func TestEmailMeTheShoppingListOfAllWarehouses(t *testing.T) {
	t.Parallel()
	k := newShoppingAllKit(t)
	setEmail(t, k.st, k.user, "compra-todos@ejemplo.mx")
	box := &mailbox{}
	m := app.NewNotificationMailer(appRoleStore(t), func() time.Time { return time.Date(2026, 11, 23, 15, 0, 0, 0, time.UTC) }, box.send, "https://app.ejemplo.mx")
	if _, err := m.SendShoppingListTo(context.Background(), defaultCompanyID, k.user); err != nil {
		t.Fatal(err)
	}
	if box.count() != 1 {
		t.Fatalf("salieron %d correos", box.count())
	}
	html := box.sent[0].html
	if !strings.Contains(html, "Arizona todos") {
		t.Fatal("el correo de la lista no trae el «se acabó» de Bodega")
	}
	if l := itemLine(t, html, "Arizona todos"); !strings.Contains(l, "en Bodega") {
		t.Fatalf("el renglón de Arizona no dice de qué almacén: %s", l)
	}
}

// Con un solo almacén el correo queda como antes: sin nombrarlo.
func TestEmailMeTheShoppingListWithOneWarehouseDoesNotNameIt(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	user := makeUser(t, st, "compra_uno", "gerente")
	setEmail(t, st, user, "compra-uno@ejemplo.mx")
	ing := pieceIngredient(t, st, "Tapas uno")
	alertOn(t, st, ing, "10", true, false)
	adjust(t, st, "ingrediente", ing, "2")
	box := &mailbox{}
	m := app.NewNotificationMailer(appRoleStore(t), func() time.Time { return time.Date(2026, 11, 23, 15, 0, 0, 0, time.UTC) }, box.send, "https://app.ejemplo.mx")
	if _, err := m.SendShoppingListTo(context.Background(), defaultCompanyID, user); err != nil {
		t.Fatal(err)
	}
	if l := itemLine(t, box.sent[0].html, "Tapas uno"); strings.Contains(l, " en Local") {
		t.Fatalf("con un solo almacén el correo lo nombra: %s", l)
	}
}

// «Todos» desde otra empresa, con la conexión reciclada o sin empresa no trae nada de la dueña.
func TestShoppingListAllIsIsolatedInTheThreeCases(t *testing.T) {
	t.Parallel()
	k := newShoppingAllKit(t)
	other := makeCompany(t, k.st, "compra-todos-ajena")
	inTheThreeCases(t, defaultCompanyID, other, func(t *testing.T, as *store.Store, ctx context.Context) {
		lines, err := app.NewNotificationsService(as, clock).ShoppingList(ctx, domain.WarehouseSel{All: true})
		if err != nil {
			return
		}
		for _, l := range lines {
			if l.WarehouseID == k.local || l.WarehouseID == k.bodega {
				t.Fatalf("«Todos» de otra sesión trae un renglón del almacén %d de la dueña (%s)", l.WarehouseID, l.Name)
			}
		}
	})
}
