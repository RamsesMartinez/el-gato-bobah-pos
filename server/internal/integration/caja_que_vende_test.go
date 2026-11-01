//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/shopspring/decimal"
	"uuid"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
)

// cajaQueVendeYCierra deja la caja principal con un turno que vendió $80 en efectivo y ya cerró.
// Devuelve la caja y el turno.
func cajaQueVendeYCierra(t *testing.T, st *store.Store, cajero int64) (reg, sess int64) {
	t.Helper()
	ctx := context.Background()
	orders := app.NewOrdersService(st, clock)
	back := app.NewBackofficeService(st, clock)
	reg = principalRegister(t, st)
	abierta, err := back.OpenSession(ctx, reg, app.AperturaCmd{}, cajero)
	if err != nil {
		t.Fatalf("abrir: %v", err)
	}
	prod := makeProduct(t, st, "Café 049", decimal.RequireFromString("80"), false)
	if _, err := crearYCobrar(t, ctx, orders, app.CreateOrderCmd{
		ClientUUID: uuid.New(), ServiceType: "mostrador", OpenedBy: cajero,
		Lines:    []domain.OrderLineInput{{ProductID: prod, Qty: decimal.RequireFromString("1")}},
		Payments: []app.PaymentInput{{MethodID: paymentMethodID(t, st, "Efectivo"), Amount: decimal.RequireFromString("80")}},
	}); err != nil {
		t.Fatalf("vender: %v", err)
	}
	entregarPendientes(t, st)
	if _, err := st.Pool.Exec(ctx, `update register_sessions set status = 'cerrada', closed_at = now(), closed_by = $2 where id = $1`, abierta.ID, cajero); err != nil {
		t.Fatalf("cerrar: %v", err)
	}
	return reg, abierta.ID
}

// R1: QUÉ CAJA VENDIÓ UN TURNO PASADO NO SE LEE DE QUÉ CAJA VENDE HOY. A vende y cierra; se pasa la
// venta a B. «Hasta cuándo se ve un entregado» y el detalle del corte de A no pueden cambiar: si
// cambian, pasar la venta reescribe el pasado.
func TestMovingSalesDoesNotRewriteThePastOfTheOldRegister(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()
	back := app.NewBackofficeService(st, clock)
	cajero := makeUser(t, st, "cajero_049_pasado", "cajero")
	regA, sessA := cajaQueVendeYCierra(t, st, cajero)

	cerroAntes, err := st.Q.CerroLaCajaPrincipal(ctx)
	if err != nil {
		t.Fatalf("último cierre antes de pasar la venta: %v", err)
	}
	detalleAntes, err := back.SessionDetail(ctx, sessA)
	if err != nil {
		t.Fatal(err)
	}

	var regB int64
	if err := st.Pool.QueryRow(ctx, `insert into cash_registers (name) values ('Barra 049') returning id`).Scan(&regB); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Pool.Exec(ctx, `update cash_registers set is_primary = false where id = $1`, regA); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Pool.Exec(ctx, `update cash_registers set is_primary = true, counts_cash = true where id = $1`, regB); err != nil {
		t.Fatal(err)
	}

	cerroDespues, err := st.Q.CerroLaCajaPrincipal(ctx)
	if err != nil {
		t.Fatalf("último cierre tras pasar la venta a otra caja: %v (los entregados del día dejarían de tener corte)", err)
	}
	if !cerroDespues.Time.Equal(cerroAntes.Time) {
		t.Fatalf("el último cierre pasó de %v a %v solo por cambiar qué caja vende", cerroAntes.Time, cerroDespues.Time)
	}
	detalleDespues, err := back.SessionDetail(ctx, sessA)
	if err != nil {
		t.Fatal(err)
	}
	a, _ := json.Marshal(detalleAntes)
	b, _ := json.Marshal(detalleDespues)
	if string(a) != string(b) {
		t.Fatalf("el corte de la caja vieja cambió al pasar la venta:\nantes   %s\ndespués %s", a, b)
	}
}
