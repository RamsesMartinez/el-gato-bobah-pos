//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"

	"uuid"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
)

// UN PRODUCTO AGREGADO POR ERROR A UN PEDIDO YA COBRADO SE PUEDE QUITAR AUNQUE FIGURE ENTREGADO.
//
// El caso real: un pedido de mostrador se cobró, se le agregó un producto más (que se cobró en otro
// pedido) y al marcar el pedido como entregado el renglón nuevo quedó «entregado» sin servirse desde
// ahí. Cancelar el renglón se rechazaba (ya entregado) y devolver dinero no aplicaba (no había
// dinero): el pedido se quedaba con un saldo pendiente para siempre.
func TestUnRenglonEntregadoSinPagoSePuedeAnular(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()
	svc := app.NewOrdersService(st, clock)

	cajero := makeUser(t, st, "cajero_anular_sin_pago", "cajero")
	efectivo := paymentMethodID(t, st, "Efectivo")
	abrirCajaPrincipal(t, st, cajero)

	mochi := makeProduct(t, st, "Producto cobrado", pesos("80"), false)
	cafe := makeProduct(t, st, "Producto agregado por error", pesos("69"), false)

	ord, err := svc.Create(ctx, app.CreateOrderCmd{
		ClientUUID: uuid.New(), ServiceType: "mostrador", OpenedBy: cajero,
		Lines: []domain.OrderLineInput{{ProductID: mochi, Qty: pesos("1")}},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := svc.Charge(ctx, app.ChargeCmd{OrderID: ord.ID, MethodID: efectivo, Amount: pesos("80"), ActorID: cajero}); err != nil {
		t.Fatalf("cobrar el pedido: %v", err)
	}
	// Se agrega el producto al pedido que ya estaba cobrado y luego se entrega TODO el pedido.
	ord, err = svc.AddLines(ctx, ord.ID, []domain.OrderLineInput{{ProductID: cafe, Qty: pesos("1")}}, cajero, uuid.New())
	if err != nil {
		t.Fatalf("AddLines: %v", err)
	}
	if err := svc.DeliverAll(ctx, ord.ID); err != nil {
		t.Fatalf("DeliverAll: %v", err)
	}
	var cobrado, agregado int64
	for _, l := range ord.Lines {
		if l.ProductName == "Producto agregado por error" {
			agregado = l.ID
		} else {
			cobrado = l.ID
		}
	}
	if agregado == 0 || cobrado == 0 {
		t.Fatalf("no encontré los dos renglones en %+v", ord.Lines)
	}

	// El punto de partida: cancelar el renglón normal se rechaza, que es el problema.
	if _, err := svc.CancelarRenglon(ctx, ord.ID, agregado, cajero, "agregado por error"); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("CancelarRenglon sobre el entregado debía ser 409, fue %v", err)
	}

	// Cobrado POR MONTO no deja piezas cubiertas, así que lo que impide quitar el renglón equivocado
	// es la otra barrera: el total no puede quedar por debajo de lo cobrado.
	if _, err := svc.AnularRenglonSinPago(ctx, ord.ID, cobrado, cajero, "prueba"); !errors.Is(err, domain.ErrOrderWouldBeOverpaid) {
		t.Fatalf("quitar el renglón cobrado debía rechazarse por sobrepago, fue %v", err)
	}
	// Sin motivo no se hace nada.
	if _, err := svc.AnularRenglonSinPago(ctx, ord.ID, agregado, cajero, "  "); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("sin motivo debía ser validación, fue %v", err)
	}

	// El que no tiene pago sí se anula.
	if _, err := svc.AnularRenglonSinPago(ctx, ord.ID, agregado, cajero, "agregado por error; se cobró en otro pedido"); err != nil {
		t.Fatalf("AnularRenglonSinPago: %v", err)
	}
	// Idempotente: un doble toque no falla ni anula otra cosa.
	if _, err := svc.AnularRenglonSinPago(ctx, ord.ID, agregado, cajero, "agregado por error; se cobró en otro pedido"); err != nil {
		t.Fatalf("el segundo toque debía ser un no-op, fue %v", err)
	}

	det, err := svc.Detail(ctx, ord.ID)
	if err != nil {
		t.Fatalf("Detail: %v", err)
	}
	var cancelado bool
	for _, l := range det.Lines {
		if l.ID == agregado {
			cancelado = l.Cancelled
		}
	}
	if !cancelado {
		t.Fatal("el renglón debía quedar cancelado")
	}
	if !det.Total.Equal(pesos("80")) || !det.Outstanding.IsZero() || !det.Paid {
		t.Fatalf("total=%s saldo=%s pagado=%v: debían quedar en 80, sin saldo y pagado", det.Total, det.Outstanding, det.Paid)
	}
	var eventos int
	if err := st.Pool.QueryRow(ctx, `select count(*) from audit_events where order_id = $1 and action = 'order.line_removed' and after->>'unpaid' = 'true'`, ord.ID).Scan(&eventos); err != nil {
		t.Fatalf("leer la auditoría: %v", err)
	}
	if eventos != 1 {
		t.Fatalf("debía haber UN evento de auditoría y hay %d", eventos)
	}
}

// Un renglón CUBIERTO por un pago (cobro por productos) no se anula: primero se devuelve el pago.
func TestUnRenglonCubiertoPorUnPagoNoSeAnula(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()
	svc := app.NewOrdersService(st, clock)

	cajero := makeUser(t, st, "cajero_anular_cubierto", "cajero")
	efectivo := paymentMethodID(t, st, "Efectivo")
	abrirCajaPrincipal(t, st, cajero)

	a := makeProduct(t, st, "Producto uno", pesos("80"), false)
	b := makeProduct(t, st, "Producto dos", pesos("69"), false)
	ord, err := svc.Create(ctx, app.CreateOrderCmd{
		ClientUUID: uuid.New(), ServiceType: "mostrador", OpenedBy: cajero,
		Lines: []domain.OrderLineInput{{ProductID: a, Qty: pesos("1")}, {ProductID: b, Qty: pesos("1")}},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := svc.Charge(ctx, app.ChargeCmd{
		OrderID: ord.ID, MethodID: efectivo, ActorID: cajero,
		Lines: []domain.SelectedPieces{{LineID: ord.Lines[0].ID, Qty: pesos("1")}},
	}); err != nil {
		t.Fatalf("cobrar el primer producto: %v", err)
	}
	if err := svc.DeliverAll(ctx, ord.ID); err != nil {
		t.Fatalf("DeliverAll: %v", err)
	}
	if _, err := svc.AnularRenglonSinPago(ctx, ord.ID, ord.Lines[0].ID, cajero, "prueba"); !errors.Is(err, domain.ErrPieceAlreadyPaid) {
		t.Fatalf("anular un renglón cubierto debía rechazarse con ErrPieceAlreadyPaid, fue %v", err)
	}
}
