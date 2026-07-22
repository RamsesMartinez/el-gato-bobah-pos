//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
)

func paymentMethodID(t *testing.T, st *store.Store, name string) int16 {
	t.Helper()
	var id int16
	if err := st.Pool.QueryRow(context.Background(),
		`select id from payment_methods where name = $1`, name).Scan(&id); err != nil {
		t.Fatalf("paymentMethodID(%s): %v", name, err)
	}
	return id
}

// Un método marcado auto_declare cierra SIEMPRE con declarado = esperado del servidor: el
// valor que el cliente mande al cerrar caja para ese método se ignora. Cubre de punta a
// punta (DB → servicio → persistencia) el control central de domain.ResolveDeclared, que
// evita que un front comprometido/con bug subdeclare un método que nunca requirió conteo.
func TestCloseSessionAutoDeclareIgnoresClientValue(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	ordersSvc := app.NewOrdersService(st, clock)
	backoffice := app.NewBackofficeService(st, clock)

	cashier := makeUser(t, st, "cajero_auto", "cajero")
	prod := makeProduct(t, st, "Café", decimal.RequireFromString("80"), false)
	cardID := paymentMethodID(t, st, "Tarjeta débito")

	if _, err := backoffice.SetPaymentMethodAutoDeclare(ctx, int(cardID), true); err != nil {
		t.Fatalf("SetPaymentMethodAutoDeclare: %v", err)
	}
	if _, err := backoffice.OpenSession(ctx, decimal.Zero, cashier); err != nil {
		t.Fatalf("OpenSession: %v", err)
	}

	// Venta de 80 con tarjeta (método auto_declare).
	if _, err := ordersSvc.Create(ctx, app.CreateOrderCmd{
		ClientUUID:  uuid.New(),
		ServiceType: "mostrador",
		OpenedBy:    cashier,
		Lines:       []domain.OrderLineInput{{ProductID: prod, Qty: decimal.RequireFromString("1")}},
		Payment:     &app.PaymentInput{MethodID: cardID, Amount: decimal.RequireFromString("80")},
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Cierre con un declarado FALSEADO (1, muy por debajo del esperado) para ese método.
	declared := map[int]decimal.Decimal{int(cardID): decimal.RequireFromString("1")}
	sess, err := backoffice.CloseSession(ctx, cashier, declared, "")
	if err != nil {
		t.Fatalf("CloseSession: %v", err)
	}

	var got *app.MethodTotal
	for i := range sess.Totals {
		if sess.Totals[i].MethodID == int(cardID) {
			got = &sess.Totals[i]
		}
	}
	if got == nil {
		t.Fatal("no hay total para el método de tarjeta en la respuesta")
	}
	if !got.Declared.Equal(decimal.RequireFromString("80")) {
		t.Fatalf("declared = %s, want 80 (esperado del servidor, no el 1 falseado por el cliente)", got.Declared)
	}
	if !got.Difference.IsZero() {
		t.Fatalf("difference = %s, want 0", got.Difference)
	}

	// Lo persistido en DB también refleja el esperado, no el valor falseado del cliente.
	var declaredDB, expectedDB decimal.Decimal
	if err := st.Pool.QueryRow(ctx,
		`select declared, expected from register_session_totals where session_id = $1 and payment_method_id = $2`,
		sess.ID, cardID).Scan(&declaredDB, &expectedDB); err != nil {
		t.Fatalf("leer register_session_totals: %v", err)
	}
	if !declaredDB.Equal(decimal.RequireFromString("80")) {
		t.Fatalf("declared persistido = %s, want 80", declaredDB)
	}
}

// Efectivo es justo el método que exige conteo físico: no debe poder marcarse auto_declare,
// o el corte de caja perdería la única forma de detectar un faltante de efectivo.
func TestSetPaymentMethodAutoDeclareRejectsCashDrawer(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	backoffice := app.NewBackofficeService(st, clock)

	cashID := paymentMethodID(t, st, "Efectivo")

	_, err := backoffice.SetPaymentMethodAutoDeclare(ctx, int(cashID), true)
	if !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("SetPaymentMethodAutoDeclare(Efectivo, true) = %v, want ErrValidation", err)
	}

	// Marcarlo false (el default) sigue permitido — el rechazo es solo al prender auto en
	// un método que afecta el cajón.
	if _, err := backoffice.SetPaymentMethodAutoDeclare(ctx, int(cashID), false); err != nil {
		t.Fatalf("SetPaymentMethodAutoDeclare(Efectivo, false): %v", err)
	}
}
