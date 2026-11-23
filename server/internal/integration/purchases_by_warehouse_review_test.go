//go:build integration

package integration

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/auth"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/cache"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/config"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/httpapi"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
)

// LO QUE ENCONTRÓ LA REVISIÓN DE LA SEGUNDA SUBIDA DE LA 094 (compras con destino). Cada prueba
// nombra el defecto que atrapa.

// Cambiar el almacén de un renglón no es una entrega: el traslado no puede mover el día en que la
// compra quedó recibida ni aparecer en su historial como un día de entrega.
func TestChangingTheWarehouseDoesNotMoveTheReceivedDay(t *testing.T) {
	t.Parallel()
	k := newPurchaseKit(t)
	_, bodega := comoEnLaSegundaSubida(t, k.st, defaultCompanyID)
	id, err := k.svc.CreateExpense(k.ctx, k.bultos("2", "1000"))
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	// La compra se recibió hace un mes.
	if _, err := k.st.Pool.Exec(ctx, `update stock_movements set effective_on = date '2026-06-01' where expense_id = $1`, id); err != nil {
		t.Fatal(err)
	}
	if _, err := k.st.Pool.Exec(ctx, `update expenses set received_at = date '2026-06-01' where id = $1`, id); err != nil {
		t.Fatal(err)
	}
	item, applied := k.itemOf(t, id)
	line := app.StockLineInput{ID: &item, SeenApplied: &applied, ItemType: "ingrediente", IngredientID: &k.azucar,
		Quantity: dd("2"), Received: dd("2"), PresentationID: &k.bulto, Amount: dd("1000"), Description: "Azúcar",
		WarehouseID: &bodega}
	if _, err := k.svc.SaveStockLines(k.ctx, id, []app.StockLineInput{line}, k.user); err != nil {
		t.Fatal(err)
	}
	var received time.Time
	if err := k.st.Pool.QueryRow(ctx, `select received_at from expenses where id = $1`, id).Scan(&received); err != nil {
		t.Fatal(err)
	}
	if received.Format("2006-01-02") != "2026-06-01" {
		t.Fatalf("la compra recibida el 2026-06-01 quedó recibida el %s por cambiarle el almacén", received.Format("2006-01-02"))
	}
	var days int
	if err := k.st.Pool.QueryRow(ctx, `select count(distinct effective_on) from stock_movements
		where expense_id = $1 and movement_type = 'compra' and not warehouse_relocation`, id).Scan(&days); err != nil || days != 1 {
		t.Fatalf("el historial de entregas tiene %d días, %v; quiere el único día en que llegó", days, err)
	}
}

// Anular una compra cuyo destino se cambió deja los dos almacenes en cero neto.
func TestCancellingAfterChangingTheWarehouseLeavesBothAtZero(t *testing.T) {
	t.Parallel()
	k := newPurchaseKit(t)
	local, bodega := comoEnLaSegundaSubida(t, k.st, defaultCompanyID)
	id, err := k.svc.CreateExpense(k.ctx, k.bultos("2", "1000"))
	if err != nil {
		t.Fatal(err)
	}
	item, applied := k.itemOf(t, id)
	line := app.StockLineInput{ID: &item, SeenApplied: &applied, ItemType: "ingrediente", IngredientID: &k.azucar,
		Quantity: dd("2"), Received: dd("2"), PresentationID: &k.bulto, Amount: dd("1000"), Description: "Azúcar",
		WarehouseID: &bodega}
	if _, err := k.svc.SaveStockLines(k.ctx, id, []app.StockLineInput{line}, k.user); err != nil {
		t.Fatal(err)
	}
	if err := k.svc.CancelExpense(k.ctx, id, "prueba", k.user, true); err != nil {
		t.Fatalf("anular: %v", err)
	}
	if v := onHandIn(t, k.st, local, "ingredient_id", k.azucar); !v.IsZero() {
		t.Errorf("el local quedó con %s después de anular", v)
	}
	if v := onHandIn(t, k.st, bodega, "ingredient_id", k.azucar); !v.IsZero() {
		t.Errorf("la bodega quedó con %s después de anular", v)
	}
}

