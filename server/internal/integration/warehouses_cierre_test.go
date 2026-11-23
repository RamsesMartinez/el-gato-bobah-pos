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

// El almacén de otra sucursal de la misma empresa: RLS no lo separa (es de la misma empresa), lo
// separa el filtro de sucursal. Buscar «se acabó» con su id no alcanza nada y responde 404.
func TestStockOutSearchWithAWarehouseOfAnotherBranch(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	co := makeCompany(t, st, "busca-otra-sucursal")
	var norte int64
	if err := st.Pool.QueryRow(context.Background(), `with b as (insert into branches (company_id, branch_number, code, name)
		values ($1, 2, 'NORTE', 'Norte') returning *)
		insert into warehouses (company_id, branch_id, name, is_sales) select company_id, id, 'Local Norte', true from b returning id`,
		co).Scan(&norte); err != nil {
		t.Fatal(err)
	}
	var norteBranch int64
	if err := st.Pool.QueryRow(context.Background(), `select branch_id from warehouses where id = $1`, norte).Scan(&norteBranch); err != nil {
		t.Fatal(err)
	}
	// La sesión trabaja en una sucursal: `app.branch_id` es la puerta del selector de sucursal. Con
	// una sola conexión en el pool, el ajuste de sesión sigue puesto cuando AcquireTenant la toma.
	searchFrom := func(branch int64) error {
		t.Helper()
		as := appRoleStoreWithOneConn(t)
		if _, err := as.Pool.Exec(context.Background(), "select set_config('app.branch_id', $1, false)", itoa(int(branch))); err != nil {
			t.Fatal(err)
		}
		ctx, release, err := as.AcquireTenant(context.Background(), co)
		if err != nil {
			t.Fatal(err)
		}
		defer release()
		_, err = app.NewStockOutService(as, clock).Search(ctx, "Leche", domain.WarehouseSel{ID: norte})
		return err
	}
	if err := searchFrom(norteBranch); err != nil {
		t.Fatalf("desde Norte, buscar en su propio almacén = %v; la prueba no está eligiendo sucursal", err)
	}
	if err := searchFrom(headquarters(t, st, co)); !errors.Is(err, domain.ErrWarehouseNotFound) {
		t.Fatalf("desde la matriz, buscar en el almacén de Norte = %v; quiere 404 por el almacén", err)
	}
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

// whyIn es el porqué de compra de un artículo en la lista de un almacén.
func whyIn(t *testing.T, warehouse, ing int64) string {
	t.Helper()
	ns, _, ctx := asNotifications(t, defaultCompanyID)
	lines, err := ns.ShoppingList(ctx, domain.WarehouseSel{ID: warehouse})
	if err != nil {
		t.Fatal(err)
	}
	for _, l := range lines {
		if l.ItemType == "ingrediente" && l.ID == ing {
			return l.Why
		}
	}
	t.Fatalf("el artículo %d no está en la lista de compra del almacén %d", ing, warehouse)
	return ""
}

// La sugerencia de compra de un aviso de Bodega salía de la lista del almacén de venta: con leche
// que se vende diario en Local y una Bodega que nunca vende, el correo de Bodega decía cuánto comprar
// según el consumo de Local.
func TestTheEmailSuggestionComesFromTheWarehouseOfTheNotice(t *testing.T) {
	t.Parallel()
	k := newMailerKit(t)
	ctx := context.Background()
	local, bodega := comoEnLaSegundaSubida(t, k.st, defaultCompanyID)
	ing := pieceIngredient(t, k.st, "Leche sugerida")
	alertOn(t, k.st, ing, "100", false, true) // en la lista de Local por mínimo, sin aviso que estorbe
	adjustIn(t, k.st, local, "ingrediente", ing, "90")
	for d := 1; d <= 8; d++ { // vende 10 diarios en Local: consumo medido
		if _, err := k.st.Pool.Exec(ctx, `insert into stock_movements (warehouse_id, item_type, ingredient_id, movement_type, quantity, created_at)
			values ($1, 'ingrediente', $2, 'venta', -10, now() - make_interval(days => $3))`, local, ing, d); err != nil {
			t.Fatal(err)
		}
	}
	so, sctx := asStockOut(t, defaultCompanyID)
	if _, _, err := so.ReportOut(sctx, app.ReportOutCmd{ItemType: "ingrediente", ID: ing, ClientKey: uuid.New(), Actor: k.user,
		Warehouse: domain.WarehouseSel{ID: bodega}}); err != nil {
		t.Fatal(err)
	}
	if _, err := k.st.Pool.Exec(ctx, `update notifications set opened_at = $2 where ingredient_id = $1 and status = 'open'`, ing, k.block.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	inLocal, inBodega := whyIn(t, local, ing), whyIn(t, bodega, ing)
	if inLocal == inBodega {
		t.Fatalf("la prueba necesita porqués distintos por almacén; los dos dicen %q", inLocal)
	}
	if err := k.mailer(k.block.Add(6*time.Minute)).SendWindows(ctx, defaultCompanyID); err != nil {
		t.Fatal(err)
	}
	if k.box.count() != 1 {
		t.Fatalf("salieron %d correos; quiere 1", k.box.count())
	}
	card := itemLine(t, k.box.sent[0].html, "Leche sugerida")
	if strings.Contains(card, inLocal) || !strings.Contains(card, inBodega) {
		t.Fatalf("el aviso de Bodega trae la sugerencia de Local (%q); quiere la de Bodega (%q): %s", inLocal, inBodega, card)
	}
}

// El enfriamiento de 12 h era por artículo: un correo de leche agotada en Local callaba la leche
// agotada en Bodega dos horas después, y quien compra no se enteraba de que faltaba en los dos.
func TestTheCooldownIsPerWarehouse(t *testing.T) {
	t.Parallel()
	k := newMailerKit(t)
	ctx := context.Background()
	_, bodega := comoEnLaSegundaSubida(t, k.st, defaultCompanyID)
	ing, _ := k.openAt(t, "Popotes enfriamiento", k.block.Add(time.Minute))
	if err := k.mailer(k.block.Add(6*time.Minute)).SendWindows(ctx, defaultCompanyID); err != nil {
		t.Fatal(err)
	}
	if k.box.count() != 1 {
		t.Fatalf("primer correo: %d", k.box.count())
	}
	if _, err := k.st.Pool.Exec(ctx, `update notification_email_sends set sent_at = $1`, k.block.Add(6*time.Minute)); err != nil {
		t.Fatal(err)
	}
	so, sctx := asStockOut(t, defaultCompanyID)
	if _, _, err := so.ReportOut(sctx, app.ReportOutCmd{ItemType: "ingrediente", ID: ing, ClientKey: uuid.New(), Actor: k.user,
		Warehouse: domain.WarehouseSel{ID: bodega}}); err != nil {
		t.Fatal(err)
	}
	later := k.block.Add(2 * time.Hour)
	if _, err := k.st.Pool.Exec(ctx, `update notifications set opened_at = $3 where ingredient_id = $1 and warehouse_id = $2 and status = 'open'`,
		ing, bodega, later.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := k.mailer(later.Add(6*time.Minute)).SendWindows(ctx, defaultCompanyID); err != nil {
		t.Fatal(err)
	}
	if k.box.count() != 2 {
		t.Fatalf("el correo de Local calló el aviso de Bodega: %d correos; quiere 2", k.box.count())
	}
	if l := itemLine(t, k.box.sent[1].html, "Popotes enfriamiento"); !strings.Contains(l, "En Bodega") {
		t.Fatalf("el segundo correo no es el de Bodega: %s", l)
	}
}

// En «Todos», el valor total redondeaba la existencia ya sumada de los almacenes, y cada almacén se
// redondeaba por su lado: 333.3333 ml a $0.025 en dos almacenes daba 8.33 + 8.33 = 16.66 en el
// desglose y 16.67 en el total. La pantalla mostraba un total que no era la suma de sus partes.
func TestStockValueOfAllIsTheSumOfTheWarehouses(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	owner := makeCompany(t, st, "valor-todos-duena")
	other := makeCompany(t, st, "valor-todos-ajena")
	local, bodega := comoEnLaSegundaSubida(t, st, owner)
	leche := ingredientIn(t, st, owner, "Leche valor todos", "ml")
	if _, err := st.Pool.Exec(context.Background(), `update ingredients set current_cost = 0.025 where id = $1`, leche); err != nil {
		t.Fatal(err)
	}
	// adjustIn no lleva empresa y la columna toma la de omisión: aquí va explícita.
	for _, wh := range []int64{local, bodega} {
		if _, err := st.Pool.Exec(context.Background(), `insert into stock_movements (company_id, warehouse_id, item_type, ingredient_id, movement_type, quantity)
			values ($1, $2, 'ingrediente', $3, 'ajuste', 333.3333)`, owner, wh, leche); err != nil {
			t.Fatal(err)
		}
	}

	svc, ctx := asTenant(t, owner)
	items, err := svc.StockLevels(ctx, domain.WarehouseSel{All: true})
	if err != nil {
		t.Fatal(err)
	}
	total := app.StockValueTotal(items)
	byWarehouse, err := svc.StockValueByWarehouse(ctx)
	if err != nil {
		t.Fatal(err)
	}
	sumWarehouses := decimal.Zero
	for _, v := range byWarehouse {
		sumWarehouses = sumWarehouses.Add(v.Value)
	}
	sumRows := decimal.Zero
	for _, it := range items {
		sumRows = sumRows.Add(it.Value)
	}
	if !sumWarehouses.Equal(total) {
		t.Fatalf("la suma del valor por almacén es %s y el total de «Todos» es %s; el total no cuadra con su desglose",
			sumWarehouses.StringFixed(2), total.StringFixed(2))
	}
	if !sumRows.Equal(total) {
		t.Fatalf("la suma de los renglones es %s y el total es %s", sumRows.StringFixed(2), total.StringFixed(2))
	}
	if total.StringFixed(2) != "16.66" {
		t.Fatalf("el total es %s; quiere 16.66 (8.33 por almacén)", total.StringFixed(2))
	}

	inTheThreeCases(t, owner, other, func(t *testing.T, as *store.Store, ctx context.Context) {
		svc := app.NewBackofficeService(as, nil)
		if items, err := svc.StockLevels(ctx, domain.WarehouseSel{All: true}); err == nil {
			for _, it := range items {
				if it.IngredientID != nil && *it.IngredientID == leche {
					t.Fatal("«Todos» de otra sesión trae la leche de la dueña")
				}
			}
		}
		if vals, err := svc.StockValueByWarehouse(ctx); err == nil {
			for _, v := range vals {
				if v.WarehouseID == local || v.WarehouseID == bodega {
					t.Fatalf("el valor por almacén de otra sesión trae el almacén %d de la dueña", v.WarehouseID)
				}
			}
		}
	})
}

// El historial de merma (Almacén › Se echó a perder) no decía de qué almacén salió cada registro:
// con Local y Bodega, «Leche · 4 ml» no decía de dónde se restó. Las listas de hoy y por aprobar ya
// lo traían; la del periodo, no.
func TestWasteHistoryNamesTheWarehouse(t *testing.T) {
	t.Parallel()
	k := newWhCountKit(t, "merma_historial")
	caduco := reasonNamed(t, k.st, defaultCompanyID, "Caducó")
	res, err := k.svc.RecordWaste(k.ctx, app.RecordWasteCmd{ReasonID: caduco, IngredientID: &k.leche, Qty: decimal.NewFromInt(4),
		Actor: k.user, Warehouse: domain.WarehouseSel{ID: k.bodega}})
	if err != nil {
		t.Fatal(err)
	}
	from, to := todayRange(t, k.st)
	rows, err := k.svc.ListWaste(k.ctx, from, to, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rows {
		if r.ID == res.ID {
			if r.WarehouseName != "Bodega" {
				t.Fatalf("el historial dice almacén %q; quiere «Bodega»", r.WarehouseName)
			}
			return
		}
	}
	t.Fatal("el historial no trae la merma de Bodega")
}

// Encender las alertas del artículo desde la hoja de Bodega: la confirmación decía «no se envía nada»
// (la vista previa salía del de venta, o de Bodega con las alertas apagadas de antes) y al guardar se
// abría el aviso de Bodega, con 800 ml contra su mínimo de 2 L. La vista previa de Bodega se calcula
// con los interruptores propuestos y anuncia lo mismo que después pasa.
func TestPreviewInBodegaWithTheProposedSwitchesAnnouncesItsNotice(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	local, bodega := comoEnLaSegundaSubida(t, st, defaultCompanyID)
	aceite := ingredientIn(t, st, defaultCompanyID, "Aceite vista previa bodega", "ml")
	adjustIn(t, st, local, "ingrediente", aceite, "5000")
	adjustIn(t, st, bodega, "ingrediente", aceite, "800")
	ns, _, ctx := asNotifications(t, defaultCompanyID)
	actor := makeUser(t, st, "alertas_bodega_"+itoa(int(bodega)), "gerente")
	dos := decimal.NewFromInt(2000)
	if _, err := ns.ConfigureLowStock(ctx, "ingrediente", aceite, app.LowStockInput{
		BranchMinStock: app.Field[decimal.Decimal]{Set: true, Value: &dos}, Warehouse: domain.WarehouseSel{ID: bodega}}, actor, false); err != nil {
		t.Fatal(err)
	}
	prev, err := ns.ConfigureLowStock(ctx, "ingrediente", aceite, app.LowStockInput{Enabled: true, OnMin: true, OnOut: true,
		BranchMinStock: app.Field[decimal.Decimal]{Set: true, Value: &dos}, Warehouse: domain.WarehouseSel{ID: bodega}}, actor, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(prev.Effect.Opens) != 1 || prev.Effect.Opens[0] != domain.NotificationKind("stock_low") || !prev.OnHand.Equal(decimal.NewFromInt(800)) {
		t.Fatalf("la vista previa de Bodega = %+v con %s; quiere que abra el aviso de stock bajo con 800 ml", prev.Effect, prev.OnHand)
	}
	// La vista previa no guarda: las alertas del artículo siguen apagadas.
	var enabled bool
	if err := st.Pool.QueryRow(context.Background(), `select low_stock_enabled from ingredients where id = $1`, aceite).Scan(&enabled); err != nil || enabled {
		t.Fatalf("la vista previa encendió las alertas (%v, %v)", enabled, err)
	}
	// Y lo anunciado es lo que pasa: encender el artículo (sin almacén) abre el aviso de Bodega.
	if _, err := ns.ConfigureLowStock(ctx, "ingrediente", aceite, app.LowStockInput{Enabled: true, OnMin: true, OnOut: true}, actor, false); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := st.Pool.QueryRow(context.Background(), `select count(*) from notifications where ingredient_id = $1 and warehouse_id = $2
		and kind = 'stock_low' and status = 'open'`, aceite, bodega).Scan(&n); err != nil || n != 1 {
		t.Fatalf("avisos abiertos de Bodega = %d, %v; quiere 1, el que anunció la vista previa", n, err)
	}
}

// Guardar (no la vista previa) desde Bodega sigue ignorando los interruptores: son del artículo.
func TestSavingFromBodegaStillIgnoresTheSwitches(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	_, bodega := comoEnLaSegundaSubida(t, st, defaultCompanyID)
	aceite := ingredientIn(t, st, defaultCompanyID, "Aceite guarda bodega", "ml")
	ns, _, ctx := asNotifications(t, defaultCompanyID)
	actor := makeUser(t, st, "alertas_bodega_"+itoa(int(bodega)), "gerente")
	dos := decimal.NewFromInt(2000)
	if _, err := ns.ConfigureLowStock(ctx, "ingrediente", aceite, app.LowStockInput{Enabled: true, OnMin: true,
		BranchMinStock: app.Field[decimal.Decimal]{Set: true, Value: &dos}, Warehouse: domain.WarehouseSel{ID: bodega}}, actor, false); err != nil {
		t.Fatal(err)
	}
	var enabled bool
	if err := st.Pool.QueryRow(context.Background(), `select low_stock_enabled from ingredients where id = $1`, aceite).Scan(&enabled); err != nil || enabled {
		t.Fatalf("guardar desde Bodega encendió las alertas del artículo (%v, %v)", enabled, err)
	}
}
