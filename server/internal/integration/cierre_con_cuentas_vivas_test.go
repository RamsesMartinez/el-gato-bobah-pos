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

// CERRAR CAJA CON CUENTAS VIVAS (US8, D-10, FR-014, caso 21) Y SIN FIADOS (dueño, 2026-10-09).
//
// Bloquea lo que está en cocina o listo, y desde el 2026-10-09 también lo entregado que debe, de
// cualquier día: no hay fiados. Lo que se está capturando se LISTA —para abrirlo o descartarlo—
// pero no detiene el cierre. Un entregado que debe se resuelve cobrándolo o cancelándolo con motivo.
func TestCloseLiveAccountsDoNotBlock(t *testing.T) {
	t.Parallel()
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
		// «Falta entregar 3 pedidos» sin montos obligaba a abrir cada uno para saber de cuánto era.
		if !view.Pending[0].Total.Equal(inKitchen.Total) {
			t.Fatalf("pending total = %s, quería %s: el cierre dice de cuánto es cada pedido sin abrirlo",
				view.Pending[0].Total, inKitchen.Total)
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
	// De plataforma y debiendo: lo paga la plataforma, no bloquea la caja.
	platform := k.order(t, "1", "0", true)
	if _, err := k.st.Pool.Exec(ctx,
		`update orders set service_type = 'domicilio', delivery_platform_id = (select min(id) from delivery_platforms) where id = $1`, platform.ID); err != nil {
		t.Fatal(err)
	}

	view, err := back.CurrentByRegister(ctx, principal)
	if err != nil {
		t.Fatal(err)
	}
	listed := map[string]app.AccountItem{}
	for _, it := range view.LiveAccounts {
		listed[it.Key] = it
	}
	if _, ok := listed[draftKey(capturing.ID)]; !ok {
		t.Error("liveAccounts no trae la cuenta que se captura")
	}
	// La lista del cierre sale del MISMO predicado que la guardia.
	owing := map[int64]bool{}
	for _, o := range view.Owing {
		owing[o.ID] = true
	}
	if !owing[owes.ID] || !owing[old.ID] || owing[platform.ID] || len(view.Owing) != 2 {
		t.Fatalf("owing = %+v: quiere el de hoy y el de hace 100 días, sin el de plataforma", view.Owing)
	}

	err = closeShift()
	if !errors.Is(err, domain.ErrUnpaidOrders) {
		t.Fatalf("cerrar con entregados que deben = %v, quiere ErrUnpaidOrders: no hay fiados", err)
	}

	// Uno se cobra y el otro se cancela con su motivo; entonces sí cierra.
	if _, err := k.orders.Charge(ctx, app.ChargeCmd{OrderID: owes.ID, MethodID: k.efectivo, Amount: owes.Total, ActorID: k.user}); err != nil {
		t.Fatalf("cobrar el que debía: %v", err)
	}
	if err := closeShift(); !errors.Is(err, domain.ErrUnpaidOrders) {
		t.Fatalf("con uno todavía debiendo, cerrar = %v, quiere ErrUnpaidOrders", err)
	}
	if err := k.orders.CancelarConDevolucion(ctx, app.CancelacionCmd{
		OrderID: old.ID, Motivo: "se fue sin pagar", ActorID: k.user,
	}); err != nil {
		t.Fatalf("cancelar el entregado que debía: %v", err)
	}
	if err := closeShift(); err != nil {
		t.Fatalf("con lo entregado cobrado o cancelado, el cierre se negó: %v", err)
	}

	t.Run("la cuenta que se capturaba sigue viva al día siguiente", func(t *testing.T) {
		tomorrow := app.NewOrdersService(k.appSt, func() time.Time { return fixedNow.Add(24 * time.Hour) })
		res, err := app.NewAccountsService(k.appSt, tomorrow).Live(ctx, false)
		if err != nil {
			t.Fatal(err)
		}
		byKey := map[string]app.AccountItem{}
		for _, it := range res.Items {
			byKey[it.Key] = it
		}
		if _, ok := byKey[draftKey(capturing.ID)]; !ok {
			t.Fatal("la cuenta capturándose desapareció al cerrar el turno: no cuelga de ninguno (D-6)")
		}
	})
}

// Cancelar un entregado que ya tiene un cobro adentro se niega: sacaría de Ventas un dinero que sí
// está en el cajón. Lo que falta se cobra.
func TestCancelDeliveredWithPaymentsIsRejected(t *testing.T) {
	t.Parallel()
	k := newLiveKit(t)
	abrirCajaPrincipal(t, k.st, k.user)
	half := k.order(t, "1", "0", true)
	if _, err := k.orders.Charge(k.ctx, app.ChargeCmd{OrderID: half.ID, MethodID: k.efectivo,
		Amount: half.Total.Div(decimal.NewFromInt(2)).Round(2), ActorID: k.user}); err != nil {
		t.Fatal(err)
	}
	err := k.orders.CancelarConDevolucion(k.ctx, app.CancelacionCmd{OrderID: half.ID, Motivo: "se fue sin pagar", ActorID: k.user})
	if !errors.Is(err, domain.ErrCancelDeliveredWithPayments) {
		t.Fatalf("cancelar un entregado con pagos = %v, quiere ErrCancelDeliveredWithPayments", err)
	}
}

// liveAccounts y pending viajan como arreglo, nunca null, en el JSON crudo de la vista del turno.
func TestSessionViewArraysAreNeverNull(t *testing.T) {
	t.Parallel()
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
	mustArray(t, m, "owing")
}