// Corregir la cantidad de un renglón que ya se trasladó a la bodega corrige la bodega.
func TestCorrectingAfterMovingCorrectsTheNewWarehouse(t *testing.T) {
	t.Parallel()
	k := newPurchaseKit(t)
	local, bodega := comoEnLaSegundaSubida(t, k.st, defaultCompanyID)
	id, err := k.svc.CreateExpense(k.ctx, k.bultos("2", "1000"))
	if err != nil {
		t.Fatal(err)
	}
	item, applied := k.itemOf(t, id)
	line := app.StockLineInput{ID: &item, SeenApplied: &applied, ItemType: "ingrediente", IngredientID: &k.azucar,
		Quantity: dd("2"), Received: dd("2"), PresentationID: &k.bulto, Amount: dd("1000"), Description: "Azúcar",
		WarehouseID: &bodega}
	if _, err := k.svc.SaveStockLines(k.ctx, id, []app.StockLineInput{line}, k.user); err != nil {
		t.Fatal(err)
	}
	_, applied = k.itemOf(t, id)
	line.SeenApplied, line.Received, line.WarehouseID = &applied, dd("1"), nil
	if _, err := k.svc.SaveStockLines(k.ctx, id, []app.StockLineInput{line}, k.user); err != nil {
		t.Fatal(err)
	}
	if v := onHandIn(t, k.st, bodega, "ingredient_id", k.azucar); !v.Equal(dd("25000")) {
		t.Errorf("la bodega quedó con %s; quiere 25,000 g (un bulto)", v)
	}
	if v := onHandIn(t, k.st, local, "ingredient_id", k.azucar); !v.IsZero() {
		t.Errorf("el local quedó con %s: un renglón ausente de almacén debe conservar el suyo", v)
	}
}

