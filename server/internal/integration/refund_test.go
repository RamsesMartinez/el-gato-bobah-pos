//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"
	"uuid"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store/db"
)

// Reembolso de una orden entregada: cambia a 'reembolsada', guarda el monto como pérdida,
// NO repone stock, es idempotente, y los reportes lo excluyen del ingreso y lo cuentan como
// devolución. Es el flujo nuevo y toca BD de punta a punta, así que va en integración.
func TestRefundFlow(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	svc := app.NewOrdersService(st, clock)

	cashier := makeUser(t, st, "cajero_refund", "cajero")
	admin := makeUser(t, st, "admin_refund", "admin")
	prod := makeProduct(t, st, "Crepa Nutella", decimal.RequireFromString("50"), true) // con stock

	// Crear orden (2 × 50 = 100) y entregarla.
	ov, err := svc.Create(ctx, app.CreateOrderCmd{
		ClientUUID:  uuid.New(),
		ServiceType: "mostrador",
		OpenedBy:    cashier,
		Lines:       []domain.OrderLineInput{{ProductID: prod, Qty: decimal.RequireFromString("2")}},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := svc.SetStatus(ctx, ov.ID, domain.StatusEntregada); err != nil {
		t.Fatalf("SetStatus entregada: %v", err)
	}
	movementsBefore := countOrderMovements(t, st, ov.ID)
	if movementsBefore == 0 {
		t.Fatal("la venta de un producto con stock debió generar un movimiento de depleción")
	}

	// Reembolsar.
	if err := svc.Refund(ctx, ov.ID, admin, "cliente insatisfecho"); err != nil {
		t.Fatalf("Refund: %v", err)
	}

	// Estado + monto + actor persistidos.
	o, err := st.Q.GetOrder(ctx, ov.ID)
	if err != nil {
		t.Fatalf("GetOrder: %v", err)
	}
	if string(o.Status) != domain.StatusReembolsada {
		t.Fatalf("status = %s, want reembolsada", o.Status)
	}
	if !o.RefundAmount.Equal(decimal.RequireFromString("100")) {
		t.Fatalf("refund_amount = %s, want 100", o.RefundAmount)
	}
	if o.RefundedBy == nil || *o.RefundedBy != admin {
		t.Fatalf("refunded_by no quedó seteado (%v)", o.RefundedBy)
	}

	// SIN restock: el reembolso es una pérdida (mercancía consumida), no repone el ledger.
	if got := countOrderMovements(t, st, ov.ID); got != movementsBefore {
		t.Fatalf("el reembolso no debe tocar el stock: movimientos %d → %d", movementsBefore, got)
	}

	// Idempotente: un segundo reembolso (doble-tap) → ErrConflict, no doble pérdida.
	if err := svc.Refund(ctx, ov.ID, admin, "otra vez"); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("segundo refund debe dar ErrConflict, got %v", err)
	}

	// Reportes: la orden reembolsada NO cuenta como ingreso; sí como pérdida por devolución.
	day := pgtype.Date{Time: fixedNow, Valid: true}
	sales, err := st.Q.SalesByDay(ctx, db.SalesByDayParams{BusinessDate: day, BusinessDate_2: day})
	if err != nil {
		t.Fatalf("SalesByDay: %v", err)
	}
	if len(sales) != 0 {
		t.Fatalf("la única orden está reembolsada; SalesByDay no debe contarla, got %+v", sales)
	}
	refunds, err := st.Q.RefundsByDay(ctx, db.RefundsByDayParams{BusinessDate: day, BusinessDate_2: day})
	if err != nil {
		t.Fatalf("RefundsByDay: %v", err)
	}
	if len(refunds) != 1 || !refunds[0].Amount.Equal(decimal.RequireFromString("100")) {
		t.Fatalf("RefundsByDay debe reportar 100 de pérdida, got %+v", refunds)
	}
}

// Solo se puede reembolsar desde 'entregada': una orden abierta se cancela, no se reembolsa.
func TestRefundRejectsNonDelivered(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	svc := app.NewOrdersService(st, clock)

	cashier := makeUser(t, st, "cajero_nd", "cajero")
	admin := makeUser(t, st, "admin_nd", "admin")
	prod := makeProduct(t, st, "Café", decimal.RequireFromString("30"), false)

	ov, err := svc.Create(ctx, app.CreateOrderCmd{
		ClientUUID:  uuid.New(),
		ServiceType: "mostrador",
		OpenedBy:    cashier,
		Lines:       []domain.OrderLineInput{{ProductID: prod, Qty: decimal.RequireFromString("1")}},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	// Sigue 'abierta' (no entregada).
	if err := svc.Refund(ctx, ov.ID, admin, "n/a"); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("reembolsar una orden no entregada debe dar ErrConflict, got %v", err)
	}
}
