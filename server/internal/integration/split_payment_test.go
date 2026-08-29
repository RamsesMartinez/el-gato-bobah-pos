//go:build integration

package integration

import (
	"context"
	"testing"
	"uuid"

	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
)

// Pago dividido: una orden puede cobrarse con más de un método. Se registran N filas en
// order_payments y la orden queda "pagada" cuando la suma de amounts cubre el total.
func TestSplitPayment(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	svc := app.NewOrdersService(st, clock)

	cashier := makeUser(t, st, "cajero_split", "cajero")
	prod := makeProduct(t, st, "Combo", decimal.RequireFromString("100"), false)
	cash := paymentMethodID(t, st, "Efectivo")
	card := paymentMethodID(t, st, "Tarjeta débito")
	abrirCajaPrincipal(t, st, cashier)

	ov, err := svc.Create(ctx, app.CreateOrderCmd{
		ClientUUID:  uuid.New(),
		ServiceType: "mostrador",
		OpenedBy:    cashier,
		Lines:       []domain.OrderLineInput{{ProductID: prod, Qty: decimal.RequireFromString("1")}},
		// $100 dividido: $60 efectivo + $40 tarjeta (propina $10 en la primera línea).
		Payments: []app.PaymentInput{
			{MethodID: cash, Amount: decimal.RequireFromString("60"), Tip: decimal.RequireFromString("10")},
			{MethodID: card, Amount: decimal.RequireFromString("40")},
		},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if !ov.Paid {
		t.Fatalf("60+40 cubre el total 100 → la orden debe quedar pagada")
	}

	// Dos filas de pago persistidas, con sus montos.
	rows, err := st.Q.ListOrderPayments(ctx, ov.ID)
	if err != nil {
		t.Fatalf("ListOrderPayments: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("esperaba 2 líneas de pago, hay %d", len(rows))
	}
	var sum decimal.Decimal
	for _, p := range rows {
		sum = sum.Add(p.Amount)
	}
	if !sum.Equal(decimal.RequireFromString("100")) {
		t.Fatalf("suma de pagos = %s, esperaba 100", sum)
	}

	// Un pago parcial NO marca pagada la orden (60 < 100).
	ov2, err := svc.Create(ctx, app.CreateOrderCmd{
		ClientUUID:  uuid.New(),
		ServiceType: "mostrador",
		OpenedBy:    cashier,
		Lines:       []domain.OrderLineInput{{ProductID: prod, Qty: decimal.RequireFromString("1")}},
		Payments:    []app.PaymentInput{{MethodID: cash, Amount: decimal.RequireFromString("60")}},
	})
	if err != nil {
		t.Fatalf("Create parcial: %v", err)
	}
	if ov2.Paid {
		t.Fatalf("60 < 100 → la orden NO debe quedar pagada")
	}
}
