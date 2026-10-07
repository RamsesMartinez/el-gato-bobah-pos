//go:build integration

package integration

import (
	"context"
	"testing"
	"uuid"

	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
)

// EL ALMACÉN DESCUENTA LO QUE DE VERDAD SE VENDIÓ (spec 028).

func onHand(t *testing.T, st *store.Store, product int64) decimal.Decimal {
	t.Helper()
	var q decimal.Decimal
	if err := st.Pool.QueryRow(context.Background(),
		`select coalesce(sum(on_hand), 0) from stock_levels where product_id = $1`, product).Scan(&q); err != nil {
		t.Fatal(err)
	}
	return q
}

// CANCELAR UN RENGLÓN Y DESPUÉS EL PEDIDO REPONE UNA SOLA VEZ.
//
// `RestockCancelledOrder` invertía TODOS los movimientos de venta del pedido: lo que ya se había
// repuesto al cancelar un renglón volvía a entrar, y lo que ya se había preparado también. Con los
// extras y los componentes descontando, el sobrante falso crece con cada cancelación.
func TestCancellingALineThenTheOrderRestocksOnce(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	cashier := makeUser(t, st, "cajero_reponer", "cajero")
	soda := makeProduct(t, st, "Refresco reponer", decimal.RequireFromString("30"), true)
	water := makeProduct(t, st, "Agua reponer", decimal.RequireFromString("20"), true)
	abrirCajaPrincipal(t, st, cashier)
	svc := app.NewOrdersService(st, clock)

	order, err := svc.Create(ctx, app.CreateOrderCmd{
		ClientUUID: uuid.New(), ServiceType: "mostrador", OpenedBy: cashier,
		Lines: []domain.OrderLineInput{
			{ProductID: soda, Qty: decimal.RequireFromString("2")},
			{ProductID: water, Qty: decimal.RequireFromString("1")},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	var sodaLine int64
	for _, l := range order.Lines {
		if l.ProductName == "Refresco reponer" {
			sodaLine = l.ID
		}
	}
	// Confirmar manda todo a cocina; un renglón que todavía no sale (uno agregado y no enviado) es el
	// que se repone al cancelarlo. Se simula ese estado.
	if _, err := st.Pool.Exec(ctx, `update order_lines set enviado_a_cocina_at = null where id = $1`, sodaLine); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CancelarRenglon(ctx, order.ID, sodaLine, cashier, "el cliente cambió de opinión"); err != nil {
		t.Fatalf("cancelar el renglón: %v", err)
	}
	if got := onHand(t, st, soda); !got.IsZero() {
		t.Fatalf("cancelar el renglón antes de cocina repone el refresco: quería 0, hay %s", got)
	}
	if err := svc.CancelarConDevolucion(ctx, app.CancelacionCmd{OrderID: order.ID, Motivo: "se fue el cliente", ActorID: cashier}); err != nil {
		t.Fatalf("cancelar el pedido: %v", err)
	}
	if got := onHand(t, st, soda); !got.IsZero() {
		t.Fatalf("el refresco se repuso dos veces: quedaron %s de más en el almacén", got)
	}
	if got := onHand(t, st, water); !got.IsZero() {
		t.Fatalf("el agua se repone al cancelar el pedido, como hoy: quería 0, hay %s", got)
	}
}
