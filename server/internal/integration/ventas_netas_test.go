//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"

	"github.com/shopspring/decimal"
	"uuid"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
)

// Spec 029: el Total de Ventas es lo que se puede facturar, y el detalle y la API dicen la verdad.
// Los cuatro hallazgos de la prueba por API en el ambiente de pruebas están aquí, cada uno contra
// el caso exacto que se midió.

// EL DETALLE DE UN PEDIDO DICE CUÁNTO SE DEVOLVIÓ.
//
// `GET /orders/:id` respondía `"refund":"0"` siempre: `load` nunca llenaba el campo, solo el tablero
// y las entregadas. Medido con un pedido devuelto completo por $100. Es el otro extremo del tope de
// una devolución, así que una pantalla que lo leyera ofrecería devolver dos veces lo mismo.
func TestOrderDetailCarriesTheRefundedAmount(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	orders := app.NewOrdersService(st, clock)
	cajero := makeUser(t, st, "cajero_029_detalle", "gerente")
	efectivo := paymentMethodID(t, st, "Efectivo")
	abrirCajaPrincipal(t, st, cajero)

	ord := pedidoCobradoParcial(t, ctx, st, orders, "029-detalle", "100", "100", cajero, efectivo, true)
	refund(t, ctx, orders, ord, nil, "100", cajero)

	v, err := orders.Detail(ctx, ord)
	if err != nil {
		t.Fatalf("Detail: %v", err)
	}
	if !v.Refund.Equal(decimal.RequireFromString("100")) {
		t.Fatalf("refund del detalle = %s, quiere 100: el detalle decía 0 de un pedido devuelto completo", v.Refund)
	}
}

// DEVOLVER UN PRODUCTO YA DEVUELTO DICE QUE NO QUEDA NADA.
//
// Sin monto se pide «lo que queda», que es $0, y el rechazo decía «el monto a devolver no es una
// cantidad de dinero». Igual tras devolver la cuenta entera.
func TestRefundingAFullyRefundedLineSaysNothingIsLeft(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	orders := app.NewOrdersService(st, clock)
	cajero := makeUser(t, st, "cajero_029_renglon", "gerente")
	efectivo := paymentMethodID(t, st, "Efectivo")
	abrirCajaPrincipal(t, st, cajero)

	ord, lines := twoLineOrder(t, ctx, st, orders, "029r", "60", "40", cajero)
	charge(t, ctx, orders, ord, efectivo, "100", cajero)
	refundLine(t, ctx, orders, ord, lines[0], "60", cajero)

	for _, caso := range []struct {
		nombre string
		antes  func()
	}{
		{"renglón ya devuelto", func() {}},
		{"tras devolver la cuenta entera", func() { refund(t, ctx, orders, ord, nil, "40", cajero) }},
	} {
		caso.antes()
		monto, err := orders.PorDevolver(ctx, ord, &lines[0])
		if err != nil {
			t.Fatalf("%s: PorDevolver: %v", caso.nombre, err)
		}
		err = orders.Devolver(ctx, app.DevolucionCmd{OrderID: ord, LineID: &lines[0], Monto: monto, Motivo: "prueba", ActorID: cajero})
		if !errors.Is(err, domain.ErrNothingLeftOnLine) {
			t.Fatalf("%s: err = %v, quiere «de ese producto ya no queda nada por devolver»", caso.nombre, err)
		}
	}
}

// UN PEDIDO DE PLATAFORMA DE MOSTRADOR ES UN ERROR DE CAPTURA, NO DEL SERVIDOR.
//
// La base lo rechaza con `orders_servicio_de_plataforma` (23514) y ese error subía crudo: 500.
func TestAPlatformOrderAtTheCounterIsAValidationError(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	orders := app.NewOrdersService(st, clock)
	cajero := makeUser(t, st, "cajero_029_plataforma", "gerente")
	abrirCajaPrincipal(t, st, cajero)
	uber := platformID(t, st, defaultCompanyID, "Uber Eats")
	prod := makeProduct(t, st, "Crepa 029", decimal.RequireFromString("135"), false)

	_, err := orders.Create(ctx, app.CreateOrderCmd{
		ClientUUID: uuid.New(), ServiceType: "mostrador", DeliveryPlatformID: &uber, OpenedBy: cajero,
		Lines: []domain.OrderLineInput{{ProductID: prod, Qty: decimal.RequireFromString("1")}},
	})
	if !errors.Is(err, domain.ErrPlatformOrderAtCounter) {
		t.Fatalf("err = %v, quiere ErrPlatformOrderAtCounter (un 4xx, no el check de la base)", err)
	}
}

// MEDIO CENTAVO NO SALDA NADA.
//
// $0.005 se redondeaba a $0.01 y entraba como pago.
func TestChargingLessThanACentIsRejected(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	orders := app.NewOrdersService(st, clock)
	cajero := makeUser(t, st, "cajero_029_centavo", "gerente")
	efectivo := paymentMethodID(t, st, "Efectivo")
	abrirCajaPrincipal(t, st, cajero)
	prod := makeProduct(t, st, "Frappe 029", decimal.RequireFromString("100"), false)
	ord := crearPedidoSimple(t, ctx, orders, prod, cajero)

	_, err := orders.Charge(ctx, app.ChargeCmd{OrderID: ord, MethodID: efectivo, Amount: decimal.RequireFromString("0.005"), ActorID: cajero})
	if !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("cobrar 0.005: err = %v, quiere validación", err)
	}
	v, err := orders.Detail(ctx, ord)
	if err != nil {
		t.Fatal(err)
	}
	if !v.Outstanding.Equal(decimal.RequireFromString("100")) {
		t.Fatalf("por cobrar = %s, quiere 100: el medio centavo entró como pago", v.Outstanding)
	}
}
