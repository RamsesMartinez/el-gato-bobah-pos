//go:build integration

package integration

import (
	"context"
	"testing"

	"github.com/shopspring/decimal"
	"uuid"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
)

// El ajuste nace APAGADO: cobrar es del punto de venta, y una pantalla de cocina con botón de
// cobrar le da acceso al dinero a quien solo tiene que preparar comida. Se enciende a propósito en
// el local donde cocina y mostrador son la misma persona.
func TestCobrarDesdePedidosNaceApagado(t *testing.T) {
	st := newTestStore(t)
	settings := app.NewSettingsService(st, "pepper-de-prueba")

	ajustes, err := settings.Get(context.Background())
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if ajustes.KitchenCanCharge {
		t.Fatal("el ajuste nació encendido: el tablero podría cobrar sin que nadie lo decidiera")
	}
}

// Se enciende sin pisar los demás ajustes del ticket, que viven en la misma fila y se escriben con
// el mismo UPDATE. Un interruptor que apaga otro es el fallo clásico de este patrón.
func TestElAjusteDeCobroNoPisaLosDelTicket(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	settings := app.NewSettingsService(st, "pepper-de-prueba")
	admin := makeUser(t, st, "admin_kcc", "admin")

	antes, err := settings.Get(ctx)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	// Se deja la comanda ENCENDIDA para que el test note si guardar el cobro la apaga.
	print := domain.PrintSettings{
		AutoPrintOnClose:   antes.AutoPrintOnClose,
		PrintFreeModifiers: antes.PrintFreeModifiers,
		PrintKitchenTicket: true,
		KitchenCanCharge:   true,
	}
	info := domain.BusinessInfo{
		Name: antes.BusinessName, Address: antes.Address, Phone: antes.Phone,
		HeaderNote: antes.HeaderNote, FooterNote: antes.FooterNote,
	}
	// Los ajustes de identificación se pasan como están: este test es de los del ticket y no debe
	// moverlos de paso.
	ident := domain.IdentitySettings{
		PinOnlyUnlock:    antes.PinOnlyUnlock,
		LockAfterSeconds: antes.LockAfterSeconds,
		SessionHours:     antes.SessionHours,
	}
	if _, err := settings.SetBusinessInfo(ctx, info, print, ident, antes.Timezone, admin); err != nil {
		t.Fatalf("SetBusinessInfo: %v", err)
	}

	tras, err := settings.Get(ctx)
	if err != nil {
		t.Fatalf("Get tras update: %v", err)
	}
	if !tras.KitchenCanCharge {
		t.Error("el ajuste no se guardó")
	}
	if !tras.PrintKitchenTicket {
		t.Error("encender el cobro apagó la comanda de cocina")
	}
}

// El aviso del POS lista lo que falta por cobrar, en cualquier estado que siga siendo cobrable.
// Existe aparte de la lista de entregadas porque esa es de admin/gerente, y el pendiente más caro
// —entregado y sin cobrar, el cliente ya se fue— tiene que poder saldarlo quien está en la caja.
func TestElAvisoDelPOSListaLoQueFaltaPorCobrar(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	svc := app.NewOrdersService(st, clock)

	cajero := makeUser(t, st, "cajero_unpaid", "cajero")
	prod := makeProduct(t, st, "Café unpaid", decimal.RequireFromString("50"), false)
	efectivo := paymentMethodID(t, st, "Efectivo")
	abrirCajaPrincipal(t, st, cajero)

	nuevo := func(pagos []app.PaymentInput) *app.OrderView {
		t.Helper()
		o, err := svc.Create(ctx, app.CreateOrderCmd{
			ClientUUID: uuid.New(), ServiceType: "mostrador", OpenedBy: cajero,
			Lines:    []domain.OrderLineInput{{ProductID: prod, Qty: decimal.RequireFromString("1")}},
			Payments: pagos,
		})
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		return o
	}

	pagado := nuevo([]app.PaymentInput{{MethodID: efectivo, Amount: decimal.RequireFromString("50")}})
	sinCobrar := nuevo(nil)
	abonado := nuevo([]app.PaymentInput{{MethodID: efectivo, Amount: decimal.RequireFromString("20")}})
	cancelado := nuevo(nil)
	if err := svc.Cancel(ctx, cancelado.ID, cajero, "se arrepintió"); err != nil {
		t.Fatalf("Cancel: %v", err)
	}

	items, err := svc.Unpaid(ctx)
	if err != nil {
		t.Fatalf("Unpaid: %v", err)
	}
	falta := map[int64]string{}
	for _, o := range items {
		falta[o.ID] = o.Outstanding.String()
	}

	if _, hay := falta[pagado.ID]; hay {
		t.Error("un pedido ya cobrado salió en la lista de pendientes")
	}
	// Su dinero ya se decidió: listarlo mandaría al operador a perseguir un cobro que nadie debe.
	if _, hay := falta[cancelado.ID]; hay {
		t.Error("un pedido cancelado salió en la lista de pendientes")
	}
	if falta[sinCobrar.ID] != "50" {
		t.Errorf("el pedido sin cobrar debe 50, dice %q", falta[sinCobrar.ID])
	}
	// Lo que FALTA, no el total: cobrarle 50 a quien ya dejó 20 es cobrarle dos veces esa parte.
	if falta[abonado.ID] != "30" {
		t.Errorf("el pedido abonado debe 30, dice %q", falta[abonado.ID])
	}
}
