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
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
)

// pedidoSinCobrar deja un pedido de $250 mandado a cocina y NO cobrado, que es lo que produce
// "Enviar a cocina".
func pedidoSinCobrar(t *testing.T, st *store.Store, svc *app.OrdersService, sufijo string) (*app.OrderView, int64, int16) {
	t.Helper()
	cajero := makeUser(t, st, "cajero_"+sufijo, "cajero")
	prod := makeProduct(t, st, "Alitas "+sufijo, decimal.RequireFromString("250"), false)
	efectivo := paymentMethodID(t, st, "Efectivo")
	abrirCajaPrincipal(t, st, cajero)

	ord, err := crearYCobrar(t, context.Background(), svc, app.CreateOrderCmd{
		ClientUUID: uuid.New(), ServiceType: "mostrador", OpenedBy: cajero,
		Lines: []domain.OrderLineInput{{ProductID: prod, Qty: decimal.RequireFromString("1")}},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if ord.Paid {
		t.Fatal("el pedido nació pagado y debía nacer por cobrar")
	}
	return ord, cajero, efectivo
}

// EL HUECO QUE ESTO CIERRA: el tablero marcaba "POR COBRAR" y no había forma de saldarlo — el
// único lugar del sistema que registraba un pago de pedido era la creación. El operador veía la
// deuda y su única salida era levantar un pedido nuevo con los mismos productos, que descuenta el
// inventario dos veces y reporta una venta que no ocurrió.
func TestUnPedidoMandadoACocinaSePuedeCobrarDespues(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	svc := app.NewOrdersService(st, clock)
	ord, cajero, efectivo := pedidoSinCobrar(t, st, svc, "despues")

	if _, err := svc.Charge(ctx, app.ChargeCmd{
		OrderID: ord.ID, MethodID: efectivo, Amount: decimal.RequireFromString("250"), ActorID: cajero,
	}); err != nil {
		t.Fatalf("Charge: %v", err)
	}
	tras, err := svc.Detail(ctx, ord.ID)
	if err != nil {
		t.Fatalf("Detail: %v", err)
	}
	if !tras.Paid {
		t.Fatal("después de cobrarlo sigue marcado por cobrar")
	}
}

// Un doble tap sobre "Cobrar $250" registraría $500 de ingreso por comida que se vendió una vez, y
// el corte cuadraría contra una cifra inventada.
func TestNoSePuedeCobrarDosVecesElMismoPedido(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	svc := app.NewOrdersService(st, clock)
	ord, cajero, efectivo := pedidoSinCobrar(t, st, svc, "doble")

	cobro := app.ChargeCmd{OrderID: ord.ID, MethodID: efectivo, Amount: decimal.RequireFromString("250"), ActorID: cajero}
	if _, err := svc.Charge(ctx, cobro); err != nil {
		t.Fatalf("primer cobro: %v", err)
	}
	if _, err := svc.Charge(ctx, cobro); !errors.Is(err, domain.ErrPedidoYaPagado) {
		t.Fatalf("segundo cobro = %v, quiere ErrPedidoYaPagado", err)
	}
}

// Se puede abonar: el cliente deja algo y termina de pagar al recoger.
func TestSePuedeAbonarYLuegoCompletar(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	svc := app.NewOrdersService(st, clock)
	ord, cajero, efectivo := pedidoSinCobrar(t, st, svc, "abono")

	if _, err := svc.Charge(ctx, app.ChargeCmd{
		OrderID: ord.ID, MethodID: efectivo, Amount: decimal.RequireFromString("100"), ActorID: cajero,
	}); err != nil {
		t.Fatalf("abono: %v", err)
	}
	medio, _ := svc.Detail(ctx, ord.ID)
	if medio.Paid {
		t.Fatal("con $100 de $250 no debería estar pagado")
	}

	// Y no se puede pasar del resto.
	if _, err := svc.Charge(ctx, app.ChargeCmd{
		OrderID: ord.ID, MethodID: efectivo, Amount: decimal.RequireFromString("151"), ActorID: cajero,
	}); !errors.Is(err, domain.ErrCobroExcede) {
		t.Fatalf("cobrar 151 cuando faltan 150 = %v, quiere ErrCobroExcede", err)
	}

	if _, err := svc.Charge(ctx, app.ChargeCmd{
		OrderID: ord.ID, MethodID: efectivo, Amount: decimal.RequireFromString("150"), ActorID: cajero,
	}); err != nil {
		t.Fatalf("completar: %v", err)
	}
	final, _ := svc.Detail(ctx, ord.ID)
	if !final.Paid {
		t.Fatal("con los $250 completos sigue por cobrar")
	}
}

// Un pedido cancelado ya repuso su stock: cobrarlo reportaría ingreso por comida que volvió al
// almacén.
func TestNoSeCobraUnPedidoCancelado(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	svc := app.NewOrdersService(st, clock)
	ord, cajero, efectivo := pedidoSinCobrar(t, st, svc, "cancelado")

	if err := svc.Cancel(ctx, ord.ID, cajero, "el cliente se fue"); err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	_, err := svc.Charge(ctx, app.ChargeCmd{
		OrderID: ord.ID, MethodID: efectivo, Amount: decimal.RequireFromString("250"), ActorID: cajero,
	})
	if !errors.Is(err, domain.ErrPedidoNoCobrable) {
		t.Fatalf("cobrar un cancelado = %v, quiere ErrPedidoNoCobrable", err)
	}
}

// El pago entra en el turno abierto AHORA. Es lo que hace que el corte del día cuadre contra el
// efectivo que de verdad está en el cajón.
func TestElCobroEntraEnElTurnoDeHoy(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	svc := app.NewOrdersService(st, clock)
	ord, cajero, efectivo := pedidoSinCobrar(t, st, svc, "turno")

	if _, err := svc.Charge(ctx, app.ChargeCmd{
		OrderID: ord.ID, MethodID: efectivo, Amount: decimal.RequireFromString("250"), ActorID: cajero,
	}); err != nil {
		t.Fatalf("Charge: %v", err)
	}
	var sesion *int64
	if err := st.Pool.QueryRow(ctx,
		`select register_session_id from order_payments where order_id = $1`, ord.ID).Scan(&sesion); err != nil {
		t.Fatalf("leer el pago: %v", err)
	}
	if sesion == nil {
		t.Fatal("el pago quedó sin turno: no aparecería en ningún corte")
	}
}
