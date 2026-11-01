//go:build integration

package integration

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"testing"

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

// RENGLONES EXCLUIDOS (spec 049, historia 1). Un ticket trae cosas del negocio y cosas que no lo son;
// lo que no es se guarda como evidencia y no entra a existencias, costo ni a la suma contra el total.

// mixto arma un ticket de $1,000: un bulto de azúcar del negocio por $600 y leche de la casa,
// excluida, por $800. Sin excluir, los renglones pasarían del total.
func (k *purchaseKit) mixto(t *testing.T, leche int64, lecheRecibida *decimal.Decimal) app.ExpenseInput {
	t.Helper()
	in := k.bultos("1", "600")
	in.Amount = dd("1000")
	in.Items = append(in.Items, app.ExpenseItemInput{
		ItemType: "ingrediente", IngredientID: &leche, Description: "Leche de la casa", Quantity: dd("1000"),
		UnitID: ptrUnit(unitID(t, k.st, "ml")), QtyReceived: lecheRecibida, Amount: dd("800"), Excluded: true,
	})
	return in
}

func ptrUnit(u int16) *int16 { return &u }

func (k *purchaseKit) itemByDescription(t *testing.T, expense int64, desc string) (id int64, applied decimal.Decimal, excluded bool) {
	t.Helper()
	if err := k.st.Pool.QueryRow(context.Background(), `select id, applied_qty_base, excluded from expense_items
		where expense_id = $1 and description = $2 and voided_at is null`, expense, desc).Scan(&id, &applied, &excluded); err != nil {
		t.Fatalf("renglón %q: %v", desc, err)
	}
	return id, applied, excluded
}

