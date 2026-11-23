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

// shoppingRouter arma la API con la sesión de un usuario de la empresa dada.
func shoppingRouter(t *testing.T, st *store.Store, user, company int64) (http.Handler, string) {
	t.Helper()
	jm := auth.NewManager("secreto-de-pruebas-suficientemente-largo-para-el-manager", nil)
	h := httpapi.NewHandlers(httpapi.Deps{JWT: jm, Admin: app.NewAdminService(st), Backoffice: app.NewBackofficeService(st, nil),
		Notifications: app.NewNotificationsService(st, clock), MenuCache: cache.NewMenuCache(""), Broker: realtime.NewBroker()})
	tok, err := jm.Issue(domain.User{ID: user, CompanyID: company, Name: "gerente", Role: "gerente"})
	if err != nil {
		t.Fatal(err)
	}
	return httpapi.Router(config.Config{}, jm, h, st), tok
}

// Revisión de código: una lista vacía tiene que salir como arreglo. Un `null` tumba la pantalla con la
// API en 200 y sin una línea en el log (AGENTS.md); deserializar a Go borra esa diferencia, por eso se
// lee el JSON crudo.
func TestShoppingListAllWithNothingIsAnEmptyArray(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	co := makeCompany(t, st, "compra-vacia")
	comoEnLaSegundaSubida(t, st, co)
	user := makeUserIn(t, st, co, "compra_vacia", "gerente")
	r, tok := shoppingRouter(t, st, user, co)
	for _, q := range []string{"", "?warehouseId=all"} {
		w := do(t, r, http.MethodGet, "/api/v1/stock/shopping-list"+q, tok, nil, "")
		if w.Code != http.StatusOK {
			t.Fatalf("lista vacía con %q = %d %s", q, w.Code, w.Body.String())
		}
		var raw map[string]json.RawMessage
		if err := json.Unmarshal(w.Body.Bytes(), &raw); err != nil {
			t.Fatal(err)
		}
		if got := strings.TrimSpace(string(raw["items"])); got != "[]" {
			t.Fatalf("la lista vacía con %q trae \"items\":%s; la pantalla espera [] y con null no se pinta", q, got)
		}
	}
}

// Con un solo almacén, «Todos» es esa misma lista y en el mismo orden: quien no tiene Bodega no ve
// nada distinto por pedir «Todos». Los nombres con acento y minúscula son los que un orden por bytes
// pone en otro lugar que la base.
func TestShoppingListAllWithOneWarehouseIsTheSalesList(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	user := makeUser(t, st, "compra_un_almacen", "gerente")
	for _, name := range []string{"Bicarbonato uno", "Ácido uno", "agua uno"} {
		ing := pieceIngredient(t, st, name)
		alertOn(t, st, ing, "100", true, false)
		adjust(t, st, "ingrediente", ing, "1")
	}
	r, tok := shoppingRouter(t, st, user, defaultCompanyID)
	sales := do(t, r, http.MethodGet, "/api/v1/stock/shopping-list", tok, nil, "")
	all := do(t, r, http.MethodGet, "/api/v1/stock/shopping-list?warehouseId=all", tok, nil, "")
	if sales.Code != http.StatusOK || all.Code != http.StatusOK {
		t.Fatalf("lista = %d, todos = %d", sales.Code, all.Code)
	}
	if sales.Body.String() != all.Body.String() {
		t.Fatalf("con un solo almacén «Todos» no es la lista del de venta:\nsin parámetro: %s\ntodos:         %s", sales.Body.String(), all.Body.String())
	}
}

