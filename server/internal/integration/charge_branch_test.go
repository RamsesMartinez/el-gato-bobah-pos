//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"
	"uuid"

	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
)

// CON DOS SUCURSALES Y SU CAJA ABIERTA CADA UNA, COBRAR NO ESCOGE TURNO AL AZAR (0076).
//
// El cobro amarra el pago al turno con `LockOpenPrimarySession`. Si esa consulta no filtra por
// sucursal, `limit 1` toma cualquiera de las dos cajas abiertas y el efectivo de la matriz aparece
// en el corte de NORTE: un sobrante allá y un faltante aquí, por el mismo monto, sin un solo error.
// Sin selector de sucursal, lo correcto es rechazar.
func TestChargingWithTwoOpenBranchesIsRejectedNotGuessed(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	cashier := makeUser(t, st, "cajero_dos_cajas", "cajero")
	product := makeProduct(t, st, "Malteada", decimal.RequireFromString("60.00"), false)
	abrirCajaPrincipal(t, st, cashier)

	svc := app.NewOrdersService(st, clock)
	order, err := svc.Create(ctx, app.CreateOrderCmd{
		ClientUUID: uuid.New(), ServiceType: "mostrador", OpenedBy: cashier,
		Lines: []domain.OrderLineInput{{ProductID: product, Qty: decimal.RequireFromString("1")}},
	})
	if err != nil {
		t.Fatalf("crear el pedido con una sucursal: %v", err)
	}

	_, northRegister := addBranch(t, st, defaultCompanyID, "NORTE")
	if _, err := st.Pool.Exec(ctx,
		`insert into register_sessions (business_date, opening_cash, opened_by, register_id) values ($1, 0, $2, $3)`,
		fixedNow, cashier, northRegister); err != nil {
		t.Fatalf("abrir la caja de NORTE: %v", err)
	}

	_, err = svc.Charge(ctx, app.ChargeCmd{
		OrderID: order.ID, MethodID: paymentMethodID(t, st, "Efectivo"),
		Amount: decimal.RequireFromString("60.00"), ActorID: cashier,
	})
	if !errors.Is(err, domain.ErrBranchAmbiguous) {
		var session int64
		_ = st.Pool.QueryRow(ctx, `select coalesce(max(register_session_id), 0) from order_payments where order_id = $1`, order.ID).Scan(&session)
		t.Fatalf("con dos cajas principales abiertas y sin sucursal elegida, cobrar debe rechazarse; "+
			"fue %v y el pago quedó en el turno %d", err, session)
	}
}
