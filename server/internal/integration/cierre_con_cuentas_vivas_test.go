//go:build integration

package integration

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
)

// CERRAR CAJA CON CUENTAS VIVAS (US8, D-10, FR-014, caso 21).
//
// Bloquea solo lo que está en cocina o listo, como hoy. Lo que se está capturando y lo entregado que
// debe se LISTA en la vista del turno —para abrirlo o descartarlo— pero no detiene el cierre: una
// cuenta fiada tiene que poder pasar al día siguiente.
func TestCloseLiveAccountsDoNotBlock(t *testing.T) {
	k := newLiveKit(t)
	ctx := k.ctx
	back := app.NewBackofficeService(k.appSt, clock)
	principal := registerID(t, k.st, "Caja principal")
	abrirCajaPrincipal(t, k.st, k.user)
	closeShift := func() error {
		_, err := back.CloseSession(ctx, principal, k.user, cierreDelCajonAMano(t, k.st, map[int]decimal.Decimal{int(k.efectivo): pesos("100")}))
		return err
	}

	t.Run("un pedido en cocina bloquea, y la vista trae su id para abrirlo", func(t *testing.T) {
		inKitchen := k.order(t, "1", "0", false)
		if err := closeShift(); !errors.Is(err, domain.ErrOpenOrders) {
			t.Fatalf("cerrar con un pedido en cocina = %v, quería OPEN_ORDERS", err)
		}
		view, err := back.CurrentByRegister(ctx, principal)
		if err != nil {
			t.Fatal(err)
		}
		if len(view.Pending) != 1 || view.Pending[0].ID != inKitchen.ID {
			t.Fatalf("pending = %+v: «Abrir» del cierre necesita el id del pedido", view.Pending)
		}
		for _, it := range view.LiveAccounts {
			if it.OrderID != nil && *it.OrderID == inKitchen.ID {
				t.Fatal("el pedido en cocina salió también en liveAccounts: se listaría dos veces")
			}
		}
		if err := k.orders.DeliverAll(ctx, inKitchen.ID); err != nil {
			t.Fatal(err)
		}
		if _, err := k.orders.Charge(ctx, app.ChargeCmd{OrderID: inKitchen.ID, MethodID: k.efectivo, Amount: pesos("100"), ActorID: k.user}); err != nil {
			t.Fatal(err)
		}
	})

	capturing := k.newDraft(t, addOf(k.product, "1"))
	owes := k.order(t, "1", "0", true)
	old := k.order(t, "1", "0", true)
	k.ageOrder(t, old.ID, 100)

	view, err := back.CurrentByRegister(ctx, principal)
	if err != nil {
		t.Fatal(err)
	}
	listed := map[string]app.AccountItem{}
	for _, it := range view.LiveAccounts {
		listed[it.Key] = it
	}
	for _, key := range []string{draftKey(capturing.ID), orderKey(owes.ID), orderKey(old.ID)} {
		if _, ok := listed[key]; !ok {
			t.Errorf("liveAccounts no trae %s (la de hace 100 días también: el cierre pide olderDebts)", key)
		}
	}
	if err := closeShift(); err != nil {
		t.Fatalf("con solo cuentas capturándose y entregadas que deben, el cierre se negó: %v", err)
	}

	t.Run("al día siguiente la deuda está en «De días anteriores» y la cuenta sigue viva", func(t *testing.T) {
		tomorrow := app.NewOrdersService(k.appSt, func() time.Time { return fixedNow.Add(24 * time.Hour) })
		res, err := app.NewAccountsService(k.appSt, tomorrow).Live(ctx, false)
		if err != nil {
			t.Fatal(err)
		}
		byKey := map[string]app.AccountItem{}
		for _, it := range res.Items {
			byKey[it.Key] = it
		}
		if it, ok := byKey[orderKey(owes.ID)]; !ok || it.Group != domain.GroupPreviousDays {
			t.Fatalf("la deuda de ayer = %+v", it)
		}
		if _, ok := byKey[draftKey(capturing.ID)]; !ok {
			t.Fatal("la cuenta capturándose desapareció al cerrar el turno: no cuelga de ninguno (D-6)")
		}
	})
}

// liveAccounts y pending viajan como arreglo, nunca null, en el JSON crudo de la vista del turno.
func TestSessionViewArraysAreNeverNull(t *testing.T) {
	k := newLiveKit(t)
	back := app.NewBackofficeService(k.appSt, clock)
	principal := registerID(t, k.st, "Caja principal")
	abrirCajaPrincipal(t, k.st, k.user)
	view, err := back.CurrentByRegister(k.ctx, principal)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(view)
	m := rawJSON(t, b)
	mustArray(t, m, "liveAccounts")
	mustArray(t, m, "pending")
}