// ?warehouseId=<Bodega> lee solo el mínimo propio de Bodega: los popotes (mínimo 50 en Bodega) salen
// con ese mínimo, y la leche (mínimo 100 del artículo, que es del de venta) no sale.
func TestShoppingListOfBodegaUsesOnlyItsOwnMinimum(t *testing.T) {
	t.Parallel()
	k := newShoppingAllKit(t)
	ns, _, ctx := asNotifications(t, defaultCompanyID)
	lines, err := ns.ShoppingList(ctx, domain.WarehouseSel{ID: k.bodega})
	if err != nil {
		t.Fatal(err)
	}
	for _, l := range lines {
		if l.WarehouseID != k.bodega {
			t.Fatalf("la lista de Bodega trae un renglón del almacén %d (%s)", l.WarehouseID, l.Name)
		}
	}
	if l, ok := linesOf(lines, k.popotes)[k.bodega]; !ok || l.Min == nil || l.Min.String() != "50" {
		t.Fatalf("los popotes no salen en Bodega con su mínimo propio de 50: %+v", linesOf(lines, k.popotes))
	}
	if got := linesOf(lines, k.leche); len(got) != 0 {
		t.Fatalf("la leche sale en la lista de Bodega con %+v: aplicó el mínimo del artículo, que es del almacén de venta", got)
	}
}

// Un almacén archivado no se surte: su «se acabó» viejo no puede seguir pidiendo compra en «Todos».
func TestShoppingListAllLeavesOutAnArchivedWarehouse(t *testing.T) {
	t.Parallel()
	k := newShoppingAllKit(t)
	if _, err := k.st.Pool.Exec(context.Background(), `update warehouses set is_active = false where id = $1`, k.bodega); err != nil {
		t.Fatal(err)
	}
	ns, _, ctx := asNotifications(t, defaultCompanyID)
	lines, err := ns.ShoppingList(ctx, domain.WarehouseSel{All: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, l := range lines {
		if l.WarehouseID == k.bodega {
			t.Fatalf("«Todos» trae %s de Bodega, que está archivada", l.Name)
		}
	}
}

// Solo «all» tal cual: una variante en mayúsculas o con espacio es un parámetro malformado, no «Todos»
// ni el de venta en silencio.
func TestShoppingListRejectsVariantsOfAll(t *testing.T) {
	t.Parallel()
	k := newShoppingAllKit(t)
	r, tok := shoppingRouter(t, k.st, k.user, defaultCompanyID)
	for _, q := range []string{"?warehouseId=ALL", "?warehouseId=%20all", "?warehouseId=all%20"} {
		if w := do(t, r, http.MethodGet, "/api/v1/stock/shopping-list"+q, tok, nil, ""); w.Code != http.StatusBadRequest {
			t.Errorf("lista de compra con %q = %d %s; quiere 400", q, w.Code, w.Body.String())
		}
	}
}

// Revisión de código: «Todos» reordenaba con los bytes de Go y no con la collation de la base, así que
// «agua» y «Ácido» salían en otro lugar que en la lista de su almacén. Ahora es la lista de cada almacén
// tal como la da la consulta, el de venta primero. No se fija el orden esperado de los tres nombres: lo
// decide la collation de la base, que no es la misma en todos los Postgres.
func TestShoppingListAllKeepsTheOrderOfEachWarehouse(t *testing.T) {
	t.Parallel()
	k := newShoppingAllKit(t)
	for _, name := range []string{"Bicarbonato orden", "Ácido orden", "agua orden"} {
		ing := pieceIngredient(t, k.st, name)
		alertOn(t, k.st, ing, "100", true, false)
		adjustIn(t, k.st, k.local, "ingrediente", ing, "1")
	}
	ns, _, ctx := asNotifications(t, defaultCompanyID)
	all, err := ns.ShoppingList(ctx, domain.WarehouseSel{All: true})
	if err != nil {
		t.Fatal(err)
	}
	local, err := ns.ShoppingList(ctx, domain.WarehouseSel{ID: k.local})
	if err != nil {
		t.Fatal(err)
	}
	bodega, err := ns.ShoppingList(ctx, domain.WarehouseSel{ID: k.bodega})
	if err != nil {
		t.Fatal(err)
	}
	key := func(ls []app.ShoppingLine) []string {
		out := []string{}
		for _, l := range ls {
			out = append(out, l.WarehouseName+"/"+l.Name)
		}
		return out
	}
	want := append(key(local), key(bodega)...)
	if got := key(all); strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("«Todos» no es la lista de Local y luego la de Bodega, cada una en su orden:\n got  %v\n want %v", got, want)
	}
}
