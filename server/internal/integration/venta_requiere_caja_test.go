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

// Sin caja abierta no se vende. Antes sí se podía: la orden se creaba con register_session_id en
// NULL y el corte la recogía después por ventana de tiempo, así que el dinero entraba a un arqueo
// que nadie abrió — o se perdía si nunca se abría uno.
func TestVentaSinCajaAbiertaSeRechaza(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	ordersSvc := app.NewOrdersService(st, clock)

	cajero := makeUser(t, st, "cajero_sin_caja", "cajero")
	prod := makeProduct(t, st, "Café", decimal.RequireFromString("80"), false)
	efectivo := paymentMethodID(t, st, "Efectivo")

	_, err := ordersSvc.Create(ctx, app.CreateOrderCmd{
		ClientUUID:  uuid.New(),
		ServiceType: "mostrador",
		OpenedBy:    cajero,
		Lines:       []domain.OrderLineInput{{ProductID: prod, Qty: decimal.RequireFromString("1")}},
		Payments:    []app.PaymentInput{{MethodID: efectivo, Amount: decimal.RequireFromString("80")}},
	})
	if !errors.Is(err, domain.ErrNoOpenRegister) {
		t.Fatalf("debe rechazarse con ErrNoOpenRegister, fue %v", err)
	}

	var n int
	if err := st.Pool.QueryRow(ctx, `select count(*) from orders`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("no debe quedar ninguna orden a medias, hay %d", n)
	}
}

// Con la caja principal abierta la venta pasa Y queda ATADA a esa sesión, tanto la orden como cada
// pago. Ese vínculo es lo que deja que el corte sume por sesión en vez de por hora.
func TestVentaConCajaPrincipalQuedaAtadaALaSesion(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	ordersSvc := app.NewOrdersService(st, clock)
	backoffice := app.NewBackofficeService(st, clock)

	cajero := makeUser(t, st, "cajero_con_caja", "cajero")
	prod := makeProduct(t, st, "Café", decimal.RequireFromString("80"), false)
	efectivo := paymentMethodID(t, st, "Efectivo")

	principal := registerID(t, st, "Caja principal")
	sess, err := backoffice.OpenSession(ctx, principal, decimal.Zero, cajero)
	if err != nil {
		t.Fatalf("OpenSession: %v", err)
	}

	ord, err := ordersSvc.Create(ctx, app.CreateOrderCmd{
		ClientUUID:  uuid.New(),
		ServiceType: "mostrador",
		OpenedBy:    cajero,
		Lines:       []domain.OrderLineInput{{ProductID: prod, Qty: decimal.RequireFromString("1")}},
		Payments:    []app.PaymentInput{{MethodID: efectivo, Amount: decimal.RequireFromString("80")}},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	var ordSess, paySess *int64
	if err := st.Pool.QueryRow(ctx,
		`select register_session_id from orders where id = $1`, ord.ID).Scan(&ordSess); err != nil {
		t.Fatal(err)
	}
	if err := st.Pool.QueryRow(ctx,
		`select register_session_id from order_payments where order_id = $1`, ord.ID).Scan(&paySess); err != nil {
		t.Fatal(err)
	}
	if ordSess == nil || *ordSess != sess.ID {
		t.Fatalf("la orden debe apuntar a la sesión %d, apunta a %v", sess.ID, ordSess)
	}
	if paySess == nil || *paySess != sess.ID {
		t.Fatalf("el pago debe apuntar a la sesión %d, apunta a %v", sess.ID, paySess)
	}
}

// Solo la caja PRINCIPAL habilita ventas. Las secundarias (caja fuerte, caja externa) existen para
// traspasos y gastos; si una de ellas abierta bastara para cobrar, el efectivo de la venta caería
// en un arqueo que no es el del mostrador.
func TestCajaSecundariaAbiertaNoHabilitaVender(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	ordersSvc := app.NewOrdersService(st, clock)
	backoffice := app.NewBackofficeService(st, clock)

	cajero := makeUser(t, st, "cajero_secundaria", "cajero")
	prod := makeProduct(t, st, "Café", decimal.RequireFromString("80"), false)
	efectivo := paymentMethodID(t, st, "Efectivo")

	var secundaria int64
	if err := st.Pool.QueryRow(ctx,
		`select id from cash_registers where not is_primary and is_active order by id limit 1`).Scan(&secundaria); err != nil {
		t.Fatalf("no hay caja secundaria de referencia: %v", err)
	}
	if _, err := backoffice.OpenSession(ctx, secundaria, decimal.Zero, cajero); err != nil {
		t.Fatalf("OpenSession(secundaria): %v", err)
	}

	_, err := ordersSvc.Create(ctx, app.CreateOrderCmd{
		ClientUUID:  uuid.New(),
		ServiceType: "mostrador",
		OpenedBy:    cajero,
		Lines:       []domain.OrderLineInput{{ProductID: prod, Qty: decimal.RequireFromString("1")}},
		Payments:    []app.PaymentInput{{MethodID: efectivo, Amount: decimal.RequireFromString("80")}},
	})
	if !errors.Is(err, domain.ErrNoOpenRegister) {
		t.Fatalf("una caja secundaria no debe habilitar la venta, fue %v", err)
	}
}

// /cash-status es lo que el POS consulta para decidir si muestra la pantalla de venta. Tiene que
// contestar la MISMA pregunta que el cobro, o la pantalla deja armar un ticket entero para tronar
// al final: una caja secundaria abierta no es "hay caja".
func TestEstadoDeCajaSigueLaMismaReglaQueElCobro(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	backoffice := app.NewBackofficeService(st, clock)
	cajero := makeUser(t, st, "cajero_estado", "cajero")

	abierta, err := backoffice.SellingRegisterOpen(ctx)
	if err != nil {
		t.Fatalf("SellingRegisterOpen: %v", err)
	}
	if abierta {
		t.Fatal("sin ninguna caja abierta debe decir que no")
	}

	var secundaria int64
	if err := st.Pool.QueryRow(ctx,
		`select id from cash_registers where not is_primary and is_active order by id limit 1`).Scan(&secundaria); err != nil {
		t.Fatal(err)
	}
	if _, err := backoffice.OpenSession(ctx, secundaria, decimal.Zero, cajero); err != nil {
		t.Fatalf("OpenSession(secundaria): %v", err)
	}
	if abierta, err = backoffice.SellingRegisterOpen(ctx); err != nil {
		t.Fatalf("SellingRegisterOpen: %v", err)
	}
	if abierta {
		t.Fatal("una caja secundaria abierta no habilita vender, y el estado no debe decir que sí")
	}

	if _, err := backoffice.OpenSession(ctx, registerID(t, st, "Caja principal"), decimal.Zero, cajero); err != nil {
		t.Fatalf("OpenSession(principal): %v", err)
	}
	if abierta, err = backoffice.SellingRegisterOpen(ctx); err != nil {
		t.Fatalf("SellingRegisterOpen: %v", err)
	}
	if !abierta {
		t.Fatal("con la principal abierta debe decir que sí")
	}
}
