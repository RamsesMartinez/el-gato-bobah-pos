//go:build integration

package integration

import (
	"context"
	"sync"
	"testing"

	"uuid"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
)

// «ENVIAR Y COBRAR» POR PRODUCTOS DESDE UNA CUENTA EN CAPTURA (US4, casos 11 y 26).
//
// La tableta manda la cuenta y cobra sobre el pedido que regresa (research R-5): «Por productos»
// necesita ids de renglón de pedido, que antes de enviar no existen.
func TestSendThenChargeByProducts(t *testing.T) {
	k := newLiveKit(t)
	abrirCajaPrincipal(t, k.st, k.user)
	taro := makeProduct(t, k.st, "Taro por productos", pesos("50"), false)
	crepa := makeProduct(t, k.st, "Crepa por productos", pesos("80"), false)
	v := k.newDraft(t, addOf(taro, "2"), addOf(crepa, "2")) // $260

	sent, err := k.drafts.Send(k.ctx, v.ID, k.user)
	if err != nil {
		t.Fatal(err)
	}
	var taroLine int64
	for _, l := range sent.Order.Lines {
		if l.ProductName == "Taro por productos" {
			taroLine = l.ID
		}
	}
	if _, err := k.orders.Charge(k.ctx, app.ChargeCmd{OrderID: sent.Order.ID, MethodID: k.efectivo, ActorID: k.user,
		ClientUUID: uuid.New(), Lines: []domain.SelectedPieces{{LineID: taroLine, Qty: pesos("2")}}}); err != nil {
		t.Fatalf("cobrar 2 de 4 productos: %v", err)
	}
	_, byKey := k.live(t, k.ctx, false)
	it, ok := byKey[orderKey(sent.Order.ID)]
	if !ok || it.State != domain.AccountPartlyPaid || !it.Outstanding.Equal(pesos("160")) {
		t.Fatalf("la fila dice %+v; quería «Pago parcial · falta $160»", it)
	}
	if _, ok := byKey[draftKey(v.ID)]; ok {
		t.Fatal("la cuenta enviada sigue como ficha aparte: dos fichas para la misma mesa")
	}
}

// Dos tabletas «Enviar y cobrar» la misma cuenta a la vez: un pedido, y el segundo cobro solo cobra lo
// que falta (edge case del spec).
func TestTwoTabletsSendAndChargeTheSameAccount(t *testing.T) {
	k := newLiveKit(t)
	abrirCajaPrincipal(t, k.st, k.user)
	v := k.newDraft(t, addOf(k.product, "2")) // $200
	ctxs := []context.Context{k.tenant(t, k.company), k.tenant(t, k.company)}
	orders := make([]int64, 2)
	errs := make([]error, 2)
	var wg sync.WaitGroup
	for i := range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			res, err := k.drafts.Send(ctxs[i], v.ID, k.user)
			if err != nil {
				errs[i] = err
				return
			}
			orders[i] = res.Order.ID
			_, errs[i] = k.orders.Charge(ctxs[i], app.ChargeCmd{OrderID: res.Order.ID, MethodID: k.efectivo,
				ActorID: k.user, ClientUUID: uuid.New(), AllRemaining: true})
		}()
	}
	wg.Wait()
	if orders[0] == 0 || orders[0] != orders[1] {
		t.Fatalf("pedidos %v (errores %v): la misma cuenta se volvió dos pedidos", orders, errs)
	}
	paid, _ := sumaDePagos(t, k.st, orders[0])
	if !paid.Equal(pesos("200")) {
		t.Fatalf("pagado = %s sobre $200 (errores %v): el segundo cobro tenía que cobrar solo lo que faltaba", paid, errs)
	}
}

func TestChargedAccountInTheThreeCases(t *testing.T) {
	k := newLiveKit(t)
	abrirCajaPrincipal(t, k.st, k.user)
	other := makeCompany(t, k.st, "otra-cobro")
	v := k.newDraft(t, addOf(k.product, "1"))
	sent, err := k.drafts.Send(k.ctx, v.ID, k.user)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := k.orders.Charge(k.ctx, app.ChargeCmd{OrderID: sent.Order.ID, MethodID: k.efectivo, Amount: pesos("40"), ActorID: k.user}); err != nil {
		t.Fatal(err)
	}
	inTheThreeCases(t, k.company, other, func(t *testing.T, st *store.Store, ctx context.Context) {
		res, err := app.NewAccountsService(st, app.NewOrdersService(st, clock)).Live(ctx, true)
		if err != nil {
			return
		}
		for _, it := range res.Items {
			if it.OrderID != nil && *it.OrderID == sent.Order.ID {
				t.Fatal("la otra empresa ve la cuenta cobrada a medias de la dueña")
			}
		}
	})
}