func TestAnExcludedLineIsKeptButNeverEntersStock(t *testing.T) {
	t.Parallel()
	k := newPurchaseKit(t)
	leche := ingredientIn(t, k.st, defaultCompanyID, "Leche casa", "ml")
	// El front manda el excluido con su artículo y con lo recibido puesto: igual no entra.
	llego := dd("1000")
	id, err := k.svc.CreateExpense(k.ctx, k.mixto(t, leche, &llego))
	if err != nil {
		t.Fatalf("ticket mixto: %v (la regla de la 038 no debe contar el excluido)", err)
	}
	if got := k.onHand(t); !got.Equal(dd("25000")) {
		t.Errorf("azúcar = %s; quiere 25000", got)
	}
	if got := onHandIngredient(t, k.st, leche); !got.IsZero() {
		t.Errorf("la leche excluida sumó %s ml al almacén", got)
	}
	var lecheCost decimal.Decimal
	if err := k.st.Pool.QueryRow(context.Background(), `select current_cost from ingredients where id = $1`, leche).Scan(&lecheCost); err != nil {
		t.Fatal(err)
	}
	if !lecheCost.IsZero() {
		t.Errorf("el costo de la leche pasó a %s por un renglón excluido", lecheCost)
	}
	_, _, excluded := k.itemByDescription(t, id, "Leche de la casa")
	if !excluded {
		t.Fatal("el renglón de la casa no quedó guardado como excluido: se perdió la evidencia")
	}
	detail, err := k.svc.ExpenseDetail(k.ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if detail.ReceivedAt == nil {
		t.Error("la compra quedó pendiente por un renglón excluido")
	}
	var seen bool
	for _, it := range detail.Items {
		seen = seen || (it.Excluded && it.Description == "Leche de la casa" && it.Amount.Equal(dd("800")))
	}
	if !seen {
		t.Errorf("el detalle no trae el excluido con su importe: %+v", detail.Items)
	}
}

func TestAnExcludedLineIsNotPendingAndReceiveAllSkipsIt(t *testing.T) {
	t.Parallel()
	k := newPurchaseKit(t)
	leche := ingredientIn(t, k.st, defaultCompanyID, "Leche pendiente", "ml")
	in := k.mixto(t, leche, nil)
	in.ReceivedAt = "" // nada llegó todavía
	in.Items[0].QtyReceived = nil
	id, err := k.svc.CreateExpense(k.ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	pend, err := k.svc.PendingReceipts(k.ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range pend {
		if p.ItemName == "Leche pendiente" || p.ItemName == "Leche de la casa" {
			t.Fatalf("el excluido salió en «Por recibir»: %+v", p)
		}
	}
	var n int32
	if n, err = k.st.Q.PendingReceiptsOf(context.Background(), &leche); err != nil || n != 0 {
		t.Errorf("renglones pendientes de la leche = %d (%v); quiere 0", n, err)
	}
	if err := k.svc.ReceiveExpense(k.ctx, id, "", k.user); err != nil {
		t.Fatalf("recibir todo: %v", err)
	}
	if got := onHandIngredient(t, k.st, leche); !got.IsZero() {
		t.Errorf("«recibir todo» metió %s ml del excluido", got)
	}
	if got := k.onHand(t); !got.Equal(dd("25000")) {
		t.Errorf("azúcar tras recibir todo = %s; quiere 25000", got)
	}
}

// Excluir lo que ya entró lo saca con una corrección; incluirlo lo vuelve a meter. El historial de
// precios no cuenta la compra excluida.
func TestExcludingAndIncludingAfterSaving(t *testing.T) {
	t.Parallel()
	k := newPurchaseKit(t)
	id, err := k.svc.CreateExpense(k.ctx, k.bultos("2", "1000"))
	if err != nil {
		t.Fatal(err)
	}
	item, _ := k.itemOf(t, id)
	if _, err := k.svc.SetItemExcluded(k.ctx, id, item, true, k.user); err != nil {
		t.Fatalf("excluir: %v", err)
	}
	if got := k.onHand(t); !got.IsZero() {
		t.Fatalf("tras excluir quedan %s g; quiere 0", got)
	}
	var reason string
	if err := k.st.Pool.QueryRow(context.Background(), `select reason from stock_movements where expense_item_id = $1 and quantity < 0`, item).Scan(&reason); err != nil || !strings.HasPrefix(reason, "corrección de compra") {
		t.Errorf("la salida del excluido = %q (%v); quiere una «corrección de compra»", reason, err)
	}
	last, err := k.st.Q.OldestOfLastPurchases(context.Background(), &k.azucar)
	if err != nil {
		t.Fatal(err)
	}
	if last.Purchases != 0 {
		t.Errorf("el historial de precios cuenta %d compras; una excluida no es un precio", last.Purchases)
	}
	var before, after string
	if err := k.st.Pool.QueryRow(context.Background(), `select before->>'excluded', after->>'excluded' from audit_events
		where action = 'expense.item_excluded' and entity_id = $1`, id).Scan(&before, &after); err != nil || before != "false" || after != "true" {
		t.Errorf("bitácora expense.item_excluded = %s→%s (%v); quiere false→true", before, after, err)
	}
	if _, err := k.svc.SetItemExcluded(k.ctx, id, item, false, k.user); err != nil {
		t.Fatalf("incluir: %v", err)
	}
	if got := k.onHand(t); !got.Equal(dd("50000")) {
		t.Fatalf("tras incluir de nuevo = %s g; quiere 50000", got)
	}
	// Repetir no mueve nada.
	if _, err := k.svc.SetItemExcluded(k.ctx, id, item, false, k.user); err != nil {
		t.Fatal(err)
	}
	if got := k.onHand(t); !got.Equal(dd("50000")) {
		t.Fatalf("incluir dos veces dejó %s g", got)
	}
}

// Incluir un renglón que haría pasar la suma del total se rechaza con el mensaje de la 038; justo
// en el total, pasa.
func TestIncludingFollowsTheTotalRule(t *testing.T) {
	t.Parallel()
	k := newPurchaseKit(t)
	leche := ingredientIn(t, k.st, defaultCompanyID, "Leche regla", "ml")
	for _, c := range []struct {
		amount string
		ok     bool
	}{{"400", true}, {"400.01", false}} {
		in := k.mixto(t, leche, nil)
		in.Items[1].Amount = dd(c.amount)
		id, err := k.svc.CreateExpense(k.ctx, in)
		if err != nil {
			t.Fatal(err)
		}
		item, _, _ := k.itemByDescription(t, id, "Leche de la casa")
		_, err = k.svc.SetItemExcluded(k.ctx, id, item, false, k.user)
		if c.ok && err != nil {
			t.Errorf("incluir $%s con $600 en un total de $1,000: %v", c.amount, err)
		}
		if !c.ok && (!errors.Is(err, domain.ErrValidation) || !strings.Contains(err.Error(), "el total pagado")) {
			t.Errorf("incluir $%s = %v; quiere ErrValidation con el mensaje de la regla", c.amount, err)
		}
		if _, _, excluded := k.itemByDescription(t, id, "Leche de la casa"); excluded == c.ok {
			t.Errorf("incluir $%s dejó excluded=%v", c.amount, excluded)
		}
	}
}

// Dos tabletas: una excluye y otra incluye a la vez. Lo guardado queda en 0 o en N, nunca en 2N.
func TestOppositeExclusionsAtOnce(t *testing.T) {
	t.Parallel()
	k := newPurchaseKit(t)
	id, err := k.svc.CreateExpense(k.ctx, k.bultos("1", "500"))
	if err != nil {
		t.Fatal(err)
	}
	item, _ := k.itemOf(t, id)
	var wg sync.WaitGroup
	for _, v := range []bool{true, false, true, false} {
		wg.Add(1)
		go func(v bool) {
			defer wg.Done()
			svc, ctx := asTenant(t, defaultCompanyID)
			if _, err := svc.SetItemExcluded(ctx, id, item, v, k.user); err != nil && !errors.Is(err, domain.ErrConflict) {
				t.Errorf("excluir=%v: %v", v, err)
			}
		}(v)
	}
	wg.Wait()
	got := k.onHand(t)
	_, applied, excluded := k.itemByDescription(t, id, "Azúcar")
	if !(got.IsZero() && excluded) && !(got.Equal(dd("25000")) && !excluded) {
		t.Fatalf("lo guardado = %s con excluded=%v; quiere 0 excluido o 25000 incluido", got, excluded)
	}
	if !applied.Equal(got) {
		t.Fatalf("lo aplicado del renglón (%s) no es lo guardado (%s)", applied, got)
	}
}

// Todos excluidos: el gasto queda con su total como «otros» y nada entra.
func TestAllLinesExcludedLeavesTheTotal(t *testing.T) {
	t.Parallel()
	k := newPurchaseKit(t)
	in := k.bultos("1", "500")
	in.Items[0].Excluded = true
	// Un excluido en 0 que no es regalo también se guarda: es evidencia, no gasto.
	in.Items = append(in.Items, app.ExpenseItemInput{Description: "Muestra", Quantity: dd("1"), Amount: dd("0"), Excluded: true})
	id, err := k.svc.CreateExpense(k.ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	if got := k.onHand(t); !got.IsZero() {
		t.Errorf("todo excluido y entraron %s g", got)
	}
	d, err := k.svc.ExpenseDetail(k.ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if !d.Amount.Equal(dd("500")) || len(d.Items) != 2 || d.ReceivedAt != nil {
		t.Fatalf("gasto = $%s, %d renglones, recibido %v; quiere $500, 2 y sin recibir", d.Amount, len(d.Items), d.ReceivedAt)
	}
}

// «Corregir lo que llegó» no manda los excluidos: la edición los conserva (R5).
func TestSaveStockLinesKeepsExcludedLines(t *testing.T) {
	t.Parallel()
	k := newPurchaseKit(t)
	leche := ingredientIn(t, k.st, defaultCompanyID, "Leche corrige", "ml")
	id, err := k.svc.CreateExpense(k.ctx, k.mixto(t, leche, nil))
	if err != nil {
		t.Fatal(err)
	}
	item, applied, _ := k.itemByDescription(t, id, "Azúcar")
	one := dd("1")
	if _, err := k.svc.SaveStockLines(k.ctx, id, []app.StockLineInput{{
		ID: &item, SeenApplied: &applied, ItemType: "ingrediente", IngredientID: &k.azucar, Description: "Azúcar",
		Quantity: one, Received: one, PresentationID: &k.bulto, Amount: dd("600"),
	}}, k.user); err != nil {
		t.Fatalf("corregir sin mandar el excluido: %v", err)
	}
	var voided bool
	var excluded bool
	if err := k.st.Pool.QueryRow(context.Background(), `select voided_at is not null, excluded from expense_items
		where expense_id = $1 and description = 'Leche de la casa'`, id).Scan(&voided, &excluded); err != nil {
		t.Fatal(err)
	}
	if voided || !excluded {
		t.Fatalf("tras corregir, el excluido quedó quitado=%v excluido=%v; quiere conservado y excluido", voided, excluded)
	}
}

// Bajo el rol de la app, en los tres casos donde RLS ya falló: nadie fuera de la empresa excluye.
func TestSetItemExcludedIsIsolated(t *testing.T) {
	t.Parallel()
	k := newPurchaseKit(t)
	other := makeCompany(t, k.st, "excluidos-otra")
	id, err := k.svc.CreateExpense(k.ctx, k.bultos("1", "500"))
	if err != nil {
		t.Fatal(err)
	}
	item, _ := k.itemOf(t, id)
	inTheThreeCases(t, defaultCompanyID, other, func(t *testing.T, st *store.Store, ctx context.Context) {
		if _, err := app.NewBackofficeService(st, nil).SetItemExcluded(ctx, id, item, true, k.user); err == nil {
			t.Fatal("se excluyó un renglón de otra empresa")
		}
		if _, _, excluded := k.itemByDescription(t, id, "Azúcar"); excluded {
			t.Fatal("el renglón de la empresa dueña quedó excluido")
		}
	})
}

// TOTAL DEL TICKET (spec 049, historia 2).
func TestDocumentTotal(t *testing.T) {
	t.Parallel()
	k := newPurchaseKit(t)
	five := dd("500")
	in := k.bultos("1", "380")
	in.DocumentTotal = &five
	id, err := k.svc.CreateExpense(k.ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	d, err := k.svc.ExpenseDetail(k.ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if d.DocumentTotal == nil || !d.DocumentTotal.Equal(five) || !d.Amount.Equal(dd("380")) {
		t.Fatalf("total del ticket %v y del gasto %s; quiere 500 y 380", d.DocumentTotal, d.Amount)
	}
	id, err = k.svc.CreateExpense(k.ctx, k.bultos("1", "380"))
	if err != nil {
		t.Fatal(err)
	}
	if d, _ = k.svc.ExpenseDetail(k.ctx, id); d.DocumentTotal != nil {
		t.Fatalf("un gasto a mano trae total del ticket %v", d.DocumentTotal)
	}
	for _, bad := range []string{"-1", "100000000000"} {
		v := dd(bad)
		in := k.bultos("1", "380")
		in.DocumentTotal = &v
		if _, err := k.svc.CreateExpense(k.ctx, in); !errors.Is(err, domain.ErrValidation) {
			t.Errorf("total del ticket %s = %v; quiere ErrValidation", bad, err)
		}
	}
}

// Por el router: el permiso de recibir mercancía, la frontera del total del ticket y la forma del
// renglón en el detalle.
func TestExcludedItemsHTTP(t *testing.T) {
	t.Parallel()
	k := newPurchaseKit(t)
	st := k.st
	jm := auth.NewManager("secreto-de-pruebas-suficientemente-largo-para-el-manager", nil)
	h := httpapi.NewHandlers(httpapi.Deps{
		JWT: jm, Backoffice: app.NewBackofficeService(st, nil), Settings: app.NewSettingsService(st, "pepper-de-prueba"),
		MenuCache: cache.NewMenuCache(""), Broker: realtime.NewBroker(),
	})
	r := httpapi.Router(config.Config{}, jm, h, st)
	token := func(username, role string) string {
		id := makeUser(t, st, username, role)
		tok, err := jm.Issue(domain.User{ID: id, CompanyID: defaultCompanyID, Name: username, Role: domain.Role(role)})
		if err != nil {
			t.Fatal(err)
		}
		return tok
	}
	admin, cajero := token("admin_excl", "admin"), token("cajero_excl", "cajero")
	id, err := k.svc.CreateExpense(k.ctx, k.bultos("1", "500"))
	if err != nil {
		t.Fatal(err)
	}
	item, _ := k.itemOf(t, id)
	path := "/api/v1/expenses/" + strconv.FormatInt(id, 10) + "/items/" + strconv.FormatInt(item, 10)
	if w := do(t, r, http.MethodPatch, path, cajero, []byte(`{"excluded":true}`), "application/json"); w.Code != http.StatusForbidden {
		t.Fatalf("cajero excluye = %d; quiere 403", w.Code)
	}
	if w := do(t, r, http.MethodPatch, path, admin, []byte(`{}`), "application/json"); w.Code != http.StatusBadRequest {
		t.Errorf("sin «excluded» = %d; quiere 400, no un default", w.Code)
	}
	w := do(t, r, http.MethodPatch, path, admin, []byte(`{"excluded":true}`), "application/json")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"excluded":true`) || !strings.Contains(w.Body.String(), `"documentTotal":null`) {
		t.Fatalf("excluir = %d %s", w.Code, w.Body.String())
	}
	body := `{"categoryId":` + strconv.FormatInt(k.category, 10) + `,"amount":"100","status":"pendiente","documentTotal":"%s",
		"items":[{"description":"Bolsa","quantity":"1","amount":"10","personal":true}]}`
	for _, bad := range []string{"abc", "NaN", "-5"} {
		if w := do(t, r, http.MethodPost, "/api/v1/expenses", admin, []byte(strings.Replace(body, "%s", bad, 1)), "application/json"); w.Code != http.StatusBadRequest {
			t.Errorf("documentTotal %q = %d %s; quiere 400", bad, w.Code, w.Body.String())
		}
	}
	w = do(t, r, http.MethodPost, "/api/v1/expenses", admin, []byte(strings.Replace(body, "%s", "120", 1)), "application/json")
	if w.Code != http.StatusCreated && w.Code != http.StatusOK {
		t.Fatalf("crear con total del ticket = %d %s", w.Code, w.Body.String())
	}
	// El viejo «personal» se guarda excluido.
	var n int
	if err := st.Pool.QueryRow(context.Background(), `select count(*) from expense_items where description = 'Bolsa' and excluded`).Scan(&n); err != nil || n != 1 {
		t.Errorf("renglones «personal» guardados como excluidos = %d (%v); quiere 1", n, err)
	}
}

// «Corregir lo que llegó» con un renglón nuevo excluido de importe 0 y con lo recibido puesto: se
// guarda (es evidencia) y no mete nada al almacén.
func TestSaveStockLinesAcceptsANewExcludedLine(t *testing.T) {
	t.Parallel()
	k := newPurchaseKit(t)
	id, err := k.svc.CreateExpense(k.ctx, k.bultos("1", "500"))
	if err != nil {
		t.Fatal(err)
	}
	item, applied := k.itemOf(t, id)
	one := dd("1")
	lines := []app.StockLineInput{
		{ID: &item, SeenApplied: &applied, ItemType: "ingrediente", IngredientID: &k.azucar, Description: "Azúcar",
			Quantity: one, Received: one, PresentationID: &k.bulto, Amount: dd("500")},
		{ItemType: "ingrediente", IngredientID: &k.azucar, Description: "Muestra", Quantity: one, Received: one,
			PresentationID: &k.bulto, Amount: dd("0"), Excluded: true},
	}
	if _, err := k.svc.SaveStockLines(k.ctx, id, lines, k.user); err != nil {
		t.Fatalf("renglón nuevo excluido de $0: %v", err)
	}
	if got := k.onHand(t); !got.Equal(dd("25000")) {
		t.Fatalf("lo guardado = %s; el excluido nuevo metió mercancía", got)
	}
	var recv *decimal.Decimal
	if err := k.st.Pool.QueryRow(context.Background(), `select qty_received from expense_items where expense_id = $1 and description = 'Muestra'`, id).Scan(&recv); err != nil {
		t.Fatal(err)
	}
	if recv != nil {
		t.Errorf("el excluido guardó %s como recibido; no es del negocio", recv)
	}
}

// Un importe negativo en un excluido es 400 por los dos caminos, no un 500 de la base.
func TestNegativeExcludedAmountIsRejected(t *testing.T) {
	t.Parallel()
	k := newPurchaseKit(t)
	in := k.bultos("1", "500")
	in.Items = append(in.Items, app.ExpenseItemInput{Description: "Negativo", Quantity: dd("1"), Amount: dd("-1"), Excluded: true})
	if _, err := k.svc.CreateExpense(k.ctx, in); !errors.Is(err, domain.ErrValidation) {
		t.Errorf("alta con excluido negativo = %v; quiere ErrValidation", err)
	}
	id, err := k.svc.CreateExpense(k.ctx, k.bultos("1", "500"))
	if err != nil {
		t.Fatal(err)
	}
	item, applied := k.itemOf(t, id)
	one := dd("1")
	if _, err := k.svc.SaveStockLines(k.ctx, id, []app.StockLineInput{
		{ID: &item, SeenApplied: &applied, ItemType: "ingrediente", IngredientID: &k.azucar, Description: "Azúcar",
			Quantity: one, Received: one, PresentationID: &k.bulto, Amount: dd("500")},
		{ItemType: "ingrediente", IngredientID: &k.azucar, Description: "Negativo", Quantity: one,
			PresentationID: &k.bulto, Amount: dd("-1"), Excluded: true},
	}, k.user); !errors.Is(err, domain.ErrValidation) {
		t.Errorf("corregir con excluido negativo = %v; quiere ErrValidation", err)
	}
	if _, err := k.st.Pool.Exec(context.Background(), `update expense_items set excluded = true, amount = -1 where id = $1`, item); err == nil {
		t.Error("la base aceptó un excluido con importe negativo")
	}
}

// Incluir un excluido de $0 que no es regalo: es 400 con el mensaje del importe, no un 500 por el
// check de la base (23514).
func TestIncludingAZeroExcludedLineIsAValidationError(t *testing.T) {
	t.Parallel()
	k := newPurchaseKit(t)
	in := k.bultos("1", "500")
	in.Items = append(in.Items, app.ExpenseItemInput{Description: "Muestra", Quantity: dd("1"), Amount: dd("0"), Excluded: true})
	id, err := k.svc.CreateExpense(k.ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	item, _, _ := k.itemByDescription(t, id, "Muestra")
	_, err = k.svc.SetItemExcluded(k.ctx, id, item, false, k.user)
	if !errors.Is(err, domain.ErrValidation) || !strings.Contains(err.Error(), "importe") {
		t.Fatalf("incluir un excluido de $0 = %v; quiere ErrValidation que hable del importe", err)
	}
	if _, _, excluded := k.itemByDescription(t, id, "Muestra"); !excluded {
		t.Fatal("el rechazo dejó el renglón incluido")
	}
}

// Decisión (spec 049): lo recibido de un excluido se ignora, también con «llegó todo». Al incluirlo
// después queda por recibir y entra con «Recibir» o «Llegó todo»: no se inventa que llegó.
func TestIncludingAnExcludedLineFromAReceivedPurchaseLeavesItPending(t *testing.T) {
	t.Parallel()
	k := newPurchaseKit(t)
	leche := ingredientIn(t, k.st, defaultCompanyID, "Leche llegó todo", "ml")
	in := k.mixto(t, leche, nil)
	in.Items[1].Amount = dd("400")
	id, err := k.svc.CreateExpense(k.ctx, in) // ReceivedAt puesto: «llegó todo»
	if err != nil {
		t.Fatal(err)
	}
	item, _, _ := k.itemByDescription(t, id, "Leche de la casa")
	d, err := k.svc.SetItemExcluded(k.ctx, id, item, false, k.user)
	if err != nil {
		t.Fatal(err)
	}
	if got := onHandIngredient(t, k.st, leche); !got.IsZero() {
		t.Errorf("incluir metió %s ml sin que nadie dijera que llegó", got)
	}
	if d.ReceivedAt != nil {
		t.Error("la compra sigue como recibida con un renglón incluido que no ha llegado")
	}
	pend, err := k.svc.PendingReceipts(k.ctx)
	if err != nil {
		t.Fatal(err)
	}
	var seen bool
	for _, p := range pend {
		seen = seen || p.ItemID == item
	}
	if !seen {
		t.Error("el renglón incluido no aparece en «Por recibir»")
	}
}