// Un renglón por recibir al que se le cambia el destino no mueve nada: no ha llegado.
func TestChangingTheWarehouseOfAPendingLineMovesNothing(t *testing.T) {
	t.Parallel()
	k := newPurchaseKit(t)
	_, bodega := comoEnLaSegundaSubida(t, k.st, defaultCompanyID)
	in := k.bultos("2", "1000")
	zero := dd("0")
	in.Items[0].QtyReceived = &zero
	id, err := k.svc.CreateExpense(k.ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	item, applied := k.itemOf(t, id)
	line := app.StockLineInput{ID: &item, SeenApplied: &applied, ItemType: "ingrediente", IngredientID: &k.azucar,
		Quantity: dd("2"), Received: dd("0"), PresentationID: &k.bulto, Amount: dd("1000"), Description: "Azúcar",
		WarehouseID: &bodega}
	if _, err := k.svc.SaveStockLines(k.ctx, id, []app.StockLineInput{line}, k.user); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := k.st.Pool.QueryRow(context.Background(), `select count(*) from stock_movements where expense_id = $1`, id).Scan(&n); err != nil || n != 0 {
		t.Fatalf("un renglón que no ha llegado dejó %d movimientos al cambiarle el destino, %v", n, err)
	}
}

// Un almacén con compras por recibir no se archiva: lo que llegue después entraría a un almacén que
// ya no sale en ninguna lista (revisión de seguridad). Y recibir en uno archivado se rechaza.
func TestAWarehouseWithPendingPurchasesIsNotArchived(t *testing.T) {
	t.Parallel()
	k := newPurchaseKit(t)
	_, bodega := comoEnLaSegundaSubida(t, k.st, defaultCompanyID)
	in := k.bultos("2", "1000")
	zero := dd("0")
	in.Items[0].QtyReceived = &zero
	in.Items[0].WarehouseID = &bodega
	id, err := k.svc.CreateExpense(k.ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	if err := k.svc.ArchiveWarehouse(k.ctx, bodega); !errors.Is(err, domain.ErrWarehousePendingPurchases) {
		t.Fatalf("archivar con una compra por recibir = %v; quiere ErrWarehousePendingPurchases", err)
	}
	// Archivada por fuera (una base vieja): recibir ahí se rechaza en vez de esconder la mercancía.
	if _, err := k.st.Pool.Exec(context.Background(), `update warehouses set is_active = false where id = $1`, bodega); err != nil {
		t.Fatal(err)
	}
	item, applied := k.itemOf(t, id)
	err = k.svc.ReceiveItem(k.ctx, id, item, app.ReceiveItemInput{Qty: dd("2"), SeenApplied: applied}, k.user)
	if !errors.Is(err, domain.ErrWarehouseNotFound) {
		t.Fatalf("recibir en un almacén archivado = %v; quiere «no existe»", err)
	}
}

// Recibir en otro almacén cuando ya llegó algo al del renglón trasladaría también lo que ya llegó:
// se rechaza y se pide cambiarlo desde corregir.
func TestReceivingElsewhereAfterSomethingArrivedIsRejected(t *testing.T) {
	t.Parallel()
	k := newPurchaseKit(t)
	local, bodega := comoEnLaSegundaSubida(t, k.st, defaultCompanyID)
	in := k.bultos("2", "1000")
	one := dd("1")
	in.Items[0].QtyReceived = &one
	id, err := k.svc.CreateExpense(k.ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	item, applied := k.itemOf(t, id)
	err = k.svc.ReceiveItem(k.ctx, id, item, app.ReceiveItemInput{Qty: dd("1"), SeenApplied: applied, WarehouseID: &bodega}, k.user)
	if !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("recibir en la bodega con un bulto ya en el local = %v; quiere conflicto", err)
	}
	if v := onHandIn(t, k.st, local, "ingredient_id", k.azucar); !v.Equal(dd("25000")) {
		t.Fatalf("el local quedó con %s; el bulto que ya había llegado no se mueve", v)
	}
}

// El almacén del renglón: 0 o negativo es 400; sin artículo se ignora; otra sucursal o archivado no
// existe.
func TestTheWarehouseOfALineIsValidated(t *testing.T) {
	t.Parallel()
	k := newPurchaseKit(t)
	_, bodega := comoEnLaSegundaSubida(t, k.st, defaultCompanyID)
	for _, bad := range []int64{0, -3} {
		in := k.bultos("1", "500")
		in.Items[0].WarehouseID = &bad
		if _, err := k.svc.CreateExpense(k.ctx, in); !errors.Is(err, domain.ErrValidation) {
			t.Errorf("almacén %d = %v; quiere 400", bad, err)
		}
	}
	gas := app.ExpenseInput{ReceivedAt: "2026-07-20", CategoryID: k.category, Amount: dd("100"), Status: domain.ExpensePendiente,
		Description: "Gas", UserID: k.user, Items: []app.ExpenseItemInput{{Description: "Gas", Quantity: dd("1"), Amount: dd("100"), WarehouseID: &bodega}}}
	if _, err := k.svc.CreateExpense(k.ctx, gas); err != nil {
		t.Fatalf("un renglón sin artículo con almacén: %v", err)
	}
	var norte int64
	if err := k.st.Pool.QueryRow(context.Background(), `with b as (insert into branches (company_id, branch_number, code, name, is_active)
		values ($1, 7, 'NORTE7', 'Norte', false) returning *)
		insert into warehouses (company_id, branch_id, name, is_sales) select company_id, id, 'Local Norte', true from b returning id`,
		defaultCompanyID).Scan(&norte); err != nil {
		t.Fatal(err)
	}
	in := k.bultos("1", "500")
	in.Items[0].WarehouseID = &norte
	if _, err := k.svc.CreateExpense(k.ctx, in); !errors.Is(err, domain.ErrWarehouseNotFound) {
		t.Fatalf("comprar al almacén de otra sucursal = %v; quiere «no existe»", err)
	}
	if _, err := k.st.Pool.Exec(context.Background(), `update warehouses set is_active = false where id = $1`, bodega); err != nil {
		t.Fatal(err)
	}
	in = k.bultos("1", "500")
	in.Items[0].WarehouseID = &bodega
	if _, err := k.svc.CreateExpense(k.ctx, in); !errors.Is(err, domain.ErrWarehouseNotFound) {
		t.Fatalf("comprar a un almacén archivado = %v; quiere «no existe»", err)
	}
}

// Las escrituras de compra con almacén, en los tres casos de RLS: nadie mete mercancía en la bodega de
// otra empresa.
func TestPurchaseWarehouseWritesAreIsolatedInTheThreeCases(t *testing.T) {
	t.Parallel()
	k := newPurchaseKit(t)
	_, bodega := comoEnLaSegundaSubida(t, k.st, defaultCompanyID)
	other := makeCompany(t, k.st, "compra-tres-casos")
	inTheThreeCases(t, defaultCompanyID, other, func(t *testing.T, as *store.Store, ctx context.Context) {
		svc := app.NewBackofficeService(as, clock)
		in := k.bultos("1", "500")
		in.Items[0].WarehouseID = &bodega
		if _, err := svc.CreateExpense(ctx, in); err == nil {
			t.Fatal("se compró a la bodega de otra empresa")
		}
		if err := svc.SetSalesWarehouse(ctx, bodega); err == nil {
			t.Fatal("se cambió el almacén de venta de otra empresa")
		}
	})
}

// El PATCH de un almacén aplica nombre y «de venta» en una sola transacción: si lo segundo falla, lo
// primero no queda.
func TestUpdatingAWarehouseIsAtomic(t *testing.T) {
	t.Parallel()
	k := newPurchaseKit(t)
	_, bodega := comoEnLaSegundaSubida(t, k.st, defaultCompanyID)
	if _, err := k.st.Pool.Exec(context.Background(), `update warehouses set is_active = false where id = $1`, bodega); err != nil {
		t.Fatal(err)
	}
	name := "Bodega renombrada"
	if err := k.svc.UpdateWarehouse(k.ctx, bodega, &name, true); err == nil {
		t.Fatal("se marcó como de venta un almacén archivado")
	}
	var got string
	if err := k.st.Pool.QueryRow(context.Background(), `select name from warehouses where id = $1`, bodega).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got == name {
		t.Fatal("el nombre cambió aunque marcarlo de venta falló")
	}
}

// El PATCH de un almacén: «isSales: false» y un cuerpo vacío son 400 (no un éxito que no hizo nada);
// un almacén de otra empresa es 404 igual que uno que no existe.
func TestPatchWarehouseHTTP(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	jm := auth.NewManager("secreto-de-pruebas-suficientemente-largo-para-el-manager", nil)
	h := httpapi.NewHandlers(httpapi.Deps{JWT: jm, Backoffice: app.NewBackofficeService(st, clock), MenuCache: cache.NewMenuCache("")})
	r := httpapi.Router(config.Config{}, jm, h, st)
	id := makeUser(t, st, "admin_patch_almacen", "admin")
	tok, err := jm.Issue(domain.User{ID: id, CompanyID: defaultCompanyID, Name: "admin", Role: domain.RoleAdmin})
	if err != nil {
		t.Fatal(err)
	}
	_, bodega := comoEnLaSegundaSubida(t, st, defaultCompanyID)
	other := makeCompany(t, st, "patch-almacen-ajeno")
	var foreign int64
	if err := st.Pool.QueryRow(context.Background(), `select id from warehouses where company_id = $1`, other).Scan(&foreign); err != nil {
		t.Fatal(err)
	}
	patch := func(wh int64, body string) int {
		req := httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/v1/stock/warehouses/%d", wh), strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+tok)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w.Code
	}
	for _, c := range []struct {
		name string
		wh   int64
		body string
		want int
	}{
		{"isSales false", bodega, `{"isSales":false}`, http.StatusBadRequest},
		{"cuerpo vacío", bodega, `{}`, http.StatusBadRequest},
		{"otra empresa", foreign, `{"isSales":true}`, http.StatusNotFound},
		{"de venta la bodega", bodega, `{"isSales":true}`, http.StatusNoContent},
	} {
		if got := patch(c.wh, c.body); got != c.want {
			t.Errorf("%s: %d; quiere %d", c.name, got, c.want)
		}
	}
}
