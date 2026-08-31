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

// Un pedido que se cobra y se entrega en el mismo acto nace ENTREGADO.
//
// Es el refresco de mostrador: nada pasa por cocina y el cliente se va con él en la mano. Hacerlo
// viajar por el tablero —abierta, lista, entregada— son dos taps por una venta que ya terminó, y
// desde que la caja no cierra con pendientes, son dos taps que además bloquean el cierre.
func TestUnPedidoEntregadoEnElActoNaceEntregado(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	svc := app.NewOrdersService(st, clock)

	cajero := makeUser(t, st, "cajero_inmediato", "cajero")
	prod := makeProduct(t, st, "Refresco", decimal.RequireFromString("25"), false)
	sinPreparacion(t, st, prod)
	efectivo := paymentMethodID(t, st, "Efectivo")
	abrirCajaPrincipal(t, st, cajero)

	ord, err := svc.Create(ctx, app.CreateOrderCmd{
		ClientUUID:  uuid.New(),
		ServiceType: "mostrador",
		OpenedBy:    cajero,
		Lines:       []domain.OrderLineInput{{ProductID: prod, Qty: decimal.RequireFromString("1")}},
		Payments:    []app.PaymentInput{{MethodID: efectivo, Amount: decimal.RequireFromString("25")}},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if ord.Status != domain.StatusEntregada {
		t.Fatalf("estado = %s, quiere entregada", ord.Status)
	}

	// Y no aparece en el tablero: ya no hay nada que hacer con él.
	board, err := svc.Board(ctx)
	if err != nil {
		t.Fatalf("Board: %v", err)
	}
	for _, b := range board {
		if b.ID == ord.ID {
			t.Fatal("un pedido entregado en el acto no debe aparecer en el tablero")
		}
	}

	// Y por lo tanto no bloquea el cierre de caja.
	backoffice := app.NewBackofficeService(st, clock)
	principal := registerID(t, st, "Caja principal")
	if _, err := backoffice.CloseSession(ctx, principal, cajero,
		map[int]decimal.Decimal{int(efectivo): decimal.RequireFromString("25")}, ""); err != nil {
		t.Fatalf("no debe bloquear el cierre: %v", err)
	}
}

// Sin la marca, el pedido sigue naciendo abierto y pasa por el tablero. Es el caso de todos los
// días y no puede cambiar por agregar la opción.
func TestSinLaMarcaElPedidoSigueNaciendoAbierto(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	svc := app.NewOrdersService(st, clock)

	cajero := makeUser(t, st, "cajero_normal", "cajero")
	prod := makeProduct(t, st, "Café normal", decimal.RequireFromString("50"), false)
	efectivo := paymentMethodID(t, st, "Efectivo")
	abrirCajaPrincipal(t, st, cajero)

	ord, err := svc.Create(ctx, app.CreateOrderCmd{
		ClientUUID:  uuid.New(),
		ServiceType: "mostrador",
		OpenedBy:    cajero,
		Lines:       []domain.OrderLineInput{{ProductID: prod, Qty: decimal.RequireFromString("1")}},
		Payments:    []app.PaymentInput{{MethodID: efectivo, Amount: decimal.RequireFromString("50")}},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if ord.Status != domain.StatusAbierta {
		t.Fatalf("estado = %s, quiere abierta", ord.Status)
	}
}
